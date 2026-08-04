package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	theorydbErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	theorymocks "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
)

func TestEnsureWalletNotLinkedElsewhere_AllowsNotFound(t *testing.T) {
	db := new(theorymocks.MockDB)
	q := new(theorymocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db)
	db.On("Model", mock.Anything).Return(q)
	q.On("Where", "PK", "=", "WALLET#ethereum#0xabc").Return(q)
	q.On("All", mock.Anything).Return(theorydbErrors.ErrItemNotFound)

	require.NoError(t, ensureWalletNotLinkedElsewhere(context.Background(), db, "0xabc", "alice"))
	db.AssertExpectations(t)
	q.AssertExpectations(t)
}

func TestEnsureWalletNotLinkedElsewhere_RejectsWalletLinkedToOtherUser(t *testing.T) {
	db := new(theorymocks.MockDB)
	q := new(theorymocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db)
	db.On("Model", mock.Anything).Return(q)
	q.On("Where", "PK", "=", "WALLET#ethereum#0xabc").Return(q)
	q.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		dest, ok := args.Get(0).(*[]storagemodels.WalletIndex)
		require.True(t, ok)
		*dest = []storagemodels.WalletIndex{
			{Username: ""},
			{Username: "bob"},
		}
	})

	err := ensureWalletNotLinkedElsewhere(context.Background(), db, "0xabc", "alice")
	require.Error(t, err)
	require.Contains(t, err.Error(), "already linked")

	db.AssertExpectations(t)
	q.AssertExpectations(t)
}

func TestEnsureWalletNotLinkedElsewhere_AllowsWalletLinkedToSameUser(t *testing.T) {
	db := new(theorymocks.MockDB)
	q := new(theorymocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db)
	db.On("Model", mock.Anything).Return(q)
	q.On("Where", "PK", "=", "WALLET#ethereum#0xabc").Return(q)
	q.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		dest, ok := args.Get(0).(*[]storagemodels.WalletIndex)
		require.True(t, ok)
		*dest = []storagemodels.WalletIndex{
			{Username: "alice"},
		}
	})

	require.NoError(t, ensureWalletNotLinkedElsewhere(context.Background(), db, "0xabc", "alice"))
	db.AssertExpectations(t)
	q.AssertExpectations(t)
}

func TestEnsureAdminUser_CreatesWhenMissing(t *testing.T) {
	db := new(theorymocks.MockDB)
	qGet := new(theorymocks.MockQuery)
	qCreate := new(theorymocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db)
	db.On("Model", mock.Anything).Return(qGet).Once()
	db.On("Model", mock.Anything).Return(qCreate).Once()

	username := "alice"
	pk := "USER#alice"
	now := time.Now().UTC()

	qGet.On("Where", "PK", "=", pk).Return(qGet)
	qGet.On("Where", "SK", "=", storagemodels.SKMetadata).Return(qGet)
	qGet.On("ConsistentRead").Return(qGet)
	qGet.On("First", mock.Anything).Return(theorydbErrors.ErrItemNotFound)

	qCreate.On("Create").Return(nil)

	require.NoError(t, ensureAdminUser(context.Background(), db, username, now))
	db.AssertExpectations(t)
	qGet.AssertExpectations(t)
	qCreate.AssertExpectations(t)
}

func TestEnsureAdminUser_IgnoresConditionalCreateConflict(t *testing.T) {
	db := new(theorymocks.MockDB)
	qGet := new(theorymocks.MockQuery)
	qCreate := new(theorymocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db)
	db.On("Model", mock.Anything).Return(qGet).Once()
	db.On("Model", mock.Anything).Return(qCreate).Once()

	username := "alice"
	pk := "USER#alice"
	now := time.Now().UTC()

	qGet.On("Where", "PK", "=", pk).Return(qGet)
	qGet.On("Where", "SK", "=", storagemodels.SKMetadata).Return(qGet)
	qGet.On("ConsistentRead").Return(qGet)
	qGet.On("First", mock.Anything).Return(theorydbErrors.ErrItemNotFound)

	qCreate.On("Create").Return(theorydbErrors.ErrConditionFailed)

	require.NoError(t, ensureAdminUser(context.Background(), db, username, now))
	db.AssertExpectations(t)
	qGet.AssertExpectations(t)
	qCreate.AssertExpectations(t)
}

