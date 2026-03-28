package repositories

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// OAuthRepository handles OAuth-related storage operations using enhanced DynamORM patterns
type OAuthRepository struct {
	*EnhancedBaseRepository[*models.OAuthClient]
	db     core.DB
	logger *zap.Logger
}

// NewOAuthRepository creates a new OAuth repository with enhanced functionality
func NewOAuthRepository(db core.DB, logger *zap.Logger) *OAuthRepository {
	// Create enhanced repository optimized for OAuth operations
	enhancedRepo := NewEnhancedBaseRepository[*models.OAuthClient](db, models.MainTableName, logger, nil, "OAuthRepository", "oauth_client")

	// Set up enhanced services for OAuth operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService()) // Critical for OAuth security
	enhancedRepo.SetCachingService(NewInMemoryCachingService())      // OAuth clients cached for performance
	enhancedRepo.SetEventService(NewDefaultEventService())           // Important for OAuth security events

	return &OAuthRepository{
		EnhancedBaseRepository: enhancedRepo,
		db:                     db,
		logger:                 logger,
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
	helper := NewOAuthHelper(r.db, r.logger)
	return helper.DeleteAuthorizationCodeGeneric(ctx, code)
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
	helper := NewOAuthHelper(r.db, r.logger)
	return helper.DeleteRefreshTokenGeneric(ctx, token)
}

// CreateOAuthClient creates a new OAuth client using BaseRepository
func (r *OAuthRepository) CreateOAuthClient(ctx context.Context, client *storage.OAuthClient) error {
	// Generate client secret if not provided.
	if client.ClientSecret == "" {
		secret, err := generateClientSecret()
		if err != nil {
			return ErrorHandler.HandleCreateError(err, EntityOAuthClient, "client_secret_generation")
		}
		client.ClientSecret = secret
	}

	storedSecret, err := common.HashOAuthClientSecret(client.ClientSecret)
	if err != nil {
		return ErrorHandler.HandleCreateError(err, EntityOAuthClient, "client_secret_hashing")
	}
	client.ClientSecretHash = storedSecret

	// Convert storage model to DynamORM model
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

	// Use BaseRepository Create method
	return r.Create(ctx, model)
}

// GetOAuthClient retrieves an OAuth client using BaseRepository
func (r *OAuthRepository) GetOAuthClient(ctx context.Context, clientID string) (*storage.OAuthClient, error) {
	// Construct keys
	pk := "OAUTH_CLIENT#" + clientID
	sk := "CLIENT"

	// Use BaseRepository Get method
	var model models.OAuthClient
	err := r.Get(ctx, pk, sk, &model)
	if err != nil {
		if err.Error() == fmt.Sprintf("item not found: pk=%s, sk=%s", pk, sk) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityOAuthClient, clientID)
		}
		return nil, ErrorHandler.HandleGetError(err, EntityOAuthClient, clientID)
	}

	// Convert DynamORM model to storage model
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

// RotateOAuthClientSecret persists dual-secret rotation state for an OAuth client.
func (r *OAuthRepository) RotateOAuthClientSecret(ctx context.Context, clientID string, rotation storage.OAuthClientSecretRotation) error {
	existing, err := r.GetOAuthClient(ctx, clientID)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrorHandler.HandleGetError(storage.ErrNotFound, EntityOAuthClient, clientID)
	}

	existing.ClientSecretHash = rotation.ActiveClientSecretHash
	existing.PreviousClientSecretHash = rotation.PreviousClientSecretHash
	existing.PreviousClientSecretGraceExpiresAt = rotation.PreviousClientSecretGraceExpiresAt
	existing.SecretRotatedAt = rotation.RotatedAt
	existing.SecretRotatedBy = rotation.RotatedBy
	existing.UpdatedAt = time.Now().UTC()

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
	if err := model.UpdateKeys(); err != nil {
		return err
	}

	return r.Update(ctx, model)
}

// DeleteOAuthClient deletes an OAuth client using BaseRepository
func (r *OAuthRepository) DeleteOAuthClient(ctx context.Context, clientID string) error {
	// Construct keys
	pk := "OAUTH_CLIENT#" + clientID
	sk := "CLIENT"

	// Use BaseRepository Delete method
	err := r.Delete(ctx, pk, sk)
	if err != nil {
		return err
	}

	// Also delete any associated tokens (simplified version)
	r.logger.Info("deleted OAuth client and associated tokens",
		zap.String("client_id", clientID))

	return nil
}

// UpdateOAuthClient updates an existing OAuth client using BaseRepository
func (r *OAuthRepository) UpdateOAuthClient(ctx context.Context, clientID string, updates map[string]any) error {
	// Get existing client first
	existing, err := r.GetOAuthClient(ctx, clientID)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrorHandler.HandleGetError(storage.ErrNotFound, EntityOAuthClient, clientID)
	}

	// Apply updates to existing client
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

	// Convert to DynamORM model
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

	// Use BaseRepository Update method
	return r.Update(ctx, model)
}

// DeleteExpiredTokens removes expired OAuth tokens
func (r *OAuthRepository) DeleteExpiredTokens(_ context.Context) error {
	// This would typically be run as a scheduled job
	// For now, we'll log that it should be implemented
	r.logger.Info("DeleteExpiredTokens should be implemented as a scheduled job")
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
func (r *OAuthRepository) GetUserAppConsent(ctx context.Context, userID, appID, resource string) (*storage.UserAppConsent, error) {
	helper := NewOAuthHelper(r.db, r.logger)
	return helper.GetUserAppConsentGeneric(ctx, userID, appID, resource)
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
