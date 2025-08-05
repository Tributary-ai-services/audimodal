package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/jscharber/eAIIngest/internal/database"
	"github.com/jscharber/eAIIngest/internal/server/response"
	"github.com/jscharber/eAIIngest/internal/services"
	"github.com/jscharber/eAIIngest/pkg/storage"
)

// StorageHandler handles cloud storage related HTTP requests
type StorageHandler struct {
	db             *database.Database
	storageService *services.StorageService
}

// NewStorageHandler creates a new storage handler
func NewStorageHandler(db *database.Database, storageService *services.StorageService) *StorageHandler {
	return &StorageHandler{
		db:             db,
		storageService: storageService,
	}
}

// ValidateURLRequest represents a URL validation request
type ValidateURLRequest struct {
	URL string `json:"url"`
}

// ValidateURLResponse represents a URL validation response
type ValidateURLResponse struct {
	Valid    bool                  `json:"valid"`
	Provider storage.CloudProvider `json:"provider,omitempty"`
	Error    string                `json:"error,omitempty"`
}

// FileInfoResponse represents file information response
type FileInfoResponse struct {
	URL          string            `json:"url"`
	Size         int64             `json:"size"`
	ContentType  string            `json:"content_type"`
	LastModified string            `json:"last_modified"`
	ETag         string            `json:"etag"`
	Checksum     string            `json:"checksum"`
	ChecksumType string            `json:"checksum_type"`
	Metadata     map[string]string `json:"metadata"`
	Tags         map[string]string `json:"tags"`
	StorageClass string            `json:"storage_class"`
	Encrypted    bool              `json:"encrypted"`
	KMSKeyID     string            `json:"kms_key_id,omitempty"`
}

// ListFilesRequest represents a request to list files
type ListFilesRequest struct {
	URL               string `json:"url"`
	Prefix            string `json:"prefix,omitempty"`
	Delimiter         string `json:"delimiter,omitempty"`
	MaxKeys           int    `json:"max_keys,omitempty"`
	ContinuationToken string `json:"continuation_token,omitempty"`
	Recursive         bool   `json:"recursive,omitempty"`
	IncludeMetadata   bool   `json:"include_metadata,omitempty"`
}

// ListFilesResponse represents the response from listing files
type ListFilesResponse struct {
	Files             []*FileInfoResponse `json:"files"`
	CommonPrefixes    []string            `json:"common_prefixes,omitempty"`
	IsTruncated       bool                `json:"is_truncated"`
	ContinuationToken string              `json:"continuation_token,omitempty"`
	KeyCount          int                 `json:"key_count"`
}

// PresignedURLRequest represents a presigned URL request
type PresignedURLRequest struct {
	URL        string `json:"url"`
	Method     string `json:"method"`
	Expiration int    `json:"expiration"` // seconds
}

// PresignedURLResponse represents a presigned URL response
type PresignedURLResponse struct {
	URL       string            `json:"url"`
	Method    string            `json:"method"`
	Headers   map[string]string `json:"headers,omitempty"`
	ExpiresAt string            `json:"expires_at"`
}

// ServeHTTP implements the http.Handler interface
func (h *StorageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tenantCtx := getTenantContext(r.Context())
	if tenantCtx == nil {
		response.WriteBadRequest(w, getRequestID(r), "Tenant context required", nil)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/v1/tenants/"+tenantCtx.TenantID.String()+"/storage")
	parts := strings.Split(strings.Trim(path, "/"), "/")

	if len(parts) == 0 || parts[0] == "" {
		response.WriteNotFound(w, getRequestID(r), "Storage endpoint not found")
		return
	}

	switch parts[0] {
	case "validate":
		h.handleValidateURL(w, r, tenantCtx.TenantID)
	case "info":
		h.handleGetFileInfo(w, r, tenantCtx.TenantID)
	case "list":
		h.handleListFiles(w, r, tenantCtx.TenantID)
	case "presigned":
		h.handlePresignedURL(w, r, tenantCtx.TenantID)
	case "providers":
		h.handleGetProviders(w, r)
	default:
		response.WriteNotFound(w, getRequestID(r), "Storage endpoint not found")
	}
}

// handleValidateURL validates a cloud storage URL
func (h *StorageHandler) handleValidateURL(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) {
	if r.Method != http.MethodPost {
		response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		return
	}

	var req ValidateURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.WriteBadRequest(w, getRequestID(r), "Invalid JSON request", err.Error())
		return
	}

	if req.URL == "" {
		response.WriteBadRequest(w, getRequestID(r), "URL is required", nil)
		return
	}

	// Parse URL to get provider
	storageURL, err := h.storageService.ParseURL(req.URL)
	var provider storage.CloudProvider
	if err == nil {
		provider = storageURL.Provider
	}

	// Validate access
	err = h.storageService.ValidateDataSourceURL(r.Context(), tenantID, req.URL)

	resp := ValidateURLResponse{
		Valid:    err == nil,
		Provider: provider,
	}

	if err != nil {
		resp.Error = err.Error()
	}

	response.WriteSuccess(w, getRequestID(r), resp, nil)
}

