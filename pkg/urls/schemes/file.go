package schemes

import (
	"context"
	"fmt"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jscharber/eAIIngest/pkg/core"
)

// FileResolver handles file:// URLs for local filesystem access
type FileResolver struct{}

// NewFileResolver creates a new file resolver
func NewFileResolver() *FileResolver {
	return &FileResolver{}
}

// Supports returns true for file scheme URLs
func (f *FileResolver) Supports(scheme string) bool {
	return strings.ToLower(scheme) == "file"
}

// Resolve resolves a file:// URL to a ResolvedFile
func (f *FileResolver) Resolve(ctx context.Context, fileURL string) (*core.ResolvedFile, error) {
	parsed, err := url.Parse(fileURL)
	if err != nil {
		return nil, &core.URLError{
			Type:    "INVALID_FILE_URL",
			Message: fmt.Sprintf("invalid file URL: %v", err),
			URL:     fileURL,
		}
	}

	if parsed.Scheme != "file" {
		return nil, &core.URLError{
			Type:    "WRONG_SCHEME",
			Message: fmt.Sprintf("expected file:// scheme, got %s://", parsed.Scheme),
			URL:     fileURL,
			Scheme:  parsed.Scheme,
		}
	}

	// Convert URL path to local file path
	filePath := parsed.Path
	if parsed.Host != "" && parsed.Host != "localhost" {
		return nil, &core.URLError{
			Type:    "REMOTE_FILE_NOT_SUPPORTED",
			Message: fmt.Sprintf("remote file access not supported: %s", parsed.Host),
			URL:     fileURL,
		}
	}

	// On Windows, handle drive letters properly
	if len(filePath) > 0 && filePath[0] == '/' && len(filePath) > 3 && filePath[2] == ':' {
		filePath = filePath[1:] // Remove leading slash for Windows paths like /C:/path
	}

	// Get file information
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &core.URLError{
				Type:    "FILE_NOT_FOUND",
				Message: fmt.Sprintf("file not found: %s", filePath),
				URL:     fileURL,
			}
		}
		return nil, &core.URLError{
			Type:    "FILE_ACCESS_ERROR",
			Message: fmt.Sprintf("cannot access file: %v", err),
			URL:     fileURL,
		}
	}

	if fileInfo.IsDir() {
		return nil, &core.URLError{
			Type:    "IS_DIRECTORY",
			Message: fmt.Sprintf("path is a directory, not a file: %s", filePath),
			URL:     fileURL,
		}
	}

	// Determine content type from extension
	contentType := mime.TypeByExtension(filepath.Ext(filePath))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Create resolved file
	resolved := &core.ResolvedFile{
		URL:         fileURL,
		Scheme:      "file",
		Size:        fileInfo.Size(),
		ContentType: contentType,
		Metadata: map[string]string{
			"path":      filePath,
			"name":      fileInfo.Name(),
			"extension": filepath.Ext(filePath),
			"directory": filepath.Dir(filePath),
		},
		AccessInfo: core.AccessInfo{
			RequiresAuth: false,
		},
		LastModified: fileInfo.ModTime().Unix(),
	}

	return resolved, nil
}

// CreateIterator creates a chunk iterator for a local file
func (f *FileResolver) CreateIterator(ctx context.Context, fileURL string, config map[string]any) (core.ChunkIterator, error) {
	resolved, err := f.Resolve(ctx, fileURL)
	if err != nil {
		return nil, err
	}

	filePath := resolved.Metadata["path"]
	return NewFileIterator(filePath, config)
}

// GetName returns the resolver name
func (f *FileResolver) GetName() string {
	return "file_resolver"
}

// GetVersion returns the resolver version
func (f *FileResolver) GetVersion() string {
	return "1.0.0"
}

// FileIterator implements ChunkIterator for local files
type FileIterator struct {
	filePath string
	config   map[string]any
	file     *os.File
	closed   bool
	position int64
	size     int64
	chunkID  int
}

// NewFileIterator creates a new file iterator
func NewFileIterator(filePath string, config map[string]any) (core.ChunkIterator, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", filePath, err)
	}

	stat, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to stat file %s: %w", filePath, err)
	}

	return &FileIterator{
		filePath: filePath,
		config:   config,
		file:     file,
		size:     stat.Size(),
		chunkID:  0,
	}, nil
}

// Next returns the next chunk from the file
func (fi *FileIterator) Next(ctx context.Context) (core.Chunk, error) {
	if fi.closed {
		return core.Chunk{}, fmt.Errorf("iterator is closed")
	}

	select {
	case <-ctx.Done():
		return core.Chunk{}, ctx.Err()
	default:
	}

	if fi.position >= fi.size {
		return core.Chunk{}, fmt.Errorf("no more data available")
	}

	chunkSize := fi.size
	if configChunkSize, ok := fi.config["chunk_size"]; ok {
		if size, ok := configChunkSize.(int64); ok && size > 0 {
			chunkSize = size
		}
	}

	remainingSize := fi.size - fi.position
	if chunkSize > remainingSize {
		chunkSize = remainingSize
	}

	data := make([]byte, chunkSize)
	n, err := fi.file.Read(data)
	if err != nil {
		return core.Chunk{}, fmt.Errorf("failed to read file: %w", err)
	}

	startPos := fi.position
	fi.position += int64(n)
	fi.chunkID++

	chunk := core.Chunk{
		Data: string(data[:n]),
		Metadata: core.ChunkMetadata{
			SourcePath:    fi.filePath,
			ChunkID:       fmt.Sprintf("chunk_%d", fi.chunkID-1),
			ChunkType:     "text",
			SizeBytes:     int64(n),
			StartPosition: &startPos,
			EndPosition:   &fi.position,
			ProcessedAt:   time.Now(),
			ProcessedBy:   "file_iterator",
		},
	}

	return chunk, nil
}

// Close closes the file iterator
func (fi *FileIterator) Close() error {
	if !fi.closed {
		fi.closed = true
		if fi.file != nil {
			return fi.file.Close()
		}
	}
	return nil
}

// Reset resets the iterator to the beginning
func (fi *FileIterator) Reset() error {
	if fi.closed {
		return fmt.Errorf("iterator is closed")
	}

	_, err := fi.file.Seek(0, 0)
	if err != nil {
		return fmt.Errorf("failed to reset file position: %w", err)
	}

	fi.position = 0
	fi.chunkID = 0
	return nil
}

// Progress returns the current progress (0.0 to 1.0)
func (fi *FileIterator) Progress() float64 {
	if fi.size == 0 {
		return 1.0
	}
	return float64(fi.position) / float64(fi.size)
}
