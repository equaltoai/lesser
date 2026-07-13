// Package observability provides a standalone alert repository to avoid import cycles
package observability

import (
	"context"
	"fmt"
	"time"

	"github.com/theory-cloud/tabletheory/v2/pkg/core"
	"github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// StandaloneAlertRepository provides CRUD operations for alerts using DynamORM without import cycles
type StandaloneAlertRepository struct {
	db          core.DB
	tableName   string
	logger      *zap.Logger
	costService dynamoCostTracker
}

type dynamoCostTracker interface {
	TrackDynamoOperation(ctx context.Context, operation cost.DynamoOperation) error
}

// NewStandaloneAlertRepository creates a new standalone alert repository
func NewStandaloneAlertRepository(db core.DB, tableName string, logger *zap.Logger, costService dynamoCostTracker) *StandaloneAlertRepository {
	return &StandaloneAlertRepository{
		db:          db,
		tableName:   tableName,
		logger:      logger,
		costService: costService,
	}
}

// CreateAlert creates a new alert
func (r *StandaloneAlertRepository) CreateAlert(ctx context.Context, alert *models.Alert) error {
	if alert.AlertID == "" {
		return fmt.Errorf("alert_id is required")
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

	// Update keys before saving
	if err := alert.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}

	// Track cost if cost service is available
	if r.costService != nil {
		operation := cost.DynamoOperation{
			Type:               "PutItem",
			TableName:          r.tableName,
			ConsumedReadUnits:  0,
			ConsumedWriteUnits: 1,
			ItemCount:          1,
			Timestamp:          time.Now(),
			OperationID:        fmt.Sprintf("alert_create_%d", time.Now().UnixNano()),
		}

		defer func() {
			if trackErr := r.costService.TrackDynamoOperation(ctx, operation); trackErr != nil {
				r.logger.Warn("failed to track DynamoDB create operation cost",
					zap.String("alert_id", alert.AlertID),
					zap.Error(trackErr))
			}
		}()
	}

	// Create the alert
	err := r.db.WithContext(ctx).Model(alert).Create()
	if err != nil {
		r.logger.Error("failed to create alert",
			zap.Error(err),
			zap.String("alert_id", alert.AlertID))
		return fmt.Errorf("failed to create alert: %w", err)
	}

	return nil
}

// GetByID retrieves an alert by its ID
func (r *StandaloneAlertRepository) GetByID(ctx context.Context, alertID string) (*models.Alert, error) {
	alert := &models.Alert{}
	pk := fmt.Sprintf("ALERT#%s", alertID)
	sk := "METADATA"

	err := r.db.WithContext(ctx).Model(alert).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(alert)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("alert not found: %s", alertID)
		}
		r.logger.Error("failed to get alert",
			zap.Error(err),
			zap.String("alert_id", alertID))
		return nil, fmt.Errorf("failed to get alert: %w", err)
	}

	return alert, nil
}

// Update updates an existing alert
func (r *StandaloneAlertRepository) Update(ctx context.Context, alert *models.Alert) error {
	alert.UpdatedAt = time.Now()

	// Track cost if cost service is available
	if r.costService != nil {
		operation := cost.DynamoOperation{
			Type:               "UpdateItem",
			TableName:          r.tableName,
			ConsumedReadUnits:  0,
			ConsumedWriteUnits: 1,
			ItemCount:          1,
			Timestamp:          time.Now(),
			OperationID:        fmt.Sprintf("alert_update_%d", time.Now().UnixNano()),
		}

		defer func() {
			if trackErr := r.costService.TrackDynamoOperation(ctx, operation); trackErr != nil {
				r.logger.Warn("failed to track DynamoDB update operation cost",
					zap.String("alert_id", alert.AlertID),
					zap.Error(trackErr))
			}
		}()
	}

	err := r.db.WithContext(ctx).Model(alert).Update()
	if err != nil {
		r.logger.Error("failed to update alert",
			zap.Error(err),
			zap.String("alert_id", alert.AlertID))
		return fmt.Errorf("failed to update alert: %w", err)
	}

	return nil
}

