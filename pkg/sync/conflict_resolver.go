package sync

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// ConflictResolver handles sync conflicts across connectors
type ConflictResolver struct {
	config *ConflictResolverConfig
	tracer trace.Tracer

	// Active conflict tracking
	activeConflicts map[string]*SyncConflict

	// Resolution strategies
	strategies map[ConflictStrategy]ConflictResolutionStrategy
}

// ConflictResolverConfig contains conflict resolution configuration
type ConflictResolverConfig struct {
	DefaultStrategy         ConflictStrategy `json:"default_strategy"`
	AutoResolveTimeout      time.Duration    `json:"auto_resolve_timeout"`
	MaxConflictAge          time.Duration    `json:"max_conflict_age"`
	EnableVersioning        bool             `json:"enable_versioning"`
	VersioningSuffix        string           `json:"versioning_suffix"`
	ConflictNotificationURL string           `json:"conflict_notification_url"`
	EnableManualResolution  bool             `json:"enable_manual_resolution"`
	BackupConflictedFiles   bool             `json:"backup_conflicted_files"`
	ConflictBackupPath      string           `json:"conflict_backup_path"`
}

// ConflictStrategy defines how conflicts should be resolved
type ConflictStrategy string

const (
	ConflictStrategyLastWrite      ConflictStrategy = "last_write"      // Use most recently modified version
	ConflictStrategyFirstWrite     ConflictStrategy = "first_write"     // Use original version
	ConflictStrategyLargestFile    ConflictStrategy = "largest_file"    // Use largest file
	ConflictStrategySourcePriority ConflictStrategy = "source_priority" // Use based on source priority
	ConflictStrategyManual         ConflictStrategy = "manual"          // Require manual resolution
	ConflictStrategyMerge          ConflictStrategy = "merge"           // Attempt to merge changes
	ConflictStrategyPreserveBoth   ConflictStrategy = "preserve_both"   // Keep both versions
	ConflictStrategySkip           ConflictStrategy = "skip"            // Skip conflicted files
)

// SyncConflict represents a synchronization conflict
type SyncConflict struct {
	ID                  uuid.UUID              `json:"id"`
	ConflictType        ConflictType           `json:"conflict_type"`
	FilePath            string                 `json:"file_path"`
	DetectedAt          time.Time              `json:"detected_at"`
	ResolvedAt          *time.Time             `json:"resolved_at,omitempty"`
	Resolution          ConflictStrategy       `json:"resolution"`
	Status              ConflictStatus         `json:"status"`
	InvolvedDataSources []uuid.UUID            `json:"involved_data_sources"`
	FileVersions        []*ConflictFileVersion `json:"file_versions"`
	ResolvedVersion     *ConflictFileVersion   `json:"resolved_version,omitempty"`
	ResolutionMetadata  map[string]interface{} `json:"resolution_metadata,omitempty"`
	AutoResolvable      bool                   `json:"auto_resolvable"`
	RequiresUserInput   bool                   `json:"requires_user_input"`
	Severity            ConflictSeverity       `json:"severity"`
	Description         string                 `json:"description"`
	SuggestedResolution ConflictStrategy       `json:"suggested_resolution"`
}

// ConflictType defines types of sync conflicts
type ConflictType string

const (
	ConflictTypeModifyModify       ConflictType = "modify_modify"       // Same file modified in multiple sources
	ConflictTypeModifyDelete       ConflictType = "modify_delete"       // File modified in one source, deleted in another
	ConflictTypeCreateCreate       ConflictType = "create_create"       // Same file created in multiple sources with different content
	ConflictTypeRenameRename       ConflictType = "rename_rename"       // File renamed differently in multiple sources
	ConflictTypeRenameModify       ConflictType = "rename_modify"       // File renamed in one source, modified in another
	ConflictTypeDirectoryFile      ConflictType = "directory_file"      // Directory in one source, file in another
	ConflictTypePermissionConflict ConflictType = "permission_conflict" // Permission changes conflict
	ConflictTypeMetadataConflict   ConflictType = "metadata_conflict"   // Metadata conflicts (tags, properties, etc.)
	ConflictTypeContentConflict    ConflictType = "content_conflict"    // Content merge conflicts
	ConflictTypeCyclicMove         ConflictType = "cyclic_move"         // Cyclic move operations
	ConflictTypeModified           ConflictType = "modified"            // General modification conflict
	ConflictTypeDeleted            ConflictType = "deleted"             // General deletion conflict
)