// handleGetFileInfo retrieves file information from a URL
func (h *StorageHandler) handleGetFileInfo(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) {
	if r.Method != http.MethodGet {
		response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		return
	}

	url := r.URL.Query().Get("url")
	if url == "" {
		response.WriteBadRequest(w, getRequestID(r), "URL parameter is required", nil)
		return
	}

	fileInfo, err := h.storageService.GetFileInfoFromURL(r.Context(), tenantID, url)
	if err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to get file info", err.Error())
		return
	}

	resp := &FileInfoResponse{
		URL:          fileInfo.URL,
		Size:         fileInfo.Size,
		ContentType:  fileInfo.ContentType,
		LastModified: fileInfo.LastModified.Format(time.RFC3339),
		ETag:         fileInfo.ETag,
		Checksum:     fileInfo.Checksum,
		ChecksumType: fileInfo.ChecksumType,
		Metadata:     fileInfo.Metadata,
		Tags:         fileInfo.Tags,
		StorageClass: fileInfo.StorageClass,
		Encrypted:    fileInfo.Encrypted,
		KMSKeyID:     fileInfo.KMSKeyID,
	}

	response.WriteSuccess(w, getRequestID(r), resp, nil)
}

// handleListFiles lists files from a cloud storage URL
func (h *StorageHandler) handleListFiles(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) {
	if r.Method != http.MethodPost {
		response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		return
	}

	var req ListFilesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.WriteBadRequest(w, getRequestID(r), "Invalid JSON request", err.Error())
		return
	}

	if req.URL == "" {
		response.WriteBadRequest(w, getRequestID(r), "URL is required", nil)
		return
	}

	// Convert to storage options
	options := &storage.ListOptions{
		Prefix:            req.Prefix,
		Delimiter:         req.Delimiter,
		MaxKeys:           req.MaxKeys,
		ContinuationToken: req.ContinuationToken,
		Recursive:         req.Recursive,
		IncludeMetadata:   req.IncludeMetadata,
	}

	listResult, err := h.storageService.DiscoverFilesFromURL(r.Context(), tenantID, req.URL, options)
	if err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to list files", err.Error())
		return
	}

	// Convert file info
	files := make([]*FileInfoResponse, len(listResult.Files))
	for i, fileInfo := range listResult.Files {
		files[i] = &FileInfoResponse{
			URL:          fileInfo.URL,
			Size:         fileInfo.Size,
			ContentType:  fileInfo.ContentType,
			LastModified: fileInfo.LastModified.Format(time.RFC3339),
			ETag:         fileInfo.ETag,
			Checksum:     fileInfo.Checksum,
			ChecksumType: fileInfo.ChecksumType,
			Metadata:     fileInfo.Metadata,
			Tags:         fileInfo.Tags,
			StorageClass: fileInfo.StorageClass,
			Encrypted:    fileInfo.Encrypted,
			KMSKeyID:     fileInfo.KMSKeyID,
		}
	}

	resp := &ListFilesResponse{
		Files:             files,
		CommonPrefixes:    listResult.CommonPrefixes,
		IsTruncated:       listResult.IsTruncated,
		ContinuationToken: listResult.ContinuationToken,
		KeyCount:          listResult.KeyCount,
	}

	response.WriteSuccess(w, getRequestID(r), resp, nil)
}

// handlePresignedURL generates a presigned URL
func (h *StorageHandler) handlePresignedURL(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) {
	if r.Method != http.MethodPost {
		response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		return
	}

	var req PresignedURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.WriteBadRequest(w, getRequestID(r), "Invalid JSON request", err.Error())
		return
	}

	if req.URL == "" {
		response.WriteBadRequest(w, getRequestID(r), "URL is required", nil)
		return
	}

	if req.Method == "" {
		req.Method = "GET"
	}

	expiration := time.Duration(req.Expiration) * time.Second
	if expiration == 0 {
		expiration = 1 * time.Hour // Default expiration
	}

	presignedURL, err := h.storageService.GeneratePresignedURL(r.Context(), tenantID, req.URL, req.Method, expiration)
	if err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to generate presigned URL", err.Error())
		return
	}

	resp := &PresignedURLResponse{
		URL:       presignedURL.URL,
		Method:    presignedURL.Method,
		Headers:   presignedURL.Headers,
		ExpiresAt: presignedURL.ExpiresAt.Format(time.RFC3339),
	}

	response.WriteSuccess(w, getRequestID(r), resp, nil)
}

// handleGetProviders returns supported cloud providers
func (h *StorageHandler) handleGetProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		return
	}

	providers := h.storageService.GetSupportedProviders()

	providersInfo := make([]map[string]interface{}, len(providers))
	for i, provider := range providers {
		providersInfo[i] = map[string]interface{}{
			"name":         provider,
			"display_name": getProviderDisplayName(provider),
			"supported":    true,
		}
	}

	resp := map[string]interface{}{
		"providers": providersInfo,
		"count":     len(providers),
	}

	response.WriteSuccess(w, getRequestID(r), resp, nil)
}

