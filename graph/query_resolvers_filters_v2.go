package graph

import (
	"context"
	"errors"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/mastodon"
	"go.uber.org/zap"
)

func (r *queryResolver) Filters(ctx context.Context) ([]*mastodon.Filter, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	moderationRepo := r.Storage.Moderation()
	if moderationRepo == nil {
		return nil, errors.New("moderation repository is not available")
	}

	filters, err := moderationRepo.GetFiltersForUser(ctx, username)
	if err != nil {
		r.Logger.Error("failed to get filters", zap.String("user", username), zap.Error(err))
		return nil, errors.Join(errors.New("failed to get filters"), err)
	}

	converter := r.MastodonConv
	if converter == nil {
		baseURL := ""
		if r.Config != nil {
			baseURL = r.Config.BaseURL()
		}
		converter = mastodon.NewConverter(baseURL)
	}

	result := make([]*mastodon.Filter, 0, len(filters))
	for _, filter := range filters {
		if filter == nil {
			continue
		}
		keywords, err := moderationRepo.GetFilterKeywords(ctx, filter.ID)
		if err != nil {
			r.Logger.Warn("failed to get filter keywords",
				zap.String("filter_id", filter.ID),
				zap.Error(err))
			keywords = nil
		}

		statuses, err := moderationRepo.GetFilterStatuses(ctx, filter.ID)
		if err != nil {
			r.Logger.Warn("failed to get filter statuses",
				zap.String("filter_id", filter.ID),
				zap.Error(err))
			statuses = nil
		}

		result = append(result, converter.ConvertFilterToMastodon(filter, keywords, statuses))
	}

	return result, nil
}

func (r *queryResolver) Filter(ctx context.Context, id string) (*mastodon.Filter, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if err := common.ValidateFilterParamID(id); err != nil {
		return nil, err
	}

	moderationRepo := r.Storage.Moderation()
	if moderationRepo == nil {
		return nil, errors.New("moderation repository is not available")
	}

	filter, err := moderationRepo.GetFilter(ctx, id)
	if err != nil {
		r.Logger.Error("failed to get filter",
			zap.String("user", username),
			zap.String("filter_id", id),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to get filter"), err)
	}

	if filter == nil || filter.Username != username {
		return nil, common.ErrNotFound("filter")
	}

	keywords, err := moderationRepo.GetFilterKeywords(ctx, id)
	if err != nil {
		r.Logger.Warn("failed to get filter keywords",
			zap.String("filter_id", id),
			zap.Error(err))
		keywords = nil
	}
	statuses, err := moderationRepo.GetFilterStatuses(ctx, id)
	if err != nil {
		r.Logger.Warn("failed to get filter statuses",
			zap.String("filter_id", id),
			zap.Error(err))
		statuses = nil
	}

	converter := r.MastodonConv
	if converter == nil {
		baseURL := ""
		if r.Config != nil {
			baseURL = r.Config.BaseURL()
		}
		converter = mastodon.NewConverter(baseURL)
	}

	return converter.ConvertFilterToMastodon(filter, keywords, statuses), nil
}
