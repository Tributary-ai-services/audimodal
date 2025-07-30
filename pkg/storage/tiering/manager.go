package tiering

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/jscharber/eAIIngest/pkg/storage"
)

// TieringManager manages multi-tier storage operations and policies
type TieringManager struct {
	config            *TieringConfig
	policyStore       PolicyStore
	jobStore          JobStore
	metricsStore      MetricsStore
	recommendationStore RecommendationStore
	storageManager    storage.StorageManager
	
	// Analysis and optimization
	accessAnalyzer    *AccessAnalyzer
	costOptimizer     *CostOptimizer
	transitionEngine  *TransitionEngine
	
	// Scheduling and execution
	scheduler         *cron.Cron
	jobExecutor       *TieringJobExecutor
	
	// Monitoring
	tracer            trace.Tracer
	metrics           *TieringMetrics
	
	// Runtime state
	activePolicies    map[uuid.UUID]*TieringPolicy
	activeJobs        map[uuid.UUID]*TieringJob
	
	// Channels and synchronization
	jobQueue          chan *TieringJob
	stopCh            chan struct{}
	wg                sync.WaitGroup
	mu                sync.RWMutex
}

// Store interfaces
type PolicyStore interface {
	StorePolicy(ctx context.Context, policy *TieringPolicy) error
	GetPolicy(ctx context.Context, policyID uuid.UUID) (*TieringPolicy, error)
	ListPolicies(ctx context.Context, tenantID uuid.UUID) ([]*TieringPolicy, error)
	UpdatePolicy(ctx context.Context, policy *TieringPolicy) error
	DeletePolicy(ctx context.Context, policyID uuid.UUID) error
	GetActivePolicies(ctx context.Context) ([]*TieringPolicy, error)
}

type JobStore interface {
	StoreJob(ctx context.Context, job *TieringJob) error
	GetJob(ctx context.Context, jobID uuid.UUID) (*TieringJob, error)
	UpdateJob(ctx context.Context, job *TieringJob) error
	ListJobs(ctx context.Context, filters TieringJobFilters) ([]*TieringJob, error)
	DeleteJob(ctx context.Context, jobID uuid.UUID) error
}

type MetricsStore interface {
	StoreAccessMetrics(ctx context.Context, metrics *FileAccessMetrics) error
	GetAccessMetrics(ctx context.Context, filePath string) (*FileAccessMetrics, error)
	UpdateAccessMetrics(ctx context.Context, filePath string, accessType string) error
	ListFilesByAccessPattern(ctx context.Context, pattern TierAccessPattern, limit int) ([]*FileAccessMetrics, error)
	GetTieringMetrics(ctx context.Context, tenantID uuid.UUID) (*TieringMetrics, error)
}

type RecommendationStore interface {
	StoreRecommendation(ctx context.Context, rec *TieringRecommendation) error
	GetRecommendation(ctx context.Context, recID uuid.UUID) (*TieringRecommendation, error)
	ListRecommendations(ctx context.Context, filters RecommendationFilters) ([]*TieringRecommendation, error)
	UpdateRecommendation(ctx context.Context, rec *TieringRecommendation) error
	DeleteRecommendation(ctx context.Context, recID uuid.UUID) error
}

// Filter types
type TieringJobFilters struct {
	TenantID   *uuid.UUID   `json:"tenant_id,omitempty"`
	PolicyID   *uuid.UUID   `json:"policy_id,omitempty"`
	Status     *TierStatus  `json:"status,omitempty"`
	FromTier   *StorageTier `json:"from_tier,omitempty"`
	ToTier     *StorageTier `json:"to_tier,omitempty"`
	StartTime  *time.Time   `json:"start_time,omitempty"`
	EndTime    *time.Time   `json:"end_time,omitempty"`
	Limit      int          `json:"limit,omitempty"`
	Offset     int          `json:"offset,omitempty"`
}

