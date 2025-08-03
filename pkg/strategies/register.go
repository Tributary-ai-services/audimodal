package strategies

import (
	"github.com/jscharber/eAIIngest/pkg/core"
	"github.com/jscharber/eAIIngest/pkg/registry"
	"github.com/jscharber/eAIIngest/pkg/strategies/hybrid"
	"github.com/jscharber/eAIIngest/pkg/strategies/structured"
	"github.com/jscharber/eAIIngest/pkg/strategies/text"
)

// RegisterBasicStrategies registers all basic chunking strategies with the global registry
func RegisterBasicStrategies() {
	// Register text strategies
	registry.GlobalRegistry.RegisterStrategy("fixed_size_text", func() core.ChunkingStrategy {
		return text.NewFixedSizeStrategy()
	})

	registry.GlobalRegistry.RegisterStrategy("semantic_text", func() core.ChunkingStrategy {
		return text.NewSemanticStrategy()
	})

	// Register structured data strategies
	registry.GlobalRegistry.RegisterStrategy("row_based_structured", func() core.ChunkingStrategy {
		return structured.NewRowBasedStrategy()
	})

	// Register hybrid strategies
	registry.GlobalRegistry.RegisterStrategy("adaptive_hybrid", func() core.ChunkingStrategy {
		return hybrid.NewAdaptiveStrategy()
	})
}

// GetStrategyForDataType returns the recommended strategy name for a data type
func GetStrategyForDataType(dataType string) string {
	switch dataType {
	case "unstructured":
		return "semantic_text"
	case "structured":
		return "row_based_structured"
	case "semi_structured":
		return "adaptive_hybrid"
	case "text":
		return "semantic_text"
	case "csv", "table":
		return "row_based_structured"
	case "json", "jsonl":
		return "adaptive_hybrid"
	case "mixed":
		return "adaptive_hybrid"
	default:
		return "adaptive_hybrid" // Default to adaptive for unknown types
	}
}

// GetStrategyForFileType returns the recommended strategy name for a file type
func GetStrategyForFileType(fileType string) string {
	switch fileType {
	case "txt", "text", "md", "markdown", "log":
		return "semantic_text"
	case "csv", "tsv":
		return "row_based_structured"
	case "json", "jsonl", "ndjson":
		return "adaptive_hybrid"
	default:
		return "adaptive_hybrid"
	}
}

// GetRecommendedStrategies returns a list of recommended strategies for given content characteristics
func GetRecommendedStrategies(contentType string, size int64, complexity string) []string {
	var strategies []string

	switch contentType {
	case "text":
		if size < 10*1024 { // < 10KB
			strategies = append(strategies, "fixed_size_text")
		} else {
			strategies = append(strategies, "semantic_text", "fixed_size_text")
		}
	case "structured":
		strategies = append(strategies, "row_based_structured")
		if complexity == "high" {
			strategies = append(strategies, "adaptive_hybrid")
		}
	case "mixed", "semi_structured":
		strategies = append(strategies, "adaptive_hybrid")
		if size < 1024*1024 { // < 1MB
			strategies = append(strategies, "semantic_text")
		}
	default:
		strategies = append(strategies, "adaptive_hybrid", "semantic_text")
	}

	return strategies
}

// StrategyCapabilities describes what each strategy is good for
type StrategyCapabilities struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	BestFor     []string `json:"best_for"`
	DataTypes   []string `json:"data_types"`
	Complexity  string   `json:"complexity"`
	Performance string   `json:"performance"`
	MemoryUsage string   `json:"memory_usage"`
}

// GetStrategyCapabilities returns detailed information about all registered strategies
func GetStrategyCapabilities() []StrategyCapabilities {
	return []StrategyCapabilities{
		{
			Name:        "fixed_size_text",
			Description: "Splits text into fixed-size chunks with optional overlap",
			BestFor:     []string{"simple text processing", "consistent chunk sizes", "fast processing"},
			DataTypes:   []string{"text", "unstructured"},
			Complexity:  "low",
			Performance: "high",
			MemoryUsage: "low",
		},
		{
			Name:        "semantic_text",
			Description: "Splits text based on semantic boundaries (paragraphs, sentences)",
			BestFor:     []string{"natural language", "documents", "articles", "better context preservation"},
			DataTypes:   []string{"text", "unstructured", "documents"},
			Complexity:  "medium",
			Performance: "medium",
			MemoryUsage: "medium",
		},
		{
			Name:        "row_based_structured",
			Description: "Groups structured data rows into chunks",
			BestFor:     []string{"CSV files", "database tables", "spreadsheets", "tabular data"},
			DataTypes:   []string{"structured", "csv", "table"},
			Complexity:  "low",
			Performance: "high",
			MemoryUsage: "low",
		},
		{
			Name:        "adaptive_hybrid",
			Description: "Automatically adapts to different data types and structures",
			BestFor:     []string{"mixed content", "unknown data types", "JSON", "flexible processing"},
			DataTypes:   []string{"mixed", "semi_structured", "json", "unknown"},
			Complexity:  "high",
			Performance: "medium",
			MemoryUsage: "medium",
		},
	}
}

