package archive

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jscharber/eAIIngest/pkg/core"
)

// ZIPReader implements DataSourceReader for ZIP archives
type ZIPReader struct {
	name    string
	version string
}

// NewZIPReader creates a new ZIP archive reader
func NewZIPReader() *ZIPReader {
	return &ZIPReader{
		name:    "zip_reader",
		version: "1.0.0",
	}
}

// GetConfigSpec returns the configuration specification
func (r *ZIPReader) GetConfigSpec() []core.ConfigSpec {
	return []core.ConfigSpec{
		{
			Name:        "extract_nested_archives",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Extract content from nested archives within the ZIP",
		},
		{
			Name:        "include_hidden_files",
			Type:        "bool",
			Required:    false,
			Default:     false,
			Description: "Include hidden files and directories",
		},
		{
			Name:        "max_file_size",
			Type:        "int",
			Required:    false,
			Default:     100,
			Description: "Maximum file size to extract in MB (0 = no limit)",
			MinValue:    ptrFloat64(0.0),
			MaxValue:    ptrFloat64(1000.0),
		},
		{
			Name:        "file_extensions_filter",
			Type:        "string",
			Required:    false,
			Default:     "",
			Description: "Comma-separated list of file extensions to extract (empty = all)",
		},
		{
			Name:        "max_files",
			Type:        "int",
			Required:    false,
			Default:     0,
			Description: "Maximum number of files to extract (0 = no limit)",
			MinValue:    ptrFloat64(0.0),
			MaxValue:    ptrFloat64(10000.0),
		},
		{
			Name:        "password",
			Type:        "string",
			Required:    false,
			Default:     "",
			Description: "Password for encrypted ZIP files",
		},
		{
			Name:        "skip_binary_files",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Skip binary files that cannot be processed as text",
		},
		{
			Name:        "preserve_directory_structure",
			Type:        "bool",
			Required:    false,
			Default:     true,
			Description: "Preserve directory structure in chunk metadata",
		},
	}
}

// ValidateConfig validates the provided configuration
func (r *ZIPReader) ValidateConfig(config map[string]any) error {
	if maxSize, ok := config["max_file_size"]; ok {
		if size, ok := maxSize.(float64); ok {
			if size < 0 || size > 1000 {
				return fmt.Errorf("max_file_size must be between 0 and 1000 MB")
			}
		}
	}

	if maxFiles, ok := config["max_files"]; ok {
		if files, ok := maxFiles.(float64); ok {
			if files < 0 || files > 10000 {
				return fmt.Errorf("max_files must be between 0 and 10000")
			}
		}
	}

	return nil
}

// TestConnection tests if the ZIP can be read
func (r *ZIPReader) TestConnection(ctx context.Context, config map[string]any) core.ConnectionTestResult {
	start := time.Now()

	err := r.ValidateConfig(config)
	if err != nil {
		return core.ConnectionTestResult{
			Success: false,
			Message: "Configuration validation failed",
			Latency: time.Since(start),
			Errors:  []string{err.Error()},
		}
	}

	return core.ConnectionTestResult{
		Success: true,
		Message: "ZIP reader ready",
		Latency: time.Since(start),
		Details: map[string]any{
			"extract_nested_archives": config["extract_nested_archives"],
			"skip_binary_files":       config["skip_binary_files"],
			"max_file_size":          config["max_file_size"],
		},
	}
}

// GetType returns the connector type
func (r *ZIPReader) GetType() string {
	return "reader"
}

// GetName returns the reader name
func (r *ZIPReader) GetName() string {
	return r.name
}

// GetVersion returns the reader version
func (r *ZIPReader) GetVersion() string {
	return r.version
}

