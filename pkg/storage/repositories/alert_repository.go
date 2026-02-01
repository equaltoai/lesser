package repositories

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/theory-cloud/tabletheory/pkg/core"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// AlertRepository provides CRUD operations for alerts using enhanced repository patterns
type AlertRepository struct {
	*EnhancedBaseRepository[*models.Alert]
}

// NewAlertRepository creates a new alert repository with enhanced functionality
func NewAlertRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *AlertRepository {
	// Create enhanced repository optimized for alert operations
	enhancedRepo := NewEnhancedBaseRepository[*models.Alert](db, tableName, logger, costService, "AlertRepository", "alert")

	// Set up enhanced services for alert operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // Alerts cached for performance
	enhancedRepo.SetEventService(NewDefaultEventService())      // Critical for alert notifications

	return &AlertRepository{
		EnhancedBaseRepository: enhancedRepo,
	}
}

// CreateAlert creates a new alert
func (r *AlertRepository) CreateAlert(ctx context.Context, alert *models.Alert) error {
	if alert.AlertID == "" {
		return ErrorHandler.HandleCreateError(errors.New("alert_id is required"), EntityAlert, "validation")
	}
	if alert.CreatedAt.IsZero() {
		alert.CreatedAt = time.Now()
	}
	if alert.UpdatedAt.IsZero() {
		alert.UpdatedAt = time.Now()
	}
	if alert.FiredAt.IsZero() {
		alert.FiredAt = time.Now()
	}

	return r.ValidateAndCreate(ctx, alert)
}

// GetByID retrieves an alert by its ID
func (r *AlertRepository) GetByID(ctx context.Context, alertID string) (*models.Alert, error) {
	alert := &models.Alert{}
	pk := fmt.Sprintf("ALERT#%s", alertID)
	sk := models.SKMetadata

	err := r.Get(ctx, pk, sk, alert)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, ErrorHandler.HandleGetError(errors.New("alert not found"), EntityAlert, alertID)
		}
		return nil, err
	}

	return alert, nil
}

// Update updates an existing alert
func (r *AlertRepository) Update(ctx context.Context, alert *models.Alert) error {
	alert.UpdatedAt = time.Now()
	return r.ValidateAndUpdate(ctx, alert)
}

// Delete deletes an alert
func (r *AlertRepository) Delete(ctx context.Context, alertID string) error {
	pk := fmt.Sprintf("ALERT#%s", alertID)
	sk := models.SKMetadata
	return r.ValidateAndDelete(ctx, pk, sk)
}

// GetActiveAlerts retrieves all currently active (firing) alerts
func (r *AlertRepository) GetActiveAlerts(ctx context.Context, limit int) ([]*models.Alert, error) {
	var alerts []*models.Alert

	// Query GSI3 for firing alerts
	err := r.db.WithContext(ctx).Model(&models.Alert{}).
		Index("gsi3").
		Where("gsi3PK", "=", "STATUS#firing").
		OrderBy("gsi3SK", "DESC").
		Limit(limit).
		All(&alerts)

	if err != nil {
		r.logger.Error("failed to get active alerts", zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, "alert", "active alerts")
	}

	return alerts, nil
}

// GetAlertsByType retrieves alerts by type within a time range
func (r *AlertRepository) GetAlertsByType(ctx context.Context, alertType string, since time.Time, limit int) ([]*models.Alert, error) {
	var alerts []*models.Alert

	sinceTimestamp := fmt.Sprintf("TIMESTAMP#%s", since.Format(time.RFC3339))

	// Query GSI1 for alerts by type
	err := r.db.WithContext(ctx).Model(&models.Alert{}).
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("ALERT_TYPE#%s", alertType)).
		Where("gsi1SK", ">=", sinceTimestamp).
		OrderBy("gsi1SK", "DESC").
		Limit(limit).
		All(&alerts)

	if err != nil {
		r.logger.Error("failed to get alerts by type",
			zap.String("type", alertType),
			zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, "alert", "alerts by type")
	}

	return alerts, nil
}

// GetAlertsByService retrieves alerts for a specific service
func (r *AlertRepository) GetAlertsByService(ctx context.Context, service string, limit int) ([]*models.Alert, error) {
	var alerts []*models.Alert

	// Query GSI2 for alerts by service
	err := r.db.WithContext(ctx).Model(&models.Alert{}).
		Index("gsi2").
		Where("gsi2PK", "=", fmt.Sprintf("SERVICE#%s", service)).
		OrderBy("gsi2SK", "DESC").
		Limit(limit).
		All(&alerts)

	if err != nil {
		r.logger.Error("failed to get alerts by service",
			zap.String("service", service),
			zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, "alert", "alerts by service")
	}

	return alerts, nil
}

