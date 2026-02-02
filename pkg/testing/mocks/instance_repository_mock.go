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

// MockInstanceRepository is a mock implementation of interfaces.InstanceRepository
// using testify/mock for expectation-based testing.
type MockInstanceRepository struct {
	mock.Mock
}

// NewMockInstanceRepository creates a new mock instance repository
func NewMockInstanceRepository() *MockInstanceRepository {
	return &MockInstanceRepository{}
}

// GetInstanceState mocks the GetInstanceState method
func (m *MockInstanceRepository) GetInstanceState(ctx context.Context) (*models.InstanceState, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.InstanceState), args.Error(1)
}

// EnsureInstanceState mocks the EnsureInstanceState method
func (m *MockInstanceRepository) EnsureInstanceState(ctx context.Context) (*models.InstanceState, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.InstanceState), args.Error(1)
}

// SetInstanceLocked mocks the SetInstanceLocked method
func (m *MockInstanceRepository) SetInstanceLocked(ctx context.Context, locked bool) error {
	args := m.Called(ctx, locked)
	return args.Error(0)
}

// SetBootstrapWalletAddress mocks the SetBootstrapWalletAddress method
func (m *MockInstanceRepository) SetBootstrapWalletAddress(ctx context.Context, address string) error {
	args := m.Called(ctx, address)
	return args.Error(0)
}

// SetPrimaryAdminUsername mocks the SetPrimaryAdminUsername method
func (m *MockInstanceRepository) SetPrimaryAdminUsername(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

// GetInstanceRules mocks the GetInstanceRules method
func (m *MockInstanceRepository) GetInstanceRules(ctx context.Context) ([]storage.InstanceRule, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]storage.InstanceRule), args.Error(1)
}

// SetInstanceRules mocks the SetInstanceRules method
func (m *MockInstanceRepository) SetInstanceRules(ctx context.Context, rules []storage.InstanceRule) error {
	args := m.Called(ctx, rules)
	return args.Error(0)
}

// GetRulesByCategory mocks the GetRulesByCategory method
func (m *MockInstanceRepository) GetRulesByCategory(ctx context.Context, category string) ([]storage.InstanceRule, error) {
	args := m.Called(ctx, category)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]storage.InstanceRule), args.Error(1)
}

// GetExtendedDescription mocks the GetExtendedDescription method
func (m *MockInstanceRepository) GetExtendedDescription(ctx context.Context) (string, time.Time, error) {
	args := m.Called(ctx)
	return args.String(0), args.Get(1).(time.Time), args.Error(2)
}

// SetExtendedDescription mocks the SetExtendedDescription method
func (m *MockInstanceRepository) SetExtendedDescription(ctx context.Context, description string) error {
	args := m.Called(ctx, description)
	return args.Error(0)
}

// GetTotalUserCount mocks the GetTotalUserCount method
func (m *MockInstanceRepository) GetTotalUserCount(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

// GetTotalStatusCount mocks the GetTotalStatusCount method
func (m *MockInstanceRepository) GetTotalStatusCount(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

// GetTotalDomainCount mocks the GetTotalDomainCount method
func (m *MockInstanceRepository) GetTotalDomainCount(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

// GetActiveUserCount mocks the GetActiveUserCount method
func (m *MockInstanceRepository) GetActiveUserCount(ctx context.Context, days int) (int64, error) {
	args := m.Called(ctx, days)
	return args.Get(0).(int64), args.Error(1)
}

// GetDailyActiveUserCount mocks the GetDailyActiveUserCount method
func (m *MockInstanceRepository) GetDailyActiveUserCount(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

// GetLocalPostCount mocks the GetLocalPostCount method
func (m *MockInstanceRepository) GetLocalPostCount(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

// GetLocalCommentCount mocks the GetLocalCommentCount method
func (m *MockInstanceRepository) GetLocalCommentCount(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

// GetWeeklyActivity mocks the GetWeeklyActivity method
func (m *MockInstanceRepository) GetWeeklyActivity(ctx context.Context, weekTimestamp int64) (*storage.WeeklyActivity, error) {
	args := m.Called(ctx, weekTimestamp)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.WeeklyActivity), args.Error(1)
}

// RecordActivity mocks the RecordActivity method
func (m *MockInstanceRepository) RecordActivity(ctx context.Context, activityType string, userID string, timestamp time.Time) error {
	args := m.Called(ctx, activityType, userID, timestamp)
	return args.Error(0)
}

// GetContactAccount mocks the GetContactAccount method
func (m *MockInstanceRepository) GetContactAccount(ctx context.Context) (*storage.ActorRecord, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.ActorRecord), args.Error(1)
}

// GetStorageUsage mocks the GetStorageUsage method
func (m *MockInstanceRepository) GetStorageUsage(ctx context.Context) (any, error) {
	args := m.Called(ctx)
	return args.Get(0), args.Error(1)
}

// GetStorageHistory mocks the GetStorageHistory method
func (m *MockInstanceRepository) GetStorageHistory(ctx context.Context, days int) ([]any, error) {
	args := m.Called(ctx, days)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]any), args.Error(1)
}

// GetUserGrowthHistory mocks the GetUserGrowthHistory method
func (m *MockInstanceRepository) GetUserGrowthHistory(ctx context.Context, days int) ([]any, error) {
	args := m.Called(ctx, days)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]any), args.Error(1)
}

// GetDomainStats mocks the GetDomainStats method
func (m *MockInstanceRepository) GetDomainStats(ctx context.Context, domain string) (any, error) {
	args := m.Called(ctx, domain)
	return args.Get(0), args.Error(1)
}

// RecordDailyMetrics mocks the RecordDailyMetrics method
func (m *MockInstanceRepository) RecordDailyMetrics(ctx context.Context, date string, metrics map[string]interface{}) error {
	args := m.Called(ctx, date, metrics)
	return args.Error(0)
}

// GetMetricsSummary mocks the GetMetricsSummary method
func (m *MockInstanceRepository) GetMetricsSummary(ctx context.Context, timeRange string) (map[string]interface{}, error) {
	args := m.Called(ctx, timeRange)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]interface{}), args.Error(1)
}

// Ensure MockInstanceRepository implements interfaces.InstanceRepository
var _ interfaces.InstanceRepository = (*MockInstanceRepository)(nil)
