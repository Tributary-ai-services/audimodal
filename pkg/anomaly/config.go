package anomaly

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ConfigurationManager manages anomaly detection configuration
type ConfigurationManager struct {
	config          *GlobalConfig
	detectorConfigs map[string]*DetectorConfig
	tenantConfigs   map[uuid.UUID]*TenantConfig
	ruleConfigs     map[uuid.UUID]*AlertRule
	repository      ConfigRepository
	validator       *ConfigValidator
	hotReload       bool
	watchers        []ConfigWatcher
	mutex           sync.RWMutex
	lastUpdate      time.Time
}

// ConfigRepository defines the interface for configuration persistence
type ConfigRepository interface {
	// Global configuration
	SaveGlobalConfig(ctx context.Context, config *GlobalConfig) error
	LoadGlobalConfig(ctx context.Context) (*GlobalConfig, error)

	// Detector configurations
	SaveDetectorConfig(ctx context.Context, detectorName string, config *DetectorConfig) error
	LoadDetectorConfig(ctx context.Context, detectorName string) (*DetectorConfig, error)
	LoadAllDetectorConfigs(ctx context.Context) (map[string]*DetectorConfig, error)

	// Tenant configurations
	SaveTenantConfig(ctx context.Context, tenantID uuid.UUID, config *TenantConfig) error
	LoadTenantConfig(ctx context.Context, tenantID uuid.UUID) (*TenantConfig, error)
	LoadAllTenantConfigs(ctx context.Context) (map[uuid.UUID]*TenantConfig, error)

	// Alert rule configurations
	SaveAlertRule(ctx context.Context, rule *AlertRule) error
	LoadAlertRule(ctx context.Context, ruleID uuid.UUID) (*AlertRule, error)
	LoadAlertRulesByTenant(ctx context.Context, tenantID uuid.UUID) ([]*AlertRule, error)
	DeleteAlertRule(ctx context.Context, ruleID uuid.UUID) error

	// Configuration versioning
	SaveConfigVersion(ctx context.Context, version *ConfigVersion) error
	GetConfigHistory(ctx context.Context, configType string, entityID string) ([]*ConfigVersion, error)

	// Bulk operations
	ExportConfiguration(ctx context.Context) (*ConfigurationExport, error)
	ImportConfiguration(ctx context.Context, export *ConfigurationExport) error
}

// GlobalConfig contains global anomaly detection configuration
type GlobalConfig struct {
	Version     string    `json:"version"`
	LastUpdated time.Time `json:"last_updated"`
	UpdatedBy   uuid.UUID `json:"updated_by"`

	// Detection settings
	DetectionEnabled  bool            `json:"detection_enabled"`
	DefaultSeverity   AnomalySeverity `json:"default_severity"`
	MinConfidence     float64         `json:"min_confidence"`
	ProcessingTimeout time.Duration   `json:"processing_timeout"`
	MaxConcurrentJobs int             `json:"max_concurrent_jobs"`

	// Storage settings
	RetentionPolicy *RetentionPolicy `json:"retention_policy"`
	StorageQuotas   map[string]int64 `json:"storage_quotas"`

	// Alerting settings
	AlertingEnabled      bool                   `json:"alerting_enabled"`
	DefaultAlertChannels []string               `json:"default_alert_channels"`
	AlertThrottling      *AlertThrottlingConfig `json:"alert_throttling"`

	// Security settings
	SecurityEnabled   bool `json:"security_enabled"`
	EncryptionEnabled bool `json:"encryption_enabled"`
	AuditLogging      bool `json:"audit_logging"`

	// Performance settings
	CacheEnabled bool          `json:"cache_enabled"`
	CacheTTL     time.Duration `json:"cache_ttl"`
	BatchSize    int           `json:"batch_size"`

	// Feature flags
	FeatureFlags map[string]bool `json:"feature_flags"`

	// External integrations
	Integrations map[string]IntegrationConfig `json:"integrations"`
}

