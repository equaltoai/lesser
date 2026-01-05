package ratelimit

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
	liftMiddleware "github.com/pay-theory/lift/pkg/middleware"
	"go.uber.org/zap"
)

// endpointRateLimit defines rate limit configuration for a specific endpoint (internal use only)
type endpointRateLimit struct {
	limit  int
	window time.Duration
}

// endpointRateLimits maps METHOD:PATH to rate limits - only abuse-prone operations are listed
// Unlisted endpoints are NOT rate limited (like Autheory's pattern)
var endpointRateLimits = map[string]endpointRateLimit{
	// OAuth - prevent token grinding
	"POST:/oauth/token":    {limit: 10, window: time.Minute},
	"GET:/oauth/authorize": {limit: 20, window: 5 * time.Minute},

	// Account updates - prevent spam
	"PATCH:/api/v1/accounts/update_credentials": {limit: 10, window: time.Hour},

	// Export/Import - expensive operations
	"POST:/exports": {limit: 5, window: 24 * time.Hour},
	"POST:/imports": {limit: 5, window: 24 * time.Hour},

	// Community notes - prevent abuse
	"POST:/notes":        {limit: 20, window: time.Hour},
	"POST:/notes/*/vote": {limit: 100, window: time.Hour},

	// Media - prevent storage abuse
	"POST:/media": {limit: 20, window: time.Hour},

	// Search - prevent scraping
	"GET:/api/v1/accounts/search":  {limit: 30, window: 5 * time.Minute},
	"POST:/api/v1/search/statuses": {limit: 30, window: 5 * time.Minute},

	// Quote posts - prevent spam
	"POST:/api/v1/statuses/*/quote": {limit: 30, window: time.Hour},
}

// rateLimiterCache caches created rate limiters to avoid recreating them per request
var (
	rateLimiterCache = make(map[string]lift.Middleware)
	cacheMutex       sync.RWMutex
)

// LiftRateLimitMiddleware creates endpoint-specific rate limiting using the Limited library
// This follows the pattern from the lift documentation for DynamoDB-backed rate limiting
func LiftRateLimitMiddleware(logger *zap.Logger) lift.Middleware {
	// Get AWS region from environment
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = os.Getenv("AWS_DEFAULT_REGION")
	}
	if region == "" {
		region = "us-east-1" // fallback
	}

	// Get table name from environment (Limited uses LIMITED_TABLE_NAME or RATE_LIMIT_TABLE_NAME)
	tableName := os.Getenv("RATE_LIMIT_TABLE_NAME")
	if tableName == "" {
		tableName = os.Getenv("LIMITED_TABLE_NAME")
	}
	if tableName == "" {
		tableName = "rate-limits" // default from Limited library
	}

	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Build endpoint key: METHOD:PATH
			endpointKey := ctx.Request.Method + ":" + ctx.Request.Path

			// Normalize (remove query params, trailing slash)
			endpointKey = normalizeEndpointKey(endpointKey)

			// Look up rate limit config
			limitCfg, shouldLimit := lookupEndpointLimit(endpointKey)

			// Log for debugging
			logger.Debug("rate limit check",
				zap.String("endpoint", endpointKey),
				zap.Bool("should_limit", shouldLimit))

			if !shouldLimit {
				// This endpoint is not rate limited - skip entirely
				logger.Debug("endpoint not rate limited, skipping",
					zap.String("endpoint", endpointKey))
				return next.Handle(ctx)
			}

			// Get or create cached rate limiter for this endpoint configuration
			cacheKey := buildCacheKey(limitCfg)
			rateLimiter := getCachedRateLimiter(cacheKey, region, tableName, limitCfg, logger)

			if rateLimiter == nil {
				// Failed to create rate limiter - fail open (allow request)
				logger.Error("failed to get rate limiter - allowing request",
					zap.String("endpoint", endpointKey),
					zap.Int("limit", limitCfg.limit),
					zap.Duration("window", limitCfg.window))
				return next.Handle(ctx)
			}

			// Apply the rate limiter middleware
			return rateLimiter(next).Handle(ctx)
		})
	}
}

// getCachedRateLimiter returns a cached rate limiter or creates a new one
func getCachedRateLimiter(cacheKey, region, tableName string, cfg endpointRateLimit, logger *zap.Logger) lift.Middleware {
	// Try to get from cache first (read lock)
	cacheMutex.RLock()
	if limiter, exists := rateLimiterCache[cacheKey]; exists {
		cacheMutex.RUnlock()
		return limiter
	}
	cacheMutex.RUnlock()

	// Need to create new limiter (write lock)
	cacheMutex.Lock()
	defer cacheMutex.Unlock()

	// Check again in case another goroutine created it while we were waiting
	if limiter, exists := rateLimiterCache[cacheKey]; exists {
		return limiter
	}

	// Create new rate limiter using Limited library
	limiter, err := limitedRateLimitFunc(liftMiddleware.LimitedConfig{
		Region:    region,
		TableName: tableName,
		Window:    cfg.window,
		Limit:     cfg.limit,
		Strategy:  "sliding", // Use sliding window for more precise rate limiting
		Logger:    logger,
	})

	if err != nil {
		logger.Error("failed to create Limited rate limiter",
			zap.Error(err),
			zap.String("region", region),
			zap.String("table", tableName),
			zap.Int("limit", cfg.limit),
			zap.Duration("window", cfg.window))
		return nil
	}

	// Cache for future use
	rateLimiterCache[cacheKey] = limiter
	logger.Info("created and cached Limited rate limiter",
		zap.String("cache_key", cacheKey),
		zap.Int("limit", cfg.limit),
		zap.Duration("window", cfg.window))

	return limiter
}

// buildCacheKey creates a cache key for a rate limit configuration
func buildCacheKey(cfg endpointRateLimit) string {
	return fmt.Sprintf("%d:%s", cfg.limit, cfg.window.String())
}

// lookupEndpointLimit finds rate limit config for an endpoint
func lookupEndpointLimit(endpoint string) (endpointRateLimit, bool) {
	// Direct match
	if cfg, exists := endpointRateLimits[endpoint]; exists {
		return cfg, true
	}

	// Wildcard matching
	for pattern, cfg := range endpointRateLimits {
		if matchEndpointPattern(endpoint, pattern) {
			return cfg, true
		}
	}

	return endpointRateLimit{}, false
}

// normalizeEndpointKey removes query params and trailing slashes
func normalizeEndpointKey(endpoint string) string {
	if idx := strings.Index(endpoint, "?"); idx != -1 {
		endpoint = endpoint[:idx]
	}
	return strings.TrimSuffix(endpoint, "/")
}

// matchEndpointPattern checks if endpoint matches a wildcard pattern
func matchEndpointPattern(endpoint, pattern string) bool {
	if !strings.Contains(pattern, "*") {
		return endpoint == pattern
	}

	parts := strings.Split(pattern, "*")
	if len(parts) != 2 {
		return false
	}

	prefix, suffix := parts[0], parts[1]
	return strings.HasPrefix(endpoint, prefix) && strings.HasSuffix(endpoint, suffix)
}
