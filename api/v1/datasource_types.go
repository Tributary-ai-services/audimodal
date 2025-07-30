package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DataSourceSpec defines the desired state of DataSource
type DataSourceSpec struct {
	// Name is the data source name
	Name string `json:"name"`

	// Description is the data source description
	Description string `json:"description,omitempty"`

	// Type is the data source type
	// +kubebuilder:validation:Enum=filesystem;s3;gcs;azure_blob;sharepoint;google_drive;box;dropbox;onedrive;confluence;jira;slack;notion;database;api;ftp;sftp;webdav
	Type string `json:"type"`

	// Priority is the processing priority
	// +kubebuilder:validation:Enum=low;normal;high;critical
	// +kubebuilder:default=normal
	Priority string `json:"priority,omitempty"`

	// Enabled determines whether the data source is enabled
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// Config contains type-specific configuration
	Config DataSourceConfig `json:"config"`

	// Sync contains synchronization settings
	Sync DataSourceSync `json:"sync,omitempty"`

	// Processing contains processing configuration
	Processing DataSourceProcessing `json:"processing,omitempty"`

	// Monitoring contains monitoring and alerting configuration
	Monitoring DataSourceMonitoring `json:"monitoring,omitempty"`
}

// DataSourceConfig contains type-specific configuration
type DataSourceConfig struct {
	// Path is the base path or URL for the data source
	Path string `json:"path,omitempty"`

	// Credentials reference to Kubernetes secret containing credentials
	Credentials DataSourceCredentials `json:"credentials,omitempty"`

	// File filtering options
	Includes           []string `json:"includes,omitempty"`
	Excludes           []string `json:"excludes,omitempty"`
	MaxFileSize        string   `json:"maxFileSize,omitempty"`
	SupportedExtensions []string `json:"supportedExtensions,omitempty"`

	// Cloud storage specific
	Bucket string `json:"bucket,omitempty"`
	Region string `json:"region,omitempty"`

	// SharePoint/Office 365 specific
	TenantID     string   `json:"tenantId,omitempty"`
	SiteURL      string   `json:"siteUrl,omitempty"`
	LibraryNames []string `json:"libraryNames,omitempty"`

	// Database specific
	ConnectionString string   `json:"connectionString,omitempty"`
	Tables          []string `json:"tables,omitempty"`

	// API specific
	Endpoint  string            `json:"endpoint,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	RateLimit DataSourceRateLimit `json:"rateLimit,omitempty"`
}

// DataSourceCredentials contains credential configuration
type DataSourceCredentials struct {
	// SecretRef is a reference to a Kubernetes secret containing credentials
	SecretRef DataSourceSecretRef `json:"secretRef,omitempty"`
}

// DataSourceSecretRef references a Kubernetes secret
type DataSourceSecretRef struct {
	// Name is the name of the secret
	Name string `json:"name"`
	// Namespace is the namespace of the secret
	Namespace string `json:"namespace,omitempty"`
}

// DataSourceRateLimit contains rate limiting configuration
type DataSourceRateLimit struct {
	// RequestsPerSecond is the maximum requests per second
	// +kubebuilder:validation:Minimum=0.1
	// +kubebuilder:validation:Maximum=1000
	// +kubebuilder:default=10
	RequestsPerSecond float64 `json:"requestsPerSecond,omitempty"`

	// MaxConcurrent is the maximum concurrent requests
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:default=5
	MaxConcurrent int `json:"maxConcurrent,omitempty"`
}

// DataSourceSync contains synchronization settings
type DataSourceSync struct {
	// Mode is the sync mode
	// +kubebuilder:validation:Enum=manual;scheduled;realtime;webhook
	// +kubebuilder:default=scheduled
	Mode string `json:"mode,omitempty"`

	// Schedule is the cron schedule for periodic sync
	// +kubebuilder:default="0 */6 * * *"
	Schedule string `json:"schedule,omitempty"`

	// FullSyncSchedule is the schedule for full synchronization
	// +kubebuilder:default="0 2 * * 0"
	FullSyncSchedule string `json:"fullSyncSchedule,omitempty"`

	// BatchSize is the number of files to process in each batch
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=1000
	// +kubebuilder:default=100
	BatchSize int `json:"batchSize,omitempty"`

	// ParallelWorkers is the number of parallel workers for processing
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=50
	// +kubebuilder:default=5
	ParallelWorkers int `json:"parallelWorkers,omitempty"`

	// RetryPolicy contains retry configuration
	RetryPolicy DataSourceRetryPolicy `json:"retryPolicy,omitempty"`
}

// DataSourceRetryPolicy contains retry configuration
type DataSourceRetryPolicy struct {
	// MaxRetries is the maximum number of retries
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=10
	// +kubebuilder:default=3
	MaxRetries int `json:"maxRetries,omitempty"`

	// BackoffStrategy is the backoff strategy
	// +kubebuilder:validation:Enum=linear;exponential;fixed
	// +kubebuilder:default=exponential
	BackoffStrategy string `json:"backoffStrategy,omitempty"`

	// InitialDelay is the initial delay before retry
	// +kubebuilder:default="30s"
	InitialDelay string `json:"initialDelay,omitempty"`

	// MaxDelay is the maximum delay between retries
	// +kubebuilder:default="10m"
	MaxDelay string `json:"maxDelay,omitempty"`
}

// DataSourceProcessing contains processing configuration
type DataSourceProcessing struct {
	// ChunkingStrategy is the text chunking strategy
	// +kubebuilder:validation:Enum=fixed_size;semantic;adaptive;structured
	// +kubebuilder:default=semantic
	ChunkingStrategy string `json:"chunkingStrategy,omitempty"`

	// ChunkSize is the chunk size in tokens
	// +kubebuilder:validation:Minimum=100
	// +kubebuilder:validation:Maximum=8000
	// +kubebuilder:default=1000
	ChunkSize int `json:"chunkSize,omitempty"`

	// ChunkOverlap is the overlap between chunks in tokens
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=500
	// +kubebuilder:default=100
	ChunkOverlap int `json:"chunkOverlap,omitempty"`

	// EnableOCR enables OCR for image files
	EnableOCR bool `json:"enableOCR,omitempty"`

	// EnableAnalysis enables content analysis
	// +kubebuilder:default=true
	EnableAnalysis bool `json:"enableAnalysis,omitempty"`

	// EnableDLP enables Data Loss Prevention scanning
	EnableDLP bool `json:"enableDLP,omitempty"`

	// CustomProcessors are custom processing pipeline steps
	CustomProcessors []DataSourceCustomProcessor `json:"customProcessors,omitempty"`
}

// DataSourceCustomProcessor defines a custom processing step
type DataSourceCustomProcessor struct {
	// Name is the processor name
	Name string `json:"name"`
	// Type is the processor type
	Type string `json:"type"`
	// Config is the processor configuration
	Config map[string]interface{} `json:"config,omitempty"`
}

// DataSourceMonitoring contains monitoring and alerting configuration
type DataSourceMonitoring struct {
	// HealthCheck contains health check configuration
	HealthCheck DataSourceHealthCheck `json:"healthCheck,omitempty"`

	// Alerts are alert rules for this data source
	Alerts []DataSourceAlert `json:"alerts,omitempty"`

	// Metrics contains metrics configuration
	Metrics DataSourceMetrics `json:"metrics,omitempty"`
}

// DataSourceHealthCheck contains health check configuration
type DataSourceHealthCheck struct {
	// Enabled determines if health checks are enabled
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// Interval is the health check interval
	// +kubebuilder:default="5m"
	Interval string `json:"interval,omitempty"`

	// Timeout is the health check timeout
	// +kubebuilder:default="30s"
	Timeout string `json:"timeout,omitempty"`
}

// DataSourceAlert defines an alert rule
type DataSourceAlert struct {
	// Name is the alert name
	Name string `json:"name"`
	// Condition is the alert condition
	Condition string `json:"condition"`
	// Threshold is the alert threshold
	Threshold float64 `json:"threshold"`
	// Severity is the alert severity
	// +kubebuilder:validation:Enum=info;warning;critical
	Severity string `json:"severity"`
}

// DataSourceMetrics contains metrics configuration
type DataSourceMetrics struct {
	// CollectMetrics determines if metrics are collected
	// +kubebuilder:default=true
	CollectMetrics bool `json:"collectMetrics,omitempty"`

	// CustomLabels are custom metric labels
	CustomLabels map[string]string `json:"customLabels,omitempty"`
}

// DataSourceStatus defines the observed state of DataSource
type DataSourceStatus struct {
	// Phase is the current lifecycle phase
	// +kubebuilder:validation:Enum=Pending;Configuring;Active;Syncing;Error;Suspended;Terminating
	Phase string `json:"phase,omitempty"`

	// Conditions represent the latest available observations of the data source's state
	Conditions []DataSourceCondition `json:"conditions,omitempty"`

	// LastSync contains information about the last sync operation
	LastSync DataSourceSyncStatus `json:"lastSync,omitempty"`

	// NextSync is the next scheduled sync time
	NextSync *metav1.Time `json:"nextSync,omitempty"`

	// Statistics contains data source statistics
	Statistics DataSourceStatistics `json:"statistics,omitempty"`

	// Health contains health status information
	Health DataSourceHealth `json:"health,omitempty"`

	// ObservedGeneration is the generation of the spec that was last processed
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// DataSourceCondition describes the state of a data source at a certain point
type DataSourceCondition struct {
	// Type of data source condition
	// +kubebuilder:validation:Enum=Ready;Connected;Configured;Syncing;Healthy;CredentialsValid
	Type string `json:"type"`

	// Status of the condition
	// +kubebuilder:validation:Enum=True;False;Unknown
	Status string `json:"status"`

	// LastTransitionTime is the last time the condition transitioned
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`

	// Reason is a unique, one-word, CamelCase reason for the condition's last transition
	Reason string `json:"reason,omitempty"`

	// Message is a human-readable message indicating details about the transition
	Message string `json:"message,omitempty"`
}

