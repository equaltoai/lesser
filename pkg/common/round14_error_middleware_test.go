package common

import (
	"context"
	stdErrors "errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
	"go.uber.org/zap"
)

func TestDefaultErrorConfig_SetsDefaults(t *testing.T) {
	logger := zap.NewNop()
	cfg := DefaultErrorConfig("svc", logger)
	assert.Equal(t, "svc", cfg.ServiceName)
	assert.Same(t, logger, cfg.Logger)
	assert.True(t, cfg.EnablePanicRecovery)
	assert.True(t, cfg.EnableErrorMetrics)
	assert.Equal(t, 2000, cfg.MaxErrorLogLength)
}

func TestErrorHandlingMiddleware_PanicRecovery(t *testing.T) {
	cfg := ErrorMiddlewareConfig{
		Logger:              zap.NewNop(),
		ServiceName:         "svc",
		EnableStackTrace:    true,
		EnablePanicRecovery: true,
		EnableErrorMetrics:  true,
		MaxErrorLogLength:   100,
	}

	mw := ErrorHandlingMiddleware(cfg)
	h := mw(func(_ *apptheory.Context) (*apptheory.Response, error) {
		panic("boom")
	})

	ctx := newTestContext("GET", "/panic")
	resp, err := h(ctx)
	require.NoError(t, err)

	status, parsed := parseResponse(t, resp)
	assert.Equal(t, 500, status)
	assert.NotEmpty(t, parsed.Error)
	assert.Equal(t, string(errors.CodeInternal), parsed.Code)
}

func TestErrorHandlingMiddleware_ConvertsReturnedErrors(t *testing.T) {
	cfg := ErrorMiddlewareConfig{
		Logger:              zap.NewNop(),
		ServiceName:         "svc",
		EnableStackTrace:    true,
		EnablePanicRecovery: false,
		EnableErrorMetrics:  true,
		MaxErrorLogLength:   10, // trigger truncation branch
	}

	tests := []struct {
		name           string
		err            error
		wantStatusCode int
		wantCode       errors.ErrorCode
	}{
		{
			name:           "app error passes through",
			err:            errors.Forbidden("nope"),
			wantStatusCode: 403,
			wantCode:       errors.CodeForbidden,
		},
		{
			name:           "apptheory error uses explicit status",
			err:            apptheory.NewAppTheoryError("app.custom", "limited").WithStatusCode(429),
			wantStatusCode: 429,
			wantCode:       errors.CodeRateLimited,
		},
		{
			name:           "apptheory error derives status from code",
			err:            apptheory.NewAppTheoryError("app.too_large", "too large"),
			wantStatusCode: 413,
			wantCode:       errors.CodeContentTooLarge,
		},
		{
			name:           "not found converts",
			err:            ActorNotFoundError{Username: "alice"},
			wantStatusCode: 404,
			wantCode:       errors.CodeNotFound,
		},
		{
			name:           "validation converts",
			err:            ValidationError{Field: "x", Message: "bad"},
			wantStatusCode: 400,
			wantCode:       errors.CodeValidationFailed,
		},
		{
			name:           "authentication converts",
			err:            AuthenticationError{Message: "bad"},
			wantStatusCode: 401,
			wantCode:       errors.CodeUnauthorized,
		},
		{
			name:           "authorization converts",
			err:            AuthorizationError{Action: "read", Resource: "x"},
			wantStatusCode: 403,
			wantCode:       errors.CodeForbidden,
		},
		{
			name:           "conflict converts",
			err:            ConflictError{Resource: "x", Message: "exists"},
			wantStatusCode: 409,
			wantCode:       errors.CodeConflict,
		},
		{
			name:           "federation converts",
			err:            FederationError{Operation: "deliver", Remote: "example.com", Err: stdErrors.New("down")},
			wantStatusCode: 503,
			wantCode:       errors.CodeExternalServiceUnavailable,
		},
		{
			name:           "unknown converts to internal",
			err:            stdErrors.New("weird"),
			wantStatusCode: 500,
			wantCode:       errors.CodeInternal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mw := ErrorHandlingMiddleware(cfg)
			h := mw(func(_ *apptheory.Context) (*apptheory.Response, error) { return nil, tt.err })

			ctx := newTestContext("GET", "/test")
			resp, err := h(ctx)
			require.NoError(t, err)

			status, parsed := parseResponse(t, resp)
			assert.Equal(t, tt.wantStatusCode, status)
			assert.Equal(t, string(tt.wantCode), parsed.Code)
		})
	}
}

func TestErrorHandlingMiddleware_PreservesOpaqueLegacyAppTheoryError(t *testing.T) {
	_, legacyErr := (&apptheory.Context{}).JSONValue()
	require.Error(t, legacyErr)

	code, message, ok := legacyAppTheoryError(legacyErr)
	require.True(t, ok)
	require.Equal(t, "app.bad_request", code)
	require.NotEmpty(t, message)

	mw := ErrorHandlingMiddleware(ErrorMiddlewareConfig{
		Logger:              zap.NewNop(),
		ServiceName:         "svc",
		EnablePanicRecovery: false,
	})
	h := mw(func(_ *apptheory.Context) (*apptheory.Response, error) { return nil, legacyErr })

	resp, err := h(newTestContext("POST", "/test"))
	require.NoError(t, err)

	status, parsed := parseResponse(t, resp)
	assert.Equal(t, 400, status)
	assert.Equal(t, string(errors.CodeBadRequest), parsed.Code)
}

func TestErrorMiddlewareTimeoutHelpers_Round26(t *testing.T) {
	assert.False(t, isTimeoutError(nil))
	assert.False(t, isTimeoutError(stdErrors.New("plain")))
	assert.True(t, isTimeoutError(context.DeadlineExceeded))
	assert.True(t, isTimeoutError(errors.TimeoutError("slow")))
	assert.Equal(t, "", sanitizedRequestLogPath(nil))
}
