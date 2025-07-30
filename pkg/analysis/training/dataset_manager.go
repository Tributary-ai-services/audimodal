package training

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
)

// DatasetManager manages training datasets
type DatasetManager struct {
	datasets      map[uuid.UUID]*Dataset
	dataLoaders   map[uuid.UUID]*DataLoader
	preprocessors map[string]Preprocessor
	mutex         sync.RWMutex
}

// Dataset represents a training dataset
type Dataset struct {
	ID              uuid.UUID                 `json:"id"`
	Name            string                    `json:"name"`
	Description     string                    `json:"description"`
	TenantID        uuid.UUID                 `json:"tenant_id"`
	DatasetType     DatasetType               `json:"dataset_type"`
	Format          DatasetFormat             `json:"format"`
	Status          DatasetStatus             `json:"status"`
	CreatedAt       time.Time                 `json:"created_at"`
	UpdatedAt       time.Time                 `json:"updated_at"`
	CreatedBy       uuid.UUID                 `json:"created_by"`
	
	// Dataset content
	SourcePath      string                    `json:"source_path"`
	CachedPath      string                    `json:"cached_path,omitempty"`
	Size            int64                     `json:"size_bytes"`
	RecordCount     int                       `json:"record_count"`
	FeatureCount    int                       `json:"feature_count"`
	
	// Data schema
	Schema          *DatasetSchema            `json:"schema"`
	Statistics      *DatasetStatistics        `json:"statistics,omitempty"`
	
	// Splits
	TrainingSplit   *DatasetSplit             `json:"training_split,omitempty"`
	ValidationSplit *DatasetSplit             `json:"validation_split,omitempty"`
	TestSplit       *DatasetSplit             `json:"test_split,omitempty"`
	
	// Metadata
	Tags            []string                  `json:"tags"`
	Labels          map[string]string         `json:"labels"`
	Metadata        map[string]interface{}    `json:"metadata"`
	
	// Quality metrics
	QualityScore    float64                   `json:"quality_score"`
	QualityMetrics  *DataQualityMetrics       `json:"quality_metrics,omitempty"`
	
	// Versioning
	Version         string                    `json:"version"`
	ParentDatasetID *uuid.UUID                `json:"parent_dataset_id,omitempty"`
	ChangeLog       []DatasetChange           `json:"change_log,omitempty"`
}

// DatasetType represents the type of dataset
type DatasetType string

const (
	DatasetTypeText         DatasetType = "text"
	DatasetTypeImage        DatasetType = "image"
	DatasetTypeAudio        DatasetType = "audio"
	DatasetTypeVideo        DatasetType = "video"
	DatasetTypeTabular      DatasetType = "tabular"
	DatasetTypeTimeSeries   DatasetType = "time_series"
	DatasetTypeGraph        DatasetType = "graph"
	DatasetTypeMultimodal   DatasetType = "multimodal"
)

// DatasetFormat represents the format of dataset files
type DatasetFormat string

const (
	DatasetFormatCSV     DatasetFormat = "csv"
	DatasetFormatJSON    DatasetFormat = "json"
	DatasetFormatJSONL   DatasetFormat = "jsonl"
	DatasetFormatParquet DatasetFormat = "parquet"
	DatasetFormatAvro    DatasetFormat = "avro"
	DatasetFormatTFRecord DatasetFormat = "tfrecord"
	DatasetFormatHDF5    DatasetFormat = "hdf5"
	DatasetFormatNumPy   DatasetFormat = "numpy"
	DatasetFormatPickle  DatasetFormat = "pickle"
)

// DatasetStatus represents the status of a dataset
type DatasetStatus string

const (
	DatasetStatusCreating   DatasetStatus = "creating"
	DatasetStatusProcessing DatasetStatus = "processing"
	DatasetStatusReady      DatasetStatus = "ready"
	DatasetStatusError      DatasetStatus = "error"
	DatasetStatusArchived   DatasetStatus = "archived"
)

// DatasetSchema defines the structure of dataset records
type DatasetSchema struct {
	Fields         []FieldSchema             `json:"fields"`
	TargetField    string                    `json:"target_field,omitempty"`
	FeatureFields  []string                  `json:"feature_fields"`
	IndexField     string                    `json:"index_field,omitempty"`
	TimestampField string                    `json:"timestamp_field,omitempty"`
	Constraints    []SchemaConstraint        `json:"constraints,omitempty"`
}

