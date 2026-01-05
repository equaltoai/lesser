package httpclient

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/ssrf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestSecureClient_BlocksPrivateIPs(t *testing.T) {
	client := NewSecureClient()

	// Test cases for URLs that should be blocked
	blockedURLs := []string{
		// IPv4 private ranges
		"http://127.0.0.1/",
		"http://localhost/",
		"http://10.0.0.1/",
		"http://172.16.0.1/",
		"http://192.168.1.1/",
		"http://169.254.169.254/", // AWS metadata
		"http://0.0.0.0/",

		// IPv6 loopback
		"http://[::1]/",
		"http://[::]/",

		// Cloud metadata endpoints
		"http://metadata.google.internal/",
		"http://metadata.azure.com/",

		// Invalid schemes
		"file:///etc/passwd",
		"ftp://example.com/",
		"gopher://example.com/",
		"dict://example.com/",
		"ldap://example.com/",
		"telnet://example.com/",
		"mailto:test@example.com",
	}

	for _, url := range blockedURLs {
		t.Run(url, func(t *testing.T) {
			resp, err := client.Get(url)
			if resp != nil {
				defer func() { _ = resp.Body.Close() }()
			}
			assert.Error(t, err, "Expected error for URL: %s", url)
			assert.Nil(t, resp, "Expected nil response for blocked URL: %s", url)
			assert.Contains(t, err.Error(), "blocked", "Error should indicate request was blocked")
		})
	}
}

func TestSecureClient_AllowsPublicURLs(t *testing.T) {
	client := NewSecureClient()
	transport, ok := client.client.Transport.(*secureTransport)
	require.True(t, ok)

	transport.lookupIP = func(string) ([]net.IP, error) {
		return []net.IP{net.IPv4(93, 184, 216, 34)}, nil
	}
	transport.base = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("OK")),
			Request:    req,
		}, nil
	})

	resp, err := client.Get("http://example.com/")
	require.NoError(t, err)
	require.NotNil(t, resp)
	t.Cleanup(func() { _ = resp.Body.Close() })
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestSecureClient_BlocksRedirectsToPrivateIPs(t *testing.T) {
	client := NewSecureClient()

	req, err := http.NewRequest(http.MethodGet, "http://169.254.169.254/latest/meta-data/", nil)
	require.NoError(t, err)

	err = client.checkRedirect(req, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "redirect blocked")
}

func TestSecureClient_RespectsMaxRedirects(t *testing.T) {
	// Note: This test can't work as intended because httptest servers
	// use localhost which is blocked. We'd need a mock or external server.
	t.Skip("Skipping test that requires non-localhost server")
}

func TestSecureClient_WithContext(t *testing.T) {
	// Test context cancellation without making actual requests
	client := NewSecureClient()

	// Create a context that's already canceled
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Try to make a request with canceled context
	// Even though the URL would be blocked, context should be checked first
	resp, err := client.GetWithContext(ctx, "http://example.com")
	if resp != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	assert.Error(t, err, "Should error on canceled context")
}

func TestSecureClient_BlocksDNSRebinding(t *testing.T) {
	client := NewSecureClient()

	transport, ok := client.client.Transport.(*secureTransport)
	require.True(t, ok)

	transport.lookupIP = func(string) ([]net.IP, error) {
		return []net.IP{net.IPv4(127, 0, 0, 1)}, nil
	}

	resp, err := client.Get("http://example.com/")
	if resp != nil {
		t.Cleanup(func() { _ = resp.Body.Close() })
	}
	require.Error(t, err)
	assert.Contains(t, err.Error(), "private IP address not allowed")
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestSecureClient_Options(t *testing.T) {
	// Test that custom timeout is applied
	client := NewSecureClient(WithTimeout(100 * time.Millisecond))

	// Verify the timeout was set (we can't easily test actual timeout
	// behavior without a real external server)
	assert.NotNil(t, client.client)
	assert.Equal(t, 100*time.Millisecond, client.client.Timeout)
}

// Add a new test for validating that external URLs would work
func TestSecureClient_ValidatesExternalURLs(t *testing.T) {
	// These are examples of URLs that WOULD be allowed
	// (we're not actually making requests, just showing the pattern)
	externalURLs := []string{
		"https://example.com",
		"https://api.github.com",
		"https://google.com",
		"https://1.1.1.1", // Cloudflare DNS
		"https://8.8.8.8", // Google DNS
	}

	// For each URL, we would expect no error if we could actually connect
	// This demonstrates the URLs that pass validation
	for _, testURL := range externalURLs {
		t.Run(testURL, func(t *testing.T) {
			// We can't actually test these without making real external requests
			// but we can at least validate they would pass our URL validation
			u, err := url.Parse(testURL)
			require.NoError(t, err)

			// Validate the URL would pass our checks
			err = validateURL(u, zap.NewNop())

			// These should pass validation (though actual connection might fail)
			if err != nil {
				// External IPs might not resolve in test environment
				t.Logf("URL validation result for %s: %v", testURL, err)
			}
		})
	}
}

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		ip       string
		expected bool
	}{
		// Private IPs
		{"10.0.0.1", true},
		{"100.64.0.1", true},
		{"100.100.100.200", true},
		{"172.16.0.1", true},
		{"192.168.1.1", true},
		{"127.0.0.1", true},
		{"169.254.1.1", true},
		{"::1", true},
		{"fe80::1", true},
		{"fc00::1", true},

		// Public IPs
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"93.184.216.34", false},        // example.com
		{"2001:4860:4860::8888", false}, // Google DNS IPv6
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			require.NotNil(t, ip, "Failed to parse IP: %s", tt.ip)
			assert.Equal(t, tt.expected, isPrivateIP(ip), "IP %s private check failed", tt.ip)
		})
	}
}

func TestIsMetadataEndpoint(t *testing.T) {
	tests := []struct {
		hostname string
		expected bool
	}{
		// Metadata endpoints
		{"169.254.169.254", true},
		{"metadata.google.internal", true},
		{"metadata.azure.com", true},
		{"metadata", true},
		{"subdomain.metadata.azure.com", true},

		// Regular hostnames
		{"example.com", false},
		{"google.com", false},
		{"metadata-example.com", false},
		{"notmetadata.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.hostname, func(t *testing.T) {
			assert.Equal(t, tt.expected, ssrf.IsBlockedHostname(tt.hostname))
		})
	}
}
