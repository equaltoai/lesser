package handlers

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/services/notifications"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/transformations"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

const (
	// Search type constants
	searchTypeStatuses = "statuses"

	// Common status constants
	statusCompleted              = "completed"
	notificationTargetTypeStatus = "status"

	// Moderation category constants
	moderationCategoryOther   = "other"
	moderationCategoryGeneral = "general"

	// API path components
	pathComponentStatuses = "statuses"

	notificationPostSnapshotKey = "postSnapshot"
)

// SearchParams holds search request parameters
type SearchParams struct {
	Query     string
	Type      string
	AccountID string
	Limit     int
	Resolve   bool
}

// HandleSearchLift performs a search across accounts, statuses, and hashtags
func (h *Handler) HandleSearchLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Optional authentication for search
	viewerUsername := h.searchViewerUsername(ctx)

	// Parse and validate parameters
	params, resp, err := h.parseSearchParams(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	// Initialize results structure
	result := models.SearchResult{
		Accounts: []models.Account{},
		Statuses: []models.Status{},
		Hashtags: []models.Tag{},
	}

	// Execute searches based on type
	h.executeAccountSearch(ctx, params, &result)
	h.executeStatusSearch(ctx, params, viewerUsername, &result)
	h.executeHashtagSearch(ctx, params, &result)

	return okJSON(result)
}

// searchViewerUsername returns the optional authenticated viewer for search requests.
func (h *Handler) searchViewerUsername(ctx *apptheory.Context) string {
	if username := h.getOptionalAuthenticatedUser(ctx); username != "" {
		h.logger.Debug("Authenticated search", zap.String("username", username))
		return username
	}

	return ""
}

// parseSearchParams extracts and validates search parameters
func (h *Handler) parseSearchParams(ctx *apptheory.Context) (*SearchParams, *apptheory.Response, error) {
	query := queryValue(ctx, "q")
	if err := common.ValidateSearchQuery(query); err != nil {
		resp, respErr := common.RespondValidationError(ctx, err)
		return nil, resp, respErr
	}

	// Parse limit
	limitStr := queryValue(ctx, "limit")
	limit, err := common.ParseTimelineLimit(limitStr)
	if err != nil {
		limit = 20 // Use default on error
	}

	return &SearchParams{
		Query:     query,
		Type:      queryValue(ctx, "type"),
		AccountID: queryValue(ctx, "account_id"),
		Limit:     limit,
		Resolve:   h.parseBoolParam(ctx, "resolve"),
	}, nil, nil
}

// executeAccountSearch performs account search if requested
func (h *Handler) executeAccountSearch(ctx *apptheory.Context, params *SearchParams, result *models.SearchResult) {
	if params.Type != "" && params.Type != "accounts" {
		return
	}

	actors, err := h.repos.Search().SearchAccounts(ctx.Context(), params.Query, params.Limit, false, 0)
	if err != nil {
		h.logger.Error("account search failed",
			zap.String("query", params.Query),
			zap.Error(err))
		return
	}

	// Convert actors to accounts
	for _, actor := range actors {
		account := h.convertActorToAccount(ctx.Context(), actor)
		result.Accounts = append(result.Accounts, account)
	}
}

// executeStatusSearch performs status search if requested
func (h *Handler) executeStatusSearch(ctx *apptheory.Context, params *SearchParams, viewerUsername string, result *models.SearchResult) {
	if params.Type != "" && params.Type != searchTypeStatuses {
		return
	}

	if strings.HasPrefix(params.Query, schemeHTTP) {
		h.searchStatusByURL(ctx, params.Query, viewerUsername, result)
	} else {
		h.searchStatusByContent(ctx, params, viewerUsername, result)
	}
}

// executeHashtagSearch performs hashtag search if requested
func (h *Handler) executeHashtagSearch(ctx *apptheory.Context, params *SearchParams, result *models.SearchResult) {
	if params.Type != "" && params.Type != "hashtags" {
		return
	}

	hashtags, err := h.repos.Search().SearchHashtags(ctx.Context(), params.Query, params.Limit)
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

// convertActorToAccount converts an ActivityPub actor to a public Mastodon account payload.
func (h *Handler) convertActorToAccount(ctx context.Context, actor *activitypub.Actor) models.Account {
	return h.publicAccountFromActor(ctx, actor)
}

// searchStatusByURL searches for a status by direct URL
func (h *Handler) searchStatusByURL(ctx *apptheory.Context, statusURL string, viewerUsername string, result *models.SearchResult) {
	if fullStatus, err := h.resolveStatusBySearchURL(ctx.Context(), statusURL); err == nil && fullStatus != nil {
		if !h.statusVisibleInSearch(fullStatus, viewerUsername) {
			return
		}
		apiStatus, convErr := h.convertStorageStatusToAPI(fullStatus, viewerUsername)
		if convErr == nil && apiStatus != nil {
			result.Statuses = append(result.Statuses, *apiStatus)
		}
	}
}

// searchStatusByContent searches for statuses by content
func (h *Handler) searchStatusByContent(ctx *apptheory.Context, params *SearchParams, viewerUsername string, result *models.SearchResult) {
	searchOptions := storage.StatusSearchOptions{
		Limit:     params.Limit,
		AccountID: params.AccountID,
	}

	statusResults, err := h.repos.Search().SearchStatusesWithPrivacy(ctx.Context(), params.Query, searchOptions, h.searchViewerActorID(viewerUsername))
	if err != nil {
		h.logger.Warn("status search failed", zap.Error(err))
		return
	}

	seen := make(map[string]struct{}, len(statusResults))

	// Convert search results to API format
	for _, sr := range statusResults {
		if len(result.Statuses) >= params.Limit {
			break
		}
		status := h.convertStatusResultToAPI(ctx, sr, viewerUsername)
		if strings.TrimSpace(status.ID) == "" {
			continue
		}
		if _, exists := seen[status.ID]; exists {
			continue
		}
		result.Statuses = append(result.Statuses, status)
		seen[status.ID] = struct{}{}
	}

	h.addAuthorMatchedStatuses(ctx, params, viewerUsername, seen, result)
}

// convertStatusResultToAPI converts status result to API format
func (h *Handler) convertStatusResultToAPI(ctx *apptheory.Context, sr *storage.StatusSearchResult, viewerUsername string) models.Status {
	if sr == nil {
		return models.Status{}
	}
	if status, err := h.resolveStatusFromSearchResult(ctx.Context(), sr); err == nil && status != nil {
		if !h.statusVisibleInSearch(status, viewerUsername) {
			return models.Status{}
		}
		apiStatus, convErr := h.convertStorageStatusToAPI(status, viewerUsername)
		if convErr == nil && apiStatus != nil {
			return *apiStatus
		}
	}

	if !searchResultVisibleInSearch(sr, viewerUsername) {
		return models.Status{}
	}
	return h.convertThinStatusResultToAPI(ctx, sr)
}

func (h *Handler) convertThinStatusResultToAPI(ctx *apptheory.Context, sr *storage.StatusSearchResult) models.Status {
	// Convert search result to object map for transformation framework
	statusMap := map[string]interface{}{
		"id":        sr.StatusID,
		"content":   sr.Content,
		"url":       sr.URL,
		"published": sr.Published.Format(time.RFC3339),
	}

	// Use centralized transformation framework - ELIMINATES 8+ LINES OF DUPLICATE CODE
	transformer := transformations.NewStatusResponseTransformer(h.cfg.BaseURL(), transformations.ObjectToStatusWithContext)
	transformCtx := context.WithValue(ctx.Context(), baseURLContextKey, h.cfg.BaseURL())

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
			account := h.convertActorToAccount(ctx.Context(), statusActor)
			status.Account = account
		}
	}

	return status
}

