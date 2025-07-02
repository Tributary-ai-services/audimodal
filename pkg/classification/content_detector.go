package classification

import (
	"context"
	"mime"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// ContentTypeDetector provides advanced content type detection
type ContentTypeDetector struct {
	name                string
	version             string
	mimePatterns        map[string]ContentType
	extensionPatterns   map[string]ContentType
	contentPatterns     map[ContentType][]*regexp.Regexp
	magicBytes          map[string]ContentType
	confidenceWeights   map[string]float64
}

// ContentTypeResult represents content type detection result
type ContentTypeResult struct {
	ContentType    ContentType `json:"content_type"`
	DocumentType   DocumentType `json:"document_type"`
	Confidence     float64     `json:"confidence"`
	DetectionMethod string     `json:"detection_method"`
	MimeType       string      `json:"mime_type"`
	FileExtension  string      `json:"file_extension"`
	Characteristics map[string]interface{} `json:"characteristics"`
}

// NewContentTypeDetector creates a new content type detector
func NewContentTypeDetector() *ContentTypeDetector {
	detector := &ContentTypeDetector{
		name:              "advanced-content-detector",
		version:           "1.0.0",
		mimePatterns:      make(map[string]ContentType),
		extensionPatterns: make(map[string]ContentType),
		contentPatterns:   make(map[ContentType][]*regexp.Regexp),
		magicBytes:        make(map[string]ContentType),
		confidenceWeights: map[string]float64{
			"magic_bytes":    1.0,
			"mime_type":      0.9,
			"file_extension": 0.7,
			"content_pattern": 0.8,
			"structure":      0.6,
		},
	}
	
	detector.initializePatterns()
	return detector
}

// DetectContentType detects the content type of the given input
func (d *ContentTypeDetector) DetectContentType(ctx context.Context, input *ClassificationInput) (*ContentTypeResult, error) {
	result := &ContentTypeResult{
		ContentType:     ContentTypeUnknown,
		DocumentType:    DocumentTypeText,
		Confidence:      0.0,
		DetectionMethod: "multi-factor",
		Characteristics: make(map[string]interface{}),
	}
	
	// Multiple detection strategies with confidence scoring
	detectionResults := []detectionResult{}
	
	// 1. Magic bytes detection (highest confidence)
	if magicResult := d.detectByMagicBytes(input.Content); magicResult != nil {
		detectionResults = append(detectionResults, *magicResult)
	}
	
	// 2. MIME type detection
	if input.MimeType != "" {
		if mimeResult := d.detectByMimeType(input.MimeType); mimeResult != nil {
			detectionResults = append(detectionResults, *mimeResult)
		}
		result.MimeType = input.MimeType
	} else if input.Filename != "" {
		// Derive MIME type from filename
		if derivedMime := mime.TypeByExtension(filepath.Ext(input.Filename)); derivedMime != "" {
			result.MimeType = derivedMime
			if mimeResult := d.detectByMimeType(derivedMime); mimeResult != nil {
				detectionResults = append(detectionResults, *mimeResult)
			}
		}
	}
	
	// 3. File extension detection
	if input.Filename != "" {
		if extResult := d.detectByFileExtension(input.Filename); extResult != nil {
			detectionResults = append(detectionResults, *extResult)
		}
		result.FileExtension = strings.ToLower(filepath.Ext(input.Filename))
	}
	
	// 4. Content pattern detection
	if patternResult := d.detectByContentPatterns(input.Content); patternResult != nil {
		detectionResults = append(detectionResults, *patternResult)
	}
	
	// 5. Structure analysis
	if structResult := d.detectByStructure(input.Content); structResult != nil {
		detectionResults = append(detectionResults, *structResult)
	}
	
	// Combine results using weighted confidence
	if len(detectionResults) > 0 {
		finalResult := d.combineDetectionResults(detectionResults)
		result.ContentType = finalResult.contentType
		result.DocumentType = d.mapContentToDocumentType(finalResult.contentType, input)
		result.Confidence = finalResult.confidence
		result.DetectionMethod = finalResult.methods
	}
	
	// Add content characteristics
	d.analyzeContentCharacteristics(input.Content, result)
	
	return result, nil
}

// detectionResult represents a single detection result
type detectionResult struct {
	contentType ContentType
	confidence  float64
	method      string
	weight      float64
}

// combineDetectionResults combines multiple detection results
func (d *ContentTypeDetector) combineDetectionResults(results []detectionResult) struct {
	contentType ContentType
	confidence  float64
	methods     string
} {
	// Group results by content type
	typeScores := make(map[ContentType]float64)
	typeMethods := make(map[ContentType][]string)
	
	for _, result := range results {
		weightedScore := result.confidence * result.weight
		typeScores[result.contentType] += weightedScore
		typeMethods[result.contentType] = append(typeMethods[result.contentType], result.method)
	}
	
	// Find the highest scoring type
	var bestType ContentType
	var bestScore float64
	
	for contentType, score := range typeScores {
		if score > bestScore {
			bestScore = score
			bestType = contentType
		}
	}
	
	// Normalize confidence
	normalizedConfidence := bestScore / 4.0 // Max possible weighted score
	if normalizedConfidence > 1.0 {
		normalizedConfidence = 1.0
	}
	
	methods := strings.Join(typeMethods[bestType], "+")
	
	return struct {
		contentType ContentType
		confidence  float64
		methods     string
	}{
		contentType: bestType,
		confidence:  normalizedConfidence,
		methods:     methods,
	}
}

// detectByMagicBytes detects content type by magic bytes/file signatures
func (d *ContentTypeDetector) detectByMagicBytes(content string) *detectionResult {
	if len(content) < 8 {
		return nil
	}
	
	contentBytes := []byte(content)
	
	// Check common file signatures
	signatures := map[string]ContentType{
		"%PDF":     ContentTypeDocument,
		"PK\x03\x04": ContentTypeDocument, // ZIP-based formats (DOCX, XLSX, etc.)
		"\xFF\xD8\xFF": ContentTypeImage,   // JPEG
		"\x89PNG":   ContentTypeImage,      // PNG
		"GIF8":      ContentTypeImage,      // GIF
		"<html":     ContentTypeWeb,        // HTML
		"<?xml":     ContentTypeDocument,   // XML
		"{":         ContentTypeDocument,   // Likely JSON
	}
	
	for signature, contentType := range signatures {
		if len(contentBytes) >= len(signature) {
			if string(contentBytes[:len(signature)]) == signature ||
				strings.HasPrefix(strings.ToLower(content), strings.ToLower(signature)) {
				return &detectionResult{
					contentType: contentType,
					confidence:  0.95,
					method:      "magic_bytes",
					weight:      d.confidenceWeights["magic_bytes"],
				}
			}
		}
	}
	
	return nil
}

// detectByMimeType detects content type by MIME type
func (d *ContentTypeDetector) detectByMimeType(mimeType string) *detectionResult {
	if contentType, exists := d.mimePatterns[mimeType]; exists {
		return &detectionResult{
			contentType: contentType,
			confidence:  0.9,
			method:      "mime_type",
			weight:      d.confidenceWeights["mime_type"],
		}
	}
	
	// Check MIME type patterns
	switch {
	case strings.HasPrefix(mimeType, "text/"):
		return &detectionResult{
			contentType: ContentTypeDocument,
			confidence:  0.8,
			method:      "mime_type",
			weight:      d.confidenceWeights["mime_type"],
		}
	case strings.HasPrefix(mimeType, "image/"):
		return &detectionResult{
			contentType: ContentTypeImage,
			confidence:  0.9,
			method:      "mime_type",
			weight:      d.confidenceWeights["mime_type"],
		}
	case strings.HasPrefix(mimeType, "video/"):
		return &detectionResult{
			contentType: ContentTypeVideo,
			confidence:  0.9,
			method:      "mime_type",
			weight:      d.confidenceWeights["mime_type"],
		}
	case strings.HasPrefix(mimeType, "audio/"):
		return &detectionResult{
			contentType: ContentTypeAudio,
			confidence:  0.9,
			method:      "mime_type",
			weight:      d.confidenceWeights["mime_type"],
		}
	case strings.Contains(mimeType, "spreadsheet") || strings.Contains(mimeType, "excel"):
		return &detectionResult{
			contentType: ContentTypeSpreadsheet,
			confidence:  0.9,
			method:      "mime_type",
			weight:      d.confidenceWeights["mime_type"],
		}
	case strings.Contains(mimeType, "presentation") || strings.Contains(mimeType, "powerpoint"):
		return &detectionResult{
			contentType: ContentTypePresentation,
			confidence:  0.9,
			method:      "mime_type",
			weight:      d.confidenceWeights["mime_type"],
		}
	}
	
	return nil
}

// detectByFileExtension detects content type by file extension
func (d *ContentTypeDetector) detectByFileExtension(filename string) *detectionResult {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		return nil
	}
	
	ext = ext[1:] // Remove the dot
	
	if contentType, exists := d.extensionPatterns[ext]; exists {
		return &detectionResult{
			contentType: contentType,
			confidence:  0.8,
			method:      "file_extension",
			weight:      d.confidenceWeights["file_extension"],
		}
	}
	
	return nil
}

