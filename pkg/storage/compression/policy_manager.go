package compression

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/jscharber/eAIIngest/pkg/storage"
)

// CompressionPolicy defines compression rules for different scenarios
type CompressionPolicy struct {
	ID          uuid.UUID                   `json:"id"`
	Name        string                      `json:"name"`
	Description string                      `json:"description"`
	TenantID    uuid.UUID                   `json:"tenant_id"`
	
	// Policy rules
	Rules       []CompressionRule           `json:"rules"`
	DefaultRule *CompressionRule            `json:"default_rule"`
	
	// Policy metadata
	CreatedAt   time.Time                   `json:"created_at"`
	UpdatedAt   time.Time                   `json:"updated_at"`
	CreatedBy   string                      `json:"created_by"`
	Enabled     bool                        `json:"enabled"`
	Priority    int                         `json:"priority"`
	
	// Policy settings
	Settings    CompressionPolicySettings   `json:"settings"`
}

// CompressionRule defines when and how to apply compression
type CompressionRule struct {
	ID          uuid.UUID                   `json:"id"`
	Name        string                      `json:"name"`
	Conditions  []CompressionCondition      `json:"conditions"`
	Action      CompressionAction           `json:"action"`
	Priority    int                         `json:"priority"`
	Enabled     bool                        `json:"enabled"`
}

// CompressionCondition defines conditions for applying compression
type CompressionCondition struct {
	Type     ConditionType    `json:"type"`
	Operator ConditionOperator `json:"operator"`
	Value    interface{}      `json:"value"`
	Field    string           `json:"field,omitempty"`
}

// ConditionType defines the type of condition
type ConditionType string

const (
	ConditionFileSize      ConditionType = "file_size"
	ConditionFileExtension ConditionType = "file_extension"
	ConditionMimeType      ConditionType = "mime_type"
	ConditionAge           ConditionType = "age"
	ConditionPath          ConditionType = "path"
	ConditionMetadata      ConditionType = "metadata"
	ConditionDataSource    ConditionType = "data_source"
	ConditionTenant        ConditionType = "tenant"
)

// ConditionOperator defines comparison operators
type ConditionOperator string

const (
	OperatorEquals       ConditionOperator = "eq"
	OperatorNotEquals    ConditionOperator = "ne"
	OperatorGreaterThan  ConditionOperator = "gt"
	OperatorLessThan     ConditionOperator = "lt"
	OperatorGreaterEqual ConditionOperator = "ge"
	OperatorLessEqual    ConditionOperator = "le"
	OperatorContains     ConditionOperator = "contains"
	OperatorStartsWith   ConditionOperator = "starts_with"
	OperatorEndsWith     ConditionOperator = "ends_with"
	OperatorMatches      ConditionOperator = "matches" // Regex
	OperatorIn           ConditionOperator = "in"
	OperatorNotIn        ConditionOperator = "not_in"
)

// CompressionAction defines what compression action to take
type CompressionAction struct {
	Strategy         DocumentCompressionStrategy `json:"strategy"`
	CompressionLevel int                         `json:"compression_level"`
	StoreOriginal    bool                        `json:"store_original"`
	CacheResult      bool                        `json:"cache_result"`
	CacheTTL         time.Duration               `json:"cache_ttl"`
	AsyncCompression bool                        `json:"async_compression"`
	
	// Advanced options
	BatchSize        int           `json:"batch_size"`
	RetryAttempts    int           `json:"retry_attempts"`
	RetryDelay       time.Duration `json:"retry_delay"`
	NotifyOnComplete bool          `json:"notify_on_complete"`
}

// CompressionPolicySettings contains policy-wide settings
type CompressionPolicySettings struct {
	EnableMetrics     bool          `json:"enable_metrics"`
	EnableAuditLog    bool          `json:"enable_audit_log"`
	MaxConcurrency    int           `json:"max_concurrency"`
	DefaultCacheTTL   time.Duration `json:"default_cache_ttl"`
	ErrorHandling     string        `json:"error_handling"` // "fail", "skip", "retry"
	
	// Performance settings
	BufferSize        int `json:"buffer_size"`
	BatchTimeout      time.Duration `json:"batch_timeout"`
	
	// Storage settings
	PreferCompressed  bool `json:"prefer_compressed"`
	CleanupOriginals  bool `json:"cleanup_originals"`
	CleanupDelay      time.Duration `json:"cleanup_delay"`
}

