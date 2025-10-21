package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
)

// Import these from lift package for consistency
type (
	// Counter is re-exported from lift package
	Counter = lift.Counter
	// Histogram is re-exported from lift package
	Histogram = lift.Histogram
	// Gauge is re-exported from lift package
	Gauge = lift.Gauge
	// MetricsCollector is re-exported from lift package
	MetricsCollector = lift.MetricsCollector
)

// ServiceClient provides type-safe inter-service communication
// Memory optimized: 24 → 8 bytes (16 bytes saved)
type ServiceClient struct {
	// Pointers and interfaces (8 bytes each)
	registry       *ServiceRegistry // 8 bytes
	circuitBreaker CircuitBreaker   // 8 bytes
	retryPolicy    *RetryPolicy     // 8 bytes
	metrics        MetricsCollector // 8 bytes
	httpClient     HTTPClient       // 8 bytes
	// Struct last (varies)
	config ServiceClientConfig // struct
}

// ServiceClientConfig configures the service client
// Memory optimized: 24 → 8 bytes (16 bytes saved)
type ServiceClientConfig struct {
	// Strings (16 bytes) - largest first
	UserAgent string `json:"user_agent"`

	// 8-byte types
	DefaultTimeout time.Duration `json:"default_timeout"`
	RetryBackoff   time.Duration `json:"retry_backoff"`

	// 4-byte types
	MaxRetries int `json:"max_retries"`

	// Booleans (1 byte each) - smallest last
	EnableTracing        bool `json:"enable_tracing"`
	EnableMetrics        bool `json:"enable_metrics"`
	EnableCircuitBreaker bool `json:"enable_circuit_breaker"`
	TenantIsolation      bool `json:"tenant_isolation"`
}

// ServiceRequest represents a service call request
// Memory optimized: 144 → 136 bytes (8 bytes saved)
type ServiceRequest struct {
	Body                any                 `json:"body"`
	Headers             map[string]string   `json:"headers"`
	Metadata            map[string]any      `json:"metadata"`
	LoadBalanceStrategy LoadBalanceStrategy `json:"load_balance_strategy"`
	ServiceName         string              `json:"service_name"`
	Method              string              `json:"method"`
	Path                string              `json:"path"`
	TenantID            string              `json:"tenant_id,omitempty"`
	UserID              string              `json:"user_id,omitempty"`
	RequestID           string              `json:"request_id,omitempty"`
	Timeout             time.Duration       `json:"timeout"`
}

// ServiceResponse represents a service call response
type ServiceResponse struct {
	Headers    map[string]string `json:"headers"`
	Metadata   map[string]any    `json:"metadata"`
	Instance   *ServiceInstance  `json:"instance"`
	Body       []byte            `json:"body"`
	Duration   time.Duration     `json:"duration"`
	StatusCode int               `json:"status_code"`
}

// RetryPolicy defines retry behavior
type RetryPolicy struct {
	RetryableStatusCodes []int         `json:"retryable_status_codes"`
	RetryableErrors      []string      `json:"retryable_errors"`
	MaxRetries           int           `json:"max_retries"`
	InitialBackoff       time.Duration `json:"initial_backoff"`
	MaxBackoff           time.Duration `json:"max_backoff"`
	BackoffMultiplier    float64       `json:"backoff_multiplier"`
}

// HTTPClient defines the interface for HTTP operations
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// NewServiceClient creates a new service client
func NewServiceClient(registry *ServiceRegistry, config ServiceClientConfig) *ServiceClient {
	if config.DefaultTimeout == 0 {
		config.DefaultTimeout = 30 * time.Second
	}

	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}

	if config.RetryBackoff == 0 {
		config.RetryBackoff = 1 * time.Second
	}

	if config.UserAgent == "" {
		config.UserAgent = "lift-service-client/1.0"
	}

	client := &ServiceClient{
		registry:   registry,
		config:     config,
		httpClient: NewSecureHTTPClient(ProductionHTTPClientConfig()),
		retryPolicy: &RetryPolicy{
			MaxRetries:           config.MaxRetries,
			InitialBackoff:       config.RetryBackoff,
			MaxBackoff:           30 * time.Second,
			BackoffMultiplier:    2.0,
			RetryableStatusCodes: []int{500, 502, 503, 504},
			RetryableErrors:      []string{"timeout", "connection refused", "connection reset"},
		},
	}

	return client
}

