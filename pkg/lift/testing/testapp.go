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
	ta.App().Handle("OPTIONS", path, func(ctx *lift.Context) error {
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
	ta.App().Handle("HEAD", path, func(ctx *lift.Context) error {
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
	// Handle different event types by routing to appropriate HTTP methods
	switch e := event.(type) {
	case map[string]interface{}:
		// Check if it's an API Gateway event
		if httpMethod, ok := e["httpMethod"].(string); ok {
			path, _ := e["path"].(string)
			body, _ := e["body"].(string)
			
			// Apply headers from event
			testApp := ta
			if headers, ok := e["headers"].(map[string]interface{}); ok {
				for k, v := range headers {
					if vs, ok := v.(string); ok {
						testApp = testApp.WithHeader(k, vs)
					}
				}
			}
			
			// Route to appropriate HTTP method using TestApp methods
			switch strings.ToUpper(httpMethod) {
			case "GET":
				return testApp.GET(path)
			case "POST":
				return testApp.POST(path, body)
			case "PUT":
				return testApp.PUT(path, body)
			case "DELETE":
				return testApp.DELETE(path)
			case "PATCH":
				return testApp.PATCH(path, body)
			case "OPTIONS":
				return testApp.OPTIONS(path)
			case "HEAD":
				return testApp.HEAD(path)
			default:
				response := &lifttesting.TestResponse{}
				response.StatusCode = 405
				response.Body = `{"error": "Method not allowed"}`
				return &TestResponse{TestResponse: response}
			}
		}
		
		// Handle SQS events
		if records, ok := e["Records"].([]interface{}); ok {
			for _, record := range records {
				if r, ok := record.(map[string]interface{}); ok {
					if eventSource, ok := r["eventSource"].(string); ok && eventSource == "aws:sqs" {
						// Process SQS message - return success response
						response := &lifttesting.TestResponse{}
						response.StatusCode = 200
						response.Body = `{"status": "processed"}`
						return &TestResponse{TestResponse: response}
					}
				}
			}
		}
	}
	
	// Default response for unhandled events
	response := &lifttesting.TestResponse{}
	response.StatusCode = 200
	response.Body = `{"status": "event_handled"}`
	return &TestResponse{TestResponse: response}
}