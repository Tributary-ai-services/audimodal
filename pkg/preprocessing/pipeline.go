package preprocessing

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileProcessor handles the complete preprocessing pipeline
type FileProcessor struct {
	decompressor     *FileDecompressor
	encodingDetector *EncodingDetector
	tempDir          string
	cleanupFiles     []string
}

// NewFileProcessor creates a new file processor
func NewFileProcessor(tempDir string) *FileProcessor {
	if tempDir == "" {
		tempDir = os.TempDir()
	}

	return &FileProcessor{
		decompressor:     NewFileDecompressor(tempDir),
		encodingDetector: NewEncodingDetector(),
		tempDir:          tempDir,
		cleanupFiles:     make([]string, 0),
	}
}

// ProcessedFile represents a file that has been preprocessed
type ProcessedFile struct {
	OriginalPath       string
	ProcessedPath      string
	Name               string
	Size               int64
	IsCompressed       bool
	CompressionType    string
	OriginalEncoding   string
	EncodingConfidence float64
	IsDirectory        bool
	SourceArchive      string // Path to original archive if extracted
}

// ProcessFile handles decompression and encoding conversion for a single file
func (fp *FileProcessor) ProcessFile(filePath string) ([]ProcessedFile, error) {
	var results []ProcessedFile

	// Check if file is compressed
	isCompressed, err := fp.decompressor.IsCompressed(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to check compression: %w", err)
	}

	if isCompressed {
		// Decompress file
		compressionType, _ := fp.decompressor.DetectCompressionType(filePath)
		decompressedFiles, err := fp.decompressor.DecompressFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to decompress file: %w", err)
		}

		// Process each decompressed file
		for _, decompFile := range decompressedFiles {
			if decompFile.IsDirectory {
				results = append(results, ProcessedFile{
					OriginalPath:    filePath,
					ProcessedPath:   decompFile.Path,
					Name:            decompFile.Name,
					Size:            decompFile.Size,
					IsCompressed:    true,
					CompressionType: compressionType,
					IsDirectory:     true,
					SourceArchive:   filePath,
				})
				continue
			}

			// Process encoding for regular files
			processedFile, err := fp.processFileEncoding(decompFile.Path, filePath, true, compressionType)
			if err != nil {
				return nil, fmt.Errorf("failed to process encoding for %s: %w", decompFile.Name, err)
			}

			processedFile.Name = decompFile.Name
			processedFile.SourceArchive = filePath
			results = append(results, *processedFile)
		}
	} else {
		// Process encoding for uncompressed file
		processedFile, err := fp.processFileEncoding(filePath, filePath, false, "")
		if err != nil {
			return nil, fmt.Errorf("failed to process encoding: %w", err)
		}

		stat, err := os.Stat(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to stat file: %w", err)
		}

		processedFile.Name = filepath.Base(filePath)
		processedFile.Size = stat.Size()
		processedFile.IsDirectory = stat.IsDir()
		results = append(results, *processedFile)
	}

	return results, nil
}

// processFileEncoding handles encoding detection and conversion
func (fp *FileProcessor) processFileEncoding(filePath, originalPath string, isCompressed bool, compressionType string) (*ProcessedFile, error) {
	// Detect encoding
	encodingInfo, err := fp.encodingDetector.DetectEncoding(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to detect encoding: %w", err)
	}

	// Convert to UTF-8 if needed
	processedPath := filePath
	if encodingInfo.Name != "utf-8" {
		convertedPath, err := fp.encodingDetector.ConvertToUTF8(filePath, encodingInfo.Name)
		if err != nil {
			// Log warning but continue with original file
			fmt.Printf("Warning: failed to convert encoding from %s to UTF-8: %v\n", encodingInfo.Name, err)
		} else {
			processedPath = convertedPath
			fp.cleanupFiles = append(fp.cleanupFiles, convertedPath)
		}
	}

	return &ProcessedFile{
		OriginalPath:       originalPath,
		ProcessedPath:      processedPath,
		IsCompressed:       isCompressed,
		CompressionType:    compressionType,
		OriginalEncoding:   encodingInfo.Name,
		EncodingConfidence: encodingInfo.Confidence,
		IsDirectory:        false,
	}, nil
}

