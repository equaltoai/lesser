// Package interfaces defines the repository interfaces for the Lesser application.
package interfaces

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

// AuditRepository defines the interface for audit log operations.
// This handles authentication audit logging with enhanced security features.
type AuditRepository interface {
	// ===== Core Audit Operations =====

	// StoreAuditLog stores an audit log entry
	StoreAuditLog(ctx context.Context, log *models.AuthAuditLog) error

	// GetAuditLogByID retrieves an audit log by ID and date
	GetAuditLogByID(ctx context.Context, id string, date time.Time) (*models.AuthAuditLog, error)

	// ===== Query Operations =====

	// GetUserAuditLogs retrieves audit logs for a specific user
	GetUserAuditLogs(ctx context.Context, username string, limit int, startTime, endTime time.Time) ([]*models.AuthAuditLog, error)

	// GetIPAuditLogs retrieves audit logs for a specific IP address
	GetIPAuditLogs(ctx context.Context, ipAddress string, limit int, startTime, endTime time.Time) ([]*models.AuthAuditLog, error)

	// GetSessionAuditLogs retrieves audit logs for a specific session
	GetSessionAuditLogs(ctx context.Context, sessionID string) ([]*models.AuthAuditLog, error)

	// GetSecurityEvents retrieves security events by severity within a time range
	GetSecurityEvents(ctx context.Context, severity string, startTime, endTime time.Time, limit int, cursor string) ([]*models.AuthAuditLog, string, error)

	// ===== Analytics Operations =====

	// GetRecentFailedLogins gets recent failed login attempts for a user
	GetRecentFailedLogins(ctx context.Context, username string, duration time.Duration) (int, error)

	// GetRecentIPFailures gets recent failures from an IP address
	GetRecentIPFailures(ctx context.Context, ipAddress string, duration time.Duration) (int, error)

	// ===== Cleanup Operations =====

	// CleanupOldLogs removes audit logs older than retention period
	CleanupOldLogs(ctx context.Context, retentionDays int) error

	// ===== Event Storage =====

	// StoreAuditEvent stores an audit event with full metadata
	StoreAuditEvent(ctx context.Context, eventType, severity, username, userID, ipAddress, userAgent, deviceName, sessionID, requestID string, success bool, failureReason string, metadata map[string]interface{}) error
}
