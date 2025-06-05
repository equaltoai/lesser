package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"

	"github.com/aron23/lesser/cmd/api/models"
)

// HandleGetRelationships handles GET /api/v1/accounts/relationships
// It accepts multiple account IDs as query parameters: id[]=1&id[]=2
func (h *Handler) HandleGetRelationships(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
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

	// Check read:follows scope (relationships include follow status)
	if !claims.HasScope("read:follows") && !claims.HasScope(auth.ScopeRead) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Get the user's actor
	actor, err := h.store.GetActor(ctx, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Extract account IDs from query parameters
	accountIDs := extractAccountIDs(request.QueryStringParameters)
	if len(accountIDs) == 0 {
		return common.BadRequest(errors.New("no account IDs provided")), nil
	}

	// Build relationships for each requested account
	relationships := make([]models.Relationship, 0, len(accountIDs))

	for _, accountID := range accountIDs {
		// Skip empty IDs
		if accountID == "" {
			continue
		}

		// Get the target actor
		targetActor, err := h.store.GetActor(ctx, accountID)
		if err != nil {
			// Skip accounts that don't exist
			h.logger.Warn("account not found for relationship",
				zap.String("account_id", accountID),
				zap.Error(err))
			continue
		}

		// Build the relationship
		relationship := h.buildRelationship(ctx, actor, targetActor)
		relationships = append(relationships, relationship)
	}

	body, _ := json.Marshal(relationships)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}, nil
}

// buildRelationship constructs a Relationship object between two actors
func (h *Handler) buildRelationship(ctx context.Context, actor, targetActor *activitypub.Actor) models.Relationship {
	relationship := models.Relationship{
		ID:                  targetActor.PreferredUsername,
		Following:           false,
		ShowingReblogs:      true,
		Notifying:           false,
		FollowedBy:          false,
		Blocking:            false,
		BlockedBy:           false,
		Muting:              false,
		MutingNotifications: false,
		Requested:           false,
		DomainBlocking:      false,
		Endorsed:            false,
		Note:                "",
	}

	// Check if following
	isFollowing, err := h.store.IsFollowing(ctx, actor.ID, targetActor.ID)
	if err == nil && isFollowing {
		relationship.Following = true
		// TODO: Check if this is a pending follow request
		// For now, assume approved if following
		relationship.Requested = false
	}

	// Check if followed by
	isFollowedBy, err := h.store.IsFollowing(ctx, targetActor.ID, actor.ID)
	if err == nil && isFollowedBy {
		relationship.FollowedBy = true
	}

	// Check if blocking
	_, err = h.store.GetBlock(ctx, actor.ID, targetActor.ID)
	if err == nil {
		// Block exists
		relationship.Blocking = true
		// If blocking, can't be following
		relationship.Following = false
		relationship.ShowingReblogs = false
		relationship.Notifying = false
	}

	// Check if blocked by
	_, err = h.store.GetBlock(ctx, targetActor.ID, actor.ID)
	if err == nil {
		// Blocked by the target
		relationship.BlockedBy = true
	}

	// Check if muting
	mute, err := h.store.GetMute(ctx, actor.PreferredUsername, targetActor.PreferredUsername)
	if err == nil && mute != nil {
		relationship.Muting = true
		relationship.MutingNotifications = mute.HideNotifications
	}

	// TODO: Implement domain blocking, endorsements, and notes

	return relationship
}

// extractAccountIDs extracts account IDs from query parameters
// Supports both id[]=1&id[]=2 and id=1,2 formats
func extractAccountIDs(params map[string]string) []string {
	var accountIDs []string

	// Check for array format: id[]=1&id[]=2
	for key, value := range params {
		if strings.HasPrefix(key, "id[") && strings.HasSuffix(key, "]") {
			accountIDs = append(accountIDs, value)
		}
	}

	// If no array format found, check for comma-separated format: id=1,2
	if len(accountIDs) == 0 {
		if idParam, ok := params["id"]; ok && idParam != "" {
			accountIDs = strings.Split(idParam, ",")
		}
	}

	// Remove duplicates
	seen := make(map[string]bool)
	unique := []string{}
	for _, id := range accountIDs {
		id = strings.TrimSpace(id)
		if id != "" && !seen[id] {
			seen[id] = true
			unique = append(unique, id)
		}
	}

	return unique
}
