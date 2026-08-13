package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/config"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
)

func agentAccessSignToken(t *testing.T, secret, username, delegatedBy string, isAgent bool) string {
	t.Helper()
	now := time.Now()
	claims := auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   username,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
		Username:    username,
		ClientID:    "test-client",
		Scopes:      []string{auth.ScopeRead},
		IsAgent:     isAgent,
		DelegatedBy: delegatedBy,
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	require.NoError(t, err)
	return signed
}

func agentAccessConfig() *config.Config {
	cfg := round11TestConfig()
	cfg.AllowAgents = true
	return cfg
}

func agentAccessState(agentOwner string) *round10QueryState {
	now := time.Now()
	return &round10QueryState{
		agentInstanceConfig: &storagemodels.AgentInstanceConfig{AllowAgents: true},
		usersByUsername: map[string]storagemodels.User{
			"agent-one": {
				Username:   "agent-one",
				IsAgent:    true,
				AgentType:  "counsel",
				AgentOwner: agentOwner,
				Approved:   true,
				Version:    1,
				CreatedAt:  now,
				UpdatedAt:  now,
			},
			"human": {
				Username:  "human",
				IsAgent:   false,
				Approved:  true,
				Version:   1,
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
	}
}

func agentAccessContext(t *testing.T, method, path, token string) *apptheory.Context {
	t.Helper()
	ctx, err := round10NewLiftContext(method, path, map[string]string{"Authorization": "Bearer " + token}, nil, nil)
	require.NoError(t, err)
	return ctx
}

func decodeAgentAccessResponse(t *testing.T, resp *apptheory.Response) apimodels.AgentAccessResponse {
	t.Helper()
	var body apimodels.AgentAccessResponse
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	return body
}

func TestHandleGetAgentAccessLiftOwnerAuthorized(t *testing.T) {
	cfg := agentAccessConfig()
	state := agentAccessState("@owner")
	h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{
		AgentSharesSvc: &actAsShareServiceStub{
			isActiveFunc: func(context.Context, string, string) (bool, error) {
				t.Fatal("owner path must not consult the share-grant check")
				return false, nil
			},
		},
	})

	token := agentAccessSignToken(t, cfg.JWTSecret, "agent-one", "@owner", true)
	ctx := agentAccessContext(t, http.MethodGet, "/api/v1/agents/agent-one/access", token)
	ctx.Params["username"] = "agent-one"

	resp := requireStatus(t, http.StatusOK)(h.HandleGetAgentAccessLift(ctx))
	body := decodeAgentAccessResponse(t, resp)
	require.Equal(t, "agent-one", body.Actor)
	require.Equal(t, "owner", body.Relationship)
	require.True(t, body.Authorized)
	require.Equal(t, "owner", body.ActedBy)
}

func TestHandleGetAgentAccessLiftGranteeAuthorized(t *testing.T) {
	cfg := agentAccessConfig()
	state := agentAccessState("@owner")
	h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{
		AgentSharesSvc: &actAsShareServiceStub{
			isActiveFunc: func(_ context.Context, agent, grantee string) (bool, error) {
				return agent == "agent-one" && grantee == "alice", nil
			},
		},
	})

	token := agentAccessSignToken(t, cfg.JWTSecret, "agent-one", "@alice", true)
	ctx := agentAccessContext(t, http.MethodGet, "/api/v1/agents/agent-one/access", token)
	ctx.Params["username"] = "agent-one"

	resp := requireStatus(t, http.StatusOK)(h.HandleGetAgentAccessLift(ctx))
	body := decodeAgentAccessResponse(t, resp)
	require.Equal(t, "agent-one", body.Actor)
	require.Equal(t, "grantee", body.Relationship)
	require.True(t, body.Authorized)
	require.Equal(t, "alice", body.ActedBy)
}

func TestHandleGetAgentAccessLiftRevokedGrantDeniedOnNextRequest(t *testing.T) {
	cfg := agentAccessConfig()
	state := agentAccessState("@owner")

	grantActive := true
	h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{
		AgentSharesSvc: &actAsShareServiceStub{
			isActiveFunc: func(_ context.Context, agent, grantee string) (bool, error) {
				return grantActive, nil
			},
		},
	})

	token := agentAccessSignToken(t, cfg.JWTSecret, "agent-one", "@alice", true)

	firstCtx := agentAccessContext(t, http.MethodGet, "/api/v1/agents/agent-one/access", token)
	firstCtx.Params["username"] = "agent-one"
	firstResp := requireStatus(t, http.StatusOK)(h.HandleGetAgentAccessLift(firstCtx))
	require.Equal(t, "grantee", decodeAgentAccessResponse(t, firstResp).Relationship)

	// Revocation must take effect on the very next request — no refresh involved.
	grantActive = false

	secondCtx := agentAccessContext(t, http.MethodGet, "/api/v1/agents/agent-one/access", token)
	secondCtx.Params["username"] = "agent-one"
	requireStatus(t, http.StatusForbidden)(h.HandleGetAgentAccessLift(secondCtx))
}

func TestHandleGetAgentAccessLiftBlankDelegatedByDenied(t *testing.T) {
	cfg := agentAccessConfig()
	state := agentAccessState("@owner")
	h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{
		AgentSharesSvc: &actAsShareServiceStub{
			isActiveFunc: func(context.Context, string, string) (bool, error) {
				t.Fatal("blank DelegatedBy must fail closed before the grant check")
				return false, nil
			},
		},
	})

	for _, delegatedBy := range []string{"", "   "} {
		token := agentAccessSignToken(t, cfg.JWTSecret, "agent-one", delegatedBy, true)
		ctx := agentAccessContext(t, http.MethodGet, "/api/v1/agents/agent-one/access", token)
		ctx.Params["username"] = "agent-one"
		requireStatus(t, http.StatusForbidden)(h.HandleGetAgentAccessLift(ctx))
	}
}

