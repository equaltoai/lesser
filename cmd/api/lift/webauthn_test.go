package lift

import (
	"context"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// Simplified tests focusing on authentication and error handling

func TestHandleBeginWebAuthnRegistrationLift_Authentication(t *testing.T) {
	tests := []struct {
		name           string
		headers        map[string]string
		expectedStatus int
	}{
		{
			name: "missing authentication returns 401",
			headers: map[string]string{
				"Content-Type": "application/json",
			},
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := new(MockStorageAdapter)
			
			handler := &Handler{
				cfg: &config.Config{
					JWTSecret: "test-secret",
					Domain:    "test.example.com",
				},
				store:          mockStore,
				logger:         zap.NewNop(),
				authMiddleware: &auth.Middleware{},
			}
			
			ctx := &lift.Context{
				Context: context.Background(),
				Request: &lift.Request{
					Method:  "POST",
					Path:    "/api/v1/auth/webauthn/register/begin",
					Headers: tt.headers,
				},
			}
			ctx.Response = &lift.Response{
				Headers:    make(map[string]string),
				StatusCode: 200,
			}
			
			err := handler.HandleBeginWebAuthnRegistrationLift(ctx)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
		})
	}
}

func TestHandleFinishWebAuthnRegistrationLift_Authentication(t *testing.T) {
	tests := []struct {
		name           string
		headers        map[string]string
		expectedStatus int
	}{
		{
			name: "missing authentication returns 401",
			headers: map[string]string{
				"Content-Type": "application/json",
			},
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := new(MockStorageAdapter)
			
			handler := &Handler{
				cfg: &config.Config{
					JWTSecret: "test-secret",
					Domain:    "test.example.com",
				},
				store:          mockStore,
				logger:         zap.NewNop(),
				authMiddleware: &auth.Middleware{},
			}
			
			ctx := &lift.Context{
				Context: context.Background(),
				Request: &lift.Request{
					Method:  "POST",
					Path:    "/api/v1/auth/webauthn/register/finish",
					Headers: tt.headers,
				},
			}
			ctx.Response = &lift.Response{
				Headers:    make(map[string]string),
				StatusCode: 200,
			}
			
			err := handler.HandleFinishWebAuthnRegistrationLift(ctx)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
		})
	}
}

func TestHandleBeginWebAuthnLoginLift_BadRequest(t *testing.T) {
	tests := []struct {
		name           string
		headers        map[string]string
		expectedStatus int
	}{
		{
			name: "missing request body returns 400", // ParseRequest will fail on empty body
			headers: map[string]string{
				"Content-Type": "application/json",
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := new(MockStorageAdapter)
			
			handler := &Handler{
				cfg: &config.Config{
					JWTSecret: "test-secret",
					Domain:    "test.example.com",
				},
				store:          mockStore,
				logger:         zap.NewNop(),  
				authMiddleware: &auth.Middleware{},
			}
			
			ctx := &lift.Context{
				Context: context.Background(),
				Request: &lift.Request{
					Method:  "POST",
					Path:    "/api/v1/auth/webauthn/login/begin",
					Headers: tt.headers,
				},
			}
			ctx.Response = &lift.Response{
				Headers:    make(map[string]string),
				StatusCode: 200,
			}
			
			err := handler.HandleBeginWebAuthnLoginLift(ctx)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
		})
	}
}

func TestHandleFinishWebAuthnLoginLift_BadRequest(t *testing.T) {
	tests := []struct {
		name           string
		headers        map[string]string
		expectedStatus int
	}{
		{
			name: "missing request body returns 400", // ParseRequest will fail on empty body  
			headers: map[string]string{
				"Content-Type": "application/json",
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := new(MockStorageAdapter)
			
			handler := &Handler{
				cfg: &config.Config{
					JWTSecret: "test-secret",
					Domain:    "test.example.com",
				},
				store:          mockStore,
				logger:         zap.NewNop(),
				authMiddleware: &auth.Middleware{},
			}
			
			ctx := &lift.Context{
				Context: context.Background(),
				Request: &lift.Request{
					Method:  "POST",
					Path:    "/api/v1/auth/webauthn/login/finish",
					Headers: tt.headers,
				},
			}
			ctx.Response = &lift.Response{
				Headers:    make(map[string]string),
				StatusCode: 200,
			}
			
			err := handler.HandleFinishWebAuthnLoginLift(ctx)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
		})
	}
}

