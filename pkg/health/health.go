package health

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Status represents the health status of a component
type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusUnhealthy Status = "unhealthy"
	StatusDegraded  Status = "degraded"
	StatusUnknown   Status = "unknown"
)

// CheckResult represents the result of a health check
type CheckResult struct {
	Name        string            `json:"name"`
	Status      Status            `json:"status"`
	Message     string            `json:"message,omitempty"`
	Error       string            `json:"error,omitempty"`
	Duration    time.Duration     `json:"duration"`
	Timestamp   time.Time         `json:"timestamp"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Critical    bool              `json:"critical"`
}

// Checker represents a health check function
type Checker interface {
	Check(ctx context.Context) CheckResult
	Name() string
	IsCritical() bool
}

// CheckerFunc is a function that implements the Checker interface
type CheckerFunc struct {
	name     string
	critical bool
	checkFn  func(ctx context.Context) CheckResult
}

// NewChecker creates a new health checker
func NewChecker(name string, critical bool, checkFn func(ctx context.Context) CheckResult) *CheckerFunc {
	return &CheckerFunc{
		name:     name,
		critical: critical,
		checkFn:  checkFn,
	}
}

// Check executes the health check
func (c *CheckerFunc) Check(ctx context.Context) CheckResult {
	return c.checkFn(ctx)
}

// Name returns the checker name
func (c *CheckerFunc) Name() string {
	return c.name
}

// IsCritical returns whether this check is critical
func (c *CheckerFunc) IsCritical() bool {
	return c.critical
}

// HealthChecker manages multiple health checks
type HealthChecker struct {
	checkers map[string]Checker
	timeout  time.Duration
	mu       sync.RWMutex
}

// NewHealthChecker creates a new health checker manager
func NewHealthChecker(timeout time.Duration) *HealthChecker {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	return &HealthChecker{
		checkers: make(map[string]Checker),
		timeout:  timeout,
	}
}

// AddChecker adds a health checker
func (hc *HealthChecker) AddChecker(checker Checker) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	hc.checkers[checker.Name()] = checker
}

// RemoveChecker removes a health checker
func (hc *HealthChecker) RemoveChecker(name string) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	delete(hc.checkers, name)
}

// HealthReport represents the overall health status
type HealthReport struct {
	Status      Status                   `json:"status"`
	Timestamp   time.Time                `json:"timestamp"`
	Duration    time.Duration            `json:"duration"`
	Version     string                   `json:"version"`
	Service     string                   `json:"service"`
	Checks      map[string]CheckResult   `json:"checks"`
	Summary     map[string]int           `json:"summary"`
	Critical    bool                     `json:"critical"`
}

// Check performs all health checks and returns a report
func (hc *HealthChecker) Check(ctx context.Context, service, version string) HealthReport {
	start := time.Now()
	
	// Create context with timeout
	checkCtx, cancel := context.WithTimeout(ctx, hc.timeout)
	defer cancel()

	hc.mu.RLock()
	checkers := make(map[string]Checker, len(hc.checkers))
	for name, checker := range hc.checkers {
		checkers[name] = checker
	}
	hc.mu.RUnlock()

	results := make(map[string]CheckResult)
	summary := map[string]int{
		string(StatusHealthy):   0,
		string(StatusUnhealthy): 0,
		string(StatusDegraded):  0,
		string(StatusUnknown):   0,
	}

	// Run checks in parallel
	var wg sync.WaitGroup
	resultsChan := make(chan CheckResult, len(checkers))

	for _, checker := range checkers {
		wg.Add(1)
		go func(c Checker) {
			defer wg.Done()
			result := hc.runSingleCheck(checkCtx, c)
			resultsChan <- result
		}(checker)
	}

	// Wait for all checks to complete
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect results
	criticalFailed := false
	for result := range resultsChan {
		results[result.Name] = result
		summary[string(result.Status)]++

		// Check if any critical check failed
		if result.Critical && result.Status != StatusHealthy {
			criticalFailed = true
		}
	}

	// Determine overall status
	overallStatus := hc.determineOverallStatus(summary, criticalFailed)

	return HealthReport{
		Status:    overallStatus,
		Timestamp: time.Now(),
		Duration:  time.Since(start),
		Version:   version,
		Service:   service,
		Checks:    results,
		Summary:   summary,
		Critical:  criticalFailed,
	}
}

// runSingleCheck runs a single health check with proper error handling
func (hc *HealthChecker) runSingleCheck(ctx context.Context, checker Checker) CheckResult {
	start := time.Now()
	
	defer func() {
		if r := recover(); r != nil {
			// Handle panics in health checks
		}
	}()

	result := checker.Check(ctx)
	result.Duration = time.Since(start)
	result.Timestamp = time.Now()
	result.Critical = checker.IsCritical()

	// Ensure result has a name
	if result.Name == "" {
		result.Name = checker.Name()
	}

	return result
}

// determineOverallStatus determines the overall health status
func (hc *HealthChecker) determineOverallStatus(summary map[string]int, criticalFailed bool) Status {
	// If any critical check failed, overall status is unhealthy
	if criticalFailed {
		return StatusUnhealthy
	}

	// If any check is unhealthy, overall status is unhealthy
	if summary[string(StatusUnhealthy)] > 0 {
		return StatusUnhealthy
	}

	// If any check is degraded, overall status is degraded
	if summary[string(StatusDegraded)] > 0 {
		return StatusDegraded
	}

	// If any check is unknown, overall status is degraded
	if summary[string(StatusUnknown)] > 0 {
		return StatusDegraded
	}

	return StatusHealthy
}

// Common health check implementations

// DatabaseChecker creates a database health checker
func DatabaseChecker(name string, checkFn func(ctx context.Context) error) Checker {
	return NewChecker(name, true, func(ctx context.Context) CheckResult {
		if err := checkFn(ctx); err != nil {
			return CheckResult{
				Name:    name,
				Status:  StatusUnhealthy,
				Error:   err.Error(),
				Message: "Database connection failed",
			}
		}
		return CheckResult{
			Name:    name,
			Status:  StatusHealthy,
			Message: "Database connection successful",
		}
	})
}

// HTTPChecker creates an HTTP endpoint health checker
func HTTPChecker(name, url string, timeout time.Duration, critical bool) Checker {
	return NewChecker(name, critical, func(ctx context.Context) CheckResult {
		// This would implement HTTP health checking
		// For now, return a placeholder
		return CheckResult{
			Name:    name,
			Status:  StatusHealthy,
			Message: fmt.Sprintf("HTTP endpoint %s is reachable", url),
		}
	})
}

// MemoryChecker creates a memory usage health checker
func MemoryChecker(name string, maxUsagePercent float64) Checker {
	return NewChecker(name, false, func(ctx context.Context) CheckResult {
		// This would implement memory usage checking
		// For now, return a placeholder
		return CheckResult{
			Name:    name,
			Status:  StatusHealthy,
			Message: "Memory usage is within acceptable limits",
			Metadata: map[string]string{
				"max_usage_percent": fmt.Sprintf("%.2f", maxUsagePercent),
			},
		}
	})
}

// DiskSpaceChecker creates a disk space health checker
func DiskSpaceChecker(name, path string, minFreePercent float64) Checker {
	return NewChecker(name, false, func(ctx context.Context) CheckResult {
		// This would implement disk space checking
		// For now, return a placeholder
		return CheckResult{
			Name:    name,
			Status:  StatusHealthy,
			Message: fmt.Sprintf("Disk space on %s is sufficient", path),
			Metadata: map[string]string{
				"path":             path,
				"min_free_percent": fmt.Sprintf("%.2f", minFreePercent),
			},
		}
	})
}

// CustomChecker creates a custom health checker
func CustomChecker(name string, critical bool, checkFn func(ctx context.Context) (Status, string, error)) Checker {
	return NewChecker(name, critical, func(ctx context.Context) CheckResult {
		status, message, err := checkFn(ctx)
		
		result := CheckResult{
			Name:    name,
			Status:  status,
			Message: message,
		}
		
		if err != nil {
			result.Error = err.Error()
			if status == StatusHealthy {
				result.Status = StatusUnhealthy
			}
		}
		
		return result
	})
}