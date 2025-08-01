package lift

import (
	"context"
	"errors"
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

func TestHandleGetFollowRequestsLift(t *testing.T) {
	var mockStore *MockStorageAdapter

	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func()
		expectedStatus int
		expectError    bool
	}{
		{
			name: "successful follow requests for locked account - test mode",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/follow_requests",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
					},
				}
				return lift.NewContext(context.Background(), req)
			},
			setupMocks: func() {
				// Test mode - get current user actor (locked)
				mockStore.On("GetActor", mock.Anything, "testuser").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID: "https://example.com/users/testuser",
					},
					PreferredUsername:         "testuser",
					ManuallyApprovesFollowers: true,
				}, nil)

				// Get pending follow requests
				mockStore.On("GetPendingFollowRequests", mock.Anything, "testuser", 100, "").Return(
					[]string{"follower1", "follower2"}, "", nil)

				// Get follower actors
				mockStore.On("GetActor", mock.Anything, "follower1").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID: "https://remote.com/users/follower1",
					},
					PreferredUsername: "follower1",
					Name:              "Follower One",
					Summary:           "Test follower 1",
					URL:               "https://remote.com/users/follower1",
				}, nil)

				mockStore.On("GetActor", mock.Anything, "follower2").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID: "https://remote.com/users/follower2",
					},
					PreferredUsername: "follower2",
					Name:              "Follower Two",
					Summary:           "Test follower 2",
					URL:               "https://remote.com/users/follower2",
				}, nil)

				// Mock metadata calls for convertActorToAccount
				mockStore.On("GetActorWithMetadata", mock.Anything, "follower1").Return(
					nil, &storage.ActorMetadata{
						CreatedAt: time.Now(),
					}, nil)
				mockStore.On("GetActorWithMetadata", mock.Anything, "follower2").Return(
					nil, &storage.ActorMetadata{
						CreatedAt: time.Now(),
					}, nil)

				// Mock count calls
				mockStore.On("GetStatusCount", mock.Anything, mock.AnythingOfType("string")).Return(0, nil)
				mockStore.On("GetFollowersCount", mock.Anything, mock.AnythingOfType("string")).Return(0, nil)
				mockStore.On("GetFollowing", mock.Anything, mock.AnythingOfType("string"), 1, "").Return([]string{}, "", nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "empty array for unlocked account - test mode",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/follow_requests",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
					},
				}
				return lift.NewContext(context.Background(), req)
			},
			setupMocks: func() {
				// Test mode - get current user actor (unlocked)
				mockStore.On("GetActor", mock.Anything, "testuser").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID: "https://example.com/users/testuser",
					},
					PreferredUsername:         "testuser",
					ManuallyApprovesFollowers: false, // Account is not locked
				}, nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "actor not found in test mode",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/follow_requests",
						Headers: map[string]string{
							"X-Test-Username": "nonexistent",
						},
					},
				}
				return lift.NewContext(context.Background(), req)
			},
			setupMocks: func() {
				mockStore.On("GetActor", mock.Anything, "nonexistent").Return(
					nil, errors.New("actor not found"))
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
					Domain:    "example.com",
				},
				store:          mockStore,
				logger:         zap.NewNop(),
				authMiddleware: &auth.Middleware{},
			}

			// Get context
			ctx := tt.setupContext()

			// Execute
			err := handler.HandleGetFollowRequestsLift(ctx)

			// Verify
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

