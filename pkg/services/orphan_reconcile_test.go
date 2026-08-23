package services

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	pkgconfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/common"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// fakeOrphanDrafts pages failed drafts; the first page returns pageOne, the
// second returns pageTwo, and any later page is empty.
type fakeOrphanDrafts struct {
	pageOne []*models.Draft
	pageTwo []*models.Draft
	err     error
}

func (f *fakeOrphanDrafts) ListDraftsByStatusPaginated(_ context.Context, _ string, _ int, cursor string) ([]*models.Draft, string, error) {
	if f.err != nil {
		return nil, "", f.err
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
	return &models.Draft{ID: id, AuthorID: "alice", Status: draftStatusFailed, ObjectID: objectID, EditorialMedia: usages}
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
