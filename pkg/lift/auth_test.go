package lift

import (
	"context"
	"testing"

	"github.com/aron23/lesser/pkg/auth"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// AuthServiceInterface defines the interface we need for testing
type AuthServiceInterface interface {
	ValidateAccessToken(token string) (*auth.EnhancedClaims, error)
}

// MockAuthService is a mock that implements only the methods we need
type MockAuthService struct {
	mock.Mock
}

func (m *MockAuthService) ValidateAccessToken(token string) (*auth.EnhancedClaims, error) {
	args := m.Called(token)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*auth.EnhancedClaims), args.Error(1)
}

// testLiftAuthService is a modified version for testing
type testLiftAuthService struct {
	authService AuthServiceInterface
}

func newTestLiftAuthService(authService AuthServiceInterface) *testLiftAuthService {
	return &testLiftAuthService{
		authService: authService,
	}
}

// Copy the middleware methods from the original but use our interface
func (las *testLiftAuthService) RequireAuth() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			token := extractBearerToken(ctx)
			if token == "" {
				return ctx.Unauthorized("Authentication required", nil)
			}

			claims, err := las.authService.ValidateAccessToken(token)
			if err != nil {
				return ctx.Unauthorized("Invalid token", err)
			}

			ctx.Set("claims", claims)
			ctx.Set("username", claims.Username)
			ctx.Set("session_id", claims.SessionID)
			ctx.Set("device_id", claims.DeviceID)

			return next.Handle(ctx)
		})
	}
}

func (las *testLiftAuthService) OptionalAuth() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			token := extractBearerToken(ctx)
			if token == "" {
				return next.Handle(ctx)
			}

			claims, err := las.authService.ValidateAccessToken(token)
			if err != nil {
				return next.Handle(ctx)
			}

			ctx.Set("claims", claims)
			ctx.Set("username", claims.Username)
			ctx.Set("session_id", claims.SessionID)
			ctx.Set("device_id", claims.DeviceID)

			return next.Handle(ctx)
		})
	}
}

func (las *testLiftAuthService) RequireScope(scope string) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			claims, ok := ctx.Get("claims").(*auth.EnhancedClaims)
			if !ok {
				return ctx.Forbidden("Authentication required", nil)
			}

			if !claims.HasScope(scope) {
				return ctx.Forbidden("Insufficient permissions", nil)
			}

			return next.Handle(ctx)
		})
	}
}

// createTestContext creates a Lift context for testing following Lift patterns
func createTestContext() *lift.Context {
	ctx := &lift.Context{
		Context: context.Background(),
		Request: &lift.Request{
			Method:  "GET",
			Path:    "/test",
			Headers: make(map[string]string),
		},
		Response: &lift.Response{
			StatusCode: 200,
			Headers:    make(map[string]string),
		},
	}
	// Initialize internal maps that Lift uses
	ctx.Set("__test", "init") // This initializes the internal storage
	ctx.Get("__test")         // Clean up the test key
	
	return ctx
}

// createTestContextWithAuth creates a test context with authorization header
func createTestContextWithAuth(token string) *lift.Context {
	ctx := createTestContext()
	if token != "" {
		ctx.Request.Headers["Authorization"] = "Bearer " + token
	}
	return ctx
}

// createTestContextWithHost creates a test context with host header
func createTestContextWithHost(host string) *lift.Context {
	ctx := createTestContext()
	ctx.Request.Headers["Host"] = host
	return ctx
}

// createTestContextWithPath creates a test context with specific path
func createTestContextWithPath(path string) *lift.Context {
	ctx := createTestContext()
	ctx.Request.Path = path
	return ctx
}

// createValidClaims creates valid test claims
func createValidClaims() *auth.EnhancedClaims {
	return &auth.EnhancedClaims{
		Claims: auth.Claims{
			Username: "testuser",
			Scopes:   []string{"read", "write"},
			ClientID: "test-client",
		},
		SessionID: "test-session",
		DeviceID:  "test-device",
	}
}

