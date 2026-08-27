package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestMediaRepository_Sweep_ExportedMethods(t *testing.T) {
	ctx := context.Background()
	userID := "alice1234"

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewMediaRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Create").Return(nil).Maybe()
	mockQuery.On("Update", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Delete").Return(nil).Maybe()

	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		switch dest := args.Get(0).(type) {
		case *models.MediaJob:
			dest.JobID = "job-1"
			dest.MediaID = "m1"
			dest.Username = "alice"
			dest.Status = "pending"
			dest.S3Key = "s3://bucket/key"
			dest.MimeType = "image/png"
			dest.CreatedAt = time.Now().Add(-time.Hour)
			dest.UpdatedAt = time.Now().Add(-time.Hour)
		case *models.Media:
			dest.MediaID = "m1"
			dest.UserID = userID
			dest.ContentType = "image/png"
			dest.FileSize = 1
			dest.Status = models.StatusPending
			dest.Version = "original"
			dest.PK = "media#m1"
			dest.SK = "version#original"
			dest.CreatedAt = time.Now().Add(-time.Hour)
			dest.UpdatedAt = time.Now().Add(-time.Hour)
			dest.UploadedAt = time.Now().Add(-time.Hour)
			dest.GSI1SK = "t#m1"
			dest.GSI2SK = "t#m1"
			dest.Variants = map[string]models.MediaVariant{
				"thumb": {ContentType: "image/png", FileSize: 10},
			}
		case *models.UserMediaConfig:
			dest.UserID = userID
			dest.Username = "alice"
			dest.PlanTier = "free"
		case *models.MediaSpending:
			dest.UserID = userID
			dest.Period = "2025-12"
			dest.PeriodType = models.PeriodMonthly
		case *models.TranscodingJob:
			dest.JobID = "tj-1"
			dest.UserID = userID
			dest.MediaID = "m1"
			dest.JobType = "video"
			dest.Status = "completed"
			dest.StartedAt = time.Now().Add(-time.Hour)
			dest.TotalCostMicros = 10
			dest.CostBreakdown = map[string]int64{"svc": 10}
		}
	}).Return(nil).Maybe()

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		switch dest := args.Get(0).(type) {
		case *[]*models.MediaJob:
			*dest = []*models.MediaJob{{JobID: "job-1"}}
		case *[]*models.Media:
			olderThan := time.Now().Add(-time.Hour)
			usedAt := olderThan.Add(-time.Minute)
			*dest = []*models.Media{
				{MediaID: "m-unused", UserID: userID, ContentType: "image/png", UsageCount: 0},
				{MediaID: "m-old", UserID: userID, ContentType: "image/png", UsageCount: 1, LastUsedAt: &usedAt},
			}
		case *[]*models.MediaSpending:
			*dest = []*models.MediaSpending{{UserID: userID, Period: "2025-12", PeriodType: models.PeriodMonthly}}
		case *[]*models.MediaSpendingTransaction:
			*dest = []*models.MediaSpendingTransaction{{UserID: userID, Category: models.ResourceProcessing}}
		case *[]*models.TranscodingJob:
			*dest = []*models.TranscodingJob{
				{JobID: "tj-1", UserID: userID, MediaID: "m1", JobType: "video", Status: "completed", StartedAt: time.Now().Add(-time.Hour), TotalCostMicros: 10, CostBreakdown: map[string]int64{"svc": 10}},
			}
		}
	}).Return(nil).Maybe()

	mockQuery.On("Count").Return(int64(0), nil).Maybe()

	job := &models.MediaJob{JobID: "job-1", MediaID: "m1", Username: "alice", Status: "pending", S3Key: "s3://bucket/key", MimeType: "image/png"}
	require.NoError(t, repo.CreateMediaJob(ctx, job))
	_, err := repo.GetMediaJob(ctx, "job-1")
	require.NoError(t, err)
	require.NoError(t, repo.UpdateMediaJob(ctx, job))
	require.NoError(t, repo.DeleteMediaJob(ctx, "job-1"))
	require.NoError(t, repo.DeleteMedia(ctx, "m1"))

	media := &models.Media{MediaID: "m1", UserID: userID, ContentType: "image/png", FileSize: 1, Status: models.StatusPending}
	require.NoError(t, repo.CreateMedia(ctx, media))
	_, err = repo.GetMedia(ctx, "m1")
	require.NoError(t, err)
	require.NoError(t, repo.UpdateMedia(ctx, media))

	_, err = repo.GetJobsByStatus(ctx, "pending", 10)
	require.NoError(t, err)
	_, err = repo.GetJobsByUser(ctx, "alice", 10)
	require.NoError(t, err)
	_, err = repo.GetMediaByUser(ctx, userID, 10)
	require.NoError(t, err)
	_, err = repo.GetMediaByStatus(ctx, "pending", 10)
	require.NoError(t, err)
	_, err = repo.GetMediaByContentType(ctx, "image/png", 10)
	require.NoError(t, err)

	_, err = repo.GetUserMediaLegacy(ctx, "alice")
	require.NoError(t, err)

	require.NoError(t, repo.UpdateMediaAttachment(ctx, "m1", map[string]any{
		FieldDescription: " hello ",
		"focus":          "center",
		"sensitive":      true,
		"spoiler_text":   " spoiler ",
	}))
	require.NoError(t, repo.UnmarkAllMediaAsSensitive(ctx, "alice"))

	cfg := &models.UserMediaConfig{UserID: userID, Username: "alice", PlanTier: "free"}
	require.NoError(t, repo.CreateUserMediaConfig(ctx, cfg))
	_, err = repo.GetUserMediaConfig(ctx, userID)
	require.NoError(t, err)
	require.NoError(t, repo.UpdateUserMediaConfig(ctx, cfg))
	require.NoError(t, repo.DeleteUserMediaConfig(ctx, userID))

	spending := &models.MediaSpending{UserID: userID, Period: "2025-12", PeriodType: models.PeriodMonthly}
	require.NoError(t, repo.CreateMediaSpending(ctx, spending))
	_, err = repo.GetMediaSpending(ctx, userID, "2025-12")
	require.NoError(t, err)
	require.NoError(t, repo.UpdateMediaSpending(ctx, spending))
	_, err = repo.GetMediaSpendingByTimeRange(ctx, userID, models.PeriodMonthly, 10)
	require.NoError(t, err)

	txn := &models.MediaSpendingTransaction{UserID: userID, Category: models.ResourceProcessing, CostMicros: 10}
	require.NoError(t, repo.CreateMediaSpendingTransaction(ctx, txn))
	_, err = repo.GetMediaSpendingTransactions(ctx, userID, 10)
	require.NoError(t, err)
	require.NoError(t, repo.AddSpendingTransaction(ctx, txn))

	transcoding := &models.TranscodingJob{JobID: "tj-1", MediaID: "m1", UserID: userID, JobType: "video", Status: "completed", StartedAt: time.Now().Add(-time.Hour)}
	require.NoError(t, repo.CreateTranscodingJob(ctx, transcoding))
	_, err = repo.GetTranscodingJob(ctx, "tj-1")
	require.NoError(t, err)
	require.NoError(t, repo.UpdateTranscodingJob(ctx, transcoding))
	_, err = repo.GetTranscodingJobsByUser(ctx, userID, 10)
	require.NoError(t, err)
	_, err = repo.GetTranscodingJobsByMedia(ctx, "m1", 10)
	require.NoError(t, err)
	_, err = repo.GetTranscodingJobsByStatus(ctx, "completed", 1)
	require.NoError(t, err)
	require.NoError(t, repo.DeleteTranscodingJob(ctx, "tj-1"))
	_, err = repo.GetTranscodingCostsByUser(ctx, userID, "day")
	require.NoError(t, err)

	require.NoError(t, repo.MarkMediaProcessing(ctx, "m1"))
	require.NoError(t, repo.MarkMediaReady(ctx, "m1"))
	require.NoError(t, repo.MarkMediaFailed(ctx, "m1", "boom"))

	_, err = repo.GetPendingMedia(ctx, interfaces.PaginationOptions{Limit: 5})
	require.NoError(t, err)
	_, err = repo.GetProcessingMedia(ctx, interfaces.PaginationOptions{Limit: 5})
	require.NoError(t, err)

	require.NoError(t, repo.AddMediaVariant(ctx, "m1", "thumb", models.MediaVariant{ContentType: "image/png", FileSize: 10}))
	_, err = repo.GetMediaVariant(ctx, "m1", "thumb")
	require.NoError(t, err)
	require.NoError(t, repo.DeleteMediaVariant(ctx, "m1", "thumb"))

	_, err = repo.GetUserMedia(ctx, "alice", interfaces.PaginationOptions{Limit: 5})
	require.NoError(t, err)
	_, err = repo.GetUserMediaByType(ctx, "alice", "image/png", interfaces.PaginationOptions{Limit: 5})
	require.NoError(t, err)
	_, err = repo.GetUnusedMedia(ctx, time.Now().Add(-time.Hour), interfaces.PaginationOptions{Limit: 10, Cursor: "1"})
	require.NoError(t, err)

	require.NoError(t, repo.MarkMediaUsed(ctx, "m1"))
	_, _, err = repo.GetMediaUsageStats(ctx, "m1")
	require.NoError(t, err)
	require.NoError(t, repo.SetMediaModeration(ctx, "m1", true, 0.7, []string{"label"}))

	_, err = repo.GetModerationPendingMedia(ctx, interfaces.PaginationOptions{Limit: 5, Cursor: time.Now().Add(-time.Hour).Format(time.RFC3339)})
	require.NoError(t, err)

	_, err = repo.GetMediaByIDs(ctx, []string{"m1"})
	require.NoError(t, err)
	_, err = repo.DeleteExpiredMedia(ctx, time.Now())
	require.NoError(t, err)
	_, err = repo.GetMediaStorageUsage(ctx, userID)
	require.NoError(t, err)
	_, err = repo.GetTotalStorageUsage(ctx)
	require.NoError(t, err)

	require.NoError(t, repo.TrackRead(ctx, "read", 1))
	require.NoError(t, repo.TrackWrite(ctx, "write", 1))
	require.NoError(t, repo.TrackQuery(ctx, "gsi1", 1, 1))
}

