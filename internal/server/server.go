package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jscharber/eAIIngest/internal/database"
	"github.com/jscharber/eAIIngest/internal/server/handlers"
)

// Server represents the HTTP server
type Server struct {
	config     *Config
	db         *database.Database
	httpServer *http.Server
	router     *Router
}

// New creates a new HTTP server
func New(config *Config, db *database.Database) (*Server, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid server config: %w", err)
	}

	server := &Server{
		config: config,
		db:     db,
	}

	// Initialize router
	server.router = NewRouter(config, db)

	// Create HTTP server
	server.httpServer = &http.Server{
		Addr:         config.GetAddress(),
		Handler:      server.router,
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
		IdleTimeout:  config.IdleTimeout,
	}

	return server, nil
}

// Start starts the HTTP server
func (s *Server) Start(ctx context.Context) error {
	// Start server in a goroutine
	go func() {
		fmt.Printf("Starting server on %s\n", s.config.GetAddress())
		
		var err error
		if s.config.TLSEnabled {
			err = s.httpServer.ListenAndServeTLS(s.config.TLSCertFile, s.config.TLSKeyFile)
		} else {
			err = s.httpServer.ListenAndServe()
		}
		
		if err != nil && err != http.ErrServerClosed {
			fmt.Printf("Server error: %v\n", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-quit:
		fmt.Println("Shutting down server...")
	case <-ctx.Done():
		fmt.Println("Context cancelled, shutting down server...")
	}

	return s.Shutdown()
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), s.config.ShutdownTimeout)
	defer cancel()

	if err := s.httpServer.Shutdown(ctx); err != nil {
		fmt.Printf("Server shutdown error: %v\n", err)
		return err
	}

	fmt.Println("Server shutdown complete")
	return nil
}

// Router represents the HTTP router
type Router struct {
	*http.ServeMux
	config *Config
	db     *database.Database
	middleware *MiddlewareStack
}

// NewRouter creates a new HTTP router
func NewRouter(config *Config, db *database.Database) *Router {
	router := &Router{
		ServeMux: http.NewServeMux(),
		config:   config,
		db:       db,
		middleware: NewMiddlewareStack(),
	}

	router.setupMiddleware()
	router.setupRoutes()

	return router
}

// ServeHTTP implements the http.Handler interface
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// Apply middleware stack
	handler := r.middleware.Apply(r.ServeMux)
	handler.ServeHTTP(w, req)
}

// setupMiddleware configures the middleware stack
func (r *Router) setupMiddleware() {
	// Add middleware in order of execution
	r.middleware.Use(RecoveryMiddleware())
	r.middleware.Use(SecurityHeadersMiddleware())
	r.middleware.Use(RequestIDMiddleware(r.config.RequestIDHeader))
	r.middleware.Use(LoggingMiddleware(r.config))
	r.middleware.Use(CORSMiddleware(r.config))
	r.middleware.Use(RateLimitMiddleware(r.config))
	r.middleware.Use(MaxRequestSizeMiddleware(r.config.MaxRequestSize))
	r.middleware.Use(ContentTypeMiddleware())
	r.middleware.Use(PaginationMiddleware(r.config))
}

// setupRoutes configures all API routes
func (r *Router) setupRoutes() {
	// Health check endpoint (no auth required)
	r.HandleFunc(r.config.HealthCheckPath, r.healthCheckHandler)

	// Metrics endpoint (no auth required)
	if r.config.MetricsEnabled {
		r.HandleFunc(r.config.MetricsPath, r.metricsHandler)
	}

	// API routes with authentication
	apiPrefix := r.config.APIPrefix

	// Create handlers
	tenantHandler := handlers.NewTenantHandler(r.db)
	dataSourceHandler := handlers.NewDataSourceHandler(r.db)
	sessionHandler := handlers.NewProcessingSessionHandler(r.db)
	dlpHandler := handlers.NewDLPHandler(r.db)
	fileHandler := handlers.NewFileHandler(r.db)
	chunkHandler := handlers.NewChunkHandler(r.db)

	// Apply authentication middleware to API routes
	authMiddleware := AuthenticationMiddleware(r.config, r.db)

	// Tenant management routes
	r.Handle(fmt.Sprintf("%s/tenants", apiPrefix), authMiddleware(http.HandlerFunc(tenantHandler.ListTenants)))
	r.Handle(fmt.Sprintf("%s/tenants/", apiPrefix), authMiddleware(http.HandlerFunc(tenantHandler.HandleTenant)))

	// Tenant-scoped routes (require tenant context)
	tenantMiddleware := TenantMiddleware(r.db)
	tenantAuth := func(h http.Handler) http.Handler {
		return authMiddleware(tenantMiddleware(h))
	}

	// Data source routes
	r.Handle(fmt.Sprintf("%s/tenants/", apiPrefix), tenantAuth(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if isDataSourceRoute(req.URL.Path, apiPrefix) {
			dataSourceHandler.ServeHTTP(w, req)
		} else {
			http.NotFound(w, req)
		}
	})))

	// Processing session routes
	r.Handle(fmt.Sprintf("%s/tenants/", apiPrefix), tenantAuth(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if isSessionRoute(req.URL.Path, apiPrefix) {
			sessionHandler.ServeHTTP(w, req)
		} else {
			http.NotFound(w, req)
		}
	})))

	// DLP policy routes
	r.Handle(fmt.Sprintf("%s/tenants/", apiPrefix), tenantAuth(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if isDLPRoute(req.URL.Path, apiPrefix) {
			dlpHandler.ServeHTTP(w, req)
		} else {
			http.NotFound(w, req)
		}
	})))

	// File routes
	r.Handle(fmt.Sprintf("%s/tenants/", apiPrefix), tenantAuth(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if isFileRoute(req.URL.Path, apiPrefix) {
			fileHandler.ServeHTTP(w, req)
		} else {
			http.NotFound(w, req)
		}
	})))

	// Chunk routes
	r.Handle(fmt.Sprintf("%s/tenants/", apiPrefix), tenantAuth(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if isChunkRoute(req.URL.Path, apiPrefix) {
			chunkHandler.ServeHTTP(w, req)
		} else {
			http.NotFound(w, req)
		}
	})))

	// API documentation route
	r.HandleFunc(fmt.Sprintf("%s/docs", apiPrefix), r.docsHandler)
	r.HandleFunc(fmt.Sprintf("%s/", apiPrefix), r.apiRootHandler)
}

