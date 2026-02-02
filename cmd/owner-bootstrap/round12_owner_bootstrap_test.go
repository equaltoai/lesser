package main

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	smstypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	testifyMock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	theorydbErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	theoryMocks "github.com/theory-cloud/tabletheory/pkg/mocks"

	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
)

type fakeSecretsManager struct {
	describeErr error
	createErrs  []error
	deleteErrs  []error

	describeInputs []*secretsmanager.DescribeSecretInput
	createInputs   []*secretsmanager.CreateSecretInput
	deleteInputs   []*secretsmanager.DeleteSecretInput
}

func (f *fakeSecretsManager) DescribeSecret(_ context.Context, params *secretsmanager.DescribeSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.DescribeSecretOutput, error) {
	f.describeInputs = append(f.describeInputs, params)
	if f.describeErr != nil {
		return nil, f.describeErr
	}
	return &secretsmanager.DescribeSecretOutput{}, nil
}

func (f *fakeSecretsManager) CreateSecret(_ context.Context, params *secretsmanager.CreateSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error) {
	f.createInputs = append(f.createInputs, params)
	if len(f.createErrs) > 0 {
		err := f.createErrs[0]
		f.createErrs = f.createErrs[1:]
		if err != nil {
			return nil, err
		}
	}
	return &secretsmanager.CreateSecretOutput{}, nil
}

func (f *fakeSecretsManager) DeleteSecret(_ context.Context, params *secretsmanager.DeleteSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error) {
	f.deleteInputs = append(f.deleteInputs, params)
	if len(f.deleteErrs) > 0 {
		err := f.deleteErrs[0]
		f.deleteErrs = f.deleteErrs[1:]
		if err != nil {
			return nil, err
		}
	}
	return &secretsmanager.DeleteSecretOutput{}, nil
}

type fakeKMS struct {
	encryptInput *kms.EncryptInput
	encryptOut   *kms.EncryptOutput
	encryptErr   error
}

func (f *fakeKMS) Encrypt(_ context.Context, params *kms.EncryptInput, _ ...func(*kms.Options)) (*kms.EncryptOutput, error) {
	f.encryptInput = params
	if f.encryptErr != nil {
		return nil, f.encryptErr
	}
	if f.encryptOut != nil {
		return f.encryptOut, nil
	}
	return &kms.EncryptOutput{CiphertextBlob: []byte("cipher")}, nil
}

func TestOwnerBootstrapArgs_DefaultsAndValidate(t *testing.T) {
	args := ownerBootstrapArgs{
		environment: "dev",
		domain:      "example.com",
		kmsKeyID:    "alias/test",
		username:    "admin",
		chainID:     1,
	}
	args.applyDefaults()
	require.Equal(t, "lesser-dev", args.tableName)
	require.Equal(t, "lesser/dev/admin-wallet", args.walletSecretName)
	require.Equal(t, "lesser/dev/admin-oauth", args.oauthSecretName)
	require.NoError(t, args.validate())

	require.Error(t, ownerBootstrapArgs{domain: "example.com", username: "admin", kmsKeyID: "k", chainID: 1}.validate())
	require.Error(t, ownerBootstrapArgs{environment: "dev", username: "admin", kmsKeyID: "k", chainID: 1}.validate())
	require.Error(t, ownerBootstrapArgs{environment: "dev", domain: "example.com", kmsKeyID: "k", chainID: 1}.validate())
	require.Error(t, ownerBootstrapArgs{environment: "dev", domain: "example.com", username: "admin", chainID: 1}.validate())
	require.Error(t, ownerBootstrapArgs{environment: "dev", domain: "example.com", username: "admin", kmsKeyID: "k", chainID: 0}.validate())
}

func TestParseOwnerBootstrapArgs_ParsesAndErrors(t *testing.T) {
	args, err := parseOwnerBootstrapArgs([]string{
		"-environment=dev",
		"-domain=example.com",
		"-table=tbl",
		"-kms-key-id=alias/test",
		"-wallet-secret=w",
		"-oauth-secret=o",
		"-username=admin",
		"-chain-id=2",
		"-force=true",
	})
	require.NoError(t, err)
	require.Equal(t, "dev", args.environment)
	require.Equal(t, "example.com", args.domain)
	require.Equal(t, "tbl", args.tableName)
	require.Equal(t, "alias/test", args.kmsKeyID)
	require.Equal(t, "w", args.walletSecretName)
	require.Equal(t, "o", args.oauthSecretName)
	require.Equal(t, "admin", args.username)
	require.Equal(t, 2, args.chainID)
	require.True(t, args.force)

	_, err = parseOwnerBootstrapArgs([]string{"-nope"})
	require.Error(t, err)
}

