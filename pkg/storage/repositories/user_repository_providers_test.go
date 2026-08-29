package repositories

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

// ============================================
// Test LinkProviderAccount
// ============================================

func TestUserRepository_LinkProviderAccount_UserNotFound(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	// Setup expectations - user lookup fails
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", &models.User{}).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "USER#nonexistent").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "METADATA").Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound)

	err := repo.LinkProviderAccount(ctx, "nonexistent", "google", "123")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed to retrieve user")
}

func TestUserRepository_LinkProviderAccount_CreateConflict(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	// Setup expectations - user lookup succeeds
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", &models.User{}).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "USER#testuser").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "METADATA").Return(mockQuery)
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.User)
		out.Username = "testuser"
	}).Return(nil)

	// Setup expectations - duplicate provider lookup finds no existing link
	mockDB.On("Model", &models.ProviderAccount{}).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "PROVIDER#google").Return(mockQuery)
	mockQuery.On("Where", "gsi1SK", "BEGINS_WITH", "123#").Return(mockQuery)
	mockQuery.On("Limit", 2).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(nil)

	// Setup expectations - provider creation fails with condition failed (conflict)
	mockDB.On("Model", mock.AnythingOfType("*models.ProviderAccount")).Return(mockQuery)
	mockQuery.On("Create").Return(dynamormerrors.ErrConditionFailed)

	err := repo.LinkProviderAccount(ctx, "testuser", "google", "123")

	assert.Error(t, err)
	// Conflict error is wrapped by ErrorHandler with "Failed to create provider account"
	assert.Contains(t, err.Error(), "Failed to create provider account")
}

func TestUserRepository_LinkProviderAccount_CreateError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	// Setup expectations - user lookup succeeds
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", &models.User{}).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "USER#testuser").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "METADATA").Return(mockQuery)
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.User)
		out.Username = "testuser"
	}).Return(nil)

	// Setup expectations - duplicate provider lookup finds no existing link
	mockDB.On("Model", &models.ProviderAccount{}).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "PROVIDER#google").Return(mockQuery)
	mockQuery.On("Where", "gsi1SK", "BEGINS_WITH", "123#").Return(mockQuery)
	mockQuery.On("Limit", 2).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(nil)

	// Setup expectations - provider creation fails with generic error
	mockDB.On("Model", mock.AnythingOfType("*models.ProviderAccount")).Return(mockQuery)
	mockQuery.On("Create").Return(ErrTestMockError)

	err := repo.LinkProviderAccount(ctx, "testuser", "google", "123")

	assert.Error(t, err)
}

// ============================================
// Test UnlinkProviderAccount
// ============================================

func TestUserRepository_UnlinkProviderAccount_QueryError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", &models.ProviderAccount{}).Return(mockQuery)
	mockQuery.On("Index", "gsi2").Return(mockQuery)
	mockQuery.On("Where", "gsi2PK", "=", "USER_PROVIDERS#testuser").Return(mockQuery)
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.Anything).Return(&core.PaginatedResult{HasMore: false}, ErrTestMockError).Once()

	err := repo.UnlinkProviderAccount(ctx, "testuser", "google")

	assert.Error(t, err)
}

func TestUserRepository_UnlinkProviderAccount_NotLinked(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", &models.ProviderAccount{}).Return(mockQuery)
	mockQuery.On("Index", "gsi2").Return(mockQuery)
	mockQuery.On("Where", "gsi2PK", "=", "USER_PROVIDERS#testuser").Return(mockQuery)
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		// Return accounts for different providers
		out := args.Get(0).(*[]models.ProviderAccount)
		*out = []models.ProviderAccount{
			{UserID: "testuser", Provider: "github", ProviderID: "456", IsActive: true},
			{UserID: "testuser", Provider: "facebook", ProviderID: "789", IsActive: true},
		}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	err := repo.UnlinkProviderAccount(ctx, "testuser", "google")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed to retrieve provider account")
}

func TestUserRepository_UnlinkProviderAccount_DeleteSuccess(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", &models.ProviderAccount{}).Return(mockQuery).Once()
	mockQuery.On("Index", "gsi2").Return(mockQuery)
	mockQuery.On("Where", "gsi2PK", "=", "USER_PROVIDERS#testuser").Return(mockQuery)
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.ProviderAccount)
		*out = []models.ProviderAccount{
			{UserID: "testuser", Provider: "google", ProviderID: "123", IsActive: true},
		}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	// Expect delete
	mockDB.On("Model", mock.AnythingOfType("*models.ProviderAccount")).Return(mockQuery)
	mockQuery.On("Delete").Return(nil)

	err := repo.UnlinkProviderAccount(ctx, "testuser", "google")

	assert.NoError(t, err)
}

func TestUserRepository_UnlinkProviderAccount_DeleteError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", &models.ProviderAccount{}).Return(mockQuery).Once()
	mockQuery.On("Index", "gsi2").Return(mockQuery)
	mockQuery.On("Where", "gsi2PK", "=", "USER_PROVIDERS#testuser").Return(mockQuery)
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.ProviderAccount)
		*out = []models.ProviderAccount{
			{UserID: "testuser", Provider: "google", ProviderID: "123", IsActive: true},
		}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	// Expect delete to fail
	mockDB.On("Model", mock.AnythingOfType("*models.ProviderAccount")).Return(mockQuery)
	mockQuery.On("Delete").Return(ErrTestMockError)

	err := repo.UnlinkProviderAccount(ctx, "testuser", "google")

	assert.Error(t, err)
}

