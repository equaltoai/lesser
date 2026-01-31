package main

import (
	"context"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
)

func TestShouldRouteToAPIAppTheoryRound12(t *testing.T) {
	require.True(t, shouldRouteToAPIAppTheory("GET", "/oauth/authorize"))
	require.True(t, shouldRouteToAPIAppTheory("POST", "/oauth/token"))
	require.True(t, shouldRouteToAPIAppTheory("DELETE", "/auth/wallet/unlink/0xabc"))
	require.True(t, shouldRouteToAPIAppTheory("PUT", "/api/v1/auth/webauthn/credentials/cred123"))
	require.True(t, shouldRouteToAPIAppTheory("GET", "/setup/status"))

	require.True(t, shouldRouteToAPIAppTheory("GET", "/api/v1/accounts/verify_credentials"))
	require.True(t, shouldRouteToAPIAppTheory("PATCH", "/api/v1/accounts/update_credentials"))
	require.True(t, shouldRouteToAPIAppTheory("GET", "/api/v1/accounts/123/statuses"))
	require.True(t, shouldRouteToAPIAppTheory("POST", "/api/v1/exports"))
	require.True(t, shouldRouteToAPIAppTheory("GET", "/api/v1/imports/imp123"))
	require.True(t, shouldRouteToAPIAppTheory("GET", "/api/v1/statuses/status123"))
	require.True(t, shouldRouteToAPIAppTheory("POST", "/api/v1/statuses/status123/favourite"))
	require.True(t, shouldRouteToAPIAppTheory("GET", "/api/v2/filters/filter123"))

	require.False(t, shouldRouteToAPIAppTheory("OPTIONS", "/oauth/authorize"))
	require.False(t, shouldRouteToAPIAppTheory("POST", "/setup/status"))
	require.False(t, shouldRouteToAPIAppTheory("DELETE", "/auth/wallet/unlink"))
	require.False(t, shouldRouteToAPIAppTheory("GET", "/health"))
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

func TestHandleAPIRequest_StripsStagePrefixForAppTheoryRound12(t *testing.T) {
	app := apptheory.New(apptheory.WithTier(apptheory.TierP0))
	app.Get("/oauth/authorize", func(_ *apptheory.Context) (*apptheory.Response, error) {
		return apptheory.Text(200, "ok"), nil
	})

	out, err := handleAPIRequest(context.Background(), lift.New(), nil, app, map[string]any{
		"version":  "2.0",
		"rawPath":  "/dev/oauth/authorize",
		"routeKey": "GET /dev/oauth/authorize",
		"requestContext": map[string]any{
			"stage": "dev",
			"http": map[string]any{
				"method": "GET",
				"path":   "/dev/oauth/authorize",
			},
		},
	})
	require.NoError(t, err)

	resp, ok := out.(events.APIGatewayV2HTTPResponse)
	require.True(t, ok)
	require.Equal(t, 200, resp.StatusCode)
}
