package lift

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/go-playground/validator/v10"
)

// Context represents a request context
type Context struct {
	context.Context
	Request     events.APIGatewayProxyRequest
	Response    events.APIGatewayProxyResponse
	Method      string
	Path        string
	RequestID   string
	Deadline    time.Time
	PathParams  map[string]string
	QueryParams map[string]string
	Headers     map[string]string
	Body        []byte
	tenantID    string
	userID      string
	values      map[string]interface{}
	validate    *validator.Validate
}

// NewContext creates a new context
func NewContext(ctx context.Context, request events.APIGatewayProxyRequest, requestID string, deadline time.Time) *Context {
	// Initialize response
	response := events.APIGatewayProxyResponse{
		StatusCode: 200,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
	}

	// Parse body
	var body []byte
	if request.Body != "" {
		if request.IsBase64Encoded {
			body, _ = base64.StdEncoding.DecodeString(request.Body)
		} else {
			body = []byte(request.Body)
		}
	}

	// Create context
	return &Context{
		Context:     ctx,
		Request:     request,
		Response:    response,
		Method:      request.HTTPMethod,
		Path:        request.Path,
		RequestID:   requestID,
		Deadline:    deadline,
		PathParams:  request.PathParameters,
		QueryParams: request.QueryStringParameters,
		Headers:     request.Headers,
		Body:        body,
		values:      make(map[string]interface{}),
		validate:    validator.New(),
	}
}

// ParseRequest parses the request body into the given struct
func (c *Context) ParseRequest(v interface{}) error {
	// Skip if body is empty
	if len(c.Body) == 0 {
		return nil
	}

	// Parse JSON
	if err := json.Unmarshal(c.Body, v); err != nil {
		return BadRequest("Invalid JSON: " + err.Error())
	}

	// Validate struct
	if err := c.validate.Struct(v); err != nil {
		return ValidationError(err.Error())
	}

	return nil
}

// JSON sends a JSON response
func (c *Context) JSON(v interface{}) error {
	// Marshal JSON
	body, err := json.Marshal(v)
	if err != nil {
		return InternalServerError("Failed to marshal JSON: " + err.Error())
	}

	// Set response
	c.Response.StatusCode = 200
	c.Response.Body = string(body)
	c.Response.Headers["Content-Type"] = "application/json"

	return nil
}

// JSONWithStatus sends a JSON response with a custom status code
func (c *Context) JSONWithStatus(v interface{}, statusCode int) error {
	// Marshal JSON
	body, err := json.Marshal(v)
	if err != nil {
		return InternalServerError("Failed to marshal JSON: " + err.Error())
	}

	// Set response
	c.Response.StatusCode = statusCode
	c.Response.Body = string(body)
	c.Response.Headers["Content-Type"] = "application/json"

	return nil
}

// String sends a string response
func (c *Context) String(s string) error {
	c.Response.StatusCode = 200
	c.Response.Body = s
	c.Response.Headers["Content-Type"] = "text/plain"
	return nil
}

// StringWithStatus sends a string response with a custom status code
func (c *Context) StringWithStatus(s string, statusCode int) error {
	c.Response.StatusCode = statusCode
	c.Response.Body = s
	c.Response.Headers["Content-Type"] = "text/plain"
	return nil
}

// NoContent sends a response with no content
func (c *Context) NoContent(statusCode int) error {
	c.Response.StatusCode = statusCode
	c.Response.Body = ""
	delete(c.Response.Headers, "Content-Type")
	return nil
}

// Redirect sends a redirect response
func (c *Context) Redirect(url string, statusCode int) error {
	c.Response.StatusCode = statusCode
	c.Response.Headers["Location"] = url
	c.Response.Body = ""
	return nil
}

// Error sends an error response
func (c *Context) Error(statusCode int, message string) error {
	return NewError(statusCode, message, nil)
}

// SetHeader sets a response header
func (c *Context) SetHeader(key, value string) {
	c.Response.Headers[key] = value
}

// Header gets a request header
func (c *Context) Header(key string) string {
	return c.Headers[key]
}

// Query gets a query parameter
func (c *Context) Query(key string) string {
	return c.QueryParams[key]
}

// Param gets a path parameter
func (c *Context) Param(key string) string {
	return c.PathParams[key]
}

// Set sets a value in the context
func (c *Context) Set(key string, value interface{}) {
	c.values[key] = value
}

// Get gets a value from the context
func (c *Context) Get(key string) interface{} {
	return c.values[key]
}

// GetString gets a string value from the context
func (c *Context) GetString(key string) string {
	if value, ok := c.values[key].(string); ok {
		return value
	}
	return ""
}

// GetBool gets a boolean value from the context
func (c *Context) GetBool(key string) bool {
	if value, ok := c.values[key].(bool); ok {
		return value
	}
	return false
}

// GetInt gets an integer value from the context
func (c *Context) GetInt(key string) int {
	if value, ok := c.values[key].(int); ok {
		return value
	}
	return 0
}

// SetTenantID sets the tenant ID
func (c *Context) SetTenantID(tenantID string) {
	c.tenantID = tenantID
}

// TenantID gets the tenant ID
func (c *Context) TenantID() string {
	return c.tenantID
}

// SetUserID sets the user ID
func (c *Context) SetUserID(userID string) {
	c.userID = userID
}

// UserID gets the user ID
func (c *Context) UserID() string {
	return c.userID
}

// FormValue gets a form value
func (c *Context) FormValue(key string) string {
	// Check if content type is form
	contentType := c.Header("Content-Type")
	if !strings.Contains(contentType, "application/x-www-form-urlencoded") {
		return ""
	}

	// Parse form
	form, err := url.ParseQuery(string(c.Body))
	if err != nil {
		return ""
	}

	return form.Get(key)
}

// FormFile gets a form file (not implemented for Lambda)
func (c *Context) FormFile(key string) (multipart.File, *multipart.FileHeader, error) {
	return nil, nil, ErrNotImplemented
}

// MultipartForm gets the multipart form (not implemented for Lambda)
func (c *Context) MultipartForm() (*multipart.Form, error) {
	return nil, ErrNotImplemented
}

// Bind binds the request body to the given struct
func (c *Context) Bind(v interface{}) error {
	return c.ParseRequest(v)
}

// Validate validates the given struct
func (c *Context) Validate(v interface{}) error {
	if err := c.validate.Struct(v); err != nil {
		return ValidationError(err.Error())
	}
	return nil
}

// ErrNotImplemented is returned when a feature is not implemented
var ErrNotImplemented = NewError(501, "Not implemented", nil)

// multipart is a stub for the multipart package
type multipart struct{}

// File is a stub for multipart.File
type File interface {
	io.Reader
	io.ReaderAt
	io.Seeker
	io.Closer
}

// FileHeader is a stub for multipart.FileHeader
type FileHeader struct {
	Filename string
	Header   http.Header
	Size     int64
}

// Form is a stub for multipart.Form
type Form struct {
	Value map[string][]string
	File  map[string][]*FileHeader
}
