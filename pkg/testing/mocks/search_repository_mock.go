// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/stretchr/testify/mock"
)

// MockSearchRepository is a mock implementation of interfaces.SearchRepository
// using testify/mock for expectation-based testing.
type MockSearchRepository struct {
	mock.Mock
}

// NewMockSearchRepository creates a new mock search repository
func NewMockSearchRepository() *MockSearchRepository {
	return &MockSearchRepository{}
}

// ===== Account Search Operations =====

// SearchAccounts mocks the SearchAccounts method
func (m *MockSearchRepository) SearchAccounts(ctx context.Context, query string, limit int, followingOnly bool, offset int) ([]*activitypub.Actor, error) {
	args := m.Called(ctx, query, limit, followingOnly, offset)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*activitypub.Actor), args.Error(1)
}

// SearchAccountsWithPrivacy mocks the SearchAccountsWithPrivacy method
func (m *MockSearchRepository) SearchAccountsWithPrivacy(ctx context.Context, query string, limit int, followingOnly bool, offset int, searcherActorID string) ([]*activitypub.Actor, error) {
	args := m.Called(ctx, query, limit, followingOnly, offset, searcherActorID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*activitypub.Actor), args.Error(1)
}

// SearchAccountsAdvanced mocks the SearchAccountsAdvanced method
func (m *MockSearchRepository) SearchAccountsAdvanced(ctx context.Context, query string, resolve bool, limit int, offset int, following bool, accountID string) ([]*activitypub.Actor, error) {
	args := m.Called(ctx, query, resolve, limit, offset, following, accountID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*activitypub.Actor), args.Error(1)
}

// ===== Status Search Operations =====

// SearchStatuses mocks the SearchStatuses method
func (m *MockSearchRepository) SearchStatuses(ctx context.Context, query string, limit int) ([]*storage.StatusSearchResult, error) {
	args := m.Called(ctx, query, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusSearchResult), args.Error(1)
}


// SearchStatusesWithOptions mocks the SearchStatusesWithOptions method
func (m *MockSearchRepository) SearchStatusesWithOptions(ctx context.Context, query string, options storage.StatusSearchOptions) ([]*storage.StatusSearchResult, error) {
	args := m.Called(ctx, query, options)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusSearchResult), args.Error(1)
}

// SearchStatusesWithOptionsPaginated mocks the SearchStatusesWithOptionsPaginated method
func (m *MockSearchRepository) SearchStatusesWithOptionsPaginated(ctx context.Context, query string, options storage.StatusSearchOptions) ([]*storage.StatusSearchResult, *interfaces.PaginationResult, error) {
	args := m.Called(ctx, query, options)
	if args.Get(0) == nil {
		return nil, nil, args.Error(2)
	}
	var pagination *interfaces.PaginationResult
	if args.Get(1) != nil {
		pagination = args.Get(1).(*interfaces.PaginationResult)
	}
	return args.Get(0).([]*storage.StatusSearchResult), pagination, args.Error(2)
}

// SearchStatusesWithPrivacy mocks the SearchStatusesWithPrivacy method
func (m *MockSearchRepository) SearchStatusesWithPrivacy(ctx context.Context, query string, options storage.StatusSearchOptions, searcherActorID string) ([]*storage.StatusSearchResult, error) {
	args := m.Called(ctx, query, options, searcherActorID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusSearchResult), args.Error(1)
}

// SearchStatusesWithPrivacyPaginated mocks the SearchStatusesWithPrivacyPaginated method
func (m *MockSearchRepository) SearchStatusesWithPrivacyPaginated(ctx context.Context, query string, options storage.StatusSearchOptions, searcherActorID string) ([]*storage.StatusSearchResult, *interfaces.PaginationResult, error) {
	args := m.Called(ctx, query, options, searcherActorID)
	if args.Get(0) == nil {
		return nil, nil, args.Error(2)
	}
	var pagination *interfaces.PaginationResult
	if args.Get(1) != nil {
		pagination = args.Get(1).(*interfaces.PaginationResult)
	}
	return args.Get(0).([]*storage.StatusSearchResult), pagination, args.Error(2)
}

// SearchStatusesAdvanced mocks the SearchStatusesAdvanced method
func (m *MockSearchRepository) SearchStatusesAdvanced(ctx context.Context, query string, limit int, maxID, minID *string, accountID string) ([]*storage.StatusSearchResult, error) {
	args := m.Called(ctx, query, limit, maxID, minID, accountID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusSearchResult), args.Error(1)
}

// ===== Dependency Management =====

// SetDependencies mocks the SetDependencies method
func (m *MockSearchRepository) SetDependencies(deps interfaces.SearchRepositoryDeps) {
	m.Called(deps)
}

// Ensure MockSearchRepository implements interfaces.SearchRepository
var _ interfaces.SearchRepository = (*MockSearchRepository)(nil)
