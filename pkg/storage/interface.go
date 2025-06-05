package storage

import (
	"context"
	"time"

	"github.com/aron23/lesser/pkg/activitypub"
)

// Storage defines the interface for data storage operations
type Storage interface {
	// Actor operations
	CreateActor(ctx context.Context, actor *activitypub.Actor, privateKey string) error
	GetActor(ctx context.Context, username string) (*activitypub.Actor, error)
	GetActorPrivateKey(ctx context.Context, username string) (string, error)
	UpdateActor(ctx context.Context, actor *activitypub.Actor) error
	DeleteActor(ctx context.Context, username string) error
	SearchAccounts(ctx context.Context, query string, limit int, followingOnly bool, offset int) ([]*activitypub.Actor, error)
	GetSearchSuggestions(ctx context.Context, prefix string) ([]SearchSuggestion, error)

	// Activity operations
	CreateActivity(ctx context.Context, activity *activitypub.Activity) error
	GetActivity(ctx context.Context, id string) (*activitypub.Activity, error)
	GetOutboxActivities(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Activity, string, error)
	GetInboxActivities(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Activity, string, error)

	// Object operations
	CreateObject(ctx context.Context, object interface{}) error
	GetObject(ctx context.Context, id string) (interface{}, error)
	UpdateObject(ctx context.Context, object interface{}) error
	DeleteObject(ctx context.Context, id string) error
	GetObjectsByActor(ctx context.Context, actorID string, cursor string, limit int) ([]interface{}, string, error)

	// Relationship operations
	CreateFollow(ctx context.Context, followerUsername, followedUsername, followActivityID string) error
	AcceptFollow(ctx context.Context, followerUsername, followedUsername string) error
	RejectFollow(ctx context.Context, followerUsername, followedUsername string) error
	RemoveFollow(ctx context.Context, followerUsername, followedUsername string) error
	GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
	GetFollowing(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
	IsFollowing(ctx context.Context, followerUsername, followedUsername string) (bool, error)

	// Collection operations
	GetCollection(ctx context.Context, username, collectionType string, limit int, cursor string) (*activitypub.OrderedCollectionPage, error)

	// OAuth operations
	CreateAuthorizationCode(ctx context.Context, code *AuthorizationCode) error
	GetAuthorizationCode(ctx context.Context, code string) (*AuthorizationCode, error)
	DeleteAuthorizationCode(ctx context.Context, code string) error
	CreateRefreshToken(ctx context.Context, token *RefreshToken) error
	GetRefreshToken(ctx context.Context, token string) (*RefreshToken, error)
	DeleteRefreshToken(ctx context.Context, token string) error

	// User operations
	CreateUser(ctx context.Context, user *User) error
	GetUser(ctx context.Context, username string) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	UpdateUser(ctx context.Context, username string, updates map[string]interface{}) error
	DeleteUser(ctx context.Context, username string) error
	ListUsers(ctx context.Context, limit int32, cursor string) ([]*User, string, error)

	// OAuth Client operations
	CreateOAuthClient(ctx context.Context, client *OAuthClient) error
	GetOAuthClient(ctx context.Context, clientID string) (*OAuthClient, error)
	UpdateOAuthClient(ctx context.Context, clientID string, updates map[string]interface{}) error
	DeleteOAuthClient(ctx context.Context, clientID string) error
	ListOAuthClients(ctx context.Context, limit int32, cursor string) ([]*OAuthClient, string, error)

	// Like operations
	CreateLike(ctx context.Context, like *Like) error
	GetLike(ctx context.Context, actor, object string) (*Like, error)
	DeleteLike(ctx context.Context, actor, object string) error
	GetObjectLikes(ctx context.Context, objectID string, limit int, cursor string) ([]*Like, string, error)
	GetActorLikes(ctx context.Context, actorID string, limit int, cursor string) ([]*Like, string, error)
	CountObjectLikes(ctx context.Context, objectID string) (int, error)

	// Announce operations
	CreateAnnounce(ctx context.Context, announce *Announce) error
	GetAnnounce(ctx context.Context, actor, object string) (*Announce, error)
	DeleteAnnounce(ctx context.Context, actor, object string) error
	GetObjectAnnounces(ctx context.Context, objectID string, limit int, cursor string) ([]*Announce, string, error)
	GetActorAnnounces(ctx context.Context, actorID string, limit int, cursor string) ([]*Announce, string, error)
	CountObjectAnnounces(ctx context.Context, objectID string) (int, error)

	// Delete/Tombstone operations
	TombstoneObject(ctx context.Context, objectID string, deletedBy string) error
	GetTombstone(ctx context.Context, objectID string) (*Tombstone, error)
	CascadeDeleteLikes(ctx context.Context, objectID string) error
	CascadeDeleteAnnounces(ctx context.Context, objectID string) error

	// Update history operations
	CreateUpdateHistory(ctx context.Context, history *UpdateHistory) error
	GetUpdateHistory(ctx context.Context, objectID string, limit int) ([]*UpdateHistory, error)

	// Block operations
	CreateBlock(ctx context.Context, block *Block) error
	GetBlock(ctx context.Context, actor, blockedActor string) (*Block, error)
	DeleteBlock(ctx context.Context, actor, blockedActor string) error
	GetBlockedActors(ctx context.Context, actor string, limit int, cursor string) ([]*Block, string, error)
	GetBlockedByActors(ctx context.Context, actor string, limit int, cursor string) ([]*Block, string, error)
	IsBlocked(ctx context.Context, actor, targetActor string) (bool, error)
	IsBlockedBidirectional(ctx context.Context, actor1, actor2 string) (bool, error)

	// Flag operations (content moderation)
	CreateFlag(ctx context.Context, flag *Flag) error
	GetFlag(ctx context.Context, id string) (*Flag, error)
	GetFlagsByObject(ctx context.Context, objectID string, limit int, cursor string) ([]*Flag, string, error)
	GetFlagsByActor(ctx context.Context, actorID string, limit int, cursor string) ([]*Flag, string, error)
	GetPendingFlags(ctx context.Context, limit int, cursor string) ([]*Flag, string, error)
	UpdateFlagStatus(ctx context.Context, id string, status FlagStatus, reviewedBy string, reviewNote string) error
	CountPendingFlags(ctx context.Context) (int, error)

	// Move operations (account migration)
	CreateMove(ctx context.Context, move *Move) error
	GetMove(ctx context.Context, actor string) (*Move, error)
	GetMoveByTarget(ctx context.Context, target string) ([]*Move, error)
	HasMovedFrom(ctx context.Context, oldActor, newActor string) (bool, error)

	// Collection operations (Add/Remove activities)
	AddToCollection(ctx context.Context, collection string, item *CollectionItem) error
	RemoveFromCollection(ctx context.Context, collection string, itemID string) error
	GetCollectionItems(ctx context.Context, collection string, limit int, cursor string) ([]*CollectionItem, string, error)
	IsInCollection(ctx context.Context, collection string, itemID string) (bool, error)
	CountCollectionItems(ctx context.Context, collection string) (int, error)

	// Timeline operations
	WriteToTimeline(ctx context.Context, timeline *TimelineEntry) error
	WriteToTimelines(ctx context.Context, entries []*TimelineEntry) error
	GetHomeTimeline(ctx context.Context, username string, limit int, cursor string) ([]*TimelineEntry, string, error)
	GetPublicTimeline(ctx context.Context, local bool, limit int, cursor string) ([]*TimelineEntry, string, error)
	GetListTimeline(ctx context.Context, listID string, limit int, cursor string) ([]*TimelineEntry, string, error)
	GetHashtagTimeline(ctx context.Context, hashtag string, local bool, limit int, cursor string) ([]*TimelineEntry, string, error)
	DeleteFromTimeline(ctx context.Context, timelineType, timelineID, entryID string) error
	DeleteExpiredTimelineEntries(ctx context.Context, before time.Time) error
	FanOutPost(ctx context.Context, activity *activitypub.Activity) error // Fan out posts to timelines

	// Instance configuration operations
	GetInstanceRules(ctx context.Context) ([]InstanceRule, error)
	SetInstanceRules(ctx context.Context, rules []InstanceRule) error
	GetExtendedDescription(ctx context.Context) (string, time.Time, error)
	SetExtendedDescription(ctx context.Context, description string) error

	// Bookmark operations
	CreateBookmark(ctx context.Context, username, objectID string) error
	RemoveBookmark(ctx context.Context, username, objectID string) error
	GetBookmarks(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
	IsBookmarked(ctx context.Context, username, objectID string) (bool, error)

	// Conversation operations
	CreateConversation(ctx context.Context, conversation *Conversation) error
	GetConversation(ctx context.Context, id string) (*Conversation, error)
	GetConversationByParticipants(ctx context.Context, participants []string) (*Conversation, error)
	UpdateConversationLastStatus(ctx context.Context, id, lastStatusID string) error
	MarkConversationRead(ctx context.Context, id, username string) error
	DeleteConversation(ctx context.Context, id string) error
	GetUserConversations(ctx context.Context, username string, limit int, cursor string) ([]*Conversation, string, error)
	AddParticipantToConversation(ctx context.Context, conversationID, participantID string) error

	// List operations
	CreateList(ctx context.Context, username, title, repliesPolicy string) (*List, error)
	GetList(ctx context.Context, listID string) (*List, error)
	GetListsForUser(ctx context.Context, username string) ([]*List, error)
	UpdateList(ctx context.Context, listID string, updates map[string]interface{}) error
	DeleteList(ctx context.Context, listID string) error
	AddAccountsToList(ctx context.Context, listID string, accountIDs []string) error
	RemoveAccountsFromList(ctx context.Context, listID string, accountIDs []string) error
	GetListAccounts(ctx context.Context, listID string) ([]string, error)
	IsAccountInList(ctx context.Context, listID, accountID string) (bool, error)
	GetListsContainingAccount(ctx context.Context, accountID, username string) ([]*List, error)

	// Notification operations
	CreateNotification(ctx context.Context, notification *Notification) error
	GetNotification(ctx context.Context, id string) (*Notification, error)
	GetNotifications(ctx context.Context, username string, limit int, cursor string) ([]*Notification, string, error)
	GetNotificationsFiltered(ctx context.Context, username string, filter *NotificationFilter) ([]*Notification, string, error)
	MarkNotificationAsRead(ctx context.Context, id string) error
	MarkAllNotificationsAsRead(ctx context.Context, username string) error
	DeleteNotification(ctx context.Context, id string) error
	ClearNotifications(ctx context.Context, username string) error
	CountUnreadNotifications(ctx context.Context, username string) (int, error)

	// Remote actor caching operations
	CacheRemoteActor(ctx context.Context, handle string, actor *activitypub.Actor, ttl time.Duration) error
	GetCachedRemoteActor(ctx context.Context, handle string) (*activitypub.Actor, error)

	// Push notification operations
	CreatePushSubscription(ctx context.Context, username string, subscription *PushSubscription) error
	GetPushSubscription(ctx context.Context, username, subscriptionID string) (*PushSubscription, error)
	GetUserPushSubscriptions(ctx context.Context, username string) ([]*PushSubscription, error)
	UpdatePushSubscription(ctx context.Context, username, subscriptionID string, alerts PushSubscriptionAlerts) error
	DeletePushSubscription(ctx context.Context, username, subscriptionID string) error
	DeleteAllPushSubscriptions(ctx context.Context, username string) error

	// VAPID key operations
	GetVAPIDKeys(ctx context.Context) (*VAPIDKeys, error)
	SetVAPIDKeys(ctx context.Context, keys *VAPIDKeys) error

	// Poll operations
	CreatePoll(ctx context.Context, poll *Poll) error
	GetPoll(ctx context.Context, pollID string) (*Poll, error)
	GetPollByStatusID(ctx context.Context, statusID string) (*Poll, error)
	VoteOnPoll(ctx context.Context, pollID string, voterID string, choices []int) error
	GetPollVotes(ctx context.Context, pollID string) (map[string][]int, error)
	HasUserVoted(ctx context.Context, pollID string, userID string) (bool, []int, error)

	// Mute operations
	CreateMute(ctx context.Context, mute *Mute) error
	GetMute(ctx context.Context, actor, mutedActor string) (*Mute, error)
	DeleteMute(ctx context.Context, actor, mutedActor string) error
	GetMutedActors(ctx context.Context, actor string, limit int, cursor string) ([]*Mute, string, error)
	IsMuted(ctx context.Context, actor, targetActor string) (bool, error)

	// Filter operations (v2)
	CreateFilter(ctx context.Context, filter *Filter) error
	GetFilter(ctx context.Context, filterID string) (*Filter, error)
	GetFiltersForUser(ctx context.Context, username string) ([]*Filter, error)
	UpdateFilter(ctx context.Context, filterID string, updates map[string]interface{}) error
	DeleteFilter(ctx context.Context, filterID string) error

	// Filter keyword operations
	AddFilterKeyword(ctx context.Context, filterID string, keyword *FilterKeyword) error
	GetFilterKeywords(ctx context.Context, filterID string) ([]*FilterKeyword, error)
	UpdateFilterKeyword(ctx context.Context, keywordID string, updates map[string]interface{}) error
	DeleteFilterKeyword(ctx context.Context, keywordID string) error

	// Filter status operations
	AddFilterStatus(ctx context.Context, filterID string, status *FilterStatus) error
	GetFilterStatuses(ctx context.Context, filterID string) ([]*FilterStatus, error)
	DeleteFilterStatus(ctx context.Context, statusID string) error
}

// User represents a user account in the system
type User struct {
	Username     string    `dynamodbav:"username"`
	Email        string    `dynamodbav:"email"`
	PasswordHash string    `dynamodbav:"password_hash"`
	CreatedAt    time.Time `dynamodbav:"created_at"`
	UpdatedAt    time.Time `dynamodbav:"updated_at"`
	Approved     bool      `dynamodbav:"approved"`
	Suspended    bool      `dynamodbav:"suspended"`
	Role         string    `dynamodbav:"role"` // user, moderator, admin
	Locale       string    `dynamodbav:"locale,omitempty"`
}

// OAuthClient represents an OAuth client application
type OAuthClient struct {
	ClientID     string    `dynamodbav:"client_id"`
	ClientSecret string    `dynamodbav:"client_secret"`
	Name         string    `dynamodbav:"name"`
	Website      string    `dynamodbav:"website,omitempty"`
	RedirectURIs []string  `dynamodbav:"redirect_uris"`
	Scopes       []string  `dynamodbav:"scopes,omitempty"`
	CreatedAt    time.Time `dynamodbav:"created_at"`
	UpdatedAt    time.Time `dynamodbav:"updated_at,omitempty"`
}

// Like represents a Like activity
type Like struct {
	Actor     string    `dynamodbav:"actor"`      // Who liked
	Object    string    `dynamodbav:"object"`     // What was liked
	ID        string    `dynamodbav:"id"`         // Like activity ID
	Published time.Time `dynamodbav:"published"`  // When it was liked
	CreatedAt time.Time `dynamodbav:"created_at"` // When stored in DB
}

// Announce represents an Announce activity (boost/repost)
type Announce struct {
	Actor     string    `dynamodbav:"actor"`        // Who announced
	Object    string    `dynamodbav:"object"`       // What was announced
	ID        string    `dynamodbav:"id"`           // Announce activity ID
	Published time.Time `dynamodbav:"published"`    // When it was announced
	CreatedAt time.Time `dynamodbav:"created_at"`   // When stored in DB
	To        []string  `dynamodbav:"to,omitempty"` // Audience
	CC        []string  `dynamodbav:"cc,omitempty"` // CC audience
}

// Tombstone represents a deleted object
type Tombstone struct {
	ID         string    `dynamodbav:"id"`                // Original object ID
	Type       string    `dynamodbav:"type"`              // Always "Tombstone"
	FormerType string    `dynamodbav:"formerType"`        // Original object type
	Deleted    time.Time `dynamodbav:"deleted"`           // When it was deleted
	DeletedBy  string    `dynamodbav:"deletedBy"`         // Actor who deleted it
	Summary    string    `dynamodbav:"summary,omitempty"` // Optional deletion reason
}

// Block represents a block relationship
type Block struct {
	Actor     string    // The actor doing the blocking
	Object    string    // The actor being blocked
	ID        string    // The block activity ID
	Published time.Time // When the block was created
	CreatedAt time.Time // Database timestamp
}

// UpdateHistory represents the edit history of an object
type UpdateHistory struct {
	ObjectID      string    `dynamodbav:"objectId"`          // The object that was updated
	Version       int       `dynamodbav:"version"`           // Version number (1 is original)
	UpdatedAt     time.Time `dynamodbav:"updatedAt"`         // When the update occurred
	UpdatedBy     string    `dynamodbav:"updatedBy"`         // Actor who made the update
	PreviousState string    `dynamodbav:"previousState"`     // JSON of previous state
	Summary       string    `dynamodbav:"summary,omitempty"` // Edit summary
}

// ActorRecord represents an actor stored in DynamoDB
type ActorRecord struct {
	PK         string `dynamodbav:"PK"`
	SK         string `dynamodbav:"SK"`
	Actor      *activitypub.Actor
	PrivateKey string    `dynamodbav:"PrivateKey,omitempty"`
	CreatedAt  time.Time `dynamodbav:"CreatedAt"`
	UpdatedAt  time.Time `dynamodbav:"UpdatedAt"`
}

// ActivityRecord represents an activity stored in DynamoDB
type ActivityRecord struct {
	PK        string `dynamodbav:"PK"`
	SK        string `dynamodbav:"SK"`
	GSI1PK    string `dynamodbav:"GSI1PK,omitempty"`
	GSI1SK    string `dynamodbav:"GSI1SK,omitempty"`
	Activity  *activitypub.Activity
	CreatedAt time.Time `dynamodbav:"CreatedAt"`
}

// RelationshipRecord represents a follow relationship in DynamoDB
type RelationshipRecord struct {
	PK         string    `dynamodbav:"PK"`
	SK         string    `dynamodbav:"SK"`
	GSI1PK     string    `dynamodbav:"GSI1PK"`
	GSI1SK     string    `dynamodbav:"GSI1SK"`
	ActivityID string    `dynamodbav:"ActivityID"`
	State      string    `dynamodbav:"State"` // pending, accepted, rejected
	CreatedAt  time.Time `dynamodbav:"CreatedAt"`
	UpdatedAt  time.Time `dynamodbav:"UpdatedAt"`
}

// ObjectRecord represents an object stored in DynamoDB
type ObjectRecord struct {
	PK        string      `dynamodbav:"PK"`
	SK        string      `dynamodbav:"SK"`
	Type      string      `dynamodbav:"Type"`
	Object    interface{} `dynamodbav:"Object"`
	CreatedAt time.Time   `dynamodbav:"CreatedAt"`
	UpdatedAt time.Time   `dynamodbav:"UpdatedAt"`
}

// AuthorizationCode represents an OAuth authorization code
type AuthorizationCode struct {
	Code          string    `dynamodbav:"Code"`
	ClientID      string    `dynamodbav:"ClientID"`
	Username      string    `dynamodbav:"Username"`
	CodeChallenge string    `dynamodbav:"CodeChallenge"`
	ExpiresAt     time.Time `dynamodbav:"ExpiresAt"`
	Scopes        []string  `dynamodbav:"Scopes"`
}

// RefreshToken represents an OAuth refresh token
type RefreshToken struct {
	Token     string    `dynamodbav:"Token"`
	ClientID  string    `dynamodbav:"ClientID"`
	Username  string    `dynamodbav:"Username"`
	ExpiresAt time.Time `dynamodbav:"ExpiresAt"`
	Scopes    []string  `dynamodbav:"Scopes"`
}

// AuthorizationCodeRecord represents an authorization code stored in DynamoDB
type AuthorizationCodeRecord struct {
	PK        string             `dynamodbav:"PK"`
	SK        string             `dynamodbav:"SK"`
	Code      *AuthorizationCode `dynamodbav:"Code"`
	CreatedAt time.Time          `dynamodbav:"CreatedAt"`
}

// RefreshTokenRecord represents a refresh token stored in DynamoDB
type RefreshTokenRecord struct {
	PK        string        `dynamodbav:"PK"`
	SK        string        `dynamodbav:"SK"`
	Token     *RefreshToken `dynamodbav:"Token"`
	CreatedAt time.Time     `dynamodbav:"CreatedAt"`
}

// InstanceRule represents a server rule
type InstanceRule struct {
	ID   string `json:"id" dynamodbav:"ID"`
	Text string `json:"text" dynamodbav:"Text"`
}

// Constants for DynamoDB key patterns
const (
	// Actor keys
	ActorPKPrefix = "ACTOR#"
	ActorSK       = "PROFILE"

	// Activity keys
	ActivitySKPrefix  = "ACTIVITY#"
	InboxGSI1PKPrefix = "INBOX#"

	// Relationship keys
	FollowPKPrefix    = "FOLLOW#"
	FollowingSKPrefix = "FOLLOWING#"
	FollowerSKPrefix  = "FOLLOWER#"

	// Object keys
	ObjectPKPrefix  = "OBJECT#"
	VersionSKPrefix = "VERSION#"

	// OAuth keys
	AuthCodePKPrefix     = "AUTHCODE#"
	AuthCodeSK           = "CODE"
	RefreshTokenPKPrefix = "REFRESHTOKEN#"
	RefreshTokenSK       = "TOKEN"

	// Relationship states
	RelationshipPending  = "pending"
	RelationshipAccepted = "accepted"
	RelationshipRejected = "rejected"
)

// FlagStatus represents the status of a flag
type FlagStatus string

const (
	FlagStatusPending   FlagStatus = "pending"
	FlagStatusReviewed  FlagStatus = "reviewed"
	FlagStatusResolved  FlagStatus = "resolved"
	FlagStatusDismissed FlagStatus = "dismissed"
)

// Flag represents a Flag activity for content moderation
type Flag struct {
	ID         string     // The flag activity ID
	Actor      string     // Who flagged
	Object     []string   // What was flagged (can be multiple objects)
	Content    string     // Reason/description for the flag
	Published  time.Time  // When it was flagged
	Status     FlagStatus // Current status of the flag
	ReviewedBy string     // Moderator who reviewed (if reviewed)
	ReviewedAt *time.Time // When it was reviewed
	ReviewNote string     // Note from reviewer
	CreatedAt  time.Time  // Database timestamp
}

// Move represents a Move activity for account migration
type Move struct {
	ID        string    // The move activity ID
	Actor     string    // The old account moving
	Target    string    // The new account location
	Published time.Time // When the move was announced
	CreatedAt time.Time // Database timestamp
}

// CollectionItem represents an item in a collection (for Add/Remove activities)
type CollectionItem struct {
	Collection string    // The collection ID (e.g., featured, likes, etc.)
	ItemID     string    // The item being added/removed
	ItemType   string    // Type of the item (Note, Article, etc.)
	AddedBy    string    // Who added the item
	AddedAt    time.Time // When it was added
	Position   int       // Optional position in ordered collections
}

// TimelineEntry represents an entry in a user's timeline
type TimelineEntry struct {
	TimelineType string    // HOME, PUBLIC, LIST
	TimelineID   string    // Username for HOME, LOCAL/FEDERATED for PUBLIC, list ID for LIST
	EntryID      string    // Unique ID for this entry (usually timestamp + post ID)
	PostID       string    // The actual post/object ID
	ActorID      string    // Who created the post
	ActorHandle  string    // Actor's handle for quick display
	Content      string    // First 500 chars for preview
	ContentType  string    // Note, Article, etc.
	HasMedia     bool      // Quick flag for media
	IsReply      bool      // Is this a reply?
	InReplyTo    string    // ID of post being replied to
	IsBoost      bool      // Is this a boost/announce?
	BoostedBy    string    // Who boosted it (if applicable)
	Visibility   string    // public, unlisted, private, direct
	Language     string    // Language code
	Sensitive    bool      // Content warning flag
	SpoilerText  string    // Content warning text
	CreatedAt    time.Time // When the post was created
	TimelineAt   time.Time // When it was added to timeline (for sorting)
	ExpiresAt    time.Time // TTL for auto-deletion
}

// Conversation represents a direct message conversation
type Conversation struct {
	ID           string    `dynamodbav:"id"`
	Participants []string  `dynamodbav:"participants"` // Actor IDs
	LastStatusID string    `dynamodbav:"last_status_id"`
	CreatedAt    time.Time `dynamodbav:"created_at"`
	UpdatedAt    time.Time `dynamodbav:"updated_at"`
}

// ConversationStatus tracks read status per user
type ConversationStatus struct {
	ConversationID string    `dynamodbav:"conversation_id"`
	UserID         string    `dynamodbav:"user_id"`
	Unread         bool      `dynamodbav:"unread"`
	LastReadAt     time.Time `dynamodbav:"last_read_at"`
}

// List represents a user-created list for organizing followed accounts
type List struct {
	ID            string    `dynamodbav:"id"`
	Username      string    `dynamodbav:"username"` // Owner of the list
	Title         string    `dynamodbav:"title"`
	RepliesPolicy string    `dynamodbav:"replies_policy"` // "followed", "list", or "none"
	CreatedAt     time.Time `dynamodbav:"created_at"`
	UpdatedAt     time.Time `dynamodbav:"updated_at"`
}

// ListMember represents membership of an account in a list
type ListMember struct {
	ListID    string    `dynamodbav:"list_id"`
	AccountID string    `dynamodbav:"account_id"`
	AddedAt   time.Time `dynamodbav:"added_at"`
}

// Notification represents a notification to a user
type Notification struct {
	ID        string    `dynamodbav:"id"`
	Type      string    `dynamodbav:"type"`                // follow, mention, favourite, reblog
	Username  string    `dynamodbav:"username"`            // Recipient of the notification
	AccountID string    `dynamodbav:"account_id"`          // Who triggered the notification
	StatusID  string    `dynamodbav:"status_id,omitempty"` // Related status (if any)
	Read      bool      `dynamodbav:"read"`
	CreatedAt time.Time `dynamodbav:"created_at"`
}

// NotificationFilter represents parameters for filtering notifications
type NotificationFilter struct {
	Types        []string // Filter by notification types
	ExcludeTypes []string // Exclude specific notification types
	AccountID    string   // Filter by account
	Limit        int      // Maximum number to return
	MinID        string   // Return results newer than this ID
	MaxID        string   // Return results older than this ID
	SinceID      string   // Return results newer than this ID (for polling)
}

// SearchSuggestion represents a search suggestion
type SearchSuggestion struct {
	Type        string `json:"type"`     // "username", "display_name", "hashtag"
	Value       string `json:"value"`    // The suggestion text
	DisplayText string `json:"display"`  // What to show to the user
	Username    string `json:"username"` // For account suggestions
}

// PushSubscription represents a web push subscription
type PushSubscription struct {
	ID        string                 `dynamodbav:"id"`
	Username  string                 `dynamodbav:"username"`
	Endpoint  string                 `dynamodbav:"endpoint"`
	P256dh    string                 `dynamodbav:"p256dh"`
	Auth      string                 `dynamodbav:"auth"`
	Alerts    PushSubscriptionAlerts `dynamodbav:"alerts"`
	Policy    string                 `dynamodbav:"policy,omitempty"`
	CreatedAt time.Time              `dynamodbav:"created_at"`
	UpdatedAt time.Time              `dynamodbav:"updated_at"`
}

// PushSubscriptionAlerts represents which events trigger push notifications
type PushSubscriptionAlerts struct {
	Follow        bool `dynamodbav:"follow"`
	Favourite     bool `dynamodbav:"favourite"`
	Reblog        bool `dynamodbav:"reblog"`
	Mention       bool `dynamodbav:"mention"`
	Poll          bool `dynamodbav:"poll"`
	FollowRequest bool `dynamodbav:"follow_request"`
	Status        bool `dynamodbav:"status"`
	Update        bool `dynamodbav:"update"`
	AdminSignUp   bool `dynamodbav:"admin_sign_up"`
	AdminReport   bool `dynamodbav:"admin_report"`
}

// VAPIDKeys represents the VAPID keys for web push
type VAPIDKeys struct {
	PublicKey  string    `dynamodbav:"public_key"`
	PrivateKey string    `dynamodbav:"private_key"`
	Subject    string    `dynamodbav:"subject"`
	CreatedAt  time.Time `dynamodbav:"created_at"`
}

// Poll represents a poll in the storage layer
type Poll struct {
	ID          string           `dynamodbav:"id"`
	StatusID    string           `dynamodbav:"statusId"`   // The status this poll belongs to
	CreatedBy   string           `dynamodbav:"createdBy"`  // Actor ID who created the poll
	Options     []string         `dynamodbav:"options"`    // Poll options
	Multiple    bool             `dynamodbav:"multiple"`   // Allow multiple choices
	HideTotals  bool             `dynamodbav:"hideTotals"` // Hide vote counts until poll ends
	ExpiresAt   time.Time        `dynamodbav:"expiresAt"`  // When the poll expires
	CreatedAt   time.Time        `dynamodbav:"createdAt"`
	UpdatedAt   time.Time        `dynamodbav:"updatedAt"`
	VotesCount  int              `dynamodbav:"votesCount"`  // Total number of votes
	VotersCount int              `dynamodbav:"votersCount"` // Total number of voters
	Votes       map[string][]int `dynamodbav:"votes"`       // Map of voter ID to option indices
}

// Mute represents a mute relationship
type Mute struct {
	Actor             string    `dynamodbav:"actor"`              // The actor doing the muting
	Object            string    `dynamodbav:"object"`             // The actor being muted
	ID                string    `dynamodbav:"id"`                 // The mute activity ID
	HideNotifications bool      `dynamodbav:"hide_notifications"` // Whether to hide notifications from this user
	Published         time.Time `dynamodbav:"published"`          // When the mute was created
	CreatedAt         time.Time `dynamodbav:"created_at"`         // Database timestamp
}

// Filter represents a filter in the storage layer
type Filter struct {
	ID           string     `dynamodbav:"id"`
	Username     string     `dynamodbav:"username"`
	Title        string     `dynamodbav:"title"`
	Context      []string   `dynamodbav:"context"`       // home, notifications, public, thread, account
	FilterAction string     `dynamodbav:"filter_action"` // warn, hide, blur
	ExpiresAt    *time.Time `dynamodbav:"expires_at,omitempty"`
	CreatedAt    time.Time  `dynamodbav:"created_at"`
	UpdatedAt    time.Time  `dynamodbav:"updated_at"`
}

// FilterKeyword represents a keyword in a filter
type FilterKeyword struct {
	ID        string    `dynamodbav:"id"`
	FilterID  string    `dynamodbav:"filter_id"`
	Keyword   string    `dynamodbav:"keyword"`
	WholeWord bool      `dynamodbav:"whole_word"`
	CreatedAt time.Time `dynamodbav:"created_at"`
}

// FilterStatus represents a status in a filter
type FilterStatus struct {
	ID        string    `dynamodbav:"id"`
	FilterID  string    `dynamodbav:"filter_id"`
	StatusID  string    `dynamodbav:"status_id"`
	CreatedAt time.Time `dynamodbav:"created_at"`
}
