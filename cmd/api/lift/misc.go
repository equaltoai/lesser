package lift

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/services/notifications"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/transformations"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

const (
	// Search type constants
	searchTypeStatuses = "statuses"

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
	token := h.getBearerTokenLift(ctx)
	if err := common.ValidateRequiredParam("token", token); err != nil {
		return
	}

	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	if claims, err := oauthSvc.ValidateAccessToken(token); err == nil {
		h.logger.Debug("Authenticated search", zap.String("username", claims.Username))
	}
}

// parseSearchParams extracts and validates search parameters
func (h *Handler) parseSearchParams(ctx *lift.Context) (*SearchParams, error) {
	query := ctx.Query("q")
	if err := common.ValidateSearchQuery(query); err != nil {
		if err := common.RespondValidationError(ctx, err); err != nil {
			h.logger.Error("failed to send error response", zap.Error(err))
		}
		return nil, err
	}

	// Parse limit
	limitStr := ctx.Query("limit")
	limit, err := common.ParseTimelineLimit(limitStr)
	if err != nil {
		limit = 20 // Use default on error
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

// convertActorToAccount converts an ActivityPub actor to API account using transformation framework
func (h *Handler) convertActorToAccount(actor *activitypub.Actor) models.Account {
	// Use centralized transformation framework - ELIMINATES 25+ LINES OF DUPLICATE CODE
	account := transformations.ActorToAccountBase(actor, h.cfg.BaseURL())

	// Override ID to use PreferredUsername instead of numeric ID for this specific use case
	account.ID = actor.PreferredUsername

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
	case *activitypub.Note:
		result := &storage.StatusSearchResult{
			StatusID: v.ID,
			URL:      v.ID,
			Content:  v.Content,
			AuthorID: v.AttributedTo,
		}
		if v.Published != nil {
			result.Published = *v.Published
		}
		return result
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
	// Convert search result to object map for transformation framework
	statusMap := map[string]interface{}{
		"id":        sr.StatusID,
		"content":   sr.Content,
		"url":       sr.URL,
		"published": sr.Published.Format(time.RFC3339),
	}

	// Use centralized transformation framework - ELIMINATES 8+ LINES OF DUPLICATE CODE
	transformer := transformations.NewStatusResponseTransformer(h.cfg.BaseURL(), transformations.ObjectToStatusWithContext)
	transformCtx := context.WithValue(ctx.Context, baseURLContextKey, h.cfg.BaseURL())

	status, err := transformer.Transform(transformCtx, statusMap)
	if err != nil || status.ID == "" {
		// Fallback to minimal status if transformation fails
		status = models.Status{
			ID:        sr.StatusID,
			Content:   sr.Content,
			URL:       sr.URL,
			CreatedAt: sr.Published.Format(time.RFC3339),
		}
	}

	// Add account info if we can get the actor
	if sr.AuthorID != "" {
		if statusActor := h.getActorFromAuthorID(ctx, sr.AuthorID); statusActor != nil {
			account := transformations.ActorToAccountBase(statusActor, h.cfg.BaseURL())
			status.Account = account
		}
	}

	return status
}

// getActorFromAuthorID extracts actor from author ID
func (h *Handler) getActorFromAuthorID(ctx *lift.Context, authorID string) *activitypub.Actor {
	parts := strings.Split(authorID, "/")
	if err := common.ValidateSliceNotEmpty("author_id_parts", parts); err != nil {
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
	apiHistory := make([]models.TagHistory, 0, min(len(history), 7))

	// Create history entries (most recent first)
	for i := 0; i < len(history) && i < 7; i++ {
		day := time.Now().AddDate(0, 0, -i).Format(common.DateFormat)
		apiHistory = append(apiHistory, models.TagHistory{
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
		History: []models.TagHistory{{
			Day:      time.Now().Format(common.DateFormat),
			Uses:     "0",
			Accounts: "0",
		}},
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
	// Authenticate user with read:notifications scope
	username, err := h.authenticateUser(ctx, []string{"read:notifications", auth.ScopeRead})
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondMissingAuth(ctx)
	}

	// Build notification filter from query parameters
	notificationFilter := h.buildNotificationFilter(ctx)

	// Convert to includeRead flag for the service method
	includeRead := len(notificationFilter.ExcludeTypes) == 0

	// Use the Notifications service to get notifications
	notificationService := h.registry.Notifications()
	if notificationService == nil {
		return common.RespondServiceUnavailable(ctx, "notification")
	}

	listResult, err := notificationService.ListNotifications(ctx.Context, &notifications.ListNotificationsQuery{
		UserID:       username,
		Types:        notificationFilter.Types,
		ExcludeTypes: notificationFilter.ExcludeTypes,
		IncludeRead:  includeRead,
		Pagination: interfaces.PaginationOptions{
			Limit:  notificationFilter.Limit,
			Cursor: notificationFilter.MaxID,
		},
	})
	if err != nil {
		h.logger.Error("failed to get notifications",
			zap.String("username", username),
			zap.Error(err))
		return common.RespondFailedToGet(ctx, "notifications")
	}

	notificationsList := listResult.Notifications
	cursor := ""
	if listResult.Pagination != nil && listResult.Pagination.NextCursor != "" {
		cursor = listResult.Pagination.NextCursor
	}

	// Convert notifications to storage format for API converter
	storageNotifications := make([]*storage.Notification, 0, len(notificationsList))
	for _, notification := range notificationsList {
		storageNotif := &storage.Notification{
			ID:        notification.ID,
			Type:      notification.Type,
			AccountID: notification.ActorID,
			TargetID:  notification.TargetID,
			Read:      notification.IsRead,
			CreatedAt: notification.CreatedAt,
			Username:  notification.UserID,
		}
		storageNotifications = append(storageNotifications, storageNotif)
	}

	// Convert notifications to API format
	apiNotifications := h.convertNotificationsToAPI(ctx, storageNotifications)

	// Set pagination header if needed
	if cursor != "" {
		h.setNotificationPaginationHeader(ctx, cursor, notificationFilter.Limit)
	}

	return ctx.JSON(apiNotifications)
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
		if limit, err := common.ParseAndValidateAPILimit(limitStr, 40); err == nil {
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

	account := transformations.ActorToAccountBase(actor, h.cfg.BaseURL())
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
	if err := common.ValidateMastodonStatusID(notif.StatusID); err != nil {
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

	// Convert obj to map for transformation
	if objMap, ok := obj.(map[string]interface{}); ok {
		status := transformations.ObjectToStatusBase(objMap, statusActor, h.cfg.BaseURL())
		apiNotif.Status = &status
	} else {
		// Fallback for non-map objects
		status := transformations.ObjectToStatusAny(obj, statusActor, h.cfg.BaseURL())
		apiNotif.Status = &status
	}
}

// extractStatusAuthor extracts the author actor from a status object
func (h *Handler) extractStatusAuthor(ctx *lift.Context, obj any) *activitypub.Actor {
	note, ok := obj.(*activitypub.Note)
	if !ok {
		return nil
	}
	if err := common.ValidateRequiredParam("attributed_to", note.AttributedTo); err != nil {
		return nil
	}

	parts := strings.Split(note.AttributedTo, "/")
	if err := common.ValidateSliceNotEmpty("attributed_to_parts", parts); err != nil {
		return nil
	}

	username := parts[len(parts)-1]
	statusActor, _ := h.repos.Actor().GetActor(ctx.Context, username)
	return statusActor
}

// setNotificationPaginationHeader sets the pagination Link header for notifications
func (h *Handler) setNotificationPaginationHeader(ctx *lift.Context, cursor string, limit int) {
	host := ctx.Header("host")
	if err := common.ValidateRequiredParam("host", host); err != nil {
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
	state, stateErr := h.repos.Instance().GetInstanceState(ctx.Context)
	locked := stateErr != nil || state.Locked

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
		// Check if we're in production mode
		env := h.cfg.Stage
		if env == "production" || env == "prod" {
			// In production, VAPID keys are required for push notifications
			h.logger.Error("VAPID keys are required in production but not found", zap.Error(err))
			return common.RespondInternalServerError(ctx, "VAPID keys not configured - push notifications unavailable")
		}

		// In non-production, auto-generate VAPID keys if they don't exist
		h.logger.Info("VAPID keys not found in non-production environment, generating new keys")
		vapidKeys, err = h.generateAndStoreVAPIDKeys(ctx.Context)
		if err != nil {
			h.logger.Error("failed to generate VAPID keys", zap.Error(err))
			// Return empty vapid key to disable push notifications
			vapidPublicKey = ""
		} else {
			vapidPublicKey = vapidKeys.PublicKey
		}
	} else {
		vapidPublicKey = vapidKeys.PublicKey
	}

	// Convert rules for API response
	apiRules := make([]models.Rule, len(rules))
	for i, rule := range rules {
		apiRules[i] = models.Rule{ID: rule.ID, Text: rule.Text}
	}

	resp := models.InstanceV2Response{
		Domain:      h.cfg.Domain,
		Title:       instanceConfig.Title,
		Version:     instanceConfig.Version,
		SourceURL:   "https://github.com/equaltoai/lesser",
		Description: instanceConfig.Description,
		Usage: map[string]any{
			"users": map[string]any{
				"active_month": h.getActiveMonthlyUsers(ctx),
			},
		},
		Thumbnail: map[string]any{
			"url": h.cfg.BaseURL() + "/assets/thumbnail.png",
		},
		Icon:      []any{},
		Languages: instanceConfig.Languages,
		Configuration: map[string]any{
			"urls": map[string]any{
				"streaming":        h.cfg.BaseURL(),
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
		Registrations: map[string]any{
			"enabled":           instanceConfig.RegistrationsOpen && !locked,
			"approval_required": instanceConfig.ApprovalRequired,
			"message":           nil,
			"min_age":           nil,
			"reason_required":   false,
		},
		APIVersions: map[string]any{
			"mastodon": 1,
		},
		Contact: map[string]any{
			"email":   instanceConfig.Email,
			"account": h.getAdminAccount(ctx),
		},
		Rules: apiRules,
	}

	// Log the response to debug
	h.logger.Info("HandleGetInstanceV2Lift response",
		zap.String("domain", resp.Domain),
		zap.String("title", resp.Title),
		zap.String("version", resp.Version),
	)

	return ctx.JSON(resp)
}

// HandleGetNotificationLift handles GET /api/v1/notifications/:id
func (h *Handler) HandleGetNotificationLift(ctx *lift.Context) error {
	notificationID := ctx.Param("id")
	if err := common.ValidateRequiredParam("notification_id", notificationID); err != nil {
		return common.RespondMissingParameter(ctx, "notification ID")
	}

	// Authenticate user with read:notifications scope
	username, err := h.authenticateUser(ctx, []string{"read:notifications", auth.ScopeRead})
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondMissingAuth(ctx)
	}

	// Get notification
	notification, err := h.repos.Notification().GetNotification(ctx.Context, notificationID)
	if err != nil {
		return common.RespondNotFound(ctx, "notification")
	}

	// Verify ownership
	if notification.UserID != username {
		return common.RespondNotFound(ctx, "notification")
	}

	// Get the account that triggered the notification
	actor, err := h.repos.Actor().GetActor(ctx.Context, notification.ActorID)
	if err != nil {
		h.logger.Error("failed to get actor for notification",
			zap.String("notification_id", notification.ID),
			zap.String("actor_id", notification.ActorID),
			zap.Error(err))
		return common.RespondFailedToGet(ctx, "notification details")
	}

	account := transformations.ActorToAccountBase(actor, h.cfg.BaseURL())
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
		statusModel, err := h.registry.Notes().GetNote(ctx.Context, notification.TargetID)
		if err == nil && statusModel != nil && statusModel.Note != nil {
			// Convert note to status format
			var statusActor *activitypub.Actor
			if statusModel.AuthorUsername != "" {
				account, _ := h.registry.Accounts().GetAccount(ctx.Context, statusModel.AuthorUsername)
				if account != nil {
					statusActor = account.Actor
				}
			}
			// Use the embedded Note from the status model
			status := transformations.ObjectToStatusAny(statusModel.Note, statusActor, h.cfg.BaseURL())
			apiNotif.Status = &status
		}
	}

	return ctx.JSON(apiNotif)
}

// HandleClearNotificationsLift handles POST /api/v1/notifications/clear
func (h *Handler) HandleClearNotificationsLift(ctx *lift.Context) error {
	// Authenticate user with write:notifications scope
	username, err := h.authenticateUser(ctx, []string{"write:notifications", auth.ScopeWrite})
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondMissingAuth(ctx)
	}

	// Use the Notifications service to clear all notifications
	notificationService := h.registry.Notifications()
	if notificationService == nil {
		return common.RespondServiceUnavailable(ctx, "notification")
	}

	clearResult, err := notificationService.ClearNotifications(ctx.Context, &notifications.ClearCommand{
		UserID:   username,
		ClearAll: true,
	})
	if err != nil {
		h.logger.Error("failed to clear notifications",
			zap.String("username", username),
			zap.Error(err))
		return common.RespondFailedToUpdate(ctx, "notifications")
	}

	h.logger.Info("cleared notifications", zap.String("username", username), zap.Int64("deleted", clearResult.ClearedCount))

	ctx.Status(204)
	return nil
}

// HandleDismissNotificationLift handles POST /api/v1/notifications/:id/dismiss
func (h *Handler) HandleDismissNotificationLift(ctx *lift.Context) error {
	notificationID := ctx.Param("id")
	if err := common.ValidateRequiredParam("notification_id", notificationID); err != nil {
		return common.RespondMissingParameter(ctx, "notification ID")
	}

	// Authenticate user with write:notifications scope
	username, err := h.authenticateUser(ctx, []string{"write:notifications", auth.ScopeWrite})
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondMissingAuth(ctx)
	}

	// Use the Notifications service to mark as read (dismiss)
	notificationService := h.registry.Notifications()
	if notificationService == nil {
		return common.RespondServiceUnavailable(ctx, "notification")
	}

	// Mark notification as read (which is effectively dismissing it in Mastodon API)
	_, err = notificationService.MarkAsRead(ctx.Context, &notifications.MarkAsReadCommand{
		NotificationID: notificationID,
		UserID:         username,
	})
	if err != nil {
		h.logger.Error("failed to dismiss notification",
			zap.String("notification_id", notificationID),
			zap.Error(err))
		// Check if the error is due to not found
		if strings.Contains(err.Error(), "not found") {
			return common.RespondNotFound(ctx, "notification")
		}
		return common.RespondInternalServerError(ctx, "failed to dismiss notification")
	}

	ctx.Status(204)
	return nil
}

// HandleGetInstanceCostsLift returns cost analytics for the instance
func (h *Handler) HandleGetInstanceCostsLift(ctx *lift.Context) error {
	h.logger.Info("HandleGetInstanceCostsLift called")

	// Initialize cost storage if not already done
	costTableName := h.cfg.CostHistoryTableName
	if err := common.ValidateRequiredParam("COST_HISTORY_TABLE_NAME", costTableName); err != nil {
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
			"streaming": h.cfg.BaseURL(),
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
	vapidKey := h.cfg.VAPIDPublicKey
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
	adminUsername := h.cfg.AdminUsername
	if err := common.ValidateRequiredParam("ADMIN_USERNAME", adminUsername); err != nil {
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
		"created_at":      h.formatActorCreatedTimeLift(actor.CreatedAt),
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
// generateAndStoreVAPIDKeys generates new VAPID keys and stores them in the database
func (h *Handler) generateAndStoreVAPIDKeys(ctx context.Context) (*storage.VAPIDKeys, error) {
	h.logger.Info("generating new VAPID keys for push notifications")

	// Generate ECDSA P-256 key pair
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, errors.Join(failedToGenerateVAPIDPrivateKey(), err)
	}

	// Convert to ECDH and get public key bytes
	ecdhKey, err := privateKey.ECDH()
	if err != nil {
		return nil, errors.Join(failedToConvertToECDHKey(), err)
	}
	publicKeyBytes := ecdhKey.PublicKey().Bytes()
	publicKeyBase64 := base64.RawURLEncoding.EncodeToString(publicKeyBytes)

	// Encode private key (32 bytes)
	privateKeyBytes := privateKey.D.Bytes()
	// Pad to 32 bytes if necessary
	if len(privateKeyBytes) < 32 {
		padding := make([]byte, 32-len(privateKeyBytes))
		privateKeyBytes = append(padding, privateKeyBytes...)
	}
	privateKeyBase64 := base64.RawURLEncoding.EncodeToString(privateKeyBytes)

	// Determine the subject (domain)
	domain := h.cfg.Domain
	if err := common.ValidateRequiredParam("domain", domain); err != nil {
		domain = "localhost" // fallback for development
	}

	// Create VAPID keys object
	vapidKeys := &storage.VAPIDKeys{
		PublicKey:  publicKeyBase64,
		PrivateKey: privateKeyBase64,
		Subject:    fmt.Sprintf("mailto:admin@%s", domain),
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	// Store the keys
	err = h.repos.PushSubscription().SetVAPIDKeys(ctx, vapidKeys)
	if err != nil {
		return nil, errors.Join(failedToStoreVAPIDKeys(), err)
	}

	h.logger.Info("successfully generated and stored new VAPID keys",
		zap.String("public_key", publicKeyBase64),
		zap.String("subject", vapidKeys.Subject))

	return vapidKeys, nil
}

// HandleGetGroupedNotificationsLift handles GET /api/v2/notifications/grouped
// Returns notifications grouped by type and target with enhanced metadata
func (h *Handler) HandleGetGroupedNotificationsLift(ctx *lift.Context) error {
	// Authenticate user with read:notifications scope
	username, err := h.authenticateUser(ctx, []string{"read:notifications", auth.ScopeRead})
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondMissingAuth(ctx)
	}

	// Parse grouping options from query parameters
	groupingOptions := h.parseGroupingOptions(ctx)

	// Build notification filter from query parameters
	notificationFilter := h.buildNotificationFilter(ctx)

	// Get notifications using the existing service
	notificationService := h.registry.Notifications()
	if notificationService == nil {
		return common.RespondServiceUnavailable(ctx, "notification")
	}

	listResult, err := notificationService.ListNotifications(ctx.Context, &notifications.ListNotificationsQuery{
		UserID:       username,
		Types:        notificationFilter.Types,
		ExcludeTypes: notificationFilter.ExcludeTypes,
		IncludeRead:  true, // Include all for grouping
		Pagination: interfaces.PaginationOptions{
			Limit:  notificationFilter.Limit * 2, // Get more for better grouping
			Cursor: notificationFilter.MaxID,
		},
	})
	if err != nil {
		h.logger.Error("failed to get notifications for grouping",
			zap.String("username", username),
			zap.Error(err))
		return common.RespondFailedToGet(ctx, "notifications")
	}

	// Group notifications using the grouping service
	groupingService := notifications.NewGroupedNotificationsService(h.logger)
	groupedNotifications, err := groupingService.GroupNotifications(
		ctx.Context,
		listResult.Notifications, // Use storage notifications directly
		groupingOptions,
	)
	if err != nil {
		h.logger.Error("failed to group notifications", zap.Error(err))
		return common.RespondInternalServerError(ctx, "failed to group notifications")
	}

	// Convert to API format with enhanced metadata
	apiResponse := h.convertGroupedNotificationsToAPI(ctx, groupedNotifications)

	// Set pagination header if available
	if listResult.Pagination != nil && listResult.Pagination.NextCursor != "" {
		h.setNotificationPaginationHeader(ctx, listResult.Pagination.NextCursor, notificationFilter.Limit)
	}

	return ctx.JSON(apiResponse)
}

// parseGroupingOptions parses grouping options from query parameters
func (h *Handler) parseGroupingOptions(ctx *lift.Context) *notifications.GroupingStrategy {
	strategy := notifications.DefaultGroupingStrategy()

	// Parse time window (in hours)
	if timeWindowStr := ctx.Query("time_window"); timeWindowStr != "" {
		if hours, err := common.ParseAndValidateIntWithBounds("time_window", timeWindowStr, 0, 168, 0); err == nil { // Max 1 week
			strategy.TimeWindow = time.Duration(hours) * time.Hour
		}
	}

	// Parse max group size
	if maxSizeStr := ctx.Query("max_group_size"); maxSizeStr != "" {
		if maxSize, err := common.ParseAndValidateIntWithBounds("max_group_size", maxSizeStr, 0, 100, 0); err == nil {
			strategy.MaxGroupSize = maxSize
		}
	}

	// Parse min group size
	if minSizeStr := ctx.Query("min_group_size"); minSizeStr != "" {
		if minSize, err := common.ParseAndValidateIntWithBounds("min_group_size", minSizeStr, 0, 10, 0); err == nil && minSize >= 1 {
			strategy.MinGroupSize = minSize
		}
	}

	// Parse sample size
	if sampleSizeStr := ctx.Query("sample_size"); sampleSizeStr != "" {
		if sampleSize, err := common.ParseAndValidateIntWithBounds("sample_size", sampleSizeStr, 0, 10, 0); err == nil {
			strategy.SampleSize = sampleSize
		}
	}

	// Parse grouping flags
	if groupByType := ctx.Query("group_by_type"); groupByType != "" {
		if result, _ := common.ParseAndValidateBoolean(groupByType); !result {
			strategy.GroupByType = false
		}
	}

	if groupByTarget := ctx.Query("group_by_target"); groupByTarget != "" {
		if result, _ := common.ParseAndValidateBoolean(groupByTarget); !result {
			strategy.GroupByTarget = false
		}
	}

	return strategy
}

// convertGroupedNotificationsToAPI converts grouped notifications to API format
func (h *Handler) convertGroupedNotificationsToAPI(
	ctx *lift.Context,
	groupedNotifications []*notifications.GroupedNotification,
) []models.GroupedNotificationGroup {
	apiResponse := make([]models.GroupedNotificationGroup, 0, len(groupedNotifications))

	for _, group := range groupedNotifications {
		groupResponse := models.GroupedNotificationGroup{
			ID:                group.ID,
			Type:              group.Type,
			GroupKey:          group.GroupKey,
			Count:             group.Count,
			LatestCreatedAt:   group.LatestCreatedAt.Format(time.RFC3339),
			EarliestCreatedAt: group.EarliestCreatedAt.Format(time.RFC3339),
			Read:              group.IsRead,
			SampleAccounts:    h.convertNotificationAccountsToAPI(group.SampleAccounts),
			Summary:           h.generateGroupSummary(group),
		}

		// Add target status if available
		if group.TargetStatus != nil {
			groupResponse.Status = &models.GroupedNotificationStatus{
				ID:         group.TargetStatus.ID,
				Content:    group.TargetStatus.Content,
				CreatedAt:  group.TargetStatus.CreatedAt.Format(time.RFC3339),
				URL:        group.TargetStatus.URL,
				Visibility: group.TargetStatus.Visibility,
			}
		}

		// Add most recent notification details
		if group.MostRecentNotif != nil {
			groupResponse.MostRecent = &models.GroupedNotificationMostRecent{
				ID:        group.MostRecentNotif.ID,
				CreatedAt: group.MostRecentNotif.CreatedAt.Format(time.RFC3339),
				ActorID:   group.MostRecentNotif.ActorID,
			}
		}

		// Optionally include all notifications if requested
		if func() bool { result, _ := common.ParseAndValidateBoolean(ctx.Query("include_all")); return result }() && len(group.AllNotifications) > 0 {
			allNotifs := make([]models.GroupedNotificationEntry, 0, len(group.AllNotifications))
			for _, notif := range group.AllNotifications {
				allNotifs = append(allNotifs, models.GroupedNotificationEntry{
					ID:        notif.ID,
					CreatedAt: notif.CreatedAt.Format(time.RFC3339),
					ActorID:   notif.ActorID,
					TargetID:  notif.TargetID,
					Read:      notif.IsRead,
				})
			}
			groupResponse.AllNotifications = allNotifs
		}

		apiResponse = append(apiResponse, groupResponse)
	}

	return apiResponse
}

// convertNotificationAccountsToAPI converts notification accounts to API format
func (h *Handler) convertNotificationAccountsToAPI(
	accounts []notifications.NotificationAccount,
) []models.GroupedNotificationAccount {
	apiAccounts := make([]models.GroupedNotificationAccount, 0, len(accounts))

	for _, account := range accounts {
		apiAccount := models.GroupedNotificationAccount{
			ID:          account.ID,
			Username:    account.Username,
			DisplayName: account.DisplayName,
			Avatar:      account.Avatar,
			Bot:         account.IsBot,
			CreatedAt:   account.CreatedAt.Format(time.RFC3339),
		}
		apiAccounts = append(apiAccounts, apiAccount)
	}

	return apiAccounts
}

// generateGroupSummary generates a summary for a notification group
func (h *Handler) generateGroupSummary(group *notifications.GroupedNotification) string {
	groupingService := notifications.NewGroupedNotificationsService(h.logger)
	return groupingService.GenerateGroupSummary(group)
}

// HandleMarkGroupAsReadLift handles POST /api/v2/notifications/groups/:group_id/read
// Marks all notifications in a group as read
func (h *Handler) HandleMarkGroupAsReadLift(ctx *lift.Context) error {
	groupID := ctx.Param("group_id")
	if err := common.ValidateRequiredParam("group_id", groupID); err != nil {
		return common.RespondMissingParameter(ctx, "group ID")
	}

	// Authenticate user with write:notifications scope
	username, err := h.authenticateUser(ctx, []string{"write:notifications"})
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondMissingAuth(ctx)
	}

	// Get notification service
	notificationService := h.registry.Notifications()
	if notificationService == nil {
		return common.RespondServiceUnavailable(ctx, "notification")
	}

	// Mark notifications as read based on group ID
	// For now, this is a simplified implementation
	// In a full implementation, you would:
	// 1. Parse the group_id to extract grouping criteria
	// 2. Find all notifications matching that criteria
	// 3. Mark them all as read

	_, err = notificationService.MarkAsRead(ctx.Context, &notifications.MarkAsReadCommand{
		NotificationID: groupID,
		UserID:         username,
	})
	if err != nil {
		h.logger.Error("failed to mark group as read",
			zap.String("group_id", groupID),
			zap.String("username", username),
			zap.Error(err))
		return common.RespondInternalServerError(ctx, "failed to mark group as read")
	}

	return ctx.Status(200).JSON(models.MessageResponse{Message: "group marked as read"})
}
