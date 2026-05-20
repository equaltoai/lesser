package cms

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/cmsrender"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestM7ArticleRenderFailureEmitsOperationalMetricLog(t *testing.T) {
	t.Setenv("STAGE", "dev")

	core, observed := observer.New(zap.DebugLevel)
	svc := NewArticleService(
		&fakeArticleRepo{articles: map[string]*models.Article{}},
		fakeActorRepo{err: errors.New("no actor")},
		nil,
		nil,
		nil,
		&fakeFederation{},
		zap.New(core),
	)

	err := svc.CreateArticle(context.Background(), &models.Article{
		Object: models.Object{
			ID:      "https://example.com/articles/too-large",
			Type:    activitypub.ArticleType,
			Name:    "Too Large",
			Content: string(make([]byte, cmsrender.MaxArticleSourceBytes+1)),
		},
		Slug:          "too-large",
		ContentFormat: "markdown",
	})
	require.ErrorIs(t, err, cmsrender.ErrArticleContentTooLarge)

	entries := observed.FilterMessage(cmsLogArticleRenderFailure).All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	require.Equal(t, "dev", fields["Stage"])
	require.Equal(t, "create_article", fields["Operation"])
	require.Equal(t, cmsMetricStatusFailure, fields["Status"])
	require.Equal(t, float64(1), fields[cmsMetricArticleRenderFailure])
	require.Equal(t, "source_too_large", fields["error_kind"])
	require.Equal(t, "https://example.com/articles/too-large", fields["article_id"])
	require.Equal(t, int64(cmsrender.MaxArticleSourceBytes+1), fields["source_bytes"])
	require.NotContains(t, fields, "content")
}

func TestM7DraftPreviewRenderFailureEmitsOperationalMetricLog(t *testing.T) {
	t.Setenv("STAGE", "dev")

	core, observed := observer.New(zap.DebugLevel)
	repo := newMemDraftRepo()
	require.NoError(t, repo.CreateDraft(context.Background(), &models.Draft{
		ID:            "draft-1",
		AuthorID:      "alice",
		Title:         "Preview",
		ContentType:   activitypub.ArticleType,
		Content:       "draft body",
		ContentFormat: "asciidoc",
	}))

	svc := NewDraftService(repo, nil, "example.com", false, zap.New(core))
	_, err := svc.PreviewDraft(context.Background(), "alice", "draft-1")
	require.ErrorIs(t, err, cmsrender.ErrUnsupportedContentFormat)

	entries := observed.FilterMessage(cmsLogDraftRenderFailure).All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	require.Equal(t, "preview_draft", fields["Operation"])
	require.Equal(t, float64(1), fields[cmsMetricArticleRenderFailure])
	require.Equal(t, "unsupported_content_format", fields["error_kind"])
	require.Equal(t, "draft-1", fields["draft_id"])
	require.Equal(t, "asciidoc", fields["content_format"])
	require.Equal(t, int64(len("draft body")), fields["source_bytes"])
	require.NotContains(t, fields, "content")
}

func TestM7ArticleFederationFailureEmitsOperationalMetricLog(t *testing.T) {
	t.Setenv("STAGE", "dev")

	core, observed := observer.New(zap.DebugLevel)
	svc := NewArticleService(
		&fakeArticleRepo{articles: map[string]*models.Article{}},
		fakeActorRepo{err: errors.New("actor lookup failed")},
		nil,
		nil,
		nil,
		&fakeFederation{},
		zap.New(core),
	)

	article := &models.Article{
		Object: models.Object{
			ID:           "https://example.com/articles/fed",
			Type:         activitypub.ArticleType,
			Name:         "Federation",
			Content:      "hello",
			Published:    time.Now(),
			AttributedTo: "https://example.com/users/alice",
			To:           []string{activitypub.PublicAddress},
		},
		Slug:          "fed",
		ContentFormat: "markdown",
	}

	svc.federateArticleWriteActivity(context.Background(), article, activitypub.CreateType, "create")

	attempts := observed.FilterMessage(cmsLogArticleFederationAttempt).All()
	require.Len(t, attempts, 1)
	require.Equal(t, float64(1), attempts[0].ContextMap()[cmsMetricArticleFederationAttempt])

	failures := observed.FilterMessage(cmsLogArticleFederationOutcome).All()
	require.Len(t, failures, 1)
	fields := failures[0].ContextMap()
	require.Equal(t, "create", fields["Operation"])
	require.Equal(t, cmsMetricStatusFailure, fields["Status"])
	require.Equal(t, cmsFederationFailureStageActor, fields["failure_stage"])
	require.Equal(t, activitypub.CreateType, fields["activity_type"])
	require.Equal(t, float64(1), fields[cmsMetricArticleFederationFailure])
	require.Equal(t, "https://example.com/articles/fed", fields["article_id"])
	require.NotContains(t, fields, "content")
}

