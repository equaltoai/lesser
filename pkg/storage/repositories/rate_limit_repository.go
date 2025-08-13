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

	// Check limit with escalating penalties
	if current.Count > limit {
		// Get violation history for escalating penalties
		violationCount, err := r.GetViolationCount(ctx, userID, "", 24*time.Hour)
		if err != nil {
			r.logger.Warn("failed to get violation count, using default penalty",
				zap.String("user_id", userID),
				zap.Error(err))
			violationCount = 1
		} else {
			violationCount++ // Current violation
		}

		// Calculate escalating penalty
		penaltyDuration := r.calculatePenaltyDuration(violationCount)
		current.Blocked = true
		current.BlockedUntil = now.Add(penaltyDuration)
		current.ViolationCount = violationCount
		if current.FirstViolation.IsZero() {
			current.FirstViolation = now
		}
		current.LastViolation = now

		// Record the violation
		violation := models.NewRateLimitViolation(userID, "", endpoint, "api", int(penaltyDuration.Minutes()))
		if err := r.db.WithContext(ctx).Model(violation).Create(); err != nil {
			r.logger.Error("failed to record API rate limit violation",
				zap.String("user_id", userID),
				zap.String("endpoint", endpoint),
				zap.Error(err))
		}

		// Update the record
		if err := r.updateAPIRateLimit(ctx, &current); err != nil {
			r.logger.Error("failed to update blocked API rate limit",
				zap.String("user_id", userID),
				zap.String("endpoint", endpoint),
				zap.Error(err))
		}

		return fmt.Errorf("rate limit exceeded (%d > %d), blocked for %v", current.Count, limit, penaltyDuration)
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

// CheckFederationRateLimit checks and updates federation rate limiting for a domain/endpoint combination
func (r *RateLimitRepository) CheckFederationRateLimit(ctx context.Context, domain, endpoint string, limit int, window time.Duration) error {
	now := time.Now()
	windowStart := now.Truncate(window)
	key := fmt.Sprintf("DOMAIN#%s:%s", domain, endpoint)

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
			current = *models.NewFederationRateLimit(domain, endpoint, windowStart)
		} else {
			r.logger.Error("failed to get federation rate limit",
				zap.String("domain", domain),
				zap.String("endpoint", endpoint),
				zap.Error(err))
			// Don't fail the request if we can't get rate limit data
			return nil
		}
	}

	// Check if explicitly blocked
	if current.Blocked && now.Before(current.BlockedUntil) {
		return fmt.Errorf("federation rate limit exceeded for domain %s, blocked until %v", domain, current.BlockedUntil)
	}

	// Reset if new window
	if current.Window.Before(windowStart) {
		current.Count = 0
		current.Window = windowStart
		current.Blocked = false
	}

	// Increment counter
	current.Count++
	current.UpdatedAt = now

	// Check limit with escalating penalties
	if current.Count > limit {
		// Get violation history for escalating penalties
		violationCount, err := r.GetViolationCount(ctx, "", domain, 24*time.Hour)
		if err != nil {
			r.logger.Warn("failed to get violation count, using default penalty",
				zap.String("domain", domain),
				zap.Error(err))
			violationCount = 1
		} else {
			violationCount++ // Current violation
		}

		// Calculate escalating penalty
		penaltyDuration := r.calculatePenaltyDuration(violationCount)
		current.Blocked = true
		current.BlockedUntil = now.Add(penaltyDuration)
		current.ViolationCount = violationCount
		if current.FirstViolation.IsZero() {
			current.FirstViolation = now
		}
		current.LastViolation = now

		// Record the violation
		violation := models.NewRateLimitViolation("", domain, endpoint, "federation", int(penaltyDuration.Minutes()))
		if err := r.db.WithContext(ctx).Model(violation).Create(); err != nil {
			r.logger.Error("failed to record federation rate limit violation",
				zap.String("domain", domain),
				zap.String("endpoint", endpoint),
				zap.Error(err))
		}

		// Update the rate limit record
		if err := r.updateAPIRateLimit(ctx, &current); err != nil {
			r.logger.Error("failed to update blocked federation rate limit",
				zap.String("domain", domain),
				zap.String("endpoint", endpoint),
				zap.Error(err))
		}

		return fmt.Errorf("federation rate limit exceeded for domain %s (%d > %d), blocked for %v", domain, current.Count, limit, penaltyDuration)
	}

	// Update counter
	if err := r.updateAPIRateLimit(ctx, &current); err != nil {
		r.logger.Error("failed to update federation rate limit counter",
			zap.String("domain", domain),
			zap.String("endpoint", endpoint),
			zap.Error(err))
		// Don't fail the request if we can't update the counter
	}

	return nil
}

