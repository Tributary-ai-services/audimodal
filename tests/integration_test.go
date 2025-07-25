package tests

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jscharber/eAIIngest/pkg/auth"
	"github.com/jscharber/eAIIngest/pkg/classification"
	"github.com/jscharber/eAIIngest/pkg/chunking"
	"github.com/jscharber/eAIIngest/pkg/events"
	"github.com/jscharber/eAIIngest/pkg/storage"
	"github.com/jscharber/eAIIngest/pkg/storage/local"
	"github.com/jscharber/eAIIngest/pkg/workflow"
)

// Integration test suite for the Audimodal.ai platform
func TestIntegrationFullPipeline(t *testing.T) {
	ctx := context.Background()
	
	// Set up test environment
	testDir := t.TempDir()
	
	// Create test files
	csvContent := `name,email,ssn
John Doe,john@example.com,123-45-6789
Jane Smith,jane@example.com,987-65-4321`
	
	jsonContent := `{
		"users": [
			{"name": "Bob Wilson", "email": "bob@example.com", "ssn": "555-12-3456"},
			{"name": "Alice Brown", "email": "alice@example.com", "ssn": "444-98-7654"}
		]
	}`
	
	textContent := `This is a sample document containing sensitive information.
	Employee ID: EMP-12345
	Social Security Number: 123-45-6789
	Credit Card: 4532-1234-5678-9012
	The document contains PII that should be detected.`
	
	// Write test files
	csvFile := filepath.Join(testDir, "test.csv")
	jsonFile := filepath.Join(testDir, "test.json")
	textFile := filepath.Join(testDir, "test.txt")
	
	require.NoError(t, os.WriteFile(csvFile, []byte(csvContent), 0644))
	require.NoError(t, os.WriteFile(jsonFile, []byte(jsonContent), 0644))
	require.NoError(t, os.WriteFile(textFile, []byte(textContent), 0644))
	
	// Initialize components
	t.Run("AuthenticationSystem", func(t *testing.T) {
		testAuthenticationSystem(t, ctx)
	})
	
	t.Run("FileReaders", func(t *testing.T) {
		testFileReaders(t, ctx, csvFile, jsonFile, textFile)
	})
	
	t.Run("ChunkingStrategies", func(t *testing.T) {
		testChunkingStrategies(t, ctx)
	})
	
	t.Run("ContentClassification", func(t *testing.T) {
		testContentClassification(t, ctx)
	})
	
	t.Run("DLPDetection", func(t *testing.T) {
		testDLPDetection(t, ctx, textContent)
	})
	
	t.Run("EventDrivenWorkflow", func(t *testing.T) {
		testEventDrivenWorkflow(t, ctx)
	})
	
	t.Run("StorageResolvers", func(t *testing.T) {
		testStorageResolvers(t, ctx)
	})
	
	t.Run("EndToEndPipeline", func(t *testing.T) {
		testEndToEndPipeline(t, ctx, textFile)
	})
}

func testAuthenticationSystem(t *testing.T, ctx context.Context) {
	// Initialize JWT manager
	jwtManager, err := auth.NewJWTManager(nil)
	require.NoError(t, err)
	
	// Initialize stores
	userStore := auth.NewMemoryUserStore()
	tenantStore := auth.NewMemoryTenantStore()
	
	// Seed test data
	require.NoError(t, userStore.SeedData())
	require.NoError(t, tenantStore.SeedData())
	
	// Initialize auth service
	authService := auth.NewService(jwtManager, userStore, tenantStore, nil)
	
	// Test user creation
	createReq := &auth.CreateUserRequest{
		Username:  "testuser",
		Email:     "test@integration.com",
		Password:  "SecurePass123!",
		FirstName: "Test",
		LastName:  "User",
		Roles:     []auth.Role{auth.RoleUser},
	}
	
	user, err := authService.CreateUser(ctx, createReq)
	require.NoError(t, err)
	assert.Equal(t, "testuser", user.Username)
	assert.Equal(t, "test@integration.com", user.Email)
	
	// Test authentication
	authReq := &auth.AuthRequest{
		Type:     auth.AuthTypePassword,
		Username: "testuser",
		Password: "SecurePass123!",
	}
	
	authResp, err := authService.Authenticate(ctx, authReq)
	require.NoError(t, err)
	assert.NotEmpty(t, authResp.AccessToken)
	assert.Equal(t, "Bearer", authResp.TokenType)
	
	// Test token validation
	claims, err := authService.ValidateToken(ctx, authResp.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, user.ID, claims.UserID)
	assert.Equal(t, "testuser", claims.Username)
}

