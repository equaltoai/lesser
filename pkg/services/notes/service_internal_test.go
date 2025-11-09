package notes

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/services/notifications"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestResolveConversationID(t *testing.T) {
	ctx := context.Background()

	t.Run("nil status", func(t *testing.T) {
		assert.Equal(t, "", resolveConversationID(ctx, nil, nil))
	})

	t.Run("existing conversation id", func(t *testing.T) {
		status := &models.Status{
			StatusID:       "status-1",
			ConversationID: "conversation-1",
		}
		assert.Equal(t, "conversation-1", resolveConversationID(ctx, status, nil))
	})

	t.Run("new top level post", func(t *testing.T) {
		status := &models.Status{
			StatusID: "status-2",
		}
		assert.Equal(t, "status-2", resolveConversationID(ctx, status, nil))
	})

	t.Run("reply inherits parent conversation", func(t *testing.T) {
		status := &models.Status{
			StatusID:    "child-status",
			InReplyToID: "parent-status",
		}
		fetcher := func(_ context.Context, _ string) (*models.Status, error) {
			return &models.Status{ConversationID: "parent-conversation"}, nil
		}
		assert.Equal(t, "parent-conversation", resolveConversationID(ctx, status, fetcher))
	})

	t.Run("reply falls back to reply target", func(t *testing.T) {
		status := &models.Status{
			StatusID:    "child-status",
			InReplyToID: "parent-status",
		}
		fetcher := func(_ context.Context, _ string) (*models.Status, error) {
			return nil, errors.New("not found")
		}
		assert.Equal(t, "parent-status", resolveConversationID(ctx, status, fetcher))
	})
}

type stubPublisher struct {
	userEvents   []streamingEventRecord
	streamEvents []streamingEventRecord
}

type streamingEventRecord struct {
	target string
	event  *streaming.Event
}

func (s *stubPublisher) PublishToUser(_ context.Context, userID string, event *streaming.Event) error {
	s.userEvents = append(s.userEvents, streamingEventRecord{target: userID, event: event})
	return nil
}

func (s *stubPublisher) PublishToStream(_ context.Context, streamName string, event *streaming.Event) error {
	s.streamEvents = append(s.streamEvents, streamingEventRecord{target: streamName, event: event})
	return nil
}

func (s *stubPublisher) PublishToConversation(_ context.Context, _ string, _ *streaming.Event) error {
	return nil
}

func (s *stubPublisher) Close() error { return nil }

type stubNotificationService struct {
	cmds []*notifications.CreateNotificationCommand
}

func (s *stubNotificationService) CreateNotification(_ context.Context, cmd *notifications.CreateNotificationCommand) (*notifications.NotificationResult, error) {
	s.cmds = append(s.cmds, cmd)
	return &notifications.NotificationResult{}, nil
}

type stubFederation struct {
	activities []*activitypub.Activity
}

func (s *stubFederation) QueueActivity(_ context.Context, activity *activitypub.Activity) error {
	s.activities = append(s.activities, activity)
	return nil
}

func TestEmitReblogEventsPublishesBoostedEvents(t *testing.T) {
	publisher := &stubPublisher{}
	service := &Service{
		publisher: publisher,
		logger:    zap.NewNop(),
	}

	status := &models.Status{
		StatusID:       "status-boost",
		AuthorUsername: "author",
		Visibility:     VisibilityPublic,
		ReblogCount:    3,
	}

	events := service.emitReblogEvents(context.Background(), status, "booster")
	require.Len(t, events, 2)
	require.Len(t, publisher.userEvents, 1)
	require.Len(t, publisher.streamEvents, 1)

	assert.Equal(t, streaming.StatusBoosted, publisher.userEvents[0].event.Type)
	assert.Equal(t, streaming.StatusBoosted, publisher.streamEvents[0].event.Type)

	payloadStatus, ok := publisher.userEvents[0].event.Payload["status"].(*models.Status)
	require.True(t, ok)
	assert.Equal(t, status, payloadStatus)
	assert.Equal(t, 3, payloadStatus.ReblogCount)
}

func TestNotifyBoostCreatesNotification(t *testing.T) {
	notifier := &stubNotificationService{}
	service := &Service{
		logger:        zap.NewNop(),
		notifications: notifier,
		domainName:    "example.com",
	}

	status := &models.Status{
		StatusID:       "status-123",
		AuthorUsername: "author",
	}

	service.notifyBoost(context.Background(), status, "booster")

	require.Len(t, notifier.cmds, 1)
	cmd := notifier.cmds[0]
	assert.Equal(t, "author", cmd.UserID)
	assert.Equal(t, "booster", cmd.ActorID)
	assert.Equal(t, "status-123", cmd.TargetID)
	assert.Equal(t, "reblog", cmd.Type)
}

func TestQueueAnnounceActivityEnqueuesFederation(t *testing.T) {
	fed := &stubFederation{}
	service := &Service{
		logger:     zap.NewNop(),
		federation: fed,
	}

	now := time.Now()
	announce := &storage.Announce{
		ID:        "announce-1",
		Actor:     "https://example.com/users/booster",
		Object:    "https://example.com/users/author/statuses/status-1",
		Published: now,
	}
	status := &models.Status{
		StatusID: "status-1",
		ToRecipients: []string{
			"https://example.com/users/author/followers",
		},
	}

	service.queueAnnounceActivity(context.Background(), status, announce)

	require.Len(t, fed.activities, 1)
	assert.Equal(t, activitypub.AnnounceType, fed.activities[0].Type)
	assert.Equal(t, announce.Actor, fed.activities[0].Actor)
	assert.Equal(t, announce.Object, fed.activities[0].Object)
}
