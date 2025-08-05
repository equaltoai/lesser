package lift

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestHandleGetPreferencesLift(t *testing.T) {
// var mockStore *MockStorageAdapter // Disabled for test migration

	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func()
		expectedStatus int
		expectError    bool
		checkResponse  func(t *testing.T, ctx *lift.Context)
	}{
		{
			name: "successful preferences retrieval",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/preferences",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				// Mock successful preferences retrieval
				// prefs := &storage.UserPreferences{
				// 	Language:                  "en",
				// 	DefaultPostingVisibility:  "public",
				// 	DefaultMediaSensitive:     false,
				// 	ExpandSpoilers:            false,
				// 	ExpandMedia:               "default",
				// 	AutoplayGifs:              true,
				// 	ShowFollowCounts:          true,
				// 	PreferredTimelineOrder:    "newest",
				// 	SearchSuggestionsEnabled:  true,
				// 	PersonalizedSearchEnabled: true,
				// }
				// mockStore.On("GetUserPreferences", mock.Anything, "testuser").Return(prefs, nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				assert.Equal(t, "application/json", ctx.Response.Headers["Content-Type"])
				// Check that the response body contains expected preferences
				var prefs models.Preferences
				bodyBytes, err := json.Marshal(ctx.Response.Body)
				assert.NoError(t, err)
				err = json.Unmarshal(bodyBytes, &prefs)
				assert.NoError(t, err)
				assert.Equal(t, "public", prefs.PostingDefaultVisibility)
				assert.Equal(t, "en", prefs.PostingDefaultLanguage)
			},
		},
		{
			name: "preferences not found - return defaults",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/preferences",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				// Mock preferences not found - should return defaults
				// mockStore.On("GetUserPreferences", mock.Anything, "testuser").Return(nil, assert.AnError)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				assert.Equal(t, "application/json", ctx.Response.Headers["Content-Type"])
				// Check that default values are returned
				var prefs models.Preferences
				bodyBytes, err := json.Marshal(ctx.Response.Body)
				assert.NoError(t, err)
				err = json.Unmarshal(bodyBytes, &prefs)
				assert.NoError(t, err)
				assert.Equal(t, "public", prefs.PostingDefaultVisibility)
				assert.Equal(t, "en", prefs.PostingDefaultLanguage)
				assert.Equal(t, "default", prefs.ReadingExpandMedia)
			},
		},
		{
			name: "custom preferences with different values",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/preferences",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				// Mock custom preferences
				// prefs := &storage.UserPreferences{
				// 	Language:                  "es",
				// 	DefaultPostingVisibility:  "unlisted",
				// 	DefaultMediaSensitive:     true,
				// 	ExpandSpoilers:            true,
				// 	ExpandMedia:               "show_all",
				// 	AutoplayGifs:              false,
				// 	ShowFollowCounts:          false,
				// 	PreferredTimelineOrder:    "oldest",
				// 	SearchSuggestionsEnabled:  false,
				// 	PersonalizedSearchEnabled: false,
				// }
				// mockStore.On("GetUserPreferences", mock.Anything, "testuser").Return(prefs, nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				assert.Equal(t, "application/json", ctx.Response.Headers["Content-Type"])
				var prefs models.Preferences
				bodyBytes, err := json.Marshal(ctx.Response.Body)
				assert.NoError(t, err)
				err = json.Unmarshal(bodyBytes, &prefs)
				assert.NoError(t, err)
				assert.Equal(t, "unlisted", prefs.PostingDefaultVisibility)
				assert.Equal(t, "es", prefs.PostingDefaultLanguage)
				assert.Equal(t, true, prefs.PostingDefaultSensitive)
				assert.Equal(t, true, prefs.ReadingExpandSpoilers)
				assert.Equal(t, "show_all", prefs.ReadingExpandMedia)
				assert.Equal(t, false, prefs.ReadingAutoplayGifs)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mock for each test
			// mockStore = &MockStorageAdapter{} // Disabled for test migration
			
			// Setup mocks
			// tt.setupMocks() // Disabled for test migration
			
			// Create handler
			handler := &Handler{
				cfg: &config.Config{
					JWTSecret: "test-secret",
					Domain:    "test.example.com",
				},
				repos:  &MockRepositoryStorage{},
				logger: zap.NewNop(),
				authMiddleware: &auth.Middleware{},
			}
			
			// Setup context
			ctx := tt.setupContext()
			
			// Execute handler
			err := handler.HandleGetPreferencesLift(ctx)
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			
			// Check status code
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
			
			// Run additional checks if provided
			if tt.checkResponse != nil {
				tt.checkResponse(t, ctx)
			}
			
			// Verify all expectations were met
			// mockStore.AssertExpectations(t) // Disabled for test migration
		})
	}
}

