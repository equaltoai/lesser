package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/pkg/errors"
	"go.uber.org/zap"
)

// OAuth field constants
const (
	FieldName         = "name"
	FieldWebsite      = "website"
	FieldRedirectURIs = "redirect_uris"
	FieldScopes       = "scopes"
)

const (
	oauthClientsIndexName       = models.IndexOAuthClients
	oauthClientsIndexPK         = "OAUTH_CLIENTS"
	oauthClientSortKey          = "CLIENT"
	oauthClientsPartitionAttr   = "OAuthClientsPK"
	oauthClientsSortAttr        = "OAuthClientsSK"
	oauthClientsDefaultLimit    = 20
	oauthClientsMaxAllowedLimit = 200
)

// ===== OAuth Methods =====
// This file contains OAuth-related methods for the AccountRepository

// StoreOAuthState stores OAuth state for CSRF protection
func (r *AccountRepository) StoreOAuthState(ctx context.Context, state string, data *storage.OAuthState) error {
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
	err := r.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		r.logger.Error("failed to store OAuth state",
			zap.String("state", state),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityOAuthState, state)
	}

	r.logger.Debug("stored OAuth state",
		zap.String("state", state),
		zap.String("provider", data.Provider),
		zap.Time("expires_at", data.ExpiresAt))

	return nil
}

