package handlers

import (
	"strings"

	apptheory "github.com/theory-cloud/apptheory/runtime"
)

func headerValue(ctx *apptheory.Context, key string) string {
	if ctx == nil {
		return ""
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}

	if value := firstStringValue(ctx.Request.Headers, strings.ToLower(key)); value != "" {
		return value
	}
	if value := firstStringValue(ctx.Request.Headers, key); value != "" {
		return value
	}

	for headerKey, values := range ctx.Request.Headers {
		if !strings.EqualFold(strings.TrimSpace(headerKey), key) {
			continue
		}
		for _, v := range values {
			if vv := strings.TrimSpace(v); vv != "" {
				return vv
			}
		}
	}

	return ""
}

func queryValue(ctx *apptheory.Context, key string) string {
	if ctx == nil {
		return ""
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}

	if value := firstStringValue(ctx.Request.Query, key); value != "" {
		return value
	}

	for queryKey, values := range ctx.Request.Query {
		if !strings.EqualFold(strings.TrimSpace(queryKey), key) {
			continue
		}
		for _, v := range values {
			if vv := strings.TrimSpace(v); vv != "" {
				return vv
			}
		}
	}

	return ""
}

func queryValues(ctx *apptheory.Context, key string) []string {
	if ctx == nil {
		return nil
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}

	if values := ctx.Request.Query[key]; len(values) > 0 {
		out := make([]string, 0, len(values))
		for _, value := range values {
			if v := strings.TrimSpace(value); v != "" {
				out = append(out, v)
			}
		}
		return out
	}

	for queryKey, values := range ctx.Request.Query {
		if !strings.EqualFold(strings.TrimSpace(queryKey), key) {
			continue
		}

		out := make([]string, 0, len(values))
		for _, value := range values {
			if v := strings.TrimSpace(value); v != "" {
				out = append(out, v)
			}
		}

		return out
	}

	return nil
}

func setHeader(resp *apptheory.Response, key, value string) {
	if resp == nil {
		return
	}
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return
	}

	if resp.Headers == nil {
		resp.Headers = map[string][]string{}
	}
	resp.Headers[key] = []string{value}
}

func okJSON(value any) (*apptheory.Response, error) {
	return apptheory.JSON(200, value)
}

func createdJSON(value any) (*apptheory.Response, error) {
	return apptheory.JSON(201, value)
}

func noContent() *apptheory.Response {
	return &apptheory.Response{Status: 204}
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
