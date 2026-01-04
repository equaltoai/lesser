package ai

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

var aiBlockedIPv4Nets = []net.IPNet{
	{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}, // 100.64.0.0/10 (RFC 6598: shared address space)
}

func newSSRFProtectedHTTPClient(logger *zap.Logger) *http.Client {
	if logger == nil {
		logger = zap.NewNop()
	}

	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		base = &http.Transport{}
	}
	transport := base.Clone()

	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("invalid dial address: %w", err)
		}

		if isAIMetadataEndpoint(host) {
			logger.Warn("blocked dial to metadata endpoint", zap.String("host", host))
			return nil, fmt.Errorf("%w: %s", ErrLocalNetworkAccess, host)
		}

		if ip := net.ParseIP(host); ip != nil {
			if isAIBlockedIP(ip) {
				logger.Warn("blocked dial to private IP", zap.String("ip", ip.String()))
				return nil, fmt.Errorf("%w: %s", ErrLocalNetworkAccess, ip.String())
			}
			return (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		}

		ips, err := net.LookupIP(host)
		if err != nil {
			return nil, fmt.Errorf("DNS resolution failed: %w", err)
		}
		for _, ip := range ips {
			if isAIBlockedIP(ip) {
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

func isAIBlockedIP(ip net.IP) bool {
	for _, block := range aiBlockedIPv4Nets {
		if block.Contains(ip) {
			return true
		}
	}

	for _, metadataIP := range []string{
		"169.254.169.254", // AWS/Azure/GCP metadata
		"169.254.170.2",   // AWS ECS metadata
		"100.100.100.200", // Alibaba metadata
	} {
		if ip.Equal(net.ParseIP(metadataIP)) {
			return true
		}
	}

	return ip.IsPrivate() ||
		ip.IsLoopback() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified()
}

func isAIMetadataEndpoint(hostname string) bool {
	hostname = strings.ToLower(hostname)

	for _, endpoint := range []string{
		"169.254.169.254",
		"metadata.google.internal",
		"metadata.azure.com",
		"metadata",
	} {
		if hostname == endpoint || strings.HasSuffix(hostname, "."+endpoint) {
			return true
		}
	}

	return false
}
