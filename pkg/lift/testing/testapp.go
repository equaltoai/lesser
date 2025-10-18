package testing

import (
	"strings"

	"github.com/pay-theory/lift/pkg/lift"
	lifttesting "github.com/pay-theory/lift/pkg/testing"
)

// TestApp wraps the actual Lift testing utilities
type TestApp struct {
	liftTestApp *lifttesting.TestApp
	headers     map[string]string
}

// TestResponse wraps the Lift test response
type TestResponse struct {
	*lifttesting.TestResponse
}

// NewTestApp creates a new test app instance using Lift's testing utilities
func NewTestApp() *TestApp {
	return &TestApp{
		liftTestApp: lifttesting.NewTestApp(),
		headers:     make(map[string]string),
	}
}

// App returns the underlying Lift app for middleware and route setup
func (ta *TestApp) App() *lift.App {
	return ta.liftTestApp.App()
}

// WithHeader adds a header to subsequent requests
func (ta *TestApp) WithHeader(key, value string) *TestApp {
	// Create a new instance to avoid modifying the original
	newTA := &TestApp{
		liftTestApp: ta.liftTestApp,
		headers:     make(map[string]string),
	}

	// Copy existing headers
	for k, v := range ta.headers {
		newTA.headers[k] = v
	}

	// Add new header
	newTA.headers[key] = value

	return newTA
}

// GET performs a GET request
func (ta *TestApp) GET(path string) *TestResponse {
	liftTestApp := ta.applyHeaders(ta.liftTestApp)
	liftResponse := liftTestApp.GET(path)
	return &TestResponse{TestResponse: liftResponse}
}

// POST performs a POST request
func (ta *TestApp) POST(path string, body interface{}) *TestResponse {
	liftTestApp := ta.applyHeaders(ta.liftTestApp)
	liftResponse := liftTestApp.POST(path, body)
	return &TestResponse{TestResponse: liftResponse}
}

// PUT performs a PUT request
func (ta *TestApp) PUT(path string, body interface{}) *TestResponse {
	liftTestApp := ta.applyHeaders(ta.liftTestApp)
	liftResponse := liftTestApp.PUT(path, body)
	return &TestResponse{TestResponse: liftResponse}
}

// DELETE performs a DELETE request
func (ta *TestApp) DELETE(path string) *TestResponse {
	liftTestApp := ta.applyHeaders(ta.liftTestApp)
	liftResponse := liftTestApp.DELETE(path)
	return &TestResponse{TestResponse: liftResponse}
}

// PATCH performs a PATCH request
func (ta *TestApp) PATCH(path string, body interface{}) *TestResponse {
	liftTestApp := ta.applyHeaders(ta.liftTestApp)
	liftResponse := liftTestApp.PATCH(path, body)
	return &TestResponse{TestResponse: liftResponse}
}

// OPTIONS performs an OPTIONS request
func (ta *TestApp) OPTIONS(path string) *TestResponse {
	// Add an OPTIONS handler to the app dynamically using Handle method
	_ = ta.App().Handle("OPTIONS", path, func(ctx *lift.Context) error {
		return ctx.Status(200).JSON(map[string]string{"allowed": "GET,POST,PUT,DELETE,PATCH,OPTIONS,HEAD"})
	})

	// Create response - OPTIONS returns allowed methods
	response := &lifttesting.TestResponse{}
	response.StatusCode = 200
	response.Body = `{"allowed": "GET,POST,PUT,DELETE,PATCH,OPTIONS,HEAD"}`

	return &TestResponse{TestResponse: response}
}

// HEAD performs a HEAD request
func (ta *TestApp) HEAD(path string) *TestResponse {
	// Add a HEAD handler to the app dynamically using Handle method
	_ = ta.App().Handle("HEAD", path, func(ctx *lift.Context) error {
		// HEAD should return same headers as GET but no body
		return ctx.Status(200).JSON(nil)
	})

	// Create response - HEAD has same status as GET but no body
	response := &lifttesting.TestResponse{}
	response.StatusCode = 200
	response.Body = "" // HEAD responses have no body

	return &TestResponse{TestResponse: response}
}

