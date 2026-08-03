package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/emoji"
	"github.com/equaltoai/lesser/pkg/storage"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	"go.uber.org/zap"
)

// extractUsernameFromContext extracts the username from auth header
func (h *Handler) extractUsernameFromContext(ctx *apptheory.Context) string {
	return h.getOptionalAuthenticatedUser(ctx)
}

// HandleGetAnnouncementsLift handles GET /api/v1/announcements
func (h *Handler) HandleGetAnnouncementsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Extract token (optional for public announcements)
	username := h.extractUsernameFromContext(ctx)

	// Get active announcements
	announcements, err := h.repos.Announcement().GetAnnouncements(ctx.Context(), true) // Only active announcements
	if err != nil {
		h.logger.Error("failed to get announcements", zap.Error(err))
		return common.RespondInternalServerError(ctx, "Internal server error")
	}

	// Get dismissed announcements for authenticated user
	dismissedIDs := make(map[string]bool)
	if username != "" {
		dismissed, err := h.repos.Announcement().GetDismissedAnnouncements(ctx.Context(), username)
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
		apiReactions := h.buildAnnouncementReactions(ctx, announcement.ID, username)

		// Merge reactions
		announcement.Reactions = h.mergeReactions(announcement.Reactions, apiReactions)

		// Extract content elements from the announcement
		mentions := h.extractAnnouncementMentions(ctx.Context(), announcement.Content)
		tags := h.extractAnnouncementTags(announcement.Content)
		emojis := h.extractAnnouncementEmojis(ctx.Context(), announcement.Content)
		statuses := h.extractAnnouncementStatuses(ctx.Context(), announcement.Content)

		apiAnnouncement := models.Announcement{
			ID:          announcement.ID,
			Content:     announcement.Content,
			Text:        announcement.Text,
			PublishedAt: announcement.PublishedAt.Format(time.RFC3339),
			UpdatedAt:   announcement.UpdatedAt.Format(time.RFC3339),
			AllDay:      announcement.AllDay,
			Read:        false, // Not dismissed
			Reactions:   convertReactionsToAPILift(announcement.Reactions),
			Mentions:    mentions,
			Statuses:    statuses,
			Tags:        tags,
			Emojis:      emojis,
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

	return okJSON(apiAnnouncements)
}

// buildAnnouncementReactions builds the reactions for an announcement
func (h *Handler) buildAnnouncementReactions(ctx *apptheory.Context, announcementID string, username string) []models.AnnouncementReaction {
	reactions, err := h.repos.Announcement().GetAnnouncementReactions(ctx.Context(), announcementID)
	if err != nil {
		h.logger.Warn("failed to get announcement reactions",
			zap.String("announcement_id", announcementID),
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
			emoji, err := h.repos.Emoji().GetCustomEmoji(ctx.Context(), shortcode)
			if err == nil && emoji != nil && !emoji.Disabled {
				reaction.URL = emoji.URL
				reaction.StaticURL = emoji.StaticURL
			}
		}

		apiReactions = append(apiReactions, reaction)
	}
	return apiReactions
}

// mergeReactions merges actual reactions with available reactions
func (h *Handler) mergeReactions(availableReactions []storage.Reaction, actualReactions []models.AnnouncementReaction) []storage.Reaction {
	// Set up default reactions if none are specified
	if err := common.ValidateSliceNotEmpty("available_reactions", availableReactions); err != nil {
		availableReactions = []storage.Reaction{
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
	for i, availableReaction := range availableReactions {
		for _, actualReaction := range actualReactions {
			if availableReaction.Name == actualReaction.Name {
				availableReactions[i].Count = actualReaction.Count
				availableReactions[i].Me = actualReaction.Me
				break
			}
		}
	}

	return availableReactions
}

// HandleDismissAnnouncementLift handles POST /api/v1/announcements/:id/dismiss
func (h *Handler) HandleDismissAnnouncementLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	announcementID := ctx.Param("id")
	if err := common.ValidateRequiredParam("announcement_id", announcementID); err != nil {
		return common.RespondBadRequest(ctx, "Announcement ID is required")
	}

	claims, resp, err := h.authenticatedClaimsWithResponder(
		ctx,
		func(ctx *apptheory.Context) (*apptheory.Response, error) { return common.RespondUnauthorized(ctx) },
		func(ctx *apptheory.Context) (*apptheory.Response, error) { return common.RespondUnauthorized(ctx) },
	)
	if resp != nil || err != nil {
		return resp, err
	}

	// Check if announcement exists
	announcement, err := h.repos.Announcement().GetAnnouncement(ctx.Context(), announcementID)
	if err != nil || announcement == nil {
		return common.RespondNotFound(ctx, "Announcement not found")
	}

	// Dismiss the announcement
	err = h.repos.Announcement().DismissAnnouncement(ctx.Context(), claims.Username, announcementID)
	if err != nil {
		h.logger.Error("failed to dismiss announcement",
			zap.String("announcement_id", announcementID),
			zap.String("username", claims.Username),
			zap.Error(err))
		return common.RespondInternalServerError(ctx, "Internal server error")
	}

	return okJSON(models.EmptyObject{})
}

// HandleAddAnnouncementReactionLift handles PUT /api/v1/announcements/:id/reactions/:name
func (h *Handler) HandleAddAnnouncementReactionLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	return h.handleAnnouncementReaction(ctx, "add")
}

// HandleRemoveAnnouncementReactionLift handles DELETE /api/v1/announcements/:id/reactions/:name
func (h *Handler) HandleRemoveAnnouncementReactionLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	return h.handleAnnouncementReaction(ctx, "remove")
}

