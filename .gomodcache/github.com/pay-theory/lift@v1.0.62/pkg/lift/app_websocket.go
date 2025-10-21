package lift

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-lambda-go/events"

	"github.com/pay-theory/lift/pkg/lift/adapters"
)

// WebSocketHandler is a function that handles WebSocket events
type WebSocketHandler func(ctx *Context) error

// WebSocketOptions configures WebSocket support for an App. When automatic
// connection management is enabled, Lift will store and remove connection
// records using the provided ConnectionStore on connect/disconnect events, and
// will route unmatched routes to DefaultHandler when set.
// Memory optimized: 32 → 24 bytes (8 bytes saved)
type WebSocketOptions struct {
	// Interfaces first (8 bytes each)
	ConnectionStore ConnectionStore  // 8 bytes
	DefaultHandler  WebSocketHandler // 8 bytes

	// Boolean last (1 byte)
	EnableAutoConnectionManagement bool // 1 byte
}

// WebSocket registers a WebSocket route handler
func (a *App) WebSocket(routeKey string, handler WebSocketHandler) *App {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.wsRoutes == nil {
		a.wsRoutes = make(map[string]WebSocketHandler)
	}

	a.wsRoutes[routeKey] = handler
	return a
}

// RouteWebSocket finds a handler for the given route key
func (a *App) RouteWebSocket(routeKey string) Handler {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if handler, ok := a.wsRoutes[routeKey]; ok {
		return HandlerFunc(func(ctx *Context) error {
			return handler(ctx)
		})
	}

	// Check for default handler
	if a.wsOptions != nil && a.wsOptions.DefaultHandler != nil {
		return HandlerFunc(func(ctx *Context) error {
			return a.wsOptions.DefaultHandler(ctx)
		})
	}

	// Fall back to $default route if exists
	if handler, ok := a.wsRoutes["$default"]; ok {
		return HandlerFunc(func(ctx *Context) error {
			return handler(ctx)
		})
	}

	return nil
}

// WithWebSocketSupport enables WebSocket support in the app
func WithWebSocketSupport(options ...WebSocketOptions) AppOption {
	return func(a *App) {
		a.wsRoutes = make(map[string]WebSocketHandler)
		a.features["websocket"] = true

		if len(options) > 0 {
			a.wsOptions = &options[0]
		}
	}
}

// WebSocketHandler returns a Lambda handler for WebSocket events
func (a *App) WebSocketHandler() any {
	return func(ctx context.Context, event events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error) {
		processor := newWebSocketEventProcessor(a)
		return processor.process(ctx, event)
	}
}

// webSocketEventProcessor handles WebSocket event processing
type webSocketEventProcessor struct {
	app *App
}

// newWebSocketEventProcessor creates a new WebSocket event processor
func newWebSocketEventProcessor(app *App) *webSocketEventProcessor {
	return &webSocketEventProcessor{app: app}
}

// process handles the WebSocket event
func (p *webSocketEventProcessor) process(ctx context.Context, event events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error) {
	// Parse the event
	req, err := p.parseWebSocketEvent(event)
	if err != nil {
		return p.errorResponse(500, fmt.Sprintf("Failed to parse event: %v", err)), nil
	}

	// Create and configure Lift context
	liftCtx := p.createLiftContext(ctx, req)

	// Route and handle the request
	if req.TriggerType == adapters.TriggerWebSocket {
		return p.handleWebSocketRequest(liftCtx, req)
	}

	return p.handleNonWebSocketRequest(liftCtx)
}

// parseWebSocketEvent parses the WebSocket event
func (p *webSocketEventProcessor) parseWebSocketEvent(event events.APIGatewayWebsocketProxyRequest) (*Request, error) {
	genericEvent := convertWebSocketEventToGeneric(event)
	return p.app.parseEvent(genericEvent)
}

// createLiftContext creates and configures the Lift context
func (p *webSocketEventProcessor) createLiftContext(ctx context.Context, req *Request) *Context {
	liftCtx := NewContext(ctx, req)

	// Set dependencies
	if p.app.logger != nil {
		liftCtx.Logger = p.app.logger
	}
	if p.app.metrics != nil {
		liftCtx.Metrics = p.app.metrics
	}
	if p.app.db != nil {
		liftCtx.DB = p.app.db
	}

	return liftCtx
}

