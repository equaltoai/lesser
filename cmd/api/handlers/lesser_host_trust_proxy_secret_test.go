package handlers

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/stretchr/testify/require"
)

type stubTrustSecretsManagerClient struct {
	out       *secretsmanager.GetSecretValueOutput
	err       error
	callCount int
}

func (s *stubTrustSecretsManagerClient) GetSecretValue(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	s.callCount++
	if s.err != nil {
		return nil, s.err
	}
	return s.out, nil
}

func resetTrustSecretCache() {
	trustSecretCacheMu.Lock()
	trustSecretCache = map[string]cachedTrustSecret{}
	trustSecretCacheMu.Unlock()
}

func TestResolveTrustSecretValue_MissingSecretIDReturnsError(t *testing.T) {
	resetTrustSecretCache()
	_, err := resolveTrustSecretValue(context.Background(), "  ")
	require.Error(t, err)
}

func TestResolveTrustSecretValue_TrimsCachesAndParsesPlainSecret(t *testing.T) {
	resetTrustSecretCache()

	origLoad := loadAWSConfigForTrustSecrets
	origNewClient := newSecretsManagerClientForTrustSecret
	t.Cleanup(func() {
		loadAWSConfigForTrustSecrets = origLoad
		newSecretsManagerClientForTrustSecret = origNewClient
		resetTrustSecretCache()
	})

	loadCalls := 0
	loadAWSConfigForTrustSecrets = func(ctx context.Context, optFns ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
		loadCalls++
		return aws.Config{}, nil
	}

	secret := "  super-secret  "
	client := &stubTrustSecretsManagerClient{
		out: &secretsmanager.GetSecretValueOutput{
			SecretString: &secret,
		},
	}
	newSecretsManagerClientForTrustSecret = func(aws.Config) trustSecretsManagerClient {
		return client
	}

	got, err := resolveTrustSecretValue(context.Background(), " arn:aws:secretsmanager:us-east-1:123:secret:abc ")
	require.NoError(t, err)
	require.Equal(t, "super-secret", got)
	require.Equal(t, 1, loadCalls)
	require.Equal(t, 1, client.callCount)

	got2, err := resolveTrustSecretValue(context.Background(), "arn:aws:secretsmanager:us-east-1:123:secret:abc")
	require.NoError(t, err)
	require.Equal(t, "super-secret", got2)
	require.Equal(t, 1, loadCalls)
	require.Equal(t, 1, client.callCount)
}

func TestResolveTrustSecretValue_ExtractsSecretFieldFromJSONPayload(t *testing.T) {
	resetTrustSecretCache()

	origLoad := loadAWSConfigForTrustSecrets
	origNewClient := newSecretsManagerClientForTrustSecret
	t.Cleanup(func() {
		loadAWSConfigForTrustSecrets = origLoad
		newSecretsManagerClientForTrustSecret = origNewClient
		resetTrustSecretCache()
	})

	loadAWSConfigForTrustSecrets = func(ctx context.Context, optFns ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
		return aws.Config{}, nil
	}

	secret := `{"secret":"  xyz  "}`
	client := &stubTrustSecretsManagerClient{
		out: &secretsmanager.GetSecretValueOutput{
			SecretString: &secret,
		},
	}
	newSecretsManagerClientForTrustSecret = func(aws.Config) trustSecretsManagerClient {
		return client
	}

	got, err := resolveTrustSecretValue(context.Background(), "secret-id")
	require.NoError(t, err)
	require.Equal(t, "xyz", got)
}

func TestResolveTrustSecretValue_PropagatesErrorsAndRejectsEmptyValues(t *testing.T) {
	t.Run("load_aws_config_error", func(t *testing.T) {
		resetTrustSecretCache()

		origLoad := loadAWSConfigForTrustSecrets
		origNewClient := newSecretsManagerClientForTrustSecret
		t.Cleanup(func() {
			loadAWSConfigForTrustSecrets = origLoad
			newSecretsManagerClientForTrustSecret = origNewClient
			resetTrustSecretCache()
		})

		loadAWSConfigForTrustSecrets = func(ctx context.Context, optFns ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
			return aws.Config{}, errors.New("boom")
		}
		newSecretsManagerClientForTrustSecret = func(aws.Config) trustSecretsManagerClient {
			return &stubTrustSecretsManagerClient{}
		}

		_, err := resolveTrustSecretValue(context.Background(), "secret-id")
		require.Error(t, err)
	})

	t.Run("missing_secret_string", func(t *testing.T) {
		resetTrustSecretCache()

		origLoad := loadAWSConfigForTrustSecrets
		origNewClient := newSecretsManagerClientForTrustSecret
		t.Cleanup(func() {
			loadAWSConfigForTrustSecrets = origLoad
			newSecretsManagerClientForTrustSecret = origNewClient
			resetTrustSecretCache()
		})

		loadAWSConfigForTrustSecrets = func(ctx context.Context, optFns ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
			return aws.Config{}, nil
		}
		newSecretsManagerClientForTrustSecret = func(aws.Config) trustSecretsManagerClient {
			return &stubTrustSecretsManagerClient{out: &secretsmanager.GetSecretValueOutput{SecretString: nil}}
		}

		_, err := resolveTrustSecretValue(context.Background(), "secret-id")
		require.Error(t, err)
	})

	t.Run("empty_secret_value", func(t *testing.T) {
		resetTrustSecretCache()

		origLoad := loadAWSConfigForTrustSecrets
		origNewClient := newSecretsManagerClientForTrustSecret
		t.Cleanup(func() {
			loadAWSConfigForTrustSecrets = origLoad
			newSecretsManagerClientForTrustSecret = origNewClient
			resetTrustSecretCache()
		})

		loadAWSConfigForTrustSecrets = func(ctx context.Context, optFns ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
			return aws.Config{}, nil
		}

		empty := "   "
		newSecretsManagerClientForTrustSecret = func(aws.Config) trustSecretsManagerClient {
			return &stubTrustSecretsManagerClient{out: &secretsmanager.GetSecretValueOutput{SecretString: &empty}}
		}

		_, err := resolveTrustSecretValue(context.Background(), "secret-id")
		require.Error(t, err)
	})
}
