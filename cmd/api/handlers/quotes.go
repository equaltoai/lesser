package handlers

import (
	"context"
	"strings"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	commonerrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/services/quotes"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	"go.uber.org/zap"
)

const (
	quoteListPageSize   = 80
	quoteListMaxScanned = 10080
)

// HandleCreateQuotePostLift handles POST /api/v1/statuses/:id/quote.
// It uses the same Notes.CreateNote -> QuoteService.AttachQuoteToStatus path as
// GraphQL quote creation, including the notes service's persistence, streaming,
// and federation fanout.
func (h *Handler) HandleCreateQuotePostLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	statusID := ctx.Param("id")
	if err := common.ValidateStatusParamID(statusID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	claims, err := h.authenticateWithAnyScope(ctx, "write:statuses", auth.ScopeWrite)
	if err != nil {
		return respondQuoteAuthError(ctx, err)
	}

	var params apimodels.CreateQuotePostRequest
	if err := common.ParseRequestWithFallback(ctx, &params); err != nil {
		return common.RespondBadRequest(ctx, "invalid request body")
	}

	statusRequest := quoteStatusRequest(params)
	if statusRequest.Visibility == "" {
		statusRequest.Visibility = storageModels.VisibilityPublic
	}
	if resp, err := validateCreateStatusRequest(ctx, &statusRequest); resp != nil || err != nil {
		return resp, err
	}

	if h.registry == nil || h.registry.Notes() == nil || h.registry.Quotes() == nil {
		h.logger.Error("quote services unavailable")
		return common.RespondInternalServerError(ctx)
	}

	notesService := h.registry.Notes()
	quoteTarget, err := notesService.ResolveQuoteTarget(ctx.Context(), claims.Username, statusID)
	if err != nil {
		return h.respondQuoteTargetError(ctx, statusID, claims.Username, err)
	}
	if quoteTarget == nil {
		return common.RespondStatusNotFound(ctx)
	}
	if err := notes.ValidateChildReach("quote", quoteTarget, statusRequest.Visibility); err != nil {
		return respondQuoteAppError(ctx, err)
	}

	quotesService := h.registry.Quotes()
	allowed, err := quotesService.CheckQuotePermissions(ctx.Context(), claims.Username, quoteTarget)
	if err != nil {
		h.logger.Error("failed to check quote permissions",
			zap.String("viewer", claims.Username),
			zap.String("target_status_id", quoteTarget.StatusID),
			zap.Error(err))
		return common.RespondWithAppError(ctx, quotes.ErrCheckQuotePermissions(err))
	}
	if !allowed {
		return common.RespondWithAppError(ctx, quotes.ErrNotAuthorizedToQuote)
	}

	agentAttribution, resp, err := h.prepareAgentStatusCreate(ctx, claims, &statusRequest)
	if resp != nil || err != nil {
		return resp, err
	}

	result, err := notesService.CreateNote(ctx.Context(), createNoteCommandFromStatusRequest(claims, &statusRequest, agentAttribution))
	if err != nil {
		h.logger.Error("failed to create quote status", zap.Error(err))
		return respondQuoteAppError(ctx, err)
	}
	if result == nil || result.Note == nil {
		h.logger.Error("notes service returned no quote status")
		return common.RespondInternalServerError(ctx)
	}

	if _, err := quotesService.AttachQuoteToStatus(ctx.Context(), result.Note, quoteTarget.StatusID); err != nil {
		h.logger.Error("failed to attach quote status",
			zap.String("quote_status_id", result.Note.StatusID),
			zap.String("target_status_id", quoteTarget.StatusID),
			zap.Error(err))
		return respondQuoteAppError(ctx, err)
	}

	return okJSON(quoteStatusSummary(result.Note))
}

