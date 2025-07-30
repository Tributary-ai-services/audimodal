package sync

import (
	"container/heap"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// SyncQueueManager manages sync job queuing and priority scheduling
type SyncQueueManager struct {
	queues       map[SyncPriority]*PriorityQueue
	activeJobs   map[uuid.UUID]*QueuedSyncJob
	mutex        sync.RWMutex
	
	// Configuration
	config       *QueueConfig
	
	// Channels for job processing
	jobChan      chan *QueuedSyncJob
	stopChan     chan struct{}
	
	// Metrics and monitoring
	queueMetrics *QueueMetrics
	
	tracer       trace.Tracer
}

// QueueConfig contains queue management configuration
type QueueConfig struct {
	MaxConcurrentJobs    int           `json:"max_concurrent_jobs"`
	MaxQueueSize         int           `json:"max_queue_size"`
	JobTimeoutDuration   time.Duration `json:"job_timeout_duration"`
	QueueTimeoutDuration time.Duration `json:"queue_timeout_duration"`
	EnableJobBatching    bool          `json:"enable_job_batching"`
	BatchSize            int           `json:"batch_size"`
	BatchTimeout         time.Duration `json:"batch_timeout"`
	PriorityWeights      map[SyncPriority]float64 `json:"priority_weights"`
	EnableDynamicPriority bool         `json:"enable_dynamic_priority"`
	ResourceLimits       *ResourceLimits `json:"resource_limits"`
}

// ResourceLimits defines resource constraints for job execution
type ResourceLimits struct {
	MaxMemoryMB      int64   `json:"max_memory_mb"`
	MaxCPUPercent    float64 `json:"max_cpu_percent"`
	MaxBandwidthMBps int64   `json:"max_bandwidth_mbps"`
	MaxConnections   int     `json:"max_connections"`
}

// QueuedSyncJob represents a sync job in the queue
type QueuedSyncJob struct {
	ID               uuid.UUID            `json:"id"`
	DataSourceID     uuid.UUID            `json:"data_source_id"`
	ConnectorType    string               `json:"connector_type"`
	Options          *UnifiedSyncOptions  `json:"options"`
	Priority         SyncPriority         `json:"priority"`
	QueuedAt         time.Time            `json:"queued_at"`
	ScheduledFor     *time.Time           `json:"scheduled_for,omitempty"`
	EstimatedDuration time.Duration       `json:"estimated_duration"`
	ResourceRequirements *ResourceRequirements `json:"resource_requirements,omitempty"`
	Dependencies     []uuid.UUID          `json:"dependencies,omitempty"`
	RetryCount       int                  `json:"retry_count"`
	MaxRetries       int                  `json:"max_retries"`
	LastAttempt      *time.Time           `json:"last_attempt,omitempty"`
	FailureReason    string               `json:"failure_reason,omitempty"`
	
	// Internal fields for priority queue
	index            int                  `json:"-"`
	dynamicPriority  float64              `json:"-"`
}

// ResourceRequirements defines resource requirements for a job
type ResourceRequirements struct {
	EstimatedMemoryMB int64   `json:"estimated_memory_mb"`
	EstimatedCPU      float64 `json:"estimated_cpu"`
	EstimatedBandwidth int64  `json:"estimated_bandwidth_mbps"`
	RequiredConnections int   `json:"required_connections"`
}

// PriorityQueue implements a priority queue for sync jobs
type PriorityQueue struct {
	jobs     []*QueuedSyncJob
	priority SyncPriority
}

// QueueMetrics tracks queue performance metrics
type QueueMetrics struct {
	TotalJobsQueued      int64            `json:"total_jobs_queued"`
	TotalJobsProcessed   int64            `json:"total_jobs_processed"`
	TotalJobsFailed      int64            `json:"total_jobs_failed"`
	AverageQueueTime     time.Duration    `json:"average_queue_time"`
	AverageProcessingTime time.Duration   `json:"average_processing_time"`
	CurrentQueueSizes    map[SyncPriority]int `json:"current_queue_sizes"`
	ActiveJobCount       int              `json:"active_job_count"`
	ResourceUtilization  *ResourceUtilization `json:"resource_utilization"`
	LastUpdated          time.Time        `json:"last_updated"`
}

// ResourceUtilization tracks current resource usage
type ResourceUtilization struct {
	MemoryUsageMB     int64   `json:"memory_usage_mb"`
	CPUUsagePercent   float64 `json:"cpu_usage_percent"`
	BandwidthUsageMBps int64  `json:"bandwidth_usage_mbps"`
	ActiveConnections int     `json:"active_connections"`
}

// NewSyncQueueManager creates a new sync queue manager
func NewSyncQueueManager(config *QueueConfig) *SyncQueueManager {
	if config == nil {
		config = &QueueConfig{
			MaxConcurrentJobs:    10,
			MaxQueueSize:         1000,
			JobTimeoutDuration:   2 * time.Hour,
			QueueTimeoutDuration: 24 * time.Hour,
			EnableJobBatching:    false,
			BatchSize:            5,
			BatchTimeout:         5 * time.Minute,
			PriorityWeights: map[SyncPriority]float64{
				SyncPriorityUrgent: 15.0,
				SyncPriorityHigh:   10.0,
				SyncPriorityNormal: 5.0,
				SyncPriorityLow:    1.0,
			},
			EnableDynamicPriority: true,
			ResourceLimits: &ResourceLimits{
				MaxMemoryMB:      8192,  // 8GB
				MaxCPUPercent:    80.0,
				MaxBandwidthMBps: 1000,  // 1Gbps
				MaxConnections:   100,
			},
		}
	}

	manager := &SyncQueueManager{
		queues:     make(map[SyncPriority]*PriorityQueue),
		activeJobs: make(map[uuid.UUID]*QueuedSyncJob),
		config:     config,
		jobChan:    make(chan *QueuedSyncJob, config.MaxConcurrentJobs),
		stopChan:   make(chan struct{}),
		queueMetrics: &QueueMetrics{
			CurrentQueueSizes: make(map[SyncPriority]int),
			ResourceUtilization: &ResourceUtilization{},
			LastUpdated: time.Now(),
		},
		tracer: otel.Tracer("sync-queue-manager"),
	}

	// Initialize priority queues
	for _, priority := range []SyncPriority{SyncPriorityLow, SyncPriorityNormal, SyncPriorityHigh, SyncPriorityUrgent} {
		manager.queues[priority] = &PriorityQueue{
			jobs:     make([]*QueuedSyncJob, 0),
			priority: priority,
		}
	}

	// Start background processing
	manager.startProcessing()

	return manager
}

// EnqueueJob adds a sync job to the appropriate priority queue
func (sqm *SyncQueueManager) EnqueueJob(ctx context.Context, request *StartSyncRequest) (*QueuedSyncJob, error) {
	ctx, span := sqm.tracer.Start(ctx, "queue_manager.enqueue_job")
	defer span.End()

	span.SetAttributes(
		attribute.String("data_source_id", request.DataSourceID.String()),
		attribute.String("connector_type", request.ConnectorType),
		attribute.String("priority", string(request.Options.Priority)),
	)

	// Check queue size limits
	if err := sqm.checkQueueLimits(); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("queue limit exceeded: %w", err)
	}

	// Create queued job
	queuedJob := &QueuedSyncJob{
		ID:               uuid.New(),
		DataSourceID:     request.DataSourceID,
		ConnectorType:    request.ConnectorType,
		Options:          request.Options,
		Priority:         request.Options.Priority,
		QueuedAt:         time.Now(),
		EstimatedDuration: sqm.estimateJobDuration(request),
		ResourceRequirements: sqm.estimateResourceRequirements(request),
		MaxRetries:       3, // Default retry count
	}

	// Handle scheduled jobs
	if request.ScheduleOptions != nil && request.ScheduleOptions.StartTime != nil {
		queuedJob.ScheduledFor = request.ScheduleOptions.StartTime
	}

	// Calculate dynamic priority if enabled
	if sqm.config.EnableDynamicPriority {
		queuedJob.dynamicPriority = sqm.calculateDynamicPriority(queuedJob)
	}

	// Add to appropriate queue
	sqm.mutex.Lock()
	defer sqm.mutex.Unlock()

	queue := sqm.queues[queuedJob.Priority]
	heap.Push(queue, queuedJob)
	
	// Update metrics
	sqm.queueMetrics.TotalJobsQueued++
	sqm.queueMetrics.CurrentQueueSizes[queuedJob.Priority]++
	sqm.queueMetrics.LastUpdated = time.Now()

	span.SetAttributes(
		attribute.String("job_id", queuedJob.ID.String()),
		attribute.Int("queue_size", len(queue.jobs)),
	)

	return queuedJob, nil
}

