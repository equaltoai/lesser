package auth

import (
	"context"
	"fmt"
	"testing"

	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
)

// mockOAuthService is a stub implementation of OAuthServiceInterface for testing
type mockOAuthService struct {
	tokens map[string]*Claims // maps token string → Claims
}

// newMockOAuthService creates a mock OAuth service with predefined tokens
func newMockOAuthService() *mockOAuthService {
	return &mockOAuthService{
		tokens: make(map[string]*Claims),
	}
}

// addToken registers a token with the mock service
func (m *mockOAuthService) addToken(token string, claims *Claims) {
	m.tokens[token] = claims
}

// ValidateAccessToken implements OAuthServiceInterface
func (m *mockOAuthService) ValidateAccessToken(token string) (*Claims, error) {
	if claims, ok := m.tokens[token]; ok {
		return claims, nil
	}
	return nil, ErrInvalidToken
}

// createTestClaims creates a Claims object with the given username and scopes
func createTestClaims(username string, scopes []string) *Claims {
	return &Claims{
		Username: username,
		Scopes:   scopes,
	}
}

// ============================================================================
// 1) Header + Bearer Token Extraction via GetAccountFromContext
// ============================================================================

func TestGetAccountFromContext(t *testing.T) {
	tests := []struct {
		name          string
		authHeader    string
		setupMock     func(*mockOAuthService)
		wantUsername  string
		wantErrorCode apperrors.ErrorCode
		wantErrMsg    string
	}{
		{
			name:          "missing authorization header",
			authHeader:    "",
			setupMock:     func(_ *mockOAuthService) {},
			wantErrorCode: apperrors.CodeUnauthorized,
			wantErrMsg:    "missing authorization header",
		},
		{
			name:          "invalid header format - not Bearer",
			authHeader:    "Basic dXNlcjpwYXNz",
			setupMock:     func(_ *mockOAuthService) {},
			wantErrorCode: apperrors.CodeUnauthorized,
			wantErrMsg:    "invalid bearer token format",
		},
		{
			name:          "invalid header format - only Bearer word",
			authHeader:    "Bearer",
			setupMock:     func(_ *mockOAuthService) {},
			wantErrorCode: apperrors.CodeUnauthorized,
			wantErrMsg:    "invalid bearer token format",
		},
		{
			name:       "token validation error",
			authHeader: "Bearer invalid-token",
			setupMock:  func(_ *mockOAuthService) {},
			// Mock has no tokens registered, so validation will fail
			wantErrorCode: apperrors.CodeUnauthorized,
			wantErrMsg:    "invalid access token",
		},
		{
			name:       "valid token returns authenticated account",
			authHeader: "Bearer valid-token-123",
			setupMock: func(m *mockOAuthService) {
				m.addToken("valid-token-123", createTestClaims("testuser", []string{ScopeRead, ScopeWrite}))
			},
			wantUsername: "testuser",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := []apptheoryContextOption{}
			if tt.authHeader != "" {
				opts = append(opts, withHeaders(map[string]string{"Authorization": tt.authHeader}))
			}
			ctx := newTestContext("GET", "/api/test", opts...)

			// Setup mock OAuth service
			mockService := newMockOAuthService()
			tt.setupMock(mockService)

			// Execute
			account, err := GetAccountFromContext(ctx, mockService)

			// Assert
			if tt.wantErrorCode != "" {
				require.Error(t, err)
				assert.True(t, apperrors.HasCode(err, tt.wantErrorCode),
					"expected error code %s, got %v", tt.wantErrorCode, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)
				assert.Nil(t, account)
			} else {
				require.NoError(t, err)
				require.NotNil(t, account)
				assert.Equal(t, tt.wantUsername, account.Username)
				assert.NotNil(t, account.Claims)
				assert.Equal(t, tt.wantUsername, account.Claims.GetUsername())
			}
		})
	}
}