// PolicyManager manages compression policies
type PolicyManager struct {
	policies   map[uuid.UUID]*CompressionPolicy
	compressor *DocumentCompressor
	tracer     trace.Tracer
	metrics    *PolicyMetrics
}

// NewPolicyManager creates a new policy manager
func NewPolicyManager(compressor *DocumentCompressor) *PolicyManager {
	return &PolicyManager{
		policies:   make(map[uuid.UUID]*CompressionPolicy),
		compressor: compressor,
		tracer:     otel.Tracer("compression-policy-manager"),
		metrics:    NewPolicyMetrics(),
	}
}

// CreatePolicy creates a new compression policy
func (pm *PolicyManager) CreatePolicy(ctx context.Context, policy *CompressionPolicy) error {
	ctx, span := pm.tracer.Start(ctx, "create_compression_policy")
	defer span.End()

	if policy.ID == uuid.Nil {
		policy.ID = uuid.New()
	}

	// Validate policy
	if err := pm.validatePolicy(policy); err != nil {
		span.RecordError(err)
		return fmt.Errorf("invalid policy: %w", err)
	}

	// Set timestamps
	now := time.Now()
	policy.CreatedAt = now
	policy.UpdatedAt = now

	pm.policies[policy.ID] = policy

	span.SetAttributes(
		attribute.String("policy.id", policy.ID.String()),
		attribute.String("policy.name", policy.Name),
		attribute.String("policy.tenant_id", policy.TenantID.String()),
	)

	pm.metrics.RecordPolicyCreated(policy.TenantID)
	return nil
}

// UpdatePolicy updates an existing compression policy
func (pm *PolicyManager) UpdatePolicy(ctx context.Context, policyID uuid.UUID, updates *CompressionPolicy) error {
	ctx, span := pm.tracer.Start(ctx, "update_compression_policy")
	defer span.End()

	policy, exists := pm.policies[policyID]
	if !exists {
		return fmt.Errorf("policy not found: %s", policyID)
	}

	// Validate updates
	if err := pm.validatePolicy(updates); err != nil {
		span.RecordError(err)
		return fmt.Errorf("invalid policy updates: %w", err)
	}

	// Apply updates
	updates.ID = policyID
	updates.CreatedAt = policy.CreatedAt
	updates.UpdatedAt = time.Now()

	pm.policies[policyID] = updates

	span.SetAttributes(
		attribute.String("policy.id", policyID.String()),
		attribute.String("policy.name", updates.Name),
	)

	pm.metrics.RecordPolicyUpdated(updates.TenantID)
	return nil
}

// DeletePolicy deletes a compression policy
func (pm *PolicyManager) DeletePolicy(ctx context.Context, policyID uuid.UUID) error {
	ctx, span := pm.tracer.Start(ctx, "delete_compression_policy")
	defer span.End()

	policy, exists := pm.policies[policyID]
	if !exists {
		return fmt.Errorf("policy not found: %s", policyID)
	}

	delete(pm.policies, policyID)

	span.SetAttributes(attribute.String("policy.id", policyID.String()))
	pm.metrics.RecordPolicyDeleted(policy.TenantID)
	return nil
}

// GetPolicy retrieves a compression policy
func (pm *PolicyManager) GetPolicy(ctx context.Context, policyID uuid.UUID) (*CompressionPolicy, error) {
	policy, exists := pm.policies[policyID]
	if !exists {
		return nil, fmt.Errorf("policy not found: %s", policyID)
	}
	return policy, nil
}

// ListPolicies lists all policies for a tenant
func (pm *PolicyManager) ListPolicies(ctx context.Context, tenantID uuid.UUID) ([]*CompressionPolicy, error) {
	var policies []*CompressionPolicy
	
	for _, policy := range pm.policies {
		if policy.TenantID == tenantID {
			policies = append(policies, policy)
		}
	}

	return policies, nil
}

