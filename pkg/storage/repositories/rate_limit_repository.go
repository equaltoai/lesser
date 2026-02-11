package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/errors"
	"go.uber.org/zap"
)

// RateLimitRepository handles rate limiting operations using enhanced repository patterns
type RateLimitRepository struct {
	// Embedded EnhancedBaseRepository for different rate limit models
	loginAttempts       *EnhancedBaseRepository[*models.LoginAttempt]
	rateLimitLockouts   *EnhancedBaseRepository[*models.RateLimitLockout]
	apiRateLimits       *EnhancedBaseRepository[*models.APIRateLimit]
	rateLimitViolations *EnhancedBaseRepository[*models.RateLimitViolation]
	communityNotes      *EnhancedBaseRepository[*models.CommunityNote]
	db                  core.DB
	tableName           string
	logger              *zap.Logger
}

// NewRateLimitRepository creates a new RateLimitRepository with enhanced functionality
func NewRateLimitRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *RateLimitRepository {
	// Create enhanced repositories for all rate limit operations
	loginAttempts := NewEnhancedBaseRepository[*models.LoginAttempt](db, tableName, logger, costService, "LoginAttemptRepository", "login_attempt")
	loginAttempts.SetValidationService(NewDefaultValidationService())
	loginAttempts.SetPermissionService(NewDefaultPermissionService())
	loginAttempts.SetCachingService(NewInMemoryCachingService())
	loginAttempts.SetEventService(NewDefaultEventService())

	rateLimitLockouts := NewEnhancedBaseRepository[*models.RateLimitLockout](db, tableName, logger, costService, "RateLimitLockoutRepository", "rate_limit_lockout")
	rateLimitLockouts.SetValidationService(NewDefaultValidationService())
	rateLimitLockouts.SetPermissionService(NewDefaultPermissionService())
	rateLimitLockouts.SetCachingService(NewInMemoryCachingService())
	rateLimitLockouts.SetEventService(NewDefaultEventService())

	apiRateLimits := NewEnhancedBaseRepository[*models.APIRateLimit](db, tableName, logger, costService, "APIRateLimitRepository", "api_rate_limit")
	apiRateLimits.SetValidationService(NewDefaultValidationService())
	apiRateLimits.SetPermissionService(NewDefaultPermissionService())
	apiRateLimits.SetCachingService(NewInMemoryCachingService())
	apiRateLimits.SetEventService(NewDefaultEventService())

	rateLimitViolations := NewEnhancedBaseRepository[*models.RateLimitViolation](db, tableName, logger, costService, "RateLimitViolationRepository", "rate_limit_violation")
	rateLimitViolations.SetValidationService(NewDefaultValidationService())
	rateLimitViolations.SetPermissionService(NewDefaultPermissionService())
	rateLimitViolations.SetCachingService(NewInMemoryCachingService())
	rateLimitViolations.SetEventService(NewDefaultEventService())

	communityNotes := NewEnhancedBaseRepository[*models.CommunityNote](db, tableName, logger, costService, "CommunityNoteRepository", "community_note")
	communityNotes.SetValidationService(NewDefaultValidationService())
	communityNotes.SetPermissionService(NewDefaultPermissionService())
	communityNotes.SetCachingService(NewInMemoryCachingService())
	communityNotes.SetEventService(NewDefaultEventService())

	return &RateLimitRepository{
		loginAttempts:       loginAttempts,
		rateLimitLockouts:   rateLimitLockouts,
		apiRateLimits:       apiRateLimits,
		rateLimitViolations: rateLimitViolations,
		communityNotes:      communityNotes,
		db:                  db,
		tableName:           tableName,
		logger:              logger,
	}
}

