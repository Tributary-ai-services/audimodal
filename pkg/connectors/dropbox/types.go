package dropbox

import (
	"context"
	"sync"
	"time"
)

// Dropbox API request/response structures

// DropboxEntry represents a file, folder, or deleted item in Dropbox
type DropboxEntry struct {
	Tag                          string                     `json:".tag"`
	Name                         string                     `json:"name"`
	ID                           string                     `json:"id"`
	ClientModified               string                     `json:"client_modified,omitempty"`
	ServerModified               string                     `json:"server_modified,omitempty"`
	Rev                          string                     `json:"rev,omitempty"`
	Size                         int64                      `json:"size,omitempty"`
	PathLower                    string                     `json:"path_lower,omitempty"`
	PathDisplay                  string                     `json:"path_display,omitempty"`
	ParentSharedFolderID         string                     `json:"parent_shared_folder_id,omitempty"`
	PreviewURL                   string                     `json:"preview_url,omitempty"`
	MediaInfo                    *MediaInfo                 `json:"media_info,omitempty"`
	SharingInfo                  *SharingInfo               `json:"sharing_info,omitempty"`
	PropertyGroups               []PropertyGroup            `json:"property_groups,omitempty"`
	HasExplicitSharedMembers     bool                       `json:"has_explicit_shared_members,omitempty"`
	ContentHash                  string                     `json:"content_hash,omitempty"`
	FileLockInfo                 *FileLockInfo              `json:"file_lock_info,omitempty"`
	IsDownloadable               bool                       `json:"is_downloadable,omitempty"`
	ExportInfo                   *ExportInfo                `json:"export_info,omitempty"`
}

// DropboxFile represents a file in Dropbox with additional file-specific fields
type DropboxFile struct {
	DropboxEntry
	SymlinkInfo                  *SymlinkInfo               `json:"symlink_info,omitempty"`
}

// DropboxFolder represents a folder in Dropbox with additional folder-specific fields
type DropboxFolder struct {
	DropboxEntry
	SharedFolderID               string                     `json:"shared_folder_id,omitempty"`
}

// MediaInfo contains media-specific metadata
type MediaInfo struct {
	Tag       string     `json:".tag"`
	Pending   bool       `json:"pending,omitempty"`
	Metadata  *MediaMetadata `json:"metadata,omitempty"`
}

// MediaMetadata contains specific media metadata
type MediaMetadata struct {
	Tag        string      `json:".tag"`
	Photo      *PhotoMetadata `json:"photo,omitempty"`
	Video      *VideoMetadata `json:"video,omitempty"`
}

// PhotoMetadata contains photo-specific metadata
type PhotoMetadata struct {
	Dimensions  *Dimensions `json:"dimensions,omitempty"`
	Location    *GPSCoordinates `json:"location,omitempty"`
	TimeTaken   string      `json:"time_taken,omitempty"`
}

// VideoMetadata contains video-specific metadata
type VideoMetadata struct {
	Dimensions  *Dimensions `json:"dimensions,omitempty"`
	Location    *GPSCoordinates `json:"location,omitempty"`
	TimeTaken   string      `json:"time_taken,omitempty"`
	Duration    uint64      `json:"duration,omitempty"`
}

// Dimensions represents width and height
type Dimensions struct {
	Height uint64 `json:"height"`
	Width  uint64 `json:"width"`
}

// GPSCoordinates represents GPS location
type GPSCoordinates struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// SharingInfo contains sharing information for files and folders
type SharingInfo struct {
	ReadOnly                     bool   `json:"read_only"`
	ParentSharedFolderID         string `json:"parent_shared_folder_id,omitempty"`
	ModifiedBy                   string `json:"modified_by,omitempty"`
	TraverseOnly                 bool   `json:"traverse_only,omitempty"`
	NoAccess                     bool   `json:"no_access,omitempty"`
}

// PropertyGroup represents custom properties on files/folders
type PropertyGroup struct {
	TemplateID string              `json:"template_id"`
	Fields     []PropertyGroupField `json:"fields"`
}

