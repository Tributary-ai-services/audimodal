package confluence

import (
	"context"
	"sync"
	"time"
)

// Confluence API request/response structures

// Space represents a Confluence space
type Space struct {
	ID          int64       `json:"id"`
	Key         string      `json:"key"`
	Name        string      `json:"name"`
	Type        string      `json:"type"`
	Status      string      `json:"status"`
	Description *Description `json:"description,omitempty"`
	Homepage    *Content    `json:"homepage,omitempty"`
	Expandable  *Expandable `json:"_expandable,omitempty"`
	Links       *Links      `json:"_links,omitempty"`
}

// Content represents a Confluence content (page, blog post, comment, etc.)
type Content struct {
	ID          string        `json:"id"`
	Type        string        `json:"type"`
	Status      string        `json:"status"`
	Title       string        `json:"title"`
	Space       *Space        `json:"space,omitempty"`
	History     *History      `json:"history,omitempty"`
	Version     *Version      `json:"version,omitempty"`
	Ancestors   []Content     `json:"ancestors,omitempty"`
	Operations  []Operation   `json:"operations,omitempty"`
	Children    *Children     `json:"children,omitempty"`
	Descendants *Descendants  `json:"descendants,omitempty"`
	Container   *Container    `json:"container,omitempty"`
	Body        *Body         `json:"body,omitempty"`
	Restrictions *Restrictions `json:"restrictions,omitempty"`
	Metadata    *Metadata     `json:"metadata,omitempty"`
	Extensions  *Extensions   `json:"extensions,omitempty"`
	Expandable  *Expandable   `json:"_expandable,omitempty"`
	Links       *Links        `json:"_links,omitempty"`
}

// Description represents a description of a space or content
type Description struct {
	Plain *Plain `json:"plain,omitempty"`
	View  *View  `json:"view,omitempty"`
}

// Plain represents plain text representation
type Plain struct {
	Value          string `json:"value"`
	Representation string `json:"representation"`
	EmbeddedContent []Content `json:"embeddedContent,omitempty"`
}

// View represents view representation
type View struct {
	Value          string `json:"value"`
	Representation string `json:"representation"`
	EmbeddedContent []Content `json:"embeddedContent,omitempty"`
}

// History represents content history information
type History struct {
	Latest      bool        `json:"latest"`
	CreatedBy   *User       `json:"createdBy,omitempty"`
	CreatedDate string      `json:"createdDate,omitempty"`
	LastUpdated *LastUpdated `json:"lastUpdated,omitempty"`
	PreviousVersion *Version `json:"previousVersion,omitempty"`
	Contributors *Contributors `json:"contributors,omitempty"`
	NextVersion *Version     `json:"nextVersion,omitempty"`
	Expandable  *Expandable  `json:"_expandable,omitempty"`
	Links       *Links       `json:"_links,omitempty"`
}

// Version represents content version information
type Version struct {
	By          *User       `json:"by,omitempty"`
	When        string      `json:"when,omitempty"`
	FriendlyWhen string     `json:"friendlyWhen,omitempty"`
	Message     string      `json:"message,omitempty"`
	Number      int         `json:"number"`
	MinorEdit   bool        `json:"minorEdit"`
	SyncRev     string      `json:"syncRev,omitempty"`
	SyncRevSource string    `json:"syncRevSource,omitempty"`
	ConfRev     string      `json:"confRev,omitempty"`
	ContentTypeModified bool `json:"contentTypeModified"`
	Expandable  *Expandable `json:"_expandable,omitempty"`
	Links       *Links      `json:"_links,omitempty"`
}

// User represents a Confluence user
type User struct {
	Type           string         `json:"type"`
	AccountID      string         `json:"accountId,omitempty"`
	AccountType    string         `json:"accountType,omitempty"`
	Email          string         `json:"email,omitempty"`
	PublicName     string         `json:"publicName,omitempty"`
	ProfilePicture *ProfilePicture `json:"profilePicture,omitempty"`
	DisplayName    string         `json:"displayName,omitempty"`
	Operations     []Operation    `json:"operations,omitempty"`
	Details        *UserDetails   `json:"details,omitempty"`
	PersonalSpace  *Space         `json:"personalSpace,omitempty"`
	Expandable     *Expandable    `json:"_expandable,omitempty"`
	Links          *Links         `json:"_links,omitempty"`
}

// ProfilePicture represents a user's profile picture
type ProfilePicture struct {
	Path      string `json:"path"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	IsDefault bool   `json:"isDefault"`
}

// UserDetails represents additional user details
type UserDetails struct {
	Business *BusinessDetails `json:"business,omitempty"`
	Personal *PersonalDetails `json:"personal,omitempty"`
}

// BusinessDetails represents business-related user details
type BusinessDetails struct {
	Position   string `json:"position,omitempty"`
	Department string `json:"department,omitempty"`
	Location   string `json:"location,omitempty"`
}

// PersonalDetails represents personal user details
type PersonalDetails struct {
	Phone   string `json:"phone,omitempty"`
	Website string `json:"website,omitempty"`
	Im      string `json:"im,omitempty"`
}

// LastUpdated represents last updated information
type LastUpdated struct {
	By          *User  `json:"by,omitempty"`
	When        string `json:"when,omitempty"`
	FriendlyWhen string `json:"friendlyWhen,omitempty"`
	Message     string `json:"message,omitempty"`
	Number      int    `json:"number"`
	MinorEdit   bool   `json:"minorEdit"`
	SyncRev     string `json:"syncRev,omitempty"`
	ConfRev     string `json:"confRev,omitempty"`
	Expandable  *Expandable `json:"_expandable,omitempty"`
	Links       *Links      `json:"_links,omitempty"`
}

// Contributors represents content contributors
type Contributors struct {
	Publishers *PublisherCollection `json:"publishers,omitempty"`
	Expandable *Expandable          `json:"_expandable,omitempty"`
	Links      *Links               `json:"_links,omitempty"`
}

// PublisherCollection represents a collection of publishers
type PublisherCollection struct {
	Users   []User      `json:"users"`
	UserKeys []string    `json:"userKeys,omitempty"`
	Expandable *Expandable `json:"_expandable,omitempty"`
	Links    *Links       `json:"_links,omitempty"`
}

// Operation represents an operation that can be performed
type Operation struct {
	Operation   string `json:"operation"`
	TargetType  string `json:"targetType"`
}

// Children represents child content
type Children struct {
	Page       *ContentCollection `json:"page,omitempty"`
	Comment    *ContentCollection `json:"comment,omitempty"`
	Attachment *ContentCollection `json:"attachment,omitempty"`
	Expandable *Expandable        `json:"_expandable,omitempty"`
	Links      *Links             `json:"_links,omitempty"`
}

// Descendants represents descendant content
type Descendants struct {
	Page       *ContentCollection `json:"page,omitempty"`
	Comment    *ContentCollection `json:"comment,omitempty"`
	Attachment *ContentCollection `json:"attachment,omitempty"`
	Expandable *Expandable        `json:"_expandable,omitempty"`
	Links      *Links             `json:"_links,omitempty"`
}

// ContentCollection represents a collection of content
type ContentCollection struct {
	Results    []Content   `json:"results"`
	Start      int         `json:"start"`
	Limit      int         `json:"limit"`
	Size       int         `json:"size"`
	Expandable *Expandable `json:"_expandable,omitempty"`
	Links      *Links      `json:"_links,omitempty"`
}

// Container represents a content container
type Container struct {
	ID         string      `json:"id"`
	Key        string      `json:"key"`
	Name       string      `json:"name"`
	Type       string      `json:"type"`
	Status     string      `json:"status"`
	Expandable *Expandable `json:"_expandable,omitempty"`
	Links      *Links      `json:"_links,omitempty"`
}

// Body represents content body
type Body struct {
	View                *View       `json:"view,omitempty"`
	ExportView          *View       `json:"export_view,omitempty"`
	StyledView          *View       `json:"styled_view,omitempty"`
	Storage             *Storage    `json:"storage,omitempty"`
	Editor              *View       `json:"editor,omitempty"`
	AtlasDocFormat      *View       `json:"atlas_doc_format,omitempty"`
	WikiMarkup          *View       `json:"wiki,omitempty"`
	Anonymous           *View       `json:"anonymous_export_view,omitempty"`
	Expandable          *Expandable `json:"_expandable,omitempty"`
	Links               *Links      `json:"_links,omitempty"`
}

// Storage represents storage format content
type Storage struct {
	Value          string    `json:"value"`
	Representation string    `json:"representation"`
	EmbeddedContent []Content `json:"embeddedContent,omitempty"`
}

// Restrictions represents content restrictions
type Restrictions struct {
	Read   *RestrictionCollection `json:"read,omitempty"`
	Update *RestrictionCollection `json:"update,omitempty"`
	Expandable *Expandable         `json:"_expandable,omitempty"`
	Links  *Links                  `json:"_links,omitempty"`
}

// RestrictionCollection represents a collection of restrictions
type RestrictionCollection struct {
	Results    []Restriction `json:"results"`
	Start      int           `json:"start"`
	Limit      int           `json:"limit"`
	Size       int           `json:"size"`
	Expandable *Expandable   `json:"_expandable,omitempty"`
	Links      *Links        `json:"_links,omitempty"`
}

// Restriction represents a single restriction
type Restriction struct {
	Operation   string        `json:"operation"`
	Restrictions *Restrictions `json:"restrictions,omitempty"`
	Content     *Content      `json:"content,omitempty"`
	Expandable  *Expandable   `json:"_expandable,omitempty"`
	Links       *Links        `json:"_links,omitempty"`
}

// Metadata represents content metadata
type Metadata struct {
	Properties   map[string]interface{} `json:"properties,omitempty"`
	Frontend     *Frontend              `json:"frontend,omitempty"`
	EditorConfig *EditorConfig          `json:"editorConfig,omitempty"`
	Expandable   *Expandable            `json:"_expandable,omitempty"`
	Links        *Links                 `json:"_links,omitempty"`
}

// Frontend represents frontend metadata
type Frontend struct {
	Title  string `json:"title,omitempty"`
	Diff   string `json:"diff,omitempty"`
}

// EditorConfig represents editor configuration
type EditorConfig struct {
	CollaborativeEditingEnabled bool `json:"collaborativeEditingEnabled"`
}

// Extensions represents content extensions
type Extensions struct {
	Position   string      `json:"position,omitempty"`
	Expandable *Expandable `json:"_expandable,omitempty"`
	Links      *Links      `json:"_links,omitempty"`
}

// Expandable represents expandable resources
type Expandable struct {
	ChildTypes        string `json:"childTypes,omitempty"`
	Container         string `json:"container,omitempty"`
	Metadata          string `json:"metadata,omitempty"`
	Operations        string `json:"operations,omitempty"`
	Children          string `json:"children,omitempty"`
	Restrictions      string `json:"restrictions,omitempty"`
	History           string `json:"history,omitempty"`
	Ancestors         string `json:"ancestors,omitempty"`
	Body              string `json:"body,omitempty"`
	Version           string `json:"version,omitempty"`
	Descendants       string `json:"descendants,omitempty"`
	Space             string `json:"space,omitempty"`
}

// Links represents HAL links
type Links struct {
	Base       string `json:"base,omitempty"`
	Context    string `json:"context,omitempty"`
	Self       string `json:"self,omitempty"`
	Tinyui     string `json:"tinyui,omitempty"`
	Editui     string `json:"editui,omitempty"`
	Webui      string `json:"webui,omitempty"`
	Download   string `json:"download,omitempty"`
	Thumbnail  string `json:"thumbnail,omitempty"`
}

// Attachment represents a Confluence attachment
type Attachment struct {
	Content
	MediaType    string         `json:"mediaType,omitempty"`
	MediaTypeDescription string `json:"mediaTypeDescription,omitempty"`
	Comment      string         `json:"comment,omitempty"`
	FileSize     int64          `json:"fileSize,omitempty"`
}

// Label represents a content label
type Label struct {
	ID      string      `json:"id,omitempty"`
	Name    string      `json:"name"`
	Prefix  string      `json:"prefix"`
	Expandable *Expandable `json:"_expandable,omitempty"`
	Links   *Links       `json:"_links,omitempty"`
}

// LabelCollection represents a collection of labels
type LabelCollection struct {
	Results    []Label     `json:"results"`
	Start      int         `json:"start"`
	Limit      int         `json:"limit"`
	Size       int         `json:"size"`
	Expandable *Expandable `json:"_expandable,omitempty"`
	Links      *Links      `json:"_links,omitempty"`
}

// Search represents search results
type SearchResult struct {
	ID                    string             `json:"id"`
	Type                  string             `json:"type"`
	Status                string             `json:"status"`
	Title                 string             `json:"title"`
	Excerpt               string             `json:"excerpt,omitempty"`
	URL                   string             `json:"url,omitempty"`
	ResultGlobalContainer *Container         `json:"resultGlobalContainer,omitempty"`
	BreadcrumbTrail       []BreadcrumbItem   `json:"breadcrumbTrail,omitempty"`
	EntityType            string             `json:"entityType,omitempty"`
	IconCssClass          string             `json:"iconCssClass,omitempty"`
	LastModified          string             `json:"lastModified,omitempty"`
	FriendlyLastModified  string             `json:"friendlyLastModified,omitempty"`
	Content               *Content           `json:"content,omitempty"`
	Space                 *Space             `json:"space,omitempty"`
	User                  *User              `json:"user,omitempty"`
	Expandable            *Expandable        `json:"_expandable,omitempty"`
	Links                 *Links             `json:"_links,omitempty"`
}

// BreadcrumbItem represents a breadcrumb item
type BreadcrumbItem struct {
	Label      string `json:"label"`
	URL        string `json:"url"`
	Separator  string `json:"separator,omitempty"`
}

// SearchResultCollection represents search results
type SearchResultCollection struct {
	Results              []SearchResult       `json:"results"`
	Start                int                  `json:"start"`
	Limit                int                  `json:"limit"`
	Size                 int                  `json:"size"`
	TotalSize            int                  `json:"totalSize"`
	CqlQuery             string               `json:"cqlQuery,omitempty"`
	SearchDuration       int                  `json:"searchDuration,omitempty"`
	ArchivedResultCount  int                  `json:"archivedResultCount,omitempty"`
	Expandable           *Expandable          `json:"_expandable,omitempty"`
	Links                *Links               `json:"_links,omitempty"`
}

// SpaceCollection represents a collection of spaces
type SpaceCollection struct {
	Results    []Space     `json:"results"`
	Start      int         `json:"start"`
	Limit      int         `json:"limit"`
	Size       int         `json:"size"`
	Expandable *Expandable `json:"_expandable,omitempty"`
	Links      *Links      `json:"_links,omitempty"`
}

// UserCollection represents a collection of users
type UserCollection struct {
	Results    []User      `json:"results"`
	Start      int         `json:"start"`
	Limit      int         `json:"limit"`
	Size       int         `json:"size"`
	Expandable *Expandable `json:"_expandable,omitempty"`
	Links      *Links      `json:"_links,omitempty"`
}

// Connector-specific types

// SyncState tracks the synchronization state
type SyncState struct {
	IsRunning        bool          `json:"is_running"`
	LastSyncStart    time.Time     `json:"last_sync_start"`
	LastSyncEnd      time.Time     `json:"last_sync_end"`
	LastSyncDuration time.Duration `json:"last_sync_duration"`
	TotalContent     int64         `json:"total_content"`
	ChangedContent   int64         `json:"changed_content"`
	ErrorCount       int64         `json:"error_count"`
	LastError        string        `json:"last_error,omitempty"`
	LastSyncToken    string        `json:"last_sync_token,omitempty"` // For incremental sync
}

// ConnectorMetrics tracks connector performance metrics
type ConnectorMetrics struct {
	LastConnectionTime time.Time     `json:"last_connection_time"`
	ContentListed      int64         `json:"content_listed"`
	ContentRetrieved   int64         `json:"content_retrieved"`
	ContentDownloaded  int64         `json:"content_downloaded"`
	BytesDownloaded    int64         `json:"bytes_downloaded"`
	SyncCount          int64         `json:"sync_count"`
	LastSyncTime       time.Time     `json:"last_sync_time"`
	LastSyncDuration   time.Duration `json:"last_sync_duration"`
	LastListTime       time.Time     `json:"last_list_time"`
	ErrorCount         int64         `json:"error_count"`
	LastError          string        `json:"last_error,omitempty"`
	
	// Confluence-specific metrics
	APICallsCount      int64         `json:"api_calls_count"`
	RateLimitHits      int64         `json:"rate_limit_hits"`
	CacheHits          int64         `json:"cache_hits"`
	CacheMisses        int64         `json:"cache_misses"`
	SpacesProcessed    int64         `json:"spaces_processed"`
	PagesProcessed     int64         `json:"pages_processed"`
	AttachmentsProcessed int64       `json:"attachments_processed"`
}

// RetryPolicy defines retry behavior for API calls
type RetryPolicy struct {
	MaxRetries         int           `json:"max_retries"`
	InitialDelay       time.Duration `json:"initial_delay"`
	ExponentialBackoff bool          `json:"exponential_backoff"`
}

// RateLimiter implements token bucket rate limiting
type RateLimiter struct {
	rate       float64
	burst      int
	tokens     float64
	lastUpdate time.Time
	mu         sync.Mutex
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(rate float64, burst int) *RateLimiter {
	return &RateLimiter{
		rate:       rate,
		burst:      burst,
		tokens:     float64(burst),
		lastUpdate: time.Now(),
	}
}

// Wait blocks until a token is available
func (rl *RateLimiter) Wait(ctx context.Context) error {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastUpdate).Seconds()
	
	// Add tokens based on elapsed time
	rl.tokens = min(float64(rl.burst), rl.tokens+elapsed*rl.rate)
	rl.lastUpdate = now

	if rl.tokens >= 1.0 {
		rl.tokens--
		return nil
	}

	// Calculate wait time
	waitTime := time.Duration((1.0-rl.tokens)/rl.rate) * time.Second
	
	// Release lock and wait
	rl.mu.Unlock()
	
	select {
	case <-ctx.Done():
		rl.mu.Lock() // Re-acquire lock for defer
		return ctx.Err()
	case <-time.After(waitTime):
		rl.mu.Lock() // Re-acquire lock for defer
		rl.tokens = 0 // Consume the token
		return nil
	}
}

// Allow returns true if a token is available without blocking
func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastUpdate).Seconds()
	
	// Add tokens based on elapsed time
	rl.tokens = min(float64(rl.burst), rl.tokens+elapsed*rl.rate)
	rl.lastUpdate = now

	if rl.tokens >= 1.0 {
		rl.tokens--
		return true
	}

	return false
}

// Configuration structures for enterprise features

// EnterpriseConfig represents enterprise-specific configuration
type EnterpriseConfig struct {
	SiteURL               string            `yaml:"site_url"`
	EnableEnterpriseFeatures bool           `yaml:"enable_enterprise_features"`
	EnableAuditLog        bool              `yaml:"enable_audit_log"`
	DataGovernance        DataGovernanceConfig `yaml:"data_governance"`
	ContentManagement     ContentManagementConfig `yaml:"content_management"`
	SecuritySettings      SecurityConfig    `yaml:"security_settings"`
	ComplianceSettings    ComplianceConfig  `yaml:"compliance_settings"`
}

// DataGovernanceConfig represents data governance settings
type DataGovernanceConfig struct {
	EnableDataRetention    bool              `yaml:"enable_data_retention"`
	RetentionPeriodDays    int               `yaml:"retention_period_days"`
	EnableLegalHold        bool              `yaml:"enable_legal_hold"`
	ComplianceReporting    bool              `yaml:"compliance_reporting"`
	DataClassification     []string          `yaml:"data_classification"`
	EnableArchiving        bool              `yaml:"enable_archiving"`
	ArchivePolicies        []string          `yaml:"archive_policies"`
}

// ContentManagementConfig represents content management settings
type ContentManagementConfig struct {
	EnableVersioning       bool              `yaml:"enable_versioning"`
	MaxVersions            int               `yaml:"max_versions"`
	EnableLabels           bool              `yaml:"enable_labels"`
	EnableComments         bool              `yaml:"enable_comments"`
	EnableWatching         bool              `yaml:"enable_watching"`
	EnableLikes            bool              `yaml:"enable_likes"`
	PreviewGeneration      bool              `yaml:"preview_generation"`
	ThumbnailGeneration    bool              `yaml:"thumbnail_generation"`
	EnableMacros           bool              `yaml:"enable_macros"`
	EnableTemplates        bool              `yaml:"enable_templates"`
}

// SecurityConfig represents security configuration
type SecurityConfig struct {
	EnableEncryption       bool              `yaml:"enable_encryption"`
	EncryptionAlgorithm    string            `yaml:"encryption_algorithm"`
	EnableSSO              bool              `yaml:"enable_sso"`
	SSOProvider            string            `yaml:"sso_provider"`
	AccessControls         AccessControlConfig `yaml:"access_controls"`
	AuditSettings          AuditConfig       `yaml:"audit_settings"`
	ThreatProtection       ThreatProtectionConfig `yaml:"threat_protection"`
}

// AccessControlConfig represents access control settings
type AccessControlConfig struct {
	EnableSpacePermissions bool              `yaml:"enable_space_permissions"`
	EnablePageRestrictions bool              `yaml:"enable_page_restrictions"`
	EnableIPRestrictions   bool              `yaml:"enable_ip_restrictions"`
	IPAllowlist            []string          `yaml:"ip_allowlist"`
	SessionTimeout         time.Duration     `yaml:"session_timeout"`
	EnableTwoFactor        bool              `yaml:"enable_two_factor"`
}

// AuditConfig represents audit logging configuration
type AuditConfig struct {
	EnableContentAccess    bool              `yaml:"enable_content_access"`
	EnableUserActivity     bool              `yaml:"enable_user_activity"`
	EnableAdminActivity    bool              `yaml:"enable_admin_activity"`
	EnableSpaceActivity    bool              `yaml:"enable_space_activity"`
	RetentionPeriod        time.Duration     `yaml:"retention_period"`
	ExportFormat           string            `yaml:"export_format"`
	LogLevel               string            `yaml:"log_level"`
}

// ComplianceConfig represents compliance settings
type ComplianceConfig struct {
	EnableRecordsManagement bool             `yaml:"enable_records_management"`
	EnableeDiscovery       bool              `yaml:"enable_ediscovery"`
	EnableDataExport       bool              `yaml:"enable_data_export"`
	ComplianceCenter       string            `yaml:"compliance_center"`
	PolicyEnforcement      bool              `yaml:"policy_enforcement"`
	EnableGDPR             bool              `yaml:"enable_gdpr"`
}

// ThreatProtectionConfig represents threat protection settings
type ThreatProtectionConfig struct {
	EnableMalwareScanning  bool             `yaml:"enable_malware_scanning"`
	EnableContentFiltering bool             `yaml:"enable_content_filtering"`
	EnableSpamProtection   bool             `yaml:"enable_spam_protection"`
	QuarantineActions      []string         `yaml:"quarantine_actions"`
	AlertThresholds        map[string]int   `yaml:"alert_thresholds"`
}

// Webhook and event structures

// WebhookEvent represents a Confluence webhook event
type WebhookEvent struct {
	Timestamp    int64                  `json:"timestamp"`
	Event        string                 `json:"event"`
	UserAccountID string                `json:"userAccountId,omitempty"`
	Space        *Space                 `json:"space,omitempty"`
	Content      *Content               `json:"content,omitempty"`
	Comment      *Content               `json:"comment,omitempty"`
	User         *User                  `json:"user,omitempty"`
	Attachment   *Attachment            `json:"attachment,omitempty"`
	Label        *Label                 `json:"label,omitempty"`
	WebhookEvent string                 `json:"webhookEvent"`
	UserKey      string                 `json:"userKey,omitempty"`
}

// WebhookResponse represents the response to a webhook
type WebhookResponse struct {
	Status    string    `json:"status"`
	Message   string    `json:"message,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// Helper function for min operation
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}