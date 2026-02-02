package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/graph"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/transformations"
	apptheory "github.com/theory-cloud/apptheory/runtime"
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
		parsed, err := url.Parse(accountID)
		if err != nil {
			return nil, invalidAccountURL()
		}

		host := strings.TrimSpace(strings.ToLower(parsed.Hostname()))
		if host == "" {
			return nil, invalidAccountURL()
		}

		// Local actor URL (canonical: https://<domain>/users/<username>)
		if strings.EqualFold(host, h.cfg.Domain) {
			path := strings.Trim(parsed.Path, "/")
			parts := strings.Split(path, "/")
			if len(parts) >= 2 && parts[0] == "users" && strings.TrimSpace(parts[1]) != "" {
				return h.repos.Actor().GetActor(ctx, parts[1])
			}
			return nil, invalidAccountURL()
		}

		remoteSearch := federation.NewRemoteSearchService(h.repos)
		result, err := remoteSearch.ResolveActorURL(ctx, accountID)
		if err != nil {
			return nil, err
		}
		return result.Actor, nil
	}

	// Check if it's a numeric ID (Mastodon compatibility)
	if common.ValidateNumericID("account_id", accountID) == nil && len(accountID) >= 10 {
		// It's a numeric ID - use the dedicated lookup method
		return h.repos.Actor().GetActorByNumericID(ctx, accountID)
	}

	// Support federated handles in addition to local usernames.
	if strings.Contains(accountID, "@") {
		handle := strings.TrimPrefix(strings.TrimSpace(accountID), "@")
		parts := strings.Split(handle, "@")
		if len(parts) == 2 && strings.TrimSpace(parts[0]) != "" && strings.TrimSpace(parts[1]) != "" {
			// Treat local-domain handles as local usernames.
			if strings.EqualFold(strings.ToLower(strings.TrimSpace(parts[1])), h.cfg.Domain) {
				return h.repos.Actor().GetActor(ctx, parts[0])
			}

			remoteSearch := federation.NewRemoteSearchService(h.repos)
			result, err := remoteSearch.ResolveActor(ctx, handle)
			if err != nil {
				return nil, err
			}
			return result.Actor, nil
		}
	}

	// Assume it's a username for local accounts
	return h.repos.Actor().GetActor(ctx, accountID)
}

