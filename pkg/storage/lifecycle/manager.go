package lifecycle

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/jscharber/eAIIngest/pkg/storage"
)

// LifecycleManager manages the lifecycle of data across storage systems
type LifecycleManager struct {
	config         *LifecycleConfig
	policyStore    PolicyStore
	jobStore       JobStore
	eventStore     EventStore
	storageManager storage.StorageManager
	scheduler      *cron.Cron
	jobExecutor    *JobExecutor
	tracer         trace.Tracer
	metrics        *LifecycleMetrics
	mu             sync.RWMutex
	
	// Runtime state
	activePolicies map[uuid.UUID]*LifecyclePolicy
	activeJobs     map[uuid.UUID]*LifecycleJob
	
	// Channels for coordination
	jobQueue       chan *LifecycleJob
	stopCh         chan struct{}
	wg             sync.WaitGroup
}

// PolicyStore interface for persistent policy storage
type PolicyStore interface {
	StorePolicy(ctx context.Context, policy *LifecyclePolicy) error
	GetPolicy(ctx context.Context, policyID uuid.UUID) (*LifecyclePolicy, error)
	ListPolicies(ctx context.Context, tenantID uuid.UUID) ([]*LifecyclePolicy, error)
	UpdatePolicy(ctx context.Context, policy *LifecyclePolicy) error
	DeletePolicy(ctx context.Context, policyID uuid.UUID) error
	GetActivePolicies(ctx context.Context) ([]*LifecyclePolicy, error)
}

// JobStore interface for persistent job storage
type JobStore interface {
	StoreJob(ctx context.Context, job *LifecycleJob) error
	GetJob(ctx context.Context, jobID uuid.UUID) (*LifecycleJob, error)
	UpdateJob(ctx context.Context, job *LifecycleJob) error
	ListJobs(ctx context.Context, filters JobFilters) ([]*LifecycleJob, error)
	DeleteJob(ctx context.Context, jobID uuid.UUID) error
}

// EventStore interface for event storage
type EventStore interface {
	StoreEvent(ctx context.Context, event *LifecycleEvent) error
	GetEvents(ctx context.Context, filters EventFilters) ([]*LifecycleEvent, error)
}

// JobFilters for filtering job queries
type JobFilters struct {
	TenantID   *uuid.UUID        `json:"tenant_id,omitempty"`
	PolicyID   *uuid.UUID        `json:"policy_id,omitempty"`
	Status     *LifecycleStatus  `json:"status,omitempty"`
	Type       *string           `json:"type,omitempty"`
	StartTime  *time.Time        `json:"start_time,omitempty"`
	EndTime    *time.Time        `json:"end_time,omitempty"`
	Limit      int               `json:"limit,omitempty"`
	Offset     int               `json:"offset,omitempty"`
}

// EventFilters for filtering event queries
type EventFilters struct {
	TenantID   *uuid.UUID  `json:"tenant_id,omitempty"`
	JobID      *uuid.UUID  `json:"job_id,omitempty"`
	PolicyID   *uuid.UUID  `json:"policy_id,omitempty"`
	EventType  *string     `json:"event_type,omitempty"`
	StartTime  *time.Time  `json:"start_time,omitempty"`
	EndTime    *time.Time  `json:"end_time,omitempty"`
	Limit      int         `json:"limit,omitempty"`
	Offset     int         `json:"offset,omitempty"`
}

// NewLifecycleManager creates a new lifecycle manager
func NewLifecycleManager(
	config *LifecycleConfig,
	policyStore PolicyStore,
	jobStore JobStore,
	eventStore EventStore,
	storageManager storage.StorageManager,
) *LifecycleManager {
	if config == nil {
		config = DefaultLifecycleConfig()
	}

	lm := &LifecycleManager{
		config:         config,
		policyStore:    policyStore,
		jobStore:       jobStore,
		eventStore:     eventStore,
		storageManager: storageManager,
		scheduler:      cron.New(cron.WithSeconds()),
		tracer:         otel.Tracer("lifecycle-manager"),
		metrics:        &LifecycleMetrics{},
		activePolicies: make(map[uuid.UUID]*LifecyclePolicy),
		activeJobs:     make(map[uuid.UUID]*LifecycleJob),
		jobQueue:       make(chan *LifecycleJob, 100),
		stopCh:         make(chan struct{}),
	}

	// Initialize job executor
	lm.jobExecutor = NewJobExecutor(config, storageManager, lm)

	return lm
}

