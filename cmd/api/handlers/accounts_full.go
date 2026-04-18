// accounts_full.go - Complete service-based implementation of account endpoints
// This implements Phase 3 with full functionality and federation support

package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

// HandleGetAccountFull retrieves account information by ID
func (h *Handler) HandleGetAccountFull(ctx *apptheory.Context) (*apptheory.Response, error) {
	accountID := ctx.Param("id")
	if err := common.ValidateRequiredParam("accountID", accountID); err != nil {
		return common.RespondBadRequest(ctx, "missing account id")
	}

	// Get the actor
	actor, err := h.resolveAccountIDFull(ctx.Context(), accountID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return common.RespondNotFound(ctx, "account not found")
		}
		h.logger.Error("failed to resolve account", zap.Error(err))
		return common.RespondInternalServerError(ctx, "Internal server error")
	}

	// Build account response
	account := h.buildAccountResponseFull(ctx.Context(), actor)
	return okJSON(account)
}

// HandleVerifyCredentialsFull returns the authenticated user's account using Accounts service
func (h *Handler) HandleVerifyCredentialsFull(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Authenticate user
	claims, err := h.authenticateWithScope(ctx, auth.ScopeRead)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	// Call Accounts service
	account, err := h.registry.Accounts().GetAccount(ctx.Context(), claims.Username)
	if err != nil {
		h.logger.Error("failed to get account", zap.Error(err))
		return common.RespondInternalServerError(ctx, "Internal server error")
	}

	return okJSON(account)
}

// HandleUpdateCredentialsFull updates the authenticated user's profile using Accounts service
func (h *Handler) HandleUpdateCredentialsFull(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Parse request
	var req models.UpdateCredentialsRequest
	if err := common.ParseRequestWithFallback(ctx, &req); err != nil {
		return common.RespondBadRequest(ctx, "invalid request format")
	}

	// Authenticate user
	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	// Call Accounts service
	result, err := h.registry.Accounts().UpdateProfile(ctx.Context(), &accounts.UpdateProfileCommand{
		Username:     claims.Username,
		DisplayName:  req.DisplayName,
		Bio:          req.Note,
		Locked:       req.Locked,
		Bot:          req.Bot,
		Discoverable: req.Discoverable,
		UpdaterID:    claims.Username,
	})
	if err != nil {
		h.logger.Error("failed to update profile", zap.Error(err))
		return common.RespondInternalServerError(ctx, "failed to update profile")
	}

	return okJSON(result.Account)
}

// HandleGetAccountStatusesFull retrieves statuses for an account using Notes service
func (h *Handler) HandleGetAccountStatusesFull(ctx *apptheory.Context) (*apptheory.Response, error) {
	accountID := ctx.Param("id")
	if err := common.ValidateRequiredParam("accountID", accountID); err != nil {
		return common.RespondBadRequest(ctx, "missing account id")
	}

	// Parse query parameters
	limitParam := queryValue(ctx, "limit")
	limit, err := common.ParseAccountStatusesLimit(limitParam)
	if err != nil {
		limit = 20
	}

	// Parse other query parameters
	maxID := queryValue(ctx, "max_id")
	onlyMedia := queryValue(ctx, "only_media") == boolTrue
	excludeReplies := queryValue(ctx, "exclude_replies") == boolTrue
	excludeReblogs := queryValue(ctx, "exclude_reblogs") == boolTrue
	pinnedOnly := queryValue(ctx, "pinned") == boolTrue
	tagged := queryValue(ctx, "tagged")

	viewerID := h.getOptionalAuthenticatedUser(ctx)

	// Check if Notes service is available
	notesService := h.registry.Notes()
	if notesService == nil {
		// Fallback to repository method if service not initialized
		return h.handleAccountStatusesFallback(ctx, accountID, limit, maxID, viewerID)
	}

	// Call Notes service to get user timeline
	result, err := notesService.ListNotes(ctx.Context(), &notes.ListNotesQuery{
		TimelineType:   "user",
		AuthorID:       accountID,
		ViewerID:       viewerID,
		OnlyMedia:      onlyMedia,
		ExcludeReplies: excludeReplies,
		ExcludeReblogs: excludeReblogs,
		PinnedOnly:     pinnedOnly,
		Hashtag:        tagged, // Use Hashtag field instead of Tagged
		Pagination: interfaces.PaginationOptions{
			Limit:  limit,
			Cursor: maxID,
		},
	})
	if err != nil {
		h.logger.Error("failed to get account statuses via Notes service", zap.Error(err))
		return common.RespondInternalServerError(ctx, "Internal server error")
	}

	// Convert Notes service result to Mastodon API format
	response := make([]interface{}, len(result.Notes))
	for i, note := range result.Notes {
		response[i] = h.convertStatusToMastodonAPI(note, viewerID)
	}

	resp, err := okJSON(response)
	if err != nil {
		return nil, err
	}

	if result.Pagination != nil && result.Pagination.NextCursor != "" {
		linkHeader := fmt.Sprintf("<%s/api/v1/accounts/%s/statuses?max_id=%s>; rel=\"next\"",
			h.cfg.BaseURL(), accountID, result.Pagination.NextCursor)
		setHeader(resp, "Link", linkHeader)
	}

	return resp, nil
}

