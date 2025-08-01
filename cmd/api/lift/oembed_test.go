package lift

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

func TestHandleOEmbedLift(t *testing.T) {
	publishedTime := time.Now()
	
	// Sample test objects
	testNote := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:        "https://example.com/objects/test-status-123",
			Type:      "Note",
			To:        []string{activitypub.PublicAddress},
			CC:        []string{"https://example.com/users/testuser/followers"},
			Published: &publishedTime,
			Summary:   "Test Status",
		},
		Content:      "<p>This is a test status for oEmbed testing</p>",
		AttributedTo: "https://example.com/users/testuser",
		Attachment: []activitypub.Attachment{
			{
				Type:      "Image",
				URL:       "https://example.com/media/image1.jpg",
				MediaType: "image/jpeg",
				Name:      "Test Image",
			},
		},
	}

	testActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/users/testuser",
			Type: "Person",
		},
		PreferredUsername: "testuser",
		Name:              "Test User",
		URL:               "https://example.com/users/testuser",
	}

	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func(mockStore *MockStorageAdapter)
		expectedStatus int
		expectError    bool
		checkResponse  func(t *testing.T, ctx *lift.Context)
	}{
		{
			name: "successful oembed json response",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/oembed",
						QueryParams: map[string]string{
							"url":    "https://example.com/web/@testuser/test-status-123",
							"format": "json",
						},
						Headers: map[string]string{
							"User-Agent": "Bot/1.0",
						},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				mockStore.On("GetObject", mock.Anything, "https://example.com/objects/test-status-123").Return(testNote, nil)
				mockStore.On("GetActor", mock.Anything, "testuser").Return(testActor, nil)
			},
			expectedStatus: 200,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				// Check that response was set
				assert.NotNil(t, ctx.Response)
				assert.Equal(t, 200, ctx.Response.StatusCode)
				
				// Parse response body
				var response OEmbedResponse
				bodyBytes, err := json.Marshal(ctx.Response.Body)
				assert.NoError(t, err)
				err = json.Unmarshal(bodyBytes, &response)
				assert.NoError(t, err)
				
				// Check oEmbed structure
				assert.Equal(t, "rich", response.Type)
				assert.Equal(t, "1.0", response.Version)
				assert.Equal(t, "Test User", response.AuthorName)
				assert.Equal(t, "https://example.com/users/testuser", response.AuthorURL)
				assert.Equal(t, "Lesser Instance", response.ProviderName)
				assert.Equal(t, "https://example.com", response.ProviderURL)
				assert.Equal(t, 86400, response.CacheAge)
				assert.Equal(t, 650, response.Width)
				assert.NotEmpty(t, response.HTML)
				assert.Equal(t, "Test Status", response.Title)
				assert.Equal(t, "https://example.com/media/image1.jpg", response.ThumbnailURL)
			},
		},
		{
			name: "successful oembed xml response",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/oembed",
						QueryParams: map[string]string{
							"url":    "https://example.com/@testuser/test-status-123",
							"format": "xml",
						},
						Headers: map[string]string{
							"User-Agent": "Bot/1.0",
						},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				mockStore.On("GetObject", mock.Anything, "https://example.com/objects/test-status-123").Return(testNote, nil)
				mockStore.On("GetActor", mock.Anything, "testuser").Return(testActor, nil)
			},
			expectedStatus: 200,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				// Check XML response
				assert.NotNil(t, ctx.Response)
				assert.Equal(t, 200, ctx.Response.StatusCode)
				assert.Contains(t, ctx.Response.Headers["Content-Type"], "text/xml")
				
				// Check XML content contains required elements
				xmlBody := ctx.Response.Body.(string)
				assert.Contains(t, xmlBody, "<?xml version=\"1.0\" encoding=\"utf-8\"?>")
				assert.Contains(t, xmlBody, "<type>rich</type>")
				assert.Contains(t, xmlBody, "<version>1.0</version>")
				assert.Contains(t, xmlBody, "<author_name>Test User</author_name>")
			},
		},
		{
			name: "missing url parameter",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/oembed",
						QueryParams: map[string]string{},
						Headers: map[string]string{
							"User-Agent": "Bot/1.0",
						},
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
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				assert.Equal(t, 400, ctx.Response.StatusCode)
				
				// Parse error response
				responseMap := ctx.Response.Body.(map[string]string)
				assert.Equal(t, "missing required parameter: url", responseMap["error"])
			},
		},
		{
			name: "invalid url parameter",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/oembed",
						QueryParams: map[string]string{
							"url": "not-a-valid-url",
						},
						Headers: map[string]string{
							"User-Agent": "Bot/1.0",
						},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				// No mocks needed
			},
			expectedStatus: 404,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				assert.Equal(t, 404, ctx.Response.StatusCode)
				
				responseMap := ctx.Response.Body.(map[string]string)
				assert.Equal(t, "URL does not belong to this instance", responseMap["error"])
			},
		},
		{
			name: "url from different host",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/oembed",
						QueryParams: map[string]string{
							"url": "https://other-instance.com/@user/123",
						},
						Headers: map[string]string{
							"User-Agent": "Bot/1.0",
						},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				// No mocks needed
			},
			expectedStatus: 404,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				assert.Equal(t, 404, ctx.Response.StatusCode)
				
				responseMap := ctx.Response.Body.(map[string]string)
				assert.Equal(t, "URL does not belong to this instance", responseMap["error"])
			},
		},
		{
			name: "status not found",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/oembed",
						QueryParams: map[string]string{
							"url": "https://example.com/@testuser/nonexistent",
						},
						Headers: map[string]string{
							"User-Agent": "Bot/1.0",
						},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				mockStore.On("GetObject", mock.Anything, "https://example.com/objects/nonexistent").Return(nil, fmt.Errorf("not found"))
			},
			expectedStatus: 404,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				assert.Equal(t, 404, ctx.Response.StatusCode)
				
				responseMap := ctx.Response.Body.(map[string]string)
				assert.Equal(t, "status not found", responseMap["error"])
			},
		},
		{
			name: "private status not embeddable",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/oembed",
						QueryParams: map[string]string{
							"url": "https://example.com/@testuser/private-status",
						},
						Headers: map[string]string{
							"User-Agent": "Bot/1.0",
						},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				privateNote := &activitypub.Note{
					BaseObject: activitypub.BaseObject{
						ID:        "https://example.com/objects/private-status",
						Type:      "Note",
						To:        []string{"https://example.com/users/testuser/followers"}, // No public address
						CC:        []string{},
						Published: &publishedTime,
					},
					Content:      "<p>This is a private status</p>",
					AttributedTo: "https://example.com/users/testuser",
				}
				mockStore.On("GetObject", mock.Anything, "https://example.com/objects/private-status").Return(privateNote, nil)
			},
			expectedStatus: 403,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				assert.Equal(t, 403, ctx.Response.StatusCode)
				
				responseMap := ctx.Response.Body.(map[string]string)
				assert.Equal(t, "status is not embeddable", responseMap["error"])
			},
		},
		{
			name: "custom maxwidth parameter",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/oembed",
						QueryParams: map[string]string{
							"url":      "https://example.com/objects/test-status-123",
							"maxwidth": "400",
						},
						Headers: map[string]string{
							"User-Agent": "Bot/1.0",
						},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				mockStore.On("GetObject", mock.Anything, "https://example.com/objects/test-status-123").Return(testNote, nil)
				mockStore.On("GetActor", mock.Anything, "testuser").Return(testActor, nil)
			},
			expectedStatus: 200,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				// Parse response body
				var response OEmbedResponse
				bodyBytes, err := json.Marshal(ctx.Response.Body)
				assert.NoError(t, err)
				err = json.Unmarshal(bodyBytes, &response)
				assert.NoError(t, err)
				
				// Check custom width
				assert.Equal(t, 400, response.Width)
			},
		},
		{
			name: "unsupported format",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/oembed",
						QueryParams: map[string]string{
							"url":    "https://example.com/@testuser/test-status-123",
							"format": "yaml",
						},
						Headers: map[string]string{
							"User-Agent": "Bot/1.0",
						},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				mockStore.On("GetObject", mock.Anything, "https://example.com/objects/test-status-123").Return(testNote, nil)
				mockStore.On("GetActor", mock.Anything, "testuser").Return(testActor, nil)
			},
			expectedStatus: 400,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				assert.Equal(t, 400, ctx.Response.StatusCode)
				
				responseMap := ctx.Response.Body.(map[string]string)
				assert.Equal(t, "unsupported format", responseMap["error"])
			},
		},
		{
			name: "direct object URL format",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/oembed",
						QueryParams: map[string]string{
							"url": "https://example.com/objects/direct-object-id",
						},
						Headers: map[string]string{
							"User-Agent": "Bot/1.0",
						},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				mockStore.On("GetObject", mock.Anything, "https://example.com/objects/direct-object-id").Return(testNote, nil)
				mockStore.On("GetActor", mock.Anything, "testuser").Return(testActor, nil)
			},
			expectedStatus: 200,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				assert.Equal(t, 200, ctx.Response.StatusCode)
				
				// Parse response body
				var response OEmbedResponse
				bodyBytes, err := json.Marshal(ctx.Response.Body)
				assert.NoError(t, err)
				err = json.Unmarshal(bodyBytes, &response)
				assert.NoError(t, err)
				
				assert.Equal(t, "rich", response.Type)
				assert.Equal(t, "Test User", response.AuthorName)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock storage adapter
			mockStore := new(MockStorageAdapter)
			if tt.setupMocks != nil {
				tt.setupMocks(mockStore)
			}
			
			// Create handler
			cfg := &config.Config{
				Domain: "example.com",
			}
			logger := zap.NewNop()
			authMiddleware := &auth.Middleware{}
			handler := NewHandler(cfg, mockStore, logger, authMiddleware)

			// Setup context
			ctx := tt.setupContext()

			// Execute handler
			err := handler.HandleOEmbedLift(ctx)

			// Check results
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				
				// Run custom response checks
				if tt.checkResponse != nil {
					tt.checkResponse(t, ctx)
				}
			}

			// Verify mocks
			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandleEmbedPageLift(t *testing.T) {
	publishedTime := time.Now()
	
	testNote := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:        "https://example.com/objects/embed-test-123",
			Type:      "Note",
			To:        []string{activitypub.PublicAddress},
			Published: &publishedTime,
		},
		Content:      "<p>This is a test status for embed display</p>",
		AttributedTo: "https://example.com/users/embeduser",
	}

	testActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/users/embeduser",
			Type: "Person",
		},
		PreferredUsername: "embeduser",
		Name:              "Embed User",
		URL:               "https://example.com/users/embeduser",
	}

	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func(mockStore *MockStorageAdapter)
		expectedStatus int
		expectError    bool
		checkResponse  func(t *testing.T, ctx *lift.Context)
	}{
		{
			name: "successful embed page",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/embed/embed-test-123",
						PathParams: map[string]string{
							"id": "embed-test-123",
						},
						Headers: map[string]string{
							"User-Agent": "Browser/1.0",
						},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				mockStore.On("GetObject", mock.Anything, "https://example.com/objects/embed-test-123").Return(testNote, nil)
				mockStore.On("GetActor", mock.Anything, "embeduser").Return(testActor, nil)
			},
			expectedStatus: 200,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				assert.Equal(t, 200, ctx.Response.StatusCode)
				assert.Contains(t, ctx.Response.Headers["Content-Type"], "text/html")
				assert.Equal(t, "ALLOWALL", ctx.Response.Headers["X-Frame-Options"])
				
				// Check HTML content
				htmlBody := ctx.Response.Body.(string)
				assert.Contains(t, htmlBody, "<!DOCTYPE html>")
				assert.Contains(t, htmlBody, "Embed User - Lesser Instance")
				assert.Contains(t, htmlBody, "This is a test status for embed display")
				assert.Contains(t, htmlBody, "@embeduser")
				assert.Contains(t, htmlBody, "sendHeight")
			},
		},
		{
			name: "missing status ID",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method:     "GET",
						Path:       "/embed/",
						PathParams: map[string]string{},
						Headers: map[string]string{
							"User-Agent": "Browser/1.0",
						},
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
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				assert.Equal(t, 400, ctx.Response.StatusCode)
				
				responseMap := ctx.Response.Body.(map[string]string)
				assert.Equal(t, "missing status ID", responseMap["error"])
			},
		},
		{
			name: "status not found for embed",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/embed/nonexistent",
						PathParams: map[string]string{
							"id": "nonexistent",
						},
						Headers: map[string]string{
							"User-Agent": "Browser/1.0",
						},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				mockStore.On("GetObject", mock.Anything, "https://example.com/objects/nonexistent").Return(nil, fmt.Errorf("not found"))
			},
			expectedStatus: 404,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				assert.Equal(t, 404, ctx.Response.StatusCode)
				
				responseMap := ctx.Response.Body.(map[string]string)
				assert.Equal(t, "status not found", responseMap["error"])
			},
		},
		{
			name: "private status not embeddable",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/embed/private-embed",
						PathParams: map[string]string{
							"id": "private-embed",
						},
						Headers: map[string]string{
							"User-Agent": "Browser/1.0",
						},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				privateNote := &activitypub.Note{
					BaseObject: activitypub.BaseObject{
						ID:        "https://example.com/objects/private-embed",
						Type:      "Note",
						To:        []string{"https://example.com/users/embeduser/followers"}, // No public address
						CC:        []string{},
						Published: &publishedTime,
					},
					Content:      "<p>This is a private status</p>",
					AttributedTo: "https://example.com/users/embeduser",
				}
				mockStore.On("GetObject", mock.Anything, "https://example.com/objects/private-embed").Return(privateNote, nil)
			},
			expectedStatus: 403,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				assert.Equal(t, 403, ctx.Response.StatusCode)
				
				responseMap := ctx.Response.Body.(map[string]string)
				assert.Equal(t, "status is not embeddable", responseMap["error"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock storage adapter
			mockStore := new(MockStorageAdapter)
			if tt.setupMocks != nil {
				tt.setupMocks(mockStore)
			}
			
			// Create handler
			cfg := &config.Config{
				Domain: "example.com",
			}
			logger := zap.NewNop()
			authMiddleware := &auth.Middleware{}
			handler := NewHandler(cfg, mockStore, logger, authMiddleware)

			// Setup context
			ctx := tt.setupContext()

			// Execute handler
			err := handler.HandleEmbedPageLift(ctx)

			// Check results
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				
				// Run custom response checks
				if tt.checkResponse != nil {
					tt.checkResponse(t, ctx)
				}
			}

			// Verify mocks
			mockStore.AssertExpectations(t)
		})
	}
}

