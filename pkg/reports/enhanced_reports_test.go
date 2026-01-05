package reports

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/moderation"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/trust"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type stubModerationRepo struct {
	stats    *storage.ReportStats
	statsErr error

	createdEvent  *storage.ModerationEvent
	createEventErr error

	report    *storage.Report
	reportErr error

	updateStatusArgs struct {
		reportID  string
		status    storage.ReportStatus
		action    string
		updatedBy string
	}

	incrementFalseReportsCalled bool
	incrementFalseReportsErr    error

	moderationEvent    *storage.ModerationEvent
	moderationEventErr error

	moderationDecision    *storage.ModerationDecision
	moderationDecisionErr error
}

func (s *stubModerationRepo) GetReportStats(_ context.Context, _ string) (*storage.ReportStats, error) {
	return s.stats, s.statsErr
}

func (s *stubModerationRepo) CreateModerationEvent(_ context.Context, event *storage.ModerationEvent) error {
	s.createdEvent = event
	return s.createEventErr
}

func (s *stubModerationRepo) GetReport(_ context.Context, _ string) (*storage.Report, error) {
	return s.report, s.reportErr
}

func (s *stubModerationRepo) UpdateReportStatus(_ context.Context, reportID string, status storage.ReportStatus, action string, updatedBy string) error {
	s.updateStatusArgs.reportID = reportID
	s.updateStatusArgs.status = status
	s.updateStatusArgs.action = action
	s.updateStatusArgs.updatedBy = updatedBy
	return nil
}

func (s *stubModerationRepo) IncrementFalseReports(_ context.Context, _ string) error {
	s.incrementFalseReportsCalled = true
	return s.incrementFalseReportsErr
}

func (s *stubModerationRepo) GetModerationEvent(_ context.Context, _ string) (*storage.ModerationEvent, error) {
	return s.moderationEvent, s.moderationEventErr
}

func (s *stubModerationRepo) GetModerationDecision(_ context.Context, _ string) (*storage.ModerationDecision, error) {
	return s.moderationDecision, s.moderationDecisionErr
}

type stubTrustRepo struct {
	score    *storage.TrustScore
	scoreErr error

	createdRel *storage.TrustRelationship
	createErr  error
}

func (s *stubTrustRepo) GetTrustScore(_ context.Context, _ string, _ string) (*storage.TrustScore, error) {
	return s.score, s.scoreErr
}

func (s *stubTrustRepo) CreateTrustRelationship(_ context.Context, relationship *storage.TrustRelationship) error {
	s.createdRel = relationship
	return s.createErr
}

func Test_getSeverityString(t *testing.T) {
	assert.Equal(t, "1", getSeverityString(moderation.SeverityLow))
	assert.Equal(t, "2", getSeverityString(moderation.SeverityMedium))
	assert.Equal(t, "3", getSeverityString(moderation.SeverityHigh))
	assert.Equal(t, "4", getSeverityString(moderation.SeverityCritical))
	assert.Equal(t, "2", getSeverityString(moderation.Severity(999)))
}

func TestEnhancedReportService_CalculateReporterReliability(t *testing.T) {
	svc := &EnhancedReportService{
		moderation: &stubModerationRepo{statsErr: errors.New("boom")},
		logger:     zap.NewNop(),
	}

	rel, err := svc.CalculateReporterReliability(context.Background(), "alice")
	require.NoError(t, err)
	assert.Equal(t, 0.5, rel.ReliabilityScore)
	assert.Equal(t, 1.0, rel.TrustModifier)

	svc.moderation = &stubModerationRepo{stats: &storage.ReportStats{TotalReports: 5, ResolvedReports: 0}}
	rel, err = svc.CalculateReporterReliability(context.Background(), "alice")
	require.NoError(t, err)
	assert.Equal(t, 0.5, rel.ReliabilityScore)

	svc.moderation = &stubModerationRepo{stats: &storage.ReportStats{TotalReports: 10, ResolvedReports: 10, FalseReports: 0}}
	rel, err = svc.CalculateReporterReliability(context.Background(), "alice")
	require.NoError(t, err)
	assert.Greater(t, rel.ReliabilityScore, 0.6)
	assert.LessOrEqual(t, rel.ReliabilityScore, 0.95)
}

