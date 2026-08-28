package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap/zaptest"
)

func TestRound08_AccountRepository_RevokeAdvancedUserTokens_ErrorAggregate(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("AllPaginated", mock.AnythingOfType("*[]models.AuthRefreshToken")).Run(func(args mock.Arguments) {
		dst := args.Get(0).(*[]models.AuthRefreshToken)
		*dst = []models.AuthRefreshToken{
			{Token: "token-0001", UserID: "user-1", Family: "family-1", ExpiresAt: baseTime.Add(time.Hour).Unix()},
			{Token: "token-0002", UserID: "user-1", Family: "family-1", ExpiresAt: baseTime.Add(time.Hour).Unix()},
		}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()
	mockQuery.On("Update", mock.Anything).Return(errors.New("update failed")).Once()
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	require.Error(t, repo.RevokeAdvancedUserTokens(ctx, "user-1", "logout"))
}
