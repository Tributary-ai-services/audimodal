# Client-Side Encryption System

A comprehensive client-side encryption system for the AudiModal platform that provides secure, policy-based encryption for sensitive documents with seamless storage integration.

## Features

### Core Encryption Capabilities
- **Multiple Encryption Algorithms**: AES-256-GCM, AES-256-CBC, ChaCha20-Poly1305, XChaCha20-Poly1305
- **Advanced Key Management**: Hierarchical key structure with automatic rotation and secure storage
- **Key Derivation Functions**: Argon2id, PBKDF2, Scrypt for secure key derivation
- **Authenticated Encryption**: Built-in integrity protection with AEAD algorithms

### Policy-Based Management
- **Flexible Encryption Policies**: Define encryption rules based on file properties, classification, and context
- **Tenant Isolation**: Per-tenant encryption policies and key management
- **Conditional Logic**: Complex conditions for determining when and how to encrypt
- **Dynamic Configuration**: Runtime policy updates without system restarts

### Storage Integration
- **Transparent Encryption**: Seamless integration with existing storage resolvers
- **Automatic Key Management**: Secure key generation, rotation, and lifecycle management
- **Metadata Management**: Encrypted metadata storage alongside encrypted files
- **Cache Integration**: Intelligent caching of decrypted content for performance

### Security Features
- **Zero-Knowledge Architecture**: Client controls all encryption keys
- **Perfect Forward Secrecy**: Key rotation with secure key destruction
- **Secure Key Storage**: HSM/KMS integration ready
- **Audit Logging**: Comprehensive audit trail for all encryption operations

## Architecture

### Components

1. **KeyManager**: Manages encryption keys with rotation and caching
2. **Encryptor**: Performs encryption/decryption operations
3. **PolicyManager**: Evaluates and applies encryption policies
4. **EncryptedStorageResolver**: Storage integration layer

### Encryption Algorithms

| Algorithm | Key Size | Nonce Size | Use Case |
|-----------|----------|------------|----------|
| AES-256-GCM | 256 bits | 96 bits | High performance, FIPS 140-2 |
| AES-256-CBC | 256 bits | 128 bits | Legacy compatibility |
| ChaCha20-Poly1305 | 256 bits | 96 bits | Mobile/embedded devices |
| XChaCha20-Poly1305 | 256 bits | 192 bits | Large nonce requirements |

### Key Types

| Type | Purpose | Rotation | Storage |
|------|---------|----------|---------|
| Master | Tenant root key | Annually | HSM/KMS |
| Data | Document encryption | Monthly | Encrypted storage |
| Wrapping | Key encryption | Quarterly | HSM/KMS |
| Backup | Backup encryption | Semi-annually | Secure backup |
| Transit | Transport encryption | Weekly | Memory only |

## Usage

### Basic Encryption

```go
// Create encryption configuration
config := encryption.DefaultEncryptionConfig()
config.Algorithm = encryption.AlgorithmAES256GCM
config.KeyRotationEnabled = true

// Initialize components
keyManager := encryption.NewKeyManager(config, keyStore, auditLogger)
encryptor := encryption.NewEncryptor(config, keyManager)

// Create encryption context
encCtx := &encryption.EncryptionContext{
    TenantID:     tenantID,
    Purpose:      encryption.PurposeDocumentEncryption,
    ResourceID:   "document-123",
    ResourceType: "pdf",
}

// Encrypt document
data := []byte("Sensitive document content...")
result, err := encryptor.EncryptDocument(ctx, data, encCtx)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Encrypted %d bytes to %d bytes\n", 
    result.Metadata.OriginalSize, result.Metadata.EncryptedSize)
```

### Policy-Based Encryption

