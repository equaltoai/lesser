package federation

import (
	"context"
	stdErrors "errors"
	"net/http"
	"testing"

	pkgerrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type timeoutNetError struct{}

func (timeoutNetError) Error() string   { return "i/o timeout" }
func (timeoutNetError) Timeout() bool   { return true }
func (timeoutNetError) Temporary() bool { return true }

func TestClassifyFetchRequestError(t *testing.T) {
	t.Run("nil error becomes remote fetch failed", func(t *testing.T) {
		appErr, ok := pkgerrors.AsAppError(classifyFetchRequestError("https://remote.example/objects/1", nil))
		require.True(t, ok)
		assert.Equal(t, pkgerrors.CodeRemoteFetchFailed, appErr.Code)
	})

	t.Run("context deadline becomes timeout", func(t *testing.T) {
		appErr, ok := pkgerrors.AsAppError(classifyFetchRequestError("https://remote.example/objects/1", context.DeadlineExceeded))
		require.True(t, ok)
		assert.Equal(t, pkgerrors.CodeTimeout, appErr.Code)
	})

	t.Run("net timeout becomes timeout", func(t *testing.T) {
		appErr, ok := pkgerrors.AsAppError(classifyFetchRequestError("https://remote.example/objects/1", timeoutNetError{}))
		require.True(t, ok)
		assert.Equal(t, pkgerrors.CodeTimeout, appErr.Code)
	})

	t.Run("generic network failure becomes external service unavailable", func(t *testing.T) {
		appErr, ok := pkgerrors.AsAppError(classifyFetchRequestError("https://remote.example/objects/1", stdErrors.New("dial tcp failure")))
		require.True(t, ok)
		assert.Equal(t, pkgerrors.CodeExternalServiceUnavailable, appErr.Code)
		assert.True(t, appErr.Retryable)
	})
}

func TestClassifyFetchHTTPStatus(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantCode   pkgerrors.ErrorCode
		wantRetry  bool
	}{
		{name: "unauthorized", statusCode: http.StatusUnauthorized, wantCode: pkgerrors.CodeUnauthorized},
		{name: "forbidden", statusCode: http.StatusForbidden, wantCode: pkgerrors.CodeUnauthorized},
		{name: "not found", statusCode: http.StatusNotFound, wantCode: pkgerrors.CodeNotFound},
		{name: "gone", statusCode: http.StatusGone, wantCode: pkgerrors.CodeGone},
		{name: "request timeout", statusCode: http.StatusRequestTimeout, wantCode: pkgerrors.CodeTimeout, wantRetry: true},
		{name: "gateway timeout", statusCode: http.StatusGatewayTimeout, wantCode: pkgerrors.CodeTimeout, wantRetry: true},
		{name: "rate limited", statusCode: http.StatusTooManyRequests, wantCode: pkgerrors.CodeExternalServiceUnavailable, wantRetry: true},
		{name: "bad gateway", statusCode: http.StatusBadGateway, wantCode: pkgerrors.CodeExternalServiceUnavailable, wantRetry: true},
		{name: "service unavailable", statusCode: http.StatusServiceUnavailable, wantCode: pkgerrors.CodeExternalServiceUnavailable, wantRetry: true},
		{name: "server error fallback", statusCode: http.StatusInternalServerError, wantCode: pkgerrors.CodeExternalServiceUnavailable, wantRetry: true},
		{name: "unexpected status", statusCode: http.StatusTeapot, wantCode: pkgerrors.CodeRemoteFetchFailed, wantRetry: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appErr, ok := pkgerrors.AsAppError(classifyFetchHTTPStatus("https://remote.example/objects/1", tt.statusCode))
			require.True(t, ok)
			assert.Equal(t, tt.wantCode, appErr.Code)
			assert.Equal(t, tt.wantRetry, appErr.Retryable)
		})
	}
}
