package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// AuditRepository handles audit log storage operations
type AuditRepository struct {
	db     core.DB
	logger *zap.Logger
}

// NewAuditRepository creates a new audit repository
func NewAuditRepository(db core.DB, logger *zap.Logger) *AuditRepository {
	return &AuditRepository{
		db:     db,
		logger: logger,
	}
}

// StoreAuditLog stores an audit log entry
func (r *AuditRepository) StoreAuditLog(ctx context.Context, log *models.AuthAuditLog) error {
	// Update keys before saving
	log.UpdateKeys()

	// Save to DynamoDB
	if err := r.db.WithContext(ctx).Model(log).Create(); err != nil {
		r.logger.Error("failed to store audit log",
			zap.String("event_id", log.ID),
			zap.String("event_type", log.EventType),
			zap.Error(err))
		return fmt.Errorf("failed to store audit log: %w", err)
	}

	return nil
}

// GetAuditLogByID retrieves an audit log by ID and date
func (r *AuditRepository) GetAuditLogByID(ctx context.Context, id string, date time.Time) (*models.AuthAuditLog, error) {
	var log models.AuthAuditLog

	pk := fmt.Sprintf("AUDIT#%s", date.Format("2006-01-02"))
	// We need to search within the date partition for the specific ID

	err := r.db.WithContext(ctx).Model(&models.AuthAuditLog{}).
		Where("PK", "=", pk).
		Where("SK", "BEGINS_WITH", fmt.Sprintf("EVENT#%s", id)).
		First(&log)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get audit log: %w", err)
	}

	return &log, nil
}

// GetUserAuditLogs retrieves audit logs for a specific user
func (r *AuditRepository) GetUserAuditLogs(ctx context.Context, username string, limit int, startTime, endTime time.Time) ([]*models.AuthAuditLog, error) {
	return AuditLogQueryHelper(r.db, ctx, "GSI1", fmt.Sprintf("USER#%s", username), limit, startTime, endTime, "user")
}

// GetIPAuditLogs retrieves audit logs for a specific IP address
func (r *AuditRepository) GetIPAuditLogs(ctx context.Context, ipAddress string, limit int, startTime, endTime time.Time) ([]*models.AuthAuditLog, error) {
	return AuditLogQueryHelper(r.db, ctx, "GSI2", fmt.Sprintf("IP#%s", ipAddress), limit, startTime, endTime, "IP")
}

// GetSessionAuditLogs retrieves audit logs for a specific session
func (r *AuditRepository) GetSessionAuditLogs(ctx context.Context, sessionID string) ([]*models.AuthAuditLog, error) {
	var logs []*models.AuthAuditLog

	err := r.db.WithContext(ctx).Model(&models.AuthAuditLog{}).
		Index("GSI3").
		Where("GSI3PK", "=", fmt.Sprintf("SESSION#%s", sessionID)).
		All(&logs)

	if err != nil {
		return nil, fmt.Errorf("failed to get session audit logs: %w", err)
	}

	return logs, nil
}

// GetSecurityEvents retrieves security events by severity within a time range
func (r *AuditRepository) GetSecurityEvents(ctx context.Context, severity string, startTime, endTime time.Time, limit int) ([]*models.AuthAuditLog, error) {
	var logs []*models.AuthAuditLog

	query := r.db.WithContext(ctx).Model(&models.AuthAuditLog{}).
		Index("GSI4").
		Where("GSI4PK", "=", fmt.Sprintf("SEVERITY#%s", severity))

	// Add time range filter
	if !startTime.IsZero() && !endTime.IsZero() {
		startTimestamp := fmt.Sprintf("AUDIT#%d", startTime.Unix())
		endTimestamp := fmt.Sprintf("AUDIT#%d", endTime.Unix())
		query = query.Where("GSI4SK", ">=", startTimestamp).Where("GSI4SK", "<=", endTimestamp)
	}

	// Apply limit
	if limit > 0 {
		query = query.Limit(limit)
	}

	// Execute query
	if err := query.All(&logs); err != nil {
		return nil, fmt.Errorf("failed to get security events: %w", err)
	}

	return logs, nil
}

