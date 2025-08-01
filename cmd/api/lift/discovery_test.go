package lift

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

func TestHandleGetDirectoryLift(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		setupMocks     func(*MockStorageAdapter)
		expectedStatus int
		expectedCount  int
	}{
		{
			name:        "default parameters",
			queryParams: "",
			setupMocks: func(m *MockStorageAdapter) {
				actors := []*activitypub.Actor{
					{
						BaseObject: activitypub.BaseObject{
							ID:        "https://test.example.com/users/user1",
							Type:      "Person",
							Published: &time.Time{},
						},
						PreferredUsername: "user1",
						Name:              "User One",
						Icon:              &activitypub.Image{URL: "https://test.example.com/avatar1.jpg"},
					},
					{
						BaseObject: activitypub.BaseObject{
							ID:        "https://test.example.com/users/user2",
							Type:      "Person",
							Published: &time.Time{},
						},
						PreferredUsername: "user2",
						Name:              "User Two",
						Icon:              &activitypub.Image{URL: "https://test.example.com/avatar2.jpg"},
					},
				}
				m.On("SearchAccounts", mock.Anything, "", 80, false, 0).Return(actors, nil)
				m.On("GetFollowersCount", mock.Anything, mock.Anything).Return(10, nil)
				m.On("GetFollowingCount", mock.Anything, mock.Anything).Return(5, nil)
				m.On("GetStatusCount", mock.Anything, mock.Anything).Return(20, nil)
			},
			expectedStatus: http.StatusOK,
			expectedCount:  2,
		},
		{
			name:        "with limit and offset",
			queryParams: "?limit=10&offset=20",
			setupMocks: func(m *MockStorageAdapter) {
				m.On("SearchAccounts", mock.Anything, "", 20, false, 20).Return([]*activitypub.Actor{}, nil)
			},
			expectedStatus: http.StatusOK,
			expectedCount:  0,
		},
		{
			name:        "local only filter",
			queryParams: "?local=true",
			setupMocks: func(m *MockStorageAdapter) {
				actors := []*activitypub.Actor{
					{
						BaseObject: activitypub.BaseObject{
							ID:        "https://test.example.com/users/localuser",
							Type:      "Person",
							Published: &time.Time{},
						},
						PreferredUsername: "localuser",
						Icon:              &activitypub.Image{URL: "https://test.example.com/avatar.jpg"},
					},
					{
						BaseObject: activitypub.BaseObject{
							ID:        "https://remote.example.com/users/remoteuser",
							Type:      "Person",
							Published: &time.Time{},
						},
						PreferredUsername: "remoteuser",
						Icon:              &activitypub.Image{URL: "https://remote.example.com/avatar.jpg"},
					},
				}
				m.On("SearchAccounts", mock.Anything, "", 80, false, 0).Return(actors, nil)
				m.On("GetFollowersCount", mock.Anything, mock.Anything).Return(0, nil)
				m.On("GetFollowingCount", mock.Anything, mock.Anything).Return(0, nil)
				m.On("GetStatusCount", mock.Anything, mock.Anything).Return(0, nil)
			},
			expectedStatus: http.StatusOK,
			expectedCount:  1, // Only local user
		},
		{
			name:        "error from storage",
			queryParams: "",
			setupMocks: func(m *MockStorageAdapter) {
				m.On("SearchAccounts", mock.Anything, "", 80, false, 0).Return(nil, errors.New("database error"))
			},
			expectedStatus: http.StatusInternalServerError,
			expectedCount:  0,
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

			ctx := &lift.Context{
				Context: context.Background(),
				Request: &lift.Request{
					Method: "GET",
					Path:   "/api/v1/directory" + tt.queryParams,
					Headers: map[string]string{
						"Content-Type": "application/json",
					},
				},
			}
			ctx.Response = &lift.Response{
				Headers:    make(map[string]string),
				StatusCode: 200,
			}

			err := handler.HandleGetDirectoryLift(ctx)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)

			if tt.expectedStatus == http.StatusOK {
				accounts, ok := ctx.Response.Body.([]map[string]any)
				assert.True(t, ok)
				assert.Equal(t, tt.expectedCount, len(accounts))
			}

			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandleGetSuggestionsV1Lift(t *testing.T) {
	tests := []struct {
		name           string
		headers        map[string]string
		setupMocks     func(*MockStorageAdapter)
		expectedStatus int
		expectedCount  int
	}{
		{
			name:    "authenticated user gets suggestions",
			headers: map[string]string{"X-Test-Username": "testuser"},
			setupMocks: func(m *MockStorageAdapter) {
				actors := []*activitypub.Actor{
					{
						BaseObject: activitypub.BaseObject{
							ID:        "https://test.example.com/users/suggested1",
							Type:      "Person",
							Published: &time.Time{},
						},
						PreferredUsername: "suggested1",
						Name:              "Suggested User 1",
						Icon:              &activitypub.Image{URL: "https://test.example.com/avatar.jpg"},
						Discoverable:      true,
					},
				}
				m.On("GetAccountSuggestions", mock.Anything, "testuser", 40).Return(actors, nil)
				m.On("GetFollowersCount", mock.Anything, mock.Anything).Return(100, nil)
				m.On("GetFollowingCount", mock.Anything, mock.Anything).Return(50, nil)
				m.On("GetStatusCount", mock.Anything, mock.Anything).Return(200, nil)
			},
			expectedStatus: http.StatusOK,
			expectedCount:  1,
		},
		{
			name:           "unauthenticated returns 401",
			headers:        map[string]string{},
			setupMocks:     func(m *MockStorageAdapter) {},
			expectedStatus: http.StatusUnauthorized,
			expectedCount:  0,
		},
		{
			name:    "storage error returns 500",
			headers: map[string]string{"X-Test-Username": "testuser"},
			setupMocks: func(m *MockStorageAdapter) {
				m.On("GetAccountSuggestions", mock.Anything, "testuser", 40).Return(nil, errors.New("database error"))
			},
			expectedStatus: http.StatusInternalServerError,
			expectedCount:  0,
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

			ctx := &lift.Context{
				Context: context.Background(),
				Request: &lift.Request{
					Method:  "GET",
					Path:    "/api/v1/suggestions",
					Headers: tt.headers,
				},
			}
			ctx.Response = &lift.Response{
				Headers:    make(map[string]string),
				StatusCode: 200,
			}

			err := handler.HandleGetSuggestionsV1Lift(ctx)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)

			if tt.expectedStatus == http.StatusOK {
				suggestions, ok := ctx.Response.Body.([]map[string]any)
				assert.True(t, ok)
				assert.Equal(t, tt.expectedCount, len(suggestions))
				if tt.expectedCount > 0 {
					// V1 format wraps account in suggestion object
					assert.NotNil(t, suggestions[0]["account"])
				}
			}

			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandleGetSuggestionsV2Lift(t *testing.T) {
	tests := []struct {
		name           string
		headers        map[string]string
		setupMocks     func(*MockStorageAdapter)
		expectedStatus int
		expectedCount  int
	}{
		{
			name:    "authenticated user gets suggestions with source",
			headers: map[string]string{"X-Test-Username": "testuser"},
			setupMocks: func(m *MockStorageAdapter) {
				actors := []*activitypub.Actor{
					{
						BaseObject: activitypub.BaseObject{
							ID:        "https://test.example.com/users/suggested1",
							Type:      "Person",
							Published: &time.Time{},
						},
						PreferredUsername: "suggested1",
						Name:              "Suggested User 1",
						Icon:              &activitypub.Image{URL: "https://test.example.com/avatar.jpg"},
						Discoverable:      true,
					},
					{
						BaseObject: activitypub.BaseObject{
							ID:        "https://test.example.com/users/testuser",
							Type:      "Person",
							Published: &time.Time{},
						},
						PreferredUsername: "testuser", // Should be filtered out
						Icon:              &activitypub.Image{URL: "https://test.example.com/avatar.jpg"},
					},
				}
				m.On("SearchAccounts", mock.Anything, "", 80, false, 0).Return(actors, nil)
				m.On("IsFollowing", mock.Anything, "testuser", "suggested1").Return(false, nil)
				m.On("IsFollowing", mock.Anything, "testuser", "testuser").Return(false, nil)
				m.On("GetFollowersCount", mock.Anything, mock.Anything).Return(100, nil)
				m.On("GetFollowingCount", mock.Anything, mock.Anything).Return(50, nil)
				m.On("GetStatusCount", mock.Anything, mock.Anything).Return(200, nil)
			},
			expectedStatus: http.StatusOK,
			expectedCount:  1, // testuser filtered out
		},
		{
			name:    "filters already following",
			headers: map[string]string{"X-Test-Username": "testuser"},
			setupMocks: func(m *MockStorageAdapter) {
				actors := []*activitypub.Actor{
					{
						BaseObject: activitypub.BaseObject{
							ID:        "https://test.example.com/users/alreadyfollowing",
							Type:      "Person",
							Published: &time.Time{},
						},
						PreferredUsername: "alreadyfollowing",
						Icon:              &activitypub.Image{URL: "https://test.example.com/avatar.jpg"},
					},
				}
				m.On("SearchAccounts", mock.Anything, "", 80, false, 0).Return(actors, nil)
				m.On("IsFollowing", mock.Anything, "testuser", "alreadyfollowing").Return(true, nil)
			},
			expectedStatus: http.StatusOK,
			expectedCount:  0, // Filtered out because already following
		},
		{
			name:           "unauthenticated returns 401",
			headers:        map[string]string{},
			setupMocks:     func(m *MockStorageAdapter) {},
			expectedStatus: http.StatusUnauthorized,
			expectedCount:  0,
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

			ctx := &lift.Context{
				Context: context.Background(),
				Request: &lift.Request{
					Method:  "GET",
					Path:    "/api/v2/suggestions",
					Headers: tt.headers,
				},
			}
			ctx.Response = &lift.Response{
				Headers:    make(map[string]string),
				StatusCode: 200,
			}

			err := handler.HandleGetSuggestionsV2Lift(ctx)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)

			if tt.expectedStatus == http.StatusOK {
				suggestions, ok := ctx.Response.Body.([]map[string]any)
				assert.True(t, ok)
				assert.Equal(t, tt.expectedCount, len(suggestions))
				if tt.expectedCount > 0 {
					// V2 format includes source
					assert.Equal(t, "global", suggestions[0]["source"])
					assert.NotNil(t, suggestions[0]["account"])
				}
			}

			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandleRemoveSuggestionLift(t *testing.T) {
	tests := []struct {
		name           string
		headers        map[string]string
		path           string
		setupMocks     func(*MockStorageAdapter)
		expectedStatus int
	}{
		{
			name:    "successfully removes suggestion",
			headers: map[string]string{"X-Test-Username": "testuser"},
			path:    "/api/v1/suggestions/account123",
			setupMocks: func(m *MockStorageAdapter) {
				m.On("RemoveAccountSuggestion", mock.Anything, "testuser", "account123").Return(nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "missing account ID returns 400",
			headers:        map[string]string{"X-Test-Username": "testuser"},
			path:           "/api/v1/suggestions/",
			setupMocks:     func(m *MockStorageAdapter) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "unauthenticated returns 401",
			headers:        map[string]string{},
			path:           "/api/v1/suggestions/account123",
			setupMocks:     func(m *MockStorageAdapter) {},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:    "storage error returns 500",
			headers: map[string]string{"X-Test-Username": "testuser"},
			path:    "/api/v1/suggestions/account123",
			setupMocks: func(m *MockStorageAdapter) {
				m.On("RemoveAccountSuggestion", mock.Anything, "testuser", "account123").Return(errors.New("database error"))
			},
			expectedStatus: http.StatusInternalServerError,
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

			ctx := &lift.Context{
				Context: context.Background(),
				Request: &lift.Request{
					Method:  "DELETE",
					Path:    tt.path,
					Headers: tt.headers,
				},
			}
			ctx.Response = &lift.Response{
				Headers:    make(map[string]string),
				StatusCode: 200,
			}

			err := handler.HandleRemoveSuggestionLift(ctx)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)

			mockStore.AssertExpectations(t)
		})
	}
}

func TestDiscoveryHelpers(t *testing.T) {
	handler := &Handler{
		cfg: &config.Config{
			Domain: "test.example.com",
		},
		store: new(MockStorageAdapter),
	}

	t.Run("isLocalLift", func(t *testing.T) {
		assert.True(t, handler.isLocalLift("https://test.example.com/users/local"))
		assert.False(t, handler.isLocalLift("https://remote.example.com/users/remote"))
	})

	t.Run("getAccountAcctLift", func(t *testing.T) {
		localActor := &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID: "https://test.example.com/users/local",
			},
			PreferredUsername: "local",
		}
		assert.Equal(t, "local", handler.getAccountAcctLift(localActor))

		remoteActor := &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID: "https://remote.example.com/users/remote",
			},
			PreferredUsername: "remote",
		}
		assert.Equal(t, "remote@remote.example.com", handler.getAccountAcctLift(remoteActor))
	})

	t.Run("getHeaderURLLift", func(t *testing.T) {
		actorWithImage := &activitypub.Actor{
			Image: &activitypub.Image{URL: "https://test.example.com/header.jpg"},
		}
		assert.Equal(t, "https://test.example.com/header.jpg", handler.getHeaderURLLift(actorWithImage))

		actorWithoutImage := &activitypub.Actor{}
		assert.Equal(t, "", handler.getHeaderURLLift(actorWithoutImage))
	})
}