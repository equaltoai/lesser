package conversations

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestFederatedDirectMessageIdentityHelpers(t *testing.T) {
	actor := &activitypub.Actor{
		BaseObject:        activitypub.BaseObject{ID: "https://remote.example/users/bob", Type: activitypub.PersonType},
		PreferredUsername: "bob",
	}
	identity := federationActorIdentityForDirectMessage(actor, "example.com")
	require.Equal(t, "bob", identity.username)
	require.Equal(t, "remote.example", identity.domain)

	localIdentity := federationActorIdentityForDirectMessage(&activitypub.Actor{
		BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice", Type: activitypub.PersonType},
	}, "example.com")
	require.Equal(t, "alice", localIdentity.username)
	require.Empty(t, localIdentity.domain)
	require.Empty(t, federationActorIdentityForDirectMessage(nil, "example.com").username)

	ref := &models.ConversationParticipantRef{
		ParticipantType: models.ConversationParticipantTypeRemoteActor,
		ParticipantID:   "https://remote.example/actors/carol",
		Acct:            "carol@remote.example",
	}
	require.Equal(t, "carol", remoteUsernameFromParticipantRef(ref))
	require.Equal(t, "dave", remoteUsernameFromParticipantRef(&models.ConversationParticipantRef{ParticipantID: "https://remote.example/users/dave"}))
	require.Empty(t, remoteUsernameFromParticipantRef(nil))
}

func TestFederatedDirectMessageRecipientIdentifierHelpers(t *testing.T) {
	require.True(t, isRemoteDirectMessageRecipientIdentifier("bob@remote.example", "example.com"))
	require.True(t, isRemoteDirectMessageRecipientIdentifier("https://remote.example/users/bob", "example.com"))
	require.False(t, isRemoteDirectMessageRecipientIdentifier("alice", "example.com"))
	require.False(t, isRemoteDirectMessageRecipientIdentifier("alice@example.com", "example.com"))
	require.False(t, isRemoteDirectMessageRecipientIdentifier("", "example.com"))

	require.True(t, isRemoteDirectMessageActorID("https://remote.example/users/bob", "example.com"))
	require.False(t, isRemoteDirectMessageActorID("https://example.com/users/alice", "example.com"))
	require.False(t, isRemoteDirectMessageActorID("alice", "example.com"))
	require.False(t, isRemoteDirectMessageActorID("", "example.com"))
}

func TestFederatedDirectMessageUsernameExtraction(t *testing.T) {
	require.Equal(t, "bob", extractUsernameFromActorIdentifier("bob@remote.example"))
	require.Equal(t, "bob", extractUsernameFromActorIdentifier("https://remote.example/users/bob"))
	require.Equal(t, "carol", extractUsernameFromActorIdentifier("https://remote.example/actors/@carol"))
	require.Equal(t, "dave", extractUsernameFromActorIdentifier("https://remote.example/profiles/dave"))
	require.Equal(t, "erin", extractUsernameFromActorIdentifier("@erin"))
	require.Empty(t, extractUsernameFromActorIdentifier("https://remote.example"))
	require.Empty(t, extractUsernameFromActorIdentifier(""))
}

func TestGetSendMessageAccountsForSend_LocalConversationParticipant(t *testing.T) {
	ctx := context.Background()
	accountRepo := &mockAccountRepository{}
	service := NewService(nil, nil, nil, accountRepo, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")
	conversation := &models.Conversation{
		ID:           "conv-local",
		Participants: []string{"alice", "bob"},
		ParticipantRefs: []models.ConversationParticipantRef{
			{ParticipantType: models.ConversationParticipantTypeLocalUser, ParticipantID: "alice"},
			{ParticipantType: models.ConversationParticipantTypeLocalUser, ParticipantID: "bob"},
		},
	}
	sendCmd := &SendDirectMessageCommand{SenderID: "alice", Recipients: []string{"bob"}}

	accountRepo.On("GetAccount", ctx, "alice").Return(createTestAccount("alice", "alice"), nil).Once()
	accountRepo.On("GetAccount", ctx, "bob").Return(createTestAccount("bob", "bob"), nil).Once()

	sender, recipient, err := service.getSendMessageAccountsForSend(ctx, sendCmd, conversation, "alice", "bob")
	require.NoError(t, err)
	require.Equal(t, "alice", sender.User.Username)
	require.Equal(t, "bob", recipient.User.Username)
	require.Nil(t, sendCmd.ResolvedRecipientRef)
	require.Nil(t, sendCmd.ResolvedRecipientActor)
	accountRepo.AssertExpectations(t)
}

func TestGetSendMessageAccountsForSend_RemoteConversationParticipant(t *testing.T) {
	ctx := context.Background()
	accountRepo := &mockAccountRepository{}
	service := NewService(nil, nil, nil, accountRepo, nil, nil, nil, nil, nil, nil, zaptest.NewLogger(t), "example.com")
	remoteID := "https://remote.example/users/bob"
	conversation := &models.Conversation{
		ID:           "conv-remote",
		Participants: []string{"alice", remoteID},
		ParticipantRefs: []models.ConversationParticipantRef{
			{ParticipantType: models.ConversationParticipantTypeLocalUser, ParticipantID: "alice"},
			{ParticipantType: models.ConversationParticipantTypeRemoteActor, ParticipantID: remoteID, Acct: "bob@remote.example", Domain: "remote.example"},
		},
	}
	sendCmd := &SendDirectMessageCommand{SenderID: "alice", Recipients: []string{remoteID}}

	accountRepo.On("GetAccount", ctx, "alice").Return(createTestAccount("alice", "alice"), nil).Once()

	sender, recipient, err := service.getSendMessageAccountsForSend(ctx, sendCmd, conversation, "alice", remoteID)
	require.NoError(t, err)
	require.Equal(t, "alice", sender.User.Username)
	require.Equal(t, remoteID, recipient.User.ID)
	require.Equal(t, remoteID, recipient.User.Username)
	require.Equal(t, "bob", recipient.User.DisplayName)
	require.NotNil(t, recipient.Actor)
	require.Equal(t, remoteID, recipient.Actor.ID)
	require.Equal(t, "bob", recipient.Actor.PreferredUsername)
	require.Equal(t, models.ConversationParticipantTypeRemoteActor, sendCmd.ResolvedRecipientRef.ParticipantType)
	require.Same(t, recipient.Actor, sendCmd.ResolvedRecipientActor)
	accountRepo.AssertExpectations(t)
}
