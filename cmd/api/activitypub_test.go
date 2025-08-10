package main

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	apiLift "github.com/equaltoai/lesser/cmd/api/lift"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/assert"
)

func TestWebFingerResponseStructure(t *testing.T) {
	// Test WebFinger response structure compliance
	response := apiLift.WebFingerResponse{
		Subject: "acct:testuser@example.com",
		Aliases: []string{"https://example.com/users/testuser"},
		Links: []apiLift.WebFingerLink{
			{
				Rel:  "self",
				Type: "application/activity+json",
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

	// Verify required WebFinger fields
	assert.Contains(t, parsed, "subject")
	assert.Contains(t, parsed, "links")
	assert.Equal(t, "acct:testuser@example.com", parsed["subject"])

	// Verify links structure
	links := parsed["links"].([]interface{})
	assert.Len(t, links, 1)

	link := links[0].(map[string]interface{})
	assert.Equal(t, "self", link["rel"])
	assert.Equal(t, "application/activity+json", link["type"])
}

func TestNodeInfoResponseStructure(t *testing.T) {
	// Test NodeInfo response structure compliance
	response := apiLift.NodeInfo{
		Version: "2.0",
		Software: apiLift.NodeInfoSoftware{
			Name:    "lesser",
			Version: "0.1.0",
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
			LocalPosts: 500,
		},
		Metadata: map[string]any{
			"nodeName": "Test Instance",
		},
	}

	// Serialize to JSON to verify structure
	jsonData, err := json.Marshal(response)
	assert.NoError(t, err)

	// Parse back to verify all required fields are present
	var parsed map[string]interface{}
	err = json.Unmarshal(jsonData, &parsed)
	assert.NoError(t, err)

	// Verify required NodeInfo 2.0 fields
	assert.Equal(t, "2.0", parsed["version"])
	assert.Contains(t, parsed, "software")
	assert.Contains(t, parsed, "protocols")
	assert.Contains(t, parsed, "services")
	assert.Contains(t, parsed, "openRegistrations")
	assert.Contains(t, parsed, "usage")

	// Verify software section
	software := parsed["software"].(map[string]interface{})
	assert.Equal(t, "lesser", software["name"])
	assert.Contains(t, software, "version")

	// Verify protocols contain ActivityPub
	protocols := parsed["protocols"].([]interface{})
	assert.Contains(t, protocols, "activitypub")

	// Verify usage statistics structure
	usage := parsed["usage"].(map[string]interface{})
	assert.Contains(t, usage, "users")
	users := usage["users"].(map[string]interface{})
	assert.Contains(t, users, "total")
	assert.Contains(t, users, "activeMonth")
	assert.Contains(t, users, "activeHalfyear")
}

func TestNodeInfoWellKnownStructure(t *testing.T) {
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

	// Verify links structure
	assert.Contains(t, parsed, "links")
	links := parsed["links"].([]interface{})
	assert.Len(t, links, 1)

	link := links[0].(map[string]interface{})
	assert.Equal(t, "http://nodeinfo.diaspora.software/ns/schema/2.0", link["rel"])
	assert.Contains(t, link["href"], "nodeinfo/2.0")
}

func TestCORSMiddleware(t *testing.T) {
	// Create CORS middleware
	corsMiddleware := createCORSMiddleware()

	// Create mock handler
	mockHandler := &activityPubMockHandler{}

	// Create handler with middleware
	handler := corsMiddleware(mockHandler)

	// Create context with headers
	ctx := &lift.Context{
		Context: context.Background(),
		Request: &lift.Request{
			Method: "POST",
			Path:   "/inbox",
			Headers: map[string]string{
				common.DigestHeader:      "SHA-256=abcd1234",
				common.SignatureHeader:   "keyId=\"test\",signature=\"test\"",
				common.DateHeader:        "Wed, 21 Oct 2015 07:28:00 GMT",
				common.ContentTypeHeader: common.ContentTypeActivityPubJSON,
			},
		},
		Response: &lift.Response{
			StatusCode: http.StatusOK,
			Headers:    make(map[string]string),
		},
	}

	// Call handler
	err := handler.Handle(ctx)
	assert.NoError(t, err)

	// Verify CORS headers are set correctly
	corsHeaders := ctx.Response.Headers
	assert.Equal(t, "*", corsHeaders["Access-Control-Allow-Origin"])

	// Verify ActivityPub headers are allowed
	allowedHeaders := corsHeaders["Access-Control-Allow-Headers"]
	assert.Contains(t, allowedHeaders, "Digest")
	assert.Contains(t, allowedHeaders, "Signature")
	assert.Contains(t, allowedHeaders, "Date")
	assert.Contains(t, allowedHeaders, "Content-Type")
	assert.Contains(t, allowedHeaders, "Authorization")

	// Verify methods are allowed
	allowedMethods := corsHeaders["Access-Control-Allow-Methods"]
	assert.Contains(t, allowedMethods, "POST")
	assert.Contains(t, allowedMethods, "GET")
	assert.Contains(t, allowedMethods, "OPTIONS")

	// Verify mock handler was called
	assert.True(t, mockHandler.called)
}

func TestCORSOptionsRequest(t *testing.T) {
	// Create CORS middleware
	corsMiddleware := createCORSMiddleware()

	// Create mock handler
	mockHandler := &activityPubMockHandler{}

	// Create handler with middleware
	handler := corsMiddleware(mockHandler)

	// Create OPTIONS request context
	ctx := &lift.Context{
		Context: context.Background(),
		Request: &lift.Request{
			Method: "OPTIONS",
			Path:   "/inbox",
		},
		Response: &lift.Response{
			StatusCode: http.StatusOK,
			Headers:    make(map[string]string),
		},
	}

	// Call handler
	err := handler.Handle(ctx)
	assert.NoError(t, err)

	// Verify OPTIONS was handled (returns 200 without calling next handler)
	assert.Equal(t, http.StatusOK, ctx.Response.StatusCode)
	assert.False(t, mockHandler.called, "OPTIONS request should not call next handler")

	// Verify CORS headers are set
	assert.Equal(t, "*", ctx.Response.Headers["Access-Control-Allow-Origin"])
	assert.Contains(t, ctx.Response.Headers["Access-Control-Allow-Headers"], "Digest")
	assert.Contains(t, ctx.Response.Headers["Access-Control-Allow-Headers"], "Signature")
	assert.Contains(t, ctx.Response.Headers["Access-Control-Allow-Headers"], "Date")
	assert.Contains(t, ctx.Response.Headers["Access-Control-Allow-Methods"], "POST")
}

func TestActivityPubContentTypes(t *testing.T) {
	// Test that we have the correct content type constants
	assert.Equal(t, "application/activity+json", common.ContentTypeActivityPubJSON)
	assert.Equal(t, "application/ld+json", common.ContentTypeLDJSON)
	assert.Equal(t, "application/json", common.ContentTypeJSON)
}

func TestActivityPubHeaders(t *testing.T) {
	// Test that we have the correct header constants
	assert.Equal(t, "Digest", common.DigestHeader)
	assert.Equal(t, "Signature", common.SignatureHeader)
	assert.Equal(t, "Date", common.DateHeader)
	assert.Equal(t, "Host", common.HostHeader)
}

func TestWebFingerResourceValidation(t *testing.T) {
	tests := []struct {
		resource string
		valid    bool
		name     string
	}{
		{"acct:user@example.com", true, "basic valid resource"},
		{"acct:user.name@example.com", true, "valid resource with dots"},
		{"acct:user+tag@example.com", true, "valid resource with plus"},
		{"acct:user_name@example.com", true, "valid resource with underscore"},
		{"user@example.com", false, "missing acct: prefix"},
		{"acct:user", false, "missing domain"},
		{"acct:@example.com", false, "missing username"},
		{"", false, "empty resource"},
		{"acct:user@", false, "missing domain part"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := activitypub.ValidateWebfinger(tt.resource)
			if tt.valid {
				assert.NoError(t, err, "Expected valid WebFinger resource")
			} else {
				assert.Error(t, err, "Expected invalid WebFinger resource")
			}
		})
	}
}

func TestCommonHeadersFunction(t *testing.T) {
	// Test the GetCORSHeaders function includes ActivityPub headers
	headers := common.GetCORSHeaders()

	assert.Equal(t, "*", headers["Access-Control-Allow-Origin"])

	allowedHeaders := headers["Access-Control-Allow-Headers"]
	assert.Contains(t, allowedHeaders, "Digest")
	assert.Contains(t, allowedHeaders, "Signature")
	assert.Contains(t, allowedHeaders, "Date")
	assert.Contains(t, allowedHeaders, "Content-Type")
	assert.Contains(t, allowedHeaders, "Authorization")

	allowedMethods := headers["Access-Control-Allow-Methods"]
	assert.Contains(t, allowedMethods, "GET")
	assert.Contains(t, allowedMethods, "POST")
	assert.Contains(t, allowedMethods, "PUT")
	assert.Contains(t, allowedMethods, "DELETE")
	assert.Contains(t, allowedMethods, "OPTIONS")
}

// Mock handler for testing ActivityPub endpoints
type activityPubMockHandler struct {
	called bool
}

func (m *activityPubMockHandler) Handle(_ *lift.Context) error {
	m.called = true
	return nil
}
