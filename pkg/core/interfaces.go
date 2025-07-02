// Package core defines the foundational interfaces and types for the document processing platform.
// This package establishes the plugin architecture that allows extensible readers, chunking strategies,
// and content discoverers while maintaining type safety and consistent patterns.
package core

import (
	"context"
	"fmt"
	"time"
)

// Connector is the base interface that all plugin components must implement.
// It provides common functionality for configuration, validation, and introspection.
type Connector interface {
	// GetConfigSpec returns the configuration specification for this connector.
	// This allows the system to automatically generate UIs, validate configs, and provide documentation.
	GetConfigSpec() []ConfigSpec

	// ValidateConfig validates the provided configuration against the connector's requirements.
	// It should return a descriptive error if the configuration is invalid.
	ValidateConfig(config map[string]any) error

	// TestConnection tests if the connector can work with the given configuration.
	// This is used for health checks and setup validation.
	TestConnection(ctx context.Context, config map[string]any) ConnectionTestResult

	// GetType returns the connector type category (reader, strategy, discoverer).
	GetType() string

	// GetName returns the unique name identifier for this connector.
	GetName() string

	// GetVersion returns the version of this connector implementation.
	GetVersion() string
}

// DataSourceReader defines the interface for reading data from various sources.
// Implementations handle different file formats and storage systems while providing
// a consistent interface for schema discovery, size estimation, and chunk iteration.
type DataSourceReader interface {
	Connector

	// DiscoverSchema analyzes the data source and returns structural information.
	// This includes field definitions, data types, and metadata about the source.
	DiscoverSchema(ctx context.Context, sourcePath string) (SchemaInfo, error)

	// EstimateSize returns size and complexity estimates for the data source.
	// Used for processing tier determination and resource planning.
	EstimateSize(ctx context.Context, sourcePath string) (SizeEstimate, error)

	// CreateIterator creates a chunk iterator configured for the specified strategy.
	// The strategy config influences how the reader prepares and presents data.
	CreateIterator(ctx context.Context, sourcePath string, strategyConfig map[string]any) (ChunkIterator, error)

	// SupportsStreaming indicates whether this reader can process data without
	// loading entire files into memory. Critical for large file handling.
	SupportsStreaming() bool

	// GetSupportedFormats returns the list of file formats this reader can handle.
	// Used for automatic reader selection based on file type.
	GetSupportedFormats() []string
}

// ChunkingStrategy defines how raw data should be divided into processable chunks.
// Different strategies optimize for different use cases: semantic coherence,
// compliance requirements, or processing efficiency.
type ChunkingStrategy interface {
	Connector

	// GetRequiredReaderCapabilities returns capabilities the reader must support.
	// For example: ["streaming", "random_access", "column_iteration"]
	GetRequiredReaderCapabilities() []string

	// ConfigureReader modifies reader configuration to optimize for this strategy.
	// Allows strategies to influence how readers prepare data for processing.
	ConfigureReader(readerConfig map[string]any) (map[string]any, error)

	// ProcessChunk takes raw data and metadata, applies the chunking strategy,
	// and returns one or more processed chunks ready for embedding.
	ProcessChunk(ctx context.Context, rawData any, metadata ChunkMetadata) ([]Chunk, error)

	// GetOptimalChunkSize recommends chunk size based on the source schema.
	// Helps optimize processing performance and embedding quality.
	GetOptimalChunkSize(sourceSchema SchemaInfo) int

	// SupportsParallelProcessing indicates if chunks can be processed concurrently.
	// Some strategies require sequential processing to maintain context.
	SupportsParallelProcessing() bool
}

// DataSourceDiscoverer analyzes data sources to recommend optimal processing strategies.
// It examines content patterns, structure, and characteristics to suggest the best
// combination of readers and chunking strategies.
type DataSourceDiscoverer interface {
	Connector

	// AnalyzeSource performs comprehensive analysis of the data source structure and content.
	// Returns hints about data patterns, content types, and processing recommendations.
	AnalyzeSource(ctx context.Context, sourcePath string) (DiscoveryHints, error)

	// RecommendStrategies suggests optimal processing strategies based on analysis results.
	// Considers schema, content hints, and processing requirements to rank strategies.
	RecommendStrategies(ctx context.Context, schema SchemaInfo, hints DiscoveryHints, requirements map[string]any) ([]RecommendedStrategy, error)

	// GetSupportedSourceTypes returns the types of sources this discoverer can analyze.
	// Used for automatic discoverer selection based on source characteristics.
	GetSupportedSourceTypes() []string
}

