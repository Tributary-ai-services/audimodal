package sync

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// LastWriteStrategy resolves conflicts by choosing the most recently modified version
type LastWriteStrategy struct{}

func (s *LastWriteStrategy) CanResolve(conflict *SyncConflict) bool {
	// Can resolve any conflict where we have modification times
	for _, version := range conflict.FileVersions {
		if version.ModifiedTime.IsZero() {
			return false // Can't resolve without timestamps
		}
	}
	return true
}

func (s *LastWriteStrategy) Resolve(ctx context.Context, conflict *SyncConflict) (*ConflictResolution, error) {
	if len(conflict.FileVersions) == 0 {
		return nil, fmt.Errorf("no file versions to resolve")
	}

	// Find the version with the latest modification time
	var latestVersion *ConflictFileVersion
	latestTime := time.Time{}

	for _, version := range conflict.FileVersions {
		if version.ModifiedTime.After(latestTime) {
			latestTime = version.ModifiedTime
			latestVersion = version
		}
	}

	if latestVersion == nil {
		return nil, fmt.Errorf("could not determine latest version")
	}

	resolution := &ConflictResolution{
		ResolvedVersion: latestVersion,
		RequiresSync:    true,
		Actions: []ConflictResolutionAction{
			{
				Type:        "update",
				Source:      latestVersion.ConnectorType,
				Target:      conflict.FilePath,
				Description: fmt.Sprintf("Updated to latest version from %s (modified: %s)", 
					latestVersion.ConnectorType, latestVersion.ModifiedTime.Format(time.RFC3339)),
			},
		},
		Metadata: map[string]interface{}{
			"strategy":        "last_write",
			"chosen_version":  latestVersion.DataSourceID.String(),
			"modified_time":   latestVersion.ModifiedTime,
			"rejected_count":  len(conflict.FileVersions) - 1,
		},
	}

	return resolution, nil
}

func (s *LastWriteStrategy) GetPriority() int {
	return 100 // High priority, commonly used
}

// FirstWriteStrategy resolves conflicts by choosing the original version
type FirstWriteStrategy struct{}

func (s *FirstWriteStrategy) CanResolve(conflict *SyncConflict) bool {
	// Can resolve any conflict where we have creation/modification times
	for _, version := range conflict.FileVersions {
		if version.ModifiedTime.IsZero() && version.CreatedTime.IsZero() {
			return false
		}
	}
	return true
}

func (s *FirstWriteStrategy) Resolve(ctx context.Context, conflict *SyncConflict) (*ConflictResolution, error) {
	if len(conflict.FileVersions) == 0 {
		return nil, fmt.Errorf("no file versions to resolve")
	}

	// Find the version with the earliest creation or modification time
	var earliestVersion *ConflictFileVersion
	earliestTime := time.Now()

	for _, version := range conflict.FileVersions {
		var versionTime time.Time
		if !version.CreatedTime.IsZero() {
			versionTime = version.CreatedTime
		} else {
			versionTime = version.ModifiedTime
		}

		if versionTime.Before(earliestTime) {
			earliestTime = versionTime
			earliestVersion = version
		}
	}

	if earliestVersion == nil {
		return nil, fmt.Errorf("could not determine earliest version")
	}

	resolution := &ConflictResolution{
		ResolvedVersion: earliestVersion,
		RequiresSync:    true,
		Actions: []ConflictResolutionAction{
			{
				Type:        "revert",
				Source:      earliestVersion.ConnectorType,
				Target:      conflict.FilePath,
				Description: fmt.Sprintf("Reverted to original version from %s", earliestVersion.ConnectorType),
			},
		},
		Metadata: map[string]interface{}{
			"strategy":       "first_write",
			"chosen_version": earliestVersion.DataSourceID.String(),
			"creation_time":  earliestVersion.CreatedTime,
		},
	}

	return resolution, nil
}

func (s *FirstWriteStrategy) GetPriority() int {
	return 80
}

