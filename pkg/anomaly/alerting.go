package anomaly

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// AlertingService provides real-time anomaly alerting capabilities
type AlertingService struct {
	config        *AlertingConfig
	channels      map[string]AlertChannel
	rules         map[uuid.UUID]*AlertRule
	subscriptions map[uuid.UUID]*AlertSubscription
	alertHistory  map[uuid.UUID][]*Alert
	ruleEngine    *RuleEngine
	tracer        trace.Tracer
	mutex         sync.RWMutex

	// Real-time processing
	alertQueue chan *Alert
	stopChan   chan struct{}
	workers    []*AlertWorker

	// Rate limiting and throttling
	rateLimiters   map[string]*RateLimiter
	alertThrottler *AlertThrottler
}

// AlertingConfig contains configuration for the alerting system
type AlertingConfig struct {
	Enabled                bool          `json:"enabled"`
	MaxAlertsPerMinute     int           `json:"max_alerts_per_minute"`
	MaxAlertsPerHour       int           `json:"max_alerts_per_hour"`
	AlertRetentionDays     int           `json:"alert_retention_days"`
	WorkerPoolSize         int           `json:"worker_pool_size"`
	AlertQueueSize         int           `json:"alert_queue_size"`
	DeduplicationWindow    time.Duration `json:"deduplication_window"`
	EscalationTimeout      time.Duration `json:"escalation_timeout"`
	DefaultCooldownPeriod  time.Duration `json:"default_cooldown_period"`
	EnableBatchAlerts      bool          `json:"enable_batch_alerts"`
	BatchSize              int           `json:"batch_size"`
	BatchTimeout           time.Duration `json:"batch_timeout"`
	RetryAttempts          int           `json:"retry_attempts"`
	RetryBackoffMultiplier float64       `json:"retry_backoff_multiplier"`
}

// Alert represents an alert generated from an anomaly
type Alert struct {
	ID               uuid.UUID                 `json:"id"`
	AnomalyID        uuid.UUID                 `json:"anomaly_id"`
	RuleID           uuid.UUID                 `json:"rule_id"`
	Title            string                    `json:"title"`
	Message          string                    `json:"message"`
	Severity         AlertSeverity             `json:"severity"`
	Status           AlertStatus               `json:"status"`
	CreatedAt        time.Time                 `json:"created_at"`
	UpdatedAt        time.Time                 `json:"updated_at"`
	ResolvedAt       *time.Time                `json:"resolved_at,omitempty"`
	EscalatedAt      *time.Time                `json:"escalated_at,omitempty"`
	AcknowledgedAt   *time.Time                `json:"acknowledged_at,omitempty"`
	AcknowledgedBy   *uuid.UUID                `json:"acknowledged_by,omitempty"`
	TenantID         uuid.UUID                 `json:"tenant_id"`
	DataSourceID     *uuid.UUID                `json:"data_source_id,omitempty"`
	UserID           *uuid.UUID                `json:"user_id,omitempty"`
	Tags             []string                  `json:"tags"`
	Metadata         map[string]interface{}    `json:"metadata"`
	Channels         []string                  `json:"channels"`
	Recipients       []AlertRecipient          `json:"recipients"`
	DeliveryStatus   map[string]DeliveryStatus `json:"delivery_status"`
	RetryCount       int                       `json:"retry_count"`
	NextRetryAt      *time.Time                `json:"next_retry_at,omitempty"`
	EscalationLevel  int                       `json:"escalation_level"`
	RelatedAlerts    []uuid.UUID               `json:"related_alerts,omitempty"`
	SuppressionRules []uuid.UUID               `json:"suppression_rules,omitempty"`
}

// AlertSeverity represents the severity level of an alert
type AlertSeverity string

const (
	AlertSeverityLow      AlertSeverity = "low"
	AlertSeverityMedium   AlertSeverity = "medium"
	AlertSeverityHigh     AlertSeverity = "high"
	AlertSeverityCritical AlertSeverity = "critical"
)

// AlertStatus represents the current status of an alert
type AlertStatus string

