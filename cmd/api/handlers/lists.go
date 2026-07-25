package handlers

import (
	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/lists"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/transformations"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
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
func (h *Handler) HandleGetListsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	claims, err := h.authenticateWithScope(ctx, auth.ScopeRead)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	// Check if lists service is available
	if h.registry == nil || h.registry.Lists() == nil {
		return common.RespondServiceUnavailable(ctx, "service unavailable")
	}

	result, err := h.registry.Lists().ListUserLists(ctx.Context(), &lists.ListUserListsQuery{
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
	return okJSON(response)
}

// HandleCreateListLift handles POST /api/v1/lists
func (h *Handler) HandleCreateListLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	req, err := apptheory.BindRequest[models.CreateListRequest](ctx, apptheory.BindConfig[models.CreateListRequest]{
		Body: true,
	})
	if err != nil {
		return common.RespondBadRequest(ctx, "invalid request body")
	}

	// Validate list parameters
	params := map[string]interface{}{
		"title":          req.Title,
		"replies_policy": req.RepliesPolicy,
	}
	if err := common.ValidateListParams(params); err != nil {
		h.logger.Info("list validation failed", zap.Error(err))
		return common.RespondValidationError(ctx, err)
	}

	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	// Check if lists service is available
	if h.registry == nil || h.registry.Lists() == nil {
		return common.RespondServiceUnavailable(ctx, "service unavailable")
	}

	result, err := h.registry.Lists().CreateList(ctx.Context(), &lists.CreateListCommand{
		Username:      claims.Username,
		Title:         req.Title,
		RepliesPolicy: req.RepliesPolicy,
		CreatorID:     claims.Username,
	})
	if err != nil {
		h.logger.Error("failed to create list", zap.Error(err))
		return common.RespondFailedToCreate(ctx, "list")
	}

	return createdJSON(apiListFromStorage(result.List))
}

// HandleGetListLift handles GET /api/v1/lists/:id
func (h *Handler) HandleGetListLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	listID := ctx.Param("id")
	if err := common.ValidateRequiredParam("listID", listID); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	claims, err := h.authenticateWithScope(ctx, auth.ScopeRead)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	// Check if lists service is available
	if h.registry == nil || h.registry.Lists() == nil {
		return common.RespondServiceUnavailable(ctx, "service unavailable")
	}

	list, err := h.registry.Lists().GetList(ctx.Context(), &lists.GetListQuery{
		ListID:   listID,
		ViewerID: claims.Username,
	})
	if err != nil {
		return common.RespondNotFound(ctx, "list not found")
	}

	return okJSON(apiListFromStorage(list))
}

// HandleUpdateListLift handles PUT /api/v1/lists/:id
func (h *Handler) HandleUpdateListLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	listID := ctx.Param("id")
	if err := common.ValidateRequiredParam("listID", listID); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	req, err := apptheory.BindRequest[models.UpdateListRequest](ctx, apptheory.BindConfig[models.UpdateListRequest]{
		Body: true,
	})
	if err != nil {
		return common.RespondBadRequest(ctx, "invalid request body")
	}

	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	// Check if lists service is available
	if h.registry == nil || h.registry.Lists() == nil {
		return common.RespondServiceUnavailable(ctx, "service unavailable")
	}

	result, err := h.registry.Lists().UpdateList(ctx.Context(), &lists.UpdateListCommand{
		ListID:        listID,
		Title:         req.Title,
		RepliesPolicy: req.RepliesPolicy,
		UpdaterID:     claims.Username,
	})
	if err != nil {
		h.logger.Error("failed to update list", zap.Error(err))
		return common.RespondInternalServerError(ctx, "failed to update list")
	}

	return okJSON(apiListFromStorage(result.List))
}

// HandleDeleteListLift handles DELETE /api/v1/lists/:id
func (h *Handler) HandleDeleteListLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	listID := ctx.Param("id")
	if err := common.ValidateRequiredParam("listID", listID); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	// Check if lists service is available
	if h.registry == nil || h.registry.Lists() == nil {
		return common.RespondServiceUnavailable(ctx, "service unavailable")
	}

	if err := h.registry.Lists().DeleteList(ctx.Context(), &lists.DeleteListCommand{
		ListID:    listID,
		DeleterID: claims.Username,
	}); err != nil {
		h.logger.Error("failed to delete list", zap.Error(err))
		return common.RespondInternalServerError(ctx, "failed to delete list")
	}

	return okJSON(models.EmptyObject{})
}

