package httpclient

import (
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewSecureHTTPClient_ReturnsConfiguredClient(t *testing.T) {
	t.Parallel()

	client := NewSecureHTTPClient()
	require.NotNil(t, client)

	_, ok := client.Transport.(*secureTransport)
	require.True(t, ok)
}

func TestValidateURL_EdgeCases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name            string
		u               *url.URL
		wantErrIs       error
		wantErrContains string
		wantErrExact    string
	}{
		{
			name:         "nil_url",
			u:            nil,
			wantErrExact: "nil URL",
		},
		{
			name:            "invalid_scheme_blocked",
			u:               &url.URL{Scheme: "ftp", Host: "example.com"},
			wantErrIs:       ErrInvalidScheme,
			wantErrContains: "ftp",
		},
		{
			name:            "invalid_scheme_not_blocked",
			u:               &url.URL{Scheme: "ws", Host: "example.com"},
			wantErrIs:       ErrInvalidScheme,
			wantErrContains: "only http/https allowed, got ws",
		},
		{
			name:         "empty_hostname",
			u:            &url.URL{Scheme: "https"},
			wantErrExact: "empty hostname",
		},
		{
			name:         "blocked_hostname",
			u:            &url.URL{Scheme: "https", Host: "localhost"},
			wantErrExact: "hostname is blocked",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateURL(tc.u, zap.NewNop())
			require.Error(t, err)

			if tc.wantErrIs != nil {
				require.True(t, errors.Is(err, tc.wantErrIs))
			}
			if tc.wantErrContains != "" {
				require.ErrorContains(t, err, tc.wantErrContains)
			}
			if tc.wantErrExact != "" {
				require.Equal(t, tc.wantErrExact, err.Error())
			}
		})
	}
}

func TestSecureClient_checkRedirect_AllowsSafeRedirect(t *testing.T) {
	t.Parallel()

	client := NewSecureClient()
	req, err := http.NewRequest(http.MethodGet, "https://example.com/", nil)
	require.NoError(t, err)

	require.NoError(t, client.checkRedirect(req, nil))
}
