package lift

import (
	"sync"
)

// ResponseBuffer stores response data for middleware interception
type ResponseBuffer struct {
	Body         any
	CapturedData any
	Headers      map[string]string
	StatusCode   int
	mu           sync.RWMutex
}

// NewResponseBuffer creates a new response buffer
func NewResponseBuffer() *ResponseBuffer {
	return &ResponseBuffer{
		StatusCode: 200,
		Headers:    make(map[string]string),
	}
}

// SetBody sets the response body and captured data
func (rb *ResponseBuffer) SetBody(body any, capturedData any) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.Body = body
	rb.CapturedData = capturedData
}

// SetStatusCode sets the status code
func (rb *ResponseBuffer) SetStatusCode(code int) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.StatusCode = code
}

// SetHeader sets a header
func (rb *ResponseBuffer) SetHeader(key, value string) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.Headers[key] = value
}

// Get returns all buffer data
func (rb *ResponseBuffer) Get() (body any, statusCode int, headers map[string]string, capturedData any) {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	// Make a copy of headers
	headersCopy := make(map[string]string)
	for k, v := range rb.Headers {
		headersCopy[k] = v
	}

	return rb.Body, rb.StatusCode, headersCopy, rb.CapturedData
}
