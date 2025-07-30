package onedrive

import (
	"context"
	"sync"
	"time"
)

// OneDrive API request/response structures

// DriveItem represents a file, folder, or other item in OneDrive
type DriveItem struct {
	ID               string                 `json:"id"`
	Name             string                 `json:"name"`
	CreatedDateTime  string                 `json:"createdDateTime"`
	LastModifiedDateTime string             `json:"lastModifiedDateTime"`
	Size             int64                  `json:"size"`
	WebURL           string                 `json:"webUrl"`
	DownloadURL      string                 `json:"@microsoft.graph.downloadUrl,omitempty"`
	File             *FileMetadata          `json:"file,omitempty"`
	Folder           *FolderMetadata        `json:"folder,omitempty"`
	Package          *PackageMetadata       `json:"package,omitempty"`
	Deleted          *DeletedMetadata       `json:"deleted,omitempty"`
	ParentReference  *ItemReference         `json:"parentReference,omitempty"`
	FileSystemInfo   *FileSystemInfo        `json:"fileSystemInfo,omitempty"`
	Image            *ImageMetadata         `json:"image,omitempty"`
	Photo            *PhotoMetadata         `json:"photo,omitempty"`
	Audio            *AudioMetadata         `json:"audio,omitempty"`
	Video            *VideoMetadata         `json:"video,omitempty"`
	Location         *GeoCoordinates        `json:"location,omitempty"`
	Malware          *MalwareMetadata       `json:"malware,omitempty"`
	Root             *RootMetadata          `json:"root,omitempty"`
	SearchResult     *SearchResultMetadata  `json:"searchResult,omitempty"`
	Shared           *SharedMetadata        `json:"shared,omitempty"`
	SharepointIds    *SharepointIds         `json:"sharepointIds,omitempty"`
	SpecialFolder    *SpecialFolderMetadata `json:"specialFolder,omitempty"`
	RemoteItem       *RemoteItemMetadata    `json:"remoteItem,omitempty"`
	PublicationFacet *PublicationFacet      `json:"publication,omitempty"`
	CTag             string                 `json:"cTag,omitempty"`
	ETag             string                 `json:"eTag,omitempty"`
}

// FileMetadata contains file-specific metadata
type FileMetadata struct {
	MimeType string                `json:"mimeType"`
	Hashes   *HashesMetadata       `json:"hashes,omitempty"`
}

// FolderMetadata contains folder-specific metadata
type FolderMetadata struct {
	ChildCount int32                 `json:"childCount"`
	View       *FolderViewMetadata   `json:"view,omitempty"`
}

// PackageMetadata contains package-specific metadata
type PackageMetadata struct {
	Type string `json:"type"`
}

// DeletedMetadata contains information about deleted items
type DeletedMetadata struct {
	State string `json:"state"`
}

