package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func oauthActorGranteePKCE() (verifier, challenge string) {
	verifier = "lesser-grantee-mcp-verifier"
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:])
}

func oauthActorGranteeState(agentOwner string) *round10QueryState {
	return &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"agent-one": {Username: "agent-one", IsAgent: true, AgentType: "counsel", AgentOwner: agentOwner},
			"alice":     {Username: "alice", Approved: true},
			"owner":     {Username: "owner", Approved: true},
		},
	}
}

func oauthActorGranteeAccountsStub(captured **storage.AuthorizationCode) *AccountsServiceStub {
	return &AccountsServiceStub{
		GetUserAppConsentFunc: func(context.Context, *accounts.GetUserAppConsentQuery) (*accounts.GetUserAppConsentResult, error) {
			return &accounts.GetUserAppConsentResult{
				Consent: &storage.UserAppConsent{Scopes: []string{auth.ScopeRead, auth.ScopeWrite}},
			}, nil
		},
		CreateAuthorizationCodeFunc: func(_ context.Context, cmd *accounts.CreateAuthorizationCodeCommand) (*accounts.CreateAuthorizationCodeResult, error) {
			*captured = cmd.AuthCode
			return &accounts.CreateAuthorizationCodeResult{}, nil
		},
	}
}

func oauthActorGranteeSeedCode(state *round10QueryState, code *storage.AuthorizationCode) {
	if code == nil {
		return
	}
	state.authorizationCodesByCode = map[string]storagemodels.AuthorizationCode{
		code.Code: {
			Code:              code.Code,
			ClientID:          code.ClientID,
			RedirectURI:       code.RedirectURI,
			Resource:          code.Resource,
			Username:          code.Username,
			PrincipalUsername: code.PrincipalUsername,
			CodeChallenge:     code.CodeChallenge,
			ExpiresAt:         code.ExpiresAt,
			Scopes:            code.Scopes,
		},
	}
}

func oauthActorGranteeDynamicClient() map[string]storagemodels.OAuthClient {
	return map[string]storagemodels.OAuthClient{
		"mcp-client": {
			ClientID:           "mcp-client",
			Name:               "Grantee MCP Connector",
			RedirectURIs:       []string{"https://client.example/callback"},
			Scopes:             []string{auth.ScopeRead, auth.ScopeWrite},
			ClientClass:        auth.ClientClassCLI,
			RegistrationSource: oauthRegistrationSourceDynamic,
			CreatedAt:          time.Now().Add(-24 * time.Hour),
		},
	}
}

func TestOAuthActorResourceGranteeReceivesCodeWithAgentSubject(t *testing.T) {
	cfg := round11TestConfig()
	resource := auth.BuildPublicMCPAccessBundle(cfg.BaseURL(), "agent-one").MCPURL
	_, challenge := oauthActorGranteePKCE()

	state := oauthActorGranteeState("@owner")
	state.oauthClientsByID = oauthActorGranteeDynamicClient()

	var issuedAuthCode *storage.AuthorizationCode
	h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{
		AccountsSvc:    oauthActorGranteeAccountsStub(&issuedAuthCode),
		AgentSharesSvc: actAsActiveGrantStub(),
	})

	ctx, err := round10NewLiftContext(http.MethodGet, "/oauth/authorize", nil, map[string]string{
		"response_type":         "code",
		"client_id":             "mcp-client",
		"redirect_uri":          "https://client.example/callback",
		"resource":              resource,
		"scope":                 "read write",
		"state":                 "grantee",
		"code_challenge":        challenge,
		"code_challenge_method": "S256",
	}, nil)
	require.NoError(t, err)
	ctx.Set("username", "alice")

	resp := requireStatus(t, http.StatusFound)(h.HandleOAuthAuthorizeLift(ctx))
	redirectURL, err := url.Parse(firstStringValue(resp.Headers, "location"))
	require.NoError(t, err)
	require.NotEmpty(t, redirectURL.Query().Get("code"))
	require.NotContains(t, redirectURL.Query().Get("error"), "access_denied")

	require.NotNil(t, issuedAuthCode)
	require.Equal(t, resource, issuedAuthCode.Resource)
	require.Equal(t, "agent-one", issuedAuthCode.Username)
	require.Equal(t, "alice", issuedAuthCode.PrincipalUsername)
}