func TestMediaRepository_TargetedBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("GetOrCreateMediaSpending creates on not found and wraps other errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewMediaRepository(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()

		mockQuery.On("First", mock.AnythingOfType("*models.MediaSpending")).Return(dynamormerrors.ErrItemNotFound).Once()
		mockQuery.On("Create").Return(nil).Once()

		spending, err := repo.GetOrCreateMediaSpending(ctx, "alice", "2025-12", models.PeriodMonthly)
		require.NoError(t, err)
		require.NotNil(t, spending)

		mockQuery.On("First", mock.AnythingOfType("*models.MediaSpending")).Return(errors.New("boom")).Once()
		_, err = repo.GetOrCreateMediaSpending(ctx, "alice", "2025-12", models.PeriodMonthly)
		require.Error(t, err)
	})

	t.Run("GetUserMediaConfigByUsername uses dependency when present", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewMediaRepository(mockDB, "test-table", zap.NewNop(), nil)

		repo.SetDependencies(map[string]interface{}{
			"user": &testUserIDResolver{userID: "alice"},
		})

		mockDB.On("WithContext", ctx).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("First", mock.AnythingOfType("*models.UserMediaConfig")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.UserMediaConfig)
			dest.UserID = "alice"
			dest.Username = "alice"
			dest.PlanTier = "free"
		}).Return(nil).Once()

		_, err := repo.GetUserMediaConfigByUsername(ctx, "alice")
		require.NoError(t, err)
	})
}

