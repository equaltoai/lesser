package cms

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type memArticleServiceWithErrors struct {
	base      *memArticleService
	getErr    error
	updateErr error
	createErr error
}

func (s *memArticleServiceWithErrors) GetArticle(ctx context.Context, articleID string) (*models.Article, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.base.GetArticle(ctx, articleID)
}

func (s *memArticleServiceWithErrors) GetArticleBySlug(ctx context.Context, slug string) (*models.Article, error) {
	return s.base.GetArticleBySlug(ctx, slug)
}

func (s *memArticleServiceWithErrors) CreateArticle(ctx context.Context, article *models.Article) error {
	if s.createErr != nil {
		return s.createErr
	}
	return s.base.CreateArticle(ctx, article)
}

func (s *memArticleServiceWithErrors) UpdateArticle(ctx context.Context, article *models.Article) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	return s.base.UpdateArticle(ctx, article)
}

func TestDraftServicePublishDraft_RequiresDomain(t *testing.T) {
	t.Parallel()

	svc := &DraftService{
		domain: "",
		logger: zap.NewNop(),
	}

	_, err := svc.PublishDraft(context.Background(), "alice", "draft-1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "domain")
}

func TestDraftServicePublishDraft_RejectsNonArticleDrafts(t *testing.T) {
	t.Parallel()

	repo := newMemDraftRepo()
	articles := newMemArticleService()
	svc := &DraftService{
		draftRepo:      repo,
		articleService: articles,
		domain:         "example.com",
		logger:         zap.NewNop(),
	}

	draft := &models.Draft{
		ID:          "draft-1",
		AuthorID:    "alice",
		ContentType: "Note",
		Title:       "Hello",
		Slug:        "hello",
		Content:     "hi",
		Status:      "draft",
	}
	require.NoError(t, repo.CreateDraft(context.Background(), draft))

	_, err := svc.PublishDraft(context.Background(), draft.AuthorID, draft.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "only article drafts")

	after, getErr := repo.GetDraft(context.Background(), draft.AuthorID, draft.ID)
	require.NoError(t, getErr)
	require.Equal(t, "draft", after.Status)
}

func TestDraftServicePublishDraft_PublishedCleanupPermissionDenied(t *testing.T) {
	t.Parallel()

	repo := newMemDraftRepo()
	articles := newMemArticleService()
	svc := &DraftService{
		draftRepo:      repo,
		articleService: articles,
		domain:         "example.com",
		logger:         zap.NewNop(),
	}

	objectID := "https://example.com/objects/existing"
	draft := &models.Draft{
		ID:          "draft-1",
		AuthorID:    "alice",
		ContentType: activitypub.ArticleType,
		Title:       "Hello",
		Slug:        "hello",
		Content:     "hi",
		Status:      "published",
		ObjectID:    &objectID,
	}
	require.NoError(t, repo.CreateDraft(context.Background(), draft))

	articles.items[objectID] = &models.Article{
		Object: models.Object{
			ID:           objectID,
			Type:         activitypub.ArticleType,
			AttributedTo: "https://example.com/users/bob",
		},
	}

	_, err := svc.PublishDraft(context.Background(), draft.AuthorID, draft.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "permission")
}

func TestDraftServicePublishDraft_PublishedCleanupDeleteFailureIsBestEffort(t *testing.T) {
	t.Parallel()

	repo := newMemDraftRepo()
	repo.deleteErr = errors.New("delete failed")
	articles := newMemArticleService()
	svc := &DraftService{
		draftRepo:      repo,
		articleService: articles,
		domain:         "example.com",
		logger:         zap.NewNop(),
	}

	objectID := "https://example.com/objects/existing"
	draft := &models.Draft{
		ID:          "draft-1",
		AuthorID:    "alice",
		ContentType: activitypub.ArticleType,
		Title:       "Hello",
		Slug:        "hello",
		Content:     "hi",
		Status:      "published",
		ObjectID:    &objectID,
	}
	require.NoError(t, repo.CreateDraft(context.Background(), draft))

	articles.items[objectID] = &models.Article{
		Object: models.Object{
			ID:           objectID,
			Type:         activitypub.ArticleType,
			AttributedTo: "https://example.com/users/alice",
		},
		Slug: "hello",
	}

	article, err := svc.PublishDraft(context.Background(), draft.AuthorID, draft.ID)
	require.NoError(t, err)
	require.NotNil(t, article)

	_, getErr := repo.GetDraft(context.Background(), draft.AuthorID, draft.ID)
	require.NoError(t, getErr)
}

func TestDraftServiceResolveArticleDraftTarget_ValidatesSlugAndObjectID(t *testing.T) {
	t.Parallel()

	svc := &DraftService{}

	_, _, err := svc.resolveArticleDraftTarget("example.com", &models.Draft{})
	require.Error(t, err)

	nonLocal := "https://remote.example/articles/hello"
	_, _, err = svc.resolveArticleDraftTarget("example.com", &models.Draft{Title: "Hello", ObjectID: &nonLocal})
	require.Error(t, err)
	require.Contains(t, err.Error(), "local id")

	wrongPath := "https://example.com/not-articles/hello"
	_, _, err = svc.resolveArticleDraftTarget("example.com", &models.Draft{Title: "Hello", ObjectID: &wrongPath})
	require.Error(t, err)
	require.Contains(t, err.Error(), "local article id")

	validObjectID := "https://example.com/articles/hello"
	objectID, slug, err := svc.resolveArticleDraftTarget("example.com", &models.Draft{Title: "Hello", ObjectID: &validObjectID})
	require.NoError(t, err)
	require.Equal(t, validObjectID, objectID)
	require.Equal(t, "hello", slug)

	objectID, slug, err = svc.resolveArticleDraftTarget("example.com", &models.Draft{Title: "Hello World"})
	require.NoError(t, err)
	require.Equal(t, "https://example.com/articles/hello-world", objectID)
	require.Equal(t, "hello-world", slug)

	validLegacyObjectID := "https://example.com/objects/legacy"
	objectID, slug, err = svc.resolveArticleDraftTarget("example.com", &models.Draft{Title: "Legacy", ObjectID: &validLegacyObjectID})
	require.NoError(t, err)
	require.Equal(t, validLegacyObjectID, objectID)
	require.Equal(t, "legacy", slug)
}

func TestDraftServicePublishDraft_UpdateExistingArticlePermissionDeniedMarksFailed(t *testing.T) {
	t.Parallel()

	repo := newMemDraftRepo()
	articles := newMemArticleService()
	svc := &DraftService{
		draftRepo:      repo,
		articleService: articles,
		domain:         "example.com",
		logger:         zap.NewNop(),
	}

	objectID := "https://example.com/objects/existing"
	articles.items[objectID] = &models.Article{
		Object: models.Object{
			ID:           objectID,
			Type:         activitypub.ArticleType,
			AttributedTo: "https://example.com/users/bob",
		},
	}

	draft := &models.Draft{
		ID:            "draft-1",
		AuthorID:      "alice",
		ContentType:   activitypub.ArticleType,
		Title:         "Hello World",
		Slug:          "hello-world",
		Content:       "content",
		ContentFormat: "markdown",
		Status:        "draft",
		ObjectID:      &objectID,
	}
	require.NoError(t, repo.CreateDraft(context.Background(), draft))

	_, err := svc.PublishDraft(context.Background(), draft.AuthorID, draft.ID)
	require.Error(t, err)

	after, getErr := repo.GetDraft(context.Background(), draft.AuthorID, draft.ID)
	require.NoError(t, getErr)
	require.Equal(t, "failed", after.Status)
}

func TestDraftServicePublishDraft_UpdateExistingArticleUpdateErrorMarksFailed(t *testing.T) {
	t.Parallel()

	repo := newMemDraftRepo()
	baseArticles := newMemArticleService()
	articles := &memArticleServiceWithErrors{
		base:      baseArticles,
		updateErr: errors.New("update failed"),
	}

	svc := &DraftService{
		draftRepo:      repo,
		articleService: articles,
		domain:         "example.com",
		logger:         zap.NewNop(),
	}

	objectID := "https://example.com/objects/existing"
	baseArticles.items[objectID] = &models.Article{
		Object: models.Object{
			ID:           objectID,
			Type:         activitypub.ArticleType,
			AttributedTo: "https://example.com/users/alice",
		},
	}

	draft := &models.Draft{
		ID:            "draft-1",
		AuthorID:      "alice",
		ContentType:   activitypub.ArticleType,
		Title:         "Hello World",
		Slug:          "hello-world",
		Content:       "content",
		ContentFormat: "markdown",
		Status:        "draft",
		ObjectID:      &objectID,
	}
	require.NoError(t, repo.CreateDraft(context.Background(), draft))

	_, err := svc.PublishDraft(context.Background(), draft.AuthorID, draft.ID)
	require.Error(t, err)

	after, getErr := repo.GetDraft(context.Background(), draft.AuthorID, draft.ID)
	require.NoError(t, getErr)
	require.Equal(t, "failed", after.Status)
}

func TestDraftServicePublishDraftCreateNewArticle_DeleteDraftFailurePublishesDraftStatus(t *testing.T) {
	t.Parallel()

	repo := newMemDraftRepo()
	repo.deleteErr = errors.New("delete failed")
	articles := newMemArticleService()
	svc := &DraftService{
		draftRepo:      repo,
		articleService: articles,
		logger:         zap.NewNop(),
	}

	draft := &models.Draft{
		ID:            "draft-1",
		AuthorID:      "alice",
		ContentType:   activitypub.ArticleType,
		Title:         "Hello World",
		Slug:          "hello-world",
		Content:       "content",
		ContentFormat: "markdown",
		Status:        "publishing",
	}
	require.NoError(t, repo.CreateDraft(context.Background(), draft))

	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	objectID := "https://example.com/objects/new"
	article, err := svc.publishDraftCreateNewArticle(context.Background(), draft.AuthorID, draft.ID, "example.com", objectID, "hello-world", draft, now, "", nil)
	require.NoError(t, err)
	require.NotNil(t, article)
	require.Equal(t, objectID, article.ID)

	after, getErr := repo.GetDraft(context.Background(), draft.AuthorID, draft.ID)
	require.NoError(t, getErr)
	require.Equal(t, "published", after.Status)
	require.NotNil(t, after.ObjectID)
	require.Equal(t, objectID, *after.ObjectID)
}
