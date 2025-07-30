package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/jscharber/eAIIngest/pkg/storage"
)

// MemoryCacheConfig contains configuration for in-memory caching
type MemoryCacheConfig struct {
	// Size limits
	MaxSize      int64 `yaml:"max_size"`       // Maximum total memory size
	MaxItems     int   `yaml:"max_items"`      // Maximum number of items
	MaxItemSize  int64 `yaml:"max_item_size"`  // Maximum size per item
	
	// TTL settings
	DefaultTTL   time.Duration `yaml:"default_ttl"`
	CleanupInterval time.Duration `yaml:"cleanup_interval"`
	
	// Eviction policy
	EvictionPolicy string `yaml:"eviction_policy"` // lru, lfu, ttl, random
	
	// Performance settings
	EnableCompression bool `yaml:"enable_compression"`
	CompressionLevel  int  `yaml:"compression_level"`
	EnableMetrics     bool `yaml:"enable_metrics"`
	
	// Concurrency
	Shards int `yaml:"shards"` // Number of shards for concurrent access
}

// DefaultMemoryCacheConfig returns default memory cache configuration
func DefaultMemoryCacheConfig() *MemoryCacheConfig {
	return &MemoryCacheConfig{
		MaxSize:           100 * 1024 * 1024, // 100MB
		MaxItems:          10000,
		MaxItemSize:       10 * 1024 * 1024,  // 10MB
		DefaultTTL:        1 * time.Hour,
		CleanupInterval:   5 * time.Minute,
		EvictionPolicy:    "lru",
		EnableCompression: true,
		CompressionLevel:  6,
		EnableMetrics:     true,
		Shards:           16,
	}
}

// MemoryCache implements an in-memory cache with LRU eviction
type MemoryCache struct {
	config     *MemoryCacheConfig
	shards     []*CacheShard
	tracer     trace.Tracer
	compressor Compressor
	metrics    *CacheMetrics
	stopCh     chan struct{}
	wg         sync.WaitGroup
}

// CacheShard represents a shard of the cache for concurrent access
type CacheShard struct {
	items       map[string]*CacheEntry
	lruList     *LRUList
	mu          sync.RWMutex
	currentSize int64
	maxSize     int64
}

// LRUList implements a doubly-linked list for LRU tracking
type LRUList struct {
	head *LRUNode
	tail *LRUNode
	size int
}

// LRUNode represents a node in the LRU list
type LRUNode struct {
	key   string
	prev  *LRUNode
	next  *LRUNode
}

// NewMemoryCache creates a new in-memory cache
func NewMemoryCache(config *MemoryCacheConfig) (*MemoryCache, error) {
	if config == nil {
		config = DefaultMemoryCacheConfig()
	}

	// Initialize compressor
	var compressor Compressor
	if config.EnableCompression {
		compressor = NewGzipCompressor(config.CompressionLevel)
	} else {
		compressor = NewNoOpCompressor()
	}

	// Initialize metrics
	var metrics *CacheMetrics
	if config.EnableMetrics {
		metrics = NewCacheMetrics()
	}

	// Create shards
	shards := make([]*CacheShard, config.Shards)
	shardMaxSize := config.MaxSize / int64(config.Shards)
	
	for i := 0; i < config.Shards; i++ {
		shards[i] = &CacheShard{
			items:   make(map[string]*CacheEntry),
			lruList: NewLRUList(),
			maxSize: shardMaxSize,
		}
	}

	cache := &MemoryCache{
		config:     config,
		shards:     shards,
		tracer:     otel.Tracer("memory-cache"),
		compressor: compressor,
		metrics:    metrics,
		stopCh:     make(chan struct{}),
	}

	// Start cleanup routine
	cache.wg.Add(1)
	go cache.cleanupLoop()

	return cache, nil
}

// NewLRUList creates a new LRU list
func NewLRUList() *LRUList {
	head := &LRUNode{}
	tail := &LRUNode{}
	head.next = tail
	tail.prev = head
	
	return &LRUList{
		head: head,
		tail: tail,
	}
}

