package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TenantSpec defines the desired state of Tenant
type TenantSpec struct {
	// Name is the human-readable tenant name
	Name string `json:"name"`

	// Slug is the URL-safe tenant identifier
	Slug string `json:"slug,omitempty"`

	// Plan is the subscription plan
	// +kubebuilder:validation:Enum=free;pro;enterprise
	Plan string `json:"plan"`

	// Status is the tenant status
	// +kubebuilder:validation:Enum=active;suspended;pending;deleted
	// +kubebuilder:default=pending
	Status string `json:"status,omitempty"`

	// Settings contains tenant configuration settings
	Settings TenantSettings `json:"settings,omitempty"`

	// Resources defines Kubernetes resource requirements
	Resources TenantResources `json:"resources,omitempty"`

	// Security contains security configuration
	Security TenantSecurity `json:"security,omitempty"`

	// Billing contains billing information
	Billing TenantBilling `json:"billing,omitempty"`
}

// TenantSettings contains tenant configuration settings
type TenantSettings struct {
	// MaxStorageGB is the maximum storage in GB
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10000
	MaxStorageGB int `json:"maxStorageGB,omitempty"`

	// MaxUsers is the maximum number of users
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=1000
	MaxUsers int `json:"maxUsers,omitempty"`

	// MaxDataSources is the maximum number of data sources
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	MaxDataSources int `json:"maxDataSources,omitempty"`

	// RetentionDays is the data retention period in days
	// +kubebuilder:validation:Minimum=30
	// +kubebuilder:validation:Maximum=2555
	// +kubebuilder:default=365
	RetentionDays int `json:"retentionDays,omitempty"`

	// EnableDLP enables Data Loss Prevention
	EnableDLP bool `json:"enableDLP,omitempty"`

	// EnableAdvancedAnalytics enables advanced analytics features
	EnableAdvancedAnalytics bool `json:"enableAdvancedAnalytics,omitempty"`

	// AllowedRegions are the allowed regions for data processing
	AllowedRegions []string `json:"allowedRegions,omitempty"`
}

// TenantResources defines Kubernetes resource requirements
type TenantResources struct {
	// CPU limit (e.g., '2', '500m')
	// +kubebuilder:default="1"
	CPU string `json:"cpu,omitempty"`

	// Memory limit (e.g., '2Gi', '512Mi')
	// +kubebuilder:default="1Gi"
	Memory string `json:"memory,omitempty"`

	// Storage limit (e.g., '10Gi', '1Ti')
	// +kubebuilder:default="10Gi"
	Storage string `json:"storage,omitempty"`

	// GPUCount is the number of GPUs (for ML processing)
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=8
	GPUCount int `json:"gpuCount,omitempty"`
}

// TenantSecurity contains security configuration
type TenantSecurity struct {
	// IsolationLevel defines the tenant isolation level
	// +kubebuilder:validation:Enum=namespace;cluster;node
	// +kubebuilder:default=namespace
	IsolationLevel string `json:"isolationLevel,omitempty"`

	// EncryptionAtRest enables encryption at rest
	// +kubebuilder:default=true
	EncryptionAtRest bool `json:"encryptionAtRest,omitempty"`

	// EncryptionInTransit enables encryption in transit
	// +kubebuilder:default=true
	EncryptionInTransit bool `json:"encryptionInTransit,omitempty"`

	// NetworkPolicies enables network policies
	// +kubebuilder:default=true
	NetworkPolicies bool `json:"networkPolicies,omitempty"`

	// AllowedIPs are the allowed IP ranges for access
	AllowedIPs []string `json:"allowedIPs,omitempty"`
}

// TenantBilling contains billing information
type TenantBilling struct {
	// BillingContact is the billing contact email
	BillingContact string `json:"billingContact,omitempty"`

	// CostCenter is the cost center or department
	CostCenter string `json:"costCenter,omitempty"`

	// PaymentMethod is the payment method identifier
	PaymentMethod string `json:"paymentMethod,omitempty"`
}

// TenantStatus defines the observed state of Tenant
type TenantStatus struct {
	// Phase is the current phase of tenant lifecycle
	// +kubebuilder:validation:Enum=Pending;Provisioning;Active;Suspended;Terminating;Failed
	Phase string `json:"phase,omitempty"`

	// Conditions represent the latest available observations of the tenant's state
	Conditions []TenantCondition `json:"conditions,omitempty"`

	// Resources contains information about created Kubernetes resources
	Resources TenantResourceStatus `json:"resources,omitempty"`

	// Usage contains current usage statistics
	Usage TenantUsage `json:"usage,omitempty"`

	// ObservedGeneration is the generation of the spec that was last processed
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// TenantCondition describes the state of a tenant at a certain point
type TenantCondition struct {
	// Type of tenant condition
	// +kubebuilder:validation:Enum=Ready;Provisioned;NetworkConfigured;StorageConfigured;DLPEnabled;SecurityConfigured
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

// TenantResourceStatus contains information about created Kubernetes resources
type TenantResourceStatus struct {
	// Namespace is the Kubernetes namespace for this tenant
	Namespace string `json:"namespace,omitempty"`

	// ServiceAccount is the service account for tenant workloads
	ServiceAccount string `json:"serviceAccount,omitempty"`

	// ConfigMaps are the created config maps
	ConfigMaps []string `json:"configMaps,omitempty"`

	// Secrets are the created secrets
	Secrets []string `json:"secrets,omitempty"`

	// PersistentVolumeClaims are the created PVCs
	PersistentVolumeClaims []string `json:"persistentVolumeClaims,omitempty"`
}

// TenantUsage contains current usage statistics
type TenantUsage struct {
	// StorageUsedGB is the current storage usage in GB
	StorageUsedGB float64 `json:"storageUsedGB,omitempty"`

	// DocumentsProcessed is the total documents processed
	DocumentsProcessed int64 `json:"documentsProcessed,omitempty"`

	// APICallsToday is the API calls made today
	APICallsToday int `json:"apiCallsToday,omitempty"`

	// EmbeddingsGenerated is the total embeddings generated
	EmbeddingsGenerated int64 `json:"embeddingsGenerated,omitempty"`

	// LastActivity is the last tenant activity timestamp
	LastActivity *metav1.Time `json:"lastActivity,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas
//+kubebuilder:printcolumn:name="Plan",type="string",JSONPath=".spec.plan",description="Subscription plan"
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".spec.status",description="Tenant status"
//+kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase",description="Current phase"
//+kubebuilder:printcolumn:name="Storage Used",type="string",JSONPath=".status.usage.storageUsedGB",description="Storage usage"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Tenant is the Schema for the tenants API
type Tenant struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TenantSpec   `json:"spec,omitempty"`
	Status TenantStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// TenantList contains a list of Tenant
type TenantList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Tenant `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Tenant{}, &TenantList{})
}
