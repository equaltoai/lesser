package lift

import (
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleAdminCreateUserLift handles the creation of a new user by an admin.
func (h *Handler) HandleAdminCreateUserLift(ctx *lift.Context) error {
	if _, err := h.requireAdminLift(ctx); err != nil {
		return h.respondUnauthorized(ctx)
	}

	var req models.AdminCreateUserRequest
	if err := ctx.ParseRequest(&req); err != nil {
		h.logger.Error("failed to parse user from request", zap.Error(err))
		return h.respondUnprocessableEntity(ctx, "invalid user data")
	}

	hashedPassword, err := auth.HashPassword(req.Password)
	if err != nil {
		h.logger.Error("failed to hash password", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	user := &storage.User{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: hashedPassword,
		DisplayName:  req.DisplayName,
		Role:         req.Role,
		Approved:     true,
		CreatedAt:    time.Now(),
	}

	if err := h.repos.User().CreateUser(ctx.Context, user); err != nil {
		h.logger.Error("failed to create user", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	return ctx.Status(201).JSON(user)
}
