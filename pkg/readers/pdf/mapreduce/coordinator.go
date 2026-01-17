package mapreduce

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/jscharber/audimodal/internal/database/models"
)

// DefaultCoordinator orchestrates the map-reduce PDF processing
type DefaultCoordinator struct {
	config     *CoordinatorConfig
	classifier *DefaultPageClassifier
	pool       WorkerPool
	db         *gorm.DB
}

// NewCoordinator creates a new map-reduce coordinator
func NewCoordinator(config *CoordinatorConfig, db *gorm.DB, workerPath string) *DefaultCoordinator {
	if config == nil {
		config = DefaultCoordinatorConfig()
	}

	classifier := NewPageClassifier(config.Extraction)

	var pool WorkerPool
	if workerPath != "" {
		pool = NewWorkerPool(config, workerPath)
	} else {
		// Fall back to inline processing if no worker binary
		pool = NewInlineWorkerPool(config)
	}

	return &DefaultCoordinator{
		config:     config,
		classifier: classifier,
		pool:       pool,
		db:         db,
	}
}

// Process processes a PDF using the map-reduce pipeline
func (c *DefaultCoordinator) Process(ctx context.Context, pdfPath string, tenantID, fileID uuid.UUID) (*ProcessingResult, error) {
	startTime := time.Now()

	// Get page count
	totalPages, err := GetPageCount(ctx, pdfPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get page count: %w", err)
	}

	log.Printf("[MapReduce] Starting processing of %s with %d pages", pdfPath, totalPages)

	result := &ProcessingResult{
		FileID:      fileID,
		TotalPages:  totalPages,
		PageResults: make([]*PageResult, totalPages),
	}

	// Phase 1: Classify all pages
	log.Printf("[MapReduce] Phase 1: Classifying %d pages", totalPages)
	classifications, err := c.classifier.ClassifyAllPages(ctx, pdfPath, totalPages)
	if err != nil {
		return nil, fmt.Errorf("classification failed: %w", err)
	}

	// Store classification results and count by type
	for _, cl := range classifications {
		if cl == nil {
			continue
		}
		switch cl.Classification {
		case ClassTextOnly:
			result.TextOnlyPages++
		case ClassScanned, ClassImageOnly:
			result.OCRPages++
		case ClassEmpty:
			result.SkippedPages++
		}
	}

	log.Printf("[MapReduce] Classification complete: %d text-only, %d OCR, %d skipped",
		result.TextOnlyPages, result.OCRPages, result.SkippedPages)

	// Phase 2: Create page results in database (for checkpoint/recovery)
	if c.db != nil {
		if err := c.initializePageResults(ctx, tenantID, fileID, totalPages, classifications); err != nil {
			log.Printf("[MapReduce] Warning: failed to initialize page results: %v", err)
		}
	}

	// Phase 3: Extract content from all pages
	log.Printf("[MapReduce] Phase 2: Extracting content from %d pages", totalPages)

	var mu sync.Mutex
	var wg sync.WaitGroup
	errors := make([]ProcessingError, 0)

	// Process pages with retries
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		wg.Add(1)

		go func(pn int) {
			defer wg.Done()

			classification := ClassUnknown
			if pn-1 < len(classifications) && classifications[pn-1] != nil {
				classification = classifications[pn-1].Classification
			}

			// Create job
			job := &PageJob{
				JobID:          uuid.New().String(),
				TenantID:       tenantID,
				FileID:         fileID,
				PDFPath:        pdfPath,
				PageNumber:     pn,
				TotalPages:     totalPages,
				Classification: classification,
				Config:         c.config.Extraction,
			}

			// Process with retries
			var pageResult *PageResult
			var processErr error

			for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
				if attempt > 0 {
					time.Sleep(time.Duration(c.config.RetryDelay) * time.Second)
					log.Printf("[MapReduce] Retrying page %d (attempt %d/%d)", pn, attempt, c.config.MaxRetries)
				}

				pageResult, processErr = c.pool.ProcessPage(ctx, job)
				if processErr == nil && pageResult != nil && pageResult.Status == StatusCompleted {
					break
				}
			}

			mu.Lock()
			defer mu.Unlock()

			if processErr != nil || pageResult == nil || pageResult.Status != StatusCompleted {
				errMsg := "unknown error"
				if processErr != nil {
					errMsg = processErr.Error()
				} else if pageResult != nil && pageResult.Error != "" {
					errMsg = pageResult.Error
				}

				errors = append(errors, ProcessingError{
					PageNumber: pn,
					Error:      errMsg,
					Retryable:  true,
					RetryCount: c.config.MaxRetries,
				})
				result.FailedPages++

				// Create failed result
				if pageResult == nil {
					pageResult = &PageResult{
						PageNumber:     pn,
						TotalPages:     totalPages,
						Classification: classification,
						Status:         StatusFailed,
						Error:          errMsg,
						ProcessedAt:    time.Now(),
					}
				}
			} else {
				result.TotalCharacters += len(pageResult.Content)
			}

			result.PageResults[pn-1] = pageResult

			// Save to database
			if c.db != nil {
				c.savePageResultToDB(ctx, tenantID, fileID, pageResult)
			}

			// Log progress periodically
			if pn%c.config.CheckpointInterval == 0 {
				log.Printf("[MapReduce] Progress: %d/%d pages processed", pn, totalPages)
			}
		}(pageNum)
	}

	wg.Wait()

	result.Errors = errors
	result.TotalDuration = int(time.Since(startTime).Milliseconds())

	log.Printf("[MapReduce] Processing complete: %d pages in %dms (%d failed)",
		totalPages, result.TotalDuration, result.FailedPages)

	return result, nil
}

