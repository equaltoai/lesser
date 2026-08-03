package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

// ============================================================================
// GetByActorURL Tests
// ============================================================================

// Note: TestPublicKeyCacheRepository_GetByActorURL_UpdateKeysError is not needed
// because UpdateKeys() in public_key_cache.go always returns nil

func TestPublicKeyCacheRepository_GetByActorURL_NotFound(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewPublicKeyCacheRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()
	actorURL := "https://mastodon.social/users/alice"

	// Set up mock expectations
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.PublicKeyCache")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.PublicKeyCache")).Return(dynamormerrors.ErrItemNotFound)

	// Execute
	result, err := repo.GetByActorURL(ctx, actorURL)

	// Assert
	require.Error(t, err)
	assert.Nil(t, result)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestPublicKeyCacheRepository_GetByActorURL_DBError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewPublicKeyCacheRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()
	actorURL := "https://mastodon.social/users/alice"
	testErr := errors.New("database connection failed")

	// Set up mock expectations
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.PublicKeyCache")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.PublicKeyCache")).Return(testErr)

	// Execute
	result, err := repo.GetByActorURL(ctx, actorURL)

	// Assert
	require.Error(t, err)
	assert.Nil(t, result)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestPublicKeyCacheRepository_GetByActorURL_Expired(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewPublicKeyCacheRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()
	actorURL := "https://mastodon.social/users/alice"

	// Create an expired cache entry (TTL in the past)
	expiredTTL := time.Now().Add(-1 * time.Hour).Unix()

	// Set up mock expectations - using broad matchers
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.PublicKeyCache")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)

	// First call returns expired entry, subsequent calls return nil (for ValidateAndDelete)
	mockQuery.On("First", mock.AnythingOfType("*models.PublicKeyCache")).Run(func(args mock.Arguments) {
		cache := args.Get(0).(*models.PublicKeyCache)
		cache.ActorURL = actorURL
		cache.TTL = expiredTTL
		cache.KeyID = "key123"
		cache.PublicKeyPEM = "-----BEGIN PUBLIC KEY-----"
		cache.Algorithm = "rsa-sha256"
		cache.PK = "PUBKEY_CACHE#https://mastodon.social/users/alice"
		cache.SK = "KEY"
	}).Return(nil).Once()

	// Second First call from ValidateAndDelete
	mockQuery.On("First", mock.AnythingOfType("*models.PublicKeyCache")).Return(nil).Maybe()
	mockQuery.On("Delete").Return(nil).Maybe()

	// Execute
	result, err := repo.GetByActorURL(ctx, actorURL)

	// Assert - should return error for expired entry
	require.Error(t, err)
	assert.Nil(t, result)

	mockDB.AssertExpectations(t)
}

func TestPublicKeyCacheRepository_GetByActorURL_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewPublicKeyCacheRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()
	actorURL := "https://mastodon.social/users/alice"

	// Create a valid cache entry (TTL in the future)
	validTTL := time.Now().Add(24 * time.Hour).Unix()

	// Set up mock expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.PublicKeyCache")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "PUBKEY_CACHE#https://mastodon.social/users/alice").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "KEY").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.PublicKeyCache")).Run(func(args mock.Arguments) {
		cache := args.Get(0).(*models.PublicKeyCache)
		cache.ActorURL = actorURL
		cache.TTL = validTTL
		cache.KeyID = "key123"
		cache.PublicKeyPEM = "-----BEGIN PUBLIC KEY-----\nMIIBIjANBg..."
		cache.Algorithm = "rsa-sha256"
		cache.PK = "PUBKEY_CACHE#https://mastodon.social/users/alice"
		cache.SK = "KEY"
	}).Return(nil)

	// Execute
	result, err := repo.GetByActorURL(ctx, actorURL)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, actorURL, result.ActorURL)
	assert.Equal(t, "key123", result.KeyID)
	assert.Equal(t, "rsa-sha256", result.Algorithm)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ============================================================================
// Store Tests
// ============================================================================

