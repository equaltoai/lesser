package adapters

import (
	"encoding/base64"
	"fmt"
	"strings"
)

const (
	apiGatewayV2 = "2.0"
	defaultRoute = "$default"
)

// APIGatewayAdapter adapts API Gateway V1 (REST API) events into the
// normalized Request structure used by Lift.
type APIGatewayAdapter struct {
	BaseAdapter
}

// NewAPIGatewayAdapter creates a new API Gateway V1 adapter.
func NewAPIGatewayAdapter() *APIGatewayAdapter {
	return &APIGatewayAdapter{
		BaseAdapter: BaseAdapter{triggerType: TriggerAPIGateway},
	}
}

// CanHandle reports whether the adapter recognizes the given raw event as an
// API Gateway V1 request.
func (a *APIGatewayAdapter) CanHandle(event any) bool {
	eventMap, ok := event.(map[string]any)
	if !ok {
		return false
	}

	// Check for API Gateway V1 specific fields
	_, hasResource := eventMap["resource"]
	_, hasHttpMethod := eventMap["httpMethod"]
	_, hasRequestContext := eventMap["requestContext"]

	// API Gateway V1 events have resource, httpMethod, and requestContext
	// but no version field (or version "1.0")
	if hasResource && hasHttpMethod && hasRequestContext {
		// If version exists, it should be "1.0" or not apiGatewayV2
		if version, exists := eventMap["version"]; exists {
			if versionStr, ok := version.(string); ok {
				return versionStr == "1.0" || versionStr != apiGatewayV2
			}
		}
		return true
	}

	return false
}

// Validate checks that the raw event has the required API Gateway V1 fields
// before adapting it.
func (a *APIGatewayAdapter) Validate(event any) error {
	eventMap, ok := event.(map[string]any)
	if !ok {
		return fmt.Errorf("event must be a map[string]any")
	}

	// Check required fields
	requiredFields := []string{"resource", "httpMethod", "requestContext"}
	for _, field := range requiredFields {
		if _, exists := eventMap[field]; !exists {
			return fmt.Errorf("missing required field: %s", field)
		}
	}

	return nil
}

// Adapt converts an API Gateway V1 event into a normalized Request.
func (a *APIGatewayAdapter) Adapt(rawEvent any) (*Request, error) {
	if err := a.Validate(rawEvent); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	eventMap, ok := rawEvent.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("event must be a map[string]any, got %T", rawEvent)
	}

	extractor := newAPIGatewayEventExtractor(eventMap)
	return extractor.extract()
}

// apiGatewayEventExtractor extracts data from API Gateway events
type apiGatewayEventExtractor struct {
	eventMap       map[string]any
	requestContext map[string]any
}

// newAPIGatewayEventExtractor creates a new event extractor
func newAPIGatewayEventExtractor(eventMap map[string]any) *apiGatewayEventExtractor {
	return &apiGatewayEventExtractor{
		eventMap:       eventMap,
		requestContext: extractMapField(eventMap, "requestContext"),
	}
}

// extract converts the event to a Request
func (e *apiGatewayEventExtractor) extract() (*Request, error) {
	// Extract basic HTTP information
	method := extractStringField(e.eventMap, "httpMethod")
	path, err := e.extractPath()
	if err != nil {
		return nil, err
	}

	// Extract all request data
	headers := e.extractHeaders()
	queryParams := e.extractQueryParams()
	pathParams := e.extractPathParams()
	body, err := e.extractBody()
	if err != nil {
		return nil, err
	}

	// Extract event metadata
	eventID := extractStringField(e.requestContext, "requestId")
	timestamp := extractStringField(e.requestContext, "requestTimeEpoch")

	return &Request{
		TriggerType: TriggerAPIGateway,
		RawEvent:    e.eventMap,
		EventID:     eventID,
		Timestamp:   timestamp,
		Method:      method,
		Path:        path,
		Headers:     headers,
		QueryParams: queryParams,
		PathParams:  pathParams,
		Body:        body,
	}, nil
}

