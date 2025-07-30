package intelligence

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// KnowledgeGraph manages the knowledge graph for content intelligence
type KnowledgeGraph struct {
	config     *KnowledgeGraphConfig
	nodes      map[uuid.UUID]*Node
	edges      map[uuid.UUID]*Edge
	indices    *GraphIndices
	processors map[string]EntityProcessor
	extractors map[string]RelationExtractor
	tracer     trace.Tracer
	mutex      sync.RWMutex
}

// KnowledgeGraphConfig contains configuration for the knowledge graph
type KnowledgeGraphConfig struct {
	Enabled                   bool          `json:"enabled"`
	MaxNodes                  int           `json:"max_nodes"`
	MaxEdges                  int           `json:"max_edges"`
	IndexingEnabled           bool          `json:"indexing_enabled"`
	SemanticSearchEnabled     bool          `json:"semantic_search_enabled"`
	EntityExtractionEnabled   bool          `json:"entity_extraction_enabled"`
	RelationExtractionEnabled bool          `json:"relation_extraction_enabled"`
	AutoUpdateInterval        time.Duration `json:"auto_update_interval"`
	CacheSize                 int           `json:"cache_size"`
	CacheTTL                  time.Duration `json:"cache_ttl"`
	SimilarityThreshold       float64       `json:"similarity_threshold"`
	ConfidenceThreshold       float64       `json:"confidence_threshold"`
	MaxDepth                  int           `json:"max_depth"`
	EnableVersioning          bool          `json:"enable_versioning"`
	EnableProvenanceTracking  bool          `json:"enable_provenance_tracking"`
}

// Node represents a node in the knowledge graph
type Node struct {
	ID          uuid.UUID                 `json:"id"`
	Type        NodeType                  `json:"type"`
	Label       string                    `json:"label"`
	Properties  map[string]interface{}    `json:"properties"`
	Embeddings  []float64                 `json:"embeddings,omitempty"`
	CreatedAt   time.Time                 `json:"created_at"`
	UpdatedAt   time.Time                 `json:"updated_at"`
	Version     int                       `json:"version"`
	
	// Metadata
	SourceDocumentID *uuid.UUID            `json:"source_document_id,omitempty"`
	TenantID         uuid.UUID             `json:"tenant_id"`
	Confidence       float64               `json:"confidence"`
	Weight           float64               `json:"weight"`
	
	// Semantic information
	Synonyms         []string              `json:"synonyms,omitempty"`
	Categories       []string              `json:"categories,omitempty"`
	Tags             []string              `json:"tags,omitempty"`
	
	// Graph metrics
	Degree           int                   `json:"degree"`
	PageRank         float64               `json:"page_rank"`
	BetweennessCentrality float64          `json:"betweenness_centrality"`
	ClusteringCoefficient float64          `json:"clustering_coefficient"`
	
	// Provenance
	Provenance       *ProvenanceInfo       `json:"provenance,omitempty"`
	
	// Status
	Status           NodeStatus            `json:"status"`
	
	// Metadata
	Metadata         map[string]interface{} `json:"metadata"`
}

// Edge represents an edge in the knowledge graph
type Edge struct {
	ID          uuid.UUID                 `json:"id"`
	Type        EdgeType                  `json:"type"`
	Label       string                    `json:"label"`
	SourceID    uuid.UUID                 `json:"source_id"`
	TargetID    uuid.UUID                 `json:"target_id"`
	Properties  map[string]interface{}    `json:"properties"`
	CreatedAt   time.Time                 `json:"created_at"`
	UpdatedAt   time.Time                 `json:"updated_at"`
	Version     int                       `json:"version"`
	
	// Relationship metadata
	Confidence   float64                  `json:"confidence"`
	Weight       float64                  `json:"weight"`
	Strength     float64                  `json:"strength"`
	Direction    EdgeDirection            `json:"direction"`
	
	// Temporal information
	ValidFrom    *time.Time               `json:"valid_from,omitempty"`
	ValidTo      *time.Time               `json:"valid_to,omitempty"`
	Frequency    int                      `json:"frequency"`
	
	// Source information
	SourceDocumentID *uuid.UUID           `json:"source_document_id,omitempty"`
	TenantID         uuid.UUID            `json:"tenant_id"`
	
	// Provenance
	Provenance   *ProvenanceInfo          `json:"provenance,omitempty"`
	
	// Status
	Status       EdgeStatus               `json:"status"`
	
	// Metadata
	Metadata     map[string]interface{}   `json:"metadata"`
}

// GraphIndices contains various indices for efficient querying
type GraphIndices struct {
	NodeTypeIndex    map[NodeType][]uuid.UUID          `json:"node_type_index"`
	EdgeTypeIndex    map[EdgeType][]uuid.UUID          `json:"edge_type_index"`
	LabelIndex       map[string][]uuid.UUID            `json:"label_index"`
	PropertyIndex    map[string]map[string][]uuid.UUID `json:"property_index"`
	AdjacencyList    map[uuid.UUID][]uuid.UUID         `json:"adjacency_list"`
	IncomingEdges    map[uuid.UUID][]uuid.UUID         `json:"incoming_edges"`
	OutgoingEdges    map[uuid.UUID][]uuid.UUID         `json:"outgoing_edges"`
	SemanticIndex    *SemanticIndex                    `json:"semantic_index,omitempty"`
	LastUpdated      time.Time                         `json:"last_updated"`
}

// SemanticIndex provides semantic search capabilities
type SemanticIndex struct {
	VectorIndex      map[uuid.UUID][]float64           `json:"vector_index"`
	ClusterIndex     map[string][]uuid.UUID            `json:"cluster_index"`
	SimilarityCache  map[string][]SimilarityResult     `json:"similarity_cache"`
	ConceptHierarchy *ConceptHierarchy                 `json:"concept_hierarchy,omitempty"`
	LastUpdated      time.Time                         `json:"last_updated"`
}

// ConceptHierarchy represents hierarchical relationships between concepts
type ConceptHierarchy struct {
	Root     *ConceptNode                      `json:"root"`
	Concepts map[string]*ConceptNode           `json:"concepts"`
	Depth    int                               `json:"depth"`
}

