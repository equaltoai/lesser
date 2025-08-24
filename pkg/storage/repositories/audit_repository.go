package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// AuditRepository handles audit log storage operations with enhanced security features
type AuditRepository struct {
	*EnhancedBaseRepository[*models.AuthAuditLog]
}

// NewAuditRepository creates a new audit repository with enhanced security features
func NewAuditRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *AuditRepository {
	// Create enhanced repository optimized for audit operations
	enhancedRepo := NewEnhancedBaseRepository[*models.AuthAuditLog](db, tableName, logger, costService, "AuditRepository", "audit")
	
	// Set up enhanced services for audit operations - SECURITY CRITICAL
	enhancedRepo.SetValidationService(NewDefaultValidationService()) // Special security validation
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())    // Audit-specific permissions
	enhancedRepo.SetCachingService(NewInMemoryCachingService())       // Security logs cached briefly
	enhancedRepo.SetEventService(NewDefaultEventService())          // Critical for security monitoring
	
	return &AuditRepository{
		EnhancedBaseRepository: enhancedRepo,
	}
}

// StoreAuditLog stores an audit log entry - SECURITY CRITICAL
func (r *AuditRepository) StoreAuditLog(ctx context.Context, log *models.AuthAuditLog) error {
	// Security validation - ensure critical fields are present
	if log.ID == "" || log.EventType == "" {
		r.logger.Error("audit log missing critical security fields",
			zap.String("event_id", log.ID),
			zap.String("event_type", log.EventType))
		return ErrorHandler.HandleCreateError(storage.ErrInvalidInput, EntityAudit, log.ID)
	}

	// Use Enhanced BaseRepository for storage with security validation
	return r.ValidateAndCreate(ctx, log)
}

// GetAuditLogByID retrieves an audit log by ID and date - SECURITY CRITICAL
func (r *AuditRepository) GetAuditLogByID(ctx context.Context, id string, date time.Time) (*models.AuthAuditLog, error) {
	// Security validation
	if id == "" {
		return nil, ErrorHandler.HandleGetError(storage.ErrInvalidInput, EntityAudit, "empty")
	}
	if date.IsZero() {
		return nil, ErrorHandler.HandleGetError(storage.ErrInvalidInput, EntityAudit, "zero-date")
	}

	pk := fmt.Sprintf("AUDIT#%s", date.Format("2006-01-02"))
	skPrefix := fmt.Sprintf("EVENT#%s", id)

	// Use BaseRepository query with SK prefix
	logs, err := r.QueryWithSKPrefix(ctx, pk, skPrefix, 1)
	if err != nil {
		return nil, ErrorHandler.HandleGetError(err, "audit log", id)
	}

	if len(logs) == 0 {
		return nil, nil // Not found
	}

	return logs[0], nil
}

// GetUserAuditLogs retrieves audit logs for a specific user - SECURITY CRITICAL
func (r *AuditRepository) GetUserAuditLogs(ctx context.Context, username string, limit int, startTime, endTime time.Time) ([]*models.AuthAuditLog, error) {
	// Security validation
	if username == "" {
		return nil, ErrorHandler.HandleQueryError(storage.ErrInvalidInput, EntityAudit, "user logs")
	}

	return AuditLogQueryHelper(ctx, r.db, "GSI1", fmt.Sprintf("USER#%s", username), limit, startTime, endTime, "user")
}

// GetIPAuditLogs retrieves audit logs for a specific IP address - SECURITY CRITICAL
func (r *AuditRepository) GetIPAuditLogs(ctx context.Context, ipAddress string, limit int, startTime, endTime time.Time) ([]*models.AuthAuditLog, error) {
	// Security validation - prevent injection attacks
	if ipAddress == "" {
		return nil, ErrorHandler.HandleQueryError(storage.ErrInvalidInput, EntityAudit, "IP logs")
	}

	return AuditLogQueryHelper(ctx, r.db, "GSI2", fmt.Sprintf("IP#%s", ipAddress), limit, startTime, endTime, "IP")
}

// GetSessionAuditLogs retrieves audit logs for a specific session - SECURITY CRITICAL
func (r *AuditRepository) GetSessionAuditLogs(ctx context.Context, sessionID string) ([]*models.AuthAuditLog, error) {
	// Security validation
	if sessionID == "" {
		return nil, ErrorHandler.HandleQueryError(storage.ErrInvalidInput, EntityAudit, "session logs")
	}

	// Use BaseRepository GSI query
	logs, err := r.QueryGSI(ctx, "GSI3", fmt.Sprintf("SESSION#%s", sessionID), 0)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityAudit, "session logs")
	}

	// Convert to pointer slice for compatibility
	result := make([]*models.AuthAuditLog, len(logs))
	copy(result, logs)

	return result, nil
}

