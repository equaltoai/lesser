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

// ConversationUpdates is the resolver for the conversationUpdates field.
func (r *subscriptionResolver) ConversationUpdates(ctx context.Context) (<-chan *model.Conversation, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Create channel for streaming
	ch := make(chan *model.Conversation, 100)

	r.Logger.Info("Conversation updates subscription started",
		zap.String("user", username))

	// Get internal EventBus for real-time conversation updates
	internalEventBus := streaming.GetGlobalEventBus(r.Logger)
	if internalEventBus == nil || !internalEventBus.IsRunning() {
		r.Logger.Error("Internal EventBus not available for ConversationUpdates")
		close(ch)
		return ch, ErrInternalEventBusUnavailable
	}

	// Subscribe to conversation events for this user
	filter := &streaming.EventFilter{
		Types: []streaming.EventType{
			"conversation.update",
			"conversation.create",
		},
		Streams: []string{fmt.Sprintf("user:%s", username)},
		ActorID: username,
	}

	subscriber, err := internalEventBus.Subscribe(fmt.Sprintf("conv_%s_%d", username, time.Now().UnixNano()), filter, 100)
	if err != nil {
		r.Logger.Error("Failed to subscribe to event bus for ConversationUpdates", zap.Error(err))
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

				// Convert internal event to Conversation
				conversation := r.convertEventToConversation(ctx, event)
				if conversation != nil {
					select {
					case ch <- conversation:
					case <-ctx.Done():
						return
					default:
						// Drop event if channel is full
						r.Logger.Warn("Dropping conversation event - channel full", zap.String("event_id", event.ID))
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
