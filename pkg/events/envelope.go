package events

import (
	"time"

	"github.com/google/uuid"
)

// EnvelopeSchemaVersion is the wire-format version consumers use to decide
// whether they can parse a message. Increment only on breaking changes.
const EnvelopeSchemaVersion = "1"

// ActivityEventType discriminates payload shapes within the shared envelope.
type ActivityEventType string

const (
	ActivityDocumentUploaded  ActivityEventType = "document.uploaded"
	ActivityDocumentProcessed ActivityEventType = "document.processed"
	ActivityDocumentFailed    ActivityEventType = "document.failed"
)

// ceTypeMap maps legacy event types to CloudEvents type strings.
var ceTypeMap = map[ActivityEventType]string{
	ActivityDocumentUploaded:  "com.tas.activity.document.uploaded",
	ActivityDocumentProcessed: "com.tas.activity.document.processed",
	ActivityDocumentFailed:    "com.tas.activity.document.failed",
}

// CEType returns the CloudEvents 1.0 type string for this event type.
func (t ActivityEventType) CEType() string {
	if s, ok := ceTypeMap[t]; ok {
		return s
	}
	return string(t)
}

// Envelope is the legacy v1 wire format. Retained for dual-publish during the
// CloudEvents migration window. Will be removed once all consumers confirm
// zero legacy messages for 24h.
type Envelope struct {
	SchemaVersion string            `json:"schema_version"`
	EventID       string            `json:"event_id"`
	EventType     ActivityEventType `json:"event_type"`
	Timestamp     time.Time         `json:"timestamp"`

	TenantID  string `json:"tenant_id,omitempty"`
	SpaceID   string `json:"space_id,omitempty"`
	UserID    string `json:"user_id,omitempty"`
	RequestID string `json:"request_id,omitempty"`

	SourceService string `json:"source_service"`

	Payload any `json:"payload"`
}

// DocumentUploadedPayload is carried by ActivityDocumentUploaded events.
type DocumentUploadedPayload struct {
	FileID    string `json:"file_id"`
	FileName  string `json:"file_name"`
	SizeBytes int64  `json:"size_bytes"`
	MimeType  string `json:"mime_type,omitempty"`
	Source    string `json:"source,omitempty"` // e.g. "upload", "sharepoint", "s3"
}

// DocumentProcessedPayload is carried by ActivityDocumentProcessed events.
// Confidence is the pipeline's quality score for the extracted content, in
// the range [0.0, 1.0] (from ProcessingResult.QualityScore). The Live Streams
// panel surfaces this as the document's analysis confidence.
type DocumentProcessedPayload struct {
	FileID     string  `json:"file_id"`
	FileName   string  `json:"file_name,omitempty"`
	ChunkCount int     `json:"chunk_count,omitempty"`
	DurationMS int64   `json:"duration_ms,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
}

// DocumentFailedPayload is carried by ActivityDocumentFailed events.
type DocumentFailedPayload struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name,omitempty"`
	Stage    string `json:"stage,omitempty"` // e.g. "extract", "embed", "dlp"
	Error    string `json:"error"`
}

// NewActivityEnvelope builds a legacy Envelope v1 with a caller-supplied ID.
func NewActivityEnvelope(eventType ActivityEventType, tenantID, userID, requestID string, payload any) Envelope {
	return NewActivityEnvelopeWithID(uuid.NewString(), eventType, tenantID, userID, requestID, payload)
}

// NewActivityEnvelopeWithID builds a legacy Envelope using a specific event ID.
// Used by dual-publish to share the same ID across legacy and CE messages.
func NewActivityEnvelopeWithID(id string, eventType ActivityEventType, tenantID, userID, requestID string, payload any) Envelope {
	return Envelope{
		SchemaVersion: EnvelopeSchemaVersion,
		EventID:       id,
		EventType:     eventType,
		Timestamp:     time.Now().UTC(),
		TenantID:      tenantID,
		UserID:        userID,
		RequestID:     requestID,
		SourceService: "audimodal",
		Payload:       payload,
	}
}
