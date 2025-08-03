package sync

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// CronScheduler handles cron-based scheduling
type CronScheduler struct {
	entries map[uuid.UUID]*CronEntry
	mutex   sync.RWMutex
	ctx     context.Context
	cancel  context.CancelFunc
}

// CronEntry represents a scheduled cron job
type CronEntry struct {
	ID       uuid.UUID
	Schedule string
	NextRun  time.Time
	Callback func() error
}

// NewCronScheduler creates a new cron scheduler
func NewCronScheduler() *CronScheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &CronScheduler{
		entries: make(map[uuid.UUID]*CronEntry),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// AddEntry adds a new cron entry
func (c *CronScheduler) AddEntry(id uuid.UUID, schedule string, callback func() error) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	nextRun := time.Now().Add(time.Hour) // Simple implementation - would use real cron parsing

	c.entries[id] = &CronEntry{
		ID:       id,
		Schedule: schedule,
		NextRun:  nextRun,
		Callback: callback,
	}

	return nil
}

// RemoveEntry removes a cron entry
func (c *CronScheduler) RemoveEntry(id uuid.UUID) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	delete(c.entries, id)
}

// Start starts the cron scheduler
func (c *CronScheduler) Start() {
	go c.run()
}

// Stop stops the cron scheduler
func (c *CronScheduler) Stop() {
	c.cancel()
}

// run is the main scheduler loop
func (c *CronScheduler) run() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.checkAndRunEntries()
		}
	}
}

// checkAndRunEntries checks for entries that need to run
func (c *CronScheduler) checkAndRunEntries() {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	now := time.Now()
	for _, entry := range c.entries {
		if now.After(entry.NextRun) {
			go entry.Callback()
			// Update next run time (simplified)
			entry.NextRun = now.Add(time.Hour)
		}
	}
}

// SyncScheduler manages scheduled sync operations and throttling
type SyncScheduler struct {
	config     *SchedulerConfig
	schedules  map[uuid.UUID]*SyncSchedule
	throttlers map[string]*ThrottleManager
	scheduler  *CronScheduler
	mutex      sync.RWMutex
	tracer     trace.Tracer

	// Background processing
	ticker   *time.Ticker
	stopChan chan struct{}

	// Dependencies
	orchestrator *SyncOrchestrator
}

// SchedulerConfig contains scheduler configuration
type SchedulerConfig struct {
	EnableScheduling       bool          `json:"enable_scheduling"`
	MaxConcurrentSchedules int           `json:"max_concurrent_schedules"`
	ScheduleCheckInterval  time.Duration `json:"schedule_check_interval"`
	DefaultTimeout         time.Duration `json:"default_timeout"`
	RetryFailedSchedules   bool          `json:"retry_failed_schedules"`
	MaxRetries             int           `json:"max_retries"`
	RetryBackoffMultiplier float64       `json:"retry_backoff_multiplier"`
	SchedulePersistence    bool          `json:"schedule_persistence"`

	// Throttling configuration
	EnableThrottling     bool                        `json:"enable_throttling"`
	GlobalRateLimit      *RateLimitConfig            `json:"global_rate_limit"`
	ConnectorRateLimits  map[string]*RateLimitConfig `json:"connector_rate_limits"`
	ThrottleRecoveryTime time.Duration               `json:"throttle_recovery_time"`
	AdaptiveThrottling   bool                        `json:"adaptive_throttling"`

	// Resource management
	ResourceLimits           *ResourceLimitConfig `json:"resource_limits"`
	EnableResourceMonitoring bool                 `json:"enable_resource_monitoring"`
	ResourceCheckInterval    time.Duration        `json:"resource_check_interval"`
}