// GetFederationRateLimitInfo returns current federation rate limit info
func (r *RateLimitRepository) GetFederationRateLimitInfo(ctx context.Context, domain, endpoint string, limit int, window time.Duration) (remaining int, resetTime time.Time, err error) {
	now := time.Now()
	windowStart := now.Truncate(window)
	resetTime = windowStart.Add(window)
	key := fmt.Sprintf("DOMAIN#%s:%s", domain, endpoint)

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
		r.logger.Error("failed to get federation rate limit info",
			zap.String("domain", domain),
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

// GetViolationCount returns the number of violations in a time period for escalating penalties
func (r *RateLimitRepository) GetViolationCount(ctx context.Context, userID, domain string, since time.Duration) (int, error) {
	identifier := userID
	if domain != "" {
		identifier = fmt.Sprintf("DOMAIN#%s", domain)
	}

	var violations []models.RateLimitViolation
	pk := fmt.Sprintf("RATELIMIT_VIOLATION#%s", identifier)
	sinceKey := time.Now().Add(-since).Format(time.RFC3339Nano)

	err := r.db.WithContext(ctx).Model(&models.RateLimitViolation{}).
		Where("PK", "=", pk).
		Where("SK", ">", sinceKey).
		All(&violations)

	if err != nil {
		r.logger.Error("failed to get violation count",
			zap.String("user_id", userID),
			zap.String("domain", domain),
			zap.Duration("since", since),
			zap.Error(err))
		return 0, err
	}

	return len(violations), nil
}

// calculatePenaltyDuration calculates escalating penalty duration based on violation count
func (r *RateLimitRepository) calculatePenaltyDuration(violationCount int) time.Duration {
	switch violationCount {
	case 1:
		return time.Minute // First violation: 1 minute
	case 2:
		return 5 * time.Minute // Second violation: 5 minutes
	case 3:
		return 15 * time.Minute // Third violation: 15 minutes
	default:
		return time.Hour // Repeated violations: 1 hour
	}
}

// IsUserBlocked checks if a user is currently blocked due to rate limiting
func (r *RateLimitRepository) IsUserBlocked(ctx context.Context, userID string) (bool, time.Time, error) {
	now := time.Now()

	// Check recent rate limit records for any blocks
	var rateLimits []models.APIRateLimit
	pk := fmt.Sprintf("RATELIMIT#%s", userID)

	err := r.db.WithContext(ctx).Model(&models.APIRateLimit{}).
		Where("PK", "=", pk).
		All(&rateLimits)

	if err != nil {
		if errors.IsNotFound(err) {
			return false, time.Time{}, nil
		}
		return false, time.Time{}, err
	}

	// Find the longest block time that's still active
	var latestBlockUntil time.Time
	isBlocked := false

	for _, limit := range rateLimits {
		if limit.Blocked && now.Before(limit.BlockedUntil) {
			isBlocked = true
			if limit.BlockedUntil.After(latestBlockUntil) {
				latestBlockUntil = limit.BlockedUntil
			}
		}
	}

	return isBlocked, latestBlockUntil, nil
}

// IsDomainBlocked checks if a federation domain is currently blocked
func (r *RateLimitRepository) IsDomainBlocked(ctx context.Context, domain string) (bool, time.Time, error) {
	now := time.Now()

	// Check recent rate limit records for any blocks
	var rateLimits []models.APIRateLimit
	pkPrefix := fmt.Sprintf("RATELIMIT#DOMAIN#%s", domain)

	err := r.db.WithContext(ctx).Model(&models.APIRateLimit{}).
		Where("PK", "begins_with", pkPrefix).
		All(&rateLimits)

	if err != nil {
		if errors.IsNotFound(err) {
			return false, time.Time{}, nil
		}
		return false, time.Time{}, err
	}

	// Find the longest block time that's still active
	var latestBlockUntil time.Time
	isBlocked := false

	for _, limit := range rateLimits {
		if limit.Blocked && now.Before(limit.BlockedUntil) {
			isBlocked = true
			if limit.BlockedUntil.After(latestBlockUntil) {
				latestBlockUntil = limit.BlockedUntil
			}
		}
	}

	return isBlocked, latestBlockUntil, nil
}
