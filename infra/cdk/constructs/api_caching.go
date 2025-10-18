package constructs

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2/awsapigatewayv2"
	"github.com/aws/jsii-runtime-go"
)

// CachingConfig defines cache configuration for API Gateway
type CachingConfig struct {
	Environment        string
	CachingEnabled     bool
	CacheTTLSeconds    int
	CacheClusterSize   string // 0.5, 1.6, 6.1, 13.5, 28.4, 58.2, 118, 237 GB
	CacheKeyParameters []string
}

// GetCachingConfig returns environment-specific caching configuration
func GetCachingConfig(environment string) CachingConfig {
	switch environment {
	case "production":
		return CachingConfig{
			Environment:      environment,
			CachingEnabled:   true,
			CacheTTLSeconds:  300,   // 5 minutes
			CacheClusterSize: "1.6", // 1.6 GB for production
			CacheKeyParameters: []string{
				"authorization",
				"accept",
				"accept-language",
			},
		}
	case "staging":
		return CachingConfig{
			Environment:      environment,
			CachingEnabled:   true,
			CacheTTLSeconds:  120,   // 2 minutes
			CacheClusterSize: "0.5", // 0.5 GB for staging
			CacheKeyParameters: []string{
				"authorization",
				"accept",
			},
		}
	default: // development
		return CachingConfig{
			Environment:        environment,
			CachingEnabled:     false, // No caching in development
			CacheTTLSeconds:    0,
			CacheClusterSize:   "",
			CacheKeyParameters: []string{},
		}
	}
}

// CacheableRoute defines which routes should have caching enabled
type CacheableRoute struct {
	Path            string
	Method          string
	CacheTTLSeconds int
	CacheKeyParams  []string
	RequireAuth     bool
}

// GetCacheableRoutes returns the list of routes that should be cached
func GetCacheableRoutes() []CacheableRoute {
	return []CacheableRoute{
		// Public timeline - can be cached for all users
		{
			Path:            "/api/v1/timelines/public",
			Method:          "GET",
			CacheTTLSeconds: 60, // 1 minute
			CacheKeyParams:  []string{"local", "only_media", "max_id", "since_id", "min_id", "limit"},
			RequireAuth:     false,
		},
		// Instance information - rarely changes
		{
			Path:            "/api/v1/instance",
			Method:          "GET",
			CacheTTLSeconds: 3600, // 1 hour
			CacheKeyParams:  []string{},
			RequireAuth:     false,
		},
		{
			Path:            "/api/v2/instance",
			Method:          "GET",
			CacheTTLSeconds: 3600, // 1 hour
			CacheKeyParams:  []string{},
			RequireAuth:     false,
		},
		// Custom emojis - rarely change
		{
			Path:            "/api/v1/custom_emojis",
			Method:          "GET",
			CacheTTLSeconds: 3600, // 1 hour
			CacheKeyParams:  []string{},
			RequireAuth:     false,
		},
		// Trends - can be cached briefly
		{
			Path:            "/api/v1/trends/tags",
			Method:          "GET",
			CacheTTLSeconds: 300, // 5 minutes
			CacheKeyParams:  []string{"limit", "offset"},
			RequireAuth:     false,
		},
		{
			Path:            "/api/v1/trends/statuses",
			Method:          "GET",
			CacheTTLSeconds: 300, // 5 minutes
			CacheKeyParams:  []string{"limit", "offset"},
			RequireAuth:     false,
		},
		{
			Path:            "/api/v1/trends/links",
			Method:          "GET",
			CacheTTLSeconds: 300, // 5 minutes
			CacheKeyParams:  []string{"limit", "offset"},
			RequireAuth:     false,
		},
		// Directory listings
		{
			Path:            "/api/v1/directory",
			Method:          "GET",
			CacheTTLSeconds: 600, // 10 minutes
			CacheKeyParams:  []string{"offset", "limit", "order", "local"},
			RequireAuth:     false,
		},
		// Nodeinfo - rarely changes
		{
			Path:            "/.well-known/nodeinfo",
			Method:          "GET",
			CacheTTLSeconds: 3600, // 1 hour
			CacheKeyParams:  []string{},
			RequireAuth:     false,
		},
		{
			Path:            "/nodeinfo/2.0",
			Method:          "GET",
			CacheTTLSeconds: 3600, // 1 hour
			CacheKeyParams:  []string{},
			RequireAuth:     false,
		},
		// Public account information
		{
			Path:            "/api/v1/accounts/{id}",
			Method:          "GET",
			CacheTTLSeconds: 300, // 5 minutes
			CacheKeyParams:  []string{},
			RequireAuth:     false,
		},
		// Public statuses
		{
			Path:            "/api/v1/statuses/{id}",
			Method:          "GET",
			CacheTTLSeconds: 300, // 5 minutes
			CacheKeyParams:  []string{},
			RequireAuth:     false,
		},
		// Context (replies) - can be cached briefly
		{
			Path:            "/api/v1/statuses/{id}/context",
			Method:          "GET",
			CacheTTLSeconds: 60, // 1 minute
			CacheKeyParams:  []string{},
			RequireAuth:     false,
		},
		// Search suggestions
		{
			Path:            "/api/v2/search",
			Method:          "GET",
			CacheTTLSeconds: 300, // 5 minutes
			CacheKeyParams:  []string{"q", "type", "resolve", "following", "account_id", "exclude_unreviewed", "limit", "offset"},
			RequireAuth:     false,
		},
		// WebFinger - can be cached
		{
			Path:            "/.well-known/webfinger",
			Method:          "GET",
			CacheTTLSeconds: 600, // 10 minutes
			CacheKeyParams:  []string{"resource"},
			RequireAuth:     false,
		},
		// Hashtag timeline
		{
			Path:            "/api/v1/timelines/tag/{hashtag}",
			Method:          "GET",
			CacheTTLSeconds: 120, // 2 minutes
			CacheKeyParams:  []string{"local", "only_media", "max_id", "since_id", "min_id", "limit"},
			RequireAuth:     false,
		},
		// Lists of public accounts
		{
			Path:            "/api/v1/accounts/{id}/followers",
			Method:          "GET",
			CacheTTLSeconds: 300, // 5 minutes
			CacheKeyParams:  []string{"max_id", "since_id", "limit"},
			RequireAuth:     false,
		},
		{
			Path:            "/api/v1/accounts/{id}/following",
			Method:          "GET",
			CacheTTLSeconds: 300, // 5 minutes
			CacheKeyParams:  []string{"max_id", "since_id", "limit"},
			RequireAuth:     false,
		},
	}
}

