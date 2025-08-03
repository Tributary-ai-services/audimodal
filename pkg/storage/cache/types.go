package cache

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jscharber/eAIIngest/pkg/storage"
)

// Common cache errors
var (
	ErrCacheMiss    = errors.New("cache miss")
	ErrCacheExpired = errors.New("cache expired")
	ErrInvalidKey   = errors.New("invalid cache key")
	ErrSizeExceeded = errors.New("content size exceeds cache limit")
	ErrNotSupported = errors.New("operation not supported")
)

// Cache defines the interface for storage caching
type Cache interface {
	// File info operations
	GetFileInfo(ctx context.Context, url string) (*storage.FileInfo, error)
	SetFileInfo(ctx context.Context, url string, fileInfo *storage.FileInfo) error

	// Content operations
	GetContent(ctx context.Context, url string, checksum string) ([]byte, error)
	SetContent(ctx context.Context, url string, checksum string, content []byte) error

	// Metadata operations
	GetMetadata(ctx context.Context, key string) (map[string]interface{}, error)
	SetMetadata(ctx context.Context, key string, metadata map[string]interface{}, ttl time.Duration) error

	// General operations
	Delete(ctx context.Context, keys ...string) error
	Clear(ctx context.Context) error
	Exists(ctx context.Context, keys ...string) (int64, error)
	Expire(ctx context.Context, key string, ttl time.Duration) error

	// Batch operations
	BatchGet(ctx context.Context, keys []string) (map[string][]byte, error)
	BatchSet(ctx context.Context, items map[string][]byte, ttl time.Duration) error

	// Statistics and health
	GetStats(ctx context.Context) (*CacheStats, error)
	HealthCheck(ctx context.Context) error

	// Lifecycle
	Close() error
}

// CacheStats contains cache performance statistics
type CacheStats struct {
	// Provider information
	Provider string `json:"provider"`

	// Hit/miss statistics
	Hits     int64   `json:"hits"`
	Misses   int64   `json:"misses"`
	HitRatio float64 `json:"hit_ratio"`

	// Memory usage
	MemoryUsed  int64 `json:"memory_used"`
	MemoryLimit int64 `json:"memory_limit"`

	// Key statistics
	Keys    int64 `json:"keys"`
	Expired int64 `json:"expired"`
	Evicted int64 `json:"evicted"`

	// Performance metrics
	Operations     int64         `json:"operations"`
	Errors         int64         `json:"errors"`
	AverageLatency time.Duration `json:"average_latency"`

	// Size information
	TotalSize       int64 `json:"total_size"`
	AverageItemSize int64 `json:"average_item_size"`

	// Connection information
	Connections int           `json:"connections"`
	Uptime      time.Duration `json:"uptime"`
}

// CacheMetrics tracks application-level cache metrics
type CacheMetrics struct {
	operations map[string]*OperationMetrics
	mu         sync.RWMutex
}

// OperationMetrics tracks metrics for specific cache operations
type OperationMetrics struct {
	Count       int64         `json:"count"`
	Successes   int64         `json:"successes"`
	Failures    int64         `json:"failures"`
	TotalTime   time.Duration `json:"total_time"`
	MinTime     time.Duration `json:"min_time"`
	MaxTime     time.Duration `json:"max_time"`
	LastUpdated time.Time     `json:"last_updated"`
}

// NewCacheMetrics creates a new cache metrics tracker
func NewCacheMetrics() *CacheMetrics {
	return &CacheMetrics{
		operations: make(map[string]*OperationMetrics),
	}
}

// RecordOperation records a cache operation
func (m *CacheMetrics) RecordOperation(cacheType, operation string, duration time.Duration, success bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := cacheType + ":" + operation
	metrics, exists := m.operations[key]
	if !exists {
		metrics = &OperationMetrics{
			MinTime: duration,
			MaxTime: duration,
		}
		m.operations[key] = metrics
	}

	metrics.Count++
	if success {
		metrics.Successes++
	} else {
		metrics.Failures++
	}

	metrics.TotalTime += duration
	if duration < metrics.MinTime {
		metrics.MinTime = duration
	}
	if duration > metrics.MaxTime {
		metrics.MaxTime = duration
	}
	metrics.LastUpdated = time.Now()
}

