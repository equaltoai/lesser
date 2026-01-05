package lift

import (
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
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

	req.Username = strings.TrimSpace(req.Username)
	if err := common.ValidateRequiredParam("username", req.Username); err != nil {
		return h.respondUnprocessableEntity(ctx, "username is required")
	}

	role := strings.TrimSpace(req.Role)
	if role == "" {
		role = "user"
	}

	// NOTE: Password-based authentication is disabled (wallet/passkey only). Ignore any provided credentials.
	if req.Password != "" {
		h.logger.Warn("password provided in admin create user request but passwords are disabled",
			zap.String("username", req.Username))
		req.Password = ""
	}
	if req.Email != "" {
		h.logger.Warn("email provided in admin create user request but email is disabled",
			zap.String("username", req.Username))
		req.Email = ""
	}

	user := &storage.User{
		Username:     req.Username,
		Email:        "",
		PasswordHash: "",
		DisplayName:  strings.TrimSpace(req.DisplayName),
		Role:         role,
		Approved:     true,
		CreatedAt:    time.Now(),
	}

	if err := h.repos.User().CreateUser(ctx.Context, user); err != nil {
		h.logger.Error("failed to create user", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	user.Email = ""
	user.PasswordHash = ""
	return ctx.Status(201).JSON(user)
}
