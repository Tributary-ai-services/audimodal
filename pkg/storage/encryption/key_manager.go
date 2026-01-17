package encryption

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/pbkdf2"
	"golang.org/x/crypto/scrypt"

	"crypto/sha256"
	"hash"
)

// KeyManager manages encryption keys for tenants
type KeyManager struct {
	config      *EncryptionConfig
	keyStore    KeyStore
	keyCache    *KeyCache
	rotationMgr *KeyRotationManager
	auditLogger AuditLogger
	tracer      trace.Tracer
	metrics     *EncryptionMetrics
	mu          sync.RWMutex
}

// KeyStore interface for persistent key storage
type KeyStore interface {
	StoreKey(ctx context.Context, key *EncryptionKey) error
	GetKey(ctx context.Context, keyID uuid.UUID) (*EncryptionKey, error)
	GetKeyByTenant(ctx context.Context, tenantID uuid.UUID, keyType KeyType, purpose KeyPurpose) (*EncryptionKey, error)
	ListKeys(ctx context.Context, tenantID uuid.UUID) ([]*EncryptionKey, error)
	UpdateKey(ctx context.Context, key *EncryptionKey) error
	DeleteKey(ctx context.Context, keyID uuid.UUID) error
	GetActiveKeys(ctx context.Context, tenantID uuid.UUID) ([]*EncryptionKey, error)
}

// AuditLogger interface for audit logging
type AuditLogger interface {
	LogEvent(ctx context.Context, event *EncryptionAuditEvent) error
}

// NewKeyManager creates a new key manager
func NewKeyManager(config *EncryptionConfig, keyStore KeyStore, auditLogger AuditLogger) *KeyManager {
	if config == nil {
		config = DefaultEncryptionConfig()
	}

	km := &KeyManager{
		config:      config,
		keyStore:    keyStore,
		keyCache:    NewKeyCache(config.KeyCacheTTL),
		auditLogger: auditLogger,
		tracer:      otel.Tracer("key-manager"),
		metrics:     &EncryptionMetrics{},
	}

	// Initialize rotation manager if enabled
	if config.KeyRotationEnabled {
		km.rotationMgr = NewKeyRotationManager(km, config.KeyRotationInterval)
	}

	return km
}

// GenerateKey generates a new encryption key for a tenant
func (km *KeyManager) GenerateKey(ctx context.Context, tenantID uuid.UUID, keyType KeyType, purpose KeyPurpose) (*EncryptionKey, error) {
	ctx, span := km.tracer.Start(ctx, "generate_encryption_key")
	defer span.End()

	span.SetAttributes(
		attribute.String("tenant.id", tenantID.String()),
		attribute.String("key.type", string(keyType)),
		attribute.String("key.purpose", string(purpose)),
	)

	// Generate key material
	keySize := km.getKeySizeForAlgorithm(km.config.Algorithm)
	keyMaterial := make([]byte, keySize)
	if _, err := rand.Read(keyMaterial); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to generate key material: %w", err)
	}

	// Create key object
	key := &EncryptionKey{
		ID:          uuid.New(),
		TenantID:    tenantID,
		KeyType:     keyType,
		Algorithm:   km.config.Algorithm,
		Version:     1,
		CreatedAt:   time.Now(),
		Status:      KeyStatusActive,
		Purpose:     purpose,
		Exportable:  false,
		Deletable:   true,
		RequiresMFA: keyType == KeyTypeMaster,
		KeyMetadata: make(map[string]string),
	}

	// Set expiration if rotation is enabled
	if km.config.KeyRotationEnabled {
		expiresAt := time.Now().Add(km.config.KeyRotationInterval)
		key.ExpiresAt = &expiresAt
	}

	// Encrypt key material for storage
	encryptedKey, err := km.encryptKeyMaterial(keyMaterial)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to encrypt key material: %w", err)
	}
	key.EncryptedKey = encryptedKey

	// Store key
	if err := km.keyStore.StoreKey(ctx, key); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to store key: %w", err)
	}

	// Cache the key
	km.keyCache.Set(key.ID, key)

	// Audit log
	km.auditKeyEvent(ctx, AuditEventKeyCreate, tenantID, key.ID, true, "")

	// Update metrics
	km.metrics.ActiveKeys++

	span.SetAttributes(
		attribute.String("key.id", key.ID.String()),
		attribute.String("key.algorithm", string(key.Algorithm)),
	)

	return key, nil
}

