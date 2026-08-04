package federation

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/stretchr/testify/require"
)

func TestNormalizeActorDomainCanonicalizesAndRejectsUnsafeIPForms(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "hostname", input: " Example.COM ", want: "example.com"},
		{name: "hostname with port and path", input: "https://Example.COM:8443/users/alice", want: "example.com"},
		{name: "hostname with service port", input: "example.com:https", want: "example.com"},
		{name: "canonical IPv4", input: "192.0.2.1", want: "192.0.2.1"},
		{name: "bracketed IPv6", input: "[2001:db8::1]", want: "2001:db8::1"},
		{name: "bracketed IPv6 with port", input: "[2001:db8::1]:8443", want: "2001:db8::1"},
		{name: "expanded IPv6", input: "2001:0db8:0000:0000:0000:0000:0000:0001", want: "2001:db8::1"},
		{name: "mapped IPv4", input: "::ffff:192.0.2.1", want: "192.0.2.1"},
		{name: "legacy global integer", input: "0xc0000201", want: "192.0.2.1"},
		{name: "loopback short", input: "127.1", want: ""},
		{name: "loopback integer", input: "2130706433", want: ""},
		{name: "loopback hex", input: "0x7f.0.0.1", want: ""},
		{name: "loopback octal", input: "0177.0.0.1", want: ""},
		{name: "unspecified", input: "0", want: ""},
		{name: "zoned link local", input: "[fe80::1%eth0]", want: ""},
		{name: "zoned global IPv6", input: "[2001:db8::1%eth0]", want: "2001:db8::1"},
		{name: "link local multicast", input: "ff02::1", want: ""},
		{name: "missing bracket", input: "[2001:db8::1", want: ""},
		{name: "invalid bracket suffix", input: "[2001:db8::1]invalid", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, normalizeActorDomain(tt.input))
		})
	}
}

func TestLegacyIPv4ParserRejectsMalformedFamilies(t *testing.T) {
	for _, value := range []string{
		"1.2.3.4.5",
		"1..2",
		"not-a-number",
		"256.1",
		"1.16777216",
		"1.256.1",
		"1.1.65536",
		"1.2.3.256",
	} {
		_, ok := parseLegacyIPv4(value)
		require.False(t, ok, value)
	}
}

func TestActorDomainIPv6SpellingsDeduplicateAcrossDerivationPaths(t *testing.T) {
	derived := deriveActorDomain(&activitypub.Actor{
		BaseObject: activitypub.BaseObject{ID: "https://[2001:0db8:0000:0000:0000:0000:0000:0001]/users/alice"},
	})
	_, parsed, err := parseLooseHandle("alice@[2001:db8::1]")
	require.NoError(t, err)

	domains := map[string]struct{}{derived: {}, parsed: {}}
	require.Equal(t, map[string]struct{}{"2001:db8::1": {}}, domains)
}

func TestNormalizeActorDomainRejectsPercentEncodedHostnames(t *testing.T) {
	for _, value := range []string{
		"evil.com%00.example.com",
		"mastodon.social%x.attacker.example",
		"exam%70le.com",
		"100%.example.com",
		"a%b%c.example.com",
	} {
		t.Run(value, func(t *testing.T) {
			require.Empty(t, normalizeActorDomain(value))

			_, _, err := parseLooseHandle("alice@" + value)
			require.Error(t, err)
		})
	}

	left := normalizeActorDomain("100%.a.example")
	right := normalizeActorDomain("100%.b.example")
	require.Empty(t, left)
	require.Empty(t, right)
	require.NotEqual(t, "0.0.0.100", left)
	require.NotEqual(t, "0.0.0.100", right)
}
