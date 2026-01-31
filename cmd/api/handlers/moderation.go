package lift

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/moderation"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/trust"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// convertEvidenceToAny converts moderation.Evidence to []any
func convertEvidenceToAny(evidence []moderation.Evidence) []any {
	result := make([]any, len(evidence))
	for i, e := range evidence {
		result[i] = map[string]any{
			"type":        e.Type,
			"score":       e.Score,
			"description": e.Description,
			"metadata":    e.Metadata,
			"timestamp":   e.Timestamp,
		}
	}
	return result
}

// parseSeverity converts string severity to int
func parseSeverity(severity string) int {
	switch severity {
	case "1":
		return 1
	case "2":
		return 2
	case "3":
		return 3
	case "4":
		return 4
	default:
		return 2 // Default to medium
	}
}

// HandleModerationFlagLift handles POST /api/v1/moderation/flag
func (h *Handler) HandleModerationFlagLift(ctx *lift.Context) error {
	// Extract and validate token
	token := h.getBearerTokenLift(ctx)
	if err := common.ValidateRequiredParam("token", token); err != nil {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "authentication required",
		})
	}

	// Validate token and get claims
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "invalid token",
		})
	}

	// Parse request
	var req models.FlagRequest
	if err := ctx.ParseRequest(&req); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": err.Error(),
		})
	}

	// Validate request
	if err := common.ValidateRequiredParam("object_id", req.ObjectID); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": err.Error(),
		})
	}
	if err := common.ValidateRequiredParam("reason", req.Reason); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": err.Error(),
		})
	}
	if common.ValidateRequiredParam("category", req.Category) != nil {
		req.Category = moderationCategoryOther
	}
	if req.Severity < 1 || req.Severity > 4 {
		req.Severity = 2 // Default to medium
	}
	if req.ConfidenceScore <= 0 || req.ConfidenceScore > 1 {
		req.ConfidenceScore = 0.5
	}

	// Get actor ID for the flagger
	actor, err := h.repos.Actor().GetActor(ctx.Context, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "internal server error",
		})
	}

	// Create moderation event
	event := &moderation.ModerationEvent{
		ID:              fmt.Sprintf("mod-%s-%d", req.ObjectID, time.Now().Unix()),
		EventType:       moderation.EventTypeFlagged,
		ObjectID:        req.ObjectID,
		ObjectType:      req.ObjectType,
		Category:        moderation.Category(req.Category),
		Severity:        moderation.Severity(req.Severity),
		ConfidenceScore: req.ConfidenceScore,
		ActorID:         actor.ID,
		Evidence: []moderation.Evidence{{
			Type:        "user_report",
			Score:       req.ConfidenceScore,
			Description: req.Reason,
			Timestamp:   time.Now(),
		}},
		Reason:  req.Reason,
		Created: time.Now(),
		Updated: time.Now(),
	}

	// Convert to storage type and store the event
	storageEvent := &storage.ModerationEvent{
		ID:              event.ID,
		EventType:       string(event.EventType),
		ObjectID:        event.ObjectID,
		ObjectType:      event.ObjectType,
		ActorID:         event.ActorID,
		Category:        string(event.Category),
		Severity:        fmt.Sprintf("%d", event.Severity),
		ConfidenceScore: event.ConfidenceScore,
		Evidence:        convertEvidenceToAny(event.Evidence),
		Reason:          event.Reason,
		Created:         event.Created,
		Updated:         event.Updated,
		TTL:             event.TTL,
	}
	if err := h.repos.Moderation().CreateModerationEvent(ctx.Context, storageEvent); err != nil {
		h.logger.Error("failed to create moderation event", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "internal server error",
		})
	}

	// Convert to response format
	resp := models.ModerationEventResponse{
		ID:              event.ID,
		EventType:       string(event.EventType),
		ObjectID:        event.ObjectID,
		ObjectType:      event.ObjectType,
		Category:        string(event.Category),
		Severity:        int(event.Severity),
		ConfidenceScore: event.ConfidenceScore,
		Status:          "pending", // Default status for new events
		CreatedAt:       event.Created.Format(time.RFC3339),
	}

	ctx.Status(http.StatusCreated)
	return ctx.JSON(resp)
}