// FieldSchema defines the schema for a single field
type FieldSchema struct {
	Name         string                    `json:"name"`
	Type         FieldType                 `json:"type"`
	Description  string                    `json:"description,omitempty"`
	Required     bool                      `json:"required"`
	Nullable     bool                      `json:"nullable"`
	DefaultValue interface{}               `json:"default_value,omitempty"`
	Constraints  []FieldConstraint         `json:"constraints,omitempty"`
	Metadata     map[string]interface{}    `json:"metadata,omitempty"`
}

// FieldType represents the data type of a field
type FieldType string

const (
	FieldTypeString     FieldType = "string"
	FieldTypeInteger    FieldType = "integer"
	FieldTypeFloat      FieldType = "float"
	FieldTypeBoolean    FieldType = "boolean"
	FieldTypeDateTime   FieldType = "datetime"
	FieldTypeCategory   FieldType = "category"
	FieldTypeText       FieldType = "text"
	FieldTypeImage      FieldType = "image"
	FieldTypeAudio      FieldType = "audio"
	FieldTypeVideo      FieldType = "video"
	FieldTypeEmbedding  FieldType = "embedding"
	FieldTypeJSON       FieldType = "json"
	FieldTypeArray      FieldType = "array"
)

// SchemaConstraint represents a schema-level constraint
type SchemaConstraint struct {
	Type        string                    `json:"type"`
	Description string                    `json:"description"`
	Parameters  map[string]interface{}    `json:"parameters"`
}

// FieldConstraint represents a field-level constraint
type FieldConstraint struct {
	Type        string                    `json:"type"`
	Value       interface{}               `json:"value"`
	Message     string                    `json:"message,omitempty"`
}

// DatasetStatistics contains statistical information about the dataset
type DatasetStatistics struct {
	RecordCount      int                       `json:"record_count"`
	FieldCount       int                       `json:"field_count"`
	MissingValues    map[string]int            `json:"missing_values"`
	UniqueValues     map[string]int            `json:"unique_values"`
	NumericalStats   map[string]NumericalStats `json:"numerical_stats"`
	CategoricalStats map[string]CategoricalStats `json:"categorical_stats"`
	TextStats        map[string]TextStats      `json:"text_stats"`
	CorrelationMatrix map[string]map[string]float64 `json:"correlation_matrix,omitempty"`
	ComputedAt       time.Time                 `json:"computed_at"`
}

// NumericalStats contains statistics for numerical fields
type NumericalStats struct {
	Count      int     `json:"count"`
	Mean       float64 `json:"mean"`
	Median     float64 `json:"median"`
	Mode       float64 `json:"mode"`
	StdDev     float64 `json:"std_dev"`
	Variance   float64 `json:"variance"`
	Min        float64 `json:"min"`
	Max        float64 `json:"max"`
	Q1         float64 `json:"q1"`
	Q3         float64 `json:"q3"`
	IQR        float64 `json:"iqr"`
	Skewness   float64 `json:"skewness"`
	Kurtosis   float64 `json:"kurtosis"`
	Outliers   int     `json:"outliers"`
}

// CategoricalStats contains statistics for categorical fields
type CategoricalStats struct {
	Count          int                `json:"count"`
	UniqueValues   int                `json:"unique_values"`
	MostFrequent   string             `json:"most_frequent"`
	LeastFrequent  string             `json:"least_frequent"`
	Distribution   map[string]int     `json:"distribution"`
	Entropy        float64            `json:"entropy"`
	ConcentrationRatio float64        `json:"concentration_ratio"`
}

// TextStats contains statistics for text fields
type TextStats struct {
	Count            int                `json:"count"`
	AverageLength    float64            `json:"average_length"`
	MinLength        int                `json:"min_length"`
	MaxLength        int                `json:"max_length"`
	VocabularySize   int                `json:"vocabulary_size"`
	MostCommonWords  map[string]int     `json:"most_common_words"`
	LanguageDetection map[string]float64 `json:"language_detection"`
	ReadabilityScore float64            `json:"readability_score"`
}

