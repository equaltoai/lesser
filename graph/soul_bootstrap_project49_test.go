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

func TestProject49HostedInProgressPersistsConversationAndBlocksPublish(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	now := time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC)

	const (
		agentID        = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		registrationID = "hreg_p49_001"
		conversationID = "hconv_p49_001"
	)

	var (
		publishCalls int
		bindCalls    int
	)
	resolver.soulsClient = &stubSoulService{
		beginHostedBootstrapFunc: func(_ context.Context, input soulservice.BootstrapBeginInput) (*soulservice.BootstrapBeginResult, error) {
			require.Equal(t, "drone-p49", input.Username)
			return &soulservice.BootstrapBeginResult{
				RegistrationID:     registrationID,
				HostSoulAgentID:    agentID,
				AuthorityModel:     soulservice.SoulAuthorityModelInstanceTrust,
				AnchorState:        soulservice.SoulAnchorStateHostedOffchain,
				RegistrationStatus: "pending",
				HostRequestID:      "host-req-p49-begin",
			}, nil
		},
		sendBootstrapConversationFunc: func(_ context.Context, input soulservice.BootstrapConversationMessageInput) (*soulservice.BootstrapConversationMessageResult, error) {
			require.Equal(t, registrationID, input.RegistrationID)
			require.Empty(t, input.ConversationID)
			require.Equal(t, "start hosted genesis", input.Message)
			return &soulservice.BootstrapConversationMessageResult{
				RegistrationID:  registrationID,
				HostSoulAgentID: agentID,
				ConversationID:  conversationID,
				Status:          "in_progress",
				MessageCount:    1,
				HostRequestID:   "host-req-p49-turn-001",
			}, nil
		},
		publishHostedBootstrapFunc: func(context.Context, soulservice.HostedBootstrapPublishInput) (*soulservice.BootstrapFinalizeResult, error) {
			publishCalls++
			t.Fatalf("PublishHostedBootstrap must not be called for in_progress hosted genesis")
			return nil, nil
		},
		bindHostedBootstrapFunc: func(context.Context, string, *soulservice.BootstrapFinalizeResult) (*soulservice.Soul, error) {
			bindCalls++
			t.Fatalf("BindHostedBootstrap must not be called for in_progress hosted genesis")
			return nil, nil
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
		Username:    "drone-p49",
		DisplayName: "Drone P49",
		Approved:    true,
		IsAgent:     true,
		AgentOwner:  "@owner",
		CreatedAt:   now.Add(-24 * time.Hour),
		UpdatedAt:   now,
	}, &storage.AgentGovernanceState{
		Username:        "drone-p49",
		DelegatedScopes: []string{auth.ScopeRead, auth.ScopeWrite},
	})

	started, err := (&mutationResolver{resolver}).StartHostedSoulBootstrap(
		round13DroneAuthContext("owner", auth.ScopeWrite),
		model.StartHostedSoulBootstrapInput{Username: "drone-p49"},
	)
	require.NoError(t, err)
	require.Nil(t, started.Error)
	require.Equal(t, registrationID, derefString(started.Bootstrap.State.HostRegistrationID))

	sent, err := (&mutationResolver{resolver}).SendHostedSoulGenesisMessage(
		round13DroneAuthContext("owner", auth.ScopeWrite),
		model.SendHostedSoulGenesisMessageInput{
			Username: "drone-p49",
			Message:  "start hosted genesis",
		},
	)
	require.NoError(t, err)
	require.Nil(t, sent.Error)
	require.Nil(t, sent.Bootstrap.State.Error)
	require.Equal(t, model.SoulBootstrapPhaseConversation, sent.Bootstrap.State.Phase)
	require.Equal(t, workflow.SoulBootstrapStateConversationInProgress, sent.Bootstrap.State.State)
	require.Equal(t, model.SoulBootstrapNextActionRefreshState, sent.Bootstrap.TypedNextAction)
	require.Equal(t, model.SoulBootstrapRecoveryCategoryRefreshState, *sent.Bootstrap.State.RecoveryCategory)
	require.Equal(t, model.SoulBootstrapRecoveryActionRefreshState, *sent.Bootstrap.State.RecoveryAction)
	require.Equal(t, conversationID, derefString(sent.Bootstrap.State.HostConversationID))
	require.Equal(t, "host-req-p49-turn-001", derefString(sent.Bootstrap.State.LastHostRequestID))
	require.Empty(t, sent.Bootstrap.State.SigningCheckpoints)

	storedUser, err := storageRepo.Account().GetUser(context.Background(), "drone-p49")
	require.NoError(t, err)
	storedWorkflow, err := workflow.ParseDroneWorkflowMetadata(storedUser.Metadata)
	require.NoError(t, err)
	require.NotNil(t, storedWorkflow.SoulBootstrap)
	require.Equal(t, conversationID, storedWorkflow.SoulBootstrap.HostConversationID)
	require.Equal(t, workflow.SoulBootstrapStateConversationInProgress, storedWorkflow.SoulBootstrap.State)

	published, err := (&mutationResolver{resolver}).PublishHostedSoul(
		round13DroneAuthContext("owner", auth.ScopeWrite),
		model.PublishHostedSoulInput{
			Username:       "drone-p49",
			ConversationID: conversationID,
		},
	)
	require.NoError(t, err)
	require.NotNil(t, published.Error)
	require.NotEqual(t, "HOST_RESPONSE_INVALID", published.Error.Code)
	require.Equal(t, workflow.SoulBootstrapErrorHostBootstrapReplayRejected, published.Error.Code)
	require.Equal(t, model.SoulBootstrapNextActionRefreshState, published.Bootstrap.TypedNextAction)
	require.Equal(t, 0, publishCalls)
	require.Equal(t, 0, bindCalls)
}
