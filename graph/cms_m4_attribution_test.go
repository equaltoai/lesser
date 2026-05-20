package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestM4CMSConvertersExposeAuthoringAttribution(t *testing.T) {
	t.Parallel()

	resolver := &Resolver{Config: &config.Config{Domain: "example.com"}}
	ctx := context.Background()

	article := resolver.convertCMSArticle(ctx, &models.Article{
		Object: models.Object{
			ID:           "https://example.com/articles/hello",
			Type:         activitypub.ArticleType,
			Name:         "Hello",
			Content:      "content",
			AttributedTo: "https://example.com/users/alice",
			Published:    time.Now(),
			Updated:      time.Now(),
			CreatedAt:    time.Now(),
		},
		Slug:          "hello",
		ContentFormat: "markdown",
		GeneratedBy:   "https://example.com/users/agent-0",
		ReviewedBy:    "https://example.com/users/alice",
		PublishedBy:   "https://example.com/users/alice",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}, false)
	require.NotNil(t, article.GeneratedBy)
	require.Equal(t, "https://example.com/users/agent-0", article.GeneratedBy.ID)
	require.NotNil(t, article.ReviewedBy)
	require.Equal(t, "https://example.com/users/alice", article.ReviewedBy.ID)
	require.NotNil(t, article.PublishedBy)
	require.Equal(t, "https://example.com/users/alice", article.PublishedBy.ID)

	draft := resolver.convertCMSDraft(ctx, &models.Draft{
		ID:            "draft-1",
		AuthorID:      "alice",
		ContentType:   activitypub.ArticleType,
		Content:       "content",
		ContentFormat: "markdown",
		Status:        "draft",
		LastSavedAt:   time.Now(),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		GeneratedBy:   "https://example.com/users/agent-0",
		ReviewedBy:    "https://example.com/users/alice",
	})
	require.NotNil(t, draft.GeneratedBy)
	require.Equal(t, "https://example.com/users/agent-0", draft.GeneratedBy.ID)
	require.NotNil(t, draft.ReviewedBy)
	require.Equal(t, "https://example.com/users/alice", draft.ReviewedBy.ID)

	revision := resolver.convertCMSRevision(ctx, &models.Revision{
		ID:          "rev-1",
		ObjectID:    "https://example.com/articles/hello",
		Version:     1,
		Content:     "content",
		ChangedBy:   "https://example.com/users/alice",
		ChangeType:  "update",
		GeneratedBy: "https://example.com/users/agent-0",
		ReviewedBy:  "https://example.com/users/alice",
		PublishedBy: "https://example.com/users/alice",
		CreatedAt:   time.Now(),
	})
	require.NotNil(t, revision.GeneratedBy)
	require.Equal(t, "https://example.com/users/agent-0", revision.GeneratedBy.ID)
	require.NotNil(t, revision.ReviewedBy)
	require.Equal(t, "https://example.com/users/alice", revision.ReviewedBy.ID)
	require.NotNil(t, revision.PublishedBy)
	require.Equal(t, "https://example.com/users/alice", revision.PublishedBy.ID)
}

func TestM4DraftRequestAttributionUsesAgentAndHumanReviewContext(t *testing.T) {
	t.Parallel()

	mut := &mutationResolver{Resolver: &Resolver{Config: &config.Config{Domain: "example.com"}}}
	draft := &models.Draft{}

	agentCtx := auth.WithClaims(context.Background(), &auth.Claims{
		Username: "agent-0",
		IsAgent:  true,
	})
	mut.cmsApplyDraftRequestAttribution(agentCtx, "agent-0", draft, false)
	require.Equal(t, "https://localhost/users/agent-0", draft.GeneratedBy)
	require.Empty(t, draft.ReviewedBy)

	humanCtx := auth.WithClaims(context.Background(), &auth.Claims{Username: "alice"})
	mut.cmsApplyDraftRequestAttribution(humanCtx, "alice", draft, true)
	require.Equal(t, "https://localhost/users/agent-0", draft.GeneratedBy)
	require.Equal(t, "https://localhost/users/alice", draft.ReviewedBy)
}
