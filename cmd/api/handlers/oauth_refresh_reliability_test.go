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
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
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

func TestOAuthInstancePlaneRefreshAuthorityDistinguishesStorageFault(t *testing.T) {
	now := time.Now().UTC()
	resource := oauthInstanceTestResource(oauthInstanceSurfacePtah)
	client := oauthRefreshReliabilityClient("client-1")
	client.ClientClass = auth.ClientClassCLI

	t.Run("direct storage fault is retryable without revocation", func(t *testing.T) {
		token := oauthRefreshReliabilityToken("rt-instance-fault", client.ClientID, now)
		token.Username = "admin"
		token.PrincipalUsername = "admin"
		token.Resource = resource
		token.ClientClass = auth.ClientClassOperator
		require.NoError(t, token.UpdateKeys())
		state := &round10QueryState{
			oauthClientsByID:     map[string]storagemodels.OAuthClient{client.ClientID: client},
			refreshTokensByToken: map[string]storagemodels.RefreshToken{token.Token: token},
			usersByUsername:      map[string]storagemodels.User{},
			firstErrorByType:     map[string]error{"*repositories.userCoreProjection": dynamormerrors.ErrTableNotFound},
			disableAuditRepo:     true,
		}
		h, _, _ := round11NewHandler(t, state)

		params := url.Values{"grant_type": {auth.GrantTypeRefreshToken}, "refresh_token": {token.Token}, "client_id": {client.ClientID}}
		resp := requireStatus(t, http.StatusServiceUnavailable)(h.HandleOAuthTokenLift(round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte(params.Encode()))))
		requireOAuthTokenError(t, resp.Status, "temporarily_unavailable", resp.Body)
		require.Equal(t, []string{"1"}, resp.Headers["retry-after"])
		require.False(t, state.refreshTokensByToken[token.Token].Revoked)
	})

	t.Run("resource owner resolution fault is retryable without revocation", func(t *testing.T) {
		token := oauthRefreshReliabilityToken("rt-instance-owner-fault", client.ClientID, now)
		token.Username = "member"
		token.PrincipalUsername = "member"
		token.Resource = resource
		token.ClientClass = auth.ClientClassCLI
		require.NoError(t, token.UpdateKeys())
		state := &round10QueryState{
			oauthClientsByID:     map[string]storagemodels.OAuthClient{client.ClientID: client},
			refreshTokensByToken: map[string]storagemodels.RefreshToken{token.Token: token},
			usersByUsername: map[string]storagemodels.User{
				"member": {Username: "member", Approved: true, Role: "user"},
			},
			firstErrorByType: map[string]error{"*repositories.userCoreProjection": errors.New("resource owner read throttled")},
			disableAuditRepo: true,
		}
		h, _, _ := round11NewHandler(t, state)

		params := url.Values{"grant_type": {auth.GrantTypeRefreshToken}, "refresh_token": {token.Token}, "client_id": {client.ClientID}}
		resp := requireStatus(t, http.StatusServiceUnavailable)(h.HandleOAuthTokenLift(round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte(params.Encode()))))
		requireOAuthTokenError(t, resp.Status, "temporarily_unavailable", resp.Body)
		require.False(t, state.refreshTokensByToken[token.Token].Revoked)
	})

	t.Run("rescue storage fault is retryable without revocation", func(t *testing.T) {
		stale := oauthRefreshReliabilityToken("rt-instance-stale", client.ClientID, now)
		stale.Username = "admin"
		stale.PrincipalUsername = "admin"
		stale.Resource = resource
		stale.ClientClass = auth.ClientClassOperator
		stale.Current = false
		stale.Revoked = true
		stale.RevokedAt = now.Add(-time.Second)
		stale.RevokedReason = "rotated"
		require.NoError(t, stale.UpdateKeys())
		active := stale
		active.Token = "rt-instance-active"
		active.Generation = 2
		active.Current = true
		active.Revoked = false
		active.RevokedAt = time.Time{}
		active.RevokedReason = ""
		require.NoError(t, active.UpdateKeys())
		state := &round10QueryState{
			oauthClientsByID: map[string]storagemodels.OAuthClient{client.ClientID: client},
			refreshTokensByToken: map[string]storagemodels.RefreshToken{
				stale.Token:  stale,
				active.Token: active,
			},
			usersByUsername: map[string]storagemodels.User{
				"admin": {Username: "admin", Approved: true, Role: "admin"},
			},
			firstErrorByType: map[string]error{"*repositories.userCoreProjection": errors.New("account transport unavailable")},
			disableAuditRepo: true,
		}
		h, _, _ := round11NewHandler(t, state)

		params := url.Values{"grant_type": {auth.GrantTypeRefreshToken}, "refresh_token": {stale.Token}, "client_id": {client.ClientID}}
		resp := requireStatus(t, http.StatusServiceUnavailable)(h.HandleOAuthTokenLift(round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte(params.Encode()))))
		requireOAuthTokenError(t, resp.Status, "temporarily_unavailable", resp.Body)
		require.Equal(t, []string{"1"}, resp.Headers["retry-after"])
		require.Equal(t, "rotated", state.refreshTokensByToken[stale.Token].RevokedReason)
		require.False(t, state.refreshTokensByToken[active.Token].Revoked)
	})

	t.Run("authoritative non operator is terminal", func(t *testing.T) {
		token := oauthRefreshReliabilityToken("rt-instance-member", client.ClientID, now)
		token.Username = "member"
		token.PrincipalUsername = "member"
		token.Resource = resource
		token.ClientClass = auth.ClientClassOperator
		require.NoError(t, token.UpdateKeys())
		state := &round10QueryState{
			oauthClientsByID:     map[string]storagemodels.OAuthClient{client.ClientID: client},
			refreshTokensByToken: map[string]storagemodels.RefreshToken{token.Token: token},
			usersByUsername: map[string]storagemodels.User{
				"member": {Username: "member", Approved: true, Role: "user"},
			},
			disableAuditRepo: true,
		}
		h, _, _ := round11NewHandler(t, state)

		params := url.Values{"grant_type": {auth.GrantTypeRefreshToken}, "refresh_token": {token.Token}, "client_id": {client.ClientID}}
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte(params.Encode()))))
		requireOAuthTokenError(t, resp.Status, "invalid_grant", resp.Body)
	})

	t.Run("deleted principal is terminal without revocation", func(t *testing.T) {
		token := oauthRefreshReliabilityToken("rt-instance-deleted", client.ClientID, now)
		token.Username = "deleted-admin"
		token.PrincipalUsername = "deleted-admin"
		token.Resource = resource
		token.ClientClass = auth.ClientClassOperator
		require.NoError(t, token.UpdateKeys())
		state := &round10QueryState{
			oauthClientsByID: map[string]storagemodels.OAuthClient{client.ClientID: client},
			refreshTokensByToken: map[string]storagemodels.RefreshToken{
				token.Token: token,
			},
			usersByUsername:  map[string]storagemodels.User{},
			notFoundPKs:      map[string]bool{"USER#deleted-admin": true},
			disableAuditRepo: true,
		}
		h, _, _ := round11NewHandler(t, state)

		params := url.Values{"grant_type": {auth.GrantTypeRefreshToken}, "refresh_token": {token.Token}, "client_id": {client.ClientID}}
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte(params.Encode()))))
		requireOAuthTokenError(t, resp.Status, "invalid_grant", resp.Body)
		require.Empty(t, resp.Headers["retry-after"])
		stored := state.refreshTokensByToken[token.Token]
		require.False(t, stored.Revoked)
		require.True(t, stored.Current)
	})
}

