package handlers

import (
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
)

func TestValidateClaimsScopesRound13(t *testing.T) {
	err := validateClaimsScopes(nil)
	require.Error(t, err)
	require.True(t, apperrors.HasCode(err, apperrors.CodeUnauthorized))

	require.NoError(t, validateClaimsScopes(&auth.Claims{Scopes: []string{"admin:all", "moderation"}}))

	err = validateClaimsScopes(&auth.Claims{Scopes: []string{"bogus:scope"}})
	require.ErrorIs(t, err, auth.ErrInvalidToken)
}

func TestAuthenticatedClaimsLiftRound13(t *testing.T) {
	cfg := round11TestConfig()
	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	t.Run("uses principal claims directly", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/test", nil, nil, nil)
		require.NoError(t, err)
		ctx.AuthPrincipal = auth.PrincipalFromClaims(&auth.Claims{
			Username: "alice",
			Scopes:   []string{auth.ScopeRead},
			ClientID: "web",
		})

		claims, err := h.authenticatedClaimsLift(ctx)
		require.NoError(t, err)
		require.NotNil(t, claims)
		require.Equal(t, "alice", claims.GetUsername())
		require.Equal(t, "web", claims.ClientID)
	})

	t.Run("rejects invalid scopes from principal claims", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/test", nil, nil, nil)
		require.NoError(t, err)
		ctx.AuthPrincipal = auth.PrincipalFromClaims(&auth.Claims{
			Username: "alice",
			Scopes:   []string{"bogus:scope"},
		})

		claims, err := h.authenticatedClaimsLift(ctx)
		require.Nil(t, claims)
		require.ErrorIs(t, err, auth.ErrInvalidToken)
	})
}

func TestOptionalAuthenticatedClaimsLiftRound13(t *testing.T) {
	cfg := round11TestConfig()
	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	t.Run("returns principal claims when valid", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/test", nil, nil, nil)
		require.NoError(t, err)
		ctx.AuthPrincipal = auth.PrincipalFromClaims(&auth.Claims{
			Username: "alice",
			Scopes:   []string{auth.ScopeRead},
		})

		claims := h.optionalAuthenticatedClaimsLift(ctx)
		require.NotNil(t, claims)
		require.Equal(t, "alice", claims.GetUsername())
	})

	t.Run("returns nil when principal scopes are invalid", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/test", nil, nil, nil)
		require.NoError(t, err)
		ctx.AuthPrincipal = auth.PrincipalFromClaims(&auth.Claims{
			Username: "alice",
			Scopes:   []string{"bogus:scope"},
		})

		require.Nil(t, h.optionalAuthenticatedClaimsLift(ctx))
	})
}

func TestAuthenticatedClaimsWithResponderRound13(t *testing.T) {
	cfg := round11TestConfig()
	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	t.Run("returns principal claims without responders", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/test", nil, nil, nil)
		require.NoError(t, err)
		ctx.AuthPrincipal = auth.PrincipalFromClaims(&auth.Claims{
			Username: "alice",
			Scopes:   []string{auth.ScopeRead},
		})

		claims, resp, err := h.authenticatedClaimsWithResponder(ctx, nil, nil)
		require.NoError(t, err)
		require.NotNil(t, claims)
		require.Nil(t, resp)
		require.Equal(t, "alice", claims.GetUsername())
	})

	t.Run("missing token uses missing responder", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/test", nil, nil, nil)
		require.NoError(t, err)

		missingCalls := 0
		missingResponder := func(*apptheory.Context) (*apptheory.Response, error) {
			missingCalls++
			return &apptheory.Response{Status: 401}, nil
		}

		claims, resp, err := h.authenticatedClaimsWithResponder(ctx, missingResponder, nil)
		require.NoError(t, err)
		require.Nil(t, claims)
		require.NotNil(t, resp)
		require.Equal(t, 401, resp.Status)
		require.Equal(t, 1, missingCalls)
	})

	t.Run("invalid principal uses invalid responder", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/test", nil, nil, nil)
		require.NoError(t, err)
		ctx.AuthPrincipal = auth.PrincipalFromClaims(&auth.Claims{
			Username: "alice",
			Scopes:   []string{"bogus:scope"},
		})

		invalidCalls := 0
		invalidResponder := func(*apptheory.Context) (*apptheory.Response, error) {
			invalidCalls++
			return &apptheory.Response{Status: 403}, nil
		}

		claims, resp, err := h.authenticatedClaimsWithResponder(ctx, nil, invalidResponder)
		require.NoError(t, err)
		require.Nil(t, claims)
		require.NotNil(t, resp)
		require.Equal(t, 403, resp.Status)
		require.Equal(t, 1, invalidCalls)
	})

	t.Run("missing token without responder returns unauthorized", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/test", nil, nil, nil)
		require.NoError(t, err)

		claims, resp, err := h.authenticatedClaimsWithResponder(ctx, nil, nil)
		require.Nil(t, claims)
		require.Nil(t, resp)
		require.Error(t, err)
		require.True(t, apperrors.HasCode(err, apperrors.CodeUnauthorized))
	})

	t.Run("invalid principal without responder returns invalid token", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/test", nil, nil, nil)
		require.NoError(t, err)
		ctx.AuthPrincipal = auth.PrincipalFromClaims(&auth.Claims{
			Username: "alice",
			Scopes:   []string{"bogus:scope"},
		})

		claims, resp, err := h.authenticatedClaimsWithResponder(ctx, nil, nil)
		require.Nil(t, claims)
		require.Nil(t, resp)
		require.ErrorIs(t, err, auth.ErrInvalidToken)
	})
}

