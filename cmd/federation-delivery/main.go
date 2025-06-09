package main

import (
	"context"
	"fmt"
	"time"

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/config"
	"github.com/aron23/lesser/pkg/federation"
	"github.com/aron23/lesser/pkg/storage"
	storageDB "github.com/aron23/lesser/pkg/storage/dynamodb"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"go.uber.org/zap"
)

var (
	logger          *zap.Logger
	store           storage.Storage
	deliveryService *federation.DeliveryService
	cfg             *config.Config
)

func init() {
	var err error
	logger = common.Logger()
	cfg = config.Get()

	store, err = storageDB.New()
	if err != nil {
		logger.Fatal("Failed to initialize storage", zap.Error(err))
	}

	deliveryService = federation.NewDeliveryService(store)
}

// FederationDeliveryMessage represents a message from the SQS queue
type FederationDeliveryMessage struct {
	DeliveryID     string                `json:"delivery_id"`
	Activity       *activitypub.Activity `json:"activity"`
	TargetInbox    string                `json:"target_inbox"`
	SigningActorID string                `json:"signing_actor_id"`
	RetryCount     int                   `json:"retry_count"`
	MaxRetries     int                   `json:"max_retries"`
	CreatedAt      time.Time             `json:"created_at"`
	LastAttemptAt  *time.Time            `json:"last_attempt_at,omitempty"`
	NextRetryAfter *time.Time            `json:"next_retry_after,omitempty"`
	FailureReason  string                `json:"failure_reason,omitempty"`
}

// DeliveryStatus tracks the status of a delivery attempt
type DeliveryStatus struct {
	DeliveryID    string     `json:"delivery_id"`
	TargetInbox   string     `json:"target_inbox"`
	Status        string     `json:"status"` // pending, delivered, failed, retrying
	Attempts      int        `json:"attempts"`
	LastAttemptAt time.Time  `json:"last_attempt_at"`
	LastError     string     `json:"last_error,omitempty"`
	DeliveredAt   *time.Time `json:"delivered_at,omitempty"`
}

func main() {
	lambda.Start(handleSQSEvent)
}

func handleSQSEvent(ctx context.Context, sqsEvent events.SQSEvent) error {
	for _, record := range sqsEvent.Records {
		if err := processDeliveryMessage(ctx, record); err != nil {
			logger.Error("failed to process delivery message",
				zap.String("message_id", record.MessageId),
				zap.Error(err))
			// Return error to let SQS handle retry
			return err
		}
	}
	return nil
}

