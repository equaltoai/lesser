package httpclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"go.uber.org/zap"
)

var (
	// ErrBlockedRequest indicates the request was blocked for security reasons
	ErrBlockedRequest = errors.New("request blocked for security reasons")

	// ErrPrivateIPAddress indicates the target resolves to a private IP
	ErrPrivateIPAddress = errors.New("private IP address not allowed")

	// ErrInvalidScheme indicates an unsupported URL scheme
	ErrInvalidScheme = errors.New("invalid URL scheme")

	// ErrRedirectBlocked indicates a redirect was blocked
	ErrRedirectBlocked = errors.New("redirect blocked")

	// ErrDNSRebindingDetected indicates a DNS rebinding attack was detected
	ErrDNSRebindingDetected = errors.New("DNS rebinding attack detected")

	// Private IP ranges to block (RFC 1918 and others)
	privateIPBlocks = []net.IPNet{
		// IPv4 private ranges
		{IP: net.IPv4(10, 0, 0, 0), Mask: net.CIDRMask(8, 32)},     // 10.0.0.0/8
		{IP: net.IPv4(172, 16, 0, 0), Mask: net.CIDRMask(12, 32)},  // 172.16.0.0/12
		{IP: net.IPv4(192, 168, 0, 0), Mask: net.CIDRMask(16, 32)}, // 192.168.0.0/16
		{IP: net.IPv4(127, 0, 0, 0), Mask: net.CIDRMask(8, 32)},    // 127.0.0.0/8 (loopback)
		{IP: net.IPv4(169, 254, 0, 0), Mask: net.CIDRMask(16, 32)}, // 169.254.0.0/16 (link-local)
		{IP: net.IPv4(0, 0, 0, 0), Mask: net.CIDRMask(8, 32)},      // 0.0.0.0/8

		// IPv6 private ranges
		{IP: net.ParseIP("::1"), Mask: net.CIDRMask(128, 128)},   // ::1/128 (loopback)
		{IP: net.ParseIP("fc00::"), Mask: net.CIDRMask(7, 128)},  // fc00::/7 (unique local)
		{IP: net.ParseIP("fe80::"), Mask: net.CIDRMask(10, 128)}, // fe80::/10 (link-local)
		{IP: net.ParseIP("::"), Mask: net.CIDRMask(128, 128)},    // ::/128 (unspecified)
	}

	// Blocked schemes
	blockedSchemes = map[string]bool{
		"file":   true,
		"ftp":    true,
		"gopher": true,
		"dict":   true,
		"ldap":   true,
		"mailto": true,
		"telnet": true,
	}

	// DNS cache TTL
	dnsCacheTTL = 5 * time.Minute
)

// SecureClient is an HTTP client with SSRF protections
type SecureClient struct {
	client       *http.Client
	logger       *zap.Logger
	maxRedirects int
	store        core.RepositoryStorage
	dnsCache     *dnsCacheManager
}

// Option is a functional option for configuring SecureClient
type Option func(*SecureClient)

// WithTimeout sets a custom timeout
func WithTimeout(timeout time.Duration) Option {
	return func(c *SecureClient) {
		c.client.Timeout = timeout
	}
}

// WithLogger sets a custom logger
func WithLogger(logger *zap.Logger) Option {
	return func(c *SecureClient) {
		c.logger = logger
	}
}

// WithMaxRedirects sets the maximum number of redirects to follow
func WithMaxRedirects(max int) Option {
	return func(c *SecureClient) {
		c.maxRedirects = max
	}
}

// WithStorage sets the storage backend for DNS caching
func WithStorage(store core.RepositoryStorage) Option {
	return func(c *SecureClient) {
		c.store = store
		if c.dnsCache != nil {
			c.dnsCache.store = store
		}
	}
}

