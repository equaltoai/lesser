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
	// Set expiration time if not set (10 minutes default)
	if data.ExpiresAt.IsZero() {
		data.ExpiresAt = time.Now().Add(10 * time.Minute)
	}

	// Create DynamORM model
	model := &models.OAuthState{
		State:               state,
		Provider:            data.Provider,
		RedirectURI:         data.RedirectURI,
		Username:            data.Username,
		ClientID:            data.ClientID,
		Scopes:              data.Scopes,
		CodeChallenge:       data.CodeChallenge,
		CodeChallengeMethod: data.CodeChallengeMethod,
		CreatedAt:           data.CreatedAt,
		ExpiresAt:           data.ExpiresAt,
	}

	// Update keys
	model.UpdateKeys()

	// Create the item
	err := r.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		r.logger.Error("failed to store OAuth state",
			zap.String("state", state),
			zap.Error(err))
		return fmt.Errorf("failed to store OAuth state: %w", err)
	}

	r.logger.Debug("stored OAuth state",
		zap.String("state", state),
		zap.String("provider", data.Provider),
		zap.Time("expires_at", data.ExpiresAt))

	return nil
}

// GetOAuthState retrieves OAuth state
func (r *OAuthRepository) GetOAuthState(ctx context.Context, state string) (*storage.OAuthState, error) {
	// Construct the key
	pk := fmt.Sprintf("OAUTH_STATE#%s", state)
	sk := models.SKState

	// Query for the item
	var model models.OAuthState
	err := r.db.WithContext(ctx).Model(&models.OAuthState{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("OAuth state not found: %s", state)
		}
		r.logger.Error("failed to get OAuth state", zap.Error(err))
		return nil, fmt.Errorf("failed to get OAuth state: %w", err)
	}

	// Check if expired
	if time.Now().After(model.ExpiresAt) {
		// Clean up expired state
		_ = r.DeleteOAuthState(ctx, state)
		return nil, fmt.Errorf("OAuth state expired: %s", state)
	}

	// Convert to storage model
	result := &storage.OAuthState{
		State:               model.State,
		Provider:            model.Provider,
		RedirectURI:         model.RedirectURI,
		Username:            model.Username,
		ClientID:            model.ClientID,
		Scopes:              model.Scopes,
		CodeChallenge:       model.CodeChallenge,
		CodeChallengeMethod: model.CodeChallengeMethod,
		CreatedAt:           model.CreatedAt,
		ExpiresAt:           model.ExpiresAt,
	}

	r.logger.Debug("retrieved OAuth state",
		zap.String("state", state),
		zap.String("provider", result.Provider))

	return result, nil
}

// DeleteOAuthState deletes OAuth state
func (r *OAuthRepository) DeleteOAuthState(ctx context.Context, state string) error {
	// Construct the key
	pk := fmt.Sprintf("OAUTH_STATE#%s", state)
	sk := models.SKState

	// Delete the item
	err := r.db.WithContext(ctx).Model(&models.OAuthState{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		Delete()

	if err != nil {
		r.logger.Error("failed to delete OAuth state", zap.Error(err))
		return fmt.Errorf("failed to delete OAuth state: %w", err)
	}

	r.logger.Debug("deleted OAuth state", zap.String("state", state))
	return nil
}

// Placeholder methods for other OAuth operations that need to be implemented

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
	sk := models.SKCode

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
	sk := models.SKCode

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
	sk := models.SKToken

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
	sk := models.SKToken

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
	// Validate required fields
	if client.Name == "" || len(client.RedirectURIs) == 0 {
		return fmt.Errorf("client name and redirect_uris are required")
	}

	// Generate client ID if not provided
	if client.ClientID == "" {
		clientID, err := generateClientID()
		if err != nil {
			return fmt.Errorf("failed to generate client ID: %w", err)
		}
		client.ClientID = clientID
	}

	// Generate client secret if not provided
	if client.ClientSecret == "" {
		clientSecret, err := generateClientSecret()
		if err != nil {
			return fmt.Errorf("failed to generate client secret: %w", err)
		}
		client.ClientSecret = clientSecret
	}

	// Create DynamORM model
	model := &models.OAuthClient{
		ClientID:     client.ClientID,
		ClientSecret: client.ClientSecret,
		Name:         client.Name,
		Website:      client.Website,
		RedirectURIs: client.RedirectURIs,
		Scopes:       client.Scopes,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	// BeforeCreate will set up keys
	if err := model.BeforeCreate(); err != nil {
		return fmt.Errorf("failed to prepare OAuth client: %w", err)
	}

	// Create the item with condition that it doesn't exist
	err := r.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		// Check if it's a duplicate key error
		if strings.Contains(err.Error(), "ConditionalCheckFailed") || strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("client with ID %s already exists", client.ClientID)
		}
		r.logger.Error("failed to create OAuth client", zap.Error(err))
		return fmt.Errorf("failed to create OAuth client: %w", err)
	}

	// Update the input client with generated values
	client.CreatedAt = model.CreatedAt
	client.UpdatedAt = model.UpdatedAt

	r.logger.Debug("created OAuth client",
		zap.String("client_id", client.ClientID),
		zap.String("name", client.Name))

	return nil
}