// TenantConfig contains tenant-specific configuration
type TenantConfig struct {
	TenantID    uuid.UUID `json:"tenant_id"`
	TenantName  string    `json:"tenant_name"`
	Version     string    `json:"version"`
	LastUpdated time.Time `json:"last_updated"`
	UpdatedBy   uuid.UUID `json:"updated_by"`

	// Detection overrides
	DetectionEnabled  *bool    `json:"detection_enabled,omitempty"`
	MinConfidence     *float64 `json:"min_confidence,omitempty"`
	EnabledDetectors  []string `json:"enabled_detectors,omitempty"`
	DisabledDetectors []string `json:"disabled_detectors,omitempty"`

	// Thresholds
	TypeThresholds     map[AnomalyType]float64     `json:"type_thresholds,omitempty"`
	SeverityThresholds map[AnomalySeverity]float64 `json:"severity_thresholds,omitempty"`

	// Alerting configuration
	AlertingEnabled   *bool               `json:"alerting_enabled,omitempty"`
	AlertChannels     []string            `json:"alert_channels,omitempty"`
	NotificationRules []*NotificationRule `json:"notification_rules,omitempty"`

	// Custom settings
	CustomThresholds map[string]float64     `json:"custom_thresholds,omitempty"`
	CustomSettings   map[string]interface{} `json:"custom_settings,omitempty"`

	// Business rules
	BusinessHours *BusinessHours `json:"business_hours,omitempty"`
	Timezone      string         `json:"timezone,omitempty"`

	// Compliance settings
	ComplianceMode    string `json:"compliance_mode,omitempty"`
	DataRetentionDays *int   `json:"data_retention_days,omitempty"`

	// Resource limits
	ResourceLimits *TenantResourceLimits `json:"resource_limits,omitempty"`
}

// RetentionPolicy defines data retention policies
type RetentionPolicy struct {
	AnomalyRetentionDays int `json:"anomaly_retention_days"`
	AlertRetentionDays   int `json:"alert_retention_days"`
	MetricsRetentionDays int `json:"metrics_retention_days"`
	ArchiveAfterDays     int `json:"archive_after_days"`
	DeleteAfterDays      int `json:"delete_after_days"`
}

// AlertThrottlingConfig defines alert throttling configuration
type AlertThrottlingConfig struct {
	MaxAlertsPerMinute  int           `json:"max_alerts_per_minute"`
	MaxAlertsPerHour    int           `json:"max_alerts_per_hour"`
	DeduplicationWindow time.Duration `json:"deduplication_window"`
	CooldownPeriod      time.Duration `json:"cooldown_period"`
}

// IntegrationConfig defines external integration configuration
type IntegrationConfig struct {
	Enabled    bool                   `json:"enabled"`
	Endpoint   string                 `json:"endpoint"`
	APIKey     string                 `json:"api_key,omitempty"`
	Timeout    time.Duration          `json:"timeout"`
	RetryCount int                    `json:"retry_count"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

// NotificationRule defines when and how to send notifications
type NotificationRule struct {
	ID         uuid.UUID               `json:"id"`
	Name       string                  `json:"name"`
	Enabled    bool                    `json:"enabled"`
	Conditions []NotificationCondition `json:"conditions"`
	Channels   []string                `json:"channels"`
	Recipients []AlertRecipient        `json:"recipients"`
	Template   string                  `json:"template,omitempty"`
	Priority   int                     `json:"priority"`
	Frequency  string                  `json:"frequency"` // immediate, batch, digest
	Schedule   *NotificationSchedule   `json:"schedule,omitempty"`
}

// NotificationCondition defines conditions for triggering notifications
type NotificationCondition struct {
	Field    string      `json:"field"`
	Operator string      `json:"operator"`
	Value    interface{} `json:"value"`
}

// NotificationSchedule defines when notifications can be sent
type NotificationSchedule struct {
	ActiveHours []int      `json:"active_hours"`
	ActiveDays  []int      `json:"active_days"`
	Timezone    string     `json:"timezone"`
	StartDate   *time.Time `json:"start_date,omitempty"`
	EndDate     *time.Time `json:"end_date,omitempty"`
}

// BusinessHours defines business hours for a tenant
type BusinessHours struct {
	Monday    *TimeRange `json:"monday,omitempty"`
	Tuesday   *TimeRange `json:"tuesday,omitempty"`
	Wednesday *TimeRange `json:"wednesday,omitempty"`
	Thursday  *TimeRange `json:"thursday,omitempty"`
	Friday    *TimeRange `json:"friday,omitempty"`
	Saturday  *TimeRange `json:"saturday,omitempty"`
	Sunday    *TimeRange `json:"sunday,omitempty"`
	Holidays  []Holiday  `json:"holidays,omitempty"`
}

// TimeRange defines a time range within a day
type TimeRange struct {
	Start string `json:"start"` // HH:MM format
	End   string `json:"end"`   // HH:MM format
}

// Holiday defines a holiday date
type Holiday struct {
	Date        time.Time `json:"date"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
}

