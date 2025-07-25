package compliance

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jscharber/eAIIngest/pkg/dlp/types"
)

// BasicComplianceChecker implements basic compliance checking functionality
type BasicComplianceChecker struct {
	name    string
	version string
	rules   map[string]ComplianceRuleSet
}

// ComplianceRuleSet defines rules for a specific regulation
type ComplianceRuleSet struct {
	Regulation  string
	Description string
	Rules       []ComplianceRuleDefinition
}

// ComplianceRuleDefinition defines a specific compliance rule
type ComplianceRuleDefinition struct {
	RuleID      string
	Name        string
	Description string
	PIITypes    []types.PIIType
	Severity    string
	Required    bool
	Validator   func(*types.ScanResult) []types.ComplianceViolation
}

// NewBasicComplianceChecker creates a new basic compliance checker
func NewBasicComplianceChecker() *BasicComplianceChecker {
	checker := &BasicComplianceChecker{
		name:    "basic_compliance_checker",
		version: "1.0.0",
		rules:   make(map[string]ComplianceRuleSet),
	}
	
	// Register built-in compliance rule sets
	checker.registerGDPRRules()
	checker.registerHIPAARules()
	checker.registerPCIDSSRules()
	checker.registerCCPARules()
	
	return checker
}

// CheckCompliance validates content against specific compliance requirements
func (c *BasicComplianceChecker) CheckCompliance(ctx context.Context, scanResult *types.ScanResult, rules []types.ComplianceRule) (*types.ComplianceResult, error) {
	startTime := time.Now()
	
	result := &types.ComplianceResult{
		IsCompliant:     true,
		Violations:      []types.ComplianceViolation{},
		CheckedAt:       startTime,
		RequiredActions: []string{},
	}
	
	// Group rules by regulation
	rulesByRegulation := make(map[string][]types.ComplianceRule)
	for _, rule := range rules {
		rulesByRegulation[rule.Regulation] = append(rulesByRegulation[rule.Regulation], rule)
	}
	
	// Check each regulation
	for regulation, regulationRules := range rulesByRegulation {
		regulationResult, err := c.checkRegulation(scanResult, regulation, regulationRules)
		if err != nil {
			continue // Skip problematic regulations
		}
		
		// Merge results
		if !regulationResult.IsCompliant {
			result.IsCompliant = false
		}
		
		result.Violations = append(result.Violations, regulationResult.Violations...)
		result.RequiredActions = append(result.RequiredActions, regulationResult.RequiredActions...)
		
		// Set regulation if this is the first one
		if result.Regulation == "" {
			result.Regulation = regulation
		}
	}
	
	return result, nil
}

// GetSupportedRegulations returns supported compliance frameworks
func (c *BasicComplianceChecker) GetSupportedRegulations() []string {
	var regulations []string
	for regulation := range c.rules {
		regulations = append(regulations, regulation)
	}
	return regulations
}

// checkRegulation checks compliance for a specific regulation
func (c *BasicComplianceChecker) checkRegulation(scanResult *types.ScanResult, regulation string, rules []types.ComplianceRule) (*types.ComplianceResult, error) {
	ruleSet, exists := c.rules[regulation]
	if !exists {
		return nil, fmt.Errorf("unsupported regulation: %s", regulation)
	}
	
	result := &types.ComplianceResult{
		IsCompliant:     true,
		Violations:      []types.ComplianceViolation{},
		Regulation:      regulation,
		CheckedAt:       time.Now(),
		RequiredActions: []string{},
	}
	
	// Check each rule definition
	for _, ruleDef := range ruleSet.Rules {
		// Check if this rule is enabled
		if !c.isRuleEnabled(ruleDef.RuleID, rules) {
			continue
		}
		
		// Apply rule validator
		violations := ruleDef.Validator(scanResult)
		
		if len(violations) > 0 {
			result.IsCompliant = false
			result.Violations = append(result.Violations, violations...)
			
			// Add required actions based on rule
			actions := c.generateRequiredActions(ruleDef, violations)
			result.RequiredActions = append(result.RequiredActions, actions...)
		}
	}
	
	return result, nil
}

// Register compliance rule sets

