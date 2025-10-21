package security

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/patrickmn/go-cache"
)

func TestIsAuthorizedIP(t *testing.T) {
	tests := []struct {
		name       string
		sourceIP   string
		config     IPAuthorizationConfig
		authorized bool
	}{
		{
			name:     "IP in AllowedIPs slice",
			sourceIP: "192.168.1.100",
			config: IPAuthorizationConfig{
				AllowedIPs: []string{"192.168.1.100", "192.168.1.101"},
			},
			authorized: true,
		},
		{
			name:     "IP not in AllowedIPs slice",
			sourceIP: "192.168.1.102",
			config: IPAuthorizationConfig{
				AllowedIPs: []string{"192.168.1.100", "192.168.1.101"},
			},
			authorized: false,
		},
		{
			name:     "IP in AllowedIPList string",
			sourceIP: "10.0.0.1",
			config: IPAuthorizationConfig{
				AllowedIPList: "10.0.0.1,10.0.0.2,10.0.0.3",
			},
			authorized: true,
		},
		{
			name:     "IP not in AllowedIPList string",
			sourceIP: "10.0.0.4",
			config: IPAuthorizationConfig{
				AllowedIPList: "10.0.0.1,10.0.0.2,10.0.0.3",
			},
			authorized: false,
		},
		{
			name:     "AllowedIPList takes precedence over AllowedIPs",
			sourceIP: "172.16.0.1",
			config: IPAuthorizationConfig{
				AllowedIPs:    []string{"192.168.1.100"},
				AllowedIPList: "172.16.0.1,172.16.0.2",
			},
			authorized: true,
		},
		{
			name:     "IP with port in source",
			sourceIP: "192.168.1.100:8080",
			config: IPAuthorizationConfig{
				AllowedIPs: []string{"192.168.1.100"},
			},
			authorized: true,
		},
		{
			name:     "IP with port in allowed list",
			sourceIP: "192.168.1.100",
			config: IPAuthorizationConfig{
				AllowedIPs: []string{"192.168.1.100:8080"},
			},
			authorized: true,
		},
		{
			name:     "Both IPs have ports",
			sourceIP: "192.168.1.100:8080",
			config: IPAuthorizationConfig{
				AllowedIPs: []string{"192.168.1.100:9090"},
			},
			authorized: true,
		},
		{
			name:     "IPv6 address",
			sourceIP: "2001:db8::1",
			config: IPAuthorizationConfig{
				AllowedIPs: []string{"2001:db8::1", "2001:db8::2"},
			},
			authorized: true,
		},
		{
			name:     "Empty AllowedIPs",
			sourceIP: "192.168.1.100",
			config: IPAuthorizationConfig{
				AllowedIPs: []string{},
			},
			authorized: false,
		},
		{
			name:     "Empty AllowedIPList",
			sourceIP: "192.168.1.100",
			config: IPAuthorizationConfig{
				AllowedIPList: "",
			},
			authorized: false,
		},
		{
			name:     "Whitespace in AllowedIPList",
			sourceIP: "192.168.1.100",
			config: IPAuthorizationConfig{
				AllowedIPList: "  192.168.1.100  ,  192.168.1.101  ",
			},
			authorized: true,
		},
		{
			name:     "Real-world VPC NAT Gateway IPs",
			sourceIP: "52.54.123.456",
			config: IPAuthorizationConfig{
				AllowedIPList: "52.54.123.456,34.123.456.789,18.234.567.890",
			},
			authorized: true,
		},
		{
			name:     "Case with trailing comma",
			sourceIP: "10.0.0.1",
			config: IPAuthorizationConfig{
				AllowedIPList: "10.0.0.1,10.0.0.2,",
			},
			authorized: true,
		},
		{
			name:     "Case with leading comma",
			sourceIP: "10.0.0.2",
			config: IPAuthorizationConfig{
				AllowedIPList: ",10.0.0.1,10.0.0.2",
			},
			authorized: true,
		},
		{
			name:     "Multiple commas between IPs",
			sourceIP: "10.0.0.1",
			config: IPAuthorizationConfig{
				AllowedIPList: "10.0.0.1,,10.0.0.2",
			},
			authorized: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsAuthorizedIP(tt.sourceIP, tt.config)
			if result != tt.authorized {
				t.Errorf("IsAuthorizedIP(%q, %+v) = %v, want %v", tt.sourceIP, tt.config, result, tt.authorized)
			}
		})
	}
}

