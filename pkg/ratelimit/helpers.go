// Package ratelimit provides helpers for applying distributed request throttling.
package ratelimit

import (
	"os"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
	liftMiddleware "github.com/pay-theory/lift/pkg/middleware"
	"go.uber.org/zap"
)

// ApplyRateLimit wraps a handler with Limited-based rate limiting
// Returns the handler unchanged if rate limiter creation fails (fail-open)
func ApplyRateLimit(handler lift.Handler, limit int, window time.Duration, logger *zap.Logger) lift.Handler {
	// Get environment configuration
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = os.Getenv("AWS_DEFAULT_REGION")
	}
	if region == "" {
		region = "us-east-1"
	}

	tableName := os.Getenv("RATE_LIMIT_TABLE_NAME")
	if tableName == "" {
		tableName = os.Getenv("LIMITED_TABLE_NAME")
	}
	if tableName == "" {
		tableName = "rate-limits"
	}

	// Create rate limiter using Limited library
	limiter, err := liftMiddleware.LimitedRateLimit(liftMiddleware.LimitedConfig{
		Region:    region,
		TableName: tableName,
		Window:    window,
		Limit:     limit,
		Strategy:  "sliding",
		Logger:    logger,
	})

	if err != nil {
		logger.Error("failed to create rate limiter - allowing request",
			zap.Error(err),
			zap.Int("limit", limit),
			zap.Duration("window", window))
		// Fail open - return original handler
		return handler
	}

	// Wrap handler with rate limiter
	return limiter(handler)
}
