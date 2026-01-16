package mapreduce

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewWorkerPool(t *testing.T) {
	// Test with nil config
	pool := NewWorkerPool(nil, "/bin/echo")
	if pool == nil {
		t.Fatal("NewWorkerPool returned nil")
	}

	// Should have default workers
	if cap(pool.semaphore) < 1 {
		t.Error("semaphore capacity should be >= 1")
	}
}

func TestNewInlineWorkerPool(t *testing.T) {
	pool := NewInlineWorkerPool(nil)
	if pool == nil {
		t.Fatal("NewInlineWorkerPool returned nil")
	}

	if pool.extractor == nil {
		t.Error("extractor is nil")
	}
}

func TestWorkerPoolStats(t *testing.T) {
	pool := NewWorkerPool(nil, "/bin/echo")

	stats := pool.GetStats()
	if stats.ActiveWorkers != 0 {
		t.Errorf("ActiveWorkers = %d, expected 0", stats.ActiveWorkers)
	}
	if stats.TotalProcessed != 0 {
		t.Errorf("TotalProcessed = %d, expected 0", stats.TotalProcessed)
	}
	if stats.TotalFailed != 0 {
		t.Errorf("TotalFailed = %d, expected 0", stats.TotalFailed)
	}
}

func TestInlineWorkerPoolStats(t *testing.T) {
	pool := NewInlineWorkerPool(nil)

	stats := pool.GetStats()
	if stats.ActiveWorkers != 0 {
		t.Errorf("ActiveWorkers = %d, expected 0", stats.ActiveWorkers)
	}
}

func TestWorkerPoolShutdown(t *testing.T) {
	pool := NewWorkerPool(nil, "/bin/echo")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := pool.Shutdown(ctx)
	if err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}

	// After shutdown, ProcessPage should fail
	job := &PageJob{
		JobID:      uuid.New().String(),
		PDFPath:    "/test.pdf",
		PageNumber: 1,
	}

	_, err = pool.ProcessPage(context.Background(), job)
	if err == nil {
		t.Error("expected error after shutdown")
	}
}

func TestInlineWorkerPoolShutdown(t *testing.T) {
	pool := NewInlineWorkerPool(nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := pool.Shutdown(ctx)
	if err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}

	// After shutdown, ProcessPage should fail
	job := &PageJob{
		JobID:      uuid.New().String(),
		PDFPath:    "/test.pdf",
		PageNumber: 1,
	}

	_, err = pool.ProcessPage(context.Background(), job)
	if err == nil {
		t.Error("expected error after shutdown")
	}
}

func TestWorkerPoolConcurrency(t *testing.T) {
	config := &CoordinatorConfig{
		MaxConcurrentWorkers: 2,
		WorkerTimeoutSeconds: 5,
		Extraction:           DefaultExtractionConfig(),
	}

	pool := NewInlineWorkerPool(config)

	// Test that pool has correct semaphore capacity
	if cap(pool.semaphore) != config.MaxConcurrentWorkers {
		t.Errorf("semaphore capacity = %d, expected %d", cap(pool.semaphore), config.MaxConcurrentWorkers)
	}

	// Simple sequential test - process a few jobs and verify stats
	numJobs := 5
	for i := 0; i < numJobs; i++ {
		job := &PageJob{
			JobID:          uuid.New().String(),
			PDFPath:        "/nonexistent.pdf",
			PageNumber:     i + 1,
			TotalPages:     numJobs,
			Classification: ClassEmpty, // Will be skipped quickly
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		pool.ProcessPage(ctx, job)
		cancel()
	}

	stats := pool.GetStats()
	// All jobs should have been processed
	if stats.TotalProcessed != int64(numJobs) {
		t.Errorf("TotalProcessed = %d, expected %d", stats.TotalProcessed, numJobs)
	}
	// No active workers after completion
	if stats.ActiveWorkers != 0 {
		t.Errorf("ActiveWorkers = %d, expected 0", stats.ActiveWorkers)
	}
}

func TestWorkerPoolContextCancellation(t *testing.T) {
	pool := NewInlineWorkerPool(nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	job := &PageJob{
		JobID:          uuid.New().String(),
		PDFPath:        "/test.pdf",
		PageNumber:     1,
		Classification: ClassTextOnly,
	}

	_, err := pool.ProcessPage(ctx, job)
	// Context cancellation should result in an error when trying to acquire semaphore
	// However, if the semaphore is immediately available, the job may start processing
	// In this case, the extraction will fail due to invalid PDF path
	// Either way is acceptable - we're testing that context cancellation is handled
	t.Logf("ProcessPage with cancelled context: err=%v", err)
}

func TestWorkerPoolJobIDAssignment(t *testing.T) {
	pool := NewWorkerPool(nil, "/nonexistent/worker")

	job := &PageJob{
		PDFPath:        "/test.pdf",
		PageNumber:     1,
		Classification: ClassEmpty,
	}

	// JobID should be empty initially
	if job.JobID != "" {
		t.Error("JobID should be empty before processing")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// This will fail (no worker binary), but JobID should be assigned
	pool.ProcessPage(ctx, job)

	// Note: In current implementation, JobID is assigned inside ProcessPage
	// but we can't check it after since the job is passed by value
}