// ChunkIterator provides iteration over data chunks from a source.
// Implementations handle the complexity of reading, parsing, and presenting
// data in a streaming, memory-efficient manner.
type ChunkIterator interface {
	// Next returns the next chunk of data, or an error when iteration is complete.
	// Should return a specific error (like EOF) when no more data is available.
	// The context allows for cancellation of long-running operations.
	Next(ctx context.Context) (Chunk, error)

	// Close releases any resources held by the iterator (files, connections, etc.).
	// Must be called when iteration is complete, typically in a defer statement.
	Close() error

	// Reset restarts iteration from the beginning of the data source.
	// Useful for multi-pass processing or error recovery scenarios.
	Reset() error

	// Progress returns the current iteration progress as a value between 0.0 and 1.0.
	// Used for progress reporting in long-running operations.
	Progress() float64
}

// URLResolver defines the interface for resolving URLs to accessible files.
// Each resolver implementation handles a specific URL scheme (file, s3, gdrive, etc.)
type URLResolver interface {
	// Supports returns true if this resolver can handle the given URL scheme
	Supports(scheme string) bool

	// Resolve converts a URL into a ResolvedFile with access information
	Resolve(ctx context.Context, fileURL string) (*ResolvedFile, error)

	// CreateIterator creates a chunk iterator for the file at the given URL
	CreateIterator(ctx context.Context, fileURL string, config map[string]any) (ChunkIterator, error)

	// GetName returns the name of this resolver
	GetName() string

	// GetVersion returns the version of this resolver
	GetVersion() string
}

// ConfigSpec defines a configuration parameter for connectors.
// Used for automatic validation, UI generation, and documentation.
type ConfigSpec struct {
	Name        string   `json:"name"`                // Parameter name
	Type        string   `json:"type"`                // Data type: "string", "int", "bool", "object", "array"
	Required    bool     `json:"required"`            // Whether this parameter is mandatory
	Default     any      `json:"default,omitempty"`   // Default value if not specified
	Description string   `json:"description"`         // Human-readable description
	Enum        []string `json:"enum,omitempty"`      // Valid values for enumerated types
	MinValue    *float64 `json:"min_value,omitempty"` // Minimum value for numeric types
	MaxValue    *float64 `json:"max_value,omitempty"` // Maximum value for numeric types
	Pattern     string   `json:"pattern,omitempty"`   // Regex pattern for string validation
	Examples    []string `json:"examples,omitempty"`  // Example values for documentation
}

// ConnectionTestResult represents the outcome of testing a connector's configuration.
type ConnectionTestResult struct {
	Success bool           `json:"success"`           // Whether the test succeeded
	Message string         `json:"message"`           // Human-readable result message
	Details map[string]any `json:"details,omitempty"` // Additional diagnostic information
	Latency time.Duration  `json:"latency,omitempty"` // Time taken for the test
	Errors  []string       `json:"errors,omitempty"`  // List of specific errors encountered
}

// Chunk represents a processed unit of data ready for further processing or embedding.
type Chunk struct {
	Data     any           `json:"data"`     // The actual chunk content (text, structured data, etc.)
	Metadata ChunkMetadata `json:"metadata"` // Metadata about this chunk
}

// ChunkMetadata contains information about a chunk's source, position, and characteristics.
type ChunkMetadata struct {
	SourcePath    string            `json:"source_path"`              // Original source file/URL
	ChunkID       string            `json:"chunk_id"`                 // Unique identifier for this chunk
	ChunkType     string            `json:"chunk_type"`               // Type: "text", "table", "image", etc.
	SizeBytes     int64             `json:"size_bytes"`               // Size of chunk content in bytes
	StartPosition *int64            `json:"start_position,omitempty"` // Byte offset in source file
	EndPosition   *int64            `json:"end_position,omitempty"`   // End byte offset in source file
	SchemaInfo    map[string]any    `json:"schema_info,omitempty"`    // Structure information for this chunk
	Relationships []string          `json:"relationships,omitempty"`  // IDs of related chunks
	ProcessedAt   time.Time         `json:"processed_at"`             // When this chunk was created
	ProcessedBy   string            `json:"processed_by"`             // Strategy that created this chunk
	Quality       *QualityMetrics   `json:"quality,omitempty"`        // Quality assessment of chunk content
	Context       map[string]string `json:"context,omitempty"`        // Additional contextual information
}

