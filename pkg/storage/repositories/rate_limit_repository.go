package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// RateLimitRepository handles rate limiting operations using DynamORM
type RateLimitRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewRateLimitRepository creates a new RateLimitRepository
func NewRateLimitRepository(db core.DB, tableName string, logger *zap.Logger) *RateLimitRepository {
	return &RateLimitRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// RecordLoginAttempt records a login attempt for rate limiting
func (r *RateLimitRepository) RecordLoginAttempt(ctx context.Context, identifier string, success bool) error {
	attempt := models.NewLoginAttempt(identifier, success)

	err := r.db.WithContext(ctx).Model(attempt).Create()
	if err != nil {
		r.logger.Error("failed to record login attempt",
			zap.String("identifier", identifier),
			zap.Bool("success", success),
			zap.Error(err))
		return err
	}

	r.logger.Debug("recorded login attempt",
		zap.String("identifier", identifier),
		zap.Bool("success", success))

	return nil
}

// GetLoginAttemptCount returns the number of login attempts since the given time
func (r *RateLimitRepository) GetLoginAttemptCount(ctx context.Context, identifier string, since time.Time) (int, error) {
	var attempts []models.LoginAttempt

	pk := fmt.Sprintf("RATELIMIT#%s", identifier)
	sinceKey := since.Format(time.RFC3339Nano)

	err := r.db.WithContext(ctx).Model(&models.LoginAttempt{}).
		Where("PK", "=", pk).
		Where("SK", ">", sinceKey).
		All(&attempts)

	if err != nil {
		r.logger.Error("failed to get login attempt count",
			zap.String("identifier", identifier),
			zap.Time("since", since),
			zap.Error(err))
		return 0, err
	}

	count := len(attempts)
	r.logger.Debug("retrieved login attempt count",
		zap.String("identifier", identifier),
		zap.Time("since", since),
		zap.Int("count", count))

	return count, nil
}

// IsRateLimited checks if an identifier is currently rate limited
func (r *RateLimitRepository) IsRateLimited(ctx context.Context, identifier string) (bool, time.Time, error) {
	var lockout models.RateLimitLockout

	pk := fmt.Sprintf("RATELIMIT#%s", identifier)

	err := r.db.WithContext(ctx).Model(&models.RateLimitLockout{}).
		Where("PK", "=", pk).
		Where("SK", "=", "LOCKOUT").
		First(&lockout)

	if err != nil {
		if errors.IsNotFound(err) {
			// No lockout record found, not rate limited
			return false, time.Time{}, nil
		}
		r.logger.Error("failed to check rate limit",
			zap.String("identifier", identifier),
			zap.Error(err))
		return false, time.Time{}, err
	}

	// Check if lockout is still active
	now := time.Now()
	if now.Before(lockout.UnlockTime) {
		r.logger.Debug("identifier is rate limited",
			zap.String("identifier", identifier),
			zap.Time("unlock_time", lockout.UnlockTime))
		return true, lockout.UnlockTime, nil
	}

	// Lockout has expired, not rate limited
	r.logger.Debug("lockout expired, identifier not rate limited",
		zap.String("identifier", identifier),
		zap.Time("unlock_time", lockout.UnlockTime))

	return false, time.Time{}, nil
}

// ClearLoginAttempts clears all login attempts for an identifier
func (r *RateLimitRepository) ClearLoginAttempts(ctx context.Context, identifier string) error {
	// First, query all attempts for this identifier
	var attempts []models.LoginAttempt
	pk := fmt.Sprintf("RATELIMIT#%s", identifier)

	err := r.db.WithContext(ctx).Model(&models.LoginAttempt{}).
		Where("PK", "=", pk).
		All(&attempts)

	if err != nil {
		r.logger.Error("failed to query login attempts for clearing",
			zap.String("identifier", identifier),
			zap.Error(err))
		return err
	}

	// Delete each attempt
	for _, attempt := range attempts {
		err := r.db.WithContext(ctx).Model(&models.LoginAttempt{}).
			Where("PK", "=", attempt.PK).
			Where("SK", "=", attempt.SK).
			Delete()

		if err != nil {
			r.logger.Error("failed to delete login attempt",
				zap.String("identifier", identifier),
				zap.String("pk", attempt.PK),
				zap.String("sk", attempt.SK),
				zap.Error(err))
			// Continue with other deletions even if one fails
		}
	}

	// Also clear any lockout record
	err = r.db.WithContext(ctx).Model(&models.RateLimitLockout{}).
		Where("PK", "=", pk).
		Where("SK", "=", "LOCKOUT").
		Delete()

	if err != nil && !errors.IsNotFound(err) {
		r.logger.Error("failed to delete lockout record",
			zap.String("identifier", identifier),
			zap.Error(err))
		// Don't return error as this is not critical
	}

	r.logger.Debug("cleared login attempts",
		zap.String("identifier", identifier),
		zap.Int("attempts_cleared", len(attempts)))

	return nil
}

// CheckCommunityNoteRateLimit checks if a user can create more community notes today
func (r *RateLimitRepository) CheckCommunityNoteRateLimit(ctx context.Context, userID string, limit int) (bool, int, error) {
	// Query notes created by user in last 24 hours using GSI3
	// This matches the legacy implementation pattern exactly
	yesterday := time.Now().Add(-24 * time.Hour)

	// Use DynamORM to query CommunityNote model with GSI3
	var notes []models.CommunityNote
	gsi3PK := fmt.Sprintf("AUTHOR#%s#NOTES", userID)
	gsi3SKPrefix := yesterday.Format(time.RFC3339)

	err := r.db.WithContext(ctx).Model(&models.CommunityNote{}).
		Index("gsi3").
		Where("GSI3PK", "=", gsi3PK).
		Where("GSI3SK", ">", gsi3SKPrefix).
		All(&notes)

	if err != nil {
		r.logger.Error("failed to check community note rate limit",
			zap.String("userID", userID),
			zap.Int("limit", limit),
			zap.Error(err))
		return false, 0, err
	}

	count := len(notes)
	canCreate := count < limit
	remaining := limit - count
	if remaining < 0 {
		remaining = 0
	}

	r.logger.Debug("checked community note rate limit",
		zap.String("userID", userID),
		zap.Int("limit", limit),
		zap.Int("current_count", count),
		zap.Bool("can_create", canCreate),
		zap.Int("remaining", remaining))

	return canCreate, remaining, nil
}

// CheckAPIRateLimit checks and updates API rate limiting for a user/endpoint combination
// This matches the behavior from the original limiter.go implementation
func (r *RateLimitRepository) CheckAPIRateLimit(ctx context.Context, userID, endpoint string, limit int, window time.Duration) error {
	now := time.Now()
	windowStart := now.Truncate(window)
	key := fmt.Sprintf("%s:%s", userID, endpoint)

	// Get current rate limit data
	var current models.APIRateLimit
	pk := fmt.Sprintf("RATELIMIT#%s", key)
	sk := fmt.Sprintf("WINDOW#%s", windowStart.Format(time.RFC3339))

	err := r.db.WithContext(ctx).Model(&models.APIRateLimit{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&current)

	if err != nil {
		if errors.IsNotFound(err) {
			// Create new rate limit record
			current = *models.NewAPIRateLimit(userID, endpoint, windowStart)
		} else {
			r.logger.Error("failed to get API rate limit",
				zap.String("user_id", userID),
				zap.String("endpoint", endpoint),
				zap.Error(err))
			// Don't fail the request if we can't get rate limit data
			return nil
		}
	}

	// Check if explicitly blocked
	if current.Blocked && now.Before(current.BlockedUntil) {
		return fmt.Errorf("rate limit exceeded, blocked until %v", current.BlockedUntil)
	}

	// Reset if new window (this shouldn't happen with our PK/SK design, but safety check)
	if current.Window.Before(windowStart) {
		current.Count = 0
		current.Window = windowStart
		current.Blocked = false
	}

	// Increment counter
	current.Count++
	current.UpdatedAt = now

	// Check limit
	if current.Count > limit {
		// Block for increasing durations based on how much over limit
		blockDuration := time.Duration(current.Count/limit) * time.Hour
		if blockDuration > 24*time.Hour {
			blockDuration = 24 * time.Hour
		}

		current.Blocked = true
		current.BlockedUntil = now.Add(blockDuration)

		// Update the record
		if err := r.updateAPIRateLimit(ctx, &current); err != nil {
			r.logger.Error("failed to update blocked API rate limit",
				zap.String("user_id", userID),
				zap.String("endpoint", endpoint),
				zap.Error(err))
		}

		return fmt.Errorf("rate limit exceeded (%d > %d)", current.Count, limit)
	}

	// Update counter
	if err := r.updateAPIRateLimit(ctx, &current); err != nil {
		r.logger.Error("failed to update API rate limit counter",
			zap.String("user_id", userID),
			zap.String("endpoint", endpoint),
			zap.Error(err))
		// Don't fail the request if we can't update the counter
	}

	return nil
}

// GetAPIRateLimitInfo returns current rate limit info for response headers
func (r *RateLimitRepository) GetAPIRateLimitInfo(ctx context.Context, userID, endpoint string, limit int, window time.Duration) (remaining int, resetTime time.Time, err error) {
	now := time.Now()
	windowStart := now.Truncate(window)
	resetTime = windowStart.Add(window)
	key := fmt.Sprintf("%s:%s", userID, endpoint)

	// Get current rate limit data
	var current models.APIRateLimit
	pk := fmt.Sprintf("RATELIMIT#%s", key)
	sk := fmt.Sprintf("WINDOW#%s", windowStart.Format(time.RFC3339))

	err = r.db.WithContext(ctx).Model(&models.APIRateLimit{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&current)

	if err != nil {
		if errors.IsNotFound(err) {
			// No data yet, full limit available
			return limit, resetTime, nil
		}
		r.logger.Error("failed to get API rate limit info",
			zap.String("user_id", userID),
			zap.String("endpoint", endpoint),
			zap.Error(err))
		// Return full limit on error to avoid blocking legitimate requests
		return limit, resetTime, nil
	}

	// Calculate remaining
	remaining = limit - current.Count
	if remaining < 0 {
		remaining = 0
	}

	return remaining, resetTime, nil
}

// updateAPIRateLimit updates an API rate limit record using DynamORM
func (r *RateLimitRepository) updateAPIRateLimit(ctx context.Context, limit *models.APIRateLimit) error {
	// Ensure keys are set correctly
	limit.UpdateKeys()

	// For DynamORM, we can use Create which will overwrite existing items
	// This acts like a PUT operation in DynamoDB
	err := r.db.WithContext(ctx).Model(limit).Create()
	if err != nil {
		r.logger.Error("failed to create/update API rate limit",
			zap.String("pk", limit.PK),
			zap.String("sk", limit.SK),
			zap.Error(err))
		return err
	}

	return nil
}
