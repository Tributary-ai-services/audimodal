package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/jscharber/audimodal/internal/database"
	"github.com/jscharber/audimodal/internal/database/models"
	"github.com/jscharber/audimodal/internal/processors"
	"github.com/jscharber/audimodal/internal/server/response"
	"github.com/jscharber/audimodal/internal/services"
	"github.com/jscharber/audimodal/pkg/embeddings"
)

const (
	// MaxMultipartFileSize is the maximum file size for multipart uploads (10MB)
	MaxMultipartFileSize = 10 << 20 // 10 MB
	// MaxFormMemory is the maximum memory used for multipart form parsing (32MB)
	MaxFormMemory = 32 << 20 // 32 MB
)

// FileHandler handles file HTTP requests
type FileHandler struct {
	db                   *database.Database
	embeddingCoordinator *processors.EmbeddingCoordinator
	storageService       *services.StorageService
	pipeline             *processors.Pipeline
}

// NewFileHandler creates a new file handler
func NewFileHandler(db *database.Database, storageService *services.StorageService) *FileHandler {
	// Initialize processing pipeline
	pipelineConfig := processors.GetDefaultPipelineConfig()
	pipeline := processors.NewPipeline(db, pipelineConfig)

	// Initialize embedding coordinator with default config
	embeddingCoordinator, err := processors.NewEmbeddingCoordinator(db, nil)
	if err != nil {
		// Log error but don't fail - embedding functionality will be disabled
		// In production, you might want to handle this differently
		fmt.Printf("WARNING: Failed to initialize embedding coordinator: %v\n", err)
		fmt.Printf("Embedding functionality will be disabled. Set OPENAI_API_KEY environment variable to enable.\n")
		embeddingCoordinator = nil
	}

	return &FileHandler{
		db:                   db,
		embeddingCoordinator: embeddingCoordinator,
		storageService:       storageService,
		pipeline:             pipeline,
	}
}

// FileResponse represents a file response
type FileResponse struct {
	ID                  uuid.UUID              `json:"id"`
	TenantID            uuid.UUID              `json:"tenant_id"`
	DataSourceID        *uuid.UUID             `json:"data_source_id,omitempty"`
	ProcessingSessionID *uuid.UUID             `json:"processing_session_id,omitempty"`
	URL                 string                 `json:"url"`
	Path                string                 `json:"path"`
	Filename            string                 `json:"filename"`
	Extension           string                 `json:"extension"`
	ContentType         string                 `json:"content_type"`
	Size                int64                  `json:"size"`
	Checksum            string                 `json:"checksum"`
	ChecksumType        string                 `json:"checksum_type"`
	LastModified        string                 `json:"last_modified"`
	Status              string                 `json:"status"`
	ProcessingTier      string                 `json:"processing_tier"`
	ProcessedAt         *string                `json:"processed_at,omitempty"`
	ProcessingError     *string                `json:"processing_error,omitempty"`
	ProcessingDuration  *int64                 `json:"processing_duration,omitempty"`
	Language            string                 `json:"language,omitempty"`
	LanguageConf        float64                `json:"language_confidence,omitempty"`
	ContentCategory     string                 `json:"content_category,omitempty"`
	SensitivityLevel    string                 `json:"sensitivity_level,omitempty"`
	Classifications     []string               `json:"classifications,omitempty"`
	SchemaInfo          models.FileSchemaInfo  `json:"schema_info"`
	ChunkCount          int                    `json:"chunk_count"`
	ChunkingStrategy    string                 `json:"chunking_strategy,omitempty"`
	PIIDetected         bool                   `json:"pii_detected"`
	ComplianceFlags     []string               `json:"compliance_flags,omitempty"`
	EncryptionStatus    string                 `json:"encryption_status"`
	Metadata            map[string]interface{} `json:"metadata,omitempty"`
	CustomFields        map[string]interface{} `json:"custom_fields,omitempty"`
	CreatedAt           string                 `json:"created_at"`
	UpdatedAt           string                 `json:"updated_at"`
}

