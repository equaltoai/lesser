package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnsureToolAvailable(t *testing.T) {
	require.NoError(t, ensureToolAvailable("go"))

	err := ensureToolAvailable("definitely-not-a-real-tool-12345")
	require.Error(t, err)
	require.Contains(t, err.Error(), "required tool")
}