func TestExtractStatusID(t *testing.T) {
	// Create a minimal handler for testing the helper method
	cfg := &config.Config{Domain: "example.com"}
	logger := zap.NewNop()
	authMiddleware := &auth.Middleware{}
	mockStore := new(MockStorageAdapter)
	handler := NewHandler(cfg, mockStore, logger, authMiddleware)

	tests := []struct {
		name     string
		urlPath  string
		expected string
	}{
		{
			name:     "web format with username",
			urlPath:  "/web/@testuser/status123",
			expected: "status123",
		},
		{
			name:     "direct username format",
			urlPath:  "/@testuser/status456",
			expected: "status456",
		},
		{
			name:     "users api format",
			urlPath:  "/users/testuser/statuses/status789",
			expected: "status789",
		},
		{
			name:     "direct objects format",
			urlPath:  "/objects/direct-object-id",
			expected: "direct-object-id",
		},
		{
			name:     "unknown format",
			urlPath:  "/some/other/path",
			expected: "",
		},
		{
			name:     "empty path",
			urlPath:  "",
			expected: "",
		},
		{
			name:     "incomplete web format",
			urlPath:  "/web/@testuser",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.extractStatusID(tt.urlPath)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateOEmbed(t *testing.T) {
	cfg := &config.Config{Domain: "example.com"}
	logger := zap.NewNop()
	authMiddleware := &auth.Middleware{}
	mockStore := new(MockStorageAdapter)
	handler := NewHandler(cfg, mockStore, logger, authMiddleware)

	publishedTime := time.Now()
	testNote := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:        "https://example.com/objects/test-123",
			Type:      "Note",
			Summary:   "Test Summary",
			To:        []string{activitypub.PublicAddress},
			Published: &publishedTime,
		},
		Content: "<p>Test content</p>",
		Attachment: []activitypub.Attachment{
			{
				Type:      "Image",
				URL:       "https://example.com/media/test.jpg",
				MediaType: "image/jpeg",
				Name:      "Test Image",
			},
		},
	}

	testActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID: "https://example.com/users/testuser",
		},
		Name: "Test User",
		URL:  "https://example.com/users/testuser",
	}

	result := handler.generateOEmbed(testNote, testActor, 500)

	assert.Equal(t, "rich", result.Type)
	assert.Equal(t, "1.0", result.Version)
	assert.Equal(t, "Test User", result.AuthorName)
	assert.Equal(t, "https://example.com/users/testuser", result.AuthorURL)
	assert.Equal(t, "Lesser Instance", result.ProviderName)
	assert.Equal(t, "https://example.com", result.ProviderURL)
	assert.Equal(t, 86400, result.CacheAge)
	assert.Equal(t, 500, result.Width)
	assert.NotNil(t, result.Height)
	assert.Equal(t, "Test Summary", result.Title)
	assert.Equal(t, "https://example.com/media/test.jpg", result.ThumbnailURL)
	assert.Contains(t, result.HTML, "iframe")
	assert.Contains(t, result.HTML, "test-123")
}