func TestAuthenticateWithAnyScopeRound13(t *testing.T) {
	cfg := round11TestConfig()
	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	t.Run("requires at least one scope", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/test", nil, nil, nil)
		require.NoError(t, err)

		claims, err := h.authenticateWithAnyScope(ctx)
		require.Nil(t, claims)
		require.Error(t, err)
		require.True(t, apperrors.HasCode(err, apperrors.CodeInternal))
	})

	t.Run("validates required scope names", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/test", nil, nil, nil)
		require.NoError(t, err)
		ctx.AuthPrincipal = auth.PrincipalFromClaims(&auth.Claims{
			Username: "alice",
			Scopes:   []string{auth.ScopeRead},
		})

		claims, err := h.authenticateWithAnyScope(ctx, "bad")
		require.Nil(t, claims)
		require.Error(t, err)
		require.True(t, apperrors.HasCode(err, apperrors.CodeInternal))
	})

	t.Run("accepts any matching scope", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/test", nil, nil, nil)
		require.NoError(t, err)
		ctx.AuthPrincipal = auth.PrincipalFromClaims(&auth.Claims{
			Username: "alice",
			Scopes:   []string{auth.ScopeRead},
		})

		claims, err := h.authenticateWithAnyScope(ctx, auth.ScopeWrite, auth.ScopeRead)
		require.NoError(t, err)
		require.NotNil(t, claims)
		require.Equal(t, "alice", claims.GetUsername())
	})

	t.Run("returns insufficient scope when nothing matches", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/test", nil, nil, nil)
		require.NoError(t, err)
		ctx.AuthPrincipal = auth.PrincipalFromClaims(&auth.Claims{
			Username: "alice",
			Scopes:   []string{auth.ScopeRead},
		})

		claims, err := h.authenticateWithAnyScope(ctx, auth.ScopeWrite)
		require.Nil(t, claims)
		require.Error(t, err)
		require.True(t, apperrors.HasCode(err, apperrors.CodeInsufficientScope))
	})
}

func TestGetOptionalAuthenticatedUserRound13(t *testing.T) {
	cfg := round11TestConfig()
	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/test", nil, nil, nil)
	require.NoError(t, err)
	ctx.AuthPrincipal = auth.PrincipalFromClaims(&auth.Claims{
		Username: "alice",
		Scopes:   []string{auth.ScopeRead},
	})

	require.Equal(t, "alice", h.getOptionalAuthenticatedUser(ctx))
}

func TestClaimsHaveAnyScopeRound13(t *testing.T) {
	require.False(t, claimsHaveAnyScope(nil, auth.ScopeRead))
	require.True(t, claimsHaveAnyScope(&auth.Claims{Scopes: []string{auth.ScopeRead}}, auth.ScopeWrite, auth.ScopeRead))
	require.False(t, claimsHaveAnyScope(&auth.Claims{Scopes: []string{auth.ScopeRead}}, auth.ScopeWrite))
}