// extractPath extracts and normalizes the request path
func (e *apiGatewayEventExtractor) extractPath() (string, error) {
	path := extractStringField(e.eventMap, "path")
	if path == "" {
		// API Gateway v1 should always have a path field when properly configured
		// The resource field contains the route template (e.g., /users/{id}) not the actual path
		return "", fmt.Errorf("API Gateway v1 event missing 'path' field - check your API Gateway integration configuration")
	}

	// Handle stage prefix in path
	return e.normalizePathWithStage(path), nil
}

// normalizePathWithStage removes stage prefix from path if present
func (e *apiGatewayEventExtractor) normalizePathWithStage(path string) string {
	stage := extractStringField(e.requestContext, "stage")
	if stage == "" || stage == defaultRoute {
		return path
	}

	stagePrefix := "/" + stage
	if path == stagePrefix {
		// Path is exactly the stage, return root
		return "/"
	} else if strings.HasPrefix(path, stagePrefix+"/") {
		// Strip stage prefix from path only if followed by "/"
		return strings.TrimPrefix(path, stagePrefix)
	}

	return path
}

// extractHeaders extracts and normalizes headers
func (e *apiGatewayEventExtractor) extractHeaders() map[string]string {
	headers := make(map[string]string)

	// Extract single-value headers
	e.extractSingleValueHeaders(headers)

	// Extract multi-value headers
	e.extractMultiValueHeaders(headers)

	return headers
}

// extractSingleValueHeaders extracts headers from the headers field
func (e *apiGatewayEventExtractor) extractSingleValueHeaders(headers map[string]string) {
	headersMap := extractMapField(e.eventMap, "headers")
	for k, v := range headersMap {
		if str, ok := v.(string); ok {
			headers[strings.ToLower(k)] = str
		}
	}
}

// extractMultiValueHeaders extracts headers from multiValueHeaders field
func (e *apiGatewayEventExtractor) extractMultiValueHeaders(headers map[string]string) {
	multiHeaders := extractMapField(e.eventMap, "multiValueHeaders")
	for k, v := range multiHeaders {
		if slice, ok := v.([]any); ok && len(slice) > 0 {
			// Take the first value for simplicity
			if str, ok := slice[0].(string); ok {
				headers[strings.ToLower(k)] = str
			}
		}
	}
}

// extractQueryParams extracts query parameters
func (e *apiGatewayEventExtractor) extractQueryParams() map[string]string {
	// Start with single-value parameters
	queryParams := extractStringMapField(e.eventMap, "queryStringParameters")
	if queryParams == nil {
		queryParams = make(map[string]string)
	}

	// Merge multi-value parameters
	e.mergeMultiValueQueryParams(queryParams)

	return queryParams
}

// mergeMultiValueQueryParams merges multi-value query parameters
func (e *apiGatewayEventExtractor) mergeMultiValueQueryParams(queryParams map[string]string) {
	multiQuery := extractMapField(e.eventMap, "multiValueQueryStringParameters")
	for k, v := range multiQuery {
		if slice, ok := v.([]any); ok && len(slice) > 0 {
			// Take the first value for simplicity
			if str, ok := slice[0].(string); ok {
				queryParams[k] = str
			}
		}
	}
}

// extractPathParams extracts path parameters
func (e *apiGatewayEventExtractor) extractPathParams() map[string]string {
	pathParams := extractStringMapField(e.eventMap, "pathParameters")
	if pathParams == nil {
		pathParams = make(map[string]string)
	}
	return pathParams
}

// extractBody extracts and decodes the request body
func (e *apiGatewayEventExtractor) extractBody() ([]byte, error) {
	bodyStr := extractStringField(e.eventMap, "body")
	if bodyStr == "" {
		return nil, nil
	}

	// Check if body is base64 encoded
	if isBase64Encoded, ok := e.eventMap["isBase64Encoded"].(bool); ok && isBase64Encoded {
		decoded, err := base64.StdEncoding.DecodeString(bodyStr)
		if err != nil {
			return nil, fmt.Errorf("failed to decode base64 body: %w", err)
		}
		return decoded, nil
	}

	return []byte(bodyStr), nil
}
