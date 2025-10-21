package middleware

import (
	"context"
	"testing"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateLimit(t *testing.T) {
	tests := []struct {
		name          string
		config        RateLimitConfig
		requests      int
		expectedAllow int
		expectedDeny  int
		checkHeaders  bool
	}{
		{
			name: "default configuration allows requests within limit",
			config: RateLimitConfig{
				DefaultLimit: 5,
				Window:       time.Minute,
			},
			requests:      3,
			expectedAllow: 3,
			expectedDeny:  0,
			checkHeaders:  true,
		},
		{
			name: "exceeding limit denies requests",
			config: RateLimitConfig{
				DefaultLimit: 2,
				Window:       time.Minute,
			},
			requests:      5,
			expectedAllow: 2,
			expectedDeny:  3,
			checkHeaders:  true,
		},
		{
			name: "tenant-specific limits override default",
			config: RateLimitConfig{
				DefaultLimit: 2,
				Window:       time.Minute,
				TenantLimits: map[string]int{
					"premium": 10,
				},
			},
			requests:      5,
			expectedAllow: 5, // Premium tenant has higher limit
			expectedDeny:  0,
			checkHeaders:  true,
		},
		{
			name: "fixed window strategy",
			config: RateLimitConfig{
				Strategy:     "fixed_window",
				DefaultLimit: 3,
				Window:       time.Minute,
			},
			requests:      4,
			expectedAllow: 3,
			expectedDeny:  1,
		},
		{
			name: "sliding window strategy",
			config: RateLimitConfig{
				Strategy:     "sliding_window",
				DefaultLimit: 3,
				Window:       time.Minute,
				Granularity:  10 * time.Second,
			},
			requests:      4,
			expectedAllow: 3,
			expectedDeny:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: These tests would require a mock DynamORM instance
			// For now, we're testing the configuration and structure

			middleware := RateLimit(tt.config)
			assert.NotNil(t, middleware)

			// Just verify middleware was created successfully
			// The actual rate limiting would require DynamORM setup
		})
	}
}

func TestRateLimitKeyFunctions(t *testing.T) {
	// Create a test context with proper initialization
	adapterReq := &adapters.Request{
		Path:   "/api/users",
		Method: "GET",
		Headers: map[string]string{
			"X-Forwarded-For": "192.168.1.1",
		},
	}
	req := lift.NewRequest(adapterReq)
	ctx := lift.NewContext(context.TODO(), req)
	ctx.Set("tenant_id", "test-tenant")
	ctx.Set("user_id", "test-user")

	t.Run("default key function", func(t *testing.T) {
		key := defaultKeyFunc(ctx)
		assert.Equal(t, "test-tenant:test-user", key.Identifier)
		assert.Equal(t, "/api/users", key.Resource)
		assert.Equal(t, "GET", key.Operation)
		assert.Equal(t, "test-tenant", key.Metadata["tenant_id"])
		assert.Equal(t, "test-user", key.Metadata["user_id"])
	})

	t.Run("tenant rate limit key", func(t *testing.T) {
		middleware := TenantRateLimit(100, time.Minute)
		assert.NotNil(t, middleware)
	})

	t.Run("user rate limit key", func(t *testing.T) {
		middleware := UserRateLimit(50, time.Minute)
		assert.NotNil(t, middleware)
	})

	t.Run("IP rate limit key", func(t *testing.T) {
		middleware := IPRateLimit(1000, time.Hour)
		assert.NotNil(t, middleware)
	})

	t.Run("endpoint rate limit key", func(t *testing.T) {
		middleware := EndpointRateLimit(200, time.Minute)
		assert.NotNil(t, middleware)
	})
}

func TestDefaultErrorHandler(t *testing.T) {
	ctx := &lift.Context{
		Response: lift.NewResponse(),
	}

	result := &RateLimitResult{
		Allowed:    false,
		Limit:      100,
		Remaining:  0,
		ResetAt:    time.Now().Add(30 * time.Second),
		RetryAfter: 30 * time.Second,
	}

	err := defaultErrorHandler(ctx, result)
	require.NoError(t, err)
	assert.Equal(t, 429, ctx.Response.StatusCode)
}

func TestRateLimitHeaders(t *testing.T) {
	// This test would verify that rate limit headers are properly set
	// It requires a full integration test with DynamORM
	t.Skip("Integration test - requires DynamORM setup")
}

func TestMultiTenantRateLimit(t *testing.T) {
	// This test would verify multi-tenant rate limiting
	// It requires a full integration test with DynamORM
	t.Skip("Integration test - requires DynamORM setup")
}

func TestRateLimitStrategies(t *testing.T) {
	strategies := []string{"fixed_window", "sliding_window", "multi_window"}

	for _, strategy := range strategies {
		t.Run(strategy, func(t *testing.T) {
			config := RateLimitConfig{
				Strategy:     strategy,
				DefaultLimit: 10,
				Window:       time.Minute,
			}

			middleware := RateLimit(config)
			assert.NotNil(t, middleware)
		})
	}
}

func TestCompositeRateLimit(t *testing.T) {
	config := RateLimitConfig{
		Strategy:     "sliding_window",
		Window:       5 * time.Minute,
		DefaultLimit: 500,
		TenantLimits: map[string]int{
			"enterprise": 10000,
			"premium":    5000,
			"basic":      1000,
		},
		Granularity: 30 * time.Second,
	}

	middleware := CompositeRateLimit(config)
	assert.NotNil(t, middleware)
}