// TenantResourceLimits defines resource limits for a tenant
type TenantResourceLimits struct {
	MaxAnomaliesPerDay   int   `json:"max_anomalies_per_day"`
	MaxAlertsPerDay      int   `json:"max_alerts_per_day"`
	MaxStorageMB         int64 `json:"max_storage_mb"`
	MaxConcurrentJobs    int   `json:"max_concurrent_jobs"`
	MaxDetectionRequests int   `json:"max_detection_requests"`
}

// ConfigVersion represents a version of configuration changes
type ConfigVersion struct {
	ID           uuid.UUID              `json:"id"`
	ConfigType   string                 `json:"config_type"` // global, detector, tenant, rule
	EntityID     string                 `json:"entity_id"`
	Version      string                 `json:"version"`
	Changes      map[string]interface{} `json:"changes"`
	PreviousHash string                 `json:"previous_hash"`
	CurrentHash  string                 `json:"current_hash"`
	CreatedAt    time.Time              `json:"created_at"`
	CreatedBy    uuid.UUID              `json:"created_by"`
	Comment      string                 `json:"comment,omitempty"`
}

// ConfigurationExport represents an exported configuration
type ConfigurationExport struct {
	ExportID        uuid.UUID                  `json:"export_id"`
	ExportedAt      time.Time                  `json:"exported_at"`
	ExportedBy      uuid.UUID                  `json:"exported_by"`
	Version         string                     `json:"version"`
	GlobalConfig    *GlobalConfig              `json:"global_config"`
	DetectorConfigs map[string]*DetectorConfig `json:"detector_configs"`
	TenantConfigs   map[string]*TenantConfig   `json:"tenant_configs"`
	AlertRules      map[string]*AlertRule      `json:"alert_rules"`
	Metadata        map[string]interface{}     `json:"metadata"`
}

// ConfigWatcher defines the interface for configuration change watchers
type ConfigWatcher interface {
	OnConfigChange(ctx context.Context, change *ConfigChange) error
	GetWatchedTypes() []string
}

// ConfigChange represents a configuration change event
type ConfigChange struct {
	Type      string                 `json:"type"`
	EntityID  string                 `json:"entity_id"`
	OldConfig interface{}            `json:"old_config"`
	NewConfig interface{}            `json:"new_config"`
	Changes   map[string]interface{} `json:"changes"`
	Timestamp time.Time              `json:"timestamp"`
	ChangedBy uuid.UUID              `json:"changed_by"`
}

// ConfigValidator validates configuration objects
type ConfigValidator struct {
	rules map[string][]ValidationRule
}

// ValidationRule defines a validation rule
type ValidationRule struct {
	Field           string                  `json:"field"`
	Required        bool                    `json:"required"`
	Type            string                  `json:"type"`
	MinValue        *float64                `json:"min_value,omitempty"`
	MaxValue        *float64                `json:"max_value,omitempty"`
	AllowedValues   []interface{}           `json:"allowed_values,omitempty"`
	Pattern         string                  `json:"pattern,omitempty"`
	CustomValidator func(interface{}) error `json:"-"`
}

// ValidationError represents a configuration validation error
type ValidationError struct {
	Field   string      `json:"field"`
	Message string      `json:"message"`
	Value   interface{} `json:"value,omitempty"`
}

// NewConfigurationManager creates a new configuration manager
func NewConfigurationManager(repository ConfigRepository, hotReload bool) *ConfigurationManager {
	return &ConfigurationManager{
		repository:      repository,
		detectorConfigs: make(map[string]*DetectorConfig),
		tenantConfigs:   make(map[uuid.UUID]*TenantConfig),
		ruleConfigs:     make(map[uuid.UUID]*AlertRule),
		hotReload:       hotReload,
		watchers:        make([]ConfigWatcher, 0),
		validator:       NewConfigValidator(),
		lastUpdate:      time.Now(),
	}
}

