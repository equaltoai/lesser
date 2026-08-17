package accounts

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
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
		Username:                 "Alice",
		Agreement:                true,
		Locale:                   "en",
		PasskeyRegistrationProof: "proof-1",
	})
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.Actor)
	require.Equal(t, "alice", got.Account.User.Username)

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

func TestService_RegisterAccount_WithPasskeyRegistrationProof_SucceedsInSetupAdminBootstrapMode(t *testing.T) {
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
		ID:              "proof-setup-1",
		Username:        "admin",
		CeremonyID:      "ceremony-setup-1",
		CredentialID:    "cred-setup-1",
		PublicKey:       []byte("public-key"),
		AttestationType: "packed",
		AAGUID:          []byte("aaguid"),
		SignCount:       7,
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
		Username:                 "Admin",
		DisplayName:              "Primary Admin",
		Agreement:                true,
		Locale:                   "en",
		RegistrationMode:         RegisterAccountModeSetupAdminBootstrap,
		PasskeyRegistrationProof: "proof-setup-1",
	})
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "admin", got.Account.User.Username)
	require.Equal(t, "admin", got.Account.User.Role)
	require.Equal(t, "Primary Admin", got.Account.User.DisplayName)
	require.NotNil(t, got.Account.Actor)
	require.Equal(t, "Primary Admin", got.Account.Actor.Name)

	credentials, err := accountRepo.GetUserWebAuthnCredentials(ctx, "admin")
	require.NoError(t, err)
	require.Len(t, credentials, 1)
	require.Equal(t, "cred-setup-1", credentials[0].ID)

	proof, err := accountRepo.GetPasskeyRegistrationProof(ctx, "proof-setup-1")
	require.NoError(t, err)
	require.True(t, proof.Consumed)
}

func TestService_RegisterAccount_WithPasskeyRegistrationProof_RetriesSetupAdminBootstrapAfterPartialFailure(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	baseTime := time.Now().UTC()

	db := newRegistrationPasskeyDB(0)
	db.failCreateOnce("*models.WebAuthnCredential", errors.New("credential create failed"))
	db.failDeleteOnce("*models.Actor", errors.New("actor rollback failed"))
	db.failDeleteOnce("*models.User", errors.New("user rollback failed"))

	accountRepo, storageImpl := newRegistrationPasskeyTestStorage(t, db, logger)

	require.NoError(t, accountRepo.StorePasskeyRegistrationProof(ctx, &models.PasskeyRegistrationProof{
		ID:              "proof-retry-1",
		Username:        "admin",
		CeremonyID:      "ceremony-retry-1",
		CredentialID:    "cred-retry-1",
		PublicKey:       []byte("public-key"),
		AttestationType: "packed",
		AAGUID:          []byte("aaguid"),
		SignCount:       7,
		BackupEligible:  true,
		BackupState:     true,
		CreatedAt:       baseTime,
		ExpiresAt:       baseTime.Add(time.Hour),
	}))

	svc := newRegistrationPasskeyTestService(storageImpl, logger, "PUBLIC KEY", "PRIVATE KEY")
	cmd := &RegisterAccountCommand{
		Username:                 "Admin",
		DisplayName:              "Primary Admin",
		Agreement:                true,
		Locale:                   "en",
		RegistrationMode:         RegisterAccountModeSetupAdminBootstrap,
		PasskeyRegistrationProof: "proof-retry-1",
	}

	got, err := svc.RegisterAccount(ctx, cmd)
	require.Nil(t, got)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to store initial passkey credential")

	existingAccount, err := accountRepo.GetAccount(ctx, "admin")
	require.NoError(t, err)
	require.NotNil(t, existingAccount)
	require.NotNil(t, existingAccount.User)
	require.NotNil(t, existingAccount.Actor)

	credentials, err := accountRepo.GetUserWebAuthnCredentials(ctx, "admin")
	require.NoError(t, err)
	require.Empty(t, credentials)

	proof, err := accountRepo.GetPasskeyRegistrationProof(ctx, "proof-retry-1")
	require.NoError(t, err)
	require.False(t, proof.Consumed)

	got, err = svc.RegisterAccount(ctx, cmd)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "admin", got.Account.User.Username)
	require.Equal(t, "Primary Admin", got.Account.User.DisplayName)

	credentials, err = accountRepo.GetUserWebAuthnCredentials(ctx, "admin")
	require.NoError(t, err)
	require.Len(t, credentials, 1)
	require.Equal(t, "cred-retry-1", credentials[0].ID)

	proof, err = accountRepo.GetPasskeyRegistrationProof(ctx, "proof-retry-1")
	require.NoError(t, err)
	require.True(t, proof.Consumed)
}

