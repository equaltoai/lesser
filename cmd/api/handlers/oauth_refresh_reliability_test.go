package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func oauthRefreshReliabilityClient(clientID string) storagemodels.OAuthClient {
	return storagemodels.OAuthClient{
		ClientID:    clientID,
		Name:        clientID,
		GrantTypes:  []string{auth.GrantTypeAuthorizationCode, auth.GrantTypeRefreshToken},
		ClientClass: auth.ClientClassWeb,
		Scopes:      []string{auth.ScopeRead},
		CreatedAt:   time.Now().UTC().Add(-time.Hour),
	}
}

func oauthRefreshReliabilityDeviceClient() storagemodels.OAuthClient {
	client := oauthRefreshReliabilityClient("client-1")
	client.ClientClass = auth.ClientClassCLI
	client.GrantTypes = []string{oauthDeviceCodeGrantType, auth.GrantTypeRefreshToken}
	return client
}

func oauthRefreshReliabilityToken(token, clientID string, now time.Time) storagemodels.RefreshToken {
	row := storagemodels.RefreshToken{
		Token:      token,
		ClientID:   clientID,
		Username:   "alice",
		Scopes:     []string{auth.ScopeRead},
		CreatedAt:  now.Add(-time.Hour),
		ExpiresAt:  now.Add(time.Hour),
		FamilyID:   "family-1",
		Generation: 1,
		Current:    true,
	}
	if err := row.BeforeCreate(); err != nil {
		panic(err)
	}
	return row
}

func requireOAuthTokenError(t *testing.T, responseStatus int, expectedError string, responseBody []byte) {
	t.Helper()
	var body map[string]string
	require.NoError(t, json.Unmarshal(responseBody, &body))
	require.Equal(t, expectedError, body["error"])
	require.NotEmpty(t, body["error_description"])
	require.NotZero(t, responseStatus)
}

func TestOAuthRefreshTransientStorageFaultReturnsRetryable503(t *testing.T) {
	state := &round10QueryState{
		oauthClientsByID: map[string]storagemodels.OAuthClient{"client-1": oauthRefreshReliabilityClient("client-1")},
		firstErrorByType: map[string]error{"*models.RefreshToken": errors.New("storage throttled")},
	}
	h, _, _ := round11NewHandler(t, state)

	ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=refresh_token&refresh_token=rt-1&client_id=client-1"))
	resp := requireStatus(t, http.StatusServiceUnavailable)(h.HandleOAuthTokenLift(ctx))
	requireOAuthTokenError(t, resp.Status, "temporarily_unavailable", resp.Body)
	require.Equal(t, []string{"1"}, resp.Headers["retry-after"])
}

func TestOAuthRefreshShareReadFaultReturnsRetryable503(t *testing.T) {
	now := time.Now().UTC()
	resource := "https://api.example.com/mcp/agent-one"
	token := oauthRefreshReliabilityToken("rt-share", "client-1", now)
	token.Username = "agent-one"
	token.PrincipalUsername = "grantee"
	token.Resource = resource
	state := &round10QueryState{
		oauthClientsByID:     map[string]storagemodels.OAuthClient{"client-1": oauthRefreshReliabilityClient("client-1")},
		refreshTokensByToken: map[string]storagemodels.RefreshToken{"rt-share": token},
		usersByUsername: map[string]storagemodels.User{
			"agent-one": {Username: "agent-one", IsAgent: true, AgentOwner: "@owner"},
		},
	}
	registry := &RegistryStub{AgentSharesSvc: &actAsShareServiceStub{isActiveFunc: func(context.Context, string, string) (bool, error) {
		return false, errors.New("share store unavailable")
	}}}
	h, _, _ := round11NewHandler(t, state, registry)

	params := url.Values{"grant_type": {auth.GrantTypeRefreshToken}, "refresh_token": {token.Token}, "client_id": {token.ClientID}, "resource": {resource}}
	ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte(params.Encode()))
	resp := requireStatus(t, http.StatusServiceUnavailable)(h.HandleOAuthTokenLift(ctx))
	requireOAuthTokenError(t, resp.Status, "temporarily_unavailable", resp.Body)
	require.Equal(t, []string{"1"}, resp.Headers["retry-after"])
}