// ValidateBasicStrategies validates all registered basic strategies
func ValidateBasicStrategies() error {
	strategies := []string{"fixed_size_text", "semantic_text", "row_based_structured", "adaptive_hybrid"}

	for _, name := range strategies {
		if err := registry.GlobalRegistry.ValidatePlugin("strategy", name); err != nil {
			return err
		}
	}

	return nil
}

// GetOptimalStrategyConfig returns optimal configuration for a strategy given content characteristics
func GetOptimalStrategyConfig(strategyName string, contentSize int64, dataType string) map[string]any {
	config := make(map[string]any)

	switch strategyName {
	case "fixed_size_text":
		// Adjust chunk size based on content size
		chunkSize := 1000
		if contentSize < 10*1024 { // < 10KB
			chunkSize = 500
		} else if contentSize > 1024*1024 { // > 1MB
			chunkSize = 2000
		}
		config["chunk_size"] = chunkSize
		config["overlap_size"] = chunkSize / 10
		config["split_on_sentences"] = true

	case "semantic_text":
		// Optimize for semantic coherence
		config["max_chunk_size"] = 1500
		config["min_chunk_size"] = 200
		config["respect_sections"] = true
		config["prefer_sentence_breaks"] = true

	case "row_based_structured":
		// Optimize for data size and complexity
		rowsPerChunk := 100
		if contentSize > 10*1024*1024 { // > 10MB
			rowsPerChunk = 50 // Smaller chunks for large datasets
		} else if contentSize < 1024*1024 { // < 1MB
			rowsPerChunk = 200 // Larger chunks for small datasets
		}
		config["rows_per_chunk"] = rowsPerChunk
		config["include_headers"] = true
		config["output_format"] = "records"

	case "adaptive_hybrid":
		// Balance between different data types
		config["auto_detect_type"] = true
		config["mixed_content_strategy"] = "separate"
		config["preserve_structure"] = true
		config["quality_threshold"] = 0.5
	}

	return config
}

// StrategyPerformanceHints provides performance optimization hints for strategies
type StrategyPerformanceHints struct {
	StrategyName     string         `json:"strategy_name"`
	ParallelSafe     bool           `json:"parallel_safe"`
	MemoryEfficient  bool           `json:"memory_efficient"`
	RecommendedBatch int            `json:"recommended_batch_size"`
	OptimalConfig    map[string]any `json:"optimal_config"`
	Warnings         []string       `json:"warnings"`
}

// GetPerformanceHints returns performance optimization hints for each strategy
func GetPerformanceHints() []StrategyPerformanceHints {
	return []StrategyPerformanceHints{
		{
			StrategyName:     "fixed_size_text",
			ParallelSafe:     true,
			MemoryEfficient:  true,
			RecommendedBatch: 1000,
			OptimalConfig: map[string]any{
				"chunk_size":         1000,
				"overlap_size":       100,
				"split_on_sentences": true,
			},
			Warnings: []string{"May split sentences mid-word if text has no punctuation"},
		},
		{
			StrategyName:     "semantic_text",
			ParallelSafe:     true,
			MemoryEfficient:  false,
			RecommendedBatch: 500,
			OptimalConfig: map[string]any{
				"max_chunk_size":         2000,
				"respect_sections":       true,
				"prefer_sentence_breaks": true,
			},
			Warnings: []string{"Higher memory usage due to text analysis", "Slower than fixed-size for simple text"},
		},
		{
			StrategyName:     "row_based_structured",
			ParallelSafe:     true,
			MemoryEfficient:  true,
			RecommendedBatch: 2000,
			OptimalConfig: map[string]any{
				"rows_per_chunk":  100,
				"include_headers": true,
				"output_format":   "records",
			},
			Warnings: []string{"Assumes consistent row structure", "May create large chunks with wide tables"},
		},
		{
			StrategyName:     "adaptive_hybrid",
			ParallelSafe:     true,
			MemoryEfficient:  false,
			RecommendedBatch: 250,
			OptimalConfig: map[string]any{
				"auto_detect_type":       true,
				"mixed_content_strategy": "separate",
				"quality_threshold":      0.5,
			},
			Warnings: []string{"Higher CPU overhead due to analysis", "May not be optimal for homogeneous data"},
		},
	}
}
