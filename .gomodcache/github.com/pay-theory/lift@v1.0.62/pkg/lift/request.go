package lift

import (
	"context"

	"github.com/pay-theory/lift/pkg/lift/adapters"
)

// Re-export types from adapters for backward compatibility
type TriggerType = adapters.TriggerType

// Request wraps the adapter Request with additional methods and exposes fields directly
type Request struct {
	*adapters.Request

	// Expose adapter fields directly for backward compatibility
	Method      string            `json:"method,omitempty"`
	Path        string            `json:"path,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	QueryParams map[string]string `json:"query_params,omitempty"`
	Body        []byte            `json:"body,omitempty"`
}

// NewRequest creates a new Request from an adapter Request
func NewRequest(adapterReq *adapters.Request) *Request {
	if adapterReq == nil {
		return &Request{
			Request:     &adapters.Request{},
			Headers:     make(map[string]string),
			QueryParams: make(map[string]string),
		}
	}

	return &Request{
		Request:     adapterReq,
		Method:      adapterReq.Method,
		Path:        adapterReq.Path,
		Headers:     adapterReq.Headers,
		QueryParams: adapterReq.QueryParams,
		Body:        adapterReq.Body,
	}
}

// Re-export constants from adapters
const (
	TriggerAPIGateway   = adapters.TriggerAPIGateway
	TriggerAPIGatewayV2 = adapters.TriggerAPIGatewayV2
	TriggerSQS          = adapters.TriggerSQS
	TriggerS3           = adapters.TriggerS3
	TriggerEventBridge  = adapters.TriggerEventBridge
	TriggerWebSocket    = adapters.TriggerWebSocket
	TriggerUnknown      = adapters.TriggerUnknown
)

// RequestContext provides backward compatibility for accessing request context
func (r *Request) RequestContext() map[string]any {
	if r.RawEvent == nil {
		return make(map[string]any)
	}

	// Try to extract request context from raw event
	if eventMap, ok := r.RawEvent.(map[string]any); ok {
		if requestContext, ok := eventMap["requestContext"].(map[string]any); ok {
			return requestContext
		}
	}

	return make(map[string]any)
}

// GetHeader retrieves a header value (case-insensitive)
func (r *Request) GetHeader(key string) string {
	if r.Headers == nil {
		return ""
	}

	// Try exact match first
	if value, exists := r.Headers[key]; exists {
		return value
	}

	// Try case-insensitive match
	for k, v := range r.Headers {
		if equalFold(k, key) {
			return v
		}
	}

	return ""
}

// GetQuery retrieves a query parameter value
func (r *Request) GetQuery(key string) string {
	if r.QueryParams == nil {
		return ""
	}
	return r.QueryParams[key]
}

// GetParam retrieves a path parameter value
func (r *Request) GetParam(key string) string {
	if r.PathParams == nil {
		return ""
	}
	return r.PathParams[key]
}

// equalFold is a simple case-insensitive string comparison
func equalFold(s1, s2 string) bool {
	if len(s1) != len(s2) {
		return false
	}
	for i := 0; i < len(s1); i++ {
		c1, c2 := s1[i], s2[i]
		if c1 >= 'A' && c1 <= 'Z' {
			c1 += 'a' - 'A'
		}
		if c2 >= 'A' && c2 <= 'Z' {
			c2 += 'a' - 'A'
		}
		if c1 != c2 {
			return false
		}
	}
	return true
}

// Context returns the request context (for compatibility)
func (r *Request) Context() context.Context {
	// Return a background context for now
	// In a real implementation, this would be the actual request context
	return context.Background()
}

// Header returns the request headers (for compatibility)
func (r *Request) Header() map[string]string {
	if r.Headers == nil {
		return make(map[string]string)
	}
	return r.Headers
}

// RemoteAddr returns the remote address (for compatibility)
func (r *Request) RemoteAddr() string {
	// In Lambda, this would typically come from the event context
	// For now, return a placeholder
	if r.Headers != nil {
		if xForwardedFor := r.Headers["X-Forwarded-For"]; xForwardedFor != "" {
			return xForwardedFor
		}
		if xRealIP := r.Headers["X-Real-IP"]; xRealIP != "" {
			return xRealIP
		}
	}
	return "127.0.0.1"
}

// UserAgent returns the user agent string (for compatibility)
func (r *Request) UserAgent() string {
	if r.Headers == nil {
		return ""
	}
	return r.Headers["User-Agent"]
}

// URL returns a simple URL structure (for compatibility)
func (r *Request) URL() *SimpleURL {
	return &SimpleURL{
		Path: r.Path,
	}
}

// SimpleURL provides basic URL functionality for compatibility
type SimpleURL struct {
	Path string
}
