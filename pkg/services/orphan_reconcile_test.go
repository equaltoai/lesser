package services

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	pkgconfig "github.com/equaltoai/lesser/pkg/config"
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
	if cursor == "" {
		return f.pageOne, "page-1", nil
	}
	if cursor == "page-1" {
		return f.pageTwo, "", nil
	}
	return nil, "", nil
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
	byID map[string]*models.Article
	err  error
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

	t.Run("create-path orphan mint is returned", func(t *testing.T) {
		core, _ := observer.New(zap.DebugLevel)
		src := &cmsOrphanedPublishedMintSource{
			drafts:   &fakeOrphanDrafts{pageOne: []*models.Draft{orphanFailedDraft("d1", nil, models.DraftMediaUsage{MediaID: "hero"})}},
			media:    &fakeOrphanMedia{byID: map[string]*models.Media{"hero": hero}},
			articles: &fakeOrphanArticles{byID: map[string]*models.Article{}},
			logger:   zap.New(core),
		}
		ids, err := src.ListOrphanedPublishedMintIDs(context.Background())
		require.NoError(t, err)
		require.Equal(t, []string{"hero"}, ids)
	})

	t.Run("live article referencing by featured image is untouched", func(t *testing.T) {
		core, _ := observer.New(zap.DebugLevel)
		article := &models.Article{Object: models.Object{ID: objectID}}
		article.FeaturedImage = &models.Media{MediaID: "hero", S3Key: "published/alice/hero.png"}
		src := &cmsOrphanedPublishedMintSource{
			drafts:   &fakeOrphanDrafts{pageOne: []*models.Draft{orphanFailedDraft("d1", &objectID, models.DraftMediaUsage{MediaID: "hero"})}},
			media:    &fakeOrphanMedia{byID: map[string]*models.Media{"hero": hero}},
			articles: &fakeOrphanArticles{byID: map[string]*models.Article{objectID: article}},
			logger:   zap.New(core),
		}
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
		src := &cmsOrphanedPublishedMintSource{
			drafts:   &fakeOrphanDrafts{pageOne: []*models.Draft{orphanFailedDraft("d1", &objectID, models.DraftMediaUsage{MediaID: "hero"})}},
			media:    &fakeOrphanMedia{byID: map[string]*models.Media{"hero": hero}},
			articles: &fakeOrphanArticles{byID: map[string]*models.Article{objectID: article}},
			logger:   zap.New(core),
		}
		ids, err := src.ListOrphanedPublishedMintIDs(context.Background())
		require.NoError(t, err)
		require.Empty(t, ids)
	})

	t.Run("update-path orphan without a surviving reference is returned", func(t *testing.T) {
		core, _ := observer.New(zap.DebugLevel)
		src := &cmsOrphanedPublishedMintSource{
			drafts:   &fakeOrphanDrafts{pageOne: []*models.Draft{orphanFailedDraft("d1", &objectID, models.DraftMediaUsage{MediaID: "hero"})}},
			media:    &fakeOrphanMedia{byID: map[string]*models.Media{"hero": hero}},
			articles: &fakeOrphanArticles{byID: map[string]*models.Article{}},
			logger:   zap.New(core),
		}
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
		src := &cmsOrphanedPublishedMintSource{
			drafts:   &fakeOrphanDrafts{pageOne: []*models.Draft{orphanFailedDraft("d1", nil, models.DraftMediaUsage{MediaID: "hero"})}},
			media:    &fakeOrphanMedia{byID: map[string]*models.Media{"hero": &unpublished}},
			articles: &fakeOrphanArticles{byID: map[string]*models.Article{}},
			logger:   zap.New(core),
		}
		ids, err := src.ListOrphanedPublishedMintIDs(context.Background())
		require.NoError(t, err)
		require.Empty(t, ids)
	})

	t.Run("failed drafts are paged and mints are deduplicated", func(t *testing.T) {
		core, _ := observer.New(zap.DebugLevel)
		src := &cmsOrphanedPublishedMintSource{
			drafts: &fakeOrphanDrafts{
				pageOne: []*models.Draft{
					orphanFailedDraft("d1", nil, models.DraftMediaUsage{MediaID: "hero"}, models.DraftMediaUsage{MediaID: "card"}),
					orphanFailedDraft("d2", nil, models.DraftMediaUsage{MediaID: "hero"}),
				},
				pageTwo: []*models.Draft{
					orphanFailedDraft("d3", nil, models.DraftMediaUsage{MediaID: "inline-1"}),
				},
			},
			media: &fakeOrphanMedia{byID: map[string]*models.Media{
				"hero": hero, "card": orphanReconcilePublishedMedia("card"), "inline-1": orphanReconcilePublishedMedia("inline-1"),
			}},
			articles: &fakeOrphanArticles{byID: map[string]*models.Article{}},
			logger:   zap.New(core),
		}
		ids, err := src.ListOrphanedPublishedMintIDs(context.Background())
		require.NoError(t, err)
		require.ElementsMatch(t, []string{"hero", "card", "inline-1"}, ids)
	})

	t.Run("unverifiable media is skipped fail-closed", func(t *testing.T) {
		core, observed := observer.New(zap.DebugLevel)
		src := &cmsOrphanedPublishedMintSource{
			drafts:   &fakeOrphanDrafts{pageOne: []*models.Draft{orphanFailedDraft("d1", nil, models.DraftMediaUsage{MediaID: "hero"})}},
			media:    &fakeOrphanMedia{err: errors.New("media lookup failed")},
			articles: &fakeOrphanArticles{byID: map[string]*models.Article{}},
			logger:   zap.New(core),
		}
		ids, err := src.ListOrphanedPublishedMintIDs(context.Background())
		require.NoError(t, err)
		require.Empty(t, ids, "an unverifiable candidate must not be unpublished")
		require.NotEmpty(t, observed.FilterMessage("orphan reconciliation skipped unverifiable media").All())
	})

	t.Run("enumeration failure is surfaced", func(t *testing.T) {
		core, _ := observer.New(zap.DebugLevel)
		src := &cmsOrphanedPublishedMintSource{
			drafts:   &fakeOrphanDrafts{err: errors.New("draft enumeration failed")},
			media:    &fakeOrphanMedia{byID: map[string]*models.Media{}},
			articles: &fakeOrphanArticles{byID: map[string]*models.Article{}},
			logger:   zap.New(core),
		}
		_, err := src.ListOrphanedPublishedMintIDs(context.Background())
		require.Error(t, err)
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
