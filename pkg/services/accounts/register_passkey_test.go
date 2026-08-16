package accounts

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestService_RegisterAccount_WithPasskeyRegistrationProof_Succeeds(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	baseTime := time.Now().UTC()
	tableName := "test-table"

	db := newPermissiveDynamormDB(t, permissiveDBOptions{forceUserNotFound: true})
	accountRepo := repositories.NewAccountRepository(db, tableName, "example.com", logger)
	accountRepo.SetEncryptor(noopEncryptor{})
	accountRepo.SetPermissionService(nil)
	accountRepo.SetEventService(nil)
	accountRepo.SetCachingService(nil)

	storageImpl := &permissiveAccountsStorage{
		MockRepositoryStorage: NewMockRepositoryStorage(),
		db:                    db,
		tableName:             tableName,
		logger:                logger,
		account:               accountRepo,
		actor:                 repositories.NewActorRepository(db, tableName, logger),
		relationship:          repositories.NewRelationshipRepository(db, tableName, logger),
		social:                repositories.NewSocialRepository(db, tableName, logger, nil),
		user:                  repositories.NewUserRepository(db, tableName, logger),
		marker:                repositories.NewMarkerRepository(db, tableName, logger, nil),
		analytics:             repositories.NewTrendingRepository(db, logger, nil),
		instance:              repositories.NewInstanceRepository(db, tableName, logger),
		domainBlock:           repositories.NewDomainBlockRepository(db, tableName, logger),
		quote:                 repositories.NewQuoteRepository(db, tableName, logger, nil),
		activity:              repositories.NewActivityRepository(db, tableName, logger, nil),
	}

	require.NoError(t, accountRepo.StorePasskeyRegistrationProof(ctx, &models.PasskeyRegistrationProof{
		ID:              "proof-1",
		Username:        "alice",
		CeremonyID:      "ceremony-1",
		CredentialID:    "cred-1",
		PublicKey:       []byte("public-key"),
		AttestationType: "packed",
		AAGUID:          []byte("aaguid"),
		SignCount:       11,
		BackupEligible:  true,
		BackupState:     true,
		CreatedAt:       baseTime,
		ExpiresAt:       baseTime.Add(time.Hour),
	}))

	cryptoSvc := staticCryptoService{
		publicKeyPEM:  []byte("PUBLIC KEY"),
		privateKeyPEM: []byte("PRIVATE KEY"),
		key:           struct{}{},
	}

	svc := NewService(storageImpl, streaming.NewMockPublisher(), nil, cryptoSvc, staticAuthService{hash: "hash"}, logger, "example.com")

	got, err := svc.RegisterAccount(ctx, &RegisterAccountCommand{
		Username:                 "alice",
		Agreement:                true,
		Locale:                   "en",
		PasskeyRegistrationProof: "proof-1",
	})
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.Actor)

	credentials, err := accountRepo.GetUserWebAuthnCredentials(ctx, "alice")
	require.NoError(t, err)
	require.Len(t, credentials, 1)
	require.Equal(t, "cred-1", credentials[0].ID)
	require.Equal(t, "alice", credentials[0].UserID)
	require.Equal(t, "Passkey 1", credentials[0].Name)
	require.Equal(t, []byte("public-key"), credentials[0].PublicKey)
	require.True(t, credentials[0].BackupEligible)
	require.True(t, credentials[0].BackupState)

	proof, err := accountRepo.GetPasskeyRegistrationProof(ctx, "proof-1")
	require.NoError(t, err)
	require.True(t, proof.Consumed)
	require.False(t, proof.ConsumedAt.IsZero())
}

