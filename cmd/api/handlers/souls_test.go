package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	soulservice "github.com/equaltoai/lesser/pkg/services/souls"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
)

type stubSoulHandlerService struct {
	listMineOut     []soulservice.Soul
	listMineErr     error
	incorporateOut  *soulservice.Soul
	incorporateErr  error
	bindOut         *soulservice.SoulBindingProjection
	bindErr         error
	getBindingOut   *soulservice.SoulBindingProjection
	getBindingErr   error
	resolveBoundOut *soulservice.Soul
	resolveBoundErr error
	lastUsername    string
	lastSoulAgentID string
	lastTargetAgent string
	lastBindInput   soulservice.BindSoulBodyInput
	lastGetAgentID  string
}

func (s *stubSoulHandlerService) ListMine(_ context.Context, username string) ([]soulservice.Soul, error) {
	s.lastUsername = username
	return s.listMineOut, s.listMineErr
}

func (s *stubSoulHandlerService) Incorporate(_ context.Context, username string, targetAgentUsername string, soulAgentID string) (*soulservice.Soul, error) {
	s.lastUsername = username
	s.lastTargetAgent = targetAgentUsername
	s.lastSoulAgentID = soulAgentID
	return s.incorporateOut, s.incorporateErr
}

func (s *stubSoulHandlerService) ResolveBoundAgent(_ context.Context, agentUsername string) (*soulservice.Soul, error) {
	s.lastTargetAgent = agentUsername
	return s.resolveBoundOut, s.resolveBoundErr
}

func (s *stubSoulHandlerService) BindSoulBody(_ context.Context, input soulservice.BindSoulBodyInput) (*soulservice.SoulBindingProjection, error) {
	s.lastBindInput = input
	return s.bindOut, s.bindErr
}

func (s *stubSoulHandlerService) GetSoulBinding(_ context.Context, agentID string, actorUsername string) (*soulservice.SoulBindingProjection, error) {
	s.lastGetAgentID = agentID
	s.lastTargetAgent = actorUsername
	return s.getBindingOut, s.getBindingErr
}

type soulPrivateHostReadClientFunc func(*http.Request) (*http.Response, error)

func (f soulPrivateHostReadClientFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

type soulPrivateFailingBody struct{}

func (soulPrivateFailingBody) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}

func (soulPrivateFailingBody) Close() error {
	return nil
}

func TestHandleGetMySoulsLift_ListsOwnedSoulsWithBindingState(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC)
	alphaENS := "alpha.eth"
	service := &stubSoulHandlerService{
		listMineOut: []soulservice.Soul{
			{
				AgentID:          "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Domain:           "example.com",
				LocalID:          "alpha",
				ENSName:          &alphaENS,
				Wallet:           "0x1111111111111111111111111111111111111111",
				PrincipalAddress: "0x1111111111111111111111111111111111111111",
				Status:           "active",
			},
			{
				AgentID:               "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				Domain:                "example.com",
				LocalID:               "beta",
				Wallet:                "0x2222222222222222222222222222222222222222",
				PrincipalAddress:      "0x2222222222222222222222222222222222222222",
				Status:                "active",
				Bound:                 true,
				BoundAgentUsername:    "agent-alpha",
				BoundPrincipalAddress: "0x2222222222222222222222222222222222222222",
				BoundAt:               now,
				BoundUpdatedAt:        now,
			},
			{
				AgentID:               "0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
				Domain:                "example.com",
				LocalID:               "gamma",
				Wallet:                "0x3333333333333333333333333333333333333333",
				PrincipalAddress:      "0x3333333333333333333333333333333333333333",
				Status:                "active",
				Bound:                 true,
				BoundAgentUsername:    "agent-beta",
				BoundPrincipalAddress: "0x3333333333333333333333333333333333333333",
				BoundAt:               now,
				BoundUpdatedAt:        now,
			},
		},
	}

	cfg := round10TestConfig()
	h := &Handler{cfg: cfg, logger: round10TestLogger(t), soulsService: service}
	token := round11SignToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead}, "sess-1")

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/souls/mine", map[string]string{
		"Authorization": "Bearer " + token,
	}, nil, nil)
	require.NoError(t, err)

	resp := requireStatus(t, http.StatusOK)(h.HandleGetMySoulsLift(ctx))
	require.Contains(t, string(resp.Body), "\"ens_name\":\"alpha.eth\"")
	require.Contains(t, string(resp.Body), "\"ens_name\":null")

	var body apimodels.SoulsMineResponse
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	require.Equal(t, "alice", service.lastUsername)
	require.Len(t, body.Souls, 3)
	require.Equal(t, 3, body.Count)
	require.NotNil(t, body.Souls[0].Agent.ENSName)
	require.Equal(t, alphaENS, *body.Souls[0].Agent.ENSName)
	require.Equal(t, "unbound", body.Souls[0].BindingState)
	require.True(t, body.Souls[0].AvailableForIncorporation)
	require.Equal(t, "bound", body.Souls[1].BindingState)
	require.False(t, body.Souls[1].AvailableForIncorporation)
	require.NotNil(t, body.Souls[1].Binding)
	require.Equal(t, "agent-alpha", body.Souls[1].Binding.AgentUsername)
	require.Nil(t, body.Souls[1].Agent.ENSName)
	require.Equal(t, "bound", body.Souls[2].BindingState)
	require.False(t, body.Souls[2].AvailableForIncorporation)
	require.NotNil(t, body.Souls[2].Binding)
	require.Equal(t, "agent-beta", body.Souls[2].Binding.AgentUsername)
}

func TestHandleGetMySoulsLift_MapsServiceErrors(t *testing.T) {
	t.Parallel()

	cfg := round10TestConfig()
	h := &Handler{
		cfg:          cfg,
		logger:       round10TestLogger(t),
		soulsService: &stubSoulHandlerService{listMineErr: soulservice.ErrTrustNotConfigured},
	}
	token := round11SignToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead}, "sess-get-error")

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/souls/mine", map[string]string{
		"Authorization": "Bearer " + token,
	}, nil, nil)
	require.NoError(t, err)

	requireStatus(t, http.StatusUnprocessableEntity)(h.HandleGetMySoulsLift(ctx))
}

func TestHandleGetMySoulsLift_AuthGuards(t *testing.T) {
	t.Parallel()

	cfg := round10TestConfig()
	readToken := round11SignToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead}, "sess-read")
	adminToken := round11SignToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeAdmin}, "sess-admin")
	h := &Handler{cfg: cfg, logger: round10TestLogger(t), soulsService: &stubSoulHandlerService{}}

	testCases := []struct {
		name       string
		headers    map[string]string
		wantStatus int
	}{
		{
			name:       "missing token",
			headers:    nil,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "insufficient scope",
			headers: map[string]string{
				"Authorization": "Bearer " + adminToken,
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "read scope works",
			headers: map[string]string{
				"Authorization": "Bearer " + readToken,
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/souls/mine", tc.headers, nil, nil)
			require.NoError(t, err)

			requireStatus(t, tc.wantStatus)(h.HandleGetMySoulsLift(ctx))
		})
	}
}

func TestHandleGetMySoulsLift_AuthFailures(t *testing.T) {
	t.Parallel()

	cfg := round10TestConfig()
	h := &Handler{
		cfg:          cfg,
		logger:       round10TestLogger(t),
		soulsService: &stubSoulHandlerService{},
	}

	t.Run("missing token returns unauthorized", func(t *testing.T) {
		t.Parallel()

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/souls/mine", nil, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusUnauthorized)(h.HandleGetMySoulsLift(ctx))
	})

	t.Run("token without read or write scope returns forbidden", func(t *testing.T) {
		t.Parallel()

		token := round11SignToken(t, cfg.JWTSecret, "alice", []string{"follow"}, "sess-get-forbidden")
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/souls/mine", map[string]string{
			"Authorization": "Bearer " + token,
		}, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusForbidden)(h.HandleGetMySoulsLift(ctx))
	})
}

func TestHandleGetBoundSoulMeLift_ReturnsActiveBoundSoul(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 1, 14, 30, 0, 0, time.UTC)
	cfg := round10TestConfig()
	service := &stubSoulHandlerService{
		resolveBoundOut: &soulservice.Soul{
			AgentID:               "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Domain:                "example.com",
			LocalID:               "ops",
			Wallet:                "0x1111111111111111111111111111111111111111",
			PrincipalAddress:      "0x2222222222222222222222222222222222222222",
			Status:                "active",
			LifecycleStatus:       "active",
			Bound:                 true,
			BoundAgentUsername:    "ops",
			BoundPrincipalAddress: "0x2222222222222222222222222222222222222222",
			BoundAt:               now,
			BoundUpdatedAt:        now,
		},
	}
	h := &Handler{cfg: cfg, logger: round10TestLogger(t), soulsService: service}
	token := round11SignToken(t, cfg.JWTSecret, "ops", []string{auth.ScopeRead}, "sess-bound")

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/souls/bound/me", map[string]string{
		"Authorization": "Bearer " + token,
	}, nil, nil)
	require.NoError(t, err)

	resp := requireStatus(t, http.StatusOK)(h.HandleGetBoundSoulMeLift(ctx))

	var body apimodels.BoundSoulResponse
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	require.Equal(t, "ops", service.lastTargetAgent)
	require.Equal(t, "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", body.Agent.AgentID)
	require.Equal(t, "ops", body.Agent.LocalID)
	require.Equal(t, "bound", body.BindingState)
	require.Equal(t, "ops", body.Binding.AgentUsername)
	require.Equal(t, "0x2222222222222222222222222222222222222222", body.Binding.PrincipalAddress)
	require.NotContains(t, string(resp.Body), "available_for_incorporation")
}

