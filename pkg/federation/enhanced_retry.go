package federation

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"go.uber.org/zap"
)

// EnhancedRetryMessage represents a message for polynomial retry delivery
type EnhancedRetryMessage struct {
	DeliveryID        string                `json:"delivery_id"`
	Activity          *activitypub.Activity `json:"activity"`
	SigningActorID    string                `json:"signing_actor_id"`
	ActivityType      string                `json:"activity_type"`
	RetryCount        int                   `json:"retry_count"`
	MaxRetries        int                   `json:"max_retries"`
	RetryPolicy       string                `json:"retry_policy"`
	MaxRetryDuration  time.Duration         `json:"max_retry_duration"`
	CreatedAt         time.Time             `json:"created_at"`
	NextRetryAt       time.Time             `json:"next_retry_at"`
	TargetInboxes     []string              `json:"target_inboxes,omitempty"`
	Recipients        []string              `json:"recipients,omitempty"`
	FailedInboxes     map[string]string     `json:"failed_inboxes,omitempty"` // inbox -> error message
	SuccessfulInboxes []string              `json:"successful_inboxes,omitempty"`
}

// EnhancedRetryProcessor handles polynomial retry delivery for critical activities
type EnhancedRetryProcessor struct {
	deliveryService *DeliveryService
	logger          *zap.Logger
	sqsClient       *sqs.Client
	queueURL        string
}

// NewEnhancedRetryProcessor creates a new enhanced retry processor
func NewEnhancedRetryProcessor(deliveryService *DeliveryService, sqsClient *sqs.Client, queueURL string) *EnhancedRetryProcessor {
	return &EnhancedRetryProcessor{
		deliveryService: deliveryService,
		logger:          deliveryService.logger,
		sqsClient:       sqsClient,
		queueURL:        queueURL,
	}
}

// QueueForEnhancedRetry queues an activity for polynomial retry delivery
func (p *EnhancedRetryProcessor) QueueForEnhancedRetry(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor, recipients []string, activityType string) error {
	deliveryID := generateEnhancedDeliveryID()

	message := &EnhancedRetryMessage{
		DeliveryID:        deliveryID,
		Activity:          activity,
		SigningActorID:    actor.PreferredUsername,
		ActivityType:      activityType,
		RetryCount:        0,
		MaxRetries:        25, // 25 attempts as per requirements
		RetryPolicy:       "polynomial",
		MaxRetryDuration:  20 * 24 * time.Hour, // 20 days
		CreatedAt:         time.Now(),
		NextRetryAt:       time.Now().Add(p.calculatePolynomialDelay(1)), // First retry
		Recipients:        recipients,
		FailedInboxes:     make(map[string]string),
		SuccessfulInboxes: make([]string, 0),
	}

	// Serialize message
	messageJSON, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal enhanced retry message: %w", err)
	}

	// Calculate delay for first retry
	delay := p.calculatePolynomialDelay(1)

	// Send to SQS queue with delay
	if p.sqsClient != nil {
		_, err = p.sqsClient.SendMessage(ctx, &sqs.SendMessageInput{
			QueueUrl:     aws.String(p.queueURL),
			MessageBody:  aws.String(string(messageJSON)),
			DelaySeconds: int32(delay.Seconds()),
			MessageAttributes: map[string]types.MessageAttributeValue{
				"delivery_type": {
					DataType:    aws.String("String"),
					StringValue: aws.String("enhanced_retry"),
				},
				"activity_type": {
					DataType:    aws.String("String"),
					StringValue: aws.String(activityType),
				},
				"retry_count": {
					DataType:    aws.String("Number"),
					StringValue: aws.String("1"),
				},
			},
		})
		if err != nil {
			return fmt.Errorf("failed to queue for enhanced retry: %w", err)
		}
	}

	p.logger.Info("Activity queued for enhanced retry",
		zap.String("delivery_id", deliveryID),
		zap.String("activity_id", activity.ID),
		zap.String("activity_type", activityType),
		zap.Int("recipients", len(recipients)),
		zap.Duration("first_retry_delay", delay))

	return nil
}

// ProcessEnhancedRetry processes a retry message with polynomial backoff
func (p *EnhancedRetryProcessor) ProcessEnhancedRetry(ctx context.Context, message *EnhancedRetryMessage) error {
	log := p.logger.With(
		zap.String("delivery_id", message.DeliveryID),
		zap.String("activity_id", message.Activity.ID),
		zap.Int("retry_count", message.RetryCount),
		zap.String("activity_type", message.ActivityType))

	// Check if we've exceeded max retries or time limit
	if message.RetryCount >= message.MaxRetries {
		log.Warn("Maximum retry attempts exceeded, giving up")
		return p.recordFinalFailure(ctx, message, "max_retries_exceeded")
	}

	if time.Since(message.CreatedAt) > message.MaxRetryDuration {
		log.Warn("Maximum retry duration exceeded, giving up")
		return p.recordFinalFailure(ctx, message, "max_duration_exceeded")
	}

	// Get signing actor
	signingActor, err := p.deliveryService.store.GetActor(ctx, message.SigningActorID)
	if err != nil {
		log.Error("Failed to get signing actor", zap.Error(err))
		return p.requeueForRetry(ctx, message)
	}

	// Attempt delivery to failed/pending inboxes
	partialSuccess := false
	newFailures := make(map[string]string)

	// Determine which inboxes to retry
	inboxesToRetry := make(map[string]bool)
	if len(message.TargetInboxes) > 0 {
		// Retry specific inboxes
		for _, inbox := range message.TargetInboxes {
			if !contains(message.SuccessfulInboxes, inbox) {
				inboxesToRetry[inbox] = true
			}
		}
	} else {
		// Retry recipients that haven't succeeded
		for _, recipient := range message.Recipients {
			// Would need to resolve recipient to inbox - simplified here
			inboxesToRetry[recipient] = true
		}
	}

	// Attempt delivery to each inbox
	for inbox := range inboxesToRetry {
		if err := p.deliveryService.DeliverActivity(ctx, message.Activity, inbox, signingActor); err != nil {
			log.Warn("Retry delivery failed for inbox",
				zap.String("inbox", inbox),
				zap.Error(err))
			newFailures[inbox] = err.Error()
		} else {
			log.Info("Retry delivery successful for inbox",
				zap.String("inbox", inbox))
			message.SuccessfulInboxes = append(message.SuccessfulInboxes, inbox)
			partialSuccess = true
		}
	}

	// Update failed inboxes
	for inbox, errMsg := range newFailures {
		message.FailedInboxes[inbox] = errMsg
	}

	// Check if all deliveries succeeded
	if len(newFailures) == 0 {
		log.Info("All retry deliveries successful")
		return p.recordFinalSuccess(ctx, message)
	}

	// If we had partial success, log it
	if partialSuccess {
		log.Info("Partial success in retry delivery",
			zap.Int("successful", len(message.SuccessfulInboxes)),
			zap.Int("failed", len(newFailures)))
	}

	// Requeue for next retry
	return p.requeueForRetry(ctx, message)
}

