package tests

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jscharber/eAIIngest/pkg/auth"
	"github.com/jscharber/eAIIngest/pkg/classification"
	"github.com/jscharber/eAIIngest/pkg/dlp/types"
	"github.com/jscharber/eAIIngest/pkg/events"
	"github.com/jscharber/eAIIngest/pkg/storage"
	"github.com/jscharber/eAIIngest/pkg/workflow"
)

// TestRunner provides a simple way to run integration tests programmatically
type TestRunner struct {
	env     *TestEnvironment
	results map[string]TestResult
}

// TestResult stores the result of a test execution
type TestResult struct {
	Name     string
	Success  bool
	Duration time.Duration
	Error    error
	Details  map[string]interface{}
}

// NewTestRunner creates a new test runner
func NewTestRunner() *TestRunner {
	return &TestRunner{
		results: make(map[string]TestResult),
	}
}

// Setup initializes the test environment
func (tr *TestRunner) Setup() error {
	// Create a mock testing.T for environment setup
	mockT := &testing.T{}
	tr.env = SetupTestEnvironment(mockT)
	return nil
}

// RunAllTests executes all integration tests
func (tr *TestRunner) RunAllTests() {
	tests := []struct {
		name string
		fn   func() error
	}{
		{"Authentication", tr.testAuthentication},
		{"FileReading", tr.testFileReading},
		{"ContentChunking", tr.testContentChunking},
		{"ContentClassification", tr.testContentClassification},
		{"DLPDetection", tr.testDLPDetection},
		{"EventSystem", tr.testEventSystem},
		{"StorageResolvers", tr.testStorageResolvers},
		{"WorkflowExecution", tr.testWorkflowExecution},
		{"EndToEndPipeline", tr.testEndToEndPipeline},
	}

	fmt.Printf("Running %d integration tests...\n\n", len(tests))

	successCount := 0
	for _, test := range tests {
		fmt.Printf("Running test: %s... ", test.name)

		start := time.Now()
		err := test.fn()
		duration := time.Since(start)

		success := err == nil
		if success {
			successCount++
			fmt.Printf("PASS (%v)\n", duration)
		} else {
			fmt.Printf("FAIL (%v): %v\n", duration, err)
		}

		tr.results[test.name] = TestResult{
			Name:     test.name,
			Success:  success,
			Duration: duration,
			Error:    err,
		}
	}

	fmt.Printf("\nTest Results: %d/%d passed\n", successCount, len(tests))

	if successCount == len(tests) {
		fmt.Println("All tests passed! ✅")
	} else {
		fmt.Printf("%d tests failed ❌\n", len(tests)-successCount)
	}
}

// GetResults returns the test results
func (tr *TestRunner) GetResults() map[string]TestResult {
	return tr.results
}

// Test implementations

func (tr *TestRunner) testAuthentication() error {
	ctx := context.Background()

	// Test user creation
	createReq := &auth.CreateUserRequest{
		Username:  "runner_test_user",
		Email:     "runner@test.com",
		Password:  "RunnerPass123!",
		FirstName: "Runner",
		LastName:  "Test",
		Roles:     []auth.Role{auth.RoleUser},
	}

	user, err := tr.env.AuthService.CreateUser(ctx, createReq)
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	// Test authentication
	authReq := &auth.AuthRequest{
		Type:     auth.AuthTypePassword,
		Username: "runner_test_user",
		Password: "RunnerPass123!",
	}

	authResp, err := tr.env.AuthService.Authenticate(ctx, authReq)
	if err != nil {
		return fmt.Errorf("failed to authenticate: %w", err)
	}

	// Test token validation
	claims, err := tr.env.AuthService.ValidateToken(ctx, authResp.AccessToken)
	if err != nil {
		return fmt.Errorf("failed to validate token: %w", err)
	}

	if claims.UserID != user.ID {
		return fmt.Errorf("token user ID mismatch")
	}

	return nil
}

