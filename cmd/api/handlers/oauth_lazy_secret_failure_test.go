package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/stretchr/testify/require"
)

func withFailingOAuthSecretResolver(t testing.TB) {
	t.Helper()
	orig := resolveOAuthJWTSecretFn
	resolveOAuthJWTSecretFn = func(*config.Config) (string, error) {
		return "", errors.New("secrets manager unavailable")
	}
	t.Cleanup(func() { resolveOAuthJWTSecretFn = orig })
}

func TestCreateOAuthServiceFailsClosedWhenLazyJWTResolutionFails(t *testing.T) {
	withFailingOAuthSecretResolver(t)

	cfg := round11TestConfig()
	cfg.JWTSecret = ""
	cfg.JWTSecretARN = "arn:aws:secretsmanager:us-east-1:123456789012:secret:jwt"
	h, _, _ := round11NewHandler(t, cfg)

	oauthSvc, err := createOAuthService("", cfg, h.repos, h.logger)
	require.ErrorContains(t, err, "resolve JWT secret")
	require.Nil(t, oauthSvc)
}

func TestOAuthTokenEndpointFailsClosedWhenLazyJWTResolutionFails(t *testing.T) {
	withFailingOAuthSecretResolver(t)

	cfg := round11TestConfig()
	cfg.JWTSecret = ""
	cfg.JWTSecretARN = "arn:aws:secretsmanager:us-east-1:123456789012:secret:jwt"
	h, _, _ := round11NewHandler(t, cfg)

	ctx := round10NewLiftContextWithBodyBytes(
		http.MethodPost,
		"/oauth/token",
		map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		nil,
		[]byte("grant_type=refresh_token&refresh_token=rt&client_id=client"),
	)

	resp := requireStatus(t, http.StatusInternalServerError)(h.HandleOAuthTokenLift(ctx))
	var body models.OAuthErrorResponse
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	require.Equal(t, "server_error", body.Error)
}

func TestAuthenticatedClaimsFailsClosedWhenLazyJWTResolutionFails(t *testing.T) {
	withFailingOAuthSecretResolver(t)

	cfg := round11TestConfig()
	cfg.JWTSecret = ""
	cfg.JWTSecretARN = "arn:aws:secretsmanager:us-east-1:123456789012:secret:jwt"
	h, _, _ := round11NewHandler(t, cfg)

	emptyKeyToken := round11SignAccessToken(t, "", "alice", []string{auth.ScopeRead})
	ctx := round10NewLiftContextWithBodyBytes(
		http.MethodGet,
		"/api/v1/accounts/verify_credentials",
		map[string]string{"Authorization": "Bearer " + emptyKeyToken},
		nil,
		nil,
	)

	claims, err := h.authenticatedClaimsLift(ctx)
	require.Nil(t, claims)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "OAuth service unavailable") || strings.Contains(err.Error(), "secrets manager unavailable"))
}
