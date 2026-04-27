package repositories

import (
	"strings"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestConversationParticipantLookupV2Helpers(t *testing.T) {
	refs := []models.ConversationParticipantRef{
		{ParticipantType: models.ConversationParticipantTypeRemoteActor, ParticipantID: "https://remote.example/users/bob", Acct: "bob@remote.example"},
		{ParticipantType: models.ConversationParticipantTypeLocalUser, ParticipantID: "Alice"},
	}
	conversation := &models.Conversation{ID: "conv-1", ParticipantRefs: refs}

	key := conversationParticipantLookupV2PK(refs)
	require.True(t, strings.HasPrefix(key, "CONVERSATION_PARTICIPANTS_V2#"))
	require.Equal(t, key, conversationParticipantLookupPKForConversation(conversation, nil))
	require.True(t, useConversationParticipantLookupV2(conversation, nil))
	require.Equal(t, "conv-1", conversationIDFromModel(conversation))
	require.Empty(t, conversationIDFromModel(nil))

	lookup := newConversationParticipantLookupForConversation(conversation, nil)
	require.Equal(t, key, lookup.PK)
	require.Equal(t, conversationParticipantLookupSK, lookup.SK)
	require.Equal(t, key, lookup.GSI1PK)
	require.Equal(t, "conv-1", lookup.ConversationID)
}

func TestConversationParticipantRefsForLookupLegacyAndRemoteFallbacks(t *testing.T) {
	localRefs := conversationParticipantRefsForLookup(nil, []string{"Bob", "alice"})
	require.Equal(t, []models.ConversationParticipantRef{
		{ParticipantType: models.ConversationParticipantTypeLocalUser, ParticipantID: "alice"},
		{ParticipantType: models.ConversationParticipantTypeLocalUser, ParticipantID: "bob"},
	}, localRefs)
	require.False(t, useConversationParticipantLookupV2(nil, []string{"alice", "bob"}))
	require.Equal(t, conversationParticipantLookupPK([]string{"alice", "bob"}), conversationParticipantLookupPKForConversation(nil, []string{"Bob", "alice"}))

	remoteRefs := conversationParticipantRefsForLookup(nil, []string{"alice", "https://remote.example/users/bob"})
	require.Equal(t, models.ConversationParticipantTypeRemoteActor, remoteRefs[1].ParticipantType)
	require.True(t, useConversationParticipantLookupV2(nil, []string{"alice", "https://remote.example/users/bob"}))
	require.True(t, strings.HasPrefix(conversationParticipantLookupPKForConversation(nil, []string{"alice", "https://remote.example/users/bob"}), "CONVERSATION_PARTICIPANTS_V2#"))
}
