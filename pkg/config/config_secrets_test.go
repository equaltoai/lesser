package config

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetOptionalSecretFromEnvOrARN_ValueEnvTakesPrecedence(t *testing.T) {
	t.Setenv("UNIT_TEST_SECRET_VALUE", "  value-from-env  ")
	t.Setenv("UNIT_TEST_SECRET_ARN", "arn:aws:secretsmanager:us-east-1:123456789012:secret:ignored")

	assert.Equal(t, "value-from-env", getOptionalSecretFromEnvOrARN("UNIT_TEST_SECRET_VALUE", "UNIT_TEST_SECRET_ARN"))
}

func TestGetOptionalSecretFromEnvOrARN_EmptyWhenUnset(t *testing.T) {
	t.Setenv("UNIT_TEST_SECRET_VALUE", "")
	t.Setenv("UNIT_TEST_SECRET_ARN", "")

	assert.Equal(t, "", getOptionalSecretFromEnvOrARN("UNIT_TEST_SECRET_VALUE", "UNIT_TEST_SECRET_ARN"))
}

func TestGetOptionalSecretFromEnvOrARN_ArnSkipsAwsInTests(t *testing.T) {
	originalArgs := append([]string(nil), os.Args...)
	t.Cleanup(func() { os.Args = originalArgs })

	// Ensure we do not attempt to call AWS in unit tests, even if an ARN is present.
	os.Args[0] = "config.test"

	t.Setenv("UNIT_TEST_SECRET_VALUE", "")
	t.Setenv("UNIT_TEST_SECRET_ARN", "arn:aws:secretsmanager:us-east-1:123456789012:secret:unit-test")

	assert.Equal(t, "", getOptionalSecretFromEnvOrARN("UNIT_TEST_SECRET_VALUE", "UNIT_TEST_SECRET_ARN"))
}

func TestGetOptionalSecretFromEnvOrARN_ArnFetchesValueOutsideTests(t *testing.T) {
	ResetForTests()

	origLoad := loadDefaultAWSConfig
	origNew := newSecretsManagerValueGetter
	origArgs := os.Args
	t.Cleanup(func() {
		loadDefaultAWSConfig = origLoad
		newSecretsManagerValueGetter = origNew
		os.Args = origArgs
	})

	os.Args = []string{"app"}
	t.Setenv("UNIT_TEST_SECRET_VALUE", "")
	t.Setenv("UNIT_TEST_SECRET_ARN", "arn:aws:secretsmanager:us-east-1:123456789012:secret:unit-test")

	loadDefaultAWSConfig = func(_ context.Context, _ ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
		return aws.Config{Region: "us-east-1"}, nil
	}
	secretString := `{"secret":"from-secrets-manager"}`
	newSecretsManagerValueGetter = func(_ aws.Config) secretsManagerValueGetter {
		return stubSecretsManagerGetter{
			output: &secretsmanager.GetSecretValueOutput{SecretString: &secretString},
		}
	}

	assert.Equal(t, "from-secrets-manager", getOptionalSecretFromEnvOrARN("UNIT_TEST_SECRET_VALUE", "UNIT_TEST_SECRET_ARN"))
}

func TestGetOptionalSecretFromEnvOrARN_ReturnsEmptyWhenFetchFailsOutsideTests(t *testing.T) {
	ResetForTests()

	origLoad := loadDefaultAWSConfig
	origNew := newSecretsManagerValueGetter
	origArgs := os.Args
	t.Cleanup(func() {
		loadDefaultAWSConfig = origLoad
		newSecretsManagerValueGetter = origNew
		os.Args = origArgs
		ResetForTests()
	})

	os.Args = []string{"app"}
	t.Setenv("UNIT_TEST_SECRET_VALUE", "")
	t.Setenv("UNIT_TEST_SECRET_ARN", "arn:aws:secretsmanager:us-east-1:123456789012:secret:unit-test-failure")

	loadDefaultAWSConfig = func(_ context.Context, _ ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
		return aws.Config{Region: "us-east-1"}, nil
	}
	newSecretsManagerValueGetter = func(_ aws.Config) secretsManagerValueGetter {
		return stubSecretsManagerGetter{err: errors.New("boom")}
	}

	require.Equal(t, "", getOptionalSecretFromEnvOrARN("UNIT_TEST_SECRET_VALUE", "UNIT_TEST_SECRET_ARN"))
}

func TestConfigResolveInstanceAPIKey_ResolvesSecretARNOutsideTests(t *testing.T) {
	ResetForTests()

	origLoad := loadDefaultAWSConfig
	origNew := newSecretsManagerValueGetter
	origArgs := os.Args
	t.Cleanup(func() {
		loadDefaultAWSConfig = origLoad
		newSecretsManagerValueGetter = origNew
		os.Args = origArgs
		ResetForTests()
	})

	os.Args = []string{"app"}

	loadDefaultAWSConfig = func(_ context.Context, _ ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
		return aws.Config{Region: "us-east-1"}, nil
	}

	callCount := 0
	secretString := `{"secret":"instance-api-key"}`
	newSecretsManagerValueGetter = func(_ aws.Config) secretsManagerValueGetter {
		callCount++
		return stubSecretsManagerGetter{
			output: &secretsmanager.GetSecretValueOutput{SecretString: &secretString},
		}
	}

	cfg := &Config{
		InstanceAPIKeyARN: "arn:aws:secretsmanager:us-east-1:123456789012:secret:instance-api-key",
	}

	value, err := cfg.ResolveInstanceAPIKey()
	require.NoError(t, err)
	require.Equal(t, "instance-api-key", value)

	value, err = cfg.ResolveInstanceAPIKey()
	require.NoError(t, err)
	require.Equal(t, "instance-api-key", value)
	require.Equal(t, 1, callCount)
}

func TestConfigResolveLesserHostInstanceKey_ReturnsFetchErrorsOutsideTests(t *testing.T) {
	ResetForTests()

	origLoad := loadDefaultAWSConfig
	origNew := newSecretsManagerValueGetter
	origArgs := os.Args
	t.Cleanup(func() {
		loadDefaultAWSConfig = origLoad
		newSecretsManagerValueGetter = origNew
		os.Args = origArgs
		ResetForTests()
	})

	os.Args = []string{"app"}

	loadDefaultAWSConfig = func(_ context.Context, _ ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
		return aws.Config{Region: "us-east-1"}, nil
	}
	newSecretsManagerValueGetter = func(_ aws.Config) secretsManagerValueGetter {
		return stubSecretsManagerGetter{err: errors.New("boom")}
	}

	cfg := &Config{
		LesserHostInstanceKeyARN: "arn:aws:secretsmanager:us-east-1:123456789012:secret:lesser-host-key",
	}

	value, err := cfg.ResolveLesserHostInstanceKey()
	require.Error(t, err)
	require.Equal(t, "", value)
}