// LoadConfiguration loads all configuration from the repository
func (cm *ConfigurationManager) LoadConfiguration(ctx context.Context) error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	// Load global configuration
	globalConfig, err := cm.repository.LoadGlobalConfig(ctx)
	if err != nil {
		log.Printf("Failed to load global config: %v", err)
		// Use default configuration
		globalConfig = cm.getDefaultGlobalConfig()
	}
	cm.config = globalConfig

	// Load detector configurations
	detectorConfigs, err := cm.repository.LoadAllDetectorConfigs(ctx)
	if err != nil {
		log.Printf("Failed to load detector configs: %v", err)
	} else {
		cm.detectorConfigs = detectorConfigs
	}

	// Load tenant configurations
	tenantConfigs, err := cm.repository.LoadAllTenantConfigs(ctx)
	if err != nil {
		log.Printf("Failed to load tenant configs: %v", err)
	} else {
		cm.tenantConfigs = tenantConfigs
	}

	cm.lastUpdate = time.Now()
	log.Printf("Configuration loaded successfully at %s", cm.lastUpdate.Format(time.RFC3339))

	return nil
}

// GetGlobalConfig returns the global configuration
func (cm *ConfigurationManager) GetGlobalConfig(ctx context.Context) *GlobalConfig {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()
	return cm.config
}

// UpdateGlobalConfig updates the global configuration
func (cm *ConfigurationManager) UpdateGlobalConfig(ctx context.Context, config *GlobalConfig, updatedBy uuid.UUID) error {
	// Validate configuration
	if err := cm.validator.ValidateGlobalConfig(config); err != nil {
		return fmt.Errorf("invalid global configuration: %w", err)
	}

	cm.mutex.Lock()
	oldConfig := cm.config
	config.LastUpdated = time.Now()
	config.UpdatedBy = updatedBy
	cm.config = config
	cm.mutex.Unlock()

	// Save to repository
	if err := cm.repository.SaveGlobalConfig(ctx, config); err != nil {
		// Rollback on save failure
		cm.mutex.Lock()
		cm.config = oldConfig
		cm.mutex.Unlock()
		return fmt.Errorf("failed to save global configuration: %w", err)
	}

	// Notify watchers
	change := &ConfigChange{
		Type:      "global",
		EntityID:  "global",
		OldConfig: oldConfig,
		NewConfig: config,
		Timestamp: time.Now(),
		ChangedBy: updatedBy,
	}
	cm.notifyWatchers(ctx, change)

	log.Printf("Global configuration updated by %s", updatedBy)
	return nil
}

// GetDetectorConfig returns configuration for a specific detector
func (cm *ConfigurationManager) GetDetectorConfig(ctx context.Context, detectorName string) *DetectorConfig {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	if config, exists := cm.detectorConfigs[detectorName]; exists {
		return config
	}

	// Return default configuration if not found
	return cm.getDefaultDetectorConfig(detectorName)
}

// UpdateDetectorConfig updates configuration for a specific detector
func (cm *ConfigurationManager) UpdateDetectorConfig(ctx context.Context, detectorName string, config *DetectorConfig, updatedBy uuid.UUID) error {
	// Validate configuration
	if err := cm.validator.ValidateDetectorConfig(config); err != nil {
		return fmt.Errorf("invalid detector configuration: %w", err)
	}

	cm.mutex.Lock()
	oldConfig := cm.detectorConfigs[detectorName]
	config.UpdateInterval = time.Now().Sub(time.Time{}) // Set update interval
	cm.detectorConfigs[detectorName] = config
	cm.mutex.Unlock()

	// Save to repository
	if err := cm.repository.SaveDetectorConfig(ctx, detectorName, config); err != nil {
		// Rollback on save failure
		cm.mutex.Lock()
		if oldConfig != nil {
			cm.detectorConfigs[detectorName] = oldConfig
		} else {
			delete(cm.detectorConfigs, detectorName)
		}
		cm.mutex.Unlock()
		return fmt.Errorf("failed to save detector configuration: %w", err)
	}

	// Notify watchers
	change := &ConfigChange{
		Type:      "detector",
		EntityID:  detectorName,
		OldConfig: oldConfig,
		NewConfig: config,
		Timestamp: time.Now(),
		ChangedBy: updatedBy,
	}
	cm.notifyWatchers(ctx, change)

	log.Printf("Detector configuration updated for %s by %s", detectorName, updatedBy)
	return nil
}

