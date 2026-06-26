package graph

import (
	"context"
	"strings"
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
	require.Equal(t, workflow.SoulBootstrapHostConversationStatusInProgress, derefString(sent.Bootstrap.State.HostConversationStatus))
	require.Equal(t, model.SoulBootstrapNextActionRefreshState, sent.Bootstrap.TypedNextAction)
	require.Equal(t, model.SoulBootstrapRecoveryCategoryRefreshState, *sent.Bootstrap.State.RecoveryCategory)
	require.Equal(t, model.SoulBootstrapRecoveryActionRefreshState, *sent.Bootstrap.State.RecoveryAction)
	require.Equal(t, conversationID, derefString(sent.Bootstrap.State.HostConversationID))
	require.Equal(t, "host-req-p49-turn-001", derefString(sent.Bootstrap.State.LastHostRequestID))
	require.Empty(t, sent.Bootstrap.State.SigningCheckpoints)
	require.Nil(t, sent.Bootstrap.State.TerminalDeclarationEvidence)
	require.NotNil(t, sent.Bootstrap.State.PublishGate)
	require.False(t, sent.Bootstrap.State.PublishGate.CanPublishHostedSoul)
	require.Equal(t, "blocked:conversation_in_progress", sent.Bootstrap.State.PublishGate.Reason)

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
	require.False(t, published.Bootstrap.State.PublishGate.CanPublishHostedSoul)
	require.Equal(t, 0, publishCalls)
	require.Equal(t, 0, bindCalls)
}

func TestProject51HostedFailureRecoveryActionsProjectServerAuthoredNextOperation(t *testing.T) {
	now := time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC)
	agent := &storage.User{Username: "drone-p51"}

	tests := []struct {
		name         string
		hostAction   string
		retryable    bool
		wantCategory model.SoulBootstrapRecoveryCategory
		wantAction   model.SoulBootstrapRecoveryAction
		wantNext     model.SoulBootstrapNextAction
		wantRestart  bool
	}{
		{
			name:         "refresh_state",
			hostAction:   "refresh_state",
			retryable:    false,
			wantCategory: model.SoulBootstrapRecoveryCategoryRefreshState,
			wantAction:   model.SoulBootstrapRecoveryActionRefreshState,
			wantNext:     model.SoulBootstrapNextActionRefreshState,
		},
		{
			name:         "retry_same_step",
			hostAction:   "retry_same_step",
			retryable:    true,
			wantCategory: model.SoulBootstrapRecoveryCategoryRetrySameStep,
			wantAction:   model.SoulBootstrapRecoveryActionRetrySameStep,
			wantNext:     model.SoulBootstrapNextActionRetrySameStep,
		},
		{
			name:         "restart_soul_bootstrap",
			hostAction:   "restart_soul_bootstrap",
			retryable:    false,
			wantCategory: model.SoulBootstrapRecoveryCategoryRestartRequired,
			wantAction:   model.SoulBootstrapRecoveryActionRestartBootstrap,
			wantNext:     model.SoulBootstrapNextActionRestartSoulBootstrap,
			wantRestart:  true,
		},
		{
			name:         "operator_action",
			hostAction:   "operator_action",
			retryable:    false,
			wantCategory: model.SoulBootstrapRecoveryCategoryOperatorActionRequired,
			wantAction:   model.SoulBootstrapRecoveryActionContactOperator,
			wantNext:     model.SoulBootstrapNextActionOperatorActionRequired,
		},
		{
			name:         "unknown_action_fails_closed_to_operator",
			hostAction:   "resume_microvm",
			retryable:    true,
			wantCategory: model.SoulBootstrapRecoveryCategoryOperatorActionRequired,
			wantAction:   model.SoulBootstrapRecoveryActionContactOperator,
			wantNext:     model.SoulBootstrapNextActionOperatorActionRequired,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			state := soulBootstrapStateAfterHostedConversationSnapshot(agent, nil, nil, &soulservice.BootstrapConversationCompleteResult{
				RegistrationID:        "reg_p51",
				HostSoulAgentID:       "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				ConversationID:        "conv_p51",
				Status:                workflow.SoulBootstrapHostConversationStatusFailed,
				MessageCount:          1,
				HostRequestID:         "host-req-" + tt.name,
				FailureCode:           "llm_unavailable",
				FailureMessage:        "Host authored " + tt.name,
				FailureRetryable:      tt.retryable,
				FailureRecoveryAction: tt.hostAction,
			}, now)
			projected := graphSoulBootstrapStoredStateModel(state)
			require.NotNil(t, projected)
			require.Equal(t, model.SoulBootstrapPhaseError, projected.Phase)
			require.Equal(t, workflow.SoulBootstrapStateHostFailed, projected.State)
			require.Equal(t, workflow.SoulBootstrapHostConversationStatusFailed, derefString(projected.HostConversationStatus))
			require.Equal(t, tt.wantNext, projected.TypedNextAction)
			require.Equal(t, tt.wantCategory, *projected.RecoveryCategory)
			require.Equal(t, tt.wantAction, *projected.RecoveryAction)
			require.Equal(t, tt.retryable, projected.Retryable)
			require.Equal(t, tt.wantRestart, projected.RestartRequired)
			require.Equal(t, "host-req-"+tt.name, derefString(projected.LastHostRequestID))
			require.NotNil(t, projected.Error)
			require.Equal(t, string(tt.wantCategory), string(*projected.Error.RecoveryCategory))
			require.Equal(t, string(tt.wantAction), string(*projected.Error.RecoveryAction))
			require.False(t, projected.PublishGate.CanPublishHostedSoul)
			require.Equal(t, "blocked:host_failure", projected.PublishGate.Reason)
		})
	}
}