// LargestFileStrategy resolves conflicts by choosing the largest file
type LargestFileStrategy struct{}

func (s *LargestFileStrategy) CanResolve(conflict *SyncConflict) bool {
	// Can resolve conflicts involving files (not directories)
	for _, version := range conflict.FileVersions {
		if version.IsDirectory {
			return false
		}
	}
	return true
}

func (s *LargestFileStrategy) Resolve(ctx context.Context, conflict *SyncConflict) (*ConflictResolution, error) {
	if len(conflict.FileVersions) == 0 {
		return nil, fmt.Errorf("no file versions to resolve")
	}

	// Find the version with the largest file size
	var largestVersion *ConflictFileVersion
	largestSize := int64(0)

	for _, version := range conflict.FileVersions {
		if version.FileSize > largestSize {
			largestSize = version.FileSize
			largestVersion = version
		}
	}

	if largestVersion == nil {
		return nil, fmt.Errorf("could not determine largest version")
	}

	resolution := &ConflictResolution{
		ResolvedVersion: largestVersion,
		RequiresSync:    true,
		Actions: []ConflictResolutionAction{
			{
				Type:        "update",
				Source:      largestVersion.ConnectorType,
				Target:      conflict.FilePath,
				Description: fmt.Sprintf("Updated to largest version from %s (%d bytes)", 
					largestVersion.ConnectorType, largestVersion.FileSize),
			},
		},
		Metadata: map[string]interface{}{
			"strategy":      "largest_file",
			"chosen_version": largestVersion.DataSourceID.String(),
			"file_size":     largestVersion.FileSize,
		},
	}

	return resolution, nil
}

func (s *LargestFileStrategy) GetPriority() int {
	return 60
}

// PreserveBothStrategy resolves conflicts by keeping all versions with different names
type PreserveBothStrategy struct {
	config *ConflictResolverConfig
}

func (s *PreserveBothStrategy) CanResolve(conflict *SyncConflict) bool {
	// Can resolve most conflicts by preserving all versions
	return conflict.ConflictType != ConflictTypeDirectoryFile
}

func (s *PreserveBothStrategy) Resolve(ctx context.Context, conflict *SyncConflict) (*ConflictResolution, error) {
	if len(conflict.FileVersions) == 0 {
		return nil, fmt.Errorf("no file versions to resolve")
	}

	// Choose the first version as the "main" version
	mainVersion := conflict.FileVersions[0]
	
	var additionalVersions []*ConflictFileVersion
	var actions []ConflictResolutionAction

	// Create versioned names for additional versions
	baseName := strings.TrimSuffix(conflict.FilePath, filepath.Ext(conflict.FilePath))
	extension := filepath.Ext(conflict.FilePath)

	for i, version := range conflict.FileVersions {
		if i == 0 {
			continue // Skip the main version
		}

		// Generate versioned filename
		versionedName := fmt.Sprintf("%s_%s_%d%s", 
			baseName, 
			version.ConnectorType, 
			i, 
			extension)

		// Create a new version record with the versioned name
		versionedVersion := *version
		versionedVersion.FilePath = versionedName

		additionalVersions = append(additionalVersions, &versionedVersion)

		actions = append(actions, ConflictResolutionAction{
			Type:        "create",
			Source:      version.ConnectorType,
			Target:      versionedName,
			Description: fmt.Sprintf("Created versioned copy: %s", versionedName),
			Metadata: map[string]interface{}{
				"original_path": conflict.FilePath,
				"source_id":     version.DataSourceID.String(),
			},
		})
	}

	// Add action for the main version
	actions = append(actions, ConflictResolutionAction{
		Type:        "update",
		Source:      mainVersion.ConnectorType,
		Target:      conflict.FilePath,
		Description: fmt.Sprintf("Kept original path: %s", conflict.FilePath),
	})

	resolution := &ConflictResolution{
		ResolvedVersion:    mainVersion,
		AdditionalVersions: additionalVersions,
		RequiresSync:       true,
		Actions:            actions,
		Metadata: map[string]interface{}{
			"strategy":         "preserve_both",
			"main_version":     mainVersion.DataSourceID.String(),
			"additional_count": len(additionalVersions),
		},
	}

	return resolution, nil
}

