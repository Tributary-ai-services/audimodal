package email

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jscharber/eAIIngest/pkg/core"
)

// PSTReader implements DataSourceReader for PST (Personal Storage Table) archive files
type PSTReader struct {
	name    string
	version string
}

// NewPSTReader creates a new PST file reader
func NewPSTReader() *PSTReader {
	return &PSTReader{
		name:    "pst_reader",
		version: "1.0.0",
	}
}

// GetConfigSpec returns the configuration specification
func (r *PSTReader) GetConfigSpec() []core.ConfigSpec {
	return []core.ConfigSpec{
		{
			Name:        "extract_emails",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Extract email messages from PST archive",
		},
		{
			Name:        "extract_attachments",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Extract email attachments as separate chunks",
		},
		{
			Name:        "extract_contacts",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Extract contact entries from PST archive",
		},
		{
			Name:        "extract_calendar",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Extract calendar entries from PST archive",
		},
		{
			Name:        "extract_tasks",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Extract task entries from PST archive",
		},
		{
			Name:        "include_folders",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Include folder hierarchy information",
		},
		{
			Name:        "max_attachment_size_mb",
			Type:        "int",
			Required:    false,
			Default:     100,
			Description: "Maximum attachment size to process in MB",
			MinValue:    ptrFloat64(0.0),
			MaxValue:    ptrFloat64(1000.0),
		},
		{
			Name:        "extract_deleted_items",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Extract items from deleted items folder",
		},
		{
			Name:        "decode_unicode",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Decode Unicode strings in PST format",
		},
		{
			Name:        "include_metadata",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Include PST metadata and properties",
		},
	}
}

// ValidateConfig validates the provided configuration
func (r *PSTReader) ValidateConfig(config map[string]any) error {
	if maxSize, ok := config["max_attachment_size_mb"]; ok {
		if num, ok := maxSize.(float64); ok {
			if num < 0 || num > 1000 {
				return fmt.Errorf("max_attachment_size_mb must be between 0 and 1000")
			}
		}
	}

	return nil
}

// TestConnection tests if the PST can be read
func (r *PSTReader) TestConnection(ctx context.Context, config map[string]any) core.ConnectionTestResult {
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
		Message: "PST reader ready",
		Latency: time.Since(start),
		Details: map[string]any{
			"extract_emails":      config["extract_emails"],
			"extract_attachments": config["extract_attachments"],
			"include_folders":     config["include_folders"],
		},
	}
}

// GetType returns the connector type
func (r *PSTReader) GetType() string {
	return "reader"
}

// GetName returns the reader name
func (r *PSTReader) GetName() string {
	return r.name
}

// GetVersion returns the reader version
func (r *PSTReader) GetVersion() string {
	return r.version
}

// DiscoverSchema analyzes the PST file structure
func (r *PSTReader) DiscoverSchema(ctx context.Context, sourcePath string) (core.SchemaInfo, error) {
	// Validate PST file format
	if !r.isPSTFile(sourcePath) {
		return core.SchemaInfo{}, fmt.Errorf("not a valid PST file")
	}

	pstInfo, err := r.extractPSTInfo(sourcePath)
	if err != nil {
		return core.SchemaInfo{}, fmt.Errorf("failed to extract PST info: %w", err)
	}

	schema := core.SchemaInfo{
		Format:   "pst",
		Encoding: "binary",
		Fields: []core.FieldInfo{
			{
				Name:        "content",
				Type:        "text",
				Nullable:    false,
				Description: "Email content, contact info, or calendar entry",
			},
			{
				Name:        "item_type",
				Type:        "string",
				Nullable:    false,
				Description: "Type of item (email, contact, calendar, task, attachment)",
			},
			{
				Name:        "folder_path",
				Type:        "string",
				Nullable:    true,
				Description: "Folder hierarchy path",
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
				Description: "Item date/timestamp",
			},
			{
				Name:        "attachment_name",
				Type:        "string",
				Nullable:    true,
				Description: "Attachment filename if applicable",
			},
			{
				Name:        "message_id",
				Type:        "string",
				Nullable:    true,
				Description: "Unique message identifier",
			},
		},
		Metadata: map[string]any{
			"pst_version":      pstInfo.PSTVersion,
			"created_by":       pstInfo.CreatedBy,
			"last_modified":    pstInfo.LastModified,
			"total_items":      pstInfo.TotalItems,
			"email_count":      pstInfo.EmailCount,
			"folder_count":     pstInfo.FolderCount,
			"attachment_count": pstInfo.AttachmentCount,
			"contact_count":    pstInfo.ContactCount,
			"calendar_count":   pstInfo.CalendarCount,
			"encryption_type":  pstInfo.EncryptionType,
			"is_unicode":       pstInfo.IsUnicode,
			"file_size":        pstInfo.FileSize,
		},
	}

	// Sample first email for analysis
	if pstInfo.EmailCount > 0 {
		schema.SampleData = []map[string]any{
			{
				"content":      "Sample email content extracted from PST archive",
				"item_type":    "email",
				"folder_path":  "Inbox",
				"from":         "sender@example.com",
				"to":           "recipient@example.com",
				"subject":      "Sample PST Email Subject",
				"date":         time.Now().Format("2006-01-02 15:04:05"),
				"message_id":   "sample-message-id@example.com",
			},
		}
	}

	return schema, nil
}

