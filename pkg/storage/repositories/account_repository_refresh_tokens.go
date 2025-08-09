package repositories

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// ===== Advanced Refresh Token Methods =====
// This file contains advanced refresh token management with token families for security

// CreateAdvancedRefreshToken generates and stores a new refresh token with family-based security
func (r *AccountRepository) CreateAdvancedRefreshToken(ctx context.Context, userID string, deviceName string, ipAddress string) (*models.AuthRefreshToken, error) {
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
		LastUsedAt: now.Unix(),
	}

	err := r.db.WithContext(ctx).Model(token).Create()
	if err != nil {
		r.logger.Error("failed to create advanced refresh token",
			zap.String("userID", userID),
			zap.String("deviceName", deviceName),
			zap.Error(err))
		return nil, fmt.Errorf("failed to create refresh token: %w", err)
	}

	r.logger.Info("created advanced refresh token",
		zap.String("userID", userID),
		zap.String("family", token.Family),
		zap.String("deviceName", deviceName))

	return token, nil
}

// GetAdvancedRefreshToken retrieves an advanced refresh token by token value
func (r *AccountRepository) GetAdvancedRefreshToken(ctx context.Context, token string) (*models.AuthRefreshToken, error) {
	var authToken models.AuthRefreshToken

	err := r.db.WithContext(ctx).Model(&authToken).
		Where("PK", "=", fmt.Sprintf("REFRESH_TOKEN#%s", token)).
		Where("SK", "=", "TOKEN").
		First(&authToken)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, common.ErrTokenNotFound
		}
		r.logger.Error("failed to get advanced refresh token",
			zap.String("token", token[:8]+"..."),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get refresh token: %w", err)
	}

	// Check if token is expired
	if time.Now().Unix() > authToken.ExpiresAt {
		return nil, common.ErrTokenExpired
	}

	// Check if token is revoked
	if authToken.Revoked {
		return nil, common.ErrTokenRevoked
	}

	return &authToken, nil
}

// RotateAdvancedRefreshToken rotates a refresh token (creates new one, revokes old one)
func (r *AccountRepository) RotateAdvancedRefreshToken(ctx context.Context, oldTokenValue string, ipAddress string) (*models.AuthRefreshToken, error) {
	// Get the old token
	oldToken, err := r.GetAdvancedRefreshToken(ctx, oldTokenValue)
	if err != nil {
		return nil, err
	}

	// Check for token reuse (security check)
	if oldToken.Revoked {
		r.logger.Warn("detected refresh token reuse attempt - revoking family",
			zap.String("family", oldToken.Family),
			zap.String("userID", oldToken.UserID),
			zap.String("ipAddress", ipAddress))

		// Revoke entire token family due to potential compromise
		if revokeErr := r.RevokeAdvancedTokenFamily(ctx, oldToken.Family, "token_reuse_detected"); revokeErr != nil {
			r.logger.Error("failed to revoke token family after reuse detection",
				zap.String("family", oldToken.Family),
				zap.Error(revokeErr))
		}

		return nil, common.ErrTokenRevoked
	}

	// Generate a new token in the same family
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("failed to generate new token: %w", err)
	}

	now := time.Now()
	newToken := &models.AuthRefreshToken{
		Token:      base64.URLEncoding.EncodeToString(tokenBytes),
		UserID:     oldToken.UserID,
		Family:     oldToken.Family, // Same family
		Generation: oldToken.Generation + 1,
		CreatedAt:  now.Unix(),
		ExpiresAt:  now.Add(30 * 24 * time.Hour).Unix(),
		Revoked:    false,
		DeviceName: oldToken.DeviceName,
		IPAddress:  ipAddress,
		LastUsedAt: now.Unix(),
	}

	// Create the new token
	err = r.db.WithContext(ctx).Model(newToken).Create()
	if err != nil {
		r.logger.Error("failed to create rotated refresh token",
			zap.String("userID", oldToken.UserID),
			zap.String("family", oldToken.Family),
			zap.Error(err))
		return nil, fmt.Errorf("failed to create rotated token: %w", err)
	}

	// Revoke the old token
	if err := r.revokeAdvancedRefreshToken(ctx, oldToken, "rotated"); err != nil {
		r.logger.Error("failed to revoke old token during rotation",
			zap.String("token", oldTokenValue[:8]+"..."),
			zap.Error(err))
		// Continue - new token was created successfully
	}

	r.logger.Info("rotated advanced refresh token",
		zap.String("userID", oldToken.UserID),
		zap.String("family", oldToken.Family),
		zap.Int("oldGeneration", oldToken.Generation),
		zap.Int("newGeneration", newToken.Generation))

	return newToken, nil
}