func (s *PreserveBothStrategy) GetPriority() int {
	return 70
}

// SourcePriorityStrategy resolves conflicts based on predefined source priorities
type SourcePriorityStrategy struct {
	sourcePriorities map[string]int
}

func (s *SourcePriorityStrategy) CanResolve(conflict *SyncConflict) bool {
	// Initialize default priorities if not set
	if s.sourcePriorities == nil {
		s.sourcePriorities = map[string]int{
			"googledrive": 100,
			"onedrive":    90,
			"dropbox":     80,
			"box":         70,
			"sharepoint":  60,
			"slack":       50,
			"notion":      40,
		}
	}
	
	// Can resolve if we have priority info for at least one source
	for _, version := range conflict.FileVersions {
		if _, exists := s.sourcePriorities[version.ConnectorType]; exists {
			return true
		}
	}
	return false
}

func (s *SourcePriorityStrategy) Resolve(ctx context.Context, conflict *SyncConflict) (*ConflictResolution, error) {
	if len(conflict.FileVersions) == 0 {
		return nil, fmt.Errorf("no file versions to resolve")
	}

	// Find the version from the highest priority source
	var selectedVersion *ConflictFileVersion
	highestPriority := -1

	for _, version := range conflict.FileVersions {
		priority, exists := s.sourcePriorities[version.ConnectorType]
		if exists && priority > highestPriority {
			highestPriority = priority
			selectedVersion = version
		}
	}

	if selectedVersion == nil {
		// Fallback to first version if no priorities match
		selectedVersion = conflict.FileVersions[0]
	}

	resolution := &ConflictResolution{
		ResolvedVersion: selectedVersion,
		RequiresSync:    true,
		Actions: []ConflictResolutionAction{
			{
				Type:        "update",
				Source:      selectedVersion.ConnectorType,
				Target:      conflict.FilePath,
				Description: fmt.Sprintf("Updated to version from highest priority source: %s", selectedVersion.ConnectorType),
			},
		},
		Metadata: map[string]interface{}{
			"strategy":         "source_priority",
			"chosen_version":   selectedVersion.DataSourceID.String(),
			"source_priority":  highestPriority,
			"winning_source":   selectedVersion.ConnectorType,
		},
	}

	return resolution, nil
}

func (s *SourcePriorityStrategy) GetPriority() int {
	return 90
}

// SkipStrategy resolves conflicts by skipping the file entirely
type SkipStrategy struct{}

func (s *SkipStrategy) CanResolve(conflict *SyncConflict) bool {
	// Can "resolve" any conflict by skipping it
	return true
}

func (s *SkipStrategy) Resolve(ctx context.Context, conflict *SyncConflict) (*ConflictResolution, error) {
	resolution := &ConflictResolution{
		ResolvedVersion: nil, // No version selected
		RequiresSync:    false,
		Actions: []ConflictResolutionAction{
			{
				Type:        "skip",
				Source:      "system",
				Target:      conflict.FilePath,
				Description: fmt.Sprintf("Skipped conflicted file: %s", conflict.FilePath),
			},
		},
		Metadata: map[string]interface{}{
			"strategy":      "skip",
			"skipped_count": len(conflict.FileVersions),
			"reason":        "conflict_resolution_skipped",
		},
	}

	return resolution, nil
}

func (s *SkipStrategy) GetPriority() int {
	return 10 // Lowest priority, used as last resort
}

// MergeStrategy attempts to merge changes from multiple versions
type MergeStrategy struct{}

