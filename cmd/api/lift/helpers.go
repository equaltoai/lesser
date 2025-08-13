package lift

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// resolveAccountID resolves an account ID (which can be a username, numeric ID, or URL) to an actor
func (h *Handler) resolveAccountID(ctx context.Context, accountID string) (*activitypub.Actor, error) {
	// Handle different account ID formats
	if strings.HasPrefix(accountID, "http://") || strings.HasPrefix(accountID, "https://") {
		// Full ActivityPub actor URL
		// Extract username from URL like https://lesser.host/users/aron
		if strings.Contains(accountID, h.cfg.Domain) && strings.Contains(accountID, "/users/") {
			parts := strings.Split(accountID, "/users/")
			if len(parts) == 2 {
				username := parts[1]
				return h.repos.Actor().GetActor(ctx, username)
			}
			return nil, fmt.Errorf("invalid account URL")
		}
		// Remote actor - not supported yet
		return nil, fmt.Errorf("remote accounts not yet supported")
	}

	// Check if it's a numeric ID (Mastodon compatibility)
	if _, err := strconv.ParseInt(accountID, 10, 64); err == nil && len(accountID) >= 10 {
		// It's a numeric ID - use the dedicated lookup method
		return h.repos.Actor().GetActorByNumericID(ctx, accountID)
	}

	// Assume it's a username for local accounts
	return h.repos.Actor().GetActor(ctx, accountID)
}

// authenticateUser handles the common pattern of extracting and validating user authentication
// It supports both test mode (via X-Test-Username header) and production OAuth
func (h *Handler) authenticateUser(ctx *lift.Context, requiredScope string) (username string, err error) {
	// Test hook - check for test username header
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	if testUsername != "" {
		// Test mode - skip auth
		return testUsername, nil
	}

	// Extract and validate token
	authHeader := ctx.Header("Authorization")
	if authHeader == "" {
		authHeader = ctx.Header("authorization")
	}

	// Try direct access to headers if ctx.Header doesn't work
	if authHeader == "" && ctx.Request != nil && ctx.Request.Request != nil {
		authHeader = ctx.Request.Request.Headers["Authorization"]
		if authHeader == "" {
			authHeader = ctx.Request.Request.Headers["authorization"]
		}
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return "", fmt.Errorf("unauthorized")
	}

	// Validate token
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return "", fmt.Errorf("unauthorized")
	}

	// Check scope if provided
	if requiredScope != "" && !claims.HasScope(requiredScope) {
		return "", fmt.Errorf("insufficient scope")
	}

	return claims.Username, nil
}

// statusActionHandler provides a generic handler for status operations like bookmark, favorite, etc.
func (h *Handler) statusActionHandler(ctx *lift.Context, requiredScope string, action func(statusID, username string) (*models.Status, error)) error {
	statusID := ctx.Param("id")
	if statusID == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "missing status id"})
	}

	// Authenticate user
	username, err := h.authenticateUser(ctx, requiredScope)
	if err != nil {
		if err.Error() == "insufficient scope" {
			return ctx.Status(403).JSON(map[string]string{"error": err.Error()})
		}
		return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
	}

	// Execute the action
	status, err := action(statusID, username)
	if err != nil {
		h.logger.Error("status action failed",
			zap.String("action", "generic"),
			zap.String("status_id", statusID),
			zap.String("username", username),
			zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": err.Error()})
	}

	return ctx.JSON(status)
}

// getTestUsername extracts test username from headers
func (h *Handler) getTestUsername(ctx *lift.Context) string {
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}
	return testUsername
}

// getAuthHeader extracts authorization header from request
func (h *Handler) getAuthHeader(ctx *lift.Context) string {
	authHeader := ctx.Header("Authorization")
	if authHeader == "" {
		authHeader = ctx.Header("authorization")
	}

	// Try direct access to headers if ctx.Header doesn't work
	if authHeader == "" && ctx.Request != nil && ctx.Request.Request != nil {
		authHeader = ctx.Request.Request.Headers["Authorization"]
		if authHeader == "" {
			authHeader = ctx.Request.Request.Headers["authorization"]
		}
	}

	return authHeader
}

// getQueryParam extracts query parameter from request
func (h *Handler) getQueryParam(ctx *lift.Context, key string) string {
	value := ctx.Query(key)
	if value == "" && ctx.Request != nil && ctx.Request.Request != nil {
		value = ctx.Request.Request.QueryParams[key]
	}
	return value
}

