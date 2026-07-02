package graph

import (
	"context"
	"encoding/json"
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

func TestProject51SoulBootstrapQueryRelaysAssistantReadyTranscriptAndActions(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	now := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	updatedAt := now.Add(30 * time.Second)
	createdAt := now.Add(1 * time.Second)

	const (
		agentID        = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		registrationID = "reg_p51_transcript"
		conversationID = "conv_p51_transcript"
	)

	readCalls := 0
	resolver.soulsClient = &stubSoulService{
		readBootstrapConversationFunc: func(_ context.Context, input soulservice.BootstrapConversationCompleteInput) (*soulservice.BootstrapConversationCompleteResult, error) {
			readCalls++
			require.Equal(t, registrationID, input.RegistrationID)
			require.Equal(t, conversationID, input.ConversationID)
			return &soulservice.BootstrapConversationCompleteResult{
				RegistrationID:    registrationID,
				HostSoulAgentID:   agentID,
				ConversationID:    conversationID,
				Status:            workflow.SoulBootstrapHostConversationStatusAssistantTurnReady,
				LatestTurnID:      "turn_p51_assistant_001",
				MessageCount:      2,
				MessagesTruncated: false,
				Messages: []soulservice.BootstrapConversationMessage{
					{
						ID:        "msg_000001",
						Role:      "user",
						Content:   "Describe the managed Lesser agent you are becoming.",
						Order:     1,
						CreatedAt: &createdAt,
					},
					{
						ID:      "msg_000002",
						Role:    "assistant",
						Content: "I will be a managed Lesser agent with bounded same-origin GraphQL relay.",
						Order:   2,
					},
				},
				UpdatedAt:     &updatedAt,
				HostRequestID: "host-req-p51-assistant-ready",
			}, nil
		},
	}

	metadata, err := workflow.SetDroneWorkflowMetadata(nil, &workflow.DroneWorkflowState{
		SoulBootstrap: &workflow.SoulBootstrapState{
			Username:           "drone-p51-transcript",
			BodyID:             "drone-p51-transcript",
			HostRegistrationID: registrationID,
			HostConversationID: conversationID,
			HostSoulAgentID:    agentID,
			BootstrapMode:      workflow.SoulBootstrapModeHosted,
			AuthorityModel:     workflow.SoulBootstrapAuthorityModelInstanceTrust,
			AnchorState:        workflow.SoulBootstrapAnchorStateHostedOffchain,
			AssuranceState:     workflow.SoulBootstrapAnchorStateHostedOffchain,
			Phase:              workflow.SoulBootstrapPhaseConversation,
			State:              workflow.SoulBootstrapStateConversationInProgress,
			NextAction:         workflow.SoulBootstrapNextActionRefreshState,
			RecoveryCategory:   workflow.SoulBootstrapRecoveryCategoryRefreshState,
			RecoveryAction:     workflow.SoulBootstrapRecoveryActionRefreshState,
			UpdatedAt:          &now,
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
		Username:    "drone-p51-transcript",
		DisplayName: "Drone P51 Transcript",
		Approved:    true,
		IsAgent:     true,
		AgentOwner:  "@owner",
		Metadata:    metadata,
		CreatedAt:   now.Add(-24 * time.Hour),
		UpdatedAt:   now,
	}, &storage.AgentGovernanceState{
		Username:        "drone-p51-transcript",
		DelegatedScopes: []string{auth.ScopeRead, auth.ScopeWrite},
	})

	surface, err := (&queryResolver{resolver}).SoulBootstrap(round13DroneAuthContext("owner", auth.ScopeRead), "drone-p51-transcript")
	require.NoError(t, err)
	require.Equal(t, 1, readCalls)
	require.NotNil(t, surface.State.HostedGenesisConversation)
	conversation := surface.State.HostedGenesisConversation
	require.Equal(t, conversationID, conversation.ConversationID)
	require.Equal(t, registrationID, derefString(conversation.RegistrationID))
	require.Equal(t, workflow.SoulBootstrapHostConversationStatusAssistantTurnReady, conversation.Status)
	require.Equal(t, "turn_p51_assistant_001", derefString(conversation.LatestTurnID))
	require.Equal(t, 2, conversation.MessageCount)
	require.Equal(t, "host-req-p51-assistant-ready", derefString(conversation.RequestID))
	require.NotNil(t, conversation.UpdatedAt)
	require.Equal(t, updatedAt.UTC(), time.Time(*conversation.UpdatedAt))
	require.False(t, conversation.MessagesTruncated)
	require.Len(t, conversation.Messages, 2)
	require.Equal(t, "msg_000001", conversation.Messages[0].ID)
	require.Equal(t, model.SoulBootstrapHostedGenesisMessageRoleUser, conversation.Messages[0].Role)
	require.Equal(t, "Describe the managed Lesser agent you are becoming.", conversation.Messages[0].Content)
	require.NotNil(t, conversation.Messages[0].CreatedAt)
	require.Equal(t, model.SoulBootstrapHostedGenesisMessageRoleAssistant, conversation.Messages[1].Role)
	require.Contains(t, conversation.Messages[1].Content, "same-origin GraphQL relay")
	require.Equal(t, model.SoulBootstrapNextActionCompleteHostedSoulGenesis, surface.TypedNextAction)
	require.ElementsMatch(t, []model.SoulBootstrapNextAction{
		model.SoulBootstrapNextActionSendHostedSoulGenesisMessage,
		model.SoulBootstrapNextActionCompleteHostedSoulGenesis,
	}, surface.AvailableActions)
	require.ElementsMatch(t, surface.AvailableActions, surface.State.AvailableActions)
	require.False(t, surface.State.PublishGate.CanPublishHostedSoul)
	require.Equal(t, "blocked:terminal_declaration_evidence_absent", surface.State.PublishGate.Reason)
}

func TestProject51SendHostedSoulGenesisMessageContinuesAssistantReadyConversation(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	now := time.Date(2026, 6, 28, 13, 0, 0, 0, time.UTC)
	updatedAt := now.Add(time.Minute)

	const (
		agentID        = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		registrationID = "reg_p51_continue"
		conversationID = "conv_p51_continue"
	)

	sendCalls := 0
	resolver.soulsClient = &stubSoulService{
		sendBootstrapConversationFunc: func(_ context.Context, input soulservice.BootstrapConversationMessageInput) (*soulservice.BootstrapConversationMessageResult, error) {
			sendCalls++
			require.Equal(t, registrationID, input.RegistrationID)
			require.Equal(t, conversationID, input.ConversationID)
			require.Equal(t, "Add one more boundary.", input.Message)
			return &soulservice.BootstrapConversationMessageResult{
				RegistrationID:    registrationID,
				HostSoulAgentID:   agentID,
				ConversationID:    conversationID,
				Status:            workflow.SoulBootstrapHostConversationStatusAssistantTurnReady,
				LatestTurnID:      "turn_p51_assistant_002",
				MessageCount:      4,
				MessagesTruncated: false,
				Messages: []soulservice.BootstrapConversationMessage{
					{ID: "msg_000001", Role: "user", Content: "Describe yourself.", Order: 1},
					{ID: "msg_000002", Role: "assistant", Content: "I am a hosted Lesser soul.", Order: 2},
					{ID: "msg_000003", Role: "user", Content: "Add one more boundary.", Order: 3},
					{ID: "msg_000004", Role: "assistant", Content: "I will keep operator credentials private.", Order: 4},
				},
				UpdatedAt:     &updatedAt,
				HostRequestID: "host-req-p51-continue",
			}, nil
		},
	}

	metadata, err := workflow.SetDroneWorkflowMetadata(nil, &workflow.DroneWorkflowState{
		SoulBootstrap: &workflow.SoulBootstrapState{
			Username:           "drone-p51-continue",
			BodyID:             "drone-p51-continue",
			HostRegistrationID: registrationID,
			HostConversationID: conversationID,
			HostSoulAgentID:    agentID,
			BootstrapMode:      workflow.SoulBootstrapModeHosted,
			AuthorityModel:     workflow.SoulBootstrapAuthorityModelInstanceTrust,
			AnchorState:        workflow.SoulBootstrapAnchorStateHostedOffchain,
			AssuranceState:     workflow.SoulBootstrapAnchorStateHostedOffchain,
			Phase:              workflow.SoulBootstrapPhaseConversation,
			State:              workflow.SoulBootstrapStateConversationAssistantTurnReady,
			NextAction:         workflow.SoulBootstrapNextActionCompleteHostedGenesis,
			UpdatedAt:          &now,
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
		Username:    "drone-p51-continue",
		DisplayName: "Drone P51 Continue",
		Approved:    true,
		IsAgent:     true,
		AgentOwner:  "@owner",
		Metadata:    metadata,
		CreatedAt:   now.Add(-24 * time.Hour),
		UpdatedAt:   now,
	}, &storage.AgentGovernanceState{
		Username:        "drone-p51-continue",
		DelegatedScopes: []string{auth.ScopeRead, auth.ScopeWrite},
	})

	payload, err := (&mutationResolver{resolver}).SendHostedSoulGenesisMessage(
		round13DroneAuthContext("owner", auth.ScopeWrite),
		model.SendHostedSoulGenesisMessageInput{
			Username: "drone-p51-continue",
			Message:  "Add one more boundary.",
		},
	)
	require.NoError(t, err)
	require.Equal(t, 1, sendCalls)
	require.Nil(t, payload.Error)
	require.NotNil(t, payload.Bootstrap.State.HostedGenesisConversation)
	require.Equal(t, conversationID, payload.Bootstrap.State.HostedGenesisConversation.ConversationID)
	require.Equal(t, 4, payload.Bootstrap.State.HostedGenesisConversation.MessageCount)
	require.Equal(t, "turn_p51_assistant_002", derefString(payload.Bootstrap.State.HostedGenesisConversation.LatestTurnID))
	require.Len(t, payload.Bootstrap.State.HostedGenesisConversation.Messages, 4)
	require.Equal(t, model.SoulBootstrapNextActionCompleteHostedSoulGenesis, payload.Bootstrap.TypedNextAction)
	require.ElementsMatch(t, []model.SoulBootstrapNextAction{
		model.SoulBootstrapNextActionSendHostedSoulGenesisMessage,
		model.SoulBootstrapNextActionCompleteHostedSoulGenesis,
	}, payload.Bootstrap.AvailableActions)
	require.False(t, payload.Bootstrap.State.PublishGate.CanPublishHostedSoul)
}

func TestProject51HostedInProgressReadAdvertisesSendAndRelaysTranscript(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	updatedAt := now.Add(2 * time.Minute)

	const (
		agentID        = "0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
		registrationID = "reg_p51_in_progress"
		conversationID = "conv_p51_in_progress"
	)

	readCalls := 0
	resolver.soulsClient = &stubSoulService{
		readBootstrapConversationFunc: func(_ context.Context, input soulservice.BootstrapConversationCompleteInput) (*soulservice.BootstrapConversationCompleteResult, error) {
			readCalls++
			require.Equal(t, registrationID, input.RegistrationID)
			require.Equal(t, conversationID, input.ConversationID)
			return &soulservice.BootstrapConversationCompleteResult{
				RegistrationID:    registrationID,
				HostSoulAgentID:   agentID,
				ConversationID:    conversationID,
				Status:            workflow.SoulBootstrapHostConversationStatusInProgress,
				LatestTurnID:      "turn_p51_in_progress_002",
				MessageCount:      3,
				MessagesTruncated: false,
				Messages: []soulservice.BootstrapConversationMessage{
					{ID: "msg_000001", Role: "user", Content: "Describe the soul.", Order: 1},
					{ID: "msg_000002", Role: "assistant", Content: "I need one more boundary.", Order: 2},
					{ID: "msg_000003", Role: "user", Content: "Add the boundary.", Order: 3},
				},
				UpdatedAt:     &updatedAt,
				HostRequestID: "host-req-p51-in-progress",
			}, nil
		},
	}

	metadata, err := workflow.SetDroneWorkflowMetadata(nil, &workflow.DroneWorkflowState{
		SoulBootstrap: &workflow.SoulBootstrapState{
			Username:           "drone-p51-in-progress",
			BodyID:             "drone-p51-in-progress",
			HostRegistrationID: registrationID,
			HostConversationID: conversationID,
			HostSoulAgentID:    agentID,
			BootstrapMode:      workflow.SoulBootstrapModeHosted,
			AuthorityModel:     workflow.SoulBootstrapAuthorityModelInstanceTrust,
			AnchorState:        workflow.SoulBootstrapAnchorStateHostedOffchain,
			AssuranceState:     workflow.SoulBootstrapAnchorStateHostedOffchain,
			Phase:              workflow.SoulBootstrapPhaseConversation,
			State:              workflow.SoulBootstrapStateConversationInProgress,
			NextAction:         workflow.SoulBootstrapNextActionRefreshState,
			RecoveryCategory:   workflow.SoulBootstrapRecoveryCategoryRefreshState,
			RecoveryAction:     workflow.SoulBootstrapRecoveryActionRefreshState,
			UpdatedAt:          &now,
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
		Username:    "drone-p51-in-progress",
		DisplayName: "Drone P51 In Progress",
		Approved:    true,
		IsAgent:     true,
		AgentOwner:  "@owner",
		Metadata:    metadata,
		CreatedAt:   now.Add(-24 * time.Hour),
		UpdatedAt:   now,
	}, &storage.AgentGovernanceState{
		Username:        "drone-p51-in-progress",
		DelegatedScopes: []string{auth.ScopeRead, auth.ScopeWrite},
	})

	surface, err := (&queryResolver{resolver}).SoulBootstrap(round13DroneAuthContext("owner", auth.ScopeRead), "drone-p51-in-progress")
	require.NoError(t, err)
	require.Equal(t, 1, readCalls)
	require.Equal(t, workflow.SoulBootstrapStateConversationInProgress, surface.State.State)
	require.Equal(t, workflow.SoulBootstrapHostConversationStatusInProgress, derefString(surface.State.HostConversationStatus))
	require.Equal(t, model.SoulBootstrapNextActionSendHostedSoulGenesisMessage, surface.TypedNextAction)
	require.ElementsMatch(t, []model.SoulBootstrapNextAction{
		model.SoulBootstrapNextActionSendHostedSoulGenesisMessage,
	}, surface.AvailableActions)
	require.ElementsMatch(t, surface.AvailableActions, surface.State.AvailableActions)
	require.Nil(t, surface.State.RecoveryCategory)
	require.Nil(t, surface.State.RecoveryAction)

	require.NotNil(t, surface.State.HostedGenesisConversation)
	conversation := surface.State.HostedGenesisConversation
	require.Equal(t, conversationID, conversation.ConversationID)
	require.Equal(t, workflow.SoulBootstrapHostConversationStatusInProgress, conversation.Status)
	require.Equal(t, 3, conversation.MessageCount)
	require.Equal(t, "turn_p51_in_progress_002", derefString(conversation.LatestTurnID))
	require.False(t, conversation.MessagesTruncated)
	require.Len(t, conversation.Messages, 3)
	require.Equal(t, model.SoulBootstrapHostedGenesisMessageRoleUser, conversation.Messages[0].Role)
	require.Equal(t, "Describe the soul.", conversation.Messages[0].Content)
	require.Equal(t, model.SoulBootstrapHostedGenesisMessageRoleAssistant, conversation.Messages[1].Role)
	require.Equal(t, "I need one more boundary.", conversation.Messages[1].Content)
	require.Equal(t, model.SoulBootstrapHostedGenesisMessageRoleUser, conversation.Messages[2].Role)
	require.Equal(t, "Add the boundary.", conversation.Messages[2].Content)
}

func TestProject51HostedConversationAvailableActionsByStatus(t *testing.T) {
	now := time.Date(2026, 6, 29, 13, 0, 0, 0, time.UTC)
	agent := &storage.User{Username: "drone-p51-actions"}
	validDeclaration := `{"selfDescription":{"summary":"ready"},"capabilities":[],"boundaries":[],"transparency":{}}`

	tests := []struct {
		name             string
		hostStatus       string
		produced         string
		failureRecovery  string
		wantState        string
		wantNext         model.SoulBootstrapNextAction
		wantAvailable    []model.SoulBootstrapNextAction
		wantRecoveryKind model.SoulBootstrapRecoveryCategory
	}{
		{
			name:       "created",
			hostStatus: workflow.SoulBootstrapHostConversationStatusCreated,
			wantState:  workflow.SoulBootstrapStateConversationInProgress,
			wantNext:   model.SoulBootstrapNextActionSendHostedSoulGenesisMessage,
			wantAvailable: []model.SoulBootstrapNextAction{
				model.SoulBootstrapNextActionSendHostedSoulGenesisMessage,
			},
		},
		{
			name:       "in_progress",
			hostStatus: workflow.SoulBootstrapHostConversationStatusInProgress,
			wantState:  workflow.SoulBootstrapStateConversationInProgress,
			wantNext:   model.SoulBootstrapNextActionSendHostedSoulGenesisMessage,
			wantAvailable: []model.SoulBootstrapNextAction{
				model.SoulBootstrapNextActionSendHostedSoulGenesisMessage,
			},
		},
		{
			name:       "assistant_turn_ready",
			hostStatus: workflow.SoulBootstrapHostConversationStatusAssistantTurnReady,
			wantState:  workflow.SoulBootstrapStateConversationAssistantTurnReady,
			wantNext:   model.SoulBootstrapNextActionCompleteHostedSoulGenesis,
			wantAvailable: []model.SoulBootstrapNextAction{
				model.SoulBootstrapNextActionSendHostedSoulGenesisMessage,
				model.SoulBootstrapNextActionCompleteHostedSoulGenesis,
			},
		},
		{
			name:       "declaration_extraction_pending",
			hostStatus: workflow.SoulBootstrapHostConversationStatusDeclarationExtractionPending,
			wantState:  workflow.SoulBootstrapStateConversationDeclarationExtractionPending,
			wantNext:   model.SoulBootstrapNextActionSendHostedSoulGenesisMessage,
			wantAvailable: []model.SoulBootstrapNextAction{
				model.SoulBootstrapNextActionSendHostedSoulGenesisMessage,
			},
		},
		{
			name:       "declaration_ready",
			hostStatus: workflow.SoulBootstrapHostConversationStatusDeclarationReady,
			produced:   validDeclaration,
			wantState:  workflow.SoulBootstrapStateConversationDeclarationReady,
			wantNext:   model.SoulBootstrapNextActionPublishHostedSoul,
			wantAvailable: []model.SoulBootstrapNextAction{
				model.SoulBootstrapNextActionPublishHostedSoul,
			},
		},
		{
			name:            "failed",
			hostStatus:      workflow.SoulBootstrapHostConversationStatusFailed,
			failureRecovery: workflow.SoulBootstrapRecoveryActionRetrySameStep,
			wantState:       workflow.SoulBootstrapStateHostFailed,
			wantNext:        model.SoulBootstrapNextActionRetrySameStep,
			wantAvailable: []model.SoulBootstrapNextAction{
				model.SoulBootstrapNextActionRetrySameStep,
			},
			wantRecoveryKind: model.SoulBootstrapRecoveryCategoryRetrySameStep,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			result := &soulservice.BootstrapConversationCompleteResult{
				RegistrationID:        "reg_p51_actions",
				HostSoulAgentID:       "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
				ConversationID:        "conv_p51_actions",
				Status:                tt.hostStatus,
				ProducedDeclarations:  tt.produced,
				FailureRetryable:      true,
				FailureRecoveryAction: tt.failureRecovery,
				HostRequestID:         "host-req-p51-actions",
			}

			stored := soulBootstrapStateAfterHostedConversationSnapshot(agent, nil, nil, result, now)
			projected := graphSoulBootstrapStoredStateModel(stored)

			require.Equal(t, tt.wantState, projected.State)
			require.Equal(t, tt.wantNext, projected.TypedNextAction)
			require.ElementsMatch(t, tt.wantAvailable, projected.AvailableActions)
			if tt.wantRecoveryKind == "" {
				require.Nil(t, projected.RecoveryCategory)
				require.Nil(t, projected.RecoveryAction)
			} else {
				require.Equal(t, tt.wantRecoveryKind, *projected.RecoveryCategory)
			}
		})
	}
}

func TestProject51HostedTranscriptProjectionOmitsUnsafeHostCredentialMaterial(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	now := time.Date(2026, 6, 28, 14, 0, 0, 0, time.UTC)

	const (
		agentID        = "0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
		registrationID = "reg_p51_secret"
		conversationID = "conv_p51_secret"
	)

	resolver.soulsClient = &stubSoulService{
		readBootstrapConversationFunc: func(_ context.Context, input soulservice.BootstrapConversationCompleteInput) (*soulservice.BootstrapConversationCompleteResult, error) {
			require.Equal(t, registrationID, input.RegistrationID)
			require.Equal(t, conversationID, input.ConversationID)
			return &soulservice.BootstrapConversationCompleteResult{
				RegistrationID:    registrationID,
				HostSoulAgentID:   agentID,
				ConversationID:    conversationID,
				Status:            workflow.SoulBootstrapHostConversationStatusAssistantTurnReady,
				LatestTurnID:      "turn_p51_secret",
				MessageCount:      2,
				MessagesTruncated: false,
				Messages: []soulservice.BootstrapConversationMessage{
					{ID: "msg_000001", Role: "user", Content: "safe prompt", Order: 1},
					{ID: "msg_000002", Role: "assistant", Content: "AWS_SECRET_ACCESS_KEY=do-not-project", Order: 2},
				},
				HostRequestID: "host-req-p51-secret",
			}, nil
		},
	}

	metadata, err := workflow.SetDroneWorkflowMetadata(nil, &workflow.DroneWorkflowState{
		SoulBootstrap: &workflow.SoulBootstrapState{
			Username:           "drone-p51-secret",
			BodyID:             "drone-p51-secret",
			HostRegistrationID: registrationID,
			HostConversationID: conversationID,
			HostSoulAgentID:    agentID,
			BootstrapMode:      workflow.SoulBootstrapModeHosted,
			AuthorityModel:     workflow.SoulBootstrapAuthorityModelInstanceTrust,
			AnchorState:        workflow.SoulBootstrapAnchorStateHostedOffchain,
			AssuranceState:     workflow.SoulBootstrapAnchorStateHostedOffchain,
			Phase:              workflow.SoulBootstrapPhaseConversation,
			State:              workflow.SoulBootstrapStateConversationInProgress,
			NextAction:         workflow.SoulBootstrapNextActionRefreshState,
			UpdatedAt:          &now,
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
		Username:    "drone-p51-secret",
		DisplayName: "Drone P51 Secret",
		Approved:    true,
		IsAgent:     true,
		AgentOwner:  "@owner",
		Metadata:    metadata,
		CreatedAt:   now.Add(-24 * time.Hour),
		UpdatedAt:   now,
	}, &storage.AgentGovernanceState{
		Username:        "drone-p51-secret",
		DelegatedScopes: []string{auth.ScopeRead, auth.ScopeWrite},
	})

	surface, err := (&queryResolver{resolver}).SoulBootstrap(round13DroneAuthContext("owner", auth.ScopeRead), "drone-p51-secret")
	require.NoError(t, err)
	require.NotNil(t, surface.State.HostedGenesisConversation)
	require.Empty(t, surface.State.HostedGenesisConversation.Messages)
	require.True(t, surface.State.HostedGenesisConversation.MessagesTruncated)
	encoded, err := json.Marshal(surface.State.HostedGenesisConversation)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "AWS_SECRET_ACCESS_KEY")
	require.NotContains(t, string(encoded), "do-not-project")
	require.NotContains(t, string(encoded), "Bearer ")
}

func TestProject51RecoverHostedSoulGenesisTurnCallsHostRecoverWithoutUserMessage(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	now := time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)

	const (
		agentID        = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		registrationID = "hreg_p51_recover"
		conversationID = "hconv_p51_recover"
	)

	var (
		recoverCalls     int
		sendMessageCalls int
	)
	resolver.soulsClient = &stubSoulService{
		beginHostedBootstrapFunc: func(_ context.Context, input soulservice.BootstrapBeginInput) (*soulservice.BootstrapBeginResult, error) {
			return &soulservice.BootstrapBeginResult{
				RegistrationID:     registrationID,
				HostSoulAgentID:    agentID,
				AuthorityModel:     soulservice.SoulAuthorityModelInstanceTrust,
				AnchorState:        soulservice.SoulAnchorStateHostedOffchain,
				RegistrationStatus: "pending",
				HostRequestID:      "host-req-p51-begin",
			}, nil
		},
		sendBootstrapConversationFunc: func(_ context.Context, input soulservice.BootstrapConversationMessageInput) (*soulservice.BootstrapConversationMessageResult, error) {
			sendMessageCalls++
			require.Equal(t, "start genesis", input.Message)
			return &soulservice.BootstrapConversationMessageResult{
				RegistrationID:  registrationID,
				HostSoulAgentID: agentID,
				ConversationID:  conversationID,
				Status:          "in_progress",
				MessageCount:    1,
				HostRequestID:   "host-req-p51-send",
			}, nil
		},
		recoverHostedGenesisTurnFunc: func(_ context.Context, input soulservice.BootstrapConversationRecoverInput) (*soulservice.BootstrapConversationCompleteResult, error) {
			recoverCalls++
			require.Equal(t, registrationID, input.RegistrationID)
			require.Equal(t, conversationID, input.ConversationID)
			return &soulservice.BootstrapConversationCompleteResult{
				RegistrationID:  registrationID,
				HostSoulAgentID: agentID,
				ConversationID:  conversationID,
				Status:          "assistant_turn_ready",
				LatestTurnID:    "turn_recovered",
				MessageCount:    2,
				HostRequestID:   "host-req-p51-recover",
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
		Username:    "drone-p51",
		DisplayName: "Drone P51",
		Approved:    true,
		IsAgent:     true,
		AgentOwner:  "@owner",
		CreatedAt:   now.Add(-24 * time.Hour),
		UpdatedAt:   now,
	}, &storage.AgentGovernanceState{
		Username:        "drone-p51",
		DelegatedScopes: []string{auth.ScopeRead, auth.ScopeWrite},
	})

	started, err := (&mutationResolver{resolver}).StartHostedSoulBootstrap(
		round13DroneAuthContext("owner", auth.ScopeWrite),
		model.StartHostedSoulBootstrapInput{Username: "drone-p51"},
	)
	require.NoError(t, err)
	require.Nil(t, started.Error)

	sent, err := (&mutationResolver{resolver}).SendHostedSoulGenesisMessage(
		round13DroneAuthContext("owner", auth.ScopeWrite),
		model.SendHostedSoulGenesisMessageInput{
			Username: "drone-p51",
			Message:  "start genesis",
		},
	)
	require.NoError(t, err)
	require.Nil(t, sent.Error)
	require.Equal(t, conversationID, derefString(sent.Bootstrap.State.HostConversationID))
	require.Equal(t, 1, sendMessageCalls)

	recovered, err := (&mutationResolver{resolver}).RecoverHostedSoulGenesisTurn(
		round13DroneAuthContext("owner", auth.ScopeWrite),
		model.RecoverHostedSoulGenesisTurnInput{
			Username:       "drone-p51",
			ConversationID: conversationID,
		},
	)
	require.NoError(t, err)
	require.Nil(t, recovered.Error)
	require.Equal(t, 1, recoverCalls)
	require.Equal(t, 1, sendMessageCalls, "recovery must not send a new user message")

	require.Equal(t, model.SoulBootstrapPhaseConversation, recovered.Bootstrap.State.Phase)
	require.Equal(t, workflow.SoulBootstrapStateConversationAssistantTurnReady, recovered.Bootstrap.State.State)
	require.Equal(t, model.SoulBootstrapNextActionCompleteHostedSoulGenesis, recovered.Bootstrap.TypedNextAction)
	require.Equal(t, "assistant_turn_ready", derefString(recovered.Bootstrap.State.HostConversationStatus))
	require.Equal(t, 2, recovered.Bootstrap.State.HostedGenesisConversation.MessageCount)
	require.Equal(t, "host-req-p51-recover", derefString(recovered.Bootstrap.State.LastHostRequestID))
}

func TestProject51RecoverHostedSoulGenesisTurnIsIdempotentForNonStuckSession(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	now := time.Date(2026, 7, 2, 11, 0, 0, 0, time.UTC)

	const (
		agentID        = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		registrationID = "hreg_p51_idem"
		conversationID = "hconv_p51_idem"
	)

	var recoverCalls int
	resolver.soulsClient = &stubSoulService{
		beginHostedBootstrapFunc: func(_ context.Context, input soulservice.BootstrapBeginInput) (*soulservice.BootstrapBeginResult, error) {
			return &soulservice.BootstrapBeginResult{
				RegistrationID:     registrationID,
				HostSoulAgentID:    agentID,
				AuthorityModel:     soulservice.SoulAuthorityModelInstanceTrust,
				AnchorState:        soulservice.SoulAnchorStateHostedOffchain,
				RegistrationStatus: "pending",
				HostRequestID:      "host-req-p51-idem-begin",
			}, nil
		},
		sendBootstrapConversationFunc: func(_ context.Context, input soulservice.BootstrapConversationMessageInput) (*soulservice.BootstrapConversationMessageResult, error) {
			return &soulservice.BootstrapConversationMessageResult{
				RegistrationID:  registrationID,
				HostSoulAgentID: agentID,
				ConversationID:  conversationID,
				Status:          "in_progress",
				MessageCount:    1,
				HostRequestID:   "host-req-p51-idem-send",
			}, nil
		},
		recoverHostedGenesisTurnFunc: func(_ context.Context, input soulservice.BootstrapConversationRecoverInput) (*soulservice.BootstrapConversationCompleteResult, error) {
			recoverCalls++
			return &soulservice.BootstrapConversationCompleteResult{
				RegistrationID:  registrationID,
				HostSoulAgentID: agentID,
				ConversationID:  conversationID,
				Status:          "in_progress",
				LatestTurnID:    "turn_user_001",
				MessageCount:    1,
				HostRequestID:   "host-req-p51-idem-recover",
			}, nil
		},
	}

	round13SeedGraphUser(t, storageRepo, &storage.User{
		Username:    "owner",
		DisplayName: "Owner",
		Approved:    true,
		CreatedAt:   now.Add(-48 * time.Hour),
	}, nil)
	round13SeedGraphUser(t, storageRepo, &storage.User{
		Username:    "drone-p51-idem",
		DisplayName: "Drone P51 Idem",
		Approved:    true,
		IsAgent:     true,
		AgentOwner:  "@owner",
		CreatedAt:   now.Add(-24 * time.Hour),
	}, &storage.AgentGovernanceState{
		Username:        "drone-p51-idem",
		DelegatedScopes: []string{auth.ScopeRead, auth.ScopeWrite},
	})

	started, err := (&mutationResolver{resolver}).StartHostedSoulBootstrap(
		round13DroneAuthContext("owner", auth.ScopeWrite),
		model.StartHostedSoulBootstrapInput{Username: "drone-p51-idem"},
	)
	require.NoError(t, err)
	require.Nil(t, started.Error)

	sent, err := (&mutationResolver{resolver}).SendHostedSoulGenesisMessage(
		round13DroneAuthContext("owner", auth.ScopeWrite),
		model.SendHostedSoulGenesisMessageInput{
			Username: "drone-p51-idem",
			Message:  "start genesis",
		},
	)
	require.NoError(t, err)
	require.Nil(t, sent.Error)

	recovered, err := (&mutationResolver{resolver}).RecoverHostedSoulGenesisTurn(
		round13DroneAuthContext("owner", auth.ScopeWrite),
		model.RecoverHostedSoulGenesisTurnInput{
			Username:       "drone-p51-idem",
			ConversationID: conversationID,
		},
	)
	require.NoError(t, err)
	require.Nil(t, recovered.Error)
	require.Equal(t, 1, recoverCalls)
	require.Equal(t, model.SoulBootstrapPhaseConversation, recovered.Bootstrap.State.Phase)
	require.Equal(t, workflow.SoulBootstrapStateConversationInProgress, recovered.Bootstrap.State.State)
	require.Equal(t, "in_progress", derefString(recovered.Bootstrap.State.HostConversationStatus))
	require.Equal(t, 1, recovered.Bootstrap.State.HostedGenesisConversation.MessageCount)
}

func TestProject51RecoverHostedSoulGenesisTurnRequiresWriteScope(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)

	_, err := (&mutationResolver{resolver}).RecoverHostedSoulGenesisTurn(
		round13DroneAuthContext("owner", auth.ScopeRead),
		model.RecoverHostedSoulGenesisTurnInput{
			Username:       "drone-p51",
			ConversationID: "conv_001",
		},
	)
	require.Error(t, err)
}

func TestProject51RecoverHostedSoulGenesisTurnRequiresAuth(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)

	_, err := (&mutationResolver{resolver}).RecoverHostedSoulGenesisTurn(
		context.Background(),
		model.RecoverHostedSoulGenesisTurnInput{
			Username:       "drone-p51",
			ConversationID: "conv_001",
		},
	)
	require.Error(t, err)
}

func TestProject51ListHostedGenesisConversationsReturnsSummaries(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)

	const (
		agentID        = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		registrationID = "hreg_p51_list"
	)

	var listCalls int
	resolver.soulsClient = &stubSoulService{
		beginHostedBootstrapFunc: func(_ context.Context, input soulservice.BootstrapBeginInput) (*soulservice.BootstrapBeginResult, error) {
			return &soulservice.BootstrapBeginResult{
				RegistrationID:     registrationID,
				HostSoulAgentID:    agentID,
				AuthorityModel:     soulservice.SoulAuthorityModelInstanceTrust,
				AnchorState:        soulservice.SoulAnchorStateHostedOffchain,
				RegistrationStatus: "pending",
				HostRequestID:      "host-req-p51-list-begin",
			}, nil
		},
		listHostedGenesisConversationsFunc: func(_ context.Context, agentIDArg string) ([]soulservice.HostedGenesisConversationSummary, error) {
			listCalls++
			require.Equal(t, agentID, agentIDArg)
			return []soulservice.HostedGenesisConversationSummary{
				{
					ConversationID: "conv_list_001",
					RegistrationID: registrationID,
					Status:         "in_progress",
					MessageCount:   2,
					LatestTurnID:   "turn_001",
					CreatedAt:      &now,
					UpdatedAt:      &now,
				},
				{
					ConversationID: "conv_list_002",
					RegistrationID: registrationID,
					Status:         "declaration_ready",
					MessageCount:   4,
					LatestTurnID:   "turn_004",
				},
			}, nil
		},
	}

	round13SeedGraphUser(t, storageRepo, &storage.User{
		Username:    "owner",
		DisplayName: "Owner",
		Approved:    true,
		CreatedAt:   now.Add(-48 * time.Hour),
	}, nil)
	round13SeedGraphUser(t, storageRepo, &storage.User{
		Username:    "drone-p51-list",
		DisplayName: "Drone P51 List",
		Approved:    true,
		IsAgent:     true,
		AgentOwner:  "@owner",
		CreatedAt:   now.Add(-24 * time.Hour),
	}, &storage.AgentGovernanceState{
		Username:        "drone-p51-list",
		DelegatedScopes: []string{auth.ScopeRead, auth.ScopeWrite},
	})

	started, err := (&mutationResolver{resolver}).StartHostedSoulBootstrap(
		round13DroneAuthContext("owner", auth.ScopeWrite),
		model.StartHostedSoulBootstrapInput{Username: "drone-p51-list"},
	)
	require.NoError(t, err)
	require.Nil(t, started.Error)

	summaries, err := (&queryResolver{resolver}).ListHostedGenesisConversations(
		round13DroneAuthContext("owner", auth.ScopeRead),
		"drone-p51-list",
	)
	require.NoError(t, err)
	require.Equal(t, 1, listCalls)
	require.Len(t, summaries, 2)

	require.Equal(t, "conv_list_001", summaries[0].ConversationID)
	require.Equal(t, "in_progress", summaries[0].Status)
	require.Equal(t, 2, summaries[0].MessageCount)
	require.NotNil(t, summaries[0].CreatedAt)
	require.NotNil(t, summaries[0].UpdatedAt)

	require.Equal(t, "conv_list_002", summaries[1].ConversationID)
	require.Equal(t, "declaration_ready", summaries[1].Status)
	require.Equal(t, 4, summaries[1].MessageCount)
}

