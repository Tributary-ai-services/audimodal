package onedrive

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/jscharber/eAIIngest/pkg/storage"
	"github.com/jscharber/eAIIngest/pkg/sync"
)

// OneDriveSyncManager handles OneDrive synchronization operations
type OneDriveSyncManager struct {
	connector *OneDriveConnector
	config    *OneDriveSyncConfig
	tracer    trace.Tracer
	
	// Sync state
	syncJobs     map[uuid.UUID]*sync.SyncJob
	webhookUrls  map[string]string // File ID -> Webhook URL for change notifications
	
	// Delta tracking
	deltaTokens  map[string]string // Path -> Delta token for incremental sync
	lastSyncTime time.Time
}

// OneDriveSyncConfig contains sync-specific configuration
type OneDriveSyncConfig struct {
	// Sync intervals
	IncrementalSyncInterval time.Duration `yaml:"incremental_sync_interval"`
	FullSyncInterval        time.Duration `yaml:"full_sync_interval"`
	
	// Batch processing
	BatchSize               int           `yaml:"batch_size"`
	MaxConcurrentSyncs      int           `yaml:"max_concurrent_syncs"`
	
	// Change detection
	UseWebhooks             bool          `yaml:"use_webhooks"`
	WebhookTimeout          time.Duration `yaml:"webhook_timeout"`
	UseDeltaAPI             bool          `yaml:"use_delta_api"`
	
	// Conflict resolution
	ConflictResolution      string        `yaml:"conflict_resolution"` // "latest", "manual", "preserve_both"
	PreserveBothSuffix      string        `yaml:"preserve_both_suffix"`
	
	// Performance
	EnableParallelSync      bool          `yaml:"enable_parallel_sync"`
	SyncWorkerCount         int           `yaml:"sync_worker_count"`
	
	// Filtering
	SyncSharedFiles         bool          `yaml:"sync_shared_files"`
	SyncVersionHistory      bool          `yaml:"sync_version_history"`
	IncludeHiddenFiles      bool          `yaml:"include_hidden_files"`
}

// DefaultOneDriveSyncConfig returns default sync configuration
func DefaultOneDriveSyncConfig() *OneDriveSyncConfig {
	return &OneDriveSyncConfig{
		IncrementalSyncInterval: 5 * time.Minute,
		FullSyncInterval:        24 * time.Hour,
		BatchSize:              200,
		MaxConcurrentSyncs:     5,
		UseWebhooks:            true,
		WebhookTimeout:         30 * time.Second,
		UseDeltaAPI:            true,
		ConflictResolution:     "latest",
		PreserveBothSuffix:     "_conflict",
		EnableParallelSync:     true,
		SyncWorkerCount:        4,
		SyncSharedFiles:        true,
		SyncVersionHistory:     false,
		IncludeHiddenFiles:     false,
	}
}

// NewOneDriveSyncManager creates a new OneDrive sync manager
func NewOneDriveSyncManager(connector *OneDriveConnector, config *OneDriveSyncConfig) *OneDriveSyncManager {
	if config == nil {
		config = DefaultOneDriveSyncConfig()
	}

	return &OneDriveSyncManager{
		connector:   connector,
		config:      config,
		tracer:      otel.Tracer("onedrive-sync-manager"),
		syncJobs:    make(map[uuid.UUID]*sync.SyncJob),
		webhookUrls: make(map[string]string),
		deltaTokens: make(map[string]string),
	}
}

// StartSync initiates a sync operation
func (sm *OneDriveSyncManager) StartSync(ctx context.Context, request *sync.StartSyncRequest) (*sync.SyncJob, error) {
	ctx, span := sm.tracer.Start(ctx, "onedrive_sync.start_sync")
	defer span.End()

	span.SetAttributes(
		attribute.String("data_source_id", request.DataSourceID.String()),
		attribute.String("sync_type", string(request.Options.SyncType)),
		attribute.String("direction", string(request.Options.Direction)),
	)

	// Create sync job
	job := &sync.SyncJob{
		ID:            uuid.New(),
		DataSourceID:  request.DataSourceID,
		ConnectorType: "onedrive",
		Options:       request.Options,
		Status: &sync.SyncJobStatus{
			State:        sync.SyncStateInitializing,
			StartTime:    time.Now(),
			LastActivity: time.Now(),
			Progress: &sync.SyncProgress{
				Phase:           "initializing",
				TotalFiles:      0,
				ProcessedFiles:  0,
				PercentComplete: 0,
			},
			Metrics: &sync.SyncMetrics{
				FilesScanned:      0,
				FilesProcessed:    0,
				BytesTransferred:  0,
				AverageTransferRate: 0,
				Conflicts:         0,
				Errors:           0,
			},
		},
	}

	// Store sync job
	sm.syncJobs[job.ID] = job

	// Start sync operation in background
	go sm.executeSyncJob(context.Background(), job)

	return job, nil
}