func TestOAuthActorResourceGranteeTokenKeepsAgentSubjectDelegatedByGrantee(t *testing.T) {
	cfg := round11TestConfig()
	resource := auth.BuildPublicMCPAccessBundle(cfg.BaseURL(), "agent-one").MCPURL
	verifier, challenge := oauthActorGranteePKCE()

	state := oauthActorGranteeState("@owner")
	state.oauthClientsByID = oauthActorGranteeDynamicClient()

	var issuedAuthCode *storage.AuthorizationCode
	h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{
		AccountsSvc:    oauthActorGranteeAccountsStub(&issuedAuthCode),
		AgentSharesSvc: actAsActiveGrantStub(),
	})

	authorizeCtx, err := round10NewLiftContext(http.MethodGet, "/oauth/authorize", nil, map[string]string{
		"response_type":         "code",
		"client_id":             "mcp-client",
		"redirect_uri":          "https://client.example/callback",
		"resource":              resource,
		"scope":                 "read write",
		"state":                 "grantee",
		"code_challenge":        challenge,
		"code_challenge_method": "S256",
	}, nil)
	require.NoError(t, err)
	authorizeCtx.Set("username", "alice")

	authorizeResp := requireStatus(t, http.StatusFound)(h.HandleOAuthAuthorizeLift(authorizeCtx))
	redirectURL, err := url.Parse(firstStringValue(authorizeResp.Headers, "location"))
	require.NoError(t, err)
	require.NotEmpty(t, redirectURL.Query().Get("code"))
	require.NotNil(t, issuedAuthCode)

	oauthActorGranteeSeedCode(state, issuedAuthCode)

	tokenParams := url.Values{
		"grant_type":    {oauthGrantTypeAuthorizationCode},
		"code":          {issuedAuthCode.Code},
		"client_id":     {"mcp-client"},
		"redirect_uri":  {"https://client.example/callback"},
		"resource":      {resource},
		"code_verifier": {verifier},
	}
	tokenResp := requireStatus(t, http.StatusOK)(h.HandleOAuthTokenLift(round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte(tokenParams.Encode()))))
	var tokenBody apimodels.OAuthTokenResponse
	require.NoError(t, json.Unmarshal(tokenResp.Body, &tokenBody))
	require.NotEmpty(t, tokenBody.AccessToken)

	claims := round12DecodeJWTClaims(t, tokenBody.AccessToken)
	require.Equal(t, "agent-one", claims.Username)
	require.Equal(t, "agent-one", claims.Subject)
	require.Equal(t, []string{resource}, []string(claims.Audience))
	require.True(t, claims.IsAgent)
	require.Equal(t, auth.ClientClassCLI, claims.ClientClass)
	require.Equal(t, "@alice", claims.DelegatedBy)

	// A grantee MCP token carries no scoped delegation binding, so the
	// delegation-credential validation path is not engaged by the DelegatedBy change.
	principal, present, err := auth.ValidateDelegationAttestation(&claims, auth.DelegationContentClassNote)
	require.NoError(t, err)
	require.False(t, present)
	require.Empty(t, principal)

	storedRefresh, ok := state.refreshTokensByToken[tokenBody.RefreshToken]
	require.True(t, ok)
	require.Equal(t, "agent-one", storedRefresh.Username)
	require.Equal(t, resource, storedRefresh.Resource)

	// The same issued claims, fed through the audit recorder, attribute the agent
	// as the actor and the grantee as the driver. The token issuance itself is also
	// audited, so match the agent-action event specifically.
	auditCtx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", nil, nil, nil)
	require.NoError(t, err)
	h.recordAgentAuditEvent(auditCtx, &claims, "agent.status.create", "target-1", nil)
	var auditEntry *storagemodels.AuthAuditLog
	for _, entry := range state.auditLogsByUser["agent-one"] {
		if entry.EventType == "agent.status.create" {
			auditEntry = entry
			break
		}
	}
	require.NotNil(t, auditEntry)
	require.Equal(t, "agent-one", auditEntry.Username)
	require.Contains(t, auditEntry.Metadata, `"delegated_by":"@alice"`)
	require.Contains(t, auditEntry.Metadata, `"target_id":"target-1"`)
}

