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
