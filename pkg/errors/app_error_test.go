package errors

import (
	stdErrors "errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestErrorCode_GetHTTPStatusCode(t *testing.T) {
	require.Equal(t, 404, CodeNotFound.GetHTTPStatusCode())
	require.Equal(t, 401, CodeUnauthorized.GetHTTPStatusCode())
	require.Equal(t, 403, CodeForbidden.GetHTTPStatusCode())
	require.Equal(t, 409, CodeConflict.GetHTTPStatusCode())
	require.Equal(t, 410, CodeGone.GetHTTPStatusCode())
	require.Equal(t, 422, CodeUnprocessableEntity.GetHTTPStatusCode())
	require.Equal(t, 400, CodeInvalidInput.GetHTTPStatusCode())
	require.Equal(t, 500, ErrorCode("UNKNOWN").GetHTTPStatusCode())
}

func TestNewAppError_SetsDefaults(t *testing.T) {
	err := NewAppError(CodeNotFound, CategoryAPI, "missing")
	require.Equal(t, CodeNotFound, err.Code)
	require.Equal(t, CategoryAPI, err.Category)
	require.Equal(t, "missing", err.Message)
	require.Equal(t, 404, err.HTTPStatusCode)
	require.NotNil(t, err.Metadata)
	require.False(t, err.Retryable)
}

func TestWrapError_WrapsUnderlyingError(t *testing.T) {
	inner := stdErrors.New("boom")
	err := WrapError(inner, CodeInternal, CategoryInternal, "wrapped")

	require.Equal(t, "wrapped", err.Message)
	require.Equal(t, "boom", err.InternalMessage)
	require.ErrorIs(t, err, inner)
	require.Equal(t, 500, err.HTTPStatusCode)
}

func TestWrapError_WhenWrappingAppError_PreservesRetryable(t *testing.T) {
	inner := NewAppError(CodeTimeout, CategoryExternal, "timeout").WithInternalMessage("root").AsRetryable()
	err := WrapError(inner, CodeInternal, CategoryInternal, "wrapped")

	require.True(t, err.Retryable)
	require.ErrorIs(t, err, inner)
	require.Contains(t, err.InternalMessage, "wrapped:")
}

func TestHelpers_ExtractFields(t *testing.T) {
	err := Forbidden("")

	require.True(t, IsAppError(err))
	require.True(t, HasCode(err, CodeForbidden))
	require.True(t, HasCategory(err, CategoryAuth))
	require.Equal(t, 403, GetHTTPStatus(err))
	require.Equal(t, CodeForbidden, GetErrorCode(err))
	require.Equal(t, CategoryAuth, GetErrorCategory(err))

	require.False(t, IsRetryable(err))
}

func TestAsAppError_WorksThroughWrapping(t *testing.T) {
	inner := Forbidden("nope")
	wrapped := fmt.Errorf("wrapped: %w", inner)

	got, ok := AsAppError(wrapped)
	require.True(t, ok)
	require.Same(t, inner, got)

	require.True(t, IsAppError(wrapped))
	require.True(t, HasCode(wrapped, CodeForbidden))
	require.True(t, HasCategory(wrapped, CategoryAuth))
	require.Equal(t, 403, GetHTTPStatus(wrapped))
}
