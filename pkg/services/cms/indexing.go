package cms

import (
	"context"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"go.uber.org/zap"
)

func cmsArticleIndexEntries(article *models.Article) []*models.CMSArticleIndex {
	if article == nil {
		return nil
	}

	now := time.Now()
	articleID := strings.TrimSpace(article.ID)
	if articleID == "" {
		return nil
	}

	sk := models.CMSArticleIndexSK(article.Published, articleID)
	if sk == "" {
		return nil
	}

	published := article.Published
	entries := make([]*models.CMSArticleIndex, 0, 2+len(article.CategoryIDs))

	if pk := models.CMSArticleIndexPKForAuthor(article.AttributedTo); pk != "" {
		entries = append(entries, &models.CMSArticleIndex{
			PK:          pk,
			SK:          sk,
			ArticleID:   articleID,
			PublishedAt: published,
			CreatedAt:   now,
		})
	}

	if seriesID := cmsExtractSeriesGraphQLID(article); seriesID != "" {
		if pk := models.CMSArticleIndexPKForSeries(seriesID); pk != "" {
			entries = append(entries, &models.CMSArticleIndex{
				PK:          pk,
				SK:          sk,
				ArticleID:   articleID,
				PublishedAt: published,
				CreatedAt:   now,
			})
		}
	}

	seenCategories := map[string]struct{}{}
	for _, categoryID := range article.CategoryIDs {
		categoryID = strings.TrimSpace(categoryID)
		if categoryID == "" {
			continue
		}
		if _, ok := seenCategories[categoryID]; ok {
			continue
		}
		seenCategories[categoryID] = struct{}{}

		if pk := models.CMSArticleIndexPKForCategory(categoryID); pk != "" {
			entries = append(entries, &models.CMSArticleIndex{
				PK:          pk,
				SK:          sk,
				ArticleID:   articleID,
				PublishedAt: published,
				CreatedAt:   now,
			})
		}
	}

	return entries
}

func cmsArticleIndexEntriesForRemovedGroups(before *models.Article, after *models.Article) []*models.CMSArticleIndex {
	if before == nil {
		return nil
	}
	if after == nil {
		return cmsArticleIndexEntries(before)
	}

	beforeID := strings.TrimSpace(before.ID)
	if beforeID == "" {
		return nil
	}

	sk := models.CMSArticleIndexSK(before.Published, beforeID)
	if sk == "" {
		return nil
	}

	now := time.Now()
	removed := make([]*models.CMSArticleIndex, 0, 2+len(before.CategoryIDs))

	beforeAuthor := strings.TrimSpace(before.AttributedTo)
	afterAuthor := strings.TrimSpace(after.AttributedTo)
	if beforeAuthor != "" && !strings.EqualFold(beforeAuthor, afterAuthor) {
		if pk := models.CMSArticleIndexPKForAuthor(beforeAuthor); pk != "" {
			removed = append(removed, &models.CMSArticleIndex{
				PK:        pk,
				SK:        sk,
				ArticleID: beforeID,
				CreatedAt: now,
			})
		}
	}

	beforeSeries := cmsExtractSeriesGraphQLID(before)
	afterSeries := cmsExtractSeriesGraphQLID(after)
	if beforeSeries != "" && !strings.EqualFold(beforeSeries, afterSeries) {
		if pk := models.CMSArticleIndexPKForSeries(beforeSeries); pk != "" {
			removed = append(removed, &models.CMSArticleIndex{
				PK:        pk,
				SK:        sk,
				ArticleID: beforeID,
				CreatedAt: now,
			})
		}
	}

	beforeCats := cmsExtractCategoryIDSet(before)
	afterCats := cmsExtractCategoryIDSet(after)
	for categoryID := range beforeCats {
		if _, ok := afterCats[categoryID]; ok {
			continue
		}
		if pk := models.CMSArticleIndexPKForCategory(categoryID); pk != "" {
			removed = append(removed, &models.CMSArticleIndex{
				PK:        pk,
				SK:        sk,
				ArticleID: beforeID,
				CreatedAt: now,
			})
		}
	}

	return removed
}

func cmsExtractSeriesGraphQLID(article *models.Article) string {
	if article == nil || article.SeriesID == nil {
		return ""
	}
	return strings.TrimSpace(*article.SeriesID)
}

func cmsParseSeriesGraphQLID(value string) (authorID string, seriesID string, ok bool) {
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

func cmsExtractCategoryIDSet(article *models.Article) map[string]struct{} {
	out := map[string]struct{}{}
	if article == nil {
		return out
	}

	for _, categoryID := range article.CategoryIDs {
		categoryID = strings.TrimSpace(categoryID)
		if categoryID == "" {
			continue
		}
		out[categoryID] = struct{}{}
	}

	return out
}

func cmsUpdateArticleCountsBestEffort(ctx context.Context, seriesRepo *repositories.SeriesRepository, categoryRepo *repositories.CategoryRepository, before *models.Article, after *models.Article, logger *zap.Logger) {
	if seriesRepo == nil && categoryRepo == nil {
		return
	}

	seriesBefore := cmsExtractSeriesGraphQLID(before)
	seriesAfter := cmsExtractSeriesGraphQLID(after)

	if seriesRepo != nil && seriesBefore != seriesAfter {
		if authorID, rawID, ok := cmsParseSeriesGraphQLID(seriesBefore); ok {
			if err := seriesRepo.UpdateArticleCount(ctx, authorID, rawID, -1); err != nil && logger != nil {
				logger.Warn("failed to decrement series article count", zap.Error(err), zap.String("series_id", seriesBefore))
			}
		}
		if authorID, rawID, ok := cmsParseSeriesGraphQLID(seriesAfter); ok {
			if err := seriesRepo.UpdateArticleCount(ctx, authorID, rawID, 1); err != nil && logger != nil {
				logger.Warn("failed to increment series article count", zap.Error(err), zap.String("series_id", seriesAfter))
			}
		}
	}

	if categoryRepo == nil {
		return
	}

	beforeCats := cmsExtractCategoryIDSet(before)
	afterCats := cmsExtractCategoryIDSet(after)

	for categoryID := range beforeCats {
		if _, ok := afterCats[categoryID]; ok {
			continue
		}
		if err := categoryRepo.UpdateArticleCount(ctx, categoryID, -1); err != nil && logger != nil {
			logger.Warn("failed to decrement category article count", zap.Error(err), zap.String("category_id", categoryID))
		}
	}

	for categoryID := range afterCats {
		if _, ok := beforeCats[categoryID]; ok {
			continue
		}
		if err := categoryRepo.UpdateArticleCount(ctx, categoryID, 1); err != nil && logger != nil {
			logger.Warn("failed to increment category article count", zap.Error(err), zap.String("category_id", categoryID))
		}
	}
}
