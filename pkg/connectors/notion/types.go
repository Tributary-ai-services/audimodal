package notion

import "time"

// NotionPage represents a Notion page
type NotionPage struct {
	Object         string                 `json:"object"`
	ID             string                 `json:"id"`
	CreatedTime    string                 `json:"created_time"`
	LastEditedTime string                 `json:"last_edited_time"`
	CreatedBy      NotionUser             `json:"created_by"`
	LastEditedBy   NotionUser             `json:"last_edited_by"`
	Cover          *NotionFile            `json:"cover,omitempty"`
	Icon           *NotionIcon            `json:"icon,omitempty"`
	Parent         NotionParent           `json:"parent"`
	Archived       bool                   `json:"archived"`
	Properties     map[string]interface{} `json:"properties,omitempty"`
	URL            string                 `json:"url"`
	PublicURL      string                 `json:"public_url,omitempty"`
}

// NotionDatabase represents a Notion database
type NotionDatabase struct {
	Object         string                            `json:"object"`
	ID             string                            `json:"id"`
	CreatedTime    string                            `json:"created_time"`
	LastEditedTime string                            `json:"last_edited_time"`
	CreatedBy      NotionUser                        `json:"created_by"`
	LastEditedBy   NotionUser                        `json:"last_edited_by"`
	Title          []NotionRichText                  `json:"title"`
	Description    []NotionRichText                  `json:"description,omitempty"`
	Icon           *NotionIcon                       `json:"icon,omitempty"`
	Cover          *NotionFile                       `json:"cover,omitempty"`
	Properties     map[string]NotionDatabaseProperty `json:"properties"`
	Parent         NotionParent                      `json:"parent"`
	URL            string                            `json:"url"`
	PublicURL      string                            `json:"public_url,omitempty"`
	Archived       bool                              `json:"archived"`
	InTrash        bool                              `json:"in_trash"`
}

// NotionUser represents a Notion user
type NotionUser struct {
	Object    string `json:"object"`
	ID        string `json:"id"`
	Type      string `json:"type"` // person, bot
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
	Email     string `json:"email,omitempty"`
	Person    *struct {
		Email string `json:"email"`
	} `json:"person,omitempty"`
	Bot *struct {
		Owner NotionBotOwner `json:"owner"`
	} `json:"bot,omitempty"`
}

// NotionBotOwner represents bot owner information
type NotionBotOwner struct {
	Type      string      `json:"type"` // workspace, user
	Workspace bool        `json:"workspace,omitempty"`
	User      *NotionUser `json:"user,omitempty"`
}

// NotionBlock represents a Notion block
type NotionBlock struct {
	Object         string     `json:"object"`
	ID             string     `json:"id"`
	Type           string     `json:"type"`
	CreatedTime    string     `json:"created_time"`
	LastEditedTime string     `json:"last_edited_time"`
	CreatedBy      NotionUser `json:"created_by"`
	LastEditedBy   NotionUser `json:"last_edited_by"`
	HasChildren    bool       `json:"has_children"`
	Archived       bool       `json:"archived"`
	InTrash        bool       `json:"in_trash"`

	// Block type specific content
	Paragraph        *NotionParagraph       `json:"paragraph,omitempty"`
	Heading1         *NotionHeading         `json:"heading_1,omitempty"`
	Heading2         *NotionHeading         `json:"heading_2,omitempty"`
	Heading3         *NotionHeading         `json:"heading_3,omitempty"`
	BulletedListItem *NotionListItem        `json:"bulleted_list_item,omitempty"`
	NumberedListItem *NotionListItem        `json:"numbered_list_item,omitempty"`
	ToDo             *NotionToDo            `json:"to_do,omitempty"`
	Toggle           *NotionToggle          `json:"toggle,omitempty"`
	ChildPage        *NotionChildPage       `json:"child_page,omitempty"`
	ChildDatabase    *NotionChildDatabase   `json:"child_database,omitempty"`
	Embed            *NotionEmbed           `json:"embed,omitempty"`
	Image            *NotionFile            `json:"image,omitempty"`
	Video            *NotionFile            `json:"video,omitempty"`
	File             *NotionFile            `json:"file,omitempty"`
	PDF              *NotionFile            `json:"pdf,omitempty"`
	Bookmark         *NotionBookmark        `json:"bookmark,omitempty"`
	Callout          *NotionCallout         `json:"callout,omitempty"`
	Quote            *NotionQuote           `json:"quote,omitempty"`
	Equation         *NotionEquation        `json:"equation,omitempty"`
	Divider          *NotionDivider         `json:"divider,omitempty"`
	TableOfContents  *NotionTableOfContents `json:"table_of_contents,omitempty"`
	Column           *NotionColumn          `json:"column,omitempty"`
	ColumnList       *NotionColumnList      `json:"column_list,omitempty"`
	LinkPreview      *NotionLinkPreview     `json:"link_preview,omitempty"`
	SyncedBlock      *NotionSyncedBlock     `json:"synced_block,omitempty"`
	Template         *NotionTemplate        `json:"template,omitempty"`
	LinkToPage       *NotionLinkToPage      `json:"link_to_page,omitempty"`
	Table            *NotionTable           `json:"table,omitempty"`
	TableRow         *NotionTableRow        `json:"table_row,omitempty"`
	Unsupported      *NotionUnsupported     `json:"unsupported,omitempty"`
}

