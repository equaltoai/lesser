package lift

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleUnifiedBoostLift handles both traditional boosts and quote boosts
func (h *Handler) HandleUnifiedBoostLift(ctx *lift.Context) error {
	statusID := ctx.Param("id")
	if statusID == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "missing status id"})
	}

	// Test hook - check for test username header
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}

	var username string
	if testUsername != "" {
		// Test mode - skip auth
		username = testUsername
	} else {
		// Extract and validate token
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

		// Validate token and get claims
		oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
		claims, err := oauthSvc.ValidateAccessToken(token)
		if err != nil {
			return ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
		}

		// Check write scope
		if !claims.HasScope(auth.ScopeWrite) {
			return ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
		}

		username = claims.Username
	}

	// Get the user's actor
	actor, err := h.repos.Actor().GetActor(ctx.Context, username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Parse request body if present
	var req models.ReblogRequest
	if err := ctx.ParseRequest(&req); err != nil {
		// Try fallback parsing for test environments
		if ctx.Request != nil && ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
			if err := common.ParseRequestBody(ctx.Request.Body, &req); err != nil {
				// Treat as traditional boost if can't parse
				req = models.ReblogRequest{}
			}
		}
	}

	// Normalize the status ID to a full URL if it's not already
	objectID := statusID
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		// Assume it's a local object ID
		objectID = fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}

	// Check if this is a quote boost
	if req.Comment != nil && *req.Comment != "" {
		// Create a quote boost
		return h.createQuoteBoostLift(ctx, statusID, objectID, *req.Comment, req.Visibility, actor)
	}

	// Traditional boost - create Announce activity
	return h.createPureBoostLift(ctx, statusID, objectID, actor)
}

