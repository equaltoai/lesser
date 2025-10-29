package config

import (
	"os"
	"testing"
)

// SetupTestEnvironment sets up environment variables for testing
func SetupTestEnvironment(t *testing.T) {
	// Save original environment variables to restore later
	originalEnv := make(map[string]string)
	for _, key := range []string{
		"JWT_SECRET",
		"DOMAIN",
		"INSTANCE_NAME",
		"AWS_REGION",
		"ENVIRONMENT",
		"STAGE",
		"S3_BUCKET_NAME",
	} {
		originalEnv[key] = os.Getenv(key)
	}

	// Set test environment variables
	_ = os.Setenv("JWT_SECRET", "test_jwt_secret_for_testing")
	_ = os.Setenv("DOMAIN", "localhost")
	_ = os.Setenv("INSTANCE_NAME", "Lesser Test")
	_ = os.Setenv("AWS_REGION", "us-east-1")
	_ = os.Setenv("ENVIRONMENT", "test")
	_ = os.Setenv("STAGE", "test")
	_ = os.Setenv("S3_BUCKET_NAME", "lesser-test-media")

	// Cleanup function to restore original environment
	t.Cleanup(func() {
		for key, value := range originalEnv {
			if value == "" {
				_ = os.Unsetenv(key)
			} else {
				_ = os.Setenv(key, value)
			}
		}
	})
}
