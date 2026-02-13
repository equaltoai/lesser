package repositories

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	dynamock "github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap/zaptest"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

func TestAccountRepository_ListOAuthClients_FirstPage(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamock.MockDB)
	mockQuery := new(dynamock.MockQuery)
	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

	limit := 2

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
	mockDB.On("Model", mock.MatchedBy(func(model any) bool {
		_, ok := model.(*models.OAuthClient)
		return ok
	})).Return(mockQuery).Once()
	mockQuery.On("Index", oauthClientsIndexName).Return(mockQuery).Once()
	mockQuery.On("Where", oauthClientsPartitionAttr, "=", oauthClientsIndexPK).Return(mockQuery).Once()
	mockQuery.On("OrderBy", oauthClientsSortAttr, "ASC").Return(mockQuery).Once()
	mockQuery.On("Limit", limit+1).Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.OAuthClient)
		newest := buildOAuthClientModel(t, "client-newest", time.Date(2024, 3, 10, 15, 0, 0, 0, time.UTC))
		mid := buildOAuthClientModel(t, "client-mid", time.Date(2024, 3, 9, 15, 0, 0, 0, time.UTC))
		oldest := buildOAuthClientModel(t, "client-oldest", time.Date(2024, 3, 8, 15, 0, 0, 0, time.UTC))
		*dest = []*models.OAuthClient{newest, mid, oldest}
	}).Return(nil).Once()

	clients, nextCursor, err := repo.ListOAuthClients(ctx, limit, "")
	require.NoError(t, err)
	require.Len(t, clients, limit)
	assert.Equal(t, "client-newest", clients[0].ClientID)
	assert.Equal(t, "client-mid", clients[1].ClientID)
	assert.NotEmpty(t, nextCursor)

	pk, sk, err := Utils.Pagination.DecodeCursor(nextCursor)
	require.NoError(t, err)
	assert.Equal(t, oauthClientsIndexPK, pk)
	assert.Equal(t, buildOAuthClientModel(t, "client-mid", time.Date(2024, 3, 9, 15, 0, 0, 0, time.UTC)).OAuthClientsSK, sk)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestAccountRepository_ListOAuthClients_WithCursor(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamock.MockDB)
	mockQuery := new(dynamock.MockQuery)
	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

	startModel := buildOAuthClientModel(t, "client-mid", time.Date(2024, 3, 9, 15, 0, 0, 0, time.UTC))
	cursor := Utils.Pagination.EncodeCursor(oauthClientsIndexPK, startModel.OAuthClientsSK)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
	mockDB.On("Model", mock.MatchedBy(func(model any) bool {
		_, ok := model.(*models.OAuthClient)
		return ok
	})).Return(mockQuery).Once()
	mockQuery.On("Index", oauthClientsIndexName).Return(mockQuery).Once()
	mockQuery.On("Where", oauthClientsPartitionAttr, "=", oauthClientsIndexPK).Return(mockQuery).Once()
	mockQuery.On("OrderBy", oauthClientsSortAttr, "ASC").Return(mockQuery).Once()
	mockQuery.On("Limit", oauthClientsDefaultLimit+1).Return(mockQuery).Once()
	mockQuery.On("Where", oauthClientsSortAttr, ">", startModel.OAuthClientsSK).Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.OAuthClient)
		next := buildOAuthClientModel(t, "client-oldest", time.Date(2024, 3, 8, 15, 0, 0, 0, time.UTC))
		*dest = []*models.OAuthClient{next}
	}).Return(nil).Once()

	clients, nextCursor, err := repo.ListOAuthClients(ctx, 0, cursor)
	require.NoError(t, err)
	require.Len(t, clients, 1)
	assert.Equal(t, "client-oldest", clients[0].ClientID)
	assert.Empty(t, nextCursor)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestAccountRepository_ListOAuthClients_InvalidCursor(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamock.MockDB)
	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

	clients, nextCursor, err := repo.ListOAuthClients(ctx, 25, "invalid@@")
	require.Error(t, err)
	assert.Nil(t, clients)
	assert.Empty(t, nextCursor)

	mockDB.AssertExpectations(t)
}

func TestAccountRepository_DeleteRefreshTokensByUsernameAndClientID_ScanNotFound(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamock.MockDB)
	mockQuery := new(dynamock.MockQuery)
	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Scan", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

	deleted, err := repo.DeleteRefreshTokensByUsernameAndClientID(ctx, "alice", "client-1")
	require.NoError(t, err)
	require.Equal(t, 0, deleted)
	mockQuery.AssertNotCalled(t, "Delete")
}

func TestAccountRepository_DeleteRefreshTokensByUsernameAndClientID_DeletesAll(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamock.MockDB)
	mockQuery := new(dynamock.MockQuery)
	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.RefreshToken)
		*dest = []models.RefreshToken{
			{Token: "token-1", Username: "alice", ClientID: "client-1"},
			{Token: "token-2", Username: "alice", ClientID: "client-1"},
		}
	}).Return(nil).Once()
	mockQuery.On("Delete").Return(nil).Maybe()

	deleted, err := repo.DeleteRefreshTokensByUsernameAndClientID(ctx, "alice", "client-1")
	require.NoError(t, err)
	require.Equal(t, 2, deleted)
	mockQuery.AssertNumberOfCalls(t, "Delete", 2)
}

func buildOAuthClientModel(t *testing.T, clientID string, createdAt time.Time) *models.OAuthClient {
	t.Helper()

	model := &models.OAuthClient{
		ClientID:  clientID,
		Name:      strings.ToUpper(clientID),
		CreatedAt: createdAt,
	}
	require.NoError(t, model.BeforeCreate())
	return model
}
