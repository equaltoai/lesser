package lift

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// TagInfo represents detailed information about a hashtag
type TagInfo struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	URL           string   `json:"url"`
	Following     bool     `json:"following"`
	History       []TagUse `json:"history"`
	TotalUses     int      `json:"total_uses"`
	TotalAccounts int      `json:"total_accounts"`
}

// TagUse represents hashtag usage statistics for a day
type TagUse struct {
	Day      string `json:"day"`
	Uses     string `json:"uses"`
	Accounts string `json:"accounts"`
}

// FeaturedTag represents a featured hashtag
type FeaturedTag struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	URL           string    `json:"url"`
	StatusesCount int       `json:"statuses_count"`
	LastStatusAt  string    `json:"last_status_at"`
	CreatedAt     time.Time `json:"-"`
}

// HandleGetTagLift retrieves information about a specific hashtag
func (h *Handler) HandleGetTagLift(ctx *lift.Context) error {
	tagName := ctx.Param("id")
	if err := common.ValidateRequiredParam("id", tagName); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Validate hashtag format
	if err := common.ValidateHashtag(tagName); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Get actual tag statistics from storage
	tagStatsRaw, err := h.repos.Hashtag().GetHashtagStats(ctx.Context, tagName)
	var tagStats *storage.HashtagStats
	if err != nil || tagStatsRaw == nil {
		h.logger.Error("failed to get hashtag stats", zap.Error(err))
		// Fall back to empty stats
		tagStats = &storage.HashtagStats{
			Name:          tagName,
			TotalUses:     0,
			TotalAccounts: 0,
			History:       []storage.HashtagHistoryEntry{},
		}
	} else {
		// Type assert the any to *storage.HashtagStats
		var ok bool
		tagStats, ok = tagStatsRaw.(*storage.HashtagStats)
		if !ok {
			// Fall back to empty stats if type assertion fails
			tagStats = &storage.HashtagStats{
				Name:          tagName,
				TotalUses:     0,
				TotalAccounts: 0,
				History:       []storage.HashtagHistoryEntry{},
			}
		}
	}

	// Convert history to API format
	history := make([]struct {
		Day      string `json:"day"`
		Uses     string `json:"uses"`
		Accounts string `json:"accounts"`
	}, len(tagStats.History))

	for i, entry := range tagStats.History {
		history[i] = struct {
			Day      string `json:"day"`
			Uses     string `json:"uses"`
			Accounts string `json:"accounts"`
		}{
			Day:      entry.Date,       // Already a string timestamp
			Uses:     entry.UsageCount, // Already a string
			Accounts: entry.UserCount,  // Already a string
		}
	}

	tag := models.Tag{
		Name:    tagName,
		URL:     fmt.Sprintf("%s/tags/%s", h.cfg.BaseURL(), tagName),
		History: history,
	}

	// Check if user is following this tag (if authenticated)
	authHeader := h.getAuthorizationHeader(ctx)

	if authHeader != "" {
		token, err := auth.ExtractBearerToken(authHeader)
		if err == nil {
			oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
			claims, err := oauthSvc.ValidateAccessToken(token)
			if err == nil {
				// Check if following
				following, _ := h.repos.Hashtag().IsFollowingHashtag(ctx.Context, claims.Username, tagName)
				// Return tag with following info in a wrapper
				response := map[string]any{
					"name":      tag.Name,
					"url":       tag.URL,
					"history":   history,
					"following": following,
				}
				return ctx.JSON(response)
			}
		}
	}

	return ctx.JSON(tag)
}

// tagFollowAction represents the type of tag follow action
type tagFollowAction int

const (
	tagFollow   tagFollowAction = iota
	tagUnfollow
)

// authenticateTagRequest handles authentication for tag operations
func (h *Handler) authenticateTagRequest(ctx *lift.Context) (string, error) {
	// Test hook - check for test username header
	testUsername := ctx.Header("X-Test-Username")
	if common.ValidateRequiredParam(testUsername, "testUsername") != nil && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	if testUsername != "" {
		return testUsername, nil
	}

	// Extract token from Authorization header
	authHeader := h.getAuthorizationHeader(ctx)

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return "", err
	}

	// Validate token and get claims
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return "", err
	}

	return claims.Username, nil
}

