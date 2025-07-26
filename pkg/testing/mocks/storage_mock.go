package mocks

import (
	"context"
	"fmt"
	"sync"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/mock"
)

// MockStorage is a mock implementation of the Storage interface
type MockStorage struct {
	mock.Mock
	mu sync.RWMutex

	// In-memory storage for testing
	actors     map[string]*activitypub.Actor
	activities map[string]*activitypub.Activity
	objects    map[string]any
}

// NewMockStorage creates a new mock storage instance
func NewMockStorage() *MockStorage {
	return &MockStorage{
		actors:     make(map[string]*activitypub.Actor),
		activities: make(map[string]*activitypub.Activity),
		objects:    make(map[string]any),
	}
}

// Actor operations
func (m *MockStorage) CreateActor(ctx context.Context, actor *activitypub.Actor, privateKey string) error {
	args := m.Called(ctx, actor, privateKey)
	return args.Error(0)
}

func (m *MockStorage) GetActor(ctx context.Context, username string) (*activitypub.Actor, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activitypub.Actor), args.Error(1)
}

func (m *MockStorage) GetActorByNumericID(ctx context.Context, numericID string) (*activitypub.Actor, error) {
	args := m.Called(ctx, numericID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activitypub.Actor), args.Error(1)
}

func (m *MockStorage) GetActorWithMetadata(ctx context.Context, username string) (*activitypub.Actor, *storage.ActorMetadata, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, nil, args.Error(2)
	}
	return args.Get(0).(*activitypub.Actor), args.Get(1).(*storage.ActorMetadata), args.Error(2)
}

func (m *MockStorage) GetActorPrivateKey(ctx context.Context, username string) (string, error) {
	args := m.Called(ctx, username)
	return args.String(0), args.Error(1)
}

func (m *MockStorage) UpdateActor(ctx context.Context, actor *activitypub.Actor) error {
	args := m.Called(ctx, actor)
	return args.Error(0)
}

func (m *MockStorage) UpdateActorLastStatusTime(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

func (m *MockStorage) SetActorFields(ctx context.Context, username string, fields []storage.ActorField) error {
	args := m.Called(ctx, username, fields)
	return args.Error(0)
}

func (m *MockStorage) DeleteActor(ctx context.Context, username string) error {
	args := m.Called(ctx, username)
	return args.Error(0)
}

func (m *MockStorage) SearchAccounts(ctx context.Context, query string, limit int, followingOnly bool, offset int) ([]*activitypub.Actor, error) {
	args := m.Called(ctx, query, limit, followingOnly, offset)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*activitypub.Actor), args.Error(1)
}

func (m *MockStorage) GetSearchSuggestions(ctx context.Context, prefix string) ([]storage.SearchSuggestion, error) {
	args := m.Called(ctx, prefix)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]storage.SearchSuggestion), args.Error(1)
}

// Status search operations
func (m *MockStorage) SearchStatuses(ctx context.Context, query string, limit int) ([]*storage.StatusSearchResult, error) {
	args := m.Called(ctx, query, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusSearchResult), args.Error(1)
}

func (m *MockStorage) SearchStatusesWithOptions(ctx context.Context, query string, options storage.StatusSearchOptions) ([]*storage.StatusSearchResult, error) {
	args := m.Called(ctx, query, options)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusSearchResult), args.Error(1)
}

func (m *MockStorage) SearchStatusesByURL(ctx context.Context, url string) (*storage.StatusSearchResult, error) {
	args := m.Called(ctx, url)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.StatusSearchResult), args.Error(1)
}

// Enhanced search operations
func (m *MockStorage) SearchAll(ctx context.Context, query string, limit int, accountID string) (*storage.SearchResults, error) {
	args := m.Called(ctx, query, limit, accountID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.SearchResults), args.Error(1)
}

