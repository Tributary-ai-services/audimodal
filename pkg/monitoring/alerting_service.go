package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/jscharber/eAIIngest/pkg/events"
	"github.com/jscharber/eAIIngest/pkg/metrics"
)

// AlertingService manages alerts and notifications for the system
type AlertingService struct {
	rules       map[string]*AlertRule
	rulesMutex  sync.RWMutex
	alerts      map[string]*Alert
	alertsMutex sync.RWMutex
	producer    *events.Producer
	tracer      trace.Tracer
	config      *AlertingConfig
	stopCh      chan struct{}
	running     bool
}

// AlertingConfig contains configuration for the alerting service
type AlertingConfig struct {
	// Evaluation settings
	EvaluationInterval time.Duration `yaml:"evaluation_interval"`
	DefaultThreshold   float64       `yaml:"default_threshold"`
	
	// Notification settings
	NotificationChannels []NotificationChannel `yaml:"notification_channels"`
	DefaultSeverity      AlertSeverity         `yaml:"default_severity"`
	
	// Alert lifecycle
	AlertTimeout         time.Duration `yaml:"alert_timeout"`
	AlertRetention       time.Duration `yaml:"alert_retention"`
	ResolutionTimeout    time.Duration `yaml:"resolution_timeout"`
	
	// Rate limiting
	MaxAlertsPerMinute   int `yaml:"max_alerts_per_minute"`
	GroupingWindow       time.Duration `yaml:"grouping_window"`
}

// AlertSeverity represents the severity level of an alert
type AlertSeverity string

const (
	SeverityCritical AlertSeverity = "critical"
	SeverityWarning  AlertSeverity = "warning"
	SeverityInfo     AlertSeverity = "info"
)

// AlertState represents the current state of an alert
type AlertState string

const (
	AlertStateFiring   AlertState = "firing"
	AlertStatePending  AlertState = "pending"
	AlertStateResolved AlertState = "resolved"
)

// NotificationChannel represents a channel for sending alerts
type NotificationChannel struct {
	Type     string                 `yaml:"type"`     // slack, email, webhook, pagerduty
	Name     string                 `yaml:"name"`
	Config   map[string]interface{} `yaml:"config"`
	Enabled  bool                   `yaml:"enabled"`
	Severity []AlertSeverity        `yaml:"severity"` // Which severities to send to this channel
}

// AlertRule defines conditions for triggering alerts
type AlertRule struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Query       string            `json:"query"`       // Metric query expression
	Condition   AlertCondition    `json:"condition"`   // Threshold condition
	Severity    AlertSeverity     `json:"severity"`
	Labels      map[string]string `json:"labels"`
	
	// Timing
	For          time.Duration `json:"for"`           // How long condition must be true
	Interval     time.Duration `json:"interval"`      // How often to evaluate
	
	// State
	LastEvaluated time.Time  `json:"last_evaluated"`
	LastTriggered *time.Time `json:"last_triggered,omitempty"`
	Enabled       bool       `json:"enabled"`
	
	// Internal state
	consecutiveFailures int
	pendingSince        *time.Time
}

// AlertCondition defines the condition for triggering an alert
type AlertCondition struct {
	Operator  string  `json:"operator"`  // gt, lt, eq, ne, gte, lte
	Threshold float64 `json:"threshold"`
	Function  string  `json:"function"`  // avg, max, min, sum, count
}