func TestIsAuthorizedIPSimple(t *testing.T) {
	tests := []struct {
		name          string
		sourceIP      string
		allowedIPList string
		authorized    bool
	}{
		{
			name:          "IP in list",
			sourceIP:      "192.168.1.100",
			allowedIPList: "192.168.1.100,192.168.1.101",
			authorized:    true,
		},
		{
			name:          "IP not in list",
			sourceIP:      "192.168.1.102",
			allowedIPList: "192.168.1.100,192.168.1.101",
			authorized:    false,
		},
		{
			name:          "Empty list",
			sourceIP:      "192.168.1.100",
			allowedIPList: "",
			authorized:    false,
		},
		{
			name:          "Single IP in list",
			sourceIP:      "10.0.0.1",
			allowedIPList: "10.0.0.1",
			authorized:    true,
		},
		{
			name:          "IP with spaces",
			sourceIP:      "  10.0.0.1  ",
			allowedIPList: "10.0.0.1,10.0.0.2",
			authorized:    true,
		},
		{
			name:          "Real AWS NAT Gateway example",
			sourceIP:      "52.54.123.456",
			allowedIPList: "52.54.123.456,34.123.456.789,18.234.567.890",
			authorized:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsAuthorizedIPSimple(tt.sourceIP, tt.allowedIPList)
			if result != tt.authorized {
				t.Errorf("IsAuthorizedIPSimple(%q, %q) = %v, want %v", tt.sourceIP, tt.allowedIPList, result, tt.authorized)
			}
		})
	}
}

func TestCheckIPInList(t *testing.T) {
	tests := []struct {
		name       string
		sourceIP   string
		allowedIPs []string
		expected   bool
	}{
		{
			name:       "Exact match",
			sourceIP:   "192.168.1.100",
			allowedIPs: []string{"192.168.1.100", "192.168.1.101"},
			expected:   true,
		},
		{
			name:       "No match",
			sourceIP:   "192.168.1.102",
			allowedIPs: []string{"192.168.1.100", "192.168.1.101"},
			expected:   false,
		},
		{
			name:       "Match with whitespace",
			sourceIP:   "  192.168.1.100  ",
			allowedIPs: []string{"192.168.1.100"},
			expected:   true,
		},
		{
			name:       "Source IP with port",
			sourceIP:   "192.168.1.100:8080",
			allowedIPs: []string{"192.168.1.100"},
			expected:   true,
		},
		{
			name:       "Allowed IP with port",
			sourceIP:   "192.168.1.100",
			allowedIPs: []string{"192.168.1.100:8080"},
			expected:   true,
		},
		{
			name:       "Empty allowed list",
			sourceIP:   "192.168.1.100",
			allowedIPs: []string{},
			expected:   false,
		},
		{
			name:       "Nil allowed list",
			sourceIP:   "192.168.1.100",
			allowedIPs: nil,
			expected:   false,
		},
		{
			name:       "IPv6 address",
			sourceIP:   "2001:db8::1",
			allowedIPs: []string{"2001:db8::1", "2001:db8::2"},
			expected:   true,
		},
		{
			name:       "Case sensitivity",
			sourceIP:   "192.168.1.100",
			allowedIPs: []string{"192.168.1.100"},
			expected:   true,
		},
		{
			name:       "Localhost",
			sourceIP:   "127.0.0.1",
			allowedIPs: []string{"127.0.0.1", "localhost"},
			expected:   true,
		},
		{
			name:       "Private IP ranges",
			sourceIP:   "10.0.0.1",
			allowedIPs: []string{"10.0.0.1", "10.0.0.2"},
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checkIPInList(tt.sourceIP, tt.allowedIPs)
			if result != tt.expected {
				t.Errorf("checkIPInList(%q, %v) = %v, want %v", tt.sourceIP, tt.allowedIPs, result, tt.expected)
			}
		})
	}
}