// NotionRichText represents rich text content
type NotionRichText struct {
	Type        string              `json:"type"`
	Text        *NotionText         `json:"text,omitempty"`
	Mention     *NotionMention      `json:"mention,omitempty"`
	Equation    *NotionTextEquation `json:"equation,omitempty"`
	Annotations NotionAnnotations   `json:"annotations"`
	PlainText   string              `json:"plain_text"`
	Href        string              `json:"href,omitempty"`
}

// NotionText represents text content
type NotionText struct {
	Content string `json:"content"`
	Link    *struct {
		URL string `json:"url"`
	} `json:"link,omitempty"`
}

// NotionMention represents a mention in rich text
type NotionMention struct {
	Type string      `json:"type"`
	User *NotionUser `json:"user,omitempty"`
	Page *struct {
		ID string `json:"id"`
	} `json:"page,omitempty"`
	Database *struct {
		ID string `json:"id"`
	} `json:"database,omitempty"`
	Date        *NotionDate `json:"date,omitempty"`
	LinkPreview *struct {
		URL string `json:"url"`
	} `json:"link_preview,omitempty"`
	TemplateMention *struct {
		Type                string `json:"type"`
		TemplateMentionDate string `json:"template_mention_date,omitempty"`
		TemplateMentionUser string `json:"template_mention_user,omitempty"`
	} `json:"template_mention,omitempty"`
}

// NotionAnnotations represents text formatting
type NotionAnnotations struct {
	Bold          bool   `json:"bold"`
	Italic        bool   `json:"italic"`
	Strikethrough bool   `json:"strikethrough"`
	Underline     bool   `json:"underline"`
	Code          bool   `json:"code"`
	Color         string `json:"color"`
}

// NotionParent represents the parent of a page or database
type NotionParent struct {
	Type        string `json:"type"`
	PageID      string `json:"page_id,omitempty"`
	DatabaseID  string `json:"database_id,omitempty"`
	WorkspaceID string `json:"workspace,omitempty"`
	BlockID     string `json:"block_id,omitempty"`
}

// NotionIcon represents an icon
type NotionIcon struct {
	Type     string      `json:"type"`
	Emoji    string      `json:"emoji,omitempty"`
	File     *NotionFile `json:"file,omitempty"`
	External *struct {
		URL string `json:"url"`
	} `json:"external,omitempty"`
}

// NotionFile represents a file or image
type NotionFile struct {
	Type string `json:"type"`
	File *struct {
		URL        string `json:"url"`
		ExpiryTime string `json:"expiry_time"`
	} `json:"file,omitempty"`
	External *struct {
		URL string `json:"url"`
	} `json:"external,omitempty"`
	Name    string           `json:"name,omitempty"`
	Caption []NotionRichText `json:"caption,omitempty"`
}

// Block-specific types
type NotionParagraph struct {
	RichText []NotionRichText `json:"rich_text"`
	Color    string           `json:"color"`
	Children []NotionBlock    `json:"children,omitempty"`
}

