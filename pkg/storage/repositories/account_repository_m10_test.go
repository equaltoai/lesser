package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
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

func TestAccountRepository_SearchAccounts_OneCharacterQueryFansOutHandlePrefixes(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 7, 8, 9, 10, 11, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	var currentPrefix string
	queriedPrefixes := make(map[string]int)
	mockQuery.On("Where", "gsi5PK", "=", mock.AnythingOfType("string")).Run(func(args mock.Arguments) {
		currentPrefix = args.String(2)
		queriedPrefixes[currentPrefix]++
	}).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.User)
		switch currentPrefix {
		case "USER_HANDLE_PREFIX#a":
			*dest = []models.User{m10SearchUser("a", baseTime)}
		case "USER_HANDLE_PREFIX#al":
			*dest = []models.User{m10SearchUser("alice", baseTime.Add(time.Minute))}
		case "USER_HANDLE_PREFIX#an":
			*dest = []models.User{m10SearchUser("ann", baseTime.Add(2*time.Minute))}
		default:
			*dest = []models.User{}
		}
	}).Return(nil)
	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	firstPage, err := repo.SearchAccounts(ctx, "a", interfaces.PaginationOptions{Limit: 2})
	require.NoError(t, err)
	require.True(t, firstPage.HasMore)
	require.NotEmpty(t, firstPage.NextCursor)
	require.Len(t, firstPage.Items, 2)
	require.Equal(t, "a", firstPage.Items[0].User.Username)
	require.Equal(t, "alice", firstPage.Items[1].User.Username)

	secondPage, err := repo.SearchAccounts(ctx, "a", interfaces.PaginationOptions{Limit: 2, Cursor: firstPage.NextCursor})
	require.NoError(t, err)
	require.False(t, secondPage.HasMore)
	require.Empty(t, secondPage.NextCursor)
	require.Len(t, secondPage.Items, 1)
	require.Equal(t, "ann", secondPage.Items[0].User.Username)

	require.GreaterOrEqual(t, queriedPrefixes["USER_HANDLE_PREFIX#a"], 1)
	require.GreaterOrEqual(t, queriedPrefixes["USER_HANDLE_PREFIX#al"], 1)
	require.GreaterOrEqual(t, queriedPrefixes["USER_HANDLE_PREFIX#an"], 1)
}

func m10SearchUser(username string, createdAt time.Time) models.User {
	user := models.User{
		Username:  username,
		Role:      "user",
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
		Approved:  true,
	}
	_ = user.UpdateKeys()
	return user
}