func TestGetUsernameFromContext(t *testing.T) {
	t.Run("returns username from valid token", func(t *testing.T) {
		ctx := newTestContext("GET", "/api/test", withHeaders(map[string]string{"Authorization": "Bearer valid-token"}))

		mockService := newMockOAuthService()
		mockService.addToken("valid-token", createTestClaims("alice", []string{ScopeRead}))

		username, err := GetUsernameFromContext(ctx, mockService)
		require.NoError(t, err)
		assert.Equal(t, "alice", username)
	})

	t.Run("returns error on missing auth", func(t *testing.T) {
		ctx := newTestContext("GET", "/api/test")
		mockService := newMockOAuthService()

		username, err := GetUsernameFromContext(ctx, mockService)
		require.Error(t, err)
		assert.True(t, apperrors.HasCode(err, apperrors.CodeUnauthorized))
		assert.Empty(t, username)
	})
}

func TestRequireAuth(t *testing.T) {
	t.Run("returns nil for authenticated request", func(t *testing.T) {
		ctx := newTestContext("GET", "/api/test", withHeaders(map[string]string{"Authorization": "Bearer token123"}))

		mockService := newMockOAuthService()
		mockService.addToken("token123", createTestClaims("bob", []string{ScopeRead}))

		err := RequireAuth(ctx, mockService)
		assert.NoError(t, err)
	})

	t.Run("returns error for unauthenticated request", func(t *testing.T) {
		ctx := newTestContext("GET", "/api/test")
		mockService := newMockOAuthService()

		err := RequireAuth(ctx, mockService)
		require.Error(t, err)
		assert.True(t, apperrors.HasCode(err, apperrors.CodeUnauthorized))
	})
}

// ============================================================================
// 2) Scope Enforcement Helpers
// ============================================================================

func TestRequireAuthWithScope(t *testing.T) {
	tests := []struct {
		name          string
		tokenScopes   []string
		requiredScope string
		wantErrorCode apperrors.ErrorCode
		wantSuccess   bool
	}{
		{
			name:          "has required scope - success",
			tokenScopes:   []string{ScopeRead, ScopeWrite},
			requiredScope: ScopeRead,
			wantSuccess:   true,
		},
		{
			name:          "missing required scope - forbidden",
			tokenScopes:   []string{ScopeRead},
			requiredScope: ScopeWrite,
			wantErrorCode: apperrors.CodeForbidden,
		},
		{
			name:          "no scopes at all - forbidden",
			tokenScopes:   []string{},
			requiredScope: ScopeRead,
			wantErrorCode: apperrors.CodeForbidden,
		},
		{
			name:          "has admin scope",
			tokenScopes:   []string{ScopeAdmin},
			requiredScope: ScopeAdmin,
			wantSuccess:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newTestContext("POST", "/api/test", withHeaders(map[string]string{"Authorization": "Bearer scope-token"}))

			mockService := newMockOAuthService()
			mockService.addToken("scope-token", createTestClaims("user1", tt.tokenScopes))

			account, err := RequireAuthWithScope(ctx, mockService, tt.requiredScope)

			if tt.wantSuccess {
				require.NoError(t, err)
				require.NotNil(t, account)
				assert.Equal(t, "user1", account.Username)
			} else {
				require.Error(t, err)
				assert.True(t, apperrors.HasCode(err, tt.wantErrorCode),
					"expected error code %s, got %v", tt.wantErrorCode, err)
				assert.Contains(t, err.Error(), "insufficient scope")
				assert.Nil(t, account)
			}
		})
	}

	t.Run("no auth header - unauthorized", func(t *testing.T) {
		ctx := newTestContext("POST", "/api/test")
		mockService := newMockOAuthService()

		account, err := RequireAuthWithScope(ctx, mockService, ScopeWrite)
		require.Error(t, err)
		assert.True(t, apperrors.HasCode(err, apperrors.CodeUnauthorized))
		assert.Nil(t, account)
	})
}

