package mocks

import (
	"context"
	"reflect"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/theorydb"
	"github.com/stretchr/testify/mock"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
)

// MockUserRepository mocks the UserRepository interface
type MockUserRepository struct {
	mock.Mock
	*theorydb.BaseRepository
}

// NewMockUserRepository creates a new mock user repository
func NewMockUserRepository() *MockUserRepository {
	mockDB := new(mocks.MockDB)
	return &MockUserRepository{
		BaseRepository: theorydb.NewBaseRepository(mockDB, "test-table"),
	}
}

// Create mocks the Create method
func (m *MockUserRepository) Create(ctx context.Context, user *models.User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}

// GetByID mocks the GetByID method
func (m *MockUserRepository) GetByID(ctx context.Context, userID string) (*models.User, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}

// GetByUsername mocks the GetByUsername method
func (m *MockUserRepository) GetByUsername(ctx context.Context, username string) (*models.User, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}

// Update mocks the Update method
func (m *MockUserRepository) Update(ctx context.Context, user *models.User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}

// Delete mocks the Delete method
func (m *MockUserRepository) Delete(ctx context.Context, userID string) error {
	args := m.Called(ctx, userID)
	return args.Error(0)
}

// MockStatusRepository mocks the StatusRepository interface
type MockStatusRepository struct {
	mock.Mock
	*theorydb.BaseRepository
}

// NewMockStatusRepository creates a new mock status repository
func NewMockStatusRepository() *MockStatusRepository {
	mockDB := new(mocks.MockDB)
	return &MockStatusRepository{
		BaseRepository: theorydb.NewBaseRepository(mockDB, "test-table"),
	}
}

// Create mocks the Create method
func (m *MockStatusRepository) Create(ctx context.Context, status *models.Status) error {
	args := m.Called(ctx, status)
	return args.Error(0)
}

// GetByID mocks the GetByID method
func (m *MockStatusRepository) GetByID(ctx context.Context, statusID string) (*models.Status, error) {
	args := m.Called(ctx, statusID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Status), args.Error(1)
}

// GetByAuthor mocks the GetByAuthor method
func (m *MockStatusRepository) GetByAuthor(ctx context.Context, authorID string, limit int, cursor string) ([]*models.Status, string, error) {
	args := m.Called(ctx, authorID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Status), args.String(1), args.Error(2)
}

// Update mocks the Update method
func (m *MockStatusRepository) Update(ctx context.Context, status *models.Status) error {
	args := m.Called(ctx, status)
	return args.Error(0)
}

// Delete mocks the Delete method
func (m *MockStatusRepository) Delete(ctx context.Context, statusID string) error {
	args := m.Called(ctx, statusID)
	return args.Error(0)
}

// MockTimelineRepository mocks the TimelineRepository interface
type MockTimelineRepository struct {
	mock.Mock
	*theorydb.BaseRepository
}

// NewMockTimelineRepository creates a new mock timeline repository
func NewMockTimelineRepository() *MockTimelineRepository {
	mockDB := new(mocks.MockDB)
	return &MockTimelineRepository{
		BaseRepository: theorydb.NewBaseRepository(mockDB, "test-table"),
	}
}

// CreateEntry mocks the CreateEntry method
func (m *MockTimelineRepository) CreateEntry(ctx context.Context, entry *models.Timeline) error {
	args := m.Called(ctx, entry)
	return args.Error(0)
}

// GetTimeline mocks the GetTimeline method
func (m *MockTimelineRepository) GetTimeline(ctx context.Context, timelineType, timelineID string, limit int, cursor string) ([]*models.Timeline, string, error) {
	args := m.Called(ctx, timelineType, timelineID, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*models.Timeline), args.String(1), args.Error(2)
}

// DeleteEntry mocks the DeleteEntry method
func (m *MockTimelineRepository) DeleteEntry(ctx context.Context, timelineType, timelineID, entryID string) error {
	args := m.Called(ctx, timelineType, timelineID, entryID)
	return args.Error(0)
}

// MockRepositoryFactory provides all mock repositories
type MockRepositoryFactory struct {
	UserRepo     *MockUserRepository
	StatusRepo   *MockStatusRepository
	TimelineRepo *MockTimelineRepository
}

// NewMockRepositoryFactory creates a new factory with all mock repositories
func NewMockRepositoryFactory() *MockRepositoryFactory {
	return &MockRepositoryFactory{
		UserRepo:     NewMockUserRepository(),
		StatusRepo:   NewMockStatusRepository(),
		TimelineRepo: NewMockTimelineRepository(),
	}
}

// NewMockDBWithExpectations creates a mock DB with common expectations
func NewMockDBWithExpectations() *mocks.MockDB {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// Common query patterns
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()

	return mockDB
}

// RepositoryTestHelper provides utilities for repository testing
type RepositoryTestHelper struct {
	MockDB       *mocks.MockDB
	MockQuery    *mocks.MockQuery
	Repositories *MockRepositoryFactory
}

// NewRepositoryTestHelper creates a new test helper
func NewRepositoryTestHelper() *RepositoryTestHelper {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// Set up common expectations
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	return &RepositoryTestHelper{
		MockDB:       mockDB,
		MockQuery:    mockQuery,
		Repositories: NewMockRepositoryFactory(),
	}
}

// ExpectCreate sets up expectations for a create operation
func (h *RepositoryTestHelper) ExpectCreate(_ interface{}, err error) {
	h.MockQuery.On("Create").Return(err).Once()
}

// ExpectGet sets up expectations for a get operation
func (h *RepositoryTestHelper) ExpectGet(result interface{}, err error) {
	h.MockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		// Copy result to the destination if successful
		if err == nil && result != nil {
			dest := args.Get(0)
			// Use reflection to copy the mock result to the destination
			destValue := reflect.ValueOf(dest).Elem()
			resultValue := reflect.ValueOf(result).Elem()
			destValue.Set(resultValue)
		}
	}).Return(err).Once()
}

// ExpectUpdate sets up expectations for an update operation
func (h *RepositoryTestHelper) ExpectUpdate(err error) {
	h.MockQuery.On("Update").Return(err).Once()
}

// ExpectDelete sets up expectations for a delete operation
func (h *RepositoryTestHelper) ExpectDelete(err error) {
	h.MockQuery.On("Delete").Return(err).Once()
}

// ExpectQuery sets up expectations for a query operation
func (h *RepositoryTestHelper) ExpectQuery(results interface{}, err error) {
	h.MockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		// Copy results to the destination if successful
		if err == nil && results != nil {
			dest := args.Get(0)
			// Use reflection to copy the mock results to the destination slice
			destValue := reflect.ValueOf(dest).Elem()
			resultsValue := reflect.ValueOf(results)
			destValue.Set(resultsValue)
		}
	}).Return(err).Once()
}
