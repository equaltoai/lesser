package graph

import (
	"context"
	"errors"

	"github.com/equaltoai/lesser/graph/model"
	"go.uber.org/zap"
)

// UpdateQuotePermissions is the resolver for the updateQuotePermissions field.
func (r *mutationResolver) UpdateQuotePermissions(ctx context.Context, noteID string, quoteable bool, permission model.QuotePermission) (*model.UpdateQuotePermissionsPayload, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// TODO: Implement the quotes service and UpdateQuotePermissions
	// For now, return a stub response
	r.Logger.Warn("UpdateQuotePermissions not fully implemented yet - quotes service needed",
		zap.String("user", username),
		zap.String("note_id", noteID),
		zap.Bool("quoteable", quoteable),
		zap.String("permission", string(permission)))

	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", 1)

	return &model.UpdateQuotePermissionsPayload{
		Success:        false,
		Note:           nil,
		AffectedQuotes: 0,
	}, errors.New("UpdateQuotePermissions not yet implemented - quotes service needs to be created")
}
