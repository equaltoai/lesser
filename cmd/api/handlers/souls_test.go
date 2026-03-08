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
	lastUsername    string
	lastIncorporate string
}

func (s *stubSoulHandlerService) ListMine(_ context.Context, username string) ([]soulservice.Soul, error) {
	s.lastUsername = username
	return s.listMineOut, s.listMineErr
}

func (s *stubSoulHandlerService) Incorporate(_ context.Context, username string, agentID string) (*soulservice.Soul, error) {
	s.lastUsername = username
	s.lastIncorporate = agentID
	return s.incorporateOut, s.incorporateErr
}

func TestHandleGetMySoulsLift_ListsOwnedSoulsWithBindingState(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC)
	service := &stubSoulHandlerService{
		listMineOut: []soulservice.Soul{
			{
				AgentID:          "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Domain:           "example.com",
				LocalID:          "alpha",
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
				BoundUsername:         "alice",
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
				BoundUsername:         "bob",
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

	var body apimodels.SoulsMineResponse
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	require.Equal(t, "alice", service.lastUsername)
	require.Len(t, body.Souls, 3)
	require.Equal(t, 3, body.Count)
	require.Equal(t, "unbound", body.Souls[0].BindingState)
	require.True(t, body.Souls[0].AvailableForIncorporation)
	require.Equal(t, "bound", body.Souls[1].BindingState)
	require.True(t, body.Souls[1].AvailableForIncorporation)
	require.NotNil(t, body.Souls[1].Binding)
	require.Equal(t, "alice", body.Souls[1].Binding.Username)
	require.Equal(t, "bound", body.Souls[2].BindingState)
	require.False(t, body.Souls[2].AvailableForIncorporation)
	require.NotNil(t, body.Souls[2].Binding)
	require.Equal(t, "bob", body.Souls[2].Binding.Username)
}

func TestHandleIncorporateSoulLift_MapsServiceResponses(t *testing.T) {
	t.Parallel()

	const agentID = "0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"

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
					Wallet:                "0x1111111111111111111111111111111111111111",
					PrincipalAddress:      "0x1111111111111111111111111111111111111111",
					Status:                "active",
					Bound:                 true,
					BoundUsername:         "alice",
					BoundPrincipalAddress: "0x1111111111111111111111111111111111111111",
					BoundAt:               time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC),
					BoundUpdatedAt:        time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC),
				},
			},
			wantStatus:    http.StatusOK,
			wantAvailable: true,
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
			name:       "body already has soul",
			service:    &stubSoulHandlerService{incorporateErr: soulservice.ErrBodyAlreadyHasSoul},
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
			}, nil, nil)
			require.NoError(t, err)
			ctx.Params["agentId"] = agentID

			resp := requireStatus(t, tc.wantStatus)(h.HandleIncorporateSoulLift(ctx))
			if tc.wantStatus != http.StatusOK {
				return
			}

			var body apimodels.SoulIncorporateResponse
			require.NoError(t, json.Unmarshal(resp.Body, &body))
			require.Equal(t, "alice", tc.service.lastUsername)
			require.Equal(t, agentID, tc.service.lastIncorporate)
			require.Equal(t, agentID, body.Soul.Agent.AgentID)
			require.True(t, body.Soul.AvailableForIncorporation)
			require.NotNil(t, body.Soul.Binding)
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
	}, nil, nil)
	require.NoError(t, err)

	requireStatus(t, http.StatusBadRequest)(h.HandleIncorporateSoulLift(ctx))
}

func TestSoulHandlerHelpers_ServiceResolutionAndErrorMapping(t *testing.T) {
	t.Parallel()

	cfg := round10TestConfig()
	h := &Handler{cfg: cfg, logger: round10TestLogger(t)}
	require.Nil(t, h.getSoulService())

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/souls/mine", nil, nil, nil)
	require.NoError(t, err)

	requireStatus(t, http.StatusUnprocessableEntity)(h.respondSoulServiceError(ctx, soulservice.ErrTrustNotConfigured))
	requireStatus(t, http.StatusNotFound)(h.respondSoulServiceError(ctx, soulservice.ErrSoulNotAvailable))
	requireStatus(t, http.StatusConflict)(h.respondSoulServiceError(ctx, soulservice.ErrSoulAlreadyBound))
	requireStatus(t, http.StatusConflict)(h.respondSoulServiceError(ctx, soulservice.ErrBodyAlreadyHasSoul))
	requireStatus(t, http.StatusInternalServerError)(h.respondSoulServiceError(ctx, errors.New("boom")))
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
