package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSetupTestEnvironment(t *testing.T) {
	// Clear environment variables before test
	_ = os.Unsetenv("JWT_SECRET")
	_ = os.Unsetenv("DOMAIN")
	_ = os.Unsetenv("INSTANCE_NAME")
	_ = os.Unsetenv("AWS_REGION")
	_ = os.Unsetenv("ENVIRONMENT")
	_ = os.Unsetenv("STAGE")
	_ = os.Unsetenv("S3_BUCKET_NAME")

	// Setup test environment
	SetupTestEnvironment(t)

	// Check that environment variables are set
	assert.Equal(t, "test_jwt_secret_for_testing", os.Getenv("JWT_SECRET"))
	assert.Equal(t, "localhost", os.Getenv("DOMAIN"))
	assert.Equal(t, "Lesser Test", os.Getenv("INSTANCE_NAME"))
	assert.Equal(t, "us-east-1", os.Getenv("AWS_REGION"))
	assert.Equal(t, "test", os.Getenv("ENVIRONMENT"))
	assert.Equal(t, "test", os.Getenv("STAGE"))
	assert.Equal(t, "lesser-test-media", os.Getenv("S3_BUCKET_NAME"))
}

func TestConfig(t *testing.T) {
	// Setup test environment
	SetupTestEnvironment(t)

	// Get config
	cfg := Get()

	// Check config values
	assert.Equal(t, "localhost", cfg.Domain)
	assert.Equal(t, "Lesser Test", cfg.InstanceName)
	assert.Equal(t, "us-east-1", cfg.Region)
	assert.Equal(t, "lesser-test", cfg.DynamoTableName)
	assert.Equal(t, "lesser-test-media", cfg.S3BucketName)
	assert.Equal(t, "test_jwt_secret_for_testing", cfg.JWTSecret)
}

func TestConfig_InstanceAPIKey_LoadsValueAndARN(t *testing.T) {
	SetupTestEnvironment(t)
	t.Setenv("INSTANCE_API_KEY", "  instance-key  ")
	t.Setenv("INSTANCE_API_KEY_ARN", "  arn:aws:secretsmanager:us-east-1:123456789012:secret:instance-api-key  ")

	ResetForTests()
	cfg := Get()

	assert.Equal(t, "instance-key", cfg.InstanceAPIKey)
	assert.Equal(t, "arn:aws:secretsmanager:us-east-1:123456789012:secret:instance-api-key", cfg.InstanceAPIKeyARN)
}

func TestConfig_InstanceAPIKey_ArnCapturedWhenValueMissing(t *testing.T) {
	SetupTestEnvironment(t)
	t.Setenv("INSTANCE_API_KEY", "")
	t.Setenv("INSTANCE_API_KEY_ARN", "  arn:aws:secretsmanager:us-east-1:123456789012:secret:instance-api-key  ")

	ResetForTests()
	cfg := Get()

	assert.Equal(t, "", cfg.InstanceAPIKey)
	assert.Equal(t, "arn:aws:secretsmanager:us-east-1:123456789012:secret:instance-api-key", cfg.InstanceAPIKeyARN)
}

func TestConfig_OAuthClientSecretRotationGracePeriod_DefaultsTo24Hours(t *testing.T) {
	SetupTestEnvironment(t)
	t.Setenv("OAUTH_CLIENT_SECRET_ROTATION_GRACE_PERIOD", "")

	ResetForTests()
	cfg := Get()

	assert.Equal(t, 24*time.Hour, cfg.OAuthClientSecretRotationGracePeriod)
}

func TestConfig_GraphQLMaxComplexity_DefaultAndOverride(t *testing.T) {
	SetupTestEnvironment(t)
	t.Setenv("GRAPHQL_MAX_COMPLEXITY", "")

	ResetForTests()
	cfg := Get()
	assert.Equal(t, DefaultGraphQLMaxComplexity, cfg.GraphQLMaxComplexity)

	t.Setenv("GRAPHQL_MAX_COMPLEXITY", "2500")
	ResetForTests()
	cfg = Get()
	assert.Equal(t, 2500, cfg.GraphQLMaxComplexity)
}
