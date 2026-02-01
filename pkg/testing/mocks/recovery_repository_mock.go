// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/stretchr/testify/mock"
)

// MockRecoveryRepository is a mock implementation of interfaces.RecoveryRepository
// using testify/mock for expectation-based testing.
type MockRecoveryRepository struct {
	mock.Mock
}

// NewMockRecoveryRepository creates a new mock recovery repository
func NewMockRecoveryRepository() *MockRecoveryRepository {
	return &MockRecoveryRepository{}
}

// StoreTrustee mocks the StoreTrustee method
func (m *MockRecoveryRepository) StoreTrustee(ctx context.Context, username string, trustee *storage.TrusteeConfig) error {
	args := m.Called(ctx, username, trustee)
	return args.Error(0)
}

// GetTrustees mocks the GetTrustees method
func (m *MockRecoveryRepository) GetTrustees(ctx context.Context, username string) ([]*storage.TrusteeConfig, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.TrusteeConfig), args.Error(1)
}

// DeleteTrustee mocks the DeleteTrustee method
func (m *MockRecoveryRepository) DeleteTrustee(ctx context.Context, username, trusteeActorID string) error {
	args := m.Called(ctx, username, trusteeActorID)
	return args.Error(0)
}

// UpdateTrusteeConfirmed mocks the UpdateTrusteeConfirmed method
func (m *MockRecoveryRepository) UpdateTrusteeConfirmed(ctx context.Context, username, trusteeActorID string, confirmed bool) error {
	args := m.Called(ctx, username, trusteeActorID, confirmed)
	return args.Error(0)
}

// StoreRecoveryRequest mocks the StoreRecoveryRequest method
func (m *MockRecoveryRepository) StoreRecoveryRequest(ctx context.Context, request *storage.SocialRecoveryRequest) error {
	args := m.Called(ctx, request)
	return args.Error(0)
}

// GetRecoveryRequest mocks the GetRecoveryRequest method
func (m *MockRecoveryRepository) GetRecoveryRequest(ctx context.Context, requestID string) (*storage.SocialRecoveryRequest, error) {
	args := m.Called(ctx, requestID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.SocialRecoveryRequest), args.Error(1)
}

// UpdateRecoveryRequest mocks the UpdateRecoveryRequest method
func (m *MockRecoveryRepository) UpdateRecoveryRequest(ctx context.Context, request *storage.SocialRecoveryRequest) error {
	args := m.Called(ctx, request)
	return args.Error(0)
}

// DeleteRecoveryRequest mocks the DeleteRecoveryRequest method
func (m *MockRecoveryRepository) DeleteRecoveryRequest(ctx context.Context, requestID string) error {
	args := m.Called(ctx, requestID)
	return args.Error(0)
}

// GetActiveRecoveryRequests mocks the GetActiveRecoveryRequests method
func (m *MockRecoveryRepository) GetActiveRecoveryRequests(ctx context.Context, username string) ([]*storage.SocialRecoveryRequest, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.SocialRecoveryRequest), args.Error(1)
}

// StoreRecoveryCode mocks the StoreRecoveryCode method
func (m *MockRecoveryRepository) StoreRecoveryCode(ctx context.Context, username string, code *storage.RecoveryCodeItem) error {
	args := m.Called(ctx, username, code)
	return args.Error(0)
}

// GetRecoveryCodes mocks the GetRecoveryCodes method
func (m *MockRecoveryRepository) GetRecoveryCodes(ctx context.Context, username string) ([]*storage.RecoveryCodeItem, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.RecoveryCodeItem), args.Error(1)
}

// MarkRecoveryCodeUsed mocks the MarkRecoveryCodeUsed method
func (m *MockRecoveryRepository) MarkRecoveryCodeUsed(ctx context.Context, username, codeHash string) error {
	args := m.Called(ctx, username, codeHash)
	return args.Error(0)
}

// DeleteAllRecoveryCodes mocks the DeleteAllRecoveryCodes method
func (m *MockRecoveryRepository) DeleteAllRecoveryCodes(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

// CountUnusedRecoveryCodes mocks the CountUnusedRecoveryCodes method
func (m *MockRecoveryRepository) CountUnusedRecoveryCodes(ctx context.Context, username string) (int, error) {
	args := m.Called(ctx, username)
	return args.Int(0), args.Error(1)
}

// StoreRecoveryToken mocks the StoreRecoveryToken method
func (m *MockRecoveryRepository) StoreRecoveryToken(ctx context.Context, key string, data map[string]any) error {
	args := m.Called(ctx, key, data)
	return args.Error(0)
}

// GetRecoveryToken mocks the GetRecoveryToken method
func (m *MockRecoveryRepository) GetRecoveryToken(ctx context.Context, key string) (map[string]any, error) {
	args := m.Called(ctx, key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]any), args.Error(1)
}

// DeleteRecoveryToken mocks the DeleteRecoveryToken method
func (m *MockRecoveryRepository) DeleteRecoveryToken(ctx context.Context, key string) error {
	args := m.Called(ctx, key)
	return args.Error(0)
}

// Ensure MockRecoveryRepository implements interfaces.RecoveryRepository
var _ interfaces.RecoveryRepository = (*MockRecoveryRepository)(nil)
