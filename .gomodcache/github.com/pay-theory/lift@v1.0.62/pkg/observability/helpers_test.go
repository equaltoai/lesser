package observability

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestNewDefaultLoggerConfig(t *testing.T) {
	// Set up test environment variables
	if err := os.Setenv("AWS_LAMBDA_FUNCTION_NAME", "test-function"); err != nil {
		t.Logf("Warning: failed to set environment variable: %v", err)
	}
	if err := os.Setenv("PARTNER", "test-partner"); err != nil {
		t.Logf("Warning: failed to set environment variable: %v", err)
	}
	if err := os.Setenv("STAGE", "test-stage"); err != nil {
		t.Logf("Warning: failed to set environment variable: %v", err)
	}
	if err := os.Setenv("AWS_LAMBDA_FUNCTION_VERSION", "v1.0.0"); err != nil {
		t.Logf("Warning: failed to set environment variable: %v", err)
	}

	defer func() {
		// Clean up
		if err := os.Unsetenv("AWS_LAMBDA_FUNCTION_NAME"); err != nil {
			t.Logf("Warning: failed to unset environment variable: %v", err)
		}
		if err := os.Unsetenv("PARTNER"); err != nil {
			t.Logf("Warning: failed to unset environment variable: %v", err)
		}
		if err := os.Unsetenv("STAGE"); err != nil {
			t.Logf("Warning: failed to unset environment variable: %v", err)
		}
		if err := os.Unsetenv("AWS_LAMBDA_FUNCTION_VERSION"); err != nil {
			t.Logf("Warning: failed to unset environment variable: %v", err)
		}
	}()

	tests := []struct {
		name      string
		level     string
		wantLevel string
	}{
		{
			name:      "debug level",
			level:     "debug",
			wantLevel: "debug",
		},
		{
			name:      "info level",
			level:     "info",
			wantLevel: "info",
		},
		{
			name:      "warn level",
			level:     "warn",
			wantLevel: "warn",
		},
		{
			name:      "error level",
			level:     "error",
			wantLevel: "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := NewDefaultLoggerConfig(tt.level)

			// Check basic configuration
			if config.Level != tt.wantLevel {
				t.Errorf("Level = %v, want %v", config.Level, tt.wantLevel)
			}

			if config.Format != "json" {
				t.Errorf("Format = %v, want json", config.Format)
			}

			// Check CloudWatch configuration
			expectedLogGroup := "/aws/lambda/test-function-test-partner-test-stage"
			if config.LogGroup != expectedLogGroup {
				t.Errorf("LogGroup = %v, want %v", config.LogGroup, expectedLogGroup)
			}

			// Check log stream format
			if !strings.Contains(config.LogStream, time.Now().Format("2006/01/02")) {
				t.Errorf("LogStream should contain today's date")
			}

			if !strings.Contains(config.LogStream, "[v1.0.0]") {
				t.Errorf("LogStream should contain version")
			}

			// Check default values
			if config.BatchSize != 10 {
				t.Errorf("BatchSize = %v, want 10", config.BatchSize)
			}

			if config.FlushInterval != 2*time.Second {
				t.Errorf("FlushInterval = %v, want 2s", config.FlushInterval)
			}

			if config.BufferSize != 50 {
				t.Errorf("BufferSize = %v, want 50", config.BufferSize)
			}
		})
	}
}

func TestNewDefaultLoggerConfig_MissingEnvVars(t *testing.T) {
	// Clear all environment variables
	os.Unsetenv("AWS_LAMBDA_FUNCTION_NAME")
	os.Unsetenv("PARTNER")
	os.Unsetenv("STAGE")
	os.Unsetenv("AWS_LAMBDA_FUNCTION_VERSION")

	config := NewDefaultLoggerConfig("info")

	// Should still create a valid config with empty values
	expectedLogGroup := "/aws/lambda/--"
	if config.LogGroup != expectedLogGroup {
		t.Errorf("LogGroup = %v, want %v", config.LogGroup, expectedLogGroup)
	}

	// Should use $LATEST as default version
	if !strings.Contains(config.LogStream, "[$LATEST]") {
		t.Errorf("LogStream should contain default version $LATEST")
	}
}

func TestNewLoggerConfigWithOptions(t *testing.T) {
	// Set up test environment variables
	os.Setenv("AWS_LAMBDA_FUNCTION_NAME", "test-function")
	os.Setenv("PARTNER", "test-partner")
	os.Setenv("STAGE", "test-stage")

	defer func() {
		os.Unsetenv("AWS_LAMBDA_FUNCTION_NAME")
		os.Unsetenv("PARTNER")
		os.Unsetenv("STAGE")
	}()

	config := NewLoggerConfigWithOptions("debug",
		WithLogGroup("/custom/log/group"),
		WithLogStream("custom-stream"),
		WithBatchSize(25),
		WithFlushInterval(5*time.Second),
		WithBufferSize(100),
		WithFormat("console"),
		WithTenantContext("tenant-123", "user-456"),
		WithAsyncLogging(),
		WithRetryConfig(3, 1*time.Second),
		WithCallerInfo(),
		WithStackTrace(),
	)

	// Verify all options were applied
	if config.Level != "debug" {
		t.Errorf("Level = %v, want debug", config.Level)
	}

	if config.LogGroup != "/custom/log/group" {
		t.Errorf("LogGroup = %v, want /custom/log/group", config.LogGroup)
	}

	if config.LogStream != "custom-stream" {
		t.Errorf("LogStream = %v, want custom-stream", config.LogStream)
	}

	if config.BatchSize != 25 {
		t.Errorf("BatchSize = %v, want 25", config.BatchSize)
	}

	if config.FlushInterval != 5*time.Second {
		t.Errorf("FlushInterval = %v, want 5s", config.FlushInterval)
	}

	if config.BufferSize != 100 {
		t.Errorf("BufferSize = %v, want 100", config.BufferSize)
	}

	if config.Format != "console" {
		t.Errorf("Format = %v, want console", config.Format)
	}

	if config.DefaultTenantID != "tenant-123" {
		t.Errorf("DefaultTenantID = %v, want tenant-123", config.DefaultTenantID)
	}

	if config.DefaultUserID != "user-456" {
		t.Errorf("DefaultUserID = %v, want user-456", config.DefaultUserID)
	}

	if !config.AsyncLogging {
		t.Error("AsyncLogging should be true")
	}

	if config.MaxRetries != 3 {
		t.Errorf("MaxRetries = %v, want 3", config.MaxRetries)
	}

	if config.RetryDelay != 1*time.Second {
		t.Errorf("RetryDelay = %v, want 1s", config.RetryDelay)
	}

	if !config.EnableCaller {
		t.Error("EnableCaller should be true")
	}

	if !config.EnableStack {
		t.Error("EnableStack should be true")
	}
}

func TestLoggerConfigOptions_PartialOverride(t *testing.T) {
	// Test that only specified options override defaults
	config := NewLoggerConfigWithOptions("info",
		WithBatchSize(20),
		WithFormat("console"),
	)

	// These should be overridden
	if config.BatchSize != 20 {
		t.Errorf("BatchSize = %v, want 20", config.BatchSize)
	}

	if config.Format != "console" {
		t.Errorf("Format = %v, want console", config.Format)
	}

	// These should still have default values
	if config.FlushInterval != 2*time.Second {
		t.Errorf("FlushInterval = %v, want 2s (default)", config.FlushInterval)
	}

	if config.BufferSize != 50 {
		t.Errorf("BufferSize = %v, want 50 (default)", config.BufferSize)
	}

	if config.AsyncLogging {
		t.Error("AsyncLogging should be false by default")
	}
}
