package repositories

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// AuthRefreshTokenRepository handles auth refresh tokens with enhanced security features
type AuthRefreshTokenRepository struct {
	*EnhancedBaseRepository[*models.AuthRefreshToken]
}

// NewAuthRefreshTokenRepository creates a new auth refresh token repository with enhanced security
func NewAuthRefreshTokenRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *AuthRefreshTokenRepository {
	// Create enhanced repository optimized for auth token operations
	enhancedRepo := NewEnhancedBaseRepository[*models.AuthRefreshToken](db, tableName, logger, costService, "AuthRefreshTokenRepository", "auth_token")

	// Set up enhanced services for auth token operations - SECURITY CRITICAL
	enhancedRepo.SetValidationService(NewDefaultValidationService()) // Critical token validation
	enhancedRepo.SetPermissionService(NewDefaultPermissionService()) // Auth-specific permissions
	enhancedRepo.SetCachingService(NewInMemoryCachingService())      // Short-term token caching
	enhancedRepo.SetEventService(NewDefaultEventService())           // Security event tracking

	return &AuthRefreshTokenRepository{
		EnhancedBaseRepository: enhancedRepo,
	}
}

// CreateRefreshToken generates and stores a new refresh token
func (r *AuthRefreshTokenRepository) CreateRefreshToken(ctx context.Context, userID string, deviceName string, ipAddress string) (*models.AuthRefreshToken, error) {
	tokenBytes := make([]byte, 32)
	familyBytes := make([]byte, 16)

	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, ErrorHandler.HandleCreateError(err, EntityRefreshToken, "token generation")
	}
	if _, err := rand.Read(familyBytes); err != nil {
		return nil, ErrorHandler.HandleCreateError(err, EntityRefreshToken, "family generation")
	}

	now := time.Now()
	token := &models.AuthRefreshToken{
		Token:      base64.URLEncoding.EncodeToString(tokenBytes),
		UserID:     userID,
		Family:     base64.URLEncoding.EncodeToString(familyBytes),
		Generation: 1,
		CreatedAt:  now.Unix(),
		ExpiresAt:  now.Add(30 * 24 * time.Hour).Unix(), // 30 days
		Revoked:    false,
		DeviceName: deviceName,
		IPAddress:  ipAddress,
	}

	// Create the token using Enhanced BaseRepository with security validation
	err := r.ValidateAndCreate(ctx, token)
	if err != nil {
		return nil, ErrorHandler.HandleCreateError(err, EntityRefreshToken, token.Token)
	}

	r.logger.Info("Created new auth refresh token",
		zap.String("user_id", userID),
		zap.String("family", token.Family),
		zap.String("device", deviceName),
		zap.String("ip", ipAddress))

	return token, nil
}

// GetRefreshToken retrieves a refresh token by token value
func (r *AuthRefreshTokenRepository) GetRefreshToken(ctx context.Context, token string) (*models.AuthRefreshToken, error) {
	var authToken models.AuthRefreshToken

	// Use BaseRepository.Get to retrieve the token
	err := r.Get(ctx, token, models.SKToken, &authToken)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(storage.ErrInvalidRefreshToken, EntityRefreshToken, token)
		}
		return nil, ErrorHandler.HandleGetError(err, EntityRefreshToken, token)
	}

	// Check expiration - CRITICAL AUTHENTICATION SECURITY
	if authToken.IsExpired() {
		return nil, ErrorHandler.HandleGetError(storage.ErrExpiredRefreshToken, EntityRefreshToken, token)
	}

	return &authToken, nil
}

