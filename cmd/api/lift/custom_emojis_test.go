package lift

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

func TestHandleGetCustomEmojisLift(t *testing.T) {
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
			name: "successful custom emojis retrieval",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/custom_emojis",
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				// Mock custom emojis (mix of visible and hidden)
				emojis := []*storage.CustomEmoji{
					{
						Shortcode:       "test_emoji",
						URL:             "https://example.com/emoji.png",
						StaticURL:       "https://example.com/emoji.png",
						VisibleInPicker: true,
						Category:        "custom",
						Domain:          "", // Local emoji
					},
					{
						Shortcode:       "hidden_emoji",
						URL:             "https://example.com/hidden.png",
						StaticURL:       "https://example.com/hidden.png",
						VisibleInPicker: false,
						Category:        "hidden",
						Domain:          "", // Local emoji - should be filtered out
					},
					{
						Shortcode:       "remote_emoji",
						URL:             "https://remote.com/emoji.png",
						StaticURL:       "https://remote.com/emoji.png",
						VisibleInPicker: false,
						Category:        "remote",
						Domain:          "remote.com", // Remote emoji - should be included
					},
				}
				mockStore.On("GetCustomEmojis", mock.Anything).Return(emojis, nil)
			},
			expectedStatus: 200,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				var response []models.CustomEmoji
				bodyBytes, err := json.Marshal(ctx.Response.Body)
				assert.NoError(t, err)
				err = json.Unmarshal(bodyBytes, &response)
				assert.NoError(t, err)
				assert.Len(t, response, 2) // Only visible local and remote emojis
				
				// Check first emoji (visible local)
				assert.Equal(t, "test_emoji", response[0].Shortcode)
				assert.Equal(t, "https://example.com/emoji.png", response[0].URL)
				assert.Equal(t, true, response[0].VisibleInPicker)
				
				// Check second emoji (remote, even if not visible in picker)
				assert.Equal(t, "remote_emoji", response[1].Shortcode)
				assert.Equal(t, "https://remote.com/emoji.png", response[1].URL)
			},
		},
		{
			name: "empty custom emojis list",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/custom_emojis",
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				mockStore.On("GetCustomEmojis", mock.Anything).Return([]*storage.CustomEmoji{}, nil)
			},
			expectedStatus: 200,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				var response []models.CustomEmoji
				bodyBytes, err := json.Marshal(ctx.Response.Body)
				assert.NoError(t, err)
				err = json.Unmarshal(bodyBytes, &response)
				assert.NoError(t, err)
				assert.Len(t, response, 0)
			},
		},
		{
			name: "storage error",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/custom_emojis",
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				mockStore.On("GetCustomEmojis", mock.Anything).Return(nil, assert.AnError)
			},
			expectedStatus: 500,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				var response map[string]string
				bodyBytes, err := json.Marshal(ctx.Response.Body)
				assert.NoError(t, err)
				err = json.Unmarshal(bodyBytes, &response)
				assert.NoError(t, err)
				assert.Equal(t, "internal server error", response["error"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock
			mockStore = new(MockStorageAdapter)
			tt.setupMocks()

			// Create handler
			cfg := &config.Config{}
			logger := zap.NewNop()
			handler := NewHandler(cfg, mockStore, logger, nil)

			// Setup context
			ctx := tt.setupContext()

			// Call handler
			err := handler.HandleGetCustomEmojisLift(ctx)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
				if tt.checkResponse != nil {
					tt.checkResponse(t, ctx)
				}
			}

			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandleCreateCustomEmojiLift(t *testing.T) {
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
			name: "successful custom emoji creation by admin",
			setupContext: func() *lift.Context {
				reqBody := models.CreateCustomEmojiRequest{
					Shortcode: "new_emoji",
					URL:       "https://example.com/new.png",
					StaticURL: "https://example.com/new.png",
					Category:  "custom",
				}
				bodyBytes, _ := json.Marshal(reqBody)

				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/admin/custom_emojis",
						Headers: map[string]string{
							"X-Test-Username": "admin_user",
						},
					},
					Body: bodyBytes,
				}
				
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				// Mock admin user
				adminUser := &storage.User{
					Username: "admin_user",
					Role:     "admin",
				}
				mockStore.On("GetUser", mock.Anything, "admin_user").Return(adminUser, nil)
				
				// Mock successful emoji creation
				mockStore.On("CreateCustomEmoji", mock.Anything, mock.MatchedBy(func(emoji *storage.CustomEmoji) bool {
					return emoji.Shortcode == "new_emoji" && emoji.URL == "https://example.com/new.png"
				})).Return(nil)
			},
			expectedStatus: 200,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				var response models.CustomEmoji
				bodyBytes, err := json.Marshal(ctx.Response.Body)
				assert.NoError(t, err)
				err = json.Unmarshal(bodyBytes, &response)
				assert.NoError(t, err)
				assert.Equal(t, "new_emoji", response.Shortcode)
				assert.Equal(t, "https://example.com/new.png", response.URL)
				assert.Equal(t, true, response.VisibleInPicker)
			},
		},
		{
			name: "create emoji with default static URL",
			setupContext: func() *lift.Context {
				reqBody := models.CreateCustomEmojiRequest{
					Shortcode: "no_static",
					URL:       "https://example.com/emoji.png",
					// StaticURL not provided
					Category: "custom",
				}
				bodyBytes, _ := json.Marshal(reqBody)

				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/admin/custom_emojis",
						Headers: map[string]string{
							"X-Test-Username": "admin_user",
						},
					},
					Body: bodyBytes,
				}
				
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				// Mock admin user
				adminUser := &storage.User{
					Username: "admin_user",
					Role:     "admin",
				}
				mockStore.On("GetUser", mock.Anything, "admin_user").Return(adminUser, nil)
				
				// Mock successful emoji creation with static URL defaulting to URL
				mockStore.On("CreateCustomEmoji", mock.Anything, mock.MatchedBy(func(emoji *storage.CustomEmoji) bool {
					return emoji.Shortcode == "no_static" && emoji.StaticURL == emoji.URL
				})).Return(nil)
			},
			expectedStatus: 200,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				var response models.CustomEmoji
				bodyBytes, err := json.Marshal(ctx.Response.Body)
				assert.NoError(t, err)
				err = json.Unmarshal(bodyBytes, &response)
				assert.NoError(t, err)
				assert.Equal(t, "no_static", response.Shortcode)
				assert.Equal(t, "https://example.com/emoji.png", response.StaticURL) // Should default to URL
			},
		},
		{
			name: "non-admin user forbidden",
			setupContext: func() *lift.Context {
				reqBody := models.CreateCustomEmojiRequest{
					Shortcode: "test_emoji",
					URL:       "https://example.com/test.png",
				}
				bodyBytes, _ := json.Marshal(reqBody)

				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/admin/custom_emojis",
						Headers: map[string]string{
							"X-Test-Username": "regular_user",
						},
					},
					Body: bodyBytes,
				}
				
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				// Mock regular user (not admin)
				regularUser := &storage.User{
					Username: "regular_user",
					Role:     "user",
				}
				mockStore.On("GetUser", mock.Anything, "regular_user").Return(regularUser, nil)
			},
			expectedStatus: 403,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				var response map[string]string
				bodyBytes, err := json.Marshal(ctx.Response.Body)
				assert.NoError(t, err)
				err = json.Unmarshal(bodyBytes, &response)
				assert.NoError(t, err)
				assert.Equal(t, "admin access required", response["error"])
			},
		},
		{
			name: "emoji already exists",
			setupContext: func() *lift.Context {
				reqBody := models.CreateCustomEmojiRequest{
					Shortcode: "existing_emoji",
					URL:       "https://example.com/existing.png",
				}
				bodyBytes, _ := json.Marshal(reqBody)

				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/admin/custom_emojis",
						Headers: map[string]string{
							"X-Test-Username": "admin_user",
						},
					},
					Body: bodyBytes,
				}
				
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				// Mock admin user
				adminUser := &storage.User{
					Username: "admin_user",
					Role:     "admin",
				}
				mockStore.On("GetUser", mock.Anything, "admin_user").Return(adminUser, nil)
				
				// Mock emoji already exists error
				mockStore.On("CreateCustomEmoji", mock.Anything, mock.Anything).Return(storage.ErrAlreadyExists)
			},
			expectedStatus: 422,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				var response map[string]string
				bodyBytes, err := json.Marshal(ctx.Response.Body)
				assert.NoError(t, err)
				err = json.Unmarshal(bodyBytes, &response)
				assert.NoError(t, err)
				assert.Contains(t, response["error"], "existing_emoji already exists")
			},
		},
		{
			name: "missing required fields",
			setupContext: func() *lift.Context {
				reqBody := models.CreateCustomEmojiRequest{
					// Missing shortcode and URL
					Category: "custom",
				}
				bodyBytes, _ := json.Marshal(reqBody)

				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/admin/custom_emojis",
						Headers: map[string]string{
							"X-Test-Username": "admin_user",
						},
					},
					Body: bodyBytes,
				}
				
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				// Mock admin user
				adminUser := &storage.User{
					Username: "admin_user",
					Role:     "admin",
				}
				mockStore.On("GetUser", mock.Anything, "admin_user").Return(adminUser, nil)
			},
			expectedStatus: 422,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				var response map[string]string
				bodyBytes, err := json.Marshal(ctx.Response.Body)
				assert.NoError(t, err)
				err = json.Unmarshal(bodyBytes, &response)
				assert.NoError(t, err)
				assert.Equal(t, "shortcode and url are required", response["error"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock
			mockStore = new(MockStorageAdapter)
			tt.setupMocks()

			// Create handler
			cfg := &config.Config{}
			logger := zap.NewNop()
			handler := NewHandler(cfg, mockStore, logger, nil)

			// Setup context
			ctx := tt.setupContext()

			// Call handler
			err := handler.HandleCreateCustomEmojiLift(ctx)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
				if tt.checkResponse != nil {
					tt.checkResponse(t, ctx)
				}
			}

			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandleUpdateCustomEmojiLift(t *testing.T) {
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
			name: "successful custom emoji update by admin",
			setupContext: func() *lift.Context {
				reqBody := models.UpdateCustomEmojiRequest{
					Category:        stringPtr("updated"),
					VisibleInPicker: boolPtr(false),
					Disabled:        boolPtr(true),
				}
				bodyBytes, _ := json.Marshal(reqBody)

				req := &lift.Request{
					Request: &adapters.Request{
						Method: "PUT",
						Path:   "/api/v1/admin/custom_emojis/test_emoji",
						Headers: map[string]string{
							"X-Test-Username": "admin_user",
						},
						PathParams: map[string]string{
							"shortcode": "test_emoji",
						},
					},
					Body: bodyBytes,
				}
				
				ctx := lift.NewContext(context.Background(), req)
				
				// Set up path parameters on the context
				ctx.SetParam("shortcode", "test_emoji")
				
				return ctx
			},
			setupMocks: func() {
				// Mock admin user
				adminUser := &storage.User{
					Username: "admin_user",
					Role:     "admin",
				}
				mockStore.On("GetUser", mock.Anything, "admin_user").Return(adminUser, nil)
				
				// Mock existing emoji
				existingEmoji := &storage.CustomEmoji{
					Shortcode:       "test_emoji",
					URL:             "https://example.com/test.png",
					StaticURL:       "https://example.com/test.png",
					VisibleInPicker: true,
					Category:        "old",
					Disabled:        false,
				}
				mockStore.On("GetCustomEmoji", mock.Anything, "test_emoji").Return(existingEmoji, nil)
				
				// Mock successful update
				mockStore.On("UpdateCustomEmoji", mock.Anything, mock.MatchedBy(func(emoji *storage.CustomEmoji) bool {
					return emoji.Category == "updated" && emoji.VisibleInPicker == false && emoji.Disabled == true
				})).Return(nil)
			},
			expectedStatus: 200,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				var response models.CustomEmoji
				bodyBytes, err := json.Marshal(ctx.Response.Body)
				assert.NoError(t, err)
				err = json.Unmarshal(bodyBytes, &response)
				assert.NoError(t, err)
				assert.Equal(t, "test_emoji", response.Shortcode)
				assert.Equal(t, "updated", response.Category)
				assert.Equal(t, false, response.VisibleInPicker)
			},
		},
		{
			name: "emoji not found",
			setupContext: func() *lift.Context {
				reqBody := models.UpdateCustomEmojiRequest{
					Category: stringPtr("updated"),
				}
				bodyBytes, _ := json.Marshal(reqBody)

				req := &lift.Request{
					Request: &adapters.Request{
						Method: "PUT",
						Path:   "/api/v1/admin/custom_emojis/nonexistent",
						Headers: map[string]string{
							"X-Test-Username": "admin_user",
						},
						PathParams: map[string]string{
							"shortcode": "nonexistent",
						},
					},
					Body: bodyBytes,
				}
				
				ctx := lift.NewContext(context.Background(), req)
				
				// Set up path parameters on the context
				ctx.SetParam("shortcode", "nonexistent")
				
				return ctx
			},
			setupMocks: func() {
				// Mock admin user
				adminUser := &storage.User{
					Username: "admin_user",
					Role:     "admin",
				}
				mockStore.On("GetUser", mock.Anything, "admin_user").Return(adminUser, nil)
				
				// Mock emoji not found
				mockStore.On("GetCustomEmoji", mock.Anything, "nonexistent").Return(nil, storage.ErrNotFound)
			},
			expectedStatus: 404,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				var response map[string]string
				bodyBytes, err := json.Marshal(ctx.Response.Body)
				assert.NoError(t, err)
				err = json.Unmarshal(bodyBytes, &response)
				assert.NoError(t, err)
				assert.Equal(t, "custom emoji not found", response["error"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock
			mockStore = new(MockStorageAdapter)
			tt.setupMocks()

			// Create handler
			cfg := &config.Config{}
			logger := zap.NewNop()
			handler := NewHandler(cfg, mockStore, logger, nil)

			// Setup context
			ctx := tt.setupContext()

			// Call handler
			err := handler.HandleUpdateCustomEmojiLift(ctx)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
				if tt.checkResponse != nil {
					tt.checkResponse(t, ctx)
				}
			}

			mockStore.AssertExpectations(t)
		})
	}
}