func TestRequireAuthWithMultipleScopes(t *testing.T) {
	tests := []struct {
		name          string
		tokenScopes   []string
		allowedScopes []string
		wantSuccess   bool
		wantErrorCode apperrors.ErrorCode
	}{
		{
			name:          "any-of scopes matches first one",
			tokenScopes:   []string{ScopeRead},
			allowedScopes: []string{ScopeRead, ScopeWrite},
			wantSuccess:   true,
		},
		{
			name:          "any-of scopes matches second one",
			tokenScopes:   []string{ScopeWrite},
			allowedScopes: []string{ScopeRead, ScopeWrite},
			wantSuccess:   true,
		},
		{
			name:          "any-of scopes matches all",
			tokenScopes:   []string{ScopeRead, ScopeWrite},
			allowedScopes: []string{ScopeRead, ScopeWrite},
			wantSuccess:   true,
		},
		{
			name:          "none-of scopes - forbidden",
			tokenScopes:   []string{ScopeAdmin},
			allowedScopes: []string{ScopeRead, ScopeWrite},
			wantErrorCode: apperrors.CodeForbidden,
		},
		{
			name:          "empty token scopes - forbidden",
			tokenScopes:   []string{},
			allowedScopes: []string{ScopeRead},
			wantErrorCode: apperrors.CodeForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newTestContext("PUT", "/api/test", withHeaders(map[string]string{"Authorization": "Bearer multi-scope-token"}))

			mockService := newMockOAuthService()
			mockService.addToken("multi-scope-token", createTestClaims("user2", tt.tokenScopes))

			account, err := RequireAuthWithMultipleScopes(ctx, mockService, tt.allowedScopes)

			if tt.wantSuccess {
				require.NoError(t, err)
				require.NotNil(t, account)
			} else {
				require.Error(t, err)
				assert.True(t, apperrors.HasCode(err, tt.wantErrorCode))
				assert.Contains(t, err.Error(), "insufficient scope")
				assert.Nil(t, account)
			}
		})
	}
}

func TestConvenienceScopeHelpers(t *testing.T) {
	t.Run("RequireReadScope", func(t *testing.T) {
		ctx := newTestContext("GET", "/api/test", withHeaders(map[string]string{"Authorization": "Bearer read-token"}))

		mockService := newMockOAuthService()
		mockService.addToken("read-token", createTestClaims("reader", []string{ScopeRead}))

		account, err := RequireReadScope(ctx, mockService)
		require.NoError(t, err)
		assert.Equal(t, "reader", account.Username)

		// Test failure case
		mockService.addToken("no-read-token", createTestClaims("noread", []string{ScopeWrite}))
		ctx2 := newTestContext("GET", "/api/test", withHeaders(map[string]string{"Authorization": "Bearer no-read-token"}))

		account, err = RequireReadScope(ctx2, mockService)
		require.Error(t, err)
		assert.True(t, apperrors.HasCode(err, apperrors.CodeForbidden))
		assert.Nil(t, account)
	})

	t.Run("RequireWriteScope", func(t *testing.T) {
		ctx := newTestContext("POST", "/api/test", withHeaders(map[string]string{"Authorization": "Bearer write-token"}))

		mockService := newMockOAuthService()
		mockService.addToken("write-token", createTestClaims("writer", []string{ScopeWrite}))

		account, err := RequireWriteScope(ctx, mockService)
		require.NoError(t, err)
		assert.Equal(t, "writer", account.Username)
	})

	t.Run("RequireAdminScope", func(t *testing.T) {
		ctx := newTestContext("DELETE", "/api/admin", withHeaders(map[string]string{"Authorization": "Bearer admin-token"}))

		mockService := newMockOAuthService()
		mockService.addToken("admin-token", createTestClaims("admin", []string{ScopeAdmin}))

		account, err := RequireAdminScope(ctx, mockService)
		require.NoError(t, err)
		assert.Equal(t, "admin", account.Username)

		// Non-admin should fail
		mockService.addToken("non-admin", createTestClaims("regular", []string{ScopeRead, ScopeWrite}))
		ctx2 := newTestContext("DELETE", "/api/admin", withHeaders(map[string]string{"Authorization": "Bearer non-admin"}))

		account, err = RequireAdminScope(ctx2, mockService)
		require.Error(t, err)
		assert.True(t, apperrors.HasCode(err, apperrors.CodeForbidden))
	})

	t.Run("RequireReadOrWriteScope", func(t *testing.T) {
		mockService := newMockOAuthService()
		mockService.addToken("reader", createTestClaims("r", []string{ScopeRead}))
		mockService.addToken("writer", createTestClaims("w", []string{ScopeWrite}))
		mockService.addToken("neither", createTestClaims("n", []string{ScopeAdmin}))

		// Read scope should work
		ctx := newTestContext("GET", "/api/test", withHeaders(map[string]string{"Authorization": "Bearer reader"}))
		account, err := RequireReadOrWriteScope(ctx, mockService)
		require.NoError(t, err)
		assert.Equal(t, "r", account.Username)

		// Write scope should work
		ctx2 := newTestContext("POST", "/api/test", withHeaders(map[string]string{"Authorization": "Bearer writer"}))
		account, err = RequireReadOrWriteScope(ctx2, mockService)
		require.NoError(t, err)
		assert.Equal(t, "w", account.Username)

		// Neither should fail
		ctx3 := newTestContext("GET", "/api/test", withHeaders(map[string]string{"Authorization": "Bearer neither"}))
		account, err = RequireReadOrWriteScope(ctx3, mockService)
		require.Error(t, err)
		assert.True(t, apperrors.HasCode(err, apperrors.CodeForbidden))
	})
}

