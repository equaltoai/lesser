package handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/stretchr/testify/require"
)

func TestNodeInfoHandlers_Round11(t *testing.T) {
	cfg := round10TestConfig()
	state := &round10QueryState{}
	handler, _, _ := round11NewHandler(t, cfg, state)

	handler.registry = &RegistryStub{
		AccountsSvc: &AccountsServiceStub{
			GetInstanceStatsFunc: func(ctx context.Context, query *accounts.GetInstanceStatsQuery) (*accounts.GetInstanceStatsResult, error) {
				return &accounts.GetInstanceStatsResult{
					TotalUsers:     10,
					ActiveMonth:    3,
					ActiveHalfyear: 5,
					LocalPosts:     12,
					LocalComments:  7,
				}, nil
			},
		},
	}

	ctx, err := round10NewLiftContext(http.MethodGet, "/.well-known/nodeinfo", nil, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(handler.HandleNodeInfoWellKnownLift(ctx))

	ctx2, err := round10NewLiftContext(http.MethodGet, "/nodeinfo/2.0", nil, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(handler.HandleNodeInfoLift(ctx2))

	handler.registry = &RegistryStub{
		AccountsSvc: &AccountsServiceStub{
			GetInstanceStatsFunc: func(ctx context.Context, query *accounts.GetInstanceStatsQuery) (*accounts.GetInstanceStatsResult, error) {
				return nil, errors.New("stats unavailable")
			},
		},
	}

	ctx3, err := round10NewLiftContext(http.MethodGet, "/nodeinfo/2.0", nil, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(handler.HandleNodeInfoLift(ctx3))
}