func TestOAuthRefreshAuthoritativeGrantFailuresStayTerminal(t *testing.T) {
	now := time.Now().UTC()
	baseClient := oauthRefreshReliabilityClient("client-1")
	otherClient := oauthRefreshReliabilityClient("client-2")

	tests := []struct {
		name      string
		token     string
		clientID  string
		resource  string
		configure func(*round10QueryState)
	}{
		{name: "missing", token: "missing", clientID: "client-1", configure: func(state *round10QueryState) {
			state.notFoundPKs = map[string]bool{"REFRESHTOKEN#missing": true}
		}},
		{name: "expired", token: "expired", clientID: "client-1", configure: func(state *round10QueryState) {
			row := oauthRefreshReliabilityToken("expired", "client-1", now)
			row.ExpiresAt = now.Add(-time.Second)
			state.refreshTokensByToken = map[string]storagemodels.RefreshToken{row.Token: row}
		}},
		{name: "revoked", token: "revoked", clientID: "client-1", configure: func(state *round10QueryState) {
			row := oauthRefreshReliabilityToken("revoked", "client-1", now)
			row.Current = false
			row.Revoked = true
			row.RevokedAt = now.Add(-time.Minute)
			row.RevokedReason = "rotated"
			state.refreshTokensByToken = map[string]storagemodels.RefreshToken{row.Token: row}
		}},
		{name: "wrong_client", token: "wrong-client", clientID: "client-2", configure: func(state *round10QueryState) {
			row := oauthRefreshReliabilityToken("wrong-client", "client-1", now)
			state.refreshTokensByToken = map[string]storagemodels.RefreshToken{row.Token: row}
		}},
		{name: "wrong_resource", token: "wrong-resource", clientID: "client-1", resource: "https://api.example.com/mcp/other", configure: func(state *round10QueryState) {
			row := oauthRefreshReliabilityToken("wrong-resource", "client-1", now)
			row.Resource = "https://api.example.com/mcp/alice"
			state.refreshTokensByToken = map[string]storagemodels.RefreshToken{row.Token: row}
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := &round10QueryState{oauthClientsByID: map[string]storagemodels.OAuthClient{
				"client-1": baseClient,
				"client-2": otherClient,
			}}
			tc.configure(state)
			h, _, _ := round11NewHandler(t, state)
			params := url.Values{"grant_type": {auth.GrantTypeRefreshToken}, "refresh_token": {tc.token}, "client_id": {tc.clientID}}
			if tc.resource != "" {
				params.Set("resource", tc.resource)
			}
			ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte(params.Encode()))
			resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(ctx))
			requireOAuthTokenError(t, resp.Status, "invalid_grant", resp.Body)
		})
	}
}

func TestOAuthRefreshClientLookupDistinguishesAuthorityFromInfrastructure(t *testing.T) {
	t.Run("authoritative missing client", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, &round10QueryState{notFoundPKs: map[string]bool{"OAUTH_CLIENT#missing": true}})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=refresh_token&refresh_token=rt&client_id=missing"))
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(ctx))
		requireOAuthTokenError(t, resp.Status, "invalid_client", resp.Body)
	})

	t.Run("client store fault", func(t *testing.T) {
		state := &round10QueryState{firstErrorByType: map[string]error{"*models.OAuthClient": errors.New("client store unavailable")}}
		h, _, _ := round11NewHandler(t, state)
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=refresh_token&refresh_token=rt&client_id=client-1"))
		resp := requireStatus(t, http.StatusServiceUnavailable)(h.HandleOAuthTokenLift(ctx))
		requireOAuthTokenError(t, resp.Status, "temporarily_unavailable", resp.Body)
		require.Equal(t, []string{"1"}, resp.Headers["retry-after"])
	})
}

func TestOAuthStandardRefreshRotationCASPreventsFamilyFork(t *testing.T) {
	now := time.Now().UTC()
	row := oauthRefreshReliabilityToken("predecessor", "client-1", now)
	state := &round10QueryState{refreshTokensByToken: map[string]storagemodels.RefreshToken{row.Token: row}}
	_, repos, _ := round11NewHandler(t, state)
	account := repos.Account()

	firstPredecessor := refreshTokenStorageFromTestModel(row)
	secondPredecessor := refreshTokenStorageFromTestModel(row)
	firstSuccessor := oauthRefreshReliabilitySuccessor("successor-1", firstPredecessor, now)
	secondSuccessor := oauthRefreshReliabilitySuccessor("successor-2", secondPredecessor, now)

	require.NoError(t, account.RotateRefreshToken(context.Background(), firstPredecessor, firstSuccessor, now))
	require.Error(t, account.RotateRefreshToken(context.Background(), secondPredecessor, secondSuccessor, now))

	active := 0
	for _, candidate := range state.refreshTokensByToken {
		if candidate.FamilyID == row.FamilyID && candidate.Current && !candidate.Revoked {
			active++
		}
	}
	require.Equal(t, 1, active)
	require.NotContains(t, state.refreshTokensByToken, secondSuccessor.Token)
}

