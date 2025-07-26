package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
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

// HandleGetTag retrieves information about a specific hashtag
func (h *Handler) HandleGetTag(ctx context.Context, request events.APIGatewayV2HTTPRequest, tagName string) (*events.APIGatewayV2HTTPResponse, error) {
	// Get actual tag statistics from storage
	tagStatsRaw, err := h.store.GetHashtagStats(ctx, tagName)
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
			Day:      strconv.FormatInt(entry.Date.Unix(), 10),
			Uses:     strconv.FormatInt(entry.UsageCount, 10),
			Accounts: strconv.FormatInt(entry.UserCount, 10),
		}
	}

	tag := models.Tag{
		Name:    tagName,
		URL:     fmt.Sprintf("%s/tags/%s", h.cfg.BaseURL(), tagName),
		History: history,
	}

	// Check if user is following this tag (if authenticated)
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	if authHeader != "" {
		token, err := auth.ExtractBearerToken(authHeader)
		if err == nil {
			oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
			claims, err := oauthSvc.ValidateAccessToken(token)
			if err == nil {
				// Check if following
				following, _ := h.store.IsFollowingHashtag(ctx, claims.Username, tagName)
				// Return tag with following info in a wrapper
				response := map[string]any{
					"name":      tag.Name,
					"url":       tag.URL,
					"history":   history,
					"following": following,
				}
				return common.OK(response), nil
			}
		}
	}

	return common.OK(tag), nil
}