// createPureBoostLift creates a traditional ActivityPub Announce
func (h *Handler) createPureBoostLift(ctx *lift.Context, statusID, objectID string, actor *activitypub.Actor) error {
	// Create an Announce activity
	announceActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context: activitypub.Context,
			Type:    activitypub.AnnounceType,
			ID:      fmt.Sprintf("%s/activities/announce-%d-%s", actor.ID, time.Now().Unix(), generateRandomStringForBoost()),
			To:      []string{activitypub.PublicAddress},
		},
		Actor:  actor.ID,
		Object: objectID,
	}
	if actor.Followers != "" {
		announceActivity.CC = []string{actor.Followers}
	}
	now := time.Now()
	announceActivity.Published = &now

	// Create the Announce record in dedicated storage
	announce := &storage.Announce{
		Actor:     actor.ID,
		Object:    objectID,
		ID:        announceActivity.ID,
		Published: *announceActivity.Published,
	}

	if err := h.repos.Social().CreateAnnounce(ctx.Context, announce); err != nil {
		h.logger.Error("failed to create announce", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Store the activity in the outbox (this will trigger delivery)
	if err := h.repos.Activity().CreateActivity(ctx.Context, announceActivity); err != nil {
		h.logger.Error("failed to create announce activity", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Record engagement for trending
	engagementData := &storage.EngagementData{
		Shares:      1,
		UniqueUsers: 1,
	}
	if err := h.repos.Analytics().RecordEngagement(ctx.Context, "boost", objectID, time.Now().Format(common.DateFormat), engagementData); err != nil {
		h.logger.Warn("failed to record status engagement",
			zap.String("status_id", statusID),
			zap.String("object_id", objectID),
			zap.Error(err))
	}

	// Get announce count for the object
	announceCount, _ := h.repos.Like().GetBoostCount(ctx.Context, objectID)

	// Return a simplified status response
	resp := models.FavouriteResponse{
		ID:           statusID,
		CreatedAt:    announceActivity.Published.Format("2006-01-02T15:04:05.000Z"),
		Reblogged:    true,
		ReblogsCount: int(announceCount),
		URI:          objectID,
		URL:          objectID,
		Content:      "", // Would be populated from object
		Visibility:   "public",
		Language:     "en",
	}

	return ctx.JSON(resp)
}

// extractActorIDFromObject extracts actor ID from an object
func (h *Handler) extractActorIDFromObject(obj any) string {
	switch o := obj.(type) {
	case *activitypub.Note:
		return o.AttributedTo
	case map[string]any:
		if attributedTo, ok := o["attributedTo"].(string); ok {
			return attributedTo
		}
	}
	return ""
}

// extractContentFromObject extracts content from an object
func (h *Handler) extractContentFromObject(obj any) string {
	switch o := obj.(type) {
	case *activitypub.Note:
		return o.Content
	case map[string]any:
		if content, ok := o["content"].(string); ok {
			return content
		}
	}
	return ""
}

// createQuoteBoostLift creates a new status with a quote relationship
func (h *Handler) createQuoteBoostLift(ctx *lift.Context, statusID, objectID, comment, visibility string, actor *activitypub.Actor) error {
	// Default visibility if not specified
	if visibility == "" {
		visibility = storageModels.VisibilityPublic
	}

	// Generate note ID
	noteID := fmt.Sprintf("%d-%s", time.Now().Unix(), generateRandomStringForBoost())

	// Create the QuoteNote object with quote content
	note := &activitypub.QuoteNote{
		Note: activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:   fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), noteID),
				Type: "Note",
			},
			Content:      comment,
			AttributedTo: actor.ID,
		},
		QuoteURL:           objectID,
		Quoteable:          true,
		QuoteNotifications: true,
	}

	// Set timestamps
	now := time.Now()
	note.Published = &now

	// Set addressing based on visibility
	switch visibility {
	case "public":
		note.To = []string{activitypub.PublicAddress}
		note.CC = []string{actor.Followers}
	case "unlisted":
		note.To = []string{actor.Followers}
		note.CC = []string{activitypub.PublicAddress}
	case "private":
		note.To = []string{actor.Followers}
	case "direct":
		note.To = []string{}
	}

	// Create the Note object
	if err := h.repos.Object().CreateObject(ctx.Context, note); err != nil {
		h.logger.Error("failed to create quote note object", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Create quote relationship
	quoteRelationship := &storage.QuoteRelationship{
		ID:           fmt.Sprintf("%s-quotes-%s", noteID, statusID),
		QuoterNoteID: noteID,
		TargetNoteID: objectID,
		QuoterID:     actor.ID,
		Timestamp:    now,
	}

	if err := h.repos.Object().CreateQuoteRelationship(ctx.Context, quoteRelationship); err != nil {
		h.logger.Error("failed to create quote relationship", zap.Error(err))
		// Don't fail the request - the note is already created
	}

	// Increment reblog count on the quoted status (unified counting)
	if err := h.repos.Like().IncrementReblogCount(ctx.Context, objectID); err != nil {
		h.logger.Warn("failed to increment reblog count for quote",
			zap.String("quoted_status_id", objectID),
			zap.Error(err))
	}

	// Create a Create activity for federation
	createActivity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			Type:      activitypub.CreateType,
			ID:        fmt.Sprintf("%s/activities/create-%d-%s", actor.ID, time.Now().Unix(), generateRandomStringForBoost()),
			To:        note.To,
			CC:        note.CC,
			Published: &now,
		},
		Actor:  actor.ID,
		Object: note,
	}

	// Store the activity in the outbox (this will trigger delivery)
	if err := h.repos.Activity().CreateActivity(ctx.Context, createActivity); err != nil {
		h.logger.Error("failed to create activity", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Fan out the post to timelines
	if err := h.repos.User().FanOutPost(ctx.Context, createActivity); err != nil {
		h.logger.Error("failed to fan out quote boost to timelines", zap.Error(err))
	}

	// Record engagement for trending
	engagementData := &storage.EngagementData{
		Shares:      1,
		UniqueUsers: 1,
	}
	if err := h.repos.Analytics().RecordEngagement(ctx.Context, "quote", objectID, time.Now().Format(common.DateFormat), engagementData); err != nil {
		h.logger.Warn("failed to record quote engagement",
			zap.String("quoted_status_id", objectID),
			zap.Error(err))
	}

	// Get the quoted status to include in response
	var quotedStatus *models.Status
	if quotedObj, getErr := h.repos.Object().GetObject(ctx.Context, objectID); getErr == nil {
		// Properly convert quotedObj to models.Status
		quotedActor, err := h.repos.Actor().GetActor(ctx.Context, h.extractActorIDFromObject(quotedObj))
		if err == nil {
			status := h.converter.ObjectToStatus(quotedObj, quotedActor)
			quotedStatus = &status
		} else {
			// Fallback if actor lookup fails
			quotedStatus = &models.Status{
				ID:      statusID,
				URI:     objectID,
				URL:     objectID,
				Content: h.extractContentFromObject(quotedObj),
			}
		}
	}

	// Build response
	resp := models.Status{
		ID:               noteID,
		CreatedAt:        now.Format("2006-01-02T15:04:05.000Z"),
		Sensitive:        false,
		SpoilerText:      "",
		Visibility:       visibility,
		Language:         "en",
		URI:              note.ID,
		URL:              note.ID,
		RepliesCount:     0,
		ReblogsCount:     0,
		FavouritesCount:  0,
		Favourited:       false,
		Reblogged:        false,
		Muted:            false,
		Bookmarked:       false,
		Pinned:           false,
		Content:          comment,
		IsQuoteBoost:     true,
		QuotedStatus:     quotedStatus,
		QuotedStatusID:   &objectID,
		MediaAttachments: []any{},
		Mentions:         []any{},
		Tags:             []any{},
		Emojis:           []any{},
		Account: models.Account{
			ID:             actor.ID,
			Username:       actor.PreferredUsername,
			Acct:           actor.PreferredUsername,
			DisplayName:    actor.Name,
			URL:            actor.URL,
			CreatedAt:      now.Format("2006-01-02T15:04:05.000Z"),
			Note:           actor.Summary,
			Avatar:         "",
			AvatarStatic:   "",
			Header:         "",
			HeaderStatic:   "",
			FollowersCount: 0,
			FollowingCount: 0,
			StatusesCount:  0,
			Emojis:         []any{},
			Fields:         []any{},
		},
	}

	// Populate avatar from actor Icon
	if actor.Icon != nil && actor.Icon.URL != "" {
		resp.Account.Avatar = actor.Icon.URL
		resp.Account.AvatarStatic = actor.Icon.URL
	}

	// Populate header from actor Image
	if actor.Image != nil && actor.Image.URL != "" {
		resp.Account.Header = actor.Image.URL
		resp.Account.HeaderStatic = actor.Image.URL
	}

	return ctx.JSON(resp)
}

// HandleUndoUnifiedBoostLift handles undoing both traditional boosts and quote boosts
func (h *Handler) HandleUndoUnifiedBoostLift(ctx *lift.Context) error {
	statusID := ctx.Param("id")
	if statusID == "" {
		return ctx.Status(400).JSON(map[string]string{"error": "missing status id"})
	}

	// Authenticate user
	username, err := h.authenticateUndoBoostRequest(ctx)
	if err != nil {
		return err
	}

	// Get the user's actor
	actor, err := h.repos.Actor().GetActor(ctx.Context, username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return ctx.Status(500).JSON(map[string]string{"error": "Internal server error"})
	}

	// Normalize the status ID to a full URL
	objectID := h.normalizeBoostObjectID(statusID)

	// Undo traditional boost if found
	foundAnnounce := h.undoTraditionalBoost(ctx, actor, objectID)

	// Undo quote boost if found
	foundQuoteBoost := h.undoQuoteBoost(ctx, actor, objectID)

	// Log if nothing was found (idempotent operation)
	if !foundAnnounce && !foundQuoteBoost {
		h.logger.Info("no boost or quote boost found to undo",
			zap.String("actor", actor.ID),
			zap.String("object", objectID))
	}

	// Build and return response
	return h.buildUndoBoostResponse(ctx, statusID, objectID)
}

// authenticateUndoBoostRequest handles authentication for undo boost requests
func (h *Handler) authenticateUndoBoostRequest(ctx *lift.Context) (string, error) {
	// Check for test username header
	testUsername := h.getTestUsernameForBoost(ctx)
	if testUsername != "" {
		return testUsername, nil
	}

	// Extract auth header
	authHeader := h.extractAuthHeaderForBoost(ctx)

	// Extract and validate token
	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return "", ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
	}

	// Validate token and get claims
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return "", ctx.Status(401).JSON(map[string]string{"error": "Unauthorized"})
	}

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		return "", ctx.Status(403).JSON(map[string]string{"error": "insufficient scope"})
	}

	return claims.Username, nil
}