// Alert represents an active or resolved alert
type Alert struct {
	ID          string                 `json:"id"`
	RuleID      string                 `json:"rule_id"`
	RuleName    string                 `json:"rule_name"`
	State       AlertState             `json:"state"`
	Severity    AlertSeverity          `json:"severity"`
	Message     string                 `json:"message"`
	Description string                 `json:"description"`
	Value       float64                `json:"value"`
	Threshold   float64                `json:"threshold"`
	Labels      map[string]string      `json:"labels"`
	
	// Timing
	FiredAt    time.Time  `json:"fired_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	UpdatedAt  time.Time  `json:"updated_at"`
	
	// Metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	
	// Notification tracking
	NotificationsSent []NotificationRecord `json:"notifications_sent"`
}

// NotificationRecord tracks sent notifications
type NotificationRecord struct {
	Channel   string    `json:"channel"`
	SentAt    time.Time `json:"sent_at"`
	Status    string    `json:"status"` // sent, failed, pending
	Error     string    `json:"error,omitempty"`
	MessageID string    `json:"message_id,omitempty"`
}

// DefaultAlertingConfig returns default alerting configuration
func DefaultAlertingConfig() *AlertingConfig {
	return &AlertingConfig{
		EvaluationInterval: 30 * time.Second,
		DefaultThreshold:   0.8,
		DefaultSeverity:    SeverityWarning,
		AlertTimeout:       24 * time.Hour,
		AlertRetention:     7 * 24 * time.Hour,
		ResolutionTimeout:  5 * time.Minute,
		MaxAlertsPerMinute: 100,
		GroupingWindow:     5 * time.Minute,
		NotificationChannels: []NotificationChannel{
			{
				Type:     "webhook",
				Name:     "default",
				Enabled:  true,
				Severity: []AlertSeverity{SeverityCritical, SeverityWarning},
				Config: map[string]interface{}{
					"url": "http://localhost:8080/webhooks/alerts",
				},
			},
		},
	}
}

// NewAlertingService creates a new alerting service
func NewAlertingService(producer *events.Producer, config *AlertingConfig) *AlertingService {
	if config == nil {
		config = DefaultAlertingConfig()
	}

	return &AlertingService{
		rules:    make(map[string]*AlertRule),
		alerts:   make(map[string]*Alert),
		producer: producer,
		tracer:   otel.Tracer("alerting-service"),
		config:   config,
		stopCh:   make(chan struct{}),
	}
}

// Start starts the alerting service
func (as *AlertingService) Start(ctx context.Context) error {
	as.running = true
	
	// Load default alert rules
	as.loadDefaultRules()
	
	// Start evaluation loop
	go as.evaluationLoop(ctx)
	
	// Start cleanup routine
	go as.cleanupLoop(ctx)
	
	return nil
}

// Stop stops the alerting service
func (as *AlertingService) Stop() error {
	if as.running {
		close(as.stopCh)
		as.running = false
	}
	return nil
}

// loadDefaultRules loads system default alert rules
func (as *AlertingService) loadDefaultRules() {
	defaultRules := []*AlertRule{
		{
			ID:          "high_memory_usage",
			Name:        "High Memory Usage",
			Description: "Memory usage is above 85%",
			Query:       "go_memstats_heap_alloc_bytes",
			Condition: AlertCondition{
				Operator:  "gt",
				Threshold: 0.85,
				Function:  "avg",
			},
			Severity: SeverityWarning,
			For:      2 * time.Minute,
			Interval: 30 * time.Second,
			Enabled:  true,
			Labels: map[string]string{
				"category": "system",
				"resource": "memory",
			},
		},
		{
			ID:          "high_error_rate",
			Name:        "High Error Rate",
			Description: "Processing error rate is above 5%",
			Query:       "processing_errors_total",
			Condition: AlertCondition{
				Operator:  "gt",
				Threshold: 0.05,
				Function:  "rate",
			},
			Severity: SeverityCritical,
			For:      1 * time.Minute,
			Interval: 15 * time.Second,
			Enabled:  true,
			Labels: map[string]string{
				"category": "application",
				"type":     "error_rate",
			},
		},
		{
			ID:          "kafka_consumer_lag",
			Name:        "Kafka Consumer Lag",
			Description: "Kafka consumer lag is too high",
			Query:       "kafka_consumer_lag",
			Condition: AlertCondition{
				Operator:  "gt",
				Threshold: 1000,
				Function:  "max",
			},
			Severity: SeverityWarning,
			For:      5 * time.Minute,
			Interval: 1 * time.Minute,
			Enabled:  true,
			Labels: map[string]string{
				"category": "infrastructure",
				"component": "kafka",
			},
		},
		{
			ID:          "processing_queue_backup",
			Name:        "Processing Queue Backup",
			Description: "Processing queue has too many pending items",
			Query:       "processing_queue_size",
			Condition: AlertCondition{
				Operator:  "gt",
				Threshold: 10000,
				Function:  "avg",
			},
			Severity: SeverityWarning,
			For:      10 * time.Minute,
			Interval: 2 * time.Minute,
			Enabled:  true,
			Labels: map[string]string{
				"category": "application",
				"component": "queue",
			},
		},
		{
			ID:          "embedding_api_failures",
			Name:        "Embedding API Failures",
			Description: "High rate of embedding API failures",
			Query:       "embedding_api_errors_total",
			Condition: AlertCondition{
				Operator:  "gt",
				Threshold: 0.1,
				Function:  "rate",
			},
			Severity: SeverityCritical,
			For:      2 * time.Minute,
			Interval: 30 * time.Second,
			Enabled:  true,
			Labels: map[string]string{
				"category": "external_api",
				"service":  "openai",
			},
		},
	}

	for _, rule := range defaultRules {
		as.AddRule(rule)
	}
}

// AddRule adds a new alert rule
func (as *AlertingService) AddRule(rule *AlertRule) error {
	as.rulesMutex.Lock()
	defer as.rulesMutex.Unlock()

	if rule.ID == "" {
		return fmt.Errorf("rule ID is required")
	}

	if rule.Interval == 0 {
		rule.Interval = as.config.EvaluationInterval
	}

	as.rules[rule.ID] = rule
	return nil
}

// RemoveRule removes an alert rule
func (as *AlertingService) RemoveRule(ruleID string) error {
	as.rulesMutex.Lock()
	defer as.rulesMutex.Unlock()

	delete(as.rules, ruleID)
	return nil
}

// GetRule gets an alert rule by ID
func (as *AlertingService) GetRule(ruleID string) (*AlertRule, error) {
	as.rulesMutex.RLock()
	defer as.rulesMutex.RUnlock()

	rule, exists := as.rules[ruleID]
	if !exists {
		return nil, fmt.Errorf("rule not found: %s", ruleID)
	}

	return rule, nil
}

// evaluationLoop runs the main evaluation loop
func (as *AlertingService) evaluationLoop(ctx context.Context) {
	ticker := time.NewTicker(as.config.EvaluationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-as.stopCh:
			return
		case <-ticker.C:
			as.evaluateRules(ctx)
		}
	}
}

// evaluateRules evaluates all active alert rules
func (as *AlertingService) evaluateRules(ctx context.Context) {
	ctx, span := as.tracer.Start(ctx, "evaluate_alert_rules")
	defer span.End()

	as.rulesMutex.RLock()
	rules := make([]*AlertRule, 0, len(as.rules))
	for _, rule := range as.rules {
		if rule.Enabled {
			rules = append(rules, rule)
		}
	}
	as.rulesMutex.RUnlock()

	span.SetAttributes(attribute.Int("rules.count", len(rules)))

	for _, rule := range rules {
		as.evaluateRule(ctx, rule)
	}
}

// evaluateRule evaluates a single alert rule
func (as *AlertingService) evaluateRule(ctx context.Context, rule *AlertRule) {
	ctx, span := as.tracer.Start(ctx, "evaluate_rule")
	defer span.End()

	span.SetAttributes(
		attribute.String("rule.id", rule.ID),
		attribute.String("rule.name", rule.Name),
	)

	// Check if it's time to evaluate this rule
	if time.Since(rule.LastEvaluated) < rule.Interval {
		return
	}

	rule.LastEvaluated = time.Now()

	// Get metric value
	value, err := as.getMetricValue(rule.Query, rule.Condition.Function)
	if err != nil {
		span.RecordError(err)
		return
	}

	span.SetAttributes(
		attribute.Float64("metric.value", value),
		attribute.Float64("rule.threshold", rule.Condition.Threshold),
	)

	// Evaluate condition
	conditionMet := as.evaluateCondition(value, rule.Condition)

	if conditionMet {
		rule.consecutiveFailures++
		if rule.pendingSince == nil {
			now := time.Now()
			rule.pendingSince = &now
		}

		// Check if alert should fire
		if time.Since(*rule.pendingSince) >= rule.For {
			as.fireAlert(ctx, rule, value)
			rule.consecutiveFailures = 0
			rule.pendingSince = nil
		}
	} else {
		// Condition not met - resolve any active alerts
		as.resolveAlert(ctx, rule.ID)
		rule.consecutiveFailures = 0
		rule.pendingSince = nil
	}
}

// getMetricValue retrieves a metric value from the metrics registry
func (as *AlertingService) getMetricValue(query, function string) (float64, error) {
	registry := metrics.GetRegistry()
	allMetrics := registry.GetMetrics()

	metric, exists := allMetrics[query]
	if !exists {
		return 0, fmt.Errorf("metric not found: %s", query)
	}

	value := metric.GetValue()

	// Apply function based on metric type and function
	switch function {
	case "avg", "current":
		return as.extractNumericValue(value)
	case "max", "min", "sum", "count":
		// For complex metrics like histograms, extract appropriate value
		return as.extractNumericValue(value)
	case "rate":
		// For rate calculations, we'd need historical data
		// For now, return the current value
		return as.extractNumericValue(value)
	default:
		return as.extractNumericValue(value)
	}
}

// extractNumericValue extracts a numeric value from various metric types
func (as *AlertingService) extractNumericValue(value interface{}) (float64, error) {
	switch v := value.(type) {
	case int64:
		return float64(v), nil
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case map[string]interface{}:
		// For complex metrics like histograms or timers
		if avg, ok := v["avg"].(float64); ok {
			return avg, nil
		}
		if count, ok := v["total_count"].(int64); ok {
			return float64(count), nil
		}
		if sum, ok := v["sum"].(int64); ok {
			return float64(sum), nil
		}
	}
	return 0, fmt.Errorf("cannot extract numeric value from %T", value)
}

// evaluateCondition checks if a condition is met
func (as *AlertingService) evaluateCondition(value float64, condition AlertCondition) bool {
	switch condition.Operator {
	case "gt":
		return value > condition.Threshold
	case "gte":
		return value >= condition.Threshold
	case "lt":
		return value < condition.Threshold
	case "lte":
		return value <= condition.Threshold
	case "eq":
		return math.Abs(value-condition.Threshold) < 0.0001 // Float comparison with epsilon
	case "ne":
		return math.Abs(value-condition.Threshold) >= 0.0001
	default:
		return false
	}
}

// fireAlert creates and fires a new alert
func (as *AlertingService) fireAlert(ctx context.Context, rule *AlertRule, value float64) {
	ctx, span := as.tracer.Start(ctx, "fire_alert")
	defer span.End()

	alertID := fmt.Sprintf("%s_%d", rule.ID, time.Now().Unix())

	alert := &Alert{
		ID:          alertID,
		RuleID:      rule.ID,
		RuleName:    rule.Name,
		State:       AlertStateFiring,
		Severity:    rule.Severity,
		Message:     as.generateAlertMessage(rule, value),
		Description: rule.Description,
		Value:       value,
		Threshold:   rule.Condition.Threshold,
		Labels:      rule.Labels,
		FiredAt:     time.Now(),
		UpdatedAt:   time.Now(),
		Metadata: map[string]interface{}{
			"query":     rule.Query,
			"condition": rule.Condition,
		},
	}

	span.SetAttributes(
		attribute.String("alert.id", alertID),
		attribute.String("alert.severity", string(rule.Severity)),
		attribute.Float64("alert.value", value),
	)

	// Store alert
	as.alertsMutex.Lock()
	as.alerts[alertID] = alert
	as.alertsMutex.Unlock()

	// Update rule state
	now := time.Now()
	rule.LastTriggered = &now

	// Send notifications
	go as.sendNotifications(ctx, alert)

	// Emit alert event
	as.emitAlertEvent(ctx, alert, "fired")
}

// resolveAlert resolves an active alert
func (as *AlertingService) resolveAlert(ctx context.Context, ruleID string) {
	as.alertsMutex.Lock()
	defer as.alertsMutex.Unlock()

	for _, alert := range as.alerts {
		if alert.RuleID == ruleID && alert.State == AlertStateFiring {
			alert.State = AlertStateResolved
			now := time.Now()
			alert.ResolvedAt = &now
			alert.UpdatedAt = now

			// Send resolution notifications
			go as.sendResolutionNotifications(ctx, alert)

			// Emit resolution event
			as.emitAlertEvent(ctx, alert, "resolved")
		}
	}
}

// generateAlertMessage generates a human-readable alert message
func (as *AlertingService) generateAlertMessage(rule *AlertRule, value float64) string {
	return fmt.Sprintf("%s: current value %.2f %s threshold %.2f",
		rule.Name, value, rule.Condition.Operator, rule.Condition.Threshold)
}

// sendNotifications sends alert notifications to configured channels
func (as *AlertingService) sendNotifications(ctx context.Context, alert *Alert) {
	ctx, span := as.tracer.Start(ctx, "send_alert_notifications")
	defer span.End()

	for _, channel := range as.config.NotificationChannels {
		if !channel.Enabled {
			continue
		}

		// Check if this channel should receive this severity
		shouldSend := false
		for _, severity := range channel.Severity {
			if severity == alert.Severity {
				shouldSend = true
				break
			}
		}

		if !shouldSend {
			continue
		}

		record := NotificationRecord{
			Channel: channel.Name,
			SentAt:  time.Now(),
			Status:  "pending",
		}

		// Send notification based on channel type
		err := as.sendNotification(ctx, &channel, alert)
		if err != nil {
			record.Status = "failed"
			record.Error = err.Error()
			span.RecordError(err)
		} else {
			record.Status = "sent"
		}

		// Store notification record
		alert.NotificationsSent = append(alert.NotificationsSent, record)
	}
}

// sendNotification sends a notification to a specific channel
func (as *AlertingService) sendNotification(ctx context.Context, channel *NotificationChannel, alert *Alert) error {
	switch channel.Type {
	case "webhook":
		return as.sendWebhookNotification(ctx, channel, alert)
	case "slack":
		return as.sendSlackNotification(ctx, channel, alert)
	case "email":
		return as.sendEmailNotification(ctx, channel, alert)
	default:
		return fmt.Errorf("unsupported notification channel type: %s", channel.Type)
	}
}

// sendWebhookNotification sends a webhook notification
func (as *AlertingService) sendWebhookNotification(ctx context.Context, channel *NotificationChannel, alert *Alert) error {
	// Implementation would make HTTP POST to webhook URL
	// For now, just log the notification
	fmt.Printf("Webhook notification sent to %s: %s\n", channel.Name, alert.Message)
	return nil
}

// sendSlackNotification sends a Slack notification
func (as *AlertingService) sendSlackNotification(ctx context.Context, channel *NotificationChannel, alert *Alert) error {
	// Implementation would use Slack API
	fmt.Printf("Slack notification sent to %s: %s\n", channel.Name, alert.Message)
	return nil
}

// sendEmailNotification sends an email notification
func (as *AlertingService) sendEmailNotification(ctx context.Context, channel *NotificationChannel, alert *Alert) error {
	// Implementation would use SMTP
	fmt.Printf("Email notification sent to %s: %s\n", channel.Name, alert.Message)
	return nil
}

// sendResolutionNotifications sends notifications when alerts are resolved
func (as *AlertingService) sendResolutionNotifications(ctx context.Context, alert *Alert) {
	// Similar to sendNotifications but for resolution
	fmt.Printf("Alert resolved: %s\n", alert.Message)
}

// emitAlertEvent emits an alert event to the event bus
func (as *AlertingService) emitAlertEvent(ctx context.Context, alert *Alert, action string) {
	// Create alert event for the event bus
	// This would allow other services to react to alerts
	
	// For now, just log the event
	fmt.Printf("Alert event: %s - %s\n", action, alert.ID)
}

// cleanupLoop periodically cleans up old resolved alerts
func (as *AlertingService) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-as.stopCh:
			return
		case <-ticker.C:
			as.cleanupOldAlerts()
		}
	}
}

// cleanupOldAlerts removes old resolved alerts
func (as *AlertingService) cleanupOldAlerts() {
	as.alertsMutex.Lock()
	defer as.alertsMutex.Unlock()

	cutoff := time.Now().Add(-as.config.AlertRetention)

	for id, alert := range as.alerts {
		if alert.State == AlertStateResolved && 
		   alert.ResolvedAt != nil && 
		   alert.ResolvedAt.Before(cutoff) {
			delete(as.alerts, id)
		}
	}
}

// GetActiveAlerts returns all currently active alerts
func (as *AlertingService) GetActiveAlerts() []*Alert {
	as.alertsMutex.RLock()
	defer as.alertsMutex.RUnlock()

	var active []*Alert
	for _, alert := range as.alerts {
		if alert.State == AlertStateFiring {
			active = append(active, alert)
		}
	}

	return active
}

// GetAllAlerts returns all alerts (active and resolved)
func (as *AlertingService) GetAllAlerts() []*Alert {
	as.alertsMutex.RLock()
	defer as.alertsMutex.RUnlock()

	var alerts []*Alert
	for _, alert := range as.alerts {
		alerts = append(alerts, alert)
	}

	return alerts
}

// GetAlertHistory returns alert history for a specific rule
func (as *AlertingService) GetAlertHistory(ruleID string) []*Alert {
	as.alertsMutex.RLock()
	defer as.alertsMutex.RUnlock()

	var history []*Alert
	for _, alert := range as.alerts {
		if alert.RuleID == ruleID {
			history = append(history, alert)
		}
	}

	return history
}

// GetMetrics returns alerting service metrics
func (as *AlertingService) GetMetrics() map[string]interface{} {
	as.rulesMutex.RLock()
	rulesCount := len(as.rules)
	as.rulesMutex.RUnlock()

	as.alertsMutex.RLock()
	activeCount := 0
	resolvedCount := 0
	for _, alert := range as.alerts {
		if alert.State == AlertStateFiring {
			activeCount++
		} else if alert.State == AlertStateResolved {
			resolvedCount++
		}
	}
	as.alertsMutex.RUnlock()

	return map[string]interface{}{
		"rules_total":     rulesCount,
		"alerts_active":   activeCount,
		"alerts_resolved": resolvedCount,
		"alerts_total":    activeCount + resolvedCount,
	}
}

// HealthCheck performs a health check on the alerting service
func (as *AlertingService) HealthCheck(ctx context.Context) error {
	if !as.running {
		return fmt.Errorf("alerting service is not running")
	}

	// Check if evaluation is working
	as.rulesMutex.RLock()
	hasRules := len(as.rules) > 0
	as.rulesMutex.RUnlock()

	if hasRules {
		// Check if rules have been evaluated recently
		for _, rule := range as.rules {
			if rule.Enabled && time.Since(rule.LastEvaluated) > 2*as.config.EvaluationInterval {
				return fmt.Errorf("rule %s has not been evaluated recently", rule.ID)
			}
		}
	}

	return nil
}