func TestPublicKeyCacheRepository_Store_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewPublicKeyCacheRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()
	actorURL := "https://mastodon.social/users/bob"
	keyID := "https://mastodon.social/users/bob#main-key"
	publicKeyPEM := "-----BEGIN PUBLIC KEY-----\nMIIBIjANBg..."
	algorithm := "rsa-sha256"

	// Set up mock for ValidateAndCreate which uses the base repository
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.PublicKeyCache")).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	// Execute
	result, err := repo.Store(ctx, actorURL, keyID, publicKeyPEM, algorithm)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, actorURL, result.ActorURL)
	assert.Equal(t, keyID, result.KeyID)
	assert.Equal(t, publicKeyPEM, result.PublicKeyPEM)
	assert.Equal(t, algorithm, result.Algorithm)
	// TTL should be set in the future
	assert.Greater(t, result.TTL, time.Now().Unix())

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestPublicKeyCacheRepository_Store_CreateError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewPublicKeyCacheRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()
	actorURL := "https://mastodon.social/users/bob"
	testErr := errors.New("create failed")

	// Set up mock for ValidateAndCreate
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.PublicKeyCache")).Return(mockQuery)
	mockQuery.On("Create").Return(testErr)

	// Execute
	result, err := repo.Store(ctx, actorURL, "keyId", "pem", "algo")

	// Assert
	require.Error(t, err)
	assert.Nil(t, result)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ============================================================================
// UpdateStats Tests
// ============================================================================

func TestPublicKeyCacheRepository_UpdateStats_NotFound_NoOp(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewPublicKeyCacheRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()
	actorURL := "https://mastodon.social/users/nonexistent"

	// Set up mock expectations - First returns not found
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.PublicKeyCache")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "PUBKEY_CACHE#https://mastodon.social/users/nonexistent").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "KEY").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.PublicKeyCache")).Return(dynamormerrors.ErrItemNotFound)

	// Execute - not found should NOT return an error, it's a noop
	err := repo.UpdateStats(ctx, actorURL, true)

	// Assert - should return nil for not found
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestPublicKeyCacheRepository_UpdateStats_GetError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewPublicKeyCacheRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()
	actorURL := "https://mastodon.social/users/alice"
	testErr := errors.New("database error")

	// Set up mock expectations
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.PublicKeyCache")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "PUBKEY_CACHE#https://mastodon.social/users/alice").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "KEY").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.PublicKeyCache")).Return(testErr)

	// Execute
	err := repo.UpdateStats(ctx, actorURL, true)

	// Assert
	require.Error(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestPublicKeyCacheRepository_UpdateStats_Success_IncrementSuccess(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)
	logger := zap.NewNop()
	repo := NewPublicKeyCacheRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()
	actorURL := "https://mastodon.social/users/alice"
	validTTL := time.Now().Add(24 * time.Hour).Unix()

	// Set up mock expectations for Get
	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.PublicKeyCache")).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "PUBKEY_CACHE#https://mastodon.social/users/alice").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "KEY").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.PublicKeyCache")).Run(func(args mock.Arguments) {
		cache := args.Get(0).(*models.PublicKeyCache)
		cache.ActorURL = actorURL
		cache.TTL = validTTL
		cache.SuccessCount = 5
		cache.FailureCount = 2
		cache.PK = "PUBKEY_CACHE#https://mastodon.social/users/alice"
		cache.SK = "KEY"
	}).Return(nil)

	// Set up mock expectations for Update
	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.PublicKeyCache")).Return(mockUpdateQuery).Once()
	mockUpdateQuery.On("UpdateBuilder").Return(mockUpdateBuilder).Once()
	mockUpdateBuilder.On("Set", "SuccessCount", 6).Return(mockUpdateBuilder).Once()
	mockUpdateBuilder.On("Set", "FailureCount", 2).Return(mockUpdateBuilder).Once()
	mockUpdateBuilder.On("Set", "LastUsed", mock.AnythingOfType("time.Time")).Return(mockUpdateBuilder).Once()
	mockUpdateBuilder.On("Execute").Return(nil).Once()

	// Execute - success == true
	err := repo.UpdateStats(ctx, actorURL, true)

	// Assert
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
	mockUpdateQuery.AssertExpectations(t)
	mockUpdateBuilder.AssertExpectations(t)
}