// DequeueJob removes and returns the highest priority job that can be executed
func (sqm *SyncQueueManager) DequeueJob(ctx context.Context) (*QueuedSyncJob, error) {
	ctx, span := sqm.tracer.Start(ctx, "queue_manager.dequeue_job")
	defer span.End()

	sqm.mutex.Lock()
	defer sqm.mutex.Unlock()

	// Check resource availability
	if !sqm.hasAvailableResources() {
		return nil, fmt.Errorf("insufficient resources available")
	}

	// Find highest priority job that can be executed
	priorities := []SyncPriority{SyncPriorityUrgent, SyncPriorityHigh, SyncPriorityNormal, SyncPriorityLow}
	
	for _, priority := range priorities {
		queue := sqm.queues[priority]
		if len(queue.jobs) == 0 {
			continue
		}

		// Check each job in priority order
		for i, job := range queue.jobs {
			// Skip scheduled jobs that aren't ready
			if job.ScheduledFor != nil && job.ScheduledFor.After(time.Now()) {
				continue
			}

			// Skip jobs with unmet dependencies
			if !sqm.dependenciesMet(job) {
				continue
			}

			// Skip jobs that would exceed resource limits
			if !sqm.canExecuteJob(job) {
				continue
			}

			// Remove job from queue
			heap.Remove(queue, i)
			
			// Mark as active
			sqm.activeJobs[job.ID] = job
			
			// Update metrics
			sqm.queueMetrics.CurrentQueueSizes[priority]--
			sqm.queueMetrics.ActiveJobCount++

			span.SetAttributes(
				attribute.String("job_id", job.ID.String()),
				attribute.String("priority", string(priority)),
				attribute.Duration("queue_time", time.Since(job.QueuedAt)),
			)

			return job, nil
		}
	}

	return nil, fmt.Errorf("no eligible jobs available")
}

