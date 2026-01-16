package events

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ExampleIntegration demonstrates how to use the event-driven processing workflow
func ExampleIntegration() error {
	ctx := context.Background()

	// 1. Create event bus
	busConfig := DefaultEventBusConfig()
	eventBus := NewInMemoryEventBus(busConfig)

	// 2. Create workflow engine
	workflowConfig := DefaultWorkflowEngineConfig()
	workflowEngine := NewWorkflowEngine(nil, nil, workflowConfig) // Producer/Consumer would be set up separately

	// 3. Register step handlers
	workflowEngine.RegisterStepHandler(NewFileReaderHandler())
	workflowEngine.RegisterStepHandler(NewChunkingHandler())
	workflowEngine.RegisterStepHandler(NewDLPScanHandler())
	workflowEngine.RegisterStepHandler(NewClassificationHandler())
	workflowEngine.RegisterStepHandler(NewEmbeddingHandler())
	workflowEngine.RegisterStepHandler(NewStorageHandler())
	workflowEngine.RegisterStepHandler(NewNotificationHandler())

	// 4. Subscribe workflow engine to event bus
	if err := eventBus.Subscribe(workflowEngine, workflowEngine.GetEventTypes()...); err != nil {
		return fmt.Errorf("failed to subscribe workflow engine: %w", err)
	}

	// 5. Create and register a file processing workflow
	workflow := createFileProcessingWorkflow()
	if err := workflowEngine.RegisterWorkflow(workflow); err != nil {
		return fmt.Errorf("failed to register workflow: %w", err)
	}

	// 6. Start services
	if err := eventBus.Start(); err != nil {
		return fmt.Errorf("failed to start event bus: %w", err)
	}
	defer eventBus.Stop()

	if err := workflowEngine.Start(ctx); err != nil {
		return fmt.Errorf("failed to start workflow engine: %w", err)
	}
	defer workflowEngine.Stop(ctx)

	// 7. Simulate file discovery event
	tenantID := uuid.New()
	fileDiscoveredEvent := NewEvent(EventFileDiscovered, tenantID, map[string]interface{}{
		"data": map[string]interface{}{
			"url":          "s3://my-bucket/documents/report.pdf",
			"source_id":    uuid.New().String(),
			"size":         int64(1024000),
			"content_type": "application/pdf",
			"priority":     "normal",
		},
	})

	fmt.Printf("Publishing file discovered event: %s\n", fileDiscoveredEvent.ID)
	if err := eventBus.Publish(fileDiscoveredEvent); err != nil {
		return fmt.Errorf("failed to publish event: %w", err)
	}

	// 8. Wait for processing to complete
	time.Sleep(2 * time.Second)

	// 9. Check metrics
	busMetrics := eventBus.GetMetrics()
	workflowMetrics := workflowEngine.GetMetrics()

	fmt.Printf("\nEvent Bus Metrics:\n")
	fmt.Printf("  Events Published: %d\n", busMetrics.EventsPublished)
	fmt.Printf("  Events Processed: %d\n", busMetrics.EventsProcessed)
	fmt.Printf("  Events Failed: %d\n", busMetrics.EventsFailed)
	fmt.Printf("  Queue Depth: %d\n", busMetrics.QueueDepth)
	fmt.Printf("  Active Handlers: %d\n", busMetrics.HandlersActive)

	fmt.Printf("\nWorkflow Engine Metrics:\n")
	fmt.Printf("  Total Workflows: %v\n", workflowMetrics["total_workflows"])
	fmt.Printf("  Total Executions: %v\n", workflowMetrics["total_executions"])
	fmt.Printf("  Running Executions: %v\n", workflowMetrics["running_executions"])
	fmt.Printf("  Completed Executions: %v\n", workflowMetrics["completed_executions"])

	// 10. List workflow executions
	executions, err := workflowEngine.ListExecutions(tenantID)
	if err != nil {
		return fmt.Errorf("failed to list executions: %w", err)
	}

	fmt.Printf("\nWorkflow Executions (%d):\n", len(executions))
	for _, execution := range executions {
		fmt.Printf("  Execution %s: %s (Progress: %d%%)\n",
			execution.ID, execution.Status, execution.Progress)

		if execution.Status == "completed" {
			fmt.Printf("    Results: %+v\n", execution.Results)
		}

		if execution.Status == "failed" && execution.LastError != nil {
			fmt.Printf("    Error: %s\n", *execution.LastError)
		}
	}

	return nil
}

