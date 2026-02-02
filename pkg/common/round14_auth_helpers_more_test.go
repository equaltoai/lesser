package common

import (
	stdErrors "errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
)

type oauthServiceStub struct {
	validateFunc func(token string) (Claims, error)
}

func (s oauthServiceStub) ValidateAccessToken(token string) (Claims, error) {
	return s.validateFunc(token)
}

func TestAuthHelpers_ExtractAndValidateAuth(t *testing.T) {
	t.Run("missing header returns 401", func(t *testing.T) {
		ctx := newTestContext("GET", "/test")
		res := ExtractAndValidateAuth(ctx, ScopeRead, oauthServiceStub{})
		require.Error(t, res.Error)
		assert.Equal(t, 401, res.ErrorCode)
	})

	t.Run("invalid bearer prefix returns 401", func(t *testing.T) {
		ctx := newTestContext("GET", "/test", withHeaders(map[string]string{
			"Authorization": "Token abc",
		}))
		res := ExtractAndValidateAuth(ctx, ScopeRead, oauthServiceStub{
			validateFunc: func(string) (Claims, error) { return nil, nil },
		})
		require.Error(t, res.Error)
		assert.Equal(t, 401, res.ErrorCode)
	})

	t.Run("oauth validation failure returns 401", func(t *testing.T) {
		ctx := newTestContext("GET", "/test", withHeaders(map[string]string{
			"Authorization": "Bearer token",
		}))
		res := ExtractAndValidateAuth(ctx, ScopeRead, oauthServiceStub{
			validateFunc: func(string) (Claims, error) { return nil, stdErrors.New("bad") },
		})
		require.Error(t, res.Error)
		assert.Equal(t, 401, res.ErrorCode)
	})

	t.Run("missing scope returns 403", func(t *testing.T) {
		ctx := newTestContext("GET", "/test", withHeaders(map[string]string{
			"Authorization": "Bearer token",
		}))
		res := ExtractAndValidateAuth(ctx, ScopeWrite, oauthServiceStub{
			validateFunc: func(string) (Claims, error) { return newMockClaims("alice", ScopeRead), nil },
		})
		require.Error(t, res.Error)
		assert.Equal(t, 403, res.ErrorCode)
	})

	t.Run("success returns context", func(t *testing.T) {
		ctx := newTestContext("GET", "/test", withHeaders(map[string]string{
			"Authorization": "Bearer token",
		}))
		res := ExtractAndValidateAuth(ctx, "", oauthServiceStub{
			validateFunc: func(string) (Claims, error) { return newMockClaims("alice", ScopeRead), nil },
		})
		require.NoError(t, res.Error)
		require.NotNil(t, res.Context)
		assert.Equal(t, "alice", res.Context.Username)
	})
}

func TestAuthHelpers_MultipleScopesAndOptionalAuth(t *testing.T) {
	t.Run("multiple scopes requires one match", func(t *testing.T) {
		ctx := newTestContext("GET", "/test", withHeaders(map[string]string{
			"Authorization": "Bearer token",
		}))
		res := ExtractAndValidateAuthWithMultipleScopes(ctx, []string{AdminRead}, oauthServiceStub{
			validateFunc: func(string) (Claims, error) { return newMockClaims("alice", ScopeRead), nil },
		})
		require.Error(t, res.Error)
		assert.Equal(t, 403, res.ErrorCode)
	})

	t.Run("optional auth with no header returns empty context", func(t *testing.T) {
		ctx := newTestContext("GET", "/test")
		res := ExtractOptionalAuth(ctx, oauthServiceStub{})
		require.NoError(t, res.Error)
		require.NotNil(t, res.Context)
		assert.Equal(t, "", res.Context.Username)
	})

	t.Run("optional auth invalid token returns empty context", func(t *testing.T) {
		ctx := newTestContext("GET", "/test", withHeaders(map[string]string{
			"Authorization": "Token x",
		}))
		res := ExtractOptionalAuth(ctx, oauthServiceStub{})
		require.NoError(t, res.Error)
		require.NotNil(t, res.Context)
		assert.Equal(t, "", res.Context.Username)
	})

	t.Run("optional auth oauth failure returns empty context", func(t *testing.T) {
		ctx := newTestContext("GET", "/test", withHeaders(map[string]string{
			"Authorization": "Bearer token",
		}))
		res := ExtractOptionalAuth(ctx, oauthServiceStub{
			validateFunc: func(string) (Claims, error) { return nil, stdErrors.New("bad") },
		})
		require.NoError(t, res.Error)
		assert.Equal(t, "", res.Context.Username)
	})
}

func TestAuthHelpers_HeaderExtractionAndAccessValidation(t *testing.T) {
	t.Run("ExtractAuthHeader fallbacks", func(t *testing.T) {
		ctx := newTestContext("GET", "/test", func(ctx *apptheory.Context) {
			ctx.Request.Headers = map[string][]string{"Authorization": {"Bearer token"}}
		})
		assert.Equal(t, "Bearer token", ExtractAuthHeader(ctx))
	})

	t.Run("ValidateWriteAccess and ValidateReadAccess", func(t *testing.T) {
		baseErr := stdErrors.New("x")
		assert.ErrorIs(t, ValidateWriteAccess(&AuthenticationResult{Error: baseErr}), baseErr)

		err := ValidateWriteAccess(&AuthenticationResult{Context: &AuthContext{}})
		assert.ErrorIs(t, err, ErrAuthenticationRequired)

		err = ValidateWriteAccess(&AuthenticationResult{Context: &AuthContext{Claims: newMockClaims("alice", ScopeRead)}})
		assert.ErrorIs(t, err, ErrInsufficientScopeWrite)

		err = ValidateReadAccess(&AuthenticationResult{Context: &AuthContext{Claims: newMockClaims("alice", ScopeWrite)}})
		assert.ErrorIs(t, err, ErrInsufficientScopeRead)

		err = ValidateReadAccess(&AuthenticationResult{Context: &AuthContext{}})
		assert.ErrorIs(t, err, ErrAuthenticationRequiredRead)

		assert.NoError(t, ValidateReadAccess(&AuthenticationResult{Context: &AuthContext{Claims: newMockClaims("alice", ScopeRead)}}))
	})

	t.Run("ExtractBearerToken validates prefix and length", func(t *testing.T) {
		_, err := ExtractBearerToken("")
		assert.ErrorIs(t, err, ErrAuthHeaderEmpty)

		_, err = ExtractBearerToken("Token x")
		assert.ErrorIs(t, err, ErrAuthHeaderInvalidPrefix)

		token, err := ExtractBearerToken("Bearer abc")
		require.NoError(t, err)
		assert.Equal(t, "abc", token)
	})
}