// GetSyncStatus returns the status of a sync job
func (sm *OneDriveSyncManager) GetSyncStatus(ctx context.Context, jobID uuid.UUID) (*sync.SyncJobStatus, error) {
	ctx, span := sm.tracer.Start(ctx, "onedrive_sync.get_sync_status")
	defer span.End()

	span.SetAttributes(attribute.String("job_id", jobID.String()))

	job, exists := sm.syncJobs[jobID]
	if !exists {
		return nil, sync.ErrSyncJobNotFound
	}

	return job.Status, nil
}

// CancelSync cancels a running sync job
func (sm *OneDriveSyncManager) CancelSync(ctx context.Context, jobID uuid.UUID) error {
	ctx, span := sm.tracer.Start(ctx, "onedrive_sync.cancel_sync")
	defer span.End()

	span.SetAttributes(attribute.String("job_id", jobID.String()))

	job, exists := sm.syncJobs[jobID]
	if !exists {
		return sync.ErrSyncJobNotFound
	}

	job.Cancel()
	return nil
}

// ListActiveSyncs returns all active sync jobs
func (sm *OneDriveSyncManager) ListActiveSyncs(ctx context.Context, filters *sync.SyncListFilters) ([]*sync.SyncJob, error) {
	ctx, span := sm.tracer.Start(ctx, "onedrive_sync.list_active_syncs")
	defer span.End()

	var jobs []*sync.SyncJob
	for _, job := range sm.syncJobs {
		// Apply filters
		if filters != nil {
			if filters.DataSourceID != uuid.Nil && job.DataSourceID != filters.DataSourceID {
				continue
			}
			if len(filters.States) > 0 {
				stateMatch := false
				for _, state := range filters.States {
					if job.Status.State == state {
						stateMatch = true
						break
					}
				}
				if !stateMatch {
					continue
				}
			}
		}

		jobs = append(jobs, job)
	}

	span.SetAttributes(attribute.Int("jobs_count", len(jobs)))
	return jobs, nil
}

// RegisterWebhook registers a webhook for change notifications
func (sm *OneDriveSyncManager) RegisterWebhook(ctx context.Context, config *sync.WebhookConfig) error {
	ctx, span := sm.tracer.Start(ctx, "onedrive_sync.register_webhook")
	defer span.End()

	if !sm.config.UseWebhooks {
		return fmt.Errorf("webhooks are disabled")
	}

	span.SetAttributes(
		attribute.String("webhook_url", config.URL),
		attribute.String("resource", config.Resource),
	)

	// Create Microsoft Graph subscription
	subscription := &GraphSubscription{
		ChangeType:         "updated,deleted,created",
		NotificationURL:    config.URL,
		Resource:          "/me/drive/root",
		ExpirationDateTime: time.Now().Add(4230 * time.Minute), // Max 3 days for OneDrive
		ClientState:       config.Secret,
	}

	subscriptionJSON, err := json.Marshal(subscription)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to marshal subscription: %w", err)
	}

	// Register subscription with Microsoft Graph
	response, err := sm.connector.executeGraphAPICall(ctx, "POST", 
		"https://graph.microsoft.com/v1.0/subscriptions", subscriptionJSON)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to register webhook: %w", err)
	}

	var subscriptionResponse GraphSubscription
	if err := json.Unmarshal(response, &subscriptionResponse); err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to parse subscription response: %w", err)
	}

	// Store webhook registration
	sm.webhookUrls[subscriptionResponse.ID] = config.URL

	return nil
}

// ProcessWebhookEvent processes incoming webhook events
func (sm *OneDriveSyncManager) ProcessWebhookEvent(ctx context.Context, event *sync.WebhookEvent) error {
	ctx, span := sm.tracer.Start(ctx, "onedrive_sync.process_webhook_event")
	defer span.End()

	span.SetAttributes(
		attribute.String("event.type", event.EventType),
		attribute.String("event.resource", event.Resource),
	)

	// Parse OneDrive webhook notification
	var notification GraphNotification
	if err := json.Unmarshal(event.Data, &notification); err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to parse webhook notification: %w", err)
	}

	// Process each change in the notification
	for _, change := range notification.Value {
		if err := sm.processFileChange(ctx, &change); err != nil {
			span.RecordError(err)
			// Log error but continue processing other changes
			continue
		}
	}

	return nil
}

// Internal methods

func (sm *OneDriveSyncManager) executeSyncJob(ctx context.Context, job *sync.SyncJob) {
	defer func() {
		if job.Status.State == sync.SyncStateRunning {
			job.Status.State = sync.SyncStateCompleted
			job.Status.EndTime = &[]time.Time{time.Now()}[0]
			duration := job.Status.EndTime.Sub(job.Status.StartTime)
			job.Status.Duration = duration
		}
	}()

	// Update job state
	job.Status.State = sync.SyncStateRunning
	job.Status.Progress.Phase = "scanning"

	// Execute sync based on type
	switch job.Options.SyncType {
	case sync.SyncTypeFull:
		sm.executeFullSync(ctx, job)
	case sync.SyncTypeIncremental:
		sm.executeIncrementalSync(ctx, job)
	case sync.SyncTypeRealtime:
		sm.executeRealtimeSync(ctx, job)
	default:
		job.Status.State = sync.SyncStateFailed
		job.Status.Error = "unsupported sync type"
		return
	}
}

func (sm *OneDriveSyncManager) executeFullSync(ctx context.Context, job *sync.SyncJob) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent("Starting full sync")

	// Get all files from OneDrive
	files, err := sm.connector.ListFiles(ctx, "", &storage.ConnectorListOptions{
		Recursive: true,
	})
	if err != nil {
		job.Status.State = sync.SyncStateFailed
		job.Status.Error = err.Error()
		return
	}

	job.Status.Progress.TotalFiles = len(files)
	job.Status.Progress.Phase = "processing"

	// Process files in batches
	batchSize := sm.config.BatchSize
	for i := 0; i < len(files); i += batchSize {
		end := i + batchSize
		if end > len(files) {
			end = len(files)
		}

		batch := files[i:end]
		if err := sm.processBatch(ctx, job, batch); err != nil {
			job.Status.State = sync.SyncStateFailed
			job.Status.Error = err.Error()
			return
		}

		// Update progress
		job.Status.Progress.ProcessedFiles = i + len(batch)
		job.Status.Progress.PercentComplete = float64(job.Status.Progress.ProcessedFiles) / float64(job.Status.Progress.TotalFiles) * 100
		job.Status.LastActivity = time.Now()

		// Check for cancellation
		if job.IsCanceled() {
			job.Status.State = sync.SyncStateCancelled
			return
		}
	}

	span.AddEvent("Full sync completed")
}

func (sm *OneDriveSyncManager) executeIncrementalSync(ctx context.Context, job *sync.SyncJob) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent("Starting incremental sync")

	if !sm.config.UseDeltaAPI {
		// Fall back to comparing modification times
		sm.executeTimestampBasedSync(ctx, job)
		return
	}

	// Use Microsoft Graph delta API
	deltaURL := "https://graph.microsoft.com/v1.0/me/drive/root/delta"
	
	// Use stored delta token if available
	if token, exists := sm.deltaTokens["root"]; exists {
		deltaURL = token
	}

	var allChanges []*DriveItem
	for {
		// Wait for rate limit
		if err := sm.connector.rateLimiter.Wait(ctx); err != nil {
			job.Status.State = sync.SyncStateFailed
			job.Status.Error = err.Error()
			return
		}

		// Get delta changes
		response, err := sm.connector.executeGraphAPICall(ctx, "GET", deltaURL, nil)
		if err != nil {
			job.Status.State = sync.SyncStateFailed
			job.Status.Error = err.Error()
			return
		}

		var deltaResponse DeltaLink
		if err := json.Unmarshal(response, &deltaResponse); err != nil {
			job.Status.State = sync.SyncStateFailed
			job.Status.Error = err.Error()
			return
		}

		allChanges = append(allChanges, deltaResponse.Value...)

		// Check for next page or delta link
		if deltaResponse.ODataNextLink != "" {
			deltaURL = deltaResponse.ODataNextLink
		} else if deltaResponse.ODataDeltaLink != "" {
			// Save delta link for next sync
			sm.deltaTokens["root"] = deltaResponse.ODataDeltaLink
			break
		} else {
			break
		}
	}

	job.Status.Progress.TotalFiles = len(allChanges)
	job.Status.Progress.Phase = "processing"

	// Process changes
	for i, change := range allChanges {
		if err := sm.processFileChange(ctx, change); err != nil {
			job.Status.Metrics.Errors++
			// Continue processing other files
		} else {
			job.Status.Metrics.FilesProcessed++
		}

		// Update progress
		job.Status.Progress.ProcessedFiles = i + 1
		job.Status.Progress.PercentComplete = float64(job.Status.Progress.ProcessedFiles) / float64(job.Status.Progress.TotalFiles) * 100
		job.Status.LastActivity = time.Now()

		// Check for cancellation
		if job.IsCanceled() {
			job.Status.State = sync.SyncStateCancelled
			return
		}
	}

	span.AddEvent("Incremental sync completed")
}

