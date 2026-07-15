package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestInstancePublicVersionUsesConfiguredLesserRelease(t *testing.T) {
	t.Setenv("VERSION", "v1.5.20")
	config.ResetForTests()
	t.Cleanup(config.ResetForTests)

	cfg := round10TestConfig()
	cfg.Stage = "development"
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{forceVapidNotFound: true})

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/instance", nil, nil, nil)
	require.NoError(t, err)

	resp := requireStatus(t, http.StatusOK)(handler.HandleGetInstanceV1Lift(ctx))
	var out apimodels.InstanceV1Response
	require.NoError(t, json.Unmarshal(resp.Body, &out))
	require.Equal(t, "4.0.0 (compatible; Lesser v1.5.20)", out.Version)
	require.NotContains(t, out.Version, "Lesser 0.1.0")
}
