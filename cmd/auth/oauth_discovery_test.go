package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/testing/mocks"
)

// TestOAuthDiscoveryEndpoint tests the OAuth 2.0 Authorization Server Metadata endpoint
func TestOAuthDiscoveryEndpoint(t *testing.T) {
	tests := []struct {
		name         string
		domain       string
		headers      map[string]string
		expectedIssuer string
	}{
		{
			name:   "with_configured_domain",
			domain: "example.com",
			headers: map[string]string{
				"Host": "example.com",
			},
			expectedIssuer: "https://example.com",
		},
		{
			name:   "with_forwarded_proto_http",
			domain: "",
			headers: map[string]string{
				"Host":               "api.example.com",
				"X-Forwarded-Proto":  "http",
			},
			expectedIssuer: "http://api.example.com",
		},
		{
			name:   "with_forwarded_proto_https",
			domain: "",
			headers: map[string]string{
				"Host":               "api.example.com",
				"X-Forwarded-Proto":  "https",
			},
			expectedIssuer: "https://api.example.com",
		},
		{
			name:   "localhost_fallback",
			domain: "",
			headers: map[string]string{},
			expectedIssuer: "http://localhost:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock repositories and config
			mockRepos := new(mocks.MockRepositoryStorage)
			cfg := &config.Config{
				Domain: tt.domain,
			}
			
			// Create handler with mocked dependencies
			handler := &AuthHandler{
				repos:  mockRepos,
				logger: common.Logger(),
				cfg:    cfg,
			}

			// Create test context
			testCtx := &lift.Context{
				Context: context.Background(),
				Request: &lift.Request{
					Method:  "GET",
					Path:    "/.well-known/oauth-authorization-server",
					Headers: tt.headers,
				},
				Response: &lift.Response{
					StatusCode: 200,
					Headers:    make(map[string]string),
				},
			}

			// Call the handler
			err := handler.handleOAuthDiscovery(testCtx)
			require.NoError(t, err)

			// Parse the response
			var metadata map[string]interface{}
			
			// The Lift framework sets the response body as the actual object, not JSON bytes
			if bodyBytes, ok := testCtx.Response.Body.([]byte); ok {
				err = json.Unmarshal(bodyBytes, &metadata)
				require.NoError(t, err)
			} else {
				// Response body is already the JSON object
				metadata, ok = testCtx.Response.Body.(map[string]interface{})
				require.True(t, ok, "Response body should be map[string]interface{} or []byte, got %T", testCtx.Response.Body)
			}

			// Verify essential OAuth 2.0 metadata fields
			assert.Equal(t, tt.expectedIssuer, metadata["issuer"])
			assert.Equal(t, tt.expectedIssuer+"/oauth/authorize", metadata["authorization_endpoint"])
			assert.Equal(t, tt.expectedIssuer+"/oauth/token", metadata["token_endpoint"])
			assert.Equal(t, tt.expectedIssuer+"/oauth/revoke", metadata["revocation_endpoint"])

			// Verify supported features
			assert.Contains(t, metadata["scopes_supported"], "read")
			assert.Contains(t, metadata["scopes_supported"], "write")
			assert.Contains(t, metadata["response_types_supported"], "code")
			assert.Contains(t, metadata["grant_types_supported"], "authorization_code")
			assert.Contains(t, metadata["code_challenge_methods_supported"], "S256")

			// Verify security-related metadata
			authMethods := metadata["token_endpoint_auth_methods_supported"].([]string)
			assert.Contains(t, authMethods, "client_secret_post")
			assert.Contains(t, authMethods, "client_secret_basic")

			// Verify response headers
			assert.Equal(t, "public, max-age=3600", testCtx.Response.Headers["Cache-Control"])
			assert.Equal(t, "application/json", testCtx.Response.Headers["Content-Type"])

			t.Logf("OAuth discovery metadata validated for issuer: %s", tt.expectedIssuer)
		})
	}
}

// TestOAuthDiscoveryCompliance tests compliance with RFC 8414
func TestOAuthDiscoveryCompliance(t *testing.T) {
	handler := &AuthHandler{
		repos:  new(mocks.MockRepositoryStorage),
		logger: common.Logger(),
		cfg:    &config.Config{Domain: "example.com"},
	}

	testCtx := &lift.Context{
		Context: context.Background(),
		Request: &lift.Request{
			Method:  "GET",
			Path:    "/.well-known/oauth-authorization-server",
			Headers: map[string]string{"Host": "example.com"},
		},
		Response: &lift.Response{
			StatusCode: 200,
			Headers:    make(map[string]string),
		},
	}

	err := handler.handleOAuthDiscovery(testCtx)
	require.NoError(t, err)

	var metadata map[string]interface{}
	// Handle response body (could be []byte or already parsed object)
	if bodyBytes, ok := testCtx.Response.Body.([]byte); ok {
		err = json.Unmarshal(bodyBytes, &metadata)
		require.NoError(t, err)
	} else {
		metadata, ok = testCtx.Response.Body.(map[string]interface{})
		require.True(t, ok, "Response body should be map[string]interface{} or []byte, got %T", testCtx.Response.Body)
	}

	// Required fields according to RFC 8414
	requiredFields := []string{
		"issuer",
		"authorization_endpoint",
		"token_endpoint",
		"response_types_supported",
		"subject_types_supported",
	}

	for _, field := range requiredFields {
		assert.Contains(t, metadata, field, "RFC 8414 requires field: %s", field)
		assert.NotEmpty(t, metadata[field], "RFC 8414 field %s cannot be empty", field)
	}

	// Verify PKCE support (RFC 7636)
	assert.Contains(t, metadata, "code_challenge_methods_supported")
	challengeMethods := metadata["code_challenge_methods_supported"].([]string)
	assert.Contains(t, challengeMethods, "S256", "PKCE S256 method must be supported")

	// Verify proper issuer format (must be HTTPS URL)
	issuer := metadata["issuer"].(string)
	assert.Contains(t, issuer, "https://", "Issuer must use HTTPS")

	t.Log("OAuth 2.0 Authorization Server Metadata is RFC 8414 compliant")
}

// TestOAuthDiscoveryMastodonCompatibility tests compatibility with Mastodon clients
func TestOAuthDiscoveryMastodonCompatibility(t *testing.T) {
	handler := &AuthHandler{
		repos:  new(mocks.MockRepositoryStorage),
		logger: common.Logger(),
		cfg:    &config.Config{Domain: "mastodon.example"},
	}

	testCtx := &lift.Context{
		Context: context.Background(),
		Request: &lift.Request{
			Method:  "GET",
			Path:    "/.well-known/oauth-authorization-server",
			Headers: map[string]string{"Host": "mastodon.example"},
		},
		Response: &lift.Response{
			StatusCode: 200,
			Headers:    make(map[string]string),
		},
	}

	err := handler.handleOAuthDiscovery(testCtx)
	require.NoError(t, err)

	var metadata map[string]interface{}
	// Handle response body (could be []byte or already parsed object)
	if bodyBytes, ok := testCtx.Response.Body.([]byte); ok {
		err = json.Unmarshal(bodyBytes, &metadata)
		require.NoError(t, err)
	} else {
		metadata, ok = testCtx.Response.Body.(map[string]interface{})
		require.True(t, ok, "Response body should be map[string]interface{} or []byte, got %T", testCtx.Response.Body)
	}

	// Verify Mastodon-compatible scopes
	scopes := metadata["scopes_supported"].([]string)
	mastodonScopes := []string{"read", "write", "follow", "push"}
	for _, scope := range mastodonScopes {
		assert.Contains(t, scopes, scope, "Mastodon compatibility requires scope: %s", scope)
	}

	// Verify only authorization_code grant type (Mastodon pattern)
	grantTypes := metadata["grant_types_supported"].([]string)
	assert.Equal(t, []string{"authorization_code"}, grantTypes)

	// Verify only code response type (Mastodon pattern)
	responseTypes := metadata["response_types_supported"].([]string)
	assert.Equal(t, []string{"code"}, responseTypes)

	// Verify PKCE S256 support (required by modern Mastodon)
	challengeMethods := metadata["code_challenge_methods_supported"].([]string)
	assert.Contains(t, challengeMethods, "S256")

	t.Log("OAuth discovery endpoint is compatible with Mastodon clients")
}