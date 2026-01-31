package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestShouldRouteToAuthAppTheoryRound12(t *testing.T) {
	require.True(t, shouldRouteToAuthAppTheory("GET", "/oauth/authorize"))
	require.True(t, shouldRouteToAuthAppTheory("POST", "/oauth/token"))
	require.True(t, shouldRouteToAuthAppTheory("DELETE", "/auth/wallet/unlink/0xabc"))
	require.True(t, shouldRouteToAuthAppTheory("PUT", "/api/v1/auth/webauthn/credentials/cred123"))
	require.True(t, shouldRouteToAuthAppTheory("GET", "/setup/status"))

	require.False(t, shouldRouteToAuthAppTheory("OPTIONS", "/oauth/authorize"))
	require.False(t, shouldRouteToAuthAppTheory("POST", "/setup/status"))
	require.False(t, shouldRouteToAuthAppTheory("DELETE", "/auth/wallet/unlink"))
	require.False(t, shouldRouteToAuthAppTheory("GET", "/health"))
}

func TestExtractHTTPMethodAndPath_StripsStagePrefixRound12(t *testing.T) {
	method, path, ok := extractHTTPMethodAndPath(map[string]any{
		"version":  "2.0",
		"routeKey": "GET /oauth/authorize",
		"rawPath":  "/dev/oauth/authorize",
		"requestContext": map[string]any{
			"stage": "dev",
			"http": map[string]any{
				"method": "GET",
				"path":   "/dev/oauth/authorize",
			},
		},
	})

	require.True(t, ok)
	require.Equal(t, "GET", method)
	require.Equal(t, "/oauth/authorize", path)
}
