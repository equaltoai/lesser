package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap/zaptest"
)

func TestRound08_AuthRepository_AdditionalBranches(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	cfg := cost.DefaultTrackingServiceConfig()
	cfg.MetricsFlushInterval = time.Hour
	cfg.MetricsBatchSize = 50
	costSvc := cost.NewTrackingService(nil, zaptest.NewLogger(t), cfg)
	t.Cleanup(func() { _ = costSvc.Close(ctx) })

	t.Run("GetWebAuthnChallenge non-notfound error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAuthRepositoryWithCostTracking(mockDB, "test-table", zaptest.NewLogger(t), costSvc)
		_, err := repo.GetWebAuthnChallenge(ctx, "c")
		require.Error(t, err)
	})

	t.Run("GetWalletChallenge expired cleanup", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			ch := args.Get(0).(*models.WalletChallenge)
			ch.ID = "wc"
			ch.Username = "user-1"
			ch.Address = "0xabc"
			ch.ChainID = 1
			ch.Nonce = "n"
			ch.Message = "m"
			ch.IssuedAt = baseTime.Add(-time.Hour)
			ch.ExpiresAt = baseTime.Add(-time.Minute)
			_ = ch.UpdateKeys()
		}).Return(nil).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAuthRepositoryWithCostTracking(mockDB, "test-table", zaptest.NewLogger(t), costSvc)
		challenge, err := repo.GetWalletChallenge(ctx, "wc")
		require.NoError(t, err)
		require.Nil(t, challenge)
	})

	t.Run("DeleteWalletChallenge delete error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Delete", mock.Anything).Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAuthRepositoryWithCostTracking(mockDB, "test-table", zaptest.NewLogger(t), costSvc)
		require.Error(t, repo.DeleteWalletChallenge(ctx, "wc"))
	})

	t.Run("StoreWalletChallenge create error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Create").Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAuthRepositoryWithCostTracking(mockDB, "test-table", zaptest.NewLogger(t), costSvc)
		require.Error(t, repo.StoreWalletChallenge(ctx, &storage.WalletChallenge{
			ID:        "wc",
			Username:  "user-1",
			Address:   "0xabc",
			ChainID:   1,
			Nonce:     "n",
			Message:   "m",
			IssuedAt:  baseTime,
			ExpiresAt: baseTime.Add(time.Minute),
		}))
	})

	t.Run("DeleteWebAuthnChallenge delete error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Delete", mock.Anything).Return(errors.New("boom")).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewAuthRepositoryWithCostTracking(mockDB, "test-table", zaptest.NewLogger(t), costSvc)
		require.Error(t, repo.DeleteWebAuthnChallenge(ctx, "c"))
	})

	t.Run("GetWalletByAddress index query error triggers fallback", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.Anything).Return(errors.New("index down")).Once()
		mockQuery.On("Scan", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewAuthRepositoryWithCostTracking(mockDB, "test-table", zaptest.NewLogger(t), costSvc)
		cred, err := repo.GetWalletByAddress(ctx, "ethereum", "0xabc")
		require.NoError(t, err)
		require.Nil(t, cred)
	})
}