// Start starts the lifecycle manager
func (lm *LifecycleManager) Start(ctx context.Context) error {
	ctx, span := lm.tracer.Start(ctx, "lifecycle_manager_start")
	defer span.End()

	// Load active policies
	if err := lm.loadActivePolicies(ctx); err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to load active policies: %w", err)
	}

	// Schedule policy execution
	if err := lm.schedulePolicies(); err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to schedule policies: %w", err)
	}

	// Start job executor
	lm.wg.Add(1)
	go lm.jobExecutor.Start(ctx, lm.jobQueue, &lm.wg)

	// Start job processor
	lm.wg.Add(1)
	go lm.processJobs(ctx)

	// Start scheduler
	lm.scheduler.Start()

	span.SetAttributes(
		attribute.Int("active_policies", len(lm.activePolicies)),
		attribute.Bool("scheduler_started", true),
	)

	return nil
}

// Stop stops the lifecycle manager
func (lm *LifecycleManager) Stop(ctx context.Context) error {
	ctx, span := lm.tracer.Start(ctx, "lifecycle_manager_stop")
	defer span.End()

	// Stop scheduler
	lm.scheduler.Stop()

	// Signal shutdown
	close(lm.stopCh)

	// Wait for goroutines to finish
	lm.wg.Wait()

	// Close job queue
	close(lm.jobQueue)

	return nil
}

// CreatePolicy creates a new lifecycle policy
func (lm *LifecycleManager) CreatePolicy(ctx context.Context, policy *LifecyclePolicy) error {
	ctx, span := lm.tracer.Start(ctx, "create_lifecycle_policy")
	defer span.End()

	if policy.ID == uuid.Nil {
		policy.ID = uuid.New()
	}

	// Validate policy
	if err := lm.validatePolicy(policy); err != nil {
		span.RecordError(err)
		return fmt.Errorf("invalid policy: %w", err)
	}

	// Set timestamps
	now := time.Now()
	policy.CreatedAt = now
	policy.UpdatedAt = now

	// Calculate next run time
	if policy.Schedule != "" {
		if nextRun, err := lm.calculateNextRun(policy.Schedule); err == nil {
			policy.NextRun = nextRun
		}
	}

	// Store policy
	if err := lm.policyStore.StorePolicy(ctx, policy); err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to store policy: %w", err)
	}

	// Add to active policies if enabled
	if policy.Enabled {
		lm.mu.Lock()
		lm.activePolicies[policy.ID] = policy
		lm.mu.Unlock()

		// Schedule the policy
		if err := lm.schedulePolicy(policy); err != nil {
			span.RecordError(err)
			// Log error but don't fail policy creation
		}
	}

	// Record event
	event := &LifecycleEvent{
		ID:        uuid.New(),
		Type:      "policy_created",
		Timestamp: time.Now(),
		TenantID:  policy.TenantID,
		PolicyID:  &policy.ID,
		Message:   fmt.Sprintf("Lifecycle policy '%s' created", policy.Name),
		Source:    "lifecycle_manager",
	}
	lm.eventStore.StoreEvent(ctx, event)

	span.SetAttributes(
		attribute.String("policy.id", policy.ID.String()),
		attribute.String("policy.name", policy.Name),
		attribute.Bool("policy.enabled", policy.Enabled),
	)

	return nil
}

// ExecutePolicy executes a lifecycle policy immediately
func (lm *LifecycleManager) ExecutePolicy(ctx context.Context, policyID uuid.UUID) (*LifecycleJob, error) {
	ctx, span := lm.tracer.Start(ctx, "execute_lifecycle_policy")
	defer span.End()

	// Get policy
	policy, err := lm.policyStore.GetPolicy(ctx, policyID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get policy: %w", err)
	}

	if !policy.Enabled {
		return nil, fmt.Errorf("policy %s is disabled", policyID)
	}

	// Create job
	job := &LifecycleJob{
		ID:        uuid.New(),
		PolicyID:  policy.ID,
		TenantID:  policy.TenantID,
		Type:      "policy_execution",
		Status:    StatusPending,
		CreatedAt: time.Now(),
		Config: LifecycleJobConfig{
			DryRun:         lm.config.DryRun || policy.Settings.DryRun,
			MaxConcurrency: lm.config.MaxConcurrentJobs,
			BatchSize:      100,
			Timeout:        lm.config.JobTimeout,
			RetryAttempts:  lm.config.RetryAttempts,
		},
		Progress: LifecycleProgress{
			StartTime: time.Now(),
		},
	}

	// Convert policy rules to job actions
	for _, rule := range policy.Rules {
		if rule.Enabled {
			job.Config.Actions = append(job.Config.Actions, rule.Actions...)
		}
	}

	// Store job
	if err := lm.jobStore.StoreJob(ctx, job); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to store job: %w", err)
	}

	// Queue job for execution
	select {
	case lm.jobQueue <- job:
		// Job queued successfully
	default:
		// Queue is full
		job.Status = StatusFailed
		job.ErrorMessage = "Job queue is full"
		lm.jobStore.UpdateJob(ctx, job)
		return nil, fmt.Errorf("job queue is full")
	}

	// Track active job
	lm.mu.Lock()
	lm.activeJobs[job.ID] = job
	lm.mu.Unlock()

	// Update metrics
	lm.metrics.TotalJobs++
	lm.metrics.ActiveJobs++

	span.SetAttributes(
		attribute.String("job.id", job.ID.String()),
		attribute.String("policy.id", policy.ID.String()),
		attribute.String("policy.name", policy.Name),
	)

	return job, nil
}

