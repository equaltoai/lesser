package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateRequestOriginDomain_BindsToInstanceDomain verifies that
// forwarded-host origin validation correctly binds the reconstructed
// origin to the expected instance domain. (CSR-043 regression probe)
func TestValidateRequestOriginDomain_BindsToInstanceDomain(t *testing.T) {
	tests := []struct {
		name        string
		headers     map[string][]string
		localDomain string
		wantErr     bool
		errContains string
	}{
		{
			name: "lesser forwarded host matches",
			headers: map[string][]string{
				"x-lesser-forwarded-host":  {"lesser.example"},
				"x-lesser-forwarded-proto": {"https"},
			},
			localDomain: "lesser.example",
			wantErr:     false,
		},
		{
			name: "forwarded header host matches",
			headers: map[string][]string{
				"forwarded": {"for=198.51.100.22;proto=https;host=lesser.example"},
			},
			localDomain: "lesser.example",
			wantErr:     false,
		},
		{
			name: "x-forwarded-host matches",
			headers: map[string][]string{
				"x-forwarded-host":  {"lesser.example"},
				"x-forwarded-proto": {"https"},
			},
			localDomain: "lesser.example",
			wantErr:     false,
		},
		{
			name: "mismatched domain is rejected",
			headers: map[string][]string{
				"x-lesser-forwarded-host":  {"evil.example"},
				"x-lesser-forwarded-proto": {"https"},
			},
			localDomain: "lesser.example",
			wantErr:     true,
			errContains: "does not match instance host",
		},
		{
			name: "empty local domain skips validation",
			headers: map[string][]string{
				"x-lesser-forwarded-host":  {"evil.example"},
				"x-lesser-forwarded-proto": {"https"},
			},
			localDomain: "",
			wantErr:     false,
		},
		{
			name: "spoiler forwarded host with userinfo is ignored",
			headers: map[string][]string{
				"x-lesser-forwarded-host": {"evil@lesser.example"},
			},
			localDomain: "lesser.example",
			wantErr:     true, // normalizeForwardedHost rejects userinfo, so no origin
		},
		{
			name: "Host header matches (no forwarded headers)",
			headers: map[string][]string{
				"host": {"lesser.example"},
			},
			localDomain: "lesser.example",
			wantErr:     false,
		},
		{
			name: "Host header mismatches (no forwarded headers)",
			headers: map[string][]string{
				"host": {"internal.execute-api.us-east-1.amazonaws.com"},
			},
			localDomain: "lesser.example",
			wantErr:     true,
			errContains: "does not match instance host",
		},
		{
			name: "Host header with port matches (no forwarded headers)",
			headers: map[string][]string{
				"host": {"lesser.example:8443"},
			},
			localDomain: "lesser.example",
			wantErr:     false,
		},
		{
			name:        "no host determinable (neither forwarded nor Host headers)",
			headers:     map[string][]string{},
			localDomain: "lesser.example",
			wantErr:     true,
			errContains: "no origin host determinable",
		},
		{
			name: "case-insensitive domain comparison",
			headers: map[string][]string{
				"x-lesser-forwarded-host":  {"Lesser.Example"},
				"x-lesser-forwarded-proto": {"https"},
			},
			localDomain: "lesser.example",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRequestOriginDomain(tt.headers, tt.localDomain)
			if tt.wantErr {
				require.Error(t, err, "expected error for %s", tt.name)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err, "unexpected error for %s: %v", tt.name, err)
			}
		})
	}
}

// TestValidateRequestOriginDomain_RejectsMalformedHosts verifies that
// malformed forwarded host values are rejected rather than trusted.
// (CSR-043 regression probe)
func TestValidateRequestOriginDomain_RejectsMalformedHosts(t *testing.T) {
	malformedTests := []struct {
		name    string
		headers map[string][]string
	}{
		{
			name: "host with path",
			headers: map[string][]string{
				"x-lesser-forwarded-host":  {"lesser.example/evil"},
				"x-lesser-forwarded-proto": {"https"},
			},
		},
		{
			name: "host with query",
			headers: map[string][]string{
				"x-lesser-forwarded-host":  {"lesser.example?evil=true"},
				"x-lesser-forwarded-proto": {"https"},
			},
		},
		{
			name: "host with fragment",
			headers: map[string][]string{
				"x-lesser-forwarded-host":  {"lesser.example#evil"},
				"x-lesser-forwarded-proto": {"https"},
			},
		},
		{
			name: "host with control characters",
			headers: map[string][]string{
				"x-lesser-forwarded-host":  {"lesser.example\r\nEvil: header"},
				"x-lesser-forwarded-proto": {"https"},
			},
		},
		{
			name: "host with spaces",
			headers: map[string][]string{
				"x-lesser-forwarded-host":  {"lesser.example evil.example"},
				"x-lesser-forwarded-proto": {"https"},
			},
		},
	}

	for _, tt := range malformedTests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRequestOriginDomain(tt.headers, "lesser.example")
			require.Error(t, err, "malformed forwarded host %q must be rejected", tt.name)
		})
	}
}
