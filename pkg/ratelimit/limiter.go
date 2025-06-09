package ratelimit

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

type RateLimiter struct {
	db        *dynamodb.Client
	tableName string
	logger    *zap.Logger
}

type RateLimit struct {
	Key          string    `dynamodbav:"key"`
	Count        int       `dynamodbav:"count"`
	Window       time.Time `dynamodbav:"window"`
	Blocked      bool      `dynamodbav:"blocked"`
	BlockedUntil time.Time `dynamodbav:"blocked_until"`
	UpdatedAt    time.Time `dynamodbav:"updated_at"`
}

func NewRateLimiter(db *dynamodb.Client, tableName string) *RateLimiter {
	return &RateLimiter{
		db:        db,
		tableName: tableName,
		logger:    common.Logger(),
	}
}

func (rl *RateLimiter) Check(ctx context.Context, userID, endpoint string, limit int, window time.Duration) error {
	key := fmt.Sprintf("%s:%s", userID, endpoint)
	now := time.Now()
	windowStart := now.Truncate(window)

	// Get current rate limit data
	result, err := rl.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(rl.tableName),
		Key: map[string]types.AttributeValue{
			"key": &types.AttributeValueMemberS{Value: key},
		},
	})

	var current RateLimit
	if err == nil && result.Item != nil {
		// Parse existing item
		if err := attributevalue.UnmarshalMap(result.Item, &current); err != nil {
			rl.logger.Error("failed to unmarshal rate limit",
				zap.String("key", key),
				zap.Error(err))
			// Continue with fresh rate limit
			current = RateLimit{Key: key}
		}
	} else {
		// New rate limit
		current.Key = key
	}

	// Check if explicitly blocked
	if current.Blocked && now.Before(current.BlockedUntil) {
		return fmt.Errorf("rate limit exceeded, blocked until %v", current.BlockedUntil)
	}

	// Reset if new window
	if current.Window.Before(windowStart) {
		current.Count = 0
		current.Window = windowStart
	}

	// Increment counter
	current.Count++
	current.UpdatedAt = now

	// Check limit
	if current.Count > limit {
		// Block for increasing durations
		blockDuration := time.Duration(current.Count/limit) * time.Hour
		if blockDuration > 24*time.Hour {
			blockDuration = 24 * time.Hour
		}

		current.Blocked = true
		current.BlockedUntil = now.Add(blockDuration)

		// Update database
		if err := rl.updateLimit(ctx, current); err != nil {
			rl.logger.Error("failed to update rate limit",
				zap.String("key", key),
				zap.Error(err))
		}

		return fmt.Errorf("rate limit exceeded (%d > %d)", current.Count, limit)
	}

	// Update counter
	if err := rl.updateLimit(ctx, current); err != nil {
		rl.logger.Error("failed to update rate limit",
			zap.String("key", key),
			zap.Error(err))
		// Don't fail the request if we can't update the counter
	}

	return nil
}

func (rl *RateLimiter) updateLimit(ctx context.Context, limit RateLimit) error {
	item, err := attributevalue.MarshalMap(limit)
	if err != nil {
		return fmt.Errorf("failed to marshal rate limit: %w", err)
	}

	// Set TTL to expire after window + 1 day
	ttl := limit.Window.Add(25 * time.Hour).Unix()
	item["ttl"] = &types.AttributeValueMemberN{Value: strconv.FormatInt(ttl, 10)}

	_, err = rl.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(rl.tableName),
		Item:      item,
	})

	return err
}

// GetLimitInfo returns current limit info for headers
func (rl *RateLimiter) GetLimitInfo(ctx context.Context, userID, endpoint string, limit int, window time.Duration) (remaining int, resetTime time.Time, err error) {
	key := fmt.Sprintf("%s:%s", userID, endpoint)
	now := time.Now()
	windowStart := now.Truncate(window)
	resetTime = windowStart.Add(window)

	// Get current rate limit data
	result, err := rl.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(rl.tableName),
		Key: map[string]types.AttributeValue{
			"key": &types.AttributeValueMemberS{Value: key},
		},
	})

	if err != nil || result.Item == nil {
		// No data yet, full limit available
		return limit, resetTime, nil
	}

	var current RateLimit
	if err := attributevalue.UnmarshalMap(result.Item, &current); err != nil {
		// Error parsing, assume full limit
		return limit, resetTime, nil
	}

	// If different window, full limit available
	if current.Window.Before(windowStart) {
		return limit, resetTime, nil
	}

	// Calculate remaining
	remaining = limit - current.Count
	if remaining < 0 {
		remaining = 0
	}

	return remaining, resetTime, nil
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
