package cache

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/jscharber/eAIIngest/pkg/storage"
)

// RedisCacheConfig contains configuration for Redis caching
type RedisCacheConfig struct {
	// Connection settings
	Addr         string `yaml:"addr"`
	Password     string `yaml:"password"`
	DB           int    `yaml:"db"`
	PoolSize     int    `yaml:"pool_size"`
	MinIdleConns int    `yaml:"min_idle_conns"`
	
	// Cache behavior
	DefaultTTL       time.Duration `yaml:"default_ttl"`
	FileInfoTTL      time.Duration `yaml:"file_info_ttl"`
	ContentTTL       time.Duration `yaml:"content_ttl"`
	MaxContentSize   int64         `yaml:"max_content_size"`
	CompressionLevel int           `yaml:"compression_level"`
	
	// Cache keys
	KeyPrefix        string `yaml:"key_prefix"`
	FileInfoPrefix   string `yaml:"file_info_prefix"`
	ContentPrefix    string `yaml:"content_prefix"`
	MetadataPrefix   string `yaml:"metadata_prefix"`
	
	// Performance settings
	EnableCompression bool `yaml:"enable_compression"`
	EnableMetrics     bool `yaml:"enable_metrics"`
	BatchSize         int  `yaml:"batch_size"`
}

// DefaultRedisCacheConfig returns default Redis cache configuration
func DefaultRedisCacheConfig() *RedisCacheConfig {
	return &RedisCacheConfig{
		Addr:             "localhost:6379",
		Password:         "",
		DB:               0,
		PoolSize:         10,
		MinIdleConns:     5,
		DefaultTTL:       1 * time.Hour,
		FileInfoTTL:      30 * time.Minute,
		ContentTTL:       2 * time.Hour,
		MaxContentSize:   50 * 1024 * 1024, // 50MB
		CompressionLevel: 6,
		KeyPrefix:        "audimodal:storage:",
		FileInfoPrefix:   "fileinfo:",
		ContentPrefix:    "content:",
		MetadataPrefix:   "metadata:",
		EnableCompression: true,
		EnableMetrics:    true,
		BatchSize:        100,
	}
}

// RedisCache implements a Redis-based caching layer for storage operations
type RedisCache struct {
	client   redis.Cmdable
	config   *RedisCacheConfig
	tracer   trace.Tracer
	compressor Compressor
	metrics  *CacheMetrics
}

// NewRedisCache creates a new Redis cache instance
func NewRedisCache(config *RedisCacheConfig) (*RedisCache, error) {
	if config == nil {
		config = DefaultRedisCacheConfig()
	}

	// Create Redis client
	rdb := redis.NewClient(&redis.Options{
		Addr:         config.Addr,
		Password:     config.Password,
		DB:           config.DB,
		PoolSize:     config.PoolSize,
		MinIdleConns: config.MinIdleConns,
	})

	// Test connection
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
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

	return &RedisCache{
		client:     rdb,
		config:     config,
		tracer:     otel.Tracer("redis-cache"),
		compressor: compressor,
		metrics:    metrics,
	}, nil
}

// GetFileInfo retrieves cached file information
func (c *RedisCache) GetFileInfo(ctx context.Context, url string) (*storage.FileInfo, error) {
	ctx, span := c.tracer.Start(ctx, "redis_cache_get_file_info")
	defer span.End()

	span.SetAttributes(
		attribute.String("cache.operation", "get"),
		attribute.String("cache.type", "file_info"),
		attribute.String("storage.url", url),
	)

	key := c.buildFileInfoKey(url)
	
	start := time.Now()
	data, err := c.client.Get(ctx, key).Result()
	duration := time.Since(start)

	if c.metrics != nil {
		c.metrics.RecordOperation("file_info", "get", duration, err == nil)
	}

	if err == redis.Nil {
		span.SetAttributes(attribute.Bool("cache.hit", false))
		return nil, ErrCacheMiss
	}
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get file info from cache: %w", err)
	}

	// Decompress if needed
	decompressed, err := c.compressor.Decompress([]byte(data))
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to decompress cached file info: %w", err)
	}

	var fileInfo storage.FileInfo
	if err := json.Unmarshal(decompressed, &fileInfo); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to unmarshal cached file info: %w", err)
	}

	span.SetAttributes(
		attribute.Bool("cache.hit", true),
		attribute.Int64("cache.data_size", int64(len(data))),
	)

	return &fileInfo, nil
}

