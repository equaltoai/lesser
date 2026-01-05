package lambda

import (
	stdErrors "errors"
	"testing"

	apperrors "github.com/equaltoai/lesser/pkg/errors"
	liftPkg "github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestErrorPattern_CreateErrorHandlingMiddleware_HandlesErrors(t *testing.T) {
	ep := NewErrorPattern(zap.NewNop())
	mw := ep.CreateErrorHandlingMiddleware()
	ctx := newErrorPatternTestContext(nil)

	handler := mw(liftPkg.HandlerFunc(func(*liftPkg.Context) error {
		return apperrors.Forbidden("nope")
	}))

	require.NoError(t, handler.Handle(ctx))
	require.Equal(t, 403, ctx.Response.StatusCode)
}

func TestErrorPattern_HandleAuthenticationAuthorizationNotFoundInternal(t *testing.T) {
	ep := NewErrorPattern(zap.NewNop())

	t.Run("authentication", func(t *testing.T) {
		ctx := newErrorPatternTestContext(nil)
		require.NoError(t, ep.HandleAuthenticationError(ctx, "bad token"))
		require.Equal(t, 401, ctx.Response.StatusCode)
	})

	t.Run("authorization", func(t *testing.T) {
		ctx := newErrorPatternTestContext(nil)
		require.NoError(t, ep.HandleAuthorizationError(ctx, "nope"))
		require.Equal(t, 403, ctx.Response.StatusCode)
	})

	t.Run("not found", func(t *testing.T) {
		ctx := newErrorPatternTestContext(nil)
		require.NoError(t, ep.HandleNotFoundError(ctx, "thing"))
		require.Equal(t, 404, ctx.Response.StatusCode)

		body, ok := ctx.Response.Body.(StandardErrorResponse)
		require.True(t, ok)
		require.Equal(t, "thing", body.Details["resource"])
	})

	t.Run("internal", func(t *testing.T) {
		ctx := newErrorPatternTestContext(nil)
		require.NoError(t, ep.HandleInternalError(ctx, stdErrors.New("boom"), "oops"))
		require.Equal(t, 500, ctx.Response.StatusCode)
	})
}

func TestErrorPattern_ValidateRequiredParam(t *testing.T) {
	ep := NewErrorPattern(zap.NewNop())

	t.Run("empty triggers validation error", func(t *testing.T) {
		ctx := newErrorPatternTestContext(nil)
		require.NoError(t, ep.ValidateRequiredParam(ctx, "param", ""))
		require.Equal(t, 400, ctx.Response.StatusCode)
	})

	t.Run("non-empty returns nil", func(t *testing.T) {
		ctx := newErrorPatternTestContext(nil)
		require.NoError(t, ep.ValidateRequiredParam(ctx, "param", "value"))
		require.NotNil(t, ctx.Response)
		require.Equal(t, 200, ctx.Response.StatusCode)
		require.Nil(t, ctx.Response.Body)
	})
}

func TestErrorPattern_logError_ExercisesBranches(t *testing.T) {
	ep := NewErrorPattern(zap.NewNop())
	ep.logError("req-1", 500, "boom", stdErrors.New("err"))
	ep.logError("req-1", 400, "boom", stdErrors.New("err"))
	ep.logError("req-1", 200, "boom", stdErrors.New("err"))
}

func TestErrorPattern_HandleActivityPubError_FallsBackToStandardJSON(t *testing.T) {
	ep := NewErrorPattern(zap.NewNop())
	ctx := newErrorPatternTestContext(map[string]string{
		"Accept": "application/json",
	})
	appErr := apperrors.NotFound("actor")

	require.NoError(t, ep.HandleActivityPubError(ctx, appErr, "not found"))
	require.Equal(t, appErr.HTTPStatusCode, ctx.Response.StatusCode)

	_, ok := ctx.Response.Body.(StandardErrorResponse)
	require.True(t, ok)
	require.NotEqual(t, "application/activity+json", ctx.Response.Headers["Content-Type"])
}
