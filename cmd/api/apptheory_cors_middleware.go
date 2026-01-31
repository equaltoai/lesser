package main

import (
	"fmt"
	"net/http"
	"strings"

	appmiddleware "github.com/equaltoai/lesser/pkg/middleware"
	apptheory "github.com/theory-cloud/apptheory/runtime"
)

func applyWebClientCORSAppTheory(app *apptheory.App) {
	if app == nil {
		return
	}
	app.Use(appTheoryCORSMiddleware(appmiddleware.GetWebClientCORSConfig()))
}

func appTheoryCORSMiddleware(config appmiddleware.CORSConfig) apptheory.Middleware {
	return func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			origin := extractOriginFromAppTheoryRequest(ctx)
			allowed, useWildcard := checkOriginAllowed(origin, config.AllowedOrigins)

			// Handle preflight requests.
			if strings.EqualFold(ctx.Request.Method, http.MethodOptions) {
				headers := map[string][]string{}
				setOriginHeader(headers, origin, allowed, useWildcard)
				headers["Access-Control-Allow-Methods"] = []string{strings.Join(config.AllowedMethods, ", ")}
				headers["Access-Control-Allow-Headers"] = []string{strings.Join(config.AllowedHeaders, ", ")}
				headers["Access-Control-Max-Age"] = []string{fmt.Sprintf("%d", config.MaxAge)}
				if config.AllowCredentials {
					headers["Access-Control-Allow-Credentials"] = []string{"true"}
				}
				headers["Vary"] = []string{"Origin"}

				return &apptheory.Response{Status: http.StatusNoContent, Headers: headers}, nil
			}

			resp, err := next(ctx)
			if err != nil || resp == nil {
				return resp, err
			}
			if resp.Headers == nil {
				resp.Headers = map[string][]string{}
			}

			setOriginHeader(resp.Headers, origin, allowed, useWildcard)

			if len(config.ExposedHeaders) > 0 {
				resp.Headers["Access-Control-Expose-Headers"] = []string{strings.Join(config.ExposedHeaders, ", ")}
			}

			if config.AllowCredentials {
				resp.Headers["Access-Control-Allow-Credentials"] = []string{"true"}
			}

			setVaryHeader(resp.Headers, "Origin")

			return resp, nil
		}
	}
}

func extractOriginFromAppTheoryRequest(ctx *apptheory.Context) string {
	if ctx == nil {
		return ""
	}

	if origin := firstHeaderValue(ctx.Request.Headers, "Origin"); origin != "" {
		return origin
	}
	return firstHeaderValue(ctx.Request.Headers, "origin")
}

func firstHeaderValue(headers map[string][]string, key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	values := headers[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func checkOriginAllowed(origin string, allowedOrigins []string) (allowed bool, useWildcard bool) {
	origin = strings.TrimSpace(origin)
	for _, allowedOrigin := range allowedOrigins {
		allowedOrigin = strings.TrimSpace(allowedOrigin)
		if allowedOrigin == "*" {
			return true, true
		}
		if allowedOrigin != "" && allowedOrigin == origin {
			return true, false
		}
	}
	return false, false
}

func setOriginHeader(headers map[string][]string, origin string, allowed, useWildcard bool) {
	if !allowed || headers == nil {
		return
	}

	if useWildcard {
		headers["Access-Control-Allow-Origin"] = []string{"*"}
		return
	}
	if origin != "" {
		headers["Access-Control-Allow-Origin"] = []string{origin}
	}
}

func setVaryHeader(headers map[string][]string, add ...string) {
	if headers == nil {
		return
	}

	existingKey := "Vary"
	existing := headers[existingKey]
	if len(existing) == 0 {
		if alt := headers["vary"]; len(alt) > 0 {
			existingKey = "vary"
			existing = alt
		}
	}
	headers[existingKey] = apptheory.Vary(existing, add...)
}

