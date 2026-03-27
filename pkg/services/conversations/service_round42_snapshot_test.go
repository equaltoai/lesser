package conversations

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestRound42_userConversationStateFromContract_DefaultsAndOverrides(t *testing.T) {
	conversation := createTestConversation("conv-1", []string{"alice", "bob"})
	conversation.CreatedAt = time.Date(2026, 3, 26, 16, 0, 0, 0, time.UTC)
	conversation.UpdatedAt = time.Date(2026, 3, 26, 16, 5, 0, 0, time.UTC)

	t.Run("nil contract falls back to canonical defaults", func(t *testing.T) {
		state := userConversationStateFromContract(conversation, "alice", "bob", nil)
		require.Equal(t, "alice", state.ViewerID)
		require.Equal(t, "bob", state.CounterpartID)
		require.Equal(t, conversation.ID, state.ConversationID)
		require.Equal(t, conversation.CreatedAt.UTC(), state.CreatedAt)
		require.Equal(t, conversation.UpdatedAt.UTC(), state.UpdatedAt)
	})

	t.Run("contract values override defaults", func(t *testing.T) {
		acceptedAt := time.Date(2026, 3, 26, 16, 6, 0, 0, time.UTC)
		state := userConversationStateFromContract(conversation, "alice", "bob", &interfaces.UserConversationStateContract{
			ViewerID:       "alice",
			ConversationID: conversation.ID,
			CounterpartID:  "charlie",
			Folder:         models.UserConversationFolderRequests,
			RequestState:   models.DmRequestStatePending,
			AcceptedAt:     &acceptedAt,
			UpdatedAt:      acceptedAt,
		})
		require.Equal(t, "charlie", state.CounterpartID)
		require.Equal(t, models.UserConversationFolderRequests, state.Folder)
		require.Equal(t, models.DmRequestStatePending, state.RequestState)
		require.Equal(t, acceptedAt, state.UpdatedAt)
		require.NotNil(t, state.AcceptedAt)
		require.Equal(t, acceptedAt, *state.AcceptedAt)
	})
}
