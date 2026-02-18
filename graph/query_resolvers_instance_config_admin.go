package graph

import (
	"context"

	"github.com/equaltoai/lesser/graph/model"
)

// AdminInstanceConfig returns layered instance-owned configuration for administrators.
func (r *queryResolver) AdminInstanceConfig(ctx context.Context) (*model.AdminInstanceConfig, error) {
	if _, err := r.requireAdmin(ctx); err != nil {
		return nil, err
	}

	return r.resolveAdminInstanceConfig(ctx)
}