func TestValidateBootstrapState_Branches(t *testing.T) {
	args := ownerBootstrapArgs{
		environment:      "dev",
		domain:           "example.com",
		tableName:        "tbl",
		kmsKeyID:         "kms",
		walletSecretName: "wallet",
		oauthSecretName:  "oauth",
		username:         "admin",
		chainID:          1,
	}
	userPK := "USER#admin"

	skipFields, failure := validateBootstrapState(bootstrapState{userExists: true}, args, userPK)
	require.NotNil(t, skipFields)
	require.Nil(t, failure)

	skipFields, failure = validateBootstrapState(bootstrapState{userExists: false, walletSecretExists: true}, args, userPK)
	require.Nil(t, skipFields)
	require.NotNil(t, failure)
	require.Equal(t, "partial_state", failure.event)

	args.force = true
	skipFields, failure = validateBootstrapState(bootstrapState{userExists: true}, args, userPK)
	require.Nil(t, skipFields)
	require.NotNil(t, failure)
	require.Equal(t, "force_not_supported", failure.event)
}

func TestUserMetadataExists(t *testing.T) {
	ctx := context.Background()

	t.Run("exists", func(t *testing.T) {
		db := theoryMocks.NewMockExtendedDBStrict()
		q := new(theoryMocks.MockQuery)

		db.On("Model", testifyMock.Anything).Return(q).Once()
		q.On("WithContext", ctx).Return(q).Once()
		q.On("Where", "PK", "=", "USER#admin").Return(q).Once()
		q.On("Where", "SK", "=", storagemodels.SKMetadata).Return(q).Once()
		q.On("ConsistentRead").Return(q).Once()
		q.On("First", testifyMock.Anything).Return(nil).Once()

		exists, err := userMetadataExists(ctx, db, "USER#admin", storagemodels.SKMetadata)
		require.NoError(t, err)
		require.True(t, exists)

		db.AssertExpectations(t)
		q.AssertExpectations(t)
	})

	t.Run("not found", func(t *testing.T) {
		db := theoryMocks.NewMockExtendedDBStrict()
		q := new(theoryMocks.MockQuery)

		db.On("Model", testifyMock.Anything).Return(q).Once()
		q.On("WithContext", ctx).Return(q).Once()
		q.On("Where", "PK", "=", "USER#admin").Return(q).Once()
		q.On("Where", "SK", "=", storagemodels.SKMetadata).Return(q).Once()
		q.On("ConsistentRead").Return(q).Once()
		q.On("First", testifyMock.Anything).Return(theorydbErrors.ErrItemNotFound).Once()

		exists, err := userMetadataExists(ctx, db, "USER#admin", storagemodels.SKMetadata)
		require.NoError(t, err)
		require.False(t, exists)

		db.AssertExpectations(t)
		q.AssertExpectations(t)
	})

	t.Run("table not found", func(t *testing.T) {
		db := theoryMocks.NewMockExtendedDBStrict()
		q := new(theoryMocks.MockQuery)

		db.On("Model", testifyMock.Anything).Return(q).Once()
		q.On("WithContext", ctx).Return(q).Once()
		q.On("Where", "PK", "=", "USER#admin").Return(q).Once()
		q.On("Where", "SK", "=", storagemodels.SKMetadata).Return(q).Once()
		q.On("ConsistentRead").Return(q).Once()
		q.On("First", testifyMock.Anything).Return(theorydbErrors.ErrTableNotFound).Once()

		exists, err := userMetadataExists(ctx, db, "USER#admin", storagemodels.SKMetadata)
		require.Error(t, err)
		require.False(t, exists)
		require.ErrorIs(t, err, theorydbErrors.ErrTableNotFound)

		db.AssertExpectations(t)
		q.AssertExpectations(t)
	})

	t.Run("nil database", func(t *testing.T) {
		exists, err := userMetadataExists(ctx, nil, "USER#admin", storagemodels.SKMetadata)
		require.Error(t, err)
		require.False(t, exists)
	})
}

func TestSecretExists_CreateAndDeleteSecret(t *testing.T) {
	sm := &fakeSecretsManager{}

	exists, err := secretExists(context.Background(), sm, "secret")
	require.NoError(t, err)
	require.True(t, exists)

	sm.describeErr = &smstypes.ResourceNotFoundException{}
	exists, err = secretExists(context.Background(), sm, "missing")
	require.NoError(t, err)
	require.False(t, exists)

	sm.describeErr = errors.New("boom")
	_, err = secretExists(context.Background(), sm, "err")
	require.Error(t, err)

	sm.describeErr = nil
	require.NoError(t, createSecret(context.Background(), sm, "name", "value", "desc"))
	require.Len(t, sm.createInputs, 1)
	require.Equal(t, "name", aws.ToString(sm.createInputs[0].Name))
	require.Equal(t, "value", aws.ToString(sm.createInputs[0].SecretString))

	require.NoError(t, deleteSecretImmediate(context.Background(), sm, "name"))
	require.Len(t, sm.deleteInputs, 1)
	require.True(t, aws.ToBool(sm.deleteInputs[0].ForceDeleteWithoutRecovery))
}

func TestEncryptWithKMS(t *testing.T) {
	kmsClient := &fakeKMS{encryptOut: &kms.EncryptOutput{CiphertextBlob: []byte("ciphertext")}}
	ciphertext, err := encryptWithKMS(context.Background(), kmsClient, "alias/test", []byte("plain"))
	require.NoError(t, err)
	require.Equal(t, []byte("ciphertext"), ciphertext)
	require.Equal(t, "alias/test", aws.ToString(kmsClient.encryptInput.KeyId))

	kmsClient.encryptErr = errors.New("boom")
	_, err = encryptWithKMS(context.Background(), kmsClient, "alias/test", []byte("plain"))
	require.Error(t, err)
}

func TestKeyAndIDGenerationHelpers(t *testing.T) {
	privateKey, address, err := generateEthereumWallet()
	require.NoError(t, err)
	require.NotEmpty(t, privateKey)
	require.NotEmpty(t, address)

	clientID, err := generateOAuthClientID()
	require.NoError(t, err)
	require.NotEmpty(t, clientID)

	clientSecret, err := generateOAuthClientSecret()
	require.NoError(t, err)
	require.NotEmpty(t, clientSecret)

	_, _, err = generateRSAKeyPairPEM(1024)
	require.Error(t, err)

	priv, pub, err := generateRSAKeyPairPEM(2048)
	require.NoError(t, err)
	require.Contains(t, priv, "PRIVATE KEY")
	require.Contains(t, pub, "PUBLIC KEY")
}

func TestCheckBootstrapState_ErrorBranches(t *testing.T) {
	ctx := context.Background()
	args := ownerBootstrapArgs{tableName: "tbl", walletSecretName: "wallet", oauthSecretName: "oauth"}

	db := theoryMocks.NewMockExtendedDBStrict()
	q := new(theoryMocks.MockQuery)
	db.On("Model", testifyMock.Anything).Return(q).Once()
	q.On("WithContext", ctx).Return(q).Once()
	q.On("Where", "PK", "=", "USER#admin").Return(q).Once()
	q.On("Where", "SK", "=", storagemodels.SKMetadata).Return(q).Once()
	q.On("ConsistentRead").Return(q).Once()
	q.On("First", testifyMock.Anything).Return(errors.New("boom")).Once()

	_, err := checkBootstrapState(ctx, db, &fakeSecretsManager{}, args, "USER#admin")
	require.Error(t, err)

	db = theoryMocks.NewMockExtendedDBStrict()
	q = new(theoryMocks.MockQuery)
	db.On("Model", testifyMock.Anything).Return(q).Once()
	q.On("WithContext", ctx).Return(q).Once()
	q.On("Where", "PK", "=", "USER#admin").Return(q).Once()
	q.On("Where", "SK", "=", storagemodels.SKMetadata).Return(q).Once()
	q.On("ConsistentRead").Return(q).Once()
	q.On("First", testifyMock.Anything).Return(nil).Once()

	_, err = checkBootstrapState(ctx, db, &fakeSecretsManager{describeErr: errors.New("boom")}, args, "USER#admin")
	require.Error(t, err)
}

func TestTransactWriteAll_CreatesEachItem(t *testing.T) {
	ctx := context.Background()

	t.Run("creates items", func(t *testing.T) {
		db := theoryMocks.NewMockExtendedDBStrict()
		builder := new(theoryMocks.MockTransactionBuilder)
		db.TransactWriteBuilder = builder

		item1 := &storagemodels.User{}
		item2 := &storagemodels.Actor{}

		db.On("TransactWrite", ctx, testifyMock.Anything).Return(nil).Once()
		builder.On("Create", item1, testifyMock.Anything).Return(builder).Once()
		builder.On("Create", item2, testifyMock.Anything).Return(builder).Once()
		builder.On("Execute").Return(nil).Once()

		require.NoError(t, transactWriteAll(ctx, db, []any{item1, item2}))

		db.AssertExpectations(t)
		builder.AssertExpectations(t)
	})

	t.Run("nil item errors", func(t *testing.T) {
		db := theoryMocks.NewMockExtendedDBStrict()
		db.On("TransactWrite", ctx, testifyMock.Anything).Return(nil).Once()

		err := transactWriteAll(ctx, db, []any{nil})
		require.Error(t, err)

		db.AssertExpectations(t)
	})

	t.Run("transact failure returns error", func(t *testing.T) {
		db := theoryMocks.NewMockExtendedDBStrict()
		db.On("TransactWrite", ctx, testifyMock.Anything).Return(errors.New("boom")).Once()

		err := transactWriteAll(ctx, db, []any{&storagemodels.User{}})
		require.Error(t, err)

		db.AssertExpectations(t)
	})
}

func TestRollbackSecretsAndTableTheory_Branches(t *testing.T) {
	ctx := context.Background()

	sm := &fakeSecretsManager{deleteErrs: []error{nil, errors.New("boom")}}
	rollbackSecretsAndTableTheory(ctx, nil, sm, []string{"s1", "s2"}, nil)
	require.Len(t, sm.deleteInputs, 2)
	require.True(t, aws.ToBool(sm.deleteInputs[0].ForceDeleteWithoutRecovery))
	require.True(t, aws.ToBool(sm.deleteInputs[1].ForceDeleteWithoutRecovery))

	db := theoryMocks.NewMockExtendedDBStrict()
	builder := new(theoryMocks.MockTransactionBuilder)
	db.TransactWriteBuilder = builder

	item := &storagemodels.User{}
	db.On("TransactWrite", ctx, testifyMock.Anything).Return(nil).Once()
	builder.On("Delete", item, testifyMock.Anything).Return(builder).Once()
	builder.On("Execute").Return(nil).Once()

	rollbackSecretsAndTableTheory(ctx, db, &fakeSecretsManager{}, nil, []any{item})

	db.AssertExpectations(t)
	builder.AssertExpectations(t)
}

func TestGenerateBootstrapArtifacts_SuccessWithStubs(t *testing.T) {
	originalWalletFn := generateEthereumWalletFn
	originalRSAFn := generateRSAKeyPairPEMFn
	originalEncryptFn := encryptWithKMSFn
	originalIDFn := generateOAuthClientIDFn
	originalSecretFn := generateOAuthClientSecretFn
	t.Cleanup(func() {
		generateEthereumWalletFn = originalWalletFn
		generateRSAKeyPairPEMFn = originalRSAFn
		encryptWithKMSFn = originalEncryptFn
		generateOAuthClientIDFn = originalIDFn
		generateOAuthClientSecretFn = originalSecretFn
	})

	generateEthereumWalletFn = func() (string, string, error) { return "0xpriv", "0xADDR", nil }
	generateRSAKeyPairPEMFn = func(_ int) (string, string, error) { return "privpem", "pubpem", nil }
	encryptWithKMSFn = func(_ context.Context, _ kmsAPI, _ string, _ []byte) ([]byte, error) { return []byte("cipher"), nil }
	generateOAuthClientIDFn = func() (string, error) { return "cid", nil }
	generateOAuthClientSecretFn = func() (string, error) { return "csecret", nil }

	now := time.Unix(0, 0).UTC()
	args := ownerBootstrapArgs{
		environment:      "dev",
		domain:           "example.com",
		tableName:        "tbl",
		kmsKeyID:         "kms",
		walletSecretName: "wallet",
		oauthSecretName:  "oauth",
		username:         "admin",
		chainID:          1,
	}

	artifacts := generateBootstrapArtifacts(context.Background(), &fakeKMS{}, args, now)
	require.Equal(t, "0xADDR", artifacts.walletAddress)
	require.Equal(t, "cid", artifacts.clientID)
	require.Len(t, artifacts.items, 5)
	require.Equal(t, "wallet", artifacts.walletSecretName)
	require.Equal(t, "oauth", artifacts.oauthSecretName)
	require.Equal(t, now.Format(time.RFC3339), artifacts.createdAtISO)
}

func TestPersistBootstrapArtifacts_SuccessAndRollbackOnSecretFailure(t *testing.T) {
	originalExitFn := exitFn
	originalTableName := storagemodels.MainTableName
	t.Cleanup(func() {
		exitFn = originalExitFn
		storagemodels.MainTableName = originalTableName
	})

	ctx := context.Background()

	successDB := theoryMocks.NewMockExtendedDBStrict()
	successBuilder := new(theoryMocks.MockTransactionBuilder)
	successDB.TransactWriteBuilder = successBuilder
	successSM := &fakeSecretsManager{}
	args := ownerBootstrapArgs{tableName: "tbl", walletSecretName: "wallet", oauthSecretName: "oauth"}
	storagemodels.MainTableName = args.tableName

	item := &storagemodels.User{}
	artifacts := bootstrapArtifacts{
		items:            []any{item},
		walletSecretJSON: []byte(`{"address":"0xADDR"}`),
		oauthSecretJSON:  []byte(`{"client_id":"cid"}`),
	}

	successDB.On("TransactWrite", ctx, testifyMock.Anything).Return(nil).Once()
	successBuilder.On("Create", item, testifyMock.Anything).Return(successBuilder).Once()
	successBuilder.On("Execute").Return(nil).Once()

	result := persistBootstrapArtifacts(ctx, successDB, successSM, args, artifacts)
	require.True(t, result.walletCreated)
	require.True(t, result.oauthCreated)
	require.Len(t, successSM.createInputs, 2)

	exitFn = func(_ int) { panic("exit") }

	failFirstDB := theoryMocks.NewMockExtendedDBStrict()
	failFirstBuilder := new(theoryMocks.MockTransactionBuilder)
	failFirstDB.TransactWriteBuilder = failFirstBuilder
	failFirstSM := &fakeSecretsManager{createErrs: []error{errors.New("boom")}}

	failFirstDB.On("TransactWrite", ctx, testifyMock.Anything).Return(nil).Twice()
	failFirstBuilder.On("Create", item, testifyMock.Anything).Return(failFirstBuilder).Once()
	failFirstBuilder.On("Delete", item, testifyMock.Anything).Return(failFirstBuilder).Once()
	failFirstBuilder.On("Execute").Return(nil).Twice()

	require.Panics(t, func() {
		_ = persistBootstrapArtifacts(ctx, failFirstDB, failFirstSM, args, artifacts)
	})

	failSecondDB := theoryMocks.NewMockExtendedDBStrict()
	failSecondBuilder := new(theoryMocks.MockTransactionBuilder)
	failSecondDB.TransactWriteBuilder = failSecondBuilder
	failSecondSM := &fakeSecretsManager{createErrs: []error{nil, errors.New("boom")}, deleteErrs: []error{nil}}

	failSecondDB.On("TransactWrite", ctx, testifyMock.Anything).Return(nil).Twice()
	failSecondBuilder.On("Create", item, testifyMock.Anything).Return(failSecondBuilder).Once()
	failSecondBuilder.On("Delete", item, testifyMock.Anything).Return(failSecondBuilder).Once()
	failSecondBuilder.On("Execute").Return(nil).Twice()

	require.Panics(t, func() {
		_ = persistBootstrapArtifacts(ctx, failSecondDB, failSecondSM, args, artifacts)
	})
	require.Len(t, failSecondSM.deleteInputs, 1)
}

