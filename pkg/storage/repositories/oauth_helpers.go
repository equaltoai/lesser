package repositories

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	stdErrors "errors"
	"fmt"
	"strings"
	"time"

	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// OAuthHelper provides common OAuth storage operations
type OAuthHelper struct {
	db     core.DB
	logger *zap.Logger
}

// NewOAuthHelper creates a new OAuth helper
func NewOAuthHelper(db core.DB, logger *zap.Logger) *OAuthHelper {
	return &OAuthHelper{
		db:     db,
		logger: logger,
	}
}

// StoreOAuthStateGeneric stores OAuth state for CSRF protection
func (h *OAuthHelper) StoreOAuthStateGeneric(ctx context.Context, state string, data *storage.OAuthState) error {
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
	_ = model.UpdateKeys() // Ignore error as this is internal model operation

	// Create the item
	err := h.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		h.logger.Error("failed to store OAuth state",
			zap.String("state", state),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityOAuthState, state)
	}

	return nil
}

// GetOAuthStateGeneric retrieves OAuth state
func (h *OAuthHelper) GetOAuthStateGeneric(ctx context.Context, state string) (*storage.OAuthState, error) {
	// Query for OAuth state
	var model models.OAuthState
	err := h.db.WithContext(ctx).Model(&models.OAuthState{}).
		Where("PK", "=", fmt.Sprintf("OAUTH_STATE#%s", state)).
		Where("SK", "=", "STATE").
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityOAuthState, state)
		}
		h.logger.Error("failed to get OAuth state",
			zap.String("state", state),
			zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, EntityOAuthState, state)
	}

	// Check expiration
	if !model.ExpiresAt.IsZero() && model.ExpiresAt.Before(time.Now()) {
		// Delete expired state
		_ = h.DeleteOAuthStateGeneric(ctx, state)
		return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityOAuthState, state)
	}

	// Convert to storage model
	return &storage.OAuthState{
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
	}, nil
}

// DeleteOAuthStateGeneric deletes OAuth state
func (h *OAuthHelper) DeleteOAuthStateGeneric(ctx context.Context, state string) error {
	model := &models.OAuthState{
		State: state,
	}
	_ = model.UpdateKeys() // Ignore error as this is internal model operation

	err := h.db.WithContext(ctx).Model(model).Delete()
	if err != nil && !errors.IsNotFound(err) {
		h.logger.Error("failed to delete OAuth state",
			zap.String("state", state),
			zap.Error(err))
		return ErrorHandler.HandleDeleteError(err, EntityOAuthState, state)
	}

	return nil
}

// CreateOAuthClientGeneric creates an OAuth client
func (h *OAuthHelper) CreateOAuthClientGeneric(ctx context.Context, client *storage.OAuthClient) error {
	// Generate client secret if not provided
	if err := common.ValidateRequiredParam("client.ClientSecret", client.ClientSecret); err != nil {
		secret, err := h.generateClientSecret()
		if err != nil {
			return ErrorHandler.HandleCreateError(err, "oauth client secret", "generation")
		}
		client.ClientSecret = secret
	}

	// Set timestamps
	if client.CreatedAt.IsZero() {
		client.CreatedAt = time.Now()
	}
	client.UpdatedAt = time.Now()

	// Create DynamORM model
	model := &models.OAuthClient{
		ClientID:     client.ClientID,
		ClientSecret: client.ClientSecret,
		Name:         client.Name,
		Description:  client.Description,
		RedirectURIs: client.RedirectURIs,
		GrantTypes:   client.GrantTypes,
		Scopes:       client.Scopes,
		Website:      client.Website,
		OwnerID:      client.OwnerID,
		Confidential: client.Confidential,
		CreatedAt:    client.CreatedAt,
		UpdatedAt:    client.UpdatedAt,
	}

	// Update keys
	_ = model.UpdateKeys() // Ignore error as this is internal model operation

	// Create the item
	err := h.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		h.logger.Error("failed to create OAuth client",
			zap.String("client_id", client.ClientID),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityOAuthClient, client.ClientID)
	}

	return nil
}

// GetOAuthClientGeneric retrieves an OAuth client
func (h *OAuthHelper) GetOAuthClientGeneric(ctx context.Context, clientID string) (*storage.OAuthClient, error) {
	// Query for OAuth client
	var model models.OAuthClient
	err := h.db.WithContext(ctx).Model(&models.OAuthClient{}).
		Where("PK", "=", fmt.Sprintf("OAUTH_CLIENT#%s", clientID)).
		Where("SK", "=", "CLIENT").
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityOAuthClient, clientID)
		}
		h.logger.Error("failed to get OAuth client",
			zap.String("client_id", clientID),
			zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, EntityOAuthClient, clientID)
	}

	// Convert to storage model
	return &storage.OAuthClient{
		ClientID:     model.ClientID,
		ClientSecret: model.ClientSecret,
		Name:         model.Name,
		Description:  model.Description,
		RedirectURIs: model.RedirectURIs,
		GrantTypes:   model.GrantTypes,
		Scopes:       model.Scopes,
		Website:      model.Website,
		OwnerID:      model.OwnerID,
		Confidential: model.Confidential,
		CreatedAt:    model.CreatedAt,
		UpdatedAt:    model.UpdatedAt,
	}, nil
}

