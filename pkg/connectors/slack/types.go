package slack

import "time"

// SlackChannel represents a Slack channel
type SlackChannel struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	IsChannel          bool     `json:"is_channel"`
	IsGroup            bool     `json:"is_group"`
	IsIM               bool     `json:"is_im"`
	IsMPIM             bool     `json:"is_mpim"`
	IsPrivate          bool     `json:"is_private"`
	IsArchived         bool     `json:"is_archived"`
	IsGeneral          bool     `json:"is_general"`
	IsShared           bool     `json:"is_shared"`
	IsOrgShared        bool     `json:"is_org_shared"`
	IsPendingExtShared bool     `json:"is_pending_ext_shared"`
	Creator            string   `json:"creator"`
	Created            string   `json:"created"`
	Updated            string   `json:"updated"`
	NumMembers         int      `json:"num_members"`
	Type               string   `json:"type"`
	Topic              Topic    `json:"topic"`
	Purpose            Purpose  `json:"purpose"`
	Members            []string `json:"members,omitempty"`
}

// Topic represents a channel topic
type Topic struct {
	Value   string `json:"value"`
	Creator string `json:"creator"`
	LastSet string `json:"last_set"`
}

// Purpose represents a channel purpose
type Purpose struct {
	Value   string `json:"value"`
	Creator string `json:"creator"`
	LastSet string `json:"last_set"`
}

// SlackUser represents a Slack user
type SlackUser struct {
	ID                 string      `json:"id"`
	TeamID             string      `json:"team_id"`
	Name               string      `json:"name"`
	Deleted            bool        `json:"deleted"`
	Color              string      `json:"color"`
	RealName           string      `json:"real_name"`
	TZ                 string      `json:"tz"`
	TZLabel            string      `json:"tz_label"`
	TZOffset           int         `json:"tz_offset"`
	Profile            UserProfile `json:"profile"`
	IsAdmin            bool        `json:"is_admin"`
	IsOwner            bool        `json:"is_owner"`
	IsPrimaryOwner     bool        `json:"is_primary_owner"`
	IsRestricted       bool        `json:"is_restricted"`
	IsUltraRestricted  bool        `json:"is_ultra_restricted"`
	IsBot              bool        `json:"is_bot"`
	IsAppUser          bool        `json:"is_app_user"`
	Updated            string      `json:"updated"`
	IsEmailConfirmed   bool        `json:"is_email_confirmed"`
	WhoCanShareContact string      `json:"who_can_share_contact_card"`
}

// UserProfile represents a user's profile information
type UserProfile struct {
	AvatarHash             string                 `json:"avatar_hash"`
	StatusText             string                 `json:"status_text"`
	StatusEmoji            string                 `json:"status_emoji"`
	StatusEmojiDisplayInfo []StatusEmojiInfo      `json:"status_emoji_display_info"`
	StatusExpiration       int                    `json:"status_expiration"`
	RealName               string                 `json:"real_name"`
	DisplayName            string                 `json:"display_name"`
	RealNameNormalized     string                 `json:"real_name_normalized"`
	DisplayNameNormalized  string                 `json:"display_name_normalized"`
	Email                  string                 `json:"email"`
	Image24                string                 `json:"image_24"`
	Image32                string                 `json:"image_32"`
	Image48                string                 `json:"image_48"`
	Image72                string                 `json:"image_72"`
	Image192               string                 `json:"image_192"`
	Image512               string                 `json:"image_512"`
	Team                   string                 `json:"team"`
	Fields                 map[string]interface{} `json:"fields"`
	FirstName              string                 `json:"first_name"`
	LastName               string                 `json:"last_name"`
	Title                  string                 `json:"title"`
	Phone                  string                 `json:"phone"`
	Skype                  string                 `json:"skype"`
}

// StatusEmojiInfo represents status emoji display information
type StatusEmojiInfo struct {
	DisplayInfo []EmojiDisplayInfo `json:"display_info"`
}

// EmojiDisplayInfo represents emoji display information
type EmojiDisplayInfo struct {
	EmojiName  string `json:"emoji_name"`
	DisplayURL string `json:"display_url"`
	Unicode    string `json:"unicode"`
}

