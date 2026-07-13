package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/services/search"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	theorydberrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	dynamormmocks "github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func TestRound31QueryResolvers_Search_AccountsHydratePartialServiceActors(t *testing.T) {
	resolver, storage := newRound12GraphResolver(t)

	mockDB := new(dynamormmocks.MockDB)
	exactQuery := new(dynamormmocks.MockQuery)
	prefixQuery := new(dynamormmocks.MockQuery)
	fullActorQuery := new(dynamormmocks.MockQuery)
	displayQuery := new(dynamormmocks.MockQuery)

	searchRepo := repositories.NewSearchRepository(mockDB, "test-table", zap.NewNop(), nil)
	searchRepo.SetDependencies(&round12SearchDeps{relationships: storage.relationshipRepo})
	storage.searchRepo = searchRepo

	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB).Times(4)
	mockDB.On("Model", mock.AnythingOfType("*models.Actor")).Return(exactQuery).Once()
	exactQuery.On("Where", "PK", "=", "ACTOR#arch").Return(exactQuery)
	exactQuery.On("Where", "SK", "=", "PROFILE").Return(exactQuery)
	exactQuery.On("First", mock.AnythingOfType("*models.Actor")).Return(theorydberrors.ErrItemNotFound)

	mockDB.On("Model", mock.AnythingOfType("*models.Actor")).Return(prefixQuery).Once()
	prefixQuery.On("Index", "gsi1").Return(prefixQuery)
	prefixQuery.On("Where", "gsi1PK", "=", "USERNAME_SEARCH#ar").Return(prefixQuery)
	prefixQuery.On("Where", "gsi1SK", "BEGINS_WITH", "arch").Return(prefixQuery)
	prefixQuery.On("Limit", 20).Return(prefixQuery)
	prefixQuery.On("All", mock.AnythingOfType("*[]models.Actor")).Run(func(args mock.Arguments) {
		matches := args.Get(0).(*[]models.Actor)
		*matches = []models.Actor{{
			PK:       "ACTOR#arch",
			Username: "arch",
			GSI1SK:   "arch",
			Actor: &activitypub.Actor{
				BaseObject:        activitypub.BaseObject{Type: activitypub.ServiceType},
				PreferredUsername: "arch",
				Name:              "Arch",
			},
		}}
	}).Return(nil)

	mockDB.On("Model", mock.AnythingOfType("*models.Actor")).Return(fullActorQuery).Once()
	fullActorQuery.On("Where", "PK", "=", "ACTOR#arch").Return(fullActorQuery)
	fullActorQuery.On("Where", "SK", "=", "PROFILE").Return(fullActorQuery)
	fullActorQuery.On("First", mock.AnythingOfType("*models.Actor")).Run(func(args mock.Arguments) {
		actor := args.Get(0).(*models.Actor)
		actor.Actor = &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: "https://localhost/users/arch", Type: activitypub.ServiceType},
			PreferredUsername: "arch",
			Name:              "Arch",
		}
	}).Return(nil)

	mockDB.On("Model", mock.AnythingOfType("*models.Actor")).Return(displayQuery).Once()
	displayQuery.On("Index", "gsi2").Return(displayQuery)
	displayQuery.On("Where", "gsi2PK", "=", "NAME_SEARCH#ar").Return(displayQuery)
	displayQuery.On("Where", "gsi2SK", "BEGINS_WITH", "arch").Return(displayQuery)
	displayQuery.On("Limit", 20).Return(displayQuery)
	displayQuery.On("All", mock.AnythingOfType("*[]models.Actor")).Run(func(args mock.Arguments) {
		matches := args.Get(0).(*[]models.Actor)
		*matches = []models.Actor{}
	}).Return(nil)

	searchType := QueryTypeAccounts
	result, err := resolver.Query().Search(ctx, "arch", &searchType, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Accounts, 1)
	require.Equal(t, "arch", result.Accounts[0].PreferredUsername)
}

func TestRound31QueryResolvers_Search_ExactRemoteHandleUsesRemoteAwareResolution(t *testing.T) {
	resolver, storage := newRound12GraphResolver(t)
	actorRepo, ok := storage.Actor().(*inmemory.ActorRepository)
	require.True(t, ok)

	remoteActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://remote.example/users/alice",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "alice",
		Name:              "Remote Alice",
		Inbox:             "https://remote.example/users/alice/inbox",
		Outbox:            "https://remote.example/users/alice/outbox",
	}
	actorRepo.SetCachedRemoteActor("alice@remote.example", remoteActor, time.Hour)

	searchType := QueryTypeAccounts
	result, err := resolver.Query().Search(context.Background(), "alice@remote.example", &searchType, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Accounts, 1)
	require.Equal(t, remoteActor.ID, result.Accounts[0].ID)
	require.Equal(t, remoteActor.PreferredUsername, result.Accounts[0].PreferredUsername)
}

func TestRound31QueryResolvers_Search_NormalizesAccountTypeAliasForExactRemoteHandle(t *testing.T) {
	resolver, storage := newRound12GraphResolver(t)
	actorRepo, ok := storage.Actor().(*inmemory.ActorRepository)
	require.True(t, ok)

	remoteActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://remote.example/users/alice",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "alice",
		Name:              "Remote Alice",
		Inbox:             "https://remote.example/users/alice/inbox",
		Outbox:            "https://remote.example/users/alice/outbox",
	}
	actorRepo.SetCachedRemoteActor("alice@remote.example", remoteActor, time.Hour)

	searchType := "ACCOUNT"
	result, err := resolver.Query().Search(context.Background(), "alice@remote.example", &searchType, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Accounts, 1)
	require.Equal(t, remoteActor.ID, result.Accounts[0].ID)
	require.Equal(t, remoteActor.PreferredUsername, result.Accounts[0].PreferredUsername)
	require.Empty(t, result.Statuses)
	require.Empty(t, result.Hashtags)
}

func TestRound31QueryResolvers_SearchResultToGraphQL_PreservesRemoteActorIdentity(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	remoteActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://remote.example/users/alice",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "alice",
		Name:              "Remote Alice",
		Inbox:             "https://remote.example/users/alice/inbox",
		Outbox:            "https://remote.example/users/alice/outbox",
	}

	result := (&queryResolver{resolver}).searchResultToGraphQL(context.Background(), &search.Result{
		Accounts: []search.AccountResult{
			{Actor: remoteActor},
		},
	}, "")
	require.NotNil(t, result)
	require.Len(t, result.Accounts, 1)
	require.Equal(t, remoteActor.ID, result.Accounts[0].ID)
	require.Equal(t, remoteActor.PreferredUsername, result.Accounts[0].PreferredUsername)
}