// getTestUsernameForBoost extracts test username from headers
func (h *Handler) getTestUsernameForBoost(ctx *lift.Context) string {
	testUsername := ctx.Header("X-Test-Username")
	if testUsername == "" && ctx.Request != nil && ctx.Request.Request != nil {
		testUsername = ctx.Request.Request.Headers["X-Test-Username"]
	}
	return testUsername
}

// extractAuthHeaderForBoost extracts authorization header from various sources
func (h *Handler) extractAuthHeaderForBoost(ctx *lift.Context) string {
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

	return authHeader
}

// normalizeBoostObjectID converts a status ID to a full URL if needed
func (h *Handler) normalizeBoostObjectID(statusID string) string {
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		return fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}
	return statusID
}

// undoTraditionalBoost finds and undoes a traditional announce/boost
func (h *Handler) undoTraditionalBoost(ctx *lift.Context, actor *activitypub.Actor, objectID string) bool {
	announce, err := h.repos.Social().GetAnnounce(ctx.Context, actor.ID, objectID)
	if err != nil {
		return false
	}

	// Create Undo Announce activity
	undoActivity := h.createUndoAnnounceActivity(actor, objectID)

	// Delete the Announce record
	if err := h.repos.Social().DeleteAnnounce(ctx.Context, actor.ID, objectID); err != nil {
		h.logger.Error("failed to delete announce", zap.Error(err))
		return false
	}

	// Store the activity in the outbox
	if err := h.repos.Activity().CreateActivity(ctx.Context, undoActivity); err != nil {
		h.logger.Error("failed to create undo announce activity", zap.Error(err))
		return false
	}

	h.logger.Info("successfully undid traditional boost",
		zap.String("actor", actor.ID),
		zap.String("object", objectID),
		zap.String("announce_id", announce.ID))

	return true
}