// DatasetSplit represents a data split (train/validation/test)
type DatasetSplit struct {
	Name        string    `json:"name"`
	RecordCount int       `json:"record_count"`
	Percentage  float64   `json:"percentage"`
	Path        string    `json:"path"`
	Checksum    string    `json:"checksum"`
	CreatedAt   time.Time `json:"created_at"`
}

// DataQualityMetrics contains data quality assessment metrics
type DataQualityMetrics struct {
	CompletenessScore float64                   `json:"completeness_score"`
	AccuracyScore     float64                   `json:"accuracy_score"`
	ConsistencyScore  float64                   `json:"consistency_score"`
	ValidityScore     float64                   `json:"validity_score"`
	UniquenessScore   float64                   `json:"uniqueness_score"`
	TimelinessScore   float64                   `json:"timeliness_score"`
	Issues            []DataQualityIssue        `json:"issues"`
	Recommendations   []DataQualityRecommendation `json:"recommendations"`
	AssessedAt        time.Time                 `json:"assessed_at"`
}

// DataQualityIssue represents a data quality issue
type DataQualityIssue struct {
	Type        string                    `json:"type"`
	Severity    string                    `json:"severity"`
	Field       string                    `json:"field,omitempty"`
	Description string                    `json:"description"`
	Count       int                       `json:"count"`
	Examples    []interface{}             `json:"examples,omitempty"`
	Impact      string                    `json:"impact"`
}

// DataQualityRecommendation represents a recommendation for improving data quality
type DataQualityRecommendation struct {
	Type         string    `json:"type"`
	Priority     string    `json:"priority"`
	Description  string    `json:"description"`
	Action       string    `json:"action"`
	EstimatedImpact float64 `json:"estimated_impact"`
}

// DatasetChange represents a change in dataset version
type DatasetChange struct {
	Version     string                    `json:"version"`
	ChangeType  string                    `json:"change_type"`
	Description string                    `json:"description"`
	ChangedBy   uuid.UUID                 `json:"changed_by"`
	ChangedAt   time.Time                 `json:"changed_at"`
	Details     map[string]interface{}    `json:"details"`
}

// DataLoader handles loading and preprocessing of dataset data
type DataLoader struct {
	ID           uuid.UUID                 `json:"id"`
	DatasetID    uuid.UUID                 `json:"dataset_id"`
	Config       *DataLoaderConfig         `json:"config"`
	Preprocessors []PreprocessingStep      `json:"preprocessors"`
	Cache        *DataCache                `json:"cache,omitempty"`
	Statistics   *LoaderStatistics         `json:"statistics,omitempty"`
}

// DataLoaderConfig contains configuration for data loading
type DataLoaderConfig struct {
	BatchSize       int                       `json:"batch_size"`
	Shuffle         bool                      `json:"shuffle"`
	RandomSeed      int                       `json:"random_seed"`
	NumWorkers      int                       `json:"num_workers"`
	PrefetchFactor  int                       `json:"prefetch_factor"`
	DropLast        bool                      `json:"drop_last"`
	PinMemory       bool                      `json:"pin_memory"`
	Timeout         time.Duration             `json:"timeout"`
	Collation       string                    `json:"collation"`
	SamplingStrategy string                   `json:"sampling_strategy"`
	WeightedSampling bool                    `json:"weighted_sampling"`
	ClassWeights    map[string]float64        `json:"class_weights,omitempty"`
	Transforms      []TransformConfig         `json:"transforms,omitempty"`
}

// TransformConfig represents a data transformation configuration
type TransformConfig struct {
	Name       string                    `json:"name"`
	Parameters map[string]interface{}    `json:"parameters"`
	Enabled    bool                      `json:"enabled"`
	Order      int                       `json:"order"`
}

// DataCache represents cached dataset data
type DataCache struct {
	Enabled      bool                      `json:"enabled"`
	CachePath    string                    `json:"cache_path"`
	CacheSize    int64                     `json:"cache_size_bytes"`
	HitRate      float64                   `json:"hit_rate"`
	LastAccessed time.Time                 `json:"last_accessed"`
	ExpiresAt    time.Time                 `json:"expires_at"`
}

// LoaderStatistics contains data loader performance statistics
type LoaderStatistics struct {
	TotalBatches     int                       `json:"total_batches"`
	AverageBatchTime time.Duration             `json:"average_batch_time"`
	ThroughputBPS    float64                   `json:"throughput_bps"`
	MemoryUsage      int64                     `json:"memory_usage_bytes"`
	CacheHitRate     float64                   `json:"cache_hit_rate"`
	ErrorRate        float64                   `json:"error_rate"`
	LastUpdated      time.Time                 `json:"last_updated"`
}

