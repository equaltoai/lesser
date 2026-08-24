package handlers

import (
	"strings"

	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
)

func headerValue(ctx *apptheory.Context, key string) string {
	if ctx == nil {
		return ""
	}
	return ctx.Header(strings.TrimSpace(key))
}

func queryValue(ctx *apptheory.Context, key string) string {
	if ctx == nil {
		return ""
	}
	return ctx.Query(strings.TrimSpace(key))
}

func queryValues(ctx *apptheory.Context, key string) []string {
	if ctx == nil {
		return nil
	}
	return ctx.QueryAll(strings.TrimSpace(key))
}

func setHeader(resp *apptheory.Response, key, value string) {
	if resp == nil {
		return
	}
	resp.SetHeader(key, value)
}

func okJSON(value any) (*apptheory.Response, error) {
	return apptheory.JSON(200, value)
}

func createdJSON(value any) (*apptheory.Response, error) {
	return apptheory.CreatedJSON(value)
}

func noContent() *apptheory.Response {
	return apptheory.NoContent()
}

func firstStringValue(values map[string][]string, key string) string {
	if values == nil {
		return ""
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	for _, v := range values[key] {
		if vv := strings.TrimSpace(v); vv != "" {
			return vv
		}
	}
	return ""
}
