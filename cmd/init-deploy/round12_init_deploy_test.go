package main

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	pkgErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
	dynamormCore "github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormmocks "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

type fakeSecretsClient struct {
	createCalls  int
	updateCalls  int
	createdNames []string
	updatedNames []string
	createErr    error
	updateErr    error
}

func (f *fakeSecretsClient) CreateSecret(_ context.Context, params *secretsmanager.CreateSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error) {
	f.createCalls++
	if params != nil && params.Name != nil {
		f.createdNames = append(f.createdNames, *params.Name)
	}
	if f.createErr != nil {
		return nil, f.createErr
	}
	return &secretsmanager.CreateSecretOutput{}, nil
}

func (f *fakeSecretsClient) UpdateSecret(_ context.Context, params *secretsmanager.UpdateSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.UpdateSecretOutput, error) {
	f.updateCalls++
	if params != nil && params.SecretId != nil {
		f.updatedNames = append(f.updatedNames, *params.SecretId)
	}
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return &secretsmanager.UpdateSecretOutput{}, nil
}

type fakeUserRepo struct {
	createCalls int
	lastUser    *storage.User
	err         error
}

func (f *fakeUserRepo) CreateUser(_ context.Context, user *storage.User) error {
	f.createCalls++
	f.lastUser = user
	return f.err
}

type fakeRepoFactory struct {
	userRepo *fakeUserRepo
}

func (f *fakeRepoFactory) User() userCreator {
	return f.userRepo
}

func TestStoreSecret_CreateAndUpdate_Round12(t *testing.T) {
	client := &fakeSecretsClient{}
	require.NoError(t, storeSecret(context.Background(), client, "a", "b"))
	require.Equal(t, 1, client.createCalls)
	require.Equal(t, 0, client.updateCalls)

	client = &fakeSecretsClient{createErr: errors.New("already exists")}
	require.NoError(t, storeSecret(context.Background(), client, "a", "b"))
	require.Equal(t, 1, client.createCalls)
	require.Equal(t, 1, client.updateCalls)

	client = &fakeSecretsClient{createErr: errors.New("already exists"), updateErr: errors.New("boom")}
	err := storeSecret(context.Background(), client, "a", "b")
	require.True(t, pkgErrors.HasCode(err, pkgErrors.CodeInternal))
}

func TestGenerateVAPIDKeys_Success_Round12(t *testing.T) {
	publicKey, privateKey, err := generateVAPIDKeys()
	require.NoError(t, err)
	require.NotEmpty(t, publicKey)
	require.Contains(t, privateKey, "EC PRIVATE KEY")
}

func TestGenerateVAPIDKeys_ErrorBranch_Round12(t *testing.T) {
	origReader := rand.Reader
	t.Cleanup(func() { rand.Reader = origReader })
	rand.Reader = failingReader{}

	_, _, err := generateVAPIDKeys()
	require.True(t, pkgErrors.HasCode(err, pkgErrors.CodeInternal))
}

func TestGenerateSecurePassword_Success_Round12(t *testing.T) {
	pw, err := generateSecurePassword(32)
	require.NoError(t, err)
	require.Len(t, pw, 32)
}

func TestGenerateSecurePassword_ErrorBranch_Round12(t *testing.T) {
	origReader := rand.Reader
	t.Cleanup(func() { rand.Reader = origReader })
	rand.Reader = io.LimitReader(failingReader{}, 1)

	_, err := generateSecurePassword(4)
	require.Error(t, err)
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("no entropy")
}

