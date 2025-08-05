package sync

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

// DependencyManager manages dependencies between sync operations
type DependencyManager struct {
	dependencies map[uuid.UUID]*SyncDependency
	graph        *DependencyGraph
	mutex        sync.RWMutex
	tracer       trace.Tracer
}

// DependencyGraph represents the dependency graph
type DependencyGraph struct {
	nodes map[uuid.UUID]*DependencyNode
	edges map[uuid.UUID][]uuid.UUID
}

// DependencyNode represents a node in the dependency graph
type DependencyNode struct {
	JobID        uuid.UUID   `json:"job_id"`
	Status       string      `json:"status"`
	Dependencies []uuid.UUID `json:"dependencies"`
	Dependents   []uuid.UUID `json:"dependents"`
}

// NewDependencyManager creates a new dependency manager
func NewDependencyManager() *DependencyManager {
	return &DependencyManager{
		dependencies: make(map[uuid.UUID]*SyncDependency),
		graph: &DependencyGraph{
			nodes: make(map[uuid.UUID]*DependencyNode),
			edges: make(map[uuid.UUID][]uuid.UUID),
		},
		tracer: otel.Tracer("dependency-manager"),
	}
}

// AddDependency adds a new dependency
func (m *DependencyManager) AddDependency(ctx context.Context, sourceJobID, targetJobID uuid.UUID, depType DependencyType) (*SyncDependency, error) {
	ctx, span := m.tracer.Start(ctx, "add_dependency")
	defer span.End()

	// Check for circular dependencies
	if m.wouldCreateCycle(sourceJobID, targetJobID) {
		return nil, ErrCircularDependency
	}

	dependency := &SyncDependency{
		DependentDataSource: targetJobID,
		DependsOnDataSource: sourceJobID,
		DependencyType:      depType,
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.dependencies[dependency.ID] = dependency
	m.updateGraph(dependency)

	span.SetAttributes(
		attribute.String("dependency.id", dependency.ID.String()),
		attribute.String("source_job_id", sourceJobID.String()),
		attribute.String("target_job_id", targetJobID.String()),
		attribute.String("type", string(depType)),
	)

	return dependency, nil
}

// GetDependencies returns all dependencies for a job
func (m *DependencyManager) GetDependencies(ctx context.Context, jobID uuid.UUID) ([]*SyncDependency, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	var dependencies []*SyncDependency
	for _, dep := range m.dependencies {
		if dep.TargetJobID == jobID {
			dependencies = append(dependencies, dep)
		}
	}

	return dependencies, nil
}

// CanJobStart checks if a job can start based on its dependencies
func (m *DependencyManager) CanJobStart(ctx context.Context, jobID uuid.UUID) (bool, error) {
	ctx, span := m.tracer.Start(ctx, "can_job_start")
	defer span.End()

	dependencies, err := m.GetDependencies(ctx, jobID)
	if err != nil {
		return false, err
	}

	for _, dep := range dependencies {
		if dep.Type == DependencyTypeMandatory && dep.Status != DependencyStatusSatisfied {
			span.SetAttributes(
				attribute.Bool("can_start", false),
				attribute.String("blocking_dependency", dep.ID.String()),
			)
			return false, nil
		}
	}

	span.SetAttributes(attribute.Bool("can_start", true))
	return true, nil
}

// UpdateDependencyStatus updates the status of dependencies based on job status changes
func (m *DependencyManager) UpdateDependencyStatus(ctx context.Context, jobID uuid.UUID, jobStatus string) error {
	ctx, span := m.tracer.Start(ctx, "update_dependency_status")
	defer span.End()

	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Update dependencies where this job is the source
	for _, dep := range m.dependencies {
		if dep.SourceJobID == jobID {
			switch jobStatus {
			case "completed":
				dep.Status = DependencyStatusSatisfied
			case "failed", "cancelled":
				dep.Status = DependencyStatusViolated
			default:
				dep.Status = DependencyStatusPending
			}
			dep.UpdatedAt = time.Now()
		}
	}

	span.SetAttributes(
		attribute.String("job_id", jobID.String()),
		attribute.String("job_status", jobStatus),
	)

	return nil
}

// RemoveDependency removes a dependency
func (m *DependencyManager) RemoveDependency(ctx context.Context, dependencyID uuid.UUID) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	dependency, exists := m.dependencies[dependencyID]
	if !exists {
		return ErrDependencyNotFound
	}

	delete(m.dependencies, dependencyID)
	m.removeFromGraph(dependency)

	return nil
}

// GetExecutionOrder returns the order in which jobs should be executed based on dependencies
func (m *DependencyManager) GetExecutionOrder(ctx context.Context, jobIDs []uuid.UUID) ([]uuid.UUID, error) {
	ctx, span := m.tracer.Start(ctx, "get_execution_order")
	defer span.End()

	// Perform topological sort
	return m.topologicalSort(jobIDs), nil
}

