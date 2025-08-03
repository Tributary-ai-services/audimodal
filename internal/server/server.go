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
	"github.com/jscharber/eAIIngest/internal/server/response"
	"github.com/jscharber/eAIIngest/pkg/health"
	"github.com/jscharber/eAIIngest/pkg/logger"
	"github.com/jscharber/eAIIngest/pkg/metrics"
)

// Server represents the HTTP server
type Server struct {
	config          *Config
	db              *database.Database
	httpServer      *http.Server
	router          *Router
	logger          *logger.Logger
	metricsRegistry *metrics.MetricsRegistry
	systemMetrics   *metrics.SystemMetrics
	healthChecker   *health.HealthChecker
	healthHandler   *health.Handler
}

// New creates a new HTTP server
func New(config *Config, db *database.Database) (*Server, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid server config: %w", err)
	}

	// Initialize structured logger
	logLevel := logger.ParseLogLevel(config.LogLevel)
	logFormat := logger.JSONFormat
	if config.LogFormat == "text" {
		logFormat = logger.TextFormat
	}

	serverLogger := logger.NewLogger(&logger.Config{
		Level:        logLevel,
		Format:       logFormat,
		Output:       os.Stdout,
		Service:      "audimodal-server",
		Version:      "1.0.0",
		EnableCaller: config.IsDevelopment(),
		Fields:       make(map[string]interface{}),
	})

	// Initialize metrics registry
	metricsRegistry := metrics.NewRegistry()
	metricsRegistry.SetEnabled(config.MetricsEnabled)
	metricsRegistry.AddGlobalLabel("service", "audimodal")
	metricsRegistry.AddGlobalLabel("version", "1.0.0")

	// Initialize system metrics
	systemMetrics := metrics.NewSystemMetrics(metricsRegistry)

	// Initialize health checker
	healthChecker := health.NewHealthChecker(config.HealthCheckTimeout)

	// Add database health check
	healthChecker.AddChecker(health.DatabaseChecker("database", func(ctx context.Context) error {
		return db.HealthCheck(ctx)
	}))

	// Add system health checks
	healthChecker.AddChecker(health.MemoryChecker("memory", 90.0))
	healthChecker.AddChecker(health.DiskSpaceChecker("disk", "/", 10.0))

	// Create health handler
	healthHandler := health.NewHandler(healthChecker, "audimodal", "1.0.0")

	server := &Server{
		config:          config,
		db:              db,
		logger:          serverLogger,
		metricsRegistry: metricsRegistry,
		systemMetrics:   systemMetrics,
		healthChecker:   healthChecker,
		healthHandler:   healthHandler,
	}

	// Initialize router
	server.router = NewRouter(config, db, serverLogger, metricsRegistry, healthHandler)

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

// Start starts the HTTP server with graceful shutdown
func (s *Server) Start(ctx context.Context) error {
	// Channel to signal server start errors
	serverErrors := make(chan error, 1)

	// Start system metrics collection
	if s.config.MetricsEnabled {
		go s.systemMetrics.Start(ctx, 15*time.Second)
		s.logger.Info("System metrics collection started")
	}

	// Start server in a goroutine
	go func() {
		s.logger.WithFields(map[string]interface{}{
			"address":     s.config.GetAddress(),
			"tls_enabled": s.config.TLSEnabled,
		}).Info("Starting HTTP server")

		var err error
		if s.config.TLSEnabled {
			s.logger.WithFields(map[string]interface{}{
				"cert_file": s.config.TLSCertFile,
				"key_file":  s.config.TLSKeyFile,
			}).Info("TLS enabled, starting HTTPS server")
			err = s.httpServer.ListenAndServeTLS(s.config.TLSCertFile, s.config.TLSKeyFile)
		} else {
			s.logger.Info("TLS disabled, starting HTTP server")
			err = s.httpServer.ListenAndServe()
		}

		if err != nil && err != http.ErrServerClosed {
			s.logger.WithField("error", err).Error("HTTP server error")
			serverErrors <- err
		}
	}()

	// Wait for server to start or fail
	select {
	case err := <-serverErrors:
		return fmt.Errorf("server failed to start: %w", err)
	case <-time.After(1 * time.Second):
		s.logger.Info("HTTP server started successfully")
	}

	// Set up signal handling for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	// Wait for shutdown signal or context cancellation
	select {
	case sig := <-quit:
		s.logger.WithField("signal", sig.String()).Info("Received shutdown signal, initiating graceful shutdown")
	case <-ctx.Done():
		s.logger.Info("Context cancelled, initiating graceful shutdown")
	case err := <-serverErrors:
		s.logger.WithField("error", err).Error("Server error detected, initiating shutdown")
		return err
	}

	return s.Shutdown()
}

