package lift

import (
	"context"
	"encoding/json"
	"net/http"
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

func TestHandleSearchLift(t *testing.T) {
	var mockStore *MockStorageAdapter

	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func()
		expectedStatus int
		expectError    bool
		checkResponse  func(t *testing.T, ctx *lift.Context)
	}{
		{
			name: "successful search with accounts results",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/search",
						Headers: map[string]string{
							"Content-Type": "application/json",
						},
						QueryParams: map[string]string{
							"q":     "test",
							"type":  "accounts",
							"limit": "20",
						},
					},
				}
				return lift.NewContext(context.Background(), req)
			},
			setupMocks: func() {
				mockStore.On("SearchAccounts", mock.Anything, "test", 20, false, 0).Return([]*activitypub.Actor{
					{
						BaseObject: activitypub.BaseObject{
							ID:   "https://test.example.com/users/testuser",
							Type: "Person",
						},
						PreferredUsername: "testuser",
						Name:              "Test User",
						URL:               "https://test.example.com/users/testuser",
						Summary:           "Test user bio",
					},
				}, nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				var result models.SearchResult
				bodyBytes, ok := ctx.Response.Body.([]byte)
				assert.True(t, ok, "Response body should be []byte")
				err := json.Unmarshal(bodyBytes, &result)
				assert.NoError(t, err)
				assert.Len(t, result.Accounts, 1)
				assert.Equal(t, "testuser", result.Accounts[0].Username)
			},
		},
		{
			name: "search with test mode authentication",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/search",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						QueryParams: map[string]string{
							"q":    "test",
							"type": "statuses",
						},
					},
				}
				return lift.NewContext(context.Background(), req)
			},
			setupMocks: func() {
				mockStore.On("SearchStatusesWithOptions", mock.Anything, "test", mock.AnythingOfType("storage.StatusSearchOptions")).Return([]*storage.StatusSearchResult{
					{
						StatusID:  "status123",
						Content:   "Test status content",
						URL:       "https://test.example.com/statuses/123",
						Published: time.Now(),
						AuthorID:  "https://test.example.com/users/testuser",
					},
				}, nil)
				mockStore.On("GetActor", mock.Anything, "testuser").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   "https://test.example.com/users/testuser",
						Type: "Person",
					},
					PreferredUsername: "testuser",
					Name:              "Test User",
				}, nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				var result models.SearchResult
				bodyBytes, ok := ctx.Response.Body.([]byte)
				assert.True(t, ok, "Response body should be []byte")
				err := json.Unmarshal(bodyBytes, &result)
				assert.NoError(t, err)
				assert.Len(t, result.Statuses, 1)
				assert.Equal(t, "status123", result.Statuses[0].ID)
			},
		},
		{
			name: "search hashtags",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/search",
						Headers: map[string]string{
							"Content-Type": "application/json",
						},
						QueryParams: map[string]string{
							"q":    "#test",
							"type": "hashtags",
						},
					},
				}
				return lift.NewContext(context.Background(), req)
			},
			setupMocks: func() {
				mockStore.On("SearchHashtags", mock.Anything, "#test", 20).Return([]*storage.Hashtag{
					{
						Name: "test",
						URL:  "https://test.example.com/tags/test",
					},
				}, nil)
				mockStore.On("GetHashtagUsageHistory", mock.Anything, "test", 7).Return([]int{5, 3, 2, 1, 0, 0, 0}, nil)
				mockStore.On("GetDailyActiveUserCount", mock.Anything).Return(int64(10), nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				var result models.SearchResult
				bodyBytes, ok := ctx.Response.Body.([]byte)
				assert.True(t, ok, "Response body should be []byte")
				err := json.Unmarshal(bodyBytes, &result)
				assert.NoError(t, err)
				assert.Len(t, result.Hashtags, 1)
				assert.Equal(t, "test", result.Hashtags[0].Name)
			},
		},
		{
			name: "search with missing query parameter",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/search",
						Headers: map[string]string{
							"Content-Type": "application/json",
						},
						QueryParams: map[string]string{},
					},
				}
				return lift.NewContext(context.Background(), req)
			},
			setupMocks:     func() {},
			expectedStatus: http.StatusBadRequest,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				var errorResponse map[string]string
				bodyBytes, ok := ctx.Response.Body.([]byte)
				assert.True(t, ok, "Response body should be []byte")
				err := json.Unmarshal(bodyBytes, &errorResponse)
				assert.NoError(t, err)
				assert.Equal(t, "q parameter is required", errorResponse["error"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockStore = new(MockStorageAdapter)
			cfg := &config.Config{
				Domain: "test.example.com",
			}
			logger := zap.NewNop()
			handler := NewHandler(cfg, mockStore, logger, nil)

			// Setup mocks
			tt.setupMocks()

			// Execute
			ctx := tt.setupContext()
			err := handler.HandleSearchLift(ctx)

			// Verify
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)

			if tt.checkResponse != nil {
				tt.checkResponse(t, ctx)
			}

			// Verify all mocks
			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandleSearchV2Lift(t *testing.T) {
	// V2 search should behave exactly like V1
	mockStore := new(MockStorageAdapter)
	cfg := &config.Config{
		Domain: "test.example.com",
	}
	logger := zap.NewNop()
	handler := NewHandler(cfg, mockStore, logger, nil)

	// Setup context
	req := &lift.Request{
		Request: &adapters.Request{
			Method: "GET",
			Path:   "/api/v2/search",
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			QueryParams: map[string]string{
				"q":    "test",
				"type": "accounts",
			},
		},
	}
	ctx := lift.NewContext(context.Background(), req)

	// Setup mocks (same as V1)
	mockStore.On("SearchAccounts", mock.Anything, "test", 20, false, 0).Return([]*activitypub.Actor{
		{
			BaseObject: activitypub.BaseObject{
				ID:   "https://test.example.com/users/testuser",
				Type: "Person",
			},
			PreferredUsername: "testuser",
			Name:              "Test User",
		},
	}, nil)

	// Execute
	err := handler.HandleSearchV2Lift(ctx)

	// Verify
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, ctx.Response.StatusCode)

	var result models.SearchResult
	bodyBytes, ok := ctx.Response.Body.([]byte)
	assert.True(t, ok, "Response body should be []byte")
	err = json.Unmarshal(bodyBytes, &result)
	assert.NoError(t, err)
	assert.Len(t, result.Accounts, 1)

	mockStore.AssertExpectations(t)
}