// ConceptNode represents a node in the concept hierarchy
type ConceptNode struct {
	ID       string                           `json:"id"`
	Label    string                           `json:"label"`
	Parent   *ConceptNode                     `json:"parent,omitempty"`
	Children []*ConceptNode                   `json:"children"`
	Level    int                              `json:"level"`
	Entities []uuid.UUID                      `json:"entities"`
	Score    float64                          `json:"score"`
}

// Enums and types

type NodeType string

const (
	NodeTypeEntity      NodeType = "entity"
	NodeTypeConcept     NodeType = "concept"
	NodeTypeDocument    NodeType = "document"
	NodeTypePerson      NodeType = "person"
	NodeTypeOrganization NodeType = "organization"
	NodeTypeLocation    NodeType = "location"
	NodeTypeEvent       NodeType = "event"
	NodeTypeTopic       NodeType = "topic"
	NodeTypeKeyword     NodeType = "keyword"
	NodeTypeCategory    NodeType = "category"
	NodeTypeTag         NodeType = "tag"
)

type EdgeType string

const (
	EdgeTypeRelatedTo      EdgeType = "related_to"
	EdgeTypeMentions       EdgeType = "mentions"
	EdgeTypeContains       EdgeType = "contains"
	EdgeTypePartOf         EdgeType = "part_of"
	EdgeTypeIsA            EdgeType = "is_a"
	EdgeTypeSubClassOf     EdgeType = "subclass_of"
	EdgeTypeInstanceOf     EdgeType = "instance_of"
	EdgeTypeSimilarTo      EdgeType = "similar_to"
	EdgeTypeReferencedBy   EdgeType = "referenced_by"
	EdgeTypeInfluences     EdgeType = "influences"
	EdgeTypeDependsOn      EdgeType = "depends_on"
	EdgeTypeOwns           EdgeType = "owns"
	EdgeTypeWorksFor       EdgeType = "works_for"
	EdgeTypeLocatedIn      EdgeType = "located_in"
	EdgeTypeOccurredAt     EdgeType = "occurred_at"
)

type EdgeDirection string

const (
	EdgeDirectionUndirected EdgeDirection = "undirected"
	EdgeDirectionDirected   EdgeDirection = "directed"
	EdgeDirectionBoth       EdgeDirection = "both"
)

type NodeStatus string

const (
	NodeStatusActive     NodeStatus = "active"
	NodeStatusInactive   NodeStatus = "inactive"
	NodeStatusPending    NodeStatus = "pending"
	NodeStatusArchived   NodeStatus = "archived"
)

type EdgeStatus string

const (
	EdgeStatusActive     EdgeStatus = "active"
	EdgeStatusInactive   EdgeStatus = "inactive"
	EdgeStatusPending    EdgeStatus = "pending"
	EdgeStatusArchived   EdgeStatus = "archived"
)

// Supporting types

type ProvenanceInfo struct {
	Source       string                    `json:"source"`
	Method       string                    `json:"method"`
	Confidence   float64                   `json:"confidence"`
	Timestamp    time.Time                 `json:"timestamp"`
	Version      string                    `json:"version"`
	Agent        string                    `json:"agent,omitempty"`
	Evidence     []EvidenceItem            `json:"evidence,omitempty"`
	Metadata     map[string]interface{}    `json:"metadata,omitempty"`
}

type EvidenceItem struct {
	Type         string                    `json:"type"`
	Source       string                    `json:"source"`
	Content      string                    `json:"content"`
	Confidence   float64                   `json:"confidence"`
	Position     *TextPosition             `json:"position,omitempty"`
	Context      string                    `json:"context,omitempty"`
}

type TextPosition struct {
	StartOffset  int `json:"start_offset"`
	EndOffset    int `json:"end_offset"`
	LineNumber   int `json:"line_number"`
	ColumnNumber int `json:"column_number"`
}

type SimilarityResult struct {
	NodeID     uuid.UUID `json:"node_id"`
	Similarity float64   `json:"similarity"`
	Type       string    `json:"type"`
}

// EntityProcessor interface for processing entities
type EntityProcessor interface {
	Name() string
	ProcessDocument(ctx context.Context, document *Document) ([]*Entity, error)
	ExtractEntities(ctx context.Context, text string) ([]*Entity, error)
	GetSupportedTypes() []NodeType
}

// RelationExtractor interface for extracting relationships
type RelationExtractor interface {
	Name() string
	ExtractRelations(ctx context.Context, entities []*Entity, text string) ([]*Relation, error)
	GetSupportedRelations() []EdgeType
}

// Document represents a document for processing
type Document struct {
	ID          uuid.UUID                 `json:"id"`
	TenantID    uuid.UUID                 `json:"tenant_id"`
	Title       string                    `json:"title"`
	Content     string                    `json:"content"`
	ContentType string                    `json:"content_type"`
	Metadata    map[string]interface{}    `json:"metadata"`
	CreatedAt   time.Time                 `json:"created_at"`
}

// Entity represents an extracted entity
type Entity struct {
	ID         uuid.UUID                 `json:"id"`
	Type       NodeType                  `json:"type"`
	Label      string                    `json:"label"`
	Text       string                    `json:"text"`
	Position   *TextPosition             `json:"position"`
	Confidence float64                   `json:"confidence"`
	Properties map[string]interface{}    `json:"properties"`
	Context    string                    `json:"context,omitempty"`
}

// Relation represents an extracted relationship
type Relation struct {
	ID         uuid.UUID                 `json:"id"`
	Type       EdgeType                  `json:"type"`
	Label      string                    `json:"label"`
	SourceID   uuid.UUID                 `json:"source_id"`
	TargetID   uuid.UUID                 `json:"target_id"`
	Confidence float64                   `json:"confidence"`
	Properties map[string]interface{}    `json:"properties"`
	Context    string                    `json:"context,omitempty"`
	Evidence   []EvidenceItem            `json:"evidence,omitempty"`
}

// Query types

type GraphQuery struct {
	Type         QueryType                 `json:"type"`
	Parameters   map[string]interface{}    `json:"parameters"`
	Filters      []QueryFilter             `json:"filters,omitempty"`
	Limit        int                       `json:"limit,omitempty"`
	Offset       int                       `json:"offset,omitempty"`
	OrderBy      string                    `json:"order_by,omitempty"`
	OrderDirection string                  `json:"order_direction,omitempty"`
}

type QueryType string