// GetRecentFailedLogins gets recent failed login attempts for a user
func (r *AuditRepository) GetRecentFailedLogins(ctx context.Context, username string, duration time.Duration) (int, error) {
	startTime := time.Now().Add(-duration)
	endTime := time.Now()

	logs, err := r.GetUserAuditLogs(ctx, username, 100, startTime, endTime)
	if err != nil {
		return 0, err
	}

	// Count failed login events
	count := 0
	for _, log := range logs {
		if log.EventType == "auth.login.failed" && !log.Success {
			count++
		}
	}

	return count, nil
}

// GetRecentIPFailures gets recent failures from an IP address
func (r *AuditRepository) GetRecentIPFailures(ctx context.Context, ipAddress string, duration time.Duration) (int, error) {
	startTime := time.Now().Add(-duration)
	endTime := time.Now()

	logs, err := r.GetIPAuditLogs(ctx, ipAddress, 100, startTime, endTime)
	if err != nil {
		return 0, err
	}

	// Count failed events
	count := 0
	for _, log := range logs {
		if !log.Success {
			count++
		}
	}

	return count, nil
}

// CleanupOldLogs removes audit logs older than retention period
// Note: With TTL properly set, DynamoDB will handle this automatically
func (r *AuditRepository) CleanupOldLogs(ctx context.Context, retentionDays int) error {
	// TTL is handled by DynamoDB automatically
	// This method is here for manual cleanup if needed

	cutoffDate := time.Now().AddDate(0, 0, -retentionDays)

	// Query old logs by date partition
	for i := 0; i < retentionDays; i++ {
		date := cutoffDate.AddDate(0, 0, -i)
		pk := fmt.Sprintf("AUDIT#%s", date.Format("2006-01-02"))

		// Delete all items with this PK
		// Note: This would need to be done in batches for large datasets
		var logs []*models.AuthAuditLog
		err := r.db.WithContext(ctx).Model(&models.AuthAuditLog{}).
			Where("PK", "=", pk).
			All(&logs)

		if err != nil {
			continue // Skip if no logs for this date
		}

		// Delete each log
		for _, log := range logs {
			if err := r.db.WithContext(ctx).Model(log).Delete(); err != nil {
				r.logger.Warn("failed to delete old audit log",
					zap.String("pk", log.PK),
					zap.String("sk", log.SK),
					zap.Error(err))
			}
		}
	}

	return nil
}

// StoreAuditEvent stores an audit event with full metadata
func (r *AuditRepository) StoreAuditEvent(ctx context.Context, eventType, severity, username, userID, ipAddress, userAgent, deviceName, sessionID, requestID string, success bool, failureReason string, metadata map[string]interface{}) error {
	// Convert metadata to JSON string
	var metadataStr string
	if metadata != nil {
		data, err := json.Marshal(metadata)
		if err != nil {
			r.logger.Warn("failed to marshal audit metadata", zap.Error(err))
		} else {
			metadataStr = string(data)
		}
	}

	// Generate unique ID
	id := fmt.Sprintf("%d-%s", time.Now().UnixNano(), generateAuditID(8))

	log := &models.AuthAuditLog{
		ID:                id,
		Timestamp:         time.Now().UTC(),
		EventType:         eventType,
		Severity:          severity,
		Username:          username,
		UserID:            userID,
		IPAddress:         ipAddress,
		UserAgent:         userAgent,
		DeviceName:        deviceName,
		SessionID:         sessionID,
		RequestID:         requestID,
		Success:           success,
		FailureReason:     failureReason,
		Metadata:          metadataStr,
		DataRetentionDays: 90, // Default retention
	}

	return r.StoreAuditLog(ctx, log)
}

// generateAuditID generates a random ID for audit logs
func generateAuditID(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}
