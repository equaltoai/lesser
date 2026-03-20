package models

import (
	"encoding/json"
	"time"
)

// AccountRegistrationRequest represents a user registration request
type AccountRegistrationRequest struct {
	Username                 string `json:"username"`
	Password                 string `json:"password,omitempty"` // Ignored - passwordless auth only
	Agreement                bool   `json:"agreement"`          // ToS agreement
	Locale                   string `json:"locale,omitempty"`
	Reason                   string `json:"reason,omitempty"` // For approval
	DefaultPostingVisibility string `json:"default_posting_visibility,omitempty"`

	// WalletChallengeID is required for passwordless wallet-based registration flows.
	// The challenge must have been verified via POST /auth/wallet/verify before registration.
	WalletChallengeID string `json:"wallet_challenge_id,omitempty"`
}

// AccountRegistrationResponse represents the response after successful registration
type AccountRegistrationResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Created  bool   `json:"created"`
}

// VerifyCredentialsResponse represents the current user info
type VerifyCredentialsResponse struct {
	ID             string         `json:"id"`
	Username       string         `json:"username"`
	Acct           string         `json:"acct"`
	DisplayName    string         `json:"display_name"`
	Locked         bool           `json:"locked"`
	Bot            bool           `json:"bot"`
	Discoverable   bool           `json:"discoverable"`
	Group          bool           `json:"group"`
	Note           string         `json:"note"`
	URL            string         `json:"url"`
	Avatar         string         `json:"avatar"`
	AvatarStatic   string         `json:"avatar_static"`
	Header         string         `json:"header"`
	HeaderStatic   string         `json:"header_static"`
	FollowersCount int            `json:"followers_count"`
	FollowingCount int            `json:"following_count"`
	StatusesCount  int            `json:"statuses_count"`
	LastStatusAt   string         `json:"last_status_at"`
	Emojis         []any          `json:"emojis"`
	Fields         []any          `json:"fields"`
	CreatedAt      string         `json:"created_at"`
	Role           string         `json:"role"`
	Source         map[string]any `json:"source"`
}

// AppRegistrationRequest represents a Mastodon-compatible client registration request
type AppRegistrationRequest struct {
	ClientName              string `json:"client_name"`
	RedirectURIs            string `json:"redirect_uris"`
	Scopes                  string `json:"scopes"`
	Website                 string `json:"website,omitempty"`
	ClientClass             string `json:"client_class,omitempty"`
	AgentUsername           string `json:"agent_username,omitempty"`
	GrantTypes              string `json:"grant_types,omitempty"`
	TokenEndpointAuthMethod string `json:"token_endpoint_auth_method,omitempty"`
}

// AppRegistrationResponse represents the response after successful app registration
type AppRegistrationResponse struct {
	ID                      string   `json:"id"`
	Name                    string   `json:"name"`
	Website                 string   `json:"website,omitempty"`
	RedirectURI             string   `json:"redirect_uri"`
	ClientID                string   `json:"client_id"`
	ClientSecret            string   `json:"client_secret,omitempty"`
	VapidKey                string   `json:"vapid_key,omitempty"` // For push notifications
	GrantTypes              []string `json:"grant_types,omitempty"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method,omitempty"`
}

// AppSecretRotationRequest represents an operator request to rotate an OAuth client secret.
type AppSecretRotationRequest struct {
	GracePeriodSeconds int  `json:"grace_period_seconds,omitempty"`
	ForceInvalidate    bool `json:"force_invalidate,omitempty"`
}

// AppSecretRotationResponse represents the response after rotating an OAuth client secret.
type AppSecretRotationResponse struct {
	ClientID                 string `json:"client_id"`
	ClientSecret             string `json:"client_secret"`
	TokenEndpointAuthMethod  string `json:"token_endpoint_auth_method,omitempty"`
	GracePeriodSeconds       int    `json:"grace_period_seconds,omitempty"`
	ForcedInvalidation       bool   `json:"forced_invalidation,omitempty"`
	RotatedAt                string `json:"rotated_at,omitempty"`
	PreviousSecretValidUntil string `json:"previous_secret_valid_until,omitempty"`
}

