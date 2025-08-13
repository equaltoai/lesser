package testing

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/assert"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// HTTP method constants
const (
	methodGET    = "GET"
	methodPOST   = "POST"
	methodPUT    = "PUT"
	methodDELETE = "DELETE"
)

// Context Builders

// NewTestContext creates a basic test context
// Note: Use TestApp for HTTP testing, this is for lower-level context testing
func NewTestContext(_, _ string) *lift.Context {
	// For now, return a basic context
	// In practice, you'd use _ = TestApp.GET(), TestApp.POST(), etc.
	ctx := context.Background()
	liftCtx := &lift.Context{Context: ctx}
	return liftCtx
}

// NewTestContextWithBody creates a test context with a request body
// Note: Use TestApp for HTTP testing, this is for lower-level context testing
func NewTestContextWithBody(method, path string, _ interface{}) *lift.Context {
	return NewTestContext(method, path)
}

// NewTestContextWithHeaders creates a test context with headers
// Note: Use TestApp for HTTP testing, this is for lower-level context testing
func NewTestContextWithHeaders(method, path string, _ map[string]string) *lift.Context {
	return NewTestContext(method, path)
}

// Authentication Test Helpers

// CreateTestClaims creates test claims for different user types
func CreateTestClaims(username, userType string) *auth.EnhancedClaims {
	var scopes []string

	switch userType {
	case "admin":
		scopes = []string{"read", "write", "follow", "push", "admin:read", "admin:write", "admin:accounts", "admin:all"}
	case "moderator":
		scopes = []string{"read", "write", "follow", "push", "admin:read", "admin:accounts"}
	case "standard":
		scopes = []string{"read", "write", "follow", "push"}
	case "read-only":
		scopes = []string{"read"}
	case "bot":
		scopes = []string{"read", "write"}
	default:
		scopes = []string{"read", "write"}
	}

	return &auth.EnhancedClaims{
		Username:  username,
		Scopes:    scopes,
		ClientID:  "test-client",
		SessionID: fmt.Sprintf("test-session-%s", username),
		DeviceID:  fmt.Sprintf("test-device-%s", username),
	}
}

// CreateCustomClaims creates claims with custom scopes
func CreateCustomClaims(username string, scopes []string) *auth.EnhancedClaims {
	return &auth.EnhancedClaims{
		Username:  username,
		Scopes:    scopes,
		ClientID:  "test-client",
		SessionID: fmt.Sprintf("test-session-%s", username),
		DeviceID:  fmt.Sprintf("test-device-%s", username),
	}
}

// NewAuthenticatedContext creates a context with authentication set
func NewAuthenticatedContext(method, path, username, userType string) *lift.Context {
	ctx := NewTestContext(method, path)
	claims := CreateTestClaims(username, userType)

	// Set authentication context
	ctx.Set("claims", claims)
	ctx.Set("username", claims.Username)
	ctx.Set("session_id", claims.SessionID)
	ctx.Set("device_id", claims.DeviceID)

	return ctx
}

// NewTenantContext creates a context with tenant and authentication
func NewTenantContext(method, path, username, userType, tenantID string) *lift.Context {
	ctx := NewAuthenticatedContext(method, path, username, userType)
	ctx.Request.Headers["X-Tenant-ID"] = tenantID
	ctx.Set("tenant_id", tenantID)
	return ctx
}

// Token Generators

// GenerateTestToken creates a test bearer token (mock implementation)
func GenerateTestToken(username, userType string) string {
	return fmt.Sprintf("test-token-%s-%s-%d", username, userType, time.Now().Unix())
}

// GenerateExpiredToken creates an expired test token
func GenerateExpiredToken(username string) string {
	return fmt.Sprintf("expired-token-%s", username)
}

// GenerateAdminToken creates an admin test token
func GenerateAdminToken(username string) string {
	return GenerateTestToken(username, "admin")
}

// Test Assertion Helpers

// AssertSuccessResponse verifies a successful response
func AssertSuccessResponse(t *testing.T, response *TestResponse) {
	t.Helper()
	assert.True(t, response.IsSuccess(), "Expected successful response, got %d", response.StatusCode)
	assert.NotEmpty(t, response.Body, "Response body should not be empty")
}

// AssertErrorResponse verifies an error response
func AssertErrorResponse(t *testing.T, response *TestResponse, expectedStatus int, expectedMessage string) {
	t.Helper()
	assert.Equal(t, expectedStatus, response.StatusCode, "Unexpected status code")
	if expectedMessage != "" {
		assert.Contains(t, response.Body, expectedMessage, "Response should contain expected message")
	}
}

// AssertJSONResponse verifies a JSON response structure
func AssertJSONResponse(t *testing.T, response *TestResponse, _ interface{}) {
	t.Helper()
	assert.True(t, response.IsSuccess(), "Response should be successful")

	var actual interface{}
	err := response.JSON(&actual)
	assert.NoError(t, err, "Response should be valid JSON")
}

