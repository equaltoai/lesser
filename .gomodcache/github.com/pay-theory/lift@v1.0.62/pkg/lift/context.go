package lift

// Context represents the enhanced context for Lambda handlers in the Lift framework.
// It provides a unified interface for accessing request data, writing responses,
// managing authentication claims, logging, and metrics. The Context struct is
// designed to be used as the first parameter in handler functions.
//
// Key features:
//   - Request and response access
//   - Path and query parameter access
//   - Header access
//   - Authentication and authorization
//   - Structured logging
//   - Metrics collection
//   - Error handling
//   - Multi-tenant support
//
// The Context struct is designed to be passed to handler functions and provides
// all the necessary functionality for handling requests and writing responses
// in a type-safe and consistent manner.

import (
	"context"
	"encoding/json"
	"time"

	"github.com/aws/aws-lambda-go/events"
)

// Validator interface for request validation
type Validator interface {
	Validate(any) error
}

// Context represents the enhanced context for Lambda handlers
// The Context struct provides a unified interface for accessing request data,
// writing responses, managing authentication claims, logging, and metrics.
//
// Fields:
//   - Context: The parent context.Context
//   - Request: The current request
//   - Response: The response to be sent
//   - Logger: The logger for structured logging
//   - Metrics: The metrics collector
//   - validator: The validator for request validation
//   - params: Path parameters
//   - values: Arbitrary values stored in the context
//   - DB: A generic database connection
//   - claims: JWT claims
//   - responseBuffer: A buffer for capturing response data
//   - startTime: The time the request started
//   - RequestID: The request ID
//   - isAuthenticated: Whether the request is authenticated
//   - bufferingEnabled: Whether response buffering is enabled
type Context struct {
	context.Context

	// 8-byte aligned fields (pointers, interfaces, maps)
	Request        *Request
	Response       *Response
	Logger         Logger
	Metrics        MetricsCollector
	Tracer         any
	validator      Validator
	params         map[string]string
	values         map[string]any
	DB             any
	claims         map[string]any
	responseBuffer *ResponseBuffer

	// Performance tracking (24 bytes)
	startTime time.Time

	// Lambda-specific (16 bytes)
	RequestID string

	// Boolean flags (1 byte each)
	isAuthenticated  bool
	bufferingEnabled bool
}

const (
	cacheKeySQSRecords = "__lift_sqs_records"
	cacheKeyS3Records  = "__lift_s3_records"
)

// NewContext creates a new enhanced context for handling a request.
//
// Parameters:
//   - baseCtx: The parent context.Context
//   - req: The request to be handled
//
// Returns:
//   - A pointer to the newly created Context
//
// Example:
//
//	ctx := lift.NewContext(context.Background(), req)
func NewContext(baseCtx context.Context, req *Request) *Context {
	return &Context{
		Context:         baseCtx,
		Request:         req,
		Response:        NewResponse(),
		params:          make(map[string]string),
		values:          make(map[string]any),
		claims:          nil, // Initialize as nil, will be set when claims are provided
		startTime:       time.Now(),
		isAuthenticated: false,
	}
}

// Param retrieves a path parameter from the request.
// If the parameter does not exist, an empty string is returned.
//
// Parameters:
//   - key: The name of the path parameter to retrieve
//
// Returns:
//   - The value of the path parameter, or an empty string if not found
//
// Example:
//
//	userID := ctx.Param("user_id")
func (c *Context) Param(key string) string {
	return c.params[key]
}

// Query retrieves a query parameter from the request.
// If the parameter does not exist, an empty string is returned.
//
// Parameters:
//   - key: The name of the query parameter to retrieve
//
// Returns:
//   - The value of the query parameter, or an empty string if not found
//
// Example:
//
//	searchTerm := ctx.Query("q")
func (c *Context) Query(key string) string {
	if c.Request == nil || c.Request.QueryParams == nil {
		return ""
	}
	return c.Request.QueryParams[key]
}