func TestHandleAuthorizeFollowRequestLift(t *testing.T) {
	var mockStore *MockStorageAdapter

	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func()
		expectedStatus int
		expectError    bool
	}{
		{
			name: "successful authorization - test mode",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/follow_requests/follower1/authorize",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						PathParams: map[string]string{"account_id": "follower1"},
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("account_id", "follower1")
				return ctx
			},
			setupMocks: func() {
				// Test mode - get current user actor (locked)
				mockStore.On("GetActor", mock.Anything, "testuser").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID: "https://example.com/users/testuser",
					},
					PreferredUsername:         "testuser",
					ManuallyApprovesFollowers: true,
				}, nil)

				// Get the follow request
				mockStore.On("GetFollowRequest", mock.Anything, "follower1", "testuser").Return(
					&storage.RelationshipRecord{}, nil)

				// Accept the follow request
				mockStore.On("AcceptFollowRequest", mock.Anything, "follower1", "testuser").Return(nil)

				// Mock for sendAcceptActivity - called in async goroutine
				// GetActor is called for follower (to get inbox)
				mockStore.On("GetActor", mock.Anything, "follower1").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID: "https://remote.com/users/follower1",
					},
					PreferredUsername: "follower1",
					Inbox:             "https://remote.com/users/follower1/inbox",
					URL:               "https://remote.com/users/follower1",
				}, nil).Maybe()
				
				// GetActor is called for followed user (to create activity)
				mockStore.On("GetActor", mock.Anything, "testuser").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID: "https://example.com/users/testuser",
					},
					PreferredUsername: "testuser",
					URL:              "https://example.com/users/testuser",
				}, nil).Maybe()
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "unlocked account cannot authorize",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/follow_requests/follower1/authorize",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						PathParams: map[string]string{"account_id": "follower1"},
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("account_id", "follower1")
				return ctx
			},
			setupMocks: func() {
				// Test mode - get current user actor (unlocked)
				mockStore.On("GetActor", mock.Anything, "testuser").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID: "https://example.com/users/testuser",
					},
					PreferredUsername:         "testuser",
					ManuallyApprovesFollowers: false, // Not locked
				}, nil)
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    false,
		},
		{
			name: "follow request not found",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/follow_requests/follower1/authorize",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						PathParams: map[string]string{"account_id": "follower1"},
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("account_id", "follower1")
				return ctx
			},
			setupMocks: func() {
				// Test mode - get current user actor (locked)
				mockStore.On("GetActor", mock.Anything, "testuser").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID: "https://example.com/users/testuser",
					},
					PreferredUsername:         "testuser",
					ManuallyApprovesFollowers: true,
				}, nil)

				// Follow request not found
				mockStore.On("GetFollowRequest", mock.Anything, "follower1", "testuser").Return(
					nil, errors.New("not found"))
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
					Domain:    "example.com",
				},
				store:          mockStore,
				logger:         zap.NewNop(),
				authMiddleware: &auth.Middleware{},
			}

			// Get context
			ctx := tt.setupContext()

			// Execute
			err := handler.HandleAuthorizeFollowRequestLift(ctx)

			// Verify
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

func TestHandleRejectFollowRequestLift(t *testing.T) {
	var mockStore *MockStorageAdapter

	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func()
		expectedStatus int
		expectError    bool
	}{
		{
			name: "successful rejection - test mode",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/follow_requests/follower1/reject",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						PathParams: map[string]string{"account_id": "follower1"},
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("account_id", "follower1")
				return ctx
			},
			setupMocks: func() {
				// Test mode - get current user actor (locked)
				mockStore.On("GetActor", mock.Anything, "testuser").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID: "https://example.com/users/testuser",
					},
					PreferredUsername:         "testuser",
					ManuallyApprovesFollowers: true,
				}, nil)

				// Get the follow request
				mockStore.On("GetFollowRequest", mock.Anything, "follower1", "testuser").Return(
					&storage.RelationshipRecord{}, nil)

				// Reject the follow request
				mockStore.On("RejectFollowRequest", mock.Anything, "follower1", "testuser").Return(nil)

				// Mock for sendRejectActivity - called in async goroutine
				// GetActor is called for follower (to get inbox)
				mockStore.On("GetActor", mock.Anything, "follower1").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID: "https://remote.com/users/follower1",
					},
					PreferredUsername: "follower1",
					Inbox:             "https://remote.com/users/follower1/inbox",
					URL:               "https://remote.com/users/follower1",
				}, nil).Maybe()
				
				// GetActor is called for followed user (to create activity)
				mockStore.On("GetActor", mock.Anything, "testuser").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID: "https://example.com/users/testuser",
					},
					PreferredUsername: "testuser",
					URL:              "https://example.com/users/testuser",
				}, nil).Maybe()
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "unlocked account cannot reject",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/follow_requests/follower1/reject",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						PathParams: map[string]string{"account_id": "follower1"},
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("account_id", "follower1")
				return ctx
			},
			setupMocks: func() {
				// Test mode - get current user actor (unlocked)
				mockStore.On("GetActor", mock.Anything, "testuser").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID: "https://example.com/users/testuser",
					},
					PreferredUsername:         "testuser",
					ManuallyApprovesFollowers: false, // Not locked
				}, nil)
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
					Domain:    "example.com",
				},
				store:          mockStore,
				logger:         zap.NewNop(),
				authMiddleware: &auth.Middleware{},
			}

			// Get context
			ctx := tt.setupContext()

			// Execute
			err := handler.HandleRejectFollowRequestLift(ctx)

			// Verify
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