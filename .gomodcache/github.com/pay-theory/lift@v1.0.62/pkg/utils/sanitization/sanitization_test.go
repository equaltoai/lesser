package sanitization

import (
	"strings"
	"testing"

	"github.com/pay-theory/lift/pkg/security"
)

func TestSanitizeFieldValue(t *testing.T) {
	s := Default()

	tests := []struct {
		value    any
		expected any
		name     string
		key      string
	}{
		// Allowed fields
		{
			name:     "allowed field card_bin",
			key:      "card_bin",
			value:    "411111",
			expected: "411111",
		},
		{
			name:     "allowed field card_brand",
			key:      "card_brand",
			value:    "Visa",
			expected: "Visa",
		},
		{
			name:     "allowed field card_type",
			key:      "card_type",
			value:    "credit",
			expected: "credit",
		},

		// Sensitive number fields (DataRestricted)
		{
			name:     "ssn with dashes",
			key:      "ssn",
			value:    "123-45-6789",
			expected: "*****6789",
		},
		{
			name:     "account_number",
			key:      "account_number",
			value:    "1234567890",
			expected: "******7890",
		},
		{
			name:     "ein with spaces",
			key:      "ein",
			value:    "12 345 6789",
			expected: "*****6789",
		},
		{
			name:     "short number",
			key:      "ssn",
			value:    "123",
			expected: redactedValue,
		},
		{
			name:     "non-string ssn",
			key:      "ssn",
			value:    12345,
			expected: redactedValue,
		},

		// Highly sensitive fields (DataConfidential)
		{
			name:     "password field",
			key:      "password",
			value:    "secret123",
			expected: redactedValue,
		},
		{
			name:     "api_token field",
			key:      "api_token",
			value:    "abcdef123456",
			expected: redactedValue,
		},
		{
			name:     "field containing auth",
			key:      "authorization_header",
			value:    "Bearer token123",
			expected: redactedValue,
		},
		{
			name:     "email field",
			key:      "email",
			value:    "user@example.com",
			expected: "user@example.com", // Email is classified as DataInternal by dataprotection
		},

		// Key fields (DataInternal - should not be redacted)
		{
			name:     "key field",
			key:      "key",
			value:    "some-key-value",
			expected: "some-key-value",
		},
		{
			name:     "api_key field",
			key:      "api_key",
			value:    "abc123xyz",
			expected: "abc123xyz",
		},
		{
			name:     "access_key field",
			key:      "access_key",
			value:    "AKIAIOSFODNN7EXAMPLE",
			expected: "AKIAIOSFODNN7EXAMPLE",
		},
		{
			name:     "public_key field",
			key:      "public_key",
			value:    "ssh-rsa AAAAB3NzaC1yc2E",
			expected: "ssh-rsa AAAAB3NzaC1yc2E",
		},
		{
			name:     "key_id field",
			key:      "key_id",
			value:    "key-12345",
			expected: "key-12345",
		},
		{
			name:     "my_key suffix pattern",
			key:      "my_key",
			value:    "custom-key-value",
			expected: "custom-key-value",
		},

		// Key fields that should be redacted (contain secret/private)
		{
			name:     "secret_key field",
			key:      "secret_key",
			value:    "wJalrXUtnFEMI/K7MDENG",
			expected: "[REDACTED]",
		},
		{
			name:     "private_key field",
			key:      "private_key",
			value:    "-----BEGIN PRIVATE KEY-----",
			expected: "[REDACTED]",
		},
		{
			name:     "cvv field",
			key:      "cvv",
			value:    "123",
			expected: redactedValue,
		},

		// User content fields (DataInternal)
		{
			name:     "request_body",
			key:      "request_body",
			value:    "some user content here",
			expected: "[USER_CONTENT_22_CHARS]",
		},
		{
			name:     "empty user content",
			key:      "body",
			value:    "",
			expected: "[USER_CONTENT]",
		},
		{
			name:     "non-string user content",
			key:      "body",
			value:    123,
			expected: 123,
		},
		{
			name:     "user comment",
			key:      "comment",
			value:    "This is a user comment",
			expected: "[USER_CONTENT_22_CHARS]",
		},

		// Error fields (DataPublic - not classified as sensitive by dataprotection)
		{
			name:     "short error message",
			key:      "error",
			value:    "file not found",
			expected: "file not found",
		},
		{
			name:     "long error message",
			key:      "error",
			value:    "this is a very long error message that might contain sensitive user data",
			expected: "this is a very long error message that might contain sensitive user data",
		},
		{
			name:     "error with input keyword",
			key:      "error",
			value:    "invalid input provided",
			expected: "invalid input provided",
		},
		{
			name:     "error_details field",
			key:      "error_details",
			value:    "invalid user input: email@example.com",
			expected: "invalid user input: email@example.com",
		},

		// Large strings (only sanitized if DataInternal)
		{
			name:     "large string",
			key:      "data",
			value:    strings.Repeat("a", 201),
			expected: strings.Repeat("a", 201), // DataPublic - not sanitized
		},
		{
			name:     "exactly 200 chars",
			key:      "data",
			value:    strings.Repeat("a", 200),
			expected: strings.Repeat("a", 200),
		},

		// Normal fields
		{
			name:     "normal field",
			key:      "status",
			value:    "active",
			expected: "active",
		},
		{
			name:     "numeric field",
			key:      "count",
			value:    42,
			expected: 42,
		},
		{
			name:     "boolean field",
			key:      "enabled",
			value:    true,
			expected: true,
		},

		// Case insensitive
		{
			name:     "uppercase PASSWORD",
			key:      "PASSWORD",
			value:    "secret",
			expected: redactedValue,
		},
		{
			name:     "mixed case Email",
			key:      "Email",
			value:    "test@example.com",
			expected: "test@example.com", // Email is DataInternal
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.SanitizeFieldValue(tt.key, tt.value)
			if result != tt.expected {
				t.Errorf("SanitizeFieldValue(%q, %v) = %v, want %v", tt.key, tt.value, result, tt.expected)
			}
		})
	}
}

