package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestMyDraftsFiltersBeforePaginationAndReportsExactCount(t *testing.T) {
	resolver, store := newRound12GraphResolver(t)
	ctx := context.Background()
	now := time.Now().UTC()
	inputs := []struct {
		id          string
		contentType string
		status      string
	}{
		{id: "a", contentType: "Article", status: "draft"},
		{id: "b", contentType: "Note", status: "draft"},
		{id: "c", contentType: "Article", status: "scheduled"},
		{id: "d", contentType: "Article", status: "draft"},
		{id: "e", contentType: "Article", status: "draft"},
	}
	for _, input := range inputs {
		draft := &models.Draft{ID: input.id, AuthorID: "owner", ContentType: input.contentType, Content: input.id, ContentFormat: "markdown", Status: input.status, CreatedAt: now, UpdatedAt: now, LastSavedAt: now}
		require.NoError(t, draft.UpdateKeys())
		require.NoError(t, store.Draft().CreateDraft(ctx, draft))
	}

	contentType := model.ObjectTypeArticle
	status := model.DraftStatusDraft
	first := 2
	page, err := resolver.Query().MyDrafts(round12AuthContext("owner"), &contentType, &status, &first, nil)
	require.NoError(t, err)
	require.Equal(t, 3, page.TotalCount)
	require.Len(t, page.Edges, 2)
	require.Equal(t, "a", page.Edges[0].Node.ID)
	require.Equal(t, "d", page.Edges[1].Node.ID)
	require.True(t, page.PageInfo.HasNextPage)

	after := page.Edges[1].Cursor
	next, err := resolver.Query().MyDrafts(round12AuthContext("owner"), &contentType, &status, &first, &after)
	require.NoError(t, err)
	require.Equal(t, 3, next.TotalCount)
	require.Len(t, next.Edges, 1)
	require.Equal(t, "e", next.Edges[0].Node.ID)
	require.False(t, next.PageInfo.HasNextPage)
}

func TestDraftProjectsTypedReviewVerdict(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	draft := resolver.convertCMSDraft(round12AuthContext("owner"), &models.Draft{ID: "d1", AuthorID: "owner", ContentType: "Article", ContentFormat: "markdown", Status: "draft", ReviewStatus: "approved"})
	require.NotNil(t, draft.ReviewVerdict)
	require.Equal(t, model.DraftReviewVerdictApproved, *draft.ReviewVerdict)

	draft = resolver.convertCMSDraft(round12AuthContext("owner"), &models.Draft{ID: "d2", AuthorID: "owner", ContentType: "Article", ContentFormat: "markdown", Status: "draft"})
	require.Nil(t, draft.ReviewVerdict)
}

func TestPaginateOwnedDraftReviewsUsesStableGrantCursor(t *testing.T) {
	grants := []*models.DraftReviewGrant{
		{OwnerID: "owner", DraftID: "a", Reviewer: "one", SK: "GRANT#a#REVIEWER#one"},
		{OwnerID: "owner", DraftID: "b", Reviewer: "two", SK: "GRANT#b#REVIEWER#two"},
		{OwnerID: "owner", DraftID: "c", Reviewer: "three", SK: "GRANT#c#REVIEWER#three"},
	}
	page, hasNext := paginateOwnedDraftReviews(grants, 1, "GRANT#a#REVIEWER#one")
	require.True(t, hasNext)
	require.Len(t, page, 1)
	require.Equal(t, "b", page[0].DraftID)
}
