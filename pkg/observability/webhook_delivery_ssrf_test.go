package observability

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebhookCheckRedirect(t *testing.T) {
	t.Run("rejects too many redirects", func(t *testing.T) {
		req := &http.Request{URL: mustParseURL("https://example.com")}
		via := make([]*http.Request, webhookMaxRedirects)
		err := webhookCheckRedirect(req, via)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "too many redirects")
	})

	t.Run("rejects blocked redirect URL", func(t *testing.T) {
		req := &http.Request{URL: mustParseURL("http://localhost:8080")}
		err := webhookCheckRedirect(req, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "redirect URL blocked")
	})

	t.Run("allows valid redirect URL", func(t *testing.T) {
		req := &http.Request{URL: mustParseURL("https://example.com")}
		err := webhookCheckRedirect(req, nil)
		require.NoError(t, err)
	})
}

func TestDialWithSSRFProtectionWithLookup(t *testing.T) {
	t.Run("invalid dial address returns error", func(t *testing.T) {
		conn, err := dialWithSSRFProtectionWithLookup(context.Background(), nil, "tcp", "missing-port", func(string) ([]net.IP, error) {
			return nil, errors.New("lookup should not be called")
		})
		if conn != nil {
			_ = conn.Close()
		}
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid dial address")
	})

	t.Run("blocks private IP literal", func(t *testing.T) {
		conn, err := dialWithSSRFProtectionWithLookup(context.Background(), nil, "tcp", "127.0.0.1:80", func(string) ([]net.IP, error) {
			return nil, errors.New("lookup should not be called")
		})
		if conn != nil {
			_ = conn.Close()
		}
		require.Error(t, err)
		assert.Contains(t, err.Error(), "blocked dial to private IP")
	})

	t.Run("attempts dial for allowed IP literal", func(t *testing.T) {
		conn, err := dialWithSSRFProtectionWithLookup(context.Background(), nil, "invalid-network", "8.8.8.8:443", func(string) ([]net.IP, error) {
			return nil, errors.New("lookup should not be called")
		})
		if conn != nil {
			_ = conn.Close()
		}
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown network")
	})

	t.Run("blocks internal hostname", func(t *testing.T) {
		conn, err := dialWithSSRFProtectionWithLookup(context.Background(), nil, "tcp", "localhost:80", func(string) ([]net.IP, error) {
			return nil, errors.New("lookup should not be called")
		})
		if conn != nil {
			_ = conn.Close()
		}
		require.Error(t, err)
		assert.Contains(t, err.Error(), "blocked dial to internal hostname")
	})

	t.Run("DNS failure is wrapped", func(t *testing.T) {
		conn, err := dialWithSSRFProtectionWithLookup(context.Background(), nil, "tcp", "example.com:443", func(string) ([]net.IP, error) {
			return nil, errors.New("boom")
		})
		if conn != nil {
			_ = conn.Close()
		}
		require.Error(t, err)
		assert.Contains(t, err.Error(), "DNS resolution failed")
	})

	t.Run("blocks private IP from DNS", func(t *testing.T) {
		conn, err := dialWithSSRFProtectionWithLookup(context.Background(), nil, "tcp", "example.com:443", func(string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("127.0.0.1")}, nil
		})
		if conn != nil {
			_ = conn.Close()
		}
		require.Error(t, err)
		assert.Contains(t, err.Error(), "blocked dial to private IP")
	})

	t.Run("ctx cancellation aborts before dialing", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		conn, err := dialWithSSRFProtectionWithLookup(ctx, nil, "tcp", "example.com:443", func(string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("8.8.8.8")}, nil
		})
		if conn != nil {
			_ = conn.Close()
		}
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("returns last dial error when all IPs fail", func(t *testing.T) {
		conn, err := dialWithSSRFProtectionWithLookup(context.Background(), nil, "invalid-network", "example.com:443", func(string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("8.8.8.8"), net.ParseIP("1.1.1.1")}, nil
		})
		if conn != nil {
			_ = conn.Close()
		}
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown network")
	})

	t.Run("returns error when DNS yields no IPs", func(t *testing.T) {
		conn, err := dialWithSSRFProtectionWithLookup(context.Background(), nil, "tcp", "example.com:443", func(string) ([]net.IP, error) {
			return nil, nil
		})
		if conn != nil {
			_ = conn.Close()
		}
		require.Error(t, err)
		assert.Contains(t, err.Error(), "DNS resolution returned no IPs")
	})
}
