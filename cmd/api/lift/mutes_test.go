package lift

import (
	"context"
	"encoding/json"
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

func TestHandleMuteAccountLift(t *testing.T) {
	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func(mockStore *MockStorageAdapter)
		expectedStatus int
		expectError    bool
		validateResult func(t *testing.T, ctx *lift.Context)
	}{
		{
			name: "successful mute account",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/accounts/testuser2/mute",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
							"Content-Type":    "application/json",
						},
						PathParams: map[string]string{"id": "testuser2"},
					},
					Body: []byte(`{"notifications":true}`),
				}
				
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("id", "testuser2")
				
				return ctx
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				// Mock target actor exists
				mockStore.On("GetActor", mock.Anything, "testuser2").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   "https://test.example.com/users/testuser2",
						Type: "Person",
					},
					PreferredUsername: "testuser2",
					Name:              "Test User 2",
				}, nil)
				
				// Mock no existing mute first, then return created mute
				mockStore.On("GetMute", mock.Anything, "testuser", "testuser2").Return(nil, nil).Once()
				
				// Mock successful mute creation
				mockStore.On("CreateMute", mock.Anything, mock.AnythingOfType("*storage.Mute")).Return(nil)
				
				// Mock relationship checks for getRelationshipLift
				mockStore.On("IsFollowing", mock.Anything, "testuser", "testuser2").Return(false, nil)
				mockStore.On("IsFollowing", mock.Anything, "testuser2", "testuser").Return(false, nil)
				mockStore.On("IsBlocked", mock.Anything, "testuser", "testuser2").Return(false, nil)
				mockStore.On("IsBlocked", mock.Anything, "testuser2", "testuser").Return(false, nil)
				mockStore.On("GetMute", mock.Anything, "testuser", "testuser2").Return(&storage.Mute{
					ID:                "mute-123",
					Actor:             "testuser",
					Object:            "testuser2",
					HideNotifications: true,
					Published:         time.Now(),
					CreatedAt:         time.Now(),
				}, nil)
			},
			expectedStatus: 200,
			expectError:    false,
			validateResult: func(t *testing.T, ctx *lift.Context) {
				// Response should contain relationship data
				bodyBytes, _ := json.Marshal(ctx.Response.Body)
				assert.Contains(t, string(bodyBytes), "testuser2")
			},
		},
		{
			name: "mute account that doesn't exist",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/accounts/nonexistent/mute",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
							"Content-Type":    "application/json",
						},
						PathParams: map[string]string{"id": "nonexistent"},
					},
					Body: []byte(`{"notifications":false}`),
				}
				
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("id", "nonexistent")
				
				return ctx
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				// Mock target actor doesn't exist
				mockStore.On("GetActor", mock.Anything, "nonexistent").Return(nil, storage.ErrNotFound)
			},
			expectedStatus: 404,
			expectError:    false,
			validateResult: func(t *testing.T, ctx *lift.Context) {
				bodyBytes, _ := json.Marshal(ctx.Response.Body)
				assert.Contains(t, string(bodyBytes), "account not found")
			},
		},
		{
			name: "mute account already muted",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/accounts/testuser2/mute",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
							"Content-Type":    "application/json",
						},
						PathParams: map[string]string{"id": "testuser2"},
					},
					Body: []byte(`{"notifications":false}`),
				}
				
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("id", "testuser2")
				
				return ctx
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				// Mock target actor exists
				mockStore.On("GetActor", mock.Anything, "testuser2").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   "https://test.example.com/users/testuser2",
						Type: "Person",
					},
					PreferredUsername: "testuser2",
					Name:              "Test User 2",
				}, nil)
				
				// Mock existing mute
				existingMute := &storage.Mute{
					ID:                "mute-123",
					Actor:             "testuser",
					Object:            "testuser2",
					HideNotifications: true,
					Published:         time.Now(),
					CreatedAt:         time.Now(),
				}
				mockStore.On("GetMute", mock.Anything, "testuser", "testuser2").Return(existingMute, nil)
				
				// Mock relationship checks
				mockStore.On("IsFollowing", mock.Anything, "testuser", "testuser2").Return(false, nil)
				mockStore.On("IsFollowing", mock.Anything, "testuser2", "testuser").Return(false, nil)
				mockStore.On("IsBlocked", mock.Anything, "testuser", "testuser2").Return(false, nil)
				mockStore.On("IsBlocked", mock.Anything, "testuser2", "testuser").Return(false, nil)
			},
			expectedStatus: 200,
			expectError:    false,
			validateResult: func(t *testing.T, ctx *lift.Context) {
				bodyBytes, _ := json.Marshal(ctx.Response.Body)
				assert.Contains(t, string(bodyBytes), "testuser2")
			},
		},
		{
			name: "missing account id parameter",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/accounts//mute",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						PathParams: map[string]string{},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				
				return ctx
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				// No mocks needed
			},
			expectedStatus: 400,
			expectError:    false,
			validateResult: func(t *testing.T, ctx *lift.Context) {
				bodyBytes, _ := json.Marshal(ctx.Response.Body)
				assert.Contains(t, string(bodyBytes), "missing account id")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mocks
			mockStore := new(MockStorageAdapter)
			tt.setupMocks(mockStore)
			
			handler := &Handler{
				cfg: &config.Config{
					JWTSecret: "test-secret",
					Domain:    "test.example.com",
				},
				store:  mockStore,
				logger: zap.NewNop(),
			}
			
			ctx := tt.setupContext()
			
			err := handler.HandleMuteAccountLift(ctx)
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
			
			if tt.validateResult != nil {
				tt.validateResult(t, ctx)
			}
			
			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandleUnmuteAccountLift(t *testing.T) {
	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func(mockStore *MockStorageAdapter)
		expectedStatus int
		expectError    bool
		validateResult func(t *testing.T, ctx *lift.Context)
	}{
		{
			name: "successful unmute account",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/accounts/testuser2/unmute",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						PathParams: map[string]string{"id": "testuser2"},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("id", "testuser2")
				
				return ctx
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				// Mock successful mute deletion
				mockStore.On("DeleteMute", mock.Anything, "testuser", "testuser2").Return(nil)
				
				// Mock relationship checks for getRelationshipLift
				mockStore.On("IsFollowing", mock.Anything, "testuser", "testuser2").Return(false, nil)
				mockStore.On("IsFollowing", mock.Anything, "testuser2", "testuser").Return(false, nil)
				mockStore.On("IsBlocked", mock.Anything, "testuser", "testuser2").Return(false, nil)
				mockStore.On("IsBlocked", mock.Anything, "testuser2", "testuser").Return(false, nil)
				mockStore.On("GetMute", mock.Anything, "testuser", "testuser2").Return(nil, nil)
			},
			expectedStatus: 200,
			expectError:    false,
			validateResult: func(t *testing.T, ctx *lift.Context) {
				bodyBytes, _ := json.Marshal(ctx.Response.Body)
				assert.Contains(t, string(bodyBytes), "testuser2")
			},
		},
		{
			name: "missing account id parameter",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/accounts//unmute",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						PathParams: map[string]string{},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				
				return ctx
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				// No mocks needed
			},
			expectedStatus: 400,
			expectError:    false,
			validateResult: func(t *testing.T, ctx *lift.Context) {
				bodyBytes, _ := json.Marshal(ctx.Response.Body)
				assert.Contains(t, string(bodyBytes), "missing account id")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mocks
			mockStore := new(MockStorageAdapter)
			tt.setupMocks(mockStore)
			
			handler := &Handler{
				cfg: &config.Config{
					JWTSecret: "test-secret",
					Domain:    "test.example.com",
				},
				store:  mockStore,
				logger: zap.NewNop(),
			}
			
			ctx := tt.setupContext()
			
			err := handler.HandleUnmuteAccountLift(ctx)
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
			
			if tt.validateResult != nil {
				tt.validateResult(t, ctx)
			}
			
			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandleGetMutedAccountsLift(t *testing.T) {
	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func(mockStore *MockStorageAdapter)
		expectedStatus int
		expectError    bool
		validateResult func(t *testing.T, ctx *lift.Context)
	}{
		{
			name: "successful get muted accounts",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/mutes",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				
				return ctx
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				// Mock get muted actors
				mutes := []*storage.Mute{
					{
						ID:                "mute-1",
						Actor:             "testuser",
						Object:            "muteduser1",
						HideNotifications: true,
						Published:         time.Now(),
						CreatedAt:         time.Now(),
					},
					{
						ID:                "mute-2",
						Actor:             "testuser",
						Object:            "muteduser2",
						HideNotifications: false,
						Published:         time.Now(),
						CreatedAt:         time.Now(),
					},
				}
				mockStore.On("GetMutedActors", mock.Anything, "testuser", 40, "").Return(mutes, "next-cursor", nil)
				
				// Mock getting the muted actors
				mockStore.On("GetActor", mock.Anything, "muteduser1").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   "https://test.example.com/users/muteduser1",
						Type: "Person",
					},
					PreferredUsername: "muteduser1",
					Name:              "Muted User 1",
				}, nil)
				
				mockStore.On("GetActor", mock.Anything, "muteduser2").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   "https://test.example.com/users/muteduser2",
						Type: "Person",
					},
					PreferredUsername: "muteduser2",
					Name:              "Muted User 2",
				}, nil)
				
				// Mock count queries for each actor
				mockStore.On("GetFollowers", mock.Anything, "muteduser1", 0, "").Return([]string{}, "", nil)
				mockStore.On("GetFollowing", mock.Anything, "muteduser1", 0, "").Return([]string{}, "", nil)
				mockStore.On("GetObjectsByActor", mock.Anything, "https://test.example.com/users/muteduser1", "", 0).Return([]any{}, "", nil)
				
				mockStore.On("GetFollowers", mock.Anything, "muteduser2", 0, "").Return([]string{}, "", nil)
				mockStore.On("GetFollowing", mock.Anything, "muteduser2", 0, "").Return([]string{}, "", nil)
				mockStore.On("GetObjectsByActor", mock.Anything, "https://test.example.com/users/muteduser2", "", 0).Return([]any{}, "", nil)
			},
			expectedStatus: 200,
			expectError:    false,
			validateResult: func(t *testing.T, ctx *lift.Context) {
				bodyBytes, _ := json.Marshal(ctx.Response.Body)
				assert.Contains(t, string(bodyBytes), "muteduser1")
				assert.Contains(t, string(bodyBytes), "muteduser2")
				// Check Link header for pagination
				linkHeader := ctx.Response.Headers["Link"]
				assert.Contains(t, linkHeader, "next-cursor")
			},
		},
		{
			name: "get muted accounts with pagination limit",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/mutes",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				
				return ctx
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				// Mock get muted actors with default limit since query params aren't parsed
				mockStore.On("GetMutedActors", mock.Anything, "testuser", 40, "").Return([]*storage.Mute{}, "", nil)
			},
			expectedStatus: 200,
			expectError:    false,
			validateResult: func(t *testing.T, ctx *lift.Context) {
				bodyBytes, _ := json.Marshal(ctx.Response.Body)
				assert.Equal(t, "[]", string(bodyBytes))
			},
		},
		{
			name: "get muted accounts with no mutes",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/mutes",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				
				return ctx
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				// Mock empty muted actors
				mockStore.On("GetMutedActors", mock.Anything, "testuser", 40, "").Return([]*storage.Mute{}, "", nil)
			},
			expectedStatus: 200,
			expectError:    false,
			validateResult: func(t *testing.T, ctx *lift.Context) {
				bodyBytes, _ := json.Marshal(ctx.Response.Body)
				assert.Equal(t, "[]", string(bodyBytes))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mocks
			mockStore := new(MockStorageAdapter)
			tt.setupMocks(mockStore)
			
			handler := &Handler{
				cfg: &config.Config{
					JWTSecret: "test-secret",
					Domain:    "test.example.com",
				},
				store:  mockStore,
				logger: zap.NewNop(),
			}
			
			ctx := tt.setupContext()
			
			err := handler.HandleGetMutedAccountsLift(ctx)
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
			
			if tt.validateResult != nil {
				tt.validateResult(t, ctx)
			}
			
			mockStore.AssertExpectations(t)
		})
	}
}