// GetQueueStatus returns the current status of all queues
func (sqm *SyncQueueManager) GetQueueStatus(ctx context.Context) (*QueueStatus, error) {
	ctx, span := sqm.tracer.Start(ctx, "queue_manager.get_queue_status")
	defer span.End()

	sqm.mutex.RLock()
	defer sqm.mutex.RUnlock()

	status := &QueueStatus{
		QueueSizes:      make(map[SyncPriority]int),
		ActiveJobs:      len(sqm.activeJobs),
		TotalQueued:     0,
		Metrics:         sqm.queueMetrics,
		ResourceStatus:  sqm.getResourceStatus(),
		EstimatedWaitTimes: make(map[SyncPriority]time.Duration),
	}

	// Calculate queue sizes and wait times
	for priority, queue := range sqm.queues {
		size := len(queue.jobs)
		status.QueueSizes[priority] = size
		status.TotalQueued += size
		
		// Estimate wait time based on queue size and average processing time
		if sqm.queueMetrics.AverageProcessingTime > 0 {
			status.EstimatedWaitTimes[priority] = time.Duration(size) * sqm.queueMetrics.AverageProcessingTime
		}
	}

	// Get sample of queued jobs
	status.SampleQueuedJobs = sqm.getSampleQueuedJobs(10)

	span.SetAttributes(
		attribute.Int("total_queued", status.TotalQueued),
		attribute.Int("active_jobs", status.ActiveJobs),
	)

	return status, nil
}

// CompleteJob marks a job as completed and updates metrics
func (sqm *SyncQueueManager) CompleteJob(ctx context.Context, jobID uuid.UUID, success bool, duration time.Duration) error {
	ctx, span := sqm.tracer.Start(ctx, "queue_manager.complete_job")
	defer span.End()

	span.SetAttributes(
		attribute.String("job_id", jobID.String()),
		attribute.Bool("success", success),
		attribute.Duration("duration", duration),
	)

	sqm.mutex.Lock()
	defer sqm.mutex.Unlock()

	job, exists := sqm.activeJobs[jobID]
	if !exists {
		return fmt.Errorf("job not found in active jobs")
	}

	// Remove from active jobs
	delete(sqm.activeJobs, jobID)

	// Update metrics
	sqm.queueMetrics.ActiveJobCount--
	sqm.queueMetrics.TotalJobsProcessed++
	
	if success {
		// Update average processing time
		if sqm.queueMetrics.AverageProcessingTime == 0 {
			sqm.queueMetrics.AverageProcessingTime = duration
		} else {
			sqm.queueMetrics.AverageProcessingTime = (sqm.queueMetrics.AverageProcessingTime + duration) / 2
		}
		
		// Update average queue time
		queueTime := time.Since(job.QueuedAt)
		if sqm.queueMetrics.AverageQueueTime == 0 {
			sqm.queueMetrics.AverageQueueTime = queueTime
		} else {
			sqm.queueMetrics.AverageQueueTime = (sqm.queueMetrics.AverageQueueTime + queueTime) / 2
		}
	} else {
		sqm.queueMetrics.TotalJobsFailed++
		
		// Handle job retry logic
		if job.RetryCount < job.MaxRetries {
			job.RetryCount++
			job.LastAttempt = &[]time.Time{time.Now()}[0]
			
			// Re-queue with delay
			go func() {
				delay := time.Duration(job.RetryCount) * time.Minute
				time.Sleep(delay)
				
				// Re-add to queue with potentially lower priority
				if job.Priority > SyncPriorityLow {
					job.Priority--
				}
				
				queue := sqm.queues[job.Priority]
				heap.Push(queue, job)
				sqm.queueMetrics.CurrentQueueSizes[job.Priority]++
			}()
		}
	}

	sqm.queueMetrics.LastUpdated = time.Now()

	return nil
}

