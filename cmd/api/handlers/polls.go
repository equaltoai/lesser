package lift

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/notifications"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleGetPollLift retrieves a poll by ID
//
//nolint:gocognit // Complex poll handling with vote counting and user permissions
func (h *Handler) HandleGetPollLift(ctx *lift.Context) error {
	pollID := ctx.Param("id")
	if err := common.ValidateRequiredParam("poll_id", pollID); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]any{"error": "poll ID required"})
	}

	// Extract token from Authorization header (optional for public polls)
	authHeader := ctx.Header("Authorization")
	if authHeader == "" {
		authHeader = ctx.Header("authorization")
	}

	// Test mode support
	var userID string
	if authHeader != "" {
		token, err := auth.ExtractBearerToken(authHeader)
		if err == nil {
			oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
			claims, err := oauthSvc.ValidateAccessToken(token)
			if err == nil {
				// Get the user's actor to get their ID
				account, err := h.registry.Accounts().GetAccount(ctx.Context, claims.Username)
				if err == nil && account.Actor != nil {
					userID = account.Actor.ID
				}
			}
		}
	}

	// Get the poll
	poll, err := h.repos.Poll().GetPoll(ctx.Context, pollID)
	if err != nil {
		h.logger.Error("failed to get poll", zap.String("poll_id", pollID), zap.Error(err))
		ctx.Status(http.StatusNotFound)
		return ctx.JSON(map[string]any{"error": "poll not found"})
	}

	// Use the pre-calculated vote counts from the poll
	optionVotes := poll.VotesCount
	if optionVotes == nil {
		optionVotes = make([]int, len(poll.Options))
	}

	// Check if poll has expired
	expired := poll.ExpiresAt != nil && !poll.ExpiresAt.IsZero() && time.Now().After(*poll.ExpiresAt)

	// Check if user has voted
	var voted bool
	var ownVotes []int
	if userID != "" {
		// Get user's votes from PollVote repository
		hasVoted, userVotes, err := h.repos.Poll().HasUserVoted(ctx, poll.ID, userID)
		if err != nil {
			// Log error but don't fail - just assume no votes
			h.logger.Warn("failed to get user poll votes",
				zap.String("poll_id", poll.ID),
				zap.String("user_id", userID),
				zap.Error(err))
			voted = false
			ownVotes = []int{}
		} else {
			voted = hasVoted
			ownVotes = userVotes
		}
	}

	// Build options data for response
	optionsData := make([]models.PollOption, len(poll.Options))
	for i, option := range poll.Options {
		optionsData[i] = models.PollOption{
			Title:      option,
			VotesCount: optionVotes[i],
		}
	}

	// Build response
	resp := models.Poll{
		ID:          poll.ID,
		ExpiresAt:   poll.ExpiresAt.Format(time.RFC3339),
		Expired:     expired,
		Multiple:    poll.Multiple,
		VotesCount:  poll.VotersCount, // Total votes, not per-option
		VotersCount: poll.VotersCount,
		Voted:       voted,
		OwnVotes:    ownVotes,
		OptionsData: optionsData,
		Emojis:      h.extractCustomEmojisLift(ctx.Context, poll.Options),
	}

	// Hide totals if requested and poll hasn't expired
	if poll.HideTotals && !expired {
		// Clear vote counts
		for i := range resp.OptionsData {
			resp.OptionsData[i].VotesCount = 0
		}
		resp.VotesCount = 0
		resp.VotersCount = 0
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(resp)
}

// HandleVoteOnPollLift submits a vote on a poll
func (h *Handler) HandleVoteOnPollLift(ctx *lift.Context) error {
	// Validate poll ID
	pollID := ctx.Param("id")
	if err := common.ValidateRequiredParam("poll_id", pollID); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]any{"error": "poll ID required"})
	}

	// Authenticate and get actor
	claims, actor, handled, err := h.authenticatePollVoter(ctx)
	if err != nil || handled {
		return err
	}

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]any{"error": "insufficient scope"})
	}

	// Parse and validate vote request
	req, handled, err := h.parsePollVoteRequest(ctx)
	if err != nil || handled {
		return err
	}

	// Submit vote
	handled, err = h.submitPollVote(ctx, pollID, actor.ID, req.Choices)
	if err != nil || handled {
		return err
	}

	// Get updated poll and build response
	resp, handled, err := h.buildPollVoteResponse(ctx, pollID, req.Choices)
	if err != nil || handled {
		return err
	}

	// Create notification if needed
	h.createPollVoteNotification(ctx, pollID, actor.ID, resp.poll)

	ctx.Status(http.StatusOK)
	return ctx.JSON(resp.response)
}

