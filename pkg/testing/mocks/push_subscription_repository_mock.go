// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/stretchr/testify/mock"
)

// MockPushSubscriptionRepository is a mock implementation of interfaces.PushSubscriptionRepository
// using testify/mock for expectation-based testing.
type MockPushSubscriptionRepository struct {
	mock.Mock
}

// NewMockPushSubscriptionRepository creates a new mock push subscription repository
func NewMockPushSubscriptionRepository() *MockPushSubscriptionRepository {
	return &MockPushSubscriptionRepository{}
}

// CreatePushSubscription mocks the CreatePushSubscription method
func (m *MockPushSubscriptionRepository) CreatePushSubscription(ctx context.Context, username string, subscription *storage.PushSubscription) error {
	args := m.Called(ctx, username, subscription)
	return args.Error(0)
}

// GetPushSubscription mocks the GetPushSubscription method
func (m *MockPushSubscriptionRepository) GetPushSubscription(ctx context.Context, username, subscriptionID string) (*storage.PushSubscription, error) {
	args := m.Called(ctx, username, subscriptionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.PushSubscription), args.Error(1)
}

// GetUserPushSubscriptions mocks the GetUserPushSubscriptions method
func (m *MockPushSubscriptionRepository) GetUserPushSubscriptions(ctx context.Context, username string) ([]*storage.PushSubscription, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.PushSubscription), args.Error(1)
}

// UpdatePushSubscription mocks the UpdatePushSubscription method
func (m *MockPushSubscriptionRepository) UpdatePushSubscription(ctx context.Context, username, subscriptionID string, alerts storage.PushSubscriptionAlerts) error {
	args := m.Called(ctx, username, subscriptionID, alerts)
	return args.Error(0)
}

// DeletePushSubscription mocks the DeletePushSubscription method
func (m *MockPushSubscriptionRepository) DeletePushSubscription(ctx context.Context, username, subscriptionID string) error {
	args := m.Called(ctx, username, subscriptionID)
	return args.Error(0)
}

// DeleteAllPushSubscriptions mocks the DeleteAllPushSubscriptions method
func (m *MockPushSubscriptionRepository) DeleteAllPushSubscriptions(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

// GetVAPIDKeys mocks the GetVAPIDKeys method
func (m *MockPushSubscriptionRepository) GetVAPIDKeys(ctx context.Context) (*storage.VAPIDKeys, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.VAPIDKeys), args.Error(1)
}

// SetVAPIDKeys mocks the SetVAPIDKeys method
func (m *MockPushSubscriptionRepository) SetVAPIDKeys(ctx context.Context, keys *storage.VAPIDKeys) error {
	args := m.Called(ctx, keys)
	return args.Error(0)
}

// Ensure MockPushSubscriptionRepository implements interfaces.PushSubscriptionRepository
var _ interfaces.PushSubscriptionRepository = (*MockPushSubscriptionRepository)(nil)
