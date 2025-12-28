package repositories

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	dynamormErrors "github.com/pay-theory/dynamorm/pkg/errors"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

type testEncryptor struct {
	decryptErr bool
}

func (e testEncryptor) Encrypt(plaintext []byte) ([]byte, error) {
	encrypted := make([]byte, len(plaintext))
	for i, b := range plaintext {
		encrypted[i] = b ^ 0xff
	}
	return encrypted, nil
}

func (e testEncryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	if e.decryptErr {
		return nil, errors.New("decrypt failed")
	}
	decrypted := make([]byte, len(ciphertext))
	for i, b := range ciphertext {
		decrypted[i] = b ^ 0xff
	}
	return decrypted, nil
}

func TestAccountRepository_CreateActor_EncryptorSuccessAndConflict(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 8, 9, 10, 11, 12, 0, time.UTC)

	t.Run("success", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		repo.SetEncryptor(testEncryptor{})

		err := repo.createActor(ctx, &activitypub.Actor{
			PreferredUsername: "alice",
			BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/alice"},
			Name:              "Alice",
		}, "private-key")
		require.NoError(t, err)
	})

	t.Run("conflict", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Create").Return(dynamormErrors.ErrConditionFailed).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		repo.SetEncryptor(testEncryptor{})

		err := repo.createActor(ctx, &activitypub.Actor{
			PreferredUsername: "alice",
			BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/alice"},
			Name:              "Alice",
		}, "private-key")
		require.Error(t, err)
	})
}

func TestAccountRepository_GetActorPrivateKey_DecryptPaths(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 8, 9, 10, 11, 12, 0, time.UTC)

	t.Run("success", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		enc := testEncryptor{}
		encrypted, err := enc.Encrypt([]byte("private-key"))
		require.NoError(t, err)

		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.Actor)
			dest.PrivateKey = base64.StdEncoding.EncodeToString(encrypted)
		}).Return(nil).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		repo.SetEncryptor(enc)

		privateKey, err := repo.GetActorPrivateKey(ctx, "alice")
		require.NoError(t, err)
		require.Equal(t, "private-key", privateKey)
	})

	t.Run("invalid base64", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.Actor)
			dest.PrivateKey = "not-base64"
		}).Return(nil).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		repo.SetEncryptor(testEncryptor{})

		privateKey, err := repo.GetActorPrivateKey(ctx, "alice")
		require.Error(t, err)
		require.Empty(t, privateKey)
	})

	t.Run("decrypt error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*models.Actor)
			dest.PrivateKey = base64.StdEncoding.EncodeToString([]byte("ciphertext"))
		}).Return(nil).Once()
		setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, nil, baseTime)

		repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
		repo.SetEncryptor(testEncryptor{decryptErr: true})

		privateKey, err := repo.GetActorPrivateKey(ctx, "alice")
		require.Error(t, err)
		require.Empty(t, privateKey)
	})
}

func TestAccountRepository_CreateAccount_WithActorAndEncryptor(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 8, 9, 10, 11, 12, 0, time.UTC)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)
	setupPermissiveAccountRepositoryMocks(mockDB, mockQuery, mockUpdateBuilder, baseTime)

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	repo.SetEncryptor(testEncryptor{})

	err := repo.CreateAccount(ctx, &storage.Account{
		User: &storage.User{
			Username: "alice",
			Role:     "user",
		},
		Actor: &activitypub.Actor{
			PreferredUsername: "alice",
			BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/alice"},
			Name:              "Alice",
		},
		PrivateKey: "private-key",
	})
	require.NoError(t, err)
}

func TestAccountRepository_CreateActor_ValidationError(t *testing.T) {
	ctx := context.Background()
	repo := &AccountRepository{}

	err := repo.createActor(ctx, &activitypub.Actor{}, "private-key")
	require.Error(t, err)
}
