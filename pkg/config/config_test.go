package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetupTestEnvironment(t *testing.T) {
	// Clear environment variables before test
	_ = os.Unsetenv("JWT_SECRET")
	_ = os.Unsetenv("DOMAIN")
	_ = os.Unsetenv("INSTANCE_NAME")
	_ = os.Unsetenv("AWS_REGION")
	_ = os.Unsetenv("DYNAMO_TABLE_NAME")
	_ = os.Unsetenv("S3_BUCKET_NAME")

	// Setup test environment
	SetupTestEnvironment(t)

	// Check that environment variables are set
	assert.Equal(t, "test_jwt_secret_for_testing", os.Getenv("JWT_SECRET"))
	assert.Equal(t, "localhost", os.Getenv("DOMAIN"))
	assert.Equal(t, "Lesser Test", os.Getenv("INSTANCE_NAME"))
	assert.Equal(t, "us-east-1", os.Getenv("AWS_REGION"))
	assert.Equal(t, "lesser-test", os.Getenv("DYNAMO_TABLE_NAME"))
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