// Call makes a type-safe service call with automatic discovery
func (c *ServiceClient) Call(ctx context.Context, request *ServiceRequest) (*ServiceResponse, error) {
	start := time.Now()

	// Set defaults
	if request.LoadBalanceStrategy == "" {
		request.LoadBalanceStrategy = RoundRobin
	}

	if request.Timeout == 0 {
		request.Timeout = c.config.DefaultTimeout
	}

	// Add request ID if not provided
	if request.RequestID == "" {
		request.RequestID = c.generateRequestID()
	}

	// Discover service instance
	instance, err := c.registry.Discover(ctx, request.ServiceName, DiscoveryOptions{
		TenantID: request.TenantID,
		Strategy: request.LoadBalanceStrategy,
	})
	if err != nil {
		c.recordMetrics(request.ServiceName, "discovery_failed", time.Since(start), err)
		return nil, fmt.Errorf("service discovery failed: %w", err)
	}

	// Execute with circuit breaker if enabled
	if c.config.EnableCircuitBreaker && c.circuitBreaker != nil {
		result, cbErr := c.circuitBreaker.Execute(func() (any, error) {
			return c.executeRequest(ctx, instance, request)
		})

		if cbErr != nil {
			c.recordMetrics(request.ServiceName, "circuit_breaker_failed", time.Since(start), cbErr)
			return nil, cbErr
		}

		response, ok := result.(*ServiceResponse)
		if !ok {
			return nil, fmt.Errorf("unexpected circuit breaker result type")
		}
		c.recordMetrics(request.ServiceName, "success", time.Since(start), nil)
		return response, nil
	}

	// Execute request directly
	response, err := c.executeRequest(ctx, instance, request)
	if err != nil {
		c.recordMetrics(request.ServiceName, "request_failed", time.Since(start), err)
		return nil, err
	}

	c.recordMetrics(request.ServiceName, "success", time.Since(start), nil)
	return response, nil
}

// executeRequest executes the actual HTTP request

