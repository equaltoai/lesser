package auth

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type secretsClientStub struct {
	createErr error
	updateErr error

	getOut *secretsmanager.GetSecretValueOutput
	getErr error

	deleteErr error
}

func (c *secretsClientStub) CreateSecret(_ context.Context, _ *secretsmanager.CreateSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error) {
	return &secretsmanager.CreateSecretOutput{}, c.createErr
}

func (c *secretsClientStub) UpdateSecret(_ context.Context, _ *secretsmanager.UpdateSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.UpdateSecretOutput, error) {
	return &secretsmanager.UpdateSecretOutput{}, c.updateErr
}

func (c *secretsClientStub) GetSecretValue(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	if c.getErr != nil {
		return nil, c.getErr
	}
	return c.getOut, nil
}

func (c *secretsClientStub) DeleteSecret(_ context.Context, _ *secretsmanager.DeleteSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error) {
	return &secretsmanager.DeleteSecretOutput{}, c.deleteErr
}

func (c *secretsClientStub) ListSecrets(_ context.Context, _ *secretsmanager.ListSecretsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretsOutput, error) {
	return &secretsmanager.ListSecretsOutput{}, nil
}

func TestAWSSecretsManager_StorePrivateKey_InvalidKeyFormat(t *testing.T) {
	t.Parallel()

	sm := newTestAWSSecretsManager(newFakeSecretsClient())
	require.ErrorIs(t, sm.StorePrivateKey(context.Background(), "actor-1", "not-a-pem"), ErrInvalidPrivateKeyFormat)
}

func TestAWSSecretsManager_StorePrivateKey_CreateSecretFailure(t *testing.T) {
	t.Parallel()

	client := &secretsClientStub{createErr: errors.New("create failed")}
	sm := newTestAWSSecretsManager(client)

	privateKeyPEM := generateTestPrivateKeyPEM(t)
	err := sm.StorePrivateKey(context.Background(), "actor-1", privateKeyPEM)
	require.ErrorIs(t, err, ErrSecretCreation)
}

func TestAWSSecretsManager_RetrievePrivateKey_SecretStringNil_AndRetryExhaustion(t *testing.T) {
	t.Parallel()

	// SecretString nil branch.
	clientNil := &secretsClientStub{getOut: &secretsmanager.GetSecretValueOutput{SecretString: nil}}
	smNil := newTestAWSSecretsManager(clientNil)
	_, err := smNil.RetrievePrivateKey(context.Background(), "actor-1")
	require.ErrorIs(t, err, ErrSecretValueNil)

	// Retry exhaustion branch.
	clientRetry := &secretsClientStub{getErr: errors.New("temporary")}
	smRetry := newTestAWSSecretsManager(clientRetry)
	_, err = smRetry.RetrievePrivateKey(context.Background(), "actor-2")
	require.ErrorIs(t, err, ErrSecretRetrievalRetries)
}

func TestAWSSecretsManager_RetrievePrivateKey_InvalidKeyInSecretValue(t *testing.T) {
	t.Parallel()

	secretJSON, err := json.Marshal(SecretValue{
		PrivateKeyPEM: "not-a-pem",
		CreatedAt:     time.Now(),
		KeyType:       "RSA",
		Version:       "1.0",
	})
	require.NoError(t, err)

	client := &secretsClientStub{getOut: &secretsmanager.GetSecretValueOutput{SecretString: aws.String(string(secretJSON))}}
	sm := newTestAWSSecretsManager(client)

	_, err = sm.RetrievePrivateKey(context.Background(), "actor-1")
	require.ErrorIs(t, err, ErrRetrievedPrivateKeyInvalid)
}

func TestAWSSecretsManager_DeletePrivateKey_NonNotFoundError(t *testing.T) {
	t.Parallel()

	client := &secretsClientStub{deleteErr: apiError{code: "AccessDeniedException"}}
	sm := newTestAWSSecretsManager(client)
	sm.logger = zap.NewNop()

	err := sm.DeletePrivateKey(context.Background(), "actor-1")
	require.ErrorIs(t, err, ErrSecretDeletion)
}