func TestOAuthActorResourceOwnerPathUnchanged(t *testing.T) {
	cfg := round11TestConfig()
	resource := auth.BuildPublicMCPAccessBundle(cfg.BaseURL(), "agent-one").MCPURL
	verifier, challenge := oauthActorGranteePKCE()

	state := oauthActorGranteeState("owner")
	state.oauthClientsByID = map[string]storagemodels.OAuthClient{
		"owner-agent-client": {
			ClientID:      "owner-agent-client",
			ClientSecret:  "secret",
			Name:          "Owner Agent Connector",
			RedirectURIs:  []string{"https://client.example/callback"},
			Scopes:        []string{auth.ScopeRead, auth.ScopeWrite},
			ClientClass:   auth.ClientClassAgent,
			AgentUsername: "agent-one",
			CreatedAt:     time.Now().Add(-24 * time.Hour),
		},
	}

	var issuedAuthCode *storage.AuthorizationCode
	h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{
		AccountsSvc: oauthActorGranteeAccountsStub(&issuedAuthCode),
		AgentSharesSvc: &actAsShareServiceStub{
			isActiveFunc: func(context.Context, string, string) (bool, error) {
				t.Fatal("owner path must not consult the share-grant check")
				return false, nil
			},
		},
	})

	authorizeCtx, err := round10NewLiftContext(http.MethodGet, "/oauth/authorize", nil, map[string]string{
		"response_type":         "code",
		"client_id":             "owner-agent-client",
		"redirect_uri":          "https://client.example/callback",
		"resource":              resource,
		"scope":                 "read write",
		"state":                 "owner",
		"code_challenge":        challenge,
		"code_challenge_method": "S256",
	}, nil)
	require.NoError(t, err)
	authorizeCtx.Set("username", "owner")

	authorizeResp := requireStatus(t, http.StatusFound)(h.HandleOAuthAuthorizeLift(authorizeCtx))
	redirectURL, err := url.Parse(firstStringValue(authorizeResp.Headers, "location"))
	require.NoError(t, err)
	require.NotEmpty(t, redirectURL.Query().Get("code"))
	require.NotNil(t, issuedAuthCode)
	require.Equal(t, resource, issuedAuthCode.Resource)
	require.Equal(t, "agent-one", issuedAuthCode.Username)
	require.Equal(t, "owner", issuedAuthCode.PrincipalUsername)

	oauthActorGranteeSeedCode(state, issuedAuthCode)

	tokenParams := url.Values{
		"grant_type":    {oauthGrantTypeAuthorizationCode},
		"code":          {issuedAuthCode.Code},
		"client_id":     {"owner-agent-client"},
		"redirect_uri":  {"https://client.example/callback"},
		"resource":      {resource},
		"code_verifier": {verifier},
	}
	tokenResp := requireStatus(t, http.StatusOK)(h.HandleOAuthTokenLift(round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte(tokenParams.Encode()))))
	var tokenBody apimodels.OAuthTokenResponse
	require.NoError(t, json.Unmarshal(tokenResp.Body, &tokenBody))
	require.NotEmpty(t, tokenBody.AccessToken)

	claims := round12DecodeJWTClaims(t, tokenBody.AccessToken)
	require.Equal(t, "agent-one", claims.Username)
	require.Equal(t, "agent-one", claims.Subject)
	require.Equal(t, []string{resource}, []string(claims.Audience))
	require.True(t, claims.IsAgent)
	require.Equal(t, auth.ClientClassAgent, claims.ClientClass)
	require.Equal(t, "@owner", claims.DelegatedBy)

	// The owner token likewise carries no scoped delegation binding, so the
	// delegation-credential validation path is not engaged.
	principal, present, err := auth.ValidateDelegationAttestation(&claims, auth.DelegationContentClassNote)
	require.NoError(t, err)
	require.False(t, present)
	require.Empty(t, principal)

	storedRefresh, ok := state.refreshTokensByToken[tokenBody.RefreshToken]
	require.True(t, ok)
	require.Equal(t, "agent-one", storedRefresh.Username)
	require.Equal(t, resource, storedRefresh.Resource)
}

