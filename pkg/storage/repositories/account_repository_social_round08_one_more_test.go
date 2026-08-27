package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap/zaptest"
)

func TestRound08_AccountRepository_GetBlocks_Error(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	// GetBlocks now iterates the block partition via a bounded page walk
	// (wave #1469): the page read is AllPaginated, not a bare All.
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.Block")).Return(nil, errors.New("all failed")).Once()
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	_, err := repo.GetBlocks(ctx, "alice")
	require.Error(t, err)
}