// GetFileInfo retrieves cached file information
func (c *MemoryCache) GetFileInfo(ctx context.Context, url string) (*storage.FileInfo, error) {
	ctx, span := c.tracer.Start(ctx, "memory_cache_get_file_info")
	defer span.End()

	span.SetAttributes(
		attribute.String("cache.operation", "get"),
		attribute.String("cache.type", "file_info"),
		attribute.String("storage.url", url),
	)

	key := "fileinfo:" + url
	shard := c.getShard(key)

	start := time.Now()
	entry, hit := shard.get(key)
	duration := time.Since(start)

	if c.metrics != nil {
		c.metrics.RecordOperation("file_info", "get", duration, hit)
	}

	if !hit {
		span.SetAttributes(attribute.Bool("cache.hit", false))
		return nil, ErrCacheMiss
	}

	// Check expiration
	if entry.IsExpired() {
		shard.delete(key)
		span.SetAttributes(attribute.Bool("cache.expired", true))
		return nil, ErrCacheExpired
	}

	// Decompress and deserialize
	decompressed, err := c.compressor.Decompress(entry.Value)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to decompress file info: %w", err)
	}

	var fileInfo storage.FileInfo
	if err := json.Unmarshal(decompressed, &fileInfo); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to unmarshal file info: %w", err)
	}

	// Update access statistics
	entry.UpdateAccess()
	shard.moveToFront(key)

	span.SetAttributes(
		attribute.Bool("cache.hit", true),
		attribute.Int64("cache.data_size", entry.Size),
	)

	return &fileInfo, nil
}

// SetFileInfo caches file information
func (c *MemoryCache) SetFileInfo(ctx context.Context, url string, fileInfo *storage.FileInfo) error {
	ctx, span := c.tracer.Start(ctx, "memory_cache_set_file_info")
	defer span.End()

	span.SetAttributes(
		attribute.String("cache.operation", "set"),
		attribute.String("cache.type", "file_info"),
		attribute.String("storage.url", url),
	)

	// Serialize file info
	data, err := json.Marshal(fileInfo)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to marshal file info: %w", err)
	}

	// Compress data
	compressed, err := c.compressor.Compress(data)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to compress file info: %w", err)
	}

	// Check size limits
	if int64(len(compressed)) > c.config.MaxItemSize {
		return fmt.Errorf("file info size %d exceeds maximum item size %d", len(compressed), c.config.MaxItemSize)
	}

	key := "fileinfo:" + url
	shard := c.getShard(key)

	entry := &CacheEntry{
		Key:         key,
		Value:       compressed,
		Compressed:  c.config.EnableCompression,
		ContentType: "application/json",
		Size:        int64(len(compressed)),
		CreatedAt:   time.Now(),
		LastAccess:  time.Now(),
		AccessCount: 1,
	}

	// Set expiration
	if c.config.DefaultTTL > 0 {
		expiresAt := time.Now().Add(c.config.DefaultTTL)
		entry.ExpiresAt = &expiresAt
	}

	start := time.Now()
	err = shard.set(key, entry)
	duration := time.Since(start)

	if c.metrics != nil {
		c.metrics.RecordOperation("file_info", "set", duration, err == nil)
	}

	if err != nil {
		span.RecordError(err)
		return err
	}

	span.SetAttributes(
		attribute.Int("cache.original_size", len(data)),
		attribute.Int("cache.compressed_size", len(compressed)),
		attribute.Float64("cache.compression_ratio", float64(len(data))/float64(len(compressed))),
	)

	return nil
}

// GetContent retrieves cached file content
func (c *MemoryCache) GetContent(ctx context.Context, url string, checksum string) ([]byte, error) {
	ctx, span := c.tracer.Start(ctx, "memory_cache_get_content")
	defer span.End()

	key := "content:" + url + ":" + checksum
	shard := c.getShard(key)

	start := time.Now()
	entry, hit := shard.get(key)
	duration := time.Since(start)

	if c.metrics != nil {
		c.metrics.RecordOperation("content", "get", duration, hit)
	}

	if !hit {
		span.SetAttributes(attribute.Bool("cache.hit", false))
		return nil, ErrCacheMiss
	}

	// Check expiration
	if entry.IsExpired() {
		shard.delete(key)
		return nil, ErrCacheExpired
	}

	// Decompress content
	decompressed, err := c.compressor.Decompress(entry.Value)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to decompress content: %w", err)
	}

	// Update access statistics
	entry.UpdateAccess()
	shard.moveToFront(key)

	span.SetAttributes(
		attribute.Bool("cache.hit", true),
		attribute.Int64("cache.compressed_size", entry.Size),
		attribute.Int64("cache.decompressed_size", int64(len(decompressed))),
	)

	return decompressed, nil
}

