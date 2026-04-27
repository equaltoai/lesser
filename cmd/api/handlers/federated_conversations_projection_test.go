package handlers

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/require"
)

func TestConversationAPIAccounts_RemoteParticipantUsesSyntheticAccountFallback(t *testing.T) {
	h := &Handler{cfg: &config.Config{Domain: "example.com"}}
	remoteActorID := "https://remote.example/users/bob"
	conversation := &storageModels.Conversation{
		ID:           "conv-remote",
		Participants: []string{"alice", remoteActorID},
		ParticipantRefs: []storageModels.ConversationParticipantRef{
			{
				ParticipantType: storageModels.ConversationParticipantTypeLocalUser,
				ParticipantID:   "alice",
			},
			{
				ParticipantType: storageModels.ConversationParticipantTypeRemoteActor,
				ParticipantID:   remoteActorID,
				Acct:            "bob@remote.example",
				Domain:          "remote.example",
			},
		},
	}

	accounts := h.conversationAPIAccounts(context.Background(), conversation, "alice", nil)

	require.Len(t, accounts, 1)
	require.Equal(t, remoteActorID, accounts[0].ID)
	require.Equal(t, "bob", accounts[0].Username)
	require.Equal(t, "bob@remote.example", accounts[0].Acct)
	require.Equal(t, remoteActorID, accounts[0].URL)
}

func TestConversationAPIProjectionHelpers_RemoteParticipantBranches(t *testing.T) {
	ctx := context.Background()
	remoteActorID := "https://remote.example/users/bob"
	conversation := &storageModels.Conversation{
		ID:           "conv-remote",
		Participants: []string{" Alice ", remoteActorID},
		ViewerState: &storageModels.UserConversationState{
			CounterpartType: storageModels.ConversationParticipantTypeRemoteActor,
			CounterpartID:   "https://remote.example/users/carol",
			CounterpartAcct: "carol@remote.example",
		},
	}

	refs := conversationParticipantRefsForProjection(conversation)
	require.Len(t, refs, 3)
	require.ElementsMatch(t, []string{remoteActorID, "https://remote.example/users/carol"}, conversationParticipantUsernames(conversation, "alice"))

	localAccount := &storage.Account{
		User: &storage.User{ID: "alice-id", Username: "Alice"},
		Actor: &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/Alice", Type: activitypub.PersonType},
			URL:               "https://example.com/@Alice",
			PreferredUsername: "Alice",
		},
	}
	keys := conversationAPIAccountPrefetchKeys(localAccount)
	require.Contains(t, keys, "alice")
	require.Contains(t, keys, "alice-id")
	require.Contains(t, keys, "https://example.com/users/alice")
	require.Nil(t, conversationAPIAccountPrefetchKeys(nil))

	statusesByID := buildConversationAPIStatusMap([]*storageModels.Status{
		nil,
		{StatusID: ""},
		{StatusID: "status-1"},
	})
	require.Len(t, statusesByID, 1)
	require.Contains(t, statusesByID, "status-1")

	remoteAccount := storageAccountFromActor(&activitypub.Actor{
		BaseObject:        activitypub.BaseObject{ID: remoteActorID, Type: activitypub.PersonType},
		PreferredUsername: "bob",
		Name:              "Bob Remote",
		URL:               remoteActorID,
	}, "example.com")
	prefetch := &conversationAPIPrefetch{
		accountsByKey: buildConversationAPIAccountMap([]*storage.Account{localAccount, remoteAccount}),
	}
	h := &Handler{cfg: &config.Config{Domain: "example.com"}}

	require.Same(t, localAccount, h.conversationAPIAccountForParticipantRef(ctx, storageModels.ConversationParticipantRef{
		ParticipantType: storageModels.ConversationParticipantTypeLocalUser,
		ParticipantID:   "Alice",
	}, prefetch))
	require.Same(t, remoteAccount, h.conversationAPIAccountForParticipantRef(ctx, storageModels.ConversationParticipantRef{
		ParticipantType: storageModels.ConversationParticipantTypeRemoteActor,
		ParticipantID:   remoteActorID,
		Acct:            "bob@remote.example",
	}, prefetch))

	require.Equal(t, "zoe", conversationAPISyntheticRemoteActor(storageModels.ConversationParticipantRef{
		ParticipantType: storageModels.ConversationParticipantTypeRemoteActor,
		ParticipantID:   "https://remote.example/actors/@zoe",
	}).PreferredUsername)
	require.Equal(t, "opaque-remote-id", conversationAPISyntheticRemoteActor(storageModels.ConversationParticipantRef{
		ParticipantType: storageModels.ConversationParticipantTypeRemoteActor,
		ParticipantID:   "opaque-remote-id",
	}).PreferredUsername)
}

