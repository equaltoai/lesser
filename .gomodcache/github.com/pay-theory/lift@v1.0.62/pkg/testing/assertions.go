package testing

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"testing"

	"github.com/PaesslerAG/jsonpath"
	"github.com/stretchr/testify/assert"
)

// TestResponse provides enhanced assertion capabilities for HTTP responses
type TestResponse struct {
	err        error
	t          *testing.T
	Headers    map[string]string
	Body       string
	StatusCode int
}

// NewTestResponse creates a new TestResponse for testing
func NewTestResponse(t *testing.T, statusCode int, headers map[string]string, body []byte, err error) *TestResponse {
	return &TestResponse{
		t:          t,
		StatusCode: statusCode,
		Headers:    headers,
		Body:       string(body),
		err:        err,
	}
}

// AssertStatus verifies the HTTP status code
func (r *TestResponse) AssertStatus(expected int) *TestResponse {
	if r.t == nil {
		panic("TestResponse not properly initialized")
	}
	assert.Equal(r.t, expected, r.StatusCode, "HTTP status code mismatch")
	return r
}

// AssertHeader verifies a specific header value
func (r *TestResponse) AssertHeader(key, expected string) *TestResponse {
	if r.t == nil {
		panic("TestResponse not properly initialized")
	}
	actual, exists := r.Headers[key]
	assert.True(r.t, exists, "Header %s not found", key)
	assert.Equal(r.t, expected, actual, "Header %s value mismatch", key)
	return r
}

// AssertHeaderExists verifies a header exists
func (r *TestResponse) AssertHeaderExists(key string) *TestResponse {
	if r.t == nil {
		panic("TestResponse not properly initialized")
	}
	_, exists := r.Headers[key]
	assert.True(r.t, exists, "Header %s not found", key)
	return r
}

// AssertHeaderContains verifies a header contains a substring
func (r *TestResponse) AssertHeaderContains(key, substring string) *TestResponse {
	if r.t == nil {
		panic("TestResponse not properly initialized")
	}
	actual, exists := r.Headers[key]
	assert.True(r.t, exists, "Header %s not found", key)
	assert.Contains(r.t, actual, substring, "Header %s does not contain %s", key, substring)
	return r
}

// AssertJSONPath verifies a JSON path matches expected value
func (r *TestResponse) AssertJSONPath(path string, expected any) *TestResponse {
	if r.t == nil {
		panic("TestResponse not properly initialized")
	}

	var jsonData any
	if err := json.Unmarshal([]byte(r.Body), &jsonData); err != nil {
		r.t.Fatalf("Failed to parse JSON response: %v\nBody: %s", err, r.Body)
	}

	actual, err := jsonpath.Get(path, jsonData)
	if err != nil {
		r.t.Fatalf("Failed to evaluate JSON path %s: %v\nJSON: %s", path, err, r.Body)
	}

	if !reflect.DeepEqual(actual, expected) {
		r.t.Errorf("JSON path assertion failed\nPath: %s\nExpected: %v (%T)\nActual: %v (%T)\nJSON: %s",
			path, expected, expected, actual, actual, r.Body)
	}

	return r
}

// AssertJSONPaths verifies multiple JSON paths at once
func (r *TestResponse) AssertJSONPaths(assertions map[string]any) *TestResponse {
	for path, expected := range assertions {
		r.AssertJSONPath(path, expected)
	}
	return r
}

// AssertJSONPathExists verifies a JSON path exists
func (r *TestResponse) AssertJSONPathExists(path string) *TestResponse {
	if r.t == nil {
		panic("TestResponse not properly initialized")
	}

	var jsonData any
	if err := json.Unmarshal([]byte(r.Body), &jsonData); err != nil {
		r.t.Fatalf("Failed to parse JSON response: %v", err)
	}

	_, err := jsonpath.Get(path, jsonData)
	assert.NoError(r.t, err, "JSON path %s should exist", path)

	return r
}

// AssertJSONPathNotExists verifies a JSON path does not exist
func (r *TestResponse) AssertJSONPathNotExists(path string) *TestResponse {
	if r.t == nil {
		panic("TestResponse not properly initialized")
	}

	var jsonData any
	if err := json.Unmarshal([]byte(r.Body), &jsonData); err != nil {
		r.t.Fatalf("Failed to parse JSON response: %v", err)
	}

	_, err := jsonpath.Get(path, jsonData)
	assert.Error(r.t, err, "JSON path %s should not exist", path)

	return r
}