type NotionHeading struct {
	RichText     []NotionRichText `json:"rich_text"`
	Color        string           `json:"color"`
	IsToggleable bool             `json:"is_toggleable"`
	Children     []NotionBlock    `json:"children,omitempty"`
}

type NotionListItem struct {
	RichText []NotionRichText `json:"rich_text"`
	Color    string           `json:"color"`
	Children []NotionBlock    `json:"children,omitempty"`
}

type NotionToDo struct {
	RichText []NotionRichText `json:"rich_text"`
	Checked  bool             `json:"checked"`
	Color    string           `json:"color"`
	Children []NotionBlock    `json:"children,omitempty"`
}

type NotionToggle struct {
	RichText []NotionRichText `json:"rich_text"`
	Color    string           `json:"color"`
	Children []NotionBlock    `json:"children,omitempty"`
}

type NotionChildPage struct {
	Title string `json:"title"`
}

type NotionChildDatabase struct {
	Title string `json:"title"`
}

type NotionEmbed struct {
	URL     string           `json:"url"`
	Caption []NotionRichText `json:"caption,omitempty"`
}

type NotionBookmark struct {
	URL     string           `json:"url"`
	Caption []NotionRichText `json:"caption,omitempty"`
}

type NotionCallout struct {
	RichText []NotionRichText `json:"rich_text"`
	Icon     *NotionIcon      `json:"icon,omitempty"`
	Color    string           `json:"color"`
	Children []NotionBlock    `json:"children,omitempty"`
}

type NotionQuote struct {
	RichText []NotionRichText `json:"rich_text"`
	Color    string           `json:"color"`
	Children []NotionBlock    `json:"children,omitempty"`
}

type NotionEquation struct {
	Expression string `json:"expression"`
}

type NotionTextEquation struct {
	Expression string `json:"expression"`
}

type NotionDivider struct{}

type NotionTableOfContents struct {
	Color string `json:"color"`
}

type NotionColumn struct {
	Children []NotionBlock `json:"children,omitempty"`
}

type NotionColumnList struct {
	Children []NotionBlock `json:"children,omitempty"`
}

type NotionLinkPreview struct {
	URL string `json:"url"`
}

type NotionSyncedBlock struct {
	SyncedFrom *struct {
		Type    string `json:"type"`
		BlockID string `json:"block_id,omitempty"`
	} `json:"synced_from,omitempty"`
	Children []NotionBlock `json:"children,omitempty"`
}

type NotionTemplate struct {
	RichText []NotionRichText `json:"rich_text"`
	Children []NotionBlock    `json:"children,omitempty"`
}

type NotionLinkToPage struct {
	Type       string `json:"type"`
	PageID     string `json:"page_id,omitempty"`
	DatabaseID string `json:"database_id,omitempty"`
}

type NotionTable struct {
	TableWidth      int           `json:"table_width"`
	HasColumnHeader bool          `json:"has_column_header"`
	HasRowHeader    bool          `json:"has_row_header"`
	Children        []NotionBlock `json:"children,omitempty"`
}

type NotionTableRow struct {
	Cells [][]NotionRichText `json:"cells"`
}

type NotionUnsupported struct{}