// Resume resumes processing from a checkpoint
func (c *DefaultCoordinator) Resume(ctx context.Context, fileID uuid.UUID) (*ProcessingResult, error) {
	if c.db == nil {
		return nil, fmt.Errorf("database required for resume")
	}

	// Get existing page results
	var pageResults []models.PageResult
	if err := c.db.Where("file_id = ?", fileID).Order("page_number ASC").Find(&pageResults).Error; err != nil {
		return nil, fmt.Errorf("failed to get page results: %w", err)
	}

	if len(pageResults) == 0 {
		return nil, fmt.Errorf("no page results found for file %s", fileID)
	}

	// Get file info for PDF path
	var file models.File
	if err := c.db.First(&file, "id = ?", fileID).Error; err != nil {
		return nil, fmt.Errorf("failed to get file: %w", err)
	}

	// Find pages that need processing
	var pendingPages []int
	var failedRetryable []int
	totalPages := pageResults[0].TotalPages

	for _, pr := range pageResults {
		if pr.ProcessingStatus == models.PageStatusPending {
			pendingPages = append(pendingPages, pr.PageNumber)
		} else if pr.ProcessingStatus == models.PageStatusFailed && pr.CanRetry() {
			failedRetryable = append(failedRetryable, pr.PageNumber)
		}
	}

	if len(pendingPages) == 0 && len(failedRetryable) == 0 {
		log.Printf("[MapReduce] Resume: all pages already processed for file %s", fileID)
		return c.buildResultFromDB(ctx, fileID, totalPages)
	}

	log.Printf("[MapReduce] Resume: %d pending, %d retryable pages for file %s",
		len(pendingPages), len(failedRetryable), fileID)

	// Process remaining pages
	allPending := append(pendingPages, failedRetryable...)

	var mu sync.Mutex
	var wg sync.WaitGroup
	errors := make([]ProcessingError, 0)
	results := make(map[int]*PageResult)

	for _, pageNum := range allPending {
		wg.Add(1)

		go func(pn int) {
			defer wg.Done()

			// Get classification from existing page result
			var classification PageClassificationType = ClassUnknown
			for _, pr := range pageResults {
				if pr.PageNumber == pn {
					classification = PageClassificationType(pr.Classification)
					break
				}
			}

			job := &PageJob{
				JobID:          uuid.New().String(),
				TenantID:       file.TenantID,
				FileID:         fileID,
				PDFPath:        file.Path,
				PageNumber:     pn,
				TotalPages:     totalPages,
				Classification: classification,
				Config:         c.config.Extraction,
			}

			pageResult, err := c.pool.ProcessPage(ctx, job)

			mu.Lock()
			defer mu.Unlock()

			if err != nil || pageResult == nil || pageResult.Status != StatusCompleted {
				errMsg := "unknown error"
				if err != nil {
					errMsg = err.Error()
				} else if pageResult != nil && pageResult.Error != "" {
					errMsg = pageResult.Error
				}

				errors = append(errors, ProcessingError{
					PageNumber: pn,
					Error:      errMsg,
					Retryable:  true,
				})
			}

			if pageResult != nil {
				results[pn] = pageResult
				c.savePageResultToDB(ctx, file.TenantID, fileID, pageResult)
			}
		}(pageNum)
	}

	wg.Wait()

	return c.buildResultFromDB(ctx, fileID, totalPages)
}

// initializePageResults creates page result records for all pages
func (c *DefaultCoordinator) initializePageResults(ctx context.Context, tenantID, fileID uuid.UUID, totalPages int, classifications []*PageClassification) error {
	for i := 1; i <= totalPages; i++ {
		pr := models.NewPageResult(tenantID, fileID, i, totalPages)

		// Set classification if available
		if i-1 < len(classifications) && classifications[i-1] != nil {
			cl := classifications[i-1]
			pr.SetClassification(
				string(cl.Classification),
				cl.Confidence,
				cl.HasEmbeddedFonts,
				cl.HasImages,
				cl.ImageCount,
				cl.LargeImageCount,
				cl.TextCharCount,
			)
		}

		// Use upsert to avoid duplicates
		if err := c.db.Where("file_id = ? AND page_number = ?", fileID, i).
			Assign(pr).
			FirstOrCreate(pr).Error; err != nil {
			log.Printf("[MapReduce] Warning: failed to create page result %d: %v", i, err)
		}
	}

	return nil
}

