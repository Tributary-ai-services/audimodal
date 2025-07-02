package registry

import (
	"context"
	"testing"

	"github.com/jscharber/eAIIngest/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock implementations for testing
type mockReader struct{}

func (m *mockReader) GetConfigSpec() []core.ConfigSpec {
	return []core.ConfigSpec{
		{Name: "path", Type: "string", Required: true, Description: "File path"},
	}
}

func (m *mockReader) ValidateConfig(config map[string]any) error {
	return nil
}

func (m *mockReader) TestConnection(ctx context.Context, config map[string]any) core.ConnectionTestResult {
	return core.ConnectionTestResult{Success: true, Message: "OK"}
}

func (m *mockReader) GetType() string    { return "reader" }
func (m *mockReader) GetName() string    { return "mock" }
func (m *mockReader) GetVersion() string { return "1.0.0" }

func (m *mockReader) DiscoverSchema(ctx context.Context, sourcePath string) (core.SchemaInfo, error) {
	return core.SchemaInfo{Format: "test"}, nil
}

func (m *mockReader) EstimateSize(ctx context.Context, sourcePath string) (core.SizeEstimate, error) {
	return core.SizeEstimate{ByteSize: 1024, Complexity: "low"}, nil
}

func (m *mockReader) CreateIterator(ctx context.Context, sourcePath string, strategyConfig map[string]any) (core.ChunkIterator, error) {
	return &mockIterator{}, nil
}

func (m *mockReader) SupportsStreaming() bool       { return true }
func (m *mockReader) GetSupportedFormats() []string { return []string{"txt", "csv"} }

type mockIterator struct{}

func (m *mockIterator) Next(ctx context.Context) (core.Chunk, error) {
	return core.Chunk{}, nil
}

func (m *mockIterator) Close() error      { return nil }
func (m *mockIterator) Reset() error      { return nil }
func (m *mockIterator) Progress() float64 { return 0.0 }

func TestRegistry(t *testing.T) {
	registry := NewRegistry()

	// Test registration
	registry.RegisterReader("mock", func() core.DataSourceReader {
		return &mockReader{}
	})

	// Test retrieval
	reader, err := registry.GetReader("mock")
	require.NoError(t, err)
	assert.Equal(t, "mock", reader.GetName())

	// Test listing
	readers := registry.ListReaders()
	assert.Contains(t, readers, "mock")

	// Test not found
	_, err = registry.GetReader("nonexistent")
	assert.Error(t, err)

	// Test plugin info
	info := registry.GetPluginInfo()
	assert.Len(t, info, 1)
	assert.Equal(t, "mock", info[0].Name)
	assert.Equal(t, "reader", info[0].Type)

	// Test stats
	stats := registry.GetStats()
	assert.Equal(t, 1, stats.ReaderCount)
	assert.Equal(t, 0, stats.StrategyCount)
}

func TestFindReaderByFormat(t *testing.T) {
	registry := NewRegistry()

	registry.RegisterReader("mock", func() core.DataSourceReader {
		return &mockReader{}
	})

	// Test finding by supported format
	name, reader, err := registry.FindReaderByFormat("csv")
	require.NoError(t, err)
	assert.Equal(t, "mock", name)
	assert.Equal(t, "mock", reader.GetName())

	// Test format not found
	_, _, err = registry.FindReaderByFormat("unknown")
	assert.Error(t, err)
}