func TestPublicKeyCacheRepository_UpdateStats_Success_IncrementFailure(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)
	logger := zap.NewNop()
	repo := NewPublicKeyCacheRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()
	actorURL := "https://mastodon.social/users/alice"
	validTTL := time.Now().Add(24 * time.Hour).Unix()

	// Set up mock expectations for Get
	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.PublicKeyCache")).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "PUBKEY_CACHE#https://mastodon.social/users/alice").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "KEY").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.PublicKeyCache")).Run(func(args mock.Arguments) {
		cache := args.Get(0).(*models.PublicKeyCache)
		cache.ActorURL = actorURL
		cache.TTL = validTTL
		cache.SuccessCount = 5
		cache.FailureCount = 2
		cache.PK = "PUBKEY_CACHE#https://mastodon.social/users/alice"
		cache.SK = "KEY"
	}).Return(nil)

	// Set up mock expectations for Update
	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.PublicKeyCache")).Return(mockUpdateQuery).Once()
	mockUpdateQuery.On("UpdateBuilder").Return(mockUpdateBuilder).Once()
	mockUpdateBuilder.On("Set", "SuccessCount", 5).Return(mockUpdateBuilder).Once()
	mockUpdateBuilder.On("Set", "FailureCount", 3).Return(mockUpdateBuilder).Once()
	mockUpdateBuilder.On("Execute").Return(nil).Once()

	// Execute - success == false (increment failure)
	err := repo.UpdateStats(ctx, actorURL, false)

	// Assert
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
	mockUpdateQuery.AssertExpectations(t)
	mockUpdateBuilder.AssertExpectations(t)
}

func TestPublicKeyCacheRepository_UpdateStats_UpdateError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)
	logger := zap.NewNop()
	repo := NewPublicKeyCacheRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()
	actorURL := "https://mastodon.social/users/alice"
	validTTL := time.Now().Add(24 * time.Hour).Unix()
	testErr := errors.New("update failed")

	// Set up mock expectations for Get
	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.PublicKeyCache")).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "PUBKEY_CACHE#https://mastodon.social/users/alice").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "KEY").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.PublicKeyCache")).Run(func(args mock.Arguments) {
		cache := args.Get(0).(*models.PublicKeyCache)
		cache.ActorURL = actorURL
		cache.TTL = validTTL
		cache.PK = "PUBKEY_CACHE#https://mastodon.social/users/alice"
		cache.SK = "KEY"
	}).Return(nil)

	// Set up mock expectations for Update - returns error
	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.PublicKeyCache")).Return(mockUpdateQuery).Once()
	mockUpdateQuery.On("UpdateBuilder").Return(mockUpdateBuilder).Once()
	mockUpdateBuilder.On("Set", "SuccessCount", 1).Return(mockUpdateBuilder).Once()
	mockUpdateBuilder.On("Set", "FailureCount", 0).Return(mockUpdateBuilder).Once()
	mockUpdateBuilder.On("Set", "LastUsed", mock.AnythingOfType("time.Time")).Return(mockUpdateBuilder).Once()
	mockUpdateBuilder.On("Execute").Return(testErr).Once()

	// Execute
	err := repo.UpdateStats(ctx, actorURL, true)

	// Assert
	require.Error(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
	mockUpdateQuery.AssertExpectations(t)
	mockUpdateBuilder.AssertExpectations(t)
}

// ============================================================================
// RefreshKey Tests
// ============================================================================

func TestPublicKeyCacheRepository_RefreshKey_NotFound_FallbackToStore(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockCreateQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewPublicKeyCacheRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()
	actorURL := "https://mastodon.social/users/newuser"
	keyID := "key123"
	publicKeyPEM := "-----BEGIN PUBLIC KEY-----"
	algorithm := "rsa-sha256"

	// Set up mock expectations for Get - not found
	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.PublicKeyCache")).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "PUBKEY_CACHE#https://mastodon.social/users/newuser").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "KEY").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.PublicKeyCache")).Return(dynamormerrors.ErrItemNotFound)

	// Set up mock expectations for Store (fallback)
	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.PublicKeyCache")).Return(mockCreateQuery).Once()
	mockCreateQuery.On("Create").Return(nil)

	// Execute
	err := repo.RefreshKey(ctx, actorURL, keyID, publicKeyPEM, algorithm)

	// Assert
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
	mockCreateQuery.AssertExpectations(t)
}