func TestHandleListWebAuthnCredentialsLift_Authentication(t *testing.T) {
	tests := []struct {
		name           string
		headers        map[string]string
		expectedStatus int
	}{
		{
			name: "missing authentication returns 401",
			headers: map[string]string{},
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := new(MockStorageAdapter)
			
			handler := &Handler{
				cfg: &config.Config{
					JWTSecret: "test-secret",
					Domain:    "test.example.com",
				},
				store:          mockStore,
				logger:         zap.NewNop(),
				authMiddleware: &auth.Middleware{},
			}
			
			ctx := &lift.Context{
				Context: context.Background(),
				Request: &lift.Request{
					Method:  "GET",
					Path:    "/api/v1/auth/webauthn/credentials",
					Headers: tt.headers,
				},
			}
			ctx.Response = &lift.Response{
				Headers:    make(map[string]string),
				StatusCode: 200,
			}
			
			err := handler.HandleListWebAuthnCredentialsLift(ctx)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
		})
	}
}

func TestHandleDeleteWebAuthnCredentialLift_Authentication(t *testing.T) {
	tests := []struct {
		name           string
		headers        map[string]string
		expectedStatus int
	}{
		{
			name: "missing authentication returns 401",
			headers: map[string]string{},
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := new(MockStorageAdapter)
			
			handler := &Handler{
				cfg: &config.Config{
					JWTSecret: "test-secret",
					Domain:    "test.example.com",
				},
				store:          mockStore,
				logger:         zap.NewNop(),
				authMiddleware: &auth.Middleware{},
			}
			
			ctx := &lift.Context{
				Context: context.Background(),
				Request: &lift.Request{
					Method:  "DELETE",
					Path:    "/api/v1/auth/webauthn/credentials/test-cred-id",
					Headers: tt.headers,
				},
			}
			ctx.Response = &lift.Response{
				Headers:    make(map[string]string),
				StatusCode: 200,
			}
			
			err := handler.HandleDeleteWebAuthnCredentialLift(ctx)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
		})
	}
}

func TestHandleUpdateWebAuthnCredentialNameLift_Authentication(t *testing.T) {
	tests := []struct {
		name           string
		headers        map[string]string
		expectedStatus int
	}{
		{
			name: "missing authentication returns 401",
			headers: map[string]string{
				"Content-Type": "application/json",
			},
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := new(MockStorageAdapter)
			
			handler := &Handler{
				cfg: &config.Config{
					JWTSecret: "test-secret",
					Domain:    "test.example.com",
				},
				store:          mockStore,
				logger:         zap.NewNop(),
				authMiddleware: &auth.Middleware{},
			}
			
			ctx := &lift.Context{
				Context: context.Background(),
				Request: &lift.Request{
					Method:  "PUT",
					Path:    "/api/v1/auth/webauthn/credentials/test-cred-id",
					Headers: tt.headers,
				},
			}
			ctx.Response = &lift.Response{
				Headers:    make(map[string]string),
				StatusCode: 200,
			}
			
			err := handler.HandleUpdateWebAuthnCredentialNameLift(ctx)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
		})
	}
}

func TestGetAuthenticatedUserLift(t *testing.T) {
	tests := []struct {
		name         string
		headers      map[string]string
		expected     string
	}{
		{
			name: "test mode with X-Test-Username header",
			headers: map[string]string{
				"X-Test-Username": "testuser",
			},
			expected: "testuser",
		},
		{
			name:     "no authentication headers",
			headers:  map[string]string{},
			expected: "",
		},
		{
			name: "invalid authorization header format",
			headers: map[string]string{
				"Authorization": "InvalidFormat token",
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &Handler{
				cfg: &config.Config{
					JWTSecret: "test-secret",
				},
				authMiddleware: &auth.Middleware{},
			}
			
			ctx := &lift.Context{
				Context: context.Background(),
				Request: &lift.Request{
					Headers: tt.headers,
				},
			}
			
			result := handler.getAuthenticatedUserLift(ctx)
			
			assert.Equal(t, tt.expected, result)
		})
	}
}