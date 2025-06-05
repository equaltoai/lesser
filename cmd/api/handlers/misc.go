package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/aron23/lesser/cmd/api/models"
	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/config"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// HandleGetInstance returns instance information
func (h *Handler) HandleGetInstance(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Get static config
	instanceConfig := config.GetInstanceConfig()

	// Get dynamic rules from DynamoDB
	rules, err := h.store.GetInstanceRules(ctx)
	if err != nil {
		h.logger.Error("failed to get instance rules", zap.Error(err))
		// Continue with empty rules on error
		rules = []storage.InstanceRule{}
	}

	// Convert rules to API format
	apiRules := make([]interface{}, len(rules))
	for i, rule := range rules {
		apiRules[i] = map[string]interface{}{
			"id":   rule.ID,
			"text": rule.Text,
		}
	}

	// Instance info doesn't require authentication
	instance := models.Instance{
		URI:              h.cfg.Domain,
		Title:            instanceConfig.Title,
		ShortDescription: instanceConfig.ShortDescription,
		Description:      instanceConfig.Description,
		Email:            instanceConfig.Email,
		Version:          instanceConfig.Version,
		Stats: map[string]interface{}{
			"user_count":   0,
			"status_count": 0,
			"domain_count": 1,
		},
		Thumbnail:        fmt.Sprintf("https://%s/instance/thumbnail.png", h.cfg.Domain),
		Languages:        instanceConfig.Languages,
		Registrations:    instanceConfig.RegistrationsOpen,
		ApprovalRequired: instanceConfig.ApprovalRequired,
		InvitesEnabled:   instanceConfig.InvitesEnabled,
		ContactAccount:   nil, // TODO: Set admin account
		Rules:            apiRules,
		Configuration: map[string]interface{}{
			"statuses": map[string]interface{}{
				"max_characters": instanceConfig.MaxStatusChars,
			},
			"media_attachments": map[string]interface{}{
				"supported_mime_types": []string{
					"image/jpeg",
					"image/png",
					"image/gif",
					"image/webp",
					"video/mp4",
					"video/webm",
				},
				"image_size_limit": instanceConfig.MaxMediaSize,
				"video_size_limit": instanceConfig.MaxVideoSize,
			},
		},
	}

	// TODO: Implement actual counts - for now just use dummy data
	instance.Stats["user_count"] = 1
	instance.Stats["status_count"] = 0

	return common.OK(instance), nil
}

// HandleSearch performs a search across accounts, statuses, and hashtags
func (h *Handler) HandleSearch(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Search can be authenticated or not
	var _ *activitypub.Actor
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	if token, err := auth.ExtractBearerToken(authHeader); err == nil {
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
		if claims, err := oauthSvc.ValidateAccessToken(token); err == nil {
			_, _ = h.store.GetActor(ctx, claims.Username)
		}
	}

	// Get search query
	query := request.QueryStringParameters["q"]
	if query == "" {
		return common.BadRequest(errors.New("q parameter is required")), nil
	}

	// Parse search parameters
	searchType := request.QueryStringParameters["type"] // accounts, hashtags, statuses
	_ = request.QueryStringParameters["resolve"] == "true"
	_ = request.QueryStringParameters["following"] == "true"
	_ = request.QueryStringParameters["account_id"]
	_ = request.QueryStringParameters["exclude_unreviewed"] == "true"
	_ = request.QueryStringParameters["min_id"]
	_ = request.QueryStringParameters["max_id"]
	// limit := 20

	// Initialize results
	result := models.SearchResult{
		Accounts: []models.Account{},
		Statuses: []models.Status{},
		Hashtags: []models.Tag{},
	}

	// Search accounts
	if searchType == "" || searchType == "accounts" {
		// Simple username search
		if strings.HasPrefix(query, "@") {
			username := strings.TrimPrefix(query, "@")
			if actor, err := h.store.GetActor(ctx, username); err == nil {
				account := models.Account{
					ID:             actor.PreferredUsername,
					Username:       actor.PreferredUsername,
					Acct:           actor.PreferredUsername,
					DisplayName:    actor.Name,
					URL:            actor.URL,
					Note:           actor.Summary,
					Avatar:         "",
					AvatarStatic:   "",
					Header:         "",
					HeaderStatic:   "",
					FollowersCount: 0,
					FollowingCount: 0,
					StatusesCount:  0,
					Emojis:         []interface{}{},
					Fields:         []interface{}{},
				}

				if actor.Icon != nil {
					account.Avatar = actor.Icon.URL
					account.AvatarStatic = actor.Icon.URL
				}

				result.Accounts = append(result.Accounts, account)
			}
		}

		// TODO: Implement full-text search for accounts
	}

	// Search statuses
	if searchType == "" || searchType == "statuses" {
		// TODO: Implement status search
		// For now, just search by exact URL
		if strings.HasPrefix(query, "http://") || strings.HasPrefix(query, "https://") {
			if obj, err := h.store.GetObject(ctx, query); err == nil {
				// Get the actor who created the object
				var attributedTo string
				var objActor *activitypub.Actor

				switch o := obj.(type) {
				case *activitypub.Note:
					attributedTo = o.AttributedTo
				case map[string]interface{}:
					if attr, ok := o["attributedTo"].(string); ok {
						attributedTo = attr
					}
				}

				if attributedTo != "" {
					parts := strings.Split(attributedTo, "/")
					if len(parts) > 0 {
						username := parts[len(parts)-1]
						objActor, _ = h.store.GetActor(ctx, username)
					}
				}

				status := ObjectToStatus(obj, objActor)
				result.Statuses = append(result.Statuses, status)
			}
		}
	}

	// Search hashtags
	if searchType == "" || searchType == "hashtags" {
		// TODO: Implement hashtag search
		if strings.HasPrefix(query, "#") {
			tag := models.Tag{
				Name: strings.TrimPrefix(query, "#"),
				URL:  fmt.Sprintf("%s/tags/%s", h.cfg.BaseURL(), strings.TrimPrefix(query, "#")),
				History: []models.TagHistory{
					{
						Day:      "0",
						Uses:     "0",
						Accounts: "0",
					},
				},
			}
			result.Hashtags = append(result.Hashtags, tag)
		}
	}

	return common.OK(result), nil
}

