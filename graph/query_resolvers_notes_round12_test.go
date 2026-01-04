package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/config"
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

	_, err = q.Timeline(context.Background(), model.TimelineTypeHome, nil, nil, nil, nil, nil, nil)
	require.Error(t, err)

	first := 5
	conn, err := q.Timeline(context.Background(), model.TimelineTypePublic, nil, nil, nil, &first, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, conn)

	_, err = q.Timeline(context.Background(), model.TimelineTypeHashtag, nil, nil, nil, nil, nil, nil)
	require.Error(t, err)

	_, err = q.Timeline(context.Background(), model.TimelineType("NOPE"), nil, nil, nil, nil, nil, nil)
	require.Error(t, err)

	result, err := q.Search(context.Background(), "alice", nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestRound12QueryResolvers_Notes_ThreadContext_StorageNil(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	resolver.Storage = nil

	q := &queryResolver{resolver}
	_, err := q.ThreadContext(context.Background(), "status-1")
	require.Error(t, err)
}