// GetTenantConfig returns configuration for a specific tenant
func (cm *ConfigurationManager) GetTenantConfig(ctx context.Context, tenantID uuid.UUID) *TenantConfig {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	if config, exists := cm.tenantConfigs[tenantID]; exists {
		return config
	}

	// Return default configuration if not found
	return cm.getDefaultTenantConfig(tenantID)
}

// UpdateTenantConfig updates configuration for a specific tenant
func (cm *ConfigurationManager) UpdateTenantConfig(ctx context.Context, tenantID uuid.UUID, config *TenantConfig, updatedBy uuid.UUID) error {
	// Validate configuration
	if err := cm.validator.ValidateTenantConfig(config); err != nil {
		return fmt.Errorf("invalid tenant configuration: %w", err)
	}

	cm.mutex.Lock()
	oldConfig := cm.tenantConfigs[tenantID]
	config.TenantID = tenantID
	config.LastUpdated = time.Now()
	config.UpdatedBy = updatedBy
	cm.tenantConfigs[tenantID] = config
	cm.mutex.Unlock()

	// Save to repository
	if err := cm.repository.SaveTenantConfig(ctx, tenantID, config); err != nil {
		// Rollback on save failure
		cm.mutex.Lock()
		if oldConfig != nil {
			cm.tenantConfigs[tenantID] = oldConfig
		} else {
			delete(cm.tenantConfigs, tenantID)
		}
		cm.mutex.Unlock()
		return fmt.Errorf("failed to save tenant configuration: %w", err)
	}

	// Notify watchers
	change := &ConfigChange{
		Type:      "tenant",
		EntityID:  tenantID.String(),
		OldConfig: oldConfig,
		NewConfig: config,
		Timestamp: time.Now(),
		ChangedBy: updatedBy,
	}
	cm.notifyWatchers(ctx, change)

	log.Printf("Tenant configuration updated for %s by %s", tenantID, updatedBy)
	return nil
}

// RegisterWatcher registers a configuration change watcher
func (cm *ConfigurationManager) RegisterWatcher(watcher ConfigWatcher) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()
	cm.watchers = append(cm.watchers, watcher)
	log.Printf("Registered configuration watcher for types: %v", watcher.GetWatchedTypes())
}

// ExportConfiguration exports all configuration to a single object
func (cm *ConfigurationManager) ExportConfiguration(ctx context.Context, exportedBy uuid.UUID) (*ConfigurationExport, error) {
	export, err := cm.repository.ExportConfiguration(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to export configuration: %w", err)
	}

	export.ExportID = uuid.New()
	export.ExportedAt = time.Now()
	export.ExportedBy = exportedBy

	return export, nil
}

// ImportConfiguration imports configuration from an export object
func (cm *ConfigurationManager) ImportConfiguration(ctx context.Context, export *ConfigurationExport, importedBy uuid.UUID) error {
	// Validate export
	if err := cm.validator.ValidateConfigurationExport(export); err != nil {
		return fmt.Errorf("invalid configuration export: %w", err)
	}

	// Import configuration
	if err := cm.repository.ImportConfiguration(ctx, export); err != nil {
		return fmt.Errorf("failed to import configuration: %w", err)
	}

	// Reload configuration
	if err := cm.LoadConfiguration(ctx); err != nil {
		return fmt.Errorf("failed to reload configuration after import: %w", err)
	}

	log.Printf("Configuration imported successfully by %s", importedBy)
	return nil
}

// Internal methods

func (cm *ConfigurationManager) notifyWatchers(ctx context.Context, change *ConfigChange) {
	for _, watcher := range cm.watchers {
		watchedTypes := watcher.GetWatchedTypes()
		for _, watchedType := range watchedTypes {
			if watchedType == change.Type || watchedType == "*" {
				go func(w ConfigWatcher) {
					if err := w.OnConfigChange(ctx, change); err != nil {
						log.Printf("Configuration watcher error: %v", err)
					}
				}(watcher)
				break
			}
		}
	}
}