// SetFileInfo caches file information
func (c *RedisCache) SetFileInfo(ctx context.Context, url string, fileInfo *storage.FileInfo) error {
	ctx, span := c.tracer.Start(ctx, "redis_cache_set_file_info")
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

	// Compress if enabled
	compressed, err := c.compressor.Compress(data)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to compress file info: %w", err)
	}

	key := c.buildFileInfoKey(url)
	
	start := time.Now()
	err = c.client.Set(ctx, key, compressed, c.config.FileInfoTTL).Err()
	duration := time.Since(start)

	if c.metrics != nil {
		c.metrics.RecordOperation("file_info", "set", duration, err == nil)
	}

	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to cache file info: %w", err)
	}

	span.SetAttributes(
		attribute.Int("cache.original_size", len(data)),
		attribute.Int("cache.compressed_size", len(compressed)),
		attribute.Float64("cache.compression_ratio", float64(len(data))/float64(len(compressed))),
	)

	return nil
}

// GetContent retrieves cached file content
func (c *RedisCache) GetContent(ctx context.Context, url string, checksum string) ([]byte, error) {
	ctx, span := c.tracer.Start(ctx, "redis_cache_get_content")
	defer span.End()

	span.SetAttributes(
		attribute.String("cache.operation", "get"),
		attribute.String("cache.type", "content"),
		attribute.String("storage.url", url),
		attribute.String("content.checksum", checksum),
	)

	key := c.buildContentKey(url, checksum)
	
	start := time.Now()
	data, err := c.client.Get(ctx, key).Result()
	duration := time.Since(start)

	if c.metrics != nil {
		c.metrics.RecordOperation("content", "get", duration, err == nil)
	}

	if err == redis.Nil {
		span.SetAttributes(attribute.Bool("cache.hit", false))
		return nil, ErrCacheMiss
	}
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get content from cache: %w", err)
	}

	// Decompress content
	decompressed, err := c.compressor.Decompress([]byte(data))
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to decompress cached content: %w", err)
	}

	span.SetAttributes(
		attribute.Bool("cache.hit", true),
		attribute.Int64("cache.compressed_size", int64(len(data))),
		attribute.Int64("cache.decompressed_size", int64(len(decompressed))),
	)

	return decompressed, nil
}

// SetContent caches file content with size limits
func (c *RedisCache) SetContent(ctx context.Context, url string, checksum string, content []byte) error {
	ctx, span := c.tracer.Start(ctx, "redis_cache_set_content")
	defer span.End()

	span.SetAttributes(
		attribute.String("cache.operation", "set"),
		attribute.String("cache.type", "content"),
		attribute.String("storage.url", url),
		attribute.String("content.checksum", checksum),
		attribute.Int64("content.size", int64(len(content))),
	)

	// Check size limits
	if int64(len(content)) > c.config.MaxContentSize {
		span.SetAttributes(attribute.Bool("cache.size_exceeded", true))
		return fmt.Errorf("content size %d exceeds maximum cacheable size %d", len(content), c.config.MaxContentSize)
	}

	// Compress content
	compressed, err := c.compressor.Compress(content)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to compress content: %w", err)
	}

	key := c.buildContentKey(url, checksum)
	
	start := time.Now()
	err = c.client.Set(ctx, key, compressed, c.config.ContentTTL).Err()
	duration := time.Since(start)

	if c.metrics != nil {
		c.metrics.RecordOperation("content", "set", duration, err == nil)
		c.metrics.RecordCacheSize("content", int64(len(compressed)))
	}

	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to cache content: %w", err)
	}

	span.SetAttributes(
		attribute.Int("cache.original_size", len(content)),
		attribute.Int("cache.compressed_size", len(compressed)),
		attribute.Float64("cache.compression_ratio", float64(len(content))/float64(len(compressed))),
	)

	return nil
}

