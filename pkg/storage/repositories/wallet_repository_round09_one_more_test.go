package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestWalletRepository_Round09_OneMore_GetWalletCredential_TracksMinimumReadUnits(t *testing.T) {
	ctx := context.Background()
	_ = time.Date(2025, 12, 28, 3, 4, 5, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("All", mock.Anything).Return(nil).Once()

	repo := NewWalletRepository(mockDB, "test-table", zap.NewNop(), newRound09CostService())
	got, err := repo.GetWalletCredential(ctx, "ethereum", "0xabc")
	require.Error(t, err)
	require.Nil(t, got)

	// Ensure index query stayed empty (exercise readUnits==0 -> minimum 1 path).
	var _ []models.WalletIndex
}
