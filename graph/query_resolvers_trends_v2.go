package graph

import (
	"context"
	"errors"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/trends"
	"go.uber.org/zap"
)

const (
	defaultTrendsLimit         = 10
	defaultTrendingStatusesLim = 20
	maxTrendsLimit             = 40

	trendTypeHashtag = "hashtag"
	trendTypeStatus  = "status"
	trendTypeLink    = "link"
)

func (r *queryResolver) Trends(ctx context.Context, limit *int) ([]*model.TrendingItem, error) {
	lim := defaultTrendsLimit
	if limit != nil && *limit > 0 && *limit <= maxTrendsLimit {
		lim = *limit
	}

	if r.Storage == nil {
		return nil, errors.New("storage is not available")
	}

	service := trends.NewService(r.Storage)
	items, err := service.GetTrends(ctx, lim)
	if err != nil {
		r.Logger.Error("failed to get trends", zap.Error(err))
		return nil, errors.Join(errors.New("failed to get trends"), err)
	}

	result := make([]*model.TrendingItem, 0, len(items))
	for _, item := range items {
		converted := convertTrendToTrendingItem(item)
		if converted == nil {
			continue
		}
		result = append(result, converted)
	}

	return result, nil
}

func (r *queryResolver) TrendingTags(ctx context.Context, limit *int) ([]*model.TrendingTag, error) {
	lim := defaultTrendsLimit
	if limit != nil && *limit > 0 && *limit <= maxTrendsLimit {
		lim = *limit
	}

	if r.Storage == nil {
		return nil, errors.New("storage is not available")
	}

	service := trends.NewService(r.Storage)
	items, err := service.GetTrendingHashtags(ctx, lim)
	if err != nil {
		r.Logger.Error("failed to get trending tags", zap.Error(err))
		return nil, errors.Join(errors.New("failed to get trending tags"), err)
	}

	result := make([]*model.TrendingTag, 0, len(items))
	for i := range items {
		tag := items[i]
		result = append(result, convertHashtagTrendToModel(&tag))
	}
	return result, nil
}

func (r *queryResolver) TrendingStatuses(ctx context.Context, limit *int) ([]*model.TrendingStatus, error) {
	lim := defaultTrendingStatusesLim
	if limit != nil && *limit > 0 && *limit <= maxTrendsLimit {
		lim = *limit
	}

	if r.Storage == nil {
		return nil, errors.New("storage is not available")
	}

	service := trends.NewService(r.Storage)
	items, err := service.GetTrendingStatuses(ctx, lim)
	if err != nil {
		r.Logger.Error("failed to get trending statuses", zap.Error(err))
		return nil, errors.Join(errors.New("failed to get trending statuses"), err)
	}

	result := make([]*model.TrendingStatus, 0, len(items))
	for i := range items {
		st := items[i]
		result = append(result, convertStatusTrendToModel(&st))
	}
	return result, nil
}

func (r *queryResolver) TrendingLinks(ctx context.Context, limit *int) ([]*model.TrendingLink, error) {
	lim := defaultTrendsLimit
	if limit != nil && *limit > 0 && *limit <= maxTrendsLimit {
		lim = *limit
	}

	if r.Storage == nil {
		return nil, errors.New("storage is not available")
	}

	service := trends.NewService(r.Storage)
	items, err := service.GetTrendingLinks(ctx, lim)
	if err != nil {
		r.Logger.Error("failed to get trending links", zap.Error(err))
		return nil, errors.Join(errors.New("failed to get trending links"), err)
	}

	result := make([]*model.TrendingLink, 0, len(items))
	for i := range items {
		ln := items[i]
		result = append(result, convertLinkTrendToModel(&ln))
	}
	return result, nil
}

func convertHashtagTrendToModel(trend *trends.HashtagTrend) *model.TrendingTag {
	if trend == nil {
		return nil
	}

	history := make([]int, 0, len(trend.History))
	for _, v := range trend.History {
		history = append(history, int(v))
	}

	return &model.TrendingTag{
		Name:     trend.Name,
		URL:      trend.URL,
		History:  history,
		Uses:     int(trend.Uses),
		Accounts: int(trend.Accounts),
	}
}

func convertStatusTrendToModel(trend *trends.StatusTrend) *model.TrendingStatus {
	if trend == nil {
		return nil
	}

	return &model.TrendingStatus{
		ID:          trend.StatusID,
		URL:         trend.URL,
		AuthorID:    trend.AuthorID,
		Content:     trend.Content,
		Engagements: int(trend.Engagements),
		PublishedAt: model.Time(trend.PublishedAt),
	}
}

func convertLinkTrendToModel(trend *trends.LinkTrend) *model.TrendingLink {
	if trend == nil {
		return nil
	}

	return &model.TrendingLink{
		URL:         trend.URL,
		Title:       trend.Title,
		Description: trend.Description,
		Type:        trend.Type,
		AuthorName:  trend.AuthorName,
		Image:       trend.Image,
		Shares:      int(trend.Shares),
	}
}

func convertTrendToTrendingItem(item trends.Trend) *model.TrendingItem {
	switch item.Type {
	case trendTypeHashtag:
		tag, ok := item.Value.(trends.HashtagTrend)
		if !ok {
			if tagPtr, ok := item.Value.(*trends.HashtagTrend); ok && tagPtr != nil {
				tag = *tagPtr
			} else {
				return nil
			}
		}
		return &model.TrendingItem{
			Type:    model.TrendingItemTypeHashtag,
			Hashtag: convertHashtagTrendToModel(&tag),
		}
	case trendTypeStatus:
		st, ok := item.Value.(trends.StatusTrend)
		if !ok {
			if stPtr, ok := item.Value.(*trends.StatusTrend); ok && stPtr != nil {
				st = *stPtr
			} else {
				return nil
			}
		}
		return &model.TrendingItem{
			Type:   model.TrendingItemTypeStatus,
			Status: convertStatusTrendToModel(&st),
		}
	case trendTypeLink:
		ln, ok := item.Value.(trends.LinkTrend)
		if !ok {
			if lnPtr, ok := item.Value.(*trends.LinkTrend); ok && lnPtr != nil {
				ln = *lnPtr
			} else {
				return nil
			}
		}
		return &model.TrendingItem{
			Type: model.TrendingItemTypeLink,
			Link: convertLinkTrendToModel(&ln),
		}
	default:
		return nil
	}
}
