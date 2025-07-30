package training

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// ModelRegistry manages model versions and deployment
type ModelRegistry struct {
	models    map[string]*ModelEntry
	versions  map[string]map[string]*ModelVersion
	aliases   map[string]*ModelAlias
	tracer    trace.Tracer
	mutex     sync.RWMutex
}

// ModelVersionManager handles model versioning and A/B testing
type ModelVersionManager struct {
	registry       *ModelRegistry
	experiments    map[uuid.UUID]*ABTestExperiment
	deployments    map[uuid.UUID]*ModelDeployment
	trafficSplits  map[string]*TrafficSplit
	metrics        *VersionMetrics
	tracer         trace.Tracer
	mutex          sync.RWMutex
}

// ModelEntry represents a model in the registry
type ModelEntry struct {
	Name           string                    `json:"name"`
	Description    string                    `json:"description"`
	ModelType      ModelType                 `json:"model_type"`
	TenantID       uuid.UUID                 `json:"tenant_id"`
	Owner          uuid.UUID                 `json:"owner"`
	CreatedAt      time.Time                 `json:"created_at"`
	UpdatedAt      time.Time                 `json:"updated_at"`
	LatestVersion  string                    `json:"latest_version"`
	ProductionVersion string                 `json:"production_version,omitempty"`
	Versions       []string                  `json:"versions"`
	Tags           []string                  `json:"tags"`
	Metadata       map[string]interface{}    `json:"metadata"`
	Status         ModelRegistryStatus       `json:"status"`
}

// ModelVersion represents a specific version of a model
type ModelVersion struct {
	ID                 uuid.UUID                 `json:"id"`
	ModelName          string                    `json:"model_name"`
	Version            string                    `json:"version"`
	Description        string                    `json:"description"`
	CreatedAt          time.Time                 `json:"created_at"`
	CreatedBy          uuid.UUID                 `json:"created_by"`
	TrainingJobID      uuid.UUID                 `json:"training_job_id"`
	Status             ModelVersionStatus        `json:"status"`
	
	// Model artifacts
	Artifacts          []ModelArtifact           `json:"artifacts"`
	ModelSize          int64                     `json:"model_size_bytes"`
	ModelChecksum      string                    `json:"model_checksum"`
	
	// Configuration and metadata
	Configuration      *ModelConfiguration       `json:"configuration"`
	TrainingParams     *TrainingParameters       `json:"training_params"`
	TrainingDataset    *DatasetInfo              `json:"training_dataset"`
	
	// Performance metrics
	ValidationMetrics  *ValidationMetrics        `json:"validation_metrics"`
	BenchmarkResults   *BenchmarkResults         `json:"benchmark_results,omitempty"`
	
	// Deployment information
	DeploymentInfo     *DeploymentInfo           `json:"deployment_info,omitempty"`
	CompatibilityInfo  *CompatibilityInfo        `json:"compatibility_info"`
	
	// Version lineage
	ParentVersion      string                    `json:"parent_version,omitempty"`
	ChildVersions      []string                  `json:"child_versions,omitempty"`
	MergeSource        []string                  `json:"merge_source,omitempty"`
	
	// Approval and governance
	ApprovalStatus     ApprovalStatus            `json:"approval_status"`
	ApprovedBy         *uuid.UUID                `json:"approved_by,omitempty"`
	ApprovedAt         *time.Time                `json:"approved_at,omitempty"`
	ReviewComments     []ReviewComment           `json:"review_comments,omitempty"`
	
	// Usage tracking
	DeploymentCount    int                       `json:"deployment_count"`
	InferenceCount     int64                     `json:"inference_count"`
	LastUsed           *time.Time                `json:"last_used,omitempty"`
	
	// Metadata
	Tags               []string                  `json:"tags"`
	Labels             map[string]string         `json:"labels"`
	Metadata           map[string]interface{}    `json:"metadata"`
}

// ModelAlias represents an alias pointing to a specific model version
type ModelAlias struct {
	Name         string                    `json:"name"`
	ModelName    string                    `json:"model_name"`
	Version      string                    `json:"version"`
	Description  string                    `json:"description"`
	CreatedAt    time.Time                 `json:"created_at"`
	UpdatedAt    time.Time                 `json:"updated_at"`
	CreatedBy    uuid.UUID                 `json:"created_by"`
	IsProduction bool                      `json:"is_production"`
	Metadata     map[string]interface{}    `json:"metadata"`
}

