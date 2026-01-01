// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/stretchr/testify/mock"
)

// MockDomainBlockRepository is a mock implementation of interfaces.DomainBlockRepository
// using testify/mock for expectation-based testing.
type MockDomainBlockRepository struct {
	mock.Mock
}

// NewMockDomainBlockRepository creates a new mock domain block repository
func NewMockDomainBlockRepository() *MockDomainBlockRepository {
	return &MockDomainBlockRepository{}
}

// AddDomainBlock mocks the AddDomainBlock method
func (m *MockDomainBlockRepository) AddDomainBlock(ctx context.Context, username, domain string) error {
	args := m.Called(ctx, username, domain)
	return args.Error(0)
}

// RemoveDomainBlock mocks the RemoveDomainBlock method
func (m *MockDomainBlockRepository) RemoveDomainBlock(ctx context.Context, username, domain string) error {
	args := m.Called(ctx, username, domain)
	return args.Error(0)
}

// GetUserDomainBlocks mocks the GetUserDomainBlocks method
func (m *MockDomainBlockRepository) GetUserDomainBlocks(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]string), args.String(1), args.Error(2)
}

// IsBlockedDomain mocks the IsBlockedDomain method
func (m *MockDomainBlockRepository) IsBlockedDomain(ctx context.Context, username, domain string) (bool, error) {
	args := m.Called(ctx, username, domain)
	return args.Bool(0), args.Error(1)
}

// CreateInstanceDomainBlock mocks the CreateInstanceDomainBlock method
func (m *MockDomainBlockRepository) CreateInstanceDomainBlock(ctx context.Context, block *storage.InstanceDomainBlock) error {
	args := m.Called(ctx, block)
	return args.Error(0)
}

// GetInstanceDomainBlock mocks the GetInstanceDomainBlock method
func (m *MockDomainBlockRepository) GetInstanceDomainBlock(ctx context.Context, domain string) (*storage.InstanceDomainBlock, error) {
	args := m.Called(ctx, domain)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.InstanceDomainBlock), args.Error(1)
}


// GetInstanceDomainBlockByID mocks the GetInstanceDomainBlockByID method
func (m *MockDomainBlockRepository) GetInstanceDomainBlockByID(ctx context.Context, id string) (*storage.InstanceDomainBlock, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.InstanceDomainBlock), args.Error(1)
}

// ListInstanceDomainBlocks mocks the ListInstanceDomainBlocks method
func (m *MockDomainBlockRepository) ListInstanceDomainBlocks(ctx context.Context, limit int, cursor string) ([]*storage.InstanceDomainBlock, string, error) {
	args := m.Called(ctx, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.InstanceDomainBlock), args.String(1), args.Error(2)
}

// UpdateInstanceDomainBlock mocks the UpdateInstanceDomainBlock method
func (m *MockDomainBlockRepository) UpdateInstanceDomainBlock(ctx context.Context, domain string, updates map[string]any) error {
	args := m.Called(ctx, domain, updates)
	return args.Error(0)
}

// DeleteInstanceDomainBlock mocks the DeleteInstanceDomainBlock method
func (m *MockDomainBlockRepository) DeleteInstanceDomainBlock(ctx context.Context, domain string) error {
	args := m.Called(ctx, domain)
	return args.Error(0)
}

// IsInstanceDomainBlocked mocks the IsInstanceDomainBlocked method
func (m *MockDomainBlockRepository) IsInstanceDomainBlocked(ctx context.Context, domain string) (bool, *storage.InstanceDomainBlock, error) {
	args := m.Called(ctx, domain)
	if args.Get(1) == nil {
		return args.Bool(0), nil, args.Error(2)
	}
	return args.Bool(0), args.Get(1).(*storage.InstanceDomainBlock), args.Error(2)
}

// GetDomainBlocks mocks the GetDomainBlocks method
func (m *MockDomainBlockRepository) GetDomainBlocks(ctx context.Context, limit int, cursor string) ([]*storage.InstanceDomainBlock, string, error) {
	args := m.Called(ctx, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.InstanceDomainBlock), args.String(1), args.Error(2)
}

// GetDomainBlock mocks the GetDomainBlock method
func (m *MockDomainBlockRepository) GetDomainBlock(ctx context.Context, id string) (*storage.InstanceDomainBlock, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.InstanceDomainBlock), args.Error(1)
}

// CreateDomainBlock mocks the CreateDomainBlock method
func (m *MockDomainBlockRepository) CreateDomainBlock(ctx context.Context, block *storage.InstanceDomainBlock) error {
	args := m.Called(ctx, block)
	return args.Error(0)
}

// UpdateDomainBlock mocks the UpdateDomainBlock method
func (m *MockDomainBlockRepository) UpdateDomainBlock(ctx context.Context, id string, updates map[string]any) error {
	args := m.Called(ctx, id, updates)
	return args.Error(0)
}

// DeleteDomainBlock mocks the DeleteDomainBlock method
func (m *MockDomainBlockRepository) DeleteDomainBlock(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// IsDomainBlocked mocks the IsDomainBlocked method
func (m *MockDomainBlockRepository) IsDomainBlocked(ctx context.Context, domain string) (bool, *storage.InstanceDomainBlock, error) {
	args := m.Called(ctx, domain)
	if args.Get(1) == nil {
		return args.Bool(0), nil, args.Error(2)
	}
	return args.Bool(0), args.Get(1).(*storage.InstanceDomainBlock), args.Error(2)
}

// CreateEmailDomainBlock mocks the CreateEmailDomainBlock method
func (m *MockDomainBlockRepository) CreateEmailDomainBlock(ctx context.Context, block *storage.EmailDomainBlock) error {
	args := m.Called(ctx, block)
	return args.Error(0)
}

// GetEmailDomainBlocks mocks the GetEmailDomainBlocks method
func (m *MockDomainBlockRepository) GetEmailDomainBlocks(ctx context.Context, limit int, cursor string) ([]*storage.EmailDomainBlock, string, error) {
	args := m.Called(ctx, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.EmailDomainBlock), args.String(1), args.Error(2)
}

// DeleteEmailDomainBlock mocks the DeleteEmailDomainBlock method
func (m *MockDomainBlockRepository) DeleteEmailDomainBlock(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// GetDomainAllows mocks the GetDomainAllows method
func (m *MockDomainBlockRepository) GetDomainAllows(ctx context.Context, limit int, cursor string) ([]*storage.DomainAllow, string, error) {
	args := m.Called(ctx, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.DomainAllow), args.String(1), args.Error(2)
}

// CreateDomainAllow mocks the CreateDomainAllow method
func (m *MockDomainBlockRepository) CreateDomainAllow(ctx context.Context, allow *storage.DomainAllow) error {
	args := m.Called(ctx, allow)
	return args.Error(0)
}

// DeleteDomainAllow mocks the DeleteDomainAllow method
func (m *MockDomainBlockRepository) DeleteDomainAllow(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// Ensure MockDomainBlockRepository implements interfaces.DomainBlockRepository
var _ interfaces.DomainBlockRepository = (*MockDomainBlockRepository)(nil)