func (cm *ConfigurationManager) getDefaultGlobalConfig() *GlobalConfig {
	return &GlobalConfig{
		Version:              "1.0.0",
		LastUpdated:          time.Now(),
		DetectionEnabled:     true,
		DefaultSeverity:      SeverityMedium,
		MinConfidence:        0.5,
		ProcessingTimeout:    30 * time.Second,
		MaxConcurrentJobs:    10,
		AlertingEnabled:      true,
		DefaultAlertChannels: []string{"email"},
		SecurityEnabled:      true,
		EncryptionEnabled:    true,
		AuditLogging:         true,
		CacheEnabled:         true,
		CacheTTL:             1 * time.Hour,
		BatchSize:            100,
		FeatureFlags:         make(map[string]bool),
		Integrations:         make(map[string]IntegrationConfig),
		RetentionPolicy: &RetentionPolicy{
			AnomalyRetentionDays: 90,
			AlertRetentionDays:   30,
			MetricsRetentionDays: 7,
			ArchiveAfterDays:     30,
			DeleteAfterDays:      365,
		},
		AlertThrottling: &AlertThrottlingConfig{
			MaxAlertsPerMinute:  10,
			MaxAlertsPerHour:    100,
			DeduplicationWindow: 5 * time.Minute,
			CooldownPeriod:      15 * time.Minute,
		},
	}
}

func (cm *ConfigurationManager) getDefaultDetectorConfig(detectorName string) *DetectorConfig {
	return &DetectorConfig{
		Name:               detectorName,
		Enabled:            true,
		Priority:           50,
		Sensitivity:        0.5,
		Thresholds:         make(map[string]float64),
		Parameters:         make(map[string]interface{}),
		UpdateInterval:     24 * time.Hour,
		RequiredSampleSize: 100,
	}
}

func (cm *ConfigurationManager) getDefaultTenantConfig(tenantID uuid.UUID) *TenantConfig {
	return &TenantConfig{
		TenantID:         tenantID,
		Version:          "1.0.0",
		LastUpdated:      time.Now(),
		Timezone:         "UTC",
		CustomThresholds: make(map[string]float64),
		CustomSettings:   make(map[string]interface{}),
		ResourceLimits: &TenantResourceLimits{
			MaxAnomaliesPerDay:   1000,
			MaxAlertsPerDay:      100,
			MaxStorageMB:         1024,
			MaxConcurrentJobs:    5,
			MaxDetectionRequests: 10000,
		},
	}
}

// NewConfigValidator creates a new configuration validator
func NewConfigValidator() *ConfigValidator {
	validator := &ConfigValidator{
		rules: make(map[string][]ValidationRule),
	}
	validator.initializeValidationRules()
	return validator
}

func (cv *ConfigValidator) initializeValidationRules() {
	// Global configuration validation rules
	cv.rules["global"] = []ValidationRule{
		{Field: "min_confidence", Required: true, Type: "float", MinValue: &[]float64{0.0}[0], MaxValue: &[]float64{1.0}[0]},
		{Field: "processing_timeout", Required: true, Type: "duration"},
		{Field: "max_concurrent_jobs", Required: true, Type: "int", MinValue: &[]float64{1}[0], MaxValue: &[]float64{100}[0]},
	}

	// Detector configuration validation rules
	cv.rules["detector"] = []ValidationRule{
		{Field: "enabled", Required: true, Type: "bool"},
		{Field: "priority", Required: true, Type: "int", MinValue: &[]float64{1}[0], MaxValue: &[]float64{100}[0]},
		{Field: "sensitivity", Required: true, Type: "float", MinValue: &[]float64{0.0}[0], MaxValue: &[]float64{1.0}[0]},
	}

	// Tenant configuration validation rules
	cv.rules["tenant"] = []ValidationRule{
		{Field: "tenant_id", Required: true, Type: "uuid"},
		{Field: "timezone", Required: false, Type: "string"},
	}
}

func (cv *ConfigValidator) ValidateGlobalConfig(config *GlobalConfig) error {
	return cv.validateConfig("global", config)
}

func (cv *ConfigValidator) ValidateDetectorConfig(config *DetectorConfig) error {
	return cv.validateConfig("detector", config)
}

func (cv *ConfigValidator) ValidateTenantConfig(config *TenantConfig) error {
	return cv.validateConfig("tenant", config)
}

