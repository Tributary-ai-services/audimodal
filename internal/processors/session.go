package processors

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/jscharber/eAIIngest/internal/database"
	"github.com/jscharber/eAIIngest/internal/database/models"
)

// SessionManager manages processing sessions and coordinates file processing
type SessionManager struct {
	db                *database.Database
	pipeline          *Pipeline
	activeSessions    map[uuid.UUID]*SessionContext
	sessionMutex      sync.RWMutex
	defaultTimeout    time.Duration
	maxFilesPerSession int
}

// SessionContext tracks the state of an active processing session
type SessionContext struct {
	SessionID      uuid.UUID              `json:"session_id"`
	TenantID       uuid.UUID              `json:"tenant_id"`
	Status         string                 `json:"status"`
	StartedAt      time.Time              `json:"started_at"`
	CompletedAt    *time.Time             `json:"completed_at,omitempty"`
	TotalFiles     int                    `json:"total_files"`
	ProcessedFiles int                    `json:"processed_files"`
	FailedFiles    int                    `json:"failed_files"`
	TotalChunks    int                    `json:"total_chunks"`
	ProcessingJobs map[uuid.UUID]*JobContext `json:"-"`
	Config         *SessionConfig         `json:"config"`
	mutex          sync.RWMutex
}

// JobContext tracks individual file processing jobs within a session
type JobContext struct {
	FileID      uuid.UUID         `json:"file_id"`
	Status      string            `json:"status"`
	StartedAt   time.Time         `json:"started_at"`
	CompletedAt *time.Time        `json:"completed_at,omitempty"`
	Progress    float64           `json:"progress"`
	Error       string            `json:"error,omitempty"`
	Result      *ProcessingResult `json:"result,omitempty"`
}

// SessionConfig contains configuration for a processing session
type SessionConfig struct {
	Priority           string         `json:"priority"`
	MaxConcurrentFiles int            `json:"max_concurrent_files"`
	RetryEnabled       bool           `json:"retry_enabled"`
	RetryAttempts      int            `json:"retry_attempts"`
	DLPScanEnabled     bool           `json:"dlp_scan_enabled"`
	ComplianceRules    []string       `json:"compliance_rules,omitempty"`
	ProcessingOptions  map[string]any `json:"processing_options,omitempty"`
}

// SessionProgress represents the overall progress of a session
type SessionProgress struct {
	SessionID        uuid.UUID     `json:"session_id"`
	Status           string        `json:"status"`
	Progress         float64       `json:"progress"`
	FilesTotal       int           `json:"files_total"`
	FilesProcessed   int           `json:"files_processed"`
	FilesFailed      int           `json:"files_failed"`
	ChunksCreated    int           `json:"chunks_created"`
	BytesProcessed   int64         `json:"bytes_processed"`
	ElapsedTime      time.Duration `json:"elapsed_time"`
	EstimatedTimeLeft time.Duration `json:"estimated_time_left"`
	CurrentFile      string        `json:"current_file,omitempty"`
}

// NewSessionManager creates a new session manager
func NewSessionManager(db *database.Database, pipeline *Pipeline) *SessionManager {
	return &SessionManager{
		db:                 db,
		pipeline:           pipeline,
		activeSessions:     make(map[uuid.UUID]*SessionContext),
		defaultTimeout:     2 * time.Hour,
		maxFilesPerSession: 1000,
	}
}

// CreateSession creates a new processing session
func (sm *SessionManager) CreateSession(ctx context.Context, tenantID uuid.UUID, config *SessionConfig) (*SessionContext, error) {
	sessionID := uuid.New()
	
	if config == nil {
		config = &SessionConfig{
			Priority:           "normal",
			MaxConcurrentFiles: 5,
			RetryEnabled:       true,
			RetryAttempts:      3,
			DLPScanEnabled:     true,
		}
	}

	// Create session in database
	dbSession := &models.ProcessingSession{
		ID:                sessionID,
		TenantID:          tenantID,
		Status:            models.SessionStatusPending,
		MaxRetries:        config.RetryAttempts,
		// Map config to ProcessingOptions structure
		Options: models.ProcessingOptions{
			DLPScanEnabled:     config.DLPScanEnabled,
			ChunkingStrategy:   "fixed_size", // Default strategy
			ParallelProcessing: true,         // Default to parallel
			BatchSize:          100,          // Default batch size
		},
		Progress:          0.0,
		TotalFiles:        0,
		ProcessedFiles:    0,
		FailedFiles:       0,
	}

	tenantService := sm.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant repository: %w", err)
	}

	if err := tenantRepo.ValidateAndCreate(dbSession); err != nil {
		return nil, fmt.Errorf("failed to create session in database: %w", err)
	}

	// Create session context
	sessionCtx := &SessionContext{
		SessionID:      sessionID,
		TenantID:       tenantID,
		Status:         models.SessionStatusPending,
		StartedAt:      time.Now(),
		ProcessingJobs: make(map[uuid.UUID]*JobContext),
		Config:         config,
	}

	// Store in active sessions
	sm.sessionMutex.Lock()
	sm.activeSessions[sessionID] = sessionCtx
	sm.sessionMutex.Unlock()

	return sessionCtx, nil
}

