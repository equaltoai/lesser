// Package ssrf provides SSRF protection utilities.
package ssrf

import (
	"net"
	"strings"
)

var blockedIPRanges = []*net.IPNet{
	mustParseCIDR("10.0.0.0/8"),
	mustParseCIDR("100.64.0.0/10"),  // RFC 6598: shared address space (includes Alibaba metadata)
	mustParseCIDR("172.16.0.0/12"),  // RFC 1918
	mustParseCIDR("192.168.0.0/16"), // RFC 1918
	mustParseCIDR("127.0.0.0/8"),    // loopback
	mustParseCIDR("169.254.0.0/16"), // link-local (includes cloud metadata)
	mustParseCIDR("0.0.0.0/8"),      // "this host on this network"
	mustParseCIDR("::1/128"),        // loopback
	mustParseCIDR("fc00::/7"),       // unique local
	mustParseCIDR("fe80::/10"),      // link-local
	mustParseCIDR("fec0::/10"),      // site-local (deprecated)
	mustParseCIDR("::/128"),         // unspecified
}

var blockedHostSuffixes = []string{
	"localhost",
	"metadata.google.internal",
	"metadata.azure.com",
	"metadata",
	"instance-data",
	"consul",
	"vault",
}

func mustParseCIDR(raw string) *net.IPNet {
	_, block, err := net.ParseCIDR(raw)
	if err != nil {
		panic("ssrf: invalid CIDR " + raw)
	}
	return block
}

// IsBlockedIP reports whether the IP should be blocked for SSRF protection.
func IsBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}

	for _, block := range blockedIPRanges {
		if block.Contains(ip) {
			return true
		}
	}

	return ip.IsPrivate() ||
		ip.IsLoopback() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified()
}

// IsBlockedHostname reports whether a hostname should be blocked for SSRF protection.
//
// It blocks:
//   - IP literals that are blocked by IsBlockedIP
//   - known internal/metadata hostnames and their subdomains (e.g. *.metadata.azure.com)
func IsBlockedHostname(hostname string) bool {
	host := strings.ToLower(strings.TrimSpace(hostname))
	if host == "" {
		return true
	}

	host = strings.TrimSuffix(host, ".")

	if ip := net.ParseIP(host); ip != nil {
		return IsBlockedIP(ip)
	}

	for _, suffix := range blockedHostSuffixes {
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return true
		}
	}

	return false
}
