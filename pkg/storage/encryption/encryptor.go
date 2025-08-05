package encryption

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/crypto/chacha20poly1305"
)

// Encryptor provides document encryption capabilities
type Encryptor struct {
	config     *EncryptionConfig
	keyManager *KeyManager
	tracer     trace.Tracer
	metrics    *EncryptionMetrics
}

// NewEncryptor creates a new encryptor
func NewEncryptor(config *EncryptionConfig, keyManager *KeyManager) *Encryptor {
	if config == nil {
		config = DefaultEncryptionConfig()
	}

	return &Encryptor{
		config:     config,
		keyManager: keyManager,
		tracer:     otel.Tracer("encryptor"),
		metrics:    &EncryptionMetrics{},
	}
}

// EncryptDocument encrypts a document with the appropriate key
func (e *Encryptor) EncryptDocument(ctx context.Context, data []byte, encCtx *EncryptionContext) (*EncryptionResult, error) {
	ctx, span := e.tracer.Start(ctx, "encrypt_document")
	defer span.End()

	start := time.Now()
	defer func() {
		e.metrics.TotalEncryptions++
		e.metrics.BytesEncrypted += int64(len(data))
		e.metrics.AverageEncryptTime = time.Since(start)
	}()

	span.SetAttributes(
		attribute.String("tenant.id", encCtx.TenantID.String()),
		attribute.Int("data.size", len(data)),
		attribute.String("purpose", string(encCtx.Purpose)),
	)

	// Get or create encryption key
	key, err := e.keyManager.GetOrCreateKey(ctx, encCtx.TenantID, KeyTypeData, encCtx.Purpose)
	if err != nil {
		span.RecordError(err)
		e.metrics.EncryptionErrors++
		return nil, fmt.Errorf("failed to get encryption key: %w", err)
	}

	// Get key material
	keyMaterial, err := e.keyManager.GetDecryptionKey(ctx, key.ID)
	if err != nil {
		span.RecordError(err)
		e.metrics.EncryptionErrors++
		return nil, fmt.Errorf("failed to get key material: %w", err)
	}

	// Compress if enabled and beneficial
	originalSize := int64(len(data))
	compressed := false
	if e.config.EnableCompression && originalSize > e.config.CompressionThreshold {
		compressedData, err := e.compress(data)
		if err == nil && len(compressedData) < len(data) {
			data = compressedData
			compressed = true
			span.SetAttributes(attribute.Bool("compressed", true))
		}
	}

	// Generate nonce
	nonceSize := e.getNonceSize(key.Algorithm)
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		span.RecordError(err)
		e.metrics.EncryptionErrors++
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt data
	encryptedData, err := e.encryptData(key.Algorithm, keyMaterial, nonce, data, encCtx)
	if err != nil {
		span.RecordError(err)
		e.metrics.EncryptionErrors++
		return nil, fmt.Errorf("encryption failed: %w", err)
	}

	// Calculate checksum
	checksum := e.calculateChecksum(encryptedData)

	// Create metadata
	metadata := &EncryptionMetadata{
		Version:       1,
		Algorithm:     key.Algorithm,
		KeyID:         key.ID,
		EncryptedAt:   time.Now(),
		OriginalSize:  originalSize,
		EncryptedSize: int64(len(encryptedData)),
		Compressed:    compressed,
		Nonce:         nonce,
		Checksum:      checksum,
		ChecksumType:  "SHA256",
	}

	if compressed {
		metadata.CompressionRatio = float64(originalSize) / float64(len(data))
	}

	// Add additional context to metadata
	if encCtx.AdditionalData != nil {
		metadata.AdditionalData = encCtx.AdditionalData
	}

	result := &EncryptionResult{
		EncryptedData: encryptedData,
		Metadata:      metadata,
		KeyID:         key.ID,
		Duration:      time.Since(start),
		Compressed:    compressed,
	}

	span.SetAttributes(
		attribute.String("key.id", key.ID.String()),
		attribute.String("algorithm", string(key.Algorithm)),
		attribute.Int("encrypted.size", len(encryptedData)),
		attribute.Float64("encryption.duration_ms", float64(result.Duration.Milliseconds())),
	)

	return result, nil
}