func TestOAuthInstancePlaneAuthorizationCodeAuthorityDistinguishesStorageFault(t *testing.T) {
	resource := oauthInstanceTestResource(oauthInstanceSurfacePtah)
	client := oauthRefreshReliabilityClient("client-1")
	client.ClientClass = auth.ClientClassCLI
	client.RedirectURIs = []string{"https://client.example/callback"}
	authCode := storagemodels.AuthorizationCode{
		Code:              "instance-code",
		ClientID:          client.ClientID,
		RedirectURI:       client.RedirectURIs[0],
		Resource:          resource,
		Username:          "admin",
		PrincipalUsername: "admin",
		ExpiresAt:         time.Now().UTC().Add(time.Minute),
		Scopes:            []string{auth.ScopeRead},
	}
	requestBody := func() []byte {
		return []byte(url.Values{
			"grant_type":   {auth.GrantTypeAuthorizationCode},
			"code":         {authCode.Code},
			"client_id":    {client.ClientID},
			"redirect_uri": {authCode.RedirectURI},
			"resource":     {resource},
		}.Encode())
	}

	t.Run("deleted principal is terminal invalid target", func(t *testing.T) {
		state := &round10QueryState{
			oauthClientsByID:         map[string]storagemodels.OAuthClient{client.ClientID: client},
			authorizationCodesByCode: map[string]storagemodels.AuthorizationCode{authCode.Code: authCode},
			usersByUsername:          map[string]storagemodels.User{},
			notFoundPKs:              map[string]bool{"USER#admin": true},
			disableAuditRepo:         true,
		}
		h, _, _ := round11NewHandler(t, state)

		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, requestBody())))
		requireOAuthTokenError(t, resp.Status, "invalid_target", resp.Body)
		require.Empty(t, resp.Headers["retry-after"])
		require.Contains(t, state.authorizationCodesByCode, authCode.Code)
		require.Empty(t, state.refreshTokensByToken)
	})

	t.Run("storage fault is retryable and preserves grant", func(t *testing.T) {
		state := &round10QueryState{
			oauthClientsByID:         map[string]storagemodels.OAuthClient{client.ClientID: client},
			authorizationCodesByCode: map[string]storagemodels.AuthorizationCode{authCode.Code: authCode},
			usersByUsername:          map[string]storagemodels.User{},
			firstErrorByType:         map[string]error{"*repositories.userCoreProjection": dynamormerrors.ErrIndexNotFound},
			disableAuditRepo:         true,
		}
		h, _, _ := round11NewHandler(t, state)

		resp := requireStatus(t, http.StatusServiceUnavailable)(h.HandleOAuthTokenLift(round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, requestBody())))
		requireOAuthTokenError(t, resp.Status, "temporarily_unavailable", resp.Body)
		require.Equal(t, []string{"1"}, resp.Headers["retry-after"])
		require.Contains(t, state.authorizationCodesByCode, authCode.Code)
		require.Empty(t, state.refreshTokensByToken)
	})
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

