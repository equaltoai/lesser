package handlers

import (
	"net/http"
	"testing"

	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestOAuthConsentHandlers_Round11(t *testing.T) {
	cfg := round10TestConfig()
	t.Run("deny clears request and returns callback error response", func(t *testing.T) {
		state := &round10QueryState{
			oauthStates: map[string]storagemodels.OAuthState{
				"state-1": {
					State:       "state-1",
					ClientID:    "client-1",
					Username:    "alice",
					RedirectURI: "https://client.example/callback",
					Scopes:      []string{"read"},
				},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctxDeny := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/consent", nil, nil, []byte("state=state-1&action=deny"))
		requireStatus(t, http.StatusOK)(handler.HandleOAuthConsentLift(ctxDeny))
	})

	t.Run("approve uses stored oauth state resource without resource form field", func(t *testing.T) {
		state := &round10QueryState{
			oauthStates: map[string]storagemodels.OAuthState{
				"state-2": {
					State:       "state-2",
					ClientID:    "client-2",
					Username:    "alice",
					RedirectURI: "https://client.example/callback",
					Resource:    "https://mcp.example/resource",
					Scopes:      []string{"read", "write"},
				},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctxApprove := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/consent", nil, nil, []byte("state=state-2&action=approve"))
		requireStatus(t, http.StatusOK)(handler.HandleOAuthConsentLift(ctxApprove))
		require.Len(t, state.authorizationCodesByCode, 1)
		for _, authCode := range state.authorizationCodesByCode {
			require.Equal(t, "https://mcp.example/resource", authCode.Resource)
		}
	})

	t.Run("oauth login redirects to hosted auth ui", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		authRequest := `{"client_id":"client-1","redirect_uri":"https://client.example/callback","state":"xyz","code_challenge":"abc","code_challenge_method":"plain","scope":"read write"}`
		ctxLogin, err := round10NewLiftContext(http.MethodGet, "/oauth/login", nil, map[string]string{
			"auth_request": authRequest,
			"return_to":    "/oauth/authorize",
		}, nil)
		require.NoError(t, err)
		respLogin := requireStatus(t, http.StatusFound)(handler.HandleOAuthLoginLift(ctxLogin))
		require.Contains(t, firstStringValue(respLogin.Headers, "location"), "https://example.com/auth/login")
	})
}