// calculatePolynomialDelay calculates delay using polynomial formula: attempt + 15 seconds + jitter
func (p *EnhancedRetryProcessor) calculatePolynomialDelay(attempt int) time.Duration {
	// Polynomial delay = attempt + 15 seconds + jitter
	baseDelay := time.Duration(attempt)*time.Second + 15*time.Second

	// Add jitter (up to 5 seconds)
	jitter := time.Duration(p.generateJitter()) * time.Second

	return baseDelay + jitter
}

// generateJitter generates a random jitter between 0-5 seconds
func (p *EnhancedRetryProcessor) generateJitter() int {
	b := make([]byte, 1)
	if _, err := rand.Read(b); err != nil {
		return int(time.Now().UnixNano() % 5)
	}
	return int(b[0]) % 5
}

// requeueForRetry requeues a message for the next retry attempt
func (p *EnhancedRetryProcessor) requeueForRetry(ctx context.Context, message *EnhancedRetryMessage) error {
	// Increment retry count
	message.RetryCount++

	// Calculate next retry delay
	delay := p.calculatePolynomialDelay(message.RetryCount)
	message.NextRetryAt = time.Now().Add(delay)

	// Serialize updated message
	messageJSON, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal retry message: %w", err)
	}

	// Requeue with delay
	if p.sqsClient != nil {
		_, err = p.sqsClient.SendMessage(ctx, &sqs.SendMessageInput{
			QueueUrl:     aws.String(p.queueURL),
			MessageBody:  aws.String(string(messageJSON)),
			DelaySeconds: int32(math.Min(float64(delay.Seconds()), 900)), // SQS max delay is 15 minutes
			MessageAttributes: map[string]types.MessageAttributeValue{
				"delivery_type": {
					DataType:    aws.String("String"),
					StringValue: aws.String("enhanced_retry"),
				},
				"retry_count": {
					DataType:    aws.String("Number"),
					StringValue: aws.String(fmt.Sprintf("%d", message.RetryCount)),
				},
			},
		})
		if err != nil {
			return fmt.Errorf("failed to requeue for retry: %w", err)
		}
	}

	p.logger.Info("Requeued for retry",
		zap.String("delivery_id", message.DeliveryID),
		zap.Int("retry_count", message.RetryCount),
		zap.Duration("delay", delay))

	return nil
}

// recordFinalSuccess records successful completion of all deliveries
func (p *EnhancedRetryProcessor) recordFinalSuccess(ctx context.Context, message *EnhancedRetryMessage) error {
	// Record federation activity as successful
	activity := &storage.FederationActivity{
		ID:           message.DeliveryID,
		ActivityType: message.ActivityType,
		ActorID:      message.SigningActorID,
		Status:       "delivered_with_retry",
		Success:      true,
		Timestamp:    time.Now(),
		Data: map[string]interface{}{
			"retry_count":        message.RetryCount,
			"successful_inboxes": message.SuccessfulInboxes,
			"total_attempts":     message.RetryCount + 1,
		},
	}

	return p.deliveryService.store.RecordFederationActivity(ctx, activity)
}

// recordFinalFailure records permanent failure after exhausting retries
func (p *EnhancedRetryProcessor) recordFinalFailure(ctx context.Context, message *EnhancedRetryMessage, reason string) error {
	// Record federation activity as failed
	activity := &storage.FederationActivity{
		ID:           message.DeliveryID,
		ActivityType: message.ActivityType,
		ActorID:      message.SigningActorID,
		Status:       "failed_permanently",
		Success:      false,
		ErrorMessage: fmt.Sprintf("Permanent failure: %s after %d attempts", reason, message.RetryCount),
		Timestamp:    time.Now(),
		Data: map[string]interface{}{
			"retry_count":        message.RetryCount,
			"failed_inboxes":     message.FailedInboxes,
			"successful_inboxes": message.SuccessfulInboxes,
			"failure_reason":     reason,
		},
	}

	return p.deliveryService.store.RecordFederationActivity(ctx, activity)
}

// contains checks if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// generateEnhancedDeliveryID generates a unique delivery ID for enhanced retry
func generateEnhancedDeliveryID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("enhanced_%x", time.Now().UnixNano())
	}
	return fmt.Sprintf("enhanced_%x", b)
}
