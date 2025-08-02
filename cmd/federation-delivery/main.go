package main

// Federation Delivery Lambda Function
//
// This function has been migrated to use:
// - Lift framework for SQS event handling and Lambda patterns
// - DynamORM for type-safe DynamoDB operations instead of direct AWS SDK
// - Structured logging with request IDs
// - Proper error handling and middleware composition
//
// The migration preserves all existing functionality:
// - Activity delivery to remote ActivityPub servers
// - Exponential backoff retry logic
// - Dead letter queue handling for permanent failures
// - HTTP signature authentication
// - Cost tracking and federation analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// FederationDeliveryProcessor handles federation delivery from SQS messages
type FederationDeliveryProcessor struct {
	storage         *dynamorm.StorageAdapter  // For storing delivery status
	deliveryService *federation.DeliveryService
	cfg             *config.Config
	sqsClient       *sqs.Client
	queueURL        string
	logger          *zap.Logger
}

var processor *FederationDeliveryProcessor

// FederationStorageAdapter adapts the DynamORM storage to implement FederationStorage interface
type FederationStorageAdapter struct {
	storage *dynamorm.StorageAdapter
}

// Ensure we implement the FederationStorage interface
var _ federation.FederationStorage = (*FederationStorageAdapter)(nil)

func (f *FederationStorageAdapter) GetActorPrivateKey(ctx context.Context, username string) (string, error) {
	return f.storage.GetActorPrivateKey(ctx, username)
}

func (f *FederationStorageAdapter) GetActor(ctx context.Context, username string) (*activitypub.Actor, error) {
	return f.storage.GetActor(ctx, username)
}

func (f *FederationStorageAdapter) GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	return f.storage.GetFollowers(ctx, username, limit, cursor)
}

func (f *FederationStorageAdapter) GetCachedRemoteActor(ctx context.Context, actorID string) (*activitypub.Actor, error) {
	return f.storage.GetCachedRemoteActor(ctx, actorID)
}

func (f *FederationStorageAdapter) CacheRemoteActor(ctx context.Context, handle string, actor *activitypub.Actor, ttl time.Duration) error {
	return f.storage.CacheRemoteActor(ctx, handle, actor, ttl)
}

func (f *FederationStorageAdapter) RecordFederationActivity(ctx context.Context, activity *storage.FederationActivity) error {
	return f.storage.RecordFederationActivity(ctx, activity)
}

func init() {
	var err error
	logger := common.Logger()
	cfg := config.Get()

	// Initialize DynamORM with Lambda optimizations
	db, err := dynamorm.NewLambdaOptimizedClient(context.Background(), cfg.Region)
	if err != nil {
		logger.Fatal("Failed to initialize DynamORM", zap.Error(err))
	}

	// Initialize DynamORM storage adapter
	storageAdapter := dynamorm.NewStorageAdapter(db, cfg.DynamoTableName, logger, nil)
	
	// Create federation storage adapter that implements FederationStorage interface
	federationStore := &FederationStorageAdapter{storage: storageAdapter}

	// Initialize federation delivery service
	deliveryService := federation.NewDeliveryService(federationStore)

	// Initialize AWS SQS client
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background())
	if err != nil {
		logger.Fatal("Failed to load AWS config", zap.Error(err))
	}
	sqsClient := sqs.NewFromConfig(awsCfg)

	// Get queue URL from environment
	queueURL := cfg.FederationDeliveryQueueURL
	if queueURL == "" {
		logger.Fatal("FEDERATION_DELIVERY_QUEUE_URL environment variable is required")
	}

	// Create processor instance
	processor = &FederationDeliveryProcessor{
		storage:         storageAdapter,
		deliveryService: deliveryService,
		cfg:             cfg,
		sqsClient:       sqsClient,
		queueURL:        queueURL,
		logger:          logger,
	}
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
	// Create Lift app
	app := lift.New()

	// Add request ID middleware
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			requestID := fmt.Sprintf("federation-delivery-%d", time.Now().UnixNano())
			ctx.Set("requestID", requestID)
			return next.Handle(ctx)
		})
	})

	// Add logging middleware
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			start := time.Now()
			requestID := ctx.Get("requestID").(string)
			processor.logger.Info("processing SQS batch",
				zap.String("request_id", requestID),
				zap.Time("start_time", start))
			
			err := next.Handle(ctx)
			
			processor.logger.Info("completed SQS batch",
				zap.String("request_id", requestID),
				zap.Duration("duration", time.Since(start)),
				zap.Error(err))
			
			return err
		})
	})

	// Handle SQS events (using Lift pattern from notification-processor)
	app.Handle("POST", "/", func(ctx *lift.Context) error {
		// Parse event from context
		var event events.SQSEvent
		if sqsEvent, ok := ctx.Request.RawEvent.(events.SQSEvent); ok {
			event = sqsEvent
		} else {
			// Try to parse from interface if it's a map
			eventBytes, err := json.Marshal(ctx.Request.RawEvent)
			if err != nil {
				return lift.NewLiftError("EVENT_PARSE_ERROR", "failed to marshal raw event", 500).WithCause(err)
			}

			if err := json.Unmarshal(eventBytes, &event); err != nil {
				return lift.NewLiftError("EVENT_PARSE_ERROR", "failed to parse SQS event", 500).WithCause(err)
			}
		}

		return processor.HandleSQS(ctx, event)
	})

	lambda.Start(app.HandleRequest)
}

