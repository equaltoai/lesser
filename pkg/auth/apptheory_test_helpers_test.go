package auth

import (
	"strings"

	apptheory "github.com/theory-cloud/apptheory/runtime"
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
			k = strings.ToLower(strings.TrimSpace(k))
			if k == "" {
				continue
			}
			ctx.Request.Headers[k] = []string{v}
		}
	}
}

func withQueryParams(params map[string]string) apptheoryContextOption {
	return func(ctx *apptheory.Context) {
		if ctx.Request.Query == nil {
			ctx.Request.Query = map[string][]string{}
		}
		for k, v := range params {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			ctx.Request.Query[k] = []string{v}
		}
	}
}
