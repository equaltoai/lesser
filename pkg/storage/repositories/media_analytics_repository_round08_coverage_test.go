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
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestMediaAnalyticsRepository_Round08_RecordAndFetchAndSummaries(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	pk := "MEDIA_ANALYTICS#hls"
	sk := "0#m1"

	// First() sequence for GetMediaAnalyticsByID:
	// - Not found
	// - Success
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.MediaAnalytics)
		*dest = models.MediaAnalytics{PK: pk, SK: sk, MediaID: "m1", Format: "hls"}
	}).Return(nil).Once()

	// Custom Scan: return deterministic analytics for any Scan call in this test.
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		switch dest := args.Get(0).(type) {
		case *[]*models.MediaAnalytics:
			eventTime := time.Date(2025, 12, 27, 10, 0, 0, 0, time.UTC)
			*dest = []*models.MediaAnalytics{
				{
					MediaID:             "m1",
					Format:              "hls",
					Date:                eventTime.Format(common.DateFormat),
					Timestamp:           eventTime,
					EventType:           "media_view",
					StreamingSessions:   2,
					TotalBandwidthBytes: 2 * 1024 * 1024 * 1024,
					QualityDistribution: map[string]int{"720p": 2},
					VariantCosts: map[string]models.MediaVariantCost{
						"v1": {Codec: "h264", Resolution: "720p", TotalCost: 10, DeliveryCount: 2, BandwidthBytes: 100, ViewerMinutes: 5},
						"v2": {Codec: "h265", Resolution: "1080p", TotalCost: 20, DeliveryCount: 3, BandwidthBytes: 200, ViewerMinutes: 7},
					},
					TotalVariantCost: 30,
				},
				{
					MediaID:             "m2",
					Format:              "hls",
					Date:                eventTime.Format(common.DateFormat),
					Timestamp:           eventTime,
					EventType:           "quality_changed",
					StreamingSessions:   1,
					TotalBandwidthBytes: 0,
					VariantCosts:        map[string]models.MediaVariantCost{},
					TotalVariantCost:    0,
				},
			}
		case *[]models.MediaAnalytics:
			*dest = nil
		}
	}).Return(nil)

	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC))

	repo := NewMediaAnalyticsRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetCachingService(nil)
	repo.SetEventService(nil)

	analytics := &models.MediaAnalytics{}
	analytics.SetGeneralEvent("media_view", "m1", "u1")
	analytics.Format = "hls"
	require.NoError(t, repo.RecordMediaAnalytics(ctx, analytics))

	// Not found branch.
	_, err := repo.GetMediaAnalyticsByID(ctx, "hls", time.Unix(0, 0).UTC(), "m1")
	require.Error(t, err)

	// Success branch.
	got, err := repo.GetMediaAnalyticsByID(ctx, "hls", time.Unix(0, 0).UTC(), "m1")
	require.NoError(t, err)
	require.Equal(t, "m1", got.MediaID)

	summary, err := repo.GetDailyCostSummary(ctx, time.Date(2025, 12, 27, 0, 0, 0, 0, time.UTC).Format(common.DateFormat))
	require.NoError(t, err)
	require.Equal(t, int64(30), summary["total_cost"].(int64))

	top, err := repo.GetTopVariantsByDemand(ctx, time.Date(2025, 12, 27, 0, 0, 0, 0, time.UTC).Format(common.DateFormat), 1)
	require.NoError(t, err)
	require.Len(t, top, 1)
}

func TestMediaAnalyticsRepository_Round08_ReportsRecommendationsCleanupAndRanges(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// Custom Scan for old record cleanup and time range queries.
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		switch dest := args.Get(0).(type) {
		case *[]*models.MediaAnalytics:
			eventTime := time.Date(2025, 12, 27, 10, 0, 0, 0, time.UTC)
			*dest = []*models.MediaAnalytics{
				{PK: "PK1", SK: "SK1", MediaID: "m1", Date: eventTime.Format(common.DateFormat), Timestamp: eventTime, TotalBandwidthBytes: 10},
				{PK: "PK2", SK: "SK2", MediaID: "m2", Date: eventTime.Format(common.DateFormat), Timestamp: eventTime.Add(5 * time.Minute), TotalBandwidthBytes: 0},
			}
		}
	}).Return(nil).Maybe()

	// CleanupOldAnalytics: Scan returns records; delete failures are swallowed.
	mockQuery.On("Delete").Return(errors.New("delete failed")).Once()

	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC))

	repo := NewMediaAnalyticsRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetCachingService(nil)
	repo.SetEventService(nil)

	// GenerateAnalyticsReport invalid dates.
	_, err := repo.GenerateAnalyticsReport(ctx, "not-a-date", "2025-12-28")
	require.Error(t, err)
	_, err = repo.GenerateAnalyticsReport(ctx, "2025-12-28", "bad")
	require.Error(t, err)

	// GenerateAnalyticsReport success.
	report, err := repo.GenerateAnalyticsReport(ctx, "2025-12-27", "2025-12-27")
	require.NoError(t, err)
	require.Equal(t, "2025-12-27", report["start_date"])

	// TrackUserBehavior delegates to RecordMediaAnalytics.
	data := map[string]interface{}{
		"media_id":          "m1",
		"preferred_quality": "720p",
		"session_duration":  float64(12.34),
	}
	require.NoError(t, repo.TrackUserBehavior(ctx, "u1", data))

	recs, err := repo.GetContentRecommendations(ctx, "u1", 3)
	require.NoError(t, err)
	require.Len(t, recs, 3)

	require.NoError(t, repo.CleanupOldAnalytics(ctx, 24*time.Hour))

	// Range queries.
	startTime := time.Date(2025, 12, 27, 0, 0, 0, 0, time.UTC)
	endTime := time.Date(2025, 12, 27, 23, 59, 59, 0, time.UTC)
	_, err = repo.GetMediaAnalyticsByTimeRange(ctx, "", startTime, endTime, 10)
	require.Error(t, err)

	mediaRange, err := repo.GetMediaAnalyticsByTimeRange(ctx, "m1", startTime, endTime, 10)
	require.NoError(t, err)
	require.NotEmpty(t, mediaRange)

	allRange, err := repo.GetAllMediaAnalyticsByTimeRange(ctx, startTime, endTime, 10)
	require.NoError(t, err)
	require.NotEmpty(t, allRange)

	bw, err := repo.GetBandwidthByTimeRange(ctx, startTime, endTime, 10)
	require.NoError(t, err)
	require.NotEmpty(t, bw)

	// Deprecated paths.
	empty, err := repo.GetPopularMedia(ctx, startTime, endTime, 10, nil)
	require.NoError(t, err)
	require.Empty(t, empty)

	metrics, err := repo.CalculatePopularityMetrics(ctx, "m1", 0)
	require.NoError(t, err)
	require.Equal(t, "m1", metrics["media_id"])

	require.NoError(t, repo.StoreMediaAnalytics(ctx, &models.MediaAnalytics{PK: "PK", SK: "SK", MediaID: "m1", Date: "2025-12-27"}))
}

func TestMediaAnalyticsRepository_Round08_GetByDateAndVariant_QueryErrors(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	queryErr := errors.New("scan failed")
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(queryErr).Twice()

	repo := NewMediaAnalyticsRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetCachingService(nil)
	repo.SetEventService(nil)

	_, err := repo.GetMediaAnalyticsByDate(ctx, "2025-12-27")
	require.Error(t, err)

	_, err = repo.GetMediaAnalyticsByVariant(ctx, "v1")
	require.Error(t, err)
}