// CancelJob removes a job from the queue or cancels an active job
func (sqm *SyncQueueManager) CancelJob(ctx context.Context, jobID uuid.UUID) error {
	ctx, span := sqm.tracer.Start(ctx, "queue_manager.cancel_job")
	defer span.End()

	span.SetAttributes(attribute.String("job_id", jobID.String()))

	sqm.mutex.Lock()
	defer sqm.mutex.Unlock()

	// Check if job is active
	if job, exists := sqm.activeJobs[jobID]; exists {
		delete(sqm.activeJobs, jobID)
		sqm.queueMetrics.ActiveJobCount--
		
		span.SetAttributes(attribute.String("job_status", "active_cancelled"))
		return nil
	}

	// Search queues for the job
	for priority, queue := range sqm.queues {
		for i, job := range queue.jobs {
			if job.ID == jobID {
				heap.Remove(queue, i)
				sqm.queueMetrics.CurrentQueueSizes[priority]--
				
				span.SetAttributes(attribute.String("job_status", "queued_cancelled"))
				return nil
			}
		}
	}

	return fmt.Errorf("job not found")
}

// UpdateJobPriority changes the priority of a queued job
func (sqm *SyncQueueManager) UpdateJobPriority(ctx context.Context, jobID uuid.UUID, newPriority SyncPriority) error {
	ctx, span := sqm.tracer.Start(ctx, "queue_manager.update_job_priority")
	defer span.End()

	span.SetAttributes(
		attribute.String("job_id", jobID.String()),
		attribute.String("new_priority", string(newPriority)),
	)

	sqm.mutex.Lock()
	defer sqm.mutex.Unlock()

	// Find job in current queues
	for priority, queue := range sqm.queues {
		for i, job := range queue.jobs {
			if job.ID == jobID {
				// Remove from current queue
				heap.Remove(queue, i)
				sqm.queueMetrics.CurrentQueueSizes[priority]--
				
				// Update priority and re-queue
				job.Priority = newPriority
				if sqm.config.EnableDynamicPriority {
					job.dynamicPriority = sqm.calculateDynamicPriority(job)
				}
				
				newQueue := sqm.queues[newPriority]
				heap.Push(newQueue, job)
				sqm.queueMetrics.CurrentQueueSizes[newPriority]++
				
				return nil
			}
		}
	}

	return fmt.Errorf("job not found in queues")
}

// Internal helper methods

func (sqm *SyncQueueManager) checkQueueLimits() error {
	totalQueued := 0
	for _, queue := range sqm.queues {
		totalQueued += len(queue.jobs)
	}
	
	if totalQueued >= sqm.config.MaxQueueSize {
		return fmt.Errorf("queue size limit exceeded: %d/%d", totalQueued, sqm.config.MaxQueueSize)
	}
	
	return nil
}

func (sqm *SyncQueueManager) estimateJobDuration(request *StartSyncRequest) time.Duration {
	// Simple estimation based on sync type and connector
	baseDuration := 10 * time.Minute
	
	switch request.Options.SyncType {
	case SyncTypeFull:
		baseDuration = 30 * time.Minute
	case SyncTypeIncremental:
		baseDuration = 10 * time.Minute
	case SyncTypeRealtime:
		baseDuration = 5 * time.Minute
	}
	
	// Adjust based on connector type
	switch request.ConnectorType {
	case "googledrive", "onedrive":
		baseDuration = time.Duration(float64(baseDuration) * 1.2)
	case "sharepoint":
		baseDuration = time.Duration(float64(baseDuration) * 1.5)
	}
	
	return baseDuration
}