func TestProject49GraphQLProjectionCoversLockedStatusTableRows(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	validDeclaration := `{"selfDescription":{"summary":"Hosted soul declaration for drone-ada"},"capabilities":[],"boundaries":[],"transparency":{}}`
	publication := &workflow.SoulBootstrapPublicationEvidence{
		AgentID:          "hsoul_example_001",
		PublishedVersion: 1,
		AuthorityModel:   workflow.SoulBootstrapAuthorityModelInstanceTrust,
		AnchorState:      workflow.SoulBootstrapAnchorStateHostedOffchain,
		PublishedAt:      &now,
	}
	terminalCheckpoint := workflow.SoulBootstrapSigningCheckpoint{
		Name:          soulBootstrapCheckpointHostedConversation,
		Status:        workflow.SoulBootstrapHostConversationStatusDeclarationReady,
		CanonicalJSON: hostedTerminalEvidenceCanonicalJSON("hconv_example_001", workflow.SoulBootstrapHostConversationStatusDeclarationReady, validDeclaration),
		HostRequestID: "host-req-complete-002",
		CompletedAt:   &now,
	}

	tests := []struct {
		name          string
		state         *workflow.SoulBootstrapState
		wantPhase     model.SoulBootstrapPhase
		wantState     string
		wantAction    model.SoulBootstrapNextAction
		wantHost      string
		wantGate      string
		wantCan       bool
		wantEvidence  bool
		wantPublished bool
	}{
		{
			name:       "no registration",
			wantPhase:  model.SoulBootstrapPhaseNotStarted,
			wantState:  workflow.SoulBootstrapStateNotStarted,
			wantAction: model.SoulBootstrapNextActionStartHostedBootstrap,
			wantGate:   "blocked:no_host_registration",
		},
		{
			name: "registration active no conversation",
			state: &workflow.SoulBootstrapState{
				Username:           "drone-ada",
				BodyID:             "drone-ada",
				HostRegistrationID: "hreg_example_001",
				HostSoulAgentID:    "hsoul_example_001",
				BootstrapMode:      workflow.SoulBootstrapModeHosted,
				AuthorityModel:     workflow.SoulBootstrapAuthorityModelInstanceTrust,
				AnchorState:        workflow.SoulBootstrapAnchorStateHostedOffchain,
				Phase:              workflow.SoulBootstrapPhaseConversation,
				State:              workflow.SoulBootstrapStateConversationRegistrationActive,
				NextAction:         workflow.SoulBootstrapNextActionSendHostedGenesisMessage,
			},
			wantPhase:  model.SoulBootstrapPhaseConversation,
			wantState:  workflow.SoulBootstrapStateConversationRegistrationActive,
			wantAction: model.SoulBootstrapNextActionSendHostedSoulGenesisMessage,
			wantHost:   "registration_active",
			wantGate:   "blocked:no_host_conversation",
		},
		{
			name: "in_progress",
			state: &workflow.SoulBootstrapState{
				Username:           "drone-ada",
				BodyID:             "drone-ada",
				HostRegistrationID: "hreg_example_001",
				HostSoulAgentID:    "hsoul_example_001",
				HostConversationID: "hconv_example_001",
				BootstrapMode:      workflow.SoulBootstrapModeHosted,
				AuthorityModel:     workflow.SoulBootstrapAuthorityModelInstanceTrust,
				AnchorState:        workflow.SoulBootstrapAnchorStateHostedOffchain,
				Phase:              workflow.SoulBootstrapPhaseConversation,
				State:              workflow.SoulBootstrapStateConversationInProgress,
				NextAction:         workflow.SoulBootstrapNextActionRefreshState,
				RecoveryCategory:   workflow.SoulBootstrapRecoveryCategoryRefreshState,
				RecoveryAction:     workflow.SoulBootstrapRecoveryActionRefreshState,
			},
			wantPhase:  model.SoulBootstrapPhaseConversation,
			wantState:  workflow.SoulBootstrapStateConversationInProgress,
			wantAction: model.SoulBootstrapNextActionRefreshState,
			wantHost:   workflow.SoulBootstrapHostConversationStatusInProgress,
			wantGate:   "blocked:conversation_in_progress",
		},
		{
			name: "assistant_turn_ready",
			state: &workflow.SoulBootstrapState{
				Username:           "drone-ada",
				BodyID:             "drone-ada",
				HostRegistrationID: "hreg_example_001",
				HostSoulAgentID:    "hsoul_example_001",
				HostConversationID: "hconv_example_001",
				BootstrapMode:      workflow.SoulBootstrapModeHosted,
				AuthorityModel:     workflow.SoulBootstrapAuthorityModelInstanceTrust,
				AnchorState:        workflow.SoulBootstrapAnchorStateHostedOffchain,
				Phase:              workflow.SoulBootstrapPhaseConversation,
				State:              workflow.SoulBootstrapStateConversationAssistantTurnReady,
				NextAction:         workflow.SoulBootstrapNextActionCompleteHostedGenesis,
			},
			wantPhase:  model.SoulBootstrapPhaseConversation,
			wantState:  workflow.SoulBootstrapStateConversationAssistantTurnReady,
			wantAction: model.SoulBootstrapNextActionCompleteHostedSoulGenesis,
			wantHost:   workflow.SoulBootstrapHostConversationStatusAssistantTurnReady,
			wantGate:   "blocked:terminal_declaration_evidence_absent",
		},
		{
			name: "declaration_extraction_pending",
			state: &workflow.SoulBootstrapState{
				Username:           "drone-ada",
				BodyID:             "drone-ada",
				HostRegistrationID: "hreg_example_001",
				HostSoulAgentID:    "hsoul_example_001",
				HostConversationID: "hconv_example_001",
				BootstrapMode:      workflow.SoulBootstrapModeHosted,
				AuthorityModel:     workflow.SoulBootstrapAuthorityModelInstanceTrust,
				AnchorState:        workflow.SoulBootstrapAnchorStateHostedOffchain,
				Phase:              workflow.SoulBootstrapPhaseConversation,
				State:              workflow.SoulBootstrapStateConversationDeclarationExtractionPending,
				NextAction:         workflow.SoulBootstrapNextActionRefreshState,
				RecoveryCategory:   workflow.SoulBootstrapRecoveryCategoryRefreshState,
				RecoveryAction:     workflow.SoulBootstrapRecoveryActionRefreshState,
			},
			wantPhase:  model.SoulBootstrapPhaseConversation,
			wantState:  workflow.SoulBootstrapStateConversationDeclarationExtractionPending,
			wantAction: model.SoulBootstrapNextActionRefreshState,
			wantHost:   workflow.SoulBootstrapHostConversationStatusDeclarationExtractionPending,
			wantGate:   "blocked:declaration_extraction_pending",
		},
		{
			name: "declaration_ready",
			state: &workflow.SoulBootstrapState{
				Username:           "drone-ada",
				BodyID:             "drone-ada",
				HostRegistrationID: "hreg_example_001",
				HostSoulAgentID:    "hsoul_example_001",
				HostConversationID: "hconv_example_001",
				BootstrapMode:      workflow.SoulBootstrapModeHosted,
				AuthorityModel:     workflow.SoulBootstrapAuthorityModelInstanceTrust,
				AnchorState:        workflow.SoulBootstrapAnchorStateHostedOffchain,
				Phase:              workflow.SoulBootstrapPhaseFinalize,
				State:              workflow.SoulBootstrapStateConversationDeclarationReady,
				NextAction:         workflow.SoulBootstrapNextActionPublishHostedSoul,
				SigningCheckpoints: []workflow.SoulBootstrapSigningCheckpoint{terminalCheckpoint},
			},
			wantPhase:    model.SoulBootstrapPhaseFinalize,
			wantState:    workflow.SoulBootstrapStateConversationDeclarationReady,
			wantAction:   model.SoulBootstrapNextActionPublishHostedSoul,
			wantHost:     workflow.SoulBootstrapHostConversationStatusDeclarationReady,
			wantGate:     "allowed:active_conversation_terminal_declaration_evidence",
			wantCan:      true,
			wantEvidence: true,
		},
		{
			name: "failed",
			state: &workflow.SoulBootstrapState{
				Username:           "drone-ada",
				BodyID:             "drone-ada",
				HostRegistrationID: "hreg_example_001",
				HostSoulAgentID:    "hsoul_example_001",
				HostConversationID: "hconv_example_001",
				BootstrapMode:      workflow.SoulBootstrapModeHosted,
				AuthorityModel:     workflow.SoulBootstrapAuthorityModelInstanceTrust,
				AnchorState:        workflow.SoulBootstrapAnchorStateHostedOffchain,
				Phase:              workflow.SoulBootstrapPhaseError,
				State:              workflow.SoulBootstrapStateHostFailed,
				NextAction:         workflow.SoulBootstrapNextActionRetrySameStep,
				RecoveryCategory:   workflow.SoulBootstrapRecoveryCategoryRetrySameStep,
				RecoveryAction:     workflow.SoulBootstrapRecoveryActionRetrySameStep,
			},
			wantPhase:  model.SoulBootstrapPhaseError,
			wantState:  workflow.SoulBootstrapStateHostFailed,
			wantAction: model.SoulBootstrapNextActionRetrySameStep,
			wantHost:   workflow.SoulBootstrapHostConversationStatusFailed,
			wantGate:   "blocked:host_failure",
		},
		{
			name: "published bound",
			state: &workflow.SoulBootstrapState{
				Username:           "drone-ada",
				BodyID:             "drone-ada",
				HostRegistrationID: "hreg_example_001",
				HostSoulAgentID:    "hsoul_example_001",
				HostConversationID: "hconv_example_001",
				BootstrapMode:      workflow.SoulBootstrapModeHosted,
				AuthorityModel:     workflow.SoulBootstrapAuthorityModelInstanceTrust,
				AnchorState:        workflow.SoulBootstrapAnchorStateHostedOffchain,
				Phase:              workflow.SoulBootstrapPhaseComplete,
				State:              workflow.SoulBootstrapStateCompleteBound,
				NextAction:         workflow.SoulBootstrapNextActionComplete,
				SigningCheckpoints: []workflow.SoulBootstrapSigningCheckpoint{terminalCheckpoint},
				Publication:        publication,
			},
			wantPhase:     model.SoulBootstrapPhaseComplete,
			wantState:     workflow.SoulBootstrapStateCompleteBound,
			wantAction:    model.SoulBootstrapNextActionComplete,
			wantHost:      workflow.SoulBootstrapHostConversationStatusBound,
			wantGate:      "complete:already_published_bound",
			wantEvidence:  true,
			wantPublished: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			projected := graphSoulBootstrapStoredStateModel(tt.state)
			require.NotNil(t, projected)
			require.Equal(t, tt.wantPhase, projected.Phase)
			require.Equal(t, tt.wantState, projected.State)
			require.Equal(t, tt.wantAction, projected.TypedNextAction)
			require.NotNil(t, projected.PublishGate)
			require.Equal(t, tt.wantCan, projected.PublishGate.CanPublishHostedSoul)
			require.Equal(t, tt.wantGate, projected.PublishGate.Reason)
			require.True(t, projected.PublishGate.RequiresActiveConversationTerminalDeclarationEvidence)
			require.Equal(t, tt.wantHost, derefString(projected.HostConversationStatus))
			require.Equal(t, tt.wantEvidence, projected.TerminalDeclarationEvidence != nil)
			if tt.wantEvidence {
				require.Equal(t, "hconv_example_001", projected.TerminalDeclarationEvidence.ConversationID)
				require.Equal(t, workflow.SoulBootstrapHostConversationStatusDeclarationReady, projected.TerminalDeclarationEvidence.HostStatus)
				require.NotEmpty(t, derefString(projected.TerminalDeclarationEvidence.DeclarationsHash))
				require.NotNil(t, projected.TerminalDeclarationEvidence.ProducedDeclarationsPreview)
			}
			require.Equal(t, tt.wantPublished, projected.PublicationEvidence != nil)
		})
	}
}

