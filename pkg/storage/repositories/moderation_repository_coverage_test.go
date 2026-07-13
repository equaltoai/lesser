package repositories

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func setupPermissiveDynamormMocks(mockDB *mocks.MockDB, mockQuery *mocks.MockQuery) {
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Between", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Cursor", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		populateSliceResult(args.Get(0))
	}).Return(nil).Maybe()

	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		populateSliceResult(args.Get(0))
	}).Return(nil).Maybe()

	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		populateStructResult(args.Get(0))
	}).Return(nil).Maybe()

	mockQuery.On("Create").Return(nil).Maybe()
	mockQuery.On("Update", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Delete").Return(nil).Maybe()
	mockQuery.On("Count").Return(int64(1), nil).Maybe()
}

func populateStructResult(target any) {
	fillModelForCoverage(target, 0)
}

func populateSliceResult(target any) {
	value := reflect.ValueOf(target)
	if value.Kind() != reflect.Ptr || value.Elem().Kind() != reflect.Slice {
		return
	}

	slice := value.Elem()
	elemType := slice.Type().Elem()

	// The moderation repository uses []interface{} in deleteFilterEntity; leave it empty to
	// exercise the not-found path deterministically.
	if elemType.Kind() == reflect.Interface {
		return
	}

	baseType := elemType
	if baseType.Kind() == reflect.Ptr {
		baseType = baseType.Elem()
	}

	// Many moderation endpoints rely on over-fetching (limit+1) for pagination, or (limit*2)+1
	// for scans; returning multiple rows ensures we cover the "next cursor" branches.
	count := 2
	if baseType == reflect.TypeOf(models.ModerationEvent{}) {
		count = 3
	}

	for i := range count {
		var element reflect.Value
		if elemType.Kind() == reflect.Ptr {
			element = reflect.New(baseType)
			fillModelForCoverage(element.Interface(), i)
		} else {
			elemPtr := reflect.New(baseType)
			fillModelForCoverage(elemPtr.Interface(), i)
			element = elemPtr.Elem()
		}

		slice = reflect.Append(slice, element)
	}

	value.Elem().Set(slice)
}

