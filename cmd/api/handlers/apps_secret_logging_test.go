package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestCreateOAuthClientAndRespond_DoesNotLogClientSecret(t *testing.T) {
	t.Parallel()

	cfg := round11TestConfig()
	harness := round10NewDynamoHarness(t, &round10QueryState{})

	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(core)

	accountRepo := repositories.NewAccountRepository(harness.db, cfg.DynamoTableName, cfg.Domain, logger)
	pushRepo := repositories.NewPushSubscriptionRepository(harness.db, cfg.DynamoTableName, logger, nil, nil, "", "mailto:push@example.com")

	repos := &MockRepositoryStorage{}
	repos.On("Account").Return(accountRepo).Maybe()
	repos.On("PushSubscription").Return(pushRepo).Maybe()

	handler := &Handler{cfg: cfg, repos: repos, logger: logger}

	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/apps", nil, nil, nil)
	require.NoError(t, err)

	req := &apimodels.AppRegistrationRequest{
		ClientName:   "Test App",
		RedirectURIs: "https://example.com/callback",
		Scopes:       "read",
		Website:      "https://example.com",
	}

	resp := requireStatus(t, http.StatusOK)(handler.createOAuthClientAndRespond(ctx, req, []string{"https://example.com/callback"}))

	var registration apimodels.AppRegistrationResponse
	require.NoError(t, json.Unmarshal(resp.Body, &registration))
	require.NotEmpty(t, registration.ClientSecret)

	for _, entry := range observed.All() {
		require.False(t, strings.Contains(entry.Message, registration.ClientSecret), "log message should not contain client secret")
		for _, v := range entry.ContextMap() {
			if s, ok := v.(string); ok {
				require.False(t, strings.Contains(s, registration.ClientSecret), "log field should not contain client secret")
			}
		}
	}
}

func TestHandleAppRotateSecretLift_DoesNotLogClientSecret(t *testing.T) {
	t.Parallel()

	cfg := round11TestConfig()
	state := &round10QueryState{
		oauthClientsByID: map[string]storagemodels.OAuthClient{
			"client-1": {
				ClientID:      "client-1",
				ClientSecret:  "secret",
				Name:          "Agent Connector",
				RedirectURIs:  []string{"https://example.com/callback"},
				Scopes:        []string{auth.ScopeRead, auth.ScopeWrite},
				ClientClass:   auth.ClientClassAgent,
				AgentUsername: "agent1",
				OwnerID:       "owner",
				Confidential:  true,
				CreatedAt:     time.Now().Add(-24 * time.Hour),
			},
		},
		usersByUsername: map[string]storagemodels.User{
			"agent1": {
				Username:   "agent1",
				IsAgent:    true,
				AgentOwner: "@owner",
			},
		},
	}

	harness := round10NewDynamoHarness(t, state)
	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(core)

	accountRepo := repositories.NewAccountRepository(harness.db, cfg.DynamoTableName, cfg.Domain, logger)
	pushRepo := repositories.NewPushSubscriptionRepository(harness.db, cfg.DynamoTableName, logger, nil, nil, "", "mailto:push@example.com")
	repos := &MockRepositoryStorage{}
	repos.On("Account").Return(accountRepo).Maybe()
	repos.On("PushSubscription").Return(pushRepo).Maybe()
	repos.On("Audit").Return(repositories.NewAuditRepository(harness.db, cfg.DynamoTableName, logger, nil)).Maybe()

	handler := &Handler{cfg: cfg, repos: repos, logger: logger}
	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/apps/client-1/rotate_secret", map[string]string{
		"Authorization": "Bearer " + round11SignAccessToken(t, cfg.JWTSecret, "owner", []string{auth.ScopeWrite}),
	}, nil, nil)
	require.NoError(t, err)
	ctx.Params["id"] = "client-1"

	resp := requireStatus(t, http.StatusOK)(handler.HandleAppRotateSecretLift(ctx))
	var rotation apimodels.AppSecretRotationResponse
	require.NoError(t, json.Unmarshal(resp.Body, &rotation))
	require.NotEmpty(t, rotation.ClientSecret)

	oauthSvc := auth.NewOAuthService(cfg.JWTSecret, cfg, repos, nil)
	require.NoError(t, oauthSvc.ValidateClient(context.Background(), "client-1", rotation.ClientSecret))

	for _, entry := range observed.All() {
		require.False(t, strings.Contains(entry.Message, rotation.ClientSecret), "log message should not contain rotated client secret")
		for _, v := range entry.ContextMap() {
			if s, ok := v.(string); ok {
				require.False(t, strings.Contains(s, rotation.ClientSecret), "log field should not contain rotated client secret")
			}
		}
	}
}