// Header retrieves a request header from the request.
// If the header does not exist, an empty string is returned.
//
// Parameters:
//   - key: The name of the header to retrieve
//
// Returns:
//   - The value of the header, or an empty string if not found
//
// Example:
//
//	authHeader := ctx.Header("Authorization")
func (c *Context) Header(key string) string {
	if c.Request == nil || c.Request.Headers == nil {
		return ""
	}
	return c.Request.Headers[key]
}

// Set stores a value in the context.
// This is useful for passing data between middleware and handlers.
//
// Parameters:
//   - key: The key to store the value under
//   - value: The value to store
//
// Example:
//
//	ctx.Set("user_id", "user_123")
func (c *Context) Set(key string, value any) {
	if c.values == nil {
		c.values = make(map[string]any)
	}
	c.values[key] = value
}

// Get retrieves a value from the context.
// This is useful for accessing data stored by middleware or other handlers.
//
// Parameters:
//   - key: The key to retrieve the value for
//
// Returns:
//   - The value stored under the key, or nil if not found
//
// Example:
//
//	userID := ctx.Get("user_id").(string)
func (c *Context) Get(key string) any {
	if c.values == nil {
		return nil
	}
	return c.values[key]
}

// SetTracer assigns the tracer implementation to the context.
func (c *Context) SetTracer(tracer any) {
	c.Tracer = tracer
	c.Set("tracer", tracer)
}

// GetTracer returns the tracer associated with the context.
func (c *Context) GetTracer() any {
	if c.Tracer != nil {
		return c.Tracer
	}
	return c.Get("tracer")
}

// SQSRecords returns the typed AWS SQS event records if present on the request.
// The result is cached after the first call to avoid repeated decoding.
func (c *Context) SQSRecords() ([]events.SQSMessage, error) {
	if cached := c.Get(cacheKeySQSRecords); cached != nil {
		if records, ok := cached.([]events.SQSMessage); ok {
			return records, nil
		}
	}

	var event events.SQSEvent
	decoded, err := c.decodeEventRecords(&event)
	if err != nil {
		return nil, err
	}
	if !decoded || len(event.Records) == 0 {
		return nil, nil
	}

	c.Set(cacheKeySQSRecords, event.Records)
	return event.Records, nil
}

// S3Records returns the typed AWS S3 event records if present on the request.
// The result is cached after the first call to avoid repeated decoding.
func (c *Context) S3Records() ([]events.S3EventRecord, error) {
	if cached := c.Get(cacheKeyS3Records); cached != nil {
		if records, ok := cached.([]events.S3EventRecord); ok {
			return records, nil
		}
	}

	var event events.S3Event
	decoded, err := c.decodeEventRecords(&event)
	if err != nil {
		return nil, err
	}
	if !decoded || len(event.Records) == 0 {
		return nil, nil
	}

	c.Set(cacheKeyS3Records, event.Records)
	return event.Records, nil
}

// decodeEventRecords populates the provided event struct with the request records.
// Returns false when no records are available on the current request.
func (c *Context) decodeEventRecords(target any) (bool, error) {
	if c.Request == nil || len(c.Request.Records) == 0 {
		return false, nil
	}

	payload := map[string]any{"Records": c.Request.Records}
	raw, err := json.Marshal(payload)
	if err != nil {
		return false, err
	}

	if err := json.Unmarshal(raw, target); err != nil {
		return false, err
	}

	return true, nil
}

// UserID retrieves the current user ID from the context.
// This is typically set by authentication middleware or by the application itself.
//
// Returns:
//   - The current user ID, or an empty string if not set
//
// Example:
//
//	userID := ctx.UserID()
func (c *Context) UserID() string {
	if userID, ok := c.values["user_id"].(string); ok {
		return userID
	}
	return ""
}

