package lift

import (
	"fmt"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleGetFavouritesLift handles GET /api/v1/favourites
func (h *Handler) HandleGetFavouritesLift(ctx *lift.Context) error {
	// Authenticate user with read scope
	username, err := h.authenticateUser(ctx, []string{auth.ScopeRead})
	if err != nil {
		return h.respondUnauthorized(ctx)
	}

	// Parse pagination parameters
	params := h.parsePaginationParams(ctx)
	if params.Limit > 40 {
		params.Limit = 40 // Cap at 40 for favorites
	}

	// Use Notes service to get favorited statuses
	notesService := h.registry.Notes()
	if notesService == nil {
		h.logger.Error("notes service not available")
		return h.respondWithError(ctx, 500, "Internal server error")
	}

	// Create query for favorited notes
	query := &notes.ListNotesQuery{
		TimelineType: "favorites", // Special timeline type for favorites
		ViewerID:     username,
		Pagination: interfaces.PaginationOptions{
			Limit:  params.Limit,
			Cursor: params.MaxID,
		},
	}

	// Get favorited notes
	result, err := notesService.GetFavoritedNotes(ctx.Context, query)
	if err != nil {
		h.logger.Error("failed to get favorited notes",
			zap.String("username", username),
			zap.Error(err))
		return h.respondWithError(ctx, 500, "failed to get favorites")
	}

	// Convert to API models
	apiStatuses := make([]*models.Status, 0, len(result.Notes))
	for _, note := range result.Notes {
		apiStatus, err := h.convertStorageStatusToAPI(note, username)
		if err != nil {
			h.logger.Warn("failed to convert status",
				zap.String("status_id", note.StatusID),
				zap.Error(err))
			continue
		}
		apiStatuses = append(apiStatuses, apiStatus)
	}

	// Set pagination headers
	if result.Pagination.NextCursor != "" {
		params.MaxID = result.Pagination.NextCursor
		h.withPaginationHeaders(ctx, fmt.Sprintf("%s/api/v1/favourites", h.cfg.BaseURL()),
			params, true, false)
	}

	return ctx.JSON(apiStatuses)
}