// SetContent caches file content
func (c *MemoryCache) SetContent(ctx context.Context, url string, checksum string, content []byte) error {
	ctx, span := c.tracer.Start(ctx, "memory_cache_set_content")
	defer span.End()

	// Check size limits
	if int64(len(content)) > c.config.MaxItemSize {
		return fmt.Errorf("content size %d exceeds maximum item size %d", len(content), c.config.MaxItemSize)
	}

	// Compress content
	compressed, err := c.compressor.Compress(content)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to compress content: %w", err)
	}

	key := "content:" + url + ":" + checksum
	shard := c.getShard(key)

	entry := &CacheEntry{
		Key:         key,
		Value:       compressed,
		Compressed:  c.config.EnableCompression,
		ContentType: "application/octet-stream",
		Size:        int64(len(compressed)),
		CreatedAt:   time.Now(),
		LastAccess:  time.Now(),
		AccessCount: 1,
	}

	// Set expiration
	if c.config.DefaultTTL > 0 {
		expiresAt := time.Now().Add(c.config.DefaultTTL)
		entry.ExpiresAt = &expiresAt
	}

	start := time.Now()
	err = shard.set(key, entry)
	duration := time.Since(start)

	if c.metrics != nil {
		c.metrics.RecordOperation("content", "set", duration, err == nil)
	}

	if err != nil {
		span.RecordError(err)
		return err
	}

	span.SetAttributes(
		attribute.Int("cache.original_size", len(content)),
		attribute.Int("cache.compressed_size", len(compressed)),
		attribute.Float64("cache.compression_ratio", float64(len(content))/float64(len(compressed))),
	)

	return nil
}

// GetMetadata retrieves cached metadata
func (c *MemoryCache) GetMetadata(ctx context.Context, key string) (map[string]interface{}, error) {
	cacheKey := "metadata:" + key
	shard := c.getShard(cacheKey)

	entry, hit := shard.get(cacheKey)
	if !hit {
		return nil, ErrCacheMiss
	}

	if entry.IsExpired() {
		shard.delete(cacheKey)
		return nil, ErrCacheExpired
	}

	var metadata map[string]interface{}
	if err := json.Unmarshal(entry.Value, &metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	entry.UpdateAccess()
	shard.moveToFront(cacheKey)

	return metadata, nil
}

// SetMetadata caches metadata
func (c *MemoryCache) SetMetadata(ctx context.Context, key string, metadata map[string]interface{}, ttl time.Duration) error {
	data, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	cacheKey := "metadata:" + key
	shard := c.getShard(cacheKey)

	entry := &CacheEntry{
		Key:         cacheKey,
		Value:       data,
		ContentType: "application/json",
		Size:        int64(len(data)),
		CreatedAt:   time.Now(),
		LastAccess:  time.Now(),
		AccessCount: 1,
	}

	if ttl > 0 {
		expiresAt := time.Now().Add(ttl)
		entry.ExpiresAt = &expiresAt
	} else if c.config.DefaultTTL > 0 {
		expiresAt := time.Now().Add(c.config.DefaultTTL)
		entry.ExpiresAt = &expiresAt
	}

	return shard.set(cacheKey, entry)
}

// Delete removes items from cache
func (c *MemoryCache) Delete(ctx context.Context, keys ...string) error {
	for _, key := range keys {
		shard := c.getShard(key)
		shard.delete(key)
	}
	return nil
}

// Clear removes all cache entries
func (c *MemoryCache) Clear(ctx context.Context) error {
	for _, shard := range c.shards {
		shard.clear()
	}
	return nil
}

// Exists checks if keys exist in cache
func (c *MemoryCache) Exists(ctx context.Context, keys ...string) (int64, error) {
	var count int64
	for _, key := range keys {
		shard := c.getShard(key)
		if shard.exists(key) {
			count++
		}
	}
	return count, nil
}

// Expire sets expiration time for cache keys
func (c *MemoryCache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	shard := c.getShard(key)
	return shard.expire(key, ttl)
}

// BatchGet retrieves multiple items from cache
func (c *MemoryCache) BatchGet(ctx context.Context, keys []string) (map[string][]byte, error) {
	result := make(map[string][]byte)
	
	for _, key := range keys {
		shard := c.getShard(key)
		if entry, hit := shard.get(key); hit && !entry.IsExpired() {
			result[key] = entry.Value
			entry.UpdateAccess()
			shard.moveToFront(key)
		}
	}
	
	return result, nil
}

// BatchSet stores multiple items in cache
func (c *MemoryCache) BatchSet(ctx context.Context, items map[string][]byte, ttl time.Duration) error {
	for key, value := range items {
		shard := c.getShard(key)
		
		entry := &CacheEntry{
			Key:         key,
			Value:       value,
			Size:        int64(len(value)),
			CreatedAt:   time.Now(),
			LastAccess:  time.Now(),
			AccessCount: 1,
		}

		if ttl > 0 {
			expiresAt := time.Now().Add(ttl)
			entry.ExpiresAt = &expiresAt
		}

		if err := shard.set(key, entry); err != nil {
			return err
		}
	}
	
	return nil
}

// GetStats returns cache statistics
func (c *MemoryCache) GetStats(ctx context.Context) (*CacheStats, error) {
	var totalSize, totalKeys int64
	
	for _, shard := range c.shards {
		shard.mu.RLock()
		totalKeys += int64(len(shard.items))
		totalSize += shard.currentSize
		shard.mu.RUnlock()
	}

	stats := &CacheStats{
		Provider:    "memory",
		Keys:        totalKeys,
		MemoryUsed:  totalSize,
		MemoryLimit: c.config.MaxSize,
	}

	if c.metrics != nil {
		appStats := c.metrics.GetStats()
		stats.Operations = appStats.Operations
		stats.Errors = appStats.Errors
		stats.AverageLatency = appStats.AverageLatency
		stats.HitRatio = appStats.HitRatio
	}

	return stats, nil
}

// HealthCheck performs a health check
func (c *MemoryCache) HealthCheck(ctx context.Context) error {
	// Memory cache is always healthy if it's running
	return nil
}

// Close closes the memory cache
func (c *MemoryCache) Close() error {
	close(c.stopCh)
	c.wg.Wait()
	return nil
}

// getShard returns the appropriate shard for a key
func (c *MemoryCache) getShard(key string) *CacheShard {
	hash := c.hashKey(key)
	return c.shards[hash%uint32(len(c.shards))]
}

// hashKey computes a hash for the key
func (c *MemoryCache) hashKey(key string) uint32 {
	var hash uint32 = 2166136261
	for i := 0; i < len(key); i++ {
		hash ^= uint32(key[i])
		hash *= 16777619
	}
	return hash
}

// cleanupLoop periodically removes expired entries
func (c *MemoryCache) cleanupLoop() {
	defer c.wg.Done()
	
	ticker := time.NewTicker(c.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.cleanup()
		}
	}
}