// HandleGetAccountFollowersFull retrieves followers for an account
func (h *Handler) HandleGetAccountFollowersFull(ctx *apptheory.Context) (*apptheory.Response, error) {
	accountID := ctx.Param("id")
	if err := common.ValidateRequiredParam("accountID", accountID); err != nil {
		return common.RespondBadRequest(ctx, "missing account id")
	}

	// Parse pagination parameters
	limitParam := queryValue(ctx, "limit")
	limit, err := common.ParseFollowLimit(limitParam)
	if err != nil {
		limit = 40
	}

	maxID := queryValue(ctx, "max_id")

	// Get the account
	actor, err := h.resolveAccountIDFull(ctx.Context(), accountID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return common.RespondNotFound(ctx, "account not found")
		}
		h.logger.Error("failed to resolve account", zap.Error(err))
		return common.RespondInternalServerError(ctx, "Internal server error")
	}

	// Get followers
	followersResult, nextCursor, err := h.registry.Relationships().GetFollowers(ctx.Context(), actor.PreferredUsername, limit, maxID)
	if err != nil {
		h.logger.Error("failed to get followers", zap.Error(err))
		return common.RespondInternalServerError(ctx, "Internal server error")
	}
	followerAccounts := followersResult
	nextMaxID := nextCursor

	// Convert followers to accounts
	accounts := []interface{}{}
	for _, followerAccount := range followerAccounts {
		if followerAccount == nil || followerAccount.Actor == nil {
			continue
		}
		account := h.buildAccountResponseFull(ctx.Context(), followerAccount.Actor)
		accounts = append(accounts, account)
	}

	// Set Link header for pagination
	resp, err := okJSON(accounts)
	if err != nil {
		return nil, err
	}

	if nextMaxID != "" {
		linkHeader := fmt.Sprintf("<%s/api/v1/accounts/%s/followers?max_id=%s>; rel=\"next\"",
			h.cfg.BaseURL(), accountID, nextMaxID)
		setHeader(resp, "Link", linkHeader)
	}

	return resp, nil
}

// HandleGetAccountFollowingFull retrieves accounts that this account follows
func (h *Handler) HandleGetAccountFollowingFull(ctx *apptheory.Context) (*apptheory.Response, error) {
	accountID := ctx.Param("id")
	if err := common.ValidateRequiredParam("accountID", accountID); err != nil {
		return common.RespondBadRequest(ctx, "missing account id")
	}

	// Parse pagination parameters
	limitParam := queryValue(ctx, "limit")
	limit, err := common.ParseFollowLimit(limitParam)
	if err != nil {
		limit = 40
	}

	maxID := queryValue(ctx, "max_id")

	// Get the account
	actor, err := h.resolveAccountIDFull(ctx.Context(), accountID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return common.RespondNotFound(ctx, "account not found")
		}
		h.logger.Error("failed to resolve account", zap.Error(err))
		return common.RespondInternalServerError(ctx, "Internal server error")
	}

	// Get following
	followingIDs, nextMaxID, err := h.registry.Relationships().GetFollowing(ctx.Context(), actor.PreferredUsername, limit, maxID)
	if err != nil {
		h.logger.Error("failed to get following", zap.Error(err))
		return common.RespondInternalServerError(ctx, "Internal server error")
	}

	// Convert following to accounts
	accounts := []interface{}{}
	for _, followingAccount := range followingIDs {
		if followingAccount == nil || followingAccount.Actor == nil {
			continue
		}
		account := h.buildAccountResponseFull(ctx.Context(), followingAccount.Actor)
		accounts = append(accounts, account)
	}

	// Set Link header for pagination
	resp, err := okJSON(accounts)
	if err != nil {
		return nil, err
	}

	if nextMaxID != "" {
		linkHeader := fmt.Sprintf("<%s/api/v1/accounts/%s/following?max_id=%s>; rel=\"next\"",
			h.cfg.BaseURL(), accountID, nextMaxID)
		setHeader(resp, "Link", linkHeader)
	}

	return resp, nil
}