// ============================================================================
// 3) Optional Auth
// ============================================================================

func TestExtractOptionalAuth(t *testing.T) {
	tests := []struct {
		name         string
		authHeader   string
		setupMock    func(*mockOAuthService)
		wantAccount  bool
		wantUsername string
	}{
		{
			name:        "no auth header - returns nil, nil",
			authHeader:  "",
			setupMock:   func(_ *mockOAuthService) {},
			wantAccount: false,
		},
		{
			name:        "invalid bearer format - returns nil, nil",
			authHeader:  "Basic dXNlcjpwYXNz",
			setupMock:   func(_ *mockOAuthService) {},
			wantAccount: false,
		},
		{
			name:        "invalid token - returns nil, nil",
			authHeader:  "Bearer invalid-token",
			setupMock:   func(_ *mockOAuthService) {},
			wantAccount: false,
		},
		{
			name:       "valid token - returns account",
			authHeader: "Bearer optional-valid",
			setupMock: func(m *mockOAuthService) {
				m.addToken("optional-valid", createTestClaims("optionaluser", []string{ScopeRead}))
			},
			wantAccount:  true,
			wantUsername: "optionaluser",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := []apptheoryContextOption{}
			if tt.authHeader != "" {
				opts = append(opts, withHeaders(map[string]string{"Authorization": tt.authHeader}))
			}
			ctx := newTestContext("GET", "/public/endpoint", opts...)

			mockService := newMockOAuthService()
			tt.setupMock(mockService)

			account, err := ExtractOptionalAuth(ctx, mockService)

			// Optional auth should NEVER return an error
			assert.NoError(t, err)

			if tt.wantAccount {
				require.NotNil(t, account)
				assert.Equal(t, tt.wantUsername, account.Username)
			} else {
				assert.Nil(t, account)
			}
		})
	}
}

// ============================================================================
// 4) Ownership Guards
// ============================================================================

