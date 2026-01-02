package common

import (
	stdErrors "errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/errors"
	liftTesting "github.com/equaltoai/lesser/pkg/testing/lift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrorResponseHelpers_Wrappers(t *testing.T) {
	t.Run("auth wrappers", func(t *testing.T) {
		ctx := liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, RespondUnauthorizedWithDescription(ctx, "details"))
		status, resp := parseResponse(t, ctx)
		assert.Equal(t, 401, status)
		assert.Equal(t, "details", resp.Description)
		assert.Equal(t, string(errors.CodeUnauthorized), resp.Code)

		ctx = liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, RespondMissingAuth(ctx))
		_, resp = parseResponse(t, ctx)
		assert.Equal(t, "authentication required", resp.Error)

		ctx = liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, RespondInvalidToken(ctx))
		_, resp = parseResponse(t, ctx)
		assert.Equal(t, string(errors.CodeTokenInvalid), resp.Code)

		ctx = liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, RespondExpiredToken(ctx))
		_, resp = parseResponse(t, ctx)
		assert.Equal(t, string(errors.CodeTokenExpired), resp.Code)

		ctx = liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, RespondInsufficientScope(ctx, "read"))
		status, resp = parseResponse(t, ctx)
		expected := errors.NewAppError(errors.CodeInsufficientScope, errors.CategoryAuth, "insufficient scope: requires read")
		assert.Equal(t, expected.HTTPStatusCode, status)
		assert.Equal(t, string(expected.Code), resp.Code)
	})

	t.Run("forbidden wrappers", func(t *testing.T) {
		ctx := liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, RespondNotAuthorized(ctx, "resource"))
		status, resp := parseResponse(t, ctx)
		assert.Equal(t, 403, status)
		assert.Equal(t, string(errors.CodeForbidden), resp.Code)

		ctx = liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, RespondNotAuthorizedToModify(ctx, "resource"))
		_, resp = parseResponse(t, ctx)
		assert.Equal(t, string(errors.CodeForbidden), resp.Code)

		ctx = liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, RespondNotAuthorizedToDelete(ctx, "resource"))
		_, resp = parseResponse(t, ctx)
		assert.Equal(t, string(errors.CodeForbidden), resp.Code)
	})

	t.Run("bad request wrappers", func(t *testing.T) {
		ctx := liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, RespondInvalidParameter(ctx, "param"))
		status, resp := parseResponse(t, ctx)
		assert.Equal(t, 400, status)
		assert.Equal(t, string(errors.CodeBadRequest), resp.Code)

		ctx = liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, RespondMissingAccountID(ctx))
		_, resp = parseResponse(t, ctx)
		assert.Equal(t, string(errors.CodeBadRequest), resp.Code)

		ctx = liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, RespondMissingStatusID(ctx))
		_, resp = parseResponse(t, ctx)
		assert.Equal(t, string(errors.CodeBadRequest), resp.Code)

		ctx = liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, RespondInvalidRequest(ctx))
		_, resp = parseResponse(t, ctx)
		assert.Equal(t, string(errors.CodeBadRequest), resp.Code)
	})

	t.Run("not found wrappers", func(t *testing.T) {
		ctx := liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, RespondAccountNotFound(ctx))
		status, resp := parseResponse(t, ctx)
		assert.Equal(t, 404, status)
		assert.Equal(t, string(errors.CodeNotFound), resp.Code)

		ctx = liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, RespondStatusNotFound(ctx))
		_, resp = parseResponse(t, ctx)
		assert.Equal(t, string(errors.CodeNotFound), resp.Code)

		ctx = liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, RespondUserNotFound(ctx))
		_, resp = parseResponse(t, ctx)
		assert.Equal(t, string(errors.CodeNotFound), resp.Code)

		ctx = liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, RespondActorNotFound(ctx))
		_, resp = parseResponse(t, ctx)
		assert.Equal(t, string(errors.CodeNotFound), resp.Code)

		ctx = liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, RespondFilterNotFound(ctx))
		_, resp = parseResponse(t, ctx)
		assert.Equal(t, string(errors.CodeNotFound), resp.Code)

		ctx = liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, RespondConversationNotFound(ctx))
		_, resp = parseResponse(t, ctx)
		assert.Equal(t, string(errors.CodeNotFound), resp.Code)
	})

	t.Run("method not allowed wrapper", func(t *testing.T) {
		ctx := liftTesting.MockLiftContext("POST", "/test")
		require.NoError(t, RespondMethodNotAllowed(ctx))
		status, resp := parseResponse(t, ctx)
		expected := errors.NewAppError(errors.CodeMethodNotAllowed, errors.CategoryAPI, "Method Not Allowed")
		assert.Equal(t, expected.HTTPStatusCode, status)
		assert.Equal(t, string(expected.Code), resp.Code)
	})

	t.Run("unprocessable wrappers", func(t *testing.T) {
		ctx := liftTesting.MockLiftContext("POST", "/test")
		require.NoError(t, RespondStatusTooLong(ctx))
		status, resp := parseResponse(t, ctx)
		assert.Equal(t, 422, status)
		assert.Equal(t, string(errors.CodeUnprocessableEntity), resp.Code)

		ctx = liftTesting.MockLiftContext("POST", "/test")
		require.NoError(t, RespondInvalidContent(ctx))
		status, resp = parseResponse(t, ctx)
		assert.Equal(t, 422, status)
		assert.Equal(t, string(errors.CodeUnprocessableEntity), resp.Code)
	})

	t.Run("internal error wrappers", func(t *testing.T) {
		ctx := liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, RespondDatabaseError(ctx))
		status, resp := parseResponse(t, ctx)
		assert.Equal(t, 500, status)
		assert.Equal(t, string(errors.CodeInternal), resp.Code)

		ctx = liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, RespondFailedToCreate(ctx, "thing"))
		_, resp = parseResponse(t, ctx)
		assert.Equal(t, string(errors.CodeInternal), resp.Code)

		ctx = liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, RespondFailedToUpdate(ctx, "thing"))
		_, resp = parseResponse(t, ctx)
		assert.Equal(t, string(errors.CodeInternal), resp.Code)

		ctx = liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, RespondFailedToDelete(ctx, "thing"))
		_, resp = parseResponse(t, ctx)
		assert.Equal(t, string(errors.CodeInternal), resp.Code)

		ctx = liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, RespondFailedToGet(ctx, "thing"))
		_, resp = parseResponse(t, ctx)
		assert.Equal(t, string(errors.CodeInternal), resp.Code)
	})
}

