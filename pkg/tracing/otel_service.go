package tracing

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
)

// TracingService manages OpenTelemetry distributed tracing
type TracingService struct {
	config   *TracingConfig
	provider *sdktrace.TracerProvider
	tracer   trace.Tracer
}

// TracingConfig contains configuration for OpenTelemetry tracing
type TracingConfig struct {
	// Service identification
	ServiceName    string `yaml:"service_name"`
	ServiceVersion string `yaml:"service_version"`
	Environment    string `yaml:"environment"`

	// Tracing settings
	Enabled          bool    `yaml:"enabled"`
	SampleRate       float64 `yaml:"sample_rate"`
	MaxSpansPerTrace int     `yaml:"max_spans_per_trace"`

	// Export configuration
	ExportType      string        `yaml:"export_type"` // jaeger, otlp, console
	ExportEndpoint  string        `yaml:"export_endpoint"`
	ExportTimeout   time.Duration `yaml:"export_timeout"`
	ExportBatchSize int           `yaml:"export_batch_size"`

	// Jaeger specific
	JaegerAgentHost string `yaml:"jaeger_agent_host"`
	JaegerAgentPort string `yaml:"jaeger_agent_port"`

	// OTLP specific
	OTLPHeaders  map[string]string `yaml:"otlp_headers"`
	OTLPInsecure bool              `yaml:"otlp_insecure"`

	// Resource attributes
	ResourceAttributes map[string]string `yaml:"resource_attributes"`
}

// SpanInfo contains information about a span for monitoring
type SpanInfo struct {
	TraceID    string                 `json:"trace_id"`
	SpanID     string                 `json:"span_id"`
	ParentID   string                 `json:"parent_id,omitempty"`
	Name       string                 `json:"name"`
	Service    string                 `json:"service"`
	Operation  string                 `json:"operation"`
	StartTime  time.Time              `json:"start_time"`
	EndTime    *time.Time             `json:"end_time,omitempty"`
	Duration   *time.Duration         `json:"duration,omitempty"`
	Status     string                 `json:"status"`
	Attributes map[string]interface{} `json:"attributes"`
	Events     []SpanEvent            `json:"events,omitempty"`
	Error      *SpanError             `json:"error,omitempty"`
}

