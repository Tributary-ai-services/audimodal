package logger

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

// HTTPLogger wraps an HTTP handler with structured logging
type HTTPLogger struct {
	logger *Logger
	config *HTTPLogConfig
}

// HTTPLogConfig configures HTTP logging behavior
type HTTPLogConfig struct {
	// SkipPaths contains paths to skip logging (e.g., health checks)
	SkipPaths []string
	// LogRequestBody enables logging of request bodies
	LogRequestBody bool
	// LogResponseBody enables logging of response bodies
	LogResponseBody bool
	// LogHeaders enables logging of request/response headers
	LogHeaders bool
	// SanitizeHeaders contains headers to sanitize (remove sensitive values)
	SanitizeHeaders []string
	// MaxBodySize limits the size of logged request/response bodies
	MaxBodySize int
}

// responseWriter wraps http.ResponseWriter to capture response data
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int
	body       []byte
	config     *HTTPLogConfig
}

// NewHTTPLogger creates a new HTTP logger middleware
func NewHTTPLogger(logger *Logger, config *HTTPLogConfig) *HTTPLogger {
	if config == nil {
		config = &HTTPLogConfig{
			SkipPaths: []string{"/health", "/metrics"},
			SanitizeHeaders: []string{
				"authorization", "x-api-key", "cookie", "set-cookie",
				"x-auth-token", "x-csrf-token", "jwt", "bearer",
			},
			MaxBodySize: 1024, // 1KB
		}
	}

	return &HTTPLogger{
		logger: logger,
		config: config,
	}
}

// Middleware returns the HTTP middleware function
func (hl *HTTPLogger) Middleware() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Skip logging for certain paths
			if hl.shouldSkip(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			// Extract request ID from context or headers
			requestID := hl.getRequestID(r)

			// Create context with request ID
			ctx := context.WithValue(r.Context(), "request_id", requestID)
			r = r.WithContext(ctx)

			// Create wrapped response writer
			rw := &responseWriter{
				ResponseWriter: w,
				statusCode:     200,
				config:         hl.config,
			}

			// Log request start
			reqLogger := hl.logger.WithContext(ctx).WithFields(map[string]interface{}{
				"http_method":      r.Method,
				"http_url":         r.URL.String(),
				"http_path":        r.URL.Path,
				"http_query":       r.URL.RawQuery,
				"http_user_agent":  r.UserAgent(),
				"http_remote_addr": hl.getClientIP(r),
				"http_host":        r.Host,
				"http_proto":       r.Proto,
				"content_length":   r.ContentLength,
			})

			// Add headers if configured
			if hl.config.LogHeaders {
				headers := hl.sanitizeHeaders(r.Header)
				if len(headers) > 0 {
					reqLogger = reqLogger.WithField("http_headers", headers)
				}
			}

			// Log request body if configured
			if hl.config.LogRequestBody && r.ContentLength > 0 {
				if body := hl.readRequestBody(r); body != "" {
					reqLogger = reqLogger.WithField("http_request_body", body)
				}
			}

			reqLogger.Info("HTTP request started")

			// Handle panics
			defer func() {
				if err := recover(); err != nil {
					stack := debug.Stack()
					reqLogger.WithFields(map[string]interface{}{
						"panic":       err,
						"stack_trace": string(stack),
					}).Error("HTTP request panicked")

					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
			}()

			// Process request
			next.ServeHTTP(rw, r)

			// Calculate duration
			duration := time.Since(start)

			// Log response
			resLogger := reqLogger.WithFields(map[string]interface{}{
				"http_status":        rw.statusCode,
				"http_response_size": rw.size,
				"duration_ms":        float64(duration.Nanoseconds()) / 1e6,
				"duration":           duration.String(),
			})

			// Add response body if configured
			if hl.config.LogResponseBody && len(rw.body) > 0 {
				resLogger = resLogger.WithField("http_response_body", string(rw.body))
			}

			// Log at appropriate level based on status code
			message := fmt.Sprintf("HTTP %s %s completed", r.Method, r.URL.Path)
			switch {
			case rw.statusCode >= 500:
				resLogger.Error(message)
			case rw.statusCode >= 400:
				resLogger.Warn(message)
			default:
				resLogger.Info(message)
			}
		})
	}
}

