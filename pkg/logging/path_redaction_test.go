package logging

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitizeLogPathRedactsPrivateMintConversationIDs(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		in          string
		rawSecret   string
		wantPrefix  string
		mustNotLeak []string
	}{
		"single read": {
			in:         "/api/v1/souls/bound/me/mint-conversations/conv-private-1",
			rawSecret:  "conv-private-1",
			wantPrefix: "/api/v1/souls/bound/me/mint-conversations/conversation-sha256:",
		},
		"single read strips query cursor": {
			in:          "/api/v1/souls/bound/me/mint-conversations/conv-private-2?cursor=raw-cursor",
			rawSecret:   "conv-private-2",
			wantPrefix:  "/api/v1/souls/bound/me/mint-conversations/conversation-sha256:",
			mustNotLeak: []string{"raw-cursor"},
		},
		"single read strips fragment": {
			in:          "/api/v1/souls/bound/me/mint-conversations/conv-private-3#raw-fragment",
			rawSecret:   "conv-private-3",
			wantPrefix:  "/api/v1/souls/bound/me/mint-conversations/conversation-sha256:",
			mustNotLeak: []string{"raw-fragment"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := SanitizeLogPath(tc.in)
			require.Truef(t, strings.HasPrefix(got, tc.wantPrefix), "expected sanitized prefix %q, got %q", tc.wantPrefix, got)
			require.NotContains(t, got, tc.rawSecret)
			for _, forbidden := range tc.mustNotLeak {
				require.NotContains(t, got, forbidden)
			}
		})
	}
}

func TestSanitizeLogPathHandlesPrivateMintConversationListRoute(t *testing.T) {
	t.Parallel()

	raw := "/api/v1/souls/bound/me/mint-conversations?cursor=raw-cursor"
	got := SanitizeLogPath(raw)
	require.Equal(t, "/api/v1/souls/bound/me/mint-conversations", got)
	require.NotContains(t, got, "raw-cursor")
}

func TestSanitizeLogPathLeavesOtherRoutesUnchanged(t *testing.T) {
	t.Parallel()

	path := "/api/v1/statuses/conv-private-1?cursor=raw-cursor"
	require.Equal(t, path, SanitizeLogPath(path))
}