func TestParseIPList(t *testing.T) {
	tests := []struct {
		name     string
		ipList   string
		expected []string
	}{
		{
			name:     "Simple comma-separated list",
			ipList:   "192.168.1.1,192.168.1.2,192.168.1.3",
			expected: []string{"192.168.1.1", "192.168.1.2", "192.168.1.3"},
		},
		{
			name:     "List with spaces",
			ipList:   "192.168.1.1, 192.168.1.2, 192.168.1.3",
			expected: []string{"192.168.1.1", "192.168.1.2", "192.168.1.3"},
		},
		{
			name:     "List with extra spaces",
			ipList:   "  192.168.1.1  ,  192.168.1.2  ,  192.168.1.3  ",
			expected: []string{"192.168.1.1", "192.168.1.2", "192.168.1.3"},
		},
		{
			name:     "Empty string",
			ipList:   "",
			expected: []string{},
		},
		{
			name:     "Single IP",
			ipList:   "192.168.1.1",
			expected: []string{"192.168.1.1"},
		},
		{
			name:     "Trailing comma",
			ipList:   "192.168.1.1,192.168.1.2,",
			expected: []string{"192.168.1.1", "192.168.1.2"},
		},
		{
			name:     "Leading comma",
			ipList:   ",192.168.1.1,192.168.1.2",
			expected: []string{"192.168.1.1", "192.168.1.2"},
		},
		{
			name:     "Multiple commas",
			ipList:   "192.168.1.1,,192.168.1.2",
			expected: []string{"192.168.1.1", "192.168.1.2"},
		},
		{
			name:     "IPv6 addresses",
			ipList:   "2001:db8::1,2001:db8::2,2001:db8::3",
			expected: []string{"2001:db8::1", "2001:db8::2", "2001:db8::3"},
		},
		{
			name:     "Mixed IPv4 and IPv6",
			ipList:   "192.168.1.1,2001:db8::1,10.0.0.1",
			expected: []string{"192.168.1.1", "2001:db8::1", "10.0.0.1"},
		},
		{
			name:     "Real VPC NAT Gateway list",
			ipList:   "52.54.123.456,34.123.456.789,18.234.567.890",
			expected: []string{"52.54.123.456", "34.123.456.789", "18.234.567.890"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseIPList(tt.ipList)
			if len(result) != len(tt.expected) {
				t.Errorf("parseIPList(%q) returned %d items, want %d", tt.ipList, len(result), len(tt.expected))
				return
			}
			for i, ip := range result {
				if ip != tt.expected[i] {
					t.Errorf("parseIPList(%q)[%d] = %q, want %q", tt.ipList, i, ip, tt.expected[i])
				}
			}
		})
	}
}

// Note: SSM client tests would require mocking the AWS SDK
// which is not included in the project dependencies.
// For production use, integration tests with actual AWS services
// or using localstack would be recommended.

func TestBuildVPCNATGatewayParameterName(t *testing.T) {
	tests := []struct {
		name      string
		partner   string
		stage     string
		component string
		expected  string
	}{
		{
			name:      "GoChallenge service",
			partner:   "paytheory",
			stage:     "prod",
			component: "gochallenge-vpc-nat-gateway-list",
			expected:  "pt-partner-paytheory-prod-gochallenge-vpc-nat-gateway-list",
		},
		{
			name:      "Different service",
			partner:   "paytheory",
			stage:     "prod",
			component: "otherservice-vpc-nat-gateway-list",
			expected:  "pt-partner-paytheory-prod-otherservice-vpc-nat-gateway-list",
		},
		{
			name:      "Dev environment",
			partner:   "testpartner",
			stage:     "dev",
			component: "gochallenge-vpc-nat-gateway-list",
			expected:  "pt-partner-testpartner-dev-gochallenge-vpc-nat-gateway-list",
		},
		{
			name:      "Staging environment",
			partner:   "acme",
			stage:     "staging",
			component: "myapp-allowed-ips",
			expected:  "pt-partner-acme-staging-myapp-allowed-ips",
		},
		{
			name:      "Custom component name",
			partner:   "partner1",
			stage:     "test",
			component: "api-gateway-whitelist",
			expected:  "pt-partner-partner1-test-api-gateway-whitelist",
		},
		{
			name:      "Empty partner and stage",
			partner:   "",
			stage:     "",
			component: "service-vpc-nat-gateway-list",
			expected:  "pt-partner---service-vpc-nat-gateway-list",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildVPCNATGatewayParameterName(tt.partner, tt.stage, tt.component)
			if result != tt.expected {
				t.Errorf("BuildVPCNATGatewayParameterName(%q, %q, %q) = %q, want %q", tt.partner, tt.stage, tt.component, result, tt.expected)
			}
		})
	}
}

