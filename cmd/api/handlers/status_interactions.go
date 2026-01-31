package lift

import (
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/transformations"
	"github.com/pay-theory/lift/pkg/lift"
)

// statusInteractionType represents the type of status interaction
type statusInteractionType int

const (
	statusFavourites statusInteractionType = iota
	statusReblogs
)

// statusInteractionConfig holds configuration for status interaction endpoints
type statusInteractionConfig struct {
	endpoint    string
	errorAction string
}

// getStatusInteractionConfig returns configuration for interaction type
func getStatusInteractionConfig(interactionType statusInteractionType) statusInteractionConfig {
	switch interactionType {
	case statusFavourites:
		return statusInteractionConfig{
			endpoint:    "favourited_by",
			errorAction: "get likes",
		}
	case statusReblogs:
		return statusInteractionConfig{
			endpoint:    "reblogged_by",
			errorAction: "get reblogs",
		}
	default:
		return statusInteractionConfig{}
	}
}

// handleStatusInteractions handles both favourited_by and reblogged_by endpoints
func (h *Handler) handleStatusInteractions(ctx *lift.Context, interactionType statusInteractionType) error {
	statusID := ctx.Param("id")
	if err := common.ValidateStatusID(statusID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Parse pagination parameters
	limitStr := ctx.Query("limit")
	limit, err := common.ParseFollowLimit(limitStr)
	if err != nil {
		limit = 20
	}
	cursor := ctx.Query("max_id")

	config := getStatusInteractionConfig(interactionType)

	// Call appropriate Notes service method and handle result
	var accounts []models.Account
	var nextCursor string

	switch interactionType {
	case statusFavourites:
		result, err := h.registry.Notes().GetLikers(ctx.Context, &notes.GetLikersQuery{
			StatusID: statusID,
			Pagination: interfaces.PaginationOptions{
				Limit:  limit,
				Cursor: cursor,
			},
		})
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				return common.RespondNotFound(ctx, "status not found")
			}
			return common.RespondInternalServerError(ctx, "failed to "+config.errorAction)
		}

		// Convert storage accounts to API accounts
		accounts = make([]models.Account, 0, len(result.Users))
		for _, user := range result.Users {
			if user.Actor != nil {
				account := transformations.ActorToAccountBase(user.Actor, h.cfg.BaseURL())
				accounts = append(accounts, account)
			}
		}
		nextCursor = result.Pagination.NextCursor

	case statusReblogs:
		result, err := h.registry.Notes().GetRebloggers(ctx.Context, &notes.GetRebloggersQuery{
			StatusID: statusID,
			Pagination: interfaces.PaginationOptions{
				Limit:  limit,
				Cursor: cursor,
			},
		})
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				return common.RespondNotFound(ctx, "status not found")
			}
			return common.RespondInternalServerError(ctx, "failed to "+config.errorAction)
		}

		// Convert storage accounts to API accounts
		accounts = make([]models.Account, 0, len(result.Users))
		for _, user := range result.Users {
			if user.Actor != nil {
				account := transformations.ActorToAccountBase(user.Actor, h.cfg.BaseURL())
				accounts = append(accounts, account)
			}
		}
		nextCursor = result.Pagination.NextCursor
	}

	// Set pagination header
	if nextCursor != "" && len(accounts) > 0 {
		linkHeader := fmt.Sprintf(`<%s/api/v1/statuses/%s/%s?max_id=%s&limit=%d>; rel="next"`,
			h.cfg.BaseURL(), statusID, config.endpoint, nextCursor, limit)
		ctx.Response.Header("Link", linkHeader)
	}

	return ctx.JSON(accounts)
}

// HandleGetStatusFavouritedByLift handles GET /api/v1/statuses/:id/favourited_by
func (h *Handler) HandleGetStatusFavouritedByLift(ctx *lift.Context) error {
	return h.handleStatusInteractions(ctx, statusFavourites)
}

// HandleGetStatusRebloggedByLift handles GET /api/v1/statuses/:id/reblogged_by
func (h *Handler) HandleGetStatusRebloggedByLift(ctx *lift.Context) error {
	return h.handleStatusInteractions(ctx, statusReblogs)
}
