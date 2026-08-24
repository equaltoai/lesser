package services

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	pkgconfig "github.com/equaltoai/lesser/pkg/config"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/services/cms"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// fakeOrphanDrafts pages failed drafts; the first page returns pageOne, the
// second returns pageTwo, and any later page is empty. Publishing-status
// enumeration (the stale-publishing sweep) is served from publishingPageOne /
// publishingPageTwo so tests can distinguish the two candidate sets.
type fakeOrphanDrafts struct {
	pageOne           []*models.Draft
	pageTwo           []*models.Draft
	publishingPageOne []*models.Draft
	publishingPageTwo []*models.Draft
	err               error
}

func (f *fakeOrphanDrafts) ListDraftsByStatusPaginated(_ context.Context, status string, _ int, cursor string) ([]*models.Draft, string, error) {
	if f.err != nil {
		return nil, "", f.err
	}
	if status == cms.DraftStatusPublishing {
		switch cursor {
		case "":
			return f.publishingPageOne, "pub-1", nil
		case "pub-1":
			return f.publishingPageTwo, "", nil
		default:
			return nil, "", nil
		}
	}
	switch cursor {
	case "":
		return f.pageOne, "page-1", nil
	case "page-1":
		return f.pageTwo, "", nil
	default:
		return nil, "", nil
	}
}

type fakeOrphanMedia struct {
	byID map[string]*models.Media
	err  error
}

func (f *fakeOrphanMedia) GetMedia(_ context.Context, mediaID string) (*models.Media, error) {
	if f.err != nil {
		return nil, f.err
	}
	m, ok := f.byID[mediaID]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return m, nil
}

type fakeOrphanArticles struct {
	byID         map[string]*models.Article
	byAuthor     map[string][]*models.Article
	err          error
	listErr      error
	infiniteNext string // when set, the author enumeration never terminates (scan-budget test)
}

func (f *fakeOrphanArticles) GetArticle(_ context.Context, articleID string) (*models.Article, error) {
	if f.err != nil {
		return nil, f.err
	}
	a, ok := f.byID[articleID]
	if !ok {
		return nil, apperrors.ItemNotFoundWithID("article", articleID)
	}
	return a, nil
}

func (f *fakeOrphanArticles) ListArticlesByAuthorPaginated(_ context.Context, authorActorID string, _ int, _ string) ([]*models.Article, string, error) {
	if f.listErr != nil {
		return nil, "", f.listErr
	}
	return f.byAuthor[authorActorID], f.infiniteNext, nil
}

// newOrphanSource wires a fully provisioned orphan source: the article fake
// serves both the direct target lookup and the owning-author cross-article
// scan, and the domain is fixed so author actor IDs resolve deterministically.
func newOrphanSource(drafts *fakeOrphanDrafts, media *fakeOrphanMedia, articles *fakeOrphanArticles, logger *zap.Logger) *cmsOrphanedPublishedMintSource {
	if articles == nil {
		articles = &fakeOrphanArticles{
			byID:     map[string]*models.Article{},
			byAuthor: map[string][]*models.Article{},
		}
	}
	return &cmsOrphanedPublishedMintSource{
		drafts:           drafts,
		media:            media,
		articles:         articles,
		articlesByAuthor: articles,
		domain:           "example.test",
		logger:           logger,
	}
}

func orphanReconcilePublishedMedia(id string) *models.Media {
	now := time.Now().UTC()
	return &models.Media{
		MediaID: id, UserID: "alice", ContentType: "image/png", FileSize: 12,
		ContentHash: "sha256:" + strings.Repeat("m", 64), Status: "ready",
		Visibility: models.MediaVisibilityInternal, S3Bucket: "media-private",
		S3Key: "alice/" + id + ".png", ModelVersion: 2,
		PublishedS3Key: "published/alice/" + id + ".png",
		PublishedURL:   "https://cdn.example.test/published/alice/" + id + ".png",
		PublishedAt:    &now,
	}
}

func orphanFailedDraft(id string, objectID *string, usages ...models.DraftMediaUsage) *models.Draft {
	return &models.Draft{ID: id, AuthorID: "alice", Status: cms.DraftStatusFailed, ObjectID: objectID, EditorialMedia: usages}
}

