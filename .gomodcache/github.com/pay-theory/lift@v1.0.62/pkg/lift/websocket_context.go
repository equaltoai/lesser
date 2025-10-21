package lift

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/apigatewaymanagementapi"
)

const (
	connectRoute    = "$connect"
	disconnectRoute = "$disconnect"
)

// WebSocketContext provides WebSocket-specific functionality
type WebSocketContext struct {
	*Context
	managementAPI *apigatewaymanagementapi.ApiGatewayManagementApi
	region        string // Configurable AWS region
}

// AsWebSocket converts a regular context to a WebSocket context
func (c *Context) AsWebSocket() (*WebSocketContext, error) {
	if c.Request.TriggerType != TriggerWebSocket {
		return nil, NewLiftError("NOT_WEBSOCKET", "Context is not from a WebSocket event", 400)
	}

	return &WebSocketContext{
		Context: c,
		region:  c.getRegionFromContext(), // Get region from context or environment
	}, nil
}

// WithRegion sets a specific AWS region for the WebSocket context
func (wc *WebSocketContext) WithRegion(region string) *WebSocketContext {
	wc.region = region
	// Reset managementAPI to force re-initialization with new region
	wc.managementAPI = nil
	return wc
}

// GetRegion returns the configured AWS region
func (wc *WebSocketContext) GetRegion() string {
	if wc.region != "" {
		return wc.region
	}

	// Fallback to environment variable or default
	return wc.getRegionFromContext()
}

// ConnectionID returns the WebSocket connection ID
func (wc *WebSocketContext) ConnectionID() string {
	if wc.Request.Metadata == nil {
		return ""
	}
	if connID, ok := wc.Request.Metadata["connectionId"].(string); ok {
		return connID
	}
	return ""
}

// RouteKey returns the WebSocket route key ($connect, $disconnect, or custom route)
func (wc *WebSocketContext) RouteKey() string {
	if wc.Request.Metadata == nil {
		return ""
	}
	if routeKey, ok := wc.Request.Metadata["routeKey"].(string); ok {
		return routeKey
	}
	return ""
}

// EventType returns the WebSocket event type (CONNECT, DISCONNECT, MESSAGE)
func (wc *WebSocketContext) EventType() string {
	if wc.Request.Metadata == nil {
		return ""
	}
	if eventType, ok := wc.Request.Metadata["eventType"].(string); ok {
		return eventType
	}
	return ""
}

// Stage returns the API Gateway stage
func (wc *WebSocketContext) Stage() string {
	if wc.Request.Metadata == nil {
		return ""
	}
	if stage, ok := wc.Request.Metadata["stage"].(string); ok {
		return stage
	}
	return ""
}

// DomainName returns the API Gateway domain name
func (wc *WebSocketContext) DomainName() string {
	if wc.Request.Metadata == nil {
		return ""
	}
	if domainName, ok := wc.Request.Metadata["domainName"].(string); ok {
		return domainName
	}
	return ""
}

// ManagementEndpoint returns the WebSocket management API endpoint
func (wc *WebSocketContext) ManagementEndpoint() string {
	if wc.Request.Metadata == nil {
		return ""
	}
	if endpoint, ok := wc.Request.Metadata["managementEndpoint"].(string); ok {
		return endpoint
	}
	return ""
}

// GetManagementAPI returns an initialized API Gateway Management API client
func (wc *WebSocketContext) GetManagementAPI() (*apigatewaymanagementapi.ApiGatewayManagementApi, error) {
	if wc.managementAPI != nil {
		return wc.managementAPI, nil
	}

	endpoint := wc.ManagementEndpoint()
	if endpoint == "" {
		return nil, fmt.Errorf("management endpoint not found in WebSocket context")
	}

	sess := session.Must(session.NewSession(&aws.Config{
		Endpoint: aws.String(endpoint),
		Region:   aws.String(wc.GetRegion()), // Use configurable region
	}))

	wc.managementAPI = apigatewaymanagementapi.New(sess)
	return wc.managementAPI, nil
}

// SendMessage sends a message to the current WebSocket connection
func (wc *WebSocketContext) SendMessage(data []byte) error {
	mgmtAPI, err := wc.GetManagementAPI()
	if err != nil {
		return fmt.Errorf("failed to get management API: %w", err)
	}

	connectionID := wc.ConnectionID()
	if connectionID == "" {
		return fmt.Errorf("connection ID not found")
	}

	_, err = mgmtAPI.PostToConnection(&apigatewaymanagementapi.PostToConnectionInput{
		ConnectionId: aws.String(connectionID),
		Data:         data,
	})

	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	return nil
}

