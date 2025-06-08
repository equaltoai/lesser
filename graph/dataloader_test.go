package graph

import (
	"context"

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/stretchr/testify/mock"
)

// TODO: These tests require a complete mock implementation of storage.Storage
// For now, they serve as documentation of how DataLoader should be tested.
// The key insight is that with DataLoader:
// - Multiple requests for the same data should result in only one storage call
// - Batching should reduce N+1 queries to 2 queries (one for objects, one for authors)

// MockStorage tracks calls for testing
type MockStorage struct {
	mock.Mock
	callCount map[string]int
}

func NewMockStorage() *MockStorage {
	return &MockStorage{
		callCount: make(map[string]int),
	}
}

func (m *MockStorage) GetActor(ctx context.Context, username string) (*activitypub.Actor, error) {
	m.callCount["GetActor"]++
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*activitypub.Actor), args.Error(1)
}

func (m *MockStorage) GetObject(ctx context.Context, id string) (interface{}, error) {
	m.callCount["GetObject"]++
	args := m.Called(ctx, id)
	return args.Get(0), args.Error(1)
}

// Implement other required storage methods...
// For brevity, we'll just add stubs here

// TODO: Uncomment when we have a proper mock implementation
/*
func TestDataLoaderPreventsN1Queries(t *testing.T) {
	// Create mock storage
	mockStorage := NewMockStorage()
	logger := zap.NewNop()

	// Create test data
	notes := []*activitypub.Note{
		{
			BaseObject: activitypub.BaseObject{
				ID:   "note1",
				Type: "Note",
			},
			Content:      "Test note 1",
			AttributedTo: "user1",
		},
		{
			BaseObject: activitypub.BaseObject{
				ID:   "note2",
				Type: "Note",
			},
			Content:      "Test note 2",
			AttributedTo: "user2",
		},
		{
			BaseObject: activitypub.BaseObject{
				ID:   "note3",
				Type: "Note",
			},
			Content:      "Test note 3",
			AttributedTo: "user1", // Same user as note1
		},
	}

	// Setup mock expectations
	for _, note := range notes {
		mockStorage.On("GetObject", mock.Anything, note.ID).Return(note, nil)
	}

	// User1 appears twice but should only be loaded once
	mockStorage.On("GetActor", mock.Anything, "user1").Return(&activitypub.Actor{
		BaseObject:        activitypub.BaseObject{ID: "user1"},
		PreferredUsername: "user1",
	}, nil).Once() // Important: Once() ensures it's only called once

	mockStorage.On("GetActor", mock.Anything, "user2").Return(&activitypub.Actor{
		BaseObject:        activitypub.BaseObject{ID: "user2"},
		PreferredUsername: "user2",
	}, nil).Once()

	// Create DataLoader
	loaders := NewLoaders(mockStorage, logger)
	ctx := WithLoaders(context.Background(), loaders)

	// Simulate loading multiple objects with their authors
	for _, note := range notes {
		// Load object
		obj, err := LoadObject(ctx, note.ID)
		assert.NoError(t, err)
		assert.NotNil(t, obj)

		// Load author (this should batch)
		if noteObj, ok := obj.(*activitypub.Note); ok {
			actor, err := LoadActor(ctx, noteObj.AttributedTo)
			assert.NoError(t, err)
			assert.NotNil(t, actor)
		}
	}

	// Verify call counts
	assert.Equal(t, 3, mockStorage.callCount["GetObject"], "Should call GetObject 3 times")
	assert.Equal(t, 2, mockStorage.callCount["GetActor"], "Should call GetActor only 2 times (user1 loaded once)")

	// Verify all expectations were met
	mockStorage.AssertExpectations(t)
}

func TestDataLoaderHandlesErrors(t *testing.T) {
	mockStorage := NewMockStorage()
	logger := zap.NewNop()

	// Setup mock to return error
	mockStorage.On("GetActor", mock.Anything, "nonexistent").Return(nil, storage.ErrNotFound)

	loaders := NewLoaders(mockStorage, logger)
	ctx := WithLoaders(context.Background(), loaders)

	// Try to load non-existent actor
	actor, err := LoadActor(ctx, "nonexistent")
	assert.Error(t, err)
	assert.Nil(t, actor)
	assert.Equal(t, storage.ErrNotFound, err)
}

// TestDataLoaderCachingWithinRequest verifies that DataLoader caches within a single request
func TestDataLoaderCachingWithinRequest(t *testing.T) {
	mockStorage := NewMockStorage()
	logger := zap.NewNop()

	// Setup mock - should only be called once due to caching
	mockStorage.On("GetActor", mock.Anything, "user1").Return(&activitypub.Actor{
		BaseObject:        activitypub.BaseObject{ID: "user1"},
		PreferredUsername: "user1",
	}, nil).Once()

	loaders := NewLoaders(mockStorage, logger)
	ctx := WithLoaders(context.Background(), loaders)

	// Load the same actor multiple times
	for i := 0; i < 5; i++ {
		actor, err := LoadActor(ctx, "user1")
		assert.NoError(t, err)
		assert.NotNil(t, actor)
		assert.Equal(t, "user1", actor.PreferredUsername)
	}

	// Verify storage was only called once
	assert.Equal(t, 1, mockStorage.callCount["GetActor"], "Should only call GetActor once due to caching")
	mockStorage.AssertExpectations(t)
}
*/
