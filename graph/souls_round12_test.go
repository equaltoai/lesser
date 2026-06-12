package graph

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	soulservice "github.com/equaltoai/lesser/pkg/services/souls"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type stubSoulService struct {
	listMineFunc                      func(context.Context, string) ([]soulservice.Soul, error)
	incorporateFunc                   func(context.Context, string, string, string) (*soulservice.Soul, error)
	resolveBoundFunc                  func(context.Context, string) (*soulservice.Soul, error)
	beginBootstrapFunc                func(context.Context, soulservice.BootstrapBeginInput) (*soulservice.BootstrapBeginResult, error)
	prepareBootstrapPrincipalFunc     func(context.Context, soulservice.BootstrapPrincipalPreflightInput) (*soulservice.BootstrapPrincipalPreflightResult, error)
	verifyBootstrapPrincipalFunc      func(context.Context, soulservice.BootstrapPrincipalVerifyInput) (*soulservice.BootstrapPrincipalVerifyResult, error)
	sendBootstrapConversationFunc     func(context.Context, soulservice.BootstrapConversationMessageInput) (*soulservice.BootstrapConversationMessageResult, error)
	completeBootstrapConversationFunc func(context.Context, soulservice.BootstrapConversationCompleteInput) (*soulservice.BootstrapConversationCompleteResult, error)
	prepareBootstrapFinalizeFunc      func(context.Context, soulservice.BootstrapFinalizePreflightInput) (*soulservice.BootstrapFinalizePreflightResult, error)
	finalizeBootstrapFunc             func(context.Context, soulservice.BootstrapFinalizeInput) (*soulservice.BootstrapFinalizeResult, error)
}

func (s *stubSoulService) ListMine(ctx context.Context, username string) ([]soulservice.Soul, error) {
	if s.listMineFunc == nil {
		return nil, nil
	}
	return s.listMineFunc(ctx, username)
}

func (s *stubSoulService) Incorporate(ctx context.Context, username string, targetAgentUsername string, agentID string) (*soulservice.Soul, error) {
	if s.incorporateFunc == nil {
		return nil, nil
	}
	return s.incorporateFunc(ctx, username, targetAgentUsername, agentID)
}

func (s *stubSoulService) ResolveBoundAgent(ctx context.Context, agentUsername string) (*soulservice.Soul, error) {
	if s.resolveBoundFunc == nil {
		return nil, nil
	}
	return s.resolveBoundFunc(ctx, agentUsername)
}

func (s *stubSoulService) BeginBootstrapRegistration(ctx context.Context, input soulservice.BootstrapBeginInput) (*soulservice.BootstrapBeginResult, error) {
	if s.beginBootstrapFunc == nil {
		return nil, errors.New("begin bootstrap not implemented")
	}
	return s.beginBootstrapFunc(ctx, input)
}

func (s *stubSoulService) PrepareBootstrapPrincipalDeclaration(ctx context.Context, input soulservice.BootstrapPrincipalPreflightInput) (*soulservice.BootstrapPrincipalPreflightResult, error) {
	if s.prepareBootstrapPrincipalFunc == nil {
		return nil, errors.New("prepare bootstrap principal declaration not implemented")
	}
	return s.prepareBootstrapPrincipalFunc(ctx, input)
}

func (s *stubSoulService) VerifyBootstrapPrincipalDeclaration(ctx context.Context, input soulservice.BootstrapPrincipalVerifyInput) (*soulservice.BootstrapPrincipalVerifyResult, error) {
	if s.verifyBootstrapPrincipalFunc == nil {
		return nil, errors.New("verify bootstrap principal declaration not implemented")
	}
	return s.verifyBootstrapPrincipalFunc(ctx, input)
}

func (s *stubSoulService) SendBootstrapConversationMessage(ctx context.Context, input soulservice.BootstrapConversationMessageInput) (*soulservice.BootstrapConversationMessageResult, error) {
	if s.sendBootstrapConversationFunc == nil {
		return nil, errors.New("send bootstrap conversation not implemented")
	}
	return s.sendBootstrapConversationFunc(ctx, input)
}

