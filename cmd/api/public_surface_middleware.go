package main

import (
	"net/http"
	"strings"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	apptheory "github.com/theory-cloud/apptheory/runtime"
)

// createPublicSurfaceMiddleware enforces Lesser's default-deny public surface policy.
//
// Policy: endpoints are treated as non-public unless explicitly allowlisted here.
// Intended source of truth: docs/security-public-surface.md.
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

//nolint:gocognit // Public surface allowlist requires many explicit method/path checks.
func apiRequestIsPublic(method, path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}

	switch method {
	case http.MethodGet, http.MethodHead:
		switch path {
		case "/",
			"/robots.txt",
			"/.well-known/nodeinfo",
			"/nodeinfo/2.0",
			"/.well-known/reputation-keys",
			"/api/oembed",
			"/api/v1/instance",
			"/api/v2/instance",
			"/api/v1/timelines/public",
			"/api/v1/timelines/link",
			"/api/v2/search",
			"/api/v2/suggestions",
			"/setup/status",
			"/oauth/authorize":
			return true
		}

		if strings.HasPrefix(path, "/embed/") {
			return true
		}

		if strings.HasPrefix(path, "/api/v1/instance/") {
			return true
		}

		if strings.HasPrefix(path, "/api/v1/trends") || strings.HasPrefix(path, "/api/v2/trends") {
			return true
		}

		if strings.HasPrefix(path, "/api/v1/timelines/tag/") {
			return true
		}

		// Public status reads (visibility still enforced in handlers/services).
		if strings.HasPrefix(path, "/api/v1/statuses/") {
			// Explicitly exclude sensitive per-status reads.
			if strings.HasSuffix(path, "/source") || strings.HasSuffix(path, "/favourited_by") || strings.HasSuffix(path, "/reblogged_by") {
				return false
			}
			return true
		}

		if strings.HasPrefix(path, "/api/v1/accounts/") {
			if strings.Contains(path, "/statuses") || strings.Contains(path, "/notes") {
				return true
			}
		}

		if strings.HasPrefix(path, "/api/v1/accounts/search") {
			return true
		}

		if strings.HasPrefix(path, "/api/v1/search/statuses") {
			return true
		}

		if strings.HasPrefix(path, "/api/v1/notes/") {
			return true
		}

		return false

	case http.MethodPost:
		switch path {
		case "/api/v1/apps",
			"/api/v1/accounts",
			"/oauth/token",
			"/oauth/consent",
			"/setup/bootstrap/challenge",
			"/setup/bootstrap/verify",
			"/setup/admin",
			"/setup/finalize",
			"/api/v1/auth/webauthn/login/begin",
			"/api/v1/auth/webauthn/login/finish":
			return true
		case "/auth/wallet/challenge",
			"/auth/wallet/verify",
			"/auth/wallet/login",
			"/auth/wallet/link":
			return true
		case "/api/v1/search/statuses":
			return true
		default:
			return false
		}
	default:
		return false
	}
}
