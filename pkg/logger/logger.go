package logger

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"
)

// LogLevel represents the logging level
type LogLevel int

const (
	// DebugLevel logs debug messages
	DebugLevel LogLevel = iota
	// InfoLevel logs info messages
	InfoLevel
	// WarnLevel logs warning messages
	WarnLevel
	// ErrorLevel logs error messages
	ErrorLevel
	// FatalLevel logs fatal messages and exits
	FatalLevel
)

// String returns string representation of log level
func (l LogLevel) String() string {
	switch l {
	case DebugLevel:
		return "DEBUG"
	case InfoLevel:
		return "INFO"
	case WarnLevel:
		return "WARN"
	case ErrorLevel:
		return "ERROR"
	case FatalLevel:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// ParseLogLevel parses a log level from string
func ParseLogLevel(level string) LogLevel {
	switch strings.ToUpper(level) {
	case "DEBUG":
		return DebugLevel
	case "INFO":
		return InfoLevel
	case "WARN", "WARNING":
		return WarnLevel
	case "ERROR":
		return ErrorLevel
	case "FATAL":
		return FatalLevel
	default:
		return InfoLevel
	}
}

// LogFormat represents the output format
type LogFormat int

const (
	// TextFormat outputs logs in human-readable text format
	TextFormat LogFormat = iota
	// JSONFormat outputs logs in JSON format
	JSONFormat
)

// Logger represents a structured logger
type Logger struct {
	level        LogLevel
	format       LogFormat
	output       io.Writer
	fields       map[string]interface{}
	service      string
	version      string
	enableCaller bool
}

// Config represents logger configuration
type Config struct {
	Level        LogLevel               `yaml:"level" json:"level"`
	Format       LogFormat              `yaml:"format" json:"format"`
	Output       io.Writer              `yaml:"-" json:"-"`
	Service      string                 `yaml:"service" json:"service"`
	Version      string                 `yaml:"version" json:"version"`
	EnableCaller bool                   `yaml:"enable_caller" json:"enable_caller"`
	Fields       map[string]interface{} `yaml:"fields" json:"fields"`
}

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp string                 `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Service   string                 `json:"service,omitempty"`
	Version   string                 `json:"version,omitempty"`
	Caller    string                 `json:"caller,omitempty"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
	RequestID string                 `json:"request_id,omitempty"`
	TenantID  string                 `json:"tenant_id,omitempty"`
	UserID    string                 `json:"user_id,omitempty"`
	TraceID   string                 `json:"trace_id,omitempty"`
	SpanID    string                 `json:"span_id,omitempty"`
}

// NewLogger creates a new structured logger
func NewLogger(config *Config) *Logger {
	if config == nil {
		config = &Config{
			Level:        InfoLevel,
			Format:       JSONFormat,
			Output:       os.Stdout,
			EnableCaller: true,
			Fields:       make(map[string]interface{}),
		}
	}

	if config.Output == nil {
		config.Output = os.Stdout
	}

	if config.Fields == nil {
		config.Fields = make(map[string]interface{})
	}

	return &Logger{
		level:        config.Level,
		format:       config.Format,
		output:       config.Output,
		fields:       config.Fields,
		service:      config.Service,
		version:      config.Version,
		enableCaller: config.EnableCaller,
	}
}

// NewDefaultLogger creates a logger with default configuration
func NewDefaultLogger(service, version string) *Logger {
	return NewLogger(&Config{
		Level:        InfoLevel,
		Format:       JSONFormat,
		Output:       os.Stdout,
		Service:      service,
		Version:      version,
		EnableCaller: true,
		Fields:       make(map[string]interface{}),
	})
}

// WithField creates a new logger with an additional field
func (l *Logger) WithField(key string, value interface{}) *Logger {
	fields := make(map[string]interface{})
	for k, v := range l.fields {
		fields[k] = v
	}
	fields[key] = value

	return &Logger{
		level:        l.level,
		format:       l.format,
		output:       l.output,
		fields:       fields,
		service:      l.service,
		version:      l.version,
		enableCaller: l.enableCaller,
	}
}

// WithFields creates a new logger with additional fields
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	newFields := make(map[string]interface{})
	for k, v := range l.fields {
		newFields[k] = v
	}
	for k, v := range fields {
		newFields[k] = v
	}

	return &Logger{
		level:        l.level,
		format:       l.format,
		output:       l.output,
		fields:       newFields,
		service:      l.service,
		version:      l.version,
		enableCaller: l.enableCaller,
	}
}

