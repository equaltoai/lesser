package handlers

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/storage"
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

func TestNormalizePublicOAuthClientClass(t *testing.T) {
	out, err := normalizePublicOAuthClientClass("")
	require.NoError(t, err)
	require.Equal(t, "", out)

	out, err = normalizePublicOAuthClientClass(" cli ")
	require.NoError(t, err)
	require.Equal(t, auth.ClientClassCLI, out)

	out, err = normalizePublicOAuthClientClass("web")
	require.NoError(t, err)
	require.Equal(t, auth.ClientClassWeb, out)

	_, err = normalizePublicOAuthClientClass(auth.ClientClassAgent)
	require.ErrorContains(t, err, "client_class=agent is not supported for public registration")
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

	out, err = normalizeOAuthClientGrantTypes("", auth.ClientClassCLI, false)
	require.NoError(t, err)
	require.Equal(t, []string{auth.GrantTypeAuthorizationCode, auth.GrantTypeRefreshToken}, out)

	out, err = normalizeOAuthClientGrantTypes("AUTHORIZATION_CODE refresh_token authorization_code", auth.ClientClassWeb, false)
	require.NoError(t, err)
	require.Equal(t, []string{auth.GrantTypeAuthorizationCode, auth.GrantTypeRefreshToken}, out)

	_, err = normalizeOAuthClientGrantTypes("password", auth.ClientClassWeb, false)
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

	method, confidential, err = normalizeOAuthTokenEndpointAuthMethod("client_secret_basic", auth.ClientClassCLI)
	require.NoError(t, err)
	require.Equal(t, "client_secret_basic", method)
	require.True(t, confidential)
}

func TestOAuthClientTokenEndpointAuthMethod(t *testing.T) {
	require.Equal(t, oauthTokenEndpointAuthMethodNone, oauthClientTokenEndpointAuthMethod(nil))
	require.Equal(t, oauthTokenEndpointAuthMethodNone, oauthClientTokenEndpointAuthMethod(&storage.OAuthClient{}))
	require.Equal(t, oauthTokenEndpointAuthMethodClientSecretPost, oauthClientTokenEndpointAuthMethod(&storage.OAuthClient{Confidential: true}))
}
