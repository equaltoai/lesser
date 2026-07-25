package main

import (
	"net/http"
	"strings"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/auth/publicsurface"
	"github.com/equaltoai/lesser/pkg/common"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
)

// createPublicSurfaceMiddleware enforces Lesser's default-deny public surface policy.
//
// Policy: endpoints are treated as non-public unless explicitly allowlisted.
// The importable source of truth lives in pkg/auth/publicsurface.
func createPublicSurfaceMiddleware() apptheory.Middleware {
	return func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			if ctx == nil {
				return next(ctx)
			}

			method := strings.ToUpper(strings.TrimSpace(ctx.Request.Method))
			path := strings.TrimSpace(ctx.Request.Path)

			// CORS preflight should always pass through.
			if method == http.MethodOptions {
				return next(ctx)
			}

			if apiRequestIsPublic(method, path) {
				return next(ctx)
			}

			if auth.IsAuthenticated(ctx) {
				return next(ctx)
			}

			return common.RespondMissingAuth(ctx)
		}
	}
}

func apiRequestIsPublic(method, path string) bool {
	return publicsurface.IsPublic(method, path)
}
