package ratelimit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNormalizeEndpointKey(t *testing.T) {
	require.Equal(t, "GET:/path", normalizeEndpointKey("GET:/path/"))
	require.Equal(t, "GET:/path", normalizeEndpointKey("GET:/path/?a=b"))
}

func TestMatchEndpointPattern(t *testing.T) {
	require.True(t, matchEndpointPattern("POST:/notes/abc/vote", "POST:/notes/*/vote"))
	require.False(t, matchEndpointPattern("POST:/notes/abc/vote", "POST:/notes/*/other"))
	require.True(t, matchEndpointPattern("POST:/oauth/token", "POST:/oauth/token"))
}

func TestLookupEndpointLimit(t *testing.T) {
	cfg, ok := lookupEndpointLimit("POST:/oauth/token")
	require.True(t, ok)
	require.Equal(t, 10, cfg.limit)
	require.Equal(t, time.Minute, cfg.window)

	cfg, ok = lookupEndpointLimit("POST:/notes/abc/vote")
	require.True(t, ok)
	require.Equal(t, 100, cfg.limit)
	require.Equal(t, time.Hour, cfg.window)

	_, ok = lookupEndpointLimit("GET:/not/limited")
	require.False(t, ok)
}

func TestBuildCacheKey(t *testing.T) {
	require.Equal(t, "10:1m0s", buildCacheKey(endpointRateLimit{limit: 10, window: time.Minute}))
}
