package handlers

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	commonerrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/security/htmlsafe"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/services/quotes"
	"github.com/equaltoai/lesser/pkg/storage"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/transformations"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
	"go.uber.org/zap"
)

const maxUnifiedBoostRequestBodyBytes = 16 * 1024

// HandleUnifiedBoostLift handles both traditional boosts and quote boosts
func (h *Handler) HandleUnifiedBoostLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	statusID := ctx.Param("id")
	if err := common.ValidateRequiredParam("id", statusID); err != nil {
		return common.RespondBadRequest(ctx, "missing status id")
	}

	// Authenticate user with write scope requirement
	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		if isInsufficientScopeError(err) {
			return common.RespondForbidden(ctx, err.Error())
		}
		return common.RespondUnauthorized(ctx)
	}
	username := claims.Username

	req, parseResp, err := h.parseUnifiedBoostRequest(ctx)
	if parseResp != nil || err != nil {
		return parseResp, err
	}

	// Get the user's actor
	actor, err := h.repos.Actor().GetActor(ctx.Context(), username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return common.RespondInternalServerError(ctx)
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
		return h.createQuoteBoostLift(ctx, username, statusID, objectID, *req.Comment, req.Visibility, actor)
	}

	// Traditional boost - create Announce activity
	return h.createPureBoostLift(ctx, statusID, objectID, actor)
}

func (h *Handler) parseUnifiedBoostRequest(ctx *apptheory.Context) (models.ReblogRequest, *apptheory.Response, error) {
	var req models.ReblogRequest
	body := ctx.Request.Body
	if len(body) == 0 {
		return req, nil, nil
	}
	if len(body) > maxUnifiedBoostRequestBodyBytes {
		resp, err := common.RespondBadRequest(ctx, "request body too large")
		return req, resp, err
	}

	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(&req); err != nil {
		resp, respErr := common.RespondBadRequest(ctx, "invalid request body")
		return req, resp, respErr
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		resp, respErr := common.RespondBadRequest(ctx, "invalid request body")
		return req, resp, respErr
	}

	comment, resp, err := h.sanitizeQuoteBoostComment(ctx, req.Comment)
	if resp != nil || err != nil {
		return req, resp, err
	}
	if comment == "" {
		req.Comment = nil
	} else {
		req.Comment = &comment
	}

	return req, nil, nil
}

func (h *Handler) sanitizeQuoteBoostComment(ctx *apptheory.Context, comment *string) (string, *apptheory.Response, error) {
	if comment == nil {
		return "", nil, nil
	}

	sanitized := strings.TrimSpace(htmlsafe.SanitizeHTMLByContract(*comment))
	if sanitized == "" {
		return "", nil, nil
	}
	if err := common.ValidateStatusContent(sanitized); err != nil {
		resp, respErr := common.RespondUnprocessableEntity(ctx, err.Error())
		return "", resp, respErr
	}

	return sanitized, nil, nil
}

