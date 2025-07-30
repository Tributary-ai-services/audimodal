package encryption

import (
	"time"

	"github.com/google/uuid"
)

// EncryptionAlgorithm represents the encryption algorithm used
type EncryptionAlgorithm string

const (
	AlgorithmAES256GCM EncryptionAlgorithm = "AES256-GCM"
	AlgorithmAES256CBC EncryptionAlgorithm = "AES256-CBC"
	AlgorithmChaCha20  EncryptionAlgorithm = "ChaCha20-Poly1305"
	AlgorithmXChaCha20 EncryptionAlgorithm = "XChaCha20-Poly1305"
)

// KeyDerivationFunction represents the KDF algorithm
type KeyDerivationFunction string

const (
	KDFArgon2id   KeyDerivationFunction = "argon2id"
	KDFPBKDF2     KeyDerivationFunction = "pbkdf2"
	KDFScrypt     KeyDerivationFunction = "scrypt"
)

// EncryptionMode represents how encryption is applied
type EncryptionMode string

const (
	ModeTransparent  EncryptionMode = "transparent"  // Automatic encryption/decryption
	ModeExplicit     EncryptionMode = "explicit"     // Manual encryption/decryption
	ModeEnvelope     EncryptionMode = "envelope"     // Envelope encryption with DEK/KEK
	ModeClientSide   EncryptionMode = "client_side"  // Client manages all keys
)

// EncryptionConfig contains configuration for the encryption system
type EncryptionConfig struct {
	// General settings
	Enabled              bool                  `yaml:"enabled"`
	Algorithm            EncryptionAlgorithm   `yaml:"algorithm"`
	Mode                 EncryptionMode        `yaml:"mode"`
	KDF                  KeyDerivationFunction `yaml:"kdf"`
	
	// Key management
	KeyRotationEnabled   bool                  `yaml:"key_rotation_enabled"`
	KeyRotationInterval  time.Duration         `yaml:"key_rotation_interval"`
	KeyCacheTTL          time.Duration         `yaml:"key_cache_ttl"`
	MaxKeysPerTenant     int                   `yaml:"max_keys_per_tenant"`
	
	// Performance settings
	BufferSize           int                   `yaml:"buffer_size"`
	MaxConcurrentOps     int                   `yaml:"max_concurrent_ops"`
	EnableCompression    bool                  `yaml:"enable_compression"`
	CompressionThreshold int64                 `yaml:"compression_threshold"`
	
	// Security settings
	MinKeyLength         int                   `yaml:"min_key_length"`
	RequireAuthentication bool                 `yaml:"require_authentication"`
	AuditEncryption      bool                  `yaml:"audit_encryption"`
	SecureDelete         bool                  `yaml:"secure_delete"`
	
	// KDF parameters
	Argon2Memory         uint32                `yaml:"argon2_memory"`         // Memory in KiB
	Argon2Iterations     uint32                `yaml:"argon2_iterations"`
	Argon2Parallelism    uint8                 `yaml:"argon2_parallelism"`
	Argon2SaltLength     int                   `yaml:"argon2_salt_length"`
	
	PBKDF2Iterations     int                   `yaml:"pbkdf2_iterations"`
	PBKDF2HashFunction   string                `yaml:"pbkdf2_hash_function"`
	
	ScryptN              int                   `yaml:"scrypt_n"`
	ScryptR              int                   `yaml:"scrypt_r"`
	ScryptP              int                   `yaml:"scrypt_p"`
}

// DefaultEncryptionConfig returns default encryption configuration
func DefaultEncryptionConfig() *EncryptionConfig {
	return &EncryptionConfig{
		Enabled:              true,
		Algorithm:            AlgorithmAES256GCM,
		Mode:                 ModeClientSide,
		KDF:                  KDFArgon2id,
		KeyRotationEnabled:   true,
		KeyRotationInterval:  30 * 24 * time.Hour, // 30 days
		KeyCacheTTL:          1 * time.Hour,
		MaxKeysPerTenant:     100,
		BufferSize:           64 * 1024, // 64KB
		MaxConcurrentOps:     10,
		EnableCompression:    true,
		CompressionThreshold: 1024, // 1KB
		MinKeyLength:         32,   // 256 bits
		RequireAuthentication: true,
		AuditEncryption:      true,
		SecureDelete:         true,
		// Argon2id parameters (OWASP recommendations)
		Argon2Memory:         64 * 1024, // 64MB
		Argon2Iterations:     3,
		Argon2Parallelism:    4,
		Argon2SaltLength:     16,
		// PBKDF2 parameters
		PBKDF2Iterations:     600000, // OWASP recommendation for PBKDF2-SHA256
		PBKDF2HashFunction:   "SHA256",
		// Scrypt parameters
		ScryptN:              32768,
		ScryptR:              8,
		ScryptP:              1,
	}
}

