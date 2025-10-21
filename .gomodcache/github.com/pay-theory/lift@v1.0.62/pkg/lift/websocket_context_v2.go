package lift

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/apigatewaymanagementapi"
	"github.com/aws/aws-sdk-go-v2/service/apigatewaymanagementapi/types"
)

// WebSocketContextV2 provides WebSocket-specific functionality using AWS SDK v2
type WebSocketContextV2 struct {
	*Context
	managementAPI *apigatewaymanagementapi.Client
	region        string
	apiMutex      sync.Mutex
}

// AsWebSocketV2 converts a regular context to a WebSocket context using SDK v2
func (c *Context) AsWebSocketV2() (*WebSocketContextV2, error) {
	if c.Request.TriggerType != TriggerWebSocket {
		return nil, NewLiftError("NOT_WEBSOCKET", "Context is not from a WebSocket event", 400)
	}

	return &WebSocketContextV2{
		Context: c,
		region:  "us-east-1", // Default region, can be overridden
	}, nil
}

// WithRegion sets the AWS region for the WebSocket context
func (wc *WebSocketContextV2) WithRegion(region string) *WebSocketContextV2 {
	wc.region = region
	return wc
}

// ConnectionID returns the WebSocket connection ID
func (wc *WebSocketContextV2) ConnectionID() string {
	if wc.Request.Metadata == nil {
		return ""
	}
	if connID, ok := wc.Request.Metadata["connectionId"].(string); ok {
		return connID
	}
	return ""
}

// RouteKey returns the WebSocket route key ($connect, $disconnect, or custom route)
func (wc *WebSocketContextV2) RouteKey() string {
	if wc.Request.Metadata == nil {
		return ""
	}
	if routeKey, ok := wc.Request.Metadata["routeKey"].(string); ok {
		return routeKey
	}
	return ""
}

// EventType returns the WebSocket event type (CONNECT, DISCONNECT, MESSAGE)
func (wc *WebSocketContextV2) EventType() string {
	if wc.Request.Metadata == nil {
		return ""
	}
	if eventType, ok := wc.Request.Metadata["eventType"].(string); ok {
		return eventType
	}
	return ""
}

// Stage returns the API Gateway stage
func (wc *WebSocketContextV2) Stage() string {
	if wc.Request.Metadata == nil {
		return ""
	}
	if stage, ok := wc.Request.Metadata["stage"].(string); ok {
		return stage
	}
	return ""
}

// DomainName returns the API Gateway domain name
func (wc *WebSocketContextV2) DomainName() string {
	if wc.Request.Metadata == nil {
		return ""
	}
	if domainName, ok := wc.Request.Metadata["domainName"].(string); ok {
		return domainName
	}
	return ""
}

// ManagementEndpoint returns the WebSocket management API endpoint
func (wc *WebSocketContextV2) ManagementEndpoint() string {
	if wc.Request.Metadata == nil {
		return ""
	}
	if endpoint, ok := wc.Request.Metadata["managementEndpoint"].(string); ok {
		return endpoint
	}
	return ""
}

// GetManagementAPI returns an initialized API Gateway Management API client
func (wc *WebSocketContextV2) GetManagementAPI(ctx context.Context) (*apigatewaymanagementapi.Client, error) {
	wc.apiMutex.Lock()
	defer wc.apiMutex.Unlock()

	if wc.managementAPI != nil {
		return wc.managementAPI, nil
	}

	endpoint := wc.ManagementEndpoint()
	if endpoint == "" {
		return nil, fmt.Errorf("management endpoint not found in WebSocket context")
	}

	// Load AWS configuration
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(wc.region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create API Gateway Management API client with custom endpoint
	wc.managementAPI = apigatewaymanagementapi.NewFromConfig(cfg, func(o *apigatewaymanagementapi.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})

	return wc.managementAPI, nil
}

// SendMessage sends a message to the current WebSocket connection
func (wc *WebSocketContextV2) SendMessage(ctx context.Context, data []byte) error {
	mgmtAPI, err := wc.GetManagementAPI(ctx)
	if err != nil {
		return fmt.Errorf("failed to get management API: %w", err)
	}

	connectionID := wc.ConnectionID()
	if connectionID == "" {
		return fmt.Errorf("connection ID not found")
	}

	_, err = mgmtAPI.PostToConnection(ctx, &apigatewaymanagementapi.PostToConnectionInput{
		ConnectionId: aws.String(connectionID),
		Data:         data,
	})

	if err != nil {
		// Check if it's a GoneException (connection no longer exists)
		var goneErr *types.GoneException
		if errors.As(err, &goneErr) {
			return fmt.Errorf("connection %s is gone: %w", connectionID, err)
		}
		return fmt.Errorf("failed to send message: %w", err)
	}

	return nil
}

// SendJSONMessage sends a JSON message to the current WebSocket connection
func (wc *WebSocketContextV2) SendJSONMessage(ctx context.Context, data any) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return wc.SendMessage(ctx, jsonData)
}

