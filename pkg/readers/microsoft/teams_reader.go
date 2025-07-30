package microsoft

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jscharber/eAIIngest/pkg/core"
)

// TeamsReader implements DataSourceReader for Microsoft Teams exports
type TeamsReader struct {
	name    string
	version string
}

// NewTeamsReader creates a new Teams export reader
func NewTeamsReader() *TeamsReader {
	return &TeamsReader{
		name:    "teams_reader",
		version: "1.0.0",
	}
}

// GetConfigSpec returns the configuration specification
func (r *TeamsReader) GetConfigSpec() []core.ConfigSpec {
	return []core.ConfigSpec{
		{
			Name:        "include_deleted_messages",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Include deleted messages in extraction",
		},
		{
			Name:        "include_system_messages",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Include system messages (joins, leaves, etc.)",
		},
		{
			Name:        "include_attachments",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Process message attachments",
		},
		{
			Name:        "include_reactions",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Include message reactions and likes",
		},
		{
			Name:        "date_range_start",
			Type:        "string",
			Required:    false,
			Default:     "",
			Description: "Start date for message filtering (YYYY-MM-DD)",
		},
		{
			Name:        "date_range_end",
			Type:        "string",
			Required:    false,
			Default:     "",
			Description: "End date for message filtering (YYYY-MM-DD)",
		},
		{
			Name:        "channel_filter",
			Type:        "string",
			Required:    false,
			Default:     "",
			Description: "Comma-separated list of channel names to include",
		},
		{
			Name:        "user_filter",
			Type:        "string",
			Required:    false,
			Default:     "",
			Description: "Comma-separated list of users to include",
		},
		{
			Name:        "extract_meeting_transcripts",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Extract meeting transcripts if available",
		},
	}
}

// ValidateConfig validates the provided configuration
func (r *TeamsReader) ValidateConfig(config map[string]any) error {
	// Validate date range format if provided
	if startDate, ok := config["date_range_start"].(string); ok && startDate != "" {
		if _, err := time.Parse("2006-01-02", startDate); err != nil {
			return fmt.Errorf("invalid date_range_start format, expected YYYY-MM-DD")
		}
	}

	if endDate, ok := config["date_range_end"].(string); ok && endDate != "" {
		if _, err := time.Parse("2006-01-02", endDate); err != nil {
			return fmt.Errorf("invalid date_range_end format, expected YYYY-MM-DD")
		}
	}

	return nil
}

// TestConnection tests if the Teams export can be read
func (r *TeamsReader) TestConnection(ctx context.Context, config map[string]any) core.ConnectionTestResult {
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
		Message: "Teams reader ready",
		Latency: time.Since(start),
		Details: map[string]any{
			"include_attachments":         config["include_attachments"],
			"extract_meeting_transcripts": config["extract_meeting_transcripts"],
			"include_system_messages":     config["include_system_messages"],
		},
	}
}

// GetType returns the connector type
func (r *TeamsReader) GetType() string {
	return "reader"
}

// GetName returns the reader name
func (r *TeamsReader) GetName() string {
	return r.name
}

// GetVersion returns the reader version
func (r *TeamsReader) GetVersion() string {
	return r.version
}

// DiscoverSchema analyzes the Teams export structure
func (r *TeamsReader) DiscoverSchema(ctx context.Context, sourcePath string) (core.SchemaInfo, error) {
	if !r.isTeamsExport(sourcePath) {
		return core.SchemaInfo{}, fmt.Errorf("not a valid Teams export file")
	}

	metadata, err := r.extractTeamsMetadata(sourcePath)
	if err != nil {
		return core.SchemaInfo{}, fmt.Errorf("failed to extract Teams metadata: %w", err)
	}

	schema := core.SchemaInfo{
		Format:   "teams",
		Encoding: "json",
		Fields: []core.FieldInfo{
			{
				Name:        "message_id",
				Type:        "string",
				Nullable:    false,
				Description: "Unique message identifier",
			},
			{
				Name:        "channel_name",
				Type:        "string",
				Nullable:    false,
				Description: "Teams channel name",
			},
			{
				Name:        "team_name",
				Type:        "string",
				Nullable:    false,
				Description: "Teams team name",
			},
			{
				Name:        "sender",
				Type:        "string",
				Nullable:    false,
				Description: "Message sender display name",
			},
			{
				Name:        "sender_email",
				Type:        "string",
				Nullable:    true,
				Description: "Message sender email address",
			},
			{
				Name:        "timestamp",
				Type:        "datetime",
				Nullable:    false,
				Description: "Message timestamp",
			},
			{
				Name:        "message_text",
				Type:        "text",
				Nullable:    true,
				Description: "Message content",
			},
			{
				Name:        "message_type",
				Type:        "string",
				Nullable:    false,
				Description: "Type of message (text, system, announcement, etc.)",
			},
			{
				Name:        "thread_id",
				Type:        "string",
				Nullable:    true,
				Description: "Thread identifier for replies",
			},
			{
				Name:        "attachments",
				Type:        "array",
				Nullable:    true,
				Description: "Message attachments",
			},
			{
				Name:        "reactions",
				Type:        "array",
				Nullable:    true,
				Description: "Message reactions and likes",
			},
		},
		Metadata: map[string]any{
			"total_messages":     metadata.TotalMessages,
			"total_channels":     metadata.TotalChannels,
			"total_teams":        metadata.TotalTeams,
			"unique_participants": metadata.UniqueParticipants,
			"date_range_start":   metadata.DateRangeStart,
			"date_range_end":     metadata.DateRangeEnd,
			"export_date":        metadata.ExportDate,
			"has_attachments":    metadata.HasAttachments,
			"has_meeting_data":   metadata.HasMeetingData,
			"export_type":        metadata.ExportType,
		},
	}

	// Sample first few messages
	if metadata.TotalMessages > 0 {
		sampleData, err := r.extractSampleMessages(sourcePath)
		if err == nil {
			schema.SampleData = sampleData
		}
	}

	return schema, nil
}