func TestValidateAccountOwnership(t *testing.T) {
	tests := []struct {
		name          string
		accountUser   string
		targetUser    string
		wantErrorCode apperrors.ErrorCode
	}{
		{
			name:        "equal usernames - success",
			accountUser: "alice",
			targetUser:  "alice",
		},
		{
			name:          "different usernames - forbidden",
			accountUser:   "alice",
			targetUser:    "bob",
			wantErrorCode: apperrors.CodeForbidden,
		},
		{
			name:          "case sensitive mismatch - forbidden",
			accountUser:   "Alice",
			targetUser:    "alice",
			wantErrorCode: apperrors.CodeForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &AuthenticatedAccount{
				Username: tt.accountUser,
				Claims:   createTestClaims(tt.accountUser, []string{ScopeRead}),
			}

			err := ValidateAccountOwnership(account, tt.targetUser)

			if tt.wantErrorCode != "" {
				require.Error(t, err)
				assert.True(t, apperrors.HasCode(err, tt.wantErrorCode))
				assert.Contains(t, err.Error(), "not authorized")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateAccountOwnershipOrAdmin(t *testing.T) {
	tests := []struct {
		name          string
		accountUser   string
		accountScopes []string
		targetUser    string
		wantErrorCode apperrors.ErrorCode
	}{
		{
			name:          "equal usernames - success",
			accountUser:   "alice",
			accountScopes: []string{ScopeRead},
			targetUser:    "alice",
		},
		{
			name:          "admin scope - success even with different username",
			accountUser:   "admin",
			accountScopes: []string{ScopeAdmin},
			targetUser:    "targetuser",
		},
		{
			name:          "admin can access any account",
			accountUser:   "superadmin",
			accountScopes: []string{ScopeRead, ScopeWrite, ScopeAdmin},
			targetUser:    "anyone",
		},
		{
			name:          "mismatch without admin - forbidden",
			accountUser:   "alice",
			accountScopes: []string{ScopeRead, ScopeWrite},
			targetUser:    "bob",
			wantErrorCode: apperrors.CodeForbidden,
		},
		{
			name:          "no scopes mismatch - forbidden",
			accountUser:   "user1",
			accountScopes: []string{},
			targetUser:    "user2",
			wantErrorCode: apperrors.CodeForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &AuthenticatedAccount{
				Username: tt.accountUser,
				Claims:   createTestClaims(tt.accountUser, tt.accountScopes),
			}

			err := ValidateAccountOwnershipOrAdmin(account, tt.targetUser)

			if tt.wantErrorCode != "" {
				require.Error(t, err)
				assert.True(t, apperrors.HasCode(err, tt.wantErrorCode))
				assert.Contains(t, err.Error(), "not authorized")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// ============================================================================
// 5) Lift Middleware Wrappers
// ============================================================================

func TestNewAuthenticationMiddleware(t *testing.T) {
	mockService := newMockOAuthService()
	mw := NewAuthenticationMiddleware(mockService)

	assert.NotNil(t, mw)
	assert.Equal(t, mockService, mw.oauthService)
}

func TestRequireAuthMiddleware(t *testing.T) {
	t.Run("missing auth - writes 401 response", func(t *testing.T) {
		mockService := newMockOAuthService()
		mw := NewAuthenticationMiddleware(mockService)
		handler := mw.RequireAuthMiddleware()(func(*apptheory.Context) (*apptheory.Response, error) {
			return &apptheory.Response{Status: 200}, nil
		})

		ctx := newTestContext("GET", "/protected")
		resp, err := handler(ctx)
		require.NoError(t, err)
		assert.Equal(t, 401, resp.Status)
	})

	t.Run("valid auth - passes through", func(t *testing.T) {
		mockService := newMockOAuthService()
		mockService.addToken("valid", createTestClaims("user", []string{ScopeRead}))
		mw := NewAuthenticationMiddleware(mockService)
		handler := mw.RequireAuthMiddleware()(func(*apptheory.Context) (*apptheory.Response, error) {
			return &apptheory.Response{Status: 200}, nil
		})

		ctx := newTestContext("GET", "/protected", withHeaders(map[string]string{"Authorization": "Bearer valid"}))
		resp, err := handler(ctx)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.Status)
	})
}

func TestRequireScopeMiddleware(t *testing.T) {
	t.Run("auth failure - 401 response", func(t *testing.T) {
		mockService := newMockOAuthService()
		mw := NewAuthenticationMiddleware(mockService)
		handler := mw.RequireScopeMiddleware(ScopeWrite)(func(*apptheory.Context) (*apptheory.Response, error) {
			return &apptheory.Response{Status: 200}, nil
		})

		ctx := newTestContext("POST", "/write-endpoint")
		resp, err := handler(ctx)
		require.NoError(t, err)
		assert.Equal(t, 401, resp.Status)
	})

	t.Run("scope failure - 403 response", func(t *testing.T) {
		mockService := newMockOAuthService()
		mockService.addToken("read-only", createTestClaims("user", []string{ScopeRead}))
		mw := NewAuthenticationMiddleware(mockService)
		handler := mw.RequireScopeMiddleware(ScopeWrite)(func(*apptheory.Context) (*apptheory.Response, error) {
			return &apptheory.Response{Status: 200}, nil
		})

		ctx := newTestContext("POST", "/write-endpoint", withHeaders(map[string]string{"Authorization": "Bearer read-only"}))
		resp, err := handler(ctx)
		require.NoError(t, err)
		assert.Equal(t, 403, resp.Status)
	})

	t.Run("has scope - passes through", func(t *testing.T) {
		mockService := newMockOAuthService()
		mockService.addToken("writer", createTestClaims("writer", []string{ScopeWrite}))
		mw := NewAuthenticationMiddleware(mockService)
		handler := mw.RequireScopeMiddleware(ScopeWrite)(func(*apptheory.Context) (*apptheory.Response, error) {
			return &apptheory.Response{Status: 200}, nil
		})

		ctx := newTestContext("POST", "/write-endpoint", withHeaders(map[string]string{"Authorization": "Bearer writer"}))
		resp, err := handler(ctx)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.Status)
	})
}

func TestOptionalAuthMiddleware(t *testing.T) {
	t.Run("valid token sets authenticated_account in context", func(t *testing.T) {
		mockService := newMockOAuthService()
		mockService.addToken("opttoken", createTestClaims("optuser", []string{ScopeRead}))
		mw := NewAuthenticationMiddleware(mockService)
		handler := mw.OptionalAuthMiddleware()(func(*apptheory.Context) (*apptheory.Response, error) {
			return &apptheory.Response{Status: 200}, nil
		})

		ctx := newTestContext("GET", "/public", withHeaders(map[string]string{"Authorization": "Bearer opttoken"}))
		resp, err := handler(ctx)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.Status)

		// Verify the account was set in context
		rawValue := ctx.Get("authenticated_account")
		require.NotNil(t, rawValue)

		account, ok := rawValue.(*AuthenticatedAccount)
		require.True(t, ok)
		assert.Equal(t, "optuser", account.Username)
	})

	t.Run("no auth - does not set account", func(t *testing.T) {
		mockService := newMockOAuthService()
		mw := NewAuthenticationMiddleware(mockService)
		handler := mw.OptionalAuthMiddleware()(func(*apptheory.Context) (*apptheory.Response, error) {
			return &apptheory.Response{Status: 200}, nil
		})

		ctx := newTestContext("GET", "/public")
		resp, err := handler(ctx)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.Status)

		rawValue := ctx.Get("authenticated_account")
		assert.Nil(t, rawValue)
	})

	t.Run("invalid token - does not set account, no error", func(t *testing.T) {
		mockService := newMockOAuthService()
		mw := NewAuthenticationMiddleware(mockService)
		handler := mw.OptionalAuthMiddleware()(func(*apptheory.Context) (*apptheory.Response, error) {
			return &apptheory.Response{Status: 200}, nil
		})

		ctx := newTestContext("GET", "/public", withHeaders(map[string]string{"Authorization": "Bearer invalid"}))
		resp, err := handler(ctx)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.Status)

		rawValue := ctx.Get("authenticated_account")
		assert.Nil(t, rawValue)
	})
}

func TestGetAuthenticatedAccountFromContext(t *testing.T) {
	t.Run("account exists and is correct type - returns account, true", func(t *testing.T) {
		ctx := newTestContext("GET", "/api")
		expectedAccount := &AuthenticatedAccount{
			Username: "fromctx",
			Claims:   createTestClaims("fromctx", []string{ScopeRead}),
		}
		ctx.Set("authenticated_account", expectedAccount)

		account, ok := GetAuthenticatedAccountFromContext(ctx)

		assert.True(t, ok)
		require.NotNil(t, account)
		assert.Equal(t, "fromctx", account.Username)
	})

	t.Run("no authenticated_account key - returns nil, false", func(t *testing.T) {
		ctx := newTestContext("GET", "/api")

		account, ok := GetAuthenticatedAccountFromContext(ctx)

		assert.False(t, ok)
		assert.Nil(t, account)
	})

	t.Run("wrong type stored - returns nil, false", func(t *testing.T) {
		ctx := newTestContext("GET", "/api")
		ctx.Set("authenticated_account", "not-an-account") // wrong type

		account, ok := GetAuthenticatedAccountFromContext(ctx)

		assert.False(t, ok)
		assert.Nil(t, account)
	})

	t.Run("nil value stored - returns nil, false", func(t *testing.T) {
		ctx := newTestContext("GET", "/api")
		ctx.Set("authenticated_account", nil)

		account, ok := GetAuthenticatedAccountFromContext(ctx)

		assert.False(t, ok)
		assert.Nil(t, account)
	})
}

// ============================================================================
// 6) Standard context.Context Helpers
// ============================================================================

func TestSetAccountInStandardContext(t *testing.T) {
	t.Run("sets account in context", func(t *testing.T) {
		originalCtx := context.Background()
		account := &AuthenticatedAccount{
			Username: "stduser",
			Claims:   createTestClaims("stduser", []string{ScopeWrite}),
		}

		newCtx := SetAccountInStandardContext(originalCtx, account)

		// Verify the account is in the new context
		stored := newCtx.Value(ContextKeyAuthenticatedAccount)
		require.NotNil(t, stored)

		retrievedAccount, ok := stored.(*AuthenticatedAccount)
		require.True(t, ok)
		assert.Equal(t, "stduser", retrievedAccount.Username)
	})

	t.Run("original context unchanged", func(t *testing.T) {
		originalCtx := context.Background()
		account := &AuthenticatedAccount{Username: "user"}

		_ = SetAccountInStandardContext(originalCtx, account)

		// Original context should not have the value
		assert.Nil(t, originalCtx.Value(ContextKeyAuthenticatedAccount))
	})
}

func TestGetAccountFromStandardContext(t *testing.T) {
	t.Run("success - account exists", func(t *testing.T) {
		account := &AuthenticatedAccount{
			Username: "existinguser",
			Claims:   createTestClaims("existinguser", []string{ScopeRead}),
		}
		ctx := context.WithValue(context.Background(), ContextKeyAuthenticatedAccount, account)

		retrieved, err := GetAccountFromStandardContext(ctx)

		require.NoError(t, err)
		require.NotNil(t, retrieved)
		assert.Equal(t, "existinguser", retrieved.Username)
	})

	t.Run("unauthorized - no account in context", func(t *testing.T) {
		ctx := context.Background()

		retrieved, err := GetAccountFromStandardContext(ctx)

		require.Error(t, err)
		assert.True(t, apperrors.HasCode(err, apperrors.CodeUnauthorized))
		assert.Contains(t, err.Error(), "no authenticated account")
		assert.Nil(t, retrieved)
	})

	t.Run("unauthorized - wrong type stored", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), ContextKeyAuthenticatedAccount, "not-an-account")

		retrieved, err := GetAccountFromStandardContext(ctx)

		require.Error(t, err)
		assert.True(t, apperrors.HasCode(err, apperrors.CodeUnauthorized))
		assert.Nil(t, retrieved)
	})
}

func TestGetUsernameFromStandardContext(t *testing.T) {
	t.Run("success - returns username", func(t *testing.T) {
		account := &AuthenticatedAccount{
			Username: "extracteduser",
			Claims:   createTestClaims("extracteduser", []string{ScopeRead}),
		}
		ctx := SetAccountInStandardContext(context.Background(), account)

		username, err := GetUsernameFromStandardContext(ctx)

		require.NoError(t, err)
		assert.Equal(t, "extracteduser", username)
	})

	t.Run("failure - no account", func(t *testing.T) {
		ctx := context.Background()

		username, err := GetUsernameFromStandardContext(ctx)

		require.Error(t, err)
		assert.True(t, apperrors.HasCode(err, apperrors.CodeUnauthorized))
		assert.Empty(t, username)
	})
}

func TestRequireAuthFromStandardContext(t *testing.T) {
	t.Run("success - account exists", func(t *testing.T) {
		account := &AuthenticatedAccount{
			Username: "authuser",
			Claims:   createTestClaims("authuser", []string{ScopeRead}),
		}
		ctx := SetAccountInStandardContext(context.Background(), account)

		err := RequireAuthFromStandardContext(ctx)

		assert.NoError(t, err)
	})

	t.Run("failure - no account", func(t *testing.T) {
		ctx := context.Background()

		err := RequireAuthFromStandardContext(ctx)

		require.Error(t, err)
		assert.True(t, apperrors.HasCode(err, apperrors.CodeUnauthorized))
	})
}

