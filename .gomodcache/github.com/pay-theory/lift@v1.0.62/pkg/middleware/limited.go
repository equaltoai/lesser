package middleware

import (
	"fmt"
	"os"
	"time"

	"github.com/pay-theory/dynamorm"
	"github.com/pay-theory/dynamorm/pkg/session"
	"github.com/pay-theory/limited"
	"go.uber.org/zap"

	"github.com/pay-theory/lift/pkg/lift"
)

// LimitedConfig holds configuration for the limited-based rate limiter
type LimitedConfig struct {
	Logger    *zap.Logger
	Region    string
	TableName string
	Endpoint  string
	Strategy  string
	Window    time.Duration
	Limit     int
}

// LimitedRateLimit creates a rate limiting middleware using the limited library
// This is the CORRECT way to do rate limiting with DynamoDB in Lift
func LimitedRateLimit(config LimitedConfig) (lift.Middleware, error) {
	// Validate required fields
	if config.Region == "" {
		return nil, fmt.Errorf("region is required for DynamoDB connection")
	}

	// Set defaults
	if config.TableName == "" {
		config.TableName = "rate-limits"
	}
	if config.Window == 0 {
		config.Window = time.Hour
	}
	if config.Limit == 0 {
		config.Limit = 1000
	}
	if config.Logger == nil {
		config.Logger = zap.NewNop()
	}

	// Create DynamoDB connection
	dbConfig := session.Config{
		Region: config.Region,
	}
	if config.Endpoint != "" {
		dbConfig.Endpoint = config.Endpoint
	}

	db, err := dynamorm.NewBasic(dbConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize DynamoDB: %w", err)
	}

	// Create strategy
	var strategy limited.RateLimitStrategy
	switch config.Strategy {
	case "sliding":
		strategy = limited.NewSlidingWindowStrategy(config.Window, config.Limit, config.Window/10) // granularity = window/10
	default:
		strategy = limited.NewFixedWindowStrategy(config.Window, config.Limit)
	}

	// Create rate limiter
	limiter := limited.NewDynamoRateLimiter(db, nil, strategy, config.Logger)

	// Return middleware
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Generate rate limit key
			key := generateKey(ctx)

			// Check rate limit
			decision, err := limiter.CheckAndIncrement(ctx.Context, key)
			if err != nil {
				// Log error but allow request on failure (fail open)
				if ctx.Logger != nil {
					ctx.Logger.Error("Rate limit check failed", map[string]any{
						"error": err.Error(),
						"key":   key,
					})
				}
				return next.Handle(ctx)
			}

			// Set headers
			remaining := decision.Limit - decision.CurrentCount
			if remaining < 0 {
				remaining = 0
			}
			ctx.Response.Header("X-RateLimit-Limit", fmt.Sprintf("%d", decision.Limit))
			ctx.Response.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
			ctx.Response.Header("X-RateLimit-Reset", fmt.Sprintf("%d", decision.ResetsAt.Unix()))

			if !decision.Allowed {
				retryAfter := 60 // default to 60 seconds
				if decision.RetryAfter != nil {
					retryAfter = int(decision.RetryAfter.Seconds())
				}
				ctx.Response.Header("Retry-After", fmt.Sprintf("%d", retryAfter))

				return ctx.Response.Status(429).JSON(map[string]any{
					"error":       "Rate limit exceeded",
					"limit":       decision.Limit,
					"remaining":   remaining,
					"reset_at":    decision.ResetsAt.Unix(),
					"retry_after": retryAfter,
				})
			}

			return next.Handle(ctx)
		})
	}, nil
}

// IPRateLimitWithLimited creates an IP-based rate limiter
func IPRateLimitWithLimited(limit int, window time.Duration) (lift.Middleware, error) {
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = os.Getenv("AWS_DEFAULT_REGION")
	}
	if region == "" {
		return nil, fmt.Errorf("AWS_REGION environment variable not set")
	}

	return LimitedRateLimit(LimitedConfig{
		Region: region,
		Limit:  limit,
		Window: window,
	})
}

// UserRateLimitWithLimited creates a user-based rate limiter
func UserRateLimitWithLimited(limit int, window time.Duration) (lift.Middleware, error) {
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = os.Getenv("AWS_DEFAULT_REGION")
	}
	if region == "" {
		return nil, fmt.Errorf("AWS_REGION environment variable not set")
	}

	return LimitedRateLimit(LimitedConfig{
		Region: region,
		Limit:  limit,
		Window: window,
	})
}

// TenantRateLimitWithLimited creates a tenant-based rate limiter
func TenantRateLimitWithLimited(limit int, window time.Duration) (lift.Middleware, error) {
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = os.Getenv("AWS_DEFAULT_REGION")
	}
	if region == "" {
		return nil, fmt.Errorf("AWS_REGION environment variable not set")
	}

	return LimitedRateLimit(LimitedConfig{
		Region: region,
		Limit:  limit,
		Window: window,
	})
}

func generateKey(ctx *lift.Context) limited.RateLimitKey {
	key := limited.RateLimitKey{
		Resource:  ctx.Request.Path,
		Operation: ctx.Request.Method,
		Metadata:  make(map[string]string),
	}

	// Priority: User ID > Tenant ID > IP
	if userID := ctx.UserID(); userID != "" {
		key.Identifier = fmt.Sprintf("user:%s", userID)
		key.Metadata["user_id"] = userID
	} else if tenantID := ctx.TenantID(); tenantID != "" {
		key.Identifier = fmt.Sprintf("tenant:%s", tenantID)
		key.Metadata["tenant_id"] = tenantID
	} else {
		// Fall back to IP address
		ip := ctx.Header("X-Forwarded-For")
		if ip == "" {
			ip = ctx.Header("X-Real-IP")
		}
		if ip == "" {
			ip = "unknown"
		}
		key.Identifier = fmt.Sprintf("ip:%s", ip)
		key.Metadata["ip"] = ip
	}

	return key
}
