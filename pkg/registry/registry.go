// Package registry provides a plugin registry system for managing data source readers,
// chunking strategies, and content discoverers. It supports both static registration
// (at compile time) and dynamic plugin loading.
package registry

import (
	"fmt"
	"sort"
	"sync"

	"github.com/jscharber/eAIIngest/pkg/core"
)

// Registry manages the registration and retrieval of plugin components.
// It provides thread-safe access to readers, strategies, and discoverers.
type Registry struct {
	mu          sync.RWMutex
	readers     map[string]ReaderFactory
	strategies  map[string]StrategyFactory
	discoverers map[string]DiscovererFactory
}

// Factory function types for creating plugin instances
type (
	ReaderFactory     func() core.DataSourceReader
	StrategyFactory   func() core.ChunkingStrategy
	DiscovererFactory func() core.DataSourceDiscoverer
)

// PluginInfo provides metadata about a registered plugin
type PluginInfo struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"` // "reader", "strategy", "discoverer"
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Formats     []string `json:"formats,omitempty"`    // Supported formats for readers
	Categories  []string `json:"categories,omitempty"` // Categories for strategies
}

// GlobalRegistry is the default registry instance used throughout the application
var GlobalRegistry = NewRegistry()

// NewRegistry creates a new plugin registry
func NewRegistry() *Registry {
	return &Registry{
		readers:     make(map[string]ReaderFactory),
		strategies:  make(map[string]StrategyFactory),
		discoverers: make(map[string]DiscovererFactory),
	}
}

// RegisterReader registers a data source reader factory with the given name.
// If a reader with the same name already exists, it will be replaced.
func (r *Registry) RegisterReader(name string, factory ReaderFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.readers[name] = factory
}

// RegisterStrategy registers a chunking strategy factory with the given name.
// If a strategy with the same name already exists, it will be replaced.
func (r *Registry) RegisterStrategy(name string, factory StrategyFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.strategies[name] = factory
}

// RegisterDiscoverer registers a content discoverer factory with the given name.
// If a discoverer with the same name already exists, it will be replaced.
func (r *Registry) RegisterDiscoverer(name string, factory DiscovererFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.discoverers[name] = factory
}

// GetReader creates a new instance of the specified reader.
// Returns an error if the reader is not registered.
func (r *Registry) GetReader(name string) (core.DataSourceReader, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	factory, exists := r.readers[name]
	if !exists {
		return nil, &RegistryError{
			Type:    "READER_NOT_FOUND",
			Message: fmt.Sprintf("reader '%s' not found", name),
			Name:    name,
		}
	}

	return factory(), nil
}

// GetStrategy creates a new instance of the specified chunking strategy.
// Returns an error if the strategy is not registered.
func (r *Registry) GetStrategy(name string) (core.ChunkingStrategy, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	factory, exists := r.strategies[name]
	if !exists {
		return nil, &RegistryError{
			Type:    "STRATEGY_NOT_FOUND",
			Message: fmt.Sprintf("strategy '%s' not found", name),
			Name:    name,
		}
	}

	return factory(), nil
}

// GetDiscoverer creates a new instance of the specified content discoverer.
// Returns an error if the discoverer is not registered.
func (r *Registry) GetDiscoverer(name string) (core.DataSourceDiscoverer, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	factory, exists := r.discoverers[name]
	if !exists {
		return nil, &RegistryError{
			Type:    "DISCOVERER_NOT_FOUND",
			Message: fmt.Sprintf("discoverer '%s' not found", name),
			Name:    name,
		}
	}

	return factory(), nil
}

// ListReaders returns the names of all registered readers
func (r *Registry) ListReaders() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.readers))
	for name := range r.readers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ListStrategies returns the names of all registered chunking strategies
func (r *Registry) ListStrategies() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.strategies))
	for name := range r.strategies {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ListDiscoverers returns the names of all registered content discoverers
func (r *Registry) ListDiscoverers() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.discoverers))
	for name := range r.discoverers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// GetPluginInfo returns detailed information about all registered plugins
func (r *Registry) GetPluginInfo() []PluginInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var plugins []PluginInfo

	// Add reader info
	for name, factory := range r.readers {
		reader := factory()
		info := PluginInfo{
			Name:        name,
			Type:        "reader",
			Version:     reader.GetVersion(),
			Description: fmt.Sprintf("Data source reader: %s", name),
			Formats:     reader.GetSupportedFormats(),
		}
		plugins = append(plugins, info)
	}

	// Add strategy info
	for name, factory := range r.strategies {
		strategy := factory()
		info := PluginInfo{
			Name:        name,
			Type:        "strategy",
			Version:     strategy.GetVersion(),
			Description: fmt.Sprintf("Chunking strategy: %s", name),
		}
		plugins = append(plugins, info)
	}

	// Add discoverer info
	for name, factory := range r.discoverers {
		discoverer := factory()
		info := PluginInfo{
			Name:        name,
			Type:        "discoverer",
			Version:     discoverer.GetVersion(),
			Description: fmt.Sprintf("Content discoverer: %s", name),
		}
		plugins = append(plugins, info)
	}

	// Sort by type then name
	sort.Slice(plugins, func(i, j int) bool {
		if plugins[i].Type != plugins[j].Type {
			return plugins[i].Type < plugins[j].Type
		}
		return plugins[i].Name < plugins[j].Name
	})

	return plugins
}

// UnregisterReader removes a reader from the registry
func (r *Registry) UnregisterReader(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.readers[name]; exists {
		delete(r.readers, name)
		return true
	}
	return false
}

