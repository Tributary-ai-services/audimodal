package box

import (
	"context"
	"sync"
	"time"
)

// Box API response structures

// BoxItem represents an item (file or folder) in Box
type BoxItem struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Type       string            `json:"type"` // "file" or "folder"
	Size       int64             `json:"size"`
	CreatedAt  string            `json:"created_at"`
	ModifiedAt string            `json:"modified_at"`
	TrashedAt  string            `json:"trashed_at,omitempty"`
	SequenceID string            `json:"sequence_id"`
	ETag       string            `json:"etag"`
	PathCollection *PathCollection `json:"path_collection,omitempty"`
	SharedLink *SharedLink       `json:"shared_link,omitempty"`
}

// BoxFile represents a file in Box with additional file-specific fields
type BoxFile struct {
	ID                   string            `json:"id"`
	Name                 string            `json:"name"`
	Size                 int64             `json:"size"`
	CreatedAt            string            `json:"created_at"`
	ModifiedAt           string            `json:"modified_at"`
	ContentCreatedAt     string            `json:"content_created_at"`
	ContentModifiedAt    string            `json:"content_modified_at"`
	TrashedAt            string            `json:"trashed_at,omitempty"`
	SequenceID           string            `json:"sequence_id"`
	ETag                 string            `json:"etag"`
	SHA1                 string            `json:"sha1"`
	FileVersion          *FileVersion      `json:"file_version,omitempty"`
	PathCollection       *PathCollection   `json:"path_collection,omitempty"`
	SharedLink           *SharedLink       `json:"shared_link,omitempty"`
	Parent               *BoxFolder        `json:"parent,omitempty"`
	VersionNumber        string            `json:"version_number,omitempty"`
	CommentCount         int               `json:"comment_count,omitempty"`
	Permissions          *Permissions      `json:"permissions,omitempty"`
	Tags                 []string          `json:"tags,omitempty"`
	Lock                 *Lock             `json:"lock,omitempty"`
	Extension            string            `json:"extension,omitempty"`
	IsPackage            bool              `json:"is_package,omitempty"`
	ExpiringEmbedLink    *ExpiringEmbedLink `json:"expiring_embed_link,omitempty"`
	Watermark            *Watermark        `json:"watermark_info,omitempty"`
	IsAccessibleViaSharedLink bool         `json:"is_accessible_via_shared_link,omitempty"`
	AllowedInviteeRoles  []string          `json:"allowed_invitee_roles,omitempty"`
	IsExternallyOwned    bool              `json:"is_externally_owned,omitempty"`
	HasCollaborations    bool              `json:"has_collaborations,omitempty"`
	Metadata             interface{}       `json:"metadata,omitempty"`
	Representations      *Representations  `json:"representations,omitempty"`
}

// BoxFolder represents a folder in Box
type BoxFolder struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	CreatedAt         string            `json:"created_at"`
	ModifiedAt        string            `json:"modified_at"`
	TrashedAt         string            `json:"trashed_at,omitempty"`
	SequenceID        string            `json:"sequence_id"`
	ETag              string            `json:"etag"`
	Description       string            `json:"description"`
	Size              int64             `json:"size"`
	PathCollection    *PathCollection   `json:"path_collection,omitempty"`
	SharedLink        *SharedLink       `json:"shared_link,omitempty"`
	FolderUploadEmail *FolderUploadEmail `json:"folder_upload_email,omitempty"`
	Parent            *BoxFolder        `json:"parent,omitempty"`
	ItemStatus        string            `json:"item_status"`
	ItemCollection    *ItemCollection   `json:"item_collection,omitempty"`
	SyncState         string            `json:"sync_state,omitempty"`
	HasCollaborations bool              `json:"has_collaborations,omitempty"`
	Permissions       *Permissions      `json:"permissions,omitempty"`
	Tags              []string          `json:"tags,omitempty"`
	CanNonOwnersInvite bool            `json:"can_non_owners_invite,omitempty"`
	IsExternallyOwned bool              `json:"is_externally_owned,omitempty"`
	Metadata          interface{}       `json:"metadata,omitempty"`
	IsAccessibleViaSharedLink bool     `json:"is_accessible_via_shared_link,omitempty"`
	AllowedInviteeRoles []string        `json:"allowed_invitee_roles,omitempty"`
	AllowedSharedLinkAccessLevels []string `json:"allowed_shared_link_access_levels,omitempty"`
	Watermark         *Watermark        `json:"watermark_info,omitempty"`
	IsCollaborationRestrictedToEnterprise bool `json:"is_collaboration_restricted_to_enterprise,omitempty"`
	Classification    *Classification   `json:"classification,omitempty"`
}

