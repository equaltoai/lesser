package lists

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// Mock implementations

type MockListRepository struct {
	mock.Mock
}

func (m *MockListRepository) CreateList(ctx context.Context, list *models.List) error {
	args := m.Called(ctx, list)
	return args.Error(0)
}

func (m *MockListRepository) GetList(ctx context.Context, listID string) (*models.List, error) {
	args := m.Called(ctx, listID)
	return args.Get(0).(*models.List), args.Error(1)
}

func (m *MockListRepository) UpdateList(ctx context.Context, list *models.List) error {
	args := m.Called(ctx, list)
	return args.Error(0)
}

func (m *MockListRepository) DeleteList(ctx context.Context, listID string) error {
	args := m.Called(ctx, listID)
	return args.Error(0)
}

func (m *MockListRepository) GetUserLists(ctx context.Context, username string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.List], error) {
	args := m.Called(ctx, username, opts)
	return args.Get(0).(*interfaces.PaginatedResult[*models.List]), args.Error(1)
}

func (m *MockListRepository) GetListsByMember(ctx context.Context, memberUsername string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.List], error) {
	args := m.Called(ctx, memberUsername, opts)
	return args.Get(0).(*interfaces.PaginatedResult[*models.List]), args.Error(1)
}

func (m *MockListRepository) AddListMember(ctx context.Context, listID, memberUsername string) error {
	args := m.Called(ctx, listID, memberUsername)
	return args.Error(0)
}

func (m *MockListRepository) RemoveListMember(ctx context.Context, listID, memberUsername string) error {
	args := m.Called(ctx, listID, memberUsername)
	return args.Error(0)
}

func (m *MockListRepository) GetListMembers(ctx context.Context, listID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	args := m.Called(ctx, listID, opts)
	return args.Get(0).(*interfaces.PaginatedResult[*storage.Account]), args.Error(1)
}

func (m *MockListRepository) IsListMember(ctx context.Context, listID, memberUsername string) (bool, error) {
	args := m.Called(ctx, listID, memberUsername)
	return args.Bool(0), args.Error(1)
}

func (m *MockListRepository) GetListTimeline(ctx context.Context, listID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, listID, opts)
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

func (m *MockListRepository) GetListStatuses(ctx context.Context, listID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, listID, opts)
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

type MockNoteRepository struct {
	mock.Mock
}

func (m *MockNoteRepository) CreateStatus(ctx context.Context, status *models.Status) error {
	args := m.Called(ctx, status)
	return args.Error(0)
}

func (m *MockNoteRepository) GetStatus(ctx context.Context, statusID string) (*models.Status, error) {
	args := m.Called(ctx, statusID)
	return args.Get(0).(*models.Status), args.Error(1)
}

func (m *MockNoteRepository) GetStatusByURL(ctx context.Context, url string) (*models.Status, error) {
	args := m.Called(ctx, url)
	return args.Get(0).(*models.Status), args.Error(1)
}

func (m *MockNoteRepository) UpdateStatus(ctx context.Context, status *models.Status) error {
	args := m.Called(ctx, status)
	return args.Error(0)
}

func (m *MockNoteRepository) DeleteStatus(ctx context.Context, statusID string) error {
	args := m.Called(ctx, statusID)
	return args.Error(0)
}

func (m *MockNoteRepository) GetPublicTimeline(ctx context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, opts)
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

func (m *MockNoteRepository) GetHomeTimeline(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, userID, opts)
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

func (m *MockNoteRepository) GetUserTimeline(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, userID, opts)
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

func (m *MockNoteRepository) GetConversationThread(ctx context.Context, conversationID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, conversationID, opts)
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

func (m *MockNoteRepository) GetReplies(ctx context.Context, parentStatusID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, parentStatusID, opts)
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

func (m *MockNoteRepository) SearchStatuses(ctx context.Context, query string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, query, opts)
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

func (m *MockNoteRepository) GetStatusesByHashtag(ctx context.Context, hashtag string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, hashtag, opts)
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

func (m *MockNoteRepository) GetTrendingStatuses(ctx context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, opts)
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

func (m *MockNoteRepository) LikeStatus(ctx context.Context, userID, statusID string) error {
	args := m.Called(ctx, userID, statusID)
	return args.Error(0)
}