// GetMetadata retrieves cached metadata
func (c *RedisCache) GetMetadata(ctx context.Context, key string) (map[string]interface{}, error) {
	ctx, span := c.tracer.Start(ctx, "redis_cache_get_metadata")
	defer span.End()

	cacheKey := c.buildMetadataKey(key)
	
	start := time.Now()
	data, err := c.client.Get(ctx, cacheKey).Result()
	duration := time.Since(start)

	if c.metrics != nil {
		c.metrics.RecordOperation("metadata", "get", duration, err == nil)
	}

	if err == redis.Nil {
		return nil, ErrCacheMiss
	}
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get metadata from cache: %w", err)
	}

	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(data), &metadata); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to unmarshal cached metadata: %w", err)
	}

	return metadata, nil
}

// SetMetadata caches metadata with custom TTL
func (c *RedisCache) SetMetadata(ctx context.Context, key string, metadata map[string]interface{}, ttl time.Duration) error {
	ctx, span := c.tracer.Start(ctx, "redis_cache_set_metadata")
	defer span.End()

	data, err := json.Marshal(metadata)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	cacheKey := c.buildMetadataKey(key)
	if ttl == 0 {
		ttl = c.config.DefaultTTL
	}
	
	start := time.Now()
	err = c.client.Set(ctx, cacheKey, data, ttl).Err()
	duration := time.Since(start)

	if c.metrics != nil {
		c.metrics.RecordOperation("metadata", "set", duration, err == nil)
	}

	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to cache metadata: %w", err)
	}

	return nil
}

// Delete removes items from cache
func (c *RedisCache) Delete(ctx context.Context, keys ...string) error {
	ctx, span := c.tracer.Start(ctx, "redis_cache_delete")
	defer span.End()

	if len(keys) == 0 {
		return nil
	}

	start := time.Now()
	err := c.client.Del(ctx, keys...).Err()
	duration := time.Since(start)

	if c.metrics != nil {
		c.metrics.RecordOperation("general", "delete", duration, err == nil)
	}

	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to delete cache keys: %w", err)
	}

	span.SetAttributes(attribute.Int("cache.keys_deleted", len(keys)))
	return nil
}

// Clear removes all cache entries with the configured prefix
func (c *RedisCache) Clear(ctx context.Context) error {
	ctx, span := c.tracer.Start(ctx, "redis_cache_clear")
	defer span.End()

	pattern := c.config.KeyPrefix + "*"
	
	start := time.Now()
	keys, err := c.client.Keys(ctx, pattern).Result()
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to list cache keys: %w", err)
	}

	if len(keys) > 0 {
		err = c.client.Del(ctx, keys...).Err()
		if err != nil {
			span.RecordError(err)
			return fmt.Errorf("failed to delete cache keys: %w", err)
		}
	}
	
	duration := time.Since(start)
	if c.metrics != nil {
		c.metrics.RecordOperation("general", "clear", duration, err == nil)
	}

	span.SetAttributes(attribute.Int("cache.keys_cleared", len(keys)))
	return nil
}

// GetStats returns cache statistics
func (c *RedisCache) GetStats(ctx context.Context) (*CacheStats, error) {
	ctx, span := c.tracer.Start(ctx, "redis_cache_get_stats")
	defer span.End()

	// Get Redis info
	info, err := c.client.Info(ctx, "memory", "stats").Result()
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get Redis info: %w", err)
	}

	stats := &CacheStats{
		Provider: "redis",
	}

	// Parse Redis info
	lines := strings.Split(info, "\r\n")
	for _, line := range lines {
		if strings.Contains(line, ":") {
			parts := strings.Split(line, ":")
			if len(parts) != 2 {
				continue
			}
			key, value := parts[0], parts[1]

			switch key {
			case "used_memory":
				if val, err := strconv.ParseInt(value, 10, 64); err == nil {
					stats.MemoryUsed = val
				}
			case "maxmemory":
				if val, err := strconv.ParseInt(value, 10, 64); err == nil {
					stats.MemoryLimit = val
				}
			case "keyspace_hits":
				if val, err := strconv.ParseInt(value, 10, 64); err == nil {
					stats.Hits = val
				}
			case "keyspace_misses":
				if val, err := strconv.ParseInt(value, 10, 64); err == nil {
					stats.Misses = val
				}
			}
		}
	}

	// Calculate hit ratio
	total := stats.Hits + stats.Misses
	if total > 0 {
		stats.HitRatio = float64(stats.Hits) / float64(total)
	}

	// Get key count for our prefix
	pattern := c.config.KeyPrefix + "*"
	keys, err := c.client.Keys(ctx, pattern).Result()
	if err == nil {
		stats.Keys = int64(len(keys))
	}

	// Add application-level metrics if available
	if c.metrics != nil {
		appStats := c.metrics.GetStats()
		stats.Operations = appStats.Operations
		stats.Errors = appStats.Errors
		stats.AverageLatency = appStats.AverageLatency
	}

	return stats, nil
}