// detectByContentPatterns detects content type by analyzing content patterns
func (d *ContentTypeDetector) detectByContentPatterns(content string) *detectionResult {
	if len(content) == 0 {
		return nil
	}
	
	// Test each content type's patterns
	bestMatch := ContentTypeUnknown
	bestScore := 0.0
	
	for contentType, patterns := range d.contentPatterns {
		score := d.calculatePatternScore(content, patterns)
		if score > bestScore {
			bestScore = score
			bestMatch = contentType
		}
	}
	
	if bestScore > 0.3 {
		return &detectionResult{
			contentType: bestMatch,
			confidence:  bestScore,
			method:      "content_pattern",
			weight:      d.confidenceWeights["content_pattern"],
		}
	}
	
	return nil
}

// calculatePatternScore calculates pattern matching score
func (d *ContentTypeDetector) calculatePatternScore(content string, patterns []*regexp.Regexp) float64 {
	if len(patterns) == 0 {
		return 0.0
	}
	
	matches := 0
	for _, pattern := range patterns {
		if pattern.MatchString(content) {
			matches++
		}
	}
	
	return float64(matches) / float64(len(patterns))
}

// detectByStructure detects content type by analyzing document structure
func (d *ContentTypeDetector) detectByStructure(content string) *detectionResult {
	if len(content) < 50 {
		return nil
	}
	
	// Analyze structural characteristics
	characteristics := d.analyzeStructure(content)
	
	// Score different content types based on structure
	scores := map[ContentType]float64{}
	
	// JSON structure
	if characteristics["json_like"] > 0.8 {
		scores[ContentTypeDocument] = 0.9
	}
	
	// XML/HTML structure
	if characteristics["markup_like"] > 0.7 {
		if characteristics["html_tags"] > 0.5 {
			scores[ContentTypeWeb] = 0.8
		} else {
			scores[ContentTypeDocument] = 0.7
		}
	}
	
	// Code structure
	if characteristics["code_like"] > 0.6 {
		scores[ContentTypeCode] = 0.8
	}
	
	// Email structure
	if characteristics["email_like"] > 0.7 {
		scores[ContentTypeEmail] = 0.9
	}
	
	// Log structure
	if characteristics["log_like"] > 0.6 {
		scores[ContentTypeLog] = 0.8
	}
	
	// Find best match
	bestType := ContentTypeUnknown
	bestScore := 0.0
	
	for contentType, score := range scores {
		if score > bestScore {
			bestScore = score
			bestType = contentType
		}
	}
	
	if bestScore > 0.5 {
		return &detectionResult{
			contentType: bestType,
			confidence:  bestScore,
			method:      "structure",
			weight:      d.confidenceWeights["structure"],
		}
	}
	
	return nil
}