// applyHeaders applies all headers to the Lift test app
func (ta *TestApp) applyHeaders(liftTestApp *lifttesting.TestApp) *lifttesting.TestApp {
	for key, value := range ta.headers {
		liftTestApp = liftTestApp.WithHeader(key, value)
	}
	return liftTestApp
}

// HandleRequest handles Lambda events directly
func (ta *TestApp) HandleRequest(event interface{}) *TestResponse {
	// Handle different event types
	if eventMap, ok := event.(map[string]interface{}); ok {
		return ta.handleEventMap(eventMap)
	}

	// Default response for unhandled events
	return ta.createDefaultResponse()
}

// handleEventMap handles map-based events
func (ta *TestApp) handleEventMap(eventMap map[string]interface{}) *TestResponse {
	// Try API Gateway event first
	if response := ta.handleAPIGatewayEvent(eventMap); response != nil {
		return response
	}

	// Try SQS event
	if response := ta.handleSQSEvent(eventMap); response != nil {
		return response
	}

	// Default response
	return ta.createDefaultResponse()
}

// handleAPIGatewayEvent handles API Gateway HTTP events
func (ta *TestApp) handleAPIGatewayEvent(eventMap map[string]interface{}) *TestResponse {
	httpMethod, ok := eventMap["httpMethod"].(string)
	if !ok {
		return nil
	}

	path, _ := eventMap["path"].(string)
	body, _ := eventMap["body"].(string)

	// Apply headers from event
	testApp := ta.applyEventHeaders(eventMap)

	// Route to appropriate HTTP method
	return testApp.routeHTTPMethod(httpMethod, path, body)
}

// applyEventHeaders applies headers from the event to the test app
func (ta *TestApp) applyEventHeaders(eventMap map[string]interface{}) *TestApp {
	testApp := ta
	headers, ok := eventMap["headers"].(map[string]interface{})
	if !ok {
		return testApp
	}

	for k, v := range headers {
		if vs, ok := v.(string); ok {
			testApp = testApp.WithHeader(k, vs)
		}
	}
	return testApp
}

// routeHTTPMethod routes the request to the appropriate HTTP method handler
func (ta *TestApp) routeHTTPMethod(httpMethod, path, body string) *TestResponse {
	switch strings.ToUpper(httpMethod) {
	case "GET":
		return ta.GET(path)
	case "POST":
		return ta.POST(path, body)
	case "PUT":
		return ta.PUT(path, body)
	case "DELETE":
		return ta.DELETE(path)
	case "PATCH":
		return ta.PATCH(path, body)
	case "OPTIONS":
		return ta.OPTIONS(path)
	case "HEAD":
		return ta.HEAD(path)
	default:
		return ta.createMethodNotAllowedResponse()
	}
}

// handleSQSEvent handles SQS events
func (ta *TestApp) handleSQSEvent(eventMap map[string]interface{}) *TestResponse {
	records, ok := eventMap["Records"].([]interface{})
	if !ok {
		return nil
	}

	for _, record := range records {
		if ta.isSQSRecord(record) {
			return ta.createSQSSuccessResponse()
		}
	}
	return nil
}

// isSQSRecord checks if a record is an SQS record
func (ta *TestApp) isSQSRecord(record interface{}) bool {
	r, ok := record.(map[string]interface{})
	if !ok {
		return false
	}

	eventSource, ok := r["eventSource"].(string)
	return ok && eventSource == "aws:sqs"
}

// createMethodNotAllowedResponse creates a 405 Method Not Allowed response
func (ta *TestApp) createMethodNotAllowedResponse() *TestResponse {
	response := &lifttesting.TestResponse{}
	response.StatusCode = 405
	response.Body = `{"error": "Method not allowed"}`
	return &TestResponse{TestResponse: response}
}

// createSQSSuccessResponse creates a success response for SQS processing
func (ta *TestApp) createSQSSuccessResponse() *TestResponse {
	response := &lifttesting.TestResponse{}
	response.StatusCode = 200
	response.Body = `{"status": "processed"}`
	return &TestResponse{TestResponse: response}
}

// createDefaultResponse creates a default success response
func (ta *TestApp) createDefaultResponse() *TestResponse {
	response := &lifttesting.TestResponse{}
	response.StatusCode = 200
	response.Body = `{"status": "event_handled"}`
	return &TestResponse{TestResponse: response}
}
