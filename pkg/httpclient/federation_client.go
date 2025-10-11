// Package httpclient provides HTTP clients for federation with SSRF protection
package httpclient

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

// FederationClient provides a secure HTTP client for ActivityPub federation
type FederationClient struct {
	client *http.Client
	logger *zap.Logger
}

// FederationClientConfig defines configuration for the federation client
type FederationClientConfig struct {
	Timeout              time.Duration
	MaxRedirects         int
	UserAgent            string
	AllowInsecureTLS     bool
	AllowPrivateNetworks bool
	MaxResponseSize      int64
	DNSTimeout           time.Duration
}

// DefaultFederationClientConfig returns default configuration
func DefaultFederationClientConfig() *FederationClientConfig {
	return &FederationClientConfig{
		Timeout:              30 * time.Second,
		MaxRedirects:         10,
		UserAgent:            "Lesser/1.0 (+https://github.com/equaltoai/lesser)",
		AllowInsecureTLS:     false,
		AllowPrivateNetworks: false,
		MaxResponseSize:      10 * 1024 * 1024, // 10MB
		DNSTimeout:           5 * time.Second,
	}
}

// NewFederationClient creates a new secure HTTP client for ActivityPub federation
func NewFederationClient(config *FederationClientConfig, logger *zap.Logger) *FederationClient {
	if config == nil {
		config = DefaultFederationClientConfig()
	}

	// Create custom dialer with DNS validation
	dialer := &net.Dialer{
		Timeout: config.DNSTimeout,
	}

	// Create secure transport
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			return secureDialContext(ctx, dialer, network, address, config, logger)
		},
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableCompression:    false,
		ForceAttemptHTTP2:     true,
	}

	// Configure TLS
	if config.AllowInsecureTLS {
		transport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	} else {
		transport.TLSClientConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}

	// Create HTTP client
	client := &http.Client{
		Timeout:   config.Timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= config.MaxRedirects {
				return fmt.Errorf("too many redirects: %d", len(via))
			}
			
			// Validate redirect URL for SSRF protection
			if err := validateFederationURL(req.URL.String(), config, logger); err != nil {
				return fmt.Errorf("redirect URL blocked: %w", err)
			}
			
			return nil
		},
	}

	return &FederationClient{
		client: client,
		logger: logger,
	}
}

// secureDialContext provides SSRF protection during connection establishment
func secureDialContext(ctx context.Context, dialer *net.Dialer, network, address string, config *FederationClientConfig, logger *zap.Logger) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("invalid address format: %w", err)
	}

	// Resolve hostname to IPs
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, fmt.Errorf("DNS resolution failed: %w", err)
	}

	// Check all resolved IPs for SSRF threats
	if !config.AllowPrivateNetworks {
		for _, ip := range ips {
			if isBlockedIP(ip) {
				logger.Warn("blocked connection to private IP",
					zap.String("host", host),
					zap.String("ip", ip.String()))
				return nil, fmt.Errorf("connection to private IP blocked: %s resolves to %s", host, ip.String())
			}
		}
	}

	// Use the original dialer to establish connection
	return dialer.DialContext(ctx, network, net.JoinHostPort(host, port))
}

// isBlockedIP checks if an IP address should be blocked for SSRF protection
func isBlockedIP(ip net.IP) bool {
	// Loopback addresses (127.0.0.0/8, ::1/128)
	if ip.IsLoopback() {
		return true
	}

	// Link-local addresses
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	// Private IPv4 ranges
	privateIPv4Ranges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"169.254.0.0/16", // Link-local
	}

	for _, cidr := range privateIPv4Ranges {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}

	// Private IPv6 ranges
	if ip.To4() == nil { // IPv6
		// Unique local addresses (fc00::/7)
		if len(ip) >= 1 && (ip[0]&0xfe) == 0xfc {
			return true
		}
		// Site-local addresses (fec0::/10) - deprecated but still blocked
		if len(ip) >= 2 && ip[0] == 0xfe && (ip[1]&0xc0) == 0xc0 {
			return true
		}
	}

	// Block certain cloud metadata endpoints
	metadataIPs := []string{
		"169.254.169.254", // AWS, Azure, GCP metadata
		"169.254.170.2",   // AWS ECS metadata
		"100.100.100.200", // Alibaba Cloud metadata
	}

	for _, metadataIP := range metadataIPs {
		if ip.Equal(net.ParseIP(metadataIP)) {
			return true
		}
	}

	return false
}

// validateFederationURL validates a URL for SSRF protection
func validateFederationURL(rawURL string, config *FederationClientConfig, logger *zap.Logger) error {
	if strings.Contains(rawURL, "127.0.0.1") ||
		strings.Contains(rawURL, "localhost") ||
		strings.Contains(rawURL, "::1") {
		return fmt.Errorf("localhost URLs not allowed")
	}

	// Block common internal hostnames
	internalHostnames := []string{
		"metadata.google.internal",
		"instance-data",
		"consul",
		"vault",
	}

	for _, hostname := range internalHostnames {
		if strings.Contains(rawURL, hostname) {
			return fmt.Errorf("internal hostname not allowed: %s", hostname)
		}
	}

	return nil
}

// Get performs a GET request with ActivityPub headers
func (fc *FederationClient) Get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set ActivityPub headers
	fc.setActivityPubHeaders(req)

	fc.logger.Debug("making federation GET request",
		zap.String("url", url),
		zap.String("user_agent", req.Header.Get("User-Agent")))

	return fc.client.Do(req)
}

// Post performs a POST request with ActivityPub headers
func (fc *FederationClient) Post(ctx context.Context, url string, contentType string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set ActivityPub headers
	fc.setActivityPubHeaders(req)
	req.Header.Set("Content-Type", contentType)

	fc.logger.Debug("making federation POST request",
		zap.String("url", url),
		zap.String("content_type", contentType),
		zap.Int("body_size", len(body)))

	return fc.client.Do(req)
}

// setActivityPubHeaders sets appropriate headers for ActivityPub federation
func (fc *FederationClient) setActivityPubHeaders(req *http.Request) {
	// Set User-Agent
	req.Header.Set("User-Agent", "Lesser/1.0 (+https://github.com/equaltoai/lesser)")
	
	// Set Accept header for ActivityPub
	req.Header.Set("Accept", "application/activity+json, application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\", application/json")
	
	// Set Date header for HTTP signature
	req.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	
	// Set Host header (required for HTTP signatures)
	req.Header.Set("Host", req.URL.Host)
}

// GetWithUserAgent performs a GET request with custom user agent
func (fc *FederationClient) GetWithUserAgent(ctx context.Context, url, userAgent string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set custom User-Agent
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/activity+json, application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\", application/json")

	fc.logger.Debug("making federation GET request with custom UA",
		zap.String("url", url),
		zap.String("user_agent", userAgent))

	return fc.client.Do(req)
}

// Close closes any idle connections
func (fc *FederationClient) Close() {
	if transport, ok := fc.client.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}
}

// SetTimeout updates the client timeout
func (fc *FederationClient) SetTimeout(timeout time.Duration) {
	fc.client.Timeout = timeout
}

// GetClient returns the underlying HTTP client (use carefully)
func (fc *FederationClient) GetClient() *http.Client {
	return fc.client
}