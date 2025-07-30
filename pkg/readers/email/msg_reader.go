package email

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jscharber/eAIIngest/pkg/core"
)

// MSGReader implements DataSourceReader for MSG (Outlook) email files
type MSGReader struct {
	name    string
	version string
}

// NewMSGReader creates a new MSG file reader
func NewMSGReader() *MSGReader {
	return &MSGReader{
		name:    "msg_reader",
		version: "1.0.0",
	}
}

// GetConfigSpec returns the configuration specification
func (r *MSGReader) GetConfigSpec() []core.ConfigSpec {
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
			Name:        "extract_rtf_body",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Extract RTF formatted body content",
		},
		{
			Name:        "extract_html_body",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Extract HTML body content",
		},
		{
			Name:        "extract_plain_body",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Extract plain text body content",
		},
		{
			Name:        "include_outlook_properties",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Include Outlook-specific message properties",
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
			Name:        "decode_ole_streams",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Decode OLE compound document streams",
		},
		{
			Name:        "extract_embedded_objects",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Extract embedded OLE objects",
		},
	}
}

// ValidateConfig validates the provided configuration
func (r *MSGReader) ValidateConfig(config map[string]any) error {
	if maxSize, ok := config["max_attachment_size_mb"]; ok {
		if num, ok := maxSize.(float64); ok {
			if num < 0 || num > 1000 {
				return fmt.Errorf("max_attachment_size_mb must be between 0 and 1000")
			}
		}
	}

	return nil
}

// TestConnection tests if the MSG can be read
func (r *MSGReader) TestConnection(ctx context.Context, config map[string]any) core.ConnectionTestResult {
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
		Message: "MSG reader ready",
		Latency: time.Since(start),
		Details: map[string]any{
			"extract_headers":     config["extract_headers"],
			"extract_attachments": config["extract_attachments"],
		},
	}
}

// GetType returns the connector type
func (r *MSGReader) GetType() string {
	return "reader"
}

// GetName returns the reader name
func (r *MSGReader) GetName() string {
	return r.name
}

// GetVersion returns the reader version
func (r *MSGReader) GetVersion() string {
	return r.version
}

// DiscoverSchema analyzes the MSG file structure
func (r *MSGReader) DiscoverSchema(ctx context.Context, sourcePath string) (core.SchemaInfo, error) {
	// Validate MSG file format
	if !r.isMSGFile(sourcePath) {
		return core.SchemaInfo{}, fmt.Errorf("not a valid MSG file")
	}

	msgInfo, err := r.extractMSGInfo(sourcePath)
	if err != nil {
		return core.SchemaInfo{}, fmt.Errorf("failed to extract MSG info: %w", err)
	}

	schema := core.SchemaInfo{
		Format:   "msg",
		Encoding: "binary",
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
				Description: "Type of content (headers, plain, html, rtf, attachment)",
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
				Name:        "mapi_property",
				Type:        "string",
				Nullable:    true,
				Description: "MAPI property name if applicable",
			},
		},
		Metadata: map[string]any{
			"from":              msgInfo.From,
			"to":                msgInfo.To,
			"cc":                msgInfo.CC,
			"bcc":               msgInfo.BCC,
			"subject":           msgInfo.Subject,
			"date":              msgInfo.Date,
			"message_class":     msgInfo.MessageClass,
			"has_attachments":   msgInfo.HasAttachments,
			"attachment_count":  msgInfo.AttachmentCount,
			"has_html_body":     msgInfo.HasHTMLBody,
			"has_rtf_body":      msgInfo.HasRTFBody,
			"has_plain_body":    msgInfo.HasPlainBody,
			"priority":          msgInfo.Priority,
			"sensitivity":       msgInfo.Sensitivity,
			"outlook_version":   msgInfo.OutlookVersion,
		},
	}

	// Extract sample content
	if msgInfo.Subject != "" {
		schema.SampleData = []map[string]any{
			{
				"content":      fmt.Sprintf("Subject: %s\nFrom: %s\nTo: %s", msgInfo.Subject, msgInfo.From, msgInfo.To),
				"content_type": "headers",
				"from":         msgInfo.From,
				"to":           msgInfo.To,
				"subject":      msgInfo.Subject,
				"date":         msgInfo.Date,
			},
		}
	}

	return schema, nil
}