func (sm *OneDriveSyncManager) executeRealtimeSync(ctx context.Context, job *sync.SyncJob) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent("Starting realtime sync")

	if !sm.config.UseWebhooks {
		job.Status.State = sync.SyncStateFailed
		job.Status.Error = "realtime sync requires webhooks to be enabled"
		return
	}

	// For realtime sync, we primarily rely on webhook notifications
	// This method would typically set up the necessary webhook subscriptions
	// and maintain a persistent connection for receiving notifications

	job.Status.Progress.Phase = "monitoring"
	job.Status.Progress.PercentComplete = 100 // Always "complete" for realtime monitoring

	// Keep the sync job alive for realtime monitoring
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			job.Status.State = sync.SyncStateCancelled
			return
		case <-ticker.C:
			job.Status.LastActivity = time.Now()
			// Webhook events are processed separately via ProcessWebhookEvent
		}

		if job.IsCanceled() {
			job.Status.State = sync.SyncStateCancelled
			return
		}
	}
}

func (sm *OneDriveSyncManager) executeTimestampBasedSync(ctx context.Context, job *sync.SyncJob) {
	// Get files modified since last sync
	cutoffTime := sm.lastSyncTime
	if job.Options.Since != nil {
		cutoffTime = *job.Options.Since
	}

	files, err := sm.connector.ListFiles(ctx, "", &storage.ConnectorListOptions{
		Recursive: true,
		Since:     &cutoffTime,
	})
	if err != nil {
		job.Status.State = sync.SyncStateFailed
		job.Status.Error = err.Error()
		return
	}

	job.Status.Progress.TotalFiles = len(files)
	job.Status.Progress.Phase = "processing"

	// Process modified files
	for i, file := range files {
		if file.ModifiedAt.After(cutoffTime) {
			if err := sm.processFileUpdate(ctx, file); err != nil {
				job.Status.Metrics.Errors++
			} else {
				job.Status.Metrics.FilesProcessed++
			}
		}

		// Update progress
		job.Status.Progress.ProcessedFiles = i + 1
		job.Status.Progress.PercentComplete = float64(job.Status.Progress.ProcessedFiles) / float64(job.Status.Progress.TotalFiles) * 100
		job.Status.LastActivity = time.Now()

		// Check for cancellation
		if job.IsCanceled() {
			job.Status.State = sync.SyncStateCancelled
			return
		}
	}

	sm.lastSyncTime = time.Now()
}

func (sm *OneDriveSyncManager) processBatch(ctx context.Context, job *sync.SyncJob, files []*storage.ConnectorFileInfo) error {
	// Process files concurrently if enabled
	if sm.config.EnableParallelSync {
		return sm.processBatchParallel(ctx, job, files)
	}

	// Sequential processing
	for _, file := range files {
		if err := sm.processFileUpdate(ctx, file); err != nil {
			job.Status.Metrics.Errors++
			// Continue processing other files
		} else {
			job.Status.Metrics.FilesProcessed++
		}

		job.Status.Metrics.BytesTransferred += file.Size
	}

	return nil
}