// RecordCacheSize records the size of cached data
func (m *CacheMetrics) RecordCacheSize(cacheType string, size int64) {
	// This would be implemented to track size metrics
}

// GetStats returns aggregated statistics
func (m *CacheMetrics) GetStats() *CacheStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var totalOps, totalSuccesses, totalFailures int64
	var totalTime time.Duration

	for _, metrics := range m.operations {
		totalOps += metrics.Count
		totalSuccesses += metrics.Successes
		totalFailures += metrics.Failures
		totalTime += metrics.TotalTime
	}

	var avgLatency time.Duration
	if totalOps > 0 {
		avgLatency = totalTime / time.Duration(totalOps)
	}

	return &CacheStats{
		Operations:     totalOps,
		Errors:         totalFailures,
		AverageLatency: avgLatency,
		HitRatio:       float64(totalSuccesses) / float64(totalOps),
	}
}

// CacheConfig contains common configuration for cache implementations
type CacheConfig struct {
	// Provider type (redis, memory, etc.)
	Provider string `yaml:"provider"`

	// General settings
	Enabled         bool          `yaml:"enabled"`
	DefaultTTL      time.Duration `yaml:"default_ttl"`
	MaxSize         int64         `yaml:"max_size"`
	MaxItemSize     int64         `yaml:"max_item_size"`
	CleanupInterval time.Duration `yaml:"cleanup_interval"`

	// Performance settings
	EnableCompression bool `yaml:"enable_compression"`
	CompressionLevel  int  `yaml:"compression_level"`
	EnableMetrics     bool `yaml:"enable_metrics"`

	// Content-specific settings
	CacheFileInfo bool          `yaml:"cache_file_info"`
	CacheContent  bool          `yaml:"cache_content"`
	CacheMetadata bool          `yaml:"cache_metadata"`
	FileInfoTTL   time.Duration `yaml:"file_info_ttl"`
	ContentTTL    time.Duration `yaml:"content_ttl"`
	MetadataTTL   time.Duration `yaml:"metadata_ttl"`

	// Size limits
	MaxContentSize  int64 `yaml:"max_content_size"`
	MaxMetadataSize int64 `yaml:"max_metadata_size"`

	// Eviction policy
	EvictionPolicy   string `yaml:"eviction_policy"` // lru, lfu, ttl, random
	EvictionMaxItems int    `yaml:"eviction_max_items"`

	// Provider-specific configuration
	Redis  *RedisCacheConfig  `yaml:"redis,omitempty"`
	Memory *MemoryCacheConfig `yaml:"memory,omitempty"`
}

// DefaultCacheConfig returns default cache configuration
func DefaultCacheConfig() *CacheConfig {
	return &CacheConfig{
		Provider:          "memory",
		Enabled:           true,
		DefaultTTL:        1 * time.Hour,
		MaxSize:           1024 * 1024 * 1024, // 1GB
		MaxItemSize:       50 * 1024 * 1024,   // 50MB
		CleanupInterval:   10 * time.Minute,
		EnableCompression: true,
		CompressionLevel:  6,
		EnableMetrics:     true,
		CacheFileInfo:     true,
		CacheContent:      true,
		CacheMetadata:     true,
		FileInfoTTL:       30 * time.Minute,
		ContentTTL:        2 * time.Hour,
		MetadataTTL:       1 * time.Hour,
		MaxContentSize:    50 * 1024 * 1024, // 50MB
		MaxMetadataSize:   1 * 1024 * 1024,  // 1MB
		EvictionPolicy:    "lru",
		EvictionMaxItems:  10000,
		Redis:             DefaultRedisCacheConfig(),
		Memory:            DefaultMemoryCacheConfig(),
	}
}