func TestRunInitDeploy_SuccessAndDomainFallback_Round12(t *testing.T) {
	origMust := mustInitializeLambdaFn
	origCfg := getAppConfigFn
	origLoad := loadAWSConfigFn
	origNewSecrets := newSecretsManagerClientFn
	origVapid := generateVAPIDKeysFn
	origPass := generateSecurePasswordFn
	origDyn := getDynamormClientFn
	origFactory := newRepositoryFactoryFn
	origHash := hashPasswordFn
	t.Cleanup(func() {
		mustInitializeLambdaFn = origMust
		getAppConfigFn = origCfg
		loadAWSConfigFn = origLoad
		newSecretsManagerClientFn = origNewSecrets
		generateVAPIDKeysFn = origVapid
		generateSecurePasswordFn = origPass
		getDynamormClientFn = origDyn
		newRepositoryFactoryFn = origFactory
		hashPasswordFn = origHash
	})

	userRepo := &fakeUserRepo{}
	secrets := &fakeSecretsClient{}

	mustInitializeLambdaFn = func(common.LambdaConfig) *common.LambdaContext {
		return &common.LambdaContext{
			Config: &config.Config{
				DynamoTableName: "table",
				AWSAccountID:    "123",
				JWTSecret:       "",
			},
			Logger: zap.NewNop(),
		}
	}

	getAppConfigFn = func() *config.Config { return &config.Config{Domain: "example.com"} }
	loadAWSConfigFn = func(context.Context, ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
		return aws.Config{Region: "us-east-1"}, nil
	}
	newSecretsManagerClientFn = func(aws.Config) secretsClient { return secrets }
	generateVAPIDKeysFn = func() (string, string, error) { return "pub", "priv", nil }
	generateSecurePasswordFn = func(int) (string, error) { return "pw", nil }
	getDynamormClientFn = func(context.Context) (dynamormCore.DB, error) { return nil, nil }
	newRepositoryFactoryFn = func(dynamormCore.DB, string, *zap.Logger) (userRepositoryFactory, error) {
		return &fakeRepoFactory{userRepo: userRepo}, nil
	}
	hashPasswordFn = func(string) (string, error) { return "hashed", nil }

	require.NoError(t, runInitDeploy(context.Background(), nil))
	require.Equal(t, 3, secrets.createCalls)
	require.Equal(t, 1, userRepo.createCalls)
	require.NotNil(t, userRepo.lastUser)

	secrets.createdNames = nil
	getAppConfigFn = func() *config.Config { return &config.Config{Domain: ""} }
	require.NoError(t, runInitDeploy(context.Background(), []string{"from-args.example"}))
	require.Contains(t, secrets.createdNames, "lesser/from-args.example/vapid-keys")
}

func TestRunInitDeploy_MissingDomain_Round12(t *testing.T) {
	origCfg := getAppConfigFn
	t.Cleanup(func() { getAppConfigFn = origCfg })

	getAppConfigFn = func() *config.Config { return &config.Config{Domain: ""} }
	err := runInitDeploy(context.Background(), nil)
	require.Error(t, err)
}

func TestRepositoryFactoryAdapter_User_Round12(t *testing.T) {
	userRepo := &fakeUserRepo{}
	adapter := repositoryFactoryAdapter{userRepo: userRepo}

	got := adapter.User()
	require.NotNil(t, got)
	require.NoError(t, got.CreateUser(context.Background(), &storage.User{}))
	require.Equal(t, 1, userRepo.createCalls)
}

func TestRunInitDeploy_ErrorBranches_Round12(t *testing.T) {
	origMust := mustInitializeLambdaFn
	origCfg := getAppConfigFn
	origLoad := loadAWSConfigFn
	origNewSecrets := newSecretsManagerClientFn
	origVapid := generateVAPIDKeysFn
	origPass := generateSecurePasswordFn
	origDyn := getDynamormClientFn
	origFactory := newRepositoryFactoryFn
	origHash := hashPasswordFn
	t.Cleanup(func() {
		mustInitializeLambdaFn = origMust
		getAppConfigFn = origCfg
		loadAWSConfigFn = origLoad
		newSecretsManagerClientFn = origNewSecrets
		generateVAPIDKeysFn = origVapid
		generateSecurePasswordFn = origPass
		getDynamormClientFn = origDyn
		newRepositoryFactoryFn = origFactory
		hashPasswordFn = origHash
	})

	baseUserRepo := &fakeUserRepo{}
	baseSecrets := &fakeSecretsClient{}

	baseMust := func(common.LambdaConfig) *common.LambdaContext {
		return &common.LambdaContext{
			Config: &config.Config{
				DynamoTableName: "table",
				AWSAccountID:    "123",
				JWTSecret:       "",
			},
			Logger: zap.NewNop(),
		}
	}
	baseCfg := func() *config.Config { return &config.Config{Domain: "example.com"} }
	baseLoad := func(context.Context, ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
		return aws.Config{Region: "us-east-1"}, nil
	}
	baseNewSecrets := func(aws.Config) secretsClient { return baseSecrets }
	baseVapid := func() (string, string, error) { return "pub", "priv", nil }
	basePass := func(int) (string, error) { return "pw", nil }
	baseDyn := func(context.Context) (dynamormCore.DB, error) { return nil, nil }
	baseFactory := func(dynamormCore.DB, string, *zap.Logger) (userRepositoryFactory, error) {
		return &fakeRepoFactory{userRepo: baseUserRepo}, nil
	}
	baseHash := func(string) (string, error) { return "hashed", nil }

	reset := func() {
		baseUserRepo.createCalls = 0
		baseUserRepo.err = nil
		baseSecrets.createCalls = 0
		baseSecrets.updateCalls = 0
		baseSecrets.createErr = nil
		baseSecrets.updateErr = nil

		mustInitializeLambdaFn = baseMust
		getAppConfigFn = baseCfg
		loadAWSConfigFn = baseLoad
		newSecretsManagerClientFn = baseNewSecrets
		generateVAPIDKeysFn = baseVapid
		generateSecurePasswordFn = basePass
		getDynamormClientFn = baseDyn
		newRepositoryFactoryFn = baseFactory
		hashPasswordFn = baseHash
	}

	cases := []struct {
		name  string
		setup func()
	}{
		{
			name: "aws_config_load_fails",
			setup: func() {
				loadAWSConfigFn = func(context.Context, ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
					return aws.Config{}, errors.New("no aws")
				}
			},
		},
		{
			name: "vapid_key_generation_fails",
			setup: func() {
				generateVAPIDKeysFn = func() (string, string, error) { return "", "", errors.New("no keys") }
			},
		},
		{
			name: "store_secret_fails",
			setup: func() {
				baseSecrets.createErr = errors.New("exists")
				baseSecrets.updateErr = errors.New("boom")
			},
		},
		{
			name: "dynamorm_client_fails",
			setup: func() {
				getDynamormClientFn = func(context.Context) (dynamormCore.DB, error) { return nil, errors.New("no db") }
			},
		},
		{
			name: "hash_password_fails",
			setup: func() {
				hashPasswordFn = func(string) (string, error) { return "", errors.New("no hash") }
			},
		},
		{
			name: "create_user_fails",
			setup: func() {
				baseUserRepo.err = errors.New("no create")
			},
		},
		{
			name: "jwt_secret_generation_fails",
			setup: func() {
				calls := 0
				generateSecurePasswordFn = func(int) (string, error) {
					calls++
					if calls == 2 {
						return "", errors.New("no jwt")
					}
					return "pw", nil
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reset()
			if tc.setup != nil {
				tc.setup()
			}
			err := runInitDeploy(context.Background(), nil)
			require.Error(t, err)
		})
	}
}

func TestNewRepositoryFactoryFn_Default_Round12(t *testing.T) {
	db := dynamormmocks.NewMockExtendedDB()
	repos, err := newRepositoryFactoryFn(db, "table", zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, repos)
	require.NotNil(t, repos.User())
}

func TestNewRepositoryFactoryFn_Default_Error_Round12(t *testing.T) {
	repos, err := newRepositoryFactoryFn(nil, "table", zap.NewNop())
	require.Error(t, err)
	require.Nil(t, repos)
}

func TestNewSecretsManagerClientFn_Default_Round12(t *testing.T) {
	client := newSecretsManagerClientFn(aws.Config{Region: "us-east-1"})
	require.NotNil(t, client)
}