// Database property types
type NotionDatabaseProperty struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`

	// Property type specific data
	Title          *NotionTitleProperty          `json:"title,omitempty"`
	RichText       *NotionRichTextProperty       `json:"rich_text,omitempty"`
	Number         *NotionNumberProperty         `json:"number,omitempty"`
	Select         *NotionSelectProperty         `json:"select,omitempty"`
	MultiSelect    *NotionMultiSelectProperty    `json:"multi_select,omitempty"`
	Date           *NotionDateProperty           `json:"date,omitempty"`
	People         *NotionPeopleProperty         `json:"people,omitempty"`
	Files          *NotionFilesProperty          `json:"files,omitempty"`
	Checkbox       *NotionCheckboxProperty       `json:"checkbox,omitempty"`
	URL            *NotionURLProperty            `json:"url,omitempty"`
	Email          *NotionEmailProperty          `json:"email,omitempty"`
	PhoneNumber    *NotionPhoneNumberProperty    `json:"phone_number,omitempty"`
	Formula        *NotionFormulaProperty        `json:"formula,omitempty"`
	Relation       *NotionRelationProperty       `json:"relation,omitempty"`
	Rollup         *NotionRollupProperty         `json:"rollup,omitempty"`
	CreatedTime    *NotionCreatedTimeProperty    `json:"created_time,omitempty"`
	CreatedBy      *NotionCreatedByProperty      `json:"created_by,omitempty"`
	LastEditedTime *NotionLastEditedTimeProperty `json:"last_edited_time,omitempty"`
	LastEditedBy   *NotionLastEditedByProperty   `json:"last_edited_by,omitempty"`
	Status         *NotionStatusProperty         `json:"status,omitempty"`
}

// Property type definitions
type NotionTitleProperty struct{}
type NotionRichTextProperty struct{}
type NotionNumberProperty struct {
	Format string `json:"format"`
}
type NotionSelectProperty struct {
	Options []NotionSelectOption `json:"options"`
}
type NotionMultiSelectProperty struct {
	Options []NotionSelectOption `json:"options"`
}
type NotionSelectOption struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}
type NotionDateProperty struct{}
type NotionPeopleProperty struct{}
type NotionFilesProperty struct{}
type NotionCheckboxProperty struct{}
type NotionURLProperty struct{}
type NotionEmailProperty struct{}
type NotionPhoneNumberProperty struct{}
type NotionFormulaProperty struct {
	Expression string `json:"expression"`
}
type NotionRelationProperty struct {
	DatabaseID     string                `json:"database_id"`
	Type           string                `json:"type"`
	SingleProperty *NotionSingleProperty `json:"single_property,omitempty"`
	DualProperty   *NotionDualProperty   `json:"dual_property,omitempty"`
}
type NotionSingleProperty struct{}
type NotionDualProperty struct {
	SyncedPropertyName string `json:"synced_property_name"`
	SyncedPropertyID   string `json:"synced_property_id"`
}
type NotionRollupProperty struct {
	RelationPropertyName string `json:"relation_property_name"`
	RelationPropertyID   string `json:"relation_property_id"`
	RollupPropertyName   string `json:"rollup_property_name"`
	RollupPropertyID     string `json:"rollup_property_id"`
	Function             string `json:"function"`
}
type NotionCreatedTimeProperty struct{}
type NotionCreatedByProperty struct{}
type NotionLastEditedTimeProperty struct{}
type NotionLastEditedByProperty struct{}
type NotionStatusProperty struct {
	Groups  []NotionStatusGroup  `json:"groups"`
	Options []NotionStatusOption `json:"options"`
}
type NotionStatusGroup struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Color     string   `json:"color"`
	OptionIDs []string `json:"option_ids"`
}
type NotionStatusOption struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// Other common types
type NotionDate struct {
	Start    string `json:"start"`
	End      string `json:"end,omitempty"`
	TimeZone string `json:"time_zone,omitempty"`
}

type NotionError struct {
	Object  string                 `json:"object"`
	Status  int                    `json:"status"`
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// Webhook types
type NotionWebhookEvent struct {
	Object         string                 `json:"object"`
	ID             string                 `json:"id"`
	CreatedTime    string                 `json:"created_time"`
	LastEditedTime string                 `json:"last_edited_time"`
	CreatedBy      NotionUser             `json:"created_by"`
	LastEditedBy   NotionUser             `json:"last_edited_by"`
	Cover          *NotionFile            `json:"cover,omitempty"`
	Icon           *NotionIcon            `json:"icon,omitempty"`
	Parent         NotionParent           `json:"parent"`
	Archived       bool                   `json:"archived"`
	Properties     map[string]interface{} `json:"properties,omitempty"`
	URL            string                 `json:"url"`
	PublicURL      string                 `json:"public_url,omitempty"`
}

// Search and filter types
type NotionSearchFilter struct {
	Value    string `json:"value"`
	Property string `json:"property"`
}

type NotionSearchSort struct {
	Property  string `json:"property"`
	Direction string `json:"direction"`
}

type NotionSearchRequest struct {
	Query       string              `json:"query,omitempty"`
	Sort        *NotionSearchSort   `json:"sort,omitempty"`
	Filter      *NotionSearchFilter `json:"filter,omitempty"`
	StartCursor string              `json:"start_cursor,omitempty"`
	PageSize    int                 `json:"page_size,omitempty"`
}

type NotionSearchResponse struct {
	Object         string        `json:"object"`
	Results        []interface{} `json:"results"`
	NextCursor     string        `json:"next_cursor,omitempty"`
	HasMore        bool          `json:"has_more"`
	Type           string        `json:"type,omitempty"`
	PageOrDatabase interface{}   `json:"page_or_database,omitempty"`
}

// Database query types
type NotionDatabaseQuery struct {
	Filter      *NotionFilter `json:"filter,omitempty"`
	Sorts       []NotionSort  `json:"sorts,omitempty"`
	StartCursor string        `json:"start_cursor,omitempty"`
	PageSize    int           `json:"page_size,omitempty"`
}

type NotionFilter struct {
	Property string         `json:"property,omitempty"`
	And      []NotionFilter `json:"and,omitempty"`
	Or       []NotionFilter `json:"or,omitempty"`

	// Property-specific filters
	RichText    *NotionRichTextFilter    `json:"rich_text,omitempty"`
	Number      *NotionNumberFilter      `json:"number,omitempty"`
	Checkbox    *NotionCheckboxFilter    `json:"checkbox,omitempty"`
	Select      *NotionSelectFilter      `json:"select,omitempty"`
	MultiSelect *NotionMultiSelectFilter `json:"multi_select,omitempty"`
	Date        *NotionDateFilter        `json:"date,omitempty"`
	People      *NotionPeopleFilter      `json:"people,omitempty"`
	Files       *NotionFilesFilter       `json:"files,omitempty"`
	Relation    *NotionRelationFilter    `json:"relation,omitempty"`
	Formula     *NotionFormulaFilter     `json:"formula,omitempty"`
}

type NotionRichTextFilter struct {
	Equals         string `json:"equals,omitempty"`
	DoesNotEqual   string `json:"does_not_equal,omitempty"`
	Contains       string `json:"contains,omitempty"`
	DoesNotContain string `json:"does_not_contain,omitempty"`
	StartsWith     string `json:"starts_with,omitempty"`
	EndsWith       string `json:"ends_with,omitempty"`
	IsEmpty        bool   `json:"is_empty,omitempty"`
	IsNotEmpty     bool   `json:"is_not_empty,omitempty"`
}

type NotionNumberFilter struct {
	Equals               *float64 `json:"equals,omitempty"`
	DoesNotEqual         *float64 `json:"does_not_equal,omitempty"`
	GreaterThan          *float64 `json:"greater_than,omitempty"`
	LessThan             *float64 `json:"less_than,omitempty"`
	GreaterThanOrEqualTo *float64 `json:"greater_than_or_equal_to,omitempty"`
	LessThanOrEqualTo    *float64 `json:"less_than_or_equal_to,omitempty"`
	IsEmpty              bool     `json:"is_empty,omitempty"`
	IsNotEmpty           bool     `json:"is_not_empty,omitempty"`
}

type NotionCheckboxFilter struct {
	Equals       bool `json:"equals"`
	DoesNotEqual bool `json:"does_not_equal"`
}

type NotionSelectFilter struct {
	Equals       string `json:"equals,omitempty"`
	DoesNotEqual string `json:"does_not_equal,omitempty"`
	IsEmpty      bool   `json:"is_empty,omitempty"`
	IsNotEmpty   bool   `json:"is_not_empty,omitempty"`
}

type NotionMultiSelectFilter struct {
	Contains       string `json:"contains,omitempty"`
	DoesNotContain string `json:"does_not_contain,omitempty"`
	IsEmpty        bool   `json:"is_empty,omitempty"`
	IsNotEmpty     bool   `json:"is_not_empty,omitempty"`
}

type NotionDateFilter struct {
	Equals     string    `json:"equals,omitempty"`
	Before     string    `json:"before,omitempty"`
	After      string    `json:"after,omitempty"`
	OnOrBefore string    `json:"on_or_before,omitempty"`
	OnOrAfter  string    `json:"on_or_after,omitempty"`
	PastWeek   *struct{} `json:"past_week,omitempty"`
	PastMonth  *struct{} `json:"past_month,omitempty"`
	PastYear   *struct{} `json:"past_year,omitempty"`
	NextWeek   *struct{} `json:"next_week,omitempty"`
	NextMonth  *struct{} `json:"next_month,omitempty"`
	NextYear   *struct{} `json:"next_year,omitempty"`
	IsEmpty    bool      `json:"is_empty,omitempty"`
	IsNotEmpty bool      `json:"is_not_empty,omitempty"`
}

type NotionPeopleFilter struct {
	Contains       string `json:"contains,omitempty"`
	DoesNotContain string `json:"does_not_contain,omitempty"`
	IsEmpty        bool   `json:"is_empty,omitempty"`
	IsNotEmpty     bool   `json:"is_not_empty,omitempty"`
}

type NotionFilesFilter struct {
	IsEmpty    bool `json:"is_empty,omitempty"`
	IsNotEmpty bool `json:"is_not_empty,omitempty"`
}

type NotionRelationFilter struct {
	Contains       string `json:"contains,omitempty"`
	DoesNotContain string `json:"does_not_contain,omitempty"`
	IsEmpty        bool   `json:"is_empty,omitempty"`
	IsNotEmpty     bool   `json:"is_not_empty,omitempty"`
}

type NotionFormulaFilter struct {
	String   *NotionRichTextFilter `json:"string,omitempty"`
	Checkbox *NotionCheckboxFilter `json:"checkbox,omitempty"`
	Number   *NotionNumberFilter   `json:"number,omitempty"`
	Date     *NotionDateFilter     `json:"date,omitempty"`
}

type NotionSort struct {
	Property  string `json:"property,omitempty"`
	Direction string `json:"direction"`
	Timestamp string `json:"timestamp,omitempty"`
}

// Analytics and metrics types
type NotionWorkspaceStats struct {
	TotalPages     int           `json:"total_pages"`
	TotalDatabases int           `json:"total_databases"`
	TotalUsers     int           `json:"total_users"`
	TotalBlocks    int           `json:"total_blocks"`
	LastSyncTime   time.Time     `json:"last_sync_time"`
	SyncDuration   time.Duration `json:"sync_duration"`
	ErrorCount     int           `json:"error_count"`
	LastError      string        `json:"last_error,omitempty"`
}

type NotionPageStats struct {
	PageID       string    `json:"page_id"`
	Title        string    `json:"title"`
	BlockCount   int       `json:"block_count"`
	WordCount    int       `json:"word_count"`
	LastModified time.Time `json:"last_modified"`
	CreatedBy    string    `json:"created_by"`
	LastEditedBy string    `json:"last_edited_by"`
	ViewCount    int       `json:"view_count"`
	IsPublic     bool      `json:"is_public"`
}

type NotionDatabaseStats struct {
	DatabaseID    string    `json:"database_id"`
	Title         string    `json:"title"`
	RowCount      int       `json:"row_count"`
	PropertyCount int       `json:"property_count"`
	LastModified  time.Time `json:"last_modified"`
	CreatedBy     string    `json:"created_by"`
	LastEditedBy  string    `json:"last_edited_by"`
	ViewCount     int       `json:"view_count"`
	IsPublic      bool      `json:"is_public"`
}

// Integration and OAuth types
type NotionOAuthConfig struct {
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	RedirectURI  string   `json:"redirect_uri"`
	Scopes       []string `json:"scopes"`
}

type NotionOAuthToken struct {
	AccessToken          string     `json:"access_token"`
	TokenType            string     `json:"token_type"`
	BotID                string     `json:"bot_id"`
	WorkspaceName        string     `json:"workspace_name"`
	WorkspaceIcon        string     `json:"workspace_icon"`
	WorkspaceID          string     `json:"workspace_id"`
	Owner                NotionUser `json:"owner"`
	DuplicatedTemplateID string     `json:"duplicated_template_id,omitempty"`
}

// Export and content types
type NotionContentExport struct {
	Format    string                 `json:"format"` // markdown, html, pdf
	Pages     []string               `json:"pages,omitempty"`
	Databases []string               `json:"databases,omitempty"`
	Options   map[string]interface{} `json:"options,omitempty"`
}

type NotionMarkdownExport struct {
	Content  string            `json:"content"`
	Metadata map[string]string `json:"metadata"`
	Assets   []NotionAsset     `json:"assets,omitempty"`
}

type NotionAsset struct {
	Type string `json:"type"` // image, file, video
	URL  string `json:"url"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}