// cleanup removes expired entries from all shards
func (c *MemoryCache) cleanup() {
	for _, shard := range c.shards {
		shard.cleanup()
	}
}

// Shard methods

// get retrieves an entry from the shard
func (s *CacheShard) get(key string) (*CacheEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	entry, exists := s.items[key]
	return entry, exists
}

// set stores an entry in the shard
func (s *CacheShard) set(key string, entry *CacheEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if we need to evict
	if existing, exists := s.items[key]; exists {
		s.currentSize -= existing.Size
		s.lruList.remove(key)
	}

	// Check size limits
	if s.currentSize+entry.Size > s.maxSize {
		s.evictLRU()
	}

	s.items[key] = entry
	s.currentSize += entry.Size
	s.lruList.addToFront(key)

	return nil
}

// delete removes an entry from the shard
func (s *CacheShard) delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entry, exists := s.items[key]; exists {
		delete(s.items, key)
		s.currentSize -= entry.Size
		s.lruList.remove(key)
	}
}

// exists checks if a key exists in the shard
func (s *CacheShard) exists(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	_, exists := s.items[key]
	return exists
}

// expire sets expiration for a key
func (s *CacheShard) expire(key string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entry, exists := s.items[key]; exists {
		expiresAt := time.Now().Add(ttl)
		entry.ExpiresAt = &expiresAt
		return nil
	}

	return fmt.Errorf("key not found: %s", key)
}

// clear removes all entries from the shard
func (s *CacheShard) clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.items = make(map[string]*CacheEntry)
	s.lruList = NewLRUList()
	s.currentSize = 0
}

// moveToFront moves a key to the front of the LRU list
func (s *CacheShard) moveToFront(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.lruList.moveToFront(key)
}

// evictLRU evicts the least recently used entry
func (s *CacheShard) evictLRU() {
	if key := s.lruList.removeTail(); key != "" {
		if entry, exists := s.items[key]; exists {
			delete(s.items, key)
			s.currentSize -= entry.Size
		}
	}
}

// cleanup removes expired entries
func (s *CacheShard) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for key, entry := range s.items {
		if entry.IsExpired() && entry.ExpiresAt.Before(now) {
			delete(s.items, key)
			s.currentSize -= entry.Size
			s.lruList.remove(key)
		}
	}
}

// LRU List methods

// addToFront adds a key to the front of the list
func (l *LRUList) addToFront(key string) {
	node := &LRUNode{key: key}
	node.next = l.head.next
	node.prev = l.head
	l.head.next.prev = node
	l.head.next = node
	l.size++
}

// remove removes a key from the list
func (l *LRUList) remove(key string) {
	// Find and remove the node
	for node := l.head.next; node != l.tail; node = node.next {
		if node.key == key {
			node.prev.next = node.next
			node.next.prev = node.prev
			l.size--
			break
		}
	}
}

// removeTail removes and returns the key at the tail
func (l *LRUList) removeTail() string {
	if l.size == 0 {
		return ""
	}
	
	lastNode := l.tail.prev
	key := lastNode.key
	lastNode.prev.next = l.tail
	l.tail.prev = lastNode.prev
	l.size--
	
	return key
}

// moveToFront moves a key to the front of the list
func (l *LRUList) moveToFront(key string) {
	l.remove(key)
	l.addToFront(key)
}