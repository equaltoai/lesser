// Package streaming provides event constants and builder helpers for real-time streaming
package streaming

import (
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// Event type constants following Mastodon streaming API conventions
const (
	// Status/Note Events
	StatusCreated     = "status.created"     // New status posted
	StatusUpdated     = "status.updated"     // Status edited
	StatusDeleted     = "status.deleted"     // Status deleted
	StatusFavorited   = "status.favorited"   // Status favorited
	StatusUnfavorited = "status.unfavorited" // Status unfavorited
	StatusBoosted     = "status.boosted"     // Status boosted/reblogged
	StatusUnboosted   = "status.unboosted"   // Status unreblogged
	StatusPinned      = "status.pinned"      // Status pinned to profile
	StatusUnpinned    = "status.unpinned"    // Status unpinned from profile

	// Account Events
	AccountUpdated  = "account.updated"  // Profile updated
	AccountFollowed = "account.followed" // Account followed
	AccountBlocked  = "account.blocked"  // Account blocked
	AccountMuted    = "account.muted"    // Account muted

	// Relationship Events
	RelationshipFollowRequested = "relationship.follow_requested" // Follow request sent
	RelationshipFollowAccepted  = "relationship.follow_accepted"  // Follow request accepted
	RelationshipFollowRejected  = "relationship.follow_rejected"  // Follow request rejected
	RelationshipUnfollowed      = "relationship.unfollowed"       // Account unfollowed
	RelationshipBlocked         = "relationship.blocked"          // Account blocked
	RelationshipUnblocked       = "relationship.unblocked"        // Account unblocked
	RelationshipMuted           = "relationship.muted"            // Account muted
	RelationshipUnmuted         = "relationship.unmuted"          // Account unmuted

	// Notification Events
	NotificationCreated = "notification.created" // New notification
	NotificationRead    = "notification.read"    // Notification marked as read
	NotificationCleared = "notification.cleared" // Notifications cleared

	// Conversation Events
	ConversationCreated = "conversation.created" // New conversation started
	ConversationUpdated = "conversation.updated" // Conversation updated (new message, read status)
	ConversationDeleted = "conversation.deleted" // Conversation deleted

	// List Events
	ListCreated       = "list.created"        // List created
	ListUpdated       = "list.updated"        // List updated (title, privacy)
	ListDeleted       = "list.deleted"        // List deleted
	ListMemberAdded   = "list.member_added"   // Account added to list
	ListMemberRemoved = "list.member_removed" // Account removed from list

	// Media Events
	MediaUploaded  = "media.uploaded"  // Media uploaded and processed
	MediaUpdated   = "media.updated"   // Media metadata updated (alt text, focus)
	MediaDeleted   = "media.deleted"   // Media deleted
	MediaProcessed = "media.processed" // Media processing completed

	// Moderation Events
	ModerationReportCreated = "moderation.report_created" // New report submitted
	ModerationReportUpdated = "moderation.report_updated" // Report status updated
	ModerationActionTaken   = "moderation.action_taken"   // Moderation action performed

	// Federation Events
	FederationInstanceBlocked   = "federation.instance_blocked"   // Instance blocked
	FederationInstanceUnblocked = "federation.instance_unblocked" // Instance unblocked
	FederationActorUpdated      = "federation.actor_updated"      // Remote actor profile updated

	// System Events
	SystemAnnouncement = "system.announcement" // System announcement
	SystemMaintenance  = "system.maintenance"  // Maintenance notification

	// Filter Events
	FilterCreated = "filter.created" // Content filter created
	FilterUpdated = "filter.updated" // Content filter updated
	FilterDeleted = "filter.deleted" // Content filter deleted

	// Poll Events
	PollVoted   = "poll.voted"   // Vote cast in poll
	PollClosed  = "poll.closed"  // Poll closed
	PollExpired = "poll.expired" // Poll expired

	// Hashtag Events
	HashtagTrending = "hashtag.trending" // Hashtag is trending
	HashtagFollowed = "hashtag.followed" // Hashtag followed
)

// Stream name constants following Mastodon streaming API
const (
	// Public Streams
	PublicStream       = "public"        // All public posts
	PublicLocalStream  = "public:local"  // Local public posts only
	PublicRemoteStream = "public:remote" // Remote public posts only

	// User Streams
	UserStream             = "user"              // User's home timeline and notifications
	UserNotificationStream = "user:notification" // User's notifications only

	// Direct Messages
	DirectStream = "direct" // Direct messages for the user

	// Hashtag Streams (template - append hashtag name)
	HashtagStreamPrefix = "hashtag" // Use as "hashtag:name"

	// List Streams (template - append list ID)
	ListStreamPrefix = "list" // Use as "list:id"

	// System Streams
	SystemStream = "system" // System announcements and maintenance

	// Moderation Streams
	ModerationStream = "moderation" // Moderation events for moderators

	// Admin Streams
	AdminStream = "admin" // Admin-only events
)

// EventBuilder provides a fluent interface for building streaming events
type EventBuilder struct {
	event *Event
}

// NewEvent creates a new EventBuilder with the specified type
func NewEvent(eventType string) *EventBuilder {
	return &EventBuilder{
		event: &Event{
			Type:      eventType,
			Payload:   make(map[string]interface{}),
			Timestamp: time.Now(),
		},
	}
}

// ForStream sets the stream name for the event
func (eb *EventBuilder) ForStream(stream string) *EventBuilder {
	eb.event.Stream = stream
	return eb
}

// WithPayload sets the entire payload for the event
func (eb *EventBuilder) WithPayload(payload map[string]interface{}) *EventBuilder {
	eb.event.Payload = payload
	return eb
}

// WithData adds a key-value pair to the event payload
func (eb *EventBuilder) WithData(key string, value interface{}) *EventBuilder {
	if eb.event.Payload == nil {
		eb.event.Payload = make(map[string]interface{})
	}
	eb.event.Payload[key] = value
	return eb
}

// WithTimestamp sets a custom timestamp for the event
func (eb *EventBuilder) WithTimestamp(timestamp time.Time) *EventBuilder {
	eb.event.Timestamp = timestamp
	return eb
}

// Build returns the constructed Event
func (eb *EventBuilder) Build() *Event {
	return eb.event
}

// Helper functions for creating common events

// NewStatusEvent creates a status-related event
func NewStatusEvent(eventType, statusID, authorID string) *EventBuilder {
	return NewEvent(eventType).
		WithData("status_id", statusID).
		WithData("author_id", authorID)
}

// NewStatusCreatedEvent creates a status.created event
func NewStatusCreatedEvent(statusID, authorID string, statusData map[string]interface{}) *Event {
	return NewEvent(StatusCreated).
		WithData("status_id", statusID).
		WithData("author_id", authorID).
		WithData("status", statusData).
		Build()
}

// NewStatusUpdatedEvent creates a status.updated event
func NewStatusUpdatedEvent(statusID, authorID string, statusData map[string]interface{}) *Event {
	return NewEvent(StatusUpdated).
		WithData("status_id", statusID).
		WithData("author_id", authorID).
		WithData("status", statusData).
		Build()
}

// NewStatusDeletedEvent creates a status.deleted event
func NewStatusDeletedEvent(statusID, authorID string) *Event {
	return NewEvent(StatusDeleted).
		WithData("status_id", statusID).
		WithData("author_id", authorID).
		Build()
}

// NewAccountEvent creates an account-related event
func NewAccountEvent(eventType, accountID string) *EventBuilder {
	return NewEvent(eventType).
		WithData("account_id", accountID)
}

// NewAccountUpdatedEvent creates an account.updated event
func NewAccountUpdatedEvent(accountID string, accountData map[string]interface{}) *Event {
	return NewEvent(AccountUpdated).
		WithData("account_id", accountID).
		WithData("account", accountData).
		Build()
}

// NewRelationshipEvent creates a relationship-related event
func NewRelationshipEvent(eventType, actorID, targetID string) *EventBuilder {
	return NewEvent(eventType).
		WithData("actor_id", actorID).
		WithData("target_id", targetID)
}

// NewFollowEvent creates a relationship.followed event
func NewFollowEvent(followerID, followeeID string) *Event {
	return NewEvent(AccountFollowed).
		WithData("follower_id", followerID).
		WithData("followee_id", followeeID).
		Build()
}

// NewUnfollowEvent creates a relationship.unfollowed event
func NewUnfollowEvent(followerID, followeeID string) *Event {
	return NewEvent(RelationshipUnfollowed).
		WithData("follower_id", followerID).
		WithData("followee_id", followeeID).
		Build()
}

// NewBlockEvent creates a relationship.blocked event
func NewBlockEvent(blockerID, blockedID string) *Event {
	return NewEvent(RelationshipBlocked).
		WithData("blocker_id", blockerID).
		WithData("blocked_id", blockedID).
		Build()
}

// NewMuteEvent creates a relationship.muted event
func NewMuteEvent(muterID, mutedID string, duration *time.Duration) *Event {
	eb := NewEvent(RelationshipMuted).
		WithData("muter_id", muterID).
		WithData("muted_id", mutedID)

	if duration != nil {
		eb.WithData("duration", duration.String())
	}

	return eb.Build()
}

// NewNotificationEvent creates a notification-related event
func NewNotificationEvent(eventType, notificationID, recipientID string) *EventBuilder {
	return NewEvent(eventType).
		WithData("notification_id", notificationID).
		WithData("recipient_id", recipientID)
}

// NewNotificationCreatedEvent creates a notification.created event
func NewNotificationCreatedEvent(notificationID, recipientID, notificationType string, notificationData map[string]interface{}) *Event {
	return NewEvent(NotificationCreated).
		WithData("notification_id", notificationID).
		WithData("recipient_id", recipientID).
		WithData("type", notificationType).
		WithData("notification", notificationData).
		Build()
}

// NewConversationEvent creates a conversation-related event
func NewConversationEvent(eventType, conversationID string) *EventBuilder {
	return NewEvent(eventType).
		WithData("conversation_id", conversationID)
}

// NewConversationUpdatedEvent creates a conversation.updated event
func NewConversationUpdatedEvent(conversationID string, conversationData map[string]interface{}) *Event {
	return NewEvent(ConversationUpdated).
		WithData("conversation_id", conversationID).
		WithData("conversation", conversationData).
		Build()
}

// NewListEvent creates a list-related event
func NewListEvent(eventType, listID, ownerID string) *EventBuilder {
	return NewEvent(eventType).
		WithData("list_id", listID).
		WithData("owner_id", ownerID)
}

// NewListUpdatedEvent creates a list.updated event
func NewListUpdatedEvent(listID, ownerID string, listData map[string]interface{}) *Event {
	return NewEvent(ListUpdated).
		WithData("list_id", listID).
		WithData("owner_id", ownerID).
		WithData("list", listData).
		Build()
}

// NewMediaEvent creates a media-related event
func NewMediaEvent(eventType, mediaID, ownerID string) *EventBuilder {
	return NewEvent(eventType).
		WithData("media_id", mediaID).
		WithData("owner_id", ownerID)
}

// NewMediaUploadedEvent creates a media.uploaded event
func NewMediaUploadedEvent(mediaID, ownerID string, mediaData map[string]interface{}) *Event {
	return NewEvent(MediaUploaded).
		WithData("media_id", mediaID).
		WithData("owner_id", ownerID).
		WithData("media", mediaData).
		Build()
}

// Stream name helper functions

// UserStreamName returns the user stream name for a specific user
func UserStreamName(userID string) string {
	return fmt.Sprintf("%s:%s", UserStream, userID)
}

// UserNotificationStreamName returns the notification stream name for a specific user
func UserNotificationStreamName(userID string) string {
	return fmt.Sprintf("%s:%s", UserNotificationStream, userID)
}

// HashtagStreamName returns the hashtag stream name for a specific hashtag
func HashtagStreamName(hashtag string) string {
	return fmt.Sprintf("%s:%s", HashtagStreamPrefix, hashtag)
}

// ListStreamName returns the list stream name for a specific list
func ListStreamName(listID string) string {
	return fmt.Sprintf("%s:%s", ListStreamPrefix, listID)
}

// DirectStreamName returns the direct message stream name for a specific user
func DirectStreamName(userID string) string {
	return fmt.Sprintf("%s:%s", DirectStream, userID)
}

// ConversationStreamName returns the conversation stream name for a specific conversation
func ConversationStreamName(conversationID string) string {
	return fmt.Sprintf("conversation:%s", conversationID)
}

// Event validation helpers

// IsValidEventType checks if an event type is valid
func IsValidEventType(eventType string) bool {
	validTypes := map[string]bool{
		StatusCreated: true, StatusUpdated: true, StatusDeleted: true,
		StatusFavorited: true, StatusUnfavorited: true,
		StatusBoosted: true, StatusUnboosted: true,
		StatusPinned: true, StatusUnpinned: true,
		AccountUpdated: true, AccountFollowed: true, AccountBlocked: true, AccountMuted: true,
		RelationshipFollowRequested: true, RelationshipFollowAccepted: true,
		RelationshipFollowRejected: true, RelationshipUnfollowed: true,
		RelationshipBlocked: true, RelationshipUnblocked: true,
		RelationshipMuted: true, RelationshipUnmuted: true,
		NotificationCreated: true, NotificationRead: true, NotificationCleared: true,
		ConversationCreated: true, ConversationUpdated: true, ConversationDeleted: true,
		ListCreated: true, ListUpdated: true, ListDeleted: true,
		ListMemberAdded: true, ListMemberRemoved: true,
		MediaUploaded: true, MediaUpdated: true, MediaDeleted: true, MediaProcessed: true,
		ModerationReportCreated: true, ModerationReportUpdated: true, ModerationActionTaken: true,
		FederationInstanceBlocked: true, FederationInstanceUnblocked: true, FederationActorUpdated: true,
		SystemAnnouncement: true, SystemMaintenance: true,
		FilterCreated: true, FilterUpdated: true, FilterDeleted: true,
		PollVoted: true, PollClosed: true, PollExpired: true,
		HashtagTrending: true, HashtagFollowed: true,
	}

	return validTypes[eventType]
}

// IsValidStreamName checks if a stream name follows valid patterns
func IsValidStreamName(streamName string) bool {
	if err := common.ValidateRequiredParam("streamName", streamName); err != nil {
		return false
	}

	// Check exact matches
	exactMatches := map[string]bool{
		PublicStream: true, PublicLocalStream: true, PublicRemoteStream: true,
		UserStream: true, UserNotificationStream: true,
		DirectStream: true, SystemStream: true,
		ModerationStream: true, AdminStream: true,
	}

	if exactMatches[streamName] {
		return true
	}

	// Check prefixes (streams with IDs)
	prefixes := []string{
		HashtagStreamPrefix + ":",
		ListStreamPrefix + ":",
		UserStream + ":",
		UserNotificationStream + ":",
		DirectStream + ":",
		"conversation:",
	}

	for _, prefix := range prefixes {
		if len(streamName) > len(prefix) && streamName[:len(prefix)] == prefix {
			return true
		}
	}

	return false
}

// GetEventCategory returns the category of an event type
func GetEventCategory(eventType string) string {
	// Use strings package for simpler implementation
	if strings.HasPrefix(eventType, "status.") {
		return "status"
	}
	if strings.HasPrefix(eventType, "account.") {
		return "account"
	}
	if strings.HasPrefix(eventType, "relationship.") {
		return "relationship"
	}
	if strings.HasPrefix(eventType, "notification.") {
		return "notification"
	}
	if strings.HasPrefix(eventType, "conversation.") {
		return "conversation"
	}
	if strings.HasPrefix(eventType, "list.") {
		return "list"
	}
	if strings.HasPrefix(eventType, "media.") {
		return "media"
	}
	if strings.HasPrefix(eventType, "moderation.") {
		return "moderation"
	}
	if strings.HasPrefix(eventType, "federation.") {
		return "federation"
	}
	if strings.HasPrefix(eventType, "system.") {
		return "system"
	}
	if strings.HasPrefix(eventType, "filter.") {
		return "filter"
	}
	if strings.HasPrefix(eventType, "poll.") {
		return "poll"
	}
	if strings.HasPrefix(eventType, "hashtag.") {
		return "hashtag"
	}
	return "unknown"
}