// DiscoverSchema analyzes the ZIP archive structure
func (r *ZIPReader) DiscoverSchema(ctx context.Context, sourcePath string) (core.SchemaInfo, error) {
	if !r.isZIPFile(sourcePath) {
		return core.SchemaInfo{}, fmt.Errorf("not a valid ZIP file")
	}

	metadata, err := r.extractZIPMetadata(sourcePath)
	if err != nil {
		return core.SchemaInfo{}, fmt.Errorf("failed to extract ZIP metadata: %w", err)
	}

	schema := core.SchemaInfo{
		Format:   "zip",
		Encoding: "binary",
		Fields: []core.FieldInfo{
			{
				Name:        "filename",
				Type:        "string",
				Nullable:    false,
				Description: "Name of the file within the archive",
			},
			{
				Name:        "file_path",
				Type:        "string",
				Nullable:    false,
				Description: "Full path of the file within the archive",
			},
			{
				Name:        "content",
				Type:        "text",
				Nullable:    true,
				Description: "File content (if text-based)",
			},
			{
				Name:        "file_size",
				Type:        "integer",
				Nullable:    false,
				Description: "Size of the file in bytes",
			},
			{
				Name:        "compressed_size",
				Type:        "integer",
				Nullable:    false,
				Description: "Compressed size in the archive",
			},
			{
				Name:        "file_type",
				Type:        "string",
				Nullable:    true,
				Description: "Detected file type/extension",
			},
			{
				Name:        "modified_time",
				Type:        "datetime",
				Nullable:    true,
				Description: "Last modified time of the file",
			},
		},
		Metadata: map[string]any{
			"total_files":        metadata.TotalFiles,
			"total_directories":  metadata.TotalDirectories,
			"compressed_size":    metadata.CompressedSize,
			"uncompressed_size":  metadata.UncompressedSize,
			"compression_ratio":  metadata.CompressionRatio,
			"created_date":       metadata.CreatedDate,
			"has_directories":    metadata.HasDirectories,
			"has_encrypted_files": metadata.HasEncryptedFiles,
			"compression_method": metadata.CompressionMethod,
			"archive_comment":    metadata.Comment,
		},
	}

	// Sample first few files
	if metadata.TotalFiles > 0 {
		sampleData, err := r.extractSampleFiles(sourcePath)
		if err == nil {
			schema.SampleData = sampleData
		}
	}

	return schema, nil
}

// EstimateSize returns size estimates for the ZIP archive
func (r *ZIPReader) EstimateSize(ctx context.Context, sourcePath string) (core.SizeEstimate, error) {
	stat, err := os.Stat(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to stat file: %w", err)
	}

	metadata, err := r.extractZIPMetadata(sourcePath)
	if err != nil {
		return core.SizeEstimate{}, fmt.Errorf("failed to extract ZIP metadata: %w", err)
	}

	// Estimate based on total files
	totalFiles := int64(metadata.TotalFiles)
	if totalFiles == 0 {
		totalFiles = 1
	}

	// Estimate chunks based on files (typically 1 file per chunk)
	estimatedChunks := metadata.TotalFiles
	if estimatedChunks == 0 {
		estimatedChunks = 1
	}

	// ZIP complexity depends on number of files and compression
	complexity := "low"
	if metadata.TotalFiles > 100 || stat.Size() > 10*1024*1024 { // > 100 files or > 10MB
		complexity = "medium"
	}
	if metadata.TotalFiles > 1000 || stat.Size() > 100*1024*1024 { // > 1000 files or > 100MB
		complexity = "high"
	}
	if metadata.TotalFiles > 5000 || stat.Size() > 1024*1024*1024 { // > 5000 files or > 1GB
		complexity = "very_high"
	}

	// Processing time depends on number of files and total size
	processTime := "fast"
	if metadata.TotalFiles > 50 || metadata.UncompressedSize > 50*1024*1024 {
		processTime = "medium"
	}
	if metadata.TotalFiles > 500 || metadata.UncompressedSize > 500*1024*1024 {
		processTime = "slow"
	}
	if metadata.TotalFiles > 2000 || metadata.UncompressedSize > 2*1024*1024*1024 {
		processTime = "very_slow"
	}

	return core.SizeEstimate{
		RowCount:    &totalFiles,
		ByteSize:    stat.Size(),
		Complexity:  complexity,
		ChunkEst:    estimatedChunks,
		ProcessTime: processTime,
	}, nil
}

// CreateIterator creates a chunk iterator for the ZIP archive
func (r *ZIPReader) CreateIterator(ctx context.Context, sourcePath string, strategyConfig map[string]any) (core.ChunkIterator, error) {
	archive, err := r.openZIPArchive(sourcePath, strategyConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to open ZIP archive: %w", err)
	}

	iterator := &ZIPIterator{
		sourcePath:   sourcePath,
		config:       strategyConfig,
		archive:      archive,
		currentFile:  0,
	}

	return iterator, nil
}

// SupportsStreaming indicates ZIP reader supports streaming
func (r *ZIPReader) SupportsStreaming() bool {
	return true
}

// GetSupportedFormats returns supported file formats
func (r *ZIPReader) GetSupportedFormats() []string {
	return []string{"zip"}
}

// ZIPMetadata contains extracted ZIP metadata
type ZIPMetadata struct {
	TotalFiles        int
	TotalDirectories  int
	CompressedSize    int64
	UncompressedSize  int64
	CompressionRatio  float64
	CreatedDate       string
	HasDirectories    bool
	HasEncryptedFiles bool
	CompressionMethod string
	Comment           string
}

// ZIPArchive represents an opened ZIP archive
type ZIPArchive struct {
	Metadata ZIPMetadata
	Files    []*ZIPFileInfo
	Reader   *zip.ReadCloser
}