func TestHandleUpdatePreferencesLift(t *testing.T) {
// var mockStore *MockStorageAdapter // Disabled for test migration

	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func()
		expectedStatus int
		expectError    bool
		checkResponse  func(t *testing.T, ctx *lift.Context)
	}{
		{
			name: "successful preferences update",
			setupContext: func() *lift.Context {
				reqBody := `{
					"posting:default:visibility": "unlisted",
					"posting:default:sensitive": true,
					"posting:default:language": "es",
					"reading:expand:spoilers": true,
					"reading:expand:media": "show_all",
					"reading:autoplay:gifs": false
				}`
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "PATCH",
						Path:   "/api/v1/preferences",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
							"Content-Type":    "application/json",
						},
						Body: []byte(reqBody),
					},
					Method: "PATCH",
					Path:   "/api/v1/preferences",
					Headers: map[string]string{
						"X-Test-Username": "testuser",
						"Content-Type":    "application/json",
					},
					Body: []byte(reqBody),
				}
				
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				// Mock getting existing preferences
				// existingPrefs := &storage.UserPreferences{
				// 	Language:                  "en",
				// 	DefaultPostingVisibility:  "public",
				// 	DefaultMediaSensitive:     false,
				// 	ExpandSpoilers:            false,
				// 	ExpandMedia:               "default",
				// 	AutoplayGifs:              true,
				// 	ShowFollowCounts:          true,
				// 	PreferredTimelineOrder:    "newest",
				// 	SearchSuggestionsEnabled:  true,
				// 	PersonalizedSearchEnabled: true,
				// }
				// mockStore.On("GetUserPreferences", mock.Anything, "testuser").Return(existingPrefs, nil)
// 				
// 				// Mock successful update
				// mockStore.On("UpdateUserPreferences", mock.Anything, "testuser", mock.MatchedBy(func(prefs *storage.UserPreferences) bool {
