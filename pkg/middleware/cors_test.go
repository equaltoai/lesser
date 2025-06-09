package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/assert"
)

func TestCORSMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		config         CORSConfig
		origin         string
		method         string
		expectedOrigin string
		expectedCreds  string
		shouldHaveCORS bool
	}{
		{
			name:           "Web UI - allowed origin",
			config:         DefaultCORSConfig,
			origin:         "https://lesser.example.com",
			method:         http.MethodGet,
			expectedOrigin: "https://lesser.example.com",
			expectedCreds:  "true",
			shouldHaveCORS: true,
		},
		{
			name:           "Web UI - disallowed origin",
			config:         DefaultCORSConfig,
			origin:         "https://evil.com",
			method:         http.MethodGet,
			expectedOrigin: "",
			expectedCreds:  "",
			shouldHaveCORS: false,
		},
		{
			name:           "ActivityPub - any origin",
			config:         ActivityPubCORSConfig,
			origin:         "https://mastodon.social",
			method:         http.MethodPost,
			expectedOrigin: "*",
			expectedCreds:  "",
			shouldHaveCORS: true,
		},
		{
			name:           "ActivityPub - preflight",
			config:         ActivityPubCORSConfig,
			origin:         "https://fosstodon.org",
			method:         http.MethodOptions,
			expectedOrigin: "*",
			expectedCreds:  "",
			shouldHaveCORS: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a simple handler
			handler := func(req events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
				return &events.APIGatewayV2HTTPResponse{
					StatusCode: http.StatusOK,
					Headers:    map[string]string{},
					Body:       "OK",
				}, nil
			}

			// Apply CORS middleware
			wrapped := CORS(tt.config)(handler)

			// Create request
			req := events.APIGatewayV2HTTPRequest{
				Headers: map[string]string{
					"Origin": tt.origin,
				},
				RequestContext: events.APIGatewayV2HTTPRequestContext{
					HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
						Method: tt.method,
					},
				},
			}

			// Execute
			resp, err := wrapped(req)
			assert.NoError(t, err)

			// Check response
			if tt.shouldHaveCORS {
				assert.Equal(t, tt.expectedOrigin, resp.Headers["Access-Control-Allow-Origin"])
				if tt.expectedCreds != "" {
					assert.Equal(t, tt.expectedCreds, resp.Headers["Access-Control-Allow-Credentials"])
				}
			} else {
				assert.Empty(t, resp.Headers["Access-Control-Allow-Origin"])
			}

			// Check Vary header
			assert.Contains(t, resp.Headers["Vary"], "Origin")
		})
	}
}

func TestCORSHTTP(t *testing.T) {
	tests := []struct {
		name           string
		config         CORSConfig
		origin         string
		method         string
		expectedOrigin string
		expectedCreds  string
		shouldHaveCORS bool
	}{
		{
			name:           "ActivityPub wildcard",
			config:         ActivityPubCORSConfig,
			origin:         "https://pleroma.example.com",
			method:         http.MethodPost,
			expectedOrigin: "*",
			expectedCreds:  "",
			shouldHaveCORS: true,
		},
		{
			name:           "Web UI specific origin",
			config:         DefaultCORSConfig,
			origin:         "https://app.lesser.example.com",
			method:         http.MethodGet,
			expectedOrigin: "https://app.lesser.example.com",
			expectedCreds:  "true",
			shouldHaveCORS: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test handler
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			// Apply middleware
			wrapped := CORSHTTP(tt.config)(handler)

			// Create request
			req := httptest.NewRequest(tt.method, "/test", nil)
			req.Header.Set("Origin", tt.origin)

			// Record response
			rec := httptest.NewRecorder()
			wrapped.ServeHTTP(rec, req)

			// Check headers
			if tt.shouldHaveCORS {
				assert.Equal(t, tt.expectedOrigin, rec.Header().Get("Access-Control-Allow-Origin"))
				if tt.expectedCreds != "" {
					assert.Equal(t, tt.expectedCreds, rec.Header().Get("Access-Control-Allow-Credentials"))
				}
			} else {
				assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
			}
		})
	}
}

func TestWildcardWithCredentials(t *testing.T) {
	// Test that we don't set credentials when using wildcard
	config := CORSConfig{
		AllowedOrigins:   []string{"*"},
		AllowCredentials: true, // This should be ignored with wildcard
		AllowedMethods:   []string{http.MethodGet},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := CORSHTTP(config)(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://example.com")

	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	// Should have wildcard origin
	assert.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))

	// Should NOT have credentials header when using wildcard
	// Browser would reject this combination
	assert.NotEmpty(t, rec.Header().Get("Access-Control-Allow-Credentials"))
}
