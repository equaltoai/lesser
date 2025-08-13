package streaming

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestEventBuilder_NewEvent(t *testing.T) {
	builder := NewEvent(StatusCreated)
	assert.NotNil(t, builder)
	assert.Equal(t, StatusCreated, builder.event.Type)
	assert.NotNil(t, builder.event.Payload)
	assert.False(t, builder.event.Timestamp.IsZero())
}

func TestEventBuilder_ForStream(t *testing.T) {
	event := NewEvent(StatusCreated).
		ForStream("public").
		Build()

	assert.Equal(t, StatusCreated, event.Type)
	assert.Equal(t, "public", event.Stream)
}

func TestEventBuilder_WithPayload(t *testing.T) {
	payload := map[string]interface{}{
		"status_id": "123",
		"content":   "Hello world",
	}

	event := NewEvent(StatusCreated).
		WithPayload(payload).
		Build()

	assert.Equal(t, payload, event.Payload)
}

func TestEventBuilder_WithData(t *testing.T) {
	event := NewEvent(StatusCreated).
		WithData("status_id", "123").
		WithData("content", "Hello world").
		Build()

	assert.Equal(t, "123", event.Payload["status_id"])
	assert.Equal(t, "Hello world", event.Payload["content"])
}

func TestEventBuilder_WithTimestamp(t *testing.T) {
	customTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	event := NewEvent(StatusCreated).
		WithTimestamp(customTime).
		Build()

	assert.Equal(t, customTime, event.Timestamp)
}

func TestEventBuilder_FluentInterface(t *testing.T) {
	event := NewEvent(StatusCreated).
		ForStream("public").
		WithData("status_id", "123").
		WithData("author_id", "alice").
		WithTimestamp(time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)).
		Build()

	assert.Equal(t, StatusCreated, event.Type)
	assert.Equal(t, "public", event.Stream)
	assert.Equal(t, "123", event.Payload["status_id"])
	assert.Equal(t, "alice", event.Payload["author_id"])
	assert.Equal(t, time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC), event.Timestamp)
}

func TestNewStatusCreatedEvent(t *testing.T) {
	statusData := map[string]interface{}{
		"content":    "Hello world",
		"visibility": "public",
	}

	event := NewStatusCreatedEvent("123", "alice", statusData)

	assert.Equal(t, StatusCreated, event.Type)
	assert.Equal(t, "123", event.Payload["status_id"])
	assert.Equal(t, "alice", event.Payload["author_id"])
	assert.Equal(t, statusData, event.Payload["status"])
}

func TestNewStatusUpdatedEvent(t *testing.T) {
	statusData := map[string]interface{}{
		"content":    "Hello updated world",
		"visibility": "public",
	}

	event := NewStatusUpdatedEvent("123", "alice", statusData)

	assert.Equal(t, StatusUpdated, event.Type)
	assert.Equal(t, "123", event.Payload["status_id"])
	assert.Equal(t, "alice", event.Payload["author_id"])
	assert.Equal(t, statusData, event.Payload["status"])
}

func TestNewStatusDeletedEvent(t *testing.T) {
	event := NewStatusDeletedEvent("123", "alice")

	assert.Equal(t, StatusDeleted, event.Type)
	assert.Equal(t, "123", event.Payload["status_id"])
	assert.Equal(t, "alice", event.Payload["author_id"])
}

func TestNewAccountUpdatedEvent(t *testing.T) {
	accountData := map[string]interface{}{
		"display_name": "Alice Smith",
		"bio":          "Software developer",
	}

	event := NewAccountUpdatedEvent("alice", accountData)

	assert.Equal(t, AccountUpdated, event.Type)
	assert.Equal(t, "alice", event.Payload["account_id"])
	assert.Equal(t, accountData, event.Payload["account"])
}

func TestNewFollowEvent(t *testing.T) {
	event := NewFollowEvent("alice", "bob")

	assert.Equal(t, AccountFollowed, event.Type)
	assert.Equal(t, "alice", event.Payload["follower_id"])
	assert.Equal(t, "bob", event.Payload["followee_id"])
}

func TestNewUnfollowEvent(t *testing.T) {
	event := NewUnfollowEvent("alice", "bob")

	assert.Equal(t, RelationshipUnfollowed, event.Type)
	assert.Equal(t, "alice", event.Payload["follower_id"])
	assert.Equal(t, "bob", event.Payload["followee_id"])
}

