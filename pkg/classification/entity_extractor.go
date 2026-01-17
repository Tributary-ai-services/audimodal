package classification

import (
	"context"
	"regexp"
	"strings"
)

// BasicEntityExtractor provides basic entity extraction using pattern matching
type BasicEntityExtractor struct {
	name               string
	version            string
	supportedTypes     []EntityType
	patterns           map[EntityType][]*regexp.Regexp
	contextualPatterns map[EntityType][]string
}

// NewBasicEntityExtractor creates a new basic entity extractor
func NewBasicEntityExtractor() *BasicEntityExtractor {
	extractor := &BasicEntityExtractor{
		name:    "basic-entity-extractor",
		version: "1.0.0",
		supportedTypes: []EntityType{
			EntityTypePerson, EntityTypeOrganization, EntityTypeLocation,
			EntityTypeDate, EntityTypeMoney, EntityTypePercentage,
			EntityTypePhone, EntityTypeEmail, EntityTypeURL,
			EntityTypeIPAddress, EntityTypeCreditCard, EntityTypeSSN,
		},
		patterns:           make(map[EntityType][]*regexp.Regexp),
		contextualPatterns: make(map[EntityType][]string),
	}

	extractor.initializePatterns()
	return extractor
}

// ExtractEntities extracts entities from the given text
func (e *BasicEntityExtractor) ExtractEntities(ctx context.Context, text string) ([]Entity, error) {
	if len(text) == 0 {
		return []Entity{}, nil
	}

	entities := []Entity{}

	// Extract entities for each supported type
	for _, entityType := range e.supportedTypes {
		typeEntities := e.extractEntitiesOfType(text, entityType)
		entities = append(entities, typeEntities...)
	}

	// Remove duplicates and overlapping entities
	entities = e.removeDuplicates(entities)

	// Sort by position
	for i := range entities {
		for j := i + 1; j < len(entities); j++ {
			if len(entities[i].Positions) > 0 && len(entities[j].Positions) > 0 {
				if entities[i].Positions[0].Start > entities[j].Positions[0].Start {
					entities[i], entities[j] = entities[j], entities[i]
				}
			}
		}
	}

	return entities, nil
}

// GetSupportedEntityTypes returns supported entity types
func (e *BasicEntityExtractor) GetSupportedEntityTypes() []EntityType {
	return e.supportedTypes
}

// extractEntitiesOfType extracts entities of a specific type
func (e *BasicEntityExtractor) extractEntitiesOfType(text string, entityType EntityType) []Entity {
	entities := []Entity{}

	patterns, exists := e.patterns[entityType]
	if !exists {
		return entities
	}

	for _, pattern := range patterns {
		matches := pattern.FindAllStringSubmatch(text, -1)
		matchIndices := pattern.FindAllStringSubmatchIndex(text, -1)

		for i, match := range matches {
			if len(match) > 0 {
				entityText := match[0]
				if len(match) > 1 && match[1] != "" {
					entityText = match[1] // Use capture group if available
				}

				// Calculate confidence based on pattern match and context
				confidence := e.calculateEntityConfidence(entityText, entityType, text)

				if confidence > 0.5 { // Only include entities with reasonable confidence
					entity := Entity{
						Text:       entityText,
						Type:       entityType,
						Confidence: confidence,
						Positions:  []EntityPosition{},
						Metadata:   make(map[string]string),
					}

					// Add position information
					if i < len(matchIndices) {
						indices := matchIndices[i]
						if len(indices) >= 2 {
							entity.Positions = append(entity.Positions, EntityPosition{
								Start: indices[0],
								End:   indices[1],
							})
						}
					}

					// Add entity-specific metadata
					e.addEntityMetadata(&entity, text)

					entities = append(entities, entity)
				}
			}
		}
	}

	return entities
}