func TestConversationAPIRemoteAccountForParticipantRef_UsesCachedRemoteActor(t *testing.T) {
	ctx := context.Background()
	remoteActorID := "https://remote.example/users/bob"
	repos := &MockRepositoryStorage{}
	actorRepo := &testmocks.MockActorRepository{}
	repos.On("Actor").Return(actorRepo).Maybe()
	actorRepo.On("GetCachedRemoteActor", ctx, remoteActorID).Return(&activitypub.Actor{
		BaseObject:        activitypub.BaseObject{ID: remoteActorID, Type: activitypub.PersonType},
		PreferredUsername: "bob",
		Name:              "Cached Bob",
		URL:               remoteActorID,
	}, nil).Once()

	h := &Handler{cfg: &config.Config{Domain: "example.com"}, repos: repos}
	account := h.conversationAPIRemoteAccountForParticipantRef(ctx, storageModels.ConversationParticipantRef{
		ParticipantType: storageModels.ConversationParticipantTypeRemoteActor,
		ParticipantID:   remoteActorID,
		Acct:            "bob@remote.example",
	})

	require.NotNil(t, account)
	require.Equal(t, "bob@remote.example", account.User.Username)
	require.Equal(t, "Cached Bob", account.User.DisplayName)
	require.Equal(t, remoteActorID, account.Actor.ID)
	actorRepo.AssertExpectations(t)
	repos.AssertExpectations(t)
}