// NewSecureClient creates a new secure HTTP client with SSRF protections
func NewSecureClient(opts ...Option) *SecureClient {
	c := &SecureClient{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger:       zap.NewNop(), // Default to no-op logger
		maxRedirects: 5,
	}

	// Apply options
	for _, opt := range opts {
		opt(c)
	}

	// Initialize DNS cache manager
	c.dnsCache = &dnsCacheManager{
		store:  c.store,
		logger: c.logger,
		local:  make(map[string]*storage.DNSCacheEntry),
	}

	// Configure transport with security checks
	c.client.Transport = &secureTransport{
		base:     http.DefaultTransport,
		logger:   c.logger,
		dnsCache: c.dnsCache,
	}

	// Configure redirect policy
	c.client.CheckRedirect = c.checkRedirect

	return c
}

// dnsCacheManager handles DNS caching with DynamoDB backend
type dnsCacheManager struct {
	store  core.RepositoryStorage
	logger *zap.Logger
	mu     sync.RWMutex
	local  map[string]*storage.DNSCacheEntry // local cache for current Lambda invocation
}

// getCachedIPs retrieves cached IP addresses for a hostname
func (d *dnsCacheManager) getCachedIPs(ctx context.Context, hostname string) ([]net.IP, bool) {
	// Check local cache first
	d.mu.RLock()
	if entry, ok := d.local[hostname]; ok {
		d.mu.RUnlock()
		if time.Since(entry.ResolvedAt) < dnsCacheTTL {
			ips := make([]net.IP, len(entry.IPs))
			for i, ipStr := range entry.IPs {
				ips[i] = net.ParseIP(ipStr)
			}
			return ips, true
		}
	}
	d.mu.RUnlock()

	// Check DynamoDB if we have a store
	if d.store != nil {
		entry, err := d.store.User().GetDNSCache(ctx, hostname)
		if err == nil && entry != nil {
			// Check if cache is still valid
			if time.Since(entry.ResolvedAt) < dnsCacheTTL {
				// Update local cache
				d.mu.Lock()
				d.local[hostname] = entry
				d.mu.Unlock()

				ips := make([]net.IP, len(entry.IPs))
				for i, ipStr := range entry.IPs {
					ips[i] = net.ParseIP(ipStr)
				}
				return ips, true
			}
		}
	}

	return nil, false
}

// setCachedIPs stores IP addresses for a hostname
func (d *dnsCacheManager) setCachedIPs(ctx context.Context, hostname string, ips []net.IP) {
	entry := &storage.DNSCacheEntry{
		Hostname:   hostname,
		IPs:        make([]string, len(ips)),
		ResolvedAt: time.Now(),
		TTL:        int64(dnsCacheTTL.Seconds()),
	}

	for i, ip := range ips {
		entry.IPs[i] = ip.String()
	}

	// Update local cache
	d.mu.Lock()
	d.local[hostname] = entry
	d.mu.Unlock()

	// Store in DynamoDB if available
	if d.store != nil {
		if err := d.store.User().SetDNSCache(ctx, entry); err != nil {
			d.logger.Warn("failed to cache DNS lookup",
				zap.String("hostname", hostname),
				zap.Error(err))
		}
	}
}

// secureTransport wraps the default transport with security checks
type secureTransport struct {
	base     http.RoundTripper
	logger   *zap.Logger
	dnsCache *dnsCacheManager
}

