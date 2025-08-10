// Package middleware provides CORS configuration and HTTP middleware utilities for API Gateway Lambda functions.
package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/equaltoai/lesser/pkg/common"
)

// CORSConfig holds CORS configuration
type CORSConfig struct {
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
	ExposedHeaders   []string
	AllowCredentials bool
	MaxAge           int
}

// DefaultCORSConfig provides secure defaults
var DefaultCORSConfig = CORSConfig{
	AllowedOrigins: []string{
		"https://lesser.example.com",
		"https://app.lesser.example.com",
	},
	AllowedMethods: []string{
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodDelete,
		http.MethodOptions,
	},
	AllowedHeaders: []string{
		common.AuthorizationHeader,
		common.ContentTypeHeader,
		"X-CSRF-Token",
		common.XRequestIDHeader,
	},
	ExposedHeaders: []string{
		"X-RateLimit-Limit",
		"X-RateLimit-Remaining",
		"X-RateLimit-Reset",
		common.XRequestIDHeader,
	},
	AllowCredentials: true,
	MaxAge:           86400, // 24 hours
}

// ActivityPubCORSConfig provides CORS config for ActivityPub endpoints
var ActivityPubCORSConfig = CORSConfig{
	AllowedOrigins: []string{"*"}, // Federation requires accepting from any domain
	AllowedMethods: []string{
		http.MethodGet,
		http.MethodPost,
		http.MethodOptions,
	},
	AllowedHeaders: []string{
		"Accept",
		"Content-Type",
		"Date",
		"Digest",
		"Host",
		"Signature",
		"User-Agent",
		"X-Forwarded-For",
		"X-Forwarded-Proto",
	},
	ExposedHeaders: []string{
		"X-RateLimit-Limit",
		"X-RateLimit-Remaining",
		"X-RateLimit-Reset",
	},
	AllowCredentials: false, // Cannot use credentials with wildcard origin
	MaxAge:           86400, // 24 hours
}

// CORS creates a CORS middleware for API Gateway
func CORS(config CORSConfig) func(func(events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error)) func(events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	return func(next func(events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error)) func(events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
		return func(request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
			origin := extractOrigin(request.Headers)
			allowed, useWildcard := checkOriginAllowed(origin, config.AllowedOrigins)

			// Handle preflight requests
			if request.RequestContext.HTTP.Method == http.MethodOptions {
				return handlePreflight(config, origin, allowed, useWildcard)
			}

			// Process actual request
			return handleActualRequest(next, request, config, origin, allowed, useWildcard)
		}
	}
}

// extractOrigin extracts the origin from request headers
func extractOrigin(headers map[string]string) string {
	origin := headers["Origin"]
	if origin == "" {
		origin = headers["origin"]
	}
	return origin
}

// checkOriginAllowed checks if the origin is allowed
func checkOriginAllowed(origin string, allowedOrigins []string) (allowed bool, useWildcard bool) {
	for _, allowedOrigin := range allowedOrigins {
		if allowedOrigin == "*" {
			return true, true
		}
		if allowedOrigin == origin {
			return true, false
		}
	}
	return false, false
}

// handlePreflight handles CORS preflight requests
func handlePreflight(config CORSConfig, origin string, allowed, useWildcard bool) (*events.APIGatewayV2HTTPResponse, error) {
	response := &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusNoContent,
		Headers:    make(map[string]string),
	}

	// Set origin header
	setOriginHeader(response.Headers, origin, allowed, useWildcard)

	// Set preflight headers
	setPreflightHeaders(response.Headers, config)

	// Add Vary header for caching
	response.Headers["Vary"] = "Origin"

	return response, nil
}

// setOriginHeader sets the Access-Control-Allow-Origin header
func setOriginHeader(headers map[string]string, origin string, allowed, useWildcard bool) {
	if !allowed {
		return
	}

	if useWildcard {
		headers["Access-Control-Allow-Origin"] = "*"
	} else if origin != "" {
		headers["Access-Control-Allow-Origin"] = origin
	}
}

// setPreflightHeaders sets headers for preflight responses
func setPreflightHeaders(headers map[string]string, config CORSConfig) {
	headers["Access-Control-Allow-Methods"] = strings.Join(config.AllowedMethods, ", ")
	headers["Access-Control-Allow-Headers"] = strings.Join(config.AllowedHeaders, ", ")
	headers["Access-Control-Max-Age"] = fmt.Sprintf("%d", config.MaxAge)

	if config.AllowCredentials {
		headers["Access-Control-Allow-Credentials"] = "true"
	}
}