// GetOAuthClient retrieves an OAuth client
func (r *OAuthRepository) GetOAuthClient(ctx context.Context, clientID string) (*storage.OAuthClient, error) {
	// Construct the key
	pk := "CLIENT#" + clientID
	sk := models.SKMetadata

	// Query for the item
	var model models.OAuthClient
	err := r.db.WithContext(ctx).Model(&models.OAuthClient{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("OAuth client not found: %s", clientID)
		}
		r.logger.Error("failed to get OAuth client", zap.Error(err))
		return nil, fmt.Errorf("failed to get OAuth client: %w", err)
	}

	// Convert to storage model
	result := &storage.OAuthClient{
		ClientID:     model.ClientID,
		ClientSecret: model.ClientSecret,
		Name:         model.Name,
		Website:      model.Website,
		RedirectURIs: model.RedirectURIs,
		Scopes:       model.Scopes,
		CreatedAt:    model.CreatedAt,
		UpdatedAt:    model.UpdatedAt,
	}

	r.logger.Debug("retrieved OAuth client",
		zap.String("client_id", clientID),
		zap.String("name", result.Name))

	return result, nil
}

// DeleteOAuthClient deletes an OAuth client
func (r *OAuthRepository) DeleteOAuthClient(ctx context.Context, clientID string) error {
	// Construct the key
	pk := "CLIENT#" + clientID
	sk := models.SKMetadata

	// Delete the item
	err := r.db.WithContext(ctx).Model(&models.OAuthClient{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		Delete()

	if err != nil {
		r.logger.Error("failed to delete OAuth client", zap.Error(err))
		return fmt.Errorf("failed to delete OAuth client: %w", err)
	}

	r.logger.Debug("deleted OAuth client", zap.String("client_id", clientID))
	return nil
}

// UpdateOAuthClient updates an existing OAuth client
func (r *OAuthRepository) UpdateOAuthClient(ctx context.Context, clientID string, updates map[string]any) error {
	if len(updates) == 0 {
		return fmt.Errorf("no updates provided")
	}

	// Construct the key
	pk := "CLIENT#" + clientID
	sk := models.SKMetadata

	// First, get the existing client
	var existingClient models.OAuthClient
	err := r.db.WithContext(ctx).Model(&models.OAuthClient{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&existingClient)

	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("OAuth client not found: %s", clientID)
		}
		return fmt.Errorf("failed to get OAuth client for update: %w", err)
	}

	// Only allow specific fields to be updated
	allowedFields := map[string]bool{
		"name":          true,
		"website":       true,
		"redirect_uris": true,
		"scopes":        true,
	}

	// Apply updates to the model
	for key, value := range updates {
		if allowedFields[key] {
			switch key {
			case "name":
				if v, ok := value.(string); ok {
					existingClient.Name = v
				}
			case "website":
				if v, ok := value.(string); ok {
					existingClient.Website = v
				}
			case "redirect_uris":
				if v, ok := value.([]string); ok {
					existingClient.RedirectURIs = v
				}
			case "scopes":
				if v, ok := value.([]string); ok {
					existingClient.Scopes = v
				}
			}
		}
	}

	// Update timestamp
	existingClient.UpdatedAt = time.Now()

	// Update the item
	err = r.db.WithContext(ctx).Model(&existingClient).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		Update()
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("OAuth client not found: %s", clientID)
		}
		r.logger.Error("failed to update OAuth client", zap.Error(err))
		return fmt.Errorf("failed to update OAuth client: %w", err)
	}

	r.logger.Debug("updated OAuth client",
		zap.String("client_id", clientID),
		zap.Any("updates", updates))

	return nil
}