func (tr *TestRunner) testFileReading() error {
	// Test CSV reading
	csvData, err := tr.env.CSVReader.Read(tr.env.TestFiles["csv"])
	if err != nil {
		return fmt.Errorf("failed to read CSV: %w", err)
	}
	if csvData.Type != "csv" {
		return fmt.Errorf("incorrect CSV data type")
	}

	// Test JSON reading
	jsonData, err := tr.env.JSONReader.Read(tr.env.TestFiles["json"])
	if err != nil {
		return fmt.Errorf("failed to read JSON: %w", err)
	}
	if jsonData.Type != "json" {
		return fmt.Errorf("incorrect JSON data type")
	}

	// Test text reading
	textData, err := tr.env.TextReader.Read(tr.env.TestFiles["text"])
	if err != nil {
		return fmt.Errorf("failed to read text: %w", err)
	}
	if textData.Type != "text" {
		return fmt.Errorf("incorrect text data type")
	}

	return nil
}

func (tr *TestRunner) testContentChunking() error {
	content := "This is a test document that will be chunked into smaller pieces for processing. Each chunk should be manageable and contain relevant content."

	chunks, err := tr.env.Chunker.Chunk(content)
	if err != nil {
		return fmt.Errorf("failed to chunk content: %w", err)
	}

	if len(chunks) == 0 {
		return fmt.Errorf("no chunks produced")
	}

	return nil
}

func (tr *TestRunner) testContentClassification() error {
	ctx := context.Background()
	testContent := "This is a business document about quarterly financial reports and earnings data."

	// Create proper classification input
	input := &classification.ClassificationInput{
		Content:  testContent,
		TenantID: uuid.New(),
	}

	result, err := tr.env.Classifier.Classify(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to classify content: %w", err)
	}

	if result.ContentType == "" {
		return fmt.Errorf("content type not detected")
	}

	if result.Language == "" {
		return fmt.Errorf("language not detected")
	}

	return nil
}

func (tr *TestRunner) testDLPDetection() error {
	testContent := "Employee SSN: 123-45-6789, Credit Card: 4532-1234-5678-9012"

	results, err := tr.env.DLPEngine.ScanText(testContent)
	if err != nil {
		return fmt.Errorf("failed to scan for PII: %w", err)
	}

	if len(results) == 0 {
		return fmt.Errorf("no PII detected in test content")
	}

	// Verify we found the expected PII types
	foundSSN := false
	foundCC := false

	for _, result := range results {
		switch result.Type {
		case string(types.PIITypeSSN):
			foundSSN = true
		case string(types.PIITypeCreditCard):
			foundCC = true
		}
	}

	if !foundSSN {
		return fmt.Errorf("SSN not detected")
	}

	if !foundCC {
		return fmt.Errorf("credit card not detected")
	}

	return nil
}

func (tr *TestRunner) testEventSystem() error {
	// Create a test event
	event := &events.Event{
		ID:       uuid.New(),
		Type:     "file_processed",
		TenantID: uuid.New(),
		Payload: map[string]interface{}{
			"test": "data",
		},
		CreatedAt: time.Now(),
	}

	// Publish event
	err := tr.env.EventBus.Publish(event)
	if err != nil {
		return fmt.Errorf("failed to publish event: %w", err)
	}

	return nil
}

func (tr *TestRunner) testStorageResolvers() error {
	// Test local file URL parsing
	localURL := "file:///tmp/test.txt"
	resolver, err := tr.env.StorageRegistry.GetResolver(storage.ProviderLocal)
	if err != nil {
		return fmt.Errorf("failed to get local resolver: %w", err)
	}

	parsedURL, err := resolver.ParseURL(localURL)
	if err != nil {
		return fmt.Errorf("failed to parse local URL: %w", err)
	}

	if parsedURL.Provider != storage.ProviderLocal {
		return fmt.Errorf("incorrect URL provider parsed")
	}

	// Test S3 URL parsing
	s3URL := "s3://test-bucket/path/file.txt"
	s3ParsedURL, err := tr.env.StorageRegistry.ParseURL(s3URL)
	if err != nil {
		return fmt.Errorf("failed to parse S3 URL: %w", err)
	}

	if s3ParsedURL.Provider != storage.ProviderAWS {
		return fmt.Errorf("incorrect S3 provider")
	}

	if s3ParsedURL.Bucket != "test-bucket" {
		return fmt.Errorf("incorrect S3 bucket")
	}

	return nil
}

