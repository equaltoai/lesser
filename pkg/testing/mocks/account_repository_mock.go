// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
)

// MockAccountRepository is a mock implementation of interfaces.AccountRepository
// using testify/mock for expectation-based testing.
type MockAccountRepository struct {
	mock.Mock
}

// NewMockAccountRepository creates a new mock account repository
func NewMockAccountRepository() *MockAccountRepository {
	return &MockAccountRepository{}
}

// Core account operations

// CreateAccount mocks the CreateAccount method
func (m *MockAccountRepository) CreateAccount(ctx context.Context, account *storage.Account) error {
	args := m.Called(ctx, account)
	return args.Error(0)
}

// GetAccount mocks the GetAccount method
func (m *MockAccountRepository) GetAccount(ctx context.Context, username string) (*storage.Account, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Account), args.Error(1)
}

// GetAccountByURL mocks the GetAccountByURL method
func (m *MockAccountRepository) GetAccountByURL(ctx context.Context, actorURL string) (*storage.Account, error) {
	args := m.Called(ctx, actorURL)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Account), args.Error(1)
}

// GetAccountByEmail mocks the GetAccountByEmail method
func (m *MockAccountRepository) GetAccountByEmail(ctx context.Context, email string) (*storage.Account, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Account), args.Error(1)
}

// UpdateAccount mocks the UpdateAccount method
func (m *MockAccountRepository) UpdateAccount(ctx context.Context, account *storage.Account) error {
	args := m.Called(ctx, account)
	return args.Error(0)
}

// DeleteAccount mocks the DeleteAccount method
func (m *MockAccountRepository) DeleteAccount(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

// Account discovery and search

// SearchAccounts mocks the SearchAccounts method
func (m *MockAccountRepository) SearchAccounts(ctx context.Context, query string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	args := m.Called(ctx, query, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*storage.Account]), args.Error(1)
}

// GetSuggestedAccounts mocks the GetSuggestedAccounts method
func (m *MockAccountRepository) GetSuggestedAccounts(ctx context.Context, forUserID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.AccountSuggestion], error) {
	args := m.Called(ctx, forUserID, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*storage.AccountSuggestion]), args.Error(1)
}

// GetFeaturedAccounts mocks the GetFeaturedAccounts method
func (m *MockAccountRepository) GetFeaturedAccounts(ctx context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	args := m.Called(ctx, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*storage.Account]), args.Error(1)
}

// Account verification and moderation

// ApproveAccount mocks the ApproveAccount method
func (m *MockAccountRepository) ApproveAccount(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

// SuspendAccount mocks the SuspendAccount method
func (m *MockAccountRepository) SuspendAccount(ctx context.Context, username string, reason string) error {
	args := m.Called(ctx, username, reason)
	return args.Error(0)
}

// UnsuspendAccount mocks the UnsuspendAccount method
func (m *MockAccountRepository) UnsuspendAccount(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

// SilenceAccount mocks the SilenceAccount method
func (m *MockAccountRepository) SilenceAccount(ctx context.Context, username string, reason string) error {
	args := m.Called(ctx, username, reason)
	return args.Error(0)
}

// UnsilenceAccount mocks the UnsilenceAccount method
func (m *MockAccountRepository) UnsilenceAccount(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

// Account metadata and preferences

// UpdateAccountPreferences mocks the UpdateAccountPreferences method
func (m *MockAccountRepository) UpdateAccountPreferences(ctx context.Context, username string, preferences map[string]interface{}) error {
	args := m.Called(ctx, username, preferences)
	return args.Error(0)
}

// GetAccountPreferences mocks the GetAccountPreferences method
func (m *MockAccountRepository) GetAccountPreferences(ctx context.Context, username string) (map[string]interface{}, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]interface{}), args.Error(1)
}

// UpdateAccountFeatures mocks the UpdateAccountFeatures method
func (m *MockAccountRepository) UpdateAccountFeatures(ctx context.Context, username string, features map[string]bool) error {
	args := m.Called(ctx, username, features)
	return args.Error(0)
}

// GetAccountFeatures mocks the GetAccountFeatures method
func (m *MockAccountRepository) GetAccountFeatures(ctx context.Context, username string) (map[string]bool, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]bool), args.Error(1)
}