func (m *MockNoteRepository) UnlikeStatus(ctx context.Context, userID, statusID string) error {
	args := m.Called(ctx, userID, statusID)
	return args.Error(0)
}

func (m *MockNoteRepository) ReblogStatus(ctx context.Context, userID, statusID string, reblogStatusID string) error {
	args := m.Called(ctx, userID, statusID, reblogStatusID)
	return args.Error(0)
}

func (m *MockNoteRepository) UnreblogStatus(ctx context.Context, userID, statusID string) error {
	args := m.Called(ctx, userID, statusID)
	return args.Error(0)
}

func (m *MockNoteRepository) BookmarkStatus(ctx context.Context, userID, statusID string) error {
	args := m.Called(ctx, userID, statusID)
	return args.Error(0)
}

func (m *MockNoteRepository) UnbookmarkStatus(ctx context.Context, userID, statusID string) error {
	args := m.Called(ctx, userID, statusID)
	return args.Error(0)
}

func (m *MockNoteRepository) FlagStatus(ctx context.Context, statusID, reason string, reportedBy string) error {
	args := m.Called(ctx, statusID, reason, reportedBy)
	return args.Error(0)
}

func (m *MockNoteRepository) UnflagStatus(ctx context.Context, statusID string) error {
	args := m.Called(ctx, statusID)
	return args.Error(0)
}

func (m *MockNoteRepository) GetFlaggedStatuses(ctx context.Context, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.Status], error) {
	args := m.Called(ctx, opts)
	return args.Get(0).(*interfaces.PaginatedResult[*models.Status]), args.Error(1)
}

func (m *MockNoteRepository) GetStatusesByIDs(ctx context.Context, statusIDs []string) ([]*models.Status, error) {
	args := m.Called(ctx, statusIDs)
	return args.Get(0).([]*models.Status), args.Error(1)
}

func (m *MockNoteRepository) GetStatusCounts(ctx context.Context, statusID string) (likes, reblogs, replies int, err error) {
	args := m.Called(ctx, statusID)
	return args.Int(0), args.Int(1), args.Int(2), args.Error(3)
}

func (m *MockNoteRepository) GetStatusContext(ctx context.Context, statusID string) (ancestors, descendants []*models.Status, err error) {
	args := m.Called(ctx, statusID)
	return args.Get(0).([]*models.Status), args.Get(1).([]*models.Status), args.Error(2)
}

func (m *MockNoteRepository) GetStatusEngagement(ctx context.Context, statusID, userID string) (liked, reblogged, bookmarked bool, err error) {
	args := m.Called(ctx, statusID, userID)
	return args.Bool(0), args.Bool(1), args.Bool(2), args.Error(3)
}

type MockPublisher struct {
	events []PublishedEvent
}

type PublishedEvent struct {
	Method   string
	TargetID string
	Event    *streaming.Event
}

func (m *MockPublisher) PublishToUser(ctx context.Context, userID string, event *streaming.Event) error {
	m.events = append(m.events, PublishedEvent{
		Method:   "PublishToUser",
		TargetID: userID,
		Event:    event,
	})
	return nil
}

func (m *MockPublisher) PublishToStream(ctx context.Context, streamName string, event *streaming.Event) error {
	m.events = append(m.events, PublishedEvent{
		Method:   "PublishToStream",
		TargetID: streamName,
		Event:    event,
	})
	return nil
}

func (m *MockPublisher) PublishToConversation(ctx context.Context, conversationID string, event *streaming.Event) error {
	m.events = append(m.events, PublishedEvent{
		Method:   "PublishToConversation",
		TargetID: conversationID,
		Event:    event,
	})
	return nil
}

func (m *MockPublisher) Close() error {
	return nil
}

func (m *MockPublisher) GetEvents() []PublishedEvent {
	return m.events
}

func (m *MockPublisher) Reset() {
	m.events = []PublishedEvent{}
}

// Test helper functions

func setupTestService() (*Service, *MockListRepository, *MockNoteRepository, *MockPublisher) {
	listRepo := &MockListRepository{}
	noteRepo := &MockNoteRepository{}
	publisher := &MockPublisher{}
	logger := zap.NewNop()

	service := NewService(listRepo, noteRepo, publisher, logger)
	return service, listRepo, noteRepo, publisher
}

