package ai

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/equaltoai/lesser/pkg/ssrf"
	"go.uber.org/zap"
)

func newSSRFProtectedHTTPClient(logger *zap.Logger) *http.Client {
	if logger == nil {
		logger = zap.NewNop()
	}

	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		base = &http.Transport{}
	}
	transport := base.Clone()
	// Disable ProxyFromEnvironment for SSRF-hardened clients. A proxy becomes the
	// dial target and can bypass destination-IP validation.
	transport.Proxy = nil

	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("invalid dial address: %w", err)
		}

		if ip := net.ParseIP(host); ip != nil {
			if ssrf.IsBlockedIP(ip) {
				logger.Warn("blocked dial to private IP", zap.String("ip", ip.String()))
				return nil, fmt.Errorf("%w: %s", ErrLocalNetworkAccess, ip.String())
			}
			return (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		}

		if ssrf.IsBlockedHostname(host) {
			logger.Warn("blocked dial to internal hostname", zap.String("host", host))
			return nil, fmt.Errorf("%w: %s", ErrLocalNetworkAccess, host)
		}

		ips, err := net.LookupIP(host)
		if err != nil {
			return nil, fmt.Errorf("DNS resolution failed: %w", err)
		}
		for _, ip := range ips {
			if ssrf.IsBlockedIP(ip) {
				logger.Warn("blocked dial to private IP",
					zap.String("host", host),
					zap.String("ip", ip.String()))
				return nil, fmt.Errorf("%w: %s", ErrLocalNetworkAccess, ip.String())
			}
		}

		var lastErr error
		for _, ip := range ips {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			conn, err := (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}

		if lastErr != nil {
			return nil, lastErr
		}
		return nil, fmt.Errorf("DNS resolution returned no IPs for %s", host)
	}

	return &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}
}
