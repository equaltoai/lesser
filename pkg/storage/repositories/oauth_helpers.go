package repositories

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	stdErrors "errors"
	"fmt"
	"strings"
	"time"

	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/errors"
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
		PrincipalUsername:   data.PrincipalUsername,
		AgentUsername:       data.AgentUsername,
		ClientID:            data.ClientID,
		Scopes:              data.Scopes,
		CodeChallenge:       data.CodeChallenge,
		CodeChallengeMethod: data.CodeChallengeMethod,
		Resource:            data.Resource,
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
		PrincipalUsername:   model.PrincipalUsername,
		AgentUsername:       model.AgentUsername,
		ClientID:            model.ClientID,
		Scopes:              model.Scopes,
		CodeChallenge:       model.CodeChallenge,
		CodeChallengeMethod: model.CodeChallengeMethod,
		Resource:            model.Resource,
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
	// Generate client secret if not provided.
	if err := common.ValidateRequiredParam("client.ClientSecret", client.ClientSecret); err != nil {
		secret, err := h.generateClientSecret()
		if err != nil {
			return ErrorHandler.HandleCreateError(err, "oauth client secret", "generation")
		}
		client.ClientSecret = secret
	}

	storedSecret, err := common.HashOAuthClientSecret(client.ClientSecret)
	if err != nil {
		return ErrorHandler.HandleCreateError(err, "oauth client secret", "hashing")
	}
	client.ClientSecretHash = storedSecret

	// Set timestamps
	if client.CreatedAt.IsZero() {
		client.CreatedAt = time.Now()
	}
	client.UpdatedAt = time.Now()

	// Create DynamORM model
	model := &models.OAuthClient{
		ClientID:                           client.ClientID,
		ClientSecret:                       storedSecret,
		PreviousClientSecret:               client.PreviousClientSecretHash,
		PreviousClientSecretGraceExpiresAt: client.PreviousClientSecretGraceExpiresAt,
		SecretRotatedAt:                    client.SecretRotatedAt,
		SecretRotatedBy:                    client.SecretRotatedBy,
		Name:                               client.Name,
		Description:                        client.Description,
		Website:                            client.Website,
		ClientURI:                          client.ClientURI,
		LogoURI:                            client.LogoURI,
		Contacts:                           client.Contacts,
		TosURI:                             client.TosURI,
		PolicyURI:                          client.PolicyURI,
		SoftwareID:                         client.SoftwareID,
		SoftwareVersion:                    client.SoftwareVersion,
		RedirectURIs:                       client.RedirectURIs,
		GrantTypes:                         client.GrantTypes,
		ResponseTypes:                      client.ResponseTypes,
		Scopes:                             client.Scopes,
		ClientClass:                        client.ClientClass,
		AgentUsername:                      client.AgentUsername,
		OwnerID:                            client.OwnerID,
		RegistrationSource:                 client.RegistrationSource,
		Confidential:                       client.Confidential,
		CreatedAt:                          client.CreatedAt,
		UpdatedAt:                          client.UpdatedAt,
	}

	// Update keys
	_ = model.UpdateKeys() // Ignore error as this is internal model operation

	// Create the item
	err = h.db.WithContext(ctx).Model(model).Create()
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
		ClientID:                           model.ClientID,
		ClientSecretHash:                   model.ClientSecret,
		PreviousClientSecretHash:           model.PreviousClientSecret,
		PreviousClientSecretGraceExpiresAt: model.PreviousClientSecretGraceExpiresAt,
		SecretRotatedAt:                    model.SecretRotatedAt,
		SecretRotatedBy:                    model.SecretRotatedBy,
		Name:                               model.Name,
		Description:                        model.Description,
		Website:                            model.Website,
		ClientURI:                          model.ClientURI,
		LogoURI:                            model.LogoURI,
		Contacts:                           model.Contacts,
		TosURI:                             model.TosURI,
		PolicyURI:                          model.PolicyURI,
		SoftwareID:                         model.SoftwareID,
		SoftwareVersion:                    model.SoftwareVersion,
		RedirectURIs:                       model.RedirectURIs,
		GrantTypes:                         model.GrantTypes,
		ResponseTypes:                      model.ResponseTypes,
		Scopes:                             model.Scopes,
		ClientClass:                        model.ClientClass,
		AgentUsername:                      model.AgentUsername,
		OwnerID:                            model.OwnerID,
		RegistrationSource:                 model.RegistrationSource,
		Confidential:                       model.Confidential,
		CreatedAt:                          model.CreatedAt,
		UpdatedAt:                          model.UpdatedAt,
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
		applyOAuthClientStorageUpdate(existing, key, value)
	}

	existing.UpdatedAt = time.Now()

	// Create updated model
	model := &models.OAuthClient{
		ClientID:                           existing.ClientID,
		ClientSecret:                       existing.ClientSecretHash,
		PreviousClientSecret:               existing.PreviousClientSecretHash,
		PreviousClientSecretGraceExpiresAt: existing.PreviousClientSecretGraceExpiresAt,
		SecretRotatedAt:                    existing.SecretRotatedAt,
		SecretRotatedBy:                    existing.SecretRotatedBy,
		Name:                               existing.Name,
		Description:                        existing.Description,
		Website:                            existing.Website,
		ClientURI:                          existing.ClientURI,
		LogoURI:                            existing.LogoURI,
		Contacts:                           existing.Contacts,
		TosURI:                             existing.TosURI,
		PolicyURI:                          existing.PolicyURI,
		SoftwareID:                         existing.SoftwareID,
		SoftwareVersion:                    existing.SoftwareVersion,
		RedirectURIs:                       existing.RedirectURIs,
		GrantTypes:                         existing.GrantTypes,
		ResponseTypes:                      existing.ResponseTypes,
		Scopes:                             existing.Scopes,
		ClientClass:                        existing.ClientClass,
		AgentUsername:                      existing.AgentUsername,
		OwnerID:                            existing.OwnerID,
		RegistrationSource:                 existing.RegistrationSource,
		Confidential:                       existing.Confidential,
		CreatedAt:                          existing.CreatedAt,
		UpdatedAt:                          existing.UpdatedAt,
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

func applyOAuthClientStorageUpdate(client *storage.OAuthClient, key string, value any) {
	if client == nil {
		return
	}

	switch key {
	case FieldName:
		if v, ok := value.(string); ok {
			client.Name = v
		}
	case "description":
		if v, ok := value.(string); ok {
			client.Description = v
		}
	case FieldRedirectURIs:
		if v, ok := value.([]string); ok {
			client.RedirectURIs = v
		}
	case "grant_types":
		if v, ok := value.([]string); ok {
			client.GrantTypes = v
		}
	case FieldScopes:
		if v, ok := value.([]string); ok {
			client.Scopes = v
		}
	case FieldWebsite:
		if v, ok := value.(string); ok {
			client.Website = v
		}
	case FieldAgentUsername:
		if v, ok := value.(string); ok {
			client.AgentUsername = v
		}
	case "confidential":
		if v, ok := value.(bool); ok {
			client.Confidential = v
		}
	}
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
		Index("gsi1").
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
			ClientID:                           model.ClientID,
			ClientSecretHash:                   model.ClientSecret,
			PreviousClientSecretHash:           model.PreviousClientSecret,
			PreviousClientSecretGraceExpiresAt: model.PreviousClientSecretGraceExpiresAt,
			SecretRotatedAt:                    model.SecretRotatedAt,
			SecretRotatedBy:                    model.SecretRotatedBy,
			Name:                               model.Name,
			Description:                        model.Description,
			Website:                            model.Website,
			ClientURI:                          model.ClientURI,
			LogoURI:                            model.LogoURI,
			Contacts:                           model.Contacts,
			TosURI:                             model.TosURI,
			PolicyURI:                          model.PolicyURI,
			SoftwareID:                         model.SoftwareID,
			SoftwareVersion:                    model.SoftwareVersion,
			RedirectURIs:                       model.RedirectURIs,
			GrantTypes:                         model.GrantTypes,
			ResponseTypes:                      model.ResponseTypes,
			Scopes:                             model.Scopes,
			ClientClass:                        model.ClientClass,
			AgentUsername:                      model.AgentUsername,
			OwnerID:                            model.OwnerID,
			RegistrationSource:                 model.RegistrationSource,
			Confidential:                       model.Confidential,
			CreatedAt:                          model.CreatedAt,
			UpdatedAt:                          model.UpdatedAt,
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
		Code:              code.Code,
		ClientID:          code.ClientID,
		RedirectURI:       code.RedirectURI,
		Resource:          code.Resource,
		Username:          code.Username,
		PrincipalUsername: code.PrincipalUsername,
		AgentUsername:     code.AgentUsername,
		CodeChallenge:     code.CodeChallenge,
		ExpiresAt:         code.ExpiresAt,
		Scopes:            code.Scopes,
		CreatedAt:         time.Now(),
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
		Code:              model.Code,
		ClientID:          model.ClientID,
		RedirectURI:       model.RedirectURI,
		Resource:          model.Resource,
		Username:          model.Username,
		PrincipalUsername: model.PrincipalUsername,
		AgentUsername:     model.AgentUsername,
		CodeChallenge:     model.CodeChallenge,
		ExpiresAt:         model.ExpiresAt,
		Scopes:            model.Scopes,
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
		IfExists().
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
	model := refreshTokenModelFromStorage(token)

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
		ConsistentRead().
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

	result := refreshTokenStorageFromModel(model)

	h.logger.Debug("retrieved refresh token",
		zap.String("client_id", result.ClientID),
		zap.String("username", result.Username))

	return result, nil
}

// UpdateRefreshTokenGeneric updates an existing OAuth refresh token.
func (h *OAuthHelper) UpdateRefreshTokenGeneric(ctx context.Context, token *storage.RefreshToken) error {
	model := refreshTokenModelFromStorage(token)
	if err := model.UpdateKeys(); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityRefreshToken, "preparation")
	}

	if model.Version == 0 {
		seedBuilder := h.db.WithContext(ctx).
			Model(&models.RefreshToken{}).
			Where("PK", "=", model.PK).
			Where("SK", "=", model.SK).
			UpdateBuilder()

		if err := seedBuilder.SetIfNotExists("Version", nil, 0).Execute(); err != nil && !strings.Contains(strings.ToLower(err.Error()), "conditionalcheckfailed") {
			h.logger.Error("failed to seed refresh token version", zap.Error(err))
			return ErrorHandler.HandleUpdateError(err, EntityRefreshToken, "token")
		}
	}

	if err := h.db.WithContext(ctx).Model(model).Update(); err != nil {
		h.logger.Error("failed to update refresh token", zap.Error(err))
		lowerErr := strings.ToLower(err.Error())
		if errors.IsConditionFailed(err) ||
			strings.Contains(lowerErr, "conditionalcheckfailed") ||
			strings.Contains(lowerErr, "conditional check failed") {
			return ErrorHandler.HandleUpdateError(stdErrors.Join(err, errors.ErrConditionFailed), EntityRefreshToken, "token")
		}
		return ErrorHandler.HandleUpdateError(err, EntityRefreshToken, "token")
	}

	token.Version = model.Version + 1
	if token.Version == 0 {
		token.Version = 1
	}

	return nil
}