// TenantID retrieves the current tenant ID from the context.
// This is typically set by authentication middleware or by the application itself.
//
// Returns:
//   - The current tenant ID, or an empty string if not set
//
// Example:
//
//	tenantID := ctx.TenantID()
func (c *Context) TenantID() string {
	if tenantID, ok := c.values["tenant_id"].(string); ok {
		return tenantID
	}
	return ""
}

// AccountID retrieves the current account ID from the context.
// This is typically set by authentication middleware or by the application itself.
//
// Returns:
//   - The current account ID, or an empty string if not set
//
// Example:
//
//	accountID := ctx.AccountID()
func (c *Context) AccountID() string {
	if accountID, ok := c.values["account_id"].(string); ok {
		return accountID
	}
	return ""
}

// SetParam sets a path parameter (used by router)
func (c *Context) SetParam(key, value string) {
	c.params[key] = value
}

// EnableResponseBuffering enables response buffering for this context
// This allows middleware to intercept and access response data after handler execution
func (c *Context) EnableResponseBuffering() {
	if !c.bufferingEnabled {
		c.responseBuffer = NewResponseBuffer()
		c.bufferingEnabled = true
	}
}

// GetResponseBuffer returns the response buffer if buffering is enabled
func (c *Context) GetResponseBuffer() *ResponseBuffer {
	if c.bufferingEnabled {
		return c.responseBuffer
	}
	return nil
}

// FlushResponse is a no-op since buffering just captures data
func (c *Context) FlushResponse() error {
	// No explicit flush needed as we're just capturing data
	return nil
}

// captureResponseData captures response data in the buffer if enabled
func (c *Context) captureResponseData() {
	if c.bufferingEnabled && c.responseBuffer != nil {
		c.responseBuffer.SetBody(c.Response.Body, c.Response.Body)
		c.responseBuffer.SetStatusCode(c.Response.StatusCode)
		for k, v := range c.Response.Headers {
			c.responseBuffer.SetHeader(k, v)
		}
	}
}

// JSON sets the response body as JSON.
// This method serializes the provided data to JSON and sets it as the response body.
// It also captures the response data if response buffering is enabled.
//
// Parameters:
//   - data: The data to serialize to JSON
//
// Returns:
//   - An error if the JSON serialization fails
//
// Example:
//
//	type Response struct {
//	    Message string `json:"message"`
//	}
//	ctx.JSON(Response{Message: "Hello, world!"})
func (c *Context) JSON(data any) error {
	err := c.Response.JSON(data)
	if err == nil {
		c.captureResponseData()
	}
	return err
}

// Text sends a text response.
// This method sets the response body to the provided text and sets the Content-Type
// header to "text/plain". It also captures the response data if response buffering
// is enabled.
//
// Parameters:
//   - text: The text to send as the response body
//
// Returns:
//   - An error if setting the response body fails
//
// Example:
//
//	ctx.Text("Hello, world!")
func (c *Context) Text(text string) error {
	err := c.Response.Text(text)
	if err == nil {
		c.captureResponseData()
	}
	return err
}

// HTML sends an HTML response.
// This method sets the response body to the provided HTML and sets the Content-Type
// header to "text/html". It also captures the response data if response buffering
// is enabled.
//
// Parameters:
//   - html: The HTML to send as the response body
//
// Returns:
//   - An error if setting the response body fails
//
// Example:
//
//	ctx.HTML("<html><body><h1>Hello, world!</h1></body></html>")
func (c *Context) HTML(html string) error {
	err := c.Response.HTML(html)
	if err == nil {
		c.captureResponseData()
	}
	return err
}

// Status sets the response status code
func (c *Context) Status(code int) *Context {
	c.Response.StatusCode = code
	return c
}