func TestService_RegisterAccount_WithConsumedPasskeyRegistrationProof_DeletesCredential(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	baseTime := time.Now().UTC()
	tableName := "test-table"

	db := newPermissiveDynamormDB(t, permissiveDBOptions{forceUserNotFound: true})
	accountRepo := repositories.NewAccountRepository(db, tableName, "example.com", logger)
	accountRepo.SetEncryptor(noopEncryptor{})
	accountRepo.SetPermissionService(nil)
	accountRepo.SetEventService(nil)
	accountRepo.SetCachingService(nil)

	storageImpl := &permissiveAccountsStorage{
		MockRepositoryStorage: NewMockRepositoryStorage(),
		db:                    db,
		tableName:             tableName,
		logger:                logger,
		account:               accountRepo,
		actor:                 repositories.NewActorRepository(db, tableName, logger),
		relationship:          repositories.NewRelationshipRepository(db, tableName, logger),
		social:                repositories.NewSocialRepository(db, tableName, logger, nil),
		user:                  repositories.NewUserRepository(db, tableName, logger),
		marker:                repositories.NewMarkerRepository(db, tableName, logger, nil),
		analytics:             repositories.NewTrendingRepository(db, logger, nil),
		instance:              repositories.NewInstanceRepository(db, tableName, logger),
		domainBlock:           repositories.NewDomainBlockRepository(db, tableName, logger),
		quote:                 repositories.NewQuoteRepository(db, tableName, logger, nil),
		activity:              repositories.NewActivityRepository(db, tableName, logger, nil),
	}

	require.NoError(t, accountRepo.StorePasskeyRegistrationProof(ctx, &models.PasskeyRegistrationProof{
		ID:              "proof-1",
		Username:        "alice",
		CeremonyID:      "ceremony-1",
		CredentialID:    "cred-1",
		PublicKey:       []byte("public-key"),
		AttestationType: "packed",
		AAGUID:          []byte("aaguid"),
		SignCount:       11,
		CreatedAt:       baseTime,
		ExpiresAt:       baseTime.Add(time.Hour),
	}))
	_, err := accountRepo.ConsumePasskeyRegistrationProof(ctx, "proof-1", "alice", "ceremony-1")
	require.NoError(t, err)

	cryptoSvc := staticCryptoService{
		publicKeyPEM:  []byte("PUBLIC KEY"),
		privateKeyPEM: []byte("PRIVATE KEY"),
		key:           struct{}{},
	}

	svc := NewService(storageImpl, streaming.NewMockPublisher(), nil, cryptoSvc, staticAuthService{hash: "hash"}, logger, "example.com")

	got, err := svc.RegisterAccount(ctx, &RegisterAccountCommand{
		Username:                 "alice",
		Agreement:                true,
		Locale:                   "en",
		PasskeyRegistrationProof: "proof-1",
	})
	require.Nil(t, got)
	require.Error(t, err)

	credentials, err := accountRepo.GetUserWebAuthnCredentials(ctx, "alice")
	require.NoError(t, err)
	require.Empty(t, credentials)
}

func TestService_validateRegisterAccountCommand_RejectsBothRegistrationProofTypes(t *testing.T) {
	svc := NewService(nil, streaming.NewMockPublisher(), nil, nil, nil, zap.NewNop(), "example.com")

	err := svc.validateRegisterAccountCommand(context.Background(), &RegisterAccountCommand{
		Username:                 "alice",
		Agreement:                true,
		RegistrationChallengeID:  "wallet-proof",
		PasskeyRegistrationProof: "passkey-proof",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot both be provided")
}

func TestPasskeyRegistrationProofToCredential(t *testing.T) {
	credential := passkeyRegistrationProofToCredential(&models.PasskeyRegistrationProof{
		Username:        "alice",
		CredentialID:    "cred-1",
		PublicKey:       []byte("public-key"),
		AttestationType: "packed",
		AAGUID:          []byte("aaguid"),
		SignCount:       3,
		CloneWarning:    true,
		BackupEligible:  true,
		BackupState:     true,
	})

	require.Equal(t, &storage.WebAuthnCredential{
		ID:              "cred-1",
		UserID:          "alice",
		PublicKey:       []byte("public-key"),
		AttestationType: "packed",
		AAGUID:          []byte("aaguid"),
		SignCount:       3,
		CloneWarning:    true,
		BackupEligible:  true,
		BackupState:     true,
		Name:            "Passkey 1",
		CreatedAt:       credential.CreatedAt,
		LastUsedAt:      credential.LastUsedAt,
	}, credential)
	require.False(t, credential.CreatedAt.IsZero())
	require.False(t, credential.LastUsedAt.IsZero())
}
