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
	soulservice "github.com/equaltoai/lesser/pkg/services/souls"
	"github.com/stretchr/testify/require"
)

type stubSoulHandlerService struct {
	listMineOut     []soulservice.Soul
	listMineErr     error
	incorporateOut  *soulservice.Soul
	incorporateErr  error
	resolveBoundOut *soulservice.Soul
	resolveBoundErr error
	lastUsername    string
	lastSoulAgentID string
	lastTargetAgent string
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