// NonCacheableRoutes returns patterns for routes that should never be cached
func GetNonCacheableRoutes() []string {
	return []string{
		// Authentication endpoints
		"/oauth/*",
		"/auth/*",

		// User-specific timelines
		"/api/v1/timelines/home",
		"/api/v1/timelines/list/*",
		"/api/v1/timelines/direct",

		// Notifications
		"/api/v1/notifications",
		"/api/v1/notifications/*",
		"/api/v1/push/*",

		// Account management
		"/api/v1/accounts/verify_credentials",
		"/api/v1/accounts/update_credentials",
		"/api/v1/accounts/relationships",

		// User actions
		"/api/v1/statuses/*/favourite",
		"/api/v1/statuses/*/unfavourite",
		"/api/v1/statuses/*/reblog",
		"/api/v1/statuses/*/unreblog",
		"/api/v1/statuses/*/bookmark",
		"/api/v1/statuses/*/unbookmark",
		"/api/v1/statuses/*/mute",
		"/api/v1/statuses/*/unmute",
		"/api/v1/statuses/*/pin",
		"/api/v1/statuses/*/unpin",

		// Follow/unfollow
		"/api/v1/accounts/*/follow",
		"/api/v1/accounts/*/unfollow",
		"/api/v1/accounts/*/block",
		"/api/v1/accounts/*/unblock",
		"/api/v1/accounts/*/mute",
		"/api/v1/accounts/*/unmute",

		// Posting
		"/api/v1/statuses",
		"/api/v1/media",
		"/api/v1/media/*",

		// Moderation
		"/api/v1/admin/*",
		"/api/v1/reports",
		"/api/v1/reports/*",

		// Streaming
		"/api/v1/streaming/*",

		// GraphQL (dynamic queries)
		"/api/graphql",

		// ActivityPub inboxes/outboxes (must be fresh)
		"/users/*/inbox",
		"/users/*/outbox",

		// Health checks (should always be fresh)
		"/health",
	}
}

// CacheInvalidationRule defines when to invalidate cache
type CacheInvalidationRule struct {
	TriggerEvent  string   // e.g., "status.created", "account.updated"
	AffectedPaths []string // Paths to invalidate
}

// GetCacheInvalidationRules returns rules for cache invalidation
func GetCacheInvalidationRules() []CacheInvalidationRule {
	return []CacheInvalidationRule{
		{
			TriggerEvent: "status.created",
			AffectedPaths: []string{
				"/api/v1/timelines/public",
				"/api/v1/timelines/tag/*",
				"/api/v1/trends/statuses",
			},
		},
		{
			TriggerEvent: "status.deleted",
			AffectedPaths: []string{
				"/api/v1/statuses/{id}",
				"/api/v1/statuses/{id}/context",
				"/api/v1/timelines/public",
			},
		},
		{
			TriggerEvent: "account.updated",
			AffectedPaths: []string{
				"/api/v1/accounts/{id}",
				"/api/v1/directory",
			},
		},
		{
			TriggerEvent: "instance.updated",
			AffectedPaths: []string{
				"/api/v1/instance",
				"/api/v2/instance",
				"/.well-known/nodeinfo",
				"/nodeinfo/2.0",
			},
		},
		{
			TriggerEvent: "emoji.changed",
			AffectedPaths: []string{
				"/api/v1/custom_emojis",
			},
		},
	}
}

