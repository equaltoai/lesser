package handlers

import (
	"context"
	"testing"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/agents"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCSR044_FillStatusAgentIdentityDefaults_NoSoulAgentIDBackfill verifies
// the CSR-044 fix: fillStatusAgentIdentityDefaults must NOT backfill
// SoulAgentID from runtime identity semantics. The SoulAgentID originates
// from a private soul body binding and must not leak to public viewers
// through AgentPostAttribution on status responses.
func TestCSR044_FillStatusAgentIdentityDefaults_NoSoulAgentIDBackfill(t *testing.T) {
	// Handler without repos: the agentIdentitySemantics call will have no
	// soul binding to look up, so it produces a drone identity without
	// SoulAgentID. The critical assertion is that fillStatusAgentIdentityDefaults
	// never ADDS SoulAgentID that wasn't already present in the attribution.
	cfg := round11TestConfig()
	h := &Handler{cfg: cfg}

	agentUser := &storage.User{
		Username:    "agent-bob",
		DisplayName: "Agent Bob",
		IsAgent:     true,
		AgentType:   "CUSTOM",
	}

	// Build attribution without SoulAgentID. The runtime must not inject one.
	out := &apimodels.AgentPostAttribution{
		SchemaVersion: activitypub.AgentAttributionSchemaVersion,
		ModelID:       "gpt-4o",
	}

	ctx := context.Background()
	h.fillStatusAgentIdentityDefaults(ctx, out, agentUser)

	// CSR-044: SoulAgentID must remain empty — no runtime backfill.
	assert.Empty(t, out.SoulAgentID,
		"CSR-044 regression: SoulAgentID must not be backfilled from runtime identity semantics")

	// Other attribution fields are fine — they are public transparency metadata.
	assert.NotEmpty(t, out.IdentityLabel, "public identity label must be filled")
	assert.NotEmpty(t, out.ModerationLabel, "public moderation label must be filled")
}

// TestCSR044_FillStatusAgentIdentityDefaults_PreservesExplicitID verifies
// that a SoulAgentID explicitly stored by the agent on the Note at creation
// time is preserved. The fix only removes the runtime backfill, not the
// agent's own declared identity.
func TestCSR044_FillStatusAgentIdentityDefaults_PreservesExplicitID(t *testing.T) {
	h := &Handler{}

	agentUser := &storage.User{
		Username:    "agent-bob",
		DisplayName: "Agent Bob",
		IsAgent:     true,
		AgentType:   "CUSTOM",
	}

	explicitID := "0xagent-self-declared"
	out := &apimodels.AgentPostAttribution{
		SchemaVersion: activitypub.AgentAttributionSchemaVersion,
		ModelID:       "gpt-4o",
		SoulAgentID:   explicitID,
	}

	ctx := context.Background()
	h.fillStatusAgentIdentityDefaults(ctx, out, agentUser)

	require.Equal(t, explicitID, out.SoulAgentID,
		"explicitly stored SoulAgentID from the Note must be preserved")
}

// TestCSR044_RedactAPIAgentPrivateFields_ClearsSoulFields verifies that
// redactAPIAgentPrivateFields correctly clears SoulAgentID and resets
// SoulBindingState for public viewers on the REST agent endpoints.
func TestCSR044_RedactAPIAgentPrivateFields_ClearsSoulFields(t *testing.T) {
	agent := &apimodels.Agent{
		Username:        "agent-test",
		DisplayName:     "Test Agent",
		AgentOwner:      "@owner",
		DelegatedScopes: []string{"write:statuses"},
		IdentitySemantics: apimodels.AgentIdentitySemantics{
			SoulBindingState: "BOUND",
			SoulAgentID:      "0xsecret-soul-id",
		},
	}

	redactAPIAgentPrivateFields(agent)

	assert.Empty(t, agent.AgentOwner)
	assert.Nil(t, agent.DelegatedScopes)
	assert.Empty(t, agent.IdentitySemantics.SoulAgentID,
		"SoulAgentID must be cleared for public viewers")
	assert.Equal(t, "UNBOUND", agent.IdentitySemantics.SoulBindingState,
		"SoulBindingState must reset to UNBOUND for public viewers")
}

// TestCSR044_AgentIdentitySemantics_ReturnsIdentityWithoutRepos verifies
// that agentIdentitySemantics gracefully handles missing repos (produces
// drone identity without soul fields).
func TestCSR044_AgentIdentitySemantics_ReturnsIdentityWithoutRepos(t *testing.T) {
	cfg := round11TestConfig()
	h := &Handler{cfg: cfg}

	agentUser := &storage.User{
		Username:    "agent-bob",
		DisplayName: "Agent Bob",
		IsAgent:     true,
		AgentType:   "CUSTOM",
	}

	ctx := context.Background()
	identity := h.agentIdentitySemantics(ctx, agentUser)

	// Without repos, no soul binding lookup is possible.
	// The identity should be a drone identity, not souled.
	assert.Equal(t, agents.DroneIdentityStateDrone, identity.IdentityState)
	assert.Empty(t, identity.SoulAgentID)
	assert.Equal(t, "UNBOUND", identity.SoulBindingState)
}

// TestCSR044_NoBackfillWithReposAndWorkflowSoulAgentID proves the CSR-044
// fix closes the actual leak path: when repos are present and the agent's
// workflow metadata contains a SoulAgentID, DeriveDroneIdentitySemantics
// propagates it into the identity semantics (as it should — the identity
// function is correct), but fillStatusAgentIdentityDefaults must NOT
// backfill it onto the public AgentPostAttribution.
//
// This is the strengthened test covering the case the original tests
// missed: Handler has repos, workflow metadata has SoulAgentID, and the
// fix must prevent that SoulAgentID from surfacing on public statuses.
func TestCSR044_NoBackfillWithReposAndWorkflowSoulAgentID(t *testing.T) {
	cfg := round11TestConfig()

	// Build workflow metadata containing a SoulAgentID. This simulates
	// the real scenario: during soul binding, the SoulAgentID is stored
	// in the drone workflow metadata on the User record.
	metadata, err := agents.SetDroneWorkflowMetadata(nil, &agents.DroneWorkflowState{
		CurrentPhase: agents.DroneWorkflowPhaseContinuity,
		CurrentState: agents.DroneWorkflowStateContinuityStable,
		SoulAgentID:  "0xleaked-soul-id",
	})
	require.NoError(t, err)

	// Handler with repos (empty query state — no soul binding in the DB,
	// so the repo lookup will fail; the workflow metadata fallback is the
	// actual leak path we are testing).
	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	agentUser := &storage.User{
		Username:    "agent-bob",
		DisplayName: "Agent Bob",
		IsAgent:     true,
		AgentType:   "CUSTOM",
		Metadata:    metadata,
	}

	// Verify that agentIdentitySemantics DOES derive SoulAgentID from
	// the workflow metadata. This is correct behavior — the identity
	// function should resolve what it can. The fix is downstream.
	identity := h.agentIdentitySemantics(context.Background(), agentUser)
	require.Equal(t, agents.DroneIdentityStateSouled, identity.IdentityState,
		"identity must resolve to Souled from workflow metadata SoulAgentID")
	require.Equal(t, "0xleaked-soul-id", identity.SoulAgentID,
		"identity semantics must derive SoulAgentID from workflow metadata")

	// Now build an attribution WITHOUT SoulAgentID (the common case for
	// statuses that don't have an explicit agent-declared ID on the Note).
	out := &apimodels.AgentPostAttribution{
		SchemaVersion: activitypub.AgentAttributionSchemaVersion,
		ModelID:       "gpt-4o",
	}

	h.fillStatusAgentIdentityDefaults(context.Background(), out, agentUser)

	// CSR-044: SoulAgentID must NOT be backfilled from identity semantics.
	require.Empty(t, out.SoulAgentID,
		"CSR-044: SoulAgentID must not be backfilled from identity semantics to public status attribution")

	// Other identity fields are public transparency metadata and should still be filled.
	require.Equal(t, agents.DroneIdentityStateSouled, out.IdentityState)
	require.Equal(t, "Souled", out.IdentityLabel)
	require.Equal(t, "Souled", out.ModerationLabel)
}

// TestCSR044_FullBuildStatusAgentAttribution_NoLeak proves the complete
// status rendering path (buildStatusAgentAttribution) does not leak
// SoulAgentID when the identity semantics resolve one from workflow
// metadata or soul body bindings.
func TestCSR044_FullBuildStatusAgentAttribution_NoLeak(t *testing.T) {
	cfg := round11TestConfig()

	metadata, err := agents.SetDroneWorkflowMetadata(nil, &agents.DroneWorkflowState{
		CurrentPhase: agents.DroneWorkflowPhaseContinuity,
		CurrentState: agents.DroneWorkflowStateContinuityStable,
		SoulAgentID:  "0xleaked-soul-id",
	})
	require.NoError(t, err)

	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	account := &storage.Account{
		User: &storage.User{
			Username:     "agent-bob",
			IsAgent:      true,
			AgentOwner:   "@owner",
			AgentType:    "CUSTOM",
			AgentVersion: "v3",
			Metadata:     metadata,
		},
	}

	// Full path: buildStatusAgentAttribution creates the attribution from
	// a Note with no agent attribution (so all values come from defaults).
	out := h.buildStatusAgentAttribution(context.Background(), account,
		&storagemodels.Status{Note: &activitypub.Note{}})

	require.NotNil(t, out)

	// CSR-044: SoulAgentID must not leak through the full status rendering path.
	require.Empty(t, out.SoulAgentID,
		"CSR-044: full buildStatusAgentAttribution must not leak SoulAgentID")

	// Public identity fields are transparent and should be filled.
	require.Equal(t, agents.DroneIdentityStateSouled, out.IdentityState)
	require.Equal(t, "Souled", out.IdentityLabel)
	require.Equal(t, "Souled", out.ModerationLabel)
}