// PathCollection represents the path to an item
type PathCollection struct {
	TotalCount int               `json:"total_count"`
	Entries    []PathCollectionEntry `json:"entries"`
}

// PathCollectionEntry represents an entry in the path collection
type PathCollectionEntry struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	SequenceID string `json:"sequence_id"`
	ETag       string `json:"etag"`
	Name       string `json:"name"`
}

// SharedLink represents a shared link for an item
type SharedLink struct {
	URL                      string      `json:"url"`
	DownloadURL              string      `json:"download_url"`
	VanityURL                string      `json:"vanity_url"`
	VanityName               string      `json:"vanity_name"`
	Access                   string      `json:"access"`
	EffectiveAccess          string      `json:"effective_access"`
	EffectivePermission      string      `json:"effective_permission"`
	UnsharedAt               string      `json:"unshared_at"`
	IsPasswordEnabled        bool        `json:"is_password_enabled"`
	Permissions              *SharedLinkPermissions `json:"permissions"`
	DownloadCount            int         `json:"download_count"`
	PreviewCount             int         `json:"preview_count"`
}

// SharedLinkPermissions represents permissions for a shared link
type SharedLinkPermissions struct {
	CanDownload bool `json:"can_download"`
	CanPreview  bool `json:"can_preview"`
	CanEdit     bool `json:"can_edit"`
}

// FileVersion represents a version of a file
type FileVersion struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	SHA1       string `json:"sha1"`
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	CreatedAt  string `json:"created_at"`
	ModifiedAt string `json:"modified_at"`
	ModifiedBy *User  `json:"modified_by"`
	TrashedAt  string `json:"trashed_at,omitempty"`
	TrashedBy  *User  `json:"trashed_by,omitempty"`
	RestoredAt string `json:"restored_at,omitempty"`
	RestoredBy *User  `json:"restored_by,omitempty"`
	PurgedAt   string `json:"purged_at,omitempty"`
	UploaderDisplayName string `json:"uploader_display_name,omitempty"`
	VersionNumber string `json:"version_number,omitempty"`
}

// User represents a Box user
type User struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	Name      string `json:"name"`
	Login     string `json:"login"`
	CreatedAt string `json:"created_at"`
	ModifiedAt string `json:"modified_at"`
	Language  string `json:"language"`
	Timezone  string `json:"timezone"`
	SpaceAmount int64 `json:"space_amount"`
	SpaceUsed   int64 `json:"space_used"`
	MaxUploadSize int64 `json:"max_upload_size"`
	Status    string `json:"status"`
	JobTitle  string `json:"job_title"`
	Phone     string `json:"phone"`
	Address   string `json:"address"`
	AvatarURL string `json:"avatar_url"`
	Role      string `json:"role"`
	TrackingCodes []TrackingCode `json:"tracking_codes"`
	CanSeeManagedUsers bool `json:"can_see_managed_users"`
	IsSyncEnabled      bool `json:"is_sync_enabled"`
	IsExternalCollabRestricted bool `json:"is_external_collab_restricted"`
	IsExemptFromDeviceLimits   bool `json:"is_exempt_from_device_limits"`
	IsExemptFromLoginVerification bool `json:"is_exempt_from_login_verification"`
	Enterprise *Enterprise `json:"enterprise"`
	MyTags     []string    `json:"my_tags"`
	Hostname   string      `json:"hostname"`
	IsPlatformAccessOnly bool `json:"is_platform_access_only"`
	ExternalAppUserId    string `json:"external_app_user_id"`
}

