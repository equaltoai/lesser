package services

import (
	"crypto/tls"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecureHTTPClientConfig(t *testing.T) {
	t.Run("DefaultSecureHTTPClientConfig returns secure defaults", func(t *testing.T) {
		config := DefaultSecureHTTPClientConfig()

		// Verify connection timeouts
		assert.Equal(t, 10*time.Second, config.ConnectTimeout, "Connect timeout should be 10 seconds")
		assert.Equal(t, 30*time.Second, config.RequestTimeout, "Request timeout should be 30 seconds")
		assert.Equal(t, 15*time.Second, config.ResponseTimeout, "Response timeout should be 15 seconds")
		assert.Equal(t, 30*time.Second, config.KeepAliveTimeout, "Keep-alive timeout should be 30 seconds")

		// Verify TLS settings
		assert.Equal(t, 10*time.Second, config.TLSHandshakeTimeout, "TLS handshake timeout should be 10 seconds")
		assert.False(t, config.InsecureSkipVerify, "TLS verification should be enabled by default")

		// Verify connection limits
		assert.Equal(t, 100, config.MaxIdleConns, "Max idle connections should be 100")
		assert.Equal(t, 10, config.MaxIdleConnsPerHost, "Max idle connections per host should be 10")
		assert.Equal(t, 20, config.MaxConnsPerHost, "Max connections per host should be 20")
		assert.Equal(t, 90*time.Second, config.IdleConnTimeout, "Idle connection timeout should be 90 seconds")

		// Verify request limits
		assert.Equal(t, int64(1<<20), config.MaxResponseHeaderBytes, "Max response header bytes should be 1MB")
		assert.False(t, config.DisableCompression, "Compression should be enabled")
		assert.False(t, config.DisableKeepAlives, "Keep-alives should be enabled")

		// Verify security
		assert.Equal(t, "lift-client/1.0", config.UserAgent, "User agent should be set")
	})

	t.Run("ProductionHTTPClientConfig returns production settings", func(t *testing.T) {
		config := ProductionHTTPClientConfig()

		// Production should have more conservative settings
		assert.Equal(t, 20*time.Second, config.RequestTimeout, "Production request timeout should be 20 seconds")
		assert.Equal(t, 5*time.Second, config.ConnectTimeout, "Production connect timeout should be 5 seconds")
		assert.Equal(t, 5, config.MaxIdleConnsPerHost, "Production max idle connections per host should be 5")
		assert.Equal(t, 10, config.MaxConnsPerHost, "Production max connections per host should be 10")
		assert.Equal(t, int64(512*1024), config.MaxResponseHeaderBytes, "Production max header bytes should be 512KB")
	})

	t.Run("DevelopmentHTTPClientConfig returns development settings", func(t *testing.T) {
		config := DevelopmentHTTPClientConfig()

		// Development should have more lenient settings
		assert.Equal(t, 60*time.Second, config.RequestTimeout, "Development request timeout should be 60 seconds")
		assert.Equal(t, 15*time.Second, config.ConnectTimeout, "Development connect timeout should be 15 seconds")
		assert.True(t, config.InsecureSkipVerify, "Development should skip TLS verification")
	})
}

func TestNewSecureHTTPClient(t *testing.T) {
	t.Run("Creates client with secure configuration", func(t *testing.T) {
		config := DefaultSecureHTTPClientConfig()
		client := NewSecureHTTPClient(config)

		require.NotNil(t, client, "Client should be created")
		assert.Equal(t, config.RequestTimeout, client.Timeout, "Client timeout should match config")

		// Verify transport configuration
		transport, ok := client.Transport.(*http.Transport)
		require.True(t, ok, "Transport should be *http.Transport")

		assert.Equal(t, config.TLSHandshakeTimeout, transport.TLSHandshakeTimeout, "TLS handshake timeout should match")
		assert.Equal(t, config.MaxIdleConns, transport.MaxIdleConns, "Max idle connections should match")
		assert.Equal(t, config.MaxIdleConnsPerHost, transport.MaxIdleConnsPerHost, "Max idle connections per host should match")
		assert.Equal(t, config.MaxConnsPerHost, transport.MaxConnsPerHost, "Max connections per host should match")
		assert.Equal(t, config.IdleConnTimeout, transport.IdleConnTimeout, "Idle connection timeout should match")
		assert.Equal(t, config.MaxResponseHeaderBytes, transport.MaxResponseHeaderBytes, "Max response header bytes should match")
		assert.Equal(t, config.DisableCompression, transport.DisableCompression, "Compression setting should match")
		assert.Equal(t, config.DisableKeepAlives, transport.DisableKeepAlives, "Keep-alives setting should match")
		assert.Equal(t, config.ResponseTimeout, transport.ResponseHeaderTimeout, "Response header timeout should match")

		// Verify TLS configuration
		require.NotNil(t, transport.TLSClientConfig, "TLS config should be set")
		assert.Equal(t, config.InsecureSkipVerify, transport.TLSClientConfig.InsecureSkipVerify, "TLS skip verify should match")
		assert.Equal(t, uint16(tls.VersionTLS12), transport.TLSClientConfig.MinVersion, "Minimum TLS version should be 1.2")
	})
}

func TestValidateHTTPClientSecurity(t *testing.T) {
	t.Run("Validates secure client has no issues", func(t *testing.T) {
		config := ProductionHTTPClientConfig()
		client := NewSecureHTTPClient(config)

		issues := ValidateHTTPClientSecurity(client)
		assert.Empty(t, issues, "Secure client should have no security issues")
	})

	t.Run("Detects insecure client configurations", func(t *testing.T) {
		// Test client with no timeout
		insecureClient := &http.Client{}
		issues := ValidateHTTPClientSecurity(insecureClient)
		assert.Contains(t, issues, "No request timeout configured - vulnerable to DoS attacks", "Should detect missing timeout")

		// Test client with very long timeout
		longTimeoutClient := &http.Client{Timeout: 5 * time.Minute}
		issues = ValidateHTTPClientSecurity(longTimeoutClient)
		assert.Contains(t, issues, "Request timeout too long - may cause resource exhaustion", "Should detect long timeout")

		// Test client with insecure transport
		insecureTransport := &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // #nosec G402 - test code for insecure transport validation
			},
		}
		insecureTLSClient := &http.Client{
			Transport: insecureTransport,
			Timeout:   30 * time.Second,
		}
		issues = ValidateHTTPClientSecurity(insecureTLSClient)
		assert.Contains(t, issues, "TLS verification disabled - vulnerable to MITM attacks", "Should detect insecure TLS")
	})

	t.Run("Detects transport configuration issues", func(t *testing.T) {
		// Test transport with no response header timeout
		transport := &http.Transport{
			MaxIdleConnsPerHost:    10,
			MaxConnsPerHost:        20,
			MaxResponseHeaderBytes: 1024 * 1024,
		}
		client := &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		}

		issues := ValidateHTTPClientSecurity(client)
		assert.Contains(t, issues, "No response header timeout - vulnerable to slow response attacks", "Should detect missing response header timeout")
		assert.Contains(t, issues, "No TLS handshake timeout - vulnerable to slow handshake attacks", "Should detect missing TLS handshake timeout")
	})

	t.Run("Detects resource exhaustion risks", func(t *testing.T) {
		// Test transport with too many connections
		transport := &http.Transport{
			MaxIdleConnsPerHost:    100,              // Too many
			MaxConnsPerHost:        200,              // Too many
			MaxResponseHeaderBytes: 50 * 1024 * 1024, // Too large
			ResponseHeaderTimeout:  5 * time.Second,
			TLSHandshakeTimeout:    5 * time.Second,
		}
		client := &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		}

		issues := ValidateHTTPClientSecurity(client)
		assert.Contains(t, issues, "Too many idle connections per host - may cause resource exhaustion", "Should detect too many idle connections")
		assert.Contains(t, issues, "Too many connections per host - may cause resource exhaustion", "Should detect too many connections")
		assert.Contains(t, issues, "Response header size limit too high - vulnerable to memory exhaustion", "Should detect large header limit")
	})
}

