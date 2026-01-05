// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
)

// MockTimelineRepositoryInterface is a mock implementation of interfaces.TimelineRepository
// using testify/mock for expectation-based testing.
// Note: This is separate from MockTimelineRepository which is an older mock that doesn't
// implement the full interface.
type MockTimelineRepositoryInterface struct {
	mock.Mock
}

// NewMockTimelineRepositoryInterface creates a new mock timeline repository
func NewMockTimelineRepositoryInterface() *MockTimelineRepositoryInterface {
	return &MockTimelineRepositoryInterface{}
}

// Core timeline entry operations

// CreateTimelineEntry mocks the CreateTimelineEntry method
func (m *MockTimelineRepositoryInterface) CreateTimelineEntry(ctx context.Context, entry *models.Timeline) error {
	args := m.Called(ctx, entry)
	return args.Error(0)
}

// CreateTimelineEntries mocks the CreateTimelineEntries method
func (m *MockTimelineRepositoryInterface) CreateTimelineEntries(ctx context.Context, entries []*models.Timeline) error {
	args := m.Called(ctx, entries)
	return args.Error(0)
}

// GetTimelineEntry mocks the GetTimelineEntry method
func (m *MockTimelineRepositoryInterface) GetTimelineEntry(ctx context.Context, timelineType, timelineID, entryID string, timelineAt time.Time) (*models.Timeline, error) {
	args := m.Called(ctx, timelineType, timelineID, entryID, timelineAt)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Timeline), args.Error(1)
}

// UpdateTimelineEntry mocks the UpdateTimelineEntry method
func (m *MockTimelineRepositoryInterface) UpdateTimelineEntry(ctx context.Context, entry *models.Timeline) error {
	args := m.Called(ctx, entry)
	return args.Error(0)
}

// DeleteTimelineEntry mocks the DeleteTimelineEntry method
func (m *MockTimelineRepositoryInterface) DeleteTimelineEntry(ctx context.Context, timelineType, timelineID, entryID string, timelineAt time.Time) error {
	args := m.Called(ctx, timelineType, timelineID, entryID, timelineAt)
	return args.Error(0)
}


// Timeline retrieval by type

// GetHomeTimeline mocks the GetHomeTimeline method
func (m *MockTimelineRepositoryInterface) GetHomeTimeline(ctx context.Context, username string, limit int, cursor string) ([]*models.Timeline, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Timeline), args.String(1), args.Error(2)
}

// GetPublicTimeline mocks the GetPublicTimeline method
func (m *MockTimelineRepositoryInterface) GetPublicTimeline(ctx context.Context, local bool, limit int, cursor string) ([]*models.Timeline, string, error) {
	args := m.Called(ctx, local, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Timeline), args.String(1), args.Error(2)
}

// GetListTimeline mocks the GetListTimeline method
func (m *MockTimelineRepositoryInterface) GetListTimeline(ctx context.Context, listID string, limit int, cursor string) ([]*models.Timeline, string, error) {
	args := m.Called(ctx, listID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Timeline), args.String(1), args.Error(2)
}

// GetDirectTimeline mocks the GetDirectTimeline method
func (m *MockTimelineRepositoryInterface) GetDirectTimeline(ctx context.Context, username string, limit int, cursor string) ([]*models.Timeline, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Timeline), args.String(1), args.Error(2)
}

// GetHashtagTimeline mocks the GetHashtagTimeline method
func (m *MockTimelineRepositoryInterface) GetHashtagTimeline(ctx context.Context, hashtag string, local bool, limit int, cursor string) ([]*models.Timeline, string, error) {
	args := m.Called(ctx, hashtag, local, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Timeline), args.String(1), args.Error(2)
}

// Timeline retrieval by index

// GetTimelineEntriesByPost mocks the GetTimelineEntriesByPost method
func (m *MockTimelineRepositoryInterface) GetTimelineEntriesByPost(ctx context.Context, postID string, limit int, cursor string) ([]*models.Timeline, string, error) {
	args := m.Called(ctx, postID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Timeline), args.String(1), args.Error(2)
}

// GetTimelineEntriesByActor mocks the GetTimelineEntriesByActor method
func (m *MockTimelineRepositoryInterface) GetTimelineEntriesByActor(ctx context.Context, actorID string, limit int, cursor string) ([]*models.Timeline, string, error) {
	args := m.Called(ctx, actorID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Timeline), args.String(1), args.Error(2)
}

// GetTimelineEntriesByVisibility mocks the GetTimelineEntriesByVisibility method
func (m *MockTimelineRepositoryInterface) GetTimelineEntriesByVisibility(ctx context.Context, visibility string, limit int, cursor string) ([]*models.Timeline, string, error) {
	args := m.Called(ctx, visibility, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Timeline), args.String(1), args.Error(2)
}

// GetTimelineEntriesByLanguage mocks the GetTimelineEntriesByLanguage method
func (m *MockTimelineRepositoryInterface) GetTimelineEntriesByLanguage(ctx context.Context, language string, limit int, cursor string) ([]*models.Timeline, string, error) {
	args := m.Called(ctx, language, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Timeline), args.String(1), args.Error(2)
}


// Advanced timeline queries

// GetTimelineEntriesInRange mocks the GetTimelineEntriesInRange method
func (m *MockTimelineRepositoryInterface) GetTimelineEntriesInRange(ctx context.Context, timelineType, timelineID string, startTime, endTime time.Time, limit int) ([]*models.Timeline, error) {
	args := m.Called(ctx, timelineType, timelineID, startTime, endTime, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Timeline), args.Error(1)
}

// GetTimelineEntriesWithFilters mocks the GetTimelineEntriesWithFilters method
func (m *MockTimelineRepositoryInterface) GetTimelineEntriesWithFilters(ctx context.Context, timelineType, timelineID string, filters interfaces.TimelineFilters, limit int, cursor string) ([]*models.Timeline, string, error) {
	args := m.Called(ctx, timelineType, timelineID, filters, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Timeline), args.String(1), args.Error(2)
}

// CountTimelineEntries mocks the CountTimelineEntries method
func (m *MockTimelineRepositoryInterface) CountTimelineEntries(ctx context.Context, timelineType, timelineID string) (int, error) {
	args := m.Called(ctx, timelineType, timelineID)
	return args.Int(0), args.Error(1)
}

// Batch operations

// DeleteTimelineEntriesByPost mocks the DeleteTimelineEntriesByPost method
func (m *MockTimelineRepositoryInterface) DeleteTimelineEntriesByPost(ctx context.Context, postID string) error {
	args := m.Called(ctx, postID)
	return args.Error(0)
}

// DeleteExpiredTimelineEntries mocks the DeleteExpiredTimelineEntries method
func (m *MockTimelineRepositoryInterface) DeleteExpiredTimelineEntries(ctx context.Context, before time.Time) error {
	args := m.Called(ctx, before)
	return args.Error(0)
}

// RemoveFromTimelines mocks the RemoveFromTimelines method
func (m *MockTimelineRepositoryInterface) RemoveFromTimelines(ctx context.Context, objectID string) error {
	args := m.Called(ctx, objectID)
	return args.Error(0)
}

// Conversation support

// GetConversations mocks the GetConversations method
func (m *MockTimelineRepositoryInterface) GetConversations(ctx context.Context, username string, limit int, cursor string) ([]*models.Conversation, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Conversation), args.String(1), args.Error(2)
}

// Ensure MockTimelineRepositoryInterface implements interfaces.TimelineRepository
var _ interfaces.TimelineRepository = (*MockTimelineRepositoryInterface)(nil)
