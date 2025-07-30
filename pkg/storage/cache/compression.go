package cache

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"math"
	"time"
)

// Compressor defines the interface for data compression
type Compressor interface {
	// Compress compresses the input data
	Compress(data []byte) ([]byte, error)
	
	// Decompress decompresses the input data
	Decompress(data []byte) ([]byte, error)
	
	// GetType returns the compression type identifier
	GetType() string
	
	// GetLevel returns the compression level
	GetLevel() int
}

// NoOpCompressor implements a pass-through compressor (no compression)
type NoOpCompressor struct{}

// NewNoOpCompressor creates a new no-op compressor
func NewNoOpCompressor() *NoOpCompressor {
	return &NoOpCompressor{}
}

// Compress returns the data unchanged
func (c *NoOpCompressor) Compress(data []byte) ([]byte, error) {
	return data, nil
}

// Decompress returns the data unchanged
func (c *NoOpCompressor) Decompress(data []byte) ([]byte, error) {
	return data, nil
}

// GetType returns the compression type
func (c *NoOpCompressor) GetType() string {
	return "none"
}

// GetLevel returns the compression level
func (c *NoOpCompressor) GetLevel() int {
	return 0
}

// GzipCompressor implements gzip compression
type GzipCompressor struct {
	level int
}

// NewGzipCompressor creates a new gzip compressor with specified compression level
func NewGzipCompressor(level int) *GzipCompressor {
	if level < gzip.DefaultCompression || level > gzip.BestCompression {
		level = gzip.DefaultCompression
	}
	return &GzipCompressor{level: level}
}

// Compress compresses data using gzip
func (c *GzipCompressor) Compress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	var buf bytes.Buffer
	writer, err := gzip.NewWriterLevel(&buf, c.level)
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip writer: %w", err)
	}

	_, err = writer.Write(data)
	if err != nil {
		writer.Close()
		return nil, fmt.Errorf("failed to write data to gzip writer: %w", err)
	}

	err = writer.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to close gzip writer: %w", err)
	}

	return buf.Bytes(), nil
}

// Decompress decompresses gzip data
func (c *GzipCompressor) Decompress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress gzip data: %w", err)
	}

	return decompressed, nil
}

// GetType returns the compression type
func (c *GzipCompressor) GetType() string {
	return "gzip"
}

// GetLevel returns the compression level
func (c *GzipCompressor) GetLevel() int {
	return c.level
}

// SimpleCompressor implements a simple byte-based compression for demo purposes
type SimpleCompressor struct {
	level int
}

// NewSimpleCompressor creates a new simple compressor
func NewSimpleCompressor(level int) *SimpleCompressor {
	return &SimpleCompressor{level: level}
}

// Compress compresses data using a simple algorithm (for demo - just uses gzip)
func (c *SimpleCompressor) Compress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	// Use gzip as a fallback for this simple implementation
	var buf bytes.Buffer
	writer, err := gzip.NewWriterLevel(&buf, gzip.DefaultCompression)
	if err != nil {
		return nil, fmt.Errorf("failed to create simple compressor writer: %w", err)
	}

	_, err = writer.Write(data)
	if err != nil {
		writer.Close()
		return nil, fmt.Errorf("failed to write data to simple compressor: %w", err)
	}

	err = writer.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to close simple compressor: %w", err)
	}

	return buf.Bytes(), nil
}

// Decompress decompresses simple compressed data
func (c *SimpleCompressor) Decompress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create simple decompressor reader: %w", err)
	}
	defer reader.Close()
	
	decompressed, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress simple data: %w", err)
	}

	return decompressed, nil
}

// GetType returns the compression type
func (c *SimpleCompressor) GetType() string {
	return "simple"
}

// GetLevel returns the compression level
func (c *SimpleCompressor) GetLevel() int {
	return c.level
}

// CompressionRegistry manages different compression algorithms
type CompressionRegistry struct {
	compressors map[string]func(level int) Compressor
}

// NewCompressionRegistry creates a new compression registry
func NewCompressionRegistry() *CompressionRegistry {
	registry := &CompressionRegistry{
		compressors: make(map[string]func(level int) Compressor),
	}

	// Register built-in compressors
	registry.Register("none", func(level int) Compressor {
		return NewNoOpCompressor()
	})
	registry.Register("gzip", func(level int) Compressor {
		return NewGzipCompressor(level)
	})
	registry.Register("simple", func(level int) Compressor {
		return NewSimpleCompressor(level)
	})

	return registry
}

// Register registers a new compression algorithm
func (r *CompressionRegistry) Register(name string, factory func(level int) Compressor) {
	r.compressors[name] = factory
}

// Get returns a compressor instance for the specified algorithm
func (r *CompressionRegistry) Get(name string, level int) (Compressor, error) {
	factory, exists := r.compressors[name]
	if !exists {
		return nil, fmt.Errorf("unknown compression algorithm: %s", name)
	}
	
	return factory(level), nil
}

// GetAvailable returns a list of available compression algorithms
func (r *CompressionRegistry) GetAvailable() []string {
	algorithms := make([]string, 0, len(r.compressors))
	for name := range r.compressors {
		algorithms = append(algorithms, name)
	}
	return algorithms
}

// DefaultCompressionRegistry returns a registry with built-in compressors
var DefaultCompressionRegistry = NewCompressionRegistry()

// AdaptiveCompressor automatically selects the best compression algorithm based on data characteristics
type AdaptiveCompressor struct {
	gzipCompressor   *GzipCompressor
	simpleCompressor *SimpleCompressor
	noopCompressor   *NoOpCompressor
	
	// Thresholds for algorithm selection
	minSizeForCompression int
	gzipThreshold        float64 // Use gzip if expected compression ratio is better than this
}