// queryAlertsWithIndex performs a common alert query pattern against a specified GSI
// This consolidates the duplicated query logic used by severity and priority lookups
func (r *AlertRepository) queryAlertsWithIndex(ctx context.Context,
	indexName, pkField, pkValue, skFieldName, skValue string,
	since time.Time, limit int,
	logAction string, logFields []zap.Field) ([]*models.Alert, error) {

	var alerts []*models.Alert

	indexName = strings.ToLower(indexName)

	sinceTimestamp := since.Format(time.RFC3339)
	skPattern := fmt.Sprintf("%s#%s#TIMESTAMP#%s", skFieldName, skValue, sinceTimestamp)

	// Derive SK field name from PK field name (GSI2PK -> GSI2SK, GSI3PK -> GSI3SK)
	skField := pkField[:len(pkField)-2] + "SK"

	// Build the primary key value
	var pkPrefix string
	switch indexName {
	case "gsi2":
		pkPrefix = "SERVICE"
	case "gsi3":
		pkPrefix = "STATUS"
	}
	pkFullValue := fmt.Sprintf("%s#%s", pkPrefix, pkValue)

	// Query the specified GSI
	err := r.db.WithContext(ctx).Model(&models.Alert{}).
		Index(indexName).
		Where(pkField, "=", pkFullValue).
		Where(skField, ">=", skPattern).
		OrderBy(skField, "DESC").
		Limit(limit).
		All(&alerts)

	if err != nil {
		logFields = append(logFields, zap.Error(err))
		r.logger.Error(logAction, logFields...)
		return nil, ErrorHandler.HandleQueryError(err, "alert", logAction)
	}

	return alerts, nil
}

// GetAlertsBySeverity retrieves alerts by severity level
func (r *AlertRepository) GetAlertsBySeverity(ctx context.Context, service, severity string, since time.Time, limit int) ([]*models.Alert, error) {
	return r.queryAlertsWithIndex(ctx,
		"gsi2", "gsi2PK", service, "SEVERITY", severity,
		since, limit,
		"failed to get alerts by severity",
		[]zap.Field{
			zap.String("service", service),
			zap.String("severity", severity),
		})
}

// GetAlertsByPriority retrieves alerts by priority level
func (r *AlertRepository) GetAlertsByPriority(ctx context.Context, status, priority string, since time.Time, limit int) ([]*models.Alert, error) {
	return r.queryAlertsWithIndex(ctx,
		"gsi3", "gsi3PK", status, "PRIORITY", priority,
		since, limit,
		"failed to get alerts by priority",
		[]zap.Field{
			zap.String("status", status),
			zap.String("priority", priority),
		})
}

// GetCriticalAlerts retrieves all critical alerts
func (r *AlertRepository) GetCriticalAlerts(ctx context.Context, limit int) ([]*models.Alert, error) {
	return r.GetAlertsByPriority(ctx, "firing", "P0", time.Now().Add(-24*time.Hour), limit)
}

// GetAlertsNeedingRetry retrieves alerts that need delivery retry
func (r *AlertRepository) GetAlertsNeedingRetry(ctx context.Context, limit int) ([]*models.Alert, error) {
	var alerts []*models.Alert

	// This is a simplified query - in practice you might need a more complex query
	// or a separate GSI for alerts that need retry
	err := r.db.WithContext(ctx).Model(&models.Alert{}).
		Index("gsi3").
		Where("gsi3PK", "=", "STATUS#firing").
		Filter("DeliveryAttempts", "<", 5).
		Filter("NextRetryAt", "<=", time.Now().Unix()).
		OrderBy("gsi3SK", "ASC").
		Limit(limit).
		All(&alerts)

	if err != nil {
		r.logger.Error("failed to get alerts needing retry", zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, "alert", "alerts needing retry")
	}

	// Filter alerts that actually need retry
	var retryAlerts []*models.Alert
	for _, alert := range alerts {
		if alert.ShouldRetry() {
			retryAlerts = append(retryAlerts, alert)
		}
	}

	return retryAlerts, nil
}

// ResolveAlert marks an alert as resolved
func (r *AlertRepository) ResolveAlert(ctx context.Context, alertID string) error {
	alert, err := r.GetByID(ctx, alertID)
	if err != nil {
		return err
	}

	alert.Resolve()
	return r.Update(ctx, alert)
}

// AcknowledgeAlert marks an alert as acknowledged
func (r *AlertRepository) AcknowledgeAlert(ctx context.Context, alertID string) error {
	alert, err := r.GetByID(ctx, alertID)
	if err != nil {
		return err
	}

	alert.Acknowledge()
	return r.Update(ctx, alert)
}

