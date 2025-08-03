package server

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/time/rate"

	"github.com/jscharber/eAIIngest/internal/database"
)

// Middleware represents HTTP middleware
type Middleware func(http.Handler) http.Handler

// MiddlewareStack represents a stack of middleware
type MiddlewareStack struct {
	middlewares []Middleware
}

// NewMiddlewareStack creates a new middleware stack
func NewMiddlewareStack() *MiddlewareStack {
	return &MiddlewareStack{
		middlewares: make([]Middleware, 0),
	}
}

// Use adds a middleware to the stack
func (ms *MiddlewareStack) Use(middleware Middleware) {
	ms.middlewares = append(ms.middlewares, middleware)
}

// Apply applies all middleware to a handler
func (ms *MiddlewareStack) Apply(handler http.Handler) http.Handler {
	// Apply middleware in reverse order so they execute in the order they were added
	for i := len(ms.middlewares) - 1; i >= 0; i-- {
		handler = ms.middlewares[i](handler)
	}
	return handler
}

// RequestIDMiddleware adds a unique request ID to each request
func RequestIDMiddleware(header string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := r.Header.Get(header)
			if requestID == "" {
				requestID = uuid.New().String()
			}

			// Add to response header
			w.Header().Set(header, requestID)

			// Add to context
			ctx := context.WithValue(r.Context(), "request_id", requestID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// CORSMiddleware handles Cross-Origin Resource Sharing
func CORSMiddleware(config *Config) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !config.CORSEnabled {
				next.ServeHTTP(w, r)
				return
			}

			origin := r.Header.Get("Origin")

			// Check if origin is allowed
			allowed := false
			for _, allowedOrigin := range config.CORSAllowedOrigins {
				if allowedOrigin == "*" || allowedOrigin == origin {
					allowed = true
					break
				}
			}

			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			}

			w.Header().Set("Access-Control-Allow-Methods", strings.Join(config.CORSAllowedMethods, ", "))
			w.Header().Set("Access-Control-Allow-Headers", strings.Join(config.CORSAllowedHeaders, ", "))
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Max-Age", "86400")

			// Handle preflight requests
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RateLimitMiddleware implements rate limiting
func RateLimitMiddleware(config *Config) Middleware {
	if !config.RateLimitEnabled {
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	limiter := rate.NewLimiter(rate.Limit(config.RateLimitRPS), config.RateLimitBurst)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.Allow() {
				http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// LoggingMiddleware logs HTTP requests
func LoggingMiddleware(config *Config) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !config.LogRequests {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()

			// Wrap response writer to capture status code
			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(wrapped, r)

			duration := time.Since(start)
			requestID := getRequestID(r.Context())

			fmt.Printf("[%s] %s %s %d %v %s\n",
				start.Format("2006-01-02 15:04:05"),
				r.Method,
				r.RequestURI,
				wrapped.statusCode,
				duration,
				requestID,
			)
		})
	}
}

// AuthenticationMiddleware handles authentication
func AuthenticationMiddleware(config *Config, db *database.Database) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !config.AuthEnabled {
				next.ServeHTTP(w, r)
				return
			}

			// Skip auth for health check and metrics endpoints
			if r.URL.Path == config.HealthCheckPath || r.URL.Path == config.MetricsPath {
				next.ServeHTTP(w, r)
				return
			}

			// Check for API key in header
			apiKey := r.Header.Get(config.APIKeyHeader)
			if apiKey == "" {
				// Check for JWT token
				authHeader := r.Header.Get("Authorization")
				if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
					http.Error(w, "Authentication required", http.StatusUnauthorized)
					return
				}

				token := strings.TrimPrefix(authHeader, "Bearer ")
				// TODO: Implement JWT validation
				_ = token
			}

			// TODO: Validate API key against database
			// For now, we'll create a simple validation
			if apiKey != "" && len(apiKey) < 32 {
				http.Error(w, "Invalid API key", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// TenantMiddleware extracts and validates tenant information
func TenantMiddleware(db *database.Database) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract tenant ID from path or header
			tenantID := extractTenantID(r)
			if tenantID == "" {
				http.Error(w, "Tenant ID required", http.StatusBadRequest)
				return
			}

			// Validate tenant ID format
			tenantUUID, err := uuid.Parse(tenantID)
			if err != nil {
				http.Error(w, "Invalid tenant ID format", http.StatusBadRequest)
				return
			}

			// Validate tenant exists and is active
			tenantService := db.NewTenantService()
			tenant, err := tenantService.GetTenant(r.Context(), tenantUUID)
			if err != nil {
				http.Error(w, "Tenant not found", http.StatusNotFound)
				return
			}

			if !tenant.IsActive() {
				http.Error(w, "Tenant is not active", http.StatusForbidden)
				return
			}

			// Add tenant context to request
			tenantCtx := &database.TenantContext{
				TenantID:   tenantUUID,
				TenantName: tenant.Name,
				RequestID:  getRequestID(r.Context()),
			}

			ctx := context.WithValue(r.Context(), "tenant_context", tenantCtx)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RecoveryMiddleware recovers from panics
func RecoveryMiddleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					requestID := getRequestID(r.Context())
					fmt.Printf("PANIC [%s]: %v\n", requestID, err)
					http.Error(w, "Internal server error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeadersMiddleware adds security headers
func SecurityHeadersMiddleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("X-XSS-Protection", "1; mode=block")
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			w.Header().Set("Content-Security-Policy", "default-src 'self'")

			next.ServeHTTP(w, r)
		})
	}
}