func TestHandleGetBoundSoulMeLift_AuthGuards(t *testing.T) {
	t.Parallel()

	cfg := round10TestConfig()
	readToken := round11SignToken(t, cfg.JWTSecret, "ops", []string{auth.ScopeRead}, "sess-bound-read")
	writeToken := round11SignToken(t, cfg.JWTSecret, "ops", []string{auth.ScopeWrite}, "sess-bound-write")
	adminToken := round11SignToken(t, cfg.JWTSecret, "ops", []string{auth.ScopeAdmin}, "sess-bound-admin")
	h := &Handler{
		cfg:    cfg,
		logger: round10TestLogger(t),
		soulsService: &stubSoulHandlerService{
			resolveBoundOut: &soulservice.Soul{
				AgentID:            "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				Domain:             "example.com",
				LocalID:            "ops",
				Wallet:             "0x1111111111111111111111111111111111111111",
				Status:             "active",
				Bound:              true,
				BoundAgentUsername: "ops",
				BoundAt:            time.Date(2026, 5, 1, 14, 30, 0, 0, time.UTC),
				BoundUpdatedAt:     time.Date(2026, 5, 1, 14, 30, 0, 0, time.UTC),
			},
		},
	}

	testCases := []struct {
		name       string
		headers    map[string]string
		wantStatus int
	}{
		{
			name:       "missing token",
			headers:    nil,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "insufficient scope",
			headers: map[string]string{
				"Authorization": "Bearer " + adminToken,
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "read scope works",
			headers: map[string]string{
				"Authorization": "Bearer " + readToken,
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "write scope works",
			headers: map[string]string{
				"Authorization": "Bearer " + writeToken,
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/souls/bound/me", tc.headers, nil, nil)
			require.NoError(t, err)

			requireStatus(t, tc.wantStatus)(h.HandleGetBoundSoulMeLift(ctx))
		})
	}
}

func TestHandleGetBoundSoulMeLift_MapsServiceFailures(t *testing.T) {
	t.Parallel()

	cfg := round10TestConfig()
	token := round11SignToken(t, cfg.JWTSecret, "ops", []string{auth.ScopeRead}, "sess-bound-errors")

	testCases := []struct {
		name       string
		service    *stubSoulHandlerService
		wantStatus int
		wantCode   string
	}{
		{
			name:       "unbound",
			service:    &stubSoulHandlerService{},
			wantStatus: http.StatusNotFound,
			wantCode:   "soul_not_bound",
		},
		{
			name:       "unavailable",
			service:    &stubSoulHandlerService{resolveBoundErr: soulservice.ErrSoulNotAvailable},
			wantStatus: http.StatusNotFound,
			wantCode:   "soul_not_available",
		},
		{
			name:       "trust not configured",
			service:    &stubSoulHandlerService{resolveBoundErr: soulservice.ErrTrustNotConfigured},
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name:       "unexpected",
			service:    &stubSoulHandlerService{resolveBoundErr: errors.New("boom")},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := &Handler{cfg: cfg, logger: round10TestLogger(t), soulsService: tc.service}
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/souls/bound/me", map[string]string{
				"Authorization": "Bearer " + token,
			}, nil, nil)
			require.NoError(t, err)

			resp := requireStatus(t, tc.wantStatus)(h.HandleGetBoundSoulMeLift(ctx))
			if tc.wantCode != "" {
				var body common.StandardErrorResponse
				require.NoError(t, json.Unmarshal(resp.Body, &body))
				require.Equal(t, tc.wantCode, body.Code)
			}
		})
	}
}

func TestHandleGetBoundSoulMeLift_ReturnsInternalWhenServiceUnavailable(t *testing.T) {
	t.Parallel()

	cfg := round10TestConfig()
	h := &Handler{cfg: cfg, logger: round10TestLogger(t)}
	token := round11SignToken(t, cfg.JWTSecret, "ops", []string{auth.ScopeRead}, "sess-bound-service")

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/souls/bound/me", map[string]string{
		"Authorization": "Bearer " + token,
	}, nil, nil)
	require.NoError(t, err)

	requireStatus(t, http.StatusInternalServerError)(h.HandleGetBoundSoulMeLift(ctx))
}

func TestHandleListBoundSoulMintConversationsLift_ProxiesThroughInstanceTrust(t *testing.T) {
	allowLocalLesserHostProxyForTests(t)

	const (
		agentID     = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		instanceKey = "instance-key-raw"
	)

	var sawUpstream atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawUpstream.Store(true)
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/v1/soul/instance/agents/"+agentID+"/mint-conversations", r.URL.Path)
		require.Equal(t, "20", r.URL.Query().Get("limit"))
		require.Equal(t, "Bearer "+instanceKey, r.Header.Get("Authorization"))
		require.NotContains(t, r.Header.Get("Authorization"), "user-token")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"version":"1",
			"conversations":[{
				"agent_id":"` + agentID + `",
				"conversation_id":"conv-1",
				"model":"gpt-test",
				"messages":"private list leak",
				"produced_declarations":"private declaration leak",
				"status":"completed",
				"usage":{"input_tokens":1},
				"charged_credits":0,
				"created_at":"2026-05-11T21:00:00Z",
				"completed_at":"2026-05-11T21:01:00Z"
			}],
			"count":1,
			"limit":20
		}`))
	}))
	t.Cleanup(upstream.Close)

	cfg := round10TestConfig()
	cfg.LesserHostURL = upstream.URL
	cfg.LesserHostInstanceKey = instanceKey
	service := &stubSoulHandlerService{
		resolveBoundOut: &soulservice.Soul{
			AgentID:            agentID,
			Domain:             "example.com",
			LocalID:            "ops",
			Status:             "active",
			Bound:              true,
			BoundAgentUsername: "ops",
			BoundAt:            time.Date(2026, 5, 1, 14, 30, 0, 0, time.UTC),
			BoundUpdatedAt:     time.Date(2026, 5, 1, 14, 30, 0, 0, time.UTC),
		},
	}
	h := &Handler{cfg: cfg, logger: round10TestLogger(t), soulsService: service}
	userToken := round11SignToken(t, cfg.JWTSecret, "ops", []string{auth.ScopeRead}, "user-token")

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/souls/bound/me/mint-conversations", map[string]string{
		"Authorization": "Bearer " + userToken,
	}, nil, nil)
	require.NoError(t, err)

	resp := requireStatus(t, http.StatusOK)(h.HandleListBoundSoulMintConversationsLift(ctx))
	require.True(t, sawUpstream.Load())
	require.Equal(t, "ops", service.lastTargetAgent)
	require.NotContains(t, string(resp.Body), "messages")
	require.NotContains(t, string(resp.Body), "produced_declarations")
	require.NotContains(t, string(resp.Body), "private list leak")

	var body apimodels.SoulMintConversationsResponse
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	require.Equal(t, "1", body.Version)
	require.Equal(t, 1, body.Count)
	require.Equal(t, 20, body.Limit)
	require.Len(t, body.Conversations, 1)
	require.Equal(t, agentID, body.Conversations[0].AgentID)
	require.Equal(t, "conv-1", body.Conversations[0].ConversationID)
}

func TestHandleGetBoundSoulMintConversationLift_ReturnsExplicitSingleRecord(t *testing.T) {
	allowLocalLesserHostProxyForTests(t)

	const (
		agentID        = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		conversationID = "conv:single.1"
		instanceKey    = "instance-key-raw"
	)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/v1/soul/instance/agents/"+agentID+"/mint-conversations/"+conversationID, r.URL.Path)
		require.Empty(t, r.URL.RawQuery)
		require.Equal(t, "Bearer "+instanceKey, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"version":"1",
			"conversation":{
				"agent_id":"` + agentID + `",
				"conversation_id":"` + conversationID + `",
				"model":"gpt-test",
				"messages":"[{\"role\":\"user\",\"content\":\"private\"}]",
				"produced_declarations":"{}",
				"status":"completed",
				"usage":{"output_tokens":2},
				"charged_credits":7,
				"created_at":"2026-05-11T21:00:00Z",
				"completed_at":"2026-05-11T21:01:00Z"
			}
		}`))
	}))
	t.Cleanup(upstream.Close)

	cfg := round10TestConfig()
	cfg.LesserHostURL = upstream.URL
	cfg.LesserHostInstanceKey = instanceKey
	service := &stubSoulHandlerService{
		resolveBoundOut: &soulservice.Soul{
			AgentID:            agentID,
			Domain:             "example.com",
			LocalID:            "ops",
			Status:             "active",
			Bound:              true,
			BoundAgentUsername: "ops",
			BoundAt:            time.Date(2026, 5, 1, 14, 30, 0, 0, time.UTC),
			BoundUpdatedAt:     time.Date(2026, 5, 1, 14, 30, 0, 0, time.UTC),
		},
	}
	h := &Handler{cfg: cfg, logger: round10TestLogger(t), soulsService: service}
	userToken := round11SignToken(t, cfg.JWTSecret, "ops", []string{auth.ScopeRead}, "sess-private-single")

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/souls/bound/me/mint-conversations/"+conversationID, map[string]string{
		"Authorization": "Bearer " + userToken,
	}, nil, nil)
	require.NoError(t, err)
	ctx.Params["conversationId"] = conversationID

	resp := requireStatus(t, http.StatusOK)(h.HandleGetBoundSoulMintConversationLift(ctx))

	var body apimodels.SoulMintConversationResponse
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	require.Equal(t, "1", body.Version)
	require.Equal(t, agentID, body.Conversation.AgentID)
	require.Equal(t, conversationID, body.Conversation.ConversationID)
	require.Contains(t, body.Conversation.Messages, "private")
	require.Equal(t, "{}", body.Conversation.ProducedDeclarations)
	require.NotNil(t, body.Conversation.ChargedCredits)
	require.Equal(t, int64(7), *body.Conversation.ChargedCredits)
}

