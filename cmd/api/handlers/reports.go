package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/aron23/lesser/cmd/api/models"
	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/moderation"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-lambda-go/events"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// CreateReportRequest represents the request body for creating a report
type CreateReportRequest struct {
	AccountID string   `json:"account_id"`
	StatusIDs []string `json:"status_ids"`
	Comment   string   `json:"comment"`
	Forward   bool     `json:"forward"`
	Category  string   `json:"category"`
	RuleIDs   []int    `json:"rule_ids"`
}

// HandleCreateReport handles POST /api/v1/reports
func (h *Handler) HandleCreateReport(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token
	token, err := auth.ExtractBearerToken(request.Headers["Authorization"])
	if err != nil {
		token, err = auth.ExtractBearerToken(request.Headers["authorization"])
		if err != nil {
			return common.Unauthorized(err), nil
		}
	}

	// Validate token
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check write scope
	if !claims.HasScope(auth.ScopeWrite) {
		return common.Forbidden(errors.New("insufficient scope")), nil
	}

	// Parse request body
	var req CreateReportRequest
	if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
		h.logger.Debug("invalid report request", zap.Error(err))
		return common.BadRequest(errors.New("invalid request")), nil
	}

	// Validate required fields
	if req.AccountID == "" {
		return common.BadRequest(errors.New("account_id is required")), nil
	}

	// Validate category
	if req.Category == "" {
		req.Category = "other"
	} else if req.Category != "spam" && req.Category != "violation" && req.Category != "other" {
		return common.BadRequest(errors.New("invalid category")), nil
	}

	// Create the report
	report := &storage.Report{
		ID:              uuid.New().String(),
		ReporterID:      claims.Username,
		TargetAccountID: req.AccountID,
		StatusIDs:       req.StatusIDs,
		Comment:         req.Comment,
		Category:        req.Category,
		RuleIDs:         req.RuleIDs,
		Forwarded:       req.Forward,
	}

	// Save the report
	if err := h.store.CreateReport(ctx, report); err != nil {
		h.logger.Error("failed to create report", zap.Error(err))
		return common.InternalServerError(fmt.Errorf("failed to create report")), nil
	}

	// Create a moderation event for the report
	now := time.Now()
	moderationEvent := &moderation.ModerationEvent{
		ID:              uuid.New().String(),
		EventType:       moderation.EventTypeFlagged,
		ObjectID:        req.AccountID,
		ObjectType:      "Actor",
		ActorID:         claims.Username,
		Category:        moderation.CategoryOther,
		Severity:        moderation.SeverityMedium,
		ConfidenceScore: 1.0, // User reports have full confidence
		Reason:          req.Comment,
		Created:         now,
		Updated:         now,
	}

	// Map report category to moderation category
	switch req.Category {
	case "spam":
		moderationEvent.Category = moderation.CategorySpam
	case "violation":
		moderationEvent.Category = moderation.CategoryOther
	}

	// If specific statuses were reported, focus on the first one
	if len(req.StatusIDs) > 0 {
		moderationEvent.ObjectType = "Note"
		moderationEvent.ObjectID = req.StatusIDs[0]
	}

	if err := h.store.CreateModerationEvent(ctx, moderationEvent); err != nil {
		h.logger.Error("failed to create moderation event for report", zap.Error(err))
		// Don't fail the request - the report was still created
	} else {
		// Update the report with the moderation event ID
		report.ModerationEventID = moderationEvent.ID
		_ = h.store.UpdateReportStatus(ctx, report.ID, report.Status, "", "")
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
		RuleIDs:       report.RuleIDs,
		TargetAccount: nil, // TODO: Load target account if needed
	}

	return common.OK(response), nil
}
