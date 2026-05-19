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

func TestSanitizeLogPathHandlesAPIConversationRoutes(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		in          string
		rawSecret   string
		want        string
		wantPrefix  string
		wantSuffix  string
		mustNotLeak []string
	}{
		"list strips query": {
			in:          "/api/v1/conversations?limit=20&max_id=raw-cursor",
			want:        "/api/v1/conversations",
			mustNotLeak: []string{"raw-cursor"},
		},
		"lookup strips counterpart query": {
			in:          "/api/v1/conversations/lookup?counterpart=ops%40example.com&max_id=raw-cursor",
			want:        "/api/v1/conversations/lookup",
			mustNotLeak: []string{"ops", "raw-cursor"},
		},
		"conversation id redacted": {
			in:          "/api/v1/conversations/conv-private-agent",
			rawSecret:   "conv-private-agent",
			wantPrefix:  "/api/v1/conversations/conversation-sha256:",
			mustNotLeak: []string{"conv-private-agent"},
		},
		"conversation id redacted without leading slash": {
			in:          "api/v1/conversations/conv-private-relative?max_id=raw-cursor",
			rawSecret:   "conv-private-relative",
			wantPrefix:  "api/v1/conversations/conversation-sha256:",
			mustNotLeak: []string{"conv-private-relative", "raw-cursor"},
		},
		"read route id redacted": {
			in:          "/api/v1/conversations/conv-private-read/read",
			rawSecret:   "conv-private-read",
			wantPrefix:  "/api/v1/conversations/conversation-sha256:",
			wantSuffix:  "/read",
			mustNotLeak: []string{"conv-private-read"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := SanitizeLogPath(tc.in)
			if tc.want != "" {
				require.Equal(t, tc.want, got)
			}
			if tc.wantPrefix != "" {
				require.Truef(t, strings.HasPrefix(got, tc.wantPrefix), "expected sanitized prefix %q, got %q", tc.wantPrefix, got)
			}
			if tc.wantSuffix != "" {
				require.Truef(t, strings.HasSuffix(got, tc.wantSuffix), "expected sanitized suffix %q, got %q", tc.wantSuffix, got)
			}
			if tc.rawSecret != "" {
				require.NotContains(t, got, tc.rawSecret)
			}
			for _, forbidden := range tc.mustNotLeak {
				require.NotContains(t, got, forbidden)
			}
		})
	}
}

func TestSanitizeLogPathLeavesMalformedConversationRoutesUnchanged(t *testing.T) {
	t.Parallel()

	for _, path := range []string{
		"/api/v1/conversations/conv-private/read/extra",
		"/api/v1/conversations/conv-private/write",
	} {
		require.Equal(t, path, SanitizeLogPath(path))
	}
}

func TestSanitizeLogPathLeavesOtherRoutesUnchanged(t *testing.T) {
	t.Parallel()

	path := "/api/v1/statuses/conv-private-1?cursor=raw-cursor"
	require.Equal(t, path, SanitizeLogPath(path))
}