// ConflictStatus represents the current state of a conflict
type ConflictStatus string

const (
	ConflictStatusDetected      ConflictStatus = "detected"
	ConflictStatusAnalyzing     ConflictStatus = "analyzing"
	ConflictStatusPending       ConflictStatus = "pending"
	ConflictStatusPendingReview ConflictStatus = "pending_review"
	ConflictStatusResolving     ConflictStatus = "resolving"
	ConflictStatusResolved      ConflictStatus = "resolved"
	ConflictStatusFailed        ConflictStatus = "failed"
	ConflictStatusSkipped       ConflictStatus = "skipped"
)

// ConflictSeverity indicates the impact level of a conflict
type ConflictSeverity string

const (
	ConflictSeverityLow      ConflictSeverity = "low"      // Minor conflicts, auto-resolvable
	ConflictSeverityMedium   ConflictSeverity = "medium"   // Moderate conflicts, may need user input
	ConflictSeverityHigh     ConflictSeverity = "high"     // Major conflicts, definitely need user input
	ConflictSeverityCritical ConflictSeverity = "critical" // Critical conflicts, may cause data loss
)

// ConflictResolutionStrategy defines how to resolve a specific type of conflict
type ConflictResolutionStrategy interface {
	CanResolve(conflict *SyncConflict) bool
	Resolve(ctx context.Context, conflict *SyncConflict) (*ConflictResolution, error)
	GetPriority() int
}

// ConflictResolution represents the result of resolving a conflict
type ConflictResolution struct {
	ResolvedVersion    *ConflictFileVersion       `json:"resolved_version"`
	AdditionalVersions []*ConflictFileVersion     `json:"additional_versions,omitempty"`
	RequiresSync       bool                       `json:"requires_sync"`
	Actions            []ConflictResolutionAction `json:"actions"`
	Metadata           map[string]interface{}     `json:"metadata"`
}

