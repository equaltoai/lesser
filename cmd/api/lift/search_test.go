package lift

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestHandleAccountSearchLift(t *testing.T) {
// var mockStore *MockStorageAdapter // Disabled for test migration

	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func()
		expectedStatus int
		expectError    bool
		expectedCount  int
	}{
		{
			name: "successful account search with results",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/accounts/search",
						Headers: map[string]string{
							"Content-Type": "application/json",
						},
						QueryParams: map[string]string{
							"q":      "test",
							"limit":  "20",
							"offset": "0",
						},
					},
				}
				return lift.NewContext(context.Background(), req)
			},
			setupMocks: func() {
				// mockStore.On("SearchAccounts", mock.Anything, "test", 20, false, 0).Return([]*activitypub.Actor{
// 					{
// 						BaseObject: activitypub.BaseObject{
// 							ID:   "https://test.example.com/users/testuser",
// 							Type: "Person",
// 						},
// 						PreferredUsername: "testuser",
// 						Name:              "Test User",
// 					},
// 				}, nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
			expectedCount:  1,
		},
		{
			name: "search with authentication and following filter",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/accounts/search",
						Headers: map[string]string{
							"Authorization":   "Bearer valid-token",
							"X-Test-Username": "testuser",
						},
						QueryParams: map[string]string{
							"q":         "test",
							"following": "true",
							"limit":     "10",
						},
					},
				}
				return lift.NewContext(context.Background(), req)
			},
			setupMocks: func() {
				// mockStore.On("SearchAccounts", mock.Anything, "test", 10, true, 0).Return([]*activitypub.Actor{
// 					{
// 						BaseObject: activitypub.BaseObject{
// 							ID:   "https://test.example.com/users/followeduser",
// 							Type: "Person",
// 						},
// 						PreferredUsername: "followeduser",
// 						Name:              "Followed User",
// 					},
// 				}, nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
			expectedCount:  1,
		},
		{
			name: "search without query parameter",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/accounts/search",
						Headers: map[string]string{
							"Content-Type": "application/json",
						},
						QueryParams: map[string]string{},
					},
				}
				return lift.NewContext(context.Background(), req)
			},
			setupMocks: func() {
				// No mocking needed for this test
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    false,
		},
		{
			name: "following filter without authentication",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/accounts/search",
						Headers: map[string]string{
							"Content-Type": "application/json",
						},
						QueryParams: map[string]string{
							"q":         "test",
							"following": "true",
						},
					},
				}
				return lift.NewContext(context.Background(), req)
			},
			setupMocks: func() {
				// No mocking needed for this test
			},
			expectedStatus: http.StatusUnauthorized,
			expectError:    false,
		},
		{
			name: "search with test username header",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/accounts/search",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						QueryParams: map[string]string{
							"q":     "test",
							"limit": "5",
						},
					},
				}
				return lift.NewContext(context.Background(), req)
			},
			setupMocks: func() {
				// mockStore.On("SearchAccounts", mock.Anything, "test", 5, false, 0).Return([]*activitypub.Actor{
// 					{
// 						BaseObject: activitypub.BaseObject{
// 							ID:   "https://test.example.com/users/anotheruser",
// 							Type: "Person",
// 						},
// 						PreferredUsername: "anotheruser",
// 						Name:              "Another User",
// 					},
// 				}, nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
			expectedCount:  1,
		},
		{
			name: "search with invalid limit (too high)",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/accounts/search",
						Headers: map[string]string{
							"Content-Type": "application/json",
						},
						QueryParams: map[string]string{
							"q":     "test",
							"limit": "100", // Should be capped at 80
						},
					},
				}
				return lift.NewContext(context.Background(), req)
			},
			setupMocks: func() {
				// Should call with limit=80 (capped)
				// mockStore.On("SearchAccounts", mock.Anything, "test", 80, false, 0).Return([]*activitypub.Actor{}, nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
			expectedCount:  0,
		},
		{
			name: "search with storage error",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/accounts/search",
						Headers: map[string]string{
							"Content-Type": "application/json",
						},
						QueryParams: map[string]string{
							"q": "test",
						},
					},
				}
				return lift.NewContext(context.Background(), req)
			},
			setupMocks: func() {
				// mockStore.On("SearchAccounts", mock.Anything, "test", 40, false, 0).Return(nil, assert.AnError)
			},
			expectedStatus: http.StatusInternalServerError,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mocks
			// mockStore = new(MockStorageAdapter) // Disabled for test migration
			// tt.setupMocks() // Disabled for test migration

			handler := &Handler{
				cfg: &config.Config{
					JWTSecret: "test-secret",
					Domain:    "test.example.com",
				},
				repos:  &MockRepositoryStorage{},
				logger: zap.NewNop(),
				authMiddleware: &auth.Middleware{},
			}

			ctx := tt.setupContext()
			err := handler.HandleAccountSearchLift(ctx)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)

			// Check response body for successful requests
			if !tt.expectError && tt.expectedStatus == http.StatusOK {
				var response []models.Account
				bodyBytes, err := json.Marshal(ctx.Response.Body)
				assert.NoError(t, err)
				err = json.Unmarshal(bodyBytes, &response)
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedCount, len(response))
			}

			// mockStore.AssertExpectations(t) // Disabled for test migration
		})
	}
}

