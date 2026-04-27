package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConversationParticipantRefs_NormalizeAndProject(t *testing.T) {
	resolvedAt := time.Date(2026, 4, 27, 12, 0, 0, 0, time.FixedZone("offset", -4*60*60))

	remote := NormalizeConversationParticipantRef(ConversationParticipantRef{
		ParticipantType: ConversationParticipantTypeRemoteActor,
		ParticipantID:   "https://Remote.Example/users/Bob",
		Acct:            "@Bob@Remote.Example",
		Domain:          "Remote.Example",
		ResolvedAt:      &resolvedAt,
	})
	require.Equal(t, ConversationParticipantTypeRemoteActor, remote.ParticipantType)
	require.Equal(t, "https://Remote.Example/users/Bob", remote.ParticipantID)
	require.Equal(t, "bob@remote.example", remote.Acct)
	require.Equal(t, "remote.example", remote.Domain)
	require.NotNil(t, remote.ResolvedAt)
	require.Equal(t, resolvedAt.UTC(), *remote.ResolvedAt)

	local := NormalizeConversationParticipantRef(ConversationParticipantRef{
		ParticipantType: ConversationParticipantTypeLocalUser,
		ParticipantID:   " Alice ",
		Acct:            "@Alice",
		Domain:          "LOCALHOST",
	})
	require.Equal(t, ConversationParticipantTypeLocalUser, local.ParticipantType)
	require.Equal(t, "alice", local.ParticipantID)
	require.Equal(t, "Alice", local.Acct)
	require.Equal(t, "localhost", local.Domain)

	inferredRemote := NormalizeConversationParticipantRef(ConversationParticipantRef{ParticipantID: "https://remote.example/actors/carol"})
	require.Equal(t, ConversationParticipantTypeRemoteActor, inferredRemote.ParticipantType)
	require.Equal(t, "https://remote.example/actors/carol", inferredRemote.ParticipantID)

	inferredLocal := NormalizeConversationParticipantRef(ConversationParticipantRef{ParticipantID: "Carol"})
	require.Equal(t, ConversationParticipantTypeLocalUser, inferredLocal.ParticipantType)
	require.Equal(t, "carol", inferredLocal.ParticipantID)
}

func TestConversationParticipantRefs_CollectionsAndCompatibilityIDs(t *testing.T) {
	refs := NormalizeConversationParticipantRefs([]ConversationParticipantRef{
		{ParticipantType: ConversationParticipantTypeRemoteActor, ParticipantID: "https://remote.example/users/bob", Acct: "bob@remote.example"},
		{ParticipantType: ConversationParticipantTypeLocalUser, ParticipantID: "Alice"},
		{ParticipantType: ConversationParticipantTypeLocalUser, ParticipantID: "alice", Acct: "duplicate"},
		{ParticipantType: ConversationParticipantTypeLocalUser},
	})
	require.Len(t, refs, 2)
	require.Equal(t, ConversationParticipantTypeLocalUser, refs[0].ParticipantType)
	require.Equal(t, "alice", refs[0].ParticipantID)
	require.Equal(t, ConversationParticipantTypeRemoteActor, refs[1].ParticipantType)
	require.Equal(t, "https://remote.example/users/bob", refs[1].ParticipantID)

	ids := ConversationParticipantIDsFromRefs(refs)
	require.Equal(t, []string{"alice", "https://remote.example/users/bob"}, ids)

	conv := &Conversation{ParticipantRefs: refs, Participants: []string{"ignored"}}
	require.True(t, ConversationHasRemoteParticipants(conv))
	require.Equal(t, []string{"alice"}, ConversationLocalParticipantIDs(conv))

	require.True(t, ConversationHasRemoteParticipants(&Conversation{Participants: []string{"alice", "https://remote.example/users/bob"}}))
	require.False(t, ConversationHasRemoteParticipants(&Conversation{Participants: []string{"alice", "bob"}}))
	require.Equal(t, []string{"alice", "bob"}, ConversationLocalParticipantIDs(&Conversation{Participants: []string{"Bob", "alice"}}))
	require.Nil(t, ConversationLocalParticipantIDs(nil))
	require.Nil(t, ConversationParticipantIDsFromRefs(nil))
	require.Nil(t, NormalizeConversationParticipantRefs(nil))
}