// HandleSQS implements the SQS handler interface for Lift
func (p *FederationDeliveryProcessor) HandleSQS(ctx *lift.Context, event events.SQSEvent) error {
	p.logger.Info("processing federation delivery batch",
		zap.String("request_id", ctx.GetRequestID()),
		zap.Int("message_count", len(event.Records)))

	// Process messages sequentially (federation delivery should be reliable)
	for _, record := range event.Records {
		if err := p.handleDeliveryMessage(ctx, record); err != nil {
			p.logger.Error("failed to process delivery message",
				zap.String("message_id", record.MessageId),
				zap.String("request_id", ctx.GetRequestID()),
				zap.Error(err))
			// Return error to let SQS handle retry
			return err
		}
	}

	return nil
}

// handleDeliveryMessage processes a single SQS message with Lift context
func (p *FederationDeliveryProcessor) handleDeliveryMessage(ctx *lift.Context, message events.SQSMessage) error {
	// Parse the message
	var msg FederationDeliveryMessage
	if err := common.ParseRequestBody([]byte(message.Body), &msg); err != nil {
		return fmt.Errorf("failed to parse message: %w", err)
	}

	p.logger.Info("processing federation delivery",
		zap.String("delivery_id", msg.DeliveryID),
		zap.String("target_inbox", msg.TargetInbox),
		zap.Int("retry_count", msg.RetryCount),
		zap.String("request_id", ctx.GetRequestID()))

	// Process the delivery message using standard context
	return p.processDeliveryMessage(ctx.Request.Context(), p.logger, msg)
}

