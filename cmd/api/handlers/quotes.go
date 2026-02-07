package handlers

import (
	"fmt"
	"strings"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	pkgErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

// HandleCreateQuotePostLift handles POST /api/v1/statuses/:id/quote
// Creates a quote post of an existing status
func (h *Handler) HandleCreateQuotePostLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	statusID := ctx.Param("id")
	if err := common.ValidateStatusParamID(statusID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Authenticate user
	username, err := h.authenticateUser(ctx, []string{"write:statuses", auth.ScopeWrite})
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	// Parse request body
	var params apimodels.CreateQuotePostRequest

	if err := common.ParseRequestWithFallback(ctx, &params); err != nil {
		return common.RespondBadRequest(ctx, "invalid request body")
	}

	// Validate quote content
	if params.Status != "" {
		if err := common.ValidateStatusContent(params.Status); err != nil {
			return common.RespondValidationError(ctx, err)
		}
	}

	// Check if original status exists and is quotable
	originalStatus, err := h.repos.Status().GetStatus(ctx.Context(), statusID)
	if err != nil {
		h.logger.Error("failed to get original status", zap.String("status_id", statusID), zap.Error(err))
		return common.RespondStatusNotFound(ctx)
	}

	if originalStatus == nil {
		return common.RespondStatusNotFound(ctx)
	}

	// Check quote permissions
	canQuote, err := h.checkQuotePermissions(ctx, username, originalStatus)
	if err != nil {
		h.logger.Error("failed to check quote permissions", zap.Error(err))
		return common.RespondInternalServerError(ctx, "failed to check permissions")
	}

	if !canQuote {
		return common.RespondNotAuthorized(ctx, "quote this status")
	}

	// Create the quote post
	quotePost, err := h.createQuotePost(ctx, username, originalStatus, &params)
	if err != nil {
		h.logger.Error("failed to create quote post", zap.Error(err))
		return common.RespondFailedToCreate(ctx, "quote post")
	}

	// Convert to API format
	apiStatus := h.convertStatusToAPI(ctx, quotePost)

	return okJSON(apiStatus)
}

// HandleGetQuotesOfStatusLift handles GET /api/v1/statuses/:id/quotes
// Returns a list of quote posts for a given status
func (h *Handler) HandleGetQuotesOfStatusLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	statusID := ctx.Param("id")
	if err := common.ValidateStatusParamID(statusID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Parse pagination parameters
	limit, err := common.ParseFollowLimit(queryValue(ctx, "limit"))
	if err != nil {
		return common.RespondValidationError(ctx, err)
	}

	offset, err := common.ParseAndValidateIntWithBounds("offset", queryValue(ctx, "offset"), -1, 10000, 0)
	if err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Get quote relationships for the status
	quotes, err := h.getQuotesForStatus(ctx, statusID, limit, offset)
	if err != nil {
		h.logger.Error("failed to get quotes", zap.String("status_id", statusID), zap.Error(err))
		return common.RespondFailedToGet(ctx, "quotes")
	}

	// Convert to API format
	apiQuotes := make([]apimodels.QuoteStatusSummary, 0, len(quotes))
	for _, quote := range quotes {
		apiQuote := h.convertQuoteToAPI(ctx, quote)
		apiQuotes = append(apiQuotes, apiQuote)
	}

	return okJSON(apiQuotes)
}

// HandleDeleteQuotePostLift handles DELETE /api/v1/statuses/:id/quote/:quote_id
// Deletes a quote post (removes the quote relationship)
func (h *Handler) HandleDeleteQuotePostLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	statusID := ctx.Param("id")
	quoteID := ctx.Param("quote_id")
	if err := common.ValidateStatusParamID(statusID); err != nil {
		return common.RespondValidationError(ctx, err)
	}
	if err := common.ValidateRequiredParam("quote_id", quoteID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Authenticate user
	username, err := h.authenticateUser(ctx, []string{"write:statuses", auth.ScopeWrite})
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	// Verify user owns the quote
	quote, err := h.getQuoteRelationship(ctx, quoteID, statusID)
	if err != nil {
		h.logger.Error("failed to get quote relationship", zap.Error(err))
		return common.RespondFailedToGet(ctx, "quote")
	}

	if quote == nil {
		return common.RespondNotFound(ctx, "quote")
	}

	if quote.QuoterID != username {
		return common.RespondNotAuthorizedToDelete(ctx, "quote")
	}

	// Delete the quote relationship
	err = h.deleteQuoteRelationship(ctx, quote)
	if err != nil {
		h.logger.Error("failed to delete quote relationship", zap.Error(err))
		return common.RespondFailedToDelete(ctx, "quote")
	}

	return okJSON(apimodels.MessageResponse{Message: "quote deleted"})
}

// HandleGetQuotePermissionsLift handles GET /api/v1/accounts/:id/quote_permissions
// Returns quote permissions for a user
func (h *Handler) HandleGetQuotePermissionsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	accountID := ctx.Param("id")
	if err := common.ValidateRequiredParam("account_id", accountID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Get quote permissions
	permissions, err := h.getQuotePermissions(ctx, accountID)
	if err != nil {
		h.logger.Error("failed to get quote permissions", zap.String("account_id", accountID), zap.Error(err))
		return common.RespondFailedToGet(ctx, "permissions")
	}

	return okJSON(apimodels.QuotePermissionsResponse{
		AllowPublic:    permissions.AllowPublic,
		AllowFollowers: permissions.AllowFollowers,
		AllowMentioned: permissions.AllowMentioned,
		BlockList:      permissions.BlockList,
	})
}

// HandleUpdateQuotePermissionsLift handles PUT /api/v1/accounts/quote_permissions
// Updates quote permissions for the authenticated user
func (h *Handler) HandleUpdateQuotePermissionsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Authenticate user
	username, err := h.authenticateUser(ctx, []string{"write:accounts", auth.ScopeWrite})
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	// Parse request body
	var params apimodels.UpdateQuotePermissionsRequest

	if err := common.ParseRequestWithFallback(ctx, &params); err != nil {
		return common.RespondBadRequest(ctx, "invalid request body")
	}

	// Get existing permissions or create new ones
	permissions, err := h.getQuotePermissions(ctx, username)
	if err != nil {
		// Create default permissions if none exist
		permissions = &storageModels.QuotePermissions{
			Username: username,
		}
		permissions.SetDefaults()
	}

	// Update permissions
	if params.AllowPublic != nil {
		permissions.AllowPublic = *params.AllowPublic
	}
	if params.AllowFollowers != nil {
		permissions.AllowFollowers = *params.AllowFollowers
	}
	if params.AllowMentioned != nil {
		permissions.AllowMentioned = *params.AllowMentioned
	}
	if params.BlockList != nil {
		permissions.BlockList = params.BlockList
	}

	// Save updated permissions
	err = h.saveQuotePermissions(ctx, permissions)
	if err != nil {
		h.logger.Error("failed to save quote permissions", zap.Error(err))
		return common.RespondFailedToUpdate(ctx, "permissions")
	}

	return okJSON(apimodels.QuotePermissionsResponse{
		AllowPublic:    permissions.AllowPublic,
		AllowFollowers: permissions.AllowFollowers,
		AllowMentioned: permissions.AllowMentioned,
		BlockList:      permissions.BlockList,
	})
}