func orphanPublishingDraft(id string, updatedAt time.Time, usages ...models.DraftMediaUsage) *models.Draft {
	return &models.Draft{ID: id, AuthorID: "alice", Status: cms.DraftStatusPublishing, UpdatedAt: updatedAt, EditorialMedia: usages}
}

// orphanPublishingDraftWithAttempt builds a publishing draft carrying an
// explicit publish-attempt stamp (PublishAttemptedAt); nil models a legacy row
// that predates the attribute.
func orphanPublishingDraftWithAttempt(id string, updatedAt time.Time, attemptedAt *time.Time, usages ...models.DraftMediaUsage) *models.Draft {
	return &models.Draft{ID: id, AuthorID: "alice", Status: cms.DraftStatusPublishing, UpdatedAt: updatedAt, PublishAttemptedAt: attemptedAt, EditorialMedia: usages}
}

// TestIsStalePublishingDraftHorizon proves the sweep horizon keys on the
// publish-attempt stamp (PublishAttemptedAt) written only by the transition,
// with an explicit UpdatedAt fallback for legacy rows, so content writes that
// advance UpdatedAt cannot re-arm a crash-stuck publishing draft's sweep.
func TestIsStalePublishingDraftHorizon(t *testing.T) {
	now := time.Now().UTC()
	old := now.Add(-25 * time.Hour)
	fresh := now.Add(-time.Hour)
	cases := []struct {
		name        string
		draft       *models.Draft
		wantStale   bool
		description string
	}{
		{
			name:        "legacy row without the attribute falls back to a stale UpdatedAt",
			draft:       orphanPublishingDraftWithAttempt("d1", old, nil),
			wantStale:   true,
			description: "a pre-attribute publishing row older than the horizon is still swept",
		},
		{
			name:        "legacy row without the attribute falls back to a fresh UpdatedAt",
			draft:       orphanPublishingDraftWithAttempt("d2", fresh, nil),
			wantStale:   false,
			description: "a legacy row freshly updated is treated as in flight",
		},
		{
			name:        "stale attempt with fresh UpdatedAt is still stale (content writes cannot re-arm)",
			draft:       orphanPublishingDraftWithAttempt("d3", fresh, &old),
			wantStale:   true,
			description: "autosave/update/editorial-media-set advance UpdatedAt; the attempt stamp stays old, so the draft is still swept",
		},
		{
			name:        "fresh attempt with stale UpdatedAt is not stale (a fresh transition re-arms)",
			draft:       orphanPublishingDraftWithAttempt("d4", old, &fresh),
			wantStale:   false,
			description: "a retry transition writes a fresh attempt stamp; the draft is in flight regardless of UpdatedAt age",
		},
		{
			name:        "stale attempt with stale UpdatedAt is stale",
			draft:       orphanPublishingDraftWithAttempt("d5", old, &old),
			wantStale:   true,
			description: "the common crash-stuck case",
		},
		{
			name:        "nil draft is never stale",
			draft:       nil,
			wantStale:   false,
			description: "guard",
		},
		{
			name:        "legacy row with zero UpdatedAt is never stale",
			draft:       orphanPublishingDraftWithAttempt("d6", time.Time{}, nil),
			wantStale:   false,
			description: "a zero fallback timestamp cannot be swept",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.wantStale, isStalePublishingDraft(tc.draft, now), "%s", tc.description)
		})
	}
}

