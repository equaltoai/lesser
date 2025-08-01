package lift

import (
	"context"
	"net/http"
	"testing"
	"time"

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

func TestHandleGetConversationsLift(t *testing.T) {
	var mockStore *MockStorageAdapter

	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func()
		expectedStatus int
		expectError    bool
	}{
		{
			name: "successful get conversations with test header",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/conversations",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				// Mock actor lookup
				mockStore.On("GetActor", mock.Anything, "testuser").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID: "https://test.example.com/users/testuser",
					},
					PreferredUsername: "testuser",
				}, nil)
				
				// Mock empty conversations
				mockStore.On("GetUserConversations", mock.Anything, "https://test.example.com/users/testuser", 20, "").Return(
					[]*storage.Conversation{},
					"",
					nil,
				)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "unauthorized - no token or test header",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/conversations",
						Headers: map[string]string{},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				// No mocks needed for unauthorized case
			},
			expectedStatus: http.StatusUnauthorized,
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
				authMiddleware: &auth.Middleware{},
			}
			
			// Get context
			ctx := tt.setupContext()
			
			// Call handler directly
			err := handler.HandleGetConversationsLift(ctx)
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			
			// Check status
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
			
			// Verify all expected calls were made
			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandleDeleteConversationLift(t *testing.T) {
	var mockStore *MockStorageAdapter

	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func()
		expectedStatus int
		expectError    bool
	}{
		{
			name: "successful delete conversation with test header",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "DELETE",
						Path:   "/api/v1/conversations/conv1",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						PathParams: map[string]string{"id": "conv1"},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("id", "conv1")
				return ctx
			},
			setupMocks: func() {
				// Mock actor lookup
				mockStore.On("GetActor", mock.Anything, "testuser").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID: "https://test.example.com/users/testuser",
					},
					PreferredUsername: "testuser",
				}, nil)
				
				// Mock conversation lookup
				mockStore.On("GetConversation", mock.Anything, "conv1").Return(&storage.Conversation{
					ID:           "conv1",
					Participants: []string{"https://test.example.com/users/testuser", "https://test.example.com/users/other"},
				}, nil)
				
				// Mock delete operation
				mockStore.On("DeleteConversation", mock.Anything, "conv1").Return(nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "not participant in conversation",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "DELETE",
						Path:   "/api/v1/conversations/conv1",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						PathParams: map[string]string{"id": "conv1"},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("id", "conv1")
				return ctx
			},
			setupMocks: func() {
				// Mock actor lookup
				mockStore.On("GetActor", mock.Anything, "testuser").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID: "https://test.example.com/users/testuser",
					},
					PreferredUsername: "testuser",
				}, nil)
				
				// Mock conversation lookup - user not in participants
				mockStore.On("GetConversation", mock.Anything, "conv1").Return(&storage.Conversation{
					ID:           "conv1",
					Participants: []string{"https://test.example.com/users/other1", "https://test.example.com/users/other2"},
				}, nil)
			},
			expectedStatus: http.StatusNotFound,
			expectError:    false,
		},
		{
			name: "conversation not found",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "DELETE",
						Path:   "/api/v1/conversations/nonexistent",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						PathParams: map[string]string{"id": "nonexistent"},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("id", "nonexistent")
				return ctx
			},
			setupMocks: func() {
				// Mock actor lookup
				mockStore.On("GetActor", mock.Anything, "testuser").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID: "https://test.example.com/users/testuser",
					},
					PreferredUsername: "testuser",
				}, nil)
				
				// Mock conversation not found
				mockStore.On("GetConversation", mock.Anything, "nonexistent").Return(nil, storage.ErrNotFound)
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
				authMiddleware: &auth.Middleware{},
			}
			
			// Get context
			ctx := tt.setupContext()
			
			// Call handler directly
			err := handler.HandleDeleteConversationLift(ctx)
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			
			// Check status
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
			
			// Verify all expected calls were made
			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandleMarkConversationReadLift(t *testing.T) {
	var mockStore *MockStorageAdapter

	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func()
		expectedStatus int
		expectError    bool
	}{
		{
			name: "successful mark as read",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/conversations/conv1/read",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						PathParams: map[string]string{"id": "conv1"},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("id", "conv1")
				return ctx
			},
			setupMocks: func() {
				// Mock actor lookup
				mockStore.On("GetActor", mock.Anything, "testuser").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID: "https://test.example.com/users/testuser",
					},
					PreferredUsername: "testuser",
				}, nil)
				
				// Mock conversation lookup
				conversation := &storage.Conversation{
					ID:           "conv1",
					Participants: []string{"https://test.example.com/users/testuser", "https://test.example.com/users/other"},
					LastStatusID: "status1",
				}
				mockStore.On("GetConversation", mock.Anything, "conv1").Return(conversation, nil)
				
				// Mock mark as read operation
				mockStore.On("MarkConversationRead", mock.Anything, "conv1", "testuser").Return(nil)
				
				// Mock participant actor lookup
				mockStore.On("GetActor", mock.Anything, "other").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID: "https://test.example.com/users/other",
					},
					PreferredUsername: "other",
				}, nil)
				
				// Mock last status
				mockStore.On("GetObject", mock.Anything, "status1").Return(&activitypub.Note{
					BaseObject: activitypub.BaseObject{
						ID:   "status1",
						Type: "Note",
					},
				}, nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "with test username header",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/conversations/conv1/read",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						PathParams: map[string]string{"id": "conv1"},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("id", "conv1")
				return ctx
			},
			setupMocks: func() {
				// Mock actor lookup
				mockStore.On("GetActor", mock.Anything, "testuser").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID: "https://test.example.com/users/testuser",
					},
					PreferredUsername: "testuser",
				}, nil)
				
				// Mock conversation lookup
				conversation := &storage.Conversation{
					ID:           "conv1",
					Participants: []string{"https://test.example.com/users/testuser", "https://test.example.com/users/other"},
					LastStatusID: "",
				}
				mockStore.On("GetConversation", mock.Anything, "conv1").Return(conversation, nil)
				
				// Mock mark as read operation
				mockStore.On("MarkConversationRead", mock.Anything, "conv1", "testuser").Return(nil)
				
				// Mock participant actor lookup
				mockStore.On("GetActor", mock.Anything, "other").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID: "https://test.example.com/users/other",
					},
					PreferredUsername: "other",
				}, nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "no auth header or test username",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/conversations/conv1/read",
						Headers: map[string]string{},
						PathParams: map[string]string{"id": "conv1"},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("id", "conv1")
				return ctx
			},
			setupMocks: func() {
				// No mocks needed for unauthorized case
			},
			expectedStatus: http.StatusUnauthorized,
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
				authMiddleware: &auth.Middleware{},
			}
			
			// Get context
			ctx := tt.setupContext()
			
			// Call handler directly
			err := handler.HandleMarkConversationReadLift(ctx)
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			
			// Check status
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
			
			// Verify all expected calls were made
			mockStore.AssertExpectations(t)
		})
	}
}

