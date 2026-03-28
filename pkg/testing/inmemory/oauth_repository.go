// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
)

// OAuthRepository is a thread-safe in-memory implementation of interfaces.OAuthRepository.
type OAuthRepository struct {
	mu sync.RWMutex

	// OAuth states: state -> OAuthState
	states map[string]*storage.OAuthState

	// Authorization codes: code -> AuthorizationCode
	authCodes map[string]*storage.AuthorizationCode

	// Refresh tokens: token -> RefreshToken
	refreshTokens map[string]*storage.RefreshToken

	// OAuth clients: clientID -> OAuthClient
	clients map[string]*storage.OAuthClient

	// User app consents: userID_appID_resource -> UserAppConsent
	consents map[string]*storage.UserAppConsent
}

// NewOAuthRepository creates a new in-memory OAuth repository
func NewOAuthRepository() *OAuthRepository {
	return &OAuthRepository{
		states:        make(map[string]*storage.OAuthState),
		authCodes:     make(map[string]*storage.AuthorizationCode),
		refreshTokens: make(map[string]*storage.RefreshToken),
		clients:       make(map[string]*storage.OAuthClient),
		consents:      make(map[string]*storage.UserAppConsent),
	}
}

// ===== OAuth State Operations =====

// StoreOAuthState stores OAuth state for CSRF protection
func (r *OAuthRepository) StoreOAuthState(_ context.Context, state string, data *storage.OAuthState) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.states[state] = data
	return nil
}

// GetOAuthState retrieves OAuth state
func (r *OAuthRepository) GetOAuthState(_ context.Context, state string) (*storage.OAuthState, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	data, exists := r.states[state]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return data, nil
}

// DeleteOAuthState deletes OAuth state
func (r *OAuthRepository) DeleteOAuthState(_ context.Context, state string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.states, state)
	return nil
}

// ===== Authorization Code Operations =====

// CreateAuthorizationCode creates a new OAuth authorization code
func (r *OAuthRepository) CreateAuthorizationCode(_ context.Context, code *storage.AuthorizationCode) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.authCodes[code.Code] = code
	return nil
}

// GetAuthorizationCode retrieves an OAuth authorization code
func (r *OAuthRepository) GetAuthorizationCode(_ context.Context, code string) (*storage.AuthorizationCode, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	authCode, exists := r.authCodes[code]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return authCode, nil
}

// DeleteAuthorizationCode deletes an OAuth authorization code
func (r *OAuthRepository) DeleteAuthorizationCode(_ context.Context, code string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.authCodes[code]; !exists {
		return storage.ErrNotFound
	}
	delete(r.authCodes, code)
	return nil
}

// ===== Refresh Token Operations =====

// CreateRefreshToken creates a new OAuth refresh token
func (r *OAuthRepository) CreateRefreshToken(_ context.Context, token *storage.RefreshToken) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.refreshTokens[token.Token] = token
	return nil
}

// GetRefreshToken retrieves an OAuth refresh token
func (r *OAuthRepository) GetRefreshToken(_ context.Context, token string) (*storage.RefreshToken, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	refreshToken, exists := r.refreshTokens[token]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return refreshToken, nil
}

// DeleteRefreshToken deletes an OAuth refresh token
func (r *OAuthRepository) DeleteRefreshToken(_ context.Context, token string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.refreshTokens, token)
	return nil
}

// ===== OAuth Client Operations =====

// CreateOAuthClient creates a new OAuth client
func (r *OAuthRepository) CreateOAuthClient(_ context.Context, client *storage.OAuthClient) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.clients[client.ClientID]; exists {
		return storage.ErrAlreadyExists
	}

	if client.CreatedAt.IsZero() {
		client.CreatedAt = time.Now()
	}
	client.UpdatedAt = time.Now()

	r.clients[client.ClientID] = client
	return nil
}

// GetOAuthClient retrieves an OAuth client by client ID
func (r *OAuthRepository) GetOAuthClient(_ context.Context, clientID string) (*storage.OAuthClient, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	client, exists := r.clients[clientID]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return client, nil
}

// UpdateOAuthClient updates an existing OAuth client
func (r *OAuthRepository) UpdateOAuthClient(_ context.Context, clientID string, updates map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	client, exists := r.clients[clientID]
	if !exists {
		return storage.ErrNotFound
	}

	// Apply updates
	for key, value := range updates {
		applyInMemoryOAuthClientUpdate(client, key, value)
	}

	client.UpdatedAt = time.Now()
	return nil
}

