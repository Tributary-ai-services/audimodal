package tiering

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/jscharber/eAIIngest/pkg/storage"
)

// TransitionEngine handles the actual execution of tier transitions
type TransitionEngine struct {
	config         *TieringConfig
	storageManager storage.StorageManager
	tieringManager *TieringManager
	tracer         trace.Tracer
	metrics        *TransitionMetrics

	// Worker pool for concurrent transitions
	workers    []TransitionWorker
	workerPool chan TransitionWorker

	// Rate limiting
	rateLimiter *RateLimiter

	// Concurrent transition tracking
	activeTransitions map[string]*TransitionContext
	mu                sync.RWMutex
}

// TransitionWorker handles individual tier transitions
type TransitionWorker interface {
	Execute(ctx context.Context, transition *TierTransition) (*TierTransitionResult, error)
	GetID() string
	IsAvailable() bool
}

// TransitionContext tracks an ongoing transition
type TransitionContext struct {
	TransitionID string
	JobID        uuid.UUID
	StartTime    time.Time
	Files        []string
	Progress     *TransitionProgress
	Worker       TransitionWorker
}

// TransitionProgress tracks progress of a transition
type TransitionProgress struct {
	TotalFiles      int64      `json:"total_files"`
	ProcessedFiles  int64      `json:"processed_files"`
	SuccessfulFiles int64      `json:"successful_files"`
	FailedFiles     int64      `json:"failed_files"`
	TotalBytes      int64      `json:"total_bytes"`
	ProcessedBytes  int64      `json:"processed_bytes"`
	LastUpdate      time.Time  `json:"last_update"`
	EstimatedETA    *time.Time `json:"estimated_eta,omitempty"`
}

// TransitionMetrics tracks transition engine performance
type TransitionMetrics struct {
	ActiveTransitions     int64         `json:"active_transitions"`
	CompletedTransitions  int64         `json:"completed_transitions"`
	FailedTransitions     int64         `json:"failed_transitions"`
	AverageTransitionTime time.Duration `json:"average_transition_time"`
	ThroughputMBps        float64       `json:"throughput_mbps"`
	ErrorRate             float64       `json:"error_rate"`
	LastReset             time.Time     `json:"last_reset"`
}