// PropertyGroupField represents a field in a property group
type PropertyGroupField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// FileLockInfo contains information about file locks
type FileLockInfo struct {
	IsLockedForEditing bool   `json:"is_lockedfor_editing"`
	LockedBy           string `json:"locked_by,omitempty"`
}

// ExportInfo contains export information for certain file types
type ExportInfo struct {
	ExportAs    string `json:"export_as,omitempty"`
	ExportOptions []ExportOption `json:"export_options,omitempty"`
}

// ExportOption represents an export option
type ExportOption struct {
	Tag string `json:".tag"`
}

// SymlinkInfo contains information about symbolic links
type SymlinkInfo struct {
	Target string `json:"target"`
}

// API Request structures

// ListFolderRequest represents a request to list folder contents
type ListFolderRequest struct {
	Path                            string `json:"path"`
	Recursive                       bool   `json:"recursive,omitempty"`
	IncludeMediaInfo               bool   `json:"include_media_info,omitempty"`
	IncludeDeleted                 bool   `json:"include_deleted,omitempty"`
	IncludeHasExplicitSharedMembers bool   `json:"include_has_explicit_shared_members,omitempty"`
	IncludeMountedFolders          bool   `json:"include_mounted_folders,omitempty"`
	Limit                          uint32 `json:"limit,omitempty"`
	SharedLink                     *SharedLink `json:"shared_link,omitempty"`
	IncludePropertyGroups          *PropertyGroupFilter `json:"include_property_groups,omitempty"`
	IncludeNonDownloadableFiles    bool   `json:"include_non_downloadable_files,omitempty"`
}

// ListFolderContinueRequest represents a request to continue listing folder contents
type ListFolderContinueRequest struct {
	Cursor string `json:"cursor"`
}

// ListFolderResponse represents the response from list folder operations
type ListFolderResponse struct {
	Entries []DropboxEntry `json:"entries"`
	Cursor  string         `json:"cursor"`
	HasMore bool           `json:"has_more"`
}

// GetMetadataRequest represents a request to get file/folder metadata
type GetMetadataRequest struct {
	Path                            string `json:"path"`
	IncludeMediaInfo               bool   `json:"include_media_info,omitempty"`
	IncludeDeleted                 bool   `json:"include_deleted,omitempty"`
	IncludeHasExplicitSharedMembers bool   `json:"include_has_explicit_shared_members,omitempty"`
	IncludePropertyGroups          *PropertyGroupFilter `json:"include_property_groups,omitempty"`
}

// DownloadRequest represents a request to download a file
type DownloadRequest struct {
	Path string `json:"path"`
	Rev  string `json:"rev,omitempty"`
}

// SharedLink represents a shared link
type SharedLink struct {
	URL      string `json:"url"`
	Password string `json:"password,omitempty"`
}

// PropertyGroupFilter represents filters for property groups
type PropertyGroupFilter struct {
	FilterSome []string `json:"filter_some,omitempty"`
}

// Account and user structures

// DropboxAccount represents a Dropbox user account
type DropboxAccount struct {
	AccountID    string `json:"account_id"`
	Name         Name   `json:"name"`
	Email        string `json:"email"`
	EmailVerified bool  `json:"email_verified"`
	Disabled     bool   `json:"disabled"`
	Locale       string `json:"locale"`
	ReferralLink string `json:"referral_link"`
	IsPaired     bool   `json:"is_paired"`
	AccountType  AccountType `json:"account_type"`
	RootInfo     RootInfo `json:"root_info"`
	ProfilePhotoURL string `json:"profile_photo_url,omitempty"`
	Country      string `json:"country,omitempty"`
	Team         *Team  `json:"team,omitempty"`
	TeamMemberID string `json:"team_member_id,omitempty"`
}