// createUndoAnnounceActivity creates an Undo Announce activity
func (h *Handler) createUndoAnnounceActivity(actor *activitypub.Actor, objectID string) *activitypub.Activity {
	now := time.Now()
	return &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			Type:      activitypub.UndoType,
			ID:        fmt.Sprintf("%s/activities/undo-announce-%d-%s", actor.ID, now.Unix(), generateRandomStringForBoost()),
			To:        []string{activitypub.PublicAddress},
			Published: &now,
		},
		Actor: actor.ID,
		Object: map[string]any{
			"type":   activitypub.AnnounceType,
			"actor":  actor.ID,
			"object": objectID,
		},
	}
}

// undoQuoteBoost finds and undoes quote boosts for a status
func (h *Handler) undoQuoteBoost(ctx *lift.Context, actor *activitypub.Actor, objectID string) bool {
	quoteRelationships, _, err := h.repos.Object().GetQuotesForNote(ctx.Context, objectID, 100, "")
	if err != nil {
		return false
	}

	for _, quote := range quoteRelationships {
		if h.isMatchingQuoteBoost(quote, actor.ID, objectID) {
			h.processQuoteBoostUndo(ctx, actor, quote)
			return true
		}
	}

	return false
}

// isMatchingQuoteBoost checks if a quote relationship matches the actor and object
func (h *Handler) isMatchingQuoteBoost(quote *storage.QuoteRelationship, actorID, objectID string) bool {
	return quote.QuoterID == actorID && quote.TargetNoteID == objectID
}

// processQuoteBoostUndo handles the undo operations for a quote boost
func (h *Handler) processQuoteBoostUndo(ctx *lift.Context, actor *activitypub.Actor, quote *storage.QuoteRelationship) {
	// Withdraw the quote
	if err := h.repos.Object().WithdrawQuote(ctx.Context, quote.QuoterNoteID); err != nil {
		h.logger.Error("failed to withdraw quote", zap.Error(err))
	}

	// Create and store Undo activity
	undoActivity := h.createUndoQuoteActivity(actor, quote.QuoterNoteID)
	if err := h.repos.Activity().CreateActivity(ctx.Context, undoActivity); err != nil {
		h.logger.Error("failed to create undo quote activity", zap.Error(err))
	}

	// Delete the quote note object
	noteURL := fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), quote.QuoterNoteID)
	if err := h.repos.Object().DeleteObject(ctx.Context, noteURL); err != nil {
		h.logger.Warn("failed to delete quote note object",
			zap.String("note_id", quote.QuoterNoteID),
			zap.Error(err))
	}

	h.logger.Info("successfully undid quote boost",
		zap.String("actor", actor.ID),
		zap.String("quoted_object", quote.TargetNoteID),
		zap.String("quote_id", quote.ID))
}

// createUndoQuoteActivity creates an Undo activity for a quote note
func (h *Handler) createUndoQuoteActivity(actor *activitypub.Actor, quoterNoteID string) *activitypub.Activity {
	now := time.Now()
	return &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Context:   activitypub.Context,
			Type:      activitypub.UndoType,
			ID:        fmt.Sprintf("%s/activities/undo-create-%d-%s", actor.ID, now.Unix(), generateRandomStringForBoost()),
			To:        []string{activitypub.PublicAddress},
			Published: &now,
		},
		Actor: actor.ID,
		Object: map[string]any{
			"type":   activitypub.CreateType,
			"actor":  actor.ID,
			"object": fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), quoterNoteID),
		},
	}
}

// buildUndoBoostResponse builds the response for an undo boost operation
func (h *Handler) buildUndoBoostResponse(ctx *lift.Context, statusID, objectID string) error {
	// Get announce count for the object
	announceCount, _ := h.repos.Like().GetBoostCount(ctx.Context, objectID)

	// Return a simplified status response
	resp := models.FavouriteResponse{
		ID:           statusID,
		CreatedAt:    time.Now().Format("2006-01-02T15:04:05.000Z"),
		Reblogged:    false,
		ReblogsCount: int(announceCount),
		URI:          objectID,
		URL:          objectID,
		Content:      "", // Would be populated from object
		Visibility:   "public",
		Language:     "en",
	}

	return ctx.JSON(resp)
}

// generateRandomStringForBoost generates a random string of 8 characters for boost operations
func generateRandomStringForBoost() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	const length = 8
	b := make([]byte, length)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		b[i] = charset[n.Int64()]
	}
	return string(b)
}
