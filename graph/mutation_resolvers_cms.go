package graph

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/google/uuid"
)

func (r *mutationResolver) CreateDraft(ctx context.Context, input model.CreateDraftInput) (*model.Draft, error) {
	if err := r.requireCMSDraftsEnabled(); err != nil {
		return nil, err
	}

	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	draftSvc := r.Registry.Drafts()
	if draftSvc == nil {
		return nil, errors.New("draft service is not available")
	}

	draftID := uuid.NewString()

	title := ""
	if input.Title != nil {
		title = strings.TrimSpace(*input.Title)
	}

	slug := ""
	if input.Slug != nil {
		slug = cmsSlugify(*input.Slug)
	} else if title != "" {
		slug = cmsSlugify(title)
	}

	draft := &models.Draft{
		ID:            draftID,
		AuthorID:      username,
		ObjectID:      input.ObjectID,
		ContentType:   cmsObjectTypeToStorage(input.ContentType),
		Title:         title,
		Slug:          slug,
		Content:       input.Content,
		ContentFormat: cmsContentFormatToStorage(input.ContentFormat),
		Status:        cmsDraftStatusToStorage(model.DraftStatusDraft),
	}

	if err := draftSvc.CreateDraft(ctx, draft); err != nil {
		return nil, err
	}

	return r.convertCMSDraft(ctx, draft), nil
}

func (r *mutationResolver) UpdateDraft(ctx context.Context, id string, input model.UpdateDraftInput) (*model.Draft, error) {
	if err := r.requireCMSDraftsEnabled(); err != nil {
		return nil, err
	}

	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	draftSvc := r.Registry.Drafts()
	if draftSvc == nil {
		return nil, errors.New("draft service is not available")
	}

	draft, err := draftSvc.GetDraft(ctx, username, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}

	if input.Title != nil {
		draft.Title = strings.TrimSpace(*input.Title)
	}
	if input.Slug != nil {
		draft.Slug = cmsSlugify(*input.Slug)
	}
	if input.Content != nil {
		draft.Content = *input.Content
	}
	if input.ContentFormat != nil {
		draft.ContentFormat = cmsContentFormatToStorage(*input.ContentFormat)
	}

	if err := draftSvc.UpdateDraft(ctx, draft); err != nil {
		return nil, err
	}

	return r.convertCMSDraft(ctx, draft), nil
}

func (r *mutationResolver) AutosaveDraft(ctx context.Context, id string, content string) (*model.Draft, error) {
	if err := r.requireCMSDraftsEnabled(); err != nil {
		return nil, err
	}

	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	draftSvc := r.Registry.Drafts()
	if draftSvc == nil {
		return nil, errors.New("draft service is not available")
	}

	draft, err := draftSvc.GetDraft(ctx, username, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}

	draft.Content = content
	if err := draftSvc.Autosave(ctx, draft); err != nil {
		return nil, err
	}

	return r.convertCMSDraft(ctx, draft), nil
}

func (r *mutationResolver) DeleteDraft(ctx context.Context, id string) (bool, error) {
	if err := r.requireCMSDraftsEnabled(); err != nil {
		return false, err
	}

	username, err := r.requireAuth(ctx)
	if err != nil {
		return false, err
	}

	draftSvc := r.Registry.Drafts()
	if draftSvc == nil {
		return false, errors.New("draft service is not available")
	}

	if err := draftSvc.DeleteDraft(ctx, username, strings.TrimSpace(id)); err != nil {
		return false, err
	}

	return true, nil
}

func (r *mutationResolver) PublishDraft(ctx context.Context, id string) (*model.Article, error) {
	if err := r.requireCMSDraftsEnabled(); err != nil {
		return nil, err
	}

	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	draftSvc := r.Registry.Drafts()
	if draftSvc == nil {
		return nil, errors.New("draft service is not available")
	}

	article, err := draftSvc.PublishDraft(ctx, username, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}

	return r.convertCMSArticle(ctx, article, true), nil
}

func (r *mutationResolver) ScheduleDraft(ctx context.Context, id string, scheduledAt model.Time) (*model.Draft, error) {
	if err := r.requireCMSSchedulingEnabled(); err != nil {
		return nil, err
	}

	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	draftSvc := r.Registry.Drafts()
	if draftSvc == nil {
		return nil, errors.New("draft service is not available")
	}

	when := time.Time(scheduledAt)
	if err := draftSvc.ScheduleDraft(ctx, username, strings.TrimSpace(id), when); err != nil {
		return nil, err
	}

	draft, err := draftSvc.GetDraft(ctx, username, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}

	return r.convertCMSDraft(ctx, draft), nil
}

func (r *mutationResolver) CancelScheduledDraft(ctx context.Context, id string) (*model.Draft, error) {
	if err := r.requireCMSDraftsEnabled(); err != nil {
		return nil, err
	}

	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	draftSvc := r.Registry.Drafts()
	if draftSvc == nil {
		return nil, errors.New("draft service is not available")
	}

	if err := draftSvc.CancelScheduledDraft(ctx, username, strings.TrimSpace(id)); err != nil {
		return nil, err
	}

	draft, err := draftSvc.GetDraft(ctx, username, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}

	return r.convertCMSDraft(ctx, draft), nil
}

func (r *mutationResolver) CreateArticle(ctx context.Context, input model.CreateArticleInput) (*model.Article, error) {
	if err := r.requireCMSLongFormEnabled(); err != nil {
		return nil, err
	}
	if input.SeriesID != nil && strings.TrimSpace(*input.SeriesID) != "" && !r.cmsSeriesEnabled() {
		return nil, errCMSSeriesDisabled
	}
	if len(input.CategoryIDs) > 0 && !r.cmsCategoriesEnabled() {
		return nil, errCMSCategoriesDisabled
	}

	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	articleSvc := r.Registry.Articles()
	if articleSvc == nil {
		return nil, errors.New("article service is not available")
	}

	store := r.cmsStorage()
	if store == nil || store.Media() == nil {
		return nil, ErrStorageUnavailable
	}

	domain := r.getDomain()
	slug := cmsSlugify(input.Slug)
	if strings.TrimSpace(slug) == "" {
		return nil, errors.New("slug is required")
	}

	now := time.Now()

	article := &models.Article{
		Object: models.Object{
			ID:           cmsArticleID(domain, slug),
			Type:         "Article",
			Name:         input.Title,
			Content:      input.Content,
			AttributedTo: cmsLocalActorID(domain, username),
			Published:    now,
			Updated:      now,
			CreatedAt:    now,
		},
		Subtitle:           derefString(input.Subtitle),
		Excerpt:            derefString(input.Excerpt),
		ContentFormat:      cmsContentFormatToStorage(input.ContentFormat),
		SeriesID:           input.SeriesID,
		SeriesOrder:        input.SeriesOrder,
		CategoryIDs:        input.CategoryIDs,
		SEOTitle:           derefString(input.SEOTitle),
		SEODescription:     derefString(input.SEODescription),
		CanonicalURL:       derefString(input.CanonicalURL),
		OGImage:            derefString(input.OGImage),
		EditorNotes:        derefString(input.EditorNotes),
		ReviewStatus:       derefString(input.ReviewStatus),
		ReadingTimeMinutes: 0,
		WordCount:          0,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if input.FeaturedImageID != nil && strings.TrimSpace(*input.FeaturedImageID) != "" {
		media, err := store.Media().GetMedia(ctx, strings.TrimSpace(*input.FeaturedImageID))
		if err != nil {
			return nil, err
		}
		article.FeaturedImage = media
	}

	if err := articleSvc.CreateArticle(ctx, article); err != nil {
		return nil, err
	}

	return r.convertCMSArticle(ctx, article, true), nil
}

func (r *mutationResolver) UpdateArticle(ctx context.Context, id string, input model.UpdateArticleInput) (*model.Article, error) {
	if err := r.requireCMSLongFormEnabled(); err != nil {
		return nil, err
	}
	if input.SeriesID != nil && strings.TrimSpace(*input.SeriesID) != "" && !r.cmsSeriesEnabled() {
		return nil, errCMSSeriesDisabled
	}
	if len(input.CategoryIDs) > 0 && !r.cmsCategoriesEnabled() {
		return nil, errCMSCategoriesDisabled
	}

	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	articleSvc := r.Registry.Articles()
	if articleSvc == nil {
		return nil, errors.New("article service is not available")
	}

	store := r.cmsStorage()
	if store == nil || store.Article() == nil {
		return nil, ErrStorageUnavailable
	}

	article, err := store.Article().GetArticle(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}

	if err := r.ensureAuthorCanWriteCMS(ctx, username, article.AttributedTo); err != nil {
		return nil, err
	}

	cmsSetStringField(&article.Name, input.Title)
	cmsSetStringField(&article.Subtitle, input.Subtitle)
	cmsSetStringField(&article.Excerpt, input.Excerpt)
	cmsSetStringField(&article.Content, input.Content)
	if input.ContentFormat != nil {
		article.ContentFormat = cmsContentFormatToStorage(*input.ContentFormat)
	}
	cmsSetStringPtrField(&article.SeriesID, input.SeriesID)
	cmsSetIntPtrField(&article.SeriesOrder, input.SeriesOrder)
	if input.CategoryIDs != nil {
		article.CategoryIDs = input.CategoryIDs
	}
	cmsSetStringField(&article.SEOTitle, input.SEOTitle)
	cmsSetStringField(&article.SEODescription, input.SEODescription)
	cmsSetStringField(&article.CanonicalURL, input.CanonicalURL)
	cmsSetStringField(&article.OGImage, input.OGImage)
	cmsSetStringField(&article.EditorNotes, input.EditorNotes)
	cmsSetStringField(&article.ReviewStatus, input.ReviewStatus)

	if err := cmsApplyArticleFeaturedImage(ctx, store.Media(), article, input.FeaturedImageID); err != nil {
		return nil, err
	}

	now := time.Now()
	article.Updated = now
	article.UpdatedAt = now

	if err := articleSvc.UpdateArticle(ctx, article); err != nil {
		return nil, err
	}

	return r.convertCMSArticle(ctx, article, true), nil
}

func (r *mutationResolver) DeleteArticle(ctx context.Context, id string) (bool, error) {
	if err := r.requireCMSLongFormEnabled(); err != nil {
		return false, err
	}

	username, err := r.requireAuth(ctx)
	if err != nil {
		return false, err
	}

	articleSvc := r.Registry.Articles()
	if articleSvc == nil {
		return false, errors.New("article service is not available")
	}

	store := r.cmsStorage()
	if store == nil || store.Article() == nil {
		return false, ErrStorageUnavailable
	}

	article, err := store.Article().GetArticle(ctx, strings.TrimSpace(id))
	if err != nil {
		return false, err
	}

	if err := r.ensureAuthorCanWriteCMS(ctx, username, article.AttributedTo); err != nil {
		return false, err
	}

	if err := articleSvc.DeleteArticle(ctx, article); err != nil {
		return false, err
	}

	return true, nil
}

func (r *mutationResolver) RestoreRevision(ctx context.Context, objectID string, version int) (*model.Article, error) {
	if err := r.requireCMSRevisionsEnabled(); err != nil {
		return nil, err
	}

	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	if r.Registry == nil || r.Registry.Revisions() == nil {
		return nil, errors.New("revision service is not available")
	}

	store := r.cmsStorage()
	if store == nil || store.Article() == nil {
		return nil, ErrStorageUnavailable
	}

	article, err := store.Article().GetArticle(ctx, strings.TrimSpace(objectID))
	if err != nil {
		return nil, err
	}

	if err := r.ensureAuthorCanWriteCMS(ctx, username, article.AttributedTo); err != nil {
		return nil, err
	}

	updated, err := r.Registry.Revisions().RestoreRevision(ctx, strings.TrimSpace(objectID), version)
	if err != nil {
		return nil, err
	}

	return r.convertCMSArticle(ctx, updated, true), nil
}

func (r *mutationResolver) CreateSeries(ctx context.Context, input model.CreateSeriesInput) (*model.Series, error) {
	if err := r.requireCMSSeriesEnabled(); err != nil {
		return nil, err
	}

	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	seriesSvc := r.Registry.Series()
	if seriesSvc == nil {
		return nil, errors.New("series service is not available")
	}

	slug := ""
	if input.Slug != nil {
		slug = cmsSlugify(*input.Slug)
	}
	if slug == "" {
		slug = cmsSlugify(input.Title)
	}

	now := time.Now()
	series := &models.Series{
		ID:          uuid.NewString(),
		AuthorID:    username,
		Title:       input.Title,
		Description: derefString(input.Description),
		Slug:        slug,
		CoverImage:  derefString(input.CoverImageURL),
		IsComplete:  input.IsComplete != nil && *input.IsComplete,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := seriesSvc.CreateSeries(ctx, series); err != nil {
		return nil, err
	}

	return r.convertCMSSeries(ctx, series), nil
}

func (r *mutationResolver) UpdateSeries(ctx context.Context, id string, input model.UpdateSeriesInput) (*model.Series, error) {
	if err := r.requireCMSSeriesEnabled(); err != nil {
		return nil, err
	}

	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	seriesSvc := r.Registry.Series()
	if seriesSvc == nil {
		return nil, errors.New("series service is not available")
	}

	authorID, seriesID, ok := parseSeriesGraphQLID(id)
	if !ok {
		return nil, errors.New("invalid series id")
	}
	if !r.isAdmin(ctx, username) && !strings.EqualFold(authorID, username) {
		return nil, errors.New("insufficient privileges for series update")
	}

	series, err := seriesSvc.GetSeries(ctx, authorID, seriesID)
	if err != nil {
		return nil, err
	}

	if input.Title != nil {
		series.Title = *input.Title
	}
	if input.Description != nil {
		series.Description = *input.Description
	}
	if input.CoverImageURL != nil {
		series.CoverImage = *input.CoverImageURL
	}
	if input.IsComplete != nil {
		series.IsComplete = *input.IsComplete
	}

	series.UpdatedAt = time.Now()
	if err := seriesSvc.UpdateSeries(ctx, series); err != nil {
		return nil, err
	}

	return r.convertCMSSeries(ctx, series), nil
}

func (r *mutationResolver) DeleteSeries(ctx context.Context, id string) (bool, error) {
	if err := r.requireCMSSeriesEnabled(); err != nil {
		return false, err
	}

	username, err := r.requireAuth(ctx)
	if err != nil {
		return false, err
	}

	seriesSvc := r.Registry.Series()
	if seriesSvc == nil {
		return false, errors.New("series service is not available")
	}

	authorID, seriesID, ok := parseSeriesGraphQLID(id)
	if !ok {
		return false, errors.New("invalid series id")
	}
	if !r.isAdmin(ctx, username) && !strings.EqualFold(authorID, username) {
		return false, errors.New("insufficient privileges for series delete")
	}

	if err := seriesSvc.DeleteSeries(ctx, authorID, seriesID); err != nil {
		return false, err
	}

	return true, nil
}

func (r *mutationResolver) AddArticleToSeries(ctx context.Context, seriesID string, articleID string, order *int) (*model.Series, error) {
	if err := r.requireCMSSeriesEnabled(); err != nil {
		return nil, err
	}

	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	seriesSvc := r.Registry.Series()
	if seriesSvc == nil {
		return nil, errors.New("series service is not available")
	}

	articleSvc := r.Registry.Articles()
	if articleSvc == nil {
		return nil, errors.New("article service is not available")
	}

	store := r.cmsStorage()
	if store == nil || store.Article() == nil {
		return nil, ErrStorageUnavailable
	}

	seriesID = strings.TrimSpace(seriesID)
	articleID = strings.TrimSpace(articleID)

	authorID, rawID, ok := parseSeriesGraphQLID(seriesID)
	if !ok {
		return nil, errors.New("invalid series id")
	}
	if !r.isAdmin(ctx, username) && !strings.EqualFold(authorID, username) {
		return nil, errors.New("insufficient privileges for series update")
	}

	if _, err := seriesSvc.GetSeries(ctx, authorID, rawID); err != nil {
		return nil, err
	}

	article, err := store.Article().GetArticle(ctx, articleID)
	if err != nil {
		return nil, err
	}
	if err := r.ensureAuthorCanWriteCMS(ctx, username, article.AttributedTo); err != nil {
		return nil, err
	}

	orderVal := 0
	if order != nil {
		orderVal = *order
	}
	article.SeriesID = &seriesID
	article.SeriesOrder = &orderVal

	if err := articleSvc.UpdateArticle(ctx, article); err != nil {
		return nil, err
	}

	// Return the updated series
	series, err := seriesSvc.GetSeries(ctx, authorID, rawID)
	if err != nil {
		return nil, err
	}

	return r.convertCMSSeries(ctx, series), nil
}

func (r *mutationResolver) RemoveArticleFromSeries(ctx context.Context, seriesID string, articleID string) (*model.Series, error) {
	if err := r.requireCMSSeriesEnabled(); err != nil {
		return nil, err
	}

	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	seriesSvc := r.Registry.Series()
	if seriesSvc == nil {
		return nil, errors.New("series service is not available")
	}

	articleSvc := r.Registry.Articles()
	if articleSvc == nil {
		return nil, errors.New("article service is not available")
	}

	store := r.cmsStorage()
	if store == nil || store.Article() == nil {
		return nil, ErrStorageUnavailable
	}

	seriesID = strings.TrimSpace(seriesID)
	articleID = strings.TrimSpace(articleID)

	authorID, rawID, ok := parseSeriesGraphQLID(seriesID)
	if !ok {
		return nil, errors.New("invalid series id")
	}
	if !r.isAdmin(ctx, username) && !strings.EqualFold(authorID, username) {
		return nil, errors.New("insufficient privileges for series update")
	}

	if _, err := seriesSvc.GetSeries(ctx, authorID, rawID); err != nil {
		return nil, err
	}

	article, err := store.Article().GetArticle(ctx, articleID)
	if err != nil {
		return nil, err
	}
	if err := r.ensureAuthorCanWriteCMS(ctx, username, article.AttributedTo); err != nil {
		return nil, err
	}

	if article.SeriesID != nil && strings.TrimSpace(*article.SeriesID) != "" && !strings.EqualFold(strings.TrimSpace(*article.SeriesID), seriesID) {
		return nil, errors.New("article does not belong to the specified series")
	}

	changed := article.SeriesID != nil || article.SeriesOrder != nil
	article.SeriesID = nil
	article.SeriesOrder = nil

	if changed {
		if err := articleSvc.UpdateArticle(ctx, article); err != nil {
			return nil, err
		}
	}

	series, err := seriesSvc.GetSeries(ctx, authorID, rawID)
	if err != nil {
		return nil, err
	}

	return r.convertCMSSeries(ctx, series), nil
}

func (r *mutationResolver) ReorderSeriesArticles(ctx context.Context, seriesID string, articleIDs []string) (*model.Series, error) {
	if err := r.requireCMSSeriesEnabled(); err != nil {
		return nil, err
	}

	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	seriesSvc := r.Registry.Series()
	if seriesSvc == nil {
		return nil, errors.New("series service is not available")
	}

	articleSvc := r.Registry.Articles()
	if articleSvc == nil {
		return nil, errors.New("article service is not available")
	}

	store := r.cmsStorage()
	if store == nil || store.Article() == nil {
		return nil, ErrStorageUnavailable
	}

	seriesID = strings.TrimSpace(seriesID)

	authorID, rawID, ok := parseSeriesGraphQLID(seriesID)
	if !ok {
		return nil, errors.New("invalid series id")
	}
	if !r.isAdmin(ctx, username) && !strings.EqualFold(authorID, username) {
		return nil, errors.New("insufficient privileges for series update")
	}

	if _, err := seriesSvc.GetSeries(ctx, authorID, rawID); err != nil {
		return nil, err
	}

	for i, id := range articleIDs {
		articleID := strings.TrimSpace(id)
		if articleID == "" {
			continue
		}

		article, err := store.Article().GetArticle(ctx, articleID)
		if err != nil {
			return nil, err
		}
		if err := r.ensureAuthorCanWriteCMS(ctx, username, article.AttributedTo); err != nil {
			return nil, err
		}
		if article.SeriesID == nil || !strings.EqualFold(strings.TrimSpace(*article.SeriesID), seriesID) {
			return nil, errors.New("article does not belong to the specified series")
		}

		order := i + 1
		article.SeriesOrder = &order
		if err := articleSvc.UpdateArticle(ctx, article); err != nil {
			return nil, err
		}
	}

	series, err := seriesSvc.GetSeries(ctx, authorID, rawID)
	if err != nil {
		return nil, err
	}

	return r.convertCMSSeries(ctx, series), nil
}

func (r *mutationResolver) CreateCategory(ctx context.Context, input model.CreateCategoryInput) (*model.Category, error) {
	if err := r.requireCMSCategoriesEnabled(); err != nil {
		return nil, err
	}

	_, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	categorySvc := r.Registry.Categories()
	if categorySvc == nil {
		return nil, errors.New("category service is not available")
	}

	slug := ""
	if input.Slug != nil {
		slug = cmsSlugify(*input.Slug)
	}
	if slug == "" {
		slug = cmsSlugify(input.Name)
	}
	if slug == "" {
		return nil, errors.New("category slug is required")
	}

	now := time.Now()
	category := &models.Category{
		ID:          cmsCategoryID(r.getDomain(), slug),
		Name:        input.Name,
		Slug:        slug,
		Description: derefString(input.Description),
		ParentID:    input.ParentID,
		Color:       derefString(input.Color),
		Order:       0,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if input.Order != nil {
		category.Order = *input.Order
	}

	if err := categorySvc.CreateCategory(ctx, category); err != nil {
		return nil, err
	}

	return r.convertCMSCategory(ctx, category, true), nil
}

func (r *mutationResolver) UpdateCategory(ctx context.Context, id string, input model.UpdateCategoryInput) (*model.Category, error) {
	if err := r.requireCMSCategoriesEnabled(); err != nil {
		return nil, err
	}

	_, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	categorySvc := r.Registry.Categories()
	if categorySvc == nil {
		return nil, errors.New("category service is not available")
	}

	category, err := categorySvc.GetCategory(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}

	if input.Slug != nil && strings.TrimSpace(*input.Slug) != "" && !strings.EqualFold(category.Slug, strings.TrimSpace(*input.Slug)) {
		return nil, errors.New("category slug updates are not supported")
	}
	if input.Name != nil {
		category.Name = *input.Name
	}
	if input.Description != nil {
		category.Description = *input.Description
	}
	if input.ParentID != nil {
		category.ParentID = input.ParentID
	}
	if input.Color != nil {
		category.Color = *input.Color
	}
	if input.Order != nil {
		category.Order = *input.Order
	}

	category.UpdatedAt = time.Now()
	if err := categorySvc.UpdateCategory(ctx, category); err != nil {
		return nil, err
	}

	return r.convertCMSCategory(ctx, category, true), nil
}

func (r *mutationResolver) DeleteCategory(ctx context.Context, id string) (bool, error) {
	if err := r.requireCMSCategoriesEnabled(); err != nil {
		return false, err
	}

	_, err := r.requireAuth(ctx)
	if err != nil {
		return false, err
	}

	categorySvc := r.Registry.Categories()
	if categorySvc == nil {
		return false, errors.New("category service is not available")
	}

	if err := categorySvc.DeleteCategory(ctx, strings.TrimSpace(id)); err != nil {
		return false, err
	}

	return true, nil
}

func (r *mutationResolver) AddArticleToCategory(ctx context.Context, categoryID string, articleID string) (*model.Article, error) {
	if err := r.requireCMSCategoriesEnabled(); err != nil {
		return nil, err
	}

	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	articleSvc := r.Registry.Articles()
	if articleSvc == nil {
		return nil, errors.New("article service is not available")
	}

	store := r.cmsStorage()
	if store == nil || store.Article() == nil {
		return nil, ErrStorageUnavailable
	}

	article, err := store.Article().GetArticle(ctx, strings.TrimSpace(articleID))
	if err != nil {
		return nil, err
	}

	if err := r.ensureAuthorCanWriteCMS(ctx, username, article.AttributedTo); err != nil {
		return nil, err
	}

	categoryID = strings.TrimSpace(categoryID)
	if categoryID == "" {
		return nil, errors.New("categoryId is required")
	}

	found := false
	for _, existing := range article.CategoryIDs {
		if strings.EqualFold(strings.TrimSpace(existing), categoryID) {
			found = true
			break
		}
	}
	if !found {
		article.CategoryIDs = append(article.CategoryIDs, categoryID)
	}

	now := time.Now()
	article.Updated = now
	article.UpdatedAt = now

	if err := articleSvc.UpdateArticle(ctx, article); err != nil {
		return nil, err
	}

	return r.convertCMSArticle(ctx, article, true), nil
}

func (r *mutationResolver) RemoveArticleFromCategory(ctx context.Context, categoryID string, articleID string) (*model.Article, error) {
	if err := r.requireCMSCategoriesEnabled(); err != nil {
		return nil, err
	}

	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	articleSvc := r.Registry.Articles()
	if articleSvc == nil {
		return nil, errors.New("article service is not available")
	}

	store := r.cmsStorage()
	if store == nil || store.Article() == nil {
		return nil, ErrStorageUnavailable
	}

	article, err := store.Article().GetArticle(ctx, strings.TrimSpace(articleID))
	if err != nil {
		return nil, err
	}

	if err := r.ensureAuthorCanWriteCMS(ctx, username, article.AttributedTo); err != nil {
		return nil, err
	}

	categoryID = strings.TrimSpace(categoryID)
	if categoryID == "" {
		return nil, errors.New("categoryId is required")
	}

	next := make([]string, 0, len(article.CategoryIDs))
	for _, existing := range article.CategoryIDs {
		if !strings.EqualFold(strings.TrimSpace(existing), categoryID) {
			next = append(next, existing)
		}
	}
	article.CategoryIDs = next

	now := time.Now()
	article.Updated = now
	article.UpdatedAt = now

	if err := articleSvc.UpdateArticle(ctx, article); err != nil {
		return nil, err
	}

	return r.convertCMSArticle(ctx, article, true), nil
}

func (r *mutationResolver) CreatePublication(ctx context.Context, input model.CreatePublicationInput) (*model.Publication, error) {
	if err := r.requireCMSLongFormEnabled(); err != nil {
		return nil, err
	}

	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	pubSvc := r.Registry.Publications()
	if pubSvc == nil {
		return nil, errors.New("publication service is not available")
	}

	store := r.cmsStorage()
	if store == nil || store.Media() == nil {
		return nil, ErrStorageUnavailable
	}

	slug := ""
	if input.Slug != nil {
		slug = cmsSlugify(*input.Slug)
	}
	if slug == "" {
		slug = cmsSlugify(input.Name)
	}
	if slug == "" {
		return nil, errors.New("publication slug is required")
	}

	now := time.Now()
	domain := r.getDomain()
	pubID := cmsPublicationID(domain, slug)

	publication := &models.Publication{
		ID:           pubID,
		Name:         input.Name,
		Tagline:      derefString(input.Tagline),
		Description:  derefString(input.Description),
		Slug:         slug,
		CustomDomain: derefString(input.CustomDomain),
		ActorID:      cmsLocalActorID(domain, username),
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if input.LogoID != nil && strings.TrimSpace(*input.LogoID) != "" {
		media, err := store.Media().GetMedia(ctx, strings.TrimSpace(*input.LogoID))
		if err != nil {
			return nil, err
		}
		publication.LogoURL = media.CDNUrl
	}
	if input.BannerID != nil && strings.TrimSpace(*input.BannerID) != "" {
		media, err := store.Media().GetMedia(ctx, strings.TrimSpace(*input.BannerID))
		if err != nil {
			return nil, err
		}
		publication.BannerURL = media.CDNUrl
	}

	if err := pubSvc.CreatePublication(ctx, publication); err != nil {
		return nil, err
	}

	ownerMember := &models.PublicationMember{
		PublicationID: pubID,
		UserID:        username,
		Role:          cmsPublicationRoleToStorage(model.PublicationRoleOwner),
		JoinedAt:      now,
	}
	_ = pubSvc.AddMember(ctx, ownerMember)

	created, err := pubSvc.GetPublication(ctx, pubID)
	if err != nil {
		return nil, err
	}

	return r.convertCMSPublication(ctx, created, true), nil
}

func (r *mutationResolver) UpdatePublication(ctx context.Context, id string, input model.UpdatePublicationInput) (*model.Publication, error) {
	if err := r.requireCMSLongFormEnabled(); err != nil {
		return nil, err
	}

	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	pubSvc := r.Registry.Publications()
	if pubSvc == nil {
		return nil, errors.New("publication service is not available")
	}

	store := r.cmsStorage()
	if store == nil || store.Media() == nil {
		return nil, ErrStorageUnavailable
	}

	publication, err := pubSvc.GetPublication(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}

	if input.Slug != nil && strings.TrimSpace(*input.Slug) != "" && !strings.EqualFold(publication.Slug, strings.TrimSpace(*input.Slug)) {
		return nil, errors.New("publication slug updates are not supported")
	}

	if err := r.ensureCanUpdatePublication(ctx, username, publication, pubSvc); err != nil {
		return nil, err
	}
	if err := cmsApplyPublicationUpdates(ctx, publication, input, store.Media()); err != nil {
		return nil, err
	}

	if err := pubSvc.UpdatePublication(ctx, publication); err != nil {
		return nil, err
	}

	updated, err := pubSvc.GetPublication(ctx, publication.ID)
	if err != nil {
		return nil, err
	}

	return r.convertCMSPublication(ctx, updated, true), nil
}

type cmsPublicationMemberGetter interface {
	GetMember(ctx context.Context, publicationID string, userID string) (*models.PublicationMember, error)
}

func (r *mutationResolver) ensureCanUpdatePublication(ctx context.Context, username string, publication *models.Publication, pubSvc cmsPublicationMemberGetter) error {
	if r.isAdmin(ctx, username) {
		return nil
	}

	member, err := pubSvc.GetMember(ctx, publication.ID, username)
	if err != nil || member == nil {
		return errors.New("insufficient privileges for publication update")
	}

	role := cmsPublicationRoleFromStorage(member.Role)
	switch role {
	case model.PublicationRoleOwner, model.PublicationRoleEditor:
		return nil
	default:
		return errors.New("insufficient privileges for publication update")
	}
}

type cmsMediaGetter interface {
	GetMedia(ctx context.Context, mediaID string) (*models.Media, error)
}

func cmsApplyPublicationUpdates(ctx context.Context, publication *models.Publication, input model.UpdatePublicationInput, mediaRepo cmsMediaGetter) error {
	if input.Name != nil {
		publication.Name = *input.Name
	}
	if input.Tagline != nil {
		publication.Tagline = *input.Tagline
	}
	if input.Description != nil {
		publication.Description = *input.Description
	}
	if input.CustomDomain != nil {
		publication.CustomDomain = *input.CustomDomain
	}
	if input.LogoID != nil {
		url, err := cmsMediaURLFromID(ctx, mediaRepo, *input.LogoID)
		if err != nil {
			return err
		}
		publication.LogoURL = url
	}
	if input.BannerID != nil {
		url, err := cmsMediaURLFromID(ctx, mediaRepo, *input.BannerID)
		if err != nil {
			return err
		}
		publication.BannerURL = url
	}
	return nil
}

func cmsMediaURLFromID(ctx context.Context, mediaRepo cmsMediaGetter, mediaID string) (string, error) {
	id := strings.TrimSpace(mediaID)
	if id == "" {
		return "", nil
	}
	media, err := mediaRepo.GetMedia(ctx, id)
	if err != nil {
		return "", err
	}
	return media.CDNUrl, nil
}

func cmsApplyArticleFeaturedImage(ctx context.Context, mediaRepo cmsMediaGetter, article *models.Article, featuredImageID *string) error {
	if featuredImageID == nil {
		return nil
	}
	if article == nil {
		return errors.New("article is required")
	}
	if mediaRepo == nil {
		return ErrStorageUnavailable
	}

	mediaID := strings.TrimSpace(*featuredImageID)
	if mediaID == "" {
		article.FeaturedImage = nil
		return nil
	}

	media, err := mediaRepo.GetMedia(ctx, mediaID)
	if err != nil {
		return err
	}
	article.FeaturedImage = media
	return nil
}

func cmsSetStringField(dest *string, value *string) {
	if value == nil {
		return
	}
	*dest = *value
}

func cmsSetStringPtrField(dest **string, value *string) {
	if value == nil {
		return
	}
	*dest = value
}

func cmsSetIntPtrField(dest **int, value *int) {
	if value == nil {
		return
	}
	*dest = value
}

func (r *mutationResolver) InvitePublicationMember(ctx context.Context, publicationID string, userID string, role model.PublicationRole) (*model.PublicationMember, error) {
	if err := r.requireCMSLongFormEnabled(); err != nil {
		return nil, err
	}

	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	pubSvc := r.Registry.Publications()
	if pubSvc == nil {
		return nil, errors.New("publication service is not available")
	}

	publicationID = strings.TrimSpace(publicationID)
	if publicationID == "" {
		return nil, errors.New("publicationId is required")
	}

	if !r.isAdmin(ctx, username) {
		member, err := pubSvc.GetMember(ctx, publicationID, username)
		if err != nil || member == nil {
			return nil, errors.New("insufficient privileges for publication membership")
		}
		if cmsPublicationRoleFromStorage(member.Role) != model.PublicationRoleOwner {
			return nil, errors.New("only publication owners can invite members")
		}
	}

	userID = cmsNormalizeUsername(userID)
	if userID == "" {
		return nil, errors.New("userId is required")
	}

	now := time.Now()
	member := &models.PublicationMember{
		PublicationID: publicationID,
		UserID:        userID,
		Role:          cmsPublicationRoleToStorage(role),
		JoinedAt:      now,
	}

	if err := pubSvc.AddMember(ctx, member); err != nil {
		return nil, err
	}

	created, err := pubSvc.GetMember(ctx, publicationID, userID)
	if err != nil {
		return nil, err
	}

	return r.convertCMSPublicationMember(ctx, created), nil
}

func (r *mutationResolver) RemovePublicationMember(ctx context.Context, publicationID string, userID string) (bool, error) {
	if err := r.requireCMSLongFormEnabled(); err != nil {
		return false, err
	}

	username, err := r.requireAuth(ctx)
	if err != nil {
		return false, err
	}

	pubSvc := r.Registry.Publications()
	if pubSvc == nil {
		return false, errors.New("publication service is not available")
	}

	publicationID = strings.TrimSpace(publicationID)
	if publicationID == "" {
		return false, errors.New("publicationId is required")
	}

	if !r.isAdmin(ctx, username) {
		member, err := pubSvc.GetMember(ctx, publicationID, username)
		if err != nil || member == nil {
			return false, errors.New("insufficient privileges for publication membership")
		}
		if cmsPublicationRoleFromStorage(member.Role) != model.PublicationRoleOwner {
			return false, errors.New("only publication owners can remove members")
		}
	}

	userID = cmsNormalizeUsername(userID)
	if userID == "" {
		return false, errors.New("userId is required")
	}

	if err := pubSvc.RemoveMember(ctx, publicationID, userID); err != nil {
		return false, err
	}

	return true, nil
}

func (r *mutationResolver) UpdatePublicationMemberRole(ctx context.Context, publicationID string, userID string, role model.PublicationRole) (*model.PublicationMember, error) {
	if err := r.requireCMSLongFormEnabled(); err != nil {
		return nil, err
	}

	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	pubSvc := r.Registry.Publications()
	if pubSvc == nil {
		return nil, errors.New("publication service is not available")
	}

	publicationID = strings.TrimSpace(publicationID)
	if publicationID == "" {
		return nil, errors.New("publicationId is required")
	}

	if !r.isAdmin(ctx, username) {
		member, err := pubSvc.GetMember(ctx, publicationID, username)
		if err != nil || member == nil {
			return nil, errors.New("insufficient privileges for publication membership")
		}
		if cmsPublicationRoleFromStorage(member.Role) != model.PublicationRoleOwner {
			return nil, errors.New("only publication owners can change roles")
		}
	}

	userID = cmsNormalizeUsername(userID)
	if userID == "" {
		return nil, errors.New("userId is required")
	}

	if err := pubSvc.UpdateMemberRole(ctx, publicationID, userID, cmsPublicationRoleToStorage(role)); err != nil {
		return nil, err
	}

	updated, err := pubSvc.GetMember(ctx, publicationID, userID)
	if err != nil {
		return nil, err
	}

	return r.convertCMSPublicationMember(ctx, updated), nil
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
