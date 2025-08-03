package models

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// MLAnalysisResult stores the results of ML-based content analysis
type MLAnalysisResult struct {
	ID             uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	DocumentID     uuid.UUID  `gorm:"type:uuid;not null;index" json:"document_id"`
	ChunkID        *uuid.UUID `gorm:"type:uuid;index" json:"chunk_id,omitempty"`
	AnalysisType   string     `gorm:"type:varchar(50);not null" json:"analysis_type"`
	ResultData     string     `gorm:"type:jsonb;not null" json:"result_data"`
	Confidence     float64    `gorm:"type:decimal(5,4);default:0.0" json:"confidence"`
	ProcessingTime int64      `gorm:"type:bigint;default:0" json:"processing_time_ms"`
	ModelVersions  string     `gorm:"type:jsonb" json:"model_versions"`
	Status         string     `gorm:"type:varchar(20);not null;default:'pending'" json:"status"`
	Error          string     `gorm:"type:text" json:"error,omitempty"`
	CreatedAt      time.Time  `gorm:"type:timestamp;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"type:timestamp;default:CURRENT_TIMESTAMP" json:"updated_at"`

	// Relationships
	File  *File  `gorm:"foreignKey:DocumentID;references:ID" json:"file,omitempty"`
	Chunk *Chunk `gorm:"foreignKey:ChunkID;references:ID" json:"chunk,omitempty"`
}

// TableName returns the table name for GORM
func (MLAnalysisResult) TableName() string {
	return "ml_analysis_results"
}

// BeforeCreate sets the ID before creating
func (m *MLAnalysisResult) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	return nil
}

// MLAnalysisJob represents a queued ML analysis job
type MLAnalysisJob struct {
	ID          uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	DocumentID  uuid.UUID  `gorm:"type:uuid;not null;index" json:"document_id"`
	ChunkID     *uuid.UUID `gorm:"type:uuid;index" json:"chunk_id,omitempty"`
	JobType     string     `gorm:"type:varchar(50);not null" json:"job_type"`
	Priority    int        `gorm:"type:int;default:1" json:"priority"`
	Status      string     `gorm:"type:varchar(20);not null;default:'pending'" json:"status"`
	Parameters  string     `gorm:"type:jsonb" json:"parameters"`
	RequestedBy string     `gorm:"type:varchar(100)" json:"requested_by"`
	Error       string     `gorm:"type:text" json:"error,omitempty"`
	RetryCount  int        `gorm:"type:int;default:0" json:"retry_count"`
	MaxRetries  int        `gorm:"type:int;default:3" json:"max_retries"`
	ScheduledAt *time.Time `gorm:"type:timestamp" json:"scheduled_at,omitempty"`
	StartedAt   *time.Time `gorm:"type:timestamp" json:"started_at,omitempty"`
	CompletedAt *time.Time `gorm:"type:timestamp" json:"completed_at,omitempty"`
	CreatedAt   time.Time  `gorm:"type:timestamp;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt   time.Time  `gorm:"type:timestamp;default:CURRENT_TIMESTAMP" json:"updated_at"`

	// Relationships
	File   *File             `gorm:"foreignKey:DocumentID;references:ID" json:"file,omitempty"`
	Chunk  *Chunk            `gorm:"foreignKey:ChunkID;references:ID" json:"chunk,omitempty"`
	Result *MLAnalysisResult `gorm:"foreignKey:DocumentID,ChunkID;references:DocumentID,ChunkID" json:"result,omitempty"`
}

// TableName returns the table name for GORM
func (MLAnalysisJob) TableName() string {
	return "ml_analysis_jobs"
}

// BeforeCreate sets the ID before creating
func (m *MLAnalysisJob) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	return nil
}

// IsRetryable checks if the job can be retried
func (m *MLAnalysisJob) IsRetryable() bool {
	return m.RetryCount < m.MaxRetries && m.Status == "failed"
}

// CanStart checks if the job can be started
func (m *MLAnalysisJob) CanStart() bool {
	return m.Status == "pending" && (m.ScheduledAt == nil || m.ScheduledAt.Before(time.Now()))
}

// MLAnalysisBatch represents a batch of ML analysis jobs
type MLAnalysisBatch struct {
	ID             uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	Name           string     `gorm:"type:varchar(255)" json:"name"`
	Description    string     `gorm:"type:text" json:"description"`
	Status         string     `gorm:"type:varchar(20);not null;default:'pending'" json:"status"`
	TotalJobs      int        `gorm:"type:int;default:0" json:"total_jobs"`
	CompletedJobs  int        `gorm:"type:int;default:0" json:"completed_jobs"`
	FailedJobs     int        `gorm:"type:int;default:0" json:"failed_jobs"`
	Parameters     string     `gorm:"type:jsonb" json:"parameters"`
	RequestedBy    string     `gorm:"type:varchar(100)" json:"requested_by"`
	ProcessingTime int64      `gorm:"type:bigint;default:0" json:"processing_time_ms"`
	StartedAt      *time.Time `gorm:"type:timestamp" json:"started_at,omitempty"`
	CompletedAt    *time.Time `gorm:"type:timestamp" json:"completed_at,omitempty"`
	CreatedAt      time.Time  `gorm:"type:timestamp;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"type:timestamp;default:CURRENT_TIMESTAMP" json:"updated_at"`

	// Relationships
	Jobs []MLAnalysisJob `gorm:"foreignKey:BatchID" json:"jobs,omitempty"`
}

