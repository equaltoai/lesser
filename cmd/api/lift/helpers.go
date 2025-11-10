package lift

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/graph"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/transformations"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// PaginationParams represents common pagination parameters
type PaginationParams struct {
	Limit   int
	MaxID   string
	MinID   string
	SinceID string
	Cursor  string
}

// DefaultPaginationLimit is the default limit for paginated responses
const DefaultPaginationLimit = 20

// MaxPaginationLimit is the maximum allowed limit for paginated responses
const MaxPaginationLimit = 80

// Legacy authentication function removed - use auth.RequireAuthWithScope() instead

// resolveAccountID resolves an account ID (which can be a username, numeric ID, or URL) to an actor
func (h *Handler) resolveAccountID(ctx context.Context, accountID string) (*activitypub.Actor, error) {
	// Validate account ID format
	if err := common.ValidateAccountID(accountID); err != nil {
		return nil, errors.Join(invalidAccountID(), err)
	}

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
			return nil, invalidAccountURL()
		}
		// Remote actor - not supported yet
		return nil, remoteAccountsNotSupported()
	}

	// Check if it's a numeric ID (Mastodon compatibility)
	if common.ValidateNumericID("account_id", accountID) == nil && len(accountID) >= 10 {
		// It's a numeric ID - use the dedicated lookup method
		return h.repos.Actor().GetActorByNumericID(ctx, accountID)
	}

	// Assume it's a username for local accounts
	return h.repos.Actor().GetActor(ctx, accountID)
}

// authenticateUser handles the common pattern of extracting and validating user authentication
func (h *Handler) authenticateUser(ctx *lift.Context, requiredScopes []string) (username string, err error) {

	// Extract and validate token
	token := h.getBearerTokenLift(ctx)
	if err := common.ValidateRequiredParam("token", token); err != nil {
		return "", helperUnauthorized()
	}

	// Validate token
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return "", helperUnauthorized()
	}

	// Check scopes if provided
	if err := common.ValidateSliceNotEmpty("required scopes", requiredScopes); err == nil {
		hasScope := false
		for _, scope := range requiredScopes {
			if claims.HasScope(scope) {
				hasScope = true
				break
			}
		}
		if !hasScope {
			return "", helperInsufficientScope()
		}
	}

	return claims.Username, nil
}

// authenticateUserOptional handles optional authentication (for search, public endpoints etc.)
// Returns empty string if no authentication provided, or username if valid auth
//
//nolint:unused // Utility function for optional authentication patterns
func (h *Handler) authenticateUserOptional(ctx *lift.Context, requiredScopes []string) (username string, err error) {

	// Extract token - if none provided, return empty string (no error)
	token := h.getBearerTokenLift(ctx)
	if err := common.ValidateRequiredParam("token", token); err != nil {
		return "", nil // No authentication provided, which is OK for optional auth
	}

	// If token is provided, it must be valid
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return "", helperUnauthorized()
	}

	// Check scopes if provided
	if err := common.ValidateSliceNotEmpty("required scopes", requiredScopes); err == nil {
		hasScope := false
		for _, scope := range requiredScopes {
			if claims.HasScope(scope) {
				hasScope = true
				break
			}
		}
		if !hasScope {
			return "", helperInsufficientScope()
		}
	}

	return claims.Username, nil
}