// ContentTypeMiddleware sets default content type
func ContentTypeMiddleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if w.Header().Get("Content-Type") == "" {
				w.Header().Set("Content-Type", "application/json")
			}
			next.ServeHTTP(w, r)
		})
	}
}

// MaxRequestSizeMiddleware limits request body size
func MaxRequestSizeMiddleware(maxSize int64) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > maxSize {
				http.Error(w, "Request too large", http.StatusRequestEntityTooLarge)
				return
			}

			r.Body = http.MaxBytesReader(w, r.Body, maxSize)
			next.ServeHTTP(w, r)
		})
	}
}

// Helper types and functions

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func getRequestID(ctx context.Context) string {
	if requestID, ok := ctx.Value("request_id").(string); ok {
		return requestID
	}
	return "unknown"
}

func extractTenantID(r *http.Request) string {
	// Try header first
	if tenantID := r.Header.Get("X-Tenant-ID"); tenantID != "" {
		return tenantID
	}

	// Try path parameter
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(pathParts) >= 3 && pathParts[0] == "api" && pathParts[1] == "v1" && pathParts[2] == "tenants" {
		if len(pathParts) >= 4 {
			return pathParts[3]
		}
	}

	// Try query parameter
	return r.URL.Query().Get("tenant_id")
}

func getTenantContext(ctx context.Context) *database.TenantContext {
	if tenantCtx, ok := ctx.Value("tenant_context").(*database.TenantContext); ok {
		return tenantCtx
	}
	return nil
}

// PaginationMiddleware extracts pagination parameters
func PaginationMiddleware(config *Config) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			page := 1
			pageSize := config.DefaultPageSize

			if pageStr := r.URL.Query().Get("page"); pageStr != "" {
				if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
					page = p
				}
			}

			if sizeStr := r.URL.Query().Get("page_size"); sizeStr != "" {
				if s, err := strconv.Atoi(sizeStr); err == nil && s > 0 && s <= config.MaxPageSize {
					pageSize = s
				}
			}

			ctx := context.WithValue(r.Context(), "pagination", map[string]int{
				"page":      page,
				"page_size": pageSize,
				"offset":    (page - 1) * pageSize,
			})

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func getPagination(ctx context.Context) (page, pageSize, offset int) {
	if pagination, ok := ctx.Value("pagination").(map[string]int); ok {
		return pagination["page"], pagination["page_size"], pagination["offset"]
	}
	return 1, 20, 0
}
