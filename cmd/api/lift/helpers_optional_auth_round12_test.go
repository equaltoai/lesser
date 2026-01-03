package lift

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestHandler_authenticateUserOptional(t *testing.T) {
	cfg := &config.Config{
		Domain:    "example.com",
		JWTSecret: "test-secret",
	}

	h := &Handler{
		cfg:    cfg,
		logger: zap.NewNop(),
	}

	t.Run("no token returns empty username and nil error", func(t *testing.T) {
		ctx, err := round10NewLiftContext("GET", "/api/v1/optional-auth", nil, nil, nil)
		require.NoError(t, err)

		username, err := h.authenticateUserOptional(ctx, nil)
		require.NoError(t, err)
		require.Empty(t, username)
	})

	t.Run("invalid token returns unauthorized", func(t *testing.T) {
		ctx, err := round10NewLiftContext("GET", "/api/v1/optional-auth", map[string]string{
			"Authorization": "Bearer not-a-real-token",
		}, nil, nil)
		require.NoError(t, err)

		username, err := h.authenticateUserOptional(ctx, nil)
		require.Error(t, err)
		require.Empty(t, username)
	})

	t.Run("valid token returns username", func(t *testing.T) {
		oauthSvc := createOAuthService(cfg.JWTSecret, cfg, nil, h.logger)
		accessToken, _, err := oauthSvc.GenerateTokens(context.Background(), "alice", "client", "1.2.3.4", []string{auth.ScopeRead})
		require.NoError(t, err)

		ctx, err := round10NewLiftContext("GET", "/api/v1/optional-auth", map[string]string{
			"Authorization": "Bearer " + accessToken,
		}, nil, nil)
		require.NoError(t, err)

		username, err := h.authenticateUserOptional(ctx, nil)
		require.NoError(t, err)
		require.Equal(t, "alice", username)
	})

	t.Run("valid token without required scope returns insufficient scope", func(t *testing.T) {
		oauthSvc := createOAuthService(cfg.JWTSecret, cfg, nil, h.logger)
		accessToken, _, err := oauthSvc.GenerateTokens(context.Background(), "alice", "client", "1.2.3.4", []string{auth.ScopeRead})
		require.NoError(t, err)

		ctx, err := round10NewLiftContext("GET", "/api/v1/optional-auth", map[string]string{
			"Authorization": "Bearer " + accessToken,
		}, nil, nil)
		require.NoError(t, err)

		username, err := h.authenticateUserOptional(ctx, []string{auth.ScopeWrite})
		require.Error(t, err)
		require.Empty(t, username)
	})
}

func TestHandler_authenticateWithClaims(t *testing.T) {
	cfg := &config.Config{
		Domain:    "example.com",
		JWTSecret: "test-secret",
	}

	h := &Handler{
		cfg:    cfg,
		logger: zap.NewNop(),
	}

	t.Run("missing token returns unauthorized", func(t *testing.T) {
		ctx, err := round10NewLiftContext("GET", "/api/v1/with-claims", nil, nil, nil)
		require.NoError(t, err)

		claims, err := h.authenticateWithClaims(ctx, nil)
		require.Error(t, err)
		require.Nil(t, claims)
	})

	t.Run("valid token returns claims", func(t *testing.T) {
		oauthSvc := createOAuthService(cfg.JWTSecret, cfg, nil, h.logger)
		accessToken, _, err := oauthSvc.GenerateTokens(context.Background(), "alice", "client", "1.2.3.4", []string{auth.ScopeRead, auth.ScopeWrite})
		require.NoError(t, err)

		ctx, err := round10NewLiftContext("GET", "/api/v1/with-claims", map[string]string{
			"Authorization": "Bearer " + accessToken,
		}, nil, nil)
		require.NoError(t, err)

		claims, err := h.authenticateWithClaims(ctx, []string{auth.ScopeWrite})
		require.NoError(t, err)
		require.NotNil(t, claims)
		require.Equal(t, "alice", claims.Username)
	})

	t.Run("valid token without required scope returns insufficient scope", func(t *testing.T) {
		oauthSvc := createOAuthService(cfg.JWTSecret, cfg, nil, h.logger)
		accessToken, _, err := oauthSvc.GenerateTokens(context.Background(), "alice", "client", "1.2.3.4", []string{auth.ScopeRead})
		require.NoError(t, err)

		ctx, err := round10NewLiftContext("GET", "/api/v1/with-claims", map[string]string{
			"Authorization": "Bearer " + accessToken,
		}, nil, nil)
		require.NoError(t, err)

		claims, err := h.authenticateWithClaims(ctx, []string{auth.ScopeWrite})
		require.Error(t, err)
		require.Nil(t, claims)
	})
}