const (
	QueryTypeNodesByType        QueryType = "nodes_by_type"
	QueryTypeNodesByLabel       QueryType = "nodes_by_label"
	QueryTypeNodesByProperty    QueryType = "nodes_by_property"
	QueryTypeEdgesByType        QueryType = "edges_by_type"
	QueryTypeNeighbors          QueryType = "neighbors"
	QueryTypeShortestPath       QueryType = "shortest_path"
	QueryTypeSemanticSimilarity QueryType = "semantic_similarity"
	QueryTypeSubgraph           QueryType = "subgraph"
	QueryTypeCentrality         QueryType = "centrality"
	QueryTypeCommunityDetection QueryType = "community_detection"
)

type QueryFilter struct {
	Field    string      `json:"field"`
	Operator string      `json:"operator"`
	Value    interface{} `json:"value"`
}

type QueryResult struct {
	Nodes        []*Node                   `json:"nodes,omitempty"`
	Edges        []*Edge                   `json:"edges,omitempty"`
	Paths        []GraphPath               `json:"paths,omitempty"`
	Communities  []Community               `json:"communities,omitempty"`
	Similarities []SimilarityResult        `json:"similarities,omitempty"`
	Metadata     map[string]interface{}    `json:"metadata"`
	ExecutionTime time.Duration            `json:"execution_time"`
}

type GraphPath struct {
	Nodes    []uuid.UUID `json:"nodes"`
	Edges    []uuid.UUID `json:"edges"`
	Length   int         `json:"length"`
	Weight   float64     `json:"weight"`
	Cost     float64     `json:"cost"`
}

type Community struct {
	ID       string      `json:"id"`
	Nodes    []uuid.UUID `json:"nodes"`
	Edges    []uuid.UUID `json:"edges"`
	Size     int         `json:"size"`
	Density  float64     `json:"density"`
	Modularity float64   `json:"modularity"`
	Score    float64     `json:"score"`
}

// Analytics types

type GraphAnalytics struct {
	NodeCount         int                       `json:"node_count"`
	EdgeCount         int                       `json:"edge_count"`
	Density           float64                   `json:"density"`
	Diameter          int                       `json:"diameter"`
	AveragePathLength float64                   `json:"average_path_length"`
	ClusteringCoefficient float64               `json:"clustering_coefficient"`
	ConnectedComponents []ConnectedComponent    `json:"connected_components"`
	CentralityMetrics *CentralityMetrics        `json:"centrality_metrics"`
	CommunityStructure *CommunityStructure      `json:"community_structure"`
	TemporalAnalysis  *TemporalAnalysis         `json:"temporal_analysis,omitempty"`
	ComputedAt        time.Time                 `json:"computed_at"`
}

type ConnectedComponent struct {
	ID    string      `json:"id"`
	Nodes []uuid.UUID `json:"nodes"`
	Size  int         `json:"size"`
}

type CentralityMetrics struct {
	PageRank              map[uuid.UUID]float64 `json:"page_rank"`
	BetweennessCentrality map[uuid.UUID]float64 `json:"betweenness_centrality"`
	ClosenessCentrality   map[uuid.UUID]float64 `json:"closeness_centrality"`
	EigenvectorCentrality map[uuid.UUID]float64 `json:"eigenvector_centrality"`
	DegreeCentrality      map[uuid.UUID]float64 `json:"degree_centrality"`
}

type CommunityStructure struct {
	Communities    []Community `json:"communities"`
	Modularity     float64     `json:"modularity"`
	NumCommunities int         `json:"num_communities"`
	SilhouetteScore float64    `json:"silhouette_score"`
}

type TemporalAnalysis struct {
	GrowthRate        float64                   `json:"growth_rate"`
	ActivityPeaks     []time.Time               `json:"activity_peaks"`
	EvolutionMetrics  []EvolutionMetric         `json:"evolution_metrics"`
	TrendAnalysis     *TrendAnalysis            `json:"trend_analysis"`
}

type EvolutionMetric struct {
	Timestamp   time.Time `json:"timestamp"`
	NodeCount   int       `json:"node_count"`
	EdgeCount   int       `json:"edge_count"`
	Density     float64   `json:"density"`
	Modularity  float64   `json:"modularity"`
}

type TrendAnalysis struct {
	NodeGrowthTrend string  `json:"node_growth_trend"`
	EdgeGrowthTrend string  `json:"edge_growth_trend"`
	DensityTrend    string  `json:"density_trend"`
	GrowthRate      float64 `json:"growth_rate"`
	Seasonality     bool    `json:"seasonality"`
}

// NewKnowledgeGraph creates a new knowledge graph
func NewKnowledgeGraph(config *KnowledgeGraphConfig) *KnowledgeGraph {
	if config == nil {
		config = &KnowledgeGraphConfig{
			Enabled:                   true,
			MaxNodes:                  1000000,
			MaxEdges:                  5000000,
			IndexingEnabled:           true,
			SemanticSearchEnabled:     true,
			EntityExtractionEnabled:   true,
			RelationExtractionEnabled: true,
			AutoUpdateInterval:        1 * time.Hour,
			CacheSize:                 10000,
			CacheTTL:                  1 * time.Hour,
			SimilarityThreshold:       0.7,
			ConfidenceThreshold:       0.6,
			MaxDepth:                  10,
			EnableVersioning:          true,
			EnableProvenanceTracking:  true,
		}
	}

	kg := &KnowledgeGraph{
		config:     config,
		nodes:      make(map[uuid.UUID]*Node),
		edges:      make(map[uuid.UUID]*Edge),
		processors: make(map[string]EntityProcessor),
		extractors: make(map[string]RelationExtractor),
		tracer:     otel.Tracer("knowledge-graph"),
		indices: &GraphIndices{
			NodeTypeIndex: make(map[NodeType][]uuid.UUID),
			EdgeTypeIndex: make(map[EdgeType][]uuid.UUID),
			LabelIndex:    make(map[string][]uuid.UUID),
			PropertyIndex: make(map[string]map[string][]uuid.UUID),
			AdjacencyList: make(map[uuid.UUID][]uuid.UUID),
			IncomingEdges: make(map[uuid.UUID][]uuid.UUID),
			OutgoingEdges: make(map[uuid.UUID][]uuid.UUID),
		},
	}

	if config.SemanticSearchEnabled {
		kg.indices.SemanticIndex = &SemanticIndex{
			VectorIndex:     make(map[uuid.UUID][]float64),
			ClusterIndex:    make(map[string][]uuid.UUID),
			SimilarityCache: make(map[string][]SimilarityResult),
		}
	}

	// Register default processors and extractors
	kg.registerDefaultProcessors()

	return kg
}

