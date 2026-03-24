package repositories

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
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

func TestAccountRepository_CreateActor_PreparesActorAndNumericIDMapping(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	var currentModel any
	var createdActor *models.Actor
	var createdMapping *models.NumericIDMapping

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Run(func(args mock.Arguments) {
		currentModel = args.Get(0)
	}).Return(mockQuery).Maybe()

	mockQuery.On("Create").Run(func(mock.Arguments) {
		switch model := currentModel.(type) {
		case *models.Actor:
			copyActor := *model
			createdActor = &copyActor
		case *models.NumericIDMapping:
			copyMapping := *model
			createdMapping = &copyMapping
		}
	}).Return(nil).Maybe()

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	repo.SetEncryptor(testEncryptor{})

	err := repo.createActor(ctx, &activitypub.Actor{
		PreferredUsername: "alice",
		BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/alice"},
		Name:              "Alice",
	}, "private-key")
	require.NoError(t, err)

	require.NotNil(t, createdActor)
	require.Equal(t, "ACTOR#alice", createdActor.PK)
	require.Equal(t, models.SKProfile, createdActor.SK)
	require.Equal(t, "DOMAIN#example.com", createdActor.GSI3PK)
	require.Equal(t, "alice", createdActor.GSI3SK)
	require.False(t, createdActor.CreatedAt.IsZero())
	require.False(t, createdActor.UpdatedAt.IsZero())

	require.NotNil(t, createdMapping)
	require.Equal(t, "NUMERIC_ID#"+common.GenerateNumericID("alice"), createdMapping.PK)
	require.Equal(t, models.SKMetadata, createdMapping.SK)
	require.Equal(t, "NumericIDMapping", createdMapping.Type)
	require.Equal(t, "alice", createdMapping.Username)
	require.False(t, createdMapping.CreatedAt.IsZero())
}

func TestAccountRepository_CreateActor_NormalizesMixedCaseNumericIdentity(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	var currentModel any
	var createdActor *models.Actor
	var createdMapping *models.NumericIDMapping

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Run(func(args mock.Arguments) {
		currentModel = args.Get(0)
	}).Return(mockQuery).Maybe()

	mockQuery.On("Create").Run(func(mock.Arguments) {
		switch model := currentModel.(type) {
		case *models.Actor:
			copyActor := *model
			createdActor = &copyActor
		case *models.NumericIDMapping:
			copyMapping := *model
			createdMapping = &copyMapping
		}
	}).Return(nil).Maybe()

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zaptest.NewLogger(t))
	repo.SetEncryptor(testEncryptor{})

	err := repo.createActor(ctx, &activitypub.Actor{
		PreferredUsername: "Agent-0",
		BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/Agent-0"},
		Name:              "Agent 0",
	}, "private-key")
	require.NoError(t, err)

	require.NotNil(t, createdActor)
	require.Equal(t, "ACTOR#agent-0", createdActor.PK)
	require.Equal(t, common.GenerateNumericID("agent-0"), createdActor.NumericID)
	require.NotNil(t, createdActor.Actor)
	require.Equal(t, "agent-0", createdActor.Actor.PreferredUsername)
	require.Equal(t, "https://example.com/users/agent-0", createdActor.Actor.ID)

	require.NotNil(t, createdMapping)
	require.Equal(t, common.GenerateNumericID("agent-0"), createdMapping.NumericID)
	require.Equal(t, "agent-0", createdMapping.Username)
	require.Equal(t, "https://example.com/users/agent-0", createdMapping.ActorID)
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
