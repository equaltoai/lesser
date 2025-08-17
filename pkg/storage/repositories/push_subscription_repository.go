package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/google/uuid"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
	"github.com/equaltoai/lesser/pkg/common"
)

// PushSubscriptionRepository handles push subscription operations
type PushSubscriptionRepository struct {
	db         core.DB
	tableName  string
	logger     *zap.Logger
	queryUtils *QueryUtils
}

// NewPushSubscriptionRepository creates a new push subscription repository
func NewPushSubscriptionRepository(db core.DB, tableName string, logger *zap.Logger) *PushSubscriptionRepository {
	return &PushSubscriptionRepository{
		db:         db,
		tableName:  tableName,
		logger:     logger,
		queryUtils: NewQueryUtils(db, logger),
	}
}

// CreatePushSubscription creates a new push subscription
func (r *PushSubscriptionRepository) CreatePushSubscription(ctx context.Context, username string, subscription *storage.PushSubscription) error {
	// Generate ID if not provided
	if err := common.ValidateRequiredParam("subscription.ID", subscription.ID); err != nil {
		subscription.ID = uuid.New().String()
	}

	// Set timestamps
	now := time.Now()
	subscription.Username = username
	subscription.CreatedAt = now
	subscription.UpdatedAt = now

	// Create the push subscription record
	record := &models.PushSubscription{
		ID:        subscription.ID,
		Username:  username,
		Endpoint:  subscription.Endpoint,
		P256dh:    subscription.P256dh,
		Auth:      subscription.Auth,
		Alerts:    convertStorageAlerts(subscription.Alerts),
		Policy:    subscription.Policy,
		CreatedAt: subscription.CreatedAt,
		UpdatedAt: subscription.UpdatedAt,
	}

	// Update keys will set PK, SK, and GSI values
	record.UpdateKeys()

	err := r.db.WithContext(ctx).Model(record).Create()
	if err != nil {
		r.logger.Error("failed to create push subscription",
			zap.String("subscription_id", subscription.ID),
			zap.String("username", username),
			zap.Error(err))
		return err
	}

	r.logger.Debug("created push subscription",
		zap.String("subscription_id", subscription.ID),
		zap.String("username", username))

	return nil
}

// GetPushSubscription retrieves a push subscription by ID
func (r *PushSubscriptionRepository) GetPushSubscription(ctx context.Context, username, subscriptionID string) (*storage.PushSubscription, error) {
	var record models.PushSubscription
	err := r.queryUtils.GetItemByPK(ctx, 
		fmt.Sprintf("PUSH#%s", username),
		fmt.Sprintf("SUB#%s", subscriptionID),
		&record)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("push subscription not found")
		}
		return nil, err
	}

	// Convert model to storage type
	return r.convertModelToStorage(&record), nil
}

// GetUserPushSubscriptions retrieves all push subscriptions for a user
func (r *PushSubscriptionRepository) GetUserPushSubscriptions(ctx context.Context, username string) ([]*storage.PushSubscription, error) {
	result, err := r.queryUtils.QueryWithPrefix(ctx, 
		fmt.Sprintf("PUSH#%s", username),
		"SUB#",
		&QueryOptions{
			Limit: 100, // Reasonable limit for push subscriptions per user
		})

	if err != nil {
		return nil, err
	}

	subscriptions := make([]*storage.PushSubscription, 0, len(result.Items))
	for _, item := range result.Items {
		if sub := r.mapItemToSubscription(item); sub != nil {
			subscriptions = append(subscriptions, sub)
		}
	}

	return subscriptions, nil
}

// UpdatePushSubscription updates the alerts for a push subscription
func (r *PushSubscriptionRepository) UpdatePushSubscription(ctx context.Context, username, subscriptionID string, alerts storage.PushSubscriptionAlerts) error {
	// First get the existing record
	var record models.PushSubscription
	err := r.queryUtils.GetItemByPK(ctx,
		fmt.Sprintf("PUSH#%s", username),
		fmt.Sprintf("SUB#%s", subscriptionID),
		&record)
	
	if err != nil {
		return err
	}

	// Update alerts and timestamp
	record.Alerts = convertStorageAlerts(alerts)
	record.UpdatedAt = time.Now()
	record.UpdateKeys()

	// Use generic update
	return r.queryUtils.UpdateItem(ctx, &record)
}

// DeletePushSubscription deletes a push subscription
func (r *PushSubscriptionRepository) DeletePushSubscription(ctx context.Context, username, subscriptionID string) error {
	err := r.queryUtils.DeleteItem(ctx,
		fmt.Sprintf("PUSH#%s", username),
		fmt.Sprintf("SUB#%s", subscriptionID),
		&models.PushSubscription{})

	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	return nil // Idempotent - don't fail if already deleted
}

// DeleteAllPushSubscriptions deletes all push subscriptions for a user
func (r *PushSubscriptionRepository) DeleteAllPushSubscriptions(ctx context.Context, username string) error {
	// First, get all subscriptions
	subscriptions, err := r.GetUserPushSubscriptions(ctx, username)
	if err != nil {
		return err
	}

	// Delete each subscription
	for _, sub := range subscriptions {
		if err := r.DeletePushSubscription(ctx, username, sub.ID); err != nil {
			r.logger.Error("failed to delete push subscription",
				zap.String("subscription_id", sub.ID),
				zap.String("username", username),
				zap.Error(err))
			// Continue with other subscriptions
		}
	}

	return nil
}

