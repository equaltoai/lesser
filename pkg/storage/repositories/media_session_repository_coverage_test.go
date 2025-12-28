package repositories

import (
	"context"
	"testing"
	"time"

	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/types"
	dynamormerrors "github.com/pay-theory/dynamorm/pkg/errors"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestMediaSessionRepository_StartStreamingSession_ValidationAndCreate(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid userID", func(t *testing.T) {
		repo := NewMediaSessionRepository(new(mocks.MockDB), zap.NewNop(), nil)
		_, err := repo.StartStreamingSession(ctx, "", "m1", types.FormatHLS, types.Quality("1080p"))
		require.Error(t, err)
	})

	t.Run("invalid mediaID", func(t *testing.T) {
		repo := NewMediaSessionRepository(new(mocks.MockDB), zap.NewNop(), nil)
		_, err := repo.StartStreamingSession(ctx, "u1", "", types.FormatHLS, types.Quality("1080p"))
		require.Error(t, err)
	})

	t.Run("create error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewMediaSessionRepositoryWithCostTracking(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB).Once()
		mockDB.On("Model", mock.AnythingOfType("*models.MediaSession")).Return(mockQuery).Once()
		mockQuery.On("Create").Return(ErrTestMockError).Once()

		_, err := repo.StartStreamingSession(ctx, "u1", "m1", types.FormatHLS, types.Quality("1080p"))
		require.Error(t, err)
	})

	t.Run("success", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewMediaSessionRepositoryWithCostTracking(mockDB, "test-table", zap.NewNop(), nil)
		repo.SetSessionTTL(5 * time.Minute)

		mockDB.On("WithContext", ctx).Return(mockDB).Once()
		mockDB.On("Model", mock.AnythingOfType("*models.MediaSession")).Return(mockQuery).Once()
		mockQuery.On("Create").Return(nil).Once()

		session, err := repo.StartStreamingSession(ctx, "u1", "m1", types.FormatHLS, types.Quality("1080p"))
		require.NoError(t, err)
		require.NotNil(t, session)
		assert.NotEmpty(t, session.SessionID)
		assert.Equal(t, "u1", session.UserID)
		assert.Equal(t, "m1", session.MediaID)
	})
}

func TestMediaSessionRepository_UpdateStreamingMetrics_Branches(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid sessionID", func(t *testing.T) {
		repo := NewMediaSessionRepository(new(mocks.MockDB), zap.NewNop(), nil)
		require.Error(t, repo.UpdateStreamingMetrics(ctx, "", 1, 10, 0.5, types.Quality("720p")))
	})

	t.Run("get error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewMediaSessionRepositoryWithCostTracking(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB).Once()
		mockDB.On("Model", mock.AnythingOfType("*models.MediaSession")).Return(mockQuery).Once()
		mockQuery.On("Where", "PK", "=", "SESSION#s1").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "METADATA").Return(mockQuery).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.MediaSession")).Return(ErrTestMockError).Once()

		err := repo.UpdateStreamingMetrics(ctx, "s1", 1, 10, 0.5, types.Quality("720p"))
		require.Error(t, err)
	})

	t.Run("inactive session", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewMediaSessionRepositoryWithCostTracking(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB).Once()
		mockDB.On("Model", mock.AnythingOfType("*models.MediaSession")).Return(mockQuery).Once()
		mockQuery.On("Where", "PK", "=", "SESSION#s1").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "METADATA").Return(mockQuery).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.MediaSession")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.MediaSession)
			dest.SessionID = "s1"
			dest.UserID = "u1"
			dest.MediaID = "m1"
			dest.StartTime = time.Now().Add(-time.Minute)
			dest.Active = false
		}).Return(nil).Once()

		err := repo.UpdateStreamingMetrics(ctx, "s1", 1, 10, 0.5, types.Quality("720p"))
		require.Error(t, err)
	})

	t.Run("update error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewMediaSessionRepositoryWithCostTracking(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB).Twice()
		mockDB.On("Model", mock.AnythingOfType("*models.MediaSession")).Return(mockQuery).Twice()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)

		mockQuery.On("First", mock.AnythingOfType("*models.MediaSession")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.MediaSession)
			dest.SessionID = "s1"
			dest.UserID = "u1"
			dest.MediaID = "m1"
			dest.StartTime = time.Now().Add(-time.Minute)
			dest.Active = true
		}).Return(nil).Once()

		mockQuery.On("Update", mock.Anything).Return(ErrTestMockError).Once()

		err := repo.UpdateStreamingMetrics(ctx, "s1", 1, 10, 0.5, types.Quality("720p"))
		require.Error(t, err)
	})

	t.Run("success", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewMediaSessionRepositoryWithCostTracking(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB).Twice()
		mockDB.On("Model", mock.AnythingOfType("*models.MediaSession")).Return(mockQuery).Twice()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)

		mockQuery.On("First", mock.AnythingOfType("*models.MediaSession")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.MediaSession)
			dest.SessionID = "s1"
			dest.UserID = "u1"
			dest.MediaID = "m1"
			dest.StartTime = time.Now().Add(-time.Minute)
			dest.Active = true
		}).Return(nil).Once()

		mockQuery.On("Update", mock.Anything).Return(nil).Once()

		err := repo.UpdateStreamingMetrics(ctx, "s1", 2, 20, 0.9, types.Quality("720p"))
		require.NoError(t, err)
	})
}

