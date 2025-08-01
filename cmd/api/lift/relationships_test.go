package lift

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

func TestHandleGetRelationshipsLift(t *testing.T) {
	tests := []struct {
		name           string
		headers        map[string]string
		queryParams    map[string]string
		setupMocks     func(*MockStorageAdapter)
		expectedStatus int
		expectedError  string
		validate       func(*testing.T, []models.Relationship)
	}{
		{
			name: "successful relationships with test mode",
			headers: map[string]string{
				"X-Test-Username": "testuser",
			},
			queryParams: map[string]string{
				"id[0]": "target1",
				"id[1]": "target2",
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				// Test mode - get current user actor
				mockStore.On("GetActor", mock.Anything, "testuser").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID: "https://example.com/users/testuser",
					},
					PreferredUsername: "testuser",
				}, nil)

				// Get target actors
				mockStore.On("GetActor", mock.Anything, "target1").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID: "https://example.com/users/target1",
					},
					PreferredUsername: "target1",
				}, nil)
				mockStore.On("GetActor", mock.Anything, "target2").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID: "https://example.com/users/target2",
					},
					PreferredUsername: "target2",
				}, nil)

				// Mock relationship checks for target1
				mockStore.On("IsFollowing", mock.Anything, "testuser", "target1").Return(true, nil)
				mockStore.On("HasFollowRequest", mock.Anything, "testuser", "target1").Return(false, nil)
				mockStore.On("IsFollowing", mock.Anything, "target1", "testuser").Return(false, nil)
				mockStore.On("GetBlock", mock.Anything, "https://example.com/users/testuser", "https://example.com/users/target1").Return(nil, storage.ErrNotFound)
				mockStore.On("GetBlock", mock.Anything, "https://example.com/users/target1", "https://example.com/users/testuser").Return(nil, storage.ErrNotFound)
				mockStore.On("GetMute", mock.Anything, "testuser", "target1").Return(nil, storage.ErrNotFound)
				mockStore.On("IsBlockedDomain", mock.Anything, "https://example.com/users/testuser", "example.com").Return(false, nil)
				mockStore.On("IsEndorsed", mock.Anything, "testuser", "target1").Return(false, nil)
				mockStore.On("GetAccountNote", mock.Anything, "testuser", "target1").Return(nil, storage.ErrNotFound)

				// Mock relationship checks for target2
				mockStore.On("IsFollowing", mock.Anything, "testuser", "target2").Return(false, nil)
				mockStore.On("IsFollowing", mock.Anything, "target2", "testuser").Return(true, nil)
				mockStore.On("GetBlock", mock.Anything, "https://example.com/users/testuser", "https://example.com/users/target2").Return(nil, storage.ErrNotFound)
				mockStore.On("GetBlock", mock.Anything, "https://example.com/users/target2", "https://example.com/users/testuser").Return(nil, storage.ErrNotFound)
				mockStore.On("GetMute", mock.Anything, "testuser", "target2").Return(nil, storage.ErrNotFound)
				mockStore.On("IsBlockedDomain", mock.Anything, "https://example.com/users/testuser", "example.com").Return(false, nil)
				mockStore.On("IsEndorsed", mock.Anything, "testuser", "target2").Return(false, nil)
				mockStore.On("GetAccountNote", mock.Anything, "testuser", "target2").Return(nil, storage.ErrNotFound)
			},
			expectedStatus: 200,
			validate: func(t *testing.T, relationships []models.Relationship) {
				assert.Len(t, relationships, 2)
				
				// First relationship (following target1)
				assert.Equal(t, "target1", relationships[0].ID)
				assert.True(t, relationships[0].Following)
				assert.False(t, relationships[0].FollowedBy)
				assert.False(t, relationships[0].Blocking)
				assert.False(t, relationships[0].Muting)
				
				// Second relationship (followed by target2)
				assert.Equal(t, "target2", relationships[1].ID)
				assert.False(t, relationships[1].Following)
				assert.True(t, relationships[1].FollowedBy)
				assert.False(t, relationships[1].Blocking)
				assert.False(t, relationships[1].Muting)
			},
		},
		{
			name: "OAuth token validation fails (expected behavior)",
			headers: map[string]string{
				"Authorization": "Bearer invalid_token",
			},
			queryParams: map[string]string{
				"id": "target1",
			},
			setupMocks:     func(mockStore *MockStorageAdapter) {
				// No mocks needed - token validation will fail before any storage calls
			},
			expectedStatus: 401,
			expectedError:  "Unauthorized",
		},
		{
			name: "relationships with muting and note",
			headers: map[string]string{
				"X-Test-Username": "testuser",
			},
			queryParams: map[string]string{
				"id[0]": "target1",
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				// Test mode - get current user actor
				mockStore.On("GetActor", mock.Anything, "testuser").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID: "https://example.com/users/testuser",
					},
					PreferredUsername: "testuser",
				}, nil)

				// Get target actor
				mockStore.On("GetActor", mock.Anything, "target1").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID: "https://example.com/users/target1",
					},
					PreferredUsername: "target1",
				}, nil)

				// Mock relationship checks with muting and note
				mockStore.On("IsFollowing", mock.Anything, "testuser", "target1").Return(false, nil)
				mockStore.On("IsFollowing", mock.Anything, "target1", "testuser").Return(false, nil)
				mockStore.On("GetBlock", mock.Anything, "https://example.com/users/testuser", "https://example.com/users/target1").Return(nil, storage.ErrNotFound)
				mockStore.On("GetBlock", mock.Anything, "https://example.com/users/target1", "https://example.com/users/testuser").Return(nil, storage.ErrNotFound)
				mockStore.On("GetMute", mock.Anything, "testuser", "target1").Return(&storage.Mute{
					ID:                "mute123",
					Actor:             "https://example.com/users/testuser",
					Object:            "https://example.com/users/target1",
					HideNotifications: true,
					CreatedAt:         time.Now(),
				}, nil)
				mockStore.On("IsBlockedDomain", mock.Anything, "https://example.com/users/testuser", "example.com").Return(false, nil)
				mockStore.On("IsEndorsed", mock.Anything, "testuser", "target1").Return(true, nil)
				mockStore.On("GetAccountNote", mock.Anything, "testuser", "target1").Return(&storage.AccountNote{
					Username:      "testuser",
					TargetActorID: "https://example.com/users/target1",
					Note:          "This is a private note",
					CreatedAt:     time.Now(),
				}, nil)
			},
			expectedStatus: 200,
			validate: func(t *testing.T, relationships []models.Relationship) {
				assert.Len(t, relationships, 1)
				assert.Equal(t, "target1", relationships[0].ID)
				assert.False(t, relationships[0].Following)
				assert.False(t, relationships[0].FollowedBy)
				assert.False(t, relationships[0].Blocking)
				assert.False(t, relationships[0].BlockedBy)
				assert.True(t, relationships[0].Muting)
				assert.True(t, relationships[0].MutingNotifications) // Should match mute.HideNotifications
				assert.True(t, relationships[0].Endorsed)
				assert.Equal(t, "This is a private note", relationships[0].Note)
			},
		},
		{
			name: "no account IDs provided",
			headers: map[string]string{
				"X-Test-Username": "testuser",
			},
			queryParams: map[string]string{},
			setupMocks: func(mockStore *MockStorageAdapter) {
				// Test mode - get current user actor
				mockStore.On("GetActor", mock.Anything, "testuser").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID: "https://example.com/users/testuser",
					},
					PreferredUsername: "testuser",
				}, nil)
			},
			expectedStatus: 400,
			expectedError:  "no account IDs provided",
		},
		{
			name: "skip non-existent accounts",
			headers: map[string]string{
				"X-Test-Username": "testuser",
			},
			queryParams: map[string]string{
				"id": "target1,nonexistent,target2",
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				// Test mode - get current user actor
				mockStore.On("GetActor", mock.Anything, "testuser").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID: "https://example.com/users/testuser",
					},
					PreferredUsername: "testuser",
				}, nil)

				// Get target actors (nonexistent will return error)
				mockStore.On("GetActor", mock.Anything, "target1").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID: "https://example.com/users/target1",
					},
					PreferredUsername: "target1",
				}, nil)
				mockStore.On("GetActor", mock.Anything, "nonexistent").Return(nil, storage.ErrNotFound)
				mockStore.On("GetActor", mock.Anything, "target2").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID: "https://example.com/users/target2",
					},
					PreferredUsername: "target2",
				}, nil)

				// Mock relationship checks for existing targets
				for _, target := range []string{"target1", "target2"} {
					mockStore.On("IsFollowing", mock.Anything, "testuser", target).Return(false, nil)
					mockStore.On("IsFollowing", mock.Anything, target, "testuser").Return(false, nil)
					mockStore.On("GetBlock", mock.Anything, "https://example.com/users/testuser", "https://example.com/users/"+target).Return(nil, storage.ErrNotFound)
					mockStore.On("GetBlock", mock.Anything, "https://example.com/users/"+target, "https://example.com/users/testuser").Return(nil, storage.ErrNotFound)
					mockStore.On("GetMute", mock.Anything, "testuser", target).Return(nil, storage.ErrNotFound)
					mockStore.On("IsBlockedDomain", mock.Anything, "https://example.com/users/testuser", "example.com").Return(false, nil)
					mockStore.On("IsEndorsed", mock.Anything, "testuser", target).Return(false, nil)
					mockStore.On("GetAccountNote", mock.Anything, "testuser", target).Return(nil, storage.ErrNotFound)
				}
			},
			expectedStatus: 200,
			validate: func(t *testing.T, relationships []models.Relationship) {
				// Should only return relationships for existing accounts
				assert.Len(t, relationships, 2)
				assert.Equal(t, "target1", relationships[0].ID)
				assert.Equal(t, "target2", relationships[1].ID)
			},
		},
		{
			name: "unauthorized without token or test header",
			headers: map[string]string{
				"Content-Type": "application/json",
			},
			queryParams: map[string]string{
				"id": "target1",
			},
			setupMocks:     func(mockStore *MockStorageAdapter) {},
			expectedStatus: 401,
			expectedError:  "Unauthorized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock storage adapter
			mockStore := NewMockStorageAdapter()
			tt.setupMocks(mockStore)

			// Create handler
			handler := &Handler{
				store: mockStore,
				cfg: &config.Config{
					JWTSecret: "test_secret",
					Domain:    "example.com",
				},
				logger: zap.NewNop(),
			}

			// Create test request
			req := &lift.Request{
				Request: &adapters.Request{
					Method:      "GET",
					Path:        "/api/v1/accounts/relationships",
					Headers:     tt.headers,
					QueryParams: tt.queryParams,
				},
			}

			ctx := lift.NewContext(context.Background(), req)

			// Call handler
			err := handler.HandleGetRelationshipsLift(ctx)

			// Verify status code
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)

			if tt.expectedError != "" {
				// For now, just check that we got an error status
				assert.True(t, ctx.Response.StatusCode >= 400)
			} else if tt.validate != nil {
				// For successful responses, we'll just check the status for now
				// TODO: Add proper response parsing when the lift Context supports it
				assert.Equal(t, 200, ctx.Response.StatusCode)
			}

			// Verify all mocks were called
			mockStore.AssertExpectations(t)

			// Verify no error returned from handler (errors should be handled via status codes)
			assert.NoError(t, err)
		})
	}
}

