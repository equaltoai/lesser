package graph

import (
	"context"
	"errors"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/notifications"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// Notifications is the resolver for the notifications field.
func (r *queryResolver) Notifications(ctx context.Context, types []string, excludeTypes []string, first *int, after *model.Cursor) (*model.NotificationConnection, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Build query
	// Build pagination
	pagination := interfaces.PaginationOptions{
		Limit: 20,
	}

	if first != nil && *first > 0 && *first <= 100 {
		pagination.Limit = *first
	}

	if after != nil {
		pagination.Cursor = string(*after)
	}

	query := &notifications.ListNotificationsQuery{
		UserID:       username,
		Types:        types,
		ExcludeTypes: excludeTypes,
		Pagination:   pagination,
	}

	// Get notifications using service
	result, err := r.Registry.Notifications().ListNotifications(ctx, query)
	if err != nil {
		r.Logger.Error("Failed to list notifications",
			zap.String("user", username),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to list notifications"), err)
	}

	// Convert to GraphQL connection
	edges := make([]*model.NotificationEdge, len(result.Notifications))
	for i, notif := range result.Notifications {
		edges[i] = &model.NotificationEdge{
			Node:   r.convertNotificationToGraphQL(ctx, notif),
			Cursor: model.Cursor(notif.ID),
		}
	}

	var startCursor, endCursor *model.Cursor
	if err := common.ValidateSliceNotEmpty("edges", edges); err == nil {
		startCursor = &edges[0].Cursor
		endCursor = &edges[len(edges)-1].Cursor
	}

	return &model.NotificationConnection{
		Edges: edges,
		PageInfo: &model.PageInfo{
			HasNextPage:     result.Pagination != nil && result.Pagination.HasMore,
			HasPreviousPage: after != nil,
			StartCursor:     startCursor,
			EndCursor:       endCursor,
		},
		TotalCount: len(result.Notifications),
	}, nil
}
