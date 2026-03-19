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

func TestNormalizeOAuthClientGrantTypes(t *testing.T) {
	out, err := normalizeOAuthClientGrantTypes("", auth.ClientClassAgent, true)
	require.NoError(t, err)
	require.Equal(t, []string{auth.GrantTypeAuthorizationCode, auth.GrantTypeRefreshToken, auth.GrantTypeClientCredentials}, out)

	out, err = normalizeOAuthClientGrantTypes("", auth.ClientClassCLI, true)
	require.NoError(t, err)
	require.Equal(t, []string{auth.GrantTypeAuthorizationCode, auth.GrantTypeRefreshToken, oauthDeviceCodeGrantType}, out)

	out, err = normalizeOAuthClientGrantTypes("authorization_code refresh_token", auth.ClientClassWeb, false)
	require.NoError(t, err)
	require.Equal(t, []string{auth.GrantTypeAuthorizationCode, auth.GrantTypeRefreshToken}, out)

	_, err = normalizeOAuthClientGrantTypes("client_credentials", auth.ClientClassWeb, true)
	require.Error(t, err)

	_, err = normalizeOAuthClientGrantTypes(oauthDeviceCodeGrantType, auth.ClientClassCLI, false)
	require.Error(t, err)
}

func TestNormalizeOAuthTokenEndpointAuthMethod(t *testing.T) {
	method, confidential, err := normalizeOAuthTokenEndpointAuthMethod("", auth.ClientClassAgent)
	require.NoError(t, err)
	require.Equal(t, "client_secret_post", method)
	require.True(t, confidential)

	method, confidential, err = normalizeOAuthTokenEndpointAuthMethod("", auth.ClientClassCLI)
	require.NoError(t, err)
	require.Equal(t, "none", method)
	require.False(t, confidential)

	method, confidential, err = normalizeOAuthTokenEndpointAuthMethod("client_secret_post", auth.ClientClassCLI)
	require.NoError(t, err)
	require.Equal(t, "client_secret_post", method)
	require.True(t, confidential)

	_, _, err = normalizeOAuthTokenEndpointAuthMethod("client_secret_basic", auth.ClientClassCLI)
	require.Error(t, err)
}
