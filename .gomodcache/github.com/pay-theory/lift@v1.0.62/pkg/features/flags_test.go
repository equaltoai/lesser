package features

import (
	"os"
	"testing"
)

func TestFeatureFlags(t *testing.T) {
	t.Run("Default flags in production", func(t *testing.T) {
		// Set production environment
		if err := os.Setenv("LIFT_ENV", "production"); err != nil {
			t.Logf("Warning: failed to set environment variable: %v", err)
		}
		defer func() {
			if err := os.Unsetenv("LIFT_ENV"); err != nil {
				t.Logf("Warning: failed to unset environment variable: %v", err)
			}
		}()

		ff, err := NewFeatureFlags(FeatureFlagConfig{
			LocalOnly: true,
		})
		if err != nil {
			t.Fatalf("Failed to create feature flags: %v", err)
		}

		// Core features should be enabled by default in production
		if !ff.IsEnabled(RateLimitingEnabled) {
			t.Error("Rate limiting should be enabled in production")
		}

		if !ff.IsEnabled(CircuitBreakerEnabled) {
			t.Error("Circuit breaker should be enabled in production")
		}

		// Dev features should be disabled in production
		if ff.IsEnabled(MockServicesEnabled) {
			t.Error("Mock services should be disabled in production")
		}

		if ff.IsEnabled(DebugLoggingEnabled) {
			t.Error("Debug logging should be disabled in production")
		}
	})

	t.Run("Default flags in development", func(t *testing.T) {
		// Set development environment
		if err := os.Setenv("LIFT_ENV", "development"); err != nil {
			t.Logf("Warning: failed to set environment variable: %v", err)
		}
		defer func() {
			if err := os.Unsetenv("LIFT_ENV"); err != nil {
				t.Logf("Warning: failed to unset environment variable: %v", err)
			}
		}()

		ff, err := NewFeatureFlags(FeatureFlagConfig{
			LocalOnly: true,
		})
		if err != nil {
			t.Fatalf("Failed to create feature flags: %v", err)
		}

		// Dev features should be enabled in development
		if !ff.IsEnabled(MockServicesEnabled) {
			t.Error("Mock services should be enabled in development")
		}

		if !ff.IsEnabled(DebugLoggingEnabled) {
			t.Error("Debug logging should be enabled in development")
		}
	})

	t.Run("Environment variable override", func(t *testing.T) {
		ff, err := NewFeatureFlags(FeatureFlagConfig{
			LocalOnly: true,
		})
		if err != nil {
			t.Fatalf("Failed to create feature flags: %v", err)
		}

		// Set feature to false
		ff.SetFlag(RateLimitingEnabled, false)

		// Verify it's false
		if ff.IsEnabled(RateLimitingEnabled) {
			t.Error("Rate limiting should be disabled")
		}

		// Override with environment variable
		if err := os.Setenv("LIFT_FEATURE_rate_limiting_enabled", "true"); err != nil {
			t.Logf("Warning: failed to set environment variable: %v", err)
		}
		defer func() {
			if err := os.Unsetenv("LIFT_FEATURE_rate_limiting_enabled"); err != nil {
				t.Logf("Warning: failed to unset environment variable: %v", err)
			}
		}()

		// Should now be true due to env override
		if !ff.IsEnabled(RateLimitingEnabled) {
			t.Error("Environment variable should override flag value")
		}
	})

	t.Run("Manual flag setting", func(t *testing.T) {
		ff, err := NewFeatureFlags(FeatureFlagConfig{
			LocalOnly: true,
		})
		if err != nil {
			t.Fatalf("Failed to create feature flags: %v", err)
		}

		// Set a custom flag
		ff.SetFlag("custom_feature", true)

		if !ff.IsEnabled("custom_feature") {
			t.Error("Custom feature should be enabled")
		}

		// Disable it
		ff.SetFlag("custom_feature", false)

		if ff.IsEnabled("custom_feature") {
			t.Error("Custom feature should be disabled")
		}
	})

	t.Run("Get all flags", func(t *testing.T) {
		ff, err := NewFeatureFlags(FeatureFlagConfig{
			LocalOnly: true,
		})
		if err != nil {
			t.Fatalf("Failed to create feature flags: %v", err)
		}

		allFlags := ff.GetAllFlags()

		// Should have all known flags
		if _, exists := allFlags[RateLimitingEnabled]; !exists {
			t.Error("Should have rate limiting flag")
		}

		if _, exists := allFlags[CircuitBreakerEnabled]; !exists {
			t.Error("Should have circuit breaker flag")
		}
	})

	t.Run("Unknown flag returns false", func(t *testing.T) {
		ff, err := NewFeatureFlags(FeatureFlagConfig{
			LocalOnly: true,
		})
		if err != nil {
			t.Fatalf("Failed to create feature flags: %v", err)
		}

		// Unknown flags should default to false
		if ff.IsEnabled("unknown_flag_xyz") {
			t.Error("Unknown flag should default to false")
		}
	})
}

func TestGlobalFeatureFlags(t *testing.T) {
	// Initialize global flags
	err := InitializeFeatureFlags(FeatureFlagConfig{
		LocalOnly: true,
	})
	if err != nil {
		t.Fatalf("Failed to initialize global flags: %v", err)
	}

	// Test global IsEnabled function
	if !IsEnabled(RateLimitingEnabled) {
		t.Error("Global IsEnabled should work for known flags")
	}
}

func BenchmarkFeatureFlagCheck(b *testing.B) {
	ff, err := NewFeatureFlags(FeatureFlagConfig{
		LocalOnly: true,
	})
	if err != nil {
		b.Fatalf("Failed to create feature flags: %v", err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ff.IsEnabled(RateLimitingEnabled)
	}
}