// calculateEntityConfidence calculates confidence for an entity
func (e *BasicEntityExtractor) calculateEntityConfidence(entityText string, entityType EntityType, fullText string) float64 {
	baseConfidence := 0.7 // Base confidence for pattern matches

	// Adjust confidence based on entity type and characteristics
	switch entityType {
	case EntityTypeEmail:
		if strings.Contains(entityText, "@") && strings.Contains(entityText, ".") {
			baseConfidence = 0.9
		}
	case EntityTypeURL:
		if strings.HasPrefix(entityText, "http") || strings.HasPrefix(entityText, "www") {
			baseConfidence = 0.9
		}
	case EntityTypePhone:
		// Check for phone number patterns
		digitCount := 0
		for _, r := range entityText {
			if r >= '0' && r <= '9' {
				digitCount++
			}
		}
		if digitCount >= 10 {
			baseConfidence = 0.8
		}
	case EntityTypeCreditCard:
		// Basic Luhn algorithm check
		if e.isValidCreditCard(entityText) {
			baseConfidence = 0.95
		}
	case EntityTypeSSN:
		// SSN pattern validation
		if len(strings.ReplaceAll(strings.ReplaceAll(entityText, "-", ""), " ", "")) == 9 {
			baseConfidence = 0.9
		}
	}

	// Check for contextual clues
	contextualPatterns, exists := e.contextualPatterns[entityType]
	if exists {
		lowerText := strings.ToLower(fullText)
		for _, pattern := range contextualPatterns {
			if strings.Contains(lowerText, pattern) {
				baseConfidence += 0.1
				break
			}
		}
	}

	// Clamp confidence to [0, 1]
	if baseConfidence > 1.0 {
		baseConfidence = 1.0
	}
	if baseConfidence < 0.0 {
		baseConfidence = 0.0
	}

	return baseConfidence
}

// addEntityMetadata adds metadata to entities
func (e *BasicEntityExtractor) addEntityMetadata(entity *Entity, fullText string) {
	switch entity.Type {
	case EntityTypePhone:
		entity.Metadata["formatted"] = e.formatPhoneNumber(entity.Text)
	case EntityTypeEmail:
		parts := strings.Split(entity.Text, "@")
		if len(parts) == 2 {
			entity.Metadata["username"] = parts[0]
			entity.Metadata["domain"] = parts[1]
		}
	case EntityTypeURL:
		entity.Metadata["protocol"] = e.extractProtocol(entity.Text)
		entity.Metadata["domain"] = e.extractDomain(entity.Text)
	case EntityTypeCreditCard:
		entity.Metadata["type"] = e.getCreditCardType(entity.Text)
		entity.Metadata["masked"] = e.maskCreditCard(entity.Text)
	case EntityTypeSSN:
		entity.Metadata["masked"] = e.maskSSN(entity.Text)
	}
}

// removeDuplicates removes duplicate and overlapping entities
func (e *BasicEntityExtractor) removeDuplicates(entities []Entity) []Entity {
	if len(entities) <= 1 {
		return entities
	}

	// Create a map to track seen entities
	seen := make(map[string]bool)
	result := []Entity{}

	for _, entity := range entities {
		key := entity.Text + ":" + string(entity.Type)
		if !seen[key] {
			seen[key] = true
			result = append(result, entity)
		}
	}

	return result
}

// isValidCreditCard performs basic credit card validation using Luhn algorithm
func (e *BasicEntityExtractor) isValidCreditCard(cardNumber string) bool {
	// Remove spaces and hyphens
	cardNumber = strings.ReplaceAll(strings.ReplaceAll(cardNumber, " ", ""), "-", "")

	// Check if all characters are digits
	for _, r := range cardNumber {
		if r < '0' || r > '9' {
			return false
		}
	}

	// Check length
	if len(cardNumber) < 13 || len(cardNumber) > 19 {
		return false
	}

	// Luhn algorithm
	sum := 0
	alternate := false

	for i := len(cardNumber) - 1; i >= 0; i-- {
		digit := int(cardNumber[i] - '0')

		if alternate {
			digit *= 2
			if digit > 9 {
				digit = (digit % 10) + 1
			}
		}

		sum += digit
		alternate = !alternate
	}

	return sum%10 == 0
}

