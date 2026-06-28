package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	workflow "github.com/equaltoai/lesser/pkg/agents"
	"github.com/equaltoai/lesser/pkg/auth"
	soulservice "github.com/equaltoai/lesser/pkg/services/souls"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestProject51HostedRetrySameStepStartRecoversRegistrationOnlyError(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	now := time.Date(2026, 6, 28, 0, 28, 3, 0, time.UTC)
	staleErrorAt := time.Date(2026, 6, 23, 15, 24, 13, 0, time.UTC)

	beginCalls := 0
	resolver.soulsClient = &stubSoulService{
		beginHostedBootstrapFunc: func(context.Context, soulservice.BootstrapBeginInput) (*soulservice.BootstrapBeginResult, error) {
			beginCalls++
			t.Fatalf("registration-only hosted retry_same_step state must not replay by calling hosted begin")
			return nil, nil
		},
	}

	metadata, err := workflow.SetDroneWorkflowMetadata(nil, &workflow.DroneWorkflowState{
		SoulBootstrap: &workflow.SoulBootstrapState{
			Username:           "della-marlowe",
			BodyID:             "della-marlowe",
			HostRegistrationID: "oIkRoQrZNOB8E41e-107dw",
			BootstrapMode:      workflow.SoulBootstrapModeHosted,
			AuthorityModel:     workflow.SoulBootstrapAuthorityModelInstanceTrust,
			Phase:              workflow.SoulBootstrapPhaseError,
			State:              workflow.SoulBootstrapStateHostUnavailable,
			NextAction:         workflow.SoulBootstrapNextActionRetrySameStep,
			RecoveryCategory:   workflow.SoulBootstrapRecoveryCategoryRetrySameStep,
			RecoveryAction:     workflow.SoulBootstrapRecoveryActionRetrySameStep,
			Retryable:          true,
			Error: &workflow.SoulBootstrapErrorState{
				Code:             workflow.SoulBootstrapErrorHostUnavailable,
				Message:          "Soul bootstrap Host bridge is unavailable.",
				Source:           "lesser",
				HostRequestID:    "a19309460cfdf688b2d6f9701be7ecd4",
				RecoveryCategory: workflow.SoulBootstrapRecoveryCategoryRetrySameStep,
				RecoveryAction:   workflow.SoulBootstrapRecoveryActionRetrySameStep,
				Retryable:        true,
				At:               &staleErrorAt,
			},
			Correlation: &workflow.SoulBootstrapCorrelationState{
				CorrelationKey:      "corr-june-23",
				BeginIdempotencyKey: "hosted-begin-june-23",
				LastHostRequestID:   "a19309460cfdf688b2d6f9701be7ecd4",
			},
			UpdatedAt: &now,
		},
	})
	require.NoError(t, err)

	round13SeedGraphUser(t, storageRepo, &storage.User{
		Username:    "owner",
		DisplayName: "Owner",
		Approved:    true,
		CreatedAt:   now.Add(-48 * time.Hour),
		UpdatedAt:   now.Add(-24 * time.Hour),
	}, nil)
	round13SeedGraphUser(t, storageRepo, &storage.User{
		Username:    "della-marlowe",
		DisplayName: "Della Marlowe",
		Approved:    true,
		IsAgent:     true,
		AgentOwner:  "@owner",
		Metadata:    metadata,
		CreatedAt:   now.Add(-24 * time.Hour),
		UpdatedAt:   now,
	}, &storage.AgentGovernanceState{
		Username:        "della-marlowe",
		DelegatedScopes: []string{auth.ScopeRead, auth.ScopeWrite},
	})

	retryAttemptID := "retry-2026-06-28"
	payload, err := (&mutationResolver{resolver}).StartHostedSoulBootstrap(
		round13DroneAuthContext("owner", auth.ScopeWrite),
		model.StartHostedSoulBootstrapInput{
			Username:          "della-marlowe",
			IdempotencyKey:    round13StringPtr("hosted-begin-june-23"),
			RecoveryAttemptID: &retryAttemptID,
			CorrelationKey:    round13StringPtr("corr-retry-june-28"),
		},
	)
	require.NoError(t, err)
	require.Equal(t, 0, beginCalls)
	require.Nil(t, payload.Error)
	require.Nil(t, payload.Bootstrap.Error)
	require.NotNil(t, payload.Bootstrap.State)
	require.Nil(t, payload.Bootstrap.State.Error)
	require.Equal(t, model.SoulBootstrapPhaseConversation, payload.Bootstrap.State.Phase)
	require.Equal(t, workflow.SoulBootstrapStateConversationRegistrationActive, payload.Bootstrap.State.State)
	require.Equal(t, "registration_active", derefString(payload.Bootstrap.State.HostConversationStatus))
	require.Equal(t, model.SoulBootstrapNextActionSendHostedSoulGenesisMessage, payload.Bootstrap.TypedNextAction)
	require.Equal(t, "oIkRoQrZNOB8E41e-107dw", derefString(payload.Bootstrap.State.HostRegistrationID))
	require.Nil(t, payload.Bootstrap.State.HostConversationID)
	require.Nil(t, payload.Bootstrap.State.RecoveryCategory)
	require.Nil(t, payload.Bootstrap.State.RecoveryAction)
	require.False(t, payload.Bootstrap.State.Retryable)
	require.False(t, payload.Bootstrap.State.RestartRequired)
	require.NotNil(t, payload.Bootstrap.State.Correlation)
	require.Equal(t, "hosted-begin-june-23", derefString(payload.Bootstrap.State.Correlation.BeginIdempotencyKey))
	require.Equal(t, "retry-2026-06-28", derefString(payload.Bootstrap.State.Correlation.RecoveryAttemptID))

	storedUser, err := storageRepo.Account().GetUser(context.Background(), "della-marlowe")
	require.NoError(t, err)
	storedWorkflow, err := workflow.ParseDroneWorkflowMetadata(storedUser.Metadata)
	require.NoError(t, err)
	require.NotNil(t, storedWorkflow.SoulBootstrap)
	require.Nil(t, storedWorkflow.SoulBootstrap.Error)
	require.Equal(t, workflow.SoulBootstrapPhaseConversation, storedWorkflow.SoulBootstrap.Phase)
	require.Equal(t, workflow.SoulBootstrapStateConversationRegistrationActive, storedWorkflow.SoulBootstrap.State)
	require.Equal(t, workflow.SoulBootstrapNextActionSendHostedGenesisMessage, storedWorkflow.SoulBootstrap.NextAction)
	require.Empty(t, storedWorkflow.SoulBootstrap.HostConversationID)
	require.NotNil(t, storedWorkflow.SoulBootstrap.UpdatedAt)
	require.True(t, storedWorkflow.SoulBootstrap.UpdatedAt.After(staleErrorAt))
}

