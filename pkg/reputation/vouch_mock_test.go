package reputation

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	pkgtesting "github.com/equaltoai/lesser/pkg/testing"
	"github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// =============================================================================
// VouchManager Tests Using MockRepositoryStorage
// These tests demonstrate that VouchManager is fully testable with mocks
// Requirements: 6.1, 6.4
// =============================================================================

// TestVouchManager_CreateVouch_WithMockStorage tests CreateVouch using MockRepositoryStorage
func TestVouchManager_CreateVouch_WithMockStorage(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	instanceURL := "https://test.example.com"

	// Create a signer for testing
	signer, err := NewSigner("", instanceURL, logger)
	require.NoError(t, err)

	t.Run("successful_vouch_creation", func(t *testing.T) {
		// Create mock user repository
		mockUserRepo := mocks.NewMockUserRepositoryInterface()

		// Set up expectations
		mockUserRepo.On("GetMonthlyVouchCount", mock.Anything, "actor1", mock.AnythingOfType("int"), mock.AnythingOfType("time.Month")).
			Return(0, nil)
		mockUserRepo.On("GetReputation", mock.Anything, "actor1").
			Return(&storage.Reputation{TotalScore: 600}, nil)
		mockUserRepo.On("CreateVouch", mock.Anything, mock.AnythingOfType("*storage.Vouch")).
			Return(nil)

		// Create MockRepositoryStorage with custom user repository
		mockStorage := pkgtesting.NewMockRepositoryStorage(
			pkgtesting.WithUserRepository(mockUserRepo),
		)

		// Create VouchManager with mock storage
		vm := NewVouchManager(mockStorage, signer, instanceURL, logger)

		// Execute
		input := &CreateVouchInput{
			FromActorID: "actor1",
			ToActorID:   "actor2",
			Confidence:  0.8,
			Context:     "test vouch",
		}
		vouch, err := vm.CreateVouch(ctx, input)

		// Assert
		require.NoError(t, err)
		require.NotNil(t, vouch)
		assert.Equal(t, "actor1", vouch.From)
		assert.Equal(t, "actor2", vouch.To)
		assert.Equal(t, 0.8, vouch.Confidence)
		assert.Equal(t, "test vouch", vouch.Context)
		assert.True(t, vouch.Active)
		assert.False(t, vouch.Revoked)
		assert.NotEmpty(t, vouch.Signature)

		// Verify all expectations were met
		mockUserRepo.AssertExpectations(t)
	})

	t.Run("insufficient_reputation", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepositoryInterface()

		mockUserRepo.On("GetMonthlyVouchCount", mock.Anything, "actor1", mock.AnythingOfType("int"), mock.AnythingOfType("time.Month")).
			Return(0, nil)
		mockUserRepo.On("GetReputation", mock.Anything, "actor1").
			Return(&storage.Reputation{TotalScore: 400}, nil) // Below 500 threshold

		mockStorage := pkgtesting.NewMockRepositoryStorage(
			pkgtesting.WithUserRepository(mockUserRepo),
		)

		vm := NewVouchManager(mockStorage, signer, instanceURL, logger)

		input := &CreateVouchInput{
			FromActorID: "actor1",
			ToActorID:   "actor2",
			Confidence:  0.8,
		}
		vouch, err := vm.CreateVouch(ctx, input)

		require.Error(t, err)
		assert.Nil(t, vouch)
		assert.Contains(t, err.Error(), "insufficient reputation")
		mockUserRepo.AssertExpectations(t)
	})

	t.Run("monthly_vouch_limit_reached", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepositoryInterface()

		mockUserRepo.On("GetMonthlyVouchCount", mock.Anything, "actor1", mock.AnythingOfType("int"), mock.AnythingOfType("time.Month")).
			Return(5, nil) // At limit

		mockStorage := pkgtesting.NewMockRepositoryStorage(
			pkgtesting.WithUserRepository(mockUserRepo),
		)

		vm := NewVouchManager(mockStorage, signer, instanceURL, logger)

		input := &CreateVouchInput{
			FromActorID: "actor1",
			ToActorID:   "actor2",
			Confidence:  0.8,
		}
		vouch, err := vm.CreateVouch(ctx, input)

		require.Error(t, err)
		assert.Nil(t, vouch)
		assert.Contains(t, err.Error(), "monthly vouch limit reached")
		mockUserRepo.AssertExpectations(t)
	})

	t.Run("invalid_confidence", func(t *testing.T) {
		mockStorage := pkgtesting.NewMockRepositoryStorage()
		vm := NewVouchManager(mockStorage, signer, instanceURL, logger)

		input := &CreateVouchInput{
			FromActorID: "actor1",
			ToActorID:   "actor2",
			Confidence:  1.5, // Invalid: > 1
		}
		vouch, err := vm.CreateVouch(ctx, input)

		require.Error(t, err)
		assert.Nil(t, vouch)
		assert.Contains(t, err.Error(), "confidence must be between 0 and 1")
	})
}