// analyzeStructure analyzes document structure
func (d *ContentTypeDetector) analyzeStructure(content string) map[string]float64 {
	characteristics := make(map[string]float64)
	
	// JSON-like characteristics
	jsonIndicators := 0
	if strings.Contains(content, "{") && strings.Contains(content, "}") {
		jsonIndicators++
	}
	if strings.Contains(content, "[") && strings.Contains(content, "]") {
		jsonIndicators++
	}
	if strings.Contains(content, ":") && strings.Contains(content, ",") {
		jsonIndicators++
	}
	characteristics["json_like"] = float64(jsonIndicators) / 3.0
	
	// Markup-like characteristics
	markupIndicators := 0
	if strings.Contains(content, "<") && strings.Contains(content, ">") {
		markupIndicators++
	}
	if regexp.MustCompile(`<[^>]+>`).MatchString(content) {
		markupIndicators++
	}
	characteristics["markup_like"] = float64(markupIndicators) / 2.0
	
	// HTML-specific tags
	htmlTags := []string{"html", "head", "body", "div", "span", "p", "a", "img"}
	htmlCount := 0
	lowerContent := strings.ToLower(content)
	for _, tag := range htmlTags {
		if strings.Contains(lowerContent, "<"+tag) {
			htmlCount++
		}
	}
	characteristics["html_tags"] = float64(htmlCount) / float64(len(htmlTags))
	
	// Code-like characteristics
	codeIndicators := 0
	codePatterns := []string{
		"function", "class", "import", "include", "var", "const", "let",
		"def", "if", "else", "for", "while", "return", "public", "private",
	}
	for _, pattern := range codePatterns {
		if strings.Contains(lowerContent, pattern) {
			codeIndicators++
		}
	}
	characteristics["code_like"] = float64(codeIndicators) / float64(len(codePatterns))
	
	// Email-like characteristics
	emailIndicators := 0
	emailPatterns := []string{"from:", "to:", "subject:", "date:", "reply-to:"}
	for _, pattern := range emailPatterns {
		if strings.Contains(lowerContent, pattern) {
			emailIndicators++
		}
	}
	characteristics["email_like"] = float64(emailIndicators) / float64(len(emailPatterns))
	
	// Log-like characteristics
	logIndicators := 0
	logPatterns := []string{"[info]", "[error]", "[warn]", "[debug]", "timestamp", "log", "trace"}
	for _, pattern := range logPatterns {
		if strings.Contains(lowerContent, pattern) {
			logIndicators++
		}
	}
	characteristics["log_like"] = float64(logIndicators) / float64(len(logPatterns))
	
	return characteristics
}

