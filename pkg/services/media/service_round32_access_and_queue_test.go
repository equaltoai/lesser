package media

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestService_checkMediaAccess_CoversBranches(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("owner can always access", func(t *testing.T) {
		service := NewService(nil, nil, nil, nil, zap.NewNop(), "bucket", "cdn.example.com")
		err := service.checkMediaAccess(ctx, &models.Media{MediaID: "m1", UserID: "alice", Status: "processing"}, "alice")
		require.NoError(t, err)
	})

	t.Run("NSFW blocked for unauthenticated user", func(t *testing.T) {
		service := NewService(nil, nil, nil, nil, zap.NewNop(), "bucket", "cdn.example.com")
		err := service.checkMediaAccess(ctx, &models.Media{MediaID: "m2", UserID: "owner", Status: "ready", IsNSFW: true}, "")
		require.Error(t, err)
		require.True(t, IsNSFWBlocked(err))
	})

	t.Run("NSFW allowed with warning still permits access when ready", func(t *testing.T) {
		service := NewService(nil, &fakeAccountPrefsRepoRound32{
			prefs: map[string]map[string]interface{}{
				"viewer": {
					"allow_nsfw":           true,
					"require_nsfw_warning": true,
				},
			},
		}, nil, nil, zap.NewNop(), "bucket", "cdn.example.com")
		err := service.checkMediaAccess(ctx, &models.Media{MediaID: "m3", UserID: "owner", Status: "ready", IsNSFW: true}, "viewer")
		require.NoError(t, err)
	})

	t.Run("non-ready media is blocked for non-owner", func(t *testing.T) {
		service := NewService(nil, nil, nil, nil, zap.NewNop(), "bucket", "cdn.example.com")
		err := service.checkMediaAccess(ctx, &models.Media{MediaID: "m4", UserID: "owner", Status: "processing"}, "viewer")
		require.ErrorIs(t, err, ErrMediaNotReady)
	})
}

func TestService_queueMediaProcessing_CoversMediaTypesAndError(t *testing.T) {
	t.Parallel()

	service, _, jobQueue, _ := createTestService(t)
	ctx := context.Background()

	jobQueue.On("QueueMediaJob", ctx, mock.MatchedBy(func(msg JobMessage) bool {
		return msg.MediaID == "img-1" && msg.Username == "alice" && msg.JobID != "" && msg.Timestamp > 0
	})).Return(nil).Once()
	require.NoError(t, service.queueMediaProcessing(ctx, &models.Media{MediaID: "img-1", UserID: "alice", ContentType: "image/png"}))

	jobQueue.On("QueueMediaJob", ctx, mock.MatchedBy(func(msg JobMessage) bool {
		return msg.MediaID == "vid-1" && msg.Username == "alice" && msg.JobID != ""
	})).Return(nil).Once()
	require.NoError(t, service.queueMediaProcessing(ctx, &models.Media{MediaID: "vid-1", UserID: "alice", ContentType: "video/mp4"}))

	jobQueue.On("QueueMediaJob", ctx, mock.MatchedBy(func(msg JobMessage) bool {
		return msg.MediaID == "aud-1" && msg.Username == "alice" && msg.JobID != ""
	})).Return(nil).Once()
	require.NoError(t, service.queueMediaProcessing(ctx, &models.Media{MediaID: "aud-1", UserID: "alice", ContentType: "audio/ogg"}))

	jobQueue.On("QueueMediaJob", ctx, mock.Anything).Return(errors.New("boom")).Once()
	err := service.queueMediaProcessing(ctx, &models.Media{MediaID: "fail-1", UserID: "alice", ContentType: "image/png"})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrMediaProcessingQueueFailed)
}
