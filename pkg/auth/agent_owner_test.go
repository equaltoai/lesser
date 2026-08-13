package auth

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAgentOwnerMatchesLocalPrincipal(t *testing.T) {
	actorURL := func(username string) string { return "https://example.com/users/" + username }
	tests := []struct {
		name      string
		owner     string
		principal string
		resolver  func(string) string
		want      bool
	}{
		{name: "at username", owner: " @Alice ", principal: "alice", resolver: actorURL, want: true},
		{name: "plain username", owner: "ALICE", principal: " alice ", resolver: actorURL, want: true},
		{name: "local actor URL", owner: "HTTPS://EXAMPLE.COM/users/alice", principal: "alice", resolver: actorURL, want: true},
		{name: "remote actor URL", owner: "https://remote.example/users/alice", principal: "alice", resolver: actorURL},
		{name: "actor URL without resolver", owner: "https://example.com/users/alice", principal: "alice"},
		{name: "path is not username", owner: "alice/path", principal: "alice", resolver: actorURL},
		{name: "empty owner", principal: "alice", resolver: actorURL},
		{name: "empty principal", owner: "alice", resolver: actorURL},
		{name: "URL principal matches URL owner", owner: "https://example.com/users/alice", principal: "https://example.com/users/alice", resolver: actorURL, want: true},
		{name: "URL principal matches URL owner without resolver", owner: "https://example.com/users/alice", principal: "https://example.com/users/alice", want: true},
		{name: "URL principal mismatch against URL owner", owner: "https://example.com/users/alice", principal: "https://example.com/users/bob", resolver: actorURL},
		{name: "URL principal does not match username owner", owner: "@alice", principal: "https://example.com/users/alice", resolver: actorURL},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, AgentOwnerMatchesLocalPrincipal(tc.owner, tc.principal, tc.resolver))
		})
	}
}
