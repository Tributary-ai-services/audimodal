# Document Compression System

A comprehensive document compression system for the AudiModal platform that provides intelligent, policy-based compression strategies for different types of documents and storage scenarios.

## Features

### Core Compression Capabilities
- **Multiple Compression Strategies**: Adaptive, text-optimized, binary-optimized, and aggressive compression
- **Intelligent Algorithm Selection**: Automatically chooses the best compression algorithm based on file characteristics
- **File Type Detection**: Recognizes file types by extension, MIME type, and content analysis
- **Size and Performance Optimization**: Balances compression ratio with processing speed

### Policy-Based Management
- **Flexible Policy Engine**: Define compression rules based on file properties, location, age, and metadata
- **Tenant Isolation**: Per-tenant compression policies with priority-based rule evaluation
- **Conditional Logic**: Complex conditions using operators like equals, greater than, contains, etc.
- **Dynamic Configuration**: Runtime policy updates without system restarts

### Storage Integration
- **Transparent Compression**: Seamlessly integrates with existing storage resolvers
- **Cache Integration**: Automatically caches compressed and decompressed content
- **Metadata Management**: Stores compression metadata alongside compressed files
- **Original File Handling**: Configurable retention of original files

### Performance and Scalability
- **Asynchronous Processing**: Background compression for large files
- **Batch Operations**: Efficient processing of multiple files
- **Concurrent Processing**: Multi-threaded compression with configurable worker pools
- **Memory Management**: Efficient memory usage with streaming and buffering

## Architecture

### Components

1. **DocumentCompressor**: Core compression engine
2. **PolicyManager**: Policy evaluation and management
3. **CompressedStorageResolver**: Storage integration layer
4. **BatchCompressor**: Batch processing capabilities

### Compression Strategies

| Strategy | Use Case | Compression Level | Performance |
|----------|----------|-------------------|-------------|
| `none` | Already compressed files | N/A | Fastest |
| `adaptive` | Mixed content | Dynamic | Balanced |
| `text` | Text documents | High | Good |
| `binary` | Binary documents | Medium | Good |
| `aggressive` | Archive/backup | Maximum | Slower |

### Policy Conditions

| Type | Description | Operators | Example Values |
|------|-------------|-----------|----------------|
| `file_size` | File size in bytes | `gt`, `lt`, `ge`, `le`, `eq` | `1048576` (1MB) |
| `file_extension` | File extension | `eq`, `ne`, `in`, `not_in` | `.txt`, `.pdf` |
| `mime_type` | MIME type | `eq`, `contains`, `starts_with` | `text/plain` |
| `age` | File age | `gt`, `lt`, `ge`, `le` | `720h` (30 days) |
| `path` | File path/URL | `contains`, `starts_with`, `matches` | `/archive/` |
| `metadata` | Custom metadata | `eq`, `ne`, `contains` | `classification: "public"` |
| `tenant` | Tenant ID | `eq`, `ne` | Tenant UUID |

## Usage

### Basic Compression

```go
// Create compressor
config := compression.DefaultDocumentCompressionConfig()
compressor := compression.NewDocumentCompressor(config)

// Compress a document
data := []byte("Sample document content...")
result, err := compressor.CompressDocument(ctx, data, "document.txt")
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Compressed %d bytes to %d bytes (ratio: %.2f)\n",
    result.OriginalSize, result.CompressedSize, result.CompressionRatio)
```

### Policy-Based Compression

```go
// Create policy manager
policyManager := compression.NewPolicyManager(compressor)

// Create a compression policy
policy := &compression.CompressionPolicy{
    Name:     "Text Documents",
    TenantID: tenantID,
    Enabled:  true,
    Priority: 100,
    Rules: []compression.CompressionRule{
        {
            Name: "Large text files",
            Conditions: []compression.CompressionCondition{
                {
                    Type:     compression.ConditionFileExtension,
                    Operator: compression.OperatorIn,
                    Value:    []string{".txt", ".md", ".log"},
                },
                {
                    Type:     compression.ConditionFileSize,
                    Operator: compression.OperatorGreaterThan,
                    Value:    float64(1024), // 1KB
                },
            },
            Action: compression.CompressionAction{
                Strategy:         compression.StrategyText,
                CompressionLevel: 9,
                CacheResult:      true,
                CacheTTL:         24 * time.Hour,
            },
        },
    },
}

// Register the policy
err := policyManager.CreatePolicy(ctx, policy)
if err != nil {
    log.Fatal(err)
}

// Apply compression based on policy
fileInfo := &storage.FileInfo{
    Name: "document.txt",
    Size: 5000,
    URL:  "file://document.txt",
}

result, err := policyManager.ApplyCompression(ctx, data, fileInfo, tenantID)
if err != nil {
    log.Fatal(err)
}
```

### Storage Integration

```go
// Wrap existing storage resolver with compression
baseResolver := myExistingResolver
cache := myCache

config := compression.DefaultCompressionIntegrationConfig()
compressedResolver := compression.NewCompressedStorageResolver(
    baseResolver, policyManager, cache, config)

// Use compressed resolver like any other storage resolver
fileInfo, err := compressedResolver.GetFileInfo(ctx, storageURL, credentials)
reader, err := compressedResolver.DownloadFile(ctx, storageURL, credentials, nil)
```

### Batch Processing