func fillModelForCoverage(target any, idx int) {
	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(idx) * time.Minute)

	switch model := target.(type) {
	case *models.ModerationEvent:
		model.ID = fmt.Sprintf("event-%d", idx+1)
		model.EventType = storage.EventTypeFlagged
		model.ObjectID = "obj-1"
		model.ObjectType = "status"
		model.ActorID = "actor-1"
		model.Category = "spam"
		model.Severity = string(storage.SeverityHigh)
		model.ConfidenceScore = 0.9
		model.Evidence = []any{"evidence"}
		model.Reason = "reason"
		model.Created = baseTime
		model.Updated = baseTime
		model.CreatedAt = baseTime
		_ = model.UpdateKeys()

	case *models.ModerationReview:
		model.ID = fmt.Sprintf("review-%d", idx+1)
		model.EventID = "event-1"
		model.ReviewerID = "reviewer-1"
		model.ReviewerRep = 0.6
		model.Action = "approve"
		model.Severity = "spam"
		model.Note = "note"
		model.Tags = []string{"tag"}
		model.Metadata = map[string]any{"k": "v"}
		model.Confidence = 0.8
		model.Created = baseTime
		model.Type = "REVIEW"

	case *models.ModerationDecision:
		expires := baseTime.Add(24 * time.Hour)
		model.ID = fmt.Sprintf("decision-%d", idx+1)
		model.EventID = "event-1"
		model.ObjectID = "obj-1"
		model.Action = "allow"
		model.ConsensusScore = 0.9
		model.ReviewerCount = 1
		model.TrustWeightTotal = 0.9
		model.Reviews = []interface{}{"review-1"}
		model.Metadata = map[string]any{"k": "v"}
		model.Decided = baseTime
		model.Expires = &expires
		model.UpdateKeys()

	case *models.ModerationPattern:
		model.PatternID = fmt.Sprintf("pattern-%d", idx+1)
		model.Name = "pattern"
		model.Description = "desc"
		model.Type = "keyword"
		model.Pattern = "bad"
		model.Severity = 2.0
		model.Active = true
		model.CreatedAt = baseTime
		model.UpdatedAt = baseTime
		model.PK = fmt.Sprintf("MODERATION_PATTERN#%s", model.PatternID)
		model.SK = "PATTERN"
		model.GSI1PK = "MODERATION_PATTERNS#ACTIVE"
		model.GSI1SK = "2.00#keyword#" + model.PatternID
		model.GSI2PK = "MODERATION_PATTERNS#2"
		model.GSI2SK = baseTime.Format(time.RFC3339) + "#" + model.PatternID

	case *models.Filter:
		model.ID = fmt.Sprintf("filter-%d", idx+1)
		model.Username = "user-1"
		model.Title = "filter title"
		model.Context = []string{"home"}
		model.FilterAction = "hide"
		model.CreatedAt = baseTime
		model.UpdatedAt = baseTime
		_ = model.UpdateKeys()

	case *models.FilterKeyword:
		model.ID = fmt.Sprintf("keyword-%d", idx+1)
		model.FilterID = "filter-1"
		model.Keyword = "spam"
		model.WholeWord = true
		model.CreatedAt = baseTime
		_ = model.UpdateKeys()

	case *models.FilterStatus:
		model.ID = fmt.Sprintf("filter-status-%d", idx+1)
		model.FilterID = "filter-1"
		model.StatusID = fmt.Sprintf("status-id-%d", idx+1)
		model.CreatedAt = baseTime
		_ = model.UpdateKeys()

	case *models.Report:
		model.ID = fmt.Sprintf("report-%d", idx+1)
		model.ReporterID = "user-1"
		model.TargetAccountID = "target-1"
		model.StatusIDs = []string{"status-1"}
		model.Comment = "comment"
		model.Category = "spam"
		model.RuleIDs = []int{1, 2}
		model.Forwarded = false
		model.Status = string(storage.ReportStatusOpen)
		model.CreatedAt = baseTime
		model.UpdatedAt = baseTime
		model.AssignedTo = "mod-1"
		model.UpdateKeys()

	case *models.ReportStats:
		now := baseTime
		model.TotalReports = 10
		model.ResolvedReports = 3
		model.FalseReports = 1
		model.LastReportAt = &now
		model.UpdateKeys("user-1")

	case *models.Flag:
		model.ID = fmt.Sprintf("flag-%d", idx+1)
		model.Actor = "actor-1"
		model.Object = []string{"obj-1"}
		model.Content = "flag reason"
		model.Published = baseTime
		model.Status = string(storage.FlagStatusPending)
		model.CreatedAt = baseTime
		model.UpdateKeys()

	case *models.AuditLog:
		model.ID = fmt.Sprintf("audit-%d", idx+1)
		model.AdminID = "admin-1"
		model.AdminRole = "admin"
		model.Action = "action"
		model.TargetType = "report"
		model.TargetID = "report-1"
		model.Reason = "reason"
		model.Details = map[string]any{"k": "v"}
		model.IPAddress = "127.0.0.1"
		model.UserAgent = "test"
		model.RequestID = "req-1"
		model.Timestamp = baseTime
		model.CreatedAt = baseTime
		model.UpdateKeys()

	case *models.User:
		model.Username = "reviewer-1"
		model.CreatedAt = baseTime

	case *models.TrustScore:
		model.ActorID = "reviewer-1"
		model.Score = 0.75

	case *models.ModerationDecisionResult:
		model.ID = fmt.Sprintf("decision-result-%d", idx+1)
		model.ContentID = "content-1"
		model.AuthorID = "author-1"
		model.Action = "remove"
		model.Confidence = 0.9
		model.Reasons = []interface{}{"reason"}
		model.RequiresReview = true
		model.ReviewPriority = 8
		model.Recommendations = []string{"review"}
		model.DecidedAt = baseTime
		model.EnforcementStatus = "pending"
		model.UpdateKeys()

	case *models.ModerationReviewQueue:
		model.ID = fmt.Sprintf("queue-%d", idx+1)
		model.ContentID = "content-1"
		model.AuthorID = "author-1"
		model.Status = StatusPending
		model.Priority = 8
		model.Category = "moderation"
		model.Severity = "medium"
		model.Reason = "reason"
		model.CreatedAt = baseTime
		model.UpdatedAt = baseTime
		model.UpdateKeys()
	}
}

