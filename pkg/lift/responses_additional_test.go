package lift

import (
	"testing"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/require"
)

func TestResponseHelpers(t *testing.T) {
	t.Run("OK", func(t *testing.T) {
		ctx := createTestContext()
		require.NoError(t, OK(ctx, map[string]string{"ok": "1"}))
		require.Equal(t, 200, ctx.Response.StatusCode)
		require.Equal(t, lift.ContentTypeJSON, ctx.Response.Headers[lift.HeaderContentType])
	})

	t.Run("Created", func(t *testing.T) {
		ctx := createTestContext()
		require.NoError(t, Created(ctx, "1", map[string]string{"x": "y"}))
		require.Equal(t, 201, ctx.Response.StatusCode)
		body, ok := ctx.Response.Body.(CreatedResponse)
		require.True(t, ok)
		require.Equal(t, "1", body.ID)
	})

	t.Run("Updated", func(t *testing.T) {
		ctx := createTestContext()
		require.NoError(t, Updated(ctx, "1", map[string]string{"x": "y"}, "x"))
		body, ok := ctx.Response.Body.(UpdatedResponse)
		require.True(t, ok)
		require.Equal(t, []string{"x"}, body.Updated)
	})

	t.Run("Deleted", func(t *testing.T) {
		ctx := createTestContext()
		require.NoError(t, Deleted(ctx, "1"))
		body, ok := ctx.Response.Body.(DeletedResponse)
		require.True(t, ok)
		require.True(t, body.Deleted)
	})

	t.Run("NoContent", func(t *testing.T) {
		ctx := createTestContext()
		require.NoError(t, NoContent(ctx))
		require.Equal(t, 204, ctx.Response.StatusCode)
	})

	t.Run("Accepted", func(t *testing.T) {
		ctx := createTestContext()
		require.NoError(t, Accepted(ctx, nil))
		require.Equal(t, 202, ctx.Response.StatusCode)

		ctx = createTestContext()
		require.NoError(t, Accepted(ctx, map[string]string{"queued": "1"}))
		require.Equal(t, 202, ctx.Response.StatusCode)
	})

	t.Run("ListAndPagination", func(t *testing.T) {
		ctx := createTestContext()
		require.NoError(t, List(ctx, []string{"a"}, 1))
		_, ok := ctx.Response.Body.(ListResponse)
		require.True(t, ok)

		ctx = createTestContext()
		require.NoError(t, Paginated(ctx, []string{"a"}, "next", "", true))
		_, ok = ctx.Response.Body.(PaginatedResponse)
		require.True(t, ok)

		ctx = createTestContext()
		require.NoError(t, PaginatedWithTotal(ctx, []string{"a"}, "", "", false, 1))
		body, ok := ctx.Response.Body.(PaginatedResponse)
		require.True(t, ok)
		require.Equal(t, 1, body.Total)
	})

	t.Run("ContentTypes", func(t *testing.T) {
		ctx := createTestContext()
		require.NoError(t, ActivityPubResponse(ctx, map[string]string{"ok": "1"}))
		require.Equal(t, "application/activity+json", ctx.Response.Headers["Content-Type"])

		ctx = createTestContext()
		require.NoError(t, WebFingerResponse(ctx, map[string]string{"ok": "1"}))
		require.Equal(t, "application/jrd+json", ctx.Response.Headers["Content-Type"])

		ctx = createTestContext()
		require.NoError(t, NodeInfoResponse(ctx, map[string]string{"ok": "1"}))
		require.Equal(t, "application/json", ctx.Response.Headers["Content-Type"])
		require.Equal(t, "*", ctx.Response.Headers["Access-Control-Allow-Origin"])
	})

	t.Run("Redirects", func(t *testing.T) {
		ctx := createTestContext()
		require.NoError(t, Redirect(ctx, "https://example.com", false))
		require.Equal(t, 302, ctx.Response.StatusCode)
		require.Equal(t, "https://example.com", ctx.Response.Headers["Location"])

		ctx = createTestContext()
		require.NoError(t, SeeOther(ctx, "https://example.com"))
		require.Equal(t, 303, ctx.Response.StatusCode)

		ctx = createTestContext()
		require.NoError(t, TemporaryRedirect(ctx, "https://example.com"))
		require.Equal(t, 307, ctx.Response.StatusCode)
	})
}
