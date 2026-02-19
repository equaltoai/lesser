// Package mocks provides mock implementations for testing.
package mocks

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
)

// MockStatusRepositoryInterface is a mock implementation of interfaces.StatusRepository
// using testify/mock for expectation-based testing.
// Note: This is separate from MockStatusRepository which is an older mock that doesn't
// implement the full interface.
type MockStatusRepositoryInterface struct {
	mock.Mock
}

// NewMockStatusRepositoryInterface creates a new mock status repository
func NewMockStatusRepositoryInterface() *MockStatusRepositoryInterface {
	return &MockStatusRepositoryInterface{}
}

// Core CRUD operations

// CreateStatus mocks the CreateStatus method
func (m *MockStatusRepositoryInterface) CreateStatus(ctx context.Context, status *models.Status) error {
	args := m.Called(ctx, status)
	return args.Error(0)
}

// CreateBoostStatus mocks the CreateBoostStatus method
func (m *MockStatusRepositoryInterface) CreateBoostStatus(ctx context.Context, status *models.Status) error {
	args := m.Called(ctx, status)
	return args.Error(0)
}

// GetStatus mocks the GetStatus method
func (m *MockStatusRepositoryInterface) GetStatus(ctx context.Context, statusID string) (*models.Status, error) {
	args := m.Called(ctx, statusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Status), args.Error(1)
}

// GetStatusByURL mocks the GetStatusByURL method
func (m *MockStatusRepositoryInterface) GetStatusByURL(ctx context.Context, url string) (*models.Status, error) {
	args := m.Called(ctx, url)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Status), args.Error(1)
}

// UpdateStatus mocks the UpdateStatus method
func (m *MockStatusRepositoryInterface) UpdateStatus(ctx context.Context, status *models.Status) error {
	args := m.Called(ctx, status)
	return args.Error(0)
}

// DeleteStatus mocks the DeleteStatus method
func (m *MockStatusRepositoryInterface) DeleteStatus(ctx context.Context, statusID string) error {
	args := m.Called(ctx, statusID)
	return args.Error(0)
}

// DeleteBoostStatus mocks the DeleteBoostStatus method
func (m *MockStatusRepositoryInterface) DeleteBoostStatus(ctx context.Context, boosterID, targetStatusID string) (*models.Status, error) {
	args := m.Called(ctx, boosterID, targetStatusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Status), args.Error(1)
}

// Timeline operations