func TestOAuthStandardRefreshGraceRescuePreservesSuccessorChain(t *testing.T) {
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
	require.Equal(t, storage.RefreshTokenReplacementHash(issued.RefreshToken), state.refreshTokensByToken["active"].ReplacedByHash)
	require.Equal(t, 3, state.refreshTokensByToken[issued.RefreshToken].Generation)

	// The legitimate client can still hold the successor that the first rescue
	// decapitated. Its chain pointer makes that token independently rescuable.
	victimRequest := []byte("grant_type=refresh_token&refresh_token=active&client_id=client-1")
	victim := requireStatus(t, http.StatusOK)(h.HandleOAuthTokenLift(round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, victimRequest)))
	var victimIssued apimodels.OAuthTokenResponse
	require.NoError(t, json.Unmarshal(victim.Body, &victimIssued))
	require.NotEmpty(t, victimIssued.RefreshToken)
	require.Equal(t, 4, state.refreshTokensByToken[victimIssued.RefreshToken].Generation)

	// A later same-client duplicate is stale, not evidence of token theft. It is
	// refused without destroying the live family head.
	lateDuplicate := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, request)))
	requireOAuthTokenError(t, lateDuplicate.Status, "invalid_grant", lateDuplicate.Body)
	require.False(t, state.refreshTokensByToken[victimIssued.RefreshToken].Revoked)
}

func TestOAuthStandardRefreshLateDuplicateAfterTwoAdvancesDoesNotRevokeFamily(t *testing.T) {
	now := time.Now().UTC()
	stale := oauthRefreshReliabilityToken("stale-two-behind", "client-1", now)
	stale.Current = false
	stale.Revoked = true
	stale.RevokedAt = now.Add(-time.Second)
	stale.RevokedReason = "rotated"
	stale.ReplacedByHash = storage.RefreshTokenReplacementHash("middle")
	middle := oauthRefreshReliabilityToken("middle", "client-1", now)
	middle.Generation = 2
	middle.Current = false
	middle.Revoked = true
	middle.RevokedAt = now.Add(-500 * time.Millisecond)
	middle.RevokedReason = "rotated"
	middle.ReplacedByHash = storage.RefreshTokenReplacementHash("head")
	head := oauthRefreshReliabilityToken("head", "client-1", now)
	head.Generation = 3
	for _, token := range []*storagemodels.RefreshToken{&stale, &middle, &head} {
		require.NoError(t, token.UpdateKeys())
	}
	state := &round10QueryState{
		oauthClientsByID: map[string]storagemodels.OAuthClient{"client-1": oauthRefreshReliabilityClient("client-1")},
		refreshTokensByToken: map[string]storagemodels.RefreshToken{
			stale.Token: stale, middle.Token: middle, head.Token: head,
		},
		disableAuditRepo: true,
	}
	h, _, _ := round11NewHandler(t, state)
	request := []byte("grant_type=refresh_token&refresh_token=stale-two-behind&client_id=client-1")

	resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, request)))
	requireOAuthTokenError(t, resp.Status, "invalid_grant", resp.Body)
	require.False(t, state.refreshTokensByToken[head.Token].Revoked)
}

