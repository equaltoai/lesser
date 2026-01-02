package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type apiError struct {
	code string
}

func (e apiError) Error() string     { return e.code }
func (e apiError) ErrorCode() string { return e.code }

type fakeSecretsClient struct {
	secrets   map[string]string
	getErrors []error

	getCalls    int
	createCalls int
	updateCalls int
	deleteCalls int
	listCalls   int

	listErr error
}

func newFakeSecretsClient() *fakeSecretsClient {
	return &fakeSecretsClient{secrets: make(map[string]string)}
}

func (c *fakeSecretsClient) CreateSecret(_ context.Context, params *secretsmanager.CreateSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error) {
	c.createCalls++
	name := aws.ToString(params.Name)
	if _, exists := c.secrets[name]; exists {
		return nil, apiError{code: "ResourceExistsException"}
	}
	c.secrets[name] = aws.ToString(params.SecretString)
	return &secretsmanager.CreateSecretOutput{}, nil
}

func (c *fakeSecretsClient) UpdateSecret(_ context.Context, params *secretsmanager.UpdateSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.UpdateSecretOutput, error) {
	c.updateCalls++
	id := aws.ToString(params.SecretId)
	c.secrets[id] = aws.ToString(params.SecretString)
	return &secretsmanager.UpdateSecretOutput{}, nil
}

func (c *fakeSecretsClient) GetSecretValue(_ context.Context, params *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	c.getCalls++
	if len(c.getErrors) > 0 {
		err := c.getErrors[0]
		c.getErrors = c.getErrors[1:]
		return nil, err
	}

	id := aws.ToString(params.SecretId)
	val, ok := c.secrets[id]
	if !ok {
		return nil, apiError{code: "ResourceNotFoundException"}
	}
	return &secretsmanager.GetSecretValueOutput{SecretString: aws.String(val)}, nil
}

func (c *fakeSecretsClient) DeleteSecret(_ context.Context, params *secretsmanager.DeleteSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error) {
	c.deleteCalls++
	id := aws.ToString(params.SecretId)
	if _, ok := c.secrets[id]; !ok {
		return nil, apiError{code: "ResourceNotFoundException"}
	}
	delete(c.secrets, id)
	return &secretsmanager.DeleteSecretOutput{}, nil
}

func (c *fakeSecretsClient) ListSecrets(_ context.Context, _ *secretsmanager.ListSecretsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretsOutput, error) {
	c.listCalls++
	if c.listErr != nil {
		return nil, c.listErr
	}
	return &secretsmanager.ListSecretsOutput{}, nil
}

func newTestAWSSecretsManager(client secretsManagerClient) *AWSSecretsManager {
	return &AWSSecretsManager{
		client:      client,
		logger:      zap.NewNop(),
		keyPrefix:   "lesser/test",
		region:      "us-east-1",
		cacheTTL:    50 * time.Millisecond,
		description: "test",
		cache: &secretCache{
			entries: make(map[string]*cacheEntry),
		},
	}
}

func generateTestPrivateKeyPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func TestAWSSecretsManager_StoreRetrieveDeletePrivateKey_WithCacheAndFallbacks(t *testing.T) {
	t.Parallel()

	client := newFakeSecretsClient()
	sm := newTestAWSSecretsManager(client)

	keyID := "actor-1"
	privateKeyPEM := generateTestPrivateKeyPEM(t)

	// Prime cache then store should invalidate.
	sm.putInCache(keyID, "cached")
	require.NoError(t, sm.StorePrivateKey(context.Background(), keyID, privateKeyPEM))
	require.Empty(t, sm.getFromCache(keyID))

	// Store again should hit update path.
	require.NoError(t, sm.StorePrivateKey(context.Background(), keyID, privateKeyPEM))
	require.GreaterOrEqual(t, client.updateCalls, 1)

	got, err := sm.RetrievePrivateKey(context.Background(), keyID)
	require.NoError(t, err)
	require.Equal(t, privateKeyPEM, got)
	require.NotEmpty(t, sm.getFromCache(keyID))

	// Second retrieve should use cache.
	before := client.getCalls
	_, err = sm.RetrievePrivateKey(context.Background(), keyID)
	require.NoError(t, err)
	require.Equal(t, before, client.getCalls)

	// Delete should remove secret and invalidate cache.
	require.NoError(t, sm.DeletePrivateKey(context.Background(), keyID))
	require.Empty(t, sm.getFromCache(keyID))

	// Deleting again is treated as success.
	require.NoError(t, sm.DeletePrivateKey(context.Background(), keyID))
}