func TestOrphanedPublishedMintSource(t *testing.T) {
	hero := orphanReconcilePublishedMedia("hero")
	objectID := "https://example.test/articles/existing"

	aliceActor := common.GenerateActorID("example.test", "alice")

	t.Run("create-path orphan mint is returned", func(t *testing.T) {
		core, _ := observer.New(zap.DebugLevel)
		src := newOrphanSource(
			&fakeOrphanDrafts{pageOne: []*models.Draft{orphanFailedDraft("d1", nil, models.DraftMediaUsage{MediaID: "hero"})}},
			&fakeOrphanMedia{byID: map[string]*models.Media{"hero": hero}},
			nil,
			zap.New(core),
		)
		ids, err := src.ListOrphanedPublishedMintIDs(context.Background())
		require.NoError(t, err)
		require.Equal(t, []string{"hero"}, ids)
	})

	t.Run("create-path orphan referenced by a different live article is untouched", func(t *testing.T) {
		core, _ := observer.New(zap.DebugLevel)
		other := &models.Article{Object: models.Object{ID: "https://example.test/articles/other"}}
		other.FeaturedImage = &models.Media{MediaID: "hero", S3Key: "published/alice/hero.png"}
		src := newOrphanSource(
			&fakeOrphanDrafts{pageOne: []*models.Draft{orphanFailedDraft("d1", nil, models.DraftMediaUsage{MediaID: "hero"})}},
			&fakeOrphanMedia{byID: map[string]*models.Media{"hero": hero}},
			&fakeOrphanArticles{
				byID:     map[string]*models.Article{},
				byAuthor: map[string][]*models.Article{aliceActor: {other}},
			},
			zap.New(core),
		)
		ids, err := src.ListOrphanedPublishedMintIDs(context.Background())
		require.NoError(t, err)
		require.Empty(t, ids, "a create-path failed draft must not orphan an asset a different live article references")
	})

	t.Run("cross-article reference by published URL is untouched", func(t *testing.T) {
		core, _ := observer.New(zap.DebugLevel)
		other := &models.Article{
			Object:  models.Object{ID: "https://example.test/articles/other"},
			OGImage: "https://cdn.example.test/published/alice/hero.png",
		}
		src := newOrphanSource(
			&fakeOrphanDrafts{pageOne: []*models.Draft{orphanFailedDraft("d1", nil, models.DraftMediaUsage{MediaID: "hero"})}},
			&fakeOrphanMedia{byID: map[string]*models.Media{"hero": hero}},
			&fakeOrphanArticles{
				byID:     map[string]*models.Article{},
				byAuthor: map[string][]*models.Article{aliceActor: {other}},
			},
			zap.New(core),
		)
		ids, err := src.ListOrphanedPublishedMintIDs(context.Background())
		require.NoError(t, err)
		require.Empty(t, ids)
	})

	t.Run("cross-article inline content reference is untouched", func(t *testing.T) {
		core, _ := observer.New(zap.DebugLevel)
		other := &models.Article{
			Object: models.Object{
				ID:      "https://example.test/articles/other",
				Content: "see <img src=\"https://cdn.example.test/published/alice/hero.png\">",
			},
		}
		src := newOrphanSource(
			&fakeOrphanDrafts{pageOne: []*models.Draft{orphanFailedDraft("d1", nil, models.DraftMediaUsage{MediaID: "hero"})}},
			&fakeOrphanMedia{byID: map[string]*models.Media{"hero": hero}},
			&fakeOrphanArticles{
				byID:     map[string]*models.Article{},
				byAuthor: map[string][]*models.Article{aliceActor: {other}},
			},
			zap.New(core),
		)
		ids, err := src.ListOrphanedPublishedMintIDs(context.Background())
		require.NoError(t, err)
		require.Empty(t, ids)
	})

	t.Run("unwired author enumerator fails closed", func(t *testing.T) {
		core, _ := observer.New(zap.DebugLevel)
		src := newOrphanSource(
			&fakeOrphanDrafts{pageOne: []*models.Draft{orphanFailedDraft("d1", nil, models.DraftMediaUsage{MediaID: "hero"})}},
			&fakeOrphanMedia{byID: map[string]*models.Media{"hero": hero}},
			nil,
			zap.New(core),
		)
		src.articlesByAuthor = nil
		ids, err := src.ListOrphanedPublishedMintIDs(context.Background())
		require.NoError(t, err)
		require.Empty(t, ids, "an unverifiable cross-article reference state must never be unpublished")
	})

	t.Run("cross-article scan budget exceeded fails closed", func(t *testing.T) {
		core, _ := observer.New(zap.DebugLevel)
		src := newOrphanSource(
			&fakeOrphanDrafts{pageOne: []*models.Draft{orphanFailedDraft("d1", nil, models.DraftMediaUsage{MediaID: "hero"})}},
			&fakeOrphanMedia{byID: map[string]*models.Media{"hero": hero}},
			&fakeOrphanArticles{
				byID:         map[string]*models.Article{},
				byAuthor:     map[string][]*models.Article{aliceActor: {}},
				infiniteNext: "still-more",
			},
			zap.New(core),
		)
		ids, err := src.ListOrphanedPublishedMintIDs(context.Background())
		require.NoError(t, err)
		require.Empty(t, ids, "a candidate whose reference scan cannot terminate must be skipped")
	})

	t.Run("live article referencing by featured image is untouched", func(t *testing.T) {
		core, _ := observer.New(zap.DebugLevel)
		article := &models.Article{Object: models.Object{ID: objectID}}
		article.FeaturedImage = &models.Media{MediaID: "hero", S3Key: "published/alice/hero.png"}
		src := newOrphanSource(
			&fakeOrphanDrafts{pageOne: []*models.Draft{orphanFailedDraft("d1", &objectID, models.DraftMediaUsage{MediaID: "hero"})}},
			&fakeOrphanMedia{byID: map[string]*models.Media{"hero": hero}},
			&fakeOrphanArticles{byID: map[string]*models.Article{objectID: article}},
			zap.New(core),
		)
		ids, err := src.ListOrphanedPublishedMintIDs(context.Background())
		require.NoError(t, err)
		require.Empty(t, ids, "a published asset referenced by a live article must never be unpublished")
	})

	t.Run("live article referencing by published URL is untouched", func(t *testing.T) {
		core, _ := observer.New(zap.DebugLevel)
		article := &models.Article{
			Object:  models.Object{ID: objectID},
			OGImage: "https://cdn.example.test/published/alice/hero.png",
		}
		src := newOrphanSource(
			&fakeOrphanDrafts{pageOne: []*models.Draft{orphanFailedDraft("d1", &objectID, models.DraftMediaUsage{MediaID: "hero"})}},
			&fakeOrphanMedia{byID: map[string]*models.Media{"hero": hero}},
			&fakeOrphanArticles{byID: map[string]*models.Article{objectID: article}},
			zap.New(core),
		)
		ids, err := src.ListOrphanedPublishedMintIDs(context.Background())
		require.NoError(t, err)
		require.Empty(t, ids)
	})

	t.Run("update-path orphan without a surviving reference is returned", func(t *testing.T) {
		core, _ := observer.New(zap.DebugLevel)
		src := newOrphanSource(
			&fakeOrphanDrafts{pageOne: []*models.Draft{orphanFailedDraft("d1", &objectID, models.DraftMediaUsage{MediaID: "hero"})}},
			&fakeOrphanMedia{byID: map[string]*models.Media{"hero": hero}},
			&fakeOrphanArticles{byID: map[string]*models.Article{}},
			zap.New(core),
		)
		ids, err := src.ListOrphanedPublishedMintIDs(context.Background())
		require.NoError(t, err)
		require.Equal(t, []string{"hero"}, ids)
	})

	t.Run("unpublished media is not returned", func(t *testing.T) {
		core, _ := observer.New(zap.DebugLevel)
		unpublished := *hero
		unpublished.PublishedS3Key = ""
		unpublished.PublishedURL = ""
		unpublished.PublishedAt = nil
		src := newOrphanSource(
			&fakeOrphanDrafts{pageOne: []*models.Draft{orphanFailedDraft("d1", nil, models.DraftMediaUsage{MediaID: "hero"})}},
			&fakeOrphanMedia{byID: map[string]*models.Media{"hero": &unpublished}},
			nil,
			zap.New(core),
		)
		ids, err := src.ListOrphanedPublishedMintIDs(context.Background())
		require.NoError(t, err)
		require.Empty(t, ids)
	})

	t.Run("stale publishing draft with an orphaned mint is reconciled", func(t *testing.T) {
		core, _ := observer.New(zap.DebugLevel)
		// Legacy row: no publish-attempt stamp, stale UpdatedAt — swept via the
		// explicit UpdatedAt fallback.
		stale := orphanPublishingDraft("dp-stale", time.Now().UTC().Add(-25*time.Hour), models.DraftMediaUsage{MediaID: "hero"})
		src := newOrphanSource(
			&fakeOrphanDrafts{publishingPageOne: []*models.Draft{stale}},
			&fakeOrphanMedia{byID: map[string]*models.Media{"hero": hero}},
			nil,
			zap.New(core),
		)
		ids, err := src.ListOrphanedPublishedMintIDs(context.Background())
		require.NoError(t, err)
		require.Equal(t, []string{"hero"}, ids,
			"a crash-stuck publishing draft older than the horizon is a failed-draft candidate")
	})

	t.Run("fresh publishing draft is not reconciled", func(t *testing.T) {
		core, _ := observer.New(zap.DebugLevel)
		// Legacy row: no publish-attempt stamp, fresh UpdatedAt — in flight via
		// the UpdatedAt fallback.
		fresh := orphanPublishingDraft("dp-fresh", time.Now().UTC(), models.DraftMediaUsage{MediaID: "hero"})
		src := newOrphanSource(
			&fakeOrphanDrafts{publishingPageOne: []*models.Draft{fresh}},
			&fakeOrphanMedia{byID: map[string]*models.Media{"hero": hero}},
			nil,
			zap.New(core),
		)
		ids, err := src.ListOrphanedPublishedMintIDs(context.Background())
		require.NoError(t, err)
		require.Empty(t, ids,
			"a publishing draft still inside the horizon is an in-flight publish and must not be reconciled")
	})

	t.Run("publishing draft re-armed by content writes is still reconciled", func(t *testing.T) {
		core, _ := observer.New(zap.DebugLevel)
		// F1 regression: an author who keeps editing a crash-stuck publishing
		// draft advances UpdatedAt but never the publish-attempt stamp, so the
		// sweep must still treat the draft as stale.
		now := time.Now().UTC()
		attempt := now.Add(-25 * time.Hour)
		edited := orphanPublishingDraftWithAttempt("dp-edited", now, &attempt, models.DraftMediaUsage{MediaID: "hero"})
		src := newOrphanSource(
			&fakeOrphanDrafts{publishingPageOne: []*models.Draft{edited}},
			&fakeOrphanMedia{byID: map[string]*models.Media{"hero": hero}},
			nil,
			zap.New(core),
		)
		ids, err := src.ListOrphanedPublishedMintIDs(context.Background())
		require.NoError(t, err)
		require.Equal(t, []string{"hero"}, ids,
			"a stale publish attempt must keep the draft a candidate no matter how recently it was edited")
	})

	t.Run("fresh publish attempt is not reconciled even with a stale UpdatedAt", func(t *testing.T) {
		core, _ := observer.New(zap.DebugLevel)
		// A fresh transition re-arms the horizon: the attempt stamp is new, so
		// the publish is in flight even if the row's UpdatedAt is old (the
		// author may have left the draft untouched before retrying).
		now := time.Now().UTC()
		attempt := now.Add(-time.Minute)
		retried := orphanPublishingDraftWithAttempt("dp-retried", now.Add(-25*time.Hour), &attempt, models.DraftMediaUsage{MediaID: "hero"})
		src := newOrphanSource(
			&fakeOrphanDrafts{publishingPageOne: []*models.Draft{retried}},
			&fakeOrphanMedia{byID: map[string]*models.Media{"hero": hero}},
			nil,
			zap.New(core),
		)
		ids, err := src.ListOrphanedPublishedMintIDs(context.Background())
		require.NoError(t, err)
		require.Empty(t, ids,
			"a fresh publish attempt must not be swept as crash-stuck")
	})

	t.Run("stale publishing candidate passes the re-check", func(t *testing.T) {
		core, _ := observer.New(zap.DebugLevel)
		stale := orphanPublishingDraft("dp-stale", time.Now().UTC().Add(-25*time.Hour), models.DraftMediaUsage{MediaID: "hero"})
		src := newOrphanSource(
			&fakeOrphanDrafts{publishingPageOne: []*models.Draft{stale}},
			&fakeOrphanMedia{byID: map[string]*models.Media{"hero": hero}},
			nil,
			zap.New(core),
		)
		ids, err := src.ListOrphanedPublishedMintIDs(context.Background())
		require.NoError(t, err)
		require.Equal(t, []string{"hero"}, ids)
		still, err := src.RecheckOrphanedPublishedMint(context.Background(), "hero")
		require.NoError(t, err)
		require.True(t, still, "the re-check must see the same stale-publishing candidate set")
	})

	t.Run("failed drafts are paged and mints are deduplicated", func(t *testing.T) {
		core, _ := observer.New(zap.DebugLevel)
		src := newOrphanSource(
			&fakeOrphanDrafts{
				pageOne: []*models.Draft{
					orphanFailedDraft("d1", nil, models.DraftMediaUsage{MediaID: "hero"}, models.DraftMediaUsage{MediaID: "card"}),
					orphanFailedDraft("d2", nil, models.DraftMediaUsage{MediaID: "hero"}),
				},
				pageTwo: []*models.Draft{
					orphanFailedDraft("d3", nil, models.DraftMediaUsage{MediaID: "inline-1"}),
				},
			},
			&fakeOrphanMedia{byID: map[string]*models.Media{
				"hero": hero, "card": orphanReconcilePublishedMedia("card"), "inline-1": orphanReconcilePublishedMedia("inline-1"),
			}},
			nil,
			zap.New(core),
		)
		ids, err := src.ListOrphanedPublishedMintIDs(context.Background())
		require.NoError(t, err)
		require.ElementsMatch(t, []string{"hero", "card", "inline-1"}, ids)
	})

	t.Run("unverifiable media is skipped fail-closed", func(t *testing.T) {
		core, observed := observer.New(zap.DebugLevel)
		src := newOrphanSource(
			&fakeOrphanDrafts{pageOne: []*models.Draft{orphanFailedDraft("d1", nil, models.DraftMediaUsage{MediaID: "hero"})}},
			&fakeOrphanMedia{err: errors.New("media lookup failed")},
			nil,
			zap.New(core),
		)
		ids, err := src.ListOrphanedPublishedMintIDs(context.Background())
		require.NoError(t, err)
		require.Empty(t, ids, "an unverifiable candidate must not be unpublished")
		require.NotEmpty(t, observed.FilterMessage("orphan reconciliation skipped unverifiable media").All())
	})

	t.Run("enumeration failure is surfaced", func(t *testing.T) {
		core, _ := observer.New(zap.DebugLevel)
		src := newOrphanSource(
			&fakeOrphanDrafts{err: errors.New("draft enumeration failed")},
			&fakeOrphanMedia{byID: map[string]*models.Media{}},
			nil,
			zap.New(core),
		)
		_, err := src.ListOrphanedPublishedMintIDs(context.Background())
		require.Error(t, err)
	})
}