func TestSanitizeHeaders(t *testing.T) {
	s := Default()

	tests := []struct {
		headers  map[string][]string
		expected map[string]string
		name     string
	}{
		{
			name: "sensitive headers",
			headers: map[string][]string{
				"Authorization": {"Bearer token123"},
				"Cookie":        {"session=abc123"},
				"X-API-Key":     {"secret-key"},
				"Content-Type":  {"application/json"},
			},
			expected: map[string]string{
				"Authorization": redactedValue,
				"Cookie":        redactedValue,
				"X-API-Key":     redactedValue,
				"Content-Type":  "application/json",
			},
		},
		{
			name: "case insensitive",
			headers: map[string][]string{
				"authorization": {"Bearer token"},
				"COOKIE":        {"session=xyz"},
				"x-auth-token":  {"auth123"},
			},
			expected: map[string]string{
				"authorization": redactedValue,
				"COOKIE":        redactedValue,
				"x-auth-token":  redactedValue,
			},
		},
		{
			name: "multiple values",
			headers: map[string][]string{
				"Accept": {"text/html", "application/json"},
				"Cookie": {"session=123", "tracking=456"},
			},
			expected: map[string]string{
				"Accept": "text/html",
				"Cookie": redactedValue,
			},
		},
		{
			name:     "empty headers",
			headers:  map[string][]string{},
			expected: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.SanitizeHeaders(tt.headers)
			if len(result) != len(tt.expected) {
				t.Errorf("SanitizeHeaders() returned %d headers, want %d", len(result), len(tt.expected))
			}
			for key, expectedValue := range tt.expected {
				if result[key] != expectedValue {
					t.Errorf("SanitizeHeaders()[%q] = %q, want %q", key, result[key], expectedValue)
				}
			}
		})
	}
}

func TestSanitizeQueryParams(t *testing.T) {
	s := Default()

	tests := []struct {
		params   map[string][]string
		expected map[string]string
		name     string
	}{
		{
			name: "sensitive params",
			params: map[string][]string{
				"token":    {"abc123"},
				"api_key":  {"secret"},
				"password": {"pass123"},
				"page":     {"1"},
			},
			expected: map[string]string{
				"token":    "[SANITIZED_QUERY_PARAMS]",
				"api_key":  "[SANITIZED_QUERY_PARAMS]",
				"password": "[SANITIZED_QUERY_PARAMS]",
				"page":     "1",
			},
		},
		{
			name: "params with sensitive values",
			params: map[string][]string{
				"filter": {"email=test@example.com"},
				"sort":   {"name"},
				"search": {"user search query"},
			},
			expected: map[string]string{
				"filter": "email=test@example.com",
				"sort":   "name",
				"search": "[USER_CONTENT_17_CHARS]",
			},
		},
		{
			name: "case insensitive",
			params: map[string][]string{
				"TOKEN":  {"abc"},
				"ApiKey": {"xyz"},
				"AUTH":   {"bearer"},
			},
			expected: map[string]string{
				"TOKEN":  "[SANITIZED_QUERY_PARAMS]",
				"ApiKey": "[SANITIZED_QUERY_PARAMS]",
				"AUTH":   "[SANITIZED_QUERY_PARAMS]",
			},
		},
		{
			name:     "empty params",
			params:   map[string][]string{},
			expected: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.SanitizeQueryParams(tt.params)
			if len(result) != len(tt.expected) {
				t.Errorf("SanitizeQueryParams() returned %d params, want %d", len(result), len(tt.expected))
			}
			for key, expectedValue := range tt.expected {
				if result[key] != expectedValue {
					t.Errorf("SanitizeQueryParams()[%q] = %q, want %q", key, result[key], expectedValue)
				}
			}
		})
	}
}