type RecommendationFilters struct {
	TenantID        *uuid.UUID   `json:"tenant_id,omitempty"`
	CurrentTier     *StorageTier `json:"current_tier,omitempty"`
	RecommendedTier *StorageTier `json:"recommended_tier,omitempty"`
	MinSavings      *float64     `json:"min_savings,omitempty"`
	Status          *string      `json:"status,omitempty"`
	Limit           int          `json:"limit,omitempty"`
	Offset          int          `json:"offset,omitempty"`
}

// NewTieringManager creates a new tiering manager
func NewTieringManager(
	config *TieringConfig,
	policyStore PolicyStore,
	jobStore JobStore,
	metricsStore MetricsStore,
	recommendationStore RecommendationStore,
	storageManager storage.StorageManager,
) *TieringManager {
	if config == nil {
		config = DefaultTieringConfig()
	}

	tm := &TieringManager{
		config:              config,
		policyStore:         policyStore,
		jobStore:            jobStore,
		metricsStore:        metricsStore,
		recommendationStore: recommendationStore,
		storageManager:      storageManager,
		scheduler:           cron.New(cron.WithSeconds()),
		tracer:              otel.Tracer("tiering-manager"),
		metrics:             &TieringMetrics{TierDistribution: make(map[StorageTier]TierStats)},
		activePolicies:      make(map[uuid.UUID]*TieringPolicy),
		activeJobs:          make(map[uuid.UUID]*TieringJob),
		jobQueue:            make(chan *TieringJob, 100),
		stopCh:              make(chan struct{}),
	}

	// Initialize components
	tm.accessAnalyzer = NewAccessAnalyzer(config, metricsStore)
	tm.costOptimizer = NewCostOptimizer(config, metricsStore)
	tm.transitionEngine = NewTransitionEngine(config, storageManager, tm)
	tm.jobExecutor = NewTieringJobExecutor(config, storageManager, tm)

	return tm
}

// Start starts the tiering manager
func (tm *TieringManager) Start(ctx context.Context) error {
	ctx, span := tm.tracer.Start(ctx, "tiering_manager_start")
	defer span.End()

	// Load active policies
	if err := tm.loadActivePolicies(ctx); err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to load active policies: %w", err)
	}

	// Schedule policy execution and analysis
	if err := tm.schedulePolicies(); err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to schedule policies: %w", err)
	}

	// Schedule access analysis
	if tm.config.AccessTrackingEnabled {
		tm.scheduleAccessAnalysis()
	}

	// Start job executor
	tm.wg.Add(1)
	go tm.jobExecutor.Start(ctx, tm.jobQueue, &tm.wg)

	// Start job processor
	tm.wg.Add(1)
	go tm.processJobs(ctx)

	// Start metrics collection
	tm.wg.Add(1)
	go tm.collectMetrics(ctx)

	// Start scheduler
	tm.scheduler.Start()

	span.SetAttributes(
		attribute.Int("active_policies", len(tm.activePolicies)),
		attribute.Bool("access_tracking", tm.config.AccessTrackingEnabled),
		attribute.Bool("auto_tiering", tm.config.AutoTiering),
	)

	return nil
}

// Stop stops the tiering manager
func (tm *TieringManager) Stop(ctx context.Context) error {
	ctx, span := tm.tracer.Start(ctx, "tiering_manager_stop")
	defer span.End()

	// Stop scheduler
	tm.scheduler.Stop()

	// Signal shutdown
	close(tm.stopCh)

	// Wait for goroutines to finish
	tm.wg.Wait()

	// Close job queue
	close(tm.jobQueue)

	return nil
}

