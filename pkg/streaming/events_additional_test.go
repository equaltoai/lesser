package streaming

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenericEventBuilders(t *testing.T) {
	ev := NewStatusEvent("status.created", "s1", "u1").Build()
	assert.Equal(t, "s1", ev.Payload["status_id"])
	assert.Equal(t, "u1", ev.Payload["author_id"])

	ev = NewAccountEvent("account.updated", "a1").Build()
	assert.Equal(t, "a1", ev.Payload["account_id"])

	ev = NewRelationshipEvent("relationship.followed", "actor1", "target1").Build()
	assert.Equal(t, "actor1", ev.Payload["actor_id"])
	assert.Equal(t, "target1", ev.Payload["target_id"])

	ev = NewNotificationEvent("notification.created", "n1", "u1").Build()
	assert.Equal(t, "n1", ev.Payload["notification_id"])
	assert.Equal(t, "u1", ev.Payload["recipient_id"])

	ev = NewConversationEvent("conversation.updated", "c1").Build()
	assert.Equal(t, "c1", ev.Payload["conversation_id"])

	ev = NewListEvent("list.updated", "l1", "u1").Build()
	assert.Equal(t, "l1", ev.Payload["list_id"])
	assert.Equal(t, "u1", ev.Payload["owner_id"])

	ev = NewMediaEvent("media.uploaded", "m1", "u1").Build()
	assert.Equal(t, "m1", ev.Payload["media_id"])
	assert.Equal(t, "u1", ev.Payload["owner_id"])
}

func TestStreamNamesAndValidation(t *testing.T) {
	assert.Equal(t, "user:u1", UserStreamName("u1"))
	assert.Equal(t, "user:notification:u1", UserNotificationStreamName("u1"))
	assert.Equal(t, "hashtag:go", HashtagStreamName("go"))
	assert.Equal(t, "list:l1", ListStreamName("l1"))
	assert.Equal(t, "direct:u1", DirectStreamName("u1"))
	assert.Equal(t, "conversation:c1", ConversationStreamName("c1"))

	assert.True(t, IsValidEventType(StatusCreated))
	assert.False(t, IsValidEventType("totally.unknown"))

	assert.True(t, IsValidStreamName(PublicStream))
	assert.True(t, IsValidStreamName("user:u1"))
	assert.True(t, IsValidStreamName("conversation:c1"))
	assert.False(t, IsValidStreamName(""))
	assert.False(t, IsValidStreamName("bad:format"))

	assert.Equal(t, "status", GetEventCategory("status.created"))
	assert.Equal(t, "account", GetEventCategory("account.updated"))
	assert.Equal(t, "unknown", GetEventCategory("nope"))
}