func TestSanitizeMap(t *testing.T) {
	s := Default()

	input := map[string]any{
		"username":    "john_doe",
		"password":    "secret123",
		"email":       "john@example.com",
		"age":         30,
		"card_bin":    "411111",
		"ssn":         "123-45-6789",
		"description": "This is a long user description that should be sanitized",
	}

	result := s.SanitizeMap(input)

	expected := map[string]any{
		"username":    "john_doe",
		"password":    redactedValue,
		"email":       "john@example.com", // Email is DataInternal, not redacted
		"age":         30,
		"card_bin":    "411111",
		"ssn":         "*****6789",
		"description": "[USER_CONTENT_56_CHARS]",
	}

	for key, expectedValue := range expected {
		if result[key] != expectedValue {
			t.Errorf("SanitizeMap()[%q] = %v, want %v", key, result[key], expectedValue)
		}
	}
}

func TestCustomDataProtectionManager(t *testing.T) {
	// Create a custom data protection config
	config := security.DataProtectionConfig{
		DefaultClassification: security.DataPublic,
		FieldClassifications: map[string]security.DataClassification{
			"custom_field":   security.DataPublic,
			"mysecret":       security.DataConfidential,
			"custom_number":  security.DataRestricted,
			"custom_content": security.DataInternal,
		},
		EncryptionKey: "test-key",
	}

	dpm, err := security.NewDataProtectionManager(config)
	if err != nil {
		t.Fatalf("Failed to create data protection manager: %v", err)
	}

	s := New(dpm)

	tests := []struct {
		value    any
		expected any
		name     string
		key      string
	}{
		{
			name:     "custom allowed field",
			key:      "custom_field",
			value:    "allowed value",
			expected: "allowed value",
		},
		{
			name:     "custom sensitive field",
			key:      "mysecret",
			value:    "secret value",
			expected: redactedValue,
		},
		{
			name:     "custom number field",
			key:      "custom_number",
			value:    "1234567890",
			expected: "******7890",
		},
		{
			name:     "custom user content",
			key:      "custom_content",
			value:    "user data",
			expected: "user data", // DataInternal without user content field name pattern shows original value
		},
		{
			name:     "large string",
			key:      "data",
			value:    strings.Repeat("x", 201),
			expected: strings.Repeat("x", 201), // DataPublic classification doesn't sanitize large strings
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.SanitizeFieldValue(tt.key, tt.value)
			if result != tt.expected {
				t.Errorf("SanitizeFieldValue(%q, %v) = %v, want %v", tt.key, tt.value, result, tt.expected)
			}
		})
	}
}

func TestGlobalFunctions(t *testing.T) {
	// Test global SanitizeFieldValue
	result := SanitizeFieldValue("password", "secret")
	if result != redactedValue {
		t.Errorf("Global SanitizeFieldValue() = %v, want [REDACTED]", result)
	}

	// Test global SanitizeHeaders
	headers := map[string][]string{
		"Authorization": {"Bearer token"},
	}
	headerResult := SanitizeHeaders(headers)
	if headerResult["Authorization"] != redactedValue {
		t.Errorf("Global SanitizeHeaders() = %v, want [REDACTED]", headerResult["Authorization"])
	}

	// Test global SanitizeQueryParams
	params := map[string][]string{
		"token": {"abc123"},
	}
	paramResult := SanitizeQueryParams(params)
	if paramResult["token"] != "[SANITIZED_QUERY_PARAMS]" {
		t.Errorf("Global SanitizeQueryParams() = %v, want [SANITIZED_QUERY_PARAMS]", paramResult["token"])
	}

	// Test global SanitizeMap
	data := map[string]any{
		"email": "test@example.com",
	}
	mapResult := SanitizeMap(data)
	if mapResult["email"] != "test@example.com" { // Email is DataInternal
		t.Errorf("Global SanitizeMap() = %v, want test@example.com", mapResult["email"])
	}
}
