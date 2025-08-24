package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/google/uuid"
	"github.com/pay-theory/dynamorm/pkg/core"
	ddbErrors "github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// PushSubscriptionRepository handles push subscription operations
type PushSubscriptionRepository struct {
	*BaseRepository[*models.PushSubscription]
	vapidRepo *BaseRepository[*models.VAPIDKeyRecord]
}

// NewPushSubscriptionRepository creates a new push subscription repository
func NewPushSubscriptionRepository(db core.DB, tableName string, logger *zap.Logger) *PushSubscriptionRepository {
	return &PushSubscriptionRepository{
		BaseRepository: NewBaseRepository[*models.PushSubscription](db, tableName, logger),
		vapidRepo:      NewBaseRepository[*models.VAPIDKeyRecord](db, tableName, logger),
	}
}

// NewPushSubscriptionRepositoryWithCostTracking creates a new push subscription repository with cost tracking
func NewPushSubscriptionRepositoryWithCostTracking(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *PushSubscriptionRepository {
	return &PushSubscriptionRepository{
		BaseRepository: NewBaseRepositoryWithCostTracking[*models.PushSubscription](db, tableName, logger, costService, "push_subscription"),
		vapidRepo:      NewBaseRepositoryWithCostTracking[*models.VAPIDKeyRecord](db, tableName, logger, costService, "vapid_keys"),
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

	// Use BaseRepository Create method
	err := r.Create(ctx, record)
	if err != nil {
		return ErrorHandler.HandleCreateError(err, "push subscription", username)
	}

	return nil
}

// GetPushSubscription retrieves a push subscription by ID
func (r *PushSubscriptionRepository) GetPushSubscription(ctx context.Context, username, subscriptionID string) (*storage.PushSubscription, error) {
	var record models.PushSubscription
	pk := fmt.Sprintf("PUSH#%s", username)
	sk := fmt.Sprintf("SUB#%s", subscriptionID)

	err := r.Get(ctx, pk, sk, &record)
	if err != nil {
		if ddbErrors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(err, "push subscription", subscriptionID)
		}
		return nil, ErrorHandler.HandleGetError(err, "push subscription", subscriptionID)
	}

	// Convert model to storage type
	return r.convertModelToStorage(&record), nil
}

// GetUserPushSubscriptions retrieves all push subscriptions for a user
func (r *PushSubscriptionRepository) GetUserPushSubscriptions(ctx context.Context, username string) ([]*storage.PushSubscription, error) {
	pk := fmt.Sprintf("PUSH#%s", username)

	// Use BaseRepository QueryWithSKPrefix method
	records, err := r.QueryWithSKPrefix(ctx, pk, "SUB#", 100)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "push subscription", "user subscriptions")
	}

	subscriptions := make([]*storage.PushSubscription, len(records))
	for i, record := range records {
		subscriptions[i] = r.convertModelToStorage(record)
	}

	return subscriptions, nil
}

// UpdatePushSubscription updates the alerts for a push subscription
func (r *PushSubscriptionRepository) UpdatePushSubscription(ctx context.Context, username, subscriptionID string, alerts storage.PushSubscriptionAlerts) error {
	// First get the existing record
	var record models.PushSubscription
	pk := fmt.Sprintf("PUSH#%s", username)
	sk := fmt.Sprintf("SUB#%s", subscriptionID)

	err := r.Get(ctx, pk, sk, &record)
	if err != nil {
		return ErrorHandler.HandleGetError(err, "push subscription", subscriptionID)
	}

	// Update alerts and timestamp
	record.Alerts = convertStorageAlerts(alerts)
	record.UpdatedAt = time.Now()

	// Use BaseRepository Update method
	return r.Update(ctx, &record)
}

// DeletePushSubscription deletes a push subscription
func (r *PushSubscriptionRepository) DeletePushSubscription(ctx context.Context, username, subscriptionID string) error {
	pk := fmt.Sprintf("PUSH#%s", username)
	sk := fmt.Sprintf("SUB#%s", subscriptionID)

	err := r.Delete(ctx, pk, sk)
	if err != nil && !ddbErrors.IsNotFound(err) {
		return ErrorHandler.HandleDeleteError(err, "push subscription", subscriptionID)
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
	pk := "INSTANCE#CONFIG"
	sk := "VAPID_KEYS"

	err := r.vapidRepo.Get(ctx, pk, sk, &record)
	if err != nil {
		if ddbErrors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(err, "VAPID keys", "instance")
		}
		return nil, ErrorHandler.HandleGetError(err, "VAPID keys", "instance")
	}

	// Convert interface{} back to storage.VAPIDKeys
	keys, ok := record.Data.(storage.VAPIDKeys)
	if !ok {
		typeErr := errors.New("type assertion failed for VAPID keys data")
		return nil, ErrorHandler.HandleGetError(typeErr, "VAPID keys", "data conversion")
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

	// Try to update first, if not found then create
	err := r.vapidRepo.Update(ctx, record)
	if err != nil {
		if ddbErrors.IsNotFound(err) {
			// Create new record if not found
			err = r.vapidRepo.Create(ctx, record)
		}
		if err != nil {
			return ErrorHandler.HandleCreateError(err, "VAPID keys", "instance")
		}
	}

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
