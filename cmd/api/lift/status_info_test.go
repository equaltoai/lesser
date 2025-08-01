package lift

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

func TestHandleGetStatusSourceLift(t *testing.T) {
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
			name:     "successful source retrieval with Note object",
			statusID: "123",
			setupMocks: func() {
				publishedTime := time.Now().Add(-1 * time.Hour)
				mockNote := &activitypub.Note{
					BaseObject: activitypub.BaseObject{
						ID:        "https://test.example.com/objects/123",
						Type:      "Note",
						Published: &publishedTime,
						Summary:   "This is a spoiler text",
					},
					AttributedTo: "https://test.example.com/users/author",
					Content:      "This is the original content",
				}
				mockStore.On("GetObject", mock.Anything, "https://test.example.com/objects/123").Return(mockNote, nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				var source models.StatusSource
				bodyBytes, err := json.Marshal(ctx.Response.Body)
				assert.NoError(t, err)
				err = json.Unmarshal(bodyBytes, &source)
				assert.NoError(t, err)
				assert.Equal(t, "123", source.ID)
				assert.Equal(t, "This is the original content", source.Text)
				assert.Equal(t, "This is a spoiler text", source.SpoilerText)
			},
		},
		{
			name:     "successful source retrieval with map object",
			statusID: "456",
			setupMocks: func() {
				mockObj := map[string]any{
					"id":      "https://test.example.com/objects/456",
					"type":    "Note",
					"content": "Map object content",
					"summary": "Map spoiler text",
				}
				mockStore.On("GetObject", mock.Anything, "https://test.example.com/objects/456").Return(mockObj, nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				var source models.StatusSource
				bodyBytes, err := json.Marshal(ctx.Response.Body)
				assert.NoError(t, err)
				err = json.Unmarshal(bodyBytes, &source)
				assert.NoError(t, err)
				assert.Equal(t, "456", source.ID)
				assert.Equal(t, "Map object content", source.Text)
				assert.Equal(t, "Map spoiler text", source.SpoilerText)
			},
		},
		{
			name:     "successful source retrieval with struct using reflection",
			statusID: "789",
			setupMocks: func() {
				// Create a custom struct that has Content and Summary fields
				customStruct := struct {
					Content string
					Summary string
				}{
					Content: "Reflection content",
					Summary: "Reflection summary",
				}
				mockStore.On("GetObject", mock.Anything, "https://test.example.com/objects/789").Return(customStruct, nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				var source models.StatusSource
				bodyBytes, err := json.Marshal(ctx.Response.Body)
				assert.NoError(t, err)
				err = json.Unmarshal(bodyBytes, &source)
				assert.NoError(t, err)
				assert.Equal(t, "789", source.ID)
				assert.Equal(t, "Reflection content", source.Text)
				assert.Equal(t, "Reflection summary", source.SpoilerText)
			},
		},
		{
			name:     "test mode with X-Test-Username header",
			statusID: "test123",
			setupMocks: func() {
				publishedTime := time.Now().Add(-1 * time.Hour)
				mockNote := &activitypub.Note{
					BaseObject: activitypub.BaseObject{
						ID:        "https://test.example.com/objects/test123",
						Type:      "Note",
						Published: &publishedTime,
					},
					AttributedTo: "https://test.example.com/users/testuser",
					Content:      "Test content",
				}
				mockStore.On("GetObject", mock.Anything, "https://test.example.com/objects/test123").Return(mockNote, nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				var source models.StatusSource
				bodyBytes, err := json.Marshal(ctx.Response.Body)
				assert.NoError(t, err)
				err = json.Unmarshal(bodyBytes, &source)
				assert.NoError(t, err)
				assert.Equal(t, "test123", source.ID)
				assert.Equal(t, "Test content", source.Text)
				assert.Empty(t, source.SpoilerText)
			},
		},
		{
			name:     "status not found",
			statusID: "404",
			setupMocks: func() {
				mockStore.On("GetObject", mock.Anything, "https://test.example.com/objects/404").Return(nil, storage.ErrNotFound)
			},
			expectedStatus: http.StatusNotFound,
			expectError:    false,
		},
		{
			name:     "missing status ID parameter",
			statusID: "",
			setupMocks: func() {
				// No mocks needed - error occurs before any storage calls
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    false,
		},
		{
			name:     "unexpected object type error",
			statusID: "invalid",
			setupMocks: func() {
				// Return an object that doesn't match any known types
				invalidObj := 42 // An integer, which shouldn't match any patterns
				mockStore.On("GetObject", mock.Anything, "https://test.example.com/objects/invalid").Return(invalidObj, nil)
			},
			expectedStatus: http.StatusInternalServerError,
			expectError:    false,
		},
		{
			name:     "full URL status ID handling",
			statusID: "https://remote.example.com/objects/remote123",
			setupMocks: func() {
				publishedTime := time.Now().Add(-1 * time.Hour)
				mockNote := &activitypub.Note{
					BaseObject: activitypub.BaseObject{
						ID:        "https://remote.example.com/objects/remote123",
						Type:      "Note",
						Published: &publishedTime,
					},
					AttributedTo: "https://remote.example.com/users/remoteuser",
					Content:      "Remote content",
				}
				mockStore.On("GetObject", mock.Anything, "https://remote.example.com/objects/remote123").Return(mockNote, nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				var source models.StatusSource
				bodyBytes, err := json.Marshal(ctx.Response.Body)
				assert.NoError(t, err)
				err = json.Unmarshal(bodyBytes, &source)
				assert.NoError(t, err)
				assert.Equal(t, "https://remote.example.com/objects/remote123", source.ID)
				assert.Equal(t, "Remote content", source.Text)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mocks
			mockStore = new(MockStorageAdapter)
			tt.setupMocks()

			// Create handler
			handler := &Handler{
				cfg: &config.Config{
					JWTSecret: "test-secret",
					Domain:    "test.example.com",
				},
				store:          mockStore,
				logger:         zap.NewNop(),
				authMiddleware: &auth.Middleware{},
			}

			// Setup context
			req := &lift.Request{
				Request: &adapters.Request{
					Method: "GET",
					Path:   "/api/v1/statuses/" + tt.statusID + "/source",
					Headers: map[string]string{
						"X-Test-Username": "testuser",
					},
					QueryParams: map[string]string{},
				},
			}

			ctx := lift.NewContext(context.Background(), req)
			ctx.SetParam("id", tt.statusID)

			// Call handler directly
			err := handler.HandleGetStatusSourceLift(ctx)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Check status
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)

			// Run additional response checks if provided
			if tt.checkResponse != nil {
				tt.checkResponse(t, ctx)
			}

			// Verify all mocks were called
			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandleGetStatusHistoryLift(t *testing.T) {
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
			name:     "successful history retrieval with edits",
			statusID: "123",
			setupMocks: func() {
				publishedTime := time.Now().Add(-2 * time.Hour)
				updatedTime := time.Now().Add(-1 * time.Hour)
				
				// Mock current object
				mockNote := &activitypub.Note{
					BaseObject: activitypub.BaseObject{
						ID:        "https://test.example.com/objects/123",
						Type:      "Note",
						Published: &publishedTime,
						Updated:   &updatedTime,
						Summary:   "Updated spoiler",
						Sensitive: true,
					},
					AttributedTo: "https://test.example.com/users/author",
					Content:      "Updated content",
				}
				mockStore.On("GetObject", mock.Anything, "https://test.example.com/objects/123").Return(mockNote, nil)

				// Mock actor lookup
				mockActor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   "https://test.example.com/users/author",
						Type: "Person",
					},
					PreferredUsername: "author",
					Name:              "Test Author",
				}
				mockStore.On("GetActor", mock.Anything, "author").Return(mockActor, nil)

				// Mock update history
				histories := []*storage.UpdateHistory{
					{
						ObjectID:      "https://test.example.com/objects/123",
						Version:       1,
						UpdatedAt:     publishedTime,
						UpdatedBy:     "https://test.example.com/users/author",
						PreviousState: `{"content":"Original content","summary":"Original spoiler","sensitive":false}`,
						Summary:       "First edit",
					},
				}
				mockStore.On("GetUpdateHistory", mock.Anything, "https://test.example.com/objects/123", 100).Return(histories, nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				var edits []models.StatusEdit
				bodyBytes, err := json.Marshal(ctx.Response.Body)
				assert.NoError(t, err)
				err = json.Unmarshal(bodyBytes, &edits)
				assert.NoError(t, err)
				assert.Len(t, edits, 2) // Current version + 1 historical version

				// Check current version (first in array)
				currentEdit := edits[0]
				assert.Equal(t, "Updated content", currentEdit.Content)
				assert.Equal(t, "Updated spoiler", currentEdit.SpoilerText)
				assert.True(t, currentEdit.Sensitive)
				assert.Equal(t, "author", currentEdit.Account.Username)

				// Check historical version
				historicalEdit := edits[1]
				assert.Equal(t, "Original content", historicalEdit.Content)
				assert.Equal(t, "Original spoiler", historicalEdit.SpoilerText)
				assert.False(t, historicalEdit.Sensitive)
			},
		},
		{
			name:     "successful history retrieval with map object",
			statusID: "456",
			setupMocks: func() {
				// Mock current object as map
				mockObj := map[string]any{
					"id":          "https://test.example.com/objects/456",
					"type":        "Note",
					"content":     "Map content",
					"summary":     "Map summary",
					"sensitive":   false,
					"attributedTo": "https://test.example.com/users/mapuser",
					"published":   "2023-01-01T12:00:00Z",
				}
				mockStore.On("GetObject", mock.Anything, "https://test.example.com/objects/456").Return(mockObj, nil)

				// Mock actor lookup
				mockActor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   "https://test.example.com/users/mapuser",
						Type: "Person",
					},
					PreferredUsername: "mapuser",
					Name:              "Map User",
				}
				mockStore.On("GetActor", mock.Anything, "mapuser").Return(mockActor, nil)

				// Mock empty update history
				mockStore.On("GetUpdateHistory", mock.Anything, "https://test.example.com/objects/456", 100).Return([]*storage.UpdateHistory{}, nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				var edits []models.StatusEdit
				bodyBytes, err := json.Marshal(ctx.Response.Body)
				assert.NoError(t, err)
				err = json.Unmarshal(bodyBytes, &edits)
				assert.NoError(t, err)
				assert.Len(t, edits, 1) // Only current version

				edit := edits[0]
				assert.Equal(t, "Map content", edit.Content)
				assert.Equal(t, "Map summary", edit.SpoilerText)
				assert.False(t, edit.Sensitive)
				assert.Equal(t, "2023-01-01T12:00:00Z", edit.CreatedAt)
			},
		},
		{
			name:     "history with unknown actor",
			statusID: "789",
			setupMocks: func() {
				publishedTime := time.Now().Add(-1 * time.Hour)
				
				// Mock current object without attributedTo
				mockNote := &activitypub.Note{
					BaseObject: activitypub.BaseObject{
						ID:        "https://test.example.com/objects/789",
						Type:      "Note",
						Published: &publishedTime,
					},
					Content: "Orphaned content",
				}
				mockStore.On("GetObject", mock.Anything, "https://test.example.com/objects/789").Return(mockNote, nil)

				// Mock empty update history
				mockStore.On("GetUpdateHistory", mock.Anything, "https://test.example.com/objects/789", 100).Return([]*storage.UpdateHistory{}, nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				var edits []models.StatusEdit
				bodyBytes, err := json.Marshal(ctx.Response.Body)
				assert.NoError(t, err)
				err = json.Unmarshal(bodyBytes, &edits)
				assert.NoError(t, err)
				assert.Len(t, edits, 1)

				edit := edits[0]
				assert.Equal(t, "Orphaned content", edit.Content)
				assert.Equal(t, "unknown", edit.Account.ID)
				assert.Equal(t, "unknown", edit.Account.Username)
			},
		},
		{
			name:     "test mode with authentication headers",
			statusID: "auth123",
			setupMocks: func() {
				publishedTime := time.Now().Add(-1 * time.Hour)
				
				mockNote := &activitypub.Note{
					BaseObject: activitypub.BaseObject{
						ID:        "https://test.example.com/objects/auth123",
						Type:      "Note",
						Published: &publishedTime,
					},
					AttributedTo: "https://test.example.com/users/authuser",
					Content:      "Authenticated content",
				}
				mockStore.On("GetObject", mock.Anything, "https://test.example.com/objects/auth123").Return(mockNote, nil)

				mockActor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   "https://test.example.com/users/authuser",
						Type: "Person",
					},
					PreferredUsername: "authuser",
					Name:              "Auth User",
				}
				mockStore.On("GetActor", mock.Anything, "authuser").Return(mockActor, nil)
				mockStore.On("GetUpdateHistory", mock.Anything, "https://test.example.com/objects/auth123", 100).Return([]*storage.UpdateHistory{}, nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:     "status not found",
			statusID: "404",
			setupMocks: func() {
				mockStore.On("GetObject", mock.Anything, "https://test.example.com/objects/404").Return(nil, storage.ErrNotFound)
			},
			expectedStatus: http.StatusNotFound,
			expectError:    false,
		},
		{
			name:     "missing status ID parameter",
			statusID: "",
			setupMocks: func() {
				// No mocks needed - error occurs before any storage calls
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    false,
		},
		{
			name:     "update history error returns empty list",
			statusID: "error123",
			setupMocks: func() {
				publishedTime := time.Now().Add(-1 * time.Hour)
				
				mockNote := &activitypub.Note{
					BaseObject: activitypub.BaseObject{
						ID:        "https://test.example.com/objects/error123",
						Type:      "Note",
						Published: &publishedTime,
					},
					AttributedTo: "https://test.example.com/users/erroruser",
					Content:      "Error content",
				}
				mockStore.On("GetObject", mock.Anything, "https://test.example.com/objects/error123").Return(mockNote, nil)

				mockActor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   "https://test.example.com/users/erroruser",
						Type: "Person",
					},
					PreferredUsername: "erroruser",
					Name:              "Error User",
				}
				mockStore.On("GetActor", mock.Anything, "erroruser").Return(mockActor, nil)

				// Mock history error
				mockStore.On("GetUpdateHistory", mock.Anything, "https://test.example.com/objects/error123", 100).Return(nil, assert.AnError)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				var edits []models.StatusEdit
				bodyBytes, err := json.Marshal(ctx.Response.Body)
				assert.NoError(t, err)
				err = json.Unmarshal(bodyBytes, &edits)
				assert.NoError(t, err)
				assert.Len(t, edits, 0) // Empty array when history fails
			},
		},
		{
			name:     "full URL status ID handling",
			statusID: "https://remote.example.com/objects/remote456",
			setupMocks: func() {
				publishedTime := time.Now().Add(-1 * time.Hour)
				
				mockNote := &activitypub.Note{
					BaseObject: activitypub.BaseObject{
						ID:        "https://remote.example.com/objects/remote456",
						Type:      "Note",
						Published: &publishedTime,
					},
					AttributedTo: "https://remote.example.com/users/remoteuser",
					Content:      "Remote history content",
				}
				mockStore.On("GetObject", mock.Anything, "https://remote.example.com/objects/remote456").Return(mockNote, nil)

				mockActor := &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   "https://remote.example.com/users/remoteuser",
						Type: "Person",
					},
					PreferredUsername: "remoteuser",
					Name:              "Remote User",
				}
				mockStore.On("GetActor", mock.Anything, "remoteuser").Return(mockActor, nil)
				mockStore.On("GetUpdateHistory", mock.Anything, "https://remote.example.com/objects/remote456", 100).Return([]*storage.UpdateHistory{}, nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mocks
			mockStore = new(MockStorageAdapter)
			tt.setupMocks()

			// Create handler
			handler := &Handler{
				cfg: &config.Config{
					JWTSecret: "test-secret",
					Domain:    "test.example.com",
				},
				store:          mockStore,
				logger:         zap.NewNop(),
				authMiddleware: &auth.Middleware{},
			}

			// Setup context
			req := &lift.Request{
				Request: &adapters.Request{
					Method: "GET",
					Path:   "/api/v1/statuses/" + tt.statusID + "/history",
					Headers: map[string]string{
						"X-Test-Username": "testuser",
						"Authorization":   "Bearer test-token",
					},
					QueryParams: map[string]string{},
				},
			}

			ctx := lift.NewContext(context.Background(), req)
			ctx.SetParam("id", tt.statusID)

			// Call handler directly
			err := handler.HandleGetStatusHistoryLift(ctx)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Check status
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)

			// Run additional response checks if provided
			if tt.checkResponse != nil {
				tt.checkResponse(t, ctx)
			}

			// Verify all mocks were called
			mockStore.AssertExpectations(t)
		})
	}
}

// TestStatusInfoHandlerEdgeCases tests edge cases and error conditions
func TestStatusInfoHandlerEdgeCases(t *testing.T) {
	t.Run("status source with empty content fields", func(t *testing.T) {
		mockStore := new(MockStorageAdapter)

		// Mock object with empty content
		mockNote := &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:   "https://test.example.com/objects/empty",
				Type: "Note",
			},
			Content: "",
		}
		mockStore.On("GetObject", mock.Anything, "https://test.example.com/objects/empty").Return(mockNote, nil)

		handler := &Handler{
			cfg: &config.Config{
				Domain: "test.example.com",
			},
			store:  mockStore,
			logger: zap.NewNop(),
		}

		req := &lift.Request{
			Request: &adapters.Request{
				Method: "GET",
				Path:   "/api/v1/statuses/empty/source",
			},
		}

		ctx := lift.NewContext(context.Background(), req)
		ctx.SetParam("id", "empty")

		err := handler.HandleGetStatusSourceLift(ctx)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, ctx.Response.StatusCode)

		var source models.StatusSource
		bodyBytes, err := json.Marshal(ctx.Response.Body)
		assert.NoError(t, err)
		err = json.Unmarshal(bodyBytes, &source)
		assert.NoError(t, err)
		assert.Equal(t, "empty", source.ID)
		assert.Empty(t, source.Text)
		assert.Empty(t, source.SpoilerText)

		mockStore.AssertExpectations(t)
	})

	t.Run("status history with malformed previous state JSON", func(t *testing.T) {
		mockStore := new(MockStorageAdapter)

		publishedTime := time.Now().Add(-1 * time.Hour)
		mockNote := &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:        "https://test.example.com/objects/malformed",
				Type:      "Note",
				Published: &publishedTime,
			},
			Content: "Current content",
		}
		mockStore.On("GetObject", mock.Anything, "https://test.example.com/objects/malformed").Return(mockNote, nil)

		// Mock history with malformed JSON
		histories := []*storage.UpdateHistory{
			{
				ObjectID:      "https://test.example.com/objects/malformed",
				Version:       1,
				UpdatedAt:     publishedTime,
				PreviousState: `{"content":malformed json}`, // Invalid JSON
			},
		}
		mockStore.On("GetUpdateHistory", mock.Anything, "https://test.example.com/objects/malformed", 100).Return(histories, nil)

		handler := &Handler{
			cfg: &config.Config{
				Domain: "test.example.com",
			},
			store:  mockStore,
			logger: zap.NewNop(),
		}

		req := &lift.Request{
			Request: &adapters.Request{
				Method: "GET",
				Path:   "/api/v1/statuses/malformed/history",
			},
		}

		ctx := lift.NewContext(context.Background(), req)
		ctx.SetParam("id", "malformed")

		err := handler.HandleGetStatusHistoryLift(ctx)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, ctx.Response.StatusCode)

		var edits []models.StatusEdit
		bodyBytes, err := json.Marshal(ctx.Response.Body)
		assert.NoError(t, err)
		err = json.Unmarshal(bodyBytes, &edits)
		assert.NoError(t, err)
		assert.Len(t, edits, 2) // Current + historical (with empty content due to malformed JSON)

		// Historical edit should have empty content due to JSON parse failure
		historicalEdit := edits[1]
		assert.Empty(t, historicalEdit.Content)

		mockStore.AssertExpectations(t)
	})
}