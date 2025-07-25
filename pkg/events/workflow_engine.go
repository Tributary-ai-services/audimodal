package events

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// WorkflowEngine orchestrates processing workflows based on events
type WorkflowEngine struct {
	producer     *Producer
	consumer     *Consumer
	workflows    map[uuid.UUID]*WorkflowDefinition
	executions   map[uuid.UUID]*WorkflowExecution
	handlers     map[string]WorkflowStepHandler
	tracer       trace.Tracer
	mu           sync.RWMutex
	
	// Configuration
	config       WorkflowEngineConfig
	
	// Lifecycle
	running      bool
	stopChan     chan struct{}
	wg           sync.WaitGroup
}

// WorkflowEngineConfig contains configuration for the workflow engine
type WorkflowEngineConfig struct {
	MaxConcurrentWorkflows int           `yaml:"max_concurrent_workflows"`
	ExecutionTimeout       time.Duration `yaml:"execution_timeout"`
	StepTimeout           time.Duration `yaml:"step_timeout"`
	RetryDelay            time.Duration `yaml:"retry_delay"`
	CleanupInterval       time.Duration `yaml:"cleanup_interval"`
	MaxRetentionDays      int           `yaml:"max_retention_days"`
	EnableMetrics         bool          `yaml:"enable_metrics"`
	EnableTracing         bool          `yaml:"enable_tracing"`
}

// DefaultWorkflowEngineConfig returns default configuration
func DefaultWorkflowEngineConfig() WorkflowEngineConfig {
	return WorkflowEngineConfig{
		MaxConcurrentWorkflows: 100,
		ExecutionTimeout:       30 * time.Minute,
		StepTimeout:           5 * time.Minute,
		RetryDelay:            30 * time.Second,
		CleanupInterval:       1 * time.Hour,
		MaxRetentionDays:      7,
		EnableMetrics:         true,
		EnableTracing:         true,
	}
}

// WorkflowStepHandler defines the interface for handling workflow steps
type WorkflowStepHandler interface {
	// ExecuteStep executes a workflow step
	ExecuteStep(ctx context.Context, execution *WorkflowExecution, step *WorkflowStep, stepExecution *StepExecution) error
	
	// GetStepType returns the step type this handler can execute
	GetStepType() string
	
	// GetName returns the handler name
	GetName() string
}

// NewWorkflowEngine creates a new workflow engine
func NewWorkflowEngine(producer *Producer, consumer *Consumer, config WorkflowEngineConfig) *WorkflowEngine {
	engine := &WorkflowEngine{
		producer:   producer,
		consumer:   consumer,
		workflows:  make(map[uuid.UUID]*WorkflowDefinition),
		executions: make(map[uuid.UUID]*WorkflowExecution),
		handlers:   make(map[string]WorkflowStepHandler),
		tracer:     otel.Tracer("workflow-engine"),
		config:     config,
		stopChan:   make(chan struct{}),
	}
	
	// Register the engine as an event handler if consumer is provided
	if consumer != nil {
		consumer.RegisterHandler(engine)
	}
	
	return engine
}

// RegisterWorkflow registers a workflow definition
func (e *WorkflowEngine) RegisterWorkflow(workflow *WorkflowDefinition) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	// Validate workflow
	if err := e.validateWorkflow(workflow); err != nil {
		return fmt.Errorf("invalid workflow: %w", err)
	}
	
	e.workflows[workflow.ID] = workflow
	return nil
}

// RegisterStepHandler registers a step handler
func (e *WorkflowEngine) RegisterStepHandler(handler WorkflowStepHandler) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.handlers[handler.GetStepType()] = handler
}

