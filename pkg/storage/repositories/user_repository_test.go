package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

func TestCreateUser_MissingUsername(t *testing.T) {
	mockDB := new(mocks.MockDB)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	user := &storage.User{
		Email: "test@example.com",
	}

	err := repo.CreateUser(context.Background(), user)

	assert.Error(t, err)
	assert.IsType(t, common.ValidationError{}, err)
}

func TestGetUserByEmail_EmptyEmail(t *testing.T) {
	mockDB := new(mocks.MockDB)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	user, err := repo.GetUserByEmail(context.Background(), "")

	assert.Error(t, err)
	assert.Nil(t, user)
	assert.IsType(t, common.ValidationError{}, err)
}

func TestUpdateUser_EmptyUpdates(t *testing.T) {
	mockDB := new(mocks.MockDB)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	err := repo.UpdateUser(context.Background(), "testuser", map[string]any{})

	assert.Error(t, err)
	assert.IsType(t, common.ValidationError{}, err)
}

func TestGetUserByProviderID_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	// Set up expectations
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", &models.ProviderAccount{}).Return(mockQuery)
	mockQuery.On("Index", "provider-index").Return(mockQuery)
	mockQuery.On("Where", "GSI1PK", "=", "PROVIDER#google").Return(mockQuery)
	mockQuery.On("Where", "GSI1SK", "=", "123#").Return(mockQuery)
	mockQuery.On("Limit", 1).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(nil) // Return success but empty

	user, err := repo.GetUserByProviderID(context.Background(), "google", "123")

	assert.Error(t, err) // It returns an error when user not found
	assert.Nil(t, user)
	assert.Contains(t, err.Error(), "user not found")

	// Verify mocks
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestLinkProviderAccount_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	// Set up expectations for GetUser call
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", &models.User{}).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "USER#testuser").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "METADATA").Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(nil) // User exists

	// Set up expectations for the provider account creation
	mockDB.On("Model", mock.AnythingOfType("*models.ProviderAccount")).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	err := repo.LinkProviderAccount(context.Background(), "testuser", "google", "123")

	assert.NoError(t, err)

	// Verify mocks
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestUnlinkProviderAccount_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	// Set up expectations for query
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", &models.ProviderAccount{}).Return(mockQuery).Once()
	mockQuery.On("Index", "user-providers-index").Return(mockQuery)
	mockQuery.On("Where", "GSI2PK", "=", "USER_PROVIDERS#testuser").Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(nil) // Return empty list

	// Since no provider accounts found, no delete will be called

	err := repo.UnlinkProviderAccount(context.Background(), "testuser", "google")

	assert.Error(t, err) // Returns error when provider account not found
	assert.Contains(t, err.Error(), "provider account not found")

	// Verify mocks
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGetLinkedProviders_ReturnsEmpty(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	// Set up expectations
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", &models.ProviderAccount{}).Return(mockQuery)
	mockQuery.On("Index", "user-providers-index").Return(mockQuery)
	mockQuery.On("Where", "GSI2PK", "=", "USER_PROVIDERS#testuser").Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(nil) // Return empty list

	providers, err := repo.GetLinkedProviders(context.Background(), "testuser")

	assert.NoError(t, err)
	assert.Empty(t, providers)

	// Verify mocks
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestModelToStorage(t *testing.T) {
	mockDB := new(mocks.MockDB)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)
	now := time.Now()

	userModel := &models.User{
		Username:        "testuser",
		Email:           "test@example.com",
		PasswordHash:    "hashedpassword",
		DisplayName:     "Test User",
		CreatedAt:       now,
		UpdatedAt:       now,
		Approved:        true,
		Suspended:       false,
		Silenced:        false,
		Role:            "user",
		Locale:          "en",
		RecoveryMethods: []string{"email", "passkey"},
	}

	storageUser := repo.modelToStorage(userModel)

	assert.Equal(t, userModel.Username, storageUser.Username)
	assert.Equal(t, userModel.Email, storageUser.Email)
	assert.Equal(t, userModel.PasswordHash, storageUser.PasswordHash)
	assert.Equal(t, userModel.DisplayName, storageUser.DisplayName)
	assert.Equal(t, userModel.CreatedAt, storageUser.CreatedAt)
	assert.Equal(t, userModel.UpdatedAt, storageUser.UpdatedAt)
	assert.Equal(t, userModel.Approved, storageUser.Approved)
	assert.Equal(t, userModel.Suspended, storageUser.Suspended)
	assert.Equal(t, userModel.Silenced, storageUser.Silenced)
	assert.Equal(t, userModel.Role, storageUser.Role)
	assert.Equal(t, userModel.Locale, storageUser.Locale)
	assert.Equal(t, userModel.RecoveryMethods, storageUser.RecoveryMethods)
}

func TestApplyUpdates(t *testing.T) {
	mockDB := new(mocks.MockDB)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)
	userModel := &models.User{
		Username: "testuser",
		Email:    "old@example.com",
		Role:     "user",
		Approved: false,
	}

	updates := map[string]any{
		"email":            "new@example.com",
		"approved":         true,
		"role":             "moderator",
		"display_name":     "New Display Name",
		"suspended":        true,
		"silenced":         false,
		"locale":           "es",
		"password_hash":    "newhash",
		"recovery_methods": []string{"passkey", "wallet"},
		"invalid_field":    "should be ignored",
	}

	repo.applyUpdates(userModel, updates)

	assert.Equal(t, "new@example.com", userModel.Email)
	assert.True(t, userModel.Approved)
	assert.Equal(t, "moderator", userModel.Role)
	assert.Equal(t, "New Display Name", userModel.DisplayName)
	assert.True(t, userModel.Suspended)
	assert.False(t, userModel.Silenced)
	assert.Equal(t, "es", userModel.Locale)
	assert.Equal(t, "newhash", userModel.PasswordHash)
	assert.Equal(t, []string{"passkey", "wallet"}, userModel.RecoveryMethods)
}

func TestNewUserRepository(t *testing.T) {
	logger := zap.NewNop()
	repo := NewUserRepository(nil, "test-table", logger)

	assert.NotNil(t, repo)
	assert.Nil(t, repo.db)
}