// mapContentToDocumentType maps content type to document type
func (d *ContentTypeDetector) mapContentToDocumentType(contentType ContentType, input *ClassificationInput) DocumentType {
	switch contentType {
	case ContentTypeDocument:
		// Determine specific document type
		if input.MimeType != "" {
			switch input.MimeType {
			case "application/pdf":
				return DocumentTypePDF
			case "application/msword", "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
				return DocumentTypeWord
			case "text/html":
				return DocumentTypeHTML
			case "application/xml", "text/xml":
				return DocumentTypeXML
			case "application/json":
				return DocumentTypeJSON
			case "text/csv":
				return DocumentTypeCSV
			case "text/markdown":
				return DocumentTypeMarkdown
			case "application/rtf":
				return DocumentTypeRTF
			}
		}
		
		// Check by file extension
		if input.Filename != "" {
			ext := strings.ToLower(filepath.Ext(input.Filename))
			switch ext {
			case ".pdf":
				return DocumentTypePDF
			case ".doc", ".docx":
				return DocumentTypeWord
			case ".html", ".htm":
				return DocumentTypeHTML
			case ".xml":
				return DocumentTypeXML
			case ".json":
				return DocumentTypeJSON
			case ".csv":
				return DocumentTypeCSV
			case ".md", ".markdown":
				return DocumentTypeMarkdown
			case ".rtf":
				return DocumentTypeRTF
			}
		}
		
		return DocumentTypeText
	default:
		return DocumentTypeText
	}
}

