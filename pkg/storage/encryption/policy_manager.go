package encryption

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/jscharber/eAIIngest/pkg/storage"
)

// PolicyManager manages encryption policies
type PolicyManager struct {
	policies    map[uuid.UUID]*EncryptionPolicy
	policyStore PolicyStore
	encryptor   *Encryptor
	tracer      trace.Tracer
	mu          sync.RWMutex
}

// PolicyStore interface for persistent policy storage
type PolicyStore interface {
	StorePolicy(ctx context.Context, policy *EncryptionPolicy) error
	GetPolicy(ctx context.Context, policyID uuid.UUID) (*EncryptionPolicy, error)
	ListPolicies(ctx context.Context, tenantID uuid.UUID) ([]*EncryptionPolicy, error)
	UpdatePolicy(ctx context.Context, policy *EncryptionPolicy) error
	DeletePolicy(ctx context.Context, policyID uuid.UUID) error
}

// NewPolicyManager creates a new policy manager
func NewPolicyManager(policyStore PolicyStore, encryptor *Encryptor) *PolicyManager {
	return &PolicyManager{
		policies:    make(map[uuid.UUID]*EncryptionPolicy),
		policyStore: policyStore,
		encryptor:   encryptor,
		tracer:      otel.Tracer("encryption-policy-manager"),
	}
}

// CreatePolicy creates a new encryption policy
func (pm *PolicyManager) CreatePolicy(ctx context.Context, policy *EncryptionPolicy) error {
	ctx, span := pm.tracer.Start(ctx, "create_encryption_policy")
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

	// Store policy
	if err := pm.policyStore.StorePolicy(ctx, policy); err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to store policy: %w", err)
	}

	// Cache policy
	pm.mu.Lock()
	pm.policies[policy.ID] = policy
	pm.mu.Unlock()

	span.SetAttributes(
		attribute.String("policy.id", policy.ID.String()),
		attribute.String("policy.name", policy.Name),
		attribute.String("tenant.id", policy.TenantID.String()),
	)

	return nil
}

// UpdatePolicy updates an existing encryption policy
func (pm *PolicyManager) UpdatePolicy(ctx context.Context, policyID uuid.UUID, updates *EncryptionPolicy) error {
	ctx, span := pm.tracer.Start(ctx, "update_encryption_policy")
	defer span.End()

	// Get existing policy
	policy, err := pm.GetPolicy(ctx, policyID)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to get policy: %w", err)
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

	// Store updated policy
	if err := pm.policyStore.UpdatePolicy(ctx, updates); err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to update policy: %w", err)
	}

	// Update cache
	pm.mu.Lock()
	pm.policies[policyID] = updates
	pm.mu.Unlock()

	span.SetAttributes(
		attribute.String("policy.id", policyID.String()),
		attribute.String("policy.name", updates.Name),
	)

	return nil
}

// DeletePolicy deletes an encryption policy
func (pm *PolicyManager) DeletePolicy(ctx context.Context, policyID uuid.UUID) error {
	ctx, span := pm.tracer.Start(ctx, "delete_encryption_policy")
	defer span.End()

	// Delete from store
	if err := pm.policyStore.DeletePolicy(ctx, policyID); err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to delete policy: %w", err)
	}

	// Remove from cache
	pm.mu.Lock()
	delete(pm.policies, policyID)
	pm.mu.Unlock()

	span.SetAttributes(attribute.String("policy.id", policyID.String()))
	return nil
}

// GetPolicy retrieves an encryption policy
func (pm *PolicyManager) GetPolicy(ctx context.Context, policyID uuid.UUID) (*EncryptionPolicy, error) {
	// Check cache first
	pm.mu.RLock()
	policy, exists := pm.policies[policyID]
	pm.mu.RUnlock()

	if exists {
		return policy, nil
	}

	// Load from store
	policy, err := pm.policyStore.GetPolicy(ctx, policyID)
	if err != nil {
		return nil, fmt.Errorf("failed to get policy: %w", err)
	}

	// Cache policy
	pm.mu.Lock()
	pm.policies[policyID] = policy
	pm.mu.Unlock()

	return policy, nil
}

// ListPolicies lists all policies for a tenant
func (pm *PolicyManager) ListPolicies(ctx context.Context, tenantID uuid.UUID) ([]*EncryptionPolicy, error) {
	return pm.policyStore.ListPolicies(ctx, tenantID)
}

