package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/stretchr/testify/require"
)

func TestNodeInfoHandlers_Round11(t *testing.T) {
	cfg := round10TestConfig()
	cfg.Version = "v1.5.20"
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
	resp := requireStatus(t, http.StatusOK)(handler.HandleNodeInfoLift(ctx2))
	var out apimodels.NodeInfo
	require.NoError(t, json.Unmarshal(resp.Body, &out))
	require.Equal(t, "v1.5.20", out.Software.Version)
	require.NotContains(t, out.Software.Version, "Lesser 0.1.0")

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