// TestLiftAuthService_RequireAuth tests the RequireAuth middleware
func TestLiftAuthService_RequireAuth(t *testing.T) {
	t.Run("valid token", func(t *testing.T) {
		mockAuthService := new(MockAuthService)
		expectedClaims := createValidClaims()
		mockAuthService.On("ValidateAccessToken", "valid-token").Return(expectedClaims, nil)

		liftAuthService := newTestLiftAuthService(mockAuthService)
		middleware := liftAuthService.RequireAuth()

		handlerCalled := false
		testHandler := lift.HandlerFunc(func(ctx *lift.Context) error {
			handlerCalled = true
			return ctx.JSON(map[string]string{"success": "true"})
		})

		wrappedHandler := middleware(testHandler)
		ctx := createTestContextWithAuth("valid-token")

		err := wrappedHandler.Handle(ctx)

		// Should succeed with valid token
		assert.NoError(t, err)
		assert.True(t, handlerCalled, "Handler should be called when auth succeeds")

		// Verify claims were stored in context
		claims, ok := ctx.Get("claims").(*auth.EnhancedClaims)
		assert.True(t, ok, "Claims should be stored in context")
		assert.Equal(t, expectedClaims.Username, claims.Username)
		assert.Equal(t, expectedClaims.SessionID, claims.SessionID)
		assert.Equal(t, expectedClaims.DeviceID, claims.DeviceID)

		// Verify other context values
		username, ok := ctx.Get("username").(string)
		assert.True(t, ok)
		assert.Equal(t, expectedClaims.Username, username)

		mockAuthService.AssertExpectations(t)
	})

	t.Run("no token provided", func(t *testing.T) {
		liftAuthService := newTestLiftAuthService(nil) // No auth service needed
		middleware := liftAuthService.RequireAuth()

		handlerCalled := false
		testHandler := lift.HandlerFunc(func(ctx *lift.Context) error {
			handlerCalled = true
			return ctx.JSON(map[string]string{"success": "true"})
		})

		wrappedHandler := middleware(testHandler)
		ctx := createTestContextWithAuth("") // No token

		_ = wrappedHandler.Handle(ctx)

		// In Lift, ctx.Unauthorized() sets status and returns JSON marshalling error (usually nil)
		// The key is that the handler should NOT be called
		// and the response status should be set to 401
		assert.False(t, handlerCalled, "Handler should not be called when no token provided")
		assert.Equal(t, 401, ctx.Response.StatusCode, "Response should have 401 status")
	})

	t.Run("invalid token", func(t *testing.T) {
		mockAuthService := new(MockAuthService)
		mockAuthService.On("ValidateAccessToken", "invalid-token").Return(nil, auth.ErrInvalidToken)

		liftAuthService := newTestLiftAuthService(mockAuthService)
		middleware := liftAuthService.RequireAuth()

		handlerCalled := false
		testHandler := lift.HandlerFunc(func(ctx *lift.Context) error {
			handlerCalled = true
			return ctx.JSON(map[string]string{"success": "true"})
		})

		wrappedHandler := middleware(testHandler)
		ctx := createTestContextWithAuth("invalid-token")

		_ = wrappedHandler.Handle(ctx)

		// Handler should not be called and status should be 401
		assert.False(t, handlerCalled, "Handler should not be called when token is invalid")
		assert.Equal(t, 401, ctx.Response.StatusCode, "Response should have 401 status")
		mockAuthService.AssertExpectations(t)
	})
}