// AddFilesToSession adds files to an existing processing session
func (sm *SessionManager) AddFilesToSession(ctx context.Context, sessionID uuid.UUID, fileIDs []uuid.UUID) error {
	sm.sessionMutex.RLock()
	sessionCtx, exists := sm.activeSessions[sessionID]
	sm.sessionMutex.RUnlock()

	if !exists {
		return fmt.Errorf("session %s not found", sessionID)
	}

	if len(fileIDs) > sm.maxFilesPerSession {
		return fmt.Errorf("too many files: maximum %d files per session", sm.maxFilesPerSession)
	}

	sessionCtx.mutex.Lock()
	defer sessionCtx.mutex.Unlock()

	// Update session file count
	sessionCtx.TotalFiles = len(fileIDs)

	// Create job contexts for each file
	for _, fileID := range fileIDs {
		jobCtx := &JobContext{
			FileID:  fileID,
			Status:  "pending",
			Progress: 0.0,
		}
		sessionCtx.ProcessingJobs[fileID] = jobCtx
	}

	// Update database
	tenantService := sm.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(ctx, sessionCtx.TenantID)
	if err != nil {
		return fmt.Errorf("failed to get tenant repository: %w", err)
	}

	var dbSession models.ProcessingSession
	if err := tenantRepo.DB().Where("id = ?", sessionID).First(&dbSession).Error; err != nil {
		return fmt.Errorf("failed to find session: %w", err)
	}

	updates := map[string]interface{}{
		"files_total": len(fileIDs),
		"status":      models.SessionStatusPending,
	}

	return tenantRepo.DB().Model(&dbSession).Updates(updates).Error
}

// StartSession begins processing all files in the session
func (sm *SessionManager) StartSession(ctx context.Context, sessionID uuid.UUID) error {
	sm.sessionMutex.RLock()
	sessionCtx, exists := sm.activeSessions[sessionID]
	sm.sessionMutex.RUnlock()

	if !exists {
		return fmt.Errorf("session %s not found", sessionID)
	}

	sessionCtx.mutex.Lock()
	if sessionCtx.Status != models.SessionStatusPending {
		sessionCtx.mutex.Unlock()
		return fmt.Errorf("session is not in a startable state: %s", sessionCtx.Status)
	}

	sessionCtx.Status = models.SessionStatusRunning
	sessionCtx.StartedAt = time.Now()
	sessionCtx.mutex.Unlock()

	// Update database status
	if err := sm.updateSessionStatus(ctx, sessionID, models.SessionStatusRunning); err != nil {
		return fmt.Errorf("failed to update session status: %w", err)
	}

	// Start processing files concurrently
	go sm.processSessionFiles(ctx, sessionCtx)

	return nil
}

// processSessionFiles processes all files in the session
func (sm *SessionManager) processSessionFiles(ctx context.Context, sessionCtx *SessionContext) {
	// Create context with timeout
	sessionCtx.mutex.RLock()
	maxConcurrent := sessionCtx.Config.MaxConcurrentFiles
	sessionCtx.mutex.RUnlock()

	// Create semaphore for concurrent processing
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup

	// Process each file
	sessionCtx.mutex.RLock()
	jobs := make([]*JobContext, 0, len(sessionCtx.ProcessingJobs))
	for _, job := range sessionCtx.ProcessingJobs {
		jobs = append(jobs, job)
	}
	sessionCtx.mutex.RUnlock()

	for _, job := range jobs {
		wg.Add(1)
		go func(jobCtx *JobContext) {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			// Process file
			sm.processFile(ctx, sessionCtx, jobCtx)
		}(job)
	}

	// Wait for all jobs to complete
	wg.Wait()

	// Update final session status
	sm.completeSession(ctx, sessionCtx)
}

