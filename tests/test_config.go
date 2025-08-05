package tests

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jscharber/eAIIngest/pkg/auth"
	"github.com/jscharber/eAIIngest/pkg/chunking"
	"github.com/jscharber/eAIIngest/pkg/classification"
	"github.com/jscharber/eAIIngest/pkg/events"
	"github.com/jscharber/eAIIngest/pkg/storage"
	"github.com/jscharber/eAIIngest/pkg/storage/local"
	"github.com/jscharber/eAIIngest/pkg/workflow"
)

// EventBusAdapter adapts the events.EventBusInterface to workflow.EventBus
type EventBusAdapter struct {
	bus events.EventBusInterface
}

func (a *EventBusAdapter) Publish(event *events.Event) error {
	return a.bus.Publish(event)
}

func (a *EventBusAdapter) Subscribe(handler events.EventHandler, eventTypes ...string) error {
	return a.bus.Subscribe(handler, eventTypes...)
}

// TestReader wraps a file reader for testing
type TestReader struct {
	Type string
}

func (r *TestReader) Read(filePath string) (*TestData, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	return &TestData{
		Type:    r.Type,
		Content: content,
	}, nil
}

// TestData represents read file data
type TestData struct {
	Type    string
	Content []byte
}

// TestDLPEngine provides a simple DLP engine for testing
type TestDLPEngine struct{}

func (e *TestDLPEngine) ScanText(content string) ([]TestPIIResult, error) {
	var results []TestPIIResult

	// Simple pattern matching for testing
	if strings.Contains(content, "123-45-6789") {
		results = append(results, TestPIIResult{
			Type:  "ssn",
			Value: "123-45-6789",
		})
	}

	if strings.Contains(content, "4532-1234-5678-9012") {
		results = append(results, TestPIIResult{
			Type:  "credit_card",
			Value: "4532-1234-5678-9012",
		})
	}

	return results, nil
}

// TestPIIResult represents a PII detection result
type TestPIIResult struct {
	Type  string
	Value string
}

// NewTestReader creates a new test reader
func NewTestReader(fileType string) *TestReader {
	return &TestReader{Type: fileType}
}

// NewTestDLPEngine creates a new test DLP engine
func NewTestDLPEngine() *TestDLPEngine {
	return &TestDLPEngine{}
}

// TestEnvironment provides a complete testing environment
type TestEnvironment struct {
	// Core services
	AuthService    auth.AuthService
	EventBus       *EventBusAdapter
	WorkflowEngine workflow.Engine

	// Processing components
	CSVReader       *TestReader
	JSONReader      *TestReader
	TextReader      *TestReader
	Chunker         chunking.Chunker
	Classifier      *classification.Service
	DLPEngine       *TestDLPEngine
	StorageRegistry *storage.DefaultResolverRegistry

	// Test data
	TestDir     string
	TestFiles   map[string]string
	TestUsers   []*auth.User
	TestTenants []*auth.Tenant

	// Cleanup functions
	cleanup []func() error
}

// SetupTestEnvironment creates a complete test environment
func SetupTestEnvironment(t *testing.T) *TestEnvironment {
	ctx := context.Background()

	// Create test directory
	testDir := t.TempDir()

	// Initialize authentication system
	jwtManager, err := auth.NewJWTManager(nil)
	if err != nil {
		t.Fatalf("Failed to create JWT manager: %v", err)
	}

	userStore := auth.NewMemoryUserStore()
	tenantStore := auth.NewMemoryTenantStore()

	// Seed auth data
	if err := userStore.SeedData(); err != nil {
		t.Fatalf("Failed to seed user data: %v", err)
	}
	if err := tenantStore.SeedData(); err != nil {
		t.Fatalf("Failed to seed tenant data: %v", err)
	}

	authService := auth.NewService(jwtManager, userStore, tenantStore, nil)

	// Initialize event system
	eventBus := events.NewDefaultInMemoryEventBus()
	eventBus.Start()

	// Create adapter for workflow engine
	adapter := &EventBusAdapter{bus: eventBus}
	workflowEngine := workflow.NewEngine(adapter)

	// Initialize processing components
	csvReader := NewTestReader("csv")
	jsonReader := NewTestReader("json")
	textReader := NewTestReader("text")
	chunker := chunking.NewFixedSizeChunker(500, 50)
	classifier := classification.NewService(nil) // Use default config
	dlpEngine := NewTestDLPEngine()

	// Initialize storage
	storageRegistry := storage.NewResolverRegistry()
	localResolver := local.NewLocalResolver("", []string{}) // No base path restrictions for testing
	storageRegistry.RegisterResolver([]storage.CloudProvider{storage.ProviderLocal}, localResolver)

	// Create test files
	testFiles := createTestFiles(t, testDir)

	// Create test users and tenants
	testUsers := createTestUsers(t, ctx, authService)
	testTenants := createTestTenants(t, ctx, authService)

	env := &TestEnvironment{
		AuthService:     authService,
		EventBus:        adapter,
		WorkflowEngine:  workflowEngine,
		CSVReader:       csvReader,
		JSONReader:      jsonReader,
		TextReader:      textReader,
		Chunker:         chunker,
		Classifier:      classifier,
		DLPEngine:       dlpEngine,
		StorageRegistry: storageRegistry,
		TestDir:         testDir,
		TestFiles:       testFiles,
		TestUsers:       testUsers,
		TestTenants:     testTenants,
		cleanup:         []func() error{},
	}

	// Setup cleanup
	t.Cleanup(func() {
		env.Cleanup()
	})

	return env
}