func (cv *ConfigValidator) ValidateConfigurationExport(export *ConfigurationExport) error {
	if export.GlobalConfig != nil {
		if err := cv.ValidateGlobalConfig(export.GlobalConfig); err != nil {
			return fmt.Errorf("invalid global config in export: %w", err)
		}
	}

	for name, config := range export.DetectorConfigs {
		if err := cv.ValidateDetectorConfig(config); err != nil {
			return fmt.Errorf("invalid detector config %s in export: %w", name, err)
		}
	}

	for tenantID, config := range export.TenantConfigs {
		if err := cv.ValidateTenantConfig(config); err != nil {
			return fmt.Errorf("invalid tenant config %s in export: %w", tenantID, err)
		}
	}

	return nil
}

func (cv *ConfigValidator) validateConfig(configType string, config interface{}) error {
	rules, exists := cv.rules[configType]
	if !exists {
		return nil // No validation rules defined
	}

	// Convert config to map for validation
	configBytes, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config for validation: %w", err)
	}

	var configMap map[string]interface{}
	if err := json.Unmarshal(configBytes, &configMap); err != nil {
		return fmt.Errorf("failed to unmarshal config for validation: %w", err)
	}

	var validationErrors []ValidationError

	// Apply validation rules
	for _, rule := range rules {
		value, exists := configMap[rule.Field]

		// Check required fields
		if rule.Required && !exists {
			validationErrors = append(validationErrors, ValidationError{
				Field:   rule.Field,
				Message: "field is required",
			})
			continue
		}

		if !exists {
			continue // Skip optional fields that are not present
		}

		// Type validation
		if err := cv.validateFieldType(rule, value); err != nil {
			validationErrors = append(validationErrors, ValidationError{
				Field:   rule.Field,
				Message: err.Error(),
				Value:   value,
			})
		}

		// Range validation
		if err := cv.validateFieldRange(rule, value); err != nil {
			validationErrors = append(validationErrors, ValidationError{
				Field:   rule.Field,
				Message: err.Error(),
				Value:   value,
			})
		}

		// Custom validation
		if rule.CustomValidator != nil {
			if err := rule.CustomValidator(value); err != nil {
				validationErrors = append(validationErrors, ValidationError{
					Field:   rule.Field,
					Message: err.Error(),
					Value:   value,
				})
			}
		}
	}

	if len(validationErrors) > 0 {
		return fmt.Errorf("validation errors: %+v", validationErrors)
	}

	return nil
}

func (cv *ConfigValidator) validateFieldType(rule ValidationRule, value interface{}) error {
	switch rule.Type {
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("expected string, got %T", value)
		}
	case "int":
		if _, ok := value.(float64); !ok { // JSON numbers are float64
			return fmt.Errorf("expected int, got %T", value)
		}
	case "float":
		if _, ok := value.(float64); !ok {
			return fmt.Errorf("expected float, got %T", value)
		}
	case "bool":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("expected bool, got %T", value)
		}
	case "uuid":
		if str, ok := value.(string); ok {
			if _, err := uuid.Parse(str); err != nil {
				return fmt.Errorf("invalid UUID format: %s", str)
			}
		} else {
			return fmt.Errorf("expected UUID string, got %T", value)
		}
	}
	return nil
}

func (cv *ConfigValidator) validateFieldRange(rule ValidationRule, value interface{}) error {
	if floatVal, ok := value.(float64); ok {
		if rule.MinValue != nil && floatVal < *rule.MinValue {
			return fmt.Errorf("value %f is below minimum %f", floatVal, *rule.MinValue)
		}
		if rule.MaxValue != nil && floatVal > *rule.MaxValue {
			return fmt.Errorf("value %f is above maximum %f", floatVal, *rule.MaxValue)
		}
	}
	return nil
}

// GetConfigurationSummary returns a summary of the current configuration
func (cm *ConfigurationManager) GetConfigurationSummary(ctx context.Context) map[string]interface{} {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	return map[string]interface{}{
		"global_config_version":  cm.config.Version,
		"last_updated":           cm.lastUpdate,
		"detector_configs_count": len(cm.detectorConfigs),
		"tenant_configs_count":   len(cm.tenantConfigs),
		"alert_rules_count":      len(cm.ruleConfigs),
		"hot_reload_enabled":     cm.hotReload,
		"watchers_count":         len(cm.watchers),
		"detection_enabled":      cm.config.DetectionEnabled,
		"alerting_enabled":       cm.config.AlertingEnabled,
		"security_enabled":       cm.config.SecurityEnabled,
	}
}