func TestConversationAPIProjectionHelpers_EdgeBranches(t *testing.T) {
	ctx := context.Background()

	require.Nil(t, prefetchedConversationAccount(nil, "alice"))
	require.Nil(t, prefetchedConversationAccount(withPrefetchedConversationAccounts(ctx, nil), "alice"))
	require.Empty(t, conversationPreviewStatusID(nil))
	require.Nil(t, conversationParticipantUsernames(nil, "alice"))
	require.Nil(t, conversationParticipantRefsForProjection(nil))

	viewerOnly := &storageModels.Conversation{
		ParticipantRefs: []storageModels.ConversationParticipantRef{{
			ParticipantType: storageModels.ConversationParticipantTypeLocalUser,
			ParticipantID:   "alice",
		}},
		ViewerState: &storageModels.UserConversationState{
			CounterpartID: "Bob",
		},
	}
	require.Equal(t, []string{"bob"}, conversationParticipantUsernames(viewerOnly, "alice"))

	remoteCaseDuplicates := &storageModels.Conversation{
		ParticipantRefs: []storageModels.ConversationParticipantRef{
			{ParticipantType: storageModels.ConversationParticipantTypeRemoteActor, ParticipantID: "https://remote.example/users/Bob"},
			{ParticipantType: storageModels.ConversationParticipantTypeRemoteActor, ParticipantID: "https://remote.example/users/bob"},
		},
	}
	require.Equal(t, []string{"https://remote.example/users/Bob"}, conversationParticipantUsernames(remoteCaseDuplicates, "alice"))

	duplicatesAndBlanks := &storageModels.Conversation{
		Participants: []string{" Alice ", "", "ALICE"},
	}
	require.Empty(t, conversationParticipantUsernames(duplicatesAndBlanks, "alice"))
	require.Len(t, conversationParticipantRefsForProjection(duplicatesAndBlanks), 1)

	var nilHandler *Handler
	require.Empty(t, nilHandler.loadConversationAPIPrefetch(ctx, []*storageModels.Conversation{{}}, "alice").accountsByKey)
	nilHandler.addRemoteConversationAPIAccountsToPrefetch(ctx, []*storageModels.Conversation{{}}, &conversationAPIPrefetch{accountsByKey: map[string]*storage.Account{}})
	require.Nil(t, nilHandler.conversationAPIAccountForParticipant(ctx, "alice", nil))
	require.Nil(t, nilHandler.conversationAPIRemoteAccountForParticipantRef(ctx, storageModels.ConversationParticipantRef{
		ParticipantType: storageModels.ConversationParticipantTypeRemoteActor,
		ParticipantID:   "",
	}))
	require.Nil(t, (&Handler{}).conversationAPIAccountForParticipantRef(ctx, storageModels.ConversationParticipantRef{}, nil))
	require.NotNil(t, conversationAPIStatusContext(nil, nil))

	derivedAccount := &storage.Account{
		User:  &storage.User{Username: "bob"},
		Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/bob", Type: activitypub.PersonType}},
	}
	prefetch := &conversationAPIPrefetch{accountsByKey: map[string]*storage.Account{"bob": derivedAccount}}
	repos := &MockRepositoryStorage{}
	actorRepo := &testmocks.MockActorRepository{}
	repos.On("Actor").Return(actorRepo).Maybe()
	h := &Handler{cfg: &config.Config{Domain: "example.com"}, repos: repos}
	require.Same(t, derivedAccount, h.conversationAPIAccountForParticipant(ctx, "bob", prefetch))
	require.Same(t, derivedAccount, h.conversationAPIAccountForParticipant(ctx, "https://example.com/users/bob", prefetch))

	acctOnlyRemote := &storage.Account{
		User:  &storage.User{Username: "acct-only@remote.example"},
		Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://remote.example/users/acct-only", Type: activitypub.PersonType}},
	}
	require.Same(t, acctOnlyRemote, h.conversationAPIAccountForParticipantRef(ctx, storageModels.ConversationParticipantRef{
		ParticipantType: storageModels.ConversationParticipantTypeRemoteActor,
		ParticipantID:   "https://remote.example/users/acct-only",
		Acct:            "acct-only@remote.example",
	}, &conversationAPIPrefetch{accountsByKey: map[string]*storage.Account{"acct-only@remote.example": acctOnlyRemote}}))

	actorRepo.On("GetActor", ctx, "missing").Return(nil, storage.ErrNotFound).Once()
	require.Nil(t, h.conversationAPIAccountForParticipant(ctx, "missing", nil))

	actorRepo.On("GetCachedRemoteActor", ctx, "https://remote.example/users/missing").Return(nil, storage.ErrNotFound).Once()
	account := h.conversationAPIRemoteAccountForParticipantRef(ctx, storageModels.ConversationParticipantRef{
		ParticipantType: storageModels.ConversationParticipantTypeRemoteActor,
		ParticipantID:   "https://remote.example/users/missing",
	})
	require.NotNil(t, account)
	require.Equal(t, "missing@remote.example", account.User.Username)
	actorRepo.On("GetCachedRemoteActor", ctx, "https://remote.example/users/prefetch-missing").Return(nil, storage.ErrNotFound).Once()
	remotePrefetch := &conversationAPIPrefetch{accountsByKey: map[string]*storage.Account{}}
	h.addRemoteConversationAPIAccountsToPrefetch(ctx, []*storageModels.Conversation{{
		ParticipantRefs: []storageModels.ConversationParticipantRef{{
			ParticipantType: storageModels.ConversationParticipantTypeRemoteActor,
			ParticipantID:   "https://remote.example/users/prefetch-missing",
		}},
	}}, remotePrefetch)
	require.NotEmpty(t, remotePrefetch.accountsByKey)
	require.Empty(t, conversationAPISyntheticRemoteActor(storageModels.ConversationParticipantRef{}).PreferredUsername)

	require.Nil(t, h.conversationAPIStatus(ctx, &storageModels.Conversation{}, nil))
	statusRepo := &testmocks.MockStatusRepositoryInterface{}
	statusRepos := &MockRepositoryStorage{}
	statusRepos.On("Status").Return(statusRepo).Maybe()
	statusRepo.On("GetStatus", ctx, "status-missing").Return(nil, storage.ErrNotFound).Once()
	require.Nil(t, (&Handler{repos: statusRepos}).conversationAPIStatus(ctx, &storageModels.Conversation{LastStatusID: "status-missing"}, &conversationAPIPrefetch{statusesByID: map[string]*storageModels.Status{}}))
	require.Empty(t, (&Handler{cfg: &config.Config{Domain: "example.com"}}).conversationAPIAccounts(ctx, &storageModels.Conversation{
		Participants: []string{"alice", "missing"},
	}, "alice", nil))

	actorRepo.AssertExpectations(t)
	repos.AssertExpectations(t)
	statusRepo.AssertExpectations(t)
	statusRepos.AssertExpectations(t)
}
