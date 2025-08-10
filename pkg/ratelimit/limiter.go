// Package ratelimit provides rate limiting functionality with sliding window algorithms for API endpoint protection.
package ratelimit

import (
	"context"
	"net/http"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"go.uber.org/zap"
)

// RateLimitStorage defines the interface for rate limiting storage operations
//
//nolint:revive // RateLimit prefix clarifies this is ratelimit-specific storage
type RateLimitStorage interface {
	CheckAPIRateLimit(ctx context.Context, userID, endpoint string, limit int, window time.Duration) error
	GetAPIRateLimitInfo(ctx context.Context, userID, endpoint string, limit int, window time.Duration) (remaining int, resetTime time.Time, err error)
}

// RateLimiter provides rate limiting functionality
type RateLimiter struct {
	storage core.RepositoryStorage
	logger  *zap.Logger
}

// NewRateLimiter creates a new rate limiter instance
func NewRateLimiter(storage core.RepositoryStorage) *RateLimiter {
	return &RateLimiter{
		storage: storage,
		logger:  common.Logger(),
	}
}

// Check verifies if the rate limit has been exceeded for the given key
func (rl *RateLimiter) Check(ctx context.Context, userID, endpoint string, limit int, window time.Duration) error {
	return rl.storage.RateLimit().CheckAPIRateLimit(ctx, userID, endpoint, limit, window)
}

// GetLimitInfo returns current limit info for headers
func (rl *RateLimiter) GetLimitInfo(ctx context.Context, userID, endpoint string, limit int, window time.Duration) (remaining int, resetTime time.Time, err error) {
	return rl.storage.RateLimit().GetAPIRateLimitInfo(ctx, userID, endpoint, limit, window)
}

// LegacyRateLimitMiddleware creates an HTTP middleware for rate limiting (deprecated - use middleware.go instead)
//
//nolint:revive // RateLimit prefix clarifies this is ratelimit-specific middleware
func LegacyRateLimitMiddleware(limiter *RateLimiter) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// Simplified legacy middleware - use middleware.go for full functionality
			next(w, r)
		}
	}
}
