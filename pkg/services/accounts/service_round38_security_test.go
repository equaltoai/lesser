package accounts

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// TestNormalizeUsername_PreservesRemoteDomainForURLActors verifies that
// the accounts service's normalizeUsername preserves remote domains for
// URL-format actor identifiers, preventing a same-named remote actor
// from being resolved as a local account. (CSR-052 regression probe)
func TestNormalizeUsername_PreservesRemoteDomainForURLActors(t *testing.T) {
	svc := NewService(nil, nil, nil, nil, nil, zap.NewNop(), "lesser.example")

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Remote URL actors must preserve the domain.
		{
			name:     "remote users URL",
			input:    "https://evil.example/users/admin",
			expected: "admin@evil.example",
		},
		{
			name:     "remote URL with @ prefix",
			input:    "https://evil.example/@admin",
			expected: "admin@evil.example",
		},
		{
			name:     "remote URL with actors path",
			input:    "https://remote.social/actors/admin",
			expected: "admin@remote.social",
		},
		{
			name:     "remote URL with trailing slash",
			input:    "https://evil.example/users/admin/",
			expected: "admin@evil.example",
		},

		// Local URL actors should NOT preserve the domain (it's local).
		{
			name:     "local users URL drops domain",
			input:    "https://lesser.example/users/alice",
			expected: "alice",
		},
		{
			name:     "local URL with @ prefix drops domain",
			input:    "https://lesser.example/@alice",
			expected: "alice",
		},

		// Handle (user@domain) format.
		{
			name:     "remote handle preserves domain",
			input:    "Alice@Evil.Example",
			expected: "alice@evil.example",
		},
		{
			name:     "local handle drops domain",
			input:    "alice@lesser.example",
			expected: "alice",
		},

		// Bare usernames (no domain indicator).
		{
			name:     "bare username unchanged",
			input:    "alice",
			expected: "alice",
		},
		{
			name:     "bare username with @ prefix",
			input:    "@alice",
			expected: "alice",
		},

		// Edge cases.
		{
			name:     "acct prefix with remote handle",
			input:    "acct:admin@evil.example",
			expected: "admin@evil.example",
		},
		{
			name:     "empty string",
			input:    "   ",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.normalizeUsername(tt.input)
			assert.Equal(t, tt.expected, got,
				"normalizeUsername(%q) = %q, want %q", tt.input, got, tt.expected)

			// Additional assertion: remote URLs must never normalize to a
			// bare username (no @ sign) when the domain is not local.
			if strings.HasPrefix(tt.input, "https://") || strings.HasPrefix(tt.input, "http://") {
				if strings.Contains(tt.expected, "@") {
					// Remote domain preserved — correct.
				} else if tt.expected != "" {
					// The result is a bare username. It must only happen for
					// local-domain URLs.
					assert.True(t,
						strings.Contains(strings.ToLower(tt.input), "lesser.example"),
						"URL input %q normalized to bare username %q but domain is not local",
						tt.input, tt.expected)
				}
			}
		})
	}
}

// TestNormalizeUsername_RemoteDomainCannotCollideWithLocal verifies that
// a remote actor URL with the same username as a local account produces
// a different normalized form (user@domain vs user), preventing
// identity collision. (CSR-052 regression probe)
func TestNormalizeUsername_RemoteDomainCannotCollideWithLocal(t *testing.T) {
	svc := NewService(nil, nil, nil, nil, nil, zap.NewNop(), "lesser.example")

	// Same username, different domains.
	localResult := svc.normalizeUsername("admin@lesser.example")
	remoteResult := svc.normalizeUsername("https://evil.example/users/admin")

	assert.Equal(t, "admin", localResult,
		"local user@domain should resolve to bare username")
	assert.Equal(t, "admin@evil.example", remoteResult,
		"remote URL must preserve domain to prevent collision")
	assert.NotEqual(t, localResult, remoteResult,
		"local and remote actors with same username must have different normalized forms")
}
