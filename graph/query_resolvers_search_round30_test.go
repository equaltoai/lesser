package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/services/search"
	"github.com/equaltoai/lesser/pkg/storage"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestRound30QueryResolvers_SearchResultToGraphQL_SkipsNilElementsAndLoadsStatuses(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	ctx := context.Background()

	require.NoError(t, storageRepo.Status().CreateStatus(ctx, &storageModels.Status{
		StatusID:       "status-1",
		AuthorID:       config.Get().ActorURL("bob"),
		AuthorUsername: "bob",
		Content:        "hello world",
		CreatedAt:      time.Now().Add(-time.Hour),
		UpdatedAt:      time.Now().Add(-time.Minute),
		Visibility:     storageModels.VisibilityPublic,
	}))

	originalBoostFn := viewerBoostStateResolverFunc
	viewerBoostStateResolverFunc = func(context.Context, *Resolver, string, string) (bool, error) { return false, nil }
	t.Cleanup(func() { viewerBoostStateResolverFunc = originalBoostFn })

	q := &queryResolver{resolver}
	out := q.searchResultToGraphQL(ctx, &search.Result{
		Accounts: []search.AccountResult{
			{Actor: nil},
			{Actor: &activitypub.Actor{PreferredUsername: "bob"}},
		},
		Statuses: []search.StatusResult{
			{Status: "not-a-status"},
			{Status: &storage.StatusSearchResult{StatusID: "status-1"}},
		},
		Hashtags: []search.HashtagResult{
			{Name: "golang", URL: "https://localhost/tags/golang"},
		},
	}, "")

	require.NotNil(t, out)
	require.NotNil(t, out.Accounts)
	require.NotNil(t, out.Statuses)
	require.NotNil(t, out.Hashtags)

	require.Len(t, out.Accounts, 1)
	require.NotNil(t, out.Accounts[0])

	require.Len(t, out.Statuses, 1)
	require.NotNil(t, out.Statuses[0])

	require.Len(t, out.Hashtags, 1)
	require.NotNil(t, out.Hashtags[0])
}

func TestRound30QueryResolvers_SearchResultToGraphQL_SelfSearchStillWorks(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	ctx := context.Background()

	require.NoError(t, storageRepo.Status().CreateStatus(ctx, &storageModels.Status{
		StatusID:       "status-self",
		AuthorID:       config.Get().ActorURL("alice"),
		AuthorUsername: "alice",
		Content:        "hello from alice",
		CreatedAt:      time.Now().Add(-time.Hour),
		UpdatedAt:      time.Now().Add(-time.Minute),
		Visibility:     storageModels.VisibilityPublic,
	}))

	originalBoostFn := viewerBoostStateResolverFunc
	viewerBoostStateResolverFunc = func(context.Context, *Resolver, string, string) (bool, error) { return false, nil }
	t.Cleanup(func() { viewerBoostStateResolverFunc = originalBoostFn })

	q := &queryResolver{resolver}
	out := q.searchResultToGraphQL(ctx, &search.Result{
		Accounts: []search.AccountResult{
			{Actor: &activitypub.Actor{PreferredUsername: "alice"}},
		},
		Statuses: []search.StatusResult{
			{Status: &storage.StatusSearchResult{StatusID: "status-self"}},
		},
		Hashtags: []search.HashtagResult{},
	}, "alice")

	require.NotNil(t, out)
	require.NotNil(t, out.Accounts)
	require.NotNil(t, out.Statuses)
	require.NotNil(t, out.Hashtags)

	require.Len(t, out.Accounts, 1)
	require.NotNil(t, out.Accounts[0])
	require.Equal(t, "alice", out.Accounts[0].PreferredUsername)

	require.Len(t, out.Statuses, 1)
	require.NotNil(t, out.Statuses[0])
	require.Equal(t, "status-self", out.Statuses[0].ID)
}
