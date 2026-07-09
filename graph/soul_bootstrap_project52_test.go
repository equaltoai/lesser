package graph

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	workflow "github.com/equaltoai/lesser/pkg/agents"
	"github.com/equaltoai/lesser/pkg/auth"
	soulservice "github.com/equaltoai/lesser/pkg/services/souls"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// TestP52_DeclarationExtractionPendingRoutesToPollNotSend proves L3.2 G14:
// when Host status is declaration_extraction_pending, the state machine routes
// the client to poll (REFRESH_STATE), never to send another message. This
// realigns the resolver with the locked projection table. Covers both the
// authored typedNextAction and availableActions.
func TestP52_DeclarationExtractionPendingRoutesToPollNotSend(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 3, 11, 0, 0, 0, time.UTC)
	agent := &storage.User{Username: "drone-p52-extraction"}
	result := &soulservice.BootstrapConversationCompleteResult{
		RegistrationID:  "reg_p52_extr",
		HostSoulAgentID: "0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
		ConversationID:  "conv_p52_extr",
		Status:          workflow.SoulBootstrapHostConversationStatusDeclarationExtractionPending,
		HostRequestID:   "host-req-p52-extr",
	}

	stored := soulBootstrapStateAfterHostedConversationSnapshot(agent, nil, nil, result, now)
	projected := graphSoulBootstrapStoredStateModel(stored)

	require.Equal(t, workflow.SoulBootstrapStateConversationDeclarationExtractionPending, projected.State)
	require.Equal(t, model.SoulBootstrapNextActionRefreshState, projected.TypedNextAction,
		"declaration_extraction_pending must route to REFRESH_STATE (poll), not SEND")
	require.NotEqual(t, model.SoulBootstrapNextActionSendHostedSoulGenesisMessage, projected.TypedNextAction)
	require.NotContains(t, projected.AvailableActions, model.SoulBootstrapNextActionSendHostedSoulGenesisMessage,
		"declaration_extraction_pending must not advertise SEND at all")
	require.Contains(t, projected.AvailableActions, model.SoulBootstrapNextActionRefreshState)
}

// TestP52_InProgressRoutesToPollPrimary proves L3.2 G14: in_progress is a
// pending turn. Poll (REFRESH_STATE) is the authored typedNextAction; SEND
// remains an allowed alternative per the projection table.
func TestP52_InProgressRoutesToPollPrimary(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 3, 11, 0, 0, 0, time.UTC)
	agent := &storage.User{Username: "drone-p52-inprogress"}
	result := &soulservice.BootstrapConversationCompleteResult{
		RegistrationID:  "reg_p52_inp",
		HostSoulAgentID: "0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
		ConversationID:  "conv_p52_inp",
		Status:          workflow.SoulBootstrapHostConversationStatusInProgress,
		HostRequestID:   "host-req-p52-inp",
	}

	stored := soulBootstrapStateAfterHostedConversationSnapshot(agent, nil, nil, result, now)
	projected := graphSoulBootstrapStoredStateModel(stored)

	require.Equal(t, workflow.SoulBootstrapStateConversationInProgress, projected.State)
	require.Equal(t, model.SoulBootstrapNextActionRefreshState, projected.TypedNextAction,
		"in_progress must author REFRESH_STATE (poll) as the primary next action")
	require.Contains(t, projected.AvailableActions, model.SoulBootstrapNextActionSendHostedSoulGenesisMessage,
		"in_progress may still advertise SEND as an alternative")
	require.Contains(t, projected.AvailableActions, model.SoulBootstrapNextActionRefreshState)
}