// StartWorkflow starts a workflow execution
func (e *WorkflowEngine) StartWorkflow(ctx context.Context, workflowID, tenantID uuid.UUID, triggerEvent interface{}, context map[string]interface{}) (*WorkflowExecution, error) {
	ctx, span := e.tracer.Start(ctx, "start_workflow")
	defer span.End()
	
	e.mu.RLock()
	workflow, exists := e.workflows[workflowID]
	e.mu.RUnlock()
	
	if !exists {
		return nil, fmt.Errorf("workflow not found: %s", workflowID)
	}
	
	// Create execution
	execution := &WorkflowExecution{
		ID:            uuid.New(),
		WorkflowID:    workflowID,
		TenantID:      tenantID,
		Status:        "pending",
		Progress:      0,
		StepExecutions: make(map[string]*StepExecution),
		Context:       context,
		Results:       make(map[string]interface{}),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	
	// Set trigger event if provided
	if triggerEvent != nil {
		if event, ok := triggerEvent.(*Event); ok {
			execution.TriggerEvent = event
		}
	}
	
	// Initialize step executions
	for _, step := range workflow.Steps {
		execution.StepExecutions[step.Name] = &StepExecution{
			StepName: step.Name,
			Status:   "pending",
			Results:  make(map[string]interface{}),
		}
	}
	
	// Store execution
	e.mu.Lock()
	e.executions[execution.ID] = execution
	e.mu.Unlock()
	
	// Start execution asynchronously
	go e.executeWorkflow(ctx, execution, workflow)
	
	span.SetAttributes(
		attribute.String("workflow.id", workflowID.String()),
		attribute.String("execution.id", execution.ID.String()),
		attribute.String("tenant.id", tenantID.String()),
	)
	
	return execution, nil
}

// executeWorkflow executes a workflow
func (e *WorkflowEngine) executeWorkflow(ctx context.Context, execution *WorkflowExecution, workflow *WorkflowDefinition) {
	ctx, span := e.tracer.Start(ctx, "execute_workflow")
	defer span.End()
	
	// Apply timeout
	if workflow.Config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, workflow.Config.Timeout)
		defer cancel()
	}
	
	execution.Status = "running"
	execution.StartedAt = &[]time.Time{time.Now()}[0]
	execution.UpdatedAt = time.Now()
	
	// Create dependency graph
	dependencies := e.buildDependencyGraph(workflow.Steps)
	
	// Execute steps based on dependencies
	if err := e.executeStepsWithDependencies(ctx, execution, workflow, dependencies); err != nil {
		execution.Status = "failed"
		execution.LastError = &[]string{err.Error()}[0]
		execution.ErrorCount++
		
		// Publish failure event
		e.publishWorkflowEvent(ctx, execution, "workflow.failed", map[string]interface{}{
			"error": err.Error(),
		})
	} else {
		execution.Status = "completed"
		execution.Progress = 100
		
		// Publish success event
		e.publishWorkflowEvent(ctx, execution, "workflow.completed", execution.Results)
	}
	
	now := time.Now()
	execution.CompletedAt = &now
	execution.UpdatedAt = now
	
	span.SetAttributes(
		attribute.String("execution.status", execution.Status),
		attribute.Int("execution.progress", execution.Progress),
	)
}

// executeStepsWithDependencies executes workflow steps respecting dependencies
func (e *WorkflowEngine) executeStepsWithDependencies(ctx context.Context, execution *WorkflowExecution, workflow *WorkflowDefinition, dependencies map[string][]string) error {
	completed := make(map[string]bool)
	totalSteps := len(workflow.Steps)
	
	for len(completed) < totalSteps {
		// Find ready steps (dependencies satisfied)
		var readySteps []*WorkflowStep
		for _, step := range workflow.Steps {
			if completed[step.Name] {
				continue
			}
			
			// Check if all dependencies are completed
			allDepsSatisfied := true
			for _, dep := range step.Dependencies {
				if !completed[dep] {
					allDepsSatisfied = false
					break
				}
			}
			
			if allDepsSatisfied {
				readySteps = append(readySteps, step)
			}
		}
		
		if len(readySteps) == 0 {
			return fmt.Errorf("workflow deadlock: no steps ready to execute")
		}
		
		// Execute ready steps
		if err := e.executeStepsParallel(ctx, execution, readySteps); err != nil {
			return err
		}
		
		// Mark completed steps
		for _, step := range readySteps {
			stepExec := execution.StepExecutions[step.Name]
			if stepExec.Status == "completed" {
				completed[step.Name] = true
				execution.Progress = (len(completed) * 100) / totalSteps
			} else if stepExec.Status == "failed" {
				return fmt.Errorf("step %s failed: %s", step.Name, *stepExec.LastError)
			}
		}
	}
	
	return nil
}

// executeStepsParallel executes multiple steps in parallel
func (e *WorkflowEngine) executeStepsParallel(ctx context.Context, execution *WorkflowExecution, steps []*WorkflowStep) error {
	var wg sync.WaitGroup
	errors := make(chan error, len(steps))
	
	// Limit parallelism
	semaphore := make(chan struct{}, e.config.MaxConcurrentWorkflows)
	
	for _, step := range steps {
		wg.Add(1)
		go func(s *WorkflowStep) {
			defer wg.Done()
			
			semaphore <- struct{}{} // Acquire
			defer func() { <-semaphore }() // Release
			
			if err := e.executeStep(ctx, execution, s); err != nil {
				errors <- err
			}
		}(step)
	}
	
	// Wait for all steps to complete
	go func() {
		wg.Wait()
		close(errors)
	}()
	
	// Check for errors
	for err := range errors {
		if err != nil {
			return err
		}
	}
	
	return nil
}

