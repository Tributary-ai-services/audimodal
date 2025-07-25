package credentials

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/jscharber/eAIIngest/internal/database"
	"github.com/jscharber/eAIIngest/pkg/storage"
)

// DatabaseCredentialProvider implements CredentialProvider using database storage
type DatabaseCredentialProvider struct {
	db            *database.Database
	encryptionKey []byte
}

// NewDatabaseCredentialProvider creates a new database-backed credential provider
func NewDatabaseCredentialProvider(db *database.Database, encryptionKey []byte) *DatabaseCredentialProvider {
	return &DatabaseCredentialProvider{
		db:            db,
		encryptionKey: encryptionKey,
	}
}

// GetCredentials returns the credentials for the specified tenant and provider
func (p *DatabaseCredentialProvider) GetCredentials(ctx context.Context, tenantID uuid.UUID, provider storage.CloudProvider) (*storage.Credentials, error) {
	tenantService := p.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(ctx, tenantID)
	if err != nil {
		return nil, storage.NewStorageError(
			storage.ErrorCodeInvalidCredentials,
			"failed to get tenant repository",
			provider,
			"",
			err,
		)
	}

	var credRecord CloudCredential
	err = tenantRepo.DB().Where("provider = ? AND status = 'active'", string(provider)).
		First(&credRecord).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, storage.NewStorageError(
				storage.ErrorCodeInvalidCredentials,
				fmt.Sprintf("no active credentials found for provider %s", provider),
				provider,
				"",
				nil,
			)
		}
		return nil, storage.NewStorageError(
			storage.ErrorCodeInternalError,
			"failed to retrieve credentials",
			provider,
			"",
			err,
		)
	}

	// Decrypt credentials
	credentials, err := p.decryptCredentials(&credRecord)
	if err != nil {
		return nil, storage.NewStorageError(
			storage.ErrorCodeInternalError,
			"failed to decrypt credentials",
			provider,
			"",
			err,
		)
	}

	// Check if credentials are expired
	if credentials.ExpiresAt != nil && time.Now().After(*credentials.ExpiresAt) {
		// Try to refresh if possible
		if refreshed, err := p.RefreshCredentials(ctx, credentials); err == nil {
			return refreshed, nil
		}

		return nil, storage.NewStorageError(
			storage.ErrorCodeInvalidCredentials,
			"credentials have expired",
			provider,
			"",
			nil,
		)
	}

	return credentials, nil
}

// ValidateCredentials checks if credentials are valid
func (p *DatabaseCredentialProvider) ValidateCredentials(ctx context.Context, credentials *storage.Credentials) error {
	// Basic validation
	if credentials == nil {
		return storage.NewStorageError(
			storage.ErrorCodeInvalidCredentials,
			"credentials cannot be nil",
			credentials.Provider,
			"",
			nil,
		)
	}

	switch credentials.Provider {
	case storage.ProviderAWS:
		if credentials.AccessKeyID == "" || credentials.SecretAccessKey == "" {
			return storage.NewStorageError(
				storage.ErrorCodeInvalidCredentials,
				"AWS credentials require access key ID and secret access key",
				credentials.Provider,
				"",
				nil,
			)
		}

	case storage.ProviderGCP:
		if credentials.KeyFile == "" && credentials.ServiceAccount == "" {
			return storage.NewStorageError(
				storage.ErrorCodeInvalidCredentials,
				"GCP credentials require either service account key file or service account email",
				credentials.Provider,
				"",
				nil,
			)
		}

	case storage.ProviderAzure:
		if credentials.AccessKeyID == "" || credentials.SecretAccessKey == "" {
			return storage.NewStorageError(
				storage.ErrorCodeInvalidCredentials,
				"Azure credentials require storage account name and access key",
				credentials.Provider,
				"",
				nil,
			)
		}

	default:
		return storage.NewStorageError(
			storage.ErrorCodeUnsupportedProvider,
			fmt.Sprintf("unsupported provider: %s", credentials.Provider),
			credentials.Provider,
			"",
			nil,
		)
	}

	return nil
}

// RefreshCredentials refreshes temporary credentials if needed
func (p *DatabaseCredentialProvider) RefreshCredentials(ctx context.Context, credentials *storage.Credentials) (*storage.Credentials, error) {
	// For now, we don't implement automatic refresh
	// This would typically involve calling the cloud provider's STS service
	// to refresh temporary credentials

	return nil, storage.NewStorageError(
		storage.ErrorCodeInternalError,
		"credential refresh not implemented",
		credentials.Provider,
		"",
		nil,
	)
}