// EncryptionKey represents an encryption key with metadata
type EncryptionKey struct {
	ID              uuid.UUID             `json:"id"`
	TenantID        uuid.UUID             `json:"tenant_id"`
	KeyType         KeyType               `json:"key_type"`
	Algorithm       EncryptionAlgorithm   `json:"algorithm"`
	Version         int                   `json:"version"`
	CreatedAt       time.Time             `json:"created_at"`
	ExpiresAt       *time.Time            `json:"expires_at,omitempty"`
	RotatedFrom     *uuid.UUID            `json:"rotated_from,omitempty"`
	Status          KeyStatus             `json:"status"`
	Purpose         KeyPurpose            `json:"purpose"`
	
	// Key material (encrypted at rest)
	EncryptedKey    []byte                `json:"-"` // Never serialize raw key material
	KeyMetadata     map[string]string     `json:"key_metadata,omitempty"`
	
	// Usage tracking
	UsageCount      int64                 `json:"usage_count"`
	LastUsed        *time.Time            `json:"last_used,omitempty"`
	
	// Security attributes
	Exportable      bool                  `json:"exportable"`
	Deletable       bool                  `json:"deletable"`
	RequiresMFA     bool                  `json:"requires_mfa"`
}

// KeyType represents the type of encryption key
type KeyType string

const (
	KeyTypeMaster    KeyType = "master"     // Master key for tenant
	KeyTypeData      KeyType = "data"       // Data encryption key
	KeyTypeWrapping  KeyType = "wrapping"   // Key encryption key
	KeyTypeBackup    KeyType = "backup"     // Backup encryption key
	KeyTypeTransit   KeyType = "transit"    // Transit encryption key
)

// KeyStatus represents the status of an encryption key
type KeyStatus string

const (
	KeyStatusActive    KeyStatus = "active"
	KeyStatusRotating  KeyStatus = "rotating"
	KeyStatusExpired   KeyStatus = "expired"
	KeyStatusRevoked   KeyStatus = "revoked"
	KeyStatusDestroyed KeyStatus = "destroyed"
)

// KeyPurpose represents what the key is used for
type KeyPurpose string

const (
	PurposeDocumentEncryption  KeyPurpose = "document_encryption"
	PurposeMetadataEncryption  KeyPurpose = "metadata_encryption"
	PurposeIndexEncryption     KeyPurpose = "index_encryption"
	PurposeBackupEncryption    KeyPurpose = "backup_encryption"
	PurposeTransportEncryption KeyPurpose = "transport_encryption"
)

// EncryptionMetadata contains metadata about encrypted data
type EncryptionMetadata struct {
	Version          int                   `json:"version"`
	Algorithm        EncryptionAlgorithm   `json:"algorithm"`
	KeyID            uuid.UUID             `json:"key_id"`
	EncryptedAt      time.Time             `json:"encrypted_at"`
	OriginalSize     int64                 `json:"original_size"`
	EncryptedSize    int64                 `json:"encrypted_size"`
	Compressed       bool                  `json:"compressed"`
	CompressionRatio float64               `json:"compression_ratio,omitempty"`
	Nonce            []byte                `json:"nonce"`
	Salt             []byte                `json:"salt,omitempty"`
	Checksum         string                `json:"checksum"`
	ChecksumType     string                `json:"checksum_type"`
	AdditionalData   map[string]string     `json:"additional_data,omitempty"`
}

// EncryptionResult contains the result of an encryption operation
type EncryptionResult struct {
	EncryptedData    []byte                `json:"-"` // Don't serialize encrypted data
	Metadata         *EncryptionMetadata   `json:"metadata"`
	KeyID            uuid.UUID             `json:"key_id"`
	Duration         time.Duration         `json:"duration"`
	Compressed       bool                  `json:"compressed"`
}

// DecryptionResult contains the result of a decryption operation
type DecryptionResult struct {
	DecryptedData    []byte                `json:"-"` // Don't serialize decrypted data
	OriginalSize     int64                 `json:"original_size"`
	Duration         time.Duration         `json:"duration"`
	KeyID            uuid.UUID             `json:"key_id"`
	Decompressed     bool                  `json:"decompressed"`
}

// EncryptionPolicy defines encryption rules for different scenarios
type EncryptionPolicy struct {
	ID               uuid.UUID             `json:"id"`
	Name             string                `json:"name"`
	Description      string                `json:"description"`
	TenantID         uuid.UUID             `json:"tenant_id"`
	
	// Policy rules
	Rules            []EncryptionRule      `json:"rules"`
	DefaultAction    EncryptionAction      `json:"default_action"`
	
	// Policy metadata
	CreatedAt        time.Time             `json:"created_at"`
	UpdatedAt        time.Time             `json:"updated_at"`
	CreatedBy        string                `json:"created_by"`
	Enabled          bool                  `json:"enabled"`
	Priority         int                   `json:"priority"`
}