// TrackingCode represents a tracking code for a user
type TrackingCode struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Enterprise represents an enterprise in Box
type Enterprise struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Permissions represents permissions for an item
type Permissions struct {
	CanDownload                bool `json:"can_download"`
	CanPreview                 bool `json:"can_preview"`
	CanUpload                  bool `json:"can_upload"`
	CanComment                 bool `json:"can_comment"`
	CanRename                  bool `json:"can_rename"`
	CanDelete                  bool `json:"can_delete"`
	CanShare                   bool `json:"can_share"`
	CanSetShareAccess          bool `json:"can_set_share_access"`
	CanInviteCollaborator      bool `json:"can_invite_collaborator"`
	CanAnnotate                bool `json:"can_annotate"`
	CanViewAnnotationsAll      bool `json:"can_view_annotations_all"`
	CanViewAnnotationsSelf     bool `json:"can_view_annotations_self"`
	CanCreateAnnotations       bool `json:"can_create_annotations"`
	CanDeleteAnnotations       bool `json:"can_delete_annotations"`
	CanEditAnnotations         bool `json:"can_edit_annotations"`
}

// Lock represents a lock on a file
type Lock struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	CreatedBy  *User  `json:"created_by"`
	CreatedAt  string `json:"created_at"`
	ExpiresAt  string `json:"expires_at"`
	IsDownloadPrevented bool `json:"is_download_prevented"`
}

// ExpiringEmbedLink represents an expiring embed link
type ExpiringEmbedLink struct {
	URL       string `json:"url"`
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
}

// Watermark represents watermark information
type Watermark struct {
	IsWatermarked bool   `json:"is_watermarked"`
	CreatedAt     string `json:"created_at"`
	ModifiedAt    string `json:"modified_at"`
}

// Representations represents file representations
type Representations struct {
	Entries []Representation `json:"entries"`
}

// Representation represents a file representation
type Representation struct {
	Content     RepresentationContent `json:"content"`
	Info        RepresentationInfo    `json:"info"`
	Properties  RepresentationProperties `json:"properties"`
	Representation string              `json:"representation"`
	Status      RepresentationStatus  `json:"status"`
}

// RepresentationContent represents representation content
type RepresentationContent struct {
	URLTemplate string `json:"url_template"`
}

// RepresentationInfo represents representation info
type RepresentationInfo struct {
	URL string `json:"url"`
}

// RepresentationProperties represents representation properties
type RepresentationProperties struct {
	Dimensions string `json:"dimensions"`
	Paged      bool   `json:"paged"`
	Thumb      bool   `json:"thumb"`
}

// RepresentationStatus represents representation status
type RepresentationStatus struct {
	State string `json:"state"`
}

// FolderUploadEmail represents folder upload email
type FolderUploadEmail struct {
	Access string `json:"access"`
	Email  string `json:"email"`
}

// ItemCollection represents a collection of items
type ItemCollection struct {
	TotalCount int       `json:"total_count"`
	Entries    []BoxItem `json:"entries"`
	Offset     int       `json:"offset"`
	Limit      int       `json:"limit"`
	Order      []Order   `json:"order"`
}

// Order represents sorting order
type Order struct {
	By        string `json:"by"`
	Direction string `json:"direction"`
}

// Classification represents content classification
type Classification struct {
	Name       string                 `json:"name"`
	Definition string                 `json:"definition"`
	Color      string                 `json:"color"`
	Instance   ClassificationInstance `json:"instance"`
}

// ClassificationInstance represents classification instance
type ClassificationInstance struct {
	ClassificationTemplate *ClassificationTemplate `json:"classificationType"`
	ClassificationName     string                  `json:"Box__Security__Classification__Key"`
}

// ClassificationTemplate represents classification template
type ClassificationTemplate struct {
	TemplateKey    string `json:"templateKey"`
	Scope          string `json:"scope"`
	DisplayName    string `json:"displayName"`
	Hidden         bool   `json:"hidden"`
	CopyInstanceOnItemCopy bool `json:"copyInstanceOnItemCopy"`
}

// API response structures

// BoxFolderItemsResponse represents the response from listing folder items
type BoxFolderItemsResponse struct {
	TotalCount int       `json:"total_count"`
	Entries    []BoxItem `json:"entries"`
	Offset     int       `json:"offset"`
	Limit      int       `json:"limit"`
	Order      []Order   `json:"order"`
}

