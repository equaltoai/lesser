package handlers

import (
	"context"
	"errors"
	"fmt"
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
		// Parse limit from query parameters
		searchLimit := 20
		if limitStr := request.QueryStringParameters["limit"]; limitStr != "" {
			if l, err := fmt.Sscanf(limitStr, "%d", &searchLimit); err == nil && l == 1 {
				if searchLimit > 40 {
					searchLimit = 40
				}
			}
		}

		// Use the new SearchAccounts method
		actors, err := h.store.SearchAccounts(ctx, query, searchLimit, false, 0)
		if err != nil {
			h.logger.Error("account search failed",
				zap.String("query", query),
				zap.Error(err))
		} else {
			// Convert actors to accounts
			for _, actor := range actors {
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

				status := h.converter.ObjectToStatus(obj, objActor)
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

	// Parse query parameters
	params := request.QueryStringParameters

	// Handle filtering parameters
	filter := &storage.NotificationFilter{
		Limit: 20, // Default limit
	}

	// Parse limit
	if limitStr := params["limit"]; limitStr != "" {
		var limit int
		if _, err := fmt.Sscanf(limitStr, "%d", &limit); err == nil && limit > 0 {
			if limit > 40 {
				limit = 40
			}
			filter.Limit = limit
		}
	}

	// Parse types filter
	if types := params["types[]"]; types != "" {
		filter.Types = []string{types}
	} else if typesStr := params["types"]; typesStr != "" {
		filter.Types = strings.Split(typesStr, ",")
	}

	// Parse exclude_types filter
	if excludeTypes := params["exclude_types[]"]; excludeTypes != "" {
		filter.ExcludeTypes = []string{excludeTypes}
	} else if excludeTypesStr := params["exclude_types"]; excludeTypesStr != "" {
		filter.ExcludeTypes = strings.Split(excludeTypesStr, ",")
	}

	// Parse account_id filter
	if accountID := params["account_id"]; accountID != "" {
		filter.AccountID = accountID
	}

	// Parse pagination parameters
	filter.MaxID = params["max_id"]
	filter.MinID = params["min_id"]
	filter.SinceID = params["since_id"]

	// Get notifications
	notifications, cursor, err := h.store.GetNotificationsFiltered(ctx, claims.Username, filter)
	if err != nil {
		h.logger.Error("failed to get notifications",
			zap.String("username", claims.Username),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to get notifications")), nil
	}

	// Convert to API format
	apiNotifications := make([]*models.Notification, 0, len(notifications))
	for _, notif := range notifications {
		// Get the account that triggered the notification
		actor, err := h.store.GetActor(ctx, notif.AccountID)
		if err != nil {
			h.logger.Warn("failed to get actor for notification",
				zap.String("notification_id", notif.ID),
				zap.String("account_id", notif.AccountID),
				zap.Error(err))
			continue
		}

		account := h.converter.ActorToAccount(actor)
		apiNotif := &models.Notification{
			ID:        notif.ID,
			Type:      notif.Type,
			CreatedAt: notif.CreatedAt,
			Account:   account,
		}

		// Add status if applicable for certain notification types
		if notif.StatusID != "" && (notif.Type == models.NotificationTypeMention ||
			notif.Type == models.NotificationTypeFavourite ||
			notif.Type == models.NotificationTypeReblog) {

			// Get the status
			obj, err := h.store.GetObject(ctx, notif.StatusID)
			if err != nil {
				h.logger.Warn("failed to get status for notification",
					zap.String("notification_id", notif.ID),
					zap.String("status_id", notif.StatusID),
					zap.Error(err))
				continue
			}

			// Get status author (for converting to Status model)
			var statusActor *activitypub.Actor
			if note, ok := obj.(*activitypub.Note); ok && note.AttributedTo != "" {
				parts := strings.Split(note.AttributedTo, "/")
				if len(parts) > 0 {
					username := parts[len(parts)-1]
					statusActor, _ = h.store.GetActor(ctx, username)
				}
			}

			status := h.converter.ObjectToStatus(obj, statusActor)
			apiNotif.Status = &status
		}

		apiNotifications = append(apiNotifications, apiNotif)
	}

	// Build response with pagination header if needed
	response := common.OK(apiNotifications)

	if cursor != "" {
		baseURL := fmt.Sprintf("https://%s%s", request.Headers["host"], request.RawPath)
		nextURL := fmt.Sprintf("%s?max_id=%s", baseURL, cursor)
		if filter.Limit > 0 {
			nextURL += fmt.Sprintf("&limit=%d", filter.Limit)
		}
		response.Headers["Link"] = fmt.Sprintf(`<%s>; rel="next"`, nextURL)
	}

	return response, nil
}

// HandleGetInstanceV2 returns instance information in v2 format
func (h *Handler) HandleGetInstanceV2(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Get static config
	instanceConfig := config.GetInstanceConfig()

	// Log configuration values
	h.logger.Info("HandleGetInstanceV2 called",
		zap.String("cfg.Domain", h.cfg.Domain),
		zap.String("cfg.BaseURL", h.cfg.BaseURL()),
		zap.String("instanceConfig.Title", instanceConfig.Title),
		zap.String("instanceConfig.Version", instanceConfig.Version),
	)

	// Get rules from storage
	rules, err := h.store.GetInstanceRules(ctx)
	if err != nil {
		h.logger.Warn("failed to get instance rules", zap.Error(err))
		rules = []storage.InstanceRule{}
	}

	// Get VAPID public key
	var vapidPublicKey string
	vapidKeys, err := h.store.GetVAPIDKeys(ctx)
	if err != nil {
		h.logger.Warn("failed to get VAPID keys, using placeholder", zap.Error(err))
		// Use a placeholder that clients will recognize as invalid
		vapidPublicKey = "BCkMmVdKDnKYwzVCDC99Iuc9GvId-x7-kKtuHnLgfF98ENiZp_aj-UNthbCdI70DqN1zUVis-x0Wrot2sBagkMc="
	} else {
		vapidPublicKey = vapidKeys.PublicKey
	}

	// Convert rules for API response
	apiRules := make([]map[string]interface{}, len(rules))
	for i, rule := range rules {
		apiRules[i] = map[string]interface{}{
			"id":   rule.ID,
			"text": rule.Text,
		}
	}

	resp := map[string]interface{}{
		"domain":      h.cfg.Domain,
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
		"icon":      []interface{}{},
		"languages": instanceConfig.Languages,
		"configuration": map[string]interface{}{
			"urls": map[string]interface{}{
				"streaming":        h.cfg.BaseURL(), // No streaming support yet
				"about":            h.cfg.BaseURL() + "/about",
				"privacy_policy":   h.cfg.BaseURL() + "/privacy-policy",
				"terms_of_service": h.cfg.BaseURL() + "/terms",
			},
			"vapid": map[string]interface{}{
				"public_key": vapidPublicKey,
			},
			"accounts": map[string]interface{}{
				"max_featured_tags":   10,
				"max_pinned_statuses": 4,
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
				"description_limit":      1500,
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
			"limited_federation": false,
		},
		"registrations": map[string]interface{}{
			"enabled":           instanceConfig.RegistrationsOpen,
			"approval_required": instanceConfig.ApprovalRequired,
			"message":           nil,
			"min_age":           nil,
			"reason_required":   false,
		},
		"api_versions": map[string]interface{}{
			"mastodon": 1,
		},
		"contact": map[string]interface{}{
			"email":   instanceConfig.Email,
			"account": nil, // TODO: Link to admin account
		},
		"rules": apiRules,
	}

	// Log the response to debug
	h.logger.Info("HandleGetInstanceV2 response",
		zap.Any("domain", resp["domain"]),
		zap.Any("title", resp["title"]),
		zap.Any("version", resp["version"]),
		zap.Any("full_response_keys", getMapKeys(resp)),
	)

	return common.OK(resp), nil
}

// Helper function to get map keys for logging
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// HandleGetNotification handles GET /api/v1/notifications/:id
func (h *Handler) HandleGetNotification(ctx context.Context, request events.APIGatewayV2HTTPRequest, notificationID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token
	token, err := auth.ExtractBearerToken(request.Headers["Authorization"])
	if err != nil {
		token, err = auth.ExtractBearerToken(request.Headers["authorization"])
		if err != nil {
			return common.Unauthorized(err), nil
		}
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check read scope
	if !claims.HasScope("read:notifications") && !claims.HasScope(auth.ScopeRead) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Get notification
	notification, err := h.store.GetNotification(ctx, notificationID)
	if err != nil {
		return common.NotFound(fmt.Errorf("notification not found")), nil
	}

	// Verify ownership
	if notification.Username != claims.Username {
		return common.NotFound(fmt.Errorf("notification not found")), nil
	}

	// Get the account that triggered the notification
	actor, err := h.store.GetActor(ctx, notification.AccountID)
	if err != nil {
		h.logger.Error("failed to get actor for notification",
			zap.String("notification_id", notification.ID),
			zap.String("account_id", notification.AccountID),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to get notification details")), nil
	}

	account := h.converter.ActorToAccount(actor)
	apiNotif := &models.Notification{
		ID:        notification.ID,
		Type:      notification.Type,
		CreatedAt: notification.CreatedAt,
		Account:   account,
	}

	// Add status if applicable
	if notification.StatusID != "" && (notification.Type == models.NotificationTypeMention ||
		notification.Type == models.NotificationTypeFavourite ||
		notification.Type == models.NotificationTypeReblog) {

		obj, err := h.store.GetObject(ctx, notification.StatusID)
		if err == nil {
			var statusActor *activitypub.Actor
			if note, ok := obj.(*activitypub.Note); ok && note.AttributedTo != "" {
				parts := strings.Split(note.AttributedTo, "/")
				if len(parts) > 0 {
					username := parts[len(parts)-1]
					statusActor, _ = h.store.GetActor(ctx, username)
				}
			}
			status := h.converter.ObjectToStatus(obj, statusActor)
			apiNotif.Status = &status
		}
	}

	return common.OK(apiNotif), nil
}

// HandleClearNotifications handles POST /api/v1/notifications/clear
func (h *Handler) HandleClearNotifications(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token
	token, err := auth.ExtractBearerToken(request.Headers["Authorization"])
	if err != nil {
		token, err = auth.ExtractBearerToken(request.Headers["authorization"])
		if err != nil {
			return common.Unauthorized(err), nil
		}
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check write scope
	if !claims.HasScope("write:notifications") && !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Clear all notifications
	if err := h.store.ClearNotifications(ctx, claims.Username); err != nil {
		h.logger.Error("failed to clear notifications",
			zap.String("username", claims.Username),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to clear notifications")), nil
	}

	return common.NoContent(), nil
}

// HandleDismissNotification handles POST /api/v1/notifications/:id/dismiss
func (h *Handler) HandleDismissNotification(ctx context.Context, request events.APIGatewayV2HTTPRequest, notificationID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token
	token, err := auth.ExtractBearerToken(request.Headers["Authorization"])
	if err != nil {
		token, err = auth.ExtractBearerToken(request.Headers["authorization"])
		if err != nil {
			return common.Unauthorized(err), nil
		}
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check write scope
	if !claims.HasScope("write:notifications") && !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Get notification to verify ownership
	notification, err := h.store.GetNotification(ctx, notificationID)
	if err != nil {
		return common.NotFound(fmt.Errorf("notification not found")), nil
	}

	// Verify ownership
	if notification.Username != claims.Username {
		return common.NotFound(fmt.Errorf("notification not found")), nil
	}

	// Delete notification
	if err := h.store.DeleteNotification(ctx, notificationID); err != nil {
		h.logger.Error("failed to dismiss notification",
			zap.String("notification_id", notificationID),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to dismiss notification")), nil
	}

	return common.NoContent(), nil
}