func TestBoundSoulMintConversationHandlers_GuardsInputsAndBinding(t *testing.T) {
	allowLocalLesserHostProxyForTests(t)

	const agentID = "0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"

	var upstreamHits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamHits.Add(1)
		_, _ = w.Write([]byte(`{"version":"1","conversations":[],"count":0,"limit":20}`))
	}))
	t.Cleanup(upstream.Close)

	cfg := round10TestConfig()
	cfg.LesserHostURL = upstream.URL
	cfg.LesserHostInstanceKey = "instance-key-raw"
	readToken := round11SignToken(t, cfg.JWTSecret, "ops", []string{auth.ScopeRead}, "sess-private-guards")

	boundService := &stubSoulHandlerService{
		resolveBoundOut: &soulservice.Soul{
			AgentID:            agentID,
			Domain:             "example.com",
			LocalID:            "ops",
			Status:             "active",
			Bound:              true,
			BoundAgentUsername: "ops",
		},
	}

	testCases := []struct {
		name        string
		handler     func(*Handler, *apptheory.Context) (*apptheory.Response, error)
		path        string
		query       map[string]string
		param       string
		service     *stubSoulHandlerService
		headers     map[string]string
		body        any
		wantStatus  int
		wantCode    string
		wantNoProxy bool
	}{
		{
			name: "missing auth",
			handler: func(h *Handler, ctx *apptheory.Context) (*apptheory.Response, error) {
				return h.HandleListBoundSoulMintConversationsLift(ctx)
			},
			path:        "/api/v1/souls/bound/me/mint-conversations",
			service:     boundService,
			wantStatus:  http.StatusUnauthorized,
			wantNoProxy: true,
		},
		{
			name: "unbound self",
			handler: func(h *Handler, ctx *apptheory.Context) (*apptheory.Response, error) {
				return h.HandleListBoundSoulMintConversationsLift(ctx)
			},
			path:        "/api/v1/souls/bound/me/mint-conversations",
			headers:     map[string]string{"Authorization": "Bearer " + readToken},
			service:     &stubSoulHandlerService{},
			wantStatus:  http.StatusNotFound,
			wantCode:    "SOUL_BOUND_AGENT_NOT_FOUND",
			wantNoProxy: true,
		},
		{
			name: "limit too high",
			handler: func(h *Handler, ctx *apptheory.Context) (*apptheory.Response, error) {
				return h.HandleListBoundSoulMintConversationsLift(ctx)
			},
			path:        "/api/v1/souls/bound/me/mint-conversations",
			headers:     map[string]string{"Authorization": "Bearer " + readToken},
			query:       map[string]string{"limit": "51"},
			service:     boundService,
			wantStatus:  http.StatusBadRequest,
			wantCode:    "SOUL_PRIVATE_LIMIT_INVALID",
			wantNoProxy: true,
		},
		{
			name: "cursor unsupported",
			handler: func(h *Handler, ctx *apptheory.Context) (*apptheory.Response, error) {
				return h.HandleListBoundSoulMintConversationsLift(ctx)
			},
			path:        "/api/v1/souls/bound/me/mint-conversations",
			headers:     map[string]string{"Authorization": "Bearer " + readToken},
			query:       map[string]string{"cursor": "abc"},
			service:     boundService,
			wantStatus:  http.StatusBadRequest,
			wantCode:    "SOUL_PRIVATE_CURSOR_UNSUPPORTED",
			wantNoProxy: true,
		},
		{
			name: "conversation id invalid",
			handler: func(h *Handler, ctx *apptheory.Context) (*apptheory.Response, error) {
				return h.HandleGetBoundSoulMintConversationLift(ctx)
			},
			path:        "/api/v1/souls/bound/me/mint-conversations/bad",
			headers:     map[string]string{"Authorization": "Bearer " + readToken},
			param:       "bad/slash",
			service:     boundService,
			wantStatus:  http.StatusBadRequest,
			wantCode:    "SOUL_PRIVATE_CONVERSATION_ID_INVALID",
			wantNoProxy: true,
		},
		{
			name: "get query unsupported",
			handler: func(h *Handler, ctx *apptheory.Context) (*apptheory.Response, error) {
				return h.HandleGetBoundSoulMintConversationLift(ctx)
			},
			path:        "/api/v1/souls/bound/me/mint-conversations/conv-1",
			headers:     map[string]string{"Authorization": "Bearer " + readToken},
			query:       map[string]string{"agentId": agentID},
			param:       "conv-1",
			service:     boundService,
			wantStatus:  http.StatusBadRequest,
			wantCode:    "SOUL_PRIVATE_QUERY_UNSUPPORTED",
			wantNoProxy: true,
		},
		{
			name: "body rejected",
			handler: func(h *Handler, ctx *apptheory.Context) (*apptheory.Response, error) {
				return h.HandleListBoundSoulMintConversationsLift(ctx)
			},
			path:        "/api/v1/souls/bound/me/mint-conversations",
			headers:     map[string]string{"Authorization": "Bearer " + readToken},
			body:        map[string]string{"agent_id": agentID},
			service:     boundService,
			wantStatus:  http.StatusBadRequest,
			wantCode:    "SOUL_PRIVATE_REQUEST_BODY_UNSUPPORTED",
			wantNoProxy: true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			before := upstreamHits.Load()
			h := &Handler{cfg: cfg, logger: round10TestLogger(t), soulsService: tc.service}
			ctx, err := round10NewLiftContext(http.MethodGet, tc.path, tc.headers, tc.query, tc.body)
			require.NoError(t, err)
			if strings.Contains(tc.path, "/mint-conversations/") {
				ctx.Params["conversationId"] = tc.param
			}

			resp := requireStatus(t, tc.wantStatus)(tc.handler(h, ctx))
			if tc.wantCode != "" {
				require.Equal(t, tc.wantCode, decodeStandardErrorResponse(t, resp).Code)
			}
			if tc.wantNoProxy {
				require.Equal(t, before, upstreamHits.Load())
			}
		})
	}
}