// TestVouchManager_RevokeVouch_WithMockStorage tests RevokeVouch using MockRepositoryStorage
func TestVouchManager_RevokeVouch_WithMockStorage(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	instanceURL := "https://test.example.com"

	signer, err := NewSigner("", instanceURL, logger)
	require.NoError(t, err)

	t.Run("successful_revocation", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepositoryInterface()

		expiresAt := time.Now().Add(180 * 24 * time.Hour)
		mockUserRepo.On("GetVouch", mock.Anything, "vouch123").
			Return(&storage.Vouch{
				ID:        "vouch123",
				From:      "actor1",
				To:        "actor2",
				Active:    true,
				ExpiresAt: &expiresAt,
			}, nil)
		mockUserRepo.On("UpdateVouchStatus", mock.Anything, "vouch123", false, mock.AnythingOfType("*time.Time")).
			Return(nil)

		mockStorage := pkgtesting.NewMockRepositoryStorage(
			pkgtesting.WithUserRepository(mockUserRepo),
		)

		vm := NewVouchManager(mockStorage, signer, instanceURL, logger)

		err := vm.RevokeVouch(ctx, "vouch123", "actor1")

		require.NoError(t, err)
		mockUserRepo.AssertExpectations(t)
	})

	t.Run("unauthorized_revocation", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepositoryInterface()

		expiresAt := time.Now().Add(180 * 24 * time.Hour)
		mockUserRepo.On("GetVouch", mock.Anything, "vouch123").
			Return(&storage.Vouch{
				ID:        "vouch123",
				From:      "actor1", // Original voucher
				To:        "actor2",
				Active:    true,
				ExpiresAt: &expiresAt,
			}, nil)

		mockStorage := pkgtesting.NewMockRepositoryStorage(
			pkgtesting.WithUserRepository(mockUserRepo),
		)

		vm := NewVouchManager(mockStorage, signer, instanceURL, logger)

		// Try to revoke as different actor
		err := vm.RevokeVouch(ctx, "vouch123", "actor3")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "only the voucher can revoke")
		mockUserRepo.AssertExpectations(t)
	})

	t.Run("vouch_not_found", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepositoryInterface()

		mockUserRepo.On("GetVouch", mock.Anything, "nonexistent").
			Return(nil, storage.ErrNotFound)

		mockStorage := pkgtesting.NewMockRepositoryStorage(
			pkgtesting.WithUserRepository(mockUserRepo),
		)

		vm := NewVouchManager(mockStorage, signer, instanceURL, logger)

		err := vm.RevokeVouch(ctx, "nonexistent", "actor1")

		require.Error(t, err)
		mockUserRepo.AssertExpectations(t)
	})
}

// TestVouchManager_GetVouchByID_WithMockStorage tests GetVouchByID using MockRepositoryStorage
func TestVouchManager_GetVouchByID_WithMockStorage(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	instanceURL := "https://test.example.com"

	signer, err := NewSigner("", instanceURL, logger)
	require.NoError(t, err)

	t.Run("successful_retrieval", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepositoryInterface()

		expiresAt := time.Now().Add(180 * 24 * time.Hour)
		createdAt := time.Now().Add(-24 * time.Hour)
		mockUserRepo.On("GetVouch", mock.Anything, "vouch123").
			Return(&storage.Vouch{
				ID:                "vouch123",
				From:              "actor1",
				To:                "actor2",
				CreatedAt:         createdAt,
				ExpiresAt:         &expiresAt,
				Confidence:        0.9,
				Context:           "trusted contributor",
				VoucherReputation: 750,
				Active:            true,
				Revoked:           false,
				Signature:         "sig123",
			}, nil)

		mockStorage := pkgtesting.NewMockRepositoryStorage(
			pkgtesting.WithUserRepository(mockUserRepo),
		)

		vm := NewVouchManager(mockStorage, signer, instanceURL, logger)

		vouch, err := vm.GetVouchByID(ctx, "vouch123")

		require.NoError(t, err)
		require.NotNil(t, vouch)
		assert.Equal(t, "vouch123", vouch.ID)
		assert.Equal(t, "actor1", vouch.From)
		assert.Equal(t, "actor2", vouch.To)
		assert.Equal(t, 0.9, vouch.Confidence)
		assert.Equal(t, "trusted contributor", vouch.Context)
		assert.Equal(t, 750, vouch.VoucherReputation)
		assert.True(t, vouch.Active)
		assert.False(t, vouch.Revoked)
		mockUserRepo.AssertExpectations(t)
	})

	t.Run("vouch_not_found", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepositoryInterface()

		mockUserRepo.On("GetVouch", mock.Anything, "nonexistent").
			Return(nil, nil) // Returns nil vouch

		mockStorage := pkgtesting.NewMockRepositoryStorage(
			pkgtesting.WithUserRepository(mockUserRepo),
		)

		vm := NewVouchManager(mockStorage, signer, instanceURL, logger)

		vouch, err := vm.GetVouchByID(ctx, "nonexistent")

		require.Error(t, err)
		assert.Nil(t, vouch)
		assert.Contains(t, err.Error(), "vouch not found")
		mockUserRepo.AssertExpectations(t)
	})
}