type testUserIDResolver struct {
	userID string
	err    error
}

func (t *testUserIDResolver) GetUserIDByUsername(_ context.Context, _ string) (string, error) {
	return t.userID, t.err
}

func TestMediaRepository_MoreBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("isWithinTimeRange covers cases", func(t *testing.T) {
		repo := NewMediaRepository(new(mocks.MockDB), "test-table", zap.NewNop(), nil)

		now := time.Now()
		assert.True(t, repo.isWithinTimeRange(now, "day"))
		assert.False(t, repo.isWithinTimeRange(now.Add(-48*time.Hour), "day"))
		assert.True(t, repo.isWithinTimeRange(now, "week"))
		assert.True(t, repo.isWithinTimeRange(now, "month"))
		assert.True(t, repo.isWithinTimeRange(now, "year"))
		assert.True(t, repo.isWithinTimeRange(now, ""))
		assert.True(t, repo.isWithinTimeRange(now, "unknown"))
	})

	t.Run("parseCursor and encodeCursor", func(t *testing.T) {
		repo := NewMediaRepository(new(mocks.MockDB), "test-table", zap.NewNop(), nil)
		assert.Equal(t, 0, repo.parseCursor(""))
		assert.Equal(t, 0, repo.parseCursor("1"))
		assert.Equal(t, "2", repo.encodeCursor(2))
	})

	t.Run("pagination helpers set cursor fields", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewMediaRepository(mockDB, "test-table", zap.NewNop(), nil)

		since := time.Now().Add(-24 * time.Hour)
		until := time.Now()

		mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()

		mockQuery.On("All", mock.AnythingOfType("*[]*models.Media")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.Media)
			*dest = []*models.Media{
				{MediaID: "m1", GSI2SK: "c1"},
				{MediaID: "m2", GSI2SK: "c2"},
			}
		}).Return(nil).Once()

		page, err := repo.GetPendingMedia(ctx, interfaces.PaginationOptions{
			Limit:  2,
			Since:  &since,
			Until:  &until,
			Cursor: "c0",
		})
		require.NoError(t, err)
		require.True(t, page.HasMore)
		require.Equal(t, "c2", page.NextCursor)

		mockQuery.On("All", mock.AnythingOfType("*[]*models.Media")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.Media)
			*dest = []*models.Media{
				{MediaID: "m1", GSI1SK: "u1"},
				{MediaID: "m2", GSI1SK: "u2"},
			}
		}).Return(nil).Once()

		page2, err := repo.GetUserMediaByType(ctx, "alice", "image/png", interfaces.PaginationOptions{
			Limit:  2,
			Since:  &since,
			Until:  &until,
			Cursor: "u0",
		})
		require.NoError(t, err)
		require.True(t, page2.HasMore)
		require.Equal(t, "u2", page2.NextCursor)
	})

	t.Run("GetModerationPendingMedia uses createdAt cursor", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewMediaRepository(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()

		t1 := time.Now().Add(-time.Hour)
		t2 := time.Now().Add(-time.Minute)
		mockQuery.On("All", mock.AnythingOfType("*[]*models.Media")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.Media)
			*dest = []*models.Media{
				{MediaID: "m1", CreatedAt: t1},
				{MediaID: "m2", CreatedAt: t2},
			}
		}).Return(nil).Once()

		page, err := repo.GetModerationPendingMedia(ctx, interfaces.PaginationOptions{Limit: 2})
		require.NoError(t, err)
		require.True(t, page.HasMore)
		require.Equal(t, t2.Format(time.RFC3339), page.NextCursor)
	})

	t.Run("GetMediaByIDs empty and skips not found", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewMediaRepository(mockDB, "test-table", zap.NewNop(), nil)

		media, err := repo.GetMediaByIDs(ctx, nil)
		require.NoError(t, err)
		require.Empty(t, media)

		mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()

		mockQuery.On("First", mock.AnythingOfType("*models.Media")).Return(dynamormerrors.ErrItemNotFound).Once()
		mockQuery.On("First", mock.AnythingOfType("*models.Media")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.Media)
			dest.MediaID = "m2"
			dest.UserID = "alice1234"
			dest.FileSize = 1
			dest.ContentType = "image/png"
			dest.Status = models.StatusPending
		}).Return(nil).Once()

		items, err := repo.GetMediaByIDs(ctx, []string{"m1", "m2"})
		require.NoError(t, err)
		require.Len(t, items, 1)
		require.Equal(t, "m2", items[0].MediaID)
	})

	t.Run("DeleteExpiredMedia continues on delete errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		repo := NewMediaRepository(mockDB, "test-table", zap.NewNop(), nil)

		deleted, err := repo.DeleteExpiredMedia(ctx, time.Now())
		require.NoError(t, err)
		require.EqualValues(t, 0, deleted)
	})

	t.Run("GetUserMediaConfigByUsername fallback and dependency error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewMediaRepository(mockDB, "test-table", zap.NewNop(), nil)

		repo.SetDependencies(map[string]interface{}{
			"user": &testUserIDResolver{err: errors.New("resolver failed")},
		})
		_, err := repo.GetUserMediaConfigByUsername(ctx, "alice")
		require.Error(t, err)

		repo.SetDependencies(map[string]interface{}{})

		mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()

		mockQuery.On("First", mock.AnythingOfType("*models.UserMediaConfig")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.UserMediaConfig)
			dest.UserID = "alice1234"
			dest.Username = "alice"
			dest.PlanTier = "free"
		}).Return(nil).Once()

		_, err = repo.GetUserMediaConfigByUsername(ctx, "alice")
		require.NoError(t, err)

		mockQuery.On("First", mock.AnythingOfType("*models.UserMediaConfig")).Return(dynamormerrors.ErrItemNotFound).Once()
		_, err = repo.GetUserMediaConfigByUsername(ctx, "alice")
		require.Error(t, err)

		mockQuery.On("First", mock.AnythingOfType("*models.UserMediaConfig")).Return(ErrTestMockError).Once()
		_, err = repo.GetUserMediaConfigByUsername(ctx, "alice")
		require.Error(t, err)
	})

	t.Run("SetCostService", func(t *testing.T) {
		repo := NewMediaRepository(new(mocks.MockDB), "test-table", zap.NewNop(), nil)
		repo.SetCostService(nil)
	})
}