func TestOAuthStandardRefreshGraceRescueRedeemsOnceThenTerminal(t *testing.T) {
	now := time.Now().UTC()
	stale := oauthRefreshReliabilityToken("stale", "client-1", now)
	stale.Current = false
	stale.Revoked = true
	stale.RevokedAt = now.Add(-time.Second)
	stale.RevokedReason = "rotated"
	active := oauthRefreshReliabilityToken("active", "client-1", now)
	active.Generation = 2
	require.NoError(t, active.UpdateKeys())
	state := &round10QueryState{
		oauthClientsByID:     map[string]storagemodels.OAuthClient{"client-1": oauthRefreshReliabilityClient("client-1")},
		refreshTokensByToken: map[string]storagemodels.RefreshToken{stale.Token: stale, active.Token: active},
		disableAuditRepo:     true,
	}
	h, _, _ := round11NewHandler(t, state)
	request := []byte("grant_type=refresh_token&refresh_token=stale&client_id=client-1")

	first := requireStatus(t, http.StatusOK)(h.HandleOAuthTokenLift(round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, request)))
	var issued apimodels.OAuthTokenResponse
	require.NoError(t, json.Unmarshal(first.Body, &issued))
	require.NotEmpty(t, issued.RefreshToken)
	require.False(t, state.refreshTokensByToken["stale"].RetryRedeemedAt.IsZero())
	require.True(t, state.refreshTokensByToken["active"].Revoked)
	require.Equal(t, 3, state.refreshTokensByToken[issued.RefreshToken].Generation)

	second := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, request)))
	requireOAuthTokenError(t, second.Status, "invalid_grant", second.Body)
}

func TestOAuthLegacyRefreshRowAdoptsLineage(t *testing.T) {
	now := time.Now().UTC()
	legacy := oauthRefreshReliabilityToken("legacy", "client-1", now)
	legacy.FamilyID = ""
	legacy.Generation = 0
	legacy.Current = false
	require.NoError(t, legacy.UpdateKeys())
	state := &round10QueryState{
		oauthClientsByID:     map[string]storagemodels.OAuthClient{"client-1": oauthRefreshReliabilityClient("client-1")},
		refreshTokensByToken: map[string]storagemodels.RefreshToken{legacy.Token: legacy},
		disableAuditRepo:     true,
	}
	h, _, _ := round11NewHandler(t, state)
	ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=refresh_token&refresh_token=legacy&client_id=client-1"))
	resp := requireStatus(t, http.StatusOK)(h.HandleOAuthTokenLift(ctx))
	var issued apimodels.OAuthTokenResponse
	require.NoError(t, json.Unmarshal(resp.Body, &issued))

	adopted := state.refreshTokensByToken[legacy.Token]
	successor := state.refreshTokensByToken[issued.RefreshToken]
	require.NotEmpty(t, adopted.FamilyID)
	require.Equal(t, adopted.FamilyID, successor.FamilyID)
	require.Equal(t, 1, adopted.Generation)
	require.Equal(t, 2, successor.Generation)
	require.True(t, adopted.Revoked)
	require.True(t, successor.Current)
}

func TestOAuthDeviceIssuanceFailureReturnsNoUnpersistedRefreshToken(t *testing.T) {
	now := time.Now().UTC()
	cfg := round11TestConfig()
	cfg.AllowDeviceFlow = true
	deviceCode := "approved-storage-failure"
	hash := oauthDeviceCodeHash(deviceCode)
	state := &round10QueryState{
		oauthClientsByID: map[string]storagemodels.OAuthClient{"client-1": oauthRefreshReliabilityDeviceClient()},
		oauthDeviceSessionsByHash: map[string]storagemodels.OAuthDeviceSession{hash: {
			DeviceCodeHash: hash, UserCode: "ABCD-EFGH", ClientID: "client-1",
			Scopes: []string{auth.ScopeRead}, Status: oauthDeviceSessionStatusApproved,
			ApprovedUsername: "alice", IntervalSeconds: oauthDevicePollIntervalSeconds,
			CreatedAt: now.Add(-time.Minute), UpdatedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Minute),
		}},
		transactionErrorOnce: errors.New("transaction unavailable"),
		disableAuditRepo:     true,
	}
	h, _, _ := round11NewHandler(t, cfg, state)
	ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type="+oauthDeviceCodeGrantType+"&device_code="+deviceCode+"&client_id=client-1"))
	resp := requireStatus(t, http.StatusServiceUnavailable)(h.HandleOAuthTokenLift(ctx))
	requireOAuthTokenError(t, resp.Status, "temporarily_unavailable", resp.Body)
	require.Equal(t, oauthDeviceSessionStatusApproved, state.oauthDeviceSessionsByHash[hash].Status)
	require.Empty(t, state.refreshTokensByToken)
}

func refreshTokenStorageFromTestModel(row storagemodels.RefreshToken) *storage.RefreshToken {
	return &storage.RefreshToken{
		Token: row.Token, ClientID: row.ClientID, Username: row.Username, Scopes: row.Scopes,
		CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt, FamilyID: row.FamilyID,
		Generation: row.Generation, Current: row.Current, Revoked: row.Revoked, Version: row.Version,
	}
}

func oauthRefreshReliabilitySuccessor(token string, predecessor *storage.RefreshToken, now time.Time) *storage.RefreshToken {
	return &storage.RefreshToken{
		Token: token, ClientID: predecessor.ClientID, Username: predecessor.Username, Scopes: predecessor.Scopes,
		CreatedAt: now, ExpiresAt: now.Add(time.Hour), FamilyID: predecessor.FamilyID,
		Generation: predecessor.Generation + 1, Current: true,
	}
}