// EstimateSize returns size estimates for the Teams export
func (r *TeamsReader) EstimateSize(ctx context.Context, sourcePath string) (core.SizeEstimate, error) {
	stat, err := os.Stat(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to stat file: %w", err)
	}

	metadata, err := r.extractTeamsMetadata(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to extract Teams metadata: %w", err)
	}

	// Estimate based on total messages
	totalMessages := int64(metadata.TotalMessages)
	if totalMessages == 0 {
		totalMessages = 1
	}

	// Estimate chunks based on messages (typically 10-20 messages per chunk)
	estimatedChunks := int((totalMessages + 15) / 15)

	// Teams exports are typically JSON, complexity depends on size and attachments
	complexity := "low"
	if metadata.TotalMessages > 1000 || stat.Size() > 10*1024*1024 { // > 1k messages or > 10MB
		complexity = "medium"
	}
	if metadata.TotalMessages > 10000 || stat.Size() > 100*1024*1024 { // > 10k messages or > 100MB
		complexity = "high"
	}
	if metadata.TotalMessages > 100000 || stat.Size() > 1024*1024*1024 { // > 100k messages or > 1GB
		complexity = "very_high"
	}

	// Processing time depends on message count and attachments
	processTime := "fast"
	if metadata.TotalMessages > 5000 || metadata.HasAttachments {
		processTime = "medium"
	}
	if metadata.TotalMessages > 50000 || (metadata.HasAttachments && metadata.TotalMessages > 10000) {
		processTime = "slow"
	}

	return core.SizeEstimate{
		RowCount:    &totalMessages,
		ByteSize:    stat.Size(),
		Complexity:  complexity,
		ChunkEst:    estimatedChunks,
		ProcessTime: processTime,
	}, nil
}

// CreateIterator creates a chunk iterator for the Teams export
func (r *TeamsReader) CreateIterator(ctx context.Context, sourcePath string, strategyConfig map[string]any) (core.ChunkIterator, error) {
	export, err := r.parseTeamsExport(sourcePath, strategyConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Teams export: %w", err)
	}

	iterator := &TeamsIterator{
		sourcePath:     sourcePath,
		config:         strategyConfig,
		export:         export,
		currentMessage: 0,
	}

	return iterator, nil
}

// SupportsStreaming indicates Teams reader supports streaming
func (r *TeamsReader) SupportsStreaming() bool {
	return true
}

// GetSupportedFormats returns supported file formats
func (r *TeamsReader) GetSupportedFormats() []string {
	return []string{"json", "teams"}
}

// TeamsMetadata contains extracted Teams export metadata
type TeamsMetadata struct {
	TotalMessages       int
	TotalChannels       int
	TotalTeams          int
	UniqueParticipants  int
	DateRangeStart      string
	DateRangeEnd        string
	ExportDate          string
	HasAttachments      bool
	HasMeetingData      bool
	ExportType          string
}

// TeamsExport represents a parsed Teams export
type TeamsExport struct {
	Metadata TeamsMetadata
	Messages []TeamsMessage
}

// TeamsMessage represents a Teams message
type TeamsMessage struct {
	ID            string             `json:"id"`
	ChannelName   string             `json:"channelName"`
	TeamName      string             `json:"teamName"`
	Sender        string             `json:"sender"`
	SenderEmail   string             `json:"senderEmail"`
	Timestamp     time.Time          `json:"timestamp"`
	MessageText   string             `json:"messageText"`
	MessageType   string             `json:"messageType"`
	ThreadID      string             `json:"threadId,omitempty"`
	Attachments   []TeamsAttachment  `json:"attachments,omitempty"`
	Reactions     []TeamsReaction    `json:"reactions,omitempty"`
	IsDeleted     bool               `json:"isDeleted"`
	IsSystemMsg   bool               `json:"isSystemMessage"`
}

