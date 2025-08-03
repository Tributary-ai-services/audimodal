package sync

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jscharber/eAIIngest/pkg/storage"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// SyncOrchestrator manages sync operations across all connectors
type SyncOrchestrator struct {
	connectorRegistry storage.ConnectorRegistry
	jobManager        *SyncJobManager
	webhookManager    *WebhookManager
	metricsCollector  *SyncMetricsCollector
	scheduler         *SyncScheduler

	// Internal state
	activeJobs map[uuid.UUID]*SyncJob
	jobsMutex  sync.RWMutex

	// Configuration
	config *OrchestratorConfig
	tracer trace.Tracer
}

// OrchestratorConfig contains orchestrator configuration
type OrchestratorConfig struct {
	MaxConcurrentSyncs   int             `json:"max_concurrent_syncs"`
	DefaultSyncTimeout   time.Duration   `json:"default_sync_timeout"`
	JobRetentionDuration time.Duration   `json:"job_retention_duration"`
	MetricsRetentionDays int             `json:"metrics_retention_days"`
	EnableRealtimeSync   bool            `json:"enable_realtime_sync"`
	EnableCrossConnector bool            `json:"enable_cross_connector"`
	ThrottleLimits       *ThrottleConfig `json:"throttle_limits"`
}

// ThrottleConfig defines rate limiting configuration
type ThrottleConfig struct {
	RequestsPerSecond  int           `json:"requests_per_second"`
	BurstLimit         int           `json:"burst_limit"`
	BackoffMultiplier  float64       `json:"backoff_multiplier"`
	MaxBackoffDuration time.Duration `json:"max_backoff_duration"`
}

// NewSyncOrchestrator creates a new sync orchestrator
func NewSyncOrchestrator(registry storage.ConnectorRegistry, config *OrchestratorConfig) *SyncOrchestrator {
	if config == nil {
		config = &OrchestratorConfig{
			MaxConcurrentSyncs:   10,
			DefaultSyncTimeout:   time.Hour,
			JobRetentionDuration: 7 * 24 * time.Hour,
			MetricsRetentionDays: 30,
			EnableRealtimeSync:   true,
			EnableCrossConnector: false,
			ThrottleLimits: &ThrottleConfig{
				RequestsPerSecond:  10,
				BurstLimit:         20,
				BackoffMultiplier:  2.0,
				MaxBackoffDuration: 5 * time.Minute,
			},
		}
	}

	orchestrator := &SyncOrchestrator{
		connectorRegistry: registry,
		activeJobs:        make(map[uuid.UUID]*SyncJob),
		config:            config,
		tracer:            otel.Tracer("sync-orchestrator"),
	}

	// Initialize components
	orchestrator.jobManager = NewSyncJobManager(config)
	orchestrator.webhookManager = NewWebhookManager(nil) // Use default webhook config
	orchestrator.metricsCollector = NewSyncMetricsCollector(config.MetricsRetentionDays)
	orchestrator.scheduler = NewSyncScheduler(orchestrator, nil) // Use default scheduler config

	return orchestrator
}

// StartSync initiates a new sync operation
func (o *SyncOrchestrator) StartSync(ctx context.Context, request *StartSyncRequest) (*SyncJob, error) {
	ctx, span := o.tracer.Start(ctx, "orchestrator.start_sync")
	defer span.End()

	span.SetAttributes(
		attribute.String("data_source_id", request.DataSourceID.String()),
		attribute.String("sync_type", string(request.Options.SyncType)),
		attribute.String("direction", string(request.Options.Direction)),
	)

	// Validate request
	if err := o.validateSyncRequest(request); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("invalid sync request: %w", err)
	}

	// Check concurrent sync limits
	if err := o.checkConcurrencyLimits(request.DataSourceID); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("concurrency limit exceeded: %w", err)
	}

	// Get connector
	connector, err := o.connectorRegistry.Get(request.ConnectorType)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get connector: %w", err)
	}

	// Create sync job
	job := o.createSyncJob(request, connector)

	// Register job
	o.registerJob(job)

	// Start sync operation in background
	go o.executeSyncJob(context.Background(), job)

	// Record metrics
	o.metricsCollector.RecordSyncStarted(job)

	return job, nil
}

// GetSyncStatus retrieves the status of a sync job
func (o *SyncOrchestrator) GetSyncStatus(ctx context.Context, jobID uuid.UUID) (*SyncJobStatus, error) {
	ctx, span := o.tracer.Start(ctx, "orchestrator.get_sync_status")
	defer span.End()

	span.SetAttributes(attribute.String("job_id", jobID.String()))

	o.jobsMutex.RLock()
	job, exists := o.activeJobs[jobID]
	o.jobsMutex.RUnlock()

	if !exists {
		// Check completed jobs in storage
		return o.jobManager.GetJobStatus(ctx, jobID)
	}

	return job.GetStatus(), nil
}

