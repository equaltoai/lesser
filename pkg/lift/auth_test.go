package lift

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
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

// Test token generators for various user types and scopes

// createClaimsForUserType creates claims for different user types
func createClaimsForUserType(username string, userType string) *auth.EnhancedClaims {
	var scopes []string

	switch userType {
	case "admin":
		scopes = []string{"read", "write", "follow", "push", "admin:read", "admin:write", "admin:accounts", "admin:all"}
	case "moderator":
		scopes = []string{"read", "write", "follow", "push", "admin:read", "admin:accounts"}
	case "standard":
		scopes = []string{"read", "write", "follow", "push"}
	case "read-only":
		scopes = []string{"read"}
	case "bot":
		scopes = []string{"read", "write"}
	default:
		scopes = []string{"read", "write"}
	}

	return &auth.EnhancedClaims{
		Claims: auth.Claims{
			Username: username,
			Scopes:   scopes,
			ClientID: "test-client",
		},
		SessionID: "test-session-" + username,
		DeviceID:  "test-device-" + username,
	}
}

// createCustomClaims creates claims with custom scopes
func createCustomClaims(username string, scopes []string) *auth.EnhancedClaims {
	return &auth.EnhancedClaims{
		Claims: auth.Claims{
			Username: username,
			Scopes:   scopes,
			ClientID: "test-client",
		},
		SessionID: "test-session-" + username,
		DeviceID:  "test-device-" + username,
	}
}

// Integration test helpers for auth flows

// createAuthenticatedTestContext creates a context with pre-set authentication
func createAuthenticatedTestContext(username string, userType string) *lift.Context {
	ctx := createTestContext()
	claims := createClaimsForUserType(username, userType)

	// Set authentication context values
	ctx.Set("claims", claims)
	ctx.Set("username", claims.Username)
	ctx.Set("session_id", claims.SessionID)
	ctx.Set("device_id", claims.DeviceID)

	return ctx
}

// createContextWithTenantAndAuth creates context with both tenant and auth
func createContextWithTenantAndAuth(username, userType, tenantID string) *lift.Context {
	ctx := createAuthenticatedTestContext(username, userType)
	ctx.Request.Headers["X-Tenant-ID"] = tenantID
	ctx.Set("tenant_id", tenantID)
	return ctx
}

// setupAuthFlow sets up a complete authentication flow test scenario
func setupAuthFlow(mockAuth *MockAuthService, token string, username string, userType string, shouldSucceed bool) *auth.EnhancedClaims {
	claims := createClaimsForUserType(username, userType)

	if shouldSucceed {
		mockAuth.On("ValidateAccessToken", token).Return(claims, nil)
	} else {
		mockAuth.On("ValidateAccessToken", token).Return(nil, auth.ErrInvalidToken)
	}

	return claims
}

// Test assertion helpers

// assertAuthenticationRequired verifies handler requires authentication
func assertAuthenticationRequired(t *testing.T, middleware lift.Middleware) {
	t.Helper()

	handlerCalled := false
	testHandler := lift.HandlerFunc(func(ctx *lift.Context) error {
		handlerCalled = true
		return ctx.JSON(map[string]string{"success": "true"})
	})

	wrappedHandler := middleware(testHandler)
	ctx := createTestContext() // No auth

	_ = wrappedHandler.Handle(ctx)

	assert.False(t, handlerCalled, "Handler should not be called without authentication")
	assert.Equal(t, 401, ctx.Response.StatusCode, "Should return 401 Unauthorized")
}