// Name represents a user's name
type Name struct {
	GivenName       string `json:"given_name"`
	Surname         string `json:"surname"`
	FamiliarName    string `json:"familiar_name"`
	DisplayName     string `json:"display_name"`
	AbbreviatedName string `json:"abbreviated_name"`
}

// AccountType represents the type of account
type AccountType struct {
	Tag string `json:".tag"`
}

// RootInfo represents information about the root folder
type RootInfo struct {
	Tag           string `json:".tag"`
	RootNamespaceID string `json:"root_namespace_id"`
	HomeNamespaceID string `json:"home_namespace_id"`
}

// Team represents team information
type Team struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	SharingPolicies TeamSharingPolicies `json:"sharing_policies"`
	OfficeAddinPolicy OfficeAddinPolicy `json:"office_addin_policy"`
}

// TeamSharingPolicies represents team sharing policies
type TeamSharingPolicies struct {
	SharedFolderMemberPolicy    PolicyValue `json:"shared_folder_member_policy"`
	SharedFolderJoinPolicy      PolicyValue `json:"shared_folder_join_policy"`
	SharedLinkCreatePolicy      PolicyValue `json:"shared_link_create_policy"`
	SharedLinkCreateMemberPolicy PolicyValue `json:"shared_link_create_member_policy"`
}

// PolicyValue represents a policy value
type PolicyValue struct {
	Tag string `json:".tag"`
}

// OfficeAddinPolicy represents Office add-in policy
type OfficeAddinPolicy struct {
	Tag string `json:".tag"`
}

// Webhook and long polling structures

// LongpollRequest represents a request for long polling
type LongpollRequest struct {
	Cursor  string `json:"cursor"`
	Timeout uint64 `json:"timeout,omitempty"`
}

// LongpollResponse represents a response from long polling
type LongpollResponse struct {
	Changes   bool   `json:"changes"`
	Backoff   uint64 `json:"backoff,omitempty"`
}

// GetLatestCursorRequest represents a request to get the latest cursor
type GetLatestCursorRequest struct {
	Path                            string `json:"path"`
	Recursive                       bool   `json:"recursive,omitempty"`
	IncludeMediaInfo               bool   `json:"include_media_info,omitempty"`
	IncludeDeleted                 bool   `json:"include_deleted,omitempty"`
	IncludeHasExplicitSharedMembers bool   `json:"include_has_explicit_shared_members,omitempty"`
	IncludeMountedFolders          bool   `json:"include_mounted_folders,omitempty"`
	IncludeNonDownloadableFiles    bool   `json:"include_non_downloadable_files,omitempty"`
}

// GetLatestCursorResponse represents a response with the latest cursor
type GetLatestCursorResponse struct {
	Cursor string `json:"cursor"`
}

// Error structures

// DropboxError represents an error from Dropbox API
type DropboxError struct {
	ErrorSummary string          `json:"error_summary"`
	Error        DropboxErrorDetail `json:"error"`
	UserMessage  *UserMessage    `json:"user_message,omitempty"`
}

// DropboxErrorDetail contains detailed error information
type DropboxErrorDetail struct {
	Tag string `json:".tag"`
}