// DecryptDocument decrypts a document
func (e *Encryptor) DecryptDocument(ctx context.Context, encryptedData []byte, metadata *EncryptionMetadata) (*DecryptionResult, error) {
	ctx, span := e.tracer.Start(ctx, "decrypt_document")
	defer span.End()

	start := time.Now()
	defer func() {
		e.metrics.TotalDecryptions++
		e.metrics.BytesDecrypted += metadata.OriginalSize
		e.metrics.AverageDecryptTime = time.Since(start)
	}()

	span.SetAttributes(
		attribute.String("key.id", metadata.KeyID.String()),
		attribute.String("algorithm", string(metadata.Algorithm)),
		attribute.Int("encrypted.size", len(encryptedData)),
	)

	// Verify checksum
	checksum := e.calculateChecksum(encryptedData)
	if checksum != metadata.Checksum {
		e.metrics.DecryptionErrors++
		return nil, fmt.Errorf("checksum verification failed")
	}

	// Get decryption key
	keyMaterial, err := e.keyManager.GetDecryptionKey(ctx, metadata.KeyID)
	if err != nil {
		span.RecordError(err)
		e.metrics.DecryptionErrors++
		return nil, fmt.Errorf("failed to get decryption key: %w", err)
	}

	// Decrypt data
	decryptedData, err := e.decryptData(metadata.Algorithm, keyMaterial, metadata.Nonce, encryptedData, metadata.AdditionalData)
	if err != nil {
		span.RecordError(err)
		e.metrics.DecryptionErrors++
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	// Decompress if needed
	decompressed := false
	if metadata.Compressed {
		decompressedData, err := e.decompress(decryptedData)
		if err != nil {
			span.RecordError(err)
			e.metrics.DecryptionErrors++
			return nil, fmt.Errorf("decompression failed: %w", err)
		}
		decryptedData = decompressedData
		decompressed = true
	}

	result := &DecryptionResult{
		DecryptedData: decryptedData,
		OriginalSize:  metadata.OriginalSize,
		Duration:      time.Since(start),
		KeyID:         metadata.KeyID,
		Decompressed:  decompressed,
	}

	span.SetAttributes(
		attribute.Int("decrypted.size", len(decryptedData)),
		attribute.Bool("decompressed", decompressed),
		attribute.Float64("decryption.duration_ms", float64(result.Duration.Milliseconds())),
	)

	return result, nil
}

// encryptData performs the actual encryption based on algorithm
func (e *Encryptor) encryptData(algorithm EncryptionAlgorithm, key, nonce, plaintext []byte, encCtx *EncryptionContext) ([]byte, error) {
	switch algorithm {
	case AlgorithmAES256GCM:
		return e.encryptAESGCM(key, nonce, plaintext, e.buildAAD(encCtx))

	case AlgorithmAES256CBC:
		return e.encryptAESCBC(key, nonce, plaintext)

	case AlgorithmChaCha20:
		return e.encryptChaCha20(key, nonce, plaintext, e.buildAAD(encCtx))

	case AlgorithmXChaCha20:
		return e.encryptXChaCha20(key, nonce, plaintext, e.buildAAD(encCtx))

	default:
		return nil, fmt.Errorf("unsupported encryption algorithm: %s", algorithm)
	}
}

// decryptData performs the actual decryption based on algorithm
func (e *Encryptor) decryptData(algorithm EncryptionAlgorithm, key, nonce, ciphertext []byte, additionalData map[string]string) ([]byte, error) {
	switch algorithm {
	case AlgorithmAES256GCM:
		return e.decryptAESGCM(key, nonce, ciphertext, e.buildAADFromMap(additionalData))

	case AlgorithmAES256CBC:
		return e.decryptAESCBC(key, nonce, ciphertext)

	case AlgorithmChaCha20:
		return e.decryptChaCha20(key, nonce, ciphertext, e.buildAADFromMap(additionalData))

	case AlgorithmXChaCha20:
		return e.decryptXChaCha20(key, nonce, ciphertext, e.buildAADFromMap(additionalData))

	default:
		return nil, fmt.Errorf("unsupported decryption algorithm: %s", algorithm)
	}
}

// AES-GCM encryption
func (e *Encryptor) encryptAESGCM(key, nonce, plaintext, additionalData []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Ensure nonce is the correct size
	if len(nonce) != gcm.NonceSize() {
		return nil, fmt.Errorf("incorrect nonce size: got %d, want %d", len(nonce), gcm.NonceSize())
	}

	return gcm.Seal(nil, nonce, plaintext, additionalData), nil
}

// AES-GCM decryption
func (e *Encryptor) decryptAESGCM(key, nonce, ciphertext, additionalData []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	return gcm.Open(nil, nonce, ciphertext, additionalData)
}

// AES-CBC encryption (with HMAC for authentication)
func (e *Encryptor) encryptAESCBC(key, iv, plaintext []byte) ([]byte, error) {
	// Split key for encryption and HMAC
	if len(key) < 32 {
		return nil, fmt.Errorf("key too short for AES-CBC-HMAC")
	}
	encKey := key[:16]
	macKey := key[16:32]

	// Pad plaintext to block size
	plaintext = e.padPKCS7(plaintext, aes.BlockSize)

	// Create cipher
	block, err := aes.NewCipher(encKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	// Encrypt
	ciphertext := make([]byte, len(plaintext))
	mode := cipher.NewCBCEncrypter(block, iv[:aes.BlockSize])
	mode.CryptBlocks(ciphertext, plaintext)

	// Calculate HMAC
	h := hmac.New(sha256.New, macKey)
	h.Write(iv)
	h.Write(ciphertext)
	mac := h.Sum(nil)

	// Combine IV + ciphertext + MAC
	result := make([]byte, len(ciphertext)+len(mac))
	copy(result, ciphertext)
	copy(result[len(ciphertext):], mac)

	return result, nil
}

// AES-CBC decryption
func (e *Encryptor) decryptAESCBC(key, iv, ciphertext []byte) ([]byte, error) {
	// Split key for encryption and HMAC
	if len(key) < 32 {
		return nil, fmt.Errorf("key too short for AES-CBC-HMAC")
	}
	encKey := key[:16]
	macKey := key[16:32]

	// Extract MAC
	if len(ciphertext) < sha256.Size {
		return nil, fmt.Errorf("ciphertext too short")
	}
	macStart := len(ciphertext) - sha256.Size
	mac := ciphertext[macStart:]
	ciphertext = ciphertext[:macStart]

	// Verify HMAC
	h := hmac.New(sha256.New, macKey)
	h.Write(iv)
	h.Write(ciphertext)
	expectedMAC := h.Sum(nil)
	if !hmac.Equal(mac, expectedMAC) {
		return nil, fmt.Errorf("HMAC verification failed")
	}

	// Create cipher
	block, err := aes.NewCipher(encKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	// Decrypt
	plaintext := make([]byte, len(ciphertext))
	mode := cipher.NewCBCDecrypter(block, iv[:aes.BlockSize])
	mode.CryptBlocks(plaintext, ciphertext)

	// Remove padding
	plaintext, err = e.unpadPKCS7(plaintext)
	if err != nil {
		return nil, fmt.Errorf("invalid padding: %w", err)
	}

	return plaintext, nil
}

// ChaCha20-Poly1305 encryption
func (e *Encryptor) encryptChaCha20(key, nonce, plaintext, additionalData []byte) ([]byte, error) {
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create ChaCha20-Poly1305: %w", err)
	}

	return aead.Seal(nil, nonce, plaintext, additionalData), nil
}

// ChaCha20-Poly1305 decryption
func (e *Encryptor) decryptChaCha20(key, nonce, ciphertext, additionalData []byte) ([]byte, error) {
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create ChaCha20-Poly1305: %w", err)
	}

	return aead.Open(nil, nonce, ciphertext, additionalData)
}

// XChaCha20-Poly1305 encryption
func (e *Encryptor) encryptXChaCha20(key, nonce, plaintext, additionalData []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create XChaCha20-Poly1305: %w", err)
	}

	return aead.Seal(nil, nonce, plaintext, additionalData), nil
}