// AddNode adds a node to the knowledge graph
func (kg *KnowledgeGraph) AddNode(ctx context.Context, node *Node) error {
	ctx, span := kg.tracer.Start(ctx, "knowledge_graph.add_node")
	defer span.End()

	if node.ID == uuid.Nil {
		node.ID = uuid.New()
	}

	node.CreatedAt = time.Now()
	node.UpdatedAt = time.Now()
	node.Version = 1
	node.Status = NodeStatusActive

	if node.Weight == 0 {
		node.Weight = 1.0
	}

	kg.mutex.Lock()
	defer kg.mutex.Unlock()

	// Check if node already exists
	if _, exists := kg.nodes[node.ID]; exists {
		return fmt.Errorf("node already exists: %s", node.ID)
	}

	// Check limits
	if len(kg.nodes) >= kg.config.MaxNodes {
		return fmt.Errorf("maximum number of nodes reached: %d", kg.config.MaxNodes)
	}

	kg.nodes[node.ID] = node

	// Update indices
	if kg.config.IndexingEnabled {
		kg.updateNodeIndices(node)
	}

	span.SetAttributes(
		attribute.String("node_id", node.ID.String()),
		attribute.String("node_type", string(node.Type)),
		attribute.String("node_label", node.Label),
	)

	log.Printf("Node added to knowledge graph: %s (%s)", node.Label, node.ID)
	return nil
}

// AddEdge adds an edge to the knowledge graph
func (kg *KnowledgeGraph) AddEdge(ctx context.Context, edge *Edge) error {
	ctx, span := kg.tracer.Start(ctx, "knowledge_graph.add_edge")
	defer span.End()

	if edge.ID == uuid.Nil {
		edge.ID = uuid.New()
	}

	edge.CreatedAt = time.Now()
	edge.UpdatedAt = time.Now()
	edge.Version = 1
	edge.Status = EdgeStatusActive

	if edge.Weight == 0 {
		edge.Weight = 1.0
	}
	if edge.Direction == "" {
		edge.Direction = EdgeDirectionDirected
	}

	kg.mutex.Lock()
	defer kg.mutex.Unlock()

	// Validate source and target nodes exist
	if _, exists := kg.nodes[edge.SourceID]; !exists {
		return fmt.Errorf("source node not found: %s", edge.SourceID)
	}
	if _, exists := kg.nodes[edge.TargetID]; !exists {
		return fmt.Errorf("target node not found: %s", edge.TargetID)
	}

	// Check if edge already exists
	if _, exists := kg.edges[edge.ID]; exists {
		return fmt.Errorf("edge already exists: %s", edge.ID)
	}

	// Check limits
	if len(kg.edges) >= kg.config.MaxEdges {
		return fmt.Errorf("maximum number of edges reached: %d", kg.config.MaxEdges)
	}

	kg.edges[edge.ID] = edge

	// Update indices
	if kg.config.IndexingEnabled {
		kg.updateEdgeIndices(edge)
	}

	// Update node degrees
	kg.nodes[edge.SourceID].Degree++
	kg.nodes[edge.TargetID].Degree++

	span.SetAttributes(
		attribute.String("edge_id", edge.ID.String()),
		attribute.String("edge_type", string(edge.Type)),
		attribute.String("source_id", edge.SourceID.String()),
		attribute.String("target_id", edge.TargetID.String()),
	)

	log.Printf("Edge added to knowledge graph: %s -> %s (%s)", edge.SourceID, edge.TargetID, edge.Type)
	return nil
}

// ProcessDocument processes a document and extracts entities and relationships
func (kg *KnowledgeGraph) ProcessDocument(ctx context.Context, document *Document) (*DocumentProcessingResult, error) {
	ctx, span := kg.tracer.Start(ctx, "knowledge_graph.process_document")
	defer span.End()

	result := &DocumentProcessingResult{
		DocumentID:  document.ID,
		TenantID:    document.TenantID,
		ProcessedAt: time.Now(),
		Entities:    make([]*Entity, 0),
		Relations:   make([]*Relation, 0),
		NodesAdded:  0,
		EdgesAdded:  0,
	}

	// Extract entities
	if kg.config.EntityExtractionEnabled {
		for _, processor := range kg.processors {
			entities, err := processor.ProcessDocument(ctx, document)
			if err != nil {
				log.Printf("Entity extraction failed: %v", err)
				continue
			}
			result.Entities = append(result.Entities, entities...)
		}
	}

	// Convert entities to nodes and add to graph
	nodeMap := make(map[string]uuid.UUID)
	for _, entity := range result.Entities {
		node := &Node{
			Type:             entity.Type,
			Label:            entity.Label,
			Properties:       entity.Properties,
			SourceDocumentID: &document.ID,
			TenantID:         document.TenantID,
			Confidence:       entity.Confidence,
			Weight:           1.0,
			Metadata:         make(map[string]interface{}),
		}

		// Check for existing similar nodes
		existingNode := kg.findSimilarNode(ctx, node)
		if existingNode != nil {
			// Update existing node
			kg.updateNode(ctx, existingNode.ID, node)
			nodeMap[entity.Label] = existingNode.ID
		} else {
			// Add new node
			err := kg.AddNode(ctx, node)
			if err != nil {
				log.Printf("Failed to add node: %v", err)
				continue
			}
			nodeMap[entity.Label] = node.ID
			result.NodesAdded++
		}
	}

	// Extract relationships
	if kg.config.RelationExtractionEnabled {
		for _, extractor := range kg.extractors {
			relations, err := extractor.ExtractRelations(ctx, result.Entities, document.Content)
			if err != nil {
				log.Printf("Relation extraction failed: %v", err)
				continue
			}
			result.Relations = append(result.Relations, relations...)
		}
	}

	// Convert relations to edges and add to graph
	for _, relation := range result.Relations {
		sourceID, sourceExists := nodeMap[kg.getEntityLabel(relation.SourceID, result.Entities)]
		targetID, targetExists := nodeMap[kg.getEntityLabel(relation.TargetID, result.Entities)]

		if !sourceExists || !targetExists {
			continue
		}

		edge := &Edge{
			Type:             relation.Type,
			Label:            relation.Label,
			SourceID:         sourceID,
			TargetID:         targetID,
			Properties:       relation.Properties,
			SourceDocumentID: &document.ID,
			TenantID:         document.TenantID,
			Confidence:       relation.Confidence,
			Weight:           1.0,
			Direction:        EdgeDirectionDirected,
			Metadata:         make(map[string]interface{}),
		}

		err := kg.AddEdge(ctx, edge)
		if err != nil {
			log.Printf("Failed to add edge: %v", err)
			continue
		}
		result.EdgesAdded++
	}

	span.SetAttributes(
		attribute.String("document_id", document.ID.String()),
		attribute.Int("entities_extracted", len(result.Entities)),
		attribute.Int("relations_extracted", len(result.Relations)),
		attribute.Int("nodes_added", result.NodesAdded),
		attribute.Int("edges_added", result.EdgesAdded),
	)

	log.Printf("Document processed: %s - %d entities, %d relations, %d nodes added, %d edges added",
		document.ID, len(result.Entities), len(result.Relations), result.NodesAdded, result.EdgesAdded)

	return result, nil
}

