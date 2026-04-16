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

// TestDualPublish_LegacyAndCE verifies that each publish call emits both a
// legacy envelope and a CloudEvents 1.0 message with the same event ID.
func TestDualPublish_LegacyAndCE(t *testing.T) {
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
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages (legacy + CE), got %d", len(msgs))
	}

	// First message: legacy envelope
	legacyMsg := msgs[0]
	var env Envelope
	if err := json.Unmarshal(legacyMsg.Value, &env); err != nil {
		t.Fatalf("unmarshal legacy: %v", err)
	}
	if env.SchemaVersion != "1" {
		t.Errorf("legacy SchemaVersion = %q, want 1", env.SchemaVersion)
	}
	if env.EventType != ActivityDocumentUploaded {
		t.Errorf("legacy EventType = %q, want %q", env.EventType, ActivityDocumentUploaded)
	}
	if env.SourceService != "audimodal" {
		t.Errorf("legacy SourceService = %q", env.SourceService)
	}
	legacyID := env.EventID

	// Second message: CloudEvents
	ceMsg := msgs[1]
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

	// Stable event ID across both formats
	if ce.ID != legacyID {
		t.Errorf("event IDs differ: legacy=%q, CE=%q — must be identical for dedup", legacyID, ce.ID)
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

// TestDualPublish_AllEventTypes verifies 3 helper methods each produce 2 messages.
func TestDualPublish_AllEventTypes(t *testing.T) {
	w := &fakeWriter{}
	p := NewActivityPublisherWithWriter(w, ActivityTopic, log.Default())
	ctx := context.Background()

	p.PublishDocumentUploaded(ctx, "t", "u", "r", DocumentUploadedPayload{FileID: "f"})
	p.PublishDocumentProcessed(ctx, "t", "u", "r", DocumentProcessedPayload{FileID: "f"})
	p.PublishDocumentFailed(ctx, "t", "u", "r", DocumentFailedPayload{FileID: "f", Error: "boom"})

	msgs := w.snapshot()
	if len(msgs) != 6 {
		t.Fatalf("expected 6 messages (3 events × 2 formats), got %d", len(msgs))
	}

	// Verify CE types for the even-indexed messages (1, 3, 5)
	wantCETypes := []string{
		"com.tas.activity.document.uploaded",
		"com.tas.activity.document.processed",
		"com.tas.activity.document.failed",
	}
	for i, wantType := range wantCETypes {
		ceIdx := i*2 + 1
		var ce tasevents.Event
		if err := json.Unmarshal(msgs[ceIdx].Value, &ce); err != nil {
			t.Fatalf("CE msg %d unmarshal: %v", ceIdx, err)
		}
		if ce.Type != wantType {
			t.Errorf("CE msg %d type = %q, want %q", ceIdx, ce.Type, wantType)
		}
	}
}

// --- helpers ---

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }
