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

func TestProductionConfigValidator_AWSResourceValidation_ErrorBranches(t *testing.T) {
	origTableExists := newTableExistsChecker
	origS3 := newS3Client
	origSecrets := newSecretsManagerClient
	t.Cleanup(func() {
		newTableExistsChecker = origTableExists
		newS3Client = origS3
		newSecretsManagerClient = origSecrets
	})

	v := &ProductionConfigValidator{
		logger:    zap.NewNop(),
		awsConfig: aws.Config{Region: "us-east-1"},
		timeout:   1,
	}

	t.Run("dynamodb missing table info returns error", func(t *testing.T) {
		t.Setenv("DYNAMODB_TABLE", "")
		t.Setenv("DYNAMO_TABLE_NAME", "")
		t.Setenv("ENVIRONMENT", "")
		t.Setenv("STAGE", "")

		status := v.validateDynamoDB(context.Background())
		assert.False(t, status.Available)
		assert.Contains(t, status.Error, "must be set")
	})

	t.Run("dynamodb checker init failure returns error", func(t *testing.T) {
		t.Setenv("DYNAMODB_TABLE", "lesser-main")
		t.Setenv("DYNAMO_TABLE_NAME", "")
		t.Setenv("ENVIRONMENT", "")
		t.Setenv("STAGE", "")

		newTableExistsChecker = func(_ aws.Config) (tableExistsChecker, error) {
			return nil, errors.New("init boom")
		}
		t.Cleanup(func() { newTableExistsChecker = origTableExists })

		status := v.validateDynamoDB(context.Background())
		assert.False(t, status.Available)
		assert.Contains(t, status.Error, "failed to initialize TableTheory DynamoDB client")
		assert.Contains(t, status.Error, "init boom")
	})

	t.Run("dynamodb table exists checker error returns error", func(t *testing.T) {
		t.Setenv("DYNAMODB_TABLE", "lesser-main")
		t.Setenv("DYNAMO_TABLE_NAME", "")
		t.Setenv("ENVIRONMENT", "")
		t.Setenv("STAGE", "")

		newTableExistsChecker = func(_ aws.Config) (tableExistsChecker, error) {
			return stubTableExistsChecker{exists: false, err: errors.New("access boom")}, nil
		}
		t.Cleanup(func() { newTableExistsChecker = origTableExists })

		status := v.validateDynamoDB(context.Background())
		assert.False(t, status.Available)
		assert.Contains(t, status.Error, "not accessible")
		assert.Contains(t, status.Error, "access boom")
	})

	t.Run("dynamodb table missing returns error", func(t *testing.T) {
		t.Setenv("DYNAMODB_TABLE", "lesser-main")
		t.Setenv("DYNAMO_TABLE_NAME", "")
		t.Setenv("ENVIRONMENT", "")
		t.Setenv("STAGE", "")

		newTableExistsChecker = func(_ aws.Config) (tableExistsChecker, error) {
			return stubTableExistsChecker{exists: false, err: nil}, nil
		}
		t.Cleanup(func() { newTableExistsChecker = origTableExists })

		status := v.validateDynamoDB(context.Background())
		assert.False(t, status.Available)
		assert.Contains(t, status.Error, "not found")
	})

	t.Run("s3 missing bucket is treated as optional", func(t *testing.T) {
		t.Setenv("S3_BUCKET_NAME", "")
		t.Setenv("S3_BUCKET", "")

		status := v.validateS3(context.Background())
		assert.False(t, status.Available)
		assert.Contains(t, status.Message, "not configured")
	})

	t.Run("s3 head bucket failure returns error", func(t *testing.T) {
		t.Setenv("S3_BUCKET_NAME", "bucket-name")
		t.Setenv("S3_BUCKET", "")

		newS3Client = func(_ aws.Config) s3Client { return stubS3Client{err: errors.New("head boom")} }
		t.Cleanup(func() { newS3Client = origS3 })

		status := v.validateS3(context.Background())
		assert.False(t, status.Available)
		assert.Contains(t, status.Error, "not accessible")
		assert.Contains(t, status.Error, "head boom")
	})

	t.Run("secrets manager missing secret name returns error", func(t *testing.T) {
		t.Setenv("PRIVATE_KEY_SECRET", "")

		status := v.validateSecretsManager(context.Background())
		assert.False(t, status.Available)
		assert.Equal(t, "Private key secret name not configured", status.Error)
	})

	t.Run("secrets manager describe failure returns error", func(t *testing.T) {
		t.Setenv("PRIVATE_KEY_SECRET", "secret-name")

		newSecretsManagerClient = func(_ aws.Config) secretsManagerClient {
			return stubSecretsManagerClient{err: errors.New("describe boom")}
		}
		t.Cleanup(func() { newSecretsManagerClient = origSecrets })

		status := v.validateSecretsManager(context.Background())
		assert.False(t, status.Available)
		assert.Contains(t, status.Error, "not accessible")
		assert.Contains(t, status.Error, "describe boom")
	})
}

func TestProductionConfigValidator_SecurityAndFormatHelpers(t *testing.T) {
	v := &ProductionConfigValidator{logger: zap.NewNop()}

	t.Run("log level warning", func(t *testing.T) {
		result := &ValidationResult{}
		v.validateEnvironmentVariableFormat("LOG_LEVEL", "weird", result)
		require.NotEmpty(t, result.Warnings)
	})

	t.Run("domain name format error", func(t *testing.T) {
		result := &ValidationResult{}
		v.validateEnvironmentVariableFormat("DOMAIN_NAME", "bad domain", result)
		require.NotEmpty(t, result.Errors)
	})

	t.Run("aws region format error", func(t *testing.T) {
		result := &ValidationResult{}
		v.validateEnvironmentVariableFormat("AWS_REGION", "not-a-region", result)
		require.NotEmpty(t, result.Errors)
	})

	t.Run("environment name warning", func(t *testing.T) {
		result := &ValidationResult{}
		v.validateEnvironmentVariableFormat("ENVIRONMENT", "weird", result)
		require.NotEmpty(t, result.Warnings)
	})

	t.Run("stage name warning", func(t *testing.T) {
		result := &ValidationResult{}
		v.validateEnvironmentVariableFormat("STAGE", "weird", result)
		require.NotEmpty(t, result.Warnings)
	})

	t.Run("jwt configured via arn", func(t *testing.T) {
		t.Setenv("JWT_SECRET", "")
		t.Setenv("JWT_SECRET_ARN", "arn:aws:secretsmanager:...")
		status := v.validateJWTConfiguration()
		assert.True(t, status.Configured)
		assert.True(t, status.Valid)
	})

	t.Run("jwt missing is invalid", func(t *testing.T) {
		t.Setenv("JWT_SECRET", "")
		t.Setenv("JWT_SECRET_ARN", "")
		status := v.validateJWTConfiguration()
		assert.False(t, status.Configured)
		assert.False(t, status.Valid)
	})

	t.Run("jwt too short is invalid", func(t *testing.T) {
		t.Setenv("JWT_SECRET", "short")
		t.Setenv("JWT_SECRET_ARN", "")
		status := v.validateJWTConfiguration()
		assert.True(t, status.Configured)
		assert.False(t, status.Valid)
	})

	t.Run("jwt long secret is valid", func(t *testing.T) {
		t.Setenv("JWT_SECRET", "abcdefghijklmnopqrstuvwxyz0123456789abcdef")
		t.Setenv("JWT_SECRET_ARN", "")
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

	t.Run("oauth partial configuration is invalid", func(t *testing.T) {
		t.Setenv("OAUTH_CLIENT_ID", "client")
		t.Setenv("OAUTH_CLIENT_SECRET", "")
		status := v.validateOAuthSecrets()
		assert.True(t, status.Configured)
		assert.False(t, status.Valid)
	})

	t.Run("oauth complete configuration is valid", func(t *testing.T) {
		t.Setenv("OAUTH_CLIENT_ID", "client")
		t.Setenv("OAUTH_CLIENT_SECRET", "secret")
		status := v.validateOAuthSecrets()
		assert.True(t, status.Configured)
		assert.True(t, status.Valid)
	})

	t.Run("encryption key too short invalid", func(t *testing.T) {
		t.Setenv("ENCRYPTION_KEY", "short")
		status := v.validateEncryptionKeys()
		assert.True(t, status.Configured)
		assert.False(t, status.Valid)
	})

	t.Run("encryption key missing invalid", func(t *testing.T) {
		t.Setenv("ENCRYPTION_KEY", "")
		status := v.validateEncryptionKeys()
		assert.False(t, status.Configured)
		assert.False(t, status.Valid)
	})

	t.Run("encryption key long valid", func(t *testing.T) {
		t.Setenv("ENCRYPTION_KEY", "abcdefghijklmnopqrstuvwxyz0123456789abcdef")
		status := v.validateEncryptionKeys()
		assert.True(t, status.Configured)
		assert.True(t, status.Valid)
	})

	t.Run("isValidDomain handles schemes", func(t *testing.T) {
		assert.True(t, v.isValidDomain("https://example.com"))
		assert.False(t, v.isValidDomain("http://%"))
		assert.False(t, v.isValidDomain("localhost"))
	})

	t.Run("domain accessibility warning", func(t *testing.T) {
		result := &ValidationResult{Warnings: make([]ValidationWarning, 0)}
		t.Setenv("DOMAIN_NAME", "bad domain")
		v.validateNetworkConfiguration(result)
		require.NotEmpty(t, result.Warnings)
	})
}

func TestSafeEnvValue_RedactsKnownSecretVars(t *testing.T) {
	// Set a real-looking secret value
	t.Setenv("JWT_SECRET", "my-jwt-token-abc123")
	t.Setenv("OAUTH_CLIENT_SECRET", "oauth-secret-xyz")
	t.Setenv("ENCRYPTION_KEY", "my-encryption-key")
	t.Setenv("INSTANCE_API_KEY", "instance-key-secret")
	t.Setenv("PRIVACY_MASTER_KEY", "privacy-master-secret")

	assert.Equal(t, RedactedSecretSentinel, safeEnvValue("JWT_SECRET"))
	assert.Equal(t, RedactedSecretSentinel, safeEnvValue("OAUTH_CLIENT_SECRET"))
	assert.Equal(t, RedactedSecretSentinel, safeEnvValue("ENCRYPTION_KEY"))
	assert.Equal(t, RedactedSecretSentinel, safeEnvValue("INSTANCE_API_KEY"))
	assert.Equal(t, RedactedSecretSentinel, safeEnvValue("PRIVACY_MASTER_KEY"))

	// Case-insensitive
	assert.Equal(t, RedactedSecretSentinel, safeEnvValue("jwt_secret"))
}

func TestSafeEnvValue_PreservesNonSecretVars(t *testing.T) {
	t.Setenv("DOMAIN_NAME", "example.com")
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("VAPID_PUBLIC_KEY", "BPubKeyExampleAbc123==")

	assert.Equal(t, "example.com", safeEnvValue("DOMAIN_NAME"))
	assert.Equal(t, "us-east-1", safeEnvValue("AWS_REGION"))
	assert.Equal(t, "BPubKeyExampleAbc123==", safeEnvValue("VAPID_PUBLIC_KEY"))
}

func TestSafeEnvValue_UnknownVarReturnsValue(t *testing.T) {
	t.Setenv("UNKNOWN_VAR", "some-value")
	assert.Equal(t, "some-value", safeEnvValue("UNKNOWN_VAR"))
}

func TestIsSecretEnvVar_Coverage(t *testing.T) {
	assert.True(t, isSecretEnvVar("JWT_SECRET"))
	assert.True(t, isSecretEnvVar("OAUTH_CLIENT_SECRET"))
	assert.True(t, isSecretEnvVar("INSTANCE_API_KEY"))
	assert.True(t, isSecretEnvVar("LESSER_HOST_INSTANCE_KEY"))
	assert.True(t, isSecretEnvVar("DYNAMODB_ENCRYPTION_KEY"))
	assert.False(t, isSecretEnvVar("DOMAIN_NAME"))
	assert.False(t, isSecretEnvVar("AWS_REGION"))
	assert.False(t, isSecretEnvVar("LOG_LEVEL"))
	assert.False(t, isSecretEnvVar("SYSTEM_ACTOR_PUBLIC_KEY"))
	assert.False(t, isSecretEnvVar("VAPID_PUBLIC_KEY"))
}

// TestValidateEnvironmentVariables_NeverLeaksSecretValues ensures that
// ValidationError.Value never contains raw secret material even when a
// secret env var value happens to match a format check that would
// normally include the value.
func TestValidateEnvironmentVariables_NeverLeaksSecretValues(t *testing.T) {
	v := &ProductionConfigValidator{
		logger:  zap.NewNop(),
		timeout: 1,
	}

	// Secret values that happen to look like bad formats
	t.Setenv("PRIVATE_KEY_SECRET", "bad domain")
	t.Setenv("DOMAIN_NAME", "bad domain")
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("DYNAMODB_TABLE", "lesser-main")
	t.Setenv("JWT_SECRET", "abcdefghijklmnopqrstuvwxyz0123456789abcdef")
	t.Setenv("JWT_SECRET_ARN", "")

	result, err := v.ValidateProductionConfig(context.Background())
	require.NoError(t, err)
	require.NotNil(t, result)

	// Check every error's Value field for raw secret material
	for _, e := range result.Errors {
		// The DOMAIN_NAME error is fine to show the actual (non-secret) value
		// But PRIVATE_KEY_SECRET or JWT_SECRET errors must redact
		if isSecretEnvVar(e.Field) {
			assert.Equal(t, RedactedSecretSentinel, e.Value,
				"secret env var %s must have redacted Value, got %q", e.Field, e.Value)
		}
	}

	// The DOMAIN_NAME error should still show the actual value (non-secret)
	foundDomainError := false
	for _, e := range result.Errors {
		if e.Field == "DOMAIN_NAME" {
			foundDomainError = true
			assert.Equal(t, "bad domain", e.Value, "non-secret field DOMAIN_NAME should preserve value")
		}
	}
	assert.True(t, foundDomainError, "expected a DOMAIN_NAME validation error")
}

func TestValidationResult_JSONSerialization_NoSecretsInValues(t *testing.T) {
	v := &ProductionConfigValidator{
		logger:  zap.NewNop(),
		timeout: 1,
	}

	t.Setenv("DOMAIN_NAME", "example.com")
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("DYNAMODB_TABLE", "lesser-main")
	t.Setenv("PRIVATE_KEY_SECRET", "secret-name")
	t.Setenv("JWT_SECRET", "abcdefghijklmnopqrstuvwxyz0123456789abcdef")
	t.Setenv("JWT_SECRET_ARN", "")

	result, err := v.ValidateProductionConfig(context.Background())
	require.NoError(t, err)
	require.NotNil(t, result)

	// A successful run produces no errors that contain secret values.
	for _, e := range result.Errors {
		assert.NotContains(t, e.Value, "abcdefghijklmnopqrstuvwxyz",
			"error Value must not contain JWT secret material")
	}
}
