package lift

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCleanDomain(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://example.com", "example.com"},
		{"http://example.com", "example.com"},
		{"example.com/path", "example.com"},
		{"EXAMPLE.COM", "example.com"},
		{"example.com:443", "example.com"},
		{"example.com:80", "example.com"},
		{"example.com:8080", "example.com:8080"},
		{"https://example.com:443/path?query=1", "example.com"},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("Clean_%s", test.input), func(t *testing.T) {
			result := cleanDomain(test.input)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestGetFieldOrDefault(t *testing.T) {
	testMap := map[string]any{
		"existing": "value",
		"number":   42,
		"boolean":  true,
	}

	tests := []struct {
		name         string
		key          string
		defaultVal   any
		expectedVal  any
	}{
		{"existing string", "existing", "default", "value"},
		{"existing number", "number", 0, 42},
		{"existing boolean", "boolean", false, true},
		{"missing string", "missing", "default", "default"},
		{"missing number", "missing", 999, 999},
		{"missing boolean", "missing", false, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := getFieldOrDefault(testMap, test.key, test.defaultVal)
			assert.Equal(t, test.expectedVal, result)
		})
	}
}

func TestAdminDomainBlockRequest_Validation(t *testing.T) {
	tests := []struct {
		name    string
		req     AdminDomainBlockRequest
		isValid bool
	}{
		{
			name: "valid suspend request",
			req: AdminDomainBlockRequest{
				Domain:   "bad.example.com",
				Severity: "suspend",
			},
			isValid: true,
		},
		{
			name: "valid silence request",
			req: AdminDomainBlockRequest{
				Domain:   "noisy.example.com",
				Severity: "silence",
			},
			isValid: true,
		},
		{
			name: "empty domain",
			req: AdminDomainBlockRequest{
				Domain:   "",
				Severity: "suspend",
			},
			isValid: false,
		},
		{
			name: "invalid severity",
			req: AdminDomainBlockRequest{
				Domain:   "bad.example.com",
				Severity: "invalid",
			},
			isValid: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Test domain validation
			domainValid := test.req.Domain != ""
			
			// Test severity validation
			severityValid := test.req.Severity == "" || test.req.Severity == "silence" || test.req.Severity == "suspend"
			
			isValid := domainValid && severityValid
			assert.Equal(t, test.isValid, isValid)
		})
	}
}

func TestAdminDomainBlockResponse_Structure(t *testing.T) {
	// Test that the response struct has all required fields
	resp := AdminDomainBlockResponse{}
	
	// Use reflection or simple assignment to verify fields exist
	resp.ID = "test-id"
	resp.Domain = "example.com"
	resp.Severity = "suspend"
	resp.RejectMedia = true
	resp.RejectReports = false
	resp.PrivateComment = "admin note"
	resp.PublicComment = "spam domain"
	resp.Obfuscate = false
	
	assert.Equal(t, "test-id", resp.ID)
	assert.Equal(t, "example.com", resp.Domain)
	assert.Equal(t, "suspend", resp.Severity)
	assert.True(t, resp.RejectMedia)
	assert.False(t, resp.RejectReports)
	assert.Equal(t, "admin note", resp.PrivateComment)
	assert.Equal(t, "spam domain", resp.PublicComment)
	assert.False(t, resp.Obfuscate)
}

func TestInstanceInfoResponse_Structure(t *testing.T) {
	// Test that the response struct has all required fields
	resp := InstanceInfoResponse{}
	
	resp.Domain = "mastodon.social"
	resp.Software = "mastodon"
	resp.Version = "4.0.0"
	resp.ActiveUsers = 1000
	resp.TotalMessages = 50000
	resp.TrustScore = 0.95
	resp.IsSilenced = false
	resp.IsSuspended = false
	
	assert.Equal(t, "mastodon.social", resp.Domain)
	assert.Equal(t, "mastodon", resp.Software)
	assert.Equal(t, "4.0.0", resp.Version)
	assert.Equal(t, 1000, resp.ActiveUsers)
	assert.Equal(t, int64(50000), resp.TotalMessages)
	assert.Equal(t, 0.95, resp.TrustScore)
	assert.False(t, resp.IsSilenced)
	assert.False(t, resp.IsSuspended)
}

func TestEmailDomainBlockResponse_Structure(t *testing.T) {
	// Test that the response struct has all required fields
	resp := EmailDomainBlockResponse{}
	
	resp.ID = "email-block-1"
	resp.Domain = "spam.com"
	
	assert.Equal(t, "email-block-1", resp.ID)
	assert.Equal(t, "spam.com", resp.Domain)
}

// Test domain cleaning edge cases
func TestCleanDomainEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string", "", ""},
		{"just protocol", "https://", ""},
		{"with www", "https://www.example.com", "www.example.com"},
		{"with subdomain", "api.example.com", "api.example.com"},
		{"with path and query", "example.com/api/v1?param=value", "example.com"},
		{"mixed case", "ExAmPlE.CoM", "example.com"},
		{"trailing slash", "example.com/", "example.com"},
		{"multiple slashes", "example.com///path", "example.com"},
		{"non-standard port", "example.com:3000", "example.com:3000"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := cleanDomain(test.input)
			assert.Equal(t, test.expected, result)
		})
	}
}