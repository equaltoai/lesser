package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
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
	client    *dynamodb.Client // For GSI queries that DynamORM doesn't support well
}

// NewRateLimitRepository creates a new RateLimitRepository
func NewRateLimitRepository(db core.DB, tableName string, logger *zap.Logger, client *dynamodb.Client) *RateLimitRepository {
	return &RateLimitRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
		client:    client,
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
	
	if r.client == nil {
		r.logger.Warn("no DynamoDB client available for community note rate limit check")
		// Fall back to allowing creation
		return true, limit, nil
	}
	
	result, err := r.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		IndexName:              aws.String("GSI3"),
		KeyConditionExpression: aws.String("GSI3PK = :pk AND GSI3SK > :sk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("AUTHOR#%s#NOTES", userID)},
			":sk": &types.AttributeValueMemberS{Value: yesterday.Format(time.RFC3339)},
		},
		Select: types.SelectCount,
	})
	if err != nil {
		r.logger.Error("failed to check community note rate limit",
			zap.String("userID", userID),
			zap.Int("limit", limit),
			zap.Error(err))
		return false, 0, err
	}
	
	count := int(result.Count)
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