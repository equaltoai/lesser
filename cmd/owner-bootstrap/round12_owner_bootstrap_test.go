package main

import (
	"context"
	"errors"
	"math"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamotypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	smstypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/stretchr/testify/require"
)

type fakeDynamoDB struct {
	getItemInput  *dynamodb.GetItemInput
	getItemOutput *dynamodb.GetItemOutput
	getItemErr    error

	transactInput *dynamodb.TransactWriteItemsInput
	transactErr   error
}

func (f *fakeDynamoDB) GetItem(_ context.Context, params *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	f.getItemInput = params
	if f.getItemErr != nil {
		return nil, f.getItemErr
	}
	if f.getItemOutput != nil {
		return f.getItemOutput, nil
	}
	return &dynamodb.GetItemOutput{}, nil
}

func (f *fakeDynamoDB) TransactWriteItems(_ context.Context, params *dynamodb.TransactWriteItemsInput, _ ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	f.transactInput = params
	if f.transactErr != nil {
		return nil, f.transactErr
	}
	return &dynamodb.TransactWriteItemsOutput{}, nil
}

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

func TestDynamoItemExists(t *testing.T) {
	ddb := &fakeDynamoDB{
		getItemOutput: &dynamodb.GetItemOutput{
			Item: map[string]dynamotypes.AttributeValue{"PK": &dynamotypes.AttributeValueMemberS{Value: "USER#admin"}},
		},
	}

	exists, err := dynamoItemExists(context.Background(), ddb, "tbl", "USER#admin", "METADATA")
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, "tbl", aws.ToString(ddb.getItemInput.TableName))

	ddb.getItemOutput = &dynamodb.GetItemOutput{Item: map[string]dynamotypes.AttributeValue{}}
	exists, err = dynamoItemExists(context.Background(), ddb, "tbl", "USER#admin", "METADATA")
	require.NoError(t, err)
	require.False(t, exists)

	ddb.getItemErr = errors.New("boom")
	_, err = dynamoItemExists(context.Background(), ddb, "tbl", "USER#admin", "METADATA")
	require.Error(t, err)
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

func TestEncodeDescendingTimestamp_ZeroTimestamp(t *testing.T) {
	nonZero := time.Unix(1, 0).UTC()
	value := encodeDescendingTimestamp(nonZero)
	require.Equal(t, int64(math.MaxInt64-nonZero.UnixNano()), value)

	zeroValue := encodeDescendingTimestamp(time.Time{})
	require.Greater(t, zeroValue, int64(0))
	require.Less(t, zeroValue, int64(math.MaxInt64))
}

func TestCheckBootstrapState_ErrorBranches(t *testing.T) {
	ctx := context.Background()
	args := ownerBootstrapArgs{tableName: "tbl", walletSecretName: "wallet", oauthSecretName: "oauth"}

	_, err := checkBootstrapState(ctx, &fakeDynamoDB{getItemErr: errors.New("boom")}, &fakeSecretsManager{}, args, "USER#admin")
	require.Error(t, err)

	_, err = checkBootstrapState(ctx, &fakeDynamoDB{getItemOutput: &dynamodb.GetItemOutput{}}, &fakeSecretsManager{describeErr: errors.New("boom")}, args, "USER#admin")
	require.Error(t, err)
}

func TestTransactWriteAll_BuildsPutRequests(t *testing.T) {
	ddb := &fakeDynamoDB{}

	puts := []transactPut{
		{
			item: map[string]dynamotypes.AttributeValue{"PK": &dynamotypes.AttributeValueMemberS{Value: "1"}},
			pk:   "1",
			sk:   "a",
		},
		{
			item: map[string]dynamotypes.AttributeValue{"PK": &dynamotypes.AttributeValueMemberS{Value: "2"}},
			pk:   "2",
			sk:   "b",
		},
	}

	require.NoError(t, transactWriteAll(context.Background(), ddb, "tbl", puts))
	require.NotNil(t, ddb.transactInput)
	require.Len(t, ddb.transactInput.TransactItems, 2)
	for _, item := range ddb.transactInput.TransactItems {
		require.NotNil(t, item.Put)
		require.Equal(t, "tbl", aws.ToString(item.Put.TableName))
		require.Equal(t, "attribute_not_exists(PK)", aws.ToString(item.Put.ConditionExpression))
	}

	ddb.transactErr = errors.New("boom")
	require.Error(t, transactWriteAll(context.Background(), ddb, "tbl", puts))
}