// buildTagResponseWithHistory builds a tag response including history
func (h *Handler) buildTagResponseWithHistory(ctx *lift.Context, tagName string, following bool) map[string]any {
	return map[string]any{
		"name": tagName,
		"url":  fmt.Sprintf("%s/tags/%s", h.cfg.BaseURL(), tagName),
		"history": func() []struct {
			Day      string `json:"day"`
			Uses     string `json:"uses"`
			Accounts string `json:"accounts"`
		} {
			// Get real hashtag statistics
			if tagStatsRaw, err := h.repos.Hashtag().GetHashtagStats(ctx.Context, tagName); err == nil && tagStatsRaw != nil {
				if tagStats, ok := tagStatsRaw.(*storage.HashtagStats); ok {
					history := make([]struct {
						Day      string `json:"day"`
						Uses     string `json:"uses"`
						Accounts string `json:"accounts"`
					}, len(tagStats.History))
					for i, entry := range tagStats.History {
						history[i] = struct {
							Day      string `json:"day"`
							Uses     string `json:"uses"`
							Accounts string `json:"accounts"`
						}{
							Day:      entry.Date,
							Uses:     entry.UsageCount,
							Accounts: entry.UserCount,
						}
					}
					return history
				}
			}
			// Fallback to empty history
			return []struct {
				Day      string `json:"day"`
				Uses     string `json:"uses"`
				Accounts string `json:"accounts"`
			}{}
		}(),
		"following": following,
	}
}