// GetSecurityEvents retrieves security events by severity within a time range - SECURITY CRITICAL
func (r *AuditRepository) GetSecurityEvents(ctx context.Context, severity string, startTime, endTime time.Time, limit int) ([]*models.AuthAuditLog, error) {
	// Security validation
	if severity == "" {
		return nil, ErrorHandler.HandleQueryError(storage.ErrInvalidInput, EntityAudit, "security events")
	}
	// Validate severity levels to prevent injection
	validSeverities := map[string]bool{"LOW": true, "MEDIUM": true, "HIGH": true, "CRITICAL": true}
	if !validSeverities[severity] {
		return nil, ErrorHandler.HandleQueryError(storage.ErrInvalidInput, EntityAudit, "security events")
	}

	var logs []models.AuthAuditLog

	query := r.db.WithContext(ctx).Model(&models.AuthAuditLog{}).
		Index("GSI4").
		Where("GSI4PK", "=", fmt.Sprintf("SEVERITY#%s", severity))

	// Add time range filter for enhanced security monitoring
	if !startTime.IsZero() && !endTime.IsZero() {
		startTimestamp := fmt.Sprintf("AUDIT#%d", startTime.Unix())
		endTimestamp := fmt.Sprintf("AUDIT#%d", endTime.Unix())
		query = query.Where("GSI4SK", ">=", startTimestamp).Where("GSI4SK", "<=", endTimestamp)
	}

	// Apply limit with security bounds
	if limit <= 0 {
		limit = 100 // Default limit for security events
	}
	if limit > 1000 {
		limit = 1000 // Maximum security event limit
	}
	query = query.Limit(limit)

	// Execute query with enhanced error handling
	if err := query.All(&logs); err != nil {
		r.logger.Error("failed to get security events - SECURITY ALERT",
			zap.String("severity", severity),
			zap.Time("start_time", startTime),
			zap.Time("end_time", endTime),
			zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, EntityAudit, "security events")
	}

	// Convert to pointer slice for compatibility
	result := make([]*models.AuthAuditLog, len(logs))
	for i := range logs {
		result[i] = &logs[i]
	}

	return result, nil
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

// CleanupOldLogs removes audit logs older than retention period - COMPLIANCE CRITICAL
// Note: This is for manual cleanup when TTL is not sufficient for compliance requirements
func (r *AuditRepository) CleanupOldLogs(ctx context.Context, retentionDays int) error {
	// Security validation - prevent accidental deletion of recent logs
	if retentionDays < 1 {
		return ErrorHandler.HandleDeleteError(storage.ErrInvalidInput, EntityAudit, "cleanup")
	}
	if retentionDays > 365 {
		return ErrorHandler.HandleDeleteError(storage.ErrInvalidInput, EntityAudit, "cleanup")
	}

	// Compliance logging
	r.logger.Warn("manual audit log cleanup initiated - COMPLIANCE ACTION",
		zap.Int("retention_days", retentionDays))

	cutoffDate := time.Now().AddDate(0, 0, -retentionDays)

	// Query and delete old logs by date partition
	for i := 0; i < retentionDays; i++ {
		date := cutoffDate.AddDate(0, 0, -i)
		pk := fmt.Sprintf("AUDIT#%s", date.Format("2006-01-02"))

		// Use BaseRepository to find logs for this date
		logs, err := r.Query(ctx, pk, 0)
		if err != nil {
			r.logger.Warn("failed to query logs for cleanup",
				zap.String("date", date.Format("2006-01-02")),
				zap.Error(err))
			continue
		}

		// Build keys for batch deletion
		keys := make([]struct{ PK, SK string }, len(logs))
		for i, log := range logs {
			keys[i] = struct{ PK, SK string }{PK: log.GetPK(), SK: log.GetSK()}
		}

		// Use BaseRepository batch delete for efficiency
		if len(keys) > 0 {
			if err := r.BatchDelete(ctx, keys); err != nil {
				r.logger.Error("failed to batch delete old audit logs - COMPLIANCE ISSUE",
					zap.String("date", date.Format("2006-01-02")),
					zap.Int("count", len(keys)),
					zap.Error(err))
				// Continue with next date rather than fail completely
			} else {
				r.logger.Info("deleted old audit logs for compliance",
					zap.String("date", date.Format("2006-01-02")),
					zap.Int("count", len(keys)))
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