// savePageResultToDB saves a page result to the database
func (c *DefaultCoordinator) savePageResultToDB(ctx context.Context, tenantID, fileID uuid.UUID, result *PageResult) {
	if c.db == nil || result == nil {
		return
	}

	// Convert to database model
	dbResult := &models.PageResult{
		TenantID:   tenantID,
		FileID:     fileID,
		PageNumber: result.PageNumber,
		TotalPages: result.TotalPages,

		Classification:           string(result.Classification),
		ClassificationConfidence: &result.ClassificationConfidence,

		Content:          result.Content,
		ContentHash:      result.ContentHash,
		ExtractionMethod: string(result.ExtractionMethod),
		OCRConfidence:    &result.OCRConfidence,
		OCRLanguage:      result.OCRLanguage,
		OCRDPI:           result.OCRDPI,

		ProcessingStatus: string(result.Status),
		WorkerID:         result.WorkerID,
	}

	if result.Error != "" {
		dbResult.ProcessingError = &result.Error
	}

	if result.ProcessingDurationMs > 0 {
		duration := int(result.ProcessingDurationMs)
		dbResult.ProcessingDurationMs = &duration
	}

	now := time.Now()
	dbResult.ProcessingCompletedAt = &now

	if result.PeakMemoryBytes > 0 {
		dbResult.PeakMemoryBytes = result.PeakMemoryBytes
	}

	// Upsert
	if err := c.db.Where("file_id = ? AND page_number = ?", fileID, result.PageNumber).
		Assign(dbResult).
		FirstOrCreate(dbResult).Error; err != nil {
		log.Printf("[MapReduce] Warning: failed to save page result %d: %v", result.PageNumber, err)
	}
}

// buildResultFromDB builds a ProcessingResult from database records
func (c *DefaultCoordinator) buildResultFromDB(ctx context.Context, fileID uuid.UUID, totalPages int) (*ProcessingResult, error) {
	var pageResults []models.PageResult
	if err := c.db.Where("file_id = ?", fileID).Order("page_number ASC").Find(&pageResults).Error; err != nil {
		return nil, fmt.Errorf("failed to get page results: %w", err)
	}

	result := &ProcessingResult{
		FileID:      fileID,
		TotalPages:  totalPages,
		PageResults: make([]*PageResult, totalPages),
	}

	for _, pr := range pageResults {
		if pr.PageNumber < 1 || pr.PageNumber > totalPages {
			continue
		}

		// Convert from database model
		pageResult := &PageResult{
			TenantID:         pr.TenantID,
			FileID:           pr.FileID,
			PageNumber:       pr.PageNumber,
			TotalPages:       pr.TotalPages,
			Classification:   PageClassificationType(pr.Classification),
			Content:          pr.Content,
			ContentHash:      pr.ContentHash,
			ExtractionMethod: ExtractionMethod(pr.ExtractionMethod),
			Status:           ProcessingStatus(pr.ProcessingStatus),
			WorkerID:         pr.WorkerID,
		}

		if pr.ClassificationConfidence != nil {
			pageResult.ClassificationConfidence = *pr.ClassificationConfidence
		}
		if pr.OCRConfidence != nil {
			pageResult.OCRConfidence = *pr.OCRConfidence
		}
		pageResult.OCRLanguage = pr.OCRLanguage
		pageResult.OCRDPI = pr.OCRDPI
		if pr.ProcessingError != nil {
			pageResult.Error = *pr.ProcessingError
		}
		if pr.ProcessingDurationMs != nil {
			pageResult.ProcessingDurationMs = int64(*pr.ProcessingDurationMs)
		}

		result.PageResults[pr.PageNumber-1] = pageResult

		// Update counts
		switch PageClassificationType(pr.Classification) {
		case ClassTextOnly:
			result.TextOnlyPages++
		case ClassScanned, ClassImageOnly:
			result.OCRPages++
		case ClassEmpty:
			result.SkippedPages++
		}

		if pr.ProcessingStatus == models.PageStatusFailed {
			result.FailedPages++
			result.Errors = append(result.Errors, ProcessingError{
				PageNumber: pr.PageNumber,
				Error:      pageResult.Error,
				RetryCount: pr.RetryCount,
			})
		}

		result.TotalCharacters += len(pr.Content)
		if pr.ProcessingDurationMs != nil {
			result.TotalDuration += *pr.ProcessingDurationMs
		}
	}

	return result, nil
}

// Shutdown gracefully shuts down the coordinator
func (c *DefaultCoordinator) Shutdown(ctx context.Context) error {
	return c.pool.Shutdown(ctx)
}
