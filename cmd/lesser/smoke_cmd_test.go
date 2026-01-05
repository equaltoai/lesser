package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunSmokeCommands(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousRunCommand := runCommandFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		runCommandFn = previousRunCommand
	})

	findRepoRootFn = func() (string, error) { return t.TempDir(), nil }
	runCommandFn = func(_ context.Context, _ string, _ []string, _ execOptions) error { return nil }

	require.Error(t, runSmoke(nil))
	require.NoError(t, runSmoke([]string{helpCommand}))
	require.Error(t, runSmoke([]string{"nope"}))

	require.Error(t, runSmokeCore(smokeArgs{}))
	require.Error(t, runSmokeFederation(smokeArgs{BaseURL: "https://example.com"}))

	require.NoError(t, runSmokeCore(smokeArgs{BaseURL: "https://example.com"}))
	require.NoError(t, runSmokeFederation(smokeArgs{
		BaseURL:      "https://example.com",
		Username:     "alice",
		ObjectID:     "https://example.com/objects/1",
		AcceptHeader: "application/activity+json",
	}))

	require.NoError(t, runSmokeCoreFromArgs([]string{"--base-url", "https://example.com"}))
	require.NoError(t, runSmokeFederationFromArgs([]string{
		"--base-url", "https://example.com",
		"--username", "alice",
		"--object-id", "https://example.com/objects/1",
	}))
}

func TestRunSmoke_DispatchAndEnvBranches(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousRunCommand := runCommandFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		runCommandFn = previousRunCommand
	})

	repoRoot := t.TempDir()
	findRepoRootFn = func() (string, error) { return repoRoot, nil }

	var gotName string
	var gotArgs []string
	var gotEnv map[string]string
	runCommandFn = func(_ context.Context, name string, args []string, opts execOptions) error {
		gotName = name
		gotArgs = append([]string(nil), args...)
		gotEnv = opts.Env
		require.Equal(t, repoRoot, opts.Dir)
		return nil
	}

	require.NoError(t, runSmoke([]string{"core", "--base-url", "https://example.com", "--token", "tok", "--insecure"}))
	require.Equal(t, "bash", gotName)
	require.Equal(t, []string{"scripts/smoke_core.sh"}, gotArgs)
	require.Equal(t, "tok", gotEnv["SMOKE_TOKEN"])
	require.Equal(t, "1", gotEnv["SMOKE_INSECURE"])

	require.Error(t, runSmokeCoreFromArgs([]string{"--badflag"}))
	require.Error(t, runSmokeFederationFromArgs([]string{"--badflag"}))

	require.Error(t, runSmokeFederation(smokeArgs{BaseURL: "https://example.com", Username: "alice"}))

	require.NoError(t, runSmokeFederation(smokeArgs{
		BaseURL:  "https://example.com",
		Username: "alice",
		ObjectID: "https://example.com/objects/1",
	}))
	require.Equal(t, []string{"scripts/smoke_federation.sh"}, gotArgs)
	require.Equal(t, "application/activity+json", gotEnv["SMOKE_ACCEPT_HEADER"])
}