// Query executes a query against the knowledge graph
func (kg *KnowledgeGraph) Query(ctx context.Context, query *GraphQuery) (*QueryResult, error) {
	ctx, span := kg.tracer.Start(ctx, "knowledge_graph.query")
	defer span.End()

	startTime := time.Now()
	result := &QueryResult{
		Metadata: make(map[string]interface{}),
	}

	kg.mutex.RLock()
	defer kg.mutex.RUnlock()

	switch query.Type {
	case QueryTypeNodesByType:
		result.Nodes = kg.queryNodesByType(query)
	case QueryTypeNodesByLabel:
		result.Nodes = kg.queryNodesByLabel(query)
	case QueryTypeNeighbors:
		result.Nodes, result.Edges = kg.queryNeighbors(query)
	case QueryTypeShortestPath:
		result.Paths = kg.queryShortestPath(query)
	case QueryTypeSemanticSimilarity:
		result.Similarities = kg.querySemanticSimilarity(query)
	case QueryTypeCommunityDetection:
		result.Communities = kg.queryCommunities(query)
	default:
		return nil, fmt.Errorf("unsupported query type: %s", query.Type)
	}

	result.ExecutionTime = time.Since(startTime)

	span.SetAttributes(
		attribute.String("query_type", string(query.Type)),
		attribute.Int("result_nodes", len(result.Nodes)),
		attribute.Int("result_edges", len(result.Edges)),
		attribute.Int64("execution_time_ms", result.ExecutionTime.Milliseconds()),
	)

	return result, nil
}

// GetAnalytics computes and returns graph analytics
func (kg *KnowledgeGraph) GetAnalytics(ctx context.Context) (*GraphAnalytics, error) {
	ctx, span := kg.tracer.Start(ctx, "knowledge_graph.get_analytics")
	defer span.End()

	kg.mutex.RLock()
	defer kg.mutex.RUnlock()

	analytics := &GraphAnalytics{
		NodeCount: len(kg.nodes),
		EdgeCount: len(kg.edges),
		ComputedAt: time.Now(),
	}

	// Compute basic metrics
	if analytics.NodeCount > 0 && analytics.EdgeCount > 0 {
		maxPossibleEdges := analytics.NodeCount * (analytics.NodeCount - 1) / 2
		analytics.Density = float64(analytics.EdgeCount) / float64(maxPossibleEdges)
	}

	// Compute centrality metrics
	analytics.CentralityMetrics = kg.computeCentralityMetrics()

	// Detect communities
	analytics.CommunityStructure = kg.detectCommunities()

	// Compute connected components
	analytics.ConnectedComponents = kg.findConnectedComponents()

	span.SetAttributes(
		attribute.Int("node_count", analytics.NodeCount),
		attribute.Int("edge_count", analytics.EdgeCount),
		attribute.Float64("density", analytics.Density),
	)

	return analytics, nil
}

// Internal methods

func (kg *KnowledgeGraph) updateNodeIndices(node *Node) {
	// Update type index
	kg.indices.NodeTypeIndex[node.Type] = append(kg.indices.NodeTypeIndex[node.Type], node.ID)

	// Update label index
	kg.indices.LabelIndex[node.Label] = append(kg.indices.LabelIndex[node.Label], node.ID)

	// Update property indices
	for key, value := range node.Properties {
		if kg.indices.PropertyIndex[key] == nil {
			kg.indices.PropertyIndex[key] = make(map[string][]uuid.UUID)
		}
		valueStr := fmt.Sprintf("%v", value)
		kg.indices.PropertyIndex[key][valueStr] = append(kg.indices.PropertyIndex[key][valueStr], node.ID)
	}

	kg.indices.LastUpdated = time.Now()
}

func (kg *KnowledgeGraph) updateEdgeIndices(edge *Edge) {
	// Update type index
	kg.indices.EdgeTypeIndex[edge.Type] = append(kg.indices.EdgeTypeIndex[edge.Type], edge.ID)

	// Update adjacency lists
	kg.indices.AdjacencyList[edge.SourceID] = append(kg.indices.AdjacencyList[edge.SourceID], edge.TargetID)
	kg.indices.OutgoingEdges[edge.SourceID] = append(kg.indices.OutgoingEdges[edge.SourceID], edge.ID)
	kg.indices.IncomingEdges[edge.TargetID] = append(kg.indices.IncomingEdges[edge.TargetID], edge.ID)

	if edge.Direction == EdgeDirectionUndirected || edge.Direction == EdgeDirectionBoth {
		kg.indices.AdjacencyList[edge.TargetID] = append(kg.indices.AdjacencyList[edge.TargetID], edge.SourceID)
		kg.indices.OutgoingEdges[edge.TargetID] = append(kg.indices.OutgoingEdges[edge.TargetID], edge.ID)
		kg.indices.IncomingEdges[edge.SourceID] = append(kg.indices.IncomingEdges[edge.SourceID], edge.ID)
	}

	kg.indices.LastUpdated = time.Now()
}