// wouldCreateCycle checks if adding a dependency would create a circular dependency
func (m *DependencyManager) wouldCreateCycle(sourceJobID, targetJobID uuid.UUID) bool {
	// Simple cycle detection (can be improved with proper graph algorithms)
	visited := make(map[uuid.UUID]bool)
	return m.hasCycle(targetJobID, sourceJobID, visited)
}

// hasCycle performs DFS to detect cycles
func (m *DependencyManager) hasCycle(current, target uuid.UUID, visited map[uuid.UUID]bool) bool {
	if current == target {
		return true
	}

	if visited[current] {
		return false
	}

	visited[current] = true

	// Check all dependencies of current
	for _, dep := range m.dependencies {
		if dep.SourceJobID == current {
			if m.hasCycle(dep.TargetJobID, target, visited) {
				return true
			}
		}
	}

	return false
}

// updateGraph updates the internal dependency graph
func (m *DependencyManager) updateGraph(dependency *SyncDependency) {
	// Ensure nodes exist
	if _, exists := m.graph.nodes[dependency.SourceJobID]; !exists {
		m.graph.nodes[dependency.SourceJobID] = &DependencyNode{
			JobID:        dependency.SourceJobID,
			Dependencies: make([]uuid.UUID, 0),
			Dependents:   make([]uuid.UUID, 0),
		}
	}

	if _, exists := m.graph.nodes[dependency.TargetJobID]; !exists {
		m.graph.nodes[dependency.TargetJobID] = &DependencyNode{
			JobID:        dependency.TargetJobID,
			Dependencies: make([]uuid.UUID, 0),
			Dependents:   make([]uuid.UUID, 0),
		}
	}

	// Add edge
	m.graph.edges[dependency.SourceJobID] = append(m.graph.edges[dependency.SourceJobID], dependency.TargetJobID)

	// Update node relationships
	sourceNode := m.graph.nodes[dependency.SourceJobID]
	targetNode := m.graph.nodes[dependency.TargetJobID]

	sourceNode.Dependents = append(sourceNode.Dependents, dependency.TargetJobID)
	targetNode.Dependencies = append(targetNode.Dependencies, dependency.SourceJobID)
}

// removeFromGraph removes a dependency from the graph
func (m *DependencyManager) removeFromGraph(dependency *SyncDependency) {
	// Remove edge
	if edges, exists := m.graph.edges[dependency.SourceJobID]; exists {
		for i, edge := range edges {
			if edge == dependency.TargetJobID {
				m.graph.edges[dependency.SourceJobID] = append(edges[:i], edges[i+1:]...)
				break
			}
		}
	}

	// Update node relationships
	if sourceNode, exists := m.graph.nodes[dependency.SourceJobID]; exists {
		for i, dependent := range sourceNode.Dependents {
			if dependent == dependency.TargetJobID {
				sourceNode.Dependents = append(sourceNode.Dependents[:i], sourceNode.Dependents[i+1:]...)
				break
			}
		}
	}

	if targetNode, exists := m.graph.nodes[dependency.TargetJobID]; exists {
		for i, dep := range targetNode.Dependencies {
			if dep == dependency.SourceJobID {
				targetNode.Dependencies = append(targetNode.Dependencies[:i], targetNode.Dependencies[i+1:]...)
				break
			}
		}
	}
}

// topologicalSort performs topological sorting to determine execution order
func (m *DependencyManager) topologicalSort(jobIDs []uuid.UUID) []uuid.UUID {
	// Simple topological sort implementation
	// In a real implementation, this would use Kahn's algorithm or DFS

	inDegree := make(map[uuid.UUID]int)
	for _, jobID := range jobIDs {
		inDegree[jobID] = 0
	}

	// Calculate in-degrees
	for _, dep := range m.dependencies {
		if _, exists := inDegree[dep.TargetJobID]; exists {
			inDegree[dep.TargetJobID]++
		}
	}

	// Queue for jobs with no dependencies
	queue := make([]uuid.UUID, 0)
	for jobID, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, jobID)
		}
	}

	result := make([]uuid.UUID, 0, len(jobIDs))

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		result = append(result, current)

		// Reduce in-degree for dependent jobs
		for _, dep := range m.dependencies {
			if dep.SourceJobID == current {
				if degree, exists := inDegree[dep.TargetJobID]; exists {
					inDegree[dep.TargetJobID] = degree - 1
					if inDegree[dep.TargetJobID] == 0 {
						queue = append(queue, dep.TargetJobID)
					}
				}
			}
		}
	}

	// Add any remaining jobs (shouldn't happen in a DAG)
	for _, jobID := range jobIDs {
		found := false
		for _, resultID := range result {
			if resultID == jobID {
				found = true
				break
			}
		}
		if !found {
			result = append(result, jobID)
		}
	}

	return result
}

// ResolveDependencies resolves dependencies for a set of jobs
func (m *DependencyManager) ResolveDependencies(ctx context.Context, jobIDs []uuid.UUID) ([]uuid.UUID, error) {
	return m.GetExecutionOrder(ctx, jobIDs)
}

// Common errors
var (
	ErrCircularDependency = fmt.Errorf("circular dependency detected")
	ErrDependencyNotFound = fmt.Errorf("dependency not found")
)