// CreatePolicy creates a new tiering policy
func (tm *TieringManager) CreatePolicy(ctx context.Context, policy *TieringPolicy) error {
	ctx, span := tm.tracer.Start(ctx, "create_tiering_policy")
	defer span.End()

	if policy.ID == uuid.Nil {
		policy.ID = uuid.New()
	}

	// Validate policy
	if err := tm.validatePolicy(policy); err != nil {
		span.RecordError(err)
		return fmt.Errorf("invalid policy: %w", err)
	}

	// Set timestamps
	now := time.Now()
	policy.CreatedAt = now
	policy.UpdatedAt = now

	// Calculate next run time
	if policy.Schedule != "" {
		if nextRun, err := tm.calculateNextRun(policy.Schedule); err == nil {
			policy.NextRun = nextRun
		}
	}

	// Store policy
	if err := tm.policyStore.StorePolicy(ctx, policy); err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to store policy: %w", err)
	}

	// Add to active policies if enabled
	if policy.Enabled {
		tm.mu.Lock()
		tm.activePolicies[policy.ID] = policy
		tm.mu.Unlock()

		// Schedule the policy
		if err := tm.schedulePolicy(policy); err != nil {
			span.RecordError(err)
			// Log error but don't fail policy creation
		}
	}

	span.SetAttributes(
		attribute.String("policy.id", policy.ID.String()),
		attribute.String("policy.name", policy.Name),
		attribute.Bool("policy.enabled", policy.Enabled),
	)

	return nil
}

// ExecutePolicy executes a tiering policy
func (tm *TieringManager) ExecutePolicy(ctx context.Context, policyID uuid.UUID) (*TieringJob, error) {
	ctx, span := tm.tracer.Start(ctx, "execute_tiering_policy")
	defer span.End()

	// Get policy
	policy, err := tm.policyStore.GetPolicy(ctx, policyID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get policy: %w", err)
	}

	if !policy.Enabled {
		return nil, fmt.Errorf("policy %s is disabled", policyID)
	}

	// Create analysis job first
	analysisJob, err := tm.createAnalysisJob(ctx, policy)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to create analysis job: %w", err)
	}

	// Queue job for execution
	select {
	case tm.jobQueue <- analysisJob:
		// Job queued successfully
	default:
		// Queue is full
		analysisJob.Status = StatusFailed
		analysisJob.ErrorMessage = "Job queue is full"
		tm.jobStore.UpdateJob(ctx, analysisJob)
		return nil, fmt.Errorf("job queue is full")
	}

	// Track active job
	tm.mu.Lock()
	tm.activeJobs[analysisJob.ID] = analysisJob
	tm.mu.Unlock()

	// Update metrics
	tm.metrics.TotalJobs++
	tm.metrics.ActiveJobs++

	span.SetAttributes(
		attribute.String("job.id", analysisJob.ID.String()),
		attribute.String("policy.id", policy.ID.String()),
		attribute.String("job.type", analysisJob.Type),
	)

	return analysisJob, nil
}

// AnalyzeAccessPatterns analyzes file access patterns for tiering recommendations
func (tm *TieringManager) AnalyzeAccessPatterns(ctx context.Context, tenantID uuid.UUID) ([]*TieringRecommendation, error) {
	ctx, span := tm.tracer.Start(ctx, "analyze_access_patterns")
	defer span.End()

	recommendations, err := tm.accessAnalyzer.GenerateRecommendations(ctx, tenantID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to generate recommendations: %w", err)
	}

	// Store recommendations
	for _, rec := range recommendations {
		if err := tm.recommendationStore.StoreRecommendation(ctx, rec); err != nil {
			span.RecordError(err)
			// Log error but continue with other recommendations
		}
	}

	span.SetAttributes(
		attribute.String("tenant.id", tenantID.String()),
		attribute.Int("recommendations.generated", len(recommendations)),
	)

	return recommendations, nil
}

// OptimizeCosts performs cost optimization analysis
func (tm *TieringManager) OptimizeCosts(ctx context.Context, tenantID uuid.UUID) (*CostOptimizationResult, error) {
	ctx, span := tm.tracer.Start(ctx, "optimize_costs")
	defer span.End()

	result, err := tm.costOptimizer.OptimizeForTenant(ctx, tenantID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to optimize costs: %w", err)
	}

	span.SetAttributes(
		attribute.String("tenant.id", tenantID.String()),
		attribute.Float64("potential.savings", result.PotentialMonthlySavings),
		attribute.Int("optimization.opportunities", len(result.Recommendations)),
	)

	return result, nil
}

