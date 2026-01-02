package common

import (
	stdErrors "errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/errors"
	liftTesting "github.com/equaltoai/lesser/pkg/testing/lift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRespondAuthErrorWithCode(t *testing.T) {
	t.Run("nil error becomes internal", func(t *testing.T) {
		ctx := liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, RespondAuthErrorWithCode(ctx, 401, nil))
		status, resp := parseResponse(t, ctx)
		assert.Equal(t, 500, status)
		assert.Equal(t, string(errors.CodeInternal), resp.Code)
	})

	t.Run("401 returns unauthorized", func(t *testing.T) {
		ctx := liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, RespondAuthErrorWithCode(ctx, 401, stdErrors.New("bad token")))
		status, resp := parseResponse(t, ctx)
		assert.Equal(t, 401, status)
		assert.Equal(t, string(errors.CodeUnauthorized), resp.Code)
	})

	t.Run("403 returns forbidden", func(t *testing.T) {
		ctx := liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, RespondAuthErrorWithCode(ctx, 403, stdErrors.New("nope")))
		status, resp := parseResponse(t, ctx)
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
		ctx := liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, RespondValidationOrError(ctx, stdErrors.New("required field")))
		status, resp := parseResponse(t, ctx)
		assert.Equal(t, 400, status)
		assert.Equal(t, string(errors.CodeValidationFailed), resp.Code)
	})

	t.Run("non-validation routes to bad request", func(t *testing.T) {
		ctx := liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, RespondValidationOrError(ctx, stdErrors.New("nope")))
		status, resp := parseResponse(t, ctx)
		assert.Equal(t, 400, status)
		assert.Equal(t, string(errors.CodeBadRequest), resp.Code)
	})
}

func TestOperationErrorHelpers(t *testing.T) {
	t.Run("RespondCreateError conflict uses 409", func(t *testing.T) {
		ctx := liftTesting.MockLiftContext("POST", "/test")
		require.NoError(t, RespondCreateError(ctx, "resource", ConflictError{Resource: "resource", Message: "exists"}))
		status, resp := parseResponse(t, ctx)
		assert.Equal(t, 409, status)
		assert.Equal(t, string(errors.CodeConflict), resp.Code)
	})

	t.Run("RespondUpdateError not found uses 404", func(t *testing.T) {
		ctx := liftTesting.MockLiftContext("PUT", "/test")
		require.NoError(t, RespondUpdateError(ctx, "resource", ActorNotFoundError{Username: "alice"}))
		status, resp := parseResponse(t, ctx)
		assert.Equal(t, 404, status)
		assert.Equal(t, string(errors.CodeNotFound), resp.Code)
	})

	t.Run("RespondDeleteError not found uses 404", func(t *testing.T) {
		ctx := liftTesting.MockLiftContext("DELETE", "/test")
		require.NoError(t, RespondDeleteError(ctx, "resource", ActivityNotFoundError{ID: "id"}))
		status, resp := parseResponse(t, ctx)
		assert.Equal(t, 404, status)
		assert.Equal(t, string(errors.CodeNotFound), resp.Code)
	})

	t.Run("RespondGetError not found uses 404", func(t *testing.T) {
		ctx := liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, RespondGetError(ctx, "resource", ActorNotFoundError{Username: "alice"}))
		status, resp := parseResponse(t, ctx)
		assert.Equal(t, 404, status)
		assert.Equal(t, string(errors.CodeNotFound), resp.Code)
	})
}

func TestAdditionalResponseHelpers(t *testing.T) {
	t.Run("rate limited", func(t *testing.T) {
		ctx := liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, RespondRateLimited(ctx))
		status, resp := parseResponse(t, ctx)
		assert.Equal(t, 429, status)
		assert.Equal(t, string(errors.CodeRateLimited), resp.Code)
	})

	t.Run("with app error", func(t *testing.T) {
		ctx := liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, RespondWithAppError(ctx, errors.NotFound("thing")))
		status, resp := parseResponse(t, ctx)
		assert.Equal(t, 404, status)
		assert.Equal(t, string(errors.CodeNotFound), resp.Code)
	})

	t.Run("with message + description + code", func(t *testing.T) {
		ctx := liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, RespondWithErrorMessage(ctx, 418, "teapot"))
		status, resp := parseResponse(t, ctx)
		assert.Equal(t, 418, status)
		assert.Equal(t, "BAD_REQUEST", resp.Code)

		ctx2 := liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, RespondWithErrorAndDescription(ctx2, 400, "bad", "details"))
		_, resp2 := parseResponse(t, ctx2)
		assert.Equal(t, "details", resp2.Description)

		ctx3 := liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, RespondWithErrorCode(ctx3, 400, "bad", "CUSTOM"))
		_, resp3 := parseResponse(t, ctx3)
		assert.Equal(t, "CUSTOM", resp3.Code)
	})

	t.Run("legacy error and success", func(t *testing.T) {
		ctx := liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, RespondLegacyError(ctx, 400, "bad"))
		assert.Equal(t, 400, ctx.Response.StatusCode)

		ctx2 := liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, RespondSuccess(ctx2, map[string]any{"ok": true}))
		assert.Equal(t, 200, ctx2.Response.StatusCode)
	})
}