// XChaCha20-Poly1305 decryption
func (e *Encryptor) decryptXChaCha20(key, nonce, ciphertext, additionalData []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create XChaCha20-Poly1305: %w", err)
	}

	return aead.Open(nil, nonce, ciphertext, additionalData)
}

// Helper methods

func (e *Encryptor) getNonceSize(algorithm EncryptionAlgorithm) int {
	switch algorithm {
	case AlgorithmAES256GCM:
		return 12 // 96 bits for GCM
	case AlgorithmAES256CBC:
		return 16 // 128 bits for CBC (IV)
	case AlgorithmChaCha20:
		return 12 // 96 bits for ChaCha20
	case AlgorithmXChaCha20:
		return 24 // 192 bits for XChaCha20
	default:
		return 12
	}
}

func (e *Encryptor) buildAAD(ctx *EncryptionContext) []byte {
	// Build additional authenticated data from context
	var aad bytes.Buffer

	binary.Write(&aad, binary.BigEndian, ctx.TenantID)
	aad.WriteString(ctx.ResourceID)
	aad.WriteString(ctx.ResourceType)
	aad.WriteString(string(ctx.Purpose))

	return aad.Bytes()
}

func (e *Encryptor) buildAADFromMap(data map[string]string) []byte {
	if data == nil {
		return nil
	}

	var aad bytes.Buffer
	for k, v := range data {
		aad.WriteString(k)
		aad.WriteString(":")
		aad.WriteString(v)
		aad.WriteString(";")
	}

	return aad.Bytes()
}

