package storage

import (
	"fmt"
	"sync"
)

// ConnectorRegistry manages available connectors
type ConnectorRegistry interface {
	// Register adds a connector to the registry
	Register(name string, connector StorageConnector) error

	// Get retrieves a connector by name
	Get(name string) (StorageConnector, error)

	// List returns all registered connector names
	List() []string
}

// DefaultConnectorRegistry is the default implementation
type DefaultConnectorRegistry struct {
	mu         sync.RWMutex
	connectors map[string]StorageConnector
}

// NewConnectorRegistry creates a new connector registry
func NewConnectorRegistry() ConnectorRegistry {
	return &DefaultConnectorRegistry{
		connectors: make(map[string]StorageConnector),
	}
}

// Register adds a connector to the registry
func (r *DefaultConnectorRegistry) Register(name string, connector StorageConnector) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.connectors[name]; exists {
		return fmt.Errorf("connector %s already registered", name)
	}

	r.connectors[name] = connector
	return nil
}

// Get retrieves a connector by name
func (r *DefaultConnectorRegistry) Get(name string) (StorageConnector, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	connector, exists := r.connectors[name]
	if !exists {
		return nil, fmt.Errorf("connector %s not found", name)
	}

	return connector, nil
}

// List returns all registered connector names
func (r *DefaultConnectorRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.connectors))
	for name := range r.connectors {
		names = append(names, name)
	}
	return names
}

// DefaultConnectorStorageManager implements ConnectorStorageManager
type DefaultConnectorStorageManager struct {
	localStore LocalStore
	registry   ConnectorRegistry
}

// NewConnectorStorageManager creates a new connector storage manager
func NewConnectorStorageManager(localStore LocalStore) ConnectorStorageManager {
	return &DefaultConnectorStorageManager{
		localStore: localStore,
		registry:   NewConnectorRegistry(),
	}
}

// GetLocalStore returns the local storage interface
func (m *DefaultConnectorStorageManager) GetLocalStore() LocalStore {
	return m.localStore
}

// GetConnector returns a connector for the specified type
func (m *DefaultConnectorStorageManager) GetConnector(connectorType string) (StorageConnector, error) {
	return m.registry.Get(connectorType)
}

// RegisterConnector registers a new connector
func (m *DefaultConnectorStorageManager) RegisterConnector(connectorType string, connector StorageConnector) error {
	return m.registry.Register(connectorType, connector)
}
