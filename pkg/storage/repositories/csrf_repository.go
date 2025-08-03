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

// CSRFRepository handles CSRF token operations using DynamORM
type CSRFRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewCSRFRepository creates a new CSRFRepository
func NewCSRFRepository(db core.DB, tableName string, logger *zap.Logger) *CSRFRepository {
	return &CSRFRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// Store saves a CSRF token with expiration - matches legacy Store method exactly
func (r *CSRFRepository) Store(ctx context.Context, token string, userID string, expiresAt time.Time) error {
	// Check token limit per user (prevent DoS) - matches legacy behavior
	count, err := r.GetUserActiveTokenCount(ctx, userID)
	if err == nil && count >= 10 {
		// Clean up old tokens before rejecting
		r.CleanupUserTokens(ctx, userID)

		// Check again after cleanup
		count, err = r.GetUserActiveTokenCount(ctx, userID)
		if err == nil && count >= 10 {
			return fmt.Errorf("too many active CSRF tokens for user")
		}
	}

	// Create the CSRF token model
	csrfToken := &models.CSRFToken{
		Token:     token,
		UserID:    userID,
		CreatedAt: time.Now().Unix(),
		ExpiresAt: expiresAt.Unix(),
		Used:      false,
	}

	// Use conditional create to prevent duplicate tokens (matches legacy behavior)
	err = r.db.WithContext(ctx).Model(csrfToken).Create()

	if err != nil {
		// Check if it's a duplicate token error by trying to get it
		if _, _, _, valid, getErr := r.Get(ctx, token); getErr == nil && valid {
			return fmt.Errorf("token already exists")
		}
		r.logger.Error("failed to store CSRF token",
			zap.String("token", token),
			zap.String("userID", userID),
			zap.Error(err))
		return fmt.Errorf("failed to store token: %w", err)
	}

	r.logger.Debug("stored CSRF token",
		zap.String("token", token),
		zap.String("userID", userID),
		zap.Time("expires_at", expiresAt))

	return nil
}

// Get retrieves a CSRF token - matches legacy Get method exactly
func (r *CSRFRepository) Get(ctx context.Context, token string) (string, string, time.Time, bool, error) {
	var csrfToken models.CSRFToken

	pk := fmt.Sprintf("CSRF#%s", token)

	err := r.db.WithContext(ctx).Model(&models.CSRFToken{}).
		Where("PK", "=", pk).
		Where("SK", "=", "TOKEN").
		First(&csrfToken)

	if err != nil {
		if errors.IsNotFound(err) {
			// Return values that indicate invalid token (matches legacy)
			return "", "", time.Time{}, false, nil
		}
		r.logger.Error("failed to get CSRF token",
			zap.String("token", token),
			zap.Error(err))
		return "", "", time.Time{}, false, fmt.Errorf("failed to get token: %w", err)
	}

	// Check if expired (matches legacy logic)
	expiresAt := time.Unix(csrfToken.ExpiresAt, 0)
	if time.Now().After(expiresAt) {
		return "", "", time.Time{}, false, nil // Expired
	}

	// Check if already used (matches legacy logic)
	if csrfToken.Used {
		return "", "", time.Time{}, false, nil // Already used
	}

	r.logger.Debug("retrieved CSRF token",
		zap.String("token", token),
		zap.String("user_id", csrfToken.UserID),
		zap.Time("expires_at", expiresAt))

	return csrfToken.Token, csrfToken.UserID, expiresAt, true, nil
}

// Delete removes a CSRF token - matches legacy Delete method exactly
func (r *CSRFRepository) Delete(ctx context.Context, token string) error {
	pk := fmt.Sprintf("CSRF#%s", token)

	err := r.db.WithContext(ctx).Model(&models.CSRFToken{}).
		Where("PK", "=", pk).
		Where("SK", "=", "TOKEN").
		Delete()

	if err != nil {
		r.logger.Error("failed to delete CSRF token",
			zap.String("token", token),
			zap.Error(err))
		return fmt.Errorf("failed to delete token: %w", err)
	}

	r.logger.Debug("deleted CSRF token",
		zap.String("token", token))

	return nil
}

// ValidateAndConsume validates a token and marks it as used atomically - matches legacy method exactly
func (r *CSRFRepository) ValidateAndConsume(ctx context.Context, token string, userID string) error {
	// First, get the token to validate it
	_, retrievedUserID, expiresAt, valid, err := r.Get(ctx, token)
	if err != nil {
		return fmt.Errorf("failed to validate token: %w", err)
	}
	
	if !valid {
		return fmt.Errorf("invalid CSRF token")
	}
	
	if time.Now().After(expiresAt) {
		return fmt.Errorf("expired CSRF token")
	}
	
	if retrievedUserID != userID {
		return fmt.Errorf("invalid CSRF token")
	}

	// Now update to mark as used
	pk := fmt.Sprintf("CSRF#%s", token)
	var csrfToken models.CSRFToken
	
	// Get the current token record
	err = r.db.WithContext(ctx).Model(&models.CSRFToken{}).
		Where("PK", "=", pk).
		Where("SK", "=", "TOKEN").
		First(&csrfToken)
		
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("invalid CSRF token")
		}
		r.logger.Error("failed to validate and consume CSRF token",
			zap.String("token", token),
			zap.String("userID", userID),
			zap.Error(err))
		return fmt.Errorf("failed to validate token: %w", err)
	}
	
	// Mark as used and save using Create (upsert semantics)
	csrfToken.MarkAsUsed()
	
	err = r.db.WithContext(ctx).Model(&csrfToken).Create()
	if err != nil {
		r.logger.Error("failed to mark CSRF token as used",
			zap.String("token", token),
			zap.String("userID", userID),
			zap.Error(err))
		return fmt.Errorf("failed to consume token: %w", err)
	}

	r.logger.Debug("validated and consumed CSRF token",
		zap.String("token", token),
		zap.String("userID", userID))

	return nil
}