// DeleteExpiredTokens deletes expired authorization codes and refresh tokens
func (r *OAuthRepository) DeleteExpiredTokens(ctx context.Context) error {
	now := time.Now()

	// Delete expired authorization codes
	// Note: With TTL, DynamoDB will automatically delete these, but we can do manual cleanup
	err := r.db.WithContext(ctx).Model(&models.AuthorizationCode{}).
		Where("ExpiresAt", "<", now).
		Delete()

	if err != nil && !errors.IsNotFound(err) {
		r.logger.Warn("failed to delete expired authorization codes", zap.Error(err))
	}

	// Delete expired refresh tokens
	err = r.db.WithContext(ctx).Model(&models.RefreshToken{}).
		Where("ExpiresAt", "<", now).
		Delete()

	if err != nil && !errors.IsNotFound(err) {
		r.logger.Warn("failed to delete expired refresh tokens", zap.Error(err))
	}

	r.logger.Debug("deleted expired tokens")
	return nil
}

// Helper functions

// generateClientID generates a unique client ID
func generateClientID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// generateClientSecret generates a secure client secret
func generateClientSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// ListOAuthClients lists OAuth clients with pagination
func (r *OAuthRepository) ListOAuthClients(ctx context.Context, limit int32, _ string) ([]*storage.OAuthClient, string, error) {
	// For now, implement a simple scan since DynamORM doesn't have great pagination support
	// In production, you might want to add a GSI for listing clients
	var clientModels []*models.OAuthClient

	query := r.db.WithContext(ctx).Model(&models.OAuthClient{}).
		Where("PK", "begins_with", "CLIENT#").
		Where("SK", "=", "METADATA")

	if limit > 0 {
		query = query.Limit(int(limit))
	}

	// For cursor-based pagination, you would need additional GSI setup
	// This is a simplified implementation
	err := query.Scan(&clientModels)
	if err != nil {
		r.logger.Error("failed to list OAuth clients", zap.Error(err))
		return nil, "", fmt.Errorf("failed to list OAuth clients: %w", err)
	}

	// Convert to storage models
	clients := make([]*storage.OAuthClient, len(clientModels))
	for i, model := range clientModels {
		clients[i] = &storage.OAuthClient{
			ClientID:     model.ClientID,
			ClientSecret: model.ClientSecret,
			Name:         model.Name,
			Website:      model.Website,
			RedirectURIs: model.RedirectURIs,
			Scopes:       model.Scopes,
			CreatedAt:    model.CreatedAt,
			UpdatedAt:    model.UpdatedAt,
		}
	}

	// Simple pagination - in production you'd want proper cursor implementation
	nextCursor := ""
	if len(clientModels) == int(limit) {
		nextCursor = "has_more" // Simplified cursor
	}

	r.logger.Debug("listed OAuth clients", zap.Int("count", len(clients)))
	return clients, nextCursor, nil
}

// GetOAuthApp retrieves an OAuth app (alias for GetOAuthClient for compatibility)
func (r *OAuthRepository) GetOAuthApp(ctx context.Context, clientID string) (*storage.OAuthApp, error) {
	// Get the OAuth client first
	client, err := r.GetOAuthClient(ctx, clientID)
	if err != nil {
		return nil, err
	}

	// Convert OAuthClient to OAuthApp format based on storage interface
	app := &storage.OAuthApp{
		ClientID:     client.ClientID,
		ClientSecret: client.ClientSecret,
		Name:         client.Name,
		RedirectURIs: client.RedirectURIs,
		Scopes:       client.Scopes,
		CreatedAt:    client.CreatedAt,
	}

	return app, nil
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

	// Convert to storage model (only fields that exist in storage interface)
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