func TestBoundSoulMintConversationHandlers_TranslateHostErrors(t *testing.T) {
	allowLocalLesserHostProxyForTests(t)

	const agentID = "0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"

	cfg := round10TestConfig()
	readToken := round11SignToken(t, cfg.JWTSecret, "ops", []string{auth.ScopeRead}, "sess-private-errors")
	service := &stubSoulHandlerService{
		resolveBoundOut: &soulservice.Soul{
			AgentID:            agentID,
			Domain:             "example.com",
			LocalID:            "ops",
			Status:             "active",
			Bound:              true,
			BoundAgentUsername: "ops",
		},
	}

	testCases := []struct {
		upstreamStatus int
		wantStatus     int
		wantCode       string
	}{
		{upstreamStatus: http.StatusBadRequest, wantStatus: http.StatusBadRequest, wantCode: "SOUL_PRIVATE_INVALID_REQUEST"},
		{upstreamStatus: http.StatusUnauthorized, wantStatus: http.StatusConflict, wantCode: "SOUL_PRIVATE_INSTANCE_TRUST_REJECTED"},
		{upstreamStatus: http.StatusForbidden, wantStatus: http.StatusConflict, wantCode: "SOUL_PRIVATE_INSTANCE_TRUST_REJECTED"},
		{upstreamStatus: http.StatusNotFound, wantStatus: http.StatusNotFound, wantCode: "SOUL_PRIVATE_CONVERSATION_NOT_FOUND"},
		{upstreamStatus: http.StatusRequestEntityTooLarge, wantStatus: http.StatusRequestEntityTooLarge, wantCode: "SOUL_PRIVATE_RESPONSE_TOO_LARGE"},
		{upstreamStatus: http.StatusTooManyRequests, wantStatus: http.StatusTooManyRequests, wantCode: "SOUL_PRIVATE_RATE_LIMITED"},
		{upstreamStatus: http.StatusInternalServerError, wantStatus: http.StatusServiceUnavailable, wantCode: "SOUL_PRIVATE_HOST_UNAVAILABLE"},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(http.StatusText(tc.upstreamStatus), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tc.upstreamStatus == http.StatusTooManyRequests {
					w.Header().Set("Retry-After", "7")
				}
				w.WriteHeader(tc.upstreamStatus)
				_, _ = w.Write([]byte(`{"error":{"message":"private secret should not leak"}}`))
			}))
			t.Cleanup(upstream.Close)

			cfg := *cfg
			cfg.LesserHostURL = upstream.URL
			cfg.LesserHostInstanceKey = "instance-key-raw"
			h := &Handler{cfg: &cfg, logger: round10TestLogger(t), soulsService: service}
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/souls/bound/me/mint-conversations/conv-err", map[string]string{
				"Authorization": "Bearer " + readToken,
			}, nil, nil)
			require.NoError(t, err)
			ctx.Params["conversationId"] = "conv-err"

			resp := requireStatus(t, tc.wantStatus)(h.HandleGetBoundSoulMintConversationLift(ctx))
			body := decodeStandardErrorResponse(t, resp)
			require.Equal(t, tc.wantCode, body.Code)
			require.NotContains(t, string(resp.Body), "private secret")
			if tc.upstreamStatus == http.StatusTooManyRequests {
				require.Equal(t, []string{"7"}, resp.Headers["retry-after"])
			}
		})
	}
}

func TestBoundSoulMintConversationHandlers_FailClosedOnHostIdentityMismatch(t *testing.T) {
	allowLocalLesserHostProxyForTests(t)

	const (
		agentID        = "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
		otherAgentID   = "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
		conversationID = "conv-mismatch"
	)

	cfg := round10TestConfig()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"version":"1",
			"conversation":{
				"agent_id":"` + otherAgentID + `",
				"conversation_id":"` + conversationID + `",
				"model":"gpt-test",
				"messages":"private secret should not leak",
				"status":"completed",
				"created_at":"2026-05-11T21:00:00Z"
			}
		}`))
	}))
	t.Cleanup(upstream.Close)
	cfg.LesserHostURL = upstream.URL
	cfg.LesserHostInstanceKey = "instance-key-raw"

	service := &stubSoulHandlerService{
		resolveBoundOut: &soulservice.Soul{
			AgentID:            agentID,
			Domain:             "example.com",
			LocalID:            "ops",
			Status:             "active",
			Bound:              true,
			BoundAgentUsername: "ops",
		},
	}
	h := &Handler{cfg: cfg, logger: round10TestLogger(t), soulsService: service}
	readToken := round11SignToken(t, cfg.JWTSecret, "ops", []string{auth.ScopeRead}, "sess-private-mismatch")

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/souls/bound/me/mint-conversations/"+conversationID, map[string]string{
		"Authorization": "Bearer " + readToken,
	}, nil, nil)
	require.NoError(t, err)
	ctx.Params["conversationId"] = conversationID

	resp := requireStatus(t, http.StatusServiceUnavailable)(h.HandleGetBoundSoulMintConversationLift(ctx))
	require.Equal(t, "SOUL_PRIVATE_UPSTREAM_SCOPE_MISMATCH", decodeStandardErrorResponse(t, resp).Code)
	require.NotContains(t, string(resp.Body), "private secret")
}

func TestBoundSoulMintConversationHandlers_AdditionalFailureCoverage(t *testing.T) {
	allowLocalLesserHostProxyForTests(t)

	const (
		agentID      = "0x1111111111111111111111111111111111111111111111111111111111111111"
		otherAgentID = "0x2222222222222222222222222222222222222222222222222222222222222222"
	)

	cfg := round10TestConfig()
	cfg.LesserHostInstanceKey = "instance-key-raw"
	readToken := round11SignToken(t, cfg.JWTSecret, "ops", []string{auth.ScopeRead}, "sess-private-extra")
	service := &stubSoulHandlerService{
		resolveBoundOut: &soulservice.Soul{
			AgentID:            agentID,
			Domain:             "example.com",
			LocalID:            "ops",
			Status:             "active",
			Bound:              true,
			BoundAgentUsername: "ops",
		},
	}

	t.Run("list invalid host json", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{`))
		}))
		t.Cleanup(upstream.Close)

		cfg := *cfg
		cfg.LesserHostURL = upstream.URL
		h := &Handler{cfg: &cfg, logger: round10TestLogger(t), soulsService: service}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/souls/bound/me/mint-conversations", map[string]string{
			"Authorization": "Bearer " + readToken,
		}, nil, nil)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusServiceUnavailable)(h.HandleListBoundSoulMintConversationsLift(ctx))
		require.Equal(t, "SOUL_PRIVATE_HOST_RESPONSE_INVALID", decodeStandardErrorResponse(t, resp).Code)
	})

	t.Run("list scope mismatch", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{
				"version":"1",
				"conversations":[{
					"agent_id":"` + otherAgentID + `",
					"conversation_id":"conv-list-mismatch",
					"status":"completed",
					"created_at":"2026-05-11T21:00:00Z"
				}],
				"count":1,
				"limit":20
			}`))
		}))
		t.Cleanup(upstream.Close)

		cfg := *cfg
		cfg.LesserHostURL = upstream.URL
		h := &Handler{cfg: &cfg, logger: round10TestLogger(t), soulsService: service}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/souls/bound/me/mint-conversations", map[string]string{
			"Authorization": "Bearer " + readToken,
		}, nil, nil)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusServiceUnavailable)(h.HandleListBoundSoulMintConversationsLift(ctx))
		require.Equal(t, "SOUL_PRIVATE_UPSTREAM_SCOPE_MISMATCH", decodeStandardErrorResponse(t, resp).Code)
	})

	t.Run("get invalid host json", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{`))
		}))
		t.Cleanup(upstream.Close)

		cfg := *cfg
		cfg.LesserHostURL = upstream.URL
		h := &Handler{cfg: &cfg, logger: round10TestLogger(t), soulsService: service}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/souls/bound/me/mint-conversations/conv-extra", map[string]string{
			"Authorization": "Bearer " + readToken,
		}, nil, nil)
		require.NoError(t, err)
		ctx.Params["conversationId"] = "conv-extra"

		resp := requireStatus(t, http.StatusServiceUnavailable)(h.HandleGetBoundSoulMintConversationLift(ctx))
		require.Equal(t, "SOUL_PRIVATE_HOST_RESPONSE_INVALID", decodeStandardErrorResponse(t, resp).Code)
	})

	t.Run("host response too large", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(strings.Repeat("x", soulPrivateMintConversationListMaxBytes+1)))
		}))
		t.Cleanup(upstream.Close)

		cfg := *cfg
		cfg.LesserHostURL = upstream.URL
		h := &Handler{cfg: &cfg, logger: round10TestLogger(t), soulsService: service}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/souls/bound/me/mint-conversations", map[string]string{
			"Authorization": "Bearer " + readToken,
		}, nil, nil)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusRequestEntityTooLarge)(h.HandleListBoundSoulMintConversationsLift(ctx))
		require.Equal(t, "SOUL_PRIVATE_RESPONSE_TOO_LARGE", decodeStandardErrorResponse(t, resp).Code)
	})
}

func TestSoulPrivateMintConversationResolveBoundAgentFailures(t *testing.T) {
	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/souls/bound/me/mint-conversations", nil, nil, nil)
	require.NoError(t, err)

	testCases := []struct {
		name       string
		handler    *Handler
		wantStatus int
		wantCode   string
	}{
		{
			name:       "service nil",
			handler:    &Handler{logger: round10TestLogger(t)},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "trust not configured",
			handler:    &Handler{logger: round10TestLogger(t), soulsService: &stubSoulHandlerService{resolveBoundErr: soulservice.ErrTrustNotConfigured}},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "SOUL_PRIVATE_TRUST_NOT_CONFIGURED",
		},
		{
			name:       "soul not available",
			handler:    &Handler{logger: round10TestLogger(t), soulsService: &stubSoulHandlerService{resolveBoundErr: soulservice.ErrSoulNotAvailable}},
			wantStatus: http.StatusNotFound,
			wantCode:   "SOUL_BOUND_AGENT_NOT_AVAILABLE",
		},
		{
			name:       "unexpected service error",
			handler:    &Handler{logger: round10TestLogger(t), soulsService: &stubSoulHandlerService{resolveBoundErr: errors.New("boom")}},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "not bound",
			handler:    &Handler{logger: round10TestLogger(t), soulsService: &stubSoulHandlerService{resolveBoundOut: &soulservice.Soul{AgentID: "", Bound: false}}},
			wantStatus: http.StatusNotFound,
			wantCode:   "SOUL_BOUND_AGENT_NOT_FOUND",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, resp := tc.handler.resolveSoulPrivateBoundAgent(ctx, "ops")
			require.NotNil(t, resp)
			require.Equal(t, tc.wantStatus, resp.Status)
			if tc.wantCode != "" {
				require.Equal(t, tc.wantCode, decodeStandardErrorResponse(t, resp).Code)
			}
		})
	}
}

