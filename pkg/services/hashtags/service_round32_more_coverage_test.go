package hashtags

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestService_GetHashtag_ViewerStateFallbacksOnNotFound(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := new(mockHashtagRepository)
	service := NewService(repo, nil, nil, nil, zap.NewNop())

	repo.On("GetHashtagInfo", mock.Anything, "golang").Return(&storage.Hashtag{Name: "golang"}, nil).Once()
	repo.On("GetHashtagStats", mock.Anything, "golang").Return((*storage.HashtagStats)(nil), nil).Once()
	repo.On("IsFollowingHashtag", mock.Anything, "alice", "golang").Return(false, storage.ErrNotFound).Once()
	repo.On("GetHashtagNotificationSettings", mock.Anything, "alice", "golang").Return((*storage.HashtagNotificationSettings)(nil), storage.ErrNotFound).Once()
	repo.On("IsHashtagMuted", mock.Anything, "alice", "golang").Return(false, storage.ErrNotFound).Once()
	repo.On("GetHashtagTimelineAdvanced", mock.Anything, "golang", (*string)(nil), defaultRelatedHashtagSampleSize, "").Return([]*storage.StatusSearchResult{}, nil).Once()

	out, err := service.GetHashtag(ctx, "golang", "alice")
	require.NoError(t, err)
	require.NotNil(t, out)
	require.False(t, out.IsFollowing)
	require.False(t, out.IsMuted)
	require.Nil(t, out.NotificationSettings)
}

func TestService_GetHashtag_ViewerStateFallbacksOnUnexpectedErrors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := new(mockHashtagRepository)
	service := NewService(repo, nil, nil, nil, zap.NewNop())

	repo.On("GetHashtagInfo", mock.Anything, "golang").Return(&storage.Hashtag{Name: "golang"}, nil).Once()
	repo.On("GetHashtagStats", mock.Anything, "golang").Return((*storage.HashtagStats)(nil), nil).Once()
	repo.On("IsFollowingHashtag", mock.Anything, "alice", "golang").Return(false, errors.New("boom")).Once()
	repo.On("GetHashtagNotificationSettings", mock.Anything, "alice", "golang").Return((*storage.HashtagNotificationSettings)(nil), errors.New("boom")).Once()
	repo.On("IsHashtagMuted", mock.Anything, "alice", "golang").Return(false, errors.New("boom")).Once()
	repo.On("GetHashtagTimelineAdvanced", mock.Anything, "golang", (*string)(nil), defaultRelatedHashtagSampleSize, "").Return([]*storage.StatusSearchResult{}, nil).Once()

	out, err := service.GetHashtag(ctx, "golang", "alice")
	require.NoError(t, err)
	require.NotNil(t, out)
	require.False(t, out.IsFollowing)
	require.False(t, out.IsMuted)
	require.Nil(t, out.NotificationSettings)
}

func TestService_GetHashtagActivity_HandlesEmptyAndCanceledContext(t *testing.T) {
	t.Parallel()

	service := NewService(nil, nil, nil, nil, zap.NewNop())

	_, err := service.GetHashtagActivity(context.Background(), []string{"", "   ", "#"})
	require.ErrorIs(t, err, ErrHashtagNameRequired)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = service.GetHashtagActivity(ctx, []string{"golang"})
	require.ErrorIs(t, err, context.Canceled)
}

func TestService_publishInternalHashtagEvent_HandlesPublishError(t *testing.T) {
	t.Parallel()

	publisher := new(mockPublisher)
	service := NewService(nil, nil, nil, publisher, zap.NewNop())

	publisher.On(
		"PublishToStream",
		mock.Anything,
		streaming.HashtagStreamName("golang"),
		mock.MatchedBy(func(event *streaming.Event) bool {
			return event != nil &&
				event.Type == string(streaming.EventTypeHashtagUpdate) &&
				event.Stream == streaming.HashtagStreamName("golang") &&
				event.Payload["action"] == string(streaming.ActionFollow)
		}),
	).Return(errors.New("publish failed")).Once()

	service.publishInternalHashtagEvent(streaming.ActionFollow, "golang")

	publisher.AssertExpectations(t)
}