// EvaluateFile determines the compression action for a file
func (pm *PolicyManager) EvaluateFile(ctx context.Context, fileInfo *storage.FileInfo, tenantID uuid.UUID) (*CompressionAction, error) {
	ctx, span := pm.tracer.Start(ctx, "evaluate_file_compression")
	defer span.End()

	span.SetAttributes(
		attribute.String("file.url", fileInfo.URL),
		attribute.Int64("file.size", fileInfo.Size),
		attribute.String("file.mime_type", fileInfo.MimeType),
		attribute.String("tenant.id", tenantID.String()),
	)

	// Get policies for tenant, sorted by priority
	policies := pm.getPoliciesForTenant(tenantID)
	
	for _, policy := range policies {
		if !policy.Enabled {
			continue
		}

		// Evaluate rules in priority order
		for _, rule := range policy.Rules {
			if !rule.Enabled {
				continue
			}

			if pm.evaluateRule(ctx, rule, fileInfo, tenantID) {
				span.SetAttributes(
					attribute.String("matched.policy", policy.Name),
					attribute.String("matched.rule", rule.Name),
					attribute.String("compression.strategy", string(rule.Action.Strategy)),
				)

				pm.metrics.RecordRuleMatched(tenantID, policy.ID, rule.ID)
				return &rule.Action, nil
			}
		}

		// Check default rule
		if policy.DefaultRule != nil {
			span.SetAttributes(
				attribute.String("matched.policy", policy.Name),
				attribute.String("matched.rule", "default"),
				attribute.String("compression.strategy", string(policy.DefaultRule.Action.Strategy)),
			)

			pm.metrics.RecordDefaultRuleUsed(tenantID, policy.ID)
			return &policy.DefaultRule.Action, nil
		}
	}

	// No matching policy found
	span.SetAttributes(attribute.Bool("policy.matched", false))
	pm.metrics.RecordNoPolicyMatched(tenantID)
	return nil, fmt.Errorf("no compression policy matched for file: %s", fileInfo.URL)
}

// ApplyCompression applies compression based on policy evaluation
func (pm *PolicyManager) ApplyCompression(ctx context.Context, data []byte, fileInfo *storage.FileInfo, tenantID uuid.UUID) (*CompressionResult, error) {
	ctx, span := pm.tracer.Start(ctx, "apply_compression_policy")
	defer span.End()

	// Evaluate compression policy
	action, err := pm.EvaluateFile(ctx, fileInfo, tenantID)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	// Apply compression
	result, err := pm.compressor.CompressDocument(ctx, data, fileInfo.Name)
	if err != nil {
		span.RecordError(err)
		pm.metrics.RecordCompressionError(tenantID)
		return nil, fmt.Errorf("compression failed: %w", err)
	}

	span.SetAttributes(
		attribute.String("compression.strategy", string(action.Strategy)),
		attribute.Float64("compression.ratio", result.CompressionRatio),
		attribute.Int("original.size", result.OriginalSize),
		attribute.Int("compressed.size", result.CompressedSize),
	)

	pm.metrics.RecordCompressionSuccess(tenantID, result.CompressionRatio)
	return result, nil
}

// getPoliciesForTenant gets policies for a tenant sorted by priority
func (pm *PolicyManager) getPoliciesForTenant(tenantID uuid.UUID) []*CompressionPolicy {
	var policies []*CompressionPolicy
	
	for _, policy := range pm.policies {
		if policy.TenantID == tenantID && policy.Enabled {
			policies = append(policies, policy)
		}
	}

	// Sort by priority (higher priority first)
	for i := 0; i < len(policies)-1; i++ {
		for j := i + 1; j < len(policies); j++ {
			if policies[i].Priority < policies[j].Priority {
				policies[i], policies[j] = policies[j], policies[i]
			}
		}
	}

	return policies
}

// evaluateRule evaluates if a rule matches the given file
func (pm *PolicyManager) evaluateRule(ctx context.Context, rule CompressionRule, fileInfo *storage.FileInfo, tenantID uuid.UUID) bool {
	// All conditions must be true for the rule to match
	for _, condition := range rule.Conditions {
		if !pm.evaluateCondition(condition, fileInfo, tenantID) {
			return false
		}
	}
	return true
}