```go
// Create batch compressor
batchCompressor := compression.NewBatchCompressor(compressor, 4) // 4 workers
batchCompressor.Start(ctx)

// Submit batch job
job := &compression.BatchCompressionJob{
    Files:    []storage.FileInfo{...},
    Strategy: compression.StrategyAdaptive,
    Priority: 1,
}

err := batchCompressor.SubmitJob(job)
if err != nil {
    log.Fatal(err)
}
```

## Configuration

### Document Compression Configuration

```yaml
# Document compression settings
enabled: true
default_strategy: "adaptive"
min_file_size_threshold: 1024      # 1KB
max_file_size_threshold: 524288000 # 500MB
compression_ratio_target: 0.7      # Target 30% size reduction

# File type mappings
text_file_extensions:
  - ".txt"
  - ".md"
  - ".json"
  - ".xml"
  - ".log"

binary_file_extensions:
  - ".pdf"
  - ".doc"
  - ".docx"

# Performance settings
buffer_size: 65536        # 64KB
compression_level: 6
max_concurrency: 4
enable_metrics: true

# Storage integration
cache_compressed: true
cache_ttl: "24h"
store_original: false
use_async_compression: true
```

### Storage Integration Configuration

```yaml
# Integration settings
enable_compression: true
cache_compressed: true
store_original: false
async_compression: true

# Thresholds
compression_threshold: 1024  # 1KB
cache_threshold: 10240       # 10KB

# Storage prefixes
compressed_prefix: "compressed/"
metadata_prefix: "metadata/"
original_prefix: "original/"

# Cleanup settings
cleanup_originals: true
cleanup_delay: "24h"
retention_period: "720h"  # 30 days
```

## Policy Examples

### Text Documents Policy
```yaml
name: "Text Documents Compression"
description: "High compression for text files"
tenant_id: "uuid"
enabled: true
priority: 100

rules:
  - name: "Large text files"
    conditions:
      - type: "file_extension"
        operator: "in"
        value: [".txt", ".md", ".log", ".json"]
      - type: "file_size"
        operator: "gt"
        value: 10240  # 10KB
    action:
      strategy: "text"
      compression_level: 9
      store_original: false
      cache_result: true
      cache_ttl: "24h"
```

### Size-Based Policy
```yaml
name: "Size-Based Compression"
description: "Aggressive compression for large files"
priority: 80

rules:
  - name: "Very large files"
    conditions:
      - type: "file_size"
        operator: "gt"
        value: 104857600  # 100MB
    action:
      strategy: "aggressive"
      compression_level: 9
      async_compression: true
```

### Age-Based Policy
```yaml
name: "Age-Based Compression"
description: "More compression for older files"
priority: 70

rules:
  - name: "Old files"
    conditions:
      - type: "age"
        operator: "gt"
        value: "720h"  # 30 days
    action:
      strategy: "aggressive"
      compression_level: 9
      store_original: false
```

## Monitoring and Metrics

### Compression Metrics
- Total compressions/decompressions
- Average compression ratios
- Processing times
- Error rates
- Cache hit rates

### Policy Metrics
- Policy evaluations
- Rule matches
- Default rule usage
- Compression success/failure rates

### Cache Metrics
- Cache hits/misses
- Memory usage
- Eviction rates
- Average latency

## Performance Considerations

### Memory Usage
- Streaming compression for large files
- Configurable buffer sizes
- Memory-efficient LRU caching
- Automatic cleanup of temporary data

### CPU Usage
- Configurable compression levels
- Adaptive algorithm selection
- Concurrent processing limits
- Background compression for non-critical files

### Storage Efficiency
- Intelligent strategy selection
- Skip compression for already-compressed files
- Original file cleanup policies
- Metadata optimization

## Best Practices

### Policy Design
1. **Use Priority Levels**: Higher priority policies are evaluated first
2. **Specific Before General**: Place more specific rules before general ones
3. **Performance Considerations**: Balance compression ratio with processing time
4. **Cache Strategy**: Cache frequently accessed compressed content
5. **Cleanup Policies**: Define clear retention and cleanup rules

### File Type Optimization
1. **Text Files**: Use high compression levels (7-9)
2. **Binary Files**: Use moderate compression levels (4-6)
3. **Already Compressed**: Skip compression entirely
4. **Large Files**: Consider async compression
5. **Small Files**: Skip compression or use fast algorithms

### Storage Integration
1. **Gradual Rollout**: Start with specific file types or directories
2. **Monitor Performance**: Track compression ratios and processing times
3. **Backup Strategy**: Ensure original files are safely backed up
4. **Error Handling**: Define clear error handling and recovery procedures

## Troubleshooting

### Common Issues

1. **Low Compression Ratios**
   - Check if files are already compressed
   - Verify compression strategy selection
   - Consider file type and content characteristics

2. **Performance Issues**
   - Reduce compression levels for better speed
   - Enable async compression for large files
   - Adjust worker pool sizes

3. **Memory Usage**
   - Reduce cache sizes
   - Lower buffer sizes for streaming
   - Enable cleanup policies

4. **Policy Not Matching**
   - Check policy priority and order
   - Verify condition operators and values
   - Test with sample files

### Debug Logging
Enable debug logging to troubleshoot:
```yaml
logging:
  level: debug
  components:
    - compression
    - policy
    - cache
```

## Integration with Existing Systems

The compression system is designed to integrate seamlessly with:
- Existing storage resolvers
- Cache implementations
- Monitoring systems
- Configuration management
- Multi-tenant architectures

For specific integration examples, see the `/examples/compression/` directory.