func TestEnhancedReportService_CreateEnhancedModerationEvent(t *testing.T) {
	modRepo := &stubModerationRepo{stats: &storage.ReportStats{}}
	trustRepo := &stubTrustRepo{score: &storage.TrustScore{Score: 1.0}}
	svc := &EnhancedReportService{
		moderation: modRepo,
		trust:      trustRepo,
		logger:     zap.NewNop(),
	}

	report := &storage.Report{
		ID:              "r1",
		ReporterID:      "alice",
		TargetAccountID: "target",
		StatusIDs:       []string{"s1"},
		Comment:         "spam report",
		Category:        "spam",
		RuleIDs:         []string{"rule1"},
		Forwarded:       true,
	}

	event, err := svc.CreateEnhancedModerationEvent(context.Background(), report, "actor1")
	require.NoError(t, err)
	require.NotNil(t, event)

	require.NotNil(t, modRepo.createdEvent)
	assert.Equal(t, "flagged", modRepo.createdEvent.EventType)
	assert.Equal(t, "Note", modRepo.createdEvent.ObjectType)
	assert.Equal(t, "s1", modRepo.createdEvent.ObjectID)
	assert.Equal(t, "actor1", modRepo.createdEvent.ActorID)
	assert.Equal(t, "spam", modRepo.createdEvent.Category)
	assert.Equal(t, "3", modRepo.createdEvent.Severity)
	assert.InDelta(t, 0.85, modRepo.createdEvent.ConfidenceScore, 0.0001)

	assert.Equal(t, moderation.CategorySpam, event.Category)
	assert.Equal(t, moderation.SeverityHigh, event.Severity)

	modRepo.createEventErr = errors.New("boom")
	_, err = svc.CreateEnhancedModerationEvent(context.Background(), report, "actor1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create moderation event")
}

func TestEnhancedReportService_UpdateReporterTrustOnDecision(t *testing.T) {
	report := &storage.Report{ID: "r1", ReporterID: "alice"}
	modRepo := &stubModerationRepo{report: report}
	trustRepo := &stubTrustRepo{score: &storage.TrustScore{Score: 0.5}}
	svc := &EnhancedReportService{
		moderation: modRepo,
		trust:      trustRepo,
		logger:     zap.NewNop(),
	}

	validDecision := &moderation.ModerationDecision{
		ID:             "d1",
		Action:         moderation.ActionTypeRemove,
		ConsensusScore: 0.5,
		Reviews:        []*moderation.Review{{Severity: moderation.SeverityHigh}},
	}

	err := svc.UpdateReporterTrustOnDecision(context.Background(), "r1", validDecision, "actor1")
	require.NoError(t, err)
	require.NotNil(t, trustRepo.createdRel)
	assert.InDelta(t, 0.545, trustRepo.createdRel.Score, 0.0001)
	assert.False(t, modRepo.incrementFalseReportsCalled)
	assert.Equal(t, storage.ReportStatusResolved, modRepo.updateStatusArgs.status)

	trustRepo.createdRel = nil
	modRepo.incrementFalseReportsCalled = false
	invalidDecision := &moderation.ModerationDecision{
		ID:             "d2",
		Action:         moderation.ActionTypeNone,
		ConsensusScore: 0.9,
	}

	err = svc.UpdateReporterTrustOnDecision(context.Background(), "r1", invalidDecision, "actor1")
	require.NoError(t, err)
	require.NotNil(t, trustRepo.createdRel)
	assert.InDelta(t, 0.35, trustRepo.createdRel.Score, 0.0001)
	assert.True(t, modRepo.incrementFalseReportsCalled)
}

func TestEnhancedReportService_GetReportModerationStatus(t *testing.T) {
	modRepo := &stubModerationRepo{
		report: &storage.Report{
			ID:                "r1",
			Status:            string(storage.ReportStatusOpen),
			ModerationEventID: "e1",
		},
		moderationEvent: &storage.ModerationEvent{
			EventType: "flagged",
			ObjectID:  "obj1",
		},
		moderationDecision: &storage.ModerationDecision{
			ConsensusScore: 0.7,
			Action:         string(moderation.ActionTypeRemove),
		},
	}
	svc := &EnhancedReportService{
		moderation: modRepo,
		trust:      &stubTrustRepo{},
		logger:     zap.NewNop(),
	}

	status, err := svc.GetReportModerationStatus(context.Background(), "r1")
	require.NoError(t, err)
	require.NotNil(t, status)
	assert.Equal(t, "r1", status.ReportID)
	assert.Equal(t, "flagged", status.ModerationStatus)
	assert.True(t, status.ConsensusReached)
	assert.Equal(t, 0.7, status.ConsensusScore)
	assert.Equal(t, string(moderation.ActionTypeRemove), status.Decision)
}

func TestEnhancedReportService_UsesTrustConstants(t *testing.T) {
	// Guardrail: trust package is imported and referenced by this file.
	assert.Equal(t, "content", string(trust.TrustCategoryContent))
}

