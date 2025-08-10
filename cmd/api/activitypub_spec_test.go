package main

import (
	"encoding/json"
	"testing"

	apiLift "github.com/equaltoai/lesser/cmd/api/lift"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/stretchr/testify/assert"
)

// Test spec compliance without initializing the full application

func TestWebFingerResponseSpecCompliance(t *testing.T) {
	// Test WebFinger response structure compliance with RFC 7033
	response := apiLift.WebFingerResponse{
		Subject: "acct:testuser@example.com",
		Aliases: []string{"https://example.com/users/testuser"},
		Links: []apiLift.WebFingerLink{
			{
				Rel:  "self",
				Type: "application/activity+json",
				Href: "https://example.com/users/testuser",
			},
			{
				Rel:  "http://webfinger.net/rel/profile-page",
				Type: "text/html",
				Href: "https://example.com/users/testuser",
			},
		},
	}

	// Serialize to JSON to verify structure
	jsonData, err := json.Marshal(response)
	assert.NoError(t, err)

	// Parse back to verify all required fields are present
	var parsed map[string]interface{}
	err = json.Unmarshal(jsonData, &parsed)
	assert.NoError(t, err)

	// Verify required WebFinger fields (RFC 7033)
	assert.Contains(t, parsed, "subject")
	assert.Contains(t, parsed, "links")
	assert.Equal(t, "acct:testuser@example.com", parsed["subject"])

	// Verify links structure
	links := parsed["links"].([]interface{})
	assert.GreaterOrEqual(t, len(links), 1, "Must have at least one link")

	// Find and verify ActivityPub self link
	var selfLink map[string]interface{}
	for _, link := range links {
		if linkMap := link.(map[string]interface{}); linkMap["rel"] == "self" {
			selfLink = linkMap
			break
		}
	}

	assert.NotEmpty(t, selfLink, "Must have self link")
	assert.Equal(t, "self", selfLink["rel"])
	assert.Equal(t, "application/activity+json", selfLink["type"])
	assert.Contains(t, selfLink["href"], "/users/testuser")
}

func TestNodeInfoResponseSpecCompliance(t *testing.T) {
	// Test NodeInfo response structure compliance with NodeInfo 2.0 schema
	response := apiLift.NodeInfo{
		Version: "2.0",
		Software: apiLift.NodeInfoSoftware{
			Name:       "lesser",
			Version:    "0.1.0",
			Repository: "https://github.com/equaltoai/lesser",
			Homepage:   "https://github.com/equaltoai/lesser",
		},
		Protocols: []string{"activitypub"},
		Services: apiLift.NodeInfoServices{
			Inbound:  []string{},
			Outbound: []string{},
		},
		OpenRegistrations: true,
		Usage: apiLift.NodeInfoUsage{
			Users: apiLift.NodeInfoUsers{
				Total:          100,
				ActiveMonth:    75,
				ActiveHalfyear: 90,
			},
			LocalPosts:    500,
			LocalComments: 0,
		},
		Metadata: map[string]any{
			"nodeName":        "Test Instance",
			"nodeDescription": "A test instance",
			"maintainer": map[string]any{
				"name":  "admin@example.com",
				"email": "admin@example.com",
			},
		},
	}

	// Serialize to JSON to verify structure
	jsonData, err := json.Marshal(response)
	assert.NoError(t, err)

	// Parse back to verify all required fields are present
	var parsed map[string]interface{}
	err = json.Unmarshal(jsonData, &parsed)
	assert.NoError(t, err)

	// Verify required NodeInfo 2.0 schema fields
	assert.Equal(t, "2.0", parsed["version"], "version field is required")
	assert.Contains(t, parsed, "software", "software field is required")
	assert.Contains(t, parsed, "protocols", "protocols field is required")
	assert.Contains(t, parsed, "services", "services field is required")
	assert.Contains(t, parsed, "openRegistrations", "openRegistrations field is required")
	assert.Contains(t, parsed, "usage", "usage field is required")

	// Verify software section
	software := parsed["software"].(map[string]interface{})
	assert.Equal(t, "lesser", software["name"], "software.name is required")
	assert.Contains(t, software, "version", "software.version is required")

	// Verify protocols contain ActivityPub
	protocols := parsed["protocols"].([]interface{})
	assert.Contains(t, protocols, "activitypub", "must support ActivityPub protocol")

	// Verify services structure
	services := parsed["services"].(map[string]interface{})
	assert.Contains(t, services, "inbound", "services.inbound is required")
	assert.Contains(t, services, "outbound", "services.outbound is required")

	// Verify usage statistics structure
	usage := parsed["usage"].(map[string]interface{})
	assert.Contains(t, usage, "users", "usage.users is required")
	users := usage["users"].(map[string]interface{})
	assert.Contains(t, users, "total", "users.total is required")
	assert.Contains(t, users, "activeMonth", "users.activeMonth is required")
	assert.Contains(t, users, "activeHalfyear", "users.activeHalfyear is required")

	// Verify usage values are non-negative
	assert.GreaterOrEqual(t, int(users["total"].(float64)), 0, "users.total must be non-negative")
	assert.GreaterOrEqual(t, int(users["activeMonth"].(float64)), 0, "users.activeMonth must be non-negative")
	assert.GreaterOrEqual(t, int(users["activeHalfyear"].(float64)), 0, "users.activeHalfyear must be non-negative")
	assert.GreaterOrEqual(t, int(usage["localPosts"].(float64)), 0, "localPosts must be non-negative")
}

func TestNodeInfoWellKnownSpecCompliance(t *testing.T) {
	// Test NodeInfo well-known discovery response
	response := apiLift.NodeInfoWellKnown{
		Links: []apiLift.NodeInfoLink{
			{
				Rel:  "http://nodeinfo.diaspora.software/ns/schema/2.0",
				Href: "https://example.com/nodeinfo/2.0",
			},
		},
	}

	// Serialize to JSON to verify structure
	jsonData, err := json.Marshal(response)
	assert.NoError(t, err)

	// Parse back to verify structure
	var parsed map[string]interface{}
	err = json.Unmarshal(jsonData, &parsed)
	assert.NoError(t, err)

	// Verify links structure per NodeInfo spec
	assert.Contains(t, parsed, "links", "links field is required")
	links := parsed["links"].([]interface{})
	assert.GreaterOrEqual(t, len(links), 1, "must have at least one NodeInfo link")

	// Verify 2.0 schema link
	found2_0 := false
	for _, link := range links {
		linkMap := link.(map[string]interface{})
		if rel := linkMap["rel"].(string); rel == "http://nodeinfo.diaspora.software/ns/schema/2.0" {
			found2_0 = true
			assert.Contains(t, linkMap["href"], "nodeinfo/2.0", "href should point to NodeInfo 2.0 endpoint")
			break
		}
	}
	assert.True(t, found2_0, "must have NodeInfo 2.0 schema link")
}

func TestActivityPubHeaderConstants(t *testing.T) {
	// Test that we have the correct ActivityPub header constants defined
	assert.Equal(t, "Digest", common.DigestHeader)
	assert.Equal(t, "Signature", common.SignatureHeader)
	assert.Equal(t, "Date", common.DateHeader)
	assert.Equal(t, "Host", common.HostHeader)
	assert.Equal(t, "Content-Type", common.ContentTypeHeader)
	assert.Equal(t, "Authorization", common.AuthorizationHeader)
}

func TestActivityPubContentTypeConstants(t *testing.T) {
	// Test that we have the correct ActivityPub content type constants
	assert.Equal(t, "application/activity+json", common.ContentTypeActivityPubJSON)
	assert.Equal(t, "application/ld+json", common.ContentTypeLDJSON)
	assert.Equal(t, "application/json", common.ContentTypeJSON)
}

func TestCORSHeadersIncludeActivityPubHeaders(t *testing.T) {
	// Test the GetCORSHeaders function includes all required ActivityPub headers
	headers := common.GetCORSHeaders()

	// Verify CORS configuration
	assert.Equal(t, "*", headers["Access-Control-Allow-Origin"])

	// Verify all ActivityPub-required headers are allowed
	allowedHeaders := headers["Access-Control-Allow-Headers"]
	requiredHeaders := []string{
		"Digest", "Signature", "Date", "Host",
		"Content-Type", "Authorization", "Accept",
		"User-Agent", "X-Forwarded-For", "X-Forwarded-Proto",
	}

	for _, header := range requiredHeaders {
		assert.Contains(t, allowedHeaders, header, "CORS must allow %s header for ActivityPub", header)
	}

	// Verify required HTTP methods are allowed
	allowedMethods := headers["Access-Control-Allow-Methods"]
	requiredMethods := []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "HEAD"}

	for _, method := range requiredMethods {
		assert.Contains(t, allowedMethods, method, "CORS must allow %s method", method)
	}
}

func TestWebFingerResourceValidationCompliance(t *testing.T) {
	// Test WebFinger resource validation per RFC 7033
	tests := []struct {
		resource    string
		valid       bool
		description string
	}{
		// Valid formats (based on current validation rules)
		{"acct:user@example.com", true, "basic valid acct URI"},
		{"acct:user_name@example.com", true, "valid with underscore in username"},
		{"acct:user123@example.com", true, "valid with numbers in username"},
		{"acct:User@EXAMPLE.COM", true, "valid with uppercase (should be handled)"},
		{"acct:user-name@example.com", true, "valid with hyphen in username"},

		// Invalid per current validation (though RFC 7033 might allow these)
		{"acct:user.name@example.com", false, "dots not allowed in current validation"},
		{"acct:user+tag@example.com", false, "plus not allowed in current validation"},

		// Invalid formats per RFC
		{"user@example.com", false, "missing acct: scheme"},
		{"acct:user", false, "missing domain part"},
		{"acct:@example.com", false, "missing username part"},
		{"", false, "empty resource"},
		{"acct:user@", false, "empty domain"},
		{"mailto:user@example.com", false, "wrong URI scheme"},
		{"acct:", false, "only scheme"},
		{"acct:user@domain.com@extra", false, "malformed with extra @"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			err := activitypub.ValidateWebfinger(tt.resource)
			if tt.valid {
				assert.NoError(t, err, "Resource %q should be valid: %s", tt.resource, tt.description)
			} else {
				assert.Error(t, err, "Resource %q should be invalid: %s", tt.resource, tt.description)
			}
		})
	}
}
