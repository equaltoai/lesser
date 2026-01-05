// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/stretchr/testify/mock"
)

// MockRelayRepository is a mock implementation of interfaces.RelayRepository
// using testify/mock for expectation-based testing.
type MockRelayRepository struct {
	mock.Mock
}

// NewMockRelayRepository creates a new mock relay repository
func NewMockRelayRepository() *MockRelayRepository {
	return &MockRelayRepository{}
}

// ===== Core Relay Operations =====

// StoreRelayInfo mocks the StoreRelayInfo method
func (m *MockRelayRepository) StoreRelayInfo(ctx context.Context, relay *storage.RelayInfo) error {
	args := m.Called(ctx, relay)
	return args.Error(0)
}

// GetRelayInfo mocks the GetRelayInfo method
func (m *MockRelayRepository) GetRelayInfo(ctx context.Context, relayURL string) (*storage.RelayInfo, error) {
	args := m.Called(ctx, relayURL)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.RelayInfo), args.Error(1)
}

// RemoveRelayInfo mocks the RemoveRelayInfo method
func (m *MockRelayRepository) RemoveRelayInfo(ctx context.Context, relayURL string) error {
	args := m.Called(ctx, relayURL)
	return args.Error(0)
}

// ===== Relay Listing Operations =====

// GetActiveRelays mocks the GetActiveRelays method
func (m *MockRelayRepository) GetActiveRelays(ctx context.Context) ([]*storage.RelayInfo, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.RelayInfo), args.Error(1)
}

// GetAllRelays mocks the GetAllRelays method
func (m *MockRelayRepository) GetAllRelays(ctx context.Context, limit int, cursor string) ([]*storage.RelayInfo, string, error) {
	args := m.Called(ctx, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*storage.RelayInfo), args.String(1), args.Error(2)
}

// ListRelays mocks the ListRelays method
func (m *MockRelayRepository) ListRelays(ctx context.Context) ([]*storage.RelayInfo, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.RelayInfo), args.Error(1)
}

// ===== Relay Status Operations =====

// UpdateRelayStatus mocks the UpdateRelayStatus method
func (m *MockRelayRepository) UpdateRelayStatus(ctx context.Context, relayURL string, active bool) error {
	args := m.Called(ctx, relayURL, active)
	return args.Error(0)
}

// UpdateRelayState mocks the UpdateRelayState method
func (m *MockRelayRepository) UpdateRelayState(ctx context.Context, relayURL string, state storage.RelayState) error {
	args := m.Called(ctx, relayURL, state)
	return args.Error(0)
}

// ===== CRUD Aliases =====

// CreateRelay mocks the CreateRelay method
func (m *MockRelayRepository) CreateRelay(ctx context.Context, relay *storage.RelayInfo) error {
	args := m.Called(ctx, relay)
	return args.Error(0)
}

// GetRelay mocks the GetRelay method
func (m *MockRelayRepository) GetRelay(ctx context.Context, relayURL string) (*storage.RelayInfo, error) {
	args := m.Called(ctx, relayURL)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.RelayInfo), args.Error(1)
}

// DeleteRelay mocks the DeleteRelay method
func (m *MockRelayRepository) DeleteRelay(ctx context.Context, relayURL string) error {
	args := m.Called(ctx, relayURL)
	return args.Error(0)
}

// Ensure MockRelayRepository implements interfaces.RelayRepository
var _ interfaces.RelayRepository = (*MockRelayRepository)(nil)