// CreateTransitionJob creates a manual tier transition job
func (tm *TieringManager) CreateTransitionJob(ctx context.Context, req *TransitionRequest) (*TieringJob, error) {
	ctx, span := tm.tracer.Start(ctx, "create_transition_job")
	defer span.End()

	// Validate transition request
	if err := tm.validateTransitionRequest(req); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("invalid transition request: %w", err)
	}

	// Create job
	job := &TieringJob{
		ID:        uuid.New(),
		TenantID:  req.TenantID,
		Type:      "manual_transition",
		FromTier:  req.FromTier,
		ToTier:    req.ToTier,
		Status:    StatusPending,
		CreatedAt: time.Now(),
		CreatedBy: req.CreatedBy,
		Config: TieringJobConfig{
			DryRun:         req.DryRun,
			MaxConcurrency: tm.config.MaxConcurrentJobs,
			BatchSize:      tm.config.BatchSize,
			Timeout:        tm.config.JobTimeout,
			RetryAttempts:  tm.config.RetryAttempts,
			PathFilters:    req.PathFilters,
			FileFilters:    req.FileFilters,
			MaxCost:        req.MaxCost,
			ThrottleLimit:  tm.config.ThrottleLimit,
		},
		Progress: TieringProgress{
			StartTime: time.Now(),
		},
	}

	// Store job
	if err := tm.jobStore.StoreJob(ctx, job); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to store job: %w", err)
	}

	// Queue job for execution
	select {
	case tm.jobQueue <- job:
		// Job queued successfully
	default:
		// Queue is full
		job.Status = StatusFailed
		job.ErrorMessage = "Job queue is full"
		tm.jobStore.UpdateJob(ctx, job)
		return nil, fmt.Errorf("job queue is full")
	}

	// Track active job
	tm.mu.Lock()
	tm.activeJobs[job.ID] = job
	tm.mu.Unlock()

	// Update metrics
	tm.metrics.TotalJobs++
	tm.metrics.ActiveJobs++

	span.SetAttributes(
		attribute.String("job.id", job.ID.String()),
		attribute.String("from.tier", string(req.FromTier)),
		attribute.String("to.tier", string(req.ToTier)),
		attribute.Bool("dry.run", req.DryRun),
	)

	return job, nil
}

// GetTieringMetrics returns comprehensive tiering metrics
func (tm *TieringManager) GetTieringMetrics(ctx context.Context, tenantID uuid.UUID) (*TieringMetrics, error) {
	ctx, span := tm.tracer.Start(ctx, "get_tiering_metrics")
	defer span.End()

	metrics, err := tm.metricsStore.GetTieringMetrics(ctx, tenantID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get tiering metrics: %w", err)
	}

	// Enrich with real-time data
	tm.mu.RLock()
	metrics.ActiveJobs = int64(len(tm.activeJobs))
	tm.mu.RUnlock()

	span.SetAttributes(
		attribute.String("tenant.id", tenantID.String()),
		attribute.Int64("active.jobs", metrics.ActiveJobs),
		attribute.Float64("monthly.savings", metrics.MonthlyCostSavings),
	)

	return metrics, nil
}

// Helper methods

