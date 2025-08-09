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
	"github.com/equaltoai/lesser/pkg/common"
)

const (
	// Search type constants
	searchTypeStatuses = "statuses"
	
	// Boolean string constants for parameter parsing
	boolTrue = "true"
	
	// Common status constants
	statusCompleted = "completed"
	
	// Moderation category constants
	moderationCategoryOther   = "other"
	moderationCategoryGeneral = "general"
	
	// API path components
	pathComponentStatuses = "statuses"
)

// SearchParams holds search request parameters
type SearchParams struct {
	Query     string
	Type      string
	AccountID string
	Limit     int
}

// HandleSearchLift performs a search across accounts, statuses, and hashtags
func (h *Handler) HandleSearchLift(ctx *lift.Context) error {
	// Optional authentication for search
	h.logSearchAuthentication(ctx)

	// Parse and validate parameters
	params, err := h.parseSearchParams(ctx)
	if err != nil {
		return err
	}

	// Initialize results structure
	result := models.SearchResult{
		Accounts: []models.Account{},
		Statuses: []models.Status{},
		Hashtags: []models.Tag{},
	}

	// Execute searches based on type
	h.executeAccountSearch(ctx, params, &result)
	h.executeStatusSearch(ctx, params, &result)
	h.executeHashtagSearch(ctx, params, &result)

	return ctx.JSON(result)
}

// logSearchAuthentication handles optional authentication logging
func (h *Handler) logSearchAuthentication(ctx *lift.Context) {
	if testUsername := h.getTestUsername(ctx); testUsername != "" {
		h.logger.Info("Using test mode authentication", zap.String("username", testUsername))
		return
	}

	token := h.getBearerTokenLift(ctx)
	if token == "" {
		return
	}

	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
	if claims, err := oauthSvc.ValidateAccessToken(token); err == nil {
		h.logger.Debug("Authenticated search", zap.String("username", claims.Username))
	}
}

// parseSearchParams extracts and validates search parameters
func (h *Handler) parseSearchParams(ctx *lift.Context) (*SearchParams, error) {
	query := ctx.Query("q")
	if query == "" {
		if err := ctx.Status(400).JSON(map[string]string{"error": "q parameter is required"}); err != nil {
			h.logger.Error("failed to send error response", zap.Error(err))
		}
		return nil, fmt.Errorf("missing query parameter")
	}

	// Parse limit
	limit := 20
	if limitStr := ctx.Query("limit"); limitStr != "" {
		if l, err := fmt.Sscanf(limitStr, "%d", &limit); err == nil && l == 1 {
			if limit > 40 {
				limit = 40
			}
		}
	}

	return &SearchParams{
		Query:     query,
		Type:      ctx.Query("type"),
		AccountID: ctx.Query("account_id"),
		Limit:     limit,
	}, nil
}

// executeAccountSearch performs account search if requested
func (h *Handler) executeAccountSearch(ctx *lift.Context, params *SearchParams, result *models.SearchResult) {
	if params.Type != "" && params.Type != "accounts" {
		return
	}

	actors, err := h.repos.Search().SearchAccounts(ctx.Context, params.Query, params.Limit, false, 0)
	if err != nil {
		h.logger.Error("account search failed",
			zap.String("query", params.Query),
			zap.Error(err))
		return
	}

	// Convert actors to accounts
	for _, actor := range actors {
		account := h.convertActorToAccount(actor)
		result.Accounts = append(result.Accounts, account)
	}
}

// executeStatusSearch performs status search if requested
func (h *Handler) executeStatusSearch(ctx *lift.Context, params *SearchParams, result *models.SearchResult) {
	if params.Type != "" && params.Type != searchTypeStatuses {
		return
	}

	if strings.HasPrefix(params.Query, "http") {
		h.searchStatusByURL(ctx, params.Query, result)
	} else {
		h.searchStatusByContent(ctx, params, result)
	}
}

// executeHashtagSearch performs hashtag search if requested
func (h *Handler) executeHashtagSearch(ctx *lift.Context, params *SearchParams, result *models.SearchResult) {
	if params.Type != "" && params.Type != "hashtags" {
		return
	}

	hashtags, err := h.repos.Search().SearchHashtags(ctx.Context, params.Query, params.Limit)
	if err != nil {
		h.logger.Warn("hashtag search failed", zap.Error(err))
		return
	}

	// Convert hashtags to API format
	for _, hashtag := range hashtags {
		tag := h.convertHashtagToTag(ctx, *hashtag)
		result.Hashtags = append(result.Hashtags, tag)
	}

	// Add placeholder hashtag if no results and query starts with #
	h.addPlaceholderHashtag(params.Query, result)
}