// ServeHTTP implements the http.Handler interface
func (h *FileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tenantCtx := getTenantContext(r.Context())
	if tenantCtx == nil {
		response.WriteBadRequest(w, getRequestID(r), "Tenant context required", nil)
		return
	}

	path := r.URL.Path
	parts := strings.Split(strings.Trim(path, "/"), "/")

	// Find files in the path
	var fileIndex int = -1
	for i, part := range parts {
		if part == "files" {
			fileIndex = i
			break
		}
	}

	if fileIndex == -1 {
		response.WriteNotFound(w, getRequestID(r), "File endpoint not found")
		return
	}

	// Handle different routes
	if len(parts) == fileIndex+1 {
		// /api/v1/tenants/{tenant_id}/files
		switch r.Method {
		case http.MethodGet:
			h.ListFiles(w, r, tenantCtx.TenantID)
		case http.MethodPost:
			h.CreateFile(w, r, tenantCtx.TenantID)
		default:
			response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		}
		return
	}

	// Handle /api/v1/tenants/{tenant_id}/files/search
	if len(parts) == fileIndex+2 && parts[fileIndex+1] == "search" {
		h.SearchSimilarDocuments(w, r, tenantCtx.TenantID)
		return
	}

	if len(parts) >= fileIndex+2 {
		fileID, err := uuid.Parse(parts[fileIndex+1])
		if err != nil {
			response.WriteBadRequest(w, getRequestID(r), "Invalid file ID format", nil)
			return
		}

		if len(parts) == fileIndex+2 {
			// /api/v1/tenants/{tenant_id}/files/{file_id}
			switch r.Method {
			case http.MethodGet:
				h.GetFile(w, r, tenantCtx.TenantID, fileID)
			case http.MethodPut:
				h.UpdateFile(w, r, tenantCtx.TenantID, fileID)
			case http.MethodDelete:
				h.DeleteFile(w, r, tenantCtx.TenantID, fileID)
			default:
				response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
			}
			return
		}

		// Handle sub-resources
		if len(parts) >= fileIndex+3 {
			subResource := parts[fileIndex+2]
			switch subResource {
			case "chunks":
				h.ListFileChunks(w, r, tenantCtx.TenantID, fileID)
			case "process":
				h.ProcessFile(w, r, tenantCtx.TenantID, fileID)
			case "download":
				h.DownloadFile(w, r, tenantCtx.TenantID, fileID)
			case "metadata":
				h.GetFileMetadata(w, r, tenantCtx.TenantID, fileID)
			case "violations":
				h.ListFileViolations(w, r, tenantCtx.TenantID, fileID)
			default:
				response.WriteNotFound(w, getRequestID(r), "Resource not found")
			}
			return
		}
	}

	response.WriteNotFound(w, getRequestID(r), "Endpoint not found")
}

// ListFiles handles GET /api/v1/tenants/{tenant_id}/files
func (h *FileHandler) ListFiles(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) {
	page, pageSize, offset := getPagination(r.Context())

	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	var files []models.File
	query := tenantRepo.DB().Limit(pageSize).Offset(offset).Order("created_at DESC")

	// Apply filters
	if status := r.URL.Query().Get("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	if contentType := r.URL.Query().Get("content_type"); contentType != "" {
		query = query.Where("content_type = ?", contentType)
	}

	if extension := r.URL.Query().Get("extension"); extension != "" {
		query = query.Where("extension = ?", extension)
	}

	if dataSourceID := r.URL.Query().Get("data_source_id"); dataSourceID != "" {
		if dsID, err := uuid.Parse(dataSourceID); err == nil {
			query = query.Where("data_source_id = ?", dsID)
		}
	}

	if sessionID := r.URL.Query().Get("session_id"); sessionID != "" {
		if sID, err := uuid.Parse(sessionID); err == nil {
			query = query.Where("processing_session_id = ?", sID)
		}
	}

	if piiDetected := r.URL.Query().Get("pii_detected"); piiDetected != "" {
		detected := piiDetected == "true"
		query = query.Where("pii_detected = ?", detected)
	}

	err = query.Find(&files).Error
	if err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to list files", err.Error())
		return
	}

	// Convert to response format
	responseData := make([]FileResponse, len(files))
	for i, file := range files {
		responseData[i] = h.toFileResponse(&file)
	}

	// Get total count
	var totalCount int64
	countQuery := tenantRepo.DB().Model(&models.File{})
	// Apply same filters for count
	if status := r.URL.Query().Get("status"); status != "" {
		countQuery = countQuery.Where("status = ?", status)
	}
	if contentType := r.URL.Query().Get("content_type"); contentType != "" {
		countQuery = countQuery.Where("content_type = ?", contentType)
	}
	// ... other filters
	countQuery.Count(&totalCount)

	response.WritePaginated(w, getRequestID(r), responseData, page, pageSize, totalCount)
}

