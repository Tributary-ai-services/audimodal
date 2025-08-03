package classification

import (
	"context"
	"regexp"
	"strings"
)

// SensitivityClassifier provides advanced content sensitivity classification
type SensitivityClassifier struct {
	name               string
	version            string
	sensitivityRules   map[SensitivityLevel]*SensitivityRuleSet
	complianceAnalyzer *ComplianceAnalyzer
	piiDetector        *PIIDetector
	dataClassifier     *DataClassifier
}

// SensitivityClassification represents detailed sensitivity classification results
type SensitivityClassification struct {
	Level                SensitivityLevel      `json:"level"`
	Score                float64               `json:"score"`
	Confidence           float64               `json:"confidence"`
	Reasons              []SensitivityReason   `json:"reasons"`
	ComplianceFlags      []ComplianceFramework `json:"compliance_flags"`
	ComplianceRisks      []ComplianceRisk      `json:"compliance_risks"`
	PIIFindings          []PIIFinding          `json:"pii_findings"`
	DataClassifications  []DataClassification  `json:"data_classifications"`
	RecommendedActions   []string              `json:"recommended_actions"`
	HandlingInstructions []string              `json:"handling_instructions"`
}

// SensitivityReason represents why content received a sensitivity classification
type SensitivityReason struct {
	Type        string  `json:"type"` // pattern, pii, context, metadata
	Description string  `json:"description"`
	Evidence    string  `json:"evidence,omitempty"`
	Confidence  float64 `json:"confidence"`
	Weight      float64 `json:"weight"`
}

// SensitivityRuleSet contains rules for a specific sensitivity level
type SensitivityRuleSet struct {
	Level           SensitivityLevel `json:"level"`
	ThresholdScore  float64          `json:"threshold_score"`
	PatternRules    []PatternRule    `json:"pattern_rules"`
	ContextRules    []ContextRule    `json:"context_rules"`
	MetadataRules   []MetadataRule   `json:"metadata_rules"`
	PIIRequirements PIIRequirements  `json:"pii_requirements"`
}

// PatternRule represents a pattern-based sensitivity rule
type PatternRule struct {
	Name        string         `json:"name"`
	Pattern     *regexp.Regexp `json:"-"`
	PatternStr  string         `json:"pattern"`
	Score       float64        `json:"score"`
	Weight      float64        `json:"weight"`
	Category    string         `json:"category"`
	Description string         `json:"description"`
}

// ContextRule represents a context-based sensitivity rule
type ContextRule struct {
	Name           string          `json:"name"`
	RequiredWords  []string        `json:"required_words"`
	ProximityRules []ProximityRule `json:"proximity_rules"`
	Score          float64         `json:"score"`
	Weight         float64         `json:"weight"`
	Description    string          `json:"description"`
}

// ProximityRule defines word proximity requirements
type ProximityRule struct {
	Word1       string `json:"word1"`
	Word2       string `json:"word2"`
	MaxDistance int    `json:"max_distance"` // in words
}

// MetadataRule represents a metadata-based sensitivity rule
type MetadataRule struct {
	Name        string   `json:"name"`
	Field       string   `json:"field"`
	Values      []string `json:"values"`
	Pattern     string   `json:"pattern,omitempty"`
	Score       float64  `json:"score"`
	Weight      float64  `json:"weight"`
	Description string   `json:"description"`
}

// PIIRequirements defines PII requirements for sensitivity levels
type PIIRequirements struct {
	MinPIICount      int      `json:"min_pii_count"`
	RequiredPIITypes []string `json:"required_pii_types"`
	HighRiskPIITypes []string `json:"high_risk_pii_types"`
	ScoreMultiplier  float64  `json:"score_multiplier"`
}

// PIIFinding represents a PII detection finding
type PIIFinding struct {
	Type        string  `json:"type"`
	Value       string  `json:"value"`
	MaskedValue string  `json:"masked_value"`
	Position    int     `json:"position"`
	Confidence  float64 `json:"confidence"`
	RiskLevel   string  `json:"risk_level"`
}

