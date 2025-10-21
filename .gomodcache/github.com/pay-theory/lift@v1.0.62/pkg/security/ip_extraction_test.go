package security

import (
	"testing"
)

func TestExtractClientIP(t *testing.T) {
	tests := []struct {
		name           string
		headers        map[string]string
		requestContext map[string]any
		expectedIP     string
		expectError    bool
	}{
		{
			name: "X-Forwarded-For with single IP",
			headers: map[string]string{
				"X-Forwarded-For": "192.168.1.100",
			},
			expectedIP: "192.168.1.100",
		},
		{
			name: "X-Forwarded-For with multiple IPs",
			headers: map[string]string{
				"X-Forwarded-For": "192.168.1.100, 10.0.0.1, 172.16.0.1",
			},
			expectedIP: "192.168.1.100",
		},
		{
			name: "X-Forwarded-For with spaces",
			headers: map[string]string{
				"X-Forwarded-For": "  192.168.1.100  ,  10.0.0.1  ",
			},
			expectedIP: "192.168.1.100",
		},
		{
			name: "X-Real-IP header",
			headers: map[string]string{
				"X-Real-IP": "192.168.1.200",
			},
			expectedIP: "192.168.1.200",
		},
		{
			name: "CF-Connecting-IP header (Cloudflare)",
			headers: map[string]string{
				"CF-Connecting-IP": "192.168.1.150",
			},
			expectedIP: "192.168.1.150",
		},
		{
			name: "X-Original-Forwarded-For header",
			headers: map[string]string{
				"X-Original-Forwarded-For": "192.168.1.175, 10.0.0.1",
			},
			expectedIP: "192.168.1.175",
		},
		{
			name: "Priority order - X-Forwarded-For wins",
			headers: map[string]string{
				"X-Forwarded-For":  "192.168.1.100",
				"X-Real-IP":        "192.168.1.200",
				"CF-Connecting-IP": "192.168.1.150",
			},
			expectedIP: "192.168.1.100",
		},
		{
			name: "API Gateway v2 request context",
			requestContext: map[string]any{
				"http": map[string]any{
					"sourceIp": "192.168.1.50",
				},
			},
			expectedIP: "192.168.1.50",
		},
		{
			name: "API Gateway v1 request context",
			requestContext: map[string]any{
				"identity": map[string]any{
					"sourceIp": "192.168.1.60",
				},
			},
			expectedIP: "192.168.1.60",
		},
		{
			name: "Direct sourceIp in request context",
			requestContext: map[string]any{
				"sourceIp": "192.168.1.70",
			},
			expectedIP: "192.168.1.70",
		},
		{
			name: "IPv6 address",
			headers: map[string]string{
				"X-Forwarded-For": "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
			},
			expectedIP: "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
		},
		{
			name: "IPv6 address simplified",
			headers: map[string]string{
				"X-Real-IP": "2001:db8:85a3::8a2e:370:7334",
			},
			expectedIP: "2001:db8:85a3::8a2e:370:7334",
		},
		{
			name: "IPv4 with port",
			headers: map[string]string{
				"X-Forwarded-For": "192.168.1.100:8080",
			},
			expectedIP: "192.168.1.100",
		},
		{
			name: "Invalid IP in X-Forwarded-For, fallback to X-Real-IP",
			headers: map[string]string{
				"X-Forwarded-For": "not-an-ip",
				"X-Real-IP":       "192.168.1.200",
			},
			expectedIP: "192.168.1.200",
		},
		{
			name:        "Empty headers and context",
			headers:     map[string]string{},
			expectError: true,
		},
		{
			name: "All invalid IPs",
			headers: map[string]string{
				"X-Forwarded-For": "invalid-ip",
				"X-Real-IP":       "not.an.ip",
			},
			expectError: true,
		},
		{
			name: "Empty string in headers",
			headers: map[string]string{
				"X-Forwarded-For": "",
				"X-Real-IP":       "",
			},
			expectError: true,
		},
		{
			name:           "Nil request context",
			headers:        map[string]string{},
			requestContext: nil,
			expectError:    true,
		},
		{
			name: "Real-world example - behind multiple proxies",
			headers: map[string]string{
				"X-Forwarded-For": "74.129.178.19, 172.31.255.255, 10.0.0.1",
			},
			expectedIP: "74.129.178.19",
		},
		{
			name: "Local IP addresses",
			headers: map[string]string{
				"X-Forwarded-For": "127.0.0.1",
			},
			expectedIP: "127.0.0.1",
		},
		{
			name: "Private IP ranges",
			headers: map[string]string{
				"X-Real-IP": "10.0.0.1",
			},
			expectedIP: "10.0.0.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip, err := ExtractClientIP(tt.headers, tt.requestContext)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none, IP: %s", ip)
				} else {
					// Verify error type
					if _, ok := err.(*IPExtractionError); !ok {
						t.Errorf("expected IPExtractionError but got %T", err)
					}
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if ip != tt.expectedIP {
					t.Errorf("expected IP %s, got %s", tt.expectedIP, ip)
				}
			}
		})
	}
}

func TestIsValidIP(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		{"Valid IPv4", "192.168.1.1", true},
		{"Valid IPv4 with port", "192.168.1.1:8080", true},
		{"Valid IPv6", "2001:db8::1", true},
		{"Valid IPv6 full", "2001:0db8:85a3:0000:0000:8a2e:0370:7334", true},
		{"Invalid IP", "not.an.ip", false},
		{"Empty string", "", false},
		{"Domain name", "example.com", false},
		{"Malformed IPv4", "192.168.1", false},
		{"Out of range IPv4", "256.256.256.256", false},
		{"Local IPv4", "127.0.0.1", true},
		{"Private IPv4", "10.0.0.1", true},
		{"IPv4 with invalid port", "192.168.1.1:99999", true}, // Still valid IP
		{"IPv6 localhost", "::1", true},
		{"IPv6 with zone", "fe80::1%eth0", false}, // Zone IDs not supported by net.ParseIP
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidIP(tt.ip)
			if result != tt.expected {
				t.Errorf("isValidIP(%s) = %v, expected %v", tt.ip, result, tt.expected)
			}
		})
	}
}

func TestIPExtractionError(t *testing.T) {
	err := &IPExtractionError{
		Message: "test error",
		Headers: map[string]string{
			"X-Forwarded-For": "invalid",
			"X-Real-IP":       "",
		},
	}

	expectedMsg := "failed to extract client IP: test error"
	if err.Error() != expectedMsg {
		t.Errorf("expected error message %q, got %q", expectedMsg, err.Error())
	}
}
