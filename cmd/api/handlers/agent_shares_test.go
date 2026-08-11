package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/agentshare"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

type agentShareServiceStub struct {
	grantFunc          func(context.Context, agentshare.ManageInput) (*storagemodels.AgentShareGrant, error)
	revokeFunc         func(context.Context, agentshare.ManageInput) (*storagemodels.AgentShareGrant, error)
	listByAgentFunc    func(context.Context, string, string, bool) ([]*storagemodels.AgentShareGrant, error)
	listSharedWithFunc func(context.Context, string) ([]*storagemodels.AgentShareGrant, error)
}

func (s *agentShareServiceStub) Grant(ctx context.Context, input agentshare.ManageInput) (*storagemodels.AgentShareGrant, error) {
	return s.grantFunc(ctx, input)
}

func (s *agentShareServiceStub) Revoke(ctx context.Context, input agentshare.ManageInput) (*storagemodels.AgentShareGrant, error) {
	return s.revokeFunc(ctx, input)
}

func (s *agentShareServiceStub) ListByAgent(ctx context.Context, agent, actor string, admin bool) ([]*storagemodels.AgentShareGrant, error) {
	return s.listByAgentFunc(ctx, agent, actor, admin)
}

func (s *agentShareServiceStub) ListSharedWith(ctx context.Context, grantee string) ([]*storagemodels.AgentShareGrant, error) {
	return s.listSharedWithFunc(ctx, grantee)
}

func newAgentShareHandler(t *testing.T, service AgentShareService) (*Handler, map[string]string, map[string]string) {
	t.Helper()
	cfg := round10TestConfig()
	cfg.AllowAgents = true
	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{agentInstanceConfig: &storagemodels.AgentInstanceConfig{AllowAgents: true}})
	h.registry = &RegistryStub{AgentSharesSvc: service}
	owner := map[string]string{"Authorization": "Bearer " + round11SignAccessToken(t, cfg.JWTSecret, "owner", []string{auth.ScopeWrite})}
	reader := map[string]string{"Authorization": "Bearer " + round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})}
	return h, owner, reader
}

func TestHandleGrantAgentShareLiftSuccessAndAuthorization(t *testing.T) {
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	var captured agentshare.ManageInput
	service := &agentShareServiceStub{
		grantFunc: func(_ context.Context, input agentshare.ManageInput) (*storagemodels.AgentShareGrant, error) {
			captured = input
			return &storagemodels.AgentShareGrant{AgentUsername: input.AgentUsername, GranteeUsername: input.GranteeUsername, GrantedBy: input.ActorUsername, GrantedAt: now}, nil
		},
	}
	h, ownerHeaders, _ := newAgentShareHandler(t, service)
	ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/agents/agent-one/share/alice", ownerHeaders, nil, nil)
	require.NoError(t, err)
	ctx.Params["username"] = "agent-one"
	ctx.Params["grantee"] = "alice"
	resp := requireStatus(t, http.StatusOK)(h.HandleGrantAgentShareLift(ctx))
	require.Equal(t, "owner", captured.ActorUsername)
	require.Equal(t, "agent-one", captured.AgentUsername)
	require.Equal(t, "alice", captured.GranteeUsername)

	var body apimodels.AgentShareGrant
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	require.True(t, body.Active)
	require.Equal(t, "alice", body.GranteeUsername)

	service.grantFunc = func(context.Context, agentshare.ManageInput) (*storagemodels.AgentShareGrant, error) {
		return nil, agentshare.ErrNotAuthorized
	}
	intruderHeaders := map[string]string{"Authorization": "Bearer " + round11SignAccessToken(t, h.cfg.JWTSecret, "intruder", []string{auth.ScopeWrite})}
	intruderCtx, err := round10NewLiftContext(http.MethodPut, "/api/v1/agents/agent-one/share/alice", intruderHeaders, nil, nil)
	require.NoError(t, err)
	intruderCtx.Params["username"] = "agent-one"
	intruderCtx.Params["grantee"] = "alice"
	requireStatus(t, http.StatusForbidden)(h.HandleGrantAgentShareLift(intruderCtx))
}