// DataClassification represents data classification results
type DataClassification struct {
	Type       string  `json:"type"` // financial, medical, personal, etc.
	Confidence float64 `json:"confidence"`
	Evidence   string  `json:"evidence"`
	Regulation string  `json:"regulation,omitempty"`
	RiskLevel  string  `json:"risk_level"`
}

// ComplianceAnalyzer analyzes regulatory compliance requirements
type ComplianceAnalyzer struct {
	frameworks map[ComplianceFramework]*ComplianceFrameworkRules
}

// ComplianceFrameworkRules contains rules for a specific compliance framework
type ComplianceFrameworkRules struct {
	Framework       ComplianceFramework `json:"framework"`
	TriggerPatterns []string            `json:"trigger_patterns"`
	RequiredPII     []string            `json:"required_pii"`
	DataTypes       []string            `json:"data_types"`
	RiskFactors     []string            `json:"risk_factors"`
}

// PIIDetector detects various types of PII
type PIIDetector struct {
	patterns map[string]*PIIPattern
}

// PIIPattern represents a PII detection pattern
type PIIPattern struct {
	Name       string              `json:"name"`
	Pattern    *regexp.Regexp      `json:"-"`
	PatternStr string              `json:"pattern"`
	RiskLevel  string              `json:"risk_level"`
	MaskFunc   func(string) string `json:"-"`
}

// DataClassifier classifies data types
type DataClassifier struct {
	classifiers map[string]*DataTypeClassifier
}

// DataTypeClassifier classifies specific data types
type DataTypeClassifier struct {
	Name        string           `json:"name"`
	Patterns    []*regexp.Regexp `json:"-"`
	PatternStrs []string         `json:"patterns"`
	Keywords    []string         `json:"keywords"`
	Context     []string         `json:"context"`
	RiskLevel   string           `json:"risk_level"`
	Regulations []string         `json:"regulations"`
}

// NewSensitivityClassifier creates a new sensitivity classifier
func NewSensitivityClassifier() *SensitivityClassifier {
	classifier := &SensitivityClassifier{
		name:               "advanced-sensitivity-classifier",
		version:            "1.0.0",
		sensitivityRules:   make(map[SensitivityLevel]*SensitivityRuleSet),
		complianceAnalyzer: NewComplianceAnalyzer(),
		piiDetector:        NewPIIDetector(),
		dataClassifier:     NewDataClassifier(),
	}

	classifier.initializeSensitivityRules()
	return classifier
}

// ClassifySensitivity performs comprehensive sensitivity classification
func (s *SensitivityClassifier) ClassifySensitivity(ctx context.Context, input *ClassificationInput) (*SensitivityClassification, error) {
	result := &SensitivityClassification{
		Level:                SensitivityPublic,
		Score:                0.0,
		Confidence:           0.0,
		Reasons:              []SensitivityReason{},
		ComplianceFlags:      []ComplianceFramework{},
		ComplianceRisks:      []ComplianceRisk{},
		PIIFindings:          []PIIFinding{},
		DataClassifications:  []DataClassification{},
		RecommendedActions:   []string{},
		HandlingInstructions: []string{},
	}

	if len(input.Content) == 0 {
		return result, nil
	}

	// Detect PII
	piiFindings := s.piiDetector.DetectPII(input.Content)
	result.PIIFindings = piiFindings

	// Classify data types
	dataClassifications := s.dataClassifier.ClassifyData(input.Content)
	result.DataClassifications = dataClassifications

	// Analyze compliance requirements
	complianceResult := s.complianceAnalyzer.AnalyzeCompliance(input.Content, piiFindings, dataClassifications)
	result.ComplianceFlags = complianceResult.Frameworks
	result.ComplianceRisks = complianceResult.Risks

	// Calculate sensitivity scores for each level
	levelScores := make(map[SensitivityLevel]float64)
	levelReasons := make(map[SensitivityLevel][]SensitivityReason)

	for level, ruleSet := range s.sensitivityRules {
		score, reasons := s.calculateLevelScore(input, ruleSet, piiFindings, dataClassifications)
		levelScores[level] = score
		levelReasons[level] = reasons
	}

	// Determine final sensitivity level
	finalLevel, finalScore, confidence := s.determineFinalLevel(levelScores)
	result.Level = finalLevel
	result.Score = finalScore
	result.Confidence = confidence
	result.Reasons = levelReasons[finalLevel]

	// Generate recommendations and handling instructions
	result.RecommendedActions = s.generateRecommendations(result)
	result.HandlingInstructions = s.generateHandlingInstructions(result)

	return result, nil
}