// TestLiftAuthService_OptionalAuth tests the OptionalAuth middleware
func TestLiftAuthService_OptionalAuth(t *testing.T) {
	t.Run("valid token", func(t *testing.T) {
		mockAuthService := new(MockAuthService)
		expectedClaims := createValidClaims()
		mockAuthService.On("ValidateAccessToken", "valid-token").Return(expectedClaims, nil)

		liftAuthService := newTestLiftAuthService(mockAuthService)
		middleware := liftAuthService.OptionalAuth()

		handlerCalled := false
		testHandler := lift.HandlerFunc(func(ctx *lift.Context) error {
			handlerCalled = true
			return ctx.JSON(map[string]string{"success": "true"})
		})

		wrappedHandler := middleware(testHandler)
		ctx := createTestContextWithAuth("valid-token")

		err := wrappedHandler.Handle(ctx)

		assert.NoError(t, err)
		assert.True(t, handlerCalled, "Handler should be called with valid token")

		// Verify claims were stored in context
		claims, ok := ctx.Get("claims").(*auth.EnhancedClaims)
		assert.True(t, ok, "Claims should be stored in context")
		assert.Equal(t, expectedClaims.Username, claims.Username)

		mockAuthService.AssertExpectations(t)
	})

	t.Run("no token", func(t *testing.T) {
		liftAuthService := newTestLiftAuthService(nil)
		middleware := liftAuthService.OptionalAuth()

		handlerCalled := false
		testHandler := lift.HandlerFunc(func(ctx *lift.Context) error {
			handlerCalled = true
			return ctx.JSON(map[string]string{"success": "true"})
		})

		wrappedHandler := middleware(testHandler)
		ctx := createTestContextWithAuth("")

		err := wrappedHandler.Handle(ctx)

		// OptionalAuth should always call the handler
		assert.NoError(t, err)
		assert.True(t, handlerCalled, "Handler should be called even without token")

		// No claims should be stored
		claims := ctx.Get("claims")
		assert.Nil(t, claims, "Claims should not be stored when no token provided")
	})

	t.Run("invalid token", func(t *testing.T) {
		mockAuthService := new(MockAuthService)
		mockAuthService.On("ValidateAccessToken", "invalid-token").Return(nil, auth.ErrInvalidToken)

		liftAuthService := newTestLiftAuthService(mockAuthService)
		middleware := liftAuthService.OptionalAuth()

		handlerCalled := false
		testHandler := lift.HandlerFunc(func(ctx *lift.Context) error {
			handlerCalled = true
			return ctx.JSON(map[string]string{"success": "true"})
		})

		wrappedHandler := middleware(testHandler)
		ctx := createTestContextWithAuth("invalid-token")

		err := wrappedHandler.Handle(ctx)

		// OptionalAuth should still call handler even with invalid token
		assert.NoError(t, err)
		assert.True(t, handlerCalled, "Handler should be called even with invalid token")

		// No claims should be stored
		claims := ctx.Get("claims")
		assert.Nil(t, claims, "Claims should not be stored when token is invalid")

		mockAuthService.AssertExpectations(t)
	})
}

// TestLiftAuthService_RequireScope tests the RequireScope middleware
func TestLiftAuthService_RequireScope(t *testing.T) {
	t.Run("user has required scope", func(t *testing.T) {
		liftAuthService := newTestLiftAuthService(nil)
		middleware := liftAuthService.RequireScope("read")

		handlerCalled := false
		testHandler := lift.HandlerFunc(func(ctx *lift.Context) error {
			handlerCalled = true
			return ctx.JSON(map[string]string{"success": "true"})
		})

		wrappedHandler := middleware(testHandler)
		ctx := createTestContext()

		// Set up claims with required scope
		claims := &auth.EnhancedClaims{
			Claims: auth.Claims{
				Username: "testuser",
				Scopes:   []string{"read", "write"},
			},
		}
		ctx.Set("claims", claims)

		err := wrappedHandler.Handle(ctx)

		assert.NoError(t, err)
		assert.True(t, handlerCalled, "Handler should be called when user has required scope")
	})

	t.Run("user missing required scope", func(t *testing.T) {
		liftAuthService := newTestLiftAuthService(nil)
		middleware := liftAuthService.RequireScope("admin")

		handlerCalled := false
		testHandler := lift.HandlerFunc(func(ctx *lift.Context) error {
			handlerCalled = true
			return ctx.JSON(map[string]string{"success": "true"})
		})

		wrappedHandler := middleware(testHandler)
		ctx := createTestContext()

		// Set up claims without required scope
		claims := &auth.EnhancedClaims{
			Claims: auth.Claims{
				Username: "testuser",
				Scopes:   []string{"read", "write"},
			},
		}
		ctx.Set("claims", claims)

		_ = wrappedHandler.Handle(ctx)

		assert.False(t, handlerCalled, "Handler should not be called when scope check fails")
		assert.Equal(t, 403, ctx.Response.StatusCode, "Response should have 403 status")
	})

	t.Run("no claims in context", func(t *testing.T) {
		liftAuthService := newTestLiftAuthService(nil)
		middleware := liftAuthService.RequireScope("read")

		handlerCalled := false
		testHandler := lift.HandlerFunc(func(ctx *lift.Context) error {
			handlerCalled = true
			return ctx.JSON(map[string]string{"success": "true"})
		})

		wrappedHandler := middleware(testHandler)
		ctx := createTestContext()

		_ = wrappedHandler.Handle(ctx)

		assert.False(t, handlerCalled, "Handler should not be called when no claims available")
		assert.Equal(t, 403, ctx.Response.StatusCode, "Response should have 403 status")
	})
}

