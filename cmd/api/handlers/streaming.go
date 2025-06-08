package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aws/aws-lambda-go/events"
)

// SSE event structure that matches Mastodon's format
type SSEEvent struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

// HandleSSEStream handles Server-Sent Events streaming
// This provides an alternative to WebSocket for clients that prefer SSE
func (h *Handler) HandleSSEStream(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	h.logger.Info("HandleSSEStream called")

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

	// Extract stream type from query parameters
	stream := request.QueryStringParameters["stream"]
	if stream == "" {
		stream = "user" // Default to user stream
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

	// For Lambda, we can't actually maintain a long-lived connection
	// This is a limitation of Lambda + API Gateway
	// In a real SSE implementation, you'd need:
	// 1. Lambda Function URLs with response streaming
	// 2. Or use API Gateway v2 with WebSockets (which we already have)
	// 3. Or use a container/EC2 for long-lived connections

	// For now, return a response explaining the limitation and directing to WebSocket
	response := map[string]interface{}{
		"error":         "SSE not supported in Lambda environment",
		"message":       "Please use WebSocket endpoint for real-time streaming",
		"websocket_url": fmt.Sprintf("wss://ws.%s/v1", h.cfg.Domain),
		"documentation": "https://docs.joinmastodon.org/methods/streaming/",
		"note":          "Lambda functions cannot maintain long-lived SSE connections. Use our WebSocket endpoint instead.",
	}

	// Return 400 Bad Request with JSON explaining the issue
	body, _ := json.Marshal(response)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: 400,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}, nil
}