// GetVAPIDKeys retrieves the VAPID keys for the instance
func (r *PushSubscriptionRepository) GetVAPIDKeys(ctx context.Context) (*storage.VAPIDKeys, error) {
	var record models.VAPIDKeyRecord
	err := r.db.WithContext(ctx).Model(&models.VAPIDKeyRecord{}).
		Where("PK", "=", "INSTANCE#CONFIG").
		Where("SK", "=", "VAPID_KEYS").
		First(&record)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("VAPID keys not found")
		}
		return nil, err
	}

	// Convert interface{} back to storage.VAPIDKeys
	keys, ok := record.Data.(storage.VAPIDKeys)
	if !ok {
		return nil, fmt.Errorf("failed to convert VAPID keys data")
	}

	return &keys, nil
}

// SetVAPIDKeys stores the VAPID keys for the instance
func (r *PushSubscriptionRepository) SetVAPIDKeys(ctx context.Context, keys *storage.VAPIDKeys) error {
	// Set creation timestamp if not set
	if keys.CreatedAt.IsZero() {
		keys.CreatedAt = time.Now()
	}

	// Create the VAPID key record
	record := &models.VAPIDKeyRecord{
		PK:        "INSTANCE#CONFIG",
		SK:        "VAPID_KEYS",
		Data:      *keys,
		UpdatedAt: time.Now(),
	}

	// Update GSI keys (no-op for VAPID)
	record.UpdateKeys()

	// Try to update first, if not found then create
	err := r.db.WithContext(ctx).Model(record).Update()
	if err != nil {
		if errors.IsNotFound(err) {
			// Create new record if not found
			err = r.db.WithContext(ctx).Model(record).Create()
		}
	}

	if err != nil {
		return err
	}

	r.logger.Debug("stored VAPID keys for instance")

	return nil
}

// convertStorageAlerts converts storage.PushSubscriptionAlerts to models.PushSubscriptionAlerts
func convertStorageAlerts(alerts storage.PushSubscriptionAlerts) models.PushSubscriptionAlerts {
	return models.PushSubscriptionAlerts{
		Follow:        alerts.Follow,
		Favourite:     alerts.Favourite,
		Reblog:        alerts.Reblog,
		Mention:       alerts.Mention,
		Poll:          alerts.Poll,
		FollowRequest: alerts.FollowRequest,
		Status:        alerts.Status,
		Update:        alerts.Update,
		AdminSignUp:   alerts.AdminSignUp,
		AdminReport:   alerts.AdminReport,
	}
}

// convertModelAlerts converts models.PushSubscriptionAlerts to storage.PushSubscriptionAlerts
func convertModelAlerts(alerts models.PushSubscriptionAlerts) storage.PushSubscriptionAlerts {
	return storage.PushSubscriptionAlerts{
		Follow:        alerts.Follow,
		Favourite:     alerts.Favourite,
		Reblog:        alerts.Reblog,
		Mention:       alerts.Mention,
		Poll:          alerts.Poll,
		FollowRequest: alerts.FollowRequest,
		Status:        alerts.Status,
		Update:        alerts.Update,
		AdminSignUp:   alerts.AdminSignUp,
		AdminReport:   alerts.AdminReport,
	}
}

// convertModelToStorage converts a model to storage type
func (r *PushSubscriptionRepository) convertModelToStorage(record *models.PushSubscription) *storage.PushSubscription {
	return &storage.PushSubscription{
		ID:        record.ID,
		Username:  record.Username,
		Endpoint:  record.Endpoint,
		P256dh:    record.P256dh,
		Auth:      record.Auth,
		Alerts:    convertModelAlerts(record.Alerts),
		Policy:    record.Policy,
		CreatedAt: record.CreatedAt,
		UpdatedAt: record.UpdatedAt,
	}
}

// mapItemToSubscription maps a generic item to PushSubscription
func (r *PushSubscriptionRepository) mapItemToSubscription(item map[string]interface{}) *storage.PushSubscription {
	sub := &storage.PushSubscription{}
	
	if id, ok := item["ID"].(string); ok {
		sub.ID = id
	}
	if username, ok := item["Username"].(string); ok {
		sub.Username = username
	}
	if endpoint, ok := item["Endpoint"].(string); ok {
		sub.Endpoint = endpoint
	}
	if p256dh, ok := item["P256dh"].(string); ok {
		sub.P256dh = p256dh
	}
	if auth, ok := item["Auth"].(string); ok {
		sub.Auth = auth
	}
	if policy, ok := item["Policy"].(string); ok {
		sub.Policy = policy
	}
	if createdAt, ok := item["CreatedAt"].(time.Time); ok {
		sub.CreatedAt = createdAt
	}
	if updatedAt, ok := item["UpdatedAt"].(time.Time); ok {
		sub.UpdatedAt = updatedAt
	}
	
	// Handle alerts as a nested structure
	if alertsMap, ok := item["Alerts"].(map[string]interface{}); ok {
		sub.Alerts = storage.PushSubscriptionAlerts{
			Follow:        getBoolFromMap(alertsMap, "Follow"),
			Favourite:     getBoolFromMap(alertsMap, "Favourite"),
			Reblog:        getBoolFromMap(alertsMap, "Reblog"),
			Mention:       getBoolFromMap(alertsMap, "Mention"),
			Poll:          getBoolFromMap(alertsMap, "Poll"),
			FollowRequest: getBoolFromMap(alertsMap, "FollowRequest"),
			Status:        getBoolFromMap(alertsMap, "Status"),
			Update:        getBoolFromMap(alertsMap, "Update"),
			AdminSignUp:   getBoolFromMap(alertsMap, "AdminSignUp"),
			AdminReport:   getBoolFromMap(alertsMap, "AdminReport"),
		}
	}
	
	return sub
}

// getBoolFromMap safely gets a bool value from a map
func getBoolFromMap(m map[string]interface{}, key string) bool {
	if val, ok := m[key].(bool); ok {
		return val
	}
	return false
}