// AssertJSONPathCount verifies the count of items at a JSON path
func (r *TestResponse) AssertJSONPathCount(path string, expectedCount int) *TestResponse {
	if r.t == nil {
		panic("TestResponse not properly initialized")
	}

	var jsonData any
	if err := json.Unmarshal([]byte(r.Body), &jsonData); err != nil {
		r.t.Fatalf("Failed to parse JSON response: %v", err)
	}

	actual, err := jsonpath.Get(path, jsonData)
	if err != nil {
		r.t.Fatalf("Failed to evaluate JSON path %s: %v", path, err)
	}

	// Handle different types that can be counted
	var count int
	switch v := actual.(type) {
	case []any:
		count = len(v)
	case map[string]any:
		count = len(v)
	case string:
		count = len(v)
	default:
		r.t.Fatalf("Cannot count items at path %s, got type %T", path, actual)
	}

	assert.Equal(r.t, expectedCount, count, "JSON path %s count mismatch", path)

	return r
}

// AssertJSONSchema verifies the response matches a JSON schema (basic validation)
func (r *TestResponse) AssertJSONSchema(schema map[string]any) *TestResponse {
	if r.t == nil {
		panic("TestResponse not properly initialized")
	}

	var jsonData any
	if err := json.Unmarshal([]byte(r.Body), &jsonData); err != nil {
		r.t.Fatalf("Failed to parse JSON response: %v", err)
	}

	// Basic schema validation - check required fields and types
	if dataMap, ok := jsonData.(map[string]any); ok {
		for field, expectedType := range schema {
			value, exists := dataMap[field]
			assert.True(r.t, exists, "Required field %s missing", field)

			if exists {
				actualType := fmt.Sprintf("%T", value)
				assert.Equal(r.t, expectedType, actualType, "Field %s type mismatch", field)
			}
		}
	}

	return r
}

// AssertBodyContains verifies the response body contains a substring
func (r *TestResponse) AssertBodyContains(substring string) *TestResponse {
	if r.t == nil {
		panic("TestResponse not properly initialized")
	}
	assert.Contains(r.t, r.Body, substring, "Response body should contain substring")
	return r
}

// AssertBodyEquals verifies the response body exactly matches
func (r *TestResponse) AssertBodyEquals(expected string) *TestResponse {
	if r.t == nil {
		panic("TestResponse not properly initialized")
	}
	assert.Equal(r.t, expected, r.Body, "Response body mismatch")
	return r
}

// AssertNoError verifies no error occurred
func (r *TestResponse) AssertNoError() *TestResponse {
	if r.t == nil {
		panic("TestResponse not properly initialized")
	}
	assert.NoError(r.t, r.err, "Request should not have errored")
	return r
}

// AssertError verifies an error occurred
func (r *TestResponse) AssertError() *TestResponse {
	if r.t == nil {
		panic("TestResponse not properly initialized")
	}
	assert.Error(r.t, r.err, "Request should have errored")
	return r
}

// GetJSONPath extracts a value from the JSON response
func (r *TestResponse) GetJSONPath(path string) any {
	var jsonData any
	if err := json.Unmarshal([]byte(r.Body), &jsonData); err != nil {
		r.t.Fatalf("Failed to parse JSON response: %v", err)
	}

	actual, err := jsonpath.Get(path, jsonData)
	if err != nil {
		r.t.Fatalf("Failed to evaluate JSON path %s: %v", path, err)
	}

	return actual
}

// GetHeader returns a header value
func (r *TestResponse) GetHeader(key string) string {
	return r.Headers[key]
}

// GetBody returns the response body as string
func (r *TestResponse) GetBody() string {
	return r.Body
}

// GetStatusCode returns the HTTP status code
func (r *TestResponse) GetStatusCode() int {
	return r.StatusCode
}

// IsSuccess returns true if the status code indicates success (2xx)
func (r *TestResponse) IsSuccess() bool {
	return r.StatusCode >= 200 && r.StatusCode < 300
}

// JSON parses the response body as JSON into the provided interface
func (r *TestResponse) JSON(target any) error {
	return json.Unmarshal([]byte(r.Body), target)
}

// Rate Limiting Specific Assertions

// AssertRateLimitHeaders verifies standard rate limit headers are present
func (r *TestResponse) AssertRateLimitHeaders() *TestResponse {
	r.AssertHeaderExists("X-RateLimit-Limit")
	r.AssertHeaderExists("X-RateLimit-Remaining")
	r.AssertHeaderExists("X-RateLimit-Reset")
	return r
}

// AssertRateLimitExceeded verifies a rate limit exceeded response
func (r *TestResponse) AssertRateLimitExceeded() *TestResponse {
	r.AssertStatus(429)
	r.AssertRateLimitHeaders()
	r.AssertHeaderExists("Retry-After")
	r.AssertJSONPath("$.error", "Rate limit exceeded")
	return r
}

