package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func TestUserRepository_StoreReputation_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Reputation")).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	rep := &storage.Reputation{
		CalculatedAt: time.Now().UTC(),
		TotalScore:   42,
	}

	err := repo.StoreReputation(context.Background(), "https://example.com/users/alice", rep)
	assert.NoError(t, err)
}

func TestUserRepository_StoreReputation_UpdateKeysError(t *testing.T) {
	repo := NewUserRepository(nil, "test-table", zap.NewNop())

	rep := &storage.Reputation{
		CalculatedAt: time.Now().UTC(),
		TotalScore:   42,
	}

	err := repo.StoreReputation(context.Background(), "", rep)
	assert.Error(t, err)
}

func TestUserRepository_StoreReputation_CreateError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Reputation")).Return(mockQuery)
	mockQuery.On("Create").Return(ErrTestMockError)

	rep := &storage.Reputation{
		CalculatedAt: time.Now().UTC(),
		TotalScore:   42,
	}

	err := repo.StoreReputation(context.Background(), "https://example.com/users/alice", rep)
	assert.Error(t, err)
}
