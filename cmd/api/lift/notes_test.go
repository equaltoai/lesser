package lift

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/notes"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// setupReputationServiceMocks sets up the minimal mock expectations for reputation service
func setupReputationServiceMocks(mockStore *MockStorageAdapter, username string, actorID string, reputationScore int) {
	// Mock reputation check - return existing reputation that's not stale
	mockStore.On("GetReputation", mock.Anything, actorID).Return(&storage.Reputation{
		ActorID:      actorID,
		TotalScore:   reputationScore,
		CalculatedAt: time.Now(), // Fresh reputation, no recalculation needed
	}, nil)
}

func TestHandleCreateNoteLift(t *testing.T) {
	var mockStore *MockStorageAdapter

	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func()
		expectedStatus int
		expectError    bool
	}{
		{
			name: "successful note creation with test header",
			setupContext: func() *lift.Context {
				reqBody := `{
					"object_id": "https://example.com/posts/123",
					"object_type": "Note",
					"content": "This is a test community note",
					"language": "en",
					"sources": [{"url": "https://example.com/source1"}]
				}`
				
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/notes",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
							"Content-Type":    "application/json",
						},
					},
					Body: []byte(reqBody),
				}
				
				return lift.NewContext(context.Background(), req)
			},
			setupMocks: func() {
				// Set up all reputation service mocks
				setupReputationServiceMocks(mockStore, "testuser", "https://test.example.com/users/testuser", 150)
				
				// Mock rate limit check
				mockStore.On("CheckCommunityNoteRateLimit", mock.Anything, "https://test.example.com/users/testuser", 1).Return(true, 5, nil)
				
				// Mock note creation
				mockStore.On("CreateCommunityNote", mock.Anything, mock.AnythingOfType("*storage.CommunityNote")).Return(nil)
			},
			expectedStatus: http.StatusCreated,
			expectError:    false,
		},
		{
			name: "insufficient reputation",
			setupContext: func() *lift.Context {
				reqBody := `{
					"object_id": "https://example.com/posts/123",
					"object_type": "Note",
					"content": "This is a test community note",
					"language": "en",
					"sources": []
				}`
				
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/notes",
						Headers: map[string]string{
							"X-Test-Username": "newuser",
							"Content-Type":    "application/json",
						},
						Body: []byte(reqBody),
					},
				}
				
				return lift.NewContext(context.Background(), req)
			},
			setupMocks: func() {
				// Set up reputation service mocks with low reputation  
				setupReputationServiceMocks(mockStore, "newuser", "https://test.example.com/users/newuser", 50)
			},
			expectedStatus: http.StatusForbidden,
			expectError:    false,
		},
		{
			name: "rate limit exceeded",
			setupContext: func() *lift.Context {
				reqBody := `{
					"object_id": "https://example.com/posts/123",
					"object_type": "Note",
					"content": "This is a test community note",
					"language": "en",
					"sources": []
				}`
				
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/notes",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
							"Content-Type":    "application/json",
						},
					},
					Body: []byte(reqBody),
				}
				
				return lift.NewContext(context.Background(), req)
			},
			setupMocks: func() {
				// Set up reputation service mocks
				setupReputationServiceMocks(mockStore, "testuser", "https://test.example.com/users/testuser", 150)
				
				// Mock rate limit exceeded
				mockStore.On("CheckCommunityNoteRateLimit", mock.Anything, "https://test.example.com/users/testuser", 1).Return(false, 0, nil)
			},
			expectedStatus: http.StatusUnprocessableEntity,
			expectError:    false,
		},
		{
			name: "too many sources",
			setupContext: func() *lift.Context {
				sources := make([]map[string]string, notes.MaxSources+1)
				for i := range sources {
					sources[i] = map[string]string{"url": "https://example.com/source"}
				}
				sourcesJSON, _ := json.Marshal(sources)
				
				reqBody := `{
					"object_id": "https://example.com/posts/123",
					"object_type": "Note",
					"content": "This is a test community note",
					"language": "en",
					"sources": ` + string(sourcesJSON) + `
				}`
				
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/notes",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
							"Content-Type":    "application/json",
						},
					},
					Body: []byte(reqBody),
				}
				
				return lift.NewContext(context.Background(), req)
			},
			setupMocks: func() {
				// Set up reputation service mocks
				setupReputationServiceMocks(mockStore, "testuser", "https://test.example.com/users/testuser", 150)
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mocks
			mockStore = new(MockStorageAdapter)
			tt.setupMocks()
			
			handler := &Handler{
				cfg: &config.Config{
					JWTSecret: "test-secret",
					Domain:    "test.example.com",
				},
				store:  mockStore,
				logger: zap.NewNop(),
			}
			
			ctx := tt.setupContext()
			err := handler.HandleCreateNoteLift(ctx)
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
			
			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandleGetNotesLift(t *testing.T) {
	var mockStore *MockStorageAdapter

	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func()
		expectedStatus int
		expectError    bool
	}{
		{
			name: "successful notes retrieval",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/notes/object123",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						PathParams: map[string]string{"object_id": "object123"},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("object_id", "object123")
				
				return ctx
			},
			setupMocks: func() {
				mockNotes := []*storage.CommunityNote{
					{
						ID:               "note1",
						ObjectID:         "object123",
						ObjectType:       "Note",
						AuthorID:         "https://test.example.com/users/author1",
						Content:          "Test note content",
						Language:         "en",
						Sources:          []string{"https://example.com/source1"},
						Score:            0.7,
						VisibilityStatus: "visible",
						HelpfulVotes:     5,
						NotHelpfulVotes:  1,
						CreatedAt:        time.Now(),
						UpdatedAt:        time.Now(),
					},
				}
				
				mockStore.On("GetVisibleCommunityNotes", mock.Anything, "object123").Return(mockNotes, nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "missing object ID",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/notes/",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						PathParams: map[string]string{},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				// Don't set param to simulate missing object_id
				
				return ctx
			},
			setupMocks: func() {
				// No mocks needed for this test
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    false,
		},
		{
			name: "storage error",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/notes/object123",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						PathParams: map[string]string{"object_id": "object123"},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("object_id", "object123")
				
				return ctx
			},
			setupMocks: func() {
				mockStore.On("GetVisibleCommunityNotes", mock.Anything, "object123").Return(nil, assert.AnError)
			},
			expectedStatus: http.StatusInternalServerError,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mocks
			mockStore = new(MockStorageAdapter)
			tt.setupMocks()
			
			handler := &Handler{
				cfg: &config.Config{
					JWTSecret: "test-secret",
					Domain:    "test.example.com",
				},
				store:  mockStore,
				logger: zap.NewNop(),
			}
			
			ctx := tt.setupContext()
			err := handler.HandleGetNotesLift(ctx)
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
			
			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandleVoteNoteLift(t *testing.T) {
	var mockStore *MockStorageAdapter

	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func()
		expectedStatus int
		expectError    bool
	}{
		{
			name: "successful vote creation",
			setupContext: func() *lift.Context {
				reqBody := `{
					"vote_type": "helpful",
					"reason": "This note is accurate and well-sourced"
				}`
				
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/notes/note123/vote",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
							"Content-Type":    "application/json",
						},
						PathParams: map[string]string{"id": "note123"},
					},
					Body: []byte(reqBody),
				}
				
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("id", "note123")
				
				return ctx
			},
			setupMocks: func() {
				// Set up reputation service mocks
				setupReputationServiceMocks(mockStore, "testuser", "https://test.example.com/users/testuser", 150)
				
				// Mock note retrieval
				mockStore.On("GetCommunityNote", mock.Anything, "note123").Return(&storage.CommunityNote{
					ID:       "note123",
					AuthorID: "https://test.example.com/users/author1", // Different from voter
					Content:  "Test note",
				}, nil)
				
				// Mock vote creation
				mockStore.On("CreateCommunityNoteVote", mock.Anything, mock.AnythingOfType("*storage.CommunityNoteVote")).Return(nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "insufficient reputation to vote",
			setupContext: func() *lift.Context {
				reqBody := `{
					"vote_type": "helpful"
				}`
				
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/notes/note123/vote",
						Headers: map[string]string{
							"X-Test-Username": "newuser",
							"Content-Type":    "application/json",
						},
						PathParams: map[string]string{"id": "note123"},
					},
					Body: []byte(reqBody),
				}
				
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("id", "note123")
				
				return ctx
			},
			setupMocks: func() {
				// Set up reputation service mocks with low reputation
				setupReputationServiceMocks(mockStore, "newuser", "https://test.example.com/users/newuser", 5)
			},
			expectedStatus: http.StatusForbidden,
			expectError:    false,
		},
		{
			name: "cannot vote on own note",
			setupContext: func() *lift.Context {
				reqBody := `{
					"vote_type": "helpful"
				}`
				
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/notes/note123/vote",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
							"Content-Type":    "application/json",
						},
						PathParams: map[string]string{"id": "note123"},
					},
					Body: []byte(reqBody),
				}
				
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("id", "note123")
				
				return ctx
			},
			setupMocks: func() {
				// Set up reputation service mocks 
				setupReputationServiceMocks(mockStore, "testuser", "https://test.example.com/users/testuser", 150)
				
				// Mock note retrieval - same author as voter
				mockStore.On("GetCommunityNote", mock.Anything, "note123").Return(&storage.CommunityNote{
					ID:       "note123",
					AuthorID: "https://test.example.com/users/testuser", // Same as voter
					Content:  "Test note",
				}, nil)
			},
			expectedStatus: http.StatusForbidden,
			expectError:    false,
		},
		{
			name: "note not found",
			setupContext: func() *lift.Context {
				reqBody := `{
					"vote_type": "helpful"
				}`
				
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/notes/nonexistent/vote",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
							"Content-Type":    "application/json",
						},
						PathParams: map[string]string{"id": "nonexistent"},
					},
					Body: []byte(reqBody),
				}
				
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("id", "nonexistent")
				
				return ctx
			},
			setupMocks: func() {
				// Set up reputation service mocks
				setupReputationServiceMocks(mockStore, "testuser", "https://test.example.com/users/testuser", 150)
				
				// Mock note not found
				mockStore.On("GetCommunityNote", mock.Anything, "nonexistent").Return(nil, assert.AnError)
			},
			expectedStatus: http.StatusNotFound,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mocks
			mockStore = new(MockStorageAdapter)
			tt.setupMocks()
			
			handler := &Handler{
				cfg: &config.Config{
					JWTSecret: "test-secret",
					Domain:    "test.example.com",
				},
				store:  mockStore,
				logger: zap.NewNop(),
			}
			
			ctx := tt.setupContext()
			err := handler.HandleVoteNoteLift(ctx)
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
			
			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandleGetUserNotesLift(t *testing.T) {
	var mockStore *MockStorageAdapter

	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func()
		expectedStatus int
		expectError    bool
	}{
		{
			name: "successful user notes retrieval",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/accounts/testuser/notes",
						Headers: map[string]string{},
						PathParams: map[string]string{"id": "testuser"},
						QueryParams: map[string]string{"limit": "10"},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("id", "testuser")
				
				return ctx
			},
			setupMocks: func() {
				mockNotes := []*storage.CommunityNote{
					{
						ID:               "note1",
						ObjectID:         "object123",
						ObjectType:       "Note",
						AuthorID:         "https://test.example.com/users/testuser",
						Content:          "Test user note",
						Language:         "en",
						Sources:          []string{"https://example.com/source1"},
						Score:            0.7,
						VisibilityStatus: "visible",
						HelpfulVotes:     3,
						NotHelpfulVotes:  1,
						CreatedAt:        time.Now(),
						UpdatedAt:        time.Now(),
					},
				}
				
				mockStore.On("GetCommunityNotesByAuthor", mock.Anything, "https://test.example.com/users/testuser", 10, "").Return(mockNotes, "", nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "missing username",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/accounts//notes",
						Headers: map[string]string{},
						PathParams: map[string]string{},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				// Don't set param to simulate missing username
				
				return ctx
			},
			setupMocks: func() {
				// No mocks needed for this test
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    false,
		},
		{
			name: "storage error",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/accounts/testuser/notes",
						Headers: map[string]string{},
						PathParams: map[string]string{"id": "testuser"},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("id", "testuser")
				
				return ctx
			},
			setupMocks: func() {
				mockStore.On("GetCommunityNotesByAuthor", mock.Anything, "https://test.example.com/users/testuser", 20, "").Return(nil, "", assert.AnError)
			},
			expectedStatus: http.StatusInternalServerError,
			expectError:    false,
		},
		{
			name: "custom limit",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/accounts/testuser/notes",
						Headers: map[string]string{},
						PathParams: map[string]string{"id": "testuser"},
						QueryParams: map[string]string{"limit": "50"},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("id", "testuser")
				
				return ctx
			},
			setupMocks: func() {
				mockStore.On("GetCommunityNotesByAuthor", mock.Anything, "https://test.example.com/users/testuser", 50, "").Return([]*storage.CommunityNote{}, "", nil)
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
			
			handler := &Handler{
				cfg: &config.Config{
					JWTSecret: "test-secret",
					Domain:    "test.example.com",
				},
				store:  mockStore,
				logger: zap.NewNop(),
			}
			
			ctx := tt.setupContext()
			err := handler.HandleGetUserNotesLift(ctx)
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
			
			mockStore.AssertExpectations(t)
		})
	}
}