// ParseRequest parses the request body into the provided interface.
// This method parses the request body as JSON and validates it if a validator
// is available. It returns an error if the request body is empty, the JSON is
// invalid, or the validation fails.
//
// Parameters:
//   - v: A pointer to the struct to parse the request body into
//
// Returns:
//   - An error if the request body is empty, the JSON is invalid, or the validation fails
//
// Example:
//
//	type CreateUserRequest struct {
//	    Name string `json:"name" validate:"required"`
//	    Age  int    `json:"age" validate:"min=0,max=120"`
//	}
//
//	var req CreateUserRequest
//	if err := ctx.ParseRequest(&req); err != nil {
//	    return err
//	}
func (c *Context) ParseRequest(v any) error {
	if c.Request == nil {
		return NewLiftError("EMPTY_BODY", "Request body is empty", 400)
	}

	body := c.Request.Body
	if len(body) == 0 {
		switch {
		case len(c.Request.Records) > 0:
			encoded, err := json.Marshal(c.Request.Records)
			if err != nil {
				return NewLiftError("INVALID_JSON", "Failed to encode event records", 400).WithCause(err)
			}
			body = encoded
		case c.Request.RawEvent != nil:
			encoded, err := json.Marshal(c.Request.RawEvent)
			if err != nil {
				return NewLiftError("INVALID_JSON", "Failed to encode raw event", 400).WithCause(err)
			}
			body = encoded
		default:
			return NewLiftError("EMPTY_BODY", "Request body is empty", 400)
		}
	}

	// Parse JSON
	if err := json.Unmarshal(body, v); err != nil {
		return NewLiftError("INVALID_JSON", "Invalid JSON in request body", 400).WithCause(err)
	}

	// Validate if validator is available
	if c.validator != nil {
		if err := c.validator.Validate(v); err != nil {
			return NewLiftError("VALIDATION_ERROR", "Validation failed", 400).WithCause(err)
		}
	}

	return nil
}