// WithContext creates a new logger with context information
func (l *Logger) WithContext(ctx context.Context) *Logger {
	fields := make(map[string]interface{})
	for k, v := range l.fields {
		fields[k] = v
	}

	// Extract common context values
	if requestID := getStringFromContext(ctx, "request_id"); requestID != "" {
		fields["request_id"] = requestID
	}
	if tenantID := getStringFromContext(ctx, "tenant_id"); tenantID != "" {
		fields["tenant_id"] = tenantID
	}
	if userID := getStringFromContext(ctx, "user_id"); userID != "" {
		fields["user_id"] = userID
	}
	if traceID := getStringFromContext(ctx, "trace_id"); traceID != "" {
		fields["trace_id"] = traceID
	}
	if spanID := getStringFromContext(ctx, "span_id"); spanID != "" {
		fields["span_id"] = spanID
	}

	return &Logger{
		level:        l.level,
		format:       l.format,
		output:       l.output,
		fields:       fields,
		service:      l.service,
		version:      l.version,
		enableCaller: l.enableCaller,
	}
}

// Debug logs a debug message
func (l *Logger) Debug(message string, args ...interface{}) {
	l.log(DebugLevel, message, args...)
}

// Info logs an info message
func (l *Logger) Info(message string, args ...interface{}) {
	l.log(InfoLevel, message, args...)
}

// Warn logs a warning message
func (l *Logger) Warn(message string, args ...interface{}) {
	l.log(WarnLevel, message, args...)
}

// Error logs an error message
func (l *Logger) Error(message string, args ...interface{}) {
	l.log(ErrorLevel, message, args...)
}

// Fatal logs a fatal message and exits
func (l *Logger) Fatal(message string, args ...interface{}) {
	l.log(FatalLevel, message, args...)
	os.Exit(1)
}

// log writes a log entry
func (l *Logger) log(level LogLevel, message string, args ...interface{}) {
	// Check if we should log at this level
	if level < l.level {
		return
	}

	// Format message if args provided
	if len(args) > 0 {
		message = fmt.Sprintf(message, args...)
	}

	// Create log entry
	entry := &LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Level:     level.String(),
		Message:   message,
		Service:   l.service,
		Version:   l.version,
		Fields:    make(map[string]interface{}),
	}

	// Add caller information if enabled
	if l.enableCaller {
		if file, line, fn := l.getCaller(); file != "" {
			entry.Caller = fmt.Sprintf("%s:%d:%s", file, line, fn)
		}
	}

	// Copy fields
	for k, v := range l.fields {
		switch k {
		case "request_id":
			if s, ok := v.(string); ok {
				entry.RequestID = s
			}
		case "tenant_id":
			if s, ok := v.(string); ok {
				entry.TenantID = s
			}
		case "user_id":
			if s, ok := v.(string); ok {
				entry.UserID = s
			}
		case "trace_id":
			if s, ok := v.(string); ok {
				entry.TraceID = s
			}
		case "span_id":
			if s, ok := v.(string); ok {
				entry.SpanID = s
			}
		default:
			entry.Fields[k] = v
		}
	}

	// Remove empty fields map
	if len(entry.Fields) == 0 {
		entry.Fields = nil
	}

	// Write log entry
	l.writeEntry(entry)
}

// writeEntry writes a log entry to the output
func (l *Logger) writeEntry(entry *LogEntry) {
	var output string

	switch l.format {
	case JSONFormat:
		data, err := json.Marshal(entry)
		if err != nil {
			// Fallback to simple format if JSON marshaling fails
			output = fmt.Sprintf("%s [%s] %s\n", entry.Timestamp, entry.Level, entry.Message)
		} else {
			output = string(data) + "\n"
		}
	case TextFormat:
		output = l.formatTextEntry(entry)
	default:
		output = fmt.Sprintf("%s [%s] %s\n", entry.Timestamp, entry.Level, entry.Message)
	}

	l.output.Write([]byte(output))
}