func TestHTTPClientMiddleware(t *testing.T) {
	t.Run("Creates new client when none provided", func(t *testing.T) {
		config := DefaultSecureHTTPClientConfig()
		middleware := HTTPClientMiddleware(config)

		client := middleware(nil)
		require.NotNil(t, client, "Should create new client")
		assert.Equal(t, config.RequestTimeout, client.Timeout, "Should have configured timeout")
	})

	t.Run("Enhances existing client", func(t *testing.T) {
		config := ProductionHTTPClientConfig()
		middleware := HTTPClientMiddleware(config)

		// Create an insecure client
		existingClient := &http.Client{
			Timeout: 5 * time.Minute, // Too long
		}

		enhancedClient := middleware(existingClient)
		assert.Equal(t, config.RequestTimeout, enhancedClient.Timeout, "Should update timeout to secure value")
	})

	t.Run("Updates default transport", func(t *testing.T) {
		config := DefaultSecureHTTPClientConfig()
		middleware := HTTPClientMiddleware(config)

		// Create client with default transport
		existingClient := &http.Client{
			Transport: http.DefaultTransport,
			Timeout:   30 * time.Second,
		}

		enhancedClient := middleware(existingClient)
		assert.NotEqual(t, http.DefaultTransport, enhancedClient.Transport, "Should replace default transport")
	})
}

func TestSecureHTTPClientExample(t *testing.T) {
	t.Run("Example creates secure client", func(t *testing.T) {
		client := SecureHTTPClientExample()

		require.NotNil(t, client, "Example should create client")

		// Validate the client is secure
		issues := ValidateHTTPClientSecurity(client)
		assert.Empty(t, issues, "Example client should be secure")
	})
}

func TestHTTPClientTimeoutBoundaries(t *testing.T) {
	t.Run("Production timeouts are within security boundaries", func(t *testing.T) {
		config := ProductionHTTPClientConfig()

		// Verify production timeouts are conservative
		assert.LessOrEqual(t, config.RequestTimeout, 30*time.Second, "Production request timeout should be <= 30s")
		assert.LessOrEqual(t, config.ConnectTimeout, 10*time.Second, "Production connect timeout should be <= 10s")
		assert.LessOrEqual(t, config.ResponseTimeout, 20*time.Second, "Production response timeout should be <= 20s")
		assert.LessOrEqual(t, config.MaxIdleConnsPerHost, 10, "Production max idle connections per host should be <= 10")
		assert.LessOrEqual(t, config.MaxConnsPerHost, 20, "Production max connections per host should be <= 20")
		assert.LessOrEqual(t, config.MaxResponseHeaderBytes, int64(1<<20), "Production max header bytes should be <= 1MB")
	})

	t.Run("Development timeouts are reasonable", func(t *testing.T) {
		config := DevelopmentHTTPClientConfig()

		// Development can be more lenient but still reasonable
		assert.LessOrEqual(t, config.RequestTimeout, 120*time.Second, "Development request timeout should be <= 2 minutes")
		assert.LessOrEqual(t, config.ConnectTimeout, 30*time.Second, "Development connect timeout should be <= 30s")
		assert.LessOrEqual(t, config.MaxResponseHeaderBytes, int64(10<<20), "Development max header bytes should be <= 10MB")
	})
}