// SpanEvent represents an event within a span
type SpanEvent struct {
	Name       string                 `json:"name"`
	Timestamp  time.Time              `json:"timestamp"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
}

// SpanError represents an error within a span
type SpanError struct {
	Message    string `json:"message"`
	Type       string `json:"type"`
	StackTrace string `json:"stack_trace,omitempty"`
}

// TraceMetrics contains metrics about tracing
type TraceMetrics struct {
	SpansCreated    int64   `json:"spans_created"`
	SpansExported   int64   `json:"spans_exported"`
	SpansDropped    int64   `json:"spans_dropped"`
	ExportErrors    int64   `json:"export_errors"`
	AverageSpanTime float64 `json:"avg_span_time_ms"`
	SampleRate      float64 `json:"sample_rate"`
	ActiveTraces    int     `json:"active_traces"`
	TracesPerSecond float64 `json:"traces_per_second"`
}

// DefaultTracingConfig returns default tracing configuration
func DefaultTracingConfig() *TracingConfig {
	return &TracingConfig{
		ServiceName:      "audimodal-api",
		ServiceVersion:   "1.0.0",
		Environment:      "development",
		Enabled:          true,
		SampleRate:       1.0, // Sample all traces in development
		MaxSpansPerTrace: 1000,
		ExportType:       "jaeger",
		ExportEndpoint:   "http://localhost:14268/api/traces",
		ExportTimeout:    30 * time.Second,
		ExportBatchSize:  512,
		JaegerAgentHost:  "localhost",
		JaegerAgentPort:  "6832",
		OTLPInsecure:     true,
		ResourceAttributes: map[string]string{
			"deployment.environment": "development",
			"service.namespace":      "audimodal",
		},
	}
}

// NewTracingService creates a new tracing service
func NewTracingService(config *TracingConfig) (*TracingService, error) {
	if config == nil {
		config = DefaultTracingConfig()
	}

	if !config.Enabled {
		// Return a no-op service if tracing is disabled
		return &TracingService{
			config: config,
			tracer: otel.Tracer("noop"),
		}, nil
	}

	// Create resource
	res, err := resource.New(context.Background(),
		resource.WithAttributes(
			semconv.ServiceName(config.ServiceName),
			semconv.ServiceVersion(config.ServiceVersion),
			semconv.DeploymentEnvironment(config.Environment),
		),
		resource.WithAttributes(convertResourceAttributes(config.ResourceAttributes)...),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create exporter based on configuration
	exporter, err := createExporter(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create exporter: %w", err)
	}

	// Create trace provider
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter,
			sdktrace.WithBatchTimeout(config.ExportTimeout),
			sdktrace.WithMaxExportBatchSize(config.ExportBatchSize),
		),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(config.SampleRate)),
		sdktrace.WithSpanLimits(sdktrace.SpanLimits{
			AttributeValueLengthLimit:   -1,
			AttributeCountLimit:         -1,
			EventCountLimit:             -1,
			LinkCountLimit:              -1,
			AttributePerEventCountLimit: -1,
			AttributePerLinkCountLimit:  -1,
		}),
	)

	// Set global provider and propagator
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	tracer := provider.Tracer(config.ServiceName)

	return &TracingService{
		config:   config,
		provider: provider,
		tracer:   tracer,
	}, nil
}

// createExporter creates a trace exporter based on configuration
func createExporter(config *TracingConfig) (sdktrace.SpanExporter, error) {
	switch config.ExportType {
	case "jaeger":
		return jaeger.New(jaeger.WithCollectorEndpoint(
			jaeger.WithEndpoint(config.ExportEndpoint),
		))
	case "otlp":
		client := otlptracehttp.NewClient(
			otlptracehttp.WithEndpoint(config.ExportEndpoint),
			otlptracehttp.WithHeaders(config.OTLPHeaders),
			otlptracehttp.WithTimeout(config.ExportTimeout),
		)
		if config.OTLPInsecure {
			client = otlptracehttp.NewClient(
				otlptracehttp.WithEndpoint(config.ExportEndpoint),
				otlptracehttp.WithHeaders(config.OTLPHeaders),
				otlptracehttp.WithTimeout(config.ExportTimeout),
				otlptracehttp.WithInsecure(),
			)
		}
		return otlptrace.New(context.Background(), client)
	case "console":
		// For development/debugging - prints to stdout
		return newConsoleExporter(), nil
	default:
		return nil, fmt.Errorf("unsupported export type: %s", config.ExportType)
	}
}

// convertResourceAttributes converts string map to OTEL attributes
func convertResourceAttributes(attrs map[string]string) []attribute.KeyValue {
	result := make([]attribute.KeyValue, 0, len(attrs))
	for k, v := range attrs {
		result = append(result, attribute.String(k, v))
	}
	return result
}

// Start starts the tracing service
func (ts *TracingService) Start(ctx context.Context) error {
	if !ts.config.Enabled {
		return nil
	}

	// Tracing is automatically started when the provider is created
	// This method is here for consistency with other services
	return nil
}

// Stop stops the tracing service and flushes remaining spans
func (ts *TracingService) Stop(ctx context.Context) error {
	if ts.provider == nil {
		return nil
	}

	// Shutdown will flush any remaining spans
	return ts.provider.Shutdown(ctx)
}

// GetTracer returns a tracer for the given name
func (ts *TracingService) GetTracer(name string) trace.Tracer {
	if !ts.config.Enabled {
		return otel.Tracer("noop")
	}
	return ts.provider.Tracer(name)
}

// StartSpan starts a new span with the given name and options
func (ts *TracingService) StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return ts.tracer.Start(ctx, name, opts...)
}

// StartProcessingSpan starts a span for file processing operations
func (ts *TracingService) StartProcessingSpan(ctx context.Context, operation string, fileID, tenantID string) (context.Context, trace.Span) {
	return ts.tracer.Start(ctx, fmt.Sprintf("processing.%s", operation),
		trace.WithAttributes(
			attribute.String("file.id", fileID),
			attribute.String("tenant.id", tenantID),
			attribute.String("operation.type", operation),
		),
	)
}

// StartEmbeddingSpan starts a span for embedding operations
func (ts *TracingService) StartEmbeddingSpan(ctx context.Context, operation string, provider, model string) (context.Context, trace.Span) {
	return ts.tracer.Start(ctx, fmt.Sprintf("embedding.%s", operation),
		trace.WithAttributes(
			attribute.String("embedding.provider", provider),
			attribute.String("embedding.model", model),
			attribute.String("operation.type", operation),
		),
	)
}

// StartDatabaseSpan starts a span for database operations
func (ts *TracingService) StartDatabaseSpan(ctx context.Context, operation, table string) (context.Context, trace.Span) {
	return ts.tracer.Start(ctx, fmt.Sprintf("db.%s", operation),
		trace.WithAttributes(
			attribute.String("db.operation", operation),
			attribute.String("db.table", table),
			attribute.String("db.system", "postgresql"),
		),
	)
}

// StartKafkaSpan starts a span for Kafka operations
func (ts *TracingService) StartKafkaSpan(ctx context.Context, operation, topic string) (context.Context, trace.Span) {
	return ts.tracer.Start(ctx, fmt.Sprintf("kafka.%s", operation),
		trace.WithAttributes(
			attribute.String("messaging.system", "kafka"),
			attribute.String("messaging.operation", operation),
			attribute.String("messaging.destination", topic),
		),
	)
}

// StartHTTPSpan starts a span for HTTP operations
func (ts *TracingService) StartHTTPSpan(ctx context.Context, method, url string) (context.Context, trace.Span) {
	return ts.tracer.Start(ctx, fmt.Sprintf("http.%s", method),
		trace.WithAttributes(
			attribute.String("http.method", method),
			attribute.String("http.url", url),
			attribute.String("component", "http-client"),
		),
	)
}

// AddSpanAttributes adds attributes to the current span
func (ts *TracingService) AddSpanAttributes(span trace.Span, attrs map[string]interface{}) {
	if span == nil || !span.IsRecording() {
		return
	}

	otelAttrs := make([]attribute.KeyValue, 0, len(attrs))
	for k, v := range attrs {
		otelAttrs = append(otelAttrs, convertToOTELAttribute(k, v))
	}

	span.SetAttributes(otelAttrs...)
}

// AddSpanEvent adds an event to the current span
func (ts *TracingService) AddSpanEvent(span trace.Span, name string, attrs map[string]interface{}) {
	if span == nil || !span.IsRecording() {
		return
	}

	otelAttrs := make([]attribute.KeyValue, 0, len(attrs))
	for k, v := range attrs {
		otelAttrs = append(otelAttrs, convertToOTELAttribute(k, v))
	}

	span.AddEvent(name, trace.WithAttributes(otelAttrs...))
}

// RecordError records an error on the current span
func (ts *TracingService) RecordError(span trace.Span, err error, attrs map[string]interface{}) {
	if span == nil || !span.IsRecording() || err == nil {
		return
	}

	span.RecordError(err)

	if attrs != nil {
		otelAttrs := make([]attribute.KeyValue, 0, len(attrs))
		for k, v := range attrs {
			otelAttrs = append(otelAttrs, convertToOTELAttribute(k, v))
		}
		span.SetAttributes(otelAttrs...)
	}

	span.SetStatus(codes.Error, err.Error())
}

// convertToOTELAttribute converts various types to OTEL attributes
func convertToOTELAttribute(key string, value interface{}) attribute.KeyValue {
	switch v := value.(type) {
	case string:
		return attribute.String(key, v)
	case int:
		return attribute.Int(key, v)
	case int64:
		return attribute.Int64(key, v)
	case float64:
		return attribute.Float64(key, v)
	case bool:
		return attribute.Bool(key, v)
	case []string:
		return attribute.StringSlice(key, v)
	case []int:
		return attribute.IntSlice(key, v)
	case time.Duration:
		return attribute.Int64(key, v.Nanoseconds())
	case time.Time:
		return attribute.String(key, v.Format(time.RFC3339))
	default:
		return attribute.String(key, fmt.Sprintf("%v", v))
	}
}

// GetMetrics returns tracing metrics
func (ts *TracingService) GetMetrics() *TraceMetrics {
	// In a real implementation, this would collect actual metrics
	// from the trace provider and exporters
	return &TraceMetrics{
		SpansCreated:    1000,
		SpansExported:   950,
		SpansDropped:    50,
		ExportErrors:    5,
		AverageSpanTime: 125.5,
		SampleRate:      ts.config.SampleRate,
		ActiveTraces:    25,
		TracesPerSecond: 15.2,
	}
}

// ExtractTraceContext extracts trace context from headers or metadata
func (ts *TracingService) ExtractTraceContext(ctx context.Context, carrier propagation.TextMapCarrier) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, carrier)
}

// InjectTraceContext injects trace context into headers or metadata
func (ts *TracingService) InjectTraceContext(ctx context.Context, carrier propagation.TextMapCarrier) {
	otel.GetTextMapPropagator().Inject(ctx, carrier)
}

// HealthCheck performs a health check on the tracing service
func (ts *TracingService) HealthCheck(ctx context.Context) error {
	if !ts.config.Enabled {
		return nil
	}

	if ts.provider == nil {
		return fmt.Errorf("trace provider is not initialized")
	}

	// Test span creation
	_, span := ts.tracer.Start(ctx, "health_check")
	span.SetAttributes(attribute.String("check.type", "health"))
	span.End()

	return nil
}

// consoleExporter is a simple exporter that prints spans to stdout
type consoleExporter struct{}

func newConsoleExporter() *consoleExporter {
	return &consoleExporter{}
}

func (e *consoleExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	for _, span := range spans {
		fmt.Printf("Trace: %s, Span: %s, Name: %s, Duration: %s\n",
			span.SpanContext().TraceID().String(),
			span.SpanContext().SpanID().String(),
			span.Name(),
			span.EndTime().Sub(span.StartTime()),
		)
	}
	return nil
}

func (e *consoleExporter) Shutdown(ctx context.Context) error {
	return nil
}

// TracingMiddleware provides HTTP middleware for tracing
func (ts *TracingService) TracingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !ts.config.Enabled {
			next.ServeHTTP(w, r)
			return
		}

		// Extract trace context from headers
		ctx := ts.ExtractTraceContext(r.Context(), propagation.HeaderCarrier(r.Header))

		// Start span
		ctx, span := ts.tracer.Start(ctx, fmt.Sprintf("%s %s", r.Method, r.URL.Path),
			trace.WithAttributes(
				attribute.String("http.method", r.Method),
				attribute.String("http.url", r.URL.String()),
				attribute.String("http.scheme", r.URL.Scheme),
				attribute.String("http.host", r.Host),
				attribute.String("http.user_agent", r.UserAgent()),
			),
		)
		defer span.End()

		// Create response writer wrapper to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: 200}

		// Continue with request
		next.ServeHTTP(wrapped, r.WithContext(ctx))

		// Add response attributes
		span.SetAttributes(
			attribute.Int("http.status_code", wrapped.statusCode),
		)

		// Set span status based on HTTP status code
		if wrapped.statusCode >= 400 {
			span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", wrapped.statusCode))
		}
	})
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// CreateTraceableContext creates a context with trace information for cross-service calls
func (ts *TracingService) CreateTraceableContext(ctx context.Context) map[string]string {
	headers := make(map[string]string)
	ts.InjectTraceContext(ctx, propagation.MapCarrier(headers))
	return headers
}