// analyzeContentCharacteristics analyzes additional content characteristics
func (d *ContentTypeDetector) analyzeContentCharacteristics(content string, result *ContentTypeResult) {
	if len(content) == 0 {
		return
	}
	
	// Basic statistics
	result.Characteristics["length"] = len(content)
	result.Characteristics["lines"] = len(strings.Split(content, "\n"))
	result.Characteristics["words"] = len(strings.Fields(content))
	
	// Character analysis
	letterCount := 0
	digitCount := 0
	punctuationCount := 0
	whitespaceCount := 0
	
	for _, r := range content {
		if unicode.IsLetter(r) {
			letterCount++
		} else if unicode.IsDigit(r) {
			digitCount++
		} else if unicode.IsPunct(r) {
			punctuationCount++
		} else if unicode.IsSpace(r) {
			whitespaceCount++
		}
	}
	
	totalChars := len(content)
	if totalChars > 0 {
		result.Characteristics["letter_ratio"] = float64(letterCount) / float64(totalChars)
		result.Characteristics["digit_ratio"] = float64(digitCount) / float64(totalChars)
		result.Characteristics["punctuation_ratio"] = float64(punctuationCount) / float64(totalChars)
		result.Characteristics["whitespace_ratio"] = float64(whitespaceCount) / float64(totalChars)
	}
	
	// Language indicators
	result.Characteristics["has_uppercase"] = strings.ToLower(content) != content
	result.Characteristics["has_numbers"] = regexp.MustCompile(`\d`).MatchString(content)
	result.Characteristics["has_special_chars"] = regexp.MustCompile(`[^a-zA-Z0-9\s]`).MatchString(content)
	
	// Structure indicators based on content type
	switch result.ContentType {
	case ContentTypeDocument:
		result.Characteristics["avg_line_length"] = float64(len(content)) / float64(len(strings.Split(content, "\n")))
		result.Characteristics["paragraph_count"] = len(regexp.MustCompile(`\n\s*\n`).Split(content, -1))
	case ContentTypeCode:
		result.Characteristics["indentation_present"] = regexp.MustCompile(`^\s+`).MatchString(content)
		result.Characteristics["bracket_count"] = strings.Count(content, "{") + strings.Count(content, "[") + strings.Count(content, "(")
	case ContentTypeWeb:
		result.Characteristics["tag_count"] = len(regexp.MustCompile(`<[^>]+>`).FindAllString(content, -1))
		result.Characteristics["link_count"] = strings.Count(strings.ToLower(content), "<a ")
	}
}

