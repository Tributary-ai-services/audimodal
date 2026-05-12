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

// TestPublish_CloudEventOnly verifies that each publish call emits a single
// CloudEvents 1.0 message. The legacy Envelope v1 mirror was retired in
// Phase 4 once aether-be reported zero legacy deliveries for 24h.
func TestPublish_CloudEventOnly(t *testing.T) {
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
		t.Fatalf("expected 1 CE message, got %d", len(msgs))
	}

	ceMsg := msgs[0]
	if !kafkabind.IsCloudEvent(ceMsg.Headers) {
		t.Error("CE message missing content-type header")
	}

	var ce tasevents.Event
	if err := json.Unmarshal(ceMsg.Value, &ce); err != nil {
		t.Fatalf("unmarshal CE: %v", err)
	}
	if ce.SpecVersion != "1.0" {
		t.Errorf("CE specversion = %q, want 1.0", ce.SpecVersion)
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
	if ce.ID == "" {
		t.Error("CE id is empty")
	}
}

// TestDualPublish_DoesNotBlockOnFailure verifies that a failing Kafka
// write is swallowed — activity events are best-effort.
func TestDualPublish_DoesNotBlockOnFailure(t *testing.T) {
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

// TestDualPublish_NilReceiverSafe verifies a nil publisher is a silent no-op.
func TestDualPublish_NilReceiverSafe(t *testing.T) {
	var p *ActivityPublisher
	p.PublishDocumentFailed(context.Background(), "t", "u", "r", DocumentFailedPayload{Error: "x"})
	if err := p.Close(); err != nil {
		t.Errorf("Close on nil publisher: %v", err)
	}
}

// TestPublish_AllEventTypes verifies the three helper methods each emit one
// CloudEvents message of the right type.
func TestPublish_AllEventTypes(t *testing.T) {
	w := &fakeWriter{}
	p := NewActivityPublisherWithWriter(w, ActivityTopic, log.Default())
	ctx := context.Background()

	p.PublishDocumentUploaded(ctx, "t", "u", "r", DocumentUploadedPayload{FileID: "f"})
	p.PublishDocumentProcessed(ctx, "t", "u", "r", DocumentProcessedPayload{FileID: "f"})
	p.PublishDocumentFailed(ctx, "t", "u", "r", DocumentFailedPayload{FileID: "f", Error: "boom"})

	msgs := w.snapshot()
	if len(msgs) != 3 {
		t.Fatalf("expected 3 CE messages, got %d", len(msgs))
	}

	wantCETypes := []string{
		"com.tas.activity.document.uploaded",
		"com.tas.activity.document.processed",
		"com.tas.activity.document.failed",
	}
	for i, wantType := range wantCETypes {
		var ce tasevents.Event
		if err := json.Unmarshal(msgs[i].Value, &ce); err != nil {
			t.Fatalf("CE msg %d unmarshal: %v", i, err)
		}
		if ce.Type != wantType {
			t.Errorf("CE msg %d type = %q, want %q", i, ce.Type, wantType)
		}
	}
}

// --- helpers ---

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }
