package graph

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

func (r *Resolver) cmsStorage() core.RepositoryStorage {
	if r == nil {
		return nil
	}
	if !r.cmsLongFormEnabled() {
		return nil
	}
	if r.Storage != nil {
		return r.Storage
	}
	if r.Registry != nil {
		return r.Registry.GetStorage()
	}
	return nil
}

func (r *Resolver) convertCMSDraft(ctx context.Context, draft *models.Draft) *model.Draft {
	if draft == nil {
		return nil
	}

	author := r.resolveActorByID(ctx, draft.AuthorID)
	contentType := cmsObjectTypeFromStorage(draft.ContentType)
	contentFormat := cmsContentFormatFromStorage(draft.ContentFormat)
	status := cmsDraftStatusFromStorage(draft.Status)

	var scheduledAt *model.Time
	if draft.ScheduledAt != nil && !draft.ScheduledAt.IsZero() {
		t := model.Time(*draft.ScheduledAt)
		scheduledAt = &t
	}

	var objectID *string
	if draft.ObjectID != nil && strings.TrimSpace(*draft.ObjectID) != "" {
		v := strings.TrimSpace(*draft.ObjectID)
		objectID = &v
	}

	return &model.Draft{
		ID:              draft.ID,
		Author:          author,
		ContentType:     contentType,
		Title:           cmsOptionalString(strings.TrimSpace(draft.Title)),
		Slug:            cmsOptionalString(strings.TrimSpace(draft.Slug)),
		Content:         draft.Content,
		ContentFormat:   contentFormat,
		Status:          status,
		ScheduledAt:     scheduledAt,
		ObjectID:        objectID,
		AutosaveVersion: draft.AutosaveVersion,
		LastSavedAt:     model.Time(draft.LastSavedAt),
		CreatedAt:       model.Time(draft.CreatedAt),
		UpdatedAt:       model.Time(draft.UpdatedAt),
	}
}

func (r *Resolver) convertCMSRevision(ctx context.Context, revision *models.Revision) *model.Revision {
	if revision == nil {
		return nil
	}

	id := strings.TrimSpace(revision.ID)
	if id == "" {
		id = fmt.Sprintf("%s#%d", revision.ObjectID, revision.Version)
	}

	changedBy := r.resolveActorByID(ctx, revision.ChangedBy)
	changeType := cmsChangeTypeFromStorage(revision.ChangeType)

	var metadataJSON *string
	if strings.TrimSpace(revision.MetadataJSON) != "" {
		v := revision.MetadataJSON
		metadataJSON = &v
	}

	var changeSummary *string
	if strings.TrimSpace(revision.ChangeSummary) != "" {
		v := revision.ChangeSummary
		changeSummary = &v
	}

	return &model.Revision{
		ID:            id,
		ObjectID:      revision.ObjectID,
		Version:       revision.Version,
		Content:       revision.Content,
		MetadataJSON:  metadataJSON,
		ChangedBy:     changedBy,
		ChangeSummary: changeSummary,
		ChangeType:    changeType,
		CreatedAt:     model.Time(revision.CreatedAt),
	}
}

func (r *Resolver) convertCMSSeries(ctx context.Context, series *models.Series) *model.Series {
	if series == nil {
		return nil
	}

	author := r.resolveActorByID(ctx, series.AuthorID)
	description := cmsOptionalString(strings.TrimSpace(series.Description))
	cover := cmsOptionalString(strings.TrimSpace(series.CoverImage))

	return &model.Series{
		ID:            cmsSeriesGraphQLID(series.AuthorID, series.ID),
		Author:        author,
		Title:         series.Title,
		Description:   description,
		Slug:          series.Slug,
		CoverImageURL: cover,
		IsComplete:    series.IsComplete,
		ArticleCount:  series.ArticleCount,
		CreatedAt:     model.Time(series.CreatedAt),
		UpdatedAt:     model.Time(series.UpdatedAt),
	}
}

func (r *Resolver) convertCMSCategory(ctx context.Context, category *models.Category, includeRelations bool) *model.Category {
	if category == nil {
		return nil
	}

	description := cmsOptionalString(strings.TrimSpace(category.Description))
	color := cmsOptionalString(strings.TrimSpace(category.Color))

	result := &model.Category{
		ID:           category.ID,
		Name:         category.Name,
		Slug:         category.Slug,
		Description:  description,
		ArticleCount: category.ArticleCount,
		Order:        category.Order,
		Color:        color,
		CreatedAt:    model.Time(category.CreatedAt),
		UpdatedAt:    model.Time(category.UpdatedAt),
		Children:     []*model.Category{},
	}

	if !includeRelations {
		return result
	}

	store := r.cmsStorage()
	if store == nil || store.Category() == nil {
		return result
	}

	if category.ParentID != nil && strings.TrimSpace(*category.ParentID) != "" {
		parent, err := store.Category().GetCategory(ctx, strings.TrimSpace(*category.ParentID))
		if err == nil {
			result.Parent = r.convertCMSCategory(ctx, parent, false)
		}
	}

	children, err := store.Category().ListCategories(ctx, &category.ID, 500)
	if err == nil && len(children) > 0 {
		result.Children = make([]*model.Category, 0, len(children))
		for _, child := range children {
			result.Children = append(result.Children, r.convertCMSCategory(ctx, child, false))
		}
	}

	return result
}

func (r *Resolver) convertCMSArticle(ctx context.Context, article *models.Article, includeRelations bool) *model.Article {
	if article == nil {
		return nil
	}

	wordCount := article.WordCount
	if wordCount == 0 {
		wordCount = cmsEstimateWordCount(article.Content)
	}

	readingMinutes := article.ReadingTimeMinutes
	if readingMinutes == 0 {
		readingMinutes = cmsEstimateReadingMinutes(wordCount)
	}

	toc := make([]*model.TOCEntry, 0, len(article.TableOfContents))
	for _, entry := range article.TableOfContents {
		toc = append(toc, &model.TOCEntry{
			ID:    entry.ID,
			Level: entry.Level,
			Text:  entry.Text,
		})
	}

	slug := strings.TrimSpace(article.Slug)
	if slug == "" {
		slug = cmsExtractSlugFromURL(article.ID)
	}

	result := &model.Article{
		ID:     article.ID,
		Slug:   slug,
		Author: r.resolveActorByID(ctx, article.AttributedTo),

		Title:    article.Name,
		Subtitle: cmsOptionalString(strings.TrimSpace(article.Subtitle)),
		Excerpt:  cmsOptionalString(strings.TrimSpace(article.Excerpt)),

		Content:       article.Content,
		ContentFormat: cmsContentFormatFromStorage(article.ContentFormat),

		FeaturedImage:   r.convertMediaToGraphQL(article.FeaturedImage),
		TableOfContents: toc,

		ReadingTimeMinutes: readingMinutes,
		WordCount:          wordCount,

		SeriesOrder: article.SeriesOrder,
		Categories:  []*model.Category{},

		SEOTitle:       cmsOptionalString(strings.TrimSpace(article.SEOTitle)),
		SEODescription: cmsOptionalString(strings.TrimSpace(article.SEODescription)),
		CanonicalURL:   cmsOptionalString(strings.TrimSpace(article.CanonicalURL)),
		OGImage:        cmsOptionalString(strings.TrimSpace(article.OGImage)),

		EditorNotes:  cmsOptionalString(strings.TrimSpace(article.EditorNotes)),
		ReviewStatus: cmsOptionalString(strings.TrimSpace(article.ReviewStatus)),

		PublishedAt: model.Time(article.Published),
		CreatedAt:   model.Time(article.CreatedAt),
		UpdatedAt:   model.Time(article.UpdatedAt),
	}

	if !includeRelations {
		return result
	}

	store := r.cmsStorage()
	if store == nil {
		return result
	}

	if article.SeriesID != nil && strings.TrimSpace(*article.SeriesID) != "" && store.Series() != nil {
		authorID, seriesID, ok := parseSeriesGraphQLID(strings.TrimSpace(*article.SeriesID))
		if ok {
			series, err := store.Series().GetSeries(ctx, authorID, seriesID)
			if err == nil {
				result.Series = r.convertCMSSeries(ctx, series)
			}
		}
	}

	if len(article.CategoryIDs) > 0 && store.Category() != nil {
		for _, categoryID := range article.CategoryIDs {
			categoryID = strings.TrimSpace(categoryID)
			if categoryID == "" {
				continue
			}
			category, err := store.Category().GetCategory(ctx, categoryID)
			if err != nil {
				continue
			}
			result.Categories = append(result.Categories, r.convertCMSCategory(ctx, category, false))
		}
	}

	return result
}

func (r *Resolver) convertCMSPublication(ctx context.Context, publication *models.Publication, includeMembers bool) *model.Publication {
	if publication == nil {
		return nil
	}

	actorID := strings.TrimSpace(publication.ActorID)
	if actorID == "" {
		actorID = publication.ID
	}

	result := &model.Publication{
		ID:           publication.ID,
		Name:         publication.Name,
		Tagline:      cmsOptionalString(strings.TrimSpace(publication.Tagline)),
		Description:  cmsOptionalString(strings.TrimSpace(publication.Description)),
		Slug:         publication.Slug,
		LogoURL:      cmsOptionalString(strings.TrimSpace(publication.LogoURL)),
		BannerURL:    cmsOptionalString(strings.TrimSpace(publication.BannerURL)),
		CustomDomain: cmsOptionalString(strings.TrimSpace(publication.CustomDomain)),
		Actor:        r.resolveActorByID(ctx, actorID),
		Members:      []*model.PublicationMember{},
		CreatedAt:    model.Time(publication.CreatedAt),
		UpdatedAt:    model.Time(publication.UpdatedAt),
	}

	if !includeMembers {
		return result
	}

	if r.Registry == nil {
		return result
	}

	pubSvc := r.Registry.Publications()
	if pubSvc == nil {
		return result
	}

	members, err := pubSvc.ListMembers(ctx, publication.ID)
	if err != nil {
		return result
	}

	result.Members = make([]*model.PublicationMember, 0, len(members))
	for _, member := range members {
		result.Members = append(result.Members, r.convertCMSPublicationMember(ctx, member))
	}

	return result
}

func (r *Resolver) convertCMSPublicationMember(ctx context.Context, member *models.PublicationMember) *model.PublicationMember {
	if member == nil {
		return nil
	}

	userID := strings.TrimSpace(member.UserID)
	if userID == "" {
		userID = member.DisplayName
	}

	actor := r.resolveActorByID(ctx, userID)
	if actor == nil {
		actor = r.resolveActorByID(ctx, member.UserID)
	}

	displayName := cmsOptionalString(strings.TrimSpace(member.DisplayName))
	bio := cmsOptionalString(strings.TrimSpace(member.Bio))

	return &model.PublicationMember{
		User:        actor,
		Role:        cmsPublicationRoleFromStorage(member.Role),
		DisplayName: displayName,
		Bio:         bio,
		JoinedAt:    model.Time(member.JoinedAt),
	}
}

func cmsOptionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func cmsSeriesGraphQLID(authorID, seriesID string) string {
	authorID = strings.TrimSpace(authorID)
	seriesID = strings.TrimSpace(seriesID)
	if authorID == "" || seriesID == "" {
		return seriesID
	}
	return fmt.Sprintf("%s|%s", authorID, seriesID)
}

func parseSeriesGraphQLID(value string) (authorID string, seriesID string, ok bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "", false
	}

	parts := strings.SplitN(value, "|", 2)
	if len(parts) != 2 {
		return "", "", false
	}

	authorID = strings.TrimSpace(parts[0])
	seriesID = strings.TrimSpace(parts[1])
	if authorID == "" || seriesID == "" {
		return "", "", false
	}

	return authorID, seriesID, true
}

func (r *Resolver) ensureAuthorCanWriteCMS(ctx context.Context, username string, attributedTo string) error {
	if username == "" {
		return ErrAuthenticationRequired
	}

	if r.isAdmin(ctx, username) {
		return nil
	}

	expected := cmsLocalActorID(r.getDomain(), username)
	if strings.EqualFold(strings.TrimSpace(attributedTo), expected) {
		return nil
	}

	if strings.EqualFold(cmsNormalizeUsername(attributedTo), username) {
		return nil
	}

	return errors.New("insufficient privileges for CMS write")
}