// BoxEventsResponse represents the response from the events API
type BoxEventsResponse struct {
	ChunkSize    int        `json:"chunk_size"`
	NextStreamPosition string `json:"next_stream_position"`
	Entries      []BoxEvent `json:"entries"`
}

// BoxEvent represents an event from Box
type BoxEvent struct {
	Type           string      `json:"type"`
	EventID        string      `json:"event_id"`
	CreatedBy      *User       `json:"created_by"`
	CreatedAt      string      `json:"created_at"`
	RecordedAt     string      `json:"recorded_at"`
	EventType      string      `json:"event_type"`
	SessionID      string      `json:"session_id"`
	Source         interface{} `json:"source"`
	AdditionalDetails map[string]interface{} `json:"additional_details"`
}

// BoxErrorResponse represents an error response from Box API
type BoxErrorResponse struct {
	Type        string    `json:"type"`
	Status      int       `json:"status"`
	Code        string    `json:"code"`
	Message     string    `json:"message"`
	ContextInfo BoxErrorContextInfo `json:"context_info"`
	HelpURL     string    `json:"help_url"`
	RequestID   string    `json:"request_id"`
}

// BoxErrorContextInfo represents additional error context
type BoxErrorContextInfo struct {
	Conflicts []BoxErrorConflict `json:"conflicts"`
}

// BoxErrorConflict represents a conflict in error response
type BoxErrorConflict struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Connector-specific types

// SyncState tracks the synchronization state
type SyncState struct {
	IsRunning        bool          `json:"is_running"`
	LastSyncStart    time.Time     `json:"last_sync_start"`
	LastSyncEnd      time.Time     `json:"last_sync_end"`
	LastSyncDuration time.Duration `json:"last_sync_duration"`
	TotalFiles       int64         `json:"total_files"`
	ChangedFiles     int64         `json:"changed_files"`
	ErrorCount       int64         `json:"error_count"`
	LastError        string        `json:"last_error,omitempty"`
}

// ConnectorMetrics tracks connector performance metrics
type ConnectorMetrics struct {
	LastConnectionTime time.Time     `json:"last_connection_time"`
	FilesListed        int64         `json:"files_listed"`
	FilesRetrieved     int64         `json:"files_retrieved"`
	FilesDownloaded    int64         `json:"files_downloaded"`
	BytesDownloaded    int64         `json:"bytes_downloaded"`
	SyncCount          int64         `json:"sync_count"`
	LastSyncTime       time.Time     `json:"last_sync_time"`
	LastSyncDuration   time.Duration `json:"last_sync_duration"`
	LastListTime       time.Time     `json:"last_list_time"`
	ErrorCount         int64         `json:"error_count"`
	LastError          string        `json:"last_error,omitempty"`
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

// BoxWebhookConfig represents webhook configuration for Box
type BoxWebhookConfig struct {
	WebhookID    string            `yaml:"webhook_id"`
	WebhookURL   string            `yaml:"webhook_url"`
	WebhookKey   string            `yaml:"webhook_key"`
	Events       []string          `yaml:"events"`
	Triggers     []string          `yaml:"triggers"`
	Address      string            `yaml:"address"`
	CreatedBy    *User             `yaml:"created_by,omitempty"`
	CreatedAt    string            `yaml:"created_at,omitempty"`
	Target       interface{}       `yaml:"target,omitempty"`
}

// BoxEnterpriseConfig represents enterprise-specific configuration
type BoxEnterpriseConfig struct {
	EnterpriseID        string `yaml:"enterprise_id"`
	AppAuth             bool   `yaml:"app_auth"`
	JWTAuth             bool   `yaml:"jwt_auth"`
	ServiceAccountAuth  bool   `yaml:"service_account_auth"`
	PublicKeyID         string `yaml:"public_key_id"`
	PrivateKey          string `yaml:"private_key"`
	PrivateKeyPassword  string `yaml:"private_key_password"`
	ClientID            string `yaml:"client_id"`
	ClientSecret        string `yaml:"client_secret"`
	JWTAssertionAudience string `yaml:"jwt_assertion_audience"`
}

// Helper function for min operation
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}