// TestOrphanedPublishedMintRecheckClosesEnumerationTOCTOU proves the re-check
// run at unpublish time re-verifies the orphan premise against current state:
// a draft that transitions failed -> published (an author retry succeeded)
// between the enumeration and the unpublish aborts the candidate.
func TestOrphanedPublishedMintRecheckClosesEnumerationTOCTOU(t *testing.T) {
	hero := orphanReconcilePublishedMedia("hero")
	ctx := context.Background()

	t.Run("draft republished between enumeration and re-check aborts the candidate", func(t *testing.T) {
		core, _ := observer.New(zap.DebugLevel)
		drafts := &fakeOrphanDrafts{pageOne: []*models.Draft{orphanFailedDraft("d1", nil, models.DraftMediaUsage{MediaID: "hero"})}}
		src := newOrphanSource(
			drafts,
			&fakeOrphanMedia{byID: map[string]*models.Media{"hero": hero}},
			nil,
			zap.New(core),
		)

		ids, err := src.ListOrphanedPublishedMintIDs(ctx)
		require.NoError(t, err)
		require.Equal(t, []string{"hero"}, ids, "the failed draft's orphan mint is a candidate")

		// The author's retry succeeds between enumeration and unpublish. A
		// republished draft leaves the failed status index, which is what the
		// enumeration fake models by no longer yielding it.
		drafts.pageOne = nil
		still, err := src.RecheckOrphanedPublishedMint(ctx, "hero")
		require.NoError(t, err)
		require.False(t, still, "a republished draft's just-minted serving must not be unpublished")
	})

	t.Run("still-failed candidate passes the re-check", func(t *testing.T) {
		core, _ := observer.New(zap.DebugLevel)
		drafts := &fakeOrphanDrafts{pageOne: []*models.Draft{orphanFailedDraft("d1", nil, models.DraftMediaUsage{MediaID: "hero"})}}
		src := newOrphanSource(
			drafts,
			&fakeOrphanMedia{byID: map[string]*models.Media{"hero": hero}},
			nil,
			zap.New(core),
		)
		ids, err := src.ListOrphanedPublishedMintIDs(ctx)
		require.NoError(t, err)
		require.Equal(t, []string{"hero"}, ids)
		still, err := src.RecheckOrphanedPublishedMint(ctx, "hero")
		require.NoError(t, err)
		require.True(t, still)
	})

	t.Run("live article appearing between enumeration and re-check aborts the candidate", func(t *testing.T) {
		core, _ := observer.New(zap.DebugLevel)
		objectID := "https://example.test/articles/existing"
		article := &models.Article{Object: models.Object{ID: objectID}}
		article.FeaturedImage = &models.Media{MediaID: "hero", S3Key: "published/alice/hero.png"}
		articles := &fakeOrphanArticles{byID: map[string]*models.Article{objectID: article}}
		src := newOrphanSource(
			&fakeOrphanDrafts{pageOne: []*models.Draft{orphanFailedDraft("d1", nil, models.DraftMediaUsage{MediaID: "hero"})}},
			&fakeOrphanMedia{byID: map[string]*models.Media{"hero": hero}},
			articles,
			zap.New(core),
		)
		ids, err := src.ListOrphanedPublishedMintIDs(ctx)
		require.NoError(t, err)
		require.Equal(t, []string{"hero"}, ids, "no live article existed at enumeration time")

		// The article appears after enumeration: the re-check must abort.
		articles.byAuthor = map[string][]*models.Article{common.GenerateActorID("example.test", "alice"): {article}}
		still, err := src.RecheckOrphanedPublishedMint(ctx, "hero")
		require.NoError(t, err)
		require.False(t, still, "a live article referencing the asset must abort the unpublish")
	})

	t.Run("unpublished media is not an orphan at re-check time", func(t *testing.T) {
		core, _ := observer.New(zap.DebugLevel)
		cleared := *hero
		cleared.PublishedS3Key = ""
		cleared.PublishedURL = ""
		cleared.PublishedAt = nil
		src := newOrphanSource(
			&fakeOrphanDrafts{pageOne: []*models.Draft{orphanFailedDraft("d1", nil, models.DraftMediaUsage{MediaID: "hero"})}},
			&fakeOrphanMedia{byID: map[string]*models.Media{"hero": &cleared}},
			nil,
			zap.New(core),
		)
		still, err := src.RecheckOrphanedPublishedMint(ctx, "hero")
		require.NoError(t, err)
		require.False(t, still)
	})
}