// convertActorToAccount converts an ActivityPub actor to API account
func (h *Handler) convertActorToAccount(actor *activitypub.Actor) models.Account {
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

	return account
}

// searchStatusByURL searches for a status by direct URL
func (h *Handler) searchStatusByURL(ctx *lift.Context, url string, result *models.SearchResult) {
	obj, err := h.repos.Object().GetObject(ctx.Context, url)
	if err != nil || obj == nil {
		return
	}

	statusResult := h.convertObjectToStatusResult(obj)
	if statusResult == nil {
		return
	}

	status := h.convertStatusResultToAPI(ctx, statusResult)
	result.Statuses = append(result.Statuses, status)
}

// searchStatusByContent searches for statuses by content
func (h *Handler) searchStatusByContent(ctx *lift.Context, params *SearchParams, result *models.SearchResult) {
	searchOptions := storage.StatusSearchOptions{
		Limit:     params.Limit,
		AccountID: params.AccountID,
	}

	statusResults, err := h.repos.Search().SearchStatusesWithOptions(ctx.Context, params.Query, searchOptions)
	if err != nil {
		h.logger.Warn("status search failed", zap.Error(err))
		return
	}

	// Convert search results to API format
	for _, sr := range statusResults {
		status := h.convertStatusResultToAPI(ctx, sr)
		result.Statuses = append(result.Statuses, status)
	}
}

// convertObjectToStatusResult converts object to status search result
func (h *Handler) convertObjectToStatusResult(obj interface{}) *storage.StatusSearchResult {
	switch v := obj.(type) {
	case *storagemodels.Object:
		return &storage.StatusSearchResult{
			StatusID:  v.ID,
			URL:       v.URL,
			Content:   v.Content,
			AuthorID:  v.AttributedTo,
			Published: v.Published,
		}
	case map[string]any:
		result := &storage.StatusSearchResult{}
		if id, ok := v["id"].(string); ok {
			result.StatusID = id
		}
		if url, ok := v["url"].(string); ok {
			result.URL = url
		}
		if content, ok := v["content"].(string); ok {
			result.Content = content
		}
		if authorID, ok := v["attributedTo"].(string); ok {
			result.AuthorID = authorID
		}
		if published, ok := v["published"].(time.Time); ok {
			result.Published = published
		}
		return result
	default:
		return nil
	}
}

// convertStatusResultToAPI converts status result to API format
func (h *Handler) convertStatusResultToAPI(ctx *lift.Context, sr *storage.StatusSearchResult) models.Status {
	status := models.Status{
		ID:        sr.StatusID,
		Content:   sr.Content,
		URL:       sr.URL,
		CreatedAt: sr.Published.Format(time.RFC3339),
	}

	// Add account info if we can get the actor
	if sr.AuthorID != "" {
		if statusActor := h.getActorFromAuthorID(ctx, sr.AuthorID); statusActor != nil {
			account := h.converter.ActorToAccount(statusActor)
			status.Account = account
		}
	}

	return status
}

// getActorFromAuthorID extracts actor from author ID
func (h *Handler) getActorFromAuthorID(ctx *lift.Context, authorID string) *activitypub.Actor {
	parts := strings.Split(authorID, "/")
	if len(parts) == 0 {
		return nil
	}
	
	username := parts[len(parts)-1]
	actor, _ := h.repos.Actor().GetActor(ctx.Context, username)
	return actor
}

// convertHashtagToTag converts hashtag with history to API tag
func (h *Handler) convertHashtagToTag(ctx *lift.Context, hashtag storage.Hashtag) models.Tag {
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
		day := time.Now().AddDate(0, 0, -i).Format(common.DateFormat)
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

	return models.Tag{
		Name:    hashtag.Name,
		URL:     hashtag.URL,
		History: apiHistory,
	}
}