func TestEnsureAdminUser_NoOpWhenAlreadyAdmin(t *testing.T) {
	db := new(theorymocks.MockDB)
	qGet := new(theorymocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db)
	db.On("Model", mock.Anything).Return(qGet).Once()

	username := "alice"
	pk := "USER#alice"
	now := time.Now().UTC()

	qGet.On("Where", "PK", "=", pk).Return(qGet)
	qGet.On("Where", "SK", "=", storagemodels.SKMetadata).Return(qGet)
	qGet.On("ConsistentRead").Return(qGet)
	qGet.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		dest, ok := args.Get(0).(*storagemodels.User)
		require.True(t, ok)
		dest.Role = "admin"
		dest.Approved = true
		dest.Locked = false
	})

	require.NoError(t, ensureAdminUser(context.Background(), db, username, now))
	db.AssertExpectations(t)
	qGet.AssertExpectations(t)
}

func TestEnsureAdminUser_UpdatesExistingUser(t *testing.T) {
	db := new(theorymocks.MockDB)
	qGet := new(theorymocks.MockQuery)
	qUpdate := new(theorymocks.MockQuery)
	builder := new(theorymocks.MockUpdateBuilder)

	db.On("WithContext", mock.Anything).Return(db)
	db.On("Model", mock.Anything).Return(qGet).Once()
	db.On("Model", mock.Anything).Return(qUpdate).Once()

	username := "alice"
	pk := "USER#alice"
	now := time.Now().UTC()

	qGet.On("Where", "PK", "=", pk).Return(qGet)
	qGet.On("Where", "SK", "=", storagemodels.SKMetadata).Return(qGet)
	qGet.On("ConsistentRead").Return(qGet)
	qGet.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		dest, ok := args.Get(0).(*storagemodels.User)
		require.True(t, ok)
		dest.Role = "user"
		dest.Approved = false
		dest.Locked = true
	})

	qUpdate.On("Where", "PK", "=", pk).Return(qUpdate)
	qUpdate.On("Where", "SK", "=", storagemodels.SKMetadata).Return(qUpdate)
	qUpdate.On("UpdateBuilder").Return(builder)

	builder.On("Set", "Role", "admin").Return(builder)
	builder.On("Set", "Approved", true).Return(builder)
	builder.On("Set", "Locked", false).Return(builder)
	builder.On("Set", "UpdatedAt", now).Return(builder)
	builder.On("Execute").Return(nil)

	require.NoError(t, ensureAdminUser(context.Background(), db, username, now))
	db.AssertExpectations(t)
	qGet.AssertExpectations(t)
	qUpdate.AssertExpectations(t)
	builder.AssertExpectations(t)
}

func TestEnsureWalletCredential_CreatesWhenMissing(t *testing.T) {
	db := new(theorymocks.MockDB)
	qGet := new(theorymocks.MockQuery)
	qCreate := new(theorymocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db)
	db.On("Model", mock.Anything).Return(qGet).Once()
	db.On("Model", mock.Anything).Return(qCreate).Once()

	username := "alice"
	pk := "USER#alice"
	addr := "0xabc"
	sk := "WALLET#0xabc"
	now := time.Now().UTC()

	qGet.On("Where", "PK", "=", pk).Return(qGet)
	qGet.On("Where", "SK", "=", sk).Return(qGet)
	qGet.On("ConsistentRead").Return(qGet)
	qGet.On("First", mock.Anything).Return(theorydbErrors.ErrItemNotFound)

	qCreate.On("Create").Return(nil)

	require.NoError(t, ensureWalletCredential(context.Background(), db, username, addr, 1, now))
	db.AssertExpectations(t)
	qGet.AssertExpectations(t)
	qCreate.AssertExpectations(t)
}

func TestEnsureWalletCredential_NoOpWhenAlreadyLinked(t *testing.T) {
	db := new(theorymocks.MockDB)
	qGet := new(theorymocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db)
	db.On("Model", mock.Anything).Return(qGet).Once()

	username := "alice"
	pk := "USER#alice"
	addr := "0xabc"
	sk := "WALLET#0xabc"
	now := time.Now().UTC()

	qGet.On("Where", "PK", "=", pk).Return(qGet)
	qGet.On("Where", "SK", "=", sk).Return(qGet)
	qGet.On("ConsistentRead").Return(qGet)
	qGet.On("First", mock.Anything).Return(nil)

	require.NoError(t, ensureWalletCredential(context.Background(), db, username, addr, 1, now))
	db.AssertExpectations(t)
	qGet.AssertExpectations(t)
}

