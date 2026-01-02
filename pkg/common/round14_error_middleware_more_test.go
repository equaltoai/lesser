package common

import (
	"context"
	stdErrors "errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/errors"
	liftTesting "github.com/equaltoai/lesser/pkg/testing/lift"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		ctx := liftTesting.MockLiftContext("GET", "/test")

		for _, mw := range []lift.Middleware{
			ValidationErrorMiddleware("svc", logger),
			ProductionErrorMiddleware("svc", logger),
			DevelopmentErrorMiddleware("svc", logger),
		} {
			ctx = liftTesting.MockLiftContext("GET", "/test") // reset
			h := mw(lift.HandlerFunc(func(*lift.Context) error {
				return ConflictError{Resource: "x", Message: "exists"}
			}))
			require.NoError(t, h.Handle(ctx))
			status, _ := parseResponse(t, ctx)
			assert.Equal(t, 409, status)
		}
	})

	t.Run("NotFoundMiddleware converts missing response to 404", func(t *testing.T) {
		mw := NotFoundMiddleware()
		h := mw(lift.HandlerFunc(func(ctx *lift.Context) error {
			ctx.Response.StatusCode = 0
			return nil
		}))

		ctx := liftTesting.MockLiftContext("GET", "/missing")
		err := h.Handle(ctx)
		require.NoError(t, err)
		assert.Equal(t, 404, ctx.Response.StatusCode)
	})

	t.Run("TimeoutErrorMiddleware converts deadline exceeded to 503", func(t *testing.T) {
		mw := TimeoutErrorMiddleware("svc", zap.NewNop())
		h := mw(lift.HandlerFunc(func(*lift.Context) error {
			return context.DeadlineExceeded
		}))

		ctx := liftTesting.MockLiftContext("GET", "/timeout")
		err := h.Handle(ctx)
		require.NoError(t, err)
		assert.Equal(t, 503, ctx.Response.StatusCode)
	})

	t.Run("ErrorRecoveryMiddleware recovers from temporary and retryable errors", func(t *testing.T) {
		logger := zap.NewNop()
		mw := ErrorRecoveryMiddleware("svc", logger)

		for _, errToReturn := range []error{
			temporaryErr{},
			retryableErr{},
			errors.Internal("x").AsRetryable(),
		} {
			ctx := liftTesting.MockLiftContext("GET", "/recovery")
			h := mw(lift.HandlerFunc(func(*lift.Context) error { return errToReturn }))
			err := h.Handle(ctx)
			require.NoError(t, err)
			assert.Equal(t, 503, ctx.Response.StatusCode)
		}
	})

	t.Run("ErrorRecoveryMiddleware returns original when unrecoverable", func(t *testing.T) {
		wantErr := stdErrors.New("no")
		mw := ErrorRecoveryMiddleware("svc", zap.NewNop())
		h := mw(lift.HandlerFunc(func(*lift.Context) error { return wantErr }))

		ctx := liftTesting.MockLiftContext("GET", "/recovery")
		err := h.Handle(ctx)
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
	ctx := liftTesting.MockLiftContext("GET", "/test")
	h := mw(lift.HandlerFunc(func(*lift.Context) error { return stdErrors.New("x") }))
	require.NoError(t, h.Handle(ctx))
	assert.Equal(t, 500, ctx.Response.StatusCode)

	cfg.Environment = "development"
	_ = CreateAPIErrorMiddleware(logger)
	_ = CreateGraphQLErrorMiddleware(logger)
	_ = CreateFederationErrorMiddleware(logger)
}