func TestOperationErrorHelpers_CoverRemainingBranches(t *testing.T) {
	t.Run("RespondCreateError branches", func(t *testing.T) {
		ctx := liftTesting.MockLiftContext("POST", "/test")
		require.NoError(t, RespondCreateError(ctx, "resource", nil))
		_, resp := parseResponse(t, ctx)
		assert.Equal(t, string(errors.CodeInternal), resp.Code)

		ctx = liftTesting.MockLiftContext("POST", "/test")
		require.NoError(t, RespondCreateError(ctx, "resource", errors.NotFound("resource")))
		status, resp := parseResponse(t, ctx)
		assert.Equal(t, 404, status)
		assert.Equal(t, string(errors.CodeNotFound), resp.Code)

		ctx = liftTesting.MockLiftContext("POST", "/test")
		require.NoError(t, RespondCreateError(ctx, "resource", stdErrors.New("cannot be blank")))
		status, resp = parseResponse(t, ctx)
		assert.Equal(t, 400, status)
		assert.Equal(t, string(errors.CodeValidationFailed), resp.Code)

		ctx = liftTesting.MockLiftContext("POST", "/test")
		require.NoError(t, RespondCreateError(ctx, "resource", stdErrors.New("db is down")))
		status, resp = parseResponse(t, ctx)
		assert.Equal(t, 500, status)
		assert.Equal(t, string(errors.CodeInternal), resp.Code)
	})

	t.Run("RespondUpdateError branches", func(t *testing.T) {
		ctx := liftTesting.MockLiftContext("PUT", "/test")
		require.NoError(t, RespondUpdateError(ctx, "resource", nil))
		_, resp := parseResponse(t, ctx)
		assert.Equal(t, string(errors.CodeInternal), resp.Code)

		ctx = liftTesting.MockLiftContext("PUT", "/test")
		require.NoError(t, RespondUpdateError(ctx, "resource", errors.NotFound("resource")))
		status, resp := parseResponse(t, ctx)
		assert.Equal(t, 404, status)
		assert.Equal(t, string(errors.CodeNotFound), resp.Code)

		ctx = liftTesting.MockLiftContext("PUT", "/test")
		require.NoError(t, RespondUpdateError(ctx, "resource", stdErrors.New("must be a thing")))
		status, resp = parseResponse(t, ctx)
		assert.Equal(t, 400, status)
		assert.Equal(t, string(errors.CodeValidationFailed), resp.Code)

		ctx = liftTesting.MockLiftContext("PUT", "/test")
		require.NoError(t, RespondUpdateError(ctx, "resource", stdErrors.New("db is down")))
		status, resp = parseResponse(t, ctx)
		assert.Equal(t, 500, status)
		assert.Equal(t, string(errors.CodeInternal), resp.Code)
	})

	t.Run("RespondDeleteError and RespondGetError default branches", func(t *testing.T) {
		ctx := liftTesting.MockLiftContext("DELETE", "/test")
		require.NoError(t, RespondDeleteError(ctx, "resource", nil))
		_, resp := parseResponse(t, ctx)
		assert.Equal(t, string(errors.CodeInternal), resp.Code)

		ctx = liftTesting.MockLiftContext("DELETE", "/test")
		require.NoError(t, RespondDeleteError(ctx, "resource", stdErrors.New("db is down")))
		status, resp := parseResponse(t, ctx)
		assert.Equal(t, 500, status)
		assert.Equal(t, string(errors.CodeInternal), resp.Code)

		ctx = liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, RespondGetError(ctx, "resource", nil))
		_, resp = parseResponse(t, ctx)
		assert.Equal(t, string(errors.CodeInternal), resp.Code)

		ctx = liftTesting.MockLiftContext("GET", "/test")
		require.NoError(t, RespondGetError(ctx, "resource", stdErrors.New("db is down")))
		status, resp = parseResponse(t, ctx)
		assert.Equal(t, 500, status)
		assert.Equal(t, string(errors.CodeInternal), resp.Code)
	})
}