// CreateStatusRequest represents a Mastodon-compatible status creation request
type CreateStatusRequest struct {
	Status      string   `json:"status"`                   // Text content of the status
	InReplyToID string   `json:"in_reply_to_id,omitempty"` // ID of the status being replied to
	MediaIDs    []string `json:"media_ids,omitempty"`      // Array of media attachment IDs
	Poll        *Poll    `json:"poll,omitempty"`           // Poll object
	Sensitive   bool     `json:"sensitive"`                // Mark status as sensitive
	SpoilerText string   `json:"spoiler_text,omitempty"`   // Content warning
	Visibility  string   `json:"visibility"`               // public, unlisted, private, direct
	Language    string   `json:"language,omitempty"`       // ISO 639-1 language code
	ScheduledAt *string  `json:"scheduled_at,omitempty"`   // ISO 8601 datetime for scheduling

	// Lesser extension: optional transparency metadata for agent-authored posts.
	AgentAttribution *AgentPostAttribution `json:"agent_attribution,omitempty"`

	// Lesser extension: optional memory event metadata for corrections/retractions.
	MemoryEvent *AgentMemoryEventRequest `json:"memory_event,omitempty"`
}

// ReblogRequest represents an enhanced reblog request that supports optional commentary
type ReblogRequest struct {
	Comment    *string `json:"comment,omitempty"`    // Optional commentary for quote boost
	Visibility string  `json:"visibility,omitempty"` // Visibility for quote boost (defaults to public)
}

// Poll represents a poll in a status
// Note: This struct is used for both requests and responses.
// For requests, Options field contains simple strings.
// For responses, OptionsData should be populated and will be marshaled as "options".
type Poll struct {
	// Request fields (used when creating a poll)
	Options    []string `json:"options,omitempty"`     // Array of poll options (2-4 options) - only for requests
	ExpiresIn  int      `json:"expires_in,omitempty"`  // Duration in seconds
	Multiple   bool     `json:"multiple,omitempty"`    // Allow multiple choices
	HideTotals bool     `json:"hide_totals,omitempty"` // Hide vote counts until poll ends

	// Response fields (returned in API responses)
	ID          string       `json:"id,omitempty"`           // Poll ID
	ExpiresAt   string       `json:"expires_at,omitempty"`   // ISO 8601 datetime
	Expired     bool         `json:"expired,omitempty"`      // Whether the poll has ended
	VotesCount  int          `json:"votes_count,omitempty"`  // Total number of votes
	VotersCount int          `json:"voters_count,omitempty"` // Total number of voters
	Voted       bool         `json:"voted,omitempty"`        // Whether the current user voted
	OwnVotes    []int        `json:"own_votes,omitempty"`    // Which options the user voted for
	OptionsData []PollOption `json:"-"`                      // Internal field for detailed option data
	Emojis      []any        `json:"emojis,omitempty"`       // Custom emojis used in options
}

// PollOption represents an option in a poll response
type PollOption struct {
	Title      string `json:"title"`       // The text of the option
	VotesCount int    `json:"votes_count"` // Number of votes for this option
}

// PollVoteRequest represents a vote submission request
type PollVoteRequest struct {
	Choices []int `json:"choices"` // Array of option indices (0-based)
}