// EvaluateFile determines if a file should be encrypted based on policies
func (pm *PolicyManager) EvaluateFile(ctx context.Context, fileInfo *storage.FileInfo, tenantID uuid.UUID) (*EncryptionAction, error) {
	ctx, span := pm.tracer.Start(ctx, "evaluate_file_encryption")
	defer span.End()

	span.SetAttributes(
		attribute.String("file.url", fileInfo.URL),
		attribute.Int64("file.size", fileInfo.Size),
		attribute.String("file.type", fileInfo.ContentType),
		attribute.String("tenant.id", tenantID.String()),
	)

	// Get policies for tenant
	policies, err := pm.getPoliciesForTenant(ctx, tenantID)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	// Evaluate policies in priority order
	for _, policy := range policies {
		if !policy.Enabled {
			continue
		}

		// Evaluate rules
		for _, rule := range policy.Rules {
			if !rule.Enabled {
				continue
			}

			if pm.evaluateRule(ctx, rule, fileInfo, tenantID) {
				span.SetAttributes(
					attribute.String("matched.policy", policy.Name),
					attribute.String("matched.rule", rule.Name),
					attribute.Bool("encrypt", rule.Action.Encrypt),
				)
				return &rule.Action, nil
			}
		}

		// Use default action if no rules matched
		if policy.DefaultAction.Encrypt {
			span.SetAttributes(
				attribute.String("matched.policy", policy.Name),
				attribute.String("matched.rule", "default"),
				attribute.Bool("encrypt", policy.DefaultAction.Encrypt),
			)
			return &policy.DefaultAction, nil
		}
	}

	// No matching policy found - default to no encryption
	span.SetAttributes(attribute.Bool("policy.matched", false))
	return &EncryptionAction{Encrypt: false}, nil
}

// ApplyEncryption applies encryption based on policy evaluation
func (pm *PolicyManager) ApplyEncryption(ctx context.Context, data []byte, fileInfo *storage.FileInfo, tenantID uuid.UUID) (*EncryptionResult, error) {
	ctx, span := pm.tracer.Start(ctx, "apply_encryption_policy")
	defer span.End()

	// Evaluate encryption policy
	action, err := pm.EvaluateFile(ctx, fileInfo, tenantID)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	if !action.Encrypt {
		// No encryption required
		return nil, nil
	}

	// Create encryption context
	encCtx := &EncryptionContext{
		TenantID:     tenantID,
		Purpose:      PurposeDocumentEncryption,
		ResourceID:   fileInfo.URL,
		ResourceType: "document",
		AdditionalData: map[string]string{
			"file_name": fileInfo.Name,
			"file_size": fmt.Sprintf("%d", fileInfo.Size),
			"file_type": fileInfo.ContentType,
		},
	}

	// Add classification if available
	if classification, ok := fileInfo.Metadata["classification"]; ok {
		encCtx.Classification = classification
	}

	// Apply encryption
	result, err := pm.encryptor.EncryptDocument(ctx, data, encCtx)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("encryption failed: %w", err)
	}

	span.SetAttributes(
		attribute.String("algorithm", string(action.Algorithm)),
		attribute.Bool("compressed", result.Compressed),
		attribute.Int("encrypted.size", len(result.EncryptedData)),
		attribute.Float64("encryption.duration_ms", float64(result.Duration.Milliseconds())),
	)

	return result, nil
}

// Helper methods

