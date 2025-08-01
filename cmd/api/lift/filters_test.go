package lift

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/mastodon"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/golang-jwt/jwt/v5"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

func TestHandleGetFiltersLift(t *testing.T) {
	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func(*MockStorageAdapter)
		expectedStatus int
		expectError    bool
	}{
		{
			name: "successful filters retrieval with test mode",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v2/filters",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
					},
				}
				return lift.NewContext(context.Background(), req)
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				filters := []*storage.Filter{
					{
						ID:           "filter1",
						Username:     "testuser",
						Title:        "Test Filter",
						Context:      []string{"home", "public"},
						FilterAction: "warn",
					},
				}
				keywords := []*storage.FilterKeyword{
					{
						ID:        "keyword1",
						Keyword:   "spam",
						WholeWord: true,
					},
				}
				statuses := []*storage.FilterStatus{}

				mockStore.On("GetFiltersForUser", mock.Anything, "testuser").Return(filters, nil)
				mockStore.On("GetFilterKeywords", mock.Anything, "filter1").Return(keywords, nil)
				mockStore.On("GetFilterStatuses", mock.Anything, "filter1").Return(statuses, nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "successful filters retrieval with authorization",
			setupContext: func() *lift.Context {
				// Create a valid JWT token
				claims := &auth.Claims{
					Username: "testuser",
					Scopes:   []string{"read", "read:filters"},
					ClientID: "test-client",
					RegisteredClaims: jwt.RegisteredClaims{
						ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
						IssuedAt:  jwt.NewNumericDate(time.Now()),
					},
				}
				token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
				tokenString, _ := token.SignedString([]byte("test-secret"))
				
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v2/filters",
						Headers: map[string]string{
							"Authorization": "Bearer " + tokenString,
						},
					},
				}
				return lift.NewContext(context.Background(), req)
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				filters := []*storage.Filter{
					{
						ID:           "filter1",
						Username:     "testuser",
						Title:        "Test Filter",
						Context:      []string{"home"},
						FilterAction: "hide",
					},
				}
				keywords := []*storage.FilterKeyword{}
				statuses := []*storage.FilterStatus{}
				
				// Mock the filter retrieval
				mockStore.On("GetFiltersForUser", mock.Anything, "testuser").Return(filters, nil)
				mockStore.On("GetFilterKeywords", mock.Anything, "filter1").Return(keywords, nil)
				mockStore.On("GetFilterStatuses", mock.Anything, "filter1").Return(statuses, nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "unauthorized - missing token",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method:  "GET",
						Path:    "/api/v2/filters",
						Headers: map[string]string{},
					},
				}
				return lift.NewContext(context.Background(), req)
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				// No mocks needed for auth failure
			},
			expectedStatus: http.StatusUnauthorized,
			expectError:    false,
		},
		{
			name: "forbidden - insufficient scope",
			setupContext: func() *lift.Context {
				// Create a JWT token with wrong scope
				claims := &auth.Claims{
					Username: "testuser",
					Scopes:   []string{"write:filters"}, // Wrong scope - needs read:filters
					ClientID: "test-client",
					RegisteredClaims: jwt.RegisteredClaims{
						ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
						IssuedAt:  jwt.NewNumericDate(time.Now()),
					},
				}
				token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
				tokenString, _ := token.SignedString([]byte("test-secret"))
				
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v2/filters",
						Headers: map[string]string{
							"Authorization": "Bearer " + tokenString,
						},
					},
				}
				return lift.NewContext(context.Background(), req)
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				// No mocks needed - the request will be rejected at the scope check
			},
			expectedStatus: http.StatusForbidden,
			expectError:    false,
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
				converter: mastodon.NewConverter("https://test.example.com"),
			}

			ctx := tt.setupContext()
			err := handler.HandleGetFiltersLift(ctx)

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

