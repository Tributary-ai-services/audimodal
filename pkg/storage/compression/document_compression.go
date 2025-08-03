package compression

import (
	"context"
	"fmt"
	"mime"
	"path/filepath"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/jscharber/eAIIngest/pkg/storage"
	"github.com/jscharber/eAIIngest/pkg/storage/cache"
)

// DocumentCompressionStrategy defines compression strategies for different document types
type DocumentCompressionStrategy string

const (
	// Compression strategies
	StrategyNone       DocumentCompressionStrategy = "none"
	StrategyAdaptive   DocumentCompressionStrategy = "adaptive"
	StrategyAggressive DocumentCompressionStrategy = "aggressive"
	StrategyText       DocumentCompressionStrategy = "text"
	StrategyBinary     DocumentCompressionStrategy = "binary"
	StrategyArchive    DocumentCompressionStrategy = "archive"
)

// DocumentCompressionConfig contains configuration for document compression
type DocumentCompressionConfig struct {
	// General settings
	Enabled                bool                        `yaml:"enabled"`
	DefaultStrategy        DocumentCompressionStrategy `yaml:"default_strategy"`
	MinFileSizeThreshold   int64                       `yaml:"min_file_size_threshold"`  // Don't compress files smaller than this
	MaxFileSizeThreshold   int64                       `yaml:"max_file_size_threshold"`  // Don't compress files larger than this
	CompressionRatioTarget float64                     `yaml:"compression_ratio_target"` // Target compression ratio

	// Strategy-specific settings
	TextCompressionLevel   int     `yaml:"text_compression_level"`
	BinaryCompressionLevel int     `yaml:"binary_compression_level"`
	AdaptiveThreshold      float64 `yaml:"adaptive_threshold"`

	// File type mappings
	TextFileExtensions    []string                               `yaml:"text_file_extensions"`
	BinaryFileExtensions  []string                               `yaml:"binary_file_extensions"`
	ArchiveFileExtensions []string                               `yaml:"archive_file_extensions"`
	StrategyMappings      map[string]DocumentCompressionStrategy `yaml:"strategy_mappings"`

	// Performance settings
	BufferSize       int  `yaml:"buffer_size"`
	CompressionLevel int  `yaml:"compression_level"`
	EnableMetrics    bool `yaml:"enable_metrics"`
	MaxConcurrency   int  `yaml:"max_concurrency"`

	// Storage integration
	CacheCompressed     bool          `yaml:"cache_compressed"`
	CacheTTL            time.Duration `yaml:"cache_ttl"`
	StoreOriginal       bool          `yaml:"store_original"` // Keep original alongside compressed
	UseAsyncCompression bool          `yaml:"use_async_compression"`
}

// DefaultDocumentCompressionConfig returns default configuration
func DefaultDocumentCompressionConfig() *DocumentCompressionConfig {
	return &DocumentCompressionConfig{
		Enabled:                true,
		DefaultStrategy:        StrategyAdaptive,
		MinFileSizeThreshold:   1024,              // 1KB
		MaxFileSizeThreshold:   500 * 1024 * 1024, // 500MB
		CompressionRatioTarget: 0.7,               // 30% size reduction
		TextCompressionLevel:   9,
		BinaryCompressionLevel: 6,
		AdaptiveThreshold:      0.8, // Use adaptive compression if entropy > 0.8
		TextFileExtensions: []string{
			".txt", ".md", ".json", ".xml", ".html", ".css", ".js", ".csv",
			".log", ".sql", ".py", ".go", ".java", ".cpp", ".c", ".h",
		},
		BinaryFileExtensions: []string{
			".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx",
			".odt", ".ods", ".odp", ".rtf",
		},
		ArchiveFileExtensions: []string{
			".zip", ".rar", ".7z", ".tar", ".gz", ".bz2", ".xz",
		},
		StrategyMappings: map[string]DocumentCompressionStrategy{
			".txt":  StrategyText,
			".log":  StrategyText,
			".json": StrategyText,
			".xml":  StrategyText,
			".pdf":  StrategyBinary,
			".doc":  StrategyBinary,
			".docx": StrategyBinary,
			".zip":  StrategyNone,
			".gz":   StrategyNone,
		},
		BufferSize:          64 * 1024, // 64KB
		CompressionLevel:    6,
		EnableMetrics:       true,
		MaxConcurrency:      4,
		CacheCompressed:     true,
		CacheTTL:            24 * time.Hour,
		StoreOriginal:       false,
		UseAsyncCompression: true,
	}
}