// initializePatterns initializes detection patterns
func (d *ContentTypeDetector) initializePatterns() {
	// MIME type mappings
	d.mimePatterns = map[string]ContentType{
		"text/plain":       ContentTypeDocument,
		"text/html":        ContentTypeWeb,
		"text/xml":         ContentTypeDocument,
		"text/csv":         ContentTypeSpreadsheet,
		"text/markdown":    ContentTypeDocument,
		"application/pdf":  ContentTypeDocument,
		"application/json": ContentTypeDocument,
		"application/xml":  ContentTypeDocument,
		"application/msword": ContentTypeDocument,
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document": ContentTypeDocument,
		"application/vnd.ms-excel": ContentTypeSpreadsheet,
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": ContentTypeSpreadsheet,
		"application/vnd.ms-powerpoint": ContentTypePresentation,
		"application/vnd.openxmlformats-officedocument.presentationml.presentation": ContentTypePresentation,
		"image/jpeg":     ContentTypeImage,
		"image/png":      ContentTypeImage,
		"image/gif":      ContentTypeImage,
		"image/svg+xml":  ContentTypeImage,
		"video/mp4":      ContentTypeVideo,
		"video/avi":      ContentTypeVideo,
		"audio/mp3":      ContentTypeAudio,
		"audio/wav":      ContentTypeAudio,
	}
	
	// File extension mappings
	d.extensionPatterns = map[string]ContentType{
		"txt":  ContentTypeDocument,
		"md":   ContentTypeDocument,
		"html": ContentTypeWeb,
		"htm":  ContentTypeWeb,
		"xml":  ContentTypeDocument,
		"json": ContentTypeDocument,
		"csv":  ContentTypeSpreadsheet,
		"pdf":  ContentTypeDocument,
		"doc":  ContentTypeDocument,
		"docx": ContentTypeDocument,
		"xls":  ContentTypeSpreadsheet,
		"xlsx": ContentTypeSpreadsheet,
		"ppt":  ContentTypePresentation,
		"pptx": ContentTypePresentation,
		"jpg":  ContentTypeImage,
		"jpeg": ContentTypeImage,
		"png":  ContentTypeImage,
		"gif":  ContentTypeImage,
		"svg":  ContentTypeImage,
		"mp4":  ContentTypeVideo,
		"avi":  ContentTypeVideo,
		"mov":  ContentTypeVideo,
		"mp3":  ContentTypeAudio,
		"wav":  ContentTypeAudio,
		"flac": ContentTypeAudio,
		"js":   ContentTypeCode,
		"py":   ContentTypeCode,
		"java": ContentTypeCode,
		"cpp":  ContentTypeCode,
		"c":    ContentTypeCode,
		"go":   ContentTypeCode,
		"rb":   ContentTypeCode,
		"php":  ContentTypeCode,
		"log":  ContentTypeLog,
		"sql":  ContentTypeDatabase,
		"db":   ContentTypeDatabase,
	}
	
	// Content pattern regexes
	d.contentPatterns = map[ContentType][]*regexp.Regexp{
		ContentTypeWeb: {
			regexp.MustCompile(`(?i)<html[^>]*>`),
			regexp.MustCompile(`(?i)<head[^>]*>`),
			regexp.MustCompile(`(?i)<body[^>]*>`),
			regexp.MustCompile(`(?i)<!DOCTYPE\s+html>`),
		},
		ContentTypeCode: {
			regexp.MustCompile(`(?i)(function|class|import|include)\s+\w+`),
			regexp.MustCompile(`(?i)(public|private|protected)\s+(static\s+)?(\w+\s+)+\w+`),
			regexp.MustCompile(`(?i)(if|while|for)\s*\(`),
			regexp.MustCompile(`(?i)(var|let|const|def)\s+\w+`),
		},
		ContentTypeEmail: {
			regexp.MustCompile(`(?i)^From:\s*.+`),
			regexp.MustCompile(`(?i)^To:\s*.+`),
			regexp.MustCompile(`(?i)^Subject:\s*.+`),
			regexp.MustCompile(`(?i)^Date:\s*.+`),
		},
		ContentTypeLog: {
			regexp.MustCompile(`(?i)\[(INFO|ERROR|WARN|DEBUG|TRACE)\]`),
			regexp.MustCompile(`\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}`),
			regexp.MustCompile(`(?i)(timestamp|logged|exception|stack trace)`),
		},
		ContentTypeDatabase: {
			regexp.MustCompile(`(?i)(SELECT|INSERT|UPDATE|DELETE)\s+`),
			regexp.MustCompile(`(?i)(CREATE|ALTER|DROP)\s+(TABLE|INDEX|DATABASE)`),
			regexp.MustCompile(`(?i)(PRIMARY KEY|FOREIGN KEY|CONSTRAINT)`),
		},
	}
}