func TestIsConversationUnreadLift(t *testing.T) {
	var mockStore *MockStorageAdapter

	tests := []struct {
		name          string
		lastMessageAt *time.Time
		isMuted       bool
		setupMocks    func()
		expected      bool
	}{
		{
			name:          "no last message",
			lastMessageAt: nil,
			expected:      false,
			setupMocks:    func() {},
		},
		{
			name:          "muted conversation",
			lastMessageAt: func() *time.Time { t := time.Now(); return &t }(),
			isMuted:       true,
			expected:      false,
			setupMocks: func() {
				mockStore.On("IsConversationMuted", mock.Anything, "user1", "conv1").Return(true, nil)
			},
		},
		{
			name:          "recent message (unread)",
			lastMessageAt: func() *time.Time { t := time.Now().Add(-1 * time.Hour); return &t }(),
			isMuted:       false,
			expected:      true,
			setupMocks: func() {
				mockStore.On("IsConversationMuted", mock.Anything, "user1", "conv1").Return(false, nil)
			},
		},
		{
			name:          "old message (read)",
			lastMessageAt: func() *time.Time { t := time.Now().Add(-48 * time.Hour); return &t }(),
			isMuted:       false,
			expected:      false,
			setupMocks: func() {
				mockStore.On("IsConversationMuted", mock.Anything, "user1", "conv1").Return(false, nil)
			},
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
				authMiddleware: &auth.Middleware{},
			}
			
			req := &lift.Request{
				Request: &adapters.Request{
					Method: "GET",
					Path:   "/test",
				},
			}
			ctx := lift.NewContext(context.Background(), req)
			
			// Call the helper function
			result := handler.isConversationUnreadLift(ctx, "conv1", "user1", tt.lastMessageAt)
			
			assert.Equal(t, tt.expected, result)
			
			// Verify all expected calls were made
			mockStore.AssertExpectations(t)
		})
	}
}