func TestEnsureWalletCredential_PropagatesQueryError(t *testing.T) {
	db := new(theorymocks.MockDB)
	qGet := new(theorymocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db)
	db.On("Model", mock.Anything).Return(qGet).Once()

	username := "alice"
	pk := "USER#alice"
	addr := "0xabc"
	sk := "WALLET#0xabc"
	now := time.Now().UTC()

	qGet.On("Where", "PK", "=", pk).Return(qGet)
	qGet.On("Where", "SK", "=", sk).Return(qGet)
	qGet.On("ConsistentRead").Return(qGet)
	qGet.On("First", mock.Anything).Return(errors.New("boom"))

	err := ensureWalletCredential(context.Background(), db, username, addr, 1, now)
	require.Error(t, err)
	require.Contains(t, err.Error(), "get wallet credential")

	db.AssertExpectations(t)
	qGet.AssertExpectations(t)
}

func TestEnsureWalletIndex_CreatesWhenMissing(t *testing.T) {
	db := new(theorymocks.MockDB)
	qGet := new(theorymocks.MockQuery)
	qCreate := new(theorymocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db)
	db.On("Model", mock.Anything).Return(qGet).Once()
	db.On("Model", mock.Anything).Return(qCreate).Once()

	username := "alice"
	addr := "0xabc"
	pk := "WALLET#ethereum#0xabc"
	sk := "USER#alice"

	qGet.On("Where", "PK", "=", pk).Return(qGet)
	qGet.On("Where", "SK", "=", sk).Return(qGet)
	qGet.On("ConsistentRead").Return(qGet)
	qGet.On("First", mock.Anything).Return(theorydbErrors.ErrItemNotFound)

	qCreate.On("Create").Return(nil)

	require.NoError(t, ensureWalletIndex(context.Background(), db, username, addr))
	db.AssertExpectations(t)
	qGet.AssertExpectations(t)
	qCreate.AssertExpectations(t)
}

func TestEnsureWalletIndex_NoOpWhenAlreadyPresent(t *testing.T) {
	db := new(theorymocks.MockDB)
	qGet := new(theorymocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db)
	db.On("Model", mock.Anything).Return(qGet).Once()

	username := "alice"
	addr := "0xabc"
	pk := "WALLET#ethereum#0xabc"
	sk := "USER#alice"

	qGet.On("Where", "PK", "=", pk).Return(qGet)
	qGet.On("Where", "SK", "=", sk).Return(qGet)
	qGet.On("ConsistentRead").Return(qGet)
	qGet.On("First", mock.Anything).Return(nil)

	require.NoError(t, ensureWalletIndex(context.Background(), db, username, addr))
	db.AssertExpectations(t)
	qGet.AssertExpectations(t)
}

func TestEnsureWalletIndex_PropagatesQueryError(t *testing.T) {
	db := new(theorymocks.MockDB)
	qGet := new(theorymocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db)
	db.On("Model", mock.Anything).Return(qGet).Once()

	username := "alice"
	addr := "0xabc"
	pk := "WALLET#ethereum#0xabc"
	sk := "USER#alice"

	qGet.On("Where", "PK", "=", pk).Return(qGet)
	qGet.On("Where", "SK", "=", sk).Return(qGet)
	qGet.On("ConsistentRead").Return(qGet)
	qGet.On("First", mock.Anything).Return(errors.New("boom"))

	err := ensureWalletIndex(context.Background(), db, username, addr)
	require.Error(t, err)
	require.Contains(t, err.Error(), "get wallet index")

	db.AssertExpectations(t)
	qGet.AssertExpectations(t)
}

