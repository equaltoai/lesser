// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/stretchr/testify/mock"
)

// MockRateLimitRepository is a mock implementation of interfaces.RateLimitRepository
// using testify/mock for expectation-based testing.
type MockRateLimitRepository struct {
	mock.Mock
}

// NewMockRateLimitRepository creates a new mock rate limit repository
func NewMockRateLimitRepository() *MockRateLimitRepository {
	return &MockRateLimitRepository{}
}

// ===== Login Attempt Operations =====

// RecordLoginAttempt mocks the RecordLoginAttempt method
func (m *MockRateLimitRepository) RecordLoginAttempt(ctx context.Context, identifier string, success bool) error {
	args := m.Called(ctx, identifier, success)
	return args.Error(0)
}

// GetLoginAttemptCount mocks the GetLoginAttemptCount method
func (m *MockRateLimitRepository) GetLoginAttemptCount(ctx context.Context, identifier string, since time.Time) (int, error) {
	args := m.Called(ctx, identifier, since)
	return args.Int(0), args.Error(1)
}

// IsRateLimited mocks the IsRateLimited method
func (m *MockRateLimitRepository) IsRateLimited(ctx context.Context, identifier string) (bool, time.Time, error) {
	args := m.Called(ctx, identifier)
	return args.Bool(0), args.Get(1).(time.Time), args.Error(2)
}

// ClearLoginAttempts mocks the ClearLoginAttempts method
func (m *MockRateLimitRepository) ClearLoginAttempts(ctx context.Context, identifier string) error {
	args := m.Called(ctx, identifier)
	return args.Error(0)
}

// ===== API Rate Limiting Operations =====

// CheckAPIRateLimit mocks the CheckAPIRateLimit method
func (m *MockRateLimitRepository) CheckAPIRateLimit(ctx context.Context, userID, endpoint string, limit int, window time.Duration) error {
	args := m.Called(ctx, userID, endpoint, limit, window)
	return args.Error(0)
}

// GetAPIRateLimitInfo mocks the GetAPIRateLimitInfo method
func (m *MockRateLimitRepository) GetAPIRateLimitInfo(ctx context.Context, userID, endpoint string, limit int, window time.Duration) (remaining int, resetTime time.Time, err error) {
	args := m.Called(ctx, userID, endpoint, limit, window)
	return args.Int(0), args.Get(1).(time.Time), args.Error(2)
}

// CheckFixedWindowRateLimit mocks the optional fixed-window atomic rate-limit helper.
func (m *MockRateLimitRepository) CheckFixedWindowRateLimit(ctx context.Context, identifier, bucket string, limit int, window time.Duration) (allowed bool, remaining int, resetTime time.Time, err error) {
	args := m.Called(ctx, identifier, bucket, limit, window)
	return args.Bool(0), args.Int(1), args.Get(2).(time.Time), args.Error(3)
}

// ===== Federation Rate Limiting Operations =====

// CheckFederationRateLimit mocks the CheckFederationRateLimit method
func (m *MockRateLimitRepository) CheckFederationRateLimit(ctx context.Context, domain, endpoint string, limit int, window time.Duration) error {
	args := m.Called(ctx, domain, endpoint, limit, window)
	return args.Error(0)
}

// GetFederationRateLimitInfo mocks the GetFederationRateLimitInfo method
func (m *MockRateLimitRepository) GetFederationRateLimitInfo(ctx context.Context, domain, endpoint string, limit int, window time.Duration) (remaining int, resetTime time.Time, err error) {
	args := m.Called(ctx, domain, endpoint, limit, window)
	return args.Int(0), args.Get(1).(time.Time), args.Error(2)
}

// ===== Violation Tracking Operations =====

// GetViolationCount mocks the GetViolationCount method
func (m *MockRateLimitRepository) GetViolationCount(ctx context.Context, userID, domain string, since time.Duration) (int, error) {
	args := m.Called(ctx, userID, domain, since)
	return args.Int(0), args.Error(1)
}

// ===== Block Status Operations =====

// IsUserBlocked mocks the IsUserBlocked method
func (m *MockRateLimitRepository) IsUserBlocked(ctx context.Context, userID string) (bool, time.Time, error) {
	args := m.Called(ctx, userID)
	return args.Bool(0), args.Get(1).(time.Time), args.Error(2)
}

// IsDomainBlocked mocks the IsDomainBlocked method
func (m *MockRateLimitRepository) IsDomainBlocked(ctx context.Context, domain string) (bool, time.Time, error) {
	args := m.Called(ctx, domain)
	return args.Bool(0), args.Get(1).(time.Time), args.Error(2)
}

// ===== Community Note Rate Limiting =====

// CheckCommunityNoteRateLimit mocks the CheckCommunityNoteRateLimit method
func (m *MockRateLimitRepository) CheckCommunityNoteRateLimit(ctx context.Context, userID string, limit int) (bool, int, error) {
	args := m.Called(ctx, userID, limit)
	return args.Bool(0), args.Int(1), args.Error(2)
}

// Ensure MockRateLimitRepository implements interfaces.RateLimitRepository
var _ interfaces.RateLimitRepository = (*MockRateLimitRepository)(nil)