// UpdateOAuthClientGeneric updates an OAuth client
func (h *OAuthHelper) UpdateOAuthClientGeneric(ctx context.Context, clientID string, updates map[string]interface{}) error {
	// Get existing client
	existing, err := h.GetOAuthClientGeneric(ctx, clientID)
	if err != nil {
		if stdErrors.Is(err, storage.ErrNotFound) {
			return ErrorHandler.HandleGetError(storage.ErrNotFound, EntityOAuthClient, clientID)
		}
		return err
	}

	// Apply updates
	for key, value := range updates {
		switch key {
		case "name":
			if v, ok := value.(string); ok {
				existing.Name = v
			}
		case "description":
			if v, ok := value.(string); ok {
				existing.Description = v
			}
		case "redirect_uris":
			if v, ok := value.([]string); ok {
				existing.RedirectURIs = v
			}
		case "grant_types":
			if v, ok := value.([]string); ok {
				existing.GrantTypes = v
			}
		case "scopes":
			if v, ok := value.([]string); ok {
				existing.Scopes = v
			}
		case "website":
			if v, ok := value.(string); ok {
				existing.Website = v
			}
		case "confidential":
			if v, ok := value.(bool); ok {
				existing.Confidential = v
			}
		}
	}

	existing.UpdatedAt = time.Now()

	// Create updated model
	model := &models.OAuthClient{
		ClientID:     existing.ClientID,
		ClientSecret: existing.ClientSecret,
		Name:         existing.Name,
		Description:  existing.Description,
		RedirectURIs: existing.RedirectURIs,
		GrantTypes:   existing.GrantTypes,
		Scopes:       existing.Scopes,
		Website:      existing.Website,
		OwnerID:      existing.OwnerID,
		Confidential: existing.Confidential,
		CreatedAt:    existing.CreatedAt,
		UpdatedAt:    existing.UpdatedAt,
	}

	// Update keys
	_ = model.UpdateKeys() // Ignore error as this is internal model operation

	// Update the item
	err = h.db.WithContext(ctx).Model(model).Update()
	if err != nil {
		h.logger.Error("failed to update OAuth client",
			zap.String("client_id", clientID),
			zap.Error(err))
		return ErrorHandler.HandleUpdateError(err, EntityOAuthClient, clientID)
	}

	return nil
}

// DeleteOAuthClientGeneric deletes an OAuth client
func (h *OAuthHelper) DeleteOAuthClientGeneric(ctx context.Context, clientID string) error {
	model := &models.OAuthClient{
		ClientID: clientID,
	}
	_ = model.UpdateKeys() // Ignore error as this is internal model operation

	err := h.db.WithContext(ctx).Model(model).Delete()
	if err != nil && !errors.IsNotFound(err) {
		h.logger.Error("failed to delete OAuth client",
			zap.String("client_id", clientID),
			zap.Error(err))
		return ErrorHandler.HandleDeleteError(err, EntityOAuthClient, clientID)
	}

	// Also delete any associated tokens
	h.deleteOAuthTokensForClient(ctx, clientID)

	return nil
}

// ListOAuthClientsGeneric lists OAuth clients for an owner
func (h *OAuthHelper) ListOAuthClientsGeneric(ctx context.Context, ownerID string, limit int) ([]*storage.OAuthClient, string, error) {
	// Query for OAuth clients by owner
	var models []models.OAuthClient
	query := h.db.WithContext(ctx).Model(&models).
		Index("GSI1").
		Where("gsi1PK", "=", fmt.Sprintf("OWNER#%s", ownerID))

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.All(&models)
	if err != nil {
		h.logger.Error("failed to list OAuth clients",
			zap.String("owner_id", ownerID),
			zap.Error(err))
		return nil, "", ErrorHandler.HandleQueryError(err, EntityOAuthClient, "list by owner")
	}

	// Convert to storage models
	clients := make([]*storage.OAuthClient, len(models))
	for i, model := range models {
		clients[i] = &storage.OAuthClient{
			ClientID:     model.ClientID,
			ClientSecret: model.ClientSecret,
			Name:         model.Name,
			Description:  model.Description,
			RedirectURIs: model.RedirectURIs,
			GrantTypes:   model.GrantTypes,
			Scopes:       model.Scopes,
			Website:      model.Website,
			OwnerID:      model.OwnerID,
			Confidential: model.Confidential,
			CreatedAt:    model.CreatedAt,
			UpdatedAt:    model.UpdatedAt,
		}
	}

	// Simple pagination - in production you'd want proper cursor implementation
	nextCursor := ""
	if len(models) == limit {
		nextCursor = "has_more" // Simplified cursor
	}

	return clients, nextCursor, nil
}