// getActorFromAuthorID extracts actor from author ID
func (h *Handler) getActorFromAuthorID(ctx *apptheory.Context, authorID string) *activitypub.Actor {
	if strings.TrimSpace(authorID) == "" {
		return nil
	}
	actor, _ := h.resolveAccountID(ctx.Context(), authorID)
	return actor
}

func (h *Handler) resolveStatusBySearchURL(ctx context.Context, statusURL string) (*storagemodels.Status, error) {
	if h == nil || h.repos == nil || h.repos.Status() == nil {
		return nil, errors.New("status repository unavailable")
	}

	if status, err := h.repos.Status().GetStatusByURL(ctx, strings.TrimSpace(statusURL)); err == nil && status != nil {
		return status, nil
	}

	for _, candidate := range h.statusLookupCandidates(statusURL) {
		status, err := h.repos.Status().GetStatus(ctx, candidate)
		if err == nil && status != nil {
			return status, nil
		}
	}

	return nil, errors.New("status not found")
}

func (h *Handler) resolveStatusFromSearchResult(ctx context.Context, sr *storage.StatusSearchResult) (*storagemodels.Status, error) {
	if h == nil || h.repos == nil || h.repos.Status() == nil || sr == nil {
		return nil, errors.New("status repository unavailable")
	}

	for _, statusURL := range []string{strings.TrimSpace(sr.URL), strings.TrimSpace(sr.StatusID)} {
		if statusURL == "" || !strings.Contains(statusURL, "://") {
			continue
		}
		status, err := h.repos.Status().GetStatusByURL(ctx, statusURL)
		if err == nil && status != nil {
			return status, nil
		}
	}

	candidates := make([]string, 0, 6)
	candidates = append(candidates, h.statusLookupCandidates(sr.URL)...)
	candidates = append(candidates, h.statusLookupCandidates(sr.StatusID)...)
	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if _, exists := seen[candidate]; exists {
			continue
		}
		seen[candidate] = struct{}{}

		status, err := h.repos.Status().GetStatus(ctx, candidate)
		if err == nil && status != nil {
			return status, nil
		}
	}
	return nil, errors.New("status not found")
}

func (h *Handler) statusLookupCandidates(value string) []string {
	localDomain := ""
	if h != nil && h.cfg != nil {
		localDomain = h.cfg.Domain
	}

	return storagemodels.StatusLookupCandidatesForDomain(value, localDomain)
}

func shouldAugmentStatusSearchByAuthor(query string) bool {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return false
	}
	if strings.ContainsAny(trimmed, " \t\r\n") {
		return false
	}
	if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, schemeHTTP) {
		return false
	}
	return true
}

func (h *Handler) addAuthorMatchedStatuses(ctx *apptheory.Context, params *SearchParams, viewerUsername string, seen map[string]struct{}, result *models.SearchResult) {
	if len(result.Statuses) >= params.Limit || params.AccountID != "" || !shouldAugmentStatusSearchByAuthor(params.Query) {
		return
	}
	if h == nil || h.repos == nil || h.repos.Search() == nil || h.repos.Status() == nil {
		return
	}

	actors, err := h.repos.Search().SearchAccounts(ctx.Context(), params.Query, params.Limit, false, 0)
	if err != nil {
		h.logger.Debug("status author search fallback failed", zap.String("query", params.Query), zap.Error(err))
		return
	}

	for _, actor := range actors {
		if actor == nil || strings.TrimSpace(actor.ID) == "" {
			continue
		}

		remaining := params.Limit - len(result.Statuses)
		if remaining <= 0 {
			return
		}

		timeline, timelineErr := h.repos.Status().GetUserTimeline(ctx.Context(), actor.ID, interfaces.PaginationOptions{Limit: remaining})
		if timelineErr != nil || timeline == nil {
			continue
		}

		for _, status := range timeline.Items {
			if len(result.Statuses) >= params.Limit {
				return
			}
			if !h.statusVisibleInSearch(status, viewerUsername) {
				continue
			}

			apiStatus, convErr := h.convertStorageStatusToAPI(status, viewerUsername)
			if convErr != nil || apiStatus == nil || strings.TrimSpace(apiStatus.ID) == "" {
				continue
			}
			if _, exists := seen[apiStatus.ID]; exists {
				continue
			}

			result.Statuses = append(result.Statuses, *apiStatus)
			seen[apiStatus.ID] = struct{}{}
		}
	}
}

