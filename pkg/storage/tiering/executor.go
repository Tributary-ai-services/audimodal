package tiering

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/jscharber/eAIIngest/pkg/storage"
)

// TieringJobExecutor executes tiering jobs with coordination and monitoring
type TieringJobExecutor struct {
	config         *TieringConfig
	storageManager storage.StorageManager
	tieringManager *TieringManager
	tracer         trace.Tracer

	// Execution components
	transitionEngine *TransitionEngine
	workers          []TieringWorker
	workerPool       chan TieringWorker

	// Metrics and monitoring
	metrics    *ExecutorMetrics
	activeJobs map[uuid.UUID]*TieringJobContext
	mu         sync.RWMutex
}

// TieringWorker interface for executing tiering operations
type TieringWorker interface {
	ExecuteJob(ctx context.Context, job *TieringJob) error
	GetID() string
	IsAvailable() bool
	GetMetrics() *WorkerMetrics
}

// TieringJobContext tracks the execution context of a tiering job
type TieringJobContext struct {
	Job       *TieringJob
	StartTime time.Time
	Worker    TieringWorker
	Progress  *TieringJobProgress
	Cancel    context.CancelFunc
}

// TieringJobProgress tracks detailed progress of a tiering job
type TieringJobProgress struct {
	Phase                string     `json:"phase"`
	FilesDiscovered      int64      `json:"files_discovered"`
	FilesAnalyzed        int64      `json:"files_analyzed"`
	TransitionsQueued    int64      `json:"transitions_queued"`
	TransitionsCompleted int64      `json:"transitions_completed"`
	TransitionsFailed    int64      `json:"transitions_failed"`
	CurrentBatch         int        `json:"current_batch"`
	TotalBatches         int        `json:"total_batches"`
	LastActivity         time.Time  `json:"last_activity"`
	EstimatedCompletion  *time.Time `json:"estimated_completion,omitempty"`
}

// ExecutorMetrics tracks executor performance
type ExecutorMetrics struct {
	ActiveWorkers         int           `json:"active_workers"`
	QueuedJobs            int           `json:"queued_jobs"`
	ProcessingJobs        int           `json:"processing_jobs"`
	CompletedJobs         int64         `json:"completed_jobs"`
	FailedJobs            int64         `json:"failed_jobs"`
	AverageJobDuration    time.Duration `json:"average_job_duration"`
	ThroughputJobsPerHour float64       `json:"throughput_jobs_per_hour"`
	ErrorRate             float64       `json:"error_rate"`
	LastReset             time.Time     `json:"last_reset"`
}

// WorkerMetrics tracks individual worker performance
type WorkerMetrics struct {
	WorkerID            string        `json:"worker_id"`
	JobsCompleted       int64         `json:"jobs_completed"`
	JobsFailed          int64         `json:"jobs_failed"`
	AverageJobTime      time.Duration `json:"average_job_time"`
	LastJobCompleted    *time.Time    `json:"last_job_completed,omitempty"`
	TotalProcessingTime time.Duration `json:"total_processing_time"`
}

// NewTieringJobExecutor creates a new tiering job executor
func NewTieringJobExecutor(config *TieringConfig, storageManager storage.StorageManager, tieringManager *TieringManager) *TieringJobExecutor {
	executor := &TieringJobExecutor{
		config:         config,
		storageManager: storageManager,
		tieringManager: tieringManager,
		tracer:         otel.Tracer("tiering-job-executor"),
		workerPool:     make(chan TieringWorker, config.MaxConcurrentJobs),
		metrics:        &ExecutorMetrics{LastReset: time.Now()},
		activeJobs:     make(map[uuid.UUID]*TieringJobContext),
	}

	// Initialize transition engine
	executor.transitionEngine = NewTransitionEngine(config, storageManager, tieringManager)

	// Initialize workers
	for i := 0; i < config.MaxConcurrentJobs; i++ {
		worker := NewDefaultTieringWorker(fmt.Sprintf("tiering-worker-%d", i), storageManager, executor.transitionEngine, config)
		executor.workers = append(executor.workers, worker)
		executor.workerPool <- worker
	}

	return executor
}

// Start starts the job executor
func (je *TieringJobExecutor) Start(ctx context.Context, jobQueue <-chan *TieringJob, wg *sync.WaitGroup) {
	defer wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			je.shutdownActiveJobs()
			return
		case job := <-jobQueue:
			if job == nil {
				je.shutdownActiveJobs()
				return // Channel closed
			}

			// Execute job in goroutine
			go je.executeJob(ctx, job)

		case <-ticker.C:
			// Update metrics and check for stalled jobs
			je.updateMetrics()
			je.checkForStalledJobs(ctx)
		}
	}
}