```go
// Create policy manager
policyManager := encryption.NewPolicyManager(policyStore, encryptor)

// Create encryption policy
policy := &encryption.EncryptionPolicy{
    Name:     "Confidential Documents",
    TenantID: tenantID,
    Enabled:  true,
    Priority: 100,
    Rules: []encryption.EncryptionRule{
        {
            Name: "Encrypt PDFs > 1MB",
            Conditions: []encryption.PolicyCondition{
                {
                    Type:     encryption.ConditionFileType,
                    Operator: "contains",
                    Value:    "pdf",
                },
                {
                    Type:     encryption.ConditionFileSize,
                    Operator: "gt",
                    Value:    float64(1024 * 1024), // 1MB
                },
            },
            Action: encryption.EncryptionAction{
                Encrypt:   true,
                Algorithm: encryption.AlgorithmAES256GCM,
                KeyType:   encryption.KeyTypeData,
                AuditAccess: true,
            },
        },
    },
    DefaultAction: encryption.EncryptionAction{
        Encrypt: false,
    },
}

// Register policy
err := policyManager.CreatePolicy(ctx, policy)
if err != nil {
    log.Fatal(err)
}

// Apply encryption based on policy
fileInfo := &storage.FileInfo{
    Name: "confidential.pdf",
    Size: 2 * 1024 * 1024, // 2MB
    ContentType: "application/pdf",
}

result, err := policyManager.ApplyEncryption(ctx, data, fileInfo, tenantID)
if err != nil {
    log.Fatal(err)
}
```

### Storage Integration

```go
// Wrap existing storage resolver with encryption
baseResolver := myExistingResolver
cache := myCache
config := encryption.DefaultEncryptionIntegrationConfig()

encryptedResolver := encryption.NewEncryptedStorageResolver(
    baseResolver, policyManager, encryptor, cache, config)

// Use encrypted resolver transparently
fileInfo, err := encryptedResolver.GetFileInfo(ctx, storageURL, credentials)
reader, err := encryptedResolver.DownloadFile(ctx, storageURL, credentials, nil)

// Files are automatically encrypted/decrypted based on policies
```

### Key Management

```go
// Generate new encryption key
key, err := keyManager.GenerateKey(ctx, tenantID, 
    encryption.KeyTypeData, encryption.PurposeDocumentEncryption)
if err != nil {
    log.Fatal(err)
}

// Rotate key
newKey, err := keyManager.RotateKey(ctx, key.ID)
if err != nil {
    log.Fatal(err)
}

// Revoke compromised key
err = keyManager.RevokeKey(ctx, key.ID, "Suspected compromise")
if err != nil {
    log.Fatal(err)
}
```

## Configuration

### Encryption Configuration

```yaml
# Encryption settings
enabled: true
algorithm: "AES256-GCM"
mode: "client_side"
kdf: "argon2id"

# Key management
key_rotation_enabled: true
key_rotation_interval: "720h"  # 30 days
key_cache_ttl: "1h"
max_keys_per_tenant: 100

# Performance
buffer_size: 65536              # 64KB
max_concurrent_ops: 10
enable_compression: true
compression_threshold: 1024     # 1KB

# Security
min_key_length: 32              # 256 bits
require_authentication: true
audit_encryption: true
secure_delete: true

# Argon2id parameters (OWASP recommended)
argon2_memory: 65536           # 64MB
argon2_iterations: 3
argon2_parallelism: 4
argon2_salt_length: 16

# PBKDF2 parameters
pbkdf2_iterations: 600000      # OWASP recommendation
pbkdf2_hash_function: "SHA256"

# Scrypt parameters
scrypt_n: 32768
scrypt_r: 8
scrypt_p: 1
```

### Storage Integration Configuration

```yaml
# Integration settings
enable_encryption: true
transparent_mode: true         # Auto encrypt/decrypt
cache_decrypted: true         # Cache decrypted content

# Thresholds
encryption_threshold: 0        # Encrypt all files
streaming_threshold: 10485760  # 10MB

# Storage paths
encrypted_prefix: ".encrypted/"
metadata_prefix: ".metadata/"
key_metadata_prefix: ".keys/"

# Security
require_encryption: false     # Fail if encryption not possible
audit_all_operations: true
secure_delete: true

# Cache settings
cache_ttl: "1h"
max_cache_size: 104857600     # 100MB
```

## Policy Examples