func TestM7LegacyArticleMigrationPlanIdentifiesLegacyAliases(t *testing.T) {
	plan := PlanLegacyArticleMigration([]*models.Article{
		{
			Object: models.Object{
				ID:   "https://example.com/objects/abc123",
				Type: activitypub.ArticleType,
			},
			Slug: "Launch Post",
		},
	}, "example.com")

	require.Empty(t, plan.Conflicts)
	require.Empty(t, plan.Skipped)
	require.Len(t, plan.Candidates, 1)
	candidate := plan.Candidates[0]
	require.Equal(t, "https://example.com/objects/abc123", candidate.ArticleID)
	require.Equal(t, "example.com", candidate.Tenant)
	require.Equal(t, "launch-post", candidate.Slug)
	require.Equal(t, candidate.ArticleID, candidate.ProposedCanonicalID)
	require.Equal(t, "https://example.com/articles/launch-post", candidate.ProposedAliasURL)
}

func TestM7LegacyArticleMigrationPlanDetectsDuplicateAndCanonicalConflicts(t *testing.T) {
	plan := PlanLegacyArticleMigration([]*models.Article{
		{
			Object: models.Object{ID: "https://example.com/objects/one", Type: activitypub.ArticleType},
			Slug:   "same",
		},
		{
			Object: models.Object{ID: "https://example.com/objects/two", Type: activitypub.ArticleType},
			Slug:   "same",
		},
		{
			Object: models.Object{ID: "https://example.com/articles/same", Type: activitypub.ArticleType},
			Slug:   "same",
		},
	}, "example.com")

	require.Len(t, plan.Candidates, 2)
	require.Len(t, plan.Conflicts, 2)
	reasons := []string{plan.Conflicts[0].Reason, plan.Conflicts[1].Reason}
	require.Contains(t, reasons, LegacyArticleConflictDuplicate)
	require.Contains(t, reasons, LegacyArticleConflictOccupied)
	for _, conflict := range plan.Conflicts {
		require.Equal(t, "https://example.com/articles/same", conflict.AliasURL)
		require.Equal(t, "same", conflict.Slug)
	}
}

func TestM7LegacyArticleMigrationPlanSkipsNonCandidates(t *testing.T) {
	plan := PlanLegacyArticleMigration([]*models.Article{
		nil,
		{Object: models.Object{ID: "", Type: activitypub.ArticleType}, Slug: "missing-id"},
		{Object: models.Object{ID: "https://example.com/objects/note", Type: activitypub.NoteType}, Slug: "note"},
		{Object: models.Object{ID: "https://example.com/articles/canonical", Type: activitypub.ArticleType}, Slug: "canonical"},
		{Object: models.Object{ID: "https://example.com/objects/no-slug", Type: activitypub.ArticleType}},
		{Object: models.Object{ID: "https://example.com/users/alice/statuses/1", Type: activitypub.ArticleType}, Slug: "status"},
	}, "example.com")

	require.Empty(t, plan.Candidates)
	require.Empty(t, plan.Conflicts)
	require.Len(t, plan.Skipped, 6)
	reasons := make([]string, 0, len(plan.Skipped))
	for _, skipped := range plan.Skipped {
		reasons = append(reasons, skipped.Reason)
	}
	require.Contains(t, reasons, LegacyArticleSkipNil)
	require.Contains(t, reasons, LegacyArticleSkipMissingID)
	require.Contains(t, reasons, LegacyArticleSkipNotArticle)
	require.Contains(t, reasons, LegacyArticleSkipAlreadyCanonical)
	require.Contains(t, reasons, LegacyArticleSkipMissingSlug)
	require.Contains(t, reasons, LegacyArticleSkipNotLegacyObjectID)
}