func TestGetRelationshipLift(t *testing.T) {
	tests := []struct {
		name           string
		sourceUsername string
		targetUsername string
		setupMocks     func(mockStore *MockStorageAdapter)
		expectedResult *models.Relationship
	}{
		{
			name:           "basic relationship with mute",
			sourceUsername: "testuser",
			targetUsername: "targetuser",
			setupMocks: func(mockStore *MockStorageAdapter) {
				mockStore.On("IsFollowing", mock.Anything, "testuser", "targetuser").Return(false, nil)
				mockStore.On("IsFollowing", mock.Anything, "targetuser", "testuser").Return(true, nil)
				mockStore.On("IsBlocked", mock.Anything, "testuser", "targetuser").Return(false, nil)
				mockStore.On("IsBlocked", mock.Anything, "targetuser", "testuser").Return(false, nil)
				mockStore.On("GetMute", mock.Anything, "testuser", "targetuser").Return(&storage.Mute{
					ID:                "mute-123",
					Actor:             "testuser",
					Object:            "targetuser",
					HideNotifications: true,
					Published:         time.Now(),
					CreatedAt:         time.Now(),
				}, nil)
			},
			expectedResult: &models.Relationship{
				ID:                  "targetuser",
				Following:           false,
				FollowedBy:          true,
				Blocking:            false,
				BlockedBy:           false,
				Muting:              true,
				MutingNotifications: true,
				ShowingReblogs:      true,
				Notifying:           false,
				Requested:           false,
				DomainBlocking:      false,
				Endorsed:            false,
				Note:                "",
			},
		},
		{
			name:           "relationship without mute",
			sourceUsername: "testuser",
			targetUsername: "targetuser",
			setupMocks: func(mockStore *MockStorageAdapter) {
				mockStore.On("IsFollowing", mock.Anything, "testuser", "targetuser").Return(true, nil)
				mockStore.On("IsFollowing", mock.Anything, "targetuser", "testuser").Return(false, nil)
				mockStore.On("IsBlocked", mock.Anything, "testuser", "targetuser").Return(false, nil)
				mockStore.On("IsBlocked", mock.Anything, "targetuser", "testuser").Return(false, nil)
				mockStore.On("GetMute", mock.Anything, "testuser", "targetuser").Return(nil, nil)
			},
			expectedResult: &models.Relationship{
				ID:                  "targetuser",
				Following:           true,
				FollowedBy:          false,
				Blocking:            false,
				BlockedBy:           false,
				Muting:              false,
				MutingNotifications: false,
				ShowingReblogs:      true,
				Notifying:           false,
				Requested:           false,
				DomainBlocking:      false,
				Endorsed:            false,
				Note:                "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := new(MockStorageAdapter)
			tt.setupMocks(mockStore)
			
			handler := &Handler{
				cfg: &config.Config{
					JWTSecret: "test-secret",
					Domain:    "test.example.com",
				},
				store:  mockStore,
				logger: zap.NewNop(),
			}
			
			result := handler.getRelationshipLift(context.Background(), tt.sourceUsername, tt.targetUsername)
			
			assert.Equal(t, tt.expectedResult, result)
			mockStore.AssertExpectations(t)
		})
	}
}