// formatTextEntry formats a log entry as human-readable text
func (l *Logger) formatTextEntry(entry *LogEntry) string {
	timestamp := entry.Timestamp
	if t, err := time.Parse(time.RFC3339Nano, entry.Timestamp); err == nil {
		timestamp = t.Format("2006-01-02 15:04:05.000")
	}

	// Build the base log line
	parts := []string{
		timestamp,
		fmt.Sprintf("[%s]", entry.Level),
	}

	// Add service and version if available
	if entry.Service != "" {
		parts = append(parts, fmt.Sprintf("service=%s", entry.Service))
	}
	if entry.Version != "" {
		parts = append(parts, fmt.Sprintf("version=%s", entry.Version))
	}

	// Add context information
	if entry.RequestID != "" {
		parts = append(parts, fmt.Sprintf("request_id=%s", entry.RequestID))
	}
	if entry.TenantID != "" {
		parts = append(parts, fmt.Sprintf("tenant_id=%s", entry.TenantID))
	}
	if entry.UserID != "" {
		parts = append(parts, fmt.Sprintf("user_id=%s", entry.UserID))
	}

	// Add message
	parts = append(parts, entry.Message)

	// Add additional fields
	if entry.Fields != nil && len(entry.Fields) > 0 {
		for k, v := range entry.Fields {
			parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		}
	}

	// Add caller if available
	if entry.Caller != "" {
		parts = append(parts, fmt.Sprintf("caller=%s", entry.Caller))
	}

	return strings.Join(parts, " ") + "\n"
}

// getCaller returns the caller information
func (l *Logger) getCaller() (file string, line int, fn string) {
	// Skip logger internals
	pc, file, line, ok := runtime.Caller(3)
	if !ok {
		return "", 0, ""
	}

	// Get function name
	if f := runtime.FuncForPC(pc); f != nil {
		fn = f.Name()
		// Remove package path, keep only function name
		if lastSlash := strings.LastIndex(fn, "/"); lastSlash >= 0 {
			fn = fn[lastSlash+1:]
		}
		if lastDot := strings.LastIndex(fn, "."); lastDot >= 0 {
			fn = fn[lastDot+1:]
		}
	}

	// Simplify file path
	if lastSlash := strings.LastIndex(file, "/"); lastSlash >= 0 {
		file = file[lastSlash+1:]
	}

	return file, line, fn
}

// getStringFromContext extracts a string value from context
func getStringFromContext(ctx context.Context, key string) string {
	if value := ctx.Value(key); value != nil {
		if s, ok := value.(string); ok {
			return s
		}
	}
	return ""
}

// SetLevel sets the logging level
func (l *Logger) SetLevel(level LogLevel) {
	l.level = level
}

// SetFormat sets the logging format
func (l *Logger) SetFormat(format LogFormat) {
	l.format = format
}

// GetLevel returns the current logging level
func (l *Logger) GetLevel() LogLevel {
	return l.level
}

// IsLevelEnabled returns true if the given level is enabled
func (l *Logger) IsLevelEnabled(level LogLevel) bool {
	return level >= l.level
}

// Global logger instance
var defaultLogger *Logger

// SetDefault sets the default global logger
func SetDefault(logger *Logger) {
	defaultLogger = logger
}

// GetDefault returns the default global logger
func GetDefault() *Logger {
	if defaultLogger == nil {
		defaultLogger = NewDefaultLogger("audimodal", "1.0.0")
	}
	return defaultLogger
}

// Global logging functions using the default logger

// Debug logs a debug message using the default logger
func Debug(message string, args ...interface{}) {
	GetDefault().Debug(message, args...)
}

// Info logs an info message using the default logger
func Info(message string, args ...interface{}) {
	GetDefault().Info(message, args...)
}

// Warn logs a warning message using the default logger
func Warn(message string, args ...interface{}) {
	GetDefault().Warn(message, args...)
}

// Error logs an error message using the default logger
func Error(message string, args ...interface{}) {
	GetDefault().Error(message, args...)
}

// Fatal logs a fatal message using the default logger and exits
func Fatal(message string, args ...interface{}) {
	GetDefault().Fatal(message, args...)
}

// WithField creates a logger with an additional field using the default logger
func WithField(key string, value interface{}) *Logger {
	return GetDefault().WithField(key, value)
}

// WithFields creates a logger with additional fields using the default logger
func WithFields(fields map[string]interface{}) *Logger {
	return GetDefault().WithFields(fields)
}

// WithContext creates a logger with context information using the default logger
func WithContext(ctx context.Context) *Logger {
	return GetDefault().WithContext(ctx)
}

// ConfigureDefault configures the default logger
func ConfigureDefault(config *Config) {
	SetDefault(NewLogger(config))
}