func TestOAuthActorResourceNonGranteeDenied(t *testing.T) {
	cfg := round11TestConfig()
	resource := auth.BuildPublicMCPAccessBundle(cfg.BaseURL(), "agent-one").MCPURL
	_, challenge := oauthActorGranteePKCE()

	state := oauthActorGranteeState("@owner")
	state.oauthClientsByID = oauthActorGranteeDynamicClient()

	var issuedAuthCode *storage.AuthorizationCode
	h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{
		AccountsSvc: oauthActorGranteeAccountsStub(&issuedAuthCode),
		AgentSharesSvc: &actAsShareServiceStub{
			isActiveFunc: func(context.Context, string, string) (bool, error) {
				return false, nil
			},
		},
	})

	ctx, err := round10NewLiftContext(http.MethodGet, "/oauth/authorize", nil, map[string]string{
		"response_type":         "code",
		"client_id":             "mcp-client",
		"redirect_uri":          "https://client.example/callback",
		"resource":              resource,
		"scope":                 "read write",
		"state":                 "denied",
		"code_challenge":        challenge,
		"code_challenge_method": "S256",
	}, nil)
	require.NoError(t, err)
	ctx.Set("username", "alice")

	resp := requireStatus(t, http.StatusFound)(h.HandleOAuthAuthorizeLift(ctx))
	redirectURL, err := url.Parse(firstStringValue(resp.Headers, "location"))
	require.NoError(t, err)
	require.Equal(t, "access_denied", redirectURL.Query().Get("error"))
	require.Empty(t, redirectURL.Query().Get("code"))
	require.Nil(t, issuedAuthCode)
}

func TestOAuthActorResourceGrantCheckErrorFailsClosed(t *testing.T) {
	cfg := round11TestConfig()
	resource := auth.BuildPublicMCPAccessBundle(cfg.BaseURL(), "agent-one").MCPURL
	_, challenge := oauthActorGranteePKCE()

	state := oauthActorGranteeState("@owner")
	state.oauthClientsByID = oauthActorGranteeDynamicClient()

	var issuedAuthCode *storage.AuthorizationCode
	h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{
		AccountsSvc: oauthActorGranteeAccountsStub(&issuedAuthCode),
		AgentSharesSvc: &actAsShareServiceStub{
			isActiveFunc: func(context.Context, string, string) (bool, error) {
				return false, errors.New("dynamodb timeout")
			},
		},
	})

	ctx, err := round10NewLiftContext(http.MethodGet, "/oauth/authorize", nil, map[string]string{
		"response_type":         "code",
		"client_id":             "mcp-client",
		"redirect_uri":          "https://client.example/callback",
		"resource":              resource,
		"scope":                 "read write",
		"state":                 "error",
		"code_challenge":        challenge,
		"code_challenge_method": "S256",
	}, nil)
	require.NoError(t, err)
	ctx.Set("username", "alice")

	resp := requireStatus(t, http.StatusFound)(h.HandleOAuthAuthorizeLift(ctx))
	redirectURL, err := url.Parse(firstStringValue(resp.Headers, "location"))
	require.NoError(t, err)
	require.Equal(t, "server_error", redirectURL.Query().Get("error"))
	require.Empty(t, redirectURL.Query().Get("code"))
	require.Nil(t, issuedAuthCode)
}

func TestRecordAgentAuditEventAgentSubjectRecordsDelegatedBy(t *testing.T) {
	cfg := round11TestConfig()
	state := &round10QueryState{}
	h, _, _ := round11NewHandler(t, cfg, state)

	claims := &auth.Claims{
		Username:       "agent-one",
		IsAgent:        true,
		AgentSessionID: "asid-1",
		DelegatedBy:    "@alice",
	}

	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", map[string]string{
		"x-forwarded-for": "1.2.3.4",
		"user-agent":      "test-agent/1.0",
	}, nil, nil)
	require.NoError(t, err)

	h.recordAgentAuditEvent(ctx, claims, "agent.status.create", "target-1", map[string]any{"foo": "bar"})

	entries := state.auditLogsByUser["agent-one"]
	require.Len(t, entries, 1)
	entry := entries[0]
	require.Equal(t, "agent.status.create", entry.EventType)
	require.Equal(t, "agent-one", entry.Username)
	require.Equal(t, "asid-1", entry.SessionID)
	require.Equal(t, "1.2.3.4", entry.IPAddress)
	require.Equal(t, "test-agent/1.0", entry.UserAgent)
	require.Contains(t, entry.Metadata, `"target_id":"target-1"`)
	require.Contains(t, entry.Metadata, `"delegated_by":"@alice"`)
	require.Contains(t, entry.Metadata, `"foo":"bar"`)
}