// GetKey retrieves an encryption key by ID
func (km *KeyManager) GetKey(ctx context.Context, keyID uuid.UUID) (*EncryptionKey, error) {
	ctx, span := km.tracer.Start(ctx, "get_encryption_key")
	defer span.End()

	span.SetAttributes(attribute.String("key.id", keyID.String()))

	// Check cache first
	if key := km.keyCache.Get(keyID); key != nil {
		span.SetAttributes(attribute.Bool("cache.hit", true))
		return key, nil
	}

	// Load from store
	key, err := km.keyStore.GetKey(ctx, keyID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get key: %w", err)
	}

	// Validate key status
	if key.Status != KeyStatusActive && key.Status != KeyStatusRotating {
		return nil, fmt.Errorf("key %s is not active (status: %s)", keyID, key.Status)
	}

	// Check expiration
	if key.ExpiresAt != nil && time.Now().After(*key.ExpiresAt) {
		key.Status = KeyStatusExpired
		km.keyStore.UpdateKey(ctx, key)
		return nil, fmt.Errorf("key %s has expired", keyID)
	}

	// Cache the key
	km.keyCache.Set(key.ID, key)

	// Update usage
	key.UsageCount++
	now := time.Now()
	key.LastUsed = &now
	go km.keyStore.UpdateKey(context.Background(), key) // Async update

	return key, nil
}

// GetOrCreateKey gets an existing key or creates a new one
func (km *KeyManager) GetOrCreateKey(ctx context.Context, tenantID uuid.UUID, keyType KeyType, purpose KeyPurpose) (*EncryptionKey, error) {
	ctx, span := km.tracer.Start(ctx, "get_or_create_key")
	defer span.End()

	// Try to get existing key
	key, err := km.keyStore.GetKeyByTenant(ctx, tenantID, keyType, purpose)
	if err == nil && key != nil && key.Status == KeyStatusActive {
		// Update cache
		km.keyCache.Set(key.ID, key)
		return key, nil
	}

	// Create new key
	return km.GenerateKey(ctx, tenantID, keyType, purpose)
}

// RotateKey rotates an encryption key
func (km *KeyManager) RotateKey(ctx context.Context, keyID uuid.UUID) (*EncryptionKey, error) {
	ctx, span := km.tracer.Start(ctx, "rotate_key")
	defer span.End()

	span.SetAttributes(attribute.String("key.id", keyID.String()))

	// Get existing key
	oldKey, err := km.GetKey(ctx, keyID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get key for rotation: %w", err)
	}

	// Mark old key as rotating
	oldKey.Status = KeyStatusRotating
	if err := km.keyStore.UpdateKey(ctx, oldKey); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to update old key status: %w", err)
	}

	// Generate new key
	newKey, err := km.GenerateKey(ctx, oldKey.TenantID, oldKey.KeyType, oldKey.Purpose)
	if err != nil {
		span.RecordError(err)
		// Revert old key status
		oldKey.Status = KeyStatusActive
		km.keyStore.UpdateKey(ctx, oldKey)
		return nil, fmt.Errorf("failed to generate new key: %w", err)
	}

	// Link new key to old key
	newKey.RotatedFrom = &oldKey.ID
	newKey.Version = oldKey.Version + 1
	if err := km.keyStore.UpdateKey(ctx, newKey); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to update new key: %w", err)
	}

	// Schedule old key for expiration
	gracePeriod := 7 * 24 * time.Hour // 7 days grace period
	expiresAt := time.Now().Add(gracePeriod)
	oldKey.ExpiresAt = &expiresAt
	oldKey.Status = KeyStatusActive // Keep active during grace period
	if err := km.keyStore.UpdateKey(ctx, oldKey); err != nil {
		span.RecordError(err)
		// Log error but continue
	}

	// Clear cache for old key
	km.keyCache.Delete(oldKey.ID)

	// Audit log
	km.auditKeyEvent(ctx, AuditEventKeyRotate, oldKey.TenantID, newKey.ID, true, "")

	// Update metrics
	km.metrics.KeyRotations++

	span.SetAttributes(
		attribute.String("new_key.id", newKey.ID.String()),
		attribute.Int("new_key.version", newKey.Version),
	)

	return newKey, nil
}

