package adapters

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// APIGatewayV2Adapter adapts API Gateway V2 (HTTP API) events into the
// normalized Request structure used by Lift.
type APIGatewayV2Adapter struct {
	BaseAdapter
}

// NewAPIGatewayV2Adapter creates a new API Gateway V2 adapter.
func NewAPIGatewayV2Adapter() *APIGatewayV2Adapter {
	return &APIGatewayV2Adapter{
		BaseAdapter: BaseAdapter{triggerType: TriggerAPIGatewayV2},
	}
}

// CanHandle reports whether the adapter recognizes the given raw event as an
// API Gateway V2 request.
func (a *APIGatewayV2Adapter) CanHandle(event any) bool {
	eventMap, ok := event.(map[string]any)
	if !ok {
		return false
	}

	// Check for API Gateway V2 specific fields
	_, hasVersion := eventMap["version"]
	_, hasRouteKey := eventMap["routeKey"]
	_, hasRequestContext := eventMap["requestContext"]

	// API Gateway V2 events have version "2.0" and routeKey
	if hasVersion && hasRouteKey && hasRequestContext {
		if version, ok := eventMap["version"].(string); ok {
			return version == "2.0"
		}
	}

	return false
}

// Validate checks that the raw event has the required API Gateway V2 structure
// before adapting it.
func (a *APIGatewayV2Adapter) Validate(event any) error {
	eventMap, ok := event.(map[string]any)
	if !ok {
		return fmt.Errorf("event must be a map[string]any")
	}

	// Check required fields
	requiredFields := []string{"version", "routeKey", "requestContext"}
	for _, field := range requiredFields {
		if _, exists := eventMap[field]; !exists {
			return fmt.Errorf("missing required field: %s", field)
		}
	}

	// Validate version
	if version := extractStringField(eventMap, "version"); version != "2.0" {
		return fmt.Errorf("unsupported API Gateway version: %s", version)
	}

	return nil
}

// Adapt converts an API Gateway V2 event into a normalized Request.
func (a *APIGatewayV2Adapter) Adapt(rawEvent any) (*Request, error) {
	adapter := newAPIGatewayV2EventAdapter(a, rawEvent)
	return adapter.build()
}

// apiGatewayV2EventAdapter builds requests from API Gateway V2 events
type apiGatewayV2EventAdapter struct {
	adapter        *APIGatewayV2Adapter
	rawEvent       any
	eventMap       map[string]any
	requestContext map[string]any
	httpContext    map[string]any
	request        *Request
}

// newAPIGatewayV2EventAdapter creates a new event adapter
func newAPIGatewayV2EventAdapter(adapter *APIGatewayV2Adapter, rawEvent any) *apiGatewayV2EventAdapter {
	return &apiGatewayV2EventAdapter{
		adapter:  adapter,
		rawEvent: rawEvent,
		request: &Request{
			TriggerType: TriggerAPIGatewayV2,
			RawEvent:    rawEvent,
		},
	}
}

// build constructs the normalized request
func (b *apiGatewayV2EventAdapter) build() (*Request, error) {
	if err := b.validate(); err != nil {
		return nil, err
	}

	b.extractEventMap()
	b.extractContexts()
	b.extractHTTPInfo()
	b.extractHeaders()
	b.extractParameters()

	if err := b.extractBody(); err != nil {
		return nil, err
	}

	b.extractMetadata()

	return b.request, nil
}

// validate validates the raw event
func (b *apiGatewayV2EventAdapter) validate() error {
	if err := b.adapter.Validate(b.rawEvent); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}
	return nil
}

// extractEventMap extracts the event as a map
func (b *apiGatewayV2EventAdapter) extractEventMap() {
	if eventMap, ok := b.rawEvent.(map[string]any); ok {
		b.eventMap = eventMap
	}
}

// extractContexts extracts request and HTTP contexts
func (b *apiGatewayV2EventAdapter) extractContexts() {
	b.requestContext = extractMapField(b.eventMap, "requestContext")
	b.httpContext = extractMapField(b.requestContext, "http")
}

// extractHTTPInfo extracts HTTP method and path
func (b *apiGatewayV2EventAdapter) extractHTTPInfo() {
	b.request.Method = extractStringField(b.httpContext, "method")
	b.request.Path = b.processPath()
}

// processPath handles path extraction and stage prefix removal
func (b *apiGatewayV2EventAdapter) processPath() string {
	path := extractStringField(b.httpContext, "path")
	stage := extractStringField(b.requestContext, "stage")

	return b.stripStagePrefix(path, stage)
}

// stripStagePrefix removes stage prefix from path for custom domains
func (b *apiGatewayV2EventAdapter) stripStagePrefix(path, stage string) string {
	if stage == "" || stage == "$default" {
		return path
	}

	stagePrefix := "/" + stage

	if path == stagePrefix {
		return "/"
	}

	if strings.HasPrefix(path, stagePrefix+"/") {
		return strings.TrimPrefix(path, stagePrefix)
	}

	return path
}

// extractHeaders extracts and normalizes headers
func (b *apiGatewayV2EventAdapter) extractHeaders() {
	headers := make(map[string]string)

	if headersMap := extractMapField(b.eventMap, "headers"); len(headersMap) > 0 {
		for k, v := range headersMap {
			if str, ok := v.(string); ok {
				headers[strings.ToLower(k)] = str
			}
		}
	}

	b.request.Headers = headers
}

// extractParameters extracts query and path parameters
func (b *apiGatewayV2EventAdapter) extractParameters() {
	b.request.QueryParams = b.extractQueryParams()
	b.request.PathParams = b.extractPathParams()
}

// extractQueryParams extracts query string parameters
func (b *apiGatewayV2EventAdapter) extractQueryParams() map[string]string {
	queryParams := extractStringMapField(b.eventMap, "queryStringParameters")
	if queryParams == nil {
		return make(map[string]string)
	}
	return queryParams
}

// extractPathParams extracts path parameters
func (b *apiGatewayV2EventAdapter) extractPathParams() map[string]string {
	pathParams := extractStringMapField(b.eventMap, "pathParameters")
	if pathParams == nil {
		return make(map[string]string)
	}
	return pathParams
}

// extractBody extracts and decodes the request body
func (b *apiGatewayV2EventAdapter) extractBody() error {
	bodyStr := extractStringField(b.eventMap, "body")
	if bodyStr == "" {
		return nil
	}

	if b.isBase64Encoded() {
		return b.decodeBase64Body(bodyStr)
	}

	b.request.Body = []byte(bodyStr)
	return nil
}

// isBase64Encoded checks if the body is base64 encoded
func (b *apiGatewayV2EventAdapter) isBase64Encoded() bool {
	if encoded, ok := b.eventMap["isBase64Encoded"].(bool); ok {
		return encoded
	}
	return false
}

// decodeBase64Body decodes a base64 encoded body
func (b *apiGatewayV2EventAdapter) decodeBase64Body(bodyStr string) error {
	decoded, err := base64.StdEncoding.DecodeString(bodyStr)
	if err != nil {
		return fmt.Errorf("failed to decode base64 body: %w", err)
	}
	b.request.Body = decoded
	return nil
}

// extractMetadata extracts event metadata
func (b *apiGatewayV2EventAdapter) extractMetadata() {
	b.request.EventID = extractStringField(b.requestContext, "requestId")
	b.request.Timestamp = extractStringField(b.requestContext, "timeEpoch")
}
