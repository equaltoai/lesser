package common

import (
	stdErrors "errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrorResponseHelpers_Wrappers(t *testing.T) {
	t.Run("auth wrappers", func(t *testing.T) {
		ctx := newTestContext("GET", "/test")
		respObj, err := RespondUnauthorizedWithDescription(ctx, "details")
		require.NoError(t, err)
		status, resp := parseResponse(t, respObj)
		assert.Equal(t, 401, status)
		assert.Equal(t, "details", resp.Description)
		assert.Equal(t, string(errors.CodeUnauthorized), resp.Code)

		ctx = newTestContext("GET", "/test")
		respObj, err = RespondMissingAuth(ctx)
		require.NoError(t, err)
		_, resp = parseResponse(t, respObj)
		assert.Equal(t, "authentication required", resp.Error)

		ctx = newTestContext("GET", "/test")
		respObj, err = RespondInvalidToken(ctx)
		require.NoError(t, err)
		_, resp = parseResponse(t, respObj)
		assert.Equal(t, string(errors.CodeTokenInvalid), resp.Code)

		ctx = newTestContext("GET", "/test")
		respObj, err = RespondExpiredToken(ctx)
		require.NoError(t, err)
		_, resp = parseResponse(t, respObj)
		assert.Equal(t, string(errors.CodeTokenExpired), resp.Code)

		ctx = newTestContext("GET", "/test")
		respObj, err = RespondInsufficientScope(ctx, "read")
		require.NoError(t, err)
		status, resp = parseResponse(t, respObj)
		expected := errors.NewAppError(errors.CodeInsufficientScope, errors.CategoryAuth, "insufficient scope: requires read")
		assert.Equal(t, expected.HTTPStatusCode, status)
		assert.Equal(t, string(expected.Code), resp.Code)
	})

	t.Run("forbidden wrappers", func(t *testing.T) {
		ctx := newTestContext("GET", "/test")
		respObj, err := RespondNotAuthorized(ctx, "resource")
		require.NoError(t, err)
		status, resp := parseResponse(t, respObj)
		assert.Equal(t, 403, status)
		assert.Equal(t, string(errors.CodeForbidden), resp.Code)

		ctx = newTestContext("GET", "/test")
		respObj, err = RespondNotAuthorizedToModify(ctx, "resource")
		require.NoError(t, err)
		_, resp = parseResponse(t, respObj)
		assert.Equal(t, string(errors.CodeForbidden), resp.Code)

		ctx = newTestContext("GET", "/test")
		respObj, err = RespondNotAuthorizedToDelete(ctx, "resource")
		require.NoError(t, err)
		_, resp = parseResponse(t, respObj)
		assert.Equal(t, string(errors.CodeForbidden), resp.Code)
	})

	t.Run("bad request wrappers", func(t *testing.T) {
		ctx := newTestContext("GET", "/test")
		respObj, err := RespondInvalidParameter(ctx, "param")
		require.NoError(t, err)
		status, resp := parseResponse(t, respObj)
		assert.Equal(t, 400, status)
		assert.Equal(t, string(errors.CodeBadRequest), resp.Code)

		ctx = newTestContext("GET", "/test")
		respObj, err = RespondMissingAccountID(ctx)
		require.NoError(t, err)
		_, resp = parseResponse(t, respObj)
		assert.Equal(t, string(errors.CodeBadRequest), resp.Code)

		ctx = newTestContext("GET", "/test")
		respObj, err = RespondMissingStatusID(ctx)
		require.NoError(t, err)
		_, resp = parseResponse(t, respObj)
		assert.Equal(t, string(errors.CodeBadRequest), resp.Code)

		ctx = newTestContext("GET", "/test")
		respObj, err = RespondInvalidRequest(ctx)
		require.NoError(t, err)
		_, resp = parseResponse(t, respObj)
		assert.Equal(t, string(errors.CodeBadRequest), resp.Code)
	})

	t.Run("not found wrappers", func(t *testing.T) {
		ctx := newTestContext("GET", "/test")
		respObj, err := RespondAccountNotFound(ctx)
		require.NoError(t, err)
		status, resp := parseResponse(t, respObj)
		assert.Equal(t, 404, status)
		assert.Equal(t, string(errors.CodeNotFound), resp.Code)

		ctx = newTestContext("GET", "/test")
		respObj, err = RespondStatusNotFound(ctx)
		require.NoError(t, err)
		_, resp = parseResponse(t, respObj)
		assert.Equal(t, string(errors.CodeNotFound), resp.Code)

		ctx = newTestContext("GET", "/test")
		respObj, err = RespondUserNotFound(ctx)
		require.NoError(t, err)
		_, resp = parseResponse(t, respObj)
		assert.Equal(t, string(errors.CodeNotFound), resp.Code)

		ctx = newTestContext("GET", "/test")
		respObj, err = RespondActorNotFound(ctx)
		require.NoError(t, err)
		_, resp = parseResponse(t, respObj)
		assert.Equal(t, string(errors.CodeNotFound), resp.Code)

		ctx = newTestContext("GET", "/test")
		respObj, err = RespondFilterNotFound(ctx)
		require.NoError(t, err)
		_, resp = parseResponse(t, respObj)
		assert.Equal(t, string(errors.CodeNotFound), resp.Code)

		ctx = newTestContext("GET", "/test")
		respObj, err = RespondConversationNotFound(ctx)
		require.NoError(t, err)
		_, resp = parseResponse(t, respObj)
		assert.Equal(t, string(errors.CodeNotFound), resp.Code)
	})

	t.Run("method not allowed wrapper", func(t *testing.T) {
		ctx := newTestContext("POST", "/test")
		respObj, err := RespondMethodNotAllowed(ctx)
		require.NoError(t, err)
		status, resp := parseResponse(t, respObj)
		expected := errors.NewAppError(errors.CodeMethodNotAllowed, errors.CategoryAPI, "Method Not Allowed")
		assert.Equal(t, expected.HTTPStatusCode, status)
		assert.Equal(t, string(expected.Code), resp.Code)
	})

	t.Run("unprocessable wrappers", func(t *testing.T) {
		ctx := newTestContext("POST", "/test")
		respObj, err := RespondStatusTooLong(ctx)
		require.NoError(t, err)
		status, resp := parseResponse(t, respObj)
		assert.Equal(t, 422, status)
		assert.Equal(t, string(errors.CodeUnprocessableEntity), resp.Code)

		ctx = newTestContext("POST", "/test")
		respObj, err = RespondInvalidContent(ctx)
		require.NoError(t, err)
		status, resp = parseResponse(t, respObj)
		assert.Equal(t, 422, status)
		assert.Equal(t, string(errors.CodeUnprocessableEntity), resp.Code)
	})

	t.Run("internal error wrappers", func(t *testing.T) {
		ctx := newTestContext("GET", "/test")
		respObj, err := RespondDatabaseError(ctx)
		require.NoError(t, err)
		status, resp := parseResponse(t, respObj)
		assert.Equal(t, 500, status)
		assert.Equal(t, string(errors.CodeInternal), resp.Code)

		ctx = newTestContext("GET", "/test")
		respObj, err = RespondFailedToCreate(ctx, "thing")
		require.NoError(t, err)
		_, resp = parseResponse(t, respObj)
		assert.Equal(t, string(errors.CodeInternal), resp.Code)

		ctx = newTestContext("GET", "/test")
		respObj, err = RespondFailedToUpdate(ctx, "thing")
		require.NoError(t, err)
		_, resp = parseResponse(t, respObj)
		assert.Equal(t, string(errors.CodeInternal), resp.Code)

		ctx = newTestContext("GET", "/test")
		respObj, err = RespondFailedToDelete(ctx, "thing")
		require.NoError(t, err)
		_, resp = parseResponse(t, respObj)
		assert.Equal(t, string(errors.CodeInternal), resp.Code)

		ctx = newTestContext("GET", "/test")
		respObj, err = RespondFailedToGet(ctx, "thing")
		require.NoError(t, err)
		_, resp = parseResponse(t, respObj)
		assert.Equal(t, string(errors.CodeInternal), resp.Code)
	})
}