func TestRunOwnerBootstrap_SkipAndFullFlow(t *testing.T) {
	originalLoadAWSConfigFn := loadAWSConfigFn
	originalNewTableTheoryDBFn := newTableTheoryDBFn
	originalNewSecretsManagerClientFn := newSecretsManagerClientFn
	originalNewKMSClientFn := newKMSClientFn
	originalWalletFn := generateEthereumWalletFn
	originalRSAFn := generateRSAKeyPairPEMFn
	originalEncryptFn := encryptWithKMSFn
	originalIDFn := generateOAuthClientIDFn
	originalSecretFn := generateOAuthClientSecretFn
	originalTableName := storagemodels.MainTableName
	t.Cleanup(func() {
		loadAWSConfigFn = originalLoadAWSConfigFn
		newTableTheoryDBFn = originalNewTableTheoryDBFn
		newSecretsManagerClientFn = originalNewSecretsManagerClientFn
		newKMSClientFn = originalNewKMSClientFn
		generateEthereumWalletFn = originalWalletFn
		generateRSAKeyPairPEMFn = originalRSAFn
		encryptWithKMSFn = originalEncryptFn
		generateOAuthClientIDFn = originalIDFn
		generateOAuthClientSecretFn = originalSecretFn
		storagemodels.MainTableName = originalTableName
	})

	loadAWSConfigFn = func(_ context.Context, _ ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
		return aws.Config{}, nil
	}

	// Skip flow: user exists.
	ctx := context.Background()
	skipDB := theoryMocks.NewMockExtendedDBStrict()
	skipQuery := new(theoryMocks.MockQuery)

	skipDB.On("Model", testifyMock.Anything).Return(skipQuery).Once()
	skipQuery.On("WithContext", ctx).Return(skipQuery).Once()
	skipQuery.On("Where", "PK", "=", "USER#admin").Return(skipQuery).Once()
	skipQuery.On("Where", "SK", "=", storagemodels.SKMetadata).Return(skipQuery).Once()
	skipQuery.On("ConsistentRead").Return(skipQuery).Once()
	skipQuery.On("First", testifyMock.Anything).Return(nil).Once()
	skipDB.On("Close").Return(nil).Once()

	skipSM := &fakeSecretsManager{describeErr: &smstypes.ResourceNotFoundException{}}
	newTableTheoryDBFn = func(aws.Config) (tableTheoryAPI, error) { return skipDB, nil }
	newSecretsManagerClientFn = func(aws.Config) secretsManagerAPI { return skipSM }
	newKMSClientFn = func(aws.Config) kmsAPI { return &fakeKMS{} }

	runOwnerBootstrap(ctx, ownerBootstrapArgs{
		environment: "dev",
		domain:      "example.com",
		tableName:   "tbl",
		kmsKeyID:    "kms",
		username:    "admin",
		chainID:     1,
	})

	skipDB.AssertNotCalled(t, "TransactWrite", testifyMock.Anything, testifyMock.Anything)
	require.Len(t, skipSM.createInputs, 0)

	// Full flow: nothing exists.
	fullDB := theoryMocks.NewMockExtendedDBStrict()
	fullQuery := new(theoryMocks.MockQuery)
	fullBuilder := new(theoryMocks.MockTransactionBuilder)
	fullDB.TransactWriteBuilder = fullBuilder

	fullDB.On("Model", testifyMock.Anything).Return(fullQuery).Once()
	fullQuery.On("WithContext", ctx).Return(fullQuery).Once()
	fullQuery.On("Where", "PK", "=", "USER#admin").Return(fullQuery).Once()
	fullQuery.On("Where", "SK", "=", storagemodels.SKMetadata).Return(fullQuery).Once()
	fullQuery.On("ConsistentRead").Return(fullQuery).Once()
	fullQuery.On("First", testifyMock.Anything).Return(theorydbErrors.ErrItemNotFound).Once()

	fullDB.On("TransactWrite", ctx, testifyMock.Anything).Return(nil).Once()
	fullBuilder.On("Create", testifyMock.Anything, testifyMock.Anything).Return(fullBuilder).Times(5)
	fullBuilder.On("Execute").Return(nil).Once()
	fullDB.On("Close").Return(nil).Once()

	fullSM := &fakeSecretsManager{describeErr: &smstypes.ResourceNotFoundException{}}
	newTableTheoryDBFn = func(aws.Config) (tableTheoryAPI, error) { return fullDB, nil }
	newSecretsManagerClientFn = func(aws.Config) secretsManagerAPI { return fullSM }
	newKMSClientFn = func(aws.Config) kmsAPI { return &fakeKMS{} }

	generateEthereumWalletFn = func() (string, string, error) { return "0xpriv", "0xADDR", nil }
	generateRSAKeyPairPEMFn = func(_ int) (string, string, error) { return "privpem", "pubpem", nil }
	encryptWithKMSFn = func(_ context.Context, _ kmsAPI, _ string, _ []byte) ([]byte, error) { return []byte("cipher"), nil }
	generateOAuthClientIDFn = func() (string, error) { return "cid", nil }
	generateOAuthClientSecretFn = func() (string, error) { return "csecret", nil }

	runOwnerBootstrap(ctx, ownerBootstrapArgs{
		environment: "dev",
		domain:      "example.com",
		tableName:   "tbl",
		kmsKeyID:    "kms",
		username:    "admin",
		chainID:     1,
	})
	require.Len(t, fullSM.createInputs, 2)
}