// evaluateCondition evaluates a single condition
func (pm *PolicyManager) evaluateCondition(condition CompressionCondition, fileInfo *storage.FileInfo, tenantID uuid.UUID) bool {
	switch condition.Type {
	case ConditionFileSize:
		return pm.evaluateNumericCondition(float64(fileInfo.Size), condition)
		
	case ConditionFileExtension:
		ext := strings.ToLower(fileInfo.Extension)
		return pm.evaluateStringCondition(ext, condition)
		
	case ConditionMimeType:
		return pm.evaluateStringCondition(fileInfo.MimeType, condition)
		
	case ConditionAge:
		age := time.Since(fileInfo.ModifiedAt)
		return pm.evaluateDurationCondition(age, condition)
		
	case ConditionPath:
		return pm.evaluateStringCondition(fileInfo.URL, condition)
		
	case ConditionTenant:
		return pm.evaluateStringCondition(tenantID.String(), condition)
		
	case ConditionMetadata:
		if condition.Field == "" {
			return false
		}
		if value, exists := fileInfo.Metadata[condition.Field]; exists {
			if strValue, ok := value.(string); ok {
				return pm.evaluateStringCondition(strValue, condition)
			}
		}
		return false
		
	default:
		return false
	}
}

// evaluateNumericCondition evaluates numeric conditions
func (pm *PolicyManager) evaluateNumericCondition(value float64, condition CompressionCondition) bool {
	target, ok := condition.Value.(float64)
	if !ok {
		return false
	}

	switch condition.Operator {
	case OperatorEquals:
		return value == target
	case OperatorNotEquals:
		return value != target
	case OperatorGreaterThan:
		return value > target
	case OperatorLessThan:
		return value < target
	case OperatorGreaterEqual:
		return value >= target
	case OperatorLessEqual:
		return value <= target
	default:
		return false
	}
}

// evaluateStringCondition evaluates string conditions
func (pm *PolicyManager) evaluateStringCondition(value string, condition CompressionCondition) bool {
	target, ok := condition.Value.(string)
	if !ok {
		return false
	}

	switch condition.Operator {
	case OperatorEquals:
		return value == target
	case OperatorNotEquals:
		return value != target
	case OperatorContains:
		return strings.Contains(value, target)
	case OperatorStartsWith:
		return strings.HasPrefix(value, target)
	case OperatorEndsWith:
		return strings.HasSuffix(value, target)
	default:
		return false
	}
}

// evaluateDurationCondition evaluates duration conditions
func (pm *PolicyManager) evaluateDurationCondition(value time.Duration, condition CompressionCondition) bool {
	target, ok := condition.Value.(time.Duration)
	if !ok {
		return false
	}

	switch condition.Operator {
	case OperatorGreaterThan:
		return value > target
	case OperatorLessThan:
		return value < target
	case OperatorGreaterEqual:
		return value >= target
	case OperatorLessEqual:
		return value <= target
	default:
		return false
	}
}

// validatePolicy validates a compression policy
func (pm *PolicyManager) validatePolicy(policy *CompressionPolicy) error {
	if policy.Name == "" {
		return fmt.Errorf("policy name is required")
	}

	if policy.TenantID == uuid.Nil {
		return fmt.Errorf("tenant ID is required")
	}

	// Validate rules
	for _, rule := range policy.Rules {
		if err := pm.validateRule(rule); err != nil {
			return fmt.Errorf("invalid rule %s: %w", rule.Name, err)
		}
	}

	// Validate default rule if present
	if policy.DefaultRule != nil {
		if err := pm.validateRule(*policy.DefaultRule); err != nil {
			return fmt.Errorf("invalid default rule: %w", err)
		}
	}

	return nil
}

// validateRule validates a compression rule
func (pm *PolicyManager) validateRule(rule CompressionRule) error {
	if rule.Name == "" {
		return fmt.Errorf("rule name is required")
	}

	// Validate conditions
	for _, condition := range rule.Conditions {
		if err := pm.validateCondition(condition); err != nil {
			return fmt.Errorf("invalid condition: %w", err)
		}
	}

	// Validate action
	if err := pm.validateAction(rule.Action); err != nil {
		return fmt.Errorf("invalid action: %w", err)
	}

	return nil
}