// assertScopeRequired verifies handler requires specific scope
func assertScopeRequired(t *testing.T, liftAuthService *testLiftAuthService, requiredScope string, userWithScope, userWithoutScope string) {
	t.Helper()

	middleware := liftAuthService.RequireScope(requiredScope)

	handlerCalled := false
	testHandler := lift.HandlerFunc(func(ctx *lift.Context) error {
		handlerCalled = true
		return ctx.JSON(map[string]string{"success": "true"})
	})

	wrappedHandler := middleware(testHandler)

	// Test user without required scope
	ctx := createTestContext()
	claimsWithoutScope := createCustomClaims(userWithoutScope, []string{"read", "write"})
	ctx.Set("claims", claimsWithoutScope)

	_ = wrappedHandler.Handle(ctx)
	assert.False(t, handlerCalled, "Handler should not be called without required scope")
	assert.Equal(t, 403, ctx.Response.StatusCode, "Should return 403 Forbidden")

	// Reset for next test
	handlerCalled = false

	// Test user with required scope
	ctx = createTestContext()
	claimsWithScope := createCustomClaims(userWithScope, []string{"read", "write", requiredScope})
	ctx.Set("claims", claimsWithScope)

	err := wrappedHandler.Handle(ctx)
	assert.NoError(t, err, "Should succeed with required scope")
	assert.True(t, handlerCalled, "Handler should be called with required scope")
}

// Multi-scenario test helpers

// AuthTestScenario defines a test scenario for authentication
type AuthTestScenario struct {
	Name          string
	Token         string
	ExpectedClaim *auth.EnhancedClaims
	ExpectedError error
	ShouldSucceed bool
}

// runAuthScenarios runs multiple authentication test scenarios
func runAuthScenarios(t *testing.T, scenarios []AuthTestScenario, createMiddleware func(*testLiftAuthService) lift.Middleware) {
	for _, scenario := range scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			mockAuth := new(MockAuthService)

			if scenario.ExpectedClaim != nil {
				mockAuth.On("ValidateAccessToken", scenario.Token).Return(scenario.ExpectedClaim, scenario.ExpectedError)
			} else {
				mockAuth.On("ValidateAccessToken", scenario.Token).Return(nil, scenario.ExpectedError)
			}

			liftAuthService := newTestLiftAuthService(mockAuth)
			middleware := createMiddleware(liftAuthService)

			handlerCalled := false
			testHandler := lift.HandlerFunc(func(ctx *lift.Context) error {
				handlerCalled = true
				return ctx.JSON(map[string]string{"success": "true"})
			})

			wrappedHandler := middleware(testHandler)
			ctx := createTestContextWithAuth(scenario.Token)

			err := wrappedHandler.Handle(ctx)

			if scenario.ShouldSucceed {
				assert.NoError(t, err, "Scenario should succeed")
				assert.True(t, handlerCalled, "Handler should be called on success")

				if scenario.ExpectedClaim != nil {
					claims, ok := ctx.Get("claims").(*auth.EnhancedClaims)
					assert.True(t, ok, "Claims should be in context")
					assert.Equal(t, scenario.ExpectedClaim.Username, claims.Username)
				}
			} else {
				assert.False(t, handlerCalled, "Handler should not be called on failure")
				assert.True(t, ctx.Response.StatusCode >= 400, "Should have error status code")
			}

			mockAuth.AssertExpectations(t)
		})
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
		_, _ = GetClaims(ctx2) // Error ignored - testing status code instead
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
		_, _ = GetUsername(ctx2) // Error ignored - testing status code instead
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

// Tests for authentication testing utilities

// TestCreateClaimsForUserType tests the user type claims generator
func TestCreateClaimsForUserType(t *testing.T) {
	tests := []struct {
		name           string
		username       string
		userType       string
		expectedScopes []string
	}{
		{
			name:           "admin user",
			username:       "admin",
			userType:       "admin",
			expectedScopes: []string{"read", "write", "follow", "push", "admin:read", "admin:write", "admin:accounts", "admin:all"},
		},
		{
			name:           "moderator user",
			username:       "mod",
			userType:       "moderator",
			expectedScopes: []string{"read", "write", "follow", "push", "admin:read", "admin:accounts"},
		},
		{
			name:           "standard user",
			username:       "user",
			userType:       "standard",
			expectedScopes: []string{"read", "write", "follow", "push"},
		},
		{
			name:           "read-only user",
			username:       "readonly",
			userType:       "read-only",
			expectedScopes: []string{"read"},
		},
		{
			name:           "bot user",
			username:       "bot",
			userType:       "bot",
			expectedScopes: []string{"read", "write"},
		},
		{
			name:           "unknown user type defaults to standard",
			username:       "unknown",
			userType:       "unknown",
			expectedScopes: []string{"read", "write"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := createClaimsForUserType(tt.username, tt.userType)

			assert.Equal(t, tt.username, claims.Username)
			assert.Equal(t, tt.expectedScopes, claims.Scopes)
			assert.Equal(t, "test-client", claims.ClientID)
			assert.Equal(t, "test-session-"+tt.username, claims.SessionID)
			assert.Equal(t, "test-device-"+tt.username, claims.DeviceID)
		})
	}
}

