package handlers

import (
	"errors"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
)

// TestNewOAuthGrantTelemetryTrimsTheRequestedIdentity pins the constructor used
// when a token request is recorded before the grant runs: the requested
// identity is trimmed, and a missing request still yields a usable record whose
// retry path is the explicit "none" rather than an empty string.
func TestNewOAuthGrantTelemetryTrimsTheRequestedIdentity(t *testing.T) {
	empty := newOAuthGrantTelemetry(nil)
	require.NotNil(t, empty)
	assert.Empty(t, empty.GrantType)
	assert.Empty(t, empty.ClientID)
	assert.Empty(t, empty.Resource)
	assert.Equal(t, oauthGrantRetryPathNone, empty.RetryPath)

	telemetry := newOAuthGrantTelemetry(&oauthTokenRequest{
		grantType: " authorization_code ",
		clientID:  " client-1 ",
		resource:  " https://api.example.com ",
	})
	require.NotNil(t, telemetry)
	assert.Equal(t, oauthGrantTypeAuthorizationCode, telemetry.GrantType)
	assert.Equal(t, "client-1", telemetry.ClientID)
	assert.Equal(t, "https://api.example.com", telemetry.Resource)
	assert.Equal(t, oauthGrantRetryPathNone, telemetry.RetryPath)
}

// TestSetOAuthGrantReasonFromErrorMapsEveryRefusal pins the metric label a failed
// grant records. The mapping is what an operator reads when a client reports a
// token failure, so each refusal class must land on its own stable reason and
// anything unrecognised must fall back to a server error rather than a silent
// success-shaped label.
func TestSetOAuthGrantReasonFromErrorMapsEveryRefusal(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{name: "temporarily unavailable", err: auth.ErrOAuthTemporarilyUnavailable, want: oauthGrantReasonTemporarilyUnavailable},
		{name: "invalid client", err: auth.ErrInvalidClient, want: oauthGrantReasonInvalidClient},
		{name: "unauthorized client", err: auth.ErrUnauthorizedClient, want: oauthGrantReasonUnauthorizedClient},
		{name: "invalid request", err: auth.ErrInvalidRequest, want: oauthGrantReasonInvalidRequest},
		{name: "pkce mismatch", err: auth.ErrInvalidCodeChallenge, want: oauthGrantReasonAuthorizationCodePKCEMismatch},
		{name: "invalid scope", err: auth.ErrInvalidScope, want: oauthGrantReasonAuthorizationCodeScopeInvalid},
		{name: "resource mismatch", err: errOAuthInvalidTarget, want: oauthGrantReasonAuthorizationCodeResourceMismatch},
		{name: "wrapped refusal", err: errors.Join(errors.New("token exchange"), auth.ErrInvalidClient), want: oauthGrantReasonInvalidClient},
		{name: "unrecognised failure", err: errors.New("boom"), want: oauthGrantReasonServerError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			telemetry := &oauthGrantTelemetry{}
			setOAuthGrantReasonFromError(telemetry, tc.err)
			assert.Equal(t, tc.want, telemetry.DetailReason)
		})
	}

	t.Run("an already-recorded reason wins", func(t *testing.T) {
		telemetry := &oauthGrantTelemetry{DetailReason: oauthGrantReasonInvalidClient}
		setOAuthGrantReasonFromError(telemetry, auth.ErrInvalidRequest)
		assert.Equal(t, oauthGrantReasonInvalidClient, telemetry.DetailReason)
	})

	t.Run("absent telemetry and absent error are no-ops", func(t *testing.T) {
		setOAuthGrantReasonFromError(nil, auth.ErrInvalidRequest)

		telemetry := &oauthGrantTelemetry{}
		setOAuthGrantReasonFromError(telemetry, nil)
		assert.Empty(t, telemetry.DetailReason)
	})
}

// TestOAuthGrantResponseStatusReportsTheStatusTheClientSaw pins the status the
// grant metric records: the response the handler already built when there is one,
// and a server error otherwise, so a grant that never produced a response is
// never counted as a success.
func TestOAuthGrantResponseStatusReportsTheStatusTheClientSaw(t *testing.T) {
	assert.Equal(t, http.StatusTooManyRequests,
		oauthGrantResponseStatus(&apptheory.Response{Status: http.StatusTooManyRequests}, errors.New("rate limited")))
	assert.Equal(t, http.StatusBadRequest,
		oauthGrantResponseStatus(&apptheory.Response{Status: http.StatusBadRequest}, nil))

	assert.Equal(t, http.StatusInternalServerError, oauthGrantResponseStatus(nil, errors.New("boom")))
	assert.Equal(t, http.StatusInternalServerError, oauthGrantResponseStatus(&apptheory.Response{}, nil))
}
