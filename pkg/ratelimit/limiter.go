package ratelimit

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"go.uber.org/zap"
)

// RateLimitStorage defines the interface for rate limiting storage operations
type RateLimitStorage interface {
	CheckAPIRateLimit(ctx context.Context, userID, endpoint string, limit int, window time.Duration) error
	GetAPIRateLimitInfo(ctx context.Context, userID, endpoint string, limit int, window time.Duration) (remaining int, resetTime time.Time, err error)
}

type RateLimiter struct {
	storage storage.Storage
	logger  *zap.Logger
}


func NewRateLimiter(storage storage.Storage) *RateLimiter {
	return &RateLimiter{
		storage: storage,
		logger:  common.Logger(),
	}
}

func (rl *RateLimiter) Check(ctx context.Context, userID, endpoint string, limit int, window time.Duration) error {
	return rl.storage.CheckAPIRateLimit(ctx, userID, endpoint, limit, window)
}


// GetLimitInfo returns current limit info for headers
func (rl *RateLimiter) GetLimitInfo(ctx context.Context, userID, endpoint string, limit int, window time.Duration) (remaining int, resetTime time.Time, err error) {
	return rl.storage.GetAPIRateLimitInfo(ctx, userID, endpoint, limit, window)
}

// RateLimitMiddleware creates a middleware for rate limiting
func RateLimitMiddleware(limiter *RateLimiter) func(http.HandlerFunc) http.HandlerFunc {
	// Define limits per endpoint
	limits := map[string]struct {
		limit  int
		window time.Duration
	}{
		"POST:/api/v1/statuses":             {limit: 30, window: time.Hour},  // 30 posts per hour
		"POST:/api/v1/media":                {limit: 10, window: time.Hour},  // 10 uploads per hour
		"POST:/api/v1/accounts":             {limit: 5, window: time.Hour},   // 5 follows per hour
		"POST:/api/v1/follows":              {limit: 30, window: time.Hour},  // 30 follows per hour
		"DELETE:/api/v1/statuses/*":         {limit: 30, window: time.Hour},  // 30 deletes per hour
		"POST:/api/v1/statuses/*/favourite": {limit: 120, window: time.Hour}, // 120 likes per hour
		"POST:/api/v1/statuses/*/reblog":    {limit: 60, window: time.Hour},  // 60 reblogs per hour
	}

	// Default limit for unspecified endpoints
	defaultLimit := struct {
		limit  int
		window time.Duration
	}{limit: 100, window: time.Hour}

	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// Get claims from context
			claims, ok := auth.GetClaims(r.Context())
			if !ok || claims == nil {
				// No authenticated user, skip rate limiting
				next(w, r)
				return
			}

			// Build endpoint key
			endpoint := fmt.Sprintf("%s:%s", r.Method, r.URL.Path)

			// Find matching limit
			limitConfig, exists := limits[endpoint]
			if !exists {
				// Try wildcard matching
				for pattern, config := range limits {
					if matchesPattern(endpoint, pattern) {
						limitConfig = config
						exists = true
						break
					}
				}
			}

			if !exists {
				limitConfig = defaultLimit
			}

			// Check rate limit
			err := limiter.Check(r.Context(), claims.Username, endpoint, limitConfig.limit, limitConfig.window)

			// Set rate limit headers
			remaining, resetTime, _ := limiter.GetLimitInfo(r.Context(), claims.Username, endpoint, limitConfig.limit, limitConfig.window)
			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", limitConfig.limit))
			w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
			w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", resetTime.Unix()))

			if err != nil {
				// Rate limit exceeded
				w.Header().Set("Retry-After", fmt.Sprintf("%d", int(limitConfig.window.Seconds())))

				http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			next(w, r)
		}
	}
}

// matchesPattern checks if an endpoint matches a pattern with wildcards
func matchesPattern(endpoint, pattern string) bool {
	// Simple wildcard matching for paths like /api/v1/statuses/*/favourite
	// This is a simplified version - in production you'd want more robust matching

	if pattern == endpoint {
		return true
	}

	// Check if pattern contains wildcard
	if !contains(pattern, "*") {
		return false
	}

	// Split by wildcard and check prefix/suffix
	parts := splitByWildcard(pattern)
	if len(parts) != 2 {
		return false
	}

	return hasPrefix(endpoint, parts[0]) && hasSuffix(endpoint, parts[1])
}

// Helper functions for string operations (to avoid importing strings for simple ops)
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func splitByWildcard(s string) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == '*' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