func (s *stubSoulService) CompleteBootstrapConversation(ctx context.Context, input soulservice.BootstrapConversationCompleteInput) (*soulservice.BootstrapConversationCompleteResult, error) {
	if s.completeBootstrapConversationFunc == nil {
		return nil, errors.New("complete bootstrap conversation not implemented")
	}
	return s.completeBootstrapConversationFunc(ctx, input)
}

func (s *stubSoulService) PrepareBootstrapFinalize(ctx context.Context, input soulservice.BootstrapFinalizePreflightInput) (*soulservice.BootstrapFinalizePreflightResult, error) {
	if s.prepareBootstrapFinalizeFunc == nil {
		return nil, errors.New("prepare bootstrap finalize not implemented")
	}
	return s.prepareBootstrapFinalizeFunc(ctx, input)
}

func (s *stubSoulService) FinalizeBootstrap(ctx context.Context, input soulservice.BootstrapFinalizeInput) (*soulservice.BootstrapFinalizeResult, error) {
	if s.finalizeBootstrapFunc == nil {
		return nil, errors.New("finalize bootstrap not implemented")
	}
	return s.finalizeBootstrapFunc(ctx, input)
}

func soulAuthContext(username string, scopes ...string) context.Context {
	return context.WithValue(context.Background(), common.ContextKeyClaims, &auth.Claims{
		Username: username,
		Scopes:   scopes,
	})
}

func TestRound12SoulServiceHelpers_GetSoulService(t *testing.T) {
	var nilResolver *Resolver
	_, err := nilResolver.getSoulService()
	require.Error(t, err)

	resolver := &Resolver{}
	_, err = resolver.getSoulService()
	require.Error(t, err)

	withOverride := &Resolver{
		soulsClient: &stubSoulService{},
	}
	svc, err := withOverride.getSoulService()
	require.NoError(t, err)
	require.NotNil(t, svc)

	fullResolver, _ := newRound12GraphResolver(t)
	svc, err = fullResolver.getSoulService()
	require.NoError(t, err)
	require.NotNil(t, svc)
}

func TestRound12SoulsGraphQLHelpers_ConvertInventoryItem(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	ensName := "alice.eth"
	blankENS := "   "

	bound := toGraphQLSoulInventoryItem(soulservice.Soul{
		AgentID:                "0x123",
		Domain:                 "souls.example",
		LocalID:                "alice-soul",
		ENSName:                &ensName,
		Wallet:                 "0xabc",
		PrincipalAddress:       "0xabc",
		Status:                 "active",
		LifecycleStatus:        "active",
		SelfDescriptionVersion: intPtr(7),
		Capabilities:           nil,
		MintTxHash:             "0xmint",
		MintedAt:               &now,
		UpdatedAt:              &now,
		Bound:                  true,
		BoundAgentUsername:     "agent-alpha",
		BoundPrincipalAddress:  "0xabc",
		BoundAt:                now,
		BoundUpdatedAt:         now,
	})
	require.NotNil(t, bound)
	require.Equal(t, "alice.eth", *bound.Agent.EnsName)
	require.Equal(t, "0xabc", *bound.Agent.PrincipalAddress)
	require.NotNil(t, bound.Agent.MintedAt)
	require.NotNil(t, bound.Agent.UpdatedAt)
	require.NotNil(t, bound.Binding)
	require.Equal(t, "agent-alpha", bound.Binding.AgentUsername)
	require.Equal(t, "0xabc", *bound.Binding.PrincipalAddress)
	require.False(t, bound.AvailableForIncorporation)
	require.Equal(t, "BOUND", bound.BindingState.String())
	require.NotNil(t, bound.Agent.Capabilities)
	require.Empty(t, bound.Agent.Capabilities)

	unbound := toGraphQLSoulInventoryItem(soulservice.Soul{
		AgentID:         "0x456",
		Domain:          "souls.example",
		LocalID:         "blank-ens",
		ENSName:         &blankENS,
		Wallet:          "0xdef",
		Status:          "active",
		LifecycleStatus: "   ",
		MintTxHash:      "   ",
		Capabilities:    []string{"chat"},
	})
	require.NotNil(t, unbound)
	require.Nil(t, unbound.Agent.EnsName)
	require.Nil(t, unbound.Agent.PrincipalAddress)
	require.Nil(t, unbound.Agent.LifecycleStatus)
	require.Nil(t, unbound.Agent.MintTxHash)
	require.Nil(t, unbound.Binding)
	require.True(t, unbound.AvailableForIncorporation)
	require.Equal(t, "UNBOUND", unbound.BindingState.String())
	require.Equal(t, []string{"chat"}, unbound.Agent.Capabilities)
}

