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
- Media uploads
- Lists management
- Filters
- Polls
- Bookmarks
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
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aron23/lesser/cmd/api/handlers"
	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/config"
	"github.com/aron23/lesser/pkg/storage"
	storageDB "github.com/aron23/lesser/pkg/storage/dynamodb"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"go.uber.org/zap"
)

var (
	cfg     *config.Config
	store   storage.Storage
	logger  *zap.Logger
	handler *handlers.Handler
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
	authMiddleware, err := auth.GetMiddleware()
	if err != nil {
		logger.Fatal("failed to initialize auth middleware", zap.Error(err))
	}

	// Create handler with all dependencies
	handler = handlers.NewHandler(cfg, store, logger, authMiddleware)
}

func lambdaHandler(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Get the path, removing the stage prefix if present
	path := request.RawPath
	rawPath := request.RawPath // Keep original for version detection

	if request.RequestContext.Stage != "" && strings.HasPrefix(path, "/"+request.RequestContext.Stage) {
		path = strings.TrimPrefix(path, "/"+request.RequestContext.Stage)
	}

	// Detect API version from the raw path
	isV2 := strings.Contains(rawPath, "/api/v2/") || strings.HasPrefix(rawPath, "/api/v2/")

	// Log request
	logger.Info("API request",
		zap.String("raw_path", request.RawPath),
		zap.String("path", path),
		zap.String("method", request.RequestContext.HTTP.Method),
		zap.Bool("is_v2", isV2),
		zap.Any("headers", request.Headers))

	// Handle OPTIONS requests for CORS preflight
	if request.RequestContext.HTTP.Method == http.MethodOptions {
		return &events.APIGatewayV2HTTPResponse{
			StatusCode: http.StatusOK,
			Headers: map[string]string{
				"Access-Control-Allow-Origin":      "*",
				"Access-Control-Allow-Methods":     "GET, POST, PUT, DELETE, PATCH, OPTIONS",
				"Access-Control-Allow-Headers":     "Content-Type, Authorization, X-Requested-With",
				"Access-Control-Max-Age":           "86400",
				"Access-Control-Allow-Credentials": "true",
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
	// TODO: GET /api/v1/apps/verify_credentials - Verify app credentials

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
	// TODO: GET /api/v1/accounts/relationships - Check relationships with multiple accounts
	// TODO: GET /api/v1/accounts/familiar_followers - Find familiar followers
	// TODO: GET /api/v1/accounts/search - Search for accounts
	// TODO: GET /api/v1/accounts/lookup - Lookup account by username@domain

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
					// TODO: GET /api/v1/statuses/:id/favourited_by - Who favourited this status
					// TODO: GET /api/v1/statuses/:id/reblogged_by - Who boosted this status
					// TODO: POST /api/v1/statuses/:id/mute - Mute conversation
					// TODO: POST /api/v1/statuses/:id/unmute - Unmute conversation
					// TODO: POST /api/v1/statuses/:id/pin - Pin status to profile
					// TODO: POST /api/v1/statuses/:id/unpin - Unpin status from profile
					// TODO: GET /api/v1/statuses/:id/source - View status source
					// TODO: GET /api/v1/statuses/:id/history - View edit history
					// TODO: PUT /api/v1/statuses/:id - Edit status
					// TODO: POST /api/v1/statuses/:id/translate - Translate status
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
					// TODO: GET /api/v1/accounts/:id/followers - Get account's followers
					// TODO: GET /api/v1/accounts/:id/following - Get account's following
					// TODO: GET /api/v1/accounts/:id/featured_tags - Get account's featured tags
					// TODO: POST /api/v1/accounts/:id/mute - Mute account
					// TODO: POST /api/v1/accounts/:id/unmute - Unmute account
					// TODO: POST /api/v1/accounts/:id/pin - Pin account to profile
					// TODO: POST /api/v1/accounts/:id/unpin - Unpin account from profile
					// TODO: POST /api/v1/accounts/:id/note - Set private note on account
					// TODO: POST /api/v1/accounts/:id/remove_from_followers - Remove follower
					// TODO: GET /api/v1/accounts/:id/lists - Lists containing this account
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
	// TODO: GET /api/v1/mutes - View muted accounts
	// TODO: GET /api/v1/domain_blocks - View blocked domains
	// TODO: POST /api/v1/domain_blocks - Block a domain
	// TODO: DELETE /api/v1/domain_blocks - Unblock a domain

	// ==================== LISTS ====================
	// Lists
	if path == "/lists" && method == http.MethodGet {
		// TODO: Implement list management
		return common.OK([]interface{}{}), nil
	}
	// TODO: GET /api/v1/lists/:id - View a single list
	// TODO: POST /api/v1/lists - Create a list
	// TODO: PUT /api/v1/lists/:id - Update a list
	// TODO: DELETE /api/v1/lists/:id - Delete a list
	// TODO: GET /api/v1/lists/:id/accounts - View accounts in list
	// TODO: POST /api/v1/lists/:id/accounts - Add accounts to list
	// TODO: DELETE /api/v1/lists/:id/accounts - Remove accounts from list

	// ==================== TIMELINES ====================
	// Timelines
	if path == "/timelines/home" && method == http.MethodGet {
		return handler.HandleHomeTimeline(ctx, request)
	}
	if path == "/timelines/public" && method == http.MethodGet {
		return handler.HandlePublicTimeline(ctx, request)
	}
	// TODO: GET /api/v1/timelines/tag/:hashtag - Hashtag timeline
	// TODO: GET /api/v1/timelines/list/:list_id - List timeline
	// TODO: GET /api/v1/conversations - View conversations

	// ==================== INSTANCE ====================
	// Instance info
	if path == "/instance" && method == http.MethodGet {
		if isV2 {
			return handler.HandleGetInstanceV2(ctx, request)
		}
		return handler.HandleGetInstance(ctx, request)
	}

	// Instance activity
	if path == "/instance/activity" && method == http.MethodGet {
		// TODO: Implement actual activity statistics
		return common.OK([]interface{}{}), nil
	}

	// Instance peers
	if path == "/instance/peers" && method == http.MethodGet {
		// TODO: Implement peer discovery
		return common.OK([]string{}), nil
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

	// Profile directory
	if path == "/directory" && method == http.MethodGet {
		// TODO: Implement profile directory
		return common.OK([]interface{}{}), nil
	}

	// Announcements
	if path == "/announcements" && method == http.MethodGet {
		// TODO: Implement announcements
		return common.OK([]interface{}{}), nil
	}
	// TODO: POST /api/v1/announcements/:id/dismiss - Dismiss announcement
	// TODO: PUT /api/v1/announcements/:id/reactions/:name - Add reaction
	// TODO: DELETE /api/v1/announcements/:id/reactions/:name - Remove reaction

	// ==================== NOTIFICATIONS ====================
	// Notifications
	if path == "/notifications" && method == http.MethodGet {
		return handler.HandleGetNotifications(ctx, request)
	}
	// TODO: GET /api/v1/notifications/:id - View single notification
	// TODO: POST /api/v1/notifications/clear - Clear all notifications
	// TODO: POST /api/v1/notifications/:id/dismiss - Dismiss single notification

	// ==================== PUSH NOTIFICATIONS ====================
	// Push subscription
	if path == "/push/subscription" {
		switch method {
		case http.MethodGet:
			// TODO: Implement push subscription storage
			return common.OK(map[string]interface{}{
				"id":       "",
				"endpoint": "",
				"alerts": map[string]bool{
					"follow":    false,
					"favourite": false,
					"reblog":    false,
					"mention":   false,
					"poll":      false,
				},
				"server_key": "",
			}), nil
		case http.MethodPost, http.MethodPut:
			// TODO: Implement push subscription storage
			return common.OK(map[string]interface{}{
				"id":       "1",
				"endpoint": request.Headers["endpoint"],
				"alerts": map[string]bool{
					"follow":    true,
					"favourite": true,
					"reblog":    true,
					"mention":   true,
					"poll":      true,
				},
				"server_key": "dummy_server_key",
			}), nil
		case http.MethodDelete:
			return common.NoContent(), nil
		}
	}

	// ==================== USER PREFERENCES ====================
	// Preferences
	if path == "/preferences" && method == http.MethodGet {
		// TODO: Store user preferences
		return common.OK(map[string]interface{}{
			"posting:default:visibility": "public",
			"posting:default:sensitive":  false,
			"posting:default:language":   "en",
			"reading:expand:media":       "default",
			"reading:expand:spoilers":    false,
		}), nil
	}

	// ==================== CUSTOM EMOJIS ====================
	// Custom emojis
	if path == "/custom_emojis" && method == http.MethodGet {
		// TODO: Implement custom emoji support
		return common.OK([]interface{}{}), nil
	}

	// ==================== SEARCH ====================
	// Search
	if path == "/search" && method == http.MethodGet {
		return handler.HandleSearch(ctx, request)
	}
	// TODO: Implement v2 search with better pagination

	// ==================== FEATURED CONTENT ====================
	// Featured tags
	if path == "/featured_tags" && method == http.MethodGet {
		// TODO: Implement featured tags
		return common.OK([]interface{}{}), nil
	}
	// TODO: POST /api/v1/featured_tags - Feature a tag
	// TODO: DELETE /api/v1/featured_tags/:id - Unfeature a tag
	// TODO: GET /api/v1/featured_tags/suggestions - Get suggested tags

	// ==================== TRENDS ====================
	// Trends
	if path == "/trends" && method == http.MethodGet {
		// TODO: Implement trending algorithm
		return common.OK([]interface{}{}), nil
	}
	if path == "/trends/statuses" && method == http.MethodGet {
		// TODO: Implement trending statuses
		return common.OK([]interface{}{}), nil
	}
	if path == "/trends/tags" && method == http.MethodGet {
		// TODO: Implement trending tags
		return common.OK([]interface{}{}), nil
	}
	if path == "/trends/links" && method == http.MethodGet {
		// TODO: Implement trending links
		return common.OK([]interface{}{}), nil
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

	// TODO: GET /api/v1/polls/:id - View a poll
	// TODO: POST /api/v1/polls/:id/votes - Vote on a poll

	// TODO: GET /api/v1/filters - View filters
	// TODO: GET /api/v1/filters/:id - View single filter
	// TODO: POST /api/v1/filters - Create filter
	// TODO: PUT /api/v1/filters/:id - Update filter
	// TODO: DELETE /api/v1/filters/:id - Delete filter
	// TODO: GET /api/v1/filters/:filter_id/keywords - View filter keywords
	// TODO: POST /api/v1/filters/:filter_id/keywords - Add filter keyword
	// TODO: GET /api/v1/filters/:filter_id/keywords/:id - View filter keyword
	// TODO: PUT /api/v1/filters/:filter_id/keywords/:id - Update filter keyword
	// TODO: DELETE /api/v1/filters/:filter_id/keywords/:id - Delete filter keyword
	// TODO: GET /api/v1/filters/:filter_id/statuses - View filtered statuses
	// TODO: POST /api/v1/filters/:filter_id/statuses - Add filtered status
	// TODO: DELETE /api/v1/filters/:filter_id/statuses/:id - Remove filtered status

	// Bookmarks
	if path == "/bookmarks" && method == http.MethodGet {
		return handler.HandleGetBookmarks(ctx, request)
	}
	// TODO: GET /api/v1/favourites - View favourited statuses

	// TODO: GET /api/v1/follow_requests - View follow requests
	// TODO: POST /api/v1/follow_requests/:account_id/authorize - Accept follow
	// TODO: POST /api/v1/follow_requests/:account_id/reject - Reject follow

	// TODO: GET /api/v1/endorsements - View endorsed accounts
	// TODO: GET /api/v1/suggestions - Get follow suggestions
	// TODO: DELETE /api/v1/suggestions/:account_id - Remove suggestion

	// TODO: GET /api/v1/tags/:id - View hashtag info
	// TODO: POST /api/v1/tags/:id/follow - Follow hashtag
	// TODO: POST /api/v1/tags/:id/unfollow - Unfollow hashtag

	// TODO: GET /api/v1/markers - Get timeline position markers
	// TODO: POST /api/v1/markers - Save timeline position

	// TODO: POST /api/v1/reports - File a report

	// TODO: Streaming API endpoints (WebSocket/SSE)
	// TODO: Admin API endpoints

	// Unknown endpoint
	return common.NotFound(fmt.Errorf("unknown API endpoint: %s %s", method, path)), nil
}

func main() {
	lambda.Start(lambdaHandler)
}