// RecordLoginAttempt records a login attempt for rate limiting using BaseRepository
func (r *RateLimitRepository) RecordLoginAttempt(ctx context.Context, identifier string, success bool) error {
	attempt := models.NewLoginAttempt(identifier, success)

	err := r.loginAttempts.Create(ctx, attempt)
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
	pk := fmt.Sprintf("RATELIMIT#%s", identifier)
	sinceKey := since.Format(time.RFC3339Nano)

	const attemptsPageLimit = 200
	count := 0
	cursor := ""

	for {
		page, err := r.loginAttempts.QueryBetweenPaginated(ctx, pk, sinceKey, "~", BasePaginationOptions{
			Limit:  attemptsPageLimit,
			Cursor: cursor,
			Order:  SortOrderAsc,
		})
		if err != nil {
			r.logger.Error("failed to get login attempt count",
				zap.String("identifier", identifier),
				zap.Time("since", since),
				zap.Error(err))
			return 0, err
		}

		count += len(page.Items)
		if page.NextCursor == "" || len(page.Items) == 0 {
			break
		}
		cursor = page.NextCursor
	}
	r.logger.Debug("retrieved login attempt count",
		zap.String("identifier", identifier),
		zap.Time("since", since),
		zap.Int("count", count))

	return count, nil
}

// IsRateLimited checks if an identifier is currently rate limited using BaseRepository
func (r *RateLimitRepository) IsRateLimited(ctx context.Context, identifier string) (bool, time.Time, error) {
	pk := fmt.Sprintf("RATELIMIT#%s", identifier)
	sk := "LOCKOUT"

	lockout := &models.RateLimitLockout{}
	err := r.rateLimitLockouts.Get(ctx, pk, sk, lockout)

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

// ImposeLockout creates or updates a lockout record for the given identifier.
//
// This is used by security and governance rails (including LLM agent circuit breakers).
func (r *RateLimitRepository) ImposeLockout(ctx context.Context, identifier string, duration time.Duration) error {
	if r == nil || r.rateLimitLockouts == nil {
		return ErrorHandler.HandleCreateError(storage.ErrDatabaseConnectionFailed, EntityRateLimit, "lockout")
	}

	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return ErrorHandler.HandleCreateError(storage.ErrInvalidInput, EntityRateLimit, "lockout")
	}

	if duration <= 0 {
		duration = time.Hour
	}

	lockout := models.NewRateLimitLockout(identifier, time.Now().Add(duration))
	return r.rateLimitLockouts.Create(ctx, lockout)
}

