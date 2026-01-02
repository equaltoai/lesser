package moderation

import (
	"testing"
)

func TestEnhancedURLMatcher(t *testing.T) {
	matcher := NewEnhancedURLMatcher()

	tests := []struct {
		name        string
		pattern     string
		patternType URLPatternType
		testURL     string
		shouldMatch bool
		description string
	}{
		{
			name:        "exact_match",
			pattern:     "https://example.com",
			patternType: URLPatternExact,
			testURL:     "https://example.com",
			shouldMatch: true,
			description: "Exact URL match should work",
		},
		{
			name:        "exact_no_match",
			pattern:     "https://example.com",
			patternType: URLPatternExact,
			testURL:     "https://other.com",
			shouldMatch: false,
			description: "Exact URL should not match different URL",
		},
		{
			name:        "domain_match",
			pattern:     "example.com",
			patternType: URLPatternDomain,
			testURL:     "https://sub.example.com/path",
			shouldMatch: true,
			description: "Domain pattern should match subdomains",
		},
		{
			name:        "subdomain_wildcard",
			pattern:     "*.example.com",
			patternType: URLPatternSubdomain,
			testURL:     "https://api.example.com",
			shouldMatch: true,
			description: "Wildcard subdomain should match",
		},
		{
			name:        "path_match",
			pattern:     "/api/*",
			patternType: URLPatternPath,
			testURL:     "https://example.com/api/users",
			shouldMatch: true,
			description: "Path wildcard should match",
		},
		{
			name:        "query_match",
			pattern:     "param=test",
			patternType: URLPatternQuery,
			testURL:     "https://example.com?param=test&other=value",
			shouldMatch: true,
			description: "Query parameter should match",
		},
		{
			name:        "regex_match",
			pattern:     `https?://.*\.evil\.com`,
			patternType: URLPatternRegex,
			testURL:     "https://sub.evil.com",
			shouldMatch: true,
			description: "Regex pattern should match",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Compile pattern
			err := matcher.CompileURLPattern(tt.pattern, tt.patternType)
			if err != nil {
				t.Fatalf("Failed to compile pattern: %v", err)
			}

			// Test matching
			matched, matchedPattern, err := matcher.MatchURL(tt.testURL, []string{tt.pattern})
			if err != nil {
				t.Fatalf("Failed to match URL: %v", err)
			}

			if matched != tt.shouldMatch {
				t.Errorf("Expected match=%v, got=%v for %s", tt.shouldMatch, matched, tt.description)
			}

			if matched && matchedPattern != tt.pattern {
				t.Errorf("Expected matched pattern=%s, got=%s", tt.pattern, matchedPattern)
			}
		})
	}
}

func TestEnhancedURLMatcher_DomainPatternWithSchemeAndPort(t *testing.T) {
	matcher := NewEnhancedURLMatcher()

	withScheme := "https://example.com"
	if err := matcher.CompileURLPattern(withScheme, URLPatternDomain); err != nil {
		t.Fatalf("failed to compile domain pattern: %v", err)
	}

	compiled, ok := matcher.compiled[withScheme]
	if !ok {
		t.Fatalf("expected compiled pattern to be stored")
	}
	if compiled.scheme != "https" {
		t.Fatalf("expected scheme=https, got %q", compiled.scheme)
	}

	withPort := "example.com:8443"
	if err := matcher.CompileURLPattern(withPort, URLPatternDomain); err != nil {
		t.Fatalf("failed to compile domain pattern: %v", err)
	}
	compiled, ok = matcher.compiled[withPort]
	if !ok {
		t.Fatalf("expected compiled pattern to be stored")
	}
	if compiled.port != "8443" {
		t.Fatalf("expected port=8443, got %q", compiled.port)
	}

	matched, matchedPattern, err := matcher.MatchURL("https://sub.example.com:8443/path", []string{withPort})
	if err != nil {
		t.Fatalf("failed to match URL: %v", err)
	}
	if !matched || matchedPattern != withPort {
		t.Fatalf("expected match for %s, got match=%v pattern=%s", withPort, matched, matchedPattern)
	}
}

