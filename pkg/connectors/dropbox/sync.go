package dropbox

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/jscharber/eAIIngest/pkg/storage"
)

// SyncManager manages synchronization with Dropbox using cursors and longpoll
type SyncManager struct {
	connector *DropboxConnector
	config    *SyncConfig
	tracer    trace.Tracer

	// Sync state management
	syncState *SyncState
	syncMu    sync.RWMutex

	// Cursor management for incremental sync
	cursorStore *CursorStore

	// Long polling for real-time changes
	longpollState *LongpollState
	longpollMu    sync.RWMutex

	// Event handlers
	eventHandlers map[string]EventHandler

	// Metrics
	metrics *SyncMetrics
}

// SyncConfig contains configuration for sync operations
type SyncConfig struct {
	// Sync intervals
	IncrementalSyncInterval time.Duration `yaml:"incremental_sync_interval"`
	FullSyncInterval        time.Duration `yaml:"full_sync_interval"`

	// Batch processing
	BatchSize          int `yaml:"batch_size"`
	MaxConcurrentSyncs int `yaml:"max_concurrent_syncs"`

	// Longpoll configuration
	EnableLongpoll     bool          `yaml:"enable_longpoll"`
	LongpollTimeout    time.Duration `yaml:"longpoll_timeout"`
	LongpollRetryDelay time.Duration `yaml:"longpoll_retry_delay"`

	// Conflict resolution
	ConflictResolution string `yaml:"conflict_resolution"` // "server_wins", "client_wins", "timestamp"

	// Feature flags
	SyncMetadata bool `yaml:"sync_metadata"`
	SyncSharing  bool `yaml:"sync_sharing"`
	SyncDeleted  bool `yaml:"sync_deleted"`
	SyncComments bool `yaml:"sync_comments"`

	// Performance tuning
	EnableDeduplication bool `yaml:"enable_deduplication"`
	EnableCompression   bool `yaml:"enable_compression"`
	EnableCaching       bool `yaml:"enable_caching"`
}

// DefaultSyncConfig returns default sync configuration
func DefaultSyncConfig() *SyncConfig {
	return &SyncConfig{
		IncrementalSyncInterval: 5 * time.Minute,
		FullSyncInterval:        24 * time.Hour,
		BatchSize:               1000,
		MaxConcurrentSyncs:      5,
		EnableLongpoll:          true,
		LongpollTimeout:         30 * time.Second,
		LongpollRetryDelay:      5 * time.Second,
		ConflictResolution:      "server_wins",
		SyncMetadata:            true,
		SyncSharing:             true,
		SyncDeleted:             false,
		SyncComments:            false,
		EnableDeduplication:     true,
		EnableCompression:       true,
		EnableCaching:           true,
	}
}

// CursorStore manages Dropbox cursors for incremental sync
type CursorStore struct {
	cursors map[string]string // path -> cursor
	mu      sync.RWMutex
}

// LongpollState manages long polling state
type LongpollState struct {
	isRunning  bool
	lastPoll   time.Time
	pollCount  int64
	errorCount int64
	lastError  string
}

// SyncMetrics tracks sync performance metrics
type SyncMetrics struct {
	TotalSyncs          int64         `json:"total_syncs"`
	IncrementalSyncs    int64         `json:"incremental_syncs"`
	FullSyncs           int64         `json:"full_syncs"`
	FilesProcessed      int64         `json:"files_processed"`
	BytesProcessed      int64         `json:"bytes_processed"`
	SyncErrors          int64         `json:"sync_errors"`
	LastSyncDuration    time.Duration `json:"last_sync_duration"`
	AverageSyncDuration time.Duration `json:"average_sync_duration"`
	LastSyncTime        time.Time     `json:"last_sync_time"`
	CursorUpdates       int64         `json:"cursor_updates"`
	LongpollEvents      int64         `json:"longpoll_events"`
}

// EventHandler defines interface for handling sync events
type EventHandler interface {
	HandleEvent(ctx context.Context, event *SyncEvent) error
}

