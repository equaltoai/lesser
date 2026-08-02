package handlers

import (
	"net/http"
	"strings"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	pkgErrors "github.com/equaltoai/lesser/pkg/errors"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
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
	_, err := h.authenticateUser(ctx, []string{"write:statuses", auth.ScopeWrite})
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

	// The routed extension is intentionally inert until quote authorization,
	// persistence, and federation are implemented. Return before looking up the
	// target so existent, private, and missing IDs remain indistinguishable.
	return apptheory.JSON(http.StatusNotImplemented, common.StandardErrorResponse{
		Error: "quote endpoint is not implemented",
	})
}

// HandleGetQuotesOfStatusLift handles GET /api/v1/statuses/:id/quotes
// Returns a list of quote posts for a given status
func (h *Handler) HandleGetQuotesOfStatusLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	statusID := ctx.Param("id")
	if err := common.ValidateStatusParamID(statusID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Parse pagination parameters
	if _, err := common.ParseFollowLimit(queryValue(ctx, "limit")); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	if _, err := common.ParseAndValidateIntWithBounds("offset", queryValue(ctx, "offset"), -1, 10000, 0); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Quote rows are not yet backed by real status projections. Return before
	// storage access so the response cannot disclose a target's existence or
	// the number of quote relationships associated with it.
	return apptheory.JSON(http.StatusNotImplemented, common.StandardErrorResponse{
		Error: "quotes endpoint is not implemented",
	})
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

	// Permission persistence is not implemented. Return before storage access so
	// existent and missing account IDs remain indistinguishable and clients are
	// never given fabricated all-permissive settings.
	return apptheory.JSON(http.StatusNotImplemented, common.StandardErrorResponse{
		Error: "quote permissions endpoint is not implemented",
	})
}

// HandleUpdateQuotePermissionsLift handles PUT /api/v1/accounts/quote_permissions
// Updates quote permissions for the authenticated user
func (h *Handler) HandleUpdateQuotePermissionsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Authenticate user
	_, err := h.authenticateUser(ctx, []string{"write:accounts", auth.ScopeWrite})
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

	// Persistence is not implemented. Refuse the update rather than echoing
	// request values as though they were saved.
	return apptheory.JSON(http.StatusNotImplemented, common.StandardErrorResponse{
		Error: "quote permission updates are not implemented",
	})
}

// Helper methods

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