func (h *Handler) statusVisibleInSearch(status *storagemodels.Status, viewerUsername string) bool {
	return statusVisibleInSearchForViewer(status, viewerUsername, h.searchViewerActorID(viewerUsername))
}

func statusVisibleInSearchForViewer(status *storagemodels.Status, viewerUsername, viewerActorID string) bool {
	if status == nil {
		return false
	}
	if status.Deleted {
		return false
	}

	switch status.Visibility {
	case storagemodels.VisibilityPublic, storagemodels.VisibilityUnlisted:
		return true
	}

	viewerUsername = strings.TrimSpace(viewerUsername)
	if viewerUsername == "" {
		return false
	}
	if strings.EqualFold(viewerUsername, strings.TrimSpace(status.AuthorUsername)) {
		return true
	}
	viewerActorID = strings.TrimSpace(viewerActorID)
	if viewerActorID == "" {
		return false
	}
	return status.IsVisibleTo(viewerActorID)
}

func (h *Handler) searchViewerActorID(viewerUsername string) string {
	viewerUsername = strings.TrimSpace(viewerUsername)
	if viewerUsername == "" || h == nil || h.cfg == nil {
		return ""
	}
	return h.cfg.ActorURL(viewerUsername)
}

func searchResultVisibleInSearch(result *storage.StatusSearchResult, viewerUsername string) bool {
	if result == nil {
		return false
	}

	switch strings.TrimSpace(strings.ToLower(result.Visibility)) {
	case storagemodels.VisibilityPublic, storagemodels.VisibilityUnlisted:
		return true
	case storagemodels.VisibilityPrivate, storagemodels.VisibilityDirect:
		return strings.TrimSpace(viewerUsername) != "" &&
			strings.EqualFold(strings.TrimSpace(viewerUsername), strings.TrimSpace(result.AuthorUsername))
	default:
		return false
	}
}

// convertHashtagToTag converts hashtag with history to API tag
func (h *Handler) convertHashtagToTag(ctx *apptheory.Context, hashtag storage.Hashtag) models.Tag {
	// Get usage history for the last 7 days
	history, _ := h.repos.Hashtag().GetHashtagUsageHistory(ctx.Context(), hashtag.Name, 7)

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
func (h *Handler) HandleSearchV2Lift(ctx *apptheory.Context) (*apptheory.Response, error) {
	params, resp, err := h.parseSearchParams(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	if exact, handled, err := h.searchV2ExactActorResult(ctx.Context(), params); handled {
		if err != nil {
			h.logger.Error("exact actor search failed", zap.String("query", params.Query), zap.Error(err))
			return common.RespondInternalServerError(ctx, "search failed")
		}
		return okJSON(exact)
	}

	return h.HandleSearchLift(ctx)
}

func (h *Handler) searchV2ExactActorResult(ctx context.Context, params *SearchParams) (*models.SearchResult, bool, error) {
	if !supportsExactActorAPISearch(params.Type) || !looksLikeExactActorSearchQuery(params.Query) {
		return nil, false, nil
	}
	if h.searchV2ExactActorRequiresResolve(params.Query) && !params.Resolve {
		return nil, false, nil
	}

	actor, err := h.resolveAccountID(ctx, params.Query)
	if err != nil {
		if actorSearchNotFound(err) {
			return &models.SearchResult{
				Accounts: []models.Account{},
				Statuses: []models.Status{},
				Hashtags: []models.Tag{},
			}, true, nil
		}
		return nil, true, err
	}
	if actor == nil {
		return &models.SearchResult{
			Accounts: []models.Account{},
			Statuses: []models.Status{},
			Hashtags: []models.Tag{},
		}, true, nil
	}

	return &models.SearchResult{
		Accounts: []models.Account{h.publicAccountFromActor(ctx, actor)},
		Statuses: []models.Status{},
		Hashtags: []models.Tag{},
	}, true, nil
}

func (h *Handler) searchV2ExactActorRequiresResolve(query string) bool {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return false
	}

	localDomain := ""
	if h != nil && h.cfg != nil {
		localDomain = normalizeLocalActorDomain(h.cfg.Domain)
	}

	if looksLikeActorURLSearchQuery(trimmed) {
		parsed, err := url.Parse(trimmed)
		if err != nil || parsed == nil {
			return false
		}
		host := normalizeLocalActorDomain(parsed.Hostname())
		return host != "" && host != localDomain
	}

	handle := strings.TrimPrefix(trimmed, "@")
	parts := strings.SplitN(handle, "@", 2)
	if len(parts) != 2 {
		return false
	}

	domain := normalizeLocalActorDomain(parts[1])
	return domain != "" && domain != localDomain
}

func supportsExactActorAPISearch(searchType string) bool {
	switch strings.ToLower(strings.TrimSpace(searchType)) {
	case "", "accounts":
		return true
	default:
		return false
	}
}

func looksLikeExactActorSearchQuery(query string) bool {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return false
	}
	if looksLikeActorURLSearchQuery(trimmed) {
		return true
	}
	return strings.Count(strings.TrimPrefix(trimmed, "@"), "@") == 1
}

