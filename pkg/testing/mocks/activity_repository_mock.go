// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/stretchr/testify/mock"
)

// MockActivityRepository is a mock implementation of interfaces.ActivityRepository
// using testify/mock for expectation-based testing.
type MockActivityRepository struct {
	mock.Mock
}

// NewMockActivityRepository creates a new mock activity repository
func NewMockActivityRepository() *MockActivityRepository {
	return &MockActivityRepository{}
}

// ===== Core Activity Operations =====

// CreateActivity mocks the CreateActivity method
func (m *MockActivityRepository) CreateActivity(ctx context.Context, activity *activitypub.Activity) error {
	args := m.Called(ctx, activity)
	return args.Error(0)
}

// GetActivity mocks the GetActivity method
func (m *MockActivityRepository) GetActivity(ctx context.Context, id string) (*activitypub.Activity, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activitypub.Activity), args.Error(1)
}

// ===== Inbox/Outbox Operations =====

// GetInboxActivities mocks the GetInboxActivities method
func (m *MockActivityRepository) GetInboxActivities(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Activity, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*activitypub.Activity), args.String(1), args.Error(2)
}

// GetOutboxActivities mocks the GetOutboxActivities method
func (m *MockActivityRepository) GetOutboxActivities(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Activity, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*activitypub.Activity), args.String(1), args.Error(2)
}

// ===== Collection Operations =====

// GetCollection mocks the GetCollection method
func (m *MockActivityRepository) GetCollection(ctx context.Context, username, collectionType string, limit int, cursor string) (*activitypub.OrderedCollectionPage, error) {
	args := m.Called(ctx, username, collectionType, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activitypub.OrderedCollectionPage), args.Error(1)
}

// ===== Analytics and Metrics Operations =====

// GetWeeklyActivity mocks the GetWeeklyActivity method
func (m *MockActivityRepository) GetWeeklyActivity(ctx context.Context, weekTimestamp int64) (*storage.WeeklyActivity, error) {
	args := m.Called(ctx, weekTimestamp)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.WeeklyActivity), args.Error(1)
}

// RecordActivity mocks the RecordActivity method
func (m *MockActivityRepository) RecordActivity(ctx context.Context, activityType string, actorID string, timestamp time.Time) error {
	args := m.Called(ctx, activityType, actorID, timestamp)
	return args.Error(0)
}

// GetHashtagActivity mocks the GetHashtagActivity method
func (m *MockActivityRepository) GetHashtagActivity(ctx context.Context, hashtag string, since time.Time) ([]*storage.Activity, error) {
	args := m.Called(ctx, hashtag, since)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Activity), args.Error(1)
}

// ===== Federation Operations =====

// RecordFederationActivity mocks the RecordFederationActivity method
func (m *MockActivityRepository) RecordFederationActivity(ctx context.Context, activity *storage.FederationActivity) error {
	args := m.Called(ctx, activity)
	return args.Error(0)
}

// Ensure MockActivityRepository implements interfaces.ActivityRepository
var _ interfaces.ActivityRepository = (*MockActivityRepository)(nil)