// formatPhoneNumber formats a phone number
func (e *BasicEntityExtractor) formatPhoneNumber(phone string) string {
	// Remove all non-digit characters
	digits := ""
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			digits += string(r)
		}
	}

	// Format based on length
	switch len(digits) {
	case 10:
		return "(" + digits[0:3] + ") " + digits[3:6] + "-" + digits[6:10]
	case 11:
		return digits[0:1] + " (" + digits[1:4] + ") " + digits[4:7] + "-" + digits[7:11]
	default:
		return phone
	}
}

// extractProtocol extracts protocol from URL
func (e *BasicEntityExtractor) extractProtocol(url string) string {
	if strings.HasPrefix(url, "https://") {
		return "https"
	} else if strings.HasPrefix(url, "http://") {
		return "http"
	} else if strings.HasPrefix(url, "ftp://") {
		return "ftp"
	}
	return "unknown"
}

// extractDomain extracts domain from URL
func (e *BasicEntityExtractor) extractDomain(url string) string {
	// Remove protocol
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimPrefix(url, "ftp://")
	url = strings.TrimPrefix(url, "www.")

	// Extract domain part
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		return parts[0]
	}
	return url
}

// getCreditCardType determines credit card type
func (e *BasicEntityExtractor) getCreditCardType(cardNumber string) string {
	cardNumber = strings.ReplaceAll(strings.ReplaceAll(cardNumber, " ", ""), "-", "")

	if strings.HasPrefix(cardNumber, "4") {
		return "Visa"
	} else if strings.HasPrefix(cardNumber, "5") || strings.HasPrefix(cardNumber, "2") {
		return "MasterCard"
	} else if strings.HasPrefix(cardNumber, "3") {
		return "American Express"
	} else if strings.HasPrefix(cardNumber, "6") {
		return "Discover"
	}
	return "Unknown"
}

// maskCreditCard masks credit card number
func (e *BasicEntityExtractor) maskCreditCard(cardNumber string) string {
	cardNumber = strings.ReplaceAll(strings.ReplaceAll(cardNumber, " ", ""), "-", "")
	if len(cardNumber) >= 4 {
		return "****-****-****-" + cardNumber[len(cardNumber)-4:]
	}
	return "****"
}

// maskSSN masks SSN
func (e *BasicEntityExtractor) maskSSN(ssn string) string {
	ssn = strings.ReplaceAll(strings.ReplaceAll(ssn, "-", ""), " ", "")
	if len(ssn) >= 4 {
		return "***-**-" + ssn[len(ssn)-4:]
	}
	return "***-**-****"
}