func processDeliveryMessage(ctx context.Context, record events.SQSMessage) error {
	// Parse the message
	var msg FederationDeliveryMessage
	if err := common.ParseRequestBody([]byte(record.Body), &msg); err != nil {
		return fmt.Errorf("failed to parse message: %w", err)
	}

	logger.Info("processing federation delivery",
		zap.String("delivery_id", msg.DeliveryID),
		zap.String("target_inbox", msg.TargetInbox),
		zap.Int("retry_count", msg.RetryCount))

	// Check if we should retry yet
	if msg.NextRetryAfter != nil && time.Now().Before(*msg.NextRetryAfter) {
		// Not time to retry yet, acknowledge message but don't process
		logger.Debug("skipping delivery, not time to retry yet",
			zap.String("delivery_id", msg.DeliveryID),
			zap.Time("retry_after", *msg.NextRetryAfter))
		return nil
	}

	// Get the signing actor
	signingActor, err := store.GetActor(ctx, msg.SigningActorID)
	if err != nil {
		return fmt.Errorf("failed to get signing actor: %w", err)
	}

	// Create delivery status record
	status := &DeliveryStatus{
		DeliveryID:    msg.DeliveryID,
		TargetInbox:   msg.TargetInbox,
		Status:        "attempting",
		Attempts:      msg.RetryCount + 1,
		LastAttemptAt: time.Now(),
	}

	// Attempt delivery
	err = deliveryService.DeliverActivity(ctx, msg.Activity, msg.TargetInbox, signingActor)
	if err != nil {
		status.Status = "failed"
		status.LastError = err.Error()

		// Check if we should retry
		if msg.RetryCount < msg.MaxRetries {
			// Calculate exponential backoff
			backoffMinutes := calculateBackoff(msg.RetryCount)
			nextRetry := time.Now().Add(time.Duration(backoffMinutes) * time.Minute)

			logger.Warn("delivery failed, will retry",
				zap.String("delivery_id", msg.DeliveryID),
				zap.Int("retry_count", msg.RetryCount),
				zap.Time("next_retry", nextRetry),
				zap.Error(err))

			// Update message for retry
			msg.RetryCount++
			msg.LastAttemptAt = &status.LastAttemptAt
			msg.NextRetryAfter = &nextRetry
			msg.FailureReason = err.Error()

			// Send back to queue with delay
			if err := requeueDelivery(ctx, &msg, backoffMinutes); err != nil {
				logger.Error("failed to requeue delivery",
					zap.String("delivery_id", msg.DeliveryID),
					zap.Error(err))
			}

			// Store delivery status
			if err := storeDeliveryStatus(ctx, status); err != nil {
				logger.Error("failed to store delivery status",
					zap.String("delivery_id", msg.DeliveryID),
					zap.Error(err))
			}

			// Don't return error - we've handled the retry
			return nil
		}

		// Max retries exceeded
		logger.Error("delivery failed after max retries",
			zap.String("delivery_id", msg.DeliveryID),
			zap.Int("attempts", msg.RetryCount+1),
			zap.Error(err))

		status.Status = "permanently_failed"

		// Store final status
		if err := storeDeliveryStatus(ctx, status); err != nil {
			logger.Error("failed to store delivery status",
				zap.String("delivery_id", msg.DeliveryID),
				zap.Error(err))
		}

		// Send to dead letter queue by returning error
		return fmt.Errorf("delivery permanently failed after %d attempts: %w", msg.RetryCount+1, err)
	}

	// Success!
	deliveredAt := time.Now()
	status.Status = "delivered"
	status.DeliveredAt = &deliveredAt

	logger.Info("successfully delivered activity",
		zap.String("delivery_id", msg.DeliveryID),
		zap.String("target_inbox", msg.TargetInbox),
		zap.Int("attempts", msg.RetryCount+1))

	// Store successful delivery status
	if err := storeDeliveryStatus(ctx, status); err != nil {
		logger.Error("failed to store delivery status",
			zap.String("delivery_id", msg.DeliveryID),
			zap.Error(err))
	}

	return nil
}

// calculateBackoff calculates exponential backoff in minutes
func calculateBackoff(retryCount int) int {
	// Start with 1 minute, double each time, max 60 minutes
	backoff := 1 << retryCount
	if backoff > 60 {
		backoff = 60
	}
	return backoff
}

// requeueDelivery sends the message back to the queue with a delay
func requeueDelivery(ctx context.Context, msg *FederationDeliveryMessage, delayMinutes int) error {
	// In production, this would use SQS SendMessage with DelaySeconds
	// For now, we'll rely on the NextRetryAfter field
	logger.Debug("would requeue message with delay",
		zap.String("delivery_id", msg.DeliveryID),
		zap.Int("delay_minutes", delayMinutes))
	return nil
}

// storeDeliveryStatus stores the delivery status in DynamoDB
func storeDeliveryStatus(ctx context.Context, status *DeliveryStatus) error {
	// Store in DynamoDB with appropriate TTL
	// This would use a pattern like:
	// PK: DELIVERY#<delivery_id>
	// SK: STATUS
	// With a TTL of 7 days for successful deliveries, 30 days for failures

	logger.Debug("storing delivery status",
		zap.String("delivery_id", status.DeliveryID),
		zap.String("status", status.Status))

	// TODO: Implement actual DynamoDB storage
	return nil
}
