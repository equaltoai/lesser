package lift

import (
	"context"
	"os"
	"testing"
)

func TestNew(t *testing.T) {
	app := New()
	if app == nil {
		t.Fatal("New() returned nil")
	}

	if app.router == nil {
		t.Error("App router is nil")
	}

	if app.config == nil {
		t.Error("App config is nil")
	}

	if app.started {
		t.Error("App should not be started initially")
	}
}

func TestAppRoutes(t *testing.T) {
	app := New()

	// Test route registration
	err := app.GET("/test", func(ctx *Context) error {
		return ctx.JSON(map[string]string{"message": "test"})
	})
	if err != nil {
		t.Fatalf("GET route registration failed: %v", err)
	}

	err = app.POST("/users", func(ctx *Context) error {
		return ctx.JSON(map[string]string{"created": "user"})
	})
	if err != nil {
		t.Fatalf("POST route registration failed: %v", err)
	}

	// Test individual route registrations (method chaining not supported with error returns)
	err = app.PUT("/update", func(_ *Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("PUT route registration failed: %v", err)
	}

	err = app.DELETE("/delete", func(_ *Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("DELETE route registration failed: %v", err)
	}
}

func TestAppStart(t *testing.T) {
	app := New()

	// Start should succeed
	err := app.Start()
	if err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	if !app.started {
		t.Error("App should be marked as started")
	}

	// Starting again should be safe
	err = app.Start()
	if err != nil {
		t.Fatalf("Second Start() failed: %v", err)
	}
}

func TestAppWithConfig(t *testing.T) {
	config := &Config{
		LogLevel: "DEBUG",
		Timeout:  60,
	}

	app := New().WithConfig(config)

	if app.config.LogLevel != "DEBUG" {
		t.Errorf("Expected LogLevel DEBUG, got %s", app.config.LogLevel)
	}

	if app.config.Timeout != 60 {
		t.Errorf("Expected Timeout 60, got %d", app.config.Timeout)
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.MaxRequestSize <= 0 {
		t.Error("MaxRequestSize should be positive")
	}

	if config.MaxResponseSize <= 0 {
		t.Error("MaxResponseSize should be positive")
	}

	if config.LogLevel == "" {
		t.Error("LogLevel should not be empty")
	}

	if !config.MetricsEnabled {
		t.Error("MetricsEnabled should be true by default")
	}
}

func TestAppHandleRequest(t *testing.T) {
	app := New()

	// Add a simple route
	if err := app.GET("/test", func(ctx *Context) error {
		return ctx.JSON(map[string]string{"status": "ok"})
	}); err != nil {
		t.Fatalf("Failed to register GET route: %v", err)
	}

	err := app.Start()
	if err != nil {
		t.Fatalf("Failed to start app: %v", err)
	}

	// Create a proper API Gateway V1 event
	event := map[string]any{
		"resource":   "/test",
		"httpMethod": "GET",
		"path":       "/test",
		"requestContext": map[string]any{
			"requestId": "test-request-id",
		},
		"headers": map[string]any{
			"Content-Type": "application/json",
		},
	}

	// Test HandleRequest
	resp, err := app.HandleRequest(context.Background(), event)
	if err != nil {
		t.Fatalf("HandleRequest failed: %v", err)
	}

	if resp == nil {
		t.Error("Response should not be nil")
	}
}

func TestIsLambda(t *testing.T) {
	app := New()

	testCases := []struct {
		envVars  map[string]string
		name     string
		expected bool
	}{
		{
			name:     "No Lambda environment variables",
			envVars:  map[string]string{},
			expected: false,
		},
		{
			name: "AWS_LAMBDA_FUNCTION_NAME set",
			envVars: map[string]string{
				"AWS_LAMBDA_FUNCTION_NAME": "my-function",
			},
			expected: true,
		},
		{
			name: "LAMBDA_TASK_ROOT set",
			envVars: map[string]string{
				"LAMBDA_TASK_ROOT": "/var/task",
			},
			expected: true,
		},
		{
			name: "AWS_EXECUTION_ENV set",
			envVars: map[string]string{
				"AWS_EXECUTION_ENV": "AWS_Lambda_go1.x",
			},
			expected: true,
		},
		{
			name: "All Lambda environment variables set",
			envVars: map[string]string{
				"AWS_LAMBDA_FUNCTION_NAME": "my-function",
				"LAMBDA_TASK_ROOT":         "/var/task",
				"AWS_EXECUTION_ENV":        "AWS_Lambda_go1.x",
			},
			expected: true,
		},
		{
			name: "Non-Lambda environment variables",
			envVars: map[string]string{
				"HOME": "/home/user",
				"PATH": "/usr/bin:/bin",
				"USER": "testuser",
			},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Save current environment
			savedEnv := make(map[string]string)
			for key := range tc.envVars {
				savedEnv[key] = os.Getenv(key)
				if err := os.Unsetenv(key); err != nil {
					t.Logf("Warning: failed to unset env var %s: %v", key, err)
				}
			}

			// Also ensure Lambda env vars are unset when testing non-Lambda scenarios
			lambdaVars := []string{"AWS_LAMBDA_FUNCTION_NAME", "LAMBDA_TASK_ROOT", "AWS_EXECUTION_ENV"}
			for _, key := range lambdaVars {
				if _, exists := tc.envVars[key]; !exists {
					savedEnv[key] = os.Getenv(key)
					if err := os.Unsetenv(key); err != nil {
						t.Logf("Warning: failed to unset lambda env var %s: %v", key, err)
					}
				}
			}

			// Set test environment variables
			for key, value := range tc.envVars {
				if err := os.Setenv(key, value); err != nil {
					t.Fatalf("Failed to set test env var %s: %v", key, err)
				}
			}

			// Test IsLambda
			result := app.IsLambda()
			if result != tc.expected {
				t.Errorf("IsLambda() = %v, expected %v", result, tc.expected)
			}

			// Restore environment
			for key, value := range savedEnv {
				if value == "" {
					if err := os.Unsetenv(key); err != nil {
						t.Logf("Warning: failed to restore env var %s: %v", key, err)
					}
				} else {
					if err := os.Setenv(key, value); err != nil {
						t.Logf("Warning: failed to restore env var %s: %v", key, err)
					}
				}
			}
		})
	}
}
