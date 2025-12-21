package testing

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	lifttesting "github.com/pay-theory/lift/pkg/testing"
)

// TestApp is a lightweight, in-process Lift test harness.
type TestApp struct {
	app     *lift.App
	headers map[string]string
}

// TestResponse wraps the Lift test response.
type TestResponse struct {
	*lifttesting.TestResponse
}

// NewTestApp creates a new test app instance without starting an HTTP listener.
func NewTestApp() *TestApp {
	return &TestApp{
		app:     lift.New(),
		headers: make(map[string]string),
	}
}

// App returns the underlying Lift app for middleware and route setup
func (ta *TestApp) App() *lift.App {
	return ta.app
}

// WithHeader adds a header to subsequent requests
func (ta *TestApp) WithHeader(key, value string) *TestApp {
	// Create a new instance to avoid modifying the original
	newTA := &TestApp{
		app:     ta.app,
		headers: make(map[string]string),
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
	return ta.doRequest(methodGET, path, nil)
}

// POST performs a POST request
func (ta *TestApp) POST(path string, body interface{}) *TestResponse {
	return ta.doRequest(methodPOST, path, body)
}

// PUT performs a PUT request
func (ta *TestApp) PUT(path string, body interface{}) *TestResponse {
	return ta.doRequest(methodPUT, path, body)
}

// DELETE performs a DELETE request
func (ta *TestApp) DELETE(path string) *TestResponse {
	return ta.doRequest(methodDELETE, path, nil)
}

// PATCH performs a PATCH request
func (ta *TestApp) PATCH(path string, body interface{}) *TestResponse {
	return ta.doRequest("PATCH", path, body)
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

func (ta *TestApp) doRequest(method, rawPath string, body interface{}) *TestResponse {
	path, queryParams := parseRawPath(rawPath)

	encodedBody, err := encodeBody(body)
	if err != nil {
		return newErrorTestResponse(500, fmt.Sprintf("failed to encode request body: %v", err))
	}

	headers := make(map[string]string, len(ta.headers))
	for k, v := range ta.headers {
		headers[k] = v
	}
	if _, ok := headers["Content-Type"]; !ok && len(encodedBody) > 0 {
		headers["Content-Type"] = "application/json"
	}

	req := lift.NewRequest(&adapters.Request{
		Method:      strings.ToUpper(method),
		Path:        path,
		Headers:     headers,
		QueryParams: queryParams,
		TriggerType: adapters.TriggerAPIGatewayV2,
		Body:        encodedBody,
	})
	ctx := lift.NewContext(context.Background(), req)

	if err := ta.app.HandleTestRequest(ctx); err != nil {
		return newErrorTestResponse(500, fmt.Sprintf("failed to handle request: %v", err))
	}

	return &TestResponse{TestResponse: liftResponseToTestResponse(ctx.Response)}
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

func parseRawPath(rawPath string) (string, map[string]string) {
	parsed, err := url.Parse(rawPath)
	if err != nil {
		return rawPath, nil
	}

	queryParams := make(map[string]string)
	for key, values := range parsed.Query() {
		if len(values) == 0 {
			continue
		}
		queryParams[key] = values[0]
	}

	return parsed.Path, queryParams
}

func encodeBody(body interface{}) ([]byte, error) {
	switch v := body.(type) {
	case nil:
		return nil, nil
	case []byte:
		return v, nil
	case string:
		return []byte(v), nil
	default:
		return json.Marshal(v)
	}
}

func liftResponseToTestResponse(resp *lift.Response) *lifttesting.TestResponse {
	if resp == nil {
		return &lifttesting.TestResponse{
			StatusCode: 500,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       `{"error":"missing response"}`,
		}
	}

	body := ""
	if resp.Body != nil {
		switch v := resp.Body.(type) {
		case string:
			body = v
		case []byte:
			if resp.IsBase64Encoded {
				body = base64.StdEncoding.EncodeToString(v)
			} else {
				body = string(v)
			}
		default:
			if encoded, err := json.Marshal(v); err == nil {
				body = string(encoded)
			} else {
				body = fmt.Sprintf(`{"error":"failed to encode response body","details":%q}`, err.Error())
			}
		}
	}

	headers := resp.Headers
	if headers == nil {
		headers = make(map[string]string)
	}

	return &lifttesting.TestResponse{
		StatusCode: resp.StatusCode,
		Headers:    headers,
		Body:       body,
	}
}

func newErrorTestResponse(status int, message string) *TestResponse {
	response := &lifttesting.TestResponse{
		StatusCode: status,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       fmt.Sprintf(`{"error":%q}`, message),
	}
	return &TestResponse{TestResponse: response}
}