// CreateFile handles POST /api/v1/tenants/{tenant_id}/files
// Supports both JSON requests (for metadata-only file records) and multipart form data (for actual file uploads)
func (h *FileHandler) CreateFile(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) {
	contentType := r.Header.Get("Content-Type")
	
	// Handle multipart form data (actual file uploads)
	if strings.HasPrefix(contentType, "multipart/form-data") {
		h.handleFileUpload(w, r, tenantID)
		return
	}
	
	// Handle JSON requests (metadata-only file records)
	h.handleJSONFileCreate(w, r, tenantID)
}

// handleFileUpload handles multipart form file uploads for files <10MB
func (h *FileHandler) handleFileUpload(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) {
	// Check content-length header for size validation before parsing
	if contentLength := r.Header.Get("Content-Length"); contentLength != "" {
		if size, err := strconv.ParseInt(contentLength, 10, 64); err == nil {
			if size > MaxMultipartFileSize {
				response.WriteBadRequest(w, getRequestID(r), fmt.Sprintf("File too large for multipart upload. Files over %d bytes should use S3 direct upload.", MaxMultipartFileSize), nil)
				return
			}
		}
	}

	// Parse multipart form with size limit
	if err := r.ParseMultipartForm(MaxFormMemory); err != nil {
		response.WriteBadRequest(w, getRequestID(r), "Failed to parse multipart form", err.Error())
		return
	}
	
	// Get the uploaded file
	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		response.WriteBadRequest(w, getRequestID(r), "No file provided", err.Error())
		return
	}
	defer file.Close()

	// Validate file size again after parsing
	if fileHeader.Size > MaxMultipartFileSize {
		response.WriteBadRequest(w, getRequestID(r), fmt.Sprintf("File too large for multipart upload (%d bytes). Files over %d bytes should use S3 direct upload.", fileHeader.Size, MaxMultipartFileSize), nil)
		return
	}
	
	// Get data source ID from form
	dataSourceIDStr := r.FormValue("datasource_id")
	if dataSourceIDStr == "" {
		response.WriteBadRequest(w, getRequestID(r), "datasource_id is required", "")
		return
	}
	
	dataSourceID, err := uuid.Parse(dataSourceIDStr)
	if err != nil {
		response.WriteBadRequest(w, getRequestID(r), "Invalid datasource_id format", err.Error())
		return
	}
	
	// Get tenant repository
	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}
	
	// Parse optional metadata if provided
	var metadata map[string]interface{}
	if metadataStr := r.FormValue("metadata"); metadataStr != "" {
		if err := json.Unmarshal([]byte(metadataStr), &metadata); err != nil {
			response.WriteBadRequest(w, getRequestID(r), "Invalid metadata JSON", err.Error())
			return
		}
	}
	
	// Extract file information
	filename := fileHeader.Filename
	extension := filepath.Ext(filename)
	if len(extension) > 0 && extension[0] == '.' {
		extension = extension[1:] // Remove leading dot
	}

	// Detect content type if not provided
	contentType := fileHeader.Header.Get("Content-Type")
	if contentType == "" {
		// Read first 512 bytes to detect content type
		buffer := make([]byte, 512)
		n, _ := file.Read(buffer)
		contentType = http.DetectContentType(buffer[:n])
		// Reset file reader
		file.Seek(0, 0)
	}

	// Store file to local storage first (for files <10MB we can handle in memory/temp storage)
	var storagePath string
	var fileURL string
	
	// Create temporary file for processing
	tempDir := os.Getenv("EAI_TEMP_DIR")
	if tempDir == "" {
		tempDir = "/tmp"
	}
	
	tempFile, err := os.CreateTemp(tempDir, fmt.Sprintf("upload_%s_*_%s", tenantID.String()[:8], filename))
	if err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to create temporary file", err.Error())
		return
	}
	tempPath := tempFile.Name()
	defer func() {
		tempFile.Close()
		os.Remove(tempPath) // Clean up temp file
	}()
	
	// Copy uploaded file to temp file
	_, err = io.Copy(tempFile, file)
	if err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to save uploaded file", err.Error())
		return
	}
	
	// TODO: For production, implement proper storage (S3, local filesystem, etc.)
	// For now, we store locally and create a file:// URL
	localStorageDir := os.Getenv("EAI_STORAGE_LOCAL_PATH")
	if localStorageDir == "" {
		localStorageDir = "/app/data/storage"
	}
	
	// Ensure storage directory exists
	if err := os.MkdirAll(filepath.Join(localStorageDir, tenantID.String()), 0755); err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to create storage directory", err.Error())
		return
	}
	
	// Generate unique filename for storage
	fileUUID := uuid.New()
	storedFilename := fmt.Sprintf("%s_%s", fileUUID.String(), filename)
	storagePath = filepath.Join(localStorageDir, tenantID.String(), storedFilename)
	fileURL = fmt.Sprintf("file://%s", storagePath)
	
	// Copy from temp to final storage location
	tempFile.Seek(0, 0)
	finalFile, err := os.Create(storagePath)
	if err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to create storage file", err.Error())
		return
	}
	defer finalFile.Close()
	
	_, err = io.Copy(finalFile, tempFile)
	if err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to store file", err.Error())
		return
	}
	
	// Create file record with storage information
	fileRecord := &models.File{
		TenantID:         tenantID,
		DataSourceID:     &dataSourceID,
		URL:              fileURL,
		Path:             storagePath,
		Filename:         filename,
		Extension:        extension,
		ContentType:      contentType,
		Size:             fileHeader.Size,
		Status:           models.FileStatusDiscovered,
		EncryptionStatus: "none",
		Metadata:         metadata,
		LastModified:     time.Now(),
	}
	
	if err := tenantRepo.ValidateAndCreate(fileRecord); err != nil {
		// Clean up stored file on database error
		os.Remove(storagePath)
		if strings.Contains(err.Error(), "validation") {
			response.WriteValidationError(w, getRequestID(r), err.Error())
			return
		}
		response.WriteInternalServerError(w, getRequestID(r), "Failed to create file record", err.Error())
		return
	}
	
	responseData := h.toFileResponse(fileRecord)
	response.WriteCreated(w, getRequestID(r), responseData)
}