// SyncSchedule represents a scheduled sync operation
type SyncSchedule struct {
	ID                 uuid.UUID           `json:"id"`
	Name               string              `json:"name"`
	Description        string              `json:"description"`
	DataSourceID       uuid.UUID           `json:"data_source_id"`
	ConnectorType      string              `json:"connector_type"`
	SyncOptions        *UnifiedSyncOptions `json:"sync_options"`
	Schedule           *ScheduleConfig     `json:"schedule"`
	IsActive           bool                `json:"is_active"`
	CreatedAt          time.Time           `json:"created_at"`
	UpdatedAt          time.Time           `json:"updated_at"`
	LastRun            *time.Time          `json:"last_run,omitempty"`
	NextRun            *time.Time          `json:"next_run,omitempty"`
	RunCount           int64               `json:"run_count"`
	SuccessCount       int64               `json:"success_count"`
	FailureCount       int64               `json:"failure_count"`
	LastResult         *ScheduleResult     `json:"last_result,omitempty"`
	RetryCount         int                 `json:"retry_count"`
	MaxRetries         int                 `json:"max_retries"`
	Timezone           string              `json:"timezone"`
	Tags               map[string]string   `json:"tags"`
	NotificationConfig *NotificationConfig `json:"notification_config,omitempty"`
}

// ScheduleConfig defines when and how often a sync should run
type ScheduleConfig struct {
	Type            ScheduleType        `json:"type"`
	CronExpression  string              `json:"cron_expression,omitempty"`
	Interval        time.Duration       `json:"interval,omitempty"`
	RunOnce         *time.Time          `json:"run_once,omitempty"`
	StartDate       *time.Time          `json:"start_date,omitempty"`
	EndDate         *time.Time          `json:"end_date,omitempty"`
	MaxRuns         *int64              `json:"max_runs,omitempty"`
	WeeklySchedule  *WeeklySchedule     `json:"weekly_schedule,omitempty"`
	MonthlySchedule *MonthlySchedule    `json:"monthly_schedule,omitempty"`
	Conditions      []ScheduleCondition `json:"conditions,omitempty"`
}

// ScheduleType defines the type of schedule
type ScheduleType string

const (
	ScheduleTypeOnce        ScheduleType = "once"
	ScheduleTypeInterval    ScheduleType = "interval"
	ScheduleTypeCron        ScheduleType = "cron"
	ScheduleTypeWeekly      ScheduleType = "weekly"
	ScheduleTypeMonthly     ScheduleType = "monthly"
	ScheduleTypeConditional ScheduleType = "conditional"
)

// WeeklySchedule defines a weekly schedule
type WeeklySchedule struct {
	DaysOfWeek []time.Weekday `json:"days_of_week"`
	TimeOfDay  string         `json:"time_of_day"` // HH:MM format
}

// MonthlySchedule defines a monthly schedule
type MonthlySchedule struct {
	DayOfMonth int    `json:"day_of_month"` // 1-31, or -1 for last day
	TimeOfDay  string `json:"time_of_day"`  // HH:MM format
}

