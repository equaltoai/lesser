package graph

import (
	"context"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
)

// AgentRuntimeSessions is the resolver for the agentRuntimeSessions field.
func (r *queryResolver) AgentRuntimeSessions(ctx context.Context, username string) ([]*model.AgentRuntimeSession, error) {
	if err := r.ensureAgentsEnabled(ctx); err != nil {
		return nil, err
	}
	username = strings.TrimSpace(username)
	if err := common.ValidateUsernameParamID(username); err != nil {
		return nil, err
	}
	if _, _, err := r.requireManagedAgentLeaseAccount(ctx, username); err != nil {
		return nil, err
	}

	sessions, err := auth.ListAgentRuntimeSessions(ctx, r.Storage, username)
	if err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to list agent runtime sessions")
	}

	out := make([]*model.AgentRuntimeSession, 0, len(sessions))
	for i := range sessions {
		out = append(out, graphAgentRuntimeSessionModel(sessions[i]))
	}
	return out, nil
}

// RevokeAgentRuntimeSession is the resolver for the revokeAgentRuntimeSession field.
func (r *mutationResolver) RevokeAgentRuntimeSession(ctx context.Context, username string, sessionID string, reason *string) (*model.AgentRuntimeSession, error) {
	if err := r.ensureAgentsEnabled(ctx); err != nil {
		return nil, err
	}
	username = strings.TrimSpace(username)
	sessionID = strings.TrimSpace(sessionID)
	if err := common.ValidateUsernameParamID(username); err != nil {
		return nil, err
	}
	if err := common.ValidateRequiredParam("sessionID", sessionID); err != nil {
		return nil, err
	}
	if _, _, err := r.requireManagedAgentLeaseAccount(ctx, username); err != nil {
		return nil, err
	}

	sessions, err := auth.ListAgentRuntimeSessions(ctx, r.Storage, username)
	if err != nil {
		return nil, apperrors.InternalWithCause(err, "failed to list agent runtime sessions")
	}

	var target *storage.RefreshToken
	for i := range sessions {
		if sessions[i].SessionID == sessionID {
			copyToken := sessions[i]
			target = &copyToken
			break
		}
	}
	if target == nil {
		return nil, apperrors.NewAppError(apperrors.CodeNotFound, apperrors.CategoryBusiness, "agent runtime session not found")
	}

	revokeReason := strings.TrimSpace(derefString(reason))
	if revokeReason == "" {
		revokeReason = "manual_runtime_session_revocation"
	}
	if err := auth.RevokeAgentRuntimeSession(ctx, r.Storage, username, sessionID, revokeReason, "", ""); err != nil {
		if err == storage.ErrNotFound {
			return nil, apperrors.NewAppError(apperrors.CodeNotFound, apperrors.CategoryBusiness, "agent runtime session not found")
		}
		return nil, apperrors.InternalWithCause(err, "failed to revoke agent runtime session")
	}

	now := time.Now().UTC()
	target.Revoked = true
	target.RevokedAt = now
	target.RevokedReason = revokeReason
	target.Current = false

	return graphAgentRuntimeSessionModel(*target), nil
}

func graphAgentRuntimeSessionModel(token storage.RefreshToken) *model.AgentRuntimeSession {
	createdAt := token.SessionCreatedAt
	if createdAt.IsZero() {
		createdAt = token.CreatedAt
	}

	var revokedAt *time.Time
	if !token.RevokedAt.IsZero() {
		value := token.RevokedAt
		revokedAt = &value
	}

	return &model.AgentRuntimeSession{
		SessionID:         token.SessionID,
		ClientID:          token.ClientID,
		DeviceLabel:       auth.CoalesceAgentRuntimeLabel(token.DeviceLabel, ""),
		Scope:             strings.Join(token.Scopes, " "),
		CreatedAt:         model.Time(createdAt),
		LastUsedAt:        model.Time(token.LastUsedAt),
		IdleExpiresAt:     model.Time(token.IdleExpiresAt),
		AbsoluteExpiresAt: model.Time(token.AbsoluteExpiresAt),
		Revoked:           token.Revoked,
		RevokedAt:         modelTimePtr(revokedAt),
		RevokedReason:     optionalString(token.RevokedReason),
	}
}