// TestLiftAuthService_RequireTenant tests the RequireTenant middleware
func TestLiftAuthService_RequireTenant(t *testing.T) {
	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		expectError    bool
		expectedTenant string
	}{
		{
			name: "tenant from header",
			setupContext: func() *lift.Context {
				ctx := createTestContext()
				ctx.Request.Headers["X-Tenant-ID"] = "header-tenant"
				return ctx
			},
			expectError:    false,
			expectedTenant: "header-tenant",
		},
		{
			name: "tenant from subdomain",
			setupContext: func() *lift.Context {
				return createTestContextWithHost("mytenant.lesser.app")
			},
			expectError:    false,
			expectedTenant: "mytenant",
		},
		{
			name: "tenant from path",
			setupContext: func() *lift.Context {
				return createTestContextWithPath("/tenant/pathtenant/api/v1/users")
			},
			expectError:    false,
			expectedTenant: "pathtenant",
		},
		{
			name: "tenant from claims",
			setupContext: func() *lift.Context {
				ctx := createTestContext()
				claims := createValidClaims()
				ctx.Set("claims", claims)
				return ctx
			},
			expectError:    false,
			expectedTenant: "testuser", // Uses username as fallback
		},
		{
			name: "no tenant available",
			setupContext: func() *lift.Context {
				return createTestContext()
			},
			expectError: true,
		},
		{
			name: "reserved subdomain",
			setupContext: func() *lift.Context {
				return createTestContextWithHost("api.lesser.app")
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			liftAuthService := NewLiftAuthService(nil)
			middleware := liftAuthService.RequireTenant()

			handlerCalled := false
			testHandler := lift.HandlerFunc(func(ctx *lift.Context) error {
				handlerCalled = true
				return ctx.JSON(map[string]string{"success": "true"})
			})

			wrappedHandler := middleware(testHandler)
			ctx := tt.setupContext()

			err := wrappedHandler.Handle(ctx)

			if tt.expectError {
				assert.False(t, handlerCalled, "Handler should not be called when tenant resolution fails")
				// Should have error status set (either 403 or 400 depending on the error type)
				assert.True(t, ctx.Response.StatusCode >= 400, "Response should have error status code")
			} else {
				assert.NoError(t, err, "Should not return error when tenant resolution succeeds")
				assert.True(t, handlerCalled, "Handler should be called when tenant resolution succeeds")

				// Verify tenant was stored in context
				tenantID, ok := ctx.Get("tenant_id").(string)
				assert.True(t, ok, "Tenant ID should be stored in context")
				assert.Equal(t, tt.expectedTenant, tenantID)
			}
		})
	}
}

