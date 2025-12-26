package cms

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type memDraftRepo struct {
	items     map[string]*models.Draft
	deleteErr error
}

func newMemDraftRepo() *memDraftRepo {
	return &memDraftRepo{
		items: map[string]*models.Draft{},
	}
}

func (r *memDraftRepo) key(authorID, draftID string) string {
	return authorID + "|" + draftID
}

func (r *memDraftRepo) CreateDraft(ctx context.Context, draft *models.Draft) error {
	if draft == nil {
		return apperrors.ValidationFailedWithField("draft")
	}
	if err := draft.UpdateKeys(); err != nil {
		return err
	}
	r.items[r.key(draft.AuthorID, draft.ID)] = cloneDraft(draft)
	return nil
}

func (r *memDraftRepo) UpdateDraft(ctx context.Context, draft *models.Draft) error {
	if draft == nil {
		return apperrors.ValidationFailedWithField("draft")
	}
	if err := draft.UpdateKeys(); err != nil {
		return err
	}
	r.items[r.key(draft.AuthorID, draft.ID)] = cloneDraft(draft)
	return nil
}

func (r *memDraftRepo) GetDraft(ctx context.Context, authorID, draftID string) (*models.Draft, error) {
	item, ok := r.items[r.key(authorID, draftID)]
	if !ok {
		return nil, apperrors.NotFound("draft")
	}
	return cloneDraft(item), nil
}

func (r *memDraftRepo) DeleteDraft(ctx context.Context, authorID, draftID string) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	delete(r.items, r.key(authorID, draftID))
	return nil
}

func cloneDraft(d *models.Draft) *models.Draft {
	if d == nil {
		return nil
	}
	cp := *d
	if d.ObjectID != nil {
		v := *d.ObjectID
		cp.ObjectID = &v
	}
	if d.ScheduledAt != nil {
		v := *d.ScheduledAt
		cp.ScheduledAt = &v
	}
	return &cp
}

type memArticleService struct {
	items map[string]*models.Article
}

func newMemArticleService() *memArticleService {
	return &memArticleService{
		items: map[string]*models.Article{},
	}
}

func (s *memArticleService) GetArticle(ctx context.Context, articleID string) (*models.Article, error) {
	article, ok := s.items[articleID]
	if !ok {
		return nil, apperrors.NotFound("article")
	}
	return cloneArticle(article), nil
}

func (s *memArticleService) CreateArticle(ctx context.Context, article *models.Article) error {
	if article == nil {
		return apperrors.ValidationFailedWithField("article")
	}
	if _, ok := s.items[article.ID]; ok {
		return apperrors.AlreadyExists("article")
	}
	s.items[article.ID] = cloneArticle(article)
	return nil
}

func (s *memArticleService) UpdateArticle(ctx context.Context, article *models.Article) error {
	if article == nil {
		return apperrors.ValidationFailedWithField("article")
	}
	s.items[article.ID] = cloneArticle(article)
	return nil
}

func cloneArticle(a *models.Article) *models.Article {
	if a == nil {
		return nil
	}
	cp := *a
	cp.CategoryIDs = append([]string{}, a.CategoryIDs...)
	cp.TableOfContents = append([]models.TOCEntry{}, a.TableOfContents...)
	if a.SeriesID != nil {
		v := *a.SeriesID
		cp.SeriesID = &v
	}
	if a.SeriesOrder != nil {
		v := *a.SeriesOrder
		cp.SeriesOrder = &v
	}
	if a.FeaturedImage != nil {
		v := *a.FeaturedImage
		cp.FeaturedImage = &v
	}
	return &cp
}

func TestDraftServiceScheduleAndCancelDraft(t *testing.T) {
	t.Parallel()

	repo := newMemDraftRepo()
	articles := newMemArticleService()
	svc := &DraftService{
		draftRepo:      repo,
		articleService: articles,
		domain:         "example.com",
		scheduling:     true,
		logger:         zap.NewNop(),
	}

	draft := &models.Draft{
		ID:              "draft-1",
		AuthorID:        "alice",
		ContentType:     activitypub.ArticleType,
		Title:           "Hello",
		Slug:            "hello",
		Content:         "hi",
		ContentFormat:   "markdown",
		Status:          "draft",
		AutosaveVersion: 0,
	}
	require.NoError(t, repo.CreateDraft(context.Background(), draft))

	scheduledAt := time.Now().Add(10 * time.Minute).UTC().Truncate(time.Second)
	require.NoError(t, svc.ScheduleDraft(context.Background(), draft.AuthorID, draft.ID, scheduledAt))

	afterSchedule, err := repo.GetDraft(context.Background(), draft.AuthorID, draft.ID)
	require.NoError(t, err)
	require.Equal(t, "scheduled", afterSchedule.Status)
	require.NotNil(t, afterSchedule.ScheduledAt)
	require.True(t, scheduledAt.Equal(*afterSchedule.ScheduledAt))

	require.NoError(t, svc.CancelScheduledDraft(context.Background(), draft.AuthorID, draft.ID))

	afterCancel, err := repo.GetDraft(context.Background(), draft.AuthorID, draft.ID)
	require.NoError(t, err)
	require.Equal(t, "draft", afterCancel.Status)
	require.Nil(t, afterCancel.ScheduledAt)
}

func TestDraftServiceScheduleDraftDisabled(t *testing.T) {
	t.Parallel()

	repo := newMemDraftRepo()
	articles := newMemArticleService()
	svc := &DraftService{
		draftRepo:      repo,
		articleService: articles,
		domain:         "example.com",
		scheduling:     false,
		logger:         zap.NewNop(),
	}

	err := svc.ScheduleDraft(context.Background(), "alice", "draft-1", time.Now())
	require.Error(t, err)
	require.Contains(t, err.Error(), "disabled")
}

func TestDraftServicePublishDraftCreatesArticleAndDeletesDraft(t *testing.T) {
	t.Parallel()

	repo := newMemDraftRepo()
	articles := newMemArticleService()
	svc := &DraftService{
		draftRepo:      repo,
		articleService: articles,
		domain:         "example.com",
		scheduling:     true,
		logger:         zap.NewNop(),
	}

	draft := &models.Draft{
		ID:            "draft-1",
		AuthorID:      "alice",
		ContentType:   activitypub.ArticleType,
		Title:         "Hello World",
		Slug:          "hello-world",
		Content:       "# Title\n\nHello world.",
		ContentFormat: "markdown",
		Status:        "draft",
	}
	require.NoError(t, repo.CreateDraft(context.Background(), draft))

	article, err := svc.PublishDraft(context.Background(), draft.AuthorID, draft.ID)
	require.NoError(t, err)
	require.NotNil(t, article)
	require.Equal(t, "https://example.com/articles/hello-world", article.ID)
	require.Equal(t, "https://example.com/users/alice", article.AttributedTo)
	require.Equal(t, activitypub.ArticleType, article.Type)

	_, err = repo.GetDraft(context.Background(), draft.AuthorID, draft.ID)
	require.Error(t, err)
	require.True(t, apperrors.HasCode(err, apperrors.CodeNotFound))
}

func TestDraftServicePublishDraftAlreadyExistsSameAuthor(t *testing.T) {
	t.Parallel()

	repo := newMemDraftRepo()
	articles := newMemArticleService()
	svc := &DraftService{
		draftRepo:      repo,
		articleService: articles,
		domain:         "example.com",
		scheduling:     true,
		logger:         zap.NewNop(),
	}

	existing := &models.Article{
		Object: models.Object{
			ID:           "https://example.com/articles/hello-world",
			Type:         activitypub.ArticleType,
			Name:         "Existing",
			AttributedTo: "https://example.com/users/alice",
		},
		ContentFormat: "markdown",
	}
	require.NoError(t, articles.CreateArticle(context.Background(), existing))

	draft := &models.Draft{
		ID:            "draft-1",
		AuthorID:      "alice",
		ContentType:   activitypub.ArticleType,
		Title:         "Hello World",
		Slug:          "hello-world",
		Content:       "new content",
		ContentFormat: "markdown",
		Status:        "draft",
	}
	require.NoError(t, repo.CreateDraft(context.Background(), draft))

	article, err := svc.PublishDraft(context.Background(), draft.AuthorID, draft.ID)
	require.NoError(t, err)
	require.NotNil(t, article)
	require.Equal(t, existing.ID, article.ID)

	_, err = repo.GetDraft(context.Background(), draft.AuthorID, draft.ID)
	require.Error(t, err)
	require.True(t, apperrors.HasCode(err, apperrors.CodeNotFound))
}

func TestDraftServicePublishDraftAlreadyExistsDifferentAuthorMarksFailed(t *testing.T) {
	t.Parallel()

	repo := newMemDraftRepo()
	articles := newMemArticleService()
	svc := &DraftService{
		draftRepo:      repo,
		articleService: articles,
		domain:         "example.com",
		scheduling:     true,
		logger:         zap.NewNop(),
	}

	existing := &models.Article{
		Object: models.Object{
			ID:           "https://example.com/articles/hello-world",
			Type:         activitypub.ArticleType,
			Name:         "Existing",
			AttributedTo: "https://example.com/users/bob",
		},
		ContentFormat: "markdown",
	}
	require.NoError(t, articles.CreateArticle(context.Background(), existing))

	draft := &models.Draft{
		ID:            "draft-1",
		AuthorID:      "alice",
		ContentType:   activitypub.ArticleType,
		Title:         "Hello World",
		Slug:          "hello-world",
		Content:       "new content",
		ContentFormat: "markdown",
		Status:        "draft",
	}
	require.NoError(t, repo.CreateDraft(context.Background(), draft))

	_, err := svc.PublishDraft(context.Background(), draft.AuthorID, draft.ID)
	require.Error(t, err)

	after, getErr := repo.GetDraft(context.Background(), draft.AuthorID, draft.ID)
	require.NoError(t, getErr)
	require.Equal(t, "failed", after.Status)
}

func TestCMSSmokeDraftLifecycle(t *testing.T) {
	repo := newMemDraftRepo()
	articles := newMemArticleService()
	svc := &DraftService{
		draftRepo:      repo,
		articleService: articles,
		domain:         "example.com",
		scheduling:     true,
		logger:         zap.NewNop(),
	}

	draft := &models.Draft{
		ID:            "draft-smoke",
		AuthorID:      "smoke",
		ContentType:   activitypub.ArticleType,
		Title:         "Smoke Test",
		Slug:          "smoke-test",
		Content:       "# Smoke\n\ncontent",
		ContentFormat: "markdown",
		Status:        "draft",
	}
	require.NoError(t, repo.CreateDraft(context.Background(), draft))

	require.NoError(t, svc.ScheduleDraft(context.Background(), draft.AuthorID, draft.ID, time.Now().Add(5*time.Minute)))
	require.NoError(t, svc.CancelScheduledDraft(context.Background(), draft.AuthorID, draft.ID))

	article, err := svc.PublishDraft(context.Background(), draft.AuthorID, draft.ID)
	require.NoError(t, err)
	require.Equal(t, "https://example.com/articles/smoke-test", article.ID)
}