// revokeAdvancedRefreshToken revokes a specific token (internal helper)
func (r *AccountRepository) revokeAdvancedRefreshToken(ctx context.Context, token *models.AuthRefreshToken, reason string) error {
	token.Revoked = true
	token.RevokedReason = reason

	err := r.db.WithContext(ctx).Model(token).Update()
	if err != nil {
		return fmt.Errorf("failed to revoke token: %w", err)
	}

	return nil
}

// RevokeAdvancedTokenFamily revokes all tokens in a family (security measure)
//
//nolint:dupl // similar token revocation pattern for different scope (family vs user)
func (r *AccountRepository) RevokeAdvancedTokenFamily(ctx context.Context, family string, reason string) error {
	tokens, err := r.GetAdvancedTokensByFamily(ctx, family)
	if err != nil {
		return fmt.Errorf("failed to get tokens in family: %w", err)
	}

	var revokeErrors []error
	for _, token := range tokens {
		if !token.Revoked {
			if err := r.revokeAdvancedRefreshToken(ctx, &token, reason); err != nil {
				r.logger.Error("failed to revoke token in family",
					zap.String("family", family),
					zap.String("token", token.Token[:8]+"..."),
					zap.Error(err))
				revokeErrors = append(revokeErrors, err)
			}
		}
	}

	if len(revokeErrors) > 0 {
		return fmt.Errorf("failed to revoke %d tokens in family", len(revokeErrors))
	}

	r.logger.Info("revoked token family",
		zap.String("family", family),
		zap.String("reason", reason),
		zap.Int("tokenCount", len(tokens)))

	return nil
}

// RevokeAdvancedUserTokens revokes all tokens for a user (e.g., on logout)
//
//nolint:dupl // similar token revocation pattern for different scope (family vs user)
func (r *AccountRepository) RevokeAdvancedUserTokens(ctx context.Context, userID string, reason string) error {
	tokens, err := r.GetAdvancedTokensByUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user tokens: %w", err)
	}

	var revokeErrors []error
	for _, token := range tokens {
		if !token.Revoked {
			if err := r.revokeAdvancedRefreshToken(ctx, &token, reason); err != nil {
				r.logger.Error("failed to revoke user token",
					zap.String("userID", userID),
					zap.String("token", token.Token[:8]+"..."),
					zap.Error(err))
				revokeErrors = append(revokeErrors, err)
			}
		}
	}

	if len(revokeErrors) > 0 {
		return fmt.Errorf("failed to revoke %d user tokens", len(revokeErrors))
	}

	r.logger.Info("revoked all user tokens",
		zap.String("userID", userID),
		zap.String("reason", reason),
		zap.Int("tokenCount", len(tokens)))

	return nil
}