func createTestList() *models.List {
	return &models.List{
		ID:            "test_list_123",
		Username:      "testuser",
		Title:         "Test List",
		RepliesPolicy: "list",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
}

// CreateList Tests

func TestCreateList_Success(t *testing.T) {
	service, listRepo, _, publisher := setupTestService()
	ctx := context.Background()

	cmd := &CreateListCommand{
		Username:      "testuser",
		Title:         "My Test List",
		RepliesPolicy: "list",
		CreatorID:     "testuser",
	}

	listRepo.On("CreateList", ctx, mock.MatchedBy(func(list *models.List) bool {
		return list.Username == "testuser" && list.Title == "My Test List"
	})).Return(nil)

	result, err := service.CreateList(ctx, cmd)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "testuser", result.List.Username)
	assert.Equal(t, "My Test List", result.List.Title)
	assert.Equal(t, "list", result.List.RepliesPolicy)
	assert.Len(t, result.Events, 1)
	assert.Equal(t, "list.created", result.Events[0].Type)

	// Verify event was published
	events := publisher.GetEvents()
	assert.Len(t, events, 1)
	assert.Equal(t, "PublishToUser", events[0].Method)
	assert.Equal(t, "testuser", events[0].TargetID)

	listRepo.AssertExpectations(t)
}

func TestCreateList_DefaultRepliesPolicy(t *testing.T) {
	service, listRepo, _, _ := setupTestService()
	ctx := context.Background()

	cmd := &CreateListCommand{
		Username:  "testuser",
		Title:     "My Test List",
		CreatorID: "testuser",
		// RepliesPolicy not specified
	}

	listRepo.On("CreateList", ctx, mock.MatchedBy(func(list *models.List) bool {
		return list.RepliesPolicy == "list"
	})).Return(nil)

	result, err := service.CreateList(ctx, cmd)

	assert.NoError(t, err)
	assert.Equal(t, "list", result.List.RepliesPolicy)
	listRepo.AssertExpectations(t)
}

func TestCreateList_ValidationErrors(t *testing.T) {
	service, _, _, _ := setupTestService()
	ctx := context.Background()

	testCases := []struct {
		name     string
		cmd      *CreateListCommand
		expected string
	}{
		{
			name: "empty username",
			cmd: &CreateListCommand{
				Title:     "Test",
				CreatorID: "testuser",
			},
			expected: "username is required",
		},
		{
			name: "empty creator_id",
			cmd: &CreateListCommand{
				Username: "testuser",
				Title:    "Test",
			},
			expected: "creator_id is required",
		},
		{
			name: "empty title",
			cmd: &CreateListCommand{
				Username:  "testuser",
				Title:     "",
				CreatorID: "testuser",
			},
			expected: "title is required",
		},
		{
			name: "title too long",
			cmd: &CreateListCommand{
				Username:  "testuser",
				Title:     strings.Repeat("x", 101),
				CreatorID: "testuser",
			},
			expected: "title too long",
		},
		{
			name: "invalid replies policy",
			cmd: &CreateListCommand{
				Username:      "testuser",
				Title:         "Test",
				RepliesPolicy: "invalid",
				CreatorID:     "testuser",
			},
			expected: "invalid replies policy",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.CreateList(ctx, tc.cmd)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tc.expected)
		})
	}
}

