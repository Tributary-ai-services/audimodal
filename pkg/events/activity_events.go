package events

// ActivityEventType names a document activity event. Each maps to a
// CloudEvents 1.0 type string via CEType.
type ActivityEventType string

const (
	ActivityDocumentUploaded  ActivityEventType = "document.uploaded"
	ActivityDocumentProcessed ActivityEventType = "document.processed"
	ActivityDocumentFailed    ActivityEventType = "document.failed"
)

// ceTypeMap maps each activity event to its CloudEvents type string.
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

// DocumentUploadedPayload is carried by ActivityDocumentUploaded events.
type DocumentUploadedPayload struct {
	FileID    string `json:"file_id"`
	FileName  string `json:"file_name"`
	SizeBytes int64  `json:"size_bytes"`
	MimeType  string `json:"mime_type,omitempty"`
	Source    string `json:"source,omitempty"` // e.g. "upload", "sharepoint", "s3"
}

// DocumentProcessedPayload is carried by ActivityDocumentProcessed events.
//
// Confidence and OCRConfidence are deliberately separate fields. They measure
// different things and are not comparable, so they must never share one:
//
//   - Confidence is the pipeline's quality score for the extracted content
//     (ProcessingResult.QualityScore), in [0.0, 1.0]. The Live Streams panel
//     shows it as the document's analysis confidence.
//   - OCRConfidence is the mean OCR word confidence across the document's pages,
//     in [0.0, 1.0]. Pages extracted as text rather than OCR report 1.0, so on a
//     text PDF it says nothing about content quality.
//
// The assembler has only OCR confidence, so it sets OCRConfidence and leaves
// Confidence empty. Putting the OCR figure in Confidence made an ordinary text
// PDF read as "100% confidence" in the UI.
type DocumentProcessedPayload struct {
	FileID        string  `json:"file_id"`
	FileName      string  `json:"file_name,omitempty"`
	ChunkCount    int     `json:"chunk_count,omitempty"`
	DurationMS    int64   `json:"duration_ms,omitempty"`
	Confidence    float64 `json:"confidence,omitempty"`
	OCRConfidence float64 `json:"ocr_confidence,omitempty"`
}

// DocumentFailedPayload is carried by ActivityDocumentFailed events.
type DocumentFailedPayload struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name,omitempty"`
	Stage    string `json:"stage,omitempty"` // e.g. "extract", "embed", "dlp"
	Error    string `json:"error"`
}
