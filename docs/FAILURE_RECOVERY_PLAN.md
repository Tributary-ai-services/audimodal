# File Upload Failure Handling & Recovery Improvement Plan

> **Document Version**: 1.0
> **Created**: 2026-01-17
> **Status**: Approved for Implementation

## GitHub Issues

| Issue | Title | Priority | Status |
|-------|-------|----------|--------|
| [#8](https://github.com/Tributary-ai-services/audimodal/issues/8) | Implement Processing Checkpoint System | High | Open |
| [#9](https://github.com/Tributary-ai-services/audimodal/issues/9) | Implement Pod Failure Detection & Recovery Service | High | Open |
| [#10](https://github.com/Tributary-ai-services/audimodal/issues/10) | Implement Dead Letter Queue for Failed Events | Medium | Open |
| [#11](https://github.com/Tributary-ai-services/audimodal/issues/11) | Implement Circuit Breakers for External Dependencies | Medium | Open |
| [#12](https://github.com/Tributary-ai-services/audimodal/issues/12) | Implement Idempotent Processing | Medium | Open |
| [#13](https://github.com/Tributary-ai-services/audimodal/issues/13) | Implement Chaos Testing Framework | Low | Open |

---

## Problem Statement

When a pod dies during file processing or an error occurs mid-upload, the TAS platform lacks mechanisms to detect and recover. Files can remain stuck in "processing" status indefinitely with no way to resume or restart.

## Current Architecture Analysis

### Strengths
- **State tracking models** - File, Chunk, ProcessingSession with clear status fields
- **Retry at DLP level** - 3 retries with 5-second delays for DLP scans
- **Async processing** - Background goroutines with 30-minute timeout
- **Kafka event publishing** - Events for cross-service communication

### Critical Gaps Identified

| Gap | Impact | Location |
|-----|--------|----------|
| **No checkpoint persistence** | Pod death loses all processing progress | `pipeline.go:136-182` |
| **In-memory session state** | `activeSessions` map lost on restart | `session.go:19` |
| **Stub DLQ implementation** | Failed events logged, not queued | `bus.go:43` |
| **No pod failure detection** | Files stuck in "processing" forever | N/A |
| **No circuit breakers** | Cascading failures possible | N/A |
| **No idempotency** | Duplicate processing on retry | N/A |

---

## Implementation Plan

### Phase 1: Checkpoint System (Week 1-2)

**GitHub Issue**: [#8](https://github.com/Tributary-ai-services/audimodal/issues/8)

#### 1.1 Create ProcessingCheckpoint Model

**File**: `internal/database/models/checkpoint.go`

```go
type ProcessingCheckpoint struct {
    ID                 uuid.UUID  // Primary key
    TenantID           uuid.UUID  // Multi-tenant isolation
    FileID             uuid.UUID  // File being processed
    ProcessorPodID     string     // Pod ownership tracking
    LastHeartbeat      time.Time  // Stale detection
    LastChunkNumber    int        // Resume point
    TotalChunksExpected *int
    BytesProcessed     int64
    IteratorState      JSONB      // Reader-specific state for resume
    ReaderType         string
    StrategyType       string
    LastError          *string
    ErrorCount         int
}
```

#### 1.2 Implement CheckpointManager

**File**: `internal/processors/checkpoint_manager.go`

Key methods:
- `CreateCheckpoint()` - Called at processing start
- `UpdateCheckpoint()` - Called every N chunks or M seconds
- `StartHeartbeat()` - Background goroutine updating last_heartbeat
- `FindStaleCheckpoints()` - Query for heartbeat > 5 minutes ago
- `ClaimOrphanedProcessing()` - Atomic claim with pod ID

#### 1.3 Integrate into Pipeline

**File**: `internal/processors/pipeline.go`

Modify `ProcessFile()` at line 136:
1. Create checkpoint at start
2. Update checkpoint every 10 chunks or 60 seconds
3. Store iterator state for resumability
4. Mark checkpoint complete on success/failure

---

### Phase 2: Pod Failure Recovery (Week 3-4)

**GitHub Issue**: [#9](https://github.com/Tributary-ai-services/audimodal/issues/9)

#### 2.1 Pod Identification

**File**: `cmd/main.go`

```go
func getPodID() string {
    if podName := os.Getenv("POD_NAME"); podName != "" {
        return podName  // Kubernetes pod name
    }
    return fmt.Sprintf("%s-%s", hostname, uuid.New().String()[:8])
}
```

#### 2.2 Recovery Service

**File**: `internal/processors/recovery_service.go`

```go
type RecoveryService struct {
    checkpointManager *CheckpointManager
    pipeline          *Pipeline
    config            *RecoveryConfig
}

type RecoveryConfig struct {
    ScanInterval       time.Duration  // 1 minute
    StaleThreshold     time.Duration  // 5 minutes
    MaxConcurrentRecovery int         // 3
}
```

Recovery logic:
1. On startup: scan for stale checkpoints
2. Periodically: scan every `ScanInterval`
3. For each stale checkpoint:
   - Attempt atomic claim (prevents race conditions)
   - If `LastChunkNumber > 0`: resume from checkpoint
   - Else: restart from beginning
4. Also reset files stuck in "processing" without checkpoints

#### 2.3 Enhanced Health Checks

**File**: `internal/server/handlers/health.go`

Add `/health/processing` endpoint:
- Report active processing count
- Report stuck processing count
- Return 503 if stuck > threshold

---

### Phase 3: Dead Letter Queue (Week 5-6)

**GitHub Issue**: [#10](https://github.com/Tributary-ai-services/audimodal/issues/10)

#### 3.1 DLQ Topics

| Topic | Purpose |
|-------|---------|
| `dlq.processing.failed` | File processing failures |
| `dlq.embedding.failed` | Embedding generation failures |
| `dlq.dlp.failed` | DLP scan failures |

#### 3.2 DLQ Message Structure

```go
type DLQMessage struct {
    OriginalTopic    string
    OriginalEvent    interface{}
    FailureReason    string
    FailureDetails   string
    FailedAt         time.Time
    RetryCount       int
    TenantID         string
    FileID           string
    CorrelationID    string
}
```

#### 3.3 Implementation

**File**: `pkg/events/dlq_producer.go`

Replace stub in `bus.go` line 43 (`DeadLetterEnabled`) with actual Kafka publishing:

```go
func (dlq *DLQProducer) SendToDeadLetter(ctx context.Context, msg DLQMessage) error {
    topic := dlq.determineTopic(msg.FailureReason)
    return dlq.writer.WriteMessages(ctx, kafka.Message{
        Topic: topic,
        Key:   []byte(msg.CorrelationID),
        Value: serializedMsg,
        Headers: []kafka.Header{
            {Key: "dlq-reason", Value: []byte(msg.FailureReason)},
            {Key: "retry-count", Value: []byte(strconv.Itoa(msg.RetryCount))},
        },
    })
}
```

---

### Phase 4: Circuit Breakers (Week 7)

**GitHub Issue**: [#11](https://github.com/Tributary-ai-services/audimodal/issues/11)

#### 4.1 Circuit Breaker Implementation

**File**: `pkg/resilience/circuit_breaker.go`

States: Closed -> Open -> Half-Open -> Closed

```go
type CircuitBreakerConfig struct {
    MaxFailures      int           // 5 failures to open
    ResetTimeout     time.Duration // 30s before half-open
    HalfOpenMaxCalls int           // 3 test calls
    SuccessThreshold int           // 3 successes to close
}
```

#### 4.2 Apply to Dependencies

**File**: `internal/processors/protected_pipeline.go`

```go
type ProtectedPipeline struct {
    cbDeepLake  *CircuitBreaker  // Vector storage
    cbMinIO     *CircuitBreaker  // File storage
    cbPostgres  *CircuitBreaker  // Database
    cbLLMRouter *CircuitBreaker  // LLM calls
}
```

---

### Phase 5: Idempotency (Week 8)

**GitHub Issue**: [#12](https://github.com/Tributary-ai-services/audimodal/issues/12)

#### 5.1 Event Deduplication Table

**File**: `migrations/XXXXXX_add_processed_events.sql`

```sql
CREATE TABLE processed_events (
    id UUID PRIMARY KEY,
    idempotency_key VARCHAR(512) UNIQUE,
    tenant_id UUID NOT NULL,
    processed_at TIMESTAMP NOT NULL,
    expires_at TIMESTAMP NOT NULL
);
CREATE INDEX idx_processed_events_expires ON processed_events(expires_at);
```

#### 5.2 Idempotency Service

**File**: `internal/services/idempotency_service.go`

```go
func (s *IdempotencyService) CheckAndMark(ctx context.Context, key string) (alreadyProcessed bool, err error)
```

Generate idempotency key: `process:{tenant_id}:{file_id}:{minute_bucket}`

---

### Phase 6: Chaos Testing (Week 9-10)

**GitHub Issue**: [#13](https://github.com/Tributary-ai-services/audimodal/issues/13)

#### 6.1 Test Scenarios

| Scenario | Description | Expected Outcome |
|----------|-------------|------------------|
| Pod kill mid-processing | Kill pod at 50% progress | Recovery completes file, no duplicates |
| Database unavailable | Block DB during checkpoint | Circuit breaker opens, retries on restore |
| Kafka unavailable | Block Kafka during event publish | Event queued or circuit breaker, eventual delivery |
| Concurrent recovery | Two pods claim same file | Only one succeeds, atomic claim |

#### 6.2 Implementation

**File**: `internal/testing/chaos/`

```go
type ChaosFramework struct {
    scenarios map[string]ChaosScenario
}

// Scenarios:
// - PodKillDuringProcessingTest
// - DatabaseUnavailableTest
// - KafkaUnavailableTest
// - ConcurrentRecoveryTest
```

---

## Database Migrations

### Migration 1: Processing Checkpoints

```sql
CREATE TABLE processing_checkpoints (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    file_id UUID NOT NULL REFERENCES files(id),
    processor_pod_id VARCHAR(255) NOT NULL,
    last_heartbeat TIMESTAMP NOT NULL,
    last_chunk_number INTEGER DEFAULT 0,
    bytes_processed BIGINT DEFAULT 0,
    iterator_state JSONB,
    reader_type VARCHAR(100),
    strategy_type VARCHAR(100),
    last_error TEXT,
    error_count INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_checkpoints_stale ON processing_checkpoints(last_heartbeat)
    WHERE last_heartbeat < NOW() - INTERVAL '5 minutes';
```

### Migration 2: Event Deduplication

```sql
CREATE TABLE processed_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    idempotency_key VARCHAR(512) UNIQUE NOT NULL,
    tenant_id UUID NOT NULL,
    processed_at TIMESTAMP DEFAULT NOW(),
    expires_at TIMESTAMP NOT NULL
);
```

---

## Critical Files to Modify

| File | Changes |
|------|---------|
| `internal/processors/pipeline.go` | Add checkpoint integration at lines 136-182 |
| `internal/processors/session.go` | Persist SessionContext to DB, add heartbeat |
| `pkg/events/bus.go` | Replace stub DLQ with Kafka publishing |
| `pkg/events/producer.go` | Implement real Kafka producer |
| `internal/server/handlers/file.go` | Add idempotency check in ProcessFile |
| `aether-be/internal/services/kafka.go` | Add retry logic and DLQ handling |

---

## Observability

### Prometheus Metrics

```go
audimodal_processing_checkpoint_created_total
audimodal_processing_recovery_attempts_total{outcome="success|failed"}
audimodal_circuit_breaker_state{circuit_name="..."}
audimodal_dlq_messages_total{topic="...", reason="..."}
audimodal_processing_stuck_files_gauge
```

### Alerting Rules

```yaml
- alert: ProcessingStuckFiles
  expr: audimodal_file_status{status="processing"}
        and on() (time() - audimodal_file_processing_started > 1800)
  for: 5m
  severity: warning

- alert: CircuitBreakerOpen
  expr: audimodal_circuit_breaker_state > 0
  for: 2m
  severity: critical

- alert: HighDLQRate
  expr: rate(audimodal_dlq_messages_total[5m]) > 1
  for: 5m
  severity: warning
```

---

## Verification Approach

### Unit Tests
- `TestCheckpointManager_CreateAndRecover`
- `TestCheckpointManager_HeartbeatExpiry`
- `TestCheckpointManager_ConcurrentClaims`
- `TestCircuitBreaker_OpenOnFailures`
- `TestIdempotencyService_DeduplicateEvents`

### Integration Tests
- `TestProcessing_PodFailureRecovery` - Full recovery flow
- `TestProcessing_DatabaseUnavailable` - Circuit breaker behavior
- `TestProcessing_NoDuplicateChunks` - Idempotency verification

### Acceptance Criteria

1. **Pod failure recovery**: File completes within 5 minutes of pod death
2. **No duplicate chunks**: After recovery, chunk numbers are sequential
3. **Circuit breaker**: Opens within configured threshold, no cascade
4. **DLQ completeness**: All failed events in DLQ with full context
5. **Tenant isolation**: Recovery maintains strict isolation

---

## Implementation Timeline

| Phase | Duration | Deliverables |
|-------|----------|--------------|
| 1. Checkpointing | 2 weeks | Checkpoint model, manager, pipeline integration |
| 2. Recovery | 2 weeks | Recovery service, pod identification, health checks |
| 3. DLQ | 1 week | DLQ producer, Kafka integration, bus.go update |
| 4. Circuit Breakers | 1 week | Circuit breaker package, dependency protection |
| 5. Idempotency | 1 week | Dedup table, idempotency service |
| 6. Chaos Testing | 2 weeks | Test framework, scenarios, CI integration |

**Total Duration**: ~10 weeks
