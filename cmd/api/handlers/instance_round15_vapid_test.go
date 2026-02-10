package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/stretchr/testify/require"
)

func TestInstanceV1_VAPIDMissingBranches_Round15(t *testing.T) {
	t.Run("production requires VAPID keys", func(t *testing.T) {
		cfg := round10TestConfig()
		cfg.Stage = EnvProduction

		state := &round10QueryState{
			forceVapidNotFound: true,
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/instance", nil, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusInternalServerError)(h.HandleGetInstanceV1Lift(ctx))
	})

	t.Run("non-production tolerates missing VAPID keys", func(t *testing.T) {
		cfg := round10TestConfig()
		cfg.Stage = "development"

		state := &round10QueryState{
			forceVapidNotFound: true,
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/instance", nil, nil, nil)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusOK)(h.HandleGetInstanceV1Lift(ctx))
		var out apimodels.InstanceV1Response
		require.NoError(t, json.Unmarshal(resp.Body, &out))
		require.Equal(t, "", out.VAPIDKey)
	})
}
