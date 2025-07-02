package processors

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/jscharber/eAIIngest/pkg/events"
)

// EventHandler handles processing-related events
type EventHandler struct {
	coordinator *Coordinator
	producer    *events.Producer
}

// ProcessingEventData represents data for processing events
type ProcessingEventData struct {
	TenantID    uuid.UUID `json:"tenant_id"`
	SessionID   uuid.UUID `json:"session_id,omitempty"`
	FileID      uuid.UUID `json:"file_id,omitempty"`
	Status      string    `json:"status"`
	Progress    float64   `json:"progress,omitempty"`
	Message     string    `json:"message,omitempty"`
	Error       string    `json:"error,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
}

// NewEventHandler creates a new event handler
func NewEventHandler(coordinator *Coordinator, producer *events.Producer) *EventHandler {
	return &EventHandler{
		coordinator: coordinator,
		producer:    producer,
	}
}

// HandleFileDiscovered handles file discovery events
func (eh *EventHandler) HandleFileDiscovered(ctx context.Context, event *events.Event) error {
	var eventData map[string]any
	if err := json.Unmarshal(event.Data, &eventData); err != nil {
		return fmt.Errorf("failed to unmarshal event data: %w", err)
	}

	tenantIDStr, ok := eventData["tenant_id"].(string)
	if !ok {
		return fmt.Errorf("missing tenant_id in event data")
	}

	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		return fmt.Errorf("invalid tenant_id: %w", err)
	}

	fileIDStr, ok := eventData["file_id"].(string)
	if !ok {
		return fmt.Errorf("missing file_id in event data")
	}

	fileID, err := uuid.Parse(fileIDStr)
	if err != nil {
		return fmt.Errorf("invalid file_id: %w", err)
	}

	// Check if auto-processing is enabled
	autoProcess, _ := eventData["auto_process"].(bool)
	if autoProcess {
		// Start processing automatically
		options := map[string]any{
			"priority": "normal",
			"dlp_scan_enabled": true,
		}

		// Process file asynchronously
		go func() {
			_, err := eh.coordinator.ProcessSingleFile(context.Background(), tenantID, fileID, options)
			if err != nil {
				// Emit error event
				eh.emitProcessingEvent("file.processing.failed", tenantID, fileID, uuid.Nil, "failed", 0, err.Error(), nil)
			}
		}()
	}

	return nil
}

// HandleProcessingRequested handles explicit processing requests
func (eh *EventHandler) HandleProcessingRequested(ctx context.Context, event *events.Event) error {
	var eventData map[string]any
	if err := json.Unmarshal(event.Data, &eventData); err != nil {
		return fmt.Errorf("failed to unmarshal event data: %w", err)
	}

	tenantIDStr, ok := eventData["tenant_id"].(string)
	if !ok {
		return fmt.Errorf("missing tenant_id in event data")
	}

	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		return fmt.Errorf("invalid tenant_id: %w", err)
	}

	// Check if it's a single file or multiple files
	if fileIDStr, ok := eventData["file_id"].(string); ok {
		// Single file processing
		fileID, err := uuid.Parse(fileIDStr)
		if err != nil {
			return fmt.Errorf("invalid file_id: %w", err)
		}

		options := eh.extractProcessingOptions(eventData)
		
		go func() {
			result, err := eh.coordinator.ProcessSingleFile(context.Background(), tenantID, fileID, options)
			if err != nil {
				eh.emitProcessingEvent("file.processing.failed", tenantID, fileID, uuid.Nil, "failed", 0, err.Error(), nil)
			} else {
				metadata := map[string]any{
					"chunks_created":   result.ChunksCreated,
					"bytes_processed":  result.BytesProcessed,
					"processing_time":  result.ProcessingTime,
					"quality_score":    result.QualityScore,
				}
				eh.emitProcessingEvent("file.processing.completed", tenantID, fileID, uuid.Nil, "completed", 1.0, "", metadata)
			}
		}()
	} else if fileIDsRaw, ok := eventData["file_ids"].([]interface{}); ok {
		// Multiple files processing
		var fileIDs []uuid.UUID
		for _, fidRaw := range fileIDsRaw {
			if fidStr, ok := fidRaw.(string); ok {
				if fid, err := uuid.Parse(fidStr); err == nil {
					fileIDs = append(fileIDs, fid)
				}
			}
		}

		if len(fileIDs) > 0 {
			options := eh.extractProcessingOptions(eventData)
			
			go func() {
				job, err := eh.coordinator.ProcessMultipleFiles(context.Background(), tenantID, fileIDs, options)
				if err != nil {
					eh.emitProcessingEvent("batch.processing.failed", tenantID, uuid.Nil, uuid.Nil, "failed", 0, err.Error(), nil)
				} else {
					metadata := map[string]any{
						"job_id":    job.ID,
						"session_id": job.SessionID,
						"file_count": len(fileIDs),
					}
					eh.emitProcessingEvent("batch.processing.started", tenantID, uuid.Nil, *job.SessionID, "started", 0, "", metadata)
				}
			}()
		}
	}

	return nil
}

// HandleDataSourceSync handles data source synchronization events
func (eh *EventHandler) HandleDataSourceSync(ctx context.Context, event *events.Event) error {
	var eventData map[string]any
	if err := json.Unmarshal(event.Data, &eventData); err != nil {
		return fmt.Errorf("failed to unmarshal event data: %w", err)
	}

	tenantIDStr, ok := eventData["tenant_id"].(string)
	if !ok {
		return fmt.Errorf("missing tenant_id in event data")
	}

	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		return fmt.Errorf("invalid tenant_id: %w", err)
	}

	dataSourceIDStr, ok := eventData["data_source_id"].(string)
	if !ok {
		return fmt.Errorf("missing data_source_id in event data")
	}

	dataSourceID, err := uuid.Parse(dataSourceIDStr)
	if err != nil {
		return fmt.Errorf("invalid data_source_id: %w", err)
	}

	// Check if auto-processing is enabled for this data source
	autoProcess, _ := eventData["auto_process"].(bool)
	if autoProcess {
		options := eh.extractProcessingOptions(eventData)
		
		go func() {
			job, err := eh.coordinator.ProcessDataSource(context.Background(), tenantID, dataSourceID, options)
			if err != nil {
				eh.emitProcessingEvent("datasource.processing.failed", tenantID, uuid.Nil, uuid.Nil, "failed", 0, err.Error(), nil)
			} else {
				metadata := map[string]any{
					"job_id":         job.ID,
					"session_id":     job.SessionID,
					"data_source_id": dataSourceID,
				}
				eh.emitProcessingEvent("datasource.processing.started", tenantID, uuid.Nil, *job.SessionID, "started", 0, "", metadata)
			}
		}()
	}

	return nil
}

// HandleSessionStatusUpdate handles session status update events
func (eh *EventHandler) HandleSessionStatusUpdate(ctx context.Context, event *events.Event) error {
	var eventData map[string]any
	if err := json.Unmarshal(event.Data, &eventData); err != nil {
		return fmt.Errorf("failed to unmarshal event data: %w", err)
	}

	sessionIDStr, ok := eventData["session_id"].(string)
	if !ok {
		return fmt.Errorf("missing session_id in event data")
	}

	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		return fmt.Errorf("invalid session_id: %w", err)
	}

	// Get session progress and emit appropriate events
	progress, err := eh.coordinator.GetSessionStatus(sessionID)
	if err != nil {
		return fmt.Errorf("failed to get session status: %w", err)
	}

	// Emit progress event
	metadata := map[string]any{
		"files_total":      progress.FilesTotal,
		"files_processed":  progress.FilesProcessed,
		"files_failed":     progress.FilesFailed,
		"chunks_created":   progress.ChunksCreated,
		"bytes_processed":  progress.BytesProcessed,
		"elapsed_time":     progress.ElapsedTime,
		"estimated_left":   progress.EstimatedTimeLeft,
	}

	eventType := "session.progress.updated"
	if progress.Status == "completed" {
		eventType = "session.completed"
	} else if progress.Status == "failed" {
		eventType = "session.failed"
	}

	return eh.emitProcessingEvent(eventType, uuid.Nil, uuid.Nil, progress.SessionID, progress.Status, progress.Progress, "", metadata)
}

// extractProcessingOptions extracts processing options from event data
func (eh *EventHandler) extractProcessingOptions(eventData map[string]any) map[string]any {
	options := make(map[string]any)

	if priority, ok := eventData["priority"].(string); ok {
		options["priority"] = priority
	}

	if dlpEnabled, ok := eventData["dlp_scan_enabled"].(bool); ok {
		options["dlp_scan_enabled"] = dlpEnabled
	}

	if readerType, ok := eventData["reader_type"].(string); ok {
		options["reader_type"] = readerType
	}

	if strategyType, ok := eventData["strategy_type"].(string); ok {
		options["strategy_type"] = strategyType
	}

	if readerConfig, ok := eventData["reader_config"].(map[string]any); ok {
		options["reader_config"] = readerConfig
	}

	if strategyConfig, ok := eventData["strategy_config"].(map[string]any); ok {
		options["strategy_config"] = strategyConfig
	}

	if complianceRules, ok := eventData["compliance_rules"].([]interface{}); ok {
		var rules []string
		for _, rule := range complianceRules {
			if ruleStr, ok := rule.(string); ok {
				rules = append(rules, ruleStr)
			}
		}
		options["compliance_rules"] = rules
	}

	return options
}

// emitProcessingEvent emits a processing-related event
func (eh *EventHandler) emitProcessingEvent(eventType string, tenantID, fileID, sessionID uuid.UUID, status string, progress float64, errorMsg string, metadata map[string]any) error {
	if eh.producer == nil {
		return nil // No producer configured
	}

	eventData := ProcessingEventData{
		Status:    status,
		Progress:  progress,
		Timestamp: time.Now(),
	}

	if tenantID != uuid.Nil {
		eventData.TenantID = tenantID
	}
	if fileID != uuid.Nil {
		eventData.FileID = fileID
	}
	if sessionID != uuid.Nil {
		eventData.SessionID = sessionID
	}
	if errorMsg != "" {
		eventData.Error = errorMsg
	}
	if metadata != nil {
		eventData.Metadata = metadata
	}

	data, err := json.Marshal(eventData)
	if err != nil {
		return fmt.Errorf("failed to marshal event data: %w", err)
	}

	event := &events.Event{
		ID:        uuid.New().String(),
		Type:      eventType,
		Source:    "processing.coordinator",
		Data:      data,
		Timestamp: time.Now(),
	}

	if tenantID != uuid.Nil {
		event.TenantID = tenantID.String()
	}

	return eh.producer.Publish(context.Background(), event)
}

// RegisterEventHandlers registers all processing event handlers
func (eh *EventHandler) RegisterEventHandlers(consumer *events.Consumer) error {
	// Register handlers for different event types
	handlers := map[string]func(context.Context, *events.Event) error{
		"file.discovered":         eh.HandleFileDiscovered,
		"processing.requested":    eh.HandleProcessingRequested,
		"datasource.synced":       eh.HandleDataSourceSync,
		"session.status.update":   eh.HandleSessionStatusUpdate,
	}

	for eventType, handler := range handlers {
		if err := consumer.Subscribe(eventType, handler); err != nil {
			return fmt.Errorf("failed to register handler for %s: %w", eventType, err)
		}
	}

	return nil
}

// ProcessingEventEmitter provides methods to emit processing events
type ProcessingEventEmitter struct {
	producer *events.Producer
}

// NewProcessingEventEmitter creates a new processing event emitter
func NewProcessingEventEmitter(producer *events.Producer) *ProcessingEventEmitter {
	return &ProcessingEventEmitter{
		producer: producer,
	}
}

// EmitFileProcessingStarted emits an event when file processing starts
func (pee *ProcessingEventEmitter) EmitFileProcessingStarted(ctx context.Context, tenantID, fileID uuid.UUID, metadata map[string]any) error {
	return pee.emitEvent(ctx, "file.processing.started", tenantID, fileID, uuid.Nil, "started", 0, "", metadata)
}

// EmitFileProcessingCompleted emits an event when file processing completes
func (pee *ProcessingEventEmitter) EmitFileProcessingCompleted(ctx context.Context, tenantID, fileID uuid.UUID, result *ProcessingResult) error {
	metadata := map[string]any{
		"chunks_created":   result.ChunksCreated,
		"bytes_processed":  result.BytesProcessed,
		"processing_time":  result.ProcessingTime,
		"quality_score":    result.QualityScore,
		"reader_used":      result.ReaderUsed,
		"strategy_used":    result.StrategyUsed,
	}
	return pee.emitEvent(ctx, "file.processing.completed", tenantID, fileID, uuid.Nil, "completed", 1.0, "", metadata)
}

// EmitFileProcessingFailed emits an event when file processing fails
func (pee *ProcessingEventEmitter) EmitFileProcessingFailed(ctx context.Context, tenantID, fileID uuid.UUID, errorMsg string) error {
	return pee.emitEvent(ctx, "file.processing.failed", tenantID, fileID, uuid.Nil, "failed", 0, errorMsg, nil)
}

// EmitSessionCreated emits an event when a processing session is created
func (pee *ProcessingEventEmitter) EmitSessionCreated(ctx context.Context, tenantID, sessionID uuid.UUID, fileCount int) error {
	metadata := map[string]any{
		"file_count": fileCount,
	}
	return pee.emitEvent(ctx, "session.created", tenantID, uuid.Nil, sessionID, "created", 0, "", metadata)
}

// EmitSessionStarted emits an event when a processing session starts
func (pee *ProcessingEventEmitter) EmitSessionStarted(ctx context.Context, tenantID, sessionID uuid.UUID) error {
	return pee.emitEvent(ctx, "session.started", tenantID, uuid.Nil, sessionID, "started", 0, "", nil)
}

// EmitSessionProgress emits an event with session progress updates
func (pee *ProcessingEventEmitter) EmitSessionProgress(ctx context.Context, tenantID, sessionID uuid.UUID, progress *SessionProgress) error {
	metadata := map[string]any{
		"files_total":      progress.FilesTotal,
		"files_processed":  progress.FilesProcessed,
		"files_failed":     progress.FilesFailed,
		"chunks_created":   progress.ChunksCreated,
		"bytes_processed":  progress.BytesProcessed,
	}
	return pee.emitEvent(ctx, "session.progress", tenantID, uuid.Nil, sessionID, progress.Status, progress.Progress, "", metadata)
}

// EmitSessionCompleted emits an event when a processing session completes
func (pee *ProcessingEventEmitter) EmitSessionCompleted(ctx context.Context, tenantID, sessionID uuid.UUID, progress *SessionProgress) error {
	metadata := map[string]any{
		"files_total":      progress.FilesTotal,
		"files_processed":  progress.FilesProcessed,
		"files_failed":     progress.FilesFailed,
		"chunks_created":   progress.ChunksCreated,
		"bytes_processed":  progress.BytesProcessed,
		"total_time":       progress.ElapsedTime,
	}
	return pee.emitEvent(ctx, "session.completed", tenantID, uuid.Nil, sessionID, "completed", 1.0, "", metadata)
}

// emitEvent emits a processing event
func (pee *ProcessingEventEmitter) emitEvent(ctx context.Context, eventType string, tenantID, fileID, sessionID uuid.UUID, status string, progress float64, errorMsg string, metadata map[string]any) error {
	if pee.producer == nil {
		return nil // No producer configured
	}

	eventData := ProcessingEventData{
		Status:    status,
		Progress:  progress,
		Timestamp: time.Now(),
	}

	if tenantID != uuid.Nil {
		eventData.TenantID = tenantID
	}
	if fileID != uuid.Nil {
		eventData.FileID = fileID
	}
	if sessionID != uuid.Nil {
		eventData.SessionID = sessionID
	}
	if errorMsg != "" {
		eventData.Error = errorMsg
	}
	if metadata != nil {
		eventData.Metadata = metadata
	}

	data, err := json.Marshal(eventData)
	if err != nil {
		return fmt.Errorf("failed to marshal event data: %w", err)
	}

	event := &events.Event{
		ID:        uuid.New().String(),
		Type:      eventType,
		Source:    "processing.pipeline",
		Data:      data,
		Timestamp: time.Now(),
	}

	if tenantID != uuid.Nil {
		event.TenantID = tenantID.String()
	}

	return pee.producer.Publish(ctx, event)
}