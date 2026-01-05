package ai

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSSRFProtectedHTTPClient_DialContext_InvalidDialAddress(t *testing.T) {
	client := newSSRFProtectedHTTPClient(nil)
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)

	conn, err := transport.DialContext(context.Background(), "tcp", "missing-port")
	if conn != nil {
		_ = conn.Close()
	}

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid dial address")
}

func TestNewSSRFProtectedHTTPClient_DialContext_BlockedIPLiteral(t *testing.T) {
	client := newSSRFProtectedHTTPClient(nil)
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)

	conn, err := transport.DialContext(context.Background(), "tcp", "127.0.0.1:80")
	if conn != nil {
		_ = conn.Close()
	}

	require.Error(t, err)
	require.ErrorIs(t, err, ErrLocalNetworkAccess)
}

func TestNewSSRFProtectedHTTPClient_DialContext_BlockedHostname(t *testing.T) {
	client := newSSRFProtectedHTTPClient(nil)
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)

	conn, err := transport.DialContext(context.Background(), "tcp", "localhost:80")
	if conn != nil {
		_ = conn.Close()
	}

	require.Error(t, err)
	require.ErrorIs(t, err, ErrLocalNetworkAccess)
}

func TestNewSSRFProtectedHTTPClient_DialContext_AllowsPublicIPLiteral(t *testing.T) {
	client := newSSRFProtectedHTTPClient(nil)
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)

	conn, err := transport.DialContext(context.Background(), "invalid-network", "8.8.8.8:443")
	if conn != nil {
		_ = conn.Close()
	}

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown network")
}

func TestNewSSRFProtectedHTTPClientWithLookup_DNSFailure(t *testing.T) {
	client := newSSRFProtectedHTTPClientWithLookup(nil, func(string) ([]net.IP, error) {
		return nil, errors.New("boom")
	})
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)

	conn, err := transport.DialContext(context.Background(), "tcp", "example.com:443")
	if conn != nil {
		_ = conn.Close()
	}

	require.Error(t, err)
	assert.Contains(t, err.Error(), "DNS resolution failed")
}

func TestNewSSRFProtectedHTTPClientWithLookup_BlockedIPFromDNS(t *testing.T) {
	client := newSSRFProtectedHTTPClientWithLookup(nil, func(string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	})
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)

	conn, err := transport.DialContext(context.Background(), "tcp", "example.com:443")
	if conn != nil {
		_ = conn.Close()
	}

	require.Error(t, err)
	require.ErrorIs(t, err, ErrLocalNetworkAccess)
}

func TestNewSSRFProtectedHTTPClientWithLookup_NoIPs(t *testing.T) {
	client := newSSRFProtectedHTTPClientWithLookup(nil, func(string) ([]net.IP, error) {
		return nil, nil
	})
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)

	conn, err := transport.DialContext(context.Background(), "tcp", "example.com:443")
	if conn != nil {
		_ = conn.Close()
	}

	require.Error(t, err)
	assert.Contains(t, err.Error(), "DNS resolution returned no IPs")
}

func TestNewSSRFProtectedHTTPClientWithLookup_DialContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := newSSRFProtectedHTTPClientWithLookup(nil, func(string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("8.8.8.8")}, nil
	})
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)

	conn, err := transport.DialContext(ctx, "tcp", "example.com:443")
	if conn != nil {
		_ = conn.Close()
	}

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestNewSSRFProtectedHTTPClientWithLookup_DialFailureReturnsLastError(t *testing.T) {
	client := newSSRFProtectedHTTPClientWithLookup(nil, func(string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("8.8.8.8")}, nil
	})
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)

	conn, err := transport.DialContext(context.Background(), "invalid-network", "example.com:443")
	if conn != nil {
		_ = conn.Close()
	}

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown network")
}
