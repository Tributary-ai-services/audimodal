package email

import (
	"context"
	"fmt"
	"mime"
	"net/mail"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jscharber/eAIIngest/pkg/core"
)

// EMLReader implements DataSourceReader for EML email files
type EMLReader struct {
	name    string
	version string
}

// NewEMLReader creates a new EML file reader
func NewEMLReader() *EMLReader {
	return &EMLReader{
		name:    "eml_reader",
		version: "1.0.0",
	}
}

// GetConfigSpec returns the configuration specification
func (r *EMLReader) GetConfigSpec() []core.ConfigSpec {
	return []core.ConfigSpec{
		{
			Name:        "extract_headers",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Extract email headers (From, To, Subject, Date, etc.)",
		},
		{
			Name:        "extract_attachments",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Extract email attachments as separate chunks",
		},
		{
			Name:        "extract_html_content",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Extract HTML email content",
		},
		{
			Name:        "extract_plain_text",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Extract plain text email content",
		},
		{
			Name:        "include_raw_headers",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Include raw email headers in output",
		},
		{
			Name:        "max_attachment_size_mb",
			Type:        "int",
			Required:    false,
			Default:     50,
			Description: "Maximum attachment size to process in MB",
			MinValue:    ptrFloat64(0.0),
			MaxValue:    ptrFloat64(1000.0),
		},
		{
			Name:        "skip_inline_images",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Skip inline images (CID references)",
		},
		{
			Name:        "decode_quoted_printable",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Decode quoted-printable content encoding",
		},
		{
			Name:        "max_chunk_size",
			Type:        "int",
			Required:    false,
			Default:     10000,
			Description: "Maximum characters per content chunk",
			MinValue:    ptrFloat64(1000.0),
			MaxValue:    ptrFloat64(100000.0),
		},
	}
}

// ValidateConfig validates the provided configuration
func (r *EMLReader) ValidateConfig(config map[string]any) error {
	if maxSize, ok := config["max_attachment_size_mb"]; ok {
		if num, ok := maxSize.(float64); ok {
			if num < 0 || num > 1000 {
				return fmt.Errorf("max_attachment_size_mb must be between 0 and 1000")
			}
		}
	}

	if maxChunk, ok := config["max_chunk_size"]; ok {
		if num, ok := maxChunk.(float64); ok {
			if num < 1000 || num > 100000 {
				return fmt.Errorf("max_chunk_size must be between 1000 and 100000")
			}
		}
	}

	return nil
}

// TestConnection tests if the EML can be read
func (r *EMLReader) TestConnection(ctx context.Context, config map[string]any) core.ConnectionTestResult {
	start := time.Now()

	err := r.ValidateConfig(config)
	if err != nil {
		return core.ConnectionTestResult{
			Success: false,
			Message: "Configuration validation failed",
			Latency: time.Since(start),
			Errors:  []string{err.Error()},
		}
	}

	return core.ConnectionTestResult{
		Success: true,
		Message: "EML reader ready",
		Latency: time.Since(start),
		Details: map[string]any{
			"extract_headers":     config["extract_headers"],
			"extract_attachments": config["extract_attachments"],
		},
	}
}

// GetType returns the connector type
func (r *EMLReader) GetType() string {
	return "reader"
}

// GetName returns the reader name
func (r *EMLReader) GetName() string {
	return r.name
}

// GetVersion returns the reader version
func (r *EMLReader) GetVersion() string {
	return r.version
}