func TestEnsureInstanceActivated_UpdatesStateWhenMissing(t *testing.T) {
	db := new(theorymocks.MockDB)
	qGet := new(theorymocks.MockQuery)
	qUpdate := new(theorymocks.MockQuery)
	builder := new(theorymocks.MockUpdateBuilder)

	previousBootstrapTable := bootstrapTableName
	t.Cleanup(func() { bootstrapTableName = previousBootstrapTable })

	db.On("WithContext", mock.Anything).Return(db)
	db.On("Model", mock.Anything).Return(qGet).Once()
	db.On("Model", mock.Anything).Return(qUpdate).Once()

	tableName := "tbl"
	username := "alice"
	now := time.Now().UTC()

	qGet.On("Where", "PK", "=", instanceConfigKeyPK).Return(qGet)
	qGet.On("Where", "SK", "=", "STATE").Return(qGet)
	qGet.On("ConsistentRead").Return(qGet)
	qGet.On("First", mock.Anything).Return(theorydbErrors.ErrItemNotFound)

	qUpdate.On("Where", "PK", "=", instanceConfigKeyPK).Return(qUpdate)
	qUpdate.On("Where", "SK", "=", "STATE").Return(qUpdate)
	qUpdate.On("UpdateBuilder").Return(builder)

	builder.On("Set", "Locked", false).Return(builder)
	builder.On("Set", "PrimaryAdminUsername", username).Return(builder)
	builder.On("Set", "BootstrapWalletAddress", "").Return(builder)
	builder.On("Set", "UpdatedAt", now).Return(builder)
	builder.On("SetIfNotExists", "BootstrapUsername", storagemodels.DefaultBootstrapUsername, storagemodels.DefaultBootstrapUsername).Return(builder)
	builder.On("SetIfNotExists", "CreatedAt", now, now).Return(builder)
	builder.On("SetIfNotExists", "ActivatedAt", now, now).Return(builder)
	builder.On("Execute").Return(nil)

	require.NoError(t, ensureInstanceActivated(context.Background(), db, tableName, username, now))
	require.Equal(t, tableName, bootstrapTableName)

	db.AssertExpectations(t)
	qGet.AssertExpectations(t)
	qUpdate.AssertExpectations(t)
	builder.AssertExpectations(t)
}

func TestEnsureInstanceActivated_RefusesOverwrite(t *testing.T) {
	db := new(theorymocks.MockDB)
	qGet := new(theorymocks.MockQuery)

	previousBootstrapTable := bootstrapTableName
	t.Cleanup(func() { bootstrapTableName = previousBootstrapTable })

	db.On("WithContext", mock.Anything).Return(db)
	db.On("Model", mock.Anything).Return(qGet).Once()

	tableName := "tbl"
	now := time.Now().UTC()

	qGet.On("Where", "PK", "=", instanceConfigKeyPK).Return(qGet)
	qGet.On("Where", "SK", "=", "STATE").Return(qGet)
	qGet.On("ConsistentRead").Return(qGet)
	qGet.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		dest, ok := args.Get(0).(*bootstrapInstanceStateRecord)
		require.True(t, ok)
		dest.PrimaryAdminUsername = "bob"
	})

	err := ensureInstanceActivated(context.Background(), db, tableName, "alice", now)
	require.Error(t, err)
	require.Contains(t, err.Error(), "refusing to overwrite")

	db.AssertExpectations(t)
	qGet.AssertExpectations(t)
}

func TestEnsureInstanceActivated_PropagatesQueryError(t *testing.T) {
	db := new(theorymocks.MockDB)
	qGet := new(theorymocks.MockQuery)

	previousBootstrapTable := bootstrapTableName
	t.Cleanup(func() { bootstrapTableName = previousBootstrapTable })

	db.On("WithContext", mock.Anything).Return(db)
	db.On("Model", mock.Anything).Return(qGet).Once()

	tableName := "tbl"
	now := time.Now().UTC()

	qGet.On("Where", "PK", "=", instanceConfigKeyPK).Return(qGet)
	qGet.On("Where", "SK", "=", "STATE").Return(qGet)
	qGet.On("ConsistentRead").Return(qGet)
	qGet.On("First", mock.Anything).Return(errors.New("boom"))

	err := ensureInstanceActivated(context.Background(), db, tableName, "alice", now)
	require.Error(t, err)
	require.Contains(t, err.Error(), "get instance state")

	db.AssertExpectations(t)
	qGet.AssertExpectations(t)
}

func TestEnsureActor_NoOpWhenAlreadyPresent(t *testing.T) {
	db := new(theorymocks.MockDB)
	qGet := new(theorymocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db)
	db.On("Model", mock.Anything).Return(qGet).Once()

	username := "alice"
	pk := "ACTOR#alice"
	now := time.Now().UTC()

	qGet.On("Where", "PK", "=", pk).Return(qGet)
	qGet.On("Where", "SK", "=", storagemodels.SKProfile).Return(qGet)
	qGet.On("ConsistentRead").Return(qGet)
	qGet.On("First", mock.Anything).Return(nil)

	require.NoError(t, ensureActor(context.Background(), db, nil, "", username, "example.com", now))
	db.AssertExpectations(t)
	qGet.AssertExpectations(t)
}