func TestCalculateNotesStats(t *testing.T) {
	tests := []struct {
		name     string
		notes    []*storage.CommunityNote
		expected map[string]any
	}{
		{
			name:  "empty notes",
			notes: []*storage.CommunityNote{},
			expected: map[string]any{
				"total":           0,
				"visible":         0,
				"average_score":   0,
				"average_helpful": 0,
			},
		},
		{
			name: "mixed visibility notes",
			notes: []*storage.CommunityNote{
				{
					ID:               "note1",
					Score:            0.8,
					HelpfulVotes:     5,
					NotHelpfulVotes:  1,
					VisibilityStatus: "visible",
				},
				{
					ID:               "note2",
					Score:            0.3,
					HelpfulVotes:     2,
					NotHelpfulVotes:  3,
					VisibilityStatus: "pending",
				},
				{
					ID:               "note3",
					Score:            0.9,
					HelpfulVotes:     8,
					NotHelpfulVotes:  0,
					VisibilityStatus: "visible",
				},
			},
			expected: map[string]any{
				"total":           3,
				"visible":         2,
				"average_score":   (0.8 + 0.3 + 0.9) / 3.0,
				"average_helpful": (5.0 + 2.0 + 8.0) / 3.0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateNotesStats(tt.notes)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGetNoteReputationService tests the helper method
func TestGetNoteReputationService(t *testing.T) {
	mockStore := new(MockStorageAdapter)
	
	handler := &Handler{
		cfg: &config.Config{
			JWTSecret:              "test-secret",
			Domain:                 "test.example.com",
			ReputationPrivateKey:   "test-private-key",
		},
		store:  mockStore,
		logger: zap.NewNop(),
	}
	
	service, err := handler.getNoteReputationService()
	assert.NoError(t, err)
	assert.NotNil(t, service)
}

