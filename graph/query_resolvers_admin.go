package graph

import (
	"context"
	"errors"
	"strings"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/services/scheduled"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// ScheduledStatuses is the resolver for the scheduledStatuses field.
func (r *queryResolver) ScheduledStatuses(ctx context.Context, first *int, after *model.Cursor) ([]*model.ScheduledStatus, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	pagination := interfaces.PaginationOptions{
		Limit: 20,
	}
	if first != nil && *first > 0 && *first <= 100 {
		pagination.Limit = *first
	}
	if after != nil {
		pagination.Cursor = string(*after)
	}

	result, err := r.Registry.Scheduled().ListScheduledStatuses(ctx, &scheduled.ListScheduledStatusesQuery{
		Username:   username,
		Pagination: pagination,
	})
	if err != nil {
		r.Logger.Error("Failed to list scheduled statuses",
			zap.String("user", username),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to list scheduled statuses"), err)
	}

	statuses := make([]*model.ScheduledStatus, len(result.ScheduledStatuses))
	for i, s := range result.ScheduledStatuses {
		statuses[i] = r.convertScheduledStatusToGraphQL(ctx, s)
	}

	return statuses, nil
}

// ScheduledStatus is the resolver for the scheduledStatus field.
func (r *queryResolver) ScheduledStatus(ctx context.Context, id string) (*model.ScheduledStatus, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	result, err := r.Registry.Scheduled().GetScheduledStatus(ctx, &scheduled.GetScheduledStatusQuery{
		ID:       id,
		Username: username,
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, nil
		}
		r.Logger.Error("Failed to get scheduled status",
			zap.String("user", username),
			zap.String("id", id),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to get scheduled status"), err)
	}

	return r.convertScheduledStatusToGraphQL(ctx, result.ScheduledStatus), nil
}
