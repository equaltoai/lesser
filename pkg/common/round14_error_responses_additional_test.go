package common

import (
	stdErrors "errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRespondAuthErrorWithCode(t *testing.T) {
	t.Run("nil error becomes internal", func(t *testing.T) {
		ctx := newTestContext("GET", "/test")
		respObj, err := RespondAuthErrorWithCode(ctx, 401, nil)
		require.NoError(t, err)
		status, resp := parseResponse(t, respObj)
		assert.Equal(t, 500, status)
		assert.Equal(t, string(errors.CodeInternal), resp.Code)
	})

	t.Run("401 returns unauthorized", func(t *testing.T) {
		ctx := newTestContext("GET", "/test")
		respObj, err := RespondAuthErrorWithCode(ctx, 401, stdErrors.New("bad token"))
		require.NoError(t, err)
		status, resp := parseResponse(t, respObj)
		assert.Equal(t, 401, status)
		assert.Equal(t, string(errors.CodeUnauthorized), resp.Code)
	})

	t.Run("403 returns forbidden", func(t *testing.T) {
		ctx := newTestContext("GET", "/test")
		respObj, err := RespondAuthErrorWithCode(ctx, 403, stdErrors.New("nope"))
		require.NoError(t, err)
		status, resp := parseResponse(t, respObj)
		assert.Equal(t, 403, status)
		assert.Equal(t, string(errors.CodeForbidden), resp.Code)
	})
}

func TestRespondValidationOrError_AndValidationKeywordHelpers(t *testing.T) {
	assert.True(t, containsValidationKeywords("required field"))
	assert.True(t, containsValidationKeywords("cannot be blank: username"))
	assert.False(t, containsValidationKeywords("something else"))

	assert.Equal(t, 1, minInt(1, 2))
	assert.Equal(t, 1, minInt(2, 1))

	t.Run("validation keyword routes to validation response", func(t *testing.T) {
		ctx := newTestContext("GET", "/test")
		respObj, err := RespondValidationOrError(ctx, stdErrors.New("required field"))
		require.NoError(t, err)
		status, resp := parseResponse(t, respObj)
		assert.Equal(t, 400, status)
		assert.Equal(t, string(errors.CodeValidationFailed), resp.Code)
	})

	t.Run("non-validation routes to bad request", func(t *testing.T) {
		ctx := newTestContext("GET", "/test")
		respObj, err := RespondValidationOrError(ctx, stdErrors.New("nope"))
		require.NoError(t, err)
		status, resp := parseResponse(t, respObj)
		assert.Equal(t, 400, status)
		assert.Equal(t, string(errors.CodeBadRequest), resp.Code)
	})
}

func TestOperationErrorHelpers(t *testing.T) {
	t.Run("RespondCreateError conflict uses 409", func(t *testing.T) {
		ctx := newTestContext("POST", "/test")
		respObj, err := RespondCreateError(ctx, "resource", ConflictError{Resource: "resource", Message: "exists"})
		require.NoError(t, err)
		status, resp := parseResponse(t, respObj)
		assert.Equal(t, 409, status)
		assert.Equal(t, string(errors.CodeConflict), resp.Code)
	})

	t.Run("RespondUpdateError not found uses 404", func(t *testing.T) {
		ctx := newTestContext("PUT", "/test")
		respObj, err := RespondUpdateError(ctx, "resource", ActorNotFoundError{Username: "alice"})
		require.NoError(t, err)
		status, resp := parseResponse(t, respObj)
		assert.Equal(t, 404, status)
		assert.Equal(t, string(errors.CodeNotFound), resp.Code)
	})

	t.Run("RespondDeleteError not found uses 404", func(t *testing.T) {
		ctx := newTestContext("DELETE", "/test")
		respObj, err := RespondDeleteError(ctx, "resource", ActivityNotFoundError{ID: "id"})
		require.NoError(t, err)
		status, resp := parseResponse(t, respObj)
		assert.Equal(t, 404, status)
		assert.Equal(t, string(errors.CodeNotFound), resp.Code)
	})

	t.Run("RespondGetError not found uses 404", func(t *testing.T) {
		ctx := newTestContext("GET", "/test")
		respObj, err := RespondGetError(ctx, "resource", ActorNotFoundError{Username: "alice"})
		require.NoError(t, err)
		status, resp := parseResponse(t, respObj)
		assert.Equal(t, 404, status)
		assert.Equal(t, string(errors.CodeNotFound), resp.Code)
	})
}

func TestAdditionalResponseHelpers(t *testing.T) {
	t.Run("rate limited", func(t *testing.T) {
		ctx := newTestContext("GET", "/test")
		respObj, err := RespondRateLimited(ctx)
		require.NoError(t, err)
		status, resp := parseResponse(t, respObj)
		assert.Equal(t, 429, status)
		assert.Equal(t, string(errors.CodeRateLimited), resp.Code)
	})

	t.Run("with app error", func(t *testing.T) {
		ctx := newTestContext("GET", "/test")
		respObj, err := RespondWithAppError(ctx, errors.NotFound("thing"))
		require.NoError(t, err)
		status, resp := parseResponse(t, respObj)
		assert.Equal(t, 404, status)
		assert.Equal(t, string(errors.CodeNotFound), resp.Code)
	})

	t.Run("with message + description + code", func(t *testing.T) {
		ctx := newTestContext("GET", "/test")
		respObj, err := RespondWithErrorMessage(ctx, 418, "teapot")
		require.NoError(t, err)
		status, resp := parseResponse(t, respObj)
		assert.Equal(t, 418, status)
		assert.Equal(t, "BAD_REQUEST", resp.Code)

		ctx2 := newTestContext("GET", "/test")
		respObj2, err := RespondWithErrorAndDescription(ctx2, 400, "bad", "details")
		require.NoError(t, err)
		_, resp2 := parseResponse(t, respObj2)
		assert.Equal(t, "details", resp2.Description)

		ctx3 := newTestContext("GET", "/test")
		respObj3, err := RespondWithErrorCode(ctx3, 400, "bad", "CUSTOM")
		require.NoError(t, err)
		_, resp3 := parseResponse(t, respObj3)
		assert.Equal(t, "CUSTOM", resp3.Code)
	})

	t.Run("legacy error and success", func(t *testing.T) {
		ctx := newTestContext("GET", "/test")
		legacyResp, err := RespondLegacyError(ctx, 400, "bad")
		require.NoError(t, err)
		assert.Equal(t, 400, legacyResp.Status)

		ctx2 := newTestContext("GET", "/test")
		successResp, err := RespondSuccess(ctx2, map[string]any{"ok": true})
		require.NoError(t, err)
		assert.Equal(t, 200, successResp.Status)
	})
}