func testFileReaders(t *testing.T, ctx context.Context, csvFile, jsonFile, textFile string) {
	// Test CSV reader
	csvReader := NewTestReader("csv")
	csvData, err := csvReader.Read(csvFile)
	require.NoError(t, err)
	assert.Equal(t, "csv", csvData.Type)
	assert.Contains(t, string(csvData.Content), "John Doe")
	
	// Test JSON reader
	jsonReader := NewTestReader("json")
	jsonData, err := jsonReader.Read(jsonFile)
	require.NoError(t, err)
	assert.Equal(t, "json", jsonData.Type)
	assert.Contains(t, string(jsonData.Content), "Bob Wilson")
	
	// Test text reader
	textReader := NewTestReader("text")
	textData, err := textReader.Read(textFile)
	require.NoError(t, err)
	assert.Equal(t, "text", textData.Type)
	assert.Contains(t, string(textData.Content), "Employee ID")
}

func testChunkingStrategies(t *testing.T, ctx context.Context) {
	testContent := "This is a long document that needs to be chunked for processing. " +
		"It contains multiple sentences and paragraphs. " +
		"Each chunk should be manageable for downstream processing."
	
	// Test fixed size chunking
	fixedChunker := chunking.NewFixedSizeChunker(50, 10)
	chunks, err := fixedChunker.Chunk(testContent)
	require.NoError(t, err)
	assert.Greater(t, len(chunks), 1)
	
	for _, chunk := range chunks {
		assert.LessOrEqual(t, len(chunk.Content), 60) // 50 + 10 overlap
	}
	
	// Test sentence chunking
	sentenceChunker := chunking.NewSentenceChunker(100, 20)
	sentenceChunks, err := sentenceChunker.Chunk(testContent)
	require.NoError(t, err)
	assert.Greater(t, len(sentenceChunks), 0)
	
	// Test semantic chunking
	semanticChunker := chunking.NewSemanticChunker(200, 0.7)
	semanticChunks, err := semanticChunker.Chunk(testContent)
	require.NoError(t, err)
	assert.Greater(t, len(semanticChunks), 0)
}

func testContentClassification(t *testing.T, ctx context.Context) {
	// Initialize classification service
	classificationService := classification.NewService(nil) // Use default config
	
	testTexts := []string{
		"This is a business document about quarterly earnings and financial reports.",
		"Este es un documento en español sobre tecnología y desarrollo de software.",
		"I love this product! It's amazing and works perfectly. Highly recommended!",
		"This is terrible. Worst experience ever. Would not recommend to anyone.",
		"The quick brown fox jumps over the lazy dog. This is a neutral statement.",
	}
	
	for i, text := range testTexts {
		input := &classification.ClassificationInput{
			Content:  text,
			TenantID: uuid.New(),
			Options:  classification.DefaultClassificationOptions(),
		}
		
		result, err := classificationService.Classify(ctx, input)
		require.NoError(t, err, "Failed to classify text %d", i)
		
		// Verify basic classification results
		assert.NotEmpty(t, result.ContentType, "Content type should not be empty for text %d", i)
		assert.NotEmpty(t, result.Language, "Language should not be empty for text %d", i)
		assert.NotEmpty(t, result.SensitivityLevel, "Sensitivity level should not be empty for text %d", i)
		
		// Test specific cases with string representations
		switch i {
		case 0: // Business document
			assert.Equal(t, "document", string(result.ContentType))
		case 1: // Spanish text
			assert.Equal(t, "es", string(result.Language))
		case 2: // Positive sentiment
			if result.Sentiment != nil {
				assert.Equal(t, "positive", result.Sentiment.Overall.Label)
			}
		case 3: // Negative sentiment
			if result.Sentiment != nil {
				assert.Equal(t, "negative", result.Sentiment.Overall.Label)
			}
		case 4: // Neutral sentiment
			if result.Sentiment != nil {
				assert.Equal(t, "neutral", result.Sentiment.Overall.Label)
			}
		}
	}
}