// calculateLevelScore calculates the score for a specific sensitivity level
func (s *SensitivityClassifier) calculateLevelScore(input *ClassificationInput, ruleSet *SensitivityRuleSet, piiFindings []PIIFinding, dataClassifications []DataClassification) (float64, []SensitivityReason) {
	totalScore := 0.0
	reasons := []SensitivityReason{}

	// Evaluate pattern rules
	for _, rule := range ruleSet.PatternRules {
		if rule.Pattern.MatchString(input.Content) {
			score := rule.Score * rule.Weight
			totalScore += score

			reasons = append(reasons, SensitivityReason{
				Type:        "pattern",
				Description: rule.Description,
				Evidence:    rule.Name,
				Confidence:  0.9,
				Weight:      rule.Weight,
			})
		}
	}

	// Evaluate context rules
	for _, rule := range ruleSet.ContextRules {
		if s.evaluateContextRule(input.Content, rule) {
			score := rule.Score * rule.Weight
			totalScore += score

			reasons = append(reasons, SensitivityReason{
				Type:        "context",
				Description: rule.Description,
				Evidence:    rule.Name,
				Confidence:  0.8,
				Weight:      rule.Weight,
			})
		}
	}

	// Evaluate metadata rules
	if input.Metadata != nil {
		for _, rule := range ruleSet.MetadataRules {
			if s.evaluateMetadataRule(input.Metadata, rule) {
				score := rule.Score * rule.Weight
				totalScore += score

				reasons = append(reasons, SensitivityReason{
					Type:        "metadata",
					Description: rule.Description,
					Evidence:    rule.Name,
					Confidence:  1.0,
					Weight:      rule.Weight,
				})
			}
		}
	}

	// Evaluate PII requirements
	piiScore := s.evaluatePIIRequirements(piiFindings, ruleSet.PIIRequirements)
	if piiScore > 0 {
		totalScore += piiScore

		reasons = append(reasons, SensitivityReason{
			Type:        "pii",
			Description: "PII detected matching sensitivity requirements",
			Evidence:    "PII analysis",
			Confidence:  0.95,
			Weight:      ruleSet.PIIRequirements.ScoreMultiplier,
		})
	}

	// Evaluate data classifications
	for _, dataClass := range dataClassifications {
		if dataClass.RiskLevel == "high" {
			totalScore += 0.3 * dataClass.Confidence

			reasons = append(reasons, SensitivityReason{
				Type:        "data_classification",
				Description: "High-risk data type detected: " + dataClass.Type,
				Evidence:    dataClass.Evidence,
				Confidence:  dataClass.Confidence,
				Weight:      0.3,
			})
		}
	}

	return totalScore, reasons
}

// evaluateContextRule evaluates a context-based rule
func (s *SensitivityClassifier) evaluateContextRule(content string, rule ContextRule) bool {
	lowerContent := strings.ToLower(content)

	// Check if all required words are present
	for _, word := range rule.RequiredWords {
		if !strings.Contains(lowerContent, strings.ToLower(word)) {
			return false
		}
	}

	// Check proximity rules
	for _, proxRule := range rule.ProximityRules {
		if !s.checkWordProximity(lowerContent, strings.ToLower(proxRule.Word1), strings.ToLower(proxRule.Word2), proxRule.MaxDistance) {
			return false
		}
	}

	return true
}

// checkWordProximity checks if two words appear within a specified distance
func (s *SensitivityClassifier) checkWordProximity(content, word1, word2 string, maxDistance int) bool {
	words := strings.Fields(content)

	for i, word := range words {
		if strings.Contains(word, word1) {
			// Look for word2 within maxDistance
			start := maxInt(0, i-maxDistance)
			end := minInt(len(words), i+maxDistance+1)

			for j := start; j < end; j++ {
				if j != i && strings.Contains(words[j], word2) {
					return true
				}
			}
		}
	}

	return false
}