func TestOAuthStandardRefreshResourceMismatchDoesNotRevokeFamily(t *testing.T) {
	now := time.Now().UTC()
	boundResource := "https://api.example.com/mcp/alice"
	mismatchedResource := "https://api.example.com/mcp/other"

	t.Run("direct", func(t *testing.T) {
		token := oauthRefreshReliabilityToken("resource-bound", "client-1", now)
		token.Resource = boundResource
		require.NoError(t, token.UpdateKeys())
		state := &round10QueryState{
			oauthClientsByID:     map[string]storagemodels.OAuthClient{"client-1": oauthRefreshReliabilityClient("client-1")},
			refreshTokensByToken: map[string]storagemodels.RefreshToken{token.Token: token},
			disableAuditRepo:     true,
		}
		h, _, _ := round11NewHandler(t, state)
		params := url.Values{
			"grant_type": {auth.GrantTypeRefreshToken}, "refresh_token": {token.Token}, "client_id": {token.ClientID},
			"resource": {mismatchedResource},
		}

		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte(params.Encode()))))
		requireOAuthTokenError(t, resp.Status, "invalid_grant", resp.Body)
		require.False(t, state.refreshTokensByToken[token.Token].Revoked)
	})

	t.Run("rescue", func(t *testing.T) {
		stale := oauthRefreshReliabilityToken("resource-stale", "client-1", now)
		stale.Resource = boundResource
		stale.Current = false
		stale.Revoked = true
		stale.RevokedAt = now.Add(-time.Second)
		stale.RevokedReason = "rotated"
		stale.ReplacedByHash = storage.RefreshTokenReplacementHash("resource-head")
		head := oauthRefreshReliabilityToken("resource-head", "client-1", now)
		head.Resource = boundResource
		head.Generation = 2
		for _, token := range []*storagemodels.RefreshToken{&stale, &head} {
			require.NoError(t, token.UpdateKeys())
		}
		state := &round10QueryState{
			oauthClientsByID: map[string]storagemodels.OAuthClient{"client-1": oauthRefreshReliabilityClient("client-1")},
			refreshTokensByToken: map[string]storagemodels.RefreshToken{
				stale.Token: stale, head.Token: head,
			},
			disableAuditRepo: true,
		}
		h, _, _ := round11NewHandler(t, state)
		params := url.Values{
			"grant_type": {auth.GrantTypeRefreshToken}, "refresh_token": {stale.Token}, "client_id": {stale.ClientID},
			"resource": {mismatchedResource},
		}

		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte(params.Encode()))))
		requireOAuthTokenError(t, resp.Status, "invalid_grant", resp.Body)
		require.False(t, state.refreshTokensByToken[head.Token].Revoked)
		require.Equal(t, "rotated", state.refreshTokensByToken[stale.Token].RevokedReason)
	})
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
		Generation: row.Generation, Current: row.Current, Revoked: row.Revoked,
		ReplacedByHash: row.ReplacedByHash, Version: row.Version,
	}
}

func oauthRefreshReliabilitySuccessor(token string, predecessor *storage.RefreshToken, now time.Time) *storage.RefreshToken {
	return &storage.RefreshToken{
		Token: token, ClientID: predecessor.ClientID, Username: predecessor.Username, Scopes: predecessor.Scopes,
		CreatedAt: now, ExpiresAt: now.Add(time.Hour), FamilyID: predecessor.FamilyID,
		Generation: predecessor.Generation + 1, Current: true,
	}
}