func TestHandleGetNotificationsLift(t *testing.T) {
	var mockStore *MockStorageAdapter

	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func()
		expectedStatus int
		expectError    bool
		checkResponse  func(t *testing.T, ctx *lift.Context)
	}{
		{
			name: "successful get notifications with test mode",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/notifications",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						QueryParams: map[string]string{
							"limit": "20",
						},
					},
				}
				return lift.NewContext(context.Background(), req)
			},
			setupMocks: func() {
				notifications := []*storage.Notification{
					{
						ID:        "notif123",
						Type:      models.NotificationTypeMention,
						AccountID: "otheruser",
						Username:  "testuser",
						CreatedAt: time.Now(),
					},
				}
				mockStore.On("GetNotificationsFiltered", mock.Anything, "testuser", mock.AnythingOfType("*storage.NotificationFilter")).Return(notifications, "", nil)
				mockStore.On("GetActor", mock.Anything, "otheruser").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   "https://test.example.com/users/otheruser",
						Type: "Person",
					},
					PreferredUsername: "otheruser",
					Name:              "Other User",
				}, nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				var notifications []*models.Notification
				bodyBytes, ok := ctx.Response.Body.([]byte)
				assert.True(t, ok, "Response body should be []byte")
				err := json.Unmarshal(bodyBytes, &notifications)
				assert.NoError(t, err)
				assert.Len(t, notifications, 1)
				assert.Equal(t, "notif123", notifications[0].ID)
				assert.Equal(t, models.NotificationTypeMention, notifications[0].Type)
			},
		},
		{
			name: "unauthorized access without token",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/notifications",
						Headers: map[string]string{
							"Content-Type": "application/json",
						},
					},
				}
				return lift.NewContext(context.Background(), req)
			},
			setupMocks:     func() {},
			expectedStatus: http.StatusUnauthorized,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				var errorResponse map[string]string
				bodyBytes, ok := ctx.Response.Body.([]byte)
				assert.True(t, ok, "Response body should be []byte")
				err := json.Unmarshal(bodyBytes, &errorResponse)
				assert.NoError(t, err)
				assert.Equal(t, "authorization required", errorResponse["error"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockStore = new(MockStorageAdapter)
			cfg := &config.Config{
				Domain:    "test.example.com",
				JWTSecret: "test-secret",
			}
			logger := zap.NewNop()
			handler := NewHandler(cfg, mockStore, logger, nil)

			// Setup mocks
			tt.setupMocks()

			// Execute
			ctx := tt.setupContext()
			err := handler.HandleGetNotificationsLift(ctx)

			// Verify
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)

			if tt.checkResponse != nil {
				tt.checkResponse(t, ctx)
			}

			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandleGetInstanceV2Lift(t *testing.T) {
	mockStore := new(MockStorageAdapter)
	cfg := &config.Config{
		Domain: "test.example.com",
	}
	logger := zap.NewNop()
	handler := NewHandler(cfg, mockStore, logger, nil)

	// Setup context
	req := &lift.Request{
		Request: &adapters.Request{
			Method: "GET",
			Path:   "/api/v2/instance",
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
		},
	}
	ctx := lift.NewContext(context.Background(), req)

	// Setup mocks
	mockStore.On("GetInstanceRules", mock.Anything).Return([]storage.InstanceRule{
		{ID: "1", Text: "Be respectful"},
		{ID: "2", Text: "No spam"},
	}, nil)
	mockStore.On("GetVAPIDKeys", mock.Anything).Return(&storage.VAPIDKeys{
		PublicKey:  "test-public-key",
		PrivateKey: "test-private-key",
	}, nil)
	mockStore.On("GetActiveUserCount", mock.Anything, 30).Return(int64(100), nil)

	// Execute
	err := handler.HandleGetInstanceV2Lift(ctx)

	// Verify
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, ctx.Response.StatusCode)

	var response map[string]any
	bodyBytes, ok := ctx.Response.Body.([]byte)
	assert.True(t, ok, "Response body should be []byte")
	err = json.Unmarshal(bodyBytes, &response)
	assert.NoError(t, err)
	assert.Equal(t, "test.example.com", response["domain"])
	assert.NotNil(t, response["rules"])

	mockStore.AssertExpectations(t)
}