func (sqm *SyncQueueManager) estimateResourceRequirements(request *StartSyncRequest) *ResourceRequirements {
	requirements := &ResourceRequirements{
		EstimatedMemoryMB:   512,  // 512MB base
		EstimatedCPU:        0.5,  // 50% CPU
		EstimatedBandwidth:  10,   // 10 Mbps
		RequiredConnections: 5,    // 5 connections
	}
	
	// Adjust based on sync type
	switch request.Options.SyncType {
	case SyncTypeFull:
		requirements.EstimatedMemoryMB *= 2
		requirements.EstimatedCPU *= 1.5
		requirements.EstimatedBandwidth *= 2
	case SyncTypeRealtime:
		requirements.EstimatedMemoryMB = int64(float64(requirements.EstimatedMemoryMB) * 0.8)
		requirements.RequiredConnections *= 2
	}
	
	return requirements
}

func (sqm *SyncQueueManager) calculateDynamicPriority(job *QueuedSyncJob) float64 {
	basePriority := sqm.config.PriorityWeights[job.Priority]
	
	// Adjust for wait time (older jobs get higher priority)
	waitTime := time.Since(job.QueuedAt)
	ageBonus := float64(waitTime.Hours()) * 0.1
	
	// Adjust for retry count (failed jobs get slightly lower priority)
	retryPenalty := float64(job.RetryCount) * 0.5
	
	// Adjust for estimated duration (shorter jobs get slight boost)
	durationFactor := 1.0
	if job.EstimatedDuration < 10*time.Minute {
		durationFactor = 1.1
	} else if job.EstimatedDuration > 1*time.Hour {
		durationFactor = 0.9
	}
	
	return (basePriority + ageBonus - retryPenalty) * durationFactor
}

func (sqm *SyncQueueManager) hasAvailableResources() bool {
	if len(sqm.activeJobs) >= sqm.config.MaxConcurrentJobs {
		return false
	}
	
	// Check resource utilization
	utilization := sqm.queueMetrics.ResourceUtilization
	limits := sqm.config.ResourceLimits
	
	if utilization.MemoryUsageMB >= limits.MaxMemoryMB {
		return false
	}
	
	if utilization.CPUUsagePercent >= limits.MaxCPUPercent {
		return false
	}
	
	if utilization.ActiveConnections >= limits.MaxConnections {
		return false
	}
	
	return true
}

func (sqm *SyncQueueManager) dependenciesMet(job *QueuedSyncJob) bool {
	for _, depID := range job.Dependencies {
		if _, exists := sqm.activeJobs[depID]; exists {
			return false // Dependency still running
		}
	}
	return true
}

func (sqm *SyncQueueManager) canExecuteJob(job *QueuedSyncJob) bool {
	if job.ResourceRequirements == nil {
		return true
	}
	
	utilization := sqm.queueMetrics.ResourceUtilization
	limits := sqm.config.ResourceLimits
	requirements := job.ResourceRequirements
	
	// Check if adding this job would exceed limits
	if utilization.MemoryUsageMB + requirements.EstimatedMemoryMB > limits.MaxMemoryMB {
		return false
	}
	
	if utilization.ActiveConnections + requirements.RequiredConnections > limits.MaxConnections {
		return false
	}
	
	return true
}

func (sqm *SyncQueueManager) getResourceStatus() *ResourceStatus {
	utilization := sqm.queueMetrics.ResourceUtilization
	limits := sqm.config.ResourceLimits
	
	return &ResourceStatus{
		Memory: &ResourceMetric{
			Used:  utilization.MemoryUsageMB,
			Limit: limits.MaxMemoryMB,
			Usage: float64(utilization.MemoryUsageMB) / float64(limits.MaxMemoryMB) * 100,
		},
		CPU: &ResourceMetric{
			Used:  int64(utilization.CPUUsagePercent),
			Limit: int64(limits.MaxCPUPercent),
			Usage: utilization.CPUUsagePercent,
		},
		Connections: &ResourceMetric{
			Used:  int64(utilization.ActiveConnections),
			Limit: int64(limits.MaxConnections),
			Usage: float64(utilization.ActiveConnections) / float64(limits.MaxConnections) * 100,
		},
	}
}

