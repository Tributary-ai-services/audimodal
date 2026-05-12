package events

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"

	tasevents "github.com/Tributary-ai-services/aether-shared/go-events"
	"github.com/Tributary-ai-services/aether-shared/go-events/kafkabind"
)

// ActivityTopic is the Kafka topic audimodal publishes activity events to.
// Consumed by aether-be's streaming consumer to feed the Live Streams page.
const ActivityTopic = "tas.activity.documents"

const ceSource = "urn:tas:service:audimodal"

// kafkaWriter is the narrow Kafka producer surface ActivityPublisher needs.
// Implemented by *kafka.Writer in production and by fakes in tests.
type kafkaWriter interface {
	WriteMessages(ctx context.Context, msgs ...kafka.Message) error
	Close() error
}

// ActivityPublisher fire-and-forget publishes envelope-v1 activity events.
// During the CloudEvents migration it dual-publishes both legacy and CE
// formats with a shared event ID so consumers can deduplicate.
type ActivityPublisher struct {
	writer  kafkaWriter
	topic   string
	logger  *log.Logger
	timeout time.Duration
}

// ActivityPublisherConfig configures the publisher. BootstrapServers is the
// only required field; everything else has sensible defaults.
type ActivityPublisherConfig struct {
	BootstrapServers string
	Topic            string
	Logger           *log.Logger
	WriteTimeout     time.Duration
}

// NewActivityPublisher creates an ActivityPublisher with a pure-Go kafka-go
// writer. Pass Close() a context to flush on shutdown.
func NewActivityPublisher(cfg ActivityPublisherConfig) *ActivityPublisher {
	topic := cfg.Topic
	if topic == "" {
		topic = ActivityTopic
	}
	logger := cfg.Logger
	if logger == nil {
		logger = log.Default()
	}
	timeout := cfg.WriteTimeout
	if timeout == 0 {
		timeout = 2 * time.Second
	}

	writer := &kafka.Writer{
		Addr:         kafka.TCP(cfg.BootstrapServers),
		Topic:        topic,
		Balancer:     &kafka.Hash{},
		BatchTimeout: 10 * time.Millisecond,
		WriteTimeout: timeout,
		RequiredAcks: kafka.RequireOne,
	}

	return &ActivityPublisher{
		writer:  writer,
		topic:   topic,
		logger:  logger,
		timeout: timeout,
	}
}

// NewActivityPublisherWithWriter is for tests: inject a fake writer implementing
// the kafkaWriter interface.
func NewActivityPublisherWithWriter(w kafkaWriter, topic string, logger *log.Logger) *ActivityPublisher {
	if logger == nil {
		logger = log.Default()
	}
	if topic == "" {
		topic = ActivityTopic
	}
	return &ActivityPublisher{
		writer:  w,
		topic:   topic,
		logger:  logger,
		timeout: 2 * time.Second,
	}
}

// PublishDocumentUploaded publishes a document.uploaded envelope.
func (p *ActivityPublisher) PublishDocumentUploaded(ctx context.Context, tenantID, userID, requestID string, payload DocumentUploadedPayload) {
	p.dualPublish(ctx, ActivityDocumentUploaded, tenantID, userID, requestID, payload.FileID, payload)
}

// PublishDocumentProcessed publishes a document.processed envelope.
func (p *ActivityPublisher) PublishDocumentProcessed(ctx context.Context, tenantID, userID, requestID string, payload DocumentProcessedPayload) {
	p.dualPublish(ctx, ActivityDocumentProcessed, tenantID, userID, requestID, payload.FileID, payload)
}

// PublishDocumentFailed publishes a document.failed envelope.
func (p *ActivityPublisher) PublishDocumentFailed(ctx context.Context, tenantID, userID, requestID string, payload DocumentFailedPayload) {
	p.dualPublish(ctx, ActivityDocumentFailed, tenantID, userID, requestID, payload.FileID, payload)
}

// dualPublish sends both legacy and CloudEvents messages with a shared event ID.
//
// Callers typically invoke this from a goroutine spawned out of an HTTP handler.
// In that case the request context is canceled the moment the response is
// flushed, which would race the Kafka write to completion. Detach from any
// request-scoped context and apply our own timeout instead, so the publish
// always gets a fair shot.
func (p *ActivityPublisher) dualPublish(ctx context.Context, eventType ActivityEventType, tenantID, userID, requestID, subject string, payload any) {
	if p == nil || p.writer == nil {
		return
	}

	eventID := uuid.NewString()

	legacyMsg := p.buildLegacy(eventID, eventType, tenantID, userID, requestID, payload)
	ceMsg := p.buildCE(eventID, eventType, tenantID, userID, requestID, subject, payload)

	// Intentionally detach from ctx — see method doc.
	_ = ctx
	writeCtx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()

	var msgs []kafka.Message
	if legacyMsg != nil {
		msgs = append(msgs, *legacyMsg)
	}
	if ceMsg != nil {
		msgs = append(msgs, *ceMsg)
	}

	if len(msgs) == 0 {
		return
	}

	if err := p.writer.WriteMessages(writeCtx, msgs...); err != nil {
		p.logger.Printf("activity_publisher: publish %s failed: %v", eventType, err)
	}
}

func (p *ActivityPublisher) buildLegacy(eventID string, eventType ActivityEventType, tenantID, userID, requestID string, payload any) *kafka.Message {
	env := NewActivityEnvelopeWithID(eventID, eventType, tenantID, userID, requestID, payload)

	data, err := json.Marshal(env)
	if err != nil {
		p.logger.Printf("activity_publisher: marshal legacy %s failed: %v", eventType, err)
		return nil
	}

	return &kafka.Message{
		Key:   []byte(tenantID + ":" + requestID),
		Value: data,
		Headers: []kafka.Header{
			{Key: "schema-version", Value: []byte(env.SchemaVersion)},
			{Key: "event-type", Value: []byte(string(eventType))},
			{Key: "source-service", Value: []byte(env.SourceService)},
		},
		Time: env.Timestamp,
	}
}

func (p *ActivityPublisher) buildCE(eventID string, eventType ActivityEventType, tenantID, userID, requestID, subject string, payload any) *kafka.Message {
	ce := tasevents.NewWithID(eventID, eventType.CEType(), ceSource,
		tasevents.WithTenant(tenantID, ""),
		tasevents.WithUser(userID),
		tasevents.WithRequest(requestID),
		tasevents.WithSubject(subject),
		tasevents.WithSeverity(tasevents.SeverityInfo),
		tasevents.WithData(payload),
	)

	value, headers, err := kafkabind.Encode(ce)
	if err != nil {
		p.logger.Printf("activity_publisher: encode CE %s failed: %v", eventType, err)
		return nil
	}

	return &kafka.Message{
		Key:     kafkabind.MessageKey(ce),
		Value:   value,
		Headers: headers,
		Time:    ce.Time,
	}
}

// Close flushes pending messages.
func (p *ActivityPublisher) Close() error {
	if p == nil || p.writer == nil {
		return nil
	}
	return p.writer.Close()
}
