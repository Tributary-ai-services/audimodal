// Package events provides event schemas and utilities for the event-driven processing pipeline.
// Events flow through Kafka topics to coordinate processing between different components.
package events

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Event types used throughout the processing pipeline
const (
	EventFileDiscovered     = "file.discovered"
	EventFileResolved       = "file.resolved"
	EventFileClassified     = "file.classified"
	EventFileChunked        = "file.chunked"
	EventChunkEmbedded      = "chunk.embedded"
	EventDLPViolation       = "dlp.violation"
	EventProcessingComplete = "processing.complete"
	EventProcessingFailed   = "processing.failed"
	EventTenantCreated      = "tenant.created"
	EventTenantUpdated      = "tenant.updated"
	EventSessionStarted     = "session.started"
	EventSessionCompleted   = "session.completed"
)

// BaseEvent provides common fields for all events in the system
type BaseEvent struct {
	ID        string                 `json:"id"`                   // Unique event identifier
	Type      string                 `json:"type"`                 // Event type constant
	Source    string                 `json:"source"`               // Component that generated the event
	Time      time.Time              `json:"time"`                 // When the event occurred
	TenantID  string                 `json:"tenant_id"`            // Tenant identifier
	SessionID string                 `json:"session_id,omitempty"` // Processing session ID
	TraceID   string                 `json:"trace_id,omitempty"`   // Distributed tracing ID
	UserID    string                 `json:"user_id,omitempty"`    // User who triggered the event
	Metadata  map[string]interface{} `json:"metadata,omitempty"`   // Additional event metadata
}

// NewBaseEvent creates a new base event with generated ID and current timestamp
func NewBaseEvent(eventType, source, tenantID string) BaseEvent {
	return BaseEvent{
		ID:       uuid.New().String(),
		Type:     eventType,
		Source:   source,
		Time:     time.Now(),
		TenantID: tenantID,
		Metadata: make(map[string]interface{}),
	}
}

// FileDiscoveredEvent is published when new files are discovered from a data source
type FileDiscoveredEvent struct {
	BaseEvent
	Data FileDiscoveredData `json:"data"`
}

type FileDiscoveredData struct {
	URL          string            `json:"url"`           // File URL
	SourceID     string            `json:"source_id"`     // DataSource that discovered this file
	DiscoveredAt time.Time         `json:"discovered_at"` // When file was discovered
	Size         int64             `json:"size"`          // File size in bytes
	ContentType  string            `json:"content_type"`  // MIME content type
	Metadata     map[string]string `json:"metadata"`      // File metadata from source
	Priority     string            `json:"priority"`      // Processing priority
}

// FileResolvedEvent is published after URL resolution determines how to access a file
type FileResolvedEvent struct {
	BaseEvent
	Data FileResolvedData `json:"data"`
}

type FileResolvedData struct {
	URL        string     `json:"url"`               // Original file URL
	Scheme     string     `json:"scheme"`            // URL scheme (file, s3, etc.)
	Size       int64      `json:"size"`              // Actual file size
	AccessInfo AccessInfo `json:"access_info"`       // How to access the file
	Tier       int        `json:"processing_tier"`   // Assigned processing tier (1, 2, 3)
	Strategy   string     `json:"chunking_strategy"` // Recommended chunking strategy
}

