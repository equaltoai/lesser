package main

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnsureToolsAvailable_UsesInjectedChecker(t *testing.T) {
	previous := ensureToolAvailableFn
	t.Cleanup(func() { ensureToolAvailableFn = previous })

	var called []string
	ensureToolAvailableFn = func(name string) error {
		called = append(called, name)
		return nil
	}

	require.NoError(t, ensureToolsAvailable())
	require.Equal(t, []string{"aws", "cdk", "go", "pnpm"}, called)

	called = nil
	require.NoError(t, ensureAWSCLIToolAvailable())
	require.Equal(t, []string{"aws"}, called)
}

func TestEnsureToolsAvailable_PropagatesError(t *testing.T) {
	previous := ensureToolAvailableFn
	t.Cleanup(func() { ensureToolAvailableFn = previous })

	ensureToolAvailableFn = func(name string) error {
		if name == "cdk" {
			return errors.New("missing")
		}
		return nil
	}

	require.ErrorContains(t, ensureToolsAvailable(), "missing")
}