// executeStep executes a single workflow step
func (e *WorkflowEngine) executeStep(ctx context.Context, execution *WorkflowExecution, step *WorkflowStep) error {
	ctx, span := e.tracer.Start(ctx, "execute_step")
	defer span.End()
	
	stepExecution := execution.StepExecutions[step.Name]
	stepExecution.Status = "running"
	stepExecution.StartedAt = &[]time.Time{time.Now()}[0]
	
	// Apply step timeout
	if step.Timeout != nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, *step.Timeout)
		defer cancel()
	}
	
	span.SetAttributes(
		attribute.String("step.name", step.Name),
		attribute.String("step.type", step.Type),
	)
	
	var err error
	
	// Handle different step types
	switch step.Type {
	case "handler":
		err = e.executeHandlerStep(ctx, execution, step, stepExecution)
	case "condition":
		err = e.executeConditionalStep(ctx, execution, step, stepExecution)
	case "parallel":
		err = e.executeParallelStep(ctx, execution, step, stepExecution)
	case "sequential":
		err = e.executeSequentialStep(ctx, execution, step, stepExecution)
	default:
		err = fmt.Errorf("unknown step type: %s", step.Type)
	}
	
	// Handle step completion
	now := time.Now()
	stepExecution.CompletedAt = &now
	if stepExecution.StartedAt != nil {
		duration := now.Sub(*stepExecution.StartedAt)
		stepExecution.Duration = &duration
	}
	
	if err != nil {
		stepExecution.Status = "failed"
		stepExecution.LastError = &[]string{err.Error()}[0]
		stepExecution.RetryCount++
		
		// Check if we should retry
		if stepExecution.RetryCount < step.Retries {
			time.Sleep(e.config.RetryDelay)
			return e.executeStep(ctx, execution, step)
		}
		
		span.RecordError(err)
		return err
	}
	
	stepExecution.Status = "completed"
	return nil
}

// executeHandlerStep executes a handler-based step
func (e *WorkflowEngine) executeHandlerStep(ctx context.Context, execution *WorkflowExecution, step *WorkflowStep, stepExecution *StepExecution) error {
	e.mu.RLock()
	handler, exists := e.handlers[step.Handler]
	e.mu.RUnlock()
	
	if !exists {
		return fmt.Errorf("handler not found: %s", step.Handler)
	}
	
	return handler.ExecuteStep(ctx, execution, step, stepExecution)
}

// executeConditionalStep executes a conditional step
func (e *WorkflowEngine) executeConditionalStep(ctx context.Context, execution *WorkflowExecution, step *WorkflowStep, stepExecution *StepExecution) error {
	if step.Condition == nil {
		return fmt.Errorf("conditional step requires condition")
	}
	
	// Evaluate condition (simplified implementation)
	result, err := e.evaluateCondition(step.Condition, execution.Context)
	if err != nil {
		return fmt.Errorf("failed to evaluate condition: %w", err)
	}
	
	stepExecution.Results["condition_result"] = result
	return nil
}

// executeParallelStep executes steps in parallel
func (e *WorkflowEngine) executeParallelStep(ctx context.Context, execution *WorkflowExecution, step *WorkflowStep, stepExecution *StepExecution) error {
	// This would execute sub-steps in parallel
	// For now, just mark as completed
	stepExecution.Results["parallel_execution"] = "completed"
	return nil
}

// executeSequentialStep executes steps sequentially
func (e *WorkflowEngine) executeSequentialStep(ctx context.Context, execution *WorkflowExecution, step *WorkflowStep, stepExecution *StepExecution) error {
	// This would execute sub-steps sequentially
	// For now, just mark as completed
	stepExecution.Results["sequential_execution"] = "completed"
	return nil
}

// evaluateCondition evaluates a step condition
func (e *WorkflowEngine) evaluateCondition(condition *StepCondition, context map[string]interface{}) (bool, error) {
	// Simplified condition evaluation
	// In a real implementation, this would use a proper expression evaluator
	switch condition.Expression {
	case "always":
		return true, nil
	case "never":
		return false, nil
	default:
		// For now, just return true
		return true, nil
	}
}

