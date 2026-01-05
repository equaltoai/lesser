package lambda

import (
	"context"
	stdErrors "errors"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/common"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	liftPkg "github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newErrorPatternTestContext(headers map[string]string) *liftPkg.Context {
	req := liftPkg.NewRequest(nil)
	req.Method = "GET"
	req.Path = "/resource"
	req.Headers = headers
	ctx := liftPkg.NewContext(context.Background(), req)
	ctx.RequestID = "req-1"
	return ctx
}

func TestErrorPattern_handleError_PassthroughLiftError(t *testing.T) {
	ep := NewErrorPattern(zap.NewNop())
	ctx := newErrorPatternTestContext(nil)

	liftErr := liftPkg.NewLiftError("BAD", "bad", http.StatusTeapot)
	err := ep.handleError(ctx, liftErr)
	require.Same(t, liftErr, err)
}

func TestErrorPattern_handleError_FormatsAppError(t *testing.T) {
	ep := NewErrorPattern(zap.NewNop())
	ctx := newErrorPatternTestContext(nil)

	appErr := apperrors.Forbidden("nope")
	require.NoError(t, ep.handleError(ctx, appErr))

	require.Equal(t, appErr.HTTPStatusCode, ctx.Response.StatusCode)
	body, ok := ctx.Response.Body.(StandardErrorResponse)
	require.True(t, ok)
	require.Equal(t, string(appErr.Code), body.Error)
	require.Equal(t, string(appErr.Code), body.ErrorCode)
	require.Equal(t, "req-1", body.RequestID)
}

func TestErrorPattern_handleError_ConvertsLegacyErrors(t *testing.T) {
	ep := NewErrorPattern(zap.NewNop())
	ctx := newErrorPatternTestContext(nil)

	require.NoError(t, ep.handleError(ctx, stdErrors.New("authentication required")))
	require.Equal(t, 401, ctx.Response.StatusCode)

	body, ok := ctx.Response.Body.(StandardErrorResponse)
	require.True(t, ok)
	require.Equal(t, string(apperrors.CodeUnauthorized), body.Error)
}

func TestErrorPattern_convertLegacyError_MatchesMessagePatterns(t *testing.T) {
	ep := NewErrorPattern(zap.NewNop())

	tests := []struct {
		name       string
		err        error
		wantCode   apperrors.ErrorCode
		wantStatus int
	}{
		{name: "unauthorized", err: stdErrors.New("invalid token"), wantCode: apperrors.CodeUnauthorized, wantStatus: 401},
		{name: "forbidden", err: stdErrors.New("access denied"), wantCode: apperrors.CodeForbidden, wantStatus: 403},
		{name: "validation", err: stdErrors.New("bad request"), wantCode: apperrors.CodeValidationFailed, wantStatus: 400},
		{name: "not found", err: stdErrors.New("does not exist"), wantCode: apperrors.CodeNotFound, wantStatus: 404},
		{name: "conflict", err: stdErrors.New("already exists"), wantCode: apperrors.CodeConflict, wantStatus: 409},
		{name: "rate limited", err: stdErrors.New("too many requests"), wantCode: apperrors.CodeRateLimited, wantStatus: 429},
		{name: "timeout", err: context.DeadlineExceeded, wantCode: apperrors.CodeTimeout, wantStatus: 408},
		{name: "lambda", err: stdErrors.New("lambda boom"), wantCode: apperrors.CodeLambdaTimeout, wantStatus: 408},
		{name: "default internal", err: stdErrors.New("unknown"), wantCode: apperrors.CodeInternal, wantStatus: 500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ep.convertLegacyError(tt.err)
			require.Equal(t, tt.wantCode, got.Code)
			require.Equal(t, tt.wantStatus, got.HTTPStatusCode)
		})
	}
}

func TestErrorPattern_convertLegacyError_ConvertsCommonAppError_ValidationFallback(t *testing.T) {
	ep := NewErrorPattern(zap.NewNop())

	legacy := common.AppError{
		Code:        "NOT_FOUND",
		UserMessage: "bad request but wrong code",
		StatusCode:  400,
	}

	got := ep.convertLegacyError(legacy)
	require.Equal(t, apperrors.CodeInternal, got.Code) // mismatched status -> internal
	require.Equal(t, 500, got.HTTPStatusCode)
}

func TestErrorPattern_HandleValidationError_SetsDetails(t *testing.T) {
	ep := NewErrorPattern(zap.NewNop())
	ctx := newErrorPatternTestContext(nil)

	require.NoError(t, ep.HandleValidationError(ctx, "field", "bad"))
	require.Equal(t, 400, ctx.Response.StatusCode)

	body, ok := ctx.Response.Body.(StandardErrorResponse)
	require.True(t, ok)
	require.Equal(t, string(apperrors.CodeValidationFailed), body.Error)
	require.Equal(t, "req-1", body.RequestID)
	require.Equal(t, "field", body.Details["field"])
}

func TestErrorPattern_HandleRateLimitError_SetsHeader(t *testing.T) {
	ep := NewErrorPattern(zap.NewNop())
	ctx := newErrorPatternTestContext(nil)

	require.NoError(t, ep.HandleRateLimitError(ctx, 7))
	require.Equal(t, 429, ctx.Response.StatusCode)
	require.Equal(t, "7", ctx.Response.Headers["Retry-After"])
}

func TestErrorPattern_WrapWithErrorHandler_HandlesError(t *testing.T) {
	ep := NewErrorPattern(zap.NewNop())
	ctx := newErrorPatternTestContext(nil)

	handler := ep.WrapWithErrorHandler(func(*liftPkg.Context) error {
		return apperrors.NotFound("thing")
	})
	require.NoError(t, handler.Handle(ctx))
	require.Equal(t, 404, ctx.Response.StatusCode)
}

func TestErrorPattern_CreatePanicRecoveryMiddleware_Recovers(t *testing.T) {
	ep := NewErrorPattern(zap.NewNop())
	ctx := newErrorPatternTestContext(nil)

	mw := ep.CreatePanicRecoveryMiddleware()
	handler := mw(liftPkg.HandlerFunc(func(*liftPkg.Context) error {
		panic("boom")
	}))
	require.NoError(t, handler.Handle(ctx))
	require.Equal(t, 500, ctx.Response.StatusCode)
}

func TestErrorPattern_HandleActivityPubError_ContentNegotiation(t *testing.T) {
	ep := NewErrorPattern(zap.NewNop())

	ctx := newErrorPatternTestContext(map[string]string{
		"Accept": "application/activity+json",
	})
	appErr := apperrors.NotFound("actor")

	require.NoError(t, ep.HandleActivityPubError(ctx, appErr, "not found"))
	require.Equal(t, appErr.HTTPStatusCode, ctx.Response.StatusCode)
	require.Equal(t, "application/activity+json", ctx.Response.Headers["Content-Type"])

	body, ok := ctx.Response.Body.(ActivityPubErrorResponse)
	require.True(t, ok)
	require.Equal(t, string(appErr.Code), body.Type)
}