// ZIPFileInfo represents a file within the archive
type ZIPFileInfo struct {
	Name             string
	Path             string
	Size             int64
	CompressedSize   int64
	ModifiedTime     time.Time
	IsDirectory      bool
	IsEncrypted      bool
	CompressionMethod string
	CRC32            uint32
	FileHeader       *zip.File
}

// ptrFloat64 returns a pointer to a float64 value
func ptrFloat64(f float64) *float64 {
	return &f
}

// isZIPFile checks if the file is a valid ZIP file
func (r *ZIPReader) isZIPFile(filePath string) bool {
	file, err := os.Open(filePath)
	if err != nil {
		return false
	}
	defer file.Close()

	// Check ZIP signature: "PK" at the beginning
	signature := make([]byte, 2)
	n, err := file.Read(signature)
	if err != nil || n != 2 {
		return false
	}

	return signature[0] == 0x50 && signature[1] == 0x4B // "PK"
}

// extractZIPMetadata extracts metadata from ZIP file
func (r *ZIPReader) extractZIPMetadata(sourcePath string) (ZIPMetadata, error) {
	reader, err := zip.OpenReader(sourcePath)
	if err != nil {
		return ZIPMetadata{}, err
	}
	defer reader.Close()

	stat, err := os.Stat(sourcePath)
	if err != nil {
		return ZIPMetadata{}, err
	}

	metadata := ZIPMetadata{
		CreatedDate:      stat.ModTime().Format("2006-01-02 15:04:05"),
		CompressionMethod: "Deflate",
		Comment:          reader.Comment,
	}

	var totalCompressed, totalUncompressed int64
	var hasEncrypted bool

	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			metadata.TotalDirectories++
			metadata.HasDirectories = true
		} else {
			metadata.TotalFiles++
		}

		totalCompressed += int64(file.CompressedSize64)
		totalUncompressed += int64(file.UncompressedSize64)

		if file.IsEncrypted() {
			hasEncrypted = true
		}
	}

	metadata.CompressedSize = totalCompressed
	metadata.UncompressedSize = totalUncompressed
	metadata.HasEncryptedFiles = hasEncrypted

	if totalUncompressed > 0 {
		metadata.CompressionRatio = float64(totalCompressed) / float64(totalUncompressed)
	}

	return metadata, nil
}