func TestRecordAgentAuditEventAgentSubjectWithNilMetadataRecordsDelegatedBy(t *testing.T) {
	cfg := round11TestConfig()
	state := &round10QueryState{}
	h, _, _ := round11NewHandler(t, cfg, state)

	claims := &auth.Claims{Username: "agent-one", IsAgent: true, DelegatedBy: "@owner"}

	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", nil, nil, nil)
	require.NoError(t, err)

	h.recordAgentAuditEvent(ctx, claims, "agent.status.create", "target-1", nil)

	entries := state.auditLogsByUser["agent-one"]
	require.Len(t, entries, 1)
	require.Contains(t, entries[0].Metadata, `"target_id":"target-1"`)
	require.Contains(t, entries[0].Metadata, `"delegated_by":"@owner"`)
}

func TestRecordAgentAuditEventIgnoresNonAgent(t *testing.T) {
	cfg := round11TestConfig()
	state := &round10QueryState{}
	h, _, _ := round11NewHandler(t, cfg, state)

	claims := &auth.Claims{Username: "alice"}

	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", nil, nil, nil)
	require.NoError(t, err)

	h.recordAgentAuditEvent(ctx, claims, "agent.status.create", "target-1", map[string]any{"foo": "bar"})

	require.Empty(t, state.auditLogsByUser)
}

// oauthActorGranteeIssue drives the full authorize -> token exchange for the grantee
// (alice) against the shared agent-one MCP resource and returns the issued token body.
func oauthActorGranteeIssue(t *testing.T, cfg *config.Config, state *round10QueryState, h *Handler, resource string, issuedAuthCode **storage.AuthorizationCode) apimodels.OAuthTokenResponse {
	t.Helper()
	verifier, challenge := oauthActorGranteePKCE()

	authorizeCtx, err := round10NewLiftContext(http.MethodGet, "/oauth/authorize", nil, map[string]string{
		"response_type":         "code",
		"client_id":             "mcp-client",
		"redirect_uri":          "https://client.example/callback",
		"resource":              resource,
		"scope":                 "read write",
		"state":                 "grantee",
		"code_challenge":        challenge,
		"code_challenge_method": "S256",
	}, nil)
	require.NoError(t, err)
	authorizeCtx.Set("username", "alice")

	authorizeResp := requireStatus(t, http.StatusFound)(h.HandleOAuthAuthorizeLift(authorizeCtx))
	redirectURL, err := url.Parse(firstStringValue(authorizeResp.Headers, "location"))
	require.NoError(t, err)
	require.NotEmpty(t, redirectURL.Query().Get("code"))
	require.NotNil(t, *issuedAuthCode)

	oauthActorGranteeSeedCode(state, *issuedAuthCode)

	tokenParams := url.Values{
		"grant_type":    {oauthGrantTypeAuthorizationCode},
		"code":          {(*issuedAuthCode).Code},
		"client_id":     {"mcp-client"},
		"redirect_uri":  {"https://client.example/callback"},
		"resource":      {resource},
		"code_verifier": {verifier},
	}
	tokenResp := requireStatus(t, http.StatusOK)(h.HandleOAuthTokenLift(round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte(tokenParams.Encode()))))
	var body apimodels.OAuthTokenResponse
	require.NoError(t, json.Unmarshal(tokenResp.Body, &body))
	require.NotEmpty(t, body.AccessToken)
	require.NotEmpty(t, body.RefreshToken)
	return body
}