// Helper methods

func (h *OAuthHelper) generateClientSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

func (h *OAuthHelper) deleteOAuthTokensForClient(_ context.Context, clientID string) {
	// This would delete all tokens associated with the client
	// Implementation depends on how tokens are stored
	h.logger.Info("deleting OAuth tokens for client",
		zap.String("client_id", clientID))
}

// NormalizeRedirectURI normalizes a redirect URI for comparison
func NormalizeRedirectURI(uri string) string {
	// Remove trailing slashes
	uri = strings.TrimRight(uri, "/")
	// Convert to lowercase for scheme and host
	if idx := strings.Index(uri, "://"); idx > 0 {
		scheme := strings.ToLower(uri[:idx])
		rest := uri[idx+3:]
		if idx2 := strings.IndexAny(rest, "/?#"); idx2 > 0 {
			host := strings.ToLower(rest[:idx2])
			path := rest[idx2:]
			uri = scheme + "://" + host + path
		} else {
			uri = scheme + "://" + strings.ToLower(rest)
		}
	}
	return uri
}

// ValidateRedirectURI validates that a redirect URI matches one of the registered URIs
func ValidateRedirectURI(registeredURIs []string, redirectURI string) bool {
	normalizedRedirect := NormalizeRedirectURI(redirectURI)
	for _, registered := range registeredURIs {
		if NormalizeRedirectURI(registered) == normalizedRedirect {
			return true
		}
	}
	return false
}

// CreateAuthorizationCodeGeneric creates a new OAuth authorization code
func (h *OAuthHelper) CreateAuthorizationCodeGeneric(ctx context.Context, code *storage.AuthorizationCode) error {
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
		return ErrorHandler.HandleCreateError(err, EntityAuthCode, "preparation")
	}

	// Create the item with condition that it doesn't exist
	err := h.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		// Check if it's a duplicate key error
		if strings.Contains(err.Error(), "ConditionalCheckFailed") || strings.Contains(err.Error(), "already exists") {
			return ErrorHandler.HandleCreateError(storage.ErrAlreadyExists, EntityAuthCode, code.Code)
		}
		h.logger.Error("failed to create authorization code", zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityAuthCode, code.Code)
	}

	h.logger.Debug("created authorization code",
		zap.String("code", code.Code),
		zap.String("client_id", code.ClientID),
		zap.String("username", code.Username))

	return nil
}

// GetAuthorizationCodeGeneric retrieves an OAuth authorization code
func (h *OAuthHelper) GetAuthorizationCodeGeneric(ctx context.Context, code string) (*storage.AuthorizationCode, error) {
	// Construct the key
	pk := "AUTHCODE#" + code
	sk := SKCode

	// Query for the item
	var model models.AuthorizationCode
	err := h.db.WithContext(ctx).Model(&models.AuthorizationCode{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityAuthCode, code)
		}
		h.logger.Error("failed to get authorization code", zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, EntityAuthCode, code)
	}

	// Check if code has expired
	if time.Now().After(model.ExpiresAt) {
		// Clean up expired code
		_ = h.DeleteAuthorizationCodeGeneric(ctx, code)
		return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityAuthCode, code)
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

	h.logger.Debug("retrieved authorization code",
		zap.String("code", code),
		zap.String("client_id", result.ClientID))

	return result, nil
}

// DeleteAuthorizationCodeGeneric deletes an OAuth authorization code
func (h *OAuthHelper) DeleteAuthorizationCodeGeneric(ctx context.Context, code string) error {
	// Construct the key
	pk := "AUTHCODE#" + code
	sk := SKCode

	// Delete the item
	err := h.db.WithContext(ctx).Model(&models.AuthorizationCode{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		Delete()

	if err != nil {
		h.logger.Error("failed to delete authorization code", zap.Error(err))
		return ErrorHandler.HandleDeleteError(err, EntityAuthCode, code)
	}

	h.logger.Debug("deleted authorization code", zap.String("code", code))
	return nil
}

// CreateRefreshTokenGeneric creates a new OAuth refresh token
func (h *OAuthHelper) CreateRefreshTokenGeneric(ctx context.Context, token *storage.RefreshToken) error {
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
		return ErrorHandler.HandleCreateError(err, EntityRefreshToken, "preparation")
	}

	// Create the item with condition that it doesn't exist
	err := h.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		// Check if it's a duplicate key error
		if strings.Contains(err.Error(), "ConditionalCheckFailed") || strings.Contains(err.Error(), "already exists") {
			return ErrorHandler.HandleCreateError(storage.ErrAlreadyExists, EntityRefreshToken, "token")
		}
		h.logger.Error("failed to create refresh token", zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityRefreshToken, "token")
	}

	h.logger.Debug("created refresh token",
		zap.String("client_id", token.ClientID),
		zap.String("username", token.Username))

	return nil
}