func TestHandleGrantAgentShareLiftRejectsInvalidAndUnavailableGrantees(t *testing.T) {
	service := &agentShareServiceStub{
		grantFunc: func(context.Context, agentshare.ManageInput) (*storagemodels.AgentShareGrant, error) {
			return nil, agentshare.ErrGranteeNotFound
		},
	}
	h, ownerHeaders, _ := newAgentShareHandler(t, service)

	unknownCtx, err := round10NewLiftContext(http.MethodPut, "/api/v1/agents/agent-one/share/missing", ownerHeaders, nil, nil)
	require.NoError(t, err)
	unknownCtx.Params["username"] = "agent-one"
	unknownCtx.Params["grantee"] = "missing"
	requireStatus(t, http.StatusNotFound)(h.HandleGrantAgentShareLift(unknownCtx))

	remoteCtx, err := round10NewLiftContext(http.MethodPut, "/api/v1/agents/agent-one/share/alice@remote.example", ownerHeaders, nil, nil)
	require.NoError(t, err)
	remoteCtx.Params["username"] = "agent-one"
	remoteCtx.Params["grantee"] = "alice@remote.example"
	requireStatus(t, http.StatusBadRequest)(h.HandleGrantAgentShareLift(remoteCtx))

	for _, serviceErr := range []error{agentshare.ErrSelfGrant, agentshare.ErrAgentSelfGrant} {
		service.grantFunc = func(context.Context, agentshare.ManageInput) (*storagemodels.AgentShareGrant, error) {
			return nil, serviceErr
		}
		ctx, ctxErr := round10NewLiftContext(http.MethodPut, "/api/v1/agents/agent-one/share/alice", ownerHeaders, nil, nil)
		require.NoError(t, ctxErr)
		ctx.Params["username"] = "agent-one"
		ctx.Params["grantee"] = "alice"
		requireStatus(t, http.StatusUnprocessableEntity)(h.HandleGrantAgentShareLift(ctx))
	}
}

func TestHandleRevokeAndListAgentSharesLift(t *testing.T) {
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	revokedAt := now.Add(time.Hour)
	revoked := &storagemodels.AgentShareGrant{AgentUsername: "agent-one", GranteeUsername: "alice", GrantedBy: "owner", GrantedAt: now, RevokedAt: &revokedAt, RevokedBy: "owner"}
	active := &storagemodels.AgentShareGrant{AgentUsername: "agent-two", GranteeUsername: "alice", GrantedBy: "other-owner", GrantedAt: now}
	service := &agentShareServiceStub{
		revokeFunc: func(context.Context, agentshare.ManageInput) (*storagemodels.AgentShareGrant, error) {
			return revoked, nil
		},
		listByAgentFunc: func(context.Context, string, string, bool) ([]*storagemodels.AgentShareGrant, error) {
			return []*storagemodels.AgentShareGrant{revoked}, nil
		},
		listSharedWithFunc: func(context.Context, string) ([]*storagemodels.AgentShareGrant, error) {
			return []*storagemodels.AgentShareGrant{active}, nil
		},
	}
	h, ownerHeaders, readerHeaders := newAgentShareHandler(t, service)

	revokeCtx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/agents/agent-one/share/alice", ownerHeaders, nil, nil)
	require.NoError(t, err)
	revokeCtx.Params["username"] = "agent-one"
	revokeCtx.Params["grantee"] = "alice"
	revokeResp := requireStatus(t, http.StatusOK)(h.HandleRevokeAgentShareLift(revokeCtx))
	var revokedBody apimodels.AgentShareGrant
	require.NoError(t, json.Unmarshal(revokeResp.Body, &revokedBody))
	require.False(t, revokedBody.Active)
	require.Equal(t, "owner", revokedBody.RevokedBy)

	ownerCtx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/agent-one/share", ownerHeaders, nil, nil)
	require.NoError(t, err)
	ownerCtx.Params["username"] = "agent-one"
	ownerResp := requireStatus(t, http.StatusOK)(h.HandleListAgentSharesLift(ownerCtx))
	var ownerList apimodels.AgentShareGrantListResponse
	require.NoError(t, json.Unmarshal(ownerResp.Body, &ownerList))
	require.Len(t, ownerList.Grants, 1)
	require.False(t, ownerList.Grants[0].Active)

	sharedCtx, err := round10NewLiftContext(http.MethodGet, "/api/v1/agents/shared-with-me", readerHeaders, nil, nil)
	require.NoError(t, err)
	sharedResp := requireStatus(t, http.StatusOK)(h.HandleListAgentsSharedWithMeLift(sharedCtx))
	var sharedList apimodels.AgentShareGrantListResponse
	require.NoError(t, json.Unmarshal(sharedResp.Body, &sharedList))
	require.Len(t, sharedList.Grants, 1)
	require.True(t, sharedList.Grants[0].Active)
	require.Equal(t, "agent-two", sharedList.Grants[0].AgentUsername)
}
