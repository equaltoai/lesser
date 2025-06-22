package handlers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// OAuthState represents the state stored during OAuth flow
type OAuthState struct {
	State       string    `json:"state"`
	Provider    string    `json:"provider"`
	RedirectURI string    `json:"redirect_uri"`
	Username    string    `json:"username,omitempty"` // For linking existing account
	CreatedAt   time.Time `json:"created_at"`
}

// HandleOAuthProviderAuthorize initiates OAuth flow with external provider
// GET /oauth/{provider}/authorize
func (h *Handler) HandleOAuthProviderAuthorize(ctx context.Context, request events.APIGatewayV2HTTPRequest, provider string) (*events.APIGatewayV2HTTPResponse, error) {
	// Generate state token
	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		return common.InternalServerError(err), nil
	}
	state := base64.URLEncoding.EncodeToString(stateBytes)

	// Get redirect URI from query params or use default
	redirectURI := request.QueryStringParameters["redirect_uri"]
	if redirectURI == "" {
		// Default to our callback endpoint
		redirectURI = fmt.Sprintf("%s/oauth/%s/callback", h.cfg.BaseURL(), provider)
	}

	// Check if user is linking account (authenticated)
	username := ""
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	if authHeader != "" {
		token, err := auth.ExtractBearerToken(authHeader)
		if err == nil {
			authService, _ := auth.NewAuthService(h.store)
			claims, err := authService.ValidateAccessToken(token)
			if err == nil {
				username = claims.Username
			}
		}
	}

	// Store state in DynamoDB
	oauthState := &storage.OAuthState{
		State:       state,
		Provider:    provider,
		RedirectURI: redirectURI,
		Username:    username,
		CreatedAt:   time.Now(),
		ExpiresAt:   time.Now().Add(10 * time.Minute), // 10 minute expiration
	}

	if err := h.store.StoreOAuthState(ctx, state, oauthState); err != nil {
		h.logger.Error("failed to store OAuth state",
			zap.String("state", state),
			zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to initialize OAuth flow")), nil
	}

	h.logger.Info("stored OAuth state",
		zap.String("state", state),
		zap.String("provider", provider),
		zap.String("username", username))

	// External OAuth providers are no longer supported
	return common.BadRequest(fmt.Errorf("external OAuth providers are not supported")), nil
}

// HandleOAuthProviderCallback handles the callback from external OAuth provider
// GET /oauth/{provider}/callback
func (h *Handler) HandleOAuthProviderCallback(ctx context.Context, request events.APIGatewayV2HTTPRequest, provider string) (*events.APIGatewayV2HTTPResponse, error) {
	// External OAuth providers are no longer supported
	return common.BadRequest(fmt.Errorf("external OAuth providers are not supported")), nil
}

// HandleLinkOAuthProvider links an OAuth provider to existing account
// POST /oauth/{provider}/link
func (h *Handler) HandleLinkOAuthProvider(ctx context.Context, request events.APIGatewayV2HTTPRequest, provider string) (*events.APIGatewayV2HTTPResponse, error) {
	// External OAuth providers are no longer supported
	return common.BadRequest(fmt.Errorf("external OAuth providers are not supported")), nil
}

// HandleUnlinkOAuthProvider unlinks an OAuth provider from account
// DELETE /oauth/{provider}/unlink
func (h *Handler) HandleUnlinkOAuthProvider(ctx context.Context, request events.APIGatewayV2HTTPRequest, provider string) (*events.APIGatewayV2HTTPResponse, error) {
	// External OAuth providers are no longer supported
	return common.BadRequest(fmt.Errorf("external OAuth providers are not supported")), nil
}

// getProvider returns the OAuth provider implementation
func (h *Handler) getProvider(name string) interface{} {
	// OAuth providers have been removed from Lesser
	// This framework is maintained for potential ActivityPub OAuth needs
	return nil
}
