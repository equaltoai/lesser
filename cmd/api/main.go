package main

/*
MASTODON API IMPLEMENTATION STATUS

✅ IMPLEMENTED (Basic functionality):
- OAuth app registration (POST /api/v1/apps)
- Account registration (POST /api/v1/accounts)
- Account verification (GET /api/v1/accounts/verify_credentials)
- Account updates (PATCH /api/v1/accounts/update_credentials)
- Status CRUD operations
- Basic timeline endpoints (home, public)
- Follow/unfollow functionality
- Block/unblock functionality
- Basic search
- Basic notifications

🚧 PARTIALLY IMPLEMENTED (Needs improvement):
- Instance information (missing fields)
- Search (limited functionality)
- Notifications (basic structure only)

❌ NOT IMPLEMENTED (Major features):
- Media uploads (partial - upload works, GET/PUT need work)
- Lists management (partial - basic CRUD works)
- Filters
- Bookmarks (partial - basic functionality works)
- Mutes
- Domain blocks
- Featured tags
- Hashtag following
- Scheduled statuses
- Status editing
- Translation
- Push notifications (stubbed)
- Streaming API
- Admin API
- Markers
- Reports
- Suggestions/recommendations
- Follow requests
- Conversations
- Announcements
- Trends (stubbed)
- Custom emojis (stubbed)

See TODO comments throughout this file for specific endpoints that need implementation.
*/

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aron23/lesser/cmd/api/handlers"
	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/config"
	"github.com/aron23/lesser/pkg/cost"
	"github.com/aron23/lesser/pkg/storage"
	storageDB "github.com/aron23/lesser/pkg/storage/dynamodb"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"go.uber.org/zap"
)

var (
	cfg            *config.Config
	store          storage.Storage
	logger         *zap.Logger
	handler        *handlers.Handler
	authMiddleware *auth.Middleware
)

func init() {
	cfg = config.Get()
	logger = common.Logger()

	var err error
	store, err = storageDB.New()
	if err != nil {
		logger.Fatal("failed to initialize storage", zap.Error(err))
	}

	// Initialize auth middleware
	authMiddleware, err = auth.GetMiddleware()
	if err != nil {
		logger.Fatal("failed to initialize auth middleware", zap.Error(err))
	}

	// Create handler with all dependencies
	handler = handlers.NewHandler(cfg, store, logger, authMiddleware)
}

func lambdaHandler(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Log POST /statuses requests for debugging
	if request.RequestContext.HTTP.Method == "POST" && strings.Contains(request.RequestContext.HTTP.Path, "statuses") {
		logger.Info("lambdaHandler received POST /statuses",
			zap.String("body", request.Body),
			zap.Bool("is_base64", request.IsBase64Encoded),
			zap.Any("headers", request.Headers),
			zap.Int("body_length", len(request.Body)),
			zap.Any("request_context", request.RequestContext),
			zap.Any("raw_path", request.RawPath),
			zap.Any("raw_query", request.RawQueryString))
		
		// Log the entire request struct for debugging
		requestJSON, _ := json.Marshal(request)
		logger.Info("Full request JSON", zap.String("request", string(requestJSON)))
		
		// Try base64 decode even if flag is false
		if request.Body != "" {
			decoded, err := base64.StdEncoding.DecodeString(request.Body)
			if err == nil {
				logger.Info("Base64 decode succeeded",
					zap.String("decoded", string(decoded)))
			} else {
				logger.Info("Base64 decode failed",
					zap.Error(err))
			}
		}
	}
	
	// Create chi router
	router := NewRouter(handler, *authMiddleware, logger)

	// Use the router-based handler with cost tracking
	routerHandler := LambdaHandlerWithRouter(router)
	return cost.WrapHandler(routerHandler, logger)(ctx, request)
}

