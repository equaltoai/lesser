package observability

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// =============================================================================
// Test Helpers: RoundTripper stub (no network)
// =============================================================================

// rtFunc implements http.RoundTripper for testing without network.
type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// stubMetricsRecorder records calls to RecordLatency for test assertions.
type stubMetricsRecorder struct {
	called       bool
	operation    string
	table        string
	duration     time.Duration
	success      bool
	dimensions   map[string]string
	err          error // error to return
	callCount    int
	recordedChan chan struct{} // optional channel for synchronization
}

func (s *stubMetricsRecorder) RecordLatency(ctx context.Context, operation, table string, duration time.Duration, success bool, dimensions map[string]string) error {
	s.called = true
	s.operation = operation
	s.table = table
	s.duration = duration
	s.success = success
	s.dimensions = dimensions
	s.callCount++
	if s.recordedChan != nil {
		close(s.recordedChan)
	}
	return s.err
}

// =============================================================================
// Tests for categorizeHTTPError
// =============================================================================

func TestCategorizeHTTPError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "nil error returns empty string",
			err:      nil,
			expected: "",
		},
		{
			name:     "context.DeadlineExceeded returns timeout",
			err:      context.DeadlineExceeded,
			expected: ErrorTypeTimeout,
		},
		{
			name:     "error containing timeout returns timeout",
			err:      errors.New("connection timeout: dial tcp"),
			expected: ErrorTypeTimeout,
		},
		{
			name:     "error containing deadline returns timeout",
			err:      errors.New("request deadline exceeded"),
			expected: ErrorTypeTimeout,
		},
		{
			name:     "error containing timed out returns timeout",
			err:      errors.New("request timed out"),
			expected: ErrorTypeTimeout,
		},
		{
			name:     "context.Canceled returns context_canceled",
			err:      context.Canceled,
			expected: "context_canceled",
		},
		{
			name:     "error containing no such host returns dns_error",
			err:      errors.New("lookup example.com: no such host"),
			expected: "dns_error",
		},
		{
			name:     "error containing dns returns dns_error",
			err:      errors.New("dns resolution failed"),
			expected: "dns_error",
		},
		{
			name:     "error containing name resolution returns dns_error",
			err:      errors.New("name resolution failed"),
			expected: "dns_error",
		},
		{
			name:     "error containing connection refused returns connection_error",
			err:      errors.New("dial tcp: connection refused"),
			expected: "connection_error",
		},
		{
			name:     "error containing connection reset returns connection_error",
			err:      errors.New("read tcp: connection reset by peer"),
			expected: "connection_error",
		},
		{
			name:     "error containing no route to host returns connection_error",
			err:      errors.New("dial tcp: no route to host"),
			expected: "connection_error",
		},
		{
			name:     "error containing tls returns tls_error",
			err:      errors.New("tls handshake failure"),
			expected: "tls_error",
		},
		{
			name:     "error containing ssl returns tls_error",
			err:      errors.New("ssl certificate problem"),
			expected: "tls_error",
		},
		{
			name:     "error containing certificate returns tls_error",
			err:      errors.New("certificate verify failed"),
			expected: "tls_error",
		},
		{
			name:     "error containing x509 returns tls_error",
			err:      errors.New("x509: certificate signed by unknown authority"),
			expected: "tls_error",
		},
		{
			name:     "unrecognized error returns network_error",
			err:      errors.New("some unknown error"),
			expected: "network_error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := categorizeHTTPError(tc.err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// =============================================================================
// Tests for categorizeHTTPStatusError
// =============================================================================

func TestCategorizeHTTPStatusError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		expected   string
	}{
		{
			name:       "401 returns authentication",
			statusCode: 401,
			expected:   ErrorTypeAuthentication,
		},
		{
			name:       "403 returns authorization",
			statusCode: 403,
			expected:   ErrorTypeAuthorization,
		},
		{
			name:       "404 returns not_found",
			statusCode: 404,
			expected:   ErrorTypeNotFound,
		},
		{
			name:       "408 returns timeout",
			statusCode: 408,
			expected:   ErrorTypeTimeout,
		},
		{
			name:       "409 returns conflict",
			statusCode: 409,
			expected:   ErrorTypeConflict,
		},
		{
			name:       "429 returns rate_limit",
			statusCode: 429,
			expected:   ErrorTypeRateLimit,
		},
		{
			name:       "400 returns validation",
			statusCode: 400,
			expected:   ErrorTypeValidation,
		},
		{
			name:       "422 returns validation (other 4xx)",
			statusCode: 422,
			expected:   ErrorTypeValidation,
		},
		{
			name:       "499 returns validation (other 4xx)",
			statusCode: 499,
			expected:   ErrorTypeValidation,
		},
		{
			name:       "500 returns server_error",
			statusCode: 500,
			expected:   "server_error",
		},
		{
			name:       "502 returns server_error",
			statusCode: 502,
			expected:   "server_error",
		},
		{
			name:       "503 returns server_error",
			statusCode: 503,
			expected:   "server_error",
		},
		{
			name:       "599 returns server_error",
			statusCode: 599,
			expected:   "server_error",
		},
		{
			name:       "200 returns empty string",
			statusCode: 200,
			expected:   "",
		},
		{
			name:       "201 returns empty string",
			statusCode: 201,
			expected:   "",
		},
		{
			name:       "301 returns empty string",
			statusCode: 301,
			expected:   "",
		},
		{
			name:       "399 returns empty string",
			statusCode: 399,
			expected:   "",
		},
		{
			name:       "100 returns empty string",
			statusCode: 100,
			expected:   "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := categorizeHTTPStatusError(tc.statusCode)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// =============================================================================
// Tests for isFederationRequest and getFederationType
// =============================================================================

func TestIsFederationRequest(t *testing.T) {
	tests := []struct {
		name     string
		url      *url.URL
		expected bool
	}{
		{
			name:     "nil URL returns false",
			url:      nil,
			expected: false,
		},
		{
			name:     "/inbox returns true",
			url:      mustParseURL("https://example.com/inbox"),
			expected: true,
		},
		{
			name:     "/outbox returns true",
			url:      mustParseURL("https://example.com/outbox"),
			expected: true,
		},
		{
			name:     "/.well-known/webfinger returns true",
			url:      mustParseURL("https://example.com/.well-known/webfinger?resource=acct:user@example.com"),
			expected: true,
		},
		{
			name:     "/.well-known/nodeinfo returns true",
			url:      mustParseURL("https://example.com/.well-known/nodeinfo"),
			expected: true,
		},
		{
			name:     "/users/alice returns true",
			url:      mustParseURL("https://example.com/users/alice"),
			expected: true,
		},
		{
			name:     "/users/ prefix returns true",
			url:      mustParseURL("https://example.com/users/bob/inbox"),
			expected: true,
		},
		{
			name:     "/actors/alice returns true",
			url:      mustParseURL("https://example.com/actors/alice"),
			expected: true,
		},
		{
			name:     "/activities/xyz returns true",
			url:      mustParseURL("https://example.com/activities/123abc"),
			expected: true,
		},
		{
			name:     "/objects/xyz returns true",
			url:      mustParseURL("https://example.com/objects/note-123"),
			expected: true,
		},
		{
			name:     "/api/v1/statuses returns false",
			url:      mustParseURL("https://example.com/api/v1/statuses"),
			expected: false,
		},
		{
			name:     "/home returns false",
			url:      mustParseURL("https://example.com/home"),
			expected: false,
		},
		{
			name:     "/ returns false",
			url:      mustParseURL("https://example.com/"),
			expected: false,
		},
		{
			name:     "/profiles/user returns false",
			url:      mustParseURL("https://example.com/profiles/user"),
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := isFederationRequest(tc.url)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestGetFederationType(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "/inbox returns inbox",
			path:     "/inbox",
			expected: "inbox",
		},
		{
			name:     "/outbox returns outbox",
			path:     "/outbox",
			expected: "outbox",
		},
		{
			name:     "/.well-known/webfinger returns discovery",
			path:     "/.well-known/webfinger",
			expected: "discovery",
		},
		{
			name:     "/.well-known/nodeinfo returns discovery",
			path:     "/.well-known/nodeinfo",
			expected: "discovery",
		},
		{
			name:     "/users/alice returns actor",
			path:     "/users/alice",
			expected: "actor",
		},
		{
			name:     "/actors/bob returns actor",
			path:     "/actors/bob",
			expected: "actor",
		},
		{
			name:     "/activities/123 returns activity",
			path:     "/activities/123",
			expected: "activity",
		},
		{
			name:     "/objects/note-123 returns object",
			path:     "/objects/note-123",
			expected: "object",
		},
		{
			name:     "/api/v1/statuses returns other",
			path:     "/api/v1/statuses",
			expected: "other",
		},
		{
			name:     "/home returns other",
			path:     "/home",
			expected: "other",
		},
		{
			name:     "empty path returns other",
			path:     "",
			expected: "other",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := getFederationType(tc.path)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// =============================================================================
// Tests for HTTPLatencyTracker.TrackRequest
// =============================================================================

func TestHTTPLatencyTracker_TrackRequest(t *testing.T) {
	tests := []struct {
		name              string
		method            string
		url               string
		statusCode        int
		duration          time.Duration
		expectedOperation string
		expectedHost      string
		expectedSuccess   bool
		hasFederationType bool
		expectedFedType   string
		recorderNil       bool
	}{
		{
			name:              "valid non-federation URL records http_request",
			method:            "GET",
			url:               "https://api.example.com/v1/users",
			statusCode:        200,
			duration:          100 * time.Millisecond,
			expectedOperation: "http_request",
			expectedHost:      "api.example.com",
			expectedSuccess:   true,
			hasFederationType: false,
		},
		{
			name:              "invalid URL sets host to unknown",
			method:            "GET",
			url:               "://invalid-url",
			statusCode:        200,
			duration:          100 * time.Millisecond,
			expectedOperation: "http_request",
			expectedHost:      unknownValue,
			expectedSuccess:   true,
			hasFederationType: false,
		},
		{
			name:              "federation URL records federation_request with type",
			method:            "POST",
			url:               "https://remote.instance/inbox",
			statusCode:        202,
			duration:          250 * time.Millisecond,
			expectedOperation: "federation_request",
			expectedHost:      "remote.instance",
			expectedSuccess:   true,
			hasFederationType: true,
			expectedFedType:   "inbox",
		},
		{
			name:              "federation webfinger URL records discovery type",
			method:            "GET",
			url:               "https://remote.instance/.well-known/webfinger?resource=acct:user@example.com",
			statusCode:        200,
			duration:          150 * time.Millisecond,
			expectedOperation: "federation_request",
			expectedHost:      "remote.instance",
			expectedSuccess:   true,
			hasFederationType: true,
			expectedFedType:   "discovery",
		},
		{
			name:              "federation user URL records actor type",
			method:            "GET",
			url:               "https://remote.instance/users/alice",
			statusCode:        200,
			duration:          120 * time.Millisecond,
			expectedOperation: "federation_request",
			expectedHost:      "remote.instance",
			expectedSuccess:   true,
			hasFederationType: true,
			expectedFedType:   "actor",
		},
		{
			name:              "4xx status is not success",
			method:            "GET",
			url:               "https://api.example.com/v1/users",
			statusCode:        404,
			duration:          50 * time.Millisecond,
			expectedOperation: "http_request",
			expectedHost:      "api.example.com",
			expectedSuccess:   false,
			hasFederationType: false,
		},
		{
			name:              "5xx status is not success",
			method:            "POST",
			url:               "https://api.example.com/v1/users",
			statusCode:        500,
			duration:          200 * time.Millisecond,
			expectedOperation: "http_request",
			expectedHost:      "api.example.com",
			expectedSuccess:   false,
			hasFederationType: false,
		},
		{
			name:              "3xx status is success",
			method:            "GET",
			url:               "https://api.example.com/redirect",
			statusCode:        301,
			duration:          30 * time.Millisecond,
			expectedOperation: "http_request",
			expectedHost:      "api.example.com",
			expectedSuccess:   true,
			hasFederationType: false,
		},
		{
			name:        "nil recorder does not panic",
			method:      "GET",
			url:         "https://api.example.com/v1/users",
			statusCode:  200,
			duration:    100 * time.Millisecond,
			recorderNil: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logger := zaptest.NewLogger(t)
			var recorder *stubMetricsRecorder
			var tracker *HTTPLatencyTracker

			if tc.recorderNil {
				tracker = NewHTTPLatencyTracker(nil, "test-service", logger)
			} else {
				recorder = &stubMetricsRecorder{}
				tracker = NewHTTPLatencyTracker(recorder, "test-service", logger)
			}

			tracker.TrackRequest(context.Background(), tc.method, tc.url, tc.statusCode, tc.duration)

			if tc.recorderNil {
				// Just ensure no panic
				return
			}

			assert.True(t, recorder.called, "RecordLatency should be called")
			assert.Equal(t, tc.expectedOperation, recorder.operation)
			assert.Equal(t, tc.expectedHost, recorder.table)
			assert.Equal(t, tc.duration, recorder.duration)
			assert.Equal(t, tc.expectedSuccess, recorder.success)
			assert.Equal(t, tc.method, recorder.dimensions["method"])

			if tc.hasFederationType {
				assert.Equal(t, tc.expectedFedType, recorder.dimensions["federation_type"])
			} else {
				_, hasFedType := recorder.dimensions["federation_type"]
				assert.False(t, hasFedType, "should not have federation_type dimension")
			}
		})
	}
}

// =============================================================================
// Tests for HTTPTracker.Do basic behavior
// =============================================================================

func TestHTTPTracker_Do_BasicBehavior(t *testing.T) {
	tests := []struct {
		name             string
		transport        rtFunc
		expectedStatus   int
		expectedSuccess  bool
		expectedErrType  string
		transportErr     error
		transportReturns bool
	}{
		{
			name: "successful 200 response",
			transport: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(strings.NewReader("ok")),
					Header:     make(http.Header),
					Request:    req,
				}, nil
			},
			expectedStatus:  200,
			expectedSuccess: true,
			expectedErrType: "",
		},
		{
			name: "successful 201 response",
			transport: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: 201,
					Body:       io.NopCloser(strings.NewReader("created")),
					Header:     make(http.Header),
					Request:    req,
				}, nil
			},
			expectedStatus:  201,
			expectedSuccess: true,
			expectedErrType: "",
		},
		{
			name: "4xx response sets error type",
			transport: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: 404,
					Body:       io.NopCloser(strings.NewReader("not found")),
					Header:     make(http.Header),
					Request:    req,
				}, nil
			},
			expectedStatus:  404,
			expectedSuccess: false,
			expectedErrType: ErrorTypeNotFound,
		},
		{
			name: "429 response returns rate_limit error type",
			transport: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: 429,
					Body:       io.NopCloser(strings.NewReader("too many requests")),
					Header:     make(http.Header),
					Request:    req,
				}, nil
			},
			expectedStatus:  429,
			expectedSuccess: false,
			expectedErrType: ErrorTypeRateLimit,
		},
		{
			name: "5xx response sets server_error type",
			transport: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: 503,
					Body:       io.NopCloser(strings.NewReader("service unavailable")),
					Header:     make(http.Header),
					Request:    req,
				}, nil
			},
			expectedStatus:  503,
			expectedSuccess: false,
			expectedErrType: "server_error",
		},
		{
			name: "timeout error sets timeout error type",
			transport: func(req *http.Request) (*http.Response, error) {
				return nil, errors.New("dial tcp: timeout")
			},
			expectedStatus:  0,
			expectedSuccess: false,
			expectedErrType: ErrorTypeTimeout,
			transportErr:    errors.New("dial tcp: timeout"),
		},
		{
			name: "connection refused error sets connection_error type",
			transport: func(req *http.Request) (*http.Response, error) {
				return nil, errors.New("dial tcp: connection refused")
			},
			expectedStatus:  0,
			expectedSuccess: false,
			expectedErrType: "connection_error",
			transportErr:    errors.New("dial tcp: connection refused"),
		},
		{
			name: "DNS error sets dns_error type",
			transport: func(req *http.Request) (*http.Response, error) {
				return nil, errors.New("lookup example.com: no such host")
			},
			expectedStatus:  0,
			expectedSuccess: false,
			expectedErrType: "dns_error",
			transportErr:    errors.New("lookup example.com: no such host"),
		},
		{
			name: "TLS error sets tls_error type",
			transport: func(req *http.Request) (*http.Response, error) {
				return nil, errors.New("x509: certificate signed by unknown authority")
			},
			expectedStatus:  0,
			expectedSuccess: false,
			expectedErrType: "tls_error",
			transportErr:    errors.New("x509: certificate signed by unknown authority"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logger := zaptest.NewLogger(t)

			// Use nil recorder to avoid goroutine synchronization issues
			client := &http.Client{Transport: tc.transport}
			tracker := NewHTTPTracker(client, logger, nil, "test-service")

			req, err := http.NewRequest(http.MethodGet, "https://example.com/test", nil)
			require.NoError(t, err)

			resp, metrics, doErr := tracker.Do(context.Background(), req)

			// Check error handling
			if tc.transportErr != nil {
				assert.Error(t, doErr)
			} else {
				assert.NoError(t, doErr)
				assert.NotNil(t, resp)
				if resp != nil && resp.Body != nil {
					defer resp.Body.Close()
				}
			}

			// Check metrics
			require.NotNil(t, metrics)
			assert.Equal(t, tc.expectedStatus, metrics.StatusCode)
			assert.Equal(t, tc.expectedSuccess, metrics.Success)
			assert.Equal(t, tc.expectedErrType, metrics.ErrorType)
			assert.Equal(t, http.MethodGet, metrics.Method)
			assert.Equal(t, "https://example.com/test", metrics.URL)
			assert.Greater(t, metrics.TotalTime, time.Duration(0))
		})
	}
}

