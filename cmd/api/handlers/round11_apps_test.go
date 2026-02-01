package handlers

import (
	"encoding/base64"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestHandleAppRegistrationLift(t *testing.T) {
	h, _, _ := round11NewHandlerSliceC(t, nil)

	body := map[string]string{
		"client_name":   "Test App",
		"redirect_uris": "https://example.com/callback",
		"scopes":        "read",
		"website":       "https://example.com",
	}

	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/apps", map[string]string{"Content-Type": "application/json"}, nil, body)
	require.NoError(t, err)

	requireStatus(t, http.StatusOK)(h.HandleAppRegistrationLift(ctx))
}

func TestHandleAppVerifyCredentialsLift(t *testing.T) {
	state := &round10QueryState{
		oauthClientsByID: map[string]storagemodels.OAuthClient{
			"client-1": {ClientID: "client-1", ClientSecret: "secret", RedirectURIs: []string{"https://example.com/callback"}},
		},
	}
	h, _, _ := round11NewHandlerSliceC(t, state)

	oauthToken := round11SignTokenWithClientID(t, h.cfg.JWTSecret, "alice", "client-1", []string{auth.ScopeRead}, "sess-1")
	claimsHeader := map[string]string{"Authorization": "Bearer " + oauthToken}
	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/apps/verify_credentials", claimsHeader, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(h.HandleAppVerifyCredentialsLift(ctx))

	basic := base64.StdEncoding.EncodeToString([]byte("client-1:secret"))
	ctxBasic, err := round10NewLiftContext(http.MethodGet, "/api/v1/apps/verify_credentials", map[string]string{"Authorization": "Bearer " + basic}, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(h.HandleAppVerifyCredentialsLift(ctxBasic))
}