// HandleGetQuotesOfStatusLift handles GET /api/v1/statuses/:id/quotes.
// The target and every returned quote are checked through Notes.GetNoteWithViewer.
func (h *Handler) HandleGetQuotesOfStatusLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	statusID := ctx.Param("id")
	if err := common.ValidateStatusParamID(statusID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	limit, err := common.ParseFollowLimit(queryValue(ctx, "limit"))
	if err != nil {
		return common.RespondValidationError(ctx, err)
	}
	offset, err := common.ParseAndValidateIntWithBounds("offset", queryValue(ctx, "offset"), -1, 10000, 0)
	if err != nil {
		return common.RespondValidationError(ctx, err)
	}

	if h.registry == nil || h.registry.Notes() == nil || h.registry.Quotes() == nil {
		h.logger.Error("quote services unavailable")
		return common.RespondInternalServerError(ctx)
	}

	viewer := h.getOptionalAuthenticatedUser(ctx)
	notesService := h.registry.Notes()
	target, err := notesService.GetNoteWithViewer(ctx.Context(), &notes.GetNoteQuery{StatusID: statusID, ViewerID: viewer})
	if err != nil {
		return h.respondVisibleQuoteStatusError(ctx, statusID, viewer, err)
	}
	if target == nil {
		return common.RespondStatusNotFound(ctx)
	}

	items, err := h.listVisibleQuoteSummaries(ctx.Context(), notesService, h.registry.Quotes(), statusID, viewer, limit, offset)
	if err != nil {
		return h.respondVisibleQuoteStatusError(ctx, statusID, viewer, err)
	}
	return okJSON(items)
}

// HandleDeleteQuotePostLift handles DELETE /api/v1/statuses/:id/quote/:quote_id.
func (h *Handler) HandleDeleteQuotePostLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	statusID := ctx.Param("id")
	quoteID := ctx.Param("quote_id")
	if err := common.ValidateStatusParamID(statusID); err != nil {
		return common.RespondValidationError(ctx, err)
	}
	if err := common.ValidateRequiredParam("quote_id", quoteID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	username, err := h.authenticateUser(ctx, []string{"write:statuses", auth.ScopeWrite})
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	quote, err := h.getQuoteRelationship(ctx, quoteID, statusID)
	if err != nil {
		h.logger.Error("failed to get quote relationship", zap.Error(err))
		return common.RespondFailedToGet(ctx, "quote")
	}
	if quote == nil {
		return common.RespondNotFound(ctx, "quote")
	}

	ownedByUser := quote.QuoterID == username
	if !ownedByUser && h.cfg != nil {
		ownedByUser = quote.QuoterID == common.GenerateActorID(h.cfg.Domain, username)
	}
	if !ownedByUser {
		return common.RespondNotFound(ctx, "quote")
	}

	if err := h.deleteQuoteRelationship(ctx, quote); err != nil {
		h.logger.Error("failed to delete quote relationship", zap.Error(err))
		return common.RespondFailedToDelete(ctx, "quote")
	}

	return okJSON(apimodels.MessageResponse{Message: "quote deleted"})
}

