package handlers

import (
	"context"
	"errors"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// HandleGetEndorsements handles GET /api/v1/endorsements
// Returns accounts that the user has endorsed (pinned to their profile)
func (h *Handler) HandleGetEndorsements(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token from Authorization header
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check read:accounts scope
	if !claims.HasScope("read:accounts") && !claims.HasScope(auth.ScopeRead) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Get pinned accounts (which are the endorsed accounts)
	pins, err := h.store.GetAccountPins(ctx, claims.Username)
	if err != nil {
		h.logger.Error("failed to get account pins", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Convert pins to account objects
	accounts := make([]models.Account, 0, len(pins))
	for _, pin := range pins {
		// Extract username from actor ID
		username := h.converter.ExtractUsernameFromActorID(pin.PinnedActorID)
		if username == "" {
			continue
		}

		// Get the actor
		actor, err := h.store.GetActor(ctx, username)
		if err != nil {
			h.logger.Warn("failed to get pinned actor",
				zap.String("actor_id", pin.PinnedActorID),
				zap.Error(err))
			continue
		}

		// Convert to account
		account := h.converter.ActorToAccount(actor)
		accounts = append(accounts, account)
	}

	return common.OK(accounts), nil
}