func TestRound12AgentLeaseWalletValidation_UsesBoundSoulIdentity(t *testing.T) {
	resolver := &Resolver{
		Logger: zap.NewNop(),
		soulsClient: &stubSoulService{
			resolveBoundFunc: func(_ context.Context, agentUsername string) (*soulservice.Soul, error) {
				require.Equal(t, "agent-alpha", agentUsername)
				return &soulservice.Soul{
					Wallet:                "0x1111111111111111111111111111111111111111",
					PrincipalAddress:      "0x2222222222222222222222222222222222222222",
					Bound:                 true,
					BoundAgentUsername:    "agent-alpha",
					BoundPrincipalAddress: "0x2222222222222222222222222222222222222222",
				}, nil
			},
		},
	}

	usedBoundSoul, principalOK, agentOK, err := resolver.validateGraphBoundAgentAccessLeaseWallets(
		context.Background(),
		"agent-alpha",
		"0x2222222222222222222222222222222222222222",
		"0x1111111111111111111111111111111111111111",
	)
	require.NoError(t, err)
	require.True(t, usedBoundSoul)
	require.True(t, principalOK)
	require.True(t, agentOK)
}

func TestRound12SoulsQuery_MySouls(t *testing.T) {
	resolver := &Resolver{
		Logger: zap.NewNop(),
		soulsClient: &stubSoulService{
			listMineFunc: func(_ context.Context, username string) ([]soulservice.Soul, error) {
				require.Equal(t, "alice", username)
				ensName := "alice.eth"
				return []soulservice.Soul{
					{
						AgentID:            "0x1",
						Domain:             "souls.example",
						LocalID:            "bound-to-viewer",
						ENSName:            &ensName,
						Wallet:             "0xabc",
						Status:             "active",
						Bound:              true,
						BoundAgentUsername: "agent-alpha",
					},
					{
						AgentID:            "0x2",
						Domain:             "souls.example",
						LocalID:            "bound-away",
						Wallet:             "0xdef",
						Status:             "active",
						Bound:              true,
						BoundAgentUsername: "agent-beta",
					},
				}, nil
			},
		},
	}

	items, err := (&queryResolver{resolver}).MySouls(soulAuthContext("alice", auth.ScopeRead))
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.False(t, items[0].AvailableForIncorporation)
	require.False(t, items[1].AvailableForIncorporation)
	require.Equal(t, "alice.eth", *items[0].Agent.EnsName)
}

func TestRound12SoulsQuery_MySouls_AuthAndErrors(t *testing.T) {
	resolver := &Resolver{
		Logger: zap.NewNop(),
		soulsClient: &stubSoulService{
			listMineFunc: func(context.Context, string) ([]soulservice.Soul, error) {
				return nil, soulservice.ErrTrustNotConfigured
			},
		},
	}
	query := &queryResolver{resolver}

	_, err := query.MySouls(context.Background())
	require.Error(t, err)
	require.True(t, apperrors.HasCode(err, apperrors.CodeUnauthorized))

	_, err = query.MySouls(soulAuthContext("alice"))
	require.Error(t, err)
	require.True(t, apperrors.HasCode(err, apperrors.CodeInsufficientScope))

	_, err = query.MySouls(soulAuthContext("alice", auth.ScopeWrite))
	require.Error(t, err)
	require.True(t, apperrors.HasCode(err, apperrors.CodeUnprocessableEntity))
}