func (c *BasicComplianceChecker) registerGDPRRules() {
	gdprRules := ComplianceRuleSet{
		Regulation:  "GDPR",
		Description: "General Data Protection Regulation",
		Rules: []ComplianceRuleDefinition{
			{
				RuleID:      "GDPR-001",
				Name:        "Personal Data Detection",
				Description: "Detect personal data that requires GDPR compliance",
				PIITypes:    []types.PIIType{types.PIITypeEmail, types.PIITypeName, types.PIITypeAddress, types.PIITypePhoneNumber},
				Severity:    "high",
				Required:    true,
				Validator:   c.validateGDPRPersonalData,
			},
			{
				RuleID:      "GDPR-002", 
				Name:        "Special Category Data",
				Description: "Detect special category personal data requiring extra protection",
				PIITypes:    []types.PIIType{types.PIITypeDateOfBirth, types.PIITypeSSN},
				Severity:    "critical",
				Required:    true,
				Validator:   c.validateGDPRSpecialCategory,
			},
		},
	}
	
	c.rules["GDPR"] = gdprRules
}

func (c *BasicComplianceChecker) registerHIPAARules() {
	hipaaRules := ComplianceRuleSet{
		Regulation:  "HIPAA",
		Description: "Health Insurance Portability and Accountability Act",
		Rules: []ComplianceRuleDefinition{
			{
				RuleID:      "HIPAA-001",
				Name:        "Protected Health Information",
				Description: "Detect PHI that requires HIPAA compliance",
				PIITypes:    []types.PIIType{types.PIITypeSSN, types.PIITypeDateOfBirth, types.PIITypeName, types.PIITypeEmail},
				Severity:    "critical",
				Required:    true,
				Validator:   c.validateHIPAAPHI,
			},
		},
	}
	
	c.rules["HIPAA"] = hipaaRules
}

func (c *BasicComplianceChecker) registerPCIDSSRules() {
	pciRules := ComplianceRuleSet{
		Regulation:  "PCI-DSS",
		Description: "Payment Card Industry Data Security Standard",
		Rules: []ComplianceRuleDefinition{
			{
				RuleID:      "PCI-001",
				Name:        "Cardholder Data",
				Description: "Detect cardholder data requiring PCI DSS compliance",
				PIITypes:    []types.PIIType{types.PIITypeCreditCard},
				Severity:    "critical",
				Required:    true,
				Validator:   c.validatePCICardholderData,
			},
		},
	}
	
	c.rules["PCI-DSS"] = pciRules
}

func (c *BasicComplianceChecker) registerCCPARules() {
	ccpaRules := ComplianceRuleSet{
		Regulation:  "CCPA",
		Description: "California Consumer Privacy Act",
		Rules: []ComplianceRuleDefinition{
			{
				RuleID:      "CCPA-001",
				Name:        "Personal Information",
				Description: "Detect personal information under CCPA",
				PIITypes:    []types.PIIType{types.PIITypeEmail, types.PIITypeName, types.PIITypeAddress, types.PIITypeSSN, types.PIITypeIPAddress},
				Severity:    "high",
				Required:    true,
				Validator:   c.validateCCPAPersonalInfo,
			},
		},
	}
	
	c.rules["CCPA"] = ccpaRules
}

// Rule validators

func (c *BasicComplianceChecker) validateGDPRPersonalData(scanResult *types.ScanResult) []types.ComplianceViolation {
	var violations []types.ComplianceViolation
	
	personalDataTypes := []types.PIIType{types.PIITypeEmail, types.PIITypeName, types.PIITypeAddress, types.PIITypePhoneNumber}
	
	for _, finding := range scanResult.Findings {
		for _, dataType := range personalDataTypes {
			if finding.Type == dataType && finding.RiskLevel != types.RiskLevelLow {
				violations = append(violations, types.ComplianceViolation{
					Rule:        "GDPR-001",
					PIIType:     finding.Type,
					Severity:    "high",
					Description: "Personal data detected that requires GDPR compliance measures",
					FindingIDs:  []string{finding.ID},
				})
			}
		}
	}
	
	return violations
}