func TestSoulPrivateHostReadConfigurationFailures(t *testing.T) {
	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/souls/bound/me/mint-conversations", nil, nil, nil)
	require.NoError(t, err)

	t.Run("missing base url", func(t *testing.T) {
		cfg := round10TestConfig()
		cfg.LesserHostURL = ""
		cfg.LesserHostInstanceKey = "instance-key-raw"
		h := &Handler{cfg: cfg, logger: round10TestLogger(t)}

		_, resp := h.getSoulPrivateHostRead(ctx, "agent", "", nil, 1024)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusUnprocessableEntity, resp.Status)
		require.Equal(t, "SOUL_PRIVATE_TRUST_NOT_CONFIGURED", decodeStandardErrorResponse(t, resp).Code)
	})

	t.Run("invalid base url", func(t *testing.T) {
		cfg := round10TestConfig()
		cfg.LesserHostURL = "://bad"
		cfg.LesserHostInstanceKey = "instance-key-raw"
		h := &Handler{cfg: cfg, logger: round10TestLogger(t)}

		_, resp := h.getSoulPrivateHostRead(ctx, "agent", "", nil, 1024)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusUnprocessableEntity, resp.Status)
		require.Equal(t, "SOUL_PRIVATE_TRUST_NOT_CONFIGURED", decodeStandardErrorResponse(t, resp).Code)
	})

	t.Run("missing instance key", func(t *testing.T) {
		cfg := round10TestConfig()
		cfg.LesserHostURL = "https://lesser-host.example"
		cfg.LesserHostInstanceKey = ""
		h := &Handler{cfg: cfg, logger: round10TestLogger(t)}

		_, resp := h.getSoulPrivateHostRead(ctx, "agent", "", nil, 1024)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusUnprocessableEntity, resp.Status)
		require.Equal(t, "SOUL_PRIVATE_TRUST_NOT_CONFIGURED", decodeStandardErrorResponse(t, resp).Code)
	})

	t.Run("proxy url validation rejected", func(t *testing.T) {
		prevValidate := validateLesserHostProxyURL
		validateLesserHostProxyURL = func(*url.URL) error { return errors.New("blocked") }
		t.Cleanup(func() { validateLesserHostProxyURL = prevValidate })

		cfg := round10TestConfig()
		cfg.LesserHostURL = "https://lesser-host.example"
		cfg.LesserHostInstanceKey = "instance-key-raw"
		h := &Handler{cfg: cfg, logger: round10TestLogger(t)}

		_, resp := h.getSoulPrivateHostRead(ctx, "agent", "", nil, 1024)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusServiceUnavailable, resp.Status)
		require.Equal(t, "SOUL_PRIVATE_HOST_UNAVAILABLE", decodeStandardErrorResponse(t, resp).Code)
	})

	t.Run("client error", func(t *testing.T) {
		prevValidate := validateLesserHostProxyURL
		prevClient := newLesserHostProxyClient
		validateLesserHostProxyURL = func(*url.URL) error { return nil }
		newLesserHostProxyClient = func() lesserHostProxyHTTPClient {
			return soulPrivateHostReadClientFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("dial failed")
			})
		}
		t.Cleanup(func() {
			validateLesserHostProxyURL = prevValidate
			newLesserHostProxyClient = prevClient
		})

		cfg := round10TestConfig()
		cfg.LesserHostURL = "https://lesser-host.example"
		cfg.LesserHostInstanceKey = "instance-key-raw"
		h := &Handler{cfg: cfg, logger: round10TestLogger(t)}

		_, resp := h.getSoulPrivateHostRead(ctx, "agent", "", nil, 1024)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusServiceUnavailable, resp.Status)
		require.Equal(t, "SOUL_PRIVATE_HOST_UNAVAILABLE", decodeStandardErrorResponse(t, resp).Code)
	})

	t.Run("response read error", func(t *testing.T) {
		prevValidate := validateLesserHostProxyURL
		prevClient := newLesserHostProxyClient
		validateLesserHostProxyURL = func(*url.URL) error { return nil }
		newLesserHostProxyClient = func() lesserHostProxyHTTPClient {
			return soulPrivateHostReadClientFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{},
					Body:       soulPrivateFailingBody{},
				}, nil
			})
		}
		t.Cleanup(func() {
			validateLesserHostProxyURL = prevValidate
			newLesserHostProxyClient = prevClient
		})

		cfg := round10TestConfig()
		cfg.LesserHostURL = "https://lesser-host.example"
		cfg.LesserHostInstanceKey = "instance-key-raw"
		h := &Handler{cfg: cfg, logger: round10TestLogger(t)}

		_, resp := h.getSoulPrivateHostRead(ctx, "agent", "", nil, 1024)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusServiceUnavailable, resp.Status)
		require.Equal(t, "SOUL_PRIVATE_HOST_UNAVAILABLE", decodeStandardErrorResponse(t, resp).Code)
	})

	t.Run("success direct helper preserves status and body", func(t *testing.T) {
		prevValidate := validateLesserHostProxyURL
		prevClient := newLesserHostProxyClient
		validateLesserHostProxyURL = func(*url.URL) error { return nil }
		newLesserHostProxyClient = func() lesserHostProxyHTTPClient {
			return soulPrivateHostReadClientFunc(func(req *http.Request) (*http.Response, error) {
				require.Equal(t, "Bearer instance-key-raw", req.Header.Get("Authorization"))
				require.Contains(t, req.URL.Path, "/mint-conversations/conv-direct")
				return &http.Response{
					StatusCode: http.StatusAccepted,
					Header:     http.Header{"X-Test": []string{"ok"}},
					Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
				}, nil
			})
		}
		t.Cleanup(func() {
			validateLesserHostProxyURL = prevValidate
			newLesserHostProxyClient = prevClient
		})

		cfg := round10TestConfig()
		cfg.LesserHostURL = "https://lesser-host.example"
		cfg.LesserHostInstanceKey = "instance-key-raw"
		h := &Handler{cfg: cfg, logger: round10TestLogger(t)}

		result, resp := h.getSoulPrivateHostRead(ctx, "agent", "conv-direct", nil, 1024)
		require.Nil(t, resp)
		require.Equal(t, http.StatusAccepted, result.status)
		require.Equal(t, []byte(`{"ok":true}`), result.body)
		require.Equal(t, "ok", result.headers.Get("x-test"))
	})
}

func TestSoulPrivateMintConversationUtilityBranches(t *testing.T) {
	require.Empty(t, firstQueryValue(nil, "limit"))
	require.False(t, soulPrivateCursorPresent(nil))
	require.Empty(t, soulPrivateHash(""))
	require.Empty(t, soulPrivateErrorCode(nil))
	require.Empty(t, soulPrivateErrorCode(&apptheory.Response{Body: []byte(`not-json`)}))
	require.Equal(t, "list_mint_conversations", soulPrivateRouteClass(""))
	require.Equal(t, "get_mint_conversation", soulPrivateRouteClass("conv-1"))

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/souls/bound/me/mint-conversations/", nil, nil, nil)
	require.NoError(t, err)
	conversationID, resp := validateSoulPrivateConversationID(ctx)
	require.Empty(t, conversationID)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusBadRequest, resp.Status)
	require.Equal(t, "SOUL_PRIVATE_CONVERSATION_ID_REQUIRED", decodeStandardErrorResponse(t, resp).Code)

	var nilHandler *Handler
	nilHandler.logSoulPrivateRead(ctx, "ops", "agent", "conv-1", "get_mint_conversation", "success", http.StatusOK, 0, 0, false, "", http.StatusOK, time.Now())
	(&Handler{}).logSoulPrivateRead(ctx, "ops", "agent", "conv-1", "get_mint_conversation", "success", http.StatusOK, 0, 0, false, "", http.StatusOK, time.Now())
}

