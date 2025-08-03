package tests

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jscharber/eAIIngest/pkg/auth"
	"github.com/jscharber/eAIIngest/pkg/chunking"
	"github.com/jscharber/eAIIngest/pkg/classification"
	"github.com/jscharber/eAIIngest/pkg/events"
	"github.com/jscharber/eAIIngest/pkg/workflow"
)

// Performance benchmarks for various components
func BenchmarkAuthenticationPerformance(b *testing.B) {
	ctx := context.Background()

	// Setup
	jwtManager, err := auth.NewJWTManager(nil)
	require.NoError(b, err)

	userStore := auth.NewMemoryUserStore()
	tenantStore := auth.NewMemoryTenantStore()
	require.NoError(b, userStore.SeedData())
	require.NoError(b, tenantStore.SeedData())

	authService := auth.NewService(jwtManager, userStore, tenantStore, nil)

	// Create test user
	createReq := &auth.CreateUserRequest{
		Username: "bench_user",
		Email:    "bench@test.com",
		Password: "BenchPass123!",
		Roles:    []auth.Role{auth.RoleUser},
	}

	user, err := authService.CreateUser(ctx, createReq)
	require.NoError(b, err)

	// Get auth response for token validation benchmarks
	authReq := &auth.AuthRequest{
		Type:     auth.AuthTypePassword,
		Username: "bench_user",
		Password: "BenchPass123!",
	}

	authResp, err := authService.Authenticate(ctx, authReq)
	require.NoError(b, err)

	b.Run("Authentication", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := authService.Authenticate(ctx, authReq)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("TokenValidation", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := authService.ValidateToken(ctx, authResp.AccessToken)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("PermissionCheck", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := authService.CheckPermission(ctx, user.ID, auth.PermissionDataRead)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("UserCreation", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			createReq := &auth.CreateUserRequest{
				Username: fmt.Sprintf("bench_user_%d", i),
				Email:    fmt.Sprintf("bench_%d@test.com", i),
				Password: "BenchPass123!",
				Roles:    []auth.Role{auth.RoleUser},
			}
			_, err := authService.CreateUser(ctx, createReq)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkChunkingPerformance(b *testing.B) {
	// Generate test content of various sizes
	testContent := generateTestContent(10000) // 10KB of text

	chunkers := map[string]chunking.Chunker{
		"FixedSize": chunking.NewFixedSizeChunker(500, 50),
		"Sentence":  chunking.NewSentenceChunker(1000, 100),
		"Semantic":  chunking.NewSemanticChunker(800, 0.7),
	}

	for name, chunker := range chunkers {
		b.Run(name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := chunker.Chunk(testContent)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}

	// Test with different content sizes
	contentSizes := []int{1000, 10000, 100000} // 1KB, 10KB, 100KB
	chunker := chunking.NewFixedSizeChunker(500, 50)

	for _, size := range contentSizes {
		b.Run(fmt.Sprintf("FixedSize_%dB", size), func(b *testing.B) {
			content := generateTestContent(size)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := chunker.Chunk(content)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkClassificationPerformance(b *testing.B) {
	ctx := context.Background()
	classifier := classification.NewService(&classification.ServiceConfig{})

	testTexts := []string{
		"This is a short business document.",
		generateTestContent(1000),  // 1KB
		generateTestContent(10000), // 10KB
		generateTestContent(50000), // 50KB
	}

	for i, text := range testTexts {
		b.Run(fmt.Sprintf("Text_%d", i), func(b *testing.B) {
			b.ResetTimer()
			for j := 0; j < b.N; j++ {
				input := &classification.ClassificationInput{
					Content:  text,
					TenantID: uuid.New(),
				}
				_, err := classifier.Classify(ctx, input)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}

	// Concurrent classification benchmark
	b.Run("ConcurrentClassification", func(b *testing.B) {
		text := generateTestContent(5000)
		concurrency := runtime.NumCPU()

		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				input := &classification.ClassificationInput{
					Content:  text,
					TenantID: uuid.New(),
				}
				_, err := classifier.Classify(ctx, input)
				if err != nil {
					b.Fatal(err)
				}
			}
		})

		b.ReportMetric(float64(concurrency), "goroutines")
	})
}

func BenchmarkDLPPerformance(b *testing.B) {
	dlpEngine := NewTestDLPEngine()

	// Generate content with embedded PII
	testContent := generateContentWithPII(10000)

	b.Run("DLPScan", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := dlpEngine.ScanText(testContent)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	// Test with different content sizes
	contentSizes := []int{1000, 10000, 100000}

	for _, size := range contentSizes {
		b.Run(fmt.Sprintf("DLPScan_%dB", size), func(b *testing.B) {
			content := generateContentWithPII(size)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := dlpEngine.ScanText(content)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}

	// Concurrent DLP scanning
	b.Run("ConcurrentDLPScan", func(b *testing.B) {
		content := generateContentWithPII(5000)

		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_, err := dlpEngine.ScanText(content)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	})
}

func BenchmarkEventSystemPerformance(b *testing.B) {
	eventBus := events.NewInMemoryEventBus(events.EventBusConfig{})

	// Setup event handler
	handlerFunc := func(ctx context.Context, event interface{}) error {
		// Simulate some processing
		time.Sleep(1 * time.Microsecond)
		return nil
	}

	handler := &events.FunctionHandler{
		HandlerFunc: handlerFunc,
	}

	eventBus.Subscribe(handler, "file_processed")

	b.Run("EventPublish", func(b *testing.B) {
		event := &events.Event{
			ID:       uuid.New(),
			Type:     "file_processed",
			TenantID: uuid.New(),
			Payload: map[string]interface{}{
				"file_path": "/test/file.txt",
				"size":      1024,
			},
			CreatedAt: time.Now(),
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			err := eventBus.Publish(event)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("ConcurrentEventPublish", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				event := &events.Event{
					ID:       uuid.New(),
					Type:     "file_processed",
					TenantID: uuid.New(),
					Payload: map[string]interface{}{
						"file_path": "/test/file.txt",
						"size":      1024,
					},
					CreatedAt: time.Now(),
				}

				err := eventBus.Publish(event)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	})
}

func BenchmarkWorkflowPerformance(b *testing.B) {
	ctx := context.Background()
	eventBus := events.NewInMemoryEventBus(events.EventBusConfig{})
	workflowEngine := workflow.NewEngine(eventBus)

	// Create a simple workflow
	testWorkflow := &workflow.WorkflowDefinition{
		ID:   uuid.New(),
		Name: "benchmark-workflow",
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
				Name: "Process",
				Type: workflow.StepTypeClassify,
				Config: map[string]interface{}{
					"enable_all": true,
				},
				Dependencies: []string{"step1"},
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := workflowEngine.RegisterWorkflow(testWorkflow)
	require.NoError(b, err)

	input := map[string]interface{}{
		"content": "Test content for workflow",
		"metadata": map[string]interface{}{
			"source": "benchmark",
		},
	}

	b.Run("WorkflowExecution", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := workflowEngine.ExecuteWorkflow(ctx, testWorkflow.ID, input)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkFileReadersPerformance(b *testing.B) {
	// Create temporary test files
	testDir := b.TempDir()

	// Generate test data
	csvData := generateCSVData(1000)
	jsonData := generateJSONData(1000)
	textData := generateTestContent(10000)

	// Write test files
	csvFile := testDir + "/test.csv"
	jsonFile := testDir + "/test.json"
	textFile := testDir + "/test.txt"

	writeTestFile(b, csvFile, csvData)
	writeTestFile(b, jsonFile, jsonData)
	writeTestFile(b, textFile, textData)

	readers := map[string]struct {
		reader *TestReader
		file   string
	}{
		"CSV":  {NewTestReader("csv"), csvFile},
		"JSON": {NewTestReader("json"), jsonFile},
		"Text": {NewTestReader("text"), textFile},
	}

	for name, r := range readers {
		b.Run(name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := r.reader.Read(r.file)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// Stress tests for high load scenarios
func TestStressAuthentication(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	ctx := context.Background()

	// Setup
	jwtManager, err := auth.NewJWTManager(nil)
	require.NoError(t, err)

	userStore := auth.NewMemoryUserStore()
	tenantStore := auth.NewMemoryTenantStore()
	authService := auth.NewService(jwtManager, userStore, tenantStore, nil)

	// Create test user
	createReq := &auth.CreateUserRequest{
		Username: "stress_user",
		Email:    "stress@test.com",
		Password: "StressPass123!",
		Roles:    []auth.Role{auth.RoleUser},
	}

	_, err = authService.CreateUser(ctx, createReq)
	require.NoError(t, err)

	authReq := &auth.AuthRequest{
		Type:     auth.AuthTypePassword,
		Username: "stress_user",
		Password: "StressPass123!",
	}

	// Stress test with concurrent authentication requests
	const numGoroutines = 100
	const requestsPerGoroutine = 100

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*requestsPerGoroutine)

	start := time.Now()

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < requestsPerGoroutine; j++ {
				_, err := authService.Authenticate(ctx, authReq)
				if err != nil {
					errors <- err
					return
				}
			}
		}()
	}

	wg.Wait()
	close(errors)

	elapsed := time.Since(start)
	totalRequests := numGoroutines * requestsPerGoroutine

	// Check for errors
	errorCount := 0
	for err := range errors {
		t.Logf("Authentication error: %v", err)
		errorCount++
	}

	assert.Equal(t, 0, errorCount, "Should have no authentication errors")

	rps := float64(totalRequests) / elapsed.Seconds()
	t.Logf("Stress test completed: %d requests in %v (%.2f req/sec)",
		totalRequests, elapsed, rps)

	// Should handle at least 1000 req/sec
	assert.Greater(t, rps, 1000.0, "Should handle at least 1000 requests per second")
}

func TestStressEventSystem(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	eventBus := events.NewInMemoryEventBus(events.EventBusConfig{})

	// Setup handler that counts events
	var eventCount int64
	handlerFunc := func(ctx context.Context, event interface{}) error {
		eventCount++
		return nil
	}

	handler := &events.FunctionHandler{
		HandlerFunc: handlerFunc,
	}

	eventBus.Subscribe(handler, "file_processed")

	const numGoroutines = 50
	const eventsPerGoroutine = 1000

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*eventsPerGoroutine)

	start := time.Now()

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				event := &events.Event{
					ID:       uuid.New(),
					Type:     "file_processed",
					TenantID: uuid.New(),
					Payload: map[string]interface{}{
						"event_number": j,
						"goroutine_id": goroutineID,
					},
					CreatedAt: time.Now(),
				}

				err := eventBus.Publish(event)
				if err != nil {
					errors <- err
					return
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	elapsed := time.Since(start)
	totalEvents := numGoroutines * eventsPerGoroutine

	// Check for errors
	errorCount := 0
	for err := range errors {
		t.Logf("Event publishing error: %v", err)
		errorCount++
	}

	assert.Equal(t, 0, errorCount, "Should have no event publishing errors")

	eps := float64(totalEvents) / elapsed.Seconds()
	t.Logf("Event stress test completed: %d events in %v (%.2f events/sec)",
		totalEvents, elapsed, eps)

	// Should handle at least 10,000 events per second
	assert.Greater(t, eps, 10000.0, "Should handle at least 10,000 events per second")
}

// Memory usage tests
func TestMemoryUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory test in short mode")
	}

	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	// Simulate heavy processing
	ctx := context.Background()

	// Initialize components
	jwtManager, _ := auth.NewJWTManager(nil)
	userStore := auth.NewMemoryUserStore()
	tenantStore := auth.NewMemoryTenantStore()
	authService := auth.NewService(jwtManager, userStore, tenantStore, nil)

	classifier := classification.NewService(&classification.ServiceConfig{})
	dlpEngine := NewTestDLPEngine()
	chunker := chunking.NewFixedSizeChunker(500, 50)

	// Process large amounts of data
	for i := 0; i < 1000; i++ {
		// Create users
		createReq := &auth.CreateUserRequest{
			Username: fmt.Sprintf("user_%d", i),
			Email:    fmt.Sprintf("user_%d@test.com", i),
			Password: "Pass123!",
			Roles:    []auth.Role{auth.RoleUser},
		}
		authService.CreateUser(ctx, createReq)

		// Process content
		content := generateTestContent(1000)
		input := &classification.ClassificationInput{
			Content:  content,
			TenantID: uuid.New(),
		}
		classifier.Classify(ctx, input)
		dlpEngine.ScanText(content)
		chunker.Chunk(content)
	}

	runtime.GC()
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	memoryUsed := m2.Alloc - m1.Alloc
	t.Logf("Memory used: %d bytes (%.2f MB)", memoryUsed, float64(memoryUsed)/(1024*1024))

	// Memory usage should be reasonable (less than 100MB for this test)
	assert.Less(t, memoryUsed, uint64(100*1024*1024), "Memory usage should be less than 100MB")
}

// Helper functions for generating test data
func generateTestContent(size int) string {
	content := "This is sample content for testing purposes. It contains various words and sentences to simulate real document processing. "
	for len(content) < size {
		content += content
	}
	return content[:size]
}

func generateContentWithPII(size int) string {
	baseContent := "Here is some content with PII data. SSN: 123-45-6789, Credit Card: 4532-1234-5678-9012, Email: test@example.com. "
	content := baseContent
	for len(content) < size {
		content += "Additional content to reach desired size. More text here. "
	}
	return content[:size]
}

func generateCSVData(rows int) string {
	data := "name,email,age\n"
	for i := 0; i < rows; i++ {
		data += fmt.Sprintf("User%d,user%d@test.com,%d\n", i, i, 20+i%50)
	}
	return data
}

func generateJSONData(items int) string {
	data := `{"items":[`
	for i := 0; i < items; i++ {
		if i > 0 {
			data += ","
		}
		data += fmt.Sprintf(`{"id":%d,"name":"Item%d","value":%d}`, i, i, i*10)
	}
	data += "]}"
	return data
}

func writeTestFile(t testing.TB, filename, content string) {
	err := os.WriteFile(filename, []byte(content), 0644)
	require.NoError(t, err)
}