func TestHandleGetAgentAccessLiftUnknownAndNonAgentDenied(t *testing.T) {
	cfg := agentAccessConfig()
	state := agentAccessState("@owner")
	h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{
		AgentSharesSvc: &actAsShareServiceStub{
			isActiveFunc: func(context.Context, string, string) (bool, error) {
				t.Fatal("unknown/non-agent target must fail closed before the grant check")
				return false, nil
			},
		},
	})

	// Unknown actor: the caller may only ask about its own subject, but the subject
	// resolves to no stored agent.
	unknownToken := agentAccessSignToken(t, cfg.JWTSecret, "nobody", "@owner", true)
	unknownCtx := agentAccessContext(t, http.MethodGet, "/api/v1/agents/nobody/access", unknownToken)
	unknownCtx.Params["username"] = "nobody"
	requireStatus(t, http.StatusForbidden)(h.HandleGetAgentAccessLift(unknownCtx))

	// Non-agent target with a matching subject and DelegatedBy set.
	humanToken := agentAccessSignToken(t, cfg.JWTSecret, "human", "@owner", true)
	humanCtx := agentAccessContext(t, http.MethodGet, "/api/v1/agents/human/access", humanToken)
	humanCtx.Params["username"] = "human"
	requireStatus(t, http.StatusForbidden)(h.HandleGetAgentAccessLift(humanCtx))
}

func TestHandleGetAgentAccessLiftStorageErrorDenied(t *testing.T) {
	cfg := agentAccessConfig()
	state := agentAccessState("@owner")
	h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{
		AgentSharesSvc: &actAsShareServiceStub{
			isActiveFunc: func(context.Context, string, string) (bool, error) {
				return false, errors.New("dynamodb timeout")
			},
		},
	})

	token := agentAccessSignToken(t, cfg.JWTSecret, "agent-one", "@alice", true)
	ctx := agentAccessContext(t, http.MethodGet, "/api/v1/agents/agent-one/access", token)
	ctx.Params["username"] = "agent-one"
	requireStatus(t, http.StatusForbidden)(h.HandleGetAgentAccessLift(ctx))
}

func TestHandleGetAgentAccessLiftSubjectMismatchDenied(t *testing.T) {
	cfg := agentAccessConfig()
	state := agentAccessState("@owner")
	h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{
		AgentSharesSvc: &actAsShareServiceStub{
			isActiveFunc: func(context.Context, string, string) (bool, error) {
				t.Fatal("subject mismatch must fail closed before the grant check")
				return false, nil
			},
		},
	})

	// A token for agent-one must not probe access to another agent.
	token := agentAccessSignToken(t, cfg.JWTSecret, "agent-one", "@owner", true)
	ctx := agentAccessContext(t, http.MethodGet, "/api/v1/agents/other-agent/access", token)
	ctx.Params["username"] = "other-agent"
	requireStatus(t, http.StatusForbidden)(h.HandleGetAgentAccessLift(ctx))
}

func TestHandleGetAgentAccessLiftURLOwnerAuthorized(t *testing.T) {
	cfg := agentAccessConfig()
	ownerURL := "https://example.com/users/owner"
	state := agentAccessState(ownerURL)
	h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{
		AgentSharesSvc: &actAsShareServiceStub{
			isActiveFunc: func(context.Context, string, string) (bool, error) {
				t.Fatal("URL owner path must not consult the share-grant check")
				return false, nil
			},
		},
	})

	// The real mint path stores the URL-form owner with the leading "@" that
	// normalizeDelegatedBy prepends to non-"@" values. The handler must strip the
	// "@" and still resolve the URL-form owner directly.
	token := agentAccessSignToken(t, cfg.JWTSecret, "agent-one", "@"+ownerURL, true)
	ctx := agentAccessContext(t, http.MethodGet, "/api/v1/agents/agent-one/access", token)
	ctx.Params["username"] = "agent-one"

	resp := requireStatus(t, http.StatusOK)(h.HandleGetAgentAccessLift(ctx))
	body := decodeAgentAccessResponse(t, resp)
	require.Equal(t, "agent-one", body.Actor)
	require.Equal(t, "owner", body.Relationship)
	require.True(t, body.Authorized)
	require.Equal(t, ownerURL, body.ActedBy)
}

func TestHandleGetAgentAccessLiftURLPrincipalMismatchDenied(t *testing.T) {
	cfg := agentAccessConfig()
	ownerURL := "https://example.com/users/owner"
	state := agentAccessState(ownerURL)
	h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{
		AgentSharesSvc: &actAsShareServiceStub{
			isActiveFunc: func(_ context.Context, agent, grantee string) (bool, error) {
				return false, nil
			},
		},
	})

	// A URL-form DelegatedBy that is not the agent's stored owner must fall
	// through to the (absent) grant check and receive a uniform 403.
	token := agentAccessSignToken(t, cfg.JWTSecret, "agent-one", "@https://example.com/users/intruder", true)
	ctx := agentAccessContext(t, http.MethodGet, "/api/v1/agents/agent-one/access", token)
	ctx.Params["username"] = "agent-one"
	requireStatus(t, http.StatusForbidden)(h.HandleGetAgentAccessLift(ctx))
}
