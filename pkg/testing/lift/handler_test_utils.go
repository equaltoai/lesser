// Package lift provides testing utilities and test cases for Lift framework handler validation.
package lift

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

	"github.com/aws/aws-lambda-go/events"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/assert"
)

// HandlerTestCase defines a test case for Lift handlers
type HandlerTestCase struct {
	Name           string
	Method         string
	Path           string
	Headers        map[string]string
	Body           interface{}
	QueryParams    map[string]string
	PathParams     map[string]string
	TenantID       string
	ExpectedStatus int
	ExpectedBody   interface{}
	ExpectedError  string
	SetupFunc      func()
	CleanupFunc    func()
}

// HandlerTestSuite provides utilities for testing Lift handlers
type HandlerTestSuite struct {
	t        *testing.T
	app      *lift.App
	contexts []*lift.Context
}

// NewHandlerTestSuite creates a new handler test suite
func NewHandlerTestSuite(t *testing.T, app *lift.App) *HandlerTestSuite {
	return &HandlerTestSuite{
		t:        t,
		app:      app,
		contexts: make([]*lift.Context, 0),
	}
}

// RunTest executes a single handler test case
func (s *HandlerTestSuite) RunTest(tc HandlerTestCase) {
	s.t.Run(tc.Name, func(t *testing.T) {
		// Run setup if provided
		if tc.SetupFunc != nil {
			tc.SetupFunc()
		}

		// Ensure cleanup runs
		if tc.CleanupFunc != nil {
			defer tc.CleanupFunc()
		}

		// Create test context
		ctx := s.createTestContext(tc)
		s.contexts = append(s.contexts, ctx)

		// Execute handler
		_, err := s.app.HandleRequest(context.Background(), ctx.Request)

		// Validate results
		s.validateResults(t, ctx, tc, err)
	})
}

// RunTests executes multiple handler test cases
func (s *HandlerTestSuite) RunTests(testCases []HandlerTestCase) {
	for _, tc := range testCases {
		s.RunTest(tc)
	}
}

// createTestContext creates a Lift context from test case
func (s *HandlerTestSuite) createTestContext(tc HandlerTestCase) *lift.Context {
	// Create base context
	baseCtx := context.Background()

	// Marshal body if provided
	var bodyBytes []byte
	if tc.Body != nil {
		switch v := tc.Body.(type) {
		case string:
			bodyBytes = []byte(v)
		case []byte:
			bodyBytes = v
		default:
			bodyBytes, _ = json.Marshal(tc.Body)
		}
	}

	// Create headers
	headers := make(map[string]string)
	if tc.Headers != nil {
		for k, v := range tc.Headers {
			headers[k] = v
		}
	}

	// Set content type if not provided
	if _, ok := headers["Content-Type"]; !ok && tc.Body != nil {
		headers["Content-Type"] = "application/json"
	}

	// Add tenant ID if provided
	if tc.TenantID != "" {
		headers["X-Tenant-ID"] = tc.TenantID
	}

	// Create query string
	queryString := ""
	if tc.QueryParams != nil {
		params := url.Values{}
		for k, v := range tc.QueryParams {
			params.Add(k, v)
		}
		queryString = params.Encode()
	}

	// Create API Gateway v2 request
	request := events.APIGatewayV2HTTPRequest{
		RouteKey:       fmt.Sprintf("%s %s", tc.Method, tc.Path),
		RawPath:        tc.Path,
		Headers:        headers,
		Body:           string(bodyBytes),
		RawQueryString: queryString,
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
				Method: tc.Method,
				Path:   tc.Path,
			},
			RequestID: fmt.Sprintf("test-request-%d", len(s.contexts)),
		},
	}

	// Add path parameters
	if tc.PathParams != nil {
		request.PathParameters = tc.PathParams
	}

	// Add query parameters
	if tc.QueryParams != nil {
		request.QueryStringParameters = tc.QueryParams
	}

	// Create Lift context
	return &lift.Context{
		Context: baseCtx,
		Request: &lift.Request{
			Request: &adapters.Request{
				Method:      tc.Method,
				Path:        tc.Path,
				Headers:     headers,
				Body:        bodyBytes,
				PathParams:  tc.PathParams,
				QueryParams: tc.QueryParams,
				RawEvent:    request,
			},
			Method:      tc.Method,
			Path:        tc.Path,
			Headers:     headers,
			QueryParams: tc.QueryParams,
			Body:        bodyBytes,
		},
		Response: &lift.Response{
			StatusCode: 200,
			Headers:    make(map[string]string),
		},
	}
}

