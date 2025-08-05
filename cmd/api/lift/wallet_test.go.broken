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

func TestHandleCreateChallengeLift_Validation(t *testing.T) {
	tests := []struct {
		name           string
		headers        map[string]string
		body           string
		expectedStatus int
	}{
		{
			name: "missing address returns 400",
			headers: map[string]string{
				"Content-Type": "application/json",
			},
			body:           `{"chainId":1}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "invalid JSON returns 400",
			headers: map[string]string{
				"Content-Type": "application/json",
			},
			body:           `{invalid json}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "empty body returns 400",
			headers: map[string]string{
				"Content-Type": "application/json",
			},
			body:           ``,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &Handler{
				cfg: &config.Config{
					JWTSecret: "test-secret",
					Domain:    "test.example.com",
				},
				repos: &MockRepositoryStorage{},
				logger:         zap.NewNop(),
				authMiddleware: &auth.Middleware{},
			}

			ctx := &lift.Context{
				Context: context.Background(),
				Request: &lift.Request{
					Method:  "POST",
					Path:    "/auth/wallet/challenge",
					Headers: tt.headers,
					Body:    []byte(tt.body),
				},
			}
			ctx.Response = &lift.Response{
				Headers:    make(map[string]string),
				StatusCode: 200,
			}

			err := handler.HandleCreateChallengeLift(ctx)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
		})
	}
}

func TestHandleCreateChallengeLift_WithUsername(t *testing.T) {
	tests := []struct {
		name           string
		headers        map[string]string
		body           string
		expectedStatus int
	}{
		{
			name: "username provided but no auth token returns 401",
			headers: map[string]string{
				"Content-Type": "application/json",
			},
			body:           `{"address":"0x1234567890123456789012345678901234567890","chainId":1,"username":"testuser"}`,
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &Handler{
				cfg: &config.Config{
					JWTSecret: "test-secret",
					Domain:    "test.example.com",
				},
				repos: &MockRepositoryStorage{},
				logger:         zap.NewNop(),
				authMiddleware: &auth.Middleware{},
			}

			ctx := &lift.Context{
				Context: context.Background(),
				Request: &lift.Request{
					Method:  "POST",
					Path:    "/auth/wallet/challenge",
					Headers: tt.headers,
					Body:    []byte(tt.body),
				},
			}
			ctx.Response = &lift.Response{
				Headers:    make(map[string]string),
				StatusCode: 200,
			}

			err := handler.HandleCreateChallengeLift(ctx)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
		})
	}
}