### Classification-Based Policy
```yaml
name: "Classification-Based Encryption"
description: "Encrypt based on data classification"
tenant_id: "uuid"
enabled: true
priority: 100

rules:
  - name: "Confidential Data"
    conditions:
      - type: "classification"
        operator: "eq"
        value: "confidential"
    action:
      encrypt: true
      algorithm: "AES256-GCM"
      key_type: "data"
      audit_access: true
      require_mfa: true

  - name: "PII Data"
    conditions:
      - type: "metadata"
        field: "contains_pii"
        operator: "eq"
        value: "true"
    action:
      encrypt: true
      algorithm: "XChaCha20-Poly1305"
      key_type: "data"
      double_encryption: true
      audit_access: true

default_action:
  encrypt: false
```

### Size and Type-Based Policy
```yaml
name: "Size and Type-Based Encryption"
description: "Encrypt large files and sensitive types"
priority: 90

rules:
  - name: "Large files"
    conditions:
      - type: "file_size"
        operator: "gt"
        value: 10485760  # 10MB
    action:
      encrypt: true
      algorithm: "ChaCha20-Poly1305"
      compress_first: true

  - name: "Financial documents"
    conditions:
      - type: "file_type"
        operator: "in"
        value: ["application/pdf", "text/csv"]
      - type: "path"
        operator: "contains"
        value: "/financial/"
    action:
      encrypt: true
      algorithm: "AES256-GCM"
      key_rotation: true
      audit_access: true
```

## Security Considerations

### Key Management
1. **Key Hierarchy**: Use proper key hierarchy with KEKs and DEKs
2. **Rotation**: Implement automatic key rotation with configurable intervals
3. **Storage**: Store keys in HSM or KMS when available
4. **Access Control**: Implement strict access controls for key operations
5. **Audit**: Log all key lifecycle events

### Encryption Best Practices
1. **Algorithm Selection**: Use AEAD algorithms (GCM, ChaCha20-Poly1305)
2. **Key Derivation**: Use strong KDFs with appropriate parameters
3. **Nonce Generation**: Use cryptographically secure random nonces
4. **Authentication**: Include additional authenticated data (AAD)
5. **Integrity**: Verify checksums and authentication tags

### Operational Security
1. **Zero-Knowledge**: Client controls all encryption keys
2. **Secure Deletion**: Implement secure key and data deletion
3. **Audit Logging**: Comprehensive audit trail for compliance
4. **Access Control**: Role-based access to encryption functions
5. **Monitoring**: Monitor for unusual encryption/decryption patterns

## Performance Optimization

### Memory Management
- **Streaming Encryption**: For files larger than threshold
- **Key Caching**: Cache frequently used keys with TTL
- **Buffer Management**: Configurable buffer sizes for encryption
- **Secure Cleanup**: Zero memory after operations

### Concurrency
- **Parallel Operations**: Support concurrent encryption/decryption
- **Worker Pools**: Manage encryption worker threads
- **Resource Limits**: Prevent resource exhaustion
- **Deadlock Prevention**: Careful lock ordering

### Caching Strategy
- **Decrypted Content**: Cache decrypted files for performance
- **Key Material**: Cache derived keys with secure cleanup
- **Metadata**: Cache encryption metadata
- **TTL Management**: Automatic cache expiration

## Compliance and Audit

### Audit Events
- Key generation, rotation, revocation, destruction
- Encryption and decryption operations
- Policy changes and evaluations
- Access attempts and failures
- System configuration changes

### Compliance Standards
- **FIPS 140-2**: Use approved algorithms and implementations
- **Common Criteria**: Follow security evaluation criteria
- **GDPR**: Support data protection and right to be forgotten
- **HIPAA**: Healthcare data protection requirements
- **SOX**: Financial data protection and audit requirements

## Integration with Existing Systems

The encryption system integrates seamlessly with:
- Existing storage resolvers and providers
- Cache implementations and strategies
- Audit and monitoring systems
- Key management services (AWS KMS, Azure Key Vault, etc.)
- Hardware security modules (HSMs)

For specific integration examples, see the test files and example configurations.