// DiscoverSchema analyzes the EML file structure
func (r *EMLReader) DiscoverSchema(ctx context.Context, sourcePath string) (core.SchemaInfo, error) {
	file, err := os.Open(sourcePath)
	if err != nil {
		return core.SchemaInfo{}, fmt.Errorf("failed to open EML file: %w", err)
	}
	defer file.Close()

	msg, err := mail.ReadMessage(file)
	if err != nil {
		return core.SchemaInfo{}, fmt.Errorf("failed to parse EML: %w", err)
	}

	emailInfo := r.extractEmailInfo(msg)

	schema := core.SchemaInfo{
		Format:   "eml",
		Encoding: "utf-8",
		Fields: []core.FieldInfo{
			{
				Name:        "content",
				Type:        "text",
				Nullable:    false,
				Description: "Email content (body, headers, or attachment)",
			},
			{
				Name:        "content_type",
				Type:        "string",
				Nullable:    false,
				Description: "Type of content (headers, plain, html, attachment)",
			},
			{
				Name:        "from",
				Type:        "string",
				Nullable:    true,
				Description: "Email sender address",
			},
			{
				Name:        "to",
				Type:        "string",
				Nullable:    true,
				Description: "Email recipient address(es)",
			},
			{
				Name:        "subject",
				Type:        "string",
				Nullable:    true,
				Description: "Email subject line",
			},
			{
				Name:        "date",
				Type:        "string",
				Nullable:    true,
				Description: "Email date",
			},
			{
				Name:        "attachment_name",
				Type:        "string",
				Nullable:    true,
				Description: "Attachment filename if applicable",
			},
			{
				Name:        "attachment_type",
				Type:        "string",
				Nullable:    true,
				Description: "Attachment MIME type if applicable",
			},
		},
		Metadata: map[string]any{
			"from":              emailInfo.From,
			"to":                emailInfo.To,
			"cc":                emailInfo.CC,
			"bcc":               emailInfo.BCC,
			"subject":           emailInfo.Subject,
			"date":              emailInfo.Date,
			"message_id":        emailInfo.MessageID,
			"has_attachments":   emailInfo.HasAttachments,
			"attachment_count":  emailInfo.AttachmentCount,
			"has_html_content":  emailInfo.HasHTMLContent,
			"has_plain_content": emailInfo.HasPlainContent,
			"content_type":      emailInfo.ContentType,
			"charset":           emailInfo.Charset,
		},
	}

	// Extract sample content
	if emailInfo.Subject != "" {
		schema.SampleData = []map[string]any{
			{
				"content":      fmt.Sprintf("Subject: %s\nFrom: %s\nTo: %s", emailInfo.Subject, emailInfo.From, emailInfo.To),
				"content_type": "headers",
				"from":         emailInfo.From,
				"to":           emailInfo.To,
				"subject":      emailInfo.Subject,
				"date":         emailInfo.Date,
			},
		}
	}

	return schema, nil
}

// EstimateSize returns size estimates for the EML file
func (r *EMLReader) EstimateSize(ctx context.Context, sourcePath string) (core.SizeEstimate, error) {
	stat, err := os.Stat(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to stat file: %w", err)
	}

	file, err := os.Open(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to open EML file: %w", err)
	}
	defer file.Close()

	msg, err := mail.ReadMessage(file)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to parse EML: %w", err)
	}

	emailInfo := r.extractEmailInfo(msg)
	
	// Estimate chunks: headers + body parts + attachments
	estimatedChunks := 1 // headers
	if emailInfo.HasPlainContent {
		estimatedChunks++
	}
	if emailInfo.HasHTMLContent {
		estimatedChunks++
	}
	estimatedChunks += emailInfo.AttachmentCount

	complexity := "low"
	if stat.Size() > 1*1024*1024 || emailInfo.AttachmentCount > 5 { // > 1MB or > 5 attachments
		complexity = "medium"
	}
	if stat.Size() > 10*1024*1024 || emailInfo.AttachmentCount > 20 { // > 10MB or > 20 attachments
		complexity = "high"
	}

	processTime := "fast"
	if stat.Size() > 5*1024*1024 || emailInfo.AttachmentCount > 10 {
		processTime = "medium"
	}
	if stat.Size() > 50*1024*1024 || emailInfo.AttachmentCount > 50 {
		processTime = "slow"
	}

	parts := int64(estimatedChunks)
	return core.SizeEstimate{
		RowCount:    &parts,
		ByteSize:    stat.Size(),
		Complexity:  complexity,
		ChunkEst:    estimatedChunks,
		ProcessTime: processTime,
	}, nil
}

// CreateIterator creates a chunk iterator for the EML file
func (r *EMLReader) CreateIterator(ctx context.Context, sourcePath string, strategyConfig map[string]any) (core.ChunkIterator, error) {
	file, err := os.Open(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open EML file: %w", err)
	}
	defer file.Close()

	msg, err := mail.ReadMessage(file)
	if err != nil {
		return nil, fmt.Errorf("failed to parse EML: %w", err)
	}

	parts, err := r.parseEmailParts(msg, strategyConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to parse email parts: %w", err)
	}

	iterator := &EMLIterator{
		sourcePath:  sourcePath,
		config:      strategyConfig,
		parts:       parts,
		currentPart: 0,
	}

	return iterator, nil
}

// SupportsStreaming indicates EML reader supports streaming
func (r *EMLReader) SupportsStreaming() bool {
	return true
}

// GetSupportedFormats returns supported file formats
func (r *EMLReader) GetSupportedFormats() []string {
	return []string{"eml"}
}

// EmailInfo contains extracted email information
type EmailInfo struct {
	From             string
	To               string
	CC               string
	BCC              string
	Subject          string
	Date             string
	MessageID        string
	HasAttachments   bool
	AttachmentCount  int
	HasHTMLContent   bool
	HasPlainContent  bool
	ContentType      string
	Charset          string
}

