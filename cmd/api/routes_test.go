package main

import (
	"context"
	"testing"

	apiHandlers "github.com/equaltoai/lesser/cmd/api/handlers"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
	"go.uber.org/zap/zaptest"
)

func TestConfigureRoutes(t *testing.T) {
	logger = zaptest.NewLogger(t)
	apiHandler = &apiHandlers.Handler{}

	app := apptheory.NewSecure(apptheory.SecureOptions{Tier: apptheory.TierP2})
	configureRoutes(app)
}

func TestConfigureRoutes_CORSPreflightHandledByRuntime(t *testing.T) {
	logger = zaptest.NewLogger(t)
	apiHandler = &apiHandlers.Handler{}

	app := apptheory.NewSecure(apptheory.SecureOptions{
		Tier: apptheory.TierP2,
		CORS: apptheory.CORSConfig{
			AllowedOrigins: []string{"*"},
			AllowHeaders:   []string{"Authorization", "Content-Type"},
		},
	})
	configureRoutes(app)

	resp := app.Serve(context.Background(), apptheory.Request{
		Method: "OPTIONS",
		Path:   "/auth/wallet/challenge",
		Headers: map[string][]string{
			"Origin":                        {"https://ui.example"},
			"Access-Control-Request-Method": {"POST"},
		},
	})

	require.Equal(t, 204, resp.Status)
	require.Equal(t, []string{"POST"}, resp.Headers["access-control-allow-methods"])
	require.Equal(t, []string{"https://ui.example"}, resp.Headers["access-control-allow-origin"])
	require.NotEmpty(t, resp.Headers["access-control-allow-headers"])
}