func TestHandleIncorporateSoulLift_MapsServiceResponses(t *testing.T) {
	t.Parallel()

	const agentID = "0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	successENS := "alpha.eth"
	targetAgentUsername := "agent-alpha"

	cfg := round10TestConfig()
	token := round11SignToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite}, "sess-2")

	testCases := []struct {
		name          string
		service       *stubSoulHandlerService
		wantStatus    int
		wantAvailable bool
	}{
		{
			name: "success",
			service: &stubSoulHandlerService{
				incorporateOut: &soulservice.Soul{
					AgentID:               agentID,
					Domain:                "example.com",
					LocalID:               "alpha",
					ENSName:               &successENS,
					Wallet:                "0x1111111111111111111111111111111111111111",
					PrincipalAddress:      "0x1111111111111111111111111111111111111111",
					Status:                "active",
					Bound:                 true,
					BoundAgentUsername:    targetAgentUsername,
					BoundPrincipalAddress: "0x1111111111111111111111111111111111111111",
					BoundAt:               time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC),
					BoundUpdatedAt:        time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC),
				},
			},
			wantStatus:    http.StatusOK,
			wantAvailable: false,
		},
		{
			name:       "trust not configured",
			service:    &stubSoulHandlerService{incorporateErr: soulservice.ErrTrustNotConfigured},
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name:       "soul not available",
			service:    &stubSoulHandlerService{incorporateErr: soulservice.ErrSoulNotAvailable},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "soul already bound",
			service:    &stubSoulHandlerService{incorporateErr: soulservice.ErrSoulAlreadyBound},
			wantStatus: http.StatusConflict,
		},
		{
			name:       "target agent already has soul",
			service:    &stubSoulHandlerService{incorporateErr: soulservice.ErrTargetAgentAlreadyHasSoul},
			wantStatus: http.StatusConflict,
		},
		{
			name:       "internal error",
			service:    &stubSoulHandlerService{incorporateErr: errors.New("boom")},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := &Handler{cfg: cfg, logger: round10TestLogger(t), soulsService: tc.service}
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/souls/"+agentID+"/incorporate", map[string]string{
				"Authorization": "Bearer " + token,
			}, nil, apimodels.SoulIncorporateRequest{TargetAgentUsername: targetAgentUsername})
			require.NoError(t, err)
			ctx.Params["agentId"] = agentID

			resp := requireStatus(t, tc.wantStatus)(h.HandleIncorporateSoulLift(ctx))
			if tc.wantStatus != http.StatusOK {
				return
			}
			require.Contains(t, string(resp.Body), "\"ens_name\":\"alpha.eth\"")

			var body apimodels.SoulIncorporateResponse
			require.NoError(t, json.Unmarshal(resp.Body, &body))
			require.Equal(t, "alice", tc.service.lastUsername)
			require.Equal(t, targetAgentUsername, tc.service.lastTargetAgent)
			require.Equal(t, agentID, tc.service.lastSoulAgentID)
			require.Equal(t, agentID, body.Soul.Agent.AgentID)
			require.NotNil(t, body.Soul.Agent.ENSName)
			require.Equal(t, successENS, *body.Soul.Agent.ENSName)
			require.False(t, body.Soul.AvailableForIncorporation)
			require.NotNil(t, body.Soul.Binding)
			require.Equal(t, targetAgentUsername, body.Soul.Binding.AgentUsername)
			require.Equal(t, tc.wantAvailable, body.Soul.AvailableForIncorporation)
		})
	}
}

func TestHandleIncorporateSoulLift_RequiresAgentID(t *testing.T) {
	t.Parallel()

	cfg := round10TestConfig()
	h := &Handler{cfg: cfg, logger: round10TestLogger(t), soulsService: &stubSoulHandlerService{}}
	token := round11SignToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite}, "sess-3")

	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/souls//incorporate", map[string]string{
		"Authorization": "Bearer " + token,
	}, nil, apimodels.SoulIncorporateRequest{TargetAgentUsername: "agent-alpha"})
	require.NoError(t, err)

	requireStatus(t, http.StatusBadRequest)(h.HandleIncorporateSoulLift(ctx))
}

func TestHandleIncorporateSoulLift_AuthAndServiceGuards(t *testing.T) {
	t.Parallel()

	cfg := round10TestConfig()
	writeToken := round11SignToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite}, "sess-write")
	readToken := round11SignToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead}, "sess-read-only")
	agentID := "0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"

	testCases := []struct {
		name       string
		headers    map[string]string
		handler    *Handler
		wantStatus int
	}{
		{
			name:       "missing token",
			headers:    nil,
			handler:    &Handler{cfg: cfg, logger: round10TestLogger(t), soulsService: &stubSoulHandlerService{}},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "insufficient scope",
			headers: map[string]string{
				"Authorization": "Bearer " + readToken,
			},
			handler:    &Handler{cfg: cfg, logger: round10TestLogger(t), soulsService: &stubSoulHandlerService{}},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "service unavailable",
			headers: map[string]string{
				"Authorization": "Bearer " + writeToken,
			},
			handler:    &Handler{cfg: cfg, logger: round10TestLogger(t)},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/souls/"+agentID+"/incorporate", tc.headers, nil, apimodels.SoulIncorporateRequest{
				TargetAgentUsername: "agent-alpha",
			})
			require.NoError(t, err)
			ctx.Params["agentId"] = agentID

			requireStatus(t, tc.wantStatus)(tc.handler.HandleIncorporateSoulLift(ctx))
		})
	}
}

func TestHandleIncorporateSoulLift_AuthFailures(t *testing.T) {
	t.Parallel()

	cfg := round10TestConfig()
	h := &Handler{
		cfg:          cfg,
		logger:       round10TestLogger(t),
		soulsService: &stubSoulHandlerService{},
	}
	agentID := "0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"

	t.Run("missing token returns unauthorized", func(t *testing.T) {
		t.Parallel()

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/souls/"+agentID+"/incorporate", nil, nil, apimodels.SoulIncorporateRequest{
			TargetAgentUsername: "agent-alpha",
		})
		require.NoError(t, err)
		ctx.Params["agentId"] = agentID

		requireStatus(t, http.StatusUnauthorized)(h.HandleIncorporateSoulLift(ctx))
	})

	t.Run("read-only token returns forbidden", func(t *testing.T) {
		t.Parallel()

		token := round11SignToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead}, "sess-incorporate-forbidden")
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/souls/"+agentID+"/incorporate", map[string]string{
			"Authorization": "Bearer " + token,
		}, nil, apimodels.SoulIncorporateRequest{
			TargetAgentUsername: "agent-alpha",
		})
		require.NoError(t, err)
		ctx.Params["agentId"] = agentID

		requireStatus(t, http.StatusForbidden)(h.HandleIncorporateSoulLift(ctx))
	})
}

func TestHandleIncorporateSoulLift_RequiresTargetAgentUsername(t *testing.T) {
	t.Parallel()

	cfg := round10TestConfig()
	h := &Handler{cfg: cfg, logger: round10TestLogger(t), soulsService: &stubSoulHandlerService{}}
	token := round11SignToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite}, "sess-target-agent")
	agentID := "0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"

	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/souls/"+agentID+"/incorporate", map[string]string{
		"Authorization": "Bearer " + token,
	}, nil, apimodels.SoulIncorporateRequest{})
	require.NoError(t, err)
	ctx.Params["agentId"] = agentID

	requireStatus(t, http.StatusBadRequest)(h.HandleIncorporateSoulLift(ctx))
}

func TestHandleIncorporateSoulLift_RejectsInvalidRequestBody(t *testing.T) {
	t.Parallel()

	cfg := round10TestConfig()
	h := &Handler{cfg: cfg, logger: round10TestLogger(t), soulsService: &stubSoulHandlerService{}}
	token := round11SignToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite}, "sess-invalid-body")
	agentID := "0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"

	ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/souls/"+agentID+"/incorporate", map[string]string{
		"Authorization": "Bearer " + token,
	}, nil, []byte("{"))
	ctx.Params["agentId"] = agentID

	requireStatus(t, http.StatusBadRequest)(h.HandleIncorporateSoulLift(ctx))
}

func TestHandleIncorporateSoulLift_RejectsInvalidTargetAgentUsername(t *testing.T) {
	t.Parallel()

	cfg := round10TestConfig()
	h := &Handler{cfg: cfg, logger: round10TestLogger(t), soulsService: &stubSoulHandlerService{}}
	token := round11SignToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite}, "sess-invalid-target-agent")
	agentID := "0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"

	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/souls/"+agentID+"/incorporate", map[string]string{
		"Authorization": "Bearer " + token,
	}, nil, apimodels.SoulIncorporateRequest{TargetAgentUsername: "not valid"})
	require.NoError(t, err)
	ctx.Params["agentId"] = agentID

	requireStatus(t, http.StatusBadRequest)(h.HandleIncorporateSoulLift(ctx))
}