// TableName returns the table name for GORM
func (MLAnalysisBatch) TableName() string {
	return "ml_analysis_batches"
}

// BeforeCreate sets the ID before creating
func (m *MLAnalysisBatch) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	return nil
}

// IsComplete checks if the batch is complete
func (m *MLAnalysisBatch) IsComplete() bool {
	return m.CompletedJobs+m.FailedJobs >= m.TotalJobs
}

// UpdateProgress updates the batch progress
func (m *MLAnalysisBatch) UpdateProgress(tx *gorm.DB) error {
	var completed, failed int64

	if err := tx.Model(&MLAnalysisJob{}).Where("batch_id = ? AND status = ?", m.ID, "completed").Count(&completed).Error; err != nil {
		return err
	}

	if err := tx.Model(&MLAnalysisJob{}).Where("batch_id = ? AND status = ?", m.ID, "failed").Count(&failed).Error; err != nil {
		return err
	}

	m.CompletedJobs = int(completed)
	m.FailedJobs = int(failed)

	if m.IsComplete() {
		m.Status = "completed"
		now := time.Now()
		m.CompletedAt = &now
	}

	return tx.Save(m).Error
}

// MLAnalysisMetrics represents aggregated analysis metrics
type MLAnalysisMetrics struct {
	ID                   uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	DocumentID           *uuid.UUID `gorm:"type:uuid;index" json:"document_id,omitempty"`
	TimeFrame            string     `gorm:"type:varchar(20);not null" json:"time_frame"` // daily, weekly, monthly
	Date                 time.Time  `gorm:"type:date;not null;index" json:"date"`
	TotalAnalyses        int        `gorm:"type:int;default:0" json:"total_analyses"`
	AvgConfidence        float64    `gorm:"type:decimal(5,4);default:0.0" json:"avg_confidence"`
	AvgProcessingTime    int64      `gorm:"type:bigint;default:0" json:"avg_processing_time_ms"`
	SentimentPositive    int        `gorm:"type:int;default:0" json:"sentiment_positive"`
	SentimentNegative    int        `gorm:"type:int;default:0" json:"sentiment_negative"`
	SentimentNeutral     int        `gorm:"type:int;default:0" json:"sentiment_neutral"`
	TopicDistribution    string     `gorm:"type:jsonb" json:"topic_distribution"`
	EntityCounts         string     `gorm:"type:jsonb" json:"entity_counts"`
	QualityScores        string     `gorm:"type:jsonb" json:"quality_scores"`
	LanguageDistribution string     `gorm:"type:jsonb" json:"language_distribution"`
	CreatedAt            time.Time  `gorm:"type:timestamp;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt            time.Time  `gorm:"type:timestamp;default:CURRENT_TIMESTAMP" json:"updated_at"`

	// Relationships
	Document *File `gorm:"foreignKey:DocumentID;references:ID" json:"document,omitempty"`
}

// TableName returns the table name for GORM
func (MLAnalysisMetrics) TableName() string {
	return "ml_analysis_metrics"
}

// BeforeCreate sets the ID before creating
func (m *MLAnalysisMetrics) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	return nil
}

// MLAnalysisConfig represents ML analysis configuration per tenant
type MLAnalysisConfig struct {
	ID                uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	Name              string    `gorm:"type:varchar(255);not null" json:"name"`
	Description       string    `gorm:"type:text" json:"description"`
	IsDefault         bool      `gorm:"type:boolean;default:false" json:"is_default"`
	EnableSentiment   bool      `gorm:"type:boolean;default:true" json:"enable_sentiment"`
	EnableTopics      bool      `gorm:"type:boolean;default:true" json:"enable_topics"`
	EnableEntities    bool      `gorm:"type:boolean;default:true" json:"enable_entities"`
	EnableSummary     bool      `gorm:"type:boolean;default:true" json:"enable_summary"`
	EnableQuality     bool      `gorm:"type:boolean;default:true" json:"enable_quality"`
	EnableLanguage    bool      `gorm:"type:boolean;default:true" json:"enable_language"`
	EnableEmotion     bool      `gorm:"type:boolean;default:true" json:"enable_emotion"`
	MaxTopics         int       `gorm:"type:int;default:5" json:"max_topics"`
	SummaryRatio      float64   `gorm:"type:decimal(3,2);default:0.3" json:"summary_ratio"`
	MinConfidence     float64   `gorm:"type:decimal(3,2);default:0.5" json:"min_confidence"`
	BatchSize         int       `gorm:"type:int;default:10" json:"batch_size"`
	ProcessingTimeout int       `gorm:"type:int;default:300" json:"processing_timeout_seconds"`
	RetryAttempts     int       `gorm:"type:int;default:3" json:"retry_attempts"`
	CacheEnabled      bool      `gorm:"type:boolean;default:true" json:"cache_enabled"`
	CacheTTL          int       `gorm:"type:int;default:3600" json:"cache_ttl_seconds"`
	AutoAnalyze       bool      `gorm:"type:boolean;default:false" json:"auto_analyze"`
	AnalyzeTriggers   string    `gorm:"type:jsonb" json:"analyze_triggers"`
	ModelPreferences  string    `gorm:"type:jsonb" json:"model_preferences"`
	CustomParameters  string    `gorm:"type:jsonb" json:"custom_parameters"`
	CreatedAt         time.Time `gorm:"type:timestamp;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt         time.Time `gorm:"type:timestamp;default:CURRENT_TIMESTAMP" json:"updated_at"`
	CreatedBy         string    `gorm:"type:varchar(100)" json:"created_by"`
	UpdatedBy         string    `gorm:"type:varchar(100)" json:"updated_by"`
}

