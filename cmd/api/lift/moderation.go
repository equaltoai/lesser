package lift

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
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
	if token == "" {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "authentication required",
		})
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
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
	if req.ObjectID == "" {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "object_id is required",
		})
	}
	if req.Reason == "" {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "reason is required",
		})
	}
	if req.Category == "" {
		req.Category = "other"
	}
	if req.Severity < 1 || req.Severity > 4 {
		req.Severity = 2 // Default to medium
	}
	if req.ConfidenceScore <= 0 || req.ConfidenceScore > 1 {
		req.ConfidenceScore = 0.5
	}

	// Get actor ID for the flagger
	actor, err := h.store.GetActor(ctx.Context, claims.Username)
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
	if err := h.store.CreateModerationEvent(ctx.Context, storageEvent); err != nil {
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
	if token == "" {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "authentication required",
		})
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "invalid token",
		})
	}

	// Check if user is moderator or admin
	user, err := h.store.GetUser(ctx.Context, claims.Username)
	if err != nil || (user.Role != "moderator" && user.Role != "admin") {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{
			"error": "requires admin or moderator role",
		})
	}

	// Parse query parameters
	limit := 20
	if limitStr := ctx.Query("limit"); limitStr != "" {
		fmt.Sscanf(limitStr, "%d", &limit)
		if limit > 100 {
			limit = 100
		}
	}

	cursor := ctx.Query("cursor")

	// Get queue items from storage
	queueItems, nextCursor, err := h.store.GetModerationQueuePaginated(ctx.Context, limit, cursor)
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
			Category:        string(item.Event.Category),
			Severity:        parseSeverity(item.Event.Severity),
			ConfidenceScore: item.Event.ConfidenceScore,
			PriorityScore:   item.Priority,
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
	if token == "" {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "authentication required",
		})
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "invalid token",
		})
	}

	// Check if user is moderator or admin
	user, err := h.store.GetUser(ctx.Context, claims.Username)
	if err != nil || (user.Role != "moderator" && user.Role != "admin") {
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
	if req.EventID == "" {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "event_id is required",
		})
	}
	if req.Confidence < 0 || req.Confidence > 1 {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "confidence must be between 0 and 1",
		})
	}

	// Get reviewer's actor
	actor, err := h.store.GetActor(ctx.Context, claims.Username)
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
		Category:   moderation.Category(req.Category),
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
	if err := h.store.AddModerationReview(ctx.Context, storageReview); err != nil {
		h.logger.Error("failed to add moderation review", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "internal server error",
		})
	}

	// Return response
	resp := map[string]any{
		"review_id":   review.ID,
		"event_id":    review.EventID,
		"action":      string(review.Action),
		"reviewed_at": review.Created.Format(time.RFC3339),
	}

	ctx.Status(http.StatusCreated)
	return ctx.JSON(resp)
}

// HandleModerationHistoryLift handles GET /api/v1/moderation/history/:object_id
func (h *Handler) HandleModerationHistoryLift(ctx *lift.Context) error {
	// Extract and validate token
	token := h.getBearerTokenLift(ctx)
	if token == "" {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "authentication required",
		})
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "invalid token",
		})
	}

	// Check if user is moderator or admin
	user, err := h.store.GetUser(ctx.Context, claims.Username)
	if err != nil || (user.Role != "moderator" && user.Role != "admin") {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{
			"error": "requires admin or moderator role",
		})
	}

	// Get object ID from path parameter
	objectID := ctx.Param("object_id")
	if objectID == "" {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "object_id is required",
		})
	}

	// Get moderation history
	history, err := h.store.GetModerationHistory(ctx.Context, objectID)
	if err != nil {
		h.logger.Error("failed to get moderation history", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "internal server error",
		})
	}

	// Convert to response format
	timeline := make([]map[string]any, 0)
	for _, event := range history.Events {
		timeline = append(timeline, map[string]any{
			"timestamp": event.Created.Format(time.RFC3339),
			"type":      "event",
			"event":     event,
		})
	}
	for _, decision := range history.Decisions {
		timeline = append(timeline, map[string]any{
			"timestamp": decision.Decided.Format(time.RFC3339),
			"type":      "decision",
			"decision":  decision,
		})
	}

	resp := map[string]any{
		"object_id":      objectID,
		"events":         history.Events,
		"timeline":       timeline,
		"current_status": history.CurrentStatus,
		"last_updated":   time.Now().Format(time.RFC3339), // Use current time as last updated
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(resp)
}

