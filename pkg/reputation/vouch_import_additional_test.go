package reputation

import (
	"context"
	"errors"
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

type stubVouchVerifier struct {
	verifyFn func(v *Vouch) (bool, error)
}

func (s stubVouchVerifier) VerifyVouchSignature(v *Vouch) (bool, error) {
	if s.verifyFn == nil {
		return true, nil
	}
	return s.verifyFn(v)
}

func TestVouchManager_ImportVouch_ValidationAndVerificationFailures(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	vm := NewVouchManager(nil, nil, "https://local.example", logger)
	verifier := stubVouchVerifier{}

	t.Run("nil vouch rejected", func(t *testing.T) {
		err := vm.ImportVouch(context.Background(), nil, verifier)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "vouch cannot be nil")
	})

	t.Run("inactive rejected", func(t *testing.T) {
		err := vm.ImportVouch(context.Background(), &Vouch{
			ID:        "v1",
			Active:    false,
			Revoked:   false,
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}, verifier)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not active")
	})

	t.Run("revoked rejected", func(t *testing.T) {
		err := vm.ImportVouch(context.Background(), &Vouch{
			ID:        "v1",
			Active:    true,
			Revoked:   true,
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}, verifier)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "revoked")
	})

	t.Run("expired rejected", func(t *testing.T) {
		err := vm.ImportVouch(context.Background(), &Vouch{
			ID:        "v1",
			Active:    true,
			Revoked:   false,
			ExpiresAt: time.Now().Add(-1 * time.Second),
		}, verifier)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "expired")
	})

	t.Run("signature verifier error surfaces", func(t *testing.T) {
		vm := NewVouchManager(nil, nil, "https://local.example", logger)
		verifier := stubVouchVerifier{
			verifyFn: func(*Vouch) (bool, error) {
				return false, errors.New("boom")
			},
		}

		err := vm.ImportVouch(context.Background(), &Vouch{
			ID:                "v1",
			Active:            true,
			Revoked:           false,
			ExpiresAt:         time.Now().Add(24 * time.Hour),
			VoucherReputation: 600,
			Signature:         "sig",
		}, verifier)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to verify vouch signature")
	})

	t.Run("invalid signature rejected", func(t *testing.T) {
		vm := NewVouchManager(nil, nil, "https://local.example", logger)
		verifier := stubVouchVerifier{
			verifyFn: func(*Vouch) (bool, error) {
				return false, nil
			},
		}

		err := vm.ImportVouch(context.Background(), &Vouch{
			ID:                "v1",
			Active:            true,
			Revoked:           false,
			ExpiresAt:         time.Now().Add(24 * time.Hour),
			VoucherReputation: 600,
			Signature:         "sig",
		}, verifier)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "signature is invalid")
	})
}