func looksLikeActorURLSearchQuery(query string) bool {
	parsed, err := url.Parse(strings.TrimSpace(query))
	if err != nil || parsed == nil || parsed.Scheme == "" || parsed.Host == "" {
		return false
	}

	path := strings.Trim(strings.ToLower(parsed.Path), "/")
	if path == "" {
		return false
	}
	if strings.Contains(path, "/statuses/") || strings.Contains(path, "/objects/") {
		return false
	}
	if strings.HasPrefix(path, "@") {
		return true
	}

	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return false
	}

	switch parts[0] {
	case "users", "actors", "profiles":
		return strings.TrimSpace(parts[1]) != ""
	default:
		return false
	}
}

func actorSearchNotFound(err error) bool {
	if err == nil {
		return false
	}
	if common.IsNotFound(err) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "not found")
}

// HandleGetNotificationsLift retrieves notifications for the authenticated user
func (h *Handler) HandleGetNotificationsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
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

	listResult, err := notificationService.ListNotifications(ctx.Context(), &notifications.ListNotificationsQuery{
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
		var data map[string]interface{}
		if len(notification.Data) > 0 || notification.Title != "" || notification.Body != "" {
			data = make(map[string]interface{}, len(notification.Data)+2)
			for key, value := range notification.Data {
				data[key] = value
			}
			if notification.Title != "" {
				data["subject"] = notification.Title
			}
			if notification.Body != "" {
				data["body"] = notification.Body
			}
		}

		storageNotif := &storage.Notification{
			ID:        notification.ID,
			Type:      notification.Type,
			AccountID: notification.ActorID,
			TargetID:  notification.TargetID,
			Read:      notification.IsRead,
			CreatedAt: notification.CreatedAt,
			Username:  notification.UserID,
			Data:      data,
		}
		if notification.TargetType == notificationTargetTypeStatus {
			storageNotif.StatusID = notification.TargetID
		}
		storageNotifications = append(storageNotifications, storageNotif)
	}

	// Convert notifications to API format
	apiNotifications := h.convertNotificationsToAPI(ctx, storageNotifications)

	// Set pagination header if needed
	if cursor != "" {
		h.setNotificationPaginationHeader(ctx, cursor, notificationFilter.Limit)
	}

	return okJSON(apiNotifications)
}

// buildNotificationFilter builds a notification filter from query parameters
func (h *Handler) buildNotificationFilter(ctx *apptheory.Context) *storage.NotificationFilter {
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
	if accountID := queryValue(ctx, "account_id"); accountID != "" {
		filter.AccountID = accountID
	}

	// Parse pagination parameters
	filter.MaxID = queryValue(ctx, "max_id")
	filter.MinID = queryValue(ctx, "min_id")
	filter.SinceID = queryValue(ctx, "since_id")

	return filter
}

// parseNotificationLimit parses and validates the limit parameter
func (h *Handler) parseNotificationLimit(ctx *apptheory.Context, filter *storage.NotificationFilter) {
	if limitStr := queryValue(ctx, "limit"); limitStr != "" {
		if limit, err := common.ParseAndValidateAPILimit(limitStr, 40); err == nil {
			filter.Limit = limit
		}
	}
}

// parseNotificationTypes parses the types filter parameter
func (h *Handler) parseNotificationTypes(ctx *apptheory.Context, filter *storage.NotificationFilter) {
	if types := queryValue(ctx, "types[]"); types != "" {
		filter.Types = []string{types}
	} else if typesStr := queryValue(ctx, "types"); typesStr != "" {
		filter.Types = strings.Split(typesStr, ",")
	}
}

// parseNotificationExcludeTypes parses the exclude_types filter parameter
func (h *Handler) parseNotificationExcludeTypes(ctx *apptheory.Context, filter *storage.NotificationFilter) {
	if excludeTypes := queryValue(ctx, "exclude_types[]"); excludeTypes != "" {
		filter.ExcludeTypes = []string{excludeTypes}
	} else if excludeTypesStr := queryValue(ctx, "exclude_types"); excludeTypesStr != "" {
		filter.ExcludeTypes = strings.Split(excludeTypesStr, ",")
	}
}