// HandleModerationQueueLift handles GET /api/v1/moderation/queue
func (h *Handler) HandleModerationQueueLift(ctx *lift.Context) error {
	// Extract and validate token
	token := h.getBearerTokenLift(ctx)
	if err := common.ValidateRequiredParam("token", token); err != nil {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "authentication required",
		})
	}

	// Validate token and get claims
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "invalid token",
		})
	}

	// Check if user is moderator or admin
	user, err := h.repos.Account().GetUser(ctx.Context, claims.Username)
	if err != nil || (user.Role != roleModerator && user.Role != roleAdmin) {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{
			"error": "requires admin or moderator role",
		})
	}

	// Parse query parameters
	limit, err := common.ParseAndValidateAPILimit(ctx.Query("limit"), 100)
	if err != nil {
		limit = 20 // Use default on error
	}

	cursor := ctx.Query("cursor")

	// Get queue items from storage
	queueItems, nextCursor, err := h.repos.Moderation().GetModerationQueuePaginated(ctx.Context, limit, cursor)
	if err != nil {
		h.logger.Error("failed to get moderation queue", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "internal server error",
		})
	}

	// Convert to response format
	response := make([]models.ReviewQueueItem, len(queueItems))
	for i, item := range queueItems {
		response[i] = models.ReviewQueueItem{
			ID:              item.Event.ID,
			ObjectID:        item.Event.ObjectID,
			ObjectType:      item.Event.ObjectType,
			ObjectPreview:   h.getObjectPreview(ctx.Context, item.Event.ObjectID, item.Event.ObjectType),
			AuthorID:        item.Event.ActorID,
			Category:        item.Event.Category,
			Severity:        parseSeverity(item.Event.Severity),
			ConfidenceScore: item.Event.ConfidenceScore,
			PriorityScore:   float64(item.Priority),
			ReportCount:     len(item.Event.Evidence),
			Status:          "pending", // Default status
			CreatedAt:       item.Event.Created.Format(time.RFC3339),
		}
	}

	// Add pagination header if there's more data
	if nextCursor != "" {
		ctx.Response.Header("X-Next-Cursor", nextCursor)
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(response)
}

// HandleModerationReviewLift handles POST /api/v1/moderation/review
func (h *Handler) HandleModerationReviewLift(ctx *lift.Context) error {
	// Extract and validate token
	token := h.getBearerTokenLift(ctx)
	if err := common.ValidateRequiredParam("token", token); err != nil {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "authentication required",
		})
	}

	// Validate token and get claims
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "invalid token",
		})
	}

	// Check if user is moderator or admin
	user, err := h.repos.Account().GetUser(ctx.Context, claims.Username)
	if err != nil || (user.Role != roleModerator && user.Role != roleAdmin) {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{
			"error": "requires admin or moderator role",
		})
	}

	// Parse request
	var req models.ReviewRequest
	if err := ctx.ParseRequest(&req); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": err.Error(),
		})
	}

	// Validate request
	if err := common.ValidateRequiredParam("event_id", req.EventID); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": err.Error(),
		})
	}
	if err := common.ValidateFloatRange("confidence", req.Confidence, 0, 1); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": err.Error(),
		})
	}

	// Get reviewer's actor
	actor, err := h.repos.Actor().GetActor(ctx.Context, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "internal server error",
		})
	}

	// Create review
	review := &moderation.Review{
		ID:         fmt.Sprintf("review-%s-%s-%d", req.EventID, claims.Username, time.Now().Unix()),
		EventID:    req.EventID,
		ReviewerID: actor.ID,
		Action:     moderation.ActionType(req.Action),
		Severity:   moderation.Severity(req.Severity),
		Confidence: req.Confidence,
		Notes:      req.Notes,
		Created:    time.Now(),
	}

	// Convert to storage type and submit review
	storageReview := &storage.ModerationReview{
		ID:          review.ID,
		EventID:     review.EventID,
		ReviewerID:  review.ReviewerID,
		ReviewerRep: 0.5, // Default reputation
		Action:      string(review.Action),
		Severity:    fmt.Sprintf("%d", review.Severity),
		Note:        review.Notes,
		Tags:        []string{},
		Metadata:    map[string]interface{}{},
		Confidence:  review.Confidence,
		Created:     review.Created,
	}
	if err := h.repos.Moderation().AddModerationReview(ctx.Context, storageReview); err != nil {
		h.logger.Error("failed to add moderation review", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "internal server error",
		})
	}

	// Return response
	resp := models.ModerationReviewResponse{
		ReviewID:   review.ID,
		EventID:    review.EventID,
		Action:     string(review.Action),
		ReviewedAt: review.Created.Format(time.RFC3339),
	}

	ctx.Status(http.StatusCreated)
	return ctx.JSON(resp)
}