// EstimateSize returns size estimates for the PST file
func (r *PSTReader) EstimateSize(ctx context.Context, sourcePath string) (core.SizeEstimate, error) {
	stat, err := os.Stat(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to stat file: %w", err)
	}

	pstInfo, err := r.extractPSTInfo(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to extract PST info: %w", err)
	}

	// Estimate chunks based on items in PST
	estimatedChunks := pstInfo.TotalItems
	if estimatedChunks == 0 {
		estimatedChunks = 1
	}

	// PST files are inherently complex due to proprietary format
	complexity := "high"
	if stat.Size() > 100*1024*1024 || pstInfo.TotalItems > 10000 { // > 100MB or > 10k items
		complexity = "very_high"
	}

	// PST processing is typically slow due to format complexity
	processTime := "slow"
	if stat.Size() > 1*1024*1024*1024 || pstInfo.TotalItems > 50000 { // > 1GB or > 50k items
		processTime = "very_slow"
	}

	totalItems := int64(pstInfo.TotalItems)
	return core.SizeEstimate{
		RowCount:    &totalItems,
		ByteSize:    stat.Size(),
		Complexity:  complexity,
		ChunkEst:    estimatedChunks,
		ProcessTime: processTime,
	}, nil
}

// CreateIterator creates a chunk iterator for the PST file
func (r *PSTReader) CreateIterator(ctx context.Context, sourcePath string, strategyConfig map[string]any) (core.ChunkIterator, error) {
	if !r.isPSTFile(sourcePath) {
		return nil, fmt.Errorf("not a valid PST file")
	}

	items, err := r.parsePSTItems(sourcePath, strategyConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to parse PST items: %w", err)
	}

	iterator := &PSTIterator{
		sourcePath:  sourcePath,
		config:      strategyConfig,
		items:       items,
		currentItem: 0,
	}

	return iterator, nil
}

// SupportsStreaming indicates PST reader supports streaming
func (r *PSTReader) SupportsStreaming() bool {
	return true
}

// GetSupportedFormats returns supported file formats
func (r *PSTReader) GetSupportedFormats() []string {
	return []string{"pst"}
}

// PSTInfo contains extracted PST information
type PSTInfo struct {
	PSTVersion      string
	CreatedBy       string
	LastModified    string
	TotalItems      int
	EmailCount      int
	FolderCount     int
	AttachmentCount int
	ContactCount    int
	CalendarCount   int
	EncryptionType  string
	IsUnicode       bool
	FileSize        int64
}

// PSTItem represents an item from a PST archive
type PSTItem struct {
	ItemType       string
	Content        string
	FolderPath     string
	Properties     map[string]string
	From           string
	To             string
	CC             string
	BCC            string
	Subject        string
	Date           string
	MessageID      string
	IsAttachment   bool
	AttachmentName string
	AttachmentType string
	Size           int64
}

// isPSTFile checks if the file is a valid PST file by checking signature
func (r *PSTReader) isPSTFile(filePath string) bool {
	file, err := os.Open(filePath)
	if err != nil {
		return false
	}
	defer file.Close()

	// Check PST signature: "!BDN" at offset 0
	signature := make([]byte, 4)
	n, err := file.Read(signature)
	if err != nil || n != 4 {
		return false
	}

	// PST files start with "!BDN" signature
	expectedSignature := []byte{'!', 'B', 'D', 'N'}
	return bytes.Equal(signature, expectedSignature)
}

