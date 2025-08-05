package events

import (
	"context"

	"github.com/google/uuid"
)

// Event type constants for workflow events
const (
	EventTypeFileProcessed     = "file.processed"
	EventTypeWorkflowStarted   = "workflow.started"
	EventTypeWorkflowCompleted = "workflow.completed"
	EventTypeWorkflowFailed    = "workflow.failed"
)

// GenerateEventID generates a new event ID
func GenerateEventID() uuid.UUID {
	return uuid.New()
}

// NewDefaultInMemoryEventBus creates a new in-memory event bus with default config
func NewDefaultInMemoryEventBus() EventBusInterface {
	config := DefaultEventBusConfig()
	return NewInMemoryEventBus(config)
}

// FunctionHandler wraps a function as an EventHandler
type FunctionHandler struct {
	HandlerFunc func(ctx context.Context, event interface{}) error
	name        string
}

func (h *FunctionHandler) HandleEvent(ctx context.Context, event interface{}) error {
	return h.HandlerFunc(ctx, event)
}

func (h *FunctionHandler) GetEventTypes() []string {
	return []string{} // Default to handle all types
}

func (h *FunctionHandler) GetName() string {
	if h.name == "" {
		return "function_handler"
	}
	return h.name
}

// NewFunctionHandler creates a new function-based event handler
func NewFunctionHandler(name string, fn func(ctx context.Context, event interface{}) error) *FunctionHandler {
	return &FunctionHandler{
		HandlerFunc: fn,
		name:        name,
	}
}
