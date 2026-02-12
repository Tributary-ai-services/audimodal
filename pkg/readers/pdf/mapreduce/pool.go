package mapreduce

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/google/uuid"
)

// DefaultWorkerPool implements WorkerPool with subprocess isolation
type DefaultWorkerPool struct {
	config     *CoordinatorConfig
	workerPath string
	semaphore  chan struct{}

	// Metrics
	activeWorkers  int64
	totalProcessed int64
	totalFailed    int64

	// Shutdown
	shutdown bool
	mu       sync.RWMutex
}

// NewWorkerPool creates a new worker pool
func NewWorkerPool(config *CoordinatorConfig, workerPath string) *DefaultWorkerPool {
	if config == nil {
		config = DefaultCoordinatorConfig()
	}

	maxWorkers := config.MaxConcurrentWorkers
	if maxWorkers <= 0 {
		maxWorkers = runtime.NumCPU() / 2
		if maxWorkers < 1 {
			maxWorkers = 1
		}
		if maxWorkers > 4 {
			maxWorkers = 4
		}
	}

	return &DefaultWorkerPool{
		config:     config,
		workerPath: workerPath,
		semaphore:  make(chan struct{}, maxWorkers),
	}
}

// ProcessPage sends a job to a worker subprocess and returns the result
func (p *DefaultWorkerPool) ProcessPage(ctx context.Context, job *PageJob) (*PageResult, error) {
	p.mu.RLock()
	if p.shutdown {
		p.mu.RUnlock()
		return nil, fmt.Errorf("worker pool is shutdown")
	}
	p.mu.RUnlock()

	// Acquire semaphore to limit concurrency.
	// We use the caller's context here so cancellation is respected while waiting.
	select {
	case p.semaphore <- struct{}{}:
		defer func() { <-p.semaphore }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	atomic.AddInt64(&p.activeWorkers, 1)
	defer atomic.AddInt64(&p.activeWorkers, -1)

	// Create per-page timeout from context.Background() so it does NOT inherit
	// any deadline from the parent context. This ensures each page gets its full
	// timeout budget regardless of how long prior pages took or any upstream
	// deadline on the parent context.
	timeout := time.Duration(p.config.WorkerTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	pageCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Propagate only explicit parent cancellation (for graceful shutdown),
	// not deadline exceeded. This prevents an upstream timeout from killing
	// page processing that has its own independent timeout.
	go func() {
		select {
		case <-ctx.Done():
			if ctx.Err() == context.Canceled {
				cancel()
			}
		case <-pageCtx.Done():
		}
	}()

	// Assign a unique job ID if not set
	if job.JobID == "" {
		job.JobID = uuid.New().String()
	}

	// Serialize job to JSON
	jobJSON, err := json.Marshal(job)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize job: %w", err)
	}

	// Execute worker subprocess
	result, err := p.executeWorker(pageCtx, jobJSON)
	if err != nil {
		atomic.AddInt64(&p.totalFailed, 1)
		return nil, err
	}

	atomic.AddInt64(&p.totalProcessed, 1)
	return result, nil
}

// executeWorker runs the pdfworker binary as a subprocess
func (p *DefaultWorkerPool) executeWorker(ctx context.Context, jobJSON []byte) (*PageResult, error) {
	// Do NOT use exec.CommandContext — it only kills the direct process, not
	// child processes (e.g. tesseract). Instead we manage the timeout ourselves
	// and kill the entire process group on cancellation.
	cmd := exec.Command(p.workerPath)

	// Set up stdin/stdout pipes
	cmd.Stdin = bytes.NewReader(jobJSON)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Set process attributes for Linux: use a new process group so we can
	// kill the worker AND all its children (tesseract, etc.) on timeout.
	if runtime.GOOS == "linux" {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Setpgid:   true,           // Create new process group
			Pdeathsig: syscall.SIGKILL, // Kill worker if parent dies
		}
	}

	// Start the worker process
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start worker: %w", err)
	}

	// Kill entire process group on context cancellation/timeout
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			if cmd.Process != nil {
				// Kill the entire process group (negative PID)
				_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			}
		case <-done:
		}
	}()

	// Wait for completion
	err := cmd.Wait()
	close(done) // stop the killer goroutine

	if err != nil {
		// Check if it was a context cancellation/timeout
		if ctx.Err() != nil {
			return nil, fmt.Errorf("worker timed out: %w", ctx.Err())
		}
		return nil, fmt.Errorf("worker failed: %w (stderr: %s)", err, stderr.String())
	}

	// Parse result
	var result PageResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse worker output: %w (raw: %s)", err, stdout.String())
	}

	return &result, nil
}