// buildDependencyGraph builds a dependency graph for workflow steps
func (e *WorkflowEngine) buildDependencyGraph(steps []*WorkflowStep) map[string][]string {
	dependencies := make(map[string][]string)
	for _, step := range steps {
		dependencies[step.Name] = step.Dependencies
	}
	return dependencies
}

// publishWorkflowEvent publishes a workflow-related event
func (e *WorkflowEngine) publishWorkflowEvent(ctx context.Context, execution *WorkflowExecution, eventType string, data map[string]interface{}) {
	event := map[string]interface{}{
		"execution_id": execution.ID.String(),
		"workflow_id":  execution.WorkflowID.String(),
		"tenant_id":    execution.TenantID.String(),
		"status":       execution.Status,
		"progress":     execution.Progress,
		"data":         data,
	}
	
	// In a real implementation, this would create proper event objects
	// For now, we'll just log it
	_ = event
}

// validateWorkflow validates a workflow definition
func (e *WorkflowEngine) validateWorkflow(workflow *WorkflowDefinition) error {
	if workflow.ID == uuid.Nil {
		return fmt.Errorf("workflow ID is required")
	}
	
	if workflow.Name == "" {
		return fmt.Errorf("workflow name is required")
	}
	
	if len(workflow.Steps) == 0 {
		return fmt.Errorf("workflow must have at least one step")
	}
	
	// Validate step names are unique
	stepNames := make(map[string]bool)
	for _, step := range workflow.Steps {
		if stepNames[step.Name] {
			return fmt.Errorf("duplicate step name: %s", step.Name)
		}
		stepNames[step.Name] = true
		
		// Validate step dependencies exist
		for _, dep := range step.Dependencies {
			if !stepNames[dep] && dep != step.Name {
				// Check if dependency exists in other steps
				found := false
				for _, otherStep := range workflow.Steps {
					if otherStep.Name == dep {
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("step %s depends on non-existent step: %s", step.Name, dep)
				}
			}
		}
	}
	
	return nil
}

// Event handler implementation for WorkflowEngine
func (e *WorkflowEngine) HandleEvent(ctx context.Context, event interface{}) error {
	// Handle events that should trigger workflows
	switch evt := event.(type) {
	case *FileDiscoveredEvent:
		return e.handleFileDiscoveredEvent(ctx, evt)
	case *FileClassifiedEvent:
		return e.handleFileClassifiedEvent(ctx, evt)
	case *DLPViolationEvent:
		return e.handleDLPViolationEvent(ctx, evt)
	default:
		// Event not handled by workflow engine
		return nil
	}
}

// GetEventTypes returns event types handled by the workflow engine
func (e *WorkflowEngine) GetEventTypes() []string {
	return []string{
		EventFileDiscovered,
		EventFileClassified,
		EventDLPViolation,
	}
}

// GetName returns the handler name
func (e *WorkflowEngine) GetName() string {
	return "workflow-engine"
}

// handleFileDiscoveredEvent handles file discovered events
func (e *WorkflowEngine) handleFileDiscoveredEvent(ctx context.Context, event *FileDiscoveredEvent) error {
	// Find workflows that should be triggered by file discovery
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	for _, workflow := range e.workflows {
		// Check if this workflow should be triggered by file discovery
		if e.shouldTriggerWorkflow(workflow, "file.discovered", event.TenantID) {
			tenantID, _ := uuid.Parse(event.TenantID)
			
			// Start workflow execution
			_, err := e.StartWorkflow(ctx, workflow.ID, tenantID, event, map[string]interface{}{
				"file_url": event.Data.URL,
				"source_id": event.Data.SourceID,
				"trigger_event": "file.discovered",
			})
			
			if err != nil {
				return fmt.Errorf("failed to start workflow %s: %w", workflow.Name, err)
			}
		}
	}
	
	return nil
}

// handleFileClassifiedEvent handles file classified events
func (e *WorkflowEngine) handleFileClassifiedEvent(ctx context.Context, event *FileClassifiedEvent) error {
	// Handle file classification events
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	for _, workflow := range e.workflows {
		if e.shouldTriggerWorkflow(workflow, "file.classified", event.TenantID) {
			tenantID, _ := uuid.Parse(event.TenantID)
			
			_, err := e.StartWorkflow(ctx, workflow.ID, tenantID, event, map[string]interface{}{
				"file_url": event.Data.URL,
				"data_class": event.Data.DataClass,
				"pii_types": event.Data.PIITypes,
				"trigger_event": "file.classified",
			})
			
			if err != nil {
				return fmt.Errorf("failed to start workflow %s: %w", workflow.Name, err)
			}
		}
	}
	
	return nil
}

// handleDLPViolationEvent handles DLP violation events
func (e *WorkflowEngine) handleDLPViolationEvent(ctx context.Context, event *DLPViolationEvent) error {
	// Handle DLP violation events (high priority)
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	for _, workflow := range e.workflows {
		if e.shouldTriggerWorkflow(workflow, "dlp.violation", event.TenantID) {
			tenantID, _ := uuid.Parse(event.TenantID)
			
			_, err := e.StartWorkflow(ctx, workflow.ID, tenantID, event, map[string]interface{}{
				"file_url": event.Data.URL,
				"violation_type": event.Data.ViolationType,
				"severity": event.Data.Severity,
				"trigger_event": "dlp.violation",
			})
			
			if err != nil {
				return fmt.Errorf("failed to start workflow %s: %w", workflow.Name, err)
			}
		}
	}
	
	return nil
}

// shouldTriggerWorkflow determines if a workflow should be triggered by an event
func (e *WorkflowEngine) shouldTriggerWorkflow(workflow *WorkflowDefinition, eventType, tenantID string) bool {
	// Check tenant match
	if workflow.TenantID.String() != tenantID {
		return false
	}
	
	// Check if workflow is active
	if workflow.Status != "active" {
		return false
	}
	
	// Check if workflow has any trigger configuration
	// For now, assume all active workflows can be triggered by any event
	// In a real implementation, this would check workflow trigger configuration
	return true
}

// GetExecution returns a workflow execution by ID
func (e *WorkflowEngine) GetExecution(id uuid.UUID) (*WorkflowExecution, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	execution, exists := e.executions[id]
	if !exists {
		return nil, fmt.Errorf("execution not found: %s", id)
	}
	
	return execution, nil
}

// ListExecutions returns all executions for a tenant
func (e *WorkflowEngine) ListExecutions(tenantID uuid.UUID) ([]*WorkflowExecution, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	var executions []*WorkflowExecution
	for _, execution := range e.executions {
		if execution.TenantID == tenantID {
			executions = append(executions, execution)
		}
	}
	
	return executions, nil
}

// CancelExecution cancels a running workflow execution
func (e *WorkflowEngine) CancelExecution(ctx context.Context, id uuid.UUID) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	execution, exists := e.executions[id]
	if !exists {
		return fmt.Errorf("execution not found: %s", id)
	}
	
	if execution.Status != "running" {
		return fmt.Errorf("execution is not running: %s", execution.Status)
	}
	
	execution.Status = "cancelled"
	execution.UpdatedAt = time.Now()
	
	return nil
}

