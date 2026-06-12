package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	workflow "github.com/equaltoai/lesser/pkg/agents"
	"github.com/equaltoai/lesser/pkg/auth"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestRound44SoulBootstrapQueryProjectsZeroStateWorkflow(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	now := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)

	round13SeedGraphUser(t, storageRepo, &storage.User{
		Username:    "owner",
		DisplayName: "Owner",
		Approved:    true,
		CreatedAt:   now.Add(-48 * time.Hour),
		UpdatedAt:   now.Add(-24 * time.Hour),
	}, nil)
	round13SeedGraphUser(t, storageRepo, &storage.User{
		Username:    "drone-alpha",
		DisplayName: "Drone Alpha",
		Approved:    true,
		IsAgent:     true,
		AgentOwner:  "@owner",
		CreatedAt:   now.Add(-24 * time.Hour),
		UpdatedAt:   now,
	}, &storage.AgentGovernanceState{
		Username:        "drone-alpha",
		DelegatedScopes: []string{auth.ScopeRead, auth.ScopeWrite},
	})

	surface, err := (&queryResolver{resolver}).SoulBootstrap(round13DroneAuthContext("owner", auth.ScopeRead), "drone-alpha")
	require.NoError(t, err)
	require.NotNil(t, surface)
	require.Equal(t, "drone-alpha", surface.Username)
	require.NotNil(t, surface.Body)
	require.Equal(t, "drone-alpha", surface.Body.Username)
	require.NotEmpty(t, surface.Body.BodyID)
	require.False(t, surface.HostBridgeAvailable)
	require.False(t, surface.Executable)
	require.Equal(t, model.SoulBindingStateUnbound, surface.SoulBindingState)
	require.Equal(t, "begin", derefString(surface.NextAction))
	require.NotNil(t, surface.Workflow)
	require.NotNil(t, surface.Workflow.SoulBootstrap)
	require.Equal(t, model.SoulBootstrapPhaseNotStarted, surface.State.Phase)
	require.Equal(t, workflow.SoulBootstrapStateNotStarted, surface.State.State)
	require.Equal(t, surface.State.Phase, surface.Workflow.SoulBootstrap.Phase)

	selfSurface, err := (&queryResolver{resolver}).SoulBootstrap(round13DroneAuthContext("drone-alpha", auth.ScopeRead), "drone-alpha")
	require.NoError(t, err)
	require.NotNil(t, selfSurface)

	_, err = (&queryResolver{resolver}).SoulBootstrap(round13DroneAuthContext("stranger", auth.ScopeRead), "drone-alpha")
	require.Error(t, err)
	require.True(t, apperrors.HasCode(err, apperrors.CodeForbidden))
}

func TestRound44SoulBootstrapMutationSkeletonIsTypedAndDoesNotPersist(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	now := time.Date(2026, 6, 12, 13, 0, 0, 0, time.UTC)

	round13SeedGraphUser(t, storageRepo, &storage.User{
		Username:    "owner",
		DisplayName: "Owner",
		Approved:    true,
		CreatedAt:   now.Add(-48 * time.Hour),
		UpdatedAt:   now.Add(-24 * time.Hour),
	}, nil)
	round13SeedGraphUser(t, storageRepo, &storage.User{
		Username:    "drone-beta",
		DisplayName: "Drone Beta",
		Approved:    true,
		IsAgent:     true,
		AgentOwner:  "@owner",
		CreatedAt:   now.Add(-24 * time.Hour),
		UpdatedAt:   now,
	}, &storage.AgentGovernanceState{
		Username:        "drone-beta",
		DelegatedScopes: []string{auth.ScopeRead, auth.ScopeWrite},
	})

	payload, err := (&mutationResolver{resolver}).BeginSoulBootstrap(
		round13DroneAuthContext("owner", auth.ScopeWrite),
		model.BeginSoulBootstrapInput{
			Username:       "drone-beta",
			WalletAddress:  "0x1111111111111111111111111111111111111111",
			IdempotencyKey: round13StringPtr("begin-1"),
			CorrelationKey: round13StringPtr("corr-1"),
		},
	)
	require.NoError(t, err)
	require.NotNil(t, payload)
	require.False(t, payload.Executable)
	require.NotNil(t, payload.Error)
	require.Equal(t, workflow.SoulBootstrapErrorHostBridgeUnavailable, payload.Error.Code)
	require.Equal(t, model.SoulBootstrapPhaseError, payload.Bootstrap.State.Phase)
	require.Equal(t, workflow.SoulBootstrapStateHostBridgeUnavailable, payload.Bootstrap.State.State)
	require.NotNil(t, payload.Bootstrap.State.Correlation)
	require.Equal(t, "corr-1", derefString(payload.Bootstrap.State.Correlation.CorrelationKey))
	require.Equal(t, "begin-1", derefString(payload.Bootstrap.State.Correlation.BeginIdempotencyKey))

	storedUser, err := storageRepo.Account().GetUser(context.Background(), "drone-beta")
	require.NoError(t, err)
	storedWorkflow, err := workflow.ParseDroneWorkflowMetadata(storedUser.Metadata)
	require.NoError(t, err)
	require.Nil(t, storedWorkflow)

	_, err = (&mutationResolver{resolver}).BeginSoulBootstrap(
		round13DroneAuthContext("stranger", auth.ScopeWrite),
		model.BeginSoulBootstrapInput{
			Username:      "drone-beta",
			WalletAddress: "0x1111111111111111111111111111111111111111",
		},
	)
	require.Error(t, err)
	require.True(t, apperrors.HasCode(err, apperrors.CodeForbidden))
}

func TestRound44SoulBootstrapBindingProjectionWinsOverLocalError(t *testing.T) {
	now := time.Date(2026, 6, 12, 14, 0, 0, 0, time.UTC)
	workflowState := &workflow.DroneWorkflowState{
		SoulBootstrap: workflow.NewSoulBootstrapHostBridgeUnavailableState("drone-gamma", nil, now.Add(-time.Hour)),
	}

	projected := graphSoulBootstrapStateModel(
		workflowState,
		&storage.User{Username: "drone-gamma"},
		&storagemodels.InstanceSoulBodyBinding{
			AgentID:          "0xsoul",
			Username:         "drone-gamma",
			PrincipalAddress: "0xprincipal",
			UpdatedAt:        now,
		},
	)
	require.NotNil(t, projected)
	require.Equal(t, model.SoulBootstrapPhaseComplete, projected.Phase)
	require.Equal(t, workflow.SoulBootstrapStateCompleteBound, projected.State)
	require.Nil(t, projected.Error)
	require.Equal(t, "0xsoul", derefString(projected.HostSoulAgentID))
	require.Equal(t, "0xprincipal", derefString(projected.PrincipalAddress))
}