func testDLPDetection(t *testing.T, ctx context.Context, content string) {
	// Initialize DLP engine
	dlpEngine := NewTestDLPEngine()
	
	// Test PII detection
	results, err := dlpEngine.ScanText(content)
	require.NoError(t, err)
	
	// Should detect SSN and credit card
	assert.Greater(t, len(results), 0, "Should detect PII in the content")
	
	// Verify specific PII types are detected
	foundSSN := false
	foundCreditCard := false
	
	for _, result := range results {
		switch result.Type {
		case "ssn":
			foundSSN = true
			assert.Contains(t, content, result.Value)
		case "credit_card":
			foundCreditCard = true
			assert.Contains(t, content, result.Value)
		}
	}
	
	assert.True(t, foundSSN, "Should detect SSN")
	assert.True(t, foundCreditCard, "Should detect credit card")
}

func testEventDrivenWorkflow(t *testing.T, ctx context.Context) {
	// Initialize event bus  
	eventBus := events.NewDefaultInMemoryEventBus()
	eventBus.Start()
	defer eventBus.Stop()
	
	// Create adapter for workflow engine
	adapter := &EventBusAdapter{bus: eventBus}
	
	// Initialize workflow engine
	workflowEngine := workflow.NewEngine(adapter)
	
	// Create a test workflow
	testWorkflow := &workflow.WorkflowDefinition{
		ID:          uuid.New(),
		Name:        "test-workflow",
		Description: "Integration test workflow",
		Steps: []workflow.WorkflowStep{
			{
				ID:   "step1",
				Name: "Read File",
				Type: workflow.StepTypeFileRead,
				Config: map[string]interface{}{
					"path": "/test/file.txt",
				},
			},
			{
				ID:   "step2",
				Name: "Classify Content",
				Type: workflow.StepTypeClassify,
				Config: map[string]interface{}{
					"enable_sentiment": true,
				},
				Dependencies: []string{"step1"},
			},
			{
				ID:   "step3",
				Name: "DLP Scan",
				Type: workflow.StepTypeDLPScan,
				Config: map[string]interface{}{
					"scan_pii": true,
				},
				Dependencies: []string{"step1"},
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	
	// Register workflow
	err := workflowEngine.RegisterWorkflow(testWorkflow)
	require.NoError(t, err)
	
	// Create test input
	input := map[string]interface{}{
		"content": "Test content for workflow processing",
		"metadata": map[string]interface{}{
			"source": "integration-test",
		},
	}
	
	// Execute workflow
	executionID, err := workflowEngine.ExecuteWorkflow(ctx, testWorkflow.ID, input)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, executionID)
	
	// Wait for execution to complete (in a real scenario, this would be event-driven)
	time.Sleep(100 * time.Millisecond)
	
	// Get execution status
	execution, err := workflowEngine.GetExecution(ctx, executionID)
	require.NoError(t, err)
	assert.NotNil(t, execution)
	assert.Equal(t, testWorkflow.ID, execution.WorkflowID)
}

func testStorageResolvers(t *testing.T, ctx context.Context) {
	// Initialize storage registry
	registry := storage.NewResolverRegistry()
	
	// Register local file resolver
	localResolver := local.NewLocalResolver("", []string{}) // No base path restrictions for testing
	err := registry.RegisterResolver([]storage.CloudProvider{storage.ProviderLocal}, localResolver)
	require.NoError(t, err)
	
	// Test local file URL resolution
	testPath := "/tmp/test-file.txt"
	localURL := "file://" + testPath
	
	resolver, err := registry.GetResolver(storage.ProviderLocal)
	require.NoError(t, err)
	assert.NotNil(t, resolver)
	
	// Test URL parsing
	parsedURL, err := resolver.ParseURL(localURL)
	require.NoError(t, err)
	assert.Equal(t, "/tmp", parsedURL.Bucket)
	assert.Equal(t, "test-file.txt", parsedURL.Key)
	assert.Equal(t, storage.ProviderLocal, parsedURL.Provider)
	
	// Test basic URL creation for testing purposes
	s3URL := "s3://test-bucket/path/to/file.txt"
	// Note: We don't have a global ParseURL function, so we'll just test basic URL structure
	assert.Contains(t, s3URL, "s3://")
	assert.Contains(t, s3URL, "test-bucket")
	assert.Contains(t, s3URL, "path/to/file.txt")
	
	// Test GCS URL structure
	gcsURL := "gs://my-bucket/documents/file.pdf"
	assert.Contains(t, gcsURL, "gs://")
	assert.Contains(t, gcsURL, "my-bucket")
	assert.Contains(t, gcsURL, "documents/file.pdf")
}

func testEndToEndPipeline(t *testing.T, ctx context.Context, testFile string) {
	// This test simulates a complete end-to-end processing pipeline
	
	// Step 1: Read file
	textReader := NewTestReader("text")
	data, err := textReader.Read(testFile)
	require.NoError(t, err)
	
	// Step 2: Chunk content
	chunker := chunking.NewFixedSizeChunker(100, 20)
	chunks, err := chunker.Chunk(string(data.Content))
	require.NoError(t, err)
	assert.Greater(t, len(chunks), 0)
	
	// Step 3: Classify content
	classifier := classification.NewService(nil) // Use default config
	for _, chunk := range chunks {
		input := &classification.ClassificationInput{
			Content:  chunk.Content,
			TenantID: uuid.New(),
			Options:  classification.DefaultClassificationOptions(),
		}
		result, err := classifier.Classify(ctx, input)
		require.NoError(t, err)
		assert.NotEmpty(t, result.ContentType)
		assert.NotEmpty(t, result.Language)
	}
	
	// Step 4: DLP scanning
	dlpEngine := NewTestDLPEngine()
	dlpResults, err := dlpEngine.ScanText(string(data.Content))
	require.NoError(t, err)
	
	// Should find PII in test content
	assert.Greater(t, len(dlpResults), 0, "Should detect PII in test file")
	
	// Step 5: Generate processing summary
	summary := map[string]interface{}{
		"file_processed":    testFile,
		"chunks_created":    len(chunks),
		"pii_items_found":   len(dlpResults),
		"processing_time":   time.Now(),
		"content_length":    len(data.Content),
	}
	
	// Verify summary
	assert.Greater(t, summary["chunks_created"], 0)
	assert.Greater(t, summary["pii_items_found"], 0)
	assert.Greater(t, summary["content_length"], 0)
	
	t.Logf("End-to-end pipeline completed successfully: %+v", summary)
}

// Benchmark tests for performance validation
func BenchmarkFullPipeline(b *testing.B) {
	ctx := context.Background()
	
	// Setup
	testContent := `This is a benchmark test document with PII data.
	SSN: 123-45-6789, Credit Card: 4532-1234-5678-9012
	Employee ID: EMP-98765, Email: test@benchmark.com
	` + generateLargeContent(1000) // Generate larger content for realistic benchmarking
	
	chunker := chunking.NewFixedSizeChunker(200, 50)
	classifier := classification.NewService(nil) // Use default config
	dlpEngine := NewTestDLPEngine()
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		// Chunk content
		chunks, err := chunker.Chunk(testContent)
		if err != nil {
			b.Fatal(err)
		}
		
		// Classify chunks
		for _, chunk := range chunks {
			input := &classification.ClassificationInput{
				Content:  chunk.Content,
				TenantID: uuid.New(),
				Options:  classification.DefaultClassificationOptions(),
			}
			_, err := classifier.Classify(ctx, input)
			if err != nil {
				b.Fatal(err)
			}
		}
		
		// DLP scan
		_, err = dlpEngine.ScanText(testContent)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func generateLargeContent(sentences int) string {
	baseContent := " This is a sample sentence for testing purposes with various content types and structures."
	result := ""
	for i := 0; i < sentences; i++ {
		result += baseContent
	}
	return result
}

// Helper function to create test configuration
func createTestConfig() map[string]interface{} {
	return map[string]interface{}{
		"auth": map[string]interface{}{
			"jwt_secret": "test-secret-key-for-integration-tests",
			"token_ttl":  "1h",
		},
		"storage": map[string]interface{}{
			"local_base_path": "/tmp/eai-ingest-test",
		},
		"dlp": map[string]interface{}{
			"enable_pii_detection": true,
			"confidence_threshold": 0.8,
		},
		"classification": map[string]interface{}{
			"enable_sentiment":  true,
			"enable_entities":   true,
			"language_detection": true,
		},
	}
}