// EncryptionRule defines when to apply encryption
type EncryptionRule struct {
	ID               uuid.UUID             `json:"id"`
	Name             string                `json:"name"`
	Conditions       []PolicyCondition     `json:"conditions"`
	Action           EncryptionAction      `json:"action"`
	Priority         int                   `json:"priority"`
	Enabled          bool                  `json:"enabled"`
}

// PolicyCondition defines conditions for applying encryption
type PolicyCondition struct {
	Type             ConditionType         `json:"type"`
	Operator         string                `json:"operator"`
	Value            interface{}           `json:"value"`
	Field            string                `json:"field,omitempty"`
}

// ConditionType for encryption policies
type ConditionType string

const (
	ConditionFileType        ConditionType = "file_type"
	ConditionFileSize        ConditionType = "file_size"
	ConditionDataSource      ConditionType = "data_source"
	ConditionClassification  ConditionType = "classification"
	ConditionTenant          ConditionType = "tenant"
	ConditionPath            ConditionType = "path"
	ConditionMetadata        ConditionType = "metadata"
)

// EncryptionAction defines what encryption to apply
type EncryptionAction struct {
	Encrypt          bool                  `json:"encrypt"`
	Algorithm        EncryptionAlgorithm   `json:"algorithm"`
	KeyType          KeyType               `json:"key_type"`
	CompressFirst    bool                  `json:"compress_first"`
	AuditAccess      bool                  `json:"audit_access"`
	RequireMFA       bool                  `json:"require_mfa"`
	
	// Advanced options
	DoubleEncryption bool                  `json:"double_encryption"`
	KeyRotation      bool                  `json:"key_rotation"`
	SecureDelete     bool                  `json:"secure_delete"`
}

// EncryptionAuditEvent represents an encryption-related audit event
type EncryptionAuditEvent struct {
	ID               uuid.UUID             `json:"id"`
	Timestamp        time.Time             `json:"timestamp"`
	TenantID         uuid.UUID             `json:"tenant_id"`
	UserID           string                `json:"user_id"`
	EventType        AuditEventType        `json:"event_type"`
	ResourceID       string                `json:"resource_id"`
	ResourceType     string                `json:"resource_type"`
	KeyID            *uuid.UUID            `json:"key_id,omitempty"`
	Algorithm        EncryptionAlgorithm   `json:"algorithm,omitempty"`
	Success          bool                  `json:"success"`
	ErrorMessage     string                `json:"error_message,omitempty"`
	IPAddress        string                `json:"ip_address"`
	UserAgent        string                `json:"user_agent"`
	AdditionalData   map[string]interface{} `json:"additional_data,omitempty"`
}

// AuditEventType represents types of audit events
type AuditEventType string

const (
	AuditEventEncrypt        AuditEventType = "encrypt"
	AuditEventDecrypt        AuditEventType = "decrypt"
	AuditEventKeyCreate      AuditEventType = "key_create"
	AuditEventKeyRotate      AuditEventType = "key_rotate"
	AuditEventKeyRevoke      AuditEventType = "key_revoke"
	AuditEventKeyDestroy     AuditEventType = "key_destroy"
	AuditEventKeyExport      AuditEventType = "key_export"
	AuditEventPolicyChange   AuditEventType = "policy_change"
	AuditEventAccessDenied   AuditEventType = "access_denied"
)

// EncryptionMetrics tracks encryption system metrics
type EncryptionMetrics struct {
	TotalEncryptions     int64                 `json:"total_encryptions"`
	TotalDecryptions     int64                 `json:"total_decryptions"`
	EncryptionErrors     int64                 `json:"encryption_errors"`
	DecryptionErrors     int64                 `json:"decryption_errors"`
	ActiveKeys           int64                 `json:"active_keys"`
	KeyRotations         int64                 `json:"key_rotations"`
	AverageEncryptTime   time.Duration         `json:"average_encrypt_time"`
	AverageDecryptTime   time.Duration         `json:"average_decrypt_time"`
	BytesEncrypted       int64                 `json:"bytes_encrypted"`
	BytesDecrypted       int64                 `json:"bytes_decrypted"`
}

// KeyRotationStrategy defines how keys are rotated
type KeyRotationStrategy struct {
	AutoRotate       bool                  `json:"auto_rotate"`
	RotationInterval time.Duration         `json:"rotation_interval"`
	GracePeriod      time.Duration         `json:"grace_period"`
	MaxVersions      int                   `json:"max_versions"`
	RetireOldKeys    bool                  `json:"retire_old_keys"`
}

// EncryptionContext provides context for encryption operations
type EncryptionContext struct {
	TenantID         uuid.UUID             `json:"tenant_id"`
	UserID           string                `json:"user_id"`
	Purpose          KeyPurpose            `json:"purpose"`
	ResourceID       string                `json:"resource_id"`
	ResourceType     string                `json:"resource_type"`
	Classification   string                `json:"classification"`
	AdditionalData   map[string]string     `json:"additional_data"`
}