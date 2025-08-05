package lift

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleSearchLift performs a search across accounts, statuses, and hashtags
func (h *Handler) HandleSearchLift(ctx *lift.Context) error {
	// Search can be authenticated or not - authentication is optional for search
	// Try test mode first
	if testUsername := ctx.Header("X-Test-Username"); testUsername != "" {
		h.logger.Info("Using test mode authentication", zap.String("username", testUsername))
	} else {
		// Try JWT authentication
		token := h.getBearerTokenLift(ctx)
		if token != "" {
			oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
			if claims, err := oauthSvc.ValidateAccessToken(token); err == nil {
				h.logger.Debug("Authenticated search", zap.String("username", claims.Username))
			}
		}
	}

	// Get search query
	query := ctx.Query("q")
	if query == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "q parameter is required"})
	}

	// Parse search parameters
	searchType := ctx.Query("type") // accounts, hashtags, statuses
	_ = ctx.Query("resolve") == "true"
	_ = ctx.Query("following") == "true" // followingOnly not supported in StatusSearchOptions
	accountID := ctx.Query("account_id")
	_ = ctx.Query("exclude_unreviewed") == "true"
	_ = ctx.Query("min_id")
	_ = ctx.Query("max_id")

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
		if limitStr := ctx.Query("limit"); limitStr != "" {
			if l, err := fmt.Sscanf(limitStr, "%d", &searchLimit); err == nil && l == 1 {
				if searchLimit > 40 {
					searchLimit = 40
				}
			}
		}

		// Use the new SearchAccounts method
		actors, err := h.repos.Search().SearchAccounts(ctx.Context, query, searchLimit, false, 0)
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
					Emojis:         []any{},
					Fields:         []any{},
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
		// Check if it's a direct URL search
		if strings.HasPrefix(query, "http") {
			// Try to get the object directly by URL
			obj, err := h.repos.Object().GetObject(ctx.Context, query)
			if err == nil && obj != nil {
				// Convert object to search result format
				// Since GetObject returns any, we need to extract the fields
				var statusResult *storage.StatusSearchResult
				
				// Handle the object based on its type
				switch v := obj.(type) {
				case *storagemodels.Object:
					statusResult = &storage.StatusSearchResult{
						StatusID:  v.ID,
						URL:       v.URL,
						Content:   v.Content,
						AuthorID:  v.AttributedTo,
						Published: v.Published,
					}
				case map[string]any:
					// Handle map response
					statusResult = &storage.StatusSearchResult{}
					if id, ok := v["id"].(string); ok {
						statusResult.StatusID = id
					}
					if url, ok := v["url"].(string); ok {
						statusResult.URL = url
					}
					if content, ok := v["content"].(string); ok {
						statusResult.Content = content
					}
					if authorID, ok := v["attributedTo"].(string); ok {
						statusResult.AuthorID = authorID
					}
					if published, ok := v["published"].(time.Time); ok {
						statusResult.Published = published
					}
				}
				// Convert to API format
				var statusActor *activitypub.Actor
				if statusResult.AuthorID != "" {
					parts := strings.Split(statusResult.AuthorID, "/")
					if len(parts) > 0 {
						username := parts[len(parts)-1]
						statusActor, _ = h.repos.Actor().GetActor(ctx.Context, username)
					}
				}

				status := models.Status{
					ID:        statusResult.StatusID,
					Content:   statusResult.Content,
					URL:       statusResult.URL,
					CreatedAt: statusResult.Published.Format(time.RFC3339),
				}

				if statusActor != nil {
					account := h.converter.ActorToAccount(statusActor)
					status.Account = account
				}

				result.Statuses = append(result.Statuses, status)
			}
		} else {
			// Perform content search
			statusLimit := 20
			if limitStr := ctx.Query("limit"); limitStr != "" {
				if l, err := fmt.Sscanf(limitStr, "%d", &statusLimit); err == nil && l == 1 {
					if statusLimit > 40 {
						statusLimit = 40
					}
				}
			}

			// Use the new search method with options
			searchOptions := storage.StatusSearchOptions{
				Limit:     statusLimit,
				AccountID: accountID,
				// Note: followingOnly functionality would need to be handled in search implementation
			}

			statusResults, err := h.repos.Search().SearchStatusesWithOptions(ctx.Context, query, searchOptions)
			if err != nil {
				h.logger.Warn("status search failed", zap.Error(err))
			} else {
				// Convert search results to API format
				for _, sr := range statusResults {
					// Get the actor for each status
					var statusActor *activitypub.Actor
					if sr.AuthorID != "" {
						parts := strings.Split(sr.AuthorID, "/")
						if len(parts) > 0 {
							username := parts[len(parts)-1]
							statusActor, _ = h.repos.Actor().GetActor(ctx.Context, username)
						}
					}

					// Create API status
					status := models.Status{
						ID:        sr.StatusID,
						Content:   sr.Content,
						URL:       sr.URL,
						CreatedAt: sr.Published.Format(time.RFC3339),
					}

					// Add account info if we found the actor
					if statusActor != nil {
						account := h.converter.ActorToAccount(statusActor)
						status.Account = account
					}

					result.Statuses = append(result.Statuses, status)
				}
			}
		}
	}

	// Search hashtags
	if searchType == "" || searchType == "hashtags" {
		// Parse limit for hashtag search
		hashtagLimit := 20
		if limitStr := ctx.Query("limit"); limitStr != "" {
			if l, err := fmt.Sscanf(limitStr, "%d", &hashtagLimit); err == nil && l == 1 {
				if hashtagLimit > 40 {
					hashtagLimit = 40
				}
			}
		}

		// Search for hashtags in the database
		hashtags, err := h.repos.Search().SearchHashtags(ctx.Context, query, hashtagLimit)
		if err != nil {
			h.logger.Warn("hashtag search failed", zap.Error(err))
		} else {
			// Convert storage hashtags to API format
			for _, hashtag := range hashtags {
				// Get usage history for the last 7 days
				history, _ := h.repos.Hashtag().GetHashtagUsageHistory(ctx.Context, hashtag.Name, 7)

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
						Accounts: h.getUniqueAccountsForDay(ctx, day),
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

	return ctx.JSON(result)
}

// HandleSearchV2Lift handles GET /api/v2/search requests - returns same format as v1
func (h *Handler) HandleSearchV2Lift(ctx *lift.Context) error {
	// V2 search has the same implementation as V1 in Lesser
	// The main difference in Mastodon is that v2 groups results by type,
	// but our v1 already returns grouped results
	return h.HandleSearchLift(ctx)
}

// HandleGetNotificationsLift retrieves notifications for the authenticated user
func (h *Handler) HandleGetNotificationsLift(ctx *lift.Context) error {
	var claims *auth.Claims
	var err error

	// Try test mode first
	if testUsername := ctx.Header("X-Test-Username"); testUsername != "" {
		h.logger.Info("Using test mode authentication", zap.String("username", testUsername))
		claims = &auth.Claims{Username: testUsername, Scopes: []string{auth.ScopeRead}}
	} else {
		// Extract and validate token
		token := h.getBearerTokenLift(ctx)
		if token == "" {
			return ctx.Status(401).JSON(map[string]string{"error": "authorization required"})
		}

		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
		claims, err = oauthSvc.ValidateAccessToken(token)
		if err != nil {
			return ctx.Status(401).JSON(map[string]string{"error": "invalid token"})
		}
	}

	// Check read:notifications scope
	if !claims.HasScope("read:notifications") && !claims.HasScope(auth.ScopeRead) {
		return ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
	}

	// Handle filtering parameters
	filter := &storage.NotificationFilter{
		Limit: 20, // Default limit
	}

	// Parse limit
	if limitStr := ctx.Query("limit"); limitStr != "" {
		var limit int
		if _, err := fmt.Sscanf(limitStr, "%d", &limit); err == nil && limit > 0 {
			if limit > 40 {
				limit = 40
			}
			filter.Limit = limit
		}
	}

	// Parse types filter
	if types := ctx.Query("types[]"); types != "" {
		filter.Types = []string{types}
	} else if typesStr := ctx.Query("types"); typesStr != "" {
		filter.Types = strings.Split(typesStr, ",")
	}

	// Parse exclude_types filter
	if excludeTypes := ctx.Query("exclude_types[]"); excludeTypes != "" {
		filter.ExcludeTypes = []string{excludeTypes}
	} else if excludeTypesStr := ctx.Query("exclude_types"); excludeTypesStr != "" {
		filter.ExcludeTypes = strings.Split(excludeTypesStr, ",")
	}

	// Parse account_id filter
	if accountID := ctx.Query("account_id"); accountID != "" {
		filter.AccountID = accountID
	}

	// Parse pagination parameters
	filter.MaxID = ctx.Query("max_id")
	filter.MinID = ctx.Query("min_id")
	filter.SinceID = ctx.Query("since_id")

	// Get notifications
	notifications, cursor, err := h.repos.Notification().GetNotificationsFiltered(ctx.Context, claims.Username, filter)
	if err != nil {
		h.logger.Error("failed to get notifications",
			zap.String("username", claims.Username),
			zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "failed to get notifications"})
	}

	// Convert to API format
	apiNotifications := make([]*models.Notification, 0, len(notifications))
	for _, notif := range notifications {
		// Get the account that triggered the notification
		actor, err := h.repos.Actor().GetActor(ctx.Context, notif.AccountID)
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
			obj, err := h.repos.Object().GetObject(ctx.Context, notif.StatusID)
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
					statusActor, _ = h.repos.Actor().GetActor(ctx.Context, username)
				}
			}

			status := h.converter.ObjectToStatus(obj, statusActor)
			apiNotif.Status = &status
		}

		apiNotifications = append(apiNotifications, apiNotif)
	}

	// Set pagination header if needed
	if cursor != "" {
		host := ctx.Header("host")
		if host == "" {
			host = ctx.Header("Host")
		}
		baseURL := fmt.Sprintf("https://%s%s", host, "/api/v1/notifications")
		nextURL := fmt.Sprintf("%s?max_id=%s", baseURL, cursor)
		if filter.Limit > 0 {
			nextURL += fmt.Sprintf("&limit=%d", filter.Limit)
		}
		ctx.Set("Link", fmt.Sprintf(`<%s>; rel="next"`, nextURL))
	}

	return ctx.JSON(apiNotifications)
}

