package lift

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestHandleWebFingerLift(t *testing.T) {
	// Create mock storage adapter
// var mockStore *MockStorageAdapter // Disabled for test migration

	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func()
		expectedStatus int
		expectError    bool
	}{
		{
			name: "successful webfinger response",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/.well-known/webfinger",
						Headers: map[string]string{
							"User-Agent": "Mastodon/4.0.0",
						},
						QueryParams: map[string]string{
							"resource": "acct:testuser@example.com",
						},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				// Mock successful actor retrieval
				// actor := &activitypub.Actor{
// 					BaseObject: activitypub.BaseObject{
// 						ID:   "https://example.com/users/testuser",
// 						Type: "Person",
// 					},
// 					Name:              "Test User",
// 					PreferredUsername: "testuser",
// 					Icon: &activitypub.Image{
// 						BaseObject: activitypub.BaseObject{
// 							Type: "Image",
					// 	},
					// 	URL: "https://example.com/avatar.jpg",
					// },
				// }
				// mockStore.On("GetActor", mock.Anything, "testuser").Return(actor, nil)
			},
			expectedStatus: 200,
			expectError:    false,
		},
		{
			name: "missing resource parameter",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/.well-known/webfinger",
						Headers: map[string]string{
							"User-Agent": "Mastodon/4.0.0",
						},
						QueryParams: map[string]string{},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				// No mocks needed for this test
			},
			expectedStatus: 400,
			expectError:    false,
		},
		{
			name: "invalid resource format",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/.well-known/webfinger",
						Headers: map[string]string{
							"User-Agent": "Mastodon/4.0.0",
						},
						QueryParams: map[string]string{
							"resource": "invalid-format",
						},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				// No mocks needed for this test
			},
			expectedStatus: 400,
			expectError:    false,
		},
		{
			name: "wrong domain",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/.well-known/webfinger",
						Headers: map[string]string{
							"User-Agent": "Mastodon/4.0.0",
						},
						QueryParams: map[string]string{
							"resource": "acct:testuser@other-domain.com",
						},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				// No mocks needed for this test
			},
			expectedStatus: 404,
			expectError:    false,
		},
		{
			name: "actor not found",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/.well-known/webfinger",
						Headers: map[string]string{
							"User-Agent": "Mastodon/4.0.0",
						},
						QueryParams: map[string]string{
							"resource": "acct:nonexistent@example.com",
						},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				// Mock actor not found
				// mockStore.On("GetActor", mock.Anything, "nonexistent").Return(nil, assert.AnError)
			},
			expectedStatus: 404,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fresh mock for each test
			// mockStore = new(MockStorageAdapter) // Disabled for test migration
			
			// Setup mocks
			// tt.setupMocks() // Disabled for test migration
			
			// Create handler
			cfg := &config.Config{
				Domain: "example.com",
			}
			logger := zap.NewNop()
			authMiddleware := &auth.Middleware{}
			
			handler := NewHandler(cfg, &MockRepositoryStorage{}, logger, authMiddleware)
			
			// Setup context
			ctx := tt.setupContext()
			
			// Execute
			err := handler.HandleWebFingerLift(ctx)
			
			// Check error expectation
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			
			// Check status code
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
			
			// For successful test, check response structure
			if tt.expectedStatus == 200 {
				// Response should be JSON
				assert.Contains(t, ctx.Response.Headers["Content-Type"], "application/json")
				
				// Check response structure
				response, ok := ctx.Response.Body.(WebFingerResponse)
				if ok {
					assert.Equal(t, "acct:testuser@example.com", response.Subject)
					assert.Contains(t, response.Aliases, "https://example.com/users/testuser")
					assert.Len(t, response.Links, 4) // self, profile-page, updates-from, avatar
					
					// Check for self link
					selfLink := false
					avatarLink := false
					for _, link := range response.Links {
						if link.Rel == "self" && link.Type == "application/activity+json" {
							selfLink = true
						}
						if link.Rel == "http://webfinger.net/rel/avatar" {
							avatarLink = true
						}
					}
					assert.True(t, selfLink, "Should have self link")
					assert.True(t, avatarLink, "Should have avatar link")
				} else {
					assert.Fail(t, "Response body should be WebFingerResponse")
				}
			}
			
			// Verify all mocks were called as expected
			// mockStore.AssertExpectations(t) // Disabled for test migration
		})
	}
}

func TestParseWebFingerResourceLift(t *testing.T) {
	// Create handler
	cfg := &config.Config{Domain: "example.com"}
	logger := zap.NewNop()
	authMiddleware := &auth.Middleware{}
	// mockStore := new(MockStorageAdapter) // Disabled for test migration
	
	handler := NewHandler(cfg, &MockRepositoryStorage{}, logger, authMiddleware)
	
	tests := []struct {
		name           string
		resource       string
		expectedUser   string
		expectedDomain string
		expectError    bool
	}{
		{
			name:           "valid webfinger resource",
			resource:       "acct:testuser@example.com",
			expectedUser:   "testuser",
			expectedDomain: "example.com",
			expectError:    false,
		},
		{
			name:        "missing acct prefix",
			resource:    "testuser@example.com",
			expectError: true,
		},
		{
			name:        "invalid format - no @",
			resource:    "acct:testuser",
			expectError: true,
		},
		{
			name:        "empty username",
			resource:    "acct:@example.com",
			expectError: true,
		},
		{
			name:        "empty domain",
			resource:    "acct:testuser@",
			expectError: true,
		},
		{
			name:        "too many @ symbols",
			resource:    "acct:test@user@example.com",
			expectError: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			username, domain, err := handler.parseWebFingerResourceLift(tt.resource)
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedUser, username)
				assert.Equal(t, tt.expectedDomain, domain)
			}
		})
	}
}