// Shutdown gracefully shuts down the server and its dependencies
func (s *Server) Shutdown() error {
	s.logger.Info("Starting graceful shutdown sequence")

	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), s.config.ShutdownTimeout)
	defer cancel()

	// Channel to collect shutdown errors
	shutdownErrors := make(chan error, 2)
	var shutdownErr error

	// Start graceful HTTP server shutdown
	go func() {
		s.logger.Info("Shutting down HTTP server")
		if err := s.httpServer.Shutdown(ctx); err != nil {
			shutdownErrors <- fmt.Errorf("HTTP server shutdown error: %w", err)
		} else {
			s.logger.Info("HTTP server stopped gracefully")
			shutdownErrors <- nil
		}
	}()

	// Start database connection cleanup
	go func() {
		s.logger.Info("Closing database connections")
		if err := s.db.Close(); err != nil {
			shutdownErrors <- fmt.Errorf("database shutdown error: %w", err)
		} else {
			s.logger.Info("Database connections closed")
			shutdownErrors <- nil
		}
	}()

	// Stop system metrics collection
	if s.config.MetricsEnabled {
		s.systemMetrics.Stop()
		s.logger.Info("System metrics collection stopped")
	}

	// Wait for both shutdown operations to complete
	for i := 0; i < 2; i++ {
		select {
		case err := <-shutdownErrors:
			if err != nil && shutdownErr == nil {
				shutdownErr = err // Capture first error
				s.logger.WithField("error", err).Error("Shutdown component error")
			}
		case <-ctx.Done():
			if shutdownErr == nil {
				shutdownErr = fmt.Errorf("shutdown timeout exceeded: %v", ctx.Err())
			}
			break
		}
	}

	// Force close if shutdown timeout was exceeded
	if ctx.Err() != nil {
		s.logger.Warn("Graceful shutdown timeout exceeded, forcing server close")
		if err := s.httpServer.Close(); err != nil {
			s.logger.WithField("error", err).Error("Force close error")
		}
	}

	if shutdownErr != nil {
		s.logger.WithField("error", shutdownErr).Error("Shutdown completed with errors")
		return shutdownErr
	}

	s.logger.Info("Graceful shutdown completed successfully")
	return nil
}

// Router represents the HTTP router
type Router struct {
	*http.ServeMux
	config          *Config
	db              *database.Database
	logger          *logger.Logger
	metricsRegistry *metrics.MetricsRegistry
	healthHandler   *health.Handler
	middleware      *MiddlewareStack
}

