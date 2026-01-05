// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/stretchr/testify/mock"
)

// MockOAuthRepository is a mock implementation of interfaces.OAuthRepository
// using testify/mock for expectation-based testing.
type MockOAuthRepository struct {
	mock.Mock
}

// NewMockOAuthRepository creates a new mock OAuth repository
func NewMockOAuthRepository() *MockOAuthRepository {
	return &MockOAuthRepository{}
}

// ===== OAuth State Operations =====

// StoreOAuthState mocks the StoreOAuthState method
func (m *MockOAuthRepository) StoreOAuthState(ctx context.Context, state string, data *storage.OAuthState) error {
	args := m.Called(ctx, state, data)
	return args.Error(0)
}

// GetOAuthState mocks the GetOAuthState method
func (m *MockOAuthRepository) GetOAuthState(ctx context.Context, state string) (*storage.OAuthState, error) {
	args := m.Called(ctx, state)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.OAuthState), args.Error(1)
}

// DeleteOAuthState mocks the DeleteOAuthState method
func (m *MockOAuthRepository) DeleteOAuthState(ctx context.Context, state string) error {
	args := m.Called(ctx, state)
	return args.Error(0)
}

// ===== Authorization Code Operations =====

// CreateAuthorizationCode mocks the CreateAuthorizationCode method
func (m *MockOAuthRepository) CreateAuthorizationCode(ctx context.Context, code *storage.AuthorizationCode) error {
	args := m.Called(ctx, code)
	return args.Error(0)
}

// GetAuthorizationCode mocks the GetAuthorizationCode method
func (m *MockOAuthRepository) GetAuthorizationCode(ctx context.Context, code string) (*storage.AuthorizationCode, error) {
	args := m.Called(ctx, code)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.AuthorizationCode), args.Error(1)
}

// DeleteAuthorizationCode mocks the DeleteAuthorizationCode method
func (m *MockOAuthRepository) DeleteAuthorizationCode(ctx context.Context, code string) error {
	args := m.Called(ctx, code)
	return args.Error(0)
}

// ===== Refresh Token Operations =====

// CreateRefreshToken mocks the CreateRefreshToken method
func (m *MockOAuthRepository) CreateRefreshToken(ctx context.Context, token *storage.RefreshToken) error {
	args := m.Called(ctx, token)
	return args.Error(0)
}

// GetRefreshToken mocks the GetRefreshToken method
func (m *MockOAuthRepository) GetRefreshToken(ctx context.Context, token string) (*storage.RefreshToken, error) {
	args := m.Called(ctx, token)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.RefreshToken), args.Error(1)
}

// DeleteRefreshToken mocks the DeleteRefreshToken method
func (m *MockOAuthRepository) DeleteRefreshToken(ctx context.Context, token string) error {
	args := m.Called(ctx, token)
	return args.Error(0)
}

// ===== OAuth Client Operations =====

// CreateOAuthClient mocks the CreateOAuthClient method
func (m *MockOAuthRepository) CreateOAuthClient(ctx context.Context, client *storage.OAuthClient) error {
	args := m.Called(ctx, client)
	return args.Error(0)
}

// GetOAuthClient mocks the GetOAuthClient method
func (m *MockOAuthRepository) GetOAuthClient(ctx context.Context, clientID string) (*storage.OAuthClient, error) {
	args := m.Called(ctx, clientID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.OAuthClient), args.Error(1)
}

// UpdateOAuthClient mocks the UpdateOAuthClient method
func (m *MockOAuthRepository) UpdateOAuthClient(ctx context.Context, clientID string, updates map[string]any) error {
	args := m.Called(ctx, clientID, updates)
	return args.Error(0)
}

// DeleteOAuthClient mocks the DeleteOAuthClient method
func (m *MockOAuthRepository) DeleteOAuthClient(ctx context.Context, clientID string) error {
	args := m.Called(ctx, clientID)
	return args.Error(0)
}

// ListOAuthClients mocks the ListOAuthClients method
func (m *MockOAuthRepository) ListOAuthClients(ctx context.Context, limit int32, cursor string) ([]*storage.OAuthClient, string, error) {
	args := m.Called(ctx, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.OAuthClient), args.String(1), args.Error(2)
}

// ===== Token Cleanup =====

// DeleteExpiredTokens mocks the DeleteExpiredTokens method
func (m *MockOAuthRepository) DeleteExpiredTokens(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

// ===== User Consent Operations =====

// SaveUserAppConsent mocks the SaveUserAppConsent method
func (m *MockOAuthRepository) SaveUserAppConsent(ctx context.Context, consent *storage.UserAppConsent) error {
	args := m.Called(ctx, consent)
	return args.Error(0)
}

// GetUserAppConsent mocks the GetUserAppConsent method
func (m *MockOAuthRepository) GetUserAppConsent(ctx context.Context, userID, appID string) (*storage.UserAppConsent, error) {
	args := m.Called(ctx, userID, appID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.UserAppConsent), args.Error(1)
}

// Ensure MockOAuthRepository implements interfaces.OAuthRepository
var _ interfaces.OAuthRepository = (*MockOAuthRepository)(nil)
