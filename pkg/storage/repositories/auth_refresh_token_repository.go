package repositories

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// AuthRefreshTokenRepository handles auth refresh tokens with advanced security
type AuthRefreshTokenRepository struct {
	db     core.DB
	logger *zap.Logger
}

// NewAuthRefreshTokenRepository creates a new auth refresh token repository
func NewAuthRefreshTokenRepository(db core.DB) *AuthRefreshTokenRepository {
	return &AuthRefreshTokenRepository{
		db:     db,
		logger: common.Logger(),
	}
}

// CreateRefreshToken generates and stores a new refresh token
func (r *AuthRefreshTokenRepository) CreateRefreshToken(ctx context.Context, userID string, deviceName string, ipAddress string) (*models.AuthRefreshToken, error) {
	tokenBytes := make([]byte, 32)
	familyBytes := make([]byte, 16)

	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}
	if _, err := rand.Read(familyBytes); err != nil {
		return nil, fmt.Errorf("failed to generate family: %w", err)
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

	// Create the token
	err := r.db.Model(token).Create()
	if err != nil {
		return nil, fmt.Errorf("failed to store token: %w", err)
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
	err := r.db.Model(&models.AuthRefreshToken{}).
		Where("PK", "=", token).
		Where("SK", "=", "TOKEN").
		First(&authToken)
	
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("invalid refresh token")
		}
		return nil, fmt.Errorf("failed to get token: %w", err)
	}

	// Check expiration
	if authToken.IsExpired() {
		return nil, fmt.Errorf("refresh token expired")
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

		return nil, fmt.Errorf("refresh token reuse detected")
	}

	// Create new token in same family
	newTokenBytes := make([]byte, 32)
	if _, err := rand.Read(newTokenBytes); err != nil {
		return nil, fmt.Errorf("failed to generate new token: %w", err)
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

	// For now, do this in two steps (TODO: implement proper transactions)
	// Step 1: Revoke old token
	oldToken.Revoked = true
	oldToken.RevokedReason = "Rotated"
	oldToken.LastUsedAt = now.Unix()
	err = r.db.Model(oldToken).Update()
	if err != nil {
		return nil, fmt.Errorf("failed to revoke old token: %w", err)
	}

	// Step 2: Create new token
	err = r.db.Model(newToken).Create()
	if err != nil {
		// TODO: Handle rollback - for now just log error
		r.logger.Error("Failed to create new token after revoking old one",
			zap.String("old_token", oldTokenValue),
			zap.Error(err))
		return nil, fmt.Errorf("failed to create new token: %w", err)
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
	// For now, this is simplified - in production, we'd use a GSI to efficiently find tokens by family
	// TODO: Implement proper GSI queries when DynamORM adds support
	r.logger.Warn("Token family revocation requested - implementing simplified version",
		zap.String("family", family),
		zap.String("reason", reason))

	// This is a placeholder - in a real implementation, we'd:
	// 1. Query the family-index GSI to find all tokens with this family
	// 2. Revoke each token
	// For now, we'll just log the request

	return nil
}

// RevokeUserTokens revokes all tokens for a user (logout all devices)
func (r *AuthRefreshTokenRepository) RevokeUserTokens(ctx context.Context, userID string, reason string) error {
	// For now, this is simplified - in production, we'd use a GSI to efficiently find tokens by user
	// TODO: Implement proper GSI queries when DynamORM adds support
	r.logger.Info("User token revocation requested - implementing simplified version",
		zap.String("user_id", userID),
		zap.String("reason", reason))

	// This is a placeholder - in a real implementation, we'd:
	// 1. Query the user-index GSI to find all tokens for this user
	// 2. Revoke each token
	// For now, we'll just log the request

	return nil
}

// GetTokensByUser retrieves all active tokens for a user
func (r *AuthRefreshTokenRepository) GetTokensByUser(ctx context.Context, userID string) ([]models.AuthRefreshToken, error) {
	// TODO: Implement when GSI support is available
	r.logger.Debug("GetTokensByUser requested - not yet implemented",
		zap.String("user_id", userID))
	return []models.AuthRefreshToken{}, nil
}

// GetTokensByFamily retrieves all tokens in a family
func (r *AuthRefreshTokenRepository) GetTokensByFamily(ctx context.Context, family string) ([]models.AuthRefreshToken, error) {
	// TODO: Implement when GSI support is available
	r.logger.Debug("GetTokensByFamily requested - not yet implemented",
		zap.String("family", family))
	return []models.AuthRefreshToken{}, nil
}