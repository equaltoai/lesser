package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestRound10_HashtagFollowHelpers_UpdateSetting(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	t.Run("empty hashtag rejects", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		err := updateHashtagFollowSetting(ctx, mockDB, logger, "user-1", "", HashtagFollowUpdateConfig{Operation: "mute"})
		require.Error(t, err)
	})

	t.Run("unknown operation rejects", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("First", mock.Anything).Return(nil)

		err := updateHashtagFollowSetting(ctx, mockDB, logger, "user-1", "#tag", HashtagFollowUpdateConfig{Operation: "wat"})
		require.Error(t, err)
	})

	t.Run("get error is returned", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("First", mock.Anything).Return(errors.New("boom"))

		err := updateHashtagFollowSetting(ctx, mockDB, logger, "user-1", "#tag", HashtagFollowUpdateConfig{Operation: "mute"})
		require.Error(t, err)
	})

	t.Run("create error is returned", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("First", mock.Anything).Return(nil)
		mockQuery.On("Create").Return(errors.New("boom"))

		err := updateHashtagFollowSetting(ctx, mockDB, logger, "user-1", "#tag", HashtagFollowUpdateConfig{Operation: "mute"})
		require.Error(t, err)
	})

	t.Run("notification updates notifications_enabled", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		var saved *models.HashtagFollow
		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Run(func(args mock.Arguments) {
			if hf, ok := args.Get(0).(*models.HashtagFollow); ok && hf.UserID != "" {
				saved = hf
			}
		}).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			hf := args.Get(0).(*models.HashtagFollow)
			hf.UpdateKeysWithParams("user-1", "tag")
			hf.NotificationsEnabled = false
			hf.UpdatedAt = time.Now().Add(-time.Minute)
		}).Return(nil)
		mockQuery.On("Create").Return(nil)

		enabled := true
		err := updateHashtagFollowSetting(ctx, mockDB, logger, "user-1", "#tag", HashtagFollowUpdateConfig{Operation: "notification", BoolValue: &enabled})
		require.NoError(t, err)
		require.NotNil(t, saved)
		require.True(t, saved.NotificationsEnabled)
		require.False(t, saved.UpdatedAt.IsZero())
	})

	t.Run("mute and unmute update muted flag", func(t *testing.T) {
		for _, tc := range []struct {
			name      string
			operation string
			wantMuted bool
		}{
			{name: "mute", operation: "mute", wantMuted: true},
			{name: "unmute", operation: "unmute", wantMuted: false},
		} {
			t.Run(tc.name, func(t *testing.T) {
				mockDB := new(mocks.MockDB)
				mockQuery := new(mocks.MockQuery)

				var saved *models.HashtagFollow
				mockDB.On("WithContext", mock.Anything).Return(mockDB)
				mockDB.On("Model", mock.Anything).Run(func(args mock.Arguments) {
					if hf, ok := args.Get(0).(*models.HashtagFollow); ok && hf.UserID != "" {
						saved = hf
					}
				}).Return(mockQuery)
				mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
				mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
					hf := args.Get(0).(*models.HashtagFollow)
					hf.UpdateKeysWithParams("user-1", "tag")
					hf.Muted = true
				}).Return(nil)
				mockQuery.On("Create").Return(nil)

				err := updateHashtagFollowSetting(ctx, mockDB, logger, "user-1", "#tag", HashtagFollowUpdateConfig{Operation: tc.operation})
				require.NoError(t, err)
				require.NotNil(t, saved)
				require.Equal(t, tc.wantMuted, saved.Muted)
			})
		}
	})
}