func TestModerationRepository_Coverage_Smoke(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveDynamormMocks(mockDB, mockQuery)

	repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())

	require.Len(t, generateRandomString(), 12)

	event := &storage.ModerationEvent{
		EventType:       storage.EventTypeFlagged,
		ObjectID:        "obj-1",
		ObjectType:      "status",
		ActorID:         "actor-1",
		Category:        "spam",
		Severity:        string(storage.SeverityHigh),
		ConfidenceScore: 0.9,
	}
	require.NoError(t, repo.CreateModerationEvent(ctx, event))

	_, err := repo.GetModerationEvent(ctx, "event-1")
	require.NoError(t, err)

	items, err := repo.GetModerationQueue(ctx, nil)
	require.NoError(t, err)
	require.NotEmpty(t, items)

	_, nextCursor, err := repo.GetModerationQueuePaginated(ctx, 1, "cursor-0")
	require.NoError(t, err)
	require.NotEmpty(t, nextCursor)

	_, nextCursor, err = repo.GetModerationEventsByObject(ctx, "obj-1", 1, "cursor-0")
	require.NoError(t, err)
	require.NotEmpty(t, nextCursor)

	_, nextCursor, err = repo.GetModerationEventsByActor(ctx, "actor-1", 1, "cursor-0")
	require.NoError(t, err)
	require.NotEmpty(t, nextCursor)

	review := &storage.ModerationReview{
		EventID:     "event-1",
		ReviewerID:  "reviewer-1",
		Action:      "approve",
		Severity:    string(storage.SeverityMedium),
		Confidence:  0.8,
		ReviewerRep: 0.6,
		Created:     time.Now(),
	}
	require.NoError(t, repo.AddModerationReview(ctx, review))

	_, err = repo.GetModerationReviews(ctx, "event-1")
	require.NoError(t, err)

	decision := &storage.ModerationDecision{
		EventID:          "event-1",
		ObjectID:         "obj-1",
		Action:           "allow",
		ConsensusScore:   0.9,
		ReviewerCount:    1,
		TrustWeightTotal: 0.9,
		Reviews:          []interface{}{"review-1"},
	}
	require.NoError(t, repo.CreateModerationDecision(ctx, decision))

	_, err = repo.GetModerationDecision(ctx, "obj-1")
	require.NoError(t, err)

	require.NoError(t, repo.StoreModerationDecision(ctx, &storage.ModerationDecision{
		EventID:        "event-1",
		ObjectID:       "obj-1",
		Action:         "remove",
		ConsensusScore: 0.75,
	}))

	require.NoError(t, repo.UpdateModerationDecision(ctx, "obj-1", &storage.ModerationReview{
		ReviewerID: "reviewer-1",
		Action:     "remove",
		Confidence: 0.9,
	}))

	_, err = repo.GetModerationPatterns(ctx, true, "2", 10)
	require.NoError(t, err)
	_, err = repo.GetModerationPatterns(ctx, true, "", 10)
	require.NoError(t, err)
	_, err = repo.GetModerationPatterns(ctx, false, "2", 1)
	require.NoError(t, err)

	require.NoError(t, repo.CreateModerationPattern(ctx, &storage.ModerationPattern{
		Name:        "pattern",
		Description: "desc",
		Type:        "keyword",
		Content:     "bad",
		Severity:    "2.0",
		Active:      true,
	}))

	_, err = repo.GetModerationPattern(ctx, "pattern-1")
	require.NoError(t, err)

	require.NoError(t, repo.UpdateModerationPattern(ctx, &storage.ModerationPattern{
		ID:          "pattern-1",
		Name:        "pattern",
		Description: "desc",
		Type:        "keyword",
		Content:     "bad",
		Severity:    "2.0",
		Active:      true,
		CreatedAt:   time.Now(),
	}))

	require.NoError(t, repo.DeleteModerationPattern(ctx, "pattern-1"))

	history, err := repo.GetModerationHistory(ctx, "obj-1")
	require.NoError(t, err)
	require.NotEmpty(t, history.Timeline)

	_, _, err = repo.GetModerationEvents(ctx, nil, 1, "cursor-0")
	require.NoError(t, err)

	_, _, err = repo.GetModerationEvents(ctx, &storage.ModerationEventFilter{
		EventType: storage.EventTypeFlagged,
		Category:  "spam",
	}, 1, "cursor-0")
	require.NoError(t, err)

	require.NoError(t, repo.CreateAdminReview(ctx, "event-1", "admin-1", storage.ActionType("remove"), "reason"))

	_, err = repo.GetReviewerStats(ctx, "reviewer-1")
	require.NoError(t, err)

	_, err = repo.GetModerationQueueCount(ctx)
	require.NoError(t, err)

	require.NoError(t, repo.RecordPatternMatch(ctx, "pattern-1", true, time.Now()))

	require.NoError(t, repo.CreateFilter(ctx, &storage.Filter{
		Username:     "user-1",
		Title:        "filter title",
		Context:      []string{"home"},
		FilterAction: "hide",
	}))

	_, err = repo.GetFilter(ctx, "filter-1")
	require.NoError(t, err)

	_, err = repo.GetFiltersForUser(ctx, "user-1")
	require.NoError(t, err)

	expiresAt := time.Now().Add(1 * time.Hour)
	require.NoError(t, repo.UpdateFilter(ctx, "filter-1", map[string]any{
		"title":         "updated",
		"context":       []string{"public"},
		"filter_action": "warn",
		"expires_at":    &expiresAt,
	}))

	require.NoError(t, repo.AddFilterKeyword(ctx, "filter-1", &storage.FilterKeyword{
		Keyword:   "spam",
		WholeWord: true,
	}))

	_, err = repo.GetFilterKeywords(ctx, "filter-1")
	require.NoError(t, err)

	require.NoError(t, repo.UpdateFilterKeyword(ctx, "keyword-1", map[string]any{
		"keyword":    "ham",
		"whole_word": false,
	}))

	require.NoError(t, repo.AddFilterStatus(ctx, "filter-1", &storage.FilterStatus{StatusID: "status-id-1"}))

	_, err = repo.GetFilterStatuses(ctx, "filter-1")
	require.NoError(t, err)

	require.NoError(t, repo.DeleteFilter(ctx, "filter-1"))

	require.NoError(t, repo.CreateReport(ctx, &storage.Report{
		ReporterID:      "user-1",
		TargetAccountID: "target-1",
		StatusIDs:       []string{"status-1"},
		Comment:         "comment",
		Category:        "spam",
		RuleIDs:         []string{"1", "not-an-int"},
	}))

	_, err = repo.GetReport(ctx, "report-1")
	require.NoError(t, err)

	_, _, err = repo.GetUserReports(ctx, "user-1", 1, "cursor-0")
	require.NoError(t, err)

	require.NoError(t, repo.UpdateReportStatus(ctx, "report-1", storage.ReportStatusResolved, "removed", "mod-1"))

	_, _, err = repo.GetReportsByTarget(ctx, "target-1", 1, "cursor-0")
	require.NoError(t, err)

	_, _, err = repo.GetReportsByStatus(ctx, storage.ReportStatusOpen, 1, "cursor-0")
	require.NoError(t, err)

	_, err = repo.GetReportStats(ctx, "user-1")
	require.NoError(t, err)

	require.NoError(t, repo.IncrementFalseReports(ctx, "user-1"))

	require.NoError(t, repo.AssignReport(ctx, "report-1", "mod-1"))
	require.NoError(t, repo.UnassignReport(ctx, "report-1"))

	_, err = repo.GetOpenReportsCount(ctx)
	require.NoError(t, err)

	_, err = repo.GetReportedStatuses(ctx, "report-1")
	require.NoError(t, err)

	require.NoError(t, repo.CreateFlag(ctx, &storage.Flag{
		Actor:   "actor-1",
		Object:  []string{"obj-1"},
		Content: "flag reason",
	}))

	_, err = repo.GetFlag(ctx, "flag-1")
	require.NoError(t, err)

	_, _, err = repo.GetFlagsByObject(ctx, "obj-1", 1, "cursor-0")
	require.NoError(t, err)

	_, _, err = repo.GetFlagsByActor(ctx, "actor-1", 1, "cursor-0")
	require.NoError(t, err)

	_, _, err = repo.GetPendingFlags(ctx, 1, "cursor-0")
	require.NoError(t, err)

	require.NoError(t, repo.UpdateFlagStatus(ctx, "flag-1", storage.FlagStatusReviewed, "mod-1", "review note"))

	_, err = repo.CountPendingFlags(ctx)
	require.NoError(t, err)

	require.NoError(t, repo.DeleteFlag(ctx, "flag-1"))

	require.NoError(t, repo.CreateAuditLog(ctx, &storage.AuditLog{
		AdminID:    "admin-1",
		AdminRole:  "admin",
		Action:     "resolve_report",
		TargetType: "report",
		TargetID:   "report-1",
		Reason:     "reason",
		Details:    map[string]any{"k": "v"},
		IPAddress:  "127.0.0.1",
		UserAgent:  "test",
		RequestID:  "req-1",
	}))

	_, nextCursor, err = repo.GetAuditLogs(ctx, 1, "")
	require.NoError(t, err)
	require.NotEmpty(t, nextCursor)

	_, _, err = repo.GetAuditLogs(ctx, 1, "cursor-0")
	require.NoError(t, err)

	_, _, err = repo.GetAuditLogsByAdmin(ctx, "admin-1", 1, "cursor-0")
	require.NoError(t, err)

	_, _, err = repo.GetAuditLogsByTarget(ctx, "report-1", 1, "cursor-0")
	require.NoError(t, err)

	_, err = repo.GetPendingModerationCount(ctx, "mod-1")
	require.NoError(t, err)

	require.NoError(t, repo.StoreAnalysisResult(ctx, map[string]interface{}{
		"content_id":       "content-1",
		"author_id":        "author-1",
		"analysis_type":    "",
		"content_type":     "status",
		"confidence":       0.9,
		"results":          map[string]interface{}{"k": "v"},
		"pattern_matches":  []interface{}{"p1"},
		"threat_matches":   []interface{}{"t1"},
		"reputation_score": 0.5,
		"processing_time":  int64(42),
	}))

	require.NoError(t, repo.StoreDecision(ctx, map[string]interface{}{
		"content_id":      "content-1",
		"action":          "remove",
		"author_id":       "author-1",
		"confidence":      0.9,
		"reasons":         []interface{}{"r1"},
		"requires_review": true,
		"review_priority": 8,
		"recommendations": []string{"manual_review"},
		"metadata":        map[string]interface{}{"k": "v"},
	}))

	_, err = repo.GetReviewQueue(ctx, map[string]interface{}{
		"status": StatusPending,
		"limit":  1,
	})
	require.NoError(t, err)

	_, err = repo.GetDecisionHistory(ctx, "content-1")
	require.NoError(t, err)

	require.NoError(t, repo.UpdateEnforcementStatus(ctx, "content-1", "applied"))
	require.NoError(t, repo.UpdateEnforcementStatus(ctx, "content-1", "failed"))
	require.NoError(t, repo.UpdateEnforcementStatus(ctx, "content-1", "expired"))

	_, err = repo.GetModerationDecisionsByModerator(ctx, "reviewer-1", 1)
	require.NoError(t, err)
}
