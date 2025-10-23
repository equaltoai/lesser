package graph

import (
	"context"

	"github.com/equaltoai/lesser/graph/model"
	"go.uber.org/zap"
)

// QuoteActivity implements SubscriptionResolver
// This subscription is managed by the GraphQLSubscriptionManager which uses DynamoDB-backed persistence
func (r *subscriptionResolver) QuoteActivity(ctx context.Context, noteID string) (<-chan *model.QuoteActivityUpdate, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Add connection ID to context if available (from WebSocket)
	// The subscription manager will extract this for DynamoDB persistence
	ctx = WithConnectionID(ctx, r.getConnectionID(ctx))

	// Delegate to the subscription manager which handles DynamoDB persistence
	ch, err := r.SubscriptionManager.SubscribeToQuoteActivity(ctx, username, noteID, nil)
	if err != nil {
		r.Logger.Error("Failed to subscribe to quote activity", zap.Error(err))
		return nil, err
	}

	r.Logger.Info("Started quote activity subscription",
		zap.String("user", username),
		zap.String("note", noteID))

	return ch, nil
}

// getConnectionID extracts WebSocket connection ID from context
func (r *subscriptionResolver) getConnectionID(ctx context.Context) string {
	// This would be set by the WebSocket handler
	// For now, return empty string - callers should set this in context
	if connID, ok := ctx.Value(contextKeyConnectionID).(string); ok {
		return connID
	}
	return ""
}