// DocumentCompressor provides document-specific compression strategies
type DocumentCompressor struct {
	config             *DocumentCompressionConfig
	compressor         cache.Compressor
	adaptiveCompressor *cache.AdaptiveCompressor
	tracer             trace.Tracer
	metrics            *CompressionMetrics
}

// NewDocumentCompressor creates a new document compressor
func NewDocumentCompressor(config *DocumentCompressionConfig) *DocumentCompressor {
	if config == nil {
		config = DefaultDocumentCompressionConfig()
	}

	// Create base compressor
	compressor := cache.NewGzipCompressor(config.CompressionLevel)
	adaptiveCompressor := cache.NewAdaptiveCompressor()

	var metrics *CompressionMetrics
	if config.EnableMetrics {
		metrics = NewCompressionMetrics()
	}

	return &DocumentCompressor{
		config:             config,
		compressor:         compressor,
		adaptiveCompressor: adaptiveCompressor,
		tracer:             otel.Tracer("document-compressor"),
		metrics:            metrics,
	}
}

// CompressDocument compresses a document using the appropriate strategy
func (d *DocumentCompressor) CompressDocument(ctx context.Context, data []byte, filename string) (*CompressionResult, error) {
	ctx, span := d.tracer.Start(ctx, "compress_document")
	defer span.End()

	start := time.Now()
	defer func() {
		if d.metrics != nil {
			d.metrics.RecordCompression(filename, time.Since(start), len(data))
		}
	}()

	span.SetAttributes(
		attribute.String("file.name", filename),
		attribute.Int("file.original_size", len(data)),
	)

	// Check if compression should be applied
	if !d.shouldCompress(data, filename) {
		span.SetAttributes(attribute.String("compression.strategy", "none"))
		return &CompressionResult{
			Data:             data,
			OriginalSize:     len(data),
			CompressedSize:   len(data),
			CompressionRatio: 1.0,
			Strategy:         StrategyNone,
			Algorithm:        "none",
			CompressionTime:  time.Since(start),
		}, nil
	}

	// Determine compression strategy
	strategy := d.determineStrategy(data, filename)
	span.SetAttributes(attribute.String("compression.strategy", string(strategy)))

	// Apply compression
	compressed, algorithm, err := d.applyStrategy(ctx, data, strategy)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("compression failed: %w", err)
	}

	compressionRatio := float64(len(data)) / float64(len(compressed))

	// Check if compression was beneficial
	if compressionRatio < d.config.CompressionRatioTarget {
		// Compression didn't meet target, return original
		span.SetAttributes(attribute.Bool("compression.beneficial", false))
		return &CompressionResult{
			Data:             data,
			OriginalSize:     len(data),
			CompressedSize:   len(data),
			CompressionRatio: 1.0,
			Strategy:         StrategyNone,
			Algorithm:        "none",
			CompressionTime:  time.Since(start),
		}, nil
	}

	span.SetAttributes(
		attribute.Int("file.compressed_size", len(compressed)),
		attribute.Float64("compression.ratio", compressionRatio),
		attribute.String("compression.algorithm", algorithm),
		attribute.Bool("compression.beneficial", true),
	)

	return &CompressionResult{
		Data:             compressed,
		OriginalSize:     len(data),
		CompressedSize:   len(compressed),
		CompressionRatio: compressionRatio,
		Strategy:         strategy,
		Algorithm:        algorithm,
		CompressionTime:  time.Since(start),
	}, nil
}