func TestPublicKeyCacheRepository_RefreshKey_GetError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewPublicKeyCacheRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()
	actorURL := "https://mastodon.social/users/alice"
	testErr := errors.New("database error")

	// Set up mock expectations - First returns error
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.PublicKeyCache")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "PUBKEY_CACHE#https://mastodon.social/users/alice").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "KEY").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.PublicKeyCache")).Return(testErr)

	// Execute
	err := repo.RefreshKey(ctx, actorURL, "keyID", "pem", "algo")

	// Assert
	require.Error(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestPublicKeyCacheRepository_RefreshKey_ExistingEntry_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewPublicKeyCacheRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()
	actorURL := "https://mastodon.social/users/alice"
	keyID := "new-key-123"
	publicKeyPEM := "-----BEGIN PUBLIC KEY-----\nNEW_KEY"
	algorithm := "rsa-sha512"
	validTTL := time.Now().Add(24 * time.Hour).Unix()

	// Set up mock expectations for Get - found
	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.PublicKeyCache")).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "PUBKEY_CACHE#https://mastodon.social/users/alice").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "KEY").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.PublicKeyCache")).Run(func(args mock.Arguments) {
		cache := args.Get(0).(*models.PublicKeyCache)
		cache.ActorURL = actorURL
		cache.TTL = validTTL
		cache.KeyID = "old-key"
		cache.PublicKeyPEM = "old-pem"
		cache.Algorithm = "rsa-sha256"
		cache.FailureCount = 5 // Should be reset on refresh
		cache.PK = "PUBKEY_CACHE#https://mastodon.social/users/alice"
		cache.SK = "KEY"
	}).Return(nil)

	// Set up mock expectations for Update
	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.PublicKeyCache")).Return(mockUpdateQuery).Once()
	mockUpdateQuery.On("Where", "PK", "=", "PUBKEY_CACHE#https://mastodon.social/users/alice").Return(mockUpdateQuery)
	mockUpdateQuery.On("Where", "SK", "=", "KEY").Return(mockUpdateQuery)
	mockUpdateQuery.On("Update", mock.Anything).Return(nil)

	// Execute
	err := repo.RefreshKey(ctx, actorURL, keyID, publicKeyPEM, algorithm)

	// Assert
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
	mockUpdateQuery.AssertExpectations(t)
}

func TestPublicKeyCacheRepository_RefreshKey_UpdateError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewPublicKeyCacheRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()
	actorURL := "https://mastodon.social/users/alice"
	validTTL := time.Now().Add(24 * time.Hour).Unix()
	testErr := errors.New("update failed")

	// Set up mock expectations for Get - found
	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.PublicKeyCache")).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "PUBKEY_CACHE#https://mastodon.social/users/alice").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "KEY").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.PublicKeyCache")).Run(func(args mock.Arguments) {
		cache := args.Get(0).(*models.PublicKeyCache)
		cache.ActorURL = actorURL
		cache.TTL = validTTL
		cache.PK = "PUBKEY_CACHE#https://mastodon.social/users/alice"
		cache.SK = "KEY"
	}).Return(nil)

	// Set up mock expectations for Update - error
	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.PublicKeyCache")).Return(mockUpdateQuery).Once()
	mockUpdateQuery.On("Where", "PK", "=", "PUBKEY_CACHE#https://mastodon.social/users/alice").Return(mockUpdateQuery)
	mockUpdateQuery.On("Where", "SK", "=", "KEY").Return(mockUpdateQuery)
	mockUpdateQuery.On("Update", mock.Anything).Return(testErr)

	// Execute
	err := repo.RefreshKey(ctx, actorURL, "keyID", "pem", "algo")

	// Assert
	require.Error(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
	mockUpdateQuery.AssertExpectations(t)
}

// ============================================================================
// InvalidateCache Tests
// ============================================================================

func TestPublicKeyCacheRepository_InvalidateCache_NotFound_Ignored(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewPublicKeyCacheRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()
	actorURL := "https://mastodon.social/users/nonexistent"

	// Set up mock expectations - ValidateAndDelete calls Get (First) then Delete
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.PublicKeyCache")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.PublicKeyCache")).Return(nil)
	mockQuery.On("Delete").Return(dynamormerrors.ErrItemNotFound)

	// Execute - not found should NOT return an error
	err := repo.InvalidateCache(ctx, actorURL)

	// Assert
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestPublicKeyCacheRepository_InvalidateCache_RealError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewPublicKeyCacheRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()
	actorURL := "https://mastodon.social/users/alice"
	testErr := errors.New("connection failed")

	// Set up mock expectations - ValidateAndDelete calls Get (First) then Delete
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.PublicKeyCache")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.PublicKeyCache")).Return(nil)
	mockQuery.On("Delete").Return(testErr)

	// Execute
	err := repo.InvalidateCache(ctx, actorURL)

	// Assert
	require.Error(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestPublicKeyCacheRepository_InvalidateCache_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zap.NewNop()
	repo := NewPublicKeyCacheRepository(mockDB, "test-table", logger, nil)

	ctx := context.Background()
	actorURL := "https://mastodon.social/users/alice"

	// Set up mock expectations - ValidateAndDelete calls Get (First) then Delete
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.PublicKeyCache")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.PublicKeyCache")).Return(nil)
	mockQuery.On("Delete").Return(nil)

	// Execute
	err := repo.InvalidateCache(ctx, actorURL)

	// Assert
	require.NoError(t, err)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}
