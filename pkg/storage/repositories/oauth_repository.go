package repositories

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// OAuthRepository handles OAuth-related storage operations
type OAuthRepository struct {
	db     core.DB
	logger *zap.Logger
}

// NewOAuthRepository creates a new OAuth repository
func NewOAuthRepository(db core.DB, logger *zap.Logger) *OAuthRepository {
	return &OAuthRepository{
		db:     db,
		logger: logger,
	}
}

// StoreOAuthState stores OAuth state for CSRF protection
func (r *OAuthRepository) StoreOAuthState(ctx context.Context, state string, data *storage.OAuthState) error {
	helper := NewOAuthHelper(r.db, r.logger)
	return helper.StoreOAuthStateGeneric(ctx, state, data)
}

// GetOAuthState retrieves OAuth state
func (r *OAuthRepository) GetOAuthState(ctx context.Context, state string) (*storage.OAuthState, error) {
	helper := NewOAuthHelper(r.db, r.logger)
	return helper.GetOAuthStateGeneric(ctx, state)
}

// DeleteOAuthState deletes OAuth state
func (r *OAuthRepository) DeleteOAuthState(ctx context.Context, state string) error {
	helper := NewOAuthHelper(r.db, r.logger)
	return helper.DeleteOAuthStateGeneric(ctx, state)
}

// OAuth operations implementation - complete OAuth 2.0 flow support

// CreateAuthorizationCode creates a new OAuth authorization code
func (r *OAuthRepository) CreateAuthorizationCode(ctx context.Context, code *storage.AuthorizationCode) error {
	// Create DynamORM model
	model := &models.AuthorizationCode{
		Code:          code.Code,
		ClientID:      code.ClientID,
		Username:      code.Username,
		CodeChallenge: code.CodeChallenge,
		ExpiresAt:     code.ExpiresAt,
		Scopes:        code.Scopes,
		CreatedAt:     time.Now(),
	}

	// BeforeCreate will set up keys and TTL
	if err := model.BeforeCreate(); err != nil {
		return fmt.Errorf("failed to prepare authorization code: %w", err)
	}

	// Create the item with condition that it doesn't exist
	err := r.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		// Check if it's a duplicate key error
		if strings.Contains(err.Error(), "ConditionalCheckFailed") || strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("authorization code already exists: %s", code.Code)
		}
		r.logger.Error("failed to create authorization code", zap.Error(err))
		return fmt.Errorf("failed to create authorization code: %w", err)
	}

	r.logger.Debug("created authorization code",
		zap.String("code", code.Code),
		zap.String("client_id", code.ClientID),
		zap.String("username", code.Username))

	return nil
}

// GetAuthorizationCode retrieves an OAuth authorization code
func (r *OAuthRepository) GetAuthorizationCode(ctx context.Context, code string) (*storage.AuthorizationCode, error) {
	// Construct the key
	pk := "AUTHCODE#" + code
	sk := "CODE"

	// Query for the item
	var model models.AuthorizationCode
	err := r.db.WithContext(ctx).Model(&models.AuthorizationCode{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("authorization code not found: %s", code)
		}
		r.logger.Error("failed to get authorization code", zap.Error(err))
		return nil, fmt.Errorf("failed to get authorization code: %w", err)
	}

	// Check if code has expired
	if time.Now().After(model.ExpiresAt) {
		// Clean up expired code
		_ = r.DeleteAuthorizationCode(ctx, code)
		return nil, fmt.Errorf("authorization code expired: %s", code)
	}

	// Convert to storage model
	result := &storage.AuthorizationCode{
		Code:          model.Code,
		ClientID:      model.ClientID,
		Username:      model.Username,
		CodeChallenge: model.CodeChallenge,
		ExpiresAt:     model.ExpiresAt,
		Scopes:        model.Scopes,
	}

	r.logger.Debug("retrieved authorization code",
		zap.String("code", code),
		zap.String("client_id", result.ClientID))

	return result, nil
}