func TestProject51ListHostedGenesisConversationsReturnsEmptyForNoAgentID(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	now := time.Date(2026, 7, 2, 13, 0, 0, 0, time.UTC)

	var listCalls int
	resolver.soulsClient = &stubSoulService{
		listHostedGenesisConversationsFunc: func(context.Context, string) ([]soulservice.HostedGenesisConversationSummary, error) {
			listCalls++
			return nil, nil
		},
	}

	round13SeedGraphUser(t, storageRepo, &storage.User{
		Username:    "owner",
		DisplayName: "Owner",
		Approved:    true,
		CreatedAt:   now.Add(-48 * time.Hour),
	}, nil)
	round13SeedGraphUser(t, storageRepo, &storage.User{
		Username:    "drone-p51-noagent",
		DisplayName: "Drone NoAgent",
		Approved:    true,
		IsAgent:     true,
		AgentOwner:  "@owner",
		CreatedAt:   now.Add(-24 * time.Hour),
	}, &storage.AgentGovernanceState{
		Username:        "drone-p51-noagent",
		DelegatedScopes: []string{auth.ScopeRead, auth.ScopeWrite},
	})

	summaries, err := (&queryResolver{resolver}).ListHostedGenesisConversations(
		round13DroneAuthContext("owner", auth.ScopeRead),
		"drone-p51-noagent",
	)
	require.NoError(t, err)
	require.Empty(t, summaries)
	require.Equal(t, 0, listCalls, "service must not be called when there is no agent ID")
}

func TestProject51ListHostedGenesisConversationsRequiresAuth(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)

	_, err := (&queryResolver{resolver}).ListHostedGenesisConversations(
		context.Background(),
		"drone-p51",
	)
	require.Error(t, err)
}
