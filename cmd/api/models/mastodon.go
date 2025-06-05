package models

// AccountRegistrationRequest represents a user registration request
type AccountRegistrationRequest struct {
	Username  string `json:"username"`
	Email     string `json:"email"`
	Password  string `json:"password"`
	Agreement bool   `json:"agreement"` // ToS agreement
	Locale    string `json:"locale,omitempty"`
	Reason    string `json:"reason,omitempty"` // For approval
}

// AccountRegistrationResponse represents the response after successful registration
type AccountRegistrationResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Created  bool   `json:"created"`
}

// VerifyCredentialsResponse represents the current user info
type VerifyCredentialsResponse struct {
	ID             string                 `json:"id"`
	Username       string                 `json:"username"`
	DisplayName    string                 `json:"display_name"`
	Email          string                 `json:"email"`
	EmailVerified  bool                   `json:"email_verified"`
	Note           string                 `json:"note"`
	URL            string                 `json:"url"`
	Avatar         string                 `json:"avatar"`
	AvatarStatic   string                 `json:"avatar_static"`
	Header         string                 `json:"header"`
	HeaderStatic   string                 `json:"header_static"`
	FollowersCount int                    `json:"followers_count"`
	FollowingCount int                    `json:"following_count"`
	StatusesCount  int                    `json:"statuses_count"`
	LastStatusAt   string                 `json:"last_status_at,omitempty"`
	CreatedAt      string                 `json:"created_at"`
	Role           string                 `json:"role"`
	Source         map[string]interface{} `json:"source"`
}

// AppRegistrationRequest represents a Mastodon-compatible client registration request
type AppRegistrationRequest struct {
	ClientName   string `json:"client_name"`
	RedirectURIs string `json:"redirect_uris"`
	Scopes       string `json:"scopes"`
	Website      string `json:"website,omitempty"`
}

