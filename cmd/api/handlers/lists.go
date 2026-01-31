package lift

import (
	"net/http"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/lists"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/transformations"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

func apiListFromStorage(list *storageModels.List) models.List {
	if list == nil {
		return models.List{}
	}
	return models.List{
		ID:            list.ID,
		Title:         list.Title,
		RepliesPolicy: list.RepliesPolicy,
	}
}

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
		return common.RespondFailedToGet(ctx, "lists")
	}

	response := make([]models.List, 0, len(result.Lists))
	for _, list := range result.Lists {
		response = append(response, apiListFromStorage(list))
	}
	return ctx.JSON(response)
}

// HandleCreateListLift handles POST /api/v1/lists
func (h *Handler) HandleCreateListLift(ctx *lift.Context) error {
	var req models.CreateListRequest
	if err := ctx.ParseRequest(&req); err != nil {
		if ctx.Request != nil && ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
			if err := common.ParseRequestBody(ctx.Request.Body, &req); err != nil {
				return common.RespondBadRequest(ctx, "invalid request body")
			}
		} else {
			return common.RespondBadRequest(ctx, "invalid request body")
		}
	}

	// Validate list parameters
	params := map[string]interface{}{
		"title":          req.Title,
		"replies_policy": req.RepliesPolicy,
	}
	if err := common.ValidateListParams(params); err != nil {
		h.logger.Info("list validation failed", zap.Error(err))
		return common.RespondBadRequest(ctx, err.Error())
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
		return common.RespondFailedToCreate(ctx, "list")
	}

	return ctx.Status(http.StatusCreated).JSON(apiListFromStorage(result.List))
}

// HandleGetListLift handles GET /api/v1/lists/:id
func (h *Handler) HandleGetListLift(ctx *lift.Context) error {
	listID := ctx.Param("id")
	if err := common.ValidateRequiredParam("listID", listID); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
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
		return common.RespondNotFound(ctx, "list not found")
	}

	return ctx.JSON(apiListFromStorage(list))
}

// HandleUpdateListLift handles PUT /api/v1/lists/:id
func (h *Handler) HandleUpdateListLift(ctx *lift.Context) error {
	listID := ctx.Param("id")
	if err := common.ValidateRequiredParam("listID", listID); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	var req models.UpdateListRequest
	if err := ctx.ParseRequest(&req); err != nil {
		if ctx.Request != nil && ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
			if err := common.ParseRequestBody(ctx.Request.Body, &req); err != nil {
				return common.RespondBadRequest(ctx, "invalid request body")
			}
		} else {
			return common.RespondBadRequest(ctx, "invalid request body")
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
		return common.RespondInternalServerError(ctx, "failed to update list")
	}

	return ctx.JSON(apiListFromStorage(result.List))
}

// HandleDeleteListLift handles DELETE /api/v1/lists/:id
func (h *Handler) HandleDeleteListLift(ctx *lift.Context) error {
	listID := ctx.Param("id")
	if err := common.ValidateRequiredParam("listID", listID); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
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
		return common.RespondInternalServerError(ctx, "failed to delete list")
	}

	return ctx.Status(http.StatusOK).JSON(models.EmptyObject{})
}

// HandleGetListAccountsLift handles GET /api/v1/lists/:id/accounts
func (h *Handler) HandleGetListAccountsLift(ctx *lift.Context) error {
	listID := ctx.Param("id")
	if err := common.ValidateRequiredParam("listID", listID); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
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
		return common.RespondNotFound(ctx, "list not found")
	}

	// Get accounts in the list using service
	membersResult, err := h.registry.Lists().GetListMembers(ctx.Context, &lists.GetListMembersQuery{
		ListID:     listID,
		ViewerID:   claims.Username,
		Pagination: interfaces.PaginationOptions{Limit: 100},
	})
	if err != nil {
		h.logger.Error("failed to get list accounts", zap.String("list_id", listID), zap.Error(err))
		return common.RespondInternalServerError(ctx, "failed to get list accounts")
	}

	// Convert members to Account objects
	accounts := make([]*models.Account, 0, len(membersResult.Members))
	for _, member := range membersResult.Members {
		// Get the actor from the storage account
		if member.Actor != nil {
			account := transformations.ActorToAccountBase(member.Actor, h.cfg.BaseURL())
			accounts = append(accounts, &account)
		} else {
			h.logger.Warn("list member has no actor data", zap.String("username", member.User.Username))
		}
	}

	return ctx.JSON(accounts)
}