// createFileProcessingWorkflow creates a comprehensive file processing workflow
func createFileProcessingWorkflow() *WorkflowDefinition {
	tenantID := uuid.New() // In real implementation, this would be passed in

	return &WorkflowDefinition{
		ID:          uuid.New(),
		Name:        "file-processing-workflow",
		Version:     "1.0.0",
		Description: "Complete file processing pipeline with DLP scanning and classification",
		TenantID:    tenantID,
		Steps: []*WorkflowStep{
			{
				Name:         "read-file",
				Type:         "handler",
				Handler:      "file_reader",
				Dependencies: []string{},
				OnSuccess:    []string{"chunk-file"},
				OnFailure:    []string{"notify-failure"},
				Timeout:      &[]time.Duration{2 * time.Minute}[0],
				Retries:      2,
				Config: map[string]interface{}{
					"reader_type": "auto",
					"encoding":    "utf-8",
				},
			},
			{
				Name:         "chunk-file",
				Type:         "handler",
				Handler:      "chunking",
				Dependencies: []string{"read-file"},
				OnSuccess:    []string{"scan-dlp", "classify-content"},
				OnFailure:    []string{"notify-failure"},
				Timeout:      &[]time.Duration{5 * time.Minute}[0],
				Retries:      2,
				Parallel:     false,
				Config: map[string]interface{}{
					"strategy":   "adaptive",
					"chunk_size": 1000,
					"overlap":    100,
				},
			},
			{
				Name:         "scan-dlp",
				Type:         "handler",
				Handler:      "dlp_scan",
				Dependencies: []string{"chunk-file"},
				OnSuccess:    []string{"generate-embeddings"},
				OnFailure:    []string{"notify-failure"},
				Timeout:      &[]time.Duration{10 * time.Minute}[0],
				Retries:      1,
				Config: map[string]interface{}{
					"scan_depth": "deep",
					"policies":   []string{"pii-detection", "gdpr-compliance"},
				},
			},
			{
				Name:         "classify-content",
				Type:         "handler",
				Handler:      "classification",
				Dependencies: []string{"chunk-file"},
				OnSuccess:    []string{"generate-embeddings"},
				OnFailure:    []string{"notify-failure"},
				Timeout:      &[]time.Duration{3 * time.Minute}[0],
				Retries:      2,
				Parallel:     true, // Can run in parallel with DLP scan
				Config: map[string]interface{}{
					"models": []string{"content-classifier", "sentiment-analyzer"},
				},
			},
			{
				Name:         "generate-embeddings",
				Type:         "handler",
				Handler:      "embedding",
				Dependencies: []string{"scan-dlp", "classify-content"},
				OnSuccess:    []string{"store-results"},
				OnFailure:    []string{"notify-failure"},
				Timeout:      &[]time.Duration{15 * time.Minute}[0],
				Retries:      3,
				Config: map[string]interface{}{
					"model":      "text-embedding-ada-002",
					"batch_size": 10,
				},
			},
			{
				Name:         "store-results",
				Type:         "handler",
				Handler:      "storage",
				Dependencies: []string{"generate-embeddings"},
				OnSuccess:    []string{"notify-completion"},
				OnFailure:    []string{"notify-failure"},
				Timeout:      &[]time.Duration{5 * time.Minute}[0],
				Retries:      2,
				Config: map[string]interface{}{
					"vector_store": "pinecone",
					"database":     "postgresql",
				},
			},
			{
				Name:         "notify-completion",
				Type:         "handler",
				Handler:      "notification",
				Dependencies: []string{"store-results"},
				OnSuccess:    []string{},
				OnFailure:    []string{},
				Timeout:      &[]time.Duration{1 * time.Minute}[0],
				Retries:      1,
				Config: map[string]interface{}{
					"channels": []string{"email", "slack"},
					"type":     "success",
				},
			},
			{
				Name:         "notify-failure",
				Type:         "handler",
				Handler:      "notification",
				Dependencies: []string{}, // Can be triggered by any step failure
				OnSuccess:    []string{},
				OnFailure:    []string{},
				Timeout:      &[]time.Duration{1 * time.Minute}[0],
				Retries:      1,
				Config: map[string]interface{}{
					"channels": []string{"email", "slack", "pagerduty"},
					"type":     "error",
					"priority": "high",
				},
			},
		},
		Config: WorkflowConfig{
			MaxRetries:      3,
			RetryDelay:      30 * time.Second,
			Timeout:         30 * time.Minute,
			ParallelLimit:   5,
			ErrorHandling:   "continue", // Continue with other steps if one fails
			NotifyOnFailure: true,
			NotifyOnSuccess: true,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		CreatedBy: "system",
		Status:    "active",
	}
}

// DemoWorkflowExecution demonstrates a complete workflow execution
func DemoWorkflowExecution() error {
	fmt.Println("=== Event-Driven Processing Workflow Demo ===")

	if err := ExampleIntegration(); err != nil {
		return fmt.Errorf("demo failed: %w", err)
	}

	fmt.Println("\n=== Demo Completed Successfully ===")
	return nil
}

// ExampleEventTypes demonstrates the different types of events in the system
func ExampleEventTypes() {
	fmt.Println("=== Available Event Types ===")

	eventTypes := []string{
		EventFileDiscovered,
		EventFileResolved,
		EventFileClassified,
		EventFileChunked,
		EventChunkEmbedded,
		EventDLPViolation,
		EventProcessingComplete,
		EventProcessingFailed,
		EventTenantCreated,
		EventTenantUpdated,
		EventSessionStarted,
		EventSessionCompleted,
	}

	for i, eventType := range eventTypes {
		fmt.Printf("%d. %s\n", i+1, eventType)
	}

	fmt.Println("\n=== Example Event Flow ===")
	fmt.Println("1. file.discovered → triggers workflow")
	fmt.Println("2. file.resolved → starts processing pipeline")
	fmt.Println("3. file.chunked → creates chunks for processing")
	fmt.Println("4. dlp.violation → triggers security workflow (if violations found)")
	fmt.Println("5. file.classified → determines content classification")
	fmt.Println("6. chunk.embedded → creates vector embeddings")
	fmt.Println("7. processing.complete → finishes workflow")
}

// ExampleWorkflowDefinitions shows different workflow patterns
func ExampleWorkflowDefinitions() []*WorkflowDefinition {
	tenantID := uuid.New()

	return []*WorkflowDefinition{
		// Simple file processing workflow
		{
			ID:          uuid.New(),
			Name:        "simple-file-processing",
			Description: "Basic file processing without DLP",
			TenantID:    tenantID,
			Steps: []*WorkflowStep{
				{Name: "read-file", Type: "handler", Handler: "file_reader"},
				{Name: "chunk-file", Type: "handler", Handler: "chunking", Dependencies: []string{"read-file"}},
				{Name: "store-results", Type: "handler", Handler: "storage", Dependencies: []string{"chunk-file"}},
			},
			Status: "active",
		},

		// Security-focused workflow
		{
			ID:          uuid.New(),
			Name:        "security-scan-workflow",
			Description: "DLP and security scanning workflow",
			TenantID:    tenantID,
			Steps: []*WorkflowStep{
				{Name: "read-file", Type: "handler", Handler: "file_reader"},
				{Name: "scan-dlp", Type: "handler", Handler: "dlp_scan", Dependencies: []string{"read-file"}},
				{Name: "classify-security", Type: "handler", Handler: "classification", Dependencies: []string{"scan-dlp"}},
				{Name: "notify-security", Type: "handler", Handler: "notification", Dependencies: []string{"classify-security"}},
			},
			Status: "active",
		},

		// High-volume processing workflow
		{
			ID:          uuid.New(),
			Name:        "bulk-processing-workflow",
			Description: "Optimized for high-volume file processing",
			TenantID:    tenantID,
			Steps: []*WorkflowStep{
				{Name: "batch-read", Type: "handler", Handler: "file_reader", Parallel: true},
				{Name: "parallel-chunk", Type: "handler", Handler: "chunking", Dependencies: []string{"batch-read"}, Parallel: true},
				{Name: "parallel-embed", Type: "handler", Handler: "embedding", Dependencies: []string{"parallel-chunk"}, Parallel: true},
				{Name: "batch-store", Type: "handler", Handler: "storage", Dependencies: []string{"parallel-embed"}},
			},
			Config: WorkflowConfig{
				ParallelLimit: 20,
				Timeout:       60 * time.Minute,
			},
			Status: "active",
		},
	}
}