// SyncEvent represents a synchronization event
type SyncEvent struct {
	Type      string                     `json:"type"`
	Timestamp time.Time                  `json:"timestamp"`
	Path      string                     `json:"path"`
	FileInfo  *storage.ConnectorFileInfo `json:"file_info,omitempty"`
	Error     string                     `json:"error,omitempty"`
	Metadata  map[string]interface{}     `json:"metadata,omitempty"`
}

// NewSyncManager creates a new sync manager
func NewSyncManager(connector *DropboxConnector, config *SyncConfig) *SyncManager {
	if config == nil {
		config = DefaultSyncConfig()
	}

	return &SyncManager{
		connector:     connector,
		config:        config,
		tracer:        otel.Tracer("dropbox-sync"),
		syncState:     &SyncState{},
		cursorStore:   &CursorStore{cursors: make(map[string]string)},
		longpollState: &LongpollState{},
		eventHandlers: make(map[string]EventHandler),
		metrics:       &SyncMetrics{},
	}
}

// StartSync starts the synchronization process
func (sm *SyncManager) StartSync(ctx context.Context) error {
	ctx, span := sm.tracer.Start(ctx, "start_sync")
	defer span.End()

	sm.syncMu.Lock()
	defer sm.syncMu.Unlock()

	if sm.syncState.IsRunning {
		return fmt.Errorf("sync is already running")
	}

	sm.syncState.IsRunning = true
	sm.syncState.LastSyncStart = time.Now()

	// Start incremental sync loop
	go sm.incrementalSyncLoop(ctx)

	// Start longpoll if enabled
	if sm.config.EnableLongpoll {
		go sm.longpollLoop(ctx)
	}

	span.SetAttributes(
		attribute.Bool("sync.started", true),
		attribute.Bool("longpoll.enabled", sm.config.EnableLongpoll),
	)

	return nil
}

// StopSync stops the synchronization process
func (sm *SyncManager) StopSync(ctx context.Context) error {
	ctx, span := sm.tracer.Start(ctx, "stop_sync")
	defer span.End()

	sm.syncMu.Lock()
	defer sm.syncMu.Unlock()

	sm.syncState.IsRunning = false
	sm.syncState.LastSyncEnd = time.Now()

	span.SetAttributes(attribute.Bool("sync.stopped", true))

	return nil
}

