package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func TestMediaAnalyticsRepository_Round08_UpdateAndRecordView(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// UpdateMediaAnalytics: first call fails, second succeeds.
	mockQuery.On("Update", mock.Anything).Return(errors.New("update failed")).Once()
	mockQuery.On("Update", mock.Anything).Return(nil).Once()

	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC))

	repo := NewMediaAnalyticsRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetCachingService(nil)
	repo.SetEventService(nil)

	analytics := &models.MediaAnalytics{}
	analytics.SetGeneralEvent("media_view", "m1", "u1")
	analytics.Format = "hls"

	require.Error(t, repo.UpdateMediaAnalytics(ctx, analytics))
	require.NoError(t, repo.UpdateMediaAnalytics(ctx, analytics))

	// RecordMediaView creates a record and writes it.
	require.NoError(t, repo.RecordMediaView(ctx, "m1", "u1", 5*time.Second, "720p"))
}

func TestMediaAnalyticsRepository_Round08_InternalPreferenceHelpers(t *testing.T) {
	require.Equal(t, "", findMostFrequent(map[string]int{}))
	require.Equal(t, "a", findMostFrequent(map[string]int{"a": 2, "b": 1}))

	repo := &MediaAnalyticsRepository{}
	prefs := repo.analyzeUserPreferences([]*models.MediaAnalytics{
		{Quality: "720p", VariantCosts: map[string]models.MediaVariantCost{"v1": {Codec: "h264", Resolution: "720p"}}},
		{Quality: "720p", VariantCosts: map[string]models.MediaVariantCost{"v2": {Codec: "h264", Resolution: "1080p"}}},
		{Quality: "1080p", VariantCosts: map[string]models.MediaVariantCost{"v3": {Codec: "h265", Resolution: "720p"}}},
	})

	require.Equal(t, "720p", prefs["preferred_quality"])
	require.Equal(t, "h264", prefs["preferred_codec"])
	require.Equal(t, 3, prefs["total_views"])
}

func TestMediaAnalyticsRepository_Round08_GetMediaAnalyticsByVariant_Success(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.MediaAnalytics)
		*dest = []*models.MediaAnalytics{
			{MediaID: "m1"},
			{MediaID: "m2"},
		}
	}).Return(nil).Once()

	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC))

	repo := NewMediaAnalyticsRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetCachingService(nil)
	repo.SetEventService(nil)

	items, err := repo.GetMediaAnalyticsByVariant(ctx, "v1")
	require.NoError(t, err)
	require.Len(t, items, 2)
}
