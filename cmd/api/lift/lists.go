package lift

import (
	"net/http"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/lists"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleGetListsLift handles GET /api/v1/lists
func (h *Handler) HandleGetListsLift(ctx *lift.Context) error {
	claims, err := h.authenticateWithScope(ctx, auth.ScopeRead)
	if err != nil {
		return err
	}

	result, err := h.registry.Lists().ListUserLists(ctx.Context, &lists.ListUserListsQuery{
		Username: claims.Username,
		ViewerID: claims.Username,
	})
	if err != nil {
		h.logger.Error("failed to get user lists", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "failed to get lists"})
	}

	return ctx.JSON(result.Lists)
}

// HandleCreateListLift handles POST /api/v1/lists
func (h *Handler) HandleCreateListLift(ctx *lift.Context) error {
	var req models.CreateListRequest
	if err := ctx.ParseRequest(&req); err != nil {
		if ctx.Request != nil && ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
			if err := common.ParseRequestBody(ctx.Request.Body, &req); err != nil {
				return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "invalid request body"})
			}
		} else {
			return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "invalid request body"})
		}
	}

	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		return err
	}

	result, err := h.registry.Lists().CreateList(ctx.Context, &lists.CreateListCommand{
		Username:      claims.Username,
		Title:         req.Title,
		RepliesPolicy: req.RepliesPolicy,
		CreatorID:     claims.Username,
	})
	if err != nil {
		h.logger.Error("failed to create list", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "failed to create list"})
	}

	return ctx.Status(http.StatusCreated).JSON(result.List)
}

// HandleGetListLift handles GET /api/v1/lists/:id
func (h *Handler) HandleGetListLift(ctx *lift.Context) error {
	listID := ctx.Param("id")
	if listID == "" {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "missing list id"})
	}

	claims, err := h.authenticateWithScope(ctx, auth.ScopeRead)
	if err != nil {
		return err
	}

	list, err := h.registry.Lists().GetList(ctx.Context, &lists.GetListQuery{
		ListID:   listID,
		ViewerID: claims.Username,
	})
	if err != nil {
		return ctx.Status(http.StatusNotFound).JSON(map[string]string{"error": "list not found"})
	}

	return ctx.JSON(list)
}

// HandleUpdateListLift handles PUT /api/v1/lists/:id
func (h *Handler) HandleUpdateListLift(ctx *lift.Context) error {
	listID := ctx.Param("id")
	if listID == "" {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "missing list id"})
	}

	var req models.UpdateListRequest
	if err := ctx.ParseRequest(&req); err != nil {
		if ctx.Request != nil && ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
			if err := common.ParseRequestBody(ctx.Request.Body, &req); err != nil {
				return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "invalid request body"})
			}
		} else {
			return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "invalid request body"})
		}
	}

	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		return err
	}

	result, err := h.registry.Lists().UpdateList(ctx.Context, &lists.UpdateListCommand{
		ListID:        listID,
		Title:         req.Title,
		RepliesPolicy: req.RepliesPolicy,
		UpdaterID:     claims.Username,
	})
	if err != nil {
		h.logger.Error("failed to update list", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "failed to update list"})
	}

	return ctx.JSON(result.List)
}

// HandleDeleteListLift handles DELETE /api/v1/lists/:id
func (h *Handler) HandleDeleteListLift(ctx *lift.Context) error {
	listID := ctx.Param("id")
	if listID == "" {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "missing list id"})
	}

	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		return err
	}

	if err := h.registry.Lists().DeleteList(ctx.Context, &lists.DeleteListCommand{
		ListID:    listID,
		DeleterID: claims.Username,
	}); err != nil {
		h.logger.Error("failed to delete list", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "failed to delete list"})
	}

	return ctx.Status(http.StatusOK).JSON(map[string]string{})
}

