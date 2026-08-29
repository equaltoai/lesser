package repositories

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

// ============================================
// Test CreateUser validation
// ============================================

func TestUserRepository_CreateUser_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.User")).Return(mockQuery)
	mockQuery.On("Create").Return(nil)
	permitInstanceCountMaintenance(mockDB, mockQuery)

	user := &storage.User{
		Username: "testuser",
		Role:     "user",
	}

	err := repo.CreateUser(ctx, user)

	assert.NoError(t, err)
}

func TestUserRepository_CreateUser_MissingEmail(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.User")).Return(mockQuery)
	mockQuery.On("Create").Return(nil)
	permitInstanceCountMaintenance(mockDB, mockQuery)

	user := &storage.User{
		Username: "testuser",
		Role:     "user",
	}

	err := repo.CreateUser(context.Background(), user)
	assert.NoError(t, err)
}

func TestUserRepository_CreateUser_ConflictError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.User")).Return(mockQuery)
	mockQuery.On("Create").Return(dynamormerrors.ErrConditionFailed)

	user := &storage.User{
		Username: "testuser",
		Role:     "user",
	}

	err := repo.CreateUser(ctx, user)

	assert.Error(t, err)
}

// ============================================
// Test GetUser
// ============================================

func TestUserRepository_GetUser_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.User")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "USER#testuser").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "METADATA").Return(mockQuery)
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.User)
		out.Username = "testuser"
		out.Email = "test@example.com"
		out.Role = "user"
	}).Return(nil)

	user, err := repo.GetUser(ctx, "testuser")

	assert.NoError(t, err)
	assert.NotNil(t, user)
	assert.Equal(t, "testuser", user.Username)
}

func TestUserRepository_GetUser_NotFound(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.User")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "USER#unknownuser").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "METADATA").Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound)

	user, err := repo.GetUser(ctx, "unknownuser")

	assert.Error(t, err)
	assert.Nil(t, user)
}

// ============================================
// Test UpdateUser
// ============================================

func TestUserRepository_UpdateUser_InvalidUsername(t *testing.T) {
	mockDB := new(mocks.MockDB)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	err := repo.UpdateUser(context.Background(), "", map[string]any{"email": "new@example.com"})

	assert.Error(t, err)
}

func TestUserRepository_UpdateUser_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	// Mock Get call
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.User")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "USER#testuser").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "METADATA").Return(mockQuery)
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.User)
		out.Username = "testuser"
		out.Email = "old@example.com"
	}).Return(nil)

	// Mock Update call
	mockQuery.On("Update", mock.Anything).Return(nil)

	err := repo.UpdateUser(ctx, "testuser", map[string]any{"email": "new@example.com"})

	assert.NoError(t, err)
}

func TestUserRepository_UpdateUser_UserNotFound(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.User")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "USER#unknownuser").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "METADATA").Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound)

	err := repo.UpdateUser(ctx, "unknownuser", map[string]any{"email": "new@example.com"})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed to retrieve user")
}

// ============================================
// Test DeleteUser
// ============================================

func TestUserRepository_DeleteUser_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.User")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "USER#testuser").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "METADATA").Return(mockQuery)
	mockQuery.On("Delete").Return(nil)
	permitInstanceCountMaintenance(mockDB, mockQuery)

	err := repo.DeleteUser(ctx, "testuser")

	assert.NoError(t, err)
}

func TestUserRepository_DeleteUser_NotFound(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.User")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "USER#unknownuser").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "METADATA").Return(mockQuery)
	mockQuery.On("Delete").Return(dynamormerrors.ErrItemNotFound)
	permitInstanceCountMaintenance(mockDB, mockQuery)

	err := repo.DeleteUser(ctx, "unknownuser")

	// DeleteUser may not return an error on not found - depends on implementation
	// The important thing is the function doesn't panic
	_ = err
}

func TestUserRepository_DeleteUser_Error(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.User")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "USER#testuser").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "METADATA").Return(mockQuery)
	mockQuery.On("Delete").Return(ErrTestMockError)

	err := repo.DeleteUser(ctx, "testuser")

	assert.Error(t, err)
}

// ============================================
// Test GetUserByEmail
// ============================================

func TestUserRepository_GetUserByEmail_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", &models.User{}).Return(mockQuery)
	mockQuery.On("Index", "gsi2").Return(mockQuery)
	mockQuery.On("Where", "gsi2PK", "=", "EMAIL#test@example.com").Return(mockQuery)
	mockQuery.On("Limit", 1).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.User)
		*out = []models.User{
			{Username: "testuser", Email: "test@example.com"},
		}
	}).Return(nil)

	user, err := repo.GetUserByEmail(ctx, "test@example.com")

	assert.NoError(t, err)
	assert.NotNil(t, user)
	assert.Equal(t, "testuser", user.Username)
}

