package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap/zaptest"
)

func TestRound08_RateLimitRepository_GetAPIRateLimitInfo_ClampRemaining(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.APIRateLimit")).Run(func(args mock.Arguments) {
		rl := args.Get(0).(*models.APIRateLimit)
		rl.Count = 999
		rl.Window = time.Now().Truncate(time.Minute)
	}).Return(nil).Once()
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewRateLimitRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
	remaining, _, err := repo.GetAPIRateLimitInfo(ctx, "user-1", "endpoint", 10, time.Minute)
	require.NoError(t, err)
	require.Equal(t, 0, remaining)
}

func TestRound08_RateLimitRepository_ClearLoginAttempts_EmptyAttempts(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]*models.LoginAttempt")).Run(func(args mock.Arguments) {
		dst := args.Get(0).(*[]*models.LoginAttempt)
		*dst = nil
	}).Return(nil).Once()
	mockQuery.On("Delete").Return(errors.New("lockout delete failed")).Once()
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewRateLimitRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
	require.NoError(t, repo.ClearLoginAttempts(ctx, "user-1"))
}
