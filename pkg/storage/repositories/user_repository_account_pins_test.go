package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	dynamormerrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

// ============================================
// Test CreateAccountPin
// ============================================

func TestUserRepository_CreateAccountPin_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()
	pin := &storage.AccountPin{
		Username:       "alice",
		PinnedActorID:  "https://example.com/users/bob",
		PinnedUsername: "bob",
	}

	// Setup expectations for IsAccountPinned
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.AccountPin")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "SK", "=", mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

	// Setup expectations for Create
	mockQuery.On("Create").Return(nil)

	err := repo.CreateAccountPin(ctx, pin)

	assert.NoError(t, err)
	assert.False(t, pin.CreatedAt.IsZero())
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestUserRepository_CreateAccountPin_SetsCreatedAtWhenZero(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()
	pin := &storage.AccountPin{
		Username:       "alice",
		PinnedActorID:  "https://example.com/users/bob",
		PinnedUsername: "bob",
		CreatedAt:      time.Time{}, // Zero value
	}

	// Setup expectations for IsAccountPinned (not found)
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.AccountPin")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "SK", "=", mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	mockQuery.On("Create").Return(nil)

	before := time.Now()
	err := repo.CreateAccountPin(ctx, pin)
	after := time.Now()

	assert.NoError(t, err)
	assert.False(t, pin.CreatedAt.IsZero())
	assert.True(t, pin.CreatedAt.After(before) || pin.CreatedAt.Equal(before))
	assert.True(t, pin.CreatedAt.Before(after) || pin.CreatedAt.Equal(after))
}

func TestUserRepository_CreateAccountPin_PreservesExistingCreatedAt(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()
	existingTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	pin := &storage.AccountPin{
		Username:       "alice",
		PinnedActorID:  "https://example.com/users/bob",
		PinnedUsername: "bob",
		CreatedAt:      existingTime,
	}

	// Setup expectations for IsAccountPinned (not found)
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.AccountPin")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "SK", "=", mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	mockQuery.On("Create").Return(nil)

	err := repo.CreateAccountPin(ctx, pin)

	assert.NoError(t, err)
	assert.Equal(t, existingTime, pin.CreatedAt)
}

func TestUserRepository_CreateAccountPin_Conflict(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()
	pin := &storage.AccountPin{
		Username:       "alice",
		PinnedActorID:  "https://example.com/users/bob",
		PinnedUsername: "bob",
	}

	// Setup expectations for IsAccountPinned - returns found
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.AccountPin")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "SK", "=", mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		// Pin exists
		out := args.Get(0).(*models.AccountPin)
		out.Username = "alice"
		out.PinnedActorID = "https://example.com/users/bob"
	}).Return(nil)

	err := repo.CreateAccountPin(ctx, pin)

	assert.Error(t, err)
	// Conflict error is wrapped by ErrorHandler with "Failed to create account pin"
	assert.Contains(t, err.Error(), "Failed to create account pin")
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestUserRepository_CreateAccountPin_IsAccountPinnedError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()
	pin := &storage.AccountPin{
		Username:       "alice",
		PinnedActorID:  "https://example.com/users/bob",
		PinnedUsername: "bob",
	}

	// Setup expectations for IsAccountPinned - returns error
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.AccountPin")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "SK", "=", mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(ErrTestMockError)

	err := repo.CreateAccountPin(ctx, pin)

	assert.Error(t, err)
}

func TestUserRepository_CreateAccountPin_CreateError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()
	pin := &storage.AccountPin{
		Username:       "alice",
		PinnedActorID:  "https://example.com/users/bob",
		PinnedUsername: "bob",
	}

	// Setup expectations for IsAccountPinned (not found)
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.AccountPin")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "SK", "=", mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

	// Create fails
	mockQuery.On("Create").Return(ErrTestMockError)

	err := repo.CreateAccountPin(ctx, pin)
	assert.Error(t, err)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ============================================
// Test IsAccountPinned
// ============================================

func TestUserRepository_IsAccountPinned_True(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.AccountPin")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "SK", "=", mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.AccountPin)
		out.Username = "alice"
		out.PinnedActorID = "https://example.com/users/bob"
	}).Return(nil)

	isPinned, err := repo.IsAccountPinned(ctx, "alice", "https://example.com/users/bob")

	assert.NoError(t, err)
	assert.True(t, isPinned)
}

func TestUserRepository_IsAccountPinned_False(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.AccountPin")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "SK", "=", mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound)

	isPinned, err := repo.IsAccountPinned(ctx, "alice", "https://example.com/users/bob")

	assert.NoError(t, err)
	assert.False(t, isPinned)
}

func TestUserRepository_IsAccountPinned_Error(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.AccountPin")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "SK", "=", mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(ErrTestMockError)

	isPinned, err := repo.IsAccountPinned(ctx, "alice", "https://example.com/users/bob")

	assert.Error(t, err)
	assert.False(t, isPinned)
}

// ============================================
// Test GetAccountPins
// ============================================

func TestUserRepository_GetAccountPins_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.AccountPin")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", mock.Anything).Return(mockQuery)
	mockQuery.On("Filter", "SK", "BEGINS_WITH", "PIN#").Return(mockQuery)
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*[]models.AccountPin)
		*out = []models.AccountPin{
			{
				Username:       "alice",
				PinnedActorID:  "https://example.com/users/bob",
				PinnedUsername: "bob",
				CreatedAt:      time.Now(),
			},
			{
				Username:       "alice",
				PinnedActorID:  "https://example.com/users/charlie",
				PinnedUsername: "charlie",
				CreatedAt:      time.Now(),
			},
		}
	}).Return(nil)

	pins, err := repo.GetAccountPins(ctx, "alice")

	assert.NoError(t, err)
	assert.Len(t, pins, 2)
	assert.Equal(t, "bob", pins[0].PinnedUsername)
	assert.Equal(t, "charlie", pins[1].PinnedUsername)
}

func TestUserRepository_GetAccountPins_Empty(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.AccountPin")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", mock.Anything).Return(mockQuery)
	mockQuery.On("Filter", "SK", "BEGINS_WITH", "PIN#").Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(nil) // Empty result

	pins, err := repo.GetAccountPins(ctx, "alice")

	assert.NoError(t, err)
	assert.Empty(t, pins)
}

func TestUserRepository_GetAccountPins_QueryError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.AccountPin")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", mock.Anything).Return(mockQuery)
	mockQuery.On("Filter", "SK", "BEGINS_WITH", "PIN#").Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(ErrTestMockError)

	pins, err := repo.GetAccountPins(ctx, "alice")

	assert.Error(t, err)
	assert.Nil(t, pins)
}

// ============================================
// Test DeleteAccountPin
// ============================================

func TestUserRepository_DeleteAccountPin_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.AccountPin")).Return(mockQuery)
	mockQuery.On("Delete").Return(nil)

	err := repo.DeleteAccountPin(ctx, "alice", "https://example.com/users/bob")

	assert.NoError(t, err)
}

func TestUserRepository_DeleteAccountPin_Error(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewUserRepository(mockDB, "test-table", logger)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.AccountPin")).Return(mockQuery)
	mockQuery.On("Delete").Return(ErrTestMockError)

	err := repo.DeleteAccountPin(ctx, "alice", "https://example.com/users/bob")

	assert.Error(t, err)
}