// RotateRefreshToken implements secure rotation with reuse detection
func (r *AuthRefreshTokenRepository) RotateRefreshToken(ctx context.Context, oldTokenValue string, ipAddress string) (*models.AuthRefreshToken, error) {
	// Get the old token
	oldToken, err := r.GetRefreshToken(ctx, oldTokenValue)
	if err != nil {
		return nil, err
	}

	// Check if token was already used (reuse detection)
	if oldToken.Revoked {
		// SECURITY ALERT: Token reuse detected!
		// Revoke entire family
		if err := r.RevokeTokenFamily(ctx, oldToken.Family, "Token reuse detected"); err != nil {
			r.logger.Error("Failed to revoke token family after reuse detection",
				zap.String("family", oldToken.Family),
				zap.Error(err))
		}

		// Log security event
		common.LogSecurityEvent(common.EventTokenReuse,
			zap.String("user_id", oldToken.UserID),
			zap.String("family", oldToken.Family),
			zap.String("token", oldTokenValue),
			zap.String("ip", ipAddress),
			zap.Int("generation", oldToken.Generation))

		return nil, ErrorHandler.HandleUpdateError(storage.ErrTokenReuse, EntityRefreshToken, oldTokenValue)
	}

	// Create new token in same family
	newTokenBytes := make([]byte, 32)
	if _, err := rand.Read(newTokenBytes); err != nil {
		return nil, ErrorHandler.HandleCreateError(err, EntityRefreshToken, "new token generation")
	}

	now := time.Now()
	newToken := &models.AuthRefreshToken{
		Token:      base64.URLEncoding.EncodeToString(newTokenBytes),
		UserID:     oldToken.UserID,
		Family:     oldToken.Family,
		Generation: oldToken.Generation + 1,
		CreatedAt:  now.Unix(),
		ExpiresAt:  now.Add(30 * 24 * time.Hour).Unix(),
		Revoked:    false,
		DeviceName: oldToken.DeviceName,
		IPAddress:  ipAddress,
	}

	// Use DynamORM transaction to ensure atomicity
	err = r.db.Transaction(func(tx *core.Tx) error {
		// Step 1: Revoke old token
		oldToken.Revoked = true
		oldToken.RevokedReason = "Rotated"
		oldToken.LastUsedAt = now.Unix()

		if err := tx.Model(oldToken).Update(); err != nil {
			return ErrorHandler.HandleUpdateError(err, EntityRefreshToken, oldToken.Token)
		}

		// Step 2: Create new token
		if err := tx.Model(newToken).Create(); err != nil {
			return ErrorHandler.HandleCreateError(err, EntityRefreshToken, newToken.Token)
		}

		return nil
	})
	if err != nil {
		return nil, ErrorHandler.HandleUpdateError(err, EntityRefreshToken, "rotation")
	}

	r.logger.Info("Rotated auth refresh token",
		zap.String("user_id", newToken.UserID),
		zap.String("family", newToken.Family),
		zap.Int("generation", newToken.Generation),
		zap.String("ip", ipAddress))

	return newToken, nil
}

// RevokeTokenFamily revokes all tokens in a family (security breach response)
func (r *AuthRefreshTokenRepository) RevokeTokenFamily(ctx context.Context, family string, reason string) error {
	r.logger.Warn("Revoking token family due to security event",
		zap.String("family", family),
		zap.String("reason", reason))

	// Get all tokens in the family
	tokens, err := r.GetTokensByFamily(ctx, family)
	if err != nil {
		return ErrorHandler.HandleQueryError(err, EntityRefreshToken, "family tokens")
	}

	if err := common.ValidateSliceNotEmpty("tokens", tokens); err != nil {
		r.logger.Debug("No tokens found for family", zap.String("family", family))
		return nil
	}

	// Use transaction to revoke all tokens atomically
	err = r.db.Transaction(func(tx *core.Tx) error {
		now := time.Now().Unix()

		for _, token := range tokens {
			if !token.Revoked {
				token.Revoked = true
				token.RevokedReason = reason
				token.LastUsedAt = now

				if err := tx.Model(&token).Update(); err != nil {
					return ErrorHandler.HandleUpdateError(err, EntityRefreshToken, token.Token)
				}
			}
		}

		return nil
	})

	if err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityRefreshToken, "family revocation")
	}

	// Log security event
	common.LogSecurityEvent(common.EventTokenFamilyRevoked,
		zap.String("family", family),
		zap.String("reason", reason),
		zap.Int("tokens_revoked", len(tokens)))

	r.logger.Info("Successfully revoked token family",
		zap.String("family", family),
		zap.String("reason", reason),
		zap.Int("tokens_revoked", len(tokens)))

	return nil
}

