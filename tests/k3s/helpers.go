package k3s

import (
	"github.com/jscharber/audimodal/pkg/dlp/types"
)

// FilterViolationsByRule filters violations by rule prefix (e.g., "GDPR", "HIPAA")
func FilterViolationsByRule(violations []Violation, rulePrefix string) []Violation {
	var filtered []Violation
	for _, v := range violations {
		if len(v.Rule) >= len(rulePrefix) && v.Rule[:len(rulePrefix)] == rulePrefix {
			filtered = append(filtered, v)
		}
	}
	return filtered
}

// FilterViolationsByRegulation filters violations by regulation
func FilterViolationsByRegulation(violations []Violation, regulation string) []Violation {
	var filtered []Violation
	for _, v := range violations {
		if v.Regulation == regulation {
			filtered = append(filtered, v)
		}
	}
	return filtered
}

// FilterViolationsBySeverity filters violations by severity
func FilterViolationsBySeverity(violations []Violation, severity string) []Violation {
	var filtered []Violation
	for _, v := range violations {
		if v.Severity == severity {
			filtered = append(filtered, v)
		}
	}
	return filtered
}

// FilterViolationsByPIIType filters violations by PII type
func FilterViolationsByPIIType(violations []Violation, piiType types.PIIType) []Violation {
	var filtered []Violation
	for _, v := range violations {
		if v.PIIType == piiType {
			filtered = append(filtered, v)
		}
	}
	return filtered
}

// ExtractPIITypes extracts unique PII types from violations
func ExtractPIITypes(violations []Violation) []types.PIIType {
	seen := make(map[types.PIIType]bool)
	var piiTypes []types.PIIType

	for _, v := range violations {
		if !seen[v.PIIType] {
			seen[v.PIIType] = true
			piiTypes = append(piiTypes, v.PIIType)
		}
	}

	return piiTypes
}

// ExtractRules extracts unique rules from violations
func ExtractRules(violations []Violation) []string {
	seen := make(map[string]bool)
	var rules []string

	for _, v := range violations {
		if !seen[v.Rule] {
			seen[v.Rule] = true
			rules = append(rules, v.Rule)
		}
	}

	return rules
}

// ExtractRegulations extracts unique regulations from violations
func ExtractRegulations(violations []Violation) []string {
	seen := make(map[string]bool)
	var regulations []string

	for _, v := range violations {
		if !seen[v.Regulation] {
			seen[v.Regulation] = true
			regulations = append(regulations, v.Regulation)
		}
	}

	return regulations
}

// ContainsPIIType checks if any violation contains the given PII type
func ContainsPIIType(violations []Violation, piiType types.PIIType) bool {
	for _, v := range violations {
		if v.PIIType == piiType {
			return true
		}
	}
	return false
}

// ContainsRule checks if any violation contains the given rule
func ContainsRule(violations []Violation, rule string) bool {
	for _, v := range violations {
		if v.Rule == rule {
			return true
		}
	}
	return false
}

// CountViolationsByRegulation counts violations grouped by regulation
func CountViolationsByRegulation(violations []Violation) map[string]int {
	counts := make(map[string]int)
	for _, v := range violations {
		counts[v.Regulation]++
	}
	return counts
}

// CountViolationsBySeverity counts violations grouped by severity
func CountViolationsBySeverity(violations []Violation) map[string]int {
	counts := make(map[string]int)
	for _, v := range violations {
		counts[v.Severity]++
	}
	return counts
}

// HasViolations returns true if there are any violations
func HasViolations(resp *ViolationsResponse) bool {
	return resp != nil && len(resp.Violations) > 0
}

// IsCompliant returns true if the response indicates compliance
func IsCompliant(resp *ViolationsResponse) bool {
	return resp != nil && resp.IsCompliant
}
