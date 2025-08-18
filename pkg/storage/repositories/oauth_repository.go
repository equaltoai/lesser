package repositories

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"github.com/pay-theory/dynamorm/pkg/core"
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
	helper := NewOAuthHelper(r.db, r.logger)
	return helper.CreateAuthorizationCodeGeneric(ctx, code)
}

// GetAuthorizationCode retrieves an OAuth authorization code
func (r *OAuthRepository) GetAuthorizationCode(ctx context.Context, code string) (*storage.AuthorizationCode, error) {
	helper := NewOAuthHelper(r.db, r.logger)
	return helper.GetAuthorizationCodeGeneric(ctx, code)
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
	helper := NewOAuthHelper(r.db, r.logger)
	return helper.CreateRefreshTokenGeneric(ctx, token)
}

// GetRefreshToken retrieves an OAuth refresh token
func (r *OAuthRepository) GetRefreshToken(ctx context.Context, token string) (*storage.RefreshToken, error) {
	helper := NewOAuthHelper(r.db, r.logger)
	return helper.GetRefreshTokenGeneric(ctx, token)
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
	helper := NewOAuthHelper(r.db, r.logger)
	return helper.SaveUserAppConsentGeneric(ctx, consent)
}

// GetUserAppConsent retrieves user consent for an OAuth app
func (r *OAuthRepository) GetUserAppConsent(ctx context.Context, userID, appID string) (*storage.UserAppConsent, error) {
	helper := NewOAuthHelper(r.db, r.logger)
	return helper.GetUserAppConsentGeneric(ctx, userID, appID)
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
