package config

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type stubTableExistsChecker struct {
	exists bool
	err    error
}

func (s stubTableExistsChecker) TableExists(_ string) (bool, error) {
	return s.exists, s.err
}

type stubS3Client struct {
	err error
}

func (s stubS3Client) HeadBucket(_ context.Context, _ *s3.HeadBucketInput, _ ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
	return &s3.HeadBucketOutput{}, s.err
}

type stubSecretsManagerClient struct {
	err error
}

func (s stubSecretsManagerClient) DescribeSecret(_ context.Context, _ *secretsmanager.DescribeSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.DescribeSecretOutput, error) {
	return &secretsmanager.DescribeSecretOutput{}, s.err
}

func TestNewProductionConfigValidator_LoadDefaultConfigFailure(t *testing.T) {
	orig := loadDefaultAWSConfig
	t.Cleanup(func() { loadDefaultAWSConfig = orig })
	loadDefaultAWSConfig = func(_ context.Context, _ ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
		return aws.Config{}, errors.New("load failed")
	}

	validator, err := NewProductionConfigValidator(zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, validator)
	assert.Empty(t, validator.awsConfig.Region)
}

func TestQuickValidateProductionConfig(t *testing.T) {
	t.Run("missing vars returns error", func(t *testing.T) {
		t.Setenv("DOMAIN_NAME", "")
		t.Setenv("AWS_REGION", "")
		t.Setenv("DYNAMODB_TABLE", "")
		t.Setenv("DYNAMO_TABLE_NAME", "")
		t.Setenv("ENVIRONMENT", "")
		t.Setenv("STAGE", "")
		t.Setenv("PRIVATE_KEY_SECRET", "")
		t.Setenv("JWT_SECRET", "")
		t.Setenv("JWT_SECRET_ARN", "")

		err := QuickValidateProductionConfig()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing required environment variables")
	})

	t.Run("configured vars passes", func(t *testing.T) {
		t.Setenv("DOMAIN_NAME", "example.com")
		t.Setenv("AWS_REGION", "us-east-1")
		t.Setenv("DYNAMODB_TABLE", "")
		t.Setenv("DYNAMO_TABLE_NAME", "")
		t.Setenv("ENVIRONMENT", "main")
		t.Setenv("STAGE", "")
		t.Setenv("PRIVATE_KEY_SECRET", "secret-name")
		t.Setenv("JWT_SECRET", "abcdefghijklmnopqrstuvwxyz0123456789abcdef")
		t.Setenv("JWT_SECRET_ARN", "")

		require.NoError(t, QuickValidateProductionConfig())
	})
}

func TestProductionConfigValidator_ValidateProductionConfig_CriticalErrors(t *testing.T) {
	v := &ProductionConfigValidator{
		logger:  zap.NewNop(),
		timeout: 1,
	}

	t.Setenv("DOMAIN_NAME", "")
	t.Setenv("AWS_REGION", "")
	t.Setenv("DYNAMODB_TABLE", "")
	t.Setenv("PRIVATE_KEY_SECRET", "")
	t.Setenv("JWT_SECRET", "")
	t.Setenv("JWT_SECRET_ARN", "")

	result, err := v.ValidateProductionConfig(context.Background())
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.False(t, result.Valid)
	assert.Greater(t, result.Summary.CriticalErrors, 0)
	assert.NotEmpty(t, result.Errors)
}