// TestVouchManager_WithInMemoryStorage tests VouchManager with default in-memory storage
func TestVouchManager_WithInMemoryStorage(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	instanceURL := "https://test.example.com"

	signer, err := NewSigner("", instanceURL, logger)
	require.NoError(t, err)

	t.Run("full_vouch_lifecycle", func(t *testing.T) {
		// Use default in-memory storage
		mockStorage := pkgtesting.NewMockRepositoryStorage()

		// Pre-populate reputation for the voucher
		err := mockStorage.User().StoreReputation(ctx, "actor1", &storage.Reputation{
			TotalScore: 600,
		})
		require.NoError(t, err)

		vm := NewVouchManager(mockStorage, signer, instanceURL, logger)

		// Create vouch
		input := &CreateVouchInput{
			FromActorID: "actor1",
			ToActorID:   "actor2",
			Confidence:  0.85,
			Context:     "great contributor",
		}
		vouch, err := vm.CreateVouch(ctx, input)
		require.NoError(t, err)
		require.NotNil(t, vouch)

		// Retrieve vouch
		retrieved, err := vm.GetVouchByID(ctx, vouch.ID)
		require.NoError(t, err)
		assert.Equal(t, vouch.ID, retrieved.ID)
		assert.Equal(t, "actor1", retrieved.From)
		assert.Equal(t, "actor2", retrieved.To)

		// Revoke vouch
		err = vm.RevokeVouch(ctx, vouch.ID, "actor1")
		require.NoError(t, err)

		// Verify revocation
		revoked, err := vm.GetVouchByID(ctx, vouch.ID)
		require.NoError(t, err)
		assert.False(t, revoked.Active)
		assert.True(t, revoked.Revoked)
	})
}

// TestVouchManager_GetVouchesForActor_WithMockStorage tests GetVouchesForActor
func TestVouchManager_GetVouchesForActor_WithMockStorage(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	instanceURL := "https://test.example.com"

	signer, err := NewSigner("", instanceURL, logger)
	require.NoError(t, err)

	t.Run("returns_active_vouches", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepositoryInterface()

		expiresAt := time.Now().Add(180 * 24 * time.Hour)
		mockUserRepo.On("GetVouchesForActor", mock.Anything, "actor2", true).
			Return([]*storage.Vouch{
				{
					ID:         "vouch1",
					From:       "actor1",
					To:         "actor2",
					Active:     true,
					ExpiresAt:  &expiresAt,
					Confidence: 0.8,
				},
				{
					ID:         "vouch2",
					From:       "actor3",
					To:         "actor2",
					Active:     true,
					ExpiresAt:  &expiresAt,
					Confidence: 0.9,
				},
			}, nil)

		mockStorage := pkgtesting.NewMockRepositoryStorage(
			pkgtesting.WithUserRepository(mockUserRepo),
		)

		vm := NewVouchManager(mockStorage, signer, instanceURL, logger)

		vouches, err := vm.GetVouchesForActor(ctx, "actor2")

		require.NoError(t, err)
		assert.Len(t, vouches, 2)
		mockUserRepo.AssertExpectations(t)
	})
}

// TestVouchManager_GetVouchesFromActor_WithMockStorage tests GetVouchesFromActor
func TestVouchManager_GetVouchesFromActor_WithMockStorage(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	instanceURL := "https://test.example.com"

	signer, err := NewSigner("", instanceURL, logger)
	require.NoError(t, err)

	t.Run("returns_all_vouches_by_actor", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepositoryInterface()

		expiresAt := time.Now().Add(180 * 24 * time.Hour)
		mockUserRepo.On("GetVouchesByActor", mock.Anything, "actor1", false).
			Return([]*storage.Vouch{
				{
					ID:         "vouch1",
					From:       "actor1",
					To:         "actor2",
					Active:     true,
					ExpiresAt:  &expiresAt,
					Confidence: 0.8,
				},
				{
					ID:         "vouch2",
					From:       "actor1",
					To:         "actor3",
					Active:     false,
					Revoked:    true,
					ExpiresAt:  &expiresAt,
					Confidence: 0.7,
				},
			}, nil)

		mockStorage := pkgtesting.NewMockRepositoryStorage(
			pkgtesting.WithUserRepository(mockUserRepo),
		)

		vm := NewVouchManager(mockStorage, signer, instanceURL, logger)

		vouches, err := vm.GetVouchesFromActor(ctx, "actor1")

		require.NoError(t, err)
		assert.Len(t, vouches, 2)
		mockUserRepo.AssertExpectations(t)
	})
}