// RevokeUserTokens revokes all tokens for a user (logout all devices)
func (r *AuthRefreshTokenRepository) RevokeUserTokens(ctx context.Context, userID string, reason string) error {
	r.logger.Info("Revoking all tokens for user",
		zap.String("user_id", userID),
		zap.String("reason", reason))

	// Get all tokens for the user (both active and inactive for complete revocation)
	var tokens []models.AuthRefreshToken
	err := r.db.WithContext(ctx).Model(&models.AuthRefreshToken{}).
		Where("UserID", "=", userID).
		All(&tokens)

	if err != nil {
		if errors.IsNotFound(err) {
			r.logger.Debug("No tokens found for user", zap.String("user_id", userID))
			return nil
		}
		return ErrorHandler.HandleQueryError(err, EntityRefreshToken, "user tokens")
	}

	if err := common.ValidateSliceNotEmpty("tokens", tokens); err != nil {
		r.logger.Debug("No tokens found for user", zap.String("user_id", userID))
		return nil
	}

	// Use transaction to revoke all tokens atomically
	tokensToRevoke := 0
	err = r.db.Transaction(func(tx *core.Tx) error {
		now := time.Now().Unix()

		for _, token := range tokens {
			if !token.Revoked {
				tokensToRevoke++
				token.Revoked = true
				token.RevokedReason = reason
				token.LastUsedAt = now

				if err := tx.Model(&token).Update(); err != nil {
					return ErrorHandler.HandleUpdateError(err, EntityRefreshToken, token.Token)
				}
			}
		}

		return nil
	})

	if err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityRefreshToken, "user revocation")
	}

	// Log security event
	common.LogSecurityEvent(common.EventUserTokensRevoked,
		zap.String("user_id", userID),
		zap.String("reason", reason),
		zap.Int("tokens_revoked", tokensToRevoke))

	r.logger.Info("Successfully revoked user tokens",
		zap.String("user_id", userID),
		zap.String("reason", reason),
		zap.Int("tokens_revoked", tokensToRevoke))

	return nil
}

// GetTokensByUser retrieves all active tokens for a user
func (r *AuthRefreshTokenRepository) GetTokensByUser(ctx context.Context, userID string) ([]models.AuthRefreshToken, error) {
	r.logger.Debug("Getting tokens by user",
		zap.String("user_id", userID))

	var tokens []models.AuthRefreshToken
	err := r.db.WithContext(ctx).Model(&models.AuthRefreshToken{}).
		Where("UserID", "=", userID).
		All(&tokens)

	if err != nil {
		if errors.IsNotFound(err) {
			return []models.AuthRefreshToken{}, nil
		}
		return nil, ErrorHandler.HandleQueryError(err, EntityRefreshToken, "user tokens")
	}

	// Filter to only active tokens - CRITICAL AUTHENTICATION SECURITY
	var activeTokens []models.AuthRefreshToken
	for _, token := range tokens {
		if token.IsActive() {
			activeTokens = append(activeTokens, token)
		}
	}

	r.logger.Debug("Retrieved active tokens for user",
		zap.String("user_id", userID),
		zap.Int("total_tokens", len(tokens)),
		zap.Int("active_tokens", len(activeTokens)))

	return activeTokens, nil
}

// GetTokensByFamily retrieves all tokens in a family
func (r *AuthRefreshTokenRepository) GetTokensByFamily(ctx context.Context, family string) ([]models.AuthRefreshToken, error) {
	r.logger.Debug("Getting tokens by family",
		zap.String("family", family))

	var tokens []models.AuthRefreshToken
	err := r.db.WithContext(ctx).Model(&models.AuthRefreshToken{}).
		Where("Family", "=", family).
		All(&tokens)

	if err != nil {
		if errors.IsNotFound(err) {
			return []models.AuthRefreshToken{}, nil
		}
		return nil, ErrorHandler.HandleQueryError(err, EntityRefreshToken, "family tokens")
	}

	r.logger.Debug("Retrieved tokens for family",
		zap.String("family", family),
		zap.Int("token_count", len(tokens)))

	return tokens, nil
}
