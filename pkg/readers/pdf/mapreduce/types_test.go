package mapreduce

import (
	"testing"
)

func TestDefaultExtractionConfig(t *testing.T) {
	config := DefaultExtractionConfig()

	if config == nil {
		t.Fatal("DefaultExtractionConfig returned nil")
	}

	// Check default values
	tests := []struct {
		name     string
		got      interface{}
		expected interface{}
	}{
		{"OCRLanguage", config.OCRLanguage, "eng"},
		{"OCRDPI", config.OCRDPI, 150},
		{"OCRTimeout", config.OCRTimeout, 300},
		{"TextThreshold", config.TextThreshold, 100},
		{"LargeImageThreshold", config.LargeImageThreshold, 50},
		{"MaxMemoryMB", config.MaxMemoryMB, 1024},
		{"PreserveLayout", config.PreserveLayout, true},
		{"ExtractImages", config.ExtractImages, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("got %v, expected %v", tt.got, tt.expected)
			}
		})
	}
}

func TestDefaultCoordinatorConfig(t *testing.T) {
	config := DefaultCoordinatorConfig()

	if config == nil {
		t.Fatal("DefaultCoordinatorConfig returned nil")
	}

	// Check default values
	if config.MaxConcurrentWorkers != 4 {
		t.Errorf("MaxConcurrentWorkers = %d, expected 4", config.MaxConcurrentWorkers)
	}
	if config.WorkerTimeoutSeconds != 300 {
		t.Errorf("WorkerTimeoutSeconds = %d, expected 300", config.WorkerTimeoutSeconds)
	}
	if config.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, expected 3", config.MaxRetries)
	}
	if config.RetryDelay != 5 {
		t.Errorf("RetryDelay = %d, expected 5", config.RetryDelay)
	}
	if config.CheckpointInterval != 10 {
		t.Errorf("CheckpointInterval = %d, expected 10", config.CheckpointInterval)
	}
	if config.MapReducePageThreshold != 50 {
		t.Errorf("MapReducePageThreshold = %d, expected 50", config.MapReducePageThreshold)
	}
	if config.Extraction == nil {
		t.Error("Extraction config is nil")
	}
}

func TestPageClassificationNeedsOCR(t *testing.T) {
	tests := []struct {
		classification PageClassificationType
		needsOCR       bool
	}{
		{ClassTextOnly, false},
		{ClassImageOnly, true},
		{ClassHybrid, true},
		{ClassScanned, true},
		{ClassEmpty, false},
		{ClassUnknown, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.classification), func(t *testing.T) {
			pc := &PageClassification{Classification: tt.classification}
			if pc.NeedsOCR() != tt.needsOCR {
				t.Errorf("NeedsOCR() = %v, expected %v", pc.NeedsOCR(), tt.needsOCR)
			}
		})
	}
}

func TestPageResultStatus(t *testing.T) {
	tests := []struct {
		status    ProcessingStatus
		isSuccess bool
		isFailed  bool
	}{
		{StatusCompleted, true, false},
		{StatusFailed, false, true},
		{StatusPending, false, false},
		{StatusClassifying, false, false},
		{StatusExtracting, false, false},
		{StatusSkipped, false, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			pr := &PageResult{Status: tt.status}
			if pr.IsSuccess() != tt.isSuccess {
				t.Errorf("IsSuccess() = %v, expected %v", pr.IsSuccess(), tt.isSuccess)
			}
			if pr.IsFailed() != tt.isFailed {
				t.Errorf("IsFailed() = %v, expected %v", pr.IsFailed(), tt.isFailed)
			}
		})
	}
}

func TestProcessingResultCounts(t *testing.T) {
	result := &ProcessingResult{
		TotalPages:    100,
		TextOnlyPages: 70,
		OCRPages:      20,
		SkippedPages:  5,
		FailedPages:   5,
	}

	total := result.TextOnlyPages + result.OCRPages + result.SkippedPages + result.FailedPages
	if total != result.TotalPages {
		t.Errorf("Page counts don't add up: %d != %d", total, result.TotalPages)
	}
}