// Status represents a Mastodon-compatible status response
type Status struct {
	ID                 string         `json:"id"`
	CreatedAt          string         `json:"created_at"`
	InReplyToID        *string        `json:"in_reply_to_id"`
	InReplyToAccountID *string        `json:"in_reply_to_account_id"`
	Sensitive          bool           `json:"sensitive"`
	SpoilerText        string         `json:"spoiler_text"`
	Visibility         string         `json:"visibility"`
	Language           string         `json:"language"`
	URI                string         `json:"uri"`
	URL                string         `json:"url"`
	RepliesCount       int            `json:"replies_count"`
	ReblogsCount       int            `json:"reblogs_count"`
	FavouritesCount    int            `json:"favourites_count"`
	Favourited         bool           `json:"favourited"`
	Reblogged          bool           `json:"reblogged"`
	Muted              bool           `json:"muted"`
	Bookmarked         bool           `json:"bookmarked"`
	Pinned             bool           `json:"pinned"`
	Content            string         `json:"content"`
	Reblog             *Status        `json:"reblog"`
	Application        map[string]any `json:"application,omitempty"`
	Account            Account        `json:"account"`
	MediaAttachments   []any          `json:"media_attachments"`
	Mentions           []any          `json:"mentions"`
	Tags               []any          `json:"tags"`
	Emojis             []any          `json:"emojis"`
	Card               *any           `json:"card"`
	Poll               *Poll          `json:"poll"`
	Filtered           []interface{}  `json:"filtered"`
	EditedAt           *string        `json:"edited_at"`

	// Quote boost extensions
	IsQuoteBoost   bool    `json:"is_quote_boost,omitempty"`
	QuotedStatus   *Status `json:"quoted_status,omitempty"`
	QuotedStatusID *string `json:"quoted_status_id,omitempty"`

	// Lesser extension: agent transparency metadata for Service/bot accounts.
	AgentAttribution *AgentPostAttribution `json:"agent_attribution,omitempty"`
}

// Account represents a Mastodon-compatible account
type Account struct {
	ID             string `json:"id"`
	Username       string `json:"username"`
	Acct           string `json:"acct"`
	DisplayName    string `json:"display_name"`
	Locked         bool   `json:"locked"`
	Bot            bool   `json:"bot"`
	Discoverable   bool   `json:"discoverable"`
	Group          bool   `json:"group"`
	CreatedAt      string `json:"created_at"`
	Note           string `json:"note"`
	URL            string `json:"url"`
	Avatar         string `json:"avatar"`
	AvatarStatic   string `json:"avatar_static"`
	Header         string `json:"header"`
	HeaderStatic   string `json:"header_static"`
	FollowersCount int    `json:"followers_count"`
	FollowingCount int    `json:"following_count"`
	StatusesCount  int    `json:"statuses_count"`
	LastStatusAt   string `json:"last_status_at"`
	Emojis         []any  `json:"emojis"`
	Fields         []any  `json:"fields"`
}

// Context represents the ancestors and descendants of a status
type Context struct {
	Ancestors   []Status `json:"ancestors"`
	Descendants []Status `json:"descendants"`
}

// UpdateCredentialsRequest represents a request to update user credentials
type UpdateCredentialsRequest struct {
	DisplayName  string `json:"display_name"`
	Note         string `json:"note"`
	Avatar       string `json:"avatar"`
	Header       string `json:"header"`
	Locked       bool   `json:"locked"`
	Discoverable bool   `json:"discoverable"`
	Bot          bool   `json:"bot"`
}

// FollowRequest represents a follow request with options
type FollowRequest struct {
	Reblogs *bool `json:"reblogs,omitempty"`
	Notify  *bool `json:"notify,omitempty"`
}

// MuteRequest represents a mute request with options
type MuteRequest struct {
	Notifications *bool   `json:"notifications,omitempty"`
	Duration      *string `json:"duration,omitempty"`
}

// UpdateStatusRequest represents a request to update a status
type UpdateStatusRequest struct {
	Status      string   `json:"status"`
	SpoilerText string   `json:"spoiler_text"`
	Sensitive   bool     `json:"sensitive"`
	MediaIDs    []string `json:"media_ids"`
	Visibility  string   `json:"visibility,omitempty"`
	Language    string   `json:"language,omitempty"`
}