func TestHandleDeleteCustomEmojiLift(t *testing.T) {
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
			name: "successful custom emoji deletion by admin",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "DELETE",
						Path:   "/api/v1/admin/custom_emojis/test_emoji",
						Headers: map[string]string{
							"X-Test-Username": "admin_user",
						},
						PathParams: map[string]string{
							"shortcode": "test_emoji",
						},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				
				// Set up path parameters on the context
				ctx.SetParam("shortcode", "test_emoji")
				
				return ctx
			},
			setupMocks: func() {
				// Mock admin user
				adminUser := &storage.User{
					Username: "admin_user",
					Role:     "admin",
				}
				mockStore.On("GetUser", mock.Anything, "admin_user").Return(adminUser, nil)
				
				// Mock successful deletion
				mockStore.On("DeleteCustomEmoji", mock.Anything, "test_emoji").Return(nil)
			},
			expectedStatus: 200,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				var response map[string]any
				bodyBytes, err := json.Marshal(ctx.Response.Body)
				assert.NoError(t, err)
				err = json.Unmarshal(bodyBytes, &response)
				assert.NoError(t, err)
				assert.Empty(t, response) // Should return empty object
			},
		},
		{
			name: "emoji not found for deletion",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "DELETE",
						Path:   "/api/v1/admin/custom_emojis/nonexistent",
						Headers: map[string]string{
							"X-Test-Username": "admin_user",
						},
						PathParams: map[string]string{
							"shortcode": "nonexistent",
						},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				
				// Set up path parameters on the context
				ctx.SetParam("shortcode", "nonexistent")
				
				return ctx
			},
			setupMocks: func() {
				// Mock admin user
				adminUser := &storage.User{
					Username: "admin_user",
					Role:     "admin",
				}
				mockStore.On("GetUser", mock.Anything, "admin_user").Return(adminUser, nil)
				
				// Mock emoji not found
				mockStore.On("DeleteCustomEmoji", mock.Anything, "nonexistent").Return(storage.ErrNotFound)
			},
			expectedStatus: 404,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				var response map[string]string
				bodyBytes, err := json.Marshal(ctx.Response.Body)
				assert.NoError(t, err)
				err = json.Unmarshal(bodyBytes, &response)
				assert.NoError(t, err)
				assert.Equal(t, "custom emoji not found", response["error"])
			},
		},
		{
			name: "non-admin user forbidden for deletion",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "DELETE",
						Path:   "/api/v1/admin/custom_emojis/test_emoji",
						Headers: map[string]string{
							"X-Test-Username": "regular_user",
						},
						PathParams: map[string]string{
							"shortcode": "test_emoji",
						},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				
				// Set up path parameters on the context
				ctx.SetParam("shortcode", "test_emoji")
				
				return ctx
			},
			setupMocks: func() {
				// Mock regular user (not admin)
				regularUser := &storage.User{
					Username: "regular_user",
					Role:     "user",
				}
				mockStore.On("GetUser", mock.Anything, "regular_user").Return(regularUser, nil)
			},
			expectedStatus: 403,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				var response map[string]string
				bodyBytes, err := json.Marshal(ctx.Response.Body)
				assert.NoError(t, err)
				err = json.Unmarshal(bodyBytes, &response)
				assert.NoError(t, err)
				assert.Equal(t, "admin access required", response["error"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock
			mockStore = new(MockStorageAdapter)
			tt.setupMocks()

			// Create handler
			cfg := &config.Config{}
			logger := zap.NewNop()
			handler := NewHandler(cfg, mockStore, logger, nil)

			// Setup context
			ctx := tt.setupContext()

			// Call handler
			err := handler.HandleDeleteCustomEmojiLift(ctx)

			// Assert
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
				if tt.checkResponse != nil {
					tt.checkResponse(t, ctx)
				}
			}

			mockStore.AssertExpectations(t)
		})
	}
}

// Helper functions for pointer values
func stringPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}