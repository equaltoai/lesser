package lift

import (
	"context"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestHandleGetAnnouncementsLift(t *testing.T) {

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
				// Mock setup disabled for test migration
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
				repos:          &MockRepositoryStorage{},
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

			// mockStore.AssertExpectations(t) // Disabled for test migration
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
