package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/aws/aws-lambda-go/events"
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
		"Authorization",
		"Content-Type",
		"X-CSRF-Token",
		"X-Request-ID",
	},
	ExposedHeaders: []string{
		"X-RateLimit-Limit",
		"X-RateLimit-Remaining",
		"X-RateLimit-Reset",
		"X-Request-ID",
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
			origin := request.Headers["Origin"]
			if origin == "" {
				origin = request.Headers["origin"]
			}

			// Check if origin is allowed
			allowed := false
			useWildcard := false
			for _, allowedOrigin := range config.AllowedOrigins {
				if allowedOrigin == "*" {
					allowed = true
					useWildcard = true
					break
				} else if allowedOrigin == origin {
					allowed = true
					break
				}
			}

			// Handle preflight requests
			if request.RequestContext.HTTP.Method == http.MethodOptions {
				response := &events.APIGatewayV2HTTPResponse{
					StatusCode: http.StatusNoContent,
					Headers:    make(map[string]string),
				}

				if allowed {
					if useWildcard {
						response.Headers["Access-Control-Allow-Origin"] = "*"
					} else if origin != "" {
						response.Headers["Access-Control-Allow-Origin"] = origin
					}
				}

				response.Headers["Access-Control-Allow-Methods"] = strings.Join(config.AllowedMethods, ", ")
				response.Headers["Access-Control-Allow-Headers"] = strings.Join(config.AllowedHeaders, ", ")
				response.Headers["Access-Control-Max-Age"] = fmt.Sprintf("%d", config.MaxAge)

				if config.AllowCredentials {
					response.Headers["Access-Control-Allow-Credentials"] = "true"
				}

				// Add Vary header for caching
				response.Headers["Vary"] = "Origin"

				return response, nil
			}

			// Process actual request
			response, err := next(request)
			if err != nil {
				return response, err
			}

			if response.Headers == nil {
				response.Headers = make(map[string]string)
			}

			// Add CORS headers to response
			if allowed {
				if useWildcard {
					response.Headers["Access-Control-Allow-Origin"] = "*"
				} else if origin != "" {
					response.Headers["Access-Control-Allow-Origin"] = origin
				}
			}

			if len(config.ExposedHeaders) > 0 {
				response.Headers["Access-Control-Expose-Headers"] = strings.Join(config.ExposedHeaders, ", ")
			}

			if config.AllowCredentials {
				response.Headers["Access-Control-Allow-Credentials"] = "true"
			}

			// Add Vary header for caching
			existing := response.Headers["Vary"]
			if existing == "" {
				response.Headers["Vary"] = "Origin"
			} else if !strings.Contains(existing, "Origin") {
				response.Headers["Vary"] = existing + ", Origin"
			}

			return response, err
		}
	}
}

// CORSHTTP creates a standard HTTP CORS middleware
func CORSHTTP(config CORSConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Check if origin is allowed
			allowed := false
			useWildcard := false
			for _, allowedOrigin := range config.AllowedOrigins {
				if allowedOrigin == "*" {
					allowed = true
					useWildcard = true
					break
				} else if allowedOrigin == origin {
					allowed = true
					break
				}
			}

			// Handle preflight
			if r.Method == http.MethodOptions {
				if allowed {
					if useWildcard {
						w.Header().Set("Access-Control-Allow-Origin", "*")
					} else if origin != "" {
						w.Header().Set("Access-Control-Allow-Origin", origin)
					}
				}

				w.Header().Set("Access-Control-Allow-Methods", strings.Join(config.AllowedMethods, ", "))
				w.Header().Set("Access-Control-Allow-Headers", strings.Join(config.AllowedHeaders, ", "))
				w.Header().Set("Access-Control-Max-Age", fmt.Sprintf("%d", config.MaxAge))

				if config.AllowCredentials {
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}

				w.Header().Set("Vary", "Origin")
				w.WriteHeader(http.StatusNoContent)
				return
			}

			// Actual request
			if allowed {
				if useWildcard {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				} else if origin != "" {
					w.Header().Set("Access-Control-Allow-Origin", origin)
				}
			}

			if len(config.ExposedHeaders) > 0 {
				w.Header().Set("Access-Control-Expose-Headers", strings.Join(config.ExposedHeaders, ", "))
			}

			if config.AllowCredentials {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}

			// Add Vary header
			existing := w.Header().Get("Vary")
			if existing == "" {
				w.Header().Set("Vary", "Origin")
			} else if !strings.Contains(existing, "Origin") {
				w.Header().Set("Vary", existing+", Origin")
			}

			next.ServeHTTP(w, r)
		})
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