// RotateOAuthClientSecret updates the persisted dual-secret rotation state for a client.
func (r *OAuthRepository) RotateOAuthClientSecret(_ context.Context, clientID string, rotation storage.OAuthClientSecretRotation) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	client, exists := r.clients[clientID]
	if !exists {
		return storage.ErrNotFound
	}

	client.ClientSecretHash = rotation.ActiveClientSecretHash
	client.PreviousClientSecretHash = rotation.PreviousClientSecretHash
	client.PreviousClientSecretGraceExpiresAt = rotation.PreviousClientSecretGraceExpiresAt
	client.SecretRotatedAt = rotation.RotatedAt
	client.SecretRotatedBy = rotation.RotatedBy
	client.UpdatedAt = time.Now().UTC()
	return nil
}

func applyInMemoryOAuthClientUpdate(client *storage.OAuthClient, key string, value any) {
	if client == nil {
		return
	}

	switch key {
	case "name":
		if v, ok := value.(string); ok {
			client.Name = v
		}
	case "description":
		if v, ok := value.(string); ok {
			client.Description = v
		}
	case "redirect_uris":
		if v, ok := value.([]string); ok {
			client.RedirectURIs = v
		}
	case "grant_types":
		if v, ok := value.([]string); ok {
			client.GrantTypes = v
		}
	case "response_types":
		if v, ok := value.([]string); ok {
			client.ResponseTypes = v
		}
	case "scopes":
		if v, ok := value.([]string); ok {
			client.Scopes = v
		}
	case "website":
		if v, ok := value.(string); ok {
			client.Website = v
		}
	case "confidential":
		if v, ok := value.(bool); ok {
			client.Confidential = v
		}
	case "registration_source":
		if v, ok := value.(string); ok {
			client.RegistrationSource = v
		}
	}
}

// DeleteOAuthClient deletes an OAuth client
func (r *OAuthRepository) DeleteOAuthClient(_ context.Context, clientID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.clients, clientID)
	return nil
}

// ListOAuthClients lists OAuth clients with pagination
func (r *OAuthRepository) ListOAuthClients(_ context.Context, limit int32, cursor string) ([]*storage.OAuthClient, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Collect all clients
	var clients []*storage.OAuthClient
	for _, client := range r.clients {
		clients = append(clients, client)
	}

	// Sort by created at descending
	sort.Slice(clients, func(i, j int) bool {
		return clients[i].CreatedAt.After(clients[j].CreatedAt)
	})

	// Apply cursor
	startIdx := 0
	if cursor != "" {
		for i, client := range clients {
			if client.ClientID == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	// Apply limit
	if limit <= 0 {
		limit = 20
	}
	endIdx := startIdx + int(limit)
	if endIdx > len(clients) {
		endIdx = len(clients)
	}

	result := clients[startIdx:endIdx]
	nextCursor := ""
	if endIdx < len(clients) && len(result) > 0 {
		nextCursor = result[len(result)-1].ClientID
	}

	return result, nextCursor, nil
}

// ===== Token Cleanup =====

// DeleteExpiredTokens removes expired OAuth tokens
func (r *OAuthRepository) DeleteExpiredTokens(_ context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()

	// Clean up expired auth codes
	for code, authCode := range r.authCodes {
		if authCode.ExpiresAt.Before(now) {
			delete(r.authCodes, code)
		}
	}

	// Clean up expired refresh tokens
	for token, refreshToken := range r.refreshTokens {
		if refreshToken.ExpiresAt.Before(now) {
			delete(r.refreshTokens, token)
		}
	}

	return nil
}

// ===== User Consent Operations =====

// SaveUserAppConsent saves user consent for an OAuth app
func (r *OAuthRepository) SaveUserAppConsent(_ context.Context, consent *storage.UserAppConsent) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := consent.UserID + "_" + consent.AppID + "_" + consent.Resource
	r.consents[key] = consent
	return nil
}

// GetUserAppConsent retrieves user consent for an OAuth app
func (r *OAuthRepository) GetUserAppConsent(_ context.Context, userID, appID, resource string) (*storage.UserAppConsent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := userID + "_" + appID + "_" + resource
	consent, exists := r.consents[key]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return consent, nil
}

// Clear clears all data (test helper)
func (r *OAuthRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.states = make(map[string]*storage.OAuthState)
	r.authCodes = make(map[string]*storage.AuthorizationCode)
	r.refreshTokens = make(map[string]*storage.RefreshToken)
	r.clients = make(map[string]*storage.OAuthClient)
	r.consents = make(map[string]*storage.UserAppConsent)
}

// Ensure OAuthRepository implements interfaces.OAuthRepository
var _ interfaces.OAuthRepository = (*OAuthRepository)(nil)
