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
		{"accounts profile is not public", http.MethodGet, "/api/v1/accounts/1", false},
		{"accounts search is public", http.MethodGet, "/api/v1/accounts/search", true},
		{"search statuses is public", http.MethodGet, "/api/v1/search/statuses", true},
		{"notes read is public", http.MethodGet, "/api/v1/notes/1", true},

		{"apps registration is public", http.MethodPost, "/api/v1/apps", true},
		{"oauth token is public", http.MethodPost, "/oauth/token", true},
		{"oauth revoke is public", http.MethodPost, "/oauth/revoke", true},
		{"wallet login is public", http.MethodPost, "/auth/wallet/login", true},
		{"post search statuses is public", http.MethodPost, "/api/v1/search/statuses", true},
		{"notifications not public", http.MethodPost, "/api/v1/notifications", false},

		{"unsupported method not public", http.MethodPut, "/oauth/token", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, apiRequestIsPublic(tt.method, tt.path))
		})
	}
}
