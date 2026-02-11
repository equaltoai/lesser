package handlers

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/stretchr/testify/require"
)

func TestNormalizeOAuthClientClass(t *testing.T) {
	out, err := normalizeOAuthClientClass("")
	require.NoError(t, err)
	require.Equal(t, "", out)

	out, err = normalizeOAuthClientClass(" CLI ")
	require.NoError(t, err)
	require.Equal(t, auth.ClientClassCLI, out)

	out, err = normalizeOAuthClientClass("WEB")
	require.NoError(t, err)
	require.Equal(t, auth.ClientClassWeb, out)

	out, err = normalizeOAuthClientClass("agent")
	require.NoError(t, err)
	require.Equal(t, auth.ClientClassAgent, out)

	_, err = normalizeOAuthClientClass("nope")
	require.Error(t, err)
}
