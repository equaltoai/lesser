package lift_test

import (
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/pay-theory/lift/pkg/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJWTAuthentication(t *testing.T) {
	// #nosec G101 -- Test secret for unit testing only
	secret := "test-secret"

	// Create app with JWT auth (canonical middleware)
	app := lift.New()
	app.Use(middleware.JWTAuth(middleware.JWTConfig{
		Secret:      secret,
		Algorithm:   "HS256",
		TokenLookup: "header:Authorization",
		SkipPaths:   []string{"/public"},
	}))

	// Public endpoint
	if err := app.GET("/public", func(ctx *lift.Context) error {
		return ctx.OK(map[string]bool{"public": true})
	}); err != nil {
		t.Fatalf("Failed to register public route: %v", err)
	}

	// Protected endpoint
	if err := app.GET("/protected", func(ctx *lift.Context) error {
		return ctx.OK(map[string]any{
			"user_id":       ctx.UserID(),
			"tenant_id":     ctx.TenantID(),
			"claims":        ctx.Claims(),
			"authenticated": ctx.IsAuthenticated(),
		})
	}); err != nil {
		t.Fatalf("Failed to register protected route: %v", err)
	}

	t.Run("Public endpoint accessible without token", func(t *testing.T) {
		ctx := createTestContext("GET", "/public", nil)

		err := app.HandleTestRequest(ctx)
		require.NoError(t, err)
		assert.Equal(t, 200, ctx.Response.StatusCode)
	})

	t.Run("Protected endpoint requires token", func(t *testing.T) {
		ctx := createTestContext("GET", "/protected", nil)

		err := app.HandleTestRequest(ctx)
		require.NoError(t, err)
		assert.Equal(t, 401, ctx.Response.StatusCode)
	})

	t.Run("Valid token allows access", func(t *testing.T) {
		// Create valid token
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"user_id":   "test-user",
			"tenant_id": "test-tenant",
			"exp":       time.Now().Add(time.Hour).Unix(),
		})
		tokenString, err := token.SignedString([]byte(secret))
		require.NoError(t, err)

		ctx := createTestContext("GET", "/protected", nil)
		ctx.Request.Headers["Authorization"] = "Bearer " + tokenString

		err = app.HandleTestRequest(ctx)
		require.NoError(t, err)
		assert.Equal(t, 200, ctx.Response.StatusCode)

		// Verify context was populated
		assert.Equal(t, "test-user", ctx.UserID())
		assert.Equal(t, "test-tenant", ctx.TenantID())
		assert.True(t, ctx.IsAuthenticated())
		assert.NotNil(t, ctx.Claims())
	})

	t.Run("Expired token is rejected", func(t *testing.T) {
		// Create expired token
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"user_id":   "test-user",
			"tenant_id": "test-tenant",
			"exp":       time.Now().Add(-time.Hour).Unix(), // Expired
		})
		tokenString, err := token.SignedString([]byte(secret))
		require.NoError(t, err)

		ctx := createTestContext("GET", "/protected", nil)
		ctx.Request.Headers["Authorization"] = "Bearer " + tokenString

		err = app.HandleTestRequest(ctx)
		require.NoError(t, err)
		assert.Equal(t, 401, ctx.Response.StatusCode)
	})

	t.Run("Invalid signature is rejected", func(t *testing.T) {
		// Create token with wrong secret
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"user_id":   "test-user",
			"tenant_id": "test-tenant",
			"exp":       time.Now().Add(time.Hour).Unix(),
		})
		// #nosec G101 -- Test secret for unit testing only
		tokenString, err := token.SignedString([]byte("wrong-secret"))
		require.NoError(t, err)

		ctx := createTestContext("GET", "/protected", nil)
		ctx.Request.Headers["Authorization"] = "Bearer " + tokenString

		err = app.HandleTestRequest(ctx)
		require.NoError(t, err)
		assert.Equal(t, 401, ctx.Response.StatusCode)
	})
}

func TestJWTContextMethods(t *testing.T) {
	t.Run("SetClaims populates context correctly", func(t *testing.T) {
		ctx := createTestContext("GET", "/test", nil)

		claims := jwt.MapClaims{
			"user_id":   "user-123",
			"tenant_id": "tenant-456",
			"sub":       "subject-789",
			"custom":    "value",
		}

		ctx.SetClaims(claims)

		// Verify claims are stored (convert to map[string]any for comparison)
		expectedClaims := map[string]any(claims)
		assert.Equal(t, expectedClaims, ctx.Claims())

		// Verify user and tenant IDs are extracted
		assert.Equal(t, "user-123", ctx.UserID())
		assert.Equal(t, "tenant-456", ctx.TenantID())

		// Verify IsAuthenticated
		assert.True(t, ctx.IsAuthenticated())

		// Verify GetClaim
		assert.Equal(t, "value", ctx.GetClaim("custom"))
		assert.Nil(t, ctx.GetClaim("nonexistent"))
	})

	t.Run("SetClaims uses sub claim for user_id if user_id missing", func(t *testing.T) {
		ctx := createTestContext("GET", "/test", nil)

		claims := jwt.MapClaims{
			"sub":       "subject-789",
			"tenant_id": "tenant-456",
		}

		ctx.SetClaims(claims)

		// Should use sub as user_id
		assert.Equal(t, "subject-789", ctx.UserID())
		assert.Equal(t, "tenant-456", ctx.TenantID())
	})

	t.Run("Empty context returns defaults", func(t *testing.T) {
		ctx := createTestContext("GET", "/test", nil)

		assert.Nil(t, ctx.Claims())
		assert.Equal(t, "", ctx.UserID())
		assert.Equal(t, "", ctx.TenantID())
		assert.False(t, ctx.IsAuthenticated())
		assert.Nil(t, ctx.GetClaim("anything"))
	})
}

// Helper function to create test context
func createTestContext(method, path string, body []byte) *lift.Context {
	adapterReq := &adapters.Request{
		Method:      method,
		Path:        path,
		Headers:     make(map[string]string),
		QueryParams: make(map[string]string),
		PathParams:  make(map[string]string),
		Body:        body,
	}
	req := lift.NewRequest(adapterReq)
	return lift.NewContext(context.Background(), req)
}