const (
	AlertStatusPending      AlertStatus = "pending"
	AlertStatusSent         AlertStatus = "sent"
	AlertStatusAcknowledged AlertStatus = "acknowledged"
	AlertStatusResolved     AlertStatus = "resolved"
	AlertStatusSuppressed   AlertStatus = "suppressed"
	AlertStatusFailed       AlertStatus = "failed"
	AlertStatusEscalated    AlertStatus = "escalated"
)

// AlertRecipient represents a recipient of an alert
type AlertRecipient struct {
	ID       uuid.UUID              `json:"id"`
	Type     string                 `json:"type"` // user, group, role, external
	Address  string                 `json:"address"`
	Name     string                 `json:"name,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// DeliveryStatus represents the delivery status for a specific channel
type DeliveryStatus struct {
	Channel      string     `json:"channel"`
	Status       string     `json:"status"`
	AttemptedAt  time.Time  `json:"attempted_at"`
	DeliveredAt  *time.Time `json:"delivered_at,omitempty"`
	Error        string     `json:"error,omitempty"`
	ResponseCode int        `json:"response_code,omitempty"`
	ResponseBody string     `json:"response_body,omitempty"`
}

// AlertRule defines conditions and actions for generating alerts
type AlertRule struct {
	ID               uuid.UUID         `json:"id"`
	Name             string            `json:"name"`
	Description      string            `json:"description"`
	TenantID         uuid.UUID         `json:"tenant_id"`
	Enabled          bool              `json:"enabled"`
	Priority         int               `json:"priority"`
	Conditions       []AlertCondition  `json:"conditions"`
	Actions          []AlertAction     `json:"actions"`
	CooldownPeriod   time.Duration     `json:"cooldown_period"`
	EscalationRules  []EscalationRule  `json:"escalation_rules,omitempty"`
	SuppressionRules []SuppressionRule `json:"suppression_rules,omitempty"`
	Tags             []string          `json:"tags"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
	CreatedBy        uuid.UUID         `json:"created_by"`
	LastTriggered    *time.Time        `json:"last_triggered,omitempty"`
	TriggerCount     int64             `json:"trigger_count"`
	Schedule         *AlertSchedule    `json:"schedule,omitempty"`
}

// AlertCondition defines a condition that must be met to trigger an alert
type AlertCondition struct {
	Field    string                 `json:"field"`
	Operator string                 `json:"operator"`
	Value    interface{}            `json:"value"`
	DataType string                 `json:"data_type"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// AlertAction defines an action to take when an alert is triggered
type AlertAction struct {
	Type       string                 `json:"type"`
	Channel    string                 `json:"channel"`
	Recipients []AlertRecipient       `json:"recipients"`
	Template   string                 `json:"template,omitempty"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
	Delay      time.Duration          `json:"delay,omitempty"`
}

// EscalationRule defines how alerts should be escalated
type EscalationRule struct {
	Level      int              `json:"level"`
	Delay      time.Duration    `json:"delay"`
	Recipients []AlertRecipient `json:"recipients"`
	Channels   []string         `json:"channels"`
	Condition  string           `json:"condition,omitempty"`
}

// SuppressionRule defines conditions for suppressing alerts
type SuppressionRule struct {
	ID         uuid.UUID        `json:"id"`
	Conditions []AlertCondition `json:"conditions"`
	Duration   time.Duration    `json:"duration"`
	Reason     string           `json:"reason"`
}

// AlertSchedule defines when alert rules are active
type AlertSchedule struct {
	Timezone    string     `json:"timezone"`
	ActiveHours []int      `json:"active_hours,omitempty"`
	ActiveDays  []int      `json:"active_days,omitempty"`
	StartDate   *time.Time `json:"start_date,omitempty"`
	EndDate     *time.Time `json:"end_date,omitempty"`
}