func TestHandleIncorporateSoulLift_ReturnsInternalWhenServiceUnavailable(t *testing.T) {
	t.Parallel()

	cfg := round10TestConfig()
	h := &Handler{cfg: cfg, logger: round10TestLogger(t)}
	token := round11SignToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite}, "sess-incorporate-no-service")
	agentID := "0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"

	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/souls/"+agentID+"/incorporate", map[string]string{
		"Authorization": "Bearer " + token,
	}, nil, apimodels.SoulIncorporateRequest{TargetAgentUsername: "agent-alpha"})
	require.NoError(t, err)
	ctx.Params["agentId"] = agentID

	requireStatus(t, http.StatusInternalServerError)(h.HandleIncorporateSoulLift(ctx))
}

func TestHandleCreateSoulBindingLift_BindsWithDedicatedIntegrationCredential(t *testing.T) {
	t.Parallel()

	const (
		integrationKey = "body-to-lesser-binding-key"
		agentID        = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)

	cfg := round10TestConfig()
	cfg.SoulBindingIntegrationKey = integrationKey
	service := &stubSoulHandlerService{bindOut: soulBindingHandlerProjection(agentID, "drone-ada", false)}
	h := &Handler{cfg: cfg, logger: round10TestLogger(t), soulsService: service}

	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/souls/bindings", map[string]string{
		"Authorization":   "Bearer " + integrationKey,
		"Idempotency-Key": "bind-key-1",
	}, nil, apimodels.SoulBindingRequest{
		ActorUsername:      "drone-ada",
		SoulAgentID:        agentID,
		BodyActorID:        "body://ptah/drone-ada",
		HostRegistrationID: "hreg_01JZPTHOSTREG",
		HostConversationID: "hconv_01JZPTHOSTCONV",
		AuthorityModel:     soulservice.SoulAuthorityModelInstanceTrust,
		AnchorState:        soulservice.SoulAnchorStateHostedOffchain,
		OperationalBinding: soulservice.SoulOperationalBindingHostedBound,
		PrincipalAddress:   "0x2222222222222222222222222222222222222222",
		Evidence: apimodels.SoulBindingEvidence{
			Source:          "ptah",
			HostRequestID:   "hreq_01JZPTHOSTREQ",
			DeclarationHash: "sha256:4c5835f5c2c84bcaadc17af3c5a5700fdd7f39fb7f61305b02d1a02a0e6c7c56",
			IssuedAt:        "2026-07-14T16:20:00Z",
		},
	})
	require.NoError(t, err)

	resp := requireStatus(t, http.StatusOK)(h.HandleCreateSoulBindingLift(ctx))

	var body apimodels.SoulBindingResponse
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	require.Equal(t, "1", body.Version)
	require.Equal(t, "bound", body.Status)
	require.Equal(t, "bound", body.BindingState)
	require.Equal(t, agentID, body.Agent.AgentID)
	require.Equal(t, "example.com", body.Agent.Domain)
	require.Equal(t, "drone-ada", body.Agent.LocalID)
	require.Equal(t, soulservice.SoulAuthorityModelInstanceTrust, body.Agent.AuthorityModel)
	require.Equal(t, soulservice.SoulAnchorStateHostedOffchain, body.Agent.AnchorState)
	require.Equal(t, soulservice.SoulOperationalBindingHostedBound, body.Agent.OperationalBinding)
	require.Equal(t, "active", body.Agent.LifecycleStatus)
	require.Equal(t, 3, body.Agent.PublishedVersion)
	require.Equal(t, "drone-ada", body.Binding.AgentUsername)
	require.Equal(t, "0x1111111111111111111111111111111111111111", body.Binding.PrincipalAddress)
	require.NotNil(t, body.Idempotency)
	require.Equal(t, "bind-key-1", body.Idempotency.Key)
	require.False(t, body.Idempotency.Replayed)
	require.Equal(t, "sha256:handler-payload", body.Idempotency.PayloadHash)
	require.NotNil(t, body.Links)
	require.Equal(t, "/api/v1/souls/bindings/"+agentID, body.Links.Status)

	require.Equal(t, soulBindingIntegrationCallerID, service.lastBindInput.CallerID)
	require.Equal(t, "bind-key-1", service.lastBindInput.IdempotencyKey)
	require.Equal(t, "drone-ada", service.lastBindInput.ActorUsername)
	require.Equal(t, agentID, service.lastBindInput.SoulAgentID)
	require.Equal(t, "body://ptah/drone-ada", service.lastBindInput.BodyActorID)
	require.Equal(t, "hreg_01JZPTHOSTREG", service.lastBindInput.HostRegistrationID)
	require.Equal(t, "hconv_01JZPTHOSTCONV", service.lastBindInput.HostConversationID)
	require.Equal(t, soulservice.SoulAuthorityModelInstanceTrust, service.lastBindInput.AuthorityModel)
	require.Equal(t, "0x2222222222222222222222222222222222222222", service.lastBindInput.PrincipalAddressHint)
	require.Equal(t, "ptah", service.lastBindInput.Evidence.Source)
}

func TestHandleCreateSoulBindingLift_RejectsOrdinaryOAuthAndBadInputs(t *testing.T) {
	t.Parallel()

	const (
		integrationKey = "body-to-lesser-binding-key"
		agentID        = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)

	cfg := round10TestConfig()
	cfg.SoulBindingIntegrationKey = integrationKey
	oauthToken := round11SignToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite}, "ordinary-user")

	tests := []struct {
		name       string
		headers    map[string]string
		body       any
		wantStatus int
		wantCode   string
	}{
		{
			name:       "missing credential",
			headers:    nil,
			body:       apimodels.SoulBindingRequest{ActorUsername: "drone-ada", SoulAgentID: agentID},
			wantStatus: http.StatusUnauthorized,
			wantCode:   "SOUL_BINDING_AUTH_REQUIRED",
		},
		{
			name: "ordinary oauth token is not accepted",
			headers: map[string]string{
				"Authorization":   "Bearer " + oauthToken,
				"Idempotency-Key": "bind-key-ordinary",
			},
			body:       apimodels.SoulBindingRequest{ActorUsername: "drone-ada", SoulAgentID: agentID},
			wantStatus: http.StatusUnauthorized,
			wantCode:   "SOUL_BINDING_AUTH_REQUIRED",
		},
		{
			name: "missing idempotency key",
			headers: map[string]string{
				"Authorization": "Bearer " + integrationKey,
			},
			body:       apimodels.SoulBindingRequest{ActorUsername: "drone-ada", SoulAgentID: agentID},
			wantStatus: http.StatusBadRequest,
			wantCode:   "SOUL_BINDING_IDEMPOTENCY_KEY_REQUIRED",
		},
		{
			name: "service is not reached without dedicated credential",
			headers: map[string]string{
				"Authorization":   "Bearer wrong-key",
				"Idempotency-Key": "bind-key-wrong",
			},
			body:       apimodels.SoulBindingRequest{ActorUsername: "drone-ada", SoulAgentID: agentID},
			wantStatus: http.StatusUnauthorized,
			wantCode:   "SOUL_BINDING_AUTH_REQUIRED",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			service := &stubSoulHandlerService{bindOut: soulBindingHandlerProjection(agentID, "drone-ada", false)}
			h := &Handler{cfg: cfg, logger: round10TestLogger(t), soulsService: service}
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/souls/bindings", tc.headers, nil, tc.body)
			require.NoError(t, err)

			resp := requireStatus(t, tc.wantStatus)(h.HandleCreateSoulBindingLift(ctx))
			require.Equal(t, tc.wantCode, decodeStandardErrorResponse(t, resp).Code)
			require.Empty(t, service.lastBindInput.CallerID, "handler must not call service after auth/input rejection")
		})
	}

	t.Run("invalid body", func(t *testing.T) {
		t.Parallel()

		service := &stubSoulHandlerService{bindOut: soulBindingHandlerProjection(agentID, "drone-ada", false)}
		h := &Handler{cfg: cfg, logger: round10TestLogger(t), soulsService: service}
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/souls/bindings", map[string]string{
			"Authorization":   "Bearer " + integrationKey,
			"Idempotency-Key": "bind-key-invalid",
		}, nil, []byte("{"))

		resp := requireStatus(t, http.StatusBadRequest)(h.HandleCreateSoulBindingLift(ctx))
		require.Equal(t, "SOUL_BINDING_INVALID_REQUEST", decodeStandardErrorResponse(t, resp).Code)
		require.Empty(t, service.lastBindInput.CallerID)
	})
}