// handleActualRequest handles non-preflight requests
func handleActualRequest(next func(events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error),
	request events.APIGatewayV2HTTPRequest, config CORSConfig, origin string, allowed, useWildcard bool) (*events.APIGatewayV2HTTPResponse, error) {

	response, err := next(request)
	if err != nil {
		return response, err
	}

	if response.Headers == nil {
		response.Headers = make(map[string]string)
	}

	// Set CORS headers
	setCORSHeaders(response.Headers, config, origin, allowed, useWildcard)

	// Add Vary header for caching
	addVaryHeader(response.Headers)

	return response, err
}

// setCORSHeaders sets CORS headers for actual responses
func setCORSHeaders(headers map[string]string, config CORSConfig, origin string, allowed, useWildcard bool) {
	// Set origin header
	setOriginHeader(headers, origin, allowed, useWildcard)

	// Set exposed headers
	if len(config.ExposedHeaders) > 0 {
		headers["Access-Control-Expose-Headers"] = strings.Join(config.ExposedHeaders, ", ")
	}

	// Set credentials
	if config.AllowCredentials {
		headers["Access-Control-Allow-Credentials"] = "true"
	}
}

// addVaryHeader adds or updates the Vary header
func addVaryHeader(headers map[string]string) {
	existing := headers["Vary"]
	if existing == "" {
		headers["Vary"] = "Origin"
	} else if !strings.Contains(existing, "Origin") {
		headers["Vary"] = existing + ", Origin"
	}
}

// CORSHTTP creates a standard HTTP CORS middleware
func CORSHTTP(config CORSConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			allowed, useWildcard := checkOriginAllowed(origin, config.AllowedOrigins)

			// Handle preflight
			if r.Method == http.MethodOptions {
				handleHTTPPreflight(w, config, origin, allowed, useWildcard)
				return
			}

			// Handle actual request
			setHTTPCORSHeaders(w, config, origin, allowed, useWildcard)
			addHTTPVaryHeader(w)
			next.ServeHTTP(w, r)
		})
	}
}

// handleHTTPPreflight handles HTTP preflight requests
func handleHTTPPreflight(w http.ResponseWriter, config CORSConfig, origin string, allowed, useWildcard bool) {
	// Set origin header
	setHTTPOriginHeader(w, origin, allowed, useWildcard)

	// Set preflight headers
	setHTTPPreflightHeaders(w, config)

	// Add Vary header
	w.Header().Set("Vary", "Origin")
	w.WriteHeader(http.StatusNoContent)
}

// setHTTPOriginHeader sets the origin header for HTTP responses
func setHTTPOriginHeader(w http.ResponseWriter, origin string, allowed, useWildcard bool) {
	if !allowed {
		return
	}

	if useWildcard {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	} else if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	}
}

// setHTTPPreflightHeaders sets preflight headers for HTTP responses
func setHTTPPreflightHeaders(w http.ResponseWriter, config CORSConfig) {
	w.Header().Set("Access-Control-Allow-Methods", strings.Join(config.AllowedMethods, ", "))
	w.Header().Set("Access-Control-Allow-Headers", strings.Join(config.AllowedHeaders, ", "))
	w.Header().Set("Access-Control-Max-Age", fmt.Sprintf("%d", config.MaxAge))

	if config.AllowCredentials {
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}
}

// setHTTPCORSHeaders sets CORS headers for HTTP responses
func setHTTPCORSHeaders(w http.ResponseWriter, config CORSConfig, origin string, allowed, useWildcard bool) {
	// Set origin header
	setHTTPOriginHeader(w, origin, allowed, useWildcard)

	// Set exposed headers
	if len(config.ExposedHeaders) > 0 {
		w.Header().Set("Access-Control-Expose-Headers", strings.Join(config.ExposedHeaders, ", "))
	}

	// Set credentials
	if config.AllowCredentials {
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}
}

// addHTTPVaryHeader adds or updates the Vary header for HTTP responses
func addHTTPVaryHeader(w http.ResponseWriter) {
	existing := w.Header().Get("Vary")
	if existing == "" {
		w.Header().Set("Vary", "Origin")
	} else if !strings.Contains(existing, "Origin") {
		w.Header().Set("Vary", existing+", Origin")
	}
}

// IsOriginAllowed checks if an origin is allowed
func IsOriginAllowed(origin string, allowedOrigins []string) bool {
	for _, allowed := range allowedOrigins {
		if allowed == "*" {
			return true
		}
		if allowed == origin {
			return true
		}
		// Support wildcard subdomains
		if strings.HasPrefix(allowed, "*.") {
			domain := allowed[2:]
			if strings.HasSuffix(origin, domain) {
				return true
			}
		}
	}
	return false
}