// UserMessage represents a user-friendly error message
type UserMessage struct {
	Text   string `json:"text"`
	Locale string `json:"locale"`
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
	Cursor           string        `json:"cursor,omitempty"` // Dropbox cursor for incremental sync
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
	
	// Dropbox-specific metrics
	APICallsCount      int64         `json:"api_calls_count"`
	RateLimitHits      int64         `json:"rate_limit_hits"`
	CacheHits          int64         `json:"cache_hits"`
	CacheMisses        int64         `json:"cache_misses"`
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

// Paper integration structures

// PaperDoc represents a Dropbox Paper document
type PaperDoc struct {
	DocID          string    `json:"doc_id"`
	Title          string    `json:"title"`
	Owner          string    `json:"owner"`
	CreatedDate    time.Time `json:"created_date"`
	LastUpdatedDate time.Time `json:"last_updated_date"`
	LastEditor     string    `json:"last_editor"`
	Status         string    `json:"status"`
	Revision       int64     `json:"revision"`
	FolderID       string    `json:"folder_id,omitempty"`
}

// PaperExportRequest represents a request to export a Paper document
type PaperExportRequest struct {
	DocID        string `json:"doc_id"`
	ExportFormat string `json:"export_format"` // html, markdown, odt, pdf, docx
}

// PaperListRequest represents a request to list Paper documents
type PaperListRequest struct {
	FilterBy   string `json:"filter_by,omitempty"`   // created_by_me, accessed_by_me, other
	SortBy     string `json:"sort_by,omitempty"`     // accessed, modified, created
	SortOrder  string `json:"sort_order,omitempty"`  // ascending, descending
	Limit      int32  `json:"limit,omitempty"`
}

// PaperListResponse represents a response from listing Paper documents
type PaperListResponse struct {
	DocIDs     []string `json:"doc_ids"`
	Cursor     string   `json:"cursor,omitempty"`
	HasMore    bool     `json:"has_more"`
}

// Business/Team structures

// TeamSpaceAllocation represents team space allocation
type TeamSpaceAllocation struct {
	Used      uint64 `json:"used"`
	Allocated uint64 `json:"allocated"`
}

// TeamMemberInfo represents information about a team member
type TeamMemberInfo struct {
	Profile       MemberProfile `json:"profile"`
	Role          AdminRole     `json:"role"`
	Status        TeamMemberStatus `json:"status"`
	JoinedOn      string        `json:"joined_on,omitempty"`
	MemberFolderID string       `json:"member_folder_id,omitempty"`
}

// MemberProfile represents a team member's profile
type MemberProfile struct {
	TeamMemberID     string `json:"team_member_id"`
	ExternalID       string `json:"external_id,omitempty"`
	AccountID        string `json:"account_id,omitempty"`
	Email            string `json:"email"`
	EmailVerified    bool   `json:"email_verified"`
	SecondaryEmails  []SecondaryEmail `json:"secondary_emails,omitempty"`
	Name             Name   `json:"name"`
	MembershipType   MembershipType `json:"membership_type"`
	Groups           []string `json:"groups,omitempty"`
	Status           MemberStatus `json:"status"`
}

// AdminRole represents an admin role
type AdminRole struct {
	Tag string `json:".tag"`
}

// TeamMemberStatus represents a team member's status
type TeamMemberStatus struct {
	Tag string `json:".tag"`
}

// SecondaryEmail represents a secondary email address
type SecondaryEmail struct {
	Email        string `json:"email"`
	IsVerified   bool   `json:"is_verified"`
	IsPrimary    bool   `json:"is_primary"`
}

// MembershipType represents membership type
type MembershipType struct {
	Tag string `json:".tag"`
}

// MemberStatus represents member status
type MemberStatus struct {
	Tag string `json:".tag"`
}

// File requests and sharing structures

// CreateSharedLinkRequest represents a request to create a shared link
type CreateSharedLinkRequest struct {
	Path      string           `json:"path"`
	Settings  *SharedLinkSettings `json:"settings,omitempty"`
}

// SharedLinkSettings represents settings for a shared link
type SharedLinkSettings struct {
	RequiredPassword string    `json:"required_password,omitempty"`
	LinkPassword     string    `json:"link_password,omitempty"`
	Expires          *time.Time `json:"expires,omitempty"`
	Audience         *LinkAudience `json:"audience,omitempty"`
	Access           *RequestedLinkAccessLevel `json:"access,omitempty"`
}

// LinkAudience represents the audience for a shared link
type LinkAudience struct {
	Tag string `json:".tag"`
}

// RequestedLinkAccessLevel represents the requested access level for a link
type RequestedLinkAccessLevel struct {
	Tag string `json:".tag"`
}

// SharedLinkMetadata represents metadata for a shared link
type SharedLinkMetadata struct {
	URL          string    `json:"url"`
	ID           string    `json:"id,omitempty"`
	Name         string    `json:"name"`
	Expires      *time.Time `json:"expires,omitempty"`
	PathLower    string    `json:"path_lower,omitempty"`
	LinkPermissions LinkPermissions `json:"link_permissions"`
	TeamMemberInfo *TeamMemberInfo `json:"team_member_info,omitempty"`
	ContentOwnerTeamInfo *Team `json:"content_owner_team_info,omitempty"`
}

// LinkPermissions represents permissions for a shared link
type LinkPermissions struct {
	CanRevoke          bool                       `json:"can_revoke"`
	ResolvedVisibility *ResolvedVisibility        `json:"resolved_visibility,omitempty"`
	RevokeFailureReason *SharedLinkAccessFailureReason `json:"revoke_failure_reason,omitempty"`
}

// ResolvedVisibility represents resolved visibility settings
type ResolvedVisibility struct {
	Tag string `json:".tag"`
}

// SharedLinkAccessFailureReason represents reasons for shared link access failure
type SharedLinkAccessFailureReason struct {
	Tag string `json:".tag"`
}

// Helper function for min operation
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// Configuration structures for enterprise features

// EnterpriseConfig represents enterprise-specific configuration
type EnterpriseConfig struct {
	TeamID              string            `yaml:"team_id"`
	BusinessID          string            `yaml:"business_id"`
	EnableTeamFeatures  bool              `yaml:"enable_team_features"`
	EnableAuditLog      bool              `yaml:"enable_audit_log"`
	DataGovernance      DataGovernanceConfig `yaml:"data_governance"`
	ContentManagement   ContentManagementConfig `yaml:"content_management"`
	SecuritySettings    SecurityConfig    `yaml:"security_settings"`
}

// DataGovernanceConfig represents data governance settings
type DataGovernanceConfig struct {
	EnableDataRetention    bool              `yaml:"enable_data_retention"`
	RetentionPeriodDays    int               `yaml:"retention_period_days"`
	EnableLegalHold        bool              `yaml:"enable_legal_hold"`
	ComplianceReporting    bool              `yaml:"compliance_reporting"`
	DataClassification     []string          `yaml:"data_classification"`
}

// ContentManagementConfig represents content management settings
type ContentManagementConfig struct {
	EnableVersioning       bool              `yaml:"enable_versioning"`
	MaxVersions            int               `yaml:"max_versions"`
	EnableComments         bool              `yaml:"enable_comments"`
	EnableWatermarking     bool              `yaml:"enable_watermarking"`
	PreviewGeneration      bool              `yaml:"preview_generation"`
	ThumbnailGeneration    bool              `yaml:"thumbnail_generation"`
}

// SecurityConfig represents security configuration
type SecurityConfig struct {
	EnableEncryption       bool              `yaml:"enable_encryption"`
	EncryptionAlgorithm    string            `yaml:"encryption_algorithm"`
	EnableDLP              bool              `yaml:"enable_dlp"`
	DLPPolicies            []string          `yaml:"dlp_policies"`
	AccessControls         AccessControlConfig `yaml:"access_controls"`
	AuditSettings          AuditConfig       `yaml:"audit_settings"`
}

// AccessControlConfig represents access control settings
type AccessControlConfig struct {
	EnableDeviceManagement bool              `yaml:"enable_device_management"`
	AllowedDevices         []string          `yaml:"allowed_devices"`
	EnableSSO              bool              `yaml:"enable_sso"`
	SSOProvider            string            `yaml:"sso_provider"`
	SessionTimeout         time.Duration     `yaml:"session_timeout"`
}

// AuditConfig represents audit logging configuration
type AuditConfig struct {
	EnableFileAccess       bool              `yaml:"enable_file_access"`
	EnableSharingActivity  bool              `yaml:"enable_sharing_activity"`
	EnableAdminActivity    bool              `yaml:"enable_admin_activity"`
	RetentionPeriod        time.Duration     `yaml:"retention_period"`
	ExportFormat           string            `yaml:"export_format"`
}