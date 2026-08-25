package common

import (
	"context"
	stdErrors "errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
	"go.uber.org/zap"
)

type temporaryErr struct{}

func (temporaryErr) Error() string   { return "temporary" }
func (temporaryErr) Temporary() bool { return true }

type retryableErr struct{}

func (retryableErr) Error() string   { return "retryable" }
func (retryableErr) Retryable() bool { return true }
func (retryableErr) Temporary() bool { return false }
func (retryableErr) Timeout() bool   { return false }
func (retryableErr) Unwrap() error   { return nil }

func TestErrorMiddleware_WrappersAndRecoveryHelpers(t *testing.T) {
	t.Run("Validation/Production/Development middleware wrappers execute", func(t *testing.T) {
		logger := zap.NewNop()

		for _, mw := range []apptheory.Middleware{
			ValidationErrorMiddleware("svc", logger),
			ProductionErrorMiddleware("svc", logger),
			DevelopmentErrorMiddleware("svc", logger),
		} {
			ctx := newTestContext("GET", "/test")
			h := mw(func(*apptheory.Context) (*apptheory.Response, error) {
				return nil, ConflictError{Resource: "x", Message: "exists"}
			})
			resp, err := h(ctx)
			require.NoError(t, err)
			status, _ := parseResponse(t, resp)
			assert.Equal(t, 409, status)
		}
	})

	t.Run("NotFoundMiddleware converts missing response to 404", func(t *testing.T) {
		mw := NotFoundMiddleware()
		h := mw(func(*apptheory.Context) (*apptheory.Response, error) { return nil, nil })

		ctx := newTestContext("GET", "/missing")
		resp, err := h(ctx)
		require.NoError(t, err)
		assert.Equal(t, 404, resp.Status)
	})

	t.Run("TimeoutErrorMiddleware converts deadline exceeded to 503", func(t *testing.T) {
		mw := TimeoutErrorMiddleware("svc", zap.NewNop())
		h := mw(func(*apptheory.Context) (*apptheory.Response, error) { return nil, context.DeadlineExceeded })

		ctx := newTestContext("GET", "/timeout")
		resp, err := h(ctx)
		require.NoError(t, err)
		assert.Equal(t, 503, resp.Status)
	})

	t.Run("ErrorRecoveryMiddleware recovers from temporary and retryable errors", func(t *testing.T) {
		logger := zap.NewNop()
		mw := ErrorRecoveryMiddleware("svc", logger)

		for _, errToReturn := range []error{
			temporaryErr{},
			retryableErr{},
			errors.Internal("x").AsRetryable(),
		} {
			ctx := newTestContext("GET", "/recovery")
			h := mw(func(*apptheory.Context) (*apptheory.Response, error) { return nil, errToReturn })
			resp, err := h(ctx)
			require.NoError(t, err)
			assert.Equal(t, 503, resp.Status)
		}
	})

	t.Run("ErrorRecoveryMiddleware returns original when unrecoverable", func(t *testing.T) {
		wantErr := stdErrors.New("no")
		mw := ErrorRecoveryMiddleware("svc", zap.NewNop())
		h := mw(func(*apptheory.Context) (*apptheory.Response, error) { return nil, wantErr })

		ctx := newTestContext("GET", "/recovery")
		_, err := h(ctx)
		assert.ErrorIs(t, err, wantErr)
	})
}

func TestCreateStandardErrorMiddleware_Branches(t *testing.T) {
	logger := zap.NewNop()
	cfg := config.Get()
	origEnv := cfg.Environment
	t.Cleanup(func() { cfg.Environment = origEnv })

	cfg.Environment = "production"
	mw := CreateStandardErrorMiddleware("svc", logger)
	ctx := newTestContext("GET", "/test")
	h := mw(func(*apptheory.Context) (*apptheory.Response, error) { return nil, stdErrors.New("x") })
	resp, err := h(ctx)
	require.NoError(t, err)
	assert.Equal(t, 500, resp.Status)

	cfg.Environment = "development"
	_ = CreateAPIErrorMiddleware(logger)
	_ = CreateGraphQLErrorMiddleware(logger)
	_ = CreateFederationErrorMiddleware(logger)
}
