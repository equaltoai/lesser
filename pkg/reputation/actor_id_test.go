package reputation

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateActorID(t *testing.T) {
	// Valid actor URIs
	valid := []string{
		"https://example.com/users/alice",
		"http://remote.example/@bob",
		"https://example.com/users/Alice",                      // mixed-case path (URL path is case-sensitive)
		"https://EXAMPLE.com/users/alice",                       // uppercase host
		"HTTPS://example.com/users/alice",                      // uppercase scheme
		"https://social.example.co.uk/users/alice",              // multi-part TLD
		"https://example.com:443/users/alice",                  // explicit default port
		"https://192.168.1.1/users/alice",                      // IP address (though not recommended, valid URL)
		"https://example.com/api/v1/actors/alice",              // non-standard path structure
	}
	for _, actorID := range valid {
		t.Run("valid_"+actorID, func(t *testing.T) {
			require.NoError(t, ValidateActorID(actorID))
		})
	}

	invalid := []string{
		"",
		"acct:alice@example.com",                                  // non-HTTP scheme
		"https://evil.example@real.example/users/alice",           // userinfo injection
		"https://example.com/users/alice?next=https://evil.example", // query injection
		"https://example.com/users/alice#main-key",                // fragment injection
		"https://example.com/users/../admin",                      // path traversal (..)
		"https://example.com/users/%2e%2e/admin",                  // URL-encoded path traversal
		"https://example.com/users/alice\nHost: evil.example",     // newline injection
		"https://example.com/" + strings.Repeat("a", 2001),        // excessively long path
		// CSR-046 targeted edge cases:
		"https://example.com\x00/users/alice",                    // null byte in host
		"https://example.com/users/\x00alice",                    // null byte in path
		"https://example.com",                                    // bare host, no path
		"https://example.com/",                                   // root path only
		"https://example.com/users",                               // generic /users, no username
		"https://example.com/@",                                   // generic /@, no username
		"https://example.com/users/../alice",                      // path traversal mid-path
		"https://example.com/./users/alice",                       // dot segment
		"https://",                                                // no host
		"not a url",                                               // not a URL at all
		"ftp://example.com/users/alice",                           // non-HTTP scheme
	}
	for _, actorID := range invalid {
		t.Run("invalid_"+actorID, func(t *testing.T) {
			require.Error(t, ValidateActorID(actorID))
		})
	}
}

// TestCSR046_CanonicalActorIdempotency verifies that the canonical actor ID
// function produces stable, collision-free results for the same logical actor
// expressed through different valid URI serializations.
func TestCSR046_CanonicalActorIdempotency(t *testing.T) {
	// Same actor through different URL serializations should produce the same canonical form.
	samePairs := [][2]string{
		{"https://example.com/users/alice", "https://example.com/users/alice"},
		{"https://EXAMPLE.COM/users/alice", "https://example.com/users/alice"},
		{"HTTPS://example.com/users/alice", "https://example.com/users/alice"},
		{"https://example.com/users/alice/", "https://example.com/users/alice"},
	}
	for _, pair := range samePairs {
		t.Run("same_"+pair[0], func(t *testing.T) {
			require.True(t, sameCanonicalActorID(pair[0], pair[1]),
				"%q and %q should canonicalize to the same ID", pair[0], pair[1])
		})
	}

	// Different actors with deceptively similar URIs must produce different canonical forms.
	diffPairs := [][2]string{
		{"https://example.com/users/alice", "https://example.com/users/bob"},
		{"https://example.com/users/alice", "https://evil.com/users/alice"},
		{"https://example.com/users/Alice", "https://example.com/users/alice"}, // path case matters
		{"https://example.com/users/alice", "https://example.com:8443/users/alice"},
		{"https://example.com/users/alice", "https://example.com/@alice"},
	}
	for _, pair := range diffPairs {
		t.Run("diff_"+pair[0], func(t *testing.T) {
			require.False(t, sameCanonicalActorID(pair[0], pair[1]),
				"%q and %q should canonicalize to different IDs", pair[0], pair[1])
		})
	}
}

// TestCSR046_CanonicalActorIDRejectsCraftedPoisoning verifies that the canonical
// actor ID function specifically rejects crafted URIs that could be used to
// poison reputation records by masquerading as a different actor.
func TestCSR046_CanonicalActorIDRejectsCraftedPoisoning(t *testing.T) {
	poisoningAttempts := []string{
		// URI with embedded attacker host in userinfo position
		"https://attacker.example@victim.example/users/alice",
		// Query parameter masquerading as a different actor
		"https://victim.example/users/alice?real_actor=https://attacker.example/users/alice",
		// Fragment-based injection
		"https://attacker.example/users/alice#https://victim.example/users/alice",
		// Newline-injection attempting HTTP header smuggling
		"https://victim.example/users/alice\r\nX-Actor: https://attacker.example/users/alice",
		// URL-encoded newline
		"https://victim.example/users/alice%0d%0aX-Actor:%20https://attacker.example/users/alice",
		// Null byte truncation attempt
		"https://victim.example\x00.evil.example/users/alice",
		// Path traversal trying to hit a different actor resource
		"https://victim.example/users/../actors/alice",
	}
	for _, actorID := range poisoningAttempts {
		t.Run(actorID, func(t *testing.T) {
			require.Error(t, ValidateActorID(actorID),
				"crafted actor URI %q should be rejected", actorID)
		})
	}
}