// processFile processes a single file within a session
func (sm *SessionManager) processFile(ctx context.Context, sessionCtx *SessionContext, jobCtx *JobContext) {
	// Update job status
	jobCtx.Status = "processing"
	jobCtx.StartedAt = time.Now()

	// Get file information
	var file models.File
	if err := sm.db.DB().Where("id = ?", jobCtx.FileID).First(&file).Error; err != nil {
		jobCtx.Status = "failed"
		jobCtx.Error = fmt.Sprintf("failed to find file: %v", err)
		sm.updateSessionProgress(sessionCtx)
		return
	}

	// Create processing request
	request := &ProcessingRequest{
		TenantID:        sessionCtx.TenantID,
		SessionID:       sessionCtx.SessionID,
		FileID:          jobCtx.FileID,
		FilePath:        file.Path,
		Priority:        sessionCtx.Config.Priority,
		DLPScanEnabled:  sessionCtx.Config.DLPScanEnabled,
		ComplianceRules: sessionCtx.Config.ComplianceRules,
	}

	// Apply session processing options
	if sessionCtx.Config.ProcessingOptions != nil {
		if readerType, ok := sessionCtx.Config.ProcessingOptions["reader_type"].(string); ok {
			request.ReaderType = readerType
		}
		if strategyType, ok := sessionCtx.Config.ProcessingOptions["strategy_type"].(string); ok {
			request.StrategyType = strategyType
		}
		if readerConfig, ok := sessionCtx.Config.ProcessingOptions["reader_config"].(map[string]any); ok {
			request.ReaderConfig = readerConfig
		}
		if strategyConfig, ok := sessionCtx.Config.ProcessingOptions["strategy_config"].(map[string]any); ok {
			request.StrategyConfig = strategyConfig
		}
	}

	// Process file through pipeline
	result, err := sm.pipeline.ProcessFile(ctx, request)
	
	// Update job context
	now := time.Now()
	jobCtx.CompletedAt = &now
	jobCtx.Progress = 1.0
	jobCtx.Result = result

	if err != nil {
		jobCtx.Status = "failed"
		jobCtx.Error = err.Error()
		sessionCtx.mutex.Lock()
		sessionCtx.FailedFiles++
		sessionCtx.mutex.Unlock()
	} else {
		jobCtx.Status = "completed"
		sessionCtx.mutex.Lock()
		sessionCtx.ProcessedFiles++
		sessionCtx.TotalChunks += result.ChunksCreated
		sessionCtx.mutex.Unlock()
	}

	// Update session progress
	sm.updateSessionProgress(sessionCtx)
}

// updateSessionProgress updates the overall session progress
func (sm *SessionManager) updateSessionProgress(sessionCtx *SessionContext) {
	sessionCtx.mutex.RLock()
	totalFiles := sessionCtx.TotalFiles
	completedJobs := 0
	
	for _, job := range sessionCtx.ProcessingJobs {
		if job.Status == "completed" || job.Status == "failed" {
			completedJobs++
		}
	}
	sessionCtx.mutex.RUnlock()

	progress := 0.0
	if totalFiles > 0 {
		progress = float64(completedJobs) / float64(totalFiles)
	}

	// Update database
	ctx := context.Background()
	tenantService := sm.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(ctx, sessionCtx.TenantID)
	if err != nil {
		return // Log error in production
	}

	var dbSession models.ProcessingSession
	if err := tenantRepo.DB().Where("id = ?", sessionCtx.SessionID).First(&dbSession).Error; err != nil {
		return // Log error in production
	}

	sessionCtx.mutex.RLock()
	updates := map[string]interface{}{
		"progress":        progress,
		"files_processed": sessionCtx.ProcessedFiles,
		"files_failed":    sessionCtx.FailedFiles,
		"chunks_created":  sessionCtx.TotalChunks,
	}
	sessionCtx.mutex.RUnlock()

	tenantRepo.DB().Model(&dbSession).Updates(updates)
}