func TestMediaSessionRepository_EndStreamingSession_AndLegacyHelpers(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMediaSessionRepositoryWithCostTracking(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("First", mock.AnythingOfType("*models.MediaSession")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.MediaSession)
		dest.SessionID = "s1"
		dest.UserID = "u1"
		dest.MediaID = "m1"
		dest.StartTime = time.Now().Add(-2 * time.Minute)
		dest.Active = true
	}).Return(nil).Twice()

	mockQuery.On("Update", mock.Anything).Return(nil).Twice()

	require.NoError(t, repo.EndStreamingSession(ctx, "s1"))
	require.NoError(t, repo.EndSession(ctx, "s1"))
}

func TestMediaSessionRepository_QueryAndCleanupPaths(t *testing.T) {
	ctx := context.Background()

	t.Run("GetActiveStreams converts and skips nils", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewMediaSessionRepositoryWithCostTracking(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", "PK", "=", "").Return(mockQuery).Once()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("All", mock.AnythingOfType("*[]*models.MediaSession")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.MediaSession)
			*dest = []*models.MediaSession{
				nil,
				{
					SessionID: "s1",
					UserID:    "u1",
					MediaID:   "m1",
					Format:    string(types.FormatHLS),
					Active:    true,
					StartTime: time.Now().Add(-time.Minute),
				},
			}
		}).Return(nil).Once()

		streams, err := repo.GetActiveStreams(ctx, 5)
		require.NoError(t, err)
		require.Len(t, streams, 1)
	})

	t.Run("ValidateSessionAccess not found", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewMediaSessionRepositoryWithCostTracking(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB).Once()
		mockDB.On("Model", mock.AnythingOfType("*models.MediaSession")).Return(mockQuery).Once()
		mockQuery.On("Where", "PK", "=", "SESSION#s1").Return(mockQuery).Once()
		mockQuery.On("Where", "SK", "=", "METADATA").Return(mockQuery).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.MediaSession")).Return(dynamormerrors.ErrItemNotFound).Once()

		ok, err := repo.ValidateSessionAccess(ctx, "s1", "u1")
		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("GetSessionsByTimeRange filters endTime", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewMediaSessionRepositoryWithCostTracking(mockDB, "test-table", zap.NewNop(), nil)

		start := time.Now().Add(-2 * time.Hour)
		end := time.Now().Add(-time.Hour)

		mockDB.On("WithContext", ctx).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", "PK", "=", "").Return(mockQuery).Once()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("All", mock.AnythingOfType("*[]*models.MediaSession")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.MediaSession)
			*dest = []*models.MediaSession{
				{SessionID: "s1", UserID: "u1", MediaID: "m1", StartTime: start.Add(10 * time.Minute), Active: true},
				{SessionID: "s2", UserID: "u2", MediaID: "m2", StartTime: end.Add(10 * time.Minute), Active: true},
			}
		}).Return(nil).Once()

		sessions, err := repo.GetSessionsByTimeRange(ctx, start, end, 10)
		require.NoError(t, err)
		require.Len(t, sessions, 1)
		assert.Equal(t, "s1", sessions[0].SessionID)
	})

	t.Run("CleanupExpiredSessions not found is nil", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewMediaSessionRepositoryWithCostTracking(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", "PK", "=", "").Return(mockQuery).Once()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("All", mock.AnythingOfType("*[]*models.MediaSession")).Return(dynamormerrors.ErrItemNotFound).Once()

		require.NoError(t, repo.CleanupExpiredSessions(ctx, time.Hour))
	})

	t.Run("CleanupExpiredSessions deletes and continues on error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		deleteQuery := new(mocks.MockQuery)
		repo := NewMediaSessionRepositoryWithCostTracking(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", "PK", "=", "").Return(mockQuery).Once()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("All", mock.AnythingOfType("*[]*models.MediaSession")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.MediaSession)
			*dest = []*models.MediaSession{
				{PK: "SESSION#s1", SK: "METADATA", SessionID: "s1"},
				{PK: "SESSION#s2", SK: "METADATA", SessionID: "s2"},
			}
		}).Return(nil).Once()

		mockDB.On("Model", mock.Anything).Return(deleteQuery).Twice()
		deleteQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(deleteQuery).Times(4)
		deleteQuery.On("Delete").Return(ErrTestMockError).Once()
		deleteQuery.On("Delete").Return(nil).Once()

		require.NoError(t, repo.CleanupExpiredSessions(ctx, time.Hour))
	})
}