// PerformIncrementalSync performs an incremental synchronization
func (sm *SyncManager) PerformIncrementalSync(ctx context.Context, path string) (*storage.SyncResult, error) {
	ctx, span := sm.tracer.Start(ctx, "incremental_sync")
	defer span.End()

	syncStart := time.Now()
	result := &storage.SyncResult{
		StartTime:    syncStart,
		SyncType:     "incremental",
		FilesFound:   0,
		FilesChanged: 0,
		FilesDeleted: 0,
		Errors:       []string{},
	}

	span.SetAttributes(
		attribute.String("sync.type", "incremental"),
		attribute.String("sync.path", path),
	)

	// Get current cursor for the path
	cursor := sm.cursorStore.GetCursor(path)

	// If no cursor exists, get latest cursor first
	if cursor == "" {
		latestCursor, err := sm.getLatestCursor(ctx, path)
		if err != nil {
			span.RecordError(err)
			result.Errors = append(result.Errors, err.Error())
			return result, err
		}
		cursor = latestCursor
		sm.cursorStore.SetCursor(path, cursor)
	}

	// Process changes using cursor
	for {
		changes, newCursor, hasMore, err := sm.getChangesFromCursor(ctx, cursor)
		if err != nil {
			span.RecordError(err)
			result.Errors = append(result.Errors, err.Error())
			break
		}

		// Process each change
		for _, change := range changes {
			if err := sm.processChange(ctx, &change, result); err != nil {
				span.RecordError(err)
				result.Errors = append(result.Errors, err.Error())
				sm.metrics.SyncErrors++
			} else {
				result.FilesChanged++
			}
		}

		// Update cursor
		cursor = newCursor
		sm.cursorStore.SetCursor(path, cursor)
		sm.metrics.CursorUpdates++

		if !hasMore {
			break
		}
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	// Update metrics
	sm.updateSyncMetrics(result)

	span.SetAttributes(
		attribute.Int64("files.changed", result.FilesChanged),
		attribute.Float64("duration.seconds", result.Duration.Seconds()),
		attribute.Int("errors.count", len(result.Errors)),
	)

	return result, nil
}

// PerformFullSync performs a full synchronization
func (sm *SyncManager) PerformFullSync(ctx context.Context) (*storage.SyncResult, error) {
	ctx, span := sm.tracer.Start(ctx, "full_sync")
	defer span.End()

	syncStart := time.Now()
	result := &storage.SyncResult{
		StartTime:    syncStart,
		SyncType:     "full",
		FilesFound:   0,
		FilesChanged: 0,
		FilesDeleted: 0,
		Errors:       []string{},
	}

	span.SetAttributes(attribute.String("sync.type", "full"))

	// List all files recursively from root
	files, err := sm.connector.ListFiles(ctx, "", &storage.ConnectorListOptions{})
	if err != nil {
		span.RecordError(err)
		result.Errors = append(result.Errors, err.Error())
		return result, err
	}

	result.FilesFound = int64(len(files))

	// Process each file
	for _, file := range files {
		if err := sm.processFile(ctx, file, result); err != nil {
			span.RecordError(err)
			result.Errors = append(result.Errors, err.Error())
			sm.metrics.SyncErrors++
		} else {
			result.FilesChanged++
		}
	}

	// Update cursors after full sync
	if err := sm.updateAllCursors(ctx); err != nil {
		span.RecordError(err)
		result.Errors = append(result.Errors, err.Error())
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	// Update metrics
	sm.updateSyncMetrics(result)

	span.SetAttributes(
		attribute.Int64("files.found", result.FilesFound),
		attribute.Int64("files.changed", result.FilesChanged),
		attribute.Float64("duration.seconds", result.Duration.Seconds()),
		attribute.Int("errors.count", len(result.Errors)),
	)

	return result, nil
}

// GetSyncState returns current sync state
func (sm *SyncManager) GetSyncState() *SyncState {
	sm.syncMu.RLock()
	defer sm.syncMu.RUnlock()

	return &SyncState{
		IsRunning:        sm.syncState.IsRunning,
		LastSyncStart:    sm.syncState.LastSyncStart,
		LastSyncEnd:      sm.syncState.LastSyncEnd,
		LastSyncDuration: sm.syncState.LastSyncDuration,
		TotalFiles:       sm.syncState.TotalFiles,
		ChangedFiles:     sm.syncState.ChangedFiles,
		ErrorCount:       sm.syncState.ErrorCount,
		LastError:        sm.syncState.LastError,
		Cursor:           sm.syncState.Cursor,
	}
}

// GetMetrics returns sync metrics
func (sm *SyncManager) GetMetrics() *SyncMetrics {
	return &SyncMetrics{
		TotalSyncs:          sm.metrics.TotalSyncs,
		IncrementalSyncs:    sm.metrics.IncrementalSyncs,
		FullSyncs:           sm.metrics.FullSyncs,
		FilesProcessed:      sm.metrics.FilesProcessed,
		BytesProcessed:      sm.metrics.BytesProcessed,
		SyncErrors:          sm.metrics.SyncErrors,
		LastSyncDuration:    sm.metrics.LastSyncDuration,
		AverageSyncDuration: sm.metrics.AverageSyncDuration,
		LastSyncTime:        sm.metrics.LastSyncTime,
		CursorUpdates:       sm.metrics.CursorUpdates,
		LongpollEvents:      sm.metrics.LongpollEvents,
	}
}

// AddEventHandler adds an event handler
func (sm *SyncManager) AddEventHandler(eventType string, handler EventHandler) {
	sm.eventHandlers[eventType] = handler
}

// RemoveEventHandler removes an event handler
func (sm *SyncManager) RemoveEventHandler(eventType string) {
	delete(sm.eventHandlers, eventType)
}

// Private methods

func (sm *SyncManager) incrementalSyncLoop(ctx context.Context) {
	ticker := time.NewTicker(sm.config.IncrementalSyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !sm.syncState.IsRunning {
				return
			}

			// Perform incremental sync for root path
			_, err := sm.PerformIncrementalSync(ctx, "")
			if err != nil {
				sm.syncState.ErrorCount++
				sm.syncState.LastError = err.Error()
			}
		}
	}
}

func (sm *SyncManager) longpollLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			if !sm.syncState.IsRunning {
				return
			}

			sm.performLongpoll(ctx)
		}
	}
}