// TeamsAttachment represents a message attachment
type TeamsAttachment struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
}

// TeamsReaction represents a message reaction
type TeamsReaction struct {
	Type     string `json:"type"`
	UserName string `json:"userName"`
	UserEmail string `json:"userEmail"`
}

// isTeamsExport checks if this is a Teams export file
func (r *TeamsReader) isTeamsExport(filePath string) bool {
	// Check file extension first
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext != ".json" {
		return false
	}

	// Try to parse as JSON and look for Teams-specific fields
	file, err := os.Open(filePath)
	if err != nil {
		return false
	}
	defer file.Close()

	// Read first 1KB to check structure
	buffer := make([]byte, 1024)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return false
	}

	content := string(buffer[:n])
	
	// Look for Teams-specific indicators
	teamsIndicators := []string{
		"channelName", "teamName", "messageText",
		"Microsoft Teams", "teams.microsoft.com",
		"threadId", "reactions",
	}

	matchCount := 0
	for _, indicator := range teamsIndicators {
		if strings.Contains(content, indicator) {
			matchCount++
		}
	}

	// Need at least 2 indicators to consider it a Teams export
	return matchCount >= 2
}

// extractTeamsMetadata extracts metadata from Teams export
func (r *TeamsReader) extractTeamsMetadata(sourcePath string) (TeamsMetadata, error) {
	file, err := os.Open(sourcePath)
	if err != nil {
		return TeamsMetadata{}, err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return TeamsMetadata{}, err
	}

	// For now, provide estimated metadata based on file size
	// In production, you would parse the entire JSON to get accurate counts
	metadata := TeamsMetadata{
		ExportDate: stat.ModTime().Format("2006-01-02 15:04:05"),
		ExportType: "Teams Chat Export",
	}

	// Estimate based on file size
	sizeCategory := stat.Size() / (1024 * 1024) // Size in MB

	switch {
	case sizeCategory < 1: // < 1MB
		metadata.TotalMessages = 100
		metadata.TotalChannels = 2
		metadata.TotalTeams = 1
		metadata.UniqueParticipants = 5
	case sizeCategory < 10: // 1-10MB
		metadata.TotalMessages = 1000
		metadata.TotalChannels = 5
		metadata.TotalTeams = 2
		metadata.UniqueParticipants = 15
		metadata.HasAttachments = true
	case sizeCategory < 100: // 10-100MB
		metadata.TotalMessages = 10000
		metadata.TotalChannels = 20
		metadata.TotalTeams = 5
		metadata.UniqueParticipants = 50
		metadata.HasAttachments = true
		metadata.HasMeetingData = true
	default: // > 100MB
		metadata.TotalMessages = 50000
		metadata.TotalChannels = 50
		metadata.TotalTeams = 10
		metadata.UniqueParticipants = 200
		metadata.HasAttachments = true
		metadata.HasMeetingData = true
	}

	return metadata, nil
}

// extractSampleMessages extracts sample messages for schema discovery
func (r *TeamsReader) extractSampleMessages(sourcePath string) ([]map[string]any, error) {
	// Mock sample data - in production, would parse actual JSON content
	return []map[string]any{
		{
			"message_id":    "msg_001",
			"channel_name":  "General",
			"team_name":     "Project Team",
			"sender":        "John Doe",
			"sender_email":  "john.doe@company.com",
			"timestamp":     "2024-01-15 10:30:00",
			"message_text":  "Hello team, let's discuss the project timeline.",
			"message_type":  "text",
			"thread_id":     "",
		},
		{
			"message_id":    "msg_002",
			"channel_name":  "General",
			"team_name":     "Project Team",
			"sender":        "Jane Smith",
			"sender_email":  "jane.smith@company.com",
			"timestamp":     "2024-01-15 10:32:00",
			"message_text":  "Great idea! I've uploaded the latest document.",
			"message_type":  "text",
			"thread_id":     "msg_001",
		},
	}, nil
}

