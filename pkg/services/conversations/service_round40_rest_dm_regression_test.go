package conversations

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestExtractMentionHandles_SupportsRemoteAndLocalMentions(t *testing.T) {
	t.Parallel()

	mentions := ExtractMentionHandles("hello @bob@example.com and @carol, looping back to @bob@example.com")
	require.Equal(t, []string{"bob@example.com", "carol", "bob@example.com"}, mentions)
}

func TestExtractMentionHandles_PreservesCompleteHyphenatedUsernames(t *testing.T) {
	t.Parallel()

	mentions := ExtractMentionHandles(
		"hello @prototype-11 and @prototype, plus remote @user-name@remote.example",
	)
	require.Equal(t, []string{"prototype-11", "prototype", "user-name@remote.example"}, mentions)
}

func TestDirectMessageRecipientResolutionPrefersCompleteExactUsername(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	accounts := map[string]*storage.Account{
		"prototype":    createTestAccount("prototype", "prototype"),
		"prototype-11": createTestAccount("prototype-11", "prototype-11"),
	}

	for _, expectedUsername := range []string{"prototype-11", "prototype"} {
		expectedUsername := expectedUsername
		t.Run(expectedUsername, func(t *testing.T) {
			t.Parallel()

			service, _, _, accountRepo, _, _ := createTestService()
			sender := createTestAccount("prototype-13", "prototype-13")
			accountRepo.On("GetAccount", ctx, "prototype-13").Return(sender, nil).Once()
			for username, account := range accounts {
				accountRepo.On("GetAccount", ctx, username).Return(account, nil).Maybe()
			}

			recipients := ExtractMentionHandles("@" + expectedUsername + " exact recipient")
			require.Len(t, recipients, 1)
			cmd := &SendDirectMessageCommand{
				SenderID:   "prototype-13",
				Recipients: recipients,
				Content:    "@" + expectedUsername + " exact recipient",
			}

			_, recipient, resolvedAccounts, err := service.getDirectMessageAccounts(ctx, cmd, recipients[0])
			require.NoError(t, err)
			require.Same(t, accounts[expectedUsername], recipient)
			require.Same(t, accounts[expectedUsername], resolvedAccounts[expectedUsername])
			accountRepo.AssertExpectations(t)
		})
	}
}

func TestExtractMentionHandles_SupportsStartOfStringMentions(t *testing.T) {
	t.Parallel()

	mentions := ExtractMentionHandles("@alice hello and @bob@example.com hi")
	require.Equal(t, []string{"alice", "bob@example.com"}, mentions)
}

func TestExtractMentionHandles_IgnoresEmailsAndSupportsPunctuation(t *testing.T) {
	t.Parallel()

	mentions := ExtractMentionHandles("email me at user@example.com, then hey,@bob and @alice please")
	require.Equal(t, []string{"bob", "alice"}, mentions)
}

func TestService_BuildActivityPubNote_IncludesSpoilerAndAgentAttribution(t *testing.T) {
	t.Parallel()

	service, _, _, _, _, _ := createTestService()

	attr := &activitypub.AgentPostAttribution{
		TriggerType: "mention",
	}
	cmd := &SendDirectMessageCommand{
		SenderID:         "sender123",
		Recipients:       []string{"recipient456"},
		Content:          "secret hello",
		Sensitive:        true,
		SpoilerText:      "cw",
		AgentAttribution: attr,
	}
	senderAccount := createTestAccount("sender123", "alice")
	recipientAccounts := map[string]*storage.Account{
		"recipient456": createTestAccount("recipient456", "bob"),
	}

	note, err := service.buildActivityPubNote(cmd, "message123", senderAccount, "conv123", recipientAccounts)

	require.NoError(t, err)
	require.Equal(t, "cw", note.Summary)
	require.Same(t, attr, note.AgentAttribution)
	require.Equal(t, []string{"https://example.com/users/bob"}, note.To)
	require.Len(t, note.Tag, 1)
	require.Equal(t, activitypub.Tag{
		Type: "Mention",
		Href: "https://example.com/users/bob",
		Name: "@bob",
	}, note.Tag[0])
}

func TestService_BuildActivityPubNote_UsesRemoteActorIDsForMentionAudience(t *testing.T) {
	t.Parallel()

	service, _, _, _, _, _ := createTestService()

	cmd := &SendDirectMessageCommand{
		SenderID:   "sender123",
		Recipients: []string{"https://remote.example/users/bob"},
		Content:    "secret hello",
	}
	senderAccount := createTestAccount("sender123", "alice")
	recipientAccounts := map[string]*storage.Account{
		"https://remote.example/users/bob": {
			User: &storage.User{
				Username: "bob@remote.example",
			},
			Actor: &activitypub.Actor{
				BaseObject: activitypub.BaseObject{
					ID: "https://remote.example/users/bob",
				},
				PreferredUsername: "bob",
			},
		},
	}

	note, err := service.buildActivityPubNote(cmd, "message123", senderAccount, "conv123", recipientAccounts)

	require.NoError(t, err)
	require.Equal(t, []string{"https://remote.example/users/bob"}, note.To)
	require.Len(t, note.Tag, 1)
	require.Equal(t, activitypub.Tag{
		Type: "Mention",
		Href: "https://remote.example/users/bob",
		Name: "@bob@remote.example",
	}, note.Tag[0])
}

func TestService_BuildActivityPubNote_RejectsRemoteHandleWithoutActorID(t *testing.T) {
	t.Parallel()

	service, _, _, _, _, _ := createTestService()

	cmd := &SendDirectMessageCommand{
		SenderID:   "sender123",
		Recipients: []string{"bob@remote.example"},
		Content:    "secret hello",
	}
	senderAccount := createTestAccount("sender123", "alice")
	recipientAccounts := map[string]*storage.Account{
		"bob@remote.example": {
			User: &storage.User{
				Username: "bob",
			},
		},
	}

	note, err := service.buildActivityPubNote(cmd, "message123", senderAccount, "conv123", recipientAccounts)

	require.Nil(t, note)
	require.ErrorIs(t, err, ErrInvalidRecipient)
	require.ErrorIs(t, err, errDirectMessageRemoteRecipientActorRequired)
}