func TestOAuthActorResourceGranteeRefreshFailsClosedAfterRevocation(t *testing.T) {
	cfg := round11TestConfig()
	resource := auth.BuildPublicMCPAccessBundle(cfg.BaseURL(), "agent-one").MCPURL

	state := oauthActorGranteeState("@owner")
	state.oauthClientsByID = oauthActorGranteeDynamicClient()

	grantActive := true
	var issuedAuthCode *storage.AuthorizationCode
	h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{
		AccountsSvc: oauthActorGranteeAccountsStub(&issuedAuthCode),
		AgentSharesSvc: &actAsShareServiceStub{
			isActiveFunc: func(_ context.Context, agent, grantee string) (bool, error) {
				return grantActive, nil
			},
		},
	})

	issued := oauthActorGranteeIssue(t, cfg, state, h, resource, &issuedAuthCode)
	require.Equal(t, "@alice", round12DecodeJWTClaims(t, issued.AccessToken).DelegatedBy)

	// Owner revokes the grant; the grantee's refresh token must not mint new tokens.
	grantActive = false

	refreshCtx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=refresh_token&refresh_token="+url.QueryEscape(issued.RefreshToken)+"&client_id=mcp-client"))
	resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(refreshCtx))
	var body map[string]string
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	require.Equal(t, "invalid_grant", body["error"])
}

func TestOAuthActorResourceGranteeRefreshKeepsDelegatedByGrantee(t *testing.T) {
	cfg := round11TestConfig()
	resource := auth.BuildPublicMCPAccessBundle(cfg.BaseURL(), "agent-one").MCPURL

	state := oauthActorGranteeState("@owner")
	state.oauthClientsByID = oauthActorGranteeDynamicClient()

	var issuedAuthCode *storage.AuthorizationCode
	h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{
		AccountsSvc:    oauthActorGranteeAccountsStub(&issuedAuthCode),
		AgentSharesSvc: actAsActiveGrantStub(),
	})

	issued := oauthActorGranteeIssue(t, cfg, state, h, resource, &issuedAuthCode)
	require.Equal(t, "@alice", round12DecodeJWTClaims(t, issued.AccessToken).DelegatedBy)

	refreshCtx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=refresh_token&refresh_token="+url.QueryEscape(issued.RefreshToken)+"&client_id=mcp-client"))
	refreshResp := requireStatus(t, http.StatusOK)(h.HandleOAuthTokenLift(refreshCtx))
	var refreshed apimodels.OAuthTokenResponse
	require.NoError(t, json.Unmarshal(refreshResp.Body, &refreshed))
	require.NotEmpty(t, refreshed.AccessToken)
	require.NotEmpty(t, refreshed.RefreshToken)

	claims := round12DecodeJWTClaims(t, refreshed.AccessToken)
	require.Equal(t, "agent-one", claims.Username)
	require.Equal(t, "agent-one", claims.Subject)
	require.True(t, claims.IsAgent)
	require.Equal(t, "@alice", claims.DelegatedBy)

	newStored, ok := state.refreshTokensByToken[refreshed.RefreshToken]
	require.True(t, ok)
	require.Equal(t, "alice", newStored.PrincipalUsername)
}

