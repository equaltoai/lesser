package handlers

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAgentSelfRegistration_DefaultConfigRejectsWithoutOperatorOptIn(t *testing.T) {
	cfg := round10TestConfig()
	state := &round10QueryState{
		notFoundPKSK: map[string]bool{
			"INSTANCE#CONFIG#AGENT_CONFIG": true,
		},
	}
	h, _, _ := round11NewHandler(t, cfg, state)

	challengeCtx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/register/challenge", nil, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusForbidden)(h.HandleAgentRegisterChallengeLift(challengeCtx))

	registerCtx, err := round10NewLiftContext(http.MethodPost, "/api/v1/agents/register", nil, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusForbidden)(h.HandleAgentRegisterLift(registerCtx))
}