// GetActiveAlerts retrieves all currently active (firing) alerts
func (r *StandaloneAlertRepository) GetActiveAlerts(ctx context.Context, limit int) ([]*models.Alert, error) {
	var alerts []*models.Alert

	// Query GSI3 for firing alerts
	err := r.db.WithContext(ctx).Model(&models.Alert{}).
		Index(models.IndexGSI3).
		Where("gsi3PK", "=", "STATUS#firing").
		OrderBy("gsi3SK", "DESC").
		Limit(limit).
		All(&alerts)

	if err != nil {
		r.logger.Error("failed to get active alerts", zap.Error(err))
		return nil, fmt.Errorf("failed to get active alerts: %w", err)
	}

	return alerts, nil
}

// GetAlertsNeedingRetry retrieves alerts that need delivery retry
func (r *StandaloneAlertRepository) GetAlertsNeedingRetry(ctx context.Context, limit int) ([]*models.Alert, error) {
	var alerts []*models.Alert

	// This is a simplified query - in practice you might need a more complex query
	err := r.db.WithContext(ctx).Model(&models.Alert{}).
		Index(models.IndexGSI3).
		Where("gsi3PK", "=", "STATUS#firing").
		Filter("DeliveryAttempts", "<", 5).
		Filter("NextRetryAt", "<=", time.Now().Unix()).
		OrderBy("gsi3SK", "ASC").
		Limit(limit).
		All(&alerts)

	if err != nil {
		r.logger.Error("failed to get alerts needing retry", zap.Error(err))
		return nil, fmt.Errorf("failed to get alerts needing retry: %w", err)
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
func (r *StandaloneAlertRepository) ResolveAlert(ctx context.Context, alertID string) error {
	alert, err := r.GetByID(ctx, alertID)
	if err != nil {
		return err
	}

	alert.Resolve()
	return r.Update(ctx, alert)
}

// CleanupOldAlerts removes alerts older than the specified duration
func (r *StandaloneAlertRepository) CleanupOldAlerts(ctx context.Context, olderThan time.Duration) (int, error) {
	cutoffTime := time.Now().Add(-olderThan)
	cutoffTimestamp := fmt.Sprintf("TIMESTAMP#%s", cutoffTime.Format(time.RFC3339))

	// Find old alerts across all types
	var oldAlerts []*models.Alert
	types := []string{"error_rate", "latency", "cost", "health", "security", "capacity"}

	for _, alertType := range types {
		var typeAlerts []*models.Alert
		err := r.db.WithContext(ctx).Model(&models.Alert{}).
			Index(models.IndexGSI1).
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
		err := r.db.WithContext(ctx).Model(&models.Alert{}).
			Where("PK", "=", alert.PK).
			Where("SK", "=", alert.SK).
			Delete()
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

// StandaloneWebhookRepository provides webhook delivery operations without import cycles
type StandaloneWebhookRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewStandaloneWebhookRepository creates a new standalone webhook repository
func NewStandaloneWebhookRepository(db core.DB, tableName string, logger *zap.Logger) *StandaloneWebhookRepository {
	return &StandaloneWebhookRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// CreateDelivery creates a new webhook delivery record
func (r *StandaloneWebhookRepository) CreateDelivery(ctx context.Context, delivery *models.WebhookDelivery) error {
	if err := delivery.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}

	err := r.db.WithContext(ctx).Model(delivery).Create()
	if err != nil {
		r.logger.Error("failed to create webhook delivery",
			zap.Error(err),
			zap.String("delivery_id", delivery.DeliveryID))
		return fmt.Errorf("failed to create webhook delivery: %w", err)
	}

	return nil
}

// UpdateDelivery updates a webhook delivery record
func (r *StandaloneWebhookRepository) UpdateDelivery(ctx context.Context, delivery *models.WebhookDelivery) error {
	delivery.UpdatedAt = time.Now()

	err := r.db.WithContext(ctx).Model(delivery).Update()
	if err != nil {
		r.logger.Error("failed to update webhook delivery",
			zap.Error(err),
			zap.String("delivery_id", delivery.DeliveryID))
		return fmt.Errorf("failed to update webhook delivery: %w", err)
	}

	return nil
}

// GetPendingRetries retrieves webhook deliveries that need retry
func (r *StandaloneWebhookRepository) GetPendingRetries(ctx context.Context, limit int) ([]*models.WebhookDelivery, error) {
	var deliveries []*models.WebhookDelivery

	// Query GSI2 for failed deliveries ready for retry
	err := r.db.WithContext(ctx).Model(&models.WebhookDelivery{}).
		Index(models.IndexGSI2).
		Where("gsi2PK", "=", "STATUS#retrying").
		Filter("NextRetryAt", "<=", time.Now().Unix()).
		OrderBy("gsi2SK", "ASC").
		Limit(limit).
		All(&deliveries)

	if err != nil {
		r.logger.Error("failed to get pending retries", zap.Error(err))
		return nil, fmt.Errorf("failed to get pending retries: %w", err)
	}

	// Filter deliveries that actually should retry
	var retryDeliveries []*models.WebhookDelivery
	for _, delivery := range deliveries {
		if delivery.ShouldRetry() {
			retryDeliveries = append(retryDeliveries, delivery)
		}
	}

	return retryDeliveries, nil
}

// GetDeliveriesByAlert retrieves webhook deliveries for a specific alert
func (r *StandaloneWebhookRepository) GetDeliveriesByAlert(ctx context.Context, alertID string, limit int) ([]*models.WebhookDelivery, error) {
	var deliveries []*models.WebhookDelivery

	// Query GSI1 for deliveries by alert
	err := r.db.WithContext(ctx).Model(&models.WebhookDelivery{}).
		Index(models.IndexGSI1).
		Where("gsi1PK", "=", fmt.Sprintf("ALERT#%s", alertID)).
		OrderBy("gsi1SK", "DESC").
		Limit(limit).
		All(&deliveries)

	if err != nil {
		r.logger.Error("failed to get deliveries by alert",
			zap.String("alert_id", alertID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get deliveries by alert: %w", err)
	}

	return deliveries, nil
}

// StandaloneDeadLetterRepository provides dead letter message operations
type StandaloneDeadLetterRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewStandaloneDeadLetterRepository creates a new dead letter repository
func NewStandaloneDeadLetterRepository(db core.DB, tableName string, logger *zap.Logger) *StandaloneDeadLetterRepository {
	return &StandaloneDeadLetterRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// Create creates a new dead letter message
func (r *StandaloneDeadLetterRepository) Create(ctx context.Context, message *models.DeadLetterMessage) error {
	if err := message.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}

	err := r.db.WithContext(ctx).Model(message).Create()
	if err != nil {
		r.logger.Error("failed to create dead letter message",
			zap.Error(err),
			zap.String("message_id", message.MessageID))
		return fmt.Errorf("failed to create dead letter message: %w", err)
	}

	return nil
}

// GetByType retrieves dead letter messages by type
func (r *StandaloneDeadLetterRepository) GetByType(ctx context.Context, messageType string, limit int) ([]*models.DeadLetterMessage, error) {
	var messages []*models.DeadLetterMessage

	err := r.db.WithContext(ctx).Model(&models.DeadLetterMessage{}).
		Where("PK", "=", fmt.Sprintf("DLQ#%s", messageType)).
		OrderBy("SK", "DESC").
		Limit(limit).
		All(&messages)

	if err != nil {
		r.logger.Error("failed to get dead letter messages by type",
			zap.String("type", messageType),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get dead letter messages by type: %w", err)
	}

	return messages, nil
}