func TestMediaSessionRepository_ConversionAndQualityTracking(t *testing.T) {
	ctx := context.Background()

	t.Run("modelToStreamingSession covers timing branches", func(t *testing.T) {
		repo := NewMediaSessionRepository(new(mocks.MockDB), zap.NewNop(), nil)

		start := time.Now().Add(-time.Minute)
		end := time.Now()
		lastUpdate := time.Now().Add(-10 * time.Second)

		session1 := repo.modelToStreamingSession(&models.MediaSession{
			SessionID:  "s1",
			UserID:     "u1",
			MediaID:    "m1",
			Format:     string(types.FormatHLS),
			StartTime:  start,
			EndTime:    &end,
			LastUpdate: &lastUpdate,
			Active:     false,
		})
		assert.Equal(t, "session_ended", session1.Error)
		assert.Greater(t, session1.DurationWatched, int64(0))
		assert.Equal(t, lastUpdate, session1.LastActivityTime)

		session2 := repo.modelToStreamingSession(&models.MediaSession{
			SessionID: "s2",
			UserID:    "u2",
			MediaID:   "m2",
			Format:    string(types.FormatHLS),
			StartTime: start,
			Duration:  12,
			Active:    true,
		})
		assert.Equal(t, start, session2.LastActivityTime)
		assert.Equal(t, int64(12), session2.DurationWatched)
	})

	t.Run("trackQualityChange handles create error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewMediaSessionRepositoryWithCostTracking(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB).Once()
		mockDB.On("Model", mock.AnythingOfType("*models.QualityChange")).Return(mockQuery).Once()
		mockQuery.On("Create").Return(ErrTestMockError).Once()

		repo.trackQualityChange(ctx, &types.StreamingSession{SessionID: "s1", CurrentQuality: types.Quality("4k")})
	})
}

