package security

import (
	"fmt"
	"net"
	"strings"
)

// IPExtractionError represents an error during IP extraction
type IPExtractionError struct {
	Headers map[string]string
	Message string
}

func (e *IPExtractionError) Error() string {
	return fmt.Sprintf("failed to extract client IP: %s", e.Message)
}

// ExtractClientIP extracts the client's source IP address from various headers and request context.
// It follows the precedence order commonly used in production environments:
// 1. X-Forwarded-For (first IP in comma-separated list)
// 2. X-Real-IP
// 3. CF-Connecting-IP (Cloudflare)
// 4. X-Original-Forwarded-For
// 5. Request context (API Gateway specific)
//
// Returns an error if no valid IP address can be extracted.
func ExtractClientIP(headers map[string]string, requestContext map[string]any) (string, error) {
	extractor := newIPExtractor(headers, requestContext)
	return extractor.extract()
}

// ipExtractor handles IP extraction from various sources
type ipExtractor struct {
	headers         map[string]string
	requestContext  map[string]any
	relevantHeaders map[string]string
	strategies      []ipExtractionStrategy
}

// newIPExtractor creates a new IP extractor with configured strategies
func newIPExtractor(headers map[string]string, requestContext map[string]any) *ipExtractor {
	e := &ipExtractor{
		headers:         headers,
		requestContext:  requestContext,
		relevantHeaders: make(map[string]string),
	}

	// Configure extraction strategies in priority order
	e.strategies = []ipExtractionStrategy{
		&headerStrategy{name: "X-Forwarded-For", multiValue: true},
		&headerStrategy{name: "X-Real-IP", multiValue: false},
		&headerStrategy{name: "CF-Connecting-IP", multiValue: false},
		&headerStrategy{name: "X-Original-Forwarded-For", multiValue: true},
		&contextStrategy{path: []string{"http", "sourceIp"}, label: "requestContext.http.sourceIp"},
		&contextStrategy{path: []string{"identity", "sourceIp"}, label: "requestContext.identity.sourceIp"},
		&contextStrategy{path: []string{"sourceIp"}, label: "requestContext.sourceIp"},
	}

	return e
}

// extract attempts to extract client IP using configured strategies
func (e *ipExtractor) extract() (string, error) {
	for _, strategy := range e.strategies {
		if ip := strategy.extractIP(e); ip != "" {
			return ip, nil
		}
	}

	return "", &IPExtractionError{
		Message: "no valid IP address found in headers or request context",
		Headers: e.relevantHeaders,
	}
}

// ipExtractionStrategy defines the interface for IP extraction strategies
type ipExtractionStrategy interface {
	extractIP(e *ipExtractor) string
}

// headerStrategy extracts IP from HTTP headers
type headerStrategy struct {
	name       string
	multiValue bool // whether the header contains comma-separated values
}

// extractIP implements the extraction logic for header-based strategies
func (h *headerStrategy) extractIP(e *ipExtractor) string {
	value, ok := e.headers[h.name]
	if !ok || value == "" {
		return ""
	}

	e.relevantHeaders[h.name] = value

	if h.multiValue {
		// Handle comma-separated list of IPs
		ips := strings.Split(value, ",")
		if len(ips) > 0 {
			sourceIP := strings.TrimSpace(ips[0])
			if isValidIP(sourceIP) {
				return stripPort(sourceIP)
			}
		}
	} else if isValidIP(value) {
		// Single IP value
		return stripPort(value)
	}

	return ""
}

// contextStrategy extracts IP from request context
type contextStrategy struct {
	label string
	path  []string
}

// extractIP implements the extraction logic for context-based strategies
func (c *contextStrategy) extractIP(e *ipExtractor) string {
	if e.requestContext == nil {
		return ""
	}

	value := c.navigateContext(e.requestContext, c.path)
	if value == "" {
		return ""
	}

	if isValidIP(value) {
		return stripPort(value)
	}

	// Record for error reporting even if invalid
	e.relevantHeaders[c.label] = value
	return ""
}

// navigateContext traverses the context map following the given path
func (c *contextStrategy) navigateContext(context map[string]any, path []string) string {
	current := any(context)

	for _, key := range path {
		if m, ok := current.(map[string]any); ok {
			current = m[key]
		} else {
			return ""
		}
	}

	if str, ok := current.(string); ok {
		return str
	}

	return ""
}

// stripPort removes port from IP address if present
func stripPort(ip string) string {
	// Handle IPv4 with port
	if strings.Contains(ip, ":") && !strings.Contains(ip, "::") {
		host, _, err := net.SplitHostPort(ip)
		if err == nil {
			return host
		}
	}
	return ip
}

// isValidIP checks if the given string is a valid IP address
func isValidIP(ip string) bool {
	// Remove port if present
	if strings.Contains(ip, ":") && !strings.Contains(ip, "::") {
		// IPv4 with port
		host, _, err := net.SplitHostPort(ip)
		if err == nil {
			ip = host
		}
	}

	return net.ParseIP(ip) != nil
}