func TestHandleGetNotificationLift(t *testing.T) {
	var mockStore *MockStorageAdapter

	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func()
		expectedStatus int
		expectError    bool
		checkResponse  func(t *testing.T, ctx *lift.Context)
	}{
		{
			name: "successful get single notification",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/notifications/notif123",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						PathParams: map[string]string{
							"id": "notif123",
						},
					},
				}
				return lift.NewContext(context.Background(), req)
			},
			setupMocks: func() {
				notification := &storage.Notification{
					ID:        "notif123",
					Type:      models.NotificationTypeMention,
					AccountID: "otheruser",
					Username:  "testuser",
					CreatedAt: time.Now(),
				}
				mockStore.On("GetNotification", mock.Anything, "notif123").Return(notification, nil)
				mockStore.On("GetActor", mock.Anything, "otheruser").Return(&activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   "https://test.example.com/users/otheruser",
						Type: "Person",
					},
					PreferredUsername: "otheruser",
					Name:              "Other User",
				}, nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				var notification models.Notification
				bodyBytes, ok := ctx.Response.Body.([]byte)
				assert.True(t, ok, "Response body should be []byte")
				err := json.Unmarshal(bodyBytes, &notification)
				assert.NoError(t, err)
				assert.Equal(t, "notif123", notification.ID)
				assert.Equal(t, models.NotificationTypeMention, notification.Type)
			},
		},
		{
			name: "notification not found",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/notifications/notfound",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						PathParams: map[string]string{
							"id": "notfound",
						},
					},
				}
				return lift.NewContext(context.Background(), req)
			},
			setupMocks: func() {
				mockStore.On("GetNotification", mock.Anything, "notfound").Return(nil, storage.ErrNotFound)
			},
			expectedStatus: http.StatusNotFound,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				var errorResponse map[string]string
				bodyBytes, ok := ctx.Response.Body.([]byte)
				assert.True(t, ok, "Response body should be []byte")
				err := json.Unmarshal(bodyBytes, &errorResponse)
				assert.NoError(t, err)
				assert.Equal(t, "notification not found", errorResponse["error"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockStore = new(MockStorageAdapter)
			cfg := &config.Config{
				Domain:    "test.example.com",
				JWTSecret: "test-secret",
			}
			logger := zap.NewNop()
			handler := NewHandler(cfg, mockStore, logger, nil)

			// Setup mocks
			tt.setupMocks()

			// Execute
			ctx := tt.setupContext()
			err := handler.HandleGetNotificationLift(ctx)

			// Verify
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)

			if tt.checkResponse != nil {
				tt.checkResponse(t, ctx)
			}

			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandleClearNotificationsLift(t *testing.T) {
	var mockStore *MockStorageAdapter

	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func()
		expectedStatus int
		expectError    bool
	}{
		{
			name: "successful clear notifications",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/notifications/clear",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
					},
				}
				return lift.NewContext(context.Background(), req)
			},
			setupMocks: func() {
				mockStore.On("ClearNotifications", mock.Anything, "testuser").Return(nil)
			},
			expectedStatus: http.StatusNoContent,
			expectError:    false,
		},
		{
			name: "unauthorized clear notifications",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/notifications/clear",
						Headers: map[string]string{
							"Content-Type": "application/json",
						},
					},
				}
				return lift.NewContext(context.Background(), req)
			},
			setupMocks:     func() {},
			expectedStatus: http.StatusUnauthorized,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockStore = new(MockStorageAdapter)
			cfg := &config.Config{
				Domain:    "test.example.com",
				JWTSecret: "test-secret",
			}
			logger := zap.NewNop()
			handler := NewHandler(cfg, mockStore, logger, nil)

			// Setup mocks
			tt.setupMocks()

			// Execute
			ctx := tt.setupContext()
			err := handler.HandleClearNotificationsLift(ctx)

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