func (sqm *SyncQueueManager) getSampleQueuedJobs(limit int) []*QueuedSyncJob {
	var jobs []*QueuedSyncJob
	collected := 0
	
	priorities := []SyncPriority{SyncPriorityUrgent, SyncPriorityHigh, SyncPriorityNormal, SyncPriorityLow}
	
	for _, priority := range priorities {
		queue := sqm.queues[priority]
		for _, job := range queue.jobs {
			if collected >= limit {
				break
			}
			jobs = append(jobs, job)
			collected++
		}
	}
	
	return jobs
}

func (sqm *SyncQueueManager) startProcessing() {
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		
		for {
			select {
			case <-sqm.stopChan:
				return
			case <-ticker.C:
				sqm.updateResourceUtilization()
				sqm.cleanupExpiredJobs()
			}
		}
	}()
}

func (sqm *SyncQueueManager) updateResourceUtilization() {
	// This would integrate with system monitoring to get actual resource usage
	// For now, we'll simulate based on active jobs
	
	sqm.mutex.RLock()
	activeCount := len(sqm.activeJobs)
	sqm.mutex.RUnlock()
	
	// Simulate resource usage based on active jobs
	sqm.queueMetrics.ResourceUtilization.MemoryUsageMB = int64(activeCount * 512)
	sqm.queueMetrics.ResourceUtilization.CPUUsagePercent = float64(activeCount) * 10.0
	sqm.queueMetrics.ResourceUtilization.ActiveConnections = activeCount * 5
	sqm.queueMetrics.ResourceUtilization.BandwidthUsageMBps = int64(activeCount * 10)
}

func (sqm *SyncQueueManager) cleanupExpiredJobs() {
	now := time.Now()
	cutoff := now.Add(-sqm.config.QueueTimeoutDuration)
	
	sqm.mutex.Lock()
	defer sqm.mutex.Unlock()
	
	for priority, queue := range sqm.queues {
		for i := len(queue.jobs) - 1; i >= 0; i-- {
			job := queue.jobs[i]
			if job.QueuedAt.Before(cutoff) {
				heap.Remove(queue, i)
				sqm.queueMetrics.CurrentQueueSizes[priority]--
			}
		}
	}
}

// Shutdown gracefully shuts down the queue manager
func (sqm *SyncQueueManager) Shutdown(ctx context.Context) error {
	close(sqm.stopChan)
	return nil
}

// Priority queue implementation
func (pq PriorityQueue) Len() int { return len(pq.jobs) }

func (pq PriorityQueue) Less(i, j int) bool {
	// Higher priority and dynamic priority comes first
	if pq.jobs[i].Priority != pq.jobs[j].Priority {
		return pq.jobs[i].Priority > pq.jobs[j].Priority
	}
	return pq.jobs[i].dynamicPriority > pq.jobs[j].dynamicPriority
}

func (pq PriorityQueue) Swap(i, j int) {
	pq.jobs[i], pq.jobs[j] = pq.jobs[j], pq.jobs[i]
	pq.jobs[i].index = i
	pq.jobs[j].index = j
}

func (pq *PriorityQueue) Push(x interface{}) {
	n := len(pq.jobs)
	job := x.(*QueuedSyncJob)
	job.index = n
	pq.jobs = append(pq.jobs, job)
}

func (pq *PriorityQueue) Pop() interface{} {
	old := pq.jobs
	n := len(old)
	job := old[n-1]
	old[n-1] = nil
	job.index = -1
	pq.jobs = old[0 : n-1]
	return job
}

// Additional types for queue status
type QueueStatus struct {
	QueueSizes         map[SyncPriority]int  `json:"queue_sizes"`
	ActiveJobs         int                   `json:"active_jobs"`
	TotalQueued        int                   `json:"total_queued"`
	EstimatedWaitTimes map[SyncPriority]time.Duration `json:"estimated_wait_times"`
	Metrics            *QueueMetrics         `json:"metrics"`
	ResourceStatus     *ResourceStatus       `json:"resource_status"`
	SampleQueuedJobs   []*QueuedSyncJob     `json:"sample_queued_jobs"`
}

type ResourceStatus struct {
	Memory      *ResourceMetric `json:"memory"`
	CPU         *ResourceMetric `json:"cpu"`
	Connections *ResourceMetric `json:"connections"`
}

type ResourceMetric struct {
	Used  int64   `json:"used"`
	Limit int64   `json:"limit"`
	Usage float64 `json:"usage_percent"`
}