// GetRefreshTokenGeneric retrieves an OAuth refresh token
func (h *OAuthHelper) GetRefreshTokenGeneric(ctx context.Context, token string) (*storage.RefreshToken, error) {
	// Construct the key
	pk := "REFRESHTOKEN#" + token
	sk := SKToken

	// Query for the item
	var model models.RefreshToken
	err := h.db.WithContext(ctx).Model(&models.RefreshToken{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityRefreshToken, "token")
		}
		h.logger.Error("failed to get refresh token", zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, EntityRefreshToken, "token")
	}

	// Check if token has expired
	if time.Now().After(model.ExpiresAt) {
		// Clean up expired token
		_ = h.DeleteRefreshTokenGeneric(ctx, token)
		return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityRefreshToken, "token")
	}

	// Convert to storage model
	result := &storage.RefreshToken{
		Token:     model.Token,
		ClientID:  model.ClientID,
		Username:  model.Username,
		ExpiresAt: model.ExpiresAt,
		Scopes:    model.Scopes,
	}

	h.logger.Debug("retrieved refresh token",
		zap.String("client_id", result.ClientID),
		zap.String("username", result.Username))

	return result, nil
}

// DeleteRefreshTokenGeneric deletes an OAuth refresh token
func (h *OAuthHelper) DeleteRefreshTokenGeneric(ctx context.Context, token string) error {
	// Construct the key
	pk := "REFRESHTOKEN#" + token
	sk := SKToken

	// Delete the item
	err := h.db.WithContext(ctx).Model(&models.RefreshToken{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		Delete()

	if err != nil {
		h.logger.Error("failed to delete refresh token", zap.Error(err))
		return ErrorHandler.HandleDeleteError(err, EntityRefreshToken, "token")
	}

	h.logger.Debug("deleted refresh token", zap.String("token", token))
	return nil
}

// SaveUserAppConsentGeneric saves user consent for an OAuth app
func (h *OAuthHelper) SaveUserAppConsentGeneric(ctx context.Context, consent *storage.UserAppConsent) error {
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
	_ = model.UpdateKeys() // Ignore error as this is internal model operation

	// Use upsert logic - try to update first, then create if not exists
	err := h.db.WithContext(ctx).Model(model).
		Where("PK", "=", model.PK).
		Where("SK", "=", model.SK).
		Update()

	if err != nil {
		if errors.IsNotFound(err) {
			// Item doesn't exist, create it
			err = h.db.WithContext(ctx).Model(model).Create()
			if err != nil {
				h.logger.Error("failed to create user app consent", zap.Error(err))
				return ErrorHandler.HandleCreateError(err, EntityOAuthConsent, fmt.Sprintf("%s:%s", consent.UserID, consent.AppID))
			}
		} else {
			h.logger.Error("failed to update user app consent", zap.Error(err))
			return ErrorHandler.HandleUpdateError(err, EntityOAuthConsent, fmt.Sprintf("%s:%s", consent.UserID, consent.AppID))
		}
	}

	h.logger.Debug("saved user app consent",
		zap.String("user_id", consent.UserID),
		zap.String("app_id", consent.AppID))

	return nil
}

// GetUserAppConsentGeneric retrieves user consent for an OAuth app
func (h *OAuthHelper) GetUserAppConsentGeneric(ctx context.Context, userID, appID string) (*storage.UserAppConsent, error) {
	// Construct the key using the model's pattern
	pk := fmt.Sprintf("USER#%s", userID)
	sk := fmt.Sprintf("CONSENT#%s", appID)

	// Query for the item
	var model models.UserAppConsent
	err := h.db.WithContext(ctx).Model(&models.UserAppConsent{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityOAuthConsent, fmt.Sprintf("%s:%s", userID, appID))
		}
		h.logger.Error("failed to get user app consent", zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, EntityOAuthConsent, fmt.Sprintf("%s:%s", userID, appID))
	}

	// Convert to storage model
	result := &storage.UserAppConsent{
		UserID:    model.UserID,
		AppID:     model.AppID,
		Scopes:    model.Scopes,
		CreatedAt: model.CreatedAt,
	}

	h.logger.Debug("retrieved user app consent",
		zap.String("user_id", userID),
		zap.String("app_id", appID))

	return result, nil
}
