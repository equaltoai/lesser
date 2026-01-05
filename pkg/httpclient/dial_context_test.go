package httpclient

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestSecureTransport_DialContext_DialsResolvedIP(t *testing.T) {
	t.Parallel()

	client := NewSecureClient()
	transport := client.client.Transport.(*secureTransport)

	transport.lookupIP = func(host string) ([]net.IP, error) {
		require.Equal(t, "example.com", host)
		return []net.IP{net.IPv4(93, 184, 216, 34)}, nil
	}

	var dialed string
	transport.dial = func(_ context.Context, _, addr string) (net.Conn, error) {
		dialed = addr
		return nil, errors.New("dial invoked")
	}

	_, err := transport.dialContext(context.Background(), "tcp", "example.com:443")
	require.Error(t, err)
	assert.Equal(t, "93.184.216.34:443", dialed)
}

func TestSecureTransport_DialContext_UsesDNSCache(t *testing.T) {
	t.Parallel()

	client := NewSecureClient()
	transport := client.client.Transport.(*secureTransport)

	transport.dnsCache.setCachedIPs(context.Background(), "cached.example", []net.IP{net.IPv4(93, 184, 216, 34)})
	transport.lookupIP = func(string) ([]net.IP, error) {
		t.Fatalf("unexpected DNS lookup (expected cache hit)")
		return nil, nil
	}

	var dialed string
	transport.dial = func(_ context.Context, _, addr string) (net.Conn, error) {
		dialed = addr
		return nil, errors.New("dial invoked")
	}

	_, err := transport.dialContext(context.Background(), "tcp", "cached.example:80")
	require.Error(t, err)
	assert.Equal(t, "93.184.216.34:80", dialed)
}

func TestSecureTransport_DialContext_BlocksPrivateIPLiteral(t *testing.T) {
	t.Parallel()

	client := NewSecureClient()
	transport := client.client.Transport.(*secureTransport)

	transport.dial = func(_ context.Context, _, _ string) (net.Conn, error) {
		t.Fatalf("dial should not be invoked for blocked addresses")
		return nil, nil
	}

	_, err := transport.dialContext(context.Background(), "tcp", "127.0.0.1:80")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPrivateIPAddress)
}

func TestSecureTransport_DialContext_BlocksMixedAAndAAAA(t *testing.T) {
	t.Parallel()

	client := NewSecureClient()
	transport := client.client.Transport.(*secureTransport)

	transport.lookupIP = func(host string) ([]net.IP, error) {
		require.Equal(t, "example.com", host)
		return []net.IP{net.IPv4(93, 184, 216, 34), net.ParseIP("fe80::1")}, nil
	}

	transport.dial = func(_ context.Context, _, _ string) (net.Conn, error) {
		t.Fatalf("dial should not be invoked for blocked addresses")
		return nil, nil
	}

	_, err := transport.dialContext(context.Background(), "tcp", "example.com:443")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPrivateIPAddress)
}

func TestSecureTransport_DialContext_DialsResolvedIPv6(t *testing.T) {
	t.Parallel()

	client := NewSecureClient()
	transport := client.client.Transport.(*secureTransport)

	transport.lookupIP = func(host string) ([]net.IP, error) {
		require.Equal(t, "example.com", host)
		return []net.IP{net.ParseIP("2001:4860:4860::8888")}, nil
	}

	var dialed string
	transport.dial = func(_ context.Context, _, addr string) (net.Conn, error) {
		dialed = addr
		return nil, errors.New("dial invoked")
	}

	_, err := transport.dialContext(context.Background(), "tcp", "example.com:443")
	require.Error(t, err)
	assert.Equal(t, "[2001:4860:4860::8888]:443", dialed)
}

func TestSecureDialContext_DialsResolvedIP(t *testing.T) {
	origLookup := lookupIP
	origDial := dialerDialContext
	t.Cleanup(func() {
		lookupIP = origLookup
		dialerDialContext = origDial
	})

	lookupIP = func(host string) ([]net.IP, error) {
		require.Equal(t, "example.com", host)
		return []net.IP{net.IPv4(93, 184, 216, 34)}, nil
	}

	var dialed string
	dialerDialContext = func(_ *net.Dialer, _ context.Context, _, address string) (net.Conn, error) {
		dialed = address
		return nil, errors.New("dial invoked")
	}

	_, err := secureDialContext(context.Background(), &net.Dialer{}, "tcp", "example.com:443", DefaultFederationClientConfig(), zap.NewNop())
	require.Error(t, err)
	assert.Equal(t, "93.184.216.34:443", dialed)
}

func TestSecureDialContext_DialsResolvedIPv6(t *testing.T) {
	origLookup := lookupIP
	origDial := dialerDialContext
	t.Cleanup(func() {
		lookupIP = origLookup
		dialerDialContext = origDial
	})

	lookupIP = func(host string) ([]net.IP, error) {
		require.Equal(t, "example.com", host)
		return []net.IP{net.ParseIP("2001:4860:4860::8888")}, nil
	}

	var dialed string
	dialerDialContext = func(_ *net.Dialer, _ context.Context, _, address string) (net.Conn, error) {
		dialed = address
		return nil, errors.New("dial invoked")
	}

	_, err := secureDialContext(context.Background(), &net.Dialer{}, "tcp", "example.com:443", DefaultFederationClientConfig(), zap.NewNop())
	require.Error(t, err)
	assert.Equal(t, "[2001:4860:4860::8888]:443", dialed)
}

func TestSecureDialContext_BlocksMixedAAndAAAA(t *testing.T) {
	origLookup := lookupIP
	origDial := dialerDialContext
	t.Cleanup(func() {
		lookupIP = origLookup
		dialerDialContext = origDial
	})

	lookupIP = func(host string) ([]net.IP, error) {
		require.Equal(t, "example.com", host)
		return []net.IP{net.IPv4(93, 184, 216, 34), net.ParseIP("fc00::1")}, nil
	}
	dialerDialContext = func(_ *net.Dialer, _ context.Context, _, _ string) (net.Conn, error) {
		t.Fatalf("dial should not be invoked for blocked addresses")
		return nil, nil
	}

	_, err := secureDialContext(context.Background(), &net.Dialer{}, "tcp", "example.com:80", DefaultFederationClientConfig(), zap.NewNop())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocked")
}

func TestSecureDialContext_BlocksPrivateIP(t *testing.T) {
	origLookup := lookupIP
	origDial := dialerDialContext
	t.Cleanup(func() {
		lookupIP = origLookup
		dialerDialContext = origDial
	})

	lookupIP = func(host string) ([]net.IP, error) {
		require.Equal(t, "example.com", host)
		return []net.IP{net.IPv4(127, 0, 0, 1)}, nil
	}
	dialerDialContext = func(_ *net.Dialer, _ context.Context, _, _ string) (net.Conn, error) {
		t.Fatalf("dial should not be invoked for blocked addresses")
		return nil, nil
	}

	_, err := secureDialContext(context.Background(), &net.Dialer{}, "tcp", "example.com:80", DefaultFederationClientConfig(), zap.NewNop())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocked")
}
