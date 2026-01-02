package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractOrigin_HeaderCaseFallback(t *testing.T) {
	origin := extractOrigin(map[string]string{"origin": "https://example.com"})
	assert.Equal(t, "https://example.com", origin)
}

func TestAddVaryHeader_AppendsOrigin(t *testing.T) {
	headers := map[string]string{"Vary": "Accept-Encoding"}
	addVaryHeader(headers)
	assert.Equal(t, "Accept-Encoding, Origin", headers["Vary"])

	addVaryHeader(headers)
	assert.Equal(t, "Accept-Encoding, Origin", headers["Vary"])
}

func TestCORSHTTP_Preflight(t *testing.T) {
	config := DefaultCORSConfig
	config.AllowedOrigins = []string{"https://example.com"}
	config.AllowedMethods = []string{http.MethodGet, http.MethodOptions}
	config.AllowedHeaders = []string{"Content-Type"}
	config.AllowCredentials = true
	config.MaxAge = 42

	wrapped := CORSHTTP(config)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("next handler should not be called for preflight, got %s", r.Method)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	req.Header.Set("Origin", "https://example.com")

	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, "https://example.com", rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", rec.Header().Get("Access-Control-Allow-Credentials"))
	assert.Equal(t, "Origin", rec.Header().Get("Vary"))
	assert.Equal(t, "42", rec.Header().Get("Access-Control-Max-Age"))
}

func TestIsOriginAllowed_Wildcards(t *testing.T) {
	assert.True(t, IsOriginAllowed("https://a.example.com", []string{"*"}))
	assert.True(t, IsOriginAllowed("https://sub.example.com", []string{"*.example.com"}))
	assert.False(t, IsOriginAllowed("https://evil.com", []string{"*.example.com"}))
}

func TestCORS_Preflight_UsesOriginHeaderWhenNotWildcard(t *testing.T) {
	config := DefaultCORSConfig
	config.AllowedOrigins = []string{"https://example.com"}

	wrapped := CORS(config)(func(_ events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
		return &events.APIGatewayV2HTTPResponse{StatusCode: 200}, nil
	})

	req := events.APIGatewayV2HTTPRequest{
		Headers: map[string]string{"Origin": "https://example.com"},
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{Method: http.MethodOptions},
		},
	}

	resp, err := wrapped(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Equal(t, "https://example.com", resp.Headers["Access-Control-Allow-Origin"])
}