// HandleModerationHistoryLift handles GET /api/v1/moderation/history/:object_id
func (h *Handler) HandleModerationHistoryLift(ctx *lift.Context) error {
	// Extract and validate token
	token := h.getBearerTokenLift(ctx)
	if err := common.ValidateRequiredParam("token", token); err != nil {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "authentication required",
		})
	}

	// Validate token
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "invalid token",
		})
	}

	// Check if user is moderator or admin
	user, err := h.repos.Account().GetUser(ctx.Context, claims.Username)
	if err != nil || (user.Role != roleModerator && user.Role != roleAdmin) {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{
			"error": "requires admin or moderator role",
		})
	}

	// Get object ID from path parameter
	objectID := ctx.Param("object_id")
	if err := common.ValidateRequiredParam("object_id", objectID); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": err.Error(),
		})
	}

	// Get moderation history
	history, err := h.repos.Moderation().GetModerationHistory(ctx.Context, objectID)
	if err != nil {
		h.logger.Error("failed to get moderation history", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "internal server error",
		})
	}

	// Convert to response format
	timeline := make([]models.ModerationHistoryTimelineEntry, 0, len(history.Events)+len(history.Decisions))
	for i := range history.Events {
		timeline = append(timeline, models.ModerationHistoryTimelineEntry{
			Timestamp: history.Events[i].Created.Format(time.RFC3339),
			Type:      "event",
			Event:     &history.Events[i],
		})
	}
	for i := range history.Decisions {
		timeline = append(timeline, models.ModerationHistoryTimelineEntry{
			Timestamp: history.Decisions[i].Decided.Format(time.RFC3339),
			Type:      "decision",
			Decision:  &history.Decisions[i],
		})
	}

	resp := models.ModerationHistoryResponse{
		ObjectID:      objectID,
		Events:        history.Events,
		Decisions:     history.Decisions,
		Timeline:      timeline,
		CurrentStatus: history.CurrentStatus,
		LastUpdated:   time.Now().Format(time.RFC3339),
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(resp)
}

// HandleGetConsensusLift handles GET /api/v1/moderation/consensus/:event_id
func (h *Handler) HandleGetConsensusLift(ctx *lift.Context) error {
	// Extract and validate token
	token := h.getBearerTokenLift(ctx)
	if err := common.ValidateRequiredParam("token", token); err != nil {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "authentication required",
		})
	}

	// Validate token and get claims
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "invalid token",
		})
	}

	// Check if user is moderator or admin
	user, err := h.repos.Account().GetUser(ctx.Context, claims.Username)
	if err != nil || (user.Role != roleModerator && user.Role != roleAdmin) {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{
			"error": "requires admin or moderator role",
		})
	}

	// Get event ID from path parameter
	eventID := ctx.Param("event_id")
	if err := common.ValidateRequiredParam("eventID", eventID); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "event_id is required",
		})
	}

	// Get event
	event, err := h.repos.Moderation().GetModerationEvent(ctx.Context, eventID)
	if err != nil {
		h.logger.Error("failed to get moderation event", zap.Error(err))
		ctx.Status(http.StatusNotFound)
		return ctx.JSON(map[string]string{
			"error": "event not found",
		})
	}

	// Get reviews
	reviews, err := h.repos.Moderation().GetModerationReviews(ctx.Context, eventID)
	if err != nil {
		h.logger.Error("failed to get moderation reviews", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "internal server error",
		})
	}

	// Get decision if exists
	decision, _ := h.repos.Moderation().GetModerationDecision(ctx.Context, event.ObjectID)

	// Convert reviews to response format
	reviewResponses := make([]*models.ConsensusReview, len(reviews))
	for i, review := range reviews {
		// Get reviewer trust score
		trustScore := 0.5 // Default
		score, err := h.repos.Trust().GetTrustScore(ctx.Context, review.ReviewerID, string(trust.TrustCategoryContent))
		if err == nil && score != nil {
			trustScore = score.Score
		}

		reviewResponses[i] = &models.ConsensusReview{
			ReviewerID:     review.ReviewerID,
			ReviewerDomain: h.extractDomainFromActor(review.ReviewerID),
			Action:         review.Action,
			Confidence:     review.Confidence,
			TrustWeight:    trustScore,
			ReviewedAt:     review.Created.Format(time.RFC3339),
		}
	}

	// Build response
	resp := models.ConsensusVisualization{
		EventID:         event.ID,
		ObjectID:        event.ObjectID,
		Category:        event.Category,
		Severity:        parseSeverity(event.Severity),
		ConfidenceScore: event.ConfidenceScore,
		Reviews:         reviewResponses,
		ReviewerCount:   len(reviews),
	}

	if decision != nil {
		resp.ConsensusScore = decision.ConsensusScore
		resp.Decision = decision.Action
		resp.DecidedAt = decision.Decided.Format(time.RFC3339)
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(resp)
}

