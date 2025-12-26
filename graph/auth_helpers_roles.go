package graph

import (
	"context"
	"errors"
	"strings"

	"go.uber.org/zap"
)

func (r *Resolver) requireModeratorOrAdmin(ctx context.Context) (string, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return "", err
	}

	if r.Registry == nil {
		return "", errors.New("service registry is not available")
	}

	accountsService := r.Registry.Accounts()
	if accountsService == nil {
		return "", errors.New("accounts service is not available")
	}

	account, err := accountsService.GetAccount(ctx, username)
	if err != nil {
		return "", errors.Join(errors.New("failed to verify role"), err)
	}

	role := ""
	if account != nil && account.User != nil {
		role = strings.ToLower(account.User.Role)
	}

	if role != adminRoleAdmin && role != adminRoleModerator {
		r.Logger.Warn("Non-moderator attempted moderator operation",
			zap.String("username", username),
			zap.String("role", role))
		return "", ErrModeratorPrivilegesRequired
	}

	return username, nil
}
