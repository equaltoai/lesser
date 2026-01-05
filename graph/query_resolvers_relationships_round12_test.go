package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestRound12QueryResolvers_Relationships_HelperFunctions(t *testing.T) {
	limit := 1
	require.Equal(t, 40, clampLimit(nil))
	require.Equal(t, 1, clampLimit(&limit))

	large := 1000
	require.Equal(t, 80, clampLimit(&large))

	var cursor *model.Cursor
	require.Equal(t, "", cursorToString(cursor))
	require.Nil(t, stringToCursor(""))

	value := model.Cursor("c1")
	require.Equal(t, "c1", cursorToString(&value))
	require.NotNil(t, stringToCursor("c2"))
}

func TestRound12QueryResolvers_Relationships_PagesAndGraph(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	q := &queryResolver{resolver}

	ctx := round12AuthContext("alice")

	// resolveViewerActorListPage: custom fetch path and error path.
	_, err := q.resolveViewerActorListPage(ctx, nil, nil, "x", nil)
	require.Error(t, err)

	first := 10
	page, err := q.resolveViewerActorListPage(ctx, &first, nil, "custom", func(context.Context, string, int, string) ([]*storage.Account, string, error) {
		return []*storage.Account{
			{
				User:  &storage.User{Username: "bob"},
				Actor: &activitypub.Actor{PreferredUsername: "bob"},
			},
			nil,
		}, "next", nil
	})
	require.NoError(t, err)
	require.NotNil(t, page)
	require.Equal(t, 1, page.TotalCount)
	require.NotNil(t, page.NextCursor)

	// DomainBlocks should gracefully handle empty results.
	blocks, err := q.DomainBlocks(ctx, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, blocks)

	// Followers / Following return pages (empty is fine for mocks).
	followers, err := q.Followers(context.Background(), "alice", nil, nil)
	require.NoError(t, err)
	require.NotNil(t, followers)

	following, err := q.Following(context.Background(), "alice", nil, nil)
	require.NoError(t, err)
	require.NotNil(t, following)

	// Relationship lookup should return a model even when no relationship exists.
	rel, err := q.Relationship(ctx, "bob")
	require.NoError(t, err)
	require.NotNil(t, rel)

	// Relationships should populate fallback models when service errors.
	rels, err := q.Relationships(ctx, []string{"", "bob"})
	require.NoError(t, err)
	require.Len(t, rels, 2)

	// Seed trust relationships and verify TrustGraph edges.
	trustRepo := storageRepo.Trust()
	require.NoError(t, trustRepo.CreateTrustRelationship(context.Background(), &storage.TrustRelationship{
		ID:         "t1",
		TrusterID:  "alice",
		TrusteeID:  "bob",
		Category:   storageModels.TrustCategoryContent,
		Score:      0.8,
		Confidence: 0.5,
		Created:    time.Now(),
		Updated:    time.Now(),
	}))
	require.NoError(t, trustRepo.CreateTrustRelationship(context.Background(), &storage.TrustRelationship{
		ID:         "t2",
		TrusterID:  "carol",
		TrusteeID:  "alice",
		Category:   storageModels.TrustCategoryBehavior,
		Score:      0.4,
		Confidence: 1.0,
		Created:    time.Now(),
		Updated:    time.Now(),
	}))

	graph, err := q.TrustGraph(ctx, "alice", nil)
	require.NoError(t, err)
	require.NotNil(t, graph)

	cat := storageModels.TrustCategoryContent
	filtered, err := q.TrustGraph(ctx, "alice", &cat)
	require.NoError(t, err)
	require.NotNil(t, filtered)
}
