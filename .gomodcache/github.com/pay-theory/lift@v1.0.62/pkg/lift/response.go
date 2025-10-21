package lift

import (
	"encoding/base64"
	"encoding/json"
)

// Common HTTP content types and headers
const (
	ContentTypeJSON   = "application/json"
	ContentTypeHTML   = "text/html"
	ContentTypeText   = "text/plain"
	HeaderContentType = "Content-Type"
)

// Response represents a unified response structure for Lambda functions
type Response struct {
	// 8-byte aligned fields
	Body    any               `json:"body"`
	Headers map[string]string `json:"headers"`

	// 4-byte aligned fields
	StatusCode int `json:"statusCode"`

	// 1-byte aligned fields
	IsBase64Encoded bool `json:"isBase64Encoded"`
	written         bool
}

// NewResponse creates a new Response with default values
func NewResponse() *Response {
	return &Response{
		StatusCode: 200,
		Headers:    make(map[string]string),
		written:    false,
	}
}

// Status sets the HTTP status code and returns the response for chaining
func (r *Response) Status(code int) *Response {
	r.StatusCode = code
	return r
}

// Header sets a response header
func (r *Response) Header(key, value string) *Response {
	if r.Headers == nil {
		r.Headers = make(map[string]string)
	}
	r.Headers[key] = value
	return r
}

// JSON sets the response body as JSON and marks the response as written
func (r *Response) JSON(data any) error {
	if r.written {
		return NewLiftError(ErrorCodeResponseWritten, "Response has already been written", 500)
	}

	r.Body = data
	r.Header(HeaderContentType, ContentTypeJSON)
	r.written = true
	return nil
}

// Text sets the response body as plain text
func (r *Response) Text(text string) error {
	if r.written {
		return NewLiftError(ErrorCodeResponseWritten, "Response has already been written", 500)
	}

	r.Body = text
	r.Header("Content-Type", "text/plain")
	r.written = true
	return nil
}

// HTML sets the response body as HTML
func (r *Response) HTML(html string) error {
	if r.written {
		return NewLiftError(ErrorCodeResponseWritten, "Response has already been written", 500)
	}

	r.Body = html
	r.Header(HeaderContentType, ContentTypeHTML)
	r.written = true
	return nil
}

// Binary sets the response body as binary data
func (r *Response) Binary(data []byte) error {
	if r.written {
		return NewLiftError(ErrorCodeResponseWritten, "Response has already been written", 500)
	}

	// API Gateway expects base64-encoded string when IsBase64Encoded is true
	r.Body = base64.StdEncoding.EncodeToString(data)
	r.Header("Content-Type", "application/octet-stream")
	r.IsBase64Encoded = true
	r.written = true
	return nil
}

// IsWritten returns whether the response has been written
func (r *Response) IsWritten() bool {
	return r.written
}

// MarshalJSON implements custom JSON marshaling for Lambda response format
func (r *Response) MarshalJSON() ([]byte, error) {
	// Convert body to string if it's not already
	var bodyStr string
	if r.Body != nil {
		switch v := r.Body.(type) {
		case string:
			bodyStr = v
		case []byte:
			if r.IsBase64Encoded {
				bodyStr = base64.StdEncoding.EncodeToString(v)
			} else {
				bodyStr = string(v)
			}
		default:
			// Marshal non-string data as JSON
			jsonData, err := json.Marshal(v)
			if err != nil {
				return nil, NewLiftError(ErrorCodeMarshalError, "Failed to marshal response body", 500).WithCause(err)
			}
			bodyStr = string(jsonData)
		}
	}

	// Create the Lambda response structure
	lambdaResponse := struct {
		Headers         map[string]string `json:"headers"`
		Body            string            `json:"body"`
		StatusCode      int               `json:"statusCode"`
		IsBase64Encoded bool              `json:"isBase64Encoded"`
	}{
		StatusCode:      r.StatusCode,
		Body:            bodyStr,
		Headers:         r.Headers,
		IsBase64Encoded: r.IsBase64Encoded,
	}

	return json.Marshal(lambdaResponse)
}