func TestProductionConfigValidator_ValidateProductionConfig_NonCriticalErrorsStillValid(t *testing.T) {
	v := &ProductionConfigValidator{
		logger:  zap.NewNop(),
		timeout: 1,
	}

	t.Setenv("DOMAIN_NAME", "http://example.com")
	t.Setenv("AWS_REGION", "not-a-region")
	t.Setenv("DYNAMODB_TABLE", "lesser-main")
	t.Setenv("PRIVATE_KEY_SECRET", "secret-name")
	t.Setenv("JWT_SECRET", "abcdefghijklmnopqrstuvwxyz0123456789abcdef")
	t.Setenv("JWT_SECRET_ARN", "")
	t.Setenv("PORT", "0")

	result, err := v.ValidateProductionConfig(context.Background())
	require.NoError(t, err)
	require.NotNil(t, result)

	// By design, only critical errors make the config invalid.
	assert.True(t, result.Valid)
	assert.Equal(t, 0, result.Summary.CriticalErrors)
	assert.NotEmpty(t, result.Errors)

	assert.True(t, result.Security.HTTPSEnforcement.Configured)
	assert.False(t, result.Security.HTTPSEnforcement.Valid)
}

func TestProductionConfigValidator_ValidateProductionConfig_AWSResourcesValidation_UsesFactories(t *testing.T) {
	origTableExists := newTableExistsChecker
	origS3 := newS3Client
	origSecrets := newSecretsManagerClient
	t.Cleanup(func() {
		newTableExistsChecker = origTableExists
		newS3Client = origS3
		newSecretsManagerClient = origSecrets
	})

	newTableExistsChecker = func(_ aws.Config) (tableExistsChecker, error) {
		return stubTableExistsChecker{exists: true}, nil
	}
	newS3Client = func(_ aws.Config) s3Client { return stubS3Client{} }
	newSecretsManagerClient = func(_ aws.Config) secretsManagerClient { return stubSecretsManagerClient{} }

	v := &ProductionConfigValidator{
		logger:    zap.NewNop(),
		awsConfig: aws.Config{Region: "us-east-1"},
		timeout:   1,
	}

	t.Setenv("DOMAIN_NAME", "example.com")
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("DYNAMODB_TABLE", "lesser-main")
	t.Setenv("PRIVATE_KEY_SECRET", "secret-name")
	t.Setenv("JWT_SECRET", "abcdefghijklmnopqrstuvwxyz0123456789abcdef")
	t.Setenv("JWT_SECRET_ARN", "")
	t.Setenv("S3_BUCKET_NAME", "bucket-name")

	result, err := v.ValidateProductionConfig(context.Background())
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.True(t, result.Resources.DynamoDB.Available)
	assert.True(t, result.Resources.S3.Available)
	assert.True(t, result.Resources.SecretsManager.Available)
}

func TestProductionConfigValidator_SecurityAndFormatHelpers(t *testing.T) {
	v := &ProductionConfigValidator{logger: zap.NewNop()}

	t.Run("log level warning", func(t *testing.T) {
		result := &ValidationResult{}
		v.validateEnvironmentVariableFormat("LOG_LEVEL", "weird", result)
		require.NotEmpty(t, result.Warnings)
	})

	t.Run("jwt configured via arn", func(t *testing.T) {
		t.Setenv("JWT_SECRET", "")
		t.Setenv("JWT_SECRET_ARN", "arn:aws:secretsmanager:...")
		status := v.validateJWTConfiguration()
		assert.True(t, status.Configured)
		assert.True(t, status.Valid)
	})

	t.Run("oauth not configured is ok", func(t *testing.T) {
		t.Setenv("OAUTH_CLIENT_ID", "")
		t.Setenv("OAUTH_CLIENT_SECRET", "")
		status := v.validateOAuthSecrets()
		assert.False(t, status.Configured)
		assert.True(t, status.Valid)
	})

	t.Run("encryption key too short invalid", func(t *testing.T) {
		t.Setenv("ENCRYPTION_KEY", "short")
		status := v.validateEncryptionKeys()
		assert.True(t, status.Configured)
		assert.False(t, status.Valid)
	})

	t.Run("domain accessibility warning", func(t *testing.T) {
		result := &ValidationResult{Warnings: make([]ValidationWarning, 0)}
		t.Setenv("DOMAIN_NAME", "bad domain")
		v.validateNetworkConfiguration(result)
		require.NotEmpty(t, result.Warnings)
	})
}