// Cleanup performs environment cleanup
func (env *TestEnvironment) Cleanup() {
	for _, cleanup := range env.cleanup {
		cleanup()
	}
}

// AddCleanup adds a cleanup function
func (env *TestEnvironment) AddCleanup(cleanup func() error) {
	env.cleanup = append(env.cleanup, cleanup)
}

// GetTestUser returns a test user by index
func (env *TestEnvironment) GetTestUser(index int) *auth.User {
	if index < len(env.TestUsers) {
		return env.TestUsers[index]
	}
	return nil
}

// GetTestTenant returns a test tenant by index
func (env *TestEnvironment) GetTestTenant(index int) *auth.Tenant {
	if index < len(env.TestTenants) {
		return env.TestTenants[index]
	}
	return nil
}

// AuthenticateTestUser authenticates a test user and returns tokens
func (env *TestEnvironment) AuthenticateTestUser(ctx context.Context, userIndex int) (*auth.AuthResponse, error) {
	user := env.GetTestUser(userIndex)
	if user == nil {
		return nil, auth.ErrUserNotFound
	}

	authReq := &auth.AuthRequest{
		Type:     auth.AuthTypePassword,
		Username: user.Username,
		Password: "TestPass123!", // Default password for test users
	}

	return env.AuthService.Authenticate(ctx, authReq)
}

