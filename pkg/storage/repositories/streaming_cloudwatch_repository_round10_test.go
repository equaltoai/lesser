package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestRound10_StreamingCloudWatchRepository_StubsAndCache(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Now().UTC()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewStreamingCloudWatchRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	_, err := repo.GetQualityBreakdown(ctx, "media-1")
	require.Error(t, err)

	_, err = repo.GetGeographicData(ctx, "media-1")
	require.Error(t, err)

	_, err = repo.GetConcurrentViewers(ctx, "media-1")
	require.Error(t, err)

	_, err = repo.GetPerformanceMetrics(ctx, "media-1")
	require.Error(t, err)

	all, err := repo.GetAllCachedMetrics(ctx, "media-1")
	require.NoError(t, err)
	require.NotNil(t, all)

	require.NoError(t, repo.CleanupExpiredMetrics(ctx))

	quality := map[string]models.QualityMetric{
		"720p": {Quality: "720p", ViewerCount: 1},
	}
	require.NoError(t, repo.CacheQualityBreakdown(ctx, "media-1", quality))

	require.NoError(t, repo.CacheGeographicData(ctx, "media-1", map[string]models.GeographicMetric{"us": {Region: "us", ViewerCount: 2}}))
	require.NoError(t, repo.CacheConcurrentViewers(ctx, "media-1", models.ConcurrentViewerMetrics{CurrentViewers: 3}))
	require.NoError(t, repo.CachePerformanceMetrics(ctx, "media-1", models.StreamingPerformanceMetrics{OverallLatencyMs: 12}))
}
