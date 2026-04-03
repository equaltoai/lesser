package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestRound31HandleAccountSearchLift_ReturnsHyphenatedServiceActor(t *testing.T) {
	cfg := round11TestConfig()
	state := &round10QueryState{
		actorsByUser: map[string]storagemodels.Actor{
			"agent-0": {
				Username: "agent-0",
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: cfg.ActorURL("agent-0"), Type: activitypub.ServiceType},
					PreferredUsername: "agent-0",
					Name:              "Agent 0",
				},
			},
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, state)

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/search", nil, map[string]string{
		"q":     "agent-0",
		"limit": "5",
	}, nil)
	require.NoError(t, err)

	resp := requireStatus(t, http.StatusOK)(handler.HandleAccountSearchLift(ctx))

	var accounts []apimodels.Account
	require.NoError(t, json.Unmarshal(resp.Body, &accounts))
	require.Len(t, accounts, 1)
	require.Equal(t, "agent-0", accounts[0].Username)
}

func TestRound31HandleSearchV2Lift_ReturnsHydratedPartialServiceActor(t *testing.T) {
	cfg := round11TestConfig()
	state := &round10QueryState{
		actorsByUser: map[string]storagemodels.Actor{
			"arch": {
				Username: "arch",
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: cfg.ActorURL("arch"), Type: activitypub.ServiceType},
					PreferredUsername: "arch",
					Name:              "Arch",
				},
			},
		},
		actorList: []storagemodels.Actor{{
			PK:       "ACTOR#arch",
			Username: "arch",
			GSI1PK:   "USERNAME_SEARCH#ar",
			GSI1SK:   "arch",
			GSI2PK:   "NAME_SEARCH#ar",
			GSI2SK:   "arch#arch",
			Actor: &activitypub.Actor{
				BaseObject:        activitypub.BaseObject{Type: activitypub.ServiceType},
				PreferredUsername: "arch",
				Name:              "Arch",
			},
		}},
	}

	handler, _, _ := round11NewHandler(t, cfg, state)

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v2/search", nil, map[string]string{
		"q":     "ar",
		"type":  "accounts",
		"limit": "5",
	}, nil)
	require.NoError(t, err)

	resp := requireStatus(t, http.StatusOK)(handler.HandleSearchV2Lift(ctx))

	var result apimodels.SearchResult
	require.NoError(t, json.Unmarshal(resp.Body, &result))
	require.Len(t, result.Accounts, 1)
	require.Equal(t, "arch", result.Accounts[0].Username)
}