// CancelSync cancels an active sync operation
func (o *SyncOrchestrator) CancelSync(ctx context.Context, jobID uuid.UUID) error {
	ctx, span := o.tracer.Start(ctx, "orchestrator.cancel_sync")
	defer span.End()

	span.SetAttributes(attribute.String("job_id", jobID.String()))

	o.jobsMutex.RLock()
	job, exists := o.activeJobs[jobID]
	o.jobsMutex.RUnlock()

	if !exists {
		return ErrSyncJobNotFound
	}

	// Cancel the job
	job.Cancel()

	// Record metrics
	o.metricsCollector.RecordSyncCancelled(job)

	return nil
}

// ListActiveSyncs returns all currently active sync operations
func (o *SyncOrchestrator) ListActiveSyncs(ctx context.Context, filters *SyncListFilters) ([]*SyncJob, error) {
	ctx, span := o.tracer.Start(ctx, "orchestrator.list_active_syncs")
	defer span.End()

	o.jobsMutex.RLock()
	defer o.jobsMutex.RUnlock()

	var jobs []*SyncJob
	for _, job := range o.activeJobs {
		if filters == nil || o.matchesFilters(job, filters) {
			jobs = append(jobs, job)
		}
	}

	span.SetAttributes(attribute.Int("active_jobs_count", len(jobs)))

	return jobs, nil
}

// GetSyncHistory returns historical sync operations
func (o *SyncOrchestrator) GetSyncHistory(ctx context.Context, filters *SyncHistoryFilters) (*SyncHistoryResult, error) {
	ctx, span := o.tracer.Start(ctx, "orchestrator.get_sync_history")
	defer span.End()

	return o.jobManager.GetSyncHistory(ctx, filters)
}

// GetSyncMetrics returns aggregated sync metrics
func (o *SyncOrchestrator) GetSyncMetrics(ctx context.Context, request *SyncMetricsRequest) (*SyncMetricsResult, error) {
	ctx, span := o.tracer.Start(ctx, "orchestrator.get_sync_metrics")
	defer span.End()

	return o.metricsCollector.GetMetrics(ctx, request)
}

// RegisterWebhook registers a webhook endpoint for real-time sync events
func (o *SyncOrchestrator) RegisterWebhook(ctx context.Context, config *WebhookConfig) error {
	ctx, span := o.tracer.Start(ctx, "orchestrator.register_webhook")
	defer span.End()

	// TODO: Convert WebhookConfig to WebhookSubscription
	// For now, return unimplemented error
	return fmt.Errorf("webhook registration not yet implemented")
}

// UnregisterWebhook removes a webhook endpoint
func (o *SyncOrchestrator) UnregisterWebhook(ctx context.Context, webhookID uuid.UUID) error {
	ctx, span := o.tracer.Start(ctx, "orchestrator.unregister_webhook")
	defer span.End()

	// TODO: Convert webhookID to subscription ID
	// For now, return unimplemented error
	return fmt.Errorf("webhook unregistration not yet implemented")
}

// ScheduleSync schedules a sync operation to run at specified intervals
func (o *SyncOrchestrator) ScheduleSync(ctx context.Context, schedule *SyncSchedule) (*ScheduledSync, error) {
	ctx, span := o.tracer.Start(ctx, "orchestrator.schedule_sync")
	defer span.End()

	err := o.scheduler.CreateSchedule(ctx, schedule)
	if err != nil {
		return nil, err
	}
	// TODO: Return proper ScheduledSync object
	return &ScheduledSync{}, nil
}

// UnscheduleSync removes a scheduled sync operation
func (o *SyncOrchestrator) UnscheduleSync(ctx context.Context, scheduleID uuid.UUID) error {
	ctx, span := o.tracer.Start(ctx, "orchestrator.unschedule_sync")
	defer span.End()

	return o.scheduler.DeleteSchedule(ctx, scheduleID)
}

// Internal methods

func (o *SyncOrchestrator) validateSyncRequest(request *StartSyncRequest) error {
	if request.DataSourceID == uuid.Nil {
		return ErrInvalidDataSourceID
	}

	if request.ConnectorType == "" {
		return ErrInvalidConnectorType
	}

	if request.Options == nil {
		return ErrInvalidSyncOptions
	}

	// Validate sync options
	if request.Options.SyncType == "" {
		request.Options.SyncType = SyncTypeFull
	}

	if request.Options.Direction == "" {
		request.Options.Direction = SyncDirectionBidirectional
	}

	return nil
}

func (o *SyncOrchestrator) checkConcurrencyLimits(dataSourceID uuid.UUID) error {
	o.jobsMutex.RLock()
	defer o.jobsMutex.RUnlock()

	// Check global limit
	if len(o.activeJobs) >= o.config.MaxConcurrentSyncs {
		return ErrMaxConcurrentSyncsExceeded
	}

	// Check per-data-source limit (max 1 sync per data source)
	for _, job := range o.activeJobs {
		if job.DataSourceID == dataSourceID && job.Status.State != SyncStateCompleted && job.Status.State != SyncStateFailed {
			return ErrSyncAlreadyInProgress
		}
	}

	return nil
}

