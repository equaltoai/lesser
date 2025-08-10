package ratelimit

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// Config defines configuration for rate limiting
type Config struct {
	// Endpoint-specific limits
	EndpointLimits map[string]EndpointLimit `json:"endpoint_limits"`
	
	// Default limits for unspecified endpoints
	DefaultLimit int           `json:"default_limit"`
	DefaultWindow time.Duration `json:"default_window"`
	
	// Admin bypass
	AdminBypass bool `json:"admin_bypass"`
	
	// Cost tracking
	TrackCosts bool `json:"track_costs"`
}

// EndpointLimit defines rate limits for specific endpoints
type EndpointLimit struct {
	Limit  int           `json:"limit"`
	Window time.Duration `json:"window"`
}

// Middleware creates a comprehensive rate limiting middleware for Lift
// Note: High complexity (gocognit: 49) is due to comprehensive rate limit checking logic
// including IP-based, user-based, and endpoint-specific limits with detailed error handling
//nolint:gocognit // Complex rate limit logic requires checking multiple conditions
func Middleware(storage core.RepositoryStorage, config *Config) lift.Middleware {
	logger := common.Logger()
	
	// Set default config if not provided
	if config == nil {
		config = DefaultRateLimitConfig()
	}
	
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			start := time.Now()
			
			// Get user claims
			claims, hasClaims := ctx.Get("claims").(*auth.Claims)
			var userID string
			if hasClaims && claims != nil {
				userID = claims.Username
				
				// Admin bypass check
				if config.AdminBypass && isAdminUser(ctx, claims) {
					return executeWithHeaders(ctx, next, config.DefaultLimit, 0, start.Add(time.Hour))
				}
			}
			
			// For unauthenticated users, use IP-based rate limiting
			if userID == "" {
				userID = getClientIP(ctx)
				if userID == "" {
					userID = "anonymous"
				}
			}
			
			// Build endpoint pattern
			endpoint := buildEndpointPattern(ctx.Request.Method, ctx.Request.Path)
			
			// Get rate limit configuration for this endpoint
			limitConfig := getLimitConfig(endpoint, config)
			
			// Check if user is blocked
			blocked, blockedUntil, err := storage.RateLimit().IsUserBlocked(ctx.Request.Context(), userID)
			if err != nil {
				logger.Error("failed to check if user is blocked",
					zap.String("user_id", userID),
					zap.Error(err))
				// Continue on error to avoid blocking legitimate requests
			}
			
			if blocked {
				retryAfter := int(time.Until(blockedUntil).Seconds())
				ctx.Response.Header("Retry-After", strconv.Itoa(retryAfter))
				ctx.Response.Header("X-RateLimit-Reset-After", strconv.Itoa(retryAfter))
				
				return ctx.Status(429).JSON(map[string]interface{}{
					"error": "rate_limit_exceeded",
					"message": "Rate limit exceeded. You are currently blocked due to repeated violations.",
					"blocked_until": blockedUntil.Unix(),
					"retry_after": retryAfter,
				})
			}
			
			// Check rate limit
			err = storage.RateLimit().CheckAPIRateLimit(
				ctx.Request.Context(),
				userID,
				endpoint,
				limitConfig.Limit,
				limitConfig.Window,
			)
			
			// Get rate limit info for headers
			remaining, resetTime, _ := storage.RateLimit().GetAPIRateLimitInfo(
				ctx.Request.Context(),
				userID,
				endpoint,
				limitConfig.Limit,
				limitConfig.Window,
			)
			
			// Set rate limit headers on all responses
			ctx.Response.Header("X-RateLimit-Limit", strconv.Itoa(limitConfig.Limit))
			ctx.Response.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
			ctx.Response.Header("X-RateLimit-Reset", strconv.FormatInt(resetTime.Unix(), 10))
			ctx.Response.Header("X-RateLimit-Reset-After", strconv.Itoa(int(time.Until(resetTime).Seconds())))
			
			// Track cost if enabled
			if config.TrackCosts {
				tracker := ctx.Get("cost_tracker")
				if tracker != nil {
					// Track rate limiting cost (minimal DynamoDB read/write)
					if t, ok := tracker.(interface{ TrackDynamoDBRead() }); ok {
						t.TrackDynamoDBRead()
					}
					if err == nil {
						if t, ok := tracker.(interface{ TrackDynamoDBWrite() }); ok {
							t.TrackDynamoDBWrite()
						}
					}
				}
			}
			
			if err != nil {
				// Rate limit exceeded
				retryAfter := int(limitConfig.Window.Seconds())
				ctx.Response.Header("Retry-After", strconv.Itoa(retryAfter))
				
				logger.Warn("rate limit exceeded",
					zap.String("user_id", userID),
					zap.String("endpoint", endpoint),
					zap.Int("limit", limitConfig.Limit),
					zap.Duration("window", limitConfig.Window),
					zap.Error(err))
				
				return ctx.Status(429).JSON(map[string]interface{}{
					"error": "rate_limit_exceeded",
					"message": fmt.Sprintf("Rate limit exceeded for %s. Limit: %d requests per %v", endpoint, limitConfig.Limit, limitConfig.Window),
					"retry_after": retryAfter,
				})
			}
			
			// Execute the next handler
			return next.Handle(ctx)
		})
	}
}

