package adapters

import (
	"testing"
)

func TestAPIGatewayV2StageHandling(t *testing.T) {
	adapter := NewAPIGatewayV2Adapter()

	tests := []struct {
		name         string
		event        map[string]any
		expectedPath string
		description  string
	}{
		{
			name: "Custom domain with stage prefix",
			event: map[string]any{
				"version":  "2.0",
				"routeKey": "ANY /v1/customers",
				"rawPath":  "/paytheorystudy/v1/customers",
				"headers":  map[string]any{},
				"requestContext": map[string]any{
					"stage":     "paytheorystudy",
					"requestId": "test-123",
					"http": map[string]any{
						"method": "GET",
						"path":   "/paytheorystudy/v1/customers", // Stage included
					},
				},
			},
			expectedPath: "/v1/customers",
			description:  "Should strip stage prefix when present in path",
		},
		{
			name: "Direct API Gateway URL without stage in path",
			event: map[string]any{
				"version":  "2.0",
				"routeKey": "GET /users",
				"rawPath":  "/users",
				"headers":  map[string]any{},
				"requestContext": map[string]any{
					"stage":     "prod",
					"requestId": "test-456",
					"http": map[string]any{
						"method": "GET",
						"path":   "/users", // No stage prefix
					},
				},
			},
			expectedPath: "/users",
			description:  "Should not modify path when stage is not present",
		},
		{
			name: "$default stage",
			event: map[string]any{
				"version":  "2.0",
				"routeKey": "POST /api/data",
				"rawPath":  "/api/data",
				"headers":  map[string]any{},
				"requestContext": map[string]any{
					"stage":     "$default",
					"requestId": "test-789",
					"http": map[string]any{
						"method": "POST",
						"path":   "/api/data",
					},
				},
			},
			expectedPath: "/api/data",
			description:  "Should not strip $default stage",
		},
		{
			name: "Root path with stage",
			event: map[string]any{
				"version":  "2.0",
				"routeKey": "GET /",
				"rawPath":  "/dev",
				"headers":  map[string]any{},
				"requestContext": map[string]any{
					"stage":     "dev",
					"requestId": "test-root",
					"http": map[string]any{
						"method": "GET",
						"path":   "/dev",
					},
				},
			},
			expectedPath: "/",
			description:  "Should handle root path correctly after stripping stage",
		},
		{
			name: "Path with stage-like prefix but different stage",
			event: map[string]any{
				"version":  "2.0",
				"routeKey": "GET /production/data",
				"rawPath":  "/production/data",
				"headers":  map[string]any{},
				"requestContext": map[string]any{
					"stage":     "dev",
					"requestId": "test-mismatch",
					"http": map[string]any{
						"method": "GET",
						"path":   "/production/data",
					},
				},
			},
			expectedPath: "/production/data",
			description:  "Should not strip prefix if it doesn't match stage",
		},
		{
			name: "Complex path with multiple segments",
			event: map[string]any{
				"version":  "2.0",
				"routeKey": "PUT /v1/users/{id}/profile",
				"rawPath":  "/staging/v1/users/123/profile",
				"headers":  map[string]any{},
				"requestContext": map[string]any{
					"stage":     "staging",
					"requestId": "test-complex",
					"http": map[string]any{
						"method": "PUT",
						"path":   "/staging/v1/users/123/profile",
					},
				},
			},
			expectedPath: "/v1/users/123/profile",
			description:  "Should handle complex paths with parameters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request, err := adapter.Adapt(tt.event)
			if err != nil {
				t.Fatalf("Failed to adapt event: %v", err)
			}

			if request.Path != tt.expectedPath {
				t.Errorf("%s: expected path %q, got %q", tt.description, tt.expectedPath, request.Path)
			}
		})
	}
}

func TestAPIGatewayV2StageHandlingEdgeCases(t *testing.T) {
	adapter := NewAPIGatewayV2Adapter()

	tests := []struct {
		name         string
		event        map[string]any
		expectedPath string
	}{
		{
			name: "Empty stage",
			event: map[string]any{
				"version":  "2.0",
				"routeKey": "GET /test",
				"rawPath":  "/test",
				"headers":  map[string]any{},
				"requestContext": map[string]any{
					"stage":     "",
					"requestId": "test-empty",
					"http": map[string]any{
						"method": "GET",
						"path":   "/test",
					},
				},
			},
			expectedPath: "/test",
		},
		{
			name: "Missing stage in context",
			event: map[string]any{
				"version":  "2.0",
				"routeKey": "GET /test",
				"rawPath":  "/test",
				"headers":  map[string]any{},
				"requestContext": map[string]any{
					"requestId": "test-no-stage",
					"http": map[string]any{
						"method": "GET",
						"path":   "/test",
					},
				},
			},
			expectedPath: "/test",
		},
		{
			name: "Stage with special characters",
			event: map[string]any{
				"version":  "2.0",
				"routeKey": "GET /api",
				"rawPath":  "/stage-123/api",
				"headers":  map[string]any{},
				"requestContext": map[string]any{
					"stage":     "stage-123",
					"requestId": "test-special",
					"http": map[string]any{
						"method": "GET",
						"path":   "/stage-123/api",
					},
				},
			},
			expectedPath: "/api",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request, err := adapter.Adapt(tt.event)
			if err != nil {
				t.Fatalf("Failed to adapt event: %v", err)
			}

			if request.Path != tt.expectedPath {
				t.Errorf("Expected path %q, got %q", tt.expectedPath, request.Path)
			}
		})
	}
}
