package graph

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/services/lists"
	"github.com/equaltoai/lesser/pkg/streaming"
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

	// Create channel for streaming
	ch := make(chan *model.ListUpdate, 100)

	r.Logger.Info("List updates subscription started",
		zap.String("user", username),
		zap.String("list", result.ID))

	// Get internal EventBus for real-time list updates
	internalEventBus := streaming.GetGlobalEventBus(r.Logger)
	if internalEventBus == nil || !internalEventBus.IsRunning() {
		r.Logger.Error("Internal EventBus not available for ListUpdates")
		close(ch)
		return ch, ErrInternalEventBusUnavailable
	}

	// Subscribe to list events for this specific list
	filter := &streaming.EventFilter{
		Types: []streaming.EventType{
			"list.update",
			"list.member.add",
			"list.member.remove",
		},
		Streams: []string{fmt.Sprintf("list:%s", listID)},
		UserID:  username,
	}

	subscriber, err := internalEventBus.Subscribe(fmt.Sprintf("list_%s_%s_%d", listID, username, time.Now().UnixNano()), filter, 100)
	if err != nil {
		r.Logger.Error("Failed to subscribe to event bus for ListUpdates", zap.Error(err))
		close(ch)
		return ch, errors.Join(errors.New("failed to subscribe to event bus"), err)
	}

	// Start forwarding events
	go func() {
		defer func() {
			close(ch)
			if subscriber != nil {
				subscriber.Close()
			}
		}()

		for {
			select {
			case event := <-subscriber.Channel:
				if event == nil {
					return
				}

				// Convert internal event to ListUpdate
				listUpdate := r.convertEventToListUpdate(ctx, event)
				if listUpdate != nil {
					select {
					case ch <- listUpdate:
					case <-ctx.Done():
						return
					default:
						// Drop event if channel is full
						r.Logger.Warn("Dropping list update event - channel full", zap.String("event_id", event.ID))
					}
				}

			case <-subscriber.Quit:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, nil
}
