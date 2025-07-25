package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aron23/lesser/cmd/api/models"
	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// HandleGetAnnouncements handles GET /api/v1/announcements
func (h *Handler) HandleGetAnnouncements(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token (optional for public announcements)
	var username string
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	if authHeader != "" {
		token, err := auth.ExtractBearerToken(authHeader)
		if err == nil {
			// Validate token
			oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
			claims, err := oauthSvc.ValidateAccessToken(token)
			if err == nil {
				username = claims.Username
			}
		}
	}

	// Get active announcements
	announcements, err := h.store.GetAnnouncements(ctx, true) // Only active announcements
	if err != nil {
		h.logger.Error("failed to get announcements", zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to get announcements")), nil
	}

	// Get dismissed announcements for authenticated user
	dismissedIDs := make(map[string]bool)
	if username != "" {
		dismissed, err := h.store.GetDismissedAnnouncements(ctx, username)
		if err != nil {
			h.logger.Warn("failed to get dismissed announcements",
				zap.String("username", username),
				zap.Error(err))
		} else {
			for _, id := range dismissed {
				dismissedIDs[id] = true
			}
		}
	}

	// Convert to API format
	apiAnnouncements := make([]models.Announcement, 0, len(announcements))
	for _, announcement := range announcements {
		// Skip dismissed announcements
		if dismissedIDs[announcement.ID] {
			continue
		}

		// Get reactions for this announcement
		reactions, err := h.store.GetAnnouncementReactions(ctx, announcement.ID)
		if err != nil {
			h.logger.Warn("failed to get announcement reactions",
				zap.String("announcement_id", announcement.ID),
				zap.Error(err))
			reactions = make(map[string][]string)
		}

		// Convert reactions to API format
		apiReactions := make([]models.AnnouncementReaction, 0)
		for emojiName, users := range reactions {
			// Check if current user reacted
			me := false
			if username != "" {
				for _, user := range users {
					if user == username {
						me = true
						break
					}
				}
			}

			reaction := models.AnnouncementReaction{
				Name:  emojiName,
				Count: len(users),
				Me:    me,
			}

			// Check if it's a custom emoji (starts with :)
			if strings.HasPrefix(emojiName, ":") && strings.HasSuffix(emojiName, ":") {
				// Extract shortcode without colons
				shortcode := strings.TrimPrefix(strings.TrimSuffix(emojiName, ":"), ":")

				// Look up custom emoji
				emoji, err := h.store.GetCustomEmoji(ctx, shortcode)
				if err == nil && emoji != nil && !emoji.Disabled {
					reaction.URL = emoji.URL
					reaction.StaticURL = emoji.StaticURL
				}
			}

			apiReactions = append(apiReactions, reaction)
		}

		// Set up available reactions if none are specified
		if len(announcement.Reactions) == 0 {
			// Default reactions
			announcement.Reactions = []storage.Reaction{
				{Name: "👍", Count: 0, Me: false},
				{Name: "👎", Count: 0, Me: false},
				{Name: "😄", Count: 0, Me: false},
				{Name: "🎉", Count: 0, Me: false},
				{Name: "😕", Count: 0, Me: false},
				{Name: "❤️", Count: 0, Me: false},
				{Name: "🚀", Count: 0, Me: false},
				{Name: "👀", Count: 0, Me: false},
			}
		}

		// Merge actual reactions with available reactions
		for i, availableReaction := range announcement.Reactions {
			for _, actualReaction := range apiReactions {
				if availableReaction.Name == actualReaction.Name {
					announcement.Reactions[i].Count = actualReaction.Count
					announcement.Reactions[i].Me = actualReaction.Me
					break
				}
			}
		}

		apiAnnouncement := models.Announcement{
			ID:          announcement.ID,
			Content:     announcement.Content,
			Text:        announcement.Text,
			PublishedAt: announcement.PublishedAt.Format(time.RFC3339),
			UpdatedAt:   announcement.UpdatedAt.Format(time.RFC3339),
			AllDay:      announcement.AllDay,
			Read:        false, // Not dismissed
			Reactions:   convertReactionsToAPI(announcement.Reactions),
			Mentions:    []models.AnnouncementAccount{}, // Placeholder
			Statuses:    []models.AnnouncementStatus{},  // Placeholder
			Tags:        []models.AnnouncementTag{},     // Placeholder
			Emojis:      []models.CustomEmoji{},         // Placeholder
		}

		// Add optional fields
		if announcement.StartsAt != nil {
			startsAt := announcement.StartsAt.Format(time.RFC3339)
			apiAnnouncement.StartsAt = &startsAt
		}
		if announcement.EndsAt != nil {
			endsAt := announcement.EndsAt.Format(time.RFC3339)
			apiAnnouncement.EndsAt = &endsAt
		}

		apiAnnouncements = append(apiAnnouncements, apiAnnouncement)
	}

	return common.OK(apiAnnouncements), nil
}

// HandleDismissAnnouncement handles POST /api/v1/announcements/:id/dismiss
func (h *Handler) HandleDismissAnnouncement(ctx context.Context, request events.APIGatewayV2HTTPRequest, announcementID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract and validate token
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check if announcement exists
	_, err = h.store.GetAnnouncement(ctx, announcementID)
	if err != nil {
		if err == storage.ErrNotFound {
			return common.NotFound(fmt.Errorf("announcement not found")), nil
		}
		h.logger.Error("failed to get announcement",
			zap.String("announcement_id", announcementID),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to get announcement")), nil
	}

	// Dismiss the announcement
	err = h.store.DismissAnnouncement(ctx, claims.Username, announcementID)
	if err != nil {
		h.logger.Error("failed to dismiss announcement",
			zap.String("username", claims.Username),
			zap.String("announcement_id", announcementID),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to dismiss announcement")), nil
	}

	// Return empty object
	return common.OK(map[string]any{}), nil
}

// HandleAddAnnouncementReaction handles PUT /api/v1/announcements/:id/reactions/:name
func (h *Handler) HandleAddAnnouncementReaction(ctx context.Context, request events.APIGatewayV2HTTPRequest, announcementID, reactionName string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract and validate token
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check if announcement exists
	announcement, err := h.store.GetAnnouncement(ctx, announcementID)
	if err != nil {
		if err == storage.ErrNotFound {
			return common.NotFound(fmt.Errorf("announcement not found")), nil
		}
		h.logger.Error("failed to get announcement",
			zap.String("announcement_id", announcementID),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to get announcement")), nil
	}

	// Check if reaction is allowed
	reactionAllowed := false
	if len(announcement.Reactions) == 0 {
		// If no reactions specified, allow common emojis
		commonReactions := []string{"👍", "👎", "😄", "🎉", "😕", "❤️", "🚀", "👀"}
		for _, r := range commonReactions {
			if r == reactionName {
				reactionAllowed = true
				break
			}
		}
	} else {
		// Check if reaction is in allowed list
		for _, r := range announcement.Reactions {
			if r.Name == reactionName {
				reactionAllowed = true
				break
			}
		}
	}

	if !reactionAllowed {
		return common.UnprocessableEntity(fmt.Errorf("reaction not allowed")), nil
	}

	// Add the reaction
	err = h.store.AddAnnouncementReaction(ctx, claims.Username, announcementID, reactionName)
	if err != nil {
		h.logger.Error("failed to add announcement reaction",
			zap.String("username", claims.Username),
			zap.String("announcement_id", announcementID),
			zap.String("reaction", reactionName),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to add reaction")), nil
	}

	// Return empty object
	return common.OK(map[string]any{}), nil
}

// HandleRemoveAnnouncementReaction handles DELETE /api/v1/announcements/:id/reactions/:name
func (h *Handler) HandleRemoveAnnouncementReaction(ctx context.Context, request events.APIGatewayV2HTTPRequest, announcementID, reactionName string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract and validate token
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check if announcement exists
	_, err = h.store.GetAnnouncement(ctx, announcementID)
	if err != nil {
		if err == storage.ErrNotFound {
			return common.NotFound(fmt.Errorf("announcement not found")), nil
		}
		h.logger.Error("failed to get announcement",
			zap.String("announcement_id", announcementID),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to get announcement")), nil
	}

	// Remove the reaction
	err = h.store.RemoveAnnouncementReaction(ctx, claims.Username, announcementID, reactionName)
	if err != nil {
		h.logger.Error("failed to remove announcement reaction",
			zap.String("username", claims.Username),
			zap.String("announcement_id", announcementID),
			zap.String("reaction", reactionName),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to remove reaction")), nil
	}

	// Return empty object
	return common.OK(map[string]any{}), nil
}

// Helper function to convert storage reactions to API format
func convertReactionsToAPI(reactions []storage.Reaction) []models.AnnouncementReaction {
	apiReactions := make([]models.AnnouncementReaction, len(reactions))
	for i, r := range reactions {
		apiReactions[i] = models.AnnouncementReaction{
			Name:      r.Name,
			Count:     r.Count,
			Me:        r.Me,
			URL:       r.URL,
			StaticURL: r.StaticURL,
		}
	}
	return apiReactions
}

// Admin endpoints for creating/managing announcements would go here
// These would require admin authentication

// HandleCreateAnnouncement handles POST /api/v1/admin/announcements (admin only)
func (h *Handler) HandleCreateAnnouncement(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract and validate token
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check admin role
	user, err := h.store.GetUser(ctx, claims.Username)
	if err != nil || user.Role != "admin" {
		return common.Forbidden(fmt.Errorf("admin access required")), nil
	}

	// Parse request
	var req models.CreateAnnouncementRequest
	if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
		return common.BadRequest(err), nil
	}

	// Validate required fields
	if req.Content == "" {
		return common.UnprocessableEntity(fmt.Errorf("content is required")), nil
	}

	// Create announcement
	announcement := &storage.Announcement{
		Content:   req.Content,
		Text:      req.Text,
		AllDay:    req.AllDay,
		CreatedBy: claims.Username,
	}

	// Parse optional dates
	if req.StartsAt != "" {
		startsAt, err := time.Parse(time.RFC3339, req.StartsAt)
		if err != nil {
			return common.UnprocessableEntity(fmt.Errorf("invalid starts_at format")), nil
		}
		announcement.StartsAt = &startsAt
	}

	if req.EndsAt != "" {
		endsAt, err := time.Parse(time.RFC3339, req.EndsAt)
		if err != nil {
			return common.UnprocessableEntity(fmt.Errorf("invalid ends_at format")), nil
		}
		announcement.EndsAt = &endsAt
	}

	// Create the announcement
	if err := h.store.CreateAnnouncement(ctx, announcement); err != nil {
		h.logger.Error("failed to create announcement",
			zap.String("admin", claims.Username),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to create announcement")), nil
	}

	// Return the created announcement
	apiAnnouncement := models.Announcement{
		ID:          announcement.ID,
		Content:     announcement.Content,
		Text:        announcement.Text,
		PublishedAt: announcement.PublishedAt.Format(time.RFC3339),
		UpdatedAt:   announcement.UpdatedAt.Format(time.RFC3339),
		AllDay:      announcement.AllDay,
		Read:        false,
		Reactions:   []models.AnnouncementReaction{},
		Mentions:    []models.AnnouncementAccount{},
		Statuses:    []models.AnnouncementStatus{},
		Tags:        []models.AnnouncementTag{},
		Emojis:      []models.CustomEmoji{},
	}

	if announcement.StartsAt != nil {
		startsAt := announcement.StartsAt.Format(time.RFC3339)
		apiAnnouncement.StartsAt = &startsAt
	}
	if announcement.EndsAt != nil {
		endsAt := announcement.EndsAt.Format(time.RFC3339)
		apiAnnouncement.EndsAt = &endsAt
	}

	return common.OK(apiAnnouncement), nil
}