// processDeliveryMessage handles the actual delivery logic
func (p *FederationDeliveryProcessor) processDeliveryMessage(ctx context.Context, logger *zap.Logger, msg FederationDeliveryMessage) error {

	// Check if we should retry yet
	if msg.NextRetryAfter != nil && time.Now().Before(*msg.NextRetryAfter) {
		// Not time to retry yet, acknowledge message but don't process
		logger.Debug("skipping delivery, not time to retry yet",
			zap.String("delivery_id", msg.DeliveryID),
			zap.Time("retry_after", *msg.NextRetryAfter))
		return nil
	}

	// Get the signing actor
	signingActor, err := p.storage.GetActor(ctx, msg.SigningActorID)
	if err != nil {
		return fmt.Errorf("failed to get signing actor: %w", err)
	}

	// Create delivery status record using DynamORM model
	deliveryStatus := &models.DeliveryStatus{
		ActivityID:   msg.DeliveryID,
		TargetDomain: extractDomainFromURL(msg.TargetInbox),
		Status:       "attempting",
		Attempts:     msg.RetryCount + 1,
		LastAttempt:  time.Now(),
		CreatedAt:    msg.CreatedAt,
	}
	deliveryStatus.UpdateKeys()

	// Attempt delivery
	err = p.deliveryService.DeliverActivity(ctx, msg.Activity, msg.TargetInbox, signingActor)
	if err != nil {
		deliveryStatus.Status = "failed"
		deliveryStatus.Error = err.Error()

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
			msg.LastAttemptAt = &deliveryStatus.LastAttempt
			msg.NextRetryAfter = &nextRetry
			msg.FailureReason = err.Error()

			// Update delivery status for retry
			deliveryStatus.NextRetry = nextRetry
			deliveryStatus.UpdateKeys()

			// Send back to queue with delay
			if err := p.requeueDelivery(ctx, &msg, backoffMinutes); err != nil {
				logger.Error("failed to requeue delivery",
					zap.String("delivery_id", msg.DeliveryID),
					zap.Error(err))
			}

			// Store delivery status
			if err := p.storeDeliveryStatus(ctx, deliveryStatus); err != nil {
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

		deliveryStatus.Status = "permanently_failed"
		deliveryStatus.UpdateKeys()

		// Store final status
		if err := p.storeDeliveryStatus(ctx, deliveryStatus); err != nil {
			logger.Error("failed to store delivery status",
				zap.String("delivery_id", msg.DeliveryID),
				zap.Error(err))
		}

		// Send to dead letter queue by returning error
		return fmt.Errorf("delivery permanently failed after %d attempts: %w", msg.RetryCount+1, err)
	}

	// Success!
	deliveredAt := time.Now()
	deliveryStatus.Status = "delivered"
	deliveryStatus.DeliveredAt = deliveredAt
	deliveryStatus.UpdateKeys()

	logger.Info("successfully delivered activity",
		zap.String("delivery_id", msg.DeliveryID),
		zap.String("target_inbox", msg.TargetInbox),
		zap.Int("attempts", msg.RetryCount+1))

	// Store successful delivery status
	if err := p.storeDeliveryStatus(ctx, deliveryStatus); err != nil {
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
func (p *FederationDeliveryProcessor) requeueDelivery(ctx context.Context, msg *FederationDeliveryMessage, delayMinutes int) error {
	// Marshal the updated message
	messageBody, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Calculate delay seconds (SQS DelaySeconds max is 900 seconds / 15 minutes)
	delaySeconds := delayMinutes * 60
	if delaySeconds > 900 {
		delaySeconds = 900 // Max SQS delay
	}

	// Send message to SQS with delay
	_, err = p.sqsClient.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:     aws.String(p.queueURL),
		MessageBody:  aws.String(string(messageBody)),
		DelaySeconds: int32(delaySeconds),
		MessageAttributes: map[string]types.MessageAttributeValue{
			"retry_count": {
				StringValue: aws.String(fmt.Sprintf("%d", msg.RetryCount)),
				DataType:    aws.String("Number"),
			},
			"delivery_id": {
				StringValue: aws.String(msg.DeliveryID),
				DataType:    aws.String("String"),
			},
		},
	})
	if err != nil {
		p.logger.Error("failed to send message to SQS",
			zap.String("delivery_id", msg.DeliveryID),
			zap.Int("delay_minutes", delayMinutes),
			zap.Error(err))
		return fmt.Errorf("failed to requeue message: %w", err)
	}

	p.logger.Info("message requeued with delay",
		zap.String("delivery_id", msg.DeliveryID),
		zap.Int("delay_minutes", delayMinutes),
		zap.Int("delay_seconds", delaySeconds))

	return nil
}

// storeDeliveryStatus stores the delivery status using DynamORM
func (p *FederationDeliveryProcessor) storeDeliveryStatus(ctx context.Context, status *models.DeliveryStatus) error {
	p.logger.Debug("storing delivery status",
		zap.String("activity_id", status.ActivityID),
		zap.String("status", status.Status),
		zap.String("pk", status.PK),
		zap.String("sk", status.SK))

	// Use the CreateObject method to store the delivery status record
	// The DynamORM storage adapter will handle the operations
	return p.storage.CreateObject(ctx, status)
}

// extractDomainFromURL extracts the domain from a URL (helper function)
func extractDomainFromURL(urlStr string) string {
	// Simple extraction - matches the federation package implementation
	if len(urlStr) > 8 && urlStr[:8] == "https://" {
		parts := urlStr[8:]
		for i, c := range parts {
			if c == '/' {
				return parts[:i]
			}
		}
		return parts
	}
	return "unknown"
}
