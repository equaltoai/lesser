package testing

import (
	"context"
	"testing"

	storageTypes "github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestNewMockRepositoryStorage_DefaultsToInMemory verifies that the default
// MockRepositoryStorage uses in-memory implementations.
func TestNewMockRepositoryStorage_DefaultsToInMemory(t *testing.T) {
	s := NewMockRepositoryStorage()

	// Verify user repository is not nil and is functional
	require.NotNil(t, s.User(), "User repository should not be nil")

	// Test that the in-memory user repository works
	ctx := context.Background()
	user := &storageTypes.User{
		Username: "testuser",
		Email:    "test@example.com",
	}

	// Create user should succeed
	err := s.User().CreateUser(ctx, user)
	require.NoError(t, err, "CreateUser should succeed with in-memory implementation")

	// Get user should return the created user
	retrieved, err := s.User().GetUser(ctx, "testuser")
	require.NoError(t, err, "GetUser should succeed")
	assert.Equal(t, "testuser", retrieved.Username)
	assert.Equal(t, "test@example.com", retrieved.Email)
}

// TestNewMockRepositoryStorage_WithCustomUserRepository verifies that custom
// mock repositories can be injected via functional options.
func TestNewMockRepositoryStorage_WithCustomUserRepository(t *testing.T) {
	// Create a custom mock user repository
	mockUserRepo := mocks.NewMockUserRepositoryInterface()

	// Set up expectations
	expectedUser := &storageTypes.User{
		Username: "mockuser",
		Email:    "mock@example.com",
	}
	mockUserRepo.On("GetUser", context.Background(), "mockuser").Return(expectedUser, nil)

	// Create storage with custom user repository
	s := NewMockRepositoryStorage(
		WithUserRepository(mockUserRepo),
	)

	// Verify the custom mock is used
	ctx := context.Background()
	user, err := s.User().GetUser(ctx, "mockuser")
	require.NoError(t, err)
	assert.Equal(t, "mockuser", user.Username)
	assert.Equal(t, "mock@example.com", user.Email)

	// Verify mock expectations were met
	mockUserRepo.AssertExpectations(t)
}

// TestNewMockRepositoryStorage_WithLogger verifies that custom logger can be set.
func TestNewMockRepositoryStorage_WithLogger(t *testing.T) {
	logger := zap.NewExample()

	s := NewMockRepositoryStorage(
		WithLogger(logger),
	)

	assert.Equal(t, logger, s.GetLogger())
}

// TestNewMockRepositoryStorage_WithTableName verifies that custom table name can be set.
func TestNewMockRepositoryStorage_WithTableName(t *testing.T) {
	s := NewMockRepositoryStorage(
		WithTableName("custom-table"),
	)

	assert.Equal(t, "custom-table", s.GetTableName())
}

// TestMockRepositoryStorage_UserReturnsInterfaceType verifies that User()
// returns an interfaces.UserRepository type.
func TestMockRepositoryStorage_UserReturnsInterfaceType(t *testing.T) {
	s := NewMockRepositoryStorage()

	// This should compile - verifying the return type is interfaces.UserRepository
	var userRepo interfaces.UserRepository = s.User()
	assert.NotNil(t, userRepo)
}

// TestMockRepositoryStorage_GetDBReturnsNil verifies that GetDB returns nil
// for mock storage (no real database connection).
func TestMockRepositoryStorage_GetDBReturnsNil(t *testing.T) {
	s := NewMockRepositoryStorage()
	assert.Nil(t, s.GetDB())
}

// TestMockRepositoryStorage_DefaultTableName verifies the default table name.
func TestMockRepositoryStorage_DefaultTableName(t *testing.T) {
	s := NewMockRepositoryStorage()
	assert.Equal(t, "test-table", s.GetTableName())
}

// TestMockRepositoryStorage_DefaultLogger verifies the default logger is a no-op logger.
func TestMockRepositoryStorage_DefaultLogger(t *testing.T) {
	s := NewMockRepositoryStorage()
	assert.NotNil(t, s.GetLogger())
}

// TestMockRepositoryStorage_InMemoryRoundTrip verifies that the in-memory
// user repository correctly stores and retrieves data.
func TestMockRepositoryStorage_InMemoryRoundTrip(t *testing.T) {
	s := NewMockRepositoryStorage()
	ctx := context.Background()

	// Create multiple users
	users := []*storageTypes.User{
		{Username: "user1", Email: "user1@example.com"},
		{Username: "user2", Email: "user2@example.com"},
		{Username: "user3", Email: "user3@example.com"},
	}

	for _, user := range users {
		err := s.User().CreateUser(ctx, user)
		require.NoError(t, err)
	}

	// Verify all users can be retrieved
	for _, user := range users {
		retrieved, err := s.User().GetUser(ctx, user.Username)
		require.NoError(t, err)
		assert.Equal(t, user.Username, retrieved.Username)
		assert.Equal(t, user.Email, retrieved.Email)
	}

	// Verify total count
	count, err := s.User().GetTotalUserCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)
}

// TestMockRepositoryStorage_InMemoryNotFound verifies that the in-memory
// repository returns appropriate errors for non-existent data.
func TestMockRepositoryStorage_InMemoryNotFound(t *testing.T) {
	s := NewMockRepositoryStorage()
	ctx := context.Background()

	// Try to get a non-existent user
	_, err := s.User().GetUser(ctx, "nonexistent")
	assert.Error(t, err)
	assert.Equal(t, storageTypes.ErrNotFound, err)
}

// TestMockRepositoryStorage_InMemoryDuplicateError verifies that the in-memory
// repository returns appropriate errors for duplicate data.
func TestMockRepositoryStorage_InMemoryDuplicateError(t *testing.T) {
	s := NewMockRepositoryStorage()
	ctx := context.Background()

	user := &storageTypes.User{
		Username: "duplicate",
		Email:    "dup@example.com",
	}

	// First create should succeed
	err := s.User().CreateUser(ctx, user)
	require.NoError(t, err)

	// Second create should fail with ErrAlreadyExists
	err = s.User().CreateUser(ctx, user)
	assert.Error(t, err)
	assert.Equal(t, storageTypes.ErrAlreadyExists, err)
}

// TestMockRepositoryStorage_MultipleOptions verifies that multiple options
// can be applied together.
func TestMockRepositoryStorage_MultipleOptions(t *testing.T) {
	logger := zap.NewExample()
	customRepo := inmemory.NewUserRepository()

	s := NewMockRepositoryStorage(
		WithUserRepository(customRepo),
		WithLogger(logger),
		WithTableName("multi-option-table"),
	)

	assert.Equal(t, customRepo, s.User())
	assert.Equal(t, logger, s.GetLogger())
	assert.Equal(t, "multi-option-table", s.GetTableName())
}
