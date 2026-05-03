package conversations

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

// TestBuildDirectMessageParticipantStatesForSend_RemoteRecipientAppliesPreviewProjection
// verifies that when sending a direct message to a remote recipient, the sender's
// UserConversationState receives PreviewStatusID, PreviewStatusPublishedAt, SortAt,
// and UpdatedAt before the remote-recipient early return. This catches the regression
// where local→remote sends left the sender preview stale (Track B).
func TestBuildDirectMessageParticipantStatesForSend_RemoteRecipientAppliesPreviewProjection(t *testing.T) {
	publishedAt := time.Date(2026, 5, 3, 17, 0, 0, 0, time.UTC)
	remoteActorID := "https://remote.com/users/steward"

	// Existing sender state with OLD stale preview fields — simulates an existing
	// remote conversation where a prior message set the preview and a new message
	// should overwrite it.
	oldTime := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	senderState := &models.UserConversationState{
		PreviewStatusID:          "old-status-id",
		PreviewStatusPublishedAt: oldTime,
		SortAt:                   oldTime,
		UpdatedAt:                oldTime,
		Unread:                   false,
		LastReadAt:               &oldTime,
	}

	conversation := createTestConversation("conv-remote", []string{"alice", remoteActorID})

	states := buildDirectMessageParticipantStatesForSend(
		conversation,
		&models.Status{StatusID: "new-status-id", PublishedAt: publishedAt},
		"alice",
		remoteActorID,
		senderState,
		nil,   // recipientState is nil for remote recipients (no local state row)
		false, // deliversToInbox — not used for remote path
	)

	// Remote path returns only the sender state.
	require.Len(t, states, 1)

	s := states[0]

	// Preview projection must use the NEW status, overwriting the old stale values.
	require.Equal(t, "new-status-id", s.PreviewStatusID,
		"PreviewStatusID must be updated to the new status for remote-recipient sends")
	require.Equal(t, publishedAt, s.PreviewStatusPublishedAt,
		"PreviewStatusPublishedAt must be updated to the new status time")
	require.Equal(t, publishedAt, s.SortAt,
		"SortAt must be updated so conversation list ordering reflects the new message")
	require.Equal(t, publishedAt, s.UpdatedAt,
		"UpdatedAt must be updated to the new status time")

	// Read-state invariants must be preserved.
	require.False(t, s.Unread, "Sender must remain read for their own message")
	require.NotNil(t, s.LastReadAt, "Sender must have LastReadAt set")
}

// TestBuildDirectMessageParticipantStatesForSend_LocalRecipientStillAppliesPreview
// confirms the local-recipient path is unaffected by the remote-path fix: both sender
// and recipient states receive the preview projection, and the recipient-specific
// request-state / folder / unread logic is intact.
func TestBuildDirectMessageParticipantStatesForSend_LocalRecipientStillAppliesPreview(t *testing.T) {
	publishedAt := time.Date(2026, 5, 3, 17, 5, 0, 0, time.UTC)

	states := buildDirectMessageParticipantStatesForSend(
		createTestConversation("conv-local", []string{"alice", "bob"}),
		&models.Status{StatusID: "status-local", PublishedAt: publishedAt},
		"alice",
		"bob",
		&models.UserConversationState{},
		&models.UserConversationState{},
		false,
	)

	require.Len(t, states, 2)

	senderState := states[0]
	require.Equal(t, "status-local", senderState.PreviewStatusID)
	require.Equal(t, publishedAt, senderState.SortAt)
	require.False(t, senderState.Unread)

	recipientState := states[1]
	require.Equal(t, "status-local", recipientState.PreviewStatusID)
	require.Equal(t, publishedAt, recipientState.SortAt)
	require.True(t, recipientState.Unread)
}