// addPlaceholderHashtag adds a placeholder hashtag if query starts with # and no results
func (h *Handler) addPlaceholderHashtag(query string, result *models.SearchResult) {
	if len(result.Hashtags) > 0 || !strings.HasPrefix(query, "#") {
		return
	}

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
				Day:      time.Now().Format(common.DateFormat),
				Uses:     "0",
				Accounts: "0",
			},
		},
	}
	result.Hashtags = append(result.Hashtags, tag)
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
	// Authenticate request
	claims, err := h.authenticateNotificationRequest(ctx)
	if err != nil {
		return err
	}

	// Check required scope
	if !h.hasNotificationScope(claims) {
		return ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
	}

	// Build notification filter from query parameters
	filter := h.buildNotificationFilter(ctx)

	// Get notifications from repository
	notifications, cursor, err := h.repos.Notification().GetNotificationsFiltered(ctx.Context, claims.Username, filter)
	if err != nil {
		h.logger.Error("failed to get notifications",
			zap.String("username", claims.Username),
			zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "failed to get notifications"})
	}

	// Convert notifications to API format
	apiNotifications := h.convertNotificationsToAPI(ctx, notifications)

	// Set pagination header if needed
	if cursor != "" {
		h.setNotificationPaginationHeader(ctx, cursor, filter.Limit)
	}

	return ctx.JSON(apiNotifications)
}

// authenticateNotificationRequest handles authentication for notification requests
func (h *Handler) authenticateNotificationRequest(ctx *lift.Context) (*auth.Claims, error) {
	// Try test mode first
	if testUsername := ctx.Header("X-Test-Username"); testUsername != "" {
		h.logger.Info("Using test mode authentication", zap.String("username", testUsername))
		return &auth.Claims{Username: testUsername, Scopes: []string{auth.ScopeRead}}, nil
	}

	// Extract and validate token
	token := h.getBearerTokenLift(ctx)
	if token == "" {
		return nil, ctx.Status(401).JSON(map[string]string{"error": "authorization required"})
	}

	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return nil, ctx.Status(401).JSON(map[string]string{"error": "invalid token"})
	}

	return claims, nil
}

// hasNotificationScope checks if claims have the required notification scope
func (h *Handler) hasNotificationScope(claims *auth.Claims) bool {
	return claims.HasScope("read:notifications") || claims.HasScope(auth.ScopeRead)
}

// buildNotificationFilter builds a notification filter from query parameters
func (h *Handler) buildNotificationFilter(ctx *lift.Context) *storage.NotificationFilter {
	filter := &storage.NotificationFilter{
		Limit: 20, // Default limit
	}

	// Parse limit
	h.parseNotificationLimit(ctx, filter)

	// Parse types filter
	h.parseNotificationTypes(ctx, filter)

	// Parse exclude_types filter
	h.parseNotificationExcludeTypes(ctx, filter)

	// Parse account_id filter
	if accountID := ctx.Query("account_id"); accountID != "" {
		filter.AccountID = accountID
	}

	// Parse pagination parameters
	filter.MaxID = ctx.Query("max_id")
	filter.MinID = ctx.Query("min_id")
	filter.SinceID = ctx.Query("since_id")

	return filter
}

// parseNotificationLimit parses and validates the limit parameter
func (h *Handler) parseNotificationLimit(ctx *lift.Context, filter *storage.NotificationFilter) {
	if limitStr := ctx.Query("limit"); limitStr != "" {
		var limit int
		if _, err := fmt.Sscanf(limitStr, "%d", &limit); err == nil && limit > 0 {
			if limit > 40 {
				limit = 40
			}
			filter.Limit = limit
		}
	}
}

// parseNotificationTypes parses the types filter parameter
func (h *Handler) parseNotificationTypes(ctx *lift.Context, filter *storage.NotificationFilter) {
	if types := ctx.Query("types[]"); types != "" {
		filter.Types = []string{types}
	} else if typesStr := ctx.Query("types"); typesStr != "" {
		filter.Types = strings.Split(typesStr, ",")
	}
}

// parseNotificationExcludeTypes parses the exclude_types filter parameter
func (h *Handler) parseNotificationExcludeTypes(ctx *lift.Context, filter *storage.NotificationFilter) {
	if excludeTypes := ctx.Query("exclude_types[]"); excludeTypes != "" {
		filter.ExcludeTypes = []string{excludeTypes}
	} else if excludeTypesStr := ctx.Query("exclude_types"); excludeTypesStr != "" {
		filter.ExcludeTypes = strings.Split(excludeTypesStr, ",")
	}
}

