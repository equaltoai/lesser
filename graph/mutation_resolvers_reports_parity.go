package graph

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/reports"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// CreateReport covers POST /api/v1/reports.
func (r *mutationResolver) CreateReport(ctx context.Context, input model.CreateReportInput) (*model.Report, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	accountID := strings.TrimSpace(input.AccountID)
	if err := common.ValidateRequiredParam("accountId", accountID); err != nil {
		return nil, err
	}

	if r.Storage == nil || r.Storage.Moderation() == nil {
		return nil, ErrModerationRepositoryUnavailable
	}

	category := normalizeReportCategory(input.Category)
	comment, err := normalizeReportComment(input.Comment)
	if err != nil {
		return nil, err
	}

	statusIDs, err := normalizeReportStatusIDs(input.StatusIds)
	if err != nil {
		return nil, err
	}

	ruleIDs, ruleIDsOut := convertRuleIDs(input.RuleIds)
	forwarded := boolOrFalse(input.Forward)

	report := &storage.Report{
		ID:              uuid.New().String(),
		ReporterID:      username,
		TargetAccountID: accountID,
		StatusIDs:       statusIDs,
		Comment:         comment,
		Category:        category,
		RuleIDs:         ruleIDs,
		Forwarded:       forwarded,
	}

	if err := r.Storage.Moderation().CreateReport(ctx, report); err != nil {
		r.Logger.Error("failed to create report", zap.Error(err))
		return nil, errors.Join(errors.New("failed to create report"), err)
	}

	reporterActorID := r.resolveReporterActorID(ctx, username)
	r.maybeCreateModerationEvent(ctx, report, reporterActorID)

	targetActor := r.resolveActorByID(ctx, accountID)

	return &model.Report{
		ID:            report.ID,
		ActionTaken:   false,
		ActionTakenAt: nil,
		Category:      report.Category,
		Comment:       optionalString(report.Comment),
		Forwarded:     report.Forwarded,
		CreatedAt:     model.Time(report.CreatedAt),
		StatusIds:     report.StatusIDs,
		RuleIds:       ruleIDsOut,
		TargetAccount: targetActor,
	}, nil
}

func normalizeReportCategory(category *string) string {
	value := strings.TrimSpace(ptrOrEmpty(category))
	if value == "" {
		return "other"
	}
	if err := common.ValidateReportCategory(value); err != nil {
		return "other"
	}
	return value
}

func normalizeReportComment(comment *string) (string, error) {
	value := strings.TrimSpace(ptrOrEmpty(comment))
	if value == "" {
		return "", nil
	}
	if err := common.ValidateReportComment(value); err != nil {
		return "", err
	}
	return value, nil
}

func normalizeReportStatusIDs(statusIDs []string) ([]string, error) {
	out := make([]string, 0, len(statusIDs))
	for _, statusID := range statusIDs {
		statusID = strings.TrimSpace(statusID)
		if statusID == "" {
			continue
		}
		out = append(out, statusID)
	}
	if len(out) == 0 {
		return out, nil
	}
	if err := common.ValidateReportStatusIDs(out); err != nil {
		return nil, err
	}
	return out, nil
}

func convertRuleIDs(ruleIDs []int) (asStrings []string, asInts []int) {
	asStrings = make([]string, 0, len(ruleIDs))
	asInts = make([]int, 0, len(ruleIDs))
	for _, ruleID := range ruleIDs {
		asStrings = append(asStrings, fmt.Sprintf("%d", ruleID))
		asInts = append(asInts, ruleID)
	}
	return asStrings, asInts
}

func boolOrFalse(value *bool) bool {
	if value == nil {
		return false
	}
	return *value
}

func ptrOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (r *mutationResolver) resolveReporterActorID(ctx context.Context, username string) string {
	actorID := username
	if r.Storage == nil || r.Storage.Actor() == nil {
		return actorID
	}

	actor, err := r.Storage.Actor().GetActor(ctx, username)
	if err == nil && actor != nil && actor.ID != "" {
		return actor.ID
	}
	return actorID
}

func (r *mutationResolver) maybeCreateModerationEvent(ctx context.Context, report *storage.Report, reporterActorID string) {
	if report == nil {
		return
	}

	enhanced := reports.NewEnhancedReportService(r.Storage, r.Logger)
	if enhanced == nil {
		return
	}

	moderationEvent, err := enhanced.CreateEnhancedModerationEvent(ctx, report, reporterActorID)
	if err == nil && moderationEvent != nil {
		report.ModerationEventID = moderationEvent.ID
		_ = r.Storage.Moderation().UpdateReportStatus(ctx, report.ID, storage.ReportStatus(report.Status), "", "")
		return
	}

	if err != nil {
		r.Logger.Warn("failed to create enhanced moderation event; falling back",
			zap.String("report_id", report.ID),
			zap.Error(err))
		r.createBasicModerationEvent(ctx, report, reporterActorID)
	}
}

func (r *Resolver) createBasicModerationEvent(ctx context.Context, report *storage.Report, actorID string) {
	if report == nil || r == nil || r.Storage == nil || r.Storage.Moderation() == nil {
		return
	}

	now := time.Now()
	eventID := uuid.New().String()
	objectType := "Actor"
	objectID := report.TargetAccountID
	category := report.Category
	severity := "2" // medium

	if len(report.StatusIDs) > 0 {
		objectType = "Note"
		objectID = report.StatusIDs[0]
	}

	storageEvent := &storage.ModerationEvent{
		ID:              eventID,
		EventType:       "flagged",
		ObjectID:        objectID,
		ObjectType:      objectType,
		ActorID:         actorID,
		Category:        category,
		Severity:        severity,
		ConfidenceScore: 1.0,
		Evidence:        []any{},
		Reason:          report.Comment,
		Created:         now,
		Updated:         now,
		TTL:             now.Add(90 * 24 * time.Hour).Unix(),
	}

	if err := r.Storage.Moderation().CreateModerationEvent(ctx, storageEvent); err != nil {
		r.Logger.Error("failed to create basic moderation event", zap.Error(err))
	} else {
		report.ModerationEventID = eventID
		_ = r.Storage.Moderation().UpdateReportStatus(ctx, report.ID, storage.ReportStatus(report.Status), "", "")
	}
}
