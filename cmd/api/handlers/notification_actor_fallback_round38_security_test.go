package handlers

import (
	"net/url"
	"strings"
	"testing"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFallbackNotificationActor_ForeignDomainSurfaced verifies that
// the REST handler's fallbackNotificationActor surfaces foreign domains
// in the actor display name. (CSR-051 regression probe)
func TestFallbackNotificationActor_ForeignDomainSurfaced(t *testing.T) {
	cfg := &config.Config{
		Domain: "lesser.example",
	}
	h := &Handler{cfg: cfg}

	tests := []struct {
		name         string
		actorID      string
		wantID       string
		wantURL      string
		domainInName bool
		wantNil      bool
	}{
		{
			name:         "foreign URL actor surfaces domain in name",
			actorID:      "https://evil.example/users/admin",
			wantID:       "https://evil.example/users/admin",
			wantURL:      "https://evil.example/users/admin",
			domainInName: true,
		},
		{
			name:    "local bare username creates local URLs",
			actorID: "alice",
			wantURL: "https://lesser.example/@alice",
		},
		{
			name:    "email-like address treated as opaque",
			actorID: "admin@evil.example",
			wantID:  "admin@evil.example",
			// No wantURL — opaque IDs have no URL.
		},
		{
			name:    "URL with userinfo is rejected",
			actorID: "https://evil@lesser.example/users/admin",
			wantNil: true,
		},
		{
			name:    "URL with query string is rejected",
			actorID: "https://evil.example/users/admin?evil=true",
			wantNil: true,
		},
		{
			name:    "URL with fragment is rejected",
			actorID: "https://evil.example/users/admin#evil",
			wantNil: true,
		},
		{
			name:    "URL with non-http scheme is rejected",
			actorID: "javascript://evil.example/users/admin",
			wantNil: true,
		},
		{
			name:    "actorID with control characters is rejected",
			actorID: "admin\x00evil",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actor := h.fallbackNotificationActor(tt.actorID)
			if tt.wantNil {
				assert.Nil(t, actor, "expected nil for %q", tt.actorID)
				return
			}
			require.NotNil(t, actor, "unexpected nil for %q", tt.actorID)

			if tt.wantID != "" {
				assert.Equal(t, tt.wantID, actor.ID)
			}
			if tt.wantURL != "" {
				assert.Equal(t, tt.wantURL, actor.URL)
			}
			if tt.domainInName {
				assert.True(t, strings.Contains(actor.Name, "@"),
					"actor name for foreign URL should contain @domain, got %q", actor.Name)
			}
		})
	}
}

// TestValidNotificationFallbackActorURL_RejectsDangerousSchemes verifies
// that only http/https schemes are accepted for actor URLs used as
// notification fallback actor IDs. (CSR-051 regression probe)
func TestValidNotificationFallbackActorURL_RejectsDangerousSchemes(t *testing.T) {
	tests := []struct {
		name string
		url  string
		ok   bool
	}{
		{name: "https ok", url: "https://evil.example/users/admin", ok: true},
		{name: "http ok", url: "http://evil.example/users/admin", ok: true},
		{name: "javascript rejected", url: "javascript:alert(1)", ok: false},
		{name: "data rejected", url: "data:text/html,<script>alert(1)</script>", ok: false},
		{name: "ftp rejected", url: "ftp://evil.example/users/admin", ok: false},
		{name: "empty host rejected", url: "https:///users/admin", ok: false},
		{name: "userinfo rejected", url: "https://evil@evil.example/users/admin", ok: false},
		{name: "query rejected", url: "https://evil.example/users/admin?q=1", ok: false},
		{name: "fragment rejected", url: "https://evil.example/users/admin#x", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := url.Parse(tt.url)
			require.NoError(t, err, "url.Parse should not error for %q", tt.url)
			got := validNotificationFallbackActorURL(parsed)
			assert.Equal(t, tt.ok, got, "validNotificationFallbackActorURL(%q) = %v, want %v", tt.url, got, tt.ok)
		})
	}
}
