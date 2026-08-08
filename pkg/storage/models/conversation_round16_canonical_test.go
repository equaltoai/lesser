package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRound16_ConversationCanonicalHelpersAndKeys(t *testing.T) {
	t.Run("canonicalize participant IDs and participant lists", func(t *testing.T) {
		require.Equal(t, "medic", CanonicalConversationParticipantID(" Medic "))
		require.Equal(t, "https://remote.example/users/Medic", CanonicalConversationParticipantID("https://remote.example/users/Medic"))
		require.Equal(t, []string{"agent-0", "medic"}, CanonicalConversationParticipants([]string{"Medic", " ", "Agent-0"}))
	})

	t.Run("conversation key helpers return stable values", func(t *testing.T) {
		conversation := &Conversation{ID: "conv-1"}
		require.NoError(t, conversation.UpdateKeys())
		require.Equal(t, MainTableName, conversation.TableName())
		require.Equal(t, "CONVERSATION#conv-1", conversation.GetPK())
		require.Equal(t, "METADATA", conversation.GetSK())
	})

	t.Run("participant record keys canonicalize usernames", func(t *testing.T) {
		record := &ConversationParticipantRecord{
			ConversationID: "conv-1",
			Conversation: &Conversation{
				ID:           "conv-1",
				Participants: []string{"arch", "medic"},
				UpdatedAt:    time.Date(2026, 3, 24, 12, 0, 0, 0, time.UTC),
			},
		}
		require.NoError(t, record.BeforeCreate("Medic"))
		require.NoError(t, record.UpdateKeys())
		require.Equal(t, MainTableName, record.TableName())
		require.Equal(t, "USER_CONVERSATIONS#medic", record.GetPK())
		require.Equal(t, "PARTICIPANT#medic", record.GSI1SK)
		require.Equal(t, "2026-03-24T12:00:00Z#conv-1", record.GetSK())
	})

	t.Run("user conversation state keys canonicalize viewer identity", func(t *testing.T) {
		state := &UserConversationState{
			ViewerID:       "Medic",
			ConversationID: "conv-1",
			CounterpartID:  "Arch",
			Folder:         UserConversationFolderInbox,
			SortAt:         time.Date(2026, 3, 24, 12, 0, 0, 0, time.UTC),
			Unread:         true,
		}

		require.NoError(t, state.BeforeCreate())
		require.Equal(t, MainTableName, state.TableName())
		require.Equal(t, "USER_CONVERSATION_STATE#medic", state.GetPK())
		require.Equal(t, "CONVERSATION#conv-1", state.GetSK())
		require.Equal(t, "USER_CONVERSATION_FOLDER#medic#INBOX", state.GSI1PK)
		require.Equal(t, "USER_CONVERSATION_UNREAD#medic", state.GSI2PK)
		require.Equal(t, "CONVERSATION#conv-1", state.GSI3PK)
		require.Equal(t, "USER#medic", state.GSI3SK)
	})

	t.Run("user conversation state before update normalizes timing and unread visibility", func(t *testing.T) {
		requestedAt := time.Date(2026, 3, 25, 8, 30, 0, 0, time.FixedZone("EST", -5*60*60))
		deletedAt := time.Date(2026, 3, 25, 9, 0, 0, 0, time.FixedZone("EST", -5*60*60))
		lastReadAt := time.Time{}
		createdAt := time.Date(2026, 3, 24, 12, 0, 0, 0, time.UTC)
		previewAt := time.Date(2026, 3, 25, 14, 0, 0, 0, time.UTC)

		state := &UserConversationState{
			ViewerID:                 " Medic ",
			ConversationID:           " conv-1 ",
			CounterpartID:            " Arch ",
			Folder:                   UserConversationFolderHidden,
			Unread:                   true,
			RequestedAt:              &requestedAt,
			DeletedAt:                &deletedAt,
			LastReadAt:               &lastReadAt,
			CreatedAt:                createdAt,
			PreviewStatusPublishedAt: previewAt,
		}

		require.NoError(t, state.BeforeUpdate())
		require.Equal(t, "medic", state.ViewerID)
		require.Equal(t, "conv-1", state.ConversationID)
		require.Equal(t, "arch", state.CounterpartID)
		require.Equal(t, createdAt, state.CreatedAt)
		require.Equal(t, previewAt, state.SortAt)
		require.Equal(t, requestedAt.UTC(), *state.RequestedAt)
		require.Equal(t, deletedAt.UTC(), *state.DeletedAt)
		require.Nil(t, state.LastReadAt)
		require.False(t, state.UnreadQueryVisible())
		require.Empty(t, state.GSI2PK)
		require.Equal(t, "2026-03-25T14:00:00.000000000Z#conv-1", state.LegacyListCursor())

		state.Folder = UserConversationFolderInbox
		state.DeletedAt = nil
		require.True(t, state.UnreadQueryVisible())
	})

	t.Run("participant lookup key helpers expose stored keys", func(t *testing.T) {
		lookup := &ConversationParticipantKey{
			PK: "CONVERSATION_PARTICIPANTS#arch,medic",
			SK: "LOOKUP",
		}
		require.NoError(t, lookup.UpdateKeys())
		require.Equal(t, MainTableName, lookup.TableName())
		require.Equal(t, lookup.PK, lookup.GetPK())
		require.Equal(t, lookup.SK, lookup.GetSK())
	})
}