// handleJSONFileCreate handles JSON requests for S3 URLs and metadata-only file records
func (h *FileHandler) handleJSONFileCreate(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) {
	var req struct {
		URL                 string                 `json:"url"`                   // S3 URL or existing file URL
		Path                string                 `json:"path,omitempty"`        // Optional: local path
		Filename            string                 `json:"filename,omitempty"`    // Optional: will be extracted from URL if not provided
		Extension           string                 `json:"extension,omitempty"`   // Optional: will be extracted if not provided
		ContentType         string                 `json:"content_type,omitempty"`// Optional: will be detected if not provided
		Size                int64                  `json:"size,omitempty"`        // Optional: will be retrieved if not provided
		Checksum            string                 `json:"checksum,omitempty"`
		ChecksumType        string                 `json:"checksum_type,omitempty"`
		DataSourceID        *uuid.UUID             `json:"data_source_id,omitempty"`
		ProcessingSessionID *uuid.UUID             `json:"processing_session_id,omitempty"`
		Metadata            map[string]interface{} `json:"metadata,omitempty"`
		ValidateAccess      bool                   `json:"validate_access,omitempty"` // Whether to validate S3 access
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.WriteBadRequest(w, getRequestID(r), "Invalid JSON request", err.Error())
		return
	}

	// URL is required
	if req.URL == "" {
		response.WriteBadRequest(w, getRequestID(r), "URL is required", nil)
		return
	}

	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	var fileRecord *models.File

	// Check if this is an S3 URL or cloud storage URL
	if h.storageService != nil && (strings.HasPrefix(req.URL, "s3://") || strings.HasPrefix(req.URL, "gs://") || strings.HasPrefix(req.URL, "https://")) {
		// Use storage service to validate and get file info
		if req.ValidateAccess {
			if err := h.storageService.ValidateDataSourceURL(r.Context(), tenantID, req.URL); err != nil {
				response.WriteBadRequest(w, getRequestID(r), "Cannot access the provided URL", err.Error())
				return
			}
		}

		// Get file info from storage service if size/metadata not provided
		if req.Size == 0 || req.Filename == "" || req.ContentType == "" {
			fileInfo, err := h.storageService.GetFileInfoFromURL(r.Context(), tenantID, req.URL)
			if err != nil {
				response.WriteInternalServerError(w, getRequestID(r), "Failed to get file information from URL", err.Error())
				return
			}

			// Use storage service data if not provided in request
			if req.Size == 0 {
				req.Size = fileInfo.Size
			}
			if req.Filename == "" {
				// Extract filename from URL or use the name from fileInfo
				if urlParts := strings.Split(req.URL, "/"); len(urlParts) > 0 {
					req.Filename = urlParts[len(urlParts)-1]
				}
				if req.Filename == "" {
					req.Filename = fileInfo.Name
				}
			}
			if req.ContentType == "" {
				req.ContentType = fileInfo.ContentType
			}
			if req.Checksum == "" {
				req.Checksum = fileInfo.Checksum
				req.ChecksumType = fileInfo.ChecksumType
			}
		}

		// Extract extension if not provided
		if req.Extension == "" && req.Filename != "" {
			if ext := filepath.Ext(req.Filename); ext != "" && ext[0] == '.' {
				req.Extension = ext[1:] // Remove leading dot
			}
		}

		// Create file record using storage service
		fileRecord, err = h.storageService.CreateFileFromURL(r.Context(), tenantID, req.URL, req.DataSourceID, req.ProcessingSessionID)
		if err != nil {
			response.WriteInternalServerError(w, getRequestID(r), "Failed to create file from URL", err.Error())
			return
		}

		// Update any metadata provided in the request
		if req.Metadata != nil {
			fileRecord.Metadata = req.Metadata
			if err := tenantRepo.DB().Save(fileRecord).Error; err != nil {
				response.WriteInternalServerError(w, getRequestID(r), "Failed to update file metadata", err.Error())
				return
			}
		}
	} else {
		// Handle local file URLs or manual metadata-only records
		fileRecord = &models.File{
			TenantID:            tenantID,
			DataSourceID:        req.DataSourceID,
			ProcessingSessionID: req.ProcessingSessionID,
			URL:                 req.URL,
			Path:                req.Path,
			Filename:            req.Filename,
			Extension:           req.Extension,
			ContentType:         req.ContentType,
			Size:                req.Size,
			Checksum:            req.Checksum,
			ChecksumType:        req.ChecksumType,
			Status:              models.FileStatusDiscovered,
			EncryptionStatus:    "none",
			Metadata:            req.Metadata,
			LastModified:        time.Now(),
		}

		if err := tenantRepo.ValidateAndCreate(fileRecord); err != nil {
			if strings.Contains(err.Error(), "validation") {
				response.WriteValidationError(w, getRequestID(r), err.Error())
				return
			}
			response.WriteInternalServerError(w, getRequestID(r), "Failed to create file", err.Error())
			return
		}
	}

	responseData := h.toFileResponse(fileRecord)
	response.WriteCreated(w, getRequestID(r), responseData)
}

