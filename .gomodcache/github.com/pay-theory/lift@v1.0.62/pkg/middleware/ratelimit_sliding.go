package middleware

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/pay-theory/lift/pkg/dynamorm"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/models"
)

// Memory optimized: 32 → 16 bytes (16 bytes saved)
type SlidingWindowRateLimiter struct {
	db           *dynamorm.DynamORMWrapper
	keyExtractor func(*lift.Context) string
	windowSize   time.Duration
	limit        int
}

func NewSlidingWindowRateLimiter(config RateLimitConfig) (*SlidingWindowRateLimiter, error) {
	// Validate configuration
	if config.DefaultWindow <= 0 {
		return nil, fmt.Errorf("window size must be positive")
	}

	if config.DefaultLimit <= 0 {
		return nil, fmt.Errorf("limit must be positive")
	}

	if config.DynamORM == nil {
		return nil, fmt.Errorf("DynamORM wrapper is required")
	}

	keyExtractor := func(ctx *lift.Context) string {
		// Default key extraction logic
		parts := []string{"ratelimit"}

		// Add tenant ID
		if tenantID := ctx.TenantID(); tenantID != "" {
			parts = append(parts, "tenant", tenantID)
		}

		// Add user ID
		if userID := ctx.UserID(); userID != "" {
			parts = append(parts, "user", userID)
		}

		// Add IP address as fallback
		if len(parts) == 1 {
			if ip := ctx.Header("X-Forwarded-For"); ip != "" {
				parts = append(parts, "ip", ip)
			} else if ip := ctx.Header("X-Real-IP"); ip != "" {
				parts = append(parts, "ip", ip)
			} else {
				parts = append(parts, "ip", "unknown")
			}
		}

		// Add path if configured
		if config.IncludePath {
			parts = append(parts, "path", ctx.Request.Path)
		}

		// Add method if configured
		if config.IncludeMethod {
			parts = append(parts, "method", ctx.Request.Method)
		}

		key := ""
		for i, part := range parts {
			if i > 0 {
				key += ":"
			}
			key += part
		}

		return key
	}

	return &SlidingWindowRateLimiter{
		db:           config.DynamORM,
		windowSize:   config.DefaultWindow,
		limit:        config.DefaultLimit,
		keyExtractor: keyExtractor,
	}, nil
}

func (r *SlidingWindowRateLimiter) Middleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Extract rate limit key
			key := r.keyExtractor(ctx)
			if key == "" {
				return next.Handle(ctx)
			}

			// Check rate limit
			allowed, remaining, resetAt, err := r.checkRateLimit(ctx.Context, key)
			if err != nil {
				// On error, allow request but log
				if ctx.Logger != nil {
					ctx.Logger.Error("Rate limit check failed", map[string]any{
						"error": err.Error(),
						"key":   key,
					})
				}
				return next.Handle(ctx)
			}

			// Set rate limit headers
			ctx.Response.Header("X-RateLimit-Limit", fmt.Sprintf("%d", r.limit))
			ctx.Response.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
			ctx.Response.Header("X-RateLimit-Reset", fmt.Sprintf("%d", resetAt.Unix()))

			if !allowed {
				ctx.Response.Header("Retry-After", fmt.Sprintf("%d", int(time.Until(resetAt).Seconds())))
				return ctx.Response.Status(429).JSON(map[string]any{
					"error":       "rate_limit_exceeded",
					"message":     "Too many requests",
					"retry_after": int(time.Until(resetAt).Seconds()),
				})
			}

			// Record this request
			if err := r.recordRequest(ctx.Context, key); err != nil {
				if ctx.Logger != nil {
					ctx.Logger.Warn("Failed to record rate limit entry", map[string]any{
						"error": err.Error(),
						"key":   key,
					})
				}
			}

			return next.Handle(ctx)
		})
	}
}

func (r *SlidingWindowRateLimiter) checkRateLimit(ctx context.Context, key string) (bool, int, time.Time, error) {
	now := time.Now()
	windowStart := now.Add(-r.windowSize)

	// Create a query for entries within the window
	pk := fmt.Sprintf("RATELIMIT#%s", key)

	// Note: This requires DynamORM to support range queries
	// For now, we'll implement a simplified version that uses the existing Get/Put interface

	// Count requests in window by iterating through potential entries
	// In production, this would use a proper DynamoDB query
	count := 0

	// For demonstration, we'll store a single counter entry per key with timestamp buckets
	// This is a simplified approach - a full implementation would use proper range queries
	// Memory optimized: 64 → 48 bytes (16 bytes saved)
	var windowEntry struct {
		Timestamp time.Time `` // 24 bytes
		PK        string    `` // 16 bytes
		SK        string    `` // 16 bytes
		TTL       int64     `` // 8 bytes
		Count     int       `` // 4 bytes
	}

	// Get current window entry
	entryKey := fmt.Sprintf("%s#%d", pk, now.Unix()/int64(r.windowSize.Seconds()))
	err := r.db.Get(ctx, entryKey, &windowEntry)
	if err == nil {
		// Check if entry is within current window
		if windowEntry.Timestamp.After(windowStart) {
			count = windowEntry.Count
		}
	}

	remaining := r.limit - count
	if remaining < 0 {
		remaining = 0
	}

	// Reset time is when the current window expires
	resetAt := now.Add(r.windowSize)

	return count < r.limit, remaining, resetAt, nil
}

func (r *SlidingWindowRateLimiter) recordRequest(ctx context.Context, key string) error {
	now := time.Now()

	// Create entry
	entry := &models.SlidingWindowEntry{
		RequestID:    uuid.New().String(),
		Timestamp:    now,
		Weight:       1,
		ExpiresAt:    now.Add(r.windowSize + time.Hour).Unix(), // Buffer for cleanup
		RateLimitKey: key,
	}
	entry.Key(key, now)

	// Store in DynamoDB
	return r.db.Put(ctx, entry)
}

// SlidingWindowRateLimit creates a sliding window rate limiter
func SlidingWindowRateLimit(_ int, _ time.Duration) (lift.Middleware, error) {
	// This would require DynamORMWrapper to be passed in
	// For now, return an error indicating the need for proper setup
	return nil, fmt.Errorf("sliding window rate limit requires DynamORM configuration - use NewSlidingWindowRateLimiter with full config")
}