type AccessInfo struct {
	RequiresAuth bool              `json:"requires_auth"`
	Credentials  string            `json:"credentials,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	Region       string            `json:"region,omitempty"`
}

// FileClassifiedEvent is published after content classification and DLP scanning
type FileClassifiedEvent struct {
	BaseEvent
	Data FileClassifiedData `json:"data"`
}

type FileClassifiedData struct {
	URL                string                 `json:"url"`
	DataClass          string                 `json:"data_class"`          // public, internal, confidential, restricted
	PIITypes           []string               `json:"pii_types"`           // Types of PII detected
	ComplianceFlags    []string               `json:"compliance_flags"`    // GDPR, HIPAA, PCI, etc.
	ProcessingRules    ProcessingRules        `json:"processing_rules"`    // Derived processing requirements
	DLPViolations      []DLPViolation         `json:"dlp_violations"`      // Any DLP violations found
	ClassificationMeta ClassificationMetadata `json:"classification_meta"` // Additional classification info
}

type ProcessingRules struct {
	EncryptionRequired    bool     `json:"encryption_required"`
	AnonymizationRequired bool     `json:"anonymization_required"`
	AuditTrailRequired    bool     `json:"audit_trail_required"`
	ConsentRequired       bool     `json:"consent_required"`
	DataLocality          []string `json:"data_locality"`       // Geographic restrictions
	EgressRestrictions    []string `json:"egress_restrictions"` // Export restrictions
	AccessRestrictions    []string `json:"access_restrictions"` // Who can access
	RetentionPolicy       string   `json:"retention_policy"`    // How long to keep
}

type DLPViolation struct {
	PolicyID      string   `json:"policy_id"`
	RuleID        string   `json:"rule_id"`
	ViolationType string   `json:"violation_type"`
	Severity      string   `json:"severity"`
	PIITypes      []string `json:"pii_types"`
	ActionsTaken  []string `json:"actions_taken"`
	Location      string   `json:"location"` // Where in file violation occurred
}

type ClassificationMetadata struct {
	Confidence   float64  `json:"confidence"`    // Confidence in classification (0.0-1.0)
	Language     string   `json:"language"`      // Detected language
	ContentHints []string `json:"content_hints"` // Additional content characteristics
	ScanDuration string   `json:"scan_duration"` // Time taken for classification
}

// FileChunkedEvent is published after a file has been chunked using a strategy
type FileChunkedEvent struct {
	BaseEvent
	Data FileChunkedData `json:"data"`
}

type FileChunkedData struct {
	URL             string          `json:"url"`
	ChunkCount      int             `json:"chunk_count"`
	Strategy        string          `json:"strategy"` // Strategy used for chunking
	ChunkMetadata   []ChunkInfo     `json:"chunks"`   // Metadata for each chunk
	ProcessingStats ProcessingStats `json:"processing_stats"`
}

type ChunkInfo struct {
	ChunkID     string  `json:"chunk_id"`
	Sequence    int     `json:"sequence"`     // Order in file
	Size        int     `json:"size"`         // Chunk size in bytes
	StartOffset int64   `json:"start_offset"` // Start position in original file
	EndOffset   int64   `json:"end_offset"`   // End position in original file
	ChunkType   string  `json:"chunk_type"`   // text, table, image, etc.
	Quality     float64 `json:"quality"`      // Quality score (0.0-1.0)
}

type ProcessingStats struct {
	ProcessingTime   time.Duration `json:"processing_time"`
	MemoryUsed       int64         `json:"memory_used_bytes"`
	ChunksGenerated  int           `json:"chunks_generated"`
	ChunksFiltered   int           `json:"chunks_filtered"` // Chunks rejected for quality
	AverageChunkSize int           `json:"average_chunk_size"`
}

// ChunkEmbeddedEvent is published after embeddings are generated for a chunk
type ChunkEmbeddedEvent struct {
	BaseEvent
	Data ChunkEmbeddedData `json:"data"`
}

type ChunkEmbeddedData struct {
	URL            string         `json:"url"`
	ChunkID        string         `json:"chunk_id"`
	VectorID       string         `json:"vector_id"`       // ID in vector database
	Model          string         `json:"embedding_model"` // Model used for embedding
	Dimension      int            `json:"dimension"`       // Vector dimension
	VectorStore    string         `json:"vector_store"`    // Which vector store was used
	EmbeddingStats EmbeddingStats `json:"embedding_stats"`
}

type EmbeddingStats struct {
	TokenCount    int           `json:"token_count"`
	EmbeddingTime time.Duration `json:"embedding_time"`
	VectorNorm    float64       `json:"vector_norm"`
	StorageTime   time.Duration `json:"storage_time"`
}

// DLPViolationEvent is published when DLP violations are detected
type DLPViolationEvent struct {
	BaseEvent
	Data DLPViolationData `json:"data"`
}

type DLPViolationData struct {
	URL           string            `json:"url"`
	PolicyID      string            `json:"policy_id"`
	ViolationType string            `json:"violation_type"`
	Severity      string            `json:"severity"`
	PIITypes      []string          `json:"pii_types"`
	ActionsTaken  []string          `json:"actions_taken"`
	Location      ViolationLocation `json:"location"`
	Context       string            `json:"context"` // Surrounding text for context
}

type ViolationLocation struct {
	ChunkID      string `json:"chunk_id,omitempty"`
	StartOffset  int64  `json:"start_offset"`
	EndOffset    int64  `json:"end_offset"`
	LineNumber   int    `json:"line_number,omitempty"`
	ColumnNumber int    `json:"column_number,omitempty"`
}

// ProcessingCompleteEvent is published when all processing for a file is finished
type ProcessingCompleteEvent struct {
	BaseEvent
	Data ProcessingCompleteData `json:"data"`
}

type ProcessingCompleteData struct {
	URL                 string        `json:"url"`
	TotalProcessingTime time.Duration `json:"total_processing_time"`
	ChunksCreated       int           `json:"chunks_created"`
	EmbeddingsCreated   int           `json:"embeddings_created"`
	DLPViolationsFound  int           `json:"dlp_violations_found"`
	FinalDataClass      string        `json:"final_data_class"`
	StorageLocation     string        `json:"storage_location"`
	Success             bool          `json:"success"`
}

// ProcessingFailedEvent is published when processing fails
type ProcessingFailedEvent struct {
	BaseEvent
	Data ProcessingFailedData `json:"data"`
}

type ProcessingFailedData struct {
	URL          string                 `json:"url"`
	FailureStage string                 `json:"failure_stage"` // resolution, classification, chunking, embedding
	ErrorCode    string                 `json:"error_code"`
	ErrorMessage string                 `json:"error_message"`
	RetryCount   int                    `json:"retry_count"`
	Retryable    bool                   `json:"retryable"`
	Context      map[string]interface{} `json:"context"`
}

// SessionStartedEvent is published when a processing session begins
type SessionStartedEvent struct {
	BaseEvent
	Data SessionStartedData `json:"data"`
}

type SessionStartedData struct {
	SessionName string         `json:"session_name"`
	FileCount   int            `json:"file_count"`
	Priority    string         `json:"priority"`
	Options     SessionOptions `json:"options"`
}

type SessionOptions struct {
	ChunkingStrategy  string   `json:"chunking_strategy"`
	EmbeddingTypes    []string `json:"embedding_types"`
	DLPScanEnabled    bool     `json:"dlp_scan_enabled"`
	ForceReprocessing bool     `json:"force_reprocessing"`
}

// SessionCompletedEvent is published when a processing session finishes
type SessionCompletedEvent struct {
	BaseEvent
	Data SessionCompletedData `json:"data"`
}

type SessionCompletedData struct {
	SessionName       string        `json:"session_name"`
	Duration          time.Duration `json:"duration"`
	FilesProcessed    int           `json:"files_processed"`
	FilesSucceeded    int           `json:"files_succeeded"`
	FilesFailed       int           `json:"files_failed"`
	ChunksGenerated   int           `json:"chunks_generated"`
	EmbeddingsCreated int           `json:"embeddings_created"`
	DLPViolations     int           `json:"dlp_violations"`
	FinalStatus       string        `json:"final_status"` // completed, failed, cancelled
}

// EventEnvelope wraps events with routing and delivery metadata
type EventEnvelope struct {
	Event       json.RawMessage `json:"event"`         // The actual event payload
	EventType   string          `json:"event_type"`    // Type for routing
	TenantID    string          `json:"tenant_id"`     // For partitioning
	Priority    int             `json:"priority"`      // Processing priority (0=low, 10=high)
	PublishedAt time.Time       `json:"published_at"`  // When published to topic
	TTL         time.Duration   `json:"ttl,omitempty"` // Time to live
	Retries     int             `json:"retries"`       // Retry count
	DeadLetter  bool            `json:"dead_letter"`   // Should go to dead letter queue
}

// Helper functions for event creation

// NewFileDiscoveredEvent creates a file discovered event
func NewFileDiscoveredEvent(source, tenantID string, data FileDiscoveredData) *FileDiscoveredEvent {
	return &FileDiscoveredEvent{
		BaseEvent: NewBaseEvent(EventFileDiscovered, source, tenantID),
		Data:      data,
	}
}

// NewFileResolvedEvent creates a file resolved event
func NewFileResolvedEvent(source, tenantID string, data FileResolvedData) *FileResolvedEvent {
	return &FileResolvedEvent{
		BaseEvent: NewBaseEvent(EventFileResolved, source, tenantID),
		Data:      data,
	}
}

// NewFileClassifiedEvent creates a file classified event
func NewFileClassifiedEvent(source, tenantID string, data FileClassifiedData) *FileClassifiedEvent {
	return &FileClassifiedEvent{
		BaseEvent: NewBaseEvent(EventFileClassified, source, tenantID),
		Data:      data,
	}
}

// NewFileChunkedEvent creates a file chunked event
func NewFileChunkedEvent(source, tenantID string, data FileChunkedData) *FileChunkedEvent {
	return &FileChunkedEvent{
		BaseEvent: NewBaseEvent(EventFileChunked, source, tenantID),
		Data:      data,
	}
}

// NewChunkEmbeddedEvent creates a chunk embedded event
func NewChunkEmbeddedEvent(source, tenantID string, data ChunkEmbeddedData) *ChunkEmbeddedEvent {
	return &ChunkEmbeddedEvent{
		BaseEvent: NewBaseEvent(EventChunkEmbedded, source, tenantID),
		Data:      data,
	}
}

// NewProcessingCompleteEvent creates a processing complete event
func NewProcessingCompleteEvent(source, tenantID string, data ProcessingCompleteData) *ProcessingCompleteEvent {
	return &ProcessingCompleteEvent{
		BaseEvent: NewBaseEvent(EventProcessingComplete, source, tenantID),
		Data:      data,
	}
}

// NewProcessingFailedEvent creates a processing failed event
func NewProcessingFailedEvent(source, tenantID string, data ProcessingFailedData) *ProcessingFailedEvent {
	return &ProcessingFailedEvent{
		BaseEvent: NewBaseEvent(EventProcessingFailed, source, tenantID),
		Data:      data,
	}
}

// EventValidator provides validation for events
type EventValidator struct{}

// NewEventValidator creates a new event validator
func NewEventValidator() *EventValidator {
	return &EventValidator{}
}

// ValidateEvent validates an event structure
func (v *EventValidator) ValidateEvent(event interface{}) error {
	// Basic validation logic here
	// In a real implementation, this would use JSON schema validation
	// or similar comprehensive validation

	// Check if event has required BaseEvent fields
	switch e := event.(type) {
	case *FileDiscoveredEvent:
		return v.validateBaseEvent(e.BaseEvent)
	case *FileResolvedEvent:
		return v.validateBaseEvent(e.BaseEvent)
	case *FileClassifiedEvent:
		return v.validateBaseEvent(e.BaseEvent)
	case *FileChunkedEvent:
		return v.validateBaseEvent(e.BaseEvent)
	case *ChunkEmbeddedEvent:
		return v.validateBaseEvent(e.BaseEvent)
	default:
		return fmt.Errorf("unknown event type")
	}
}

// EventHandler defines the unified interface for handling events
type EventHandler interface {
	// HandleEvent processes a single event
	HandleEvent(ctx context.Context, event interface{}) error

	// GetEventTypes returns the event types this handler can process
	GetEventTypes() []string

	// GetName returns the handler name for logging
	GetName() string
}

func (v *EventValidator) validateBaseEvent(base BaseEvent) error {
	if base.ID == "" {
		return fmt.Errorf("event ID is required")
	}
	if base.Type == "" {
		return fmt.Errorf("event type is required")
	}
	if base.Source == "" {
		return fmt.Errorf("event source is required")
	}
	if base.TenantID == "" {
		return fmt.Errorf("tenant ID is required")
	}
	if base.Time.IsZero() {
		return fmt.Errorf("event time is required")
	}
	return nil
}

// EventBusMetrics represents metrics for the event bus
type EventBusMetrics struct {
	EventsPublished  int64                      `json:"events_published"`
	EventsProcessed  int64                      `json:"events_processed"`
	EventsFailed     int64                      `json:"events_failed"`
	HandlersActive   int                        `json:"handlers_active"`
	QueueDepth       int                        `json:"queue_depth"`
	ProcessingTime   time.Duration              `json:"avg_processing_time"`
	LastProcessedAt  *time.Time                 `json:"last_processed_at"`
	ErrorRate        float64                    `json:"error_rate"`
	ThroughputPerSec float64                    `json:"throughput_per_sec"`
	HandlerMetrics   map[string]*HandlerMetrics `json:"handler_metrics"`
}

// HandlerMetrics represents metrics for individual event handlers
type HandlerMetrics struct {
	EventsHandled     int64         `json:"events_handled"`
	EventsFailed      int64         `json:"events_failed"`
	AvgProcessingTime time.Duration `json:"avg_processing_time"`
	LastHandledAt     *time.Time    `json:"last_handled_at"`
	ErrorRate         float64       `json:"error_rate"`
}

// WorkflowDefinition represents a processing workflow
type WorkflowDefinition struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	Description string    `json:"description"`
	TenantID    uuid.UUID `json:"tenant_id"`

	// Workflow steps
	Steps []*WorkflowStep `json:"steps"`

	// Configuration
	Config WorkflowConfig `json:"config"`

	// Metadata
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	CreatedBy string    `json:"created_by"`
	Status    string    `json:"status"` // active, inactive, draft
}

// WorkflowStep represents a step in a processing workflow
type WorkflowStep struct {
	Name         string                 `json:"name"`
	Type         string                 `json:"type"` // handler, condition, parallel, sequential
	Handler      string                 `json:"handler,omitempty"`
	Condition    *StepCondition         `json:"condition,omitempty"`
	Config       map[string]interface{} `json:"config,omitempty"`
	Dependencies []string               `json:"dependencies,omitempty"`
	OnSuccess    []string               `json:"on_success,omitempty"`
	OnFailure    []string               `json:"on_failure,omitempty"`
	Timeout      *time.Duration         `json:"timeout,omitempty"`
	Retries      int                    `json:"retries,omitempty"`
	Parallel     bool                   `json:"parallel,omitempty"`
}

// StepCondition represents a condition for conditional workflow steps
type StepCondition struct {
	Expression string                 `json:"expression"`
	Context    map[string]interface{} `json:"context,omitempty"`
}

// WorkflowConfig represents configuration for a workflow
type WorkflowConfig struct {
	MaxRetries      int           `json:"max_retries"`
	RetryDelay      time.Duration `json:"retry_delay"`
	Timeout         time.Duration `json:"timeout"`
	ParallelLimit   int           `json:"parallel_limit"`
	ErrorHandling   string        `json:"error_handling"` // stop, continue, retry
	NotifyOnFailure bool          `json:"notify_on_failure"`
	NotifyOnSuccess bool          `json:"notify_on_success"`
}

// WorkflowExecution represents an execution of a workflow
type WorkflowExecution struct {
	ID           uuid.UUID   `json:"id"`
	WorkflowID   uuid.UUID   `json:"workflow_id"`
	TenantID     uuid.UUID   `json:"tenant_id"`
	SessionID    *uuid.UUID  `json:"session_id,omitempty"`
	TriggerEvent interface{} `json:"trigger_event,omitempty"`

	Status   string `json:"status"`   // pending, running, completed, failed, cancelled
	Progress int    `json:"progress"` // percentage

	// Step execution tracking
	StepExecutions map[string]*StepExecution `json:"step_executions"`

	// Context and results
	Context map[string]interface{} `json:"context"`
	Results map[string]interface{} `json:"results"`

	// Timestamps
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`

	// Error tracking
	LastError  *string `json:"last_error,omitempty"`
	ErrorCount int     `json:"error_count"`
}

// StepExecution represents the execution of a workflow step
type StepExecution struct {
	StepName      string                 `json:"step_name"`
	Status        string                 `json:"status"`
	StartedAt     *time.Time             `json:"started_at,omitempty"`
	CompletedAt   *time.Time             `json:"completed_at,omitempty"`
	Duration      *time.Duration         `json:"duration,omitempty"`
	RetryCount    int                    `json:"retry_count"`
	LastError     *string                `json:"last_error,omitempty"`
	Results       map[string]interface{} `json:"results,omitempty"`
	EventsEmitted []interface{}          `json:"events_emitted,omitempty"`
}