func (c *BasicComplianceChecker) validateGDPRSpecialCategory(scanResult *types.ScanResult) []types.ComplianceViolation {
	var violations []types.ComplianceViolation
	
	specialDataTypes := []types.PIIType{types.PIITypeDateOfBirth, types.PIITypeSSN}
	
	for _, finding := range scanResult.Findings {
		for _, dataType := range specialDataTypes {
			if finding.Type == dataType {
				violations = append(violations, types.ComplianceViolation{
					Rule:        "GDPR-002",
					PIIType:     finding.Type,
					Severity:    "critical",
					Description: "Special category personal data detected requiring enhanced protection",
					FindingIDs:  []string{finding.ID},
				})
			}
		}
	}
	
	return violations
}

func (c *BasicComplianceChecker) validateHIPAAPHI(scanResult *types.ScanResult) []types.ComplianceViolation {
	var violations []types.ComplianceViolation
	
	phiTypes := []types.PIIType{types.PIITypeSSN, types.PIITypeDateOfBirth, types.PIITypeName, types.PIITypeEmail}
	
	for _, finding := range scanResult.Findings {
		for _, phiType := range phiTypes {
			if finding.Type == phiType {
				violations = append(violations, types.ComplianceViolation{
					Rule:        "HIPAA-001",
					PIIType:     finding.Type,
					Severity:    "critical",
					Description: "Protected Health Information detected requiring HIPAA safeguards",
					FindingIDs:  []string{finding.ID},
				})
			}
		}
	}
	
	return violations
}

func (c *BasicComplianceChecker) validatePCICardholderData(scanResult *types.ScanResult) []types.ComplianceViolation {
	var violations []types.ComplianceViolation
	
	for _, finding := range scanResult.Findings {
		if finding.Type == types.PIITypeCreditCard {
			violations = append(violations, types.ComplianceViolation{
				Rule:        "PCI-001",
				PIIType:     finding.Type,
				Severity:    "critical",
				Description: "Cardholder data detected requiring PCI DSS compliance controls",
				FindingIDs:  []string{finding.ID},
			})
		}
	}
	
	return violations
}

func (c *BasicComplianceChecker) validateCCPAPersonalInfo(scanResult *types.ScanResult) []types.ComplianceViolation {
	var violations []types.ComplianceViolation
	
	ccpaTypes := []types.PIIType{types.PIITypeEmail, types.PIITypeName, types.PIITypeAddress, types.PIITypeSSN, types.PIITypeIPAddress}
	
	for _, finding := range scanResult.Findings {
		for _, ccpaType := range ccpaTypes {
			if finding.Type == ccpaType {
				violations = append(violations, types.ComplianceViolation{
					Rule:        "CCPA-001",
					PIIType:     finding.Type,
					Severity:    "high",
					Description: "Personal information detected that is subject to CCPA consumer rights",
					FindingIDs:  []string{finding.ID},
				})
			}
		}
	}
	
	return violations
}

// Helper methods

func (c *BasicComplianceChecker) isRuleEnabled(ruleID string, rules []types.ComplianceRule) bool {
	for _, rule := range rules {
		if rule.Rule == ruleID {
			return true
		}
		// Also check if regulation is enabled (enables all rules for that regulation)
		if strings.Contains(ruleID, rule.Regulation) {
			return true
		}
	}
	return false
}

func (c *BasicComplianceChecker) generateRequiredActions(ruleDef ComplianceRuleDefinition, violations []types.ComplianceViolation) []string {
	var actions []string
	
	switch ruleDef.RuleID {
	case "GDPR-001", "GDPR-002":
		actions = append(actions, "Implement data protection measures")
		actions = append(actions, "Document legal basis for processing")
		
	case "HIPAA-001":
		actions = append(actions, "Apply HIPAA safeguards")
		actions = append(actions, "Implement access controls")
		
	case "PCI-001":
		actions = append(actions, "Encrypt cardholder data")
		actions = append(actions, "Implement PCI DSS controls")
		
	case "CCPA-001":
		actions = append(actions, "Enable consumer rights mechanisms")
		actions = append(actions, "Implement data deletion capabilities")
	}
	
	return actions
}