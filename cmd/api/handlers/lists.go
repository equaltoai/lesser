package handlers

import (
	"context"
	"errors"
	"fmt"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// HandleGetLists handles GET /api/v1/lists
func (h *Handler) HandleGetLists(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token
	token, err := auth.ExtractBearerToken(request.Headers["Authorization"])
	if err != nil {
		token, err = auth.ExtractBearerToken(request.Headers["authorization"])
		if err != nil {
			return common.Unauthorized(err), nil
		}
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check read scope
	if !claims.HasScope(auth.ScopeRead) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Get user's lists
	lists, err := h.store.GetListsForUser(ctx, claims.Username)
	if err != nil {
		h.logger.Error("failed to get user lists",
			zap.String("username", claims.Username),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to get lists")), nil
	}

	// Convert to API format
	apiLists := make([]*models.List, len(lists))
	for i, list := range lists {
		apiLists[i] = &models.List{
			ID:            list.ID,
			Title:         list.Title,
			RepliesPolicy: list.RepliesPolicy,
		}
	}

	return common.OK(apiLists), nil
}

// HandleCreateList handles POST /api/v1/lists
func (h *Handler) HandleCreateList(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token
	token, err := auth.ExtractBearerToken(request.Headers["Authorization"])
	if err != nil {
		token, err = auth.ExtractBearerToken(request.Headers["authorization"])
		if err != nil {
			return common.Unauthorized(err), nil
		}
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Parse request
	var req models.CreateListRequest
	if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
		return common.BadRequest(fmt.Errorf("invalid request body: %w", err)), nil
	}

	// Validate request
	if req.Title == "" {
		return common.BadRequest(errors.New("title is required")), nil
	}

	// Set default replies policy if not specified
	if req.RepliesPolicy == "" {
		req.RepliesPolicy = "list"
	}

	// Create the list
	list, err := h.store.CreateList(ctx, claims.Username, req.Title, req.RepliesPolicy)
	if err != nil {
		h.logger.Error("failed to create list",
			zap.String("username", claims.Username),
			zap.String("title", req.Title),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to create list")), nil
	}

	// Convert to API format
	apiList := &models.List{
		ID:            list.ID,
		Title:         list.Title,
		RepliesPolicy: list.RepliesPolicy,
	}

	return common.OK(apiList), nil
}

// HandleGetList handles GET /api/v1/lists/:id
func (h *Handler) HandleGetList(ctx context.Context, request events.APIGatewayV2HTTPRequest, listID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token
	token, err := auth.ExtractBearerToken(request.Headers["Authorization"])
	if err != nil {
		token, err = auth.ExtractBearerToken(request.Headers["authorization"])
		if err != nil {
			return common.Unauthorized(err), nil
		}
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check read scope
	if !claims.HasScope(auth.ScopeRead) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Get the list
	list, err := h.store.GetList(ctx, listID)
	if err != nil {
		return common.NotFound(fmt.Errorf("list not found")), nil
	}

	// Verify ownership
	if list.Username != claims.Username {
		return common.NotFound(fmt.Errorf("list not found")), nil
	}

	// Convert to API format
	apiList := &models.List{
		ID:            list.ID,
		Title:         list.Title,
		RepliesPolicy: list.RepliesPolicy,
	}

	return common.OK(apiList), nil
}

// HandleUpdateList handles PUT /api/v1/lists/:id
func (h *Handler) HandleUpdateList(ctx context.Context, request events.APIGatewayV2HTTPRequest, listID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token
	token, err := auth.ExtractBearerToken(request.Headers["Authorization"])
	if err != nil {
		token, err = auth.ExtractBearerToken(request.Headers["authorization"])
		if err != nil {
			return common.Unauthorized(err), nil
		}
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Get the list to verify ownership
	list, err := h.store.GetList(ctx, listID)
	if err != nil {
		return common.NotFound(fmt.Errorf("list not found")), nil
	}

	// Verify ownership
	if list.Username != claims.Username {
		return common.NotFound(fmt.Errorf("list not found")), nil
	}

	// Parse request
	var req models.UpdateListRequest
	if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
		return common.BadRequest(fmt.Errorf("invalid request body: %w", err)), nil
	}

	// Build updates
	updates := make(map[string]any)
	if req.Title != "" {
		updates["title"] = req.Title
	}
	if req.RepliesPolicy != "" {
		updates["replies_policy"] = req.RepliesPolicy
	}

	// Update the list
	if err := h.store.UpdateList(ctx, listID, updates); err != nil {
		h.logger.Error("failed to update list",
			zap.String("list_id", listID),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to update list")), nil
	}

	// Get updated list
	updatedList, err := h.store.GetList(ctx, listID)
	if err != nil {
		return common.InternalServerError(fmt.Errorf("failed to get updated list")), nil
	}

	// Convert to API format
	apiList := &models.List{
		ID:            updatedList.ID,
		Title:         updatedList.Title,
		RepliesPolicy: updatedList.RepliesPolicy,
	}

	return common.OK(apiList), nil
}

// HandleDeleteList handles DELETE /api/v1/lists/:id
func (h *Handler) HandleDeleteList(ctx context.Context, request events.APIGatewayV2HTTPRequest, listID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token
	token, err := auth.ExtractBearerToken(request.Headers["Authorization"])
	if err != nil {
		token, err = auth.ExtractBearerToken(request.Headers["authorization"])
		if err != nil {
			return common.Unauthorized(err), nil
		}
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Get the list to verify ownership
	list, err := h.store.GetList(ctx, listID)
	if err != nil {
		return common.NotFound(fmt.Errorf("list not found")), nil
	}

	// Verify ownership
	if list.Username != claims.Username {
		return common.NotFound(fmt.Errorf("list not found")), nil
	}

	// Delete the list
	if err := h.store.DeleteList(ctx, listID); err != nil {
		h.logger.Error("failed to delete list",
			zap.String("list_id", listID),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to delete list")), nil
	}

	return common.NoContent(), nil
}

// HandleGetListAccounts handles GET /api/v1/lists/:id/accounts
func (h *Handler) HandleGetListAccounts(ctx context.Context, request events.APIGatewayV2HTTPRequest, listID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token
	token, err := auth.ExtractBearerToken(request.Headers["Authorization"])
	if err != nil {
		token, err = auth.ExtractBearerToken(request.Headers["authorization"])
		if err != nil {
			return common.Unauthorized(err), nil
		}
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check read scope
	if !claims.HasScope(auth.ScopeRead) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Get the list to verify ownership
	list, err := h.store.GetList(ctx, listID)
	if err != nil {
		return common.NotFound(fmt.Errorf("list not found")), nil
	}

	// Verify ownership
	if list.Username != claims.Username {
		return common.NotFound(fmt.Errorf("list not found")), nil
	}

	// Get accounts in the list
	accountIDs, err := h.store.GetListAccounts(ctx, listID)
	if err != nil {
		h.logger.Error("failed to get list accounts",
			zap.String("list_id", listID),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to get list accounts")), nil
	}

	// Convert account IDs to Account objects
	accounts := make([]*models.Account, 0, len(accountIDs))
	for _, accountID := range accountIDs {
		// Get actor
		actor, err := h.store.GetActor(ctx, accountID)
		if err != nil {
			h.logger.Warn("failed to get actor for list account",
				zap.String("account_id", accountID),
				zap.Error(err))
			continue
		}

		// Convert to Account
		account := h.converter.ActorToAccount(actor)
		accounts = append(accounts, &account)
	}

	return common.OK(accounts), nil
}

// HandleAddAccountsToList handles POST /api/v1/lists/:id/accounts
func (h *Handler) HandleAddAccountsToList(ctx context.Context, request events.APIGatewayV2HTTPRequest, listID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token
	token, err := auth.ExtractBearerToken(request.Headers["Authorization"])
	if err != nil {
		token, err = auth.ExtractBearerToken(request.Headers["authorization"])
		if err != nil {
			return common.Unauthorized(err), nil
		}
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Get the list to verify ownership
	list, err := h.store.GetList(ctx, listID)
	if err != nil {
		return common.NotFound(fmt.Errorf("list not found")), nil
	}

	// Verify ownership
	if list.Username != claims.Username {
		return common.NotFound(fmt.Errorf("list not found")), nil
	}

	// Parse request
	var req models.AddAccountsRequest
	if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
		return common.BadRequest(fmt.Errorf("invalid request body: %w", err)), nil
	}

	// Validate request
	if len(req.AccountIDs) == 0 {
		return common.BadRequest(errors.New("account_ids is required")), nil
	}

	// Add accounts to list
	if err := h.store.AddAccountsToList(ctx, listID, req.AccountIDs); err != nil {
		h.logger.Error("failed to add accounts to list",
			zap.String("list_id", listID),
			zap.Any("account_ids", req.AccountIDs),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to add accounts to list")), nil
	}

	return common.NoContent(), nil
}

// HandleRemoveAccountsFromList handles DELETE /api/v1/lists/:id/accounts
func (h *Handler) HandleRemoveAccountsFromList(ctx context.Context, request events.APIGatewayV2HTTPRequest, listID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token
	token, err := auth.ExtractBearerToken(request.Headers["Authorization"])
	if err != nil {
		token, err = auth.ExtractBearerToken(request.Headers["authorization"])
		if err != nil {
			return common.Unauthorized(err), nil
		}
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Get the list to verify ownership
	list, err := h.store.GetList(ctx, listID)
	if err != nil {
		return common.NotFound(fmt.Errorf("list not found")), nil
	}

	// Verify ownership
	if list.Username != claims.Username {
		return common.NotFound(fmt.Errorf("list not found")), nil
	}

	// Parse request
	var req models.RemoveAccountsRequest
	if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
		return common.BadRequest(fmt.Errorf("invalid request body: %w", err)), nil
	}

	// Validate request
	if len(req.AccountIDs) == 0 {
		return common.BadRequest(errors.New("account_ids is required")), nil
	}

	// Remove accounts from list
	if err := h.store.RemoveAccountsFromList(ctx, listID, req.AccountIDs); err != nil {
		h.logger.Error("failed to remove accounts from list",
			zap.String("list_id", listID),
			zap.Any("account_ids", req.AccountIDs),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to remove accounts from list")), nil
	}

	return common.NoContent(), nil
}

// HandleGetAccountLists handles GET /api/v1/accounts/:id/lists
// Returns lists that the specified account is a member of (only lists owned by the requesting user)
func (h *Handler) HandleGetAccountLists(ctx context.Context, request events.APIGatewayV2HTTPRequest, accountID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token
	token, err := auth.ExtractBearerToken(request.Headers["Authorization"])
	if err != nil {
		token, err = auth.ExtractBearerToken(request.Headers["authorization"])
		if err != nil {
			return common.Unauthorized(err), nil
		}
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check read scope
	if !claims.HasScope(auth.ScopeRead) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Verify the account exists
	_, err = h.store.GetActor(ctx, accountID)
	if err != nil {
		return common.NotFound(fmt.Errorf("account not found")), nil
	}

	// Get lists containing this account that are owned by the requesting user
	lists, err := h.store.GetListsContainingAccount(ctx, accountID, claims.Username)
	if err != nil {
		h.logger.Error("failed to get lists containing account",
			zap.String("account_id", accountID),
			zap.String("username", claims.Username),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to get lists")), nil
	}

	// Convert to API format
	apiLists := make([]*models.List, len(lists))
	for i, list := range lists {
		apiLists[i] = &models.List{
			ID:            list.ID,
			Title:         list.Title,
			RepliesPolicy: list.RepliesPolicy,
		}
	}

	return common.OK(apiLists), nil
}