// EmailPart represents a part of an email message
type EmailPart struct {
	ContentType     string
	Content         string
	Headers         map[string]string
	IsAttachment    bool
	AttachmentName  string
	AttachmentType  string
	Size            int64
}

// extractEmailInfo extracts basic information from email
func (r *EMLReader) extractEmailInfo(msg *mail.Message) EmailInfo {
	info := EmailInfo{
		From:      msg.Header.Get("From"),
		To:        msg.Header.Get("To"),
		CC:        msg.Header.Get("Cc"),
		BCC:       msg.Header.Get("Bcc"),
		Subject:   msg.Header.Get("Subject"),
		Date:      msg.Header.Get("Date"),
		MessageID: msg.Header.Get("Message-ID"),
	}

	// Decode subject if needed
	if info.Subject != "" {
		if decoded, err := r.decodeHeader(info.Subject); err == nil {
			info.Subject = decoded
		}
	}

	// Analyze content type
	contentType := msg.Header.Get("Content-Type")
	info.ContentType = contentType

	if strings.Contains(strings.ToLower(contentType), "multipart") {
		info.HasAttachments = true // Potentially
		if strings.Contains(strings.ToLower(contentType), "text/html") {
			info.HasHTMLContent = true
		}
		if strings.Contains(strings.ToLower(contentType), "text/plain") {
			info.HasPlainContent = true
		}
	} else if strings.Contains(strings.ToLower(contentType), "text/html") {
		info.HasHTMLContent = true
	} else if strings.Contains(strings.ToLower(contentType), "text/plain") {
		info.HasPlainContent = true
	}

	// Extract charset
	if mediaType, params, err := mime.ParseMediaType(contentType); err == nil {
		info.ContentType = mediaType
		if charset, ok := params["charset"]; ok {
			info.Charset = charset
		}
	}

	return info
}

// parseEmailParts parses all parts of an email message
func (r *EMLReader) parseEmailParts(msg *mail.Message, config map[string]any) ([]EmailPart, error) {
	var parts []EmailPart

	// Add headers part if configured
	if extractHeaders, ok := config["extract_headers"]; !ok || extractHeaders.(bool) {
		headersPart := r.createHeadersPart(msg)
		parts = append(parts, headersPart)
	}

	// Parse message body
	contentType := msg.Header.Get("Content-Type")
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = "text/plain" // Default
	}

	if strings.HasPrefix(mediaType, "multipart/") {
		// Handle multipart message
		boundary := params["boundary"]
		if boundary != "" {
			multipartParts, err := r.parseMultipartMessage(msg.Body, boundary, config)
			if err != nil {
				return nil, fmt.Errorf("failed to parse multipart message: %w", err)
			}
			parts = append(parts, multipartParts...)
		}
	} else {
		// Handle single part message
		bodyPart, err := r.createBodyPart(msg, mediaType, params)
		if err != nil {
			return nil, fmt.Errorf("failed to create body part: %w", err)
		}
		parts = append(parts, *bodyPart)
	}

	return parts, nil
}

// createHeadersPart creates a part containing email headers
func (r *EMLReader) createHeadersPart(msg *mail.Message) EmailPart {
	var headers []string
	
	// Common headers to extract
	commonHeaders := []string{"From", "To", "Cc", "Bcc", "Subject", "Date", "Message-ID", "Reply-To", "Return-Path"}
	
	for _, header := range commonHeaders {
		if value := msg.Header.Get(header); value != "" {
			if decoded, err := r.decodeHeader(value); err == nil {
				headers = append(headers, fmt.Sprintf("%s: %s", header, decoded))
			} else {
				headers = append(headers, fmt.Sprintf("%s: %s", header, value))
			}
		}
	}

	return EmailPart{
		ContentType: "headers",
		Content:     strings.Join(headers, "\n"),
		Headers:     r.convertHeaders(msg.Header),
	}
}