func (kg *KnowledgeGraph) findSimilarNode(ctx context.Context, node *Node) *Node {
	// Simple similarity check based on label and type
	for _, existingNode := range kg.nodes {
		if existingNode.Type == node.Type && 
		   existingNode.TenantID == node.TenantID &&
		   strings.EqualFold(existingNode.Label, node.Label) {
			return existingNode
		}
	}
	return nil
}

func (kg *KnowledgeGraph) updateNode(ctx context.Context, nodeID uuid.UUID, updates *Node) error {
	node, exists := kg.nodes[nodeID]
	if !exists {
		return fmt.Errorf("node not found: %s", nodeID)
	}

	// Update properties
	for key, value := range updates.Properties {
		node.Properties[key] = value
	}

	// Update confidence (average with existing)
	node.Confidence = (node.Confidence + updates.Confidence) / 2.0

	// Increment weight
	node.Weight += updates.Weight

	node.UpdatedAt = time.Now()
	node.Version++

	return nil
}

func (kg *KnowledgeGraph) getEntityLabel(entityID uuid.UUID, entities []*Entity) string {
	for _, entity := range entities {
		if entity.ID == entityID {
			return entity.Label
		}
	}
	return ""
}

// Query implementations

func (kg *KnowledgeGraph) queryNodesByType(query *GraphQuery) []*Node {
	nodeType := NodeType(query.Parameters["type"].(string))
	nodeIDs := kg.indices.NodeTypeIndex[nodeType]

	var nodes []*Node
	for _, nodeID := range nodeIDs {
		if node, exists := kg.nodes[nodeID]; exists {
			if kg.matchesFilters(node, query.Filters) {
				nodes = append(nodes, node)
			}
		}
	}

	// Apply limit
	if query.Limit > 0 && len(nodes) > query.Limit {
		nodes = nodes[:query.Limit]
	}

	return nodes
}

func (kg *KnowledgeGraph) queryNodesByLabel(query *GraphQuery) []*Node {
	label := query.Parameters["label"].(string)
	nodeIDs := kg.indices.LabelIndex[label]

	var nodes []*Node
	for _, nodeID := range nodeIDs {
		if node, exists := kg.nodes[nodeID]; exists {
			if kg.matchesFilters(node, query.Filters) {
				nodes = append(nodes, node)
			}
		}
	}

	// Apply limit
	if query.Limit > 0 && len(nodes) > query.Limit {
		nodes = nodes[:query.Limit]
	}

	return nodes
}

func (kg *KnowledgeGraph) queryNeighbors(query *GraphQuery) ([]*Node, []*Edge) {
	nodeID := uuid.MustParse(query.Parameters["node_id"].(string))
	maxHops := 1
	if hops, exists := query.Parameters["max_hops"]; exists {
		maxHops = int(hops.(float64))
	}

	visited := make(map[uuid.UUID]bool)
	var nodes []*Node
	var edges []*Edge

	kg.bfsTraversal(nodeID, maxHops, visited, &nodes, &edges)

	return nodes, edges
}

func (kg *KnowledgeGraph) queryShortestPath(query *GraphQuery) []GraphPath {
	sourceID := uuid.MustParse(query.Parameters["source_id"].(string))
	targetID := uuid.MustParse(query.Parameters["target_id"].(string))

	path := kg.dijkstraShortestPath(sourceID, targetID)
	if path != nil {
		return []GraphPath{*path}
	}

	return []GraphPath{}
}

func (kg *KnowledgeGraph) querySemanticSimilarity(query *GraphQuery) []SimilarityResult {
	nodeID := uuid.MustParse(query.Parameters["node_id"].(string))
	threshold := kg.config.SimilarityThreshold
	if t, exists := query.Parameters["threshold"]; exists {
		threshold = t.(float64)
	}

	node, exists := kg.nodes[nodeID]
	if !exists || len(node.Embeddings) == 0 {
		return []SimilarityResult{}
	}

	var results []SimilarityResult
	for id, otherNode := range kg.nodes {
		if id == nodeID || len(otherNode.Embeddings) == 0 {
			continue
		}

		similarity := kg.cosineSimilarity(node.Embeddings, otherNode.Embeddings)
		if similarity >= threshold {
			results = append(results, SimilarityResult{
				NodeID:     id,
				Similarity: similarity,
				Type:       "cosine",
			})
		}
	}

	// Sort by similarity (descending)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Similarity > results[j].Similarity
	})

	// Apply limit
	if query.Limit > 0 && len(results) > query.Limit {
		results = results[:query.Limit]
	}

	return results
}

func (kg *KnowledgeGraph) queryCommunities(query *GraphQuery) []Community {
	// Simple community detection based on connected components
	components := kg.findConnectedComponents()
	var communities []Community

	for i, component := range components {
		if component.Size < 3 { // Skip small components
			continue
		}

		// Calculate edges within the component
		var componentEdges []uuid.UUID
		for _, edge := range kg.edges {
			sourceInComponent := false
			targetInComponent := false
			for _, nodeID := range component.Nodes {
				if edge.SourceID == nodeID {
					sourceInComponent = true
				}
				if edge.TargetID == nodeID {
					targetInComponent = true
				}
			}
			if sourceInComponent && targetInComponent {
				componentEdges = append(componentEdges, edge.ID)
			}
		}

		density := float64(len(componentEdges)) / float64(component.Size*(component.Size-1)/2)

		community := Community{
			ID:       fmt.Sprintf("community_%d", i),
			Nodes:    component.Nodes,
			Edges:    componentEdges,
			Size:     component.Size,
			Density:  density,
			Score:    density * float64(component.Size),
		}

		communities = append(communities, community)
	}

	return communities
}

// Graph algorithms

func (kg *KnowledgeGraph) bfsTraversal(startID uuid.UUID, maxHops int, visited map[uuid.UUID]bool, nodes *[]*Node, edges *[]*Edge) {
	if maxHops <= 0 || visited[startID] {
		return
	}

	visited[startID] = true
	if node, exists := kg.nodes[startID]; exists {
		*nodes = append(*nodes, node)
	}

	// Get neighbors
	neighbors := kg.indices.AdjacencyList[startID]
	for _, neighborID := range neighbors {
		if !visited[neighborID] {
			// Find connecting edge
			for _, edge := range kg.edges {
				if (edge.SourceID == startID && edge.TargetID == neighborID) ||
				   (edge.TargetID == startID && edge.SourceID == neighborID) {
					*edges = append(*edges, edge)
					break
				}
			}
			
			kg.bfsTraversal(neighborID, maxHops-1, visited, nodes, edges)
		}
	}
}