// extractPSTInfo extracts basic information from PST file
func (r *PSTReader) extractPSTInfo(filePath string) (PSTInfo, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return PSTInfo{}, err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return PSTInfo{}, err
	}

	// Read PST header (first 564 bytes contain the header)
	header := make([]byte, 564)
	n, err := file.Read(header)
	if err != nil || n < 564 {
		return PSTInfo{}, fmt.Errorf("failed to read PST header")
	}

	info := PSTInfo{
		FileSize:     stat.Size(),
		LastModified: stat.ModTime().Format("2006-01-02 15:04:05"),
	}

	// Parse PST version from header
	// Bytes 10-11 contain version info
	if len(header) >= 12 {
		version := binary.LittleEndian.Uint16(header[10:12])
		switch version {
		case 14, 15:
			info.PSTVersion = "ANSI"
			info.IsUnicode = false
		case 23:
			info.PSTVersion = "Unicode"
			info.IsUnicode = true
		default:
			info.PSTVersion = fmt.Sprintf("Unknown (%d)", version)
		}
	}

	// Mock item counts based on file size (in a real implementation, 
	// you would parse the PST structure to get actual counts)
	sizeCategory := stat.Size() / (1024 * 1024) // Size in MB
	
	// Estimate items based on file size
	switch {
	case sizeCategory < 10: // < 10MB
		info.EmailCount = 50
		info.FolderCount = 5
		info.AttachmentCount = 10
		info.ContactCount = 20
		info.CalendarCount = 15
	case sizeCategory < 100: // 10-100MB
		info.EmailCount = 500
		info.FolderCount = 15
		info.AttachmentCount = 100
		info.ContactCount = 100
		info.CalendarCount = 50
	case sizeCategory < 1000: // 100MB-1GB
		info.EmailCount = 5000
		info.FolderCount = 30
		info.AttachmentCount = 1000
		info.ContactCount = 500
		info.CalendarCount = 200
	default: // > 1GB
		info.EmailCount = 20000
		info.FolderCount = 50
		info.AttachmentCount = 5000
		info.ContactCount = 2000
		info.CalendarCount = 1000
	}

	info.TotalItems = info.EmailCount + info.ContactCount + info.CalendarCount + info.AttachmentCount
	info.CreatedBy = "Microsoft Outlook"
	info.EncryptionType = "None" // PST files can be password protected

	return info, nil
}

// parsePSTItems parses all items from a PST archive
func (r *PSTReader) parsePSTItems(filePath string, config map[string]any) ([]PSTItem, error) {
	var items []PSTItem

	pstInfo, err := r.extractPSTInfo(filePath)
	if err != nil {
		return nil, err
	}

	// Extract emails if configured
	if extractEmails, ok := config["extract_emails"]; !ok || extractEmails.(bool) {
		emailItems := r.createMockEmailItems(pstInfo.EmailCount, config)
		items = append(items, emailItems...)
	}

	// Extract contacts if configured
	if extractContacts, ok := config["extract_contacts"]; ok && extractContacts.(bool) {
		contactItems := r.createMockContactItems(pstInfo.ContactCount)
		items = append(items, contactItems...)
	}

	// Extract calendar items if configured
	if extractCalendar, ok := config["extract_calendar"]; ok && extractCalendar.(bool) {
		calendarItems := r.createMockCalendarItems(pstInfo.CalendarCount)
		items = append(items, calendarItems...)
	}

	// Extract attachments if configured
	if extractAttachments, ok := config["extract_attachments"]; !ok || extractAttachments.(bool) {
		attachmentItems := r.createMockAttachmentItems(pstInfo.AttachmentCount, config)
		items = append(items, attachmentItems...)
	}

	return items, nil
}