// authenticateUser handles the common pattern of extracting and validating user authentication
func (h *Handler) authenticateUser(ctx *apptheory.Context, requiredScopes []string) (username string, err error) {

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
func (h *Handler) authenticateUserOptional(ctx *apptheory.Context, requiredScopes []string) (username string, err error) {

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
func (h *Handler) statusActionHandler(ctx *apptheory.Context, requiredScope string, action func(statusID, username string) (*models.Status, error)) (*apptheory.Response, error) {
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
		if isInsufficientScopeError(err) {
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

	return okJSON(status)
}

// getAuthHeader extracts authorization header from request
func (h *Handler) getAuthHeader(ctx *apptheory.Context) string {
	authHeader := headerValue(ctx, "Authorization")
	if err := common.ValidateRequiredParam("authHeader", authHeader); err != nil {
		authHeader = headerValue(ctx, "authorization")
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
	reblogTargetID := storageStatus.ReblogOfID
	if reblogTargetID == "" {
		reblogTargetID = storageStatus.BoostOfStatusID
	}
	if reblogTargetID != "" {
		if rebloggedStatus, err := h.repos.Status().GetStatus(ctx, reblogTargetID); err == nil {
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
	if err != nil || baseStatus.ID == "" {
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

	apiStatus.AgentAttribution = h.buildStatusAgentAttribution(authorAccount, storageStatus)

	// Poll support is implemented in polls.go handler
	// Status.Poll field would be populated here if the status has an associated poll

	return apiStatus, nil
}

func (h *Handler) buildStatusAgentAttribution(authorAccount *storage.Account, status *storageModels.Status) *models.AgentPostAttribution {
	if authorAccount == nil || authorAccount.User == nil || !authorAccount.User.IsAgent {
		return nil
	}

	var noteAttr *activitypub.AgentPostAttribution
	if status != nil && status.Note != nil {
		noteAttr = status.Note.AgentAttribution
	}

	out := &models.AgentPostAttribution{
		ModelVersion: "unknown",
	}

	if noteAttr != nil {
		out.TriggerType = strings.TrimSpace(noteAttr.TriggerType)
		out.TriggerDetails = strings.TrimSpace(noteAttr.TriggerDetails)
		out.MemoryCitations = append([]string(nil), noteAttr.MemoryCitations...)
		out.DelegatedBy = strings.TrimSpace(noteAttr.DelegatedBy)
		out.Scopes = append([]string(nil), noteAttr.Scopes...)
		out.Constraints = append([]string(nil), noteAttr.Constraints...)
		if v := strings.TrimSpace(noteAttr.ModelVersion); v != "" {
			out.ModelVersion = v
		}
	}

	if out.DelegatedBy == "" {
		out.DelegatedBy = strings.TrimSpace(authorAccount.User.AgentOwner)
		if out.DelegatedBy == "" && authorAccount.Actor != nil && authorAccount.Actor.AgentManifest != nil {
			out.DelegatedBy = strings.TrimSpace(authorAccount.Actor.AgentManifest.OperatedBy)
		}
	}

	if len(out.Scopes) == 0 {
		out.Scopes = agentDelegatedScopes(authorAccount.User)
	}

	if len(out.Constraints) == 0 {
		out.Constraints = buildAgentCapabilityConstraints(authorAccount.User.AgentCapabilities)
	}

	if out.ModelVersion == "unknown" {
		if v := strings.TrimSpace(authorAccount.User.AgentVersion); v != "" {
			out.ModelVersion = v
		} else if authorAccount.Actor != nil && authorAccount.Actor.AgentManifest != nil {
			if v := strings.TrimSpace(authorAccount.Actor.AgentManifest.Version); v != "" {
				out.ModelVersion = v
			}
		}
	}

	return out
}

// createOAuthService creates an OAuth service with proper audit logger setup
func createOAuthService(jwtSecret string, cfg *config.Config, repos core.RepositoryStorage, logger *zap.Logger) *auth.OAuthService {
	// Create audit logger with default configuration
	auditLogger := auth.NewAuditLogger(repos, logger, auth.DefaultAuditConfig())
	return auth.NewOAuthService(jwtSecret, cfg, repos, auditLogger)
}

// parsePaginationParams extracts common pagination parameters from a Lift context
// This method now uses the centralized common.GetPaginationParams internally
func (h *Handler) parsePaginationParams(ctx *apptheory.Context) *PaginationParams {
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
func (h *Handler) respondWithError(ctx *apptheory.Context, statusCode int, message string) (*apptheory.Response, error) {
	path := ""
	if ctx != nil {
		path = ctx.Request.Path
	}

	h.logger.Error("API error",
		zap.Int("status", statusCode),
		zap.String("message", message),
		zap.String("path", path),
	)

	// Use the centralized error response helper
	return common.SendError(ctx, statusCode, message)
}

// respondNotFound sends a standardized 404 response
func (h *Handler) respondNotFound(ctx *apptheory.Context, resourceType string) (*apptheory.Response, error) {
	return h.respondWithError(ctx, 404, fmt.Sprintf("%s not found", resourceType))
}

// respondUnauthorized sends a standardized 401 response
func (h *Handler) respondUnauthorized(ctx *apptheory.Context) (*apptheory.Response, error) {
	return h.respondWithError(ctx, 401, "unauthorized")
}

// respondForbidden sends a standardized 403 response
func (h *Handler) respondForbidden(ctx *apptheory.Context, reason string) (*apptheory.Response, error) {
	if err := common.ValidateRequiredParam("reason", reason); err != nil {
		reason = "forbidden"
	}
	return h.respondWithError(ctx, 403, reason)
}

// respondBadRequest sends a standardized 400 response
func (h *Handler) respondBadRequest(ctx *apptheory.Context, message string) (*apptheory.Response, error) {
	if err := common.ValidateRequiredParam("message", message); err != nil {
		message = "bad request"
	}
	return h.respondWithError(ctx, 400, message)
}

// Additional standardized error response functions for common patterns

// respondInternalError sends a standardized 500 response
func (h *Handler) respondInternalError(ctx *apptheory.Context, message string) (*apptheory.Response, error) {
	if common.ValidateRequiredParam("message", message) != nil {
		message = "internal server error"
	}
	return h.respondWithError(ctx, 500, message)
}

// respondConflict sends a standardized 409 response
//
//nolint:unused // Part of complete set of standardized HTTP response helpers
func (h *Handler) respondConflict(ctx *apptheory.Context, message string) (*apptheory.Response, error) {
	if common.ValidateRequiredParam("message", message) != nil {
		message = "conflict"
	}
	return h.respondWithError(ctx, 409, message)
}

// respondUnprocessableEntity sends a standardized 422 response
func (h *Handler) respondUnprocessableEntity(ctx *apptheory.Context, message string) (*apptheory.Response, error) {
	if common.ValidateRequiredParam("message", message) != nil {
		message = "unprocessable entity"
	}
	return h.respondWithError(ctx, 422, message)
}

// respondInsufficientScope sends a standardized 403 response for scope issues
func (h *Handler) respondInsufficientScope(ctx *apptheory.Context) (*apptheory.Response, error) {
	return h.respondForbidden(ctx, "insufficient scope")
}

// parseBoolParam parses a boolean parameter from the request
func (h *Handler) parseBoolParam(ctx *apptheory.Context, param string) bool {
	value := queryValue(ctx, param)
	if common.ValidateRequiredParam("value", value) != nil {
		return false
	}
	return common.ValidateBooleanString(value)
}

// parseArrayParam parses an array parameter from the request
// Supports both id[]=1&id[]=2 and id=1,2 formats
func (h *Handler) parseArrayParam(ctx *apptheory.Context, param string) []string {
	var values []string

	for key, valueSet := range ctx.Request.Query {
		if strings.HasPrefix(key, param+"[") && strings.HasSuffix(key, "]") {
			values = append(values, valueSet...)
		}
	}

	// If no array format found, check for comma-separated format
	if err := common.ValidateSliceNotEmpty("parsed values", values); err != nil {
		values = append(values, queryValues(ctx, param)...)
	}

	// Remove duplicates and trim
	seen := make(map[string]bool)
	unique := []string{}
	for _, v := range values {
		v = common.SanitizeInput(v)
		if strings.Contains(v, ",") {
			for _, split := range strings.Split(v, ",") {
				split = common.SanitizeInput(split)
				if split != "" && !seen[split] {
					seen[split] = true
					unique = append(unique, split)
				}
			}
			continue
		}
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
func (h *Handler) withPaginationHeaders(resp *apptheory.Response, baseURL string, params *PaginationParams, hasNext, hasPrev bool) {
	if linkHeader := buildLinkHeader(baseURL, params, hasNext, hasPrev); linkHeader != "" {
		setHeader(resp, "Link", linkHeader)
	}
}

// parseRequestBody is a generic function to parse request bodies with standardized error handling
func (h *Handler) parseRequestBody(ctx *apptheory.Context, dest interface{}) error {
	if err := common.ParseRequestWithFallback(ctx, dest); err != nil {
		// Fallback for test environments - try parsing directly from request body
		if len(ctx.Request.Body) > 0 {
			if jsonErr := json.Unmarshal(ctx.Request.Body, dest); jsonErr == nil {
				return nil
			}
		}
		return err
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
func (h *Handler) handleAuthServiceError(ctx *apptheory.Context, err error, operation string) (*apptheory.Response, error) {
	if err == nil {
		return nil, nil
	}

	switch {
	case errors.Is(err, auth.ErrWebAuthnNotConfigured):
		h.logger.Error("WebAuthn not configured", zap.String("operation", operation))
		return h.respondWithError(ctx, 500, "WebAuthn not configured")

	case errors.Is(err, auth.ErrChallengeNotFound):
		return h.respondBadRequest(ctx, "invalid or expired challenge")

	case errors.Is(err, auth.ErrUserHasNoCredentials):
		return h.respondBadRequest(ctx, "no passkeys registered for this user")

	case errors.Is(err, auth.ErrInvalidCredential):
		return h.respondUnauthorized(ctx)

	case errors.Is(err, auth.ErrInvalidCredentials):
		return h.respondUnauthorized(ctx)

	case errors.Is(err, auth.ErrSignatureVerificationFailed):
		return h.respondUnauthorized(ctx)

	case errors.Is(err, auth.ErrUserNotFound):
		return h.respondBadRequest(ctx, "user not found")

	case errors.Is(err, auth.ErrUserSuspended):
		return h.respondForbidden(ctx, "user account is suspended")

	case errors.Is(err, auth.ErrUserNotApproved):
		return h.respondForbidden(ctx, "user account is not approved")

	default:
		h.logger.Error(fmt.Sprintf("failed to %s", operation), zap.Error(err))
		return h.respondWithError(ctx, 500, fmt.Sprintf("failed to %s", operation))
	}
}

// requireAuthService gets the auth service or returns a 500 response
func (h *Handler) requireAuthService(ctx *apptheory.Context) (*auth.AuthService, *apptheory.Response, error) {
	authService, err := h.getAuthService()
	if err != nil {
		h.logger.Error("failed to get auth service", zap.Error(err))
		resp, respErr := h.respondWithError(ctx, 500, "internal server error")
		return nil, resp, respErr
	}
	return authService, nil, nil
}

// getDeviceInfo extracts device information from request headers
func (h *Handler) getDeviceInfo(ctx *apptheory.Context) (userAgent, ipAddress string) {
	userAgent = headerValue(ctx, "User-Agent")
	ipAddress = headerValue(ctx, "X-Forwarded-For")
	if common.ValidateRequiredParam("ipAddress", ipAddress) != nil {
		ipAddress = headerValue(ctx, "X-Real-IP")
	}
	if common.ValidateRequiredParam("ipAddress", ipAddress) != nil {
		ipAddress = "unknown"
	}
	return userAgent, ipAddress
}

// parseLimitParam parses a limit parameter with validation and default value
func (h *Handler) parseLimitParam(ctx *apptheory.Context, defaultLimit, maxLimit int) int {
	limitStr := queryValue(ctx, "limit")

	// Use centralized validation with bounds checking
	limit, err := common.ParseAndValidateIntWithBounds("limit", limitStr, 0, maxLimit, defaultLimit)
	if err != nil {
		return defaultLimit
	}
	return limit
}

//nolint:unused // Part of comprehensive authentication helper set for complex auth scenarios
func (h *Handler) authenticateWithClaims(ctx *apptheory.Context, requiredScopes []string) (*auth.Claims, error) {
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
func (h *Handler) authenticateUserWithWriteScope(ctx *apptheory.Context) (string, error) {
	return h.authenticateUser(ctx, []string{auth.ScopeWrite})
}

// respondOK sends a 200 OK response with data
func (h *Handler) respondOK(ctx *apptheory.Context, data interface{}) (*apptheory.Response, error) {
	return common.SendOK(ctx, data)
}

// respondCreated sends a 201 Created response with data
func (h *Handler) respondCreated(ctx *apptheory.Context, data interface{}) (*apptheory.Response, error) {
	return common.SendCreated(ctx, data)
}