// handleAnnouncementReaction consolidates the common logic for adding/removing announcement reactions
func (h *Handler) handleAnnouncementReaction(ctx *apptheory.Context, action string) (*apptheory.Response, error) {
	announcementID := ctx.Param("id")
	reactionName := ctx.Param("name")

	if err := common.ValidateRequiredParam("announcement_id", announcementID); err != nil {
		return common.RespondBadRequest(ctx, "Announcement ID is required")
	}
	if err := common.ValidateRequiredParam("reaction_name", reactionName); err != nil {
		return common.RespondBadRequest(ctx, "Reaction name is required")
	}

	claims, resp, err := h.authenticatedClaimsWithResponder(
		ctx,
		func(ctx *apptheory.Context) (*apptheory.Response, error) { return common.RespondUnauthorized(ctx) },
		func(ctx *apptheory.Context) (*apptheory.Response, error) { return common.RespondUnauthorized(ctx) },
	)
	if resp != nil || err != nil {
		return resp, err
	}

	// Check if announcement exists
	announcement, err := h.repos.Announcement().GetAnnouncement(ctx.Context(), announcementID)
	if err != nil || announcement == nil {
		return common.RespondNotFound(ctx, "Announcement not found")
	}

	// Perform the appropriate action
	if action == "add" {
		err = h.repos.Announcement().AddAnnouncementReaction(ctx.Context(), claims.Username, announcementID, reactionName)
	} else {
		err = h.repos.Announcement().RemoveAnnouncementReaction(ctx.Context(), claims.Username, announcementID, reactionName)
	}

	if err != nil {
		h.logger.Error(fmt.Sprintf("failed to %s announcement reaction", action),
			zap.String("announcement_id", announcementID),
			zap.String("username", claims.Username),
			zap.String("reaction", reactionName),
			zap.Error(err))
		return common.RespondInternalServerError(ctx, "Internal server error")
	}

	// Return the announcement with updated reactions
	return h.HandleGetAnnouncementsLift(ctx)
}

// HandleCreateAnnouncementLift handles POST /api/v1/admin/announcements
func (h *Handler) HandleCreateAnnouncementLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Parse request
	var req models.CreateAnnouncementRequest
	body := ctx.Request.Body
	if err := json.Unmarshal(body, &req); err != nil {
		return common.RespondBadRequest(ctx, "Invalid request body")
	}

	// Validate required fields
	if err := common.ValidateRequiredParam("text", req.Text); err != nil {
		return common.RespondUnprocessableEntity(ctx, "Text is required")
	}

	// Require admin authentication
	if _, err := h.requireAdminLift(ctx); err != nil {
		return common.RespondForbidden(ctx, "Admin access required")
	}

	// Create announcement
	announcement := &storage.Announcement{
		ID:          time.Now().Format("20060102150405"),
		Content:     req.Text, // HTML content
		Text:        req.Text, // Plain text
		AllDay:      req.AllDay,
		PublishedAt: time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Parse optional dates
	if req.StartsAt != "" {
		if err := common.ValidateTimestamp(req.StartsAt, "starts_at"); err != nil {
			return common.RespondUnprocessableEntity(ctx, err.Error())
		}
		startsAt, _ := time.Parse(time.RFC3339, req.StartsAt) // Validation passed, parse is safe
		announcement.StartsAt = &startsAt
	}

	if req.EndsAt != "" {
		if err := common.ValidateTimestamp(req.EndsAt, "ends_at"); err != nil {
			return common.RespondUnprocessableEntity(ctx, err.Error())
		}
		endsAt, _ := time.Parse(time.RFC3339, req.EndsAt) // Validation passed, parse is safe
		announcement.EndsAt = &endsAt
	}

	// Store announcement
	if err := h.repos.Announcement().CreateAnnouncement(ctx.Context(), announcement); err != nil {
		h.logger.Error("failed to create announcement", zap.Error(err))
		return common.RespondInternalServerError(ctx, "Failed to create announcement")
	}

	// Return created announcement
	return apptheory.JSON(201, announcement)
}

