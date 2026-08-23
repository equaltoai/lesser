package cms

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/cmsrender"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/transformations"
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

func (r *memDraftRepo) UpdateDraft(ctx context.Context, authorID string, draft *models.Draft) error {
	if draft == nil || strings.TrimSpace(authorID) == "" {
		return apperrors.ValidationFailedWithField("draft")
	}
	if err := draft.UpdateKeys(); err != nil {
		return err
	}
	if strings.TrimSpace(draft.AuthorID) != strings.TrimSpace(authorID) {
		return apperrors.NotFound("draft")
	}
	r.items[r.key(authorID, draft.ID)] = cloneDraft(draft)
	return nil
}

func (r *memDraftRepo) UpdateDraftEditorialMedia(ctx context.Context, authorID string, draft *models.Draft) error {
	if draft == nil || strings.TrimSpace(authorID) == "" {
		return apperrors.ValidationFailedWithField("draft")
	}
	stored, ok := r.items[r.key(authorID, draft.ID)]
	if !ok || strings.TrimSpace(draft.AuthorID) != strings.TrimSpace(authorID) {
		return apperrors.NotFound("draft")
	}
	stored.EditorialMedia = append([]models.DraftMediaUsage(nil), draft.EditorialMedia...)
	stored.UpdatedAt = draft.UpdatedAt
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
	cp.EditorialMedia = append([]models.DraftMediaUsage(nil), d.EditorialMedia...)
	for i := range cp.EditorialMedia {
		if d.EditorialMedia[i].InlinePosition != nil {
			position := *d.EditorialMedia[i].InlinePosition
			cp.EditorialMedia[i].InlinePosition = &position
		}
	}
	return &cp
}

type memArticleService struct {
	items     map[string]*models.Article
	slugIndex map[string]string
}

func newMemArticleService() *memArticleService {
	return &memArticleService{
		items:     map[string]*models.Article{},
		slugIndex: map[string]string{},
	}
}

func (s *memArticleService) GetArticle(ctx context.Context, articleID string) (*models.Article, error) {
	article, ok := s.items[articleID]
	if !ok {
		return nil, apperrors.NotFound("article")
	}
	return cloneArticle(article), nil
}

func (s *memArticleService) GetArticleBySlug(ctx context.Context, slug string) (*models.Article, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return nil, apperrors.ValidationFailedWithField("slug")
	}

	articleID, ok := s.slugIndex[slug]
	if !ok {
		return nil, apperrors.ItemNotFoundWithID("article slug", slug)
	}

	return s.GetArticle(ctx, articleID)
}

func (s *memArticleService) CreateArticle(ctx context.Context, article *models.Article) error {
	if article == nil {
		return apperrors.ValidationFailedWithField("article")
	}

	slug := strings.TrimSpace(article.Slug)
	if slug == "" {
		return apperrors.ValidationFailedWithField("slug")
	}

	if existingID, ok := s.slugIndex[slug]; ok && !strings.EqualFold(existingID, article.ID) {
		return apperrors.ItemAlreadyExistsWithID("article slug", slug)
	}

	if _, ok := s.items[article.ID]; ok {
		return apperrors.AlreadyExists("article")
	}
	s.items[article.ID] = cloneArticle(article)
	s.slugIndex[slug] = article.ID
	return nil
}

func (s *memArticleService) UpdateArticle(ctx context.Context, article *models.Article) error {
	if article == nil {
		return apperrors.ValidationFailedWithField("article")
	}
	s.items[article.ID] = cloneArticle(article)
	if slug := strings.TrimSpace(article.Slug); slug != "" {
		if existingID, ok := s.slugIndex[slug]; ok && !strings.EqualFold(existingID, article.ID) {
			return apperrors.ItemAlreadyExistsWithID("article slug", slug)
		}
		s.slugIndex[slug] = article.ID
	}
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

func TestDraftServicePreviewUsesPublicationRenderer(t *testing.T) {
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
		ID:            "draft-preview",
		AuthorID:      "alice",
		ContentType:   activitypub.ArticleType,
		Title:         "Preview",
		Slug:          "preview",
		Content:       "# Preview\n\nBody<script>alert(1)</script>\n\n![alt](https://example.com/media/cover.png)",
		ContentFormat: "markdown",
		Status:        "draft",
	}
	require.NoError(t, svc.CreateDraft(context.Background(), draft))
	draft.Title = "Preview updated"
	require.NoError(t, svc.UpdateDraft(context.Background(), "alice", draft),
		"create and update must apply the same source-storage policy")
	require.Contains(t, draft.Content, "<script>", "canonical source remains Markdown; rendering owns sanitization")

	preview, err := svc.PreviewDraft(context.Background(), "alice", "draft-preview")
	require.NoError(t, err)
	require.Contains(t, preview.HTML, `<h1 id="preview">Preview</h1>`)
	require.Contains(t, preview.HTML, "<p>Body</p>")
	require.NotContains(t, preview.HTML, "<script")

	article, err := svc.PublishDraft(context.Background(), "alice", "draft-preview")
	require.NoError(t, err)
	apArticle, err := transformations.StorageArticleToActivityPub(article)
	require.NoError(t, err)
	require.Equal(t, preview.HTML, apArticle.Content)
}

func TestDraftServiceRejectsOversizedArticleDrafts(t *testing.T) {
	t.Parallel()

	repo := newMemDraftRepo()
	svc := &DraftService{
		draftRepo: repo,
		domain:    "example.com",
		logger:    zap.NewNop(),
	}

	err := svc.CreateDraft(context.Background(), &models.Draft{
		ID:            "draft-big",
		AuthorID:      "alice",
		ContentType:   activitypub.ArticleType,
		Title:         "Big",
		Slug:          "big",
		Content:       strings.Repeat("a", cmsrender.MaxArticleSourceBytes+1),
		ContentFormat: "markdown",
		Status:        "draft",
	})
	require.ErrorIs(t, err, cmsrender.ErrArticleContentTooLarge)
}

func TestRenderDraftPreviewRequiresArticleDraft(t *testing.T) {
	t.Parallel()

	rendered, err := RenderDraftPreview(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "draft is required")
	require.Empty(t, rendered.HTML)

	rendered, err = RenderDraftPreview(&models.Draft{
		ID:          "draft-note",
		AuthorID:    "alice",
		ContentType: activitypub.NoteType,
		Content:     "note body",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "only article drafts can be previewed")
	require.Empty(t, rendered.HTML)
}

func TestDraftServicePreviewPropagatesRepositoryAndRendererErrors(t *testing.T) {
	t.Parallel()

	repo := newMemDraftRepo()
	svc := &DraftService{
		draftRepo: repo,
		domain:    "example.com",
		logger:    zap.NewNop(),
	}

	preview, err := svc.PreviewDraft(context.Background(), "alice", "missing")
	require.Error(t, err)
	require.Empty(t, preview.HTML)

	require.NoError(t, repo.CreateDraft(context.Background(), &models.Draft{
		ID:            "draft-unsupported",
		AuthorID:      "alice",
		ContentType:   activitypub.ArticleType,
		Content:       "body",
		ContentFormat: "asciidoc",
		Status:        "draft",
	}))

	preview, err = svc.PreviewDraft(context.Background(), "alice", "draft-unsupported")
	require.ErrorIs(t, err, cmsrender.ErrUnsupportedContentFormat)
	require.Empty(t, preview.HTML)
}

func TestDraftServiceAutosaveRejectsOversizedRenderedArticle(t *testing.T) {
	t.Parallel()

	repo := newMemDraftRepo()
	svc := &DraftService{
		draftRepo: repo,
		domain:    "example.com",
		logger:    zap.NewNop(),
	}

	err := svc.Autosave(context.Background(), "alice", &models.Draft{
		ID:            "draft-render-big",
		AuthorID:      "alice",
		ContentType:   activitypub.ArticleType,
		Title:         "Rendered Big",
		Slug:          "rendered-big",
		Content:       strings.Repeat("&", cmsrender.MaxArticleSourceBytes/2),
		ContentFormat: "markdown",
		Status:        "draft",
	})
	require.ErrorIs(t, err, cmsrender.ErrArticleRenderedContentTooLarge)
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
	require.Equal(t, "hello-world", article.Slug)
	require.Equal(t, "https://example.com/users/alice", article.AttributedTo)
	require.Equal(t, activitypub.ArticleType, article.Type)

	_, err = repo.GetDraft(context.Background(), draft.AuthorID, draft.ID)
	require.Error(t, err)
	require.True(t, apperrors.HasCode(err, apperrors.CodeNotFound))
}

func TestDraftServicePublishDraftAlreadyExistsSameAuthorMarksFailed(t *testing.T) {
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
			ID:           "https://example.com/objects/existing",
			Type:         activitypub.ArticleType,
			Name:         "Existing",
			AttributedTo: "https://example.com/users/alice",
		},
		Slug:          "hello-world",
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
	require.Error(t, err)
	require.Nil(t, article)
	require.True(t, apperrors.HasCode(err, apperrors.CodeAlreadyExists))

	after, getErr := repo.GetDraft(context.Background(), draft.AuthorID, draft.ID)
	require.NoError(t, getErr)
	require.Equal(t, "failed", after.Status)

	stored, getExistingErr := articles.GetArticle(context.Background(), existing.ID)
	require.NoError(t, getExistingErr)
	require.Equal(t, "Existing", stored.Name)
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
			ID:           "https://example.com/objects/existing",
			Type:         activitypub.ArticleType,
			Name:         "Existing",
			AttributedTo: "https://example.com/users/bob",
		},
		Slug:          "hello-world",
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
	require.Equal(t, "smoke-test", article.Slug)
}
