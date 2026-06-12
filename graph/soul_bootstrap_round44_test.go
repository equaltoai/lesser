package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	workflow "github.com/equaltoai/lesser/pkg/agents"
	"github.com/equaltoai/lesser/pkg/auth"
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

func TestRound44SoulBootstrapBeginPersistsHostState(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	now := time.Date(2026, 6, 12, 13, 0, 0, 0, time.UTC)
	hostIssuedAt := time.Date(2026, 6, 12, 13, 1, 0, 0, time.UTC)
	hostExpiresAt := hostIssuedAt.Add(10 * time.Minute)
	resolver.soulsClient = &stubSoulService{
		beginBootstrapFunc: func(_ context.Context, input soulservice.BootstrapBeginInput) (*soulservice.BootstrapBeginResult, error) {
			require.NotEmpty(t, input.BodyID)
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
		ID:          "drone-beta",
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
