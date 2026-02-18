package graph

import (
	"context"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/security/authz"
)

// ViewerRole returns the authenticated user's normalized role and admin status.
func (r *queryResolver) ViewerRole(ctx context.Context) (*model.ViewerRole, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	role, err := r.normalizedRole(ctx, username)
	if err != nil {
		return nil, err
	}
	if role == "" {
		role = authz.NormalizeRole("")
	}

	return &model.ViewerRole{
		Role:    role,
		IsAdmin: authz.IsAdmin(role),
	}, nil
}