// DefaultRateLimitConfig returns the default rate limiting configuration
func DefaultRateLimitConfig() *Config {
	return &Config{
		EndpointLimits: map[string]EndpointLimit{
			// Posting limits
			"POST:/api/v1/statuses":             {Limit: 30, Window: time.Hour},   // 30 posts per hour
			"DELETE:/api/v1/statuses/*":         {Limit: 30, Window: time.Hour},   // 30 deletes per hour
			"PUT:/api/v1/statuses/*":            {Limit: 30, Window: time.Hour},   // 30 edits per hour
			
			// Media upload limits
			"POST:/api/v1/media":                {Limit: 20, Window: time.Hour},   // 20 uploads per hour
			"POST:/api/v1/media_attachments":    {Limit: 20, Window: time.Hour},   // 20 attachments per hour
			
			// Interaction limits
			"POST:/api/v1/statuses/*/favourite": {Limit: 100, Window: time.Hour},  // 100 likes per hour
			"DELETE:/api/v1/statuses/*/favourite": {Limit: 100, Window: time.Hour}, // 100 unlikes per hour
			"POST:/api/v1/statuses/*/reblog":    {Limit: 60, Window: time.Hour},   // 60 reblogs per hour
			"DELETE:/api/v1/statuses/*/reblog":  {Limit: 60, Window: time.Hour},   // 60 unreblogs per hour
			
			// Follow limits  
			"POST:/api/v1/accounts/*/follow":    {Limit: 30, Window: time.Hour},   // 30 follows per hour
			"POST:/api/v1/accounts/*/unfollow":  {Limit: 30, Window: time.Hour},   // 30 unfollows per hour
			"POST:/api/v1/follow_requests/*/authorize": {Limit: 100, Window: time.Hour}, // 100 approvals per hour
			"POST:/api/v1/follow_requests/*/reject": {Limit: 100, Window: time.Hour},    // 100 rejections per hour
			
			// Account management
			"PATCH:/api/v1/accounts/update_credentials": {Limit: 10, Window: time.Hour}, // 10 profile updates per hour
			"POST:/api/v1/accounts":             {Limit: 5, Window: 24 * time.Hour}, // 5 account creations per day
			
			// Search limits
			"GET:/api/v1/search":                {Limit: 100, Window: 5 * time.Minute}, // 100 searches per 5 minutes
			"GET:/api/v2/search":                {Limit: 100, Window: 5 * time.Minute}, // 100 searches per 5 minutes
			
			// Timeline limits (higher because they're reads)
			"GET:/api/v1/timelines/*":           {Limit: 300, Window: 5 * time.Minute}, // 300 timeline requests per 5 min
			
			// Notifications
			"GET:/api/v1/notifications":         {Limit: 100, Window: 5 * time.Minute}, // 100 notification checks per 5 min
			
			// Lists
			"POST:/api/v1/lists":                {Limit: 10, Window: time.Hour},   // 10 list creations per hour
			"PUT:/api/v1/lists/*":               {Limit: 20, Window: time.Hour},   // 20 list updates per hour
			"POST:/api/v1/lists/*/accounts":     {Limit: 100, Window: time.Hour},  // 100 list member additions per hour
			
			// Reports/Moderation
			"POST:/api/v1/reports":              {Limit: 10, Window: time.Hour},   // 10 reports per hour
		},
		DefaultLimit:  300,          // 300 requests per 5 minutes for unspecified endpoints
		DefaultWindow: 5 * time.Minute,
		AdminBypass:   true,         // Admins bypass rate limits
		TrackCosts:    true,         // Track rate limiting costs
	}
}

