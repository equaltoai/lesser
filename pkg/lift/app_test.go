package lift

import (
	"os"
	"testing"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

func TestDefaultConfig(t *testing.T) {
	// Test without DEBUG env var
	_ = os.Unsetenv("DEBUG")
	config := DefaultConfig()

	assert.False(t, config.Debug)
	assert.Equal(t, 30*time.Second, config.Timeout)
	assert.True(t, config.EnableCORS)
	assert.True(t, config.EnableMetrics)
	assert.True(t, config.EnableCostTracking)
	assert.False(t, config.AuthRequired)
	assert.False(t, config.TenantRequired)

	// Test with DEBUG env var
	_ = os.Setenv("DEBUG", "true")
	defer func() { _ = os.Unsetenv("DEBUG") }()

	config = DefaultConfig()
	assert.True(t, config.Debug)
}

func TestNewAppBuilder(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultConfig()

	builder := NewAppBuilder(config, logger)

	assert.NotNil(t, builder)
	assert.Equal(t, config, builder.config)
	assert.NotNil(t, builder.app)
	assert.Equal(t, logger, builder.logger)
}

func TestAppBuilderWithStandardMiddleware(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultConfig()

	builder := NewAppBuilder(config, logger)
	app := builder.WithStandardMiddleware().Build()

	assert.NotNil(t, app)

	// Test that the app can handle a basic request
	_ = app.GET("/test", func(ctx *lift.Context) error {
		return ctx.JSON(map[string]string{"status": "ok"})
	})

	// This would normally be handled by Lambda, but we can test the basic structure
	// The actual request handling would require more complex setup
}

func TestLoggingMiddleware(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultConfig()

	builder := NewAppBuilder(config, logger)
	middleware := builder.createLoggingMiddleware()

	assert.NotNil(t, middleware)

	// Test that middleware can be created without panicking
	handler := middleware(lift.HandlerFunc(func(ctx *lift.Context) error {
		return ctx.JSON(map[string]string{"test": "response"})
	}))

	assert.NotNil(t, handler)
}

func TestCORSMiddleware(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultConfig()

	builder := NewAppBuilder(config, logger)
	middleware := builder.createCORSMiddleware()

	assert.NotNil(t, middleware)

	// Test that middleware can be created without panicking
	handler := middleware(lift.HandlerFunc(func(ctx *lift.Context) error {
		return ctx.JSON(map[string]string{"test": "response"})
	}))

	assert.NotNil(t, handler)
}

func TestCostTrackingMiddleware(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultConfig()

	builder := NewAppBuilder(config, logger)
	middleware := builder.createCostTrackingMiddleware()

	assert.NotNil(t, middleware)

	// Test that middleware can be created without panicking
	handler := middleware(lift.HandlerFunc(func(ctx *lift.Context) error {
		return ctx.JSON(map[string]string{"test": "response"})
	}))

	assert.NotNil(t, handler)
}

func TestNewHTTPApp(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultConfig()

	app := NewHTTPApp(config, logger)

	assert.NotNil(t, app)
}

func TestNewSQSApp(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultConfig()
	config.EnableCORS = true // Should be disabled by NewSQSApp

	app := NewSQSApp(config, logger)

	assert.NotNil(t, app)
	// Note: We can't directly test that CORS is disabled since the config
	// is modified in the function, but the app should be created successfully
}

func TestNewDynamoDBStreamApp(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultConfig()
	config.EnableCORS = true   // Should be disabled
	config.AuthRequired = true // Should be disabled

	app := NewDynamoDBStreamApp(config, logger)

	assert.NotNil(t, app)
	// Note: Similar to SQS test, we can't directly verify the config changes
	// but the app should be created successfully
}

func TestAppBuilderBuild(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultConfig()

	builder := NewAppBuilder(config, logger)
	app1 := builder.Build()
	app2 := builder.Build()

	// Should return the same app instance
	assert.Equal(t, app1, app2)
}

func TestAppConfigVariations(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tests := []struct {
		name   string
		config AppConfig
	}{
		{
			name: "minimal config",
			config: AppConfig{
				Debug:              false,
				Timeout:            0, // No timeout
				EnableCORS:         false,
				EnableMetrics:      false,
				EnableCostTracking: false,
			},
		},
		{
			name: "debug enabled",
			config: AppConfig{
				Debug:              true,
				Timeout:            10 * time.Second,
				EnableCORS:         true,
				EnableMetrics:      true,
				EnableCostTracking: true,
			},
		},
		{
			name: "auth required",
			config: AppConfig{
				Debug:              false,
				Timeout:            30 * time.Second,
				EnableCORS:         true,
				EnableMetrics:      true,
				EnableCostTracking: true,
				AuthRequired:       true,
				TenantRequired:     true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewAppBuilder(tt.config, logger)
			app := builder.WithStandardMiddleware().Build()

			assert.NotNil(t, app)
		})
	}
}

// Integration test that verifies the middleware stack works together
func TestMiddlewareIntegration(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultConfig()

	app := NewHTTPApp(config, logger)

	// Add a test route
	_ = app.GET("/health", func(ctx *lift.Context) error {
		return ctx.JSON(map[string]string{"status": "healthy"})
	})

	// The app should be properly configured with all middleware
	assert.NotNil(t, app)

	// Note: Full integration testing would require setting up Lambda context
	// and API Gateway events, which is beyond the scope of unit tests
}