// HandleGetListAccountsLift handles GET /api/v1/lists/:id/accounts
func (h *Handler) HandleGetListAccountsLift(ctx *lift.Context) error {
	listID := ctx.Param("id")
	if listID == "" {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "missing list id"})
	}

	claims, err := h.authenticateWithScope(ctx, auth.ScopeRead)
	if err != nil {
		return err
	}

	// Verify list ownership using service
	_, err = h.registry.Lists().GetList(ctx.Context, &lists.GetListQuery{
		ListID:   listID,
		ViewerID: claims.Username,
	})
	if err != nil {
		return ctx.Status(http.StatusNotFound).JSON(map[string]string{"error": "list not found"})
	}

	// Get accounts in the list using service
	membersResult, err := h.registry.Lists().GetListMembers(ctx.Context, &lists.GetListMembersQuery{
		ListID:     listID,
		ViewerID:   claims.Username,
		Pagination: interfaces.PaginationOptions{Limit: 100},
	})
	if err != nil {
		h.logger.Error("failed to get list accounts", zap.String("list_id", listID), zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "failed to get list accounts"})
	}

	// Convert members to Account objects
	accounts := make([]*models.Account, 0, len(membersResult.Members))
	for _, member := range membersResult.Members {
		// Get the actor from the storage account
		if member.Actor != nil {
			account := h.converter.ActorToAccount(member.Actor)
			accounts = append(accounts, &account)
		} else {
			h.logger.Warn("list member has no actor data", zap.String("username", member.User.Username))
		}
	}

	return ctx.JSON(accounts)
}

// HandleAddAccountsToListLift handles POST /api/v1/lists/:id/accounts
func (h *Handler) HandleAddAccountsToListLift(ctx *lift.Context) error {
	listID := ctx.Param("id")
	if listID == "" {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "missing list id"})
	}

	var req models.AddAccountsRequest
	if err := ctx.ParseRequest(&req); err != nil {
		if ctx.Request != nil && ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
			if err := common.ParseRequestBody(ctx.Request.Body, &req); err != nil {
				return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "invalid request body"})
			}
		} else {
			return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "invalid request body"})
		}
	}

	if len(req.AccountIDs) == 0 {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "account_ids is required"})
	}

	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		return err
	}

	// Add accounts to list via service (iterating for API compatibility)
	for _, accountID := range req.AccountIDs {
		_, err := h.registry.Lists().AddToList(ctx.Context, &lists.AddToListCommand{
			ListID:         listID,
			MemberUsername: accountID,
			AdderID:        claims.Username,
		})
		if err != nil {
			h.logger.Error("failed to add account to list", zap.String("account_id", accountID), zap.Error(err))
			return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "failed to add accounts to list"})
		}
	}

	return ctx.Status(http.StatusOK).JSON(map[string]string{})
}

// HandleRemoveAccountsFromListLift handles DELETE /api/v1/lists/:id/accounts
func (h *Handler) HandleRemoveAccountsFromListLift(ctx *lift.Context) error {
	listID := ctx.Param("id")
	if listID == "" {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "missing list id"})
	}

	var req models.RemoveAccountsRequest
	if err := ctx.ParseRequest(&req); err != nil {
		if ctx.Request != nil && ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
			if err := common.ParseRequestBody(ctx.Request.Body, &req); err != nil {
				return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "invalid request body"})
			}
		} else {
			return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "invalid request body"})
		}
	}

	if len(req.AccountIDs) == 0 {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "account_ids is required"})
	}

	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		return err
	}

	// Remove accounts from list via service (iterating for API compatibility)
	for _, accountID := range req.AccountIDs {
		_, err := h.registry.Lists().RemoveFromList(ctx.Context, &lists.RemoveFromListCommand{
			ListID:         listID,
			MemberUsername: accountID,
			RemoverID:      claims.Username,
		})
		if err != nil {
			h.logger.Error("failed to remove account from list", zap.String("account_id", accountID), zap.Error(err))
			return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "failed to remove accounts from list"})
		}
	}

	return ctx.Status(http.StatusOK).JSON(map[string]string{})
}