func TestOAuthActorResourceOwnerRefreshUnchanged(t *testing.T) {
	cfg := round11TestConfig()
	resource := auth.BuildPublicMCPAccessBundle(cfg.BaseURL(), "agent-one").MCPURL
	verifier, challenge := oauthActorGranteePKCE()

	state := oauthActorGranteeState("owner")
	state.oauthClientsByID = map[string]storagemodels.OAuthClient{
		"owner-agent-client": {
			ClientID:      "owner-agent-client",
			ClientSecret:  "secret",
			Name:          "Owner Agent Connector",
			RedirectURIs:  []string{"https://client.example/callback"},
			Scopes:        []string{auth.ScopeRead, auth.ScopeWrite},
			ClientClass:   auth.ClientClassAgent,
			AgentUsername: "agent-one",
			CreatedAt:     time.Now().Add(-24 * time.Hour),
		},
	}

	var issuedAuthCode *storage.AuthorizationCode
	h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{
		AccountsSvc: oauthActorGranteeAccountsStub(&issuedAuthCode),
		AgentSharesSvc: &actAsShareServiceStub{
			isActiveFunc: func(context.Context, string, string) (bool, error) {
				t.Fatal("owner refresh must not consult the share-grant check")
				return false, nil
			},
		},
	})

	authorizeCtx, err := round10NewLiftContext(http.MethodGet, "/oauth/authorize", nil, map[string]string{
		"response_type":         "code",
		"client_id":             "owner-agent-client",
		"redirect_uri":          "https://client.example/callback",
		"resource":              resource,
		"scope":                 "read write",
		"state":                 "owner",
		"code_challenge":        challenge,
		"code_challenge_method": "S256",
	}, nil)
	require.NoError(t, err)
	authorizeCtx.Set("username", "owner")

	authorizeResp := requireStatus(t, http.StatusFound)(h.HandleOAuthAuthorizeLift(authorizeCtx))
	redirectURL, err := url.Parse(firstStringValue(authorizeResp.Headers, "location"))
	require.NoError(t, err)
	require.NotEmpty(t, redirectURL.Query().Get("code"))
	require.NotNil(t, issuedAuthCode)

	oauthActorGranteeSeedCode(state, issuedAuthCode)

	tokenParams := url.Values{
		"grant_type":    {oauthGrantTypeAuthorizationCode},
		"code":          {issuedAuthCode.Code},
		"client_id":     {"owner-agent-client"},
		"redirect_uri":  {"https://client.example/callback"},
		"resource":      {resource},
		"code_verifier": {verifier},
	}
	tokenResp := requireStatus(t, http.StatusOK)(h.HandleOAuthTokenLift(round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte(tokenParams.Encode()))))
	var tokenBody apimodels.OAuthTokenResponse
	require.NoError(t, json.Unmarshal(tokenResp.Body, &tokenBody))
	require.NotEmpty(t, tokenBody.RefreshToken)
	require.Equal(t, "@owner", round12DecodeJWTClaims(t, tokenBody.AccessToken).DelegatedBy)

	refreshCtx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=refresh_token&refresh_token="+url.QueryEscape(tokenBody.RefreshToken)+"&client_id=owner-agent-client"))
	refreshResp := requireStatus(t, http.StatusOK)(h.HandleOAuthTokenLift(refreshCtx))
	var refreshBody apimodels.OAuthTokenResponse
	require.NoError(t, json.Unmarshal(refreshResp.Body, &refreshBody))

	claims := round12DecodeJWTClaims(t, refreshBody.AccessToken)
	require.Equal(t, "agent-one", claims.Username)
	require.True(t, claims.IsAgent)
	require.Equal(t, "@owner", claims.DelegatedBy)

	newStored, ok := state.refreshTokensByToken[refreshBody.RefreshToken]
	require.True(t, ok)
	require.Equal(t, "owner", newStored.PrincipalUsername)
}

func TestOAuthActorResourceLegacyRefreshTokenFailsClosed(t *testing.T) {
	cfg := round11TestConfig()
	resource := auth.BuildPublicMCPAccessBundle(cfg.BaseURL(), "agent-one").MCPURL

	state := oauthActorGranteeState("@owner")
	state.oauthClientsByID = oauthActorGranteeDynamicClient()
	state.refreshTokensByToken = map[string]storagemodels.RefreshToken{
		"rt-legacy": {
			Token:     "rt-legacy",
			ClientID:  "mcp-client",
			Username:  "agent-one",
			Resource:  resource,
			Scopes:    []string{auth.ScopeRead, auth.ScopeWrite},
			CreatedAt: time.Now().Add(-time.Hour),
			ExpiresAt: time.Now().Add(24 * time.Hour),
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{
		AgentSharesSvc: &actAsShareServiceStub{
			isActiveFunc: func(context.Context, string, string) (bool, error) {
				t.Fatal("legacy refresh must fail closed before consulting the grant check")
				return false, nil
			},
		},
	})

	refreshCtx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=refresh_token&refresh_token=rt-legacy&client_id=mcp-client"))
	resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(refreshCtx))
	var body map[string]string
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	require.Equal(t, "invalid_grant", body["error"])
}
