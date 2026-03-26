package repositories

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRound44_NewConversationParticipantLookup_CanonicalizesParticipants(t *testing.T) {
	lookup := newConversationParticipantLookup("conv-1", []string{"Medic", "Arch"})

	require.NotNil(t, lookup)
	require.Equal(t, "CONVERSATION_PARTICIPANTS#arch,medic", lookup.PK)
	require.Equal(t, conversationParticipantLookupSK, lookup.SK)
	require.Equal(t, "CONVERSATION_PARTICIPANTS#arch,medic", lookup.GSI1PK)
	require.Equal(t, "conv-1", lookup.ConversationID)
}