func TestProject51HostedBeginReplayRemainsIdempotentForActiveRegistration(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	now := time.Date(2026, 6, 28, 1, 0, 0, 0, time.UTC)
	const agentID = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	beginCalls := 0
	resolver.soulsClient = &stubSoulService{
		beginHostedBootstrapFunc: func(_ context.Context, input soulservice.BootstrapBeginInput) (*soulservice.BootstrapBeginResult, error) {
			beginCalls++
			require.Equal(t, 1, beginCalls, "active hosted begin replay must not duplicate Host begin")
			require.Equal(t, "drone-p51-active", input.Username)
			return &soulservice.BootstrapBeginResult{
				RegistrationID:  "reg_p51_active",
				HostSoulAgentID: agentID,
				AuthorityModel:  soulservice.SoulAuthorityModelInstanceTrust,
				AnchorState:     soulservice.SoulAnchorStateHostedOffchain,
				HostRequestID:   "host-req-p51-active-begin",
			}, nil
		},
	}

	round13SeedGraphUser(t, storageRepo, &storage.User{
		Username:    "owner",
		DisplayName: "Owner",
		Approved:    true,
		CreatedAt:   now.Add(-48 * time.Hour),
		UpdatedAt:   now.Add(-24 * time.Hour),
	}, nil)
	round13SeedGraphUser(t, storageRepo, &storage.User{
		Username:    "drone-p51-active",
		DisplayName: "Drone P51 Active",
		Approved:    true,
		IsAgent:     true,
		AgentOwner:  "@owner",
		CreatedAt:   now.Add(-24 * time.Hour),
		UpdatedAt:   now,
	}, &storage.AgentGovernanceState{
		Username:        "drone-p51-active",
		DelegatedScopes: []string{auth.ScopeRead, auth.ScopeWrite},
	})

	input := model.StartHostedSoulBootstrapInput{
		Username:       "drone-p51-active",
		IdempotencyKey: round13StringPtr("hosted-begin-active"),
		CorrelationKey: round13StringPtr("corr-active"),
	}
	first, err := (&mutationResolver{resolver}).StartHostedSoulBootstrap(round13DroneAuthContext("owner", auth.ScopeWrite), input)
	require.NoError(t, err)
	require.Nil(t, first.Error)
	require.Equal(t, "reg_p51_active", derefString(first.Bootstrap.State.HostRegistrationID))
	require.Equal(t, model.SoulBootstrapNextActionSendHostedSoulGenesisMessage, first.Bootstrap.TypedNextAction)

	second, err := (&mutationResolver{resolver}).StartHostedSoulBootstrap(round13DroneAuthContext("owner", auth.ScopeWrite), input)
	require.NoError(t, err)
	require.Nil(t, second.Error)
	require.Equal(t, 1, beginCalls)
	require.Equal(t, "reg_p51_active", derefString(second.Bootstrap.State.HostRegistrationID))
	require.Equal(t, agentID, derefString(second.Bootstrap.State.HostSoulAgentID))
	require.Equal(t, model.SoulBootstrapNextActionSendHostedSoulGenesisMessage, second.Bootstrap.TypedNextAction)
	require.Equal(t, workflow.SoulBootstrapStateConversationRegistrationActive, second.Bootstrap.State.State)
}
