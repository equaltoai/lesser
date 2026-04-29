package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap/zaptest"
)

func TestAccountRepository_GetAccountByEmail_ReturnsAccountAfterUserLookup(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 7, 8, 9, 10, 11, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		projection := args.Get(0).(*userEmailLookupProjection)
		projection.Table = "test-table"
		projection.PK = "USER#user-1"
		projection.SK = "METADATA"
		projection.GSI2PK = "EMAIL#user-1@example.com"
		projection.GSI2SK = "USER#user-1"
		projection.Username = "user-1"
		projection.Email = "user-1@example.com"
	}).Return(nil).Once()
	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	account, err := repo.GetAccountByEmail(ctx, " User-1@Example.com ")
	require.NoError(t, err)
	require.NotNil(t, account)
	require.NotNil(t, account.User)
	require.Equal(t, "user-1", account.User.Username)
}
