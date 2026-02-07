// statuses_full.go - Complete service-based implementation of status endpoints
// This implements Phase 3 with full ActivityPub and federation support

package handlers

import (
	"context"
	"errors"
	"strings"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/notes"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/equaltoai/lesser/pkg/transformations"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

// Import streaming constants
const (
	StatusCreated = streaming.StatusCreated
	StatusDeleted = streaming.StatusDeleted
	UserStream    = streaming.UserStream
	PublicStream  = streaming.PublicStream
)

// HandleCreateStatusFull creates a new status using the Notes service
func (h *Handler) HandleCreateStatusFull(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Parse request
	var req models.CreateStatusRequest
	if err := common.ParseRequestWithFallback(ctx, &req); err != nil {
		return common.RespondBadRequest(ctx, "invalid request format")
	}

	// Authenticate with write scope
	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	// Extract poll data if poll is provided
	var pollOptions []string
	var pollExpiresIn int
	var pollMultiple bool
	var pollHideTotals bool

	if req.Poll != nil && len(req.Poll.Options) > 0 {
		// Validate poll parameters using centralized validation
		pollMap := map[string]interface{}{
			"options":     req.Poll.Options,
			"expires_in":  req.Poll.ExpiresIn,
			"multiple":    req.Poll.Multiple,
			"hide_totals": req.Poll.HideTotals,
		}
		if err := common.ValidatePollParams(pollMap); err != nil {
			return common.RespondUnprocessableEntity(ctx, "Invalid poll parameters: "+err.Error())
		}

		pollOptions = req.Poll.Options
		pollExpiresIn = req.Poll.ExpiresIn
		pollMultiple = req.Poll.Multiple
		pollHideTotals = req.Poll.HideTotals
	}

	// Call Notes service
	result, err := h.registry.Notes().CreateNote(ctx.Context(), &notes.CreateNoteCommand{
		AuthorID:       claims.Username,
		Content:        req.Status,
		Visibility:     req.Visibility,
		Sensitive:      req.Sensitive,
		SpoilerText:    req.SpoilerText,
		Language:       req.Language,
		InReplyToID:    req.InReplyToID,
		MediaIDs:       req.MediaIDs,
		PollOptions:    pollOptions,
		PollExpiresIn:  pollExpiresIn,
		PollMultiple:   pollMultiple,
		PollHideTotals: pollHideTotals,
	})
	if err != nil {
		h.logger.Error("failed to create note", zap.Error(err))
		return common.RespondInternalServerError(ctx, "failed to create status")
	}

	// Convert to Mastodon API format using converter
	mastodonStatus := transformations.NotesToStatusAny(result.Note, h.cfg.BaseURL())

	// Enrich with poll data if poll exists
	if err := h.enrichStatusWithPoll(ctx.Context(), &mastodonStatus, result.Note.StatusID, claims.Username); err != nil {
		h.logger.Warn("failed to enrich status with poll data", zap.Error(err))
	}

	// Return created status
	return createdJSON(mastodonStatus)
}

// HandleGetStatusFull retrieves a status by ID using the Notes service
func (h *Handler) HandleGetStatusFull(ctx *apptheory.Context) (*apptheory.Response, error) {
	statusID := ctx.Param("id")
	if err := common.ValidateStatusParamID(statusID); err != nil {
		return common.RespondBadRequest(ctx, "missing status id")
	}

	// Extract optional authenticated user context for privacy filtering
	viewerID := h.getOptionalAuthenticatedUser(ctx)

	// Call Notes service to get the note
	note, err := h.registry.Notes().GetNoteWithViewer(ctx.Context(), &notes.GetNoteQuery{
		StatusID: statusID,
		ViewerID: viewerID,
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "access denied") {
			return common.RespondNotFound(ctx, "status not found")
		}
		return common.RespondInternalServerError(ctx, "Internal server error")
	}

	// Convert to Mastodon API format using converter
	mastodonStatus := transformations.NotesToStatusAny(note, h.cfg.BaseURL())

	// Enrich with poll data if poll exists
	if err := h.enrichStatusWithPoll(ctx.Context(), &mastodonStatus, note.StatusID, viewerID); err != nil {
		h.logger.Warn("failed to enrich status with poll data", zap.Error(err))
	}

	return okJSON(mastodonStatus)
}

// HandleDeleteStatusFull deletes a status using the Notes service
func (h *Handler) HandleDeleteStatusFull(ctx *apptheory.Context) (*apptheory.Response, error) {
	statusID := ctx.Param("id")
	if err := common.ValidateStatusParamID(statusID); err != nil {
		return common.RespondBadRequest(ctx, "missing status id")
	}

	// Authenticate with write scope
	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	// Call Notes service
	err = h.registry.Notes().DeleteNote(ctx.Context(), &notes.DeleteNoteCommand{
		StatusID:  statusID,
		DeleterID: claims.Username,
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return common.RespondNotFound(ctx, "status not found")
		}
		if strings.Contains(err.Error(), "not authorized") {
			return common.RespondForbidden(ctx, "not authorized to delete this status")
		}
		return common.RespondInternalServerError(ctx, "failed to delete status")
	}

	// Return empty response for successful deletion
	return okJSON(map[string]interface{}{})
}

// Helper methods

// checkStatusViewPermission implements comprehensive privacy checking for status visibility
func (h *Handler) checkStatusViewPermission(ctx context.Context, status *storageModels.Status, viewerID string) (bool, error) {
	// Import the visibility constants
	const (
		VisibilityPublic   = "public"
		VisibilityUnlisted = "unlisted"
		VisibilityPrivate  = "private"
		VisibilityDirect   = "direct"
	)

	// Public and unlisted posts are viewable by anyone
	if status.Visibility == VisibilityPublic || status.Visibility == VisibilityUnlisted {
		return true, nil
	}

	// Unauthenticated users can only see public/unlisted posts
	if err := common.ValidateRequiredParam("viewer_id", viewerID); err != nil {
		return false, nil
	}

	// Status author can always view their own posts
	if status.AuthorUsername == viewerID {
		return true, nil
	}

	// Handle private (followers-only) posts
	if status.Visibility == VisibilityPrivate {
		return h.checkPrivateVisibility(ctx, status, viewerID)
	}

	// Handle direct messages
	if status.Visibility == VisibilityDirect {
		return h.checkDirectMessageVisibility(status, viewerID), nil
	}

	// Unknown visibility level - default to private
	h.logger.Warn("unknown visibility level encountered",
		zap.String("status_id", status.StatusID),
		zap.String("visibility", status.Visibility))
	return false, nil
}

// checkPrivateVisibility checks if viewer can see private (followers-only) posts
func (h *Handler) checkPrivateVisibility(ctx context.Context, status *storageModels.Status, viewerID string) (bool, error) {
	isFollowing, err := h.repos.Relationship().IsFollowing(ctx, viewerID, status.AuthorUsername)
	if err != nil {
		return false, errors.New("failed to check following relationship: " + err.Error())
	}
	return isFollowing, nil
}

// checkDirectMessageVisibility checks if viewer can see direct messages
func (h *Handler) checkDirectMessageVisibility(status *storageModels.Status, viewerID string) bool {
	// Check if viewer is explicitly mentioned in the status
	if h.isViewerMentioned(status.Mentions, viewerID) {
		return true
	}

	// Check if viewer is in any recipient list
	return h.isViewerInRecipientLists(status, viewerID)
}

// isViewerMentioned checks if viewer is mentioned in the status
func (h *Handler) isViewerMentioned(mentions []string, viewerID string) bool {
	for _, mention := range mentions {
		if mention == viewerID {
			return true
		}
	}
	return false
}

// isViewerInRecipientLists checks all recipient lists for viewer
func (h *Handler) isViewerInRecipientLists(status *storageModels.Status, viewerID string) bool {
	viewerActorID := "https://" + h.cfg.Domain + "/users/" + viewerID

	// Check all recipient lists
	recipientLists := [][]string{
		status.ToRecipients,
		status.CcRecipients,
		status.BtoRecipients,
		status.BccRecipients,
	}

	for _, recipients := range recipientLists {
		for _, recipient := range recipients {
			if recipient == viewerActorID {
				return true
			}
		}
	}

	return false
}

// enrichStatusWithPoll adds poll data to a status if it exists
func (h *Handler) enrichStatusWithPoll(ctx context.Context, status *models.Status, statusID, userID string) error {
	// Try to get poll by status ID
	poll, err := h.repos.Poll().GetPollByStatusID(ctx, statusID)
	if err != nil {
		// If poll not found, that's okay - not all statuses have polls
		if strings.Contains(err.Error(), "not found") {
			return nil
		}
		return errors.New("failed to get poll: " + err.Error())
	}

	// Get user's votes if authenticated
	var userVotes []int
	if userID != "" {
		// Get user's actor ID
		if account, err := h.registry.Accounts().GetAccount(ctx, userID); err == nil && account.Actor != nil {
			if hasVoted, votes, err := h.repos.Poll().HasUserVoted(ctx, poll.ID, account.Actor.ID); err == nil && hasVoted {
				userVotes = votes
			}
		}
	}

	// Convert poll to API format and add to status
	pollAPI := h.converter.PollToAPI(poll, userVotes)
	status.Poll = &pollAPI

	return nil
}

// generateRandomStringFull generates a random string for IDs