// ApplyCachingToHttpApi applies caching configuration to the HTTP API
func ApplyCachingToHttpApi(api awsapigatewayv2.HttpApi, config CachingConfig) {
	if !config.CachingEnabled {
		return
	}

	// Note: HTTP APIs (v2) don't support caching directly like REST APIs
	// We need to implement caching at the CloudFront level or use REST API
	// This function would be used if we switch to REST API or add CloudFront

	// For now, we'll add response headers to enable client-side caching
	stage := api.DefaultStage()
	if stage != nil {
		// Add stage variables for cache control
		stage.Node().AddMetadata(jsii.String("CacheConfig"), map[string]interface{}{
			"enabled":          config.CachingEnabled,
			"ttl":              config.CacheTTLSeconds,
			"cacheClusterSize": config.CacheClusterSize,
		}, nil)
	}
}

// CreateCachePolicy creates cache policies for CloudFront distribution
func CreateCachePolicy(environment string) map[string]interface{} {
	config := GetCachingConfig(environment)

	return map[string]interface{}{
		"defaultTTL": config.CacheTTLSeconds,
		"maxTTL":     config.CacheTTLSeconds * 2,
		"minTTL":     0,
		"parametersInCacheKeyAndForwardedToOrigin": map[string]interface{}{
			"enableAcceptEncodingGzip":   true,
			"enableAcceptEncodingBrotli": true,
			"queryStringsConfig": map[string]interface{}{
				"queryStringBehavior": "whitelist",
				"queryStrings":        config.CacheKeyParameters,
			},
			"headersConfig": map[string]interface{}{
				"headerBehavior": "whitelist",
				"headers": []string{
					"Authorization",
					"Accept",
					"Accept-Language",
					"CloudFront-Viewer-Country",
				},
			},
			"cookiesConfig": map[string]interface{}{
				"cookieBehavior": "none",
			},
		},
	}
}

// GetCacheHeaders returns appropriate cache headers for a response
func GetCacheHeaders(path string, method string, isAuthenticated bool) map[string]string {
	headers := make(map[string]string)

	// Check if route is cacheable
	cacheable := false
	var ttl int

	for _, route := range GetCacheableRoutes() {
		if route.Path == path && route.Method == method {
			if !route.RequireAuth || !isAuthenticated {
				cacheable = true
				ttl = route.CacheTTLSeconds
				break
			}
		}
	}

	if cacheable {
		// Public caching for non-authenticated content
		if !isAuthenticated {
			headers["Cache-Control"] = fmt.Sprintf("public, max-age=%d", ttl)
			headers["CDN-Cache-Control"] = fmt.Sprintf("public, max-age=%d", ttl)
		} else {
			// Private caching for authenticated content
			headers["Cache-Control"] = fmt.Sprintf("private, max-age=%d", ttl)
			headers["CDN-Cache-Control"] = "no-cache"
		}

		// Add ETag support
		headers["Vary"] = "Accept-Encoding, Accept, Authorization"

		// Stale-while-revalidate for better performance
		if ttl > 60 {
			headers["Cache-Control"] += fmt.Sprintf(", stale-while-revalidate=%d", ttl/2)
		}
	} else {
		// No caching for non-cacheable routes
		headers["Cache-Control"] = "no-cache, no-store, must-revalidate"
		headers["Pragma"] = "no-cache"
		headers["Expires"] = "0"
	}

	return headers
}

// CreateResponseHeadersPolicy creates a response headers policy for security and caching
func CreateResponseHeadersPolicy(environment string) map[string]interface{} {
	return map[string]interface{}{
		"securityHeadersConfig": map[string]interface{}{
			"contentTypeOptions": map[string]interface{}{
				"override": true,
			},
			"frameOptions": map[string]interface{}{
				"frameOption": "DENY",
				"override":    true,
			},
			"referrerPolicy": map[string]interface{}{
				"referrerPolicy": "strict-origin-when-cross-origin",
				"override":       true,
			},
			"xssProtection": map[string]interface{}{
				"modeBlock":  true,
				"protection": true,
				"override":   true,
			},
			"strictTransportSecurity": map[string]interface{}{
				"accessControlMaxAgeSec": 63072000, // 2 years
				"includeSubdomains":      true,
				"override":               true,
			},
			"contentSecurityPolicy": map[string]interface{}{
				"contentSecurityPolicy": "default-src 'self'; img-src 'self' data: https:; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'",
				"override":              true,
			},
		},
		"customHeadersConfig": map[string]interface{}{
			"items": []map[string]interface{}{
				{
					"header":   "X-Environment",
					"value":    environment,
					"override": false,
				},
				{
					"header":   "X-Cache-Status",
					"value":    "HIT",
					"override": false,
				},
			},
		},
		"corsConfig": map[string]interface{}{
			"accessControlAllowCredentials": false,
			"accessControlAllowHeaders": map[string]interface{}{
				"items": []string{
					"Content-Type",
					"Authorization",
					"Accept",
					"X-Request-ID",
				},
			},
			"accessControlAllowMethods": map[string]interface{}{
				"items": []string{
					"GET",
					"POST",
					"PUT",
					"DELETE",
					"PATCH",
					"OPTIONS",
				},
			},
			"accessControlAllowOrigins": map[string]interface{}{
				"items": []string{"*"},
			},
			"accessControlMaxAgeSec": 86400, // 24 hours
			"originOverride":         false,
		},
	}
}
