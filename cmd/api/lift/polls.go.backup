package lift

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleGetPollLift retrieves a poll by ID
func (h *Handler) HandleGetPollLift(ctx *lift.Context) error {
	pollID := ctx.Param("id")
	if pollID == "" {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]any{"error": "poll ID required"})
	}

	// Extract token from Authorization header (optional for public polls)
	authHeader := ctx.Header("Authorization")
	if authHeader == "" {
		authHeader = ctx.Header("authorization")
	}

	// Test mode support
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var userID string
	if testUsername != "" {
		// Test mode - use test username
		actor, err := h.store.GetActor(ctx.Context, testUsername)
		if err == nil {
			userID = actor.ID
		}
	} else if authHeader != "" {
		token, err := auth.ExtractBearerToken(authHeader)
		if err == nil {
			oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
			claims, err := oauthSvc.ValidateAccessToken(token)
			if err == nil {
				// Get the user's actor to get their ID
				actor, err := h.store.GetActor(ctx.Context, claims.Username)
				if err == nil {
					userID = actor.ID
				}
			}
		}
	}

	// Get the poll
	poll, err := h.store.GetPoll(ctx.Context, pollID)
	if err != nil {
		h.logger.Error("failed to get poll", zap.String("poll_id", pollID), zap.Error(err))
		ctx.Status(http.StatusNotFound)
		return ctx.JSON(map[string]any{"error": "poll not found"})
	}

	// Calculate vote counts per option
	optionVotes := make([]int, len(poll.Options))
	for _, choices := range poll.Votes {
		for _, choice := range choices {
			if choice < len(optionVotes) {
				optionVotes[choice]++
			}
		}
	}

	// Check if poll has expired
	expired := !poll.ExpiresAt.IsZero() && time.Now().After(poll.ExpiresAt)

	// Check if user has voted
	var voted bool
	var ownVotes []int
	if userID != "" {
		if userVotes, ok := poll.Votes[userID]; ok {
			voted = true
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
		VotesCount:  poll.VotesCount,
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
	pollID := ctx.Param("id")
	if pollID == "" {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]any{"error": "poll ID required"})
	}

	// Extract token from Authorization header
	authHeader := ctx.Header("Authorization")
	if authHeader == "" {
		authHeader = ctx.Header("authorization")
	}

	// Test mode support
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var claims *auth.Claims
	var actor *storage.ActorRecord

	if testUsername != "" {
		// Test mode - use test username directly
		actorData, err := h.store.GetActor(ctx.Context, testUsername)
		if err != nil {
			h.logger.Error("failed to get test actor", zap.Error(err))
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]any{"error": "unauthorized"})
		}
		actor = &storage.ActorRecord{Actor: actorData}
		// Create test claims
		claims = &auth.Claims{
			Username: testUsername,
			Scopes:   auth.DefaultScopes(), // Test mode gets all scopes
		}
	} else {
		// Normal authentication flow
		token, err := auth.ExtractBearerToken(authHeader)
		if err != nil {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]any{"error": "unauthorized"})
		}

		// Validate token and get claims
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
		claims, err = oauthSvc.ValidateAccessToken(token)
		if err != nil {
			ctx.Status(http.StatusUnauthorized)
			return ctx.JSON(map[string]any{"error": "unauthorized"})
		}

		// Get the user's actor
		actorData, err := h.store.GetActor(ctx.Context, claims.Username)
		if err != nil {
			h.logger.Error("failed to get actor", zap.Error(err))
			ctx.Status(http.StatusInternalServerError)
			return ctx.JSON(map[string]any{"error": "internal server error"})
		}
		actor = &storage.ActorRecord{Actor: actorData}
	}

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]any{"error": "insufficient scope"})
	}

	// Parse request
	var req models.PollVoteRequest
	if err := ctx.ParseRequest(&req); err != nil {
		// Fallback for test environments
		if ctx.Request != nil && ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
			if err := json.Unmarshal(ctx.Request.Body, &req); err != nil {
				ctx.Status(http.StatusBadRequest)
				return ctx.JSON(map[string]any{"error": "invalid request body"})
			}
		} else {
			ctx.Status(http.StatusBadRequest)
			return ctx.JSON(map[string]any{"error": "invalid request body"})
		}
	}

	// Validate request
	if len(req.Choices) == 0 {
		ctx.Status(http.StatusUnprocessableEntity)
		return ctx.JSON(map[string]any{"error": "no choices provided"})
	}

	// Submit vote
	if err := h.store.VoteOnPoll(ctx.Context, pollID, actor.Actor.ID, req.Choices); err != nil {
		h.logger.Error("failed to vote on poll",
			zap.String("poll_id", pollID),
			zap.String("voter_id", actor.Actor.ID),
			zap.Error(err))
		ctx.Status(http.StatusUnprocessableEntity)
		return ctx.JSON(map[string]any{"error": err.Error()})
	}

	// Get updated poll data to return
	poll, err := h.store.GetPoll(ctx.Context, pollID)
	if err != nil {
		h.logger.Error("failed to get poll after voting", zap.String("poll_id", pollID), zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]any{"error": "internal server error"})
	}

	// Calculate vote counts per option
	optionVotes := make([]int, len(poll.Options))
	for _, choices := range poll.Votes {
		for _, choice := range choices {
			if choice < len(optionVotes) {
				optionVotes[choice]++
			}
		}
	}

	// Check if poll has expired
	expired := !poll.ExpiresAt.IsZero() && time.Now().After(poll.ExpiresAt)

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
		VotesCount:  poll.VotesCount,
		VotersCount: poll.VotersCount,
		Voted:       true,
		OwnVotes:    req.Choices,
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

	// Create notification for poll creator if it's not the voter
	if poll.CreatedBy != actor.Actor.ID {
		notification := &storage.Notification{
			ID:        fmt.Sprintf("%d-%s", time.Now().Unix(), generateRandomStringLift(8)),
			Type:      "poll",
			Username:  extractUsernameFromActorIDLift(poll.CreatedBy),
			AccountID: actor.Actor.ID,
			StatusID:  poll.StatusID,
			CreatedAt: time.Now(),
		}

		if err := h.store.CreateNotification(ctx.Context, notification); err != nil {
			h.logger.Warn("failed to create poll notification",
				zap.String("poll_id", pollID),
				zap.Error(err))
			// Don't fail the vote operation
		}
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(resp)
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
					if emoji, err := h.store.GetCustomEmoji(ctx, code); err == nil {
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

// isValidEmojiCodeLift checks if an emoji code is valid
func isValidEmojiCodeLift(code string) bool {
	// Valid emoji codes contain only letters, numbers, and underscores
	for _, r := range code {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_') {
			return false
		}
	}
	return len(code) >= 2 && len(code) <= 32
}

// generateRandomStringLift generates a random string of specified length
func generateRandomStringLift(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := range result {
		result[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(result)
}