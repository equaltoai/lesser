package repositories

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestAccountRepository_UpdateOAuthClientSecretHash_Success(t *testing.T) {
	t.Parallel()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdate := new(mocks.MockUpdateBuilder)

	repo := &AccountRepository{
		db:     mockDB,
		logger: zap.NewNop(),
	}

	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.OAuthClient")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "OAUTH_CLIENT#client-1").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", oauthClientSortKey).Return(mockQuery)
	mockQuery.On("UpdateBuilder").Return(mockUpdate)

	mockUpdate.On("Set", "ClientSecret", "hash-1").Return(mockUpdate)
	mockUpdate.On("Set", "UpdatedAt", mock.Anything).Return(mockUpdate)
	mockUpdate.On("Execute").Return(nil)

	err := repo.UpdateOAuthClientSecretHash(ctx, "client-1", "hash-1")
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
	mockUpdate.AssertExpectations(t)
}

func TestAccountRepository_UpdateOAuthClientSecretHash_ExecuteError(t *testing.T) {
	t.Parallel()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdate := new(mocks.MockUpdateBuilder)

	repo := &AccountRepository{
		db:     mockDB,
		logger: zap.NewNop(),
	}

	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.OAuthClient")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "OAUTH_CLIENT#client-1").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", oauthClientSortKey).Return(mockQuery)
	mockQuery.On("UpdateBuilder").Return(mockUpdate)

	mockUpdate.On("Set", "ClientSecret", "hash-1").Return(mockUpdate)
	mockUpdate.On("Set", "UpdatedAt", mock.Anything).Return(mockUpdate)
	mockUpdate.On("Execute").Return(ErrTestMockError)

	err := repo.UpdateOAuthClientSecretHash(ctx, "client-1", "hash-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OAuth client")

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
	mockUpdate.AssertExpectations(t)
}