// TierTransition represents a single tier transition operation
type TierTransition struct {
	ID            string            `json:"id"`
	JobID         uuid.UUID         `json:"job_id"`
	FilePath      string            `json:"file_path"`
	FileSize      int64             `json:"file_size"`
	FromTier      StorageTier       `json:"from_tier"`
	ToTier        StorageTier       `json:"to_tier"`
	Priority      int               `json:"priority"`
	EstimatedCost float64           `json:"estimated_cost"`
	MaxRetries    int               `json:"max_retries"`
	RetryCount    int               `json:"retry_count"`
	CreatedAt     time.Time         `json:"created_at"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// RateLimiter controls the rate of transitions
type RateLimiter struct {
	limit      float64
	tokens     float64
	lastUpdate time.Time
	mu         sync.Mutex
}

// NewTransitionEngine creates a new transition engine
func NewTransitionEngine(config *TieringConfig, storageManager storage.StorageManager, tieringManager *TieringManager) *TransitionEngine {
	engine := &TransitionEngine{
		config:            config,
		storageManager:    storageManager,
		tieringManager:    tieringManager,
		tracer:            otel.Tracer("transition-engine"),
		metrics:           &TransitionMetrics{LastReset: time.Now()},
		workerPool:        make(chan TransitionWorker, config.MaxConcurrentJobs),
		activeTransitions: make(map[string]*TransitionContext),
		rateLimiter:       NewRateLimiter(config.ThrottleLimit),
	}

	// Initialize workers
	for i := 0; i < config.MaxConcurrentJobs; i++ {
		worker := NewDefaultTransitionWorker(fmt.Sprintf("worker-%d", i), storageManager, config)
		engine.workers = append(engine.workers, worker)
		engine.workerPool <- worker
	}

	return engine
}

// ExecuteTransitions executes multiple tier transitions
func (te *TransitionEngine) ExecuteTransitions(ctx context.Context, transitions []*TierTransition) ([]*TierTransitionResult, error) {
	ctx, span := te.tracer.Start(ctx, "execute_transitions")
	defer span.End()

	if len(transitions) == 0 {
		return nil, nil
	}

	span.SetAttributes(
		attribute.Int("transitions.count", len(transitions)),
	)

	// Sort transitions by priority
	te.sortTransitionsByPriority(transitions)

	var results []*TierTransitionResult
	var resultsMu sync.Mutex
	var wg sync.WaitGroup

	// Execute transitions concurrently
	for _, transition := range transitions {
		// Rate limiting
		if !te.rateLimiter.Allow() {
			time.Sleep(time.Millisecond * 100)
		}

		wg.Add(1)
		go func(t *TierTransition) {
			defer wg.Done()

			result, err := te.executeTransition(ctx, t)
			if err != nil {
				span.RecordError(err)
				result = &TierTransitionResult{
					FilePath:    t.FilePath,
					FromTier:    t.FromTier,
					ToTier:      t.ToTier,
					FileSize:    t.FileSize,
					CompletedAt: time.Now(),
					Metadata: map[string]string{
						"error": err.Error(),
					},
				}
			}

			resultsMu.Lock()
			results = append(results, result)
			resultsMu.Unlock()
		}(transition)
	}

	wg.Wait()

	// Update metrics
	te.updateMetrics(results)

	span.SetAttributes(
		attribute.Int("transitions.completed", len(results)),
	)

	return results, nil
}

// executeTransition executes a single tier transition
func (te *TransitionEngine) executeTransition(ctx context.Context, transition *TierTransition) (*TierTransitionResult, error) {
	ctx, span := te.tracer.Start(ctx, "execute_single_transition")
	defer span.End()

	span.SetAttributes(
		attribute.String("file.path", transition.FilePath),
		attribute.String("from.tier", string(transition.FromTier)),
		attribute.String("to.tier", string(transition.ToTier)),
		attribute.Int64("file.size", transition.FileSize),
	)

	// Get a worker
	worker := <-te.workerPool
	defer func() {
		te.workerPool <- worker
	}()

	// Track transition
	transitionCtx := &TransitionContext{
		TransitionID: transition.ID,
		JobID:        transition.JobID,
		StartTime:    time.Now(),
		Files:        []string{transition.FilePath},
		Worker:       worker,
		Progress: &TransitionProgress{
			TotalFiles: 1,
			TotalBytes: transition.FileSize,
			LastUpdate: time.Now(),
		},
	}

	te.mu.Lock()
	te.activeTransitions[transition.ID] = transitionCtx
	te.mu.Unlock()

	defer func() {
		te.mu.Lock()
		delete(te.activeTransitions, transition.ID)
		te.mu.Unlock()
	}()

	// Execute transition with retries
	var result *TierTransitionResult
	var err error

	for attempt := 0; attempt <= transition.MaxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			backoff := time.Duration(math.Pow(2, float64(attempt))) * time.Second
			time.Sleep(backoff)
		}

		result, err = worker.Execute(ctx, transition)
		if err == nil {
			break
		}

		transition.RetryCount = attempt + 1

		// Check if error is retryable
		if !te.isRetryableError(err) {
			break
		}
	}

	// Update progress
	transitionCtx.Progress.ProcessedFiles = 1
	transitionCtx.Progress.ProcessedBytes = transition.FileSize
	transitionCtx.Progress.LastUpdate = time.Now()

	if err == nil {
		transitionCtx.Progress.SuccessfulFiles = 1
		te.metrics.CompletedTransitions++
	} else {
		transitionCtx.Progress.FailedFiles = 1
		te.metrics.FailedTransitions++
	}

	span.SetAttributes(
		attribute.Bool("transition.success", err == nil),
		attribute.Int("retry.count", transition.RetryCount),
	)

	if err != nil {
		span.RecordError(err)
		return result, err
	}

	return result, nil
}

// ValidateTransition validates that a transition is supported and safe
func (te *TransitionEngine) ValidateTransition(ctx context.Context, fromTier, toTier StorageTier) error {
	ctx, span := te.tracer.Start(ctx, "validate_transition")
	defer span.End()

	span.SetAttributes(
		attribute.String("from.tier", string(fromTier)),
		attribute.String("to.tier", string(toTier)),
	)

	// Check if transition is supported
	if !te.isTransitionSupported(fromTier, toTier) {
		err := fmt.Errorf("transition from %s to %s is not supported", fromTier, toTier)
		span.RecordError(err)
		return err
	}

	// Check minimum retention periods
	if minRetention, exists := te.config.MinRetentionPeriods[fromTier]; exists {
		if minRetention > 0 {
			// This would need file creation time to validate properly
			// For now, we'll assume it's valid
		}
	}

	return nil
}

// EstimateTransitionCost estimates the cost of a tier transition
func (te *TransitionEngine) EstimateTransitionCost(ctx context.Context, fileSizeBytes int64, fromTier, toTier StorageTier) (*TransitionCostEstimate, error) {
	ctx, span := te.tracer.Start(ctx, "estimate_transition_cost")
	defer span.End()

	fileSizeGB := float64(fileSizeBytes) / (1024 * 1024 * 1024)

	// Get transition cost per GB
	transitionCostPerGB := te.getTransitionCostPerGB(fromTier, toTier)
	transitionCost := fileSizeGB * transitionCostPerGB

	// Get storage costs
	currentStorageCost := te.getStorageCostPerGB(fromTier)
	targetStorageCost := te.getStorageCostPerGB(toTier)

	// Calculate monthly savings
	monthlySavings := fileSizeGB * (currentStorageCost - targetStorageCost)

	// Calculate payback period
	var paybackMonths int
	if monthlySavings > 0 {
		paybackMonths = int(math.Ceil(transitionCost / monthlySavings))
	} else {
		paybackMonths = -1 // Never pays back
	}

	estimate := &TransitionCostEstimate{
		TransitionCost:     transitionCost,
		MonthlySavings:     monthlySavings,
		AnnualSavings:      monthlySavings * 12,
		PaybackMonths:      paybackMonths,
		CurrentStorageCost: fileSizeGB * currentStorageCost,
		TargetStorageCost:  fileSizeGB * targetStorageCost,
		EstimatedAt:        time.Now(),
	}

	span.SetAttributes(
		attribute.Float64("transition.cost", transitionCost),
		attribute.Float64("monthly.savings", monthlySavings),
		attribute.Int("payback.months", paybackMonths),
	)

	return estimate, nil
}

// GetActiveTransitions returns currently active transitions
func (te *TransitionEngine) GetActiveTransitions(ctx context.Context) []*TransitionContext {
	te.mu.RLock()
	defer te.mu.RUnlock()

	contexts := make([]*TransitionContext, 0, len(te.activeTransitions))
	for _, ctx := range te.activeTransitions {
		contexts = append(contexts, ctx)
	}

	return contexts
}

// GetMetrics returns transition engine metrics
func (te *TransitionEngine) GetMetrics() *TransitionMetrics {
	te.mu.RLock()
	defer te.mu.RUnlock()

	metrics := *te.metrics
	metrics.ActiveTransitions = int64(len(te.activeTransitions))
	return &metrics
}

// Helper methods

func (te *TransitionEngine) isTransitionSupported(from, to StorageTier) bool {
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

func (te *TransitionEngine) isRetryableError(err error) bool {
	// Check for retryable error patterns
	errorMsg := err.Error()
	retryablePatterns := []string{
		"timeout",
		"connection",
		"network",
		"temporary",
		"rate limit",
		"503",
		"502",
		"500",
	}

	for _, pattern := range retryablePatterns {
		if containsString(errorMsg, pattern) {
			return true
		}
	}

	return false
}

func (te *TransitionEngine) getTransitionCostPerGB(from, to StorageTier) float64 {
	// Default transition costs (would be configurable per provider)
	transitionCosts := map[string]float64{
		string(TierHot) + "->" + string(TierWarm):        0.0,
		string(TierHot) + "->" + string(TierCold):        0.01,
		string(TierWarm) + "->" + string(TierHot):        0.01,
		string(TierWarm) + "->" + string(TierCold):       0.0,
		string(TierWarm) + "->" + string(TierArchive):    0.05,
		string(TierCold) + "->" + string(TierWarm):       0.02,
		string(TierCold) + "->" + string(TierArchive):    0.02,
		string(TierCold) + "->" + string(TierGlacier):    0.05,
		string(TierArchive) + "->" + string(TierCold):    0.03,
		string(TierArchive) + "->" + string(TierGlacier): 0.03,
		string(TierGlacier) + "->" + string(TierArchive): 0.05,
	}

	key := string(from) + "->" + string(to)
	if cost, exists := transitionCosts[key]; exists {
		return cost
	}

	return 0.01 // Default cost
}

func (te *TransitionEngine) getStorageCostPerGB(tier StorageTier) float64 {
	if cost, exists := te.config.CostThresholds[tier]; exists {
		return cost
	}

	// Default costs
	switch tier {
	case TierHot:
		return 0.023
	case TierWarm:
		return 0.0125
	case TierCold:
		return 0.004
	case TierArchive:
		return 0.00099
	case TierGlacier:
		return 0.0004
	default:
		return 0.023
	}
}

func (te *TransitionEngine) sortTransitionsByPriority(transitions []*TierTransition) {
	// Sort by priority (higher priority first), then by estimated cost savings
	for i := 0; i < len(transitions)-1; i++ {
		for j := i + 1; j < len(transitions); j++ {
			if transitions[i].Priority < transitions[j].Priority {
				transitions[i], transitions[j] = transitions[j], transitions[i]
			}
		}
	}
}

func (te *TransitionEngine) updateMetrics(results []*TierTransitionResult) {
	te.mu.Lock()
	defer te.mu.Unlock()

	var totalDuration time.Duration
	var totalBytes int64
	var successCount int64

	for _, result := range results {
		if result.Success {
			successCount++
			totalBytes += result.FileSize
			if !result.CompletedAt.IsZero() && !result.StartedAt.IsZero() {
				totalDuration += result.CompletedAt.Sub(result.StartedAt)
			}
		}
	}

	if len(results) > 0 {
		te.metrics.AverageTransitionTime = totalDuration / time.Duration(len(results))

		if totalDuration > 0 {
			mbps := float64(totalBytes) / (1024 * 1024) / totalDuration.Seconds()
			te.metrics.ThroughputMBps = mbps
		}

		te.metrics.ErrorRate = float64(len(results)-int(successCount)) / float64(len(results))
	}
}

// TransitionCostEstimate represents cost estimation for a transition
type TransitionCostEstimate struct {
	TransitionCost     float64   `json:"transition_cost"`
	MonthlySavings     float64   `json:"monthly_savings"`
	AnnualSavings      float64   `json:"annual_savings"`
	PaybackMonths      int       `json:"payback_months"`
	CurrentStorageCost float64   `json:"current_storage_cost"`
	TargetStorageCost  float64   `json:"target_storage_cost"`
	EstimatedAt        time.Time `json:"estimated_at"`
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(limit float64) *RateLimiter {
	return &RateLimiter{
		limit:      limit,
		tokens:     limit,
		lastUpdate: time.Now(),
	}
}

// Allow checks if an operation is allowed based on rate limiting
func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastUpdate).Seconds()

	// Add tokens based on elapsed time
	rl.tokens = math.Min(rl.limit, rl.tokens+elapsed*rl.limit)
	rl.lastUpdate = now

	if rl.tokens >= 1.0 {
		rl.tokens--
		return true
	}

	return false
}

// DefaultTransitionWorker implements TransitionWorker interface
type DefaultTransitionWorker struct {
	id             string
	storageManager storage.StorageManager
	config         *TieringConfig
	available      bool
	tracer         trace.Tracer
}

// NewDefaultTransitionWorker creates a new default transition worker
func NewDefaultTransitionWorker(id string, storageManager storage.StorageManager, config *TieringConfig) *DefaultTransitionWorker {
	return &DefaultTransitionWorker{
		id:             id,
		storageManager: storageManager,
		config:         config,
		available:      true,
		tracer:         otel.Tracer("transition-worker"),
	}
}

// Execute executes a tier transition
func (w *DefaultTransitionWorker) Execute(ctx context.Context, transition *TierTransition) (*TierTransitionResult, error) {
	ctx, span := w.tracer.Start(ctx, "worker_execute_transition")
	defer span.End()

	w.available = false
	defer func() { w.available = true }()

	startTime := time.Now()

	span.SetAttributes(
		attribute.String("worker.id", w.id),
		attribute.String("file.path", transition.FilePath),
		attribute.String("from.tier", string(transition.FromTier)),
		attribute.String("to.tier", string(transition.ToTier)),
		attribute.Int64("file.size", transition.FileSize),
	)

	// For now, this is a placeholder implementation
	// In a real implementation, this would:
	// 1. Validate the file exists and is in the expected tier
	// 2. Initiate the storage class transition via the storage manager
	// 3. Monitor the transition progress
	// 4. Verify the transition completed successfully

	// Simulate transition time based on file size
	simulatedDuration := time.Duration(float64(transition.FileSize)/1024/1024/10) * time.Second
	if simulatedDuration > 30*time.Second {
		simulatedDuration = 30 * time.Second
	}
	if simulatedDuration < time.Second {
		simulatedDuration = time.Second
	}

	// Simulate the transition
	time.Sleep(simulatedDuration)

	result := &TierTransitionResult{
		FilePath:       transition.FilePath,
		FromTier:       transition.FromTier,
		ToTier:         transition.ToTier,
		FileSize:       transition.FileSize,
		Success:        true,
		TransitionCost: transition.EstimatedCost,
		StartedAt:      startTime,
		CompletedAt:    time.Now(),
		Metadata: map[string]string{
			"worker_id":     w.id,
			"transition_id": transition.ID,
			"retry_count":   fmt.Sprintf("%d", transition.RetryCount),
		},
	}

	// Calculate savings (simplified)
	currentCost := w.getStorageCostPerGB(transition.FromTier)
	targetCost := w.getStorageCostPerGB(transition.ToTier)
	fileSizeGB := float64(transition.FileSize) / (1024 * 1024 * 1024)
	result.MonthlySavings = fileSizeGB * (currentCost - targetCost)

	span.SetAttributes(
		attribute.Bool("transition.success", result.Success),
		attribute.Float64("monthly.savings", result.MonthlySavings),
		attribute.Float64("duration.seconds", result.CompletedAt.Sub(result.StartedAt).Seconds()),
	)

	return result, nil
}

// GetID returns the worker ID
func (w *DefaultTransitionWorker) GetID() string {
	return w.id
}

// IsAvailable returns whether the worker is available
func (w *DefaultTransitionWorker) IsAvailable() bool {
	return w.available
}

func (w *DefaultTransitionWorker) getStorageCostPerGB(tier StorageTier) float64 {
	switch tier {
	case TierHot:
		return 0.023
	case TierWarm:
		return 0.0125
	case TierCold:
		return 0.004
	case TierArchive:
		return 0.00099
	case TierGlacier:
		return 0.0004
	default:
		return 0.023
	}
}

// Helper function
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr) != -1
}

func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