func TestRunOwnerBootstrap_FailureBranches(t *testing.T) {
	originalLoadAWSConfigFn := loadAWSConfigFn
	originalNewTableTheoryDBFn := newTableTheoryDBFn
	originalNewSecretsManagerClientFn := newSecretsManagerClientFn
	originalNewKMSClientFn := newKMSClientFn
	originalExitFn := exitFn
	t.Cleanup(func() {
		loadAWSConfigFn = originalLoadAWSConfigFn
		newTableTheoryDBFn = originalNewTableTheoryDBFn
		newSecretsManagerClientFn = originalNewSecretsManagerClientFn
		newKMSClientFn = originalNewKMSClientFn
		exitFn = originalExitFn
	})

	loadAWSConfigFn = func(_ context.Context, _ ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
		return aws.Config{}, nil
	}
	newKMSClientFn = func(aws.Config) kmsAPI { return &fakeKMS{} }
	exitFn = func(_ int) { panic("exit") }

	// Partial state: user missing but secrets exist.
	partialDB := theoryMocks.NewMockExtendedDBStrict()
	partialQuery := new(theoryMocks.MockQuery)
	partialDB.On("Model", testifyMock.Anything).Return(partialQuery).Once()
	partialQuery.On("WithContext", testifyMock.Anything).Return(partialQuery).Once()
	partialQuery.On("Where", "PK", "=", "USER#admin").Return(partialQuery).Once()
	partialQuery.On("Where", "SK", "=", storagemodels.SKMetadata).Return(partialQuery).Once()
	partialQuery.On("ConsistentRead").Return(partialQuery).Once()
	partialQuery.On("First", testifyMock.Anything).Return(theorydbErrors.ErrItemNotFound).Once()
	partialDB.On("Close").Return(nil).Once()
	newTableTheoryDBFn = func(aws.Config) (tableTheoryAPI, error) { return partialDB, nil }
	newSecretsManagerClientFn = func(aws.Config) secretsManagerAPI {
		return &fakeSecretsManager{describeErr: nil}
	}

	require.Panics(t, func() {
		runOwnerBootstrap(context.Background(), ownerBootstrapArgs{
			environment: "dev",
			domain:      "example.com",
			tableName:   "tbl",
			kmsKeyID:    "kms",
			username:    "admin",
			chainID:     1,
		})
	})

	// Force mode unsupported when user exists.
	forceDB := theoryMocks.NewMockExtendedDBStrict()
	forceQuery := new(theoryMocks.MockQuery)
	forceDB.On("Model", testifyMock.Anything).Return(forceQuery).Once()
	forceQuery.On("WithContext", testifyMock.Anything).Return(forceQuery).Once()
	forceQuery.On("Where", "PK", "=", "USER#admin").Return(forceQuery).Once()
	forceQuery.On("Where", "SK", "=", storagemodels.SKMetadata).Return(forceQuery).Once()
	forceQuery.On("ConsistentRead").Return(forceQuery).Once()
	forceQuery.On("First", testifyMock.Anything).Return(nil).Once()
	forceDB.On("Close").Return(nil).Once()
	newTableTheoryDBFn = func(aws.Config) (tableTheoryAPI, error) { return forceDB, nil }
	newSecretsManagerClientFn = func(aws.Config) secretsManagerAPI {
		return &fakeSecretsManager{describeErr: &smstypes.ResourceNotFoundException{}}
	}

	require.Panics(t, func() {
		runOwnerBootstrap(context.Background(), ownerBootstrapArgs{
			environment: "dev",
			domain:      "example.com",
			tableName:   "tbl",
			kmsKeyID:    "kms",
			username:    "admin",
			chainID:     1,
			force:       true,
		})
	})
}