func (sm *SyncManager) performLongpoll(ctx context.Context) {
	sm.longpollMu.Lock()
	sm.longpollState.isRunning = true
	sm.longpollState.lastPoll = time.Now()
	sm.longpollMu.Unlock()

	defer func() {
		sm.longpollMu.Lock()
		sm.longpollState.isRunning = false
		sm.longpollMu.Unlock()
	}()

	// Get latest cursor for longpoll
	cursor := sm.cursorStore.GetCursor("")
	if cursor == "" {
		// Get initial cursor
		latestCursor, err := sm.getLatestCursor(ctx, "")
		if err != nil {
			sm.longpollState.errorCount++
			sm.longpollState.lastError = err.Error()
			time.Sleep(sm.config.LongpollRetryDelay)
			return
		}
		cursor = latestCursor
		sm.cursorStore.SetCursor("", cursor)
	}

	// Perform longpoll request
	hasChanges, err := sm.performLongpollRequest(ctx, cursor)
	if err != nil {
		sm.longpollState.errorCount++
		sm.longpollState.lastError = err.Error()
		time.Sleep(sm.config.LongpollRetryDelay)
		return
	}

	sm.longpollState.pollCount++

	// If changes detected, trigger incremental sync
	if hasChanges {
		sm.metrics.LongpollEvents++
		go func() {
			_, err := sm.PerformIncrementalSync(ctx, "")
			if err != nil {
				sm.syncState.ErrorCount++
				sm.syncState.LastError = err.Error()
			}
		}()
	}
}

func (sm *SyncManager) getLatestCursor(ctx context.Context, path string) (string, error) {
	requestBody := &GetLatestCursorRequest{
		Path:                            sm.connector.normalizePath(path),
		Recursive:                       sm.config.SyncMetadata,
		IncludeMediaInfo:                sm.connector.config.IncludeMediaInfo,
		IncludeDeleted:                  sm.config.SyncDeleted,
		IncludeHasExplicitSharedMembers: sm.config.SyncSharing,
		IncludeMountedFolders:           true,
		IncludeNonDownloadableFiles:     true,
	}

	response, err := sm.connector.executeDropboxAPICall(ctx,
		"https://api.dropboxapi.com/2/files/list_folder/get_latest_cursor", requestBody)
	if err != nil {
		return "", fmt.Errorf("failed to get latest cursor: %w", err)
	}

	var cursorResponse GetLatestCursorResponse
	if err := json.Unmarshal(response, &cursorResponse); err != nil {
		return "", fmt.Errorf("failed to parse cursor response: %w", err)
	}

	return cursorResponse.Cursor, nil
}

func (sm *SyncManager) getChangesFromCursor(ctx context.Context, cursor string) ([]DropboxEntry, string, bool, error) {
	requestBody := &ListFolderContinueRequest{
		Cursor: cursor,
	}

	response, err := sm.connector.executeDropboxAPICall(ctx,
		"https://api.dropboxapi.com/2/files/list_folder/continue", requestBody)
	if err != nil {
		return nil, "", false, fmt.Errorf("failed to get changes: %w", err)
	}

	var listResponse ListFolderResponse
	if err := json.Unmarshal(response, &listResponse); err != nil {
		return nil, "", false, fmt.Errorf("failed to parse changes response: %w", err)
	}

	return listResponse.Entries, listResponse.Cursor, listResponse.HasMore, nil
}

func (sm *SyncManager) performLongpollRequest(ctx context.Context, cursor string) (bool, error) {
	requestBody := &LongpollRequest{
		Cursor:  cursor,
		Timeout: uint64(sm.config.LongpollTimeout.Seconds()),
	}

	response, err := sm.connector.executeDropboxAPICall(ctx,
		"https://notify.dropboxapi.com/2/files/list_folder/longpoll", requestBody)
	if err != nil {
		return false, fmt.Errorf("longpoll request failed: %w", err)
	}

	var longpollResponse LongpollResponse
	if err := json.Unmarshal(response, &longpollResponse); err != nil {
		return false, fmt.Errorf("failed to parse longpoll response: %w", err)
	}

	// Handle backoff if requested
	if longpollResponse.Backoff > 0 {
		time.Sleep(time.Duration(longpollResponse.Backoff) * time.Second)
	}

	return longpollResponse.Changes, nil
}