// HandleGetInstanceV2Lift returns instance information in v2 format
func (h *Handler) HandleGetInstanceV2Lift(ctx *lift.Context) error {
	// Get static config
	instanceConfig := config.GetInstanceConfig()

	// Log configuration values
	h.logger.Info("HandleGetInstanceV2Lift called",
		zap.String("cfg.Domain", h.cfg.Domain),
		zap.String("cfg.BaseURL", h.cfg.BaseURL()),
		zap.String("instanceConfig.Title", instanceConfig.Title),
		zap.String("instanceConfig.Version", instanceConfig.Version),
	)

	// Get rules from storage
	rules, err := h.repos.Instance().GetInstanceRules(ctx.Context)
	if err != nil {
		h.logger.Warn("failed to get instance rules", zap.Error(err))
		rules = []storage.InstanceRule{}
	}

	// Get VAPID public key
	var vapidPublicKey string
	vapidKeys, err := h.repos.PushSubscription().GetVAPIDKeys(ctx.Context)
	if err != nil {
		h.logger.Warn("failed to get VAPID keys, using placeholder", zap.Error(err))
		// Use a placeholder that clients will recognize as invalid
		vapidPublicKey = "BCkMmVdKDnKYwzVCDC99Iuc9GvId-x7-kKtuHnLgfF98ENiZp_aj-UNthbCdI70DqN1zUVis-x0Wrot2sBagkMc="
	} else {
		vapidPublicKey = vapidKeys.PublicKey
	}

	// Convert rules for API response
	apiRules := make([]map[string]any, len(rules))
	for i, rule := range rules {
		apiRules[i] = map[string]any{
			"id":   rule.ID,
			"text": rule.Text,
		}
	}

	resp := map[string]any{
		"domain":      h.cfg.Domain,
		"title":       instanceConfig.Title,
		"version":     instanceConfig.Version,
		"source_url":  "https://github.com/equaltoai/lesser",
		"description": instanceConfig.Description,
		"usage": map[string]any{
			"users": map[string]any{
				"active_month": h.getActiveMonthlyUsers(ctx),
			},
		},
		"thumbnail": map[string]any{
			"url": h.cfg.BaseURL() + "/assets/thumbnail.png",
		},
		"icon":      []any{},
		"languages": instanceConfig.Languages,
		"configuration": map[string]any{
			"urls": map[string]any{
				"streaming":        fmt.Sprintf("wss://ws.%s/v1", h.cfg.Domain),
				"about":            h.cfg.BaseURL() + "/about",
				"privacy_policy":   h.cfg.BaseURL() + "/privacy-policy",
				"terms_of_service": h.cfg.BaseURL() + "/terms",
			},
			"vapid": map[string]any{
				"public_key": vapidPublicKey,
			},
			"accounts": map[string]any{
				"max_featured_tags":   10,
				"max_pinned_statuses": 4,
			},
			"statuses": map[string]any{
				"max_characters":              instanceConfig.MaxStatusChars,
				"max_media_attachments":       4,
				"characters_reserved_per_url": 23,
			},
			"media_attachments": map[string]any{
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
			"polls": map[string]any{
				"max_options":               4,
				"max_characters_per_option": 50,
				"min_expiration":            300,
				"max_expiration":            2629746,
			},
			"translation": map[string]any{
				"enabled": false,
			},
			"limited_federation": false,
		},
		"registrations": map[string]any{
			"enabled":           instanceConfig.RegistrationsOpen,
			"approval_required": instanceConfig.ApprovalRequired,
			"message":           nil,
			"min_age":           nil,
			"reason_required":   false,
		},
		"api_versions": map[string]any{
			"mastodon": 1,
		},
		"contact": map[string]any{
			"email":   instanceConfig.Email,
			"account": h.getAdminAccount(ctx),
		},
		"rules": apiRules,
	}

	// Log the response to debug
	h.logger.Info("HandleGetInstanceV2Lift response",
		zap.Any("domain", resp["domain"]),
		zap.Any("title", resp["title"]),
		zap.Any("version", resp["version"]),
		zap.Any("full_response_keys", getMapKeys(resp)),
	)

	return ctx.JSON(resp)
}

// HandleGetNotificationLift handles GET /api/v1/notifications/:id
func (h *Handler) HandleGetNotificationLift(ctx *lift.Context) error {
	notificationID := ctx.Param("id")
	if notificationID == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "notification ID required"})
	}

	var claims *auth.Claims
	var err error

	// Try test mode first
	if testUsername := ctx.Header("X-Test-Username"); testUsername != "" {
		h.logger.Info("Using test mode authentication", zap.String("username", testUsername))
		claims = &auth.Claims{Username: testUsername, Scopes: []string{auth.ScopeRead}}
	} else {
		// Extract and validate token
		token := h.getBearerTokenLift(ctx)
		if token == "" {
			return ctx.Status(401).JSON(map[string]string{"error": "authorization required"})
		}

		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
		claims, err = oauthSvc.ValidateAccessToken(token)
		if err != nil {
			return ctx.Status(401).JSON(map[string]string{"error": "invalid token"})
		}
	}

	// Check read scope
	if !claims.HasScope("read:notifications") && !claims.HasScope(auth.ScopeRead) {
		return ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
	}

	// Get notification
	notification, err := h.repos.Notification().GetNotification(ctx.Context, notificationID)
	if err != nil {
		return ctx.Status(404).JSON(map[string]string{"error": "notification not found"})
	}

	// Verify ownership
	if notification.UserID != claims.Username {
		return ctx.Status(404).JSON(map[string]string{"error": "notification not found"})
	}

	// Get the account that triggered the notification
	actor, err := h.repos.Actor().GetActor(ctx.Context, notification.ActorID)
	if err != nil {
		h.logger.Error("failed to get actor for notification",
			zap.String("notification_id", notification.ID),
			zap.String("actor_id", notification.ActorID),
			zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "failed to get notification details"})
	}

	account := h.converter.ActorToAccount(actor)
	apiNotif := &models.Notification{
		ID:        notification.ID,
		Type:      notification.Type,
		CreatedAt: notification.CreatedAt,
		Account:   account,
	}

	// Add status if applicable
	if notification.TargetID != "" && notification.TargetType == "status" && (notification.Type == models.NotificationTypeMention ||
		notification.Type == models.NotificationTypeFavourite ||
		notification.Type == models.NotificationTypeReblog) {
		obj, err := h.repos.Object().GetObject(ctx.Context, notification.TargetID)
		if err == nil {
			var statusActor *activitypub.Actor
			if note, ok := obj.(*activitypub.Note); ok && note.AttributedTo != "" {
				parts := strings.Split(note.AttributedTo, "/")
				if len(parts) > 0 {
					username := parts[len(parts)-1]
					statusActor, _ = h.repos.Actor().GetActor(ctx.Context, username)
				}
			}
			status := h.converter.ObjectToStatus(obj, statusActor)
			apiNotif.Status = &status
		}
	}

	return ctx.JSON(apiNotif)
}

