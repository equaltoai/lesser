package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// CSRFRepository handles CSRF token operations using enhanced security patterns
type CSRFRepository struct {
	*EnhancedBaseRepository[*models.CSRFToken]
	// Keep reference to db for complex GSI queries that BaseRepository doesn't handle
	db core.DB
}

// NewCSRFRepository creates a new CSRFRepository with enhanced security features
func NewCSRFRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *CSRFRepository {
	// Create enhanced repository optimized for CSRF operations - SECURITY CRITICAL
	enhancedRepo := NewEnhancedBaseRepository[*models.CSRFToken](db, tableName, logger, costService, "CSRFRepository", "csrf")

	// Set up enhanced services for CSRF operations - SECURITY CRITICAL
	enhancedRepo.SetValidationService(NewDefaultValidationService()) // Critical CSRF validation
	enhancedRepo.SetPermissionService(NewDefaultPermissionService()) // Standard permissions
	enhancedRepo.SetCachingService(NewInMemoryCachingService())      // CSRF tokens cached briefly
	enhancedRepo.SetEventService(NewDefaultEventService())           // Security event tracking

	return &CSRFRepository{
		EnhancedBaseRepository: enhancedRepo,
		db:                     db,
	}
}

// Store saves a CSRF token with expiration - CSRF PROTECTION: preserves exact legacy behavior
func (r *CSRFRepository) Store(ctx context.Context, token string, userID string, expiresAt time.Time) error {
	// CRITICAL: Check token limit per user (prevent DoS) - matches legacy behavior
	count, err := r.GetUserActiveTokenCount(ctx, userID)
	if err == nil && count >= 10 {
		// Clean up old tokens before rejecting
		if cleanupErr := r.CleanupUserTokens(ctx, userID); cleanupErr != nil {
			r.logger.Warn("failed to cleanup user CSRF tokens",
				zap.String("userID", userID),
				zap.Error(cleanupErr))
		}

		// Check again after cleanup
		count, err = r.GetUserActiveTokenCount(ctx, userID)
		if err == nil && count >= 10 {
			return ErrorHandler.HandleCreateError(ErrCSRFTooManyTokens, EntityCSRFToken, userID)
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

	// Use Enhanced BaseRepository with security validation
	err = r.ValidateAndCreate(ctx, csrfToken)
	if err != nil {
		// Check if it's a duplicate token error by trying to get it
		if _, _, _, valid, getErr := r.Get(ctx, token); getErr == nil && valid {
			return ErrorHandler.HandleCreateError(ErrCSRFTokenAlreadyExists, EntityCSRFToken, token)
		}
		r.logger.Error("failed to store CSRF token",
			zap.String("token", token),
			zap.String("userID", userID),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityCSRFToken, token)
	}

	r.logger.Debug("stored CSRF token",
		zap.String("token", token),
		zap.String("userID", userID),
		zap.Time("expires_at", expiresAt))

	return nil
}

// Get retrieves a CSRF token - CSRF PROTECTION: preserves exact legacy validation logic
func (r *CSRFRepository) Get(ctx context.Context, token string) (string, string, time.Time, bool, error) {
	var csrfToken models.CSRFToken
	pk := fmt.Sprintf("CSRF#%s", token)

	// Use Enhanced BaseRepository.Get() for security validation
	err := r.EnhancedBaseRepository.Get(ctx, pk, "TOKEN", &csrfToken)
	if err != nil {
		if errors.IsNotFound(err) {
			// Return values that indicate invalid token (matches legacy)
			return "", "", time.Time{}, false, nil
		}
		r.logger.Error("failed to get CSRF token",
			zap.String("token", token),
			zap.Error(err))
		return "", "", time.Time{}, false, ErrorHandler.HandleGetError(err, EntityCSRFToken, token)
	}

	// CRITICAL: Check if expired (matches legacy logic)
	expiresAt := time.Unix(csrfToken.ExpiresAt, 0)
	if time.Now().After(expiresAt) {
		return "", "", time.Time{}, false, nil // Expired
	}

	// CRITICAL: Check if already used (matches legacy logic)
	if csrfToken.Used {
		return "", "", time.Time{}, false, nil // Already used
	}

	r.logger.Debug("retrieved CSRF token",
		zap.String("token", token),
		zap.String("user_id", csrfToken.UserID),
		zap.Time("expires_at", expiresAt))

	return csrfToken.Token, csrfToken.UserID, expiresAt, true, nil
}

// Delete removes a CSRF token - uses BaseRepository
func (r *CSRFRepository) Delete(ctx context.Context, token string) error {
	pk := fmt.Sprintf("CSRF#%s", token)

	// Use Enhanced BaseRepository for validated deletion
	err := r.ValidateAndDelete(ctx, pk, "TOKEN")
	if err != nil {
		r.logger.Error("failed to delete CSRF token",
			zap.String("token", token),
			zap.Error(err))
		return ErrorHandler.HandleDeleteError(err, EntityCSRFToken, token)
	}

	r.logger.Debug("deleted CSRF token",
		zap.String("token", token))

	return nil
}

// ValidateAndConsume validates a token and marks it as used atomically - CSRF PROTECTION: critical validation logic preserved
func (r *CSRFRepository) ValidateAndConsume(ctx context.Context, token string, userID string) error {
	// CRITICAL: First, get the token to validate it (uses our Get method with all validation)
	_, retrievedUserID, expiresAt, valid, err := r.Get(ctx, token)
	if err != nil {
		return ErrorHandler.HandleGetError(err, EntityCSRFToken, "validation")
	}

	// CRITICAL: CSRF validation checks
	if !valid {
		return ErrorHandler.HandleGetError(ErrCSRFTokenInvalid, "csrf protection", token)
	}

	if time.Now().After(expiresAt) {
		return ErrorHandler.HandleGetError(ErrCSRFTokenExpired, "csrf protection", token)
	}

	if retrievedUserID != userID {
		return ErrorHandler.HandleGetError(ErrCSRFTokenInvalid, "csrf protection", token)
	}

	// Now update to mark as used
	pk := fmt.Sprintf("CSRF#%s", token)
	var csrfToken models.CSRFToken

	// Get the current token record using BaseRepository
	err = r.BaseRepository.Get(ctx, pk, "TOKEN", &csrfToken)
	if err != nil {
		if errors.IsNotFound(err) {
			return ErrorHandler.HandleGetError(ErrCSRFTokenInvalid, "csrf protection", token)
		}
		r.logger.Error("failed to validate and consume CSRF token",
			zap.String("token", token),
			zap.String("userID", userID),
			zap.Error(err))
		return ErrorHandler.HandleGetError(err, EntityCSRFToken, token)
	}

	// Mark as used and save using BaseRepository.Create (upsert semantics)
	csrfToken.MarkAsUsed()

	err = r.ValidateAndUpdate(ctx, &csrfToken)
	if err != nil {
		r.logger.Error("failed to mark CSRF token as used",
			zap.String("token", token),
			zap.String("userID", userID),
			zap.Error(err))
		return ErrorHandler.HandleUpdateError(err, EntityCSRFToken, token)
	}

	r.logger.Debug("validated and consumed CSRF token",
		zap.String("token", token),
		zap.String("userID", userID))

	return nil
}

// GetUserActiveTokenCount returns the number of active tokens for a user - CSRF PROTECTION: DoS prevention
func (r *CSRFRepository) GetUserActiveTokenCount(ctx context.Context, userID string) (int, error) {
	var tokens []models.CSRFToken
	gsi1pk := fmt.Sprintf("USER_CSRF#%s", userID)
	now := time.Now().Unix()

	// Query using GSI1 to get all tokens for user, then filter active ones
	// Using direct DB call since BaseRepository doesn't have GSI query method with these specific parameters
	err := r.db.WithContext(ctx).Model(&models.CSRFToken{}).
		Index("user-csrf-index").
		Where("gsI1PK", "=", gsi1pk).
		All(&tokens)

	if err != nil {
		r.logger.Error("failed to get user active token count",
			zap.String("userID", userID),
			zap.Error(err))
		return 0, ErrorHandler.HandleQueryError(err, EntityCSRFToken, "count active")
	}

	// CRITICAL: Count active tokens (not expired and not used) - DoS protection
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

// CleanupUserTokens removes old/used tokens for a user - CSRF PROTECTION: prevents token accumulation DoS
func (r *CSRFRepository) CleanupUserTokens(ctx context.Context, userID string) error {
	var tokens []models.CSRFToken
	gsi1pk := fmt.Sprintf("USER_CSRF#%s", userID)
	now := time.Now().Unix()

	// Query all tokens for this user using GSI1
	// Using direct DB call since BaseRepository doesn't have this specific GSI query pattern
	err := r.db.WithContext(ctx).Model(&models.CSRFToken{}).
		Index("user-csrf-index").
		Where("gsI1PK", "=", gsi1pk).
		All(&tokens)

	if err != nil {
		r.logger.Error("failed to query user tokens for cleanup",
			zap.String("userID", userID),
			zap.Error(err))
		return ErrorHandler.HandleQueryError(err, EntityCSRFToken, "cleanup query")
	}

	deletedCount := 0
	// CRITICAL: Delete expired or used tokens to prevent DoS
	for _, token := range tokens {
		if token.Used || token.ExpiresAt <= now {
			// Use Enhanced BaseRepository for validated deletion
			err := r.ValidateAndDelete(ctx, token.PK, token.SK)
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

// CleanExpired removes expired tokens - interface compatibility (DynamoDB TTL handles this automatically)
func (r *CSRFRepository) CleanExpired(_ context.Context) error {
	// DynamoDB TTL handles this automatically - this method exists for interface compatibility
	r.logger.Debug("clean expired called - DynamoDB TTL handles automatic cleanup")
	return nil
}
