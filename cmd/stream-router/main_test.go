package main

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestConversationIDFromParticipant(t *testing.T) {
	t.Run("prefers explicit conversation id", func(t *testing.T) {
		participant := models.ConversationParticipantRecord{
			ConversationID: "conv-explicit",
			GSI1PK:         "CONVERSATION#conv-gsi",
			SK:             "2026-03-26T00:00:00Z#conv-sk",
		}

		require.Equal(t, "conv-explicit", conversationIDFromParticipant(participant))
	})

	t.Run("falls back to gsi1 conversation key", func(t *testing.T) {
		participant := models.ConversationParticipantRecord{
			GSI1PK: "CONVERSATION#conv-gsi",
			SK:     "2026-03-26T00:00:00Z#conv-sk",
		}

		require.Equal(t, "conv-gsi", conversationIDFromParticipant(participant))
	})

	t.Run("falls back to legacy sort key tail", func(t *testing.T) {
		participant := models.ConversationParticipantRecord{
			SK: "2026-03-26T00:00:00Z#conv-sk",
		}

		require.Equal(t, "conv-sk", conversationIDFromParticipant(participant))
	})

	t.Run("returns empty when no key carries a conversation id", func(t *testing.T) {
		require.Empty(t, conversationIDFromParticipant(models.ConversationParticipantRecord{}))
	})
}