func (s *MergeStrategy) CanResolve(conflict *SyncConflict) bool {
	// Only attempt merge for text files and specific conflict types
	if conflict.ConflictType != ConflictTypeModifyModify {
		return false
	}

	// Check if files are text-based and suitable for merging
	for _, version := range conflict.FileVersions {
		if !s.isTextFile(version) {
			return false
		}
	}

	return true
}

func (s *MergeStrategy) isTextFile(version *ConflictFileVersion) bool {
	textMimeTypes := []string{
		"text/plain",
		"text/markdown",
		"text/csv",
		"application/json",
		"application/xml",
		"text/xml",
	}

	for _, mimeType := range textMimeTypes {
		if strings.Contains(version.ContentType, mimeType) {
			return true
		}
	}

	// Check file extensions
	textExtensions := []string{".txt", ".md", ".json", ".xml", ".csv", ".log"}
	ext := strings.ToLower(filepath.Ext(version.FilePath))
	for _, textExt := range textExtensions {
		if ext == textExt {
			return true
		}
	}

	return false
}

func (s *MergeStrategy) Resolve(ctx context.Context, conflict *SyncConflict) (*ConflictResolution, error) {
	if len(conflict.FileVersions) < 2 {
		return nil, fmt.Errorf("merge requires at least 2 versions")
	}

	// For now, this is a placeholder implementation
	// In a real system, this would:
	// 1. Download content from all versions
	// 2. Use a text merging algorithm (like diff3)
	// 3. Create a merged version
	// 4. Handle merge conflicts within the content

	// Select the base version (could be common ancestor or earliest version)
	baseVersion := conflict.FileVersions[0]

	// Create a merged version (simplified)
	mergedVersion := *baseVersion
	mergedVersion.FilePath = conflict.FilePath
	mergedVersion.ModifiedTime = time.Now()

	resolution := &ConflictResolution{
		ResolvedVersion: &mergedVersion,
		RequiresSync:    true,
		Actions: []ConflictResolutionAction{
			{
				Type:        "merge",
				Source:      "system",
				Target:      conflict.FilePath,
				Description: fmt.Sprintf("Merged %d versions into single file", len(conflict.FileVersions)),
				Metadata: map[string]interface{}{
					"merged_sources": s.getSourceList(conflict.FileVersions),
				},
			},
		},
		Metadata: map[string]interface{}{
			"strategy":       "merge",
			"merged_count":   len(conflict.FileVersions),
			"merge_type":     "text",
			"base_version":   baseVersion.DataSourceID.String(),
		},
	}

	return resolution, nil
}

func (s *MergeStrategy) getSourceList(versions []*ConflictFileVersion) []string {
	var sources []string
	for _, version := range versions {
		sources = append(sources, version.ConnectorType)
	}
	return sources
}

func (s *MergeStrategy) GetPriority() int {
	return 85 // High priority for compatible files
}

// ManualStrategy marks conflicts for manual resolution
type ManualStrategy struct{}

func (s *ManualStrategy) CanResolve(conflict *SyncConflict) bool {
	// Can handle any conflict by marking it for manual resolution
	return true
}

func (s *ManualStrategy) Resolve(ctx context.Context, conflict *SyncConflict) (*ConflictResolution, error) {
	// This doesn't actually resolve the conflict, but prepares it for manual resolution
	resolution := &ConflictResolution{
		ResolvedVersion: nil,
		RequiresSync:    false,
		Actions: []ConflictResolutionAction{
			{
				Type:        "manual_review",
				Source:      "system",
				Target:      conflict.FilePath,
				Description: fmt.Sprintf("Marked for manual resolution: %s", conflict.FilePath),
			},
		},
		Metadata: map[string]interface{}{
			"strategy":        "manual",
			"requires_review": true,
			"conflict_id":     conflict.ID.String(),
			"versions_count":  len(conflict.FileVersions),
		},
	}

	return resolution, nil
}

func (s *ManualStrategy) GetPriority() int {
	return 1 // Very low priority, used when auto-resolution fails
}