// ListRefreshTokensByUserClientGeneric returns all refresh tokens for a username and client ID.
func (h *OAuthHelper) ListRefreshTokensByUserClientGeneric(ctx context.Context, username, clientID string) ([]storage.RefreshToken, error) {
	var modelsOut []models.RefreshToken
	if err := h.db.WithContext(ctx).Model(&models.RefreshToken{}).
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("RUNTIME_USER#%s#%s", username, clientID)).
		All(&modelsOut); err != nil {
		if errors.IsNotFound(err) {
			return []storage.RefreshToken{}, nil
		}
		return nil, ErrorHandler.HandleQueryError(err, EntityRefreshToken, "runtime user/client")
	}

	return refreshTokenStorageSliceFromModels(modelsOut), nil
}

// ListRefreshTokensByFamilyGeneric returns all refresh tokens for a runtime token family.
func (h *OAuthHelper) ListRefreshTokensByFamilyGeneric(ctx context.Context, familyID string) ([]storage.RefreshToken, error) {
	var modelsOut []models.RefreshToken
	if err := h.db.WithContext(ctx).Model(&models.RefreshToken{}).
		Index("gsi2").
		Where("gsi2PK", "=", "RUNTIME_FAMILY#"+familyID).
		All(&modelsOut); err != nil {
		if errors.IsNotFound(err) {
			return []storage.RefreshToken{}, nil
		}
		return nil, ErrorHandler.HandleQueryError(err, EntityRefreshToken, "runtime family")
	}

	return refreshTokenStorageSliceFromModels(modelsOut), nil
}