func TestEnhancedIPMatcher(t *testing.T) {
	matcher := NewEnhancedIPMatcher()

	tests := []struct {
		name        string
		pattern     string
		patternType IPPatternType
		testIP      string
		shouldMatch bool
		description string
	}{
		{
			name:        "single_ipv4",
			pattern:     "192.168.1.1",
			patternType: IPPatternSingle,
			testIP:      "192.168.1.1",
			shouldMatch: true,
			description: "Single IPv4 should match exactly",
		},
		{
			name:        "single_ipv4_no_match",
			pattern:     "192.168.1.1",
			patternType: IPPatternSingle,
			testIP:      "192.168.1.2",
			shouldMatch: false,
			description: "Single IPv4 should not match different IP",
		},
		{
			name:        "cidr_block",
			pattern:     "192.168.1.0/24",
			patternType: IPPatternCIDR,
			testIP:      "192.168.1.100",
			shouldMatch: true,
			description: "CIDR block should match IPs in range",
		},
		{
			name:        "cidr_block_no_match",
			pattern:     "192.168.1.0/24",
			patternType: IPPatternCIDR,
			testIP:      "192.168.2.1",
			shouldMatch: false,
			description: "CIDR block should not match IPs outside range",
		},
		{
			name:        "ip_range",
			pattern:     "192.168.1.1-192.168.1.10",
			patternType: IPPatternRange,
			testIP:      "192.168.1.5",
			shouldMatch: true,
			description: "IP range should match IPs within range",
		},
		{
			name:        "ip_range_no_match",
			pattern:     "192.168.1.1-192.168.1.10",
			patternType: IPPatternRange,
			testIP:      "192.168.1.20",
			shouldMatch: false,
			description: "IP range should not match IPs outside range",
		},
		{
			name:        "ipv6_single",
			pattern:     "2001:db8::1",
			patternType: IPPatternSingle,
			testIP:      "2001:db8::1",
			shouldMatch: true,
			description: "Single IPv6 should match exactly",
		},
		{
			name:        "ipv6_cidr",
			pattern:     "2001:db8::/32",
			patternType: IPPatternCIDR,
			testIP:      "2001:db8::100",
			shouldMatch: true,
			description: "IPv6 CIDR should match IPs in range",
		},
		{
			name:        "regex_ipv4",
			pattern:     `^192\.168\.[0-9]+\.[0-9]+$`,
			patternType: IPPatternRegex,
			testIP:      "192.168.1.5",
			shouldMatch: true,
			description: "Regex IP pattern should match",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Compile pattern
			err := matcher.CompileIPPattern(tt.pattern, tt.patternType)
			if err != nil {
				t.Fatalf("Failed to compile pattern: %v", err)
			}

			// Test matching
			matched, matchedPattern, err := matcher.MatchIP(tt.testIP, []string{tt.pattern})
			if err != nil {
				t.Fatalf("Failed to match IP: %v", err)
			}

			if matched != tt.shouldMatch {
				t.Errorf("Expected match=%v, got=%v for %s", tt.shouldMatch, matched, tt.description)
			}

			if matched && matchedPattern != tt.pattern {
				t.Errorf("Expected matched pattern=%s, got=%s", tt.pattern, matchedPattern)
			}
		})
	}
}

func TestURLValidation(t *testing.T) {
	tests := []struct {
		name        string
		pattern     string
		patternType URLPatternType
		shouldError bool
		description string
	}{
		{
			name:        "valid_exact",
			pattern:     "https://example.com",
			patternType: URLPatternExact,
			shouldError: false,
			description: "Valid exact URL should pass validation",
		},
		{
			name:        "valid_domain",
			pattern:     "example.com",
			patternType: URLPatternDomain,
			shouldError: false,
			description: "Valid domain should pass validation",
		},
		{
			name:        "valid_domain_with_scheme_and_port",
			pattern:     "https://example.com:8443",
			patternType: URLPatternDomain,
			shouldError: false,
			description: "Domain patterns may include scheme and port",
		},
		{
			name:        "valid_subdomain",
			pattern:     "*.example.com",
			patternType: URLPatternSubdomain,
			shouldError: false,
			description: "Valid subdomain pattern should pass validation",
		},
		{
			name:        "valid_query",
			pattern:     "?param=test",
			patternType: URLPatternQuery,
			shouldError: false,
			description: "Valid query pattern should pass validation",
		},
		{
			name:        "invalid_query_spaces",
			pattern:     "param=bad value",
			patternType: URLPatternQuery,
			shouldError: true,
			description: "Query patterns may not include spaces",
		},
		{
			name:        "invalid_domain_character",
			pattern:     "example!.com",
			patternType: URLPatternDomain,
			shouldError: true,
			description: "Domains may not include punctuation",
		},
		{
			name:        "invalid_domain_hyphen",
			pattern:     "bad-.com",
			patternType: URLPatternDomain,
			shouldError: true,
			description: "Domain parts may not start or end with hyphen",
		},
		{
			name:        "invalid_domain_port",
			pattern:     "example.com:80:90",
			patternType: URLPatternDomain,
			shouldError: true,
			description: "Invalid host:port format should be rejected",
		},
		{
			name:        "invalid_path_missing_slash",
			pattern:     "api/*",
			patternType: URLPatternPath,
			shouldError: true,
			description: "Path patterns must start with /",
		},
		{
			name:        "invalid_path_traversal",
			pattern:     "/../etc",
			patternType: URLPatternPath,
			shouldError: true,
			description: "Path traversal should be rejected",
		},
		{
			name:        "invalid_domain",
			pattern:     "invalid..domain",
			patternType: URLPatternDomain,
			shouldError: true,
			description: "Invalid domain should fail validation",
		},
		{
			name:        "empty_pattern",
			pattern:     "",
			patternType: URLPatternExact,
			shouldError: true,
			description: "Empty pattern should fail validation",
		},
		{
			name:        "too_long_pattern",
			pattern:     string(make([]byte, 3000)),
			patternType: URLPatternExact,
			shouldError: true,
			description: "Too long pattern should fail validation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateURLPattern(tt.pattern, tt.patternType)
			hasError := err != nil

			if hasError != tt.shouldError {
				t.Errorf("Expected error=%v, got error=%v for %s. Error: %v",
					tt.shouldError, hasError, tt.description, err)
			}
		})
	}
}

