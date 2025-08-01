package lift

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

func TestHandleGetAnnouncementsLift(t *testing.T) {
	now := time.Now()
	startsAt := now.Add(-time.Hour)
	endsAt := now.Add(time.Hour)

	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func(mockStore *MockStorageAdapter)
		expectedStatus int
		expectError    bool
		checkResponse  func(t *testing.T, ctx *lift.Context)
	}{
		{
			name: "successful retrieval with authenticated user",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/announcements",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
					},
				}
				return lift.NewContext(context.Background(), req)
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				// Mock announcements
				announcements := []*storage.Announcement{
					{
						ID:          "ann1",
						Content:     "<p>Test announcement</p>",
						Text:        "Test announcement",
						PublishedAt: now,
						UpdatedAt:   now,
						AllDay:      false,
						StartsAt:    &startsAt,
						EndsAt:      &endsAt,
						Reactions:   []storage.Reaction{},
					},
				}
				mockStore.On("GetAnnouncements", mock.Anything, true).Return(announcements, nil)

				// Mock dismissed announcements
				mockStore.On("GetDismissedAnnouncements", mock.Anything, "testuser").Return([]string{}, nil)

				// Mock reactions for announcement
				mockStore.On("GetAnnouncementReactions", mock.Anything, "ann1").Return(map[string][]string{
					"👍": {"testuser", "otheruser"},
				}, nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
			checkResponse: func(t *testing.T, ctx *lift.Context) {
				var announcements []models.Announcement
				bodyBytes, err := json.Marshal(ctx.Response.Body)
				assert.NoError(t, err)
				err = json.Unmarshal(bodyBytes, &announcements)
				assert.NoError(t, err)
				assert.Len(t, announcements, 1)
				assert.Equal(t, "ann1", announcements[0].ID)
				assert.Equal(t, "<p>Test announcement</p>", announcements[0].Content)
				assert.NotNil(t, announcements[0].StartsAt)
				assert.NotNil(t, announcements[0].EndsAt)
			},
		},
		{
			name: "successful retrieval without authentication",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/announcements",
					},
				}
				return lift.NewContext(context.Background(), req)
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				// Mock announcements
				announcements := []*storage.Announcement{
					{
						ID:          "ann1",
						Content:     "<p>Test announcement</p>",
						Text:        "Test announcement",
						PublishedAt: now,
						UpdatedAt:   now,
						AllDay:      true,
						Reactions:   []storage.Reaction{},
					},
				}
				mockStore.On("GetAnnouncements", mock.Anything, true).Return(announcements, nil)

				// Mock reactions for announcement
				mockStore.On("GetAnnouncementReactions", mock.Anything, "ann1").Return(map[string][]string{}, nil)
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "announcement retrieval failure",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/announcements",
					},
				}
				return lift.NewContext(context.Background(), req)
			},
			setupMocks: func(mockStore *MockStorageAdapter) {
				mockStore.On("GetAnnouncements", mock.Anything, true).Return(nil, assert.AnError)
			},
			expectedStatus: http.StatusInternalServerError,
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
				store:          mockStore,
				logger:         zap.NewNop(),
				authMiddleware: &auth.Middleware{},
			}

			ctx := tt.setupContext()
			err := handler.HandleGetAnnouncementsLift(ctx)

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

func TestHandleDismissAnnouncementLift(t *testing.T) {
	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func(mockStore *MockStorageAdapter)
		expectedStatus int
		expectError    bool
	}{
		{
			name: "unauthorized - no token",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method:     "POST",
						Path:       "/api/v1/announcements/ann1/dismiss",
						PathParams: map[string]string{"id": "ann1"},
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("id", "ann1")
				return ctx
			},
			setupMocks:     func(mockStore *MockStorageAdapter) {},
			expectedStatus: http.StatusUnauthorized,
			expectError:    false,
		},
		{
			name: "missing announcement ID",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/announcements//dismiss",
						Headers: map[string]string{
							"Authorization": "Bearer valid-token",
						},
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				// No param set - simulates missing ID
				return ctx
			},
			setupMocks:     func(mockStore *MockStorageAdapter) {},
			expectedStatus: http.StatusBadRequest,
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
				store:          mockStore,
				logger:         zap.NewNop(),
				authMiddleware: &auth.Middleware{},
			}

			ctx := tt.setupContext()
			err := handler.HandleDismissAnnouncementLift(ctx)

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