// HandleFollowTag follows a hashtag
func (h *Handler) HandleFollowTag(ctx context.Context, request events.APIGatewayV2HTTPRequest, tagName string) (*events.APIGatewayV2HTTPResponse, error) {
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

	// Normalize the tag name
	tagName = strings.TrimPrefix(tagName, "#")
	tagName = strings.ToLower(tagName)

	// Create the follow relationship
	err = h.store.FollowHashtag(ctx, claims.Username, tagName)
	if err != nil {
		h.logger.Error("failed to follow hashtag",
			zap.String("user_id", claims.Username),
			zap.String("tag", tagName),
			zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Return the tag with following status
	response := map[string]any{
		"name": tagName,
		"url":  fmt.Sprintf("%s/tags/%s", h.cfg.BaseURL(), tagName),
		"history": []struct {
			Day      string `json:"day"`
			Uses     string `json:"uses"`
			Accounts string `json:"accounts"`
		}{
			{
				Day:      "1668556800",
				Uses:     "0",
				Accounts: "0",
			},
		},
		"following": true,
	}

	return common.OK(response), nil
}

// HandleUnfollowTag unfollows a hashtag
func (h *Handler) HandleUnfollowTag(ctx context.Context, request events.APIGatewayV2HTTPRequest, tagName string) (*events.APIGatewayV2HTTPResponse, error) {
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

	// Normalize the tag name
	tagName = strings.TrimPrefix(tagName, "#")
	tagName = strings.ToLower(tagName)

	// Remove the follow relationship
	err = h.store.UnfollowHashtag(ctx, claims.Username, tagName)
	if err != nil {
		h.logger.Error("failed to unfollow hashtag",
			zap.String("user_id", claims.Username),
			zap.String("tag", tagName),
			zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Return the tag with following status
	response := map[string]any{
		"name": tagName,
		"url":  fmt.Sprintf("%s/tags/%s", h.cfg.BaseURL(), tagName),
		"history": []struct {
			Day      string `json:"day"`
			Uses     string `json:"uses"`
			Accounts string `json:"accounts"`
		}{
			{
				Day:      "1668556800",
				Uses:     "0",
				Accounts: "0",
			},
		},
		"following": false,
	}

	return common.OK(response), nil
}

// HandleGetFollowedTags retrieves the list of hashtags the user is following
func (h *Handler) HandleGetFollowedTags(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
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

	// Parse pagination parameters
	limit := 100
	if limitStr := request.QueryStringParameters["limit"]; limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 200 {
			limit = l
		}
	}

	cursor := request.QueryStringParameters["max_id"]

	// Get followed hashtags
	hashtags, nextCursor, err := h.store.GetFollowedHashtags(ctx, claims.Username, limit, cursor)
	if err != nil {
		h.logger.Error("failed to get followed hashtags", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Convert to tag models with following set to true
	tags := make([]map[string]any, len(hashtags))
	for i, hashtag := range hashtags {
		tags[i] = map[string]any{
			"name": hashtag,
			"url":  fmt.Sprintf("%s/tags/%s", h.cfg.BaseURL(), hashtag),
			"history": []struct {
				Day      string `json:"day"`
				Uses     string `json:"uses"`
				Accounts string `json:"accounts"`
			}{
				{
					Day:      "1668556800",
					Uses:     "0",
					Accounts: "0",
				},
			},
			"following": true,
		}
	}

	// Set Link header for pagination if there's a cursor
	headers := map[string]string{
		"Content-Type": "application/json",
	}
	if nextCursor != "" {
		headers["Link"] = fmt.Sprintf(`<%s/api/v1/followed_tags?max_id=%s>; rel="next"`, h.cfg.BaseURL(), nextCursor)
	}

	body, err := json.Marshal(tags)
	if err != nil {
		return common.InternalServerError(err), nil
	}

	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers:    headers,
		Body:       string(body),
	}, nil
}

// HandleGetFeaturedTags retrieves the user's featured tags
func (h *Handler) HandleGetFeaturedTags(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
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

	// Get featured tags
	featuredTags, err := h.store.GetFeaturedTags(ctx, claims.Username)
	if err != nil {
		h.logger.Error("failed to get featured tags", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Convert to API models
	tags := make([]FeaturedTag, len(featuredTags))
	for i, ft := range featuredTags {
		tags[i] = FeaturedTag{
			ID:            ft.ID,
			Name:          ft.Name,
			URL:           ft.URL,
			StatusesCount: ft.StatusesCount,
			LastStatusAt:  ft.LastStatusAt,
		}
	}

	return common.OK(tags), nil
}

// HandleCreateFeaturedTag features a hashtag on the user's profile
func (h *Handler) HandleCreateFeaturedTag(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
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

	// Parse request body
	var req struct {
		Name string `json:"name"`
	}
	if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
		return common.BadRequest(err), nil
	}

	if req.Name == "" {
		return common.BadRequest(fmt.Errorf("name is required")), nil
	}

	// Create featured tag
	featuredTag, err := h.store.CreateFeaturedTag(ctx, claims.Username, req.Name)
	if err != nil {
		// Check if it's a duplicate
		if err.Error() == "item already exists" {
			return common.UnprocessableEntity(fmt.Errorf("tag already featured")), nil
		}
		// Check if limit reached
		if err.Error() == "featured tag limit reached" {
			return common.UnprocessableEntity(fmt.Errorf("cannot feature more than 10 tags")), nil
		}
		h.logger.Error("failed to create featured tag", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Return the created featured tag
	tag := FeaturedTag{
		ID:            featuredTag.ID,
		Name:          featuredTag.Name,
		URL:           featuredTag.URL,
		StatusesCount: featuredTag.StatusesCount,
		LastStatusAt:  featuredTag.LastStatusAt,
	}

	return common.OK(tag), nil
}

// HandleDeleteFeaturedTag removes a featured tag from the user's profile
func (h *Handler) HandleDeleteFeaturedTag(ctx context.Context, request events.APIGatewayV2HTTPRequest, tagID string) (*events.APIGatewayV2HTTPResponse, error) {
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

	// Delete the featured tag
	err = h.store.DeleteFeaturedTag(ctx, claims.Username, tagID)
	if err != nil {
		if err.Error() == "item not found" {
			return common.NotFound(fmt.Errorf("featured tag not found")), nil
		}
		h.logger.Error("failed to delete featured tag", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Return empty object on success
	return common.OK(map[string]any{}), nil
}

// HandleGetFeaturedTagSuggestions suggests hashtags to feature based on usage
func (h *Handler) HandleGetFeaturedTagSuggestions(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
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

	// Get tag suggestions based on user's posting history
	suggestions, err := h.store.GetTagSuggestions(ctx, claims.Username, 10)
	if err != nil {
		h.logger.Error("failed to get tag suggestions", zap.Error(err))
		// Return empty array on error
		return common.OK([]any{}), nil
	}

	// Convert to tag models
	tags := make([]models.Tag, len(suggestions))
	for i, tagName := range suggestions {
		tags[i] = models.Tag{
			Name: tagName,
			URL:  fmt.Sprintf("%s/tags/%s", h.cfg.BaseURL(), tagName),
			History: []struct {
				Day      string `json:"day"`
				Uses     string `json:"uses"`
				Accounts string `json:"accounts"`
			}{
				{
					Day:      "1668556800",
					Uses:     "0",
					Accounts: "0",
				},
			},
		}
	}

	return common.OK(tags), nil
}

// HandleGetAccountFeaturedTags retrieves featured tags for a specific account
func (h *Handler) HandleGetAccountFeaturedTags(ctx context.Context, request events.APIGatewayV2HTTPRequest, accountID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Get featured tags for the account
	featuredTags, err := h.store.GetFeaturedTags(ctx, accountID)
	if err != nil {
		h.logger.Error("failed to get account featured tags", zap.Error(err))
		// Return empty array on error
		return common.OK([]any{}), nil
	}

	// Convert to API models
	tags := make([]FeaturedTag, len(featuredTags))
	for i, ft := range featuredTags {
		tags[i] = FeaturedTag{
			ID:            ft.ID,
			Name:          ft.Name,
			URL:           ft.URL,
			StatusesCount: ft.StatusesCount,
			LastStatusAt:  ft.LastStatusAt,
		}
	}

	return common.OK(tags), nil
}