// GetFile handles GET /api/v1/tenants/{tenant_id}/files/{id}
func (h *FileHandler) GetFile(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, fileID uuid.UUID) {
	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	var file models.File
	err = tenantRepo.DB().Where("id = ?", fileID).First(&file).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "File not found")
		return
	}

	responseData := h.toFileResponse(&file)
	response.WriteSuccess(w, getRequestID(r), responseData, nil)
}

// UpdateFile handles PUT /api/v1/tenants/{tenant_id}/files/{id}
func (h *FileHandler) UpdateFile(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, fileID uuid.UUID) {
	var req struct {
		Status           string                 `json:"status"`
		ContentCategory  string                 `json:"content_category"`
		SensitivityLevel string                 `json:"sensitivity_level"`
		Classifications  []string               `json:"classifications"`
		ComplianceFlags  []string               `json:"compliance_flags"`
		Metadata         map[string]interface{} `json:"metadata"`
		CustomFields     map[string]interface{} `json:"custom_fields"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.WriteBadRequest(w, getRequestID(r), "Invalid JSON request", err.Error())
		return
	}

	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	var file models.File
	err = tenantRepo.DB().Where("id = ?", fileID).First(&file).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "File not found")
		return
	}

	// Update allowed fields
	if req.Status != "" {
		file.Status = req.Status
	}
	if req.ContentCategory != "" {
		file.ContentCategory = req.ContentCategory
	}
	if req.SensitivityLevel != "" {
		file.SensitivityLevel = req.SensitivityLevel
	}
	if req.Classifications != nil {
		file.Classifications = req.Classifications
	}
	if req.ComplianceFlags != nil {
		file.ComplianceFlags = req.ComplianceFlags
	}
	if req.Metadata != nil {
		file.Metadata = req.Metadata
	}
	if req.CustomFields != nil {
		file.CustomFields = req.CustomFields
	}

	if err := tenantRepo.ValidateAndUpdate(&file); err != nil {
		if strings.Contains(err.Error(), "validation") {
			response.WriteValidationError(w, getRequestID(r), err.Error())
			return
		}
		response.WriteInternalServerError(w, getRequestID(r), "Failed to update file", err.Error())
		return
	}

	responseData := h.toFileResponse(&file)
	response.WriteSuccess(w, getRequestID(r), responseData, nil)
}

// DeleteFile handles DELETE /api/v1/tenants/{tenant_id}/files/{id}
func (h *FileHandler) DeleteFile(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, fileID uuid.UUID) {
	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	var file models.File
	err = tenantRepo.DB().Where("id = ?", fileID).First(&file).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "File not found")
		return
	}

	// Use Unscoped() to perform hard delete instead of soft delete
	if err := tenantRepo.DB().Unscoped().Delete(&file).Error; err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to delete file", err.Error())
		return
	}

	response.WriteNoContent(w)
}

// ListFileChunks handles GET /api/v1/tenants/{tenant_id}/files/{id}/chunks
func (h *FileHandler) ListFileChunks(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, fileID uuid.UUID) {
	if r.Method != http.MethodGet {
		response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		return
	}

	page, pageSize, offset := getPagination(r.Context())

	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	// Check if file exists
	var file models.File
	err = tenantRepo.DB().Where("id = ?", fileID).First(&file).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "File not found")
		return
	}

	// Get chunks for this file
	var chunks []models.Chunk
	err = tenantRepo.DB().Where("file_id = ?", fileID).
		Limit(pageSize).Offset(offset).Order("chunk_number ASC").Find(&chunks).Error
	if err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to list chunks", err.Error())
		return
	}

	// Get total count
	var totalCount int64
	tenantRepo.DB().Model(&models.Chunk{}).Where("file_id = ?", fileID).Count(&totalCount)

	response.WritePaginated(w, getRequestID(r), chunks, page, pageSize, totalCount)
}

// ProcessFile handles POST /api/v1/tenants/{tenant_id}/files/{id}/process
func (h *FileHandler) ProcessFile(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, fileID uuid.UUID) {
	if r.Method != http.MethodPost {
		response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		return
	}

	var req struct {
		ChunkingStrategy string `json:"chunking_strategy"`
		Priority         string `json:"priority"`
		DLPScanEnabled   bool   `json:"dlp_scan_enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.WriteBadRequest(w, getRequestID(r), "Invalid JSON request", err.Error())
		return
	}

	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	var file models.File
	err = tenantRepo.DB().Where("id = ?", fileID).First(&file).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "File not found")
		return
	}

	// Start processing
	file.StartProcessing()
	if req.ChunkingStrategy != "" {
		file.ChunkingStrategy = req.ChunkingStrategy
	}

	if err := tenantRepo.DB().Save(&file).Error; err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to start processing", err.Error())
		return
	}

	// Use the pipeline for processing
	if h.pipeline != nil {
		// Process file in background using the pipeline
		go func() {
			// Create processing request
			processingRequest := &processors.ProcessingRequest{
				TenantID:        tenantID,
				FileID:          fileID,
				FilePath:        file.Path,
				StrategyType:    req.ChunkingStrategy,
				Priority:        req.Priority,
				DLPScanEnabled:  req.DLPScanEnabled,
			}

			// Process the file through the pipeline (use background context to avoid cancellation)
			ctx := context.Background()
			result, err := h.pipeline.ProcessFile(ctx, processingRequest)
			
			// Update file status based on results - create a fresh tenant service and repository
			backgroundTenantService := h.db.NewTenantService()
			backgroundTenantRepo, repoErr := backgroundTenantService.GetTenantRepository(ctx, tenantID)
			if repoErr != nil {
				fmt.Printf("ERROR: Failed to get tenant repository for status update: %v\n", repoErr)
				return
			}

			var updatedFile models.File
			if dbErr := backgroundTenantRepo.DB().Where("id = ?", fileID).First(&updatedFile).Error; dbErr == nil {
				if err != nil {
					// Processing failed - mark as error
					updatedFile.MarkAsError("Failed to process file: " + err.Error())
				} else if result != nil {
					// Processing succeeded - mark as processed with chunk count
					updatedFile.MarkAsProcessed(result.ChunksCreated, req.ChunkingStrategy)
					now := time.Now()
					updatedFile.ProcessedAt = &now
				}
				backgroundTenantRepo.DB().Save(&updatedFile)
			} else {
				fmt.Printf("ERROR: Failed to reload file %s for status update: %v\n", fileID.String(), dbErr)
			}
		}()
	} else {
		// Fallback: mark as processing but warn about lack of pipeline
		fmt.Printf("WARNING: No processing pipeline available for file %s\n", fileID.String())
	}

	responseData := map[string]interface{}{
		"message":  "File processing started",
		"file_id":  fileID,
		"status":   file.Status,
		"strategy": file.ChunkingStrategy,
	}

	response.WriteSuccess(w, getRequestID(r), responseData, nil)
}