// Preprocessor interface for data preprocessing
type Preprocessor interface {
	Name() string
	Process(ctx context.Context, data interface{}) (interface{}, error)
	Validate(config map[string]interface{}) error
	GetConfig() map[string]interface{}
}

// NewDatasetManager creates a new dataset manager
func NewDatasetManager() *DatasetManager {
	dm := &DatasetManager{
		datasets:      make(map[uuid.UUID]*Dataset),
		dataLoaders:   make(map[uuid.UUID]*DataLoader),
		preprocessors: make(map[string]Preprocessor),
	}
	
	// Register default preprocessors
	dm.registerDefaultPreprocessors()
	
	return dm
}

// CreateDataset creates a new dataset
func (dm *DatasetManager) CreateDataset(ctx context.Context, dataset *Dataset) error {
	if dataset.ID == uuid.Nil {
		dataset.ID = uuid.New()
	}
	
	dataset.Status = DatasetStatusCreating
	dataset.CreatedAt = time.Now()
	dataset.UpdatedAt = time.Now()
	dataset.Version = "1.0.0"
	
	// Validate dataset
	if err := dm.validateDataset(dataset); err != nil {
		return fmt.Errorf("invalid dataset: %w", err)
	}
	
	dm.mutex.Lock()
	dm.datasets[dataset.ID] = dataset
	dm.mutex.Unlock()
	
	// Process dataset asynchronously
	go dm.processDataset(ctx, dataset)
	
	log.Printf("Dataset created: %s (%s)", dataset.Name, dataset.ID)
	return nil
}

// GetDataset retrieves a dataset by ID
func (dm *DatasetManager) GetDataset(ctx context.Context, datasetID uuid.UUID) (*Dataset, error) {
	dm.mutex.RLock()
	defer dm.mutex.RUnlock()
	
	dataset, exists := dm.datasets[datasetID]
	if !exists {
		return nil, fmt.Errorf("dataset not found: %s", datasetID)
	}
	
	return dataset, nil
}

// ListDatasets lists datasets with filtering
func (dm *DatasetManager) ListDatasets(ctx context.Context, filter *DatasetFilter) ([]*Dataset, error) {
	dm.mutex.RLock()
	defer dm.mutex.RUnlock()
	
	var filteredDatasets []*Dataset
	for _, dataset := range dm.datasets {
		if dm.matchesDatasetFilter(dataset, filter) {
			filteredDatasets = append(filteredDatasets, dataset)
		}
	}
	
	return filteredDatasets, nil
}

// UpdateDataset updates an existing dataset
func (dm *DatasetManager) UpdateDataset(ctx context.Context, datasetID uuid.UUID, updates *DatasetUpdate) error {
	dm.mutex.Lock()
	defer dm.mutex.Unlock()
	
	dataset, exists := dm.datasets[datasetID]
	if !exists {
		return fmt.Errorf("dataset not found: %s", datasetID)
	}
	
	// Apply updates
	if updates.Name != "" {
		dataset.Name = updates.Name
	}
	if updates.Description != "" {
		dataset.Description = updates.Description
	}
	if len(updates.Tags) > 0 {
		dataset.Tags = updates.Tags
	}
	if len(updates.Labels) > 0 {
		dataset.Labels = updates.Labels
	}
	
	dataset.UpdatedAt = time.Now()
	
	// Add change log entry
	change := DatasetChange{
		Version:     dataset.Version,
		ChangeType:  "update",
		Description: "Dataset metadata updated",
		ChangedBy:   updates.UpdatedBy,
		ChangedAt:   time.Now(),
		Details:     map[string]interface{}{"updates": updates},
	}
	dataset.ChangeLog = append(dataset.ChangeLog, change)
	
	log.Printf("Dataset updated: %s", datasetID)
	return nil
}

// DeleteDataset deletes a dataset
func (dm *DatasetManager) DeleteDataset(ctx context.Context, datasetID uuid.UUID) error {
	dm.mutex.Lock()
	defer dm.mutex.Unlock()
	
	dataset, exists := dm.datasets[datasetID]
	if !exists {
		return fmt.Errorf("dataset not found: %s", datasetID)
	}
	
	// Archive instead of delete
	dataset.Status = DatasetStatusArchived
	dataset.UpdatedAt = time.Now()
	
	log.Printf("Dataset archived: %s", datasetID)
	return nil
}