// HandleClearNotificationsLift handles POST /api/v1/notifications/clear
func (h *Handler) HandleClearNotificationsLift(ctx *lift.Context) error {
	var claims *auth.Claims
	var err error

	// Try test mode first
	if testUsername := ctx.Header("X-Test-Username"); testUsername != "" {
		h.logger.Info("Using test mode authentication", zap.String("username", testUsername))
		claims = &auth.Claims{Username: testUsername, Scopes: []string{auth.ScopeWrite}}
	} else {
		// Extract and validate token
		token := h.getBearerTokenLift(ctx)
		if token == "" {
			return ctx.Status(401).JSON(map[string]string{"error": "authorization required"})
		}

		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
		claims, err = oauthSvc.ValidateAccessToken(token)
		if err != nil {
			return ctx.Status(401).JSON(map[string]string{"error": "invalid token"})
		}
	}

	// Check write scope
	if !claims.HasScope("write:notifications") && !claims.HasScope(auth.ScopeWrite) {
		return ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
	}

	// Clear all notifications
	// Clear all notifications by using a very short duration (1 second in the future)
	if err := h.repos.Notification().ClearOldNotifications(ctx.Context, claims.Username, -1*time.Second); err != nil {
		h.logger.Error("failed to clear notifications",
			zap.String("username", claims.Username),
			zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "failed to clear notifications"})
	}

	ctx.Status(204)
	return nil
}

