package main

import (
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/auth/publicsurface"
	"github.com/stretchr/testify/require"
)

func TestPublicSurfacePackageMatchesLegacyGateDecision(t *testing.T) {
	// These expectations snapshot apiRequestIsPublic's behavior before the
	// public-surface decision moved into pkg/auth/publicsurface. Keep this table
	// comprehensive so the importable SSOT stays behavior-neutral.
	tests := []struct {
		name   string
		method string
		path   string
		want   bool
	}{
		// Exact GET/HEAD allowlist.
		{"get root", http.MethodGet, "/", true},
		{"get robots", http.MethodGet, "/robots.txt", true},
		{"get oauth metadata", http.MethodGet, "/.well-known/oauth-authorization-server", true},
		{"get nodeinfo metadata", http.MethodGet, "/.well-known/nodeinfo", true},
		{"get lesser soul agent", http.MethodGet, "/.well-known/lesser-soul-agent", true},
		{"get nodeinfo 2", http.MethodGet, "/nodeinfo/2.0", true},
		{"get reputation keys", http.MethodGet, "/.well-known/reputation-keys", true},
		{"get health", http.MethodGet, "/health", true},
		{"get health live", http.MethodGet, "/health/live", true},
		{"get health ready", http.MethodGet, "/health/ready", true},
		{"get auth device", http.MethodGet, "/auth/device", true},
		{"get oembed", http.MethodGet, "/api/oembed", true},
		{"get v1 instance", http.MethodGet, "/api/v1/instance", true},
		{"get v2 instance", http.MethodGet, "/api/v2/instance", true},
		{"get custom emojis", http.MethodGet, "/api/v1/custom_emojis", true},
		{"get directory", http.MethodGet, "/api/v1/directory", true},
		{"get announcements", http.MethodGet, "/api/v1/announcements", true},
		{"get public timeline", http.MethodGet, "/api/v1/timelines/public", true},
		{"get link timeline", http.MethodGet, "/api/v1/timelines/link", true},
		{"get v2 search", http.MethodGet, "/api/v2/search", true},
		{"get v2 suggestions", http.MethodGet, "/api/v2/suggestions", true},
		{"get setup status", http.MethodGet, "/setup/status", true},
		{"get oauth authorize", http.MethodGet, "/oauth/authorize", true},
		{"get trust jwks", http.MethodGet, "/api/v1/trust/jwks.json", true},
		{"get trust attestations", http.MethodGet, "/api/v1/trust/attestations", true},
		{"head exact allowlist", http.MethodHead, "/api/v1/instance", true},

		// Representative GET prefix allowlist members.
		{"get embed prefix", http.MethodGet, "/embed/status/1", true},
		{"get instance prefix", http.MethodGet, "/api/v1/instance/peers", true},
		{"get trends v1 prefix", http.MethodGet, "/api/v1/trends/tags", true},
		{"get trends v2 prefix", http.MethodGet, "/api/v2/trends/links", true},
		{"get timeline tag prefix", http.MethodGet, "/api/v1/timelines/tag/golang", true},
		{"get trust attestation id prefix", http.MethodGet, "/api/v1/trust/attestations/abc", true},
		{"get status read prefix", http.MethodGet, "/api/v1/statuses/123", true},
		{"get status context remains public", http.MethodGet, "/api/v1/statuses/123/context", true},
		{"get status source excluded", http.MethodGet, "/api/v1/statuses/123/source", false},
		{"get status favourited by excluded", http.MethodGet, "/api/v1/statuses/123/favourited_by", false},
		{"get status reblogged by excluded", http.MethodGet, "/api/v1/statuses/123/reblogged_by", false},
		{"get account statuses branch", http.MethodGet, "/api/v1/accounts/123/statuses", true},
		{"get account notes branch", http.MethodGet, "/api/v1/accounts/123/notes", true},
		{"get accounts search prefix", http.MethodGet, "/api/v1/accounts/search", true},
		{"get accounts search suggestions prefix", http.MethodGet, "/api/v1/accounts/search/suggestions", true},
		{"get skills exact", http.MethodGet, "/api/v1/skills", true},
		{"get skills member prefix", http.MethodGet, "/api/v1/skills/translate", true},
		{"get skills resolve exact excluded", http.MethodGet, "/api/v1/skills/resolve", false},
		{"get skills resolve child remains prefix-public", http.MethodGet, "/api/v1/skills/resolve/child", true},
		{"get search statuses prefix", http.MethodGet, "/api/v1/search/statuses/abc", true},
		{"get notes prefix", http.MethodGet, "/api/v1/notes/abc", true},

		// Exact POST allowlist.
		{"post apps", http.MethodPost, "/api/v1/apps", true},
		{"post oauth register", http.MethodPost, "/oauth/register", true},
		{"post accounts", http.MethodPost, "/api/v1/accounts", true},
		{"post notifications deliver", http.MethodPost, "/api/v1/notifications/deliver", true},
		{"post oauth token", http.MethodPost, "/oauth/token", true},
		{"post oauth revoke", http.MethodPost, "/oauth/revoke", true},
		{"post oauth consent", http.MethodPost, "/oauth/consent", true},
		{"post device code", http.MethodPost, "/oauth/device/code", true},
		{"post device verify", http.MethodPost, "/oauth/device/verify", true},
		{"post setup challenge", http.MethodPost, "/setup/bootstrap/challenge", true},
		{"post setup verify", http.MethodPost, "/setup/bootstrap/verify", true},
		{"post setup admin", http.MethodPost, "/setup/admin", true},
		{"post setup finalize", http.MethodPost, "/setup/finalize", true},
		{"post webauthn login begin", http.MethodPost, "/api/v1/auth/webauthn/login/begin", true},
		{"post webauthn login finish", http.MethodPost, "/api/v1/auth/webauthn/login/finish", true},
		{"post wallet challenge", http.MethodPost, "/auth/wallet/challenge", true},
		{"post wallet verify", http.MethodPost, "/auth/wallet/verify", true},
		{"post wallet login", http.MethodPost, "/auth/wallet/login", true},
		{"post wallet link", http.MethodPost, "/auth/wallet/link", true},
		{"post search statuses", http.MethodPost, "/api/v1/search/statuses", true},

		// Known-private/default-deny routes and method/path edge cases.
		{"empty path", http.MethodGet, "", false},
		{"blank path", http.MethodGet, "   ", false},
		{"lowercase method remains non-public at function boundary", "get", "/api/v1/instance", false},
		{"get notifications private", http.MethodGet, "/api/v1/notifications", false},
		{"get verify credentials private", http.MethodGet, "/api/v1/accounts/verify_credentials", false},
		{"get account profile private", http.MethodGet, "/api/v1/accounts/123", false},
		{"get account pinned statuses private", http.MethodGet, "/api/v1/accounts/123/pinned_statuses", false},
		{"get trust preview private", http.MethodGet, "/api/v1/trust/previews/abc", false},
		{"post notifications private", http.MethodPost, "/api/v1/notifications", false},
		{"post apps child private", http.MethodPost, "/api/v1/apps/123", false},
		{"put oauth token private", http.MethodPut, "/oauth/token", false},
		{"delete status private", http.MethodDelete, "/api/v1/statuses/123", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			legacy := apiRequestIsPublic(tt.method, tt.path)
			actual := publicsurface.IsPublic(tt.method, tt.path)

			require.Equal(t, tt.want, legacy, "legacy pre-refactor snapshot expectation changed")
			require.Equal(t, legacy, actual, "importable public-surface decision drifted from gate helper")
		})
	}
}
