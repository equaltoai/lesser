package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// SSE event structure that matches Mastodon's format
type SSEEvent struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

// HandleSSEStream handles Server-Sent Events streaming
// This provides an alternative to WebSocket for clients that prefer SSE
func (h *Handler) HandleSSEStream(ctx context.Context, request events.APIGatewayV2HTTPRequest, streamParam ...string) (*events.APIGatewayV2HTTPResponse, error) {
	h.logger.Info("HandleSSEStream called", 
		zap.String("path", request.RequestContext.HTTP.Path),
		zap.Any("query_params", request.QueryStringParameters))

	// Extract access token from query parameters or Authorization header
	token := request.QueryStringParameters["access_token"]
	if token == "" {
		authHeader := request.Headers["Authorization"]
		if authHeader == "" {
			authHeader = request.Headers["authorization"]
		}
		token, _ = auth.ExtractBearerToken(authHeader)
	}

	if token == "" {
		return common.Unauthorized(fmt.Errorf("access token required")), nil
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Extract stream type from route parameter, path, or query parameters
	stream := ""
	
	// First check if stream was passed as a route parameter
	if len(streamParam) > 0 && streamParam[0] != "" {
		stream = streamParam[0]
	} else {
		// Parse stream type from path like /api/v1/streaming/user or /streaming/user
		path := request.RequestContext.HTTP.Path
		if strings.Contains(path, "/streaming/") {
			parts := strings.Split(path, "/streaming/")
			if len(parts) > 1 {
				stream = parts[1]
			}
		}
	}
	
	// Fall back to query parameter if not found
	if stream == "" {
		stream = request.QueryStringParameters["stream"]
		if stream == "" {
			stream = "user" // Default to user stream
		}
	}

	// Validate stream type
	validStreams := []string{"public", "public:local", "public:remote", "user", "user:notification", "list", "direct", "hashtag"}
	isValid := false
	for _, valid := range validStreams {
		if stream == valid || strings.HasPrefix(stream, valid+":") {
			isValid = true
			break
		}
	}

	if !isValid {
		return common.BadRequest(fmt.Errorf("invalid stream type: %s", stream)), nil
	}

	// Check authorization for private streams
	if stream == "user" || strings.HasPrefix(stream, "user:") ||
		stream == "direct" || strings.HasPrefix(stream, "list:") {
		if claims.Subject == "" {
			return common.Forbidden(fmt.Errorf("authentication required for stream: %s", stream)), nil
		}
	}

	// For Mastodon compatibility, we need to return a proper redirect to WebSocket
	// Build the WebSocket URL with the appropriate stream and token
	wsURL := fmt.Sprintf("wss://ws.%s/v1?stream=%s&access_token=%s", h.cfg.Domain, stream, token)
	
	// Some Mastodon clients expect different behavior:
	// 1. Some expect a redirect to WebSocket
	// 2. Some expect an error message
	// 3. Some try SSE first, then fall back to WebSocket
	
	// Check if client explicitly wants SSE (rare, but some clients do)
	acceptHeader := request.Headers["Accept"]
	if acceptHeader == "" {
		acceptHeader = request.Headers["accept"]
	}
	
	// If client explicitly requests SSE, return an error
	if strings.Contains(acceptHeader, "text/event-stream") {
		// Return 501 Not Implemented for SSE
		response := map[string]interface{}{
			"error": "Streaming API requires WebSocket",
			"websocket_url": wsURL,
		}
		body, _ := json.Marshal(response)
		return &events.APIGatewayV2HTTPResponse{
			StatusCode: 501,
			Headers: map[string]string{
				"Content-Type": "application/json",
				"X-Websocket-Url": wsURL,
			},
			Body: string(body),
		}, nil
	}
	
	// Default: Return redirect to WebSocket endpoint
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: 301,
		Headers: map[string]string{
			"Location": wsURL,
			"X-Websocket-Url": wsURL,
		},
	}, nil
}