func (e *Encryptor) compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)

	if _, err := w.Write(data); err != nil {
		return nil, err
	}

	if err := w.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (e *Encryptor) decompress(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()

	return io.ReadAll(r)
}

func (e *Encryptor) calculateChecksum(data []byte) string {
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum)
}

// PKCS7 padding
func (e *Encryptor) padPKCS7(data []byte, blockSize int) []byte {
	padding := blockSize - (len(data) % blockSize)
	padText := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(data, padText...)
}

func (e *Encryptor) unpadPKCS7(data []byte) ([]byte, error) {
	length := len(data)
	if length == 0 {
		return nil, fmt.Errorf("empty data")
	}

	padding := int(data[length-1])
	if padding > length || padding == 0 {
		return nil, fmt.Errorf("invalid padding")
	}

	for i := length - padding; i < length; i++ {
		if data[i] != byte(padding) {
			return nil, fmt.Errorf("invalid padding")
		}
	}

	return data[:length-padding], nil
}

// StreamEncryptor provides streaming encryption for large files
type StreamEncryptor struct {
	encryptor  *Encryptor
	key        []byte
	algorithm  EncryptionAlgorithm
	bufferSize int
}

// NewStreamEncryptor creates a new streaming encryptor
func (e *Encryptor) NewStreamEncryptor(ctx context.Context, encCtx *EncryptionContext) (*StreamEncryptor, error) {
	// Get encryption key
	key, err := e.keyManager.GetOrCreateKey(ctx, encCtx.TenantID, KeyTypeData, encCtx.Purpose)
	if err != nil {
		return nil, fmt.Errorf("failed to get encryption key: %w", err)
	}

	// Get key material
	keyMaterial, err := e.keyManager.GetDecryptionKey(ctx, key.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get key material: %w", err)
	}

	return &StreamEncryptor{
		encryptor:  e,
		key:        keyMaterial,
		algorithm:  key.Algorithm,
		bufferSize: e.config.BufferSize,
	}, nil
}

// EncryptStream encrypts data from reader to writer
func (se *StreamEncryptor) EncryptStream(ctx context.Context, reader io.Reader, writer io.Writer) (*EncryptionMetadata, error) {
	// This would implement streaming encryption
	// For now, this is a placeholder
	return nil, fmt.Errorf("streaming encryption not yet implemented")
}