// convertNotificationsToAPI converts storage notifications to API format
func (h *Handler) convertNotificationsToAPI(ctx *lift.Context, notifications []*storage.Notification) []*models.Notification {
	apiNotifications := make([]*models.Notification, 0, len(notifications))
	
	for _, notif := range notifications {
		apiNotif := h.convertSingleNotification(ctx, notif)
		if apiNotif != nil {
			apiNotifications = append(apiNotifications, apiNotif)
		}
	}
	
	return apiNotifications
}

// convertSingleNotification converts a single notification to API format
func (h *Handler) convertSingleNotification(ctx *lift.Context, notif *storage.Notification) *models.Notification {
	// Get the account that triggered the notification
	actor, err := h.repos.Actor().GetActor(ctx.Context, notif.AccountID)
	if err != nil {
		h.logger.Warn("failed to get actor for notification",
			zap.String("notification_id", notif.ID),
			zap.String("account_id", notif.AccountID),
			zap.Error(err))
		return nil
	}

	account := h.converter.ActorToAccount(actor)
	apiNotif := &models.Notification{
		ID:        notif.ID,
		Type:      notif.Type,
		CreatedAt: notif.CreatedAt,
		Account:   account,
	}

	// Add status if applicable
	if h.shouldIncludeStatus(notif) {
		h.attachStatusToNotification(ctx, notif, apiNotif)
	}

	return apiNotif
}

// shouldIncludeStatus checks if a status should be included in the notification
func (h *Handler) shouldIncludeStatus(notif *storage.Notification) bool {
	if notif.StatusID == "" {
		return false
	}
	return notif.Type == models.NotificationTypeMention ||
		notif.Type == models.NotificationTypeFavourite ||
		notif.Type == models.NotificationTypeReblog
}

// attachStatusToNotification attaches status information to a notification
func (h *Handler) attachStatusToNotification(ctx *lift.Context, notif *storage.Notification, apiNotif *models.Notification) {
	// Get the status
	obj, err := h.repos.Object().GetObject(ctx.Context, notif.StatusID)
	if err != nil {
		h.logger.Warn("failed to get status for notification",
			zap.String("notification_id", notif.ID),
			zap.String("status_id", notif.StatusID),
			zap.Error(err))
		return
	}

	// Get status author
	statusActor := h.extractStatusAuthor(ctx, obj)

	status := h.converter.ObjectToStatus(obj, statusActor)
	apiNotif.Status = &status
}

// extractStatusAuthor extracts the author actor from a status object
func (h *Handler) extractStatusAuthor(ctx *lift.Context, obj any) *activitypub.Actor {
	note, ok := obj.(*activitypub.Note)
	if !ok || note.AttributedTo == "" {
		return nil
	}

	parts := strings.Split(note.AttributedTo, "/")
	if len(parts) == 0 {
		return nil
	}

	username := parts[len(parts)-1]
	statusActor, _ := h.repos.Actor().GetActor(ctx.Context, username)
	return statusActor
}

// setNotificationPaginationHeader sets the pagination Link header for notifications
func (h *Handler) setNotificationPaginationHeader(ctx *lift.Context, cursor string, limit int) {
	host := ctx.Header("host")
	if host == "" {
		host = ctx.Header("Host")
	}
	
	baseURL := fmt.Sprintf("https://%s%s", host, "/api/v1/notifications")
	nextURL := fmt.Sprintf("%s?max_id=%s", baseURL, cursor)
	if limit > 0 {
		nextURL += fmt.Sprintf("&limit=%d", limit)
	}
	ctx.Set("Link", fmt.Sprintf(`<%s>; rel="next"`, nextURL))
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
			searchTypeStatuses: map[string]any{
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
		dynamoPercent = 60.0   // Estimate: DynamoDB typically 60%
		lambdaPercent = 25.0   // Estimate: Lambda typically 25%
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
			medianCostPerUser = avgCostPerUser                                               // Simplified
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
			"data_transfer_gb":     0,                                                             // Not tracked in the new structure
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
		"daily_costs":   formattedDailyCosts,
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
	_, err := time.Parse(common.DateFormat, day)
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
	return count
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
		"bot":             actor.Type == actorTypeService,
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
	formatted := lastStatusAt.Format(common.DateFormat)
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
