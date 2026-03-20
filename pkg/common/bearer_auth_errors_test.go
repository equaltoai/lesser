package common

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBearerAuthErrorHelpers_ForAPIPaths(t *testing.T) {
	t.Run("missing auth uses canonical invalid_token contract", func(t *testing.T) {
		ctx := newTestContext("GET", "/api/v1/test")
		resp, err := RespondMissingAuth(ctx)
		require.NoError(t, err)
		require.Equal(t, 401, resp.Status)

		var body BearerAuthErrorResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, BearerErrorInvalidToken, body.Error)
		require.Equal(t, "authentication required", body.Description)
		require.Contains(t, resp.Headers["www-authenticate"][0], `Bearer realm="lesser"`)
		require.Contains(t, resp.Headers["www-authenticate"][0], `error="invalid_token"`)
	})

	t.Run("insufficient scope uses canonical scope contract", func(t *testing.T) {
		ctx := newTestContext("GET", "/api/v1/test")
		resp, err := RespondInsufficientScope(ctx, "read")
		require.NoError(t, err)
		require.Equal(t, 403, resp.Status)

		var body BearerAuthErrorResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, BearerErrorInsufficientScope, body.Error)
		require.Equal(t, "insufficient scope: requires read", body.Description)
		require.Equal(t, "read", body.Scope)
		require.Contains(t, resp.Headers["www-authenticate"][0], `scope="read"`)
	})

	t.Run("direct bearer helpers cover invalid token, expired token, and rate limiting", func(t *testing.T) {
		apiCtx := newTestContext("GET", "/api/v1/test")

		resp, err := RespondBearerInvalidToken(apiCtx, "bad bearer token")
		require.NoError(t, err)
		require.Equal(t, 401, resp.Status)

		var invalidBody BearerAuthErrorResponse
		require.NoError(t, json.Unmarshal(resp.Body, &invalidBody))
		require.Equal(t, BearerErrorInvalidToken, invalidBody.Error)
		require.Equal(t, "bad bearer token", invalidBody.Description)
		require.Contains(t, resp.Headers["www-authenticate"][0], `error_description="bad bearer token"`)

		resp, err = RespondBearerExpiredToken(apiCtx)
		require.NoError(t, err)
		require.Equal(t, 401, resp.Status)

		var expiredBody BearerAuthErrorResponse
		require.NoError(t, json.Unmarshal(resp.Body, &expiredBody))
		require.Equal(t, BearerErrorInvalidToken, expiredBody.Error)
		require.Equal(t, "token expired", expiredBody.Description)

		resp, err = RespondBearerExpiredToken(apiCtx, "session drift")
		require.NoError(t, err)
		require.Equal(t, 401, resp.Status)
		require.NoError(t, json.Unmarshal(resp.Body, &expiredBody))
		require.Equal(t, "session drift", expiredBody.Description)

		resp, err = RespondBearerRateLimited(apiCtx, "")
		require.NoError(t, err)
		require.Equal(t, 429, resp.Status)

		var rateLimitedBody BearerAuthErrorResponse
		require.NoError(t, json.Unmarshal(resp.Body, &rateLimitedBody))
		require.Equal(t, BearerErrorSlowDown, rateLimitedBody.Error)
		require.Equal(t, "rate limit exceeded", rateLimitedBody.Description)
		_, hasChallenge := resp.Headers["www-authenticate"]
		require.False(t, hasChallenge)
	})

	t.Run("legacy non api helpers remain unchanged", func(t *testing.T) {
		ctx := newTestContext("GET", "/auth/setup")
		resp, err := RespondUnauthorized(ctx)
		require.NoError(t, err)

		status, parsed := parseResponse(t, resp)
		require.Equal(t, 401, status)
		require.Equal(t, "Unauthorized", parsed.Error)
		require.Equal(t, "UNAUTHORIZED", parsed.Code)
	})

	t.Run("nil context does not opt into bearer api behavior", func(t *testing.T) {
		require.False(t, isBearerAPIAuthPath(nil))
	})

	t.Run("api unauthorized with description uses bearer contract", func(t *testing.T) {
		ctx := newTestContext("GET", "/api/v1/test")
		resp, err := RespondUnauthorizedWithDescription(ctx, "refresh required")
		require.NoError(t, err)
		require.Equal(t, 401, resp.Status)

		var body BearerAuthErrorResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, BearerErrorInvalidToken, body.Error)
		require.Equal(t, "refresh required", body.Description)
	})
}