// evaluateMetadataRule evaluates a metadata-based rule
func (s *SensitivityClassifier) evaluateMetadataRule(metadata map[string]string, rule MetadataRule) bool {
	value, exists := metadata[rule.Field]
	if !exists {
		return false
	}

	// Check against specific values
	for _, ruleValue := range rule.Values {
		if strings.EqualFold(value, ruleValue) {
			return true
		}
	}

	// Check against pattern if specified
	if rule.Pattern != "" {
		if matched, _ := regexp.MatchString(rule.Pattern, value); matched {
			return true
		}
	}

	return false
}

// evaluatePIIRequirements evaluates PII requirements for a sensitivity level
func (s *SensitivityClassifier) evaluatePIIRequirements(piiFindings []PIIFinding, requirements PIIRequirements) float64 {
	if len(piiFindings) < requirements.MinPIICount {
		return 0.0
	}

	score := 0.0

	// Check for required PII types
	foundTypes := make(map[string]bool)
	for _, finding := range piiFindings {
		foundTypes[finding.Type] = true

		// Give extra score for high-risk PII types
		for _, highRiskType := range requirements.HighRiskPIITypes {
			if finding.Type == highRiskType {
				score += 0.5 * finding.Confidence
			}
		}
	}

	// Check if all required types are present
	requiredFound := 0
	for _, requiredType := range requirements.RequiredPIITypes {
		if foundTypes[requiredType] {
			requiredFound++
		}
	}

	if requiredFound == len(requirements.RequiredPIITypes) {
		score += requirements.ScoreMultiplier
	}

	return score
}

