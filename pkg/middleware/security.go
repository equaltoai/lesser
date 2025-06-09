package middleware

import (
	"net/http"

	"github.com/aws/aws-lambda-go/events"
)

// SecurityHeaders adds security headers to API Gateway responses
func SecurityHeaders(next func(events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error)) func(events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	return func(request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
		// Call the next handler
		response, err := next(request)
		if err != nil {
			return response, err
		}

		// Add security headers
		if response.Headers == nil {
			response.Headers = make(map[string]string)
		}

		// Prevent MIME type sniffing
		response.Headers["X-Content-Type-Options"] = "nosniff"

		// Prevent clickjacking
		response.Headers["X-Frame-Options"] = "DENY"

		// Enable XSS filter
		response.Headers["X-XSS-Protection"] = "1; mode=block"

		// Control referrer information
		response.Headers["Referrer-Policy"] = "strict-origin-when-cross-origin"

		// Content Security Policy
		response.Headers["Content-Security-Policy"] = "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline';"

		// HSTS for HTTPS enforcement (only in production)
		// Since Lambda/API Gateway handles TLS termination, always add this
		response.Headers["Strict-Transport-Security"] = "max-age=31536000; includeSubDomains"

		// Permissions Policy (formerly Feature Policy)
		response.Headers["Permissions-Policy"] = "geolocation=(), microphone=(), camera=()"

		return response, err
	}
}

// SecurityHeadersHTTP is a standard HTTP middleware for security headers
func SecurityHeadersHTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Security headers
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline';")
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

		// HSTS for HTTPS enforcement
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		next.ServeHTTP(w, r)
	})
}

// SecurityConfig allows customization of security headers
type SecurityConfig struct {
	ContentTypeOptions      string
	FrameOptions            string
	XSSProtection           string
	ReferrerPolicy          string
	ContentSecurityPolicy   string
	StrictTransportSecurity string
	PermissionsPolicy       string
}

// DefaultSecurityConfig provides secure defaults
var DefaultSecurityConfig = SecurityConfig{
	ContentTypeOptions:      "nosniff",
	FrameOptions:            "DENY",
	XSSProtection:           "1; mode=block",
	ReferrerPolicy:          "strict-origin-when-cross-origin",
	ContentSecurityPolicy:   "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline';",
	StrictTransportSecurity: "max-age=31536000; includeSubDomains",
	PermissionsPolicy:       "geolocation=(), microphone=(), camera=()",
}

// SecurityHeadersWithConfig adds configurable security headers
func SecurityHeadersWithConfig(config SecurityConfig) func(func(events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error)) func(events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	return func(next func(events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error)) func(events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
		return func(request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
			response, err := next(request)
			if err != nil {
				return response, err
			}

			if response.Headers == nil {
				response.Headers = make(map[string]string)
			}

			// Apply configured headers
			if config.ContentTypeOptions != "" {
				response.Headers["X-Content-Type-Options"] = config.ContentTypeOptions
			}
			if config.FrameOptions != "" {
				response.Headers["X-Frame-Options"] = config.FrameOptions
			}
			if config.XSSProtection != "" {
				response.Headers["X-XSS-Protection"] = config.XSSProtection
			}
			if config.ReferrerPolicy != "" {
				response.Headers["Referrer-Policy"] = config.ReferrerPolicy
			}
			if config.ContentSecurityPolicy != "" {
				response.Headers["Content-Security-Policy"] = config.ContentSecurityPolicy
			}
			if config.StrictTransportSecurity != "" {
				response.Headers["Strict-Transport-Security"] = config.StrictTransportSecurity
			}
			if config.PermissionsPolicy != "" {
				response.Headers["Permissions-Policy"] = config.PermissionsPolicy
			}

			return response, err
		}
	}
}
