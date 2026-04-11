package handlers

import (
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestOAuthAuthorizeTargetErrorRound14(t *testing.T) {
	var nilErr *oauthAuthorizeTargetError
	require.Equal(t, "", nilErr.Error())

	err := &oauthAuthorizeTargetError{
		code:        "invalid_target",
		description: "bad target",
	}
	require.Equal(t, "bad target", err.Error())
}

func TestOAuthDeviceApprovedTokenContextRound14(t *testing.T) {
	h := &Handler{}

	t.Run("defaults empty client class to cli and mints session id", func(t *testing.T) {
		username, clientClass, sessionID, accessTTL, err := h.oauthDeviceApprovedTokenContext(
			t.Context(),
			&storage.OAuthClient{},
			" alice ",
		)
		require.NoError(t, err)
		require.Equal(t, "alice", username)
		require.Equal(t, auth.ClientClassCLI, clientClass)
		require.NotEmpty(t, sessionID)
		require.Equal(t, auth.AccessTokenDuration, accessTTL)
	})

	t.Run("agent clients are rejected", func(t *testing.T) {
		_, _, _, _, err := h.oauthDeviceApprovedTokenContext(
			t.Context(),
			&storage.OAuthClient{ClientClass: auth.ClientClassAgent},
			"alice",
		)
		require.ErrorIs(t, err, auth.ErrInvalidGrant)
	})
}

func TestOAuthDeviceApprovedTokenContextErrorResponseRound14(t *testing.T) {
	h := &Handler{}

	resp, err := h.oauthDeviceApprovedTokenContextErrorResponse(auth.ErrInvalidGrant)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 400, resp.Status)
	require.Contains(t, string(resp.Body), `"error":"invalid_grant"`)

	resp, err = h.oauthDeviceApprovedTokenContextErrorResponse(errors.New("boom"))
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 500, resp.Status)
	require.Contains(t, string(resp.Body), `"error":"server_error"`)
}

func TestOAuthDeviceSessionTokenResponseRound14(t *testing.T) {
	h := &Handler{}

	t.Run("pending returns authorization pending", func(t *testing.T) {
		resp, err := h.oauthDeviceSessionTokenResponse(t.Context(), nil, &storage.OAuthDeviceSession{
			Status: oauthDeviceSessionStatusPending,
		}, "client", time.Now())
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 400, resp.Status)
		require.Contains(t, string(resp.Body), `"error":"authorization_pending"`)
	})

	t.Run("denied returns access denied", func(t *testing.T) {
		resp, err := h.oauthDeviceSessionTokenResponse(t.Context(), nil, &storage.OAuthDeviceSession{
			Status: oauthDeviceSessionStatusDenied,
		}, "client", time.Now())
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 400, resp.Status)
		require.Contains(t, string(resp.Body), `"error":"access_denied"`)
	})

	t.Run("consumed returns invalid grant", func(t *testing.T) {
		resp, err := h.oauthDeviceSessionTokenResponse(t.Context(), nil, &storage.OAuthDeviceSession{
			Status: oauthDeviceSessionStatusConsumed,
		}, "client", time.Now())
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 400, resp.Status)
		require.Contains(t, string(resp.Body), `"error":"invalid_grant"`)
	})

	t.Run("unknown status returns server error", func(t *testing.T) {
		resp, err := h.oauthDeviceSessionTokenResponse(t.Context(), nil, &storage.OAuthDeviceSession{
			Status: "mystery",
		}, "client", time.Now())
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, 500, resp.Status)
		require.Contains(t, string(resp.Body), `"error":"server_error"`)
	})
}