func (pm *PolicyManager) getPoliciesForTenant(ctx context.Context, tenantID uuid.UUID) ([]*EncryptionPolicy, error) {
	policies, err := pm.policyStore.ListPolicies(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	// Sort by priority (higher priority first)
	for i := 0; i < len(policies)-1; i++ {
		for j := i + 1; j < len(policies); j++ {
			if policies[i].Priority < policies[j].Priority {
				policies[i], policies[j] = policies[j], policies[i]
			}
		}
	}

	return policies, nil
}

func (pm *PolicyManager) evaluateRule(ctx context.Context, rule EncryptionRule, fileInfo *storage.FileInfo, tenantID uuid.UUID) bool {
	// All conditions must be true for the rule to match
	for _, condition := range rule.Conditions {
		if !pm.evaluateCondition(condition, fileInfo, tenantID) {
			return false
		}
	}
	return true
}

func (pm *PolicyManager) evaluateCondition(condition PolicyCondition, fileInfo *storage.FileInfo, tenantID uuid.UUID) bool {
	switch condition.Type {
	case ConditionFileType:
		return pm.evaluateStringCondition(fileInfo.ContentType, condition)

	case ConditionFileSize:
		return pm.evaluateNumericCondition(float64(fileInfo.Size), condition)

	case ConditionPath:
		return pm.evaluateStringCondition(fileInfo.URL, condition)

	case ConditionTenant:
		return pm.evaluateStringCondition(tenantID.String(), condition)

	case ConditionClassification:
		if classification, ok := fileInfo.Metadata["classification"]; ok {
			return pm.evaluateStringCondition(classification, condition)
		}
		return false

	case ConditionMetadata:
		if condition.Field == "" {
			return false
		}
		if value, exists := fileInfo.Metadata[condition.Field]; exists {
			return pm.evaluateStringCondition(value, condition)
		}
		return false

	default:
		return false
	}
}

func (pm *PolicyManager) evaluateNumericCondition(value float64, condition PolicyCondition) bool {
	target, ok := condition.Value.(float64)
	if !ok {
		// Try to convert
		if intVal, ok := condition.Value.(int); ok {
			target = float64(intVal)
		} else {
			return false
		}
	}

	switch condition.Operator {
	case "eq", "equals":
		return value == target
	case "ne", "not_equals":
		return value != target
	case "gt", "greater_than":
		return value > target
	case "lt", "less_than":
		return value < target
	case "ge", "greater_equal":
		return value >= target
	case "le", "less_equal":
		return value <= target
	default:
		return false
	}
}

func (pm *PolicyManager) evaluateStringCondition(value string, condition PolicyCondition) bool {
	target, ok := condition.Value.(string)
	if !ok {
		return false
	}

	switch condition.Operator {
	case "eq", "equals":
		return value == target
	case "ne", "not_equals":
		return value != target
	case "contains":
		return strings.Contains(value, target)
	case "starts_with":
		return strings.HasPrefix(value, target)
	case "ends_with":
		return strings.HasSuffix(value, target)
	case "in":
		if list, ok := condition.Value.([]string); ok {
			for _, item := range list {
				if value == item {
					return true
				}
			}
		}
		return false
	case "not_in":
		if list, ok := condition.Value.([]string); ok {
			for _, item := range list {
				if value == item {
					return false
				}
			}
			return true
		}
		return false
	default:
		return false
	}
}

func (pm *PolicyManager) validatePolicy(policy *EncryptionPolicy) error {
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

	// Validate default action
	if err := pm.validateAction(policy.DefaultAction); err != nil {
		return fmt.Errorf("invalid default action: %w", err)
	}

	return nil
}

func (pm *PolicyManager) validateRule(rule EncryptionRule) error {
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

func (pm *PolicyManager) validateCondition(condition PolicyCondition) error {
	// Validate condition type
	validTypes := []ConditionType{
		ConditionFileType, ConditionFileSize, ConditionDataSource,
		ConditionClassification, ConditionTenant, ConditionPath, ConditionMetadata,
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
	validOperators := []string{
		"eq", "equals", "ne", "not_equals", "gt", "greater_than",
		"lt", "less_than", "ge", "greater_equal", "le", "less_equal",
		"contains", "starts_with", "ends_with", "in", "not_in",
	}

	validOperator := false
	for _, vo := range validOperators {
		if condition.Operator == vo {
			validOperator = true
			break
		}
	}

	if !validOperator {
		return fmt.Errorf("invalid operator: %s", condition.Operator)
	}

	return nil
}

func (pm *PolicyManager) validateAction(action EncryptionAction) error {
	if action.Encrypt {
		// Validate algorithm
		validAlgorithms := []EncryptionAlgorithm{
			AlgorithmAES256GCM, AlgorithmAES256CBC,
			AlgorithmChaCha20, AlgorithmXChaCha20,
		}

		validAlgorithm := false
		for _, va := range validAlgorithms {
			if action.Algorithm == va {
				validAlgorithm = true
				break
			}
		}

		if !validAlgorithm {
			return fmt.Errorf("invalid encryption algorithm: %s", action.Algorithm)
		}

		// Validate key type
		validKeyTypes := []KeyType{
			KeyTypeMaster, KeyTypeData, KeyTypeWrapping,
			KeyTypeBackup, KeyTypeTransit,
		}

		validKeyType := false
		for _, vk := range validKeyTypes {
			if action.KeyType == vk {
				validKeyType = true
				break
			}
		}

		if !validKeyType {
			return fmt.Errorf("invalid key type: %s", action.KeyType)
		}
	}

	return nil
}