// Relationship represents a relationship between two accounts
type Relationship struct {
	ID                  string `json:"id"`
	Following           bool   `json:"following"`
	ShowingReblogs      bool   `json:"showing_reblogs"`
	Notifying           bool   `json:"notifying"`
	FollowedBy          bool   `json:"followed_by"`
	Blocking            bool   `json:"blocking"`
	BlockedBy           bool   `json:"blocked_by"`
	Muting              bool   `json:"muting"`
	MutingNotifications bool   `json:"muting_notifications"`
	Requested           bool   `json:"requested"`
	DomainBlocking      bool   `json:"domain_blocking"`
	Endorsed            bool   `json:"endorsed"`
	Note                string `json:"note"`
}

// SearchResults represents search results
type SearchResults struct {
	Accounts []Account `json:"accounts"`
	Statuses []Status  `json:"statuses"`
	Hashtags []any     `json:"hashtags"`
}

// Instance represents instance information
type Instance struct {
	URI              string         `json:"uri"`
	Title            string         `json:"title"`
	ShortDescription string         `json:"short_description"`
	Description      string         `json:"description"`
	Version          string         `json:"version"`
	Languages        []string       `json:"languages"`
	Registrations    bool           `json:"registrations"`
	ApprovalRequired bool           `json:"approval_required"`
	InvitesEnabled   bool           `json:"invites_enabled"`
	Configuration    map[string]any `json:"configuration"`
	Stats            map[string]any `json:"stats"`
	Thumbnail        string         `json:"thumbnail"`
	ContactAccount   *Account       `json:"contact_account"`
	Rules            []any          `json:"rules"`
}

// MediaAttachment represents a media attachment
type MediaAttachment struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	URL         string         `json:"url"`
	PreviewURL  string         `json:"preview_url"`
	RemoteURL   *string        `json:"remote_url"`
	TextURL     string         `json:"text_url"`
	Meta        map[string]any `json:"meta"`
	Description string         `json:"description"`
	Blurhash    string         `json:"blurhash"`
}

// MediaUploadResponse represents the response after uploading media
type MediaUploadResponse struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	URL         string         `json:"url"`
	PreviewURL  string         `json:"preview_url"`
	RemoteURL   *string        `json:"remote_url"`
	TextURL     string         `json:"text_url"`
	Meta        map[string]any `json:"meta"`
	Description string         `json:"description"`
	Blurhash    string         `json:"blurhash"`
}

// StatusContext represents the context of a status (ancestors and descendants)
type StatusContext struct {
	Ancestors   []Status `json:"ancestors"`
	Descendants []Status `json:"descendants"`
}

// FavouriteResponse represents the response for favourite/unfavourite actions
type FavouriteResponse struct {
	ID              string `json:"id"`
	CreatedAt       string `json:"created_at"`
	Favourited      bool   `json:"favourited,omitempty"`
	Reblogged       bool   `json:"reblogged,omitempty"`
	FavouritesCount int    `json:"favourites_count,omitempty"`
	ReblogsCount    int    `json:"reblogs_count,omitempty"`
	URI             string `json:"uri"`
	URL             string `json:"url"`
	Content         string `json:"content"`
	Visibility      string `json:"visibility"`
	Language        string `json:"language"`
}

// StatusSource represents the source/raw content of a status
type StatusSource struct {
	ID          string `json:"id"`
	Text        string `json:"text"`
	SpoilerText string `json:"spoiler_text"`
}

// StatusEdit represents a single edit in the status history
type StatusEdit struct {
	Content          string  `json:"content"`
	SpoilerText      string  `json:"spoiler_text"`
	Sensitive        bool    `json:"sensitive"`
	CreatedAt        string  `json:"created_at"`
	Account          Account `json:"account"`
	Poll             *Poll   `json:"poll,omitempty"`
	MediaAttachments []any   `json:"media_attachments"`
	Emojis           []any   `json:"emojis"`
}

// SearchResult represents search results
type SearchResult struct {
	Accounts []Account `json:"accounts"`
	Statuses []Status  `json:"statuses"`
	Hashtags []Tag     `json:"hashtags"`
}

