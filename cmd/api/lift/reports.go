package lift

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/moderation"
	"github.com/equaltoai/lesser/pkg/reports"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/transformations"
	"github.com/google/uuid"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleCreateReportLift handles POST /api/v1/reports
func (h *Handler) HandleCreateReportLift(ctx *lift.Context) error {
	var username string
	var claims *auth.Claims

	// Extract token from Authorization header
	authHeader := ctx.Header("Authorization")
	if err := common.ValidateRequiredParam("authorization", authHeader); err != nil {
		return h.respondUnauthorized(ctx)
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return h.respondUnauthorized(ctx)
	}

	// Validate token and get claims
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err = oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return h.respondUnauthorized(ctx)
	}

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		return h.respondInsufficientScope(ctx)
	}

	username = claims.Username

	// Parse request body
	var req models.CreateReportRequest
	if err := ctx.ParseRequest(&req); err != nil {
		// Fallback for test environment - try parsing directly from request body
		if ctx.Request != nil && ctx.Request.Body != nil && len(ctx.Request.Body) > 0 {
			if jsonErr := json.Unmarshal(ctx.Request.Body, &req); jsonErr != nil {
				h.logger.Debug("invalid report request",
					zap.Error(err),
					zap.Error(jsonErr))
				return h.respondBadRequest(ctx, "invalid request")
			}
		} else {
			h.logger.Debug("invalid report request", zap.Error(err))
			return h.respondBadRequest(ctx, "invalid request")
		}
	}

	// Validate report parameters
	params := map[string]interface{}{
		"account_id": req.AccountID,
		"status_ids": req.StatusIDs,
		"comment":    req.Comment,
		"category":   req.Category,
		"forward":    req.Forward,
	}
	if err := common.ValidateReportParams(params); err != nil {
		h.logger.Info("report validation failed", zap.Error(err))
		return h.respondBadRequest(ctx, err.Error())
	}

	// Validate required fields
	if err := common.ValidateRequiredParam("accountID", req.AccountID); err != nil {
		return h.respondBadRequest(ctx, err.Error())
	}

	// Validate category using common validation function
	if err := common.ValidateReportCategory(req.Category); err != nil {
		// Default to "other" if validation fails
		req.Category = "other"
	}

	// Validate comment if provided
	if req.Comment != "" {
		if err := common.ValidateReportComment(req.Comment); err != nil {
			return h.respondBadRequest(ctx, err.Error())
		}
	}

	// Validate status IDs if provided
	if len(req.StatusIDs) > 0 {
		if err := common.ValidateReportStatusIDs(req.StatusIDs); err != nil {
			return h.respondBadRequest(ctx, err.Error())
		}
	}

	// Create the report
	report := &storage.Report{
		ID:              uuid.New().String(),
		ReporterID:      username,
		TargetAccountID: req.AccountID,
		StatusIDs:       req.StatusIDs,
		Comment:         req.Comment,
		Category:        req.Category,
		RuleIDs:         convertIntArrayToStringArray(req.RuleIDs),
		Forwarded:       req.Forward,
	}

	// Save the report
	if err := h.repos.Moderation().CreateReport(ctx.Context, report); err != nil {
		h.logger.Error("failed to create report", zap.Error(err))
		return h.respondInternalError(ctx, "failed to create report")
	}

	// Get reporter's actor ID for trust integration
	reporterActor, err := h.repos.Actor().GetActor(ctx.Context, username)
	if err != nil {
		h.logger.Warn("failed to get reporter actor", zap.Error(err))
		// Continue with report creation even if actor lookup fails
	}

	reporterActorID := username
	if reporterActor != nil {
		reporterActorID = reporterActor.ID
	}

	// Create enhanced moderation event with trust weighting
	enhancedService := reports.NewEnhancedReportService(h.repos, h.logger)
	moderationEvent, err := enhancedService.CreateEnhancedModerationEvent(ctx.Context, report, reporterActorID)
	if err != nil {
		h.logger.Error("failed to create enhanced moderation event", zap.Error(err))
		// Don't fail the report creation - fall back to basic moderation event
		h.createBasicModerationEventLift(ctx.Context, report, reporterActorID)
	} else {
		// Update the report with the moderation event ID
		report.ModerationEventID = moderationEvent.ID
		_ = h.repos.Moderation().UpdateReportStatus(ctx.Context, report.ID, storage.ReportStatus(report.Status), "", "")

		h.logger.Info("created report with enhanced moderation",
			zap.String("report_id", report.ID),
			zap.String("moderation_event_id", moderationEvent.ID),
			zap.Float64("confidence_score", moderationEvent.ConfidenceScore))
	}

	// Convert to Mastodon API format
	response := &models.Report{
		ID:            report.ID,
		ActionTaken:   false,
		ActionTakenAt: nil,
		Category:      report.Category,
		Comment:       report.Comment,
		Forwarded:     report.Forwarded,
		CreatedAt:     report.CreatedAt.Format(time.RFC3339),
		StatusIDs:     report.StatusIDs,
		RuleIDs:       convertStringArrayToIntArray(report.RuleIDs),
		TargetAccount: h.loadTargetAccountLift(ctx.Context, report.TargetAccountID),
	}

	return ctx.JSON(response)
}