// AssertAuthenticationRequired verifies that a handler requires authentication
func AssertAuthenticationRequired(t *testing.T, app *TestApp, method, path string) {
	t.Helper()

	var response *TestResponse
	switch method {
	case methodGET:
		response = app.GET(path)
	case methodPOST:
		response = app.POST(path, nil)
	case methodPUT:
		response = app.PUT(path, nil)
	case methodDELETE:
		response = app.DELETE(path)
	default:
		t.Fatalf("Unsupported method: %s", method)
	}

	assert.Equal(t, 401, response.StatusCode, "Should require authentication")
}

// AssertScopeRequired verifies that a handler requires specific scope
func AssertScopeRequired(t *testing.T, app *TestApp, method, path, _ string) {
	t.Helper()

	// Test with user that doesn't have the required scope
	userWithoutScope := app.WithHeader("Authorization", "Bearer "+GenerateTestToken("user", "standard"))

	var response *TestResponse
	switch method {
	case methodGET:
		response = userWithoutScope.GET(path)
	case methodPOST:
		response = userWithoutScope.POST(path, nil)
	case methodPUT:
		response = userWithoutScope.PUT(path, nil)
	case methodDELETE:
		response = userWithoutScope.DELETE(path)
	default:
		t.Fatalf("Unsupported method: %s", method)
	}

	assert.Equal(t, 403, response.StatusCode, "Should require specific scope")
}

// Event Testing Helpers

// CreateSQSEvent creates a test SQS event
func CreateSQSEvent(messageBody string) map[string]interface{} {
	return map[string]interface{}{
		"Records": []map[string]interface{}{
			{
				"eventSource":   "aws:sqs",
				"body":          messageBody,
				"messageId":     "test-message-id",
				"receiptHandle": "test-receipt-handle",
			},
		},
	}
}

// CreateDynamoDBStreamEvent creates a test DynamoDB stream event
func CreateDynamoDBStreamEvent(eventName string, item map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"Records": []map[string]interface{}{
			{
				"eventSource": "aws:dynamodb",
				"eventName":   eventName,
				"dynamodb": map[string]interface{}{
					"NewImage": item,
					"Keys": map[string]interface{}{
						"id": map[string]interface{}{
							"S": "test-id",
						},
					},
				},
			},
		},
	}
}

// CreateAPIGatewayEvent creates a test API Gateway event
func CreateAPIGatewayEvent(method, path string, body string, headers map[string]string) map[string]interface{} {
	if headers == nil {
		headers = make(map[string]string)
	}

	return map[string]interface{}{
		"httpMethod": method,
		"path":       path,
		"body":       body,
		"headers":    headers,
		"requestContext": map[string]interface{}{
			"requestId": "test-request-id",
		},
	}
}

// Performance Testing Helpers

// MeasureExecutionTime measures the execution time of a function
func MeasureExecutionTime(fn func()) time.Duration {
	start := time.Now()
	fn()
	return time.Since(start)
}

// AssertExecutionTime verifies that execution time is within expected bounds
func AssertExecutionTime(t *testing.T, fn func(), maxDuration time.Duration) {
	t.Helper()
	duration := MeasureExecutionTime(fn)
	assert.LessOrEqual(t, duration.Nanoseconds(), maxDuration.Nanoseconds(),
		"Execution took %v, expected less than %v", duration, maxDuration)
}

// Test Data Factories

// TestUser represents a test user structure
type TestUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Name     string `json:"name"`
}

// NewTestUser creates a test user
func NewTestUser(username string) *TestUser {
	return &TestUser{
		ID:       fmt.Sprintf("user-%s", username),
		Username: username,
		Email:    fmt.Sprintf("%s@example.com", username),
		Name:     fmt.Sprintf("Test %s", cases.Title(language.English).String(username)),
	}
}

// CreateUserRequest represents a user creation request
type CreateUserRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Name     string `json:"name"`
}

// NewCreateUserRequest creates a user creation request
func NewCreateUserRequest(username string) *CreateUserRequest {
	return &CreateUserRequest{
		Username: username,
		Email:    fmt.Sprintf("%s@example.com", username),
		Name:     fmt.Sprintf("Test %s", cases.Title(language.English).String(username)),
	}
}

// Test Environment Helpers

// TestEnvironment holds test environment state
type TestEnvironment struct {
	App        *TestApp
	TestData   map[string]interface{}
	TeardownFn func()
}

// NewTestEnvironment creates a new test environment
func NewTestEnvironment() *TestEnvironment {
	return &TestEnvironment{
		App:        NewTestApp(),
		TestData:   make(map[string]interface{}),
		TeardownFn: func() {}, // No-op by default
	}
}

// WithTeardown adds a teardown function
func (te *TestEnvironment) WithTeardown(fn func()) *TestEnvironment {
	te.TeardownFn = fn
	return te
}

// Cleanup runs the teardown function
func (te *TestEnvironment) Cleanup() {
	if te.TeardownFn != nil {
		te.TeardownFn()
	}
}

// SetTestData stores test data for later use
func (te *TestEnvironment) SetTestData(key string, value interface{}) {
	te.TestData[key] = value
}

// GetTestData retrieves stored test data
func (te *TestEnvironment) GetTestData(key string) interface{} {
	return te.TestData[key]
}
