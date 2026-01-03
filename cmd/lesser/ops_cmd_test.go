package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpsCommands_ValidateInputsAndInvokeAWS(t *testing.T) {
	previousRunCommand := runCommandFn
	previousEnsureTool := ensureToolAvailableFn
	t.Cleanup(func() {
		runCommandFn = previousRunCommand
		ensureToolAvailableFn = previousEnsureTool
	})

	ensureToolAvailableFn = func(string) error { return nil }

	var called int
	runCommandFn = func(_ context.Context, name string, _ []string, _ execOptions) error {
		require.Equal(t, "aws", name)
		called++
		return nil
	}

	require.Error(t, runLogs([]string{}))
	require.Error(t, runMetrics([]string{}))

	require.NoError(t, runLogs([]string{"--function", "api"}))
	require.NoError(t, runMetrics([]string{"--function", "api"}))
	require.NoError(t, runErrors(nil))

	require.GreaterOrEqual(t, called, 3)
}

func TestRunLogs_InvalidEnv(t *testing.T) {
	previousEnsureTool := ensureToolAvailableFn
	t.Cleanup(func() { ensureToolAvailableFn = previousEnsureTool })
	ensureToolAvailableFn = func(string) error { return nil }

	require.Error(t, runLogs([]string{"--function", "api", "--env", "nope"}))
}

func TestRunDashboard_PrintsURL(t *testing.T) {
	require.NoError(t, runDashboard([]string{"--app", "app", "--env", "dev", "--region", "us-east-1"}))
}

func TestOpsCommands_ParseErrorsAndEnvOverrides(t *testing.T) {
	previousRunCommand := runCommandFn
	previousEnsureTool := ensureToolAvailableFn
	t.Cleanup(func() {
		runCommandFn = previousRunCommand
		ensureToolAvailableFn = previousEnsureTool
	})

	require.Error(t, runLogs([]string{"--badflag"}))
	require.Error(t, runMetrics([]string{"--badflag"}))
	require.Error(t, runErrors([]string{"--badflag"}))
	require.Error(t, runDashboard([]string{"--badflag"}))

	ensureToolAvailableFn = func(string) error { return errSentinel }
	require.ErrorIs(t, runLogs([]string{"--function", "api"}), errSentinel)

	ensureToolAvailableFn = func(string) error { return nil }

	var lastEnv map[string]string
	runCommandFn = func(_ context.Context, name string, _ []string, opts execOptions) error {
		require.Equal(t, "aws", name)
		lastEnv = opts.Env
		return nil
	}

	require.NoError(t, runLogs([]string{"--function", "api", "--aws-profile", "profile"}))
	require.Equal(t, "profile", lastEnv["AWS_PROFILE"])

	require.NoError(t, runMetrics([]string{"--function", "api", "--aws-profile", "profile", "--region", "us-west-2"}))
	require.Equal(t, "profile", lastEnv["AWS_PROFILE"])
	require.Equal(t, "us-west-2", lastEnv["AWS_REGION"])
	require.Equal(t, "us-west-2", lastEnv["AWS_DEFAULT_REGION"])

	require.NoError(t, runErrors([]string{"--aws-profile", "profile", "--max-items", "5"}))
	require.Equal(t, "profile", lastEnv["AWS_PROFILE"])
}
