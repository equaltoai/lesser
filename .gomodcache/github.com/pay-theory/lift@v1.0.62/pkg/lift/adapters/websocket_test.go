package adapters

import (
	"encoding/base64"
	"testing"
)

const (
	connectMethod    = "CONNECT"
	messageMethod    = "MESSAGE"
	disconnectMethod = "DISCONNECT"
)

func TestWebSocketAdapter_CanHandle(t *testing.T) {
	adapter := NewWebSocketAdapter()

	tests := []struct {
		event    any
		name     string
		expected bool
	}{
		{
			name: "Valid WebSocket connect event",
			event: map[string]any{
				"requestContext": map[string]any{
					"connectionId": "abc123",
					"routeKey":     "$connect",
					"eventType":    "CONNECT",
				},
			},
			expected: true,
		},
		{
			name: "Valid WebSocket disconnect event",
			event: map[string]any{
				"requestContext": map[string]any{
					"connectionId": "abc123",
					"routeKey":     "$disconnect",
					"eventType":    "DISCONNECT",
				},
			},
			expected: true,
		},
		{
			name: "Valid WebSocket message event",
			event: map[string]any{
				"requestContext": map[string]any{
					"connectionId": "abc123",
					"routeKey":     "sendMessage",
				},
			},
			expected: true,
		},
		{
			name: "Missing connectionId",
			event: map[string]any{
				"requestContext": map[string]any{
					"routeKey": "$connect",
				},
			},
			expected: false,
		},
		{
			name: "Not a WebSocket event",
			event: map[string]any{
				"httpMethod": "GET",
				"path":       "/test",
			},
			expected: false,
		},
		{
			name:     "Invalid event type",
			event:    "not a map",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := adapter.CanHandle(tt.event)
			if result != tt.expected {
				t.Errorf("CanHandle() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestWebSocketAdapter_Adapt(t *testing.T) {
	adapter := NewWebSocketAdapter()

	tests := []struct {
		event       any
		checkResult func(*testing.T, *Request)
		name        string
		wantErr     bool
	}{
		{
			name: "Connect event with query parameters",
			event: map[string]any{
				"requestContext": map[string]any{
					"connectionId": "abc123",
					"routeKey":     "$connect",
					"stage":        "prod",
					"requestId":    "req123",
					"domainName":   "api.example.com",
					"apiId":        "api123",
				},
				"queryStringParameters": map[string]any{
					"Authorization": "Bearer token123",
				},
				"headers": map[string]any{
					"User-Agent": "WebSocket Client",
				},
			},
			wantErr: false,
			checkResult: func(t *testing.T, req *Request) {
				if req.Method != connectMethod {
					t.Errorf("Method = %s, want %s", req.Method, connectMethod)
				}
				if req.Path != "/connect" {
					t.Errorf("Path = %s, want /connect", req.Path)
				}
				if req.QueryParams["Authorization"] != "Bearer token123" {
					t.Errorf("QueryParams[Authorization] = %s, want Bearer token123", req.QueryParams["Authorization"])
				}
				if metadata, ok := req.Metadata["connectionId"].(string); !ok || metadata != "abc123" {
					t.Errorf("Metadata[connectionId] = %v, want abc123", req.Metadata["connectionId"])
				}
			},
		},
		{
			name: "Message event with body",
			event: map[string]any{
				"requestContext": map[string]any{
					"connectionId": "xyz789",
					"routeKey":     "sendMessage",
					"stage":        "dev",
					"requestId":    "req456",
					"domainName":   "api.example.com",
					"apiId":        "api456",
				},
				"body": `{"action": "sendMessage", "data": "Hello"}`,
			},
			wantErr: false,
			checkResult: func(t *testing.T, req *Request) {
				if req.Method != messageMethod {
					t.Errorf("Method = %s, want %s", req.Method, messageMethod)
				}
				if req.Path != "/sendMessage" {
					t.Errorf("Path = %s, want /sendMessage", req.Path)
				}
				expectedBody := `{"action": "sendMessage", "data": "Hello"}`
				if string(req.Body) != expectedBody {
					t.Errorf("Body = %s, want %s", string(req.Body), expectedBody)
				}
			},
		},
		{
			name: "Disconnect event",
			event: map[string]any{
				"requestContext": map[string]any{
					"connectionId": "conn123",
					"routeKey":     "$disconnect",
					"stage":        "prod",
					"requestId":    "req789",
					"domainName":   "api.example.com",
					"apiId":        "api789",
				},
			},
			wantErr: false,
			checkResult: func(t *testing.T, req *Request) {
				if req.Method != disconnectMethod {
					t.Errorf("Method = %s, want %s", req.Method, disconnectMethod)
				}
				if req.Path != "/disconnect" {
					t.Errorf("Path = %s, want /disconnect", req.Path)
				}
			},
		},
		{
			name: "Base64 encoded body",
			event: map[string]any{
				"requestContext": map[string]any{
					"connectionId": "conn456",
					"routeKey":     "message",
					"stage":        "prod",
					"requestId":    "req999",
					"domainName":   "api.example.com",
					"apiId":        "api999",
				},
				"body":            base64.StdEncoding.EncodeToString([]byte("binary data")),
				"isBase64Encoded": true,
			},
			wantErr: false,
			checkResult: func(t *testing.T, req *Request) {
				expectedBody := "binary data"
				if string(req.Body) != expectedBody {
					t.Errorf("Body = %s, want %s", string(req.Body), expectedBody)
				}
			},
		},
		{
			name: "Invalid event - missing requestContext",
			event: map[string]any{
				"body": "test",
			},
			wantErr: true,
		},
		{
			name: "Invalid event - missing connectionId",
			event: map[string]any{
				"requestContext": map[string]any{
					"routeKey": "$connect",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := adapter.Adapt(tt.event)
			if (err != nil) != tt.wantErr {
				t.Errorf("Adapt() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.checkResult != nil {
				tt.checkResult(t, result)
			}
		})
	}
}

func TestMapWebSocketRoute(t *testing.T) {
	tests := []struct {
		routeKey   string
		wantMethod string
		wantPath   string
	}{
		{"$connect", "CONNECT", "/connect"},
		{"$disconnect", "DISCONNECT", "/disconnect"},
		{"$default", "MESSAGE", "/message"},
		{"sendMessage", "MESSAGE", "/sendMessage"},
		{"customRoute", "MESSAGE", "/customRoute"},
	}

	for _, tt := range tests {
		t.Run(tt.routeKey, func(t *testing.T) {
			method, path := mapWebSocketRoute(tt.routeKey)
			if method != tt.wantMethod {
				t.Errorf("mapWebSocketRoute() method = %s, want %s", method, tt.wantMethod)
			}
			if path != tt.wantPath {
				t.Errorf("mapWebSocketRoute() path = %s, want %s", path, tt.wantPath)
			}
		})
	}
}

// TestWebSocketQueryParameterBugFix verifies that the bug where query parameters
// from map[string]string were not being extracted is fixed
func TestWebSocketQueryParameterBugFix(t *testing.T) {
	adapter := NewWebSocketAdapter()

	// Test with map[string]string query parameters (the bug case)
	event := map[string]any{
		"requestContext": map[string]any{
			"connectionId": "test-connection-123",
			"routeKey":     "$connect",
			"stage":        "test",
			"requestId":    "test-req-123",
			"domainName":   "api.example.com",
			"apiId":        "test-api",
		},
		// This is the key: queryStringParameters as map[string]string
		// (as it comes from AWS Lambda events)
		"queryStringParameters": map[string]string{
			"Authorization": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...",
			"userId":        "12345",
		},
	}

	req, err := adapter.Adapt(event)
	if err != nil {
		t.Fatalf("Failed to adapt event: %v", err)
	}

	// Verify query parameters are correctly extracted
	if req.QueryParams == nil {
		t.Fatal("QueryParams is nil")
	}

	if len(req.QueryParams) != 2 {
		t.Errorf("Expected 2 query parameters, got %d", len(req.QueryParams))
	}

	if req.QueryParams["Authorization"] != "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9..." {
		t.Errorf("Authorization = %s, want eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...", req.QueryParams["Authorization"])
	}

	if req.QueryParams["userId"] != "12345" {
		t.Errorf("userId = %s, want 12345", req.QueryParams["userId"])
	}
}

// TestExtractStringMapFieldBothTypes tests that extractStringMapField handles both
// map[string]string and map[string]any input types
func TestExtractStringMapFieldBothTypes(t *testing.T) {
	tests := []struct {
		data     map[string]any
		expected map[string]string
		name     string
		key      string
	}{
		{
			name: "map[string]string input",
			data: map[string]any{
				"params": map[string]string{
					"Authorization": "Bearer token123",
					"userId":        "12345",
				},
			},
			key: "params",
			expected: map[string]string{
				"Authorization": "Bearer token123",
				"userId":        "12345",
			},
		},
		{
			name: "map[string]any input",
			data: map[string]any{
				"params": map[string]any{
					"Authorization": "Bearer token123",
					"userId":        "12345",
				},
			},
			key: "params",
			expected: map[string]string{
				"Authorization": "Bearer token123",
				"userId":        "12345",
			},
		},
		{
			name: "mixed types in map[string]any",
			data: map[string]any{
				"params": map[string]any{
					"Authorization": "Bearer token123",
					"userId":        12345, // non-string value should be ignored
					"active":        true,  // non-string value should be ignored
				},
			},
			key: "params",
			expected: map[string]string{
				"Authorization": "Bearer token123",
			},
		},
		{
			name: "empty map",
			data: map[string]any{
				"params": map[string]string{},
			},
			key:      "params",
			expected: map[string]string{},
		},
		{
			name:     "missing key",
			data:     map[string]any{},
			key:      "params",
			expected: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractStringMapField(tt.data, tt.key)

			if len(result) != len(tt.expected) {
				t.Errorf("extractStringMapField() returned %d items, expected %d", len(result), len(tt.expected))
			}

			for k, v := range tt.expected {
				if result[k] != v {
					t.Errorf("extractStringMapField()[%s] = %s, want %s", k, result[k], v)
				}
			}
		})
	}
}

// Edge case tests for WebSocket adapter
func TestWebSocketAdapter_EdgeCases(t *testing.T) {
	adapter := NewWebSocketAdapter()

	tests := []struct {
		event       any
		checkResult func(*testing.T, *Request)
		name        string
		wantErr     bool
	}{
		{
			name: "extremely large message body",
			event: map[string]any{
				"requestContext": map[string]any{
					"connectionId": "large-msg-conn",
					"routeKey":     "message",
					"stage":        "prod",
					"requestId":    "req-large",
					"domainName":   "api.example.com",
					"apiId":        "api-large",
				},
				"body": string(make([]byte, 1024*1024)), // 1MB body
			},
			wantErr: false,
			checkResult: func(t *testing.T, req *Request) {
				if len(req.Body) != 1024*1024 {
					t.Errorf("Body length = %d, want %d", len(req.Body), 1024*1024)
				}
			},
		},
		{
			name: "malformed base64 body",
			event: map[string]any{
				"requestContext": map[string]any{
					"connectionId": "malformed-b64",
					"routeKey":     "message",
					"stage":        "prod",
					"requestId":    "req-b64",
					"domainName":   "api.example.com",
					"apiId":        "api-b64",
				},
				"body":            "!!!not-valid-base64!!!",
				"isBase64Encoded": true,
			},
			wantErr: true, // Base64 decode error is returned
			checkResult: func(_ *testing.T, _ *Request) {
				// Should not reach here due to error
			},
		},
		{
			name: "nil values in requestContext",
			event: map[string]any{
				"requestContext": map[string]any{
					"connectionId": "nil-test",
					"routeKey":     "$connect",
					"stage":        nil,
					"requestId":    nil,
					"domainName":   nil,
					"apiId":        nil,
				},
			},
			wantErr: false,
			checkResult: func(t *testing.T, req *Request) {
				// Should handle nil values gracefully - they become empty strings
				if stage, ok := req.Metadata["stage"].(string); !ok || stage != "" {
					t.Errorf("Expected nil stage to be converted to empty string, got %v", req.Metadata["stage"])
				}
			},
		},
		{
			name: "empty connectionId",
			event: map[string]any{
				"requestContext": map[string]any{
					"connectionId": "",
					"routeKey":     "$connect",
				},
			},
			wantErr: false, // Empty connectionId is actually allowed by the implementation
		},
		{
			name: "special characters in route key",
			event: map[string]any{
				"requestContext": map[string]any{
					"connectionId": "special-chars",
					"routeKey":     "route/with/slashes",
					"stage":        "prod",
				},
			},
			wantErr: false,
			checkResult: func(t *testing.T, req *Request) {
				if req.Path != "/route/with/slashes" {
					t.Errorf("Path = %s, want /route/with/slashes", req.Path)
				}
			},
		},
		{
			name: "unicode in body",
			event: map[string]any{
				"requestContext": map[string]any{
					"connectionId": "unicode-test",
					"routeKey":     "message",
					"stage":        "prod",
				},
				"body": "Hello 世界 🌍 emoji test",
			},
			wantErr: false,
			checkResult: func(t *testing.T, req *Request) {
				expected := "Hello 世界 🌍 emoji test"
				if string(req.Body) != expected {
					t.Errorf("Body = %s, want %s", string(req.Body), expected)
				}
			},
		},
		{
			name: "mixed type headers",
			event: map[string]any{
				"requestContext": map[string]any{
					"connectionId": "mixed-headers",
					"routeKey":     "$connect",
					"stage":        "prod",
				},
				"headers": map[string]any{
					"String-Header": "value",
					"Number-Header": 12345,
					"Bool-Header":   true,
					"Nil-Header":    nil,
				},
			},
			wantErr: false,
			checkResult: func(t *testing.T, req *Request) {
				// Only string headers should be included - headers are converted to lowercase
				if req.Headers["string-header"] != "value" {
					t.Errorf("string-header = %s, want value", req.Headers["string-header"])
				}
				if _, exists := req.Headers["number-header"]; exists {
					t.Errorf("number-header should not be included")
				}
			},
		},
		{
			name: "deeply nested query parameters",
			event: map[string]any{
				"requestContext": map[string]any{
					"connectionId": "nested-query",
					"routeKey":     "$connect",
					"stage":        "prod",
				},
				"queryStringParameters": map[string]any{
					"nested": map[string]any{
						"should": "not work",
					},
				},
			},
			wantErr: false,
			checkResult: func(t *testing.T, req *Request) {
				// Nested objects should be ignored
				if _, exists := req.QueryParams["nested"]; exists {
					t.Errorf("Nested query parameter should not be included")
				}
			},
		},
		{
			name: "requestContext is wrong type",
			event: map[string]any{
				"requestContext": "not a map",
			},
			wantErr: true,
		},
		{
			name: "connectionId is wrong type",
			event: map[string]any{
				"requestContext": map[string]any{
					"connectionId": 12345, // number instead of string
					"routeKey":     "$connect",
				},
			},
			wantErr: true,
		},
		{
			name:    "empty event",
			event:   map[string]any{},
			wantErr: true,
		},
		{
			name:    "nil event",
			event:   nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := adapter.Adapt(tt.event)
			if (err != nil) != tt.wantErr {
				t.Errorf("Adapt() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.checkResult != nil {
				tt.checkResult(t, result)
			}
		})
	}
}

// Test concurrent adapter usage
func TestWebSocketAdapter_Concurrent(t *testing.T) {
	adapter := NewWebSocketAdapter()

	// Create multiple events
	events := []any{
		map[string]any{
			"requestContext": map[string]any{
				"connectionId": "conn1",
				"routeKey":     "$connect",
			},
		},
		map[string]any{
			"requestContext": map[string]any{
				"connectionId": "conn2",
				"routeKey":     "$disconnect",
			},
		},
		map[string]any{
			"requestContext": map[string]any{
				"connectionId": "conn3",
				"routeKey":     "message",
			},
			"body": "test message",
		},
	}

	// Run concurrent adaptations
	done := make(chan bool, len(events)*10)
	for i := 0; i < 10; i++ {
		for _, event := range events {
			go func(e any) {
				_, err := adapter.Adapt(e)
				if err != nil {
					t.Errorf("Concurrent Adapt() failed: %v", err)
				}
				done <- true
			}(event)
		}
	}

	// Wait for all goroutines
	for i := 0; i < len(events)*10; i++ {
		<-done
	}
}

// Test adapter with real-world WebSocket event structure
func TestWebSocketAdapter_RealWorldEvent(t *testing.T) {
	adapter := NewWebSocketAdapter()

	// This is a more complete real-world WebSocket event from API Gateway
	event := map[string]any{
		"requestContext": map[string]any{
			"routeKey":          "$connect",
			"messageId":         nil,
			"eventType":         "CONNECT",
			"extendedRequestId": "ZPGHtFb6oAMFtmA=",
			"requestTime":       "23/Jan/2024:12:34:56 +0000",
			"messageDirection":  "IN",
			"stage":             "production",
			"connectedAt":       1706014496000,
			"requestTimeEpoch":  1706014496123,
			"identity": map[string]any{
				"cognitoIdentityPoolId":         nil,
				"cognitoIdentityId":             nil,
				"principalOrgId":                nil,
				"cognitoAuthenticationType":     nil,
				"userArn":                       nil,
				"userAgent":                     "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)",
				"accountId":                     nil,
				"caller":                        nil,
				"sourceIp":                      "192.168.1.1",
				"accessKey":                     nil,
				"cognitoAuthenticationProvider": nil,
				"user":                          nil,
			},
			"requestId":    "ZPGHtFb6oAMFtmA=",
			"domainName":   "abcdefghij.execute-api.us-east-1.amazonaws.com",
			"connectionId": "ZPGHtd0toAMCJeg=",
			"apiId":        "abcdefghij",
		},
		"queryStringParameters": map[string]string{
			"Authorization": "Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...",
			"version":       "1.0",
		},
		"multiValueQueryStringParameters": map[string]any{
			"Authorization": []string{"Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9..."},
			"version":       []string{"1.0"},
		},
		"headers": map[string]any{
			"Host":                     "abcdefghij.execute-api.us-east-1.amazonaws.com",
			"Sec-WebSocket-Extensions": "permessage-deflate; client_max_window_bits",
			"Sec-WebSocket-Key":        "wPwGEh8hbiMmL8+a+y5FRQ==",
			"Sec-WebSocket-Version":    "13",
			"X-Amzn-Trace-Id":          "Root=1-65af9df0-7aa124343182650e3da81234",
			"X-Forwarded-For":          "192.168.1.1",
			"X-Forwarded-Port":         "443",
			"X-Forwarded-Proto":        "https",
			"accept-encoding":          "gzip, deflate, br",
			"accept-language":          "en-US,en;q=0.9",
			"cache-control":            "no-cache",
			"origin":                   "https://example.com",
			"pragma":                   "no-cache",
			"sec-websocket-protocol":   "chat",
			"user-agent":               "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)",
		},
		"multiValueHeaders": map[string]any{
			"Host":                     []string{"abcdefghij.execute-api.us-east-1.amazonaws.com"},
			"Sec-WebSocket-Extensions": []string{"permessage-deflate; client_max_window_bits"},
			"Sec-WebSocket-Key":        []string{"wPwGEh8hbiMmL8+a+y5FRQ=="},
			"Sec-WebSocket-Version":    []string{"13"},
			"X-Amzn-Trace-Id":          []string{"Root=1-65af9df0-7aa124343182650e3da81234"},
			"X-Forwarded-For":          []string{"192.168.1.1"},
			"X-Forwarded-Port":         []string{"443"},
			"X-Forwarded-Proto":        []string{"https"},
		},
		"requestId":       "ZPGHtFb6oAMFtmA=",
		"routeKey":        "$connect",
		"eventType":       "CONNECT",
		"isBase64Encoded": false,
	}

	req, err := adapter.Adapt(event)
	if err != nil {
		t.Fatalf("Failed to adapt real-world event: %v", err)
	}

	// Verify key fields are extracted correctly
	if req.Method != "CONNECT" {
		t.Errorf("Method = %s, want CONNECT", req.Method)
	}

	if req.QueryParams["Authorization"] != "Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9..." {
		t.Errorf("Missing or incorrect Authorization query param")
	}

	// Headers are lowercase in the adapter
	if req.Headers["user-agent"] != "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)" {
		t.Errorf("Missing or incorrect user-agent header, got: %v", req.Headers["user-agent"])
	}

	if metadata, ok := req.Metadata["connectionId"].(string); !ok || metadata != "ZPGHtd0toAMCJeg=" {
		t.Errorf("Missing or incorrect connectionId in metadata")
	}

	// Verify management endpoint is constructed
	if endpoint, ok := req.Metadata["managementEndpoint"].(string); !ok || endpoint == "" {
		t.Errorf("Management endpoint not constructed")
	}
}