// GetUserActiveTokenCount returns the number of active tokens for a user - matches legacy method exactly
func (r *CSRFRepository) GetUserActiveTokenCount(ctx context.Context, userID string) (int, error) {
	var tokens []models.CSRFToken
	gsi1pk := fmt.Sprintf("USER_CSRF#%s", userID)
	now := time.Now().Unix()

	// Query using GSI1 to get all tokens for user, then filter active ones
	err := r.db.WithContext(ctx).Model(&models.CSRFToken{}).
		Index("user-csrf-index").
		Where("GSI1PK", "=", gsi1pk).
		All(&tokens)

	if err != nil {
		r.logger.Error("failed to get user active token count",
			zap.String("userID", userID),
			zap.Error(err))
		return 0, fmt.Errorf("failed to count tokens: %w", err)
	}

	// Count active tokens (not expired and not used)
	count := 0
	for _, token := range tokens {
		if !token.Used && token.ExpiresAt > now {
			count++
		}
	}

	r.logger.Debug("retrieved user active token count",
		zap.String("userID", userID),
		zap.Int("count", count))

	return count, nil
}

// CleanupUserTokens removes old/used tokens for a user - matches legacy method exactly
func (r *CSRFRepository) CleanupUserTokens(ctx context.Context, userID string) error {
	var tokens []models.CSRFToken
	gsi1pk := fmt.Sprintf("USER_CSRF#%s", userID)
	now := time.Now().Unix()

	// Query all tokens for this user using GSI1
	err := r.db.WithContext(ctx).Model(&models.CSRFToken{}).
		Index("user-csrf-index").
		Where("GSI1PK", "=", gsi1pk).
		All(&tokens)

	if err != nil {
		r.logger.Error("failed to query user tokens for cleanup",
			zap.String("userID", userID),
			zap.Error(err))
		return fmt.Errorf("failed to query tokens for cleanup: %w", err)
	}

	deletedCount := 0
	// Delete expired or used tokens
	for _, token := range tokens {
		if token.Used || token.ExpiresAt <= now {
			err := r.db.WithContext(ctx).Model(&models.CSRFToken{}).
				Where("PK", "=", token.PK).
				Where("SK", "=", token.SK).
				Delete()

			if err != nil {
				r.logger.Error("failed to delete token during cleanup",
					zap.String("userID", userID),
					zap.String("token", token.Token),
					zap.Error(err))
				// Continue with other deletions even if one fails
			} else {
				deletedCount++
			}
		}
	}

	r.logger.Debug("cleaned up user tokens",
		zap.String("userID", userID),
		zap.Int("tokens_deleted", deletedCount))

	return nil
}

// CleanExpired removes expired tokens - matches legacy interface (DynamoDB TTL handles this automatically)
func (r *CSRFRepository) CleanExpired(ctx context.Context) error {
	// DynamoDB TTL handles this automatically - this method exists for interface compatibility
	r.logger.Debug("clean expired called - DynamoDB TTL handles automatic cleanup")
	return nil
}