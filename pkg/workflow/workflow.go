package workflow

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jscharber/eAIIngest/pkg/events"
)

// EventBus defines the interface for event bus operations needed by workflow engine
type EventBus interface {
	Publish(event *events.Event) error
	Subscribe(handler events.EventHandler, eventTypes ...string) error
}

// Engine defines the workflow engine interface
type Engine interface {
	RegisterWorkflow(workflow *WorkflowDefinition) error
	ExecuteWorkflow(ctx context.Context, workflowID uuid.UUID, input map[string]interface{}) (ExecutionID, error)
	GetExecution(ctx context.Context, executionID ExecutionID) (*WorkflowExecution, error)
	GetWorkflow(ctx context.Context, workflowID uuid.UUID) (*WorkflowDefinition, error)
}

// WorkflowDefinition represents a workflow definition
type WorkflowDefinition struct {
	ID          uuid.UUID       `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Version     string          `json:"version"`
	Steps       []WorkflowStep  `json:"steps"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	CreatedBy   string          `json:"created_by"`
	Tags        []string        `json:"tags"`
	Status      WorkflowStatus  `json:"status"`
}

// WorkflowStep represents a single step in a workflow
type WorkflowStep struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Type         StepType               `json:"type"`
	Config       map[string]interface{} `json:"config"`
	Dependencies []string               `json:"dependencies"`
	Timeout      *time.Duration         `json:"timeout,omitempty"`
	RetryPolicy  *RetryPolicy           `json:"retry_policy,omitempty"`
	Condition    *StepCondition         `json:"condition,omitempty"`
}

// WorkflowExecution represents a workflow execution instance
type WorkflowExecution struct {
	ID           ExecutionID            `json:"id"`
	WorkflowID   uuid.UUID              `json:"workflow_id"`
	Status       ExecutionStatus        `json:"status"`
	Input        map[string]interface{} `json:"input"`
	Output       map[string]interface{} `json:"output,omitempty"`
	StepResults  map[string]StepResult  `json:"step_results"`
	StartedAt    time.Time              `json:"started_at"`
	CompletedAt  *time.Time             `json:"completed_at,omitempty"`
	Error        *string                `json:"error,omitempty"`
	Context      map[string]interface{} `json:"context,omitempty"`
}