// HandleGetTrustRelationshipsLift handles GET /api/v1/moderation/trust
func (h *Handler) HandleGetTrustRelationshipsLift(ctx *lift.Context) error {
	// Extract and validate token
	token := h.getBearerTokenLift(ctx)
	if err := common.ValidateRequiredParam("token", token); err != nil {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "authentication required",
		})
	}

	// Validate token and get claims
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "invalid token",
		})
	}

	// Get actor
	actor, err := h.repos.Actor().GetActor(ctx.Context, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "internal server error",
		})
	}

	// Get direction parameter (default to outgoing)
	direction := ctx.Query("direction")
	if direction == "" {
		direction = "outgoing"
	} else if err := common.ValidateEnum("direction", direction, []string{"outgoing", "incoming"}); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "direction must be 'outgoing' or 'incoming'",
		})
	}

	// Get relationships based on direction
	var relationships []*trust.TrustRelationship
	var nextCursor string

	switch direction {
	case "outgoing":
		relationships, nextCursor, err = h.repos.Trust().GetTrustRelationships(ctx.Context, actor.ID, 100, "")
	case "incoming":
		relationships, nextCursor, err = h.repos.Trust().GetTrustedByRelationships(ctx.Context, actor.ID, 100, "")
	}

	if err != nil {
		h.logger.Error("failed to get trust relationships", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "internal server error",
		})
	}

	// Convert to response format
	response := make([]models.TrustRelationshipResponse, len(relationships))
	for i, rel := range relationships {
		response[i] = models.TrustRelationshipResponse{
			ID:            rel.ID,
			TrusteeID:     rel.TrusteeID,
			TrusteeDomain: h.extractDomainFromActor(rel.TrusteeID),
			Category:      string(rel.Category),
			Score:         rel.Score,
			Confidence:    rel.Confidence,
			UpdatedAt:     rel.Updated.Format(time.RFC3339),
		}
	}

	// Add pagination header if there's more data
	if nextCursor != "" {
		ctx.Response.Header("X-Next-Cursor", nextCursor)
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(response)
}

// HandleUpdateTrustLift handles PUT /api/v1/moderation/trust
func (h *Handler) HandleUpdateTrustLift(ctx *lift.Context) error {
	// Extract and validate token
	token := h.getBearerTokenLift(ctx)
	if err := common.ValidateRequiredParam("token", token); err != nil {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "authentication required",
		})
	}

	// Validate token and get claims
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "invalid token",
		})
	}

	// Parse request
	var req models.UpdateTrustRequest
	if err := ctx.ParseRequest(&req); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": err.Error(),
		})
	}

	// Validate request
	if err := common.ValidateRequiredParam("trusteeID", req.TrusteeID); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "trustee_id is required",
		})
	}
	if err := common.ValidateFloatRange("score", req.Score, -1, 1); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": err.Error(),
		})
	}
	if err := common.ValidateFloatRange("confidence", req.Confidence, 0, 1); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": err.Error(),
		})
	}
	if common.ValidateRequiredParam("category", req.Category) != nil {
		req.Category = moderationCategoryGeneral
	}

	// Get truster's actor
	actor, err := h.repos.Actor().GetActor(ctx.Context, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "internal server error",
		})
	}

	// Create or update trust relationship
	relationship := &trust.TrustRelationship{
		ID:         fmt.Sprintf("%s-%s-%s", actor.ID, req.TrusteeID, req.Category),
		TrusterID:  actor.ID,
		TrusteeID:  req.TrusteeID,
		Category:   trust.TrustCategory(req.Category),
		Score:      req.Score,
		Confidence: req.Confidence,
		Evidence: []trust.TrustEvidence{{
			Type:        "manual_update",
			Score:       req.Score,
			Description: "Manual trust update",
			Timestamp:   time.Now(),
		}},
		Updated: time.Now(),
		TTL:     time.Now().Add(30 * 24 * time.Hour).Unix(), // 30 days
	}

	// Check if relationship exists
	existing, err := h.repos.Trust().GetTrustRelationship(ctx.Context, actor.ID, req.TrusteeID, req.Category)
	if err == nil && existing != nil {
		// Update existing
		relationship.Created = existing.Created
		if err := h.repos.Trust().UpdateTrustRelationship(ctx.Context, relationship); err != nil {
			h.logger.Error("failed to update trust relationship", zap.Error(err))
			ctx.Status(http.StatusInternalServerError)
			return ctx.JSON(map[string]string{
				"error": "internal server error",
			})
		}
	} else {
		// Create new
		relationship.Created = time.Now()
		if err := h.repos.Trust().CreateTrustRelationship(ctx.Context, relationship); err != nil {
			h.logger.Error("failed to create trust relationship", zap.Error(err))
			ctx.Status(http.StatusInternalServerError)
			return ctx.JSON(map[string]string{
				"error": "internal server error",
			})
		}
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(models.SuccessResponse{
		Success: true,
		Message: "Trust relationship updated successfully",
	})
}

