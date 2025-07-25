package health

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// Handler provides HTTP endpoints for health checks
type Handler struct {
	checker *HealthChecker
	service string
	version string
}

// NewHandler creates a new health check HTTP handler
func NewHandler(checker *HealthChecker, service, version string) *Handler {
	return &Handler{
		checker: checker,
		service: service,
		version: version,
	}
}

// HealthCheckHandler handles health check HTTP requests
func (h *Handler) HealthCheckHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Parse query parameters
		detailed := r.URL.Query().Get("detailed") == "true"
		
		// Perform health check
		report := h.checker.Check(r.Context(), h.service, h.version)

		// Set appropriate HTTP status code
		statusCode := http.StatusOK
		switch report.Status {
		case StatusUnhealthy:
			statusCode = http.StatusServiceUnavailable
		case StatusDegraded:
			statusCode = http.StatusOK // 200 but with degraded status
		}

		// Prepare response
		var response interface{}
		if detailed {
			response = report
		} else {
			response = map[string]interface{}{
				"status":    report.Status,
				"timestamp": report.Timestamp,
				"version":   report.Version,
				"service":   report.Service,
				"summary":   report.Summary,
			}
		}

		// Set headers
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("X-Health-Check-Duration", report.Duration.String())
		w.WriteHeader(statusCode)

		// Encode response
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(response); err != nil {
			http.Error(w, "Failed to encode health check response", http.StatusInternalServerError)
		}
	}
}

// ReadinessHandler handles readiness probe requests
func (h *Handler) ReadinessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Perform health check focusing on critical components
		report := h.checker.Check(r.Context(), h.service, h.version)

		// Readiness means all critical components are healthy
		ready := true
		for _, check := range report.Checks {
			if check.Critical && check.Status != StatusHealthy {
				ready = false
				break
			}
		}

		statusCode := http.StatusOK
		status := "ready"
		if !ready {
			statusCode = http.StatusServiceUnavailable
			status = "not ready"
		}

		response := map[string]interface{}{
			"status":    status,
			"ready":     ready,
			"timestamp": time.Now(),
			"service":   h.service,
			"version":   h.version,
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.WriteHeader(statusCode)

		encoder := json.NewEncoder(w)
		if err := encoder.Encode(response); err != nil {
			http.Error(w, "Failed to encode readiness response", http.StatusInternalServerError)
		}
	}
}

// LivenessHandler handles liveness probe requests
func (h *Handler) LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Liveness is a simpler check - just verify the service is running
		response := map[string]interface{}{
			"status":    "alive",
			"timestamp": time.Now(),
			"service":   h.service,
			"version":   h.version,
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.WriteHeader(http.StatusOK)

		encoder := json.NewEncoder(w)
		if err := encoder.Encode(response); err != nil {
			http.Error(w, "Failed to encode liveness response", http.StatusInternalServerError)
		}
	}
}

// MetricsHandler provides health check metrics in a simple format
func (h *Handler) MetricsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		report := h.checker.Check(r.Context(), h.service, h.version)

		// Simple metrics format
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)

		// Overall status
		healthyValue := 0
		if report.Status == StatusHealthy {
			healthyValue = 1
		}
		w.Write([]byte("health_status " + strconv.Itoa(healthyValue) + "\n"))

		// Individual check metrics
		for name, check := range report.Checks {
			checkHealthy := 0
			if check.Status == StatusHealthy {
				checkHealthy = 1
			}
			w.Write([]byte("health_check{name=\"" + name + "\"} " + strconv.Itoa(checkHealthy) + "\n"))
			w.Write([]byte("health_check_duration_seconds{name=\"" + name + "\"} " + strconv.FormatFloat(check.Duration.Seconds(), 'f', 6, 64) + "\n"))
		}

		// Summary metrics
		for status, count := range report.Summary {
			w.Write([]byte("health_checks_total{status=\"" + status + "\"} " + strconv.Itoa(count) + "\n"))
		}
	}
}

// RegisterHandlers registers health check handlers with a ServeMux
func (h *Handler) RegisterHandlers(mux *http.ServeMux, basePath string) {
	if basePath == "" {
		basePath = "/health"
	}

	mux.HandleFunc(basePath, h.HealthCheckHandler())
	mux.HandleFunc(basePath+"/ready", h.ReadinessHandler())
	mux.HandleFunc(basePath+"/live", h.LivenessHandler())
	mux.HandleFunc(basePath+"/metrics", h.MetricsHandler())
}