// TestP52_HostUnavailableTimeoutRoutesToRefreshNotRetry proves L3.2 G15: a
// HOST_UNAVAILABLE error (the accept-timeout mapping from L3.1) must NOT map
// to RETRY_SAME_STEP. RETRY_SAME_STEP would instruct the client to re-issue the
// same blocking send — the binding constraint this project removes. It must
// route to REFRESH_STATE so the client polls and reconcile recovers.
func TestP52_HostUnavailableTimeoutRoutesToRefreshNotRetry(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 3, 11, 0, 0, 0, time.UTC)
	agent := &storage.User{Username: "drone-p52-timeout"}
	hostErr := &soulservice.HostBootstrapError{
		Code:    workflow.SoulBootstrapErrorHostUnavailable,
		Message: "Host bootstrap endpoint is unavailable.",
		Source:  "host",
		Err:     errors.New("context deadline exceeded (client.Timeout exceeded while awaiting headers)"),
	}

	state := soulBootstrapErrorState(agent, nil, nil, hostErr, now)

	require.Equal(t, workflow.SoulBootstrapPhaseError, state.Phase)
	require.Equal(t, workflow.SoulBootstrapStateHostUnavailable, state.State)
	// G15: the killer assertion — timeout must not instruct a blocking retry.
	require.Equal(t, workflow.SoulBootstrapNextActionRefreshState, state.NextAction,
		"HOST_UNAVAILABLE (accept timeout) must route to REFRESH_STATE (poll), not RETRY_SAME_STEP")
	require.NotEqual(t, workflow.SoulBootstrapNextActionRetrySameStep, state.NextAction)
	require.Equal(t, workflow.SoulBootstrapRecoveryCategoryRefreshState, state.RecoveryCategory)
	require.Equal(t, workflow.SoulBootstrapRecoveryActionRefreshState, state.RecoveryAction)
	require.False(t, state.Retryable, "accept timeout must not be flagged as a blocking retryable re-issue")
	require.False(t, state.RestartRequired)
	// And the retry-repair gate must not fire for a timeout (only genuine
	// Host-authored retry_same_step qualifies), closing the G15 loop.
	require.False(t, soulBootstrapHostedGenesisMessageRetryRequired(state),
		"accept-timeout state must not qualify for the retry_same_step send repair")
}

// TestP52_HostAuthoredFailedStillRoutesToRetry proves the G15 change is scoped
// to HOST_UNAVAILABLE only: a genuine Host-authored `failed` conversation with
// a `retry_same_step` recovery action still routes to RETRY_SAME_STEP (the
// failed row of the projection table). G15 does not regress the failed path.
func TestP52_HostAuthoredFailedStillRoutesToRetry(t *testing.T) {
	t.Parallel()

	plan := soulBootstrapRecoveryForError(workflow.SoulBootstrapErrorHostConversationFailed, 0, "")
	require.Equal(t, workflow.SoulBootstrapRecoveryCategoryRetrySameStep, plan.Category)
	require.Equal(t, workflow.SoulBootstrapNextActionRetrySameStep, plan.NextAction)
}

// TestP52_EmptyHostStatusDoesNotSilentlyCoerceToInProgress proves L3.2 G16: an
// empty/unknown Host status is no longer silently coerced to in_progress. When
// a prior state exists, its status is preserved instead of inventing progress.
func TestP52_EmptyHostStatusDoesNotSilentlyCoerceToInProgress(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 3, 11, 0, 0, 0, time.UTC)
	agent := &storage.User{Username: "drone-p52-empty"}
	// Existing state is assistant_turn_ready — a non-progress step.
	existing := &workflow.SoulBootstrapState{
		Username:           "drone-p52-empty",
		BodyID:             "drone-p52-empty",
		HostRegistrationID: "reg_p52_empty",
		HostConversationID: "conv_p52_empty",
		BootstrapMode:      workflow.SoulBootstrapModeHosted,
		Phase:              workflow.SoulBootstrapPhaseConversation,
		State:              workflow.SoulBootstrapStateConversationAssistantTurnReady,
		NextAction:         workflow.SoulBootstrapNextActionCompleteHostedGenesis,
	}
	// A result with an unrecognized status that would previously have been
	// silently coerced to in_progress (progress), masking the gap.
	result := &soulservice.BootstrapConversationCompleteResult{
		RegistrationID: "reg_p52_empty",
		ConversationID: "conv_p52_empty",
		Status:         "totally_unknown_status",
	}

	stored := soulBootstrapStateAfterHostedConversationSnapshot(agent, existing, nil, result, now)
	// The prior assistant_turn_ready status is preserved rather than silently
	// rewritten to in_progress.
	require.Equal(t, workflow.SoulBootstrapStateConversationAssistantTurnReady, stored.State,
		"empty/unknown Host status must preserve the existing state, not silently coerce to in_progress")
}

