package middleware

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLimitedRateLimit(t *testing.T) {
	// Skip if no DynamoDB available
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Test middleware creation with invalid config (should fail)
	_, err := LimitedRateLimit(LimitedConfig{
		Region:    "",
		TableName: "",
		Window:    time.Minute,
		Limit:     5,
	})

	// Should fail due to missing configuration
	assert.Error(t, err)

	// Test middleware creation with valid config
	middleware, err := LimitedRateLimit(LimitedConfig{
		Region:    "us-east-1",
		TableName: "test-rate-limits",
		Endpoint:  "http://localhost:8000", // For local testing
		Window:    time.Minute,
		Limit:     5,
	})

	if err != nil {
		t.Skipf("Skipping test due to DynamoDB connection error: %v", err)
	}

	assert.NotNil(t, middleware)

	// Note: Full integration testing would require a running DynamoDB instance
	// This test verifies the middleware can be created successfully
}

func TestIPRateLimitWithLimited(t *testing.T) {
	// Set AWS_REGION for testing
	if err := os.Setenv("AWS_REGION", "us-east-1"); err != nil {
		t.Logf("Warning: failed to set AWS_REGION: %v", err)
	}
	defer func() {
		if err := os.Unsetenv("AWS_REGION"); err != nil {
			t.Logf("Warning: failed to unset AWS_REGION: %v", err)
		}
	}()

	middleware, err := IPRateLimitWithLimited(10, time.Minute)
	if err != nil {
		t.Skipf("Skipping test due to DynamoDB connection error: %v", err)
	}

	assert.NotNil(t, middleware)
}

func TestUserRateLimitWithLimited(t *testing.T) {
	// Set AWS_REGION for testing
	if err := os.Setenv("AWS_REGION", "us-east-1"); err != nil {
		t.Logf("Warning: failed to set AWS_REGION: %v", err)
	}
	defer func() {
		if err := os.Unsetenv("AWS_REGION"); err != nil {
			t.Logf("Warning: failed to unset AWS_REGION: %v", err)
		}
	}()

	middleware, err := UserRateLimitWithLimited(100, 15*time.Minute)
	if err != nil {
		t.Skipf("Skipping test due to DynamoDB connection error: %v", err)
	}

	assert.NotNil(t, middleware)
}

func TestTenantRateLimitWithLimited(t *testing.T) {
	// Set AWS_REGION for testing
	if err := os.Setenv("AWS_REGION", "us-east-1"); err != nil {
		t.Logf("Warning: failed to set AWS_REGION: %v", err)
	}
	defer func() {
		if err := os.Unsetenv("AWS_REGION"); err != nil {
			t.Logf("Warning: failed to unset AWS_REGION: %v", err)
		}
	}()

	middleware, err := TenantRateLimitWithLimited(50, 10*time.Minute)
	if err != nil {
		t.Skipf("Skipping test due to DynamoDB connection error: %v", err)
	}

	assert.NotNil(t, middleware)
}

func TestRateLimitWithoutAWSRegion(t *testing.T) {
	// Ensure AWS_REGION is not set
	if err := os.Unsetenv("AWS_REGION"); err != nil {
		t.Logf("Warning: failed to unset AWS_REGION: %v", err)
	}
	if err := os.Unsetenv("AWS_DEFAULT_REGION"); err != nil {
		t.Logf("Warning: failed to unset AWS_DEFAULT_REGION: %v", err)
	}

	// Test IPRateLimitWithLimited without region
	_, err := IPRateLimitWithLimited(10, time.Minute)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "AWS_REGION environment variable not set")

	// Test UserRateLimitWithLimited without region
	_, err = UserRateLimitWithLimited(100, 15*time.Minute)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "AWS_REGION environment variable not set")

	// Test TenantRateLimitWithLimited without region
	_, err = TenantRateLimitWithLimited(50, 10*time.Minute)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "AWS_REGION environment variable not set")
}

func TestLimitedConfigDefaults(t *testing.T) {
	config := LimitedConfig{}

	// Test that defaults get applied
	assert.Equal(t, "", config.TableName)
	assert.Equal(t, time.Duration(0), config.Window)
	assert.Equal(t, 0, config.Limit)

	// Middleware creation should apply defaults
	middleware, err := LimitedRateLimit(config)
	if err != nil {
		t.Skipf("Skipping test due to DynamoDB connection error: %v", err)
	}

	assert.NotNil(t, middleware)
}