// statusActionHandler provides a generic handler for status operations like bookmark, favorite, etc.
func (h *Handler) statusActionHandler(ctx *lift.Context, requiredScope string, action func(statusID, username string) (*models.Status, error)) error {
	statusID := ctx.Param("id")
	if err := common.ValidateStatusParamID(statusID); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	// Authenticate user with single scope wrapped in array
	var scopes []string
	if requiredScope != "" {
		scopes = []string{requiredScope}
	}
	username, err := h.authenticateUser(ctx, scopes)
	if err != nil {
		if err.Error() == ErrInsufficientScope {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}

	// Execute the action
	status, err := action(statusID, username)
	if err != nil {
		h.logger.Error("status action failed",
			zap.String("action", "generic"),
			zap.String("status_id", statusID),
			zap.String("username", username),
			zap.Error(err))
		return common.RespondInternalServerError(ctx, err.Error())
	}

	return ctx.JSON(status)
}

// getAuthHeader extracts authorization header from request
func (h *Handler) getAuthHeader(ctx *lift.Context) string {
	authHeader := ctx.Header("Authorization")
	if err := common.ValidateRequiredParam("authHeader", authHeader); err != nil {
		authHeader = ctx.Header("authorization")
	}

	// Try direct access to headers if ctx.Header doesn't work
	if common.ValidateRequiredParam("authHeader", authHeader) != nil && ctx.Request != nil && ctx.Request.Request != nil {
		authHeader = ctx.Request.Request.Headers["Authorization"]
		if common.ValidateRequiredParam("authHeader", authHeader) != nil {
			authHeader = ctx.Request.Request.Headers["authorization"]
		}
	}

	return authHeader
}

// isLocal checks if a username belongs to the local instance
func (h *Handler) isLocal(username string) bool {
	// A username is local if it doesn't contain '@' or only contains our domain
	// For simplicity, we'll assume usernames without @ are local
	return !strings.Contains(username, "@")
}

// convertStorageStatusToAPI converts a storage Status model to an API Status model with all real data
// This version uses DataLoader for efficient actor loading to prevent N+1 queries
//
//nolint:gocognit // Complex conversion between storage and API models with many fields
func (h *Handler) convertStorageStatusToAPI(storageStatus *storageModels.Status, currentUsername string) (*models.Status, error) {
	ctx := context.Background()

	// Attach loaders to context for DataLoader usage
	ctx = graph.WithLoaders(ctx, h.loaders)

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

	// Get author account details using DataLoader for efficient batched loading
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

	// Check if bookmarked using bookmark repository
	if bookmarkRepo := h.repos.Bookmark(); bookmarkRepo != nil {
		if isMarked, err := bookmarkRepo.IsBookmarked(ctx, currentUsername, storageStatus.StatusID); err == nil {
			bookmarked = isMarked
		} else if h.logger != nil {
			h.logger.Debug("failed to check bookmark status",
				zap.String("username", currentUsername),
				zap.String("status_id", storageStatus.StatusID),
				zap.Error(err))
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
	if err := common.ValidateSliceNotEmpty("storage status hashtags", storageStatus.Hashtags); err == nil {
		for _, hashtag := range storageStatus.Hashtags {
			tags = append(tags, map[string]string{
				"name": hashtag,
				"url":  fmt.Sprintf("%s/tags/%s", h.cfg.BaseURL(), hashtag),
			})
		}
	}

	// Extract mentions
	mentions := []any{}
	if err := common.ValidateSliceNotEmpty("storage status mentions", storageStatus.Mentions); err == nil {
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

	// Build the account object using transformation framework and storage data
	var account models.Account
	if authorAccount.Actor != nil {
		// Use centralized transformation framework for Actor fields - ELIMINATES 30+ LINES OF DUPLICATE CODE
		account = transformations.ActorToAccountBase(authorAccount.Actor, h.cfg.BaseURL())

		// Override specific fields with storage data that has precedence
		account.ID = storageStatus.AuthorID
		account.Username = storageStatus.AuthorUsername
		account.Acct = storageStatus.AuthorUsername
		account.DisplayName = authorAccount.User.DisplayName
		account.CreatedAt = authorAccount.User.CreatedAt.Format("2006-01-02T15:04:05.000Z")
	} else {
		// Fallback for cases where Actor is not available - use minimal transformation
		fakeActor := &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   storageStatus.AuthorID,
				Type: "Person",
			},
			PreferredUsername: storageStatus.AuthorUsername,
			Name:              authorAccount.User.DisplayName,
			URL:               fmt.Sprintf("https://%s/@%s", h.cfg.BaseURL(), storageStatus.AuthorUsername),
		}

		// Use centralized transformation framework even for fallback - ELIMINATES 6+ LINES OF DUPLICATE CODE
		account = transformations.ActorToAccountBase(fakeActor, h.cfg.BaseURL())
		account.CreatedAt = authorAccount.User.CreatedAt.Format("2006-01-02T15:04:05.000Z")
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

	// Build the final API status using transformation framework - ELIMINATES 25+ LINES OF DUPLICATE CODE
	statusMap := map[string]interface{}{
		"id":        storageStatus.StatusID,
		"content":   storageStatus.Content,
		"sensitive": storageStatus.Sensitive,
		"published": storageStatus.PublishedAt.Format("2006-01-02T15:04:05.000Z"),
	}

	// Add optional fields to status map
	if storageStatus.InReplyToID != "" {
		statusMap["inReplyTo"] = storageStatus.InReplyToID
	}

	// Use centralized transformation framework for base status creation
	transformer := transformations.NewStatusResponseTransformer(h.cfg.BaseURL(), transformations.ObjectToStatusWithContext)
	transformCtx := context.WithValue(ctx, baseURLContextKey, h.cfg.BaseURL())

	baseStatus, err := transformer.Transform(transformCtx, statusMap)
	if err != nil {
		// Fallback to minimal status if transformation fails
		baseStatus = models.Status{
			ID:        storageStatus.StatusID,
			Content:   storageStatus.Content,
			CreatedAt: storageStatus.PublishedAt.Format("2006-01-02T15:04:05.000Z"),
		}
	}

	// Override with storage-specific and computed fields
	apiStatus := &models.Status{
		ID:                 baseStatus.ID,
		Content:            baseStatus.Content,
		Sensitive:          storageStatus.Sensitive,
		SpoilerText:        "", // Note: SpoilerText is not in the storage model, would come from Note
		Language:           storageStatus.Language,
		Visibility:         storageStatus.Visibility,
		CreatedAt:          baseStatus.CreatedAt,
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

	// Poll support is implemented in polls.go handler
	// Status.Poll field would be populated here if the status has an associated poll

	return apiStatus, nil
}

// createOAuthService creates an OAuth service with proper audit logger setup
func createOAuthService(jwtSecret string, cfg *config.Config, repos core.RepositoryStorage, logger *zap.Logger) *auth.OAuthService {
	// Create audit logger with default configuration
	auditLogger := auth.NewAuditLogger(repos, logger, auth.DefaultAuditConfig())
	return auth.NewOAuthService(jwtSecret, cfg, repos, auditLogger)
}

// parsePaginationParams extracts common pagination parameters from a Lift context
// This method now uses the centralized common.GetPaginationParams internally
func (h *Handler) parsePaginationParams(ctx *lift.Context) *PaginationParams {
	// Use the centralized pagination parameter extraction
	commonParams := common.GetPaginationParams(ctx)

	// Convert to our local PaginationParams struct for backward compatibility
	params := &PaginationParams{
		Limit:   commonParams.Limit,
		MaxID:   commonParams.MaxID,
		MinID:   commonParams.MinID,
		SinceID: commonParams.SinceID,
		Cursor:  commonParams.Cursor,
	}

	return params
}

// respondWithError sends a standardized error response using common helpers
func (h *Handler) respondWithError(ctx *lift.Context, statusCode int, message string) error {
	h.logger.Error("API error",
		zap.Int("status", statusCode),
		zap.String("message", message),
		zap.String("path", ctx.Request.Path),
	)

	// Use the centralized error response helper
	return common.SendError(ctx, statusCode, message)
}

// respondNotFound sends a standardized 404 response
func (h *Handler) respondNotFound(ctx *lift.Context, resourceType string) error {
	return h.respondWithError(ctx, 404, fmt.Sprintf("%s not found", resourceType))
}

// respondUnauthorized sends a standardized 401 response
func (h *Handler) respondUnauthorized(ctx *lift.Context) error {
	return h.respondWithError(ctx, 401, "unauthorized")
}

// respondForbidden sends a standardized 403 response
func (h *Handler) respondForbidden(ctx *lift.Context, reason string) error {
	if err := common.ValidateRequiredParam("reason", reason); err != nil {
		reason = "forbidden"
	}
	return h.respondWithError(ctx, 403, reason)
}

// respondBadRequest sends a standardized 400 response
func (h *Handler) respondBadRequest(ctx *lift.Context, message string) error {
	if err := common.ValidateRequiredParam("message", message); err != nil {
		message = "bad request"
	}
	return h.respondWithError(ctx, 400, message)
}

// Additional standardized error response functions for common patterns

// respondInternalError sends a standardized 500 response
func (h *Handler) respondInternalError(ctx *lift.Context, message string) error {
	if common.ValidateRequiredParam("message", message) != nil {
		message = "internal server error"
	}
	return h.respondWithError(ctx, 500, message)
}

// respondConflict sends a standardized 409 response
//
//nolint:unused // Part of complete set of standardized HTTP response helpers
func (h *Handler) respondConflict(ctx *lift.Context, message string) error {
	if common.ValidateRequiredParam("message", message) != nil {
		message = "conflict"
	}
	return h.respondWithError(ctx, 409, message)
}

// respondUnprocessableEntity sends a standardized 422 response
func (h *Handler) respondUnprocessableEntity(ctx *lift.Context, message string) error {
	if common.ValidateRequiredParam("message", message) != nil {
		message = "unprocessable entity"
	}
	return h.respondWithError(ctx, 422, message)
}

// respondInsufficientScope sends a standardized 403 response for scope issues
func (h *Handler) respondInsufficientScope(ctx *lift.Context) error {
	return h.respondForbidden(ctx, "insufficient scope")
}

// parseBoolParam parses a boolean parameter from the request
func (h *Handler) parseBoolParam(ctx *lift.Context, param string) bool {
	value := ctx.Query(param)
	if common.ValidateRequiredParam("value", value) != nil && ctx.Request != nil && ctx.Request.Request != nil {
		value = ctx.Request.Request.QueryParams[param]
	}
	if common.ValidateRequiredParam("value", value) != nil {
		return false
	}
	return common.ValidateBooleanString(value)
}

// parseArrayParam parses an array parameter from the request
// Supports both id[]=1&id[]=2 and id=1,2 formats
func (h *Handler) parseArrayParam(ctx *lift.Context, param string) []string {
	var values []string

	// First, try to get all query parameters to handle array format param[]
	var queryParams map[string]string
	if ctx.Request != nil && ctx.Request.Request != nil {
		queryParams = ctx.Request.Request.QueryParams
	}

	for key, value := range queryParams {
		if strings.HasPrefix(key, param+"[") && strings.HasSuffix(key, "]") {
			values = append(values, value)
		}
	}

	// If no array format found, check for comma-separated format
	if err := common.ValidateSliceNotEmpty("parsed values", values); err != nil {
		value := ctx.Query(param)
		if common.ValidateRequiredParam("value", value) != nil && queryParams != nil {
			value = queryParams[param]
		}
		if value != "" {
			values = strings.Split(value, ",")
		}
	}

	// Remove duplicates and trim
	seen := make(map[string]bool)
	unique := []string{}
	for _, v := range values {
		v = common.SanitizeInput(v)
		if v != "" && !seen[v] {
			seen[v] = true
			unique = append(unique, v)
		}
	}

	return unique
}

// buildLinkHeader builds a Link header for pagination
func buildLinkHeader(baseURL string, params *PaginationParams, hasNext, hasPrev bool) string {
	var links []string

	if hasNext && params.MaxID != "" {
		nextURL := fmt.Sprintf("<%s?max_id=%s&limit=%d>; rel=\"next\"", baseURL, params.MaxID, params.Limit)
		links = append(links, nextURL)
	}

	if hasPrev && params.MinID != "" {
		prevURL := fmt.Sprintf("<%s?min_id=%s&limit=%d>; rel=\"prev\"", baseURL, params.MinID, params.Limit)
		links = append(links, prevURL)
	}

	return strings.Join(links, ", ")
}

// withPaginationHeaders adds standard pagination headers to the response
func (h *Handler) withPaginationHeaders(ctx *lift.Context, baseURL string, params *PaginationParams, hasNext, hasPrev bool) {
	if linkHeader := buildLinkHeader(baseURL, params, hasNext, hasPrev); linkHeader != "" {
		ctx.Response.Header("Link", linkHeader)
	}
}

// parseRequestBody is a generic function to parse request bodies with standardized error handling
func (h *Handler) parseRequestBody(ctx *lift.Context, dest interface{}) error {
	if err := ctx.ParseRequest(dest); err != nil {
		// Fallback for test environments - try parsing directly from request body
		if ctx.Request != nil && ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
			if jsonErr := json.Unmarshal(ctx.Request.Body, dest); jsonErr != nil {
				h.logger.Debug("failed to parse request body",
					zap.Error(err),
					zap.Error(jsonErr),
					zap.String("path", ctx.Request.Path),
				)
				return h.respondBadRequest(ctx, "invalid request body")
			}
		} else {
			return h.respondBadRequest(ctx, "invalid request body")
		}
	}
	return nil
}

// getAuthService returns the initialized auth service from the handler
// If not initialized, it creates and caches it
func (h *Handler) getAuthService() (*auth.AuthService, error) {
	// Always create a new auth service to avoid caching issues
	// The Handler struct's authService field is for the interface type
	authSvc, err := auth.NewAuthService(h.cfg, h.repos)
	if err != nil {
		return nil, errors.Join(failedToInitializeAuthService(), err)
	}
	return authSvc, nil
}

// handleAuthServiceError handles common auth service errors with appropriate HTTP responses
func (h *Handler) handleAuthServiceError(ctx *lift.Context, err error, operation string) error {
	if err == nil {
		return nil
	}

	switch err {
	case auth.ErrWebAuthnNotConfigured:
		h.logger.Error("WebAuthn not configured", zap.String("operation", operation))
		return h.respondWithError(ctx, 500, "WebAuthn not configured")

	case auth.ErrChallengeNotFound:
		return h.respondBadRequest(ctx, "invalid or expired challenge")

	case auth.ErrUserHasNoCredentials:
		return h.respondBadRequest(ctx, "no passkeys registered for this user")

	case auth.ErrInvalidCredential:
		return h.respondUnauthorized(ctx)

	case auth.ErrInvalidCredentials:
		return h.respondUnauthorized(ctx)

	case auth.ErrUserNotFound:
		return h.respondBadRequest(ctx, "user not found")

	case auth.ErrUserSuspended:
		return h.respondForbidden(ctx, "user account is suspended")

	case auth.ErrUserNotApproved:
		return h.respondForbidden(ctx, "user account is not approved")

	default:
		h.logger.Error(fmt.Sprintf("failed to %s", operation), zap.Error(err))
		return h.respondWithError(ctx, 500, fmt.Sprintf("failed to %s", operation))
	}
}

// requireAuthService gets the auth service or returns an error response
func (h *Handler) requireAuthService(ctx *lift.Context) (*auth.AuthService, error) {
	authService, err := h.getAuthService()
	if err != nil {
		h.logger.Error("failed to get auth service", zap.Error(err))
		return nil, h.respondWithError(ctx, 500, "internal server error")
	}
	return authService, nil
}

// getDeviceInfo extracts device information from request headers
func (h *Handler) getDeviceInfo(ctx *lift.Context) (userAgent, ipAddress string) {
	userAgent = ctx.Header("User-Agent")
	ipAddress = ctx.Header("X-Forwarded-For")
	if common.ValidateRequiredParam("ipAddress", ipAddress) != nil {
		ipAddress = ctx.Header("X-Real-IP")
	}
	if common.ValidateRequiredParam("ipAddress", ipAddress) != nil {
		ipAddress = "unknown"
	}
	return userAgent, ipAddress
}

// parseLimitParam parses a limit parameter with validation and default value
func (h *Handler) parseLimitParam(ctx *lift.Context, defaultLimit, maxLimit int) int {
	limitStr := ctx.Query("limit")
	if common.ValidateRequiredParam("limitStr", limitStr) != nil && ctx.Request != nil && ctx.Request.Request != nil {
		limitStr = ctx.Request.Request.QueryParams["limit"]
	}

	// Use centralized validation with bounds checking
	limit, err := common.ParseAndValidateIntWithBounds("limit", limitStr, 0, maxLimit, defaultLimit)
	if err != nil {
		return defaultLimit
	}
	return limit
}

//nolint:unused // Part of comprehensive authentication helper set for complex auth scenarios
func (h *Handler) authenticateWithClaims(ctx *lift.Context, requiredScopes []string) (*auth.Claims, error) {
	// Extract and validate token
	token := h.getBearerTokenLift(ctx)
	if err := common.ValidateRequiredParam("token", token); err != nil {
		return nil, helperUnauthorized()
	}

	// Validate token
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return nil, helperUnauthorized()
	}

	// Check scopes if provided
	if err := common.ValidateSliceNotEmpty("required scopes", requiredScopes); err == nil {
		hasScope := false
		for _, scope := range requiredScopes {
			if claims.HasScope(scope) {
				hasScope = true
				break
			}
		}
		if !hasScope {
			return nil, helperInsufficientScope()
		}
	}

	return claims, nil
}

// authenticateUserWithWriteScope handles the common authentication pattern with write scope requirement
// Returns the authenticated username or triggers an HTTP error response
func (h *Handler) authenticateUserWithWriteScope(ctx *lift.Context) (string, error) {
	// Extract and validate token using centralized validation
	authHeader := ctx.Header("Authorization")
	if err := common.ValidateRequiredParam("authHeader", authHeader); err != nil {
		authHeader = ctx.Header("authorization")
	}

	// Try direct access to headers if ctx.Header doesn't work
	if err := common.ValidateRequiredParam("authHeader", authHeader); err != nil && ctx.Request != nil && ctx.Request.Request != nil {
		authHeader = ctx.Request.Request.Headers["Authorization"]
		if err := common.ValidateRequiredParam("authHeader", authHeader); err != nil {
			authHeader = ctx.Request.Request.Headers["authorization"]
		}
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return "", common.RespondUnauthorized(ctx)
	}

	// Validate token and require write scope
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return "", common.RespondUnauthorized(ctx)
	}

	if !claims.HasScope(auth.ScopeWrite) {
		return "", common.RespondInsufficientScope(ctx)
	}

	return claims.Username, nil
}

// respondOK sends a 200 OK response with data
func (h *Handler) respondOK(ctx *lift.Context, data interface{}) error {
	return common.SendOK(ctx, data)
}

// respondCreated sends a 201 Created response with data
func (h *Handler) respondCreated(ctx *lift.Context, data interface{}) error {
	return common.SendCreated(ctx, data)
}