// Authentication and session management

// ValidateCredentials mocks the ValidateCredentials method
func (m *MockAccountRepository) ValidateCredentials(ctx context.Context, username, password string) (*storage.Account, error) {
	args := m.Called(ctx, username, password)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Account), args.Error(1)
}

// UpdatePassword mocks the UpdatePassword method
func (m *MockAccountRepository) UpdatePassword(ctx context.Context, username, newPasswordHash string) error {
	args := m.Called(ctx, username, newPasswordHash)
	return args.Error(0)
}

// CreatePasswordReset mocks the CreatePasswordReset method
func (m *MockAccountRepository) CreatePasswordReset(ctx context.Context, reset *storage.PasswordReset) error {
	args := m.Called(ctx, reset)
	return args.Error(0)
}

// GetPasswordReset mocks the GetPasswordReset method
func (m *MockAccountRepository) GetPasswordReset(ctx context.Context, token string) (*storage.PasswordReset, error) {
	args := m.Called(ctx, token)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.PasswordReset), args.Error(1)
}

// UsePasswordReset mocks the UsePasswordReset method
func (m *MockAccountRepository) UsePasswordReset(ctx context.Context, token string) error {
	args := m.Called(ctx, token)
	return args.Error(0)
}

// Activity and usage tracking

// RecordLogin mocks the RecordLogin method
func (m *MockAccountRepository) RecordLogin(ctx context.Context, attempt *storage.LoginAttempt) error {
	args := m.Called(ctx, attempt)
	return args.Error(0)
}

// GetLoginHistory mocks the GetLoginHistory method
func (m *MockAccountRepository) GetLoginHistory(ctx context.Context, username string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.LoginAttempt], error) {
	args := m.Called(ctx, username, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*storage.LoginAttempt]), args.Error(1)
}

// UpdateLastActivity mocks the UpdateLastActivity method
func (m *MockAccountRepository) UpdateLastActivity(ctx context.Context, username string, activity time.Time) error {
	args := m.Called(ctx, username, activity)
	return args.Error(0)
}

// Bookmark operations

// AddBookmark mocks the AddBookmark method
func (m *MockAccountRepository) AddBookmark(ctx context.Context, username, objectID string) error {
	args := m.Called(ctx, username, objectID)
	return args.Error(0)
}

// RemoveBookmark mocks the RemoveBookmark method
func (m *MockAccountRepository) RemoveBookmark(ctx context.Context, username, objectID string) error {
	args := m.Called(ctx, username, objectID)
	return args.Error(0)
}

// GetBookmarks mocks the GetBookmarks method
func (m *MockAccountRepository) GetBookmarks(ctx context.Context, username string, limit int, cursor string) ([]*storage.Bookmark, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.Bookmark), args.String(1), args.Error(2)
}

// GetBookmarkedStatuses mocks the GetBookmarkedStatuses method
func (m *MockAccountRepository) GetBookmarkedStatuses(ctx context.Context, username string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, username, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

// Batch operations

// GetAccountsByUsernames mocks the GetAccountsByUsernames method
func (m *MockAccountRepository) GetAccountsByUsernames(ctx context.Context, usernames []string) ([]*storage.Account, error) {
	args := m.Called(ctx, usernames)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Account), args.Error(1)
}

// GetAccountsCount mocks the GetAccountsCount method
func (m *MockAccountRepository) GetAccountsCount(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

// Ensure MockAccountRepository implements interfaces.AccountRepository
var _ interfaces.AccountRepository = (*MockAccountRepository)(nil)