// DecompressDocument decompresses a document
func (d *DocumentCompressor) DecompressDocument(ctx context.Context, data []byte, metadata *CompressionMetadata) ([]byte, error) {
	ctx, span := d.tracer.Start(ctx, "decompress_document")
	defer span.End()

	start := time.Now()
	defer func() {
		if d.metrics != nil {
			d.metrics.RecordDecompression(metadata.OriginalFilename, time.Since(start), len(data))
		}
	}()

	span.SetAttributes(
		attribute.String("compression.strategy", string(metadata.Strategy)),
		attribute.String("compression.algorithm", metadata.Algorithm),
		attribute.Int("file.compressed_size", len(data)),
	)

	if metadata.Strategy == StrategyNone || metadata.Algorithm == "none" {
		return data, nil
	}

	// Get appropriate decompressor
	var decompressed []byte
	var err error

	switch metadata.Algorithm {
	case "gzip":
		decompressed, err = d.compressor.Decompress(data)
	case "adaptive":
		decompressed, err = d.adaptiveCompressor.Decompress(data)
	default:
		// Try adaptive decompressor as fallback
		decompressed, err = d.adaptiveCompressor.Decompress(data)
	}

	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("decompression failed: %w", err)
	}

	span.SetAttributes(
		attribute.Int("file.decompressed_size", len(decompressed)),
		attribute.Float64("compression.ratio", float64(len(decompressed))/float64(len(data))),
	)

	return decompressed, nil
}

// shouldCompress determines if a file should be compressed
func (d *DocumentCompressor) shouldCompress(data []byte, filename string) bool {
	if !d.config.Enabled {
		return false
	}

	size := int64(len(data))

	// Check size thresholds
	if size < d.config.MinFileSizeThreshold || size > d.config.MaxFileSizeThreshold {
		return false
	}

	// Check if file is already compressed
	ext := strings.ToLower(filepath.Ext(filename))
	for _, archiveExt := range d.config.ArchiveFileExtensions {
		if ext == archiveExt {
			return false
		}
	}

	return true
}

// determineStrategy determines the best compression strategy for a file
func (d *DocumentCompressor) determineStrategy(data []byte, filename string) DocumentCompressionStrategy {
	ext := strings.ToLower(filepath.Ext(filename))

	// Check explicit mappings first
	if strategy, exists := d.config.StrategyMappings[ext]; exists {
		return strategy
	}

	// Check file type categories
	for _, textExt := range d.config.TextFileExtensions {
		if ext == textExt {
			return StrategyText
		}
	}

	for _, binaryExt := range d.config.BinaryFileExtensions {
		if ext == binaryExt {
			return StrategyBinary
		}
	}

	// Use MIME type detection as fallback
	mimeType := mime.TypeByExtension(ext)
	if strings.HasPrefix(mimeType, "text/") ||
		strings.Contains(mimeType, "json") ||
		strings.Contains(mimeType, "xml") {
		return StrategyText
	}

	// Default to adaptive strategy
	return d.config.DefaultStrategy
}

// applyStrategy applies the specified compression strategy
func (d *DocumentCompressor) applyStrategy(ctx context.Context, data []byte, strategy DocumentCompressionStrategy) ([]byte, string, error) {
	switch strategy {
	case StrategyNone:
		return data, "none", nil

	case StrategyText:
		compressed, err := cache.NewGzipCompressor(d.config.TextCompressionLevel).Compress(data)
		return compressed, "gzip", err

	case StrategyBinary:
		compressed, err := cache.NewGzipCompressor(d.config.BinaryCompressionLevel).Compress(data)
		return compressed, "gzip", err

	case StrategyAdaptive:
		compressed, err := d.adaptiveCompressor.Compress(data)
		return compressed, "adaptive", err

	case StrategyAggressive:
		// Use maximum compression level
		compressed, err := cache.NewGzipCompressor(9).Compress(data)
		return compressed, "gzip", err

	default:
		// Fallback to default compressor
		compressed, err := d.compressor.Compress(data)
		return compressed, "gzip", err
	}
}

// CompressionResult contains the result of a compression operation
type CompressionResult struct {
	Data             []byte                      `json:"data"`
	OriginalSize     int                         `json:"original_size"`
	CompressedSize   int                         `json:"compressed_size"`
	CompressionRatio float64                     `json:"compression_ratio"`
	Strategy         DocumentCompressionStrategy `json:"strategy"`
	Algorithm        string                      `json:"algorithm"`
	CompressionTime  time.Duration               `json:"compression_time"`
}