func (tm *TieringManager) loadActivePolicies(ctx context.Context) error {
	policies, err := tm.policyStore.GetActivePolicies(ctx)
	if err != nil {
		return err
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()

	for _, policy := range policies {
		tm.activePolicies[policy.ID] = policy
	}

	return nil
}

func (tm *TieringManager) schedulePolicies() error {
	tm.mu.RLock()
	policies := make([]*TieringPolicy, 0, len(tm.activePolicies))
	for _, policy := range tm.activePolicies {
		policies = append(policies, policy)
	}
	tm.mu.RUnlock()

	for _, policy := range policies {
		if err := tm.schedulePolicy(policy); err != nil {
			// Log error but continue with other policies
			continue
		}
	}

	return nil
}

func (tm *TieringManager) schedulePolicy(policy *TieringPolicy) error {
	if policy.Schedule == "" {
		policy.Schedule = "0 2 * * *" // Default: daily at 2 AM
	}

	_, err := tm.scheduler.AddFunc(policy.Schedule, func() {
		ctx := context.Background()
		_, err := tm.ExecutePolicy(ctx, policy.ID)
		if err != nil {
			// Log error - would use proper logging in production
		}
	})

	return err
}

func (tm *TieringManager) scheduleAccessAnalysis() {
	// Schedule access pattern analysis
	_, err := tm.scheduler.AddFunc("@every 6h", func() {
		ctx := context.Background()
		
		// Get all tenants and analyze their access patterns
		// This would be implemented to iterate through tenants
		tm.runAccessAnalysis(ctx)
	})
	
	if err != nil {
		// Log error
	}
}

func (tm *TieringManager) runAccessAnalysis(ctx context.Context) {
	// This would implement comprehensive access pattern analysis
	// For now, this is a placeholder
}

func (tm *TieringManager) calculateNextRun(schedule string) (time.Time, error) {
	cronSchedule, err := cron.ParseStandard(schedule)
	if err != nil {
		return time.Time{}, err
	}
	return cronSchedule.Next(time.Now()), nil
}

func (tm *TieringManager) validatePolicy(policy *TieringPolicy) error {
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

	// Validate transition rules
	for i, rule := range policy.TransitionRules {
		if err := tm.validateTransitionRule(rule); err != nil {
			return fmt.Errorf("invalid transition rule %d: %w", i, err)
		}
	}

	return nil
}

func (tm *TieringManager) validateTransitionRule(rule TierTransitionRule) error {
	if rule.Name == "" {
		return fmt.Errorf("rule name is required")
	}

	if rule.FromTier == rule.ToTier {
		return fmt.Errorf("from tier and to tier cannot be the same")
	}

	// Validate tier transition is supported
	if !tm.isTransitionSupported(rule.FromTier, rule.ToTier) {
		return fmt.Errorf("transition from %s to %s is not supported", rule.FromTier, rule.ToTier)
	}

	// Validate conditions
	for i, condition := range rule.Conditions {
		if err := tm.validateCondition(condition); err != nil {
			return fmt.Errorf("invalid condition %d: %w", i, err)
		}
	}

	return nil
}

func (tm *TieringManager) validateCondition(condition TieringCondition) error {
	validTypes := []TieringConditionType{
		ConditionAge, ConditionAccessCount, ConditionAccessPattern,
		ConditionFileSize, ConditionFileType, ConditionCostPerGB,
		ConditionStorageUsage, ConditionRetrievalCost, ConditionLastAccessed,
		ConditionDataClassification, ConditionCompliance,
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

func (tm *TieringManager) isTransitionSupported(from, to StorageTier) bool {
	// Define supported transitions
	supportedTransitions := map[StorageTier][]StorageTier{
		TierHot:     {TierWarm, TierCold},
		TierWarm:    {TierHot, TierCold, TierArchive},
		TierCold:    {TierWarm, TierArchive, TierGlacier},
		TierArchive: {TierCold, TierGlacier},
		TierGlacier: {TierArchive},
	}

	validToTiers, exists := supportedTransitions[from]
	if !exists {
		return false
	}

	for _, validTier := range validToTiers {
		if validTier == to {
			return true
		}
	}

	return false
}

func (tm *TieringManager) validateTransitionRequest(req *TransitionRequest) error {
	if req.TenantID == uuid.Nil {
		return fmt.Errorf("tenant ID is required")
	}

	if req.FromTier == req.ToTier {
		return fmt.Errorf("from tier and to tier cannot be the same")
	}

	if !tm.isTransitionSupported(req.FromTier, req.ToTier) {
		return fmt.Errorf("transition from %s to %s is not supported", req.FromTier, req.ToTier)
	}

	return nil
}

func (tm *TieringManager) createAnalysisJob(ctx context.Context, policy *TieringPolicy) (*TieringJob, error) {
	job := &TieringJob{
		ID:        uuid.New(),
		PolicyID:  &policy.ID,
		TenantID:  policy.TenantID,
		Type:      "policy_analysis",
		Status:    StatusPending,
		CreatedAt: time.Now(),
		Config: TieringJobConfig{
			DryRun:         policy.Settings.DryRun,
			MaxConcurrency: tm.config.MaxConcurrentJobs,
			BatchSize:      tm.config.BatchSize,
			Timeout:        tm.config.JobTimeout,
			RetryAttempts:  tm.config.RetryAttempts,
			CostEstimation: true,
			ThrottleLimit:  tm.config.ThrottleLimit,
		},
		Progress: TieringProgress{
			StartTime: time.Now(),
		},
	}

	// Store job
	if err := tm.jobStore.StoreJob(ctx, job); err != nil {
		return nil, fmt.Errorf("failed to store analysis job: %w", err)
	}

	return job, nil
}

func (tm *TieringManager) processJobs(ctx context.Context) {
	defer tm.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-tm.stopCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			tm.updateJobStatuses(ctx)
		}
	}
}

func (tm *TieringManager) updateJobStatuses(ctx context.Context) {
	tm.mu.RLock()
	jobs := make([]*TieringJob, 0, len(tm.activeJobs))
	for _, job := range tm.activeJobs {
		jobs = append(jobs, job)
	}
	tm.mu.RUnlock()

	for _, job := range jobs {
		// Get updated job status
		updatedJob, err := tm.jobStore.GetJob(ctx, job.ID)
		if err != nil {
			continue
		}

		// Update local cache
		tm.mu.Lock()
		tm.activeJobs[job.ID] = updatedJob
		tm.mu.Unlock()

		// Remove completed jobs from active list
		if updatedJob.Status == StatusCompleted || 
		   updatedJob.Status == StatusFailed || 
		   updatedJob.Status == StatusCancelled {
			tm.mu.Lock()
			delete(tm.activeJobs, job.ID)
			tm.mu.Unlock()

			// Update metrics
			tm.metrics.ActiveJobs--
			if updatedJob.Status == StatusCompleted {
				tm.metrics.CompletedJobs++
			} else if updatedJob.Status == StatusFailed {
				tm.metrics.FailedJobs++
			}
		}
	}
}

func (tm *TieringManager) collectMetrics(ctx context.Context) {
	defer tm.wg.Done()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-tm.stopCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			tm.updateMetrics(ctx)
		}
	}
}