// UnregisterStrategy removes a strategy from the registry
func (r *Registry) UnregisterStrategy(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.strategies[name]; exists {
		delete(r.strategies, name)
		return true
	}
	return false
}

// UnregisterDiscoverer removes a discoverer from the registry
func (r *Registry) UnregisterDiscoverer(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.discoverers[name]; exists {
		delete(r.discoverers, name)
		return true
	}
	return false
}

// Clear removes all registered plugins from the registry
func (r *Registry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.readers = make(map[string]ReaderFactory)
	r.strategies = make(map[string]StrategyFactory)
	r.discoverers = make(map[string]DiscovererFactory)
}

// Count returns the total number of registered plugins
func (r *Registry) Count() (readers, strategies, discoverers int) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.readers), len(r.strategies), len(r.discoverers)
}

// FindReaderByFormat finds a reader that supports the specified file format.
// Returns the first matching reader found, or an error if none support the format.
func (r *Registry) FindReaderByFormat(format string) (string, core.DataSourceReader, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for name, factory := range r.readers {
		reader := factory()
		for _, supportedFormat := range reader.GetSupportedFormats() {
			if supportedFormat == format {
				return name, reader, nil
			}
		}
	}

	return "", nil, &RegistryError{
		Type:    "NO_READER_FOR_FORMAT",
		Message: fmt.Sprintf("no reader found for format '%s'", format),
		Name:    format,
	}
}

// ValidatePlugin validates that a plugin properly implements its interface
func (r *Registry) ValidatePlugin(pluginType, name string) error {
	switch pluginType {
	case "reader":
		reader, err := r.GetReader(name)
		if err != nil {
			return err
		}
		return r.validateReader(reader)
	case "strategy":
		strategy, err := r.GetStrategy(name)
		if err != nil {
			return err
		}
		return r.validateStrategy(strategy)
	case "discoverer":
		discoverer, err := r.GetDiscoverer(name)
		if err != nil {
			return err
		}
		return r.validateDiscoverer(discoverer)
	default:
		return &RegistryError{
			Type:    "INVALID_PLUGIN_TYPE",
			Message: fmt.Sprintf("invalid plugin type '%s'", pluginType),
			Name:    pluginType,
		}
	}
}

// validateReader ensures a reader implements the interface correctly
func (r *Registry) validateReader(reader core.DataSourceReader) error {
	// Check required methods return valid data
	if reader.GetName() == "" {
		return &RegistryError{
			Type:    "INVALID_READER",
			Message: "reader name cannot be empty",
		}
	}
	if reader.GetVersion() == "" {
		return &RegistryError{
			Type:    "INVALID_READER",
			Message: "reader version cannot be empty",
		}
	}
	if reader.GetType() != "reader" {
		return &RegistryError{
			Type:    "INVALID_READER",
			Message: fmt.Sprintf("reader type must be 'reader', got '%s'", reader.GetType()),
		}
	}
	if len(reader.GetConfigSpec()) == 0 {
		return &RegistryError{
			Type:    "INVALID_READER",
			Message: "reader must provide config spec",
		}
	}
	return nil
}

// validateStrategy ensures a strategy implements the interface correctly
func (r *Registry) validateStrategy(strategy core.ChunkingStrategy) error {
	if strategy.GetName() == "" {
		return &RegistryError{
			Type:    "INVALID_STRATEGY",
			Message: "strategy name cannot be empty",
		}
	}
	if strategy.GetVersion() == "" {
		return &RegistryError{
			Type:    "INVALID_STRATEGY",
			Message: "strategy version cannot be empty",
		}
	}
	if strategy.GetType() != "strategy" {
		return &RegistryError{
			Type:    "INVALID_STRATEGY",
			Message: fmt.Sprintf("strategy type must be 'strategy', got '%s'", strategy.GetType()),
		}
	}
	return nil
}

// validateDiscoverer ensures a discoverer implements the interface correctly
func (r *Registry) validateDiscoverer(discoverer core.DataSourceDiscoverer) error {
	if discoverer.GetName() == "" {
		return &RegistryError{
			Type:    "INVALID_DISCOVERER",
			Message: "discoverer name cannot be empty",
		}
	}
	if discoverer.GetVersion() == "" {
		return &RegistryError{
			Type:    "INVALID_DISCOVERER",
			Message: "discoverer version cannot be empty",
		}
	}
	if discoverer.GetType() != "discoverer" {
		return &RegistryError{
			Type:    "INVALID_DISCOVERER",
			Message: fmt.Sprintf("discoverer type must be 'discoverer', got '%s'", discoverer.GetType()),
		}
	}
	return nil
}

// RegistryError represents errors that occur during registry operations
type RegistryError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Name    string `json:"name,omitempty"`
}

func (e *RegistryError) Error() string {
	return e.Message
}

// RegistryStats provides statistics about the registry
type RegistryStats struct {
	ReaderCount     int      `json:"reader_count"`
	StrategyCount   int      `json:"strategy_count"`
	DiscovererCount int      `json:"discoverer_count"`
	TotalPlugins    int      `json:"total_plugins"`
	ReaderNames     []string `json:"reader_names"`
	StrategyNames   []string `json:"strategy_names"`
	DiscovererNames []string `json:"discoverer_names"`
}

// GetStats returns current registry statistics
func (r *Registry) GetStats() RegistryStats {
	readers, strategies, discoverers := r.Count()

	return RegistryStats{
		ReaderCount:     readers,
		StrategyCount:   strategies,
		DiscovererCount: discoverers,
		TotalPlugins:    readers + strategies + discoverers,
		ReaderNames:     r.ListReaders(),
		StrategyNames:   r.ListStrategies(),
		DiscovererNames: r.ListDiscoverers(),
	}
}