// DeleteAuthorizationCode deletes an OAuth authorization code
func (r *OAuthRepository) DeleteAuthorizationCode(ctx context.Context, code string) error {
	// Construct the key
	pk := "AUTHCODE#" + code
	sk := "CODE"

	// Delete the item
	err := r.db.WithContext(ctx).Model(&models.AuthorizationCode{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		Delete()

	if err != nil {
		r.logger.Error("failed to delete authorization code", zap.Error(err))
		return fmt.Errorf("failed to delete authorization code: %w", err)
	}

	r.logger.Debug("deleted authorization code", zap.String("code", code))
	return nil
}

// CreateRefreshToken creates a new OAuth refresh token
func (r *OAuthRepository) CreateRefreshToken(ctx context.Context, token *storage.RefreshToken) error {
	// Create DynamORM model
	model := &models.RefreshToken{
		Token:     token.Token,
		ClientID:  token.ClientID,
		Username:  token.Username,
		ExpiresAt: token.ExpiresAt,
		Scopes:    token.Scopes,
		CreatedAt: time.Now(),
	}

	// BeforeCreate will set up keys and TTL
	if err := model.BeforeCreate(); err != nil {
		return fmt.Errorf("failed to prepare refresh token: %w", err)
	}

	// Create the item with condition that it doesn't exist
	err := r.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		// Check if it's a duplicate key error
		if strings.Contains(err.Error(), "ConditionalCheckFailed") || strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("refresh token already exists")
		}
		r.logger.Error("failed to create refresh token", zap.Error(err))
		return fmt.Errorf("failed to create refresh token: %w", err)
	}

	r.logger.Debug("created refresh token",
		zap.String("client_id", token.ClientID),
		zap.String("username", token.Username))

	return nil
}

// GetRefreshToken retrieves an OAuth refresh token
func (r *OAuthRepository) GetRefreshToken(ctx context.Context, token string) (*storage.RefreshToken, error) {
	// Construct the key
	pk := "REFRESHTOKEN#" + token
	sk := "TOKEN"

	// Query for the item
	var model models.RefreshToken
	err := r.db.WithContext(ctx).Model(&models.RefreshToken{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("refresh token not found")
		}
		r.logger.Error("failed to get refresh token", zap.Error(err))
		return nil, fmt.Errorf("failed to get refresh token: %w", err)
	}

	// Check if token has expired
	if time.Now().After(model.ExpiresAt) {
		// Clean up expired token
		_ = r.DeleteRefreshToken(ctx, token)
		return nil, fmt.Errorf("refresh token expired")
	}

	// Convert to storage model
	result := &storage.RefreshToken{
		Token:     model.Token,
		ClientID:  model.ClientID,
		Username:  model.Username,
		ExpiresAt: model.ExpiresAt,
		Scopes:    model.Scopes,
	}

	r.logger.Debug("retrieved refresh token",
		zap.String("client_id", result.ClientID),
		zap.String("username", result.Username))

	return result, nil
}

