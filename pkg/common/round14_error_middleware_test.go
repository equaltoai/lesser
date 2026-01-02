package common

import (
	stdErrors "errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/errors"
	liftTesting "github.com/equaltoai/lesser/pkg/testing/lift"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	h := mw(lift.HandlerFunc(func(_ *lift.Context) error {
		panic("boom")
	}))

	ctx := liftTesting.MockLiftContext("GET", "/panic")
	err := h.Handle(ctx)
	require.NoError(t, err)

	status, resp := parseResponse(t, ctx)
	assert.Equal(t, 500, status)
	assert.NotEmpty(t, resp.Error)
	assert.Equal(t, string(errors.CodeInternal), resp.Code)
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
			h := mw(lift.HandlerFunc(func(_ *lift.Context) error {
				return tt.err
			}))

			ctx := liftTesting.MockLiftContext("GET", "/test")
			err := h.Handle(ctx)
			require.NoError(t, err)

			status, resp := parseResponse(t, ctx)
			assert.Equal(t, tt.wantStatusCode, status)
			assert.Equal(t, string(tt.wantCode), resp.Code)
		})
	}
}