func TestMediaSessionRepository_LegacyCRUDAndQueries(t *testing.T) {
	ctx := context.Background()

	t.Run("CreateSession GetSession UpdateSession", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewMediaSessionRepositoryWithCostTracking(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()

		start := time.Now().Add(-time.Minute)

		// CreateSession (ValidateAndCreate -> BaseRepository.Create -> Create)
		mockQuery.On("Create").Return(nil).Twice()
		require.NoError(t, repo.CreateSession(ctx, &types.StreamingSession{
			SessionID:      "s1",
			UserID:         "u1",
			MediaID:        "m1",
			Format:         types.FormatHLS,
			CurrentQuality: types.Quality("720p"),
			StartTime:      start,
		}))

		// GetSession and UpdateSession both fetch the model.
		mockQuery.On("First", mock.AnythingOfType("*models.MediaSession")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.MediaSession)
			dest.SessionID = "s1"
			dest.UserID = "u1"
			dest.MediaID = "m1"
			dest.Format = string(types.FormatHLS)
			dest.CurrentQuality = "720p"
			dest.StartTime = start
			dest.Active = true
			dest.BufferHealth = 0.5
		}).Return(nil).Twice()

		mockQuery.On("Update", mock.Anything).Return(nil).Once()

		sess, err := repo.GetSession(ctx, "s1")
		require.NoError(t, err)
		require.Equal(t, "s1", sess.SessionID)

		require.NoError(t, repo.UpdateSession(ctx, &types.StreamingSession{
			SessionID:        "s1",
			CurrentQuality:   types.Quality("1080p"),
			LastSegmentIndex: 2,
			BytesTransferred: 123,
			BufferHealth:     0.9,
		}))
	})

	t.Run("GetUserSessions filters inactive", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewMediaSessionRepositoryWithCostTracking(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("All", mock.AnythingOfType("*[]*models.MediaSession")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.MediaSession)
			*dest = []*models.MediaSession{
				{SessionID: "s1", UserID: "u1", MediaID: "m1", Active: true, Format: string(types.FormatHLS), StartTime: time.Now().Add(-time.Minute)},
				{SessionID: "s2", UserID: "u1", MediaID: "m2", Active: false, Format: string(types.FormatHLS), StartTime: time.Now().Add(-time.Minute)},
				nil,
			}
		}).Return(nil).Once()

		sessions, err := repo.GetUserSessions(ctx, "u1")
		require.NoError(t, err)
		require.Len(t, sessions, 1)
		assert.Equal(t, "s1", sessions[0].SessionID)
	})

	t.Run("GetMediaSessions and GetActiveSessionsCount", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewMediaSessionRepositoryWithCostTracking(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Where", "PK", "=", "").Return(mockQuery).Maybe()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()

		mockQuery.On("All", mock.AnythingOfType("*[]*models.MediaSession")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.MediaSession)
			*dest = []*models.MediaSession{
				{SessionID: "s1", UserID: "u1", MediaID: "m1", Active: true, Format: string(types.FormatHLS), StartTime: time.Now().Add(-time.Minute)},
				nil,
			}
		}).Return(nil).Once()

		mockQuery.On("All", mock.AnythingOfType("*[]*models.MediaSession")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.MediaSession)
			*dest = []*models.MediaSession{
				{SessionID: "s1", UserID: "u1", MediaID: "m1", Active: true, Format: string(types.FormatHLS), StartTime: time.Now().Add(-time.Minute)},
			}
		}).Return(nil).Once()

		sessions, err := repo.GetMediaSessions(ctx, "m1", 10)
		require.NoError(t, err)
		require.Len(t, sessions, 1)

		count, err := repo.GetActiveSessionsCount(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("ValidateSessionAccess matches and mismatches", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewMediaSessionRepositoryWithCostTracking(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("First", mock.AnythingOfType("*models.MediaSession")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.MediaSession)
			dest.SessionID = "s1"
			dest.UserID = "u1"
			dest.Active = true
			dest.StartTime = time.Now().Add(-time.Minute)
		}).Return(nil).Twice()

		ok, err := repo.ValidateSessionAccess(ctx, "s1", "u1")
		require.NoError(t, err)
		assert.True(t, ok)

		ok, err = repo.ValidateSessionAccess(ctx, "s1", "u2")
		require.NoError(t, err)
		assert.False(t, ok)
	})
}