// NewAdaptiveCompressor creates a new adaptive compressor
func NewAdaptiveCompressor() *AdaptiveCompressor {
	return &AdaptiveCompressor{
		gzipCompressor:        NewGzipCompressor(gzip.DefaultCompression),
		simpleCompressor:      NewSimpleCompressor(0),
		noopCompressor:        NewNoOpCompressor(),
		minSizeForCompression: 1024,    // 1KB minimum
		gzipThreshold:        0.7,      // Use gzip if compression ratio > 70%
	}
}

// Compress compresses data using the most appropriate algorithm
func (c *AdaptiveCompressor) Compress(data []byte) ([]byte, error) {
	// Skip compression for small data
	if len(data) < c.minSizeForCompression {
		// Prepend compression type indicator
		result := make([]byte, len(data)+1)
		result[0] = 0 // 0 = no compression
		copy(result[1:], data)
		return result, nil
	}

	// Analyze data characteristics
	entropy := c.calculateEntropy(data)
	
	var compressor Compressor
	var typeIndicator byte
	
	// Select algorithm based on data characteristics
	if entropy < 6.0 { // Low entropy, likely compressible
		compressor = c.gzipCompressor
		typeIndicator = 1 // 1 = gzip
	} else if entropy < 7.0 { // Medium entropy
		compressor = c.simpleCompressor
		typeIndicator = 2 // 2 = simple
	} else { // High entropy, likely not compressible
		compressor = c.noopCompressor
		typeIndicator = 0 // 0 = no compression
	}

	compressed, err := compressor.Compress(data)
	if err != nil {
		return nil, err
	}

	// Check if compression was beneficial
	if len(compressed) >= len(data) && typeIndicator != 0 {
		// Compression didn't help, use no compression
		compressed = data
		typeIndicator = 0
	}

	// Prepend type indicator
	result := make([]byte, len(compressed)+1)
	result[0] = typeIndicator
	copy(result[1:], compressed)
	
	return result, nil
}

// Decompress decompresses data using the appropriate algorithm based on type indicator
func (c *AdaptiveCompressor) Decompress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	// Extract type indicator
	typeIndicator := data[0]
	compressedData := data[1:]

	var compressor Compressor
	switch typeIndicator {
	case 0:
		compressor = c.noopCompressor
	case 1:
		compressor = c.gzipCompressor
	case 2:
		compressor = c.simpleCompressor
	default:
		return nil, fmt.Errorf("unknown compression type indicator: %d", typeIndicator)
	}

	return compressor.Decompress(compressedData)
}

// GetType returns the compression type
func (c *AdaptiveCompressor) GetType() string {
	return "adaptive"
}

// GetLevel returns the compression level
func (c *AdaptiveCompressor) GetLevel() int {
	return 0
}

// calculateEntropy calculates the entropy of data to determine compressibility
func (c *AdaptiveCompressor) calculateEntropy(data []byte) float64 {
	if len(data) == 0 {
		return 0
	}

	// Count byte frequencies
	frequencies := make(map[byte]int)
	for _, b := range data {
		frequencies[b]++
	}

	// Calculate entropy
	var entropy float64
	dataLen := float64(len(data))
	
	for _, freq := range frequencies {
		if freq > 0 {
			p := float64(freq) / dataLen
			entropy -= p * log2(p)
		}
	}

	return entropy
}

// log2 calculates log base 2
func log2(x float64) float64 {
	if x <= 0 {
		return 0
	}
	// Using change of base: log2(x) = ln(x) / ln(2)
	return math.Log(x) / math.Log(2)
}

// CompressionBenchmark benchmarks different compression algorithms
type CompressionBenchmark struct {
	algorithms []Compressor
	testData   [][]byte
}

// NewCompressionBenchmark creates a new compression benchmark
func NewCompressionBenchmark() *CompressionBenchmark {
	return &CompressionBenchmark{
		algorithms: []Compressor{
			NewNoOpCompressor(),
			NewGzipCompressor(gzip.DefaultCompression),
			NewSimpleCompressor(0),
			NewAdaptiveCompressor(),
		},
	}
}

// BenchmarkResult contains the results of a compression benchmark
type BenchmarkResult struct {
	Algorithm      string        `json:"algorithm"`
	OriginalSize   int           `json:"original_size"`
	CompressedSize int           `json:"compressed_size"`
	CompressionRatio float64     `json:"compression_ratio"`
	CompressTime   time.Duration `json:"compress_time"`
	DecompressTime time.Duration `json:"decompress_time"`
	TotalTime      time.Duration `json:"total_time"`
}

// Benchmark runs compression benchmarks on test data
func (b *CompressionBenchmark) Benchmark(data []byte) []BenchmarkResult {
	var results []BenchmarkResult

	for _, compressor := range b.algorithms {
		result := BenchmarkResult{
			Algorithm:    compressor.GetType(),
			OriginalSize: len(data),
		}

		// Benchmark compression
		start := time.Now()
		compressed, err := compressor.Compress(data)
		result.CompressTime = time.Since(start)

		if err != nil {
			continue
		}

		result.CompressedSize = len(compressed)
		result.CompressionRatio = float64(result.OriginalSize) / float64(result.CompressedSize)

		// Benchmark decompression
		start = time.Now()
		_, err = compressor.Decompress(compressed)
		result.DecompressTime = time.Since(start)

		if err != nil {
			continue
		}

		result.TotalTime = result.CompressTime + result.DecompressTime
		results = append(results, result)
	}

	return results
}