func TestHandleGetFilterLift(t *testing.T) {
	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func(*MockStorageAdapter)
		expectedStatus int
		expectError    bool
	}{
		{
			name: "successful filter retrieval",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v2/filters/filter1",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						PathParams: map[string]string{"id": "filter1"},
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("id", "filter1")
				return ctx
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				filter := &storage.Filter{
					ID:           "filter1",
					Username:     "testuser",
					Title:        "Test Filter",
					Context:      []string{"home"},
					FilterAction: "warn",
				}
				keywords := []*storage.FilterKeyword{}
				statuses := []*storage.FilterStatus{}

				mockStore.On("GetFilter", mock.Anything, "filter1").Return(filter, nil)
				mockStore.On("GetFilterKeywords", mock.Anything, "filter1").Return(keywords, nil)
				mockStore.On("GetFilterStatuses", mock.Anything, "filter1").Return(statuses, nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "filter not found",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v2/filters/nonexistent",
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
			setupMocks: func(mockStore *MockStorageAdapter) {
				mockStore.On("GetFilter", mock.Anything, "nonexistent").Return(nil, nil)
			},
			expectedStatus: http.StatusNotFound,
			expectError:    false,
		},
		{
			name: "filter belongs to different user",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v2/filters/filter1",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						PathParams: map[string]string{"id": "filter1"},
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("id", "filter1")
				return ctx
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				filter := &storage.Filter{
					ID:           "filter1",
					Username:     "otheruser", // Different user
					Title:        "Test Filter",
					Context:      []string{"home"},
					FilterAction: "warn",
				}
				mockStore.On("GetFilter", mock.Anything, "filter1").Return(filter, nil)
			},
			expectedStatus: http.StatusNotFound,
			expectError:    false,
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
				converter: mastodon.NewConverter("https://test.example.com"),
			}

			ctx := tt.setupContext()
			err := handler.HandleGetFilterLift(ctx)

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

func TestHandleCreateFilterLift(t *testing.T) {
	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func(*MockStorageAdapter)
		expectedStatus int
		expectError    bool
	}{
		{
			name: "successful filter creation",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v2/filters",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
							"Content-Type":    "application/json",
						},
						Body: []byte(`{
							"title": "Test Filter",
							"context": ["home", "public"],
							"filter_action": "warn",
							"keywords_attributes": [
								{"keyword": "spam", "whole_word": true}
							]
						}`),
					},
				}
				return lift.NewContext(context.Background(), req)
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				mockStore.On("CreateFilter", mock.Anything, mock.MatchedBy(func(f *storage.Filter) bool {
					return f.Username == "testuser" && f.Title == "Test Filter" && f.FilterAction == "warn"
				})).Return(nil).Run(func(args mock.Arguments) {
					filter := args.Get(1).(*storage.Filter)
					filter.ID = "filter1" // Simulate generated ID
				})

				mockStore.On("AddFilterKeyword", mock.Anything, "filter1", mock.MatchedBy(func(kw *storage.FilterKeyword) bool {
					return kw.Keyword == "spam" && kw.WholeWord == true
				})).Return(nil).Run(func(args mock.Arguments) {
					keyword := args.Get(2).(*storage.FilterKeyword)
					keyword.ID = "keyword1" // Simulate generated ID
				})
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "invalid context",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v2/filters",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
							"Content-Type":    "application/json",
						},
						Body: []byte(`{
							"title": "Test Filter",
							"context": ["invalid_context"],
							"filter_action": "warn"
						}`),
					},
				}
				return lift.NewContext(context.Background(), req)
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				// No mocks needed for validation error
			},
			expectedStatus: http.StatusUnprocessableEntity,
			expectError:    false,
		},
		{
			name: "missing title",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v2/filters",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
							"Content-Type":    "application/json",
						},
						Body: []byte(`{
							"context": ["home"],
							"filter_action": "warn"
						}`),
					},
				}
				return lift.NewContext(context.Background(), req)
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				// No mocks needed for validation error
			},
			expectedStatus: http.StatusUnprocessableEntity,
			expectError:    false,
		},
		{
			name: "filter with expiration",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v2/filters",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
							"Content-Type":    "application/json",
						},
						Body: []byte(`{
							"title": "Temporary Filter",
							"context": ["home"],
							"filter_action": "hide",
							"expires_in": 3600
						}`),
					},
				}
				return lift.NewContext(context.Background(), req)
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				mockStore.On("CreateFilter", mock.Anything, mock.MatchedBy(func(f *storage.Filter) bool {
					return f.Username == "testuser" && 
						f.Title == "Temporary Filter" && 
						f.FilterAction == "hide" &&
						f.ExpiresAt != nil
				})).Return(nil).Run(func(args mock.Arguments) {
					filter := args.Get(1).(*storage.Filter)
					filter.ID = "filter1"
				})
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
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
				converter: mastodon.NewConverter("https://test.example.com"),
			}

			ctx := tt.setupContext()
			err := handler.HandleCreateFilterLift(ctx)

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