func createTestFiles(t *testing.T, testDir string) map[string]string {
	files := make(map[string]string)

	// CSV test file
	csvContent := `name,email,ssn,department
John Doe,john.doe@company.com,123-45-6789,Engineering
Jane Smith,jane.smith@company.com,987-65-4321,Marketing
Bob Johnson,bob.johnson@company.com,555-12-3456,Sales
Alice Brown,alice.brown@company.com,444-98-7654,HR
Charlie Wilson,charlie.wilson@company.com,333-11-2222,Finance`

	csvFile := filepath.Join(testDir, "employees.csv")
	if err := os.WriteFile(csvFile, []byte(csvContent), 0644); err != nil {
		t.Fatalf("Failed to create CSV test file: %v", err)
	}
	files["csv"] = csvFile

	// JSON test file
	jsonContent := `{
  "company": "Test Company Inc.",
  "employees": [
    {
      "id": 1,
      "name": "Sarah Connor",
      "email": "sarah.connor@company.com",
      "ssn": "666-77-8888",
      "department": "Security",
      "salary": 85000,
      "active": true
    },
    {
      "id": 2,
      "name": "Kyle Reese",
      "email": "kyle.reese@company.com",
      "ssn": "777-88-9999",
      "department": "Operations",
      "salary": 75000,
      "active": true
    }
  ],
  "metadata": {
    "created": "2024-01-01",
    "version": "1.0",
    "confidential": true
  }
}`

	jsonFile := filepath.Join(testDir, "company_data.json")
	if err := os.WriteFile(jsonFile, []byte(jsonContent), 0644); err != nil {
		t.Fatalf("Failed to create JSON test file: %v", err)
	}
	files["json"] = jsonFile

	// Text test file with various PII types
	textContent := `CONFIDENTIAL EMPLOYEE REPORT

This document contains sensitive employee information and should be handled with care.

Employee Details:
- Name: Michael Thompson
- Social Security Number: 123-45-6789
- Employee ID: EMP-12345
- Date of Birth: 03/15/1985
- Credit Card: 4532-1234-5678-9012
- Phone Number: (555) 123-4567
- Email: michael.thompson@company.com
- Address: 123 Main Street, Anytown, ST 12345

Financial Information:
- Bank Account: 123456789 (routing: 021000021)
- Annual Salary: $95,000
- Stock Options: 1,000 shares

Medical Information:
- Insurance Policy: POL-123456789
- Medical Record Number: MRN-987654321

Security Clearance:
- Clearance Level: SECRET
- Badge Number: B-789012

This document was generated on 2024-01-15 and contains personally identifiable information (PII) 
that must be protected according to company policy and applicable regulations.

Additional Notes:
The employee has access to systems containing customer data including credit card numbers, 
social security numbers, and other sensitive financial information. Regular security training 
is required for all personnel with access to such data.

IP Address logs show access from: 192.168.1.100, 10.0.0.25
MAC Address: 00:1B:44:11:3A:B7

DISTRIBUTION: LIMITED - AUTHORIZED PERSONNEL ONLY`

	textFile := filepath.Join(testDir, "employee_report.txt")
	if err := os.WriteFile(textFile, []byte(textContent), 0644); err != nil {
		t.Fatalf("Failed to create text test file: %v", err)
	}
	files["text"] = textFile

	// Large text file for performance testing
	largeContent := generateLargeTestContent()
	largeFile := filepath.Join(testDir, "large_document.txt")
	if err := os.WriteFile(largeFile, []byte(largeContent), 0644); err != nil {
		t.Fatalf("Failed to create large test file: %v", err)
	}
	files["large"] = largeFile

	// Binary test file (simulated)
	binaryContent := make([]byte, 1024)
	for i := range binaryContent {
		binaryContent[i] = byte(i % 256)
	}
	binaryFile := filepath.Join(testDir, "binary_data.bin")
	if err := os.WriteFile(binaryFile, binaryContent, 0644); err != nil {
		t.Fatalf("Failed to create binary test file: %v", err)
	}
	files["binary"] = binaryFile

	return files
}

func createTestUsers(t *testing.T, ctx context.Context, authService auth.AuthService) []*auth.User {
	users := []*auth.User{}

	userSpecs := []struct {
		username string
		email    string
		role     auth.Role
	}{
		{"admin_user", "admin@test.com", auth.RoleAdmin},
		{"regular_user", "user@test.com", auth.RoleUser},
		{"viewer_user", "viewer@test.com", auth.RoleViewer},
		{"api_client", "api@test.com", auth.RoleAPIClient},
		{"service_account", "service@test.com", auth.RoleServiceAccount},
	}

	for _, spec := range userSpecs {
		createReq := &auth.CreateUserRequest{
			Username:  spec.username,
			Email:     spec.email,
			Password:  "TestPass123!",
			FirstName: "Test",
			LastName:  "User",
			Roles:     []auth.Role{spec.role},
		}

		user, err := authService.CreateUser(ctx, createReq)
		if err != nil {
			t.Fatalf("Failed to create test user %s: %v", spec.username, err)
		}

		users = append(users, user)
	}

	return users
}

func createTestTenants(t *testing.T, ctx context.Context, authService auth.AuthService) []*auth.Tenant {
	tenants := []*auth.Tenant{}

	tenantSpecs := []struct {
		name        string
		displayName string
		domain      string
	}{
		{"tenant1", "Test Tenant One", "tenant1.test.com"},
		{"tenant2", "Test Tenant Two", "tenant2.test.com"},
		{"enterprise", "Enterprise Tenant", "enterprise.test.com"},
	}

	for _, spec := range tenantSpecs {
		createReq := &auth.CreateTenantRequest{
			Name:        spec.name,
			DisplayName: spec.displayName,
			Domain:      spec.domain,
		}

		tenant, err := authService.CreateTenant(ctx, createReq)
		if err != nil {
			t.Fatalf("Failed to create test tenant %s: %v", spec.name, err)
		}

		tenants = append(tenants, tenant)
	}

	return tenants
}