func TestMediaRepository_ErrorBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("isNotFoundError handles dynamorm", func(t *testing.T) {
		repo := NewMediaRepository(new(mocks.MockDB), "test-table", zap.NewNop(), nil)
		assert.False(t, repo.isNotFoundError(nil))
		assert.True(t, repo.isNotFoundError(dynamormerrors.ErrItemNotFound))
	})

	t.Run("CreateMediaJob validations and DB error", func(t *testing.T) {
		repo := NewMediaRepository(new(mocks.MockDB), "test-table", zap.NewNop(), nil)
		require.Error(t, repo.CreateMediaJob(ctx, &models.MediaJob{}))

		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo = NewMediaRepository(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB).Once()
		mockDB.On("Model", mock.AnythingOfType("*models.MediaJob")).Return(mockQuery).Once()
		mockQuery.On("Create").Return(ErrTestMockError).Once()

		job := &models.MediaJob{
			JobID:    "job-1",
			MediaID:  "m1",
			Username: "alice",
			S3Key:    "s3://bucket/key",
			MimeType: "image/png",
			Status:   "pending",
		}
		require.Error(t, repo.CreateMediaJob(ctx, job))

		// BeforeCreate failure (missing required job fields) should return an error before DB write.
		job2 := &models.MediaJob{JobID: "job-2", MediaID: "m2", Username: "alice"}
		require.Error(t, repo.CreateMediaJob(ctx, job2))
	})

	t.Run("GetMediaJob invalid and not found", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewMediaRepository(mockDB, "test-table", zap.NewNop(), nil)

		_, err := repo.GetMediaJob(ctx, "")
		require.Error(t, err)

		mockDB.On("WithContext", ctx).Return(mockDB).Once()
		mockDB.On("Model", mock.AnythingOfType("*models.MediaJob")).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Twice()
		mockQuery.On("First", mock.AnythingOfType("*models.MediaJob")).Return(dynamormerrors.ErrItemNotFound).Once()

		_, err = repo.GetMediaJob(ctx, "job-1")
		require.Error(t, err)
	})

	t.Run("UpdateMediaJob BeforeUpdate and DB error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewMediaRepository(mockDB, "test-table", zap.NewNop(), nil)

		require.Error(t, repo.UpdateMediaJob(ctx, &models.MediaJob{JobID: "job-1"}))

		mockDB.On("WithContext", ctx).Return(mockDB).Once()
		mockDB.On("Model", mock.AnythingOfType("*models.MediaJob")).Return(mockQuery).Once()
		mockQuery.On("Update", mock.Anything).Return(ErrTestMockError).Once()

		job := &models.MediaJob{
			JobID:     "job-1",
			MediaID:   "m1",
			Username:  "alice",
			S3Key:     "s3://bucket/key",
			MimeType:  "image/png",
			Status:    "pending",
			CreatedAt: time.Now().Add(-time.Minute),
			UpdatedAt: time.Now().Add(-time.Minute),
		}
		require.Error(t, repo.UpdateMediaJob(ctx, job))
	})

	t.Run("GetJobsByStatus validations and scan error", func(t *testing.T) {
		repo := NewMediaRepository(new(mocks.MockDB), "test-table", zap.NewNop(), nil)
		_, err := repo.GetJobsByStatus(ctx, "", 10)
		require.Error(t, err)
		_, err = repo.GetJobsByStatus(ctx, "pending", 5000)
		require.Error(t, err)

		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo = NewMediaRepository(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB).Once()
		mockDB.On("Model", mock.AnythingOfType("*models.MediaJob")).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("All", mock.AnythingOfType("*[]*models.MediaJob")).Return(ErrTestMockError).Once()

		_, err = repo.GetJobsByStatus(ctx, "pending", 10)
		require.Error(t, err)
	})

	t.Run("CreateMedia BeforeCreate error", func(t *testing.T) {
		repo := NewMediaRepository(new(mocks.MockDB), "test-table", zap.NewNop(), nil)
		require.Error(t, repo.CreateMedia(ctx, &models.Media{MediaID: "m1", UserID: "u1", ContentType: "image/png", FileSize: 0, Status: models.StatusPending}))
	})

	t.Run("GetUserMediaLegacy GetMediaByUser error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewMediaRepository(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB).Once()
		mockDB.On("Model", mock.AnythingOfType("*models.Media")).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("All", mock.AnythingOfType("*[]*models.Media")).Return(ErrTestMockError).Once()

		_, err := repo.GetUserMediaLegacy(ctx, "alice")
		require.Error(t, err)
	})

	t.Run("UnmarkAllMediaAsSensitive continues on update errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewMediaRepository(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()

		mockQuery.On("All", mock.AnythingOfType("*[]*models.Media")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]*models.Media)
			*dest = []*models.Media{
				{MediaID: "m1", UserID: "alice1234", ContentType: "image/png", FileSize: 1, Status: models.StatusPending},
				{MediaID: "m2", UserID: "alice1234", ContentType: "image/png", FileSize: 1, Status: models.StatusPending},
			}
		}).Return(nil).Once()

		mockQuery.On("Update", mock.Anything).Return(ErrTestMockError).Once()
		mockQuery.On("Update", mock.Anything).Return(nil).Once()

		require.NoError(t, repo.UnmarkAllMediaAsSensitive(ctx, "alice1234"))
	})

	t.Run("GetMediaVariant and DeleteMediaVariant error branches", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewMediaRepository(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
		mockDB.On("Model", mock.AnythingOfType("*models.Media")).Return(mockQuery).Maybe()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()

		mockQuery.On("First", mock.AnythingOfType("*models.Media")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.Media)
			dest.MediaID = "m1"
			dest.UserID = "alice1234"
			dest.ContentType = "image/png"
			dest.FileSize = 1
			dest.Status = models.StatusPending
			dest.Variants = nil
		}).Return(nil).Once()

		_, err := repo.GetMediaVariant(ctx, "m1", "thumb")
		require.Error(t, err)

		mockQuery.On("First", mock.AnythingOfType("*models.Media")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.Media)
			dest.MediaID = "m1"
			dest.UserID = "alice1234"
			dest.ContentType = "image/png"
			dest.FileSize = 1
			dest.Status = models.StatusPending
			dest.Variants = map[string]models.MediaVariant{}
		}).Return(nil).Once()

		require.Error(t, repo.DeleteMediaVariant(ctx, "m1", "thumb"))
	})

	t.Run("MarkMediaProcessing/Ready/Failed propagate get errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewMediaRepository(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
		mockDB.On("Model", mock.AnythingOfType("*models.Media")).Return(mockQuery).Maybe()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("First", mock.AnythingOfType("*models.Media")).Return(ErrTestMockError).Times(3)

		require.Error(t, repo.MarkMediaProcessing(ctx, "m1"))
		require.Error(t, repo.MarkMediaReady(ctx, "m1"))
		require.Error(t, repo.MarkMediaFailed(ctx, "m1", "boom"))
	})

	t.Run("GetMediaStorageUsage get media list error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewMediaRepository(mockDB, "test-table", zap.NewNop(), nil)

		mockDB.On("WithContext", ctx).Return(mockDB).Once()
		mockDB.On("Model", mock.AnythingOfType("*models.Media")).Return(mockQuery).Once()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Limit", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("All", mock.AnythingOfType("*[]*models.Media")).Return(ErrTestMockError).Once()

		_, err := repo.GetMediaStorageUsage(ctx, "alice1234")
		require.Error(t, err)
	})
}
