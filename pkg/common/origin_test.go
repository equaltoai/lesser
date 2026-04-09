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