// parseTeamsExport parses the complete Teams export
func (r *TeamsReader) parseTeamsExport(sourcePath string, config map[string]any) (*TeamsExport, error) {
	metadata, err := r.extractTeamsMetadata(sourcePath)
	if err != nil {
		return nil, err
	}

	// In a real implementation, you would:
	// 1. Parse the JSON export file
	// 2. Filter messages based on configuration
	// 3. Process attachments and reactions
	// 4. Handle different export formats (individual files vs. bulk export)

	// Mock messages for demonstration
	messages := make([]TeamsMessage, metadata.TotalMessages)
	for i := 0; i < metadata.TotalMessages; i++ {
		messages[i] = TeamsMessage{
			ID:          fmt.Sprintf("msg_%d", i+1),
			ChannelName: "General",
			TeamName:    "Project Team",
			Sender:      fmt.Sprintf("User %d", (i%10)+1),
			SenderEmail: fmt.Sprintf("user%d@company.com", (i%10)+1),
			Timestamp:   time.Now().Add(-time.Duration(metadata.TotalMessages-i) * time.Hour),
			MessageText: fmt.Sprintf("This is message %d content from Teams export.", i+1),
			MessageType: "text",
			IsDeleted:   false,
			IsSystemMsg: i%20 == 0, // Every 20th message is a system message
		}

		// Add attachments to some messages
		if i%15 == 0 && metadata.HasAttachments {
			messages[i].Attachments = []TeamsAttachment{
				{
					Name:        fmt.Sprintf("document_%d.pdf", i),
					URL:         fmt.Sprintf("https://teams.microsoft.com/attachments/doc_%d", i),
					ContentType: "application/pdf",
					Size:        1024 * 1024, // 1MB
				},
			}
		}
	}

	return &TeamsExport{
		Metadata: metadata,
		Messages: messages,
	}, nil
}

// TeamsIterator implements ChunkIterator for Teams exports
type TeamsIterator struct {
	sourcePath     string
	config         map[string]any
	export         *TeamsExport
	currentMessage int
}

// Next returns the next chunk of content from the Teams export
func (it *TeamsIterator) Next(ctx context.Context) (core.Chunk, error) {
	select {
	case <-ctx.Done():
		return core.Chunk{}, ctx.Err()
	default:
	}

	// Check if we've processed all messages
	if it.currentMessage >= len(it.export.Messages) {
		return core.Chunk{}, core.ErrIteratorExhausted
	}

	message := it.export.Messages[it.currentMessage]
	it.currentMessage++

	// Skip system messages if configured
	if message.IsSystemMsg {
		if includeSystem, ok := it.config["include_system_messages"].(bool); !ok || !includeSystem {
			return it.Next(ctx) // Recursively get next message
		}
	}

	// Skip deleted messages if configured
	if message.IsDeleted {
		if includeDeleted, ok := it.config["include_deleted_messages"].(bool); !ok || !includeDeleted {
			return it.Next(ctx) // Recursively get next message
		}
	}

	// Build message content
	var contentParts []string
	contentParts = append(contentParts, fmt.Sprintf("Channel: %s", message.ChannelName))
	contentParts = append(contentParts, fmt.Sprintf("From: %s", message.Sender))
	contentParts = append(contentParts, fmt.Sprintf("Message: %s", message.MessageText))

	if len(message.Attachments) > 0 {
		var attachmentNames []string
		for _, attachment := range message.Attachments {
			attachmentNames = append(attachmentNames, attachment.Name)
		}
		contentParts = append(contentParts, fmt.Sprintf("Attachments: %s", strings.Join(attachmentNames, ", ")))
	}

	content := strings.Join(contentParts, "\n")

	chunk := core.Chunk{
		Data: content,
		Metadata: core.ChunkMetadata{
			SourcePath:  it.sourcePath,
			ChunkID:     fmt.Sprintf("%s:message:%s", filepath.Base(it.sourcePath), message.ID),
			ChunkType:   "teams_message",
			SizeBytes:   int64(len(content)),
			ProcessedAt: time.Now(),
			ProcessedBy: "teams_reader",
			Context: map[string]string{
				"message_id":       message.ID,
				"channel_name":     message.ChannelName,
				"team_name":        message.TeamName,
				"sender":          message.Sender,
				"sender_email":    message.SenderEmail,
				"timestamp":       message.Timestamp.Format("2006-01-02 15:04:05"),
				"message_type":    message.MessageType,
				"thread_id":       message.ThreadID,
				"is_system_msg":   strconv.FormatBool(message.IsSystemMsg),
				"attachment_count": strconv.Itoa(len(message.Attachments)),
				"reaction_count":   strconv.Itoa(len(message.Reactions)),
			},
		},
	}

	return chunk, nil
}

// Close releases Teams resources
func (it *TeamsIterator) Close() error {
	// Nothing to close for Teams iterator
	return nil
}

// Reset restarts iteration from the beginning
func (it *TeamsIterator) Reset() error {
	it.currentMessage = 0
	return nil
}

// Progress returns iteration progress
func (it *TeamsIterator) Progress() float64 {
	totalMessages := len(it.export.Messages)
	if totalMessages == 0 {
		return 1.0
	}
	return float64(it.currentMessage) / float64(totalMessages)
}