// Tag represents a hashtag
type Tag struct {
	Name      string       `json:"name"`
	URL       string       `json:"url"`
	History   []TagHistory `json:"history,omitempty"`
	Following *bool        `json:"following,omitempty"`
}

// TagHistory represents hashtag usage statistics for a day
type TagHistory struct {
	Day      string `json:"day"`
	Uses     string `json:"uses"`
	Accounts string `json:"accounts"`
}

// ScheduledStatus represents a scheduled status
type ScheduledStatus struct {
	ID               string       `json:"id"`
	ScheduledAt      string       `json:"scheduled_at"`
	Params           StatusParams `json:"params"`
	MediaAttachments []any        `json:"media_attachments"`
}

// StatusParams represents the parameters for a scheduled status
type StatusParams struct {
	Text          string   `json:"text"`
	MediaIDs      []string `json:"media_ids,omitempty"`
	Sensitive     bool     `json:"sensitive"`
	SpoilerText   string   `json:"spoiler_text,omitempty"`
	Visibility    string   `json:"visibility"`
	Language      string   `json:"language,omitempty"`
	InReplyToID   string   `json:"in_reply_to_id,omitempty"`
	ApplicationID string   `json:"application_id,omitempty"`
	Poll          *Poll    `json:"poll,omitempty"`
}

// ScheduledStatusUpdateRequest represents a request to update a scheduled status
type ScheduledStatusUpdateRequest struct {
	ScheduledAt string `json:"scheduled_at"`
}

// MarshalJSON custom marshaling for Poll to handle options field
func (p Poll) MarshalJSON() ([]byte, error) {
	type Alias Poll

	// If OptionsData is populated (response case), we need to marshal it as "options"
	if len(p.OptionsData) > 0 {
		return json.Marshal(&struct {
			Options []PollOption `json:"options"`
			*Alias
		}{
			Options: p.OptionsData,
			Alias:   (*Alias)(&p),
		})
	}

	// Otherwise, use default marshaling (request case with string options)
	return json.Marshal((*Alias)(&p))
}

// Announcement represents an announcement in the Mastodon API
type Announcement struct {
	ID          string                 `json:"id"`
	Content     string                 `json:"content"`
	Text        string                 `json:"text"`
	PublishedAt string                 `json:"published_at"`
	UpdatedAt   string                 `json:"updated_at"`
	AllDay      bool                   `json:"all_day"`
	StartsAt    *string                `json:"starts_at,omitempty"`
	EndsAt      *string                `json:"ends_at,omitempty"`
	Read        bool                   `json:"read"` // Whether the user has dismissed it
	Reactions   []AnnouncementReaction `json:"reactions"`
	Mentions    []AnnouncementAccount  `json:"mentions"`
	Statuses    []AnnouncementStatus   `json:"statuses"`
	Tags        []AnnouncementTag      `json:"tags"`
	Emojis      []CustomEmoji          `json:"emojis"`
}

// AnnouncementReaction represents a reaction to an announcement
type AnnouncementReaction struct {
	Name      string `json:"name"`
	Count     int    `json:"count"`
	Me        bool   `json:"me"`
	URL       string `json:"url,omitempty"`
	StaticURL string `json:"static_url,omitempty"`
}

// AnnouncementAccount represents an account mentioned in an announcement
type AnnouncementAccount struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	URL      string `json:"url"`
	Acct     string `json:"acct"`
}

// AnnouncementStatus represents a status linked in an announcement
type AnnouncementStatus struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

// AnnouncementTag represents a hashtag in an announcement
type AnnouncementTag struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// CustomEmoji represents a custom emoji
type CustomEmoji struct {
	Shortcode       string `json:"shortcode"`
	URL             string `json:"url"`
	StaticURL       string `json:"static_url"`
	VisibleInPicker bool   `json:"visible_in_picker"`
	Category        string `json:"category,omitempty"`
}