// SendJSONMessage sends a JSON message to the current WebSocket connection
func (wc *WebSocketContext) SendJSONMessage(data any) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return wc.SendMessage(jsonData)
}

// BroadcastMessage sends a message to multiple WebSocket connections
func (wc *WebSocketContext) BroadcastMessage(connectionIDs []string, data []byte) error {
	mgmtAPI, err := wc.GetManagementAPI()
	if err != nil {
		return fmt.Errorf("failed to get management API: %w", err)
	}

	var errors []error
	for _, connID := range connectionIDs {
		_, err := mgmtAPI.PostToConnection(&apigatewaymanagementapi.PostToConnectionInput{
			ConnectionId: aws.String(connID),
			Data:         data,
		})
		if err != nil {
			errors = append(errors, fmt.Errorf("failed to send to %s: %w", connID, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("broadcast errors: %v", errors)
	}

	return nil
}

// Disconnect forcefully disconnects a WebSocket connection
func (wc *WebSocketContext) Disconnect(connectionID string) error {
	mgmtAPI, err := wc.GetManagementAPI()
	if err != nil {
		return fmt.Errorf("failed to get management API: %w", err)
	}

	_, err = mgmtAPI.DeleteConnection(&apigatewaymanagementapi.DeleteConnectionInput{
		ConnectionId: aws.String(connectionID),
	})

	if err != nil {
		return fmt.Errorf("failed to disconnect: %w", err)
	}

	return nil
}

// GetConnectionInfo retrieves information about a WebSocket connection
func (wc *WebSocketContext) GetConnectionInfo(connectionID string) (*apigatewaymanagementapi.GetConnectionOutput, error) {
	mgmtAPI, err := wc.GetManagementAPI()
	if err != nil {
		return nil, fmt.Errorf("failed to get management API: %w", err)
	}

	return mgmtAPI.GetConnection(&apigatewaymanagementapi.GetConnectionInput{
		ConnectionId: aws.String(connectionID),
	})
}

// IsConnectEvent returns true if this is a $connect event
func (wc *WebSocketContext) IsConnectEvent() bool {
	return wc.RouteKey() == connectRoute
}

// IsDisconnectEvent returns true if this is a $disconnect event
func (wc *WebSocketContext) IsDisconnectEvent() bool {
	return wc.RouteKey() == disconnectRoute
}

// IsMessageEvent returns true if this is a message event (not connect/disconnect)
func (wc *WebSocketContext) IsMessageEvent() bool {
	routeKey := wc.RouteKey()
	return routeKey != connectRoute && routeKey != disconnectRoute
}

// GetAuthorizationFromQuery extracts authorization token from query parameters
// This is commonly used in WebSocket $connect events since headers aren't always available
func (wc *WebSocketContext) GetAuthorizationFromQuery() string {
	return wc.Query("Authorization")
}

// getRegionFromContext extracts AWS region from context, environment, or defaults
func (c *Context) getRegionFromContext() string {
	// 1. Check if region is set in context values
	if region := c.Get("aws_region"); region != nil {
		if regionStr, ok := region.(string); ok && regionStr != "" {
			return regionStr
		}
	}

	// 2. Check request metadata for region information
	if c.Request != nil && c.Request.Metadata != nil {
		if region, ok := c.Request.Metadata["region"].(string); ok && region != "" {
			return region
		}

		// Check request context for region (Lambda execution context)
		if requestContext := c.Request.RequestContext(); len(requestContext) > 0 {
			if region, ok := requestContext["region"].(string); ok && region != "" {
				return region
			}
		}
	}

	// 3. Check environment variables
	if region := os.Getenv("AWS_REGION"); region != "" {
		return region
	}
	if region := os.Getenv("AWS_DEFAULT_REGION"); region != "" {
		return region
	}

	// 4. Default to us-east-1 (most common Lambda region)
	return "us-east-1"
}

// getRegionFromContext for WebSocketContext (delegate to embedded Context)
func (wc *WebSocketContext) getRegionFromContext() string {
	return wc.Context.getRegionFromContext()
}
