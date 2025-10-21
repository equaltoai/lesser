package services

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

// SecureHTTPClientConfig provides comprehensive HTTP client security configuration
type SecureHTTPClientConfig struct {
	UserAgent              string        `json:"user_agent"`
	MaxIdleConns           int           `json:"max_idle_conns"`
	ResponseTimeout        time.Duration `json:"response_timeout"`
	KeepAliveTimeout       time.Duration `json:"keepalive_timeout"`
	TLSHandshakeTimeout    time.Duration `json:"tls_handshake_timeout"`
	ConnectTimeout         time.Duration `json:"connect_timeout"`
	MaxIdleConnsPerHost    int           `json:"max_idle_conns_per_host"`
	MaxConnsPerHost        int           `json:"max_conns_per_host"`
	IdleConnTimeout        time.Duration `json:"idle_conn_timeout"`
	MaxResponseHeaderBytes int64         `json:"max_response_header_bytes"`
	RequestTimeout         time.Duration `json:"request_timeout"`
	InsecureSkipVerify     bool          `json:"insecure_skip_verify"`
	DisableCompression     bool          `json:"disable_compression"`
	DisableKeepAlives      bool          `json:"disable_keep_alives"`
}

// DefaultSecureHTTPClientConfig returns secure default configuration
func DefaultSecureHTTPClientConfig() SecureHTTPClientConfig {
	return SecureHTTPClientConfig{
		// Connection timeouts (prevent slow connections)
		ConnectTimeout:   10 * time.Second,
		RequestTimeout:   30 * time.Second,
		ResponseTimeout:  15 * time.Second,
		KeepAliveTimeout: 30 * time.Second,

		// TLS security
		TLSHandshakeTimeout: 10 * time.Second,
		InsecureSkipVerify:  false,

		// Connection pooling (prevent resource exhaustion)
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		MaxConnsPerHost:     20,
		IdleConnTimeout:     90 * time.Second,

		// Request limits (prevent large response attacks)
		MaxResponseHeaderBytes: 1 << 20, // 1 MB
		DisableCompression:     false,
		DisableKeepAlives:      false,

		// Security
		UserAgent: "lift-client/1.0",
	}
}

// ProductionHTTPClientConfig returns production-ready configuration
func ProductionHTTPClientConfig() SecureHTTPClientConfig {
	config := DefaultSecureHTTPClientConfig()

	// More conservative production settings
	config.RequestTimeout = 20 * time.Second   // Shorter request timeout
	config.ConnectTimeout = 5 * time.Second    // Faster connection timeout
	config.MaxIdleConnsPerHost = 5             // Lower connection pool
	config.MaxConnsPerHost = 10                // Lower max connections
	config.MaxResponseHeaderBytes = 512 * 1024 // 512KB max headers

	return config
}

// DevelopmentHTTPClientConfig returns development-friendly configuration
func DevelopmentHTTPClientConfig() SecureHTTPClientConfig {
	config := DefaultSecureHTTPClientConfig()

	// More lenient development settings
	config.RequestTimeout = 60 * time.Second // Longer for debugging
	config.ConnectTimeout = 15 * time.Second // Longer for local services
	config.InsecureSkipVerify = true         // Skip TLS for local dev

	return config
}

