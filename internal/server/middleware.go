package server

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/time/rate"

	"github.com/jscharber/audimodal/internal/database"
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

			_ = time.Since(start)
			_ = getRequestID(r.Context())

			// Log request - using proper logger would go here
		})
	}
}

// AuthenticationMiddleware handles authentication
// authenticateAPIKey reports whether the presented key matches one of the
// configured keys. Comparison is constant-time so a caller cannot recover a key
// by timing the response, and every candidate is compared so the work does not
// depend on which key matched.
//
// With no keys configured this always returns false: API-key auth is then off,
// not open. The previous implementation accepted ANY key of 32+ characters.
func authenticateAPIKey(config *Config, presented string) bool {
	if presented == "" {
		return false
	}
	ok := false
	for _, candidate := range config.APIKeys {
		if candidate == "" {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(presented), []byte(candidate)) == 1 {
			ok = true
		}
	}
	return ok
}

// authenticateJWT validates a bearer token as an HMAC-signed JWT.
//
// The signing method is pinned to HMAC. Accepting whatever the token's own
// header asks for is what makes "alg: none" and RS256-key-confusion attacks
// work, so anything else is rejected before the signature is checked.
// Expiry is enforced by the parser; a token without an exp claim is rejected
// too, since an unexpiring bearer token is indistinguishable from a password.
func authenticateJWT(config *Config, token string) bool {
	if config.JWTSecret == "" || token == "" {
		return false
	}
	parsed, err := jwt.Parse(token, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method %v", t.Header["alg"])
		}
		return []byte(config.JWTSecret), nil
	}, jwt.WithValidMethods([]string{"HS256", "HS384", "HS512"}), jwt.WithExpirationRequired())
	return err == nil && parsed.Valid
}

func AuthenticationMiddleware(config *Config, db *database.Database) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !config.AuthEnabled {
				next.ServeHTTP(w, r)
				return
			}

			// Health and metrics stay reachable without credentials: they are
			// what tells an operator the service is up, including when auth
			// itself is misconfigured.
			if r.URL.Path == config.HealthCheckPath || r.URL.Path == config.MetricsPath {
				next.ServeHTTP(w, r)
				return
			}

			// An API key and a bearer token are alternatives, and either must
			// actually verify. Every path below that does not verify falls
			// through to 401 -- there is deliberately no branch that continues
			// without having authenticated something.
			if apiKey := r.Header.Get(config.APIKeyHeader); apiKey != "" {
				if authenticateAPIKey(config, apiKey) {
					next.ServeHTTP(w, r)
					return
				}
				http.Error(w, "Invalid API key", http.StatusUnauthorized)
				return
			}

			authHeader := r.Header.Get("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				if authenticateJWT(config, strings.TrimPrefix(authHeader, "Bearer ")) {
					next.ServeHTTP(w, r)
					return
				}
				http.Error(w, "Invalid token", http.StatusUnauthorized)
				return
			}

			http.Error(w, "Authentication required", http.StatusUnauthorized)
		})
	}
}

// TenantMiddleware extracts and validates tenant information
func TenantMiddleware(db *database.Database) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// TODO: Add proper logger to context

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
					// Log panic - using proper logger would go here
					_ = requestID // Prevent unused variable error
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