// TestP52_EmptyHostStatusNoPriorStateDefaultsToInProgress proves the honest
// fallback: a brand-new conversation with no prior status and an empty Host
// status defaults to in_progress (the only defensible default for a new
// conversation), rather than inventing a more specific state.
func TestP52_EmptyHostStatusNoPriorStateDefaultsToInProgress(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 3, 11, 0, 0, 0, time.UTC)
	agent := &storage.User{Username: "drone-p52-new"}
	result := &soulservice.BootstrapConversationCompleteResult{
		RegistrationID: "reg_p52_new",
		ConversationID: "conv_p52_new",
		Status:         "",
	}

	stored := soulBootstrapStateAfterHostedConversationSnapshot(agent, nil, nil, result, now)
	require.Equal(t, workflow.SoulBootstrapStateConversationInProgress, stored.State,
		"empty Host status with no prior state defaults honestly to in_progress for a new conversation")
}

// TestP52_ReconcileLogsHostReadErrorNotSilent proves L3.2 G16: reconcile no
// longer silently swallows Host read errors. The error is surfaced via the
// resolver logger (observed here) while reconcile stays best-effort (the
// SoulBootstrap query still succeeds). Exercised through the real query entry
// point that invokes reconcileHostedSoulBootstrapState.
func TestP52_ReconcileLogsHostReadErrorNotSilent(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 3, 11, 0, 0, 0, time.UTC)
	core, recorded := observer.New(zap.WarnLevel)
	resolver, storageRepo := newRound12GraphResolver(t)
	resolver.Logger = zap.New(core)

	const (
		registrationID = "reg_p52_rec"
		conversationID = "conv_p52_rec"
		agentID        = "0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	)

	metadata, err := workflow.SetDroneWorkflowMetadata(nil, &workflow.DroneWorkflowState{
		SoulBootstrap: &workflow.SoulBootstrapState{
			Username:           "drone-p52-rec",
			BodyID:             "drone-p52-rec",
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
		Username:    "drone-p52-rec",
		DisplayName: "Drone P52 Rec",
		Approved:    true,
		IsAgent:     true,
		AgentOwner:  "@owner",
		Metadata:    metadata,
		CreatedAt:   now.Add(-24 * time.Hour),
		UpdatedAt:   now,
	}, &storage.AgentGovernanceState{
		Username:        "drone-p52-rec",
		DelegatedScopes: []string{auth.ScopeRead, auth.ScopeWrite},
	})

	resolver.soulsClient = &stubSoulService{
		readBootstrapConversationFunc: func(_ context.Context, _ soulservice.BootstrapConversationCompleteInput) (*soulservice.BootstrapConversationCompleteResult, error) {
			return nil, &soulservice.HostBootstrapError{Code: workflow.SoulBootstrapErrorHostUnavailable, Message: "transient host read failure", Source: "host"}
		},
	}

	// The SoulBootstrap query invokes reconcile. Reconcile read-repairs the
	// in_progress hosted state by calling Host's read endpoint, which here
	// fails. The query must still succeed (best-effort reconcile) AND the
	// previously-swallowed error must now be logged.
	surface, err := (&queryResolver{resolver}).SoulBootstrap(round13DroneAuthContext("owner", auth.ScopeRead), "drone-p52-rec")
	require.NoError(t, err, "reconcile must stay best-effort (non-fatal) on Host read error")
	require.NotNil(t, surface)

	found := false
	for _, e := range recorded.All() {
		if e.Message == "hosted soul bootstrap reconcile Host read failed" {
			found = true
			break
		}
	}
	require.True(t, found, "reconcile must log the Host read error instead of silently swallowing it")
}

// TestP52_ReconcileNilLoggerDoesNotPanic proves the G16 logging helper is
// nil-safe so best-effort reconcile paths never panic when no logger is wired.
func TestP52_ReconcileNilLoggerDoesNotPanic(t *testing.T) {
	t.Parallel()

	require.NotPanics(t, func() {
		soulBootstrapReconcileLog(nil, "nil resolver must not panic", errors.New("x"))
	})
	require.NotPanics(t, func() {
		soulBootstrapReconcileLog(&Resolver{}, "nil logger must not panic", errors.New("x"))
	})
}