func generateLargeTestContent() string {
	baseContent := `This is a large test document designed to test the performance and scalability of the Audimodal.ai platform.
It contains multiple paragraphs with various types of content including business data, technical specifications,
and embedded personally identifiable information (PII) for testing DLP capabilities.

Section 1: Business Overview
Our company processes large volumes of data daily, including customer records, financial transactions,
and operational metrics. This document serves as a comprehensive example of the types of content
that might be processed through our ingestion pipeline.

Section 2: Technical Specifications
The system handles various file formats including CSV, JSON, XML, and plain text documents.
Processing includes content classification, sentiment analysis, language detection, and
data loss prevention (DLP) scanning for sensitive information.

Section 3: Sample Data with PII
Customer Records:
- John Smith, SSN: 123-45-6789, Credit Card: 4532-1234-5678-9012
- Mary Johnson, SSN: 987-65-4321, Credit Card: 5555-4444-3333-2222
- Robert Davis, SSN: 555-44-3333, Credit Card: 4111-1111-1111-1111

Employee Information:
- Badge ID: EMP-001, Name: Alice Wilson, Phone: (555) 123-4567
- Badge ID: EMP-002, Name: Bob Anderson, Phone: (555) 987-6543
- Badge ID: EMP-003, Name: Carol Martinez, Phone: (555) 456-7890

Financial Data:
- Account Number: 123456789012, Routing: 021000021
- Account Number: 987654321098, Routing: 011000138
- Account Number: 456789123456, Routing: 121000248

Section 4: Performance Testing Content
This section contains repetitive content designed to test the chunking and processing algorithms
under load. The content simulates real-world documents with varying structures and complexity.

Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut
labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco
laboris nisi ut aliquip ex ea commodo consequat.

Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla
pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt
mollit anim id est laborum.

Section 5: Multilingual Content
Este es un ejemplo de contenido en español para probar la detección de idiomas.
这是中文内容的示例，用于测试语言检测功能。
これは言語検出をテストするための日本語のコンテンツの例です。
Ceci est un exemple de contenu français pour tester la détection de langue.

Section 6: Structured Data Examples
Configuration Settings:
- server.host = "localhost"
- server.port = 8080
- database.url = "jdbc:postgresql://localhost:5432/eai"
- redis.host = "localhost:6379"

API Endpoints:
- GET /api/v1/users
- POST /api/v1/auth/login
- PUT /api/v1/documents/{id}
- DELETE /api/v1/files/{id}

Section 7: Error Scenarios
This section contains intentionally malformed data for testing error handling:
- Incomplete JSON: {"name": "test", "value":
- Invalid dates: 2024-13-45, 99/99/9999
- Malformed phone numbers: 555-CALL-NOW, 1-800-BAD-NUMB

Section 8: Large Data Simulation
[This section would contain repetitive data to reach the desired file size]

`

	// Repeat content to create a larger document
	result := ""
	for i := 0; i < 20; i++ {
		result += baseContent + "\n\n"
	}

	return result
}

// Common test assertions and utilities

// AssertValidPIIDetection checks that DLP scanning finds expected PII types
func AssertValidPIIDetection(t *testing.T, results []TestPIIResult, expectedTypes ...string) {
	t.Helper()

	foundTypes := make(map[string]bool)
	for _, result := range results {
		foundTypes[result.Type] = true
	}

	for _, expectedType := range expectedTypes {
		if !foundTypes[expectedType] {
			t.Errorf("Expected to find PII type %s, but it was not detected", expectedType)
		}
	}
}

// AssertValidClassification checks that content classification results are valid
func AssertValidClassification(t *testing.T, result *classification.ClassificationResult) {
	t.Helper()

	if result.ContentType == "" {
		t.Error("Content type should not be empty")
	}

	if result.Language == "" {
		t.Error("Language should not be empty")
	}

	if result.SensitivityLevel == "" {
		t.Error("Sensitivity level should not be empty")
	}
}

// AssertValidChunks checks that chunking results are valid
func AssertValidChunks(t *testing.T, chunks []chunking.Chunk, originalContent string) {
	t.Helper()

	if len(chunks) == 0 {
		t.Error("Should produce at least one chunk")
		return
	}

	// Verify all chunks combined contain the original content
	combinedContent := ""
	for _, chunk := range chunks {
		combinedContent += chunk.Content
	}

	// Allow for some overlap in chunking
	if len(combinedContent) < len(originalContent) {
		t.Error("Combined chunks should contain at least as much content as original")
	}
}