// determineFinalLevel determines the final sensitivity level based on scores
func (s *SensitivityClassifier) determineFinalLevel(levelScores map[SensitivityLevel]float64) (SensitivityLevel, float64, float64) {
	// Define level hierarchy and thresholds
	levels := []SensitivityLevel{
		SensitivityTopSecret,
		SensitivityRestricted,
		SensitivityConfidential,
		SensitivityInternal,
		SensitivityPublic,
	}

	thresholds := map[SensitivityLevel]float64{
		SensitivityTopSecret:    0.9,
		SensitivityRestricted:   0.7,
		SensitivityConfidential: 0.5,
		SensitivityInternal:     0.3,
		SensitivityPublic:       0.0,
	}

	// Find the highest level that meets the threshold
	for _, level := range levels {
		score := levelScores[level]
		threshold := thresholds[level]

		if score >= threshold {
			confidence := minFloat(score/threshold, 1.0)
			return level, score, confidence
		}
	}

	return SensitivityPublic, levelScores[SensitivityPublic], 0.5
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// generateRecommendations generates recommended actions based on classification
func (s *SensitivityClassifier) generateRecommendations(result *SensitivityClassification) []string {
	recommendations := []string{}

	switch result.Level {
	case SensitivityTopSecret:
		recommendations = append(recommendations, "Implement maximum security controls")
		recommendations = append(recommendations, "Restrict access to authorized personnel only")
		recommendations = append(recommendations, "Enable audit logging for all access")
		recommendations = append(recommendations, "Encrypt data at rest and in transit")
	case SensitivityRestricted:
		recommendations = append(recommendations, "Apply strong access controls")
		recommendations = append(recommendations, "Enable monitoring and alerting")
		recommendations = append(recommendations, "Encrypt sensitive data")
	case SensitivityConfidential:
		recommendations = append(recommendations, "Implement access controls")
		recommendations = append(recommendations, "Monitor access patterns")
		recommendations = append(recommendations, "Consider data encryption")
	case SensitivityInternal:
		recommendations = append(recommendations, "Restrict to internal users only")
		recommendations = append(recommendations, "Apply basic access controls")
	case SensitivityPublic:
		recommendations = append(recommendations, "Standard security measures sufficient")
	}

	// Add PII-specific recommendations
	if len(result.PIIFindings) > 0 {
		recommendations = append(recommendations, "Implement PII protection measures")
		recommendations = append(recommendations, "Consider data masking or tokenization")
	}

	// Add compliance-specific recommendations
	for _, framework := range result.ComplianceFlags {
		switch framework {
		case ComplianceGDPR:
			recommendations = append(recommendations, "Ensure GDPR compliance measures")
		case ComplianceHIPAA:
			recommendations = append(recommendations, "Apply HIPAA safeguards")
		case CompliancePCI:
			recommendations = append(recommendations, "Implement PCI DSS controls")
		}
	}

	return recommendations
}

// generateHandlingInstructions generates handling instructions
func (s *SensitivityClassifier) generateHandlingInstructions(result *SensitivityClassification) []string {
	instructions := []string{}

	switch result.Level {
	case SensitivityTopSecret:
		instructions = append(instructions, "Handle as TOP SECRET - maximum protection required")
		instructions = append(instructions, "Do not transmit over unsecured channels")
		instructions = append(instructions, "Store in approved secure facilities only")
	case SensitivityRestricted:
		instructions = append(instructions, "Handle as RESTRICTED - limited distribution")
		instructions = append(instructions, "Encrypt during transmission and storage")
	case SensitivityConfidential:
		instructions = append(instructions, "Handle as CONFIDENTIAL - controlled access")
		instructions = append(instructions, "Protect from unauthorized disclosure")
	case SensitivityInternal:
		instructions = append(instructions, "Handle as INTERNAL - organization use only")
	case SensitivityPublic:
		instructions = append(instructions, "Standard handling procedures apply")
	}

	return instructions
}

// initializeSensitivityRules initializes sensitivity classification rules
func (s *SensitivityClassifier) initializeSensitivityRules() {
	// Top Secret rules
	s.sensitivityRules[SensitivityTopSecret] = &SensitivityRuleSet{
		Level:          SensitivityTopSecret,
		ThresholdScore: 0.9,
		PatternRules: []PatternRule{
			{
				Name:        "top_secret_marking",
				Pattern:     regexp.MustCompile(`(?i)(top\s+secret|ts|classification:\s*top\s*secret)`),
				PatternStr:  `(?i)(top\s+secret|ts|classification:\s*top\s*secret)`,
				Score:       1.0,
				Weight:      1.0,
				Category:    "classification_marking",
				Description: "Top Secret classification marking detected",
			},
			{
				Name:        "national_security",
				Pattern:     regexp.MustCompile(`(?i)(national\s+security|defense\s+intelligence|military\s+classified)`),
				PatternStr:  `(?i)(national\s+security|defense\s+intelligence|military\s+classified)`,
				Score:       0.9,
				Weight:      0.8,
				Category:    "security_content",
				Description: "National security content detected",
			},
		},
		PIIRequirements: PIIRequirements{
			MinPIICount:      3,
			RequiredPIITypes: []string{"ssn", "passport", "security_clearance"},
			HighRiskPIITypes: []string{"security_clearance", "classified_access"},
			ScoreMultiplier:  0.8,
		},
	}

	// Restricted rules
	s.sensitivityRules[SensitivityRestricted] = &SensitivityRuleSet{
		Level:          SensitivityRestricted,
		ThresholdScore: 0.7,
		PatternRules: []PatternRule{
			{
				Name:        "restricted_marking",
				Pattern:     regexp.MustCompile(`(?i)(restricted|classification:\s*restricted)`),
				PatternStr:  `(?i)(restricted|classification:\s*restricted)`,
				Score:       0.8,
				Weight:      1.0,
				Category:    "classification_marking",
				Description: "Restricted classification marking detected",
			},
			{
				Name:        "sensitive_financial",
				Pattern:     regexp.MustCompile(`(?i)(trade\s+secret|proprietary|confidential\s+financial)`),
				PatternStr:  `(?i)(trade\s+secret|proprietary|confidential\s+financial)`,
				Score:       0.7,
				Weight:      0.7,
				Category:    "business_sensitive",
				Description: "Sensitive business information detected",
			},
		},
		PIIRequirements: PIIRequirements{
			MinPIICount:      2,
			RequiredPIITypes: []string{"ssn", "credit_card"},
			HighRiskPIITypes: []string{"ssn", "passport", "credit_card"},
			ScoreMultiplier:  0.6,
		},
	}

	// Confidential rules
	s.sensitivityRules[SensitivityConfidential] = &SensitivityRuleSet{
		Level:          SensitivityConfidential,
		ThresholdScore: 0.5,
		PatternRules: []PatternRule{
			{
				Name:        "confidential_marking",
				Pattern:     regexp.MustCompile(`(?i)(confidential|classification:\s*confidential)`),
				PatternStr:  `(?i)(confidential|classification:\s*confidential)`,
				Score:       0.6,
				Weight:      1.0,
				Category:    "classification_marking",
				Description: "Confidential classification marking detected",
			},
			{
				Name:        "medical_records",
				Pattern:     regexp.MustCompile(`(?i)(medical\s+record|patient\s+information|health\s+record)`),
				PatternStr:  `(?i)(medical\s+record|patient\s+information|health\s+record)`,
				Score:       0.7,
				Weight:      0.8,
				Category:    "medical_data",
				Description: "Medical information detected",
			},
		},
		PIIRequirements: PIIRequirements{
			MinPIICount:      1,
			RequiredPIITypes: []string{"email", "phone", "name"},
			HighRiskPIITypes: []string{"ssn", "credit_card", "medical_id"},
			ScoreMultiplier:  0.4,
		},
	}

	// Internal rules
	s.sensitivityRules[SensitivityInternal] = &SensitivityRuleSet{
		Level:          SensitivityInternal,
		ThresholdScore: 0.3,
		PatternRules: []PatternRule{
			{
				Name:        "internal_marking",
				Pattern:     regexp.MustCompile(`(?i)(internal\s+use|company\s+confidential|internal\s+only)`),
				PatternStr:  `(?i)(internal\s+use|company\s+confidential|internal\s+only)`,
				Score:       0.4,
				Weight:      1.0,
				Category:    "classification_marking",
				Description: "Internal use marking detected",
			},
		},
		PIIRequirements: PIIRequirements{
			MinPIICount:      0,
			RequiredPIITypes: []string{},
			HighRiskPIITypes: []string{},
			ScoreMultiplier:  0.2,
		},
	}

	// Public rules
	s.sensitivityRules[SensitivityPublic] = &SensitivityRuleSet{
		Level:          SensitivityPublic,
		ThresholdScore: 0.0,
		PatternRules:   []PatternRule{},
		PIIRequirements: PIIRequirements{
			MinPIICount:      0,
			RequiredPIITypes: []string{},
			HighRiskPIITypes: []string{},
			ScoreMultiplier:  0.0,
		},
	}
}

// NewComplianceAnalyzer creates a new compliance analyzer
func NewComplianceAnalyzer() *ComplianceAnalyzer {
	analyzer := &ComplianceAnalyzer{
		frameworks: make(map[ComplianceFramework]*ComplianceFrameworkRules),
	}
	analyzer.initializeFrameworks()
	return analyzer
}

// ComplianceResult represents compliance analysis results
type ComplianceResult struct {
	Frameworks []ComplianceFramework `json:"frameworks"`
	Risks      []ComplianceRisk      `json:"risks"`
}

// AnalyzeCompliance analyzes compliance requirements
func (c *ComplianceAnalyzer) AnalyzeCompliance(content string, piiFindings []PIIFinding, dataClassifications []DataClassification) *ComplianceResult {
	result := &ComplianceResult{
		Frameworks: []ComplianceFramework{},
		Risks:      []ComplianceRisk{},
	}

	lowerContent := strings.ToLower(content)

	for framework, rules := range c.frameworks {
		triggered := false

		// Check trigger patterns
		for _, pattern := range rules.TriggerPatterns {
			if strings.Contains(lowerContent, strings.ToLower(pattern)) {
				triggered = true
				break
			}
		}

		// Check PII requirements
		if !triggered {
			for _, finding := range piiFindings {
				for _, requiredPII := range rules.RequiredPII {
					if finding.Type == requiredPII {
						triggered = true
						break
					}
				}
				if triggered {
					break
				}
			}
		}

		// Check data type requirements
		if !triggered {
			for _, dataClass := range dataClassifications {
				for _, dataType := range rules.DataTypes {
					if dataClass.Type == dataType {
						triggered = true
						break
					}
				}
				if triggered {
					break
				}
			}
		}

		if triggered {
			result.Frameworks = append(result.Frameworks, framework)

			// Generate compliance risk
			risk := ComplianceRisk{
				Framework:   framework,
				RiskLevel:   "medium",
				Description: "Content subject to " + string(framework) + " requirements",
				Mitigation:  []string{"Implement appropriate controls", "Ensure compliance monitoring"},
			}
			result.Risks = append(result.Risks, risk)
		}
	}

	return result
}

// initializeFrameworks initializes compliance framework rules
func (c *ComplianceAnalyzer) initializeFrameworks() {
	c.frameworks[ComplianceGDPR] = &ComplianceFrameworkRules{
		Framework:       ComplianceGDPR,
		TriggerPatterns: []string{"personal data", "eu citizen", "european union"},
		RequiredPII:     []string{"email", "name", "phone"},
		DataTypes:       []string{"personal", "contact"},
		RiskFactors:     []string{"cross_border_transfer", "automated_decision"},
	}

	c.frameworks[ComplianceHIPAA] = &ComplianceFrameworkRules{
		Framework:       ComplianceHIPAA,
		TriggerPatterns: []string{"health information", "medical record", "patient data"},
		RequiredPII:     []string{"medical_id", "health_record"},
		DataTypes:       []string{"medical", "health"},
		RiskFactors:     []string{"phi_disclosure", "unauthorized_access"},
	}

	c.frameworks[CompliancePCI] = &ComplianceFrameworkRules{
		Framework:       CompliancePCI,
		TriggerPatterns: []string{"payment card", "credit card", "cardholder data"},
		RequiredPII:     []string{"credit_card", "payment_info"},
		DataTypes:       []string{"payment", "financial"},
		RiskFactors:     []string{"card_data_exposure", "payment_processing"},
	}
}

// NewPIIDetector creates a new PII detector
func NewPIIDetector() *PIIDetector {
	detector := &PIIDetector{
		patterns: make(map[string]*PIIPattern),
	}
	detector.initializePatterns()
	return detector
}

// DetectPII detects PII in content
func (p *PIIDetector) DetectPII(content string) []PIIFinding {
	findings := []PIIFinding{}

	for name, pattern := range p.patterns {
		matches := pattern.Pattern.FindAllStringSubmatch(content, -1)
		matchIndices := pattern.Pattern.FindAllStringSubmatchIndex(content, -1)

		for i, match := range matches {
			if len(match) > 0 {
				value := match[0]
				maskedValue := pattern.MaskFunc(value)
				position := 0

				if i < len(matchIndices) && len(matchIndices[i]) >= 2 {
					position = matchIndices[i][0]
				}

				findings = append(findings, PIIFinding{
					Type:        name,
					Value:       value,
					MaskedValue: maskedValue,
					Position:    position,
					Confidence:  0.9, // Could be made more sophisticated
					RiskLevel:   pattern.RiskLevel,
				})
			}
		}
	}

	return findings
}

// initializePatterns initializes PII detection patterns
func (p *PIIDetector) initializePatterns() {
	p.patterns["ssn"] = &PIIPattern{
		Name:       "ssn",
		Pattern:    regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),
		PatternStr: `\b\d{3}-\d{2}-\d{4}\b`,
		RiskLevel:  "high",
		MaskFunc:   func(s string) string { return "***-**-****" },
	}

	p.patterns["credit_card"] = &PIIPattern{
		Name:       "credit_card",
		Pattern:    regexp.MustCompile(`\b4\d{3}[-\s]?\d{4}[-\s]?\d{4}[-\s]?\d{4}\b`),
		PatternStr: `\b4\d{3}[-\s]?\d{4}[-\s]?\d{4}[-\s]?\d{4}\b`,
		RiskLevel:  "high",
		MaskFunc:   func(s string) string { return "****-****-****-" + s[len(s)-4:] },
	}

	p.patterns["email"] = &PIIPattern{
		Name:       "email",
		Pattern:    regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`),
		PatternStr: `\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`,
		RiskLevel:  "medium",
		MaskFunc: func(s string) string {
			parts := strings.Split(s, "@")
			if len(parts) == 2 {
				return "***@" + parts[1]
			}
			return "***@***.***"
		},
	}

	p.patterns["phone"] = &PIIPattern{
		Name:       "phone",
		Pattern:    regexp.MustCompile(`\b\d{3}[-.]?\d{3}[-.]?\d{4}\b`),
		PatternStr: `\b\d{3}[-.]?\d{3}[-.]?\d{4}\b`,
		RiskLevel:  "medium",
		MaskFunc:   func(s string) string { return "***-***-****" },
	}
}

// NewDataClassifier creates a new data classifier
func NewDataClassifier() *DataClassifier {
	classifier := &DataClassifier{
		classifiers: make(map[string]*DataTypeClassifier),
	}
	classifier.initializeClassifiers()
	return classifier
}

// ClassifyData classifies data types in content
func (d *DataClassifier) ClassifyData(content string) []DataClassification {
	classifications := []DataClassification{}

	lowerContent := strings.ToLower(content)

	for name, classifier := range d.classifiers {
		confidence := 0.0
		evidence := ""

		// Check patterns
		for i, pattern := range classifier.Patterns {
			if pattern.MatchString(content) {
				confidence += 0.3
				evidence = classifier.PatternStrs[i]
			}
		}

		// Check keywords
		for _, keyword := range classifier.Keywords {
			if strings.Contains(lowerContent, strings.ToLower(keyword)) {
				confidence += 0.2
				if evidence != "" {
					evidence += ", "
				}
				evidence += keyword
			}
		}

		// Check context
		for _, context := range classifier.Context {
			if strings.Contains(lowerContent, strings.ToLower(context)) {
				confidence += 0.1
			}
		}

		if confidence > 0.3 {
			classifications = append(classifications, DataClassification{
				Type:       name,
				Confidence: minFloat(confidence, 1.0),
				Evidence:   evidence,
				RiskLevel:  classifier.RiskLevel,
			})
		}
	}

	return classifications
}

// initializeClassifiers initializes data type classifiers
func (d *DataClassifier) initializeClassifiers() {
	d.classifiers["financial"] = &DataTypeClassifier{
		Name:        "financial",
		Patterns:    []*regexp.Regexp{regexp.MustCompile(`\$\d+`), regexp.MustCompile(`\d+\.\d{2}`)},
		PatternStrs: []string{`\$\d+`, `\d+\.\d{2}`},
		Keywords:    []string{"bank", "account", "payment", "financial", "money", "credit"},
		Context:     []string{"transaction", "invoice", "statement"},
		RiskLevel:   "high",
		Regulations: []string{"PCI", "SOX"},
	}

	d.classifiers["medical"] = &DataTypeClassifier{
		Name:        "medical",
		Patterns:    []*regexp.Regexp{regexp.MustCompile(`(?i)patient\s+\w+`)},
		PatternStrs: []string{`(?i)patient\s+\w+`},
		Keywords:    []string{"patient", "medical", "health", "diagnosis", "treatment"},
		Context:     []string{"hospital", "clinic", "doctor", "physician"},
		RiskLevel:   "high",
		Regulations: []string{"HIPAA"},
	}

	d.classifiers["personal"] = &DataTypeClassifier{
		Name:        "personal",
		Patterns:    []*regexp.Regexp{regexp.MustCompile(`(?i)name:\s*\w+`)},
		PatternStrs: []string{`(?i)name:\s*\w+`},
		Keywords:    []string{"name", "address", "personal", "contact", "individual"},
		Context:     []string{"profile", "contact", "identity"},
		RiskLevel:   "medium",
		Regulations: []string{"GDPR", "CCPA"},
	}
}