func TestHandleUpdateFilterLift(t *testing.T) {
	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func(*MockStorageAdapter)
		expectedStatus int
		expectError    bool
	}{
		{
			name: "successful filter update",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "PUT",
						Path:   "/api/v2/filters/filter1",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
							"Content-Type":    "application/json",
						},
						PathParams: map[string]string{"id": "filter1"},
						Body: []byte(`{
							"title": "Updated Filter",
							"filter_action": "hide"
						}`),
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("id", "filter1")
				return ctx
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				filter := &storage.Filter{
					ID:           "filter1",
					Username:     "testuser",
					Title:        "Test Filter",
					Context:      []string{"home"},
					FilterAction: "warn",
				}
				updatedFilter := &storage.Filter{
					ID:           "filter1",
					Username:     "testuser",
					Title:        "Updated Filter",
					Context:      []string{"home"},
					FilterAction: "hide",
				}

				mockStore.On("GetFilter", mock.Anything, "filter1").Return(filter, nil).Once()
				mockStore.On("UpdateFilter", mock.Anything, "filter1", mock.MatchedBy(func(updates map[string]any) bool {
					return updates["title"] == "Updated Filter" && updates["filter_action"] == "hide"
				})).Return(nil)
				mockStore.On("GetFilter", mock.Anything, "filter1").Return(updatedFilter, nil).Once()
				mockStore.On("GetFilterKeywords", mock.Anything, "filter1").Return([]*storage.FilterKeyword{}, nil)
				mockStore.On("GetFilterStatuses", mock.Anything, "filter1").Return([]*storage.FilterStatus{}, nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "filter not found for update",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "PUT",
						Path:   "/api/v2/filters/nonexistent",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
							"Content-Type":    "application/json",
						},
						PathParams: map[string]string{"id": "nonexistent"},
						Body: []byte(`{"title": "Updated Filter"}`),
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("id", "nonexistent")
				return ctx
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				mockStore.On("GetFilter", mock.Anything, "nonexistent").Return(nil, nil)
			},
			expectedStatus: http.StatusNotFound,
			expectError:    false,
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
				converter: mastodon.NewConverter("https://test.example.com"),
			}

			ctx := tt.setupContext()
			err := handler.HandleUpdateFilterLift(ctx)

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

