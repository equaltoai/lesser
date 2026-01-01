// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces

import (
	"context"
	"time"
)

// RateLimitRepository defines the interface for rate limiting operations.
// This handles login attempts, API rate limiting, federation rate limiting, and violations.
type RateLimitRepository interface {
	// ===== Login Attempt Operations =====

	// RecordLoginAttempt records a login attempt for rate limiting
	RecordLoginAttempt(ctx context.Context, identifier string, success bool) error

	// GetLoginAttemptCount returns the number of login attempts since the given time
	GetLoginAttemptCount(ctx context.Context, identifier string, since time.Time) (int, error)

	// IsRateLimited checks if an identifier is currently rate limited
	IsRateLimited(ctx context.Context, identifier string) (bool, time.Time, error)

	// ClearLoginAttempts clears all login attempts for an identifier
	ClearLoginAttempts(ctx context.Context, identifier string) error

	// ===== API Rate Limiting Operations =====

	// CheckAPIRateLimit checks and updates API rate limiting for a user/endpoint combination
	CheckAPIRateLimit(ctx context.Context, userID, endpoint string, limit int, window time.Duration) error

	// GetAPIRateLimitInfo returns current rate limit info for response headers
	GetAPIRateLimitInfo(ctx context.Context, userID, endpoint string, limit int, window time.Duration) (remaining int, resetTime time.Time, err error)

	// ===== Federation Rate Limiting Operations =====

	// CheckFederationRateLimit checks and updates federation rate limiting for a domain/endpoint combination
	CheckFederationRateLimit(ctx context.Context, domain, endpoint string, limit int, window time.Duration) error

	// GetFederationRateLimitInfo returns current federation rate limit info
	GetFederationRateLimitInfo(ctx context.Context, domain, endpoint string, limit int, window time.Duration) (remaining int, resetTime time.Time, err error)

	// ===== Violation Tracking Operations =====

	// GetViolationCount returns the number of violations in a time period for escalating penalties
	GetViolationCount(ctx context.Context, userID, domain string, since time.Duration) (int, error)

	// ===== Block Status Operations =====

	// IsUserBlocked checks if a user is currently blocked due to rate limiting
	IsUserBlocked(ctx context.Context, userID string) (bool, time.Time, error)

	// IsDomainBlocked checks if a federation domain is currently blocked
	IsDomainBlocked(ctx context.Context, domain string) (bool, time.Time, error)

	// ===== Community Note Rate Limiting =====

	// CheckCommunityNoteRateLimit checks if a user can create more community notes today
	CheckCommunityNoteRateLimit(ctx context.Context, userID string, limit int) (bool, int, error)
}