func (o *SyncOrchestrator) createSyncJob(request *StartSyncRequest, connector storage.StorageConnector) *SyncJob {
	jobID := uuid.New()

	job := &SyncJob{
		ID:            jobID,
		DataSourceID:  request.DataSourceID,
		ConnectorType: request.ConnectorType,
		Connector:     connector,
		Options:       request.Options,
		Status: &SyncJobStatus{
			JobID:     jobID,
			State:     SyncStatePending,
			StartTime: time.Now(),
			Progress: &SyncProgress{
				TotalFiles:      0,
				ProcessedFiles:  0,
				PercentComplete: 0,
			},
		},
		cancelCtx:    context.Background(),
		orchestrator: o,
	}

	// Set up cancellation
	job.cancelCtx, job.cancelFunc = context.WithCancel(context.Background())

	return job
}

func (o *SyncOrchestrator) registerJob(job *SyncJob) {
	o.jobsMutex.Lock()
	defer o.jobsMutex.Unlock()

	o.activeJobs[job.ID] = job
}

func (o *SyncOrchestrator) unregisterJob(jobID uuid.UUID) {
	o.jobsMutex.Lock()
	defer o.jobsMutex.Unlock()

	delete(o.activeJobs, jobID)
}

func (o *SyncOrchestrator) executeSyncJob(ctx context.Context, job *SyncJob) {
	defer o.unregisterJob(job.ID)

	// Set timeout
	ctx, cancel := context.WithTimeout(job.cancelCtx, o.config.DefaultSyncTimeout)
	defer cancel()

	// Update job state
	job.updateState(SyncStateRunning)

	// Execute sync based on type
	var err error
	switch job.Options.SyncType {
	case SyncTypeFull:
		err = o.executeFullSync(ctx, job)
	case SyncTypeIncremental:
		err = o.executeIncrementalSync(ctx, job)
	case SyncTypeRealtime:
		err = o.executeRealtimeSync(ctx, job)
	default:
		err = fmt.Errorf("unsupported sync type: %s", job.Options.SyncType)
	}

	// Update final state
	if err != nil {
		job.updateState(SyncStateFailed)
		job.Status.Error = err.Error()
		o.metricsCollector.RecordSyncFailed(job, err)
	} else {
		job.updateState(SyncStateCompleted)
		o.metricsCollector.RecordSyncCompleted(job)
	}

	// Store job result
	o.jobManager.StoreJobResult(ctx, job)

	// Send webhook notifications
	// TODO: Implement job completion notification
	_ = job // Suppress unused variable warning
}

func (o *SyncOrchestrator) executeFullSync(ctx context.Context, job *SyncJob) error {
	// Implement full sync logic
	// This would typically involve:
	// 1. List all files from connector
	// 2. Compare with local state
	// 3. Sync differences
	// 4. Update progress throughout

	// For now, simulate sync progress
	totalFiles := 100 // This would come from connector
	job.Status.Progress.TotalFiles = totalFiles

	for i := 0; i < totalFiles; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Simulate file processing
		time.Sleep(10 * time.Millisecond)

		// Update progress
		job.Status.Progress.ProcessedFiles = i + 1
		job.Status.Progress.PercentComplete = float64(i+1) / float64(totalFiles) * 100
		job.Status.LastActivity = time.Now()

		// Notify progress
		// TODO: Implement progress notification
		_ = job // Suppress unused variable warning
	}

	return nil
}

func (o *SyncOrchestrator) executeIncrementalSync(ctx context.Context, job *SyncJob) error {
	// Implement incremental sync logic
	// This would use change detection mechanisms specific to each connector
	return o.executeFullSync(ctx, job) // Simplified for now
}

func (o *SyncOrchestrator) executeRealtimeSync(ctx context.Context, job *SyncJob) error {
	if !o.config.EnableRealtimeSync {
		return ErrRealtimeSyncDisabled
	}

	// Implement real-time sync logic
	// This would set up webhook listeners or polling mechanisms
	return o.executeFullSync(ctx, job) // Simplified for now
}

func (o *SyncOrchestrator) matchesFilters(job *SyncJob, filters *SyncListFilters) bool {
	if filters.DataSourceID != uuid.Nil && job.DataSourceID != filters.DataSourceID {
		return false
	}

	if filters.ConnectorType != "" && job.ConnectorType != filters.ConnectorType {
		return false
	}

	if len(filters.States) > 0 {
		found := false
		for _, state := range filters.States {
			if job.Status.State == state {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// Shutdown gracefully shuts down the orchestrator
func (o *SyncOrchestrator) Shutdown(ctx context.Context) error {
	// Cancel all active jobs
	o.jobsMutex.RLock()
	for _, job := range o.activeJobs {
		job.Cancel()
	}
	o.jobsMutex.RUnlock()

	// Shutdown components
	if err := o.scheduler.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown scheduler: %w", err)
	}

	if err := o.webhookManager.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown webhook manager: %w", err)
	}

	if err := o.jobManager.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown job manager: %w", err)
	}

	return nil
}