// NewSecureHTTPClient creates a secure HTTP client with comprehensive timeout configurations
func NewSecureHTTPClient(config SecureHTTPClientConfig) *http.Client {
	// Create custom transport with security settings
	transport := &http.Transport{
		// Connection timeouts
		DialContext: (&net.Dialer{
			Timeout:   config.ConnectTimeout,
			KeepAlive: config.KeepAliveTimeout,
		}).DialContext,

		// TLS security
		TLSHandshakeTimeout: config.TLSHandshakeTimeout,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: config.InsecureSkipVerify, // #nosec G402 - configurable for development/testing
			MinVersion:         tls.VersionTLS12,          // Minimum TLS 1.2
		},

		// Connection pooling (prevent resource exhaustion)
		MaxIdleConns:        config.MaxIdleConns,
		MaxIdleConnsPerHost: config.MaxIdleConnsPerHost,
		MaxConnsPerHost:     config.MaxConnsPerHost,
		IdleConnTimeout:     config.IdleConnTimeout,

		// Response limits (prevent large response attacks)
		MaxResponseHeaderBytes: config.MaxResponseHeaderBytes,
		DisableCompression:     config.DisableCompression,
		DisableKeepAlives:      config.DisableKeepAlives,

		// Response header timeout (prevent slow response attacks)
		ResponseHeaderTimeout: config.ResponseTimeout,
	}

	// Create HTTP client with transport and total timeout
	client := &http.Client{
		Transport: transport,
		Timeout:   config.RequestTimeout, // Total request timeout
	}

	return client
}

// HTTPClientMiddleware provides HTTP client configuration middleware
func HTTPClientMiddleware(config SecureHTTPClientConfig) func(*http.Client) *http.Client {
	return func(existingClient *http.Client) *http.Client {
		// If no existing client, create new secure one
		if existingClient == nil {
			return NewSecureHTTPClient(config)
		}

		// Enhance existing client with security settings
		if existingClient.Timeout == 0 || existingClient.Timeout > config.RequestTimeout {
			existingClient.Timeout = config.RequestTimeout
		}

		// Update transport if it's the default transport
		if existingClient.Transport == http.DefaultTransport {
			existingClient.Transport = NewSecureHTTPClient(config).Transport
		}

		return existingClient
	}
}

// ValidateHTTPClientSecurity validates HTTP client security configuration
func ValidateHTTPClientSecurity(client *http.Client) []string {
	var issues []string

	// Check for timeout configuration
	if client.Timeout == 0 {
		issues = append(issues, "No request timeout configured - vulnerable to DoS attacks")
	} else if client.Timeout > 60*time.Second {
		issues = append(issues, "Request timeout too long - may cause resource exhaustion")
	}

	// Check transport configuration
	if transport, ok := client.Transport.(*http.Transport); ok {
		// Check connection timeouts
		if transport.ResponseHeaderTimeout == 0 {
			issues = append(issues, "No response header timeout - vulnerable to slow response attacks")
		}

		if transport.TLSHandshakeTimeout == 0 {
			issues = append(issues, "No TLS handshake timeout - vulnerable to slow handshake attacks")
		}

		// Check connection limits
		if transport.MaxIdleConnsPerHost > 50 {
			issues = append(issues, "Too many idle connections per host - may cause resource exhaustion")
		}

		if transport.MaxConnsPerHost > 100 {
			issues = append(issues, "Too many connections per host - may cause resource exhaustion")
		}

		// Check response limits
		if transport.MaxResponseHeaderBytes == 0 || transport.MaxResponseHeaderBytes > 10<<20 {
			issues = append(issues, "Response header size limit too high - vulnerable to memory exhaustion")
		}

		// Check TLS security
		if transport.TLSClientConfig != nil && transport.TLSClientConfig.InsecureSkipVerify {
			issues = append(issues, "TLS verification disabled - vulnerable to MITM attacks")
		}
	} else {
		issues = append(issues, "Custom transport detected - cannot validate security settings")
	}

	return issues
}

// SecureHTTPClientExample demonstrates secure HTTP client usage
func SecureHTTPClientExample() *http.Client {
	// Production configuration
	config := ProductionHTTPClientConfig()

	// Create secure client
	client := NewSecureHTTPClient(config)

	// Validate security
	if issues := ValidateHTTPClientSecurity(client); len(issues) > 0 {
		// In production, log these issues
		for _, issue := range issues {
			_ = issue // Would log: fmt.Printf("HTTP Client Security Issue: %s\n", issue)
		}
	}

	return client
}