// Helper methods

func getProviderDisplayName(provider storage.CloudProvider) string {
	switch provider {
	case storage.ProviderAWS:
		return "Amazon Web Services (S3)"
	case storage.ProviderGCP:
		return "Google Cloud Platform (Cloud Storage)"
	case storage.ProviderAzure:
		return "Microsoft Azure (Blob Storage)"
	case storage.ProviderLocal:
		return "Local File System"
	default:
		return string(provider)
	}
}

// CredentialHandler handles cloud storage credential management
type CredentialHandler struct {
	storageService *services.StorageService
}

// NewCredentialHandler creates a new credential handler
func NewCredentialHandler(storageService *services.StorageService) *CredentialHandler {
	return &CredentialHandler{
		storageService: storageService,
	}
}

// CredentialRequest represents a credential storage request
type CredentialRequest struct {
	Provider        storage.CloudProvider `json:"provider"`
	AccessKeyID     string                `json:"access_key_id,omitempty"`
	SecretAccessKey string                `json:"secret_access_key,omitempty"`
	SessionToken    string                `json:"session_token,omitempty"`
	Region          string                `json:"region,omitempty"`
	Project         string                `json:"project,omitempty"`
	ServiceAccount  string                `json:"service_account,omitempty"`
	KeyFile         string                `json:"key_file,omitempty"`
	ExpiresAt       *time.Time            `json:"expires_at,omitempty"`
}

// CredentialResponse represents credential information without sensitive data
type CredentialResponse struct {
	ID        uuid.UUID             `json:"id"`
	Provider  storage.CloudProvider `json:"provider"`
	Name      string                `json:"name"`
	Region    string                `json:"region,omitempty"`
	Project   string                `json:"project,omitempty"`
	Status    string                `json:"status"`
	ExpiresAt *time.Time            `json:"expires_at,omitempty"`
	CreatedAt time.Time             `json:"created_at"`
	UpdatedAt time.Time             `json:"updated_at"`
}

// ServeHTTP implements the http.Handler interface for credential management
func (h *CredentialHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tenantCtx := getTenantContext(r.Context())
	if tenantCtx == nil {
		response.WriteBadRequest(w, getRequestID(r), "Tenant context required", nil)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/v1/tenants/"+tenantCtx.TenantID.String()+"/credentials")
	parts := strings.Split(strings.Trim(path, "/"), "/")

	if len(parts) == 0 || parts[0] == "" {
		// Handle /api/v1/tenants/{tenant_id}/credentials
		switch r.Method {
		case http.MethodGet:
			h.listCredentials(w, r, tenantCtx.TenantID)
		case http.MethodPost:
			h.storeCredentials(w, r, tenantCtx.TenantID)
		default:
			response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		}
		return
	}

	// Handle /api/v1/tenants/{tenant_id}/credentials/{provider}
	provider := storage.CloudProvider(parts[0])
	switch r.Method {
	case http.MethodDelete:
		h.deleteCredentials(w, r, tenantCtx.TenantID, provider)
	default:
		response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
	}
}

func (h *CredentialHandler) storeCredentials(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) {
	var req CredentialRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.WriteBadRequest(w, getRequestID(r), "Invalid JSON request", err.Error())
		return
	}

	credentials := &storage.Credentials{
		Provider:        req.Provider,
		AccessKeyID:     req.AccessKeyID,
		SecretAccessKey: req.SecretAccessKey,
		SessionToken:    req.SessionToken,
		Region:          req.Region,
		Project:         req.Project,
		ServiceAccount:  req.ServiceAccount,
		KeyFile:         req.KeyFile,
		ExpiresAt:       req.ExpiresAt,
	}
	_ = credentials // TODO: implement credentials storage

	// This would need to be implemented in the storage service
	// if err := h.storageService.StoreCredentials(r.Context(), tenantID, credentials); err != nil {
	// 	response.WriteInternalServerError(w, getRequestID(r), "Failed to store credentials", err.Error())
	// 	return
	// }

	resp := map[string]interface{}{
		"message":  "Credentials stored successfully",
		"provider": req.Provider,
	}

	response.WriteCreated(w, getRequestID(r), resp)
}

func (h *CredentialHandler) listCredentials(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) {
	// This would need to be implemented in the storage service
	// credentials, err := h.storageService.ListCredentials(r.Context(), tenantID)
	// if err != nil {
	// 	response.WriteInternalServerError(w, getRequestID(r), "Failed to list credentials", err.Error())
	// 	return
	// }

	// Placeholder response
	resp := []CredentialResponse{}

	response.WriteSuccess(w, getRequestID(r), resp, nil)
}

func (h *CredentialHandler) deleteCredentials(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, provider storage.CloudProvider) {
	// This would need to be implemented in the storage service
	// if err := h.storageService.DeleteCredentials(r.Context(), tenantID, provider); err != nil {
	// 	response.WriteInternalServerError(w, getRequestID(r), "Failed to delete credentials", err.Error())
	// 	return
	// }

	response.WriteNoContent(w)
}