// parseAccountIDsRequestWithAuth parses account IDs request and validates authentication
func (h *Handler) parseAccountIDsRequestWithAuth(ctx *lift.Context, requestType string) (string, []string, *auth.Claims, error) {
	listID := ctx.Param("id")
	if err := common.ValidateRequiredParam("listID", listID); err != nil {
		return "", nil, nil, common.RespondBadRequest(ctx, err.Error())
	}

	// Parse request body based on request type
	var accountIDs []string
	if requestType == "add" {
		var req models.AddAccountsRequest
		if err := ctx.ParseRequest(&req); err != nil {
			if ctx.Request != nil && ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
				if err := common.ParseRequestBody(ctx.Request.Body, &req); err != nil {
					return "", nil, nil, common.RespondBadRequest(ctx, "invalid request body")
				}
			} else {
				return "", nil, nil, common.RespondBadRequest(ctx, "invalid request body")
			}
		}
		accountIDs = req.AccountIDs
	} else {
		var req models.RemoveAccountsRequest
		if err := ctx.ParseRequest(&req); err != nil {
			if ctx.Request != nil && ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
				if err := common.ParseRequestBody(ctx.Request.Body, &req); err != nil {
					return "", nil, nil, common.RespondBadRequest(ctx, "invalid request body")
				}
			} else {
				return "", nil, nil, common.RespondBadRequest(ctx, "invalid request body")
			}
		}
		accountIDs = req.AccountIDs
	}

	if err := common.ValidateSliceNotEmpty("req.AccountIDs", accountIDs); err != nil {
		return "", nil, nil, common.RespondBadRequest(ctx, "account_ids is required")
	}

	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		return "", nil, nil, err
	}

	return listID, accountIDs, claims, nil
}

// HandleAddAccountsToListLift handles POST /api/v1/lists/:id/accounts
func (h *Handler) HandleAddAccountsToListLift(ctx *lift.Context) error {
	listID, accountIDs, claims, err := h.parseAccountIDsRequestWithAuth(ctx, "add")
	if err != nil {
		return err
	}

	// Add accounts to list via service (iterating for API compatibility)
	for _, accountID := range accountIDs {
		_, err := h.registry.Lists().AddToList(ctx.Context, &lists.AddToListCommand{
			ListID:         listID,
			MemberUsername: accountID,
			AdderID:        claims.Username,
		})
		if err != nil {
			h.logger.Error("failed to add account to list", zap.String("account_id", accountID), zap.Error(err))
			return common.RespondInternalServerError(ctx, "failed to add accounts to list")
		}
	}

	return ctx.Status(http.StatusOK).JSON(models.EmptyObject{})
}

// HandleRemoveAccountsFromListLift handles DELETE /api/v1/lists/:id/accounts
func (h *Handler) HandleRemoveAccountsFromListLift(ctx *lift.Context) error {
	listID, accountIDs, claims, err := h.parseAccountIDsRequestWithAuth(ctx, "remove")
	if err != nil {
		return err
	}

	// Remove accounts from list via service (iterating for API compatibility)
	for _, accountID := range accountIDs {
		_, err := h.registry.Lists().RemoveFromList(ctx.Context, &lists.RemoveFromListCommand{
			ListID:         listID,
			MemberUsername: accountID,
			RemoverID:      claims.Username,
		})
		if err != nil {
			h.logger.Error("failed to remove account from list", zap.String("account_id", accountID), zap.Error(err))
			return common.RespondInternalServerError(ctx, "failed to remove accounts from list")
		}
	}

	return ctx.Status(http.StatusOK).JSON(models.EmptyObject{})
}