// GetAdvancedTokensByUser retrieves all tokens for a user
func (r *AccountRepository) GetAdvancedTokensByUser(ctx context.Context, userID string) ([]models.AuthRefreshToken, error) {
	var tokens []models.AuthRefreshToken

	err := r.db.WithContext(ctx).Model(&models.AuthRefreshToken{}).
		Index("user-tokens-index").
		Where("GSI1PK", "=", fmt.Sprintf("USER#%s", userID)).
		Where("GSI1SK", "BEGINS_WITH", "TOKEN#").
		All(&tokens)

	if err != nil {
		r.logger.Error("failed to get tokens by user",
			zap.String("userID", userID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get user tokens: %w", err)
	}

	return tokens, nil
}

// GetAdvancedTokensByFamily retrieves all tokens in a family
func (r *AccountRepository) GetAdvancedTokensByFamily(ctx context.Context, family string) ([]models.AuthRefreshToken, error) {
	var tokens []models.AuthRefreshToken

	err := r.db.WithContext(ctx).Model(&models.AuthRefreshToken{}).
		Index("family-tokens-index").
		Where("GSI2PK", "=", fmt.Sprintf("FAMILY#%s", family)).
		Where("GSI2SK", "BEGINS_WITH", "TOKEN#").
		All(&tokens)

	if err != nil {
		r.logger.Error("failed to get tokens by family",
			zap.String("family", family),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get family tokens: %w", err)
	}

	return tokens, nil
}

// GetActiveAdvancedTokensForUser retrieves all active (non-expired, non-revoked) tokens for a user
func (r *AccountRepository) GetActiveAdvancedTokensForUser(ctx context.Context, userID string) ([]models.AuthRefreshToken, error) {
	allTokens, err := r.GetAdvancedTokensByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	now := time.Now().Unix()
	var activeTokens []models.AuthRefreshToken

	for _, token := range allTokens {
		if !token.Revoked && token.ExpiresAt > now {
			activeTokens = append(activeTokens, token)
		}
	}

	return activeTokens, nil
}

// UpdateAdvancedTokenLastUsed updates the last used timestamp for a token
func (r *AccountRepository) UpdateAdvancedTokenLastUsed(ctx context.Context, tokenValue string, ipAddress string) error {
	// Get existing token
	var token models.AuthRefreshToken
	err := r.db.WithContext(ctx).Model(&token).
		Where("PK", "=", fmt.Sprintf("REFRESH_TOKEN#%s", tokenValue)).
		Where("SK", "=", "TOKEN").
		First(&token)

	if err != nil {
		if errors.IsNotFound(err) {
			return common.ErrTokenNotFound
		}
		return fmt.Errorf("failed to get token for update: %w", err)
	}

	// Update last used info
	now := time.Now()
	token.LastUsedAt = now.Unix()
	if ipAddress != "" {
		token.IPAddress = ipAddress
	}

	err = r.db.WithContext(ctx).Model(&token).Update()
	if err != nil {
		r.logger.Error("failed to update token last used",
			zap.String("token", tokenValue[:8]+"..."),
			zap.Error(err))
		return fmt.Errorf("failed to update token: %w", err)
	}

	return nil
}

// CleanupExpiredAdvancedTokens removes expired tokens (maintenance task)
func (r *AccountRepository) CleanupExpiredAdvancedTokens(ctx context.Context) (int, error) {
	// Note: In production, this would use a more efficient scan or be handled by DynamoDB TTL
	// For now, we'll implement a basic cleanup

	var allTokens []models.AuthRefreshToken
	err := r.db.WithContext(ctx).Model(&models.AuthRefreshToken{}).
		Where("SK", "=", "TOKEN").
		All(&allTokens)

	if err != nil {
		return 0, fmt.Errorf("failed to scan tokens for cleanup: %w", err)
	}

	now := time.Now().Unix()
	var deletedCount int

	for _, token := range allTokens {
		if token.ExpiresAt < now {
			deleteErr := r.db.WithContext(ctx).Model(&models.AuthRefreshToken{}).
				Where("PK", "=", fmt.Sprintf("REFRESH_TOKEN#%s", token.Token)).
				Where("SK", "=", "TOKEN").
				Delete()

			if deleteErr != nil {
				r.logger.Error("failed to delete expired token during cleanup",
					zap.String("token", token.Token[:8]+"..."),
					zap.Error(deleteErr))
			} else {
				deletedCount++
			}
		}
	}

	if deletedCount > 0 {
		r.logger.Info("cleaned up expired refresh tokens",
			zap.Int("deletedCount", deletedCount))
	}

	return deletedCount, nil
}

// GetAdvancedTokenStats returns statistics about tokens for monitoring
func (r *AccountRepository) GetAdvancedTokenStats(ctx context.Context, userID string) (*TokenStats, error) {
	tokens, err := r.GetAdvancedTokensByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	now := time.Now().Unix()
	stats := &TokenStats{
		UserID: userID,
	}

	familyCount := make(map[string]int)

	for _, token := range tokens {
		stats.TotalTokens++
		familyCount[token.Family]++

		if token.Revoked {
			stats.RevokedTokens++
		} else if token.ExpiresAt < now {
			stats.ExpiredTokens++
		} else {
			stats.ActiveTokens++
		}

		if token.LastUsedAt > stats.LastUsedAt {
			stats.LastUsedAt = token.LastUsedAt
		}
	}

	stats.UniqueFamilies = len(familyCount)

	return stats, nil
}

// TokenStats represents token statistics for a user
type TokenStats struct {
	UserID         string `json:"user_id"`
	TotalTokens    int    `json:"total_tokens"`
	ActiveTokens   int    `json:"active_tokens"`
	RevokedTokens  int    `json:"revoked_tokens"`
	ExpiredTokens  int    `json:"expired_tokens"`
	UniqueFamilies int    `json:"unique_families"`
	LastUsedAt     int64  `json:"last_used_at"`
}