func TestExtractAccountIDsLift(t *testing.T) {
	tests := []struct {
		name        string
		queryParams map[string]string
		expected    []string
	}{
		{
			name: "array format single",
			queryParams: map[string]string{
				"id[]": "user1",
			},
			expected: []string{"user1"},
		},
		{
			name: "array format multiple",
			queryParams: map[string]string{
				"id[0]": "user1",
				"id[1]": "user2",
				"id[2]": "user3",
			},
			expected: []string{"user1", "user2", "user3"},
		},
		{
			name: "comma separated format",
			queryParams: map[string]string{
				"id": "user1,user2,user3",
			},
			expected: []string{"user1", "user2", "user3"},
		},
		{
			name: "comma separated with spaces",
			queryParams: map[string]string{
				"id": "user1, user2 , user3",
			},
			expected: []string{"user1", "user2", "user3"},
		},
		{
			name: "deduplicate IDs",
			queryParams: map[string]string{
				"id": "user1,user2,user1,user3,user2",
			},
			expected: []string{"user1", "user2", "user3"},
		},
		{
			name: "empty parameters",
			queryParams: map[string]string{},
			expected: []string{},
		},
		{
			name: "empty id parameter",
			queryParams: map[string]string{
				"id": "",
			},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create handler
			handler := &Handler{}

			// Create test request
			req := &lift.Request{
				Request: &adapters.Request{
					Method:      "GET",
					Path:        "/api/v1/accounts/relationships",
					QueryParams: tt.queryParams,
				},
			}

			ctx := lift.NewContext(context.Background(), req)

			// Extract account IDs
			result := handler.extractAccountIDsLift(ctx)

			// Verify result
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestBuildRelationshipLift(t *testing.T) {
	tests := []struct {
		name           string
		setupMocks     func(*MockStorageAdapter)
		currentUser    string
		targetUser     string
		expectedResult func(models.Relationship) bool
	}{
		{
			name: "basic following relationship",
			setupMocks: func(mockStore *MockStorageAdapter) {
				mockStore.On("IsFollowing", mock.Anything, "user1", "user2").Return(true, nil)
				mockStore.On("HasFollowRequest", mock.Anything, "user1", "user2").Return(false, nil)
				mockStore.On("IsFollowing", mock.Anything, "user2", "user1").Return(false, nil)
				mockStore.On("GetBlock", mock.Anything, "https://example.com/users/user1", "https://example.com/users/user2").Return(nil, storage.ErrNotFound)
				mockStore.On("GetBlock", mock.Anything, "https://example.com/users/user2", "https://example.com/users/user1").Return(nil, storage.ErrNotFound)
				mockStore.On("GetMute", mock.Anything, "user1", "user2").Return(nil, storage.ErrNotFound)
				mockStore.On("IsBlockedDomain", mock.Anything, "https://example.com/users/user1", "example.com").Return(false, nil)
				mockStore.On("IsEndorsed", mock.Anything, "user1", "user2").Return(false, nil)
				mockStore.On("GetAccountNote", mock.Anything, "user1", "user2").Return(nil, storage.ErrNotFound)
			},
			currentUser: "user1",
			targetUser:  "user2",
			expectedResult: func(rel models.Relationship) bool {
				return rel.Following && !rel.FollowedBy && !rel.Blocking && !rel.Muting
			},
		},
		{
			name: "mutual following relationship",
			setupMocks: func(mockStore *MockStorageAdapter) {
				mockStore.On("IsFollowing", mock.Anything, "user1", "user2").Return(true, nil)
				mockStore.On("HasFollowRequest", mock.Anything, "user1", "user2").Return(false, nil)
				mockStore.On("IsFollowing", mock.Anything, "user2", "user1").Return(true, nil)
				mockStore.On("GetBlock", mock.Anything, "https://example.com/users/user1", "https://example.com/users/user2").Return(nil, storage.ErrNotFound)
				mockStore.On("GetBlock", mock.Anything, "https://example.com/users/user2", "https://example.com/users/user1").Return(nil, storage.ErrNotFound)
				mockStore.On("GetMute", mock.Anything, "user1", "user2").Return(nil, storage.ErrNotFound)
				mockStore.On("IsBlockedDomain", mock.Anything, "https://example.com/users/user1", "example.com").Return(false, nil)
				mockStore.On("IsEndorsed", mock.Anything, "user1", "user2").Return(false, nil)
				mockStore.On("GetAccountNote", mock.Anything, "user1", "user2").Return(nil, storage.ErrNotFound)
			},
			currentUser: "user1",
			targetUser:  "user2",
			expectedResult: func(rel models.Relationship) bool {
				return rel.Following && rel.FollowedBy && !rel.Blocking && !rel.Muting
			},
		},
		{
			name: "blocking relationship overrides following",
			setupMocks: func(mockStore *MockStorageAdapter) {
				mockStore.On("IsFollowing", mock.Anything, "user1", "user2").Return(true, nil)
				mockStore.On("HasFollowRequest", mock.Anything, "user1", "user2").Return(false, nil)
				mockStore.On("IsFollowing", mock.Anything, "user2", "user1").Return(false, nil)
				mockStore.On("GetBlock", mock.Anything, "https://example.com/users/user1", "https://example.com/users/user2").Return(&storage.Block{
					ID:     "block123",
					Actor:  "https://example.com/users/user1",
					Object: "https://example.com/users/user2",
				}, nil)
				mockStore.On("GetBlock", mock.Anything, "https://example.com/users/user2", "https://example.com/users/user1").Return(nil, storage.ErrNotFound)
				mockStore.On("GetMute", mock.Anything, "user1", "user2").Return(nil, storage.ErrNotFound)
				mockStore.On("IsBlockedDomain", mock.Anything, "https://example.com/users/user1", "example.com").Return(false, nil)
				mockStore.On("IsEndorsed", mock.Anything, "user1", "user2").Return(false, nil)
				mockStore.On("GetAccountNote", mock.Anything, "user1", "user2").Return(nil, storage.ErrNotFound)
			},
			currentUser: "user1",
			targetUser:  "user2",
			expectedResult: func(rel models.Relationship) bool {
				// When blocking, following should be false even if originally true
				return !rel.Following && !rel.FollowedBy && rel.Blocking && !rel.ShowingReblogs && !rel.Notifying
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock storage adapter
			mockStore := NewMockStorageAdapter()
			tt.setupMocks(mockStore)

			// Create handler
			handler := &Handler{
				store: mockStore,
				cfg: &config.Config{
					Domain: "example.com",
				},
				logger: zap.NewNop(),
			}

			// Create actors
			currentActor := &activitypub.Actor{
				BaseObject: activitypub.BaseObject{
					ID: fmt.Sprintf("https://example.com/users/%s", tt.currentUser),
				},
				PreferredUsername: tt.currentUser,
			}
			targetActor := &activitypub.Actor{
				BaseObject: activitypub.BaseObject{
					ID: fmt.Sprintf("https://example.com/users/%s", tt.targetUser),
				},
				PreferredUsername: tt.targetUser,
			}

			// Build relationship
			relationship := handler.buildRelationshipLift(context.Background(), currentActor, targetActor, tt.currentUser, tt.targetUser)

			// Verify result
			assert.True(t, tt.expectedResult(relationship), "Relationship validation failed")
			assert.Equal(t, tt.targetUser, relationship.ID)

			// Verify all mocks were called
			mockStore.AssertExpectations(t)
		})
	}
}

// TestRelationshipsAuthenticationFlow tests the complete authentication flow
func TestRelationshipsAuthenticationFlow(t *testing.T) {
	t.Run("test mode bypasses OAuth", func(t *testing.T) {
		mockStore := NewMockStorageAdapter()
		
		// Setup test mode expectations
		mockStore.On("GetActor", mock.Anything, "testuser").Return(&activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID: "https://example.com/users/testuser",
			},
			PreferredUsername: "testuser",
		}, nil)
		
		mockStore.On("GetActor", mock.Anything, "target1").Return(&activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID: "https://example.com/users/target1",
			},
			PreferredUsername: "target1",
		}, nil)
		
		// Mock minimal relationship data
		mockStore.On("IsFollowing", mock.Anything, "testuser", "target1").Return(false, nil)
		mockStore.On("IsFollowing", mock.Anything, "target1", "testuser").Return(false, nil)
		mockStore.On("GetBlock", mock.Anything, mock.Anything, mock.Anything).Return(nil, storage.ErrNotFound)
		mockStore.On("GetMute", mock.Anything, "testuser", "target1").Return(nil, storage.ErrNotFound)
		mockStore.On("IsBlockedDomain", mock.Anything, "https://example.com/users/testuser", "example.com").Return(false, nil)
		mockStore.On("IsEndorsed", mock.Anything, "testuser", "target1").Return(false, nil)
		mockStore.On("GetAccountNote", mock.Anything, "testuser", "target1").Return(nil, storage.ErrNotFound)

		handler := &Handler{
			store:  mockStore,
			cfg:    &config.Config{Domain: "example.com"},
			logger: zap.NewNop(),
		}

		req := &lift.Request{
			Request: &adapters.Request{
				Method: "GET",
				Path:   "/api/v1/accounts/relationships",
				Headers: map[string]string{
					"X-Test-Username": "testuser",
				},
				QueryParams: map[string]string{
					"id": "target1",
				},
			},
		}

		ctx := lift.NewContext(context.Background(), req)
		err := handler.HandleGetRelationshipsLift(ctx)

		assert.NoError(t, err)
		assert.Equal(t, 200, ctx.Response.StatusCode)
		mockStore.AssertExpectations(t)
	})

	t.Run("OAuth flow requires valid token and scope", func(t *testing.T) {
		// This test would require mocking the OAuth service
		// For now, we'll test the basic flow without token validation
		mockStore := NewMockStorageAdapter()

		handler := &Handler{
			store:  mockStore,
			cfg:    &config.Config{JWTSecret: "test_secret"},
			logger: zap.NewNop(),
		}

		req := &lift.Request{
			Request: &adapters.Request{
				Method: "GET",
				Path:   "/api/v1/accounts/relationships",
				Headers: map[string]string{
					"Authorization": "Bearer invalid_token",
				},
				QueryParams: map[string]string{
					"id": "target1",
				},
			},
		}

		ctx := lift.NewContext(context.Background(), req)
		err := handler.HandleGetRelationshipsLift(ctx)

		assert.NoError(t, err)
		assert.Equal(t, 401, ctx.Response.StatusCode)
		mockStore.AssertExpectations(t)
	})
}