// TableName returns the table name for GORM
func (MLAnalysisConfig) TableName() string {
	return "ml_analysis_configs"
}

// BeforeCreate sets the ID before creating
func (m *MLAnalysisConfig) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	return nil
}

// ValidateConfig validates the configuration parameters
func (m *MLAnalysisConfig) ValidateConfig() error {
	if m.MaxTopics < 1 || m.MaxTopics > 20 {
		return fmt.Errorf("max_topics must be between 1 and 20")
	}
	if m.SummaryRatio < 0.1 || m.SummaryRatio > 1.0 {
		return fmt.Errorf("summary_ratio must be between 0.1 and 1.0")
	}
	if m.MinConfidence < 0.0 || m.MinConfidence > 1.0 {
		return fmt.Errorf("min_confidence must be between 0.0 and 1.0")
	}
	if m.BatchSize < 1 || m.BatchSize > 100 {
		return fmt.Errorf("batch_size must be between 1 and 100")
	}
	if m.ProcessingTimeout < 10 || m.ProcessingTimeout > 3600 {
		return fmt.Errorf("processing_timeout_seconds must be between 10 and 3600")
	}
	return nil
}

// GetEnabledAnalysis returns a list of enabled analysis types
func (m *MLAnalysisConfig) GetEnabledAnalysis() []string {
	var enabled []string
	if m.EnableSentiment {
		enabled = append(enabled, "sentiment")
	}
	if m.EnableTopics {
		enabled = append(enabled, "topics")
	}
	if m.EnableEntities {
		enabled = append(enabled, "entities")
	}
	if m.EnableSummary {
		enabled = append(enabled, "summary")
	}
	if m.EnableQuality {
		enabled = append(enabled, "quality")
	}
	if m.EnableLanguage {
		enabled = append(enabled, "language")
	}
	if m.EnableEmotion {
		enabled = append(enabled, "emotion")
	}
	return enabled
}

