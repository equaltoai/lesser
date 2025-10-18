package graph

import (
	"context"

	"github.com/equaltoai/lesser/graph/model"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// ConversationUpdates is the resolver for the conversationUpdates field.
func (r *subscriptionResolver) ConversationUpdates(ctx context.Context) (<-chan *model.Conversation, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	r.Logger.Info("Conversation updates subscription started",
		zap.String("user", username))

	// Use SubscriptionManager for consistent subscription handling
	sm := r.SubscriptionManager
	if sm == nil {
		r.Logger.Error("subscription manager not available for conversation updates")
		ch := make(chan *model.Conversation)
		close(ch)
		return ch, ErrSubscriptionManagerNotRunning
	}

	if !sm.IsRunning() {
		r.Logger.Error("subscription manager not running for conversation updates")
		ch := make(chan *model.Conversation)
		close(ch)
		return ch, ErrSubscriptionManagerNotRunning
	}

	conversationChan, err := sm.SubscribeToConversation(ctx, username)
	if err != nil {
		r.Logger.Error("failed to create conversation subscription",
			zap.String("user", username),
			zap.Error(err))
		return nil, err
	}

	r.Logger.Info("started conversation subscription",
		zap.String("user", username))

	return conversationChan, nil
}