// HandleGetListAccountsLift handles GET /api/v1/lists/:id/accounts
func (h *Handler) HandleGetListAccountsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	listID := ctx.Param("id")
	if err := common.ValidateRequiredParam("listID", listID); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	claims, err := h.authenticateWithScope(ctx, auth.ScopeRead)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	// Check if lists service is available
	if h.registry == nil || h.registry.Lists() == nil {
		return common.RespondServiceUnavailable(ctx, "service unavailable")
	}

	// Verify list ownership using service
	_, err = h.registry.Lists().GetList(ctx.Context(), &lists.GetListQuery{
		ListID:   listID,
		ViewerID: claims.Username,
	})
	if err != nil {
		return common.RespondNotFound(ctx, "list not found")
	}

	// Get accounts in the list using service
	membersResult, err := h.registry.Lists().GetListMembers(ctx.Context(), &lists.GetListMembersQuery{
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

	return okJSON(accounts)
}

// parseAccountIDsRequestWithAuth parses account IDs request and validates authentication
func (h *Handler) parseAccountIDsRequestWithAuth(ctx *apptheory.Context, requestType string) (string, []string, *auth.Claims, *apptheory.Response, error) {
	listID := ctx.Param("id")
	if err := common.ValidateRequiredParam("listID", listID); err != nil {
		resp, respErr := common.RespondBadRequest(ctx, err.Error())
		return "", nil, nil, resp, respErr
	}

	// Parse request body based on request type
	var accountIDs []string
	if requestType == "add" {
		var req models.AddAccountsRequest
		if err := common.ParseRequestWithFallback(ctx, &req); err != nil {
			resp, respErr := common.RespondBadRequest(ctx, "invalid request body")
			return "", nil, nil, resp, respErr
		}
		accountIDs = req.AccountIDs
	} else {
		var req models.RemoveAccountsRequest
		if err := common.ParseRequestWithFallback(ctx, &req); err != nil {
			resp, respErr := common.RespondBadRequest(ctx, "invalid request body")
			return "", nil, nil, resp, respErr
		}
		accountIDs = req.AccountIDs
	}

	if err := common.ValidateSliceNotEmpty("req.AccountIDs", accountIDs); err != nil {
		resp, respErr := common.RespondBadRequest(ctx, "account_ids is required")
		return "", nil, nil, resp, respErr
	}

	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		if isInsufficientScopeError(err) {
			resp, respErr := common.RespondForbidden(ctx, err.Error())
			return "", nil, nil, resp, respErr
		}
		resp, respErr := common.RespondUnauthorized(ctx)
		return "", nil, nil, resp, respErr
	}

	return listID, accountIDs, claims, nil, nil
}

// HandleAddAccountsToListLift handles POST /api/v1/lists/:id/accounts
func (h *Handler) HandleAddAccountsToListLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	listID, accountIDs, claims, resp, err := h.parseAccountIDsRequestWithAuth(ctx, "add")
	if resp != nil || err != nil {
		return resp, err
	}

	// Add accounts to list via service (iterating for API compatibility)
	for _, accountID := range accountIDs {
		_, err := h.registry.Lists().AddToList(ctx.Context(), &lists.AddToListCommand{
			ListID:         listID,
			MemberUsername: accountID,
			AdderID:        claims.Username,
		})
		if err != nil {
			h.logger.Error("failed to add account to list", zap.String("account_id", accountID), zap.Error(err))
			return common.RespondInternalServerError(ctx, "failed to add accounts to list")
		}
	}

	return okJSON(models.EmptyObject{})
}

// HandleRemoveAccountsFromListLift handles DELETE /api/v1/lists/:id/accounts
func (h *Handler) HandleRemoveAccountsFromListLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	listID, accountIDs, claims, resp, err := h.parseAccountIDsRequestWithAuth(ctx, "remove")
	if resp != nil || err != nil {
		return resp, err
	}

	// Remove accounts from list via service (iterating for API compatibility)
	for _, accountID := range accountIDs {
		_, err := h.registry.Lists().RemoveFromList(ctx.Context(), &lists.RemoveFromListCommand{
			ListID:         listID,
			MemberUsername: accountID,
			RemoverID:      claims.Username,
		})
		if err != nil {
			h.logger.Error("failed to remove account from list", zap.String("account_id", accountID), zap.Error(err))
			return common.RespondInternalServerError(ctx, "failed to remove accounts from list")
		}
	}

	return okJSON(models.EmptyObject{})
}