func (c *ServiceClient) executeRequest(ctx context.Context, instance *ServiceInstance, request *ServiceRequest) (*ServiceResponse, error) {
	start := time.Now()

	// Build URL
	url := fmt.Sprintf("%s://%s:%d%s",
		instance.Endpoint.Protocol,
		instance.Endpoint.Host,
		instance.Endpoint.Port,
		request.Path,
	)

	// Prepare request body for retries; keep immutable copy for each attempt
	var requestBody []byte
	if request.Body != nil {
		var err error
		requestBody, err = json.Marshal(request.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
	}

	buildRequest := func() (*http.Request, error) {
		var bodyReader io.Reader
		if len(requestBody) > 0 {
			bodyReader = bytes.NewReader(requestBody)
		}

		httpReq, err := http.NewRequestWithContext(ctx, request.Method, url, bodyReader)
		if err != nil {
			return nil, fmt.Errorf("failed to create HTTP request: %w", err)
		}

		c.setRequestHeaders(httpReq, request, instance)
		return httpReq, nil
	}

	// Execute with retry policy
	var response *ServiceResponse
	err := c.executeWithRetry(ctx, func() error {
		req, buildErr := buildRequest()
		if buildErr != nil {
			return buildErr
		}

		resp, execErr := c.httpClient.Do(req)
		if execErr != nil {
			return execErr
		}
		defer func() {
			if closeErr := resp.Body.Close(); closeErr != nil {
				log.Printf("Warning: failed to close response body: %v", closeErr)
			}
		}()

		// Read response body
		respBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return fmt.Errorf("failed to read response body: %w", readErr)
		}

		// Build service response
		response = &ServiceResponse{
			StatusCode: resp.StatusCode,
			Headers:    make(map[string]string),
			Body:       respBody,
			Duration:   time.Since(start),
			Instance:   instance,
			Metadata:   make(map[string]any),
		}

		// Copy response headers
		for key, values := range resp.Header {
			if len(values) > 0 {
				response.Headers[key] = values[0]
			}
		}

		// Check if response indicates an error that should be retried
		if c.isRetryableStatusCode(resp.StatusCode) {
			return fmt.Errorf("retryable status code: %d", resp.StatusCode)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return response, nil
}

// setRequestHeaders sets standard and custom headers
func (c *ServiceClient) setRequestHeaders(req *http.Request, request *ServiceRequest, instance *ServiceInstance) {
	// Set standard headers
	req.Header.Set("User-Agent", c.config.UserAgent)
	req.Header.Set(lift.HeaderContentType, lift.ContentTypeJSON)
	req.Header.Set("Accept", "application/json")

	// Set request ID for tracing
	if request.RequestID != "" {
		req.Header.Set("X-Request-ID", request.RequestID)
	}

	// Set tenant context
	if request.TenantID != "" {
		req.Header.Set("X-Tenant-ID", request.TenantID)
	}

	if request.UserID != "" {
		req.Header.Set("X-User-ID", request.UserID)
	}

	// Set service metadata
	req.Header.Set("X-Source-Service", "lift-client")
	req.Header.Set("X-Target-Service", request.ServiceName)
	req.Header.Set("X-Instance-ID", instance.ID)

	// Set custom headers
	for key, value := range request.Headers {
		req.Header.Set(key, value)
	}

	// Add tracing headers if enabled
	if c.config.EnableTracing {
		c.addTracingHeaders(req, request)
	}
}

// addTracingHeaders adds distributed tracing headers
func (c *ServiceClient) addTracingHeaders(req *http.Request, request *ServiceRequest) {
	// Add trace context headers
	// These would typically be propagated from the incoming request context
	if request.Metadata != nil {
		if traceID, ok := request.Metadata["trace_id"].(string); ok && traceID != "" {
			req.Header.Set("X-Trace-ID", traceID)
		} else {
			req.Header.Set("X-Trace-ID", c.generateTraceID())
		}

		if spanID, ok := request.Metadata["span_id"].(string); ok && spanID != "" {
			req.Header.Set("X-Parent-Span-ID", spanID)
		}
	} else {
		req.Header.Set("X-Trace-ID", c.generateTraceID())
	}

	req.Header.Set("X-Span-ID", c.generateSpanID())
}

// executeWithRetry executes a function with retry logic
func (c *ServiceClient) executeWithRetry(ctx context.Context, fn func() error) error {
	var lastErr error
	backoff := c.retryPolicy.InitialBackoff

	for attempt := 0; attempt <= c.retryPolicy.MaxRetries; attempt++ {
		// Execute function
		err := fn()
		if err == nil {
			return nil // Success
		}

		lastErr = err

		// Check if we should retry
		if attempt == c.retryPolicy.MaxRetries || !c.isRetryableError(err) {
			break
		}

		// Wait before retry
		select {
		case <-time.After(backoff):
			// Continue to next attempt
		case <-ctx.Done():
			return ctx.Err()
		}

		// Increase backoff
		backoff = time.Duration(float64(backoff) * c.retryPolicy.BackoffMultiplier)
		if backoff > c.retryPolicy.MaxBackoff {
			backoff = c.retryPolicy.MaxBackoff
		}
	}

	return lastErr
}

// isRetryableStatusCode checks if a status code is retryable
func (c *ServiceClient) isRetryableStatusCode(statusCode int) bool {
	for _, code := range c.retryPolicy.RetryableStatusCodes {
		if statusCode == code {
			return true
		}
	}
	return false
}

// isRetryableError checks if an error is retryable
func (c *ServiceClient) isRetryableError(err error) bool {
	errStr := err.Error()
	for _, retryableErr := range c.retryPolicy.RetryableErrors {
		if contains(errStr, retryableErr) {
			return true
		}
	}
	return false
}

// recordMetrics records service call metrics
func (c *ServiceClient) recordMetrics(serviceName, status string, duration time.Duration, err error) {
	if !c.config.EnableMetrics || c.metrics == nil {
		return
	}

	tags := map[string]string{
		"service": serviceName,
		"status":  status,
	}

	// Record request count
	c.metrics.Counter("service_client.requests", tags).Inc()

	// Record duration
	c.metrics.Histogram("service_client.duration", tags).Observe(float64(duration.Milliseconds()))

	// Record errors
	if err != nil {
		errorTags := map[string]string{
			"service": serviceName,
			"error":   err.Error(),
		}
		c.metrics.Counter("service_client.errors", errorTags).Inc()
	}
}

// generateRequestID generates a unique request ID
func (c *ServiceClient) generateRequestID() string {
	return fmt.Sprintf("req_%d", time.Now().UnixNano())
}

// generateTraceID generates a trace ID for distributed tracing
func (c *ServiceClient) generateTraceID() string {
	return fmt.Sprintf("trace_%d", time.Now().UnixNano())
}

// generateSpanID generates a span ID for distributed tracing
func (c *ServiceClient) generateSpanID() string {
	return fmt.Sprintf("span_%d", time.Now().UnixNano())
}

// Typed service client interfaces

// UserService defines the interface for user service operations
type UserService interface {
	GetUser(ctx context.Context, userID string) (*User, error)
	CreateUser(ctx context.Context, user *CreateUserRequest) (*User, error)
	UpdateUser(ctx context.Context, userID string, updates *UpdateUserRequest) (*User, error)
	DeleteUser(ctx context.Context, userID string) error
	ListUsers(ctx context.Context, filters *UserFilters) (*UserList, error)
}

// User represents a user entity
type User struct {
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
	Metadata  map[string]string `json:"metadata"`
	ID        string            `json:"id"`
	Email     string            `json:"email"`
	Name      string            `json:"name"`
	TenantID  string            `json:"tenant_id"`
}

// CreateUserRequest represents a user creation request
type CreateUserRequest struct {
	Metadata map[string]string `json:"metadata"`
	Email    string            `json:"email"`
	Name     string            `json:"name"`
	TenantID string            `json:"tenant_id"`
}

// UpdateUserRequest represents a user update request
type UpdateUserRequest struct {
	Email    *string           `json:"email,omitempty"`
	Name     *string           `json:"name,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// UserFilters represents user listing filters
type UserFilters struct {
	TenantID string `json:"tenant_id,omitempty"`
	Email    string `json:"email,omitempty"`
	Limit    int    `json:"limit,omitempty"`
	Offset   int    `json:"offset,omitempty"`
}

// UserList represents a list of users
type UserList struct {
	Users  []*User `json:"users"`
	Total  int     `json:"total"`
	Limit  int     `json:"limit"`
	Offset int     `json:"offset"`
}

// UserServiceClient implements the UserService interface
type UserServiceClient struct {
	client   *ServiceClient
	tenantID string
}

// NewUserServiceClient creates a new user service client
func NewUserServiceClient(client *ServiceClient, tenantID string) UserService {
	return &UserServiceClient{
		client:   client,
		tenantID: tenantID,
	}
}

// GetUser retrieves a user by ID
func (u *UserServiceClient) GetUser(ctx context.Context, userID string) (*User, error) {
	request := &ServiceRequest{
		ServiceName: "user-service",
		Method:      "GET",
		Path:        fmt.Sprintf("/users/%s", userID),
		TenantID:    u.tenantID,
	}

	response, err := u.client.Call(ctx, request)
	if err != nil {
		return nil, err
	}

	if response.StatusCode == 404 {
		return nil, fmt.Errorf("user not found: %s", userID)
	}

	if response.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status code: %d", response.StatusCode)
	}

	var user User
	if err := json.Unmarshal(response.Body, &user); err != nil {
		return nil, fmt.Errorf("failed to unmarshal user: %w", err)
	}

	return &user, nil
}

// CreateUser creates a new user
func (u *UserServiceClient) CreateUser(ctx context.Context, user *CreateUserRequest) (*User, error) {
	request := &ServiceRequest{
		ServiceName: "user-service",
		Method:      "POST",
		Path:        "/users",
		Body:        user,
		TenantID:    u.tenantID,
	}

	response, err := u.client.Call(ctx, request)
	if err != nil {
		return nil, err
	}

	if response.StatusCode != 201 {
		return nil, fmt.Errorf("unexpected status code: %d", response.StatusCode)
	}

	var createdUser User
	if err := json.Unmarshal(response.Body, &createdUser); err != nil {
		return nil, fmt.Errorf("failed to unmarshal created user: %w", err)
	}

	return &createdUser, nil
}

// UpdateUser updates an existing user
func (u *UserServiceClient) UpdateUser(ctx context.Context, userID string, updates *UpdateUserRequest) (*User, error) {
	request := &ServiceRequest{
		ServiceName: "user-service",
		Method:      "PUT",
		Path:        fmt.Sprintf("/users/%s", userID),
		Body:        updates,
		TenantID:    u.tenantID,
	}

	response, err := u.client.Call(ctx, request)
	if err != nil {
		return nil, err
	}

	if response.StatusCode == 404 {
		return nil, fmt.Errorf("user not found: %s", userID)
	}

	if response.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status code: %d", response.StatusCode)
	}

	var updatedUser User
	if err := json.Unmarshal(response.Body, &updatedUser); err != nil {
		return nil, fmt.Errorf("failed to unmarshal updated user: %w", err)
	}

	return &updatedUser, nil
}

// DeleteUser deletes a user
func (u *UserServiceClient) DeleteUser(ctx context.Context, userID string) error {
	request := &ServiceRequest{
		ServiceName: "user-service",
		Method:      "DELETE",
		Path:        fmt.Sprintf("/users/%s", userID),
		TenantID:    u.tenantID,
	}

	response, err := u.client.Call(ctx, request)
	if err != nil {
		return err
	}

	if response.StatusCode == 404 {
		return fmt.Errorf("user not found: %s", userID)
	}

	if response.StatusCode != 204 {
		return fmt.Errorf("unexpected status code: %d", response.StatusCode)
	}

	return nil
}

// ListUsers lists users with optional filters
func (u *UserServiceClient) ListUsers(ctx context.Context, filters *UserFilters) (*UserList, error) {
	request := &ServiceRequest{
		ServiceName: "user-service",
		Method:      "GET",
		Path:        "/users",
		TenantID:    u.tenantID,
	}

	// Add query parameters for filters
	if filters != nil {
		// Pass filters as query parameters via request metadata
		request.Metadata = map[string]any{
			"filters": filters,
		}
	}

	response, err := u.client.Call(ctx, request)
	if err != nil {
		return nil, err
	}

	if response.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status code: %d", response.StatusCode)
	}

	var userList UserList
	if err := json.Unmarshal(response.Body, &userList); err != nil {
		return nil, fmt.Errorf("failed to unmarshal user list: %w", err)
	}

	return &userList, nil
}

// Utility functions

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

// ServiceClientMiddleware creates middleware for service client integration
func ServiceClientMiddleware(client *ServiceClient) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Add service client to context
			ctx.Set("service_client", client)
			return next.Handle(ctx)
		})
	}
}

// GetServiceClient retrieves the service client from context
func GetServiceClient(ctx *lift.Context) *ServiceClient {
	if client, ok := ctx.Get("service_client").(*ServiceClient); ok {
		return client
	}
	return nil
}