// validateCondition validates a compression condition
func (pm *PolicyManager) validateCondition(condition CompressionCondition) error {
	// Validate condition type
	validTypes := []ConditionType{
		ConditionFileSize, ConditionFileExtension, ConditionMimeType,
		ConditionAge, ConditionPath, ConditionMetadata, ConditionTenant,
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

	// Validate operator
	validOperators := []ConditionOperator{
		OperatorEquals, OperatorNotEquals, OperatorGreaterThan, OperatorLessThan,
		OperatorGreaterEqual, OperatorLessEqual, OperatorContains,
		OperatorStartsWith, OperatorEndsWith, OperatorMatches, OperatorIn, OperatorNotIn,
	}
	
	validOperator := false
	for _, vo := range validOperators {
		if condition.Operator == vo {
			validOperator = true
			break
		}
	}
	
	if !validOperator {
		return fmt.Errorf("invalid condition operator: %s", condition.Operator)
	}

	return nil
}

// validateAction validates a compression action
func (pm *PolicyManager) validateAction(action CompressionAction) error {
	// Validate strategy
	validStrategies := []DocumentCompressionStrategy{
		StrategyNone, StrategyAdaptive, StrategyAggressive,
		StrategyText, StrategyBinary, StrategyArchive,
	}
	
	validStrategy := false
	for _, vs := range validStrategies {
		if action.Strategy == vs {
			validStrategy = true
			break
		}
	}
	
	if !validStrategy {
		return fmt.Errorf("invalid compression strategy: %s", action.Strategy)
	}

	// Validate compression level
	if action.CompressionLevel < 0 || action.CompressionLevel > 9 {
		return fmt.Errorf("compression level must be between 0 and 9")
	}

	return nil
}

// PolicyMetrics tracks policy-related metrics
type PolicyMetrics struct {
	PoliciesCreated      int64 `json:"policies_created"`
	PoliciesUpdated      int64 `json:"policies_updated"`
	PoliciesDeleted      int64 `json:"policies_deleted"`
	RulesMatched         int64 `json:"rules_matched"`
	DefaultRulesUsed     int64 `json:"default_rules_used"`
	NoPolicyMatched      int64 `json:"no_policy_matched"`
	CompressionSuccesses int64 `json:"compression_successes"`
	CompressionErrors    int64 `json:"compression_errors"`
}

// NewPolicyMetrics creates new policy metrics
func NewPolicyMetrics() *PolicyMetrics {
	return &PolicyMetrics{}
}

// RecordPolicyCreated records policy creation
func (m *PolicyMetrics) RecordPolicyCreated(tenantID uuid.UUID) {
	m.PoliciesCreated++
}

// RecordPolicyUpdated records policy update
func (m *PolicyMetrics) RecordPolicyUpdated(tenantID uuid.UUID) {
	m.PoliciesUpdated++
}

// RecordPolicyDeleted records policy deletion
func (m *PolicyMetrics) RecordPolicyDeleted(tenantID uuid.UUID) {
	m.PoliciesDeleted++
}

// RecordRuleMatched records rule match
func (m *PolicyMetrics) RecordRuleMatched(tenantID, policyID, ruleID uuid.UUID) {
	m.RulesMatched++
}

// RecordDefaultRuleUsed records default rule usage
func (m *PolicyMetrics) RecordDefaultRuleUsed(tenantID, policyID uuid.UUID) {
	m.DefaultRulesUsed++
}

// RecordNoPolicyMatched records when no policy matches
func (m *PolicyMetrics) RecordNoPolicyMatched(tenantID uuid.UUID) {
	m.NoPolicyMatched++
}

// RecordCompressionSuccess records successful compression
func (m *PolicyMetrics) RecordCompressionSuccess(tenantID uuid.UUID, ratio float64) {
	m.CompressionSuccesses++
}

// RecordCompressionError records compression error
func (m *PolicyMetrics) RecordCompressionError(tenantID uuid.UUID) {
	m.CompressionErrors++
}