// RevokeKey revokes an encryption key
func (km *KeyManager) RevokeKey(ctx context.Context, keyID uuid.UUID, reason string) error {
	ctx, span := km.tracer.Start(ctx, "revoke_key")
	defer span.End()

	span.SetAttributes(
		attribute.String("key.id", keyID.String()),
		attribute.String("reason", reason),
	)

	key, err := km.GetKey(ctx, keyID)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to get key for revocation: %w", err)
	}

	// Update key status
	key.Status = KeyStatusRevoked
	key.KeyMetadata["revocation_reason"] = reason
	key.KeyMetadata["revoked_at"] = time.Now().Format(time.RFC3339)

	if err := km.keyStore.UpdateKey(ctx, key); err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to revoke key: %w", err)
	}

	// Remove from cache
	km.keyCache.Delete(key.ID)

	// Audit log
	km.auditKeyEvent(ctx, AuditEventKeyRevoke, key.TenantID, key.ID, true, reason)

	// Update metrics
	km.metrics.ActiveKeys--

	return nil
}

// DestroyKey permanently destroys a key
func (km *KeyManager) DestroyKey(ctx context.Context, keyID uuid.UUID) error {
	ctx, span := km.tracer.Start(ctx, "destroy_key")
	defer span.End()

	span.SetAttributes(attribute.String("key.id", keyID.String()))

	key, err := km.keyStore.GetKey(ctx, keyID)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to get key for destruction: %w", err)
	}

	// Check if key is deletable
	if !key.Deletable {
		return fmt.Errorf("key %s is not deletable", keyID)
	}

	// Check if key is in use
	if key.Status == KeyStatusActive {
		return fmt.Errorf("cannot destroy active key %s", keyID)
	}

	// Delete from store
	if err := km.keyStore.DeleteKey(ctx, keyID); err != nil {
		span.RecordError(err)
		return fmt.Errorf("failed to destroy key: %w", err)
	}

	// Remove from cache
	km.keyCache.Delete(key.ID)

	// Audit log
	km.auditKeyEvent(ctx, AuditEventKeyDestroy, key.TenantID, key.ID, true, "")

	return nil
}

// DeriveKey derives a key from a master key using KDF
func (km *KeyManager) DeriveKey(ctx context.Context, masterKey []byte, salt []byte, info []byte) ([]byte, error) {
	ctx, span := km.tracer.Start(ctx, "derive_key")
	defer span.End()

	keySize := km.getKeySizeForAlgorithm(km.config.Algorithm)

	switch km.config.KDF {
	case KDFArgon2id:
		return km.deriveKeyArgon2(masterKey, salt, keySize), nil

	case KDFPBKDF2:
		return km.deriveKeyPBKDF2(masterKey, salt, keySize), nil

	case KDFScrypt:
		return km.deriveKeyScrypt(masterKey, salt, keySize)

	default:
		return nil, fmt.Errorf("unsupported KDF: %s", km.config.KDF)
	}
}

// Helper methods

func (km *KeyManager) getKeySizeForAlgorithm(algorithm EncryptionAlgorithm) int {
	switch algorithm {
	case AlgorithmAES256GCM, AlgorithmAES256CBC:
		return 32 // 256 bits
	case AlgorithmChaCha20, AlgorithmXChaCha20:
		return 32 // 256 bits
	default:
		return 32 // Default to 256 bits
	}
}

func (km *KeyManager) encryptKeyMaterial(keyMaterial []byte) ([]byte, error) {
	// In production, this would use a Hardware Security Module (HSM) or
	// Key Management Service (KMS) to encrypt the key material.
	// For now, we'll use a simple base64 encoding as a placeholder.
	// TODO: Implement proper key wrapping with KEK
	return []byte(base64.StdEncoding.EncodeToString(keyMaterial)), nil
}

func (km *KeyManager) decryptKeyMaterial(encryptedKey []byte) ([]byte, error) {
	// TODO: Implement proper key unwrapping with KEK
	return base64.StdEncoding.DecodeString(string(encryptedKey))
}

func (km *KeyManager) deriveKeyArgon2(masterKey, salt []byte, keySize int) []byte {
	return argon2.IDKey(
		masterKey,
		salt,
		km.config.Argon2Iterations,
		km.config.Argon2Memory,
		km.config.Argon2Parallelism,
		uint32(keySize),
	)
}

func (km *KeyManager) deriveKeyPBKDF2(masterKey, salt []byte, keySize int) []byte {
	var h func() hash.Hash
	switch km.config.PBKDF2HashFunction {
	case "SHA256":
		h = sha256.New
	default:
		h = sha256.New
	}

	return pbkdf2.Key(masterKey, salt, km.config.PBKDF2Iterations, keySize, h)
}

func (km *KeyManager) deriveKeyScrypt(masterKey, salt []byte, keySize int) ([]byte, error) {
	return scrypt.Key(
		masterKey,
		salt,
		km.config.ScryptN,
		km.config.ScryptR,
		km.config.ScryptP,
		keySize,
	)
}

