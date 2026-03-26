package conversations

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestBuildDirectMessageParticipantStatesForSend_ReopensDeclinedRecipientAsPendingRequest(t *testing.T) {
	publishedAt := time.Date(2026, 3, 26, 19, 0, 0, 0, time.UTC)
	states := buildDirectMessageParticipantStatesForSend(
		createTestConversation("conv-request", []string{"alice", "bob"}),
		&models.Status{StatusID: "status-request", PublishedAt: publishedAt},
		"alice",
		"bob",
		&models.ConversationParticipantRecord{RequestState: models.DmRequestStateAccepted},
		&models.ConversationParticipantRecord{RequestState: models.DmRequestStateDeclined},
		false,
	)

	require.Len(t, states, 2)
	recipientState := states[1]
	require.Equal(t, models.UserConversationFolderRequests, recipientState.Folder)
	require.Equal(t, models.DmRequestStatePending, recipientState.RequestState)
	require.Nil(t, recipientState.AcceptedAt)
	require.Nil(t, recipientState.DeclinedAt)
	require.NotNil(t, recipientState.RequestedAt)
}

func TestBuildDirectMessageParticipantStatesForSend_KeepsAcceptedRecipientInInbox(t *testing.T) {
	publishedAt := time.Date(2026, 3, 26, 19, 5, 0, 0, time.UTC)
	states := buildDirectMessageParticipantStatesForSend(
		createTestConversation("conv-inbox", []string{"alice", "bob"}),
		&models.Status{StatusID: "status-inbox", PublishedAt: publishedAt},
		"alice",
		"bob",
		&models.ConversationParticipantRecord{RequestState: models.DmRequestStateAccepted},
		&models.ConversationParticipantRecord{RequestState: models.DmRequestStateAccepted},
		false,
	)

	require.Len(t, states, 2)
	recipientState := states[1]
	require.Equal(t, models.UserConversationFolderInbox, recipientState.Folder)
	require.Equal(t, models.DmRequestStateAccepted, recipientState.RequestState)
	require.Nil(t, recipientState.RequestedAt)
	require.NotNil(t, recipientState.AcceptedAt)
}
