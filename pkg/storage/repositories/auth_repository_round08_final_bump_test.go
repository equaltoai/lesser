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
	"go.uber.org/zap/zaptest"
)

func TestRound08_AuthRepository_FinalBumps(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	cfg := cost.DefaultTrackingServiceConfig()
	cfg.MetricsFlushInterval = time.Hour
	cfg.MetricsBatchSize = 50
	costSvc := cost.NewTrackingService(nil, zaptest.NewLogger(t), cfg)
	t.Cleanup(func() { _ = costSvc.Close(ctx) })

	t.Run("queryWalletCredentials cursor branch and limit passthrough", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAuthRepositoryWithCostTracking(mockDB, "test-table", zaptest.NewLogger(t), costSvc)
		_, _, err := repo.queryWalletCredentials(ctx, "USER#user-1", "WALLET#", 5, "cursor")
		require.NoError(t, err)
		require.Equal(t, 5, clampWalletCredentialLimit(5))
	})

	t.Run("fallback GetWalletByAddress scan success", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.Anything).Return(nil).Once() // empty index => fallback scan
		mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
			out := args.Get(0).(*[]models.WalletCredential)
			*out = append(*out, models.WalletCredential{
				Username: "user-1",
				Address:  "0xabc",
				ChainID:  1,
				Type:     "ethereum",
				LinkedAt: baseTime,
				LastUsed: baseTime,
			})
		}).Return(nil).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAuthRepositoryWithCostTracking(mockDB, "test-table", zaptest.NewLogger(t), costSvc)
		cred, err := repo.GetWalletByAddress(ctx, "ethereum", "0xAbC")
		require.NoError(t, err)
		require.NotNil(t, cred)
	})
}
