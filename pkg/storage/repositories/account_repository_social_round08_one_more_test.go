package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestRound08_AccountRepository_GetBlocks_Error(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]models.Block")).Return(errors.New("all failed")).Once()
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	_, err := repo.GetBlocks(ctx, "alice")
	require.Error(t, err)
}