// CreateDataLoader creates a data loader for a dataset
func (dm *DatasetManager) CreateDataLoader(ctx context.Context, datasetID uuid.UUID, config *DataLoaderConfig) (*DataLoader, error) {
	dataset, err := dm.GetDataset(ctx, datasetID)
	if err != nil {
		return nil, err
	}
	
	if dataset.Status != DatasetStatusReady {
		return nil, fmt.Errorf("dataset is not ready: %s", dataset.Status)
	}
	
	loader := &DataLoader{
		ID:        uuid.New(),
		DatasetID: datasetID,
		Config:    config,
	}
	
	dm.mutex.Lock()
	dm.dataLoaders[loader.ID] = loader
	dm.mutex.Unlock()
	
	log.Printf("Data loader created for dataset %s: %s", datasetID, loader.ID)
	return loader, nil
}

// Internal methods

func (dm *DatasetManager) validateDataset(dataset *Dataset) error {
	if dataset.Name == "" {
		return fmt.Errorf("dataset name is required")
	}
	if dataset.TenantID == uuid.Nil {
		return fmt.Errorf("tenant_id is required")
	}
	if dataset.SourcePath == "" {
		return fmt.Errorf("source_path is required")
	}
	return nil
}

func (dm *DatasetManager) processDataset(ctx context.Context, dataset *Dataset) {
	dataset.Status = DatasetStatusProcessing
	
	// Simulate dataset processing
	time.Sleep(2 * time.Second)
	
	// Analyze dataset and compute statistics
	err := dm.analyzeDataset(ctx, dataset)
	if err != nil {
		dataset.Status = DatasetStatusError
		log.Printf("Failed to process dataset %s: %v", dataset.ID, err)
		return
	}
	
	dataset.Status = DatasetStatusReady
	dataset.UpdatedAt = time.Now()
	
	log.Printf("Dataset processing completed: %s", dataset.ID)
}

func (dm *DatasetManager) analyzeDataset(ctx context.Context, dataset *Dataset) error {
	// Generate sample statistics
	dataset.Statistics = &DatasetStatistics{
		RecordCount:   1000,
		FieldCount:    10,
		MissingValues: make(map[string]int),
		UniqueValues:  make(map[string]int),
		NumericalStats: map[string]NumericalStats{
			"age": {
				Count:  1000,
				Mean:   35.5,
				Median: 34.0,
				StdDev: 12.3,
				Min:    18.0,
				Max:    65.0,
				Q1:     28.0,
				Q3:     45.0,
			},
		},
		CategoricalStats: map[string]CategoricalStats{
			"category": {
				Count:        1000,
				UniqueValues: 5,
				MostFrequent: "A",
				Distribution: map[string]int{
					"A": 300,
					"B": 250,
					"C": 200,
					"D": 150,
					"E": 100,
				},
				Entropy: 2.1,
			},
		},
		ComputedAt: time.Now(),
	}
	
	// Assess data quality
	dataset.QualityMetrics = &DataQualityMetrics{
		CompletenessScore: 0.95,
		AccuracyScore:     0.98,
		ConsistencyScore:  0.92,
		ValidityScore:     0.97,
		UniquenessScore:   0.99,
		TimelinessScore:   0.90,
		Issues: []DataQualityIssue{
			{
				Type:        "missing_values",
				Severity:    "medium",
				Field:       "optional_field",
				Description: "5% of records have missing values in optional_field",
				Count:       50,
				Impact:      "May affect model performance",
			},
		},
		Recommendations: []DataQualityRecommendation{
			{
				Type:         "imputation",
				Priority:     "medium",
				Description:  "Consider imputing missing values in optional_field",
				Action:       "Apply mean/mode imputation",
				EstimatedImpact: 0.02,
			},
		},
		AssessedAt: time.Now(),
	}
	
	dataset.QualityScore = 0.95
	dataset.RecordCount = 1000
	dataset.FeatureCount = 9
	dataset.Size = 1024 * 1024 * 10 // 10MB
	
	return nil
}