// authenticatePollVoter handles authentication for poll voting
func (h *Handler) authenticatePollVoter(ctx *lift.Context) (*auth.Claims, *storage.ActorRecord, bool, error) {
	// Extract auth header
	authHeader := h.getPollAuthHeader(ctx)

	// Extract token
	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		ctx.Status(http.StatusUnauthorized)
		if err := ctx.JSON(map[string]any{"error": "unauthorized"}); err != nil {
			return nil, nil, false, err
		}
		return nil, nil, true, nil
	}

	// Validate token
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		ctx.Status(http.StatusUnauthorized)
		if err := ctx.JSON(map[string]any{"error": "unauthorized"}); err != nil {
			return nil, nil, false, err
		}
		return nil, nil, true, nil
	}

	// Get actor
	account, err := h.registry.Accounts().GetAccount(ctx.Context, claims.Username)
	if err != nil || account.Actor == nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		if err := ctx.JSON(map[string]any{"error": "internal server error"}); err != nil {
			return nil, nil, false, err
		}
		return nil, nil, true, nil
	}

	actor := h.convertToActorRecord(account.Actor)
	return claims, actor, false, nil
}

// getPollAuthHeader extracts authorization header
func (h *Handler) getPollAuthHeader(ctx *lift.Context) string {
	authHeader := ctx.Header("Authorization")
	if authHeader == "" {
		authHeader = ctx.Header("authorization")
	}
	return authHeader
}

