package lift

import (
	"context"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

type oauthSessionRepoStub struct {
	createErr error
}

func (s oauthSessionRepoStub) CreateOAuthSession(ctx context.Context, session *storagemodels.OAuthAuthSession) error {
	if session != nil && session.SessionID == "" {
		session.SessionID = "session-1"
	}
	return s.createErr
}

func TestOAuthConsentHandlers_Round11(t *testing.T) {
	origFactory := newOAuthSessionRepository
	newOAuthSessionRepository = func(_ *Handler) oauthSessionRepository {
		return oauthSessionRepoStub{}
	}
	t.Cleanup(func() {
		newOAuthSessionRepository = origFactory
	})

	cfg := round10TestConfig()
	state := &round10QueryState{
		oauthStates: map[string]storagemodels.OAuthState{
			"state-1": {
				State:       "state-1",
				ClientID:    "client-1",
				Username:    "alice",
				RedirectURI: "https://client.example/callback",
				Scopes:      []string{"read"},
			},
			"state-2": {
				State:       "state-2",
				ClientID:    "client-2",
				Username:    "alice",
				RedirectURI: "https://client.example/callback",
				Scopes:      []string{"read", "write"},
			},
		},
	}
	handler, _, _ := round11NewHandler(t, cfg, state)

	ctxDeny := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/consent", nil, nil, []byte("state=state-1&action=deny"))
	require.NoError(t, handler.HandleOAuthConsentLift(ctxDeny))
	require.Equal(t, http.StatusOK, ctxDeny.Response.StatusCode)

	ctxApprove := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/consent", nil, nil, []byte("state=state-2&action=approve"))
	require.NoError(t, handler.HandleOAuthConsentLift(ctxApprove))
	require.Equal(t, http.StatusOK, ctxApprove.Response.StatusCode)

	authRequest := `{"client_id":"client-1","redirect_uri":"https://client.example/callback","state":"xyz","code_challenge":"abc","code_challenge_method":"plain","scope":"read write"}`
	ctxLogin, err := round10NewLiftContext(http.MethodGet, "/oauth/login", nil, map[string]string{
		"auth_request": authRequest,
		"return_to":    "/oauth/authorize",
	}, nil)
	require.NoError(t, err)
	ctxLogin.Context = context.WithValue(ctxLogin.Context, common.ContextKeyClaims, &auth.Claims{Username: "alice"})
	require.NoError(t, handler.HandleOAuthLoginLift(ctxLogin))
	require.Equal(t, http.StatusOK, ctxLogin.Response.StatusCode)
}
