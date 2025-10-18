package graph

import (
	"context"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/services/lists"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// ListUpdates is the resolver for the listUpdates field.
func (r *subscriptionResolver) ListUpdates(ctx context.Context, listID string) (<-chan *model.ListUpdate, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Verify list ownership
	result, err := r.Registry.Lists().GetList(ctx, &lists.GetListQuery{
		ViewerID: username,
		ListID:   listID,
	})
	if err != nil {
		return nil, ErrListNotFoundOrAccessDenied
	}

	r.Logger.Info("List updates subscription started",
		zap.String("user", username),
		zap.String("list", result.ID))

	// Use SubscriptionManager for consistent subscription handling
	sm := r.SubscriptionManager
	if sm == nil {
		r.Logger.Error("subscription manager not available for list updates")
		ch := make(chan *model.ListUpdate)
		close(ch)
		return ch, ErrSubscriptionManagerNotRunning
	}

	if !sm.IsRunning() {
		r.Logger.Error("subscription manager not running for list updates")
		ch := make(chan *model.ListUpdate)
		close(ch)
		return ch, ErrSubscriptionManagerNotRunning
	}

	listChan, err := sm.SubscribeToListActivity(ctx, username, listID)
	if err != nil {
		r.Logger.Error("failed to create list activity subscription",
			zap.String("user", username),
			zap.String("list", listID),
			zap.Error(err))
		return nil, err
	}

	r.Logger.Info("started list activity subscription",
		zap.String("user", username),
		zap.String("list", listID))

	return listChan, nil
}
