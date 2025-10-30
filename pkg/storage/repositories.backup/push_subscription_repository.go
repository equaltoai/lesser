package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/google/uuid"
	"github.com/pay-theory/dynamorm/pkg/core"
	ddbErrors "github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// PushSubscriptionRepository handles push subscription operations using enhanced DynamORM patterns
type PushSubscriptionRepository struct {
	*EnhancedBaseRepository[*models.PushSubscription]
	vapidRepo         *EnhancedBaseRepository[*models.VAPIDKeyRecord]
	secretsClient     SecretsManagerClient
	vapidSecretARN    string
	defaultVAPIDEmail string
}

// SecretsManagerClient defines the subset of AWS Secrets Manager client methods used by the repository.
type SecretsManagerClient interface {
	GetSecretValue(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
	PutSecretValue(ctx context.Context, params *secretsmanager.PutSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error)
}

// NewPushSubscriptionRepository creates a new push subscription repository with enhanced functionality and cost tracking
func NewPushSubscriptionRepository(
	db core.DB,
	tableName string,
	logger *zap.Logger,
	costService *cost.TrackingService,
	secretsClient SecretsManagerClient,
	vapidSecretARN string,
	defaultSubject string,
) *PushSubscriptionRepository {
	// Create enhanced repository for push subscription operations
	enhancedRepo := NewEnhancedBaseRepository[*models.PushSubscription](db, tableName, logger, costService, "PushSubscriptionRepository", "push_subscription")

	// Set up enhanced services for push subscription operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // Cache push subscriptions for performance
	enhancedRepo.SetEventService(NewDefaultEventService())

	// Create enhanced VAPID repo
	vapidRepo := NewEnhancedBaseRepository[*models.VAPIDKeyRecord](db, tableName, logger, costService, "VAPIDRepository", "vapid_keys")
	vapidRepo.SetValidationService(NewDefaultValidationService())
	vapidRepo.SetPermissionService(NewDefaultPermissionService())
	vapidRepo.SetCachingService(NewInMemoryCachingService())
	vapidRepo.SetEventService(NewDefaultEventService())

	return &PushSubscriptionRepository{
		EnhancedBaseRepository: enhancedRepo,
		vapidRepo:              vapidRepo,
		secretsClient:          secretsClient,
		vapidSecretARN:         vapidSecretARN,
		defaultVAPIDEmail:      defaultSubject,
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

	// Use enhanced validation and creation
	err := r.ValidateAndCreate(ctx, record)
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
	var records []*models.PushSubscription
	query := r.BaseRepository.db.WithContext(ctx).
		Model(&models.PushSubscription{}).
		Where("PK", "=", pk).
		Where("SK", "BEGINS_WITH", "SUB#").
		Limit(100)

	if err := query.All(&records); err != nil {
		r.logger.Error("failed to query push subscriptions by prefix",
			zap.Error(err),
			zap.String("pk", pk))
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
	if keys, err := r.getVAPIDKeysFromSecret(ctx); err == nil && keys != nil {
		return keys, nil
	} else if err != nil {
		r.logger.Warn("failed to load VAPID keys from secret, falling back to DynamoDB",
			zap.String("secret_arn", r.vapidSecretARN),
			zap.Error(err))
	}

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
	if keys == nil {
		return errors.New("vapid keys payload cannot be nil")
	}

	// Set creation timestamp if not set
	if keys.CreatedAt.IsZero() {
		keys.CreatedAt = time.Now().UTC()
	}
	keys.UpdatedAt = time.Now().UTC()

	if keys.Subject == "" {
		keys.Subject = r.defaultVAPIDEmail
	}

	if err := r.setVAPIDKeysInSecret(ctx, keys); err != nil {
		r.logger.Error("failed to store VAPID keys in secrets manager",
			zap.String("secret_arn", r.vapidSecretARN),
			zap.Error(err))
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

type vapidSecretPayload struct {
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key"`
	Subject    string `json:"subject"`
	CreatedAt  string `json:"created_at,omitempty"`
	UpdatedAt  string `json:"updated_at,omitempty"`
}

func (r *PushSubscriptionRepository) getVAPIDKeysFromSecret(ctx context.Context) (*storage.VAPIDKeys, error) {
	if r.secretsClient == nil || r.vapidSecretARN == "" {
		return nil, nil
	}

	result, err := r.secretsClient.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(r.vapidSecretARN),
	})
	if err != nil {
		return nil, err
	}

	if result.SecretString == nil {
		return nil, errors.New("vapid secret does not contain SecretString")
	}

	var payload vapidSecretPayload
	if err := json.Unmarshal([]byte(*result.SecretString), &payload); err != nil {
		return nil, err
	}

	if payload.PublicKey == "" || payload.PrivateKey == "" {
		return nil, errors.New("vapid secret missing key material")
	}

	subject := payload.Subject
	if subject == "" {
		subject = r.defaultVAPIDEmail
	}

	return &storage.VAPIDKeys{
		PublicKey:  payload.PublicKey,
		PrivateKey: payload.PrivateKey,
		Subject:    subject,
		CreatedAt:  parseVAPIDTimestamp(payload.CreatedAt),
		UpdatedAt:  parseVAPIDTimestamp(payload.UpdatedAt),
	}, nil
}

func (r *PushSubscriptionRepository) setVAPIDKeysInSecret(ctx context.Context, keys *storage.VAPIDKeys) error {
	if r.secretsClient == nil || r.vapidSecretARN == "" {
		return nil
	}

	payload := vapidSecretPayload{
		PublicKey:  keys.PublicKey,
		PrivateKey: keys.PrivateKey,
		Subject:    keys.Subject,
		CreatedAt:  keys.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:  keys.UpdatedAt.UTC().Format(time.RFC3339),
	}

	secretBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, err = r.secretsClient.PutSecretValue(ctx, &secretsmanager.PutSecretValueInput{
		SecretId:     aws.String(r.vapidSecretARN),
		SecretString: aws.String(string(secretBytes)),
	})
	return err
}

func parseVAPIDTimestamp(value string) time.Time {
	if value == "" {
		return time.Time{}
	}

	if ts, err := time.Parse(time.RFC3339, value); err == nil {
		return ts
	}

	return time.Time{}
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
