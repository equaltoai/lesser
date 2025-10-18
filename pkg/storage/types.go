package storage

import (
	"errors"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

// Common errors
var (
	ErrVersionConflict = errors.New("version conflict")
)

// Conflict resolution strategies
const (
	// ConflictResolutionLatest chooses the most recent version
	ConflictResolutionLatest = "latest"
	// ConflictResolutionHighestQuality chooses the highest quality version
	ConflictResolutionHighestQuality = "highest_quality"
	// ConflictResolutionLowestBandwidth chooses the lowest bandwidth version
	ConflictResolutionLowestBandwidth = "lowest_bandwidth"
)

// User represents a user account
type User struct {
	ID              string                 `json:"id"`
	Username        string                 `json:"username"`
	Email           string                 `json:"email,omitempty"`
	PasswordHash    string                 `json:"password_hash,omitempty"`
	DisplayName     string                 `json:"display_name,omitempty"`
	Note            string                 `json:"note,omitempty"`   // Bio/description
	Avatar          string                 `json:"avatar,omitempty"` // Avatar image URL
	Header          string                 `json:"header,omitempty"` // Header image URL
	URL             string                 `json:"url,omitempty"`    // Profile URL
	Locked          bool                   `json:"locked"`           // Requires approval for follows
	Discoverable    bool                   `json:"discoverable"`     // Appears in directory
	Fields          []map[string]string    `json:"fields,omitempty"` // Profile metadata fields
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
	Approved        bool                   `json:"approved"`
	Suspended       bool                   `json:"suspended"`
	Silenced        bool                   `json:"silenced"`
	Role            string                 `json:"role"`
	Locale          string                 `json:"locale,omitempty"`
	RecoveryMethods []string               `json:"recovery_methods,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// AccountPin represents a pinned account relationship
type AccountPin struct {
	Username       string    `json:"username"`
	PinnedActorID  string    `json:"pinned_actor_id"`
	PinnedUsername string    `json:"pinned_username"`
	Position       int       `json:"position"`
	CreatedAt      time.Time `json:"created_at"`
}

// AccountNote represents a private note about another account
type AccountNote struct {
	Username      string    `json:"username"`
	TargetActorID string    `json:"target_actor_id"`
	Note          string    `json:"note"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// ActorField represents a custom field on an actor profile
type ActorField struct {
	Name       string    `json:"name"`
	Value      string    `json:"value"`
	VerifiedAt time.Time `json:"verified_at,omitempty"`
}

// SearchResults represents search results with pagination
type SearchResults struct {
	Results      []interface{}         `json:"results"`
	Accounts     []Account             `json:"accounts,omitempty"`
	Statuses     []StatusSearchResult  `json:"statuses,omitempty"`
	Hashtags     []HashtagSearchResult `json:"hashtags,omitempty"`
	NextCursor   string                `json:"next_cursor,omitempty"`
	HasNextPage  bool                  `json:"has_next_page"`
	Total        int                   `json:"total,omitempty"`
	TotalScanned int                   `json:"total_scanned,omitempty"` // Number of items scanned during search
}

// Announce represents a boost/reblog announcement
type Announce struct {
	ID        string    `json:"id"`
	ActorID   string    `json:"actor_id"`
	ObjectID  string    `json:"object_id"`
	Actor     string    `json:"actor"`  // ActivityPub actor URL
	Object    string    `json:"object"` // ActivityPub object URL
	Published time.Time `json:"published"`
	To        []string  `json:"to,omitempty"`
	CC        []string  `json:"cc,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Public    bool      `json:"public"`
}

// StatusPin represents a pinned status
type StatusPin struct {
	Username  string    `json:"username"`
	ActorID   string    `json:"actor_id"`
	StatusID  string    `json:"status_id"`
	Position  int       `json:"position"`
	CreatedAt time.Time `json:"created_at"`
}

// ConflictResolutionStrategy represents strategy for resolving conflicts
type ConflictResolutionStrategy string

const (
	// ConflictResolutionMerge attempts to merge conflicting data
	ConflictResolutionMerge ConflictResolutionStrategy = "merge"
	// ConflictResolutionOverwrite overwrites existing data with new data
	ConflictResolutionOverwrite ConflictResolutionStrategy = "overwrite"
	// ConflictResolutionReject rejects conflicting data
	ConflictResolutionReject ConflictResolutionStrategy = "reject"
)

// DNSCacheEntry represents a DNS cache entry
type DNSCacheEntry struct {
	Hostname   string    `json:"hostname"`
	IPAddress  string    `json:"ip_address"`
	IPs        []string  `json:"ips,omitempty"`
	ResolvedAt time.Time `json:"resolved_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	RecordType string    `json:"record_type"`
	TTL        int64     `json:"ttl,omitempty"`
}

// UserPreferences represents user preference settings
type UserPreferences struct {
	Username                  string            `json:"username"`
	Language                  string            `json:"language,omitempty"`
	Preferences               map[string]string `json:"preferences"`
	UpdatedAt                 time.Time         `json:"updated_at"`
	DefaultPostingVisibility  string            `json:"default_posting_visibility"`
	DefaultMediaSensitive     bool              `json:"default_media_sensitive"`
	ExpandSpoilers            bool              `json:"expand_spoilers"`
	ExpandMedia               string            `json:"expand_media"`
	AutoplayGifs              bool              `json:"autoplay_gifs"`
	ShowFollowCounts          bool              `json:"show_follow_counts"`
	PreferredTimelineOrder    string            `json:"preferred_timeline_order"`
	SearchSuggestionsEnabled  bool              `json:"search_suggestions_enabled"`
	PersonalizedSearchEnabled bool              `json:"personalized_search_enabled"`
	ReblogFilters             map[string]bool   `json:"reblog_filters,omitempty"`
}

// Device represents a push notification device
type Device struct {
	ID                string    `json:"id"`
	DeviceID          string    `json:"device_id"`
	Username          string    `json:"username"`
	Endpoint          string    `json:"endpoint"`
	PublicKey         string    `json:"public_key"`
	AuthKey           string    `json:"auth_key"`
	ServerKey         string    `json:"server_key,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	LastSeenAt        time.Time `json:"last_seen_at"`
	UserAgent         string    `json:"user_agent,omitempty"`
	NotificationTypes []string  `json:"notification_types,omitempty"`
	DeviceName        string    `json:"device_name,omitempty"`
	DeviceType        string    `json:"device_type,omitempty"`
	LastIPAddress     string    `json:"last_ip_address,omitempty"`
	LastUserAgent     string    `json:"last_user_agent,omitempty"`
	TrustLevel        string    `json:"trust_level,omitempty"`
}

// List represents a user-created list
type List struct {
	ID            string    `json:"id"`
	Username      string    `json:"username"`
	Title         string    `json:"title"`
	RepliesPolicy string    `json:"replies_policy"`
	Exclusive     bool      `json:"exclusive"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// TrustRelationship represents a trust relationship between actors
type TrustRelationship = models.TrustRelationship

// TrustScore represents a trust score value
type TrustScore = models.TrustScore

// TrustUpdate represents an update to trust score
type TrustUpdate = models.TrustUpdate

// TrustCategory represents a category of trust
type TrustCategory = models.TrustCategory

// TrustEvidence represents evidence for trust decisions
type TrustEvidence = models.TrustEvidence

// Trust category constants
const (
	// TrustCategoryContent represents trust in content quality
	TrustCategoryContent = models.TrustCategoryContent
	// TrustCategoryBehavior represents trust in user behavior
	TrustCategoryBehavior = models.TrustCategoryBehavior
	// TrustCategoryTechnical represents trust in technical aspects
	TrustCategoryTechnical = models.TrustCategoryTechnical
	// TrustCategoryGeneral represents general trust level
	TrustCategoryGeneral = models.TrustCategoryGeneral
)

// Note: Common storage errors are already defined in errors.go

// Session represents a user session
type Session struct {
	ID                   string    `json:"id"`
	SessionID            string    `json:"session_id"`
	Username             string    `json:"username"`
	CreatedAt            time.Time `json:"created_at"`
	ExpiresAt            time.Time `json:"expires_at"`
	RefreshToken         string    `json:"refresh_token,omitempty"`
	LastActivity         time.Time `json:"last_activity"`
	UserAgent            string    `json:"user_agent,omitempty"`
	IPAddress            string    `json:"ip_address,omitempty"`
	DeviceID             string    `json:"device_id,omitempty"`
	DeviceName           string    `json:"device_name,omitempty"`
	AuthMethod           string    `json:"auth_method,omitempty"`
	PreviousRefreshToken string    `json:"previous_refresh_token,omitempty"`
	TokenRotatedAt       time.Time `json:"token_rotated_at,omitempty"`
}

// AuthorizationCode represents an OAuth authorization code
type AuthorizationCode struct {
	Code          string    `json:"code"`
	ClientID      string    `json:"client_id"`
	Username      string    `json:"username"`
	CodeChallenge string    `json:"code_challenge"`
	ExpiresAt     time.Time `json:"expires_at"`
	Scopes        []string  `json:"scopes"`
}

// RefreshToken represents an OAuth refresh token
type RefreshToken struct {
	Token     string    `json:"token"`
	Username  string    `json:"username"`
	ClientID  string    `json:"client_id"`
	Scopes    []string  `json:"scopes"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// WebAuthnChallenge represents a WebAuthn challenge
type WebAuthnChallenge struct {
	ID          string      `json:"id"`
	Username    string      `json:"username"`
	UserID      string      `json:"user_id"`
	Challenge   string      `json:"challenge"`
	SessionData interface{} `json:"session_data"`
	CreatedAt   time.Time   `json:"created_at"`
	ExpiresAt   time.Time   `json:"expires_at"`
	Type        string      `json:"type"` // "registration" or "authentication"
}

// WebAuthnCredential represents a WebAuthn credential
type WebAuthnCredential struct {
	ID              string    `json:"id"`
	Username        string    `json:"username"`
	UserID          string    `json:"user_id"`
	CredentialID    string    `json:"credential_id"`
	PublicKey       []byte    `json:"public_key"`
	AttestationType string    `json:"attestation_type"`
	AAGUID          []byte    `json:"aaguid"`
	SignCount       uint32    `json:"sign_count"`
	CloneWarning    bool      `json:"clone_warning"`
	BackupEligible  bool      `json:"backup_eligible"`
	BackupState     bool      `json:"backup_state"`
	CreatedAt       time.Time `json:"created_at"`
	LastUsed        time.Time `json:"last_used"`
	LastUsedAt      time.Time `json:"last_used_at"`
	Name            string    `json:"name,omitempty"`
}

// OAuthState represents OAuth authorization state with PKCE
type OAuthState struct {
	State               string    `json:"state"`
	Provider            string    `json:"provider"`
	Username            string    `json:"username,omitempty"`
	Scopes              []string  `json:"scopes,omitempty"`
	CodeChallenge       string    `json:"code_challenge"`
	CodeChallengeMethod string    `json:"code_challenge_method"`
	ClientID            string    `json:"client_id"`
	RedirectURI         string    `json:"redirect_uri"`
	Scope               string    `json:"scope"`
	CreatedAt           time.Time `json:"created_at"`
	ExpiresAt           time.Time `json:"expires_at"`
}

// OAuthClient represents an OAuth client application
type OAuthClient struct {
	ID           string    `json:"id"`
	ClientID     string    `json:"client_id"`
	ClientSecret string    `json:"client_secret"`
	Name         string    `json:"name"`
	Description  string    `json:"description,omitempty"`
	Website      string    `json:"website,omitempty"`
	RedirectURIs []string  `json:"redirect_uris"`
	GrantTypes   []string  `json:"grant_types,omitempty"`
	Scopes       []string  `json:"scopes"`
	OwnerID      string    `json:"owner_id,omitempty"`
	Confidential bool      `json:"confidential"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// OAuthApp represents a registered OAuth application
type OAuthApp struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Website      string    `json:"website,omitempty"`
	ClientID     string    `json:"client_id"`
	ClientSecret string    `json:"client_secret"`
	RedirectURI  string    `json:"redirect_uri"`
	RedirectURIs []string  `json:"redirect_uris,omitempty"`
	Scopes       []string  `json:"scopes,omitempty"`
	VapidKey     string    `json:"vapid_key,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// UserAppConsent represents user consent for an OAuth application
type UserAppConsent struct {
	Username  string    `json:"username"`
	UserID    string    `json:"user_id"`
	ClientID  string    `json:"client_id"`
	AppID     string    `json:"app_id"`
	Scopes    []string  `json:"scopes"`
	GrantedAt time.Time `json:"granted_at"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Block represents a user blocking relationship
type Block struct {
	ID             string    `json:"id"`
	Username       string    `json:"username"`
	Actor          string    `json:"actor"`
	Object         string    `json:"object"`
	BlockedActorID string    `json:"blocked_actor_id"`
	CreatedAt      time.Time `json:"created_at"`
	Published      time.Time `json:"published"`
	Reason         string    `json:"reason,omitempty"`
}

// Mute represents a user muting relationship
type Mute struct {
	ID                string     `json:"id"`
	Username          string     `json:"username"`
	Actor             string     `json:"actor"`
	Object            string     `json:"object"`
	MutedActorID      string     `json:"muted_actor_id"`
	HideNotifications bool       `json:"hide_notifications"`
	CreatedAt         time.Time  `json:"created_at"`
	Published         time.Time  `json:"published"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
	Notifications     bool       `json:"notifications"`
}

// TimelineEntry represents an entry in a user's timeline
type TimelineEntry struct {
	ID           string                 `json:"id"`
	TimelineType string                 `json:"timeline_type"`
	TimelineID   string                 `json:"timeline_id"`
	EntryID      string                 `json:"entry_id"`
	Username     string                 `json:"username"`
	ObjectID     string                 `json:"object_id"`
	PostID       string                 `json:"post_id"`
	ActorID      string                 `json:"actor_id"`
	ActorHandle  string                 `json:"actor_handle"`
	ActivityType string                 `json:"activity_type"`
	Content      string                 `json:"content"`
	ContentType  string                 `json:"content_type"`
	HasMedia     bool                   `json:"has_media"`
	IsReply      bool                   `json:"is_reply"`
	InReplyTo    string                 `json:"in_reply_to,omitempty"`
	BoostedBy    string                 `json:"boosted_by,omitempty"`
	Language     string                 `json:"language,omitempty"`
	Sensitive    bool                   `json:"sensitive"`
	SpoilerText  string                 `json:"spoiler_text,omitempty"`
	Timestamp    time.Time              `json:"timestamp"`
	CreatedAt    time.Time              `json:"created_at"`
	TimelineAt   time.Time              `json:"timeline_at"`
	ExpiresAt    *time.Time             `json:"expires_at,omitempty"`
	Visibility   string                 `json:"visibility"`
	Tags         []string               `json:"tags,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	IsBoost      bool                   `json:"is_boost"`
	OriginalID   string                 `json:"original_id,omitempty"`
}

// Conversation represents a conversation thread
type Conversation struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	LastStatusID string    `json:"last_status_id"`
	Accounts     []string  `json:"accounts"`
	Participants []string  `json:"participants"`
	Unread       bool      `json:"unread"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Marker represents a read position marker
type Marker struct {
	Username   string    `json:"username"`
	Timeline   string    `json:"timeline"`
	LastReadID string    `json:"last_read_id"`
	UpdatedAt  time.Time `json:"updated_at"`
	Version    int       `json:"version"`
}

// WalletCredential represents a blockchain wallet credential
type WalletCredential struct {
	ID         string    `json:"id"`
	Username   string    `json:"username"`
	WalletType string    `json:"wallet_type"`
	Type       string    `json:"type"`
	Address    string    `json:"address"`
	PublicKey  string    `json:"public_key"`
	ChainID    int       `json:"chain_id,omitempty"`
	ENS        string    `json:"ens,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsed   time.Time `json:"last_used"`
	LinkedAt   time.Time `json:"linked_at"`
	Name       string    `json:"name,omitempty"`
	Verified   bool      `json:"verified"`
}

// WalletChallenge represents a wallet authentication challenge
type WalletChallenge struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Challenge string    `json:"challenge"`
	Address   string    `json:"address"`
	ChainID   int       `json:"chain_id"`
	Nonce     string    `json:"nonce"`
	Message   string    `json:"message"`
	IssuedAt  time.Time `json:"issued_at"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Type      string    `json:"type"` // "registration" or "authentication"
}

// WeeklyActivity represents aggregated weekly activity metrics
type WeeklyActivity struct {
	Week          string `json:"week"`          // ISO week format: "2024-W01"
	Statuses      int    `json:"statuses"`      // Number of statuses posted
	Logins        int    `json:"logins"`        // Number of login sessions
	Registrations int    `json:"registrations"` // Number of new registrations
}

// Activity represents a user activity event
type Activity struct {
	ID        string                 `json:"id"`
	Username  string                 `json:"username"`
	Actor     string                 `json:"actor"`
	Object    string                 `json:"object"`
	Published time.Time              `json:"published"`
	Content   string                 `json:"content"`
	Type      string                 `json:"type"`
	ObjectID  string                 `json:"object_id,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	IPAddress string                 `json:"ip_address,omitempty"`
	UserAgent string                 `json:"user_agent,omitempty"`
}

// FederationActivity represents a federation-related activity
type FederationActivity struct {
	ID           string                 `json:"id"`
	Type         string                 `json:"type"`
	ActivityType string                 `json:"activity_type"`
	ByteSize     int64                  `json:"byte_size"`
	Success      bool                   `json:"success"`
	ResponseTime int64                  `json:"response_time"`
	ErrorMessage string                 `json:"error_message,omitempty"`
	ActorID      string                 `json:"actor_id"`
	ObjectID     string                 `json:"object_id,omitempty"`
	TargetID     string                 `json:"target_id,omitempty"`
	Data         map[string]interface{} `json:"data,omitempty"`
	Timestamp    time.Time              `json:"timestamp"`
	Domain       string                 `json:"domain"`
	Status       string                 `json:"status"` // "pending", "delivered", "failed"
}

// ActorMetadata represents additional metadata for an actor
type ActorMetadata struct {
	ActorID        string                 `json:"actor_id"`
	FollowersCount int                    `json:"followers_count"`
	FollowingCount int                    `json:"following_count"`
	StatusesCount  int                    `json:"statuses_count"`
	LastStatusAt   *time.Time             `json:"last_status_at,omitempty"`
	Fields         []ActorField           `json:"fields,omitempty"`
	Bot            bool                   `json:"bot"`
	Discoverable   bool                   `json:"discoverable"`
	Group          bool                   `json:"group"`
	Locked         bool                   `json:"locked"`
	CreatedAt      time.Time              `json:"created_at"`
	NoIndex        bool                   `json:"no_index"`
	Suspended      bool                   `json:"suspended"`
	Limited        bool                   `json:"limited"`
	Memorial       bool                   `json:"memorial"`
	CustomFields   map[string]interface{} `json:"custom_fields,omitempty"`
	UpdatedAt      time.Time              `json:"updated_at"`
}

// SearchSuggestion represents a search suggestion
type SearchSuggestion struct {
	Type        string  `json:"type"`  // "account", "hashtag", "status"
	Value       string  `json:"value"` // The suggested text
	Score       float64 `json:"score"` // Relevance score
	Description string  `json:"description,omitempty"`
}

// TrendingHashtag represents a trending hashtag
type TrendingHashtag struct {
	Name         string                   `json:"name"`
	URL          string                   `json:"url"`
	History      []map[string]interface{} `json:"history"`
	Uses         int                      `json:"uses"`
	UsageCount   int64                    `json:"usage_count"`
	UniqueUsers  int64                    `json:"unique_users"`
	Accounts     int                      `json:"accounts"`
	TrendingRank int                      `json:"trending_rank"`
	UserID       string                   `json:"user_id"`
	LastUsed     time.Time                `json:"last_used"`
	FirstSeen    time.Time                `json:"first_seen"`
	CreatedAt    time.Time                `json:"created_at"`
	UpdatedAt    time.Time                `json:"updated_at"`
}

// TrendingStatus represents a trending status
type TrendingStatus struct {
	ID              string    `json:"id"`
	StatusID        string    `json:"status_id"`
	AuthorID        string    `json:"author_id"`
	Content         string    `json:"content"`
	URL             string    `json:"url"`
	Score           float64   `json:"score"`
	ReblogsCount    int       `json:"reblogs_count"`
	FavouritesCount int       `json:"favourites_count"`
	RepliesCount    int       `json:"replies_count"`
	Engagements     int64     `json:"engagements"`
	TrendingRank    int       `json:"trending_rank"`
	PublishedAt     time.Time `json:"published_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	CreatedAt       time.Time `json:"created_at"`
	Likes           int       `json:"likes"`
	Boosts          int       `json:"boosts"`
	Replies         int       `json:"replies"`
}

// TrendingLink represents a trending link
type TrendingLink struct {
	URL          string                   `json:"url"`
	Title        string                   `json:"title"`
	Description  string                   `json:"description,omitempty"`
	Type         string                   `json:"type"`
	AuthorName   string                   `json:"author_name,omitempty"`
	AuthorURL    string                   `json:"author_url,omitempty"`
	ProviderName string                   `json:"provider_name,omitempty"`
	ProviderURL  string                   `json:"provider_url,omitempty"`
	HTML         string                   `json:"html,omitempty"`
	Width        int                      `json:"width,omitempty"`
	ShareCount   int64                    `json:"share_count"`
	Height       int                      `json:"height,omitempty"`
	Image        string                   `json:"image,omitempty"`
	EmbedURL     string                   `json:"embed_url,omitempty"`
	BlurhashURL  string                   `json:"blurhash_url,omitempty"`
	UserID       string                   `json:"user_id"`
	CreatedAt    time.Time                `json:"created_at"`
	History      []map[string]interface{} `json:"history"`
	TrendingRank int                      `json:"trending_rank"`
	UpdatedAt    time.Time                `json:"updated_at"`
}

// EngagementMetrics represents engagement metrics for content
type EngagementMetrics struct {
	StatusID         string    `json:"status_id"`
	ContentID        string    `json:"content_id"`
	ContentType      string    `json:"content_type"` // "status", "account", "hashtag"
	Views            int64     `json:"views"`
	Likes            int64     `json:"likes"`
	LikeCount        int64     `json:"like_count"`
	Shares           int64     `json:"shares"`
	Comments         int64     `json:"comments"`
	BoostCount       int64     `json:"boost_count"`
	ReplyCount       int64     `json:"reply_count"`
	Score            float64   `json:"score"`
	EngagementBucket string    `json:"engagement_bucket"`
	ClickThroughs    int64     `json:"click_throughs"`
	TimeSpent        int64     `json:"time_spent"` // in seconds
	UniqueViewers    int64     `json:"unique_viewers"`
	Reach            int64     `json:"reach"`
	Impressions      int64     `json:"impressions"`
	EngagementRate   float64   `json:"engagement_rate"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// Reaction represents an emoji reaction to content
type Reaction struct {
	ID        string    `json:"id"`
	StatusID  string    `json:"status_id"`
	AccountID string    `json:"account_id"`
	Name      string    `json:"name"`                 // emoji name or unicode
	Count     int       `json:"count"`                // number of accounts with this reaction
	Me        bool      `json:"me"`                   // whether the authenticated user reacted
	URL       string    `json:"url,omitempty"`        // custom emoji URL
	StaticURL string    `json:"static_url,omitempty"` // static custom emoji URL
	CreatedAt time.Time `json:"created_at"`
}

// Mention represents a mention in a status
type Mention struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Acct     string `json:"acct"`
	URL      string `json:"url"`
	ActorID  string `json:"actor_id"`
}

// CustomEmoji represents a custom emoji
type CustomEmoji struct {
	Shortcode           string    `json:"shortcode"`
	URL                 string    `json:"url"`
	StaticURL           string    `json:"static_url"`
	VisibleInPicker     bool      `json:"visible_in_picker"`
	Category            string    `json:"category,omitempty"`
	Disabled            bool      `json:"disabled"`
	Domain              string    `json:"domain,omitempty"`
	ImageRemoteURL      string    `json:"image_remote_url,omitempty"`
	ImageStorageVersion int       `json:"image_storage_version"`
	ImageUpdatedAt      time.Time `json:"image_updated_at"`
	ImageFileSize       int64     `json:"image_file_size"`
	ImageContentType    string    `json:"image_content_type"`
	ImageWidth          int       `json:"image_width"`
	ImageHeight         int       `json:"image_height"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`

	// Enhanced fields for sophisticated queries
	UsageCount      int64     `json:"usage_count"`        // How many times this emoji has been used
	LastUsedAt      time.Time `json:"last_used_at"`       // When this emoji was last used
	PopularityScore float64   `json:"popularity_score"`   // Calculated popularity score
	SearchKeywords  []string  `json:"search_keywords"`    // Additional search terms
	AltText         string    `json:"alt_text,omitempty"` // Alternative text for accessibility
}

// Announcement represents an admin announcement
type Announcement struct {
	ID          string                 `json:"id"`
	Content     string                 `json:"content"`
	Text        string                 `json:"text"`
	StartsAt    *time.Time             `json:"starts_at,omitempty"`
	EndsAt      *time.Time             `json:"ends_at,omitempty"`
	Published   bool                   `json:"published"`
	AllDay      bool                   `json:"all_day"`
	PublishedAt time.Time              `json:"published_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	CreatedBy   string                 `json:"created_by"`
	Read        bool                   `json:"read,omitempty"`
	Mentions    []Mention              `json:"mentions,omitempty"`
	Statuses    []string               `json:"statuses,omitempty"`
	Tags        []string               `json:"tags,omitempty"`
	Emojis      []CustomEmoji          `json:"emojis,omitempty"`
	Reactions   []Reaction             `json:"reactions,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// SearchQueryStats represents statistics for search queries
type SearchQueryStats struct {
	Query        string    `json:"query"`
	Count        int       `json:"count"`
	LastQueried  time.Time `json:"last_queried"`
	LastUsed     time.Time `json:"last_used"`
	UserCount    int       `json:"user_count"`
	ResultsFound int       `json:"results_found"`
	AvgResults   float64   `json:"avg_results"`
}

// SearchHistoryEntry represents a search history entry
type SearchHistoryEntry struct {
	ID          string    `json:"id"`
	Username    string    `json:"username"`
	UserID      string    `json:"user_id"`
	Query       string    `json:"query"`
	Type        string    `json:"type"`    // "accounts", "statuses", "hashtags"
	Results     int       `json:"results"` // number of results returned
	ResultCount int       `json:"result_count"`
	Timestamp   time.Time `json:"timestamp"`
	SearchedAt  time.Time `json:"searched_at"`
	UserAgent   string    `json:"user_agent,omitempty"`
	IPAddress   string    `json:"ip_address,omitempty"`
}

// CommunityNote represents a community-contributed note
type CommunityNote struct {
	ID               string                 `json:"id"`
	StatusID         string                 `json:"status_id"`
	ObjectID         string                 `json:"object_id"`
	ObjectType       string                 `json:"object_type"`
	AuthorID         string                 `json:"author_id"`
	Content          string                 `json:"content"`
	Language         string                 `json:"language"`
	Category         string                 `json:"category"` // "misleading", "lacking_context", etc.
	Sources          []string               `json:"sources,omitempty"`
	Score            float64                `json:"score"`      // helpfulness score
	VoteCount        int                    `json:"vote_count"` // total votes
	HelpfulVotes     int                    `json:"helpful_votes"`
	NotHelpfulVotes  int                    `json:"not_helpful_votes"`
	Status           string                 `json:"status"` // "pending", "approved", "rejected"
	VisibilityStatus string                 `json:"visibility_status"`
	Sentiment        float64                `json:"sentiment"`
	Objectivity      float64                `json:"objectivity"`
	SourceQuality    float64                `json:"source_quality"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

// CommunityNoteVote represents a vote on a community note
type CommunityNoteVote struct {
	ID        string    `json:"id"`
	NoteID    string    `json:"note_id"`
	VoterID   string    `json:"voter_id"`
	VoteType  string    `json:"vote_type"` // "helpful", "not_helpful", "needs_more_info"
	Helpful   bool      `json:"helpful"`
	Weight    float64   `json:"weight"`
	Rationale string    `json:"rationale,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ConversationStatus represents a status in a conversation
type ConversationStatus struct {
	ConversationID string     `json:"conversation_id"`
	StatusID       string     `json:"status_id"`
	UserID         string     `json:"user_id"`
	Position       int        `json:"position"` // order in conversation
	Unread         bool       `json:"unread"`
	LastReadAt     *time.Time `json:"last_read_at,omitempty"`
	ReplyToID      string     `json:"reply_to_id,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// ConversationMute represents a muted conversation
type ConversationMute struct {
	Username       string    `json:"username"`
	ConversationID string    `json:"conversation_id"`
	MutedAt        time.Time `json:"muted_at"`
	CreatedAt      time.Time `json:"created_at"`
	ExpiresAt      time.Time `json:"expires_at"`
}

// InstanceDomainBlock represents a blocked domain at the instance level
type InstanceDomainBlock struct {
	ID             string    `json:"id"`
	Domain         string    `json:"domain"`
	Severity       string    `json:"severity"` // "silence", "suspend"
	RejectMedia    bool      `json:"reject_media"`
	RejectReports  bool      `json:"reject_reports"`
	Obfuscate      bool      `json:"obfuscate"`
	PrivateComment string    `json:"private_comment,omitempty"`
	PublicComment  string    `json:"public_comment,omitempty"`
	CreatedBy      string    `json:"created_by"`
	CreatedByID    string    `json:"created_by_id"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// InstanceInfo represents instance-level information and metadata
type InstanceInfo struct {
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
	URLs             map[string]string      `json:"urls,omitempty"`
	Stats            map[string]interface{} `json:"stats,omitempty"`
	Thumbnail        string                 `json:"thumbnail,omitempty"`
	ContactAccount   string                 `json:"contact_account,omitempty"`
	Rules            []string               `json:"rules,omitempty"`
	UpdatedAt        time.Time              `json:"updated_at"`
	// Federation fields
	Domain        string    `json:"domain"`
	Software      string    `json:"software"`
	FirstSeen     time.Time `json:"first_seen"`
	LastSeen      time.Time `json:"last_seen"`
	PublicKey     string    `json:"public_key"`
	SharedInbox   string    `json:"shared_inbox"`
	TrustScore    float64   `json:"trust_score"`
	ActiveUsers   int64     `json:"active_users"`
	TotalMessages int64     `json:"total_messages"`
}

// FeaturedTag represents a tag featured on a user's profile
type FeaturedTag struct {
	ID            string     `json:"id"`
	Username      string     `json:"username"`
	Name          string     `json:"name"`
	URL           string     `json:"url"`
	StatusesCount int        `json:"statuses_count"`
	LastStatusAt  *time.Time `json:"last_status_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

// EmailDomainBlock represents a blocked email domain
type EmailDomainBlock struct {
	ID        string    `json:"id"`
	Domain    string    `json:"domain"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DomainAllow represents an explicitly allowed domain
type DomainAllow struct {
	ID        string    `json:"id"`
	Domain    string    `json:"domain"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DomainBlock represents a user-level domain block
type DomainBlock struct {
	Username  string    `json:"username"`
	Domain    string    `json:"domain"`
	CreatedAt time.Time `json:"created_at"`
}

// FederationStats represents federation statistics
type FederationStats struct {
	Domain               string    `json:"domain"`
	ActiveConnections    int       `json:"active_connections"`
	TotalActivities      int64     `json:"total_activities"`
	SuccessfulDeliveries int64     `json:"successful_deliveries"`
	FailedDeliveries     int64     `json:"failed_deliveries"`
	AverageResponseTime  float64   `json:"average_response_time"`
	LastActivity         time.Time `json:"last_activity"`
	HealthScore          float64   `json:"health_score"`
	UpdatedAt            time.Time `json:"updated_at"`
	// Additional federation stats
	ActiveInstances int64 `json:"active_instances"`
	TotalMessages   int64 `json:"total_messages"`
	TotalUsers      int64 `json:"total_users"`
}

// InstanceStats represents instance-wide statistics
type InstanceStats struct {
	Domain        string    `json:"domain"`
	UserCount     int64     `json:"user_count"`
	StatusCount   int64     `json:"status_count"`
	DomainCount   int64     `json:"domain_count"`
	LastWeekUsers int64     `json:"last_week_users"`
	UpdatedAt     time.Time `json:"updated_at"`
	// Additional instance info
	Software      string    `json:"software"`
	Version       string    `json:"version"`
	ActiveUsers   int64     `json:"active_users"`
	TotalMessages int64     `json:"total_messages"`
	FirstSeen     time.Time `json:"first_seen"`
	LastSeen      time.Time `json:"last_seen"`
	TrustScore    float64   `json:"trust_score"`
	// Federation stats
	ErrorRate       float64   `json:"error_rate"`
	AvgResponseTime float64   `json:"avg_response_time"`
	TotalRequests   int64     `json:"total_requests"`
	LastDayStats    *DayStats `json:"last_day_stats,omitempty"`
}

// DayStats represents daily statistics
type DayStats struct {
	Date         time.Time `json:"date"`
	Requests     int64     `json:"requests"`
	Errors       int64     `json:"errors"`
	ResponseTime float64   `json:"response_time"`
	ActiveUsers  int64     `json:"active_users"`
	Messages     int64     `json:"messages"`
}

// FederationCost represents the cost of federation activities
type FederationCost struct {
	Domain        string    `json:"domain"`
	ActivityType  string    `json:"activity_type"`
	Count         int64     `json:"count"`
	TotalCost     float64   `json:"total_cost"`
	AverageCost   float64   `json:"average_cost"`
	Currency      string    `json:"currency"`
	BillingPeriod string    `json:"billing_period"`
	UpdatedAt     time.Time `json:"updated_at"`
	// Additional cost tracking fields
	Period           string    `json:"period"`
	IngressBytes     int64     `json:"ingress_bytes"`
	EgressBytes      int64     `json:"egress_bytes"`
	RequestCount     int64     `json:"request_count"`
	ErrorCount       int64     `json:"error_count"`
	ErrorRate        float64   `json:"error_rate"`
	AvgResponseTime  float64   `json:"avg_response_time"`
	EstimatedCostUSD float64   `json:"estimated_cost_usd"`
	LastUpdated      time.Time `json:"last_updated"`
}

// InstanceHealthReport represents health metrics for an instance
type InstanceHealthReport struct {
	Domain              string    `json:"domain"`
	Status              string    `json:"status"`        // "healthy", "degraded", "down"
	ResponseTime        float64   `json:"response_time"` // in milliseconds
	Uptime              float64   `json:"uptime"`        // percentage
	ErrorRate           float64   `json:"error_rate"`    // percentage
	LastChecked         time.Time `json:"last_checked"`
	ConsecutiveFailures int       `json:"consecutive_failures"`
	Issues              []string  `json:"issues,omitempty"`
	// Additional health metrics
	FederationDelay float64  `json:"federation_delay"`
	QueueDepth      int64    `json:"queue_depth"`
	Recommendations []string `json:"recommendations,omitempty"`
}

// CostProjection represents projected federation costs
type CostProjection struct {
	Domain        string    `json:"domain"`
	Period        string    `json:"period"` // "daily", "weekly", "monthly"
	ProjectedCost float64   `json:"projected_cost"`
	ActualCost    float64   `json:"actual_cost,omitempty"`
	Currency      string    `json:"currency"`
	Confidence    float64   `json:"confidence"` // 0.0-1.0
	CalculatedAt  time.Time `json:"calculated_at"`
	// Additional projection fields
	CurrentCost     float64  `json:"current_cost"`
	Variance        float64  `json:"variance"`
	TopDrivers      []Driver `json:"top_drivers,omitempty"`
	Recommendations []string `json:"recommendations,omitempty"`
}

// Driver represents a factor that drives federation costs
type Driver struct {
	Name         string  `json:"name"`
	Impact       float64 `json:"impact"` // Impact factor on total cost
	Description  string  `json:"description"`
	Optimization string  `json:"optimization,omitempty"`
	// Additional driver fields
	Type           string  `json:"type"`
	Domain         string  `json:"domain"`
	Cost           float64 `json:"cost"`
	PercentOfTotal float64 `json:"percent_of_total"`
	Trend          string  `json:"trend"` // "increasing", "decreasing", "stable"
}

// FederationNode represents a node in the federation network
type FederationNode struct {
	Domain      string                 `json:"domain"`
	Software    string                 `json:"software"` // "mastodon", "lemmy", etc.
	Version     string                 `json:"version"`
	UserCount   int64                  `json:"user_count"`
	StatusCount int64                  `json:"status_count"`
	Connections int                    `json:"connections"` // number of connected instances
	LastSeen    time.Time              `json:"last_seen"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	// Additional federation node fields
	DisplayName       string    `json:"display_name"`
	Description       string    `json:"description"`
	ActiveUsers       int64     `json:"active_users"`
	FirstSeen         time.Time `json:"first_seen"`
	Health            string    `json:"health"` // "healthy", "degraded", "down"
	ErrorRate         float64   `json:"error_rate"`
	ResponseTime      float64   `json:"response_time"`
	ConnectionType    string    `json:"connection_type"` // "direct", "relay", etc.
	TotalConnections  int64     `json:"total_connections"`
	ActiveConnections int64     `json:"active_connections"`
	ActivityVolume    int64     `json:"activity_volume"`
}

// FederationEdge represents a connection between federation nodes
type FederationEdge struct {
	SourceDomain   string    `json:"source_domain"`
	TargetDomain   string    `json:"target_domain"`
	ConnectionType string    `json:"connection_type"` // "follow", "delivery", "block"
	Strength       float64   `json:"strength"`        // connection strength metric
	LastActivity   time.Time `json:"last_activity"`
	CreatedAt      time.Time `json:"created_at"`
	// Additional edge metrics
	VolumeIn    int64   `json:"volume_in"`
	VolumeOut   int64   `json:"volume_out"`
	SharedUsers int64   `json:"shared_users"`
	ErrorCount  int64   `json:"error_count"`
	SuccessRate float64 `json:"success_rate"`
}

// InstanceMetadata represents metadata about a federated instance
type InstanceMetadata struct {
	Domain       string                 `json:"domain"`
	Title        string                 `json:"title"`
	Description  string                 `json:"description"`
	Software     string                 `json:"software"`
	Version      string                 `json:"version"`
	AdminContact string                 `json:"admin_contact,omitempty"`
	Languages    []string               `json:"languages,omitempty"`
	Categories   []string               `json:"categories,omitempty"`
	Rules        []string               `json:"rules,omitempty"`
	Thumbnail    string                 `json:"thumbnail,omitempty"`
	CustomFields map[string]interface{} `json:"custom_fields,omitempty"`
	LastUpdated  time.Time              `json:"last_updated"`
	// Additional metadata fields
	DisplayName     string `json:"display_name"`
	UserCount       int64  `json:"user_count"`
	StatusCount     int64  `json:"status_count"`
	NodeInfo        string `json:"node_info,omitempty"`
	InstanceInfo    string `json:"instance_info,omitempty"`
	FederationNotes string `json:"federation_notes,omitempty"`
}

// InstanceCluster represents a group of related instances
type InstanceCluster struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Domains     []string  `json:"domains"`
	ClusterType string    `json:"cluster_type"` // "geographic", "thematic", "technical"
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Additional cluster fields
	ClusterID  string   `json:"cluster_id"`
	Instances  []string `json:"instances"`
	CenterNode string   `json:"center_node"`
	Cohesion   float64  `json:"cohesion"`
	Size       int      `json:"size"`
}

// InstanceConnection represents a connection status between instances
type InstanceConnection struct {
	SourceDomain  string    `json:"source_domain"`
	TargetDomain  string    `json:"target_domain"`
	Status        string    `json:"status"`    // "connected", "blocked", "limited"
	Direction     string    `json:"direction"` // "bidirectional", "outbound", "inbound"
	EstablishedAt time.Time `json:"established_at"`
	LastActivity  time.Time `json:"last_activity"`
	ActivityCount int64     `json:"activity_count"`
	// Additional connection fields
	Domain         string  `json:"domain"`
	ConnectionType string  `json:"connection_type"`
	ResponseTimeMs float64 `json:"response_time_ms"`
	Health         string  `json:"health"` // "healthy", "degraded", "unhealthy"
	VolumeIn       int64   `json:"volume_in"`
	VolumeOut      int64   `json:"volume_out"`
	Success        bool    `json:"success"`
}

// StatusSearchResult represents a status found in search results
type StatusSearchResult struct {
	ID             string    `json:"id"`
	StatusID       string    `json:"status_id"`
	URL            string    `json:"url"`
	Content        string    `json:"content"`
	AuthorID       string    `json:"author_id"`
	AuthorUsername string    `json:"author_username"`
	Published      time.Time `json:"published"`
	Visibility     string    `json:"visibility"`
	Language       string    `json:"language,omitempty"`
	Tags           []string  `json:"tags,omitempty"`
	Score          float64   `json:"score,omitempty"`
	Highlights     []string  `json:"highlights,omitempty"`
}

// Move represents an account move/migration
type Move struct {
	ID         string    `json:"id"`
	Actor      string    `json:"actor"`  // Old actor (moving from)
	Target     string    `json:"target"` // New actor (moving to)
	OldActorID string    `json:"old_actor_id"`
	NewActorID string    `json:"new_actor_id"`
	Published  time.Time `json:"published"`
	CreatedAt  time.Time `json:"created_at"`
	Verified   bool      `json:"verified"`
}

// StreamingPreferences represents user streaming preferences
type StreamingPreferences struct {
	Username                string    `json:"username"`
	DeviceID                string    `json:"device_id,omitempty"`
	NotificationSounds      bool      `json:"notification_sounds"`
	AutoPlayGifs            bool      `json:"auto_play_gifs"`
	AutoPlayVideos          bool      `json:"auto_play_videos"`
	ExpandSpoilers          bool      `json:"expand_spoilers"`
	ReduceMotion            bool      `json:"reduce_motion"`
	DefaultQuality          string    `json:"default_quality,omitempty"`
	AutoQuality             bool      `json:"auto_quality"`
	PreloadNext             bool      `json:"preload_next"`
	DataSaverMode           bool      `json:"data_saver_mode"`
	PreferredCodec          string    `json:"preferred_codec,omitempty"`
	MaxBandwidthMbps        int64     `json:"max_bandwidth_mbps,omitempty"`
	BufferSizeSeconds       int       `json:"buffer_size_seconds,omitempty"`
	HDREnabled              bool      `json:"hdr_enabled"`
	ColorSpace              string    `json:"color_space,omitempty"`
	SubtitleEnabled         bool      `json:"subtitle_enabled"`
	SubtitleLanguage        string    `json:"subtitle_language,omitempty"`
	AudioDescriptionEnabled bool      `json:"audio_description_enabled"`
	ClosedCaptionsEnabled   bool      `json:"closed_captions_enabled"`
	Version                 int       `json:"version"`
	SchemaVersion           int       `json:"schema_version"`
	UpdatedAt               time.Time `json:"updated_at"`
}

// ScheduledStatus represents a scheduled status post
type ScheduledStatus struct {
	ID            string                 `json:"id"`
	Username      string                 `json:"username"`
	Content       string                 `json:"content"`
	Status        string                 `json:"status"`
	Visibility    string                 `json:"visibility"`
	Sensitive     bool                   `json:"sensitive"`
	SpoilerText   string                 `json:"spoiler_text,omitempty"`
	Language      string                 `json:"language,omitempty"`
	InReplyToID   string                 `json:"in_reply_to_id,omitempty"`
	ScheduledAt   time.Time              `json:"scheduled_at"`
	Published     bool                   `json:"published"`
	PublishedAt   *time.Time             `json:"published_at,omitempty"`
	ApplicationID string                 `json:"application_id,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
	MediaIDs      []string               `json:"media_ids,omitempty"`
	Poll          map[string]interface{} `json:"poll,omitempty"`
	InReplyTo     string                 `json:"in_reply_to,omitempty"`
}

// Reputation represents reputation score for an actor
type Reputation struct {
	ActorID           string    `json:"actor_id"`
	InstanceURL       string    `json:"instance_url"`
	Score             float64   `json:"score"`
	TotalScore        float64   `json:"total_score"`
	TrustScore        float64   `json:"trust_score"`
	ActivityScore     float64   `json:"activity_score"`
	ModerationScore   float64   `json:"moderation_score"`
	CommunityScore    float64   `json:"community_score"`
	Timestamp         time.Time `json:"timestamp"`
	CalculatedAt      time.Time `json:"calculated_at"`
	Evidence          []string  `json:"evidence,omitempty"`
	Category          string    `json:"category"`
	Version           int       `json:"version"`
	TotalPosts        int       `json:"total_posts"`
	TotalFollowers    int       `json:"total_followers"`
	AccountAge        int       `json:"account_age"`
	VouchCount        int       `json:"vouch_count"`
	TrustingActors    []string  `json:"trusting_actors,omitempty"`
	AverageTrustScore float64   `json:"average_trust_score"`
	ReportsReceived   int       `json:"reports_received"`
	ReportsUpheld     int       `json:"reports_upheld"`
	FalseReports      int       `json:"false_reports"`
	Signature         string    `json:"signature,omitempty"`
	PublicKey         string    `json:"public_key,omitempty"`
}

// Report represents a moderation report
type Report struct {
	ID                string     `json:"id"`
	ReporterID        string     `json:"reporter_id"`
	TargetID          string     `json:"target_id"`
	TargetAccountID   string     `json:"target_account_id"`
	StatusIDs         []string   `json:"status_ids,omitempty"`
	Comment           string     `json:"comment"`
	Category          string     `json:"category"`
	RuleIDs           []string   `json:"rule_ids,omitempty"`
	ActionTaken       string     `json:"action_taken"`
	ActionTakenAt     *time.Time `json:"action_taken_at,omitempty"`
	Forwarded         bool       `json:"forwarded"`
	Status            string     `json:"status"`
	ModeratorID       string     `json:"moderator_id,omitempty"`
	ModerationEventID string     `json:"moderation_event_id,omitempty"`
	AssignedTo        string     `json:"assigned_to,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// RelayInfo represents ActivityPub relay information
type RelayInfo struct {
	URL        string    `json:"url"`
	Domain     string    `json:"domain"`
	Enabled    bool      `json:"enabled"`
	Active     bool      `json:"active"`
	State      string    `json:"state"`  // "pending", "accepted", "rejected"
	Status     string    `json:"status"` // health status
	InboxURL   string    `json:"inbox_url"`
	ActorURL   string    `json:"actor_url"`
	LastSeenAt time.Time `json:"last_seen_at"`
	ErrorCount int       `json:"error_count"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	TTL        int64     `json:"ttl,omitempty"`
}

// PushSubscriptionAlerts represents push notification alert preferences
type PushSubscriptionAlerts struct {
	Follow        bool `json:"follow"`
	Favourite     bool `json:"favourite"`
	Reblog        bool `json:"reblog"`
	Mention       bool `json:"mention"`
	Poll          bool `json:"poll"`
	FollowRequest bool `json:"follow_request"`
	Status        bool `json:"status"`
	Update        bool `json:"update"`
	AdminSignUp   bool `json:"admin.sign_up"`
	AdminReport   bool `json:"admin.report"`
}

// Flag represents a content flag for moderation
type Flag struct {
	ID         string     `json:"id"`
	ContentID  string     `json:"content_id"`
	FlaggerID  string     `json:"flagger_id"`
	Reason     string     `json:"reason"`
	Status     string     `json:"status"` // "pending", "resolved", "dismissed"
	Published  time.Time  `json:"published"`
	Actor      string     `json:"actor"`
	Object     []string   `json:"object"`
	Content    string     `json:"content"`
	ReviewedBy string     `json:"reviewed_by,omitempty"`
	ReviewedAt *time.Time `json:"reviewed_at,omitempty"`
	ReviewNote string     `json:"review_note,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// Vouch represents a trust vouch between actors
type Vouch struct {
	ID                string     `json:"id"`
	VoucherID         string     `json:"voucher_id"`
	VoucheeID         string     `json:"vouchee_id"`
	From              string     `json:"from"` // Same as VoucherID
	To                string     `json:"to"`   // Same as VoucheeID
	Active            bool       `json:"active"`
	Revoked           bool       `json:"revoked"`
	RevokedAt         *time.Time `json:"revoked_at,omitempty"`
	Category          string     `json:"category"`
	Strength          float64    `json:"strength"`
	Comment           string     `json:"comment,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
	Confidence        float64    `json:"confidence"`
	Context           string     `json:"context,omitempty"`
	VoucherReputation float64    `json:"voucher_reputation"`
	Signature         string     `json:"signature,omitempty"`
}

// Like represents a like/favorite activity
type Like struct {
	ID        string    `json:"id"`
	Actor     string    `json:"actor"`
	Object    string    `json:"object"`
	Published time.Time `json:"published"`
	CreatedAt time.Time `json:"created_at"`
}

// AffectedFollow represents follow relationships affected by domain blocks
type AffectedFollow struct {
	FollowerID   string    `json:"follower_id"`
	FolloweeID   string    `json:"followee_id"`
	CreatedAt    time.Time `json:"created_at"`
	Domain       string    `json:"domain"`
	LocalUser    string    `json:"local_user"`
	RemoteUser   string    `json:"remote_user"`
	Direction    string    `json:"direction"` // "following", "follower", "mutual"
	LastActivity time.Time `json:"last_activity"`
}

// UpdateHistory represents edit history for a status
type UpdateHistory struct {
	StatusID      string                 `json:"status_id"`
	ObjectID      string                 `json:"object_id"` // For object updates
	Version       int                    `json:"version"`
	Content       string                 `json:"content"`
	UpdatedAt     time.Time              `json:"updated_at"`
	EditedBy      string                 `json:"edited_by"`
	UpdatedBy     string                 `json:"updated_by"` // Alternative field name
	Changes       map[string]interface{} `json:"changes,omitempty"`
	PreviousState map[string]interface{} `json:"previous_state,omitempty"`
	Summary       string                 `json:"summary,omitempty"`
}

// ModerationEvent represents a moderation action event
type ModerationEvent struct {
	ID              string                 `json:"id"`
	Type            string                 `json:"type"` // "warn", "silence", "suspend", etc.
	EventType       string                 `json:"event_type"`
	TargetID        string                 `json:"target_id"` // user or content ID
	ObjectID        string                 `json:"object_id"`
	ObjectType      string                 `json:"object_type"`
	ModeratorID     string                 `json:"moderator_id"`
	ActorID         string                 `json:"actor_id"`
	Category        string                 `json:"category"`
	Severity        string                 `json:"severity"`
	ConfidenceScore float64                `json:"confidence_score"`
	Evidence        []any                  `json:"evidence,omitempty"`
	Reason          string                 `json:"reason"`
	Data            map[string]interface{} `json:"data,omitempty"`
	Created         time.Time              `json:"created"`
	Updated         time.Time              `json:"updated"`
	CreatedAt       time.Time              `json:"created_at"`
	ExpiresAt       *time.Time             `json:"expires_at,omitempty"`
	TTL             int64                  `json:"TTL,omitempty"`
}

// HashtagSearchResult represents a hashtag found in search results
type HashtagSearchResult struct {
	Name     string                `json:"name"`
	URL      string                `json:"url"`
	Uses     int                   `json:"uses"`
	Accounts int                   `json:"accounts"`
	History  []HashtagHistoryEntry `json:"history"`
}

// SeveredRelationship represents a severed federation relationship
type SeveredRelationship struct {
	ID               string           `json:"id"`
	Domain           string           `json:"domain"`
	ActorID          string           `json:"actor_id"`
	TargetActorID    string           `json:"target_actor_id"`
	Type             string           `json:"type"` // "suspend", "block", "limit"
	Reason           string           `json:"reason"`
	Acknowledged     bool             `json:"acknowledged"`
	NotificationSent bool             `json:"notification_sent"`
	CreatedAt        time.Time        `json:"created_at"`
	SeveredAt        time.Time        `json:"severed_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
	AdminComment     string           `json:"admin_comment,omitempty"`
	LocalInstance    string           `json:"local_instance"`
	RemoteInstance   string           `json:"remote_instance"`
	Timestamp        time.Time        `json:"timestamp"`
	Reversible       bool             `json:"reversible"`
	Details          string           `json:"details,omitempty"` // Changed from map[string]interface{} to string
	EstimatedImpact  int              `json:"estimated_impact"`
	AffectedFollows  []AffectedFollow `json:"affected_follows,omitempty"`
}

// RelationshipRecord represents a record of actor relationships
type RelationshipRecord struct {
	PK            string                 `json:"PK"`
	SK            string                 `json:"SK"`
	GSI1PK        string                 `json:"GSI1PK"`
	GSI1SK        string                 `json:"GSI1SK"`
	ActivityID    string                 `json:"activity_id"`
	SourceID      string                 `json:"source_id"`
	TargetID      string                 `json:"target_id"`
	ActorID       string                 `json:"actor_id"`
	TargetActorID string                 `json:"target_actor_id"`
	Type          string                 `json:"type"` // "follow", "block", "mute", etc.
	Status        string                 `json:"status"`
	State         string                 `json:"state"`
	Action        string                 `json:"action"`
	PreviousState string                 `json:"previous_state,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
	Timestamp     time.Time              `json:"timestamp"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	Properties    map[string]interface{} `json:"properties,omitempty"`
	TTL           int64                  `json:"TTL,omitempty"`
}

// FederationTimeSeries represents time-series data for federation metrics
type FederationTimeSeries struct {
	ID             string                 `json:"id"`
	Domain         string                 `json:"domain"`
	Metric         string                 `json:"metric"` // "deliveries", "activities", "errors"
	Timestamp      time.Time              `json:"timestamp"`
	TimeWindow     string                 `json:"time_window"`
	Value          float64                `json:"value"`
	Period         string                 `json:"period"` // "hour", "day", "week"
	Metrics        map[string]interface{} `json:"metrics"`
	VolumeIn       int64                  `json:"volume_in"`
	VolumeOut      int64                  `json:"volume_out"`
	InboundVolume  int64                  `json:"inbound_volume"`
	OutboundVolume int64                  `json:"outbound_volume"`
	ErrorCount     int64                  `json:"error_count"`
	SuccessCount   int64                  `json:"success_count"`
	ErrorRate      float64                `json:"error_rate"`
	ResponseTime   float64                `json:"response_time"`
	ActivePeers    int64                  `json:"active_peers"`
	ActivityVolume int64                  `json:"activity_volume"`
	LatencyAvg     float64                `json:"latency_avg"`
	LatencyP95     float64                `json:"latency_p95"`
	LatencyP99     float64                `json:"latency_p99"`
	CreatedAt      time.Time              `json:"created_at"`
}

// DeliveryStatus represents the status of an activity delivery
type DeliveryStatus struct {
	ActivityID   string     `json:"activity_id"`
	TargetDomain string     `json:"target_domain"`
	Status       string     `json:"status"` // "pending", "delivered", "failed", "retrying"
	Attempts     int        `json:"attempts"`
	LastAttempt  time.Time  `json:"last_attempt"`
	NextRetry    *time.Time `json:"next_retry,omitempty"`
	Error        string     `json:"error,omitempty"`
	ErrorMsg     string     `json:"error_msg,omitempty"`
	DeliveredAt  *time.Time `json:"delivered_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// Hashtag represents a hashtag entity
type Hashtag struct {
	Name       string    `json:"name"`
	URL        string    `json:"url"`
	Uses       int       `json:"uses"`
	Accounts   int       `json:"accounts"`
	UsageCount int       `json:"usage_count"`
	FirstSeen  time.Time `json:"first_seen"`
	LastUsed   time.Time `json:"last_used"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	Trending   bool      `json:"trending"`
}

// HashtagHistoryEntry represents historical data for a hashtag
type HashtagHistoryEntry struct {
	Day        string `json:"day"`
	Date       string `json:"date"`
	Uses       string `json:"uses"`
	UsageCount string `json:"usage_count"`
	Accounts   string `json:"accounts"`
	UserCount  string `json:"user_count"`
}

// HashtagStats represents statistics for a hashtag
type HashtagStats struct {
	Name          string                `json:"name"`
	UsageCount    int                   `json:"usage_count"`
	UniqueUsers   int                   `json:"unique_users"`
	FirstSeen     time.Time             `json:"first_seen"`
	LastUsed      time.Time             `json:"last_used"`
	Trending      bool                  `json:"trending"`
	TrendingScore float64               `json:"trending_score"`
	TotalUses     int64                 `json:"total_uses"`
	TotalAccounts int64                 `json:"total_accounts"`
	History       []HashtagHistoryEntry `json:"history"`
}

// HashtagFollow is the storage-facing representation of a follow record.
type HashtagFollow struct {
	PK                   string    `json:"pk"`
	SK                   string    `json:"sk"`
	UserID               string    `json:"user_id"`
	Hashtag              string    `json:"hashtag"`
	NotificationsEnabled bool      `json:"notifications_enabled"`
	Muted                bool      `json:"muted"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// HashtagMute captures mute metadata, including optional expiration.
type HashtagMute struct {
	PK        string     `json:"pk"`
	SK        string     `json:"sk"`
	UserID    string     `json:"user_id"`
	Hashtag   string     `json:"hashtag"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	TTL       int64      `json:"ttl,omitempty"`
}

// HashtagNotificationSettings stores notification preferences for a user+hashtag pair.
type HashtagNotificationSettings struct {
	PK         string                 `json:"pk"`
	SK         string                 `json:"sk"`
	UserID     string                 `json:"user_id"`
	Hashtag    string                 `json:"hashtag"`
	Level      string                 `json:"level"`
	Muted      bool                   `json:"muted"`
	MutedUntil *time.Time             `json:"muted_until,omitempty"`
	Filters    []*NotificationFilter  `json:"filters,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// InstanceRule represents a rule for the instance
type InstanceRule struct {
	ID        string    `json:"id"`
	Text      string    `json:"text"`
	Priority  int       `json:"priority"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ActorRecord represents a lightweight actor record
type ActorRecord struct {
	PK          string    `json:"PK"`
	SK          string    `json:"SK"`
	ID          string    `json:"id"`
	Username    string    `json:"username"`
	Domain      string    `json:"domain"`
	ActorType   string    `json:"actor_type"`
	DisplayName string    `json:"display_name"`
	Avatar      string    `json:"avatar,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Tombstone represents a deleted object placeholder
type Tombstone struct {
	ID         string    `json:"id"`
	ObjectID   string    `json:"object_id"`
	ObjectType string    `json:"object_type"`
	Type       string    `json:"type"`
	FormerType string    `json:"former_type"`
	ActorID    string    `json:"actor_id"`
	Deleted    time.Time `json:"deleted"`
	DeletedBy  string    `json:"deleted_by"`
	DeletedAt  time.Time `json:"deleted_at"`
	Summary    string    `json:"summary,omitempty"`
	Reason     string    `json:"reason,omitempty"`
}

// ListMember represents a member of a list
type ListMember struct {
	ListID    string    `json:"list_id"`
	AccountID string    `json:"account_id"`
	ActorID   string    `json:"actor_id"`
	AddedBy   string    `json:"added_by"`
	AddedAt   time.Time `json:"added_at"`
}

// ModerationFilter represents content filtering rules
type ModerationFilter struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Type        string     `json:"type"` // "keyword", "domain", "pattern"
	Pattern     string     `json:"pattern"`
	Action      string     `json:"action"` // "hide", "warn", "block"
	Scope       []string   `json:"scope"`  // "home", "public", "notifications"
	Limit       int        `json:"limit"`
	MinScore    float64    `json:"min_score"`
	MaxScore    float64    `json:"max_score"`
	ContentType string     `json:"content_type"`
	StartTime   *time.Time `json:"start_time,omitempty"`
	EndTime     *time.Time `json:"end_time,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// ModerationQueueItem represents an item in the moderation queue
type ModerationQueueItem struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"` // "report", "appeal", "flag"
	Priority    int                    `json:"priority"`
	Status      string                 `json:"status"` // "pending", "reviewing", "resolved"
	TargetID    string                 `json:"target_id"`
	ReporterID  string                 `json:"reporter_id"`
	AssigneeID  string                 `json:"assignee_id,omitempty"`
	Reason      string                 `json:"reason"`
	Event       *ModerationEvent       `json:"event,omitempty"`
	ReviewCount int                    `json:"review_count"`
	Evidence    map[string]interface{} `json:"evidence,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	ReviewedAt  *time.Time             `json:"reviewed_at,omitempty"`
}

// ModerationReview represents a moderation review
type ModerationReview struct {
	ID          string                 `json:"id"`
	QueueItemID string                 `json:"queue_item_id"`
	EventID     string                 `json:"event_id"`
	ReviewerID  string                 `json:"reviewer_id"`
	Decision    string                 `json:"decision"` // "approve", "reject", "escalate"
	Comments    string                 `json:"comments"`
	Evidence    map[string]interface{} `json:"evidence,omitempty"`
	ReviewerRep float64                `json:"reviewer_rep"`
	Confidence  float64                `json:"confidence"`
	Action      string                 `json:"action"`
	Severity    string                 `json:"severity"`
	Note        string                 `json:"note"`
	Tags        []string               `json:"tags,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Created     time.Time              `json:"created"`
	CreatedAt   time.Time              `json:"created_at"`
}

// ModerationDecision represents a final moderation decision
type ModerationDecision struct {
	ID               string                 `json:"id"`
	QueueItemID      string                 `json:"queue_item_id"`
	EventID          string                 `json:"event_id"`
	ObjectID         string                 `json:"object_id"`
	DeciderID        string                 `json:"decider_id"`
	Decision         string                 `json:"decision"` // "no_action", "warn", "suspend", "delete"
	Action           string                 `json:"action"`
	Reason           string                 `json:"reason"`
	Appeal           bool                   `json:"appeal"` // whether this can be appealed
	Decided          time.Time              `json:"decided"`
	ConsensusScore   float64                `json:"consensus_score"`
	ReviewerCount    int                    `json:"reviewer_count"`
	TrustWeightTotal float64                `json:"trust_weight_total"`
	Reviews          []interface{}          `json:"reviews,omitempty"`
	Expires          *time.Time             `json:"expires,omitempty"`
	ExpiresAt        *time.Time             `json:"expires_at,omitempty"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
}

// ModerationPattern represents a moderation pattern for automated detection
type ModerationPattern struct {
	ID                 string     `json:"id"`
	Name               string     `json:"name"`
	Description        string     `json:"description"`
	Pattern            string     `json:"pattern"` // regex or keyword pattern
	Content            string     `json:"content"`
	Type               string     `json:"type"`     // "regex", "keyword", "ml_model"
	Severity           string     `json:"severity"` // "low", "medium", "high", "critical"
	Action             string     `json:"action"`   // "flag", "hide", "warn", "block"
	Enabled            bool       `json:"enabled"`
	Active             bool       `json:"active"`
	LastMatch          *time.Time `json:"last_match,omitempty"`
	CreatedBy          string     `json:"created_by"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	MatchCount         int        `json:"match_count"`
	FalsePositiveCount int        `json:"false_positive_count"`
}

// ModerationHistory represents historical moderation actions
type ModerationHistory struct {
	ID            string                    `json:"id"`
	ObjectID      string                    `json:"object_id"`
	TargetID      string                    `json:"target_id"`   // user or content ID
	TargetType    string                    `json:"target_type"` // "user", "status", "media"
	ModeratorID   string                    `json:"moderator_id"`
	Action        string                    `json:"action"` // "warn", "suspend", "delete", etc.
	Reason        string                    `json:"reason"`
	Duration      *time.Duration            `json:"duration,omitempty"`
	Events        []ModerationEvent         `json:"events,omitempty"`
	Decisions     []ModerationDecision      `json:"decisions,omitempty"`
	Timeline      []ModerationTimelineEntry `json:"timeline,omitempty"`
	Metadata      map[string]interface{}    `json:"metadata,omitempty"`
	CurrentStatus string                    `json:"current_status"`
	CreatedAt     time.Time                 `json:"created_at"`
}

// ModerationTimelineEntry represents an entry in the moderation timeline
type ModerationTimelineEntry struct {
	Timestamp   time.Time              `json:"timestamp"`
	Type        string                 `json:"type"` // "event", "decision", "review"
	ActorID     string                 `json:"actor_id"`
	Description string                 `json:"description"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// ModerationEventFilter represents filters for moderation events
type ModerationEventFilter struct {
	EventType   string     `json:"event_type,omitempty"`
	EventTypes  []string   `json:"event_types,omitempty"`
	Category    string     `json:"category,omitempty"`
	ActorID     string     `json:"actor_id,omitempty"`
	ObjectID    string     `json:"object_id,omitempty"`
	Moderators  []string   `json:"moderators,omitempty"`
	TargetTypes []string   `json:"target_types,omitempty"`
	MinSeverity *int       `json:"min_severity,omitempty"`
	StartDate   *time.Time `json:"start_date,omitempty"`
	EndDate     *time.Time `json:"end_date,omitempty"`
	Limit       int        `json:"limit,omitempty"`
	Offset      int        `json:"offset,omitempty"`
}

// ActionType represents the type of moderation action
type ActionType string

// EventType represents the type of moderation event
type EventType string

// Category represents a moderation category
type Category string

// Severity represents the severity level
type Severity string

const (
	// SeverityLow represents low severity issues
	SeverityLow Severity = "low"
	// SeverityMedium represents medium severity issues
	SeverityMedium Severity = "medium"
	// SeverityHigh represents high severity issues
	SeverityHigh Severity = "high"
	// SeverityCritical represents critical severity issues
	SeverityCritical Severity = "critical"
)

// EventType constants for moderation events
const (
	// EventTypeFlagged represents content flagging events
	EventTypeFlagged = "flagged"
	// EventTypeApproved represents content approval events
	EventTypeApproved = "approved"
	// EventTypeRejected represents content rejection events
	EventTypeRejected = "rejected"
	// EventTypeSuspended represents account suspension events
	EventTypeSuspended = "suspended"
	// EventTypeWarned represents warning events
	EventTypeWarned = "warned"
)

const (
	// ActionTypeNone represents no moderation action
	ActionTypeNone ActionType = "none"
	// ActionTypeWarn represents a warning action
	ActionTypeWarn ActionType = "warn"
	// ActionTypeWarning represents a warning action (alternative)
	ActionTypeWarning ActionType = "warning"
	// ActionTypeSilence represents silencing content
	ActionTypeSilence ActionType = "silence"
	// ActionTypeSuspend represents suspending an account
	ActionTypeSuspend ActionType = "suspend"
	// ActionTypeDelete represents deleting content
	ActionTypeDelete ActionType = "delete"
	// ActionTypeRestore represents restoring content or account
	ActionTypeRestore ActionType = "restore"
)

// ReviewerStats represents statistics for a moderator/reviewer
type ReviewerStats struct {
	ReviewerID        string         `json:"reviewer_id"`
	TotalReviews      int            `json:"total_reviews"`
	AcceptedReviews   int            `json:"accepted_reviews"`
	RejectedReviews   int            `json:"rejected_reviews"`
	EscalatedReviews  int            `json:"escalated_reviews"`
	AccurateReviews   int            `json:"accurate_reviews"`
	AccuracyRate      float64        `json:"accuracy_rate"`
	ReviewsByCategory map[string]int `json:"reviews_by_category"`
	JoinedAt          time.Time      `json:"joined_at"`
	TrustScore        float64        `json:"trust_score"`
	AvgReviewTime     int            `json:"avg_review_time"` // in minutes
	LastReviewAt      time.Time      `json:"last_review_at"`
	AccuracyScore     float64        `json:"accuracy_score"`
}

// Filter represents a content filter
type Filter struct {
	ID            string          `json:"id"`
	Username      string          `json:"username"`
	Title         string          `json:"title"`
	Context       []string        `json:"context"`        // "home", "notifications", "public", "thread"
	FilterAction  string          `json:"filter_action"`  // "warn", "hide", "blur", "silence", "limit_reach"
	Severity      string          `json:"severity"`       // "low", "medium", "high"
	MatchMode     string          `json:"match_mode"`     // "keyword", "regex", "semantic", "exact"
	CaseSensitive bool            `json:"case_sensitive"` // Case-sensitive matching
	ExpiresAt     *time.Time      `json:"expires_at,omitempty"`
	Irreversible  bool            `json:"irreversible"`
	WholeWord     bool            `json:"whole_word"`
	Keywords      []FilterKeyword `json:"keywords,omitempty"`
	Statuses      []FilterStatus  `json:"statuses,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// FilterKeyword represents a keyword in a filter
type FilterKeyword struct {
	ID           string    `json:"id"`
	FilterID     string    `json:"filter_id"`
	Keyword      string    `json:"keyword"`
	WholeWord    bool      `json:"whole_word"`
	IsRegex      bool      `json:"is_regex"`      // Whether keyword is a regex pattern
	MatchWeight  float64   `json:"match_weight"`  // Weight for scoring matches (0.0-1.0)
	ContextTypes []string  `json:"context_types"` // Specific contexts where this keyword applies
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// FilterStatus represents a status in a filter
type FilterStatus struct {
	ID        string    `json:"id"`
	FilterID  string    `json:"filter_id"`
	StatusID  string    `json:"status_id"`
	CreatedAt time.Time `json:"created_at"`
}

// FlagStatus represents the status of a content flag
type FlagStatus string

const (
	// FlagStatusPending represents a pending flag
	FlagStatusPending FlagStatus = "pending"
	// FlagStatusReviewed represents a reviewed flag
	FlagStatusReviewed FlagStatus = "reviewed"
	// FlagStatusApproved represents an approved flag
	FlagStatusApproved FlagStatus = "approved"
	// FlagStatusRejected represents a rejected flag
	FlagStatusRejected FlagStatus = "rejected"
	// FlagStatusEscalated represents an escalated flag
	FlagStatusEscalated FlagStatus = "escalated"
)

// ReportStatus represents the status of a moderation report
type ReportStatus string

const (
	// ReportStatusOpen represents an open report
	ReportStatusOpen ReportStatus = "open"
	// ReportStatusInProgress represents a report being processed
	ReportStatusInProgress ReportStatus = "in_progress"
	// ReportStatusResolved represents a resolved report
	ReportStatusResolved ReportStatus = "resolved"
	// ReportStatusClosed represents a closed report
	ReportStatusClosed ReportStatus = "closed"
	// ReportStatusAppealed represents an appealed report
	ReportStatusAppealed ReportStatus = "appealed"
	// ReportStatusRejected represents a rejected report
	ReportStatusRejected ReportStatus = "rejected"
)

// ReportStats represents statistics for moderation reports
type ReportStats struct {
	TotalReports      int            `json:"total_reports"`
	OpenReports       int            `json:"open_reports"`
	ResolvedReports   int            `json:"resolved_reports"`
	FalseReports      int            `json:"false_reports"`
	AvgResolutionTime int            `json:"avg_resolution_time"` // in hours
	ReportsByCategory map[string]int `json:"reports_by_category"`
	LastReportAt      *time.Time     `json:"last_report_at,omitempty"`
	LastUpdated       time.Time      `json:"last_updated"`
}

// NotificationFilter represents filters for notification queries
type NotificationFilter struct {
	Types        []string `json:"types,omitempty"`
	AccountID    string   `json:"account_id,omitempty"`
	MinID        string   `json:"min_id,omitempty"`
	MaxID        string   `json:"max_id,omitempty"`
	SinceID      string   `json:"since_id,omitempty"`
	Limit        int      `json:"limit,omitempty"`
	ExcludeTypes []string `json:"exclude_types,omitempty"`
}

// Notification represents a user notification
type Notification struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"` // "mention", "status", "reblog", "follow", etc.
	CreatedAt time.Time              `json:"created_at"`
	AccountID string                 `json:"account_id"` // who triggered the notification
	TargetID  string                 `json:"target_id"`  // target user
	StatusID  string                 `json:"status_id,omitempty"`
	Read      bool                   `json:"read"`
	Username  string                 `json:"username,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

// NotificationPreferences represents user notification preferences
type NotificationPreferences struct {
	Username      string `json:"username"`
	Email         bool   `json:"email"`
	Push          bool   `json:"push"`
	Mention       bool   `json:"mention"`
	Status        bool   `json:"status"`
	Reblog        bool   `json:"reblog"`
	Follow        bool   `json:"follow"`
	FollowRequest bool   `json:"follow_request"`
	Favourite     bool   `json:"favourite"`
	Poll          bool   `json:"poll"`
	// Additional fields for compatibility
	EmailEnabled    bool      `json:"email_enabled"`
	PushEnabled     bool      `json:"push_enabled"`
	FollowEnabled   bool      `json:"follow_enabled"`
	MentionEnabled  bool      `json:"mention_enabled"`
	ReblogEnabled   bool      `json:"reblog_enabled"`
	FavoriteEnabled bool      `json:"favorite_enabled"`
	PollEnabled     bool      `json:"poll_enabled"`
	UpdatedAt       time.Time `json:"updated_at"`
	Update          bool      `json:"update"`
	AdminSignUp     bool      `json:"admin_sign_up"`
	AdminReport     bool      `json:"admin_report"`
}

// PushSubscription represents a push notification subscription
type PushSubscription struct {
	ID        string                 `json:"id"`
	Username  string                 `json:"username"`
	Endpoint  string                 `json:"endpoint"`
	ServerKey string                 `json:"server_key"`
	Auth      string                 `json:"auth"`
	P256dh    string                 `json:"p256dh"`
	Alerts    PushSubscriptionAlerts `json:"alerts"`
	Policy    string                 `json:"policy"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// CollectionItem represents an item in an ActivityPub collection
type CollectionItem struct {
	CollectionID string    `json:"collection_id"`
	ItemID       string    `json:"item_id"`
	ItemType     string    `json:"item_type"` // "Note", "Activity", etc.
	Position     int       `json:"position"`
	AddedAt      time.Time `json:"added_at"`
	AddedBy      string    `json:"added_by,omitempty"`
}

// QuoteRelationship represents a quote relationship between statuses
type QuoteRelationship struct {
	ID             string     `json:"id"`
	QuoterNoteID   string     `json:"quoter_note_id"`
	TargetNoteID   string     `json:"target_note_id"`
	QuoterID       string     `json:"quoter_id"`
	TargetAuthorID string     `json:"target_author_id"`
	Timestamp      time.Time  `json:"timestamp"`
	Withdrawn      bool       `json:"withdrawn"`
	WithdrawnAt    *time.Time `json:"withdrawn_at,omitempty"`
	// Legacy fields for compatibility
	QuotingID string    `json:"quoting_id"`
	QuotedID  string    `json:"quoted_id"`
	CreatedAt time.Time `json:"created_at"`
}

// ThreadContext represents context information for a thread
type ThreadContext struct {
	StatusID    string    `json:"status_id"`
	Ancestors   []string  `json:"ancestors"`
	Descendants []string  `json:"descendants"`
	ThreadRoot  string    `json:"thread_root"`
	ReplyDepth  int       `json:"reply_depth"`
	LastUpdated time.Time `json:"last_updated"`
}

// QuotePermissions represents permissions for quoting content
type QuotePermissions struct {
	StatusID       string    `json:"status_id"`
	AuthorID       string    `json:"author_id"`
	AllowQuotes    bool      `json:"allow_quotes"`
	AllowPublic    bool      `json:"allow_public"`
	AllowFollowers bool      `json:"allow_followers"`
	AllowMentioned bool      `json:"allow_mentioned"`
	Conditions     []string  `json:"conditions,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Poll represents a poll attached to a status
type Poll struct {
	ID          string     `json:"id"`
	StatusID    string     `json:"status_id"`
	Question    string     `json:"question"`
	Options     []string   `json:"options"`
	Votes       []int      `json:"votes"`
	VotesCount  []int      `json:"votes_count"` // Votes per option
	VotersCount int        `json:"voters_count"`
	Multiple    bool       `json:"multiple"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	Expired     bool       `json:"expired"`
	CreatedBy   string     `json:"created_by"`
	HideTotals  bool       `json:"hide_totals"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// VAPIDKeys represents VAPID keys for push notifications
type VAPIDKeys struct {
	PublicKey  string    `json:"public_key"`
	PrivateKey string    `json:"private_key"`
	Subject    string    `json:"subject"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// SeveranceReason represents a reason for relationship severance
type SeveranceReason string

const (
	// SeveranceReasonDomainBlock represents severance due to domain blocking
	SeveranceReasonDomainBlock SeveranceReason = "domain_block"
	// SeveranceReasonAccountBlock represents severance due to account blocking
	SeveranceReasonAccountBlock SeveranceReason = "account_block"
	// SeveranceReasonAccountSuspend represents severance due to account suspension
	SeveranceReasonAccountSuspend SeveranceReason = "account_suspend"
	// SeveranceReasonModerationAction represents severance due to moderation action
	SeveranceReasonModerationAction SeveranceReason = "moderation_action"
	// SeveranceReasonPolicyViolation represents severance due to policy violation
	SeveranceReasonPolicyViolation SeveranceReason = "policy_violation"
)

// TrusteeConfig represents configuration for social recovery trustees
type TrusteeConfig struct {
	Username       string    `json:"username"`
	TrusteeID      string    `json:"trustee_id"`
	ActorID        string    `json:"actor_id"`
	TrusteeType    string    `json:"trustee_type"` // "social", "hardware", "institutional"
	RequiredShares int       `json:"required_shares"`
	TotalShares    int       `json:"total_shares"`
	BackupMethods  []string  `json:"backup_methods"`
	ContactInfo    string    `json:"contact_info,omitempty"`
	AddedAt        time.Time `json:"added_at"`
	Confirmed      bool      `json:"confirmed"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	Status         string    `json:"status"` // "active", "inactive", "pending"
}

// SocialRecoveryRequest represents a social recovery request
type SocialRecoveryRequest struct {
	ID            string                 `json:"id"`
	Username      string                 `json:"username"`
	RequestorID   string                 `json:"requestor_id"`
	RecoveryType  string                 `json:"recovery_type"` // "password", "account", "key"
	Status        string                 `json:"status"`        // "pending", "approved", "rejected", "completed"
	RequiredVotes int                    `json:"required_votes"`
	ReceivedVotes int                    `json:"received_votes"`
	TrusteeVotes  []string               `json:"trustee_votes"`
	RequestData   map[string]interface{} `json:"request_data,omitempty"`
	InitiatedAt   time.Time              `json:"initiated_at"`
	RecoveryToken string                 `json:"recovery_token,omitempty"`
	ExpiresAt     time.Time              `json:"expires_at"`
	CreatedAt     time.Time              `json:"created_at"`
	CompletedAt   *time.Time             `json:"completed_at,omitempty"`
}

// RecoveryCodeItem represents a recovery code item
type RecoveryCodeItem struct {
	Username  string     `json:"username"`
	Code      string     `json:"code"`
	CodeHash  string     `json:"code_hash"`
	Position  int        `json:"position"`
	Used      bool       `json:"used"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt time.Time  `json:"expires_at"`
}

// RelayState represents the state of a relay connection
type RelayState struct {
	URL        string                 `json:"url"`
	Active     bool                   `json:"active"`
	Status     string                 `json:"status"` // "pending", "accepted", "rejected", "error"
	LastPing   time.Time              `json:"last_ping"`
	LastError  string                 `json:"last_error,omitempty"`
	ErrorCount int                    `json:"error_count"`
	RetryCount int                    `json:"retry_count"`
	NextRetry  *time.Time             `json:"next_retry,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
}

// StatusSearchOptions represents options for status search
type StatusSearchOptions struct {
	Query          string     `json:"query"`
	AccountID      string     `json:"account_id,omitempty"`
	MaxID          string     `json:"max_id,omitempty"` // Legacy pagination support
	MinID          string     `json:"min_id,omitempty"` // Legacy pagination support
	Limit          int        `json:"limit,omitempty"`
	Offset         int        `json:"offset,omitempty"` // Legacy pagination support
	OnlyMedia      bool       `json:"only_media"`
	ExcludeReplies bool       `json:"exclude_replies"`
	ExcludeReblogs bool       `json:"exclude_reblogs"`
	LanguageCode   string     `json:"language_code,omitempty"`
	DateFrom       *time.Time `json:"date_from,omitempty"`
	DateTo         *time.Time `json:"date_to,omitempty"`
	LocalOnly      bool       `json:"local_only"`

	// New cursor-based pagination fields
	Cursor    string `json:"cursor,omitempty"`     // Cursor for pagination
	SortOrder string `json:"sort_order,omitempty"` // Sort order: "relevance", "time_asc", "time_desc"
}

// AuditLog represents an audit log entry for admin actions
type AuditLog struct {
	ID         string    `json:"id"`
	AdminID    string    `json:"admin_id"`          // Who performed the action
	AdminRole  string    `json:"admin_role"`        // admin or moderator
	Action     string    `json:"action"`            // suspend, silence, resolve_report, etc.
	TargetType string    `json:"target_type"`       // account, status, report, domain
	TargetID   string    `json:"target_id"`         // ID of the target
	Reason     string    `json:"reason,omitempty"`  // Reason for action
	Details    any       `json:"details,omitempty"` // Additional details
	IPAddress  string    `json:"ip_address,omitempty"`
	UserAgent  string    `json:"user_agent,omitempty"`
	RequestID  string    `json:"request_id,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
	CreatedAt  time.Time `json:"created_at"`
}