func TestEnsureActor_PropagatesQueryError(t *testing.T) {
	db := new(theorymocks.MockDB)
	qGet := new(theorymocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db)
	db.On("Model", mock.Anything).Return(qGet).Once()

	username := "alice"
	pk := "ACTOR#alice"
	now := time.Now().UTC()

	qGet.On("Where", "PK", "=", pk).Return(qGet)
	qGet.On("Where", "SK", "=", storagemodels.SKProfile).Return(qGet)
	qGet.On("ConsistentRead").Return(qGet)
	qGet.On("First", mock.Anything).Return(errors.New("boom"))

	err := ensureActor(context.Background(), db, nil, "", username, "example.com", now)
	require.Error(t, err)
	require.Contains(t, err.Error(), "get actor")

	db.AssertExpectations(t)
	qGet.AssertExpectations(t)
}

func TestEnsureActor_CreatesWhenMissing_UsesKMSAndDB(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"CiphertextBlob":"AQID"}`))
	}))
	t.Cleanup(srv.Close)

	cfg := aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("AKID", "SECRET", ""),
		EndpointResolverWithOptions: aws.EndpointResolverWithOptionsFunc(func(service, region string, _ ...any) (aws.Endpoint, error) {
			if service == kms.ServiceID {
				return aws.Endpoint{URL: srv.URL, SigningRegion: region, HostnameImmutable: true}, nil
			}
			return aws.Endpoint{}, &aws.EndpointNotFoundError{}
		}),
	}
	kmsClient := kms.NewFromConfig(cfg)

	db := new(theorymocks.MockDB)
	qGet := new(theorymocks.MockQuery)
	qCreate := new(theorymocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db)
	db.On("Model", mock.Anything).Return(qGet).Once()
	db.On("Model", mock.Anything).Return(qCreate).Once()

	username := "alice"
	pk := "ACTOR#alice"
	now := time.Now().UTC()

	qGet.On("Where", "PK", "=", pk).Return(qGet)
	qGet.On("Where", "SK", "=", storagemodels.SKProfile).Return(qGet)
	qGet.On("ConsistentRead").Return(qGet)
	qGet.On("First", mock.Anything).Return(theorydbErrors.ErrItemNotFound)

	qCreate.On("Create").Return(nil)

	require.NoError(t, ensureActor(context.Background(), db, kmsClient, "alias/test", username, "example.com", now))
	db.AssertExpectations(t)
	qGet.AssertExpectations(t)
	qCreate.AssertExpectations(t)
}

func TestEncryptWithKMS_RequiresInputs(t *testing.T) {
	_, err := encryptWithKMS(context.Background(), nil, "key", []byte("x"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "kms client is nil")

	_, err = encryptWithKMS(context.Background(), &kms.Client{}, "", []byte("x"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "kms key id is empty")
}

func TestGenerateRSAKeyPairPEM_ValidatesMinimumKeySize(t *testing.T) {
	_, _, err := generateRSAKeyPairPEM(1024)
	require.Error(t, err)
	require.Contains(t, err.Error(), "too small")
}

func TestGenerateRSAKeyPairPEM_GeneratesPEM(t *testing.T) {
	priv, pub, err := generateRSAKeyPairPEM(2048)
	require.NoError(t, err)
	require.Contains(t, priv, "PRIVATE KEY")
	require.Contains(t, pub, "PUBLIC KEY")
}

func TestBuildActorModel_ProducesStableIDsAndKeys(t *testing.T) {
	now := time.Now().UTC()
	model, err := buildActorModel("alice", "example.com", "PUBLIC", "ENCRYPTED", now)
	require.NoError(t, err)
	require.Equal(t, "alice", model.Username)
	require.Equal(t, "RSA", model.KeyType)
	require.Equal(t, "DOMAIN#example.com", model.GSI3PK)
	require.Equal(t, "alice", model.GSI3SK)
	require.NotNil(t, model.Actor)
	require.Contains(t, model.Actor.ID, "https://example.com/users/alice")
	require.True(t, strings.HasPrefix(model.PK, "ACTOR#"))
	require.Equal(t, storagemodels.SKProfile, model.SK)
}