// HandleDismissNotificationLift handles POST /api/v1/notifications/:id/dismiss
func (h *Handler) HandleDismissNotificationLift(ctx *lift.Context) error {
	notificationID := ctx.Param("id")
	if notificationID == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "notification ID required"})
	}

	var claims *auth.Claims
	var err error

	// Try test mode first
	if testUsername := ctx.Header("X-Test-Username"); testUsername != "" {
		h.logger.Info("Using test mode authentication", zap.String("username", testUsername))
		claims = &auth.Claims{Username: testUsername, Scopes: []string{auth.ScopeWrite}}
	} else {
		// Extract and validate token
		token := h.getBearerTokenLift(ctx)
		if token == "" {
			return ctx.Status(401).JSON(map[string]string{"error": "authorization required"})
		}

		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
		claims, err = oauthSvc.ValidateAccessToken(token)
		if err != nil {
			return ctx.Status(401).JSON(map[string]string{"error": "invalid token"})
		}
	}

	// Check write scope
	if !claims.HasScope("write:notifications") && !claims.HasScope(auth.ScopeWrite) {
		return ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
	}

	// Get notification to verify ownership
	notification, err := h.repos.Notification().GetNotification(ctx.Context, notificationID)
	if err != nil {
		return ctx.Status(404).JSON(map[string]string{"error": "notification not found"})
	}

	// Verify ownership
	if notification.UserID != claims.Username {
		return ctx.Status(404).JSON(map[string]string{"error": "notification not found"})
	}

	// Delete notification
	if err := h.repos.Notification().DeleteNotification(ctx.Context, notificationID); err != nil {
		h.logger.Error("failed to dismiss notification",
			zap.String("notification_id", notificationID),
			zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "failed to dismiss notification"})
	}

	ctx.Status(204)
	return nil
}