func TestUserRepository_UnlinkProviderAccount_MultipleProviders(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", &models.ProviderAccount{}).Return(mockQuery).Once()
	mockQuery.On("Index", "gsi2").Return(mockQuery)
	mockQuery.On("Where", "gsi2PK", "=", "USER_PROVIDERS#testuser").Return(mockQuery)
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		// User has multiple google accounts
		out := args.Get(0).(*[]models.ProviderAccount)
		*out = []models.ProviderAccount{
			{UserID: "testuser", Provider: "google", ProviderID: "123", IsActive: true},
			{UserID: "testuser", Provider: "google", ProviderID: "456", IsActive: true},
			{UserID: "testuser", Provider: "github", ProviderID: "789", IsActive: true},
		}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	// Expect two deletes (for both google accounts)
	mockDB.On("Model", mock.AnythingOfType("*models.ProviderAccount")).Return(mockQuery)
	mockQuery.On("Delete").Return(nil).Twice()

	err := repo.UnlinkProviderAccount(ctx, "testuser", "google")

	assert.NoError(t, err)
}

// ============================================
// Test GetLinkedProviders
// ============================================

func TestUserRepository_GetLinkedProviders_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", &models.ProviderAccount{}).Return(mockQuery)
	mockQuery.On("Index", "gsi2").Return(mockQuery)
	mockQuery.On("Where", "gsi2PK", "=", "USER_PROVIDERS#testuser").Return(mockQuery)
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.ProviderAccount)
		*out = []models.ProviderAccount{
			{UserID: "testuser", Provider: "google", ProviderID: "123", IsActive: true},
			{UserID: "testuser", Provider: "github", ProviderID: "456", IsActive: true},
			{UserID: "testuser", Provider: "facebook", ProviderID: "789", IsActive: true},
		}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	providers, err := repo.GetLinkedProviders(ctx, "testuser")

	assert.NoError(t, err)
	assert.Len(t, providers, 3)
	assert.Contains(t, providers, "google")
	assert.Contains(t, providers, "github")
	assert.Contains(t, providers, "facebook")
}

func TestUserRepository_GetLinkedProviders_FilterInactive(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", &models.ProviderAccount{}).Return(mockQuery)
	mockQuery.On("Index", "gsi2").Return(mockQuery)
	mockQuery.On("Where", "gsi2PK", "=", "USER_PROVIDERS#testuser").Return(mockQuery)
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.ProviderAccount)
		*out = []models.ProviderAccount{
			{UserID: "testuser", Provider: "google", ProviderID: "123", IsActive: true},
			{UserID: "testuser", Provider: "github", ProviderID: "456", IsActive: false}, // Inactive
			{UserID: "testuser", Provider: "facebook", ProviderID: "789", IsActive: true},
		}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	providers, err := repo.GetLinkedProviders(ctx, "testuser")

	assert.NoError(t, err)
	assert.Len(t, providers, 2)
	assert.Contains(t, providers, "google")
	assert.Contains(t, providers, "facebook")
	assert.NotContains(t, providers, "github")
}

func TestUserRepository_GetLinkedProviders_UniqueProviders(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", &models.ProviderAccount{}).Return(mockQuery)
	mockQuery.On("Index", "gsi2").Return(mockQuery)
	mockQuery.On("Where", "gsi2PK", "=", "USER_PROVIDERS#testuser").Return(mockQuery)
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		// Multiple google accounts
		out := args.Get(0).(*[]models.ProviderAccount)
		*out = []models.ProviderAccount{
			{UserID: "testuser", Provider: "google", ProviderID: "123", IsActive: true},
			{UserID: "testuser", Provider: "google", ProviderID: "456", IsActive: true},
			{UserID: "testuser", Provider: "github", ProviderID: "789", IsActive: true},
		}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	providers, err := repo.GetLinkedProviders(ctx, "testuser")

	assert.NoError(t, err)
	assert.Len(t, providers, 2) // Should be unique
	assert.Contains(t, providers, "google")
	assert.Contains(t, providers, "github")
}

func TestUserRepository_GetLinkedProviders_QueryError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", &models.ProviderAccount{}).Return(mockQuery)
	mockQuery.On("Index", "gsi2").Return(mockQuery)
	mockQuery.On("Where", "gsi2PK", "=", "USER_PROVIDERS#testuser").Return(mockQuery)
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.Anything).Return(&core.PaginatedResult{HasMore: false}, ErrTestMockError).Once()

	providers, err := repo.GetLinkedProviders(ctx, "testuser")

	assert.Error(t, err)
	assert.Nil(t, providers)
}

func TestUserRepository_GetLinkedProviders_NoProviders(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", &models.ProviderAccount{}).Return(mockQuery)
	mockQuery.On("Index", "gsi2").Return(mockQuery)
	mockQuery.On("Where", "gsi2PK", "=", "USER_PROVIDERS#testuser").Return(mockQuery)
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.Anything).Return(&core.PaginatedResult{HasMore: false}, nil).Once() // Empty result

	providers, err := repo.GetLinkedProviders(ctx, "testuser")

	assert.NoError(t, err)
	assert.Empty(t, providers)
}