func TestHandleDismissNotificationLift(t *testing.T) {
	var mockStore *MockStorageAdapter

	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func()
		expectedStatus int
		expectError    bool
	}{
		{
			name: "successful dismiss notification",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/notifications/notif123/dismiss",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						PathParams: map[string]string{
							"id": "notif123",
						},
					},
				}
				return lift.NewContext(context.Background(), req)
			},
			setupMocks: func() {
				notification := &storage.Notification{
					ID:       "notif123",
					Username: "testuser",
				}
				mockStore.On("GetNotification", mock.Anything, "notif123").Return(notification, nil)
				mockStore.On("DeleteNotification", mock.Anything, "notif123").Return(nil)
			},
			expectedStatus: http.StatusNoContent,
			expectError:    false,
		},
		{
			name: "dismiss notification not owned by user",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/notifications/notif123/dismiss",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						PathParams: map[string]string{
							"id": "notif123",
						},
					},
				}
				return lift.NewContext(context.Background(), req)
			},
			setupMocks: func() {
				notification := &storage.Notification{
					ID:       "notif123",
					Username: "otheruser", // Different user
				}
				mockStore.On("GetNotification", mock.Anything, "notif123").Return(notification, nil)
			},
			expectedStatus: http.StatusNotFound,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockStore = new(MockStorageAdapter)
			cfg := &config.Config{
				Domain:    "test.example.com",
				JWTSecret: "test-secret",
			}
			logger := zap.NewNop()
			handler := NewHandler(cfg, mockStore, logger, nil)

			// Setup mocks
			tt.setupMocks()

			// Execute
			ctx := tt.setupContext()
			err := handler.HandleDismissNotificationLift(ctx)

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

