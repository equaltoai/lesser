package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/config"
	pkgerrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/services/search"
	"github.com/equaltoai/lesser/pkg/storage"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestRound12QueryResolvers_Notes_ObjectTimelineSearch(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	ctx := context.Background()

	require.NoError(t, storageRepo.Status().CreateStatus(ctx, &storageModels.Status{
		StatusID:       "status-1",
		AuthorID:       config.Get().ActorURL("alice"),
		AuthorUsername: "alice",
		Content:        "hello world",
		CreatedAt:      time.Now().Add(-time.Hour),
		UpdatedAt:      time.Now().Add(-time.Minute),
		Visibility:     storageModels.VisibilityPublic,
	}))

	originalBoostFn := viewerBoostStateResolverFunc
	viewerBoostStateResolverFunc = func(context.Context, *Resolver, string, string) (bool, error) { return false, nil }
	t.Cleanup(func() { viewerBoostStateResolverFunc = originalBoostFn })

	q := resolver.Query()

	obj, err := q.Object(ctx, "status-1")
	require.NoError(t, err)
	require.NotNil(t, obj)

	_, err = q.Timeline(context.Background(), model.TimelineTypeHome, nil, nil, nil, nil, nil, nil, nil)
	require.Error(t, err)

	first := 5
	conn, err := q.Timeline(context.Background(), model.TimelineTypePublic, nil, nil, nil, &first, nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, conn)

	_, err = q.Timeline(context.Background(), model.TimelineTypeHashtag, nil, nil, nil, nil, nil, nil, nil)
	require.Error(t, err)

	_, err = q.Timeline(context.Background(), model.TimelineType("NOPE"), nil, nil, nil, nil, nil, nil, nil)
	require.Error(t, err)

	result, err := q.Search(context.Background(), "alice", nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestQueryResolverTimeline_ListRequiresAuthAndWiresListID(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	query := resolver.Query()
	listID := "list-1"

	t.Run("anonymous list matches authenticated timeline error shape", func(t *testing.T) {
		_, err := query.Timeline(context.Background(), model.TimelineTypeList, nil, &listID, nil, nil, nil, nil, nil)
		require.ErrorIs(t, err, ErrAuthenticationRequired)

		for _, timelineType := range []model.TimelineType{model.TimelineTypeHome, model.TimelineTypeDirect} {
			_, siblingErr := query.Timeline(context.Background(), timelineType, nil, nil, nil, nil, nil, nil, nil)
			require.Error(t, siblingErr)
			require.Equal(t, pkgerrors.GetErrorCode(siblingErr), pkgerrors.GetErrorCode(err))
			require.Equal(t, pkgerrors.GetErrorCategory(siblingErr), pkgerrors.GetErrorCategory(err))
			require.Equal(t, pkgerrors.GetHTTPStatus(siblingErr), pkgerrors.GetHTTPStatus(err))
		}
	})

	t.Run("list ID validation precedes authentication", func(t *testing.T) {
		emptyListID := ""
		for _, test := range []struct {
			name   string
			listID *string
		}{
			{name: "missing"},
			{name: "empty", listID: &emptyListID},
		} {
			t.Run(test.name, func(t *testing.T) {
				_, err := query.Timeline(context.Background(), model.TimelineTypeList, nil, test.listID, nil, nil, nil, nil, nil)
				require.ErrorIs(t, err, ErrListIDParameterRequired)
			})
		}
	})

	t.Run("authenticated list reaches notes service with list ID", func(t *testing.T) {
		connection, err := query.Timeline(
			round12AuthContext("alice"), model.TimelineTypeList, nil, &listID, nil, nil, nil, nil, nil,
		)
		require.NoError(t, err)
		require.NotNil(t, connection)
		require.Empty(t, connection.Edges)
	})
}

func TestRound12QueryResolvers_Notes_ThreadContext_StorageNil(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	resolver.Storage = nil

	q := &queryResolver{resolver}
	_, err := q.ThreadContext(context.Background(), "status-1")
	require.Error(t, err)
}

func TestRound12QueryResolvers_Notes_AnonymousVisibilityContract(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	ctx := context.Background()

	now := time.Now()
	require.NoError(t, storageRepo.Status().CreateStatus(ctx, &storageModels.Status{
		StatusID:       "root-public",
		AuthorID:       config.Get().ActorURL("alice"),
		AuthorUsername: "alice",
		Content:        "shared keyword root",
		CreatedAt:      now.Add(-3 * time.Hour),
		UpdatedAt:      now.Add(-3 * time.Hour),
		Visibility:     storageModels.VisibilityPublic,
	}))
	require.NoError(t, storageRepo.Status().CreateStatus(ctx, &storageModels.Status{
		StatusID:       "reply-public",
		AuthorID:       config.Get().ActorURL("bob"),
		AuthorUsername: "bob",
		Content:        "shared keyword reply public",
		InReplyToID:    "root-public",
		CreatedAt:      now.Add(-2 * time.Hour),
		UpdatedAt:      now.Add(-2 * time.Hour),
		Visibility:     storageModels.VisibilityPublic,
	}))
	require.NoError(t, storageRepo.Status().CreateStatus(ctx, &storageModels.Status{
		StatusID:       "reply-private",
		AuthorID:       config.Get().ActorURL("carol"),
		AuthorUsername: "carol",
		Content:        "shared keyword reply private",
		InReplyToID:    "root-public",
		CreatedAt:      now.Add(-90 * time.Minute),
		UpdatedAt:      now.Add(-90 * time.Minute),
		Visibility:     storageModels.VisibilityPrivate,
	}))
	require.NoError(t, storageRepo.Status().CreateStatus(ctx, &storageModels.Status{
		StatusID:       "reply-direct",
		AuthorID:       config.Get().ActorURL("dave"),
		AuthorUsername: "dave",
		Content:        "shared keyword reply direct",
		InReplyToID:    "root-public",
		CreatedAt:      now.Add(-75 * time.Minute),
		UpdatedAt:      now.Add(-75 * time.Minute),
		Visibility:     storageModels.VisibilityDirect,
	}))
	require.NoError(t, storageRepo.Status().CreateStatus(ctx, &storageModels.Status{
		StatusID:       "status-private",
		AuthorID:       config.Get().ActorURL("erin"),
		AuthorUsername: "erin",
		Content:        "shared keyword private status",
		CreatedAt:      now.Add(-time.Hour),
		UpdatedAt:      now.Add(-time.Hour),
		Visibility:     storageModels.VisibilityPrivate,
	}))

	originalBoostFn := viewerBoostStateResolverFunc
	viewerBoostStateResolverFunc = func(context.Context, *Resolver, string, string) (bool, error) { return false, nil }
	t.Cleanup(func() { viewerBoostStateResolverFunc = originalBoostFn })

	q := resolver.Query()

	obj, err := q.Object(ctx, "status-private")
	require.NoError(t, err)
	require.Nil(t, obj)

	result := (&queryResolver{resolver}).searchResultToGraphQL(ctx, &search.Result{
		Statuses: []search.StatusResult{
			{Status: &storage.StatusSearchResult{StatusID: "root-public"}},
			{Status: &storage.StatusSearchResult{StatusID: "reply-public"}},
			{Status: &storage.StatusSearchResult{StatusID: "reply-private"}},
			{Status: &storage.StatusSearchResult{StatusID: "reply-direct"}},
			{Status: &storage.StatusSearchResult{StatusID: "status-private"}},
		},
	}, "")
	require.NotNil(t, result)

	statusIDs := make([]string, 0, len(result.Statuses))
	for _, status := range result.Statuses {
		if status == nil {
			continue
		}
		statusIDs = append(statusIDs, status.ID)
	}
	require.Contains(t, statusIDs, "root-public")
	require.Contains(t, statusIDs, "reply-public")
	require.NotContains(t, statusIDs, "reply-private")
	require.NotContains(t, statusIDs, "reply-direct")
	require.NotContains(t, statusIDs, "status-private")

	thread, err := q.ThreadContext(ctx, "root-public")
	require.NoError(t, err)
	require.NotNil(t, thread)
	require.NotNil(t, thread.RootNote)
	require.Equal(t, "root-public", thread.RootNote.ID)

	descendantIDs := make([]string, 0, len(thread.Descendants))
	for _, descendant := range thread.Descendants {
		if descendant == nil {
			continue
		}
		descendantIDs = append(descendantIDs, descendant.ID)
	}
	require.Equal(t, []string{"reply-public"}, descendantIDs)

	hiddenThread, err := q.ThreadContext(ctx, "status-private")
	require.NoError(t, err)
	require.Nil(t, hiddenThread)
}