func TestHTTPTracker_Do_FederationDetection(t *testing.T) {
	logger := zaptest.NewLogger(t)

	transport := rtFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader("ok")),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})

	client := &http.Client{Transport: transport}
	tracker := NewHTTPTracker(client, logger, nil, "test-service")

	// Test federation URL (/inbox)
	req, err := http.NewRequest(http.MethodPost, "https://remote.instance/inbox", nil)
	require.NoError(t, err)

	resp, metrics, doErr := tracker.Do(context.Background(), req)
	assert.NoError(t, doErr)
	assert.NotNil(t, resp)
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	assert.NotNil(t, metrics)
	assert.True(t, metrics.Success)
}

func TestHTTPTracker_Do_UnknownHost(t *testing.T) {
	logger := zaptest.NewLogger(t)

	transport := rtFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader("ok")),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})

	client := &http.Client{Transport: transport}
	tracker := NewHTTPTracker(client, logger, nil, "test-service")

	// Create request with empty host
	req, err := http.NewRequest(http.MethodGet, "https://example.com/test", nil)
	require.NoError(t, err)
	// Override URL to have empty host
	req.URL.Host = ""

	resp, metrics, doErr := tracker.Do(context.Background(), req)
	assert.NoError(t, doErr)
	assert.NotNil(t, resp)
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	assert.NotNil(t, metrics)
}

