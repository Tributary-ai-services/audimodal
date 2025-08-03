package strategies

import (
	"context"
	"testing"
	"time"

	"github.com/jscharber/eAIIngest/pkg/core"
	"github.com/jscharber/eAIIngest/pkg/registry"
	"github.com/jscharber/eAIIngest/pkg/strategies/hybrid"
	"github.com/jscharber/eAIIngest/pkg/strategies/structured"
	"github.com/jscharber/eAIIngest/pkg/strategies/text"
)

func TestBasicStrategiesIntegration(t *testing.T) {
	// Create a new registry for testing
	testRegistry := registry.NewRegistry()

	// Register strategies with test registry
	testRegistry.RegisterStrategy("fixed_size_text", func() core.ChunkingStrategy {
		return text.NewFixedSizeStrategy()
	})
	testRegistry.RegisterStrategy("semantic_text", func() core.ChunkingStrategy {
		return text.NewSemanticStrategy()
	})
	testRegistry.RegisterStrategy("row_based_structured", func() core.ChunkingStrategy {
		return structured.NewRowBasedStrategy()
	})
	testRegistry.RegisterStrategy("adaptive_hybrid", func() core.ChunkingStrategy {
		return hybrid.NewAdaptiveStrategy()
	})

	// Test that all strategies are registered
	strategies := testRegistry.ListStrategies()
	expectedStrategies := []string{"adaptive_hybrid", "fixed_size_text", "row_based_structured", "semantic_text"}

	if len(strategies) != len(expectedStrategies) {
		t.Errorf("Expected %d strategies, got %d", len(expectedStrategies), len(strategies))
	}

	for _, expected := range expectedStrategies {
		found := false
		for _, strategy := range strategies {
			if strategy == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected strategy '%s' not found", expected)
		}
	}

	// Test strategy retrieval and validation
	for _, strategyName := range expectedStrategies {
		strategy, err := testRegistry.GetStrategy(strategyName)
		if err != nil {
			t.Errorf("Failed to get strategy '%s': %v", strategyName, err)
			continue
		}

		// Validate strategy implements interface correctly
		if err := testRegistry.ValidatePlugin("strategy", strategyName); err != nil {
			t.Errorf("Strategy '%s' failed validation: %v", strategyName, err)
		}

		// Test basic methods
		if strategy.GetName() == "" {
			t.Errorf("Strategy '%s' has empty name", strategyName)
		}

		if strategy.GetVersion() == "" {
			t.Errorf("Strategy '%s' has empty version", strategyName)
		}

		if strategy.GetType() != "strategy" {
			t.Errorf("Strategy '%s' has wrong type: %s", strategyName, strategy.GetType())
		}

		if len(strategy.GetConfigSpec()) == 0 {
			t.Errorf("Strategy '%s' has no config spec", strategyName)
		}
	}
}

func TestStrategyRecommendations(t *testing.T) {
	tests := []struct {
		dataType string
		expected string
	}{
		{"unstructured", "semantic_text"},
		{"structured", "row_based_structured"},
		{"semi_structured", "adaptive_hybrid"},
		{"text", "semantic_text"},
		{"csv", "row_based_structured"},
		{"json", "adaptive_hybrid"},
		{"unknown", "adaptive_hybrid"},
	}

	for _, test := range tests {
		result := GetStrategyForDataType(test.dataType)
		if result != test.expected {
			t.Errorf("For data type %s, expected strategy %s, got %s", test.dataType, test.expected, result)
		}
	}
}

func TestFileTypeRecommendations(t *testing.T) {
	tests := []struct {
		fileType string
		expected string
	}{
		{"txt", "semantic_text"},
		{"csv", "row_based_structured"},
		{"json", "adaptive_hybrid"},
		{"unknown", "adaptive_hybrid"},
	}

	for _, test := range tests {
		result := GetStrategyForFileType(test.fileType)
		if result != test.expected {
			t.Errorf("For file type %s, expected strategy %s, got %s", test.fileType, test.expected, result)
		}
	}
}

