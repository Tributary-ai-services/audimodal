package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/jscharber/audimodal/internal/database"
	"github.com/jscharber/audimodal/internal/database/models"
	"github.com/jscharber/audimodal/internal/processors"
	"github.com/jscharber/audimodal/internal/server/response"
	"github.com/jscharber/audimodal/internal/services"
	"github.com/jscharber/audimodal/pkg/dlp/types"
	"github.com/jscharber/audimodal/pkg/embeddings"
	"github.com/jscharber/audimodal/pkg/events"
)

var s3UploaderInstance *services.S3Uploader

func init() {
	var err error
	s3UploaderInstance, err = services.NewS3Uploader()
	if err != nil {
		fmt.Printf("WARNING: Failed to initialize S3 uploader: %v\n", err)
	}
}

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
	mlAnalysisService    *services.MLAnalysisService
	eventProducer        *events.SimpleProducer
	activityPublisher    *events.ActivityPublisher
	splitter             *processors.Splitter
}

// NewFileHandler creates a new file handler. activityPublisher is optional —
// pass nil to disable tas.activity.documents streaming without breaking uploads.
func NewFileHandler(db *database.Database, storageService *services.StorageService, eventProducer *events.SimpleProducer, activityPublisher *events.ActivityPublisher) *FileHandler {
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

	// Initialize ML analysis service
	mlAnalysisConfig := services.DefaultMLAnalysisServiceConfig()
	mlAnalysisService := services.NewMLAnalysisService(db, nil, mlAnalysisConfig)

	// Initialize Splitter for Kafka pipeline (requires S3 + Kafka producer)
	var splitter *processors.Splitter
	if s3UploaderInstance != nil && eventProducer != nil {
		splitter = processors.NewSplitter(db.DB(), s3UploaderInstance, eventProducer)
	}

	return &FileHandler{
		db:                   db,
		embeddingCoordinator: embeddingCoordinator,
		storageService:       storageService,
		pipeline:             pipeline,
		mlAnalysisService:    mlAnalysisService,
		eventProducer:        eventProducer,
		activityPublisher:    activityPublisher,
		splitter:             splitter,
	}
}

