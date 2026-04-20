package httpclient

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSecureClient_RequestHelpers_ReturnErrorOnInvalidURL(t *testing.T) {
	t.Parallel()

	client := NewSecureClient()
	const invalidURL = "http://[::1"

	resp, err := client.Post(invalidURL, "text/plain", strings.NewReader("body"))
	require.Error(t, err)
	require.Nil(t, resp)

	resp, err = client.Head(invalidURL)
	require.Error(t, err)
	require.Nil(t, resp)

	resp, err = client.GetWithContext(context.Background(), invalidURL)
	require.Error(t, err)
	require.Nil(t, resp)

	resp, err = client.PostWithContext(context.Background(), invalidURL, "text/plain", strings.NewReader("ctx"))
	require.Error(t, err)
	require.Nil(t, resp)
}

func TestSecureTransport_dialIP_UsesNetDialerWhenUnset(t *testing.T) {
	t.Parallel()

	transport := &secureTransport{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := transport.dialIP(ctx, "tcp", "127.0.0.1:80")
	require.ErrorIs(t, err, context.Canceled)
}