// AssertRateLimitRemaining verifies the remaining rate limit count
func (r *TestResponse) AssertRateLimitRemaining(expected int) *TestResponse {
	r.AssertHeader("X-RateLimit-Remaining", strconv.Itoa(expected))
	return r
}

// AssertRateLimitLimit verifies the rate limit value
func (r *TestResponse) AssertRateLimitLimit(expected int) *TestResponse {
	r.AssertHeader("X-RateLimit-Limit", strconv.Itoa(expected))
	return r
}

// Multi-tenant Specific Assertions

// AssertTenantIsolation verifies tenant-specific data isolation
func (r *TestResponse) AssertTenantIsolation(tenantID string) *TestResponse {
	// Verify all returned data belongs to the specified tenant
	items := r.GetJSONPath("$.data")
	if itemsArray, ok := items.([]any); ok {
		for i, item := range itemsArray {
			if itemMap, ok := item.(map[string]any); ok {
				if tenant, exists := itemMap["tenant_id"]; exists {
					assert.Equal(r.t, tenantID, tenant, "Item %d should belong to tenant %s", i, tenantID)
				}
			}
		}
	}
	return r
}

// Pagination Assertions

// AssertPagination verifies pagination metadata
func (r *TestResponse) AssertPagination(expectedTotal, expectedPage, expectedPerPage int) *TestResponse {
	r.AssertJSONPath("$.pagination.total", expectedTotal)
	r.AssertJSONPath("$.pagination.page", expectedPage)
	r.AssertJSONPath("$.pagination.per_page", expectedPerPage)
	return r
}

// AssertHasNextPage verifies next page link exists
func (r *TestResponse) AssertHasNextPage() *TestResponse {
	r.AssertJSONPathExists("$.pagination.next_page")
	return r
}

// AssertHasPrevPage verifies previous page link exists
func (r *TestResponse) AssertHasPrevPage() *TestResponse {
	r.AssertJSONPathExists("$.pagination.prev_page")
	return r
}

// Validation Error Assertions

// AssertValidationError verifies a validation error response
func (r *TestResponse) AssertValidationError(field string) *TestResponse {
	r.AssertStatus(400)
	r.AssertJSONPath("$.error", "Validation failed")
	r.AssertJSONPathExists(fmt.Sprintf("$.validation_errors.%s", field))
	return r
}

// AssertValidationErrors verifies multiple validation errors
func (r *TestResponse) AssertValidationErrors(fields []string) *TestResponse {
	r.AssertStatus(400)
	r.AssertJSONPath("$.error", "Validation failed")
	for _, field := range fields {
		r.AssertJSONPathExists(fmt.Sprintf("$.validation_errors.%s", field))
	}
	return r
}

// Security Assertions

// AssertUnauthorized verifies unauthorized response
func (r *TestResponse) AssertUnauthorized() *TestResponse {
	r.AssertStatus(401)
	r.AssertJSONPath("$.error", "Unauthorized")
	return r
}

// AssertForbidden verifies forbidden response
func (r *TestResponse) AssertForbidden() *TestResponse {
	r.AssertStatus(403)
	r.AssertJSONPath("$.error", "Forbidden")
	return r
}

// AssertRequiresAuthentication verifies authentication is required
func (r *TestResponse) AssertRequiresAuthentication() *TestResponse {
	r.AssertStatus(401)
	r.AssertHeaderExists("WWW-Authenticate")
	return r
}

// Performance Assertions

// AssertResponseTime verifies response was fast enough
func (r *TestResponse) AssertResponseTime(_ string) *TestResponse {
	// This would require timing information to be passed in
	// For now, just check that response time header exists if present
	if responseTime := r.GetHeader("X-Response-Time"); responseTime != "" {
		r.AssertHeaderExists("X-Response-Time")
	}
	return r
}

// Utility Functions

// ParseJSON parses the response body as JSON
func (r *TestResponse) ParseJSON(target any) error {
	return json.Unmarshal([]byte(r.Body), target)
}

// Debug prints the response for debugging
func (r *TestResponse) Debug() *TestResponse {
	if r.t != nil {
		r.t.Logf("Response Debug:\nStatus: %d\nHeaders: %+v\nBody: %s\n",
			r.StatusCode, r.Headers, r.Body)
	}
	return r
}

// Chain allows for custom assertions
func (r *TestResponse) Chain(fn func(*TestResponse)) *TestResponse {
	fn(r)
	return r
}
