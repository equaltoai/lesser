package ratelimit

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// These tests focus on the configuration and utility functions, not the middleware integration

// TestDefaultRateLimitConfig tests the default configuration
func TestDefaultRateLimitConfig(t *testing.T) {
	config := DefaultRateLimitConfig()
	
	assert.NotNil(t, config)
	assert.True(t, config.AdminBypass)
	assert.True(t, config.TrackCosts)
	assert.Equal(t, 300, config.DefaultLimit)
	assert.Equal(t, 5*time.Minute, config.DefaultWindow)
	
	// Test some specific endpoint limits
	statusLimit, exists := config.EndpointLimits["POST:/api/v1/statuses"]
	assert.True(t, exists)
	assert.Equal(t, 30, statusLimit.Limit)
	assert.Equal(t, time.Hour, statusLimit.Window)
	
	mediaLimit, exists := config.EndpointLimits["POST:/api/v1/media"]
	assert.True(t, exists)
	assert.Equal(t, 20, mediaLimit.Limit)
	assert.Equal(t, time.Hour, mediaLimit.Window)
}

// TestGetLimitConfig tests the endpoint matching logic
func TestGetLimitConfig(t *testing.T) {
	config := DefaultRateLimitConfig()
	
	// Test exact match
	limitConfig := getLimitConfig("POST:/api/v1/statuses", config)
	assert.Equal(t, 30, limitConfig.Limit)
	assert.Equal(t, time.Hour, limitConfig.Window)
	
	// Test wildcard match
	limitConfig = getLimitConfig("POST:/api/v1/statuses/123/favourite", config)
	assert.Equal(t, 100, limitConfig.Limit)
	assert.Equal(t, time.Hour, limitConfig.Window)
	
	// Test default fallback
	limitConfig = getLimitConfig("GET:/api/v1/unknown", config)
	assert.Equal(t, 300, limitConfig.Limit)
	assert.Equal(t, 5*time.Minute, limitConfig.Window)
}

// TestMatchesWildcard tests the wildcard matching logic
func TestMatchesWildcard(t *testing.T) {
	tests := []struct {
		endpoint string
		pattern  string
		expected bool
	}{
		{"POST:/api/v1/statuses/123/favourite", "POST:/api/v1/statuses/*/favourite", true},
		{"DELETE:/api/v1/statuses/456/favourite", "POST:/api/v1/statuses/*/favourite", false},
		{"POST:/api/v1/statuses/123/reblog", "POST:/api/v1/statuses/*/favourite", false},
		{"POST:/api/v1/statuses", "POST:/api/v1/statuses", true},
		{"GET:/api/v1/timelines/home", "GET:/api/v1/timelines/*", true},
	}
	
	for _, test := range tests {
		t.Run(fmt.Sprintf("%s matches %s", test.endpoint, test.pattern), func(t *testing.T) {
			result := matchesWildcard(test.endpoint, test.pattern)
			assert.Equal(t, test.expected, result)
		})
	}
}

// TestBuildEndpointPattern tests endpoint pattern building
func TestBuildEndpointPattern(t *testing.T) {
	tests := []struct {
		method   string
		path     string
		expected string
	}{
		{"POST", "/api/v1/statuses", "POST:/api/v1/statuses"},
		{"GET", "/api/v1/timelines/home", "GET:/api/v1/timelines/home"},
		{"DELETE", "/api/v1/statuses/123", "DELETE:/api/v1/statuses/123"},
	}
	
	for _, test := range tests {
		t.Run(fmt.Sprintf("%s %s", test.method, test.path), func(t *testing.T) {
			result := buildEndpointPattern(test.method, test.path)
			assert.Equal(t, test.expected, result)
		})
	}
}

// TestExtractDomainFromKeyID tests ActivityPub domain extraction
func TestExtractDomainFromKeyID(t *testing.T) {
	tests := []struct {
		keyID    string
		expected string
	}{
		{"https://example.com/users/alice#main-key", "example.com"},
		{"https://mastodon.social/users/bob#main-key", "mastodon.social"},
		{"https://pixelfed.social/users/charlie", "pixelfed.social"},
		{"invalid-url", ""},
		{"", ""},
	}
	
	for _, test := range tests {
		t.Run(test.keyID, func(t *testing.T) {
			result := extractDomainFromKeyID(test.keyID)
			assert.Equal(t, test.expected, result)
		})
	}
}

// TestParseKeyIDFromSignature tests signature parsing
func TestParseKeyIDFromSignature(t *testing.T) {
	tests := []struct {
		signature string
		expected  string
	}{
		{`keyId="https://example.com/users/alice#main-key",algorithm="rsa-sha256"`, "https://example.com/users/alice#main-key"},
		{`algorithm="rsa-sha256",keyId="https://mastodon.social/users/bob#main-key"`, "https://mastodon.social/users/bob#main-key"},
		{`invalid-signature-format`, ""},
		{"", ""},
	}
	
	for _, test := range tests {
		t.Run(test.signature, func(t *testing.T) {
			result := parseKeyIDFromSignature(test.signature)
			assert.Equal(t, test.expected, result)
		})
	}
}