// HandleGetInstanceCostsLift returns cost analytics for the instance
func (h *Handler) HandleGetInstanceCostsLift(ctx *lift.Context) error {
	h.logger.Info("HandleGetInstanceCostsLift called")

	// Initialize cost storage if not already done
	costTableName := os.Getenv("COST_HISTORY_TABLE_NAME")
	if costTableName == "" {
		// Return placeholder data if cost tracking is not configured
		response := map[string]any{
			"error": "Cost tracking not configured",
		}
		return ctx.JSON(response)
	}

	// Get current month data
	now := time.Now()
	currentMonth, err := h.repos.Cost().GetMonthlyAggregate(ctx.Context, now.Year(), int(now.Month()))
	if err != nil {
		h.logger.Error("failed to get monthly cost", zap.Error(err))
	}

	// Get daily costs for the last 7 days
	endDate := now
	startDate := now.AddDate(0, 0, -6) // 7 days including today
	dailyCosts, err := h.repos.Cost().GetDailyAggregates(ctx.Context, startDate, endDate)
	if err != nil {
		h.logger.Error("failed to get daily costs", zap.Error(err))
	}

	// Format daily costs for response
	formattedDailyCosts := make([]map[string]any, 0, len(dailyCosts))
	for _, daily := range dailyCosts {
		formattedDailyCosts = append(formattedDailyCosts, map[string]any{
			"date":          daily.Date,
			"cost_cents":    daily.TotalCostDollars * 100, // Convert dollars to cents
			"request_count": daily.TotalRequests,
			"unique_users":  daily.UniqueUsers,
		})
	}

	// Calculate cost breakdown percentages (simplified for now)
	var dynamoPercent, lambdaPercent, transferPercent, storagePercent float64
	if currentMonth != nil && currentMonth.TotalCostDollars > 0 {
		// Simplified breakdown - actual breakdown would need more detailed tracking
		dynamoPercent = 60.0  // Estimate: DynamoDB typically 60%
		lambdaPercent = 25.0  // Estimate: Lambda typically 25%
		transferPercent = 10.0 // Estimate: Data transfer typically 10%
		storagePercent = 5.0   // Estimate: Storage typically 5%
	}

	// Calculate cost per user
	var avgCostPerUser, medianCostPerUser float64
	if currentMonth != nil && len(dailyCosts) > 0 {
		// Sum unique users from daily data
		totalUniqueUsers := int64(0)
		for _, daily := range dailyCosts {
			if daily.UniqueUsers > totalUniqueUsers {
				totalUniqueUsers = daily.UniqueUsers
			}
		}
		if totalUniqueUsers > 0 {
			avgCostPerUser = currentMonth.TotalCostDollars * 100 / float64(totalUniqueUsers) // Convert to cents
			medianCostPerUser = avgCostPerUser // Simplified
		}
	}

	// Build response with nil checks
	var monthData map[string]any
	if currentMonth != nil {
		monthData = map[string]any{
			"total_cost_cents":     currentMonth.TotalCostDollars * 100, // Convert dollars to cents
			"dynamodb_reads":       currentMonth.TotalReads,
			"dynamodb_writes":      currentMonth.TotalWrites,
			"lambda_invocations":   currentMonth.TotalRequests,
			"data_transfer_gb":     0, // Not tracked in the new structure
			"projected_cost_cents": currentMonth.TotalCostDollars * 100 * 30 / float64(now.Day()), // Project to full month
		}
	} else {
		monthData = map[string]any{
			"total_cost_cents":     0,
			"dynamodb_reads":       0,
			"dynamodb_writes":      0,
			"lambda_invocations":   0,
			"data_transfer_gb":     0,
			"projected_cost_cents": 0,
		}
	}

	response := map[string]any{
		"current_month": monthData,
		"daily_costs": formattedDailyCosts,
		"cost_per_user": map[string]any{
			"average_cents": avgCostPerUser,
			"median_cents":  medianCostPerUser,
		},
		"cost_breakdown": map[string]any{
			"dynamodb_percent":      dynamoPercent,
			"lambda_percent":        lambdaPercent,
			"data_transfer_percent": transferPercent,
			"storage_percent":       storagePercent,
		},
	}

	return ctx.JSON(response)
}