// QualityMetrics provides assessment of chunk content quality and characteristics.
type QualityMetrics struct {
	Completeness float64 `json:"completeness"`  // How complete the content appears (0.0-1.0)
	Coherence    float64 `json:"coherence"`     // How semantically coherent the content is
	Uniqueness   float64 `json:"uniqueness"`    // How unique this chunk is vs others
	Readability  float64 `json:"readability"`   // Text readability score
	LanguageConf float64 `json:"language_conf"` // Confidence in detected language
	Language     string  `json:"language"`      // Detected language code
}

// SchemaInfo represents discovered structural information about a data source.
type SchemaInfo struct {
	Fields      []FieldInfo      `json:"fields"`                // Field/column definitions
	Constraints map[string]any   `json:"constraints,omitempty"` // Schema constraints (foreign keys, etc.)
	Metadata    map[string]any   `json:"metadata,omitempty"`    // Additional schema metadata
	SampleData  []map[string]any `json:"sample_data,omitempty"` // Sample records for analysis
	Format      string           `json:"format"`                // Data format: "structured", "unstructured", "semi_structured"
	Encoding    string           `json:"encoding,omitempty"`    // Character encoding
	Delimiter   string           `json:"delimiter,omitempty"`   // Field delimiter for delimited formats
	Version     string           `json:"version,omitempty"`     // Schema version if applicable
}

// FieldInfo describes a single field or column in the data source.
type FieldInfo struct {
	Name        string      `json:"name"`                  // Field name
	Type        string      `json:"type"`                  // Data type
	Nullable    bool        `json:"nullable"`              // Whether field can be null
	Primary     bool        `json:"primary,omitempty"`     // Whether this is a primary key field
	Description string      `json:"description,omitempty"` // Field description
	Format      string      `json:"format,omitempty"`      // Format specification (e.g., date format)
	Examples    []any       `json:"examples,omitempty"`    // Example values
	Statistics  *FieldStats `json:"statistics,omitempty"`  // Statistical information about field values
}

// FieldStats provides statistical information about field values.
type FieldStats struct {
	Count       int64   `json:"count"`                // Number of non-null values
	Distinct    int64   `json:"distinct,omitempty"`   // Number of distinct values
	Min         any     `json:"min,omitempty"`        // Minimum value
	Max         any     `json:"max,omitempty"`        // Maximum value
	Mean        float64 `json:"mean,omitempty"`       // Mean value for numeric fields
	Median      float64 `json:"median,omitempty"`     // Median value for numeric fields
	StdDev      float64 `json:"std_dev,omitempty"`    // Standard deviation for numeric fields
	TopValues   []any   `json:"top_values,omitempty"` // Most common values
	NullPercent float64 `json:"null_percent"`         // Percentage of null values
}

// SizeEstimate provides information about data source size and complexity.
type SizeEstimate struct {
	RowCount    *int64 `json:"row_count,omitempty"`    // Estimated number of records/rows
	ByteSize    int64  `json:"byte_size"`              // Total size in bytes
	ColumnCount *int   `json:"column_count,omitempty"` // Number of columns for structured data
	Complexity  string `json:"complexity"`             // "low", "medium", "high" - processing difficulty
	ChunkEst    int    `json:"chunk_estimate"`         // Estimated number of chunks this will produce
	ProcessTime string `json:"process_time_estimate"`  // Estimated processing time
}

// DiscoveryHints provides information discovered through content analysis.
type DiscoveryHints struct {
	HasTimestamps    bool           `json:"has_timestamps"`           // Contains timestamp data
	HasTextContent   bool           `json:"has_text_content"`         // Contains natural language text
	HasRelationships bool           `json:"has_relationships"`        // Contains relational data
	ContentPatterns  []string       `json:"content_patterns"`         // Detected content patterns
	Languages        []string       `json:"languages"`                // Detected languages
	DataClasses      []string       `json:"data_classes"`             // Classifications: "personal", "financial", etc.
	CustomHints      map[string]any `json:"custom_hints,omitempty"`   // Discoverer-specific hints
	Confidence       float64        `json:"confidence"`               // Confidence in these hints (0.0-1.0)
	SampleContent    []string       `json:"sample_content,omitempty"` // Content samples for strategy selection
}

// RecommendedStrategy represents a suggested processing approach.
type RecommendedStrategy struct {
	StrategyType string         `json:"strategy_type"`          // Name of recommended strategy
	Confidence   float64        `json:"confidence"`             // Confidence in this recommendation (0.0-1.0)
	Config       map[string]any `json:"config"`                 // Recommended configuration parameters
	Reasoning    string         `json:"reasoning"`              // Human-readable explanation of recommendation
	Performance  *PerfEstimate  `json:"performance,omitempty"`  // Expected performance characteristics
	Alternatives []string       `json:"alternatives,omitempty"` // Alternative strategies to consider
}