// NewRouter creates a new HTTP router
func NewRouter(config *Config, db *database.Database, logger *logger.Logger, metricsRegistry *metrics.MetricsRegistry, healthHandler *health.Handler) *Router {
	router := &Router{
		ServeMux:        http.NewServeMux(),
		config:          config,
		db:              db,
		logger:          logger,
		metricsRegistry: metricsRegistry,
		healthHandler:   healthHandler,
		middleware:      NewMiddlewareStack(),
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

	// Add metrics collection middleware if enabled
	if r.config.MetricsEnabled {
		r.middleware.Use(metrics.GetHTTPMetricsMiddleware())
	}

	// Add structured logging middleware if request logging is enabled
	if r.config.LogRequests {
		httpLogConfig := &logger.HTTPLogConfig{
			SkipPaths:       []string{r.config.HealthCheckPath, r.config.MetricsPath},
			LogRequestBody:  r.config.LogRequestBody,
			LogResponseBody: r.config.LogResponseBody,
			LogHeaders:      r.config.LogHeaders,
			SanitizeHeaders: []string{
				"authorization", "x-api-key", "cookie", "set-cookie",
				"x-auth-token", "x-csrf-token", "jwt", "bearer",
			},
			MaxBodySize: 1024, // 1KB
		}
		r.middleware.Use(logger.RequestLoggingMiddleware(r.logger, httpLogConfig))
	}

	r.middleware.Use(CORSMiddleware(r.config))
	r.middleware.Use(RateLimitMiddleware(r.config))
	r.middleware.Use(MaxRequestSizeMiddleware(r.config.MaxRequestSize))
	r.middleware.Use(ContentTypeMiddleware())
	r.middleware.Use(PaginationMiddleware(r.config))
}

// setupRoutes configures all API routes
func (r *Router) setupRoutes() {
	// Health check endpoints (no auth required)
	r.HandleFunc(r.config.HealthCheckPath, r.healthHandler.HealthCheckHandler())
	r.HandleFunc(r.config.HealthCheckPath+"/ready", r.healthHandler.ReadinessHandler())
	r.HandleFunc(r.config.HealthCheckPath+"/live", r.healthHandler.LivenessHandler())

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
	webHandler := handlers.NewWebHandler(r.db, r.logger)
	mlAnalysisHandler := handlers.NewMLAnalysisHandler(r.db, r.logger)

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

	// ML Analysis routes
	r.Handle(fmt.Sprintf("%s/tenants/", apiPrefix), tenantAuth(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if isMLAnalysisRoute(req.URL.Path, apiPrefix) {
			mlAnalysisHandler.ServeHTTP(w, req)
		} else {
			http.NotFound(w, req)
		}
	})))

	// API documentation route
	r.HandleFunc(fmt.Sprintf("%s/docs", apiPrefix), r.docsHandler)
	r.HandleFunc(fmt.Sprintf("%s/", apiPrefix), r.apiRootHandler)

	// Web UI routes
	r.HandleFunc("/", webHandler.RedirectToLogin())
	r.HandleFunc("/login", webHandler.LoginHandler())
	r.HandleFunc("/dashboard", webHandler.DashboardHandler())
	r.HandleFunc("/static/", webHandler.StaticFileHandler())
}

// metricsHandler handles metrics requests
func (r *Router) metricsHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Use the metrics registry handler
	if r.metricsRegistry != nil {
		handler := metrics.HTTPMetricsHandler(r.metricsRegistry)
		handler.ServeHTTP(w, req)
		return
	}

	// Fallback to basic response if metrics are disabled
	http.Error(w, "Metrics collection is disabled", http.StatusServiceUnavailable)
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
			"files":               fmt.Sprintf("%s/tenants/{tenant_id}/files", r.config.APIPrefix),
			"chunks":              fmt.Sprintf("%s/tenants/{tenant_id}/chunks", r.config.APIPrefix),
		},
		"authentication": map[string]interface{}{
			"type":        "API Key or JWT Bearer Token",
			"header":      r.config.APIKeyHeader,
			"bearer_auth": "Authorization: Bearer <token>",
		},
	}

	requestID := getRequestID(req.Context())
	response.WriteSuccess(w, requestID, docs, nil)
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
	response.WriteSuccess(w, requestID, info, nil)
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

func isMLAnalysisRoute(path, apiPrefix string) bool {
	return contains(path, "/ml-analysis")
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