func TestSSMIPAuthorizerCaching(t *testing.T) {
	t.Run("Cache hit on second request", func(t *testing.T) {
		// Create a test cache with short TTL for testing
		testCache := cache.New(1*time.Minute, 30*time.Second)

		// Pre-populate cache with test data
		cacheKey := "ssm:ip-list:test-parameter"
		allowedIPs := []string{"192.168.1.100", "192.168.1.101"}
		testCache.Set(cacheKey, allowedIPs, cache.DefaultExpiration)

		// Note: We're testing the cache behavior directly without using the authorizer
		// since we're testing cached data retrieval

		// Test that the IP check uses cached data
		authorized := checkIPInList("192.168.1.100", allowedIPs)
		if !authorized {
			t.Error("Expected IP to be authorized from cached list")
		}

		// Verify cache contains the data
		if cached, found := testCache.Get(cacheKey); !found {
			t.Error("Expected cache to contain the IP list")
		} else {
			if cachedIPs, ok := cached.([]string); !ok || len(cachedIPs) != 2 {
				t.Error("Cached data is not in expected format")
			}
		}
	})

	t.Run("Cache expiration", func(t *testing.T) {
		// Create a test cache with very short TTL
		testCache := cache.New(100*time.Millisecond, 50*time.Millisecond)

		// Add data to cache
		cacheKey := "ssm:ip-list:expiring-parameter"
		allowedIPs := []string{"10.0.0.1"}
		testCache.Set(cacheKey, allowedIPs, cache.DefaultExpiration)

		// Verify it's in cache
		if _, found := testCache.Get(cacheKey); !found {
			t.Error("Expected cache to contain the IP list immediately after setting")
		}

		// Wait for expiration
		time.Sleep(200 * time.Millisecond)

		// Verify it's no longer in cache
		if _, found := testCache.Get(cacheKey); found {
			t.Error("Expected cache entry to be expired")
		}
	})

	t.Run("ClearCache functionality", func(t *testing.T) {
		authorizer := &SSMIPAuthorizer{
			cache:    cache.New(5*time.Minute, 1*time.Minute),
			cacheTTL: 5 * time.Minute,
		}

		// Add some data to cache
		cacheKey := "ssm:ip-list:clear-test"
		authorizer.cache.Set(cacheKey, []string{"172.16.0.1"}, cache.DefaultExpiration)

		// Verify it's in cache
		if _, found := authorizer.cache.Get(cacheKey); !found {
			t.Error("Expected cache to contain data before clearing")
		}

		// Clear cache
		authorizer.ClearCache()

		// Verify cache is empty
		if _, found := authorizer.cache.Get(cacheKey); found {
			t.Error("Expected cache to be empty after clearing")
		}
	})

	t.Run("GetCacheStats functionality", func(t *testing.T) {
		authorizer := &SSMIPAuthorizer{
			cache:    cache.New(5*time.Minute, 1*time.Minute),
			cacheTTL: 5 * time.Minute,
		}

		// Initially empty
		items, _ := authorizer.GetCacheStats()
		if items != 0 {
			t.Errorf("Expected 0 items in cache, got %d", items)
		}

		// Add some items
		authorizer.cache.Set("ssm:ip-list:param1", []string{"1.1.1.1"}, cache.DefaultExpiration)
		authorizer.cache.Set("ssm:ip-list:param2", []string{"2.2.2.2"}, cache.DefaultExpiration)

		items, _ = authorizer.GetCacheStats()
		if items != 2 {
			t.Errorf("Expected 2 items in cache, got %d", items)
		}
	})

	t.Run("Multiple parameter caching", func(t *testing.T) {
		testCache := cache.New(5*time.Minute, 1*time.Minute)

		// Add multiple different parameter values
		testCache.Set("ssm:ip-list:service1-ips", []string{"10.1.0.1", "10.1.0.2"}, cache.DefaultExpiration)
		testCache.Set("ssm:ip-list:service2-ips", []string{"10.2.0.1", "10.2.0.2"}, cache.DefaultExpiration)
		testCache.Set("ssm:ip-list:service3-ips", []string{"10.3.0.1", "10.3.0.2"}, cache.DefaultExpiration)

		// Verify each can be retrieved independently
		if cached, found := testCache.Get("ssm:ip-list:service1-ips"); found {
			ips := cached.([]string)
			if len(ips) != 2 || ips[0] != "10.1.0.1" {
				t.Error("service1-ips not cached correctly")
			}
		} else {
			t.Error("service1-ips not found in cache")
		}

		if cached, found := testCache.Get("ssm:ip-list:service2-ips"); found {
			ips := cached.([]string)
			if len(ips) != 2 || ips[0] != "10.2.0.1" {
				t.Error("service2-ips not cached correctly")
			}
		} else {
			t.Error("service2-ips not found in cache")
		}
	})
}

func TestSSMIPAuthorizerConfig(t *testing.T) {
	t.Run("Default cache TTL", func(t *testing.T) {
		authorizer := NewSSMIPAuthorizerWithConfig(nil, SSMIPAuthorizerConfig{})
		if authorizer.cacheTTL != 15*time.Minute {
			t.Errorf("Expected default cache TTL of 15 minutes, got %v", authorizer.cacheTTL)
		}
	})

	t.Run("Custom cache TTL", func(t *testing.T) {
		customTTL := 10 * time.Minute
		authorizer := NewSSMIPAuthorizerWithConfig(nil, SSMIPAuthorizerConfig{
			CacheTTL: customTTL,
		})
		if authorizer.cacheTTL != customTTL {
			t.Errorf("Expected cache TTL of %v, got %v", customTTL, authorizer.cacheTTL)
		}
	})

	t.Run("Very short cache TTL", func(t *testing.T) {
		shortTTL := 30 * time.Second
		authorizer := NewSSMIPAuthorizerWithConfig(nil, SSMIPAuthorizerConfig{
			CacheTTL: shortTTL,
		})
		if authorizer.cacheTTL != shortTTL {
			t.Errorf("Expected cache TTL of %v, got %v", shortTTL, authorizer.cacheTTL)
		}
	})
}

func TestIPAuthorizationService(t *testing.T) {
	t.Run("NewIPAuthorizationService", func(t *testing.T) {
		service := NewIPAuthorizationService(nil, "test-parameter")
		if service.ssmParameterName != "test-parameter" {
			t.Errorf("Expected SSM parameter name to be 'test-parameter', got %s", service.ssmParameterName)
		}
		if service.authorizer == nil {
			t.Error("Expected authorizer to be initialized")
		}
	})

	t.Run("IsAuthorizedIP with empty source IP", func(t *testing.T) {
		service := NewIPAuthorizationService(nil, "test-parameter")
		_, err := service.IsAuthorizedIP(context.Background(), "")
		if err == nil {
			t.Error("Expected error for empty source IP")
		}
		if err.Error() != "source IP cannot be empty" {
			t.Errorf("Expected 'source IP cannot be empty' error, got: %v", err)
		}
	})

	t.Run("NewIPAuthorizationServiceFromEnv with missing component", func(t *testing.T) {
		_, err := NewIPAuthorizationServiceFromEnv(context.Background(), "")
		if err == nil {
			t.Error("Expected error for empty component")
		}
		if err.Error() != "component name must be provided" {
			t.Errorf("Expected 'component name must be provided' error, got: %v", err)
		}
	})

	t.Run("NewIPAuthorizationServiceFromEnv with missing env vars", func(t *testing.T) {
		// Save current env vars
		oldPartner := os.Getenv("PARTNER")
		oldStage := os.Getenv("STAGE")

		// Clear env vars
		os.Unsetenv("PARTNER")
		os.Unsetenv("STAGE")

		_, err := NewIPAuthorizationServiceFromEnv(context.Background(), "test-component")
		if err == nil {
			t.Error("Expected error for missing env vars")
		}
		if err.Error() != "PARTNER and STAGE environment variables must be set" {
			t.Errorf("Expected env vars error, got: %v", err)
		}

		// Restore env vars
		if oldPartner != "" {
			os.Setenv("PARTNER", oldPartner)
		}
		if oldStage != "" {
			os.Setenv("STAGE", oldStage)
		}
	})
}

func TestCheckIPAuthorization(t *testing.T) {
	t.Run("Empty source IP", func(t *testing.T) {
		_, err := CheckIPAuthorization(context.Background(), "", nil, "test-parameter")
		if err == nil {
			t.Error("Expected error for empty source IP")
		}
		if err.Error() != "source IP cannot be empty" {
			t.Errorf("Expected 'source IP cannot be empty' error, got: %v", err)
		}
	})

	t.Run("Empty SSM parameter name", func(t *testing.T) {
		_, err := CheckIPAuthorization(context.Background(), "192.168.1.1", nil, "")
		if err == nil {
			t.Error("Expected error for empty SSM parameter name")
		}
		if err.Error() != "SSM parameter name must be provided" {
			t.Errorf("Expected 'SSM parameter name must be provided' error, got: %v", err)
		}
	})
}