// CacheEntry represents a cached item with metadata
type CacheEntry struct {
	Key         string                 `json:"key"`
	Value       []byte                 `json:"value"`
	Compressed  bool                   `json:"compressed"`
	ContentType string                 `json:"content_type"`
	Size        int64                  `json:"size"`
	CreatedAt   time.Time              `json:"created_at"`
	ExpiresAt   *time.Time             `json:"expires_at,omitempty"`
	AccessCount int64                  `json:"access_count"`
	LastAccess  time.Time              `json:"last_access"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// IsExpired checks if the cache entry has expired
func (e *CacheEntry) IsExpired() bool {
	if e.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*e.ExpiresAt)
}

// UpdateAccess updates the access statistics
func (e *CacheEntry) UpdateAccess() {
	e.AccessCount++
	e.LastAccess = time.Now()
}

// CacheOperationResult represents the result of a cache operation
type CacheOperationResult struct {
	Success  bool          `json:"success"`
	Hit      bool          `json:"hit"`
	Size     int64         `json:"size"`
	Duration time.Duration `json:"duration"`
	Error    error         `json:"-"`
	ErrorMsg string        `json:"error,omitempty"`
}

// CacheLayer represents different caching layers
type CacheLayer string

const (
	CacheLayerL1 CacheLayer = "l1" // In-memory cache
	CacheLayerL2 CacheLayer = "l2" // Redis cache
	CacheLayerL3 CacheLayer = "l3" // Persistent cache
)

// MultiLevelCache supports multiple cache layers
type MultiLevelCache interface {
	Cache

	// Layer-specific operations
	GetFromLayer(ctx context.Context, layer CacheLayer, key string) ([]byte, error)
	SetToLayer(ctx context.Context, layer CacheLayer, key string, value []byte, ttl time.Duration) error

	// Cache promotion/demotion
	PromoteToL1(ctx context.Context, key string) error
	DemoteFromL1(ctx context.Context, key string) error

	// Layer statistics
	GetLayerStats(ctx context.Context, layer CacheLayer) (*CacheStats, error)
}

// CacheEventType represents different cache events
type CacheEventType string

const (
	CacheEventHit    CacheEventType = "hit"
	CacheEventMiss   CacheEventType = "miss"
	CacheEventSet    CacheEventType = "set"
	CacheEventDelete CacheEventType = "delete"
	CacheEventExpire CacheEventType = "expire"
	CacheEventEvict  CacheEventType = "evict"
	CacheEventError  CacheEventType = "error"
)

// CacheEvent represents a cache event for monitoring/logging
type CacheEvent struct {
	Type      CacheEventType         `json:"type"`
	Key       string                 `json:"key"`
	Layer     CacheLayer             `json:"layer,omitempty"`
	Size      int64                  `json:"size,omitempty"`
	Duration  time.Duration          `json:"duration,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Error     string                 `json:"error,omitempty"`
}

// CacheObserver defines an interface for cache event observation
type CacheObserver interface {
	OnCacheEvent(event *CacheEvent)
}

// ObservableCache extends Cache with event observation capabilities
type ObservableCache interface {
	Cache

	// Observer management
	AddObserver(observer CacheObserver)
	RemoveObserver(observer CacheObserver)

	// Event emission
	EmitEvent(event *CacheEvent)
}

// CacheStrategy defines different caching strategies
type CacheStrategy string

const (
	CacheStrategyWriteThrough CacheStrategy = "write_through"
	CacheStrategyWriteBack    CacheStrategy = "write_back"
	CacheStrategyWriteAround  CacheStrategy = "write_around"
	CacheStrategyLazyLoading  CacheStrategy = "lazy_loading"
	CacheStrategyRefreshAhead CacheStrategy = "refresh_ahead"
)

// CacheOptions contains options for cache operations
type CacheOptions struct {
	TTL           time.Duration          `json:"ttl,omitempty"`
	Tags          []string               `json:"tags,omitempty"`
	Compress      bool                   `json:"compress,omitempty"`
	Layer         CacheLayer             `json:"layer,omitempty"`
	Strategy      CacheStrategy          `json:"strategy,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	RefreshAhead  bool                   `json:"refresh_ahead,omitempty"`
	RefreshMargin time.Duration          `json:"refresh_margin,omitempty"`
}

// DefaultCacheOptions returns default cache options
func DefaultCacheOptions() *CacheOptions {
	return &CacheOptions{
		TTL:           1 * time.Hour,
		Compress:      true,
		Layer:         CacheLayerL1,
		Strategy:      CacheStrategyLazyLoading,
		RefreshAhead:  false,
		RefreshMargin: 5 * time.Minute,
	}
}
