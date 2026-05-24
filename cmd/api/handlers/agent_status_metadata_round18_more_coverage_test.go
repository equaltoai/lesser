package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/agents"
	"github.com/equaltoai/lesser/pkg/auth"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestAgentStatusMetadataRound18_NormalizeAgentMemoryEventRequest(t *testing.T) {
	cfg := round11TestConfig()

	t.Run("returns 503 when storage missing", func(t *testing.T) {
		h := &Handler{cfg: cfg}
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", nil, nil, nil)
		require.NoError(t, err)

		claims := &auth.Claims{Username: "agent", IsAgent: true}
		req := &apimodels.CreateStatusRequest{
			MemoryEvent: &apimodels.AgentMemoryEventRequest{EventType: "correction", OriginalID: "status-1"},
		}
		resp, respErr := h.normalizeAgentMemoryEventRequest(ctx, claims, req)
		require.NoError(t, respErr)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusServiceUnavailable, resp.Status)
	})

	t.Run("validates event type", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", nil, nil, nil)
		require.NoError(t, err)

		claims := &auth.Claims{Username: "agent", IsAgent: true}
		req := &apimodels.CreateStatusRequest{
			MemoryEvent: &apimodels.AgentMemoryEventRequest{EventType: "unknown"},
		}
		resp, respErr := h.normalizeAgentMemoryEventRequest(ctx, claims, req)
		require.NoError(t, respErr)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusBadRequest, resp.Status)
	})

	t.Run("requires original id when correction/retraction", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", nil, nil, nil)
		require.NoError(t, err)

		claims := &auth.Claims{Username: "agent", IsAgent: true}
		req := &apimodels.CreateStatusRequest{
			MemoryEvent: &apimodels.AgentMemoryEventRequest{EventType: "correction"},
		}
		resp, respErr := h.normalizeAgentMemoryEventRequest(ctx, claims, req)
		require.NoError(t, respErr)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusBadRequest, resp.Status)
	})

	t.Run("rejects invalid original status id", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", nil, nil, nil)
		require.NoError(t, err)

		claims := &auth.Claims{Username: "agent", IsAgent: true}
		req := &apimodels.CreateStatusRequest{
			MemoryEvent: &apimodels.AgentMemoryEventRequest{EventType: "correction", OriginalID: "%"},
		}
		resp, respErr := h.normalizeAgentMemoryEventRequest(ctx, claims, req)
		require.NoError(t, respErr)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusBadRequest, resp.Status)
	})

	t.Run("returns 404 when original status missing", func(t *testing.T) {
		state := &round10QueryState{
			notFoundPKs: map[string]bool{"status#orig": true},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", nil, nil, nil)
		require.NoError(t, err)

		claims := &auth.Claims{Username: "agent", IsAgent: true}
		req := &apimodels.CreateStatusRequest{
			MemoryEvent: &apimodels.AgentMemoryEventRequest{EventType: "correction", OriginalID: "orig"},
		}
		resp, respErr := h.normalizeAgentMemoryEventRequest(ctx, claims, req)
		require.NoError(t, respErr)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusNotFound, resp.Status)
	})

	t.Run("returns 403 when correcting someone else's post", func(t *testing.T) {
		state := &round10QueryState{
			statusByID: map[string]storagemodels.Status{
				"orig": {PK: "status#orig", SK: "status#orig", StatusID: "orig", AuthorUsername: "other"},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", nil, nil, nil)
		require.NoError(t, err)

		claims := &auth.Claims{Username: "agent", IsAgent: true}
		req := &apimodels.CreateStatusRequest{
			MemoryEvent: &apimodels.AgentMemoryEventRequest{EventType: "correction", OriginalID: "orig"},
		}
		resp, respErr := h.normalizeAgentMemoryEventRequest(ctx, claims, req)
		require.NoError(t, respErr)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusForbidden, resp.Status)
	})

	t.Run("normalizes and threads correction", func(t *testing.T) {
		state := &round10QueryState{
			statusByID: map[string]storagemodels.Status{
				"orig": {PK: "status#orig", SK: "status#orig", StatusID: "orig", AuthorUsername: "agent"},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", nil, nil, nil)
		require.NoError(t, err)

		claims := &auth.Claims{Username: "agent", IsAgent: true}
		req := &apimodels.CreateStatusRequest{
			MemoryEvent: &apimodels.AgentMemoryEventRequest{EventType: "Correction", OriginalID: "orig"},
		}
		resp, respErr := h.normalizeAgentMemoryEventRequest(ctx, claims, req)
		require.NoError(t, respErr)
		require.Nil(t, resp)
		require.Equal(t, "correction", req.MemoryEvent.EventType)
		require.Equal(t, "orig", req.MemoryEvent.OriginalID)
		require.Equal(t, "orig", req.InReplyToID)
	})
}

func TestAgentStatusMetadataRound18_BuildAgentStatusAttribution(t *testing.T) {
	cfg := round11TestConfig()

	makeAgentState := func() *round10QueryState {
		now := time.Now().UTC()
		return &round10QueryState{
			usersByUsername: map[string]storagemodels.User{
				"agent": {
					PK:                "USER#agent",
					SK:                storagemodels.SKMetadata,
					Username:          "agent",
					Approved:          true,
					Role:              "user",
					CreatedAt:         now.Add(-24 * time.Hour),
					UpdatedAt:         now,
					IsAgent:           true,
					AgentOwner:        "owner",
					AgentVersion:      "",
					AgentCapabilities: &agents.Capabilities{MaxPostsPerHour: 5, RequiresApproval: true},
				},
			},
		}
	}

	t.Run("returns 503 when storage missing", func(t *testing.T) {
		h := &Handler{cfg: cfg}
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", nil, nil, nil)
		require.NoError(t, err)

		claims := &auth.Claims{Username: "agent", IsAgent: true}
		req := &apimodels.CreateStatusRequest{}
		_, resp, respErr := h.buildAgentStatusAttribution(ctx, claims, req)
		require.NoError(t, respErr)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusServiceUnavailable, resp.Status)
	})

	t.Run("returns 403 when user is not an agent", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", nil, nil, nil)
		require.NoError(t, err)

		claims := &auth.Claims{Username: "alice", IsAgent: true}
		req := &apimodels.CreateStatusRequest{}
		_, resp, respErr := h.buildAgentStatusAttribution(ctx, claims, req)
		require.NoError(t, respErr)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusForbidden, resp.Status)
	})

	t.Run("validates trigger details length", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, makeAgentState())
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", nil, nil, nil)
		require.NoError(t, err)

		claims := &auth.Claims{Username: "agent", IsAgent: true}
		req := &apimodels.CreateStatusRequest{
			AgentAttribution: &apimodels.AgentPostAttribution{TriggerDetails: strings.Repeat("x", agentMaxTriggerDetails+1)},
		}
		_, resp, respErr := h.buildAgentStatusAttribution(ctx, claims, req)
		require.NoError(t, respErr)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusBadRequest, resp.Status)
	})

	t.Run("validates memory citation limits and format", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, makeAgentState())
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", nil, nil, nil)
		require.NoError(t, err)

		claims := &auth.Claims{Username: "agent", IsAgent: true}
		citations := make([]string, 0, agentMaxMemoryCitations+1)
		for i := 0; i < agentMaxMemoryCitations+1; i++ {
			citations = append(citations, "status-"+strconv.Itoa(i))
		}
		req := &apimodels.CreateStatusRequest{
			AgentAttribution: &apimodels.AgentPostAttribution{MemoryCitations: citations},
		}
		_, resp, respErr := h.buildAgentStatusAttribution(ctx, claims, req)
		require.NoError(t, respErr)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusBadRequest, resp.Status)

		req.AgentAttribution = &apimodels.AgentPostAttribution{MemoryCitations: []string{"%"}}
		_, resp, respErr = h.buildAgentStatusAttribution(ctx, claims, req)
		require.NoError(t, respErr)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusBadRequest, resp.Status)
	})

	t.Run("rejects invalid trigger types", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, makeAgentState())
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", nil, nil, nil)
		require.NoError(t, err)

		claims := &auth.Claims{Username: "agent", IsAgent: true}
		req := &apimodels.CreateStatusRequest{AgentAttribution: &apimodels.AgentPostAttribution{TriggerType: "unsupported"}}
		_, resp, respErr := h.buildAgentStatusAttribution(ctx, claims, req)
		require.NoError(t, respErr)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusBadRequest, resp.Status)
	})

	t.Run("success builds attribution and applies defaults", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, makeAgentState())
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", nil, nil, nil)
		require.NoError(t, err)

		claims := &auth.Claims{
			Username: "agent",
			Scopes:   []string{"write:statuses"},
			IsAgent:  true,
		}
		req := &apimodels.CreateStatusRequest{}
		attribution, resp, respErr := h.buildAgentStatusAttribution(ctx, claims, req)
		require.NoError(t, respErr)
		require.Nil(t, resp)
		require.NotNil(t, attribution)
		require.Equal(t, "manual", attribution.TriggerType)
		require.Equal(t, cfg.ActorURL("owner"), attribution.DelegatedBy)
		require.Equal(t, activitypub.AgentAttributionSchemaVersion, attribution.SchemaVersion)
		require.Equal(t, agentVersionUnknown, attribution.ModelID)
		require.Equal(t, []string{"write:statuses"}, attribution.Scopes)
		require.Equal(t, agents.DroneIdentityStateDrone, attribution.IdentityState)
		require.Equal(t, "Drone", attribution.IdentityLabel)
		require.Equal(t, agents.DroneContinuityStatePlanned, attribution.ContinuityState)
		require.Equal(t, "Drone", attribution.ModerationLabel)
	})

	t.Run("includes souled identity metadata when present", func(t *testing.T) {
		metadata, err := agents.SetDroneWorkflowMetadata(nil, &agents.DroneWorkflowState{
			CurrentPhase: agents.DroneWorkflowPhaseContinuity,
			CurrentState: agents.DroneWorkflowStateContinuityStable,
			SoulAgentID:  "0xagent-soul",
		})
		require.NoError(t, err)

		state := makeAgentState()
		agentUser := state.usersByUsername["agent"]
		agentUser.Metadata = metadata
		state.usersByUsername["agent"] = agentUser
		state.soulBodyBindingsByAgentID = map[string]storagemodels.InstanceSoulBodyBinding{
			"0xagent-soul": *storagemodels.NewInstanceSoulBodyBinding("0xagent-soul", "agent", "0xprincipal"),
		}
		state.soulBodyBindingUsernames = map[string]storagemodels.InstanceSoulBodyBindingUsername{
			"agent": *storagemodels.NewInstanceSoulBodyBindingUsername("agent", "0xagent-soul"),
		}

		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", nil, nil, nil)
		require.NoError(t, err)

		claims := &auth.Claims{
			Username: "agent",
			Scopes:   []string{"write:statuses"},
			IsAgent:  true,
		}
		attribution, resp, respErr := h.buildAgentStatusAttribution(ctx, claims, &apimodels.CreateStatusRequest{})
		require.NoError(t, respErr)
		require.Nil(t, resp)
		require.NotNil(t, attribution)
		require.Equal(t, agents.DroneIdentityStateSouled, attribution.IdentityState)
		require.Equal(t, "Souled", attribution.IdentityLabel)
		require.Equal(t, agents.DroneContinuityStateStable, attribution.ContinuityState)
		// CSR-044 M2.2 second rework: SoulAgentID must NOT be stored on the Note's
		// AgentPostAttribution at creation time. The identity semantics resolve the
		// SoulAgentID from soul body bindings, but it is private data that must not
		// persist into stored public post attribution.
		require.Empty(t, attribution.SoulAgentID,
			"CSR-044: SoulAgentID must not be stored on the Note's AgentPostAttribution")
		require.Equal(t, "Souled", attribution.ModerationLabel)
	})
}
