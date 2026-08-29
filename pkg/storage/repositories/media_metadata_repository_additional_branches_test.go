package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestMediaMetadataRepository_GetMediaMetadataByStatus_TracksCost(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	config := cost.DefaultTrackingServiceConfig()
	config.MetricsBatchSize = 1
	config.MetricsFlushInterval = time.Hour
	costService := cost.NewTrackingService(nil, zap.NewNop(), config)
	t.Cleanup(func() {
		_ = costService.Close(context.Background())
	})

	repo := NewMediaMetadataRepository(mockDB, "test-table", zap.NewNop(), costService)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaMetadata")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "STATUS#pending").Return(mockQuery)
	// limit=0 previously skipped Limit entirely (degenerate-input class, wave
	// #1469); the floor now always issues Limit(500).
	mockQuery.On("Limit", 500).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.MediaMetadata")).Run(func(args mock.Arguments) {
		records := args.Get(0).(*[]*models.MediaMetadata)
		*records = []*models.MediaMetadata{} // Empty result triggers estimatedRU==0 branch.
	}).Return(nil).Once()

	result, err := repo.GetMediaMetadataByStatus(ctx, "pending", 0)
	require.NoError(t, err)
	require.Empty(t, result)
}

func TestMediaMetadataRepository_CleanupExpiredMetadata_TracksCost(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	config := cost.DefaultTrackingServiceConfig()
	config.MetricsBatchSize = 1
	config.MetricsFlushInterval = time.Hour
	costService := cost.NewTrackingService(nil, zap.NewNop(), config)
	t.Cleanup(func() {
		_ = costService.Close(context.Background())
	})

	repo := NewMediaMetadataRepository(mockDB, "test-table", zap.NewNop(), costService)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.MediaMetadata")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "STATUS#failed").Return(mockQuery)
	mockQuery.On("Where", "gsi1SK", "<", mock.AnythingOfType("string")).Return(mockQuery)
	mockQuery.On("Limit", 100).Return(mockQuery)

	mockQuery.On("All", mock.AnythingOfType("*[]*models.MediaMetadata")).Run(func(args mock.Arguments) {
		records := args.Get(0).(*[]*models.MediaMetadata)
		*records = []*models.MediaMetadata{{MediaID: "expired-1", Status: "failed"}}
	}).Return(nil).Once()

	mockQuery.On("Where", "PK", "=", "MEDIA#expired-1").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", "METADATA").Return(mockQuery)
	mockQuery.On("Delete").Return(nil).Once()

	require.NoError(t, repo.CleanupExpiredMetadata(ctx))
}