// healthCheckHandler handles health check requests
func (r *Router) healthCheckHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	checks := make(map[string]string)
	status := "healthy"

	// Check database connection
	if err := r.db.HealthCheck(req.Context()); err != nil {
		checks["database"] = "unhealthy: " + err.Error()
		status = "unhealthy"
	} else {
		checks["database"] = "healthy"
	}

	// Add more health checks as needed
	checks["server"] = "healthy"

	WriteHealthCheck(w, status, "1.0.0", checks)
}

// metricsHandler handles metrics requests
func (r *Router) metricsHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get database stats
	stats, err := r.db.GetStats()
	if err != nil {
		http.Error(w, "Failed to get metrics", http.StatusInternalServerError)
		return
	}

	requestID := getRequestID(req.Context())
	WriteSuccess(w, requestID, stats, nil)
}

// docsHandler handles API documentation requests
func (r *Router) docsHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	docs := map[string]interface{}{
		"title":       "eAIIngest API",
		"version":     "1.0.0",
		"description": "Enterprise AI Document Processing Platform API",
		"base_url":    fmt.Sprintf("http://%s%s", req.Host, r.config.APIPrefix),
		"endpoints": map[string]interface{}{
			"tenants":             fmt.Sprintf("%s/tenants", r.config.APIPrefix),
			"data_sources":        fmt.Sprintf("%s/tenants/{tenant_id}/data-sources", r.config.APIPrefix),
			"processing_sessions": fmt.Sprintf("%s/tenants/{tenant_id}/sessions", r.config.APIPrefix),
			"dlp_policies":        fmt.Sprintf("%s/tenants/{tenant_id}/dlp-policies", r.config.APIPrefix),
			"files":              fmt.Sprintf("%s/tenants/{tenant_id}/files", r.config.APIPrefix),
			"chunks":             fmt.Sprintf("%s/tenants/{tenant_id}/chunks", r.config.APIPrefix),
		},
		"authentication": map[string]interface{}{
			"type":        "API Key or JWT Bearer Token",
			"header":      r.config.APIKeyHeader,
			"bearer_auth": "Authorization: Bearer <token>",
		},
	}

	requestID := getRequestID(req.Context())
	WriteSuccess(w, requestID, docs, nil)
}

// apiRootHandler handles API root requests
func (r *Router) apiRootHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	info := map[string]interface{}{
		"name":        "eAIIngest API",
		"version":     "1.0.0",
		"description": "Enterprise AI Document Processing Platform",
		"status":      "active",
		"endpoints": map[string]string{
			"health":        r.config.HealthCheckPath,
			"metrics":       r.config.MetricsPath,
			"documentation": fmt.Sprintf("%s/docs", r.config.APIPrefix),
		},
	}

	requestID := getRequestID(req.Context())
	WriteSuccess(w, requestID, info, nil)
}

// Route helper functions
func isDataSourceRoute(path, apiPrefix string) bool {
	return contains(path, "/data-sources")
}

func isSessionRoute(path, apiPrefix string) bool {
	return contains(path, "/sessions")
}

func isDLPRoute(path, apiPrefix string) bool {
	return contains(path, "/dlp-policies") || contains(path, "/violations")
}

func isFileRoute(path, apiPrefix string) bool {
	return contains(path, "/files")
}

func isChunkRoute(path, apiPrefix string) bool {
	return contains(path, "/chunks")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[len(s)-len(substr):] == substr || 
		   len(s) > len(substr) && s[len(s)-len(substr)-1:len(s)-len(substr)] == "/" && s[len(s)-len(substr):] == substr ||
		   s == substr ||
		   (len(s) > len(substr) && s[:len(substr)] == substr && s[len(substr)] == '/')
}

// RunServer is a convenience function to run the server
func RunServer(config *Config, db *database.Database) error {
	server, err := New(config, db)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	ctx := context.Background()
	return server.Start(ctx)
}