func (dm *DatasetManager) matchesDatasetFilter(dataset *Dataset, filter *DatasetFilter) bool {
	if filter == nil {
		return true
	}
	
	if filter.TenantID != uuid.Nil && dataset.TenantID != filter.TenantID {
		return false
	}
	
	if len(filter.DatasetTypes) > 0 {
		found := false
		for _, datasetType := range filter.DatasetTypes {
			if dataset.DatasetType == datasetType {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	
	if len(filter.Statuses) > 0 {
		found := false
		for _, status := range filter.Statuses {
			if dataset.Status == status {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	
	return true
}

func (dm *DatasetManager) registerDefaultPreprocessors() {
	// Register built-in preprocessors
	dm.preprocessors["normalize"] = &NormalizationPreprocessor{}
	dm.preprocessors["tokenize"] = &TokenizationPreprocessor{}
	dm.preprocessors["encode"] = &EncodingPreprocessor{}
	dm.preprocessors["augment"] = &AugmentationPreprocessor{}
}

// Filter and update types

// DatasetFilter represents filters for dataset queries
type DatasetFilter struct {
	TenantID     uuid.UUID       `json:"tenant_id,omitempty"`
	DatasetTypes []DatasetType   `json:"dataset_types,omitempty"`
	Statuses     []DatasetStatus `json:"statuses,omitempty"`
	CreatedBy    *uuid.UUID      `json:"created_by,omitempty"`
	Tags         []string        `json:"tags,omitempty"`
	Limit        int             `json:"limit,omitempty"`
	Offset       int             `json:"offset,omitempty"`
}

// DatasetUpdate represents updates to a dataset
type DatasetUpdate struct {
	Name        string                    `json:"name,omitempty"`
	Description string                    `json:"description,omitempty"`
	Tags        []string                  `json:"tags,omitempty"`
	Labels      map[string]string         `json:"labels,omitempty"`
	Metadata    map[string]interface{}    `json:"metadata,omitempty"`
	UpdatedBy   uuid.UUID                 `json:"updated_by"`
}

// Default preprocessor implementations

// NormalizationPreprocessor normalizes numerical data
type NormalizationPreprocessor struct {
	config map[string]interface{}
}

func (p *NormalizationPreprocessor) Name() string {
	return "normalize"
}

func (p *NormalizationPreprocessor) Process(ctx context.Context, data interface{}) (interface{}, error) {
	// Implementation would normalize the data
	return data, nil
}

func (p *NormalizationPreprocessor) Validate(config map[string]interface{}) error {
	return nil
}

func (p *NormalizationPreprocessor) GetConfig() map[string]interface{} {
	return p.config
}

// TokenizationPreprocessor tokenizes text data
type TokenizationPreprocessor struct {
	config map[string]interface{}
}

func (p *TokenizationPreprocessor) Name() string {
	return "tokenize"
}

func (p *TokenizationPreprocessor) Process(ctx context.Context, data interface{}) (interface{}, error) {
	// Implementation would tokenize text data
	return data, nil
}

func (p *TokenizationPreprocessor) Validate(config map[string]interface{}) error {
	return nil
}

func (p *TokenizationPreprocessor) GetConfig() map[string]interface{} {
	return p.config
}

// EncodingPreprocessor encodes categorical data
type EncodingPreprocessor struct {
	config map[string]interface{}
}

func (p *EncodingPreprocessor) Name() string {
	return "encode"
}

func (p *EncodingPreprocessor) Process(ctx context.Context, data interface{}) (interface{}, error) {
	// Implementation would encode categorical data
	return data, nil
}

func (p *EncodingPreprocessor) Validate(config map[string]interface{}) error {
	return nil
}

func (p *EncodingPreprocessor) GetConfig() map[string]interface{} {
	return p.config
}

// AugmentationPreprocessor augments training data
type AugmentationPreprocessor struct {
	config map[string]interface{}
}

func (p *AugmentationPreprocessor) Name() string {
	return "augment"
}

func (p *AugmentationPreprocessor) Process(ctx context.Context, data interface{}) (interface{}, error) {
	// Implementation would augment training data
	return data, nil
}

func (p *AugmentationPreprocessor) Validate(config map[string]interface{}) error {
	return nil
}

func (p *AugmentationPreprocessor) GetConfig() map[string]interface{} {
	return p.config
}