// DataSourceSyncStatus contains information about sync operations
type DataSourceSyncStatus struct {
	// StartTime is the sync start time
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// EndTime is the sync end time
	EndTime *metav1.Time `json:"endTime,omitempty"`

	// Status is the sync status
	// +kubebuilder:validation:Enum=success;partial;failed;running
	Status string `json:"status,omitempty"`

	// FilesProcessed is the number of files processed
	FilesProcessed int `json:"filesProcessed,omitempty"`

	// FilesSkipped is the number of files skipped
	FilesSkipped int `json:"filesSkipped,omitempty"`

	// FilesErrored is the number of files with errors
	FilesErrored int `json:"filesErrored,omitempty"`

	// TotalSize is the total size of processed files
	TotalSize string `json:"totalSize,omitempty"`

	// ErrorMessage contains error details if the sync failed
	ErrorMessage string `json:"errorMessage,omitempty"`
}

// DataSourceStatistics contains data source statistics
type DataSourceStatistics struct {
	// TotalFiles is the total files discovered
	TotalFiles int `json:"totalFiles,omitempty"`

	// ProcessedFiles is the successfully processed files
	ProcessedFiles int `json:"processedFiles,omitempty"`

	// ErroredFiles is the files with processing errors
	ErroredFiles int `json:"erroredFiles,omitempty"`

	// TotalSizeBytes is the total size of all files in bytes
	TotalSizeBytes int64 `json:"totalSizeBytes,omitempty"`

	// AverageProcessingTime is the average processing time per file
	AverageProcessingTime string `json:"averageProcessingTime,omitempty"`

	// LastSuccessfulSync is the last successful sync timestamp
	LastSuccessfulSync *metav1.Time `json:"lastSuccessfulSync,omitempty"`
}

// DataSourceHealth contains health status information
type DataSourceHealth struct {
	// Status is the health status
	// +kubebuilder:validation:Enum=healthy;degraded;unhealthy;unknown
	Status string `json:"status,omitempty"`

	// LastCheck is the last health check timestamp
	LastCheck metav1.Time `json:"lastCheck,omitempty"`

	// Message contains health status details
	Message string `json:"message,omitempty"`

	// ResponseTime is the response time in milliseconds
	ResponseTime string `json:"responseTime,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.type",description="Data source type"
//+kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase",description="Current phase"
//+kubebuilder:printcolumn:name="Health",type="string",JSONPath=".status.health.status",description="Health status"
//+kubebuilder:printcolumn:name="Last Sync",type="string",JSONPath=".status.lastSync.status",description="Last sync status"
//+kubebuilder:printcolumn:name="Files",type="integer",JSONPath=".status.statistics.totalFiles",description="Total files"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// DataSource is the Schema for the datasources API
type DataSource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DataSourceSpec   `json:"spec,omitempty"`
	Status DataSourceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DataSourceList contains a list of DataSource
type DataSourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DataSource `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DataSource{}, &DataSourceList{})
}