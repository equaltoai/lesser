// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/google/uuid"
)

// AuditRepository is a thread-safe in-memory implementation of interfaces.AuditRepository.
type AuditRepository struct {
	mu sync.RWMutex

	// Logs by ID: id -> log
	logsByID map[string]*models.AuthAuditLog

	// Logs by date: date (YYYY-MM-DD) -> []logs
	logsByDate map[string][]*models.AuthAuditLog

	// Logs by user: username -> []logs
	logsByUser map[string][]*models.AuthAuditLog

	// Logs by IP: ipAddress -> []logs
	logsByIP map[string][]*models.AuthAuditLog

	// Logs by session: sessionID -> []logs
	logsBySession map[string][]*models.AuthAuditLog

	// Logs by severity: severity -> []logs
	logsBySeverity map[string][]*models.AuthAuditLog
}

// NewAuditRepository creates a new in-memory audit repository
func NewAuditRepository() *AuditRepository {
	return &AuditRepository{
		logsByID:       make(map[string]*models.AuthAuditLog),
		logsByDate:     make(map[string][]*models.AuthAuditLog),
		logsByUser:     make(map[string][]*models.AuthAuditLog),
		logsByIP:       make(map[string][]*models.AuthAuditLog),
		logsBySession:  make(map[string][]*models.AuthAuditLog),
		logsBySeverity: make(map[string][]*models.AuthAuditLog),
	}
}

// StoreAuditLog stores an audit log entry
func (r *AuditRepository) StoreAuditLog(_ context.Context, log *models.AuthAuditLog) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if log == nil || log.ID == "" {
		return fmt.Errorf("audit log ID is required")
	}

	// Store by ID
	r.logsByID[log.ID] = log

	// Store by date
	dateKey := log.Timestamp.Format("2006-01-02")
	r.logsByDate[dateKey] = append(r.logsByDate[dateKey], log)

	// Store by user
	if log.Username != "" {
		r.logsByUser[log.Username] = append(r.logsByUser[log.Username], log)
	}

	// Store by IP
	if log.IPAddress != "" {
		r.logsByIP[log.IPAddress] = append(r.logsByIP[log.IPAddress], log)
	}

	// Store by session
	if log.SessionID != "" {
		r.logsBySession[log.SessionID] = append(r.logsBySession[log.SessionID], log)
	}

	// Store by severity
	if log.Severity != "" {
		r.logsBySeverity[log.Severity] = append(r.logsBySeverity[log.Severity], log)
	}

	return nil
}

// GetAuditLogByID retrieves an audit log by ID and date
func (r *AuditRepository) GetAuditLogByID(_ context.Context, id string, _ time.Time) (*models.AuthAuditLog, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	log, exists := r.logsByID[id]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return log, nil
}

// GetUserAuditLogs retrieves audit logs for a specific user
func (r *AuditRepository) GetUserAuditLogs(_ context.Context, username string, limit int, startTime, endTime time.Time) ([]*models.AuthAuditLog, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	logs := r.logsByUser[username]
	return r.filterLogsByTimeRange(logs, limit, startTime, endTime), nil
}

// GetIPAuditLogs retrieves audit logs for a specific IP address
func (r *AuditRepository) GetIPAuditLogs(_ context.Context, ipAddress string, limit int, startTime, endTime time.Time) ([]*models.AuthAuditLog, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	logs := r.logsByIP[ipAddress]
	return r.filterLogsByTimeRange(logs, limit, startTime, endTime), nil
}

// GetSessionAuditLogs retrieves audit logs for a specific session
func (r *AuditRepository) GetSessionAuditLogs(_ context.Context, sessionID string) ([]*models.AuthAuditLog, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	logs := r.logsBySession[sessionID]
	result := make([]*models.AuthAuditLog, len(logs))
	copy(result, logs)
	return result, nil
}

// GetSecurityEvents retrieves security events by severity within a time range
func (r *AuditRepository) GetSecurityEvents(_ context.Context, severity string, startTime, endTime time.Time, limit int, cursor string) ([]*models.AuthAuditLog, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	logs := r.logsBySeverity[severity]
	filtered := r.filterLogsByTimeRange(logs, 0, startTime, endTime)

	// Sort by timestamp
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Timestamp.Before(filtered[j].Timestamp)
	})

	// Apply cursor
	startIdx := 0
	if cursor != "" {
		for i, log := range filtered {
			if log.GSI4SK > cursor {
				startIdx = i
				break
			}
		}
	}

	// Apply limit
	if limit <= 0 {
		limit = 100
	}
	endIdx := startIdx + limit
	if endIdx > len(filtered) {
		endIdx = len(filtered)
	}

	result := filtered[startIdx:endIdx]
	nextCursor := ""
	if endIdx < len(filtered) && len(result) > 0 {
		nextCursor = result[len(result)-1].GSI4SK
	}

	return result, nextCursor, nil
}

