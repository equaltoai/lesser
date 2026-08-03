package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap/zaptest"
)

func TestRound08_AuthRepository_QueryWalletCredentialsError(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	cfg := cost.DefaultTrackingServiceConfig()
	cfg.MetricsFlushInterval = time.Hour
	cfg.MetricsBatchSize = 50
	costSvc := cost.NewTrackingService(nil, zaptest.NewLogger(t), cfg)
	t.Cleanup(func() { _ = costSvc.Close(ctx) })

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("All", mock.Anything).Return(errors.New("boom")).Once()
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAuthRepositoryWithCostTracking(mockDB, "test-table", zaptest.NewLogger(t), costSvc)
	_, _, err := repo.queryWalletCredentials(ctx, "USER#user-1", "WALLET#", 5, "")
	require.Error(t, err)
}

func TestRound08_AuthRepository_IsRecoverableIndexError_Recoverable(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewAuthRepository(mockDB, "test-table", zaptest.NewLogger(t))
	require.True(t, repo.isRecoverableIndexError(errors.New("timeout")))
}

func TestRound08_AuthRepository_IsRecoverableIndexError_NonRecoverable(t *testing.T) {
	mockDB := new(mocks.MockDB)
	repo := NewAuthRepository(mockDB, "test-table", zaptest.NewLogger(t))
	require.False(t, repo.isRecoverableIndexError(errors.New("conditional check failed")))
}