func TestHandleGetInstanceCostsLift(t *testing.T) {
	tests := []struct {
		name           string
		costTableName  string
		expectedStatus int
		checkResponse  func(t *testing.T, ctx *lift.Context)
	}{
		{
			name:           "cost tracking not configured",
			costTableName:  "", // Empty means not configured
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				var response map[string]any
				bodyBytes, ok := ctx.Response.Body.([]byte)
				assert.True(t, ok, "Response body should be []byte")
				err := json.Unmarshal(bodyBytes, &response)
				assert.NoError(t, err)
				assert.Equal(t, "Cost tracking not configured", response["error"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockStore := new(MockStorageAdapter)
			cfg := &config.Config{
				Domain: "test.example.com",
				Region: "us-east-1",
			}
			logger := zap.NewNop()
			handler := NewHandler(cfg, mockStore, logger, nil)

			// Set environment variable
			if tt.costTableName != "" {
				t.Setenv("COST_HISTORY_TABLE_NAME", tt.costTableName)
			}

			// Setup context
			req := &lift.Request{
				Request: &adapters.Request{
					Method: "GET",
					Path:   "/api/v1/instance/costs",
					Headers: map[string]string{
						"Content-Type": "application/json",
					},
				},
			}
			ctx := lift.NewContext(context.Background(), req)

			// Execute
			err := handler.HandleGetInstanceCostsLift(ctx)

			// Verify
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)

			if tt.checkResponse != nil {
				tt.checkResponse(t, ctx)
			}

			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandleGetInstanceConfigurationLift(t *testing.T) {
	mockStore := new(MockStorageAdapter)
	cfg := &config.Config{
		Domain: "test.example.com",
	}
	logger := zap.NewNop()
	handler := NewHandler(cfg, mockStore, logger, nil)

	// Setup context
	req := &lift.Request{
		Request: &adapters.Request{
			Method: "GET",
			Path:   "/api/v1/instance/configuration",
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
		},
	}
	ctx := lift.NewContext(context.Background(), req)

	// Execute
	err := handler.HandleGetInstanceConfigurationLift(ctx)

	// Verify
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, ctx.Response.StatusCode)

	var config map[string]any
	bodyBytes, ok := ctx.Response.Body.([]byte)
	assert.True(t, ok, "Response body should be []byte")
	err = json.Unmarshal(bodyBytes, &config)
	assert.NoError(t, err)

	// Check required configuration fields
	assert.Contains(t, config, "urls")
	assert.Contains(t, config, "accounts")
	assert.Contains(t, config, "statuses")
	assert.Contains(t, config, "media_attachments")
	assert.Contains(t, config, "polls")
	assert.Contains(t, config, "translation")

	// Check streaming URL
	urls := config["urls"].(map[string]any)
	assert.Equal(t, "wss://ws.test.example.com/v1", urls["streaming"])

	mockStore.AssertExpectations(t)
}

// Helper functions for testing

func createValidJWT(t *testing.T, username string, scopes []string) string {
	// Create a simple JWT for testing (not production quality)
	token := "Bearer test-token-" + username
	return token
}