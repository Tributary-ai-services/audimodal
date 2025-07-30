package lifecycle

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/jscharber/eAIIngest/pkg/storage"
)

// JobExecutor executes lifecycle jobs
type JobExecutor struct {
	config         *LifecycleConfig
	storageManager storage.StorageManager
	lifecycleManager *LifecycleManager
	tracer         trace.Tracer
	
	// Worker pool
	workers        []Worker
	workerPool     chan Worker
	
	// Metrics
	metrics        *ExecutorMetrics
}

// Worker represents a worker that can execute lifecycle actions
type Worker interface {
	Execute(ctx context.Context, job *LifecycleJob, action LifecycleActionSpec, files []storage.FileInfo) error
	GetID() string
	IsAvailable() bool
}

// ExecutorMetrics tracks executor performance
type ExecutorMetrics struct {
	ActiveWorkers     int           `json:"active_workers"`
	QueuedJobs        int           `json:"queued_jobs"`
	ProcessingJobs    int           `json:"processing_jobs"`
	AverageJobTime    time.Duration `json:"average_job_time"`
	ThroughputPerSec  float64       `json:"throughput_per_sec"`
	ErrorRate         float64       `json:"error_rate"`
}

// NewJobExecutor creates a new job executor
func NewJobExecutor(config *LifecycleConfig, storageManager storage.StorageManager, lifecycleManager *LifecycleManager) *JobExecutor {
	executor := &JobExecutor{
		config:           config,
		storageManager:   storageManager,
		lifecycleManager: lifecycleManager,
		tracer:          otel.Tracer("lifecycle-job-executor"),
		workerPool:      make(chan Worker, config.MaxConcurrentJobs),
		metrics:         &ExecutorMetrics{},
	}

	// Initialize workers
	for i := 0; i < config.MaxConcurrentJobs; i++ {
		worker := NewDefaultWorker(fmt.Sprintf("worker-%d", i), storageManager, config)
		executor.workers = append(executor.workers, worker)
		executor.workerPool <- worker
	}

	return executor
}

// Start starts the job executor
func (je *JobExecutor) Start(ctx context.Context, jobQueue <-chan *LifecycleJob, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case job := <-jobQueue:
			if job == nil {
				return // Channel closed
			}
			
			// Execute job in goroutine
			go je.executeJob(ctx, job)
		}
	}
}

// executeJob executes a single lifecycle job
func (je *JobExecutor) executeJob(ctx context.Context, job *LifecycleJob) {
	ctx, span := je.tracer.Start(ctx, "execute_lifecycle_job")
	defer span.End()

	span.SetAttributes(
		attribute.String("job.id", job.ID.String()),
		attribute.String("job.type", job.Type),
		attribute.String("tenant.id", job.TenantID.String()),
	)

	// Update job status
	job.Status = StatusInProgress
	now := time.Now()
	job.StartedAt = &now
	job.Progress.StartTime = now

	if err := je.lifecycleManager.jobStore.UpdateJob(ctx, job); err != nil {
		span.RecordError(err)
		return
	}

	// Get a worker
	worker := <-je.workerPool
	defer func() {
		je.workerPool <- worker
	}()

	// Execute job actions
	err := je.executeJobActions(ctx, job, worker)
	
	// Update job status
	completedAt := time.Now()
	job.CompletedAt = &completedAt
	job.Progress.LastUpdate = completedAt
	job.Results.Duration = completedAt.Sub(job.Progress.StartTime)

	if err != nil {
		job.Status = StatusFailed
		job.ErrorMessage = err.Error()
		span.RecordError(err)
	} else {
		job.Status = StatusCompleted
	}

	// Calculate performance metrics
	if job.Results.Duration > 0 && job.Results.TotalBytesProcessed > 0 {
		mbps := float64(job.Results.TotalBytesProcessed) / (1024 * 1024) / job.Results.Duration.Seconds()
		job.Results.ThroughputMBps = mbps
	}

	if job.Results.Duration > 0 && job.Results.TotalFilesProcessed > 0 {
		opsPerSec := float64(job.Results.TotalFilesProcessed) / job.Results.Duration.Seconds()
		job.Results.OperationsPerSecond = opsPerSec
	}

	// Save final job state
	if err := je.lifecycleManager.jobStore.UpdateJob(ctx, job); err != nil {
		span.RecordError(err)
	}

	// Record completion event
	event := &LifecycleEvent{
		ID:        uuid.New(),
		Type:      "job_completed",
		Timestamp: time.Now(),
		TenantID:  job.TenantID,
		JobID:     &job.ID,
		Status:    job.Status,
		Message:   fmt.Sprintf("Job %s completed with status %s", job.ID, job.Status),
		Source:    "job_executor",
	}
	je.lifecycleManager.eventStore.StoreEvent(ctx, event)

	span.SetAttributes(
		attribute.String("job.status", string(job.Status)),
		attribute.Int64("files.processed", job.Results.TotalFilesProcessed),
		attribute.Int64("bytes.processed", job.Results.TotalBytesProcessed),
		attribute.Float64("duration.seconds", job.Results.Duration.Seconds()),
	)
}