// extractSampleFiles extracts sample file information
func (r *ZIPReader) extractSampleFiles(sourcePath string) ([]map[string]any, error) {
	reader, err := zip.OpenReader(sourcePath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	var samples []map[string]any
	count := 0
	maxSamples := 3

	for _, file := range reader.File {
		if count >= maxSamples {
			break
		}

		if file.FileInfo().IsDir() {
			continue
		}

		samples = append(samples, map[string]any{
			"filename":        filepath.Base(file.Name),
			"file_path":       file.Name,
			"file_size":       file.UncompressedSize64,
			"compressed_size": file.CompressedSize64,
			"file_type":       filepath.Ext(file.Name),
			"modified_time":   file.Modified.Format("2006-01-02 15:04:05"),
		})

		count++
	}

	return samples, nil
}

// openZIPArchive opens and prepares the ZIP archive for iteration
func (r *ZIPReader) openZIPArchive(sourcePath string, config map[string]any) (*ZIPArchive, error) {
	reader, err := zip.OpenReader(sourcePath)
	if err != nil {
		return nil, err
	}

	metadata, err := r.extractZIPMetadata(sourcePath)
	if err != nil {
		reader.Close()
		return nil, err
	}

	// Filter files based on configuration
	var files []*ZIPFileInfo
	maxFiles := 0
	if max, ok := config["max_files"].(float64); ok {
		maxFiles = int(max)
	}

	includeHidden := false
	if hidden, ok := config["include_hidden_files"].(bool); ok {
		includeHidden = hidden
	}

	skipBinary := true
	if skip, ok := config["skip_binary_files"].(bool); ok {
		skipBinary = skip
	}

	extensionFilter := ""
	if filter, ok := config["file_extensions_filter"].(string); ok {
		extensionFilter = strings.ToLower(filter)
	}

	fileCount := 0
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}

		// Check max files limit
		if maxFiles > 0 && fileCount >= maxFiles {
			break
		}

		// Check hidden files
		if !includeHidden && strings.HasPrefix(filepath.Base(file.Name), ".") {
			continue
		}

		// Check extension filter
		if extensionFilter != "" {
			ext := strings.ToLower(filepath.Ext(file.Name))
			if ext != "" {
				ext = ext[1:] // Remove the dot
			}
			extensions := strings.Split(extensionFilter, ",")
			found := false
			for _, filterExt := range extensions {
				if strings.TrimSpace(filterExt) == ext {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		// Check if file is binary (simplified check)
		if skipBinary && r.isBinaryFile(file.Name) {
			continue
		}

		fileInfo := &ZIPFileInfo{
			Name:             filepath.Base(file.Name),
			Path:             file.Name,
			Size:             int64(file.UncompressedSize64),
			CompressedSize:   int64(file.CompressedSize64),
			ModifiedTime:     file.Modified,
			IsDirectory:      file.FileInfo().IsDir(),
			IsEncrypted:      file.IsEncrypted(),
			CompressionMethod: file.Method.String(),
			CRC32:            file.CRC32,
			FileHeader:       file,
		}

		files = append(files, fileInfo)
		fileCount++
	}

	return &ZIPArchive{
		Metadata: metadata,
		Files:    files,
		Reader:   reader,
	}, nil
}

// isBinaryFile checks if a file is likely binary based on extension
func (r *ZIPReader) isBinaryFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	binaryExtensions := map[string]bool{
		".exe": true, ".dll": true, ".so": true, ".dylib": true,
		".bin": true, ".dat": true, ".db": true, ".sqlite": true,
		".jpg": true, ".jpeg": true, ".png": true, ".gif": true,
		".bmp": true, ".tiff": true, ".ico": true, ".svg": true,
		".mp3": true, ".wav": true, ".flac": true, ".ogg": true,
		".mp4": true, ".avi": true, ".mkv": true, ".mov": true,
		".zip": true, ".rar": true, ".7z": true, ".tar": true,
		".gz": true, ".bz2": true, ".xz": true,
	}
	return binaryExtensions[ext]
}

// ZIPIterator implements ChunkIterator for ZIP archives
type ZIPIterator struct {
	sourcePath  string
	config      map[string]any
	archive     *ZIPArchive
	currentFile int
}

// Next returns the next chunk of content from the ZIP archive
func (it *ZIPIterator) Next(ctx context.Context) (core.Chunk, error) {
	select {
	case <-ctx.Done():
		return core.Chunk{}, ctx.Err()
	default:
	}

	// Check if we've processed all files
	if it.currentFile >= len(it.archive.Files) {
		return core.Chunk{}, core.ErrIteratorExhausted
	}

	fileInfo := it.archive.Files[it.currentFile]
	it.currentFile++

	// Extract file content
	content, err := it.extractFileContent(fileInfo)
	if err != nil {
		// Skip files that can't be extracted but continue iteration
		return it.Next(ctx)
	}

	chunk := core.Chunk{
		Data: content,
		Metadata: core.ChunkMetadata{
			SourcePath:  it.sourcePath,
			ChunkID:     fmt.Sprintf("%s:file:%s", filepath.Base(it.sourcePath), fileInfo.Path),
			ChunkType:   "zip_file",
			SizeBytes:   int64(len(content)),
			ProcessedAt: time.Now(),
			ProcessedBy: "zip_reader",
			Context: map[string]string{
				"filename":           fileInfo.Name,
				"file_path":          fileInfo.Path,
				"file_size":          strconv.FormatInt(fileInfo.Size, 10),
				"compressed_size":    strconv.FormatInt(fileInfo.CompressedSize, 10),
				"modified_time":      fileInfo.ModifiedTime.Format("2006-01-02 15:04:05"),
				"compression_method": fileInfo.CompressionMethod,
				"is_encrypted":       strconv.FormatBool(fileInfo.IsEncrypted),
				"file_type":          filepath.Ext(fileInfo.Name),
				"archive_type":       "zip",
			},
		},
	}

	return chunk, nil
}

// extractFileContent extracts content from a file in the archive
func (it *ZIPIterator) extractFileContent(fileInfo *ZIPFileInfo) (string, error) {
	// Check file size limit
	maxSize := int64(100 * 1024 * 1024) // Default 100MB
	if size, ok := it.config["max_file_size"].(float64); ok && size > 0 {
		maxSize = int64(size * 1024 * 1024)
	}

	if fileInfo.Size > maxSize {
		return fmt.Sprintf("[File too large: %d bytes, limit: %d bytes]", fileInfo.Size, maxSize), nil
	}

	// Open the file in the archive
	rc, err := fileInfo.FileHeader.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()

	// Read file content
	content, err := io.ReadAll(rc)
	if err != nil {
		return "", err
	}

	// Convert to string (assume UTF-8 for text files)
	return string(content), nil
}

// Close releases ZIP resources
func (it *ZIPIterator) Close() error {
	if it.archive != nil && it.archive.Reader != nil {
		return it.archive.Reader.Close()
	}
	return nil
}

// Reset restarts iteration from the beginning
func (it *ZIPIterator) Reset() error {
	it.currentFile = 0
	return nil
}

// Progress returns iteration progress
func (it *ZIPIterator) Progress() float64 {
	totalFiles := len(it.archive.Files)
	if totalFiles == 0 {
		return 1.0
	}
	return float64(it.currentFile) / float64(totalFiles)
}