// GetRecentFailedLogins gets recent failed login attempts for a user
func (r *AuditRepository) GetRecentFailedLogins(_ context.Context, username string, duration time.Duration) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	startTime := time.Now().Add(-duration)
	logs := r.logsByUser[username]

	count := 0
	for _, log := range logs {
		if log.Timestamp.After(startTime) && log.EventType == "auth.login.failed" && !log.Success {
			count++
		}
	}

	return count, nil
}

// GetRecentIPFailures gets recent failures from an IP address
func (r *AuditRepository) GetRecentIPFailures(_ context.Context, ipAddress string, duration time.Duration) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	startTime := time.Now().Add(-duration)
	logs := r.logsByIP[ipAddress]

	count := 0
	for _, log := range logs {
		if log.Timestamp.After(startTime) && !log.Success {
			count++
		}
	}

	return count, nil
}

// CleanupOldLogs removes audit logs older than retention period
func (r *AuditRepository) CleanupOldLogs(_ context.Context, retentionDays int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	cutoff := time.Now().AddDate(0, 0, -retentionDays)

	// Remove old logs from all indexes
	for id, log := range r.logsByID {
		if log.Timestamp.Before(cutoff) {
			delete(r.logsByID, id)
		}
	}

	// Clean up other indexes
	r.cleanupIndex(r.logsByDate, cutoff)
	r.cleanupIndex(r.logsByUser, cutoff)
	r.cleanupIndex(r.logsByIP, cutoff)
	r.cleanupIndex(r.logsBySession, cutoff)
	r.cleanupIndex(r.logsBySeverity, cutoff)

	return nil
}

// StoreAuditEvent stores an audit event with full metadata
func (r *AuditRepository) StoreAuditEvent(ctx context.Context, eventType, severity, username, userID, ipAddress, userAgent, deviceName, sessionID, requestID string, success bool, failureReason string, _ map[string]interface{}) error {
	log := &models.AuthAuditLog{
		ID:            uuid.New().String(),
		Timestamp:     time.Now().UTC(),
		EventType:     eventType,
		Severity:      severity,
		Username:      username,
		UserID:        userID,
		IPAddress:     ipAddress,
		UserAgent:     userAgent,
		DeviceName:    deviceName,
		SessionID:     sessionID,
		RequestID:     requestID,
		Success:       success,
		FailureReason: failureReason,
	}

	return r.StoreAuditLog(ctx, log)
}

// filterLogsByTimeRange filters logs by time range and limit
func (r *AuditRepository) filterLogsByTimeRange(logs []*models.AuthAuditLog, limit int, startTime, endTime time.Time) []*models.AuthAuditLog {
	var result []*models.AuthAuditLog

	for _, log := range logs {
		if !startTime.IsZero() && log.Timestamp.Before(startTime) {
			continue
		}
		if !endTime.IsZero() && log.Timestamp.After(endTime) {
			continue
		}
		result = append(result, log)
	}

	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}

	return result
}

// cleanupIndex removes old logs from an index
func (r *AuditRepository) cleanupIndex(index map[string][]*models.AuthAuditLog, cutoff time.Time) {
	for key, logs := range index {
		var filtered []*models.AuthAuditLog
		for _, log := range logs {
			if !log.Timestamp.Before(cutoff) {
				filtered = append(filtered, log)
			}
		}
		if len(filtered) == 0 {
			delete(index, key)
		} else {
			index[key] = filtered
		}
	}
}

// Clear clears all data (test helper)
func (r *AuditRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.logsByID = make(map[string]*models.AuthAuditLog)
	r.logsByDate = make(map[string][]*models.AuthAuditLog)
	r.logsByUser = make(map[string][]*models.AuthAuditLog)
	r.logsByIP = make(map[string][]*models.AuthAuditLog)
	r.logsBySession = make(map[string][]*models.AuthAuditLog)
	r.logsBySeverity = make(map[string][]*models.AuthAuditLog)
}

// Ensure AuditRepository implements interfaces.AuditRepository
var _ interfaces.AuditRepository = (*AuditRepository)(nil)
