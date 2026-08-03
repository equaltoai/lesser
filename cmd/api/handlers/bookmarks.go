package handlers

import (
	"context"
	"errors"
	"fmt"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	"go.uber.org/zap"
)

// bookmarkAction performs the bookmark action for a status using the Notes service
func (h *Handler) bookmarkAction(statusID, username string) (*models.Status, error) {
	// Use the Notes service to bookmark the status
	cmd := &notes.BookmarkNoteCommand{
		StatusID:     statusID,
		BookmarkerID: username,
	}

	result, err := h.registry.Notes().BookmarkNote(context.Background(), cmd)
	if err != nil {
		return nil, err
	}

	// Convert the storage Status model to API Status model
	apiStatus, err := h.convertStorageStatusToAPI(result.Status, username)
	if err != nil {
		return nil, errors.Join(failedToConvertStatus(), err)
	}

	return apiStatus, nil
}

// HandleBookmarkLift handles POST /api/v1/statuses/:id/bookmark
func (h *Handler) HandleBookmarkLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	return h.statusActionHandler(ctx, auth.ScopeWrite, h.bookmarkAction)
}

// HandleUnbookmarkLift handles POST /api/v1/statuses/:id/unbookmark
func (h *Handler) HandleUnbookmarkLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	statusID := ctx.Param("id")
	if err := common.ValidateStatusParamID(statusID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Authenticate user
	username, err := h.authenticateUser(ctx, []string{auth.ScopeWrite})
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	// Use the Notes service to unbookmark the status
	cmd := &notes.UnbookmarkNoteCommand{
		StatusID:       statusID,
		UnbookmarkerID: username,
	}

	result, err := h.registry.Notes().UnbookmarkNote(ctx.Context(), cmd)
	if err != nil {
		if err.Error() == "status not found" {
			return common.RespondNotFound(ctx, "status not found")
		}
		h.logger.Error("failed to unbookmark status",
			zap.String("username", username),
			zap.String("status_id", statusID),
			zap.Error(err))
		return common.RespondInternalServerError(ctx, "failed to unbookmark status")
	}

	// Convert the storage Status model to API Status model
	apiStatus, err := h.convertStorageStatusToAPI(result.Status, username)
	if err != nil {
		h.logger.Error("failed to convert status to API format",
			zap.String("status_id", statusID),
			zap.Error(err))
		return common.RespondInternalServerError(ctx, "failed to convert status")
	}

	return okJSON(apiStatus)
}

// HandleGetBookmarksLift handles GET /api/v1/bookmarks
func (h *Handler) HandleGetBookmarksLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Authenticate user
	username, err := h.authenticateUser(ctx, []string{auth.ScopeRead})
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	// Parse query parameters
	limit := 20
	limitStr := queryValue(ctx, "limit")
	if limitStr != "" {
		if l, err := common.ParseTimelineLimit(limitStr); err == nil {
			limit = l
		}
	}

	cursor := queryValue(ctx, "max_id")

	// Use the Notes service to get bookmarks
	query := &notes.GetBookmarksQuery{
		UserID: username,
		Pagination: interfaces.PaginationOptions{
			Limit:  limit,
			Cursor: cursor,
		},
	}

	result, err := h.registry.Notes().GetBookmarks(ctx.Context(), query)
	if err != nil {
		h.logger.Error("failed to get bookmarks",
			zap.String("username", username),
			zap.Error(err))
		return common.RespondInternalServerError(ctx, "failed to get bookmarks")
	}

	// Convert storage Status models to API Status models
	statuses := make([]*models.Status, 0, len(result.Notes))
	for _, storageStatus := range result.Notes {
		apiStatus, err := h.convertStorageStatusToAPI(storageStatus, username)
		if err != nil {
			h.logger.Warn("failed to convert status to API format",
				zap.String("status_id", storageStatus.StatusID),
				zap.Error(err))
			continue
		}
		statuses = append(statuses, apiStatus)
	}

	resp, err := okJSON(statuses)
	if err != nil {
		return nil, err
	}

	if result.Pagination != nil && result.Pagination.NextCursor != "" && len(statuses) > 0 {
		linkHeader := fmt.Sprintf(`<%s/api/v1/bookmarks?max_id=%s&limit=%d>; rel="next"`,
			h.cfg.BaseURL(), result.Pagination.NextCursor, limit)
		setHeader(resp, "Link", linkHeader)
	}

	return resp, nil
}

// NOTE: This function has been moved to a shared location
// The bookmarks handler should use the same convertStorageStatusToAPI from timelines.go
// That version populates all fields with real data