func (sm *SyncManager) processChange(ctx context.Context, entry *DropboxEntry, result *storage.SyncResult) error {
	// Convert to file info
	fileInfo := sm.connector.convertToFileInfo(entry)

	// Emit sync event
	event := &SyncEvent{
		Type:      "file_change",
		Timestamp: time.Now(),
		Path:      entry.PathLower,
		FileInfo:  fileInfo,
		Metadata: map[string]interface{}{
			"tag":          entry.Tag,
			"revision":     entry.Rev,
			"content_hash": entry.ContentHash,
		},
	}

	sm.emitEvent(ctx, event)

	// Handle different change types
	switch entry.Tag {
	case "file":
		return sm.processFileChange(ctx, entry, result)
	case "folder":
		return sm.processFolderChange(ctx, entry, result)
	case "deleted":
		return sm.processDeletedChange(ctx, entry, result)
	default:
		return fmt.Errorf("unknown entry tag: %s", entry.Tag)
	}
}

func (sm *SyncManager) processFile(ctx context.Context, file *storage.ConnectorFileInfo, result *storage.SyncResult) error {
	// Process file for full sync
	sm.metrics.FilesProcessed++
	sm.metrics.BytesProcessed += file.Size

	// Emit sync event
	event := &SyncEvent{
		Type:      "file_processed",
		Timestamp: time.Now(),
		Path:      file.Path,
		FileInfo:  file,
	}

	sm.emitEvent(ctx, event)

	return nil
}

func (sm *SyncManager) processFileChange(ctx context.Context, entry *DropboxEntry, result *storage.SyncResult) error {
	// Handle file change
	sm.metrics.FilesProcessed++
	sm.metrics.BytesProcessed += entry.Size
	return nil
}

func (sm *SyncManager) processFolderChange(ctx context.Context, entry *DropboxEntry, result *storage.SyncResult) error {
	// Handle folder change
	return nil
}

func (sm *SyncManager) processDeletedChange(ctx context.Context, entry *DropboxEntry, result *storage.SyncResult) error {
	// Handle deleted file/folder
	if sm.config.SyncDeleted {
		result.FilesDeleted++
	}
	return nil
}

func (sm *SyncManager) updateAllCursors(ctx context.Context) error {
	// Update cursor for root path
	cursor, err := sm.getLatestCursor(ctx, "")
	if err != nil {
		return err
	}
	sm.cursorStore.SetCursor("", cursor)
	return nil
}

func (sm *SyncManager) updateSyncMetrics(result *storage.SyncResult) {
	sm.metrics.TotalSyncs++
	sm.metrics.LastSyncDuration = result.Duration
	sm.metrics.LastSyncTime = result.StartTime

	if result.SyncType == "incremental" {
		sm.metrics.IncrementalSyncs++
	} else {
		sm.metrics.FullSyncs++
	}

	// Calculate average duration
	if sm.metrics.TotalSyncs > 0 {
		totalDuration := time.Duration(sm.metrics.TotalSyncs)*sm.metrics.AverageSyncDuration + result.Duration
		sm.metrics.AverageSyncDuration = totalDuration / time.Duration(sm.metrics.TotalSyncs+1)
	}
}

func (sm *SyncManager) emitEvent(ctx context.Context, event *SyncEvent) {
	if handler, exists := sm.eventHandlers[event.Type]; exists {
		go func() {
			if err := handler.HandleEvent(ctx, event); err != nil {
				// Log error but don't fail the sync
			}
		}()
	}
}

// CursorStore methods

func (cs *CursorStore) GetCursor(path string) string {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.cursors[path]
}

func (cs *CursorStore) SetCursor(path, cursor string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.cursors[path] = cursor
}

func (cs *CursorStore) DeleteCursor(path string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	delete(cs.cursors, path)
}

func (cs *CursorStore) GetAllCursors() map[string]string {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	result := make(map[string]string)
	for k, v := range cs.cursors {
		result[k] = v
	}
	return result
}
