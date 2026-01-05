package lift

import (
	"net/http"
	"strings"
	"testing"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
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

	require.NoError(t, handler.createOAuthClientAndRespond(ctx, req, []string{"https://example.com/callback"}))
	resp := ctx.Response.Body.(apimodels.AppRegistrationResponse)
	require.NotEmpty(t, resp.ClientSecret)

	for _, entry := range observed.All() {
		require.False(t, strings.Contains(entry.Message, resp.ClientSecret), "log message should not contain client secret")
		for _, v := range entry.ContextMap() {
			if s, ok := v.(string); ok {
				require.False(t, strings.Contains(s, resp.ClientSecret), "log field should not contain client secret")
			}
		}
	}
}