// Start starts the workflow engine
func (e *WorkflowEngine) Start(ctx context.Context) error {
	if e.running {
		return fmt.Errorf("workflow engine is already running")
	}
	
	e.running = true
	
	// Start cleanup routine
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		e.cleanupRoutine(ctx)
	}()
	
	return nil
}

// Stop stops the workflow engine
func (e *WorkflowEngine) Stop(ctx context.Context) error {
	if !e.running {
		return nil
	}
	
	close(e.stopChan)
	e.running = false
	
	// Wait for routines to finish
	e.wg.Wait()
	
	return nil
}

// cleanupRoutine periodically cleans up old executions
func (e *WorkflowEngine) cleanupRoutine(ctx context.Context) {
	ticker := time.NewTicker(e.config.CleanupInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-e.stopChan:
			return
		case <-ticker.C:
			e.cleanupOldExecutions()
		}
	}
}

// cleanupOldExecutions removes old completed executions
func (e *WorkflowEngine) cleanupOldExecutions() {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	cutoff := time.Now().AddDate(0, 0, -e.config.MaxRetentionDays)
	
	for id, execution := range e.executions {
		if execution.CompletedAt != nil && execution.CompletedAt.Before(cutoff) {
			delete(e.executions, id)
		}
	}
}

// GetMetrics returns workflow engine metrics
func (e *WorkflowEngine) GetMetrics() map[string]interface{} {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	var pendingCount, runningCount, completedCount, failedCount int
	
	for _, execution := range e.executions {
		switch execution.Status {
		case "pending":
			pendingCount++
		case "running":
			runningCount++
		case "completed":
			completedCount++
		case "failed":
			failedCount++
		}
	}
	
	return map[string]interface{}{
		"total_workflows":     len(e.workflows),
		"total_executions":    len(e.executions),
		"pending_executions":  pendingCount,
		"running_executions":  runningCount,
		"completed_executions": completedCount,
		"failed_executions":   failedCount,
		"running":            e.running,
	}
}