func TestProject49PublishHostedSoulFailsClosedWithoutActiveTerminalEvidence(t *testing.T) {
	now := time.Date(2026, 6, 21, 13, 0, 0, 0, time.UTC)
	validDeclaration := `{"selfDescription":{"summary":"ready"},"capabilities":[],"boundaries":[],"transparency":{}}`
	validEvidence := hostedTerminalEvidenceCanonicalJSON("hconv_active", workflow.SoulBootstrapHostConversationStatusDeclarationReady, validDeclaration)
	staleEvidence := hostedTerminalEvidenceCanonicalJSON("hconv_stale", workflow.SoulBootstrapHostConversationStatusDeclarationReady, validDeclaration)

	tests := []struct {
		name                string
		state               workflow.SoulBootstrapState
		inputConversationID string
		wantGateReason      string
	}{
		{
			name: "in_progress",
			state: workflow.SoulBootstrapState{
				HostRegistrationID: "hreg_active",
				HostConversationID: "hconv_active",
				Phase:              workflow.SoulBootstrapPhaseConversation,
				State:              workflow.SoulBootstrapStateConversationInProgress,
				NextAction:         workflow.SoulBootstrapNextActionRefreshState,
			},
			inputConversationID: "hconv_active",
			wantGateReason:      "blocked:conversation_in_progress",
		},
		{
			name: "failed",
			state: workflow.SoulBootstrapState{
				HostRegistrationID: "hreg_active",
				HostConversationID: "hconv_active",
				Phase:              workflow.SoulBootstrapPhaseError,
				State:              workflow.SoulBootstrapStateHostFailed,
				NextAction:         workflow.SoulBootstrapNextActionRetrySameStep,
			},
			inputConversationID: "hconv_active",
			wantGateReason:      "blocked:host_failure",
		},
		{
			name: "missing declarations",
			state: workflow.SoulBootstrapState{
				HostRegistrationID: "hreg_active",
				HostConversationID: "hconv_active",
				Phase:              workflow.SoulBootstrapPhaseFinalize,
				State:              workflow.SoulBootstrapStateConversationDeclarationReady,
				NextAction:         workflow.SoulBootstrapNextActionPublishHostedSoul,
			},
			inputConversationID: "hconv_active",
			wantGateReason:      "blocked:terminal_declaration_evidence_absent",
		},
		{
			name: "mismatched conversation id",
			state: workflow.SoulBootstrapState{
				HostRegistrationID: "hreg_active",
				HostConversationID: "hconv_active",
				Phase:              workflow.SoulBootstrapPhaseFinalize,
				State:              workflow.SoulBootstrapStateConversationDeclarationReady,
				NextAction:         workflow.SoulBootstrapNextActionPublishHostedSoul,
				SigningCheckpoints: []workflow.SoulBootstrapSigningCheckpoint{{
					Name:          soulBootstrapCheckpointHostedConversation,
					Status:        workflow.SoulBootstrapHostConversationStatusDeclarationReady,
					CanonicalJSON: validEvidence,
					HostRequestID: "host-req-terminal",
				}},
			},
			inputConversationID: "hconv_other",
			wantGateReason:      "allowed:active_conversation_terminal_declaration_evidence",
		},
		{
			name: "stale evidence conversation id",
			state: workflow.SoulBootstrapState{
				HostRegistrationID: "hreg_active",
				HostConversationID: "hconv_active",
				Phase:              workflow.SoulBootstrapPhaseFinalize,
				State:              workflow.SoulBootstrapStateConversationDeclarationReady,
				NextAction:         workflow.SoulBootstrapNextActionPublishHostedSoul,
				SigningCheckpoints: []workflow.SoulBootstrapSigningCheckpoint{{
					Name:          soulBootstrapCheckpointHostedConversation,
					Status:        workflow.SoulBootstrapHostConversationStatusDeclarationReady,
					CanonicalJSON: staleEvidence,
					HostRequestID: "host-req-stale",
				}},
			},
			inputConversationID: "hconv_active",
			wantGateReason:      "blocked:terminal_declaration_evidence_absent",
		},
		{
			name: "declaration only stale evidence",
			state: workflow.SoulBootstrapState{
				HostRegistrationID: "hreg_active",
				HostConversationID: "hconv_active",
				Phase:              workflow.SoulBootstrapPhaseFinalize,
				State:              workflow.SoulBootstrapStateConversationDeclarationReady,
				NextAction:         workflow.SoulBootstrapNextActionPublishHostedSoul,
				SigningCheckpoints: []workflow.SoulBootstrapSigningCheckpoint{{
					Name:          soulBootstrapCheckpointHostedConversation,
					Status:        workflow.SoulBootstrapHostConversationStatusDeclarationReady,
					CanonicalJSON: validDeclaration,
					HostRequestID: "host-req-legacy-raw",
				}},
			},
			inputConversationID: "hconv_active",
			wantGateReason:      "blocked:terminal_declaration_evidence_absent",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			resolver, storageRepo := newRound12GraphResolver(t)
			publishCalls := 0
			bindCalls := 0
			resolver.soulsClient = &stubSoulService{
				publishHostedBootstrapFunc: func(context.Context, soulservice.HostedBootstrapPublishInput) (*soulservice.BootstrapFinalizeResult, error) {
					publishCalls++
					t.Fatalf("PublishHostedBootstrap must not be called for %s", tt.name)
					return nil, nil
				},
				bindHostedBootstrapFunc: func(context.Context, string, *soulservice.BootstrapFinalizeResult) (*soulservice.Soul, error) {
					bindCalls++
					t.Fatalf("BindHostedBootstrap must not be called for %s", tt.name)
					return nil, nil
				},
			}

			username := "drone-p49-" + strings.ReplaceAll(tt.name, " ", "-")
			state := tt.state
			state.Username = username
			state.BodyID = username
			state.HostSoulAgentID = "hsoul_active"
			state.BootstrapMode = workflow.SoulBootstrapModeHosted
			state.AuthorityModel = workflow.SoulBootstrapAuthorityModelInstanceTrust
			state.AnchorState = workflow.SoulBootstrapAnchorStateHostedOffchain
			metadata, err := workflow.SetDroneWorkflowMetadata(nil, &workflow.DroneWorkflowState{SoulBootstrap: &state})
			require.NoError(t, err)

			round13SeedGraphUser(t, storageRepo, &storage.User{
				Username:    "owner",
				DisplayName: "Owner",
				Approved:    true,
				CreatedAt:   now.Add(-48 * time.Hour),
				UpdatedAt:   now.Add(-24 * time.Hour),
			}, nil)
			round13SeedGraphUser(t, storageRepo, &storage.User{
				Username:    username,
				DisplayName: "Drone P49",
				Approved:    true,
				IsAgent:     true,
				AgentOwner:  "@owner",
				Metadata:    metadata,
				CreatedAt:   now.Add(-24 * time.Hour),
				UpdatedAt:   now,
			}, &storage.AgentGovernanceState{
				Username:        username,
				DelegatedScopes: []string{auth.ScopeRead, auth.ScopeWrite},
			})

			surface, err := (&queryResolver{resolver}).SoulBootstrap(round13DroneAuthContext("owner", auth.ScopeRead), username)
			require.NoError(t, err)
			require.NotNil(t, surface.State.PublishGate)
			require.Equal(t, tt.wantGateReason, surface.State.PublishGate.Reason)

			payload, err := (&mutationResolver{resolver}).PublishHostedSoul(
				round13DroneAuthContext("owner", auth.ScopeWrite),
				model.PublishHostedSoulInput{
					Username:       username,
					ConversationID: tt.inputConversationID,
				},
			)
			require.NoError(t, err)
			require.NotNil(t, payload.Error)
			require.Equal(t, workflow.SoulBootstrapErrorHostBootstrapReplayRejected, payload.Error.Code)
			require.Equal(t, 0, publishCalls)
			require.Equal(t, 0, bindCalls)
		})
	}
}
