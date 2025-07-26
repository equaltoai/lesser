package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/moderation"
	"github.com/equaltoai/lesser/pkg/trust"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// HandleModerationFlag handles POST /api/v1/moderation/flag
func (h *Handler) HandleModerationFlag(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract and validate token
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

	// Parse request
	var req models.FlagRequest
	if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
		return common.BadRequest(err), nil
	}

	// Validate request
	if req.ObjectID == "" {
		return common.BadRequest(errors.New("object_id is required")), nil
	}
	if req.Reason == "" {
		return common.BadRequest(errors.New("reason is required")), nil
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
	actor, err := h.store.GetActor(ctx, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return common.InternalServerError(err), nil
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

	// Store the event
	if err := h.store.CreateModerationEvent(ctx, event); err != nil {
		h.logger.Error("failed to create moderation event", zap.Error(err))
		return common.InternalServerError(err), nil
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

	return common.Created(resp), nil
}

// HandleModerationQueue handles GET /api/v1/moderation/queue
func (h *Handler) HandleModerationQueue(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract and validate token
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

	// Check if user is moderator or admin
	user, err := h.store.GetUser(ctx, claims.Username)
	if err != nil || (user.Role != "moderator" && user.Role != "admin") {
		return common.Forbidden(errors.New("requires admin or moderator role")), nil
	}

	// Parse query parameters
	limit := 20
	if limitStr := request.QueryStringParameters["limit"]; limitStr != "" {
		fmt.Sscanf(limitStr, "%d", &limit)
		if limit > 100 {
			limit = 100
		}
	}

	cursor := request.QueryStringParameters["cursor"]

	// Get queue items from storage
	queueItems, nextCursor, err := h.store.GetModerationQueuePaginated(ctx, limit, cursor)
	if err != nil {
		h.logger.Error("failed to get moderation queue", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Convert to response format
	response := make([]models.ReviewQueueItem, len(queueItems))
	for i, item := range queueItems {
		response[i] = models.ReviewQueueItem{
			ID:              item.Event.ID,
			ObjectID:        item.Event.ObjectID,
			ObjectType:      item.Event.ObjectType,
			ObjectPreview:   h.getObjectPreview(ctx, item.Event.ObjectID, item.Event.ObjectType),
			AuthorID:        item.Event.ActorID,
			Category:        string(item.Event.Category),
			Severity:        int(item.Event.Severity),
			ConfidenceScore: item.Event.ConfidenceScore,
			PriorityScore:   item.Priority,
			ReportCount:     len(item.Event.Evidence),
			Status:          "pending", // Default status
			CreatedAt:       item.Event.Created.Format(time.RFC3339),
		}
	}

	// Add pagination header if there's more data
	headers := common.Headers()
	if nextCursor != "" {
		headers["X-Next-Cursor"] = nextCursor
	}

	body, _ := json.Marshal(response)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: 200,
		Headers:    headers,
		Body:       string(body),
	}, nil
}

// HandleModerationReview handles POST /api/v1/moderation/review
func (h *Handler) HandleModerationReview(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract and validate token
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

	// Check if user is moderator or admin
	user, err := h.store.GetUser(ctx, claims.Username)
	if err != nil || (user.Role != "moderator" && user.Role != "admin") {
		return common.Forbidden(errors.New("requires admin or moderator role")), nil
	}

	// Parse request
	var req models.ReviewRequest
	if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
		return common.BadRequest(err), nil
	}

	// Validate request
	if req.EventID == "" {
		return common.BadRequest(errors.New("event_id is required")), nil
	}
	if req.Confidence < 0 || req.Confidence > 1 {
		return common.BadRequest(errors.New("confidence must be between 0 and 1")), nil
	}

	// Get reviewer's actor
	actor, err := h.store.GetActor(ctx, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return common.InternalServerError(err), nil
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

	// Submit review
	if err := h.store.AddModerationReview(ctx, review); err != nil {
		h.logger.Error("failed to add moderation review", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Return response
	resp := map[string]any{
		"review_id":   review.ID,
		"event_id":    review.EventID,
		"action":      string(review.Action),
		"reviewed_at": review.Created.Format(time.RFC3339),
	}

	return common.Created(resp), nil
}

// HandleModerationHistory handles GET /api/v1/moderation/history/:object_id
func (h *Handler) HandleModerationHistory(ctx context.Context, request events.APIGatewayV2HTTPRequest, objectID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract and validate token
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check if user is moderator or admin
	user, err := h.store.GetUser(ctx, claims.Username)
	if err != nil || (user.Role != "moderator" && user.Role != "admin") {
		return common.Forbidden(errors.New("requires admin or moderator role")), nil
	}

	// Get moderation history
	history, err := h.store.GetModerationHistory(ctx, objectID)
	if err != nil {
		h.logger.Error("failed to get moderation history", zap.Error(err))
		return common.InternalServerError(err), nil
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

	return common.OK(resp), nil
}

// HandleGetConsensus handles GET /api/v1/moderation/consensus/:event_id
func (h *Handler) HandleGetConsensus(ctx context.Context, request events.APIGatewayV2HTTPRequest, eventID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract and validate token
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

	// Check if user is moderator or admin
	user, err := h.store.GetUser(ctx, claims.Username)
	if err != nil || (user.Role != "moderator" && user.Role != "admin") {
		return common.Forbidden(errors.New("requires admin or moderator role")), nil
	}

	// Get event
	event, err := h.store.GetModerationEvent(ctx, eventID)
	if err != nil {
		h.logger.Error("failed to get moderation event", zap.Error(err))
		return common.NotFound(errors.New("event not found")), nil
	}

	// Get reviews
	reviews, err := h.store.GetModerationReviews(ctx, eventID)
	if err != nil {
		h.logger.Error("failed to get moderation reviews", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Get decision if exists
	decision, _ := h.store.GetModerationDecision(ctx, event.ObjectID)

	// Convert reviews to response format
	reviewResponses := make([]*models.ConsensusReview, len(reviews))
	for i, review := range reviews {
		// Get reviewer trust score
		trustScore := 0.5 // Default
		score, err := h.store.GetTrustScore(ctx, review.ReviewerID, string(trust.TrustCategoryContent))
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
		Severity:        int(event.Severity),
		ConfidenceScore: event.ConfidenceScore,
		Reviews:         reviewResponses,
		ReviewerCount:   len(reviews),
	}

	if decision != nil {
		resp.ConsensusScore = decision.ConsensusScore
		resp.Decision = string(decision.Action)
		resp.DecidedAt = decision.Decided.Format(time.RFC3339)
	}

	return common.OK(resp), nil
}

// HandleGetTrustRelationships handles GET /api/v1/moderation/trust
func (h *Handler) HandleGetTrustRelationships(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract and validate token
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

	// Get actor
	actor, err := h.store.GetActor(ctx, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Get direction parameter (default to outgoing)
	direction := request.QueryStringParameters["direction"]
	if direction == "" {
		direction = "outgoing"
	}

	// Get relationships based on direction
	var relationships []*trust.TrustRelationship
	var nextCursor string

	switch direction {
	case "outgoing":
		relationships, nextCursor, err = h.store.GetTrustRelationships(ctx, actor.ID, 100, "")
	case "incoming":
		relationships, nextCursor, err = h.store.GetTrustedByRelationships(ctx, actor.ID, 100, "")
	default:
		return common.BadRequest(errors.New("direction must be 'outgoing' or 'incoming'")), nil
	}

	if err != nil {
		h.logger.Error("failed to get trust relationships", zap.Error(err))
		return common.InternalServerError(err), nil
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
	headers := common.Headers()
	if nextCursor != "" {
		headers["X-Next-Cursor"] = nextCursor
	}

	body, _ := json.Marshal(response)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: 200,
		Headers:    headers,
		Body:       string(body),
	}, nil
}

// HandleUpdateTrust handles PUT /api/v1/moderation/trust
func (h *Handler) HandleUpdateTrust(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract and validate token
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

	// Parse request
	var req models.UpdateTrustRequest
	if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
		return common.BadRequest(err), nil
	}

	// Validate request
	if req.TrusteeID == "" {
		return common.BadRequest(errors.New("trustee_id is required")), nil
	}
	if req.Score < -1 || req.Score > 1 {
		return common.BadRequest(errors.New("score must be between -1 and 1")), nil
	}
	if req.Confidence < 0 || req.Confidence > 1 {
		return common.BadRequest(errors.New("confidence must be between 0 and 1")), nil
	}
	if req.Category == "" {
		req.Category = "general"
	}

	// Get truster's actor
	actor, err := h.store.GetActor(ctx, claims.Username)
	if err != nil {
		h.logger.Error("failed to get actor", zap.Error(err))
		return common.InternalServerError(err), nil
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
	existing, err := h.store.GetTrustRelationship(ctx, actor.ID, req.TrusteeID, req.Category)
	if err == nil && existing != nil {
		// Update existing
		relationship.Created = existing.Created
		if err := h.store.UpdateTrustRelationship(ctx, relationship); err != nil {
			h.logger.Error("failed to update trust relationship", zap.Error(err))
			return common.InternalServerError(err), nil
		}
	} else {
		// Create new
		relationship.Created = time.Now()
		if err := h.store.CreateTrustRelationship(ctx, relationship); err != nil {
			h.logger.Error("failed to create trust relationship", zap.Error(err))
			return common.InternalServerError(err), nil
		}
	}

	return common.OK(map[string]any{
		"success": true,
		"message": "Trust relationship updated successfully",
	}), nil
}

// HandleGetTrustScore handles GET /api/v1/moderation/trust/:actor_id/score
func (h *Handler) HandleGetTrustScore(ctx context.Context, request events.APIGatewayV2HTTPRequest, actorID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract and validate token
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check if user is moderator or admin
	user, err := h.store.GetUser(ctx, claims.Username)
	if err != nil || (user.Role != "moderator" && user.Role != "admin") {
		return common.Forbidden(errors.New("requires admin or moderator role")), nil
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
		score, err := h.store.GetTrustScore(ctx, actorID, string(category))
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
	trusters, _, err := h.store.GetTrustedByRelationships(ctx, actorID, 100, "")
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

	return common.OK(resp), nil
}

// Helper methods for moderation
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
