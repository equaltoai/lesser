// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
)

// MockListRepository is a mock implementation of interfaces.ListRepository
// using testify/mock for expectation-based testing.
type MockListRepository struct {
	mock.Mock
}

// NewMockListRepository creates a new mock list repository
func NewMockListRepository() *MockListRepository {
	return &MockListRepository{}
}

// CreateList mocks the CreateList method
func (m *MockListRepository) CreateList(ctx context.Context, list *models.List) error {
	args := m.Called(ctx, list)
	return args.Error(0)
}

// GetList mocks the GetList method
func (m *MockListRepository) GetList(ctx context.Context, listID string) (*models.List, error) {
	args := m.Called(ctx, listID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.List), args.Error(1)
}

// UpdateList mocks the UpdateList method
func (m *MockListRepository) UpdateList(ctx context.Context, list *models.List) error {
	args := m.Called(ctx, list)
	return args.Error(0)
}

// DeleteList mocks the DeleteList method
func (m *MockListRepository) DeleteList(ctx context.Context, listID string) error {
	args := m.Called(ctx, listID)
	return args.Error(0)
}

// GetUserLists mocks the GetUserLists method
func (m *MockListRepository) GetUserLists(ctx context.Context, username string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.List], error) {
	args := m.Called(ctx, username, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.List]), args.Error(1)
}

// GetListsByMember mocks the GetListsByMember method
func (m *MockListRepository) GetListsByMember(ctx context.Context, memberUsername string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.List], error) {
	args := m.Called(ctx, memberUsername, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.List]), args.Error(1)
}

// GetListsForUser mocks the GetListsForUser method
func (m *MockListRepository) GetListsForUser(ctx context.Context, username string) ([]*storage.List, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.List), args.Error(1)
}

// GetListsForUserPaginated mocks the GetListsForUserPaginated method
func (m *MockListRepository) GetListsForUserPaginated(ctx context.Context, username string, limit int, cursor string) ([]*storage.List, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.List), args.String(1), args.Error(2)
}

// CountUserLists mocks the CountUserLists method
func (m *MockListRepository) CountUserLists(ctx context.Context, username string) (int, error) {
	args := m.Called(ctx, username)
	return args.Int(0), args.Error(1)
}

// AddListMember mocks the AddListMember method
func (m *MockListRepository) AddListMember(ctx context.Context, listID, memberUsername string) error {
	args := m.Called(ctx, listID, memberUsername)
	return args.Error(0)
}

// RemoveListMember mocks the RemoveListMember method
func (m *MockListRepository) RemoveListMember(ctx context.Context, listID, memberUsername string) error {
	args := m.Called(ctx, listID, memberUsername)
	return args.Error(0)
}

// GetListMembers mocks the GetListMembers method
func (m *MockListRepository) GetListMembers(ctx context.Context, listID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	args := m.Called(ctx, listID, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*storage.Account]), args.Error(1)
}

// IsListMember mocks the IsListMember method
func (m *MockListRepository) IsListMember(ctx context.Context, listID, memberUsername string) (bool, error) {
	args := m.Called(ctx, listID, memberUsername)
	return args.Bool(0), args.Error(1)
}

// CountListMembers mocks the CountListMembers method
func (m *MockListRepository) CountListMembers(ctx context.Context, listID string) (int, error) {
	args := m.Called(ctx, listID)
	return args.Int(0), args.Error(1)
}

// GetAccountLists mocks the GetAccountLists method
func (m *MockListRepository) GetAccountLists(ctx context.Context, accountID string) ([]*storage.List, error) {
	args := m.Called(ctx, accountID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.List), args.Error(1)
}

// GetAccountListsPaginated mocks the GetAccountListsPaginated method
func (m *MockListRepository) GetAccountListsPaginated(ctx context.Context, accountID string, limit int, cursor string) ([]*storage.List, string, error) {
	args := m.Called(ctx, accountID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.List), args.String(1), args.Error(2)
}

// GetAccountListsForUser mocks the GetAccountListsForUser method
func (m *MockListRepository) GetAccountListsForUser(ctx context.Context, accountID, username string) ([]*storage.List, error) {
	args := m.Called(ctx, accountID, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.List), args.Error(1)
}

// RemoveAccountFromAllLists mocks the RemoveAccountFromAllLists method
func (m *MockListRepository) RemoveAccountFromAllLists(ctx context.Context, accountID string) error {
	args := m.Called(ctx, accountID)
	return args.Error(0)
}

// GetExclusiveLists mocks the GetExclusiveLists method
func (m *MockListRepository) GetExclusiveLists(ctx context.Context, username string) ([]*storage.List, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.List), args.Error(1)
}

// AddAccountsToList mocks the AddAccountsToList method
func (m *MockListRepository) AddAccountsToList(ctx context.Context, listID string, accountIDs []string) error {
	args := m.Called(ctx, listID, accountIDs)
	return args.Error(0)
}

// RemoveAccountsFromList mocks the RemoveAccountsFromList method
func (m *MockListRepository) RemoveAccountsFromList(ctx context.Context, listID string, accountIDs []string) error {
	args := m.Called(ctx, listID, accountIDs)
	return args.Error(0)
}

// GetListAccounts mocks the GetListAccounts method
func (m *MockListRepository) GetListAccounts(ctx context.Context, listID string) ([]string, error) {
	args := m.Called(ctx, listID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

// GetListsContainingAccount mocks the GetListsContainingAccount method
func (m *MockListRepository) GetListsContainingAccount(ctx context.Context, accountID, username string) ([]*storage.List, error) {
	args := m.Called(ctx, accountID, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.List), args.Error(1)
}

// GetListTimeline mocks the GetListTimeline method
func (m *MockListRepository) GetListTimeline(ctx context.Context, listID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, listID, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

// GetListStatuses mocks the GetListStatuses method
func (m *MockListRepository) GetListStatuses(ctx context.Context, listID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, listID, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

// Ensure MockListRepository implements interfaces.ListRepository
var _ interfaces.ListRepository = (*MockListRepository)(nil)