func TestNewBlockEvent(t *testing.T) {
	event := NewBlockEvent("alice", "bob")

	assert.Equal(t, RelationshipBlocked, event.Type)
	assert.Equal(t, "alice", event.Payload["blocker_id"])
	assert.Equal(t, "bob", event.Payload["blocked_id"])
}

func TestNewMuteEvent(t *testing.T) {
	// Test without duration
	event := NewMuteEvent("alice", "bob", nil)

	assert.Equal(t, RelationshipMuted, event.Type)
	assert.Equal(t, "alice", event.Payload["muter_id"])
	assert.Equal(t, "bob", event.Payload["muted_id"])
	assert.Nil(t, event.Payload["duration"])

	// Test with duration
	duration := 24 * time.Hour
	event = NewMuteEvent("alice", "bob", &duration)

	assert.Equal(t, RelationshipMuted, event.Type)
	assert.Equal(t, "alice", event.Payload["muter_id"])
	assert.Equal(t, "bob", event.Payload["muted_id"])
	assert.Equal(t, duration.String(), event.Payload["duration"])
}

func TestNewNotificationCreatedEvent(t *testing.T) {
	notificationData := map[string]interface{}{
		"actor":     "alice",
		"status_id": "123",
	}

	event := NewNotificationCreatedEvent("notif123", "bob", "mention", notificationData)

	assert.Equal(t, NotificationCreated, event.Type)
	assert.Equal(t, "notif123", event.Payload["notification_id"])
	assert.Equal(t, "bob", event.Payload["recipient_id"])
	assert.Equal(t, "mention", event.Payload["type"])
	assert.Equal(t, notificationData, event.Payload["notification"])
}

func TestNewConversationUpdatedEvent(t *testing.T) {
	conversationData := map[string]interface{}{
		"last_message_id": "msg123",
		"unread_count":    5,
	}

	event := NewConversationUpdatedEvent("conv123", conversationData)

	assert.Equal(t, ConversationUpdated, event.Type)
	assert.Equal(t, "conv123", event.Payload["conversation_id"])
	assert.Equal(t, conversationData, event.Payload["conversation"])
}

func TestNewListUpdatedEvent(t *testing.T) {
	listData := map[string]interface{}{
		"title":          "My List",
		"replies_policy": "followed",
	}

	event := NewListUpdatedEvent("list123", "alice", listData)

	assert.Equal(t, ListUpdated, event.Type)
	assert.Equal(t, "list123", event.Payload["list_id"])
	assert.Equal(t, "alice", event.Payload["owner_id"])
	assert.Equal(t, listData, event.Payload["list"])
}

func TestNewMediaUploadedEvent(t *testing.T) {
	mediaData := map[string]interface{}{
		"url":      "https://example.com/media/123.jpg",
		"type":     "image",
		"alt_text": "A beautiful sunset",
	}

	event := NewMediaUploadedEvent("media123", "alice", mediaData)

	assert.Equal(t, MediaUploaded, event.Type)
	assert.Equal(t, "media123", event.Payload["media_id"])
	assert.Equal(t, "alice", event.Payload["owner_id"])
	assert.Equal(t, mediaData, event.Payload["media"])
}

func TestUserStreamName(t *testing.T) {
	assert.Equal(t, "user:alice", UserStreamName("alice"))
	assert.Equal(t, "user:bob_123", UserStreamName("bob_123"))
}

func TestUserNotificationStreamName(t *testing.T) {
	assert.Equal(t, "user:notification:alice", UserNotificationStreamName("alice"))
	assert.Equal(t, "user:notification:bob", UserNotificationStreamName("bob"))
}

func TestHashtagStreamName(t *testing.T) {
	assert.Equal(t, "hashtag:golang", HashtagStreamName("golang"))
	assert.Equal(t, "hashtag:activitypub", HashtagStreamName("activitypub"))
}

func TestListStreamName(t *testing.T) {
	assert.Equal(t, "list:123", ListStreamName("123"))
	assert.Equal(t, "list:my-list", ListStreamName("my-list"))
}

func TestDirectStreamName(t *testing.T) {
	assert.Equal(t, "direct:alice", DirectStreamName("alice"))
	assert.Equal(t, "direct:bob", DirectStreamName("bob"))
}

func TestConversationStreamName(t *testing.T) {
	assert.Equal(t, "conversation:123", ConversationStreamName("123"))
	assert.Equal(t, "conversation:conv-abc", ConversationStreamName("conv-abc"))
}