// createBasicModerationEventLift creates a basic moderation event as fallback
func (h *Handler) createBasicModerationEventLift(ctx context.Context, report *storage.Report, actorID string) {
	now := time.Now()
	moderationEvent := &moderation.ModerationEvent{
		ID:              uuid.New().String(),
		EventType:       moderation.EventTypeFlagged,
		ObjectID:        report.TargetAccountID,
		ObjectType:      "Actor",
		ActorID:         actorID,
		Category:        moderation.CategoryOther,
		Severity:        moderation.SeverityMedium,
		ConfidenceScore: 1.0, // User reports have full confidence
		Reason:          report.Comment,
		Created:         now,
		Updated:         now,
	}

	// Map report category to moderation category
	switch report.Category {
	case "spam":
		moderationEvent.Category = moderation.CategorySpam
	case "violation":
		moderationEvent.Category = moderation.CategoryOther
	}

	// If specific statuses were reported, focus on the first one
	if len(report.StatusIDs) > 0 {
		moderationEvent.ObjectType = "Note"
		moderationEvent.ObjectID = report.StatusIDs[0]
	}

	// Convert to storage type
	storageEvent := &storage.ModerationEvent{
		ID:              moderationEvent.ID,
		EventType:       string(moderationEvent.EventType),
		ObjectID:        moderationEvent.ObjectID,
		ObjectType:      moderationEvent.ObjectType,
		ActorID:         moderationEvent.ActorID,
		Category:        string(moderationEvent.Category),
		Severity:        fmt.Sprintf("%d", moderationEvent.Severity),
		ConfidenceScore: moderationEvent.ConfidenceScore,
		Evidence:        []any{},
		Reason:          moderationEvent.Reason,
		Created:         moderationEvent.Created,
		Updated:         moderationEvent.Updated,
		TTL:             moderationEvent.TTL,
	}
	if err := h.repos.Moderation().CreateModerationEvent(ctx, storageEvent); err != nil {
		h.logger.Error("failed to create basic moderation event", zap.Error(err))
	} else {
		// Update the report with the moderation event ID
		report.ModerationEventID = moderationEvent.ID
		_ = h.repos.Moderation().UpdateReportStatus(ctx, report.ID, storage.ReportStatus(report.Status), "", "")
	}
}

// loadTargetAccountLift helper method to load target account for reports
func (h *Handler) loadTargetAccountLift(ctx context.Context, targetAccountID string) *models.Account {
	if err := common.ValidateRequiredParam("targetAccountID", targetAccountID); err != nil {
		return nil
	}

	account, err := h.repos.Account().GetUser(ctx, targetAccountID)
	if err != nil {
		h.logger.Warn("failed to load target account", zap.Error(err))
		return nil
	}

	// Convert to models.Account format using transformation framework - ELIMINATES 5+ LINES OF DUPLICATE CODE
	fakeActor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   account.Username,
			Type: "Person",
		},
		PreferredUsername: account.Username,
		Name:              account.DisplayName,
	}

	apiAccount := transformations.ActorToAccountBase(fakeActor, h.cfg.BaseURL())
	return &apiAccount
}

// convertIntArrayToStringArray converts []int to []string
func convertIntArrayToStringArray(ints []int) []string {
	strings := make([]string, len(ints))
	for i, v := range ints {
		strings[i] = fmt.Sprintf("%d", v)
	}
	return strings
}

// convertStringArrayToIntArray converts []string to []int
func convertStringArrayToIntArray(strings []string) []int {
	ints := make([]int, 0, len(strings))
	for _, s := range strings {
		var i int
		if _, err := fmt.Sscanf(s, "%d", &i); err == nil {
			ints = append(ints, i)
		}
	}
	return ints
}
