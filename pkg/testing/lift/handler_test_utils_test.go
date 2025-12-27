package lift

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMockLiftContext_Options(t *testing.T) {
	ctx := MockLiftContext(
		"POST",
		"/api/test",
		WithHeaders(map[string]string{"X-Test": "1"}),
		WithQueryParams(map[string]string{"q": "ok"}),
		WithPathParams(map[string]string{"id": "123"}),
		WithTenant("tenant-1"),
		WithAuth("alice", []string{"read"}),
		WithBody(map[string]interface{}{"ok": true}),
	)

	require.Equal(t, "POST", ctx.Request.Method)
	require.Equal(t, "/api/test", ctx.Request.Path)
	require.Equal(t, "1", ctx.Request.Headers["X-Test"])
	require.Equal(t, "tenant-1", ctx.Request.Headers["X-Tenant-ID"])
	require.Equal(t, "ok", ctx.Request.QueryParams["q"])
	require.Equal(t, "123", ctx.Request.PathParams["id"])

	require.Equal(t, "tenant-1", ctx.Get("tenant_id"))
	require.Equal(t, "alice", ctx.Get("username"))
	require.Equal(t, []string{"read"}, ctx.Get("scopes"))
	require.Equal(t, true, ctx.Get("authenticated"))

	require.Equal(t, "application/json", ctx.Request.Headers["Content-Type"])

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(ctx.Request.Body, &body))
	require.Equal(t, true, body["ok"])
}