func TestHandleSoulBindingLift_MapsServiceProjectionAndErrors(t *testing.T) {
	t.Parallel()

	const (
		integrationKey = "body-to-lesser-binding-key"
		agentID        = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	)

	cfg := round10TestConfig()
	cfg.SoulBindingIntegrationKey = integrationKey

	t.Run("get returns status projection without idempotency block", func(t *testing.T) {
		t.Parallel()

		service := &stubSoulHandlerService{getBindingOut: soulBindingHandlerProjection(agentID, "drone-ada", false)}
		h := &Handler{cfg: cfg, logger: round10TestLogger(t), soulsService: service}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/souls/bindings/"+agentID, map[string]string{
			"Authorization": "Bearer " + integrationKey,
		}, map[string]string{"actor_username": "drone-ada"}, nil)
		require.NoError(t, err)
		ctx.Params["agentId"] = agentID

		resp := requireStatus(t, http.StatusOK)(h.HandleGetSoulBindingLift(ctx))

		var body apimodels.SoulBindingResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "bound", body.Status)
		require.Equal(t, agentID, body.Agent.AgentID)
		require.Nil(t, body.Idempotency)
		require.Nil(t, body.Links)
		require.Equal(t, agentID, service.lastGetAgentID)
		require.Equal(t, "drone-ada", service.lastTargetAgent)
	})

	t.Run("actor mismatch maps to stable conflict code", func(t *testing.T) {
		t.Parallel()

		service := &stubSoulHandlerService{getBindingErr: soulservice.ErrSoulBindingActorMismatch}
		h := &Handler{cfg: cfg, logger: round10TestLogger(t), soulsService: service}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/souls/bindings/"+agentID, map[string]string{
			"Authorization": "Bearer " + integrationKey,
		}, map[string]string{"actor_username": "wrong-agent"}, nil)
		require.NoError(t, err)
		ctx.Params["agentId"] = agentID

		resp := requireStatus(t, http.StatusConflict)(h.HandleGetSoulBindingLift(ctx))
		require.Equal(t, "SOUL_BINDING_ACTOR_MISMATCH", decodeStandardErrorResponse(t, resp).Code)
	})

	t.Run("host unavailable fails closed", func(t *testing.T) {
		t.Parallel()

		service := &stubSoulHandlerService{getBindingErr: soulservice.ErrSoulBindingHostRegistryUnavailable}
		h := &Handler{cfg: cfg, logger: round10TestLogger(t), soulsService: service}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/souls/bindings/"+agentID, map[string]string{
			"Authorization": "Bearer " + integrationKey,
		}, nil, nil)
		require.NoError(t, err)
		ctx.Params["agentId"] = agentID

		resp := requireStatus(t, http.StatusServiceUnavailable)(h.HandleGetSoulBindingLift(ctx))
		require.Equal(t, "SOUL_BINDING_HOST_REGISTRY_UNAVAILABLE", decodeStandardErrorResponse(t, resp).Code)
	})

	t.Run("write conflict maps body conflict code", func(t *testing.T) {
		t.Parallel()

		service := &stubSoulHandlerService{bindErr: soulservice.ErrTargetAgentAlreadyHasSoul}
		h := &Handler{cfg: cfg, logger: round10TestLogger(t), soulsService: service}
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/souls/bindings", map[string]string{
			"Authorization":   "Bearer " + integrationKey,
			"Idempotency-Key": "bind-key-conflict",
		}, nil, apimodels.SoulBindingRequest{ActorUsername: "drone-ada", SoulAgentID: agentID})
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusConflict)(h.HandleCreateSoulBindingLift(ctx))
		require.Equal(t, "SOUL_BINDING_BODY_CONFLICT", decodeStandardErrorResponse(t, resp).Code)
	})
}

func TestHandleSoulBindingLift_FailsClosedWhenIntegrationCredentialMissing(t *testing.T) {
	t.Parallel()

	cfg := round10TestConfig()
	cfg.SoulBindingIntegrationKey = ""
	h := &Handler{cfg: cfg, logger: round10TestLogger(t), soulsService: &stubSoulHandlerService{}}

	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/souls/bindings", map[string]string{
		"Authorization":   "Bearer any-key",
		"Idempotency-Key": "bind-key",
	}, nil, apimodels.SoulBindingRequest{
		ActorUsername: "drone-ada",
		SoulAgentID:   "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	})
	require.NoError(t, err)

	resp := requireStatus(t, http.StatusServiceUnavailable)(h.HandleCreateSoulBindingLift(ctx))
	require.Equal(t, "SOUL_BINDING_INTERNAL", decodeStandardErrorResponse(t, resp).Code)
}

func soulBindingHandlerProjection(agentID string, username string, replayed bool) *soulservice.SoulBindingProjection {
	now := time.Date(2026, 7, 14, 16, 20, 2, 0, time.UTC)
	return &soulservice.SoulBindingProjection{
		Soul: soulservice.Soul{
			AgentID:               agentID,
			Domain:                "example.com",
			LocalID:               username,
			AuthorityModel:        soulservice.SoulAuthorityModelInstanceTrust,
			AnchorState:           soulservice.SoulAnchorStateHostedOffchain,
			OperationalBinding:    soulservice.SoulOperationalBindingHostedBound,
			Status:                "active",
			LifecycleStatus:       "active",
			PublishedVersion:      3,
			Bound:                 true,
			BoundAgentUsername:    username,
			BoundPrincipalAddress: "0x1111111111111111111111111111111111111111",
			BoundAt:               now,
			BoundUpdatedAt:        now,
		},
		IdempotencyKey: "bind-key-1",
		PayloadHash:    "sha256:handler-payload",
		Replayed:       replayed,
	}
}

func TestSoulHandlerHelpers_ServiceResolutionAndErrorMapping(t *testing.T) {
	t.Parallel()

	cfg := round10TestConfig()
	var nilHandler *Handler
	require.Nil(t, nilHandler.getSoulService())

	h := &Handler{cfg: cfg, logger: round10TestLogger(t)}
	require.Nil(t, h.getSoulService())

	builtHandler, _, _ := round11NewHandlerSliceC(t, nil)
	require.NotNil(t, builtHandler.getSoulService())

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/souls/mine", nil, nil, nil)
	require.NoError(t, err)

	requireStatus(t, http.StatusUnprocessableEntity)(h.respondSoulServiceError(ctx, soulservice.ErrTrustNotConfigured))
	requireStatus(t, http.StatusNotFound)(h.respondSoulServiceError(ctx, soulservice.ErrSoulNotAvailable))
	requireStatus(t, http.StatusUnprocessableEntity)(h.respondSoulServiceError(ctx, soulservice.ErrTargetAgentRequired))
	requireStatus(t, http.StatusNotFound)(h.respondSoulServiceError(ctx, soulservice.ErrTargetAgentNotFound))
	requireStatus(t, http.StatusForbidden)(h.respondSoulServiceError(ctx, soulservice.ErrTargetAgentNotOwned))
	requireStatus(t, http.StatusUnprocessableEntity)(h.respondSoulServiceError(ctx, soulservice.ErrTargetAgentMustBeAgent))
	requireStatus(t, http.StatusConflict)(h.respondSoulServiceError(ctx, soulservice.ErrSoulAlreadyBound))
	requireStatus(t, http.StatusConflict)(h.respondSoulServiceError(ctx, soulservice.ErrTargetAgentAlreadyHasSoul))
	requireStatus(t, http.StatusInternalServerError)(h.respondSoulServiceError(ctx, errors.New("boom")))
}

func TestSoulAvatarConversionIncludesStyles(t *testing.T) {
	t.Parallel()

	styleID := 7
	avatar := toAPISoulAvatar(&soulservice.SoulAvatar{
		TokenURI:               "ipfs://avatar",
		Image:                  "https://cdn.example/avatar.png",
		CurrentStyleID:         &styleID,
		CurrentStyleName:       "classic",
		CurrentRendererAddress: "0xrenderer",
		Styles: []soulservice.SoulAvatarStyle{
			{
				StyleID:         styleID,
				StyleName:       "classic",
				RendererAddress: "0xrenderer",
				Image:           "https://cdn.example/classic.png",
				Selected:        true,
			},
		},
	})

	require.NotNil(t, avatar)
	require.Equal(t, "ipfs://avatar", avatar.TokenURI)
	require.Equal(t, "https://cdn.example/avatar.png", avatar.Image)
	require.NotNil(t, avatar.CurrentStyleID)
	require.Equal(t, styleID, *avatar.CurrentStyleID)
	require.Equal(t, "classic", avatar.CurrentStyleName)
	require.Equal(t, "0xrenderer", avatar.CurrentRendererAddress)
	require.Len(t, avatar.Styles, 1)
	require.Equal(t, styleID, avatar.Styles[0].StyleID)
	require.Equal(t, "classic", avatar.Styles[0].StyleName)
	require.Equal(t, "0xrenderer", avatar.Styles[0].RendererAddress)
	require.Equal(t, "https://cdn.example/classic.png", avatar.Styles[0].Image)
	require.True(t, avatar.Styles[0].Selected)
}

func TestHandleGetMySoulsLift_ReturnsInternalWhenServiceUnavailable(t *testing.T) {
	t.Parallel()

	cfg := round10TestConfig()
	h := &Handler{cfg: cfg, logger: round10TestLogger(t)}
	token := round11SignToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead}, "sess-4")

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/souls/mine", map[string]string{
		"Authorization": "Bearer " + token,
	}, nil, nil)
	require.NoError(t, err)

	requireStatus(t, http.StatusInternalServerError)(h.HandleGetMySoulsLift(ctx))
}