// createMockEmailItems creates mock email items for demonstration
func (r *PSTReader) createMockEmailItems(count int, config map[string]any) []PSTItem {
	var items []PSTItem
	
	folders := []string{"Inbox", "Sent Items", "Drafts", "Deleted Items", "Junk Email"}
	
	for i := 0; i < count; i++ {
		folder := folders[i%len(folders)]
		
		// Skip deleted items unless configured
		if folder == "Deleted Items" {
			if extractDeleted, ok := config["extract_deleted_items"]; !ok || !extractDeleted.(bool) {
				continue
			}
		}
		
		item := PSTItem{
			ItemType:   "email",
			Content:    fmt.Sprintf("This is the content of email %d from the PST archive. It contains the message body text.", i+1),
			FolderPath: folder,
			From:       fmt.Sprintf("sender%d@example.com", (i%10)+1),
			To:         fmt.Sprintf("recipient%d@example.com", (i%5)+1),
			Subject:    fmt.Sprintf("Email Subject %d from PST Archive", i+1),
			Date:       time.Now().AddDate(0, 0, -i).Format("2006-01-02 15:04:05"),
			MessageID:  fmt.Sprintf("msg-%d@example.com", i+1),
			Properties: map[string]string{
				"PR_MESSAGE_CLASS":        "IPM.Note",
				"PR_IMPORTANCE":          "Normal",
				"PR_SENSITIVITY":         "Normal",
				"PR_MESSAGE_FLAGS":       "1",
				"PR_CLIENT_SUBMIT_TIME":  time.Now().AddDate(0, 0, -i).Format("2006-01-02 15:04:05"),
			},
			Size: int64(len(fmt.Sprintf("Email content %d", i+1)) + 200), // Estimated size
		}
		
		items = append(items, item)
	}
	
	return items
}

// createMockContactItems creates mock contact items for demonstration
func (r *PSTReader) createMockContactItems(count int) []PSTItem {
	var items []PSTItem
	
	for i := 0; i < count; i++ {
		content := fmt.Sprintf(`Contact: Contact Person %d
Email: contact%d@example.com
Phone: +1-555-0%03d
Company: Company %d
Address: %d Main Street, City, State`, i+1, i+1, 100+i, (i%10)+1, (i*10)+100)
		
		item := PSTItem{
			ItemType:   "contact",
			Content:    content,
			FolderPath: "Contacts",
			Subject:    fmt.Sprintf("Contact Person %d", i+1),
			Date:       time.Now().AddDate(0, 0, -i*2).Format("2006-01-02 15:04:05"),
			Properties: map[string]string{
				"PR_MESSAGE_CLASS":     "IPM.Contact",
				"PR_DISPLAY_NAME":      fmt.Sprintf("Contact Person %d", i+1),
				"PR_EMAIL_ADDRESS":     fmt.Sprintf("contact%d@example.com", i+1),
				"PR_BUSINESS_TELEPHONE_NUMBER": fmt.Sprintf("+1-555-0%03d", 100+i),
				"PR_COMPANY_NAME":      fmt.Sprintf("Company %d", (i%10)+1),
			},
			Size: int64(len(content)),
		}
		
		items = append(items, item)
	}
	
	return items
}

// createMockCalendarItems creates mock calendar items for demonstration
func (r *PSTReader) createMockCalendarItems(count int) []PSTItem {
	var items []PSTItem
	
	for i := 0; i < count; i++ {
		startTime := time.Now().AddDate(0, 0, i-30) // Events from past month to future
		endTime := startTime.Add(time.Hour)
		
		content := fmt.Sprintf(`Meeting: %s
Start: %s
End: %s
Location: Conference Room %d
Attendees: attendee%d@example.com, attendee%d@example.com
Description: This is the description for calendar event %d from the PST archive.`,
			fmt.Sprintf("Meeting %d", i+1),
			startTime.Format("2006-01-02 15:04:05"),
			endTime.Format("2006-01-02 15:04:05"),
			(i%5)+1, (i%3)+1, (i%3)+2, i+1)
		
		item := PSTItem{
			ItemType:   "calendar",
			Content:    content,
			FolderPath: "Calendar",
			Subject:    fmt.Sprintf("Meeting %d", i+1),
			Date:       startTime.Format("2006-01-02 15:04:05"),
			Properties: map[string]string{
				"PR_MESSAGE_CLASS":    "IPM.Appointment",
				"PR_START_DATE":       startTime.Format("2006-01-02 15:04:05"),
				"PR_END_DATE":         endTime.Format("2006-01-02 15:04:05"),
				"PR_LOCATION":         fmt.Sprintf("Conference Room %d", (i%5)+1),
				"PR_MEETING_TYPE":     "1",
			},
			Size: int64(len(content)),
		}
		
		items = append(items, item)
	}
	
	return items
}