// StepResult represents the result of a workflow step execution
type StepResult struct {
	StepID      string                 `json:"step_id"`
	Status      StepStatus             `json:"status"`
	Output      map[string]interface{} `json:"output,omitempty"`
	Error       *string                `json:"error,omitempty"`
	StartedAt   time.Time              `json:"started_at"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
	Attempts    int                    `json:"attempts"`
}

// RetryPolicy defines retry behavior for a step
type RetryPolicy struct {
	MaxAttempts int           `json:"max_attempts"`
	Delay       time.Duration `json:"delay"`
	BackoffType BackoffType   `json:"backoff_type"`
	MaxDelay    time.Duration `json:"max_delay"`
}

// StepCondition defines conditional execution
type StepCondition struct {
	Expression string `json:"expression"`
	Type       string `json:"type"` // "javascript", "simple", etc.
}

// Types and constants

type ExecutionID uuid.UUID
type WorkflowStatus string
type ExecutionStatus string
type StepStatus string
type StepType string
type BackoffType string

const (
	// Workflow statuses
	WorkflowStatusActive   WorkflowStatus = "active"
	WorkflowStatusInactive WorkflowStatus = "inactive"
	WorkflowStatusDraft    WorkflowStatus = "draft"
	WorkflowStatusArchived WorkflowStatus = "archived"

	// Execution statuses
	ExecutionStatusPending   ExecutionStatus = "pending"
	ExecutionStatusRunning   ExecutionStatus = "running"
	ExecutionStatusCompleted ExecutionStatus = "completed"
	ExecutionStatusFailed    ExecutionStatus = "failed"
	ExecutionStatusCancelled ExecutionStatus = "cancelled"

	// Step statuses
	StepStatusPending   StepStatus = "pending"
	StepStatusRunning   StepStatus = "running"
	StepStatusCompleted StepStatus = "completed"
	StepStatusFailed    StepStatus = "failed"
	StepStatusSkipped   StepStatus = "skipped"

	// Step types
	StepTypeFileRead   StepType = "file_read"
	StepTypeClassify   StepType = "classify"
	StepTypeDLPScan    StepType = "dlp_scan"
	StepTypeChunk      StepType = "chunk"
	StepTypeTransform  StepType = "transform"
	StepTypeValidate   StepType = "validate"
	StepTypeNotify     StepType = "notify"
	StepTypeScript     StepType = "script"
	StepTypeAPI        StepType = "api"
	StepTypeCondition  StepType = "condition"

	// Backoff types
	BackoffTypeLinear      BackoffType = "linear"
	BackoffTypeExponential BackoffType = "exponential"
	BackoffTypeFixed       BackoffType = "fixed"
)

// SimpleWorkflowEngine provides a basic workflow engine implementation
type SimpleWorkflowEngine struct {
	workflows  map[uuid.UUID]*WorkflowDefinition
	executions map[ExecutionID]*WorkflowExecution
	eventBus   EventBus
	mu         sync.RWMutex
}

// NewEngine creates a new workflow engine
func NewEngine(eventBus EventBus) Engine {
	return &SimpleWorkflowEngine{
		workflows:  make(map[uuid.UUID]*WorkflowDefinition),
		executions: make(map[ExecutionID]*WorkflowExecution),
		eventBus:   eventBus,
	}
}

// RegisterWorkflow registers a new workflow definition
func (e *SimpleWorkflowEngine) RegisterWorkflow(workflow *WorkflowDefinition) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if workflow.ID == uuid.Nil {
		workflow.ID = uuid.New()
	}

	if workflow.Status == "" {
		workflow.Status = WorkflowStatusActive
	}

	e.workflows[workflow.ID] = workflow
	return nil
}

// ExecuteWorkflow starts a new workflow execution
func (e *SimpleWorkflowEngine) ExecuteWorkflow(ctx context.Context, workflowID uuid.UUID, input map[string]interface{}) (ExecutionID, error) {
	e.mu.RLock()
	workflow, exists := e.workflows[workflowID]
	e.mu.RUnlock()

	if !exists {
		return ExecutionID(uuid.Nil), fmt.Errorf("workflow not found: %s", workflowID)
	}

	if workflow.Status != WorkflowStatusActive {
		return ExecutionID(uuid.Nil), fmt.Errorf("workflow is not active: %s", workflow.Status)
	}

	executionID := ExecutionID(uuid.New())
	execution := &WorkflowExecution{
		ID:          executionID,
		WorkflowID:  workflowID,
		Status:      ExecutionStatusPending,
		Input:       input,
		StepResults: make(map[string]StepResult),
		StartedAt:   time.Now(),
		Context:     make(map[string]interface{}),
	}

	e.mu.Lock()
	e.executions[executionID] = execution
	e.mu.Unlock()

	// Start execution in a goroutine
	go e.executeWorkflowAsync(ctx, execution, workflow)

	return executionID, nil
}

// GetExecution retrieves a workflow execution by ID
func (e *SimpleWorkflowEngine) GetExecution(ctx context.Context, executionID ExecutionID) (*WorkflowExecution, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	execution, exists := e.executions[executionID]
	if !exists {
		return nil, fmt.Errorf("execution not found: %s", executionID)
	}

	return execution, nil
}

// GetWorkflow retrieves a workflow definition by ID
func (e *SimpleWorkflowEngine) GetWorkflow(ctx context.Context, workflowID uuid.UUID) (*WorkflowDefinition, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	workflow, exists := e.workflows[workflowID]
	if !exists {
		return nil, fmt.Errorf("workflow not found: %s", workflowID)
	}

	return workflow, nil
}

// executeWorkflowAsync executes a workflow asynchronously
func (e *SimpleWorkflowEngine) executeWorkflowAsync(ctx context.Context, execution *WorkflowExecution, workflow *WorkflowDefinition) {
	e.mu.Lock()
	execution.Status = ExecutionStatusRunning
	e.mu.Unlock()

	// Publish workflow started event
	startEvent := &events.Event{
		ID:       uuid.New(),
		Type:     "workflow.started",
		TenantID: uuid.Nil, // Default tenant
		Payload: map[string]interface{}{
			"execution_id":   execution.ID,
			"workflow_id":    execution.WorkflowID,
			"workflow_name":  workflow.Name,
		},
		CreatedAt: time.Now(),
	}
	e.eventBus.Publish(startEvent)

	// Execute steps based on dependencies
	err := e.executeSteps(ctx, execution, workflow)

	e.mu.Lock()
	if err != nil {
		execution.Status = ExecutionStatusFailed
		errStr := err.Error()
		execution.Error = &errStr
	} else {
		execution.Status = ExecutionStatusCompleted
	}
	now := time.Now()
	execution.CompletedAt = &now
	e.mu.Unlock()

	// Publish workflow completed event
	eventType := "workflow.completed"
	if err != nil {
		eventType = "workflow.failed"
	}

	completeEvent := &events.Event{
		ID:       uuid.New(),
		Type:     eventType,
		TenantID: uuid.Nil, // Default tenant
		Payload: map[string]interface{}{
			"execution_id": execution.ID,
			"workflow_id":  execution.WorkflowID,
			"status":       execution.Status,
			"error":        execution.Error,
		},
		CreatedAt: time.Now(),
	}
	e.eventBus.Publish(completeEvent)
}

// executeSteps executes workflow steps in dependency order
func (e *SimpleWorkflowEngine) executeSteps(ctx context.Context, execution *WorkflowExecution, workflow *WorkflowDefinition) error {
	// Build dependency graph
	stepMap := make(map[string]*WorkflowStep)
	for i := range workflow.Steps {
		step := &workflow.Steps[i]
		stepMap[step.ID] = step
	}

	// Track completed steps
	completed := make(map[string]bool)
	
	// Execute steps in topological order
	for len(completed) < len(workflow.Steps) {
		progress := false
		
		for _, step := range workflow.Steps {
			if completed[step.ID] {
				continue
			}
			
			// Check if all dependencies are completed
			canExecute := true
			for _, depID := range step.Dependencies {
				if !completed[depID] {
					canExecute = false
					break
				}
			}
			
			if canExecute {
				err := e.executeStep(ctx, execution, &step)
				if err != nil {
					return fmt.Errorf("step %s failed: %w", step.ID, err)
				}
				completed[step.ID] = true
				progress = true
			}
		}
		
		if !progress {
			return fmt.Errorf("circular dependency detected or missing dependencies")
		}
	}
	
	return nil
}

// executeStep executes a single workflow step
func (e *SimpleWorkflowEngine) executeStep(ctx context.Context, execution *WorkflowExecution, step *WorkflowStep) error {
	stepResult := StepResult{
		StepID:    step.ID,
		Status:    StepStatusRunning,
		StartedAt: time.Now(),
		Attempts:  1,
	}

	e.mu.Lock()
	execution.StepResults[step.ID] = stepResult
	e.mu.Unlock()

	// Simulate step execution based on type
	var output map[string]interface{}
	var err error

	switch step.Type {
	case StepTypeFileRead:
		output, err = e.executeFileReadStep(ctx, step, execution)
	case StepTypeClassify:
		output, err = e.executeClassifyStep(ctx, step, execution)
	case StepTypeDLPScan:
		output, err = e.executeDLPScanStep(ctx, step, execution)
	case StepTypeChunk:
		output, err = e.executeChunkStep(ctx, step, execution)
	default:
		// Default simulation
		output = map[string]interface{}{
			"step_type": string(step.Type),
			"message":   "Step executed successfully",
		}
	}

	// Update step result
	e.mu.Lock()
	stepResult = execution.StepResults[step.ID]
	if err != nil {
		stepResult.Status = StepStatusFailed
		errStr := err.Error()
		stepResult.Error = &errStr
	} else {
		stepResult.Status = StepStatusCompleted
		stepResult.Output = output
	}
	now := time.Now()
	stepResult.CompletedAt = &now
	execution.StepResults[step.ID] = stepResult
	e.mu.Unlock()

	return err
}

// Step execution implementations

func (e *SimpleWorkflowEngine) executeFileReadStep(ctx context.Context, step *WorkflowStep, execution *WorkflowExecution) (map[string]interface{}, error) {
	path, ok := step.Config["path"].(string)
	if !ok {
		return nil, fmt.Errorf("file path not specified")
	}

	return map[string]interface{}{
		"file_path": path,
		"content_type": "text/plain",
		"size_bytes": 1024,
		"message": "File read simulated",
	}, nil
}

func (e *SimpleWorkflowEngine) executeClassifyStep(ctx context.Context, step *WorkflowStep, execution *WorkflowExecution) (map[string]interface{}, error) {
	return map[string]interface{}{
		"content_type": "document",
		"language": "en",
		"sentiment": "neutral",
		"confidence": 0.85,
		"message": "Classification completed",
	}, nil
}

func (e *SimpleWorkflowEngine) executeDLPScanStep(ctx context.Context, step *WorkflowStep, execution *WorkflowExecution) (map[string]interface{}, error) {
	return map[string]interface{}{
		"pii_found": true,
		"pii_types": []string{"ssn", "email"},
		"risk_score": 0.7,
		"message": "DLP scan completed",
	}, nil
}

func (e *SimpleWorkflowEngine) executeChunkStep(ctx context.Context, step *WorkflowStep, execution *WorkflowExecution) (map[string]interface{}, error) {
	return map[string]interface{}{
		"chunks_created": 5,
		"total_size": 2048,
		"strategy": "fixed_size",
		"message": "Content chunked successfully",
	}, nil
}

// Utility functions

// GenerateWorkflowID generates a new workflow ID
func GenerateWorkflowID() uuid.UUID {
	return uuid.New()
}

// GenerateExecutionID generates a new execution ID
func GenerateExecutionID() ExecutionID {
	return ExecutionID(uuid.New())
}

// IsValidStepType checks if a step type is valid
func IsValidStepType(stepType StepType) bool {
	validTypes := []StepType{
		StepTypeFileRead,
		StepTypeClassify,
		StepTypeDLPScan,
		StepTypeChunk,
		StepTypeTransform,
		StepTypeValidate,
		StepTypeNotify,
		StepTypeScript,
		StepTypeAPI,
		StepTypeCondition,
	}

	for _, valid := range validTypes {
		if stepType == valid {
			return true
		}
	}
	return false
}