// convertReactionsToAPILift converts storage reactions to API format
func convertReactionsToAPILift(reactions []storage.Reaction) []models.AnnouncementReaction {
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

// extractAnnouncementMentions extracts @mentions from announcement content
func (h *Handler) extractAnnouncementMentions(ctx context.Context, content string) []models.AnnouncementAccount {
	mentions := []models.AnnouncementAccount{}
	words := strings.Fields(content)

	for _, word := range words {
		if strings.HasPrefix(word, "@") {
			// Remove @ and any trailing punctuation
			username := strings.TrimPrefix(word, "@")
			username = strings.TrimRight(username, ".,!?;:")

			if username != "" {
				// Get user details from storage
				user, err := h.repos.User().GetUser(ctx, username)
				if err != nil {
					h.logger.Debug("mentioned user not found", zap.String("username", username))
					continue
				}

				// Convert to announcement account format
				mention := models.AnnouncementAccount{
					ID:       user.ID,
					Username: user.Username,
					URL:      fmt.Sprintf("%s/users/%s", h.cfg.BaseURL(), user.Username),
					Acct:     user.Username, // For local users, acct is just username
				}
				mentions = append(mentions, mention)
			}
		}
	}

	return mentions
}

// extractAnnouncementTags extracts #hashtags from announcement content
func (h *Handler) extractAnnouncementTags(content string) []models.AnnouncementTag {
	tags := []models.AnnouncementTag{}
	words := strings.Fields(content)

	for _, word := range words {
		if strings.HasPrefix(word, "#") && len(word) > 1 {
			hashtag := strings.TrimPrefix(word, "#")
			hashtag = strings.TrimRight(hashtag, ".,!?;:")
			hashtag = strings.ToLower(hashtag)

			if hashtag != "" {
				tag := models.AnnouncementTag{
					Name: hashtag,
					URL:  fmt.Sprintf("%s/tags/%s", h.cfg.BaseURL(), hashtag),
				}
				tags = append(tags, tag)
			}
		}
	}

	return tags
}

// extractAnnouncementEmojis extracts custom emojis from announcement content
func (h *Handler) extractAnnouncementEmojis(ctx context.Context, content string) []models.CustomEmoji {
	// Create emoji parser
	emojiParser := emoji.NewParser(h.repos, h.logger)

	// Parse emojis from content
	parsed, err := emojiParser.ParseAll(ctx, content)
	if err != nil {
		h.logger.Warn("failed to parse emojis from announcement", zap.Error(err))
		return []models.CustomEmoji{}
	}

	// Convert to API format
	emojis := make([]models.CustomEmoji, 0, len(parsed.CustomEmojis))
	for _, parsedEmoji := range parsed.CustomEmojis {
		if parsedEmoji.Emoji != nil {
			emoji := models.CustomEmoji{
				Shortcode:       parsedEmoji.Emoji.Shortcode,
				URL:             parsedEmoji.Emoji.URL,
				StaticURL:       parsedEmoji.Emoji.StaticURL,
				VisibleInPicker: parsedEmoji.Emoji.VisibleInPicker,
				Category:        parsedEmoji.Emoji.Category,
			}
			emojis = append(emojis, emoji)
		}
	}

	return emojis
}

// statusURLRegex matches various status/post URL formats
var statusURLRegex = regexp.MustCompile(`https?://[^/\s]+(?:/api/v1)?/statuses/([a-zA-Z0-9_-]+)`)

// extractAnnouncementStatuses extracts status/post references from announcement content
func (h *Handler) extractAnnouncementStatuses(ctx context.Context, content string) []models.AnnouncementStatus {
	statuses := []models.AnnouncementStatus{}

	// Find all status URLs in the content
	matches := statusURLRegex.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		if len(match) >= 2 {
			statusID := match[1]
			fullURL := match[0]

			// Verify the status exists
			_, err := h.repos.Status().GetStatus(ctx, statusID)
			if err != nil {
				h.logger.Debug("referenced status not found", zap.String("status_id", statusID))
				continue
			}

			status := models.AnnouncementStatus{
				ID:  statusID,
				URL: fullURL,
			}
			statuses = append(statuses, status)
		}
	}

	return statuses
}