// BroadcastMessage sends a message to multiple WebSocket connections
func (wc *WebSocketContextV2) BroadcastMessage(ctx context.Context, connectionIDs []string, data []byte) error {
	mgmtAPI, err := wc.GetManagementAPI(ctx)
	if err != nil {
		return fmt.Errorf("failed to get management API: %w", err)
	}

	var broadcastErrors []error
	var goneConnections []string

	for _, connID := range connectionIDs {
		_, err := mgmtAPI.PostToConnection(ctx, &apigatewaymanagementapi.PostToConnectionInput{
			ConnectionId: aws.String(connID),
			Data:         data,
		})
		if err != nil {
			// Track gone connections separately
			var goneErr *types.GoneException
			if errors.As(err, &goneErr) {
				goneConnections = append(goneConnections, connID)
			} else {
				broadcastErrors = append(broadcastErrors, fmt.Errorf("failed to send to %s: %w", connID, err))
			}
		}
	}

	// Log gone connections for cleanup
	if len(goneConnections) > 0 && wc.Logger != nil {
		wc.Logger.Info("Gone connections detected", map[string]any{
			"connections": goneConnections,
			"count":       len(goneConnections),
		})
	}

	if len(broadcastErrors) > 0 {
		return fmt.Errorf("broadcast errors: %v", broadcastErrors)
	}

	return nil
}

// BroadcastJSONMessage sends a JSON message to multiple WebSocket connections
func (wc *WebSocketContextV2) BroadcastJSONMessage(ctx context.Context, connectionIDs []string, data any) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return wc.BroadcastMessage(ctx, connectionIDs, jsonData)
}

// Disconnect forcefully disconnects a WebSocket connection
func (wc *WebSocketContextV2) Disconnect(ctx context.Context, connectionID string) error {
	mgmtAPI, err := wc.GetManagementAPI(ctx)
	if err != nil {
		return fmt.Errorf("failed to get management API: %w", err)
	}

	_, err = mgmtAPI.DeleteConnection(ctx, &apigatewaymanagementapi.DeleteConnectionInput{
		ConnectionId: aws.String(connectionID),
	})

	if err != nil {
		// Check if it's a GoneException (connection already gone)
		var goneErr *types.GoneException
		if errors.As(err, &goneErr) {
			// Connection is already gone, not an error
			return nil
		}
		return fmt.Errorf("failed to disconnect: %w", err)
	}

	return nil
}

// GetConnectionInfo retrieves information about a WebSocket connection
func (wc *WebSocketContextV2) GetConnectionInfo(ctx context.Context, connectionID string) (*apigatewaymanagementapi.GetConnectionOutput, error) {
	mgmtAPI, err := wc.GetManagementAPI(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get management API: %w", err)
	}

	output, err := mgmtAPI.GetConnection(ctx, &apigatewaymanagementapi.GetConnectionInput{
		ConnectionId: aws.String(connectionID),
	})
	if err != nil {
		// Check if it's a GoneException
		var goneErr *types.GoneException
		if errors.As(err, &goneErr) {
			return nil, fmt.Errorf("connection %s not found", connectionID)
		}
		return nil, fmt.Errorf("failed to get connection info: %w", err)
	}

	return output, nil
}

// IsConnectEvent returns true if this is a $connect event
func (wc *WebSocketContextV2) IsConnectEvent() bool {
	return wc.RouteKey() == connectRoute
}

// IsDisconnectEvent returns true if this is a $disconnect event
func (wc *WebSocketContextV2) IsDisconnectEvent() bool {
	return wc.RouteKey() == disconnectRoute
}

// IsMessageEvent returns true if this is a message event (not connect/disconnect)
func (wc *WebSocketContextV2) IsMessageEvent() bool {
	routeKey := wc.RouteKey()
	return routeKey != connectRoute && routeKey != disconnectRoute
}

// GetAuthorizationFromQuery extracts authorization token from query parameters
// This is commonly used in WebSocket $connect events since headers aren't always available
func (wc *WebSocketContextV2) GetAuthorizationFromQuery() string {
	return wc.Query("Authorization")
}

// ConnectionMetadata represents metadata about a WebSocket connection
type ConnectionMetadata struct {
	Identity     map[string]string
	ConnectedAt  *time.Time
	LastActiveAt *time.Time
	ConnectionID string
}

// GetConnectionMetadata retrieves metadata about a connection
func (wc *WebSocketContextV2) GetConnectionMetadata(ctx context.Context, connectionID string) (*ConnectionMetadata, error) {
	info, err := wc.GetConnectionInfo(ctx, connectionID)
	if err != nil {
		return nil, err
	}

	metadata := &ConnectionMetadata{
		ConnectionID: connectionID,
		ConnectedAt:  info.ConnectedAt,
		LastActiveAt: info.LastActiveAt,
	}

	// Parse identity if available
	if info.Identity != nil && info.Identity.SourceIp != nil {
		metadata.Identity = map[string]string{
			"sourceIp": *info.Identity.SourceIp,
		}
		if info.Identity.UserAgent != nil {
			metadata.Identity["userAgent"] = *info.Identity.UserAgent
		}
	}

	return metadata, nil
}