// ============================================================================
// Additional Edge Cases
// ============================================================================

func TestHeaderExtractionFallbacks(t *testing.T) {
	t.Run("lowercase authorization header", func(t *testing.T) {
		ctx := newTestContext("GET", "/api/test", withHeaders(map[string]string{
			"authorization": "Bearer lowercasetoken",
		}))

		mockService := newMockOAuthService()
		mockService.addToken("lowercasetoken", createTestClaims("loweruser", []string{ScopeRead}))

		account, err := GetAccountFromContext(ctx, mockService)
		require.NoError(t, err)
		require.NotNil(t, account)
		assert.Equal(t, "loweruser", account.Username)
	})
}

func TestClaimsIntegration(t *testing.T) {
	t.Run("claims methods work correctly", func(t *testing.T) {
		claims := createTestClaims("TestUser", []string{ScopeRead, ScopeWrite})

		assert.Equal(t, "testuser", claims.GetUsername())
		assert.True(t, claims.HasScope(ScopeRead))
		assert.True(t, claims.HasScope(ScopeWrite))
		assert.False(t, claims.HasScope(ScopeAdmin))
	})
}

func TestMiddlewareChaining(t *testing.T) {
	t.Run("optional auth followed by scope check", func(t *testing.T) {
		mockService := newMockOAuthService()
		mockService.addToken("chaintoken", createTestClaims("chainuser", []string{ScopeRead, ScopeWrite}))
		mw := NewAuthenticationMiddleware(mockService)

		// Simulate middleware chain: optional auth first
		ctx := newTestContext("POST", "/api/resource", withHeaders(map[string]string{
			"Authorization": "Bearer chaintoken",
		}))

		optHandler := mw.OptionalAuthMiddleware()(func(*apptheory.Context) (*apptheory.Response, error) {
			return &apptheory.Response{Status: 200}, nil
		})
		_, err := optHandler(ctx)
		require.NoError(t, err)

		// Now check that the account was set
		account, ok := GetAuthenticatedAccountFromContext(ctx)
		require.True(t, ok)
		assert.Equal(t, "chainuser", account.Username)

		// Verify scope is present
		assert.True(t, account.Claims.HasScope(ScopeWrite))
	})
}

