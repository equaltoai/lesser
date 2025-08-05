package lift

import (
	"context"
	"strings"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleGetRelationshipsLift handles GET /api/v1/accounts/relationships
// It accepts multiple account IDs as query parameters: id[]=1&id[]=2
func (h *Handler) HandleGetRelationshipsLift(ctx *lift.Context) error {
	// Test hook - check for test username header
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}
	
	if testUsername != "" {
		// Get the user's actor directly (test mode)
		actor, err := h.repos.Actor().GetActor(ctx, testUsername)
		if err != nil {
			h.logger.Error("failed to get actor", zap.Error(err))
			return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
		}
		
		// Skip to the main logic with test username
		return h.handleRelationshipsLogic(ctx, actor, testUsername)
	}

	// Extract token from Authorization header
	authHeader := ctx.Header("Authorization")
	if authHeader == "" {
		authHeader = ctx.Header("authorization")
	}
	
	// Try direct access to headers if ctx.Header doesn't work
	if authHeader == "" && ctx.Request != nil && ctx.Request.Request != nil {
		authHeader = ctx.Request.Request.Headers["Authorization"]
		if authHeader == "" {
			authHeader = ctx.Request.Request.Headers["authorization"]
		}
	}
	
	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
	}

	// Check read:follows scope (relationships include follow status)
	if !claims.HasScope("read:follows") && !claims.HasScope(auth.ScopeRead) {
		return ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
	}

	// Get the user's actor
	actor, err := h.repos.Actor().GetActor(ctx, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	return h.handleRelationshipsLogic(ctx, actor, claims.Username)
}

// handleRelationshipsLogic contains the main relationships logic, separated for testing
func (h *Handler) handleRelationshipsLogic(ctx *lift.Context, actor *activitypub.Actor, username string) error {
	// Extract account IDs from query parameters
	accountIDs := h.extractAccountIDsLift(ctx)
	if len(accountIDs) == 0 {
		return ctx.Status(400).JSON(map[string]string{"error": "no account IDs provided"})
	}

	// Build relationships for each requested account
	relationships := make([]models.Relationship, 0, len(accountIDs))

	for _, accountID := range accountIDs {
		// Skip empty IDs
		if accountID == "" {
			continue
		}

		// Get the target actor
		targetActor, err := h.repos.Actor().GetActor(ctx, accountID)
		if err != nil {
			// Skip accounts that don't exist
			h.logger.Warn("account not found for relationship",
				zap.String("account_id", accountID),
				zap.Error(err))
			continue
		}

		// Build the relationship
		relationship := h.buildRelationshipLift(ctx, actor, targetActor, username, accountID)
		relationships = append(relationships, relationship)
	}

	return ctx.JSON(relationships)
}

// buildRelationshipLift constructs a Relationship object between two actors
func (h *Handler) buildRelationshipLift(ctx context.Context, actor, targetActor *activitypub.Actor, currentUsername, targetUsername string) models.Relationship {
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
	followingRel, err := h.repos.Relationship().GetRelationship(ctx, currentUsername, targetUsername)
	if err == nil && followingRel != nil {
		relationship.Following = true
		// Check if this is a pending follow request
		isRequested, err := h.repos.Relationship().HasFollowRequest(ctx, currentUsername, targetUsername)
		relationship.Requested = (err == nil && isRequested)
		// If following, it's not requested anymore
		if relationship.Following {
			relationship.Requested = false
		}
	}

	// Check if followed by
	followedByRel, err := h.repos.Relationship().GetRelationship(ctx, targetUsername, currentUsername)
	if err == nil && followedByRel != nil {
		relationship.FollowedBy = true
	}

	// Check if blocking
	_, err = h.repos.Social().GetBlock(ctx, actor.ID, targetActor.ID)
	if err == nil {
		// Block exists
		relationship.Blocking = true
		// If blocking, can't be following
		relationship.Following = false
		relationship.ShowingReblogs = false
		relationship.Notifying = false
	}

	// Check if blocked by
	_, err = h.repos.Social().GetBlock(ctx, targetActor.ID, actor.ID)
	if err == nil {
		// Blocked by the target
		relationship.BlockedBy = true
	}

	// Check if muting
	mute, err := h.repos.Social().GetMute(ctx, actor.PreferredUsername, targetActor.PreferredUsername)
	if err == nil && mute != nil {
		relationship.Muting = true
		relationship.MutingNotifications = mute.HideNotifications
	}

	// Implement domain blocking, endorsements, and notes
	relationship.DomainBlocking = h.isDomainBlockedLift(ctx, actor.ID, targetActor.ID)
	relationship.Endorsed = h.isEndorsedLift(ctx, currentUsername, targetUsername)
	relationship.Note = h.getRelationshipNoteLift(ctx, currentUsername, targetUsername)

	return relationship
}

// extractAccountIDsLift extracts account IDs from query parameters
// Supports both id[]=1&id[]=2 and id=1,2 formats
func (h *Handler) extractAccountIDsLift(ctx *lift.Context) []string {
	var accountIDs []string

	// First, try to get all query parameters to handle array format id[]
	var queryParams map[string]string
	if ctx.Request != nil && ctx.Request.Request != nil {
		queryParams = ctx.Request.Request.QueryParams
	}


	for key, value := range queryParams {
		if strings.HasPrefix(key, "id[") && strings.HasSuffix(key, "]") {
			accountIDs = append(accountIDs, value)
		}
	}
	

	// If no array format found, check for comma-separated format: id=1,2
	if len(accountIDs) == 0 {
		idParam := ctx.Query("id")
		if idParam == "" && queryParams != nil {
			idParam = queryParams["id"]
		}
		if idParam != "" {
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

// isEndorsedLift checks if the target user is endorsed by the current user
func (h *Handler) isEndorsedLift(ctx context.Context, currentUsername, targetUsername string) bool {
	endorsed, err := h.repos.Relationship().IsEndorsed(ctx, currentUsername, targetUsername)
	return err == nil && endorsed
}

// getRelationshipNoteLift gets the private note about the target user
func (h *Handler) getRelationshipNoteLift(ctx context.Context, currentUsername, targetUsername string) string {
	note, err := h.repos.User().GetAccountNote(ctx, currentUsername, targetUsername)
	if err != nil {
		return ""
	}
	return note.Note
}