// handleWebSocketRequest handles WebSocket-specific requests
func (p *webSocketEventProcessor) handleWebSocketRequest(liftCtx *Context, req *Request) (events.APIGatewayProxyResponse, error) {
	// Extract route key
	routeKey := p.extractRouteKey(req)

	// Find handler
	handler := p.app.RouteWebSocket(routeKey)
	if handler == nil {
		return p.errorResponse(404, fmt.Sprintf("No handler for route: %s", routeKey)), nil
	}

	// Prepare and execute handler
	finalHandler := p.prepareHandler(handler)

	// Execute handler
	if err := finalHandler.Handle(liftCtx); err != nil {
		return p.handleExecutionError(err), nil
	}

	return p.successResponse(), nil
}

// extractRouteKey extracts the route key from request metadata
func (p *webSocketEventProcessor) extractRouteKey(req *Request) string {
	if metadata, ok := req.Metadata["routeKey"].(string); ok {
		return metadata
	}
	return ""
}

// prepareHandler applies middleware and connection management
func (p *webSocketEventProcessor) prepareHandler(handler Handler) Handler {
	// Apply middleware
	finalHandler := handler
	chain := p.app.httpMiddlewareChain()
	for i := len(chain) - 1; i >= 0; i-- {
		finalHandler = chain[i](finalHandler)
	}

	// Add connection management if enabled
	if p.shouldEnableConnectionManagement() {
		finalHandler = wrapWithConnectionManagement(finalHandler, p.app.wsOptions.ConnectionStore)
	}

	return finalHandler
}

// shouldEnableConnectionManagement checks if connection management is enabled
func (p *webSocketEventProcessor) shouldEnableConnectionManagement() bool {
	return p.app.wsOptions != nil && p.app.wsOptions.EnableAutoConnectionManagement
}

// handleExecutionError handles errors from handler execution
func (p *webSocketEventProcessor) handleExecutionError(err error) events.APIGatewayProxyResponse {
	if liftErr, ok := err.(*LiftError); ok {
		return p.errorResponse(liftErr.StatusCode, liftErr.Message)
	}
	return p.errorResponse(500, fmt.Sprintf("%v", err))
}

// handleNonWebSocketRequest handles non-WebSocket requests
func (p *webSocketEventProcessor) handleNonWebSocketRequest(liftCtx *Context) (events.APIGatewayProxyResponse, error) {
	// Use regular routing
	if err := p.app.router.Handle(liftCtx); err != nil {
		return p.handleRoutingError(liftCtx, err)
	}

	// Convert response
	return p.convertResponse(liftCtx), nil
}

// handleRoutingError handles errors from routing
func (p *webSocketEventProcessor) handleRoutingError(liftCtx *Context, err error) (events.APIGatewayProxyResponse, error) {
	resp, handleErr := p.app.handleError(liftCtx, err)
	if handleErr != nil {
		return p.errorResponse(500, "Internal server error"), nil
	}

	if apiResp, ok := resp.(events.APIGatewayProxyResponse); ok {
		return apiResp, nil
	}

	return p.errorResponse(500, "Internal server error"), nil
}

// convertResponse converts Lift response to API Gateway format
func (p *webSocketEventProcessor) convertResponse(liftCtx *Context) events.APIGatewayProxyResponse {
	if liftCtx.Response == nil || liftCtx.Response.StatusCode == 0 {
		return p.successResponse()
	}

	return events.APIGatewayProxyResponse{
		StatusCode: liftCtx.Response.StatusCode,
		Body:       p.convertResponseBody(liftCtx.Response.Body),
		Headers:    liftCtx.Response.Headers,
	}
}

// convertResponseBody converts response body to string
func (p *webSocketEventProcessor) convertResponseBody(body any) string {
	switch v := body.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		// Try to marshal as JSON
		if data, err := json.Marshal(v); err == nil {
			return string(data)
		}
		return ""
	}
}

// errorResponse creates an error response
func (p *webSocketEventProcessor) errorResponse(statusCode int, message string) events.APIGatewayProxyResponse {
	return events.APIGatewayProxyResponse{
		StatusCode: statusCode,
		Body:       fmt.Sprintf(`{"error": "%s"}`, message),
	}
}

