package crawler

import (
	"testing"

	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

func TestParseProtectionMode(t *testing.T) {
	require.Equal(t, protectionModeOff, parseProtectionMode(""))
	require.Equal(t, protectionModeOff, parseProtectionMode("nope"))
	require.Equal(t, protectionModeObserve, parseProtectionMode("observe"))
	require.Equal(t, protectionModeObserve, parseProtectionMode(" OBSERVE "))
	require.Equal(t, protectionModeLimit, parseProtectionMode("limit"))
	require.Equal(t, protectionModeBlock, parseProtectionMode("block"))
}

func TestMiddleware_Off_IsNoop(t *testing.T) {
	called := 0
	next := func(ctx *apptheory.Context) (*apptheory.Response, error) {
		called++
		require.NotNil(t, ctx)
		require.Nil(t, ctx.Get(contextCrawlerCategoryKey))
		require.Nil(t, ctx.Get(contextCrawlerReasonKey))
		return apptheory.Text(200, "ok"), nil
	}

	ctx := &apptheory.Context{Request: apptheory.Request{
		Method:  "GET",
		Path:    "/users/alice",
		Headers: map[string][]string{"user-agent": {"GPTBot/1.0"}, "accept": {"application/activity+json"}},
	}}

	mw := Middleware(protectionModeOff, zap.NewNop())
	resp, err := mw(next)(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 200, resp.Status)
	require.Equal(t, 1, called)
}

func TestMiddleware_Observe_SetsContext(t *testing.T) {
	called := 0
	next := func(ctx *apptheory.Context) (*apptheory.Response, error) {
		called++
		require.Equal(t, "ai_crawler", ctx.Get(contextCrawlerCategoryKey))
		require.Equal(t, "ua:gptbot", ctx.Get(contextCrawlerReasonKey))
		return apptheory.Text(200, "ok"), nil
	}

	ctx := &apptheory.Context{Request: apptheory.Request{
		Method:  "GET",
		Path:    "/users/alice",
		Headers: map[string][]string{"user-agent": {"GPTBot/1.0"}, "accept": {"application/activity+json"}},
	}}

	mw := Middleware(protectionModeObserve, zap.NewNop())
	resp, err := mw(next)(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 200, resp.Status)
	require.Equal(t, 1, called)
}

func TestMiddleware_NilContext(t *testing.T) {
	called := 0
	next := func(*apptheory.Context) (*apptheory.Response, error) {
		called++
		return apptheory.Text(200, "ok"), nil
	}

	mw := Middleware(protectionModeObserve, zap.NewNop())
	resp, err := mw(next)(nil)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 200, resp.Status)
	require.Equal(t, 1, called)
}
