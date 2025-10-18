package graph

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/streaming"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// QuoteActivity implements SubscriptionResolver
func (r *subscriptionResolver) QuoteActivity(ctx context.Context, noteID string) (<-chan *model.QuoteActivityUpdate, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	activityChan := make(chan *model.QuoteActivityUpdate, 100)

	// Get internal EventBus for real-time quote activity
	internalEventBus := streaming.GetGlobalEventBus(r.Logger)
	if internalEventBus == nil || !internalEventBus.IsRunning() {
		r.Logger.Error("Internal EventBus not available for QuoteActivity")
		close(activityChan)
		return activityChan, ErrInternalEventBusUnavailable
	}

	// Subscribe to status events for the specific note
	filter := &streaming.EventFilter{
		Types: []streaming.EventType{
			streaming.EventTypeStatus,
			streaming.EventTypeStatusUpdate,
			streaming.EventTypeStatusReblog,
			streaming.EventTypeNotification,
		},
		Streams: []string{
			fmt.Sprintf("note:%s", noteID),
			fmt.Sprintf("quotes:%s", noteID),
		},
		UserID: username,
	}

	subscriber, err := internalEventBus.Subscribe(
		fmt.Sprintf("quotes_%s_%s_%d", noteID, username, time.Now().UnixNano()),
		filter, 100)
	if err != nil {
		r.Logger.Error("Failed to subscribe to quote activity", zap.Error(err))
		close(activityChan)
		return activityChan, errors.Join(errors.New("failed to subscribe"), err)
	}

	// Start forwarding events
	go func() {
		defer func() {
			close(activityChan)
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

				// Convert event to QuoteActivityUpdate
				update := r.convertEventToQuoteActivity(event, noteID)
				if update != nil {
					select {
					case activityChan <- update:
					case <-ctx.Done():
						return
					}
				}

			case <-ctx.Done():
				return
			}
		}
	}()

	r.Logger.Info("Started quote activity subscription",
		zap.String("user", username),
		zap.String("note", noteID))

	return activityChan, nil
}