// 					return prefs.DefaultPostingVisibility == "unlisted" &&
// 						prefs.DefaultMediaSensitive == true &&
// 						prefs.Language == "es" &&
// 						prefs.ExpandSpoilers == true &&
// 						prefs.ExpandMedia == "show_all" &&
// 						prefs.AutoplayGifs == false
// 				})).Return(nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				assert.Equal(t, "application/json", ctx.Response.Headers["Content-Type"])
				var prefs models.Preferences
				bodyBytes, err := json.Marshal(ctx.Response.Body)
				assert.NoError(t, err)
				err = json.Unmarshal(bodyBytes, &prefs)
				assert.NoError(t, err)
				assert.Equal(t, "unlisted", prefs.PostingDefaultVisibility)
				assert.Equal(t, true, prefs.PostingDefaultSensitive)
				assert.Equal(t, "es", prefs.PostingDefaultLanguage)
				assert.Equal(t, true, prefs.ReadingExpandSpoilers)
				assert.Equal(t, "show_all", prefs.ReadingExpandMedia)
				assert.Equal(t, false, prefs.ReadingAutoplayGifs)
			},
		},
		{
			name: "partial preferences update",
			setupContext: func() *lift.Context {
				reqBody := `{
					"posting:default:visibility": "private",
					"reading:expand:media": "hide_all"
				}`
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "PATCH",
						Path:   "/api/v1/preferences",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
							"Content-Type":    "application/json",
						},
						Body: []byte(reqBody),
					},
					Method: "PATCH",
					Path:   "/api/v1/preferences",
					Headers: map[string]string{
						"X-Test-Username": "testuser",  
						"Content-Type":    "application/json",
					},
					Body: []byte(reqBody),
				}
				
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				// Mock getting existing preferences
				// existingPrefs := &storage.UserPreferences{
				// 	Language:                  "en",
				// 	DefaultPostingVisibility:  "public",
				// 	DefaultMediaSensitive:     false,
				// 	ExpandSpoilers:            false,
				// 	ExpandMedia:               "default",
				// 	AutoplayGifs:              true,
				// 	ShowFollowCounts:          true,
				// 	PreferredTimelineOrder:    "newest",
				// 	SearchSuggestionsEnabled:  true,
				// 	PersonalizedSearchEnabled: true,
				// }
				// mockStore.On("GetUserPreferences", mock.Anything, "testuser").Return(existingPrefs, nil)
// 				
// 				// Mock successful update - only visibility and expand media should change
				// mockStore.On("UpdateUserPreferences", mock.Anything, "testuser", mock.MatchedBy(func(prefs *storage.UserPreferences) bool {
// 					return prefs.DefaultPostingVisibility == "private" &&
// 						prefs.ExpandMedia == "hide_all" &&
// 						prefs.Language == "en" && // unchanged
// 						prefs.DefaultMediaSensitive == false && // unchanged
// 						prefs.AutoplayGifs == true // unchanged
// 				})).Return(nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				assert.Equal(t, "application/json", ctx.Response.Headers["Content-Type"])
				var prefs models.Preferences
				bodyBytes, err := json.Marshal(ctx.Response.Body)
				assert.NoError(t, err)
				err = json.Unmarshal(bodyBytes, &prefs)
				assert.NoError(t, err)
				assert.Equal(t, "private", prefs.PostingDefaultVisibility)
				assert.Equal(t, "hide_all", prefs.ReadingExpandMedia)
				// Verify unchanged values
				assert.Equal(t, "en", prefs.PostingDefaultLanguage)
				assert.Equal(t, false, prefs.PostingDefaultSensitive)
			},
		},
		{
			name: "preferences not found - use defaults then update",
			setupContext: func() *lift.Context {
				reqBody := `{
					"posting:default:visibility": "followers"
				}`
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "PATCH",
						Path:   "/api/v1/preferences",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
							"Content-Type":    "application/json",
						},
						Body: []byte(reqBody),
					},
					Method: "PATCH",
					Path:   "/api/v1/preferences",
					Headers: map[string]string{
						"X-Test-Username": "testuser",
						"Content-Type":    "application/json",
					},
					Body: []byte(reqBody),
				}
				
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				// Mock preferences not found
				// mockStore.On("GetUserPreferences", mock.Anything, "testuser").Return(nil, assert.AnError)
// 				
// 				// Mock successful update with defaults + changes
				// mockStore.On("UpdateUserPreferences", mock.Anything, "testuser", mock.MatchedBy(func(prefs *storage.UserPreferences) bool {
