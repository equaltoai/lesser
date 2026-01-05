package repositories

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// newMinimalModerationRepo creates a minimal repository for testing pure functions
func newMinimalModerationRepo() *ModerationRepository {
	return NewModerationRepository(nil, "test-table", zap.NewNop())
}

// ============================================
// Test getSeverityValue
// ============================================

func TestModerationRepository_getSeverityValue(t *testing.T) {
	repo := newMinimalModerationRepo()

	tests := []struct {
		name     string
		severity string
		expected float64
	}{
		{
			name:     "low severity",
			severity: "low",
			expected: 1.0,
		},
		{
			name:     "medium severity",
			severity: "medium",
			expected: 2.0,
		},
		{
			name:     "high severity",
			severity: "high",
			expected: 3.0,
		},
		{
			name:     "critical severity",
			severity: "critical",
			expected: 4.0,
		},
		{
			name:     "unknown severity defaults to low",
			severity: "unknown",
			expected: 1.0,
		},
		{
			name:     "empty severity defaults to low",
			severity: "",
			expected: 1.0,
		},
		{
			name:     "numeric severity defaults to low",
			severity: "5",
			expected: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := repo.getSeverityValue(tt.severity)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// ============================================
// Test shouldScanAllEvents
// ============================================

func TestModerationRepository_shouldScanAllEvents(t *testing.T) {
	repo := newMinimalModerationRepo()

	tests := []struct {
		name     string
		filter   *storage.ModerationEventFilter
		expected bool
	}{
		{
			name:     "nil filter should scan all",
			filter:   nil,
			expected: true,
		},
		{
			name:     "empty filter should scan all",
			filter:   &storage.ModerationEventFilter{},
			expected: true,
		},
		{
			name: "filter with only event type should not scan all",
			filter: &storage.ModerationEventFilter{
				EventType: "flagged",
			},
			expected: false,
		},
		{
			name: "filter with only category should not scan all",
			filter: &storage.ModerationEventFilter{
				Category: "spam",
			},
			expected: false,
		},
		{
			name: "filter with only actor ID should not scan all",
			filter: &storage.ModerationEventFilter{
				ActorID: "actor-123",
			},
			expected: false,
		},
		{
			name: "filter with only object ID should not scan all",
			filter: &storage.ModerationEventFilter{
				ObjectID: "object-456",
			},
			expected: false,
		},
		{
			name: "filter with event type and category should not scan all",
			filter: &storage.ModerationEventFilter{
				EventType: "flagged",
				Category:  "spam",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := repo.shouldScanAllEvents(tt.filter)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// ============================================
// Test buildGSI2Key
// ============================================

func TestModerationRepository_buildGSI2Key(t *testing.T) {
	repo := newMinimalModerationRepo()

	tests := []struct {
		name     string
		filter   *storage.ModerationEventFilter
		expected string
	}{
		{
			name: "event type only",
			filter: &storage.ModerationEventFilter{
				EventType: "flagged",
			},
			expected: "TYPE#flagged",
		},
		{
			name: "event type and category",
			filter: &storage.ModerationEventFilter{
				EventType: "flagged",
				Category:  "spam",
			},
			expected: "TYPE#flagged#spam",
		},
		{
			name:     "empty filter uses default event type",
			filter:   &storage.ModerationEventFilter{},
			expected: "TYPE#flagged", // Default EventTypeFlagged
		},
		{
			name: "category without event type",
			filter: &storage.ModerationEventFilter{
				Category: "harassment",
			},
			expected: "TYPE#flagged#harassment", // Uses default event type
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := repo.buildGSI2Key(tt.filter)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// ============================================
// Test getEventType
// ============================================

func TestModerationRepository_getEventType(t *testing.T) {
	repo := newMinimalModerationRepo()

	tests := []struct {
		name     string
		filter   *storage.ModerationEventFilter
		expected storage.EventType
	}{
		{
			name: "returns filter event type when set",
			filter: &storage.ModerationEventFilter{
				EventType: "reported",
			},
			expected: storage.EventType("reported"),
		},
		{
			name:     "returns default flagged when empty",
			filter:   &storage.ModerationEventFilter{},
			expected: storage.EventTypeFlagged,
		},
		{
			name: "returns custom event type",
			filter: &storage.ModerationEventFilter{
				EventType: "custom_event",
			},
			expected: storage.EventType("custom_event"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := repo.getEventType(tt.filter)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// ============================================
// Test getCategory
// ============================================

func TestModerationRepository_getCategory(t *testing.T) {
	repo := newMinimalModerationRepo()

	tests := []struct {
		name     string
		filter   *storage.ModerationEventFilter
		expected string
	}{
		{
			name: "returns filter category when set",
			filter: &storage.ModerationEventFilter{
				Category: "spam",
			},
			expected: "spam",
		},
		{
			name:     "returns empty string when not set",
			filter:   &storage.ModerationEventFilter{},
			expected: "",
		},
		{
			name: "returns custom category",
			filter: &storage.ModerationEventFilter{
				Category: "harassment",
			},
			expected: "harassment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := repo.getCategory(tt.filter)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// ============================================
// Test determineNextCursor
// ============================================

func TestModerationRepository_determineNextCursor(t *testing.T) {
	repo := newMinimalModerationRepo()

	tests := []struct {
		name       string
		models     []models.ModerationEvent
		limit      int
		expectNext bool
	}{
		{
			name:       "empty models returns empty cursor",
			models:     []models.ModerationEvent{},
			limit:      10,
			expectNext: false,
		},
		{
			name: "models at limit returns empty cursor",
			models: []models.ModerationEvent{
				{GSI2SK: "cursor1"},
				{GSI2SK: "cursor2"},
			},
			limit:      2,
			expectNext: false,
		},
		{
			name: "models under limit returns empty cursor",
			models: []models.ModerationEvent{
				{GSI2SK: "cursor1"},
			},
			limit:      10,
			expectNext: false,
		},
		{
			name: "models over limit returns cursor",
			models: []models.ModerationEvent{
				{GSI2SK: "cursor1"},
				{GSI2SK: "cursor2"},
				{GSI2SK: "cursor3"},
			},
			limit:      2,
			expectNext: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := repo.determineNextCursor(tt.models, tt.limit)
			if tt.expectNext {
				assert.NotEmpty(t, got)
				// When there are more items than limit, cursor should be from limit-1
				assert.Equal(t, tt.models[tt.limit-1].GSI2SK, got)
			} else {
				assert.Empty(t, got)
			}
		})
	}
}

// ============================================
// Test matchesEventFilter
// ============================================

func TestModerationRepository_matchesEventFilter(t *testing.T) {
	repo := newMinimalModerationRepo()

	now := time.Now()
	yesterday := now.Add(-24 * time.Hour)
	tomorrow := now.Add(24 * time.Hour)
	lowSeverity := 1

	tests := []struct {
		name     string
		event    *storage.ModerationEvent
		filter   *storage.ModerationEventFilter
		expected bool
	}{
		{
			name: "nil filter matches everything",
			event: &storage.ModerationEvent{
				EventType: "flagged",
				Category:  "spam",
			},
			filter:   nil,
			expected: true,
		},
		{
			name: "empty filter matches everything",
			event: &storage.ModerationEvent{
				EventType: "flagged",
				Category:  "spam",
			},
			filter:   &storage.ModerationEventFilter{},
			expected: true,
		},
		{
			name: "matching event type",
			event: &storage.ModerationEvent{
				EventType: "flagged",
			},
			filter: &storage.ModerationEventFilter{
				EventType: "flagged",
			},
			expected: true,
		},
		{
			name: "non-matching event type",
			event: &storage.ModerationEvent{
				EventType: "reported",
			},
			filter: &storage.ModerationEventFilter{
				EventType: "flagged",
			},
			expected: false,
		},
		{
			name: "matching category",
			event: &storage.ModerationEvent{
				Category: "spam",
			},
			filter: &storage.ModerationEventFilter{
				Category: "spam",
			},
			expected: true,
		},
		{
			name: "non-matching category",
			event: &storage.ModerationEvent{
				Category: "harassment",
			},
			filter: &storage.ModerationEventFilter{
				Category: "spam",
			},
			expected: false,
		},
		{
			name: "event meets min severity",
			event: &storage.ModerationEvent{
				Severity: "medium", // 2.0
			},
			filter: &storage.ModerationEventFilter{
				MinSeverity: &lowSeverity, // 1
			},
			expected: true,
		},
		{
			name: "event below min severity",
			event: &storage.ModerationEvent{
				Severity: "unknown", // defaults to 1.0
			},
			filter: &storage.ModerationEventFilter{
				MinSeverity: func() *int { v := 2; return &v }(),
			},
			expected: false,
		},
		{
			name: "event after start date",
			event: &storage.ModerationEvent{
				Created: now,
			},
			filter: &storage.ModerationEventFilter{
				StartDate: &yesterday,
			},
			expected: true,
		},
		{
			name: "event before start date",
			event: &storage.ModerationEvent{
				Created: yesterday,
			},
			filter: &storage.ModerationEventFilter{
				StartDate: &now,
			},
			expected: false,
		},
		{
			name: "event before end date",
			event: &storage.ModerationEvent{
				Created: now,
			},
			filter: &storage.ModerationEventFilter{
				EndDate: &tomorrow,
			},
			expected: true,
		},
		{
			name: "event after end date",
			event: &storage.ModerationEvent{
				Created: tomorrow,
			},
			filter: &storage.ModerationEventFilter{
				EndDate: &now,
			},
			expected: false,
		},
		{
			name: "all filter criteria match",
			event: &storage.ModerationEvent{
				EventType: "flagged",
				Category:  "spam",
				Severity:  "high",
				Created:   now,
			},
			filter: &storage.ModerationEventFilter{
				EventType:   "flagged",
				Category:    "spam",
				MinSeverity: &lowSeverity,
				StartDate:   &yesterday,
				EndDate:     &tomorrow,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := repo.matchesEventFilter(tt.event, tt.filter)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// ============================================
// Test modelToEvent
// ============================================

func TestModerationRepository_modelToEvent(t *testing.T) {
	repo := newMinimalModerationRepo()

	now := time.Now()

	tests := []struct {
		name  string
		model *models.ModerationEvent
		check func(t *testing.T, event *storage.ModerationEvent)
	}{
		{
			name: "complete model conversion",
			model: &models.ModerationEvent{
				ID:              "event-123",
				EventType:       "flagged",
				ObjectID:        "obj-456",
				ObjectType:      "status",
				ActorID:         "actor-789",
				Category:        "spam",
				Severity:        "high",
				ConfidenceScore: 0.95,
				Evidence:        []any{"suspicious content"},
				Reason:          "automated detection",
				Created:         now,
				Updated:         now,
				TTL:             12345678,
			},
			check: func(t *testing.T, event *storage.ModerationEvent) {
				assert.Equal(t, "event-123", event.ID)
				assert.Equal(t, "flagged", event.EventType)
				assert.Equal(t, "obj-456", event.ObjectID)
				assert.Equal(t, "status", event.ObjectType)
				assert.Equal(t, "actor-789", event.ActorID)
				assert.Equal(t, "spam", event.Category)
				assert.Equal(t, "high", event.Severity)
				assert.Equal(t, 0.95, event.ConfidenceScore)
				assert.Equal(t, []any{"suspicious content"}, event.Evidence)
				assert.Equal(t, "automated detection", event.Reason)
				assert.Equal(t, now, event.Created)
				assert.Equal(t, now, event.Updated)
				assert.Equal(t, int64(12345678), event.TTL)
			},
		},
		{
			name: "minimal model conversion",
			model: &models.ModerationEvent{
				ID:        "event-min",
				EventType: "reported",
			},
			check: func(t *testing.T, event *storage.ModerationEvent) {
				assert.Equal(t, "event-min", event.ID)
				assert.Equal(t, "reported", event.EventType)
				assert.Empty(t, event.ObjectID)
				assert.Empty(t, event.Category)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := repo.modelToEvent(tt.model)
			assert.NotNil(t, event)
			tt.check(t, event)
		})
	}
}

// ============================================
// Test processModelsToEvents
// ============================================

func TestModerationRepository_processModelsToEvents(t *testing.T) {
	repo := newMinimalModerationRepo()

	tests := []struct {
		name          string
		models        []models.ModerationEvent
		filter        *storage.ModerationEventFilter
		limit         int
		expectedCount int
		expectedIDs   []string
	}{
		{
			name:          "empty models returns empty events",
			models:        []models.ModerationEvent{},
			filter:        nil,
			limit:         10,
			expectedCount: 0,
		},
		{
			name: "filters out non-EVENT type",
			models: []models.ModerationEvent{
				{ID: "event-1", Type: ModerationTypeEvent, EventType: "flagged"},
				{ID: "event-2", Type: "REVIEW", EventType: "flagged"},
				{ID: "event-3", Type: ModerationTypeEvent, EventType: "flagged"},
			},
			filter:        nil,
			limit:         10,
			expectedCount: 2,
			expectedIDs:   []string{"event-1", "event-3"},
		},
		{
			name: "respects limit",
			models: []models.ModerationEvent{
				{ID: "event-1", Type: ModerationTypeEvent, EventType: "flagged"},
				{ID: "event-2", Type: ModerationTypeEvent, EventType: "flagged"},
				{ID: "event-3", Type: ModerationTypeEvent, EventType: "flagged"},
			},
			filter:        nil,
			limit:         2,
			expectedCount: 2,
			expectedIDs:   []string{"event-1", "event-2"},
		},
		{
			name: "applies filter",
			models: []models.ModerationEvent{
				{ID: "event-1", Type: ModerationTypeEvent, EventType: "flagged", Category: "spam"},
				{ID: "event-2", Type: ModerationTypeEvent, EventType: "flagged", Category: "harassment"},
				{ID: "event-3", Type: ModerationTypeEvent, EventType: "flagged", Category: "spam"},
			},
			filter: &storage.ModerationEventFilter{
				Category: "spam",
			},
			limit:         10,
			expectedCount: 2,
			expectedIDs:   []string{"event-1", "event-3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := repo.processModelsToEvents(tt.models, tt.filter, tt.limit)
			assert.Len(t, events, tt.expectedCount)
			for i, expectedID := range tt.expectedIDs {
				if i < len(events) {
					assert.Equal(t, expectedID, events[i].ID)
				}
			}
		})
	}
}

// ============================================
// Test convertReportModelToStorage
// ============================================

func TestModerationRepository_convertReportModelToStorage(t *testing.T) {
	repo := newMinimalModerationRepo()

	now := time.Now()

	tests := []struct {
		name  string
		model models.Report
		check func(t *testing.T, report *storage.Report)
	}{
		{
			name: "complete report conversion",
			model: models.Report{
				ID:              "report-123",
				ReporterID:      "reporter-456",
				TargetAccountID: "target-789",
				StatusIDs:       []string{"status-1", "status-2"},
				Comment:         "Spam content",
				Category:        "spam",
				RuleIDs:         []int{1, 2, 3},
				Forwarded:       true,
				Status:          string(storage.ReportStatusOpen),
				ActionTaken:     "warned",
				ActionTakenAt:   &now,
				ModeratorID:     "mod-123",
				CreatedAt:       now,
				UpdatedAt:       now,
				AssignedTo:      "admin-456",
			},
			check: func(t *testing.T, report *storage.Report) {
				assert.Equal(t, "report-123", report.ID)
				assert.Equal(t, "reporter-456", report.ReporterID)
				assert.Equal(t, "target-789", report.TargetAccountID)
				assert.Equal(t, []string{"status-1", "status-2"}, report.StatusIDs)
				assert.Equal(t, "Spam content", report.Comment)
				assert.Equal(t, "spam", report.Category)
				assert.Equal(t, []string{"1", "2", "3"}, report.RuleIDs) // Converted to strings
				assert.True(t, report.Forwarded)
				assert.Equal(t, string(storage.ReportStatusOpen), report.Status)
				assert.Equal(t, "warned", report.ActionTaken)
				assert.Equal(t, &now, report.ActionTakenAt)
				assert.Equal(t, "mod-123", report.ModeratorID)
				assert.Equal(t, "admin-456", report.AssignedTo)
			},
		},
		{
			name: "report with no rule IDs",
			model: models.Report{
				ID:       "report-min",
				RuleIDs:  nil,
				Category: "harassment",
			},
			check: func(t *testing.T, report *storage.Report) {
				assert.Equal(t, "report-min", report.ID)
				assert.Nil(t, report.RuleIDs)
				assert.Equal(t, "harassment", report.Category)
			},
		},
		{
			name: "report with empty rule IDs",
			model: models.Report{
				ID:      "report-empty-rules",
				RuleIDs: []int{},
			},
			check: func(t *testing.T, report *storage.Report) {
				assert.Equal(t, "report-empty-rules", report.ID)
				assert.Nil(t, report.RuleIDs) // Empty slice becomes nil after conversion
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := repo.convertReportModelToStorage(tt.model)
			assert.NotNil(t, report)
			tt.check(t, report)
		})
	}
}

// ============================================
// Test convertAuditLogModelToStorage
// ============================================

func TestModerationRepository_convertAuditLogModelToStorage(t *testing.T) {
	repo := newMinimalModerationRepo()

	now := time.Now()

	tests := []struct {
		name  string
		model models.AuditLog
		check func(t *testing.T, log *storage.AuditLog)
	}{
		{
			name: "complete audit log conversion",
			model: models.AuditLog{
				ID:         "log-123",
				AdminID:    "admin-456",
				AdminRole:  "super_admin",
				Action:     "suspend_user",
				TargetType: "user",
				TargetID:   "user-789",
				Reason:     "Violation of ToS",
				Details:    map[string]interface{}{"duration": "7d"},
				IPAddress:  "192.168.1.1",
				UserAgent:  "Mozilla/5.0",
				RequestID:  "req-abc123",
				Timestamp:  now,
				CreatedAt:  now,
			},
			check: func(t *testing.T, log *storage.AuditLog) {
				assert.Equal(t, "log-123", log.ID)
				assert.Equal(t, "admin-456", log.AdminID)
				assert.Equal(t, "super_admin", log.AdminRole)
				assert.Equal(t, "suspend_user", log.Action)
				assert.Equal(t, "user", log.TargetType)
				assert.Equal(t, "user-789", log.TargetID)
				assert.Equal(t, "Violation of ToS", log.Reason)
				assert.Equal(t, map[string]interface{}{"duration": "7d"}, log.Details)
				assert.Equal(t, "192.168.1.1", log.IPAddress)
				assert.Equal(t, "Mozilla/5.0", log.UserAgent)
				assert.Equal(t, "req-abc123", log.RequestID)
				assert.Equal(t, now, log.Timestamp)
				assert.Equal(t, now, log.CreatedAt)
			},
		},
		{
			name: "minimal audit log conversion",
			model: models.AuditLog{
				ID:      "log-min",
				AdminID: "admin-min",
				Action:  "view",
			},
			check: func(t *testing.T, log *storage.AuditLog) {
				assert.Equal(t, "log-min", log.ID)
				assert.Equal(t, "admin-min", log.AdminID)
				assert.Equal(t, "view", log.Action)
				assert.Empty(t, log.TargetType)
				assert.Nil(t, log.Details)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := repo.convertAuditLogModelToStorage(tt.model)
			assert.NotNil(t, log)
			tt.check(t, log)
		})
	}
}