// ConflictResolutionAction represents an action taken during conflict resolution
type ConflictResolutionAction struct {
	Type        string                 `json:"type"`   // create, update, delete, rename, backup
	Source      string                 `json:"source"` // data source or system
	Target      string                 `json:"target"` // file path or destination
	Description string                 `json:"description"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// NewConflictResolver creates a new conflict resolver

func NewConflictResolver(config *ConflictResolverConfig) *ConflictResolver {
	if config == nil {
		config = &ConflictResolverConfig{
			DefaultStrategy:        ConflictStrategyLastWrite,
			AutoResolveTimeout:     30 * time.Minute,
			MaxConflictAge:         7 * 24 * time.Hour,
			EnableVersioning:       true,
			VersioningSuffix:       "_v{version}",
			EnableManualResolution: true,
			BackupConflictedFiles:  true,
			ConflictBackupPath:     "/backups/conflicts",
		}
	}

	resolver := &ConflictResolver{
		config:          config,
		tracer:          otel.Tracer("conflict-resolver"),
		activeConflicts: make(map[string]*SyncConflict),
		strategies:      make(map[ConflictStrategy]ConflictResolutionStrategy),
	}

	// Register default resolution strategies
	resolver.registerDefaultStrategies()

	return resolver
}

// DetectConflict analyzes file versions to detect conflicts
func (cr *ConflictResolver) DetectConflict(ctx context.Context, filePath string, versions []*ConflictFileVersion) (*SyncConflict, error) {
	ctx, span := cr.tracer.Start(ctx, "conflict_resolver.detect_conflict")
	defer span.End()

	span.SetAttributes(
		attribute.String("file_path", filePath),
		attribute.Int("versions_count", len(versions)),
	)

	if len(versions) < 2 {
		return nil, nil // No conflict if less than 2 versions
	}

	// Analyze versions to determine conflict type
	conflictType := cr.analyzeConflictType(versions)
	if conflictType == "" {
		return nil, nil // No conflict detected
	}

	// Create conflict record
	conflict := &SyncConflict{
		ID:                  uuid.New(),
		ConflictType:        conflictType,
		FilePath:            filePath,
		DetectedAt:          time.Now(),
		Status:              ConflictStatusDetected,
		FileVersions:        versions,
		AutoResolvable:      cr.isAutoResolvable(conflictType),
		Severity:            cr.calculateSeverity(conflictType, versions),
		SuggestedResolution: cr.suggestResolution(conflictType, versions),
	}

	// Extract involved data sources
	for _, version := range versions {
		conflict.InvolvedDataSources = append(conflict.InvolvedDataSources, version.DataSourceID)
	}

	// Generate description
	conflict.Description = cr.generateConflictDescription(conflict)

	// Set user input requirement
	conflict.RequiresUserInput = !conflict.AutoResolvable || conflict.Severity == ConflictSeverityHigh

	// Store active conflict
	cr.activeConflicts[conflict.ID.String()] = conflict

	span.SetAttributes(
		attribute.String("conflict.id", conflict.ID.String()),
		attribute.String("conflict.type", string(conflictType)),
		attribute.String("conflict.severity", string(conflict.Severity)),
		attribute.Bool("auto_resolvable", conflict.AutoResolvable),
	)

	return conflict, nil
}

// ResolveConflict resolves a conflict using the specified strategy
func (cr *ConflictResolver) ResolveConflict(ctx context.Context, conflict *SyncConflict, strategy ConflictStrategy) (*ConflictResolution, error) {
	ctx, span := cr.tracer.Start(ctx, "conflict_resolver.resolve_conflict")
	defer span.End()

	span.SetAttributes(
		attribute.String("conflict.id", conflict.ID.String()),
		attribute.String("strategy", string(strategy)),
	)

	conflict.Status = ConflictStatusResolving
	conflict.Resolution = strategy

	// Get resolution strategy implementation
	strategyImpl, exists := cr.strategies[strategy]
	if !exists {
		err := fmt.Errorf("resolution strategy %s not supported", strategy)
		span.RecordError(err)
		conflict.Status = ConflictStatusFailed
		return nil, err
	}

	// Check if strategy can resolve this conflict
	if !strategyImpl.CanResolve(conflict) {
		err := fmt.Errorf("strategy %s cannot resolve conflict type %s", strategy, conflict.ConflictType)
		span.RecordError(err)
		conflict.Status = ConflictStatusFailed
		return nil, err
	}

	// Execute resolution
	resolution, err := strategyImpl.Resolve(ctx, conflict)
	if err != nil {
		span.RecordError(err)
		conflict.Status = ConflictStatusFailed
		return nil, fmt.Errorf("conflict resolution failed: %w", err)
	}

	// Update conflict record
	now := time.Now()
	conflict.ResolvedAt = &now
	conflict.Status = ConflictStatusResolved
	conflict.ResolvedVersion = resolution.ResolvedVersion
	conflict.ResolutionMetadata = resolution.Metadata

	span.SetAttributes(
		attribute.Bool("resolution.requires_sync", resolution.RequiresSync),
		attribute.Int("resolution.actions_count", len(resolution.Actions)),
	)

	return resolution, nil
}

// GetConflict retrieves a conflict by ID
func (cr *ConflictResolver) GetConflict(ctx context.Context, conflictID uuid.UUID) (*SyncConflict, error) {
	ctx, span := cr.tracer.Start(ctx, "conflict_resolver.get_conflict")
	defer span.End()

	span.SetAttributes(attribute.String("conflict.id", conflictID.String()))

	conflict, exists := cr.activeConflicts[conflictID.String()]
	if !exists {
		return nil, fmt.Errorf("conflict not found")
	}

	return conflict, nil
}

// ListConflicts returns all conflicts with optional filtering
func (cr *ConflictResolver) ListConflicts(ctx context.Context, filters *ConflictFilters) ([]*SyncConflict, error) {
	ctx, span := cr.tracer.Start(ctx, "conflict_resolver.list_conflicts")
	defer span.End()

	var conflicts []*SyncConflict

	for _, conflict := range cr.activeConflicts {
		// Apply filters
		if filters != nil {
			if filters.Status != "" && conflict.Status != filters.Status {
				continue
			}
			if filters.ConflictType != "" && conflict.ConflictType != filters.ConflictType {
				continue
			}
			if filters.Severity != "" && conflict.Severity != filters.Severity {
				continue
			}
			if filters.DataSourceID != uuid.Nil {
				found := false
				for _, dsID := range conflict.InvolvedDataSources {
					if dsID == filters.DataSourceID {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}
			if filters.Since != nil && conflict.DetectedAt.Before(*filters.Since) {
				continue
			}
		}

		conflicts = append(conflicts, conflict)
	}

	span.SetAttributes(attribute.Int("conflicts_count", len(conflicts)))

	return conflicts, nil
}

// Internal methods

func (cr *ConflictResolver) analyzeConflictType(versions []*ConflictFileVersion) ConflictType {
	if len(versions) < 2 {
		return ""
	}

	// Check for modify-modify conflicts
	hasModifications := 0
	hasDeletions := 0
	hasCreations := 0

	for _, version := range versions {
		if version.IsDeleted {
			hasDeletions++
		} else if version.ModifiedTime.IsZero() {
			hasCreations++
		} else {
			hasModifications++
		}
	}

	// Determine conflict type based on operations
	if hasModifications > 1 {
		return ConflictTypeModifyModify
	}
	if hasModifications > 0 && hasDeletions > 0 {
		return ConflictTypeModifyDelete
	}
	if hasCreations > 1 {
		// Check if content is different
		if cr.hasContentConflicts(versions) {
			return ConflictTypeCreateCreate
		}
	}

	// Check for file/directory conflicts
	hasFiles := 0
	hasDirectories := 0
	for _, version := range versions {
		if version.IsDirectory {
			hasDirectories++
		} else {
			hasFiles++
		}
	}
	if hasFiles > 0 && hasDirectories > 0 {
		return ConflictTypeDirectoryFile
	}

	// Check for rename conflicts
	if cr.hasRenameConflicts(versions) {
		return ConflictTypeRenameRename
	}

	return ConflictTypeModifyModify // Default fallback
}

func (cr *ConflictResolver) hasContentConflicts(versions []*ConflictFileVersion) bool {
	if len(versions) < 2 {
		return false
	}

	// Compare checksums if available
	checksums := make(map[string]bool)
	for _, version := range versions {
		if version.Checksum != "" {
			checksums[version.Checksum] = true
		}
	}

	// If we have multiple different checksums, there's a content conflict
	return len(checksums) > 1
}

func (cr *ConflictResolver) hasRenameConflicts(versions []*ConflictFileVersion) bool {
	if len(versions) < 2 {
		return false
	}

	// Check if same file has different names across versions
	filenames := make(map[string]bool)
	for _, version := range versions {
		filename := filepath.Base(version.FilePath)
		filenames[filename] = true
	}

	return len(filenames) > 1
}

func (cr *ConflictResolver) isAutoResolvable(conflictType ConflictType) bool {
	switch conflictType {
	case ConflictTypeModifyModify:
		return true // Can use timestamp-based resolution
	case ConflictTypeModifyDelete:
		return false // Requires user decision
	case ConflictTypeCreateCreate:
		return true // Can use various strategies
	case ConflictTypeDirectoryFile:
		return false // Complex conflict requiring user input
	case ConflictTypeRenameRename:
		return false // Usually requires user decision
	default:
		return false
	}
}

func (cr *ConflictResolver) calculateSeverity(conflictType ConflictType, versions []*ConflictFileVersion) ConflictSeverity {
	switch conflictType {
	case ConflictTypeModifyDelete:
		return ConflictSeverityHigh // Risk of data loss
	case ConflictTypeDirectoryFile:
		return ConflictSeverityHigh // Structural changes
	case ConflictTypeCyclicMove:
		return ConflictSeverityCritical // Can break sync entirely
	case ConflictTypeModifyModify:
		// Check if files are significantly different
		if cr.hasSignificantChanges(versions) {
			return ConflictSeverityMedium
		}
		return ConflictSeverityLow
	default:
		return ConflictSeverityMedium
	}
}

func (cr *ConflictResolver) hasSignificantChanges(versions []*ConflictFileVersion) bool {
	if len(versions) < 2 {
		return false
	}

	// Compare file sizes
	for i := 1; i < len(versions); i++ {
		sizeDiff := versions[i].FileSize - versions[0].FileSize
		if sizeDiff < 0 {
			sizeDiff = -sizeDiff
		}

		// Consider >10% size change as significant
		if float64(sizeDiff)/float64(versions[0].FileSize) > 0.1 {
			return true
		}
	}

	return false
}

func (cr *ConflictResolver) suggestResolution(conflictType ConflictType, versions []*ConflictFileVersion) ConflictStrategy {
	switch conflictType {
	case ConflictTypeModifyModify:
		return ConflictStrategyLastWrite
	case ConflictTypeModifyDelete:
		return ConflictStrategyManual
	case ConflictTypeCreateCreate:
		return ConflictStrategyPreserveBoth
	case ConflictTypeDirectoryFile:
		return ConflictStrategyManual
	case ConflictTypeRenameRename:
		return ConflictStrategyManual
	default:
		return cr.config.DefaultStrategy
	}
}

func (cr *ConflictResolver) generateConflictDescription(conflict *SyncConflict) string {
	switch conflict.ConflictType {
	case ConflictTypeModifyModify:
		return fmt.Sprintf("File '%s' was modified in %d different data sources",
			conflict.FilePath, len(conflict.FileVersions))
	case ConflictTypeModifyDelete:
		return fmt.Sprintf("File '%s' was modified in one source and deleted in another",
			conflict.FilePath)
	case ConflictTypeCreateCreate:
		return fmt.Sprintf("File '%s' was created with different content in multiple sources",
			conflict.FilePath)
	case ConflictTypeDirectoryFile:
		return fmt.Sprintf("Path '%s' is a directory in one source and a file in another",
			conflict.FilePath)
	case ConflictTypeRenameRename:
		return fmt.Sprintf("File was renamed differently in multiple sources")
	default:
		return fmt.Sprintf("Sync conflict detected for '%s'", conflict.FilePath)
	}
}

func (cr *ConflictResolver) registerDefaultStrategies() {
	cr.strategies[ConflictStrategyLastWrite] = &LastWriteStrategy{}
	cr.strategies[ConflictStrategyFirstWrite] = &FirstWriteStrategy{}
	cr.strategies[ConflictStrategyLargestFile] = &LargestFileStrategy{}
	cr.strategies[ConflictStrategyPreserveBoth] = &PreserveBothStrategy{config: cr.config}
	cr.strategies[ConflictStrategySourcePriority] = &SourcePriorityStrategy{}
	cr.strategies[ConflictStrategySkip] = &SkipStrategy{}
}

// CleanupOldConflicts removes resolved conflicts older than the configured age
func (cr *ConflictResolver) CleanupOldConflicts(ctx context.Context) error {
	ctx, span := cr.tracer.Start(ctx, "conflict_resolver.cleanup_old_conflicts")
	defer span.End()

	cutoff := time.Now().Add(-cr.config.MaxConflictAge)
	var cleanedCount int

	for id, conflict := range cr.activeConflicts {
		if conflict.Status == ConflictStatusResolved &&
			conflict.ResolvedAt != nil &&
			conflict.ResolvedAt.Before(cutoff) {
			delete(cr.activeConflicts, id)
			cleanedCount++
		}
	}

	span.SetAttributes(attribute.Int("cleaned_conflicts", cleanedCount))

	return nil
}

// ConflictFilters contains filters for listing conflicts
type ConflictFilters struct {
	Status       ConflictStatus   `json:"status,omitempty"`
	ConflictType ConflictType     `json:"conflict_type,omitempty"`
	Severity     ConflictSeverity `json:"severity,omitempty"`
	DataSourceID uuid.UUID        `json:"data_source_id,omitempty"`
	Since        *time.Time       `json:"since,omitempty"`
}

// Shutdown gracefully shuts down the conflict resolver
func (cr *ConflictResolver) Shutdown(ctx context.Context) error {
	// In a production system, this would persist unresolved conflicts
	return nil
}