func TestHandleVerifySignatureLift_Validation(t *testing.T) {
	tests := []struct {
		name           string
		headers        map[string]string
		body           string
		expectedStatus int
	}{
		{
			name: "missing required fields returns 400",
			headers: map[string]string{
				"Content-Type": "application/json",
			},
			body:           `{"challengeId":"test-challenge"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "invalid JSON returns 400",
			headers: map[string]string{
				"Content-Type": "application/json",
			},
			body:           `{invalid json}`,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &Handler{
				cfg: &config.Config{
					JWTSecret: "test-secret",
					Domain:    "test.example.com",
				},
				repos: &MockRepositoryStorage{},
				logger:         zap.NewNop(),
				authMiddleware: &auth.Middleware{},
			}

			ctx := &lift.Context{
				Context: context.Background(),
				Request: &lift.Request{
					Method:  "POST",
					Path:    "/auth/wallet/verify",
					Headers: tt.headers,
					Body:    []byte(tt.body),
				},
			}
			ctx.Response = &lift.Response{
				Headers:    make(map[string]string),
				StatusCode: 200,
			}

			err := handler.HandleVerifySignatureLift(ctx)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
		})
	}
}

func TestHandleLinkWalletLift_Authentication(t *testing.T) {
	tests := []struct {
		name           string
		headers        map[string]string
		body           string
		expectedStatus int
	}{
		{
			name: "missing authentication returns 401",
			headers: map[string]string{
				"Content-Type": "application/json",
			},
			body:           `{"address":"0x1234","challengeId":"test","signature":"sig","message":"msg"}`,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "test mode with X-Test-Username header and missing fields returns 400",
			headers: map[string]string{
				"Content-Type":      "application/json",
				"X-Test-Username":   "testuser",
			},
			body:           `{"address":"0x1234"}`,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &Handler{
				cfg: &config.Config{
					JWTSecret: "test-secret",
					Domain:    "test.example.com",
				},
				repos: &MockRepositoryStorage{},
				logger:         zap.NewNop(),
				authMiddleware: &auth.Middleware{},
			}

			ctx := &lift.Context{
				Context: context.Background(),
				Request: &lift.Request{
					Method:  "POST",
					Path:    "/auth/wallet/link",
					Headers: tt.headers,
					Body:    []byte(tt.body),
				},
			}
			ctx.Response = &lift.Response{
				Headers:    make(map[string]string),
				StatusCode: 200,
			}

			err := handler.HandleLinkWalletLift(ctx)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
		})
	}
}

func TestHandleUnlinkWalletLift_Authentication(t *testing.T) {
	tests := []struct {
		name           string
		headers        map[string]string
		pathParams     map[string]string
		expectedStatus int
	}{
		{
			name: "missing authentication returns 401",
			headers: map[string]string{
				"Content-Type": "application/json",
			},
			pathParams: map[string]string{
				"address": "0x1234567890123456789012345678901234567890",
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "test mode with X-Test-Username header and missing address returns 400",
			headers: map[string]string{
				"Content-Type":      "application/json",
				"X-Test-Username":   "testuser",
			},
			pathParams:     map[string]string{},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &Handler{
				cfg: &config.Config{
					JWTSecret: "test-secret",
					Domain:    "test.example.com",
				},
				repos: &MockRepositoryStorage{},
				logger:         zap.NewNop(),
				authMiddleware: &auth.Middleware{},
			}

			ctx := &lift.Context{
				Context: context.Background(),
				Request: &lift.Request{
					Method:  "DELETE",
					Path:    "/auth/wallet/unlink/0x1234567890123456789012345678901234567890",
					Headers: tt.headers,
				},
			}
			ctx.Response = &lift.Response{
				Headers:    make(map[string]string),
				StatusCode: 200,
			}

			// For path parameter testing, we need to create a context with path params
			// For now, we'll just test the auth requirement which doesn't need path params

			err := handler.HandleUnlinkWalletLift(ctx)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
		})
	}
}

func TestHandleGetWalletsLift_Authentication(t *testing.T) {
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
			handler := &Handler{
				cfg: &config.Config{
					JWTSecret: "test-secret",
					Domain:    "test.example.com",
				},
				repos: &MockRepositoryStorage{},
				logger:         zap.NewNop(),
				authMiddleware: &auth.Middleware{},
			}

			ctx := &lift.Context{
				Context: context.Background(),
				Request: &lift.Request{
					Method:  "GET",
					Path:    "/auth/wallet/list",
					Headers: tt.headers,
				},
			}
			ctx.Response = &lift.Response{
				Headers:    make(map[string]string),
				StatusCode: 200,
			}

			err := handler.HandleGetWalletsLift(ctx)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
		})
	}
}

func TestGetAuthTokenLift(t *testing.T) {
	tests := []struct {
		name         string
		headers      map[string]string
		expectedLen  int // Length of returned token (0 means empty)
	}{
		{
			name: "valid Bearer token returns token",
			headers: map[string]string{
				"Authorization": "Bearer test-token-12345",
			},
			expectedLen: 16, // Length of "test-token-12345"
		},
		{
			name: "no authorization header returns empty",
			headers: map[string]string{
				"Content-Type": "application/json",
			},
			expectedLen: 0,
		},
		{
			name: "invalid authorization format returns empty",
			headers: map[string]string{
				"Authorization": "InvalidFormat token",
			},
			expectedLen: 0,
		},
		{
			name: "authorization without Bearer prefix returns empty",
			headers: map[string]string{
				"Authorization": "token-without-bearer",
			},
			expectedLen: 0,
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

			result := handler.getAuthTokenLift(ctx)
			assert.Equal(t, tt.expectedLen, len(result))

			if tt.expectedLen > 0 {
				assert.Equal(t, "test-token-12345", result)
			}
		})
	}
}

func TestWalletHandlers_JSONParsing(t *testing.T) {
	tests := []struct {
		name           string
		handler        string
		body           string
		expectedStatus int
	}{
		{
			name:           "CreateChallenge with empty body returns 400",
			handler:        "create-challenge",
			body:           "",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "VerifySignature with empty body returns 400",
			handler:        "verify-signature",
			body:           "",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "LinkWallet with empty body returns 400 (after auth)",
			handler:        "link-wallet",
			body:           "",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &Handler{
				cfg: &config.Config{
					JWTSecret: "test-secret",
					Domain:    "test.example.com",
				},
				repos: &MockRepositoryStorage{},
				logger:         zap.NewNop(),
				authMiddleware: &auth.Middleware{},
			}

			headers := map[string]string{
				"Content-Type": "application/json",
			}

			// Add test username for handlers that require auth
			if tt.handler == "link-wallet" {
				headers["X-Test-Username"] = "testuser"
			}

			ctx := &lift.Context{
				Context: context.Background(),
				Request: &lift.Request{
					Method:  "POST",
					Path:    "/auth/wallet/" + tt.handler,
					Headers: headers,
					Body:    []byte(tt.body),
				},
			}
			ctx.Response = &lift.Response{
				Headers:    make(map[string]string),
				StatusCode: 200,
			}

			var err error
			switch tt.handler {
			case "create-challenge":
				err = handler.HandleCreateChallengeLift(ctx)
			case "verify-signature":
				err = handler.HandleVerifySignatureLift(ctx)
			case "link-wallet":
				err = handler.HandleLinkWalletLift(ctx)
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, ctx.Response.StatusCode)
		})
	}
}

// Test that device headers are properly available (structure test only)
func TestHandleVerifySignatureLift_HeaderAccess(t *testing.T) {
	handler := &Handler{
		cfg: &config.Config{
			JWTSecret: "test-secret",
			Domain:    "test.example.com",
		},
		repos: &MockRepositoryStorage{},
		logger:         zap.NewNop(),
		authMiddleware: &auth.Middleware{},
	}

	ctx := &lift.Context{
		Context: context.Background(),
		Request: &lift.Request{
			Method: "POST",
			Path:   "/auth/wallet/verify",
			Headers: map[string]string{
				"Content-Type":     "application/json",
				"User-Agent":       "Mozilla/5.0 Test Browser",
				"X-Forwarded-For":  "192.168.1.1",
			},
			Body: []byte(`{"challengeId":"","address":"","signature":"","message":""}`), // Invalid data to trigger validation error
		},
	}
	ctx.Response = &lift.Response{
		Headers:    make(map[string]string),
		StatusCode: 200,
	}

	// This should return 400 due to missing required fields - testing that we get to validation
	err := handler.HandleVerifySignatureLift(ctx)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
}