// validateResults validates test results against expectations
func (s *HandlerTestSuite) validateResults(t *testing.T, ctx *lift.Context, tc HandlerTestCase, err error) {
	// Check error expectations
	if tc.ExpectedError != "" {
		assert.Error(t, err)
		assert.Contains(t, err.Error(), tc.ExpectedError)
		return
	}
	assert.NoError(t, err)

	// Check status code
	assert.Equal(t, tc.ExpectedStatus, ctx.Response.StatusCode, "Unexpected status code")

	// Check response body if expected
	if tc.ExpectedBody != nil {
		var actualBody interface{}
		bodyBytes, ok := ctx.Response.Body.([]byte)
		if !ok {
			bodyBytes = []byte(fmt.Sprintf("%v", ctx.Response.Body))
		}
		err := json.Unmarshal(bodyBytes, &actualBody)
		assert.NoError(t, err, "Failed to unmarshal response body")

		switch expected := tc.ExpectedBody.(type) {
		case string:
			// For string expectations, check if response contains the string
			assert.Contains(t, ctx.Response.Body, expected)
		case map[string]interface{}:
			// For map expectations, check specific fields
			actualMap, ok := actualBody.(map[string]interface{})
			assert.True(t, ok, "Response body is not a map")
			for k, v := range expected {
				assert.Equal(t, v, actualMap[k], fmt.Sprintf("Field %s does not match", k))
			}
		default:
			// For other types, do deep equality check
			assert.Equal(t, tc.ExpectedBody, actualBody)
		}
	}
}

// MockLiftContext creates a mock Lift context for testing
func MockLiftContext(method, path string, opts ...ContextOption) *lift.Context {
	ctx := &lift.Context{
		Context: context.Background(),
		Request: &lift.Request{
			Request: &adapters.Request{
				Method:      method,
				Path:        path,
				Headers:     make(map[string]string),
				PathParams:  make(map[string]string),
				QueryParams: make(map[string]string),
			},
			Method:      method,
			Path:        path,
			Headers:     make(map[string]string),
			QueryParams: make(map[string]string),
		},
		Response: &lift.Response{
			StatusCode: 200,
			Headers:    make(map[string]string),
		},
	}

	// Apply options
	for _, opt := range opts {
		opt(ctx)
	}

	return ctx
}

// ContextOption defines options for creating mock contexts
type ContextOption func(*lift.Context)

// WithBody adds a body to the context
func WithBody(body interface{}) ContextOption {
	return func(ctx *lift.Context) {
		switch v := body.(type) {
		case string:
			ctx.Request.Body = []byte(v)
			ctx.Request.Request.Body = []byte(v)
		case []byte:
			ctx.Request.Body = v
			ctx.Request.Request.Body = v
		default:
			data, _ := json.Marshal(body)
			ctx.Request.Body = data
			ctx.Request.Request.Body = data
			ctx.Request.Headers["Content-Type"] = "application/json"
		}
	}
}

// WithHeaders adds headers to the context
func WithHeaders(headers map[string]string) ContextOption {
	return func(ctx *lift.Context) {
		for k, v := range headers {
			ctx.Request.Headers[k] = v
		}
	}
}

// WithQueryParams adds query parameters to the context
func WithQueryParams(params map[string]string) ContextOption {
	return func(ctx *lift.Context) {
		for k, v := range params {
			ctx.Request.QueryParams[k] = v
			ctx.Request.Request.QueryParams[k] = v
		}
	}
}

// WithPathParams adds path parameters to the context
func WithPathParams(params map[string]string) ContextOption {
	return func(ctx *lift.Context) {
		for k, v := range params {
			ctx.Request.PathParams[k] = v
		}
	}
}

// WithTenant adds tenant ID to the context
func WithTenant(tenantID string) ContextOption {
	return func(ctx *lift.Context) {
		ctx.Request.Headers["X-Tenant-ID"] = tenantID
		ctx.Set("tenant_id", tenantID)
	}
}

// WithAuth adds authentication to the context
func WithAuth(username string, scopes []string) ContextOption {
	return func(ctx *lift.Context) {
		ctx.Set("username", username)
		ctx.Set("scopes", scopes)
		ctx.Set("authenticated", true)
	}
}

// AssertJSONResponse validates JSON response
func AssertJSONResponse(t *testing.T, ctx *lift.Context, expected interface{}) {
	t.Helper()

	var actual interface{}
	bodyBytes, ok := ctx.Response.Body.([]byte)
	if !ok {
		bodyBytes = []byte(fmt.Sprintf("%v", ctx.Response.Body))
	}
	err := json.Unmarshal(bodyBytes, &actual)
	assert.NoError(t, err, "Failed to unmarshal response")
	assert.Equal(t, expected, actual, "Response body mismatch")
}

