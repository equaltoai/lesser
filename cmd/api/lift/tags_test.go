package lift

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

func TestHandleGetTagLift(t *testing.T) {
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
			name: "successful tag retrieval with stats",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/tags/golang",
						PathParams: map[string]string{
							"id": "golang",
						},
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				tagStats := &storage.HashtagStats{
					Name:          "golang",
					TotalUses:     150,
					TotalAccounts: 25,
					History: []storage.HashtagHistoryEntry{
						{
							Date:       time.Now().AddDate(0, 0, -1),
							UsageCount: 15,
							UserCount:  5,
						},
					},
				}
				mockStore.On("GetHashtagStats", mock.Anything, "golang").Return(tagStats, nil)
			},
			expectedStatus: 200,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				// Check response is set
				assert.NotEmpty(t, ctx.Response.Body)
			},
		},
		{
			name: "tag retrieval with authentication and following status",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/tags/tech",
						Headers: map[string]string{
							"Authorization": "Bearer valid-token",
						},
						PathParams: map[string]string{
							"id": "tech",
						},
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				tagStats := &storage.HashtagStats{
					Name:          "tech",
					TotalUses:     200,
					TotalAccounts: 40,
					History:       []storage.HashtagHistoryEntry{},
				}
				mockStore.On("GetHashtagStats", mock.Anything, "tech").Return(tagStats, nil)
				mockStore.On("IsFollowingHashtag", mock.Anything, "testuser", "tech").Return(true, nil)
			},
			expectedStatus: 200,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				assert.NotEmpty(t, ctx.Response.Body)
			},
		},
		{
			name: "tag retrieval with empty stats fallback",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/tags/unknown",
						PathParams: map[string]string{
							"id": "unknown",
						},
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				mockStore.On("GetHashtagStats", mock.Anything, "unknown").Return(nil, fmt.Errorf("not found"))
			},
			expectedStatus: 200,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				assert.NotEmpty(t, ctx.Response.Body)
			},
		},
		{
			name: "missing tag name parameter",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/tags/",
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				// No mocks needed for validation error
			},
			expectedStatus: 400,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fresh mock for each test
			mockStore = new(MockStorageAdapter)

			// Setup mocks
			tt.setupMocks()

			// Create handler
			cfg := &config.Config{}
			logger := zap.NewNop()
			authMiddleware := &auth.Middleware{}
			handler := NewHandler(cfg, mockStore, logger, authMiddleware)

			// Setup context
			ctx := tt.setupContext()

			// Call handler
			err := handler.HandleGetTagLift(ctx)

			// Verify
			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
			} else {
				assert.NoError(t, err)
				if tt.checkResponse != nil {
					tt.checkResponse(t, ctx)
				}
			}

			// Verify all mocks were called
			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandleFollowTagLift(t *testing.T) {
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
			name: "successful tag follow with test username",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/tags/golang/follow",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						PathParams: map[string]string{
							"id": "golang",
						},
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				mockStore.On("FollowHashtag", mock.Anything, "testuser", "golang").Return(nil)
			},
			expectedStatus: 200,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				assert.NotEmpty(t, ctx.Response.Body)
			},
		},
		{
			name: "successful tag follow with authorization",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/tags/tech/follow",
						Headers: map[string]string{
							"Authorization": "Bearer valid-token",
						},
						PathParams: map[string]string{
							"id": "tech",
						},
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				mockStore.On("FollowHashtag", mock.Anything, "testuser", "tech").Return(nil)
			},
			expectedStatus: 200,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				assert.NotEmpty(t, ctx.Response.Body)
			},
		},
		{
			name: "tag follow with normalization",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/tags/#GoLang/follow",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						PathParams: map[string]string{
							"id": "#GoLang",
						},
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				mockStore.On("FollowHashtag", mock.Anything, "testuser", "golang").Return(nil)
			},
			expectedStatus: 200,
			expectError:    false,
		},
		{
			name: "unauthorized follow attempt",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/tags/golang/follow",
						Headers: map[string]string{
							"Authorization": "Bearer invalid-token",
						},
						PathParams: map[string]string{
							"id": "golang",
						},
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				// No mocks needed for auth failure
			},
			expectedStatus: 401,
			expectError:    true,
		},
		{
			name: "follow hashtag storage error",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/tags/golang/follow",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						PathParams: map[string]string{
							"id": "golang",
						},
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				mockStore.On("FollowHashtag", mock.Anything, "testuser", "golang").Return(fmt.Errorf("storage error"))
			},
			expectedStatus: 500,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fresh mock for each test
			mockStore = new(MockStorageAdapter)

			// Setup mocks
			tt.setupMocks()

			// Create handler
			cfg := &config.Config{}
			logger := zap.NewNop()
			authMiddleware := &auth.Middleware{}
			handler := NewHandler(cfg, mockStore, logger, authMiddleware)

			// Setup context
			ctx := tt.setupContext()

			// Call handler
			err := handler.HandleFollowTagLift(ctx)

			// Verify
			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
			} else {
				assert.NoError(t, err)
				if tt.checkResponse != nil {
					tt.checkResponse(t, ctx)
				}
			}

			// Verify all mocks were called
			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandleUnfollowTagLift(t *testing.T) {
	var mockStore *MockStorageAdapter

	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func()
		expectedStatus int
		expectError    bool
	}{
		{
			name: "successful tag unfollow",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/tags/golang/unfollow",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						PathParams: map[string]string{
							"id": "golang",
						},
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				mockStore.On("UnfollowHashtag", mock.Anything, "testuser", "golang").Return(nil)
			},
			expectedStatus: 200,
			expectError:    false,
		},
		{
			name: "unfollow storage error",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/tags/golang/unfollow",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						PathParams: map[string]string{
							"id": "golang",
						},
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				mockStore.On("UnfollowHashtag", mock.Anything, "testuser", "golang").Return(fmt.Errorf("storage error"))
			},
			expectedStatus: 500,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fresh mock for each test
			mockStore = new(MockStorageAdapter)

			// Setup mocks
			tt.setupMocks()

			// Create handler
			cfg := &config.Config{}
			logger := zap.NewNop()
			authMiddleware := &auth.Middleware{}
			handler := NewHandler(cfg, mockStore, logger, authMiddleware)

			// Setup context
			ctx := tt.setupContext()

			// Call handler
			err := handler.HandleUnfollowTagLift(ctx)

			// Verify
			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
			} else {
				assert.NoError(t, err)
			}

			// Verify all mocks were called
			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandleGetFollowedTagsLift(t *testing.T) {
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
			name: "successful followed tags retrieval",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/followed_tags",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						QueryParams: map[string]string{
							"limit": "50",
						},
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				hashtags := []string{"golang", "tech", "programming"}
				mockStore.On("GetFollowedHashtags", mock.Anything, "testuser", 50, "").Return(hashtags, "next-cursor", nil)
			},
			expectedStatus: 200,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				assert.NotEmpty(t, ctx.Response.Body)
				// Check Link header is set for pagination
				assert.Contains(t, ctx.Response.Headers, "Link")
			},
		},
		{
			name: "followed tags with default pagination",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/followed_tags",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				hashtags := []string{"golang", "tech"}
				mockStore.On("GetFollowedHashtags", mock.Anything, "testuser", 100, "").Return(hashtags, "", nil)
			},
			expectedStatus: 200,
			expectError:    false,
		},
		{
			name: "storage error",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/followed_tags",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				mockStore.On("GetFollowedHashtags", mock.Anything, "testuser", 100, "").Return(nil, "", fmt.Errorf("storage error"))
			},
			expectedStatus: 500,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fresh mock for each test
			mockStore = new(MockStorageAdapter)

			// Setup mocks
			tt.setupMocks()

			// Create handler
			cfg := &config.Config{}
			logger := zap.NewNop()
			authMiddleware := &auth.Middleware{}
			handler := NewHandler(cfg, mockStore, logger, authMiddleware)

			// Setup context
			ctx := tt.setupContext()

			// Call handler
			err := handler.HandleGetFollowedTagsLift(ctx)

			// Verify
			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
			} else {
				assert.NoError(t, err)
				if tt.checkResponse != nil {
					tt.checkResponse(t, ctx)
				}
			}

			// Verify all mocks were called
			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandleGetFeaturedTagsLift(t *testing.T) {
	var mockStore *MockStorageAdapter

	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func()
		expectedStatus int
		expectError    bool
	}{
		{
			name: "successful featured tags retrieval",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/featured_tags",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				featuredTags := []*storage.FeaturedTag{
					{
						ID:            "1",
						Name:          "golang",
						URL:           "https://example.com/tags/golang",
						StatusesCount: 10,
						LastStatusAt:  "2023-11-01T12:00:00Z",
					},
				}
				mockStore.On("GetFeaturedTags", mock.Anything, "testuser").Return(featuredTags, nil)
			},
			expectedStatus: 200,
			expectError:    false,
		},
		{
			name: "storage error",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/featured_tags",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				mockStore.On("GetFeaturedTags", mock.Anything, "testuser").Return(nil, fmt.Errorf("storage error"))
			},
			expectedStatus: 500,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fresh mock for each test
			mockStore = new(MockStorageAdapter)

			// Setup mocks
			tt.setupMocks()

			// Create handler
			cfg := &config.Config{}
			logger := zap.NewNop()
			authMiddleware := &auth.Middleware{}
			handler := NewHandler(cfg, mockStore, logger, authMiddleware)

			// Setup context
			ctx := tt.setupContext()

			// Call handler
			err := handler.HandleGetFeaturedTagsLift(ctx)

			// Verify
			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
			} else {
				assert.NoError(t, err)
			}

			// Verify all mocks were called
			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandleCreateFeaturedTagLift(t *testing.T) {
	var mockStore *MockStorageAdapter

	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func()
		expectedStatus int
		expectError    bool
	}{
		{
			name: "successful featured tag creation",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/featured_tags",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						Body: []byte(`{"name": "golang"}`),
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				featuredTag := &storage.FeaturedTag{
					ID:            "1",
					Name:          "golang",
					URL:           "https://example.com/tags/golang",
					StatusesCount: 0,
					LastStatusAt:  "",
				}
				mockStore.On("CreateFeaturedTag", mock.Anything, "testuser", "golang").Return(featuredTag, nil)
			},
			expectedStatus: 200,
			expectError:    false,
		},
		{
			name: "invalid request body",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/featured_tags",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						Body: []byte(`{"invalid": json}`),
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				// No mocks needed for validation error
			},
			expectedStatus: 400,
			expectError:    true,
		},
		{
			name: "duplicate featured tag",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/featured_tags",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						Body: []byte(`{"name": "golang"}`),
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				mockStore.On("CreateFeaturedTag", mock.Anything, "testuser", "golang").Return(nil, fmt.Errorf("item already exists"))
			},
			expectedStatus: 422,
			expectError:    true,
		},
		{
			name: "featured tag limit reached",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/featured_tags",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						Body: []byte(`{"name": "golang"}`),
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				mockStore.On("CreateFeaturedTag", mock.Anything, "testuser", "golang").Return(nil, fmt.Errorf("featured tag limit reached"))
			},
			expectedStatus: 422,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fresh mock for each test
			mockStore = new(MockStorageAdapter)

			// Setup mocks
			tt.setupMocks()

			// Create handler
			cfg := &config.Config{}
			logger := zap.NewNop()
			authMiddleware := &auth.Middleware{}
			handler := NewHandler(cfg, mockStore, logger, authMiddleware)

			// Setup context
			ctx := tt.setupContext()

			// Call handler
			err := handler.HandleCreateFeaturedTagLift(ctx)

			// Verify
			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
			} else {
				assert.NoError(t, err)
			}

			// Verify all mocks were called
			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandleDeleteFeaturedTagLift(t *testing.T) {
	var mockStore *MockStorageAdapter

	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func()
		expectedStatus int
		expectError    bool
	}{
		{
			name: "successful featured tag deletion",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "DELETE",
						Path:   "/api/v1/featured_tags/1",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						PathParams: map[string]string{
							"id": "1",
						},
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				mockStore.On("DeleteFeaturedTag", mock.Anything, "testuser", "1").Return(nil)
			},
			expectedStatus: 200,
			expectError:    false,
		},
		{
			name: "featured tag not found",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "DELETE",
						Path:   "/api/v1/featured_tags/999",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						PathParams: map[string]string{
							"id": "999",
						},
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				mockStore.On("DeleteFeaturedTag", mock.Anything, "testuser", "999").Return(fmt.Errorf("item not found"))
			},
			expectedStatus: 404,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fresh mock for each test
			mockStore = new(MockStorageAdapter)

			// Setup mocks
			tt.setupMocks()

			// Create handler
			cfg := &config.Config{}
			logger := zap.NewNop()
			authMiddleware := &auth.Middleware{}
			handler := NewHandler(cfg, mockStore, logger, authMiddleware)

			// Setup context
			ctx := tt.setupContext()

			// Call handler
			err := handler.HandleDeleteFeaturedTagLift(ctx)

			// Verify
			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
			} else {
				assert.NoError(t, err)
			}

			// Verify all mocks were called
			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandleGetFeaturedTagSuggestionsLift(t *testing.T) {
	var mockStore *MockStorageAdapter

	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func()
		expectedStatus int
		expectError    bool
	}{
		{
			name: "successful tag suggestions retrieval",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/featured_tags/suggestions",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				suggestions := []string{"golang", "programming", "tech"}
				mockStore.On("GetTagSuggestions", mock.Anything, "testuser", 10).Return(suggestions, nil)
			},
			expectedStatus: 200,
			expectError:    false,
		},
		{
			name: "storage error returns empty array",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/featured_tags/suggestions",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				mockStore.On("GetTagSuggestions", mock.Anything, "testuser", 10).Return(nil, fmt.Errorf("storage error"))
			},
			expectedStatus: 200,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fresh mock for each test
			mockStore = new(MockStorageAdapter)

			// Setup mocks
			tt.setupMocks()

			// Create handler
			cfg := &config.Config{}
			logger := zap.NewNop()
			authMiddleware := &auth.Middleware{}
			handler := NewHandler(cfg, mockStore, logger, authMiddleware)

			// Setup context
			ctx := tt.setupContext()

			// Call handler
			err := handler.HandleGetFeaturedTagSuggestionsLift(ctx)

			// Verify
			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
			} else {
				assert.NoError(t, err)
			}

			// Verify all mocks were called
			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandleGetAccountFeaturedTagsLift(t *testing.T) {
	var mockStore *MockStorageAdapter

	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func()
		expectedStatus int
		expectError    bool
	}{
		{
			name: "successful account featured tags retrieval",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/accounts/testuser/featured_tags",
						PathParams: map[string]string{
							"id": "testuser",
						},
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				featuredTags := []*storage.FeaturedTag{
					{
						ID:            "1",
						Name:          "golang",
						URL:           "https://example.com/tags/golang",
						StatusesCount: 5,
						LastStatusAt:  "2023-11-01T12:00:00Z",
					},
				}
				mockStore.On("GetFeaturedTags", mock.Anything, "testuser").Return(featuredTags, nil)
			},
			expectedStatus: 200,
			expectError:    false,
		},
		{
			name: "storage error returns empty array",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/accounts/testuser/featured_tags",
						PathParams: map[string]string{
							"id": "testuser",
						},
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				mockStore.On("GetFeaturedTags", mock.Anything, "testuser").Return(nil, fmt.Errorf("storage error"))
			},
			expectedStatus: 200,
			expectError:    false,
		},
		{
			name: "missing account ID parameter",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/accounts//featured_tags",
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				// No mocks needed for validation error
			},
			expectedStatus: 400,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fresh mock for each test
			mockStore = new(MockStorageAdapter)

			// Setup mocks
			tt.setupMocks()

			// Create handler
			cfg := &config.Config{}
			logger := zap.NewNop()
			authMiddleware := &auth.Middleware{}
			handler := NewHandler(cfg, mockStore, logger, authMiddleware)

			// Setup context
			ctx := tt.setupContext()

			// Call handler
			err := handler.HandleGetAccountFeaturedTagsLift(ctx)

			// Verify
			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
			} else {
				assert.NoError(t, err)
			}

			// Verify all mocks were called
			mockStore.AssertExpectations(t)
		})
	}
}