// DownloadFile handles GET /api/v1/tenants/{tenant_id}/files/{id}/download
func (h *FileHandler) DownloadFile(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, fileID uuid.UUID) {
	if r.Method != http.MethodGet {
		response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		return
	}

	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	var file models.File
	err = tenantRepo.DB().Where("id = ?", fileID).First(&file).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "File not found")
		return
	}

	// TODO: Implement actual file download
	// This would involve resolving the file URL and streaming the content
	downloadInfo := map[string]interface{}{
		"download_url": file.URL,
		"filename":     file.Filename,
		"content_type": file.ContentType,
		"size":         file.Size,
		"expires_at":   "2025-01-03T13:00:00Z", // Temporary download link
	}

	response.WriteSuccess(w, getRequestID(r), downloadInfo, nil)
}

// GetFileMetadata handles GET /api/v1/tenants/{tenant_id}/files/{id}/metadata
func (h *FileHandler) GetFileMetadata(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, fileID uuid.UUID) {
	if r.Method != http.MethodGet {
		response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		return
	}

	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	var file models.File
	err = tenantRepo.DB().Where("id = ?", fileID).First(&file).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "File not found")
		return
	}

	metadata := map[string]interface{}{
		"schema_info":       file.SchemaInfo,
		"metadata":          file.Metadata,
		"custom_fields":     file.CustomFields,
		"classifications":   file.Classifications,
		"compliance_flags":  file.ComplianceFlags,
		"processing_tier":   file.GetProcessingTier(),
		"sensitivity_level": file.GetSensitivityLevel(),
		"pii_detected":      file.HasPII(),
		"is_structured":     file.IsStructuredData(),
		"is_unstructured":   file.IsUnstructuredData(),
		"chunk_count":       file.ChunkCount,
		"chunking_strategy": file.ChunkingStrategy,
	}

	response.WriteSuccess(w, getRequestID(r), metadata, nil)
}