// ABTestExperiment represents an A/B testing experiment
type ABTestExperiment struct {
	ID              uuid.UUID                 `json:"id"`
	Name            string                    `json:"name"`
	Description     string                    `json:"description"`
	TenantID        uuid.UUID                 `json:"tenant_id"`
	ModelName       string                    `json:"model_name"`
	Status          ExperimentStatus          `json:"status"`
	CreatedAt       time.Time                 `json:"created_at"`
	StartedAt       *time.Time                `json:"started_at,omitempty"`
	EndedAt         *time.Time                `json:"ended_at,omitempty"`
	CreatedBy       uuid.UUID                 `json:"created_by"`
	
	// Experiment design
	ControlVersion  string                    `json:"control_version"`
	TreatmentVersions []TreatmentVersion      `json:"treatment_versions"`
	TrafficSplit    TrafficSplitConfig        `json:"traffic_split"`
	
	// Experiment configuration
	Duration        time.Duration             `json:"duration"`
	SampleSize      int                       `json:"sample_size"`
	SignificanceLevel float64                 `json:"significance_level"`
	PowerLevel      float64                   `json:"power_level"`
	MinEffectSize   float64                   `json:"min_effect_size"`
	
	// Metrics and objectives
	PrimaryMetric   ExperimentMetric          `json:"primary_metric"`
	SecondaryMetrics []ExperimentMetric       `json:"secondary_metrics"`
	GuardrailMetrics []GuardrailMetric        `json:"guardrail_metrics"`
	
	// Results
	Results         *ExperimentResults        `json:"results,omitempty"`
	Analysis        *StatisticalAnalysis      `json:"analysis,omitempty"`
	Recommendation  *ExperimentRecommendation `json:"recommendation,omitempty"`
	
	// Metadata
	Tags            []string                  `json:"tags"`
	Metadata        map[string]interface{}    `json:"metadata"`
}