func TestHTTPTracker_NewHTTPTracker_NilClient(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Test that nil client is handled by using http.DefaultClient
	tracker := NewHTTPTracker(nil, logger, nil, "test-service")
	assert.NotNil(t, tracker)
	assert.Equal(t, http.DefaultClient, tracker.client)
}

// =============================================================================
// Tests for helper functions
// =============================================================================

func TestIsTimeoutError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"timeout in message", errors.New("connection timeout"), true},
		{"deadline in message", errors.New("deadline exceeded"), true},
		{"timed out in message", errors.New("request timed out"), true},
		{"no timeout", errors.New("connection refused"), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := isTimeoutError(tc.err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestIsDNSError(t *testing.T) {
	tests := []struct {
		name     string
		errStr   string
		expected bool
	}{
		{"no such host", "lookup example.com: no such host", true},
		{"dns keyword", "dns resolution failed", true},
		{"name resolution", "name resolution failed", true},
		{"not dns error", "connection refused", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := isDNSError(tc.errStr)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestIsConnectionError(t *testing.T) {
	tests := []struct {
		name     string
		errStr   string
		expected bool
	}{
		{"connection refused", "dial tcp: connection refused", true},
		{"connection reset", "connection reset by peer", true},
		{"no route to host", "no route to host", true},
		{"not connection error", "timeout", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := isConnectionError(tc.errStr)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestIsTLSError(t *testing.T) {
	tests := []struct {
		name     string
		errStr   string
		expected bool
	}{
		{"tls keyword", "tls handshake failure", true},
		{"ssl keyword", "ssl certificate error", true},
		{"certificate keyword", "certificate verify failed", true},
		{"x509 keyword", "x509: certificate signed by unknown authority", true},
		{"not tls error", "connection refused", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := isTLSError(tc.errStr)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestIsContextError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"context.Canceled", context.Canceled, true},
		{"context.DeadlineExceeded", context.DeadlineExceeded, true},
		{"other error", errors.New("some error"), false},
		{"nil error", nil, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := isContextError(tc.err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestContainsAny(t *testing.T) {
	tests := []struct {
		name       string
		s          string
		substrings []string
		expected   bool
	}{
		{
			name:       "contains first substring",
			s:          "hello world",
			substrings: []string{"hello", "foo", "bar"},
			expected:   true,
		},
		{
			name:       "contains middle substring",
			s:          "hello world",
			substrings: []string{"foo", "world", "bar"},
			expected:   true,
		},
		{
			name:       "contains last substring",
			s:          "hello world",
			substrings: []string{"foo", "bar", "world"},
			expected:   true,
		},
		{
			name:       "contains none",
			s:          "hello world",
			substrings: []string{"foo", "bar", "baz"},
			expected:   false,
		},
		{
			name:       "empty substrings",
			s:          "hello world",
			substrings: []string{},
			expected:   false,
		},
		{
			name:       "empty string",
			s:          "",
			substrings: []string{"hello"},
			expected:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := containsAny(tc.s, tc.substrings)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		substr   string
		expected bool
	}{
		{"exact match", "hello", "hello", true},
		{"prefix match", "hello world", "hello", true},
		{"suffix match", "hello world", "world", true},
		{"middle match", "hello world", "lo wo", true},
		{"no match", "hello world", "xyz", false},
		{"longer substr", "hi", "hello", false},
		{"empty string", "", "hello", false},
		{"empty substr", "hello", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := contains(tc.s, tc.substr)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestFindSubstringHTTP(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		substr   string
		expected bool
	}{
		{"found at start", "hello world", "hello", true},
		{"found in middle", "hello world", "lo wo", true},
		{"found at end", "hello world", "world", true},
		{"not found", "hello world", "xyz", false},
		{"empty string", "", "hello", false},
		{"empty substr", "hello", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := findSubstringHTTP(tc.s, tc.substr)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// =============================================================================
// Tests for URL helper functions
// =============================================================================

func TestIsFederationURL(t *testing.T) {
	tests := []struct {
		name     string
		rawURL   string
		expected bool
	}{
		{
			name:     "inbox URL is federation",
			rawURL:   "https://example.com/inbox",
			expected: true,
		},
		{
			name:     "webfinger URL is federation",
			rawURL:   "https://example.com/.well-known/webfinger",
			expected: true,
		},
		{
			name:     "users path is federation",
			rawURL:   "https://example.com/users/alice",
			expected: true,
		},
		{
			name:     "API path is not federation",
			rawURL:   "https://example.com/api/v1/statuses",
			expected: false,
		},
		{
			name:     "invalid URL returns false",
			rawURL:   "://invalid",
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := isFederationURL(tc.rawURL)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestGetFederationTypeFromURL(t *testing.T) {
	tests := []struct {
		name     string
		rawURL   string
		expected string
	}{
		{
			name:     "inbox URL returns inbox",
			rawURL:   "https://example.com/inbox",
			expected: "inbox",
		},
		{
			name:     "webfinger URL returns discovery",
			rawURL:   "https://example.com/.well-known/webfinger",
			expected: "discovery",
		},
		{
			name:     "users path returns actor",
			rawURL:   "https://example.com/users/alice",
			expected: "actor",
		},
		{
			name:     "invalid URL returns unknown",
			rawURL:   "://invalid",
			expected: unknownValue,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := getFederationTypeFromURL(tc.rawURL)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestURLParse(t *testing.T) {
	tests := []struct {
		name      string
		rawURL    string
		expectErr bool
		host      string
	}{
		{
			name:      "valid URL",
			rawURL:    "https://example.com/path",
			expectErr: false,
			host:      "example.com",
		},
		{
			name:      "valid URL with port",
			rawURL:    "https://example.com:8080/path",
			expectErr: false,
			host:      "example.com:8080",
		},
		{
			name:      "invalid URL",
			rawURL:    "://invalid",
			expectErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u, err := urlParse(tc.rawURL)
			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.host, u.Host)
			}
		})
	}
}

// =============================================================================
// Helper functions
// =============================================================================

func mustParseURL(rawURL string) *url.URL {
	u, err := url.Parse(rawURL)
	if err != nil {
		panic(err)
	}
	return u
}