// executeJob executes a single tiering job
func (je *TieringJobExecutor) executeJob(ctx context.Context, job *TieringJob) {
	ctx, cancel := context.WithTimeout(ctx, job.Config.Timeout)
	defer cancel()

	ctx, span := je.tracer.Start(ctx, "execute_tiering_job")
	defer span.End()

	span.SetAttributes(
		attribute.String("job.id", job.ID.String()),
		attribute.String("job.type", job.Type),
		attribute.String("tenant.id", job.TenantID.String()),
	)

	// Update job status
	job.Status = StatusInProgress
	startTime := time.Now()
	job.StartedAt = &startTime
	job.Progress.StartTime = startTime
	job.Progress.LastUpdate = startTime

	if err := je.tieringManager.jobStore.UpdateJob(ctx, job); err != nil {
		span.RecordError(err)
		return
	}

	// Get a worker
	worker := <-je.workerPool
	defer func() {
		je.workerPool <- worker
	}()

	// Create job context
	jobCtx := &TieringJobContext{
		Job:       job,
		StartTime: startTime,
		Worker:    worker,
		Cancel:    cancel,
		Progress: &TieringJobProgress{
			Phase:        "starting",
			LastActivity: time.Now(),
		},
	}

	// Track active job
	je.mu.Lock()
	je.activeJobs[job.ID] = jobCtx
	je.mu.Unlock()

	defer func() {
		je.mu.Lock()
		delete(je.activeJobs, job.ID)
		je.mu.Unlock()
	}()

	// Execute job
	err := worker.ExecuteJob(ctx, job)

	// Update job completion
	completedAt := time.Now()
	job.CompletedAt = &completedAt
	job.Progress.LastUpdate = completedAt
	job.Results.Duration = completedAt.Sub(startTime)

	if err != nil {
		job.Status = StatusFailed
		job.ErrorMessage = err.Error()
		span.RecordError(err)
		je.metrics.FailedJobs++
	} else {
		job.Status = StatusCompleted
		je.metrics.CompletedJobs++
	}

	// Calculate performance metrics
	je.updateJobPerformanceMetrics(job)

	// Save final job state
	if err := je.tieringManager.jobStore.UpdateJob(ctx, job); err != nil {
		span.RecordError(err)
	}

	span.SetAttributes(
		attribute.String("job.status", string(job.Status)),
		attribute.Float64("duration.seconds", job.Results.Duration.Seconds()),
	)
}

// GetActiveJobs returns currently active jobs
func (je *TieringJobExecutor) GetActiveJobs() []*TieringJobContext {
	je.mu.RLock()
	defer je.mu.RUnlock()

	contexts := make([]*TieringJobContext, 0, len(je.activeJobs))
	for _, ctx := range je.activeJobs {
		contexts = append(contexts, ctx)
	}

	return contexts
}

// GetMetrics returns executor metrics
func (je *TieringJobExecutor) GetMetrics() *ExecutorMetrics {
	je.mu.RLock()
	defer je.mu.RUnlock()

	metrics := *je.metrics
	metrics.ProcessingJobs = len(je.activeJobs)
	metrics.ActiveWorkers = len(je.workers)

	return &metrics
}

// CancelJob cancels a running job
func (je *TieringJobExecutor) CancelJob(ctx context.Context, jobID uuid.UUID) error {
	je.mu.Lock()
	jobCtx, exists := je.activeJobs[jobID]
	je.mu.Unlock()

	if !exists {
		return fmt.Errorf("job %s is not currently running", jobID)
	}

	// Cancel the job context
	jobCtx.Cancel()

	// Update job status
	jobCtx.Job.Status = StatusCancelled
	now := time.Now()
	jobCtx.Job.CompletedAt = &now

	return je.tieringManager.jobStore.UpdateJob(ctx, jobCtx.Job)
}

// Helper methods

func (je *TieringJobExecutor) updateMetrics() {
	je.mu.Lock()
	defer je.mu.Unlock()

	// Calculate average job duration
	if je.metrics.CompletedJobs > 0 {
		// This would need to track actual durations
		// For now, using a placeholder calculation
	}

	// Calculate throughput
	timeSinceReset := time.Since(je.metrics.LastReset).Hours()
	if timeSinceReset > 0 {
		je.metrics.ThroughputJobsPerHour = float64(je.metrics.CompletedJobs) / timeSinceReset
	}

	// Calculate error rate
	totalJobs := je.metrics.CompletedJobs + je.metrics.FailedJobs
	if totalJobs > 0 {
		je.metrics.ErrorRate = float64(je.metrics.FailedJobs) / float64(totalJobs)
	}
}