func handleRequest(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// IMPORTANT: Mastodon API Version Notes
	// =====================================
	// We support both /api/v1 and /api/v2 endpoints.
	//
	// v2 endpoints (introduced in Mastodon 4.0.0+):
	//   - /api/v2/instance (GET) - Server information
	//   - /api/v2/search (GET) - Search with grouped results
	//   - /api/v2/suggestions (GET) - Follow suggestions
	//   - /api/v2/media (POST) - Async media upload
	//
	// v1 endpoints (most of the API):
	//   - /api/v1/accounts/* - All account endpoints
	//   - /api/v1/statuses/* - All status endpoints
	//   - /api/v1/timelines/* - All timeline endpoints
	//   - /api/v1/custom_emojis - Custom emoji list
	//   - /api/v1/instance/activity - Weekly activity
	//   - /api/v1/instance/peers - Connected domains
	//   - /api/v1/notifications - Notifications
	//   - /api/v1/bookmarks, /api/v1/favourites - User collections
	//   - /api/v1/lists/* - List management
	//   - /api/v1/filters/* - Content filters
	//   - ... and all other endpoints
	//
	// API Gateway strips the version prefix, so we receive paths like:
	// - "/instance" for both /api/v1/instance and /api/v2/instance
	// - "/timelines/public" for /api/v1/timelines/public

	// API Gateway v2 provides the clean path (with ApiMappingKey already stripped) in RequestContext.HTTP.Path
	path := request.RequestContext.HTTP.Path

	// Remove stage prefix if present (e.g., /lab)
	if request.RequestContext.Stage != "" && request.RequestContext.Stage != "$default" {
		stagePrefix := "/" + request.RequestContext.Stage
		if strings.HasPrefix(path, stagePrefix) {
			path = strings.TrimPrefix(path, stagePrefix)
		}
	}

	// Ensure path starts with /
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// Remove trailing slash for consistency (except for root path)
	if len(path) > 1 && strings.HasSuffix(path, "/") {
		path = strings.TrimSuffix(path, "/")
	}

	// Log request with all path information for debugging
	logger.Info("API request",
		zap.String("raw_path", request.RawPath),
		zap.String("http_path", request.RequestContext.HTTP.Path),
		zap.String("path", path),
		zap.String("method", request.RequestContext.HTTP.Method),
		zap.String("stage", request.RequestContext.Stage),
		zap.Any("headers", request.Headers))

	// Handle OPTIONS requests for CORS preflight
	if request.RequestContext.HTTP.Method == http.MethodOptions {
		return &events.APIGatewayV2HTTPResponse{
			StatusCode: http.StatusOK,
			Headers: map[string]string{
				"Access-Control-Allow-Origin":  "*",
				"Access-Control-Allow-Methods": "GET, POST, PUT, DELETE, PATCH, OPTIONS, HEAD",
				"Access-Control-Allow-Headers": "Content-Type, Authorization, X-Requested-With, Accept, Accept-Encoding, Accept-Language, Date, Digest, Host, Signature, User-Agent, X-Forwarded-For, X-Forwarded-Proto",
				"Access-Control-Max-Age":       "86400",
			},
		}, nil
	}

	// Route based on path and method
	method := request.RequestContext.HTTP.Method

	// ==================== APPS & OAUTH ====================
	// OAuth app registration
	if path == "/apps" && method == http.MethodPost {
		return handler.HandleAppRegistration(ctx, request)
	}
	// Verify app credentials
	if path == "/apps/verify_credentials" && method == http.MethodGet {
		return handler.HandleAppVerifyCredentials(ctx, request)
	}

	// OAuth authorization endpoint
	if path == "/oauth/authorize" && method == http.MethodGet {
		return handler.HandleOAuthAuthorize(ctx, request)
	}

	// OAuth token endpoint
	if path == "/oauth/token" && method == http.MethodPost {
		return handler.HandleOAuthToken(ctx, request)
	}

	// OAuth revoke endpoint
	if path == "/oauth/revoke" && method == http.MethodPost {
		return handler.HandleOAuthRevoke(ctx, request)
	}

	// External OAuth provider endpoints
	if strings.HasPrefix(path, "/oauth/") && len(strings.Split(path, "/")) >= 3 {
		parts := strings.Split(path, "/")
		provider := parts[2]

		// Provider authorization
		if len(parts) == 4 && parts[3] == "authorize" && method == http.MethodGet {
			return handler.HandleOAuthProviderAuthorize(ctx, request, provider)
		}

		// Provider callback
		if len(parts) == 4 && parts[3] == "callback" && method == http.MethodGet {
			return handler.HandleOAuthProviderCallback(ctx, request, provider)
		}

		// Link provider to existing account
		if len(parts) == 4 && parts[3] == "link" && method == http.MethodPost {
			return handler.HandleLinkOAuthProvider(ctx, request, provider)
		}

		// Unlink provider from account
		if len(parts) == 4 && parts[3] == "unlink" && method == http.MethodDelete {
			return handler.HandleUnlinkOAuthProvider(ctx, request, provider)
		}
	}

	// ==================== ACCOUNTS ====================
	// Account management
	if path == "/accounts" && method == http.MethodPost {
		return handler.HandleRegistration(ctx, request)
	}
	if path == "/accounts/verify_credentials" && method == http.MethodGet {
		return handler.HandleVerifyCredentials(ctx, request)
	}
	if path == "/accounts/update_credentials" && method == http.MethodPatch {
		return handler.HandleUpdateCredentials(ctx, request)
	}

	// Account recovery
	if path == "/auth/recovery/initiate" && method == http.MethodPost {
		return handler.HandleInitiateRecovery(ctx, request)
	}
	if path == "/auth/recovery/verify" && method == http.MethodGet {
		return handler.HandleVerifyRecoveryToken(ctx, request)
	}
	if path == "/auth/recovery/complete" && method == http.MethodPost {
		return handler.HandleCompleteRecovery(ctx, request)
	}
	if path == "/auth/recovery/options" && method == http.MethodGet {
		return handler.HandleAccountRecoveryOptions(ctx, request)
	}
	if path == "/auth/recovery/send-code" && method == http.MethodPost {
		return handler.HandleSendRecoveryCode(ctx, request)
	}

	// Account lookup
	if path == "/accounts/lookup" && method == http.MethodGet {
		return handler.HandleAccountLookup(ctx, request)
	}
	// Account relationships
	if path == "/accounts/relationships" && method == http.MethodGet {
		return handler.HandleGetRelationships(ctx, request)
	}
	// Account search
	if path == "/accounts/search" && method == http.MethodGet {
		return handler.HandleAccountSearch(ctx, request)
	}
	// Account search suggestions
	if path == "/accounts/search/suggestions" && method == http.MethodGet {
		return handler.HandleGetSearchSuggestions(ctx, request)
	}
	// Find familiar followers
	if path == "/accounts/familiar_followers" && method == http.MethodGet {
		return handler.HandleGetFamiliarFollowers(ctx, request)
	}

	// ==================== STATUSES ====================
	// Status management
	if path == "/statuses" && method == http.MethodPost {
		return handler.HandleCreateStatus(ctx, request)
	}
	// TODO: GET /api/v1/scheduled_statuses - View scheduled statuses
	// TODO: GET /api/v1/scheduled_statuses/:id - View single scheduled status
	// TODO: PUT /api/v1/scheduled_statuses/:id - Update scheduled status
	// TODO: DELETE /api/v1/scheduled_statuses/:id - Cancel scheduled status

	// Status interactions
	if strings.HasPrefix(path, "/statuses/") {
		parts := strings.Split(path, "/")
		if len(parts) >= 3 {
			statusID := parts[2]

			// Status actions
			if len(parts) == 4 {
				action := parts[3]
				switch action {
				case "favourite":
					if method == http.MethodPost {
						return handler.HandleFavourite(ctx, request, statusID)
					}
				case "unfavourite":
					if method == http.MethodPost {
						return handler.HandleUnfavourite(ctx, request, statusID)
					}
				case "reblog":
					if method == http.MethodPost {
						return handler.HandleReblog(ctx, request, statusID)
					}
				case "unreblog":
					if method == http.MethodPost {
						return handler.HandleUnreblog(ctx, request, statusID)
					}
				case "context":
					if method == http.MethodGet {
						return handler.HandleGetStatusContext(ctx, request, statusID)
					}
				case "bookmark":
					if method == http.MethodPost {
						return handler.HandleBookmark(ctx, request, statusID)
					}
				case "unbookmark":
					if method == http.MethodPost {
						return handler.HandleUnbookmark(ctx, request, statusID)
					}
				case "favourited_by":
					if method == http.MethodGet {
						return handler.HandleGetStatusFavouritedBy(ctx, request, statusID)
					}
				case "reblogged_by":
					if method == http.MethodGet {
						return handler.HandleGetStatusRebloggedBy(ctx, request, statusID)
					}
				case "mute":
					if method == http.MethodPost {
						return handler.HandleMuteConversation(ctx, request, statusID)
					}
				case "unmute":
					if method == http.MethodPost {
						return handler.HandleUnmuteConversation(ctx, request, statusID)
					}
				case "pin":
					if method == http.MethodPost {
						return handler.HandlePinStatus(ctx, request, statusID)
					}
				case "unpin":
					if method == http.MethodPost {
						return handler.HandleUnpinStatus(ctx, request, statusID)
					}
				case "source":
					if method == http.MethodGet {
						return handler.HandleGetStatusSource(ctx, request, statusID)
					}
				case "history":
					if method == http.MethodGet {
						return handler.HandleGetStatusHistory(ctx, request, statusID)
					}
				case "translate":
					if method == http.MethodPost {
						return handler.HandleTranslateStatus(ctx, request, statusID)
					}
				}
			} else if len(parts) == 3 {
				// Direct status operations
				switch method {
				case http.MethodGet:
					return handler.HandleGetStatus(ctx, request, statusID)
				case http.MethodDelete:
					return handler.HandleDeleteStatus(ctx, request, statusID)
				case http.MethodPut:
					return handler.HandleUpdateStatus(ctx, request, statusID)
				}
			}
		}
	}

	// ==================== ACCOUNT INTERACTIONS ====================
	// Account interactions
	if strings.HasPrefix(path, "/accounts/") {
		parts := strings.Split(path, "/")
		if len(parts) >= 3 {
			accountID := parts[2]

			if len(parts) == 4 {
				action := parts[3]
				switch action {
				case "follow":
					if method == http.MethodPost {
						return handler.HandleFollow(ctx, request, accountID)
					}
				case "unfollow":
					if method == http.MethodPost {
						return handler.HandleUnfollow(ctx, request, accountID)
					}
				case "block":
					if method == http.MethodPost {
						return handler.HandleBlock(ctx, request, accountID)
					}
				case "unblock":
					if method == http.MethodPost {
						return handler.HandleUnblock(ctx, request, accountID)
					}
				case "statuses":
					if method == http.MethodGet {
						return handler.HandleGetAccountStatuses(ctx, request, accountID)
					}
				case "followers":
					if method == http.MethodGet {
						return handler.HandleGetAccountFollowers(ctx, request, accountID)
					}
				case "following":
					if method == http.MethodGet {
						return handler.HandleGetAccountFollowing(ctx, request, accountID)
					}
				case "featured_tags":
					if method == http.MethodGet {
						return handler.HandleGetAccountFeaturedTags(ctx, request, accountID)
					}
				case "mute":
					if method == http.MethodPost {
						return handler.HandleMuteAccount(ctx, request, accountID)
					}
				case "unmute":
					if method == http.MethodPost {
						return handler.HandleUnmuteAccount(ctx, request, accountID)
					}
				case "lists":
					if method == http.MethodGet {
						return handler.HandleGetAccountLists(ctx, request, accountID)
					}
				case "pin":
					if method == http.MethodPost {
						return handler.HandlePinAccount(ctx, request, accountID)
					}
				case "unpin":
					if method == http.MethodPost {
						return handler.HandleUnpinAccount(ctx, request, accountID)
					}
				case "note":
					if method == http.MethodPost {
						return handler.HandleSetAccountNote(ctx, request, accountID)
					}
				case "remove_from_followers":
					if method == http.MethodPost {
						return handler.HandleRemoveFromFollowers(ctx, request, accountID)
					}
				}
			} else if len(parts) == 3 && method == http.MethodGet {
				return handler.HandleGetAccount(ctx, request, accountID)
			}
		}
	}

	// ==================== BLOCKS & MUTES ====================
	// Blocks list
	if path == "/blocks" && method == http.MethodGet {
		return handler.HandleGetBlocks(ctx, request)
	}
	// Mutes list
	if path == "/mutes" && method == http.MethodGet {
		return handler.HandleGetMutedAccounts(ctx, request)
	}
	// Domain blocks
	if path == "/domain_blocks" {
		switch method {
		case http.MethodGet:
			return handler.HandleGetDomainBlocks(ctx, request)
		case http.MethodPost:
			return handler.HandleCreateDomainBlock(ctx, request)
		case http.MethodDelete:
			return handler.HandleDeleteDomainBlock(ctx, request)
		}
	}

	// ==================== LISTS ====================
	// Lists
	if path == "/lists" {
		switch method {
		case http.MethodGet:
			return handler.HandleGetLists(ctx, request)
		case http.MethodPost:
			return handler.HandleCreateList(ctx, request)
		}
	}
	// List operations
	if strings.HasPrefix(path, "/lists/") {
		parts := strings.Split(path, "/")
		if len(parts) >= 3 {
			listID := parts[2]

			if len(parts) == 4 && parts[3] == "accounts" {
				// List accounts operations
				switch method {
				case http.MethodGet:
					return handler.HandleGetListAccounts(ctx, request, listID)
				case http.MethodPost:
					return handler.HandleAddAccountsToList(ctx, request, listID)
				case http.MethodDelete:
					return handler.HandleRemoveAccountsFromList(ctx, request, listID)
				}
			} else if len(parts) == 3 {
				// Single list operations
				switch method {
				case http.MethodGet:
					return handler.HandleGetList(ctx, request, listID)
				case http.MethodPut:
					return handler.HandleUpdateList(ctx, request, listID)
				case http.MethodDelete:
					return handler.HandleDeleteList(ctx, request, listID)
				}
			}
		}
	}

	// ==================== TIMELINES ====================
	// Timelines
	if path == "/timelines/home" && method == http.MethodGet {
		return handler.HandleHomeTimeline(ctx, request)
	}
	if path == "/timelines/public" && method == http.MethodGet {
		return handler.HandlePublicTimeline(ctx, request)
	}
	// Hashtag timeline
	if strings.HasPrefix(path, "/timelines/tag/") {
		parts := strings.Split(path, "/")
		if len(parts) >= 4 && method == http.MethodGet {
			hashtag := parts[3]
			return handler.HandleHashtagTimeline(ctx, request, hashtag)
		}
	}
	// List timeline
	if strings.HasPrefix(path, "/timelines/list/") {
		parts := strings.Split(path, "/")
		if len(parts) >= 4 && method == http.MethodGet {
			listID := parts[3]
			return handler.HandleListTimeline(ctx, request, listID)
		}
	}
	// Link timeline
	if path == "/timelines/link" && method == http.MethodGet {
		return handler.HandleGetLinkTimeline(ctx, request)
	}

	// ==================== CONVERSATIONS ====================
	// List conversations
	if path == "/conversations" && method == http.MethodGet {
		return handler.HandleGetConversations(ctx, request)
	}
	// Conversation operations
	if strings.HasPrefix(path, "/conversations/") {
		parts := strings.Split(path, "/")
		if len(parts) >= 3 {
			conversationID := parts[2]

			if len(parts) == 4 && parts[3] == "read" && method == http.MethodPost {
				// Mark conversation as read
				return handler.HandleMarkConversationRead(ctx, request, conversationID)
			} else if len(parts) == 3 && method == http.MethodDelete {
				// Delete conversation
				return handler.HandleDeleteConversation(ctx, request, conversationID)
			}
		}
	}

	// ==================== INSTANCE ====================
	// Instance info - determine version based on path
	if path == "/instance" && method == http.MethodGet {
		// Check if request is from v1 or v2 API based on request path or headers
		// For now, use v2 as default (most Mastodon clients use v2)
		return handler.HandleGetInstanceV2(ctx, request)
	}

	// Legacy v1 instance endpoint (if explicitly requested)
	if strings.HasPrefix(request.RawPath, "/api/v1/instance") && path == "/instance" && method == http.MethodGet {
		return handler.HandleGetInstanceV1(ctx, request)
	}

	// Instance activity
	if path == "/instance/activity" && method == http.MethodGet {
		return handler.HandleGetInstanceActivity(ctx, request)
	}

	// Instance peers
	if path == "/instance/peers" && method == http.MethodGet {
		return handler.HandleGetInstancePeers(ctx, request)
	}

	// Instance domain blocks
	if path == "/instance/domain_blocks" && method == http.MethodGet {
		return handler.HandleGetInstanceDomainBlocks(ctx, request)
	}

	// Instance privacy policy
	if path == "/instance/privacy_policy" && method == http.MethodGet {
		return handler.HandleGetInstancePrivacyPolicy(ctx, request)
	}

	// Instance terms of service
	if path == "/instance/terms_of_service" && method == http.MethodGet {
		return handler.HandleGetInstanceTermsOfService(ctx, request)
	}

	// Instance terms of service by date
	if strings.HasPrefix(path, "/instance/terms_of_service/") {
		parts := strings.Split(path, "/")
		if len(parts) == 4 && method == http.MethodGet {
			date := parts[3]
			return handler.HandleGetInstanceTermsOfServiceByDate(ctx, request, date)
		}
	}

	// Instance rules
	if path == "/instance/rules" && method == http.MethodGet {
		// Get rules from DynamoDB
		rules, err := store.GetInstanceRules(ctx)
		if err != nil {
			logger.Error("failed to get instance rules", zap.Error(err))
			return common.OK([]interface{}{}), nil
		}

		// Convert to API format
		apiRules := make([]interface{}, len(rules))
		for i, rule := range rules {
			apiRules[i] = map[string]interface{}{
				"id":   rule.ID,
				"text": rule.Text,
			}
		}

		return common.OK(apiRules), nil
	}

	// Instance extended description
	if path == "/instance/extended_description" && method == http.MethodGet {
		// Get extended description from DynamoDB
		content, updatedAt, err := store.GetExtendedDescription(ctx)
		if err != nil {
			logger.Error("failed to get extended description", zap.Error(err))
			// Return default on error
			return common.OK(map[string]interface{}{
				"updated_at": "2025-01-01T00:00:00Z",
				"content":    "<p>Welcome to Lesser ActivityPub Server</p>",
			}), nil
		}

		return common.OK(map[string]interface{}{
			"updated_at": updatedAt.Format(time.RFC3339),
			"content":    content,
		}), nil
	}

	// Instance translation languages
	if path == "/instance/translation_languages" && method == http.MethodGet {
		return handler.HandleGetTranslationLanguages(ctx, request)
	}

	// Instance costs
	if path == "/instance/costs" && method == http.MethodGet {
		return handler.HandleGetInstanceCosts(ctx, request)
	}

	// Instance metrics
	if path == "/instance/metrics" && method == http.MethodGet {
		return handler.HandleGetInstanceMetrics(ctx, request)
	}

	// Daily aggregates
	if path == "/instance/metrics/daily" && method == http.MethodGet {
		return handler.HandleGetDailyAggregates(ctx, request)
	}

	// Predictive analytics
	if path == "/instance/analytics" && method == http.MethodGet {
		return handler.HandleGetPredictiveAnalytics(ctx, request)
	}

	// Profile directory
	if path == "/directory" && method == http.MethodGet {
		return handler.HandleGetDirectory(ctx, request)
	}

	// Announcements
	if path == "/announcements" && method == http.MethodGet {
		return handler.HandleGetAnnouncements(ctx, request)
	}
	// Announcement actions
	if strings.HasPrefix(path, "/announcements/") {
		parts := strings.Split(path, "/")
		if len(parts) >= 4 {
			announcementID := parts[2]
			action := parts[3]

			switch action {
			case "dismiss":
				if method == http.MethodPost {
					return handler.HandleDismissAnnouncement(ctx, request, announcementID)
				}
			case "reactions":
				if len(parts) == 5 {
					reactionName := parts[4]
					switch method {
					case http.MethodPut:
						return handler.HandleAddAnnouncementReaction(ctx, request, announcementID, reactionName)
					case http.MethodDelete:
						return handler.HandleRemoveAnnouncementReaction(ctx, request, announcementID, reactionName)
					}
				}
			}
		}
	}

	// ==================== NOTIFICATIONS ====================
	// Notifications
	if path == "/notifications" && method == http.MethodGet {
		return handler.HandleGetNotifications(ctx, request)
	}
	if strings.HasPrefix(path, "/notifications/") {
		parts := strings.Split(path, "/")
		if len(parts) == 3 {
			notificationID := parts[2]
			if method == http.MethodGet {
				return handler.HandleGetNotification(ctx, request, notificationID)
			}
			if method == http.MethodPost && strings.HasSuffix(path, "/dismiss") {
				return handler.HandleDismissNotification(ctx, request, notificationID)
			}
		}
		if len(parts) == 3 && parts[2] == "clear" && method == http.MethodPost {
			return handler.HandleClearNotifications(ctx, request)
		}
	}

	// ==================== SCHEDULED STATUSES ====================
	// Scheduled statuses
	if path == "/scheduled_statuses" && method == http.MethodGet {
		return handler.HandleGetScheduledStatuses(ctx, request)
	}
	if strings.HasPrefix(path, "/scheduled_statuses/") {
		parts := strings.Split(path, "/")
		if len(parts) == 3 {
			scheduledID := parts[2]
			switch method {
			case http.MethodGet:
				return handler.HandleGetScheduledStatus(ctx, request, scheduledID)
			case http.MethodPut:
				return handler.HandleUpdateScheduledStatus(ctx, request, scheduledID)
			case http.MethodDelete:
				return handler.HandleDeleteScheduledStatus(ctx, request, scheduledID)
			}
		}
	}

	// ==================== PUSH NOTIFICATIONS ====================
	// Push subscription
	if path == "/push/subscription" {
		switch method {
		case http.MethodGet:
			return handler.HandleGetPushSubscription(ctx, request)
		case http.MethodPost:
			return handler.HandleCreatePushSubscription(ctx, request)
		case http.MethodPut:
			return handler.HandleUpdatePushSubscription(ctx, request)
		case http.MethodDelete:
			return handler.HandleDeletePushSubscription(ctx, request)
		}
	}

	// ==================== USER PREFERENCES ====================
	// Preferences
	if path == "/preferences" {
		switch method {
		case http.MethodGet:
			return handler.HandleGetPreferences(ctx, request)
		case http.MethodPatch:
			return handler.HandleUpdatePreferences(ctx, request)
		}
	}

	// ==================== CUSTOM EMOJIS ====================
	// Custom emojis
	if path == "/custom_emojis" && method == http.MethodGet {
		return handler.HandleGetCustomEmojis(ctx, request)
	}

	// ==================== SEARCH ====================
	// Search
	if path == "/search" && method == http.MethodGet {
		return handler.HandleSearch(ctx, request)
	}
	// Search v2 (same format as v1 in Lesser)
	if path == "/search/v2" && method == http.MethodGet {
		return handler.HandleSearchV2(ctx, request)
	}

	// ==================== MODERATION ====================
	// Moderation endpoints
	if path == "/moderation/flag" && method == http.MethodPost {
		return handler.HandleModerationFlag(ctx, request)
	}
	if path == "/moderation/queue" && method == http.MethodGet {
		return handler.HandleModerationQueue(ctx, request)
	}
	if path == "/moderation/review" && method == http.MethodPost {
		return handler.HandleModerationReview(ctx, request)
	}
	// Moderation history
	if strings.HasPrefix(path, "/moderation/history/") {
		parts := strings.Split(path, "/")
		if len(parts) == 3 && method == http.MethodGet {
			objectID := parts[2]
			return handler.HandleModerationHistory(ctx, request, objectID)
		}
	}
	// Moderation consensus
	if strings.HasPrefix(path, "/moderation/consensus/") {
		parts := strings.Split(path, "/")
		if len(parts) == 3 && method == http.MethodGet {
			eventID := parts[2]
			return handler.HandleGetConsensus(ctx, request, eventID)
		}
	}
	// Trust relationships
	if path == "/moderation/trust" {
		switch method {
		case http.MethodGet:
			return handler.HandleGetTrustRelationships(ctx, request)
		case http.MethodPut:
			return handler.HandleUpdateTrust(ctx, request)
		}
	}
	// Trust score
	if strings.HasPrefix(path, "/moderation/trust/") && strings.HasSuffix(path, "/score") {
		parts := strings.Split(path, "/")
		if len(parts) == 4 && method == http.MethodGet {
			actorID := parts[2]
			return handler.HandleGetTrustScore(ctx, request, actorID)
		}
	}

	// ==================== AI INTEGRATION ====================
	// AI Analysis endpoints
	if strings.HasPrefix(path, "/ai/analysis/") {
		parts := strings.Split(path, "/")
		if len(parts) == 3 && method == http.MethodGet {
			objectID := parts[2]
			return handler.HandleGetAIAnalysis(ctx, request, objectID)
		}
	}
	// Request AI analysis
	if path == "/ai/analyze" && method == http.MethodPost {
		return handler.HandleRequestAIAnalysis(ctx, request)
	}
	// AI statistics
	if path == "/ai/stats" && method == http.MethodGet {
		return handler.HandleGetAIStats(ctx, request)
	}
	// AI capabilities
	if path == "/ai/capabilities" && method == http.MethodGet {
		return handler.HandleGetAISummary(ctx, request)
	}

	// ==================== REPUTATION & VOUCHES ====================
	// Reputation endpoints
	if strings.HasPrefix(path, "/reputation/") {
		parts := strings.Split(path, "/")
		if len(parts) >= 3 {
			if parts[2] == "export" && method == http.MethodPost {
				return handler.HandleExportReputation(ctx, request)
			} else if parts[2] == "import" && method == http.MethodPost {
				return handler.HandleImportReputation(ctx, request)
			} else if parts[2] == "verify" && method == http.MethodPost {
				return handler.HandleVerifyReputation(ctx, request)
			} else if method == http.MethodGet {
				// GET /reputation/:actor_id
				actorID := parts[2]
				return handler.HandleGetReputation(ctx, request, actorID)
			}
		}
	}

	// Vouch endpoints
	if path == "/vouches" && method == http.MethodPost {
		return handler.HandleCreateVouch(ctx, request)
	}
	if strings.HasPrefix(path, "/vouches/") {
		parts := strings.Split(path, "/")
		if len(parts) >= 3 {
			if method == http.MethodGet {
				// GET /vouches/:actor_id
				actorID := parts[2]
				return handler.HandleGetVouches(ctx, request, actorID)
			} else if method == http.MethodDelete {
				// DELETE /vouches/:vouch_id
				vouchID := parts[2]
				return handler.HandleRevokeVouch(ctx, request, vouchID)
			}
		}
	}

	// ==================== COMMUNITY NOTES ====================
	// Create a note
	if path == "/notes" && method == http.MethodPost {
		return handler.HandleCreateNote(ctx, request)
	}
	// Get notes for an object
	if strings.HasPrefix(path, "/notes/") {
		parts := strings.Split(path, "/")
		if len(parts) >= 3 {
			if len(parts) == 4 && parts[3] == "vote" && method == http.MethodPost {
				// Vote on a note
				noteID := parts[2]
				return handler.HandleVoteNote(ctx, request, noteID)
			} else if len(parts) == 3 && method == http.MethodGet {
				// Get notes for object
				objectID := parts[2]
				return handler.HandleGetNotes(ctx, request, objectID)
			}
		}
	}
	// Get user's notes
	if strings.HasPrefix(path, "/accounts/") && strings.HasSuffix(path, "/notes") {
		parts := strings.Split(path, "/")
		if len(parts) == 4 && method == http.MethodGet {
			username := parts[2]
			return handler.HandleGetUserNotes(ctx, request, username)
		}
	}

	// ==================== DEBUG ENDPOINTS ====================
	// Debug endpoints require admin or debug scope
	if strings.HasPrefix(path, "/debug/") {
		// Federation trace
		if strings.HasPrefix(path, "/debug/federation/trace/") {
			parts := strings.Split(path, "/")
			if len(parts) == 5 && method == http.MethodGet {
				activityID := parts[4]
				return handler.HandleDebugFederationTrace(ctx, request, activityID)
			}
		}
		// Object inspection
		if strings.HasPrefix(path, "/debug/objects/") && !strings.HasSuffix(path, "/explain") {
			parts := strings.Split(path, "/")
			if len(parts) == 4 && method == http.MethodGet {
				objectID := parts[3]
				return handler.HandleDebugObject(ctx, request, objectID)
			}
		}
		// Activity replay
		if strings.HasPrefix(path, "/debug/replay/") {
			parts := strings.Split(path, "/")
			if len(parts) == 4 && method == http.MethodPost {
				activityID := parts[3]
				return handler.HandleDebugReplay(ctx, request, activityID)
			}
		}
		// Federation domain debug
		if strings.HasPrefix(path, "/debug/federation/domain/") {
			parts := strings.Split(path, "/")
			if len(parts) == 5 && method == http.MethodGet {
				domain := parts[4]
				return handler.HandleDebugFederationDomain(ctx, request, domain)
			}
		}
		// Object explanation with storage details
		if strings.HasPrefix(path, "/debug/objects/") && strings.HasSuffix(path, "/explain") {
			parts := strings.Split(path, "/")
			if len(parts) == 5 && method == http.MethodGet {
				objectID := parts[3]
				return handler.HandleDebugObjectExplain(ctx, request, objectID)
			}
		}
	}

	// ==================== FEATURED CONTENT ====================
	// Featured tags
	if path == "/featured_tags" && method == http.MethodGet {
		return handler.HandleGetFeaturedTags(ctx, request)
	}
	if path == "/featured_tags" && method == http.MethodPost {
		return handler.HandleCreateFeaturedTag(ctx, request)
	}
	if strings.HasPrefix(path, "/featured_tags/") {
		parts := strings.Split(path, "/")
		if len(parts) == 3 {
			if parts[2] == "suggestions" && method == http.MethodGet {
				return handler.HandleGetFeaturedTagSuggestions(ctx, request)
			} else if method == http.MethodDelete {
				featuredTagID := parts[2]
				return handler.HandleDeleteFeaturedTag(ctx, request, featuredTagID)
			}
		}
	}

	// ==================== TRENDS ====================
	// Trends
	if path == "/trends" && method == http.MethodGet {
		return handler.HandleGetTrends(ctx, request)
	}
	if path == "/trends/statuses" && method == http.MethodGet {
		return handler.HandleGetTrendingStatuses(ctx, request)
	}
	if path == "/trends/tags" && method == http.MethodGet {
		return handler.HandleGetTrendingTags(ctx, request)
	}
	if path == "/trends/links" && method == http.MethodGet {
		return handler.HandleGetTrendingLinks(ctx, request)
	}

	// ==================== MISSING ENDPOINTS ====================
	// Media endpoints
	if path == "/media" && method == http.MethodPost {
		return handler.HandleMediaUpload(ctx, request)
	}
	if strings.HasPrefix(path, "/media/") {
		parts := strings.Split(path, "/")
		if len(parts) == 3 {
			switch method {
			case http.MethodGet:
				return handler.HandleGetMedia(ctx, request)
			case http.MethodPut:
				return handler.HandleUpdateMedia(ctx, request)
			}
		}
	}

	// ==================== PHASE 6: MEDIA & IMPORT/EXPORT ====================
	// Media v2 endpoints (async upload)
	if path == "/media/v2" && method == http.MethodPost {
		return handler.HandleMediaUploadV2(ctx, request)
	}
	if strings.HasPrefix(path, "/media/v2/") {
		parts := strings.Split(path, "/")
		if len(parts) == 4 {
			// Media v2 GET is handled by HandleGetMedia which checks for async jobs
			if method == http.MethodGet {
				return handler.HandleGetMedia(ctx, request)
			}
		}
	}

	// Export endpoints
	if path == "/exports" {
		switch method {
		case http.MethodPost:
			return handler.HandleCreateExport(ctx, request)
		case http.MethodGet:
			return handler.HandleListExports(ctx, request)
		}
	}
	if strings.HasPrefix(path, "/exports/") {
		parts := strings.Split(path, "/")
		if len(parts) == 3 {
			if method == http.MethodGet {
				return handler.HandleGetExportStatus(ctx, request)
			}
		}
	}

	// Import endpoints
	if path == "/imports" {
		switch method {
		case http.MethodPost:
			return handler.HandleCreateImport(ctx, request)
		case http.MethodGet:
			return handler.HandleListImports(ctx, request)
		}
	}
	if strings.HasPrefix(path, "/imports/") {
		parts := strings.Split(path, "/")
		if len(parts) == 3 {
			if method == http.MethodGet {
				return handler.HandleGetImportStatus(ctx, request)
			}
		}
	}

	// oEmbed endpoint (not part of Mastodon API path)
	if path == "/oembed" && method == http.MethodGet {
		return handler.HandleOEmbed(ctx, request)
	}
	// Embed page for iframe (not part of Mastodon API)
	if strings.HasPrefix(path, "/embed/") {
		parts := strings.Split(path, "/")
		if len(parts) == 3 && method == http.MethodGet {
			statusID := parts[2]
			return handler.HandleEmbedPage(ctx, request, statusID)
		}
	}

	// ==================== POLLS ====================
	// Poll endpoints
	if strings.HasPrefix(path, "/polls/") {
		parts := strings.Split(path, "/")
		if len(parts) >= 3 {
			pollID := parts[2]

			if len(parts) == 4 && parts[3] == "votes" && method == http.MethodPost {
				// Vote on a poll
				return handler.HandleVoteOnPoll(ctx, request, pollID)
			} else if len(parts) == 3 && method == http.MethodGet {
				// View a poll
				return handler.HandleGetPoll(ctx, request, pollID)
			}
		}
	}

	// ==================== FILTERS (v2) ====================
	// Filters v2
	if path == "/filters" {
		switch method {
		case http.MethodGet:
			return handler.HandleGetFilters(ctx, request)
		case http.MethodPost:
			return handler.HandleCreateFilter(ctx, request)
		}
	}
	// Filter operations
	if strings.HasPrefix(path, "/filters/") {
		parts := strings.Split(path, "/")
		if len(parts) >= 3 {
			filterID := parts[2]

			if len(parts) == 4 {
				action := parts[3]
				switch action {
				case "keywords":
					switch method {
					case http.MethodGet:
						return handler.HandleGetFilterKeywords(ctx, request, filterID)
					case http.MethodPost:
						return handler.HandleAddFilterKeyword(ctx, request, filterID)
					}
				}
			} else if len(parts) == 3 {
				// Single filter operations
				switch method {
				case http.MethodGet:
					return handler.HandleGetFilter(ctx, request, filterID)
				case http.MethodPut:
					return handler.HandleUpdateFilter(ctx, request, filterID)
				case http.MethodDelete:
					return handler.HandleDeleteFilter(ctx, request, filterID)
				}
			}
		}
	}

	// Bookmarks
	if path == "/bookmarks" && method == http.MethodGet {
		return handler.HandleGetBookmarks(ctx, request)
	}
	// Favourites
	if path == "/favourites" && method == http.MethodGet {
		return handler.HandleGetFavourites(ctx, request)
	}

	// Follow requests
	if path == "/follow_requests" && method == http.MethodGet {
		return handler.HandleGetFollowRequests(ctx, request)
	}
	// Follow request operations
	if strings.HasPrefix(path, "/follow_requests/") {
		parts := strings.Split(path, "/")
		if len(parts) >= 3 {
			accountID := parts[2]

			if len(parts) == 4 {
				action := parts[3]
				switch action {
				case "authorize":
					if method == http.MethodPost {
						return handler.HandleAuthorizeFollowRequest(ctx, request, accountID)
					}
				case "reject":
					if method == http.MethodPost {
						return handler.HandleRejectFollowRequest(ctx, request, accountID)
					}
				}
			}
		}
	}

	// Endorsements
	if path == "/endorsements" && method == http.MethodGet {
		return handler.HandleGetEndorsements(ctx, request)
	}
	// Follow suggestions
	if path == "/suggestions" && method == http.MethodGet {
		return handler.HandleGetSuggestionsV1(ctx, request)
	}
	if path == "/suggestions/v2" && method == http.MethodGet {
		return handler.HandleGetSuggestionsV2(ctx, request)
	}
	// Remove suggestion
	if strings.HasPrefix(path, "/suggestions/") {
		parts := strings.Split(path, "/")
		if len(parts) == 3 && method == http.MethodDelete {
			accountID := parts[2]
			return handler.HandleRemoveSuggestion(ctx, request, accountID)
		}
	}

	// ==================== HASHTAGS ====================
	// Hashtag endpoints
	if strings.HasPrefix(path, "/tags/") {
		parts := strings.Split(path, "/")
		if len(parts) >= 3 {
			tagName := parts[2]

			if len(parts) == 3 && method == http.MethodGet {
				// GET /api/v1/tags/:id - View hashtag info
				return handler.HandleGetTag(ctx, request, tagName)
			} else if len(parts) == 4 {
				action := parts[3]
				switch action {
				case "follow":
					if method == http.MethodPost {
						// POST /api/v1/tags/:id/follow - Follow hashtag
						return handler.HandleFollowTag(ctx, request, tagName)
					}
				case "unfollow":
					if method == http.MethodPost {
						// POST /api/v1/tags/:id/unfollow - Unfollow hashtag
						return handler.HandleUnfollowTag(ctx, request, tagName)
					}
				}
			}
		}
	}
	// Followed tags
	if path == "/followed_tags" && method == http.MethodGet {
		return handler.HandleGetFollowedTags(ctx, request)
	}

	// Markers
	if path == "/markers" {
		switch method {
		case http.MethodGet:
			return handler.HandleGetMarkers(ctx, request)
		case http.MethodPost:
			return handler.HandleSaveMarkers(ctx, request)
		}
	}

	// Reports
	if path == "/reports" && method == http.MethodPost {
		return handler.HandleCreateReport(ctx, request)
	}

	// Streaming API endpoints (Mastodon-compatible)
	// Note: These are SSE endpoints that Mastodon clients expect
	if strings.HasPrefix(path, "/streaming/") && method == http.MethodGet {
		// Handle all streaming endpoints with the same handler
		// The handler will parse the specific stream type from the path
		return handler.HandleSSEStream(ctx, request)
	}
	
	// Legacy streaming endpoint
	if path == "/streaming/events" && method == http.MethodGet {
		return handler.HandleSSEStream(ctx, request)
	}

	// ==================== ADMIN API ENDPOINTS ====================
	// Admin endpoints require admin role
	if strings.HasPrefix(path, "/admin/") {
		adminPath := strings.TrimPrefix(path, "/admin/")

		// Admin accounts endpoints
		if adminPath == "accounts" && method == http.MethodGet {
			return handler.HandleAdminGetAccounts(ctx, request)
		}
		if strings.HasPrefix(adminPath, "accounts/") {
			parts := strings.Split(adminPath, "/")
			if len(parts) >= 2 {
				accountID := parts[1]

				if len(parts) == 2 && method == http.MethodGet {
					// GET /api/v1/admin/accounts/:id
					return handler.HandleAdminGetAccount(ctx, request, accountID)
				} else if len(parts) == 3 {
					action := parts[2]
					switch action {
					case "action":
						if method == http.MethodPost {
							// POST /api/v1/admin/accounts/:id/action
							return handler.HandleAdminAccountAction(ctx, request, accountID)
						}
					case "approve":
						if method == http.MethodPost {
							// POST /api/v1/admin/accounts/:id/approve
							return handler.HandleAdminApproveAccount(ctx, request, accountID)
						}
					case "reject":
						if method == http.MethodPost {
							// POST /api/v1/admin/accounts/:id/reject
							return handler.HandleAdminRejectAccount(ctx, request, accountID)
						}
					case "enable":
						if method == http.MethodPost {
							// POST /api/v1/admin/accounts/:id/enable
							return handler.HandleAdminEnableAccount(ctx, request, accountID)
						}
					case "unsilence":
						if method == http.MethodPost {
							// POST /api/v1/admin/accounts/:id/unsilence
							return handler.HandleAdminUnsilenceAccount(ctx, request, accountID)
						}
					case "unsuspend":
						if method == http.MethodPost {
							// POST /api/v1/admin/accounts/:id/unsuspend
							return handler.HandleAdminUnsuspendAccount(ctx, request, accountID)
						}
					case "unsensitive":
						if method == http.MethodPost {
							// POST /api/v1/admin/accounts/:id/unsensitive
							return handler.HandleAdminUnsensitiveAccount(ctx, request, accountID)
						}
					}
				}
			}
		}

		// Admin reports endpoints
		if adminPath == "reports" && method == http.MethodGet {
			return handler.HandleAdminGetReports(ctx, request)
		}
		if strings.HasPrefix(adminPath, "reports/") {
			parts := strings.Split(adminPath, "/")
			if len(parts) >= 2 {
				reportID := parts[1]

				if len(parts) == 2 && method == http.MethodGet {
					// GET /api/v1/admin/reports/:id
					return handler.HandleAdminGetReport(ctx, request, reportID)
				} else if len(parts) == 3 {
					action := parts[2]
					switch action {
					case "resolve":
						if method == http.MethodPost {
							// POST /api/v1/admin/reports/:id/resolve
							return handler.HandleAdminResolveReport(ctx, request, reportID)
						}
					case "reopen":
						if method == http.MethodPost {
							// POST /api/v1/admin/reports/:id/reopen
							return handler.HandleAdminReopenReport(ctx, request, reportID)
						}
					case "assign_to_self":
						if method == http.MethodPost {
							// POST /api/v1/admin/reports/:id/assign_to_self
							return handler.HandleAdminAssignReport(ctx, request, reportID)
						}
					case "unassign":
						if method == http.MethodPost {
							// POST /api/v1/admin/reports/:id/unassign
							return handler.HandleAdminUnassignReport(ctx, request, reportID)
						}
					}
				}
			}
		}

		// Admin moderation overview
		if adminPath == "moderation/overview" && method == http.MethodGet {
			return handler.HandleAdminModerationOverview(ctx, request)
		}

		// Admin moderation events
		if adminPath == "moderation/events" && method == http.MethodGet {
			return handler.HandleAdminGetModerationEvents(ctx, request)
		}
		if strings.HasPrefix(adminPath, "moderation/events/") && strings.HasSuffix(adminPath, "/override") {
			parts := strings.Split(adminPath, "/")
			if len(parts) == 4 && method == http.MethodPost {
				eventID := parts[2]
				return handler.HandleAdminOverrideModerationEvent(ctx, request, eventID)
			}
		}

		// Admin trust graph management
		if adminPath == "moderation/trust/graph" && method == http.MethodGet {
			return handler.HandleAdminGetTrustGraph(ctx, request)
		}
		if strings.HasPrefix(adminPath, "moderation/trust/") {
			parts := strings.Split(adminPath, "/")
			if len(parts) == 4 && method == http.MethodPut {
				fromActorID := parts[2]
				toActorID := parts[3]
				return handler.HandleAdminUpdateTrust(ctx, request, fromActorID, toActorID)
			}
		}

		// Admin reviewer management
		if adminPath == "moderation/reviewers" && method == http.MethodGet {
			return handler.HandleAdminGetReviewers(ctx, request)
		}
		if strings.HasPrefix(adminPath, "moderation/reviewers/") {
			parts := strings.Split(adminPath, "/")
			if len(parts) == 4 {
				userID := parts[2]
				action := parts[3]
				switch action {
				case "promote":
					if method == http.MethodPost {
						return handler.HandleAdminPromoteModerator(ctx, request, userID)
					}
				case "demote":
					if method == http.MethodPost {
						return handler.HandleAdminDemoteModerator(ctx, request, userID)
					}
				}
			}
		}

		// Admin domain blocks (federation management)
		if adminPath == "domain_blocks" && method == http.MethodGet {
			return handler.HandleGetAdminDomainBlocks(ctx, request)
		}
		if adminPath == "domain_blocks" && method == http.MethodPost {
			return handler.HandleCreateAdminDomainBlock(ctx, request)
		}
		if strings.HasPrefix(adminPath, "domain_blocks/") {
			parts := strings.Split(adminPath, "/")
			if len(parts) == 2 {
				switch method {
				case http.MethodGet:
					return handler.HandleGetAdminDomainBlock(ctx, request)
				case http.MethodPut:
					return handler.HandleUpdateAdminDomainBlock(ctx, request)
				case http.MethodDelete:
					return handler.HandleDeleteAdminDomainBlock(ctx, request)
				}
			}
		}

		// Admin domain allows (allowlist mode)
		if adminPath == "domain_allows" && method == http.MethodGet {
			return handler.HandleGetAdminDomainAllows(ctx, request)
		}
		if adminPath == "domain_allows" && method == http.MethodPost {
			return handler.HandleCreateAdminDomainAllow(ctx, request)
		}
		if strings.HasPrefix(adminPath, "domain_allows/") {
			parts := strings.Split(adminPath, "/")
			if len(parts) == 2 && method == http.MethodDelete {
				return handler.HandleDeleteAdminDomainAllow(ctx, request)
			}
		}

		// Admin federation insights
		if adminPath == "federation/statistics" && method == http.MethodGet {
			return handler.HandleGetFederationStatistics(ctx, request)
		}
		if adminPath == "federation/instances" && method == http.MethodGet {
			return handler.HandleGetFederationInstances(ctx, request)
		}
		if strings.HasPrefix(adminPath, "federation/instance/") {
			parts := strings.Split(adminPath, "/")
			if len(parts) == 3 && method == http.MethodGet {
				return handler.HandleGetFederationInstance(ctx, request)
			}
		}

		// Admin email domain blocks
		if adminPath == "email_domain_blocks" && method == http.MethodGet {
			return handler.HandleGetEmailDomainBlocks(ctx, request)
		}
		if adminPath == "email_domain_blocks" && method == http.MethodPost {
			return handler.HandleCreateEmailDomainBlock(ctx, request)
		}
		if strings.HasPrefix(adminPath, "email_domain_blocks/") {
			parts := strings.Split(adminPath, "/")
			if len(parts) == 2 && method == http.MethodDelete {
				return handler.HandleDeleteEmailDomainBlock(ctx, request)
			}
		}

		// TODO: More admin endpoints
		// - IP blocks
		// - Webhooks
		// - Instance configuration
		// - Analytics
	}

	// TODO: More admin API endpoints

	// Unknown endpoint
	return common.NotFound(fmt.Errorf("unknown API endpoint: %s %s", method, path)), nil
}

func main() {
	lambda.Start(lambdaHandler)
}