// CreateAnnouncementRequest represents a request to create an announcement (admin only)
type CreateAnnouncementRequest struct {
	Content  string `json:"content"`
	Text     string `json:"text,omitempty"`
	AllDay   bool   `json:"all_day"`
	StartsAt string `json:"starts_at,omitempty"`
	EndsAt   string `json:"ends_at,omitempty"`
}

// CreateCustomEmojiRequest represents a request to create a custom emoji (admin only)
type CreateCustomEmojiRequest struct {
	Shortcode string `json:"shortcode"`
	URL       string `json:"url"`
	StaticURL string `json:"static_url,omitempty"`
	Category  string `json:"category,omitempty"`
}

// UpdateCustomEmojiRequest represents a request to update a custom emoji (admin only)
type UpdateCustomEmojiRequest struct {
	Category        *string `json:"category,omitempty"`
	VisibleInPicker *bool   `json:"visible_in_picker,omitempty"`
	Disabled        *bool   `json:"disabled,omitempty"`
}

// Report represents a user report
type Report struct {
	ID            string   `json:"id"`
	ActionTaken   bool     `json:"action_taken"`
	ActionTakenAt *string  `json:"action_taken_at"`
	Category      string   `json:"category"`
	Comment       string   `json:"comment"`
	Forwarded     bool     `json:"forwarded"`
	CreatedAt     string   `json:"created_at"`
	StatusIDs     []string `json:"status_ids"`
	RuleIDs       []int    `json:"rule_ids"`
	TargetAccount *Account `json:"target_account"`
}

// Marker represents a timeline position marker
type Marker struct {
	LastReadID string `json:"last_read_id"`
	UpdatedAt  string `json:"updated_at"`
	Version    int    `json:"version"`
}

// MarkersResponse represents the response for markers endpoint
type MarkersResponse struct {
	Home          *Marker `json:"home,omitempty"`
	Notifications *Marker `json:"notifications,omitempty"`
}

// Preferences represents user preferences
type Preferences struct {
	PostingDefaultVisibility string `json:"posting:default:visibility"`
	PostingDefaultSensitive  bool   `json:"posting:default:sensitive"`
	PostingDefaultLanguage   string `json:"posting:default:language"`
	ReadingExpandMedia       string `json:"reading:expand:media"`
	ReadingExpandSpoilers    bool   `json:"reading:expand:spoilers"`
	ReadingAutoplayGifs      bool   `json:"reading:autoplay:gifs"`
}

// Field represents an account field (used in Account)
type Field struct {
	Name       string  `json:"name"`
	Value      string  `json:"value"`
	VerifiedAt *string `json:"verified_at,omitempty"`
}

// Emoji represents an emoji (used in various responses)
type Emoji struct {
	Shortcode       string `json:"shortcode"`
	URL             string `json:"url"`
	StaticURL       string `json:"static_url"`
	VisibleInPicker bool   `json:"visible_in_picker"`
}

// AccountSource represents the source information for the authenticated user's account
type AccountSource struct {
	Privacy        string  `json:"privacy"`
	Sensitive      bool    `json:"sensitive"`
	Language       string  `json:"language"`
	Note           string  `json:"note"`
	Fields         []Field `json:"fields"`
	FollowRequests int     `json:"follow_requests_count"`
}

// AdminAccount represents an account in admin context
type AdminAccount struct {
	ID                     string    `json:"id"`
	Username               string    `json:"username"`
	Domain                 *string   `json:"domain"`
	CreatedAt              time.Time `json:"created_at"`
	IP                     *string   `json:"ip"`
	IPs                    []AdminIP `json:"ips"`
	Locale                 string    `json:"locale"`
	InviteRequest          *string   `json:"invite_request"`
	Role                   Role      `json:"role"`
	Confirmed              bool      `json:"confirmed"`
	Approved               bool      `json:"approved"`
	Disabled               bool      `json:"disabled"`
	Silenced               bool      `json:"silenced"`
	Suspended              bool      `json:"suspended"`
	Account                Account   `json:"account"`
	CreatedByApplicationID *string   `json:"created_by_application_id"`
	InvitedByAccountID     *string   `json:"invited_by_account_id"`
	ReportsCount           int       `json:"reports_count"`
	ResolvedReportsCount   int       `json:"resolved_reports_count"`
}