// executeJobActions executes all actions for a job
func (je *JobExecutor) executeJobActions(ctx context.Context, job *LifecycleJob, worker Worker) error {
	// Discover files that match the job criteria
	files, err := je.discoverFiles(ctx, job)
	if err != nil {
		return fmt.Errorf("failed to discover files: %w", err)
	}

	job.Progress.TotalFiles = int64(len(files))
	job.Progress.TotalBytes = je.calculateTotalBytes(files)

	// Sort actions by priority
	actions := make([]LifecycleActionSpec, len(job.Config.Actions))
	copy(actions, job.Config.Actions)
	je.sortActionsByPriority(actions)

	// Execute each action
	for _, action := range actions {
		if err := je.executeAction(ctx, job, worker, action, files); err != nil {
			return fmt.Errorf("failed to execute action %s: %w", action.Action, err)
		}
	}

	return nil
}

// discoverFiles discovers files that should be processed by the job
func (je *JobExecutor) discoverFiles(ctx context.Context, job *LifecycleJob) ([]storage.FileInfo, error) {
	ctx, span := je.tracer.Start(ctx, "discover_files")
	defer span.End()

	var allFiles []storage.FileInfo

	// This would need to iterate through storage locations
	// For now, this is a placeholder implementation
	// TODO: Implement actual file discovery based on job criteria

	span.SetAttributes(
		attribute.Int("files.discovered", len(allFiles)),
	)

	return allFiles, nil
}

// executeAction executes a specific action on the files
func (je *JobExecutor) executeAction(ctx context.Context, job *LifecycleJob, worker Worker, action LifecycleActionSpec, files []storage.FileInfo) error {
	ctx, span := je.tracer.Start(ctx, "execute_action")
	defer span.End()

	span.SetAttributes(
		attribute.String("action.type", string(action.Action)),
		attribute.Int("files.count", len(files)),
	)

	// Filter files based on action conditions
	eligibleFiles := je.filterFilesForAction(ctx, action, files)

	if len(eligibleFiles) == 0 {
		span.SetAttributes(attribute.Int("files.eligible", 0))
		return nil
	}

	// Apply delay if configured
	if action.DelayBefore > 0 {
		time.Sleep(action.DelayBefore)
	}

	// Execute the action
	startTime := time.Now()
	err := worker.Execute(ctx, job, action, eligibleFiles)
	duration := time.Since(startTime)

	// Update job progress and results
	je.updateJobProgress(job, action, eligibleFiles, err == nil, duration)

	// Record action event
	event := &LifecycleEvent{
		ID:        uuid.New(),
		Type:      "action_executed",
		Timestamp: time.Now(),
		TenantID:  job.TenantID,
		JobID:     &job.ID,
		Action:    action.Action,
		Status:    StatusCompleted,
		Message:   fmt.Sprintf("Action %s executed on %d files", action.Action, len(eligibleFiles)),
		Source:    "job_executor",
		Metadata: map[string]interface{}{
			"files_processed": len(eligibleFiles),
			"duration_ms":     duration.Milliseconds(),
			"success":         err == nil,
		},
	}

	if err != nil {
		event.Status = StatusFailed
		event.Message = fmt.Sprintf("Action %s failed: %s", action.Action, err.Error())
	}

	je.lifecycleManager.eventStore.StoreEvent(ctx, event)

	span.SetAttributes(
		attribute.Int("files.eligible", len(eligibleFiles)),
		attribute.Bool("action.success", err == nil),
		attribute.Float64("action.duration_ms", float64(duration.Milliseconds())),
	)

	if err != nil && !action.ContinueOnError {
		span.RecordError(err)
		return err
	}

	return nil
}

// filterFilesForAction filters files that are eligible for a specific action
func (je *JobExecutor) filterFilesForAction(ctx context.Context, action LifecycleActionSpec, files []storage.FileInfo) []storage.FileInfo {
	var eligibleFiles []storage.FileInfo

	for _, file := range files {
		if je.isFileEligibleForAction(ctx, action, file) {
			eligibleFiles = append(eligibleFiles, file)
		}
	}

	return eligibleFiles
}

