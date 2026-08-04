package handlers

import (
	"context"
	"errors"
	"strings"
	"time"

	apptheory "github.com/theory-cloud/apptheory/v3/runtime"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/core"
)

var errAgentGovernanceUnavailable = errors.New("agent governance state unavailable")

func loadAgentGovernanceState(ctx context.Context, repos core.RepositoryStorage, username string) (*storage.AgentGovernanceState, error) {
	if repos == nil || repos.Account() == nil {
		return nil, nil
	}
	state, err := repos.Account().GetAgentGovernanceState(ctx, username)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return state, nil
}

func loadAgentGovernanceStates(ctx context.Context, repos core.RepositoryStorage, usernames []string) (map[string]*storage.AgentGovernanceState, error) {
	if repos == nil || repos.Account() == nil {
		return map[string]*storage.AgentGovernanceState{}, nil
	}
	states, err := repos.Account().GetAgentGovernanceStatesByUsernames(ctx, usernames)
	if err != nil {
		return nil, err
	}
	if states == nil {
		return map[string]*storage.AgentGovernanceState{}, nil
	}
	return states, nil
}

func requireAgentGovernanceState(ctx context.Context, repos core.RepositoryStorage, username string) (*storage.AgentGovernanceState, error) {
	state, err := loadAgentGovernanceState(ctx, repos, username)
	if err != nil {
		return nil, err
	}
	if state == nil {
		return nil, errAgentGovernanceUnavailable
	}
	return state, nil
}

func respondAgentGovernanceUnavailable(ctx *apptheory.Context) (*apptheory.Response, error) {
	return common.RespondServiceUnavailable(ctx, "agent governance")
}

func respondAgentGovernanceWriteError(ctx *apptheory.Context, err error) (*apptheory.Response, error) {
	if errors.Is(err, storage.ErrVersionConflict) {
		return common.RespondConflict(ctx, "agent governance state changed concurrently")
	}
	return common.RespondInternalServerError(ctx)
}

func agentVerifiedState(state *storage.AgentGovernanceState) bool {
	return state != nil && state.Verified
}

func agentDelegatedScopes(state *storage.AgentGovernanceState) []string {
	if state == nil {
		return nil
	}
	return state.DelegatedScopesCopy()
}

func applyAgentQuarantineExit(governance *storage.AgentGovernanceState, claims *auth.Claims, exitQuarantine bool, now time.Time) *storage.AgentGovernanceState {
	if !exitQuarantine {
		return governance
	}
	if governance == nil {
		governance = &storage.AgentGovernanceState{}
	}

	approvedBy := ""
	if claims != nil {
		approvedBy = strings.TrimSpace(claims.Username)
	}

	now = now.UTC()
	governance.QuarantineStatus = storage.AgentQuarantineStatusApproved
	governance.QuarantineEnd = cloneAgentGovernanceHandlerTime(&now)
	governance.QuarantineApprovedBy = approvedBy
	governance.QuarantineApprovedAt = cloneAgentGovernanceHandlerTime(&now)
	return governance
}

func agentSelfSovereignScopes(governance *storage.AgentGovernanceState) []string {
	scopes := []string{auth.ScopeRead, auth.ScopeWrite, "follow"}
	if governance == nil || len(governance.SelfScopes) == 0 {
		return scopes
	}
	return normalizeSelfSovereignScopes(governance.SelfScopesCopy())
}

func cloneAgentGovernanceHandlerTime(value *time.Time) *time.Time {
	if value == nil || value.IsZero() {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}