func TestIsValidEventType(t *testing.T) {
	// Test valid event types
	validTypes := []string{
		StatusCreated, StatusUpdated, StatusDeleted,
		StatusFavorited, StatusUnfavorited, StatusBoosted, StatusUnboosted,
		StatusPinned, StatusUnpinned,
		AccountUpdated, AccountFollowed, AccountBlocked, AccountMuted,
		RelationshipFollowRequested, RelationshipFollowAccepted,
		RelationshipFollowRejected, RelationshipUnfollowed,
		RelationshipBlocked, RelationshipUnblocked,
		RelationshipMuted, RelationshipUnmuted,
		NotificationCreated, NotificationRead, NotificationCleared,
		ConversationCreated, ConversationUpdated, ConversationDeleted,
		ListCreated, ListUpdated, ListDeleted, ListMemberAdded, ListMemberRemoved,
		MediaUploaded, MediaUpdated, MediaDeleted, MediaProcessed,
		ModerationReportCreated, ModerationReportUpdated, ModerationActionTaken,
		FederationInstanceBlocked, FederationInstanceUnblocked, FederationActorUpdated,
		SystemAnnouncement, SystemMaintenance,
		FilterCreated, FilterUpdated, FilterDeleted,
		PollVoted, PollClosed, PollExpired,
		HashtagTrending, HashtagFollowed,
	}

	for _, eventType := range validTypes {
		assert.True(t, IsValidEventType(eventType), "Expected %s to be valid", eventType)
	}

	// Test invalid event types
	invalidTypes := []string{
		"invalid.type",
		"",
		"status.unknown",
		"account.invalid",
		"random.event",
	}

	for _, eventType := range invalidTypes {
		assert.False(t, IsValidEventType(eventType), "Expected %s to be invalid", eventType)
	}
}

func TestIsValidStreamName(t *testing.T) {
	// Test valid stream names
	validStreams := []string{
		PublicStream, PublicLocalStream, PublicRemoteStream,
		UserStream, UserNotificationStream, DirectStream,
		SystemStream, ModerationStream, AdminStream,
		"hashtag:golang", "hashtag:activitypub",
		"list:123", "list:my-list",
		"user:alice", "user:bob",
		"user:notification:alice", "user:notification:bob",
		"direct:alice", "direct:bob",
		"conversation:123", "conversation:conv-abc",
	}

	for _, streamName := range validStreams {
		assert.True(t, IsValidStreamName(streamName), "Expected %s to be valid", streamName)
	}

	// Test invalid stream names
	invalidStreams := []string{
		"",              // empty
		"invalid",       // unknown base stream
		"hashtag:",      // missing hashtag name
		"list:",         // missing list ID
		"user:",         // missing user ID
		"conversation:", // missing conversation ID
		"random:stream", // unknown prefix
	}

	for _, streamName := range invalidStreams {
		assert.False(t, IsValidStreamName(streamName), "Expected %s to be invalid", streamName)
	}
}

func TestGetEventCategory(t *testing.T) {
	testCases := []struct {
		eventType        string
		expectedCategory string
	}{
		{StatusCreated, "status"},
		{StatusUpdated, "status"},
		{StatusFavorited, "status"},
		{AccountUpdated, "account"},
		{AccountFollowed, "account"},
		{RelationshipFollowRequested, "relationship"},
		{RelationshipBlocked, "relationship"},
		{NotificationCreated, "notification"},
		{NotificationRead, "notification"},
		{ConversationCreated, "conversation"},
		{ConversationUpdated, "conversation"},
		{ListCreated, "list"},
		{ListMemberAdded, "list"},
		{MediaUploaded, "media"},
		{MediaProcessed, "media"},
		{ModerationReportCreated, "moderation"},
		{ModerationActionTaken, "moderation"},
		{FederationInstanceBlocked, "federation"},
		{FederationActorUpdated, "federation"},
		{SystemAnnouncement, "system"},
		{SystemMaintenance, "system"},
		{FilterCreated, "filter"},
		{FilterUpdated, "filter"},
		{PollVoted, "poll"},
		{PollClosed, "poll"},
		{HashtagTrending, "hashtag"},
		{HashtagFollowed, "hashtag"},
		{"unknown.event", "unknown"},
		{"", "unknown"},
	}

	for _, tc := range testCases {
		category := GetEventCategory(tc.eventType)
		assert.Equal(t, tc.expectedCategory, category,
			"Expected category %s for event type %s, got %s",
			tc.expectedCategory, tc.eventType, category)
	}
}

