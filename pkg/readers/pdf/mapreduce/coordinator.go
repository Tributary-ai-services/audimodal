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

	// Create a deadline-free context for page processing.
	// The parent ctx may have a short deadline (e.g., from an HTTP handler),
	// but processing 100+ OCR pages can take 20+ minutes. Each page gets its
	// own timeout via the worker pool. We only propagate explicit cancellation
	// (context.Canceled), NOT deadline exceeded, so processing continues even
	// if the upstream context times out.
	processingCtx, processingCancel := context.WithCancel(context.Background())
	defer processingCancel()

	go func() {
		select {
		case <-ctx.Done():
			// Only propagate explicit cancellation, not deadline exceeded.
			// When the parent's deadline expires, we want processing to continue
			// since each page has its own independent timeout.
			if ctx.Err() == context.Canceled {
				processingCancel()
			}
		case <-processingCtx.Done():
		}
	}()

	// Phase 3: Extract content from all pages
	log.Printf("[MapReduce] Phase 2: Extracting content from %d pages", totalPages)

	var mu sync.Mutex
	var wg sync.WaitGroup
	errors := make([]ProcessingError, 0)

	// Use a work channel to limit goroutine creation.
	// Instead of spawning totalPages goroutines (which all block on the pool
	// semaphore), we spawn a fixed number of worker goroutines that pull jobs
	// from a channel. This prevents goroutine accumulation and ensures each
	// page gets its full timeout budget after it starts processing.
	type pageWork struct {
		pageNum        int
		classification PageClassificationType
	}

	workCh := make(chan pageWork, totalPages)
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		classification := ClassUnknown
		if pageNum-1 < len(classifications) && classifications[pageNum-1] != nil {
			classification = classifications[pageNum-1].Classification
		}
		workCh <- pageWork{pageNum: pageNum, classification: classification}
	}
	close(workCh)

	numWorkers := c.config.MaxConcurrentWorkers
	if numWorkers <= 0 {
		numWorkers = 4
	}

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for work := range workCh {
				pn := work.pageNum
				classification := work.classification

				// Check if processing was cancelled
				if processingCtx.Err() != nil {
					mu.Lock()
					errors = append(errors, ProcessingError{
						PageNumber: pn,
						Error:      "processing cancelled",
						Retryable:  true,
					})
					result.FailedPages++
					result.PageResults[pn-1] = &PageResult{
						PageNumber:     pn,
						TotalPages:     totalPages,
						Classification: classification,
						Status:         StatusFailed,
						Error:          "processing cancelled",
						ProcessedAt:    time.Now(),
					}
					mu.Unlock()
					continue
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
						log.Printf("[MapReduce] Retrying page %d (attempt %d/%d): %v", pn, attempt, c.config.MaxRetries, processErr)
					}

					pageResult, processErr = c.pool.ProcessPage(processingCtx, job)
					if processErr == nil && pageResult != nil && pageResult.Status == StatusCompleted {
						break
					}
				}

				mu.Lock()

				if processErr != nil || pageResult == nil || pageResult.Status != StatusCompleted {
					errMsg := "unknown error"
					if processErr != nil {
						errMsg = processErr.Error()
					} else if pageResult != nil && pageResult.Error != "" {
						errMsg = pageResult.Error
					}

					log.Printf("[MapReduce] Page %d failed after %d retries: %s", pn, c.config.MaxRetries, errMsg)

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
					c.savePageResultToDB(processingCtx, tenantID, fileID, pageResult)
				}

				// Log progress periodically
				completed := 0
				for _, pr := range result.PageResults {
					if pr != nil {
						completed++
					}
				}
				if completed%c.config.CheckpointInterval == 0 {
					log.Printf("[MapReduce] Progress: %d/%d pages processed (%d failed so far)", completed, totalPages, result.FailedPages)
				}

				mu.Unlock()
			}
		}()
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

	// Create a deadline-free context for page processing (same as Process method)
	processingCtx, processingCancel := context.WithCancel(context.Background())
	defer processingCancel()

	go func() {
		select {
		case <-ctx.Done():
			if ctx.Err() == context.Canceled {
				processingCancel()
			}
		case <-processingCtx.Done():
		}
	}()

	var mu sync.Mutex
	var wg sync.WaitGroup
	errors := make([]ProcessingError, 0)
	results := make(map[int]*PageResult)

	// Use a work channel instead of spawning one goroutine per page
	type resumeWork struct {
		pageNum        int
		classification PageClassificationType
	}

	workCh := make(chan resumeWork, len(allPending))
	for _, pn := range allPending {
		classification := ClassUnknown
		for _, pr := range pageResults {
			if pr.PageNumber == pn {
				classification = PageClassificationType(pr.Classification)
				break
			}
		}
		workCh <- resumeWork{pageNum: pn, classification: classification}
	}
	close(workCh)

	numWorkers := c.config.MaxConcurrentWorkers
	if numWorkers <= 0 {
		numWorkers = 4
	}

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for work := range workCh {
				pn := work.pageNum

				job := &PageJob{
					JobID:          uuid.New().String(),
					TenantID:       file.TenantID,
					FileID:         fileID,
					PDFPath:        file.Path,
					PageNumber:     pn,
					TotalPages:     totalPages,
					Classification: work.classification,
					Config:         c.config.Extraction,
				}

				pageResult, err := c.pool.ProcessPage(processingCtx, job)

				mu.Lock()

				if err != nil || pageResult == nil || pageResult.Status != StatusCompleted {
					errMsg := "unknown error"
					if err != nil {
						errMsg = err.Error()
					} else if pageResult != nil && pageResult.Error != "" {
						errMsg = pageResult.Error
					}

					log.Printf("[MapReduce] Resume: page %d failed: %s", pn, errMsg)

					errors = append(errors, ProcessingError{
						PageNumber: pn,
						Error:      errMsg,
						Retryable:  true,
					})
				}

				if pageResult != nil {
					results[pn] = pageResult
					c.savePageResultToDB(processingCtx, file.TenantID, fileID, pageResult)
				}

				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	return c.buildResultFromDB(processingCtx, fileID, totalPages)
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