// convertNotificationsToAPI converts storage notifications to API format
func (h *Handler) convertNotificationsToAPI(ctx *apptheory.Context, notifications []*storage.Notification) []*models.Notification {
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
func (h *Handler) convertSingleNotification(ctx *apptheory.Context, notif *storage.Notification) *models.Notification {
	// Get the account that triggered the notification
	actor := h.notificationActor(ctx.Context(), notif.AccountID)
	if actor == nil {
		return nil
	}

	account := transformations.ActorToAccountBase(actor, h.cfg.BaseURL())
	apiNotif := &models.Notification{
		ID:        notif.ID,
		Type:      notif.Type,
		CreatedAt: notif.CreatedAt,
		Account:   account,
		Read:      notif.Read,
	}
	apiNotif.Communication = communicationNotificationFromData(notif.Type, notif.CreatedAt, notif.Data)

	// Add status if applicable
	if h.shouldIncludeStatus(notif) {
		h.attachStatusToNotification(ctx, notif, apiNotif)
	}

	return apiNotif
}

// shouldIncludeStatus checks if a status should be included in the notification
func (h *Handler) shouldIncludeStatus(notif *storage.Notification) bool {
	if notif == nil {
		return false
	}

	if notif.Type != models.NotificationTypeMention &&
		notif.Type != models.NotificationTypeReply &&
		notif.Type != models.NotificationTypeFavourite &&
		notif.Type != models.NotificationTypeReblog {
		return false
	}

	if _, ok := h.notificationPostSnapshot(notif); ok {
		return true
	}

	return common.ValidateMastodonStatusID(notif.StatusID) == nil
}

// attachStatusToNotification attaches status information to a notification
func (h *Handler) attachStatusToNotification(ctx *apptheory.Context, notif *storage.Notification, apiNotif *models.Notification) {
	if snapshotStatus := h.statusFromNotificationSnapshot(ctx, notif); snapshotStatus != nil {
		apiNotif.Status = snapshotStatus
		return
	}

	// Get the status
	obj, err := h.repos.Object().GetObject(ctx.Context(), notif.StatusID)
	if err != nil {
		h.logger.Warn("failed to get status for notification",
			zap.String("notification_id", notif.ID),
			zap.String("status_id", notif.StatusID),
			zap.Error(err))
		return
	}

	if !h.notificationObjectVisibleToViewer(ctx.Context(), notif, obj) {
		h.logger.Debug("skipping notification status expansion due to visibility",
			zap.String("notification_id", notif.ID),
			zap.String("status_id", notif.StatusID),
			zap.String("viewer", notif.Username))
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

func (h *Handler) notificationPostSnapshot(notif *storage.Notification) (map[string]interface{}, bool) {
	if notif == nil || len(notif.Data) == 0 {
		return nil, false
	}

	rawSnapshot, ok := notif.Data[notificationPostSnapshotKey]
	if !ok {
		return nil, false
	}

	snapshot, ok := rawSnapshot.(map[string]interface{})
	if !ok || len(snapshot) == 0 {
		return nil, false
	}

	return snapshot, true
}

func (h *Handler) statusFromNotificationSnapshot(ctx *apptheory.Context, notif *storage.Notification) *models.Status {
	snapshot, ok := h.notificationPostSnapshot(notif)
	if !ok {
		return nil
	}

	if !h.notificationSnapshotVisibleToViewer(ctx.Context(), notif, snapshot) {
		h.logger.Debug("skipping notification snapshot expansion due to visibility",
			zap.String("notification_id", notif.ID),
			zap.String("viewer", notif.Username))
		return nil
	}

	statusActor := h.notificationSnapshotActor(ctx, snapshot)
	status := transformations.ObjectToStatusBase(notificationSnapshotObjectMap(snapshot), statusActor, h.cfg.BaseURL())
	if status.ID == "" {
		return nil
	}

	if url := strings.TrimSpace(notificationSnapshotString(snapshot, "url")); url != "" {
		status.URL = url
	}
	if visibility := strings.TrimSpace(notificationSnapshotString(snapshot, "visibility")); visibility != "" {
		status.Visibility = visibility
	}

	return &status
}

func (h *Handler) notificationSnapshotVisibleToViewer(ctx context.Context, notif *storage.Notification, snapshot map[string]interface{}) bool {
	return h.notificationStatusVisibleToViewer(ctx,
		notif,
		notificationSnapshotString(snapshot, "visibility"),
		notificationSnapshotString(snapshot, "attributedTo"),
		nil,
		nil,
	)
}

func (h *Handler) notificationObjectVisibleToViewer(ctx context.Context, notif *storage.Notification, obj any) bool {
	visibility, attributedTo, recipients, mentions := notificationObjectVisibilityContext(obj)
	return h.notificationStatusVisibleToViewer(ctx, notif, visibility, attributedTo, recipients, mentions)
}

func (h *Handler) notificationStatusVisibleToViewer(
	ctx context.Context,
	notif *storage.Notification,
	visibility string,
	attributedTo string,
	recipients []string,
	mentions []string,
) bool {
	visibility = strings.TrimSpace(visibility)
	if visibility == "" {
		visibility = storagemodels.VisibilityPublic
	}

	switch visibility {
	case storagemodels.VisibilityPublic, storagemodels.VisibilityUnlisted:
		return true
	}

	if notif == nil {
		return false
	}
	viewer := strings.TrimSpace(notif.Username)
	if viewer == "" {
		return false
	}
	author := extractUsernameFromNotificationActorID(attributedTo)
	if author != "" && strings.EqualFold(viewer, author) {
		return true
	}

	switch visibility {
	case storagemodels.VisibilityPrivate:
		return h.notificationViewerFollowsAuthor(ctx, viewer, author)
	case storagemodels.VisibilityDirect:
		return h.notificationDirectVisibleToViewer(viewer, recipients, mentions)
	default:
		h.logger.Warn("unknown notification status visibility",
			zap.String("notification_id", notif.ID),
			zap.String("visibility", visibility))
		return false
	}
}

func (h *Handler) notificationViewerFollowsAuthor(ctx context.Context, viewer, author string) bool {
	if viewer == "" || author == "" || h == nil || h.repos == nil || h.repos.Relationship() == nil {
		return false
	}
	ok, err := h.repos.Relationship().IsFollowing(ctx, viewer, author)
	if err != nil {
		h.logger.Warn("failed to check notification status visibility",
			zap.String("viewer", viewer),
			zap.String("author", author),
			zap.Error(err))
		return false
	}
	return ok
}

func (h *Handler) notificationDirectVisibleToViewer(viewer string, recipients []string, mentions []string) bool {
	if strings.TrimSpace(viewer) == "" {
		return false
	}
	// Legacy notification snapshots did not persist direct-message recipient
	// arrays. A notification is already scoped to a single owner, so keep those
	// owner-visible while enforcing recipient checks whenever recipient evidence
	// is present.
	if len(recipients) == 0 && len(mentions) == 0 {
		return true
	}
	viewerActorID := ""
	if h != nil && h.cfg != nil {
		viewerActorID = h.cfg.ActorURL(viewer)
	}
	for _, value := range append(recipients, mentions...) {
		if mentionMatchesNotificationViewer(value, viewer, viewerActorID) {
			return true
		}
	}
	return false
}

func (h *Handler) notificationSnapshotActor(ctx *apptheory.Context, snapshot map[string]interface{}) *activitypub.Actor {
	attributedTo := strings.TrimSpace(notificationSnapshotString(snapshot, "attributedTo"))
	if attributedTo == "" {
		return nil
	}

	fallbackActor := h.fallbackNotificationActor(attributedTo)
	if cached := h.cachedRemoteNotificationActor(ctx.Context(), attributedTo); cached != nil {
		return cached
	}
	username := extractUsernameFromNotificationActorID(attributedTo)
	if username == "" {
		return fallbackActor
	}

	statusActor, err := h.repos.Actor().GetActor(ctx.Context(), username)
	if err != nil {
		return fallbackActor
	}

	return statusActor
}

func (h *Handler) notificationActor(ctx context.Context, actorID string) *activitypub.Actor {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return nil
	}

	if h != nil && h.repos != nil && h.repos.Actor() != nil {
		if actor, err := h.repos.Actor().GetActor(ctx, actorID); err == nil && actor != nil {
			return actor
		} else if err != nil && h.logger != nil {
			h.logger.Warn("failed to get local actor for notification; trying remote cache/fallback",
				zap.String("account_id", actorID),
				zap.Error(err))
		}
		if actor := h.cachedRemoteNotificationActor(ctx, actorID); actor != nil {
			return actor
		}
	}
	return h.fallbackNotificationActor(actorID)
}

func (h *Handler) cachedRemoteNotificationActor(ctx context.Context, actorID string) *activitypub.Actor {
	if h == nil || h.repos == nil || h.repos.Actor() == nil {
		return nil
	}
	candidates := []string{actorID}
	if username := extractUsernameFromNotificationActorID(actorID); username != "" && username != actorID {
		candidates = append(candidates, username)
	}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		actor, err := h.repos.Actor().GetCachedRemoteActor(ctx, candidate)
		if err == nil && actor != nil {
			return actor
		}
	}
	return nil
}

func notificationObjectVisibilityContext(obj any) (visibility string, attributedTo string, recipients []string, mentions []string) {
	switch typed := obj.(type) {
	case *activitypub.Note:
		return typed.Visibility, typed.AttributedTo, appendNotificationRecipients(nil, typed.To, typed.CC, typed.BTo, typed.BCC), noteMentionTargets(typed.Tag)
	case *storagemodels.Object:
		return typed.Visibility, typed.AttributedTo, appendNotificationRecipients(nil, typed.To, typed.CC, typed.BTo, typed.BCC), nil
	case map[string]interface{}:
		return notificationSnapshotString(typed, "visibility"),
			notificationSnapshotString(typed, "attributedTo"),
			appendNotificationRecipients(nil,
				notificationStringSliceFromAny(typed["to"]),
				notificationStringSliceFromAny(typed["cc"]),
				notificationStringSliceFromAny(typed["bto"]),
				notificationStringSliceFromAny(typed["bcc"])),
			notificationMentionTargetsFromAny(typed["tag"])
	default:
		return "", "", nil, nil
	}
}

func appendNotificationRecipients(dst []string, lists ...[]string) []string {
	for _, list := range lists {
		for _, value := range list {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				dst = append(dst, trimmed)
			}
		}
	}
	return dst
}