// MLAnalysisAlert represents alerts for ML analysis results
type MLAnalysisAlert struct {
	ID             uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	DocumentID     uuid.UUID  `gorm:"type:uuid;not null;index" json:"document_id"`
	ChunkID        *uuid.UUID `gorm:"type:uuid;index" json:"chunk_id,omitempty"`
	AlertType      string     `gorm:"type:varchar(50);not null" json:"alert_type"`
	Severity       string     `gorm:"type:varchar(20);not null" json:"severity"` // low, medium, high, critical
	Title          string     `gorm:"type:varchar(255);not null" json:"title"`
	Description    string     `gorm:"type:text" json:"description"`
	Condition      string     `gorm:"type:jsonb" json:"condition"`
	Triggered      bool       `gorm:"type:boolean;default:true" json:"triggered"`
	Acknowledged   bool       `gorm:"type:boolean;default:false" json:"acknowledged"`
	AcknowledgedBy string     `gorm:"type:varchar(100)" json:"acknowledged_by"`
	AcknowledgedAt *time.Time `gorm:"type:timestamp" json:"acknowledged_at,omitempty"`
	ResolvedAt     *time.Time `gorm:"type:timestamp" json:"resolved_at,omitempty"`
	CreatedAt      time.Time  `gorm:"type:timestamp;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"type:timestamp;default:CURRENT_TIMESTAMP" json:"updated_at"`

	// Relationships
	Document *File  `gorm:"foreignKey:DocumentID;references:ID" json:"document,omitempty"`
	Chunk    *Chunk `gorm:"foreignKey:ChunkID;references:ID" json:"chunk,omitempty"`
}

// TableName returns the table name for GORM
func (MLAnalysisAlert) TableName() string {
	return "ml_analysis_alerts"
}

// BeforeCreate sets the ID before creating
func (m *MLAnalysisAlert) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	return nil
}

// Acknowledge marks the alert as acknowledged
func (m *MLAnalysisAlert) Acknowledge(acknowledgedBy string) {
	m.Acknowledged = true
	m.AcknowledgedBy = acknowledgedBy
	now := time.Now()
	m.AcknowledgedAt = &now
}

// Resolve marks the alert as resolved
func (m *MLAnalysisAlert) Resolve() {
	now := time.Now()
	m.ResolvedAt = &now
}

// IsActive checks if the alert is still active
func (m *MLAnalysisAlert) IsActive() bool {
	return m.Triggered && !m.Acknowledged && m.ResolvedAt == nil
}