func (sm *OneDriveSyncManager) processBatchParallel(ctx context.Context, job *sync.SyncJob, files []*storage.ConnectorFileInfo) error {
	// Create worker pool
	workers := sm.config.SyncWorkerCount
	if workers <= 0 {
		workers = 4
	}

	fileChan := make(chan *storage.ConnectorFileInfo, len(files))
	errorChan := make(chan error, len(files))

	// Start workers
	for i := 0; i < workers; i++ {
		go sm.syncWorker(ctx, job, fileChan, errorChan)
	}

	// Send files to workers
	for _, file := range files {
		fileChan <- file
	}
	close(fileChan)

	// Collect results
	var errors []error
	for i := 0; i < len(files); i++ {
		if err := <-errorChan; err != nil {
			errors = append(errors, err)
			job.Status.Metrics.Errors++
		} else {
			job.Status.Metrics.FilesProcessed++
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("batch processing completed with %d errors", len(errors))
	}

	return nil
}

func (sm *OneDriveSyncManager) syncWorker(ctx context.Context, job *sync.SyncJob, fileChan <-chan *storage.ConnectorFileInfo, errorChan chan<- error) {
	for file := range fileChan {
		if job.IsCanceled() {
			errorChan <- fmt.Errorf("sync cancelled")
			return
		}

		err := sm.processFileUpdate(ctx, file)
		errorChan <- err

		if err == nil {
			job.Status.Metrics.BytesTransferred += file.Size
		}
	}
}

func (sm *OneDriveSyncManager) processFileUpdate(ctx context.Context, file *storage.ConnectorFileInfo) error {
	// This would typically involve:
	// 1. Downloading the file if needed
	// 2. Updating local storage/database
	// 3. Handling conflicts if they exist
	// 4. Updating sync metadata

	span := trace.SpanFromContext(ctx)
	span.AddEvent("Processing file update", trace.WithAttributes(
		attribute.String("file.id", file.ID),
		attribute.String("file.name", file.Name),
		attribute.Int64("file.size", file.Size),
	))

	// For now, this is a placeholder that would integrate with the storage system
	return nil
}

func (sm *OneDriveSyncManager) processFileChange(ctx context.Context, change *DriveItem) error {
	// Process individual file change from delta API or webhook
	span := trace.SpanFromContext(ctx)
	span.AddEvent("Processing file change", trace.WithAttributes(
		attribute.String("file.id", change.ID),
		attribute.String("file.name", change.Name),
	))

	// Convert to FileInfo and process
	fileInfo := sm.connector.convertToFileInfo(change)
	return sm.processFileUpdate(ctx, fileInfo)
}

// Cleanup methods

func (sm *OneDriveSyncManager) cleanupCompletedJobs() {
	cutoff := time.Now().Add(-24 * time.Hour) // Keep jobs for 24 hours after completion

	for jobID, job := range sm.syncJobs {
		if job.Status.State == sync.SyncStateCompleted ||
		   job.Status.State == sync.SyncStateFailed ||
		   job.Status.State == sync.SyncStateCancelled {
			
			if job.Status.EndTime != nil && job.Status.EndTime.Before(cutoff) {
				delete(sm.syncJobs, jobID)
			}
		}
	}
}

// Shutdown gracefully shuts down the sync manager
func (sm *OneDriveSyncManager) Shutdown(ctx context.Context) error {
	// Cancel all running sync jobs
	for _, job := range sm.syncJobs {
		if job.Status.State == sync.SyncStateRunning {
			job.Cancel()
		}
	}

	// Clean up webhook subscriptions
	for subscriptionID := range sm.webhookUrls {
		if err := sm.unregisterWebhook(ctx, subscriptionID); err != nil {
			// Log error but continue cleanup
		}
	}

	return nil
}

func (sm *OneDriveSyncManager) unregisterWebhook(ctx context.Context, subscriptionID string) error {
	apiURL := fmt.Sprintf("https://graph.microsoft.com/v1.0/subscriptions/%s", subscriptionID)
	_, err := sm.connector.executeGraphAPICall(ctx, "DELETE", apiURL, nil)
	return err
}

// GraphSubscription represents a Microsoft Graph subscription for webhooks
type GraphSubscription struct {
	ID                 string    `json:"id,omitempty"`
	Resource           string    `json:"resource"`
	ChangeType         string    `json:"changeType"`
	ClientState        string    `json:"clientState,omitempty"`
	NotificationURL    string    `json:"notificationUrl"`
	ExpirationDateTime time.Time `json:"expirationDateTime"`
	ApplicationID      string    `json:"applicationId,omitempty"`
	CreatorID          string    `json:"creatorId,omitempty"`
}

// GraphNotification represents a webhook notification from Microsoft Graph
type GraphNotification struct {
	Value []*DriveItem `json:"value"`
}