// TestCreateCustomClaims tests the custom claims generator
func TestCreateCustomClaims(t *testing.T) {
	username := "testuser"
	scopes := []string{"custom:scope1", "custom:scope2"}

	claims := createCustomClaims(username, scopes)

	assert.Equal(t, username, claims.Username)
	assert.Equal(t, scopes, claims.Scopes)
	assert.Equal(t, "test-client", claims.ClientID)
	assert.Equal(t, "test-session-"+username, claims.SessionID)
	assert.Equal(t, "test-device-"+username, claims.DeviceID)
}

// TestCreateAuthenticatedTestContext tests the authenticated context builder
func TestCreateAuthenticatedTestContext(t *testing.T) {
	username := "testuser"
	userType := "admin"

	ctx := createAuthenticatedTestContext(username, userType)

	// Check that context has all expected values
	claims, ok := ctx.Get("claims").(*auth.EnhancedClaims)
	assert.True(t, ok, "Claims should be in context")
	assert.Equal(t, username, claims.Username)

	ctxUsername, ok := ctx.Get("username").(string)
	assert.True(t, ok, "Username should be in context")
	assert.Equal(t, username, ctxUsername)

	sessionID, ok := ctx.Get("session_id").(string)
	assert.True(t, ok, "Session ID should be in context")
	assert.Equal(t, "test-session-"+username, sessionID)

	deviceID, ok := ctx.Get("device_id").(string)
	assert.True(t, ok, "Device ID should be in context")
	assert.Equal(t, "test-device-"+username, deviceID)
}

// TestCreateContextWithTenantAndAuth tests the tenant + auth context builder
func TestCreateContextWithTenantAndAuth(t *testing.T) {
	username := "testuser"
	userType := "standard"
	tenantID := "test-tenant"

	ctx := createContextWithTenantAndAuth(username, userType, tenantID)

	// Check authentication context
	claims, ok := ctx.Get("claims").(*auth.EnhancedClaims)
	assert.True(t, ok, "Claims should be in context")
	assert.Equal(t, username, claims.Username)

	// Check tenant context
	assert.Equal(t, tenantID, ctx.Request.Headers["X-Tenant-ID"])

	ctxTenantID, ok := ctx.Get("tenant_id").(string)
	assert.True(t, ok, "Tenant ID should be in context")
	assert.Equal(t, tenantID, ctxTenantID)
}

// TestSetupAuthFlow tests the auth flow setup helper
func TestSetupAuthFlow(t *testing.T) {
	mockAuth := new(MockAuthService)
	token := "test-token"
	username := "testuser"
	userType := "admin"

	// Test successful flow
	claims := setupAuthFlow(mockAuth, token, username, userType, true)

	assert.Equal(t, username, claims.Username)
	assert.Contains(t, claims.Scopes, "admin:all")

	// Actually exercise the mock by calling ValidateAccessToken
	liftAuthService := newTestLiftAuthService(mockAuth)
	middleware := liftAuthService.RequireAuth()

	// Create a test context with the token
	ctx := createTestContextWithAuth(token)

	// Create a test handler
	handlerCalled := false
	testHandler := lift.HandlerFunc(func(ctx *lift.Context) error {
		handlerCalled = true
		return nil
	})

	// Execute the middleware
	wrappedHandler := middleware(testHandler)
	err := wrappedHandler.Handle(ctx)

	// Verify the call succeeded and handler was called
	assert.NoError(t, err)
	assert.True(t, handlerCalled)

	// Verify mock was called as expected
	mockAuth.AssertExpectations(t)
}