func TestService_RegisterAccount_WithPasskeyRegistrationProof_HardErrorsWhenRetryProofConsumedWithoutCredential(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	baseTime := time.Now().UTC()

	db := newRegistrationPasskeyDB(0)
	db.failCreateOnce("*models.WebAuthnCredential", errors.New("credential create failed"))
	db.failDeleteOnce("*models.Actor", errors.New("actor rollback failed"))
	db.failDeleteOnce("*models.User", errors.New("user rollback failed"))

	accountRepo, storageImpl := newRegistrationPasskeyTestStorage(t, db, logger)

	require.NoError(t, accountRepo.StorePasskeyRegistrationProof(ctx, &models.PasskeyRegistrationProof{
		ID:              "proof-retry-hard-error-1",
		Username:        "admin",
		CeremonyID:      "ceremony-retry-hard-error-1",
		CredentialID:    "cred-retry-hard-error-1",
		PublicKey:       []byte("public-key"),
		AttestationType: "packed",
		AAGUID:          []byte("aaguid"),
		SignCount:       7,
		BackupEligible:  true,
		BackupState:     true,
		CreatedAt:       baseTime,
		ExpiresAt:       baseTime.Add(time.Hour),
	}))

	svc := newRegistrationPasskeyTestService(storageImpl, logger, "PUBLIC KEY", "PRIVATE KEY")
	cmd := &RegisterAccountCommand{
		Username:                 "Admin",
		DisplayName:              "Primary Admin",
		Agreement:                true,
		Locale:                   "en",
		RegistrationMode:         RegisterAccountModeSetupAdminBootstrap,
		PasskeyRegistrationProof: "proof-retry-hard-error-1",
	}

	got, err := svc.RegisterAccount(ctx, cmd)
	require.Nil(t, got)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to store initial passkey credential")

	proof, err := accountRepo.ConsumePasskeyRegistrationProof(ctx, "proof-retry-hard-error-1", "admin", "ceremony-retry-hard-error-1")
	require.NoError(t, err)
	require.True(t, proof.Consumed)

	got, err = svc.RegisterAccount(ctx, cmd)
	require.Nil(t, got)
	require.Error(t, err)

	var stateErr *SetupAdminBootstrapStateError
	require.ErrorAs(t, err, &stateErr)
	assert.Equal(t, "admin", stateErr.Username)
	assert.Equal(t, accountRoleAdmin, stateErr.Role)
	assert.True(t, stateErr.ActorPresent)
	assert.Equal(t, "cred-retry-hard-error-1", stateErr.CredentialID)
	assert.False(t, stateErr.CredentialBound)
	assert.True(t, stateErr.ProofConsumed)
	assert.Contains(t, err.Error(), "passkey credential \"cred-retry-hard-error-1\"=missing")

	credentials, err := accountRepo.GetUserWebAuthnCredentials(ctx, "admin")
	require.NoError(t, err)
	require.Empty(t, credentials)
}

func TestService_RegisterAccount_SetupAdminBootstrapRejectsHealthyUserAccountEvenWithFreshPasskeyProof(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	baseTime := time.Now().UTC()

	db := newRegistrationPasskeyDB(0)
	accountRepo, storageImpl := newRegistrationPasskeyTestStorage(t, db, logger)

	require.NoError(t, accountRepo.CreateAccountIfNotExists(ctx, &storage.Account{
		User: &storage.User{
			Username:    "alice",
			DisplayName: "Alice",
			Approved:    true,
			Role:        "user",
			CreatedAt:   baseTime,
		},
		Actor: &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   "https://example.com/users/alice",
				Type: "Person",
			},
			PreferredUsername: "alice",
			Name:              "Alice",
			URL:               "https://example.com/@alice",
		},
		PrivateKey: "PRIVATE KEY",
	}))
	require.NoError(t, accountRepo.StoreWebAuthnCredential(ctx, &storage.WebAuthnCredential{
		ID:              "cred-victim-own",
		UserID:          "alice",
		PublicKey:       []byte("victim-public-key"),
		AttestationType: "packed",
		AAGUID:          []byte("victim-aaguid"),
		SignCount:       3,
		CreatedAt:       baseTime,
		LastUsedAt:      baseTime,
		Name:            "Victim Passkey",
	}))
	require.NoError(t, accountRepo.StorePasskeyRegistrationProof(ctx, &models.PasskeyRegistrationProof{
		ID:              "proof-escalation-1",
		Username:        "alice",
		CeremonyID:      "ceremony-escalation-1",
		CredentialID:    "cred-attacker",
		PublicKey:       []byte("attacker-public-key"),
		AttestationType: "packed",
		AAGUID:          []byte("attacker-aaguid"),
		SignCount:       5,
		BackupEligible:  true,
		BackupState:     true,
		CreatedAt:       baseTime,
		ExpiresAt:       baseTime.Add(time.Hour),
	}))

	svc := newRegistrationPasskeyTestService(storageImpl, logger, "PUBLIC KEY", "PRIVATE KEY")
	got, err := svc.RegisterAccount(ctx, &RegisterAccountCommand{
		Username:                 "Alice",
		DisplayName:              "Escalation Attempt",
		Agreement:                true,
		Locale:                   "en",
		RegistrationMode:         RegisterAccountModeSetupAdminBootstrap,
		PasskeyRegistrationProof: "proof-escalation-1",
	})
	require.Nil(t, got)
	require.ErrorIs(t, err, ErrUsernameAlreadyTaken)

	account, err := accountRepo.GetAccount(ctx, "alice")
	require.NoError(t, err)
	require.NotNil(t, account)
	require.NotNil(t, account.User)
	require.Equal(t, "user", account.User.Role)

	credentials, err := accountRepo.GetUserWebAuthnCredentials(ctx, "alice")
	require.NoError(t, err)
	require.Len(t, credentials, 1)
	require.Equal(t, "cred-victim-own", credentials[0].ID)

	proof, err := accountRepo.GetPasskeyRegistrationProof(ctx, "proof-escalation-1")
	require.NoError(t, err)
	require.False(t, proof.Consumed)
}