func noteMentionTargets(tags []activitypub.Tag) []string {
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		if strings.EqualFold(strings.TrimSpace(tag.Type), "Mention") {
			out = appendNotificationRecipients(out, []string{tag.Href, tag.Name})
		}
	}
	return out
}

func notificationMentionTargetsFromAny(value interface{}) []string {
	items, ok := value.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		tag, ok := item.(map[string]interface{})
		if !ok || !strings.EqualFold(notificationSnapshotString(tag, "type"), "Mention") {
			continue
		}
		out = appendNotificationRecipients(out, []string{
			notificationSnapshotString(tag, "href"),
			notificationSnapshotString(tag, "name"),
		})
	}
	return out
}

func notificationStringSliceFromAny(value interface{}) []string {
	switch typed := value.(type) {
	case []string:
		return appendNotificationRecipients(nil, typed)
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if str, ok := item.(string); ok {
				out = appendNotificationRecipients(out, []string{str})
			}
		}
		return out
	default:
		return nil
	}
}

func mentionMatchesNotificationViewer(value, viewer, viewerActorID string) bool {
	value = strings.TrimSpace(strings.TrimPrefix(value, "@"))
	viewer = strings.TrimSpace(viewer)
	if value == "" || viewer == "" {
		return false
	}
	if strings.EqualFold(value, viewer) {
		return true
	}
	if viewerActorID != "" && strings.EqualFold(value, viewerActorID) {
		return true
	}
	return strings.EqualFold(extractUsernameFromNotificationActorID(value), viewer)
}

func notificationSnapshotObjectMap(snapshot map[string]interface{}) map[string]interface{} {
	objectMap := map[string]interface{}{
		"id":        notificationSnapshotString(snapshot, "id"),
		"content":   notificationSnapshotString(snapshot, "content"),
		"published": notificationSnapshotString(snapshot, "createdAt"),
	}

	if inReplyToID := strings.TrimSpace(notificationSnapshotString(snapshot, "inReplyToId")); inReplyToID != "" {
		objectMap["inReplyTo"] = inReplyToID
	}

	return objectMap
}

func notificationSnapshotString(snapshot map[string]interface{}, key string) string {
	rawValue, ok := snapshot[key]
	if !ok {
		return ""
	}

	value, ok := rawValue.(string)
	if !ok {
		return ""
	}

	return value
}

// extractStatusAuthor extracts the author actor from a status object
func (h *Handler) extractStatusAuthor(ctx *apptheory.Context, obj any) *activitypub.Actor {
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
	statusActor, _ := h.repos.Actor().GetActor(ctx.Context(), username)
	return statusActor
}

// setNotificationPaginationHeader sets the pagination Link header for notifications
func (h *Handler) setNotificationPaginationHeader(ctx *apptheory.Context, cursor string, limit int) {
	host := headerValue(ctx, "host")
	if err := common.ValidateRequiredParam("host", host); err != nil {
		host = headerValue(ctx, "Host")
	}

	baseURL := fmt.Sprintf("https://%s%s", host, "/api/v1/notifications")
	nextURL := fmt.Sprintf("%s?max_id=%s", baseURL, cursor)
	if limit > 0 {
		nextURL += fmt.Sprintf("&limit=%d", limit)
	}
	ctx.Set("Link", fmt.Sprintf(`<%s>; rel="next"`, nextURL))
}