// GetOAuthState retrieves OAuth state
func (r *AccountRepository) GetOAuthState(ctx context.Context, state string) (*storage.OAuthState, error) {
	// Construct the key
	pk := fmt.Sprintf("OAUTH_STATE#%s", state)
	sk := "STATE"

	// Query for the item
	var model models.OAuthState
	err := r.db.WithContext(ctx).Model(&models.OAuthState{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(err, EntityOAuthState, state)
		}
		r.logger.Error("failed to get OAuth state", zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, EntityOAuthState, state)
	}

	// Check if expired
	if time.Now().After(model.ExpiresAt) {
		// Clean up expired state
		_ = r.DeleteOAuthState(ctx, state)
		return nil, ErrorHandler.HandleGetError(ErrOAuthStateExpired, EntityOAuthState, state)
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
func (r *AccountRepository) DeleteOAuthState(ctx context.Context, state string) error {
	// Construct the key
	pk := fmt.Sprintf("OAUTH_STATE#%s", state)
	sk := "STATE"

	// Delete the item
	err := r.db.WithContext(ctx).Model(&models.OAuthState{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		Delete()

	if err != nil {
		r.logger.Error("failed to delete OAuth state", zap.Error(err))
		return ErrorHandler.HandleDeleteError(err, EntityOAuthState, state)
	}

	r.logger.Debug("deleted OAuth state", zap.String("state", state))
	return nil
}

// CreateAuthorizationCode creates a new OAuth authorization code
func (r *AccountRepository) CreateAuthorizationCode(ctx context.Context, code *storage.AuthorizationCode) error {
	helper := NewOAuthHelper(r.db, r.logger)
	return helper.CreateAuthorizationCodeGeneric(ctx, code)
}

// GetAuthorizationCode retrieves an OAuth authorization code
func (r *AccountRepository) GetAuthorizationCode(ctx context.Context, code string) (*storage.AuthorizationCode, error) {
	helper := NewOAuthHelper(r.db, r.logger)
	return helper.GetAuthorizationCodeGeneric(ctx, code)
}

// DeleteAuthorizationCode deletes an OAuth authorization code
func (r *AccountRepository) DeleteAuthorizationCode(ctx context.Context, code string) error {
	// Construct the key
	pk := "AUTHCODE#" + code
	sk := "CODE"

	// Delete the item
	err := r.db.WithContext(ctx).Model(&models.AuthorizationCode{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		IfExists().
		Delete()

	if err != nil {
		r.logger.Error("failed to delete authorization code", zap.Error(err))
		return ErrorHandler.HandleDeleteError(err, EntityAuthCode, code)
	}

	r.logger.Debug("deleted authorization code", zap.String("code", code))
	return nil
}

// CreateRefreshToken creates a new OAuth refresh token
func (r *AccountRepository) CreateRefreshToken(ctx context.Context, token *storage.RefreshToken) error {
	helper := NewOAuthHelper(r.db, r.logger)
	return helper.CreateRefreshTokenGeneric(ctx, token)
}

// GetRefreshToken retrieves an OAuth refresh token
func (r *AccountRepository) GetRefreshToken(ctx context.Context, token string) (*storage.RefreshToken, error) {
	helper := NewOAuthHelper(r.db, r.logger)
	return helper.GetRefreshTokenGeneric(ctx, token)
}

// DeleteRefreshToken deletes an OAuth refresh token
func (r *AccountRepository) DeleteRefreshToken(ctx context.Context, token string) error {
	helper := NewOAuthHelper(r.db, r.logger)
	return helper.DeleteRefreshTokenGeneric(ctx, token)
}

// UpdateRefreshToken updates an OAuth refresh token record.
func (r *AccountRepository) UpdateRefreshToken(ctx context.Context, token *storage.RefreshToken) error {
	helper := NewOAuthHelper(r.db, r.logger)
	return helper.UpdateRefreshTokenGeneric(ctx, token)
}

// ListRefreshTokensByUserClient returns runtime refresh tokens for a user/client pair.
func (r *AccountRepository) ListRefreshTokensByUserClient(ctx context.Context, username, clientID string) ([]storage.RefreshToken, error) {
	helper := NewOAuthHelper(r.db, r.logger)
	return helper.ListRefreshTokensByUserClientGeneric(ctx, username, clientID)
}

// ListRefreshTokensByFamily returns runtime refresh tokens in the same family.
func (r *AccountRepository) ListRefreshTokensByFamily(ctx context.Context, familyID string) ([]storage.RefreshToken, error) {
	helper := NewOAuthHelper(r.db, r.logger)
	return helper.ListRefreshTokensByFamilyGeneric(ctx, familyID)
}

// ListRefreshTokensBySession returns runtime refresh tokens for a specific session.
func (r *AccountRepository) ListRefreshTokensBySession(ctx context.Context, sessionID string) ([]storage.RefreshToken, error) {
	helper := NewOAuthHelper(r.db, r.logger)
	return helper.ListRefreshTokensBySessionGeneric(ctx, sessionID)
}

// RevokeAccessToken stores the JWT ID (JTI) of an access token as revoked.
func (r *AccountRepository) RevokeAccessToken(ctx context.Context, jti string, expiresAt time.Time) error {
	helper := NewOAuthHelper(r.db, r.logger)
	return helper.RevokeAccessTokenGeneric(ctx, jti, expiresAt)
}

// IsAccessTokenRevoked checks whether the given JWT ID (JTI) is revoked.
func (r *AccountRepository) IsAccessTokenRevoked(ctx context.Context, jti string) (bool, error) {
	helper := NewOAuthHelper(r.db, r.logger)
	return helper.IsAccessTokenRevokedGeneric(ctx, jti)
}

// DeleteRefreshTokensByUsernameAndClientID deletes all OAuth refresh tokens for the given user+client pair.
// This is intended for revoking delegated/agent tokens where only refresh-token invalidation is required.
func (r *AccountRepository) DeleteRefreshTokensByUsernameAndClientID(ctx context.Context, username, clientID string) (int, error) {
	username = strings.TrimSpace(username)
	clientID = strings.TrimSpace(clientID)
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return 0, err
	}
	if err := common.ValidateRequiredParam("clientID", clientID); err != nil {
		return 0, err
	}

	var tokenModels []models.RefreshToken
	query := r.db.WithContext(ctx).Model(&models.RefreshToken{}).
		Where("Username", "=", username).
		Where("ClientID", "=", clientID)
	if err := query.Scan(&tokenModels); err != nil {
		if errors.IsNotFound(err) {
			return 0, nil
		}
		return 0, ErrorHandler.HandleQueryError(err, EntityRefreshToken, "by username/clientID")
	}

	helper := NewOAuthHelper(r.db, r.logger)
	deleted := 0
	for _, tokenModel := range tokenModels {
		tokenValue := strings.TrimSpace(tokenModel.Token)
		if tokenValue == "" {
			continue
		}
		if err := helper.DeleteRefreshTokenGeneric(ctx, tokenValue); err != nil && !errors.IsNotFound(err) {
			return deleted, err
		}
		deleted++
	}

	return deleted, nil
}

// ===== OAuth Client Operations =====

// CreateOAuthClient creates a new OAuth client
//
//nolint:dupl // OAuth client operations are shared between account and oauth repositories
func (r *AccountRepository) CreateOAuthClient(ctx context.Context, client *storage.OAuthClient) error {
	// Validate required fields
	if err := common.ValidateRequiredParam("client.Name", client.Name); err != nil {
		return ErrorHandler.HandleCreateError(ErrOAuthClientNameRequired, EntityOAuthClient, "validation")
	}
	if err := common.ValidateSliceNotEmpty("client.RedirectURIs", client.RedirectURIs); err != nil {
		return ErrorHandler.HandleCreateError(ErrOAuthRedirectURIsRequired, EntityOAuthClient, "validation")
	}

	// Generate client ID if not provided
	if err := common.ValidateRequiredParam("client.ClientID", client.ClientID); err != nil {
		clientID, err := generateClientID()
		if err != nil {
			return ErrorHandler.HandleCreateError(err, EntityOAuthClient, "client_id_generation")
		}
		client.ClientID = clientID
	}

	// Generate client secret if not provided
	if err := common.ValidateRequiredParam("client.ClientSecret", client.ClientSecret); err != nil {
		clientSecret, err := generateClientSecret()
		if err != nil {
			return ErrorHandler.HandleCreateError(err, EntityOAuthClient, "client_secret_generation")
		}
		client.ClientSecret = clientSecret
	}

	storedSecret, err := common.HashOAuthClientSecret(client.ClientSecret)
	if err != nil {
		return ErrorHandler.HandleCreateError(err, EntityOAuthClient, "client_secret_hashing")
	}

	// Create DynamORM model
	model := &models.OAuthClient{
		ClientID:     client.ClientID,
		ClientSecret: storedSecret,
		Name:         client.Name,
		Website:      client.Website,
		RedirectURIs: client.RedirectURIs,
		Scopes:       client.Scopes,
		ClientClass:  client.ClientClass,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	// BeforeCreate will set up keys
	if err := model.BeforeCreate(); err != nil {
		return ErrorHandler.HandleCreateError(err, EntityOAuthClient, "preparation")
	}

	// Create the item with condition that it doesn't exist
	err = r.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		// Check if it's a duplicate key error
		if strings.Contains(err.Error(), "ConditionalCheckFailed") || strings.Contains(err.Error(), "already exists") {
			return ErrorHandler.HandleCreateError(ErrOAuthClientAlreadyExists, EntityOAuthClient, client.ClientID)
		}
		r.logger.Error("failed to create OAuth client", zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityOAuthClient, client.ClientID)
	}

	// Update the input client with generated values
	client.CreatedAt = model.CreatedAt
	client.UpdatedAt = model.UpdatedAt
	client.ClientSecretHash = storedSecret

	r.logger.Debug("created OAuth client",
		zap.String("client_id", client.ClientID),
		zap.String("name", client.Name))

	return nil
}

// GetOAuthClient retrieves an OAuth client by client ID
func (r *AccountRepository) GetOAuthClient(ctx context.Context, clientID string) (*storage.OAuthClient, error) {
	// Construct the key using the correct pattern from existing OAuth repository
	pk := "OAUTH_CLIENT#" + clientID
	sk := oauthClientSortKey

	// Query for the item
	var model models.OAuthClient
	err := r.db.WithContext(ctx).Model(&models.OAuthClient{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(err, EntityOAuthClient, clientID)
		}
		r.logger.Error("failed to get OAuth client", zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, EntityOAuthClient, clientID)
	}

	// Convert to storage model
	result := &storage.OAuthClient{
		ClientID:         model.ClientID,
		ClientSecretHash: model.ClientSecret,
		Name:             model.Name,
		Description:      model.Description,
		Website:          model.Website,
		RedirectURIs:     model.RedirectURIs,
		GrantTypes:       model.GrantTypes,
		Scopes:           model.Scopes,
		ClientClass:      model.ClientClass,
		OwnerID:          model.OwnerID,
		Confidential:     model.Confidential,
		CreatedAt:        model.CreatedAt,
		UpdatedAt:        model.UpdatedAt,
	}

	r.logger.Debug("retrieved OAuth client",
		zap.String("client_id", clientID),
		zap.String("name", result.Name))

	return result, nil
}

// UpdateOAuthClientSecretHash updates the stored secret representation for an OAuth client.
// The value must be a hash string (e.g. prefixed bcrypt hash) and should never be a plaintext secret.
func (r *AccountRepository) UpdateOAuthClientSecretHash(ctx context.Context, clientID, clientSecretHash string) error {
	if err := common.ValidateMultipleRequiredParams(map[string]string{
		"clientID":         clientID,
		"clientSecretHash": clientSecretHash,
	}); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityOAuthClient, "validation")
	}

	pk := "OAUTH_CLIENT#" + clientID
	sk := oauthClientSortKey

	updateBuilder := r.db.WithContext(ctx).Model(&models.OAuthClient{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		UpdateBuilder()
	updateBuilder.Set("ClientSecret", clientSecretHash)
	updateBuilder.Set("UpdatedAt", time.Now().UTC())

	if err := updateBuilder.Execute(); err != nil {
		r.logger.Error("failed to update OAuth client secret hash",
			zap.String("client_id", clientID),
			zap.Error(err))
		return ErrorHandler.HandleUpdateError(err, EntityOAuthClient, clientID)
	}

	return nil
}

// UpdateOAuthClient updates an existing OAuth client
//
//nolint:dupl // OAuth client operations are shared between account and oauth repositories
func (r *AccountRepository) UpdateOAuthClient(ctx context.Context, clientID string, updates map[string]any) error {
	if len(updates) == 0 {
		return ErrorHandler.HandleUpdateError(ErrOAuthNoUpdatesProvided, EntityOAuthClient, "validation")
	}

	// Construct the key
	pk := "OAUTH_CLIENT#" + clientID
	sk := oauthClientSortKey

	// First, get the existing client
	var existingClient models.OAuthClient
	err := r.db.WithContext(ctx).Model(&models.OAuthClient{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&existingClient)

	if err != nil {
		if errors.IsNotFound(err) {
			return ErrorHandler.HandleGetError(err, EntityOAuthClient, clientID)
		}
		return ErrorHandler.HandleGetError(err, EntityOAuthClient, clientID)
	}

	// Only allow specific fields to be updated
	allowedFields := map[string]bool{
		FieldName:         true,
		FieldWebsite:      true,
		FieldRedirectURIs: true,
		FieldScopes:       true,
	}

	// Apply updates to the model
	for key, value := range updates {
		if allowedFields[key] {
			switch key {
			case FieldName:
				if v, ok := value.(string); ok {
					existingClient.Name = v
				}
			case FieldWebsite:
				if v, ok := value.(string); ok {
					existingClient.Website = v
				}
			case FieldRedirectURIs:
				if v, ok := value.([]string); ok {
					existingClient.RedirectURIs = v
				}
			case FieldScopes:
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
			return ErrorHandler.HandleUpdateError(err, EntityOAuthClient, clientID)
		}
		r.logger.Error("failed to update OAuth client", zap.Error(err))
		return ErrorHandler.HandleUpdateError(err, EntityOAuthClient, clientID)
	}

	r.logger.Debug("updated OAuth client",
		zap.String("client_id", clientID),
		zap.Any("updates", updates))

	return nil
}

// DeleteOAuthClient deletes an OAuth client
func (r *AccountRepository) DeleteOAuthClient(ctx context.Context, clientID string) error {
	// Construct the key
	pk := "OAUTH_CLIENT#" + clientID
	sk := oauthClientSortKey

	// Delete the item
	err := r.db.WithContext(ctx).Model(&models.OAuthClient{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		Delete()

	if err != nil {
		r.logger.Error("failed to delete OAuth client", zap.Error(err))
		return ErrorHandler.HandleDeleteError(err, EntityOAuthClient, clientID)
	}

	r.logger.Debug("deleted OAuth client", zap.String("client_id", clientID))
	return nil
}

// ListOAuthClients lists OAuth clients with deterministic cursor-based pagination.
//
//nolint:dupl // OAuth client operations are shared between account and oauth repositories
func (r *AccountRepository) ListOAuthClients(ctx context.Context, limit int, cursor string) ([]*storage.OAuthClient, string, error) {
	// Normalize pagination parameters
	switch {
	case limit <= 0:
		limit = oauthClientsDefaultLimit
	case limit > oauthClientsMaxAllowedLimit:
		limit = oauthClientsMaxAllowedLimit
	}

	var startKey string
	if cursor != "" {
		pk, sk, err := Utils.Pagination.DecodeCursor(cursor)
		if err != nil {
			r.logger.Error("failed to decode OAuth client cursor",
				zap.String("cursor", cursor),
				zap.Error(err))
			return nil, "", err
		}
		if pk != oauthClientsIndexPK {
			err := PaginationCursorData(cursor, "unexpected partition key for OAuth client listing")
			r.logger.Error("invalid OAuth client cursor partition key",
				zap.String("cursor", cursor),
				zap.Error(err))
			return nil, "", err
		}
		if sk == "" {
			err := PaginationCursorData(cursor, "missing sort key for OAuth client listing")
			r.logger.Error("invalid OAuth client cursor sort key",
				zap.String("cursor", cursor),
				zap.Error(err))
			return nil, "", err
		}
		startKey = sk
	}

	query := r.db.WithContext(ctx).Model(&models.OAuthClient{}).
		Index(oauthClientsIndexName).
		Where(oauthClientsPartitionAttr, "=", oauthClientsIndexPK).
		OrderBy(oauthClientsSortAttr, "ASC").
		Limit(limit + 1) // Fetch one extra to compute the next cursor

	if startKey != "" {
		query = query.Where(oauthClientsSortAttr, ">", startKey)
	}

	var clientModels []*models.OAuthClient
	if err := query.All(&clientModels); err != nil {
		r.logger.Error("failed to list OAuth clients", zap.Error(err))
		return nil, "", ErrorHandler.HandleQueryError(err, EntityOAuthClient, "list clients")
	}

	hasMore := len(clientModels) > limit
	if hasMore {
		clientModels = clientModels[:limit]
	}

	clients := make([]*storage.OAuthClient, len(clientModels))
	for i, model := range clientModels {
		clients[i] = &storage.OAuthClient{
			ClientID:         model.ClientID,
			ClientSecretHash: model.ClientSecret,
			Name:             model.Name,
			Website:          model.Website,
			RedirectURIs:     model.RedirectURIs,
			Scopes:           model.Scopes,
			CreatedAt:        model.CreatedAt,
			UpdatedAt:        model.UpdatedAt,
		}
	}

	nextCursor := ""
	if hasMore && len(clientModels) > 0 {
		last := clientModels[len(clientModels)-1]
		nextCursor = Utils.Pagination.EncodeCursor(oauthClientsIndexPK, last.OAuthClientsSK)
	}

	r.logger.Debug("listed OAuth clients",
		zap.Int("count", len(clients)),
		zap.Bool("has_more", hasMore))
	return clients, nextCursor, nil
}

// GetOAuthApp retrieves an OAuth application by client ID (alias for GetOAuthClient)
func (r *AccountRepository) GetOAuthApp(ctx context.Context, clientID string) (*storage.OAuthApp, error) {
	client, err := r.GetOAuthClient(ctx, clientID)
	if err != nil {
		return nil, err
	}

	return &storage.OAuthApp{
		ClientID:         client.ClientID,
		ClientSecretHash: client.ClientSecretHash,
		Name:             client.Name,
		RedirectURIs:     client.RedirectURIs,
		Scopes:           client.Scopes,
		CreatedAt:        client.CreatedAt,
	}, nil
}

// SaveUserAppConsent saves user consent for an OAuth app
func (r *AccountRepository) SaveUserAppConsent(ctx context.Context, consent *storage.UserAppConsent) error {
	helper := NewOAuthHelper(r.db, r.logger)
	return helper.SaveUserAppConsentGeneric(ctx, consent)
}

// GetUserAppConsent retrieves user consent for an OAuth app
func (r *AccountRepository) GetUserAppConsent(ctx context.Context, userID, appID string) (*storage.UserAppConsent, error) {
	helper := NewOAuthHelper(r.db, r.logger)
	return helper.GetUserAppConsentGeneric(ctx, userID, appID)
}