// Shutdown gracefully shuts down the worker pool
func (p *DefaultWorkerPool) Shutdown(ctx context.Context) error {
	p.mu.Lock()
	p.shutdown = true
	p.mu.Unlock()

	// Wait for active workers to complete
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if atomic.LoadInt64(&p.activeWorkers) == 0 {
				return nil
			}
		}
	}
}

// GetStats returns pool statistics
func (p *DefaultWorkerPool) GetStats() WorkerPoolStats {
	return WorkerPoolStats{
		ActiveWorkers:  atomic.LoadInt64(&p.activeWorkers),
		TotalProcessed: atomic.LoadInt64(&p.totalProcessed),
		TotalFailed:    atomic.LoadInt64(&p.totalFailed),
	}
}

// WorkerPoolStats contains pool statistics
type WorkerPoolStats struct {
	ActiveWorkers  int64 `json:"active_workers"`
	TotalProcessed int64 `json:"total_processed"`
	TotalFailed    int64 `json:"total_failed"`
}

// InlineWorkerPool processes pages inline (without subprocess) for testing or fallback
type InlineWorkerPool struct {
	config    *CoordinatorConfig
	extractor *DefaultPageExtractor
	semaphore chan struct{}

	activeWorkers  int64
	totalProcessed int64
	totalFailed    int64

	shutdown bool
	mu       sync.RWMutex
}

// NewInlineWorkerPool creates a worker pool that processes inline (no subprocess)
func NewInlineWorkerPool(config *CoordinatorConfig) *InlineWorkerPool {
	if config == nil {
		config = DefaultCoordinatorConfig()
	}

	maxWorkers := config.MaxConcurrentWorkers
	if maxWorkers <= 0 {
		maxWorkers = 2
	}

	return &InlineWorkerPool{
		config:    config,
		extractor: NewPageExtractor(config.Extraction),
		semaphore: make(chan struct{}, maxWorkers),
	}
}

// ProcessPage processes a page inline (without subprocess isolation)
func (p *InlineWorkerPool) ProcessPage(ctx context.Context, job *PageJob) (*PageResult, error) {
	p.mu.RLock()
	if p.shutdown {
		p.mu.RUnlock()
		return nil, fmt.Errorf("worker pool is shutdown")
	}
	p.mu.RUnlock()

	// Acquire semaphore
	select {
	case p.semaphore <- struct{}{}:
		defer func() { <-p.semaphore }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	atomic.AddInt64(&p.activeWorkers, 1)
	defer atomic.AddInt64(&p.activeWorkers, -1)

	// Create per-page timeout from context.Background() so it does NOT inherit
	// any deadline from the parent context.
	timeout := time.Duration(p.config.WorkerTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	pageCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Propagate only explicit parent cancellation, not deadline exceeded
	go func() {
		select {
		case <-ctx.Done():
			if ctx.Err() == context.Canceled {
				cancel()
			}
		case <-pageCtx.Done():
		}
	}()

	// Process inline
	result, err := p.extractor.ExtractPage(pageCtx, job)
	if err != nil {
		atomic.AddInt64(&p.totalFailed, 1)
		return nil, err
	}

	atomic.AddInt64(&p.totalProcessed, 1)
	return result, nil
}

// Shutdown gracefully shuts down the pool
func (p *InlineWorkerPool) Shutdown(ctx context.Context) error {
	p.mu.Lock()
	p.shutdown = true
	p.mu.Unlock()

	// Wait for active workers
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if atomic.LoadInt64(&p.activeWorkers) == 0 {
				return nil
			}
		}
	}
}

// GetStats returns pool statistics
func (p *InlineWorkerPool) GetStats() WorkerPoolStats {
	return WorkerPoolStats{
		ActiveWorkers:  atomic.LoadInt64(&p.activeWorkers),
		TotalProcessed: atomic.LoadInt64(&p.totalProcessed),
		TotalFailed:    atomic.LoadInt64(&p.totalFailed),
	}
}