// TestRegistryReconcileOrphanedPublishedMediaWiring proves the registry method
// fails closed when the media service cannot be built and, on a configured
// registry, wires the CMS-backed orphan source and runs an empty enumeration
// cleanly (no failed drafts, no orphans, no media writes).
func TestRegistryReconcileOrphanedPublishedMediaWiring(t *testing.T) {
	setTestAWSEnv(t)
	logger := zap.NewNop()

	regNil, err := NewRegistry(WithStorage(newMockStorage()), WithLogger(logger))
	require.NoError(t, err)
	require.ErrorContains(t, regNil.ReconcileOrphanedPublishedMedia(context.Background()), "media service is unavailable")

	storage := newPermissiveRegistryStorage(t, "example.com", logger)
	cfg := &ServiceConfig{
		BaseURL:   "https://example.com",
		JWTSecret: strings.Repeat("x", 32),
		Config: &pkgconfig.Config{
			Domain:                        "cms.example.com",
			InstanceMode:                  pkgconfig.InstanceModeHybrid,
			CMSLongFormPublishingEnabled:  true,
			CMSDraftSystemEnabled:         true,
			CMSScheduledPublishingEnabled: true,
			CMSMaxRevisionsPerObject:      5,
		},
	}
	reg, err := NewRegistry(WithStorage(storage), WithLogger(logger), WithConfig(cfg))
	require.NoError(t, err)
	require.NoError(t, reg.ReconcileOrphanedPublishedMedia(context.Background()))
}