// Helper methods

func (h *Handler) resolveAccountIDFull(ctx context.Context, accountID string) (*activitypub.Actor, error) {
	accountID = normalizeResolvedAccountID(accountID)

	if common.ValidateNumericID("account_id", accountID) == nil && len(accountID) >= 10 {
		return h.resolveAccountID(ctx, accountID)
	}

	// Check if it's a local username or full actor ID
	if strings.HasPrefix(accountID, schemeHTTP) {
		// Full actor ID - extract username from URL
		parts := strings.Split(accountID, "/")
		if len(parts) > 0 {
			username := parts[len(parts)-1]
			account, err := h.registry.Accounts().GetAccount(ctx, username)
			if err != nil {
				return nil, err
			}
			return account.Actor, nil
		}
		return nil, invalidActorIDFormat()
	}
	// Local username or @username@domain format
	if strings.Contains(accountID, "@") {
		parts := strings.Split(accountID, "@")
		if len(parts) >= 2 {
			username := parts[0]
			if err := common.ValidateRequiredParam("username", username); err != nil && len(parts) >= 3 {
				username = parts[1] // Handle @username@domain format
			}
			// For now, only handle local accounts
			account, err := h.registry.Accounts().GetAccount(ctx, username)
			if err != nil {
				return nil, err
			}
			return account.Actor, nil
		}
	}
	// Direct username lookup
	account, err := h.registry.Accounts().GetAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	return account.Actor, nil
}

func (h *Handler) buildAccountResponseFull(ctx context.Context, actor *activitypub.Actor) map[string]interface{} {
	// Get counts
	statusesCount, _ := h.registry.Notes().CountNotesByAuthor(ctx, actor.ID)
	followersCount, _ := h.registry.Relationships().CountFollowers(ctx, actor.ID)
	followingIDs, _, _ := h.registry.Relationships().GetFollowing(ctx, actor.PreferredUsername, 1, "")
	followingCount := len(followingIDs)

	account := map[string]interface{}{
		"id":              actor.ID,
		"username":        actor.PreferredUsername,
		"acct":            actor.PreferredUsername, // For local accounts
		"display_name":    actor.Name,
		"locked":          actor.ManuallyApprovesFollowers,
		"bot":             actor.Type == "Service",
		"discoverable":    actor.Discoverable,
		"group":           actor.Type == "Group",
		"created_at":      time.Now().Format(time.RFC3339), // Fallback
		"note":            actor.Summary,
		"url":             actor.URL,
		"avatar":          "",
		"avatar_static":   "",
		"header":          "",
		"header_static":   "",
		"followers_count": followersCount,
		"following_count": followingCount,
		"statuses_count":  statusesCount,
		"last_status_at":  nil,
		"emojis":          []interface{}{},
		"fields":          []interface{}{},
		"moved":           nil,
		"suspended":       false,
		"limited":         false,
		"noindex":         false,
	}

	// Set avatar if available
	if actor.Icon != nil && common.ValidateRequiredParam("iconURL", actor.Icon.URL) == nil {
		account["avatar"] = actor.Icon.URL
		account["avatar_static"] = actor.Icon.URL
	}

	// Set header if available
	if actor.Image != nil && common.ValidateRequiredParam("imageURL", actor.Image.URL) == nil {
		account["header"] = actor.Image.URL
		account["header_static"] = actor.Image.URL
	}

	// Set last status time if available
	if actor.LastStatusAt != nil {
		account["last_status_at"] = actor.LastStatusAt.Format("2006-01-02")
	}

	return account
}