// shouldSkip determines if a path should be skipped from logging
func (hl *HTTPLogger) shouldSkip(path string) bool {
	for _, skipPath := range hl.config.SkipPaths {
		if path == skipPath || strings.HasPrefix(path, skipPath) {
			return true
		}
	}
	return false
}

// getRequestID extracts or generates a request ID
func (hl *HTTPLogger) getRequestID(r *http.Request) string {
	// Try various header names for request ID
	headers := []string{
		"X-Request-ID",
		"X-Request-Id",
		"X-Correlation-ID",
		"X-Correlation-Id",
		"Request-ID",
		"Request-Id",
	}

	for _, header := range headers {
		if id := r.Header.Get(header); id != "" {
			return id
		}
	}

	// Generate a simple request ID if none found
	return fmt.Sprintf("req_%d", time.Now().UnixNano())
}

// getClientIP extracts the real client IP address
func (hl *HTTPLogger) getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the chain
		if ips := strings.Split(xff, ","); len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Check X-Forwarded header
	if xf := r.Header.Get("X-Forwarded"); xf != "" {
		return xf
	}

	// Fall back to RemoteAddr
	if ip, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return ip
	}

	return r.RemoteAddr
}

// sanitizeHeaders removes sensitive information from headers
func (hl *HTTPLogger) sanitizeHeaders(headers http.Header) map[string]interface{} {
	sanitized := make(map[string]interface{})

	for name, values := range headers {
		lowerName := strings.ToLower(name)

		// Check if this header should be sanitized
		shouldSanitize := false
		for _, sensitive := range hl.config.SanitizeHeaders {
			if strings.Contains(lowerName, strings.ToLower(sensitive)) {
				shouldSanitize = true
				break
			}
		}

		if shouldSanitize {
			sanitized[name] = "[REDACTED]"
		} else {
			if len(values) == 1 {
				sanitized[name] = values[0]
			} else {
				sanitized[name] = values
			}
		}
	}

	return sanitized
}

// readRequestBody safely reads and limits request body
func (hl *HTTPLogger) readRequestBody(r *http.Request) string {
	if r.Body == nil || hl.config.MaxBodySize <= 0 {
		return ""
	}

	// This is a simplified implementation
	// In production, you'd want to use a TeeReader to preserve the body
	return "[BODY_LOGGING_NOT_IMPLEMENTED]"
}

// Write captures response data
func (rw *responseWriter) Write(b []byte) (int, error) {
	// Capture response body if configured
	if rw.config.LogResponseBody && len(rw.body)+len(b) <= rw.config.MaxBodySize {
		rw.body = append(rw.body, b...)
	}

	size, err := rw.ResponseWriter.Write(b)
	rw.size += size
	return size, err
}

// WriteHeader captures status code
func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.statusCode = statusCode
	rw.ResponseWriter.WriteHeader(statusCode)
}

// Hijack implements http.Hijacker interface
func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, fmt.Errorf("ResponseWriter does not implement http.Hijacker")
}

// Flush implements http.Flusher interface
func (rw *responseWriter) Flush() {
	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Push implements http.Pusher interface
func (rw *responseWriter) Push(target string, opts *http.PushOptions) error {
	if pusher, ok := rw.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, opts)
	}
	return fmt.Errorf("ResponseWriter does not implement http.Pusher")
}

// RequestLoggingMiddleware is a convenience function to create HTTP logging middleware
func RequestLoggingMiddleware(logger *Logger, config *HTTPLogConfig) func(next http.Handler) http.Handler {
	httpLogger := NewHTTPLogger(logger, config)
	return httpLogger.Middleware()
}