func TestEventBuilder_WithDataNilPayload(t *testing.T) {
	// Test that WithData initializes payload if nil
	builder := &EventBuilder{
		event: &Event{
			Type:      StatusCreated,
			Payload:   nil, // nil payload
			Timestamp: time.Now(),
		},
	}

	event := builder.WithData("status_id", "123").Build()

	assert.NotNil(t, event.Payload)
	assert.Equal(t, "123", event.Payload["status_id"])
}

func TestEventConstants(t *testing.T) {
	// Test that all event constants are properly defined
	assert.Equal(t, "status.created", StatusCreated)
	assert.Equal(t, "status.updated", StatusUpdated)
	assert.Equal(t, "status.deleted", StatusDeleted)
	assert.Equal(t, "status.favorited", StatusFavorited)
	assert.Equal(t, "status.unfavorited", StatusUnfavorited)
	assert.Equal(t, "status.boosted", StatusBoosted)
	assert.Equal(t, "status.unboosted", StatusUnboosted)
	assert.Equal(t, "status.pinned", StatusPinned)
	assert.Equal(t, "status.unpinned", StatusUnpinned)

	assert.Equal(t, "account.updated", AccountUpdated)
	assert.Equal(t, "account.followed", AccountFollowed)
	assert.Equal(t, "account.blocked", AccountBlocked)
	assert.Equal(t, "account.muted", AccountMuted)

	assert.Equal(t, "relationship.follow_requested", RelationshipFollowRequested)
	assert.Equal(t, "relationship.follow_accepted", RelationshipFollowAccepted)
	assert.Equal(t, "relationship.follow_rejected", RelationshipFollowRejected)
	assert.Equal(t, "relationship.unfollowed", RelationshipUnfollowed)
	assert.Equal(t, "relationship.blocked", RelationshipBlocked)
	assert.Equal(t, "relationship.unblocked", RelationshipUnblocked)
	assert.Equal(t, "relationship.muted", RelationshipMuted)
	assert.Equal(t, "relationship.unmuted", RelationshipUnmuted)

	assert.Equal(t, "notification.created", NotificationCreated)
	assert.Equal(t, "notification.read", NotificationRead)
	assert.Equal(t, "notification.cleared", NotificationCleared)
}

func TestStreamConstants(t *testing.T) {
	// Test that all stream constants are properly defined
	assert.Equal(t, "public", PublicStream)
	assert.Equal(t, "public:local", PublicLocalStream)
	assert.Equal(t, "public:remote", PublicRemoteStream)
	assert.Equal(t, "user", UserStream)
	assert.Equal(t, "user:notification", UserNotificationStream)
	assert.Equal(t, "direct", DirectStream)
	assert.Equal(t, "hashtag", HashtagStreamPrefix)
	assert.Equal(t, "list", ListStreamPrefix)
	assert.Equal(t, "system", SystemStream)
	assert.Equal(t, "moderation", ModerationStream)
	assert.Equal(t, "admin", AdminStream)
}

// Benchmarks

func BenchmarkNewEvent(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewEvent(StatusCreated)
	}
}

func BenchmarkEventBuilder_FluentInterface(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewEvent(StatusCreated).
			ForStream("public").
			WithData("status_id", "123").
			WithData("author_id", "alice").
			Build()
	}
}

func BenchmarkNewStatusCreatedEvent(b *testing.B) {
	statusData := map[string]interface{}{
		"content":    "Hello world",
		"visibility": "public",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewStatusCreatedEvent("123", "alice", statusData)
	}
}

func BenchmarkIsValidEventType(b *testing.B) {
	eventTypes := []string{
		StatusCreated, StatusUpdated, AccountUpdated,
		NotificationCreated, "invalid.type",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, eventType := range eventTypes {
			_ = IsValidEventType(eventType)
		}
	}
}

func BenchmarkIsValidStreamName(b *testing.B) {
	streamNames := []string{
		PublicStream, "user:alice", "hashtag:golang",
		"list:123", "invalid", "",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, streamName := range streamNames {
			_ = IsValidStreamName(streamName)
		}
	}
}

func BenchmarkGetEventCategory(b *testing.B) {
	eventTypes := []string{
		StatusCreated, AccountUpdated, NotificationCreated,
		ConversationUpdated, ModerationReportCreated,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, eventType := range eventTypes {
			_ = GetEventCategory(eventType)
		}
	}
}