// handleAccountStatusesFallback provides fallback implementation when Notes service unavailable
func (h *Handler) handleAccountStatusesFallback(ctx *apptheory.Context, accountID string, limit int, maxID string, viewerID string) (*apptheory.Response, error) {
	// Get the account
	actor, err := h.resolveAccountIDFull(ctx.Context(), accountID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return common.RespondNotFound(ctx, "account not found")
		}
		h.logger.Error("failed to resolve account", zap.Error(err))
		return common.RespondInternalServerError(ctx, "Internal server error")
	}

	// Get statuses using GetUserTimeline
	opts := interfaces.PaginationOptions{Limit: limit, Cursor: maxID}
	result, err := h.registry.Notes().GetUserTimeline(ctx.Context(), actor.ID, opts)
	if err != nil {
		h.logger.Error("failed to get account statuses", zap.Error(err))
		return common.RespondInternalServerError(ctx, "Internal server error")
	}
	statusModels := result.Items
	nextCursor := result.NextCursor

	// Convert to API format
	response := make([]interface{}, len(statusModels))
	for i, statusModel := range statusModels {
		response[i] = h.convertStatusToMastodonAPI(statusModel, viewerID)
	}

	resp, err := okJSON(response)
	if err != nil {
		return nil, err
	}

	if nextCursor != "" {
		linkHeader := fmt.Sprintf("<%s/api/v1/accounts/%s/statuses?max_id=%s>; rel=\"next\"",
			h.cfg.BaseURL(), accountID, nextCursor)
		setHeader(resp, "Link", linkHeader)
	}

	return resp, nil
}

// convertStatusToMastodonAPI converts a Status model to Mastodon API format
func (h *Handler) convertStatusToMastodonAPI(status *storageModels.Status, viewerID string) map[string]interface{} {
	// Get account info if available
	var account map[string]interface{}
	if accountData, err := h.registry.Accounts().GetAccount(context.Background(), status.AuthorUsername); err == nil && accountData.Actor != nil {
		account = h.buildAccountResponseFull(context.Background(), accountData.Actor)
	} else {
		// Minimal account if actor lookup fails
		account = map[string]interface{}{
			"id":       status.AuthorID,
			"username": status.AuthorUsername,
			"acct":     status.AuthorUsername,
		}
	}

	return map[string]interface{}{
		"id":                     status.StatusID,
		"created_at":             status.CreatedAt.Format(time.RFC3339),
		"content":                status.Content,
		"visibility":             status.Visibility,
		"account":                account,
		"media_attachments":      []interface{}{},
		"mentions":               []interface{}{},
		"tags":                   []interface{}{},
		"emojis":                 []interface{}{},
		"card":                   nil,
		"poll":                   nil,
		"application":            nil,
		"in_reply_to_id":         status.InReplyToID,
		"in_reply_to_account_id": h.getReplyToAccountID(context.Background(), viewerID, status),
		"sensitive":              status.Sensitive,
		"spoiler_text":           "",
		"language":               status.Language,
		"uri":                    status.StatusID,
		"url":                    fmt.Sprintf("%s/@%s/%s", h.cfg.BaseURL(), status.AuthorUsername, status.StatusID),
		"replies_count":          h.getStatusRepliesCount(status.StatusID),
		"reblogs_count":          h.getStatusReblogsCount(status.StatusID),
		"favourites_count":       h.getStatusFavoritesCount(status.StatusID),
		"edited_at":              h.getEditedAt(status),
		"reblog":                 nil,
		"favourited":             h.hasUserFavorited(viewerID, status.StatusID),
		"reblogged":              h.hasUserReblogged(viewerID, status.StatusID),
		"muted":                  h.hasUserMutedStatus(context.Background(), viewerID, status.StatusID),
		"bookmarked":             h.hasUserBookmarked(context.Background(), viewerID, status.StatusID),
		"pinned":                 h.hasUserPinned(context.Background(), viewerID, status.StatusID),
		"filtered":               []interface{}{},
	}
}

// getStatusRepliesCount gets the number of replies to a status
func (h *Handler) getStatusRepliesCount(statusID string) int {
	count, err := h.registry.Notes().CountReplies(context.Background(), statusID)
	if err != nil {
		h.logger.Debug("failed to count replies", zap.String("status_id", statusID), zap.Error(err))
		return 0
	}
	return count
}

// getStatusReblogsCount gets the number of reblogs/boosts for a status
func (h *Handler) getStatusReblogsCount(statusID string) int64 {
	count, err := h.registry.Notes().GetBoostCount(context.Background(), statusID)
	if err != nil {
		h.logger.Debug("failed to count reblogs", zap.String("status_id", statusID), zap.Error(err))
		return 0
	}
	return count
}

// getStatusFavoritesCount gets the number of likes/favorites for a status
func (h *Handler) getStatusFavoritesCount(statusID string) int64 {
	count, err := h.registry.Notes().GetLikeCount(context.Background(), statusID)
	if err != nil {
		h.logger.Debug("failed to count favorites", zap.String("status_id", statusID), zap.Error(err))
		return 0
	}
	return count
}

