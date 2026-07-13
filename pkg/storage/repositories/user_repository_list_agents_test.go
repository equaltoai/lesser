package repositories

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func TestUserRepository_ListAgents_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", &models.User{}).Return(mockQuery)
	mockQuery.On("Index", "gsi6").Return(mockQuery)
	mockQuery.On("Where", "gsi6PK", "=", "ACCOUNT_TYPE#AGENT").Return(mockQuery)
	mockQuery.On("Limit", 21).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.User)
		*out = []models.User{
			{Username: "agent-a", IsAgent: true, GSI6SK: "2025-01-01T00:00:00Z#agent-a"},
			{Username: "agent-b", IsAgent: true, GSI6SK: "2025-01-02T00:00:00Z#agent-b"},
		}
	}).Return(nil)

	users, nextCursor, err := repo.ListAgents(context.Background(), 20, "")
	assert.NoError(t, err)
	assert.Len(t, users, 2)
	assert.Empty(t, nextCursor)
	assert.Equal(t, "agent-a", users[0].Username)
}

func TestUserRepository_ListAgents_WithCursor(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	cursor := "2025-01-01T00:00:00Z#agent-a"
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", &models.User{}).Return(mockQuery)
	mockQuery.On("Index", "gsi6").Return(mockQuery)
	mockQuery.On("Where", "gsi6PK", "=", "ACCOUNT_TYPE#AGENT").Return(mockQuery)
	mockQuery.On("Limit", 21).Return(mockQuery)
	mockQuery.On("Where", "gsi6SK", ">", cursor).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.User)
		*out = []models.User{
			{Username: "agent-b", IsAgent: true, GSI6SK: "2025-01-02T00:00:00Z#agent-b"},
		}
	}).Return(nil)

	users, nextCursor, err := repo.ListAgents(context.Background(), 20, cursor)
	assert.NoError(t, err)
	assert.Len(t, users, 1)
	assert.Empty(t, nextCursor)
	assert.Equal(t, "agent-b", users[0].Username)
}

func TestUserRepository_ListAgents_WithPaginationSetsCursor(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", &models.User{}).Return(mockQuery)
	mockQuery.On("Index", "gsi6").Return(mockQuery)
	mockQuery.On("Where", "gsi6PK", "=", "ACCOUNT_TYPE#AGENT").Return(mockQuery)
	mockQuery.On("Limit", 3).Return(mockQuery) // limit+1
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.User)
		*out = []models.User{
			{Username: "agent-a", IsAgent: true, GSI6SK: "A"},
			{Username: "agent-b", IsAgent: true, GSI6SK: "B"},
			{Username: "agent-c", IsAgent: true, GSI6SK: "C"},
		}
	}).Return(nil)

	_, nextCursor, err := repo.ListAgents(context.Background(), 2, "")
	assert.NoError(t, err)
	assert.Equal(t, "B", nextCursor)
}

func TestUserRepository_ListAgents_InvalidLimitFallsBack(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", &models.User{}).Return(mockQuery)
	mockQuery.On("Index", "gsi6").Return(mockQuery)
	mockQuery.On("Where", "gsi6PK", "=", "ACCOUNT_TYPE#AGENT").Return(mockQuery)
	mockQuery.On("Limit", 21).Return(mockQuery) // fallback 20+1
	mockQuery.On("All", mock.Anything).Return(nil)

	_, _, err := repo.ListAgents(context.Background(), -1, "")
	assert.NoError(t, err)
}

func TestUserRepository_ListAgents_QueryError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", &models.User{}).Return(mockQuery)
	mockQuery.On("Index", "gsi6").Return(mockQuery)
	mockQuery.On("Where", "gsi6PK", "=", "ACCOUNT_TYPE#AGENT").Return(mockQuery)
	mockQuery.On("Limit", 21).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(ErrTestMockError)

	users, nextCursor, err := repo.ListAgents(context.Background(), 20, "")
	assert.Error(t, err)
	assert.Nil(t, users)
	assert.Empty(t, nextCursor)
}
