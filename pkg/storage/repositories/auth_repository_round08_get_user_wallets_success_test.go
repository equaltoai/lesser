package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap/zaptest"
)

func TestRound08_AuthRepository_GetUserWallets_Success(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	cfg := cost.DefaultTrackingServiceConfig()
	cfg.MetricsFlushInterval = time.Hour
	cfg.MetricsBatchSize = 50
	costSvc := cost.NewTrackingService(nil, zaptest.NewLogger(t), cfg)
	t.Cleanup(func() { _ = costSvc.Close(ctx) })

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAuthRepositoryWithCostTracking(mockDB, "test-table", zaptest.NewLogger(t), costSvc)
	wallets, err := repo.GetUserWallets(ctx, "user-1")
	require.NoError(t, err)
	require.NotEmpty(t, wallets)
}