// TestExtractBearerToken tests the bearer token extraction logic
func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		name          string
		authHeader    string
		expectedToken string
	}{
		{
			name:          "valid bearer token",
			authHeader:    "Bearer abc123",
			expectedToken: "abc123",
		},
		{
			name:          "bearer with long token",
			authHeader:    "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
			expectedToken: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
		},
		{
			name:          "no authorization header",
			authHeader:    "",
			expectedToken: "",
		},
		{
			name:          "invalid format - basic auth",
			authHeader:    "Basic abc123",
			expectedToken: "",
		},
		{
			name:          "bearer without token",
			authHeader:    "Bearer ",
			expectedToken: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := createTestContext()
			if tt.authHeader != "" {
				ctx.Request.Headers["Authorization"] = tt.authHeader
			}

			token := extractBearerToken(ctx)
			assert.Equal(t, tt.expectedToken, token)
		})
	}
}

// TestContextHelperFunctions tests the context helper functions
func TestContextHelperFunctions(t *testing.T) {
	t.Run("GetClaims", func(t *testing.T) {
		ctx := createTestContext()
		expectedClaims := createValidClaims()
		ctx.Set("claims", expectedClaims)

		claims, err := GetClaims(ctx)
		assert.NoError(t, err)
		assert.Equal(t, expectedClaims, claims)

		// Test with no claims - should set 401 status and return error
		ctx2 := createTestContext()
		_, err = GetClaims(ctx2)
		// The helper function calls ctx.Unauthorized which sets status but may not return an error
		assert.Equal(t, 401, ctx2.Response.StatusCode, "Should set 401 status when no claims")
	})

	t.Run("GetUsername", func(t *testing.T) {
		ctx := createTestContext()
		ctx.Set("username", "testuser")

		username, err := GetUsername(ctx)
		assert.NoError(t, err)
		assert.Equal(t, "testuser", username)

		// Test with no username - should set 401 status
		ctx2 := createTestContext()
		_, err = GetUsername(ctx2)
		assert.Equal(t, 401, ctx2.Response.StatusCode, "Should set 401 status when no username")
	})

	t.Run("IsAuthenticated", func(t *testing.T) {
		ctx := createTestContext()
		assert.False(t, IsAuthenticated(ctx))

		ctx.Set("claims", createValidClaims())
		assert.True(t, IsAuthenticated(ctx))
	})

	t.Run("HasScope", func(t *testing.T) {
		ctx := createTestContext()
		assert.False(t, HasScope(ctx, "read"))

		claims := createValidClaims()
		ctx.Set("claims", claims)
		assert.True(t, HasScope(ctx, "read"))
		assert.False(t, HasScope(ctx, "admin"))
	})
}

// TestTenantExtractionFunctions tests the tenant extraction utility functions
func TestExtractTenantFromSubdomain(t *testing.T) {
	tests := []struct {
		name           string
		host           string
		expectedTenant string
	}{
		{
			name:           "valid tenant subdomain",
			host:           "mytenant.lesser.app",
			expectedTenant: "mytenant",
		},
		{
			name:           "reserved subdomain api",
			host:           "api.lesser.app",
			expectedTenant: "",
		},
		{
			name:           "reserved subdomain www",
			host:           "www.lesser.app",
			expectedTenant: "",
		},
		{
			name:           "no subdomain",
			host:           "lesser.app",
			expectedTenant: "",
		},
		{
			name:           "complex domain",
			host:           "tenant1.api.lesser.com",
			expectedTenant: "tenant1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractTenantFromSubdomain(tt.host)
			assert.Equal(t, tt.expectedTenant, result)
		})
	}
}

func TestExtractTenantFromPath(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		expectedTenant string
	}{
		{
			name:           "valid tenant path",
			path:           "/tenant/mycompany/api/v1/users",
			expectedTenant: "mycompany",
		},
		{
			name:           "tenant root",
			path:           "/tenant/mycompany/",
			expectedTenant: "mycompany",
		},
		{
			name:           "no tenant in path",
			path:           "/api/v1/users",
			expectedTenant: "",
		},
		{
			name:           "empty path",
			path:           "/",
			expectedTenant: "",
		},
		{
			name:           "tenant without value",
			path:           "/tenant/",
			expectedTenant: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractTenantFromPath(tt.path)
			assert.Equal(t, tt.expectedTenant, result)
		})
	}
}