// createMockAttachmentItems creates mock attachment items for demonstration
func (r *PSTReader) createMockAttachmentItems(count int, config map[string]any) []PSTItem {
	var items []PSTItem
	
	attachmentTypes := []string{".pdf", ".docx", ".xlsx", ".pptx", ".jpg", ".png", ".zip", ".txt"}
	
	// Get max attachment size from config
	maxSizeMB := 100
	if maxSize, ok := config["max_attachment_size_mb"]; ok {
		if size, ok := maxSize.(float64); ok {
			maxSizeMB = int(size)
		}
	}
	
	for i := 0; i < count; i++ {
		fileType := attachmentTypes[i%len(attachmentTypes)]
		fileName := fmt.Sprintf("attachment_%d%s", i+1, fileType)
		
		// Mock attachment size (KB to MB range)
		attachmentSize := int64((i%1000 + 1) * 1024) // 1KB to 1MB
		
		// Skip if over size limit
		if attachmentSize > int64(maxSizeMB*1024*1024) {
			continue
		}
		
		content := fmt.Sprintf("[Binary attachment data for %s - Size: %d bytes]", fileName, attachmentSize)
		
		item := PSTItem{
			ItemType:       "attachment",
			Content:        content,
			FolderPath:     "Attachments",
			IsAttachment:   true,
			AttachmentName: fileName,
			AttachmentType: strings.TrimPrefix(fileType, "."),
			Date:           time.Now().AddDate(0, 0, -i).Format("2006-01-02 15:04:05"),
			Properties: map[string]string{
				"PR_ATTACH_FILENAME":    fileName,
				"PR_ATTACH_EXTENSION":   fileType,
				"PR_ATTACH_SIZE":        strconv.FormatInt(attachmentSize, 10),
				"PR_ATTACH_METHOD":      "1",
			},
			Size: attachmentSize,
		}
		
		items = append(items, item)
	}
	
	return items
}

// PSTIterator implements ChunkIterator for PST files
type PSTIterator struct {
	sourcePath  string
	config      map[string]any
	items       []PSTItem
	currentItem int
}

// Next returns the next chunk of content from the PST
func (it *PSTIterator) Next(ctx context.Context) (core.Chunk, error) {
	select {
	case <-ctx.Done():
		return core.Chunk{}, ctx.Err()
	default:
	}

	// Check if we've processed all items
	if it.currentItem >= len(it.items) {
		return core.Chunk{}, core.ErrIteratorExhausted
	}

	item := it.items[it.currentItem]
	it.currentItem++

	chunk := core.Chunk{
		Data: item.Content,
		Metadata: core.ChunkMetadata{
			SourcePath:  it.sourcePath,
			ChunkID:     fmt.Sprintf("%s:%s:%d", filepath.Base(it.sourcePath), item.ItemType, it.currentItem),
			ChunkType:   "pst_item",
			SizeBytes:   item.Size,
			ProcessedAt: time.Now(),
			ProcessedBy: "pst_reader",
			Context: map[string]string{
				"item_type":    item.ItemType,
				"folder_path":  item.FolderPath,
				"item_number":  strconv.Itoa(it.currentItem),
				"total_items":  strconv.Itoa(len(it.items)),
				"file_type":    "pst",
			},
		},
	}

	// Add email-specific context
	if item.ItemType == "email" {
		if item.From != "" {
			chunk.Metadata.Context["from"] = item.From
		}
		if item.To != "" {
			chunk.Metadata.Context["to"] = item.To
		}
		if item.Subject != "" {
			chunk.Metadata.Context["subject"] = item.Subject
		}
		if item.Date != "" {
			chunk.Metadata.Context["date"] = item.Date
		}
		if item.MessageID != "" {
			chunk.Metadata.Context["message_id"] = item.MessageID
		}
	}

	// Add attachment-specific context
	if item.IsAttachment {
		chunk.Metadata.Context["is_attachment"] = "true"
		chunk.Metadata.Context["attachment_name"] = item.AttachmentName
		chunk.Metadata.Context["attachment_type"] = item.AttachmentType
		chunk.Metadata.Context["attachment_size"] = strconv.FormatInt(item.Size, 10)
	}

	// Add PST properties
	for key, value := range item.Properties {
		chunk.Metadata.Context["pst_"+strings.ToLower(key)] = value
	}

	return chunk, nil
}

// Close releases PST resources
func (it *PSTIterator) Close() error {
	// Nothing to close for PST iterator
	return nil
}

// Reset restarts iteration from the beginning
func (it *PSTIterator) Reset() error {
	it.currentItem = 0
	return nil
}

// Progress returns iteration progress
func (it *PSTIterator) Progress() float64 {
	if len(it.items) == 0 {
		return 1.0
	}
	return float64(it.currentItem) / float64(len(it.items))
}