func TestErrorMessages(t *testing.T) {
	t.Run("scope error message contains required scope", func(t *testing.T) {
		ctx := newTestContext("POST", "/api/test", withHeaders(map[string]string{"Authorization": "Bearer nowrite"}))

		mockService := newMockOAuthService()
		mockService.addToken("nowrite", createTestClaims("user", []string{ScopeRead}))

		_, err := RequireAuthWithScope(ctx, mockService, ScopeWrite)

		require.Error(t, err)
		assert.Contains(t, err.Error(), ScopeWrite)
		assert.Contains(t, err.Error(), "insufficient scope")
	})

	t.Run("multiple scopes error message shows all required", func(t *testing.T) {
		ctx := newTestContext("POST", "/api/test", withHeaders(map[string]string{"Authorization": "Bearer admin"}))

		mockService := newMockOAuthService()
		mockService.addToken("admin", createTestClaims("user", []string{ScopeAdmin}))

		_, err := RequireAuthWithMultipleScopes(ctx, mockService, []string{ScopeRead, ScopeWrite})

		require.Error(t, err)
		errStr := err.Error()
		assert.Contains(t, errStr, "insufficient scope")
		// The error message should indicate the required scopes
		assert.Contains(t, errStr, fmt.Sprintf("%v", []string{ScopeRead, ScopeWrite}))
	})
}