// CompressionMetadata contains metadata about compressed documents
type CompressionMetadata struct {
	Strategy           DocumentCompressionStrategy `json:"strategy"`
	Algorithm          string                      `json:"algorithm"`
	OriginalSize       int64                       `json:"original_size"`
	CompressedSize     int64                       `json:"compressed_size"`
	CompressionRatio   float64                     `json:"compression_ratio"`
	CompressionTime    time.Duration               `json:"compression_time"`
	OriginalFilename   string                      `json:"original_filename"`
	OriginalMimeType   string                      `json:"original_mime_type"`
	CompressedAt       time.Time                   `json:"compressed_at"`
	ChecksumOriginal   string                      `json:"checksum_original"`
	ChecksumCompressed string                      `json:"checksum_compressed"`
}

// CompressionMetrics tracks compression performance
type CompressionMetrics struct {
	TotalCompressions   int64         `json:"total_compressions"`
	TotalDecompressions int64         `json:"total_decompressions"`
	TotalBytesIn        int64         `json:"total_bytes_in"`
	TotalBytesOut       int64         `json:"total_bytes_out"`
	AverageRatio        float64       `json:"average_ratio"`
	AverageTime         time.Duration `json:"average_time"`
}

// NewCompressionMetrics creates new compression metrics
func NewCompressionMetrics() *CompressionMetrics {
	return &CompressionMetrics{}
}

// RecordCompression records compression metrics
func (m *CompressionMetrics) RecordCompression(filename string, duration time.Duration, originalSize int) {
	m.TotalCompressions++
	m.TotalBytesIn += int64(originalSize)
	// Additional metrics recording would be implemented here
}

// RecordDecompression records decompression metrics
func (m *CompressionMetrics) RecordDecompression(filename string, duration time.Duration, compressedSize int) {
	m.TotalDecompressions++
	// Additional metrics recording would be implemented here
}

// BatchCompressionJob represents a batch compression job
type BatchCompressionJob struct {
	Files       []storage.FileInfo          `json:"files"`
	Strategy    DocumentCompressionStrategy `json:"strategy"`
	Priority    int                         `json:"priority"`
	CreatedAt   time.Time                   `json:"created_at"`
	CompletedAt *time.Time                  `json:"completed_at,omitempty"`
	Status      string                      `json:"status"`
	Progress    float64                     `json:"progress"`
}

// BatchCompressor handles batch compression operations
type BatchCompressor struct {
	compressor *DocumentCompressor
	jobQueue   chan *BatchCompressionJob
	workers    int
	tracer     trace.Tracer
}

// NewBatchCompressor creates a new batch compressor
func NewBatchCompressor(compressor *DocumentCompressor, workers int) *BatchCompressor {
	return &BatchCompressor{
		compressor: compressor,
		jobQueue:   make(chan *BatchCompressionJob, 100),
		workers:    workers,
		tracer:     otel.Tracer("batch-compressor"),
	}
}

// Start starts the batch compression workers
func (b *BatchCompressor) Start(ctx context.Context) {
	for i := 0; i < b.workers; i++ {
		go b.worker(ctx, i)
	}
}

// SubmitJob submits a compression job
func (b *BatchCompressor) SubmitJob(job *BatchCompressionJob) error {
	select {
	case b.jobQueue <- job:
		return nil
	default:
		return fmt.Errorf("job queue is full")
	}
}

// worker processes compression jobs
func (b *BatchCompressor) worker(ctx context.Context, workerID int) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-b.jobQueue:
			b.processJob(ctx, job, workerID)
		}
	}
}

// processJob processes a single compression job
func (b *BatchCompressor) processJob(ctx context.Context, job *BatchCompressionJob, workerID int) {
	ctx, span := b.tracer.Start(ctx, "process_compression_job")
	defer span.End()

	span.SetAttributes(
		attribute.Int("worker.id", workerID),
		attribute.String("job.strategy", string(job.Strategy)),
		attribute.Int("job.file_count", len(job.Files)),
	)

	job.Status = "processing"

	for i := range job.Files {
		// Process each file (implementation would include actual file reading and compression)
		job.Progress = float64(i+1) / float64(len(job.Files))
	}

	now := time.Now()
	job.CompletedAt = &now
	job.Status = "completed"
	job.Progress = 1.0
}
