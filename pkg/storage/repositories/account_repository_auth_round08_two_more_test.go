package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap/zaptest"
)

func TestRound08_AccountRepositoryAuth_MinIntElseBranch(t *testing.T) {
	require.Equal(t, 1, minInt(2, 1))
}

func TestRound08_AccountRepositoryAuth_IsRateLimited_QueryError(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("First", mock.Anything).Return(errors.New("boom")).Once()
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	_, _, err := repo.IsRateLimited(ctx, "k")
	require.Error(t, err)
}
