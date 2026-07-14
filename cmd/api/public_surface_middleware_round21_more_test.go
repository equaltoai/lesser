package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAPIRequestIsPublic_Round21(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		want   bool
	}{
		{"empty path", http.MethodGet, "", false},
		{"root is public", http.MethodGet, "/", true},
		{"robots is public", http.MethodHead, "/robots.txt", true},
		{"embed prefix is public", http.MethodGet, "/embed/abc", true},
		{"instance prefix is public", http.MethodGet, "/api/v1/instance/peers", true},
		{"trends v1 prefix is public", http.MethodGet, "/api/v1/trends/tags", true},
		{"timelines tag is public", http.MethodGet, "/api/v1/timelines/tag/golang", true},
		{"public status read is public", http.MethodGet, "/api/v1/statuses/123", true},
		{"status source is not public", http.MethodGet, "/api/v1/statuses/123/source", false},
		{"status favourited_by is not public", http.MethodGet, "/api/v1/statuses/123/favourited_by", false},
		{"status reblogged_by is not public", http.MethodGet, "/api/v1/statuses/123/reblogged_by", false},
		{"accounts statuses is public", http.MethodGet, "/api/v1/accounts/1/statuses", true},
		{"accounts notes is public", http.MethodGet, "/api/v1/accounts/1/notes", true},
		{"accounts profile is public", http.MethodGet, "/api/v1/accounts/1", true},
		{"accounts lookup is public", http.MethodGet, "/api/v1/accounts/lookup", true},
		{"accounts relationships is not public", http.MethodGet, "/api/v1/accounts/relationships", false},
		{"accounts verify credentials is not public", http.MethodGet, "/api/v1/accounts/verify_credentials", false},
		{"accounts search is public", http.MethodGet, "/api/v1/accounts/search", true},
		{"agents directory is public", http.MethodGet, "/api/v1/agents", true},
		{"agent profile is public", http.MethodGet, "/api/v1/agents/della-marlowe", true},
		{"agent activity is not public", http.MethodGet, "/api/v1/agents/della-marlowe/activity", false},
		{"agent memory is not public", http.MethodGet, "/api/v1/agents/memory/search", false},
		{"search statuses is public", http.MethodGet, "/api/v1/search/statuses", true},
		{"notes read is public", http.MethodGet, "/api/v1/notes/1", true},
		{"soul well-known proof is public", http.MethodGet, "/.well-known/lesser-soul-agent", true},
		{"oauth metadata is public", http.MethodGet, "/.well-known/oauth-authorization-server", true},
		{"trust jwks proxy is public", http.MethodGet, "/api/v1/trust/jwks.json", true},
		{"trust attestations proxy is public", http.MethodGet, "/api/v1/trust/attestations", true},
		{"trust attestation id proxy is public", http.MethodGet, "/api/v1/trust/attestations/abc", true},
		{"trust previews are not public", http.MethodGet, "/api/v1/trust/previews/abc", false},

		{"apps registration is public", http.MethodPost, "/api/v1/apps", true},
		{"oauth dynamic registration is public", http.MethodPost, "/oauth/register", true},
		{"oauth token is public", http.MethodPost, "/oauth/token", true},
		{"oauth revoke is public", http.MethodPost, "/oauth/revoke", true},
		{"wallet login is public", http.MethodPost, "/auth/wallet/login", true},
		{"post search statuses is public", http.MethodPost, "/api/v1/search/statuses", true},
		{"agent register challenge is public", http.MethodPost, "/api/v1/agents/register/challenge", true},
		{"agent register is public", http.MethodPost, "/api/v1/agents/register", true},
		{"agent auth challenge is public", http.MethodPost, "/api/v1/agents/auth/challenge", true},
		{"agent auth token is public", http.MethodPost, "/api/v1/agents/auth/token", true},
		{"agent lease session challenge is public", http.MethodPost, "/api/v1/agents/della-marlowe/access-leases/lease-1/session-key/challenge", true},
		{"agent lease session key is public", http.MethodPost, "/api/v1/agents/della-marlowe/access-leases/lease-1/session-key", true},
		{"agent lease renew challenge is public", http.MethodPost, "/api/v1/agents/della-marlowe/access-leases/lease-1/renew/challenge", true},
		{"agent lease token is public", http.MethodPost, "/api/v1/agents/della-marlowe/access-leases/lease-1/token", true},
		{"agent lease creation remains private", http.MethodPost, "/api/v1/agents/della-marlowe/access-leases", false},
		{"agent lease owner challenge remains private", http.MethodPost, "/api/v1/agents/della-marlowe/access-leases/challenge/principal", false},
		{"notification delivery uses handler auth", http.MethodPost, "/api/v1/notifications/deliver", true},
		{"notifications not public", http.MethodPost, "/api/v1/notifications", false},

		{"unsupported method not public", http.MethodPut, "/oauth/token", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, apiRequestIsPublic(tt.method, tt.path))
		})
	}
}