func TestIPValidation(t *testing.T) {
	tests := []struct {
		name        string
		pattern     string
		patternType IPPatternType
		shouldError bool
		description string
	}{
		{
			name:        "valid_ipv4",
			pattern:     "192.168.1.1",
			patternType: IPPatternSingle,
			shouldError: false,
			description: "Valid IPv4 should pass validation",
		},
		{
			name:        "valid_ipv6",
			pattern:     "2001:db8::1",
			patternType: IPPatternSingle,
			shouldError: false,
			description: "Valid IPv6 should pass validation",
		},
		{
			name:        "valid_cidr",
			pattern:     "192.168.1.0/24",
			patternType: IPPatternCIDR,
			shouldError: false,
			description: "Valid CIDR should pass validation",
		},
		{
			name:        "valid_range",
			pattern:     "192.168.1.1-192.168.1.10",
			patternType: IPPatternRange,
			shouldError: false,
			description: "Valid IP range should pass validation",
		},
		{
			name:        "valid_regex",
			pattern:     `^192\.168\.[0-9]+\.[0-9]+$`,
			patternType: IPPatternRegex,
			shouldError: false,
			description: "Valid IP regex should pass validation",
		},
		{
			name:        "invalid_ipv4",
			pattern:     "256.256.256.256",
			patternType: IPPatternSingle,
			shouldError: true,
			description: "Invalid IPv4 should fail validation",
		},
		{
			name:        "invalid_cidr",
			pattern:     "192.168.1.0/33",
			patternType: IPPatternCIDR,
			shouldError: true,
			description: "Invalid CIDR should fail validation",
		},
		{
			name:        "invalid_range",
			pattern:     "192.168.1.1-invalid",
			patternType: IPPatternRange,
			shouldError: true,
			description: "Invalid IP range should fail validation",
		},
		{
			name:        "empty_pattern",
			pattern:     "",
			patternType: IPPatternSingle,
			shouldError: true,
			description: "Empty pattern should fail validation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIPPattern(tt.pattern, tt.patternType)
			hasError := err != nil

			if hasError != tt.shouldError {
				t.Errorf("Expected error=%v, got error=%v for %s. Error: %v",
					tt.shouldError, hasError, tt.description, err)
			}
		})
	}
}

func TestURLNormalization(t *testing.T) {
	matcher := NewEnhancedURLMatcher()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "add_scheme",
			input:    "example.com",
			expected: "http://example.com/",
		},
		{
			name:     "normalize_case",
			input:    "HTTPS://EXAMPLE.COM/Path",
			expected: "https://example.com/Path",
		},
		{
			name:     "remove_default_port_http",
			input:    "http://example.com:80/path",
			expected: "http://example.com/path",
		},
		{
			name:     "remove_default_port_https",
			input:    "https://example.com:443/path",
			expected: "https://example.com/path",
		},
		{
			name:     "add_trailing_slash",
			input:    "https://example.com",
			expected: "https://example.com/",
		},
		{
			name:     "clean_double_slashes",
			input:    "https://example.com//path//to//resource",
			expected: "https://example.com/path/to/resource",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalized, err := matcher.normalizeURL(tt.input)
			if err != nil {
				t.Fatalf("Failed to normalize URL: %v", err)
			}

			if normalized != tt.expected {
				t.Errorf("Expected normalized URL=%s, got=%s", tt.expected, normalized)
			}
		})
	}
}