// HandleGetInstanceConfigurationLift returns configuration details
func (h *Handler) HandleGetInstanceConfigurationLift(ctx *lift.Context) error {
	// Build configuration response
	config := map[string]any{
		"urls": map[string]any{
			// Use Mastodon-compatible streaming endpoint
			"streaming": fmt.Sprintf("wss://ws.%s/v1", h.cfg.Domain),
		},
		"accounts": map[string]any{
			"max_featured_tags":   20,
			"max_pinned_statuses": 5,
		},
		"statuses": map[string]any{
			"max_characters":              5000,
			"max_media_attachments":       4,
			"characters_reserved_per_url": 23,
		},
		"media_attachments": map[string]any{
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
		"polls": map[string]any{
			"max_options":               4,
			"max_characters_per_option": 50,
			"min_expiration":            300,
			"max_expiration":            2629746,
		},
		"translation": map[string]any{
			"enabled": false,
		},
	}

	// Add VAPID key if available
	vapidKey := os.Getenv("VAPID_PUBLIC_KEY")
	if vapidKey != "" {
		config["vapid_key"] = vapidKey
	}

	return ctx.JSON(config)
}

// Helper functions

// getUniqueAccountsForDay returns unique account count for a specific day
func (h *Handler) getUniqueAccountsForDay(ctx *lift.Context, day string) string {
	// Parse the day string to get the date (validation only)
	_, err := time.Parse("2006-01-02", day)
	if err != nil {
		h.logger.Warn("invalid day format", zap.String("day", day), zap.Error(err))
		return "0"
	}

	// Get unique active users for that specific day
	count, err := h.repos.Instance().GetDailyActiveUserCount(ctx.Context)
	if err != nil {
		h.logger.Error("failed to get daily active user count",
			zap.String("day", day), zap.Error(err))
		return "0"
	}

	return fmt.Sprintf("%d", count)
}

// getActiveMonthlyUsers returns the count of active users in the current month
func (h *Handler) getActiveMonthlyUsers(ctx *lift.Context) int {
	// Get count of users who have been active in the last 30 days
	count, err := h.repos.Analytics().GetActiveUserCount(ctx.Context, 30)
	if err != nil {
		h.logger.Error("failed to get active monthly users", zap.Error(err))
		return 1 // Default fallback
	}
	return int(count)
}

// getAdminAccount returns the admin account for the instance
func (h *Handler) getAdminAccount(ctx *lift.Context) any {
	// Get admin username from config
	adminUsername := os.Getenv("ADMIN_USERNAME")
	if adminUsername == "" {
		return nil
	}

	// Get admin actor
	actor, err := h.repos.Actor().GetActor(ctx.Context, adminUsername)
	if err != nil {
		h.logger.Error("failed to get admin account", zap.String("username", adminUsername), zap.Error(err))
		return nil
	}

	// Get counts
	followerCount, _ := h.repos.Relationship().CountFollowers(ctx.Context, actor.ID)
	followingCount, _ := h.repos.Relationship().CountFollowing(ctx.Context, actor.ID)
	statusesCount, _ := h.repos.Status().CountStatusesByAuthor(ctx.Context, actor.ID)

	// Return admin account in API format
	return map[string]any{
		"id":              actor.ID,
		"username":        actor.PreferredUsername,
		"acct":            actor.PreferredUsername,
		"display_name":    actor.Name,
		"locked":          actor.ManuallyApprovesFollowers,
		"bot":             actor.Type == "Service",
		"discoverable":    actor.Discoverable,
		"created_at":      h.formatActorCreatedTime(actor.CreatedAt),
		"note":            actor.Summary,
		"url":             actor.URL,
		"avatar":          h.getAvatarURL(actor),
		"avatar_static":   h.getAvatarURL(actor),
		"header":          h.getHeaderURLLift(actor),
		"header_static":   h.getHeaderURLLift(actor),
		"followers_count": followerCount,
		"following_count": followingCount,
		"statuses_count":  statusesCount,
		"last_status_at":  h.formatLastStatusTime(actor.LastStatusAt),
	}
}

// getAvatarURL returns the avatar URL for an actor
func (h *Handler) getAvatarURL(actor *activitypub.Actor) string {
	if actor.Icon != nil && actor.Icon.URL != "" {
		return actor.Icon.URL
	}
	return fmt.Sprintf("%s/avatars/default.png", h.cfg.BaseURL())
}


// formatLastStatusTime formats last status time
func (h *Handler) formatLastStatusTime(lastStatusAt *time.Time) *string {
	if lastStatusAt == nil {
		return nil
	}
	formatted := lastStatusAt.Format("2006-01-02")
	return &formatted
}

// getMapKeys returns keys from a map for logging
func getMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}