// HealthCheck performs a health check on the Redis cache
func (c *RedisCache) HealthCheck(ctx context.Context) error {
	ctx, span := c.tracer.Start(ctx, "redis_cache_health_check")
	defer span.End()

	start := time.Now()
	err := c.client.Ping(ctx).Err()
	duration := time.Since(start)

	span.SetAttributes(
		attribute.Bool("cache.healthy", err == nil),
		attribute.Float64("cache.ping_time_ms", float64(duration.Nanoseconds())/1e6),
	)

	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("Redis health check failed: %w", err)
	}

	return nil
}

// Close closes the Redis connection
func (c *RedisCache) Close() error {
	if client, ok := c.client.(*redis.Client); ok {
		return client.Close()
	}
	return nil
}

// buildFileInfoKey creates a cache key for file info
func (c *RedisCache) buildFileInfoKey(url string) string {
	hash := fmt.Sprintf("%x", md5.Sum([]byte(url)))
	return c.config.KeyPrefix + c.config.FileInfoPrefix + hash
}

// buildContentKey creates a cache key for file content
func (c *RedisCache) buildContentKey(url, checksum string) string {
	key := url + ":" + checksum
	hash := fmt.Sprintf("%x", md5.Sum([]byte(key)))
	return c.config.KeyPrefix + c.config.ContentPrefix + hash
}

// buildMetadataKey creates a cache key for metadata
func (c *RedisCache) buildMetadataKey(key string) string {
	hash := fmt.Sprintf("%x", md5.Sum([]byte(key)))
	return c.config.KeyPrefix + c.config.MetadataPrefix + hash
}

// BatchGet retrieves multiple items from cache
func (c *RedisCache) BatchGet(ctx context.Context, keys []string) (map[string][]byte, error) {
	ctx, span := c.tracer.Start(ctx, "redis_cache_batch_get")
	defer span.End()

	if len(keys) == 0 {
		return make(map[string][]byte), nil
	}

	start := time.Now()
	values, err := c.client.MGet(ctx, keys...).Result()
	duration := time.Since(start)

	if c.metrics != nil {
		c.metrics.RecordOperation("general", "batch_get", duration, err == nil)
	}

	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to batch get from cache: %w", err)
	}

	result := make(map[string][]byte)
	hits := 0

	for i, value := range values {
		if value != nil {
			if str, ok := value.(string); ok {
				result[keys[i]] = []byte(str)
				hits++
			}
		}
	}

	span.SetAttributes(
		attribute.Int("cache.keys_requested", len(keys)),
		attribute.Int("cache.keys_found", hits),
		attribute.Float64("cache.hit_ratio", float64(hits)/float64(len(keys))),
	)

	return result, nil
}

// BatchSet stores multiple items in cache
func (c *RedisCache) BatchSet(ctx context.Context, items map[string][]byte, ttl time.Duration) error {
	ctx, span := c.tracer.Start(ctx, "redis_cache_batch_set")
	defer span.End()

	if len(items) == 0 {
		return nil
	}

	pipe := c.client.Pipeline()
	
	for key, value := range items {
		pipe.Set(ctx, key, value, ttl)
	}

	start := time.Now()
	_, err := pipe.Exec(ctx)
	duration := time.Since(start)

	if c.metrics != nil {
		c.metrics.RecordOperation("general", "batch_set", duration, err == nil)
	}

	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to batch set cache items: %w", err)
	}

	span.SetAttributes(attribute.Int("cache.keys_set", len(items)))
	return nil
}

// Expire sets expiration time for cache keys
func (c *RedisCache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return c.client.Expire(ctx, key, ttl).Err()
}

// Exists checks if keys exist in cache
func (c *RedisCache) Exists(ctx context.Context, keys ...string) (int64, error) {
	return c.client.Exists(ctx, keys...).Result()
}