// GetJob retrieves a lifecycle job by ID
func (lm *LifecycleManager) GetJob(ctx context.Context, jobID uuid.UUID) (*LifecycleJob, error) {
	return lm.jobStore.GetJob(ctx, jobID)
}

// ListJobs lists lifecycle jobs with optional filters
func (lm *LifecycleManager) ListJobs(ctx context.Context, filters JobFilters) ([]*LifecycleJob, error) {
	return lm.jobStore.ListJobs(ctx, filters)
}

// CancelJob cancels a running job
func (lm *LifecycleManager) CancelJob(ctx context.Context, jobID uuid.UUID) error {
	ctx, span := lm.tracer.Start(ctx, "cancel_lifecycle_job")
	defer span.End()

	// Get job
	job, err := lm.jobStore.GetJob(ctx, jobID)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to get job: %w", err)
	}

	if job.Status != StatusPending && job.Status != StatusInProgress {
		return fmt.Errorf("job %s cannot be cancelled (status: %s)", jobID, job.Status)
	}

	// Update job status
	job.Status = StatusCancelled
	now := time.Now()
	job.CompletedAt = &now

	if err := lm.jobStore.UpdateJob(ctx, job); err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to update job: %w", err)
	}

	// Remove from active jobs
	lm.mu.Lock()
	delete(lm.activeJobs, job.ID)
	lm.mu.Unlock()

	// Update metrics
	lm.metrics.ActiveJobs--

	// Record event
	event := &LifecycleEvent{
		ID:        uuid.New(),
		Type:      "job_cancelled",
		Timestamp: time.Now(),
		TenantID:  job.TenantID,
		JobID:     &job.ID,
		Message:   fmt.Sprintf("Lifecycle job %s cancelled", jobID),
		Source:    "lifecycle_manager",
	}
	lm.eventStore.StoreEvent(ctx, event)

	span.SetAttributes(
		attribute.String("job.id", jobID.String()),
		attribute.String("job.status", string(job.Status)),
	)

	return nil
}

// GetMetrics returns lifecycle system metrics
func (lm *LifecycleManager) GetMetrics(ctx context.Context) *LifecycleMetrics {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	// Create a copy of metrics to avoid race conditions
	metrics := *lm.metrics
	metrics.ActiveJobs = int64(len(lm.activeJobs))

	return &metrics
}

// Helper methods

func (lm *LifecycleManager) loadActivePolicies(ctx context.Context) error {
	policies, err := lm.policyStore.GetActivePolicies(ctx)
	if err != nil {
		return err
	}

	lm.mu.Lock()
	defer lm.mu.Unlock()

	for _, policy := range policies {
		lm.activePolicies[policy.ID] = policy
	}

	return nil
}

func (lm *LifecycleManager) schedulePolicies() error {
	lm.mu.RLock()
	policies := make([]*LifecyclePolicy, 0, len(lm.activePolicies))
	for _, policy := range lm.activePolicies {
		policies = append(policies, policy)
	}
	lm.mu.RUnlock()

	for _, policy := range policies {
		if err := lm.schedulePolicy(policy); err != nil {
			// Log error but continue with other policies
			continue
		}
	}

	return nil
}

func (lm *LifecycleManager) schedulePolicy(policy *LifecyclePolicy) error {
	if policy.Schedule == "" {
		policy.Schedule = lm.config.DefaultSchedule
	}

	_, err := lm.scheduler.AddFunc(policy.Schedule, func() {
		ctx := context.Background()
		_, err := lm.ExecutePolicy(ctx, policy.ID)
		if err != nil {
			// Log error
			event := &LifecycleEvent{
				ID:        uuid.New(),
				Type:      "policy_execution_failed",
				Timestamp: time.Now(),
				TenantID:  policy.TenantID,
				PolicyID:  &policy.ID,
				Message:   fmt.Sprintf("Failed to execute policy '%s': %s", policy.Name, err.Error()),
				Source:    "lifecycle_manager",
			}
			lm.eventStore.StoreEvent(ctx, event)
		}
	})

	return err
}