// Helper methods

func (h *Handler) checkQuotePermissions(_ *apptheory.Context, _ string, _ interface{}) (bool, error) {
	// For now, return a simple implementation
	// In a full implementation, this would:
	// 1. Get the original status author
	// 2. Get their quote permissions
	// 3. Check if quoter is allowed based on relationship and permissions
	return true, nil
}

func (h *Handler) createQuotePost(_ *apptheory.Context, username string, _ interface{}, _ interface{}) (interface{}, error) {
	// Placeholder implementation
	// In a full implementation, this would:
	// 1. Create the new status with quote content
	// 2. Create the quote relationship
	// 3. Update counters and notifications
	// 4. Handle federation

	now := time.Now()
	quotePost := map[string]interface{}{
		"id":         fmt.Sprintf("quote_%d", now.Unix()),
		"created_at": now.Format(time.RFC3339),
		"account":    map[string]interface{}{"id": username, "username": username},
		"content":    "Quote post content", // Would be from params
		"quoted_status": map[string]interface{}{
			"id": "original_status_id",
			// Original status data would go here
		},
	}

	return quotePost, nil
}

func (h *Handler) getQuotesForStatus(ctx *apptheory.Context, statusID string, limit int, _ int) ([]interface{}, error) {
	if limit <= 0 {
		limit = 20
	}

	result, err := h.repos.Quote().GetQuotesForStatus(ctx.Context(), statusID, interfaces.PaginationOptions{Limit: limit})
	if err != nil {
		return nil, err
	}

	out := make([]interface{}, 0, len(result.Items))
	for _, rel := range result.Items {
		out = append(out, rel)
	}

	return out, nil
}

func (h *Handler) convertQuoteToAPI(_ *apptheory.Context, _ interface{}) apimodels.QuoteStatusSummary {
	// Placeholder implementation
	return apimodels.QuoteStatusSummary{
		ID:        "quote_id",
		CreatedAt: time.Now().Format(time.RFC3339),
		Account:   apimodels.QuoteStatusAccount{ID: "user_id"},
		Content:   "Quote content",
	}
}

func (h *Handler) convertStatusToAPI(_ *apptheory.Context, _ interface{}) apimodels.QuoteStatusSummary {
	// Placeholder implementation
	return apimodels.QuoteStatusSummary{
		ID:        "status_id",
		CreatedAt: time.Now().Format(time.RFC3339),
		Account:   apimodels.QuoteStatusAccount{ID: "user_id"},
		Content:   "Status content",
	}
}

func (h *Handler) getQuoteRelationship(ctx *apptheory.Context, quoteStatusID string, targetStatusID string) (*storageModels.QuoteRelationship, error) {
	relationship, err := h.repos.Quote().GetQuoteRelationship(ctx.Context(), quoteStatusID, targetStatusID)
	if err != nil {
		if pkgErrors.HasCode(err, pkgErrors.CodeNotFound) || strings.Contains(strings.ToLower(err.Error()), "not found") {
			return nil, nil
		}
		return nil, err
	}
	return relationship, nil
}

func (h *Handler) deleteQuoteRelationship(ctx *apptheory.Context, relationship *storageModels.QuoteRelationship) error {
	if relationship == nil {
		return nil
	}
	return h.repos.Quote().DeleteQuoteRelationship(ctx.Context(), relationship.QuoterNoteID, relationship.TargetNoteID)
}

func (h *Handler) getQuotePermissions(_ *apptheory.Context, username string) (*storageModels.QuotePermissions, error) {
	// Placeholder implementation - would query from storage
	permissions := &storageModels.QuotePermissions{
		Username: username,
	}
	permissions.SetDefaults()
	return permissions, nil
}

func (h *Handler) saveQuotePermissions(_ *apptheory.Context, _ *storageModels.QuotePermissions) error {
	// Placeholder implementation - would save to storage
	return nil
}
