package main

import (
	"context"
	"testing"
	"time"

	"github.com/aron23/lesser/internal/testutil/mocks"
	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestProcessMove(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	tests := []struct {
		name         string
		activity     *activitypub.Activity
		recipient    string
		setupMocks   func(*mocks.MockStorage)
		expectError  bool
		errorMessage string
	}{
		{
			name: "valid move with target as string",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					ID:        "https://example.com/activities/move-1",
					Type:      activitypub.MoveType,
					Published: &now,
				},
				Actor:  "https://oldserver.com/users/alice",
				Object: "https://newserver.com/users/alice",
			},
			recipient: "followers",
			setupMocks: func(m *mocks.MockStorage) {
				m.On("CreateMove", mock.Anything, mock.MatchedBy(func(move *storage.Move) bool {
					return move.ID == "https://example.com/activities/move-1" &&
						move.Actor == "https://oldserver.com/users/alice" &&
						move.Target == "https://newserver.com/users/alice"
				})).Return(nil)
			},
			expectError: false,
		},
		{
			name: "move with target in object map",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					ID:        "https://example.com/activities/move-2",
					Type:      activitypub.MoveType,
					Published: &now,
				},
				Actor: "https://oldserver.com/users/bob",
				Object: map[string]interface{}{
					"type":   "Person",
					"id":     "https://oldserver.com/users/bob",
					"target": "https://newserver.com/users/bob",
				},
			},
			recipient: "followers",
			setupMocks: func(m *mocks.MockStorage) {
				m.On("CreateMove", mock.Anything, mock.MatchedBy(func(move *storage.Move) bool {
					return move.ID == "https://example.com/activities/move-2" &&
						move.Actor == "https://oldserver.com/users/bob" &&
						move.Target == "https://newserver.com/users/bob"
				})).Return(nil)
			},
			expectError: false,
		},
		{
			name: "move without target",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					ID:   "https://example.com/activities/move-3",
					Type: activitypub.MoveType,
				},
				Actor: "https://oldserver.com/users/charlie",
				Object: map[string]interface{}{
					"type": "Person",
					"id":   "https://oldserver.com/users/charlie",
				},
			},
			recipient:    "followers",
			setupMocks:   func(m *mocks.MockStorage) {},
			expectError:  true,
			errorMessage: "move activity missing target",
		},
		{
			name: "move with published date",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					ID:        "https://example.com/activities/move-4",
					Type:      activitypub.MoveType,
					Published: &now,
				},
				Actor:  "https://oldserver.com/users/dave",
				Object: "https://newserver.com/users/dave",
			},
			recipient: "followers",
			setupMocks: func(m *mocks.MockStorage) {
				m.On("CreateMove", mock.Anything, mock.MatchedBy(func(move *storage.Move) bool {
					return move.Published.Equal(now)
				})).Return(nil)
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := &mocks.MockStorage{}
			tt.setupMocks(mockStore)

			oldStore := store
			store = mockStore
			defer func() { store = oldStore }()

			err := processMove(ctx, tt.activity, tt.recipient)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorMessage != "" {
					assert.Contains(t, err.Error(), tt.errorMessage)
				}
			} else {
				require.NoError(t, err)
			}

			mockStore.AssertExpectations(t)
		})
	}
}

func TestProcessAdd(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name         string
		activity     *activitypub.Activity
		recipient    string
		setupMocks   func(*mocks.MockStorage)
		expectError  bool
		errorMessage string
	}{
		{
			name: "add note to featured collection",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					ID:   "https://example.com/activities/add-1",
					Type: activitypub.AddType,
				},
				Actor: "https://example.com/users/alice",
				Object: map[string]interface{}{
					"id":     "https://example.com/posts/1",
					"type":   "Note",
					"target": "https://example.com/users/alice/collections/featured",
				},
			},
			recipient: "alice",
			setupMocks: func(m *mocks.MockStorage) {
				m.On("AddToCollection", mock.Anything,
					"https://example.com/users/alice/collections/featured",
					mock.MatchedBy(func(item *storage.CollectionItem) bool {
						return item.ItemID == "https://example.com/posts/1" &&
							item.ItemType == "Note" &&
							item.AddedBy == "https://example.com/users/alice" &&
							item.Collection == "https://example.com/users/alice/collections/featured"
					})).Return(nil)
			},
			expectError: false,
		},
		{
			name: "add with object as string ID",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					ID:   "https://example.com/activities/add-2",
					Type: activitypub.AddType,
				},
				Actor: "https://example.com/users/bob",
				Object: map[string]interface{}{
					"id":     "https://example.com/posts/2",
					"target": "https://example.com/users/bob/collections/bookmarks",
				},
			},
			recipient: "bob",
			setupMocks: func(m *mocks.MockStorage) {
				m.On("AddToCollection", mock.Anything,
					"https://example.com/users/bob/collections/bookmarks",
					mock.MatchedBy(func(item *storage.CollectionItem) bool {
						return item.ItemID == "https://example.com/posts/2" &&
							item.ItemType == "Object" // Default type
					})).Return(nil)
			},
			expectError: false,
		},
		{
			name: "add without object",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					ID:   "https://example.com/activities/add-3",
					Type: activitypub.AddType,
				},
				Actor:  "https://example.com/users/charlie",
				Object: map[string]interface{}{},
			},
			recipient:    "charlie",
			setupMocks:   func(m *mocks.MockStorage) {},
			expectError:  true,
			errorMessage: "add activity missing object",
		},
		{
			name: "add without target collection",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					ID:   "https://example.com/activities/add-4",
					Type: activitypub.AddType,
				},
				Actor: "https://example.com/users/dave",
				Object: map[string]interface{}{
					"id":   "https://example.com/posts/3",
					"type": "Note",
				},
			},
			recipient:    "dave",
			setupMocks:   func(m *mocks.MockStorage) {},
			expectError:  true,
			errorMessage: "add activity missing target collection",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := &mocks.MockStorage{}
			tt.setupMocks(mockStore)

			oldStore := store
			store = mockStore
			defer func() { store = oldStore }()

			err := processAdd(ctx, tt.activity, tt.recipient)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorMessage != "" {
					assert.Contains(t, err.Error(), tt.errorMessage)
				}
			} else {
				require.NoError(t, err)
			}

			mockStore.AssertExpectations(t)
		})
	}
}