func TestService_RegisterAccount_SetupAdminBootstrapActorMissingFailsBeforeProofConsumption(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	baseTime := time.Now().UTC()

	db := newRegistrationPasskeyDB(0)
	accountRepo, storageImpl := newRegistrationPasskeyTestStorage(t, db, logger)
	userRepo := repositories.NewUserRepository(db, "test-table", logger)

	require.NoError(t, userRepo.CreateUser(ctx, &storage.User{
		Username:    "admin",
		DisplayName: "Primary Admin",
		Approved:    true,
		Role:        accountRoleAdmin,
		CreatedAt:   baseTime,
	}))
	require.NoError(t, accountRepo.StorePasskeyRegistrationProof(ctx, &models.PasskeyRegistrationProof{
		ID:              "proof-actor-missing-1",
		Username:        "admin",
		CeremonyID:      "ceremony-actor-missing-1",
		CredentialID:    "cred-actor-missing-1",
		PublicKey:       []byte("public-key"),
		AttestationType: "packed",
		AAGUID:          []byte("aaguid"),
		SignCount:       7,
		BackupEligible:  true,
		BackupState:     true,
		CreatedAt:       baseTime,
		ExpiresAt:       baseTime.Add(time.Hour),
	}))

	svc := newRegistrationPasskeyTestService(storageImpl, logger, "PUBLIC KEY", "PRIVATE KEY")
	got, err := svc.RegisterAccount(ctx, &RegisterAccountCommand{
		Username:                 "Admin",
		DisplayName:              "Primary Admin",
		Agreement:                true,
		Locale:                   "en",
		RegistrationMode:         RegisterAccountModeSetupAdminBootstrap,
		PasskeyRegistrationProof: "proof-actor-missing-1",
	})
	require.Nil(t, got)
	require.Error(t, err)

	var stateErr *SetupAdminBootstrapStateError
	require.ErrorAs(t, err, &stateErr)
	assert.Equal(t, "admin", stateErr.Username)
	assert.Equal(t, accountRoleAdmin, stateErr.Role)
	assert.False(t, stateErr.ActorPresent)
	assert.Equal(t, "cred-actor-missing-1", stateErr.CredentialID)
	assert.False(t, stateErr.CredentialBound)
	assert.False(t, stateErr.ProofConsumed)

	credentials, err := accountRepo.GetUserWebAuthnCredentials(ctx, "admin")
	require.NoError(t, err)
	require.Empty(t, credentials)

	proof, err := accountRepo.GetPasskeyRegistrationProof(ctx, "proof-actor-missing-1")
	require.NoError(t, err)
	require.False(t, proof.Consumed)
}