func TestAWSSecretsManager_RetrievePrivateKey_RetryAndUnmarshalFailures(t *testing.T) {
	t.Parallel()

	client := newFakeSecretsClient()
	sm := newTestAWSSecretsManager(client)

	keyID := "actor-2"
	secretName := sm.getSecretName(keyID)

	privateKeyPEM := generateTestPrivateKeyPEM(t)
	secretJSON, err := json.Marshal(SecretValue{
		PrivateKeyPEM: privateKeyPEM,
		CreatedAt:     time.Now(),
		KeyType:       "RSA",
		Version:       "1.0",
	})
	require.NoError(t, err)
	client.secrets[secretName] = string(secretJSON)

	// Fail once to exercise retry backoff.
	client.getErrors = []error{errors.New("temporary")}

	got, err := sm.RetrievePrivateKey(context.Background(), keyID)
	require.NoError(t, err)
	require.Equal(t, privateKeyPEM, got)

	// Unmarshal failure.
	client2 := newFakeSecretsClient()
	sm2 := newTestAWSSecretsManager(client2)
	client2.secrets[sm2.getSecretName("actor-3")] = "not-json"
	_, err = sm2.RetrievePrivateKey(context.Background(), "actor-3")
	require.ErrorIs(t, err, ErrSecretValueUnmarshal)
}

func TestAWSSecretsManager_KeyPairHelpersAndCacheStats(t *testing.T) {
	t.Parallel()

	client := newFakeSecretsClient()
	sm := newTestAWSSecretsManager(client)

	publicKey, privateKey, err := sm.GenerateAndStoreKeyPair(context.Background(), "actor-4")
	require.NoError(t, err)
	require.Contains(t, publicKey, "PUBLIC KEY")
	require.Contains(t, privateKey, "PRIVATE KEY")

	stats := sm.GetCacheStats()
	require.Contains(t, stats, "total_entries")
	require.Contains(t, stats, "cache_ttl")

	sm.putInCache("expired", "x")
	sm.cache.entries["expired"].expiresAt = time.Now().Add(-time.Second)
	sm.CleanupCache()
	require.Empty(t, sm.getFromCache("expired"))

	// RotateKey should call GenerateAndStoreKeyPair.
	_, _, err = sm.RotateKey(context.Background(), "actor-5")
	require.NoError(t, err)
}

func TestSecretsManager_ErrorHelpers(t *testing.T) {
	t.Parallel()

	sm := newTestAWSSecretsManager(newFakeSecretsClient())

	require.False(t, sm.isSecretAlreadyExistsError(nil))
	require.True(t, sm.isSecretAlreadyExistsError(apiError{code: "ResourceExistsException"}))
	require.True(t, sm.isSecretNotFoundError(apiError{code: "ResourceNotFoundException"}))

	require.False(t, containsError(errors.New("x"), "ResourceExistsException"))
	require.True(t, containsError(apiError{code: "ResourceExistsException"}, "ResourceExistsException"))
	require.True(t, containsError(errors.New("ResourceExistsException"), "ResourceExistsException"))
}

func TestNewAWSSecretsManager_InternalConstructor_DefaultsAndConnectivity(t *testing.T) {
	t.Parallel()

	client := newFakeSecretsClient()
	loadConfig := func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return aws.Config{}, nil
	}
	newClient := func(aws.Config) secretsManagerClient { return client }

	sm, err := newAWSSecretsManager(SecretsManagerConfig{}, nil, loadConfig, newClient)
	require.NoError(t, err)
	require.Equal(t, "us-east-1", sm.region)
	require.Equal(t, "lesser/actor-keys", sm.keyPrefix)
	require.Equal(t, 5*time.Minute, sm.cacheTTL)
	require.Equal(t, "Lesser ActivityPub actor private keys", sm.description)
	require.Equal(t, 1, client.listCalls)

	// Config load failure.
	_, err = newAWSSecretsManager(SecretsManagerConfig{}, nil, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return aws.Config{}, errors.New("load failed")
	}, newClient)
	require.ErrorIs(t, err, ErrAWSConfigLoad)

	// Connectivity failure.
	client2 := newFakeSecretsClient()
	client2.listErr = errors.New("forbidden")
	_, err = newAWSSecretsManager(SecretsManagerConfig{Region: "us-east-1"}, zap.NewNop(), loadConfig, func(aws.Config) secretsManagerClient {
		return client2
	})
	require.ErrorIs(t, err, ErrSecretsManagerConnection)
}

func TestAWSSecretsManager_validatePrivateKey(t *testing.T) {
	t.Parallel()

	sm := newTestAWSSecretsManager(newFakeSecretsClient())

	require.ErrorIs(t, sm.validatePrivateKey("not-pem"), ErrPEMBlockDecode)

	badPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte{1, 2, 3}}))
	require.ErrorIs(t, sm.validatePrivateKey(badPEM), ErrPrivateKeyParse)

	// PKCS1.
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pkcs1 := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	require.NoError(t, sm.validatePrivateKey(pkcs1))

	// PKCS8.
	require.NoError(t, sm.validatePrivateKey(generateTestPrivateKeyPEM(t)))
}
