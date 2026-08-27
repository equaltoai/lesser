package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestMediaAnalyticsRepository_Round08_RecordMediaAnalytics_CreateError(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("Create").Return(errors.New("create failed")).Once()
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC))

	repo := NewMediaAnalyticsRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetCachingService(nil)
	repo.SetEventService(nil)

	analytics := &models.MediaAnalytics{}
	analytics.SetGeneralEvent("media_view", "m1", "u1")
	analytics.Format = "hls"

	require.Error(t, repo.RecordMediaAnalytics(ctx, analytics))
}

func TestMediaAnalyticsRepository_Round08_RecordMediaAnalytics_MissingKeysBranch(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC))

	repo := NewMediaAnalyticsRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetCachingService(nil)
	repo.SetEventService(nil)

	// PK/SK missing triggers the internal UpdateKeys attempt; Create will fail due to UpdateKeys validation.
	analytics := &models.MediaAnalytics{MediaID: "m1", Date: "2025-12-27"}
	require.Error(t, repo.RecordMediaAnalytics(ctx, analytics))
}

func TestMediaAnalyticsRepository_Round08_TimeRangeQuery_ScanErrorPaths(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)

	mockQuery.On("All", mock.Anything).Return(errors.New("scan failed")).Once()

	repo := NewMediaAnalyticsRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetCachingService(nil)
	repo.SetEventService(nil)

	start := time.Date(2025, 12, 27, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 12, 27, 23, 59, 59, 0, time.UTC)

	_, err := repo.GetMediaAnalyticsByTimeRange(ctx, "m1", start, end, 10)
	require.Error(t, err)

	mockQuery.On("All", mock.Anything).Return(errors.New("scan failed")).Once()
	_, err = repo.GetAllMediaAnalyticsByTimeRange(ctx, start, end, 10)
	require.Error(t, err)

	mockQuery.On("All", mock.Anything).Return(errors.New("scan failed")).Once()
	_, err = repo.GetBandwidthByTimeRange(ctx, start, end, 10)
	require.Error(t, err)
}

func TestMediaAnalyticsRepository_Round08_GetMediaMetricsForDate_VariantPaths(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	date := time.Date(2025, 12, 27, 0, 0, 0, 0, time.UTC).Format(common.DateFormat)
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.MediaAnalytics)
		*dest = []*models.MediaAnalytics{
			{
				MediaID:             "m1",
				EventType:           "media_view",
				StreamingSessions:   2,
				TotalBandwidthBytes: 3,
				QualityDistribution: map[string]int{"720p": 1},
			},
			{
				MediaID:             "m1",
				EventType:           "other",
				StreamingSessions:   1,
				TotalBandwidthBytes: 2,
				QualityDistribution: map[string]int{"720p": 2},
			},
		}
	}).Return(nil).Once()

	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC))

	repo := NewMediaAnalyticsRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetCachingService(nil)
	repo.SetEventService(nil)

	metrics, err := repo.GetMediaMetricsForDate(ctx, "m1", date)
	require.NoError(t, err)
	require.Equal(t, int64(1), metrics["total_views"])
	require.Equal(t, int64(3), metrics["streaming_sessions"])
	require.Equal(t, int64(5), metrics["total_bandwidth"])
}