// ScheduleCondition defines conditions that must be met for a schedule to run
type ScheduleCondition struct {
	Type     ConditionType          `json:"type"`
	Operator ConditionOperator      `json:"operator"`
	Value    interface{}            `json:"value"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// ConditionType defines the type of condition
type ConditionType string

const (
	ConditionTypeFileCount      ConditionType = "file_count"
	ConditionTypeLastSyncAge    ConditionType = "last_sync_age"
	ConditionTypeDataSourceSize ConditionType = "data_source_size"
	ConditionTypeErrorRate      ConditionType = "error_rate"
	ConditionTypeResourceUsage  ConditionType = "resource_usage"
)

// ConditionOperator defines comparison operators for conditions
type ConditionOperator string

const (
	ConditionOperatorEqual            ConditionOperator = "eq"
	ConditionOperatorNotEqual         ConditionOperator = "ne"
	ConditionOperatorGreaterThan      ConditionOperator = "gt"
	ConditionOperatorGreaterThanEqual ConditionOperator = "gte"
	ConditionOperatorLessThan         ConditionOperator = "lt"
	ConditionOperatorLessThanEqual    ConditionOperator = "lte"
)

// NotificationConfig defines how to notify about schedule results
type NotificationConfig struct {
	OnSuccess     bool     `json:"on_success"`
	OnFailure     bool     `json:"on_failure"`
	OnSkip        bool     `json:"on_skip"`
	Channels      []string `json:"channels"` // email, slack, webhook
	Recipients    []string `json:"recipients"`
	WebhookURL    string   `json:"webhook_url,omitempty"`
	EmailTemplate string   `json:"email_template,omitempty"`
}

// ScheduleResult represents the result of a scheduled sync execution
type ScheduleResult struct {
	ScheduleID   uuid.UUID      `json:"schedule_id"`
	JobID        uuid.UUID      `json:"job_id"`
	StartTime    time.Time      `json:"start_time"`
	EndTime      time.Time      `json:"end_time"`
	Duration     time.Duration  `json:"duration"`
	Status       ScheduleStatus `json:"status"`
	Message      string         `json:"message,omitempty"`
	Error        string         `json:"error,omitempty"`
	Metrics      *SyncMetrics   `json:"metrics,omitempty"`
	NextSchedule *time.Time     `json:"next_schedule,omitempty"`
}

// ScheduleStatus represents the status of a schedule execution
type ScheduleStatus string

const (
	ScheduleStatusPending   ScheduleStatus = "pending"
	ScheduleStatusRunning   ScheduleStatus = "running"
	ScheduleStatusCompleted ScheduleStatus = "completed"
	ScheduleStatusFailed    ScheduleStatus = "failed"
	ScheduleStatusSkipped   ScheduleStatus = "skipped"
	ScheduleStatusCancelled ScheduleStatus = "cancelled"
)

// RateLimitConfig defines rate limiting configuration
type RateLimitConfig struct {
	RequestsPerSecond float64       `json:"requests_per_second"`
	BurstLimit        int           `json:"burst_limit"`
	WindowSize        time.Duration `json:"window_size"`
	EnableAdaptive    bool          `json:"enable_adaptive"`
	BackoffMultiplier float64       `json:"backoff_multiplier"`
	MaxBackoffTime    time.Duration `json:"max_backoff_time"`
}

// ResourceLimitConfig defines resource limits for sync operations
type ResourceLimitConfig struct {
	MaxMemoryMB        int64   `json:"max_memory_mb"`
	MaxCPUPercent      float64 `json:"max_cpu_percent"`
	MaxBandwidthMBps   int64   `json:"max_bandwidth_mbps"`
	MaxConcurrentSyncs int     `json:"max_concurrent_syncs"`
	MaxDiskSpaceMB     int64   `json:"max_disk_space_mb"`
}

// ThrottleManager manages throttling for a specific resource or connector
type ThrottleManager struct {
	config       *RateLimitConfig
	tokens       chan struct{}
	lastRequest  time.Time
	backoffUntil time.Time
	requestCount int64
	windowStart  time.Time
	mutex        sync.Mutex
}

// NewSyncScheduler creates a new sync scheduler
func NewSyncScheduler(orchestrator *SyncOrchestrator, config *SchedulerConfig) *SyncScheduler {
	if config == nil {
		config = &SchedulerConfig{
			EnableScheduling:       true,
			MaxConcurrentSchedules: 10,
			ScheduleCheckInterval:  1 * time.Minute,
			DefaultTimeout:         2 * time.Hour,
			RetryFailedSchedules:   true,
			MaxRetries:             3,
			RetryBackoffMultiplier: 2.0,
			SchedulePersistence:    true,
			EnableThrottling:       true,
			ThrottleRecoveryTime:   5 * time.Minute,
			AdaptiveThrottling:     true,
			ResourceCheckInterval:  30 * time.Second,
		}
	}

	scheduler := &SyncScheduler{
		config:       config,
		schedules:    make(map[uuid.UUID]*SyncSchedule),
		throttlers:   make(map[string]*ThrottleManager),
		stopChan:     make(chan struct{}),
		tracer:       otel.Tracer("sync-scheduler"),
		orchestrator: orchestrator,
	}

	if config.EnableScheduling {
		scheduler.ticker = time.NewTicker(config.ScheduleCheckInterval)
		scheduler.startBackgroundProcessing()
	}

	// Initialize throttlers
	if config.EnableThrottling {
		scheduler.initializeThrottlers()
	}

	return scheduler
}

// CreateSchedule creates a new sync schedule
func (ss *SyncScheduler) CreateSchedule(ctx context.Context, schedule *SyncSchedule) error {
	ctx, span := ss.tracer.Start(ctx, "sync_scheduler.create_schedule")
	defer span.End()

	span.SetAttributes(
		attribute.String("schedule.name", schedule.Name),
		attribute.String("data_source_id", schedule.DataSourceID.String()),
		attribute.String("connector_type", schedule.ConnectorType),
	)

	ss.mutex.Lock()
	defer ss.mutex.Unlock()

	if schedule.ID == uuid.Nil {
		schedule.ID = uuid.New()
	}

	// Set default values
	now := time.Now()
	schedule.CreatedAt = now
	schedule.UpdatedAt = now
	schedule.IsActive = true

	if schedule.MaxRetries == 0 {
		schedule.MaxRetries = ss.config.MaxRetries
	}

	// Calculate next run time
	nextRun, err := ss.calculateNextRun(schedule)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to calculate next run: %w", err)
	}
	schedule.NextRun = nextRun

	ss.schedules[schedule.ID] = schedule

	span.SetAttributes(
		attribute.String("schedule.id", schedule.ID.String()),
		attribute.String("next_run", nextRun.Format(time.RFC3339)),
	)

	return nil
}

// UpdateSchedule updates an existing sync schedule
func (ss *SyncScheduler) UpdateSchedule(ctx context.Context, scheduleID uuid.UUID, updates *SyncSchedule) error {
	ctx, span := ss.tracer.Start(ctx, "sync_scheduler.update_schedule")
	defer span.End()

	span.SetAttributes(attribute.String("schedule.id", scheduleID.String()))

	ss.mutex.Lock()
	defer ss.mutex.Unlock()

	existing, exists := ss.schedules[scheduleID]
	if !exists {
		return fmt.Errorf("schedule not found")
	}

	// Update fields
	if updates.Name != "" {
		existing.Name = updates.Name
	}
	if updates.Description != "" {
		existing.Description = updates.Description
	}
	if updates.SyncOptions != nil {
		existing.SyncOptions = updates.SyncOptions
	}
	if updates.Schedule != nil {
		existing.Schedule = updates.Schedule
		// Recalculate next run time
		nextRun, err := ss.calculateNextRun(existing)
		if err != nil {
			span.RecordError(err)
			return fmt.Errorf("failed to recalculate next run: %w", err)
		}
		existing.NextRun = nextRun
	}
	if updates.NotificationConfig != nil {
		existing.NotificationConfig = updates.NotificationConfig
	}

	existing.UpdatedAt = time.Now()

	return nil
}

// DeleteSchedule removes a sync schedule
func (ss *SyncScheduler) DeleteSchedule(ctx context.Context, scheduleID uuid.UUID) error {
	ctx, span := ss.tracer.Start(ctx, "sync_scheduler.delete_schedule")
	defer span.End()

	span.SetAttributes(attribute.String("schedule.id", scheduleID.String()))

	ss.mutex.Lock()
	defer ss.mutex.Unlock()

	delete(ss.schedules, scheduleID)

	return nil
}

// GetSchedule retrieves a sync schedule
func (ss *SyncScheduler) GetSchedule(ctx context.Context, scheduleID uuid.UUID) (*SyncSchedule, error) {
	ctx, span := ss.tracer.Start(ctx, "sync_scheduler.get_schedule")
	defer span.End()

	span.SetAttributes(attribute.String("schedule.id", scheduleID.String()))

	ss.mutex.RLock()
	defer ss.mutex.RUnlock()

	schedule, exists := ss.schedules[scheduleID]
	if !exists {
		return nil, fmt.Errorf("schedule not found")
	}

	return schedule, nil
}

// ListSchedules returns all schedules with optional filtering
func (ss *SyncScheduler) ListSchedules(ctx context.Context, filters *ScheduleFilters) ([]*SyncSchedule, error) {
	ctx, span := ss.tracer.Start(ctx, "sync_scheduler.list_schedules")
	defer span.End()

	ss.mutex.RLock()
	defer ss.mutex.RUnlock()

	var schedules []*SyncSchedule
	for _, schedule := range ss.schedules {
		// Apply filters
		if filters != nil {
			if filters.IsActive != nil && schedule.IsActive != *filters.IsActive {
				continue
			}
			if filters.ConnectorType != "" && schedule.ConnectorType != filters.ConnectorType {
				continue
			}
			if filters.DataSourceID != uuid.Nil && schedule.DataSourceID != filters.DataSourceID {
				continue
			}
		}

		schedules = append(schedules, schedule)
	}

	// Sort by next run time
	sort.Slice(schedules, func(i, j int) bool {
		if schedules[i].NextRun == nil {
			return false
		}
		if schedules[j].NextRun == nil {
			return true
		}
		return schedules[i].NextRun.Before(*schedules[j].NextRun)
	})

	span.SetAttributes(attribute.Int("schedules_count", len(schedules)))

	return schedules, nil
}

// TriggerSchedule manually triggers a schedule execution
func (ss *SyncScheduler) TriggerSchedule(ctx context.Context, scheduleID uuid.UUID) (*SyncJob, error) {
	ctx, span := ss.tracer.Start(ctx, "sync_scheduler.trigger_schedule")
	defer span.End()

	span.SetAttributes(attribute.String("schedule.id", scheduleID.String()))

	ss.mutex.RLock()
	schedule, exists := ss.schedules[scheduleID]
	ss.mutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("schedule not found")
	}

	return ss.executeSchedule(ctx, schedule, true)
}

// CheckThrottle checks if an operation should be throttled
func (ss *SyncScheduler) CheckThrottle(ctx context.Context, connectorType string) error {
	if !ss.config.EnableThrottling {
		return nil
	}

	throttler, exists := ss.throttlers[connectorType]
	if !exists {
		// Use global throttler if connector-specific one doesn't exist
		throttler = ss.throttlers["global"]
	}

	if throttler == nil {
		return nil
	}

	return throttler.checkThrottle(ctx)
}

// Internal methods

func (ss *SyncScheduler) startBackgroundProcessing() {
	go func() {
		for {
			select {
			case <-ss.stopChan:
				return
			case <-ss.ticker.C:
				ss.processSchedules()
			}
		}
	}()
}

func (ss *SyncScheduler) processSchedules() {
	ctx := context.Background()
	_, span := ss.tracer.Start(ctx, "sync_scheduler.process_schedules")
	defer span.End()

	now := time.Now()
	var schedulesToRun []*SyncSchedule

	ss.mutex.RLock()
	for _, schedule := range ss.schedules {
		if !schedule.IsActive {
			continue
		}

		if schedule.NextRun == nil || schedule.NextRun.After(now) {
			continue
		}

		// Check conditions
		if !ss.checkScheduleConditions(ctx, schedule) {
			// Skip but calculate next run
			nextRun, _ := ss.calculateNextRun(schedule)
			schedule.NextRun = nextRun
			continue
		}

		schedulesToRun = append(schedulesToRun, schedule)
	}
	ss.mutex.RUnlock()

	// Execute schedules
	for _, schedule := range schedulesToRun {
		go ss.executeScheduleAsync(ctx, schedule)
	}

	span.SetAttributes(attribute.Int("schedules_to_run", len(schedulesToRun)))
}

func (ss *SyncScheduler) executeScheduleAsync(ctx context.Context, schedule *SyncSchedule) {
	_, err := ss.executeSchedule(ctx, schedule, false)
	if err != nil {
		ss.handleScheduleFailure(ctx, schedule, err)
	}
}

func (ss *SyncScheduler) executeSchedule(ctx context.Context, schedule *SyncSchedule, manual bool) (*SyncJob, error) {
	ctx, span := ss.tracer.Start(ctx, "sync_scheduler.execute_schedule")
	defer span.End()

	span.SetAttributes(
		attribute.String("schedule.id", schedule.ID.String()),
		attribute.Bool("manual_trigger", manual),
	)

	startTime := time.Now()

	// Check throttling
	if err := ss.CheckThrottle(ctx, schedule.ConnectorType); err != nil {
		span.RecordError(err)
		ss.recordScheduleResult(schedule, nil, startTime, ScheduleStatusSkipped, "Throttled", err.Error())
		return nil, fmt.Errorf("schedule throttled: %w", err)
	}

	// Create sync request
	request := &StartSyncRequest{
		DataSourceID:  schedule.DataSourceID,
		ConnectorType: schedule.ConnectorType,
		Options:       schedule.SyncOptions,
	}

	// Start sync job
	job, err := ss.orchestrator.StartSync(ctx, request)
	if err != nil {
		span.RecordError(err)
		ss.recordScheduleResult(schedule, nil, startTime, ScheduleStatusFailed, "", err.Error())
		return nil, fmt.Errorf("failed to start sync job: %w", err)
	}

	// Update schedule
	ss.mutex.Lock()
	now := time.Now()
	schedule.LastRun = &now
	schedule.RunCount++

	if !manual {
		// Calculate next run time
		nextRun, err := ss.calculateNextRun(schedule)
		if err == nil {
			schedule.NextRun = nextRun
		}
	}
	ss.mutex.Unlock()

	// Record successful start
	ss.recordScheduleResult(schedule, job, startTime, ScheduleStatusRunning, "Sync job started", "")

	// Monitor job completion in background
	go ss.monitorScheduleJob(ctx, schedule, job, startTime)

	span.SetAttributes(attribute.String("job.id", job.ID.String()))

	return job, nil
}

func (ss *SyncScheduler) monitorScheduleJob(ctx context.Context, schedule *SyncSchedule, job *SyncJob, startTime time.Time) {
	// Wait for job completion
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			status := job.GetStatus()

			switch status.State {
			case SyncStateCompleted:
				ss.mutex.Lock()
				schedule.SuccessCount++
				schedule.RetryCount = 0 // Reset retry count on success
				ss.mutex.Unlock()

				ss.recordScheduleResult(schedule, job, startTime, ScheduleStatusCompleted, "Sync completed successfully", "")
				ss.sendNotification(ctx, schedule, ScheduleStatusCompleted, nil)
				return

			case SyncStateFailed, SyncStateCancelled:
				ss.mutex.Lock()
				schedule.FailureCount++
				ss.mutex.Unlock()

				errorMsg := ""
				if status.Error != "" {
					errorMsg = status.Error
				}

				ss.recordScheduleResult(schedule, job, startTime, ScheduleStatusFailed, "Sync failed", errorMsg)
				var err error
				if errorMsg != "" {
					err = fmt.Errorf("sync failed: %s", errorMsg)
				} else {
					err = fmt.Errorf("sync failed")
				}
				ss.sendNotification(ctx, schedule, ScheduleStatusFailed, err)
				return
			}
		}
	}
}

func (ss *SyncScheduler) calculateNextRun(schedule *SyncSchedule) (*time.Time, error) {
	now := time.Now()

	switch schedule.Schedule.Type {
	case ScheduleTypeOnce:
		if schedule.Schedule.RunOnce != nil && schedule.Schedule.RunOnce.After(now) {
			return schedule.Schedule.RunOnce, nil
		}
		return nil, nil // One-time schedule already passed

	case ScheduleTypeInterval:
		if schedule.Schedule.Interval <= 0 {
			return nil, fmt.Errorf("invalid interval")
		}

		var lastRun time.Time
		if schedule.LastRun != nil {
			lastRun = *schedule.LastRun
		} else {
			lastRun = schedule.CreatedAt
		}

		nextRun := lastRun.Add(schedule.Schedule.Interval)
		if nextRun.Before(now) {
			nextRun = now.Add(schedule.Schedule.Interval)
		}
		return &nextRun, nil

	case ScheduleTypeCron:
		// This would use a cron parser library
		// For now, return a placeholder
		nextRun := now.Add(1 * time.Hour)
		return &nextRun, nil

	case ScheduleTypeWeekly:
		return ss.calculateWeeklyNextRun(schedule, now)

	case ScheduleTypeMonthly:
		return ss.calculateMonthlyNextRun(schedule, now)

	default:
		return nil, fmt.Errorf("unsupported schedule type: %s", schedule.Schedule.Type)
	}
}

func (ss *SyncScheduler) calculateWeeklyNextRun(schedule *SyncSchedule, now time.Time) (*time.Time, error) {
	if schedule.Schedule.WeeklySchedule == nil {
		return nil, fmt.Errorf("weekly schedule config missing")
	}

	weekly := schedule.Schedule.WeeklySchedule
	if len(weekly.DaysOfWeek) == 0 {
		return nil, fmt.Errorf("no days of week specified")
	}

	// Parse time of day
	timeOfDay, err := time.Parse("15:04", weekly.TimeOfDay)
	if err != nil {
		return nil, fmt.Errorf("invalid time of day format: %w", err)
	}

	// Find next occurrence
	for i := 0; i < 7; i++ {
		candidate := now.AddDate(0, 0, i)

		// Check if this day is in the schedule
		for _, dayOfWeek := range weekly.DaysOfWeek {
			if candidate.Weekday() == dayOfWeek {
				// Set the time
				nextRun := time.Date(
					candidate.Year(), candidate.Month(), candidate.Day(),
					timeOfDay.Hour(), timeOfDay.Minute(), 0, 0,
					candidate.Location(),
				)

				if nextRun.After(now) {
					return &nextRun, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("could not calculate next weekly run")
}

func (ss *SyncScheduler) calculateMonthlyNextRun(schedule *SyncSchedule, now time.Time) (*time.Time, error) {
	if schedule.Schedule.MonthlySchedule == nil {
		return nil, fmt.Errorf("monthly schedule config missing")
	}

	monthly := schedule.Schedule.MonthlySchedule

	// Parse time of day
	timeOfDay, err := time.Parse("15:04", monthly.TimeOfDay)
	if err != nil {
		return nil, fmt.Errorf("invalid time of day format: %w", err)
	}

	// Calculate next monthly occurrence
	year, month, _ := now.Date()

	var dayOfMonth int
	if monthly.DayOfMonth == -1 {
		// Last day of month
		dayOfMonth = time.Date(year, month+1, 0, 0, 0, 0, 0, now.Location()).Day()
	} else {
		dayOfMonth = monthly.DayOfMonth
	}

	nextRun := time.Date(
		year, month, dayOfMonth,
		timeOfDay.Hour(), timeOfDay.Minute(), 0, 0,
		now.Location(),
	)

	if nextRun.Before(now) || nextRun.Equal(now) {
		// Move to next month
		if monthly.DayOfMonth == -1 {
			dayOfMonth = time.Date(year, month+2, 0, 0, 0, 0, 0, now.Location()).Day()
		}
		nextRun = time.Date(
			year, month+1, dayOfMonth,
			timeOfDay.Hour(), timeOfDay.Minute(), 0, 0,
			now.Location(),
		)
	}

	return &nextRun, nil
}

func (ss *SyncScheduler) checkScheduleConditions(ctx context.Context, schedule *SyncSchedule) bool {
	if len(schedule.Schedule.Conditions) == 0 {
		return true
	}

	for _, condition := range schedule.Schedule.Conditions {
		if !ss.evaluateCondition(ctx, condition, schedule) {
			return false
		}
	}

	return true
}

func (ss *SyncScheduler) evaluateCondition(ctx context.Context, condition ScheduleCondition, schedule *SyncSchedule) bool {
	// This would implement condition evaluation logic
	// For now, return true as placeholder
	return true
}

func (ss *SyncScheduler) recordScheduleResult(schedule *SyncSchedule, job *SyncJob, startTime time.Time, status ScheduleStatus, message, errorMsg string) {
	result := &ScheduleResult{
		ScheduleID: schedule.ID,
		StartTime:  startTime,
		EndTime:    time.Now(),
		Status:     status,
		Message:    message,
		Error:      errorMsg,
	}

	if job != nil {
		result.JobID = job.ID
		result.Metrics = job.Status.Metrics
	}

	result.Duration = result.EndTime.Sub(result.StartTime)

	ss.mutex.Lock()
	schedule.LastResult = result
	ss.mutex.Unlock()
}

func (ss *SyncScheduler) handleScheduleFailure(ctx context.Context, schedule *SyncSchedule, err error) {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()

	schedule.RetryCount++

	if schedule.RetryCount < schedule.MaxRetries {
		// Schedule retry with exponential backoff
		backoffDuration := time.Duration(float64(time.Minute) *
			(ss.config.RetryBackoffMultiplier * float64(schedule.RetryCount)))

		retryTime := time.Now().Add(backoffDuration)
		schedule.NextRun = &retryTime
	} else {
		// Max retries exceeded, calculate next regular run
		nextRun, calcErr := ss.calculateNextRun(schedule)
		if calcErr == nil {
			schedule.NextRun = nextRun
		}
		schedule.RetryCount = 0
	}
}

func (ss *SyncScheduler) sendNotification(ctx context.Context, schedule *SyncSchedule, status ScheduleStatus, err error) {
	if schedule.NotificationConfig == nil {
		return
	}

	config := schedule.NotificationConfig
	shouldNotify := false

	switch status {
	case ScheduleStatusCompleted:
		shouldNotify = config.OnSuccess
	case ScheduleStatusFailed:
		shouldNotify = config.OnFailure
	case ScheduleStatusSkipped:
		shouldNotify = config.OnSkip
	}

	if !shouldNotify {
		return
	}

	// This would implement actual notification sending
	// For now, just log the notification
	_, span := ss.tracer.Start(ctx, "sync_scheduler.send_notification")
	defer span.End()

	span.SetAttributes(
		attribute.String("schedule.id", schedule.ID.String()),
		attribute.String("status", string(status)),
		attribute.StringSlice("channels", config.Channels),
	)
}

func (ss *SyncScheduler) initializeThrottlers() {
	// Global throttler
	if ss.config.GlobalRateLimit != nil {
		ss.throttlers["global"] = NewThrottleManager(ss.config.GlobalRateLimit)
	}

	// Connector-specific throttlers
	if ss.config.ConnectorRateLimits != nil {
		for connectorType, config := range ss.config.ConnectorRateLimits {
			ss.throttlers[connectorType] = NewThrottleManager(config)
		}
	}
}

// NewThrottleManager creates a new throttle manager
func NewThrottleManager(config *RateLimitConfig) *ThrottleManager {
	tm := &ThrottleManager{
		config:      config,
		tokens:      make(chan struct{}, config.BurstLimit),
		windowStart: time.Now(),
	}

	// Fill initial tokens
	for i := 0; i < config.BurstLimit; i++ {
		tm.tokens <- struct{}{}
	}

	return tm
}

func (tm *ThrottleManager) checkThrottle(ctx context.Context) error {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	now := time.Now()

	// Check if we're in a backoff period
	if now.Before(tm.backoffUntil) {
		return fmt.Errorf("throttled until %s", tm.backoffUntil.Format(time.RFC3339))
	}

	// Reset window if needed
	if now.Sub(tm.windowStart) >= tm.config.WindowSize {
		tm.windowStart = now
		tm.requestCount = 0

		// Refill tokens
		for len(tm.tokens) < tm.config.BurstLimit {
			select {
			case tm.tokens <- struct{}{}:
			default:
				break
			}
		}
	}

	// Check rate limit
	maxRequests := int64(tm.config.RequestsPerSecond * tm.config.WindowSize.Seconds())
	if tm.requestCount >= maxRequests {
		// Apply backoff if adaptive throttling is enabled
		if tm.config.EnableAdaptive {
			backoffDuration := time.Duration(float64(time.Second) * tm.config.BackoffMultiplier)
			if backoffDuration > tm.config.MaxBackoffTime {
				backoffDuration = tm.config.MaxBackoffTime
			}
			tm.backoffUntil = now.Add(backoffDuration)
		}

		return fmt.Errorf("rate limit exceeded: %d requests in %s", tm.requestCount, tm.config.WindowSize)
	}

	// Try to get a token
	select {
	case <-tm.tokens:
		tm.requestCount++
		tm.lastRequest = now
		return nil
	default:
		return fmt.Errorf("burst limit exceeded")
	}
}

// ScheduleFilters contains filters for listing schedules
type ScheduleFilters struct {
	IsActive      *bool     `json:"is_active,omitempty"`
	ConnectorType string    `json:"connector_type,omitempty"`
	DataSourceID  uuid.UUID `json:"data_source_id,omitempty"`
}

// Shutdown gracefully shuts down the scheduler
func (ss *SyncScheduler) Shutdown(ctx context.Context) error {
	if ss.ticker != nil {
		ss.ticker.Stop()
	}

	close(ss.stopChan)

	return nil
}