// ProcessDirectory recursively processes all files in a directory
func (fp *FileProcessor) ProcessDirectory(dirPath string) ([]ProcessedFile, error) {
	var allResults []ProcessedFile

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories themselves, but process files within them
		if info.IsDir() {
			return nil
		}

		results, err := fp.ProcessFile(path)
		if err != nil {
			// Log error but continue with other files
			fmt.Printf("Warning: failed to process file %s: %v\n", path, err)
			return nil
		}

		allResults = append(allResults, results...)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}

	return allResults, nil
}

// FilterProcessedFiles filters processed files by extension or pattern
func (fp *FileProcessor) FilterProcessedFiles(files []ProcessedFile, extensions []string, excludePatterns []string) []ProcessedFile {
	var filtered []ProcessedFile

	for _, file := range files {
		if file.IsDirectory {
			continue
		}

		// Check extension filter
		if len(extensions) > 0 {
			ext := strings.ToLower(filepath.Ext(file.Name))
			found := false
			for _, allowedExt := range extensions {
				if ext == strings.ToLower(allowedExt) {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		// Check exclude patterns
		excluded := false
		for _, pattern := range excludePatterns {
			if matched, _ := filepath.Match(pattern, file.Name); matched {
				excluded = true
				break
			}
		}
		if excluded {
			continue
		}

		filtered = append(filtered, file)
	}

	return filtered
}

// GetProcessingStats returns statistics about processed files
func (fp *FileProcessor) GetProcessingStats(files []ProcessedFile) map[string]interface{} {
	stats := map[string]interface{}{
		"total_files":       0,
		"compressed_files":  0,
		"converted_files":   0,
		"compression_types": make(map[string]int),
		"encodings":         make(map[string]int),
		"total_size":        int64(0),
	}

	compressionTypes := make(map[string]int)
	encodings := make(map[string]int)

	for _, file := range files {
		if file.IsDirectory {
			continue
		}

		stats["total_files"] = stats["total_files"].(int) + 1
		stats["total_size"] = stats["total_size"].(int64) + file.Size

		if file.IsCompressed {
			stats["compressed_files"] = stats["compressed_files"].(int) + 1
			compressionTypes[file.CompressionType]++
		}

		if file.OriginalEncoding != "utf-8" && file.ProcessedPath != file.OriginalPath {
			stats["converted_files"] = stats["converted_files"].(int) + 1
		}

		encodings[file.OriginalEncoding]++
	}

	stats["compression_types"] = compressionTypes
	stats["encodings"] = encodings

	return stats
}

// Cleanup removes temporary files created during processing
func (fp *FileProcessor) Cleanup() error {
	var errors []string

	// Clean up encoding conversion files
	for _, filePath := range fp.cleanupFiles {
		if err := os.Remove(filePath); err != nil {
			errors = append(errors, fmt.Sprintf("failed to remove %s: %v", filePath, err))
		}
	}

	fp.cleanupFiles = nil

	if len(errors) > 0 {
		return fmt.Errorf("cleanup errors: %s", strings.Join(errors, "; "))
	}

	return nil
}

// GetSupportedFormats returns all supported formats (compression + encoding)
func (fp *FileProcessor) GetSupportedFormats() map[string][]string {
	return map[string][]string{
		"compression": fp.decompressor.GetSupportedFormats(),
		"encoding":    fp.encodingDetector.GetSupportedEncodings(),
	}
}

// IsProcessingRequired checks if a file needs preprocessing
func (fp *FileProcessor) IsProcessingRequired(filePath string) (bool, string, error) {
	// Check compression
	isCompressed, err := fp.decompressor.IsCompressed(filePath)
	if err != nil {
		return false, "", err
	}

	if isCompressed {
		compressionType, _ := fp.decompressor.DetectCompressionType(filePath)
		return true, fmt.Sprintf("compressed (%s)", compressionType), nil
	}

	// Check encoding
	encodingInfo, err := fp.encodingDetector.DetectEncoding(filePath)
	if err != nil {
		return false, "", err
	}

	if encodingInfo.Name != "utf-8" {
		return true, fmt.Sprintf("encoding (%s)", encodingInfo.Name), nil
	}

	return false, "ready", nil
}
