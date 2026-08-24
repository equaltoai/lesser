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
	reviews := make([]resolvedDraftReview, 0, len(grants))
	for _, grant := range grants {
		reviews = append(reviews, resolvedDraftReview{grant: grant})
	}
	page, hasNext := paginateResolvedDraftReviews(reviews, 1, "GRANT#a#REVIEWER#one")
	require.True(t, hasNext)
	require.Len(t, page, 1)
	require.Equal(t, "b", page[0].grant.DraftID)
}

func TestDraftReviewQueuesSkipDeletedDraftGrants(t *testing.T) {
	resolver, drafts := newDraftReviewCursorResolver(t)
	ctx := context.Background()
	draft := &models.Draft{ID: "orphan", AuthorID: "owner", ContentType: "Article", Content: "body", ContentFormat: "markdown", Status: "draft"}
	require.NoError(t, drafts.CreateDraft(ctx, draft))
	grant := &models.DraftReviewGrant{
		OwnerID: "owner", DraftID: draft.ID, Reviewer: "reviewer", GrantedAt: time.Now().UTC(),
		ExpiresAt: m2FutureExpiry(), SK: "GRANT#orphan#REVIEWER#reviewer", GSI2SK: "TIME#2026-08-08T00:00:00Z#OWNER#owner#DRAFT#orphan",
	}
	drafts.ownedDraftReviews = []*models.DraftReviewGrant{grant}
	drafts.sharedDraftReviews = []*models.DraftReviewGrant{grant}
	require.NoError(t, drafts.DeleteDraft(ctx, "owner", draft.ID))

	owned, err := resolver.Query().MyDraftReviews(round12AuthContext("owner"), nil, nil)
	require.NoError(t, err)
	require.Zero(t, owned.TotalCount)
	require.Empty(t, owned.Edges)

	shared, err := resolver.Query().SharedDraftReviews(round12AuthContext("reviewer"), nil, nil)
	require.NoError(t, err)
	require.Equal(t, 1, shared.TotalCount, "legacy sparse-index count converges when the grant is revoked; the read must still skip its missing draft")
	require.Empty(t, shared.Edges)
}

func TestOwnedDraftReviewExposesCurrentRevisionGrantAndEligibility(t *testing.T) {
	resolver, drafts := newDraftReviewCursorResolver(t)
	ctx := context.Background()
	draft := &models.Draft{
		ID: "reviewed", AuthorID: "owner", ContentType: "Article", Content: "# Body\n\n<script>alert(1)</script>", ContentFormat: "markdown",
		Status: "draft", AutosaveVersion: 3, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	require.NoError(t, drafts.CreateDraft(ctx, draft))
	grant := &models.DraftReviewGrant{
		OwnerID: "owner", DraftID: draft.ID, Reviewer: "reviewer", GrantedAt: time.Now().UTC(),
		ExpiresAt: m2FutureExpiry(), SK: "GRANT#reviewed#REVIEWER#reviewer", GSI2SK: "TIME#2026-08-08T00:00:00Z#OWNER#owner#DRAFT#reviewed",
	}
	drafts.ownedDraftReviews = []*models.DraftReviewGrant{grant}

	result, err := resolver.Query().MyDraftReviews(round12AuthContext("owner"), nil, nil)
	require.NoError(t, err)
	require.Len(t, result.Edges, 1)
	review := result.Edges[0].Node
	require.Equal(t, 3, review.Revision)
	require.NotEmpty(t, review.ContentHash)
	require.Contains(t, review.Content, "<script>")
	require.NotNil(t, review.RenderedHTML)
	require.NotContains(t, *review.RenderedHTML, "<script>")
	require.Empty(t, review.RenderErrors)
	require.Len(t, review.Grants, 1)
	require.Equal(t, []string{"reviewer"}, review.ActiveReviewerIds)
	require.Equal(t, "owner", review.OwnerID)
	require.Equal(t, 1, review.GrantCount)
	require.False(t, review.GrantsTruncated)
	require.Equal(t, model.DraftReviewGrantStatusActive, review.Grants[0].Status)
	require.Equal(t, "reviewer", review.Grants[0].ReviewerID)
	require.NotNil(t, review.PublishEligibility)
	require.False(t, review.PublishEligibility.Eligible)
	require.Contains(t, review.PublishEligibility.BlockingReasons, "REVIEW_APPROVAL_REQUIRED")
}

func TestSharedDraftReviewExposesGrantedCrossActorSourceAndPreview(t *testing.T) {
	resolver, drafts := newDraftReviewCursorResolver(t)
	ctx := context.Background()
	draft := &models.Draft{
		ID: "cross-actor", AuthorID: "author", ContentType: "Article", Title: "Review me", Slug: "review-me",
		Content: "# Exact source", ContentFormat: "markdown", Status: "draft", AutosaveVersion: 2,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	require.NoError(t, drafts.CreateDraft(ctx, draft))
	grant := &models.DraftReviewGrant{
		OwnerID: "author", DraftID: draft.ID, Reviewer: "reviewer", GrantedAt: time.Now().UTC(),
		ExpiresAt: m2FutureExpiry(), SK: "GRANT#cross-actor#REVIEWER#reviewer", GSI2SK: "TIME#2026-08-08T00:00:00Z#OWNER#author#DRAFT#cross-actor",
	}
	drafts.sharedDraftReviews = []*models.DraftReviewGrant{grant}

	result, err := resolver.Query().SharedDraftReviews(round12AuthContext("reviewer"), nil, nil)
	require.NoError(t, err)
	require.Equal(t, 1, result.TotalCount)
	require.Len(t, result.Edges, 1)
	review := result.Edges[0].Node
	require.Equal(t, "author", review.OwnerID)
	require.Equal(t, draft.Content, review.Content)
	require.Equal(t, draft.Slug, *review.Slug)
	require.NotNil(t, review.RenderedHTML)
	require.Equal(t, []string{"reviewer"}, review.ActiveReviewerIds)
}