// HandleGetNotifications retrieves notifications for the authenticated user
func (h *Handler) HandleGetNotifications(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
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

	// Check read:notifications scope
	if !claims.HasScope("read:notifications") && !claims.HasScope(auth.ScopeRead) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Get the user's actor
	_, err = h.store.GetActor(ctx, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Parse query parameters
	_ = request.QueryStringParameters["max_id"]
	_ = request.QueryStringParameters["since_id"]
	_ = request.QueryStringParameters["min_id"]
	_ = 15
	_ = strings.Split(request.QueryStringParameters["exclude_types[]"], ",")
	_ = strings.Split(request.QueryStringParameters["types[]"], ",")
	_ = request.QueryStringParameters["account_id"]

	// Get notifications
	// TODO: Implement GetNotifications in storage
	notifications := []models.Notification{}
	cursor := ""

	// Set Link header for pagination if there's a cursor
	if cursor != "" {
		nextURL := fmt.Sprintf("%s/api/v1/notifications?max_id=%s", h.cfg.BaseURL(), cursor)
		body, _ := common.MarshalString(notifications)
		return &events.APIGatewayV2HTTPResponse{
			StatusCode: http.StatusOK,
			Headers: map[string]string{
				"Content-Type":                 "application/json",
				"Access-Control-Allow-Origin":  "*",
				"Access-Control-Allow-Headers": "Content-Type, Authorization",
				"Access-Control-Allow-Methods": "GET, POST, PUT, DELETE, OPTIONS",
				"Link":                         fmt.Sprintf(`<%s>; rel="next"`, nextURL),
			},
			Body: body,
		}, nil
	}

	return common.OK(notifications), nil
}

// HandleGetInstanceV2 returns instance information in v2 format
func (h *Handler) HandleGetInstanceV2(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Get static config
	instanceConfig := config.GetInstanceConfig()

	// Get dynamic rules from DynamoDB
	rules, err := h.store.GetInstanceRules(ctx)
	if err != nil {
		h.logger.Error("failed to get instance rules", zap.Error(err))
		rules = []storage.InstanceRule{}
	}

	// Convert rules to API format
	apiRules := make([]interface{}, len(rules))
	for i, rule := range rules {
		apiRules[i] = map[string]interface{}{
			"id":   rule.ID,
			"text": rule.Text,
		}
	}

	// V2 instance response format
	resp := map[string]interface{}{
		"domain":      strings.TrimPrefix(strings.TrimPrefix(h.cfg.BaseURL(), "https://"), "http://"),
		"title":       instanceConfig.Title,
		"version":     instanceConfig.Version,
		"source_url":  "https://github.com/aron23/lesser",
		"description": instanceConfig.Description,
		"usage": map[string]interface{}{
			"users": map[string]interface{}{
				"active_month": 1, // TODO: Implement actual counts
			},
		},
		"thumbnail": map[string]interface{}{
			"url": h.cfg.BaseURL() + "/assets/thumbnail.png",
		},
		"languages": instanceConfig.Languages,
		"configuration": map[string]interface{}{
			"urls": map[string]interface{}{
				"streaming": h.cfg.BaseURL(), // No streaming support yet
			},
			"accounts": map[string]interface{}{
				"max_featured_tags": 10,
			},
			"statuses": map[string]interface{}{
				"max_characters":              instanceConfig.MaxStatusChars,
				"max_media_attachments":       4,
				"characters_reserved_per_url": 23,
			},
			"media_attachments": map[string]interface{}{
				"supported_mime_types": []string{
					"image/jpeg",
					"image/png",
					"image/gif",
					"image/webp",
					"video/mp4",
					"video/webm",
				},
				"image_size_limit":       instanceConfig.MaxMediaSize,
				"image_matrix_limit":     16777216,
				"video_size_limit":       instanceConfig.MaxVideoSize,
				"video_frame_rate_limit": 60,
				"video_matrix_limit":     2304000,
			},
			"polls": map[string]interface{}{
				"max_options":               4,
				"max_characters_per_option": 50,
				"min_expiration":            300,
				"max_expiration":            2629746,
			},
			"translation": map[string]interface{}{
				"enabled": false,
			},
		},
		"registrations": map[string]interface{}{
			"enabled":           instanceConfig.RegistrationsOpen,
			"approval_required": instanceConfig.ApprovalRequired,
			"message":           nil,
		},
		"contact": map[string]interface{}{
			"email":   instanceConfig.Email,
			"account": nil, // TODO: Link to admin account
		},
		"rules": apiRules,
	}

	return common.OK(resp), nil
}