// createPureBoostLift creates a traditional ActivityPub Announce
func (h *Handler) createPureBoostLift(ctx *apptheory.Context, statusID, objectID string, actor *activitypub.Actor) (*apptheory.Response, error) {
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
	if err := common.ValidateRequiredParam("followers", actor.Followers); err == nil {
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

	if err := h.repos.Social().CreateAnnounce(ctx.Context(), announce); err != nil {
		h.logger.Error("failed to create announce", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	// Store the activity in the outbox (this will trigger delivery)
	if err := h.repos.Activity().CreateActivity(ctx.Context(), announceActivity); err != nil {
		h.logger.Error("failed to create announce activity", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	// Record engagement for trending
	engagementData := &storage.EngagementData{
		Shares:      1,
		UniqueUsers: 1,
	}
	if err := h.repos.Analytics().RecordEngagement(ctx.Context(), "boost", objectID, time.Now().Format(common.DateFormat), engagementData); err != nil {
		h.logger.Warn("failed to record status engagement",
			zap.String("status_id", statusID),
			zap.String("object_id", objectID),
			zap.Error(err))
	}

	// Get announce count for the object
	announceCount, _ := h.repos.Like().GetBoostCount(ctx.Context(), objectID)

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

	return okJSON(resp)
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
func (h *Handler) createQuoteBoostLift(ctx *apptheory.Context, username, statusID, objectID, comment, visibility string, actor *activitypub.Actor) (*apptheory.Response, error) {
	// Validate and default visibility if not specified
	if err := common.ValidateVisibility(visibility); err != nil {
		return common.RespondUnprocessableEntity(ctx, err.Error())
	}
	if visibility == "" {
		visibility = storageModels.VisibilityPublic
	}

	quoteTarget, failure, err := h.authorizeQuoteBoostTarget(ctx, username, statusID, visibility)
	if failure != nil || err != nil {
		return failure, err
	}
	if quoteTarget.Note != nil && strings.TrimSpace(quoteTarget.Note.ID) != "" {
		objectID = strings.TrimSpace(quoteTarget.Note.ID)
	}

	// Generate note ID
	noteID := fmt.Sprintf("%d-%s", time.Now().Unix(), generateRandomStringForBoost())

	// Create the QuoteNote object with quote content
	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:   fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), noteID),
			Type: "Note",
		},
		Content:      comment,
		AttributedTo: actor.ID,
		QuoteURL:     objectID,
		// Account and per-note quote permissions were authorized above through the same
		// QuoteService.CheckQuotePermissions predicate used by GraphQL quote creation.
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
	if err := h.repos.Object().CreateObject(ctx.Context(), note); err != nil {
		h.logger.Error("failed to create quote note object", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	// Create quote relationship
	quoteRelationship := &storage.QuoteRelationship{
		ID:           fmt.Sprintf("%s-quotes-%s", noteID, statusID),
		QuoterNoteID: noteID,
		TargetNoteID: objectID,
		QuoterID:     actor.ID,
		Timestamp:    now,
	}

	if err := h.repos.Object().CreateQuoteRelationship(ctx.Context(), quoteRelationship); err != nil {
		h.logger.Error("failed to create quote relationship", zap.Error(err))
		// Don't fail the request - the note is already created
	}

	// Increment reblog count on the quoted status (unified counting)
	if err := h.repos.Like().IncrementReblogCount(ctx.Context(), objectID); err != nil {
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
	if err := h.repos.Activity().CreateActivity(ctx.Context(), createActivity); err != nil {
		h.logger.Error("failed to create activity", zap.Error(err))
		return common.RespondInternalServerError(ctx)
	}

	// Fan out the post to timelines
	if err := h.repos.User().FanOutPost(ctx.Context(), createActivity); err != nil {
		h.logger.Error("failed to fan out quote boost to timelines", zap.Error(err))
	}

	// Record engagement for trending
	engagementData := &storage.EngagementData{
		Shares:      1,
		UniqueUsers: 1,
	}
	if err := h.repos.Analytics().RecordEngagement(ctx.Context(), "quote", objectID, time.Now().Format(common.DateFormat), engagementData); err != nil {
		h.logger.Warn("failed to record quote engagement",
			zap.String("quoted_status_id", objectID),
			zap.Error(err))
	}

	// Get the quoted status to include in response
	var quotedStatus *models.Status
	if quotedObj, getErr := h.repos.Object().GetObject(ctx.Context(), objectID); getErr == nil {
		// Properly convert quotedObj to models.Status
		quotedActor, err := h.repos.Actor().GetActor(ctx.Context(), h.extractActorIDFromObject(quotedObj))
		if err == nil {
			status := transformations.ObjectToStatusAny(quotedObj, quotedActor, h.cfg.BaseURL())
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
		Account: func() models.Account {
			// Use centralized transformation framework - ELIMINATES 15+ LINES OF DUPLICATE CODE
			account := transformations.ActorToAccountBase(actor, h.cfg.BaseURL())
			// Override ID to use Actor.ID instead of username for this specific use case
			account.ID = actor.ID
			account.CreatedAt = now.Format("2006-01-02T15:04:05.000Z")
			return account
		}(),
	}

	// Populate avatar from actor Icon
	if actor.Icon != nil && common.ValidateRequiredParam("iconURL", actor.Icon.URL) == nil {
		resp.Account.Avatar = actor.Icon.URL
		resp.Account.AvatarStatic = actor.Icon.URL
	}

	// Populate header from actor Image
	if actor.Image != nil && common.ValidateRequiredParam("imageURL", actor.Image.URL) == nil {
		resp.Account.Header = actor.Image.URL
		resp.Account.HeaderStatic = actor.Image.URL
	}

	return okJSON(resp)
}

func (h *Handler) authorizeQuoteBoostTarget(ctx *apptheory.Context, username, statusID, visibility string) (*storageModels.Status, *apptheory.Response, error) {
	if h.registry == nil {
		h.logger.Error("service registry unavailable while resolving quote target")
		resp, err := common.RespondInternalServerError(ctx)
		return nil, resp, err
	}
	notesService := h.registry.Notes()
	if notesService == nil {
		h.logger.Error("notes service unavailable while resolving quote target")
		resp, err := common.RespondInternalServerError(ctx)
		return nil, resp, err
	}
	quoteTarget, err := notesService.ResolveQuoteTarget(ctx.Context(), username, statusID)
	if err != nil {
		h.logger.Warn("quote target rejected",
			zap.String("status_id", statusID),
			zap.String("viewer", username),
			zap.Error(err))
		if appErr, ok := commonerrors.AsAppError(err); ok {
			resp, responseErr := common.RespondWithAppError(ctx, appErr)
			return nil, resp, responseErr
		}
		resp, responseErr := common.RespondInternalServerError(ctx)
		return nil, resp, responseErr
	}
	if err := notes.ValidateChildReach("quote", quoteTarget, visibility); err != nil {
		if appErr, ok := commonerrors.AsAppError(err); ok {
			resp, responseErr := common.RespondWithAppError(ctx, appErr)
			return nil, resp, responseErr
		}
		resp, responseErr := common.RespondInternalServerError(ctx)
		return nil, resp, responseErr
	}
	if !common.IsPubliclyVisible(quoteTarget.Visibility) {
		resp, responseErr := common.RespondUnprocessableEntity(ctx, restTargetNotQuotable)
		return nil, resp, responseErr
	}
	quotesService := h.registry.Quotes()
	if quotesService == nil {
		h.logger.Error("quotes service unavailable while authorizing quote")
		resp, responseErr := common.RespondInternalServerError(ctx)
		return nil, resp, responseErr
	}
	canQuote, err := quotesService.CheckQuotePermissions(ctx.Context(), username, quoteTarget)
	if err != nil {
		h.logger.Error("failed to check quote permissions",
			zap.String("viewer", username),
			zap.String("target_author", quoteTarget.AuthorUsername),
			zap.Error(err))
		resp, responseErr := common.RespondWithAppError(ctx, quotes.ErrCheckQuotePermissions(err))
		return nil, resp, responseErr
	}
	if !canQuote {
		resp, responseErr := common.RespondWithAppError(ctx, quotes.ErrNotAuthorizedToQuote)
		return nil, resp, responseErr
	}
	return quoteTarget, nil, nil
}

// HandleUndoUnifiedBoostLift handles undoing both traditional boosts and quote boosts
func (h *Handler) HandleUndoUnifiedBoostLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	statusID := ctx.Param("id")
	if err := common.ValidateRequiredParam("id", statusID); err != nil {
		return common.RespondBadRequest(ctx, "missing status id")
	}

	// Authenticate user
	username, resp, err := h.authenticateUndoBoostRequest(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	// Get the user's actor
	actor, err := h.repos.Actor().GetActor(ctx.Context(), username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return common.RespondInternalServerError(ctx)
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
func (h *Handler) authenticateUndoBoostRequest(ctx *apptheory.Context) (string, *apptheory.Response, error) {
	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		if isInsufficientScopeError(err) {
			resp, respErr := common.RespondForbidden(ctx, err.Error())
			return "", resp, respErr
		}
		resp, respErr := common.RespondUnauthorized(ctx)
		return "", resp, respErr
	}

	return claims.Username, nil, nil
}

// normalizeBoostObjectID converts a status ID to a full URL if needed
func (h *Handler) normalizeBoostObjectID(statusID string) string {
	if !strings.HasPrefix(statusID, "http://") && !strings.HasPrefix(statusID, "https://") {
		return fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), statusID)
	}
	return statusID
}

// undoTraditionalBoost finds and undoes a traditional announce/boost
func (h *Handler) undoTraditionalBoost(ctx *apptheory.Context, actor *activitypub.Actor, objectID string) bool {
	announce, err := h.repos.Social().GetAnnounce(ctx.Context(), actor.ID, objectID)
	if err != nil {
		return false
	}

	// Create Undo Announce activity
	undoActivity := h.createUndoAnnounceActivity(actor, objectID)

	// Delete the Announce record
	if err := h.repos.Social().DeleteAnnounce(ctx.Context(), actor.ID, objectID); err != nil {
		h.logger.Error("failed to delete announce", zap.Error(err))
		return false
	}

	// Store the activity in the outbox
	if err := h.repos.Activity().CreateActivity(ctx.Context(), undoActivity); err != nil {
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
func (h *Handler) undoQuoteBoost(ctx *apptheory.Context, actor *activitypub.Actor, objectID string) bool {
	quoteRelationships, _, err := h.repos.Object().GetQuotesForNote(ctx.Context(), objectID, 100, "")
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
func (h *Handler) processQuoteBoostUndo(ctx *apptheory.Context, actor *activitypub.Actor, quote *storage.QuoteRelationship) {
	// Withdraw the quote
	if err := h.repos.Object().WithdrawQuote(ctx.Context(), quote.QuoterNoteID); err != nil {
		h.logger.Error("failed to withdraw quote", zap.Error(err))
	}

	// Create and store Undo activity
	undoActivity := h.createUndoQuoteActivity(actor, quote.QuoterNoteID)
	if err := h.repos.Activity().CreateActivity(ctx.Context(), undoActivity); err != nil {
		h.logger.Error("failed to create undo quote activity", zap.Error(err))
	}

	// Delete the quote note object
	noteURL := fmt.Sprintf("%s/objects/%s", h.cfg.BaseURL(), quote.QuoterNoteID)
	if err := h.repos.Object().DeleteObject(ctx.Context(), noteURL); err != nil {
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
func (h *Handler) buildUndoBoostResponse(ctx *apptheory.Context, statusID, objectID string) (*apptheory.Response, error) {
	// Get announce count for the object
	announceCount, _ := h.repos.Like().GetBoostCount(ctx.Context(), objectID)

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

	return okJSON(resp)
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
