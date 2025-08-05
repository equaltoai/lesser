package lift

import (
	"context"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestHandleGetConversationsLift(t *testing.T) {
	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func()
		expectedStatus int
		expectError    bool
	}{
		{
			name: "successful get conversations with test header",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/conversations",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				// Mock calls disabled for test migration
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "unauthorized - no token or test header",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "GET",
						Path:   "/api/v1/conversations",
						Headers: map[string]string{},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				// No mocks needed for unauthorized case
			},
			expectedStatus: http.StatusUnauthorized,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
			
			// Get context
			ctx := tt.setupContext()
			
			// Call handler directly
			err := handler.HandleGetConversationsLift(ctx)
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			
			// Check status
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
		})
	}
}

func TestHandleDeleteConversationLift(t *testing.T) {
	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func()
		expectedStatus int
		expectError    bool
	}{
		{
			name: "successful delete conversation with test header",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "DELETE",
						Path:   "/api/v1/conversations/conv1",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						PathParams: map[string]string{"id": "conv1"},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("id", "conv1")
				return ctx
			},
			setupMocks: func() {
				// Mock calls disabled for test migration
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "conversation not found",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "DELETE",
						Path:   "/api/v1/conversations/nonexistent",
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
			setupMocks: func() {
				// Mock calls disabled for test migration
			},
			expectedStatus: http.StatusNotFound,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
			
			// Get context
			ctx := tt.setupContext()
			
			// Call handler directly
			err := handler.HandleDeleteConversationLift(ctx)
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			
			// Check status
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
		})
	}
}

func TestHandleReadConversationLift(t *testing.T) {
	tests := []struct {
		name           string
		setupContext   func() *lift.Context
		setupMocks     func()
		expectedStatus int
		expectError    bool
	}{
		{
			name: "successful mark conversation as read",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/conversations/conv1/read",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
						PathParams: map[string]string{"id": "conv1"},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				ctx.SetParam("id", "conv1")
				return ctx
			},
			setupMocks: func() {
				// Mock calls disabled for test migration
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name: "missing conversation ID",
			setupContext: func() *lift.Context {
				req := &lift.Request{
					Request: &adapters.Request{
						Method: "POST",
						Path:   "/api/v1/conversations//read",
						Headers: map[string]string{
							"X-Test-Username": "testuser",
						},
					},
				}
				
				ctx := lift.NewContext(context.Background(), req)
				return ctx
			},
			setupMocks: func() {
				// No mocks needed for bad request
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
			
			// Get context
			ctx := tt.setupContext()
			
			// Call handler directly
			err := handler.HandleMarkConversationReadLift(ctx)
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			
			// Check status
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
		})
	}
}