func TestProcessRemove(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name         string
		activity     *activitypub.Activity
		recipient    string
		setupMocks   func(*mocks.MockStorage)
		expectError  bool
		errorMessage string
	}{
		{
			name: "remove note from featured collection",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					ID:   "https://example.com/activities/remove-1",
					Type: activitypub.RemoveType,
				},
				Actor: "https://example.com/users/alice",
				Object: map[string]interface{}{
					"id":     "https://example.com/posts/1",
					"target": "https://example.com/users/alice/collections/featured",
				},
			},
			recipient: "alice",
			setupMocks: func(m *mocks.MockStorage) {
				m.On("RemoveFromCollection", mock.Anything,
					"https://example.com/users/alice/collections/featured",
					"https://example.com/posts/1").Return(nil)
			},
			expectError: false,
		},
		{
			name: "remove with object as string",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					ID:   "https://example.com/activities/remove-2",
					Type: activitypub.RemoveType,
				},
				Actor: "https://example.com/users/bob",
				Object: map[string]interface{}{
					"id":     "https://example.com/posts/2",
					"target": "https://example.com/users/bob/collections/bookmarks",
				},
			},
			recipient: "bob",
			setupMocks: func(m *mocks.MockStorage) {
				m.On("RemoveFromCollection", mock.Anything,
					"https://example.com/users/bob/collections/bookmarks",
					"https://example.com/posts/2").Return(nil)
			},
			expectError: false,
		},
		{
			name: "remove without object",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					ID:   "https://example.com/activities/remove-3",
					Type: activitypub.RemoveType,
				},
				Actor:  "https://example.com/users/charlie",
				Object: map[string]interface{}{},
			},
			recipient:    "charlie",
			setupMocks:   func(m *mocks.MockStorage) {},
			expectError:  true,
			errorMessage: "remove activity missing object",
		},
		{
			name: "remove without target collection",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					ID:   "https://example.com/activities/remove-4",
					Type: activitypub.RemoveType,
				},
				Actor: "https://example.com/users/dave",
				Object: map[string]interface{}{
					"id": "https://example.com/posts/3",
				},
			},
			recipient:    "dave",
			setupMocks:   func(m *mocks.MockStorage) {},
			expectError:  true,
			errorMessage: "remove activity missing target collection",
		},
		{
			name: "remove non-existent item",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					ID:   "https://example.com/activities/remove-5",
					Type: activitypub.RemoveType,
				},
				Actor: "https://example.com/users/eve",
				Object: map[string]interface{}{
					"id":     "https://example.com/posts/nonexistent",
					"target": "https://example.com/users/eve/collections/featured",
				},
			},
			recipient: "eve",
			setupMocks: func(m *mocks.MockStorage) {
				m.On("RemoveFromCollection", mock.Anything,
					"https://example.com/users/eve/collections/featured",
					"https://example.com/posts/nonexistent").Return(nil)
			},
			expectError: false, // RemoveFromCollection should succeed even if item doesn't exist
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := &mocks.MockStorage{}
			tt.setupMocks(mockStore)

			oldStore := store
			store = mockStore
			defer func() { store = oldStore }()

			err := processRemove(ctx, tt.activity, tt.recipient)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorMessage != "" {
					assert.Contains(t, err.Error(), tt.errorMessage)
				}
			} else {
				require.NoError(t, err)
			}

			mockStore.AssertExpectations(t)
		})
	}
}
