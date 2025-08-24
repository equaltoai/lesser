package middleware

import (
	"context"
	"strings"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
)

// SearchAnalytics represents search analytics data
type SearchAnalytics struct {
	UserID    string
	Query     string
	Type      string
	Timestamp time.Time
	Endpoint  string
}

// SearchPrivacyConfig holds configuration for search privacy middleware
type SearchPrivacyConfig struct {
	// Repos provides access to storage repositories for relationship checks
	Repos interface {
		RecordSearchAnalytics(ctx context.Context, analytics *SearchAnalytics) error
		CheckRateLimit(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
	}

	// OAuthService for JWT token validation
	OAuthService *auth.OAuthService

	// Domain for building actor IDs
	Domain string

	// Logger for debugging and monitoring
	Logger *zap.Logger

	// RequireAuth determines if search endpoints require authentication
	RequireAuth bool

	// EnableAnalytics determines if search analytics should be recorded
	EnableAnalytics bool
}

// SearchPrivacyMiddleware provides privacy enforcement for search endpoints
func SearchPrivacyMiddleware(config SearchPrivacyConfig) func(next lift.HandlerFunc) lift.HandlerFunc {
	return func(next lift.HandlerFunc) lift.HandlerFunc {
		return func(ctx *lift.Context) error {
			// Only apply to search endpoints
			if !isSearchEndpoint(ctx.Request.URL().Path) {
				return next(ctx)
			}

			userID, err := handleSearchAuthentication(ctx, config)
			if err != nil {
				return err
			}

			setUserContext(ctx, userID)
			recordAnalyticsIfEnabled(ctx, userID, config)
			applyPrivacyFilters(ctx, userID)

			return next(ctx)
		}
	}
}

// handleSearchAuthentication handles authentication logic for search endpoints
func handleSearchAuthentication(ctx *lift.Context, config SearchPrivacyConfig) (string, error) {
	searchType := ctx.Query("type")
	isStatusSearch := isStatusSearchEndpoint(ctx.Request.URL().Path, searchType)

	// Extract and validate authentication
	userID, err := extractAndValidateAuth(ctx, config)
	if err != nil {
		if config.RequireAuth || isStatusSearch {
			return "", ctx.Status(401).JSON(map[string]string{"error": "authentication required for search"})
		}
		// Allow unauthenticated access for basic account search
		userID = ""
	}

	// Status searches always require authentication
	if isStatusSearch {
		if err := common.ValidateRequiredParam("userID", userID); err != nil {
			return "", ctx.Status(401).JSON(map[string]string{"error": "authentication required for status search"})
		}
	}

	return userID, nil
}

// setUserContext sets user context variables for downstream handlers
func setUserContext(ctx *lift.Context, userID string) {
	if userID != "" {
		ctx.Set("user_id", userID)
		ctx.Set("authenticated", true)
	}
}

// recordAnalyticsIfEnabled records search analytics if enabled and user is authenticated
func recordAnalyticsIfEnabled(ctx *lift.Context, userID string, config SearchPrivacyConfig) {
	if config.EnableAnalytics && userID != "" {
		go recordSearchAnalytics(ctx, userID, config)
	}
}

// applyPrivacyFilters applies appropriate privacy filters based on search endpoint type
func applyPrivacyFilters(ctx *lift.Context, userID string) {
	path := ctx.Request.URL().Path
	searchType := ctx.Query("type")

	if isStatusSearchEndpoint(path, searchType) {
		applyStatusSearchFilters(ctx, userID)
	} else if isAccountSearchEndpoint(path, searchType) {
		applyAccountSearchFilters(ctx, userID)
	} else if isHashtagSearchEndpoint(path, searchType) {
		applyHashtagSearchFilters(ctx, userID)
	}
}

// applyStatusSearchFilters applies privacy filters for status searches
func applyStatusSearchFilters(ctx *lift.Context, userID string) {
	ctx.Set("filter_private", true)
	ctx.Set("filter_followers_only", true)
	ctx.Set("requesting_user", userID)
}

// applyAccountSearchFilters applies privacy filters for account searches
func applyAccountSearchFilters(ctx *lift.Context, userID string) {
	if err := common.ValidateRequiredParam("userID", userID); err != nil {
		ctx.Set("public_search", true)
		ctx.Set("limit_results", 20) // Limit results for unauthenticated users
	}
}

// applyHashtagSearchFilters applies privacy filters for hashtag searches
func applyHashtagSearchFilters(ctx *lift.Context, userID string) {
	ctx.Set("public_search", true)

	// Filter NSFW content for unauthenticated users
	if err := common.ValidateRequiredParam("userID", userID); err != nil {
		ctx.Set("filter_nsfw", true)
	}
}

// extractAndValidateAuth extracts and validates authentication from the request
func extractAndValidateAuth(ctx *lift.Context, config SearchPrivacyConfig) (string, error) {
	// Extract bearer token
	headers := ctx.Request.Header()
	authHeader := ""
	if authHeaders, ok := headers["Authorization"]; ok && len(authHeaders) > 0 {
		authHeader = authHeaders
	}
	if err := common.ValidateRequiredParam("authHeader", authHeader); err != nil {
		return "", nil
	}

	// Extract token from Bearer header
	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		config.Logger.Debug("invalid authorization header format", zap.Error(err))
		return "", err
	}

	// Validate JWT token
	if config.OAuthService == nil {
		config.Logger.Error("OAuthService not configured for search privacy middleware")
		return "", auth.ErrInvalidToken
	}

	claims, err := config.OAuthService.ValidateAccessToken(token)
	if err != nil {
		config.Logger.Debug("invalid token for search", zap.Error(err))
		return "", err
	}

	// Verify that the token has read scope (required for search)
	if !claims.HasScope(auth.ScopeRead) {
		config.Logger.Debug("insufficient scope for search",
			zap.String("username", claims.Username),
			zap.Strings("scopes", claims.Scopes))
		return "", auth.ErrInvalidScope
	}

	// Set additional context for downstream handlers
	ctx.Set("jwt_claims", claims)
	ctx.Set("client_id", claims.ClientID)
	ctx.Set("token_scopes", claims.Scopes)

	return claims.Username, nil
}

