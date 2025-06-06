package handlers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aron23/lesser/cmd/api/models"
	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/config"
	"github.com/aron23/lesser/pkg/cost"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-lambda-go/events"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
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
		// Parse limit for hashtag search
		hashtagLimit := 20
		if limitStr := request.QueryStringParameters["limit"]; limitStr != "" {
			if l, err := fmt.Sscanf(limitStr, "%d", &hashtagLimit); err == nil && l == 1 {
				if hashtagLimit > 40 {
					hashtagLimit = 40
				}
			}
		}

		// Search for hashtags in the database
		hashtags, err := h.store.SearchHashtags(ctx, query, hashtagLimit)
		if err != nil {
			h.logger.Warn("hashtag search failed", zap.Error(err))
		} else {
			// Convert storage hashtags to API format
			for _, hashtag := range hashtags {
				// Get usage history for the last 7 days
				history, _ := h.store.GetHashtagUsageHistory(ctx, hashtag.Name, 7)

				// Convert history to API format
				apiHistory := make([]struct {
					Day      string `json:"day"`
					Uses     string `json:"uses"`
					Accounts string `json:"accounts"`
				}, 0)

				// Create history entries (most recent first)
				for i := 0; i < len(history) && i < 7; i++ {
					day := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
					apiHistory = append(apiHistory, struct {
						Day      string `json:"day"`
						Uses     string `json:"uses"`
						Accounts string `json:"accounts"`
					}{
						Day:      day,
						Uses:     fmt.Sprintf("%d", history[i]),
						Accounts: "0", // TODO: Track unique accounts per day
					})
				}

				tag := models.Tag{
					Name:    hashtag.Name,
					URL:     hashtag.URL,
					History: apiHistory,
				}
				result.Hashtags = append(result.Hashtags, tag)
			}
		}

		// If no results and query starts with #, create a placeholder
		if len(result.Hashtags) == 0 && strings.HasPrefix(query, "#") {
			tagName := strings.ToLower(strings.TrimPrefix(query, "#"))
			tag := models.Tag{
				Name: tagName,
				URL:  fmt.Sprintf("%s/tags/%s", h.cfg.BaseURL(), tagName),
				History: []struct {
					Day      string `json:"day"`
					Uses     string `json:"uses"`
					Accounts string `json:"accounts"`
				}{
					{
						Day:      time.Now().Format("2006-01-02"),
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

// HandleSearchV2 handles GET /api/v2/search requests - returns same format as v1
func (h *Handler) HandleSearchV2(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// V2 search has the same implementation as V1 in Lesser
	// The main difference in Mastodon is that v2 groups results by type,
	// but our v1 already returns grouped results
	return h.HandleSearch(ctx, request)
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
				"streaming":        fmt.Sprintf("wss://ws.%s", h.cfg.Domain),
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

// HandleGetInstanceCosts returns cost analytics for the instance
func (h *Handler) HandleGetInstanceCosts(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	h.logger.Info("HandleGetInstanceCosts called")

	// Initialize cost storage if not already done
	costTableName := os.Getenv("COST_HISTORY_TABLE_NAME")
	if costTableName == "" {
		// Return placeholder data if cost tracking is not configured
		response := map[string]interface{}{
			"error": "Cost tracking not configured",
		}
		return common.OK(response), nil
	}

	// Create DynamoDB client for cost queries
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(h.cfg.Region),
	)
	if err != nil {
		h.logger.Error("failed to load AWS config", zap.Error(err))
		return nil, errors.New("failed to initialize cost tracking")
	}

	dynamoClient := dynamodb.NewFromConfig(awsCfg)
	costStorage := cost.NewStorage(dynamoClient, costTableName, h.logger)

	// Get current month data
	now := time.Now()
	currentMonth, err := costStorage.GetMonthlyCost(ctx, now.Year(), now.Month())
	if err != nil {
		h.logger.Error("failed to get monthly cost", zap.Error(err))
	}

	// Get daily costs for the last 7 days
	endDate := now
	startDate := now.AddDate(0, 0, -6) // 7 days including today
	dailyCosts, err := costStorage.GetDailyCosts(ctx, startDate, endDate)
	if err != nil {
		h.logger.Error("failed to get daily costs", zap.Error(err))
	}

	// Format daily costs for response
	formattedDailyCosts := make([]map[string]interface{}, 0, len(dailyCosts))
	for _, daily := range dailyCosts {
		formattedDailyCosts = append(formattedDailyCosts, map[string]interface{}{
			"date":          daily.Date,
			"cost_cents":    float64(daily.TotalCostMicrocents) / float64(cost.MicroCentsToCents),
			"request_count": daily.RequestCount,
			"unique_users":  daily.UniqueUsers,
		})
	}

	// Calculate cost breakdown percentages
	var dynamoPercent, lambdaPercent, transferPercent, storagePercent float64
	if currentMonth != nil && currentMonth.TotalCostMicrocents > 0 {
		// Calculate component costs
		dynamoCost := (currentMonth.DynamoDBReads * cost.DynamoDBReadRequestUnit / 1000000) +
			(currentMonth.DynamoDBWrites * cost.DynamoDBWriteRequestUnit / 1000000)

		lambdaCost := (currentMonth.LambdaInvocations * cost.LambdaRequestCost / 1000000) +
			int64(float64(currentMonth.LambdaDurationMs)*128/(1000*1024)*float64(cost.LambdaGBSecondCost))

		transferCost := int64(currentMonth.DataTransferGB * float64(cost.S3DataTransferPerGB))

		// Storage cost is the remainder
		storageCost := currentMonth.TotalCostMicrocents - dynamoCost - lambdaCost - transferCost
		if storageCost < 0 {
			storageCost = 0
		}

		// Calculate percentages
		total := float64(currentMonth.TotalCostMicrocents)
		dynamoPercent = float64(dynamoCost) / total * 100
		lambdaPercent = float64(lambdaCost) / total * 100
		transferPercent = float64(transferCost) / total * 100
		storagePercent = float64(storageCost) / total * 100
	}

	// Calculate cost per user
	var avgCostPerUser, medianCostPerUser float64
	if currentMonth != nil && currentMonth.UniqueUsers > 0 {
		avgCostPerUser = float64(currentMonth.TotalCostMicrocents) / float64(currentMonth.UniqueUsers) / float64(cost.MicroCentsToCents)
		// For now, use average as median (would need more detailed data for true median)
		medianCostPerUser = avgCostPerUser
	}

	response := map[string]interface{}{
		"current_month": map[string]interface{}{
			"total_cost_cents":     float64(currentMonth.TotalCostMicrocents) / float64(cost.MicroCentsToCents),
			"dynamodb_reads":       currentMonth.DynamoDBReads,
			"dynamodb_writes":      currentMonth.DynamoDBWrites,
			"lambda_invocations":   currentMonth.LambdaInvocations,
			"data_transfer_gb":     currentMonth.DataTransferGB,
			"projected_cost_cents": float64(currentMonth.ProjectedCostMicrocents) / float64(cost.MicroCentsToCents),
		},
		"daily_costs": formattedDailyCosts,
		"cost_per_user": map[string]interface{}{
			"average_cents": avgCostPerUser,
			"median_cents":  medianCostPerUser,
		},
		"cost_breakdown": map[string]interface{}{
			"dynamodb_percent":      dynamoPercent,
			"lambda_percent":        lambdaPercent,
			"data_transfer_percent": transferPercent,
			"storage_percent":       storagePercent,
		},
	}

	return common.OK(response), nil
}

// HandleGetInstanceConfiguration returns configuration details
func (h *Handler) HandleGetInstanceConfiguration(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Build configuration response
	config := map[string]interface{}{
		"urls": map[string]interface{}{
			"streaming": fmt.Sprintf("wss://ws.%s", h.cfg.Domain),
		},
		"accounts": map[string]interface{}{
			"max_featured_tags":   20,
			"max_pinned_statuses": 5,
		},
		"statuses": map[string]interface{}{
			"max_characters":              5000,
			"max_media_attachments":       4,
			"characters_reserved_per_url": 23,
		},
		"media_attachments": map[string]interface{}{
			"supported_mime_types": []string{
				"image/jpeg",
				"image/png",
				"image/gif",
				"image/heif",
				"image/heic",
				"image/webp",
				"image/avif",
				"video/webm",
				"video/mp4",
				"video/quicktime",
				"video/ogg",
				"audio/wave",
				"audio/wav",
				"audio/x-wav",
				"audio/x-pn-wave",
				"audio/vnd.wave",
				"audio/ogg",
				"audio/mpeg",
				"audio/mp3",
				"audio/webm",
				"audio/flac",
				"audio/aac",
				"audio/m4a",
				"audio/x-m4a",
				"audio/mp4",
				"audio/3gpp",
				"video/x-ms-asf",
			},
			"image_size_limit":       16777216,  // 16MB
			"image_matrix_limit":     33177600,  // 33MP
			"video_size_limit":       103809024, // 99MB
			"video_frame_rate_limit": 120,
			"video_matrix_limit":     8294400, // 4K
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
	}

	// TODO: Add vapid_key when available in config

	return common.OK(config), nil
}