func (m *MockStorage) SearchAccountsAdvanced(ctx context.Context, query string, resolve bool, limit int, offset int, following bool, accountID string) ([]*activitypub.Actor, error) {
	args := m.Called(ctx, query, resolve, limit, offset, following, accountID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*activitypub.Actor), args.Error(1)
}

func (m *MockStorage) SearchStatusesAdvanced(ctx context.Context, query string, limit int, maxID, minID *string, accountID string) ([]*storage.StatusSearchResult, error) {
	args := m.Called(ctx, query, limit, maxID, minID, accountID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StatusSearchResult), args.Error(1)
}

func (m *MockStorage) SearchHashtagsAdvanced(ctx context.Context, query string, limit int, accountID string) ([]*storage.HashtagSearchResult, error) {
	args := m.Called(ctx, query, limit, accountID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.HashtagSearchResult), args.Error(1)
}

// Activity operations
func (m *MockStorage) CreateActivity(ctx context.Context, activity *activitypub.Activity) error {
	args := m.Called(ctx, activity)
	return args.Error(0)
}

func (m *MockStorage) GetActivity(ctx context.Context, id string) (*activitypub.Activity, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activitypub.Activity), args.Error(1)
}

func (m *MockStorage) GetOutboxActivities(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Activity, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*activitypub.Activity), args.String(1), args.Error(2)
}

func (m *MockStorage) GetInboxActivities(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Activity, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*activitypub.Activity), args.String(1), args.Error(2)
}

// Object operations
func (m *MockStorage) CreateObject(ctx context.Context, object any) error {
	args := m.Called(ctx, object)
	return args.Error(0)
}

func (m *MockStorage) GetObject(ctx context.Context, id string) (any, error) {
	args := m.Called(ctx, id)
	return args.Get(0), args.Error(1)
}

func (m *MockStorage) UpdateObject(ctx context.Context, object any) error {
	args := m.Called(ctx, object)
	return args.Error(0)
}

func (m *MockStorage) DeleteObject(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockStorage) GetObjectsByActor(ctx context.Context, actorID string, cursor string, limit int) ([]any, string, error) {
	args := m.Called(ctx, actorID, cursor, limit)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]any), args.String(1), args.Error(2)
}

// MockStorageWithPresets creates a mock storage with common test presets
type MockStoragePreset string

const (
	PresetEmpty          MockStoragePreset = "empty"
	PresetWithTestUser   MockStoragePreset = "test_user"
	PresetWithMultiUsers MockStoragePreset = "multi_users"
)

// NewMockStorageWithPreset creates a mock storage with preset data
func NewMockStorageWithPreset(preset MockStoragePreset) *MockStorage {
	m := NewMockStorage()

	switch preset {
	case PresetWithTestUser:
		actor := &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   "https://example.com/users/testuser",
				Type: "Person",
			},
			PreferredUsername: "testuser",
			Inbox:             "https://example.com/users/testuser/inbox",
			Outbox:            "https://example.com/users/testuser/outbox",
		}
		m.Mock.On("GetActor", mock.Anything, "testuser").Return(actor, nil)
		m.Mock.On("GetActorPrivateKey", mock.Anything, "testuser").Return("test-private-key", nil)

	case PresetWithMultiUsers:
		for i := 1; i <= 5; i++ {
			username := fmt.Sprintf("user%d", i)
			actor := &activitypub.Actor{
				BaseObject: activitypub.BaseObject{
					ID:   fmt.Sprintf("https://example.com/users/%s", username),
					Type: "Person",
				},
				PreferredUsername: username,
				Inbox:             fmt.Sprintf("https://example.com/users/%s/inbox", username),
				Outbox:            fmt.Sprintf("https://example.com/users/%s/outbox", username),
			}
			m.Mock.On("GetActor", mock.Anything, username).Return(actor, nil)
		}
	}

	return m
}

// Helper to implement remaining methods - this would need to be completed for all Storage interface methods
// For brevity, I'm showing the pattern but you'd need to implement all methods