// ListFileViolations handles GET /api/v1/tenants/{tenant_id}/files/{id}/violations
func (h *FileHandler) ListFileViolations(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, fileID uuid.UUID) {
	if r.Method != http.MethodGet {
		response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		return
	}

	page, pageSize, offset := getPagination(r.Context())

	tenantService := h.db.NewTenantService()
	tenantRepo, err := tenantService.GetTenantRepository(r.Context(), tenantID)
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "Tenant not found")
		return
	}

	// Check if file exists
	var file models.File
	err = tenantRepo.DB().Where("id = ?", fileID).First(&file).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "File not found")
		return
	}

	// Get violations for this file
	var violations []models.DLPViolation
	err = tenantRepo.DB().Where("file_id = ?", fileID).
		Limit(pageSize).Offset(offset).Order("created_at DESC").Find(&violations).Error
	if err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to list violations", err.Error())
		return
	}

	// Get total count
	var totalCount int64
	tenantRepo.DB().Model(&models.DLPViolation{}).Where("file_id = ?", fileID).Count(&totalCount)

	response.WritePaginated(w, getRequestID(r), violations, page, pageSize, totalCount)
}

// Helper methods

func (h *FileHandler) toFileResponse(file *models.File) FileResponse {
	responseData := FileResponse{
		ID:                  file.ID,
		TenantID:            file.TenantID,
		DataSourceID:        file.DataSourceID,
		ProcessingSessionID: file.ProcessingSessionID,
		URL:                 file.URL,
		Path:                file.Path,
		Filename:            file.Filename,
		Extension:           file.Extension,
		ContentType:         file.ContentType,
		Size:                file.Size,
		Checksum:            file.Checksum,
		ChecksumType:        file.ChecksumType,
		LastModified:        file.LastModified.Format("2006-01-02T15:04:05Z"),
		Status:              file.Status,
		ProcessingTier:      file.ProcessingTier,
		ProcessingError:     file.ProcessingError,
		ProcessingDuration:  file.ProcessingDuration,
		Language:            file.Language,
		LanguageConf:        file.LanguageConf,
		ContentCategory:     file.ContentCategory,
		SensitivityLevel:    file.SensitivityLevel,
		Classifications:     file.Classifications,
		SchemaInfo:          file.SchemaInfo,
		ChunkCount:          file.ChunkCount,
		ChunkingStrategy:    file.ChunkingStrategy,
		PIIDetected:         file.PIIDetected,
		ComplianceFlags:     file.ComplianceFlags,
		EncryptionStatus:    file.EncryptionStatus,
		Metadata:            file.Metadata,
		CustomFields:        file.CustomFields,
		CreatedAt:           file.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:           file.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	if file.ProcessedAt != nil {
		processedAt := file.ProcessedAt.Format("2006-01-02T15:04:05Z")
		responseData.ProcessedAt = &processedAt
	}

	return responseData
}

// SearchSimilarDocuments handles POST /api/v1/tenants/{tenant_id}/files/search
func (h *FileHandler) SearchSimilarDocuments(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) {
	if r.Method != http.MethodPost {
		response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		return
	}

	if h.embeddingCoordinator == nil {
		response.WriteError(w, getRequestID(r), http.StatusServiceUnavailable, "EMBEDDING_SERVICE_UNAVAILABLE", "Vector search service is not available", nil)
		return
	}

	var req struct {
		Query     string                 `json:"query"`
		TopK      int                    `json:"top_k,omitempty"`
		Threshold float32                `json:"threshold,omitempty"`
		Filters   map[string]interface{} `json:"filters,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.WriteBadRequest(w, getRequestID(r), "Invalid JSON request", err.Error())
		return
	}

	if req.Query == "" {
		response.WriteBadRequest(w, getRequestID(r), "Query is required", nil)
		return
	}

	// Set default values
	if req.TopK <= 0 {
		req.TopK = 10
	}
	if req.Threshold <= 0 {
		req.Threshold = 0.7
	}

	// Prepare search options
	searchOptions := &embeddings.SearchOptions{
		TopK:            req.TopK,
		Threshold:       req.Threshold,
		MetricType:      "cosine",
		IncludeContent:  true,
		IncludeMetadata: true,
		Filters:         req.Filters,
	}

	// Perform search
	searchResults, err := h.embeddingCoordinator.SearchSimilarDocuments(r.Context(), tenantID, req.Query, searchOptions)
	if err != nil {
		h.handleSearchError(w, r, err, req.Query)
		return
	}

	response.WriteSuccess(w, getRequestID(r), searchResults, nil)
}

// handleSearchError categorizes search errors and returns appropriate HTTP responses
func (h *FileHandler) handleSearchError(w http.ResponseWriter, r *http.Request, err error, query string) {
	requestID := getRequestID(r)
	
	// Check if it's an EmbeddingError with specific types
	if embErr, ok := err.(*embeddings.EmbeddingError); ok {
		switch embErr.Type {
		case "dataset_not_found":
			// Dataset doesn't exist - return 404
			response.WriteNotFound(w, requestID, "Dataset not found: " + embErr.Message)
			return
		case "dimension_mismatch":
			// Client sent wrong dimensions - return 400
			response.WriteBadRequest(w, requestID, "Invalid query dimensions", embErr.Message)
			return
		case "search_unavailable":
			// DeepLake search functionality not available - return empty results instead of error
			// This handles the case where DeepLake dataset exists but search isn't implemented
			emptyResults := &embeddings.SearchResponse{
				Results:    []*embeddings.SimilarityResult{},
				TotalFound: 0,
				QueryTime:  0,
				HasMore:    false,
				Query:      query,
				Dataset:    "default",
			}
			response.WriteSuccess(w, requestID, emptyResults, nil)
			return
		case "search_error":
			// General search errors that might be recoverable
			if strings.Contains(strings.ToLower(embErr.Message), "no results") || 
			   strings.Contains(strings.ToLower(embErr.Message), "empty") {
				// Return empty results for "no results" scenarios
				emptyResults := &embeddings.SearchResponse{
					Results:    []*embeddings.SimilarityResult{},
					TotalFound: 0,
					QueryTime:  0,
					HasMore:    false,
					Query:      query,
					Dataset:    "default",
				}
				response.WriteSuccess(w, requestID, emptyResults, nil)
				return
			}
			// Other search errors are server errors
			response.WriteInternalServerError(w, requestID, "Search operation failed", embErr.Message)
			return
		case "invalid_input":
			// Bad input parameters - return 400
			response.WriteBadRequest(w, requestID, "Invalid search parameters", embErr.Message)
			return
		default:
			// Unknown embedding error - return 500
			response.WriteInternalServerError(w, requestID, "Search service error", embErr.Message)
			return
		}
	}
	
	// For non-EmbeddingError types, check the error message for common patterns
	errMsg := strings.ToLower(err.Error())
	if strings.Contains(errMsg, "not found") {
		response.WriteNotFound(w, requestID, "Resource not found: " + err.Error())
	} else if strings.Contains(errMsg, "invalid") || strings.Contains(errMsg, "bad request") {
		response.WriteBadRequest(w, requestID, "Invalid request", err.Error())
	} else {
		// Default to internal server error
		response.WriteInternalServerError(w, requestID, "Failed to search documents", err.Error())
	}
}
