package common

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
)

type apptheoryContextOption func(*apptheory.Context)

func newTestContext(method, path string, opts ...apptheoryContextOption) *apptheory.Context {
	ctx := &apptheory.Context{
		Request: apptheory.Request{
			Method:  method,
			Path:    path,
			Headers: map[string][]string{},
			Query:   map[string][]string{},
		},
	}
	for _, opt := range opts {
		opt(ctx)
	}
	return ctx
}

func withHeaders(headers map[string]string) apptheoryContextOption {
	return func(ctx *apptheory.Context) {
		if ctx.Request.Headers == nil {
			ctx.Request.Headers = map[string][]string{}
		}
		for k, v := range headers {
			if strings.TrimSpace(k) == "" {
				continue
			}
			ctx.Request.Headers[strings.ToLower(strings.TrimSpace(k))] = []string{v}
		}
	}
}

func withQueryParams(params map[string]string) apptheoryContextOption {
	return func(ctx *apptheory.Context) {
		if ctx.Request.Query == nil {
			ctx.Request.Query = map[string][]string{}
		}
		for k, v := range params {
			if strings.TrimSpace(k) == "" {
				continue
			}
			ctx.Request.Query[strings.TrimSpace(k)] = []string{v}
		}
	}
}

func parseResponse(t *testing.T, resp *apptheory.Response) (int, StandardErrorResponse) {
	t.Helper()

	require.NotNil(t, resp)

	var out StandardErrorResponse
	require.NoError(t, json.Unmarshal(resp.Body, &out), "response body should be valid JSON")
	return resp.Status, out
}
