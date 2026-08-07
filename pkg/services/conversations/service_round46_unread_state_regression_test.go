package conversations

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestBuildDirectMessageParticipantStatesForSend_SetsSenderReadStateImmediately(t *testing.T) {
	publishedAt := time.Date(2026, 3, 26, 19, 10, 0, 0, time.UTC)
	states := buildDirectMessageParticipantStatesForSend(
		createTestConversation("conv-read", []string{"alice", "bob"}),
		&models.Status{StatusID: "status-read", PublishedAt: publishedAt},
		"alice",
		"bob",
		&models.UserConversationState{},
		&models.UserConversationState{},
		false,
	)

	require.Len(t, states, 2)
	senderState := states[0]
	require.False(t, senderState.Unread)
	require.Zero(t, senderState.UnreadCount)
	require.NotNil(t, senderState.LastReadAt)
	require.Equal(t, publishedAt, *senderState.LastReadAt)
}

func TestBuildDirectMessageParticipantStatesForSend_SetsRecipientUnreadStateImmediately(t *testing.T) {
	publishedAt := time.Date(2026, 3, 26, 19, 15, 0, 0, time.UTC)
	states := buildDirectMessageParticipantStatesForSend(
		createTestConversation("conv-unread", []string{"alice", "bob"}),
		&models.Status{StatusID: "status-unread", PublishedAt: publishedAt},
		"alice",
		"bob",
		&models.UserConversationState{},
		&models.UserConversationState{Unread: true, UnreadCount: 2},
		false,
	)

	require.Len(t, states, 2)
	recipientState := states[1]
	require.True(t, recipientState.Unread)
	require.Equal(t, 3, recipientState.UnreadCount)
	require.Nil(t, recipientState.LastReadAt)
	require.Equal(t, "status-unread", recipientState.PreviewStatusID)
	require.Equal(t, publishedAt, recipientState.SortAt)
}
