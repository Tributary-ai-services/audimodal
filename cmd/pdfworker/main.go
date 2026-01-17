// pdfworker is a standalone binary for memory-isolated PDF page extraction.
// It receives a job via stdin, processes a single page, outputs the result
// to stdout, and exits immediately so the OS can reclaim all memory.
//
// Usage: echo '{"pdf_path": "/path/to/file.pdf", "page_number": 1, ...}' | pdfworker
//
// This design ensures Tesseract OCR memory is fully released after each page,
// preventing OOM issues with large multi-page PDFs.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/jscharber/audimodal/pkg/readers/pdf/mapreduce"
)

func main() {
	// Limit parallelism to reduce memory footprint
	runtime.GOMAXPROCS(1)

	// Set a global timeout for the entire operation
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Read job from stdin
	var job mapreduce.PageJob
	decoder := json.NewDecoder(os.Stdin)
	if err := decoder.Decode(&job); err != nil {
		outputError(fmt.Errorf("failed to decode job: %w", err))
		os.Exit(1)
	}

	// Validate job
	if job.PDFPath == "" {
		outputError(fmt.Errorf("pdf_path is required"))
		os.Exit(1)
	}
	if job.PageNumber < 1 {
		outputError(fmt.Errorf("page_number must be >= 1"))
		os.Exit(1)
	}

	// Process the page
	config := job.Config
	if config == nil {
		config = mapreduce.DefaultExtractionConfig()
	}

	extractor := mapreduce.NewPageExtractor(config)
	result, err := extractor.ExtractPage(ctx, &job)

	if err != nil {
		// Create error result
		result = &mapreduce.PageResult{
			JobID:          job.JobID,
			TenantID:       job.TenantID,
			FileID:         job.FileID,
			PageNumber:     job.PageNumber,
			TotalPages:     job.TotalPages,
			Classification: job.Classification,
			Status:         mapreduce.StatusFailed,
			Error:          err.Error(),
			ProcessedAt:    time.Now(),
			WorkerID:       getWorkerID(),
		}
	}

	// Output result to stdout
	encoder := json.NewEncoder(os.Stdout)
	if err := encoder.Encode(result); err != nil {
		fmt.Fprintf(os.Stderr, "failed to encode result: %v\n", err)
		os.Exit(1)
	}

	// Exit cleanly - OS will reclaim all memory
	os.Exit(0)
}

// outputError writes an error result to stdout
func outputError(err error) {
	result := &mapreduce.PageResult{
		Status:      mapreduce.StatusFailed,
		Error:       err.Error(),
		ProcessedAt: time.Now(),
		WorkerID:    getWorkerID(),
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.Encode(result)
}

// getWorkerID returns a unique identifier for this worker process
func getWorkerID() string {
	hostname, _ := os.Hostname()
	return fmt.Sprintf("%s-%d", hostname, os.Getpid())
}