func (lm *LifecycleManager) calculateNextRun(schedule string) (time.Time, error) {
	cronSchedule, err := cron.ParseStandard(schedule)
	if err != nil {
		return time.Time{}, err
	}
	return cronSchedule.Next(time.Now()), nil
}

func (lm *LifecycleManager) validatePolicy(policy *LifecyclePolicy) error {
	if policy.Name == "" {
		return fmt.Errorf("policy name is required")
	}

	if policy.TenantID == uuid.Nil {
		return fmt.Errorf("tenant ID is required")
	}

	// Validate schedule if provided
	if policy.Schedule != "" {
		if _, err := cron.ParseStandard(policy.Schedule); err != nil {
			return fmt.Errorf("invalid schedule: %w", err)
		}
	}

	// Validate rules
	for i, rule := range policy.Rules {
		if err := lm.validateRule(rule); err != nil {
			return fmt.Errorf("invalid rule %d: %w", i, err)
		}
	}

	return nil
}

func (lm *LifecycleManager) validateRule(rule LifecycleRule) error {
	if rule.Name == "" {
		return fmt.Errorf("rule name is required")
	}

	if len(rule.Conditions) == 0 {
		return fmt.Errorf("rule must have at least one condition")
	}

	if len(rule.Actions) == 0 {
		return fmt.Errorf("rule must have at least one action")
	}

	// Validate conditions
	for i, condition := range rule.Conditions {
		if err := lm.validateCondition(condition); err != nil {
			return fmt.Errorf("invalid condition %d: %w", i, err)
		}
	}

	// Validate actions
	for i, action := range rule.Actions {
		if err := lm.validateAction(action); err != nil {
			return fmt.Errorf("invalid action %d: %w", i, err)
		}
	}

	return nil
}

func (lm *LifecycleManager) validateCondition(condition LifecycleCondition) error {
	validTypes := []ConditionType{
		ConditionAge, ConditionLastAccessed, ConditionFileSize,
		ConditionFileType, ConditionPath, ConditionStorageClass,
		ConditionTags, ConditionMetadata, ConditionAccessCount,
		ConditionCost, ConditionCompliance, ConditionDataClassification,
	}

	validType := false
	for _, vt := range validTypes {
		if condition.Type == vt {
			validType = true
			break
		}
	}

	if !validType {
		return fmt.Errorf("invalid condition type: %s", condition.Type)
	}

	if condition.Operator == "" {
		return fmt.Errorf("condition operator is required")
	}

	return nil
}

func (lm *LifecycleManager) validateAction(action LifecycleActionSpec) error {
	validActions := []LifecycleAction{
		ActionArchive, ActionDelete, ActionCompress, ActionMove,
		ActionEncrypt, ActionReplicate, ActionNotify, ActionTag, ActionAudit,
	}

	validAction := false
	for _, va := range validActions {
		if action.Action == va {
			validAction = true
			break
		}
	}

	if !validAction {
		return fmt.Errorf("invalid action: %s", action.Action)
	}

	return nil
}

func (lm *LifecycleManager) processJobs(ctx context.Context) {
	defer lm.wg.Done()

	for {
		select {
		case <-lm.stopCh:
			return
		case <-ctx.Done():
			return
		default:
			// Process any completed jobs
			lm.updateJobStatuses(ctx)
			time.Sleep(10 * time.Second)
		}
	}
}

func (lm *LifecycleManager) updateJobStatuses(ctx context.Context) {
	lm.mu.RLock()
	jobs := make([]*LifecycleJob, 0, len(lm.activeJobs))
	for _, job := range lm.activeJobs {
		jobs = append(jobs, job)
	}
	lm.mu.RUnlock()

	for _, job := range jobs {
		// Get updated job status
		updatedJob, err := lm.jobStore.GetJob(ctx, job.ID)
		if err != nil {
			continue
		}

		// Update local cache
		lm.mu.Lock()
		lm.activeJobs[job.ID] = updatedJob
		lm.mu.Unlock()

		// Remove completed jobs from active list
		if updatedJob.Status == StatusCompleted || 
		   updatedJob.Status == StatusFailed || 
		   updatedJob.Status == StatusCancelled {
			lm.mu.Lock()
			delete(lm.activeJobs, job.ID)
			lm.mu.Unlock()

			// Update metrics
			lm.metrics.ActiveJobs--
			if updatedJob.Status == StatusCompleted {
				lm.metrics.CompletedJobs++
			} else if updatedJob.Status == StatusFailed {
				lm.metrics.FailedJobs++
			}
		}
	}
}