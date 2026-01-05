package ssrf

import (
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsBlockedIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		{"private_ipv4_10", "10.0.0.1", true},
		{"private_ipv4_shared_100_64", "100.64.0.1", true},
		{"metadata_ipv4_alibaba", "100.100.100.200", true},
		{"private_ipv4_172_16", "172.16.0.1", true},
		{"public_ipv4_172_0", "172.0.0.1", false},
		{"private_ipv4_192_168", "192.168.1.1", true},
		{"loopback_ipv4", "127.0.0.1", true},
		{"link_local_ipv4", "169.254.1.1", true},
		{"unspecified_ipv4", "0.0.0.0", true},
		{"reserved_ipv4_0_1", "0.0.0.1", true},
		{"public_ipv4", "8.8.8.8", false},
		{"loopback_ipv6", "::1", true},
		{"unique_local_ipv6", "fc00::1", true},
		{"link_local_ipv6", "fe80::1", true},
		{"site_local_ipv6", "fec0::1", true},
		{"unspecified_ipv6", "::", true},
		{"public_ipv6", "2001:4860:4860::8888", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ip := net.ParseIP(tt.ip)
			require.NotNil(t, ip)
			assert.Equal(t, tt.expected, IsBlockedIP(ip))
		})
	}
}

func TestIsBlockedHostname(t *testing.T) {
	t.Parallel()

	tests := []struct {
		hostname string
		expected bool
		testName string
	}{
		{"", true, "empty"},
		{"localhost", true, "localhost_exact"},
		{"api.localhost", true, "localhost_subdomain"},
		{"metadata.google.internal", true, "gcp_metadata"},
		{"metadata.azure.com", true, "azure_metadata"},
		{"subdomain.metadata.azure.com", true, "azure_metadata_subdomain"},
		{"metadata", true, "generic_metadata"},
		{"instance-data", true, "instance_data"},
		{"consul", true, "consul"},
		{"vault", true, "vault"},
		{"169.254.169.254", true, "metadata_ip_literal"},
		{"8.8.8.8", false, "public_ip_literal"},
		{"example.com", false, "public_hostname"},
		{"metadata-example.com", false, "substring_not_suffix"},
		{"metadata.google.internal.", true, "trailing_dot"},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, IsBlockedHostname(tt.hostname))
		})
	}
}

func TestValidateURLString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		raw       string
		wantErrIs error
		wantOK    bool
	}{
		{name: "valid_https", raw: "https://example.com/users/alice", wantOK: true},
		{name: "valid_http", raw: "http://example.com/", wantOK: true},
		{name: "invalid_scheme", raw: "ftp://example.com/", wantErrIs: ErrInvalidScheme},
		{name: "empty_hostname", raw: "https:///path", wantErrIs: ErrEmptyHostname},
		{name: "blocked_hostname", raw: "http://localhost:8080/", wantErrIs: ErrBlockedHostname},
		{name: "parse_error", raw: "https://example.com/%zz/", wantOK: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			parsed, err := ValidateURLString(tt.raw)
			if tt.wantOK {
				require.NoError(t, err)
				require.NotNil(t, parsed)
				return
			}

			require.Error(t, err)
			if tt.wantErrIs != nil {
				require.True(t, errors.Is(err, tt.wantErrIs), "expected errors.Is(%v, %v)", err, tt.wantErrIs)
			}
		})
	}
}
