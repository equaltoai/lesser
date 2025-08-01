package lift

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

func TestHandleBookmarkLift(t *testing.T) {
	var mockStore *MockStorageAdapter

	tests := []struct {
		name           string
		statusID       string
		setupMocks     func()
		expectedStatus int
		expectError    bool
		checkResponse  func(t *testing.T, ctx *lift.Context)
	}{
		{
			name:     "successful bookmark with test mode",
			statusID: "123",
			setupMocks: func() {
				// Mock object exists
				publishedTime := time.Now().Add(-1 * time.Hour)
				mockNote := &activitypub.Note{
					BaseObject: activitypub.BaseObject{
						ID:        "https://test.example.com/objects/123",
						Type:      "Note",
						Published: &publishedTime,
					},
					AttributedTo: "https://test.example.com/users/author",
					Content:      "Test note content",
				}
				mockStore.On("GetObject", mock.Anything, "https://test.example.com/objects/123").Return(mockNote, nil)

				// Mock bookmark creation
				mockStore.On("CreateBookmark", mock.Anything, "testuser", "https://test.example.com/objects/123").Return(nil)

				// Mock actor for object author
				mockAuthor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   "https://test.example.com/users/author",
						Type: "Person",
					},
					PreferredUsername: "author",
					Name:              "Author Name",
				}
				mockStore.On("GetActor", mock.Anything, "author").Return(mockAuthor, nil)

				// Mock counts and interactions
				mockStore.On("CountObjectLikes", mock.Anything, "https://test.example.com/objects/123").Return(5, nil)
				mockStore.On("CountObjectAnnounces", mock.Anything, "https://test.example.com/objects/123").Return(2, nil)
				mockStore.On("GetLike", mock.Anything, "https://test.example.com/users/testuser", "https://test.example.com/objects/123").Return(nil, errors.New("not found"))
				mockStore.On("GetAnnounce", mock.Anything, "https://test.example.com/users/testuser", "https://test.example.com/objects/123").Return(nil, errors.New("not found"))
			},
			expectedStatus: 200,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				// Response should be a status object with bookmark flag set
				status, ok := ctx.Response.Body.(*models.Status)
				assert.True(t, ok, "Response should be a Status object")
				assert.True(t, status.Bookmarked, "Status should be bookmarked")
				assert.Equal(t, "123", status.ID)
			},
		},
		{
			name:     "bookmark with full URL status ID",
			statusID: "https://remote.example.com/notes/456",
			setupMocks: func() {
				// Mock remote object exists
				publishedTime := time.Now().Add(-1 * time.Hour)
				mockNote := &activitypub.Note{
					BaseObject: activitypub.BaseObject{
						ID:        "https://remote.example.com/notes/456",
						Type:      "Note",
						Published: &publishedTime,
					},
					AttributedTo: "https://remote.example.com/users/remoteuser",
					Content:      "Remote note content",
				}
				mockStore.On("GetObject", mock.Anything, "https://remote.example.com/notes/456").Return(mockNote, nil)

				// Mock bookmark creation
				mockStore.On("CreateBookmark", mock.Anything, "testuser", "https://remote.example.com/notes/456").Return(nil)

				// Mock remote actor
				mockAuthor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   "https://remote.example.com/users/remoteuser",
						Type: "Person",
					},
					PreferredUsername: "remoteuser",
					Name:              "Remote User",
				}
				mockStore.On("GetActor", mock.Anything, "remoteuser").Return(mockAuthor, nil)

				// Mock counts and interactions
				mockStore.On("CountObjectLikes", mock.Anything, "https://remote.example.com/notes/456").Return(0, nil)
				mockStore.On("CountObjectAnnounces", mock.Anything, "https://remote.example.com/notes/456").Return(0, nil)
				mockStore.On("GetLike", mock.Anything, "https://test.example.com/users/testuser", "https://remote.example.com/notes/456").Return(nil, errors.New("not found"))
				mockStore.On("GetAnnounce", mock.Anything, "https://test.example.com/users/testuser", "https://remote.example.com/notes/456").Return(nil, errors.New("not found"))
			},
			expectedStatus: 200,
			expectError:    false,
		},
		{
			name:     "bookmark status not found",
			statusID: "nonexistent",
			setupMocks: func() {
				mockStore.On("GetObject", mock.Anything, "https://test.example.com/objects/nonexistent").Return(nil, errors.New("not found"))
			},
			expectedStatus: 404,
			expectError:    false,
		},
		{
			name:     "bookmark creation fails",
			statusID: "123",
			setupMocks: func() {
				publishedTime := time.Now().Add(-1 * time.Hour)
				mockNote := &activitypub.Note{
					BaseObject: activitypub.BaseObject{
						ID:        "https://test.example.com/objects/123",
						Type:      "Note",
						Published: &publishedTime,
					},
					AttributedTo: "https://test.example.com/users/author",
					Content:      "Test note content",
				}
				mockStore.On("GetObject", mock.Anything, "https://test.example.com/objects/123").Return(mockNote, nil)
				mockStore.On("CreateBookmark", mock.Anything, "testuser", "https://test.example.com/objects/123").Return(errors.New("database error"))
			},
			expectedStatus: 500,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mock
			mockStore = new(MockStorageAdapter)
			tt.setupMocks()

			// Create handler
			cfg := &config.Config{Domain: "test.example.com"}
			handler := NewHandler(cfg, mockStore, zap.NewNop(), nil)

			// Setup context
			req := &lift.Request{
				Request: &adapters.Request{
					Method: "POST",
					Path:   "/api/v1/statuses/" + tt.statusID + "/bookmark",
					Headers: map[string]string{
						"X-Test-Username": "testuser",
					},
				},
			}

			ctx := lift.NewContext(context.Background(), req)
			ctx.SetParam("id", tt.statusID)

			// Execute handler
			err := handler.HandleBookmarkLift(ctx)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)

				if tt.checkResponse != nil {
					tt.checkResponse(t, ctx)
				}
			}

			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandleUnbookmarkLift(t *testing.T) {
	var mockStore *MockStorageAdapter

	tests := []struct {
		name           string
		statusID       string
		setupMocks     func()
		expectedStatus int
		expectError    bool
	}{
		{
			name:     "successful unbookmark",
			statusID: "123",
			setupMocks: func() {
				publishedTime := time.Now().Add(-1 * time.Hour)
				mockNote := &activitypub.Note{
					BaseObject: activitypub.BaseObject{
						ID:        "https://test.example.com/objects/123",
						Type:      "Note",
						Published: &publishedTime,
					},
					AttributedTo: "https://test.example.com/users/author",
					Content:      "Test note content",
				}
				mockStore.On("GetObject", mock.Anything, "https://test.example.com/objects/123").Return(mockNote, nil)
				mockStore.On("RemoveBookmark", mock.Anything, "testuser", "https://test.example.com/objects/123").Return(nil)

				mockAuthor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   "https://test.example.com/users/author",
						Type: "Person",
					},
					PreferredUsername: "author",
					Name:              "Author Name",
				}
				mockStore.On("GetActor", mock.Anything, "author").Return(mockAuthor, nil)

				// Mock counts and interactions
				mockStore.On("CountObjectLikes", mock.Anything, "https://test.example.com/objects/123").Return(5, nil)
				mockStore.On("CountObjectAnnounces", mock.Anything, "https://test.example.com/objects/123").Return(2, nil)
				mockStore.On("GetLike", mock.Anything, "https://test.example.com/users/testuser", "https://test.example.com/objects/123").Return(nil, errors.New("not found"))
				mockStore.On("GetAnnounce", mock.Anything, "https://test.example.com/users/testuser", "https://test.example.com/objects/123").Return(nil, errors.New("not found"))
			},
			expectedStatus: 200,
			expectError:    false,
		},
		{
			name:     "unbookmark status not found",
			statusID: "nonexistent",
			setupMocks: func() {
				mockStore.On("GetObject", mock.Anything, "https://test.example.com/objects/nonexistent").Return(nil, errors.New("not found"))
			},
			expectedStatus: 404,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mock
			mockStore = new(MockStorageAdapter)
			tt.setupMocks()

			// Create handler
			cfg := &config.Config{Domain: "test.example.com"}
			handler := NewHandler(cfg, mockStore, zap.NewNop(), nil)

			// Setup context
			req := &lift.Request{
				Request: &adapters.Request{
					Method: "POST",
					Path:   "/api/v1/statuses/" + tt.statusID + "/unbookmark",
					Headers: map[string]string{
						"X-Test-Username": "testuser",
					},
				},
			}

			ctx := lift.NewContext(context.Background(), req)
			ctx.SetParam("id", tt.statusID)

			// Execute handler
			err := handler.HandleUnbookmarkLift(ctx)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
			}

			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandleGetBookmarksLift(t *testing.T) {
	var mockStore *MockStorageAdapter

	tests := []struct {
		name           string
		queryParams    map[string]string
		setupMocks     func()
		expectedStatus int
		expectError    bool
		checkResponse  func(t *testing.T, ctx *lift.Context)
	}{
		{
			name: "successful bookmarks retrieval with pagination",
			queryParams: map[string]string{
				"limit":  "10",
				"max_id": "cursor123",
			},
			setupMocks: func() {
				// Mock bookmarks retrieval
				objectIDs := []string{
					"https://test.example.com/objects/123",
					"https://remote.example.com/notes/456",
				}
				mockStore.On("GetBookmarks", mock.Anything, "testuser", 10, "cursor123").Return(objectIDs, "nextcursor456", nil)

				// Mock objects
				publishedTime1 := time.Now().Add(-1 * time.Hour)
				mockNote1 := &activitypub.Note{
					BaseObject: activitypub.BaseObject{
						ID:        "https://test.example.com/objects/123",
						Type:      "Note",
						Published: &publishedTime1,
					},
					AttributedTo: "https://test.example.com/users/author1",
					Content:      "First bookmarked note",
				}
				mockStore.On("GetObject", mock.Anything, "https://test.example.com/objects/123").Return(mockNote1, nil)

				publishedTime2 := time.Now().Add(-2 * time.Hour)
				mockNote2 := &activitypub.Note{
					BaseObject: activitypub.BaseObject{
						ID:        "https://remote.example.com/notes/456",
						Type:      "Note",
						Published: &publishedTime2,
					},
					AttributedTo: "https://remote.example.com/users/author2",
					Content:      "Second bookmarked note",
				}
				mockStore.On("GetObject", mock.Anything, "https://remote.example.com/notes/456").Return(mockNote2, nil)

				// Mock actors
				mockAuthor1 := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   "https://test.example.com/users/author1",
						Type: "Person",
					},
					PreferredUsername: "author1",
					Name:              "Author One",
				}
				mockStore.On("GetActor", mock.Anything, "author1").Return(mockAuthor1, nil)

				mockAuthor2 := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   "https://remote.example.com/users/author2",
						Type: "Person",
					},
					PreferredUsername: "author2",
					Name:              "Author Two",
				}
				mockStore.On("GetActor", mock.Anything, "author2").Return(mockAuthor2, nil)

				// Mock counts and interactions for both objects
				for _, objectID := range objectIDs {
					mockStore.On("CountObjectLikes", mock.Anything, objectID).Return(0, nil)
					mockStore.On("CountObjectAnnounces", mock.Anything, objectID).Return(0, nil)
					mockStore.On("GetLike", mock.Anything, "https://test.example.com/users/testuser", objectID).Return(nil, errors.New("not found"))
					mockStore.On("GetAnnounce", mock.Anything, "https://test.example.com/users/testuser", objectID).Return(nil, errors.New("not found"))
				}
			},
			expectedStatus: 200,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				// Should have Link header for pagination
				linkHeader := ctx.Response.Headers["Link"]
				assert.Contains(t, linkHeader, "next")
				assert.Contains(t, linkHeader, "nextcursor456")

				// Response should be array of statuses
				statuses, ok := ctx.Response.Body.([]*models.Status)
				assert.True(t, ok, "Response should be an array of Status objects")
				assert.Len(t, statuses, 2, "Should have 2 statuses")
				
				// Check that both statuses are bookmarked
				for _, status := range statuses {
					assert.True(t, status.Bookmarked, "All statuses should be bookmarked")
				}
			},
		},
		{
			name:        "empty bookmarks list",
			queryParams: map[string]string{},
			setupMocks: func() {
				mockStore.On("GetBookmarks", mock.Anything, "testuser", 20, "").Return([]string{}, "", nil)
			},
			expectedStatus: 200,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				// Should return empty array
				statuses, ok := ctx.Response.Body.([]*models.Status)
				assert.True(t, ok, "Response should be an array of Status objects")
				assert.Len(t, statuses, 0, "Should have empty array")
			},
		},
		{
			name:        "skip failed object retrieval",
			queryParams: map[string]string{},
			setupMocks: func() {
				// Return two object IDs, but first one fails to load
				objectIDs := []string{
					"https://test.example.com/objects/deleted",
					"https://test.example.com/objects/123",
				}
				mockStore.On("GetBookmarks", mock.Anything, "testuser", 20, "").Return(objectIDs, "", nil)

				// First object fails to load
				mockStore.On("GetObject", mock.Anything, "https://test.example.com/objects/deleted").Return(nil, errors.New("not found"))

				// Second object loads successfully
				publishedTime := time.Now().Add(-1 * time.Hour)
				mockNote := &activitypub.Note{
					BaseObject: activitypub.BaseObject{
						ID:        "https://test.example.com/objects/123",
						Type:      "Note",
						Published: &publishedTime,
					},
					AttributedTo: "https://test.example.com/users/author",
					Content:      "Valid bookmarked note",
				}
				mockStore.On("GetObject", mock.Anything, "https://test.example.com/objects/123").Return(mockNote, nil)

				mockAuthor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   "https://test.example.com/users/author",
						Type: "Person",
					},
					PreferredUsername: "author",
					Name:              "Author Name",
				}
				mockStore.On("GetActor", mock.Anything, "author").Return(mockAuthor, nil)

				// Mock counts and interactions
				mockStore.On("CountObjectLikes", mock.Anything, "https://test.example.com/objects/123").Return(0, nil)
				mockStore.On("CountObjectAnnounces", mock.Anything, "https://test.example.com/objects/123").Return(0, nil)
				mockStore.On("GetLike", mock.Anything, "https://test.example.com/users/testuser", "https://test.example.com/objects/123").Return(nil, errors.New("not found"))
				mockStore.On("GetAnnounce", mock.Anything, "https://test.example.com/users/testuser", "https://test.example.com/objects/123").Return(nil, errors.New("not found"))
			},
			expectedStatus: 200,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				// Should only contain the valid note (skipped the deleted one)
				statuses, ok := ctx.Response.Body.([]*models.Status)
				assert.True(t, ok, "Response should be an array of Status objects")
				assert.Len(t, statuses, 1, "Should have 1 status (deleted one was skipped)")
				assert.True(t, statuses[0].Bookmarked, "Status should be bookmarked")
				assert.Equal(t, "Valid bookmarked note", statuses[0].Content)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mock
			mockStore = new(MockStorageAdapter)
			tt.setupMocks()

			// Create handler
			cfg := &config.Config{Domain: "test.example.com"}
			handler := NewHandler(cfg, mockStore, zap.NewNop(), nil)

			// Setup context
			req := &lift.Request{
				Request: &adapters.Request{
					Method: "GET",
					Path:   "/api/v1/bookmarks",
					Headers: map[string]string{
						"X-Test-Username": "testuser",
					},
					QueryParams: tt.queryParams,
				},
			}

			ctx := lift.NewContext(context.Background(), req)

			// Execute handler
			err := handler.HandleGetBookmarksLift(ctx)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)

				if tt.checkResponse != nil {
					tt.checkResponse(t, ctx)
				}
			}

			mockStore.AssertExpectations(t)
		})
	}
}