// StoreCredentials stores encrypted credentials in the database
func (p *DatabaseCredentialProvider) StoreCredentials(ctx context.Context, tenantID uuid.UUID, credentials *storage.Credentials) error {
	if err := p.ValidateCredentials(ctx, credentials); err != nil {
		return err
	}

	tenantService := p.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(ctx, tenantID)
	if err != nil {
		return storage.NewStorageError(
			storage.ErrorCodeInternalError,
			"failed to get tenant repository",
			credentials.Provider,
			"",
			err,
		)
	}

	// Encrypt sensitive data
	encryptedData, err := p.encryptCredentials(credentials)
	if err != nil {
		return storage.NewStorageError(
			storage.ErrorCodeInternalError,
			"failed to encrypt credentials",
			credentials.Provider,
			"",
			err,
		)
	}

	// Deactivate existing credentials for this provider
	err = tenantRepo.DB().Model(&CloudCredential{}).
		Where("provider = ?", string(credentials.Provider)).
		Update("status", "inactive").Error
	if err != nil {
		return storage.NewStorageError(
			storage.ErrorCodeInternalError,
			"failed to deactivate existing credentials",
			credentials.Provider,
			"",
			err,
		)
	}

	// Create new credential record
	credRecord := &CloudCredential{
		TenantID:       tenantID,
		Provider:       string(credentials.Provider),
		Name:           fmt.Sprintf("%s-credentials", credentials.Provider),
		EncryptedData:  encryptedData,
		Region:         credentials.Region,
		Project:        credentials.Project,
		Status:         "active",
		ExpiresAt:      credentials.ExpiresAt,
	}

	if err := tenantRepo.ValidateAndCreate(credRecord); err != nil {
		return storage.NewStorageError(
			storage.ErrorCodeInternalError,
			"failed to store credentials",
			credentials.Provider,
			"",
			err,
		)
	}

	return nil
}

// DeleteCredentials removes credentials for a provider
func (p *DatabaseCredentialProvider) DeleteCredentials(ctx context.Context, tenantID uuid.UUID, provider storage.CloudProvider) error {
	tenantService := p.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(ctx, tenantID)
	if err != nil {
		return storage.NewStorageError(
			storage.ErrorCodeInternalError,
			"failed to get tenant repository",
			provider,
			"",
			err,
		)
	}

	err = tenantRepo.DB().Where("provider = ?", string(provider)).
		Delete(&CloudCredential{}).Error

	if err != nil {
		return storage.NewStorageError(
			storage.ErrorCodeInternalError,
			"failed to delete credentials",
			provider,
			"",
			err,
		)
	}

	return nil
}

// ListCredentials returns all stored credentials for a tenant
func (p *DatabaseCredentialProvider) ListCredentials(ctx context.Context, tenantID uuid.UUID) ([]CredentialInfo, error) {
	tenantService := p.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(ctx, tenantID)
	if err != nil {
		return nil, storage.NewStorageError(
			storage.ErrorCodeInternalError,
			"failed to get tenant repository",
			"",
			"",
			err,
		)
	}

	var credRecords []CloudCredential
	err = tenantRepo.DB().Find(&credRecords).Error
	if err != nil {
		return nil, storage.NewStorageError(
			storage.ErrorCodeInternalError,
			"failed to list credentials",
			"",
			"",
			err,
		)
	}

	credInfos := make([]CredentialInfo, len(credRecords))
	for i, record := range credRecords {
		credInfos[i] = CredentialInfo{
			ID:        record.ID,
			Provider:  storage.CloudProvider(record.Provider),
			Name:      record.Name,
			Region:    record.Region,
			Project:   record.Project,
			Status:    record.Status,
			ExpiresAt: record.ExpiresAt,
			CreatedAt: record.CreatedAt,
			UpdatedAt: record.UpdatedAt,
		}
	}

	return credInfos, nil
}

// Helper methods for encryption/decryption

func (p *DatabaseCredentialProvider) encryptCredentials(credentials *storage.Credentials) (string, error) {
	// Serialize credentials to JSON
	data, err := json.Marshal(credentials)
	if err != nil {
		return "", err
	}

	// Create cipher
	block, err := aes.NewCipher(p.encryptionKey)
	if err != nil {
		return "", err
	}

	// Generate random nonce
	nonce := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	// Create GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	// Encrypt data
	ciphertext := gcm.Seal(nonce, nonce, data, nil)

	// Encode to base64
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (p *DatabaseCredentialProvider) decryptCredentials(credRecord *CloudCredential) (*storage.Credentials, error) {
	// Decode from base64
	ciphertext, err := base64.StdEncoding.DecodeString(credRecord.EncryptedData)
	if err != nil {
		return nil, err
	}

	// Create cipher
	block, err := aes.NewCipher(p.encryptionKey)
	if err != nil {
		return nil, err
	}

	// Create GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// Extract nonce
	nonce := ciphertext[:12]
	ciphertext = ciphertext[12:]

	// Decrypt data
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	// Deserialize credentials
	var credentials storage.Credentials
	if err := json.Unmarshal(plaintext, &credentials); err != nil {
		return nil, err
	}

	return &credentials, nil
}

// CloudCredential represents stored cloud credentials in the database
type CloudCredential struct {
	ID            uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID      uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Provider      string     `gorm:"not null;index" json:"provider"`
	Name          string     `gorm:"not null" json:"name"`
	EncryptedData string     `gorm:"type:text;not null" json:"-"`
	Region        string     `json:"region,omitempty"`
	Project       string     `json:"project,omitempty"`
	Status        string     `gorm:"not null;default:'active'" json:"status"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	CreatedAt     time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt     time.Time  `gorm:"not null" json:"updated_at"`
	DeletedAt     *time.Time `gorm:"index" json:"deleted_at,omitempty"`
}

// CredentialInfo represents credential metadata without sensitive data
type CredentialInfo struct {
	ID        uuid.UUID               `json:"id"`
	Provider  storage.CloudProvider   `json:"provider"`
	Name      string                  `json:"name"`
	Region    string                  `json:"region,omitempty"`
	Project   string                  `json:"project,omitempty"`
	Status    string                  `json:"status"`
	ExpiresAt *time.Time              `json:"expires_at,omitempty"`
	CreatedAt time.Time               `json:"created_at"`
	UpdatedAt time.Time               `json:"updated_at"`
}