func TestRound12SoulsMutation_IncorporateSoul(t *testing.T) {
	resolver := &Resolver{
		Logger: zap.NewNop(),
		soulsClient: &stubSoulService{
			incorporateFunc: func(_ context.Context, username string, targetAgentUsername string, agentID string) (*soulservice.Soul, error) {
				require.Equal(t, "alice", username)
				require.Equal(t, "agent-alpha", targetAgentUsername)
				require.Equal(t, "0xabc", agentID)
				now := time.Now().UTC().Truncate(time.Second)
				return &soulservice.Soul{
					AgentID:            agentID,
					Domain:             "souls.example",
					LocalID:            "alice-soul",
					Wallet:             "0xwallet",
					Status:             "active",
					Bound:              true,
					BoundAgentUsername: targetAgentUsername,
					BoundAt:            now,
					BoundUpdatedAt:     now,
				}, nil
			},
		},
	}
	mut := &mutationResolver{resolver}

	item, err := mut.IncorporateSoul(soulAuthContext("alice", auth.ScopeWrite), "0xabc", "agent-alpha")
	require.NoError(t, err)
	require.NotNil(t, item)
	require.Equal(t, "alice-soul", item.Agent.LocalID)
	require.Equal(t, "BOUND", item.BindingState.String())
	require.False(t, item.AvailableForIncorporation)
	require.NotNil(t, item.Binding)
	require.Equal(t, "agent-alpha", item.Binding.AgentUsername)

	_, err = mut.IncorporateSoul(soulAuthContext("alice", auth.ScopeRead), "0xabc", "agent-alpha")
	require.Error(t, err)
	require.True(t, apperrors.HasCode(err, apperrors.CodeInsufficientScope))

	_, err = mut.IncorporateSoul(soulAuthContext("alice", auth.ScopeWrite), "   ", "agent-alpha")
	require.Error(t, err)

	_, err = mut.IncorporateSoul(soulAuthContext("alice", auth.ScopeWrite), "0xabc", "   ")
	require.Error(t, err)
}

func TestRound12SoulsMutation_IncorporateSoul_ErrorMapping(t *testing.T) {
	resolver := &Resolver{
		Logger: zap.NewNop(),
		soulsClient: &stubSoulService{
			incorporateFunc: func(context.Context, string, string, string) (*soulservice.Soul, error) {
				return nil, soulservice.ErrSoulAlreadyBound
			},
		},
	}

	_, err := (&mutationResolver{resolver}).IncorporateSoul(soulAuthContext("alice", auth.ScopeWrite), "0xabc", "agent-alpha")
	require.Error(t, err)
	require.True(t, apperrors.HasCode(err, apperrors.CodeConflict))

	resolver.soulsClient = &stubSoulService{
		incorporateFunc: func(context.Context, string, string, string) (*soulservice.Soul, error) {
			return nil, soulservice.ErrSoulNotAvailable
		},
	}

	_, err = (&mutationResolver{resolver}).IncorporateSoul(soulAuthContext("alice", auth.ScopeWrite), "0xabc", "agent-alpha")
	require.Error(t, err)
	require.True(t, apperrors.HasCode(err, apperrors.CodeNotFound))

	resolver.soulsClient = &stubSoulService{
		incorporateFunc: func(context.Context, string, string, string) (*soulservice.Soul, error) {
			return nil, soulservice.ErrTargetAgentRequired
		},
	}

	_, err = (&mutationResolver{resolver}).IncorporateSoul(soulAuthContext("alice", auth.ScopeWrite), "0xabc", "agent-alpha")
	require.Error(t, err)
	require.True(t, apperrors.HasCode(err, apperrors.CodeUnprocessableEntity))

	resolver.soulsClient = &stubSoulService{
		incorporateFunc: func(context.Context, string, string, string) (*soulservice.Soul, error) {
			return nil, soulservice.ErrTargetAgentNotOwned
		},
	}

	_, err = (&mutationResolver{resolver}).IncorporateSoul(soulAuthContext("alice", auth.ScopeWrite), "0xabc", "agent-beta")
	require.Error(t, err)
	require.True(t, apperrors.HasCode(err, apperrors.CodeForbidden))

	resolver.soulsClient = &stubSoulService{
		incorporateFunc: func(context.Context, string, string, string) (*soulservice.Soul, error) {
			return nil, soulservice.ErrTargetAgentMustBeAgent
		},
	}

	_, err = (&mutationResolver{resolver}).IncorporateSoul(soulAuthContext("alice", auth.ScopeWrite), "0xabc", "alice")
	require.Error(t, err)
	require.True(t, apperrors.HasCode(err, apperrors.CodeUnprocessableEntity))
}

func intPtr(value int) *int {
	return &value
}