func TestCreateList_UnauthorizedUser(t *testing.T) {
	service, _, _, _ := setupTestService()
	ctx := context.Background()

	cmd := &CreateListCommand{
		Username:  "testuser",
		Title:     "Test List",
		CreatorID: "otheruser", // Different user
	}

	_, err := service.CreateList(ctx, cmd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unauthorized")
}

func TestCreateList_RepositoryError(t *testing.T) {
	service, listRepo, _, _ := setupTestService()
	ctx := context.Background()

	cmd := &CreateListCommand{
		Username:  "testuser",
		Title:     "Test List",
		CreatorID: "testuser",
	}

	listRepo.On("CreateList", ctx, mock.Anything).Return(errors.New("database error"))

	_, err := service.CreateList(ctx, cmd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create list")

	listRepo.AssertExpectations(t)
}

// UpdateList Tests

func TestUpdateList_Success(t *testing.T) {
	service, listRepo, _, publisher := setupTestService()
	ctx := context.Background()

	existingList := createTestList()
	cmd := &UpdateListCommand{
		ListID:        "test_list_123",
		Title:         "Updated Title",
		RepliesPolicy: "followed",
		UpdaterID:     "testuser",
	}

	listRepo.On("GetList", ctx, "test_list_123").Return(existingList, nil)
	listRepo.On("UpdateList", ctx, mock.MatchedBy(func(list *models.List) bool {
		return list.Title == "Updated Title" && list.RepliesPolicy == "followed"
	})).Return(nil)

	result, err := service.UpdateList(ctx, cmd)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "Updated Title", result.List.Title)
	assert.Equal(t, "followed", result.List.RepliesPolicy)
	assert.Len(t, result.Events, 1)
	assert.Equal(t, "list.updated", result.Events[0].Type)

	// Verify event was published
	events := publisher.GetEvents()
	assert.Len(t, events, 1)
	assert.Equal(t, "PublishToUser", events[0].Method)

	listRepo.AssertExpectations(t)
}

func TestUpdateList_NoChanges(t *testing.T) {
	service, listRepo, _, publisher := setupTestService()
	ctx := context.Background()

	existingList := createTestList()
	cmd := &UpdateListCommand{
		ListID:    "test_list_123",
		Title:     "Test List", // Same as existing
		UpdaterID: "testuser",
	}

	listRepo.On("GetList", ctx, "test_list_123").Return(existingList, nil)

	result, err := service.UpdateList(ctx, cmd)

	assert.NoError(t, err)
	assert.Len(t, result.Events, 0) // No events when no changes

	// Verify no events were published
	events := publisher.GetEvents()
	assert.Len(t, events, 0)

	listRepo.AssertExpectations(t)
}

func TestUpdateList_UnauthorizedUser(t *testing.T) {
	service, listRepo, _, _ := setupTestService()
	ctx := context.Background()

	existingList := createTestList()
	cmd := &UpdateListCommand{
		ListID:    "test_list_123",
		Title:     "Updated Title",
		UpdaterID: "otheruser", // Different user
	}

	listRepo.On("GetList", ctx, "test_list_123").Return(existingList, nil)

	_, err := service.UpdateList(ctx, cmd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unauthorized")

	listRepo.AssertExpectations(t)
}

// DeleteList Tests

func TestDeleteList_Success(t *testing.T) {
	service, listRepo, _, publisher := setupTestService()
	ctx := context.Background()

	existingList := createTestList()
	cmd := &DeleteListCommand{
		ListID:    "test_list_123",
		DeleterID: "testuser",
	}

	listRepo.On("GetList", ctx, "test_list_123").Return(existingList, nil)
	listRepo.On("DeleteList", ctx, "test_list_123").Return(nil)

	err := service.DeleteList(ctx, cmd)

	assert.NoError(t, err)

	// Verify event was published
	events := publisher.GetEvents()
	assert.Len(t, events, 1)
	assert.Equal(t, "PublishToUser", events[0].Method)
	assert.Equal(t, "list.deleted", events[0].Event.Type)

	listRepo.AssertExpectations(t)
}

func TestDeleteList_UnauthorizedUser(t *testing.T) {
	service, listRepo, _, _ := setupTestService()
	ctx := context.Background()

	existingList := createTestList()
	cmd := &DeleteListCommand{
		ListID:    "test_list_123",
		DeleterID: "otheruser", // Different user
	}

	listRepo.On("GetList", ctx, "test_list_123").Return(existingList, nil)

	err := service.DeleteList(ctx, cmd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unauthorized")

	listRepo.AssertExpectations(t)
}

// AddToList Tests

func TestAddToList_Success(t *testing.T) {
	service, listRepo, _, publisher := setupTestService()
	ctx := context.Background()

	existingList := createTestList()
	cmd := &AddToListCommand{
		ListID:        "test_list_123",
		MemberUsername: "newmember",
		AdderID:       "testuser",
	}

	listRepo.On("GetList", ctx, "test_list_123").Return(existingList, nil)
	listRepo.On("IsListMember", ctx, "test_list_123", "newmember").Return(false, nil)
	listRepo.On("AddListMember", ctx, "test_list_123", "newmember").Return(nil)

	result, err := service.AddToList(ctx, cmd)

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Len(t, result.Events, 1)
	assert.Equal(t, "list.member_added", result.Events[0].Type)

	// Verify event was published
	events := publisher.GetEvents()
	assert.Len(t, events, 1)
	assert.Equal(t, "PublishToUser", events[0].Method)

	listRepo.AssertExpectations(t)
}

func TestAddToList_AlreadyMember(t *testing.T) {
	service, listRepo, _, publisher := setupTestService()
	ctx := context.Background()

	existingList := createTestList()
	cmd := &AddToListCommand{
		ListID:        "test_list_123",
		MemberUsername: "existingmember",
		AdderID:       "testuser",
	}

	listRepo.On("GetList", ctx, "test_list_123").Return(existingList, nil)
	listRepo.On("IsListMember", ctx, "test_list_123", "existingmember").Return(true, nil)

	result, err := service.AddToList(ctx, cmd)

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Len(t, result.Events, 0) // No events when already a member

	// Verify no events were published
	events := publisher.GetEvents()
	assert.Len(t, events, 0)

	listRepo.AssertExpectations(t)
}

// RemoveFromList Tests

func TestRemoveFromList_Success(t *testing.T) {
	service, listRepo, _, publisher := setupTestService()
	ctx := context.Background()

	existingList := createTestList()
	cmd := &RemoveFromListCommand{
		ListID:        "test_list_123",
		MemberUsername: "existingmember",
		RemoverID:     "testuser",
	}

	listRepo.On("GetList", ctx, "test_list_123").Return(existingList, nil)
	listRepo.On("IsListMember", ctx, "test_list_123", "existingmember").Return(true, nil)
	listRepo.On("RemoveListMember", ctx, "test_list_123", "existingmember").Return(nil)

	result, err := service.RemoveFromList(ctx, cmd)

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Len(t, result.Events, 1)
	assert.Equal(t, "list.member_removed", result.Events[0].Type)

	// Verify event was published
	events := publisher.GetEvents()
	assert.Len(t, events, 1)
	assert.Equal(t, "PublishToUser", events[0].Method)

	listRepo.AssertExpectations(t)
}

func TestRemoveFromList_NotMember(t *testing.T) {
	service, listRepo, _, publisher := setupTestService()
	ctx := context.Background()

	existingList := createTestList()
	cmd := &RemoveFromListCommand{
		ListID:        "test_list_123",
		MemberUsername: "nonmember",
		RemoverID:     "testuser",
	}

	listRepo.On("GetList", ctx, "test_list_123").Return(existingList, nil)
	listRepo.On("IsListMember", ctx, "test_list_123", "nonmember").Return(false, nil)

	result, err := service.RemoveFromList(ctx, cmd)

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Len(t, result.Events, 0) // No events when not a member

	// Verify no events were published
	events := publisher.GetEvents()
	assert.Len(t, events, 0)

	listRepo.AssertExpectations(t)
}

// GetList Tests

func TestGetList_Success(t *testing.T) {
	service, listRepo, _, _ := setupTestService()
	ctx := context.Background()

	existingList := createTestList()
	query := &GetListQuery{
		ListID:   "test_list_123",
		ViewerID: "testuser",
	}

	listRepo.On("GetList", ctx, "test_list_123").Return(existingList, nil)

	result, err := service.GetList(ctx, query)

	assert.NoError(t, err)
	assert.Equal(t, existingList, result)

	listRepo.AssertExpectations(t)
}

func TestGetList_UnauthorizedViewer(t *testing.T) {
	service, listRepo, _, _ := setupTestService()
	ctx := context.Background()

	existingList := createTestList()
	query := &GetListQuery{
		ListID:   "test_list_123",
		ViewerID: "otheruser", // Different user
	}

	listRepo.On("GetList", ctx, "test_list_123").Return(existingList, nil)

	_, err := service.GetList(ctx, query)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unauthorized")

	listRepo.AssertExpectations(t)
}

// ListUserLists Tests

func TestListUserLists_Success(t *testing.T) {
	service, listRepo, _, _ := setupTestService()
	ctx := context.Background()

	lists := []*models.List{createTestList()}
	pagination := &interfaces.PaginatedResult[*models.List]{
		Items:   lists,
		HasMore: false,
		Total:   1,
	}

	query := &ListUserListsQuery{
		Username: "testuser",
		ViewerID: "testuser",
		Pagination: interfaces.PaginationOptions{
			Limit: 20,
		},
	}

	listRepo.On("GetUserLists", ctx, "testuser", query.Pagination).Return(pagination, nil)

	result, err := service.ListUserLists(ctx, query)

	assert.NoError(t, err)
	assert.Len(t, result.Lists, 1)
	assert.Equal(t, pagination, result.Pagination)
	assert.Len(t, result.Events, 0) // No events for read operations

	listRepo.AssertExpectations(t)
}

func TestListUserLists_UnauthorizedViewer(t *testing.T) {
	service, _, _, _ := setupTestService()
	ctx := context.Background()

	query := &ListUserListsQuery{
		Username: "testuser",
		ViewerID: "otheruser", // Different user
	}

	_, err := service.ListUserLists(ctx, query)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unauthorized")
}

// GetListTimeline Tests

func TestGetListTimeline_Success(t *testing.T) {
	service, listRepo, _, _ := setupTestService()
	ctx := context.Background()

	existingList := createTestList()
	statuses := []*models.Status{}
	pagination := &interfaces.PaginatedResult[*models.Status]{
		Items:   statuses,
		HasMore: false,
		Total:   0,
	}

	query := &GetListTimelineQuery{
		ListID:   "test_list_123",
		ViewerID: "testuser",
		Pagination: interfaces.PaginationOptions{
			Limit: 20,
		},
	}

	listRepo.On("GetList", ctx, "test_list_123").Return(existingList, nil)
	listRepo.On("GetListTimeline", ctx, "test_list_123", query.Pagination).Return(pagination, nil)

	result, err := service.GetListTimeline(ctx, query)

	assert.NoError(t, err)
	assert.Equal(t, statuses, result.Statuses)
	assert.Equal(t, pagination, result.Pagination)
	assert.Len(t, result.Events, 0) // No events for read operations

	listRepo.AssertExpectations(t)
}

func TestGetListTimeline_UnauthorizedViewer(t *testing.T) {
	service, listRepo, _, _ := setupTestService()
	ctx := context.Background()

	existingList := createTestList()
	query := &GetListTimelineQuery{
		ListID:   "test_list_123",
		ViewerID: "otheruser", // Different user
	}

	listRepo.On("GetList", ctx, "test_list_123").Return(existingList, nil)

	_, err := service.GetListTimeline(ctx, query)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unauthorized")

	listRepo.AssertExpectations(t)
}

// Service Creation Tests

func TestNewService(t *testing.T) {
	listRepo := &MockListRepository{}
	noteRepo := &MockNoteRepository{}
	publisher := &MockPublisher{}

	// Test with logger
	logger := zap.NewNop()
	service := NewService(listRepo, noteRepo, publisher, logger)
	assert.NotNil(t, service)
	assert.Equal(t, listRepo, service.listRepo)
	assert.Equal(t, noteRepo, service.statusRepo)
	assert.Equal(t, publisher, service.publisher)
	assert.Equal(t, logger, service.logger)

	// Test with nil logger (should create nop logger)
	serviceNoLogger := NewService(listRepo, noteRepo, publisher, nil)
	assert.NotNil(t, serviceNoLogger)
	assert.NotNil(t, serviceNoLogger.logger)
}

// Edge cases and error scenarios

func TestValidation_EdgeCases(t *testing.T) {
	service, _, _, _ := setupTestService()
	ctx := context.Background()

	// Test with whitespace-only title
	cmd := &CreateListCommand{
		Username:  "testuser",
		Title:     "   ",
		CreatorID: "testuser",
	}

	_, err := service.CreateList(ctx, cmd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "title is required")
}

func TestGenerateListID(t *testing.T) {
	service, _, _, _ := setupTestService()

	id1 := service.generateListID()
	id2 := service.generateListID()

	assert.NotEmpty(t, id1)
	assert.NotEmpty(t, id2)
	assert.NotEqual(t, id1, id2) // Should be unique
	assert.Contains(t, id1, "list_")
	assert.Contains(t, id2, "list_")
}