func (tr *TestRunner) testWorkflowExecution() error {
	ctx := context.Background()

	// Create a simple test workflow
	testWorkflow := &workflow.WorkflowDefinition{
		ID:   workflow.GenerateWorkflowID(),
		Name: "test-runner-workflow",
		Steps: []workflow.WorkflowStep{
			{
				ID:   "step1",
				Name: "Test Step",
				Type: workflow.StepTypeFileRead,
				Config: map[string]interface{}{
					"path": "/test/file.txt",
				},
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Register workflow
	err := tr.env.WorkflowEngine.RegisterWorkflow(testWorkflow)
	if err != nil {
		return fmt.Errorf("failed to register workflow: %w", err)
	}

	// Execute workflow
	input := map[string]interface{}{
		"test": "input",
	}

	executionID, err := tr.env.WorkflowEngine.ExecuteWorkflow(ctx, testWorkflow.ID, input)
	if err != nil {
		return fmt.Errorf("failed to execute workflow: %w", err)
	}

	if executionID == (workflow.ExecutionID{}) {
		return fmt.Errorf("invalid execution ID returned")
	}

	return nil
}

func (tr *TestRunner) testEndToEndPipeline() error {
	ctx := context.Background()

	// Read a test file
	textData, err := tr.env.TextReader.Read(tr.env.TestFiles["text"])
	if err != nil {
		return fmt.Errorf("failed to read test file: %w", err)
	}

	content := string(textData.Content)

	// Chunk the content
	chunks, err := tr.env.Chunker.Chunk(content)
	if err != nil {
		return fmt.Errorf("failed to chunk content: %w", err)
	}

	// Classify content
	input := &classification.ClassificationInput{
		Content:  content,
		TenantID: uuid.New(),
	}
	classificationResult, err := tr.env.Classifier.Classify(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to classify content: %w", err)
	}

	// Scan for PII
	dlpResults, err := tr.env.DLPEngine.ScanText(content)
	if err != nil {
		return fmt.Errorf("failed to scan for PII: %w", err)
	}

	// Verify pipeline results
	if len(chunks) == 0 {
		return fmt.Errorf("no chunks produced")
	}

	if classificationResult.ContentType == "" {
		return fmt.Errorf("no content type classified")
	}

	if len(dlpResults) == 0 {
		return fmt.Errorf("no PII detected in test content")
	}

	return nil
}

// Main function for running tests programmatically
func RunIntegrationTests() {
	runner := NewTestRunner()

	fmt.Println("Setting up test environment...")
	if err := runner.Setup(); err != nil {
		log.Fatalf("Failed to setup test environment: %v", err)
	}

	fmt.Println("Test environment ready.")

	runner.RunAllTests()

	// Print detailed results
	fmt.Println("\nDetailed Results:")
	fmt.Println("================")

	for name, result := range runner.GetResults() {
		status := "PASS"
		if !result.Success {
			status = "FAIL"
		}

		fmt.Printf("%-20s: %s (%v)\n", name, status, result.Duration)
		if result.Error != nil {
			fmt.Printf("    Error: %v\n", result.Error)
		}
	}
}

// Check if we're being run as a standalone program
func init() {
	if len(os.Args) > 1 && os.Args[1] == "run-tests" {
		RunIntegrationTests()
		os.Exit(0)
	}
}
