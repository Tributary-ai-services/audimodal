package metrics

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// HTTPMetrics collects HTTP-related metrics
type HTTPMetrics struct {
	registry          *MetricsRegistry
	requestsTotal     *Counter
	requestDuration   *Histogram
	requestSize       *Histogram
	responseSize      *Histogram
	activeConnections *Gauge
}

// NewHTTPMetrics creates a new HTTP metrics collector
func NewHTTPMetrics(registry *MetricsRegistry) *HTTPMetrics {
	if registry == nil {
		registry = GetRegistry()
	}

	return &HTTPMetrics{
		registry: registry,
		requestsTotal: registry.NewCounter(
			"http_requests_total",
			"Total number of HTTP requests",
			nil,
		),
		requestDuration: registry.NewHistogram(
			"http_request_duration_seconds",
			"HTTP request duration in seconds",
			[]float64{0.001, 0.01, 0.1, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0},
			nil,
		),
		requestSize: registry.NewHistogram(
			"http_request_size_bytes",
			"HTTP request size in bytes",
			[]float64{100, 1000, 10000, 100000, 1000000, 10000000},
			nil,
		),
		responseSize: registry.NewHistogram(
			"http_response_size_bytes",
			"HTTP response size in bytes",
			[]float64{100, 1000, 10000, 100000, 1000000, 10000000},
			nil,
		),
		activeConnections: registry.NewGauge(
			"http_active_connections",
			"Number of active HTTP connections",
			nil,
		),
	}
}

// responseWriter wraps http.ResponseWriter to capture metrics
type metricsResponseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int
}

// WriteHeader captures the status code
func (mrw *metricsResponseWriter) WriteHeader(statusCode int) {
	mrw.statusCode = statusCode
	mrw.ResponseWriter.WriteHeader(statusCode)
}

// Write captures the response size
func (mrw *metricsResponseWriter) Write(b []byte) (int, error) {
	size, err := mrw.ResponseWriter.Write(b)
	mrw.size += size
	return size, err
}

// Middleware returns HTTP middleware that collects metrics
func (hm *HTTPMetrics) Middleware() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !hm.registry.IsEnabled() {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()
			
			// Increment active connections
			hm.activeConnections.Inc()
			defer hm.activeConnections.Dec()

			// Create wrapped response writer
			mrw := &metricsResponseWriter{
				ResponseWriter: w,
				statusCode:     200, // Default status code
			}

			// Process request
			next.ServeHTTP(mrw, r)

			// Calculate metrics
			duration := time.Since(start)
			
			// Extract labels
			method := r.Method
			path := normalizePath(r.URL.Path)
			status := strconv.Itoa(mrw.statusCode)
			statusClass := fmt.Sprintf("%dxx", mrw.statusCode/100)

			// Create labeled metrics
			requestsTotal := hm.registry.NewCounter(
				"http_requests_total",
				"Total number of HTTP requests",
				map[string]string{
					"method":       method,
					"path":         path,
					"status":       status,
					"status_class": statusClass,
				},
			)

			requestDuration := hm.registry.NewHistogram(
				"http_request_duration_seconds",
				"HTTP request duration in seconds",
				[]float64{0.001, 0.01, 0.1, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0},
				map[string]string{
					"method": method,
					"path":   path,
					"status": status,
				},
			)

			// Record metrics
			requestsTotal.Inc()
			requestDuration.Observe(duration.Seconds())

			if r.ContentLength > 0 {
				hm.requestSize.Observe(float64(r.ContentLength))
			}

			if mrw.size > 0 {
				hm.responseSize.Observe(float64(mrw.size))
			}
		})
	}
}

// normalizePath normalizes URL paths for metrics
func normalizePath(path string) string {
	// Skip normalization for health and metrics endpoints
	if path == "/health" || path == "/metrics" {
		return path
	}

	// Replace UUIDs and IDs with placeholders
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if isUUID(part) {
			parts[i] = "{uuid}"
		} else if isNumericID(part) {
			parts[i] = "{id}"
		}
	}

	normalized := strings.Join(parts, "/")
	
	// Limit path length to avoid cardinality explosion
	if len(normalized) > 100 {
		normalized = normalized[:100] + "..."
	}

	return normalized
}

// isUUID checks if a string looks like a UUID
func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	
	// Check UUID format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
	if s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return false
	}
	
	// Check if all other characters are hex digits
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			continue
		}
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	
	return true
}

// isNumericID checks if a string is a numeric ID
func isNumericID(s string) bool {
	if len(s) == 0 || len(s) > 20 {
		return false
	}
	
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	
	return true
}

// DatabaseMetrics collects database-related metrics
type DatabaseMetrics struct {
	registry         *MetricsRegistry
	connectionsOpen  *Gauge
	connectionsIdle  *Gauge
	connectionsInUse *Gauge
	queryDuration    *Histogram
	queriesTotal     *Counter
}