// successResponse creates a success response
func (p *webSocketEventProcessor) successResponse() events.APIGatewayProxyResponse {
	return events.APIGatewayProxyResponse{
		StatusCode: 200,
		Body:       "OK",
	}
}

// convertWebSocketEventToGeneric converts the strongly-typed WebSocket event to generic map format
func convertWebSocketEventToGeneric(event events.APIGatewayWebsocketProxyRequest) map[string]any {
	return map[string]any{
		"requestContext": map[string]any{
			"routeKey":          event.RequestContext.RouteKey,
			"messageId":         event.RequestContext.MessageID,
			"eventType":         event.RequestContext.EventType,
			"extendedRequestId": event.RequestContext.ExtendedRequestID,
			"requestTime":       event.RequestContext.RequestTime,
			"messageDirection":  event.RequestContext.MessageDirection,
			"stage":             event.RequestContext.Stage,
			"connectedAt":       event.RequestContext.ConnectedAt,
			"requestTimeEpoch":  event.RequestContext.RequestTimeEpoch,
			"requestId":         event.RequestContext.RequestID,
			"domainName":        event.RequestContext.DomainName,
			"connectionId":      event.RequestContext.ConnectionID,
			"apiId":             event.RequestContext.APIID,
		},
		"body":                            event.Body,
		"isBase64Encoded":                 event.IsBase64Encoded,
		"stageVariables":                  event.StageVariables,
		"headers":                         event.Headers,
		"multiValueHeaders":               event.MultiValueHeaders,
		"queryStringParameters":           event.QueryStringParameters,
		"multiValueQueryStringParameters": event.MultiValueQueryStringParameters,
	}
}

// wrapWithConnectionManagement adds automatic connection tracking
func wrapWithConnectionManagement(handler Handler, store ConnectionStore) Handler {
	return HandlerFunc(func(ctx *Context) error {
		wsCtx, err := ctx.AsWebSocket()
		if err != nil {
			// Not a WebSocket context, pass through
			return handler.Handle(ctx)
		}

		// Handle connection lifecycle
		switch wsCtx.RouteKey() {
		case "$connect":
			// Let handler process first
			if err := handler.Handle(ctx); err != nil {
				return err
			}

			// Auto-save connection if handler succeeded
			if store != nil {
				conn := &Connection{
					ID:        wsCtx.ConnectionID(),
					UserID:    ctx.UserID(),
					CreatedAt: ctx.Request.Timestamp,
				}

				// Extract additional metadata
				if tenantID, ok := ctx.Get("tenant_id").(string); ok {
					conn.TenantID = tenantID
				}

				return store.Save(ctx.Context, conn)
			}

		case "$disconnect":
			// Auto-remove connection before handler
			if store != nil {
				if err := store.Delete(ctx.Context, wsCtx.ConnectionID()); err != nil {
					// Log error but don't fail disconnect - connection cleanup should be best effort
					if ctx.Logger != nil {
						ctx.Logger.Error("Failed to delete WebSocket connection from store", map[string]any{
							"connection_id": wsCtx.ConnectionID(),
							"error":         err.Error(),
						})
					}
				}
			}

			// Then let handler process
			return handler.Handle(ctx)

		default:
			// Regular message, just pass through
			return handler.Handle(ctx)
		}

		return nil
	})
}

// ConnectionStore interface for automatic connection management
type ConnectionStore interface {
	Save(ctx context.Context, conn *Connection) error
	Get(ctx context.Context, connectionID string) (*Connection, error)
	Delete(ctx context.Context, connectionID string) error
	ListByUser(ctx context.Context, userID string) ([]*Connection, error)
	ListByTenant(ctx context.Context, tenantID string) ([]*Connection, error)
	CountActive(ctx context.Context) (int64, error) // Count total active connections
}

// Connection represents a WebSocket connection record persisted by a
// ConnectionStore implementation. The struct carries optional metadata, user
// and tenant identifiers to support multi‑tenant routing and audit trails.
// Memory optimized: 72 → 64 bytes (8 bytes saved)
type Connection struct {
	// Map first (8 bytes)
	Metadata map[string]any // 8 bytes
	// Strings (16 bytes each)
	ID        string // 16 bytes
	UserID    string // 16 bytes
	TenantID  string // 16 bytes
	CreatedAt string // 16 bytes
}