func TestMediaSessionRepository_ErrorBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("isNotFoundError handles dynamorm and app errors", func(t *testing.T) {
		repo := NewMediaSessionRepository(new(mocks.MockDB), zap.NewNop(), nil)
		assert.False(t, repo.isNotFoundError(nil))
		assert.True(t, repo.isNotFoundError(dynamormerrors.ErrItemNotFound))
		assert.True(t, repo.isNotFoundError(apperrors.ItemNotFoundWithID("session", "s1")))
		assert.False(t, repo.isNotFoundError(ErrTestMockError))
	})

	t.Run("GetSession and CreateSession surface DB errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewMediaSessionRepositoryWithCostTracking(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("First", mock.AnythingOfType("*models.MediaSession")).Return(ErrTestMockError).Once()

		_, err := repo.GetSession(ctx, "s1")
		require.Error(t, err)

		mockQuery.On("First", mock.AnythingOfType("*models.MediaSession")).Return(dynamormerrors.ErrItemNotFound).Once()
		_, err = repo.GetSession(ctx, "s2")
		require.Error(t, err)
		assert.True(t, apperrors.HasCode(err, apperrors.CodeNotFound))

		mockQuery.On("Create").Return(ErrTestMockError).Once()
		require.Error(t, repo.CreateSession(ctx, &types.StreamingSession{
			SessionID:      "s1",
			UserID:         "u1",
			MediaID:        "m1",
			Format:         types.FormatHLS,
			CurrentQuality: types.Quality("720p"),
			StartTime:      time.Now(),
		}))
	})

	t.Run("GetActiveStreams handles query error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewMediaSessionRepositoryWithCostTracking(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", "PK", "=", "").Return(mockQuery).Once()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("All", mock.AnythingOfType("*[]*models.MediaSession")).Return(ErrTestMockError).Once()

		_, err := repo.GetActiveStreams(ctx, 10)
		require.Error(t, err)
	})

	t.Run("EndStreamingSession handles update error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewMediaSessionRepositoryWithCostTracking(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("First", mock.AnythingOfType("*models.MediaSession")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.MediaSession)
			dest.SessionID = "s1"
			dest.UserID = "u1"
			dest.MediaID = "m1"
			dest.StartTime = time.Now().Add(-time.Minute)
			dest.Active = true
		}).Return(nil).Once()
		mockQuery.On("Update", mock.Anything).Return(ErrTestMockError).Once()

		require.Error(t, repo.EndStreamingSession(ctx, "s1"))
	})

	t.Run("ValidateSessionAccess handles get error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewMediaSessionRepositoryWithCostTracking(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("First", mock.AnythingOfType("*models.MediaSession")).Return(ErrTestMockError).Once()

		_, err := repo.ValidateSessionAccess(ctx, "s1", "u1")
		require.Error(t, err)
	})

	t.Run("GetUserSessions handles query error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewMediaSessionRepositoryWithCostTracking(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("All", mock.AnythingOfType("*[]*models.MediaSession")).Return(ErrTestMockError).Once()

		_, err := repo.GetUserSessions(ctx, "u1")
		require.Error(t, err)
	})

	t.Run("GetMediaSessions handles query error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewMediaSessionRepositoryWithCostTracking(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", "PK", "=", "").Return(mockQuery).Once()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("All", mock.AnythingOfType("*[]*models.MediaSession")).Return(ErrTestMockError).Once()

		_, err := repo.GetMediaSessions(ctx, "m1", 10)
		require.Error(t, err)
	})

	t.Run("GetSessionsByTimeRange handles query error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewMediaSessionRepositoryWithCostTracking(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", "PK", "=", "").Return(mockQuery).Once()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("All", mock.AnythingOfType("*[]*models.MediaSession")).Return(ErrTestMockError).Once()

		_, err := repo.GetSessionsByTimeRange(ctx, time.Now().Add(-time.Hour), time.Now(), 10)
		require.Error(t, err)
	})

	t.Run("ValidateSessionAccess invalid params", func(t *testing.T) {
		repo := NewMediaSessionRepository(new(mocks.MockDB), zap.NewNop(), nil)
		_, err := repo.ValidateSessionAccess(ctx, "", "u1")
		require.Error(t, err)
		_, err = repo.ValidateSessionAccess(ctx, "s1", "")
		require.Error(t, err)
	})

	t.Run("GetActiveSessionsCount handles query error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewMediaSessionRepositoryWithCostTracking(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", "PK", "=", "").Return(mockQuery).Once()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("All", mock.AnythingOfType("*[]*models.MediaSession")).Return(ErrTestMockError).Once()

		_, err := repo.GetActiveSessionsCount(ctx)
		require.Error(t, err)
	})

	t.Run("trackQualityChange success", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewMediaSessionRepositoryWithCostTracking(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB).Once()
		mockDB.On("Model", mock.AnythingOfType("*models.QualityChange")).Return(mockQuery).Once()
		mockQuery.On("Create").Return(nil).Once()

		repo.trackQualityChange(ctx, &types.StreamingSession{SessionID: "s1", CurrentQuality: types.Quality("4k")})
	})
}
