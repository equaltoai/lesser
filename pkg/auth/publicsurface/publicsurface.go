// Package publicsurface owns Lesser's importable public-surface decision.
package publicsurface

import (
	"net/http"
	"strings"
)

const apiV1AppsPath = "/api/v1/apps"

// ContractAuthClass describes auth requirements that are enforced outside the
// API gateway public-surface middleware but still need to be reflected in the
// generated public contract.
type ContractAuthClass string

const (
	// ContractAuthSetupBearer uses the temporary setup-session bearer token.
	ContractAuthSetupBearer ContractAuthClass = "setup_bearer"
	// ContractAuthBearerRequired uses the normal OAuth bearer-token posture.
	ContractAuthBearerRequired ContractAuthClass = "bearer_required"
	// ContractAuthInternalOnly is handler-enforced with internal instance keys.
	ContractAuthInternalOnly ContractAuthClass = "internal_only"
)

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

// ContractAuth returns handler-enforced contract auth requirements for routes
// that remain gate-reachable through IsPublic but must not be advertised as
// anonymous in the generated OpenAPI contract.
//
// This is additive contract metadata only. It intentionally does not change
// IsPublic's gate decision.
func ContractAuth(method, path string) (ContractAuthClass, bool) {
	method = strings.ToUpper(strings.TrimSpace(method))
	path = normalizeContractPath(path)

	switch method + " " + path {
	case "POST /setup/admin":
		return ContractAuthSetupBearer, true
	case "POST /setup/finalize":
		return ContractAuthBearerRequired, true
	case "POST /api/v1/notifications/deliver":
		return ContractAuthInternalOnly, true
	default:
		return "", false
	}
}

func normalizeContractPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	for strings.Contains(path, "//") {
		path = strings.ReplaceAll(path, "//", "/")
	}
	if len(path) > 1 {
		path = strings.TrimSuffix(path, "/")
	}
	return path
}
