package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Chunk represents a chunk of processed data from a file
type Chunk struct {
	ID       uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	FileID   uuid.UUID `gorm:"type:uuid;not null;index" json:"file_id"`
	
	// Chunk identification
	ChunkID     string `gorm:"not null;index" json:"chunk_id"`      // Unique identifier within file
	ChunkType   string `gorm:"not null;index" json:"chunk_type"`    // text, table, image, etc.
	ChunkNumber int    `gorm:"not null" json:"chunk_number"`        // Sequential number within file
	
	// Content
	Content     string `gorm:"type:text;not null" json:"content"`
	ContentHash string `gorm:"index" json:"content_hash"`           // Hash of content for deduplication
	SizeBytes   int64  `gorm:"not null" json:"size_bytes"`
	
	// Position information
	StartPosition *int64 `json:"start_position,omitempty"`
	EndPosition   *int64 `json:"end_position,omitempty"`
	PageNumber    *int   `json:"page_number,omitempty"`
	LineNumber    *int   `json:"line_number,omitempty"`
	
	// Relationships to other chunks
	ParentChunkID *uuid.UUID `gorm:"type:uuid;index" json:"parent_chunk_id,omitempty"`
	Relationships []string   `gorm:"type:jsonb" json:"relationships,omitempty"`
	
	// Processing metadata
	ProcessedAt     time.Time `gorm:"not null" json:"processed_at"`
	ProcessedBy     string    `gorm:"not null" json:"processed_by"`     // Strategy name
	ProcessingTime  int64     `json:"processing_time"`                  // Time in milliseconds
	
	// Quality metrics
	Quality ChunkQualityMetrics `gorm:"type:jsonb" json:"quality"`
	
	// Content analysis
	Language         string   `json:"language,omitempty"`
	LanguageConf     float64  `json:"language_confidence,omitempty"`
	ContentCategory  string   `json:"content_category,omitempty"`
	SensitivityLevel string   `json:"sensitivity_level,omitempty"`
	Classifications  []string `gorm:"type:jsonb" json:"classifications,omitempty"`
	
	// Embedding information
	EmbeddingStatus string     `gorm:"default:'pending'" json:"embedding_status"`
	EmbeddingModel  string     `json:"embedding_model,omitempty"`
	EmbeddingVector []float64  `gorm:"type:jsonb" json:"embedding_vector,omitempty"`
	EmbeddingDim    int        `json:"embedding_dimension,omitempty"`
	EmbeddedAt      *time.Time `json:"embedded_at,omitempty"`
	
	// Compliance and security
	PIIDetected     bool     `gorm:"default:false;index" json:"pii_detected"`
	ComplianceFlags []string `gorm:"type:jsonb" json:"compliance_flags,omitempty"`
	DLPScanStatus   string   `gorm:"default:'pending'" json:"dlp_scan_status"`
	DLPScanResult   string   `json:"dlp_scan_result,omitempty"`
	
	// Context information
	Context map[string]string `gorm:"type:jsonb" json:"context,omitempty"`
	
	// Schema information for structured chunks
	SchemaInfo map[string]interface{} `gorm:"type:jsonb" json:"schema_info,omitempty"`
	
	// Metadata
	Metadata     map[string]interface{} `gorm:"type:jsonb" json:"metadata,omitempty"`
	CustomFields map[string]interface{} `gorm:"type:jsonb" json:"custom_fields,omitempty"`
	
	// Timestamps
	CreatedAt time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt time.Time  `gorm:"not null" json:"updated_at"`
	DeletedAt *time.Time `gorm:"index" json:"deleted_at,omitempty"`
	
	// Relationships
	Tenant        *Tenant        `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	File          *File          `gorm:"foreignKey:FileID" json:"file,omitempty"`
	ParentChunk   *Chunk         `gorm:"foreignKey:ParentChunkID" json:"parent_chunk,omitempty"`
	ChildChunks   []Chunk        `gorm:"foreignKey:ParentChunkID" json:"child_chunks,omitempty"`
	DLPViolations []DLPViolation `gorm:"foreignKey:ChunkID" json:"dlp_violations,omitempty"`
}

// ChunkQualityMetrics represents quality metrics for a chunk
type ChunkQualityMetrics struct {
	Completeness float64 `json:"completeness"`  // How complete the content appears (0.0-1.0)
	Coherence    float64 `json:"coherence"`     // How semantically coherent the content is
	Uniqueness   float64 `json:"uniqueness"`    // How unique this chunk is vs others
	Readability  float64 `json:"readability"`   // Text readability score
	LanguageConf float64 `json:"language_conf"` // Confidence in detected language
	Language     string  `json:"language"`      // Detected language code
	Complexity   float64 `json:"complexity"`    // Content complexity score
	Density      float64 `json:"density"`       // Information density score
}

// GORM hooks for JSON serialization
func (c *ChunkQualityMetrics) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	return json.Unmarshal(value.([]byte), c)
}

func (c ChunkQualityMetrics) Value() (interface{}, error) {
	return json.Marshal(c)
}

// Custom slice types for GORM JSON handling
type ChunkClassifications []string
type ChunkComplianceFlags []string
type ChunkRelationships []string
type EmbeddingVector []float64

func (c *ChunkClassifications) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	return json.Unmarshal(value.([]byte), c)
}

func (c ChunkClassifications) Value() (interface{}, error) {
	return json.Marshal(c)
}

func (c *ChunkComplianceFlags) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	return json.Unmarshal(value.([]byte), c)
}

func (c ChunkComplianceFlags) Value() (interface{}, error) {
	return json.Marshal(c)
}

func (c *ChunkRelationships) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	return json.Unmarshal(value.([]byte), c)
}

func (c ChunkRelationships) Value() (interface{}, error) {
	return json.Marshal(c)
}

func (e *EmbeddingVector) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	return json.Unmarshal(value.([]byte), e)
}

func (e EmbeddingVector) Value() (interface{}, error) {
	return json.Marshal(e)
}

// TableName returns the table name for Chunk model
func (Chunk) TableName() string {
	return "chunks"
}

// IsEmbedded checks if the chunk has been embedded
func (c *Chunk) IsEmbedded() bool {
	return c.EmbeddingStatus == "completed" && len(c.EmbeddingVector) > 0
}

// HasPII checks if the chunk contains personally identifiable information
func (c *Chunk) HasPII() bool {
	return c.PIIDetected
}

// GetSensitivityLevel returns the sensitivity level of the chunk
func (c *Chunk) GetSensitivityLevel() string {
	if c.SensitivityLevel == "" {
		return "unknown"
	}
	return c.SensitivityLevel
}

// AddClassification adds a classification to the chunk
func (c *Chunk) AddClassification(classification string) {
	for _, cl := range c.Classifications {
		if cl == classification {
			return // Already exists
		}
	}
	c.Classifications = append(c.Classifications, classification)
}

// AddComplianceFlag adds a compliance flag to the chunk
func (c *Chunk) AddComplianceFlag(flag string) {
	for _, cf := range c.ComplianceFlags {
		if cf == flag {
			return // Already exists
		}
	}
	c.ComplianceFlags = append(c.ComplianceFlags, flag)
}

// SetEmbedding sets the embedding vector for the chunk
func (c *Chunk) SetEmbedding(vector []float64, model string) {
	c.EmbeddingVector = vector
	c.EmbeddingModel = model
	c.EmbeddingDim = len(vector)
	c.EmbeddingStatus = "completed"
	now := time.Now()
	c.EmbeddedAt = &now
}

// MarkEmbeddingFailed marks the embedding as failed
func (c *Chunk) MarkEmbeddingFailed() {
	c.EmbeddingStatus = "failed"
}

// SetDLPScanResult sets the DLP scan result for the chunk
func (c *Chunk) SetDLPScanResult(result string, hasPII bool) {
	c.DLPScanStatus = "completed"
	c.DLPScanResult = result
	c.PIIDetected = hasPII
}

// MarkDLPScanFailed marks the DLP scan as failed
func (c *Chunk) MarkDLPScanFailed() {
	c.DLPScanStatus = "failed"
}

// GetQualityScore returns an overall quality score for the chunk
func (c *Chunk) GetQualityScore() float64 {
	metrics := c.Quality
	
	// Weighted average of quality metrics
	weights := map[string]float64{
		"completeness": 0.25,
		"coherence":    0.25,
		"uniqueness":   0.20,
		"readability":  0.15,
		"language_conf": 0.10,
		"complexity":   0.05,
	}
	
	score := 0.0
	score += metrics.Completeness * weights["completeness"]
	score += metrics.Coherence * weights["coherence"]
	score += metrics.Uniqueness * weights["uniqueness"]
	score += metrics.Readability * weights["readability"]
	score += metrics.LanguageConf * weights["language_conf"]
	score += metrics.Complexity * weights["complexity"]
	
	return score
}

// IsHighQuality checks if the chunk meets high quality thresholds
func (c *Chunk) IsHighQuality() bool {
	return c.GetQualityScore() >= 0.8
}

// IsLowQuality checks if the chunk is below quality thresholds
func (c *Chunk) IsLowQuality() bool {
	return c.GetQualityScore() < 0.5
}

// GetContentPreview returns a preview of the content
func (c *Chunk) GetContentPreview(maxLength int) string {
	if len(c.Content) <= maxLength {
		return c.Content
	}
	return c.Content[:maxLength] + "..."
}

// AddRelationship adds a relationship to another chunk
func (c *Chunk) AddRelationship(relationshipType string, chunkID uuid.UUID) {
	relationship := relationshipType + ":" + chunkID.String()
	
	for _, r := range c.Relationships {
		if r == relationship {
			return // Already exists
		}
	}
	
	c.Relationships = append(c.Relationships, relationship)
}

// GetRelationships returns relationships of a specific type
func (c *Chunk) GetRelationships(relationshipType string) []uuid.UUID {
	var relationships []uuid.UUID
	prefix := relationshipType + ":"
	
	for _, r := range c.Relationships {
		if len(r) > len(prefix) && r[:len(prefix)] == prefix {
			if id, err := uuid.Parse(r[len(prefix):]); err == nil {
				relationships = append(relationships, id)
			}
		}
	}
	
	return relationships
}

// SetProcessingMetrics sets processing-related metrics
func (c *Chunk) SetProcessingMetrics(strategy string, processingTime int64) {
	c.ProcessedBy = strategy
	c.ProcessingTime = processingTime
	c.ProcessedAt = time.Now()
}