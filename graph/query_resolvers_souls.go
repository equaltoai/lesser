package graph

import (
	"context"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/auth"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"go.uber.org/zap"
)

// MySouls covers GET /api/v1/souls/mine.
func (r *queryResolver) MySouls(ctx context.Context) ([]*model.SoulInventoryItem, error) {
	claims, err := r.requireAuthClaims(ctx)
	if err != nil {
		return nil, err
	}
	if !claims.HasScope(auth.ScopeRead) && !claims.HasScope(auth.ScopeWrite) {
		return nil, apperrors.NewAuthError(
			apperrors.CodeInsufficientScope,
			"insufficient scope: requires one of read, write",
		)
	}

	service, err := r.getSoulService()
	if err != nil {
		return nil, err
	}

	souls, err := service.ListMine(ctx, claims.Username)
	if err != nil {
		if r.Logger != nil {
			r.Logger.Error("failed to list souls", zap.String("username", claims.Username), zap.Error(err))
		}
		return nil, mapSoulServiceError(err)
	}

	items := make([]*model.SoulInventoryItem, 0, len(souls))
	for _, soul := range souls {
		items = append(items, toGraphQLSoulInventoryItem(claims.Username, soul))
	}

	return items, nil
}