func TestHandleDeleteFilterLift(t *testing.T) {
	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func(*MockStorageAdapter)
		expectedStatus int
		expectError    bool
	}{
		{
			name: "successful filter deletion",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "DELETE",
						Path:   "/api/v2/filters/filter1",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						PathParams: map[string]string{"id": "filter1"},
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("id", "filter1")
				return ctx
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				filter := &storage.Filter{
					ID:           "filter1",
					Username:     "testuser",
					Title:        "Test Filter",
					Context:      []string{"home"},
					FilterAction: "warn",
				}

				mockStore.On("GetFilter", mock.Anything, "filter1").Return(filter, nil)
				mockStore.On("DeleteFilter", mock.Anything, "filter1").Return(nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "filter not found for deletion",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "DELETE",
						Path:   "/api/v2/filters/nonexistent",
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
			setupMocks: func(mockStore *MockStorageAdapter) {
				mockStore.On("GetFilter", mock.Anything, "nonexistent").Return(nil, nil)
			},
			expectedStatus: http.StatusNotFound,
			expectError:    false,
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
				converter: mastodon.NewConverter("https://test.example.com"),
			}

			ctx := tt.setupContext()
			err := handler.HandleDeleteFilterLift(ctx)

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

func TestHandleGetFilterKeywordsLift(t *testing.T) {
	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func(*MockStorageAdapter)
		expectedStatus int
		expectError    bool
	}{
		{
			name: "successful filter keywords retrieval",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v2/filters/filter1/keywords",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						PathParams: map[string]string{"filter_id": "filter1"},
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("filter_id", "filter1")
				return ctx
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				filter := &storage.Filter{
					ID:           "filter1",
					Username:     "testuser",
					Title:        "Test Filter",
					Context:      []string{"home"},
					FilterAction: "warn",
				}
				keywords := []*storage.FilterKeyword{
					{
						ID:        "keyword1",
						Keyword:   "spam",
						WholeWord: true,
					},
					{
						ID:        "keyword2",
						Keyword:   "advertisement",
						WholeWord: false,
					},
				}

				mockStore.On("GetFilter", mock.Anything, "filter1").Return(filter, nil)
				mockStore.On("GetFilterKeywords", mock.Anything, "filter1").Return(keywords, nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "filter not found for keywords",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v2/filters/nonexistent/keywords",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						PathParams: map[string]string{"filter_id": "nonexistent"},
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("filter_id", "nonexistent")
				return ctx
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				mockStore.On("GetFilter", mock.Anything, "nonexistent").Return(nil, nil)
			},
			expectedStatus: http.StatusNotFound,
			expectError:    false,
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
				converter: mastodon.NewConverter("https://test.example.com"),
			}

			ctx := tt.setupContext()
			err := handler.HandleGetFilterKeywordsLift(ctx)

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

func TestHandleGetFilterStatusesLift(t *testing.T) {
	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func(*MockStorageAdapter)
		expectedStatus int
		expectError    bool
	}{
		{
			name: "successful filter statuses retrieval",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v2/filters/filter1/statuses",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						PathParams: map[string]string{"filter_id": "filter1"},
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("filter_id", "filter1")
				return ctx
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				filter := &storage.Filter{
					ID:           "filter1",
					Username:     "testuser",
					Title:        "Test Filter",
					Context:      []string{"home"},
					FilterAction: "warn",
				}
				statuses := []*storage.FilterStatus{
					{
						ID:       "status_filter1",
						StatusID: "status123",
					},
				}

				mockStore.On("GetFilter", mock.Anything, "filter1").Return(filter, nil)
				mockStore.On("GetFilterStatuses", mock.Anything, "filter1").Return(statuses, nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "filter not found for statuses",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v2/filters/nonexistent/statuses",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						PathParams: map[string]string{"filter_id": "nonexistent"},
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("filter_id", "nonexistent")
				return ctx
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				mockStore.On("GetFilter", mock.Anything, "nonexistent").Return(nil, nil)
			},
			expectedStatus: http.StatusNotFound,
			expectError:    false,
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
				converter: mastodon.NewConverter("https://test.example.com"),
			}

			ctx := tt.setupContext()
			err := handler.HandleGetFilterStatusesLift(ctx)

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

func TestHandleAddFilterKeywordLift(t *testing.T) {
	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func(*MockStorageAdapter)
		expectedStatus int
		expectError    bool
	}{
		{
			name: "successful keyword addition",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v2/filters/filter1/keywords",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
							"Content-Type":    "application/json",
						},
						PathParams: map[string]string{"filter_id": "filter1"},
						Body: []byte(`{
							"keyword": "spam",
							"whole_word": true
						}`),
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("filter_id", "filter1")
				return ctx
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				filter := &storage.Filter{
					ID:           "filter1",
					Username:     "testuser",
					Title:        "Test Filter",
					Context:      []string{"home"},
					FilterAction: "warn",
				}

				mockStore.On("GetFilter", mock.Anything, "filter1").Return(filter, nil)
				mockStore.On("AddFilterKeyword", mock.Anything, "filter1", mock.MatchedBy(func(kw *storage.FilterKeyword) bool {
					return kw.Keyword == "spam" && kw.WholeWord == true
				})).Return(nil).Run(func(args mock.Arguments) {
					keyword := args.Get(2).(*storage.FilterKeyword)
					keyword.ID = "keyword1" // Simulate generated ID
				})
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "missing keyword",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v2/filters/filter1/keywords",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
							"Content-Type":    "application/json",
						},
						PathParams: map[string]string{"filter_id": "filter1"},
						Body: []byte(`{
							"whole_word": true
						}`),
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("filter_id", "filter1")
				return ctx
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				filter := &storage.Filter{
					ID:           "filter1",
					Username:     "testuser",
					Title:        "Test Filter",
					Context:      []string{"home"},
					FilterAction: "warn",
				}

				mockStore.On("GetFilter", mock.Anything, "filter1").Return(filter, nil)
			},
			expectedStatus: http.StatusUnprocessableEntity,
			expectError:    false,
		},
		{
			name: "filter not found",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v2/filters/nonexistent/keywords",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
							"Content-Type":    "application/json",
						},
						PathParams: map[string]string{"filter_id": "nonexistent"},
						Body: []byte(`{
							"keyword": "spam",
							"whole_word": true
						}`),
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("filter_id", "nonexistent")
				return ctx
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				mockStore.On("GetFilter", mock.Anything, "nonexistent").Return(nil, nil)
			},
			expectedStatus: http.StatusNotFound,
			expectError:    false,
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
				converter: mastodon.NewConverter("https://test.example.com"),
			}

			ctx := tt.setupContext()
			err := handler.HandleAddFilterKeywordLift(ctx)

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