// HandleGetInstanceV2Lift returns instance information in v2 format
func (h *Handler) HandleGetInstanceV2Lift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Get static config
	instanceConfig := config.GetInstanceConfig()
	locked := h.instanceLocked(ctx.Context())

	// Log configuration values
	h.logger.Info("HandleGetInstanceV2Lift called",
		zap.String("cfg.Domain", h.cfg.Domain),
		zap.String("cfg.BaseURL", h.cfg.BaseURL()),
		zap.String("instanceConfig.Title", instanceConfig.Title),
		zap.String("instanceConfig.Version", instanceConfig.Version),
	)

	rules := h.instanceRules(ctx.Context())

	vapidPublicKey, vapidResp, err := h.resolveVAPIDPublicKey(ctx, true)
	if vapidResp != nil || err != nil {
		return vapidResp, err
	}

	// Convert rules for API response
	apiRules := instanceRulesToAPIRules(rules)

	instanceResp := models.InstanceV2Response{
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
				"enabled": h.isTranslationEnabled(ctx.Context()),
			},
			"trust":              h.instanceTrustConfig(ctx.Context()),
			"tips":               h.instanceTipsConfig(ctx.Context()),
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
		zap.String("domain", instanceResp.Domain),
		zap.String("title", instanceResp.Title),
		zap.String("version", instanceResp.Version),
	)

	return okJSON(instanceResp)
}

func instanceRulesToAPIRules(rules []storage.InstanceRule) []models.Rule {
	apiRules := make([]models.Rule, len(rules))
	for i, rule := range rules {
		apiRules[i] = models.Rule{ID: rule.ID, Text: rule.Text}
	}
	return apiRules
}

// HandleGetNotificationLift handles GET /api/v1/notifications/:id
func (h *Handler) HandleGetNotificationLift(ctx *apptheory.Context) (*apptheory.Response, error) {
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
	notification, err := h.repos.Notification().GetNotification(ctx.Context(), notificationID)
	if err != nil {
		return common.RespondNotFound(ctx, "notification")
	}

	// Verify ownership
	if notification.UserID != username {
		return common.RespondNotFound(ctx, "notification")
	}

	// Get the account that triggered the notification
	actor := h.notificationActor(ctx.Context(), notification.ActorID)
	if actor == nil {
		return common.RespondFailedToGet(ctx, "notification details")
	}

	account := transformations.ActorToAccountBase(actor, h.cfg.BaseURL())
	apiNotif := &models.Notification{
		ID:        notification.ID,
		Type:      notification.Type,
		CreatedAt: notification.CreatedAt,
		Account:   account,
		Read:      notification.IsRead,
	}
	{
		var data map[string]interface{}
		if len(notification.Data) > 0 || notification.Title != "" || notification.Body != "" {
			data = make(map[string]interface{}, len(notification.Data)+2)
			for key, value := range notification.Data {
				data[key] = value
			}
			if notification.Title != "" {
				data["subject"] = notification.Title
			}
			if notification.Body != "" {
				data["body"] = notification.Body
			}
		}
		apiNotif.Communication = communicationNotificationFromData(notification.Type, notification.CreatedAt, data)
	}

	// Add status if applicable
	if notification.TargetID != "" && notification.TargetType == notificationTargetTypeStatus && (notification.Type == models.NotificationTypeMention ||
		notification.Type == models.NotificationTypeFavourite ||
		notification.Type == models.NotificationTypeReblog) {
		statusModel, err := h.registry.Notes().GetNoteWithViewer(ctx.Context(), &notes.GetNoteQuery{
			StatusID: notification.TargetID,
			ViewerID: username,
		})
		if err == nil && statusModel != nil && statusModel.Note != nil {
			// Convert note to status format
			var statusActor *activitypub.Actor
			if statusModel.AuthorUsername != "" {
				account, _ := h.registry.Accounts().GetAccount(ctx.Context(), statusModel.AuthorUsername)
				if account != nil {
					statusActor = account.Actor
				}
			}
			// Use the embedded Note from the status model
			status := transformations.ObjectToStatusAny(statusModel.Note, statusActor, h.cfg.BaseURL())
			apiNotif.Status = &status
		}
	}

	return okJSON(apiNotif)
}