// AlertSubscription represents a subscription to alerts
type AlertSubscription struct {
	ID        uuid.UUID        `json:"id"`
	TenantID  uuid.UUID        `json:"tenant_id"`
	UserID    uuid.UUID        `json:"user_id"`
	Name      string           `json:"name"`
	Filters   []AlertCondition `json:"filters"`
	Channels  []string         `json:"channels"`
	Frequency string           `json:"frequency"` // immediate, batch, digest
	Enabled   bool             `json:"enabled"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
}

// AlertChannel defines the interface for alert delivery channels
type AlertChannel interface {
	Send(ctx context.Context, alert *Alert) error
	SendBatch(ctx context.Context, alerts []*Alert) error
	GetName() string
	IsEnabled() bool
	ValidateConfig() error
	GetDeliveryStatus(alertID uuid.UUID) (*DeliveryStatus, error)
}

// RuleEngine evaluates alert rules against anomalies
type RuleEngine struct {
	rules         map[uuid.UUID]*AlertRule
	lastTriggered map[uuid.UUID]time.Time
	mutex         sync.RWMutex
}

// RateLimiter implements rate limiting for alerts
type RateLimiter struct {
	maxPerMinute int
	maxPerHour   int
	minuteCount  int
	hourCount    int
	lastMinute   time.Time
	lastHour     time.Time
	mutex        sync.Mutex
}

// AlertThrottler implements alert throttling and deduplication
type AlertThrottler struct {
	config       *AlertingConfig
	recentAlerts map[string]time.Time
	alertCounts  map[string]int
	mutex        sync.RWMutex
}

// AlertWorker processes alerts from the queue
type AlertWorker struct {
	id       int
	service  *AlertingService
	stopChan chan struct{}
}

// NewAlertingService creates a new alerting service
func NewAlertingService(config *AlertingConfig) *AlertingService {
	if config == nil {
		config = &AlertingConfig{
			Enabled:                true,
			MaxAlertsPerMinute:     10,
			MaxAlertsPerHour:       100,
			AlertRetentionDays:     30,
			WorkerPoolSize:         3,
			AlertQueueSize:         1000,
			DeduplicationWindow:    5 * time.Minute,
			EscalationTimeout:      30 * time.Minute,
			DefaultCooldownPeriod:  15 * time.Minute,
			EnableBatchAlerts:      true,
			BatchSize:              10,
			BatchTimeout:           1 * time.Minute,
			RetryAttempts:          3,
			RetryBackoffMultiplier: 2.0,
		}
	}

	service := &AlertingService{
		config:         config,
		channels:       make(map[string]AlertChannel),
		rules:          make(map[uuid.UUID]*AlertRule),
		subscriptions:  make(map[uuid.UUID]*AlertSubscription),
		alertHistory:   make(map[uuid.UUID][]*Alert),
		rateLimiters:   make(map[string]*RateLimiter),
		alertQueue:     make(chan *Alert, config.AlertQueueSize),
		stopChan:       make(chan struct{}),
		tracer:         otel.Tracer("anomaly-alerting"),
		ruleEngine:     NewRuleEngine(),
		alertThrottler: NewAlertThrottler(config),
	}

	// Start worker pool
	if config.Enabled {
		service.startWorkerPool()
	}

	return service
}

// ProcessAnomaly processes an anomaly and generates alerts based on rules
func (s *AlertingService) ProcessAnomaly(ctx context.Context, anomaly *Anomaly) error {
	ctx, span := s.tracer.Start(ctx, "alerting_service.process_anomaly")
	defer span.End()

	if !s.config.Enabled {
		return nil
	}

	span.SetAttributes(
		attribute.String("anomaly_id", anomaly.ID.String()),
		attribute.String("anomaly_type", string(anomaly.Type)),
		attribute.String("severity", string(anomaly.Severity)),
	)

	// Find matching rules
	matchingRules := s.ruleEngine.FindMatchingRules(anomaly)

	// Generate alerts for matching rules
	for _, rule := range matchingRules {
		alert, err := s.createAlertFromRule(ctx, anomaly, rule)
		if err != nil {
			log.Printf("Failed to create alert from rule %s: %v", rule.ID, err)
			continue
		}

		// Check throttling and deduplication
		if s.alertThrottler.ShouldThrottle(alert) {
			log.Printf("Alert throttled: %s", alert.ID)
			continue
		}

		// Queue alert for processing
		select {
		case s.alertQueue <- alert:
			log.Printf("Alert queued: %s", alert.ID)
		default:
			log.Printf("Alert queue full, dropping alert: %s", alert.ID)
		}
	}

	span.SetAttributes(attribute.Int("matching_rules", len(matchingRules)))

	return nil
}

// SendAlert sends an alert immediately
func (s *AlertingService) SendAlert(ctx context.Context, alert *Alert) error {
	ctx, span := s.tracer.Start(ctx, "alerting_service.send_alert")
	defer span.End()

	span.SetAttributes(
		attribute.String("alert_id", alert.ID.String()),
		attribute.String("severity", string(alert.Severity)),
		attribute.StringSlice("channels", alert.Channels),
	)

	// Initialize delivery status
	alert.DeliveryStatus = make(map[string]DeliveryStatus)

	// Send to each configured channel
	for _, channelName := range alert.Channels {
		if channel, exists := s.channels[channelName]; exists && channel.IsEnabled() {
			deliveryStatus := DeliveryStatus{
				Channel:     channelName,
				Status:      "attempting",
				AttemptedAt: time.Now(),
			}

			err := channel.Send(ctx, alert)
			if err != nil {
				deliveryStatus.Status = "failed"
				deliveryStatus.Error = err.Error()
				log.Printf("Failed to send alert %s via %s: %v", alert.ID, channelName, err)
			} else {
				deliveryStatus.Status = "delivered"
				now := time.Now()
				deliveryStatus.DeliveredAt = &now
			}

			alert.DeliveryStatus[channelName] = deliveryStatus
		}
	}

	// Update alert status
	alert.UpdatedAt = time.Now()
	if s.allChannelsDelivered(alert) {
		alert.Status = AlertStatusSent
	} else if s.anyChannelDelivered(alert) {
		alert.Status = AlertStatusSent // Partial success
	} else {
		alert.Status = AlertStatusFailed
	}

	// Store alert in history
	s.storeAlert(alert)

	span.SetAttributes(
		attribute.String("final_status", string(alert.Status)),
		attribute.Int("delivery_attempts", len(alert.DeliveryStatus)),
	)

	return nil
}

// RegisterChannel registers an alert channel
func (s *AlertingService) RegisterChannel(channel AlertChannel) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	name := channel.GetName()
	if err := channel.ValidateConfig(); err != nil {
		return fmt.Errorf("invalid channel configuration for %s: %w", name, err)
	}

	s.channels[name] = channel
	log.Printf("Registered alert channel: %s", name)

	return nil
}

// AddRule adds an alert rule
func (s *AlertingService) AddRule(ctx context.Context, rule *AlertRule) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if rule.ID == uuid.Nil {
		rule.ID = uuid.New()
	}

	rule.CreatedAt = time.Now()
	rule.UpdatedAt = time.Now()

	s.rules[rule.ID] = rule
	s.ruleEngine.AddRule(rule)

	log.Printf("Added alert rule: %s (%s)", rule.Name, rule.ID)

	return nil
}

// UpdateRule updates an existing alert rule
func (s *AlertingService) UpdateRule(ctx context.Context, ruleID uuid.UUID, updates *AlertRule) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	rule, exists := s.rules[ruleID]
	if !exists {
		return fmt.Errorf("alert rule not found: %s", ruleID)
	}

	// Update fields
	if updates.Name != "" {
		rule.Name = updates.Name
	}
	if updates.Description != "" {
		rule.Description = updates.Description
	}
	if updates.Conditions != nil {
		rule.Conditions = updates.Conditions
	}
	if updates.Actions != nil {
		rule.Actions = updates.Actions
	}
	if updates.CooldownPeriod > 0 {
		rule.CooldownPeriod = updates.CooldownPeriod
	}

	rule.UpdatedAt = time.Now()
	s.ruleEngine.UpdateRule(rule)

	return nil
}

// RemoveRule removes an alert rule
func (s *AlertingService) RemoveRule(ctx context.Context, ruleID uuid.UUID) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if _, exists := s.rules[ruleID]; !exists {
		return fmt.Errorf("alert rule not found: %s", ruleID)
	}

	delete(s.rules, ruleID)
	s.ruleEngine.RemoveRule(ruleID)

	log.Printf("Removed alert rule: %s", ruleID)

	return nil
}

// GetAlert retrieves an alert by ID
func (s *AlertingService) GetAlert(ctx context.Context, alertID uuid.UUID) (*Alert, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	for _, alerts := range s.alertHistory {
		for _, alert := range alerts {
			if alert.ID == alertID {
				return alert, nil
			}
		}
	}

	return nil, fmt.Errorf("alert not found: %s", alertID)
}

// ListAlerts lists alerts with filtering
func (s *AlertingService) ListAlerts(ctx context.Context, filter *AlertFilter) ([]*Alert, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	var allAlerts []*Alert
	for _, alerts := range s.alertHistory {
		allAlerts = append(allAlerts, alerts...)
	}

	// Apply filters
	var filteredAlerts []*Alert
	for _, alert := range allAlerts {
		if s.matchesFilter(alert, filter) {
			filteredAlerts = append(filteredAlerts, alert)
		}
	}

	return filteredAlerts, nil
}

// AcknowledgeAlert acknowledges an alert
func (s *AlertingService) AcknowledgeAlert(ctx context.Context, alertID uuid.UUID, userID uuid.UUID) error {
	alert, err := s.GetAlert(ctx, alertID)
	if err != nil {
		return err
	}

	now := time.Now()
	alert.Status = AlertStatusAcknowledged
	alert.AcknowledgedAt = &now
	alert.AcknowledgedBy = &userID
	alert.UpdatedAt = now

	log.Printf("Alert acknowledged: %s by user %s", alertID, userID)

	return nil
}

// ResolveAlert resolves an alert
func (s *AlertingService) ResolveAlert(ctx context.Context, alertID uuid.UUID, userID uuid.UUID) error {
	alert, err := s.GetAlert(ctx, alertID)
	if err != nil {
		return err
	}

	now := time.Now()
	alert.Status = AlertStatusResolved
	alert.ResolvedAt = &now
	alert.UpdatedAt = now

	log.Printf("Alert resolved: %s by user %s", alertID, userID)

	return nil
}

// Internal methods

func (s *AlertingService) createAlertFromRule(ctx context.Context, anomaly *Anomaly, rule *AlertRule) (*Alert, error) {
	alert := &Alert{
		ID:              uuid.New(),
		AnomalyID:       anomaly.ID,
		RuleID:          rule.ID,
		Title:           s.generateAlertTitle(anomaly, rule),
		Message:         s.generateAlertMessage(anomaly, rule),
		Severity:        s.mapAnomalySeverityToAlert(anomaly.Severity),
		Status:          AlertStatusPending,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
		TenantID:        anomaly.TenantID,
		DataSourceID:    anomaly.DataSourceID,
		UserID:          anomaly.UserID,
		Tags:            rule.Tags,
		Metadata:        make(map[string]interface{}),
		DeliveryStatus:  make(map[string]DeliveryStatus),
		EscalationLevel: 0,
	}

	// Extract channels and recipients from actions
	for _, action := range rule.Actions {
		alert.Channels = append(alert.Channels, action.Channel)
		alert.Recipients = append(alert.Recipients, action.Recipients...)
	}

	// Add anomaly metadata
	alert.Metadata["anomaly_type"] = anomaly.Type
	alert.Metadata["anomaly_score"] = anomaly.Score
	alert.Metadata["anomaly_confidence"] = anomaly.Confidence
	alert.Metadata["detector_name"] = anomaly.DetectorName

	return alert, nil
}

func (s *AlertingService) generateAlertTitle(anomaly *Anomaly, rule *AlertRule) string {
	return fmt.Sprintf("[%s] %s - %s",
		s.mapAnomalySeverityToAlert(anomaly.Severity),
		rule.Name,
		anomaly.Title)
}

func (s *AlertingService) generateAlertMessage(anomaly *Anomaly, rule *AlertRule) string {
	return fmt.Sprintf("Anomaly detected: %s\n\nDetails:\n- Type: %s\n- Severity: %s\n- Score: %.2f\n- Confidence: %.2f\n- Detector: %s\n\nDescription: %s",
		anomaly.Title,
		anomaly.Type,
		anomaly.Severity,
		anomaly.Score,
		anomaly.Confidence,
		anomaly.DetectorName,
		anomaly.Description)
}

func (s *AlertingService) mapAnomalySeverityToAlert(severity AnomalySeverity) AlertSeverity {
	switch severity {
	case SeverityLow:
		return AlertSeverityLow
	case SeverityMedium:
		return AlertSeverityMedium
	case SeverityHigh:
		return AlertSeverityHigh
	case SeverityCritical:
		return AlertSeverityCritical
	default:
		return AlertSeverityMedium
	}
}

func (s *AlertingService) allChannelsDelivered(alert *Alert) bool {
	for _, channelName := range alert.Channels {
		if status, exists := alert.DeliveryStatus[channelName]; !exists || status.Status != "delivered" {
			return false
		}
	}
	return true
}

func (s *AlertingService) anyChannelDelivered(alert *Alert) bool {
	for _, status := range alert.DeliveryStatus {
		if status.Status == "delivered" {
			return true
		}
	}
	return false
}

func (s *AlertingService) storeAlert(alert *Alert) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	alerts := s.alertHistory[alert.TenantID]
	alerts = append(alerts, alert)
	s.alertHistory[alert.TenantID] = alerts
}

func (s *AlertingService) matchesFilter(alert *Alert, filter *AlertFilter) bool {
	if filter == nil {
		return true
	}

	if filter.TenantID != uuid.Nil && alert.TenantID != filter.TenantID {
		return false
	}

	if len(filter.Severities) > 0 {
		found := false
		for _, severity := range filter.Severities {
			if alert.Severity == severity {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if len(filter.Statuses) > 0 {
		found := false
		for _, status := range filter.Statuses {
			if alert.Status == status {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

func (s *AlertingService) startWorkerPool() {
	workerCount := s.config.WorkerPoolSize
	if workerCount <= 0 {
		workerCount = 3
	}

	s.workers = make([]*AlertWorker, workerCount)

	for i := 0; i < workerCount; i++ {
		worker := &AlertWorker{
			id:       i,
			service:  s,
			stopChan: make(chan struct{}),
		}
		s.workers[i] = worker
		go worker.start()
	}

	log.Printf("Started alert worker pool with %d workers", workerCount)
}

func (s *AlertingService) Shutdown(ctx context.Context) error {
	log.Println("Shutting down alerting service...")

	// Stop all workers
	for _, worker := range s.workers {
		close(worker.stopChan)
	}

	// Close processing queue
	close(s.stopChan)

	log.Println("Alerting service shut down complete")
	return nil
}

// AlertWorker implementation

func (w *AlertWorker) start() {
	log.Printf("Starting alert worker %d", w.id)

	for {
		select {
		case <-w.stopChan:
			log.Printf("Stopping alert worker %d", w.id)
			return
		case alert := <-w.service.alertQueue:
			w.processAlert(alert)
		}
	}
}

func (w *AlertWorker) processAlert(alert *Alert) {
	ctx := context.Background()
	err := w.service.SendAlert(ctx, alert)
	if err != nil {
		log.Printf("Worker %d failed to send alert %s: %v", w.id, alert.ID, err)
	}
}

// RuleEngine implementation

func NewRuleEngine() *RuleEngine {
	return &RuleEngine{
		rules:         make(map[uuid.UUID]*AlertRule),
		lastTriggered: make(map[uuid.UUID]time.Time),
	}
}

func (re *RuleEngine) AddRule(rule *AlertRule) {
	re.mutex.Lock()
	defer re.mutex.Unlock()
	re.rules[rule.ID] = rule
}

func (re *RuleEngine) UpdateRule(rule *AlertRule) {
	re.mutex.Lock()
	defer re.mutex.Unlock()
	re.rules[rule.ID] = rule
}

func (re *RuleEngine) RemoveRule(ruleID uuid.UUID) {
	re.mutex.Lock()
	defer re.mutex.Unlock()
	delete(re.rules, ruleID)
	delete(re.lastTriggered, ruleID)
}

func (re *RuleEngine) FindMatchingRules(anomaly *Anomaly) []*AlertRule {
	re.mutex.RLock()
	defer re.mutex.RUnlock()

	var matchingRules []*AlertRule

	for _, rule := range re.rules {
		if !rule.Enabled {
			continue
		}

		// Check tenant match
		if rule.TenantID != anomaly.TenantID {
			continue
		}

		// Check cooldown period
		if lastTriggered, exists := re.lastTriggered[rule.ID]; exists {
			if time.Since(lastTriggered) < rule.CooldownPeriod {
				continue
			}
		}

		// Evaluate conditions
		if re.evaluateConditions(rule.Conditions, anomaly) {
			matchingRules = append(matchingRules, rule)
			re.lastTriggered[rule.ID] = time.Now()
		}
	}

	return matchingRules
}

func (re *RuleEngine) evaluateConditions(conditions []AlertCondition, anomaly *Anomaly) bool {
	for _, condition := range conditions {
		if !re.evaluateCondition(condition, anomaly) {
			return false // All conditions must match (AND logic)
		}
	}
	return true
}

func (re *RuleEngine) evaluateCondition(condition AlertCondition, anomaly *Anomaly) bool {
	var fieldValue interface{}

	// Extract field value from anomaly
	switch condition.Field {
	case "type":
		fieldValue = string(anomaly.Type)
	case "severity":
		fieldValue = string(anomaly.Severity)
	case "score":
		fieldValue = anomaly.Score
	case "confidence":
		fieldValue = anomaly.Confidence
	case "detector_name":
		fieldValue = anomaly.DetectorName
	default:
		// Check metadata
		if value, exists := anomaly.Metadata[condition.Field]; exists {
			fieldValue = value
		} else {
			return false
		}
	}

	// Evaluate condition based on operator
	return re.compareValues(fieldValue, condition.Operator, condition.Value)
}

func (re *RuleEngine) compareValues(actual interface{}, operator string, expected interface{}) bool {
	switch operator {
	case "eq", "==":
		return actual == expected
	case "ne", "!=":
		return actual != expected
	case "gt", ">":
		return re.compareNumeric(actual, expected, ">")
	case "gte", ">=":
		return re.compareNumeric(actual, expected, ">=")
	case "lt", "<":
		return re.compareNumeric(actual, expected, "<")
	case "lte", "<=":
		return re.compareNumeric(actual, expected, "<=")
	case "contains":
		actualStr := fmt.Sprintf("%v", actual)
		expectedStr := fmt.Sprintf("%v", expected)
		return strings.Contains(actualStr, expectedStr)
	case "in":
		expectedSlice, ok := expected.([]interface{})
		if ok {
			for _, item := range expectedSlice {
				if actual == item {
					return true
				}
			}
		}
		return false
	default:
		return false
	}
}

func (re *RuleEngine) compareNumeric(actual, expected interface{}, operator string) bool {
	actualFloat, ok1 := actual.(float64)
	expectedFloat, ok2 := expected.(float64)

	if !ok1 || !ok2 {
		return false
	}

	switch operator {
	case ">":
		return actualFloat > expectedFloat
	case ">=":
		return actualFloat >= expectedFloat
	case "<":
		return actualFloat < expectedFloat
	case "<=":
		return actualFloat <= expectedFloat
	default:
		return false
	}
}

// AlertThrottler implementation

func NewAlertThrottler(config *AlertingConfig) *AlertThrottler {
	return &AlertThrottler{
		config:       config,
		recentAlerts: make(map[string]time.Time),
		alertCounts:  make(map[string]int),
	}
}

func (at *AlertThrottler) ShouldThrottle(alert *Alert) bool {
	at.mutex.Lock()
	defer at.mutex.Unlock()

	// Create deduplication key
	dedupKey := fmt.Sprintf("%s:%s", alert.AnomalyID, alert.RuleID)

	// Check deduplication window
	if lastSeen, exists := at.recentAlerts[dedupKey]; exists {
		if time.Since(lastSeen) < at.config.DeduplicationWindow {
			return true // Throttle duplicate alert
		}
	}

	at.recentAlerts[dedupKey] = time.Now()
	return false
}

// AlertFilter represents filters for alert queries
type AlertFilter struct {
	TenantID   uuid.UUID       `json:"tenant_id,omitempty"`
	Severities []AlertSeverity `json:"severities,omitempty"`
	Statuses   []AlertStatus   `json:"statuses,omitempty"`
	RuleIDs    []uuid.UUID     `json:"rule_ids,omitempty"`
	TimeRange  *TimeWindow     `json:"time_range,omitempty"`
	Limit      int             `json:"limit,omitempty"`
	Offset     int             `json:"offset,omitempty"`
}
