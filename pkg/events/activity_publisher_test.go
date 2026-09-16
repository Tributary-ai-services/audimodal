package events

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"

	tasevents "github.com/Tributary-ai-services/aether-shared/go-events"
	"github.com/Tributary-ai-services/aether-shared/go-events/kafkabind"
)

// fakeWriter captures messages in-memory for assertions.
type fakeWriter struct {
	mu       sync.Mutex
	messages []kafka.Message
	failWith error
}

func (f *fakeWriter) WriteMessages(ctx context.Context, msgs ...kafka.Message) error {
	if f.failWith != nil {
		return f.failWith
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = append(f.messages, msgs...)
	return nil
}

func (f *fakeWriter) Close() error { return nil }

func (f *fakeWriter) snapshot() []kafka.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]kafka.Message, len(f.messages))
	copy(out, f.messages)
	return out
}

// TestPublish_EmitsOneCloudEvent verifies that each publish call emits exactly
// one CloudEvents 1.0 message and no legacy envelope.
func TestPublish_EmitsOneCloudEvent(t *testing.T) {
	w := &fakeWriter{}
	p := NewActivityPublisherWithWriter(w, ActivityTopic, log.Default())

	p.PublishDocumentUploaded(context.Background(), "tenant-1", "user-1", "req-1", DocumentUploadedPayload{
		FileID:    "file-abc",
		FileName:  "report.pdf",
		SizeBytes: 1024,
		MimeType:  "application/pdf",
		Source:    "upload",
	})

	msgs := w.snapshot()
	if len(msgs) != 1 {
		t.Fatalf("expected exactly 1 message (CloudEvents only), got %d", len(msgs))
	}

	ceMsg := msgs[0]
	if !kafkabind.IsCloudEvent(ceMsg.Headers) {
		t.Error("message missing the CloudEvents content-type header")
	}
	// A legacy envelope carried these headers; none may remain.
	for _, h := range ceMsg.Headers {
		if h.Key == "schema-version" || h.Key == "source-service" {
			t.Errorf("legacy header %q still present", h.Key)
		}
	}

	var ce tasevents.Event
	if err := json.Unmarshal(ceMsg.Value, &ce); err != nil {
		t.Fatalf("unmarshal CE: %v", err)
	}
	if ce.SpecVersion != "1.0" {
		t.Errorf("CE specversion = %q, want 1.0", ce.SpecVersion)
	}
	if ce.ID == "" {
		t.Error("CE id is empty")
	}
	if ce.Type != "com.tas.activity.document.uploaded" {
		t.Errorf("CE type = %q", ce.Type)
	}
	if ce.Source != "urn:tas:service:audimodal" {
		t.Errorf("CE source = %q", ce.Source)
	}
	if ce.TenantID != "tenant-1" {
		t.Errorf("CE tenantid = %q", ce.TenantID)
	}
	if ce.Subject != "file-abc" {
		t.Errorf("CE subject = %q, want file-abc", ce.Subject)
	}
}

// TestPublish_DoesNotBlockOnFailure verifies that a failing Kafka write is
// swallowed — activity events are best-effort.
func TestPublish_DoesNotBlockOnFailure(t *testing.T) {
	w := &fakeWriter{failWith: errors.New("broker down")}
	p := NewActivityPublisherWithWriter(w, ActivityTopic, log.New(nopWriter{}, "", 0))

	done := make(chan struct{})
	go func() {
		p.PublishDocumentProcessed(context.Background(), "tenant-1", "user-1", "req-1", DocumentProcessedPayload{
			FileID:     "file-1",
			ChunkCount: 5,
			DurationMS: 1000,
		})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("PublishDocumentProcessed blocked on writer failure")
	}
}

// TestPublish_NilReceiverSafe verifies a nil publisher is a silent no-op.
func TestPublish_NilReceiverSafe(t *testing.T) {
	var p *ActivityPublisher
	p.PublishDocumentFailed(context.Background(), "t", "u", "r", DocumentFailedPayload{Error: "x"})
	if err := p.Close(); err != nil {
		t.Errorf("Close on nil publisher: %v", err)
	}
}

// TestPublish_AllEventTypes verifies the three helpers each produce one
// CloudEvent of the right type, and that events get distinct ids.
func TestPublish_AllEventTypes(t *testing.T) {
	w := &fakeWriter{}
	p := NewActivityPublisherWithWriter(w, ActivityTopic, log.Default())
	ctx := context.Background()

	p.PublishDocumentUploaded(ctx, "t", "u", "r", DocumentUploadedPayload{FileID: "f"})
	p.PublishDocumentProcessed(ctx, "t", "u", "r", DocumentProcessedPayload{FileID: "f"})
	p.PublishDocumentFailed(ctx, "t", "u", "r", DocumentFailedPayload{FileID: "f", Error: "boom"})

	msgs := w.snapshot()
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages (one CloudEvent per call), got %d", len(msgs))
	}

	wantCETypes := []string{
		"com.tas.activity.document.uploaded",
		"com.tas.activity.document.processed",
		"com.tas.activity.document.failed",
	}
	seen := map[string]bool{}
	for i, wantType := range wantCETypes {
		var ce tasevents.Event
		if err := json.Unmarshal(msgs[i].Value, &ce); err != nil {
			t.Fatalf("CE msg %d unmarshal: %v", i, err)
		}
		if ce.Type != wantType {
			t.Errorf("CE msg %d type = %q, want %q", i, ce.Type, wantType)
		}
		if seen[ce.ID] {
			t.Errorf("CE msg %d reuses event id %q", i, ce.ID)
		}
		seen[ce.ID] = true
	}
}

// TestDocumentProcessedPayload_ConfidenceFieldsStaySeparate pins the fix for
// one wire field carrying two incomparable measures: the pipeline quality
// score travels as "confidence", OCR word confidence as "ocr_confidence", and
// setting one must not populate the other.
func TestDocumentProcessedPayload_ConfidenceFieldsStaySeparate(t *testing.T) {
	decode := func(payload DocumentProcessedPayload) map[string]any {
		t.Helper()
		b, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		return m
	}

	// Assembler shape: OCR confidence only.
	ocrOnly := decode(DocumentProcessedPayload{FileID: "f", OCRConfidence: 1.0})
	if _, ok := ocrOnly["confidence"]; ok {
		t.Error("OCR-only payload must not emit \"confidence\" — the UI would show it as analysis confidence")
	}
	if got := ocrOnly["ocr_confidence"]; got != 1.0 {
		t.Errorf("ocr_confidence = %v, want 1", got)
	}

	// Handler shape: quality score only.
	qualityOnly := decode(DocumentProcessedPayload{FileID: "f", Confidence: 0.82})
	if got := qualityOnly["confidence"]; got != 0.82 {
		t.Errorf("confidence = %v, want 0.82", got)
	}
	if _, ok := qualityOnly["ocr_confidence"]; ok {
		t.Error("quality-only payload must not emit \"ocr_confidence\"")
	}
}

// --- helpers ---

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }
