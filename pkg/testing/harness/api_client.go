// Package harness provides API client utilities for integration testing
package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/stretchr/testify/require"
)

// APIClient provides a testing-focused HTTP client for making requests to the Lesser API
type APIClient struct {
	BaseURL    string
	HTTPClient *http.Client
	Token      string
	t          *testing.T
}

// NewAPIClient creates a new API client for testing
func NewAPIClient(t *testing.T, baseURL string) *APIClient {
	return &APIClient{
		BaseURL: strings.TrimSuffix(baseURL, "/"),
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		t: t,
	}
}

// WithToken sets the authorization token for API requests
func (c *APIClient) WithToken(token string) *APIClient {
	c.Token = token
	return c
}

// APIResponse represents a standardized API response for testing
type APIResponse struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
	Response   *http.Response
}

// JSON unmarshals the response body into the provided interface
func (r *APIResponse) JSON(v interface{}) error {
	return json.Unmarshal(r.Body, v)
}

// String returns the response body as a string
func (r *APIResponse) String() string {
	return string(r.Body)
}

// GET makes a GET request to the specified path
func (c *APIClient) GET(path string, params ...map[string]string) *APIResponse {
	return c.makeRequest("GET", path, nil, params...)
}

// POST makes a POST request with JSON body
func (c *APIClient) POST(path string, body interface{}, params ...map[string]string) *APIResponse {
	return c.makeRequest("POST", path, body, params...)
}

// PUT makes a PUT request with JSON body
func (c *APIClient) PUT(path string, body interface{}, params ...map[string]string) *APIResponse {
	return c.makeRequest("PUT", path, body, params...)
}

// DELETE makes a DELETE request
func (c *APIClient) DELETE(path string, params ...map[string]string) *APIResponse {
	return c.makeRequest("DELETE", path, nil, params...)
}

// PATCH makes a PATCH request with JSON body
func (c *APIClient) PATCH(path string, body interface{}, params ...map[string]string) *APIResponse {
	return c.makeRequest("PATCH", path, body, params...)
}

// makeRequest is the core request method that handles all HTTP operations
func (c *APIClient) makeRequest(method, path string, body interface{}, params ...map[string]string) *APIResponse {
	// Build URL with query parameters
	fullURL := c.BaseURL + path
	if len(params) > 0 && params[0] != nil {
		u, err := url.Parse(fullURL)
		require.NoError(c.t, err)

		query := u.Query()
		for key, value := range params[0] {
			query.Set(key, value)
		}
		u.RawQuery = query.Encode()
		fullURL = u.String()
	}

	// Prepare request body
	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		require.NoError(c.t, err)
		bodyReader = bytes.NewReader(bodyBytes)
	}

	// Create request
	req, err := http.NewRequestWithContext(context.Background(), method, fullURL, bodyReader)
	require.NoError(c.t, err)

	// Set headers
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	// Set User-Agent for testing
	req.Header.Set("User-Agent", "Lesser-Test-Client/1.0")

	// Make request
	resp, err := c.HTTPClient.Do(req)
	require.NoError(c.t, err)
	defer func() { _ = resp.Body.Close() }()

	// Read response body
	const maxResponseBodyBytes = int64(8 * 1024 * 1024) // 8MB
	respBody, truncated, err := common.ReadUntrustedHTTPResponseBody(resp.Body, maxResponseBodyBytes)
	require.NoError(c.t, err)
	require.False(c.t, truncated, "response body exceeded limit: %d bytes", maxResponseBodyBytes)

	return &APIResponse{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       respBody,
		Response:   resp,
	}
}

// MastodonAPIClient provides Mastodon-specific API methods
type MastodonAPIClient struct {
	*APIClient
}

// NewMastodonAPIClient creates a client configured for Mastodon API endpoints
func NewMastodonAPIClient(t *testing.T, baseURL string) *MastodonAPIClient {
	client := NewAPIClient(t, baseURL)
	return &MastodonAPIClient{APIClient: client}
}

// VerifyCredentials verifies the current user's credentials
func (c *MastodonAPIClient) VerifyCredentials() *APIResponse {
	return c.GET("/api/v1/accounts/verify_credentials")
}

// GetAccount retrieves account information by ID
func (c *MastodonAPIClient) GetAccount(accountID string) *APIResponse {
	return c.GET(fmt.Sprintf("/api/v1/accounts/%s", accountID))
}

// GetHomeTimeline retrieves the home timeline
func (c *MastodonAPIClient) GetHomeTimeline(params map[string]string) *APIResponse {
	return c.GET("/api/v1/timelines/home", params)
}

// CreateStatus creates a new status/post
func (c *MastodonAPIClient) CreateStatus(status map[string]interface{}) *APIResponse {
	return c.POST("/api/v1/statuses", status)
}

// GetStatus retrieves a status by ID
func (c *MastodonAPIClient) GetStatus(statusID string) *APIResponse {
	return c.GET(fmt.Sprintf("/api/v1/statuses/%s", statusID))
}

// DeleteStatus deletes a status by ID
func (c *MastodonAPIClient) DeleteStatus(statusID string) *APIResponse {
	return c.DELETE(fmt.Sprintf("/api/v1/statuses/%s", statusID))
}

// FollowAccount follows an account
func (c *MastodonAPIClient) FollowAccount(accountID string) *APIResponse {
	return c.POST(fmt.Sprintf("/api/v1/accounts/%s/follow", accountID), nil)
}

// UnfollowAccount unfollows an account
func (c *MastodonAPIClient) UnfollowAccount(accountID string) *APIResponse {
	return c.POST(fmt.Sprintf("/api/v1/accounts/%s/unfollow", accountID), nil)
}

// GetNotifications retrieves notifications
func (c *MastodonAPIClient) GetNotifications(params map[string]string) *APIResponse {
	return c.GET("/api/v1/notifications", params)
}

// ActivityPubClient provides ActivityPub-specific API methods
type ActivityPubClient struct {
	*APIClient
}

// NewActivityPubClient creates a client configured for ActivityPub endpoints
func NewActivityPubClient(t *testing.T, baseURL string) *ActivityPubClient {
	client := NewAPIClient(t, baseURL)
	return &ActivityPubClient{APIClient: client}
}

// GetActor retrieves an ActivityPub actor
func (c *ActivityPubClient) GetActor(username string) *APIResponse {
	resp := c.GET(fmt.Sprintf("/users/%s", username))
	// Validate ActivityPub content type
	contentType := resp.Headers.Get("Content-Type")
	require.Contains(c.t, contentType, "application/")
	return resp
}

// GetOutbox retrieves an actor's outbox
func (c *ActivityPubClient) GetOutbox(username string, params map[string]string) *APIResponse {
	return c.GET(fmt.Sprintf("/users/%s/outbox", username), params)
}

// GetInbox retrieves an actor's inbox (requires authentication)
func (c *ActivityPubClient) GetInbox(username string, params map[string]string) *APIResponse {
	return c.GET(fmt.Sprintf("/users/%s/inbox", username), params)
}

// PostToInbox posts an activity to an inbox
func (c *ActivityPubClient) PostToInbox(username string, activity interface{}) *APIResponse {
	resp := c.POST(fmt.Sprintf("/users/%s/inbox", username), activity)
	return resp
}

// GetFollowing retrieves who an actor is following
func (c *ActivityPubClient) GetFollowing(username string, params map[string]string) *APIResponse {
	return c.GET(fmt.Sprintf("/users/%s/following", username), params)
}

// GetFollowers retrieves an actor's followers
func (c *ActivityPubClient) GetFollowers(username string, params map[string]string) *APIResponse {
	return c.GET(fmt.Sprintf("/users/%s/followers", username), params)
}

// Webfinger performs a WebFinger lookup
func (c *ActivityPubClient) Webfinger(resource string) *APIResponse {
	params := map[string]string{"resource": resource}
	return c.GET("/.well-known/webfinger", params)
}

// GetNodeInfo retrieves NodeInfo
func (c *ActivityPubClient) GetNodeInfo() *APIResponse {
	return c.GET("/.well-known/nodeinfo")
}

// TestAssertions provides common test assertions for API responses
type TestAssertions struct {
	t *testing.T
}

// NewTestAssertions creates a new test assertions helper
func NewTestAssertions(t *testing.T) *TestAssertions {
	return &TestAssertions{t: t}
}

// AssertStatusCode asserts the response has the expected status code
func (a *TestAssertions) AssertStatusCode(resp *APIResponse, expected int) {
	require.Equal(a.t, expected, resp.StatusCode,
		"Expected status %d, got %d. Response: %s", expected, resp.StatusCode, resp.String())
}

// AssertJSONResponse asserts the response is valid JSON and unmarshals it
func (a *TestAssertions) AssertJSONResponse(resp *APIResponse, v interface{}) {
	contentType := resp.Headers.Get("Content-Type")
	require.Contains(a.t, contentType, "application/json",
		"Expected JSON content type, got %s", contentType)

	err := resp.JSON(v)
	require.NoError(a.t, err, "Failed to parse JSON response: %s", resp.String())
}

// AssertActivityPubResponse asserts the response is a valid ActivityPub response
func (a *TestAssertions) AssertActivityPubResponse(resp *APIResponse, v interface{}) {
	contentType := resp.Headers.Get("Content-Type")
	require.True(a.t,
		strings.Contains(contentType, "application/activity+json") ||
			strings.Contains(contentType, "application/ld+json"),
		"Expected ActivityPub content type, got %s", contentType)

	err := resp.JSON(v)
	require.NoError(a.t, err, "Failed to parse ActivityPub response: %s", resp.String())
}

// AssertErrorResponse asserts the response contains an error with expected properties
func (a *TestAssertions) AssertErrorResponse(resp *APIResponse, expectedStatus int, expectedError string) {
	a.AssertStatusCode(resp, expectedStatus)

	var errorResp map[string]interface{}
	a.AssertJSONResponse(resp, &errorResp)

	if expectedError != "" {
		require.Contains(a.t, fmt.Sprintf("%v", errorResp), expectedError,
			"Expected error message containing '%s', got response: %s", expectedError, resp.String())
	}
}
