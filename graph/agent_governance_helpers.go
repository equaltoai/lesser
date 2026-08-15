package graph

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
)

func (r *Resolver) loadAgentGovernanceState(ctx context.Context, username string) (*storage.AgentGovernanceState, error) {
	if r == nil || r.Storage == nil || r.Storage.Account() == nil {
		return nil, nil
	}
	state, err := r.Storage.Account().GetAgentGovernanceState(ctx, username)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return state, nil
}

func (r *Resolver) loadAgentGovernanceStates(ctx context.Context, usernames []string) (map[string]*storage.AgentGovernanceState, error) {
	if r == nil || r.Storage == nil || r.Storage.Account() == nil {
		return map[string]*storage.AgentGovernanceState{}, nil
	}
	states, err := r.Storage.Account().GetAgentGovernanceStatesByUsernames(ctx, usernames)
	if err != nil {
		return nil, err
	}
	if states == nil {
		return map[string]*storage.AgentGovernanceState{}, nil
	}
	return states, nil
}

func (r *Resolver) requireAgentGovernanceState(ctx context.Context, username string) (*storage.AgentGovernanceState, error) {
	if r == nil || r.Storage == nil || r.Storage.Account() == nil {
		return nil, ErrStorageUnavailable
	}
	state, err := r.Storage.Account().GetAgentGovernanceState(ctx, username)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, ErrAgentGovernanceUnavailable
		}
		return nil, err
	}
	return state, nil
}

func graphAgentGovernanceLoadError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrAgentGovernanceUnavailable) || errors.Is(err, ErrStorageUnavailable) {
		return err
	}
	return apperrors.InternalWithCause(err, "failed to load agent governance state")
}

func graphAgentGovernanceWriteError(err error, action string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, storage.ErrVersionConflict) {
		return apperrors.NewAppError(
			apperrors.CodeConflict,
			apperrors.CategoryBusiness,
			"agent governance state changed concurrently",
		)
	}
	return apperrors.InternalWithCause(err, fmt.Sprintf("failed to %s agent governance state", action))
}

func (r *Resolver) agentOwnerActorURL(username string) string {
	if r == nil || r.Config == nil {
		return ""
	}
	return r.Config.ActorURL(strings.TrimSpace(username))
}

func optionalGraphAuthClaims(ctx context.Context) *auth.Claims {
	claims, _ := ctx.Value(common.ContextKeyClaims).(*auth.Claims)
	if claims == nil || strings.TrimSpace(claims.Username) == "" {
		return nil
	}
	return claims
}

func (r *Resolver) canViewAgentPrivateFields(claims *auth.Claims, agentUser *storage.User) bool {
	if claims == nil || agentUser == nil {
		return false
	}
	return isAgentOwnerOrAdmin(claims, agentUser, r.agentOwnerActorURL(claims.Username))
}

func (r *Resolver) viewerOwnsAgent(claims *auth.Claims, agentUser *storage.User) bool {
	if claims == nil || agentUser == nil {
		return false
	}
	return auth.AgentOwnerMatchesLocalPrincipal(agentUser.AgentOwner, claims.Username, r.agentOwnerActorURL)
}

func (r *Resolver) applyGraphAgentViewerState(agent *model.Agent, claims *auth.Claims, agentUser *storage.User) *model.Agent {
	if agent == nil {
		return nil
	}
	agent.ViewerIsOwner = r.viewerOwnsAgent(claims, agentUser)
	if !r.canViewAgentPrivateFields(claims, agentUser) {
		redactGraphAgentPrivateFields(agent)
	}
	return agent
}

func graphClaimsIsAdmin(claims *auth.Claims) bool {
	return claims != nil && (claims.HasScope(auth.ScopeAdmin) || claims.HasScope("admin:write") || claims.HasScope("admin:all"))
}