// SuppressAlert suppresses an alert until the specified time
func (r *AlertRepository) SuppressAlert(ctx context.Context, alertID string, until time.Time) error {
	alert, err := r.GetByID(ctx, alertID)
	if err != nil {
		return err
	}

	alert.Suppress(until)
	return r.Update(ctx, alert)
}

// GetAlertStats returns statistics about alerts
func (r *AlertRepository) GetAlertStats(ctx context.Context, since time.Time) (*AlertStats, error) {
	stats := &AlertStats{
		Period: since,
	}

	// Get counts by status
	statuses := []string{"firing", DLQStatusResolved, "acknowledged", "suppressed"}
	for _, status := range statuses {
		count, err := r.countAlertsByStatus(ctx, status, since)
		if err != nil {
			r.logger.Error("failed to count alerts by status",
				zap.String("status", status),
				zap.Error(err))
			continue
		}

		switch status {
		case "firing":
			stats.FiringCount = count
		case DLQStatusResolved:
			stats.ResolvedCount = count
		case "acknowledged":
			stats.AcknowledgedCount = count
		case "suppressed":
			stats.SuppressedCount = count
		}
	}

	// Get counts by severity
	severities := []string{"critical", "error", "warning", "info"}
	stats.BySeverity = make(map[string]int)
	for _, severity := range severities {
		count, err := r.countAlertsBySeverity(ctx, severity, since)
		if err != nil {
			r.logger.Error("failed to count alerts by severity",
				zap.String("severity", severity),
				zap.Error(err))
			continue
		}
		stats.BySeverity[severity] = count
	}

	// Get counts by type
	types := []string{"error_rate", "latency", "cost", "health", "security", "capacity"}
	stats.ByType = make(map[string]int)
	for _, alertType := range types {
		count, err := r.countAlertsByType(ctx, alertType, since)
		if err != nil {
			r.logger.Error("failed to count alerts by type",
				zap.String("type", alertType),
				zap.Error(err))
			continue
		}
		stats.ByType[alertType] = count
	}

	stats.TotalCount = stats.FiringCount + stats.ResolvedCount + stats.AcknowledgedCount + stats.SuppressedCount

	return stats, nil
}

// countAlertsByStatus counts alerts by status since a given time
func (r *AlertRepository) countAlertsByStatus(ctx context.Context, status string, since time.Time) (int, error) {
	sinceTimestamp := since.Format(time.RFC3339)
	skPattern := fmt.Sprintf("PRIORITY#P0#TIMESTAMP#%s", sinceTimestamp)

	count, err := r.db.WithContext(ctx).Model(&models.Alert{}).
		Index("gsi3").
		Where("gsi3PK", "=", fmt.Sprintf("STATUS#%s", status)).
		Where("gsi3SK", ">=", skPattern).
		Count()

	if err != nil {
		return 0, err
	}

	return int(count), nil
}

// countAlertsBySeverity counts alerts by severity since a given time
func (r *AlertRepository) countAlertsBySeverity(ctx context.Context, severity string, since time.Time) (int, error) {
	alerts, err := r.getAllAlertsSince(ctx, since, 1000) // Get a reasonable sample
	if err != nil {
		return 0, err
	}

	count := 0
	for _, alert := range alerts {
		if alert.Severity == severity {
			count++
		}
	}

	return count, nil
}

// countAlertsByType counts alerts by type since a given time
func (r *AlertRepository) countAlertsByType(ctx context.Context, alertType string, since time.Time) (int, error) {
	sinceTimestamp := fmt.Sprintf("TIMESTAMP#%s", since.Format(time.RFC3339))

	count, err := r.db.WithContext(ctx).Model(&models.Alert{}).
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("ALERT_TYPE#%s", alertType)).
		Where("gsi1SK", ">=", sinceTimestamp).
		Count()

	if err != nil {
		return 0, err
	}

	return int(count), nil
}

// getAllAlertsSince gets all alerts since a given time (for aggregation)
func (r *AlertRepository) getAllAlertsSince(ctx context.Context, since time.Time, limit int) ([]*models.Alert, error) {
	var alerts []*models.Alert

	sinceTimestamp := fmt.Sprintf("TIMESTAMP#%s", since.Format(time.RFC3339))

	// Query all alert types since the given time
	types := []string{"error_rate", "latency", "cost", "health", "security", "capacity"}
	for _, alertType := range types {
		var typeAlerts []*models.Alert
		err := r.db.WithContext(ctx).Model(&models.Alert{}).
			Index("gsi1").
			Where("gsi1PK", "=", fmt.Sprintf("ALERT_TYPE#%s", alertType)).
			Where("gsi1SK", ">=", sinceTimestamp).
			Limit(limit / len(types)).
			All(&typeAlerts)

		if err != nil {
			r.logger.Error("failed to get alerts by type for stats",
				zap.String("type", alertType),
				zap.Error(err))
			continue
		}

		alerts = append(alerts, typeAlerts...)
	}

	return alerts, nil
}