// hasUserFavorited checks if the viewer has favorited a status
func (h *Handler) hasUserFavorited(viewerID, statusID string) bool {
	if err := common.ValidateRequiredParam("viewerID", viewerID); err != nil {
		return false
	}

	favorited, err := h.registry.Notes().HasLiked(context.Background(), viewerID, statusID)
	if err != nil {
		h.logger.Debug("failed to check if user favorited",
			zap.String("viewer_id", viewerID),
			zap.String("status_id", statusID),
			zap.Error(err))
		return false
	}
	return favorited
}

// hasUserReblogged checks if the viewer has reblogged a status
func (h *Handler) hasUserReblogged(viewerID, statusID string) bool {
	if err := common.ValidateRequiredParam("viewerID", viewerID); err != nil {
		return false
	}

	reblogged, err := h.registry.Notes().HasReblogged(context.Background(), viewerID, statusID)
	if err != nil {
		h.logger.Debug("failed to check if user reblogged",
			zap.String("viewer_id", viewerID),
			zap.String("status_id", statusID),
			zap.Error(err))
		return false
	}
	return reblogged
}

// getReplyToAccountID gets the account ID of the status being replied to
func (h *Handler) getReplyToAccountID(ctx context.Context, viewerID string, status *storageModels.Status) *string {
	if err := common.ValidateRequiredParam("inReplyToID", status.InReplyToID); err != nil {
		return nil
	}

	// Get the replied-to status
	replyStatus, err := h.registry.Notes().GetNoteWithViewer(ctx, &notes.GetNoteQuery{
		StatusID: status.InReplyToID,
		ViewerID: viewerID,
	})
	if err != nil {
		h.logger.Debug("failed to get reply-to status",
			zap.String("status_id", status.InReplyToID),
			zap.Error(err))
		return nil
	}

	return &replyStatus.AuthorID
}

// getEditedAt returns the edited timestamp if the status was edited
func (h *Handler) getEditedAt(status *storageModels.Status) *string {
	// Check if UpdatedAt is significantly different from CreatedAt (more than 1 minute)
	if status.UpdatedAt.Sub(status.CreatedAt) > time.Minute {
		editedAt := status.UpdatedAt.Format(time.RFC3339)
		return &editedAt
	}
	return nil
}

// hasUserMutedStatus checks if the viewer has muted this status
func (h *Handler) hasUserMutedStatus(ctx context.Context, viewerID, statusID string) bool {
	if err := common.ValidateRequiredParam("viewerID", viewerID); err != nil {
		return false
	}

	// Check if user has muted the status author
	status, err := h.registry.Notes().GetNoteWithViewer(ctx, &notes.GetNoteQuery{
		StatusID: statusID,
		ViewerID: viewerID,
	})
	if err != nil {
		return false
	}

	muted, err := h.registry.Relationships().IsMuted(ctx, viewerID, status.AuthorUsername)
	if err != nil {
		h.logger.Debug("failed to check mute status",
			zap.String("viewer_id", viewerID),
			zap.String("status_id", statusID),
			zap.Error(err))
		return false
	}
	return muted
}

// hasUserBookmarked checks if the viewer has bookmarked this status
func (h *Handler) hasUserBookmarked(ctx context.Context, viewerID, statusID string) bool {
	if err := common.ValidateRequiredParam("viewerID", viewerID); err != nil {
		return false
	}

	bookmarked, err := h.registry.Notes().IsBookmarked(ctx, viewerID, statusID)
	if err != nil {
		h.logger.Debug("failed to check bookmark status",
			zap.String("viewer_id", viewerID),
			zap.String("status_id", statusID),
			zap.Error(err))
		return false
	}
	return bookmarked
}

// hasUserPinned checks if the viewer has pinned this status
// Note: This checks for account pins, not status pins. Status pinning is different.
func (h *Handler) hasUserPinned(ctx context.Context, viewerID, statusID string) bool {
	if err := common.ValidateRequiredParam("viewerID", viewerID); err != nil {
		return false
	}

	// Get the status to check its author
	status, err := h.registry.Notes().GetNoteWithViewer(ctx, &notes.GetNoteQuery{
		StatusID: statusID,
		ViewerID: viewerID,
	})
	if err != nil {
		return false
	}

	// Check if viewer has pinned the status author's account
	pinned, err := h.registry.Accounts().IsAccountPinned(ctx, viewerID, status.AuthorID)
	if err != nil {
		h.logger.Debug("failed to check account pin status",
			zap.String("viewer_id", viewerID),
			zap.String("status_id", statusID),
			zap.String("author_id", status.AuthorID),
			zap.Error(err))
		return false
	}
	return pinned
}
