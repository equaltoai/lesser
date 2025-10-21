package adapters

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// WebSocketAdapter adapts API Gateway WebSocket events into the normalized
// Request structure used by Lift.
type WebSocketAdapter struct {
	BaseAdapter
}

// NewWebSocketAdapter creates a new WebSocket adapter.
func NewWebSocketAdapter() *WebSocketAdapter {
	return &WebSocketAdapter{
		BaseAdapter: BaseAdapter{triggerType: TriggerWebSocket},
	}
}

// CanHandle reports whether the adapter recognizes the given raw event as an
// API Gateway WebSocket event.
func (a *WebSocketAdapter) CanHandle(event any) bool {
	eventMap, ok := event.(map[string]any)
	if !ok {
		return false
	}

	// Check for WebSocket specific fields
	requestContext, hasRequestContext := eventMap["requestContext"].(map[string]any)
	if !hasRequestContext {
		return false
	}

	// Check for WebSocket specific properties in requestContext
	_, hasConnectionID := requestContext["connectionId"]
	_, hasRouteKey := requestContext["routeKey"]
	_, hasEventType := requestContext["eventType"]

	// WebSocket events have connectionId and either routeKey or eventType
	return hasConnectionID && (hasRouteKey || hasEventType)
}

// Validate checks that the raw event has the required WebSocket fields before
// adapting it.
func (a *WebSocketAdapter) Validate(event any) error {
	eventMap, ok := event.(map[string]any)
	if !ok {
		return fmt.Errorf("event must be a map[string]any")
	}

	// Check required fields
	requestContext, ok := eventMap["requestContext"].(map[string]any)
	if !ok {
		return fmt.Errorf("missing or invalid requestContext field")
	}

	// Validate connectionId
	if _, ok := requestContext["connectionId"].(string); !ok {
		return fmt.Errorf("missing or invalid connectionId in requestContext")
	}

	// Validate routeKey
	if _, ok := requestContext["routeKey"].(string); !ok {
		return fmt.Errorf("missing or invalid routeKey in requestContext")
	}

	return nil
}

// Adapt converts a WebSocket event into a normalized Request.
func (a *WebSocketAdapter) Adapt(rawEvent any) (*Request, error) {
	if err := a.Validate(rawEvent); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	eventMap, ok := rawEvent.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("event must be a map[string]any, got %T", rawEvent)
	}

	requestContext, ok := eventMap["requestContext"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("requestContext must be a map[string]any")
	}

	// Extract WebSocket specific information
	connectionID := extractStringField(requestContext, "connectionId")
	routeKey := extractStringField(requestContext, "routeKey")
	eventType := extractStringField(requestContext, "eventType")
	stage := extractStringField(requestContext, "stage")
	requestID := extractStringField(requestContext, "requestId")

	// Extract domain and API ID for constructing management endpoint
	domainName := extractStringField(requestContext, "domainName")
	apiID := extractStringField(requestContext, "apiId")

	// Map route key to HTTP method and path
	method, path := mapWebSocketRoute(routeKey)

	// Extract headers (case-insensitive)
	headers := make(map[string]string)
	if headersMap := extractMapField(eventMap, "headers"); len(headersMap) > 0 {
		for k, v := range headersMap {
			if str, ok := v.(string); ok {
				headers[strings.ToLower(k)] = str
			}
		}
	}

	// Extract query parameters (common for $connect route)
	queryParams := extractStringMapField(eventMap, "queryStringParameters")
	if queryParams == nil {
		queryParams = make(map[string]string)
	}

	// Extract body
	var body []byte
	if bodyStr := extractStringField(eventMap, "body"); bodyStr != "" {
		// Check if body is base64 encoded
		if isBase64Encoded, ok := eventMap["isBase64Encoded"].(bool); ok && isBase64Encoded {
			decoded, err := base64.StdEncoding.DecodeString(bodyStr)
			if err != nil {
				return nil, fmt.Errorf("failed to decode base64 body: %w", err)
			}
			body = decoded
		} else {
			body = []byte(bodyStr)
		}
	}

	// Extract timestamp
	timestamp := extractStringField(requestContext, "requestTime")
	if timestamp == "" {
		timestamp = extractStringField(requestContext, "requestTimeEpoch")
	}

	// Create the request with WebSocket specific metadata
	req := &Request{
		TriggerType: TriggerWebSocket,
		RawEvent:    rawEvent,
		EventID:     requestID,
		Timestamp:   timestamp,
		Method:      method,
		Path:        path,
		Headers:     headers,
		QueryParams: queryParams,
		Body:        body,
		Source:      "aws:apigateway:websocket",

		// Store WebSocket specific data in metadata
		Metadata: map[string]any{
			"connectionId":       connectionID,
			"routeKey":           routeKey,
			"eventType":          eventType,
			"stage":              stage,
			"domainName":         domainName,
			"apiId":              apiID,
			"managementEndpoint": fmt.Sprintf("https://%s/%s", domainName, stage),
			"requestContext":     requestContext,
		},
	}

	return req, nil
}

// mapWebSocketRoute maps WebSocket route keys to HTTP method and path
func mapWebSocketRoute(routeKey string) (method, path string) {
	switch routeKey {
	case "$connect":
		return "CONNECT", "/connect"
	case "$disconnect":
		return "DISCONNECT", "/disconnect"
	case "$default":
		return "MESSAGE", "/message"
	default:
		// Custom routes are typically MESSAGE type
		return "MESSAGE", "/" + routeKey
	}
}