// handleTagFollowAction handles both follow and unfollow operations for tags
func (h *Handler) handleTagFollowAction(ctx *lift.Context, action tagFollowAction) error {
	tagName := ctx.Param("id")
	if err := common.ValidateRequiredParam("id", tagName); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Validate hashtag format
	if err := common.ValidateHashtag(tagName); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	username, err := h.authenticateTagRequest(ctx)
	if err != nil {
		return common.RespondUnauthorized(ctx)
	}

	// Normalize the tag name
	tagName = strings.TrimPrefix(tagName, "#")
	tagName = strings.ToLower(tagName)

	// Perform the action
	var following bool
	var logAction string
	switch action {
	case tagFollow:
		err = h.repos.Hashtag().FollowHashtag(ctx.Context, username, tagName)
		following = true
		logAction = "follow"
	case tagUnfollow:
		err = h.repos.Hashtag().UnfollowHashtag(ctx.Context, username, tagName)
		following = false
		logAction = "unfollow"
	}

	if err != nil {
		h.logger.Error(fmt.Sprintf("failed to %s hashtag", logAction),
			zap.String("user_id", username),
			zap.String("tag", tagName),
			zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	response := h.buildTagResponseWithHistory(ctx, tagName, following)
	return ctx.JSON(response)
}

// HandleFollowTagLift follows a hashtag
func (h *Handler) HandleFollowTagLift(ctx *lift.Context) error {
	return h.handleTagFollowAction(ctx, tagFollow)
}

// HandleUnfollowTagLift unfollows a hashtag
func (h *Handler) HandleUnfollowTagLift(ctx *lift.Context) error {
	return h.handleTagFollowAction(ctx, tagUnfollow)
}

// HandleGetFollowedTagsLift retrieves the list of hashtags the user is following
func (h *Handler) HandleGetFollowedTagsLift(ctx *lift.Context) error {
	// Extract username using authentication
	username, err := h.extractUsernameFromContextForTags(ctx)
	if err != nil {
		return common.RespondUnauthorized(ctx)
	}

	// Parse pagination parameters
	paginationParams := h.extractPaginationParams(ctx)

	// Get followed hashtags
	hashtags, nextCursor, err := h.repos.Hashtag().GetFollowedHashtags(ctx.Context, username, paginationParams.limit, paginationParams.cursor)
	if err != nil {
		h.logger.Error("failed to get followed hashtags", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	// Convert to tag models with following set to true
	tags := h.buildTagModels(ctx.Context, hashtags)

	// Set Link header for pagination if there's a cursor
	if nextCursor != "" {
		ctx.Response.Header("Link", fmt.Sprintf(`<%s/api/v1/followed_tags?max_id=%s>; rel="next"`, h.cfg.BaseURL(), nextCursor))
	}

	return ctx.JSON(tags)
}

// extractUsernameFromContextForTags extracts username from test header or OAuth token
func (h *Handler) extractUsernameFromContextForTags(ctx *lift.Context) (string, error) {
	// Test hook - check for test username header
	testUsername := h.getHeaderValue(ctx, "X-Test-Username")
	if testUsername != "" {
		return testUsername, nil
	}

	// Extract and validate OAuth token
	authHeader := h.getAuthorizationHeader(ctx)
	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return "", err
	}

	// Validate token and get claims
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return "", err
	}

	return claims.Username, nil
}

// getHeaderValue gets header value with fallback to direct request access
func (h *Handler) getHeaderValue(ctx *lift.Context, headerName string) string {
	value := ctx.Header(headerName)
	if common.ValidateRequiredParam(value, "value") != nil && ctx.Request != nil && ctx.Request.Request != nil {
		value = ctx.Request.Request.Headers[headerName]
	}
	return value
}

// getAuthorizationHeader extracts Authorization header with case variations
func (h *Handler) getAuthorizationHeader(ctx *lift.Context) string {
	authHeader := ctx.Header("Authorization")
	if common.ValidateRequiredParam(authHeader, "authHeader") != nil {
		authHeader = ctx.Header("authorization")
	}

	// Try direct access to headers if ctx.Header doesn't work
	if common.ValidateRequiredParam(authHeader, "authHeader") != nil && ctx.Request != nil && ctx.Request.Request != nil {
		authHeader = ctx.Request.Request.Headers["Authorization"]
		if common.ValidateRequiredParam(authHeader, "authHeader") != nil {
			authHeader = ctx.Request.Request.Headers["authorization"]
		}
	}

	return authHeader
}

// paginationParams holds pagination parameters
type paginationParams struct {
	limit  int
	cursor string
}

// extractPaginationParams extracts and validates pagination parameters
func (h *Handler) extractPaginationParams(ctx *lift.Context) paginationParams {
	limitStr := ctx.Query("limit")
	if common.ValidateRequiredParam(limitStr, "limitStr") != nil && ctx.Request != nil && ctx.Request.Request != nil {
		limitStr = ctx.Request.Request.QueryParams["limit"]
	}
	limit, err := common.ParseHashtagLimit(limitStr)
	if err != nil {
		limit = 100
	}

	cursor := ctx.Query("max_id")
	if common.ValidateRequiredParam(cursor, "cursor") != nil && ctx.Request != nil && ctx.Request.Request != nil {
		cursor = ctx.Request.Request.QueryParams["max_id"]
	}

	return paginationParams{limit: limit, cursor: cursor}
}

// buildTagModels converts hashtags to tag models with history
func (h *Handler) buildTagModels(ctx context.Context, hashtags []string) []map[string]any {
	tags := make([]map[string]any, len(hashtags))
	for i, hashtag := range hashtags {
		tags[i] = map[string]any{
			"name":      hashtag,
			"url":       fmt.Sprintf("%s/tags/%s", h.cfg.BaseURL(), hashtag),
			"history":   h.getHashtagHistory(ctx, hashtag),
			"following": true,
		}
	}
	return tags
}

// hashtagHistoryEntry represents a single history entry
type hashtagHistoryEntry struct {
	Day      string `json:"day"`
	Uses     string `json:"uses"`
	Accounts string `json:"accounts"`
}

// getHashtagHistory retrieves hashtag statistics and formats as history
func (h *Handler) getHashtagHistory(ctx context.Context, hashtag string) []hashtagHistoryEntry {
	tagStatsRaw, err := h.repos.Hashtag().GetHashtagStats(ctx, hashtag)
	if err != nil || tagStatsRaw == nil {
		return []hashtagHistoryEntry{}
	}

	tagStats, ok := tagStatsRaw.(*storage.HashtagStats)
	if !ok {
		return []hashtagHistoryEntry{}
	}

	history := make([]hashtagHistoryEntry, len(tagStats.History))
	for i, entry := range tagStats.History {
		history[i] = hashtagHistoryEntry{
			Day:      entry.Date,
			Uses:     entry.UsageCount,
			Accounts: entry.UserCount,
		}
	}
	return history
}

// HandleGetFeaturedTagsLift retrieves the user's featured tags
func (h *Handler) HandleGetFeaturedTagsLift(ctx *lift.Context) error {
	// Test hook - check for test username header
	testUsername := ctx.Header("X-Test-Username")
	if common.ValidateRequiredParam(testUsername, "testUsername") != nil && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var username string
	if testUsername != "" {
		username = testUsername
	} else {
		// Extract token from Authorization header
		authHeader := h.getAuthorizationHeader(ctx)

		token, err := auth.ExtractBearerToken(authHeader)
		if err != nil {
			return common.RespondUnauthorized(ctx)
		}

		// Validate token and get claims
		oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
		claims, err := oauthSvc.ValidateAccessToken(token)
		if err != nil {
			return common.RespondUnauthorized(ctx)
		}
		username = claims.Username
	}

	// Get featured tags
	featuredTags, err := h.repos.FeaturedTag().GetFeaturedTags(ctx.Context, username)
	if err != nil {
		h.logger.Error("failed to get featured tags", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	// Convert to API models
	tags := make([]FeaturedTag, len(featuredTags))
	for i, ft := range featuredTags {
		tags[i] = FeaturedTag{
			ID:            ft.ID,
			Name:          ft.Name,
			URL:           ft.URL,
			StatusesCount: ft.StatusesCount,
			LastStatusAt: func() string {
				if ft.LastStatusAt == nil {
					return ""
				}
				return ft.LastStatusAt.Format(time.RFC3339)
			}(),
		}
	}

	return ctx.JSON(tags)
}

// HandleCreateFeaturedTagLift features a hashtag on the user's profile
func (h *Handler) HandleCreateFeaturedTagLift(ctx *lift.Context) error {
	// Test hook - check for test username header
	testUsername := ctx.Header("X-Test-Username")
	if common.ValidateRequiredParam(testUsername, "testUsername") != nil && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var username string
	if testUsername != "" {
		username = testUsername
	} else {
		// Extract token from Authorization header
		authHeader := h.getAuthorizationHeader(ctx)

		token, err := auth.ExtractBearerToken(authHeader)
		if err != nil {
			return common.RespondUnauthorized(ctx)
		}

		// Validate token and get claims
		oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
		claims, err := oauthSvc.ValidateAccessToken(token)
		if err != nil {
			return common.RespondUnauthorized(ctx)
		}
		username = claims.Username
	}

	// Parse request body
	var req struct {
		Name string `json:"name"`
	}

	// Try ctx.ParseRequest first, then fall back to common.ParseRequestBody for test mode
	if err := ctx.ParseRequest(&req); err != nil {
		// Fall back to manual body parsing for test environments
		if ctx.Request != nil && ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
			if err := common.ParseRequestBody(ctx.Request.Body, &req); err != nil {
				return common.RespondInvalidRequest(ctx)
			}
		} else {
			return common.RespondInvalidRequest(ctx)
		}
	}

	if err := common.ValidateRequiredParam("name", req.Name); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Validate hashtag format
	if err := common.ValidateHashtag(req.Name); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Create featured tag
	tagName := strings.TrimPrefix(req.Name, "#")
	featuredTag := &storage.FeaturedTag{
		ID:       fmt.Sprintf("%s-%s", username, tagName),
		Username: username,
		Name:     tagName,
		URL:      fmt.Sprintf("%s/tags/%s", h.cfg.BaseURL(), tagName),
	}
	err := h.repos.FeaturedTag().CreateFeaturedTag(ctx.Context, featuredTag)
	if err != nil {
		// Check if it's a duplicate
		if err.Error() == "item already exists" {
			return common.RespondAlreadyExists(ctx, "featured tag")
		}
		// Check if limit reached
		if err.Error() == "featured tag limit reached" {
			return common.RespondUnprocessableEntity(ctx, "cannot feature more than 10 tags")
		}
		h.logger.Error("failed to create featured tag", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	// Return the created featured tag
	tag := FeaturedTag{
		ID:            featuredTag.ID,
		Name:          featuredTag.Name,
		URL:           featuredTag.URL,
		StatusesCount: featuredTag.StatusesCount,
		LastStatusAt: func() string {
			if featuredTag.LastStatusAt == nil {
				return ""
			}
			return featuredTag.LastStatusAt.Format(time.RFC3339)
		}(),
	}

	return ctx.JSON(tag)
}

// HandleDeleteFeaturedTagLift removes a featured tag from the user's profile
func (h *Handler) HandleDeleteFeaturedTagLift(ctx *lift.Context) error {
	tagID := ctx.Param("id")
	if err := common.ValidateRequiredParam("id", tagID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Validate tag ID format (could be a hashtag)
	if err := common.ValidateHashtag(tagID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Test hook - check for test username header
	testUsername := ctx.Header("X-Test-Username")
	if common.ValidateRequiredParam(testUsername, "testUsername") != nil && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var username string
	if testUsername != "" {
		username = testUsername
	} else {
		// Extract token from Authorization header
		authHeader := h.getAuthorizationHeader(ctx)

		token, err := auth.ExtractBearerToken(authHeader)
		if err != nil {
			return common.RespondUnauthorized(ctx)
		}

		// Validate token and get claims
		oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
		claims, err := oauthSvc.ValidateAccessToken(token)
		if err != nil {
			return common.RespondUnauthorized(ctx)
		}
		username = claims.Username
	}

	// Delete the featured tag
	err := h.repos.FeaturedTag().DeleteFeaturedTag(ctx.Context, username, tagID)
	if err != nil {
		if err.Error() == "item not found" {
			return common.RespondNotFound(ctx, "featured tag")
		}
		h.logger.Error("failed to delete featured tag", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	// Return empty object on success
	return ctx.JSON(map[string]any{})
}

// HandleGetFeaturedTagSuggestionsLift suggests hashtags to feature based on usage
func (h *Handler) HandleGetFeaturedTagSuggestionsLift(ctx *lift.Context) error {
	// Test hook - check for test username header
	testUsername := ctx.Header("X-Test-Username")
	if common.ValidateRequiredParam(testUsername, "testUsername") != nil && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var username string
	if testUsername != "" {
		username = testUsername
	} else {
		// Extract token from Authorization header
		authHeader := h.getAuthorizationHeader(ctx)

		token, err := auth.ExtractBearerToken(authHeader)
		if err != nil {
			return common.RespondUnauthorized(ctx)
		}

		// Validate token and get claims
		oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
		claims, err := oauthSvc.ValidateAccessToken(token)
		if err != nil {
			return common.RespondUnauthorized(ctx)
		}
		username = claims.Username
	}

	// Get tag suggestions based on user's posting history
	suggestions, err := h.repos.FeaturedTag().GetTagSuggestions(ctx.Context, username, 10)
	if err != nil {
		h.logger.Error("failed to get tag suggestions", zap.Error(err))
		// Return empty array on error
		return ctx.JSON([]any{})
	}

	// Convert to tag models
	tags := make([]models.Tag, len(suggestions))
	for i, tagName := range suggestions {
		tags[i] = models.Tag{
			Name: tagName,
			URL:  fmt.Sprintf("%s/tags/%s", h.cfg.BaseURL(), tagName),
			History: func() []struct {
				Day      string `json:"day"`
				Uses     string `json:"uses"`
				Accounts string `json:"accounts"`
			} {
				// Get real hashtag statistics
				if tagStatsRaw, err := h.repos.Hashtag().GetHashtagStats(ctx.Context, tagName); err == nil && tagStatsRaw != nil {
					if tagStats, ok := tagStatsRaw.(*storage.HashtagStats); ok {
						history := make([]struct {
							Day      string `json:"day"`
							Uses     string `json:"uses"`
							Accounts string `json:"accounts"`
						}, len(tagStats.History))
						for i, entry := range tagStats.History {
							history[i] = struct {
								Day      string `json:"day"`
								Uses     string `json:"uses"`
								Accounts string `json:"accounts"`
							}{
								Day:      entry.Date,
								Uses:     entry.UsageCount,
								Accounts: entry.UserCount,
							}
						}
						return history
					}
				}
				// Fallback to empty history
				return []struct {
					Day      string `json:"day"`
					Uses     string `json:"uses"`
					Accounts string `json:"accounts"`
				}{}
			}(),
		}
	}

	return ctx.JSON(tags)
}

// HandleGetAccountFeaturedTagsLift retrieves featured tags for a specific account
func (h *Handler) HandleGetAccountFeaturedTagsLift(ctx *lift.Context) error {
	accountID := ctx.Param("id")
	if err := common.ValidateRequiredParam("id", accountID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Get featured tags for the account
	featuredTags, err := h.repos.FeaturedTag().GetFeaturedTags(ctx.Context, accountID)
	if err != nil {
		h.logger.Error("failed to get account featured tags", zap.Error(err))
		// Return empty array on error
		return ctx.JSON([]any{})
	}

	// Convert to API models
	tags := make([]FeaturedTag, len(featuredTags))
	for i, ft := range featuredTags {
		tags[i] = FeaturedTag{
			ID:            ft.ID,
			Name:          ft.Name,
			URL:           ft.URL,
			StatusesCount: ft.StatusesCount,
			LastStatusAt: func() string {
				if ft.LastStatusAt == nil {
					return ""
				}
				return ft.LastStatusAt.Format(time.RFC3339)
			}(),
		}
	}

	return ctx.JSON(tags)
}