func TestOperationErrorHelpers_CoverRemainingBranches(t *testing.T) {
	t.Run("RespondCreateError branches", func(t *testing.T) {
		ctx := newTestContext("POST", "/test")
		respObj, err := RespondCreateError(ctx, "resource", nil)
		require.NoError(t, err)
		_, resp := parseResponse(t, respObj)
		assert.Equal(t, string(errors.CodeInternal), resp.Code)

		ctx = newTestContext("POST", "/test")
		respObj, err = RespondCreateError(ctx, "resource", errors.NotFound("resource"))
		require.NoError(t, err)
		status, resp := parseResponse(t, respObj)
		assert.Equal(t, 404, status)
		assert.Equal(t, string(errors.CodeNotFound), resp.Code)

		ctx = newTestContext("POST", "/test")
		respObj, err = RespondCreateError(ctx, "resource", stdErrors.New("cannot be blank"))
		require.NoError(t, err)
		status, resp = parseResponse(t, respObj)
		assert.Equal(t, 400, status)
		assert.Equal(t, string(errors.CodeValidationFailed), resp.Code)

		ctx = newTestContext("POST", "/test")
		respObj, err = RespondCreateError(ctx, "resource", stdErrors.New("db is down"))
		require.NoError(t, err)
		status, resp = parseResponse(t, respObj)
		assert.Equal(t, 500, status)
		assert.Equal(t, string(errors.CodeInternal), resp.Code)
	})

	t.Run("RespondUpdateError branches", func(t *testing.T) {
		ctx := newTestContext("PUT", "/test")
		respObj, err := RespondUpdateError(ctx, "resource", nil)
		require.NoError(t, err)
		_, resp := parseResponse(t, respObj)
		assert.Equal(t, string(errors.CodeInternal), resp.Code)

		ctx = newTestContext("PUT", "/test")
		respObj, err = RespondUpdateError(ctx, "resource", errors.NotFound("resource"))
		require.NoError(t, err)
		status, resp := parseResponse(t, respObj)
		assert.Equal(t, 404, status)
		assert.Equal(t, string(errors.CodeNotFound), resp.Code)

		ctx = newTestContext("PUT", "/test")
		respObj, err = RespondUpdateError(ctx, "resource", stdErrors.New("must be a thing"))
		require.NoError(t, err)
		status, resp = parseResponse(t, respObj)
		assert.Equal(t, 400, status)
		assert.Equal(t, string(errors.CodeValidationFailed), resp.Code)

		ctx = newTestContext("PUT", "/test")
		respObj, err = RespondUpdateError(ctx, "resource", stdErrors.New("db is down"))
		require.NoError(t, err)
		status, resp = parseResponse(t, respObj)
		assert.Equal(t, 500, status)
		assert.Equal(t, string(errors.CodeInternal), resp.Code)
	})

	t.Run("RespondDeleteError and RespondGetError default branches", func(t *testing.T) {
		ctx := newTestContext("DELETE", "/test")
		respObj, err := RespondDeleteError(ctx, "resource", nil)
		require.NoError(t, err)
		status, resp := parseResponse(t, respObj)
		_ = status
		assert.Equal(t, string(errors.CodeInternal), resp.Code)

		ctx = newTestContext("DELETE", "/test")
		respObj, err = RespondDeleteError(ctx, "resource", stdErrors.New("db is down"))
		require.NoError(t, err)
		status, resp = parseResponse(t, respObj)
		assert.Equal(t, 500, status)
		assert.Equal(t, string(errors.CodeInternal), resp.Code)

		ctx = newTestContext("GET", "/test")
		respObj, err = RespondGetError(ctx, "resource", nil)
		require.NoError(t, err)
		_, resp = parseResponse(t, respObj)
		assert.Equal(t, string(errors.CodeInternal), resp.Code)

		ctx = newTestContext("GET", "/test")
		respObj, err = RespondGetError(ctx, "resource", stdErrors.New("db is down"))
		require.NoError(t, err)
		status, resp = parseResponse(t, respObj)
		assert.Equal(t, 500, status)
		assert.Equal(t, string(errors.CodeInternal), resp.Code)
	})
}
