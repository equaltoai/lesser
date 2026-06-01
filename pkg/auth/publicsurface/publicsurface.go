// Package publicsurface owns Lesser's importable public-surface decision.
package publicsurface

import (
	"net/http"
	"strings"
)

const apiV1AppsPath = "/api/v1/apps"

// IsPublic reports whether the normalized method/path pair is in Lesser's
// explicitly allowlisted anonymous API surface.
//
// The default is deny: method/path pairs missing from this allowlist are not
// public.
//
//nolint:gocyclo,gocognit // Public surface allowlist requires many explicit method/path checks.
func IsPublic(method, path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}

	switch method {
	case http.MethodGet, http.MethodHead:
		switch path {
		case "/",
			"/robots.txt",
			"/.well-known/oauth-authorization-server",
			"/.well-known/nodeinfo",
			"/.well-known/lesser-soul-agent",
			"/nodeinfo/2.0",
			"/.well-known/reputation-keys",
			"/health",
			"/health/live",
			"/health/ready",
			"/auth/device",
			"/api/oembed",
			"/api/v1/instance",
			"/api/v2/instance",
			"/api/v1/custom_emojis",
			"/api/v1/directory",
			"/api/v1/announcements",
			"/api/v1/timelines/public",
			"/api/v1/timelines/link",
			"/api/v2/search",
			"/api/v2/suggestions",
			"/setup/status",
			"/oauth/authorize",
			"/api/v1/trust/jwks.json",
			"/api/v1/trust/attestations":
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

		// lesser.host trust proxy endpoints (public reads).
		if strings.HasPrefix(path, "/api/v1/trust/attestations/") {
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

		if path == "/api/v1/skills" || strings.HasPrefix(path, "/api/v1/skills/") {
			return path != "/api/v1/skills/resolve"
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
		case apiV1AppsPath,
			"/oauth/register",
			"/api/v1/accounts",
			"/api/v1/notifications/deliver",
			"/oauth/token",
			"/oauth/revoke",
			"/oauth/consent",
			"/oauth/device/code",
			"/oauth/device/verify",
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