func (kg *KnowledgeGraph) dijkstraShortestPath(sourceID, targetID uuid.UUID) *GraphPath {
	// Simplified Dijkstra implementation
	distances := make(map[uuid.UUID]float64)
	previous := make(map[uuid.UUID]uuid.UUID)
	unvisited := make(map[uuid.UUID]bool)

	// Initialize
	for nodeID := range kg.nodes {
		distances[nodeID] = math.Inf(1)
		unvisited[nodeID] = true
	}
	distances[sourceID] = 0

	for len(unvisited) > 0 {
		// Find node with minimum distance
		var currentID uuid.UUID
		minDist := math.Inf(1)
		for nodeID := range unvisited {
			if distances[nodeID] < minDist {
				minDist = distances[nodeID]
				currentID = nodeID
			}
		}

		if minDist == math.Inf(1) {
			break // No path found
		}

		delete(unvisited, currentID)

		if currentID == targetID {
			// Reconstruct path
			var pathNodes []uuid.UUID
			current := targetID
			for current != uuid.Nil {
				pathNodes = append([]uuid.UUID{current}, pathNodes...)
				current = previous[current]
			}

			return &GraphPath{
				Nodes:  pathNodes,
				Length: len(pathNodes) - 1,
				Weight: distances[targetID],
			}
		}

		// Update neighbors
		neighbors := kg.indices.AdjacencyList[currentID]
		for _, neighborID := range neighbors {
			if !unvisited[neighborID] {
				continue
			}

			// Find edge weight
			edgeWeight := 1.0
			for _, edge := range kg.edges {
				if (edge.SourceID == currentID && edge.TargetID == neighborID) ||
				   (edge.TargetID == currentID && edge.SourceID == neighborID) {
					edgeWeight = edge.Weight
					break
				}
			}

			alt := distances[currentID] + edgeWeight
			if alt < distances[neighborID] {
				distances[neighborID] = alt
				previous[neighborID] = currentID
			}
		}
	}

	return nil // No path found
}

func (kg *KnowledgeGraph) computeCentralityMetrics() *CentralityMetrics {
	metrics := &CentralityMetrics{
		PageRank:              make(map[uuid.UUID]float64),
		BetweennessCentrality: make(map[uuid.UUID]float64),
		DegreeCentrality:      make(map[uuid.UUID]float64),
	}

	// Compute degree centrality
	for nodeID, node := range kg.nodes {
		metrics.DegreeCentrality[nodeID] = float64(node.Degree) / float64(len(kg.nodes)-1)
	}

	// Simplified PageRank (power iteration method)
	damping := 0.85
	iterations := 100
	tolerance := 1e-6

	// Initialize PageRank values
	numNodes := float64(len(kg.nodes))
	for nodeID := range kg.nodes {
		metrics.PageRank[nodeID] = 1.0 / numNodes
	}

	for iter := 0; iter < iterations; iter++ {
		newPageRank := make(map[uuid.UUID]float64)
		
		for nodeID := range kg.nodes {
			newPageRank[nodeID] = (1.0 - damping) / numNodes
			
			// Sum contributions from incoming nodes
			incomingEdges := kg.indices.IncomingEdges[nodeID]
			for _, edgeID := range incomingEdges {
				edge := kg.edges[edgeID]
				sourceOutDegree := float64(len(kg.indices.OutgoingEdges[edge.SourceID]))
				if sourceOutDegree > 0 {
					newPageRank[nodeID] += damping * metrics.PageRank[edge.SourceID] / sourceOutDegree
				}
			}
		}

		// Check convergence
		converged := true
		for nodeID := range kg.nodes {
			if math.Abs(newPageRank[nodeID]-metrics.PageRank[nodeID]) > tolerance {
				converged = false
				break
			}
		}

		metrics.PageRank = newPageRank

		if converged {
			break
		}
	}

	return metrics
}

func (kg *KnowledgeGraph) detectCommunities() *CommunityStructure {
	communities := kg.queryCommunities(&GraphQuery{Type: QueryTypeCommunityDetection})
	
	// Calculate modularity
	modularity := 0.0
	totalEdges := float64(len(kg.edges))
	
	for _, community := range communities {
		internal := float64(len(community.Edges))
		total := 0.0
		for _, nodeID := range community.Nodes {
			total += float64(kg.nodes[nodeID].Degree)
		}
		
		if totalEdges > 0 {
			modularity += (internal / totalEdges) - math.Pow(total/(2*totalEdges), 2)
		}
	}

	return &CommunityStructure{
		Communities:    communities,
		Modularity:     modularity,
		NumCommunities: len(communities),
	}
}

func (kg *KnowledgeGraph) findConnectedComponents() []ConnectedComponent {
	visited := make(map[uuid.UUID]bool)
	var components []ConnectedComponent
	componentID := 0

	for nodeID := range kg.nodes {
		if !visited[nodeID] {
			var componentNodes []uuid.UUID
			kg.dfsComponent(nodeID, visited, &componentNodes)
			
			if len(componentNodes) > 0 {
				components = append(components, ConnectedComponent{
					ID:    fmt.Sprintf("component_%d", componentID),
					Nodes: componentNodes,
					Size:  len(componentNodes),
				})
				componentID++
			}
		}
	}

	return components
}

func (kg *KnowledgeGraph) dfsComponent(nodeID uuid.UUID, visited map[uuid.UUID]bool, component *[]uuid.UUID) {
	visited[nodeID] = true
	*component = append(*component, nodeID)

	neighbors := kg.indices.AdjacencyList[nodeID]
	for _, neighborID := range neighbors {
		if !visited[neighborID] {
			kg.dfsComponent(neighborID, visited, component)
		}
	}
}