// TreatmentVersion represents a treatment version in an A/B test
type TreatmentVersion struct {
	Version     string  `json:"version"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	TrafficPercentage float64 `json:"traffic_percentage"`
}

// TrafficSplit represents traffic splitting configuration
type TrafficSplit struct {
	ID            uuid.UUID                 `json:"id"`
	ModelName     string                    `json:"model_name"`
	ExperimentID  *uuid.UUID                `json:"experiment_id,omitempty"`
	Rules         []TrafficRule             `json:"rules"`
	DefaultVersion string                   `json:"default_version"`
	CreatedAt     time.Time                 `json:"created_at"`
	UpdatedAt     time.Time                 `json:"updated_at"`
	Status        TrafficSplitStatus        `json:"status"`
}

// TrafficRule represents a traffic routing rule
type TrafficRule struct {
	Version       string                    `json:"version"`
	Percentage    float64                   `json:"percentage"`
	Conditions    []RoutingCondition        `json:"conditions,omitempty"`
	Priority      int                       `json:"priority"`
	Enabled       bool                      `json:"enabled"`
}

// RoutingCondition represents a condition for traffic routing
type RoutingCondition struct {
	Field     string      `json:"field"`
	Operator  string      `json:"operator"`
	Value     interface{} `json:"value"`
}

// ModelDeployment represents a model deployment
type ModelDeployment struct {
	ID              uuid.UUID                 `json:"id"`
	ModelName       string                    `json:"model_name"`
	Version         string                    `json:"version"`
	Environment     string                    `json:"environment"`
	Status          DeploymentStatus          `json:"status"`
	CreatedAt       time.Time                 `json:"created_at"`
	DeployedAt      *time.Time                `json:"deployed_at,omitempty"`
	UndeployedAt    *time.Time                `json:"undeployed_at,omitempty"`
	DeployedBy      uuid.UUID                 `json:"deployed_by"`
	
	// Deployment configuration
	DeploymentConfig *DeploymentConfig        `json:"deployment_config"`
	ScalingConfig   *ScalingConfig            `json:"scaling_config"`
	ResourceLimits  *ResourceLimits           `json:"resource_limits"`
	
	// Health and monitoring
	HealthStatus    HealthStatus              `json:"health_status"`
	LastHealthCheck time.Time                 `json:"last_health_check"`
	Endpoints       []DeploymentEndpoint      `json:"endpoints"`
	
	// Performance metrics
	PerformanceMetrics *DeploymentMetrics     `json:"performance_metrics,omitempty"`
	SLAMetrics         *SLAMetrics            `json:"sla_metrics,omitempty"`
	
	// Metadata
	Tags            []string                  `json:"tags"`
	Metadata        map[string]interface{}    `json:"metadata"`
}

// Enums and status types

type ModelRegistryStatus string

const (
	ModelRegistryStatusActive     ModelRegistryStatus = "active"
	ModelRegistryStatusDeprecated ModelRegistryStatus = "deprecated"
	ModelRegistryStatusArchived   ModelRegistryStatus = "archived"
)

type ModelVersionStatus string

const (
	ModelVersionStatusDraft      ModelVersionStatus = "draft"
	ModelVersionStatusTesting    ModelVersionStatus = "testing"
	ModelVersionStatusApproved   ModelVersionStatus = "approved"
	ModelVersionStatusDeployed   ModelVersionStatus = "deployed"
	ModelVersionStatusDeprecated ModelVersionStatus = "deprecated"
	ModelVersionStatusArchived   ModelVersionStatus = "archived"
)

type ApprovalStatus string

const (
	ApprovalStatusPending  ApprovalStatus = "pending"
	ApprovalStatusApproved ApprovalStatus = "approved"
	ApprovalStatusRejected ApprovalStatus = "rejected"
	ApprovalStatusRevision ApprovalStatus = "revision"
)

type ExperimentStatus string

const (
	ExperimentStatusDraft     ExperimentStatus = "draft"
	ExperimentStatusRunning   ExperimentStatus = "running"
	ExperimentStatusCompleted ExperimentStatus = "completed"
	ExperimentStatusCancelled ExperimentStatus = "cancelled"
	ExperimentStatusFailed    ExperimentStatus = "failed"
)

type TrafficSplitStatus string

const (
	TrafficSplitStatusActive   TrafficSplitStatus = "active"
	TrafficSplitStatusInactive TrafficSplitStatus = "inactive"
	TrafficSplitStatusArchived TrafficSplitStatus = "archived"
)

type DeploymentStatus string

const (
	DeploymentStatusPending     DeploymentStatus = "pending"
	DeploymentStatusDeploying   DeploymentStatus = "deploying"
	DeploymentStatusRunning     DeploymentStatus = "running"
	DeploymentStatusFailed      DeploymentStatus = "failed"
	DeploymentStatusTerminating DeploymentStatus = "terminating"
	DeploymentStatusTerminated  DeploymentStatus = "terminated"
)

type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
	HealthStatusUnknown   HealthStatus = "unknown"
)

// Supporting types

type DatasetInfo struct {
	DatasetID   uuid.UUID `json:"dataset_id"`
	DatasetName string    `json:"dataset_name"`
	Version     string    `json:"version"`
	RecordCount int       `json:"record_count"`
	Checksum    string    `json:"checksum"`
}

type CompatibilityInfo struct {
	RuntimeVersion    string                    `json:"runtime_version"`
	FrameworkVersion  string                    `json:"framework_version"`
	PythonVersion     string                    `json:"python_version"`
	Dependencies      []Dependency              `json:"dependencies"`
	RequiredFeatures  []string                  `json:"required_features"`
	BackwardCompatible bool                     `json:"backward_compatible"`
	MinAPIVersion     string                    `json:"min_api_version"`
}

type Dependency struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Source  string `json:"source,omitempty"`
}

type ReviewComment struct {
	ID          uuid.UUID `json:"id"`
	ReviewerID  uuid.UUID `json:"reviewer_id"`
	Comment     string    `json:"comment"`
	CreatedAt   time.Time `json:"created_at"`
	Category    string    `json:"category"`
	Severity    string    `json:"severity"`
	Resolved    bool      `json:"resolved"`
	ResolvedBy  *uuid.UUID `json:"resolved_by,omitempty"`
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
}

type TrafficSplitConfig struct {
	Strategy    string                    `json:"strategy"` // random, user_hash, feature_flag
	Seed        int64                     `json:"seed,omitempty"`
	HashField   string                    `json:"hash_field,omitempty"`
	RampUpSpeed float64                   `json:"ramp_up_speed,omitempty"`
	Conditions  []SplitCondition          `json:"conditions,omitempty"`
}

type SplitCondition struct {
	Field    string      `json:"field"`
	Operator string      `json:"operator"`
	Value    interface{} `json:"value"`
}

type ExperimentMetric struct {
	Name        string                    `json:"name"`
	Type        string                    `json:"type"` // conversion, revenue, latency, accuracy
	Direction   string                    `json:"direction"` // increase, decrease
	Aggregation string                    `json:"aggregation"` // mean, sum, count, percentile
	Filter      map[string]interface{}    `json:"filter,omitempty"`
	Weight      float64                   `json:"weight,omitempty"`
}

type GuardrailMetric struct {
	Name        string  `json:"name"`
	Type        string  `json:"type"`
	Threshold   float64 `json:"threshold"`
	Operator    string  `json:"operator"` // lt, gt, eq
	Action      string  `json:"action"` // stop, alert
}

type ExperimentResults struct {
	SampleSizes      map[string]int            `json:"sample_sizes"`
	MetricResults    map[string]MetricResult   `json:"metric_results"`
	ConversionRates  map[string]float64        `json:"conversion_rates"`
	StatisticalPower float64                   `json:"statistical_power"`
	ConfidenceLevel  float64                   `json:"confidence_level"`
	Duration         time.Duration             `json:"duration"`
	DataQuality      *DataQualityReport        `json:"data_quality"`
}

type MetricResult struct {
	ControlValue      float64           `json:"control_value"`
	TreatmentValues   map[string]float64 `json:"treatment_values"`
	RelativeLift      map[string]float64 `json:"relative_lift"`
	AbsoluteLift      map[string]float64 `json:"absolute_lift"`
	ConfidenceInterval map[string]ConfidenceInterval `json:"confidence_interval"`
	PValue            float64           `json:"p_value"`
	IsSignificant     bool              `json:"is_significant"`
}

type ConfidenceInterval struct {
	Lower float64 `json:"lower"`
	Upper float64 `json:"upper"`
}

type StatisticalAnalysis struct {
	Method           string                    `json:"method"`
	Assumptions      []AssumptionCheck         `json:"assumptions"`
	TestStatistics   map[string]float64        `json:"test_statistics"`
	EffectSizes      map[string]float64        `json:"effect_sizes"`
	PowerAnalysis    *PowerAnalysis            `json:"power_analysis"`
	MultipleComparison *MultipleComparisonCorrection `json:"multiple_comparison,omitempty"`
}

type AssumptionCheck struct {
	Name       string  `json:"name"`
	Satisfied  bool    `json:"satisfied"`
	PValue     float64 `json:"p_value,omitempty"`
	TestStat   float64 `json:"test_stat,omitempty"`
	Details    string  `json:"details,omitempty"`
}

type PowerAnalysis struct {
	ObservedPower    float64 `json:"observed_power"`
	MinDetectableEffect float64 `json:"min_detectable_effect"`
	RequiredSampleSize int     `json:"required_sample_size"`
}

type MultipleComparisonCorrection struct {
	Method        string             `json:"method"`
	AdjustedPValues map[string]float64 `json:"adjusted_p_values"`
	CriticalValue float64             `json:"critical_value"`
}

type ExperimentRecommendation struct {
	Winner          string                    `json:"winner"`
	Confidence      float64                   `json:"confidence"`
	ExpectedLift    float64                   `json:"expected_lift"`
	RecommendedAction string                  `json:"recommended_action"`
	Reasoning       string                    `json:"reasoning"`
	RiskAssessment  *RiskAssessment           `json:"risk_assessment"`
	NextSteps       []string                  `json:"next_steps"`
}

type RiskAssessment struct {
	OverallRisk     string                    `json:"overall_risk"` // low, medium, high
	RiskFactors     []RiskFactor              `json:"risk_factors"`
	Mitigations     []string                  `json:"mitigations"`
}

type RiskFactor struct {
	Factor      string  `json:"factor"`
	Severity    string  `json:"severity"`
	Probability float64 `json:"probability"`
	Impact      string  `json:"impact"`
}

type DataQualityReport struct {
	CompletenessScore float64                   `json:"completeness_score"`
	ConsistencyScore  float64                   `json:"consistency_score"`
	BiasDetection     *BiasDetectionResult      `json:"bias_detection"`
	AnomalyDetection  *AnomalyDetectionResult   `json:"anomaly_detection"`
	Issues            []DataQualityIssue        `json:"issues"`
}

type BiasDetectionResult struct {
	BiasScore       float64                   `json:"bias_score"`
	BiasTypes       []string                  `json:"bias_types"`
	AffectedGroups  []string                  `json:"affected_groups"`
	Recommendations []string                  `json:"recommendations"`
}

type AnomalyDetectionResult struct {
	AnomalyScore    float64                   `json:"anomaly_score"`
	AnomalousPoints []AnomalousDataPoint      `json:"anomalous_points"`
	PatternChanges  []PatternChange           `json:"pattern_changes"`
}

type AnomalousDataPoint struct {
	Timestamp   time.Time                 `json:"timestamp"`
	Value       float64                   `json:"value"`
	ExpectedValue float64                 `json:"expected_value"`
	AnomalyScore float64                  `json:"anomaly_score"`
	Context     map[string]interface{}    `json:"context"`
}

type PatternChange struct {
	Timestamp   time.Time `json:"timestamp"`
	ChangeType  string    `json:"change_type"`
	Magnitude   float64   `json:"magnitude"`
	Description string    `json:"description"`
}

type DeploymentConfig struct {
	Image           string                    `json:"image"`
	ImageTag        string                    `json:"image_tag"`
	Port            int                       `json:"port"`
	Protocol        string                    `json:"protocol"`
	HealthCheckPath string                    `json:"health_check_path"`
	ReadinessProbe  *ProbeConfig              `json:"readiness_probe"`
	LivenessProbe   *ProbeConfig              `json:"liveness_probe"`
	Environment     map[string]string         `json:"environment"`
	Secrets         []SecretMount             `json:"secrets"`
	Volumes         []VolumeMount             `json:"volumes"`
}

type ProbeConfig struct {
	Path                string        `json:"path"`
	Port                int           `json:"port"`
	InitialDelaySeconds int           `json:"initial_delay_seconds"`
	PeriodSeconds       int           `json:"period_seconds"`
	TimeoutSeconds      int           `json:"timeout_seconds"`
	FailureThreshold    int           `json:"failure_threshold"`
	SuccessThreshold    int           `json:"success_threshold"`
}

type SecretMount struct {
	SecretName string `json:"secret_name"`
	MountPath  string `json:"mount_path"`
	Key        string `json:"key,omitempty"`
}

type VolumeMount struct {
	Name      string `json:"name"`
	MountPath string `json:"mount_path"`
	ReadOnly  bool   `json:"read_only"`
}

type DeploymentEndpoint struct {
	Name        string                    `json:"name"`
	URL         string                    `json:"url"`
	Type        string                    `json:"type"` // inference, health, metrics
	Method      string                    `json:"method"`
	Status      string                    `json:"status"`
	LastChecked time.Time                 `json:"last_checked"`
}

type DeploymentMetrics struct {
	RequestsPerSecond   float64       `json:"requests_per_second"`
	AverageLatency      time.Duration `json:"average_latency"`
	P95Latency          time.Duration `json:"p95_latency"`
	P99Latency          time.Duration `json:"p99_latency"`
	ErrorRate           float64       `json:"error_rate"`
	CPUUtilization      float64       `json:"cpu_utilization"`
	MemoryUtilization   float64       `json:"memory_utilization"`
	ThroughputMBPS      float64       `json:"throughput_mbps"`
	ActiveConnections   int           `json:"active_connections"`
	LastUpdated         time.Time     `json:"last_updated"`
}

type SLAMetrics struct {
	Availability        float64       `json:"availability"`
	ResponseTime        time.Duration `json:"response_time"`
	Throughput          float64       `json:"throughput"`
	ErrorRate           float64       `json:"error_rate"`
	SLATarget           *SLATarget    `json:"sla_target"`
	Violations          []SLAViolation `json:"violations"`
	LastUpdated         time.Time     `json:"last_updated"`
}

type SLATarget struct {
	AvailabilityTarget  float64       `json:"availability_target"`
	ResponseTimeTarget  time.Duration `json:"response_time_target"`
	ThroughputTarget    float64       `json:"throughput_target"`
	ErrorRateTarget     float64       `json:"error_rate_target"`
}

type SLAViolation struct {
	Type        string    `json:"type"`
	Metric      string    `json:"metric"`
	Value       float64   `json:"value"`
	Target      float64   `json:"target"`
	Timestamp   time.Time `json:"timestamp"`
	Duration    time.Duration `json:"duration"`
	Severity    string    `json:"severity"`
	Resolved    bool      `json:"resolved"`
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
}

type VersionMetrics struct {
	TotalVersions       int                       `json:"total_versions"`
	ActiveExperiments   int                       `json:"active_experiments"`
	DeploymentCount     map[string]int            `json:"deployment_count"`
	InferenceCount      map[string]int64          `json:"inference_count"`
	AverageLatency      map[string]time.Duration  `json:"average_latency"`
	ErrorRates          map[string]float64        `json:"error_rates"`
	LastUpdated         time.Time                 `json:"last_updated"`
}

// NewModelRegistry creates a new model registry
func NewModelRegistry() *ModelRegistry {
	return &ModelRegistry{
		models:   make(map[string]*ModelEntry),
		versions: make(map[string]map[string]*ModelVersion),
		aliases:  make(map[string]*ModelAlias),
		tracer:   otel.Tracer("model-registry"),
	}
}

// NewModelVersionManager creates a new model version manager
func NewModelVersionManager() *ModelVersionManager {
	return &ModelVersionManager{
		registry:      NewModelRegistry(),
		experiments:   make(map[uuid.UUID]*ABTestExperiment),
		deployments:   make(map[uuid.UUID]*ModelDeployment),
		trafficSplits: make(map[string]*TrafficSplit),
		metrics:       &VersionMetrics{},
		tracer:        otel.Tracer("model-version-manager"),
	}
}

// RegisterModel registers a new model in the registry
func (mr *ModelRegistry) RegisterModel(ctx context.Context, model *ModelEntry) error {
	ctx, span := mr.tracer.Start(ctx, "model_registry.register_model")
	defer span.End()

	if model.Name == "" {
		return fmt.Errorf("model name is required")
	}

	model.CreatedAt = time.Now()
	model.UpdatedAt = time.Now()
	model.Status = ModelRegistryStatusActive

	mr.mutex.Lock()
	mr.models[model.Name] = model
	mr.versions[model.Name] = make(map[string]*ModelVersion)
	mr.mutex.Unlock()

	span.SetAttributes(
		attribute.String("model_name", model.Name),
		attribute.String("model_type", string(model.ModelType)),
	)

	log.Printf("Model registered: %s", model.Name)
	return nil
}

// RegisterVersion registers a new version of a model
func (mr *ModelRegistry) RegisterVersion(ctx context.Context, version *ModelVersion) error {
	ctx, span := mr.tracer.Start(ctx, "model_registry.register_version")
	defer span.End()

	if version.ID == uuid.Nil {
		version.ID = uuid.New()
	}

	version.CreatedAt = time.Now()
	version.Status = ModelVersionStatusDraft
	version.ApprovalStatus = ApprovalStatusPending

	mr.mutex.Lock()
	defer mr.mutex.Unlock()

	// Check if model exists
	model, exists := mr.models[version.ModelName]
	if !exists {
		return fmt.Errorf("model not found: %s", version.ModelName)
	}

	// Check if version already exists
	if _, exists := mr.versions[version.ModelName][version.Version]; exists {
		return fmt.Errorf("version already exists: %s:%s", version.ModelName, version.Version)
	}

	mr.versions[version.ModelName][version.Version] = version
	model.Versions = append(model.Versions, version.Version)
	model.LatestVersion = version.Version
	model.UpdatedAt = time.Now()

	span.SetAttributes(
		attribute.String("model_name", version.ModelName),
		attribute.String("version", version.Version),
	)

	log.Printf("Model version registered: %s:%s", version.ModelName, version.Version)
	return nil
}

// GetModelVersion retrieves a specific model version
func (mr *ModelRegistry) GetModelVersion(ctx context.Context, modelName, version string) (*ModelVersion, error) {
	mr.mutex.RLock()
	defer mr.mutex.RUnlock()

	versions, exists := mr.versions[modelName]
	if !exists {
		return nil, fmt.Errorf("model not found: %s", modelName)
	}

	modelVersion, exists := versions[version]
	if !exists {
		return nil, fmt.Errorf("version not found: %s:%s", modelName, version)
	}

	return modelVersion, nil
}

// CreateAlias creates an alias pointing to a specific model version
func (mr *ModelRegistry) CreateAlias(ctx context.Context, alias *ModelAlias) error {
	ctx, span := mr.tracer.Start(ctx, "model_registry.create_alias")
	defer span.End()

	// Validate that the target version exists
	_, err := mr.GetModelVersion(ctx, alias.ModelName, alias.Version)
	if err != nil {
		return fmt.Errorf("target version not found: %w", err)
	}

	alias.CreatedAt = time.Now()
	alias.UpdatedAt = time.Now()

	mr.mutex.Lock()
	mr.aliases[alias.Name] = alias
	mr.mutex.Unlock()

	span.SetAttributes(
		attribute.String("alias_name", alias.Name),
		attribute.String("model_name", alias.ModelName),
		attribute.String("version", alias.Version),
	)

	log.Printf("Model alias created: %s -> %s:%s", alias.Name, alias.ModelName, alias.Version)
	return nil
}

// StartABTest starts an A/B testing experiment
func (mvm *ModelVersionManager) StartABTest(ctx context.Context, experiment *ABTestExperiment) error {
	ctx, span := mvm.tracer.Start(ctx, "model_version_manager.start_ab_test")
	defer span.End()

	if experiment.ID == uuid.Nil {
		experiment.ID = uuid.New()
	}

	experiment.CreatedAt = time.Now()
	experiment.Status = ExperimentStatusRunning
	startTime := time.Now()
	experiment.StartedAt = &startTime

	// Validate experiment configuration
	if err := mvm.validateExperiment(experiment); err != nil {
		return fmt.Errorf("invalid experiment configuration: %w", err)
	}

	// Create traffic split
	trafficSplit := &TrafficSplit{
		ID:             uuid.New(),
		ModelName:      experiment.ModelName,
		ExperimentID:   &experiment.ID,
		DefaultVersion: experiment.ControlVersion,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Status:         TrafficSplitStatusActive,
	}

	// Configure traffic rules
	trafficSplit.Rules = append(trafficSplit.Rules, TrafficRule{
		Version:    experiment.ControlVersion,
		Percentage: experiment.TrafficSplit.ControlPercentage,
		Priority:   1,
		Enabled:    true,
	})

	for i, treatment := range experiment.TreatmentVersions {
		trafficSplit.Rules = append(trafficSplit.Rules, TrafficRule{
			Version:    treatment.Version,
			Percentage: treatment.TrafficPercentage,
			Priority:   i + 2,
			Enabled:    true,
		})
	}

	mvm.mutex.Lock()
	mvm.experiments[experiment.ID] = experiment
	mvm.trafficSplits[experiment.ModelName] = trafficSplit
	mvm.mutex.Unlock()

	span.SetAttributes(
		attribute.String("experiment_id", experiment.ID.String()),
		attribute.String("model_name", experiment.ModelName),
		attribute.String("control_version", experiment.ControlVersion),
	)

	log.Printf("A/B test experiment started: %s for model %s", experiment.ID, experiment.ModelName)
	return nil
}

// GetExperimentResults retrieves results for an A/B test experiment
func (mvm *ModelVersionManager) GetExperimentResults(ctx context.Context, experimentID uuid.UUID) (*ExperimentResults, error) {
	mvm.mutex.RLock()
	defer mvm.mutex.RUnlock()

	experiment, exists := mvm.experiments[experimentID]
	if !exists {
		return nil, fmt.Errorf("experiment not found: %s", experimentID)
	}

	// Generate simulated results (in real implementation, this would query actual metrics)
	results := &ExperimentResults{
		SampleSizes: map[string]int{
			experiment.ControlVersion: 5000,
		},
		MetricResults:   make(map[string]MetricResult),
		ConversionRates: make(map[string]float64),
		StatisticalPower: 0.8,
		ConfidenceLevel:  0.95,
		Duration:        time.Since(*experiment.StartedAt),
	}

	// Add treatment sample sizes
	for _, treatment := range experiment.TreatmentVersions {
		results.SampleSizes[treatment.Version] = int(float64(5000) * treatment.TrafficPercentage / 100.0)
	}

	// Generate metric results for primary metric
	primaryMetric := experiment.PrimaryMetric
	controlValue := 0.15 // 15% baseline conversion rate
	results.ConversionRates[experiment.ControlVersion] = controlValue

	treatmentValues := make(map[string]float64)
	relativeLift := make(map[string]float64)
	absoluteLift := make(map[string]float64)
	confidenceInterval := make(map[string]ConfidenceInterval)

	for _, treatment := range experiment.TreatmentVersions {
		// Simulate treatment effect (2-5% improvement)
		treatmentValue := controlValue * (1.0 + 0.02 + float64(len(treatment.Version)%3)*0.01)
		treatmentValues[treatment.Version] = treatmentValue
		results.ConversionRates[treatment.Version] = treatmentValue

		lift := (treatmentValue - controlValue) / controlValue
		relativeLift[treatment.Version] = lift
		absoluteLift[treatment.Version] = treatmentValue - controlValue

		// Simulate confidence interval
		confidenceInterval[treatment.Version] = ConfidenceInterval{
			Lower: lift - 0.02,
			Upper: lift + 0.02,
		}
	}

	results.MetricResults[primaryMetric.Name] = MetricResult{
		ControlValue:       controlValue,
		TreatmentValues:    treatmentValues,
		RelativeLift:       relativeLift,
		AbsoluteLift:       absoluteLift,
		ConfidenceInterval: confidenceInterval,
		PValue:             0.03, // Statistically significant
		IsSignificant:      true,
	}

	return results, nil
}

// StopABTest stops an A/B testing experiment
func (mvm *ModelVersionManager) StopABTest(ctx context.Context, experimentID uuid.UUID, reason string) error {
	ctx, span := mvm.tracer.Start(ctx, "model_version_manager.stop_ab_test")
	defer span.End()

	mvm.mutex.Lock()
	defer mvm.mutex.Unlock()

	experiment, exists := mvm.experiments[experimentID]
	if !exists {
		return fmt.Errorf("experiment not found: %s", experimentID)
	}

	experiment.Status = ExperimentStatusCompleted
	endTime := time.Now()
	experiment.EndedAt = &endTime

	// Deactivate traffic split
	if trafficSplit, exists := mvm.trafficSplits[experiment.ModelName]; exists {
		trafficSplit.Status = TrafficSplitStatusInactive
		trafficSplit.UpdatedAt = time.Now()
	}

	span.SetAttributes(
		attribute.String("experiment_id", experimentID.String()),
		attribute.String("reason", reason),
	)

	log.Printf("A/B test experiment stopped: %s (reason: %s)", experimentID, reason)
	return nil
}

// Internal methods

func (mvm *ModelVersionManager) validateExperiment(experiment *ABTestExperiment) error {
	if experiment.ModelName == "" {
		return fmt.Errorf("model_name is required")
	}
	if experiment.ControlVersion == "" {
		return fmt.Errorf("control_version is required")
	}
	if len(experiment.TreatmentVersions) == 0 {
		return fmt.Errorf("at least one treatment version is required")
	}

	// Validate traffic percentages sum to 100%
	totalPercentage := experiment.TrafficSplit.ControlPercentage
	for _, treatment := range experiment.TreatmentVersions {
		totalPercentage += treatment.TrafficPercentage
	}
	if totalPercentage != 100.0 {
		return fmt.Errorf("traffic percentages must sum to 100%%, got %.2f%%", totalPercentage)
	}

	return nil
}

// Traffic splitting configuration
type TrafficSplitConfig struct {
	ControlPercentage float64 `json:"control_percentage"`
}