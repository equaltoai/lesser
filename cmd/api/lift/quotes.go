package lift

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleCreateQuotePostLift handles POST /api/v1/statuses/:id/quote
// Creates a quote post of an existing status
func (h *Handler) HandleCreateQuotePostLift(ctx *lift.Context) error {
	statusID := ctx.Param("id")
	if err := common.ValidateStatusParamID(statusID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Authenticate user
	claims, err := h.authenticateQuoteRequest(ctx)
	if err != nil {
		return err
	}

	// Check write scope
	if !claims.HasScope("write:statuses") {
		return common.RespondInsufficientScope(ctx)
	}

	// Parse request body
	var params struct {
		Status      string `json:"status"`
		Visibility  string `json:"visibility"`
		SpoilerText string `json:"spoiler_text"`
		Sensitive   bool   `json:"sensitive"`
		Language    string `json:"language"`
	}

	if err := ctx.ParseRequest(&params); err != nil {
		bodyBytes := ctx.Request.Body
		if err := common.ValidateSliceNotEmpty("bodyBytes", bodyBytes); err != nil && ctx.Request != nil && ctx.Request.Request != nil {
			bodyBytes = ctx.Request.Request.Body
		}
		if err2 := common.ParseRequestBody(bodyBytes, &params); err2 != nil {
			return common.RespondValidationError(ctx, err)
		}
	}

	// Validate quote content
	if params.Status != "" {
		if err := common.ValidateStatusContent(params.Status); err != nil {
			return common.RespondValidationError(ctx, err)
		}
	}

	// Check if original status exists and is quotable
	originalStatus, err := h.repos.Status().GetStatus(ctx.Context, statusID)
	if err != nil {
		h.logger.Error("failed to get original status", zap.String("status_id", statusID), zap.Error(err))
		return common.RespondStatusNotFound(ctx)
	}

	if originalStatus == nil {
		return common.RespondStatusNotFound(ctx)
	}

	// Check quote permissions
	canQuote, err := h.checkQuotePermissions(ctx, claims.Username, originalStatus)
	if err != nil {
		h.logger.Error("failed to check quote permissions", zap.Error(err))
		return common.RespondInternalServerError(ctx, "failed to check permissions")
	}

	if !canQuote {
		return common.RespondNotAuthorized(ctx, "quote this status")
	}

	// Create the quote post
	quotePost, err := h.createQuotePost(ctx, claims.Username, originalStatus, &params)
	if err != nil {
		h.logger.Error("failed to create quote post", zap.Error(err))
		return common.RespondFailedToCreate(ctx, "quote post")
	}

	// Convert to API format
	apiStatus := h.convertStatusToAPI(ctx, quotePost)

	return ctx.Status(200).JSON(apiStatus)
}

// HandleGetQuotesOfStatusLift handles GET /api/v1/statuses/:id/quotes
// Returns a list of quote posts for a given status
func (h *Handler) HandleGetQuotesOfStatusLift(ctx *lift.Context) error {
	statusID := ctx.Param("id")
	if err := common.ValidateStatusParamID(statusID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Parse pagination parameters
	limit, err := common.ParseFollowLimit(ctx.Query("limit"))
	if err != nil {
		return common.RespondValidationError(ctx, err)
	}

	offset, err := common.ParseAndValidateIntWithBounds("offset", ctx.Query("offset"), -1, 10000, 0)
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
	apiQuotes := make([]map[string]interface{}, 0, len(quotes))
	for _, quote := range quotes {
		apiQuote := h.convertQuoteToAPI(ctx, quote)
		apiQuotes = append(apiQuotes, apiQuote)
	}

	return ctx.Status(200).JSON(apiQuotes)
}

// HandleDeleteQuotePostLift handles DELETE /api/v1/statuses/:id/quote/:quote_id
// Deletes a quote post (removes the quote relationship)
func (h *Handler) HandleDeleteQuotePostLift(ctx *lift.Context) error {
	statusID := ctx.Param("id")
	quoteID := ctx.Param("quote_id")
	if err := common.ValidateStatusParamID(statusID); err != nil {
		return common.RespondValidationError(ctx, err)
	}
	if err := common.ValidateRequiredParam("quote_id", quoteID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Authenticate user
	claims, err := h.authenticateQuoteRequest(ctx)
	if err != nil {
		return err
	}

	// Check write scope
	if !claims.HasScope("write:statuses") {
		return common.RespondInsufficientScope(ctx)
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

	if quote.QuoterID != claims.Username {
		return common.RespondNotAuthorizedToDelete(ctx, "quote")
	}

	// Delete the quote relationship
	err = h.deleteQuoteRelationship(ctx, quote)
	if err != nil {
		h.logger.Error("failed to delete quote relationship", zap.Error(err))
		return common.RespondFailedToDelete(ctx, "quote")
	}

	return ctx.Status(200).JSON(map[string]string{"message": "quote deleted"})
}

// HandleGetQuotePermissionsLift handles GET /api/v1/accounts/:id/quote_permissions
// Returns quote permissions for a user
func (h *Handler) HandleGetQuotePermissionsLift(ctx *lift.Context) error {
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

	// Convert to API format
	apiPermissions := map[string]interface{}{
		"allow_public":    permissions.AllowPublic,
		"allow_followers": permissions.AllowFollowers,
		"allow_mentioned": permissions.AllowMentioned,
		"block_list":      permissions.BlockList,
	}

	return ctx.Status(200).JSON(apiPermissions)
}

// HandleUpdateQuotePermissionsLift handles PUT /api/v1/accounts/quote_permissions
// Updates quote permissions for the authenticated user
func (h *Handler) HandleUpdateQuotePermissionsLift(ctx *lift.Context) error {
	// Authenticate user
	claims, err := h.authenticateQuoteRequest(ctx)
	if err != nil {
		return err
	}

	// Check write scope
	if !claims.HasScope("write:accounts") {
		return common.RespondInsufficientScope(ctx)
	}

	// Parse request body
	var params struct {
		AllowPublic    *bool    `json:"allow_public"`
		AllowFollowers *bool    `json:"allow_followers"`
		AllowMentioned *bool    `json:"allow_mentioned"`
		BlockList      []string `json:"block_list"`
	}

	if err := ctx.ParseRequest(&params); err != nil {
		bodyBytes := ctx.Request.Body
		if err := common.ValidateSliceNotEmpty("bodyBytes", bodyBytes); err != nil && ctx.Request != nil && ctx.Request.Request != nil {
			bodyBytes = ctx.Request.Request.Body
		}
		if err2 := common.ParseRequestBody(bodyBytes, &params); err2 != nil {
			return common.RespondValidationError(ctx, err)
		}
	}

	// Get existing permissions or create new ones
	permissions, err := h.getQuotePermissions(ctx, claims.Username)
	if err != nil {
		// Create default permissions if none exist
		permissions = &models.QuotePermissions{
			Username: claims.Username,
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

	// Return updated permissions
	apiPermissions := map[string]interface{}{
		"allow_public":    permissions.AllowPublic,
		"allow_followers": permissions.AllowFollowers,
		"allow_mentioned": permissions.AllowMentioned,
		"block_list":      permissions.BlockList,
	}

	return ctx.Status(200).JSON(apiPermissions)
}

// Helper methods

func (h *Handler) authenticateQuoteRequest(ctx *lift.Context) (*auth.Claims, error) {
	// Extract and validate token
	authHeader := ctx.Header("Authorization")
	if err := common.ValidateRequiredParam("auth_header", authHeader); err != nil {
		authHeader = ctx.Header("authorization")
	}

	if err := common.ValidateRequiredParam("auth_header", authHeader); err != nil && ctx.Request != nil && ctx.Request.Request != nil {
		authHeader = ctx.Request.Request.Headers["Authorization"]
		if err := common.ValidateRequiredParam("auth_header", authHeader); err != nil {
			authHeader = ctx.Request.Request.Headers["authorization"]
		}
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return nil, common.RespondUnauthorized(ctx)
	}

	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return nil, common.RespondUnauthorized(ctx)
	}

	return claims, nil
}

func (h *Handler) checkQuotePermissions(_ *lift.Context, _ string, _ interface{}) (bool, error) {
	// For now, return a simple implementation
	// In a full implementation, this would:
	// 1. Get the original status author
	// 2. Get their quote permissions
	// 3. Check if quoter is allowed based on relationship and permissions
	return true, nil
}

func (h *Handler) createQuotePost(_ *lift.Context, username string, _ interface{}, _ interface{}) (interface{}, error) {
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

func (h *Handler) getQuotesForStatus(_ *lift.Context, _ string, _ int, _ int) ([]interface{}, error) {
	// Placeholder implementation
	// In a full implementation, this would query quote relationships
	return []interface{}{}, nil
}

func (h *Handler) convertQuoteToAPI(_ *lift.Context, _ interface{}) map[string]interface{} {
	// Placeholder implementation
	return map[string]interface{}{
		"id":         "quote_id",
		"created_at": time.Now().Format(time.RFC3339),
		"account":    map[string]interface{}{"id": "user_id"},
		"content":    "Quote content",
	}
}

func (h *Handler) convertStatusToAPI(_ *lift.Context, _ interface{}) map[string]interface{} {
	// Placeholder implementation
	return map[string]interface{}{
		"id":         "status_id",
		"created_at": time.Now().Format(time.RFC3339),
		"account":    map[string]interface{}{"id": "user_id"},
		"content":    "Status content",
	}
}

func (h *Handler) getQuoteRelationship(_ *lift.Context, _ string, _ string) (*models.QuoteRelationship, error) {
	// Placeholder implementation
	return nil, nil
}

func (h *Handler) deleteQuoteRelationship(_ *lift.Context, _ *models.QuoteRelationship) error {
	// Placeholder implementation
	return nil
}

func (h *Handler) getQuotePermissions(_ *lift.Context, username string) (*models.QuotePermissions, error) {
	// Placeholder implementation - would query from storage
	permissions := &models.QuotePermissions{
		Username: username,
	}
	permissions.SetDefaults()
	return permissions, nil
}

func (h *Handler) saveQuotePermissions(_ *lift.Context, _ *models.QuotePermissions) error {
	// Placeholder implementation - would save to storage
	return nil
}