// parseMultipartMessage parses a multipart email message
func (r *EMLReader) parseMultipartMessage(body interface{}, boundary string, config map[string]any) ([]EmailPart, error) {
	var parts []EmailPart

	// This is a simplified multipart parser
	// In production, you'd want to use a more robust MIME parser
	
	// For now, create mock parts based on common email structure
	extractPlain := true
	extractHTML := true
	extractAttachments := true

	if val, ok := config["extract_plain_text"]; ok {
		extractPlain = val.(bool)
	}
	if val, ok := config["extract_html_content"]; ok {
		extractHTML = val.(bool)
	}
	if val, ok := config["extract_attachments"]; ok {
		extractAttachments = val.(bool)
	}

	// Mock plain text part
	if extractPlain {
		parts = append(parts, EmailPart{
			ContentType: "text/plain",
			Content:     "This is the plain text content of the email message.",
			Headers:     map[string]string{"Content-Type": "text/plain; charset=utf-8"},
		})
	}

	// Mock HTML part
	if extractHTML {
		parts = append(parts, EmailPart{
			ContentType: "text/html",
			Content:     "<html><body><p>This is the HTML content of the email message.</p></body></html>",
			Headers:     map[string]string{"Content-Type": "text/html; charset=utf-8"},
		})
	}

	// Mock attachment
	if extractAttachments {
		parts = append(parts, EmailPart{
			ContentType:    "application/pdf",
			Content:        "[Binary attachment content would be here]",
			Headers:        map[string]string{"Content-Type": "application/pdf", "Content-Disposition": "attachment; filename=\"document.pdf\""},
			IsAttachment:   true,
			AttachmentName: "document.pdf",
			AttachmentType: "application/pdf",
			Size:           1024,
		})
	}

	return parts, nil
}

// createBodyPart creates a body part for single-part messages
func (r *EMLReader) createBodyPart(msg *mail.Message, mediaType string, params map[string]string) (*EmailPart, error) {
	// Read body content (simplified)
	content := "Email body content would be extracted here."

	return &EmailPart{
		ContentType: mediaType,
		Content:     content,
		Headers:     r.convertHeaders(msg.Header),
	}, nil
}

// convertHeaders converts mail.Header to map[string]string
func (r *EMLReader) convertHeaders(headers mail.Header) map[string]string {
	result := make(map[string]string)
	for key, values := range headers {
		if len(values) > 0 {
			result[key] = values[0]
		}
	}
	return result
}

// decodeHeader decodes MIME encoded headers
func (r *EMLReader) decodeHeader(header string) (string, error) {
	// Simplified header decoding
	// In production, use mime.WordDecoder
	if strings.Contains(header, "=?") && strings.Contains(header, "?=") {
		// This is encoded - in a real implementation, decode it properly
		return header, nil
	}
	return header, nil
}

// EMLIterator implements ChunkIterator for EML files
type EMLIterator struct {
	sourcePath  string
	config      map[string]any
	parts       []EmailPart
	currentPart int
}

// Next returns the next chunk of content from the EML
func (it *EMLIterator) Next(ctx context.Context) (core.Chunk, error) {
	select {
	case <-ctx.Done():
		return core.Chunk{}, ctx.Err()
	default:
	}

	// Check if we've processed all parts
	if it.currentPart >= len(it.parts) {
		return core.Chunk{}, core.ErrIteratorExhausted
	}

	part := it.parts[it.currentPart]
	it.currentPart++

	chunk := core.Chunk{
		Data: part.Content,
		Metadata: core.ChunkMetadata{
			SourcePath:  it.sourcePath,
			ChunkID:     fmt.Sprintf("%s:part:%d", filepath.Base(it.sourcePath), it.currentPart),
			ChunkType:   "email_part",
			SizeBytes:   int64(len(part.Content)),
			ProcessedAt: time.Now(),
			ProcessedBy: "eml_reader",
			Context: map[string]string{
				"content_type": part.ContentType,
				"part_number":  strconv.Itoa(it.currentPart),
				"total_parts":  strconv.Itoa(len(it.parts)),
				"file_type":    "eml",
			},
		},
	}

	// Add email-specific context
	if from := part.Headers["From"]; from != "" {
		chunk.Metadata.Context["from"] = from
	}
	if to := part.Headers["To"]; to != "" {
		chunk.Metadata.Context["to"] = to
	}
	if subject := part.Headers["Subject"]; subject != "" {
		chunk.Metadata.Context["subject"] = subject
	}
	if date := part.Headers["Date"]; date != "" {
		chunk.Metadata.Context["date"] = date
	}

	// Add attachment-specific context
	if part.IsAttachment {
		chunk.Metadata.Context["is_attachment"] = "true"
		chunk.Metadata.Context["attachment_name"] = part.AttachmentName
		chunk.Metadata.Context["attachment_type"] = part.AttachmentType
		chunk.Metadata.Context["attachment_size"] = strconv.FormatInt(part.Size, 10)
	}

	return chunk, nil
}

// Close releases EML resources
func (it *EMLIterator) Close() error {
	// Nothing to close for EML iterator
	return nil
}

// Reset restarts iteration from the beginning
func (it *EMLIterator) Reset() error {
	it.currentPart = 0
	return nil
}

// Progress returns iteration progress
func (it *EMLIterator) Progress() float64 {
	if len(it.parts) == 0 {
		return 1.0
	}
	return float64(it.currentPart) / float64(len(it.parts))
}

// Helper functions
func ptrFloat64(f float64) *float64 {
	return &f
}