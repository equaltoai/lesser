package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAgentRegistrationRoutesUseOAuthRegistrationRateLimit(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)

	routesPath := filepath.Join(filepath.Dir(file), "routes.go")
	body, err := os.ReadFile(routesPath)
	require.NoError(t, err)
	src := string(body)

	require.Contains(t, src, "app.Post(\"/api/v1/agents/register/challenge\", ratelimit.ApplyOAuthRegistrationRateLimit(")
	require.Contains(t, src, "app.Post(\"/api/v1/agents/register\", ratelimit.ApplyOAuthRegistrationRateLimit(")
}
