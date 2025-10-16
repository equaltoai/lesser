package graph

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/streaming"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// HashtagActivity implements SubscriptionResolver
func (r *subscriptionResolver) HashtagActivity(ctx context.Context, hashtags []string) (<-chan *model.HashtagActivityUpdate, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	activityChan := make(chan *model.HashtagActivityUpdate, 100)

	// Get internal EventBus for real-time hashtag activity
	internalEventBus := streaming.GetGlobalEventBus(r.Logger)
	if internalEventBus == nil || !internalEventBus.IsRunning() {
		r.Logger.Error("Internal EventBus not available for HashtagActivity")
		close(activityChan)
		return activityChan, ErrInternalEventBusUnavailable
	}

	// Build streams for each hashtag
	streams := []string{"hashtags:global"}
	for _, tag := range hashtags {
		streams = append(streams, fmt.Sprintf("hashtag:%s", strings.ToLower(tag)))
	}

	filter := &streaming.EventFilter{
		Types: []streaming.EventType{
			streaming.EventTypeHashtagTrend,
			streaming.EventTypeHashtagUpdate,
			streaming.EventTypeStatus, // For posts with hashtags
		},
		Streams: streams,
		UserID:  username,
	}

	subscriber, err := internalEventBus.Subscribe(
		fmt.Sprintf("hashtags_%s_%d", username, time.Now().UnixNano()),
		filter, 100)
	if err != nil {
		r.Logger.Error("Failed to subscribe to hashtag activity", zap.Error(err))
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

				// Convert event to HashtagActivityUpdate
				update := r.convertEventToHashtagActivity(event, hashtags)
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

	r.Logger.Info("Started hashtag activity subscription",
		zap.String("user", username),
		zap.Int("hashtag_count", len(hashtags)))

	return activityChan, nil
}