// AdminIP represents IP address info in admin context
type AdminIP struct {
	IP     string    `json:"ip"`
	UsedAt time.Time `json:"used_at"`
}

// Role represents a user role
type Role struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Color       string `json:"color,omitempty"`
	Permissions int    `json:"permissions"`
	Highlighted bool   `json:"highlighted"`
}

// AdminReport represents a report in admin context
type AdminReport struct {
	ID                   string     `json:"id"`
	ActionTaken          bool       `json:"action_taken"`
	ActionTakenAt        *time.Time `json:"action_taken_at"`
	Category             string     `json:"category"`
	Comment              string     `json:"comment"`
	Forwarded            bool       `json:"forwarded"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	Account              Account    `json:"account"`
	TargetAccount        Account    `json:"target_account"`
	AssignedAccount      *Account   `json:"assigned_account"`
	ActionTakenByAccount *Account   `json:"action_taken_by_account"`
	Statuses             []Status   `json:"statuses"`
	Rules                []Rule     `json:"rules"`
}

// Rule represents an instance rule
type Rule struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// AdminAccountActionRequest represents a request to take action on an account
type AdminAccountActionRequest struct {
	Type                  string `json:"type"`                // suspend, unsuspend, silence, unsilence, etc.
	ReportID              string `json:"report_id,omitempty"` // Report that caused this action
	WarningPresetID       string `json:"warning_preset_id,omitempty"`
	Text                  string `json:"text,omitempty"` // Reason for action
	SendEmailNotification bool   `json:"send_email_notification"`
}

// DomainBlock represents an instance-level domain block
type DomainBlock struct {
	ID             string    `json:"id"`
	Domain         string    `json:"domain"`
	CreatedAt      time.Time `json:"created_at"`
	Severity       string    `json:"severity"`        // "silence" or "suspend"
	RejectMedia    bool      `json:"reject_media"`    // Whether to reject media files from this domain
	RejectReports  bool      `json:"reject_reports"`  // Whether to reject reports from this domain
	PrivateComment string    `json:"private_comment"` // Private admin notes
	PublicComment  string    `json:"public_comment"`  // Public reason for the block
	Obfuscate      bool      `json:"obfuscate"`       // Whether to obfuscate the domain in public lists
}

// CreateDomainBlockRequest represents a request to create a domain block
type CreateDomainBlockRequest struct {
	Domain         string `json:"domain"`
	Severity       string `json:"severity"`        // "silence" or "suspend"
	RejectMedia    bool   `json:"reject_media"`    // Whether to reject media files from this domain
	RejectReports  bool   `json:"reject_reports"`  // Whether to reject reports from this domain
	PrivateComment string `json:"private_comment"` // Private admin notes
	PublicComment  string `json:"public_comment"`  // Public reason for the block
	Obfuscate      bool   `json:"obfuscate"`       // Whether to obfuscate the domain in public lists
}

// UpdateDomainBlockRequest represents a request to update a domain block
type UpdateDomainBlockRequest struct {
	Severity       string `json:"severity,omitempty"`        // "silence" or "suspend"
	RejectMedia    *bool  `json:"reject_media,omitempty"`    // Whether to reject media files from this domain
	RejectReports  *bool  `json:"reject_reports,omitempty"`  // Whether to reject reports from this domain
	PrivateComment string `json:"private_comment,omitempty"` // Private admin notes
	PublicComment  string `json:"public_comment,omitempty"`  // Public reason for the block
	Obfuscate      *bool  `json:"obfuscate,omitempty"`       // Whether to obfuscate the domain in public lists
}
