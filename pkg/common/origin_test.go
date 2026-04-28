package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOriginURLFromHeaders_PrefersLesserForwardedHost(t *testing.T) {
	headers := map[string][]string{
		"host":                     {"internal.execute-api.us-east-1.amazonaws.com"},
		"x-forwarded-host":         {"stale.example.com"},
		"x-forwarded-proto":        {"https"},
		"x-lesser-forwarded-host":  {"theory.dev.example.com"},
		"x-lesser-forwarded-proto": {"https"},
	}

	require.Equal(t, "https://theory.dev.example.com", OriginURLFromHeaders(headers))
}

func TestRequestURLFromHeaders_FallsBackToForwardedAndHost(t *testing.T) {
	t.Run("forwarded header fallback", func(t *testing.T) {
		headers := map[string][]string{
			"host":      {"internal.execute-api.us-east-1.amazonaws.com"},
			"forwarded": {"for=198.51.100.22;proto=https;host=theory.dev.example.com"},
		}

		u := RequestURLFromHeaders(headers, "/inbox", map[string][]string{"cursor": {"next"}})
		require.Equal(t, "https://theory.dev.example.com/inbox?cursor=next", u.String())
	})

	t.Run("host fallback", func(t *testing.T) {
		headers := map[string][]string{
			"host": {"theory.dev.example.com"},
		}

		u := RequestURLFromHeaders(headers, "/inbox", map[string][]string{"tag": {"one", "two"}})
		require.Equal(t, "https://theory.dev.example.com/inbox?tag=one&tag=two", u.String())
	})
}

func TestOriginURLFromHeaders_InvalidProtoDefaultsToHTTPS(t *testing.T) {
	headers := map[string][]string{
		"x-lesser-forwarded-host":  {"theory.dev.example.com"},
		"x-lesser-forwarded-proto": {"gopher"},
	}

	require.Equal(t, "https://theory.dev.example.com", OriginURLFromHeaders(headers))
}

func TestOriginURLFromHeaders_FallsBackToXForwardedProto(t *testing.T) {
	headers := map[string][]string{
		"x-lesser-forwarded-host": {"theory.dev.example.com"},
		"x-forwarded-proto":       {"http"},
	}

	require.Equal(t, "http://theory.dev.example.com", OriginURLFromHeaders(headers))
}

func TestRequestURLFromHeaders_EmptyHeadersStillBuildsHTTPSURL(t *testing.T) {
	u := RequestURLFromHeaders(nil, "/inbox", nil)

	require.Equal(t, SchemeHTTPS, u.Scheme)
	require.Empty(t, u.Host)
	require.Equal(t, "/inbox", u.Path)
}

func TestHeaderMapValue_UncoveredBranches(t *testing.T) {
	require.Empty(t, headerMapValue(map[string][]string{
		HostHeader: {"example.com"},
	}, ""))

	require.Empty(t, headerMapValue(map[string][]string{
		HostHeader: {},
	}, HostHeader))
}

func TestOriginURLFromHeaders_RejectsMalformedForwardedHosts(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string][]string
	}{
		{
			name: "lesser forwarded host with path is ignored",
			headers: map[string][]string{
				"x-lesser-forwarded-host": {"theory.dev.example.com/evil"},
			},
		},
		{
			name: "forwarded host with userinfo is ignored",
			headers: map[string][]string{
				"forwarded": {"for=198.51.100.22;proto=https;host=evil.example@theory.dev.example.com"},
			},
		},
		{
			name: "host fallback with path is ignored",
			headers: map[string][]string{
				"host": {"theory.dev.example.com/evil"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := RequestURLFromHeaders(tt.headers, "/inbox", nil)
			require.Empty(t, u.Host)
			require.Equal(t, "/inbox", u.Path)
		})
	}
}

func TestOriginURLFromHeaders_UsesNormalizedForwardedAuthorities(t *testing.T) {
	t.Run("x forwarded host uses first authority and proto", func(t *testing.T) {
		headers := map[string][]string{
			"x-forwarded-host":  {"Theory.Dev.Example.Com:8443, stale.example.com"},
			"x-forwarded-proto": {"http, https"},
		}

		require.Equal(t, "http://theory.dev.example.com:8443", OriginURLFromHeaders(headers))
	})

	t.Run("standard forwarded invalid proto defaults to https", func(t *testing.T) {
		headers := map[string][]string{
			"forwarded": {"for=198.51.100.22;proto=gopher;host=Theory.Dev.Example.Com:8443"},
		}

		require.Equal(t, "https://theory.dev.example.com:8443", OriginURLFromHeaders(headers))
	})
}

func TestNormalizeForwardedHost(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{
			name: "hostname lowercased",
			raw:  "Theory.Dev.Example.Com",
			want: "theory.dev.example.com",
			ok:   true,
		},
		{
			name: "hostname with port",
			raw:  "Theory.Dev.Example.Com:8443",
			want: "theory.dev.example.com:8443",
			ok:   true,
		},
		{
			name: "ipv6 literal without port",
			raw:  "[2001:db8::1]",
			want: "[2001:db8::1]",
			ok:   true,
		},
		{
			name: "ipv6 literal with port",
			raw:  "[2001:db8::1]:8443",
			want: "[2001:db8::1]:8443",
			ok:   true,
		},
		{
			name: "invalid port text",
			raw:  "example.com:notaport",
		},
		{
			name: "invalid port range",
			raw:  "example.com:70000",
		},
		{
			name: "userinfo is rejected",
			raw:  "evil@example.com",
		},
		{
			name: "path is rejected",
			raw:  "example.com/path",
		},
		{
			name: "empty is rejected",
			raw:  " ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := normalizeForwardedHost(tt.raw)
			require.Equal(t, tt.ok, ok)
			require.Equal(t, tt.want, got)
		})
	}
}