// WithTimeout executes a function with a timeout.
// This method runs the provided function in a goroutine and waits for it to complete
// within the specified duration. If the function does not complete in time, it returns
// a timeout error.
//
// Parameters:
//   - duration: The maximum duration to wait for the function to complete
//   - fn: The function to execute
//
// Returns:
//   - The result of the function, or nil if the function timed out
//   - An error if the function timed out or returned an error
//
// Example:
//
//	result, err := ctx.WithTimeout(5*time.Second, func() (any, error) {
//	    // Do some work
//	    return "result", nil
//	})
//	if err != nil {
//	    // Handle error
//	}
func (c *Context) WithTimeout(duration time.Duration, fn func() (any, error)) (any, error) {
	ctx, cancel := context.WithTimeout(c.Context, duration)
	defer cancel()

	type result struct {
		value any
		err   error
	}

	ch := make(chan result, 1)
	go func() {
		value, err := fn()
		ch <- result{value, err}
	}()

	select {
	case res := <-ch:
		return res.value, res.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Duration returns the time elapsed since the request started
func (c *Context) Duration() time.Duration {
	return time.Since(c.startTime)
}

// SetValidator sets the validator for request validation.
// This method sets the validator to be used for validating request bodies.
// The validator is used by the ParseRequest method to validate the request
// body after it has been parsed.
//
// Parameters:
//   - validator: The validator to use for request validation
//
// Example:
//
//	type CreateUserRequest struct {
//	    Name string `json:"name" validate:"required"`
//	    Age  int    `json:"age" validate:"min=0,max=120"`
//	}
//
//	ctx.SetValidator(myValidator)
func (c *Context) SetValidator(validator Validator) {
	c.validator = validator
}

// SetRequestID sets the request ID in the context.
// This method sets the request ID in the context and stores it in the values map.
// The request ID is typically set by middleware and is used for distributed tracing
// and logging.
//
// Parameters:
//   - requestID: The request ID to set
//
// Example:
//
//	ctx.SetRequestID("req_123")
func (c *Context) SetRequestID(requestID string) {
	c.RequestID = requestID
	c.Set("request_id", requestID)
}

// GetRequestID returns the request ID from the context.
// This method retrieves the request ID from the context. The request ID is
// typically set by middleware and is used for distributed tracing and logging.
//
// Returns:
//   - The request ID, or an empty string if not set
//
// Example:
//
//	requestID := ctx.GetRequestID()
func (c *Context) GetRequestID() string {
	if c.RequestID != "" {
		return c.RequestID
	}
	if requestID, ok := c.values["request_id"].(string); ok {
		return requestID
	}
	return ""
}

// SetTenantID sets the tenant ID in the context.
// This method sets the tenant ID in the context and stores it in the values map.
// The tenant ID is typically set by authentication middleware or by the application itself.
//
// Parameters:
//   - tenantID: The tenant ID to set
//
// Example:
//
//	ctx.SetTenantID("tenant_123")
func (c *Context) SetTenantID(tenantID string) {
	c.Set("tenant_id", tenantID)
}

// GetTenantID returns the tenant ID from the context.
// This method retrieves the tenant ID from the context. The tenant ID is
// typically set by authentication middleware or by the application itself.
//
// Returns:
//   - The tenant ID, or an empty string if not set
//
// Example:
//
//	tenantID := ctx.GetTenantID()
func (c *Context) GetTenantID() string {
	return c.TenantID()
}

// SetUserID sets the user ID in the context.
// This method sets the user ID in the context and stores it in the values map.
// The user ID is typically set by authentication middleware or by the application itself.
//
// Parameters:
//   - userID: The user ID to set
//
// Example:
//
//	ctx.SetUserID("user_123")
func (c *Context) SetUserID(userID string) {
	c.Set("user_id", userID)
}

// GetUserID returns the user ID from the context.
// This method retrieves the user ID from the context. The user ID is
// typically set by authentication middleware or by the application itself.
//
// Returns:
//   - The user ID, or an empty string if not set
//
// Example:
//
//	userID := ctx.GetUserID()
func (c *Context) GetUserID() string {
	return c.UserID()
}

// HTTP Response convenience methods

// OK sends a 200 OK response with JSON data.
// This method sets the response status code to 200 and serializes the provided
// data to JSON. It also captures the response data if response buffering is enabled.
//
// Parameters:
//   - data: The data to serialize to JSON
//
// Returns:
//   - An error if the JSON serialization fails
//
// Example:
//
//	type Response struct {
//	    Message string `json:"message"`
//	}
//	ctx.OK(Response{Message: "Success"})
func (c *Context) OK(data any) error {
	c.Response.StatusCode = 200
	err := c.Response.JSON(data)
	if err == nil {
		c.captureResponseData()
	}
	return err
}

// Created sends a 201 Created response with JSON data.
// This method sets the response status code to 201 and serializes the provided
// data to JSON. It also captures the response data if response buffering is enabled.
//
// Parameters:
//   - data: The data to serialize to JSON
//
// Returns:
//   - An error if the JSON serialization fails
//
// Example:
//
//	type Response struct {
//	    ID string `json:"id"`
//	}
//	ctx.Created(Response{ID: "resource_123"})
func (c *Context) Created(data any) error {
	c.Response.StatusCode = 201
	err := c.Response.JSON(data)
	if err == nil {
		c.captureResponseData()
	}
	return err
}

// BadRequest sends a 400 Bad Request response.
// Deprecated: Use ValidationError instead.
//
// This method sets the response status code to 400 and sends a JSON response
// with the provided message and error details. It also captures the response
// data if response buffering is enabled.
//
// Parameters:
//   - message: The error message to include in the response
//   - err: The original error (optional)
//
// Returns:
//   - An error if the JSON serialization fails
//
// Example:
//
//	if err := ctx.BadRequest("Invalid request", err); err != nil {
//	    // Handle error
//	}
func (c *Context) BadRequest(message string, err error) error {
	c.Response.StatusCode = 400
	response := map[string]any{
		"error":   "Bad Request",
		"message": message,
	}
	if err != nil {
		response["details"] = err.Error()
	}
	respErr := c.Response.JSON(response)
	if respErr == nil {
		c.captureResponseData()
	}
	// Return the original error to stop processing, not the response write result
	if err != nil {
		return err
	}
	return NewLiftError("BAD_REQUEST", message, 400)
}

// NotFound sends a 404 Not Found response.
// Deprecated: Prefer returning lift.NotFound(...) and let error middleware handle formatting.
//
// This method sets the response status code to 404 and sends a JSON response
// with the provided message and error details. It also captures the response
// data if response buffering is enabled.
//
// Parameters:
//   - message: The error message to include in the response
//   - err: The original error (optional)
//
// Returns:
//   - An error if the JSON serialization fails
//
// Example:
//
//	if err := ctx.NotFound("Resource not found", err); err != nil {
//	    // Handle error
//	}
func (c *Context) NotFound(message string, err error) error {
	c.Response.StatusCode = 404
	response := map[string]any{
		"error":   "Not Found",
		"message": message,
	}
	if err != nil {
		response["details"] = err.Error()
	}
	respErr := c.Response.JSON(response)
	if respErr == nil {
		c.captureResponseData()
	}
	return respErr
}

// Forbidden sends a 403 Forbidden response.
// Deprecated: Use AuthorizationError from the errors package instead.
//
// This method sets the response status code to 403 and sends a JSON response
// with the provided message and error details. It also captures the response
// data if response buffering is enabled.
//
// Parameters:
//   - message: The error message to include in the response
//   - err: The original error (optional)
//
// Returns:
//   - An error if the JSON serialization fails
//
// Example:
//
//	if err := ctx.Forbidden("Access denied", err); err != nil {
//	    // Handle error
//	}
func (c *Context) Forbidden(message string, err error) error {
	c.Response.StatusCode = 403
	response := map[string]any{
		"error":   "Forbidden",
		"message": message,
	}
	if err != nil {
		response["details"] = err.Error()
	}
	respErr := c.Response.JSON(response)
	if respErr == nil {
		c.captureResponseData()
	}
	// Return the original error to stop processing, not the response write result
	if err != nil {
		return err
	}
	return NewLiftError("FORBIDDEN", message, 403)
}

// SystemError sends a 500 Internal Server Error response.
// Deprecated: Prefer returning lift.SystemError(...) and let error middleware handle formatting.
//
// This method sets the response status code to 500 and sends a JSON response
// with the provided message and error details. It also captures the response
// data if response buffering is enabled.
//
// Parameters:
//   - message: The error message to include in the response
//   - err: The original error (optional)
//
// Returns:
//   - An error if the JSON serialization fails
//
// Example:
//
//	if err := ctx.SystemError("Internal server error", err); err != nil {
//	    // Handle error
//	}
func (c *Context) SystemError(message string, err error) error {
	c.Response.StatusCode = 500
	response := map[string]any{
		"error":   "Internal Server Error",
		"message": message,
	}
	if err != nil {
		response["details"] = err.Error()
	}
	respErr := c.Response.JSON(response)
	if respErr == nil {
		c.captureResponseData()
	}
	// Return the original error to stop processing, not the response write result
	if err != nil {
		return err
	}
	return NewLiftError("SYSTEM_ERROR", message, 500)
}

// Unauthorized sends a 401 Unauthorized response.
// Deprecated: Prefer returning lift.Unauthorized(...) and let error middleware handle formatting.
//
// This method sets the response status code to 401 and sends a JSON response
// with the provided message and error details. It also captures the response
// data if response buffering is enabled.
//
// Parameters:
//   - message: The error message to include in the response
//   - err: The original error (optional)
//
// Returns:
//   - An error if the JSON serialization fails
//
// Example:
//
//	if err := ctx.Unauthorized("Authentication required", err); err != nil {
//	    // Handle error
//	}
func (c *Context) Unauthorized(message string, err error) error {
	c.Response.StatusCode = 401
	response := map[string]any{
		"error":   "Unauthorized",
		"message": message,
	}
	if err != nil {
		response["details"] = err.Error()
	}
	respErr := c.Response.JSON(response)
	if respErr == nil {
		c.captureResponseData()
	}
	return respErr
}

// PathParam retrieves a path parameter from the request (alias for Param).
// If the parameter does not exist, an empty string is returned.
//
// Parameters:
//   - key: The name of the path parameter to retrieve
//
// Returns:
//   - The value of the path parameter, or an empty string if not found
//
// Example:
//
//	userID := ctx.PathParam("user_id")
func (c *Context) PathParam(key string) string {
	return c.Param(key)
}

// QueryParam retrieves a query parameter from the request (alias for Query).
// If the parameter does not exist, an empty string is returned.
//
// Parameters:
//   - key: The name of the query parameter to retrieve
//
// Returns:
//   - The value of the query parameter, or an empty string if not found
//
// Example:
//
//	searchTerm := ctx.QueryParam("q")
func (c *Context) QueryParam(key string) string {
	return c.Query(key)
}

// Authentication methods

// SetClaims sets JWT claims in the context and extracts user/tenant information.
// This method sets the JWT claims in the context and extracts the user ID, tenant ID,
// and account ID from the claims. It also sets the isAuthenticated flag to true.
//
// Parameters:
//   - claims: A map of JWT claims
//
// Example:
//
//	c.SetClaims(map[string]any{
//	    "user_id": "user_123",
//	    "tenant_id": "tenant_123",
//	    "account_id": "account_123",
//	})
func (c *Context) SetClaims(claims map[string]any) {
	// Initialize claims map if nil
	if c.claims == nil {
		c.claims = make(map[string]any)
	}

	// Copy claims to avoid external modifications
	for key, value := range claims {
		c.claims[key] = value
	}

	c.isAuthenticated = true

	// Extract user_id from claims (prefer user_id, fallback to sub)
	if userID, ok := claims["user_id"].(string); ok && userID != "" {
		c.SetUserID(userID)
	} else if sub, ok := claims["sub"].(string); ok && sub != "" {
		c.SetUserID(sub)
	}

	// Extract tenant_id from claims
	if tenantID, ok := claims["tenant_id"].(string); ok && tenantID != "" {
		c.SetTenantID(tenantID)
	}

	// Extract account_id from claims if present
	if accountID, ok := claims["account_id"].(string); ok && accountID != "" {
		c.Set("account_id", accountID)
	}
}

// Claims returns the JWT claims from the context.
// This method retrieves the JWT claims from the context. The claims are
// typically set by authentication middleware or by the application itself.
//
// Returns:
//   - A map of JWT claims, or nil if not set
//
// Example:
//
//	claims := ctx.Claims()
func (c *Context) Claims() map[string]any {
	return c.claims
}

// GetClaim retrieves a specific claim from the JWT.
// This method retrieves a specific claim from the JWT claims in the context.
// The claims are typically set by authentication middleware or by the application itself.
//
// Parameters:
//   - key: The key of the claim to retrieve
//
// Returns:
//   - The value of the claim, or nil if not set
//
// Example:
//
//	userID := ctx.GetClaim("user_id").(string)
func (c *Context) GetClaim(key string) any {
	if c.claims == nil {
		return nil
	}
	return c.claims[key]
}

// IsAuthenticated returns whether the context has valid authentication.
// This method returns whether the context has valid authentication. The
// isAuthenticated flag is typically set by authentication middleware or by the
// application itself.
//
// Returns:
//   - true if the context has valid authentication, false otherwise
//
// Example:
//
//	if !ctx.IsAuthenticated() {
//	    return lift.Unauthorized("Authentication required")
//	}
func (c *Context) IsAuthenticated() bool {
	return c.isAuthenticated
}