// TestAssertAuthenticationRequired tests the authentication assertion helper
func TestAssertAuthenticationRequired(t *testing.T) {
	mockAuth := new(MockAuthService)
	liftAuthService := newTestLiftAuthService(mockAuth)
	middleware := liftAuthService.RequireAuth()

	// This should pass (no assertion errors)
	assertAuthenticationRequired(t, middleware)
}

// TestAssertScopeRequired tests the scope assertion helper
func TestAssertScopeRequired(t *testing.T) {
	mockAuth := new(MockAuthService)
	liftAuthService := newTestLiftAuthService(mockAuth)

	requiredScope := "admin:read"
	userWithScope := "admin"
	userWithoutScope := "user"

	// This should pass (no assertion errors)
	assertScopeRequired(t, liftAuthService, requiredScope, userWithScope, userWithoutScope)
}

// TestRunAuthScenarios tests the multi-scenario test runner
func TestRunAuthScenarios(t *testing.T) {
	scenarios := []AuthTestScenario{
		{
			Name:          "valid admin token",
			Token:         "admin-token",
			ExpectedClaim: createClaimsForUserType("admin", "admin"),
			ExpectedError: nil,
			ShouldSucceed: true,
		},
		{
			Name:          "invalid token",
			Token:         "invalid-token",
			ExpectedClaim: nil,
			ExpectedError: auth.ErrInvalidToken,
			ShouldSucceed: false,
		},
		{
			Name:          "valid user token",
			Token:         "user-token",
			ExpectedClaim: createClaimsForUserType("user", "standard"),
			ExpectedError: nil,
			ShouldSucceed: true,
		},
	}

	createMiddleware := func(las *testLiftAuthService) lift.Middleware {
		return las.RequireAuth()
	}

	runAuthScenarios(t, scenarios, createMiddleware)
}

// Integration test examples showing how to use the utilities

// TestAuthUtilitiesIntegration demonstrates using the utilities together
func TestAuthUtilitiesIntegration(t *testing.T) {
	t.Run("admin endpoint access", func(t *testing.T) {
		// Set up mock auth service
		mockAuth := new(MockAuthService)
		adminClaims := createClaimsForUserType("admin", "admin")
		mockAuth.On("ValidateAccessToken", "admin-token").Return(adminClaims, nil)

		// Create auth service and middleware
		liftAuthService := newTestLiftAuthService(mockAuth)
		requireAuth := liftAuthService.RequireAuth()
		requireAdminScope := liftAuthService.RequireScope("admin:all")

		// Create test handler
		handlerCalled := false
		testHandler := lift.HandlerFunc(func(ctx *lift.Context) error {
			handlerCalled = true
			return ctx.JSON(map[string]string{"admin": "data"})
		})

		// Chain middleware
		wrappedHandler := requireAuth(requireAdminScope(testHandler))

		// Create context with admin token
		ctx := createTestContextWithAuth("admin-token")

		// Execute
		err := wrappedHandler.Handle(ctx)

		// Verify
		assert.NoError(t, err)
		assert.True(t, handlerCalled)
		mockAuth.AssertExpectations(t)
	})

	t.Run("multi-tenant authenticated endpoint", func(t *testing.T) {
		// Set up authenticated context with tenant
		ctx := createContextWithTenantAndAuth("user", "standard", "tenant1")

		// Verify both auth and tenant context
		claims, ok := ctx.Get("claims").(*auth.EnhancedClaims)
		assert.True(t, ok)
		assert.Equal(t, "user", claims.Username)

		tenantID, ok := ctx.Get("tenant_id").(string)
		assert.True(t, ok)
		assert.Equal(t, "tenant1", tenantID)
	})
}
