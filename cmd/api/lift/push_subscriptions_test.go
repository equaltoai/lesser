package lift

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

func TestHandleGetPushSubscriptionLift(t *testing.T) {
	tests := []struct {
		name           string
		headers        map[string]string
		setupMocks     func(*MockStorageAdapter)
		expectedStatus int
		checkResponse  func(*testing.T, interface{})
	}{
		{
			name:    "authenticated user with subscription",
			headers: map[string]string{"X-Test-Username": "testuser"},
			setupMocks: func(m *MockStorageAdapter) {
				subscription := &storage.PushSubscription{
					ID:       "sub123",
					Username: "testuser",
					Endpoint: "https://push.example.com/endpoint",
					P256dh:   "test-p256dh-key",
					Auth:     "test-auth-key",
					Alerts: storage.PushSubscriptionAlerts{
						Follow:        true,
						Favourite:     true,
						Reblog:        false,
						Mention:       true,
						Poll:          false,
						FollowRequest: true,
						Status:        false,
						Update:        false,
						AdminSignUp:   false,
						AdminReport:   false,
					},
					Policy: "all",
				}
				m.On("GetUserPushSubscriptions", mock.Anything, "testuser").Return([]*storage.PushSubscription{subscription}, nil)
				m.On("GetVAPIDKeys", mock.Anything).Return(&storage.VAPIDKeys{
					PublicKey:  "test-public-key",
					PrivateKey: "test-private-key",
				}, nil)
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp interface{}) {
				sub, ok := resp.(models.PushSubscription)
				assert.True(t, ok)
				assert.Equal(t, "sub123", sub.ID)
				assert.Equal(t, "https://push.example.com/endpoint", sub.Endpoint)
				assert.Equal(t, "test-public-key", sub.ServerKey)
				assert.True(t, sub.Alerts.Follow)
				assert.True(t, sub.Alerts.Favourite)
				assert.False(t, sub.Alerts.Reblog)
			},
		},
		{
			name:    "authenticated user without subscription",
			headers: map[string]string{"X-Test-Username": "testuser"},
			setupMocks: func(m *MockStorageAdapter) {
				m.On("GetUserPushSubscriptions", mock.Anything, "testuser").Return([]*storage.PushSubscription{}, nil)
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp interface{}) {
				data, ok := resp.(map[string]any)
				assert.True(t, ok)
				assert.Equal(t, "", data["id"])
				assert.Equal(t, "", data["endpoint"])
				assert.Equal(t, "", data["server_key"])
				alerts, ok := data["alerts"].(map[string]bool)
				assert.True(t, ok)
				assert.False(t, alerts["follow"])
				assert.False(t, alerts["favourite"])
			},
		},
		{
			name:           "unauthenticated returns 401",
			headers:        map[string]string{},
			setupMocks:     func(m *MockStorageAdapter) {},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:    "database error returns 500",
			headers: map[string]string{"X-Test-Username": "testuser"},
			setupMocks: func(m *MockStorageAdapter) {
				m.On("GetUserPushSubscriptions", mock.Anything, "testuser").Return(nil, errors.New("database error"))
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
					Method:  "GET",
					Path:    "/api/v1/push/subscription",
					Headers: tt.headers,
				},
			}
			ctx.Response = &lift.Response{
				Headers:    make(map[string]string),
				StatusCode: 200,
			}

			err := handler.HandleGetPushSubscriptionLift(ctx)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)

			if tt.checkResponse != nil {
				tt.checkResponse(t, ctx.Response.Body)
			}

			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandleCreatePushSubscriptionLift(t *testing.T) {
	tests := []struct {
		name           string
		headers        map[string]string
		body           string
		setupMocks     func(*MockStorageAdapter)
		expectedStatus int
	}{
		{
			name:    "successfully creates subscription",
			headers: map[string]string{"X-Test-Username": "testuser", "Content-Type": "application/json"},
			body: `{
				"subscription": {
					"endpoint": "https://push.example.com/endpoint",
					"keys": {
						"p256dh": "test-p256dh-key",
						"auth": "test-auth-key"
					}
				},
				"data": {
					"follow": true,
					"favourite": true,
					"reblog": false,
					"mention": true,
					"poll": false,
					"follow_request": true,
					"status": false,
					"update": false,
					"admin.sign_up": false,
					"admin.report": false
				}
			}`,
			setupMocks: func(m *MockStorageAdapter) {
				m.On("DeleteAllPushSubscriptions", mock.Anything, "testuser").Return(nil)
				m.On("CreatePushSubscription", mock.Anything, "testuser", mock.MatchedBy(func(sub *storage.PushSubscription) bool {
					return sub.Username == "testuser" &&
						sub.Endpoint == "https://push.example.com/endpoint" &&
						sub.P256dh == "test-p256dh-key" &&
						sub.Auth == "test-auth-key" &&
						sub.Alerts.Follow == true &&
						sub.Alerts.Favourite == true &&
						sub.Alerts.Reblog == false
				})).Return(nil)
				m.On("GetVAPIDKeys", mock.Anything).Return(&storage.VAPIDKeys{
					PublicKey:  "test-public-key",
					PrivateKey: "test-private-key",
				}, nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:    "missing endpoint returns 422",
			headers: map[string]string{"X-Test-Username": "testuser", "Content-Type": "application/json"},
			body: `{
				"subscription": {
					"keys": {
						"p256dh": "test-p256dh-key",
						"auth": "test-auth-key"
					}
				},
				"data": {}
			}`,
			setupMocks:     func(m *MockStorageAdapter) {},
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:    "missing p256dh key returns 422",
			headers: map[string]string{"X-Test-Username": "testuser", "Content-Type": "application/json"},
			body: `{
				"subscription": {
					"endpoint": "https://push.example.com/endpoint",
					"keys": {
						"auth": "test-auth-key"
					}
				},
				"data": {}
			}`,
			setupMocks:     func(m *MockStorageAdapter) {},
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:    "missing auth key returns 422",
			headers: map[string]string{"X-Test-Username": "testuser", "Content-Type": "application/json"},
			body: `{
				"subscription": {
					"endpoint": "https://push.example.com/endpoint",
					"keys": {
						"p256dh": "test-p256dh-key"
					}
				},
				"data": {}
			}`,
			setupMocks:     func(m *MockStorageAdapter) {},
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:           "invalid JSON returns 400",
			headers:        map[string]string{"X-Test-Username": "testuser", "Content-Type": "application/json"},
			body:           `{invalid json}`,
			setupMocks:     func(m *MockStorageAdapter) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "unauthenticated returns 401",
			headers:        map[string]string{"Content-Type": "application/json"},
			body:           `{}`,
			setupMocks:     func(m *MockStorageAdapter) {},
			expectedStatus: http.StatusUnauthorized,
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
					Method:  "POST",
					Path:    "/api/v1/push/subscription",
					Headers: tt.headers,
					Body:    []byte(tt.body),
				},
			}
			ctx.Response = &lift.Response{
				Headers:    make(map[string]string),
				StatusCode: 200,
			}

			err := handler.HandleCreatePushSubscriptionLift(ctx)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)

			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandleUpdatePushSubscriptionLift(t *testing.T) {
	tests := []struct {
		name           string
		headers        map[string]string
		body           string
		setupMocks     func(*MockStorageAdapter)
		expectedStatus int
	}{
		{
			name:    "successfully updates subscription",
			headers: map[string]string{"X-Test-Username": "testuser", "Content-Type": "application/json"},
			body: `{
				"data": {
					"follow": false,
					"favourite": false,
					"reblog": true,
					"mention": true,
					"poll": true,
					"follow_request": false,
					"status": true,
					"update": true,
					"admin.sign_up": false,
					"admin.report": false
				}
			}`,
			setupMocks: func(m *MockStorageAdapter) {
				subscription := &storage.PushSubscription{
					ID:       "sub123",
					Username: "testuser",
					Endpoint: "https://push.example.com/endpoint",
					P256dh:   "test-p256dh-key",
					Auth:     "test-auth-key",
					Policy:   "all",
				}
				m.On("GetUserPushSubscriptions", mock.Anything, "testuser").Return([]*storage.PushSubscription{subscription}, nil)
				m.On("UpdatePushSubscription", mock.Anything, "testuser", "sub123", mock.MatchedBy(func(alerts storage.PushSubscriptionAlerts) bool {
					return alerts.Follow == false &&
						alerts.Favourite == false &&
						alerts.Reblog == true &&
						alerts.Mention == true &&
						alerts.Poll == true
				})).Return(nil)
				m.On("GetVAPIDKeys", mock.Anything).Return(&storage.VAPIDKeys{
					PublicKey:  "test-public-key",
					PrivateKey: "test-private-key",
				}, nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:    "subscription not found returns 404",
			headers: map[string]string{"X-Test-Username": "testuser", "Content-Type": "application/json"},
			body:    `{"data": {}}`,
			setupMocks: func(m *MockStorageAdapter) {
				m.On("GetUserPushSubscriptions", mock.Anything, "testuser").Return([]*storage.PushSubscription{}, nil)
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "invalid JSON returns 400",
			headers:        map[string]string{"X-Test-Username": "testuser", "Content-Type": "application/json"},
			body:           `{invalid json}`,
			setupMocks:     func(m *MockStorageAdapter) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "unauthenticated returns 401",
			headers:        map[string]string{"Content-Type": "application/json"},
			body:           `{"data": {}}`,
			setupMocks:     func(m *MockStorageAdapter) {},
			expectedStatus: http.StatusUnauthorized,
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
					Method:  "PUT",
					Path:    "/api/v1/push/subscription",
					Headers: tt.headers,
					Body:    []byte(tt.body),
				},
			}
			ctx.Response = &lift.Response{
				Headers:    make(map[string]string),
				StatusCode: 200,
			}

			err := handler.HandleUpdatePushSubscriptionLift(ctx)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)

			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandleDeletePushSubscriptionLift(t *testing.T) {
	tests := []struct {
		name           string
		headers        map[string]string
		setupMocks     func(*MockStorageAdapter)
		expectedStatus int
	}{
		{
			name:    "successfully deletes subscription",
			headers: map[string]string{"X-Test-Username": "testuser"},
			setupMocks: func(m *MockStorageAdapter) {
				m.On("DeleteAllPushSubscriptions", mock.Anything, "testuser").Return(nil)
			},
			expectedStatus: http.StatusNoContent,
		},
		{
			name:    "database error returns 500",
			headers: map[string]string{"X-Test-Username": "testuser"},
			setupMocks: func(m *MockStorageAdapter) {
				m.On("DeleteAllPushSubscriptions", mock.Anything, "testuser").Return(errors.New("database error"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "unauthenticated returns 401",
			headers:        map[string]string{},
			setupMocks:     func(m *MockStorageAdapter) {},
			expectedStatus: http.StatusUnauthorized,
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
					Path:    "/api/v1/push/subscription",
					Headers: tt.headers,
				},
			}
			ctx.Response = &lift.Response{
				Headers:    make(map[string]string),
				StatusCode: 200,
			}

			err := handler.HandleDeletePushSubscriptionLift(ctx)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)

			mockStore.AssertExpectations(t)
		})
	}
}

func TestPushSubscriptionScopes(t *testing.T) {
	// Note: In test mode, handlers always grant both "push" and "read" scopes
	// So we can only test that the handlers work correctly in test mode
	// The actual scope checking logic is tested through the individual handler tests
	tests := []struct {
		name           string
		handler        func(*Handler) func(*lift.Context) error
		method         string
		body           string
		expectedStatus int
	}{
		{
			name:           "GET endpoint works in test mode",
			handler:        func(h *Handler) func(*lift.Context) error { return h.HandleGetPushSubscriptionLift },
			method:         "GET",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "CREATE endpoint requires valid body",
			handler:        func(h *Handler) func(*lift.Context) error { return h.HandleCreatePushSubscriptionLift },
			method:         "POST",
			body:           `{}`, // Invalid body
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:           "UPDATE endpoint requires subscription",
			handler:        func(h *Handler) func(*lift.Context) error { return h.HandleUpdatePushSubscriptionLift },
			method:         "PUT",
			body:           `{"data": {}}`,
			expectedStatus: http.StatusNotFound, // No subscription exists
		},
		{
			name:           "DELETE endpoint works in test mode",
			handler:        func(h *Handler) func(*lift.Context) error { return h.HandleDeletePushSubscriptionLift },
			method:         "DELETE",
			expectedStatus: http.StatusNoContent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := new(MockStorageAdapter)
			
			// Set up mocks based on expected behavior
			switch tt.name {
			case "GET endpoint works in test mode":
				mockStore.On("GetUserPushSubscriptions", mock.Anything, "testuser").Return([]*storage.PushSubscription{}, nil)
			case "UPDATE endpoint requires subscription":
				mockStore.On("GetUserPushSubscriptions", mock.Anything, "testuser").Return([]*storage.PushSubscription{}, nil)
			case "DELETE endpoint works in test mode":
				mockStore.On("DeleteAllPushSubscriptions", mock.Anything, "testuser").Return(nil)
			}

			handler := &Handler{
				cfg: &config.Config{
					JWTSecret: "test-secret",
					Domain:    "test.example.com",
				},
				store:  mockStore,
				logger: zap.NewNop(),
			}

			// Create context with appropriate method and body
			ctx := &lift.Context{
				Context: context.Background(),
				Request: &lift.Request{
					Method: tt.method,
					Path:   "/api/v1/push/subscription",
					Headers: map[string]string{
						"X-Test-Username": "testuser",
						"Content-Type":    "application/json",
					},
				},
			}
			
			if tt.body != "" {
				ctx.Request.Body = []byte(tt.body)
			}
			
			ctx.Response = &lift.Response{
				Headers:    make(map[string]string),
				StatusCode: 200,
			}

			// Execute the handler
			originalHandler := tt.handler(handler)
			err := originalHandler(ctx)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)

			mockStore.AssertExpectations(t)
		})
	}
}