// PerfEstimate provides expected performance characteristics for a strategy.
type PerfEstimate struct {
	ThroughputMBps    float64 `json:"throughput_mbps"`    // Expected processing throughput
	MemoryUsageMB     int     `json:"memory_usage_mb"`    // Expected memory usage
	CPUIntensive      bool    `json:"cpu_intensive"`      // Whether strategy is CPU-intensive
	IOIntensive       bool    `json:"io_intensive"`       // Whether strategy is I/O-intensive
	ParallelEfficient bool    `json:"parallel_efficient"` // Whether strategy benefits from parallelization
}

// ProcessingContext provides additional context for processing operations.
type ProcessingContext struct {
	TenantID       string            `json:"tenant_id"`                 // Tenant identifier for multi-tenant scenarios
	SessionID      string            `json:"session_id"`                // Processing session identifier
	RequestID      string            `json:"request_id"`                // Request correlation identifier
	UserID         string            `json:"user_id"`                   // User who initiated processing
	Priority       string            `json:"priority"`                  // Processing priority: "low", "normal", "high"
	Deadline       *time.Time        `json:"deadline,omitempty"`        // Processing deadline
	Metadata       map[string]string `json:"metadata,omitempty"`        // Additional context metadata
	ComplianceReqs []string          `json:"compliance_reqs,omitempty"` // Required compliance frameworks
	DataResidency  []string          `json:"data_residency,omitempty"`  // Data residency requirements
}

// ResolvedFile contains all information needed to access a file from a URL
type ResolvedFile struct {
	URL          string            `json:"url"`                     // Original URL
	Scheme       string            `json:"scheme"`                  // URL scheme (file, s3, etc.)
	Size         int64             `json:"size"`                    // File size in bytes
	ContentType  string            `json:"content_type"`            // MIME content type
	Metadata     map[string]string `json:"metadata"`                // Additional file metadata
	AccessInfo   AccessInfo        `json:"access_info"`             // How to access this file
	LastModified int64             `json:"last_modified,omitempty"` // Unix timestamp
}

// AccessInfo contains authentication and access details for a file
type AccessInfo struct {
	RequiresAuth bool              `json:"requires_auth"`          // Whether authentication is needed
	Credentials  string            `json:"credentials"`            // Credential reference (secret name, etc.)
	Headers      map[string]string `json:"headers,omitempty"`      // HTTP headers needed
	Region       string            `json:"region,omitempty"`       // Cloud region for regional services
	Endpoint     string            `json:"endpoint,omitempty"`     // Custom endpoint URL
	AccessToken  string            `json:"access_token,omitempty"` // Direct access token (use carefully)
}

// URLError represents errors that occur during URL resolution
type URLError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	URL     string `json:"url,omitempty"`
	Scheme  string `json:"scheme,omitempty"`
}

func (e *URLError) Error() string {
	if e.URL != "" {
		return fmt.Sprintf("%s: %s", e.URL, e.Message)
	}
	return e.Message
}

// Error types for structured error handling across the plugin system.
var (
	// ErrUnsupportedFormat indicates the reader doesn't support the requested format
	ErrUnsupportedFormat = NewPluginError("UNSUPPORTED_FORMAT", "unsupported file format")

	// ErrInvalidConfig indicates the provided configuration is invalid
	ErrInvalidConfig = NewPluginError("INVALID_CONFIG", "invalid configuration")

	// ErrConnectionFailed indicates connection to the data source failed
	ErrConnectionFailed = NewPluginError("CONNECTION_FAILED", "failed to connect to data source")

	// ErrIteratorExhausted indicates the iterator has no more data
	ErrIteratorExhausted = NewPluginError("ITERATOR_EXHAUSTED", "no more data available")

	// ErrProcessingFailed indicates chunk processing failed
	ErrProcessingFailed = NewPluginError("PROCESSING_FAILED", "chunk processing failed")
)

// PluginError provides structured error information for plugin operations.
type PluginError struct {
	Code    string `json:"code"`              // Error code for programmatic handling
	Message string `json:"message"`           // Human-readable error message
	Details any    `json:"details,omitempty"` // Additional error details
}

func (e *PluginError) Error() string {
	return e.Message
}

// NewPluginError creates a new structured plugin error.
func NewPluginError(code, message string) *PluginError {
	return &PluginError{
		Code:    code,
		Message: message,
	}
}

// WithDetails adds additional details to a plugin error.
func (e *PluginError) WithDetails(details any) *PluginError {
	return &PluginError{
		Code:    e.Code,
		Message: e.Message,
		Details: details,
	}
}