// ListRefreshTokensBySessionGeneric returns all refresh tokens for a runtime session.
func (h *OAuthHelper) ListRefreshTokensBySessionGeneric(ctx context.Context, sessionID string) ([]storage.RefreshToken, error) {
	var modelsOut []models.RefreshToken
	if err := h.db.WithContext(ctx).Model(&models.RefreshToken{}).
		Index("gsi3").
		Where("gsi3PK", "=", "RUNTIME_SESSION#"+sessionID).
		All(&modelsOut); err != nil {
		if errors.IsNotFound(err) {
			return []storage.RefreshToken{}, nil
		}
		return nil, ErrorHandler.HandleQueryError(err, EntityRefreshToken, "runtime session")
	}

	return refreshTokenStorageSliceFromModels(modelsOut), nil
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

func refreshTokenModelFromStorage(token *storage.RefreshToken) *models.RefreshToken {
	model := &models.RefreshToken{
		Token:               token.Token,
		ClientID:            token.ClientID,
		Username:            token.Username,
		Resource:            token.Resource,
		ExpiresAt:           token.ExpiresAt,
		Scopes:              token.Scopes,
		ClientClass:         token.ClientClass,
		SessionID:           token.SessionID,
		FamilyID:            token.FamilyID,
		Generation:          token.Generation,
		Current:             token.Current,
		Revoked:             token.Revoked,
		RevokedAt:           token.RevokedAt,
		RevokedReason:       token.RevokedReason,
		DeviceLabel:         token.DeviceLabel,
		LastUsedAt:          token.LastUsedAt,
		IdleExpiresAt:       token.IdleExpiresAt,
		AbsoluteExpiresAt:   token.AbsoluteExpiresAt,
		SessionCreatedAt:    token.SessionCreatedAt,
		AccessTTLSeconds:    token.AccessTTLSeconds,
		ReuseDetectedAt:     token.ReuseDetectedAt,
		ReuseDetectedFromIP: token.ReuseDetectedFromIP,
		ReuseDetectedFromUA: token.ReuseDetectedFromUA,
		LastAuthFailureCode: token.LastAuthFailureCode,
		LastAuthFailureAt:   token.LastAuthFailureAt,
		LastAuthFailureMsg:  token.LastAuthFailureMsg,
		LastAuthSuccessAt:   token.LastAuthSuccessAt,
		CreatedAt:           token.CreatedAt,
		Version:             token.Version,
	}
	if model.CreatedAt.IsZero() {
		model.CreatedAt = time.Now()
	}
	return model
}

func refreshTokenStorageFromModel(model models.RefreshToken) *storage.RefreshToken {
	return &storage.RefreshToken{
		Token:               model.Token,
		ClientID:            model.ClientID,
		Username:            model.Username,
		Resource:            model.Resource,
		ExpiresAt:           model.ExpiresAt,
		Scopes:              model.Scopes,
		ClientClass:         model.ClientClass,
		SessionID:           model.SessionID,
		FamilyID:            model.FamilyID,
		Generation:          model.Generation,
		Current:             model.Current,
		Revoked:             model.Revoked,
		RevokedAt:           model.RevokedAt,
		RevokedReason:       model.RevokedReason,
		DeviceLabel:         model.DeviceLabel,
		LastUsedAt:          model.LastUsedAt,
		IdleExpiresAt:       model.IdleExpiresAt,
		AbsoluteExpiresAt:   model.AbsoluteExpiresAt,
		SessionCreatedAt:    model.SessionCreatedAt,
		AccessTTLSeconds:    model.AccessTTLSeconds,
		ReuseDetectedAt:     model.ReuseDetectedAt,
		ReuseDetectedFromIP: model.ReuseDetectedFromIP,
		ReuseDetectedFromUA: model.ReuseDetectedFromUA,
		LastAuthFailureCode: model.LastAuthFailureCode,
		LastAuthFailureAt:   model.LastAuthFailureAt,
		LastAuthFailureMsg:  model.LastAuthFailureMsg,
		LastAuthSuccessAt:   model.LastAuthSuccessAt,
		CreatedAt:           model.CreatedAt,
		Version:             model.Version,
	}
}

func refreshTokenStorageSliceFromModels(modelsOut []models.RefreshToken) []storage.RefreshToken {
	out := make([]storage.RefreshToken, 0, len(modelsOut))
	for _, model := range modelsOut {
		out = append(out, *refreshTokenStorageFromModel(model))
	}
	return out
}

// RevokeAccessTokenGeneric stores the JWT ID (JTI) of an access token as revoked.
// The record is TTL'd to the token's expiry, so revocations are self-cleaning.
func (h *OAuthHelper) RevokeAccessTokenGeneric(ctx context.Context, jti string, expiresAt time.Time) error {
	jti = strings.TrimSpace(jti)
	if err := common.ValidateRequiredParam("jti", jti); err != nil {
		return err
	}

	model := &models.RevokedAccessToken{
		JTI:       jti,
		ExpiresAt: expiresAt,
		RevokedAt: time.Now().UTC(),
	}

	if err := model.BeforeCreate(); err != nil {
		return ErrorHandler.HandleCreateError(err, EntityRevokedAccessToken, "preparation")
	}

	// Create the item. Revoke is idempotent, so treat duplicate key as success.
	if err := h.db.WithContext(ctx).Model(model).Create(); err != nil {
		if strings.Contains(err.Error(), "ConditionalCheckFailed") || strings.Contains(err.Error(), "already exists") {
			return nil
		}
		h.logger.Error("failed to revoke access token", zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityRevokedAccessToken, "token")
	}

	h.logger.Debug("revoked access token", zap.String("jti", jti))
	return nil
}

// IsAccessTokenRevokedGeneric reports whether the given JWT ID (JTI) has been revoked.
func (h *OAuthHelper) IsAccessTokenRevokedGeneric(ctx context.Context, jti string) (bool, error) {
	if h == nil || h.db == nil {
		return false, nil
	}

	jti = strings.TrimSpace(jti)
	if err := common.ValidateRequiredParam("jti", jti); err != nil {
		return false, err
	}

	pk := "REVOKEDTOKEN#" + jti
	sk := SKToken

	var model models.RevokedAccessToken
	err := h.db.WithContext(ctx).Model(&models.RevokedAccessToken{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		ConsistentRead().
		First(&model)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		h.logger.Error("failed to check access token revocation", zap.Error(err))
		return false, ErrorHandler.HandleGetError(err, EntityRevokedAccessToken, "token")
	}

	// Defensive: treat malformed records as not revoked.
	if strings.TrimSpace(model.JTI) == "" || model.ExpiresAt.IsZero() {
		return false, nil
	}

	// Treat expired entries as not revoked; best-effort cleanup.
	if !model.ExpiresAt.IsZero() && time.Now().After(model.ExpiresAt) {
		_ = h.DeleteRevokedAccessTokenGeneric(ctx, jti)
		return false, nil
	}

	return true, nil
}

// DeleteRevokedAccessTokenGeneric removes a revoked access token record by its JWT ID (JTI).
func (h *OAuthHelper) DeleteRevokedAccessTokenGeneric(ctx context.Context, jti string) error {
	jti = strings.TrimSpace(jti)
	if err := common.ValidateRequiredParam("jti", jti); err != nil {
		return err
	}

	pk := "REVOKEDTOKEN#" + jti
	sk := SKToken

	err := h.db.WithContext(ctx).Model(&models.RevokedAccessToken{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		Delete()
	if err != nil && !errors.IsNotFound(err) {
		h.logger.Error("failed to delete revoked access token", zap.Error(err))
		return ErrorHandler.HandleDeleteError(err, EntityRevokedAccessToken, "token")
	}

	return nil
}

// SaveUserAppConsentGeneric saves user consent for an OAuth app
func (h *OAuthHelper) SaveUserAppConsentGeneric(ctx context.Context, consent *storage.UserAppConsent) error {
	userID := strings.TrimSpace(consent.UserID)
	if userID == "" {
		userID = strings.TrimSpace(consent.Username)
	}
	appID := strings.TrimSpace(consent.AppID)
	if appID == "" {
		appID = strings.TrimSpace(consent.ClientID)
	}
	resource := strings.TrimSpace(consent.Resource)

	// Create DynamORM model
	model := &models.UserAppConsent{
		UserID:    userID,
		AppID:     appID,
		Resource:  resource,
		Scopes:    consent.Scopes,
		CreatedAt: consent.CreatedAt,
		UpdatedAt: time.Now(),
		Active:    true, // Default to active
	}

	// Set default timestamps if not provided
	if model.CreatedAt.IsZero() {
		if !consent.GrantedAt.IsZero() {
			model.CreatedAt = consent.GrantedAt
		} else {
			model.CreatedAt = time.Now()
		}
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
				return ErrorHandler.HandleCreateError(err, EntityOAuthConsent, fmt.Sprintf("%s:%s:%s", userID, appID, resource))
			}
		} else {
			h.logger.Error("failed to update user app consent", zap.Error(err))
			return ErrorHandler.HandleUpdateError(err, EntityOAuthConsent, fmt.Sprintf("%s:%s:%s", userID, appID, resource))
		}
	}

	h.logger.Debug("saved user app consent",
		zap.String("user_id", userID),
		zap.String("app_id", appID),
		zap.String("resource", resource))

	return nil
}

// GetUserAppConsentGeneric retrieves user consent for an OAuth app
func (h *OAuthHelper) GetUserAppConsentGeneric(ctx context.Context, userID, appID, resource string) (*storage.UserAppConsent, error) {
	lookup := &models.UserAppConsent{
		UserID:   strings.TrimSpace(userID),
		AppID:    strings.TrimSpace(appID),
		Resource: strings.TrimSpace(resource),
	}
	_ = lookup.UpdateKeys()

	// Query for the item
	var model models.UserAppConsent
	err := h.db.WithContext(ctx).Model(&models.UserAppConsent{}).
		Where("PK", "=", lookup.PK).
		Where("SK", "=", lookup.SK).
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityOAuthConsent, fmt.Sprintf("%s:%s:%s", userID, appID, lookup.Resource))
		}
		h.logger.Error("failed to get user app consent", zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, EntityOAuthConsent, fmt.Sprintf("%s:%s:%s", userID, appID, lookup.Resource))
	}

	// Convert to storage model
	result := &storage.UserAppConsent{
		Username:  model.UserID,
		UserID:    model.UserID,
		ClientID:  model.AppID,
		AppID:     model.AppID,
		Resource:  model.Resource,
		Scopes:    model.Scopes,
		GrantedAt: model.CreatedAt,
		CreatedAt: model.CreatedAt,
		UpdatedAt: model.UpdatedAt,
	}

	h.logger.Debug("retrieved user app consent",
		zap.String("user_id", userID),
		zap.String("app_id", appID),
		zap.String("resource", lookup.Resource))

	return result, nil
}