// FileResponse represents a file response
type FileResponse struct {
	ID                  uuid.UUID              `json:"id"`
	TenantID            uuid.UUID              `json:"tenant_id"`
	DataSourceID        *uuid.UUID             `json:"data_source_id,omitempty"`
	ProcessingSessionID *uuid.UUID             `json:"processing_session_id,omitempty"`
	Neo4jDocumentID     *string                `json:"document_id,omitempty"` // Neo4j Document.id for cross-service consistency
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
			case "processing-status":
				h.GetProcessingStatus(w, r, tenantCtx.TenantID, fileID)
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

	// Get optional document_id for cross-service consistency (Neo4j Document.id from Aether-BE)
	var neo4jDocumentID *string
	if documentID := r.FormValue("document_id"); documentID != "" {
		neo4jDocumentID = &documentID
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

	// Upload file to S3 (MinIO) in tenant-specific bucket
	fileUUID := uuid.New()
	tenantShortID := services.GetTenantShortID(tenantID.String())
	bucket := services.GetTenantBucket(tenantShortID)
	s3Key := services.GetFileKey(fileUUID.String(), filename)

	if s3UploaderInstance != nil {
		tempFile.Seek(0, 0)
		if err := s3UploaderInstance.UploadFile(r.Context(), bucket, s3Key, tempFile, fileHeader.Size); err != nil {
			response.WriteInternalServerError(w, getRequestID(r), "Failed to upload file to S3", err.Error())
			return
		}
		storagePath = s3Key
		fileURL = fmt.Sprintf("s3://%s/%s", bucket, s3Key)
	} else {
		// Fallback to local storage if S3 is not available
		localStorageDir := os.Getenv("EAI_STORAGE_LOCAL_PATH")
		if localStorageDir == "" {
			localStorageDir = "/app/data/storage"
		}
		if err := os.MkdirAll(filepath.Join(localStorageDir, tenantID.String()), 0755); err != nil {
			response.WriteInternalServerError(w, getRequestID(r), "Failed to create storage directory", err.Error())
			return
		}
		storedFilename := fmt.Sprintf("%s_%s", fileUUID.String(), filename)
		storagePath = filepath.Join(localStorageDir, tenantID.String(), storedFilename)
		fileURL = fmt.Sprintf("file://%s", storagePath)
		tempFile.Seek(0, 0)
		finalFile, err := os.Create(storagePath)
		if err != nil {
			response.WriteInternalServerError(w, getRequestID(r), "Failed to create storage file", err.Error())
			return
		}
		defer finalFile.Close()
		if _, err = io.Copy(finalFile, tempFile); err != nil {
			response.WriteInternalServerError(w, getRequestID(r), "Failed to store file", err.Error())
			return
		}
	}

	// Create file record with storage information
	fileRecord := &models.File{
		TenantID:         tenantID,
		DataSourceID:     &dataSourceID,
		Neo4jDocumentID:  neo4jDocumentID,
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

	// Publish activity event so the Live Streams page sees the upload in real time.
	// Fire-and-forget — failure to publish must not fail the upload.
	if h.activityPublisher != nil {
		ceTenantID := canonicalTenantID(r, tenantID.String())
		go h.activityPublisher.PublishDocumentUploaded(r.Context(), ceTenantID, "", getRequestID(r), events.DocumentUploadedPayload{
			FileID:    fileRecord.ID.String(),
			FileName:  filename,
			SizeBytes: fileHeader.Size,
			MimeType:  contentType,
			Source:    "upload",
		})
	}

	responseData := h.toFileResponse(fileRecord)
	response.WriteCreated(w, getRequestID(r), responseData)
}

// handleJSONFileCreate handles JSON requests for S3 URLs and metadata-only file records
func (h *FileHandler) handleJSONFileCreate(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID) {
	var req struct {
		URL                 string                 `json:"url"`                    // S3 URL or existing file URL
		Path                string                 `json:"path,omitempty"`         // Optional: local path
		Filename            string                 `json:"filename,omitempty"`     // Optional: will be extracted from URL if not provided
		Extension           string                 `json:"extension,omitempty"`    // Optional: will be extracted if not provided
		ContentType         string                 `json:"content_type,omitempty"` // Optional: will be detected if not provided
		Size                int64                  `json:"size,omitempty"`         // Optional: will be retrieved if not provided
		Checksum            string                 `json:"checksum,omitempty"`
		ChecksumType        string                 `json:"checksum_type,omitempty"`
		DataSourceID        *uuid.UUID             `json:"data_source_id,omitempty"`
		ProcessingSessionID *uuid.UUID             `json:"processing_session_id,omitempty"`
		DocumentID          string                 `json:"document_id,omitempty"` // Neo4j Document.id from Aether-BE for cross-service consistency
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

		// If all metadata was provided in the request, create the record directly
		// without calling CreateFileFromURL (which requires cloud_credentials for S3 access).
		// This is the common path when aether-be provides size, filename, and content_type.
		if req.Size > 0 && req.Filename != "" && req.ContentType != "" {
			var neo4jDocID *string
			if req.DocumentID != "" {
				neo4jDocID = &req.DocumentID
			}
			fileRecord = &models.File{
				TenantID:            tenantID,
				DataSourceID:        req.DataSourceID,
				ProcessingSessionID: req.ProcessingSessionID,
				Neo4jDocumentID:     neo4jDocID,
				URL:                 req.URL,
				Path:                req.URL,
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
		} else {
			// Fall back to storage service for credential-based access
			fileRecord, err = h.storageService.CreateFileFromURL(r.Context(), tenantID, req.URL, req.DataSourceID, req.ProcessingSessionID)
			if err != nil {
				response.WriteInternalServerError(w, getRequestID(r), "Failed to create file from URL", err.Error())
				return
			}
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
		var neo4jDocID *string
		if req.DocumentID != "" {
			neo4jDocID = &req.DocumentID
		}

		fileRecord = &models.File{
			TenantID:            tenantID,
			DataSourceID:        req.DataSourceID,
			ProcessingSessionID: req.ProcessingSessionID,
			Neo4jDocumentID:     neo4jDocID,
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

	// Publish activity event for JSON-based file registration too, so URL/S3
	// uploads surface on the Live Streams page alongside multipart uploads.
	if h.activityPublisher != nil {
		ceTenantID := canonicalTenantID(r, tenantID.String())
		go h.activityPublisher.PublishDocumentUploaded(r.Context(), ceTenantID, "", getRequestID(r), events.DocumentUploadedPayload{
			FileID:    fileRecord.ID.String(),
			FileName:  fileRecord.Filename,
			SizeBytes: fileRecord.Size,
			MimeType:  fileRecord.ContentType,
			Source:    "json",
		})
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

	// Delete dependent records before deleting the file
	// 1. Delete pipeline_page_results (FK → processing_jobs)
	tenantRepo.DB().Exec("DELETE FROM pipeline_page_results WHERE job_id IN (SELECT id FROM processing_jobs WHERE file_id = ?)", fileID)
	// 2. Delete processing_jobs (FK → files)
	tenantRepo.DB().Where("file_id = ?", fileID).Delete(&models.ProcessingJob{})
	// 3. Delete chunks (FK → files)
	tenantRepo.DB().Where("file_id = ?", fileID).Delete(&models.Chunk{})

	// Use Unscoped() to perform hard delete instead of soft delete
	if err := tenantRepo.DB().Unscoped().Delete(&file).Error; err != nil {
		response.WriteInternalServerError(w, getRequestID(r), "Failed to delete file", err.Error())
		return
	}

	response.WriteNoContent(w)
}

// chunkWithContentResponse is the response struct that includes full content from S3
type chunkWithContentResponse struct {
	ID             uuid.UUID            `json:"id"`
	TenantID       uuid.UUID            `json:"tenant_id"`
	FileID         uuid.UUID            `json:"file_id"`
	ChunkID        string               `json:"chunk_id"`
	ChunkType      string               `json:"chunk_type"`
	ChunkNumber    int                  `json:"chunk_number"`
	Content        string               `json:"content"`
	ContentPreview string               `json:"content_preview,omitempty"`
	ContentHash    string               `json:"content_hash"`
	SizeBytes      int64                `json:"size_bytes"`
	PageNumber     *int                 `json:"page_number,omitempty"`
	LineNumber     *int                 `json:"line_number,omitempty"`
	ProcessedAt    string               `json:"processed_at"`
	ProcessedBy    string               `json:"processed_by"`
	Language       string               `json:"language,omitempty"`
	Metadata       models.ChunkMetadata `json:"metadata,omitempty"`
	CreatedAt      string               `json:"created_at"`
	UpdatedAt      string               `json:"updated_at"`
}

// ListFileChunks handles GET /api/v1/tenants/{tenant_id}/files/{id}/chunks
func (h *FileHandler) ListFileChunks(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, fileID uuid.UUID) {
	if r.Method != http.MethodGet {
		response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		return
	}

	page, pageSize, offset := getPagination(r.Context())

	// Check include_content param (default: true for backward compat)
	includeContent := r.URL.Query().Get("include_content") != "false"

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

	// Build response with content hydrated from S3
	responseData := make([]chunkWithContentResponse, len(chunks))

	if includeContent && s3UploaderInstance != nil && len(chunks) > 0 {
		// Hydrate content from S3 concurrently with bounded parallelism
		const maxConcurrent = 10
		sem := make(chan struct{}, maxConcurrent)
		var wg sync.WaitGroup
		contents := make([]string, len(chunks))

		for i, chunk := range chunks {
			wg.Add(1)
			sem <- struct{}{}
			go func(idx int, c models.Chunk) {
				defer wg.Done()
				defer func() { <-sem }()

				if c.S3Bucket != "" && c.S3Key != "" {
					content, err := s3UploaderInstance.GetChunkContent(r.Context(), c.S3Bucket, c.S3Key)
					if err != nil {
						log.Printf("[ListFileChunks] Failed to read S3 content for chunk %s: %v", c.ID, err)
						// Fall back to content_preview
						contents[idx] = c.ContentPreview
					} else {
						contents[idx] = content
					}
				} else {
					contents[idx] = c.ContentPreview
				}
			}(i, chunk)
		}
		wg.Wait()

		for i, chunk := range chunks {
			responseData[i] = chunkWithContentResponse{
				ID:             chunk.ID,
				TenantID:       chunk.TenantID,
				FileID:         chunk.FileID,
				ChunkID:        chunk.ChunkID,
				ChunkType:      chunk.ChunkType,
				ChunkNumber:    chunk.ChunkNumber,
				Content:        contents[i],
				ContentPreview: chunk.ContentPreview,
				ContentHash:    chunk.ContentHash,
				SizeBytes:      chunk.SizeBytes,
				PageNumber:     chunk.PageNumber,
				LineNumber:     chunk.LineNumber,
				ProcessedAt:    chunk.ProcessedAt.Format("2006-01-02T15:04:05Z"),
				ProcessedBy:    chunk.ProcessedBy,
				Language:       chunk.Language,
				Metadata:       chunk.Metadata,
				CreatedAt:      chunk.CreatedAt.Format("2006-01-02T15:04:05Z"),
				UpdatedAt:      chunk.UpdatedAt.Format("2006-01-02T15:04:05Z"),
			}
		}
	} else {
		// No content hydration — return content_preview as content for compat
		for i, chunk := range chunks {
			responseData[i] = chunkWithContentResponse{
				ID:             chunk.ID,
				TenantID:       chunk.TenantID,
				FileID:         chunk.FileID,
				ChunkID:        chunk.ChunkID,
				ChunkType:      chunk.ChunkType,
				ChunkNumber:    chunk.ChunkNumber,
				Content:        chunk.ContentPreview,
				ContentPreview: chunk.ContentPreview,
				ContentHash:    chunk.ContentHash,
				SizeBytes:      chunk.SizeBytes,
				PageNumber:     chunk.PageNumber,
				LineNumber:     chunk.LineNumber,
				ProcessedAt:    chunk.ProcessedAt.Format("2006-01-02T15:04:05Z"),
				ProcessedBy:    chunk.ProcessedBy,
				Language:       chunk.Language,
				Metadata:       chunk.Metadata,
				CreatedAt:      chunk.CreatedAt.Format("2006-01-02T15:04:05Z"),
				UpdatedAt:      chunk.UpdatedAt.Format("2006-01-02T15:04:05Z"),
			}
		}
	}

	response.WritePaginated(w, getRequestID(r), responseData, page, pageSize, totalCount)
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
		RedactionMode    string `json:"redaction_mode"` // mask, replace, hash, remove, tokenize, none
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

	// Capture start time for processing duration calculation
	processingStartTime := time.Now()

	// Primary path: Use Kafka pipeline via Splitter for S3-stored PDF files only.
	// The Splitter uses pdfinfo/pdftotext and only works with PDFs; non-PDF files
	// (txt, docx, etc.) fall through to the embedding coordinator path below.
	isPDF := strings.EqualFold(file.Extension, "pdf") || strings.EqualFold(file.ContentType, "application/pdf")
	if h.splitter != nil && strings.HasPrefix(file.URL, "s3://") && isPDF {
		go func() {
			ctx := context.Background()

			// Parse S3 bucket and key from URL: s3://bucket/key
			s3URL := strings.TrimPrefix(file.URL, "s3://")
			slashIdx := strings.Index(s3URL, "/")
			if slashIdx < 0 {
				fmt.Printf("ERROR: Invalid S3 URL for file %s: %s\n", fileID.String(), file.URL)
				return
			}
			s3Bucket := s3URL[:slashIdx]
			s3Key := s3URL[slashIdx+1:]

			// Extract tenant short ID from bucket name (upload-{shortID})
			tenantShortID := strings.TrimPrefix(s3Bucket, "upload-")

			jobID, err := h.splitter.SplitFile(ctx, tenantID, fileID, s3Bucket, s3Key, tenantShortID, req.RedactionMode, req.DLPScanEnabled)
			if err != nil {
				fmt.Printf("ERROR: Splitter failed for file %s: %v\n", fileID.String(), err)

				// Update file status to error
				backgroundTenantService := h.db.NewTenantService()
				backgroundTenantRepo, repoErr := backgroundTenantService.GetTenantRepository(ctx, tenantID)
				if repoErr == nil {
					var updatedFile models.File
					if backgroundTenantRepo.DB().Where("id = ?", fileID).First(&updatedFile).Error == nil {
						updatedFile.MarkAsError("Splitter failed: " + err.Error())
						updatedFile.CalculateProcessingDuration(processingStartTime)
						backgroundTenantRepo.DB().Save(&updatedFile)

						// Publish processing failed event to Kafka so aether-be updates Neo4j
						if h.eventProducer != nil {
							var documentID string
							if updatedFile.Neo4jDocumentID != nil {
								documentID = *updatedFile.Neo4jDocumentID
							}
							eventData := events.ProcessingCompleteData{
								FileID:              fileID.String(),
								DocumentID:          documentID,
								URL:                 updatedFile.URL,
								TotalProcessingTime: time.Since(processingStartTime),
								Success:             false,
							}
							event := events.NewProcessingCompleteEvent("audimodal", tenantID.String(), eventData)
							if pubErr := h.eventProducer.PublishEvent(ctx, event); pubErr != nil {
								fmt.Printf("WARNING: Failed to publish splitter error event for file %s: %v\n", fileID.String(), pubErr)
							} else {
								fmt.Printf("INFO: Published processing failed event for file %s to Kafka\n", fileID.String())
							}
						}

						// Activity CloudEvent for the Live Streams panel + TimescaleDB.
						// Splitter never got to the assembler, so this is the only
						// place a document.failed event for this file can originate.
						if h.activityPublisher != nil {
							ceTenantID := canonicalTenantID(r, tenantID.String())
							go h.activityPublisher.PublishDocumentFailed(r.Context(), ceTenantID, "", getRequestID(r),
								events.DocumentFailedPayload{
									FileID:   fileID.String(),
									FileName: updatedFile.Filename,
									Stage:    "split",
									Error:    err.Error(),
								})
						}
					}
				}
				return
			}

			fmt.Printf("INFO: File %s submitted to Kafka pipeline, job_id=%s\n", fileID.String(), jobID.String())
		}()

		responseData := map[string]any{
			"message":  "File processing started (Kafka pipeline)",
			"file_id":  fileID,
			"status":   file.Status,
			"strategy": file.ChunkingStrategy,
		}
		response.WriteSuccess(w, getRequestID(r), responseData, nil)
		return
	}

	// Fallback: Use the embedding coordinator for processing (includes chunk creation + embedding generation)
	// This path is used for non-S3 files or when Splitter is not available
	if h.embeddingCoordinator != nil {
		// Process file in background using the embedding coordinator (includes embeddings)
		go func() {
			// Process the file through the embedding coordinator (use background context to avoid cancellation)
			ctx := context.Background()

			// Create processing options
			options := map[string]any{
				"strategy_type":    req.ChunkingStrategy,
				"priority":         req.Priority,
				"dlp_scan_enabled": req.DLPScanEnabled,
				"redaction_mode":   req.RedactionMode, // mask, replace, hash, remove, tokenize, none
				"dataset":          "documents",       // Use shared documents dataset
			}

			tierResult, err := h.embeddingCoordinator.ProcessSingleFileWithEmbeddings(ctx, tenantID, fileID, options)

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
					updatedFile.CalculateProcessingDuration(processingStartTime)
				} else if tierResult != nil && tierResult.ProcessingResult != nil {
					// Processing succeeded - mark as processed with chunk count
					updatedFile.MarkAsProcessed(tierResult.ChunksCreated, req.ChunkingStrategy)
					now := time.Now()
					updatedFile.ProcessedAt = &now
					updatedFile.CalculateProcessingDuration(processingStartTime)

					// Log embedding results
					fmt.Printf("INFO: File %s processed with %d chunks and %d embeddings created.\n",
						fileID.String(), tierResult.ChunksCreated, tierResult.EmbeddingsCreated)

					// Trigger automatic ML analysis if chunks were created and service is available
					if tierResult.ChunksCreated > 0 && h.mlAnalysisService != nil {
						go func() {
							analysisCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
							defer cancel()

							analysisResult, analysisErr := h.mlAnalysisService.AnalyzeDocument(analysisCtx, tenantID, fileID)
							if analysisErr != nil {
								fmt.Printf("WARNING: ML analysis failed for file %s: %v\n", fileID.String(), analysisErr)
							} else {
								fmt.Printf("INFO: ML analysis completed for file %s: %d items processed\n", fileID.String(), analysisResult.Completed)
							}
						}()
					} else if tierResult.ChunksCreated == 0 {
						fmt.Printf("WARNING: File %s processed but no chunks created. ML analysis skipped.\n", fileID.String())
					}
				}
				backgroundTenantRepo.DB().Save(&updatedFile)

				// Publish processing complete/failed event to Kafka for cross-service sync
				if h.eventProducer != nil {
					var eventData events.ProcessingCompleteData

					// Get the Neo4j Document ID for cross-service consistency
					var documentID string
					if updatedFile.Neo4jDocumentID != nil {
						documentID = *updatedFile.Neo4jDocumentID
					}

					if err == nil && tierResult != nil && tierResult.ProcessingResult != nil {
						// Success case - now includes actual embedding count
						eventData = events.ProcessingCompleteData{
							FileID:              fileID.String(), // AudiModal file UUID for cross-service lookup
							DocumentID:          documentID,      // Neo4j Document.id for cross-service consistency
							URL:                 updatedFile.URL,
							TotalProcessingTime: tierResult.ProcessingTime,
							ChunksCreated:       tierResult.ChunksCreated,
							EmbeddingsCreated:   tierResult.EmbeddingsCreated, // Actual embedding count from tier processor
							DLPViolationsFound:  0,
							FinalDataClass:      "internal",
							StorageLocation:     updatedFile.Path,
							Success:             true,
						}
					} else {
						// Failure case - still publish event so downstream services know processing failed
						var processingTime time.Duration
						if tierResult != nil && tierResult.ProcessingResult != nil {
							processingTime = tierResult.ProcessingTime
						}
						eventData = events.ProcessingCompleteData{
							FileID:              fileID.String(),
							DocumentID:          documentID, // Neo4j Document.id for cross-service consistency
							URL:                 updatedFile.URL,
							TotalProcessingTime: processingTime,
							ChunksCreated:       0,
							EmbeddingsCreated:   0,
							DLPViolationsFound:  0,
							FinalDataClass:      "",
							StorageLocation:     updatedFile.Path,
							Success:             false, // Mark as failed
						}
						fmt.Printf("INFO: Publishing processing FAILED event for file %s\n", fileID.String())
					}
					event := events.NewProcessingCompleteEvent("audimodal", tenantID.String(), eventData)
					if pubErr := h.eventProducer.PublishEvent(ctx, event); pubErr != nil {
						fmt.Printf("WARNING: Failed to publish processing event for file %s: %v\n", fileID.String(), pubErr)
					} else {
						fmt.Printf("INFO: Published processing %s event for file %s to Kafka (embeddings: %d)\n",
							map[bool]string{true: "complete", false: "failed"}[eventData.Success], fileID.String(), eventData.EmbeddingsCreated)
					}
				}

				// Activity CloudEvent for the Live Streams panel + TimescaleDB
				// events table. Separate from the legacy event above, which is
				// consumed by aether-be's processing_event_handler.
				if h.activityPublisher != nil {
					ceTenantID := canonicalTenantID(r, tenantID.String())
					durationMS := time.Since(processingStartTime).Milliseconds()
					if err == nil && tierResult != nil && tierResult.ProcessingResult != nil {
						go h.activityPublisher.PublishDocumentProcessed(r.Context(), ceTenantID, "", getRequestID(r),
							events.DocumentProcessedPayload{
								FileID:     fileID.String(),
								FileName:   updatedFile.Filename,
								ChunkCount: tierResult.ChunksCreated,
								DurationMS: durationMS,
								Confidence: tierResult.ProcessingResult.QualityScore,
							})
					} else {
						errMsg := "processing returned no result"
						if err != nil {
							errMsg = err.Error()
						}
						go h.activityPublisher.PublishDocumentFailed(r.Context(), ceTenantID, "", getRequestID(r),
							events.DocumentFailedPayload{
								FileID:   fileID.String(),
								FileName: updatedFile.Filename,
								Stage:    "process",
								Error:    errMsg,
							})
					}
				}
			} else {
				fmt.Printf("ERROR: Failed to reload file %s for status update: %v\n", fileID.String(), dbErr)
			}
		}()
	} else if h.pipeline != nil {
		// Fallback to basic pipeline (no embeddings) if embedding coordinator is not available
		fmt.Printf("WARNING: Embedding coordinator not available for file %s, using basic pipeline (no embeddings will be created)\n", fileID.String())
		go func() {
			// Create processing request
			processingRequest := &processors.ProcessingRequest{
				TenantID:       tenantID,
				FileID:         fileID,
				FilePath:       file.Path,
				StrategyType:   req.ChunkingStrategy,
				Priority:       req.Priority,
				DLPScanEnabled: req.DLPScanEnabled,
				RedactionMode:  types.RedactionStrategy(req.RedactionMode),
			}

			// Process the file through the basic pipeline (use background context to avoid cancellation)
			ctx := context.Background()
			result, err := h.pipeline.ProcessFile(ctx, processingRequest)

			// Update file status based on results
			backgroundTenantService := h.db.NewTenantService()
			backgroundTenantRepo, repoErr := backgroundTenantService.GetTenantRepository(ctx, tenantID)
			if repoErr != nil {
				fmt.Printf("ERROR: Failed to get tenant repository for status update: %v\n", repoErr)
				return
			}

			var updatedFile models.File
			if dbErr := backgroundTenantRepo.DB().Where("id = ?", fileID).First(&updatedFile).Error; dbErr == nil {
				if err != nil {
					updatedFile.MarkAsError("Failed to process file: " + err.Error())
					updatedFile.CalculateProcessingDuration(processingStartTime)
				} else if result != nil {
					updatedFile.MarkAsProcessed(result.ChunksCreated, req.ChunkingStrategy)
					now := time.Now()
					updatedFile.ProcessedAt = &now
					updatedFile.CalculateProcessingDuration(processingStartTime)
				}
				backgroundTenantRepo.DB().Save(&updatedFile)

				// Activity CloudEvent for the Live Streams panel + TimescaleDB.
				// The basic pipeline doesn't publish a legacy event either, so this
				// is the only signal downstream consumers get for this path.
				if h.activityPublisher != nil {
					ceTenantID := canonicalTenantID(r, tenantID.String())
					durationMS := time.Since(processingStartTime).Milliseconds()
					if err == nil && result != nil {
						go h.activityPublisher.PublishDocumentProcessed(r.Context(), ceTenantID, "", getRequestID(r),
							events.DocumentProcessedPayload{
								FileID:     fileID.String(),
								FileName:   updatedFile.Filename,
								ChunkCount: result.ChunksCreated,
								DurationMS: durationMS,
								Confidence: result.QualityScore,
							})
					} else {
						errMsg := "processing returned no result"
						if err != nil {
							errMsg = err.Error()
						}
						go h.activityPublisher.PublishDocumentFailed(r.Context(), ceTenantID, "", getRequestID(r),
							events.DocumentFailedPayload{
								FileID:   fileID.String(),
								FileName: updatedFile.Filename,
								Stage:    "process",
								Error:    errMsg,
							})
					}
				}
			}
		}()
	} else {
		// No processing pipeline available at all
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

// GetProcessingStatus handles GET /api/v1/tenants/{tenant_id}/files/{id}/processing-status
func (h *FileHandler) GetProcessingStatus(w http.ResponseWriter, r *http.Request, tenantID uuid.UUID, fileID uuid.UUID) {
	if r.Method != http.MethodGet {
		response.WriteError(w, getRequestID(r), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		return
	}

	// Query the latest processing job for this file
	var job models.ProcessingJob
	err := h.db.DB().Where("file_id = ? AND tenant_id = ?", fileID, tenantID).
		Order("created_at DESC").First(&job).Error
	if err != nil {
		response.WriteNotFound(w, getRequestID(r), "No processing job found for this file")
		return
	}

	responseData := map[string]any{
		"job_id":          job.ID,
		"file_id":         job.FileID,
		"status":          job.Status,
		"total_pages":     job.TotalPages,
		"completed_pages": job.CompletedPages,
		"failed_pages":    job.FailedPages,
		"created_at":      job.CreatedAt,
		"updated_at":      job.UpdatedAt,
	}
	if job.CompletedAt != nil {
		responseData["completed_at"] = job.CompletedAt
	}
	if job.ErrorMessage != nil {
		responseData["error_message"] = *job.ErrorMessage
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
		Neo4jDocumentID:     file.Neo4jDocumentID,
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
			response.WriteNotFound(w, requestID, "Dataset not found: "+embErr.Message)
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
		response.WriteNotFound(w, requestID, "Resource not found: "+err.Error())
	} else if strings.Contains(errMsg, "invalid") || strings.Contains(errMsg, "bad request") {
		response.WriteBadRequest(w, requestID, "Invalid request", err.Error())
	} else {
		// Default to internal server error
		response.WriteInternalServerError(w, requestID, "Failed to search documents", err.Error())
	}
}