func TestEndToEndStrategyProcessing(t *testing.T) {
	// Register strategies
	RegisterBasicStrategies()

	ctx := context.Background()

	// Test data for different strategy types
	testCases := []struct {
		name         string
		strategy     string
		data         any
		expectedType string
		minChunks    int
	}{
		{
			name:     "Fixed size text",
			strategy: "fixed_size_text",
			data: "This is a long text that should be split into multiple chunks. " +
				"Each chunk should be roughly the same size. " +
				"This helps with consistent processing. " +
				"The strategy should split on sentence boundaries when possible.",
			expectedType: "text",
			minChunks:    1,
		},
		{
			name:     "Semantic text",
			strategy: "semantic_text",
			data: "# Introduction\n\nThis is the introduction paragraph.\n\n" +
				"## Section 1\n\nThis is section 1 content.\n\n" +
				"## Section 2\n\nThis is section 2 content.",
			expectedType: "text",
			minChunks:    1,
		},
		{
			name:     "Row-based structured",
			strategy: "row_based_structured",
			data: []map[string]any{
				{"name": "John", "age": 25, "city": "NYC"},
				{"name": "Jane", "age": 30, "city": "LA"},
				{"name": "Bob", "age": 35, "city": "Chicago"},
			},
			expectedType: "structured",
			minChunks:    1,
		},
		{
			name:         "Adaptive hybrid - text",
			strategy:     "adaptive_hybrid",
			data:         "This is text that should be detected and processed adaptively.",
			expectedType: "text",
			minChunks:    1,
		},
		{
			name:     "Adaptive hybrid - structured",
			strategy: "adaptive_hybrid",
			data: []map[string]any{
				{"id": 1, "value": "test1"},
				{"id": 2, "value": "test2"},
			},
			expectedType: "structured",
			minChunks:    1,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// Get strategy instance
			strategy, err := registry.GlobalRegistry.GetStrategy(testCase.strategy)
			if err != nil {
				t.Fatalf("Failed to get strategy %s: %v", testCase.strategy, err)
			}

			// Create metadata
			metadata := core.ChunkMetadata{
				SourcePath:  "test_file",
				ChunkID:     "test",
				ChunkType:   testCase.expectedType,
				ProcessedAt: time.Now(),
				Context:     map[string]string{},
			}

			// Process chunk
			chunks, err := strategy.ProcessChunk(ctx, testCase.data, metadata)
			if err != nil {
				t.Fatalf("ProcessChunk failed for %s: %v", testCase.name, err)
			}

			if len(chunks) < testCase.minChunks {
				t.Errorf("Expected at least %d chunks for %s, got %d", testCase.minChunks, testCase.name, len(chunks))
			}

			// Validate chunks
			for i, chunk := range chunks {
				if chunk.Data == nil {
					t.Errorf("Chunk %d has nil data", i)
				}

				if chunk.Metadata.ChunkID == "" {
					t.Errorf("Chunk %d has empty chunk ID", i)
				}

				if chunk.Metadata.ProcessedBy == "" {
					t.Errorf("Chunk %d has empty processed by field", i)
				}

				if chunk.Metadata.SizeBytes <= 0 {
					t.Errorf("Chunk %d has invalid size: %d", i, chunk.Metadata.SizeBytes)
				}

				if chunk.Metadata.Quality == nil {
					t.Errorf("Chunk %d has no quality metrics", i)
				}
			}
		})
	}
}

func TestStrategyValidation(t *testing.T) {
	// Register strategies
	RegisterBasicStrategies()

	// Validate all strategies
	if err := ValidateBasicStrategies(); err != nil {
		t.Errorf("Strategy validation failed: %v", err)
	}
}

func TestStrategyCapabilities(t *testing.T) {
	capabilities := GetStrategyCapabilities()

	if len(capabilities) == 0 {
		t.Error("Expected strategy capabilities")
	}

	expectedStrategies := map[string]bool{
		"fixed_size_text":      false,
		"semantic_text":        false,
		"row_based_structured": false,
		"adaptive_hybrid":      false,
	}

	for _, capability := range capabilities {
		if _, ok := expectedStrategies[capability.Name]; ok {
			expectedStrategies[capability.Name] = true
		}

		if capability.Description == "" {
			t.Errorf("Strategy %s has empty description", capability.Name)
		}

		if len(capability.BestFor) == 0 {
			t.Errorf("Strategy %s has no 'best for' descriptions", capability.Name)
		}

		if len(capability.DataTypes) == 0 {
			t.Errorf("Strategy %s has no supported data types", capability.Name)
		}
	}

	// Check all expected strategies are present
	for strategy, found := range expectedStrategies {
		if !found {
			t.Errorf("Strategy %s not found in capabilities", strategy)
		}
	}
}

func TestPerformanceHints(t *testing.T) {
	hints := GetPerformanceHints()

	if len(hints) == 0 {
		t.Error("Expected performance hints")
	}

	for _, hint := range hints {
		if hint.StrategyName == "" {
			t.Error("Performance hint has empty strategy name")
		}

		if hint.RecommendedBatch <= 0 {
			t.Errorf("Invalid recommended batch size for %s: %d", hint.StrategyName, hint.RecommendedBatch)
		}

		if hint.OptimalConfig == nil {
			t.Errorf("No optimal config provided for %s", hint.StrategyName)
		}
	}
}

func TestOptimalConfig(t *testing.T) {
	tests := []struct {
		strategy    string
		contentSize int64
		dataType    string
	}{
		{"fixed_size_text", 5000, "text"},
		{"semantic_text", 50000, "text"},
		{"row_based_structured", 1000000, "structured"},
		{"adaptive_hybrid", 10000, "mixed"},
	}

	for _, test := range tests {
		config := GetOptimalStrategyConfig(test.strategy, test.contentSize, test.dataType)

		if len(config) == 0 {
			t.Errorf("No optimal config returned for strategy %s", test.strategy)
		}

		// Validate that config is reasonable
		for key, value := range config {
			if value == nil {
				t.Errorf("Config key %s has nil value for strategy %s", key, test.strategy)
			}
		}
	}
}

func TestRecommendedStrategies(t *testing.T) {
	tests := []struct {
		contentType string
		size        int64
		complexity  string
		minCount    int
	}{
		{"text", 5000, "low", 1},
		{"structured", 100000, "medium", 1},
		{"mixed", 50000, "high", 1},
	}

	for _, test := range tests {
		strategies := GetRecommendedStrategies(test.contentType, test.size, test.complexity)

		if len(strategies) < test.minCount {
			t.Errorf("Expected at least %d recommended strategies for %s, got %d",
				test.minCount, test.contentType, len(strategies))
		}

		// Verify strategies exist
		for _, strategyName := range strategies {
			if strategyName == "" {
				t.Error("Empty strategy name in recommendations")
			}
		}
	}
}