// isSearchEndpoint checks if the current request is a search endpoint
func isSearchEndpoint(path string) bool {
	searchPaths := []string{
		"/api/v1/search",
		"/api/v2/search",
		"/api/v1/accounts/search",
		"/api/v1/statuses/search",
		"/api/v1/tags/search",
	}

	for _, searchPath := range searchPaths {
		if strings.HasPrefix(path, searchPath) {
			return true
		}
	}
	return false
}

// isStatusSearchEndpoint checks if this is specifically a status search
func isStatusSearchEndpoint(path, searchType string) bool {
	return strings.Contains(path, "/statuses/search") ||
		(strings.Contains(path, "/search") && searchType == "statuses")
}

// isAccountSearchEndpoint checks if this is specifically an account search
func isAccountSearchEndpoint(path, searchType string) bool {
	return strings.Contains(path, "/accounts/search") ||
		(strings.Contains(path, "/search") && searchType == "accounts")
}

// isHashtagSearchEndpoint checks if this is specifically a hashtag search
func isHashtagSearchEndpoint(path, searchType string) bool {
	return strings.Contains(path, "/tags/search") ||
		(strings.Contains(path, "/search") && searchType == "hashtags")
}

// recordSearchAnalytics records search analytics asynchronously
func recordSearchAnalytics(ctx *lift.Context, userID string, config SearchPrivacyConfig) {
	// Extract search parameters
	query := ctx.Query("q")
	searchType := ctx.Query("type")
	if err := common.ValidateRequiredParam("searchType", searchType); err != nil {
		searchType = detectSearchType(ctx.Request.URL().Path)
	}

	// Record to analytics
	analytics := &SearchAnalytics{
		UserID:    userID,
		Query:     query,
		Type:      searchType,
		Timestamp: time.Now(),
		Endpoint:  ctx.Request.URL().Path,
	}

	// Store analytics (non-blocking)
	if err := config.Repos.RecordSearchAnalytics(ctx.Request.Context(), analytics); err != nil {
		config.Logger.Warn("failed to record search analytics",
			zap.Error(err),
			zap.String("user_id", userID),
			zap.String("query", query))
	}
}

// detectSearchType detects the search type from the path
func detectSearchType(path string) string {
	if strings.Contains(path, "/statuses/") {
		return "statuses"
	}
	if strings.Contains(path, "/accounts/") {
		return "accounts"
	}
	if strings.Contains(path, "/tags/") {
		return "hashtags"
	}
	return "all"
}

// NewSearchAnalyticsMiddleware creates middleware for recording search analytics
func NewSearchAnalyticsMiddleware(repos interface {
	RecordSearchAnalytics(ctx context.Context, analytics *SearchAnalytics) error
}, logger *zap.Logger) func(next lift.HandlerFunc) lift.HandlerFunc {
	return func(next lift.HandlerFunc) lift.HandlerFunc {
		return func(ctx *lift.Context) error {
			// Only track search endpoints
			path := ctx.Request.URL().Path
			if !strings.Contains(path, "search") {
				return next(ctx)
			}

			// Get user ID if authenticated
			userID, _ := ctx.Get("user_id").(string)

			// Record search query
			query := ctx.Query("q")
			if query != "" && userID != "" {
				go func() {
					analytics := &SearchAnalytics{
						UserID:    userID,
						Query:     query,
						Type:      ctx.Query("type"),
						Timestamp: time.Now(),
						Endpoint:  path,
					}

					if err := repos.RecordSearchAnalytics(ctx.Request.Context(), analytics); err != nil {
						logger.Debug("failed to record search analytics", zap.Error(err))
					}
				}()
			}

			return next(ctx)
		}
	}
}

// NewSearchRateLimitMiddleware creates middleware for rate limiting search requests
func NewSearchRateLimitMiddleware(repos interface {
	CheckRateLimit(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}, logger *zap.Logger, requestsPerMinute int) func(next lift.HandlerFunc) lift.HandlerFunc {
	return func(next lift.HandlerFunc) lift.HandlerFunc {
		return func(ctx *lift.Context) error {
			// Only apply to search endpoints
			if !strings.Contains(ctx.Request.URL().Path, "search") {
				return next(ctx)
			}

			// Get user ID or IP for rate limiting
			userID, _ := ctx.Get("user_id").(string)
			if err := common.ValidateRequiredParam("userID", userID); err != nil {
				userID = ctx.Request.RemoteAddr()
			}

			// Check rate limit
			key := "search:" + userID
			allowed, err := repos.CheckRateLimit(ctx.Request.Context(), key, requestsPerMinute, time.Minute)
			if err != nil {
				logger.Error("failed to check rate limit", zap.Error(err))
				// Allow request on error
				return next(ctx)
			}

			if !allowed {
				return ctx.Status(429).JSON(map[string]string{
					"error": "too many search requests",
				})
			}

			return next(ctx)
		}
	}
}