func (tm *TieringManager) updateMetrics(ctx context.Context) {
	// Update tier distribution metrics
	// This would query storage systems to get current tier distribution
	// For now, this is a placeholder
}

// TransitionRequest represents a manual tier transition request
type TransitionRequest struct {
	TenantID    uuid.UUID   `json:"tenant_id"`
	FromTier    StorageTier `json:"from_tier"`
	ToTier      StorageTier `json:"to_tier"`
	PathFilters []string    `json:"path_filters,omitempty"`
	FileFilters []string    `json:"file_filters,omitempty"`
	MaxCost     float64     `json:"max_cost,omitempty"`
	DryRun      bool        `json:"dry_run"`
	CreatedBy   string      `json:"created_by"`
}

// CostOptimizationResult contains the results of cost optimization analysis
type CostOptimizationResult struct {
	TenantID                uuid.UUID                  `json:"tenant_id"`
	AnalysisDate            time.Time                  `json:"analysis_date"`
	CurrentMonthlyCost      float64                    `json:"current_monthly_cost"`
	OptimizedMonthlyCost    float64                    `json:"optimized_monthly_cost"`
	PotentialMonthlySavings float64                    `json:"potential_monthly_savings"`
	PotentialAnnualSavings  float64                    `json:"potential_annual_savings"`
	Recommendations         []*TieringRecommendation   `json:"recommendations"`
	TierDistribution        map[StorageTier]TierStats  `json:"tier_distribution"`
	OptimizationScore       float64                    `json:"optimization_score"` // 0-100
}