// convertToActorRecord converts activitypub.Actor to storage.ActorRecord
func (h *Handler) convertToActorRecord(actorData *activitypub.Actor) *storage.ActorRecord {
	return &storage.ActorRecord{
		ID:          actorData.ID,
		Username:    actorData.PreferredUsername,
		Domain:      "", // Local actor
		ActorType:   actorData.Type,
		DisplayName: actorData.Name,
		Avatar:      "", // Would need to extract from Icon
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

// parsePollVoteRequest parses the poll vote request
func (h *Handler) parsePollVoteRequest(ctx *lift.Context) (*models.PollVoteRequest, bool, error) {
	var req models.PollVoteRequest
	if err := ctx.ParseRequest(&req); err != nil {
		// Fallback for test environments
		if ctx.Request != nil && ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
			if err := json.Unmarshal(ctx.Request.Body, &req); err != nil {
				ctx.Status(http.StatusBadRequest)
				if err := ctx.JSON(map[string]any{"error": "invalid request body"}); err != nil {
					return nil, false, err
				}
				return nil, true, nil
			}
		} else {
			ctx.Status(http.StatusBadRequest)
			if err := ctx.JSON(map[string]any{"error": "invalid request body"}); err != nil {
				return nil, false, err
			}
			return nil, true, nil
		}
	}

	// Validate request
	if err := common.ValidateSliceNotEmpty("req.Choices", req.Choices); err != nil {
		ctx.Status(http.StatusUnprocessableEntity)
		if err := ctx.JSON(map[string]any{"error": "no choices provided"}); err != nil {
			return nil, false, err
		}
		return nil, true, nil
	}

	// Validate poll vote choices
	for i, choice := range req.Choices {
		if choice < 0 {
			ctx.Status(http.StatusUnprocessableEntity)
			if err := ctx.JSON(map[string]any{"error": fmt.Sprintf("invalid choice index %d at position %d", choice, i)}); err != nil {
				return nil, false, err
			}
			return nil, true, nil
		}
	}

	return &req, false, nil
}

// submitPollVote submits the vote to the poll
func (h *Handler) submitPollVote(ctx *lift.Context, pollID, voterID string, choices []int) (bool, error) {
	if err := h.repos.Poll().VoteOnPoll(ctx.Context, pollID, voterID, choices); err != nil {
		h.logger.Error("failed to vote on poll",
			zap.String("poll_id", pollID),
			zap.String("voter_id", voterID),
			zap.Error(err))
		ctx.Status(http.StatusUnprocessableEntity)
		if err := ctx.JSON(map[string]any{"error": err.Error()}); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

// pollVoteResponseData holds poll response data
type pollVoteResponseData struct {
	response models.Poll
	poll     *storage.Poll
}

// buildPollVoteResponse builds the response after voting
func (h *Handler) buildPollVoteResponse(ctx *lift.Context, pollID string, choices []int) (*pollVoteResponseData, bool, error) {
	// Get updated poll data
	poll, err := h.repos.Poll().GetPoll(ctx.Context, pollID)
	if err != nil {
		h.logger.Error("failed to get poll after voting", zap.String("poll_id", pollID), zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		if err := ctx.JSON(map[string]any{"error": "internal server error"}); err != nil {
			return nil, false, err
		}
		return nil, true, nil
	}

	// Prepare vote counts
	optionVotes := h.preparePollVoteCounts(poll)

	// Check expiration
	expired := h.isPollExpired(poll)

	// Build options data
	optionsData := h.buildPollOptionsData(poll, optionVotes)

	// Build response
	resp := models.Poll{
		ID:          poll.ID,
		ExpiresAt:   poll.ExpiresAt.Format(time.RFC3339),
		Expired:     expired,
		Multiple:    poll.Multiple,
		VotesCount:  poll.VotersCount,
		VotersCount: poll.VotersCount,
		Voted:       true,
		OwnVotes:    choices,
		OptionsData: optionsData,
		Emojis:      h.extractCustomEmojisLift(ctx.Context, poll.Options),
	}

	// Hide totals if requested
	h.hidePollTotalsIfNeeded(poll, &resp, expired)

	return &pollVoteResponseData{response: resp, poll: poll}, false, nil
}

// preparePollVoteCounts prepares vote counts for the poll
func (h *Handler) preparePollVoteCounts(poll *storage.Poll) []int {
	if poll.VotesCount != nil {
		return poll.VotesCount
	}
	return make([]int, len(poll.Options))
}

// isPollExpired checks if the poll has expired
func (h *Handler) isPollExpired(poll *storage.Poll) bool {
	return poll.ExpiresAt != nil && !poll.ExpiresAt.IsZero() && time.Now().After(*poll.ExpiresAt)
}

// buildPollOptionsData builds the options data for the response
func (h *Handler) buildPollOptionsData(poll *storage.Poll, optionVotes []int) []models.PollOption {
	optionsData := make([]models.PollOption, len(poll.Options))
	for i, option := range poll.Options {
		optionsData[i] = models.PollOption{
			Title:      option,
			VotesCount: optionVotes[i],
		}
	}
	return optionsData
}

// hidePollTotalsIfNeeded hides vote totals if requested and poll hasn't expired
func (h *Handler) hidePollTotalsIfNeeded(poll *storage.Poll, resp *models.Poll, expired bool) {
	if poll.HideTotals && !expired {
		for i := range resp.OptionsData {
			resp.OptionsData[i].VotesCount = 0
		}
		resp.VotesCount = 0
		resp.VotersCount = 0
	}
}

// createPollVoteNotification creates a notification for the poll creator
func (h *Handler) createPollVoteNotification(ctx *lift.Context, pollID, voterID string, poll *storage.Poll) {
	if poll.CreatedBy == voterID {
		return // Don't notify self
	}

	// Use the Notifications service to create the notification
	notificationService := h.registry.Notifications()
	if notificationService != nil {
		cmd := &notifications.CreateNotificationCommand{
			Type:     "poll",
			UserID:   extractUsernameFromActorIDLift(poll.CreatedBy),
			ActorID:  voterID,
			TargetID: poll.StatusID,
		}

		if _, err := notificationService.CreateNotification(ctx.Context, cmd); err != nil {
			h.logger.Warn("failed to create poll notification",
				zap.String("poll_id", pollID),
				zap.Error(err))
			// Don't fail the vote operation
		}
	}
}

// extractUsernameFromActorIDLift extracts the username from an actor ID URL
func extractUsernameFromActorIDLift(actorID string) string {
	// Actor ID format: https://example.com/users/username
	parts := strings.Split(actorID, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}

// extractCustomEmojisLift extracts custom emojis from poll options
func (h *Handler) extractCustomEmojisLift(ctx context.Context, options []string) []any {
	emojis := make([]any, 0)
	emojiMap := make(map[string]bool) // To avoid duplicates

	for _, option := range options {
		// Look for custom emoji patterns like :custom_emoji:
		if emojiCodes := h.findEmojiCodesLift(option); len(emojiCodes) > 0 {
			for _, code := range emojiCodes {
				if !emojiMap[code] {
					// Get emoji data from storage
					if emoji, err := h.repos.Emoji().GetCustomEmoji(ctx, code); err == nil {
						emojis = append(emojis, map[string]any{
							"shortcode":         emoji.Shortcode,
							"url":               emoji.URL,
							"static_url":        emoji.StaticURL,
							"visible_in_picker": emoji.VisibleInPicker,
						})
						emojiMap[code] = true
					}
				}
			}
		}
	}

	return emojis
}

// findEmojiCodesLift finds custom emoji codes in text
func (h *Handler) findEmojiCodesLift(text string) []string {
	codes := make([]string, 0)

	// Simple regex-like pattern matching for :emoji_code:
	start := 0
	for {
		startIdx := strings.Index(text[start:], ":")
		if startIdx == -1 {
			break
		}
		startIdx += start

		endIdx := strings.Index(text[startIdx+1:], ":")
		if endIdx == -1 {
			break
		}
		endIdx += startIdx + 1

		code := text[startIdx+1 : endIdx]
		if len(code) > 0 && isValidEmojiCodeLift(code) {
			codes = append(codes, code)
		}

		start = endIdx + 1
	}

	return codes
}

// isValidEmojiChar checks if a character is valid in emoji code
func isValidEmojiChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') || r == '_'
}

// isValidEmojiCodeLift checks if an emoji code is valid
func isValidEmojiCodeLift(code string) bool {
	// Valid emoji codes contain only letters, numbers, and underscores
	for _, r := range code {
		if !isValidEmojiChar(r) {
			return false
		}
	}
	return len(code) >= 2 && len(code) <= 32
}

// generateRandomStringLift generates a random string of 8 characters
func generateRandomStringLift() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	const length = 8
	result := make([]byte, length)
	for i := range result {
		result[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(result)
}
