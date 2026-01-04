// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces //nolint:revive // Standard interfaces package name

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// ModerationRepository defines the interface for moderation operations.
// This handles moderation events, reviews, decisions, patterns, reports, flags, filters, and audit logs.
type ModerationRepository interface {
	// ===== Moderation Event Operations =====

	// CreateModerationEvent creates a new moderation event
	CreateModerationEvent(ctx context.Context, event *storage.ModerationEvent) error

	// GetModerationEvent retrieves a moderation event by ID
	GetModerationEvent(ctx context.Context, eventID string) (*storage.ModerationEvent, error)

	// GetModerationEvents retrieves moderation events with optional filters
	GetModerationEvents(ctx context.Context, filter *storage.ModerationEventFilter, limit int, cursor string) ([]*storage.ModerationEvent, string, error)

	// GetModerationEventsByObject retrieves all moderation events for an object
	GetModerationEventsByObject(ctx context.Context, objectID string, limit int, cursor string) ([]*storage.ModerationEvent, string, error)

	// GetModerationEventsByActor retrieves all moderation events created by an actor
	GetModerationEventsByActor(ctx context.Context, actorID string, limit int, cursor string) ([]*storage.ModerationEvent, string, error)

	// ===== Moderation Queue Operations =====

	// GetModerationQueue retrieves pending moderation events
	GetModerationQueue(ctx context.Context, filter *storage.ModerationFilter) ([]*storage.ModerationQueueItem, error)

	// GetModerationQueuePaginated retrieves pending moderation events with pagination
	GetModerationQueuePaginated(ctx context.Context, limit int, cursor string) ([]*storage.ModerationQueueItem, string, error)

	// GetModerationQueueCount returns the count of items in the moderation queue
	GetModerationQueueCount(ctx context.Context) (int, error)

	// ===== Moderation Review Operations =====

	// AddModerationReview adds a review to a moderation event
	AddModerationReview(ctx context.Context, review *storage.ModerationReview) error

	// GetModerationReviews retrieves all reviews for a moderation event
	GetModerationReviews(ctx context.Context, eventID string) ([]*storage.ModerationReview, error)

	// CreateAdminReview creates an admin review that overrides consensus
	CreateAdminReview(ctx context.Context, eventID string, adminID string, action storage.ActionType, reason string) error

	// GetReviewerStats retrieves statistics for a reviewer
	GetReviewerStats(ctx context.Context, reviewerID string) (*storage.ReviewerStats, error)

	// ===== Moderation Decision Operations =====

	// CreateModerationDecision creates a consensus decision
	CreateModerationDecision(ctx context.Context, decision *storage.ModerationDecision) error

	// GetModerationDecision retrieves the current decision for an object
	GetModerationDecision(ctx context.Context, objectID string) (*storage.ModerationDecision, error)

	// StoreModerationDecision stores a moderation decision (alias for CreateModerationDecision)
	StoreModerationDecision(ctx context.Context, decision *storage.ModerationDecision) error

	// UpdateModerationDecision updates a moderation decision based on a review
	UpdateModerationDecision(ctx context.Context, contentID string, review *storage.ModerationReview) error

	// ===== Moderation Pattern Operations =====

	// CreateModerationPattern creates a new moderation pattern
	CreateModerationPattern(ctx context.Context, pattern *storage.ModerationPattern) error

	// GetModerationPattern retrieves a specific moderation pattern
	GetModerationPattern(ctx context.Context, patternID string) (*storage.ModerationPattern, error)

	// GetModerationPatterns retrieves moderation patterns based on criteria
	GetModerationPatterns(ctx context.Context, active bool, severity string, limit int) ([]*storage.ModerationPattern, error)

	// UpdateModerationPattern updates an existing moderation pattern
	UpdateModerationPattern(ctx context.Context, pattern *storage.ModerationPattern) error

	// DeleteModerationPattern deletes a moderation pattern
	DeleteModerationPattern(ctx context.Context, patternID string) error

	// RecordPatternMatch records a moderation pattern match for analytics
	RecordPatternMatch(ctx context.Context, patternID string, matched bool, timestamp time.Time) error

	// ===== Moderation History Operations =====

	// GetModerationHistory retrieves the complete moderation history for an object
	GetModerationHistory(ctx context.Context, objectID string) (*storage.ModerationHistory, error)

	// ===== Filter Operations =====

	// CreateFilter creates a new filter
	CreateFilter(ctx context.Context, filter *storage.Filter) error

	// GetFilter retrieves a filter by ID
	GetFilter(ctx context.Context, filterID string) (*storage.Filter, error)

	// GetFiltersForUser retrieves all filters for a user
	GetFiltersForUser(ctx context.Context, username string) ([]*storage.Filter, error)

	// UpdateFilter updates a filter
	UpdateFilter(ctx context.Context, filterID string, updates map[string]any) error

	// DeleteFilter deletes a filter and all its associated keywords and statuses
	DeleteFilter(ctx context.Context, filterID string) error

	// AddFilterKeyword adds a new keyword to a filter
	AddFilterKeyword(ctx context.Context, filterID string, keyword *storage.FilterKeyword) error

	// GetFilterKeywords retrieves all keywords for a filter
	GetFilterKeywords(ctx context.Context, filterID string) ([]*storage.FilterKeyword, error)

	// UpdateFilterKeyword updates a filter keyword
	UpdateFilterKeyword(ctx context.Context, keywordID string, updates map[string]any) error

	// DeleteFilterKeyword deletes a filter keyword
	DeleteFilterKeyword(ctx context.Context, keywordID string) error

	// AddFilterStatus adds a new status to a filter
	AddFilterStatus(ctx context.Context, filterID string, status *storage.FilterStatus) error

	// GetFilterStatuses retrieves all statuses for a filter
	GetFilterStatuses(ctx context.Context, filterID string) ([]*storage.FilterStatus, error)

	// DeleteFilterStatus deletes a filter status
	DeleteFilterStatus(ctx context.Context, statusID string) error

	// ===== Report Operations =====

	// CreateReport creates a new report
	CreateReport(ctx context.Context, report *storage.Report) error

	// GetReport retrieves a report by ID
	GetReport(ctx context.Context, id string) (*storage.Report, error)

	// GetUserReports retrieves all reports created by a user
	GetUserReports(ctx context.Context, username string, limit int, cursor string) ([]*storage.Report, string, error)

	// GetReportsByTarget retrieves reports targeting a specific account
	GetReportsByTarget(ctx context.Context, targetAccountID string, limit int, cursor string) ([]*storage.Report, string, error)

	// GetReportsByStatus retrieves reports with a specific status
	GetReportsByStatus(ctx context.Context, status storage.ReportStatus, limit int, cursor string) ([]*storage.Report, string, error)

	// UpdateReportStatus updates the status of a report
	UpdateReportStatus(ctx context.Context, id string, status storage.ReportStatus, actionTaken string, moderatorID string) error

	// AssignReport assigns a report to a moderator
	AssignReport(ctx context.Context, reportID string, assignedTo string) error

	// UnassignReport removes assignment from a report
	UnassignReport(ctx context.Context, reportID string) error

	// GetOpenReportsCount returns the count of open reports
	GetOpenReportsCount(ctx context.Context) (int, error)

	// GetReportedStatuses retrieves statuses associated with a report
	GetReportedStatuses(ctx context.Context, reportID string) ([]any, error)

	// GetReportStats retrieves reporting statistics for a user
	GetReportStats(ctx context.Context, username string) (*storage.ReportStats, error)

	// IncrementFalseReports increments the false report count for a user
	IncrementFalseReports(ctx context.Context, username string) error

	// ===== Flag Operations =====

	// CreateFlag creates a new flag
	CreateFlag(ctx context.Context, flag *storage.Flag) error

	// GetFlag retrieves a flag by ID
	GetFlag(ctx context.Context, id string) (*storage.Flag, error)

	// GetFlagsByObject retrieves all flags for a specific object
	GetFlagsByObject(ctx context.Context, objectID string, limit int, cursor string) ([]*storage.Flag, string, error)

	// GetFlagsByActor retrieves all flags created by a specific actor
	GetFlagsByActor(ctx context.Context, actorID string, limit int, cursor string) ([]*storage.Flag, string, error)

	// GetPendingFlags retrieves all pending flags
	GetPendingFlags(ctx context.Context, limit int, cursor string) ([]*storage.Flag, string, error)

	// UpdateFlagStatus updates the status of a flag
	UpdateFlagStatus(ctx context.Context, id string, status storage.FlagStatus, reviewedBy string, reviewNote string) error

	// CountPendingFlags returns the count of pending flags
	CountPendingFlags(ctx context.Context) (int, error)

	// DeleteFlag removes a flag
	DeleteFlag(ctx context.Context, id string) error

	// ===== Audit Log Operations =====

	// CreateAuditLog creates a new audit log entry
	CreateAuditLog(ctx context.Context, auditLog *storage.AuditLog) error

	// GetAuditLogs retrieves audit log entries with pagination
	GetAuditLogs(ctx context.Context, limit int, cursor string) ([]*storage.AuditLog, string, error)

	// GetAuditLogsByAdmin retrieves audit log entries for a specific admin
	GetAuditLogsByAdmin(ctx context.Context, adminID string, limit int, cursor string) ([]*storage.AuditLog, string, error)

	// GetAuditLogsByTarget retrieves audit log entries for a specific target
	GetAuditLogsByTarget(ctx context.Context, targetID string, limit int, cursor string) ([]*storage.AuditLog, string, error)

	// ===== Pending Moderation Operations =====

	// GetPendingModerationCount returns the count of pending moderation tasks for a specific moderator
	GetPendingModerationCount(ctx context.Context, moderatorID string) (int, error)

	// ===== Analysis and Decision Storage Operations =====

	// StoreAnalysisResult stores detailed analysis results for audit/appeals
	StoreAnalysisResult(ctx context.Context, analysisData map[string]interface{}) error

	// StoreDecision stores a moderation decision with enforcement tracking
	StoreDecision(ctx context.Context, decisionData map[string]interface{}) error

	// GetReviewQueue retrieves review queue items with filtering
	GetReviewQueue(ctx context.Context, filters map[string]interface{}) ([]*models.ModerationReviewQueue, error)

	// GetDecisionHistory retrieves decision history for a specific content ID
	GetDecisionHistory(ctx context.Context, contentID string) ([]*models.ModerationDecisionResult, error)

	// UpdateEnforcementStatus updates the enforcement status of a decision
	UpdateEnforcementStatus(ctx context.Context, contentID, status string) error

	// GetModerationDecisionsByModerator retrieves moderation decisions made by a specific moderator
	GetModerationDecisionsByModerator(ctx context.Context, moderatorUsername string, limit int) ([]*models.ModerationReview, error)
}