// GetPublicTimeline mocks the GetPublicTimeline method
func (m *MockStatusRepositoryInterface) GetPublicTimeline(ctx context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

// GetHomeTimeline mocks the GetHomeTimeline method
func (m *MockStatusRepositoryInterface) GetHomeTimeline(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, userID, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

// GetUserTimeline mocks the GetUserTimeline method
func (m *MockStatusRepositoryInterface) GetUserTimeline(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, userID, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

// GetConversationThread mocks the GetConversationThread method
func (m *MockStatusRepositoryInterface) GetConversationThread(ctx context.Context, conversationID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, conversationID, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

// GetConversationThreadReverse mocks the GetConversationThreadReverse method
func (m *MockStatusRepositoryInterface) GetConversationThreadReverse(ctx context.Context, conversationID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, conversationID, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

// GetReplies mocks the GetReplies method
func (m *MockStatusRepositoryInterface) GetReplies(ctx context.Context, parentStatusID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, parentStatusID, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

// Search and discovery

// SearchStatuses mocks the SearchStatuses method
func (m *MockStatusRepositoryInterface) SearchStatuses(ctx context.Context, query string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, query, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

// GetStatusesByHashtag mocks the GetStatusesByHashtag method
func (m *MockStatusRepositoryInterface) GetStatusesByHashtag(ctx context.Context, hashtag string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, hashtag, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

// GetTrendingStatuses mocks the GetTrendingStatuses method
func (m *MockStatusRepositoryInterface) GetTrendingStatuses(ctx context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

// Engagement operations

// LikeStatus mocks the LikeStatus method
func (m *MockStatusRepositoryInterface) LikeStatus(ctx context.Context, userID, statusID string) error {
	args := m.Called(ctx, userID, statusID)
	return args.Error(0)
}

// UnlikeStatus mocks the UnlikeStatus method
func (m *MockStatusRepositoryInterface) UnlikeStatus(ctx context.Context, userID, statusID string) error {
	args := m.Called(ctx, userID, statusID)
	return args.Error(0)
}

// ReblogStatus mocks the ReblogStatus method
func (m *MockStatusRepositoryInterface) ReblogStatus(ctx context.Context, userID, statusID string, reblogStatusID string) error {
	args := m.Called(ctx, userID, statusID, reblogStatusID)
	return args.Error(0)
}

// UnreblogStatus mocks the UnreblogStatus method
func (m *MockStatusRepositoryInterface) UnreblogStatus(ctx context.Context, userID, statusID string) error {
	args := m.Called(ctx, userID, statusID)
	return args.Error(0)
}

// BookmarkStatus mocks the BookmarkStatus method
func (m *MockStatusRepositoryInterface) BookmarkStatus(ctx context.Context, userID, statusID string) error {
	args := m.Called(ctx, userID, statusID)
	return args.Error(0)
}

// UnbookmarkStatus mocks the UnbookmarkStatus method
func (m *MockStatusRepositoryInterface) UnbookmarkStatus(ctx context.Context, userID, statusID string) error {
	args := m.Called(ctx, userID, statusID)
	return args.Error(0)
}

// Moderation operations

// FlagStatus mocks the FlagStatus method
func (m *MockStatusRepositoryInterface) FlagStatus(ctx context.Context, statusID, reason string, reportedBy string) error {
	args := m.Called(ctx, statusID, reason, reportedBy)
	return args.Error(0)
}

// UnflagStatus mocks the UnflagStatus method
func (m *MockStatusRepositoryInterface) UnflagStatus(ctx context.Context, statusID string) error {
	args := m.Called(ctx, statusID)
	return args.Error(0)
}

// GetFlaggedStatuses mocks the GetFlaggedStatuses method
func (m *MockStatusRepositoryInterface) GetFlaggedStatuses(ctx context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

// Batch operations

// GetStatusesByIDs mocks the GetStatusesByIDs method
func (m *MockStatusRepositoryInterface) GetStatusesByIDs(ctx context.Context, statusIDs []string) ([]*models.Status, error) {
	args := m.Called(ctx, statusIDs)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Status), args.Error(1)
}

// GetStatusCounts mocks the GetStatusCounts method
func (m *MockStatusRepositoryInterface) GetStatusCounts(ctx context.Context, statusID string) (likes, reblogs, replies int, err error) {
	args := m.Called(ctx, statusID)
	return args.Int(0), args.Int(1), args.Int(2), args.Error(3)
}

// Context and metadata

// GetStatusContext mocks the GetStatusContext method
func (m *MockStatusRepositoryInterface) GetStatusContext(ctx context.Context, statusID string) (ancestors, descendants []*models.Status, err error) {
	args := m.Called(ctx, statusID)
	var anc, desc []*models.Status
	if args.Get(0) != nil {
		anc = args.Get(0).([]*models.Status)
	}
	if args.Get(1) != nil {
		desc = args.Get(1).([]*models.Status)
	}
	return anc, desc, args.Error(2)
}

// GetStatusEngagement mocks the GetStatusEngagement method
func (m *MockStatusRepositoryInterface) GetStatusEngagement(ctx context.Context, statusID, userID string) (liked, reblogged, bookmarked bool, err error) {
	args := m.Called(ctx, statusID, userID)
	return args.Bool(0), args.Bool(1), args.Bool(2), args.Error(3)
}

// Ensure MockStatusRepositoryInterface implements interfaces.StatusRepository
var _ interfaces.StatusRepository = (*MockStatusRepositoryInterface)(nil)

// Count operations

// CountStatusesByAuthor mocks the CountStatusesByAuthor method
func (m *MockStatusRepositoryInterface) CountStatusesByAuthor(ctx context.Context, authorID string) (int, error) {
	args := m.Called(ctx, authorID)
	return args.Int(0), args.Error(1)
}

// CountReplies mocks the CountReplies method
func (m *MockStatusRepositoryInterface) CountReplies(ctx context.Context, statusID string) (int, error) {
	args := m.Called(ctx, statusID)
	return args.Int(0), args.Error(1)
}

// ListStatusesForAdmin mocks the ListStatusesForAdmin method
func (m *MockStatusRepositoryInterface) ListStatusesForAdmin(ctx context.Context, filter *interfaces.StatusFilter, limit int, cursor string) ([]*models.Status, string, error) {
	args := m.Called(ctx, filter, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Status), args.String(1), args.Error(2)
}