// completeSession marks a session as completed
func (sm *SessionManager) completeSession(ctx context.Context, sessionCtx *SessionContext) {
	sessionCtx.mutex.Lock()
	sessionCtx.Status = models.SessionStatusCompleted
	now := time.Now()
	sessionCtx.CompletedAt = &now
	sessionCtx.mutex.Unlock()

	// Update database
	if err := sm.updateSessionStatus(ctx, sessionCtx.SessionID, models.SessionStatusCompleted); err != nil {
		// Log error in production
	}

	// Remove from active sessions after a delay to allow status queries
	go func() {
		time.Sleep(5 * time.Minute)
		sm.sessionMutex.Lock()
		delete(sm.activeSessions, sessionCtx.SessionID)
		sm.sessionMutex.Unlock()
	}()
}

// GetSessionProgress returns the current progress of a session
func (sm *SessionManager) GetSessionProgress(sessionID uuid.UUID) (*SessionProgress, error) {
	sm.sessionMutex.RLock()
	sessionCtx, exists := sm.activeSessions[sessionID]
	sm.sessionMutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	sessionCtx.mutex.RLock()
	defer sessionCtx.mutex.RUnlock()

	progress := 0.0
	completedJobs := 0
	var totalBytes int64
	var currentFile string

	for _, job := range sessionCtx.ProcessingJobs {
		if job.Status == "completed" || job.Status == "failed" {
			completedJobs++
			if job.Result != nil {
				totalBytes += job.Result.BytesProcessed
			}
		} else if job.Status == "processing" && currentFile == "" {
			currentFile = job.FileID.String()
		}
	}

	if sessionCtx.TotalFiles > 0 {
		progress = float64(completedJobs) / float64(sessionCtx.TotalFiles)
	}

	elapsedTime := time.Since(sessionCtx.StartedAt)
	var estimatedTimeLeft time.Duration
	if progress > 0 {
		totalEstimatedTime := time.Duration(float64(elapsedTime) / progress)
		estimatedTimeLeft = totalEstimatedTime - elapsedTime
	}

	return &SessionProgress{
		SessionID:         sessionCtx.SessionID,
		Status:            sessionCtx.Status,
		Progress:          progress,
		FilesTotal:        sessionCtx.TotalFiles,
		FilesProcessed:    sessionCtx.ProcessedFiles,
		FilesFailed:       sessionCtx.FailedFiles,
		ChunksCreated:     sessionCtx.TotalChunks,
		BytesProcessed:    totalBytes,
		ElapsedTime:       elapsedTime,
		EstimatedTimeLeft: estimatedTimeLeft,
		CurrentFile:       currentFile,
	}, nil
}

// StopSession stops a running session
func (sm *SessionManager) StopSession(ctx context.Context, sessionID uuid.UUID) error {
	sm.sessionMutex.RLock()
	sessionCtx, exists := sm.activeSessions[sessionID]
	sm.sessionMutex.RUnlock()

	if !exists {
		return fmt.Errorf("session %s not found", sessionID)
	}

	sessionCtx.mutex.Lock()
	if sessionCtx.Status != models.SessionStatusRunning {
		sessionCtx.mutex.Unlock()
		return fmt.Errorf("session is not running")
	}

	sessionCtx.Status = models.SessionStatusCancelled
	sessionCtx.mutex.Unlock()

	return sm.updateSessionStatus(ctx, sessionID, models.SessionStatusCancelled)
}

// updateSessionStatus updates the session status in the database
func (sm *SessionManager) updateSessionStatus(ctx context.Context, sessionID uuid.UUID, status string) error {
	// Get session to find tenant
	var dbSession models.ProcessingSession
	if err := sm.db.DB().Where("id = ?", sessionID).First(&dbSession).Error; err != nil {
		return fmt.Errorf("failed to find session: %w", err)
	}

	tenantService := sm.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(ctx, dbSession.TenantID)
	if err != nil {
		return fmt.Errorf("failed to get tenant repository: %w", err)
	}

	updates := map[string]interface{}{
		"status": status,
	}

	if status == models.SessionStatusCompleted || status == models.SessionStatusCancelled {
		now := time.Now()
		updates["completed_at"] = &now
	}

	return tenantRepo.DB().Model(&dbSession).Updates(updates).Error
}

// ListActiveSessions returns all active sessions
func (sm *SessionManager) ListActiveSessions() []*SessionContext {
	sm.sessionMutex.RLock()
	defer sm.sessionMutex.RUnlock()

	sessions := make([]*SessionContext, 0, len(sm.activeSessions))
	for _, session := range sm.activeSessions {
		// Create a copy to avoid race conditions
		sessionCopy := *session
		sessions = append(sessions, &sessionCopy)
	}

	return sessions
}