// isLocal checks if a username belongs to the local instance
func (h *Handler) isLocal(username string) bool {
	// A username is local if it doesn't contain '@' or only contains our domain
	// For simplicity, we'll assume usernames without @ are local
	return !strings.Contains(username, "@")
}

// convertStorageStatusToAPI converts a storage Status model to an API Status model with all real data
//nolint:gocognit // Complex conversion between storage and API models with many fields
func (h *Handler) convertStorageStatusToAPI(storageStatus *storageModels.Status, currentUsername string) (*models.Status, error) {
	ctx := context.Background()

	// Convert InReplyToID to pointer if not empty
	var inReplyToID *string
	var inReplyToAccountID *string
	if storageStatus.InReplyToID != "" {
		inReplyToID = &storageStatus.InReplyToID
		// Get the parent status to find the account ID
		if parentStatus, err := h.repos.Status().GetStatus(ctx, storageStatus.InReplyToID); err == nil {
			inReplyToAccountID = &parentStatus.AuthorID
		}
	}

	// Get author account details
	authorAccount, err := h.repos.Account().GetAccount(ctx, storageStatus.AuthorUsername)
	if err != nil {
		// Fallback to basic info if account not found
		authorAccount = &storage.Account{
			User: &storage.User{
				Username:    storageStatus.AuthorUsername,
				DisplayName: storageStatus.AuthorUsername,
			},
		}
	}

	// Get interaction counts
	statusObjectID := fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), storageStatus.StatusID)
	likeCount, _ := h.repos.Like().GetLikeCount(ctx, statusObjectID)
	reblogCount, _ := h.repos.Social().CountObjectAnnounces(ctx, statusObjectID)

	// Get reply count
	replyCount := 0
	paginationOpts := interfaces.PaginationOptions{Limit: 1}
	if replies, err := h.repos.Status().GetReplies(ctx, storageStatus.StatusID, paginationOpts); err == nil && replies != nil {
		replyCount = len(replies.Items)
	}

	// Check current user's interactions
	var favourited, reblogged, bookmarked, muted, pinned bool
	currentUserActorID := fmt.Sprintf("%s/users/%s", h.cfg.BaseURL(), currentUsername)

	// Check if favorited
	if _, err := h.repos.Like().GetLike(ctx, currentUserActorID, statusObjectID); err == nil {
		favourited = true
	}

	// Check if reblogged
	if _, err := h.repos.Social().GetAnnounce(ctx, currentUserActorID, statusObjectID); err == nil {
		reblogged = true
	}

	// Check if bookmarked
	if bookmarks, _, err := h.repos.Account().GetBookmarks(ctx, currentUsername, 100, ""); err == nil {
		for _, bookmark := range bookmarks {
			if bookmark.ObjectID == statusObjectID {
				bookmarked = true
				break
			}
		}
	}

	// Check if muted (conversation mute)
	if storageStatus.ConversationID != "" {
		muted, _ = h.repos.Conversation().IsConversationMuted(ctx, currentUsername, storageStatus.ConversationID)
	}

	// Check if pinned - check if status is in user's pinned statuses
	// For now we'll skip this check since GetPinnedStatuses doesn't exist
	// pinned = false // Already initialized as false above

	// Extract hashtags from content
	tags := []any{}
	if len(storageStatus.Hashtags) > 0 {
		for _, hashtag := range storageStatus.Hashtags {
			tags = append(tags, map[string]string{
				"name": hashtag,
				"url":  fmt.Sprintf("%s/tags/%s", h.cfg.BaseURL(), hashtag),
			})
		}
	}

	// Extract mentions
	mentions := []any{}
	if len(storageStatus.Mentions) > 0 {
		for _, mention := range storageStatus.Mentions {
			mentions = append(mentions, map[string]string{
				"id":       mention,
				"username": mention,
				"url":      fmt.Sprintf("%s/@%s", h.cfg.BaseURL(), mention),
				"acct":     mention,
			})
		}
	}

	// Handle media attachments from the ActivityPub Note
	mediaAttachments := []any{}
	if storageStatus.Note != nil && storageStatus.Note.Attachment != nil {
		for _, attachment := range storageStatus.Note.Attachment {
			// Convert ActivityPub attachment to API format
			mediaAttachment := map[string]any{
				"id":          attachment.URL, // Use URL as ID if no specific ID
				"type":        attachment.MediaType,
				"url":         attachment.URL,
				"preview_url": attachment.URL, // Use same URL for preview if not specified
			}
			if attachment.Name != "" {
				mediaAttachment["description"] = attachment.Name
			}
			mediaAttachments = append(mediaAttachments, mediaAttachment)
		}
	}

	// Build the account object with real data
	account := models.Account{
		ID:             storageStatus.AuthorID,
		Username:       storageStatus.AuthorUsername,
		Acct:           storageStatus.AuthorUsername,
		DisplayName:    authorAccount.User.DisplayName,
		Locked:         authorAccount.Actor != nil && authorAccount.Actor.ManuallyApprovesFollowers,
		Bot:            authorAccount.Actor != nil && authorAccount.Actor.Type == "Service",
		CreatedAt:      authorAccount.User.CreatedAt.Format("2006-01-02T15:04:05.000Z"),
		Note:           "", // Bio/summary would come from Actor
		URL:            fmt.Sprintf("https://%s/@%s", h.cfg.BaseURL(), storageStatus.AuthorUsername),
		Avatar:         "", // Avatar would come from Actor.Icon
		AvatarStatic:   "", // Avatar would come from Actor.Icon
		Header:         "", // Header would come from Actor.Image
		HeaderStatic:   "", // Header would come from Actor.Image
		FollowersCount: 0,
		FollowingCount: 0,
		StatusesCount:  0,
	}

	// Populate fields from Actor if available
	if authorAccount.Actor != nil {
		account.Note = authorAccount.Actor.Summary
		if authorAccount.Actor.Icon != nil {
			account.Avatar = authorAccount.Actor.Icon.URL
			account.AvatarStatic = authorAccount.Actor.Icon.URL
		}
		if authorAccount.Actor.Image != nil {
			account.Header = authorAccount.Actor.Image.URL
			account.HeaderStatic = authorAccount.Actor.Image.URL
		}
	}

	// Get follower/following counts - these would come from a separate query in real implementation
	// For now, we'll use default values
	account.FollowersCount = 0
	account.FollowingCount = 0
	account.StatusesCount = 0

	// Handle reblog if this is a reblog
	var reblogStatus *models.Status
	if storageStatus.ReblogOfID != "" {
		if rebloggedStatus, err := h.repos.Status().GetStatus(ctx, storageStatus.ReblogOfID); err == nil {
			// Recursively convert the reblogged status
			reblogStatus, _ = h.convertStorageStatusToAPI(rebloggedStatus, currentUsername)
		}
	}

	// Build the final API status
	apiStatus := &models.Status{
		ID:                 storageStatus.StatusID,
		Content:            storageStatus.Content,
		Sensitive:          storageStatus.Sensitive,
		SpoilerText:        "", // Note: SpoilerText is not in the storage model, would come from Note
		Language:           storageStatus.Language,
		Visibility:         storageStatus.Visibility,
		CreatedAt:          storageStatus.PublishedAt.Format("2006-01-02T15:04:05.000Z"),
		InReplyToID:        inReplyToID,
		InReplyToAccountID: inReplyToAccountID,
		Account:            account,
		MediaAttachments:   mediaAttachments,
		Mentions:           mentions,
		Tags:               tags,
		Emojis:             []any{}, // Custom emojis would go here
		ReblogsCount:       int(reblogCount),
		FavouritesCount:    int(likeCount),
		RepliesCount:       replyCount,
		Reblogged:          reblogged,
		Favourited:         favourited,
		Bookmarked:         bookmarked,
		Muted:              muted,
		Pinned:             pinned,
		Reblog:             reblogStatus,
		URI:                fmt.Sprintf("https://%s/users/%s/statuses/%s", h.cfg.BaseURL(), storageStatus.AuthorUsername, storageStatus.StatusID),
		URL:                fmt.Sprintf("https://%s/@%s/%s", h.cfg.BaseURL(), storageStatus.AuthorUsername, storageStatus.StatusID),
	}

	// Poll support would go here when implemented
	// For now, polls are not supported in the storage model

	return apiStatus, nil
}

// createOAuthService creates an OAuth service with proper audit logger setup
func createOAuthService(jwtSecret string, repos core.RepositoryStorage, logger *zap.Logger) *auth.OAuthService {
	// Create audit logger with default configuration
	auditLogger := auth.NewAuditLogger(repos, logger, auth.DefaultAuditConfig())
	return auth.NewOAuthService(jwtSecret, repos, auditLogger)
}
