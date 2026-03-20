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

	t.Run("legacy non api helpers remain unchanged", func(t *testing.T) {
		ctx := newTestContext("GET", "/auth/setup")
		resp, err := RespondUnauthorized(ctx)
		require.NoError(t, err)

		status, parsed := parseResponse(t, resp)
		require.Equal(t, 401, status)
		require.Equal(t, "Unauthorized", parsed.Error)
		require.Equal(t, "UNAUTHORIZED", parsed.Code)
	})
}