// HandleClearNotificationsLift handles POST /api/v1/notifications/clear
func (h *Handler) HandleClearNotificationsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
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

	clearResult, err := notificationService.ClearNotifications(ctx.Context(), &notifications.ClearCommand{
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

	return noContent(), nil
}

// HandleDismissNotificationLift handles POST /api/v1/notifications/:id/dismiss
func (h *Handler) HandleDismissNotificationLift(ctx *apptheory.Context) (*apptheory.Response, error) {
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
	_, err = notificationService.MarkAsRead(ctx.Context(), &notifications.MarkAsReadCommand{
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

	return noContent(), nil
}

// HandleGetInstanceCostsLift returns cost analytics for the instance
func (h *Handler) HandleGetInstanceCostsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	h.logger.Info("HandleGetInstanceCostsLift called")

	// Initialize cost storage if not already done
	costTableName := h.cfg.CostHistoryTableName
	if err := common.ValidateRequiredParam("COST_HISTORY_TABLE_NAME", costTableName); err != nil {
		// Return placeholder data if cost tracking is not configured
		response := map[string]any{
			"error": "Cost tracking not configured",
		}
		return okJSON(response)
	}

	// Get current month data
	now := time.Now()
	currentMonth, err := h.repos.Cost().GetMonthlyAggregate(ctx.Context(), now.Year(), int(now.Month()))
	if err != nil {
		h.logger.Error("failed to get monthly cost", zap.Error(err))
	}

	// Get daily costs for the last 7 days
	endDate := now
	startDate := now.AddDate(0, 0, -6) // 7 days including today
	dailyCosts, err := h.repos.Cost().GetDailyAggregates(ctx.Context(), startDate, endDate)
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

	return okJSON(response)
}

// HandleGetInstanceConfigurationLift returns configuration details
func (h *Handler) HandleGetInstanceConfigurationLift(_ *apptheory.Context) (*apptheory.Response, error) {
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

	return okJSON(config)
}

// Helper functions

// getUniqueAccountsForDay returns unique account count for a specific day
func (h *Handler) getUniqueAccountsForDay(ctx *apptheory.Context, day string) string {
	// Parse the day string to get the date (validation only)
	_, err := time.Parse(common.DateFormat, day)
	if err != nil {
		h.logger.Warn("invalid day format", zap.String("day", day), zap.Error(err))
		return "0"
	}

	// Get unique active users for that specific day
	count, err := h.repos.Instance().GetDailyActiveUserCount(ctx.Context())
	if err != nil {
		h.logger.Error("failed to get daily active user count",
			zap.String("day", day), zap.Error(err))
		return "0"
	}

	return fmt.Sprintf("%d", count)
}

// getActiveMonthlyUsers returns the count of active users in the current month
func (h *Handler) getActiveMonthlyUsers(ctx *apptheory.Context) int {
	// Get count of users who have been active in the last 30 days
	count, err := h.repos.Analytics().GetActiveUserCount(ctx.Context(), 30)
	if err != nil {
		h.logger.Error("failed to get active monthly users", zap.Error(err))
		return 1 // Default fallback
	}
	return count
}

// getAdminAccount returns the admin account for the instance
func (h *Handler) getAdminAccount(ctx *apptheory.Context) any {
	// Get admin username from config
	adminUsername := h.cfg.AdminUsername
	if err := common.ValidateRequiredParam("ADMIN_USERNAME", adminUsername); err != nil {
		return nil
	}

	// Get admin actor
	actor, err := h.repos.Actor().GetActor(ctx.Context(), adminUsername)
	if err != nil {
		h.logger.Error("failed to get admin account", zap.String("username", adminUsername), zap.Error(err))
		return nil
	}

	// Get counts
	followerCount, _ := h.repos.Relationship().CountFollowers(ctx.Context(), actor.ID)
	followingCount, _ := h.repos.Relationship().CountFollowing(ctx.Context(), actor.ID)
	statusesCount, _ := h.repos.Status().CountStatusesByAuthor(ctx.Context(), actor.ID)

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

	privateKeyBytes, err := privateKey.Bytes()
	if err != nil {
		return nil, errors.Join(failedToGenerateVAPIDPrivateKey(), err)
	}
	privateKeyBase64 := base64.RawURLEncoding.EncodeToString(privateKeyBytes)

	// Determine the subject (domain)
	domain := h.cfg.Domain
	if err := common.ValidateRequiredParam("domain", domain); err != nil {
		domain = localhostDomain // fallback for development
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
func (h *Handler) HandleGetGroupedNotificationsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
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

	listResult, err := notificationService.ListNotifications(ctx.Context(), &notifications.ListNotificationsQuery{
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
		ctx.Context(),
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

	return okJSON(apiResponse)
}

// parseGroupingOptions parses grouping options from query parameters
func (h *Handler) parseGroupingOptions(ctx *apptheory.Context) *notifications.GroupingStrategy {
	strategy := notifications.DefaultGroupingStrategy()

	// Parse time window (in hours)
	if timeWindowStr := queryValue(ctx, "time_window"); timeWindowStr != "" {
		if hours, err := common.ParseAndValidateIntWithBounds("time_window", timeWindowStr, 0, 168, 0); err == nil { // Max 1 week
			strategy.TimeWindow = time.Duration(hours) * time.Hour
		}
	}

	// Parse max group size
	if maxSizeStr := queryValue(ctx, "max_group_size"); maxSizeStr != "" {
		if maxSize, err := common.ParseAndValidateIntWithBounds("max_group_size", maxSizeStr, 0, 100, 0); err == nil {
			strategy.MaxGroupSize = maxSize
		}
	}

	// Parse min group size
	if minSizeStr := queryValue(ctx, "min_group_size"); minSizeStr != "" {
		if minSize, err := common.ParseAndValidateIntWithBounds("min_group_size", minSizeStr, 0, 10, 0); err == nil && minSize >= 1 {
			strategy.MinGroupSize = minSize
		}
	}

	// Parse sample size
	if sampleSizeStr := queryValue(ctx, "sample_size"); sampleSizeStr != "" {
		if sampleSize, err := common.ParseAndValidateIntWithBounds("sample_size", sampleSizeStr, 0, 10, 0); err == nil {
			strategy.SampleSize = sampleSize
		}
	}

	// Parse grouping flags
	if groupByType := queryValue(ctx, "group_by_type"); groupByType != "" {
		if result, _ := common.ParseAndValidateBoolean(groupByType); !result {
			strategy.GroupByType = false
		}
	}

	if groupByTarget := queryValue(ctx, "group_by_target"); groupByTarget != "" {
		if result, _ := common.ParseAndValidateBoolean(groupByTarget); !result {
			strategy.GroupByTarget = false
		}
	}

	return strategy
}

// convertGroupedNotificationsToAPI converts grouped notifications to API format
func (h *Handler) convertGroupedNotificationsToAPI(
	ctx *apptheory.Context,
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
		if func() bool {
			result, _ := common.ParseAndValidateBoolean(queryValue(ctx, "include_all"))
			return result
		}() && len(group.AllNotifications) > 0 {
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
func (h *Handler) HandleMarkGroupAsReadLift(ctx *apptheory.Context) (*apptheory.Response, error) {
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

	_, err = notificationService.MarkAsRead(ctx.Context(), &notifications.MarkAsReadCommand{
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

	return okJSON(models.MessageResponse{Message: "group marked as read"})
}