func TestVouchManager_ImportVouch_DuplicateAndStorePaths(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := zap.NewNop()
	instanceURL := "https://local.example"

	t.Run("duplicate vouch is ignored", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepositoryInterface()
		mockUserRepo.On("GetVouch", mock.Anything, "vouch1").
			Return(&storage.Vouch{ID: "vouch1", From: "actor1", To: "actor2", Active: true}, nil)

		mockStorage := pkgtesting.NewMockRepositoryStorage(
			pkgtesting.WithUserRepository(mockUserRepo),
		)
		vm := NewVouchManager(mockStorage, nil, instanceURL, logger)

		err := vm.ImportVouch(ctx, &Vouch{
			ID:                "vouch1",
			From:              "actor1",
			To:                "actor2",
			InstanceURL:       "https://remote.example",
			Active:            true,
			Revoked:           false,
			ExpiresAt:         time.Now().Add(24 * time.Hour),
			VoucherReputation: 600,
			Signature:         "sig",
		}, stubVouchVerifier{})
		require.NoError(t, err)

		mockUserRepo.AssertExpectations(t)
	})

	t.Run("insufficient voucher reputation rejected", func(t *testing.T) {
		mockStorage := pkgtesting.NewMockRepositoryStorage()
		vm := NewVouchManager(mockStorage, nil, instanceURL, logger)

		err := vm.ImportVouch(ctx, &Vouch{
			ID:                "vouch-low",
			Active:            true,
			Revoked:           false,
			ExpiresAt:         time.Now().Add(24 * time.Hour),
			VoucherReputation: 499,
			Signature:         "sig",
		}, stubVouchVerifier{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "insufficient reputation")
	})

	t.Run("stores when not found", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepositoryInterface()

		mockUserRepo.On("GetVouch", mock.Anything, "vouch2").
			Return(nil, storage.ErrNotFound)
		mockUserRepo.On("CreateVouch", mock.Anything, mock.AnythingOfType("*storage.Vouch")).
			Return(nil)

		mockStorage := pkgtesting.NewMockRepositoryStorage(
			pkgtesting.WithUserRepository(mockUserRepo),
		)
		vm := NewVouchManager(mockStorage, nil, instanceURL, logger)

		err := vm.ImportVouch(ctx, &Vouch{
			ID:                "vouch2",
			From:              "actor1",
			To:                "actor2",
			InstanceURL:       "https://remote.example",
			CreatedAt:         time.Now().Add(-time.Hour),
			ExpiresAt:         time.Now().Add(24 * time.Hour),
			Confidence:        0.9,
			Context:           "friend",
			VoucherReputation: 600,
			Active:            true,
			Revoked:           false,
			Signature:         "sig",
		}, stubVouchVerifier{})
		require.NoError(t, err)

		mockUserRepo.AssertExpectations(t)
	})

	t.Run("create duplicate error is ignored", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepositoryInterface()

		mockUserRepo.On("GetVouch", mock.Anything, "vouch3").
			Return(nil, storage.ErrNotFound)
		mockUserRepo.On("CreateVouch", mock.Anything, mock.AnythingOfType("*storage.Vouch")).
			Return(errors.New("vouch already exists"))

		mockStorage := pkgtesting.NewMockRepositoryStorage(
			pkgtesting.WithUserRepository(mockUserRepo),
		)
		vm := NewVouchManager(mockStorage, nil, instanceURL, logger)

		err := vm.ImportVouch(ctx, &Vouch{
			ID:                "vouch3",
			Active:            true,
			Revoked:           false,
			ExpiresAt:         time.Now().Add(24 * time.Hour),
			VoucherReputation: 600,
			Signature:         "sig",
		}, stubVouchVerifier{})
		require.NoError(t, err)

		mockUserRepo.AssertExpectations(t)
	})

	t.Run("create other error surfaces", func(t *testing.T) {
		mockUserRepo := mocks.NewMockUserRepositoryInterface()

		mockUserRepo.On("GetVouch", mock.Anything, "vouch4").
			Return(nil, storage.ErrNotFound)
		mockUserRepo.On("CreateVouch", mock.Anything, mock.AnythingOfType("*storage.Vouch")).
			Return(errors.New("write failed"))

		mockStorage := pkgtesting.NewMockRepositoryStorage(
			pkgtesting.WithUserRepository(mockUserRepo),
		)
		vm := NewVouchManager(mockStorage, nil, instanceURL, logger)

		err := vm.ImportVouch(ctx, &Vouch{
			ID:                "vouch4",
			Active:            true,
			Revoked:           false,
			ExpiresAt:         time.Now().Add(24 * time.Hour),
			VoucherReputation: 600,
			Signature:         "sig",
		}, stubVouchVerifier{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to store imported vouch")

		mockUserRepo.AssertExpectations(t)
	})
}

func TestVouchManager_ImportVouches_ContinuesOnErrors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := zap.NewNop()
	instanceURL := "https://local.example"

	mockUserRepo := mocks.NewMockUserRepositoryInterface()

	mockUserRepo.On("GetVouch", mock.Anything, "ok-1").Return(nil, storage.ErrNotFound)
	mockUserRepo.On("CreateVouch", mock.Anything, mock.AnythingOfType("*storage.Vouch")).Return(nil).Once()

	mockUserRepo.On("GetVouch", mock.Anything, "ok-2").Return(nil, storage.ErrNotFound)
	mockUserRepo.On("CreateVouch", mock.Anything, mock.AnythingOfType("*storage.Vouch")).Return(nil).Once()

	mockStorage := pkgtesting.NewMockRepositoryStorage(
		pkgtesting.WithUserRepository(mockUserRepo),
	)
	vm := NewVouchManager(mockStorage, nil, instanceURL, logger)

	verifier := stubVouchVerifier{
		verifyFn: func(v *Vouch) (bool, error) {
			if v.ID == "bad" {
				return false, nil
			}
			return true, nil
		},
	}

	imported, err := vm.ImportVouches(ctx, []Vouch{
		{
			ID:                "ok-1",
			Active:            true,
			Revoked:           false,
			ExpiresAt:         time.Now().Add(24 * time.Hour),
			VoucherReputation: 600,
			Signature:         "sig",
		},
		{
			ID:                "bad",
			Active:            true,
			Revoked:           false,
			ExpiresAt:         time.Now().Add(24 * time.Hour),
			VoucherReputation: 600,
			Signature:         "sig",
		},
		{
			ID:                "ok-2",
			Active:            true,
			Revoked:           false,
			ExpiresAt:         time.Now().Add(24 * time.Hour),
			VoucherReputation: 600,
			Signature:         "sig",
		},
	}, verifier)
	require.NoError(t, err)
	assert.Equal(t, 2, imported)

	mockUserRepo.AssertExpectations(t)
}
