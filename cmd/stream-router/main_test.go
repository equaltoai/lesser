package main

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestConversationIDFromState(t *testing.T) {
	t.Run("prefers explicit conversation id", func(t *testing.T) {
		state := models.UserConversationState{
			ConversationID: "conv-explicit",
			GSI3PK:         "CONVERSATION#conv-gsi",
			SK:             "CONVERSATION#conv-sk",
		}

		require.Equal(t, "conv-explicit", conversationIDFromState(state))
	})

	t.Run("falls back to reverse-lookup conversation key", func(t *testing.T) {
		state := models.UserConversationState{
			GSI3PK: "CONVERSATION#conv-gsi",
			SK:     "CONVERSATION#conv-sk",
		}

		require.Equal(t, "conv-gsi", conversationIDFromState(state))
	})

	t.Run("falls back to canonical state sort key tail", func(t *testing.T) {
		state := models.UserConversationState{
			SK: "CONVERSATION#conv-sk",
		}

		require.Equal(t, "conv-sk", conversationIDFromState(state))
	})

	t.Run("returns empty when no key carries a conversation id", func(t *testing.T) {
		require.Empty(t, conversationIDFromState(models.UserConversationState{}))
	})
}