// buildEndpointPattern creates a pattern from method and path for matching
func buildEndpointPattern(method, path string) string {
	return fmt.Sprintf("%s:%s", method, path)
}

// getLimitConfig returns the appropriate limit configuration for an endpoint
func getLimitConfig(endpoint string, config *Config) EndpointLimit {
	// Direct match
	if limit, exists := config.EndpointLimits[endpoint]; exists {
		return limit
	}
	
	// Wildcard matching
	for pattern, limit := range config.EndpointLimits {
		if matchesWildcard(endpoint, pattern) {
			return limit
		}
	}
	
	// Default limit
	return EndpointLimit{
		Limit:  config.DefaultLimit,
		Window: config.DefaultWindow,
	}
}

// matchesWildcard checks if endpoint matches a wildcard pattern
func matchesWildcard(endpoint, pattern string) bool {
	if !strings.Contains(pattern, "*") {
		return endpoint == pattern
	}
	
	// Split by the wildcard
	parts := strings.Split(pattern, "*")
	if len(parts) != 2 {
		return false
	}
	
	prefix, suffix := parts[0], parts[1]
	return strings.HasPrefix(endpoint, prefix) && strings.HasSuffix(endpoint, suffix)
}

// getClientIP extracts client IP from the request
func getClientIP(ctx *lift.Context) string {
	// Try X-Forwarded-For first (API Gateway)
	if ip := ctx.Header("X-Forwarded-For"); ip != "" {
		// Take the first IP if multiple
		if idx := strings.Index(ip, ","); idx > 0 {
			return strings.TrimSpace(ip[:idx])
		}
		return ip
	}
	
	// Try X-Real-IP
	if ip := ctx.Header("X-Real-IP"); ip != "" {
		return ip
	}
	
	// Fallback to remote addr (though this may not be reliable in Lambda)
	return "unknown"
}

// isAdminUser checks if the user has admin privileges
func isAdminUser(_ *lift.Context, _ *auth.Claims) bool {
	// This would need to be implemented based on your auth system
	// For now, return false to be safe
	// You could check claims.Roles, query user permissions, etc.
	
	// Example implementation:
	// return claims != nil && contains(claims.Roles, "admin")
	
	return false // Placeholder - implement based on your auth system
}

// executeWithHeaders executes the next handler with rate limit headers set
func executeWithHeaders(ctx *lift.Context, next lift.Handler, limit, remaining int, resetTime time.Time) error {
	ctx.Response.Header("X-RateLimit-Limit", strconv.Itoa(limit))
	ctx.Response.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
	ctx.Response.Header("X-RateLimit-Reset", strconv.FormatInt(resetTime.Unix(), 10))
	ctx.Response.Header("X-RateLimit-Reset-After", strconv.Itoa(int(time.Until(resetTime).Seconds())))
	
	return next.Handle(ctx)
}