func (kg *KnowledgeGraph) cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0.0
	}

	var dotProduct, normA, normB float64
	for i := 0; i < len(a); i++ {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0.0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

func (kg *KnowledgeGraph) matchesFilters(node *Node, filters []QueryFilter) bool {
	for _, filter := range filters {
		if !kg.evaluateFilter(node, filter) {
			return false
		}
	}
	return true
}

func (kg *KnowledgeGraph) evaluateFilter(node *Node, filter QueryFilter) bool {
	var fieldValue interface{}

	switch filter.Field {
	case "type":
		fieldValue = string(node.Type)
	case "label":
		fieldValue = node.Label
	case "confidence":
		fieldValue = node.Confidence
	case "weight":
		fieldValue = node.Weight
	default:
		if val, exists := node.Properties[filter.Field]; exists {
			fieldValue = val
		} else {
			return false
		}
	}

	switch filter.Operator {
	case "eq":
		return fieldValue == filter.Value
	case "ne":
		return fieldValue != filter.Value
	case "gt":
		if fv, ok := fieldValue.(float64); ok {
			if tv, ok := filter.Value.(float64); ok {
				return fv > tv
			}
		}
	case "gte":
		if fv, ok := fieldValue.(float64); ok {
			if tv, ok := filter.Value.(float64); ok {
				return fv >= tv
			}
		}
	case "lt":
		if fv, ok := fieldValue.(float64); ok {
			if tv, ok := filter.Value.(float64); ok {
				return fv < tv
			}
		}
	case "lte":
		if fv, ok := fieldValue.(float64); ok {
			if tv, ok := filter.Value.(float64); ok {
				return fv <= tv
			}
		}
	case "contains":
		if fv, ok := fieldValue.(string); ok {
			if tv, ok := filter.Value.(string); ok {
				return strings.Contains(fv, tv)
			}
		}
	}

	return false
}

func (kg *KnowledgeGraph) registerDefaultProcessors() {
	// Register built-in entity processors
	kg.processors["ner"] = &NamedEntityRecognitionProcessor{}
	kg.processors["keyword"] = &KeywordExtractionProcessor{}
	
	// Register built-in relation extractors
	kg.extractors["pattern"] = &PatternBasedRelationExtractor{}
	kg.extractors["dependency"] = &DependencyBasedRelationExtractor{}
}

// DocumentProcessingResult represents the result of processing a document
type DocumentProcessingResult struct {
	DocumentID  uuid.UUID   `json:"document_id"`
	TenantID    uuid.UUID   `json:"tenant_id"`
	ProcessedAt time.Time   `json:"processed_at"`
	Entities    []*Entity   `json:"entities"`
	Relations   []*Relation `json:"relations"`
	NodesAdded  int         `json:"nodes_added"`
	EdgesAdded  int         `json:"edges_added"`
	ProcessingTime time.Duration `json:"processing_time"`
	Errors      []string    `json:"errors,omitempty"`
}

// Default processor implementations (simplified for demo)

type NamedEntityRecognitionProcessor struct{}

func (p *NamedEntityRecognitionProcessor) Name() string { return "NER" }
func (p *NamedEntityRecognitionProcessor) GetSupportedTypes() []NodeType { 
	return []NodeType{NodeTypePerson, NodeTypeOrganization, NodeTypeLocation} 
}

func (p *NamedEntityRecognitionProcessor) ProcessDocument(ctx context.Context, doc *Document) ([]*Entity, error) {
	// Simulate NER processing
	entities := []*Entity{
		{
			ID:         uuid.New(),
			Type:       NodeTypePerson,
			Label:      "John Doe",
			Text:       "John Doe",
			Confidence: 0.9,
			Properties: map[string]interface{}{"role": "author"},
		},
		{
			ID:         uuid.New(),
			Type:       NodeTypeOrganization,
			Label:      "Acme Corp",
			Text:       "Acme Corp",
			Confidence: 0.85,
			Properties: map[string]interface{}{"type": "company"},
		},
	}
	return entities, nil
}

func (p *NamedEntityRecognitionProcessor) ExtractEntities(ctx context.Context, text string) ([]*Entity, error) {
	return p.ProcessDocument(ctx, &Document{Content: text})
}

type KeywordExtractionProcessor struct{}

func (p *KeywordExtractionProcessor) Name() string { return "Keyword" }
func (p *KeywordExtractionProcessor) GetSupportedTypes() []NodeType { 
	return []NodeType{NodeTypeKeyword, NodeTypeTopic} 
}

func (p *KeywordExtractionProcessor) ProcessDocument(ctx context.Context, doc *Document) ([]*Entity, error) {
	// Simulate keyword extraction
	entities := []*Entity{
		{
			ID:         uuid.New(),
			Type:       NodeTypeKeyword,
			Label:      "machine learning",
			Text:       "machine learning",
			Confidence: 0.8,
			Properties: map[string]interface{}{"frequency": 5},
		},
	}
	return entities, nil
}

func (p *KeywordExtractionProcessor) ExtractEntities(ctx context.Context, text string) ([]*Entity, error) {
	return p.ProcessDocument(ctx, &Document{Content: text})
}

type PatternBasedRelationExtractor struct{}

func (p *PatternBasedRelationExtractor) Name() string { return "Pattern" }
func (p *PatternBasedRelationExtractor) GetSupportedRelations() []EdgeType { 
	return []EdgeType{EdgeTypeWorksFor, EdgeTypeLocatedIn} 
}

func (p *PatternBasedRelationExtractor) ExtractRelations(ctx context.Context, entities []*Entity, text string) ([]*Relation, error) {
	// Simulate relation extraction
	if len(entities) >= 2 {
		relations := []*Relation{
			{
				ID:         uuid.New(),
				Type:       EdgeTypeWorksFor,
				Label:      "works for",
				SourceID:   entities[0].ID,
				TargetID:   entities[1].ID,
				Confidence: 0.7,
				Properties: map[string]interface{}{"since": "2020"},
			},
		}
		return relations, nil
	}
	return []*Relation{}, nil
}

type DependencyBasedRelationExtractor struct{}

func (p *DependencyBasedRelationExtractor) Name() string { return "Dependency" }
func (p *DependencyBasedRelationExtractor) GetSupportedRelations() []EdgeType { 
	return []EdgeType{EdgeTypeRelatedTo, EdgeTypeMentions} 
}

func (p *DependencyBasedRelationExtractor) ExtractRelations(ctx context.Context, entities []*Entity, text string) ([]*Relation, error) {
	// Simulate dependency-based relation extraction
	return []*Relation{}, nil
}