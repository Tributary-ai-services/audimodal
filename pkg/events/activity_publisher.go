package events

import (
	"context"
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

// ActivityPublisher fire-and-forget publishes document activity events as
// CloudEvents 1.0 (structured mode) to tas.activity.documents.
//
// It used to dual-publish a legacy Envelope v1 message alongside each
// CloudEvent, as a migration window. That window was never needed: aether-be's
// streaming consumer detects the format from the content-type header and
// normalises legacy messages to CloudEvents, so consumers already read either.
// Publishing only CloudEvents halves the message volume and removes a second
// wire format nobody depends on.
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

// PublishDocumentUploaded publishes a com.tas.activity.document.uploaded event.
func (p *ActivityPublisher) PublishDocumentUploaded(ctx context.Context, tenantID, userID, requestID string, payload DocumentUploadedPayload) {
	p.publish(ctx, ActivityDocumentUploaded, tenantID, userID, requestID, payload.FileID, payload)
}

// PublishDocumentProcessed publishes a com.tas.activity.document.processed event.
func (p *ActivityPublisher) PublishDocumentProcessed(ctx context.Context, tenantID, userID, requestID string, payload DocumentProcessedPayload) {
	p.publish(ctx, ActivityDocumentProcessed, tenantID, userID, requestID, payload.FileID, payload)
}

// PublishDocumentFailed publishes a com.tas.activity.document.failed event.
func (p *ActivityPublisher) PublishDocumentFailed(ctx context.Context, tenantID, userID, requestID string, payload DocumentFailedPayload) {
	p.publish(ctx, ActivityDocumentFailed, tenantID, userID, requestID, payload.FileID, payload)
}

// publish sends one CloudEvents message.
//
// Callers typically invoke this from a goroutine spawned out of an HTTP handler.
// In that case the request context is canceled the moment the response is
// flushed, which would race the Kafka write to completion. Detach from any
// request-scoped context and apply our own timeout instead, so the publish
// always gets a fair shot.
func (p *ActivityPublisher) publish(ctx context.Context, eventType ActivityEventType, tenantID, userID, requestID, subject string, payload any) {
	if p == nil || p.writer == nil {
		return
	}

	msg := p.buildCE(uuid.NewString(), eventType, tenantID, userID, requestID, subject, payload)
	if msg == nil {
		return
	}

	// Intentionally detach from ctx — see method doc.
	_ = ctx
	writeCtx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()

	if err := p.writer.WriteMessages(writeCtx, *msg); err != nil {
		p.logger.Printf("activity_publisher: publish %s failed: %v", eventType, err)
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