func (je *TieringJobExecutor) checkForStalledJobs(ctx context.Context) {
	je.mu.RLock()
	stalledJobs := make([]*TieringJobContext, 0)

	for _, jobCtx := range je.activeJobs {
		// Check if job has been inactive for too long
		if time.Since(jobCtx.Progress.LastActivity) > 30*time.Minute {
			stalledJobs = append(stalledJobs, jobCtx)
		}
	}
	je.mu.RUnlock()

	// Handle stalled jobs
	for _, jobCtx := range stalledJobs {
		// Cancel stalled job
		jobCtx.Cancel()

		// Update job status
		jobCtx.Job.Status = StatusFailed
		jobCtx.Job.ErrorMessage = "Job stalled - no activity for 30 minutes"
		now := time.Now()
		jobCtx.Job.CompletedAt = &now

		je.tieringManager.jobStore.UpdateJob(ctx, jobCtx.Job)
	}
}

func (je *TieringJobExecutor) shutdownActiveJobs() {
	je.mu.Lock()
	defer je.mu.Unlock()

	for _, jobCtx := range je.activeJobs {
		jobCtx.Cancel()
	}
}

func (je *TieringJobExecutor) updateJobPerformanceMetrics(job *TieringJob) {
	// Calculate throughput metrics
	if job.Results.Duration > 0 {
		if job.Results.TotalBytesProcessed > 0 {
			mbps := float64(job.Results.TotalBytesProcessed) / (1024 * 1024) / job.Results.Duration.Seconds()
			job.Results.ThroughputMBps = mbps
		}

		if job.Results.TotalFilesProcessed > 0 {
			filesPerSec := float64(job.Results.TotalFilesProcessed) / job.Results.Duration.Seconds()
			job.Results.FilesPerSecond = filesPerSec
		}
	}
}

// DefaultTieringWorker implements TieringWorker interface
type DefaultTieringWorker struct {
	id               string
	storageManager   storage.StorageManager
	transitionEngine *TransitionEngine
	config           *TieringConfig
	available        bool
	metrics          *WorkerMetrics
	tracer           trace.Tracer
	mu               sync.Mutex
}

// NewDefaultTieringWorker creates a new default tiering worker
func NewDefaultTieringWorker(id string, storageManager storage.StorageManager, transitionEngine *TransitionEngine, config *TieringConfig) *DefaultTieringWorker {
	return &DefaultTieringWorker{
		id:               id,
		storageManager:   storageManager,
		transitionEngine: transitionEngine,
		config:           config,
		available:        true,
		metrics:          &WorkerMetrics{WorkerID: id},
		tracer:           otel.Tracer("tiering-worker"),
	}
}

// ExecuteJob executes a tiering job
func (w *DefaultTieringWorker) ExecuteJob(ctx context.Context, job *TieringJob) error {
	ctx, span := w.tracer.Start(ctx, "worker_execute_tiering_job")
	defer span.End()

	w.mu.Lock()
	w.available = false
	startTime := time.Now()
	w.mu.Unlock()

	defer func() {
		w.mu.Lock()
		w.available = true
		duration := time.Since(startTime)
		w.metrics.TotalProcessingTime += duration

		// Update average job time
		w.metrics.JobsCompleted++
		if w.metrics.JobsCompleted > 0 {
			w.metrics.AverageJobTime = w.metrics.TotalProcessingTime / time.Duration(w.metrics.JobsCompleted)
		}
		now := time.Now()
		w.metrics.LastJobCompleted = &now
		w.mu.Unlock()
	}()

	span.SetAttributes(
		attribute.String("worker.id", w.id),
		attribute.String("job.id", job.ID.String()),
		attribute.String("job.type", job.Type),
	)

	switch job.Type {
	case "policy_analysis":
		return w.executePolicyAnalysis(ctx, job)
	case "manual_transition":
		return w.executeManualTransition(ctx, job)
	case "cost_optimization":
		return w.executeCostOptimization(ctx, job)
	default:
		err := fmt.Errorf("unsupported job type: %s", job.Type)
		span.RecordError(err)
		return err
	}
}