func TestConvertBookmarkedObjectToStatus(t *testing.T) {
	var mockStore *MockStorageAdapter

	tests := []struct {
		name        string
		obj         any
		objectID    string
		setupMocks  func()
		expectError bool
		errorMsg    string
	}{
		{
			name:     "convert Note object",
			objectID: "https://test.example.com/objects/123",
			obj: &activitypub.Note{
				BaseObject: activitypub.BaseObject{
					ID:   "https://test.example.com/objects/123",
					Type: "Note",
				},
				AttributedTo: "https://test.example.com/users/author",
				Content:      "Test note content",
			},
			setupMocks: func() {
				mockAuthor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   "https://test.example.com/users/author",
						Type: "Person",
					},
					PreferredUsername: "author",
					Name:              "Author Name",
				}
				mockStore.On("GetActor", mock.Anything, "author").Return(mockAuthor, nil)
				mockStore.On("CountObjectLikes", mock.Anything, "https://test.example.com/objects/123").Return(5, nil)
				mockStore.On("CountObjectAnnounces", mock.Anything, "https://test.example.com/objects/123").Return(2, nil)
				mockStore.On("GetLike", mock.Anything, "https://test.example.com/users/testuser", "https://test.example.com/objects/123").Return(nil, errors.New("not found"))
				mockStore.On("GetAnnounce", mock.Anything, "https://test.example.com/users/testuser", "https://test.example.com/objects/123").Return(nil, errors.New("not found"))
			},
			expectError: false,
		},
		{
			name:     "convert object with map format",
			objectID: "https://test.example.com/objects/456",
			obj: map[string]any{
				"id":           "https://test.example.com/objects/456",
				"type":         "Note",
				"attributedTo": "https://test.example.com/users/mapauthor",
				"content":      "Map object content",
			},
			setupMocks: func() {
				mockAuthor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   "https://test.example.com/users/mapauthor",
						Type: "Person",
					},
					PreferredUsername: "mapauthor",
					Name:              "Map Author",
				}
				mockStore.On("GetActor", mock.Anything, "mapauthor").Return(mockAuthor, nil)
				mockStore.On("CountObjectLikes", mock.Anything, "https://test.example.com/objects/456").Return(0, nil)
				mockStore.On("CountObjectAnnounces", mock.Anything, "https://test.example.com/objects/456").Return(0, nil)
				mockStore.On("GetLike", mock.Anything, "https://test.example.com/users/testuser", "https://test.example.com/objects/456").Return(nil, errors.New("not found"))
				mockStore.On("GetAnnounce", mock.Anything, "https://test.example.com/users/testuser", "https://test.example.com/objects/456").Return(nil, errors.New("not found"))
			},
			expectError: false,
		},
		{
			name:     "object without attributedTo",
			objectID: "https://test.example.com/objects/invalid",
			obj: map[string]any{
				"id":   "https://test.example.com/objects/invalid",
				"type": "Note",
				// missing attributedTo
			},
			setupMocks:  func() {},
			expectError: true,
			errorMsg:    "object has no attributedTo field",
		},
		{
			name:     "actor not found",
			objectID: "https://test.example.com/objects/orphan",
			obj: &activitypub.Note{
				BaseObject: activitypub.BaseObject{
					ID:   "https://test.example.com/objects/orphan",
					Type: "Note",
				},
				AttributedTo: "https://test.example.com/users/nonexistent",
				Content:      "Orphaned note",
			},
			setupMocks: func() {
				mockStore.On("GetActor", mock.Anything, "nonexistent").Return(nil, errors.New("actor not found"))
			},
			expectError: true,
			errorMsg:    "failed to get actor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mock
			mockStore = new(MockStorageAdapter)
			tt.setupMocks()

			// Create handler
			cfg := &config.Config{Domain: "test.example.com"}
			handler := NewHandler(cfg, mockStore, zap.NewNop(), nil)

			// Execute conversion
			status, err := handler.convertBookmarkedObjectToStatus(context.Background(), tt.obj, tt.objectID, "testuser", true)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
				assert.Nil(t, status)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, status)
				assert.Equal(t, true, status.Bookmarked) // Should be bookmarked
			}

			mockStore.AssertExpectations(t)
		})
	}
}