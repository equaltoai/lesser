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
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestMarkersHandlersRound12(t *testing.T) {
	cfg := round11TestConfig()

	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
	writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})

	t.Run("get markers unauthorized", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{AccountsSvc: &AccountsServiceStub{}})
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/markers", nil, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusUnauthorized)(h.HandleGetMarkersLift(ctx))
	})

	t.Run("get markers insufficient scope", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{AccountsSvc: &AccountsServiceStub{}})
		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/markers", map[string]string{
			"Authorization": "Bearer " + token,
		}, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusForbidden)(h.HandleGetMarkersLift(ctx))
	})

	t.Run("get markers service error", func(t *testing.T) {
		accountsSvc := &AccountsServiceStub{
			GetMarkersFunc: func(context.Context, *accounts.GetMarkersQuery) (*accounts.GetMarkersResult, error) {
				return nil, errors.New("boom")
			},
		}
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{AccountsSvc: accountsSvc})
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/markers", map[string]string{
			"Authorization": "Bearer " + readToken,
		}, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusInternalServerError)(h.HandleGetMarkersLift(ctx))
	})

	t.Run("get markers success", func(t *testing.T) {
		now := time.Now()
		accountsSvc := &AccountsServiceStub{
			GetMarkersFunc: func(context.Context, *accounts.GetMarkersQuery) (*accounts.GetMarkersResult, error) {
				return &accounts.GetMarkersResult{
					Markers: map[string]*storage.Marker{
						"home":          {LastReadID: "1", UpdatedAt: now, Version: 1},
						"notifications": {LastReadID: "2", UpdatedAt: now, Version: 2},
					},
				}, nil
			},
		}
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{AccountsSvc: accountsSvc})
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/markers", map[string]string{
			"Authorization": "Bearer " + readToken,
		}, map[string]string{"timeline[]": "home,notifications"}, nil)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusOK)(h.HandleGetMarkersLift(ctx))

		var markers apimodels.MarkersResponse
		require.NoError(t, json.Unmarshal(resp.Body, &markers))
		require.NotNil(t, markers.Home)
		require.NotNil(t, markers.Notifications)
	})

	t.Run("save markers unauthorized", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{AccountsSvc: &AccountsServiceStub{}})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/markers", nil, nil, map[string]any{
			"home": map[string]any{"last_read_id": "1"},
		})
		require.NoError(t, err)

		requireStatus(t, http.StatusUnauthorized)(h.HandleSaveMarkersLift(ctx))
	})

	t.Run("save markers invalid JSON body", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{AccountsSvc: &AccountsServiceStub{}})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/markers", map[string]string{
			"Authorization": "Bearer " + writeToken,
		}, nil, []byte("{"))

		requireStatus(t, http.StatusBadRequest)(h.HandleSaveMarkersLift(ctx))
	})

	t.Run("save markers empty body returns bad request", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{AccountsSvc: &AccountsServiceStub{}})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/markers", map[string]string{
			"Authorization": "Bearer " + writeToken,
		}, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusBadRequest)(h.HandleSaveMarkersLift(ctx))
	})

	t.Run("save markers validation error for empty request", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{AccountsSvc: &AccountsServiceStub{}})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/markers", map[string]string{
			"Authorization": "Bearer " + writeToken,
		}, nil, map[string]any{})
		require.NoError(t, err)

		requireStatus(t, http.StatusBadRequest)(h.HandleSaveMarkersLift(ctx))
	})

	t.Run("save markers validation error for invalid timeline", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{AccountsSvc: &AccountsServiceStub{}})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/markers", map[string]string{
			"Authorization": "Bearer " + writeToken,
		}, nil, map[string]any{
			"bad": map[string]any{"last_read_id": "1"},
		})
		require.NoError(t, err)

		requireStatus(t, http.StatusBadRequest)(h.HandleSaveMarkersLift(ctx))
	})

	t.Run("save markers returns 500 when updated markers fetch fails", func(t *testing.T) {
		calls := 0
		accountsSvc := &AccountsServiceStub{
			GetMarkersFunc: func(context.Context, *accounts.GetMarkersQuery) (*accounts.GetMarkersResult, error) {
				calls++
				if calls == 1 {
					return &accounts.GetMarkersResult{Markers: map[string]*storage.Marker{}}, nil
				}
				return nil, errors.New("fail updated markers")
			},
			SaveMarkerFunc: func(context.Context, *accounts.SaveMarkerCommand) (*accounts.SaveMarkerResult, error) {
				return &accounts.SaveMarkerResult{}, nil
			},
		}
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{AccountsSvc: accountsSvc})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/markers", map[string]string{
			"Authorization": "Bearer " + writeToken,
		}, nil, map[string]any{
			"home": map[string]any{"last_read_id": "1"},
		})
		require.NoError(t, err)

		requireStatus(t, http.StatusInternalServerError)(h.HandleSaveMarkersLift(ctx))
	})

	t.Run("save markers success saves and returns updated markers", func(t *testing.T) {
		calls := 0
		savedVersions := map[string]int{}
		now := time.Now()
		accountsSvc := &AccountsServiceStub{
			GetMarkersFunc: func(context.Context, *accounts.GetMarkersQuery) (*accounts.GetMarkersResult, error) {
				calls++
				if calls == 1 {
					return &accounts.GetMarkersResult{
						Markers: map[string]*storage.Marker{
							"home": {LastReadID: "old", UpdatedAt: now, Version: 3},
						},
					}, nil
				}
				return &accounts.GetMarkersResult{
					Markers: map[string]*storage.Marker{
						"home":          {LastReadID: "new", UpdatedAt: now, Version: 4},
						"notifications": {LastReadID: "n1", UpdatedAt: now, Version: 1},
					},
				}, nil
			},
			SaveMarkerFunc: func(_ context.Context, cmd *accounts.SaveMarkerCommand) (*accounts.SaveMarkerResult, error) {
				savedVersions[cmd.Timeline] = cmd.Version
				if cmd.Timeline == "notifications" {
					return nil, errors.New("save failed")
				}
				return &accounts.SaveMarkerResult{}, nil
			},
		}
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{AccountsSvc: accountsSvc})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/markers", map[string]string{
			"Authorization": "Bearer " + writeToken,
		}, nil, map[string]any{
			"home":          map[string]any{"last_read_id": "new"},
			"notifications": map[string]any{"last_read_id": "n1"},
		})
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusOK)(h.HandleSaveMarkersLift(ctx))
		require.Equal(t, 4, savedVersions["home"])
		require.Equal(t, 1, savedVersions["notifications"])

		var markers apimodels.MarkersResponse
		require.NoError(t, json.Unmarshal(resp.Body, &markers))
		require.NotNil(t, markers.Home)
		require.NotNil(t, markers.Notifications)
	})
}