// AssertErrorResponse validates error response
func AssertErrorResponse(t *testing.T, ctx *lift.Context, expectedStatus int, expectedMessage string) {
	t.Helper()

	assert.Equal(t, expectedStatus, ctx.Response.StatusCode, "Unexpected status code")

	var errorResp map[string]interface{}
	bodyBytes, ok := ctx.Response.Body.([]byte)
	if !ok {
		bodyBytes = []byte(fmt.Sprintf("%v", ctx.Response.Body))
	}
	err := json.Unmarshal(bodyBytes, &errorResp)
	assert.NoError(t, err, "Failed to unmarshal error response")

	if expectedMessage != "" {
		message, ok := errorResp["error"].(string)
		if !ok {
			message, _ = errorResp["message"].(string)
		}
		assert.Contains(t, message, expectedMessage, "Error message mismatch")
	}
}

// HandlerFunc represents a handler function for testing
type HandlerFunc func(*lift.Context) error

// TestHandler wraps a handler function for testing
func TestHandler(t *testing.T, handler HandlerFunc, testCases []HandlerTestCase) {
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			// Setup
			if tc.SetupFunc != nil {
				tc.SetupFunc()
			}
			if tc.CleanupFunc != nil {
				defer tc.CleanupFunc()
			}

			// Create context
			opts := []ContextOption{
				WithHeaders(tc.Headers),
				WithQueryParams(tc.QueryParams),
				WithPathParams(tc.PathParams),
			}
			if tc.Body != nil {
				opts = append(opts, WithBody(tc.Body))
			}
			if tc.TenantID != "" {
				opts = append(opts, WithTenant(tc.TenantID))
			}

			ctx := MockLiftContext(tc.Method, tc.Path, opts...)

			// Execute handler
			err := handler(ctx)

			// Validate
			if tc.ExpectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.ExpectedError)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.ExpectedStatus, ctx.Response.StatusCode)
				if tc.ExpectedBody != nil {
					AssertJSONResponse(t, ctx, tc.ExpectedBody)
				}
			}
		})
	}
}

// StreamTestHelpers for testing streaming responses
type StreamTestHelpers struct{}

// NewStreamTestHelpers creates streaming test helpers
func NewStreamTestHelpers() *StreamTestHelpers {
	return &StreamTestHelpers{}
}

// CreateSSEReader creates a reader for Server-Sent Events
func (s *StreamTestHelpers) CreateSSEReader(body string) *SSEReader {
	return &SSEReader{
		reader: strings.NewReader(body),
		buffer: new(bytes.Buffer),
	}
}

// SSEReader reads Server-Sent Events
type SSEReader struct {
	reader io.Reader
	buffer *bytes.Buffer
}

// ReadEvent reads the next SSE event
func (r *SSEReader) ReadEvent() (map[string]string, error) {
	event := make(map[string]string)

	for {
		line, err := r.readLine()
		if err != nil {
			return nil, err
		}

		if line == "" {
			// Empty line signals end of event
			if len(event) > 0 {
				return event, nil
			}
			continue
		}

		if strings.HasPrefix(line, ":") {
			// Comment line, skip
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			field := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			event[field] = value
		}
	}
}

// readLine reads a single line from the SSE stream
func (r *SSEReader) readLine() (string, error) {
	r.buffer.Reset()

	var b [1]byte
	for {
		n, err := r.reader.Read(b[:])
		if n == 0 || err == io.EOF {
			if r.buffer.Len() > 0 {
				return r.buffer.String(), nil
			}
			return "", io.EOF
		}
		if err != nil {
			return "", err
		}

		if b[0] == '\n' {
			return r.buffer.String(), nil
		}
		if b[0] != '\r' {
			r.buffer.WriteByte(b[0])
		}
	}
}

// ResponseRecorder records handler responses for testing
type ResponseRecorder struct {
	StatusCode int
	Headers    http.Header
	Body       *bytes.Buffer
}

// NewResponseRecorder creates a new response recorder
func NewResponseRecorder() *ResponseRecorder {
	return &ResponseRecorder{
		StatusCode: 200,
		Headers:    make(http.Header),
		Body:       new(bytes.Buffer),
	}
}

// Write implements io.Writer
func (r *ResponseRecorder) Write(data []byte) (int, error) {
	return r.Body.Write(data)
}

// WriteHeader sets the status code
func (r *ResponseRecorder) WriteHeader(code int) {
	r.StatusCode = code
}

// Header returns the headers
func (r *ResponseRecorder) Header() http.Header {
	return r.Headers
}