func TestService_EnsureSetupAdminBootstrapPasskeyCredential_ConsumesUnspentProofForBoundCredential(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	baseTime := time.Now().UTC()
	accountRepo, _ := newRegistrationPasskeyTestStorage(t, newRegistrationPasskeyDB(0), logger)

	require.NoError(t, accountRepo.StorePasskeyRegistrationProof(ctx, &models.PasskeyRegistrationProof{
		ID:              "proof-bound-1",
		Username:        "admin",
		CeremonyID:      "ceremony-bound-1",
		CredentialID:    "cred-bound-1",
		PublicKey:       []byte("public-key"),
		AttestationType: "packed",
		AAGUID:          []byte("aaguid"),
		SignCount:       9,
		BackupEligible:  true,
		BackupState:     true,
		CreatedAt:       baseTime,
		ExpiresAt:       baseTime.Add(time.Hour),
	}))
	require.NoError(t, accountRepo.StoreWebAuthnCredential(ctx, &storage.WebAuthnCredential{
		ID:              "cred-bound-1",
		UserID:          "admin",
		PublicKey:       []byte("public-key"),
		AttestationType: "packed",
		AAGUID:          []byte("aaguid"),
		SignCount:       9,
		BackupEligible:  true,
		BackupState:     true,
		CreatedAt:       baseTime,
		LastUsedAt:      baseTime,
		Name:            "Primary Admin Passkey",
	}))

	proof, err := accountRepo.GetPasskeyRegistrationProof(ctx, "proof-bound-1")
	require.NoError(t, err)

	svc := &Service{}
	require.NoError(t, svc.ensureSetupAdminBootstrapPasskeyCredential(ctx, accountRepo, "admin", accountRoleAdmin, proof))

	bound, err := setupAdminBootstrapCredentialBound(ctx, accountRepo, "admin", "cred-bound-1")
	require.NoError(t, err)
	require.True(t, bound)
	require.Equal(t, "cred-bound-1", setupAdminBootstrapCredentialID(proof))
	require.Empty(t, setupAdminBootstrapCredentialID(nil))

	proof, err = accountRepo.GetPasskeyRegistrationProof(ctx, "proof-bound-1")
	require.NoError(t, err)
	require.True(t, proof.Consumed)
	require.False(t, proof.ConsumedAt.IsZero())
}

func TestService_PromoteSetupAdminBootstrapUser_UpdatesRoleAndDisplayName(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	tableName := "test-table"

	db := newPermissiveDynamormDB(t, permissiveDBOptions{})
	accountRepo := repositories.NewAccountRepository(db, tableName, "example.com", logger)
	accountRepo.SetEncryptor(noopEncryptor{})
	accountRepo.SetPermissionService(nil)
	accountRepo.SetEventService(nil)
	accountRepo.SetCachingService(nil)

	userRepo := repositories.NewUserRepository(db, tableName, logger)
	require.NoError(t, userRepo.CreateUser(ctx, &storage.User{
		Username:    "admin",
		DisplayName: "Before",
		Role:        "user",
		Approved:    true,
	}))

	user, err := accountRepo.GetUser(ctx, "admin")
	require.NoError(t, err)
	require.NotNil(t, user)

	svc := &Service{}
	require.NoError(t, svc.promoteSetupAdminBootstrapUser(ctx, accountRepo, user, "After"))
	assert.Equal(t, accountRoleAdmin, user.Role)
	assert.Equal(t, "After", user.DisplayName)
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

func TestService_validateRegisterAccountCommand_RequiresExactlyOneRegistrationProof(t *testing.T) {
	svc := NewService(nil, streaming.NewMockPublisher(), nil, nil, nil, zap.NewNop(), "example.com")

	err := svc.validateRegisterAccountCommand(context.Background(), &RegisterAccountCommand{
		Username:                 "alice",
		Agreement:                true,
		RegistrationChallengeID:  "wallet-proof",
		PasskeyRegistrationProof: "passkey-proof",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one")

	err = svc.validateRegisterAccountCommand(context.Background(), &RegisterAccountCommand{
		Username:  "alice",
		Agreement: true,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one")
}

func TestService_validateRegisterAccountCommand_AllowsSetupAdminBootstrapWithoutPublicProof(t *testing.T) {
	svc := NewService(nil, streaming.NewMockPublisher(), nil, nil, nil, zap.NewNop(), "example.com")

	err := svc.validateRegisterAccountCommand(context.Background(), &RegisterAccountCommand{
		Username:         "alice",
		Agreement:        true,
		RegistrationMode: RegisterAccountModeSetupAdminBootstrap,
	})
	require.NoError(t, err)
}

func TestService_validateRegisterAccountCommand_AllowsSetupAdminBootstrapWithPasskeyProof(t *testing.T) {
	svc := NewService(nil, streaming.NewMockPublisher(), nil, nil, nil, zap.NewNop(), "example.com")

	err := svc.validateRegisterAccountCommand(context.Background(), &RegisterAccountCommand{
		Username:                 "alice",
		Agreement:                true,
		RegistrationMode:         RegisterAccountModeSetupAdminBootstrap,
		PasskeyRegistrationProof: "passkey-proof",
	})
	require.NoError(t, err)
}

func TestService_validateRegisterAccountCommand_RejectsPublicProofsInSetupAdminBootstrapMode(t *testing.T) {
	svc := NewService(nil, streaming.NewMockPublisher(), nil, nil, nil, zap.NewNop(), "example.com")

	err := svc.validateRegisterAccountCommand(context.Background(), &RegisterAccountCommand{
		Username:                "alice",
		Agreement:               true,
		RegistrationChallengeID: "wallet-proof",
		RegistrationMode:        RegisterAccountModeSetupAdminBootstrap,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "wallet registration proofs")
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
