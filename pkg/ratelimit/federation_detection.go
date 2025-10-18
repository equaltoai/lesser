// Package ratelimit provides federation detection utilities for rate limiting
package ratelimit

import (
	"strings"
)

// IsFederationRequest detects if a request is likely from an ActivityPub server
func IsFederationRequest(userAgent, accept, contentType, path string) bool {
	// Check federation endpoints first
	if IsFederationEndpoint(path) {
		return true
	}

	// Check User-Agent for known ActivityPub servers
	if IsFederationUserAgent(userAgent) {
		return true
	}

	// Check Accept header for ActivityPub content types
	if IsFederationContentType(accept, contentType) {
		return true
	}

	return false
}

// IsFederationEndpoint checks if a path is a federation endpoint
func IsFederationEndpoint(path string) bool {
	federationPaths := []string{
		"/inbox",
		"/outbox",
		"/users/",
		"/.well-known/",
		"/nodeinfo",
		"/actor/",
		"/objects/",
	}

	for _, fedPath := range federationPaths {
		if strings.HasPrefix(path, fedPath) || strings.Contains(path, fedPath) {
			return true
		}
	}

	return false
}

// IsFederationUserAgent checks if a User-Agent indicates an ActivityPub server
func IsFederationUserAgent(userAgent string) bool {
	if userAgent == "" {
		return false
	}

	// Known ActivityPub server user agents
	federationUAs := []string{
		"Mastodon",
		"Pleroma",
		"Misskey",
		"PeerTube",
		"PixelFed",
		"Lemmy",
		"Kbin",
		"GoToSocial",
		"Lesser",  // Our own server
		"http.rb", // Ruby HTTP library used by many AP servers
		"Akkoma",
		"Friendica",
		"Hubzilla",
		"Sharkey",
		"Iceshrimp",
		"Firefish",
		"Calckey", // Predecessor to Firefish
		"FoundKey",
		"GNU social",
		"postActiv",
		"Smithereen",
		"Hometown", // Mastodon fork
		"Glitch",   // Another Mastodon fork
	}

	userAgentLower := strings.ToLower(userAgent)
	for _, ua := range federationUAs {
		if strings.Contains(userAgentLower, strings.ToLower(ua)) {
			return true
		}
	}

	// Check for generic federation indicators
	federationIndicators := []string{
		"activitypub",
		"federation",
		"/inbox",
		"/outbox",
		"webfinger",
	}

	for _, indicator := range federationIndicators {
		if strings.Contains(userAgentLower, indicator) {
			return true
		}
	}

	return false
}

// IsFederationContentType checks if content types indicate ActivityPub
func IsFederationContentType(accept, contentType string) bool {
	activityPubTypes := []string{
		"application/activity+json",
		"application/ld+json",
		"application/json", // Generic, but used by some servers
	}

	// Check Accept header
	if accept != "" {
		acceptLower := strings.ToLower(accept)
		for _, apType := range activityPubTypes {
			if strings.Contains(acceptLower, apType) {
				// Additional check for ActivityStreams profile
				if strings.Contains(acceptLower, "activitystreams") ||
					strings.Contains(acceptLower, "profile=") {
					return true
				}
				// For specific ActivityPub types
				if apType == "application/activity+json" {
					return true
				}
			}
		}
	}

	// Check Content-Type header
	if contentType != "" {
		contentTypeLower := strings.ToLower(contentType)
		for _, apType := range activityPubTypes {
			if strings.Contains(contentTypeLower, apType) {
				// Same additional checks as above
				if strings.Contains(contentTypeLower, "activitystreams") ||
					strings.Contains(contentTypeLower, "profile=") {
					return true
				}
				if apType == "application/activity+json" {
					return true
				}
			}
		}
	}

	return false
}

// GetFederationLimits returns appropriate rate limits for federation requests
func GetFederationLimits(endpoint string) (requestsPerMinute int, burstSize int) {
	switch {
	case strings.Contains(endpoint, "/inbox"):
		// Inbox delivery needs high limits
		return 1000, 50
	case strings.Contains(endpoint, "/outbox"):
		// Outbox fetching by remote servers
		return 600, 30
	case strings.Contains(endpoint, "/users/") || strings.Contains(endpoint, "/actor/"):
		// Actor fetching
		return 600, 30
	case strings.Contains(endpoint, "/.well-known/webfinger"):
		// WebFinger lookups
		return 600, 30
	case strings.Contains(endpoint, "/.well-known/nodeinfo") || strings.Contains(endpoint, "/nodeinfo"):
		// NodeInfo fetching
		return 300, 15
	case strings.Contains(endpoint, "/objects/"):
		// Object fetching
		return 600, 30
	default:
		// Generic federation endpoint
		return 400, 20
	}
}

// GetClientLimits returns appropriate rate limits for client requests
func GetClientLimits(endpoint string, authenticated bool) (requestsPerMinute int, burstSize int) {
	baseLimit := 60
	if authenticated {
		baseLimit = 120 // Authenticated users get higher limits
	}

	switch {
	case strings.Contains(endpoint, "/api/v1/statuses"):
		// Post creation
		if authenticated {
			return 60, 10
		}
		return 30, 5
	case strings.Contains(endpoint, "/api/v1/timelines/"):
		// Timeline fetching
		if authenticated {
			return 180, 30
		}
		return 60, 15
	case strings.Contains(endpoint, "/api/v1/media"):
		// Media uploads
		if authenticated {
			return 20, 5
		}
		return 5, 2
	case strings.Contains(endpoint, "/api/v1/search"):
		// Search requests
		if authenticated {
			return 60, 15
		}
		return 30, 10
	case strings.Contains(endpoint, "/oauth/"):
		// OAuth endpoints - very strict
		return 10, 3
	case strings.HasPrefix(endpoint, "/api/"):
		// Other API endpoints
		return baseLimit, 10
	default:
		// Default limits
		return baseLimit, 10
	}
}

// ShouldApplyFederationLimits determines if federation-friendly rate limits should be used
func ShouldApplyFederationLimits(userAgent, accept, contentType, path, signature string) bool {
	// Check for HTTP signature (definitive federation indicator)
	if signature != "" {
		return true
	}

	// Use the main federation detection logic
	return IsFederationRequest(userAgent, accept, contentType, path)
}