func (km *KeyManager) auditKeyEvent(ctx context.Context, eventType AuditEventType, tenantID, keyID uuid.UUID, success bool, errorMsg string) {
	event := &EncryptionAuditEvent{
		ID:           uuid.New(),
		Timestamp:    time.Now(),
		TenantID:     tenantID,
		EventType:    eventType,
		KeyID:        &keyID,
		Success:      success,
		ErrorMessage: errorMsg,
	}

	if km.auditLogger != nil {
		if err := km.auditLogger.LogEvent(ctx, event); err != nil {
			// Log error but don't fail the operation
			span := trace.SpanFromContext(ctx)
			span.AddEvent("audit_log_failed", trace.WithAttributes(
				attribute.String("error", err.Error()),
			))
		}
	}
}

// GetDecryptionKey gets the key material for decryption
func (km *KeyManager) GetDecryptionKey(ctx context.Context, keyID uuid.UUID) ([]byte, error) {
	key, err := km.GetKey(ctx, keyID)
	if err != nil {
		return nil, err
	}

	return km.decryptKeyMaterial(key.EncryptedKey)
}

// KeyCache provides in-memory caching for encryption keys
type KeyCache struct {
	cache map[uuid.UUID]*cacheEntry
	ttl   time.Duration
	mu    sync.RWMutex
}

type cacheEntry struct {
	key       *EncryptionKey
	expiresAt time.Time
}

// NewKeyCache creates a new key cache
func NewKeyCache(ttl time.Duration) *KeyCache {
	kc := &KeyCache{
		cache: make(map[uuid.UUID]*cacheEntry),
		ttl:   ttl,
	}

	// Start cleanup routine
	go kc.cleanup()

	return kc
}

// Set adds a key to the cache
func (kc *KeyCache) Set(keyID uuid.UUID, key *EncryptionKey) {
	kc.mu.Lock()
	defer kc.mu.Unlock()

	kc.cache[keyID] = &cacheEntry{
		key:       key,
		expiresAt: time.Now().Add(kc.ttl),
	}
}

// Get retrieves a key from the cache
func (kc *KeyCache) Get(keyID uuid.UUID) *EncryptionKey {
	kc.mu.RLock()
	defer kc.mu.RUnlock()

	entry, exists := kc.cache[keyID]
	if !exists || time.Now().After(entry.expiresAt) {
		return nil
	}

	return entry.key
}

// Delete removes a key from the cache
func (kc *KeyCache) Delete(keyID uuid.UUID) {
	kc.mu.Lock()
	defer kc.mu.Unlock()

	delete(kc.cache, keyID)
}

// cleanup removes expired entries
func (kc *KeyCache) cleanup() {
	ticker := time.NewTicker(kc.ttl / 2)
	defer ticker.Stop()

	for range ticker.C {
		kc.mu.Lock()
		now := time.Now()
		for keyID, entry := range kc.cache {
			if now.After(entry.expiresAt) {
				delete(kc.cache, keyID)
			}
		}
		kc.mu.Unlock()
	}
}

// KeyRotationManager handles automatic key rotation
type KeyRotationManager struct {
	keyManager *KeyManager
	interval   time.Duration
	stopCh     chan struct{}
	wg         sync.WaitGroup
}

// NewKeyRotationManager creates a new rotation manager
func NewKeyRotationManager(keyManager *KeyManager, interval time.Duration) *KeyRotationManager {
	return &KeyRotationManager{
		keyManager: keyManager,
		interval:   interval,
		stopCh:     make(chan struct{}),
	}
}

// Start begins the rotation process
func (krm *KeyRotationManager) Start(ctx context.Context) {
	krm.wg.Add(1)
	go krm.rotationLoop(ctx)
}

// Stop stops the rotation process
func (krm *KeyRotationManager) Stop() {
	close(krm.stopCh)
	krm.wg.Wait()
}

// rotationLoop checks for keys that need rotation
func (krm *KeyRotationManager) rotationLoop(ctx context.Context) {
	defer krm.wg.Done()

	ticker := time.NewTicker(krm.interval / 24) // Check daily
	defer ticker.Stop()

	for {
		select {
		case <-krm.stopCh:
			return
		case <-ticker.C:
			krm.checkAndRotateKeys(ctx)
		}
	}
}

// checkAndRotateKeys checks all keys and rotates those that need it
func (krm *KeyRotationManager) checkAndRotateKeys(ctx context.Context) {
	// This would iterate through all tenants and check their keys
	// For now, this is a placeholder
	// TODO: Implement full rotation logic
}