// initializePatterns initializes entity extraction patterns
func (e *BasicEntityExtractor) initializePatterns() {
	// Email patterns
	e.patterns[EntityTypeEmail] = []*regexp.Regexp{
		regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`),
	}

	// URL patterns
	e.patterns[EntityTypeURL] = []*regexp.Regexp{
		regexp.MustCompile(`https?://[^\s<>"{}|\\^` + "`" + `\[\]]+`),
		regexp.MustCompile(`www\.[^\s<>"{}|\\^` + "`" + `\[\]]+`),
		regexp.MustCompile(`ftp://[^\s<>"{}|\\^` + "`" + `\[\]]+`),
	}

	// Phone patterns
	e.patterns[EntityTypePhone] = []*regexp.Regexp{
		regexp.MustCompile(`\b\d{3}[-.]?\d{3}[-.]?\d{4}\b`),
		regexp.MustCompile(`\b\(\d{3}\)\s?\d{3}[-.]?\d{4}\b`),
		regexp.MustCompile(`\b\+?1[-.]?\d{3}[-.]?\d{3}[-.]?\d{4}\b`),
	}

	// Credit card patterns
	e.patterns[EntityTypeCreditCard] = []*regexp.Regexp{
		regexp.MustCompile(`\b4\d{3}[-\s]?\d{4}[-\s]?\d{4}[-\s]?\d{4}\b`),      // Visa
		regexp.MustCompile(`\b5[1-5]\d{2}[-\s]?\d{4}[-\s]?\d{4}[-\s]?\d{4}\b`), // MasterCard
		regexp.MustCompile(`\b3[47]\d{2}[-\s]?\d{6}[-\s]?\d{5}\b`),             // American Express
		regexp.MustCompile(`\b6011[-\s]?\d{4}[-\s]?\d{4}[-\s]?\d{4}\b`),        // Discover
	}

	// SSN patterns
	e.patterns[EntityTypeSSN] = []*regexp.Regexp{
		regexp.MustCompile(`\b\d{3}[-\s]?\d{2}[-\s]?\d{4}\b`),
	}

	// IP Address patterns
	e.patterns[EntityTypeIPAddress] = []*regexp.Regexp{
		regexp.MustCompile(`\b(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\b`),
	}

	// Date patterns
	e.patterns[EntityTypeDate] = []*regexp.Regexp{
		regexp.MustCompile(`\b\d{1,2}\/\d{1,2}\/\d{2,4}\b`),
		regexp.MustCompile(`\b\d{1,2}-\d{1,2}-\d{2,4}\b`),
		regexp.MustCompile(`\b\d{4}-\d{1,2}-\d{1,2}\b`),
		regexp.MustCompile(`\b(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)[a-z]*\s+\d{1,2},?\s+\d{4}\b`),
	}

	// Money patterns
	e.patterns[EntityTypeMoney] = []*regexp.Regexp{
		regexp.MustCompile(`\$\d{1,3}(?:,\d{3})*(?:\.\d{2})?\b`),
		regexp.MustCompile(`\b\d{1,3}(?:,\d{3})*(?:\.\d{2})?\s*(?:USD|usd|dollars?)\b`),
		regexp.MustCompile(`\b(?:USD|usd|\$)\s*\d{1,3}(?:,\d{3})*(?:\.\d{2})?\b`),
	}

	// Percentage patterns
	e.patterns[EntityTypePercentage] = []*regexp.Regexp{
		regexp.MustCompile(`\b\d{1,3}(?:\.\d{1,2})?%\b`),
		regexp.MustCompile(`\b\d{1,3}(?:\.\d{1,2})?\s*percent\b`),
	}

	// Person patterns (basic - names starting with capital letters)
	e.patterns[EntityTypePerson] = []*regexp.Regexp{
		regexp.MustCompile(`\b[A-Z][a-z]+\s+[A-Z][a-z]+\b`),
		regexp.MustCompile(`\b(?:Mr|Mrs|Ms|Dr|Prof)\.?\s+[A-Z][a-z]+(?:\s+[A-Z][a-z]+)?\b`),
	}

	// Organization patterns
	e.patterns[EntityTypeOrganization] = []*regexp.Regexp{
		regexp.MustCompile(`\b[A-Z][a-z]*(?:\s+[A-Z][a-z]*)*\s+(?:Inc|Corp|LLC|Ltd|Co|Company|Corporation|Group|Enterprises?)\.?\b`),
		regexp.MustCompile(`\b[A-Z][a-z]*(?:\s+[A-Z][a-z]*)*\s+(?:University|College|Institute|School|Hospital|Bank|Insurance)\b`),
	}

	// Location patterns
	e.patterns[EntityTypeLocation] = []*regexp.Regexp{
		regexp.MustCompile(`\b[A-Z][a-z]+,\s*[A-Z]{2}\b`),               // City, State
		regexp.MustCompile(`\b[A-Z][a-z]+\s+[A-Z][a-z]+,\s*[A-Z]{2}\b`), // City Name, State
		regexp.MustCompile(`\b\d{5}(?:-\d{4})?\b`),                      // ZIP codes
	}

	// Initialize contextual patterns
	e.contextualPatterns[EntityTypePhone] = []string{
		"phone", "telephone", "mobile", "cell", "contact", "call", "number",
	}

	e.contextualPatterns[EntityTypeEmail] = []string{
		"email", "e-mail", "contact", "reach", "send", "mail",
	}

	e.contextualPatterns[EntityTypeCreditCard] = []string{
		"credit card", "payment", "card number", "billing", "charge",
	}

	e.contextualPatterns[EntityTypeSSN] = []string{
		"ssn", "social security", "social security number", "social",
	}

	e.contextualPatterns[EntityTypePerson] = []string{
		"mr", "mrs", "ms", "dr", "prof", "person", "individual", "name",
	}
}