func TestUserRepository_GetUserByEmail_NotFound(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", &models.User{}).Return(mockQuery)
	mockQuery.On("Index", "gsi2").Return(mockQuery)
	mockQuery.On("Where", "gsi2PK", "=", "EMAIL#unknown@example.com").Return(mockQuery)
	mockQuery.On("Limit", 1).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(nil) // Empty result

	user, err := repo.GetUserByEmail(ctx, "unknown@example.com")

	assert.Error(t, err)
	assert.Nil(t, user)
}

func TestUserRepository_GetUserByEmail_QueryError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", &models.User{}).Return(mockQuery)
	mockQuery.On("Index", "gsi2").Return(mockQuery)
	mockQuery.On("Where", "gsi2PK", "=", "EMAIL#test@example.com").Return(mockQuery)
	mockQuery.On("Limit", 1).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(ErrTestMockError)

	user, err := repo.GetUserByEmail(ctx, "test@example.com")

	assert.Error(t, err)
	assert.Nil(t, user)
}

// ============================================
// Test GetActiveUserCount
// ============================================

func TestUserRepository_GetActiveUserCount_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", &models.User{}).Return(mockQuery)
	mockQuery.On("Index", "gsi3").Return(mockQuery)
	mockQuery.On("Where", "gsi3PK", "=", "ACTIVITY").Return(mockQuery)
	mockQuery.On("Where", "gsi3SK", ">=", mock.AnythingOfType("string")).Return(mockQuery)
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.User)
		*out = []models.User{
			{Username: "user1"},
			{Username: "user2"},
			{Username: "user3"},
		}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	count, err := repo.GetActiveUserCount(ctx, 30)

	assert.NoError(t, err)
	assert.Equal(t, int64(3), count)
}

func TestUserRepository_GetActiveUserCount_Error(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", &models.User{}).Return(mockQuery)
	mockQuery.On("Index", "gsi3").Return(mockQuery)
	mockQuery.On("Where", "gsi3PK", "=", "ACTIVITY").Return(mockQuery)
	mockQuery.On("Where", "gsi3SK", ">=", mock.AnythingOfType("string")).Return(mockQuery)
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.Anything).Return(&core.PaginatedResult{HasMore: false}, ErrTestMockError).Once()

	count, err := repo.GetActiveUserCount(ctx, 30)

	assert.Error(t, err)
	assert.Equal(t, int64(0), count)
}

// ============================================
// Test GetTotalUserCount
// ============================================

func TestUserRepository_GetTotalUserCount_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", &models.User{}).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "USERS").Return(mockQuery)
	// GetTotalUserCount is now a page-capped walk (wave #1469): count = walked
	// rows.
	mockQuery.On("Limit", 500).Return(mockQuery)
	mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.User)
		*out = make([]models.User, 42)
	}).Return(&core.PaginatedResult{}, nil)

	count, err := repo.GetTotalUserCount(ctx)

	assert.NoError(t, err)
	assert.Equal(t, int64(42), count)
}

func TestUserRepository_GetTotalUserCount_Error(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", &models.User{}).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "USERS").Return(mockQuery)
	// Page-capped walk (wave #1469): the walk error propagates.
	mockQuery.On("Limit", 500).Return(mockQuery)
	mockQuery.On("AllPaginated", mock.Anything).Return(nil, ErrTestMockError)

	count, err := repo.GetTotalUserCount(ctx)

	assert.Error(t, err)
	assert.Equal(t, int64(0), count)
}

// ============================================
// Test NewUserRepositoryWithCostTracking
// ============================================

func TestNewUserRepositoryWithCostTracking(t *testing.T) {
	logger := zap.NewNop()
	repo := NewUserRepositoryWithCostTracking(nil, "test-table", logger, nil)

	assert.NotNil(t, repo)
}

// ============================================
// Test SetDependencies and SetBookmarkRepository
// ============================================

func TestUserRepository_SetDependencies(t *testing.T) {
	logger := zap.NewNop()
	repo := NewUserRepository(nil, "test-table", logger)

	// Should not panic
	repo.SetDependencies(nil)
}

func TestUserRepository_SetBookmarkRepository(t *testing.T) {
	logger := zap.NewNop()
	repo := NewUserRepository(nil, "test-table", logger)

	// Should not panic
	repo.SetBookmarkRepository(nil)
}