func TestHandleAddAnnouncementReactionLift(t *testing.T) {
	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func(mockStore *MockStorageAdapter)
		expectedStatus int
		expectError    bool
	}{
		{
			name: "unauthorized - no token",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method:     "PUT",
						Path:       "/api/v1/announcements/ann1/reactions/👍",
						PathParams: map[string]string{"id": "ann1", "name": "👍"},
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("id", "ann1")
				ctx.SetParam("name", "👍")
				return ctx
			},
			setupMocks:     func(mockStore *MockStorageAdapter) {},
			expectedStatus: http.StatusUnauthorized,
			expectError:    false,
		},
		{
			name: "missing parameters",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "PUT",
						Path:   "/api/v1/announcements//reactions/",
						Headers: map[string]string{
							"Authorization": "Bearer valid-token",
						},
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				// Missing params
				return ctx
			},
			setupMocks:     func(mockStore *MockStorageAdapter) {},
			expectedStatus: http.StatusBadRequest,
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
				store:          mockStore,
				logger:         zap.NewNop(),
				authMiddleware: &auth.Middleware{},
			}

			ctx := tt.setupContext()
			err := handler.HandleAddAnnouncementReactionLift(ctx)

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

func TestHandleRemoveAnnouncementReactionLift(t *testing.T) {
	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func(mockStore *MockStorageAdapter)
		expectedStatus int
		expectError    bool
	}{
		{
			name: "unauthorized - no token",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method:     "DELETE",
						Path:       "/api/v1/announcements/ann1/reactions/👍",
						PathParams: map[string]string{"id": "ann1", "name": "👍"},
					},
				}
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("id", "ann1")
				ctx.SetParam("name", "👍")
				return ctx
			},
			setupMocks:     func(mockStore *MockStorageAdapter) {},
			expectedStatus: http.StatusUnauthorized,
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
				store:          mockStore,
				logger:         zap.NewNop(),
				authMiddleware: &auth.Middleware{},
			}

			ctx := tt.setupContext()
			err := handler.HandleRemoveAnnouncementReactionLift(ctx)

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

func TestHandleCreateAnnouncementLift(t *testing.T) {
	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func(mockStore *MockStorageAdapter)
		expectedStatus int
		expectError    bool
	}{
		{
			name: "unauthorized - no token",
			setupContext: func() *lift.Context {
				body := `{"content":"<p>New announcement</p>","text":"New announcement","all_day":true}`
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/admin/announcements",
						Headers: map[string]string{
							"Content-Type": "application/json",
						},
					},
					Body: []byte(body),
				}
				return lift.NewContext(context.Background(), req)
			},
			setupMocks:     func(mockStore *MockStorageAdapter) {},
			expectedStatus: http.StatusUnauthorized,
			expectError:    false,
		},
		{
			name: "validation error - missing content",
			setupContext: func() *lift.Context {
				body := `{"text":"Text only","all_day":false}`
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/admin/announcements",
						Headers: map[string]string{
							"Content-Type": "application/json",
						},
					},
					Body: []byte(body),
				}
				return lift.NewContext(context.Background(), req)
			},
			setupMocks:     func(mockStore *MockStorageAdapter) {},
			expectedStatus: http.StatusUnauthorized, // Will fail auth first
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
				store:          mockStore,
				logger:         zap.NewNop(),
				authMiddleware: &auth.Middleware{},
			}

			ctx := tt.setupContext()
			err := handler.HandleCreateAnnouncementLift(ctx)

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

func TestConvertReactionsToAPILift(t *testing.T) {
	storageReactions := []storage.Reaction{
		{
			Name:      "👍",
			Count:     5,
			Me:        true,
			URL:       "",
			StaticURL: "",
		},
		{
			Name:      ":custom:",
			Count:     2,
			Me:        false,
			URL:       "https://example.com/custom.png",
			StaticURL: "https://example.com/custom_static.png",
		},
	}

	apiReactions := convertReactionsToAPILift(storageReactions)

	assert.Len(t, apiReactions, 2)
	assert.Equal(t, "👍", apiReactions[0].Name)
	assert.Equal(t, 5, apiReactions[0].Count)
	assert.Equal(t, true, apiReactions[0].Me)
	assert.Equal(t, "", apiReactions[0].URL)

	assert.Equal(t, ":custom:", apiReactions[1].Name)
	assert.Equal(t, 2, apiReactions[1].Count)
	assert.Equal(t, false, apiReactions[1].Me)
	assert.Equal(t, "https://example.com/custom.png", apiReactions[1].URL)
	assert.Equal(t, "https://example.com/custom_static.png", apiReactions[1].StaticURL)
}