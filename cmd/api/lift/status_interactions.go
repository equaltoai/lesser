package lift

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/pay-theory/lift/pkg/lift"
)

// HandleGetStatusFavouritedByLift handles GET /api/v1/statuses/:id/favourited_by
func (h *Handler) HandleGetStatusFavouritedByLift(ctx *lift.Context) error {
	statusID := ctx.Param("id")
	if statusID == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "status ID is required"})
	}

	// Parse pagination parameters
	limit := 20
	if limitStr := ctx.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 80 {
			limit = l
		}
	}
	cursor := ctx.Query("max_id")

	// Call Notes service to get likers
	result, err := h.registry.Notes().GetLikers(ctx.Context, &notes.GetLikersQuery{
		StatusID: statusID,
		Pagination: interfaces.PaginationOptions{
			Limit:  limit,
			Cursor: cursor,
		},
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return ctx.Status(404).JSON(map[string]string{"error": "status not found"})
		}
		return ctx.Status(500).JSON(map[string]string{"error": "failed to get likes"})
	}

	// Convert storage accounts to API accounts
	accounts := make([]models.Account, 0, len(result.Users))
	for _, user := range result.Users {
		// Convert storage.Account to activitypub.Actor first
		if user.Actor != nil {
			account := h.converter.ActorToAccount(user.Actor)
			accounts = append(accounts, account)
		}
	}

	// Set pagination header
	if result.Pagination.NextCursor != "" && len(accounts) > 0 {
		linkHeader := fmt.Sprintf(`<%s/api/v1/statuses/%s/favourited_by?max_id=%s&limit=%d>; rel="next"`,
			h.cfg.BaseURL(), statusID, result.Pagination.NextCursor, limit)
		ctx.Response.Header("Link", linkHeader)
	}

	return ctx.JSON(accounts)
}

// HandleGetStatusRebloggedByLift handles GET /api/v1/statuses/:id/reblogged_by
func (h *Handler) HandleGetStatusRebloggedByLift(ctx *lift.Context) error {
	statusID := ctx.Param("id")
	if statusID == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "status ID is required"})
	}

	// Parse pagination parameters
	limit := 20
	if limitStr := ctx.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 80 {
			limit = l
		}
	}
	cursor := ctx.Query("max_id")

	// Call Notes service to get rebloggers
	result, err := h.registry.Notes().GetRebloggers(ctx.Context, &notes.GetRebloggersQuery{
		StatusID: statusID,
		Pagination: interfaces.PaginationOptions{
			Limit:  limit,
			Cursor: cursor,
		},
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return ctx.Status(404).JSON(map[string]string{"error": "status not found"})
		}
		return ctx.Status(500).JSON(map[string]string{"error": "failed to get reblogs"})
	}

	// Convert storage accounts to API accounts
	accounts := make([]models.Account, 0, len(result.Users))
	for _, user := range result.Users {
		// Convert storage.Account to activitypub.Actor first
		if user.Actor != nil {
			account := h.converter.ActorToAccount(user.Actor)
			accounts = append(accounts, account)
		}
	}

	// Set pagination header
	if result.Pagination.NextCursor != "" && len(accounts) > 0 {
		linkHeader := fmt.Sprintf(`<%s/api/v1/statuses/%s/reblogged_by?max_id=%s&limit=%d>; rel="next"`,
			h.cfg.BaseURL(), statusID, result.Pagination.NextCursor, limit)
		ctx.Response.Header("Link", linkHeader)
	}

	return ctx.JSON(accounts)
}