func TestMain_SkipFlow(t *testing.T) {
	originalArgs := append([]string(nil), os.Args...)
	originalLoadAWSConfigFn := loadAWSConfigFn
	originalNewTableTheoryDBFn := newTableTheoryDBFn
	originalNewSecretsManagerClientFn := newSecretsManagerClientFn
	originalNewKMSClientFn := newKMSClientFn
	t.Cleanup(func() {
		os.Args = originalArgs
		loadAWSConfigFn = originalLoadAWSConfigFn
		newTableTheoryDBFn = originalNewTableTheoryDBFn
		newSecretsManagerClientFn = originalNewSecretsManagerClientFn
		newKMSClientFn = originalNewKMSClientFn
	})

	os.Args = []string{"owner-bootstrap", "-environment=dev", "-domain=example.com"}
	loadAWSConfigFn = func(_ context.Context, _ ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
		return aws.Config{}, nil
	}

	db := theoryMocks.NewMockExtendedDBStrict()
	q := new(theoryMocks.MockQuery)
	db.On("Model", testifyMock.Anything).Return(q).Once()
	q.On("WithContext", testifyMock.Anything).Return(q).Once()
	q.On("Where", "PK", "=", "USER#admin").Return(q).Once()
	q.On("Where", "SK", "=", storagemodels.SKMetadata).Return(q).Once()
	q.On("ConsistentRead").Return(q).Once()
	q.On("First", testifyMock.Anything).Return(nil).Once()
	db.On("Close").Return(nil).Once()

	sm := &fakeSecretsManager{describeErr: &smstypes.ResourceNotFoundException{}}

	newTableTheoryDBFn = func(aws.Config) (tableTheoryAPI, error) { return db, nil }
	newSecretsManagerClientFn = func(aws.Config) secretsManagerAPI { return sm }
	newKMSClientFn = func(aws.Config) kmsAPI { return &fakeKMS{} }

	main()

	db.AssertNotCalled(t, "TransactWrite", testifyMock.Anything, testifyMock.Anything)
	require.Len(t, sm.createInputs, 0)
}

func TestMain_ParseAndValidateFailuresExit(t *testing.T) {
	originalArgs := append([]string(nil), os.Args...)
	originalExitFn := exitFn
	t.Cleanup(func() {
		os.Args = originalArgs
		exitFn = originalExitFn
	})

	exitFn = func(_ int) { panic("exit") }

	os.Args = []string{"owner-bootstrap", "-nope"}
	require.Panics(t, func() { main() })

	os.Args = []string{"owner-bootstrap", "-environment=dev"}
	require.Panics(t, func() { main() })
}

func TestGenerateBootstrapArtifacts_FailureBranchesExit(t *testing.T) {
	originalExitFn := exitFn
	originalWalletFn := generateEthereumWalletFn
	originalRSAFn := generateRSAKeyPairPEMFn
	t.Cleanup(func() {
		exitFn = originalExitFn
		generateEthereumWalletFn = originalWalletFn
		generateRSAKeyPairPEMFn = originalRSAFn
	})

	exitFn = func(_ int) { panic("exit") }
	generateEthereumWalletFn = func() (string, string, error) { return "", "", errors.New("boom") }

	require.Panics(t, func() {
		_ = generateBootstrapArtifacts(context.Background(), &fakeKMS{}, ownerBootstrapArgs{
			environment:      "dev",
			domain:           "example.com",
			tableName:        "tbl",
			kmsKeyID:         "kms",
			walletSecretName: "wallet",
			oauthSecretName:  "oauth",
			username:         "admin",
			chainID:          1,
		}, time.Unix(0, 0).UTC())
	})

	generateEthereumWalletFn = func() (string, string, error) { return "0xpriv", "0xADDR", nil }
	generateRSAKeyPairPEMFn = func(_ int) (string, string, error) { return "", "", errors.New("boom") }

	require.Panics(t, func() {
		_ = generateBootstrapArtifacts(context.Background(), &fakeKMS{}, ownerBootstrapArgs{
			environment:      "dev",
			domain:           "example.com",
			tableName:        "tbl",
			kmsKeyID:         "kms",
			walletSecretName: "wallet",
			oauthSecretName:  "oauth",
			username:         "admin",
			chainID:          1,
		}, time.Unix(0, 0).UTC())
	})
}