// isFileEligibleForAction checks if a file is eligible for a specific action
func (je *JobExecutor) isFileEligibleForAction(ctx context.Context, action LifecycleActionSpec, file storage.FileInfo) bool {
	// Check prerequisites
	for _, prereq := range action.Prerequisites {
		// Check if prerequisite action has been completed for this file
		// This would need to be tracked in the job progress
		_ = prereq // TODO: Implement prerequisite checking
	}

	// Check conditional criteria
	for _, condition := range action.ConditionalOn {
		if !je.evaluateCondition(condition, file) {
			return false
		}
	}

	// Action-specific eligibility checks
	switch action.Action {
	case ActionArchive:
		return je.isEligibleForArchive(file)
	case ActionDelete:
		return je.isEligibleForDeletion(file)
	case ActionCompress:
		return je.isEligibleForCompression(file)
	default:
		return true
	}
}

// evaluateCondition evaluates a lifecycle condition against a file
func (je *JobExecutor) evaluateCondition(condition LifecycleCondition, file storage.FileInfo) bool {
	switch condition.Type {
	case ConditionAge:
		if duration, ok := condition.Value.(time.Duration); ok {
			age := time.Since(file.LastModified)
			switch condition.Operator {
			case "gt", "greater_than":
				return age > duration
			case "lt", "less_than":
				return age < duration
			case "ge", "greater_equal":
				return age >= duration
			case "le", "less_equal":
				return age <= duration
			}
		}
		
	case ConditionFileSize:
		if size, ok := condition.Value.(int64); ok {
			switch condition.Operator {
			case "gt", "greater_than":
				return file.Size > size
			case "lt", "less_than":
				return file.Size < size
			case "ge", "greater_equal":
				return file.Size >= size
			case "le", "less_equal":
				return file.Size <= size
			case "eq", "equals":
				return file.Size == size
			}
		}
		
	case ConditionFileType:
		if fileType, ok := condition.Value.(string); ok {
			switch condition.Operator {
			case "eq", "equals":
				return file.ContentType == fileType
			case "contains":
				return contains(file.ContentType, fileType)
			case "starts_with":
				return startsWith(file.ContentType, fileType)
			case "ends_with":
				return endsWith(file.ContentType, fileType)
			}
		}
		
	case ConditionPath:
		if path, ok := condition.Value.(string); ok {
			switch condition.Operator {
			case "eq", "equals":
				return file.URL == path
			case "contains":
				return contains(file.URL, path)
			case "starts_with":
				return startsWith(file.URL, path)
			case "ends_with":
				return endsWith(file.URL, path)
			}
		}
		
	case ConditionTags:
		if tag, ok := condition.Value.(string); ok {
			if tagValue, exists := file.Tags[tag]; exists {
				return tagValue != ""
			}
		}
		
	case ConditionMetadata:
		if condition.Field != "" {
			if value, exists := file.Metadata[condition.Field]; exists {
				if expectedValue, ok := condition.Value.(string); ok {
					switch condition.Operator {
					case "eq", "equals":
						return value == expectedValue
					case "contains":
						return contains(value, expectedValue)
					}
				}
			}
		}
	}

	return false
}

// File eligibility checks
func (je *JobExecutor) isEligibleForArchive(file storage.FileInfo) bool {
	// Files are eligible for archival if they're not already archived
	return file.StorageClass != "GLACIER" && file.StorageClass != "DEEP_ARCHIVE"
}

func (je *JobExecutor) isEligibleForDeletion(file storage.FileInfo) bool {
	// Only allow deletion after minimum retention period
	age := time.Since(file.LastModified)
	return age > je.config.MinRetentionPeriod
}

func (je *JobExecutor) isEligibleForCompression(file storage.FileInfo) bool {
	// Don't compress already compressed files
	return !file.Metadata["compressed"] == "true"
}

// Helper methods
func (je *JobExecutor) calculateTotalBytes(files []storage.FileInfo) int64 {
	var total int64
	for _, file := range files {
		total += file.Size
	}
	return total
}

func (je *JobExecutor) sortActionsByPriority(actions []LifecycleActionSpec) {
	// Sort by priority (higher priority first)
	for i := 0; i < len(actions)-1; i++ {
		for j := i + 1; j < len(actions); j++ {
			if actions[i].Priority < actions[j].Priority {
				actions[i], actions[j] = actions[j], actions[i]
			}
		}
	}
}

func (je *JobExecutor) updateJobProgress(job *LifecycleJob, action LifecycleActionSpec, files []storage.FileInfo, success bool, duration time.Duration) {
	job.Progress.ProcessedFiles += int64(len(files))
	job.Progress.LastUpdate = time.Now()
	job.Progress.CurrentOperation = string(action.Action)

	// Update results
	if success {
		job.Progress.SuccessfulFiles += int64(len(files))
		
		// Update action-specific counters
		if job.Results.ActionsPerformed == nil {
			job.Results.ActionsPerformed = make(map[string]int64)
		}
		job.Results.ActionsPerformed[string(action.Action)] += int64(len(files))
		
		// Update processed bytes
		for _, file := range files {
			job.Results.TotalBytesProcessed += file.Size
		}
	} else {
		job.Progress.FailedFiles += int64(len(files))
	}

	job.Results.TotalFilesProcessed = job.Progress.ProcessedFiles
}