// ItemReference represents a reference to another drive item
type ItemReference struct {
	DriveID   string `json:"driveId,omitempty"`
	DriveType string `json:"driveType,omitempty"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Path      string `json:"path,omitempty"`
	ShareID   string `json:"shareId,omitempty"`
	SiteID    string `json:"siteId,omitempty"`
}

// FileSystemInfo contains file system information
type FileSystemInfo struct {
	CreatedDateTime      string `json:"createdDateTime"`
	LastAccessedDateTime string `json:"lastAccessedDateTime"`
	LastModifiedDateTime string `json:"lastModifiedDateTime"`
}

// HashesMetadata contains file hashes
type HashesMetadata struct {
	CRC32Hash     string `json:"crc32Hash,omitempty"`
	SHA1Hash      string `json:"sha1Hash,omitempty"`
	SHA256Hash    string `json:"sha256Hash,omitempty"`
	QuickXorHash  string `json:"quickXorHash,omitempty"`
}

// FolderViewMetadata contains folder view information
type FolderViewMetadata struct {
	SortBy    string `json:"sortBy"`
	SortOrder string `json:"sortOrder"`
	ViewType  string `json:"viewType"`
}

// ImageMetadata contains image-specific metadata
type ImageMetadata struct {
	Width  int32 `json:"width"`
	Height int32 `json:"height"`
}

// PhotoMetadata contains photo-specific metadata
type PhotoMetadata struct {
	CameraMake         string    `json:"cameraMake,omitempty"`
	CameraModel        string    `json:"cameraModel,omitempty"`
	ExposureDenominator float64  `json:"exposureDenominator,omitempty"`
	ExposureNumerator  float64   `json:"exposureNumerator,omitempty"`
	FocalLength        float64   `json:"focalLength,omitempty"`
	FNumber            float64   `json:"fNumber,omitempty"`
	Iso                int32     `json:"iso,omitempty"`
	Orientation        int32     `json:"orientation,omitempty"`
	TakenDateTime      string    `json:"takenDateTime,omitempty"`
}

// AudioMetadata contains audio-specific metadata
type AudioMetadata struct {
	Album             string `json:"album,omitempty"`
	AlbumArtist       string `json:"albumArtist,omitempty"`
	Artist            string `json:"artist,omitempty"`
	Bitrate           int64  `json:"bitrate,omitempty"`
	Composers         string `json:"composers,omitempty"`
	Copyright         string `json:"copyright,omitempty"`
	Disc              int32  `json:"disc,omitempty"`
	DiscCount         int32  `json:"discCount,omitempty"`
	Duration          int64  `json:"duration,omitempty"`
	Genre             string `json:"genre,omitempty"`
	HasDrm            bool   `json:"hasDrm,omitempty"`
	IsVariableBitrate bool   `json:"isVariableBitrate,omitempty"`
	Title             string `json:"title,omitempty"`
	Track             int32  `json:"track,omitempty"`
	TrackCount        int32  `json:"trackCount,omitempty"`
	Year              int32  `json:"year,omitempty"`
}

// VideoMetadata contains video-specific metadata
type VideoMetadata struct {
	AudioBitsPerSample int32 `json:"audioBitsPerSample,omitempty"`
	AudioChannels      int32 `json:"audioChannels,omitempty"`
	AudioFormat        string `json:"audioFormat,omitempty"`
	AudioSamplesPerSecond int32 `json:"audioSamplesPerSecond,omitempty"`
	Bitrate            int32 `json:"bitrate,omitempty"`
	Duration           int64 `json:"duration,omitempty"`
	FourCC             string `json:"fourCC,omitempty"`
	FrameRate          float64 `json:"frameRate,omitempty"`
	Height             int32 `json:"height,omitempty"`
	Width              int32 `json:"width,omitempty"`
}

// GeoCoordinates represents geographical coordinates
type GeoCoordinates struct {
	Altitude  float64 `json:"altitude,omitempty"`
	Latitude  float64 `json:"latitude,omitempty"`
	Longitude float64 `json:"longitude,omitempty"`
}

// MalwareMetadata contains malware scan information
type MalwareMetadata struct {
	Description string `json:"description,omitempty"`
}

// RootMetadata indicates an item is the root of a drive
type RootMetadata struct {
}

// SearchResultMetadata contains search result information
type SearchResultMetadata struct {
	OnClickTelemetryUrl string `json:"onClickTelemetryUrl,omitempty"`
}

// SharedMetadata contains sharing information
type SharedMetadata struct {
	Owner          *IdentitySet `json:"owner,omitempty"`
	Scope          string       `json:"scope,omitempty"`
	SharedBy       *IdentitySet `json:"sharedBy,omitempty"`
	SharedDateTime string       `json:"sharedDateTime,omitempty"`
}

// SharepointIds contains SharePoint-specific identifiers
type SharepointIds struct {
	ListID           string `json:"listId,omitempty"`
	ListItemID       string `json:"listItemId,omitempty"`
	ListItemUniqueID string `json:"listItemUniqueId,omitempty"`
	SiteID           string `json:"siteId,omitempty"`
	SiteURL          string `json:"siteUrl,omitempty"`
	TenantID         string `json:"tenantId,omitempty"`
	WebID            string `json:"webId,omitempty"`
}

// SpecialFolderMetadata contains special folder information
type SpecialFolderMetadata struct {
	Name string `json:"name"`
}

// RemoteItemMetadata contains information about items in remote drives
type RemoteItemMetadata struct {
	ID              string          `json:"id,omitempty"`
	CreatedBy       *IdentitySet    `json:"createdBy,omitempty"`
	CreatedDateTime string          `json:"createdDateTime,omitempty"`
	File            *FileMetadata   `json:"file,omitempty"`
	FileSystemInfo  *FileSystemInfo `json:"fileSystemInfo,omitempty"`
	Folder          *FolderMetadata `json:"folder,omitempty"`
	LastModifiedBy  *IdentitySet    `json:"lastModifiedBy,omitempty"`
	LastModifiedDateTime string     `json:"lastModifiedDateTime,omitempty"`
	Name            string          `json:"name,omitempty"`
	Package         *PackageMetadata `json:"package,omitempty"`
	ParentReference *ItemReference  `json:"parentReference,omitempty"`
	Shared          *SharedMetadata `json:"shared,omitempty"`
	SharepointIds   *SharepointIds  `json:"sharepointIds,omitempty"`
	Size            int64           `json:"size,omitempty"`
	SpecialFolder   *SpecialFolderMetadata `json:"specialFolder,omitempty"`
	WebDavURL       string          `json:"webDavUrl,omitempty"`
	WebURL          string          `json:"webUrl,omitempty"`
}

// PublicationFacet contains publication information
type PublicationFacet struct {
	Level         string `json:"level,omitempty"`
	VersionID     string `json:"versionId,omitempty"`
	CheckedOutBy  *IdentitySet `json:"checkedOutBy,omitempty"`
}

// IdentitySet represents a collection of identities
type IdentitySet struct {
	Application *Identity `json:"application,omitempty"`
	Device      *Identity `json:"device,omitempty"`
	User        *Identity `json:"user,omitempty"`
}

// Identity represents an identity of an actor
type Identity struct {
	DisplayName string `json:"displayName,omitempty"`
	ID          string `json:"id,omitempty"`
	ThumbnailSet *ThumbnailSet `json:"thumbnails,omitempty"`
}

// ThumbnailSet represents a collection of thumbnail resources
type ThumbnailSet struct {
	ID     string              `json:"id,omitempty"`
	Large  *ThumbnailMetadata  `json:"large,omitempty"`
	Medium *ThumbnailMetadata  `json:"medium,omitempty"`
	Small  *ThumbnailMetadata  `json:"small,omitempty"`
	Source *ThumbnailMetadata  `json:"source,omitempty"`
}

// ThumbnailMetadata represents a thumbnail resource
type ThumbnailMetadata struct {
	Content string `json:"content,omitempty"`
	Height  int32  `json:"height,omitempty"`
	SourceItemID string `json:"sourceItemId,omitempty"`
	URL     string `json:"url,omitempty"`
	Width   int32  `json:"width,omitempty"`
}

// API Request/Response structures

// DriveItemCollection represents a collection of drive items
type DriveItemCollection struct {
	ODataContext  string      `json:"@odata.context,omitempty"`
	ODataNextLink string      `json:"@odata.nextLink,omitempty"`
	Value         []DriveItem `json:"value"`
}

// Drive represents a OneDrive drive
type Drive struct {
	ID          string          `json:"id"`
	CreatedBy   *IdentitySet    `json:"createdBy,omitempty"`
	CreatedDateTime string      `json:"createdDateTime,omitempty"`
	Description string          `json:"description,omitempty"`
	DriveType   string          `json:"driveType"`
	LastModifiedBy *IdentitySet `json:"lastModifiedBy,omitempty"`
	LastModifiedDateTime string `json:"lastModifiedDateTime,omitempty"`
	Name        string          `json:"name,omitempty"`
	Owner       *IdentitySet    `json:"owner,omitempty"`
	Quota       *Quota          `json:"quota,omitempty"`
	SharepointIds *SharepointIds `json:"sharepointIds,omitempty"`
	System      *SystemFacet    `json:"system,omitempty"`
	WebURL      string          `json:"webUrl,omitempty"`
}

// Quota represents drive quota information
type Quota struct {
	Deleted   int64  `json:"deleted"`
	Remaining int64  `json:"remaining"`
	State     string `json:"state,omitempty"`
	Total     int64  `json:"total"`
	Used      int64  `json:"used"`
}

// SystemFacet indicates a system-managed item
type SystemFacet struct {
}

// DriveCollection represents a collection of drives
type DriveCollection struct {
	ODataContext  string  `json:"@odata.context,omitempty"`
	ODataNextLink string  `json:"@odata.nextLink,omitempty"`
	Value         []Drive `json:"value"`
}

// User represents a Microsoft Graph user
type User struct {
	ID                string   `json:"id"`
	BusinessPhones    []string `json:"businessPhones,omitempty"`
	DisplayName       string   `json:"displayName,omitempty"`
	GivenName         string   `json:"givenName,omitempty"`
	JobTitle          string   `json:"jobTitle,omitempty"`
	Mail              string   `json:"mail,omitempty"`
	MobilePhone       string   `json:"mobilePhone,omitempty"`
	OfficeLocation    string   `json:"officeLocation,omitempty"`
	PreferredLanguage string   `json:"preferredLanguage,omitempty"`
	Surname           string   `json:"surname,omitempty"`
	UserPrincipalName string   `json:"userPrincipalName,omitempty"`
}

// DeltaLink represents a delta link for tracking changes
type DeltaLink struct {
	ODataContext   string      `json:"@odata.context,omitempty"`
	ODataNextLink  string      `json:"@odata.nextLink,omitempty"`
	ODataDeltaLink string      `json:"@odata.deltaLink,omitempty"`
	Value          []DriveItem `json:"value"`
}

// UploadSession represents an upload session for large files
type UploadSession struct {
	UploadURL          string            `json:"uploadUrl"`
	ExpirationDateTime string            `json:"expirationDateTime"`
	NextExpectedRanges []string          `json:"nextExpectedRanges,omitempty"`
}

// Permission represents sharing permissions
type Permission struct {
	ID                    string            `json:"id"`
	GrantedTo             *IdentitySet      `json:"grantedTo,omitempty"`
	GrantedToIdentities   []IdentitySet     `json:"grantedToIdentities,omitempty"`
	HasPassword           bool              `json:"hasPassword,omitempty"`
	InheritedFrom         *ItemReference    `json:"inheritedFrom,omitempty"`
	Invitation            *SharingInvitation `json:"invitation,omitempty"`
	Link                  *SharingLink      `json:"link,omitempty"`
	Roles                 []string          `json:"roles"`
	ShareID               string            `json:"shareId,omitempty"`
}

// SharingInvitation represents a sharing invitation
type SharingInvitation struct {
	Email                 string       `json:"email,omitempty"`
	InvitedBy             *IdentitySet `json:"invitedBy,omitempty"`
	RedeemedBy            string       `json:"redeemedBy,omitempty"`
	SignInRequired        bool         `json:"signInRequired,omitempty"`
}

// SharingLink represents a sharing link
type SharingLink struct {
	Application    *Identity `json:"application,omitempty"`
	PreventsDownload bool    `json:"preventsDownload,omitempty"`
	Scope          string    `json:"scope,omitempty"`
	Type           string    `json:"type,omitempty"`
	WebHTML        string    `json:"webHtml,omitempty"`
	WebURL         string    `json:"webUrl,omitempty"`
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
	DeltaToken       string        `json:"delta_token,omitempty"` // OneDrive delta token for incremental sync
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
	
	// OneDrive-specific metrics
	APICallsCount      int64         `json:"api_calls_count"`
	ThrottlingHits     int64         `json:"throttling_hits"`
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

// Webhook and subscription structures

// Subscription represents a Microsoft Graph subscription
type Subscription struct {
	ID                         string            `json:"id,omitempty"`
	Resource                   string            `json:"resource"`
	ChangeType                 string            `json:"changeType"`
	ClientState                string            `json:"clientState,omitempty"`
	NotificationURL            string            `json:"notificationUrl"`
	ExpirationDateTime         string            `json:"expirationDateTime"`
	ApplicationID              string            `json:"applicationId,omitempty"`
	CreatorID                  string            `json:"creatorId,omitempty"`
	IncludeResourceData        bool              `json:"includeResourceData,omitempty"`
	LifecycleNotificationURL   string            `json:"lifecycleNotificationUrl,omitempty"`
	EncryptionCertificate      string            `json:"encryptionCertificate,omitempty"`
	EncryptionCertificateID    string            `json:"encryptionCertificateId,omitempty"`
	LatestSupportedTlsVersion  string            `json:"latestSupportedTlsVersion,omitempty"`
}

// ChangeNotification represents a change notification from Microsoft Graph
type ChangeNotification struct {
	SubscriptionID            string                 `json:"subscriptionId"`
	SubscriptionExpirationDateTime string            `json:"subscriptionExpirationDateTime"`
	ChangeType                string                 `json:"changeType"`
	Resource                  string                 `json:"resource"`
	ResourceData              map[string]interface{} `json:"resourceData,omitempty"`
	ClientState               string                 `json:"clientState,omitempty"`
	TenantID                  string                 `json:"tenantId,omitempty"`
}

// NotificationCollection represents a collection of change notifications
type NotificationCollection struct {
	ValidationTokens []string             `json:"validationTokens,omitempty"`
	Value            []ChangeNotification `json:"value"`
}

// Configuration structures for enterprise features

// EnterpriseConfig represents enterprise-specific configuration
type EnterpriseConfig struct {
	TenantID              string            `yaml:"tenant_id"`
	EnableTenantFeatures  bool              `yaml:"enable_tenant_features"`
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
	EnableDLP              bool              `yaml:"enable_dlp"`
	DLPPolicies            []string          `yaml:"dlp_policies"`
}

// ContentManagementConfig represents content management settings
type ContentManagementConfig struct {
	EnableVersioning       bool              `yaml:"enable_versioning"`
	MaxVersions            int               `yaml:"max_versions"`
	EnableComments         bool              `yaml:"enable_comments"`
	EnableCoAuthoring      bool              `yaml:"enable_coauthoring"`
	PreviewGeneration      bool              `yaml:"preview_generation"`
	ThumbnailGeneration    bool              `yaml:"thumbnail_generation"`
	EnableOfficeIntegration bool             `yaml:"enable_office_integration"`
}

// SecurityConfig represents security configuration
type SecurityConfig struct {
	EnableEncryption       bool              `yaml:"enable_encryption"`
	EncryptionAlgorithm    string            `yaml:"encryption_algorithm"`
	EnableConditionalAccess bool             `yaml:"enable_conditional_access"`
	AccessControls         AccessControlConfig `yaml:"access_controls"`
	AuditSettings          AuditConfig       `yaml:"audit_settings"`
	ThreatProtection       ThreatProtectionConfig `yaml:"threat_protection"`
}

// AccessControlConfig represents access control settings
type AccessControlConfig struct {
	EnableDeviceManagement bool              `yaml:"enable_device_management"`
	AllowedDevices         []string          `yaml:"allowed_devices"`
	EnableMFA              bool              `yaml:"enable_mfa"`
	SessionTimeout         time.Duration     `yaml:"session_timeout"`
	IPRestrictions         []string          `yaml:"ip_restrictions"`
}

// AuditConfig represents audit logging configuration
type AuditConfig struct {
	EnableFileAccess       bool              `yaml:"enable_file_access"`
	EnableSharingActivity  bool              `yaml:"enable_sharing_activity"`
	EnableAdminActivity    bool              `yaml:"enable_admin_activity"`
	RetentionPeriod        time.Duration     `yaml:"retention_period"`
	ExportFormat           string            `yaml:"export_format"`
	LogLevel               string            `yaml:"log_level"`
}

// ComplianceConfig represents compliance settings
type ComplianceConfig struct {
	EnableRecordsManagement bool             `yaml:"enable_records_management"`
	EnableeDiscovery       bool              `yaml:"enable_ediscovery"`
	ComplianceCenter       string            `yaml:"compliance_center"`
	PolicyEnforcement      bool              `yaml:"policy_enforcement"`
}

// ThreatProtectionConfig represents threat protection settings
type ThreatProtectionConfig struct {
	EnableAdvancedThreatProtection bool     `yaml:"enable_advanced_threat_protection"`
	EnableSafeDocs                 bool     `yaml:"enable_safe_docs"`
	EnableSafeAttachments          bool     `yaml:"enable_safe_attachments"`
	QuarantineActions              []string `yaml:"quarantine_actions"`
}

// Helper function for min operation
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}