package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	workflow "github.com/equaltoai/lesser/pkg/agents"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	soulservice "github.com/equaltoai/lesser/pkg/services/souls"
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

func TestRound44SoulBootstrapRequiresAuthWriteScopeAndBodyAuthorization(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	now := time.Date(2026, 6, 12, 12, 30, 0, 0, time.UTC)
	resolver.soulsClient = &stubSoulService{
		beginBootstrapFunc: func(context.Context, soulservice.BootstrapBeginInput) (*soulservice.BootstrapBeginResult, error) {
			t.Fatal("host bootstrap should not be called when Lesser auth or body authorization fails")
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
		Username:    "other-owner",
		DisplayName: "Other Owner",
		Approved:    true,
		CreatedAt:   now.Add(-48 * time.Hour),
		UpdatedAt:   now.Add(-24 * time.Hour),
	}, nil)
	round13SeedGraphUser(t, storageRepo, &storage.User{
		Username:    "drone-auth",
		DisplayName: "Drone Auth",
		Approved:    true,
		IsAgent:     true,
		AgentOwner:  "@owner",
		CreatedAt:   now.Add(-24 * time.Hour),
		UpdatedAt:   now,
	}, &storage.AgentGovernanceState{
		Username:        "drone-auth",
		DelegatedScopes: []string{auth.ScopeRead, auth.ScopeWrite},
	})

	_, err := (&queryResolver{resolver}).SoulBootstrap(context.Background(), "drone-auth")
	require.Error(t, err)
	require.True(t, apperrors.HasCode(err, apperrors.CodeUnauthorized))

	_, err = (&queryResolver{resolver}).SoulBootstrap(round13DroneAuthContext("owner"), "drone-auth")
	require.Error(t, err)
	require.True(t, apperrors.HasCode(err, apperrors.CodeInsufficientScope))

	beginInput := model.BeginSoulBootstrapInput{
		Username:      "drone-auth",
		WalletAddress: "0x1111111111111111111111111111111111111111",
	}
	_, err = (&mutationResolver{resolver}).BeginSoulBootstrap(context.Background(), beginInput)
	require.Error(t, err)
	require.True(t, apperrors.HasCode(err, apperrors.CodeUnauthorized))

	_, err = (&mutationResolver{resolver}).BeginSoulBootstrap(round13DroneAuthContext("owner", auth.ScopeRead), beginInput)
	require.Error(t, err)
	require.True(t, apperrors.HasCode(err, apperrors.CodeInsufficientScope))

	_, err = (&mutationResolver{resolver}).BeginSoulBootstrap(round13DroneAuthContext("other-owner", auth.ScopeWrite), beginInput)
	require.Error(t, err)
	require.True(t, apperrors.HasCode(err, apperrors.CodeForbidden))
}

func TestRound44SoulBootstrapBeginPersistsHostState(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	now := time.Date(2026, 6, 12, 13, 0, 0, 0, time.UTC)
	hostIssuedAt := time.Date(2026, 6, 12, 13, 1, 0, 0, time.UTC)
	hostExpiresAt := hostIssuedAt.Add(10 * time.Minute)
	numericBodyID := common.GenerateNumericID("drone-beta")
	resolver.soulsClient = &stubSoulService{
		beginBootstrapFunc: func(_ context.Context, input soulservice.BootstrapBeginInput) (*soulservice.BootstrapBeginResult, error) {
			require.Equal(t, "drone-beta", input.Username)
			require.Equal(t, numericBodyID, input.BodyID)
			require.NotEqual(t, input.BodyID, input.Username)
			require.Equal(t, "0x1111111111111111111111111111111111111111", input.WalletAddress)
			return &soulservice.BootstrapBeginResult{
				RegistrationID:  "reg_123",
				HostSoulAgentID: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				WalletAddress:   input.WalletAddress,
				WalletChallenge: soulservice.BootstrapWalletChallenge{
					Address:   input.WalletAddress,
					Message:   "Sign this Host wallet challenge exactly.",
					IssuedAt:  &hostIssuedAt,
					ExpiresAt: &hostExpiresAt,
				},
				HostRequestID: "host-req-begin",
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
		Username:    "drone-beta",
		ID:          numericBodyID,
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
	require.True(t, payload.Executable)
	require.Nil(t, payload.Error)
	require.True(t, payload.Bootstrap.HostBridgeAvailable)
	require.Equal(t, model.SoulBootstrapPhaseBegin, payload.Bootstrap.State.Phase)
	require.Equal(t, "begin.ready", payload.Bootstrap.State.State)
	require.Equal(t, numericBodyID, payload.Bootstrap.State.BodyID)
	require.Equal(t, "reg_123", derefString(payload.Bootstrap.State.HostRegistrationID))
	require.Equal(t, "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", derefString(payload.Bootstrap.State.HostSoulAgentID))
	require.NotNil(t, payload.Bootstrap.State.Correlation)
	require.Equal(t, "corr-1", derefString(payload.Bootstrap.State.Correlation.CorrelationKey))
	require.Equal(t, "begin-1", derefString(payload.Bootstrap.State.Correlation.BeginIdempotencyKey))
	require.Equal(t, "host-req-begin", derefString(payload.Bootstrap.State.Correlation.LastHostRequestID))
	require.Len(t, payload.Bootstrap.State.SigningCheckpoints, 1)
	require.Equal(t, "wallet", payload.Bootstrap.State.SigningCheckpoints[0].Name)
	require.Equal(t, "ready", payload.Bootstrap.State.SigningCheckpoints[0].Status)
	require.Equal(t, "utf8", derefString(payload.Bootstrap.State.SigningCheckpoints[0].MessageEncoding))
	require.Equal(t, "Sign this Host wallet challenge exactly.", derefString(payload.Bootstrap.State.SigningCheckpoints[0].Message))

	storedUser, err := storageRepo.Account().GetUser(context.Background(), "drone-beta")
	require.NoError(t, err)
	storedWorkflow, err := workflow.ParseDroneWorkflowMetadata(storedUser.Metadata)
	require.NoError(t, err)
	require.NotNil(t, storedWorkflow)
	require.NotNil(t, storedWorkflow.SoulBootstrap)
	require.Equal(t, numericBodyID, storedWorkflow.SoulBootstrap.BodyID)
	require.Equal(t, "reg_123", storedWorkflow.SoulBootstrap.HostRegistrationID)

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

func TestRound44SoulBootstrapBeginReplayDoesNotDuplicateHostRegistration(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	now := time.Date(2026, 6, 12, 13, 15, 0, 0, time.UTC)
	const (
		wallet  = "0x1111111111111111111111111111111111111111"
		agentID = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)
	beginCalls := 0
	resolver.soulsClient = &stubSoulService{
		beginBootstrapFunc: func(_ context.Context, input soulservice.BootstrapBeginInput) (*soulservice.BootstrapBeginResult, error) {
			beginCalls++
			require.Equal(t, 1, beginCalls, "idempotent begin replay must not call Host again")
			require.Equal(t, "drone-replay", input.Username)
			return &soulservice.BootstrapBeginResult{
				RegistrationID:  "reg_replay",
				HostSoulAgentID: agentID,
				WalletAddress:   input.WalletAddress,
				WalletChallenge: soulservice.BootstrapWalletChallenge{
					Address: input.WalletAddress,
					Message: "Sign this stable Host wallet challenge.",
				},
				HostRequestID: "host-req-replay",
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
		Username:    "drone-replay",
		DisplayName: "Drone Replay",
		Approved:    true,
		IsAgent:     true,
		AgentOwner:  "@owner",
		CreatedAt:   now.Add(-24 * time.Hour),
		UpdatedAt:   now,
	}, &storage.AgentGovernanceState{
		Username:        "drone-replay",
		DelegatedScopes: []string{auth.ScopeRead, auth.ScopeWrite},
	})

	input := model.BeginSoulBootstrapInput{
		Username:       "drone-replay",
		WalletAddress:  wallet,
		IdempotencyKey: round13StringPtr("begin-stable-replay"),
		CorrelationKey: round13StringPtr("corr-stable"),
	}
	first, err := (&mutationResolver{resolver}).BeginSoulBootstrap(round13DroneAuthContext("owner", auth.ScopeWrite), input)
	require.NoError(t, err)
	require.Nil(t, first.Error)
	require.Equal(t, "reg_replay", derefString(first.Bootstrap.State.HostRegistrationID))
	require.Equal(t, "begin-stable-replay", derefString(first.Bootstrap.State.Correlation.BeginIdempotencyKey))

	second, err := (&mutationResolver{resolver}).BeginSoulBootstrap(round13DroneAuthContext("owner", auth.ScopeWrite), input)
	require.NoError(t, err)
	require.Nil(t, second.Error)
	require.Equal(t, 1, beginCalls)
	require.Equal(t, "reg_replay", derefString(second.Bootstrap.State.HostRegistrationID))
	require.Equal(t, agentID, derefString(second.Bootstrap.State.HostSoulAgentID))

	mismatch := input
	mismatch.WalletAddress = "0x2222222222222222222222222222222222222222"
	rejected, err := (&mutationResolver{resolver}).BeginSoulBootstrap(round13DroneAuthContext("owner", auth.ScopeWrite), mismatch)
	require.NoError(t, err)
	require.NotNil(t, rejected.Error)
	require.Equal(t, 1, beginCalls)
	require.Equal(t, workflow.SoulBootstrapErrorHostBootstrapReplayRejected, rejected.Error.Code)
	require.Equal(t, workflow.SoulBootstrapStateCorrelationMismatch, rejected.Bootstrap.State.State)
}

func TestRound44SoulBootstrapHostErrorsPersistTypedState(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	now := time.Date(2026, 6, 12, 13, 30, 0, 0, time.UTC)
	resolver.soulsClient = &stubSoulService{
		beginBootstrapFunc: func(context.Context, soulservice.BootstrapBeginInput) (*soulservice.BootstrapBeginResult, error) {
			return nil, &soulservice.HostBootstrapError{
				Code:    workflow.SoulBootstrapErrorHostTrustNotConfigured,
				Message: "Host instance trust base URL is not configured.",
				Source:  "lesser",
				Err:     soulservice.ErrHostTrustNotConfigured,
			}
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
		Username:    "drone-error",
		DisplayName: "Drone Error",
		Approved:    true,
		IsAgent:     true,
		AgentOwner:  "@owner",
		CreatedAt:   now.Add(-24 * time.Hour),
		UpdatedAt:   now,
	}, &storage.AgentGovernanceState{
		Username:        "drone-error",
		DelegatedScopes: []string{auth.ScopeRead, auth.ScopeWrite},
	})

	payload, err := (&mutationResolver{resolver}).BeginSoulBootstrap(
		round13DroneAuthContext("owner", auth.ScopeWrite),
		model.BeginSoulBootstrapInput{
			Username:      "drone-error",
			WalletAddress: "0x1111111111111111111111111111111111111111",
		},
	)
	require.NoError(t, err)
	require.NotNil(t, payload)
	require.False(t, payload.Executable)
	require.NotNil(t, payload.Error)
	require.Equal(t, workflow.SoulBootstrapErrorHostTrustNotConfigured, payload.Error.Code)
	require.Equal(t, model.SoulBootstrapPhaseError, payload.Bootstrap.State.Phase)
	require.Equal(t, workflow.SoulBootstrapStateHostTrustNotConfigured, payload.Bootstrap.State.State)

	storedUser, err := storageRepo.Account().GetUser(context.Background(), "drone-error")
	require.NoError(t, err)
	storedWorkflow, err := workflow.ParseDroneWorkflowMetadata(storedUser.Metadata)
	require.NoError(t, err)
	require.NotNil(t, storedWorkflow)
	require.NotNil(t, storedWorkflow.SoulBootstrap)
	require.Equal(t, workflow.SoulBootstrapErrorHostTrustNotConfigured, storedWorkflow.SoulBootstrap.Error.Code)
}

func TestRound44SoulBootstrapPrincipalPreflightAndVerifyPersistIdempotently(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	now := time.Date(2026, 6, 12, 14, 30, 0, 0, time.UTC)
	declaredAt := model.Time(time.Date(2026, 6, 12, 14, 35, 0, 0, time.UTC))
	preflightDeclaredAt := time.Time(declaredAt)
	resolver.soulsClient = &stubSoulService{
		prepareBootstrapPrincipalFunc: func(_ context.Context, input soulservice.BootstrapPrincipalPreflightInput) (*soulservice.BootstrapPrincipalPreflightResult, error) {
			require.Equal(t, "reg_123", input.RegistrationID)
			return &soulservice.BootstrapPrincipalPreflightResult{
				Version:          "1",
				PrincipalAddress: input.PrincipalAddress,
				SignerAddress:    input.PrincipalAddress,
				SigningMethod:    "eip191_personal_sign",
				MessageEncoding:  "hex_bytes",
				MessageHex:       "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				DigestHex:        "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				CanonicalJSON:    `{"host":"canonical"}`,
				DeclaredAt:       &preflightDeclaredAt,
				HostRequestID:    "host-req-preflight",
			}, nil
		},
		verifyBootstrapPrincipalFunc: func(_ context.Context, input soulservice.BootstrapPrincipalVerifyInput) (*soulservice.BootstrapPrincipalVerifyResult, error) {
			require.Equal(t, "reg_123", input.RegistrationID)
			require.Equal(t, "wallet-signature", input.WalletSignature)
			require.Equal(t, "principal-signature", input.PrincipalSignature)
			return &soulservice.BootstrapPrincipalVerifyResult{
				RegistrationID:   "reg_123",
				HostSoulAgentID:  "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				WalletAddress:    "0x1111111111111111111111111111111111111111",
				PrincipalAddress: input.PrincipalAddress,
				OperationID:      "op_123",
				PromotionStage:   "approved",
				HostRequestID:    "host-req-verify",
			}, nil
		},
	}

	seedMetadata, err := workflow.SetDroneWorkflowMetadata(nil, &workflow.DroneWorkflowState{
		SoulBootstrap: &workflow.SoulBootstrapState{
			Username:           "drone-delta",
			BodyID:             "drone-delta",
			HostRegistrationID: "reg_123",
			Phase:              workflow.SoulBootstrapPhaseBegin,
			State:              "begin.ready",
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
		Username:    "drone-delta",
		DisplayName: "Drone Delta",
		Approved:    true,
		IsAgent:     true,
		AgentOwner:  "@owner",
		Metadata:    seedMetadata,
		CreatedAt:   now.Add(-24 * time.Hour),
		UpdatedAt:   now,
	}, &storage.AgentGovernanceState{
		Username:        "drone-delta",
		DelegatedScopes: []string{auth.ScopeRead, auth.ScopeWrite},
	})

	preflight, err := (&mutationResolver{resolver}).PrepareSoulBootstrapPrincipalDeclaration(
		round13DroneAuthContext("owner", auth.ScopeWrite),
		model.PrepareSoulBootstrapPrincipalDeclarationInput{
			Username:             "drone-delta",
			PrincipalAddress:     "0x2222222222222222222222222222222222222222",
			PrincipalDeclaration: "I declare.",
			DeclaredAt:           declaredAt,
		},
	)
	require.NoError(t, err)
	require.Nil(t, preflight.Error)
	require.Equal(t, model.SoulBootstrapPhasePrincipalDeclaration, preflight.Bootstrap.State.Phase)
	require.Len(t, preflight.Bootstrap.State.SigningCheckpoints, 1)
	require.Equal(t, "principal_declaration", preflight.Bootstrap.State.SigningCheckpoints[0].Name)
	require.Equal(t, "1", derefString(preflight.Bootstrap.State.SigningCheckpoints[0].Version))
	require.Equal(t, "eip191_personal_sign", derefString(preflight.Bootstrap.State.SigningCheckpoints[0].SigningMethod))
	require.Equal(t, "hex_bytes", derefString(preflight.Bootstrap.State.SigningCheckpoints[0].MessageEncoding))
	require.Equal(t, "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", derefString(preflight.Bootstrap.State.SigningCheckpoints[0].MessageHex))
	require.Equal(t, "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", derefString(preflight.Bootstrap.State.SigningCheckpoints[0].DigestHex))
	require.Equal(t, `{"host":"canonical"}`, derefString(preflight.Bootstrap.State.SigningCheckpoints[0].CanonicalJSON))

	verified, err := (&mutationResolver{resolver}).VerifySoulBootstrapPrincipalDeclaration(
		round13DroneAuthContext("owner", auth.ScopeWrite),
		model.VerifySoulBootstrapPrincipalDeclarationInput{
			Username:             "drone-delta",
			Signature:            "wallet-signature",
			PrincipalAddress:     "0x2222222222222222222222222222222222222222",
			PrincipalDeclaration: "I declare.",
			PrincipalSignature:   "principal-signature",
			DeclaredAt:           declaredAt,
		},
	)
	require.NoError(t, err)
	require.Nil(t, verified.Error)
	require.Equal(t, model.SoulBootstrapPhaseConversation, verified.Bootstrap.State.Phase)
	require.Equal(t, "conversation.pending", verified.Bootstrap.State.State)
	require.Equal(t, "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", derefString(verified.Bootstrap.State.HostSoulAgentID))
	require.Len(t, verified.Bootstrap.State.SigningCheckpoints, 1)
	require.Equal(t, "verified", verified.Bootstrap.State.SigningCheckpoints[0].Status)
	require.Equal(t, "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", derefString(verified.Bootstrap.State.SigningCheckpoints[0].MessageHex))
}

func TestRound44SoulBootstrapConversationFinalizeBindsAndProjects(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	now := time.Date(2026, 6, 12, 15, 30, 0, 0, time.UTC)
	issuedAt := time.Date(2026, 6, 12, 15, 35, 0, 0, time.UTC)
	const (
		agentID   = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		principal = "0x2222222222222222222222222222222222222222"
	)

	resolver.soulsClient = &stubSoulService{
		sendBootstrapConversationFunc: func(_ context.Context, input soulservice.BootstrapConversationMessageInput) (*soulservice.BootstrapConversationMessageResult, error) {
			require.Equal(t, "reg_123", input.RegistrationID)
			require.Empty(t, input.ConversationID)
			require.Equal(t, "Review my hosted identity.", input.Message)
			return &soulservice.BootstrapConversationMessageResult{
				RegistrationID: "reg_123",
				ConversationID: "conv_123",
				Model:          "claude",
				FullResponse:   "Host response",
				HostRequestID:  "host-req-conv",
			}, nil
		},
		completeBootstrapConversationFunc: func(_ context.Context, input soulservice.BootstrapConversationCompleteInput) (*soulservice.BootstrapConversationCompleteResult, error) {
			require.Equal(t, "reg_123", input.RegistrationID)
			require.Equal(t, "conv_123", input.ConversationID)
			return &soulservice.BootstrapConversationCompleteResult{
				RegistrationID:       "reg_123",
				HostSoulAgentID:      agentID,
				ConversationID:       "conv_123",
				Status:               "completed",
				ProducedDeclarations: `{"selfDescription":{"summary":"ready"},"capabilities":[],"boundaries":[],"transparency":{}}`,
				CompletedAt:          &issuedAt,
				HostRequestID:        "host-req-complete",
			}, nil
		},
		prepareBootstrapFinalizeFunc: func(_ context.Context, input soulservice.BootstrapFinalizePreflightInput) (*soulservice.BootstrapFinalizePreflightResult, error) {
			require.Equal(t, map[string]string{"b1": "0xboundary"}, input.BoundarySignatures)
			return &soulservice.BootstrapFinalizePreflightResult{
				Version:         "1",
				DigestHex:       "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				IssuedAt:        &issuedAt,
				ExpectedVersion: 0,
				NextVersion:     1,
				SelfAttestationSigning: soulservice.BootstrapFinalizeSigningInput{
					SignerWallet:    principal,
					SigningMethod:   "eip191_personal_sign",
					MessageEncoding: "hex_bytes",
					MessageHex:      "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
					DigestHex:       "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
					CanonicalJSON:   `{"host":"finalize"}`,
				},
				BoundaryRequirementsJSON:    `[{"boundary_id":"b1"}]`,
				FinalizeRequestTemplateJSON: `{"expected_version":0}`,
				RegistrationPreviewJSON:     `{"agent_id":"` + agentID + `"}`,
				HostRequestID:               "host-req-preflight",
			}, nil
		},
		finalizeBootstrapFunc: func(_ context.Context, input soulservice.BootstrapFinalizeInput) (*soulservice.BootstrapFinalizeResult, error) {
			require.Equal(t, "reg_123", input.RegistrationID)
			require.Equal(t, "conv_123", input.ConversationID)
			require.Equal(t, map[string]string{"b1": "0xboundary"}, input.BoundarySignatures)
			require.Equal(t, issuedAt, input.IssuedAt)
			require.Equal(t, 0, input.ExpectedVersion)
			require.Equal(t, "0xself", input.SelfAttestation)
			publishedAt := issuedAt.Add(2 * time.Minute)
			return &soulservice.BootstrapFinalizeResult{
				Version:          "1",
				HostSoulAgentID:  agentID,
				PublishedVersion: 1,
				PrincipalAddress: principal,
				Publication: soulservice.BootstrapPublicationEvidence{
					AgentID:          agentID,
					PublishedVersion: 1,
					RegistrationURI:  "s3://bucket/registry/v1/agents/" + agentID + "/registration.json",
					AnchorState:      "hosted_offchain",
					PublishedAt:      &publishedAt,
				},
				Promotion: soulservice.BootstrapPromotionEvidence{
					AgentID:                  agentID,
					RegistrationID:           "reg_123",
					Stage:                    "graduated",
					LatestConversationID:     "conv_123",
					LatestConversationStatus: "completed",
					PublishedVersion:         1,
					GraduatedAt:              &publishedAt,
				},
				HostRequestID: "host-req-finalize",
			}, nil
		},
		incorporateFunc: func(_ context.Context, principalUsername string, targetAgentUsername string, soulAgentID string) (*soulservice.Soul, error) {
			require.Equal(t, "owner", principalUsername)
			require.Equal(t, "drone-epsilon", targetAgentUsername)
			require.Equal(t, agentID, soulAgentID)
			return &soulservice.Soul{
				AgentID:               soulAgentID,
				PrincipalAddress:      principal,
				Bound:                 true,
				BoundAgentUsername:    targetAgentUsername,
				BoundPrincipalAddress: principal,
			}, nil
		},
	}

	seedMetadata, err := workflow.SetDroneWorkflowMetadata(nil, &workflow.DroneWorkflowState{
		CurrentPhase: workflow.DroneWorkflowPhaseReview,
		CurrentState: workflow.DroneWorkflowStateReviewApproved,
		Review: &workflow.DroneReviewCard{
			ID:              "drone-epsilon:review",
			Title:           "Reviewer handoff",
			Decision:        workflow.DroneReviewDecisionApproved,
			Reviewer:        workflow.DroneActor{ID: "reviewer", Name: "Reviewer", Role: "reviewer", Handle: "@reviewer"},
			DecisionSummary: "Reviewer may conduct hosted conversation.",
		},
		SoulBootstrap: &workflow.SoulBootstrapState{
			Username:           "drone-epsilon",
			BodyID:             "drone-epsilon",
			HostRegistrationID: "reg_123",
			HostSoulAgentID:    agentID,
			Phase:              workflow.SoulBootstrapPhaseConversation,
			State:              "conversation.pending",
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
		Username:    "reviewer",
		DisplayName: "Reviewer",
		Approved:    true,
		CreatedAt:   now.Add(-48 * time.Hour),
		UpdatedAt:   now.Add(-24 * time.Hour),
	}, nil)
	round13SeedGraphUser(t, storageRepo, &storage.User{
		Username:    "drone-epsilon",
		DisplayName: "Drone Epsilon",
		Approved:    true,
		IsAgent:     true,
		AgentOwner:  "@owner",
		Metadata:    seedMetadata,
		CreatedAt:   now.Add(-24 * time.Hour),
		UpdatedAt:   now,
	}, &storage.AgentGovernanceState{
		Username:        "drone-epsilon",
		DelegatedScopes: []string{auth.ScopeRead, auth.ScopeWrite},
	})

	sent, err := (&mutationResolver{resolver}).SendSoulBootstrapConversationMessage(
		round13DroneAuthContext("reviewer", auth.ScopeWrite),
		model.SendSoulBootstrapConversationMessageInput{
			Username: "drone-epsilon",
			Message:  "Review my hosted identity.",
		},
	)
	require.NoError(t, err)
	require.Equal(t, model.SoulBootstrapPhaseConversation, sent.Bootstrap.State.Phase)
	require.Equal(t, workflow.SoulBootstrapStateConversationInProgress, sent.Bootstrap.State.State)
	require.Equal(t, "conv_123", derefString(sent.Bootstrap.State.HostConversationID))
	require.Equal(t, workflow.DroneWorkflowPhaseDeclaration, sent.Bootstrap.Workflow.CurrentPhase)

	completed, err := (&mutationResolver{resolver}).CompleteSoulBootstrapConversation(
		round13DroneAuthContext("reviewer", auth.ScopeWrite),
		model.CompleteSoulBootstrapConversationInput{
			Username:       "drone-epsilon",
			ConversationID: "conv_123",
		},
	)
	require.NoError(t, err)
	require.Equal(t, workflow.SoulBootstrapStateConversationCompleted, completed.Bootstrap.State.State)
	require.Equal(t, workflow.DroneWorkflowPhaseSigning, completed.Bootstrap.Workflow.CurrentPhase)
	require.NotNil(t, completed.Bootstrap.Workflow.Declaration)

	preflight, err := (&mutationResolver{resolver}).PrepareSoulBootstrapFinalize(
		round13DroneAuthContext("reviewer", auth.ScopeWrite),
		model.PrepareSoulBootstrapFinalizeInput{
			Username:               "drone-epsilon",
			ConversationID:         "conv_123",
			BoundarySignaturesJSON: round13StringPtr(`{"b1":"0xboundary"}`),
		},
	)
	require.NoError(t, err)
	require.Equal(t, model.SoulBootstrapPhaseFinalize, preflight.Bootstrap.State.Phase)
	require.Equal(t, workflow.SoulBootstrapStateFinalizeReady, preflight.Bootstrap.State.State)
	var finalizeCheckpoint *model.SoulBootstrapSigningCheckpoint
	for _, checkpoint := range preflight.Bootstrap.State.SigningCheckpoints {
		if checkpoint.Name == "finalize" {
			finalizeCheckpoint = checkpoint
			break
		}
	}
	require.NotNil(t, finalizeCheckpoint)
	require.Equal(t, "finalize", finalizeCheckpoint.Name)
	require.Equal(t, "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", derefString(finalizeCheckpoint.MessageHex))
	require.Equal(t, 0, *finalizeCheckpoint.ExpectedVersion)
	require.Contains(t, derefString(finalizeCheckpoint.BoundaryRequirementsJSON), `"boundary_id":"b1"`)

	finalized, err := (&mutationResolver{resolver}).FinalizeSoulBootstrap(
		round13DroneAuthContext("reviewer", auth.ScopeWrite),
		model.FinalizeSoulBootstrapInput{
			Username:               "drone-epsilon",
			ConversationID:         "conv_123",
			BoundarySignaturesJSON: round13StringPtr(`{"b1":"0xboundary"}`),
			SelfAttestation:        round13StringPtr("0xself"),
		},
	)
	require.NoError(t, err)
	require.Nil(t, finalized.Error)
	require.Equal(t, model.SoulBootstrapPhaseComplete, finalized.Bootstrap.State.Phase)
	require.Equal(t, workflow.SoulBootstrapStateCompleteBound, finalized.Bootstrap.State.State)
	require.Equal(t, agentID, derefString(finalized.Bootstrap.State.HostSoulAgentID))
	require.NotNil(t, finalized.Bootstrap.State.Publication)
	require.Equal(t, "hosted_offchain", derefString(finalized.Bootstrap.State.Publication.AnchorState))
	require.Equal(t, workflow.DroneWorkflowPhaseContinuity, finalized.Bootstrap.Workflow.CurrentPhase)
	require.NotNil(t, finalized.Bootstrap.Workflow.Continuity)

	replay, err := (&mutationResolver{resolver}).CompleteSoulBootstrapConversation(
		round13DroneAuthContext("reviewer", auth.ScopeWrite),
		model.CompleteSoulBootstrapConversationInput{
			Username:       "drone-epsilon",
			ConversationID: "conv_other",
		},
	)
	require.NoError(t, err)
	require.NotNil(t, replay.Error)
	require.Equal(t, workflow.SoulBootstrapErrorHostBootstrapReplayRejected, replay.Error.Code)
}

func TestRound44SoulBootstrapFinalizeBindingConflictIsTyped(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	now := time.Date(2026, 6, 12, 16, 0, 0, 0, time.UTC)
	issuedAt := time.Date(2026, 6, 12, 16, 5, 0, 0, time.UTC)
	const agentID = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	resolver.soulsClient = &stubSoulService{
		finalizeBootstrapFunc: func(_ context.Context, input soulservice.BootstrapFinalizeInput) (*soulservice.BootstrapFinalizeResult, error) {
			require.Equal(t, "reg_conflict", input.RegistrationID)
			require.Equal(t, "conv_conflict", input.ConversationID)
			publishedAt := issuedAt.Add(time.Minute)
			return &soulservice.BootstrapFinalizeResult{
				Version:          "1",
				HostSoulAgentID:  agentID,
				PublishedVersion: 1,
				Publication: soulservice.BootstrapPublicationEvidence{
					AgentID:          agentID,
					PublishedVersion: 1,
					AnchorState:      "hosted_offchain",
					PublishedAt:      &publishedAt,
				},
				HostRequestID: "host-req-finalize-conflict",
			}, nil
		},
		incorporateFunc: func(context.Context, string, string, string) (*soulservice.Soul, error) {
			return nil, soulservice.ErrTargetAgentAlreadyHasSoul
		},
	}

	metadata, err := workflow.SetDroneWorkflowMetadata(nil, &workflow.DroneWorkflowState{
		SoulBootstrap: &workflow.SoulBootstrapState{
			Username:           "drone-conflict",
			BodyID:             "drone-conflict",
			HostRegistrationID: "reg_conflict",
			HostConversationID: "conv_conflict",
			Phase:              workflow.SoulBootstrapPhaseFinalize,
			State:              workflow.SoulBootstrapStateFinalizeReady,
			SigningCheckpoints: []workflow.SoulBootstrapSigningCheckpoint{
				{
					Name:            "finalize",
					Status:          "ready",
					ExpectedVersion: 0,
					IssuedAt:        &issuedAt,
				},
			},
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
		Username:    "drone-conflict",
		DisplayName: "Drone Conflict",
		Approved:    true,
		IsAgent:     true,
		AgentOwner:  "@owner",
		Metadata:    metadata,
		CreatedAt:   now.Add(-24 * time.Hour),
		UpdatedAt:   now,
	}, &storage.AgentGovernanceState{
		Username:        "drone-conflict",
		DelegatedScopes: []string{auth.ScopeRead, auth.ScopeWrite},
	})

	payload, err := (&mutationResolver{resolver}).FinalizeSoulBootstrap(
		round13DroneAuthContext("owner", auth.ScopeWrite),
		model.FinalizeSoulBootstrapInput{
			Username:               "drone-conflict",
			ConversationID:         "conv_conflict",
			BoundarySignaturesJSON: round13StringPtr(`{"b1":"0xboundary"}`),
			SelfAttestation:        round13StringPtr("0xself"),
		},
	)
	require.NoError(t, err)
	require.NotNil(t, payload.Error)
	require.Equal(t, workflow.SoulBootstrapErrorSoulBindingConflict, payload.Error.Code)
	require.Equal(t, workflow.SoulBootstrapStateBindingConflict, payload.Bootstrap.State.State)
	require.Equal(t, agentID, derefString(payload.Bootstrap.State.HostSoulAgentID))
	require.NotNil(t, payload.Bootstrap.State.Publication)
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