// String helper functions
func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[len(s)-len(substr):] == substr
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func endsWith(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

// DefaultWorker implements the Worker interface
type DefaultWorker struct {
	id             string
	storageManager storage.StorageManager
	config         *LifecycleConfig
	available      bool
	tracer         trace.Tracer
}

// NewDefaultWorker creates a new default worker
func NewDefaultWorker(id string, storageManager storage.StorageManager, config *LifecycleConfig) *DefaultWorker {
	return &DefaultWorker{
		id:             id,
		storageManager: storageManager,
		config:         config,
		available:      true,
		tracer:        otel.Tracer("lifecycle-worker"),
	}
}

// Execute executes a lifecycle action on files
func (w *DefaultWorker) Execute(ctx context.Context, job *LifecycleJob, action LifecycleActionSpec, files []storage.FileInfo) error {
	ctx, span := w.tracer.Start(ctx, "worker_execute_action")
	defer span.End()

	w.available = false
	defer func() { w.available = true }()

	span.SetAttributes(
		attribute.String("worker.id", w.id),
		attribute.String("action.type", string(action.Action)),
		attribute.Int("files.count", len(files)),
	)

	switch action.Action {
	case ActionArchive:
		return w.executeArchive(ctx, job, files)
	case ActionDelete:
		return w.executeDelete(ctx, job, files)
	case ActionCompress:
		return w.executeCompress(ctx, job, files)
	case ActionMove:
		return w.executeMove(ctx, job, action, files)
	case ActionNotify:
		return w.executeNotify(ctx, job, action, files)
	case ActionTag:
		return w.executeTag(ctx, job, action, files)
	case ActionAudit:
		return w.executeAudit(ctx, job, files)
	default:
		return fmt.Errorf("unsupported action: %s", action.Action)
	}
}

// GetID returns the worker ID
func (w *DefaultWorker) GetID() string {
	return w.id
}

// IsAvailable returns whether the worker is available
func (w *DefaultWorker) IsAvailable() bool {
	return w.available
}

// Action implementations
func (w *DefaultWorker) executeArchive(ctx context.Context, job *LifecycleJob, files []storage.FileInfo) error {
	// Archive files to cold storage
	for _, file := range files {
		if w.config.DryRun || job.Config.DryRun {
			// Just log what would be done
			continue
		}
		
		// TODO: Implement actual archival
		// This would involve moving files to archive storage class
	}
	return nil
}

func (w *DefaultWorker) executeDelete(ctx context.Context, job *LifecycleJob, files []storage.FileInfo) error {
	// Delete files
	for _, file := range files {
		if w.config.DryRun || job.Config.DryRun {
			// Just log what would be done
			continue
		}
		
		// TODO: Implement actual deletion
		// This would involve calling storage manager to delete files
	}
	return nil
}

func (w *DefaultWorker) executeCompress(ctx context.Context, job *LifecycleJob, files []storage.FileInfo) error {
	// Compress files
	for _, file := range files {
		if w.config.DryRun || job.Config.DryRun {
			// Just log what would be done
			continue
		}
		
		// TODO: Implement actual compression
		// This would involve downloading, compressing, and re-uploading files
	}
	return nil
}

func (w *DefaultWorker) executeMove(ctx context.Context, job *LifecycleJob, action LifecycleActionSpec, files []storage.FileInfo) error {
	// Move files to different location/storage class
	destination, ok := action.Parameters["destination"].(string)
	if !ok {
		return fmt.Errorf("move action requires destination parameter")
	}
	
	for _, file := range files {
		if w.config.DryRun || job.Config.DryRun {
			// Just log what would be done
			continue
		}
		
		// TODO: Implement actual move
		_ = destination
	}
	return nil
}

func (w *DefaultWorker) executeNotify(ctx context.Context, job *LifecycleJob, action LifecycleActionSpec, files []storage.FileInfo) error {
	// Send notifications about the files
	// TODO: Implement notification logic
	return nil
}

func (w *DefaultWorker) executeTag(ctx context.Context, job *LifecycleJob, action LifecycleActionSpec, files []storage.FileInfo) error {
	// Add tags to files
	tags, ok := action.Parameters["tags"].(map[string]string)
	if !ok {
		return fmt.Errorf("tag action requires tags parameter")
	}
	
	for _, file := range files {
		if w.config.DryRun || job.Config.DryRun {
			// Just log what would be done
			continue
		}
		
		// TODO: Implement actual tagging
		_ = tags
	}
	return nil
}

func (w *DefaultWorker) executeAudit(ctx context.Context, job *LifecycleJob, files []storage.FileInfo) error {
	// Generate audit records for the files
	// TODO: Implement audit logging
	return nil
}