package graph

import (
	"context"
	"strings"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"go.uber.org/zap"
)

// IncorporateSoul covers POST /api/v1/souls/{agentId}/incorporate.
func (r *mutationResolver) IncorporateSoul(ctx context.Context, agentID string) (*model.SoulInventoryItem, error) {
	claims, err := r.requireAuthClaims(ctx)
	if err != nil {
		return nil, err
	}
	if !claims.HasScope(auth.ScopeWrite) {
		return nil, apperrors.InsufficientScope(auth.ScopeWrite)
	}

	agentID = strings.TrimSpace(agentID)
	if err := common.ValidateRequiredParam("agentId", agentID); err != nil {
		return nil, err
	}

	service, err := r.getSoulService()
	if err != nil {
		return nil, err
	}

	soul, err := service.Incorporate(ctx, claims.Username, agentID)
	if err != nil {
		if r.Logger != nil {
			r.Logger.Error(
				"failed to incorporate soul",
				zap.String("username", claims.Username),
				zap.String("agent_id", agentID),
				zap.Error(err),
			)
		}
		return nil, mapSoulServiceError(err)
	}

	return toGraphQLSoulInventoryItem(claims.Username, *soul), nil
}
