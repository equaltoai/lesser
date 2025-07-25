package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aron23/lesser/cmd/api/models"
	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// HandleGetPoll retrieves a poll by ID
func (h *Handler) HandleGetPoll(ctx context.Context, request events.APIGatewayV2HTTPRequest, pollID string) (*events.APIGatewayV2HTTPResponse, error) {
	log := common.WithContext(ctx)

	// Extract token from Authorization header (optional for public polls)
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	var userID string
	if authHeader != "" {
		token, err := auth.ExtractBearerToken(authHeader)
		if err == nil {
			oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
			claims, err := oauthSvc.ValidateAccessToken(token)
			if err == nil {
				// Get the user's actor to get their ID
				actor, err := h.store.GetActor(ctx, claims.Username)
				if err == nil {
					userID = actor.ID
				}
			}
		}
	}

	// Get the poll
	poll, err := h.store.GetPoll(ctx, pollID)
	if err != nil {
		log.Error("failed to get poll", zap.String("poll_id", pollID), zap.Error(err))
		return common.NotFound(fmt.Errorf("poll not found")), nil
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
		Emojis:      h.extractCustomEmojis(ctx, poll.Options),
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

	body, _ := json.Marshal(resp)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}, nil
}

// HandleVoteOnPoll submits a vote on a poll
func (h *Handler) HandleVoteOnPoll(ctx context.Context, request events.APIGatewayV2HTTPRequest, pollID string) (*events.APIGatewayV2HTTPResponse, error) {
	log := common.WithContext(ctx)

	// Extract token from Authorization header
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(fmt.Errorf("insufficient scope")), nil
	}

	// Get the user's actor
	actor, err := h.store.GetActor(ctx, claims.Username)
	if err != nil {
		log.Error("failed to get actor", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Parse request
	var req models.PollVoteRequest
	if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
		return common.BadRequest(err), nil
	}

	// Validate request
	if len(req.Choices) == 0 {
		return common.UnprocessableEntity(fmt.Errorf("no choices provided")), nil
	}

	// Submit vote
	if err := h.store.VoteOnPoll(ctx, pollID, actor.ID, req.Choices); err != nil {
		log.Error("failed to vote on poll",
			zap.String("poll_id", pollID),
			zap.String("voter_id", actor.ID),
			zap.Error(err))
		return common.UnprocessableEntity(err), nil
	}

	// Get updated poll data to return
	poll, err := h.store.GetPoll(ctx, pollID)
	if err != nil {
		log.Error("failed to get poll after voting", zap.String("poll_id", pollID), zap.Error(err))
		return common.InternalServerError(err), nil
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
		Emojis:      h.extractCustomEmojis(ctx, poll.Options),
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
	if poll.CreatedBy != actor.ID {
		notification := &storage.Notification{
			ID:        fmt.Sprintf("%d-%s", time.Now().Unix(), generateRandomString(8)),
			Type:      "poll",
			Username:  extractUsernameFromActorID(poll.CreatedBy),
			AccountID: actor.ID,
			StatusID:  poll.StatusID,
			CreatedAt: time.Now(),
		}

		if err := h.store.CreateNotification(ctx, notification); err != nil {
			log.Warn("failed to create poll notification",
				zap.String("poll_id", pollID),
				zap.Error(err))
			// Don't fail the vote operation
		}
	}

	body, _ := json.Marshal(resp)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}, nil
}

// extractUsernameFromActorID extracts the username from an actor ID URL
func extractUsernameFromActorID(actorID string) string {
	// Actor ID format: https://example.com/users/username
	parts := strings.Split(actorID, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}

// extractCustomEmojis extracts custom emojis from poll options
func (h *Handler) extractCustomEmojis(ctx context.Context, options []string) []any {
	emojis := make([]any, 0)
	emojiMap := make(map[string]bool) // To avoid duplicates

	for _, option := range options {
		// Look for custom emoji patterns like :custom_emoji:
		if emojiCodes := h.findEmojiCodes(option); len(emojiCodes) > 0 {
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

// findEmojiCodes finds custom emoji codes in text
func (h *Handler) findEmojiCodes(text string) []string {
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
		if len(code) > 0 && isValidEmojiCode(code) {
			codes = append(codes, code)
		}

		start = endIdx + 1
	}

	return codes
}

// isValidEmojiCode checks if an emoji code is valid
func isValidEmojiCode(code string) bool {
	// Valid emoji codes contain only letters, numbers, and underscores
	for _, r := range code {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_') {
			return false
		}
	}
	return len(code) >= 2 && len(code) <= 32
}

// generateRandomString is defined in statuses.go to avoid duplication
