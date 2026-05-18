package handlers

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func mustCreateOAuthService(t testing.TB, jwtSecret string, cfg *config.Config, repos core.RepositoryStorage, logger *zap.Logger) *auth.OAuthService {
	t.Helper()
	oauthSvc, err := createOAuthService(jwtSecret, cfg, repos, logger)
	require.NoError(t, err)
	return oauthSvc
}