// HandleGetQuotePermissionsLift handles GET /api/v1/accounts/:id/quote_permissions.
// Authentication happens before account resolution. Only the authenticated
// account may read its policy; other and missing accounts share a 404 so the
// block list is not exposed and the route does not become an account oracle.
func (h *Handler) HandleGetQuotePermissionsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	accountID := ctx.Param("id")
	if err := common.ValidateRequiredParam("account_id", accountID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	claims, err := h.authenticateWithAnyScope(ctx, "read:accounts", auth.ScopeRead)
	if err != nil {
		return respondQuoteAuthError(ctx, err)
	}
	if h.registry == nil || h.registry.Accounts() == nil || h.registry.Quotes() == nil {
		h.logger.Error("quote permission services unavailable")
		return common.RespondInternalServerError(ctx)
	}

	actor, err := h.resolveAccountIDFull(ctx.Context(), accountID)
	if err != nil || actor == nil || !h.actorAppearsLocal(actor) || strings.TrimSpace(actor.PreferredUsername) == "" {
		if err != nil && !commonerrors.HasCode(err, commonerrors.CodeNotFound) {
			h.logger.Error("failed to resolve quote permission account", zap.String("account_id", accountID), zap.Error(err))
			return common.RespondInternalServerError(ctx)
		}
		return common.RespondNotFound(ctx, "account")
	}
	if !strings.EqualFold(actor.PreferredUsername, claims.Username) {
		return common.RespondNotFound(ctx, "account")
	}

	permissions, err := h.registry.Quotes().GetQuotePermissions(ctx.Context(), claims.Username)
	if err != nil {
		h.logger.Error("failed to get quote permissions", zap.String("account_id", accountID), zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}
	return okJSON(quotePermissionsResponse(permissions))
}

// HandleUpdateQuotePermissionsLift handles PUT /api/v1/accounts/quote_permissions.
func (h *Handler) HandleUpdateQuotePermissionsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	claims, err := h.authenticateWithAnyScope(ctx, "write:accounts", auth.ScopeWrite)
	if err != nil {
		return respondQuoteAuthError(ctx, err)
	}

	var params apimodels.UpdateQuotePermissionsRequest
	if err := common.ParseRequestWithFallback(ctx, &params); err != nil {
		return common.RespondBadRequest(ctx, "invalid request body")
	}
	blockList, err := normalizeQuoteBlockList(params.BlockList)
	if err != nil {
		return common.RespondValidationError(ctx, err)
	}

	if h.registry == nil || h.registry.Quotes() == nil {
		h.logger.Error("quote permission service unavailable")
		return common.RespondInternalServerError(ctx)
	}
	quotesService := h.registry.Quotes()
	permissions, err := quotesService.GetQuotePermissions(ctx.Context(), claims.Username)
	if err != nil {
		h.logger.Error("failed to load quote permissions", zap.String("username", claims.Username), zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}
	if permissions == nil {
		permissions = &storageModels.QuotePermissions{Username: claims.Username}
		permissions.SetDefaults()
	}

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
		permissions.BlockList = blockList
	}
	if permissions.BlockList == nil {
		permissions.BlockList = []string{}
	}
	permissions.Username = claims.Username

	if err := quotesService.UpdateQuotePermissions(ctx.Context(), permissions); err != nil {
		h.logger.Error("failed to save quote permissions", zap.String("username", claims.Username), zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}
	return okJSON(quotePermissionsResponse(permissions))
}

func quoteStatusRequest(params apimodels.CreateQuotePostRequest) apimodels.CreateStatusRequest {
	return apimodels.CreateStatusRequest{
		Status:      params.Status,
		Visibility:  params.Visibility,
		SpoilerText: params.SpoilerText,
		Sensitive:   params.Sensitive,
		Language:    params.Language,
	}
}

func respondQuoteAuthError(ctx *apptheory.Context, err error) (*apptheory.Response, error) {
	if isInsufficientScopeError(err) {
		return common.RespondForbidden(ctx, err.Error())
	}
	return common.RespondUnauthorized(ctx)
}

func respondQuoteAppError(ctx *apptheory.Context, err error) (*apptheory.Response, error) {
	if appErr, ok := commonerrors.AsAppError(err); ok {
		return common.RespondWithAppError(ctx, appErr)
	}
	return common.RespondInternalServerError(ctx)
}

func (h *Handler) respondQuoteTargetError(ctx *apptheory.Context, statusID, viewer string, err error) (*apptheory.Response, error) {
	h.logger.Warn("quote target rejected",
		zap.String("status_id", statusID),
		zap.String("viewer", viewer),
		zap.Error(err))
	if commonerrors.HasCode(err, commonerrors.CodeNotFound) {
		return common.RespondStatusNotFound(ctx)
	}
	return respondQuoteAppError(ctx, err)
}

func (h *Handler) respondVisibleQuoteStatusError(ctx *apptheory.Context, statusID, viewer string, err error) (*apptheory.Response, error) {
	if commonerrors.HasCode(err, commonerrors.CodeNotFound) {
		return common.RespondStatusNotFound(ctx)
	}
	h.logger.Error("failed to list visible quotes",
		zap.String("status_id", statusID),
		zap.String("viewer", viewer),
		zap.Error(err))
	return respondQuoteAppError(ctx, err)
}

func (h *Handler) listVisibleQuoteSummaries(ctx context.Context, notesService NotesService, quotesService QuotesService, statusID, viewer string, limit, offset int) ([]apimodels.QuoteStatusSummary, error) {
	items := make([]apimodels.QuoteStatusSummary, 0, limit)
	cursor := ""
	visible := 0
	scanned := 0
	seenCursors := map[string]struct{}{}

	for scanned < quoteListMaxScanned && len(items) < limit {
		page, err := quotesService.GetQuoteRelationshipsForStatus(ctx, statusID, quoteListPageSize, cursor)
		if err != nil {
			return nil, err
		}
		if page == nil {
			return nil, commonerrors.Internal("quote relationship page is unavailable")
		}

		for _, relationship := range page.Relationships {
			if relationship == nil || scanned >= quoteListMaxScanned {
				continue
			}
			scanned++
			status, err := notesService.GetNoteWithViewer(ctx, &notes.GetNoteQuery{
				StatusID: relationship.QuoterNoteID,
				ViewerID: viewer,
			})
			if err != nil {
				if commonerrors.HasCode(err, commonerrors.CodeNotFound) {
					continue
				}
				return nil, err
			}
			if status == nil {
				continue
			}
			if visible < offset {
				visible++
				continue
			}
			visible++
			items = append(items, quoteStatusSummary(status))
			if len(items) == limit {
				break
			}
		}

		next := strings.TrimSpace(page.NextCursor)
		if next == "" {
			break
		}
		if _, repeated := seenCursors[next]; repeated {
			return nil, commonerrors.Internal("quote relationship cursor repeated")
		}
		seenCursors[next] = struct{}{}
		cursor = next
	}

	return items, nil
}

func quoteStatusSummary(status *storageModels.Status) apimodels.QuoteStatusSummary {
	createdAt := status.CreatedAt
	if createdAt.IsZero() {
		createdAt = status.PublishedAt
	}
	createdAtText := ""
	if !createdAt.IsZero() {
		createdAtText = createdAt.UTC().Format(time.RFC3339Nano)
	}

	accountID := strings.TrimSpace(status.AuthorID)
	username := strings.TrimSpace(status.AuthorUsername)
	if accountID == "" {
		accountID = username
	}
	var usernamePtr *string
	if username != "" {
		usernameCopy := username
		usernamePtr = &usernameCopy
	}

	return apimodels.QuoteStatusSummary{
		ID:        status.StatusID,
		CreatedAt: createdAtText,
		Account: apimodels.QuoteStatusAccount{
			ID:       accountID,
			Username: usernamePtr,
		},
		Content: status.Content,
	}
}

func quotePermissionsResponse(permissions *storageModels.QuotePermissions) apimodels.QuotePermissionsResponse {
	blockList := []string{}
	if permissions != nil && permissions.BlockList != nil {
		blockList = append(blockList, permissions.BlockList...)
	}
	if permissions == nil {
		return apimodels.QuotePermissionsResponse{BlockList: blockList}
	}
	return apimodels.QuotePermissionsResponse{
		AllowPublic:    permissions.AllowPublic,
		AllowFollowers: permissions.AllowFollowers,
		AllowMentioned: permissions.AllowMentioned,
		BlockList:      blockList,
	}
}

func normalizeQuoteBlockList(raw []string) ([]string, error) {
	if raw == nil {
		return nil, nil
	}
	normalized := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, value := range raw {
		username := strings.TrimPrefix(strings.TrimSpace(value), "@")
		if err := common.ValidateUsername(username); err != nil {
			return nil, err
		}
		key := strings.ToLower(username)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, username)
	}
	return normalized, nil
}

func (h *Handler) getQuoteRelationship(ctx *apptheory.Context, quoteStatusID string, targetStatusID string) (*storageModels.QuoteRelationship, error) {
	relationship, err := h.repos.Quote().GetQuoteRelationship(ctx.Context(), quoteStatusID, targetStatusID)
	if err != nil {
		if commonerrors.HasCode(err, commonerrors.CodeNotFound) || strings.Contains(strings.ToLower(err.Error()), "not found") {
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