// ClearLoginAttempts clears all login attempts for an identifier using BaseRepository
func (r *RateLimitRepository) ClearLoginAttempts(ctx context.Context, identifier string) error {
	pk := fmt.Sprintf("RATELIMIT#%s", identifier)

	// First, query all attempts for this identifier using BaseRepository
	attempts, err := r.loginAttempts.FindByPK(ctx, pk)
	if err != nil {
		r.logger.Error("failed to query login attempts for clearing",
			zap.String("identifier", identifier),
			zap.Error(err))
		return err
	}

	// Prepare keys for batch deletion
	keys := make([]struct{ PK, SK string }, len(attempts))
	for i, attempt := range attempts {
		keys[i] = struct{ PK, SK string }{
			PK: attempt.PK,
			SK: attempt.SK,
		}
	}

	// Use batch delete from BaseRepository
	if len(keys) > 0 {
		err = r.loginAttempts.BatchDelete(ctx, keys)
		if err != nil {
			r.logger.Error("failed to batch delete login attempts",
				zap.String("identifier", identifier),
				zap.Error(err))
			// Continue to try deleting lockout record
		}
	}

	// Also clear any lockout record using BaseRepository
	err = r.rateLimitLockouts.Delete(ctx, pk, "LOCKOUT")
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

// CheckCommunityNoteRateLimit checks if a user can create more community notes today using BaseRepository
func (r *RateLimitRepository) CheckCommunityNoteRateLimit(ctx context.Context, userID string, limit int) (bool, int, error) {
	// Query notes created by user in last 24 hours using GSI3
	yesterday := time.Now().Add(-24 * time.Hour)
	gsi3PK := fmt.Sprintf("AUTHOR#%s#NOTES", userID)
	gsi3SKPrefix := yesterday.Format(time.RFC3339)

	// Use BaseRepository GSI query - need to implement this manually since it's a complex GSI query
	var notes []*models.CommunityNote
	err := r.db.WithContext(ctx).Model(&models.CommunityNote{}).
		Index("gsi3").
		Where("gsi3PK", "=", gsi3PK).
		Where("gsi3SK", ">", gsi3SKPrefix).
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

// CheckAPIRateLimit checks and updates API rate limiting for a user/endpoint combination using BaseRepository
func (r *RateLimitRepository) CheckAPIRateLimit(ctx context.Context, userID, endpoint string, limit int, window time.Duration) error {
	now := time.Now()
	windowStart := now.Truncate(window)
	key := fmt.Sprintf("%s:%s", userID, endpoint)

	// Get current rate limit data using BaseRepository
	pk := fmt.Sprintf("RATELIMIT#%s", key)
	sk := fmt.Sprintf("WINDOW#%s", windowStart.Format(time.RFC3339))

	current := &models.APIRateLimit{}
	err := r.apiRateLimits.Get(ctx, pk, sk, current)

	if err != nil {
		if errors.IsNotFound(err) {
			// Create new rate limit record
			current = models.NewAPIRateLimit(userID, endpoint, windowStart)
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
		return ErrorHandler.HandleCreateError(storage.ErrRateLimited, EntityRateLimit, key)
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

		// Record the violation using BaseRepository
		violation := models.NewRateLimitViolation(userID, "", endpoint, "api", int(penaltyDuration.Minutes()))
		if err := r.rateLimitViolations.Create(ctx, violation); err != nil {
			r.logger.Error("failed to record API rate limit violation",
				zap.String("user_id", userID),
				zap.String("endpoint", endpoint),
				zap.Error(err))
		}

		// Update the record using BaseRepository
		if err := r.updateAPIRateLimit(ctx, current); err != nil {
			r.logger.Error("failed to update blocked API rate limit",
				zap.String("user_id", userID),
				zap.String("endpoint", endpoint),
				zap.Error(err))
		}

		return ErrorHandler.HandleCreateError(storage.ErrRateLimited, EntityRateLimit, key)
	}

	// Update counter using BaseRepository
	if err := r.updateAPIRateLimit(ctx, current); err != nil {
		r.logger.Error("failed to update API rate limit counter",
			zap.String("user_id", userID),
			zap.String("endpoint", endpoint),
			zap.Error(err))
		// Don't fail the request if we can't update the counter
	}

	return nil
}

// getRateLimitInfo is a helper function that consolidates the common pattern for getting rate limit info
func (r *RateLimitRepository) getRateLimitInfo(ctx context.Context, key string, limit int, window time.Duration, logContext map[string]interface{}) (remaining int, resetTime time.Time, err error) {
	now := time.Now()
	windowStart := now.Truncate(window)
	resetTime = windowStart.Add(window)

	// Get current rate limit data using BaseRepository
	pk := fmt.Sprintf("RATELIMIT#%s", key)
	sk := fmt.Sprintf("WINDOW#%s", windowStart.Format(time.RFC3339))

	current := &models.APIRateLimit{}
	err = r.apiRateLimits.Get(ctx, pk, sk, current)

	if err != nil {
		if errors.IsNotFound(err) {
			// No data yet, full limit available
			return limit, resetTime, nil
		}
		// Build log fields from context
		logFields := []zap.Field{zap.Error(err)}
		for k, v := range logContext {
			switch val := v.(type) {
			case string:
				logFields = append(logFields, zap.String(k, val))
			}
		}
		r.logger.Error("failed to get rate limit info", logFields...)
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

// GetAPIRateLimitInfo returns current rate limit info for response headers
func (r *RateLimitRepository) GetAPIRateLimitInfo(ctx context.Context, userID, endpoint string, limit int, window time.Duration) (remaining int, resetTime time.Time, err error) {
	key := fmt.Sprintf("%s:%s", userID, endpoint)
	logContext := map[string]interface{}{
		"user_id":  userID,
		"endpoint": endpoint,
	}
	return r.getRateLimitInfo(ctx, key, limit, window, logContext)
}

// CheckFixedWindowRateLimit atomically increments a counter for a fixed time window and returns the rate limit state.
//
// Unlike CheckAPIRateLimit, this helper does not apply escalating penalties or record violations. It is intended
// for strict, predictable throttles (e.g., automation safety rails).
//
// Fail-open: storage errors are returned to the caller to decide policy; callers should generally allow the request
// rather than failing user traffic due to rate limit storage outages.
func (r *RateLimitRepository) CheckFixedWindowRateLimit(ctx context.Context, identifier, bucket string, limit int, window time.Duration) (allowed bool, remaining int, resetTime time.Time, err error) {
	if r == nil || r.db == nil {
		return true, limit, time.Time{}, nil
	}

	identifier = strings.TrimSpace(identifier)
	bucket = strings.TrimSpace(bucket)
	if identifier == "" || bucket == "" || limit <= 0 {
		return true, limit, time.Time{}, nil
	}
	if window <= 0 {
		window = time.Minute
	}

	now := time.Now().UTC()
	windowStart := now.Truncate(window)
	resetTime = windowStart.Add(window)

	key := fmt.Sprintf("%s:%s", identifier, bucket)
	pk := fmt.Sprintf("RATELIMIT#%s", key)
	sk := fmt.Sprintf("WINDOW#%s", windowStart.Format(time.RFC3339))

	ttl := resetTime.Add(24 * time.Hour).Unix()

	current := &models.APIRateLimit{PK: pk, SK: sk}
	update := r.db.Model(current).WithContext(ctx).UpdateBuilder().
		SetIfNotExists("Type", nil, "APIRateLimit").
		SetIfNotExists("UserID", nil, identifier).
		SetIfNotExists("Endpoint", nil, bucket).
		SetIfNotExists("Window", nil, windowStart).
		Set("UpdatedAt", now).
		Set("TTL", ttl).
		Increment("Count").
		ReturnValues("ALL_NEW")

	if execErr := update.ExecuteWithResult(current); execErr != nil {
		return true, limit, resetTime, execErr
	}

	// Some executors may not support returning results; fall back to a read for the updated count.
	if current.Count <= 0 && r.apiRateLimits != nil {
		fetched := &models.APIRateLimit{}
		if getErr := r.apiRateLimits.Get(ctx, pk, sk, fetched); getErr != nil {
			return true, limit, resetTime, getErr
		}
		current = fetched
	}

	if current.Count <= limit {
		allowed = true
		remaining = limit - current.Count
	} else {
		allowed = false
		remaining = 0
	}

	return allowed, remaining, resetTime, nil
}

// updateAPIRateLimit updates an API rate limit record using BaseRepository
func (r *RateLimitRepository) updateAPIRateLimit(ctx context.Context, limit *models.APIRateLimit) error {
	// Use BaseRepository Create method which acts like PUT in DynamoDB
	err := r.apiRateLimits.Create(ctx, limit)
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

	// Get current rate limit data using BaseRepository
	pk := fmt.Sprintf("RATELIMIT#%s", key)
	sk := fmt.Sprintf("WINDOW#%s", windowStart.Format(time.RFC3339))

	current := &models.APIRateLimit{}
	err := r.apiRateLimits.Get(ctx, pk, sk, current)

	if err != nil {
		if errors.IsNotFound(err) {
			// Create new rate limit record
			current = models.NewFederationRateLimit(domain, endpoint, windowStart)
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
		return ErrorHandler.HandleCreateError(storage.ErrRateLimited, EntityRateLimit, domain)
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

		// Record the violation using BaseRepository
		violation := models.NewRateLimitViolation("", domain, endpoint, "federation", int(penaltyDuration.Minutes()))
		if err := r.rateLimitViolations.Create(ctx, violation); err != nil {
			r.logger.Error("failed to record federation rate limit violation",
				zap.String("domain", domain),
				zap.String("endpoint", endpoint),
				zap.Error(err))
		}

		// Update the rate limit record using BaseRepository
		if err := r.updateAPIRateLimit(ctx, current); err != nil {
			r.logger.Error("failed to update blocked federation rate limit",
				zap.String("domain", domain),
				zap.String("endpoint", endpoint),
				zap.Error(err))
		}

		return ErrorHandler.HandleCreateError(storage.ErrRateLimited, EntityRateLimit, domain)
	}

	// Update counter using BaseRepository
	if err := r.updateAPIRateLimit(ctx, current); err != nil {
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
	key := fmt.Sprintf("DOMAIN#%s:%s", domain, endpoint)
	logContext := map[string]interface{}{
		"domain":   domain,
		"endpoint": endpoint,
	}
	return r.getRateLimitInfo(ctx, key, limit, window, logContext)
}

// GetViolationCount returns the number of violations in a time period for escalating penalties
func (r *RateLimitRepository) GetViolationCount(ctx context.Context, userID, domain string, since time.Duration) (int, error) {
	identifier := userID
	if domain != "" {
		identifier = fmt.Sprintf("DOMAIN#%s", domain)
	}

	pk := fmt.Sprintf("RATELIMIT_VIOLATION#%s", identifier)
	sinceKey := time.Now().Add(-since).Format(time.RFC3339Nano)

	const violationPageLimit = 200
	count := 0
	cursor := ""

	for {
		page, err := r.rateLimitViolations.QueryBetweenPaginated(ctx, pk, sinceKey, "~", BasePaginationOptions{
			Limit:  violationPageLimit,
			Cursor: cursor,
			Order:  SortOrderAsc,
		})
		if err != nil {
			r.logger.Error("failed to get violation count",
				zap.String("user_id", userID),
				zap.String("domain", domain),
				zap.Duration("since", since),
				zap.Error(err))
			return 0, err
		}

		count += len(page.Items)
		if page.NextCursor == "" || len(page.Items) == 0 {
			break
		}
		cursor = page.NextCursor
	}

	return count, nil
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

// checkBlockedStatus is a helper function that consolidates the common pattern for checking if something is blocked
func (r *RateLimitRepository) checkBlockedStatus(ctx context.Context, pkQuery string, usePrefix bool) (bool, time.Time, error) {
	now := time.Now()

	// Check recent rate limit records for any blocks using BaseRepository
	var rateLimits []*models.APIRateLimit
	var err error

	if usePrefix {
		// Use BaseRepository to query with PK prefix - need manual query since BaseRepository doesn't have prefix query
		err = r.db.WithContext(ctx).Model(&models.APIRateLimit{}).
			Where("PK", "begins_with", pkQuery).
			All(&rateLimits)
	} else {
		// Use BaseRepository to query exact PK
		rateLimits, err = r.apiRateLimits.FindByPK(ctx, pkQuery)
	}

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

// IsUserBlocked checks if a user is currently blocked due to rate limiting
func (r *RateLimitRepository) IsUserBlocked(ctx context.Context, userID string) (bool, time.Time, error) {
	pk := fmt.Sprintf("RATELIMIT#%s", userID)
	return r.checkBlockedStatus(ctx, pk, false)
}

// IsDomainBlocked checks if a federation domain is currently blocked
func (r *RateLimitRepository) IsDomainBlocked(ctx context.Context, domain string) (bool, time.Time, error) {
	pkPrefix := fmt.Sprintf("RATELIMIT#DOMAIN#%s", domain)
	return r.checkBlockedStatus(ctx, pkPrefix, true)
}