func TestHandleGetSearchSuggestionsLift(t *testing.T) {
// var mockStore *MockStorageAdapter // Disabled for test migration

	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func()
		expectedStatus int
		expectError    bool
		expectedCount  int
	}{
		{
			name: "successful suggestions with results",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/accounts/search/suggestions",
						Headers: map[string]string{
							"Content-Type": "application/json",
						},
						QueryParams: map[string]string{
							"q": "te",
						},
					},
				}
				return lift.NewContext(context.Background(), req)
			},
			setupMocks: func() {
				// mockStore.On("GetSearchSuggestions", mock.Anything, "te").Return([]storage.SearchSuggestion{
// 					{
// 						Type:  "account",
// 						Value: "testuser",
// 						Score: 95,
// 					},
// 					{
// 						Type:  "account",
// 						Value: "techuser",
// 						Score: 85,
// 					},
// 				}, nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
			expectedCount:  2,
		},
		{
			name: "short prefix returns empty array",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/accounts/search/suggestions",
						Headers: map[string]string{
							"Content-Type": "application/json",
						},
						QueryParams: map[string]string{
							"q": "t", // Only 1 character
						},
					},
				}
				return lift.NewContext(context.Background(), req)
			},
			setupMocks: func() {
				// No mocking needed - should return early
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
			expectedCount:  0,
		},
		{
			name: "empty prefix returns empty array",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/accounts/search/suggestions",
						Headers: map[string]string{
							"Content-Type": "application/json",
						},
						QueryParams: map[string]string{
							"q": "", // Empty
						},
					},
				}
				return lift.NewContext(context.Background(), req)
			},
			setupMocks: func() {
				// No mocking needed - should return early
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
			expectedCount:  0,
		},
		{
			name: "storage error",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/accounts/search/suggestions",
						Headers: map[string]string{
							"Content-Type": "application/json",
						},
						QueryParams: map[string]string{
							"q": "test",
						},
					},
				}
				return lift.NewContext(context.Background(), req)
			},
			setupMocks: func() {
				// mockStore.On("GetSearchSuggestions", mock.Anything, "test").Return(nil, assert.AnError)
			},
			expectedStatus: http.StatusInternalServerError,
			expectError:    false,
		},
		{
			name: "no suggestions found",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/accounts/search/suggestions",
						Headers: map[string]string{
							"Content-Type": "application/json",
						},
						QueryParams: map[string]string{
							"q": "xyz",
						},
					},
				}
				return lift.NewContext(context.Background(), req)
			},
			setupMocks: func() {
				// mockStore.On("GetSearchSuggestions", mock.Anything, "xyz").Return([]storage.SearchSuggestion{}, nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
			expectedCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mocks
			// mockStore = new(MockStorageAdapter) // Disabled for test migration
			// tt.setupMocks() // Disabled for test migration

			handler := &Handler{
				cfg: &config.Config{
					JWTSecret: "test-secret",
					Domain:    "test.example.com",
				},
				repos:  &MockRepositoryStorage{},
				logger: zap.NewNop(),
				authMiddleware: &auth.Middleware{},
			}

			ctx := tt.setupContext()
			err := handler.HandleGetSearchSuggestionsLift(ctx)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)

			// Check response body for successful requests
			if !tt.expectError && tt.expectedStatus == http.StatusOK {
				var response []map[string]any
				bodyBytes, err := json.Marshal(ctx.Response.Body)
				assert.NoError(t, err)
				err = json.Unmarshal(bodyBytes, &response)
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedCount, len(response))

				// Validate structure for non-empty responses
				if tt.expectedCount > 0 {
					for _, item := range response {
						assert.Contains(t, item, "type")
						assert.Contains(t, item, "value")
						assert.Contains(t, item, "score")
					}
				}
			}

			// mockStore.AssertExpectations(t) // Disabled for test migration
		})
	}
}

func TestIsValidHandle(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "valid federated handle with @user@domain",
			input:    "@user@example.com",
			expected: true,
		},
		{
			name:     "federated handle without leading @ (not considered valid)",
			input:    "user@example.com",
			expected: false,
		},
		{
			name:     "valid handle with subdomain",
			input:    "@user@social.example.com",
			expected: true,
		},
		{
			name:     "too short",
			input:    "@u@d",
			expected: false,
		},
		{
			name:     "no @ symbols",
			input:    "username",
			expected: false,
		},
		{
			name:     "only one @ symbol (not at start)",
			input:    "user@domain",
			expected: false,
		},
		{
			name:     "too many @ symbols",
			input:    "@user@domain@extra",
			expected: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "only @ symbols",
			input:    "@@",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidHandle(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