// CleanupOldAlerts removes alerts older than the specified duration
func (r *AlertRepository) CleanupOldAlerts(ctx context.Context, olderThan time.Duration) (int, error) {
	cutoffTime := time.Now().Add(-olderThan)
	cutoffTimestamp := fmt.Sprintf("TIMESTAMP#%s", cutoffTime.Format(time.RFC3339))

	// Find old alerts across all types
	var oldAlerts []*models.Alert
	types := []string{"error_rate", "latency", "cost", "health", "security", "capacity"}

	for _, alertType := range types {
		var typeAlerts []*models.Alert
		err := r.db.WithContext(ctx).Model(&models.Alert{}).
			Index("gsi1").
			Where("gsi1PK", "=", fmt.Sprintf("ALERT_TYPE#%s", alertType)).
			Where("gsi1SK", "<", cutoffTimestamp).
			Limit(100). // Process in batches
			All(&typeAlerts)

		if err != nil {
			r.logger.Error("failed to find old alerts for cleanup",
				zap.String("type", alertType),
				zap.Error(err))
			continue
		}

		oldAlerts = append(oldAlerts, typeAlerts...)
	}

	// Delete old alerts
	deletedCount := 0
	for _, alert := range oldAlerts {
		err := r.Delete(ctx, alert.AlertID)
		if err != nil {
			r.logger.Error("failed to delete old alert",
				zap.String("alert_id", alert.AlertID),
				zap.Error(err))
			continue
		}
		deletedCount++
	}

	if deletedCount > 0 {
		r.logger.Info("cleaned up old alerts",
			zap.Int("deleted_count", deletedCount),
			zap.Duration("older_than", olderThan))
	}

	return deletedCount, nil
}

// AlertStats represents alert statistics
type AlertStats struct {
	Period            time.Time      `json:"period"`
	TotalCount        int            `json:"total_count"`
	FiringCount       int            `json:"firing_count"`
	ResolvedCount     int            `json:"resolved_count"`
	AcknowledgedCount int            `json:"acknowledged_count"`
	SuppressedCount   int            `json:"suppressed_count"`
	BySeverity        map[string]int `json:"by_severity"`
	ByType            map[string]int `json:"by_type"`
}

// WebhookRepository provides operations for webhook deliveries
type WebhookRepository interface {
	CreateDelivery(ctx context.Context, delivery *models.WebhookDelivery) error
	UpdateDelivery(ctx context.Context, delivery *models.WebhookDelivery) error
	GetPendingRetries(ctx context.Context, limit int) ([]*models.WebhookDelivery, error)
	GetDeliveriesByAlert(ctx context.Context, alertID string, limit int) ([]*models.WebhookDelivery, error)
}

// DeadLetterRepository provides operations for dead letter messages
type DeadLetterRepository interface {
	Create(ctx context.Context, message *models.DeadLetterMessage) error
	GetByType(ctx context.Context, messageType string, limit int) ([]*models.DeadLetterMessage, error)
}

// DeadLetterMessage represents a message that failed processing
type DeadLetterMessage struct {
	PK            string                 `theorydb:"pk" json:"pk"`
	SK            string                 `theorydb:"sk" json:"sk"`
	MessageID     string                 `json:"message_id"`
	OriginalType  string                 `json:"original_type"`
	OriginalID    string                 `json:"original_id"`
	ErrorMessage  string                 `json:"error_message"`
	ErrorType     string                 `json:"error_type"`
	AttemptCount  int                    `json:"attempt_count"`
	LastAttemptAt time.Time              `json:"last_attempt_at"`
	Payload       map[string]interface{} `json:"payload"`
	CreatedAt     time.Time              `json:"created_at"`
	TTL           int64                  `json:"ttl,omitempty" theorydb:"ttl"`
}

// UpdateKeys sets the partition and sort keys
func (d *DeadLetterMessage) UpdateKeys() error {
	if d.MessageID == "" {
		return ErrorHandler.HandleCreateError(errors.New("message_id is required"), "dead letter message", "validation")
	}

	d.PK = fmt.Sprintf("DLQ#%s", d.OriginalType)
	d.SK = fmt.Sprintf("MESSAGE#%s", d.MessageID)

	// Set TTL for cleanup (30 days)
	if d.TTL == 0 {
		d.TTL = time.Now().Add(30 * 24 * time.Hour).Unix()
	}

	return nil
}

// GetPK returns the partition key
func (d *DeadLetterMessage) GetPK() string {
	return d.PK
}

// GetSK returns the sort key
func (d *DeadLetterMessage) GetSK() string {
	return d.SK
}