// HandleGetTrustScoreLift handles GET /api/v1/moderation/trust/:actor_id/score
func (h *Handler) HandleGetTrustScoreLift(ctx *lift.Context) error {
	// Extract and validate token
	token := h.getBearerTokenLift(ctx)
	if err := common.ValidateRequiredParam("token", token); err != nil {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "authentication required",
		})
	}

	// Validate token
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "invalid token",
		})
	}

	// Check if user is moderator or admin
	user, err := h.repos.Account().GetUser(ctx.Context, claims.Username)
	if err != nil || (user.Role != roleModerator && user.Role != roleAdmin) {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{
			"error": "requires admin or moderator role",
		})
	}

	// Get actor ID from path parameter
	actorID := ctx.Param("actor_id")
	if err := common.ValidateRequiredParam("actorID", actorID); err != nil {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "actor_id is required",
		})
	}

	// Clean up actor ID (remove @ prefix if present)
	actorID = strings.TrimPrefix(actorID, "@")

	// Get scores for all categories
	categories := []trust.TrustCategory{
		trust.TrustCategoryContent,
		trust.TrustCategoryBehavior,
		trust.TrustCategoryTechnical,
		trust.TrustCategoryGeneral,
	}

	scores := make(map[string]float64)
	overallScore := 0.0
	validScores := 0

	for _, category := range categories {
		score, err := h.repos.Trust().GetTrustScore(ctx.Context, actorID, string(category))
		if err == nil && score != nil {
			scores[string(category)] = score.Score
			overallScore += score.Score
			validScores++
		} else {
			scores[string(category)] = 0.5 // Default neutral score
			overallScore += 0.5
			validScores++
		}
	}

	if validScores > 0 {
		overallScore /= float64(validScores)
	}

	// Get number of trusters
	trusters, _, err := h.repos.Trust().GetTrustedByRelationships(ctx.Context, actorID, 100, "")
	trusterCount := 0
	if err == nil {
		trusterCount = len(trusters)
	}

	// Extract domain from actor ID
	actorDomain := ""
	if parts := strings.Split(actorID, "@"); len(parts) > 1 {
		actorDomain = parts[len(parts)-1]
	}

	resp := models.TrustScoreResponse{
		ActorID:      actorID,
		ActorDomain:  actorDomain,
		OverallScore: overallScore,
		Scores:       scores,
		TrusterCount: trusterCount,
		CalculatedAt: time.Now().Format(time.RFC3339),
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(resp)
}

// Helper methods for moderation

// getObjectPreview returns a preview of the object for display in the moderation queue
func (h *Handler) getObjectPreview(ctx context.Context, objectID, objectType string) string {
	switch objectType {
	case "status":
		status, err := h.repos.Status().GetStatus(ctx, objectID)
		if err != nil {
			return ""
		}

		// status is *models.Status, so we can access fields directly
		if status != nil && status.Content != "" {
			if len(status.Content) > 100 {
				return status.Content[:100] + "..."
			}
			return status.Content
		}
		return ""
	case "account":
		user, err := h.repos.Account().GetUser(ctx, objectID)
		if err != nil {
			return ""
		}
		return fmt.Sprintf("@%s - %s", user.Username, user.DisplayName)
	default:
		return ""
	}
}

// extractDomainFromActor extracts the domain from an actor ID
func (h *Handler) extractDomainFromActor(actorID string) string {
	if strings.Contains(actorID, "://") {
		parsed, err := url.Parse(actorID)
		if err != nil {
			return ""
		}
		return parsed.Host
	}

	// If actorID is just a username, assume local domain
	return h.cfg.Domain
}