// SlackMessage represents a Slack message
type SlackMessage struct {
	Type            string                 `json:"type"`
	Subtype         string                 `json:"subtype,omitempty"`
	Text            string                 `json:"text"`
	User            string                 `json:"user,omitempty"`
	BotID           string                 `json:"bot_id,omitempty"`
	Username        string                 `json:"username,omitempty"`
	Channel         string                 `json:"channel,omitempty"`
	Timestamp       string                 `json:"ts"`
	EventTS         string                 `json:"event_ts,omitempty"`
	ThreadTS        string                 `json:"thread_ts,omitempty"`
	ParentType      string                 `json:"parent_type,omitempty"`
	Edited          *EditInfo              `json:"edited,omitempty"`
	Deleted         bool                   `json:"deleted,omitempty"`
	Hidden          bool                   `json:"hidden,omitempty"`
	NoNotifications bool                   `json:"no_notifications,omitempty"`
	Attachments     []Attachment           `json:"attachments,omitempty"`
	Blocks          []Block                `json:"blocks,omitempty"`
	Files           []SlackFile            `json:"files,omitempty"`
	Upload          bool                   `json:"upload,omitempty"`
	DisplayAsBot    bool                   `json:"display_as_bot,omitempty"`
	ClientMsgID     string                 `json:"client_msg_id,omitempty"`
	Reactions       []Reaction             `json:"reactions,omitempty"`
	Replies         []Reply                `json:"replies,omitempty"`
	ReplyCount      int                    `json:"reply_count,omitempty"`
	ReplyUsers      []string               `json:"reply_users,omitempty"`
	ReplyUsersCount int                    `json:"reply_users_count,omitempty"`
	LatestReply     string                 `json:"latest_reply,omitempty"`
	IsStarred       bool                   `json:"is_starred,omitempty"`
	Pinned          bool                   `json:"pinned,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// EditInfo represents message edit information
type EditInfo struct {
	User      string `json:"user"`
	Timestamp string `json:"ts"`
}

// Attachment represents a message attachment
type Attachment struct {
	ID            int                    `json:"id"`
	Color         string                 `json:"color"`
	Fallback      string                 `json:"fallback"`
	CallbackID    string                 `json:"callback_id"`
	AuthorName    string                 `json:"author_name"`
	AuthorSubname string                 `json:"author_subname"`
	AuthorLink    string                 `json:"author_link"`
	AuthorIcon    string                 `json:"author_icon"`
	Title         string                 `json:"title"`
	TitleLink     string                 `json:"title_link"`
	Pretext       string                 `json:"pretext"`
	Text          string                 `json:"text"`
	ImageURL      string                 `json:"image_url"`
	ThumbURL      string                 `json:"thumb_url"`
	Footer        string                 `json:"footer"`
	FooterIcon    string                 `json:"footer_icon"`
	Timestamp     string                 `json:"ts"`
	MarkdownIn    []string               `json:"mrkdwn_in"`
	Actions       []AttachmentAction     `json:"actions"`
	Fields        []AttachmentField      `json:"fields"`
	Blocks        []Block                `json:"blocks"`
	Metadata      map[string]interface{} `json:"metadata"`
}

// AttachmentAction represents an attachment action
type AttachmentAction struct {
	Name            string                  `json:"name"`
	Text            string                  `json:"text"`
	Style           string                  `json:"style"`
	Type            string                  `json:"type"`
	Value           string                  `json:"value"`
	DataSource      string                  `json:"data_source"`
	MinQueryLength  int                     `json:"min_query_length"`
	Options         []AttachmentOption      `json:"options"`
	SelectedOptions []AttachmentOption      `json:"selected_options"`
	Confirm         *ConfirmationDialog     `json:"confirm"`
	URL             string                  `json:"url"`
	OptionGroups    []AttachmentOptionGroup `json:"option_groups"`
}

// AttachmentField represents an attachment field
type AttachmentField struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

// AttachmentOption represents an attachment option
type AttachmentOption struct {
	Text        string `json:"text"`
	Value       string `json:"value"`
	Description string `json:"description"`
}

// AttachmentOptionGroup represents an attachment option group
type AttachmentOptionGroup struct {
	Text    string             `json:"text"`
	Options []AttachmentOption `json:"options"`
}

// ConfirmationDialog represents a confirmation dialog
type ConfirmationDialog struct {
	Title       string `json:"title"`
	Text        string `json:"text"`
	OkText      string `json:"ok_text"`
	DismissText string `json:"dismiss_text"`
}

// Block represents a Slack Block Kit element
type Block struct {
	Type      string                 `json:"type"`
	BlockID   string                 `json:"block_id,omitempty"`
	Elements  []BlockElement         `json:"elements,omitempty"`
	Text      *TextObject            `json:"text,omitempty"`
	Fields    []TextObject           `json:"fields,omitempty"`
	Accessory *BlockElement          `json:"accessory,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// BlockElement represents a Block Kit element
type BlockElement struct {
	Type        string                 `json:"type"`
	Text        *TextObject            `json:"text,omitempty"`
	Elements    []BlockElement         `json:"elements,omitempty"`
	Style       string                 `json:"style,omitempty"`
	Value       string                 `json:"value,omitempty"`
	URL         string                 `json:"url,omitempty"`
	ActionID    string                 `json:"action_id,omitempty"`
	Placeholder *TextObject            `json:"placeholder,omitempty"`
	Confirm     *ConfirmationDialog    `json:"confirm,omitempty"`
	Options     []BlockOption          `json:"options,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// TextObject represents a text object in Block Kit
type TextObject struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Emoji    bool   `json:"emoji,omitempty"`
	Verbatim bool   `json:"verbatim,omitempty"`
}

// BlockOption represents an option in Block Kit
type BlockOption struct {
	Text        *TextObject `json:"text"`
	Value       string      `json:"value"`
	Description *TextObject `json:"description,omitempty"`
	URL         string      `json:"url,omitempty"`
}

// Reaction represents a message reaction
type Reaction struct {
	Name  string   `json:"name"`
	Users []string `json:"users"`
	Count int      `json:"count"`
}

// Reply represents a thread reply
type Reply struct {
	User      string `json:"user"`
	Timestamp string `json:"ts"`
}

// SlackFile represents a Slack file
type SlackFile struct {
	ID                 string                 `json:"id"`
	Created            string                 `json:"created"`
	Timestamp          string                 `json:"timestamp"`
	Name               string                 `json:"name"`
	Title              string                 `json:"title"`
	Mimetype           string                 `json:"mimetype"`
	FileType           string                 `json:"filetype"`
	PrettyType         string                 `json:"pretty_type"`
	User               string                 `json:"user"`
	Editable           bool                   `json:"editable"`
	Size               int64                  `json:"size"`
	Mode               string                 `json:"mode"`
	IsExternal         bool                   `json:"is_external"`
	ExternalType       string                 `json:"external_type"`
	IsPublic           bool                   `json:"is_public"`
	PublicURLShared    bool                   `json:"public_url_shared"`
	DisplayAsBot       bool                   `json:"display_as_bot"`
	Username           string                 `json:"username"`
	URLPrivate         string                 `json:"url_private"`
	URLPrivateDownload string                 `json:"url_private_download"`
	Permalink          string                 `json:"permalink"`
	PermalinkPublic    string                 `json:"permalink_public"`
	EditLink           string                 `json:"edit_link"`
	Preview            string                 `json:"preview"`
	PreviewHighlight   string                 `json:"preview_highlight"`
	Lines              int                    `json:"lines"`
	LinesMore          int                    `json:"lines_more"`
	PublicURL          string                 `json:"public_url"`
	IsStarred          bool                   `json:"is_starred"`
	HasRichPreview     bool                   `json:"has_rich_preview"`
	Channels           []string               `json:"channels"`
	Groups             []string               `json:"groups"`
	IMs                []string               `json:"ims"`
	InitialComment     *SlackMessage          `json:"initial_comment,omitempty"`
	CommentsCount      int                    `json:"comments_count"`
	NumStars           int                    `json:"num_stars"`
	IsPublicChannel    bool                   `json:"is_public_channel"`
	Shares             map[string]interface{} `json:"shares,omitempty"`
	MediaDisplayType   string                 `json:"media_display_type"`
	LastEditor         string                 `json:"last_editor"`
	Updated            string                 `json:"updated"`
	Transcription      *Transcription         `json:"transcription,omitempty"`
}

// Transcription represents file transcription data
type Transcription struct {
	Status string `json:"status"`
	Locale string `json:"locale"`
}

// SlackEvent represents a Slack event for webhooks
type SlackEvent struct {
	Token        string                 `json:"token"`
	TeamID       string                 `json:"team_id"`
	APIAppID     string                 `json:"api_app_id"`
	Event        map[string]interface{} `json:"event"`
	Type         string                 `json:"type"`
	EventID      string                 `json:"event_id"`
	EventTime    int64                  `json:"event_time"`
	AuthedUsers  []string               `json:"authed_users"`
	EventContext string                 `json:"event_context"`
	Challenge    string                 `json:"challenge,omitempty"`
}

// SlackWebhookEvent represents different types of Slack webhook events
type SlackWebhookEvent struct {
	Type            string        `json:"type"`
	Channel         string        `json:"channel,omitempty"`
	User            string        `json:"user,omitempty"`
	Text            string        `json:"text,omitempty"`
	Timestamp       string        `json:"ts,omitempty"`
	EventTS         string        `json:"event_ts,omitempty"`
	ChannelType     string        `json:"channel_type,omitempty"`
	Hidden          bool          `json:"hidden,omitempty"`
	Subtype         string        `json:"subtype,omitempty"`
	Message         *SlackMessage `json:"message,omitempty"`
	PreviousMessage *SlackMessage `json:"previous_message,omitempty"`
	File            *SlackFile    `json:"file,omitempty"`
}

// SlackTeam represents team information
type SlackTeam struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Domain string `json:"domain"`
}

// SlackScope represents OAuth scopes
type SlackScope struct {
	Scopes []string `json:"scopes"`
}

// SlackAuth represents authentication response
type SlackAuth struct {
	OK          bool       `json:"ok"`
	URL         string     `json:"url"`
	Team        SlackTeam  `json:"team"`
	User        SlackUser  `json:"user"`
	TeamID      string     `json:"team_id"`
	UserID      string     `json:"user_id"`
	BotID       string     `json:"bot_id"`
	IsBot       bool       `json:"is_bot"`
	AppID       string     `json:"app_id"`
	AuthedUser  *SlackUser `json:"authed_user,omitempty"`
	Scope       string     `json:"scope"`
	TokenType   string     `json:"token_type"`
	AccessToken string     `json:"access_token"`
	BotUserID   string     `json:"bot_user_id"`
	Enterprise  *struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"enterprise,omitempty"`
}

// WebhookConfig represents webhook configuration
type WebhookConfig struct {
	URL           string   `json:"url"`
	Secret        string   `json:"secret"`
	Events        []string `json:"events"`
	SocketMode    bool     `json:"socket_mode"`
	AppLevelToken string   `json:"app_level_token"`
}

// SyncStats represents synchronization statistics
type SyncStats struct {
	TotalChannels  int           `json:"total_channels"`
	SyncedChannels int           `json:"synced_channels"`
	TotalMessages  int           `json:"total_messages"`
	SyncedMessages int           `json:"synced_messages"`
	TotalFiles     int           `json:"total_files"`
	SyncedFiles    int           `json:"synced_files"`
	TotalUsers     int           `json:"total_users"`
	SyncedUsers    int           `json:"synced_users"`
	StartTime      time.Time     `json:"start_time"`
	EndTime        time.Time     `json:"end_time"`
	Duration       time.Duration `json:"duration"`
	ErrorCount     int           `json:"error_count"`
	LastSyncTime   time.Time     `json:"last_sync_time"`
	NextSyncTime   time.Time     `json:"next_sync_time"`
}

// ChannelInfo represents extended channel information
type ChannelInfo struct {
	*SlackChannel
	MessageCount   int                    `json:"message_count"`
	LastActivity   time.Time              `json:"last_activity"`
	ActiveUsers    []string               `json:"active_users"`
	FileCount      int                    `json:"file_count"`
	PinnedMessages []*SlackMessage        `json:"pinned_messages,omitempty"`
	SyncStatus     string                 `json:"sync_status"`
	LastSyncTime   time.Time              `json:"last_sync_time"`
	SyncError      string                 `json:"sync_error,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// UserActivity represents user activity information
type UserActivity struct {
	UserID         string    `json:"user_id"`
	LastSeen       time.Time `json:"last_seen"`
	MessageCount   int       `json:"message_count"`
	FileCount      int       `json:"file_count"`
	ReactionCount  int       `json:"reaction_count"`
	ChannelsActive []string  `json:"channels_active"`
	IsOnline       bool      `json:"is_online"`
	Status         string    `json:"status"`
	StatusEmoji    string    `json:"status_emoji"`
	TimeZone       string    `json:"timezone"`
	LocalTime      string    `json:"local_time"`
}

// MessageThread represents a message thread
type MessageThread struct {
	ParentMessage *SlackMessage   `json:"parent_message"`
	Replies       []*SlackMessage `json:"replies"`
	ReplyCount    int             `json:"reply_count"`
	LastReply     time.Time       `json:"last_reply"`
	Participants  []string        `json:"participants"`
	IsActive      bool            `json:"is_active"`
}

// SearchResult represents search results
type SearchResult struct {
	Query      string          `json:"query"`
	Total      int             `json:"total"`
	Messages   []*SlackMessage `json:"messages,omitempty"`
	Files      []*SlackFile    `json:"files,omitempty"`
	Channels   []*SlackChannel `json:"channels,omitempty"`
	Users      []*SlackUser    `json:"users,omitempty"`
	Page       int             `json:"page"`
	PerPage    int             `json:"per_page"`
	TotalPages int             `json:"total_pages"`
	SearchTime time.Duration   `json:"search_time"`
}