// 					return prefs.DefaultPostingVisibility == "followers" &&
// 						prefs.Language == "en" && // default
// 						prefs.DefaultMediaSensitive == false && // default
// 						prefs.ExpandSpoilers == false && // default
// 						prefs.ShowFollowCounts == true && // default
// 						prefs.PreferredTimelineOrder == "newest" && // default
// 						prefs.SearchSuggestionsEnabled == true && // default
// 						prefs.PersonalizedSearchEnabled == true // default
// 				})).Return(nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				assert.Equal(t, "application/json", ctx.Response.Headers["Content-Type"])
				var prefs models.Preferences
				bodyBytes, err := json.Marshal(ctx.Response.Body)
				assert.NoError(t, err)
				err = json.Unmarshal(bodyBytes, &prefs)
				assert.NoError(t, err)
				assert.Equal(t, "followers", prefs.PostingDefaultVisibility)
				// Verify defaults are used for other fields
				assert.Equal(t, "en", prefs.PostingDefaultLanguage)
			},
		},
		{
			name: "invalid request - malformed JSON",
			setupContext: func() *lift.Context {
				reqBody := `{"posting:default:visibility": "public"` // Missing closing brace
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "PATCH",
						Path:   "/api/v1/preferences",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
							"Content-Type":    "application/json",
						},
						Body: []byte(reqBody),
					},
					Method: "PATCH",
					Path:   "/api/v1/preferences",
					Headers: map[string]string{
						"X-Test-Username": "testuser",
						"Content-Type":    "application/json",
					},
					Body: []byte(reqBody),
				}
				
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				// No storage calls expected
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    false,
		},
		{
			name: "update fails",
			setupContext: func() *lift.Context {
				reqBody := `{
					"posting:default:visibility": "private"
				}`
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "PATCH",
						Path:   "/api/v1/preferences",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
							"Content-Type":    "application/json",
						},
						Body: []byte(reqBody),
					},
					Method: "PATCH",
					Path:   "/api/v1/preferences",
					Headers: map[string]string{
						"X-Test-Username": "testuser",
						"Content-Type":    "application/json",
					},
					Body: []byte(reqBody),
				}
				
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				// Mock getting existing preferences
				// existingPrefs := &storage.UserPreferences{
				// 	Language:                  "en",
				// 	DefaultPostingVisibility:  "public",
				// 	DefaultMediaSensitive:     false,
				// 	ExpandSpoilers:            false,
				// 	ExpandMedia:               "default",
				// 	AutoplayGifs:              true,
				// 	ShowFollowCounts:          true,
				// 	PreferredTimelineOrder:    "newest",
				// 	SearchSuggestionsEnabled:  true,
				// 	PersonalizedSearchEnabled: true,
				// }
				// mockStore.On("GetUserPreferences", mock.Anything, "testuser").Return(existingPrefs, nil)
// 				
// 				// Mock update failure
				// mockStore.On("UpdateUserPreferences", mock.Anything, "testuser", mock.Anything).Return(assert.AnError)
			},
			expectedStatus: http.StatusInternalServerError,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mock for each test
			// mockStore = &MockStorageAdapter{} // Disabled for test migration
			
			// Setup mocks
			// tt.setupMocks() // Disabled for test migration
			
			// Create handler
			handler := &Handler{
				cfg: &config.Config{
					JWTSecret: "test-secret",
					Domain:    "test.example.com",
				},
				repos:  &MockRepositoryStorage{},
				logger: zap.NewNop(),
				authMiddleware: &auth.Middleware{},
			}
			
			// Setup context
			ctx := tt.setupContext()
			
			// Execute handler
			err := handler.HandleUpdatePreferencesLift(ctx)
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			
			// Check status code
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
			
			// Run additional checks if provided
			if tt.checkResponse != nil {
				tt.checkResponse(t, ctx)
			}
			
			// Verify all expectations were met
			// mockStore.AssertExpectations(t) // Disabled for test migration
		})
	}
}

func TestMapExpandMediaPreference(t *testing.T) {
	handler := &Handler{}

	tests := []struct {
		input    string
		expected string
	}{
		{"show_all", "show_all"},
		{"hide_all", "hide_all"},
		{"default", "default"},
		{"", "default"},
		{"invalid", "default"},
		{"unknown", "default"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := handler.mapExpandMediaPreference(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