// RoundTrip implements http.RoundTripper with security checks
func (t *secureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()

	// First validation - before DNS resolution
	if err := validateURL(req.URL, t.logger); err != nil {
		t.logger.Warn("blocked request",
			zap.String("url", req.URL.String()),
			zap.Error(err))
		return nil, fmt.Errorf("%w: %v", ErrBlockedRequest, err)
	}

	// Get hostname
	hostname := req.URL.Hostname()

	// Check DNS cache first
	var ips []net.IP
	var fromCache bool

	if t.dnsCache != nil {
		ips, fromCache = t.dnsCache.getCachedIPs(ctx, hostname)
	}

	// If not in cache, resolve hostname
	if !fromCache {
		var err error
		ips, err = net.LookupIP(hostname)
		if err != nil {
			return nil, fmt.Errorf("DNS lookup failed: %w", err)
		}

		// Cache the result
		if t.dnsCache != nil {
			t.dnsCache.setCachedIPs(ctx, hostname, ips)
		}
	}

	// Validate each resolved IP
	for _, ip := range ips {
		if isPrivateIP(ip) {
			t.logger.Warn("DNS rebinding attempt detected",
				zap.String("hostname", hostname),
				zap.String("resolved_ip", ip.String()),
				zap.String("url", req.URL.String()),
				zap.Bool("from_cache", fromCache))
			return nil, fmt.Errorf("request blocked: %w", ErrPrivateIPAddress)
		}
	}

	// Make the request
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	// Verify the connection actually went to one of our resolved IPs
	// This prevents TOCTOU attacks where DNS changes between check and use
	if resp.Request.URL.Hostname() != hostname {
		if err := resp.Body.Close(); err != nil {
			t.logger.Warn("failed to close response body after hostname change", zap.Error(err))
		}
		return nil, fmt.Errorf("hostname changed during request: %s -> %s",
			hostname, resp.Request.URL.Hostname())
	}

	return resp, nil
}

// checkRedirect validates redirects
func (c *SecureClient) checkRedirect(req *http.Request, via []*http.Request) error {
	// Check redirect count
	if len(via) >= c.maxRedirects {
		return fmt.Errorf("too many redirects (%d)", len(via))
	}

	// Validate the redirect URL
	if err := validateURL(req.URL, c.logger); err != nil {
		c.logger.Warn("blocked redirect",
			zap.String("url", req.URL.String()),
			zap.Error(err))
		return fmt.Errorf("%w: %v", ErrRedirectBlocked, err)
	}

	return nil
}

// validateURL checks if a URL is safe to request
func validateURL(u *url.URL, logger *zap.Logger) error {
	// Check scheme
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		if blockedSchemes[scheme] {
			return fmt.Errorf("%w: %s", ErrInvalidScheme, scheme)
		}
		return fmt.Errorf("%w: only http/https allowed, got %s", ErrInvalidScheme, scheme)
	}

	// Get hostname
	hostname := u.Hostname()
	if hostname == "" {
		return errors.New("empty hostname")
	}

	// Check for metadata endpoints (AWS, GCP, Azure)
	if isMetadataEndpoint(hostname) {
		return errors.New("metadata endpoints are blocked")
	}

	return nil
}

// isPrivateIP checks if an IP address is in a private range
func isPrivateIP(ip net.IP) bool {
	// Check against all private IP blocks
	for _, block := range privateIPBlocks {
		if block.Contains(ip) {
			return true
		}
	}

	// Additional checks for special addresses
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	return false
}

// isMetadataEndpoint checks for known cloud metadata endpoints
func isMetadataEndpoint(hostname string) bool {
	metadataEndpoints := []string{
		"169.254.169.254",          // AWS
		"metadata.google.internal", // GCP
		"metadata.azure.com",       // Azure
		"metadata",                 // Generic
	}

	hostname = strings.ToLower(hostname)
	for _, endpoint := range metadataEndpoints {
		if hostname == endpoint || strings.HasSuffix(hostname, "."+endpoint) {
			return true
		}
	}

	return false
}

// Do performs an HTTP request with security checks
func (c *SecureClient) Do(req *http.Request) (*http.Response, error) {
	return c.client.Do(req)
}

// Get performs a GET request with security checks
func (c *SecureClient) Get(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

// Post performs a POST request with security checks
func (c *SecureClient) Post(url string, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return c.Do(req)
}

// Head performs a HEAD request with security checks
func (c *SecureClient) Head(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodHead, url, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

// GetWithContext performs a GET request with context
func (c *SecureClient) GetWithContext(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

// PostWithContext performs a POST request with context
func (c *SecureClient) PostWithContext(ctx context.Context, url string, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return c.Do(req)
}

// DefaultClient returns a pre-configured secure client with sensible defaults
func DefaultClient() *SecureClient {
	return NewSecureClient()
}