func TestRollbackSecretsAndDynamo_Branches(t *testing.T) {
	ctx := context.Background()
	ddb := &fakeDynamoDB{}
	sm := &fakeSecretsManager{deleteErrs: []error{nil, errors.New("boom")}}

	rollbackSecretsAndDynamo(ctx, ddb, sm, "tbl", nil, nil)
	require.Nil(t, ddb.transactInput)

	puts := []transactPut{
		{
			pk: "USER#admin",
			sk: "METADATA",
		},
	}

	ddb.transactErr = errors.New("boom")
	rollbackSecretsAndDynamo(ctx, ddb, sm, "tbl", []string{"s1", "s2"}, puts)
	require.Len(t, sm.deleteInputs, 2)
	require.NotNil(t, ddb.transactInput)
	require.Len(t, ddb.transactInput.TransactItems, 1)
	require.NotNil(t, ddb.transactInput.TransactItems[0].Delete)
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
	t.Cleanup(func() {
		exitFn = originalExitFn
	})

	ctx := context.Background()

	successDDB := &fakeDynamoDB{}
	successSM := &fakeSecretsManager{}
	args := ownerBootstrapArgs{tableName: "tbl", walletSecretName: "wallet", oauthSecretName: "oauth"}
	artifacts := bootstrapArtifacts{
		items: []transactPut{{pk: "USER#admin", sk: "METADATA"}},
	}
	result := persistBootstrapArtifacts(ctx, successDDB, successSM, args, artifacts)
	require.True(t, result.walletCreated)
	require.True(t, result.oauthCreated)
	require.Len(t, successSM.createInputs, 2)

	exitFn = func(_ int) { panic("exit") }

	failFirstSecret := &fakeSecretsManager{createErrs: []error{errors.New("boom")}}
	require.Panics(t, func() {
		_ = persistBootstrapArtifacts(ctx, &fakeDynamoDB{}, failFirstSecret, args, artifacts)
	})

	failSecondSecret := &fakeSecretsManager{createErrs: []error{nil, errors.New("boom")}}
	require.Panics(t, func() {
		_ = persistBootstrapArtifacts(ctx, &fakeDynamoDB{}, failSecondSecret, args, artifacts)
	})
}

func TestRunOwnerBootstrap_SkipAndFullFlow(t *testing.T) {
	originalLoadAWSConfigFn := loadAWSConfigFn
	originalNewDynamoClientFn := newDynamoClientFn
	originalNewSecretsManagerClientFn := newSecretsManagerClientFn
	originalNewKMSClientFn := newKMSClientFn
	originalWalletFn := generateEthereumWalletFn
	originalRSAFn := generateRSAKeyPairPEMFn
	originalEncryptFn := encryptWithKMSFn
	originalIDFn := generateOAuthClientIDFn
	originalSecretFn := generateOAuthClientSecretFn
	t.Cleanup(func() {
		loadAWSConfigFn = originalLoadAWSConfigFn
		newDynamoClientFn = originalNewDynamoClientFn
		newSecretsManagerClientFn = originalNewSecretsManagerClientFn
		newKMSClientFn = originalNewKMSClientFn
		generateEthereumWalletFn = originalWalletFn
		generateRSAKeyPairPEMFn = originalRSAFn
		encryptWithKMSFn = originalEncryptFn
		generateOAuthClientIDFn = originalIDFn
		generateOAuthClientSecretFn = originalSecretFn
	})

	loadAWSConfigFn = func(_ context.Context, _ ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
		return aws.Config{}, nil
	}

	// Skip path: user exists.
	skipDDB := &fakeDynamoDB{getItemOutput: &dynamodb.GetItemOutput{Item: map[string]dynamotypes.AttributeValue{"PK": &dynamotypes.AttributeValueMemberS{Value: "USER#admin"}}}}
	skipSM := &fakeSecretsManager{describeErr: &smstypes.ResourceNotFoundException{}}
	newDynamoClientFn = func(aws.Config) dynamodbAPI { return skipDDB }
	newSecretsManagerClientFn = func(aws.Config) secretsManagerAPI { return skipSM }
	newKMSClientFn = func(aws.Config) kmsAPI { return &fakeKMS{} }

	runOwnerBootstrap(context.Background(), ownerBootstrapArgs{
		environment: "dev",
		domain:      "example.com",
		tableName:   "tbl",
		kmsKeyID:    "kms",
		username:    "admin",
		chainID:     1,
	})
	require.Nil(t, skipDDB.transactInput)
	require.Len(t, skipSM.createInputs, 0)

	// Full flow: nothing exists.
	fullDDB := &fakeDynamoDB{getItemOutput: &dynamodb.GetItemOutput{Item: map[string]dynamotypes.AttributeValue{}}}
	fullSM := &fakeSecretsManager{describeErr: &smstypes.ResourceNotFoundException{}}
	newDynamoClientFn = func(aws.Config) dynamodbAPI { return fullDDB }
	newSecretsManagerClientFn = func(aws.Config) secretsManagerAPI { return fullSM }
	newKMSClientFn = func(aws.Config) kmsAPI { return &fakeKMS{} }

	generateEthereumWalletFn = func() (string, string, error) { return "0xpriv", "0xADDR", nil }
	generateRSAKeyPairPEMFn = func(_ int) (string, string, error) { return "privpem", "pubpem", nil }
	encryptWithKMSFn = func(_ context.Context, _ kmsAPI, _ string, _ []byte) ([]byte, error) { return []byte("cipher"), nil }
	generateOAuthClientIDFn = func() (string, error) { return "cid", nil }
	generateOAuthClientSecretFn = func() (string, error) { return "csecret", nil }

	runOwnerBootstrap(context.Background(), ownerBootstrapArgs{
		environment: "dev",
		domain:      "example.com",
		tableName:   "tbl",
		kmsKeyID:    "kms",
		username:    "admin",
		chainID:     1,
	})
	require.NotNil(t, fullDDB.transactInput)
	require.Len(t, fullSM.createInputs, 2)
}

func TestRunOwnerBootstrap_FailureBranches(t *testing.T) {
	originalLoadAWSConfigFn := loadAWSConfigFn
	originalNewDynamoClientFn := newDynamoClientFn
	originalNewSecretsManagerClientFn := newSecretsManagerClientFn
	originalNewKMSClientFn := newKMSClientFn
	originalExitFn := exitFn
	t.Cleanup(func() {
		loadAWSConfigFn = originalLoadAWSConfigFn
		newDynamoClientFn = originalNewDynamoClientFn
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
	newDynamoClientFn = func(aws.Config) dynamodbAPI {
		return &fakeDynamoDB{getItemOutput: &dynamodb.GetItemOutput{Item: map[string]dynamotypes.AttributeValue{}}}
	}
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
	newDynamoClientFn = func(aws.Config) dynamodbAPI {
		return &fakeDynamoDB{getItemOutput: &dynamodb.GetItemOutput{Item: map[string]dynamotypes.AttributeValue{"PK": &dynamotypes.AttributeValueMemberS{Value: "USER#admin"}}}}
	}
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
	originalNewDynamoClientFn := newDynamoClientFn
	originalNewSecretsManagerClientFn := newSecretsManagerClientFn
	originalNewKMSClientFn := newKMSClientFn
	t.Cleanup(func() {
		os.Args = originalArgs
		loadAWSConfigFn = originalLoadAWSConfigFn
		newDynamoClientFn = originalNewDynamoClientFn
		newSecretsManagerClientFn = originalNewSecretsManagerClientFn
		newKMSClientFn = originalNewKMSClientFn
	})

	os.Args = []string{"owner-bootstrap", "-environment=dev", "-domain=example.com"}
	loadAWSConfigFn = func(_ context.Context, _ ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
		return aws.Config{}, nil
	}

	ddb := &fakeDynamoDB{
		getItemOutput: &dynamodb.GetItemOutput{
			Item: map[string]dynamotypes.AttributeValue{"PK": &dynamotypes.AttributeValueMemberS{Value: "USER#admin"}},
		},
	}
	sm := &fakeSecretsManager{describeErr: &smstypes.ResourceNotFoundException{}}

	newDynamoClientFn = func(aws.Config) dynamodbAPI { return ddb }
	newSecretsManagerClientFn = func(aws.Config) secretsManagerAPI { return sm }
	newKMSClientFn = func(aws.Config) kmsAPI { return &fakeKMS{} }

	main()

	require.Nil(t, ddb.transactInput)
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