// AppRegistrationResponse represents the response after successful app registration
type AppRegistrationResponse struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Website      string `json:"website,omitempty"`
	RedirectURI  string `json:"redirect_uri"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	VapidKey     string `json:"vapid_key,omitempty"` // For push notifications
}

// CreateStatusRequest represents a Mastodon-compatible status creation request
type CreateStatusRequest struct {
	Status      string   `json:"status"`                   // Text content of the status
	InReplyToID string   `json:"in_reply_to_id,omitempty"` // ID of the status being replied to
	MediaIDs    []string `json:"media_ids,omitempty"`      // Array of media attachment IDs
	Poll        *Poll    `json:"poll,omitempty"`           // Poll object (not yet implemented)
	Sensitive   bool     `json:"sensitive"`                // Mark status as sensitive
	SpoilerText string   `json:"spoiler_text,omitempty"`   // Content warning
	Visibility  string   `json:"visibility"`               // public, unlisted, private, direct
	Language    string   `json:"language,omitempty"`       // ISO 639-1 language code
}

// Poll represents a poll in a status (placeholder for future implementation)
type Poll struct {
	Options    []string `json:"options"`
	ExpiresIn  int      `json:"expires_in"`
	Multiple   bool     `json:"multiple"`
	HideTotals bool     `json:"hide_totals"`
}

// Status represents a Mastodon-compatible status response
type Status struct {
	ID                 string                 `json:"id"`
	CreatedAt          string                 `json:"created_at"`
	InReplyToID        *string                `json:"in_reply_to_id"`
	InReplyToAccountID *string                `json:"in_reply_to_account_id"`
	Sensitive          bool                   `json:"sensitive"`
	SpoilerText        string                 `json:"spoiler_text"`
	Visibility         string                 `json:"visibility"`
	Language           string                 `json:"language"`
	URI                string                 `json:"uri"`
	URL                string                 `json:"url"`
	RepliesCount       int                    `json:"replies_count"`
	ReblogsCount       int                    `json:"reblogs_count"`
	FavouritesCount    int                    `json:"favourites_count"`
	Favourited         bool                   `json:"favourited"`
	Reblogged          bool                   `json:"reblogged"`
	Muted              bool                   `json:"muted"`
	Bookmarked         bool                   `json:"bookmarked"`
	Pinned             bool                   `json:"pinned"`
	Content            string                 `json:"content"`
	Reblog             *Status                `json:"reblog"`
	Application        map[string]interface{} `json:"application,omitempty"`
	Account            Account                `json:"account"`
	MediaAttachments   []interface{}          `json:"media_attachments"`
	Mentions           []interface{}          `json:"mentions"`
	Tags               []interface{}          `json:"tags"`
	Emojis             []interface{}          `json:"emojis"`
	Card               *interface{}           `json:"card"`
	Poll               *Poll                  `json:"poll"`
}

// Account represents a Mastodon-compatible account
type Account struct {
	ID             string        `json:"id"`
	Username       string        `json:"username"`
	Acct           string        `json:"acct"`
	DisplayName    string        `json:"display_name"`
	Locked         bool          `json:"locked"`
	Bot            bool          `json:"bot"`
	Discoverable   bool          `json:"discoverable"`
	Group          bool          `json:"group"`
	CreatedAt      string        `json:"created_at"`
	Note           string        `json:"note"`
	URL            string        `json:"url"`
	Avatar         string        `json:"avatar"`
	AvatarStatic   string        `json:"avatar_static"`
	Header         string        `json:"header"`
	HeaderStatic   string        `json:"header_static"`
	FollowersCount int           `json:"followers_count"`
	FollowingCount int           `json:"following_count"`
	StatusesCount  int           `json:"statuses_count"`
	LastStatusAt   string        `json:"last_status_at"`
	Emojis         []interface{} `json:"emojis"`
	Fields         []interface{} `json:"fields"`
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
	Accounts []Account     `json:"accounts"`
	Statuses []Status      `json:"statuses"`
	Hashtags []interface{} `json:"hashtags"`
}

// Instance represents instance information
type Instance struct {
	URI              string                 `json:"uri"`
	Title            string                 `json:"title"`
	ShortDescription string                 `json:"short_description"`
	Description      string                 `json:"description"`
	Email            string                 `json:"email"`
	Version          string                 `json:"version"`
	Languages        []string               `json:"languages"`
	Registrations    bool                   `json:"registrations"`
	ApprovalRequired bool                   `json:"approval_required"`
	InvitesEnabled   bool                   `json:"invites_enabled"`
	Configuration    map[string]interface{} `json:"configuration"`
	Stats            map[string]interface{} `json:"stats"`
	Thumbnail        string                 `json:"thumbnail"`
	ContactAccount   *Account               `json:"contact_account"`
	Rules            []interface{}          `json:"rules"`
}

// MediaAttachment represents a media attachment
type MediaAttachment struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	URL         string                 `json:"url"`
	PreviewURL  string                 `json:"preview_url"`
	RemoteURL   *string                `json:"remote_url"`
	TextURL     string                 `json:"text_url"`
	Meta        map[string]interface{} `json:"meta"`
	Description string                 `json:"description"`
	Blurhash    string                 `json:"blurhash"`
}

// MediaUploadResponse represents the response after uploading media
type MediaUploadResponse struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	URL         string                 `json:"url"`
	PreviewURL  string                 `json:"preview_url"`
	RemoteURL   *string                `json:"remote_url"`
	TextURL     string                 `json:"text_url"`
	Meta        map[string]interface{} `json:"meta"`
	Description string                 `json:"description"`
	Blurhash    string                 `json:"blurhash"`
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

// SearchResult represents search results
type SearchResult struct {
	Accounts []Account `json:"accounts"`
	Statuses []Status  `json:"statuses"`
	Hashtags []Tag     `json:"hashtags"`
}

// Tag represents a hashtag
type Tag struct {
	Name    string       `json:"name"`
	URL     string       `json:"url"`
	History []TagHistory `json:"history"`
}

// TagHistory represents hashtag usage history
type TagHistory struct {
	Day      string `json:"day"`
	Uses     string `json:"uses"`
	Accounts string `json:"accounts"`
}