// DeleteRefreshToken deletes an OAuth refresh token
func (r *OAuthRepository) DeleteRefreshToken(ctx context.Context, token string) error {
	// Construct the key
	pk := "REFRESHTOKEN#" + token
	sk := "TOKEN"

	// Delete the item
	err := r.db.WithContext(ctx).Model(&models.RefreshToken{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		Delete()

	if err != nil {
		r.logger.Error("failed to delete refresh token", zap.Error(err))
		return fmt.Errorf("failed to delete refresh token: %w", err)
	}

	r.logger.Debug("deleted refresh token", zap.String("token", token))
	return nil
}

// CreateOAuthClient creates a new OAuth client
func (r *OAuthRepository) CreateOAuthClient(ctx context.Context, client *storage.OAuthClient) error {
	helper := NewOAuthHelper(r.db, r.logger)
	return helper.CreateOAuthClientGeneric(ctx, client)
}

// GetOAuthClient retrieves an OAuth client
func (r *OAuthRepository) GetOAuthClient(ctx context.Context, clientID string) (*storage.OAuthClient, error) {
	helper := NewOAuthHelper(r.db, r.logger)
	return helper.GetOAuthClientGeneric(ctx, clientID)
}

// DeleteOAuthClient deletes an OAuth client
func (r *OAuthRepository) DeleteOAuthClient(ctx context.Context, clientID string) error {
	helper := NewOAuthHelper(r.db, r.logger)
	return helper.DeleteOAuthClientGeneric(ctx, clientID)
}

// UpdateOAuthClient updates an existing OAuth client
func (r *OAuthRepository) UpdateOAuthClient(ctx context.Context, clientID string, updates map[string]any) error {
	helper := NewOAuthHelper(r.db, r.logger)
	return helper.UpdateOAuthClientGeneric(ctx, clientID, updates)
}

// DeleteExpiredTokens removes expired OAuth tokens
func (r *OAuthRepository) DeleteExpiredTokens(ctx context.Context) error {
	// This would typically be run as a scheduled job
	// For now, we'll log that it should be implemented
	r.logger.Info("DeleteExpiredTokens should be implemented as a scheduled job")
	return nil
}

// validateClient checks if a client ID and secret are valid
func (r *OAuthRepository) validateClient(ctx context.Context, clientID, clientSecret string) error {
	client, err := r.GetOAuthClient(ctx, clientID)
	if err != nil {
		return err
	}
	if client == nil {
		return fmt.Errorf("invalid client_id")
	}
	if client.ClientSecret != clientSecret {
		return fmt.Errorf("invalid client_secret")
	}
	return nil
}

// ListOAuthClients lists OAuth clients with pagination
func (r *OAuthRepository) ListOAuthClients(ctx context.Context, limit int32, _ string) ([]*storage.OAuthClient, string, error) {
	helper := NewOAuthHelper(r.db, r.logger)
	return helper.ListOAuthClientsGeneric(ctx, "", int(limit))
}

// SaveUserAppConsent saves user consent for an OAuth app
func (r *OAuthRepository) SaveUserAppConsent(ctx context.Context, consent *storage.UserAppConsent) error {
	// Create DynamORM model
	model := &models.UserAppConsent{
		UserID:    consent.UserID,
		AppID:     consent.AppID,
		Scopes:    consent.Scopes,
		CreatedAt: consent.CreatedAt,
		UpdatedAt: time.Now(),
		Active:    true, // Default to active
	}

	// Set default timestamps if not provided
	if model.CreatedAt.IsZero() {
		model.CreatedAt = time.Now()
	}

	// Update keys
	model.UpdateKeys()

	// Use upsert logic - try to update first, then create if not exists
	err := r.db.WithContext(ctx).Model(model).
		Where("PK", "=", model.PK).
		Where("SK", "=", model.SK).
		Update()

	if err != nil {
		if errors.IsNotFound(err) {
			// Item doesn't exist, create it
			err = r.db.WithContext(ctx).Model(model).Create()
			if err != nil {
				r.logger.Error("failed to create user app consent", zap.Error(err))
				return fmt.Errorf("failed to create user app consent: %w", err)
			}
		} else {
			r.logger.Error("failed to update user app consent", zap.Error(err))
			return fmt.Errorf("failed to update user app consent: %w", err)
		}
	}

	r.logger.Debug("saved user app consent",
		zap.String("user_id", consent.UserID),
		zap.String("app_id", consent.AppID))

	return nil
}

// GetUserAppConsent retrieves user consent for an OAuth app
func (r *OAuthRepository) GetUserAppConsent(ctx context.Context, userID, appID string) (*storage.UserAppConsent, error) {
	// Construct the key using the model's pattern
	pk := fmt.Sprintf("USER#%s", userID)
	sk := fmt.Sprintf("CONSENT#%s", appID)

	// Query for the item
	var model models.UserAppConsent
	err := r.db.WithContext(ctx).Model(&models.UserAppConsent{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("user app consent not found: %s:%s", userID, appID)
		}
		r.logger.Error("failed to get user app consent", zap.Error(err))
		return nil, fmt.Errorf("failed to get user app consent: %w", err)
	}

	// Convert to storage model
	result := &storage.UserAppConsent{
		UserID:    model.UserID,
		AppID:     model.AppID,
		Scopes:    model.Scopes,
		CreatedAt: model.CreatedAt,
	}

	r.logger.Debug("retrieved user app consent",
		zap.String("user_id", userID),
		zap.String("app_id", appID))

	return result, nil
}

// Helper functions

func generateClientID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

func generateClientSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
