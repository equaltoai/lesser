// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/stretchr/testify/mock"
)

// MockDNSCacheRepository is a mock implementation of interfaces.DNSCacheRepository
// using testify/mock for expectation-based testing.
type MockDNSCacheRepository struct {
	mock.Mock
}

// NewMockDNSCacheRepository creates a new mock DNS cache repository
func NewMockDNSCacheRepository() *MockDNSCacheRepository {
	return &MockDNSCacheRepository{}
}

// GetDNSCache mocks the GetDNSCache method
func (m *MockDNSCacheRepository) GetDNSCache(ctx context.Context, hostname string) (*storage.DNSCacheEntry, error) {
	args := m.Called(ctx, hostname)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.DNSCacheEntry), args.Error(1)
}

// SetDNSCache mocks the SetDNSCache method
func (m *MockDNSCacheRepository) SetDNSCache(ctx context.Context, entry *storage.DNSCacheEntry) error {
	args := m.Called(ctx, entry)
	return args.Error(0)
}

// InvalidateDNSCache mocks the InvalidateDNSCache method
func (m *MockDNSCacheRepository) InvalidateDNSCache(ctx context.Context, hostname string) error {
	args := m.Called(ctx, hostname)
	return args.Error(0)
}

// Ensure MockDNSCacheRepository implements interfaces.DNSCacheRepository
var _ interfaces.DNSCacheRepository = (*MockDNSCacheRepository)(nil)