// executePolicyAnalysis executes a policy-based analysis job
func (w *DefaultTieringWorker) executePolicyAnalysis(ctx context.Context, job *TieringJob) error {
	ctx, span := w.tracer.Start(ctx, "execute_policy_analysis")
	defer span.End()

	// Phase 1: File Discovery
	job.Progress.CurrentOperation = "discovering_files"

	// This would discover files based on policy criteria
	// For now, using a placeholder
	discoveredFiles := 0 // Placeholder

	job.Progress.TotalFiles = int64(discoveredFiles)
	job.Progress.LastUpdate = time.Now()

	// Phase 2: Analysis
	job.Progress.CurrentOperation = "analyzing_files"

	// Analyze each file and generate transition recommendations
	// This would use the AccessAnalyzer to determine optimal tiers

	// Phase 3: Transition Execution (if not dry run)
	if !job.Config.DryRun {
		job.Progress.CurrentOperation = "executing_transitions"

		// Execute recommended transitions
		// This would use the TransitionEngine
	}

	job.Progress.CurrentOperation = "completed"
	job.Progress.LastUpdate = time.Now()

	span.SetAttributes(
		attribute.Int("files.discovered", discoveredFiles),
		attribute.Bool("dry.run", job.Config.DryRun),
	)

	return nil
}

// executeManualTransition executes a manual tier transition job
func (w *DefaultTieringWorker) executeManualTransition(ctx context.Context, job *TieringJob) error {
	ctx, span := w.tracer.Start(ctx, "execute_manual_transition")
	defer span.End()

	// Phase 1: File Discovery and Validation
	job.Progress.CurrentOperation = "discovering_files"

	// Discover files that match the transition criteria
	// This would filter by FromTier and apply any path/file filters

	// Phase 2: Cost Estimation
	job.Progress.CurrentOperation = "estimating_costs"

	// Estimate transition costs
	var totalEstimatedCost float64
	job.Progress.EstimatedCost = totalEstimatedCost

	// Check against cost limits
	if job.Config.MaxCost > 0 && totalEstimatedCost > job.Config.MaxCost {
		return fmt.Errorf("estimated cost %.2f exceeds maximum allowed cost %.2f", totalEstimatedCost, job.Config.MaxCost)
	}

	// Phase 3: Transition Execution
	if !job.Config.DryRun {
		job.Progress.CurrentOperation = "executing_transitions"

		// Create transition requests
		var transitions []*TierTransition

		// Execute transitions through transition engine
		results, err := w.transitionEngine.ExecuteTransitions(ctx, transitions)
		if err != nil {
			span.RecordError(err)
			return fmt.Errorf("failed to execute transitions: %w", err)
		}

		// Update job results
		job.Results.TotalFilesProcessed = int64(len(results))
		for _, result := range results {
			if result.Success {
				job.Results.TotalBytesProcessed += result.FileSize
				job.Results.MonthlyCostSavings += result.MonthlySavings
			}
		}
	}

	job.Progress.CurrentOperation = "completed"
	job.Progress.LastUpdate = time.Now()

	span.SetAttributes(
		attribute.String("from.tier", string(job.FromTier)),
		attribute.String("to.tier", string(job.ToTier)),
		attribute.Float64("estimated.cost", totalEstimatedCost),
		attribute.Bool("dry.run", job.Config.DryRun),
	)

	return nil
}

// executeCostOptimization executes a cost optimization analysis job
func (w *DefaultTieringWorker) executeCostOptimization(ctx context.Context, job *TieringJob) error {
	ctx, span := w.tracer.Start(ctx, "execute_cost_optimization")
	defer span.End()

	// Phase 1: Data Collection
	job.Progress.CurrentOperation = "collecting_data"

	// Collect current storage usage and costs
	// This would query storage systems and metrics

	// Phase 2: Analysis
	job.Progress.CurrentOperation = "analyzing_optimization_opportunities"

	// Use CostOptimizer to find optimization opportunities
	// This would generate recommendations for tier transitions

	// Phase 3: Reporting
	job.Progress.CurrentOperation = "generating_report"

	// Generate optimization report
	// This would create detailed cost analysis and recommendations

	job.Progress.CurrentOperation = "completed"
	job.Progress.LastUpdate = time.Now()

	span.SetAttributes(
		attribute.String("tenant.id", job.TenantID.String()),
	)

	return nil
}

// GetID returns the worker ID
func (w *DefaultTieringWorker) GetID() string {
	return w.id
}

// IsAvailable returns whether the worker is available
func (w *DefaultTieringWorker) IsAvailable() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.available
}

// GetMetrics returns worker metrics
func (w *DefaultTieringWorker) GetMetrics() *WorkerMetrics {
	w.mu.Lock()
	defer w.mu.Unlock()

	metrics := *w.metrics
	return &metrics
}