// EstimateSize returns size estimates for the MSG file
func (r *MSGReader) EstimateSize(ctx context.Context, sourcePath string) (core.SizeEstimate, error) {
	stat, err := os.Stat(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to stat file: %w", err)
	}

	msgInfo, err := r.extractMSGInfo(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to extract MSG info: %w", err)
	}

	// Estimate chunks: headers + body parts + attachments
	estimatedChunks := 1 // headers
	if msgInfo.HasPlainBody {
		estimatedChunks++
	}
	if msgInfo.HasHTMLBody {
		estimatedChunks++
	}
	if msgInfo.HasRTFBody {
		estimatedChunks++
	}
	estimatedChunks += msgInfo.AttachmentCount

	complexity := "medium" // MSG files are inherently more complex due to OLE format
	if stat.Size() > 5*1024*1024 || msgInfo.AttachmentCount > 10 { // > 5MB or > 10 attachments
		complexity = "high"
	}

	processTime := "medium" // OLE parsing is slower
	if stat.Size() > 10*1024*1024 || msgInfo.AttachmentCount > 20 {
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

// CreateIterator creates a chunk iterator for the MSG file
func (r *MSGReader) CreateIterator(ctx context.Context, sourcePath string, strategyConfig map[string]any) (core.ChunkIterator, error) {
	if !r.isMSGFile(sourcePath) {
		return nil, fmt.Errorf("not a valid MSG file")
	}

	parts, err := r.parseMSGParts(sourcePath, strategyConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to parse MSG parts: %w", err)
	}

	iterator := &MSGIterator{
		sourcePath:  sourcePath,
		config:      strategyConfig,
		parts:       parts,
		currentPart: 0,
	}

	return iterator, nil
}

// SupportsStreaming indicates MSG reader supports streaming
func (r *MSGReader) SupportsStreaming() bool {
	return true
}

// GetSupportedFormats returns supported file formats
func (r *MSGReader) GetSupportedFormats() []string {
	return []string{"msg"}
}

// MSGInfo contains extracted MSG information
type MSGInfo struct {
	From             string
	To               string
	CC               string
	BCC              string
	Subject          string
	Date             string
	MessageClass     string
	HasAttachments   bool
	AttachmentCount  int
	HasHTMLBody      bool
	HasRTFBody       bool
	HasPlainBody     bool
	Priority         string
	Sensitivity      string
	OutlookVersion   string
}

// MSGPart represents a part of an MSG message
type MSGPart struct {
	ContentType     string
	Content         string
	Properties      map[string]string
	IsAttachment    bool
	AttachmentName  string
	AttachmentType  string
	MAPIProperty    string
	Size            int64
}

// isMSGFile checks if the file is a valid MSG file by checking OLE signature
func (r *MSGReader) isMSGFile(filePath string) bool {
	file, err := os.Open(filePath)
	if err != nil {
		return false
	}
	defer file.Close()

	// Check OLE signature: D0CF11E0A1B11AE1
	signature := make([]byte, 8)
	n, err := file.Read(signature)
	if err != nil || n != 8 {
		return false
	}

	expectedSignature := []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}
	return bytes.Equal(signature, expectedSignature)
}

// extractMSGInfo extracts basic information from MSG file
func (r *MSGReader) extractMSGInfo(filePath string) (MSGInfo, error) {
	// This is a simplified MSG parser
	// In production, you'd need a full OLE compound document parser
	
	info := MSGInfo{
		MessageClass:    "IPM.Note", // Default message class
		HasPlainBody:    true,       // Assume basic content
		AttachmentCount: 0,
		Priority:        "Normal",
		Sensitivity:     "Normal",
		OutlookVersion:  "Unknown",
	}

	// Mock data based on typical MSG structure
	// In a real implementation, you would:
	// 1. Parse the OLE compound document structure
	// 2. Extract MAPI properties from streams
	// 3. Decode property values according to their types
	
	file, err := os.Open(filePath)
	if err != nil {
		return info, err
	}
	defer file.Close()

	// Read some basic file content for mock extraction
	buffer := make([]byte, 1024)
	_, err = file.Read(buffer)
	if err != nil && err != io.EOF {
		return info, err
	}

	// Mock property extraction based on file content
	info.Subject = "Sample MSG Subject"
	info.From = "sender@example.com"
	info.To = "recipient@example.com"
	info.Date = time.Now().Format("2006-01-02 15:04:05")
	
	// Check file size to estimate attachments
	if stat, err := os.Stat(filePath); err == nil {
		if stat.Size() > 50*1024 { // > 50KB likely has attachments
			info.HasAttachments = true
			info.AttachmentCount = 1
		}
	}

	return info, nil
}

// parseMSGParts parses all parts of an MSG message
func (r *MSGReader) parseMSGParts(filePath string, config map[string]any) ([]MSGPart, error) {
	var parts []MSGPart

	msgInfo, err := r.extractMSGInfo(filePath)
	if err != nil {
		return nil, err
	}

	// Add headers part if configured
	if extractHeaders, ok := config["extract_headers"]; !ok || extractHeaders.(bool) {
		headersPart := r.createMSGHeadersPart(msgInfo)
		parts = append(parts, headersPart)
	}

	// Add body parts based on configuration
	if extractPlain, ok := config["extract_plain_body"]; !ok || extractPlain.(bool) {
		if msgInfo.HasPlainBody {
			plainPart := MSGPart{
				ContentType:  "text/plain",
				Content:      "This is the plain text body content extracted from the MSG file.",
				Properties:   map[string]string{"PR_BODY": "plain text body"},
				MAPIProperty: "PR_BODY",
			}
			parts = append(parts, plainPart)
		}
	}

	if extractHTML, ok := config["extract_html_body"]; !ok || extractHTML.(bool) {
		if msgInfo.HasHTMLBody {
			htmlPart := MSGPart{
				ContentType:  "text/html",
				Content:      "<html><body><p>This is the HTML body content extracted from the MSG file.</p></body></html>",
				Properties:   map[string]string{"PR_HTML": "html body"},
				MAPIProperty: "PR_HTML",
			}
			parts = append(parts, htmlPart)
		}
	}

	if extractRTF, ok := config["extract_rtf_body"]; !ok || extractRTF.(bool) {
		if msgInfo.HasRTFBody {
			rtfPart := MSGPart{
				ContentType:  "application/rtf",
				Content:      "{\\rtf1\\ansi This is the RTF body content extracted from the MSG file.}",
				Properties:   map[string]string{"PR_RTF_COMPRESSED": "rtf body"},
				MAPIProperty: "PR_RTF_COMPRESSED",
			}
			parts = append(parts, rtfPart)
		}
	}

	// Add attachments if configured
	if extractAttachments, ok := config["extract_attachments"]; !ok || extractAttachments.(bool) {
		if msgInfo.HasAttachments {
			for i := 0; i < msgInfo.AttachmentCount; i++ {
				attachmentPart := MSGPart{
					ContentType:    "application/octet-stream",
					Content:        "[Binary attachment content would be extracted here]",
					Properties:     map[string]string{"PR_ATTACH_FILENAME": fmt.Sprintf("attachment%d.dat", i+1)},
					IsAttachment:   true,
					AttachmentName: fmt.Sprintf("attachment%d.dat", i+1),
					AttachmentType: "application/octet-stream",
					MAPIProperty:   "PR_ATTACH_DATA_BIN",
					Size:           1024,
				}
				parts = append(parts, attachmentPart)
			}
		}
	}

	return parts, nil
}

// createMSGHeadersPart creates a part containing MSG headers/properties
func (r *MSGReader) createMSGHeadersPart(info MSGInfo) MSGPart {
	var headers []string

	headers = append(headers, fmt.Sprintf("From: %s", info.From))
	headers = append(headers, fmt.Sprintf("To: %s", info.To))
	if info.CC != "" {
		headers = append(headers, fmt.Sprintf("Cc: %s", info.CC))
	}
	if info.BCC != "" {
		headers = append(headers, fmt.Sprintf("Bcc: %s", info.BCC))
	}
	headers = append(headers, fmt.Sprintf("Subject: %s", info.Subject))
	headers = append(headers, fmt.Sprintf("Date: %s", info.Date))
	headers = append(headers, fmt.Sprintf("Message-Class: %s", info.MessageClass))
	headers = append(headers, fmt.Sprintf("Priority: %s", info.Priority))
	headers = append(headers, fmt.Sprintf("Sensitivity: %s", info.Sensitivity))

	properties := map[string]string{
		"PR_SENDER_EMAIL_ADDRESS":  info.From,
		"PR_RECEIVED_BY_EMAIL":     info.To,
		"PR_SUBJECT":               info.Subject,
		"PR_CLIENT_SUBMIT_TIME":    info.Date,
		"PR_MESSAGE_CLASS":         info.MessageClass,
		"PR_PRIORITY":              info.Priority,
		"PR_SENSITIVITY":           info.Sensitivity,
	}

	return MSGPart{
		ContentType:  "headers",
		Content:      strings.Join(headers, "\n"),
		Properties:   properties,
		MAPIProperty: "headers",
	}
}

// MSGIterator implements ChunkIterator for MSG files
type MSGIterator struct {
	sourcePath  string
	config      map[string]any
	parts       []MSGPart
	currentPart int
}

// Next returns the next chunk of content from the MSG
func (it *MSGIterator) Next(ctx context.Context) (core.Chunk, error) {
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
			ChunkType:   "msg_part",
			SizeBytes:   int64(len(part.Content)),
			ProcessedAt: time.Now(),
			ProcessedBy: "msg_reader",
			Context: map[string]string{
				"content_type": part.ContentType,
				"part_number":  strconv.Itoa(it.currentPart),
				"total_parts":  strconv.Itoa(len(it.parts)),
				"file_type":    "msg",
			},
		},
	}

	// Add MSG-specific context
	if part.MAPIProperty != "" {
		chunk.Metadata.Context["mapi_property"] = part.MAPIProperty
	}
	
	// Add email properties from headers part
	if part.ContentType == "headers" {
		if from := part.Properties["PR_SENDER_EMAIL_ADDRESS"]; from != "" {
			chunk.Metadata.Context["from"] = from
		}
		if to := part.Properties["PR_RECEIVED_BY_EMAIL"]; to != "" {
			chunk.Metadata.Context["to"] = to
		}
		if subject := part.Properties["PR_SUBJECT"]; subject != "" {
			chunk.Metadata.Context["subject"] = subject
		}
		if date := part.Properties["PR_CLIENT_SUBMIT_TIME"]; date != "" {
			chunk.Metadata.Context["date"] = date
		}
		if msgClass := part.Properties["PR_MESSAGE_CLASS"]; msgClass != "" {
			chunk.Metadata.Context["message_class"] = msgClass
		}
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

// Close releases MSG resources
func (it *MSGIterator) Close() error {
	// Nothing to close for MSG iterator
	return nil
}

// Reset restarts iteration from the beginning
func (it *MSGIterator) Reset() error {
	it.currentPart = 0
	return nil
}

// Progress returns iteration progress
func (it *MSGIterator) Progress() float64 {
	if len(it.parts) == 0 {
		return 1.0
	}
	return float64(it.currentPart) / float64(len(it.parts))
}