// NewDatabaseMetrics creates a new database metrics collector
func NewDatabaseMetrics(registry *MetricsRegistry) *DatabaseMetrics {
	if registry == nil {
		registry = GetRegistry()
	}

	return &DatabaseMetrics{
		registry: registry,
		connectionsOpen: registry.NewGauge(
			"db_connections_open",
			"Number of open database connections",
			nil,
		),
		connectionsIdle: registry.NewGauge(
			"db_connections_idle",
			"Number of idle database connections",
			nil,
		),
		connectionsInUse: registry.NewGauge(
			"db_connections_in_use",
			"Number of database connections in use",
			nil,
		),
		queryDuration: registry.NewHistogram(
			"db_query_duration_seconds",
			"Database query duration in seconds",
			[]float64{0.001, 0.01, 0.1, 0.5, 1.0, 2.5, 5.0, 10.0},
			nil,
		),
		queriesTotal: registry.NewCounter(
			"db_queries_total",
			"Total number of database queries",
			nil,
		),
	}
}

// RecordQuery records a database query
func (dm *DatabaseMetrics) RecordQuery(operation string, duration time.Duration, err error) {
	if !dm.registry.IsEnabled() {
		return
	}

	status := "success"
	if err != nil {
		status = "error"
	}

	queriesTotal := dm.registry.NewCounter(
		"db_queries_total",
		"Total number of database queries",
		map[string]string{
			"operation": operation,
			"status":    status,
		},
	)

	queryDuration := dm.registry.NewHistogram(
		"db_query_duration_seconds",
		"Database query duration in seconds",
		[]float64{0.001, 0.01, 0.1, 0.5, 1.0, 2.5, 5.0, 10.0},
		map[string]string{
			"operation": operation,
			"status":    status,
		},
	)

	queriesTotal.Inc()
	queryDuration.Observe(duration.Seconds())
}

// UpdateConnectionStats updates database connection statistics
func (dm *DatabaseMetrics) UpdateConnectionStats(open, idle, inUse int) {
	if !dm.registry.IsEnabled() {
		return
	}

	dm.connectionsOpen.Set(int64(open))
	dm.connectionsIdle.Set(int64(idle))
	dm.connectionsInUse.Set(int64(inUse))
}

// BusinessMetrics collects business-specific metrics
type BusinessMetrics struct {
	registry           *MetricsRegistry
	documentsProcessed *Counter
	chunksCreated      *Counter
	dlpViolations      *Counter
	apiKeys            *Gauge
	tenants            *Gauge
	storageUsed        *Gauge
}

// NewBusinessMetrics creates a new business metrics collector
func NewBusinessMetrics(registry *MetricsRegistry) *BusinessMetrics {
	if registry == nil {
		registry = GetRegistry()
	}

	return &BusinessMetrics{
		registry: registry,
		documentsProcessed: registry.NewCounter(
			"documents_processed_total",
			"Total number of documents processed",
			nil,
		),
		chunksCreated: registry.NewCounter(
			"chunks_created_total",
			"Total number of chunks created",
			nil,
		),
		dlpViolations: registry.NewCounter(
			"dlp_violations_total",
			"Total number of DLP violations detected",
			nil,
		),
		apiKeys: registry.NewGauge(
			"api_keys_active",
			"Number of active API keys",
			nil,
		),
		tenants: registry.NewGauge(
			"tenants_active",
			"Number of active tenants",
			nil,
		),
		storageUsed: registry.NewGauge(
			"storage_used_bytes",
			"Total storage used in bytes",
			nil,
		),
	}
}

// RecordDocumentProcessed records a processed document
func (bm *BusinessMetrics) RecordDocumentProcessed(contentType string, success bool) {
	if !bm.registry.IsEnabled() {
		return
	}

	status := "success"
	if !success {
		status = "error"
	}

	documentsProcessed := bm.registry.NewCounter(
		"documents_processed_total",
		"Total number of documents processed",
		map[string]string{
			"content_type": contentType,
			"status":       status,
		},
	)

	documentsProcessed.Inc()
}

// RecordChunksCreated records created chunks
func (bm *BusinessMetrics) RecordChunksCreated(count int, strategy string) {
	if !bm.registry.IsEnabled() {
		return
	}

	chunksCreated := bm.registry.NewCounter(
		"chunks_created_total",
		"Total number of chunks created",
		map[string]string{
			"strategy": strategy,
		},
	)

	chunksCreated.Add(int64(count))
}

// RecordDLPViolation records a DLP violation
func (bm *BusinessMetrics) RecordDLPViolation(violationType, severity string) {
	if !bm.registry.IsEnabled() {
		return
	}

	dlpViolations := bm.registry.NewCounter(
		"dlp_violations_total",
		"Total number of DLP violations detected",
		map[string]string{
			"type":     violationType,
			"severity": severity,
		},
	)

	dlpViolations.Inc()
}

// UpdateTenantCount updates the active tenant count
func (bm *BusinessMetrics) UpdateTenantCount(count int) {
	if !bm.registry.IsEnabled() {
		return
	}
	bm.tenants.Set(int64(count))
}

// UpdateAPIKeyCount updates the active API key count
func (bm *BusinessMetrics) UpdateAPIKeyCount(count int) {
	if !bm.registry.IsEnabled() {
		return
	}
	bm.apiKeys.Set(int64(count))
}

// UpdateStorageUsage updates the storage usage
func (bm *BusinessMetrics) UpdateStorageUsage(bytes int64) {
	if !bm.registry.IsEnabled() {
		return
	}
	bm.storageUsed.Set(bytes)
}

// GetHTTPMetricsMiddleware returns a middleware function for HTTP metrics
func GetHTTPMetricsMiddleware() func(next http.Handler) http.Handler {
	httpMetrics := NewHTTPMetrics(GetRegistry())
	return httpMetrics.Middleware()
}