func TestSecurityValidation(t *testing.T) {
	tests := []struct {
		name         string
		pattern      string
		shouldBeSafe bool
		description  string
	}{
		{
			name:         "safe_pattern",
			pattern:      "example.com",
			shouldBeSafe: true,
			description:  "Simple domain pattern should be safe",
		},
		{
			name:         "redos_vulnerable",
			pattern:      "(.*)+",
			shouldBeSafe: false,
			description:  "ReDoS vulnerable pattern should be detected",
		},
		{
			name:         "nested_quantifiers",
			pattern:      "(.*){2,}",
			shouldBeSafe: false,
			description:  "Nested quantifiers should be detected",
		},
		{
			name:         "multiple_greedy_quantifiers",
			pattern:      ".*.*.*.*",
			shouldBeSafe: false,
			description:  "Multiple greedy quantifiers should be detected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRegexPattern(tt.pattern)
			isSafe := err == nil

			if isSafe != tt.shouldBeSafe {
				t.Errorf("Expected safe=%v, got safe=%v for %s. Error: %v",
					tt.shouldBeSafe, isSafe, tt.description, err)
			}
		})
	}
}

func TestEnhancedMatchers_ErrorPaths(t *testing.T) {
	urlMatcher := NewEnhancedURLMatcher()

	// Invalid URL should fail normalization.
	_, _, err := urlMatcher.MatchURL("http://[bad", []string{"example.com"})
	if err == nil {
		t.Fatalf("expected error for invalid URL")
	}

	ipMatcher := NewEnhancedIPMatcher()

	// Unsupported pattern type.
	if err := ipMatcher.CompileIPPattern("x", IPPatternType("unknown")); err == nil {
		t.Fatalf("expected error for unsupported IP pattern type")
	}

	// Invalid range format.
	if err := ipMatcher.CompileIPPattern("1.1.1.1", IPPatternRange); err == nil {
		t.Fatalf("expected error for invalid range format")
	}

	// Invalid regex pattern.
	if err := ipMatcher.CompileIPPattern("[invalid(", IPPatternRegex); err == nil {
		t.Fatalf("expected error for invalid IP regex")
	}

	// Invalid regex syntax is rejected by validator.
	if err := validateRegexPattern("[invalid("); err == nil {
		t.Fatalf("expected error for invalid regex syntax")
	}
}

func BenchmarkURLMatching(b *testing.B) {
	matcher := NewEnhancedURLMatcher()

	// Compile some test patterns
	patterns := []string{
		"example.com",
		"*.malicious.com",
		"/api/*",
		"https://exact.match.com",
	}

	patternTypes := []URLPatternType{
		URLPatternDomain,
		URLPatternSubdomain,
		URLPatternPath,
		URLPatternExact,
	}

	for i, pattern := range patterns {
		err := matcher.CompileURLPattern(pattern, patternTypes[i])
		if err != nil {
			b.Fatalf("Failed to compile pattern: %v", err)
		}
	}

	testURLs := []string{
		"https://example.com/test",
		"https://sub.malicious.com/bad",
		"https://good.com/api/users",
		"https://exact.match.com",
		"https://unmatched.com/path",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		url := testURLs[i%len(testURLs)]
		_, _, _ = matcher.MatchURL(url, patterns)
	}
}

func BenchmarkIPMatching(b *testing.B) {
	matcher := NewEnhancedIPMatcher()

	// Compile some test patterns
	patterns := []string{
		"192.168.1.1",
		"10.0.0.0/8",
		"172.16.0.1-172.16.0.100",
		"2001:db8::/32",
	}

	patternTypes := []IPPatternType{
		IPPatternSingle,
		IPPatternCIDR,
		IPPatternRange,
		IPPatternCIDR,
	}

	for i, pattern := range patterns {
		err := matcher.CompileIPPattern(pattern, patternTypes[i])
		if err != nil {
			b.Fatalf("Failed to compile pattern: %v", err)
		}
	}

	testIPs := []string{
		"192.168.1.1",
		"10.0.0.50",
		"172.16.0.50",
		"2001:db8::100",
		"8.8.8.8",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ip := testIPs[i%len(testIPs)]
		_, _, _ = matcher.MatchIP(ip, patterns)
	}
}