// FederationRateLimitMiddleware creates rate limiting middleware specifically for federation endpoints
func FederationRateLimitMiddleware(storage core.RepositoryStorage) lift.Middleware {
	logger := common.Logger()
	
	// Federation-specific limits
	federationLimits := map[string]EndpointLimit{
		"POST:/inbox":     {Limit: 60, Window: time.Minute},    // 60 activities per minute
		"POST:/users/*/inbox": {Limit: 60, Window: time.Minute}, // 60 personal inbox activities per minute
		"GET:/.well-known/webfinger": {Limit: 100, Window: time.Minute}, // 100 webfinger per minute
		"GET:/users/*":    {Limit: 100, Window: time.Minute},   // 100 actor lookups per minute
		"GET:/objects/*":  {Limit: 100, Window: time.Minute},   // 100 object lookups per minute
	}
	
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Extract domain from request
			domain := extractFederationDomain(ctx)
			if domain == "" {
				// Not a federation request, skip rate limiting
				return next.Handle(ctx)
			}
			
			// Check if domain is blocked
			blocked, blockedUntil, err := storage.RateLimit().IsDomainBlocked(ctx.Request.Context(), domain)
			if err != nil {
				logger.Error("failed to check if domain is blocked",
					zap.String("domain", domain),
					zap.Error(err))
			}
			
			if blocked {
				retryAfter := int(time.Until(blockedUntil).Seconds())
				ctx.Response.Header("Retry-After", strconv.Itoa(retryAfter))
				
				logger.Warn("federation domain is blocked",
					zap.String("domain", domain),
					zap.Time("blocked_until", blockedUntil))
				
				return ctx.Status(429).JSON(map[string]interface{}{
					"error": "federation_rate_limit_exceeded",
					"message": fmt.Sprintf("Domain %s is temporarily blocked due to rate limit violations", domain),
					"blocked_until": blockedUntil.Unix(),
					"retry_after": retryAfter,
				})
			}
			
			// Build endpoint pattern
			endpoint := buildEndpointPattern(ctx.Request.Method, ctx.Request.Path)
			
			// Get limit for this federation endpoint
			var limitConfig EndpointLimit
			var found bool
			for pattern, limit := range federationLimits {
				if matchesWildcard(endpoint, pattern) {
					limitConfig = limit
					found = true
					break
				}
			}
			
			if !found {
				// Default federation limit
				limitConfig = EndpointLimit{Limit: 30, Window: time.Minute}
			}
			
			// Check federation rate limit
			err = storage.RateLimit().CheckFederationRateLimit(
				ctx.Request.Context(),
				domain,
				endpoint,
				limitConfig.Limit,
				limitConfig.Window,
			)
			
			// Get rate limit info for headers
			remaining, resetTime, _ := storage.RateLimit().GetFederationRateLimitInfo(
				ctx.Request.Context(),
				domain,
				endpoint,
				limitConfig.Limit,
				limitConfig.Window,
			)
			
			// Set rate limit headers
			ctx.Response.Header("X-RateLimit-Limit", strconv.Itoa(limitConfig.Limit))
			ctx.Response.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
			ctx.Response.Header("X-RateLimit-Reset", strconv.FormatInt(resetTime.Unix(), 10))
			ctx.Response.Header("X-RateLimit-Reset-After", strconv.Itoa(int(time.Until(resetTime).Seconds())))
			
			if err != nil {
				retryAfter := int(limitConfig.Window.Seconds())
				ctx.Response.Header("Retry-After", strconv.Itoa(retryAfter))
				
				logger.Warn("federation rate limit exceeded",
					zap.String("domain", domain),
					zap.String("endpoint", endpoint),
					zap.Int("limit", limitConfig.Limit),
					zap.Duration("window", limitConfig.Window),
					zap.Error(err))
				
				return ctx.Status(429).JSON(map[string]interface{}{
					"error": "federation_rate_limit_exceeded",
					"message": fmt.Sprintf("Federation rate limit exceeded for domain %s. Limit: %d requests per %v", domain, limitConfig.Limit, limitConfig.Window),
					"retry_after": retryAfter,
				})
			}
			
			return next.Handle(ctx)
		})
	}
}

// extractFederationDomain extracts the originating domain from federation requests
func extractFederationDomain(ctx *lift.Context) string {
	// Try to extract from ActivityPub signature first
	if signature := ctx.Header("Signature"); signature != "" {
		// Parse keyId from signature to get domain
		if keyID := parseKeyIDFromSignature(signature); keyID != "" {
			return extractDomainFromKeyID(keyID)
		}
	}
	
	// Try to extract from User-Agent or other headers
	// Some ActivityPub implementations include domain in User-Agent
	// This is a simplified extraction - in practice you'd want more robust parsing
	_ = ctx.Header("User-Agent")
	
	// Try X-Forwarded-For or similar headers if available
	if forwardedFor := ctx.Header("X-Forwarded-For"); forwardedFor != "" {
		// Extract domain from IP (this would require reverse DNS or IP-to-domain mapping)
		// For now, return the IP as identifier
		if idx := strings.Index(forwardedFor, ","); idx > 0 {
			return strings.TrimSpace(forwardedFor[:idx])
		}
		return forwardedFor
	}
	
	return ""
}

// parseKeyIDFromSignature extracts keyId from ActivityPub signature header
func parseKeyIDFromSignature(signature string) string {
	// ActivityPub signature format: keyId="...",algorithm="...",headers="...",signature="..."
	parts := strings.Split(signature, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, `keyId="`) && strings.HasSuffix(part, `"`) {
			return strings.Trim(part[7:], `"`)
		}
	}
	return ""
}

// extractDomainFromKeyID extracts domain from ActivityPub keyId URL
func extractDomainFromKeyID(keyID string) string {
	// KeyId format: https://domain.com/users/username#main-key
	if strings.HasPrefix(keyID, "https://") {
		parts := strings.Split(keyID[8:], "/")
		if len(parts) > 0 {
			return parts[0]
		}
	}
	return ""
}