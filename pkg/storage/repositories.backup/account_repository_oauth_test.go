package repositories

import (
	"context"
	"strings"
	"testing"
	"time"

	dynamock "github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
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