// HandleGetConsensusLift handles GET /api/v1/moderation/consensus/:event_id
func (h *Handler) HandleGetConsensusLift(ctx *lift.Context) error {
	// Extract and validate token
	token := h.getBearerTokenLift(ctx)
	if token == "" {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "authentication required",
		})
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "invalid token",
		})
	}

	// Check if user is moderator or admin
	user, err := h.store.GetUser(ctx.Context, claims.Username)
	if err != nil || (user.Role != "moderator" && user.Role != "admin") {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{
			"error": "requires admin or moderator role",
		})
	}

	// Get event ID from path parameter
	eventID := ctx.Param("event_id")
	if eventID == "" {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "event_id is required",
		})
	}

	// Get event
	event, err := h.store.GetModerationEvent(ctx.Context, eventID)
	if err != nil {
		h.logger.Error("failed to get moderation event", zap.Error(err))
		ctx.Status(http.StatusNotFound)
		return ctx.JSON(map[string]string{
			"error": "event not found",
		})
	}

	// Get reviews
	reviews, err := h.store.GetModerationReviews(ctx.Context, eventID)
	if err != nil {
		h.logger.Error("failed to get moderation reviews", zap.Error(err))
		ctx.Status(http.StatusInternalServerError)
		return ctx.JSON(map[string]string{
			"error": "internal server error",
		})
	}

	// Get decision if exists
	decision, _ := h.store.GetModerationDecision(ctx.Context, event.ObjectID)

	// Convert reviews to response format
	reviewResponses := make([]*models.ConsensusReview, len(reviews))
	for i, review := range reviews {
		// Get reviewer trust score
		trustScore := 0.5 // Default
		score, err := h.store.GetTrustScore(ctx.Context, review.ReviewerID, string(trust.TrustCategoryContent))
		if err == nil && score != nil {
			trustScore = score.Score
		}

		reviewResponses[i] = &models.ConsensusReview{
			ReviewerID:     review.ReviewerID,
			ReviewerDomain: h.extractDomainFromActor(review.ReviewerID),
			Action:         string(review.Action),
			Confidence:     review.Confidence,
			TrustWeight:    trustScore,
			ReviewedAt:     review.Created.Format(time.RFC3339),
		}
	}

	// Build response
	resp := models.ConsensusVisualization{
		EventID:         event.ID,
		ObjectID:        event.ObjectID,
		Category:        string(event.Category),
		Severity:        parseSeverity(event.Severity),
		ConfidenceScore: event.ConfidenceScore,
		Reviews:         reviewResponses,
		ReviewerCount:   len(reviews),
	}

	if decision != nil {
		resp.ConsensusScore = decision.ConsensusScore
		resp.Decision = string(decision.Action)
		resp.DecidedAt = decision.Decided.Format(time.RFC3339)
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(resp)
}

// HandleGetTrustRelationshipsLift handles GET /api/v1/moderation/trust
func (h *Handler) HandleGetTrustRelationshipsLift(ctx *lift.Context) error {
	// Extract and validate token
	token := h.getBearerTokenLift(ctx)
	if token == "" {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "authentication required",
		})
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "invalid token",
		})
	}

	// Get actor
	actor, err := h.store.GetActor(ctx.Context, claims.Username)
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
	}

	// Get relationships based on direction
	var relationships []*trust.TrustRelationship
	var nextCursor string

	switch direction {
	case "outgoing":
		relationships, nextCursor, err = h.store.GetTrustRelationships(ctx.Context, actor.ID, 100, "")
	case "incoming":
		relationships, nextCursor, err = h.store.GetTrustedByRelationships(ctx.Context, actor.ID, 100, "")
	default:
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "direction must be 'outgoing' or 'incoming'",
		})
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
	if token == "" {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "authentication required",
		})
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
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
	if req.TrusteeID == "" {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "trustee_id is required",
		})
	}
	if req.Score < -1 || req.Score > 1 {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "score must be between -1 and 1",
		})
	}
	if req.Confidence < 0 || req.Confidence > 1 {
		ctx.Status(http.StatusBadRequest)
		return ctx.JSON(map[string]string{
			"error": "confidence must be between 0 and 1",
		})
	}
	if req.Category == "" {
		req.Category = "general"
	}

	// Get truster's actor
	actor, err := h.store.GetActor(ctx.Context, claims.Username)
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
	existing, err := h.store.GetTrustRelationship(ctx.Context, actor.ID, req.TrusteeID, req.Category)
	if err == nil && existing != nil {
		// Update existing
		relationship.Created = existing.Created
		if err := h.store.UpdateTrustRelationship(ctx.Context, relationship); err != nil {
			h.logger.Error("failed to update trust relationship", zap.Error(err))
			ctx.Status(http.StatusInternalServerError)
			return ctx.JSON(map[string]string{
				"error": "internal server error",
			})
		}
	} else {
		// Create new
		relationship.Created = time.Now()
		if err := h.store.CreateTrustRelationship(ctx.Context, relationship); err != nil {
			h.logger.Error("failed to create trust relationship", zap.Error(err))
			ctx.Status(http.StatusInternalServerError)
			return ctx.JSON(map[string]string{
				"error": "internal server error",
			})
		}
	}

	ctx.Status(http.StatusOK)
	return ctx.JSON(map[string]any{
		"success": true,
		"message": "Trust relationship updated successfully",
	})
}

// HandleGetTrustScoreLift handles GET /api/v1/moderation/trust/:actor_id/score
func (h *Handler) HandleGetTrustScoreLift(ctx *lift.Context) error {
	// Extract and validate token
	token := h.getBearerTokenLift(ctx)
	if token == "" {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "authentication required",
		})
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		ctx.Status(http.StatusUnauthorized)
		return ctx.JSON(map[string]string{
			"error": "invalid token",
		})
	}

	// Check if user is moderator or admin
	user, err := h.store.GetUser(ctx.Context, claims.Username)
	if err != nil || (user.Role != "moderator" && user.Role != "admin") {
		ctx.Status(http.StatusForbidden)
		return ctx.JSON(map[string]string{
			"error": "requires admin or moderator role",
		})
	}

	// Get actor ID from path parameter
	actorID := ctx.Param("actor_id")
	if actorID == "" {
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
		score, err := h.store.GetTrustScore(ctx.Context, actorID, string(category))
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
	trusters, _, err := h.store.GetTrustedByRelationships(ctx.Context, actorID, 100, "")
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
		statusInterface, err := h.store.GetStatus(ctx, objectID)
		if err != nil {
			return ""
		}

		// Handle any type with safe type assertion
		if statusMap, ok := statusInterface.(map[string]any); ok {
			if content, ok := statusMap["Content"].(string); ok {
				if len(content) > 100 {
					return content[:100] + "..."
				}
				return content
			}
		}
		return ""
	case "account":
		user, err := h.store.GetUser(ctx, objectID)
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