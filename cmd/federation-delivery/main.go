// Package main implements the federation-delivery Lambda function for delivering ActivityPub messages to remote instances.
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
	"os"
	"strings"
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
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	pkgErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/lambdastorage"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/models"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
	"go.uber.org/zap"
)

const (
	// Error type constants
	errorTypePermanent = "permanent"
	errorTypeTemporary = "temporary"
)

// FederationDeliveryProcessor handles federation delivery from SQS messages
type FederationDeliveryProcessor struct {
	repos           core.RepositoryStorage // Repository storage for data access
	deliveryService *federation.DeliveryService
	cfg             *config.Config
	sqsClient       *sqs.Client
	queueURL        string
	logger          *zap.Logger
}

var processor *FederationDeliveryProcessor

// FederationStorageAdapter adapts the repository storage to implement FederationStorage interface
type FederationStorageAdapter struct {
	repos core.RepositoryStorage
}

// Ensure we implement the FederationStorage interface
var _ federation.FederationStorage = (*FederationStorageAdapter)(nil)

// GetActorPrivateKey retrieves the private key for an actor
func (f *FederationStorageAdapter) GetActorPrivateKey(ctx context.Context, username string) (string, error) {
	return f.repos.Account().GetActorPrivateKey(ctx, username)
}

// GetActor retrieves an actor by username
func (f *FederationStorageAdapter) GetActor(ctx context.Context, username string) (*activitypub.Actor, error) {
	return f.repos.Account().GetActor(ctx, username)
}

// GetFollowers retrieves a paginated list of followers for a user
func (f *FederationStorageAdapter) GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	return f.repos.Relationship().GetFollowers(ctx, username, limit, cursor)
}

// GetCachedRemoteActor retrieves a cached remote actor by ID
func (f *FederationStorageAdapter) GetCachedRemoteActor(ctx context.Context, actorID string) (*activitypub.Actor, error) {
	return f.repos.Actor().GetCachedRemoteActor(ctx, actorID)
}

// CacheRemoteActor caches a remote actor with a TTL
func (f *FederationStorageAdapter) CacheRemoteActor(ctx context.Context, handle string, actor *activitypub.Actor, ttl time.Duration) error {
	return f.repos.User().CacheRemoteActor(ctx, handle, actor, ttl)
}

// RecordFederationActivity records a federation activity for analytics
func (f *FederationStorageAdapter) RecordFederationActivity(ctx context.Context, activity *storage.FederationActivity) error {
	return f.repos.Federation().RecordFederationActivity(ctx, activity)
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

var (
	lambdaCtx *common.LambdaContext
	cfg       *config.Config
	logger    *zap.Logger
	repos     core.RepositoryStorage
)

var (
	runningUnitTestsFn     = common.RunningUnitTests
	mustInitializeLambdaFn = common.MustInitializeLambda
	loadAWSConfigFn        = awsconfig.LoadDefaultConfig
	newSQSClientFn         = sqs.NewFromConfig
	newDeliveryServiceFn   = federation.NewDeliveryService
	lambdaStartFn          = lambda.Start

	getSigningActorFn = func(ctx context.Context, storage core.RepositoryStorage, signingActorID string) (*activitypub.Actor, error) {
		return storage.Account().GetActor(ctx, signingActorID)
	}
	getInstanceStatsFn = func(ctx context.Context, storage core.RepositoryStorage, domain string) (*storage.InstanceStats, error) {
		return storage.Federation().GetInstanceStats(ctx, domain)
	}
	recordFederationActivityFn = func(ctx context.Context, storage core.RepositoryStorage, activity *storage.FederationActivity) error {
		return storage.Federation().RecordFederationActivity(ctx, activity)
	}
	createObjectFn = func(ctx context.Context, storage core.RepositoryStorage, obj any) error {
		return storage.Object().CreateObject(ctx, obj)
	}
	deliverActivityFn = func(ctx context.Context, svc *federation.DeliveryService, activity *activitypub.Activity, targetInbox string, signingActor *activitypub.Actor) error {
		return svc.DeliverActivity(ctx, activity, targetInbox, signingActor)
	}
	sendSQSMessageFn = func(ctx context.Context, client *sqs.Client, input *sqs.SendMessageInput) (*sqs.SendMessageOutput, error) {
		return client.SendMessage(ctx, input)
	}
)

func init() {
	initializeFederationDeliveryOnStart()
}

func initializeFederationDeliveryOnStart() {
	if runningUnitTestsFn() {
		return
	}
	if err := initializeFederationDelivery(); err != nil {
		if logger == nil {
			logger = zap.NewNop()
		}
		logger.Fatal("failed to initialize federation-delivery lambda", zap.Error(err))
	}
}

func initializeFederationDelivery() error {
	// Standardized Lambda initialization for federation-delivery function
	lambdaCtx = mustInitializeLambdaFn(common.LambdaConfig{
		ServiceName: "federation-delivery",      // federation-delivery
		LambdaType:  common.LambdaTypeProcessor, // These are background processing functions
	})

	// Automatic dependency injection
	cfg = lambdaCtx.Config
	logger = lambdaCtx.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	deps, err := lambdastorage.Initialize(context.Background(), lambdaCtx, lambdastorage.Options{
		ServiceName:         "federation-delivery",
		RequireRepositories: true,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}
	repos = deps.Repos

	// Function-specific initialization only
	// Initialize AWS SQS client config
	awsCfg, err := loadAWSConfigFn(context.Background())
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create federation storage adapter that implements FederationStorage interface
	federationStore := &FederationStorageAdapter{repos: repos}

	// Initialize federation delivery service
	deliveryService := newDeliveryServiceFn(federationStore, cfg)

	sqsClient := newSQSClientFn(awsCfg)

	// Get queue URL from environment
	queueURL := cfg.FederationQueueURL
	if err := common.ValidateRequiredParam("queueURL", queueURL); err != nil {
		return fmt.Errorf("FEDERATION_DELIVERY_QUEUE_URL environment variable is required")
	}

	// Create processor instance
	processor = &FederationDeliveryProcessor{
		repos:           repos,
		deliveryService: deliveryService,
		cfg:             cfg,
		sqsClient:       sqsClient,
		queueURL:        queueURL,
		logger:          logger,
	}
	return nil
}

func main() {
	app := apptheory.New()

	appName := strings.TrimSpace(os.Getenv("APP_NAME"))
	stage := strings.TrimSpace(os.Getenv("STAGE"))
	queueName := naming.ResourceNameWithApp(appName, "federation-delivery-queue", stage)

	app.SQS(queueName, func(ctx *apptheory.EventContext, msg events.SQSMessage) error {
		if processor == nil {
			return fmt.Errorf("federation delivery processor not initialized")
		}
		return processor.HandleSQSMessage(ctx, msg)
	})

	lambdaStartFn(func(ctx context.Context, event json.RawMessage) (any, error) {
		return app.HandleLambda(ctx, event)
	})
}

func (p *FederationDeliveryProcessor) HandleSQSMessage(ctx *apptheory.EventContext, message events.SQSMessage) error {
	requestID := ""
	runCtx := context.Background()
	if ctx != nil {
		requestID = ctx.RequestID
		runCtx = ctx.Context()
	}
	return p.handleDeliveryMessage(runCtx, requestID, message)
}

func (p *FederationDeliveryProcessor) handleDeliveryMessage(ctx context.Context, requestID string, message events.SQSMessage) error {
	// Parse the message
	var msg FederationDeliveryMessage
	if err := common.ParseRequestBody([]byte(message.Body), &msg); err != nil {
		p.logger.Error("failed to parse message body",
			zap.String("message_id", message.MessageId),
			zap.String("request_id", requestID),
			zap.Error(err))
		return pkgErrors.WrapError(err, pkgErrors.CodeBadRequest, pkgErrors.CategoryLambda, "Invalid message body format")
	}

	p.logger.Info("processing federation delivery",
		zap.String("delivery_id", msg.DeliveryID),
		zap.String("target_inbox", msg.TargetInbox),
		zap.Int("retry_count", msg.RetryCount),
		zap.String("request_id", requestID))

	// Process the delivery message using standard context
	return p.processDeliveryMessage(ctx, p.logger, msg)
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
	signingActor, err := getSigningActorFn(ctx, p.repos, msg.SigningActorID)
	if err != nil {
		logger.Error("signing actor not found",
			zap.String("signing_actor_id", msg.SigningActorID),
			zap.Error(err))
		return pkgErrors.FederationDeliverySigningActorMissing()
	}

	targetDomain := extractDomainFromURL(msg.TargetInbox)

	// Perform health assessment before delivery
	shouldDeliver, healthReason := p.assessTargetHealth(ctx, targetDomain, msg.RetryCount)
	if !shouldDeliver {
		logger.Warn("skipping delivery due to target health",
			zap.String("delivery_id", msg.DeliveryID),
			zap.String("target_domain", targetDomain),
			zap.String("reason", healthReason),
			zap.Int("retry_count", msg.RetryCount))

		// If health is poor and we've tried multiple times, delay significantly
		if msg.RetryCount >= 2 {
			delayMinutes := calculateHealthBasedBackoff(msg.RetryCount, healthReason)
			nextRetry := time.Now().Add(time.Duration(delayMinutes) * time.Minute)

			// Update message for delayed retry
			now := time.Now()
			msg.RetryCount++
			msg.LastAttemptAt = &now
			msg.NextRetryAfter = &nextRetry
			msg.FailureReason = fmt.Sprintf("Health assessment failed: %s", healthReason)

			// Requeue with delay
			if err := p.requeueDelivery(ctx, &msg, delayMinutes); err != nil {
				logger.Error("failed to requeue health-delayed delivery",
					zap.String("delivery_id", msg.DeliveryID),
					zap.Error(err))
			}
			return nil
		}
	}

	// Create delivery status record using DynamORM model
	deliveryStatus := &models.DeliveryStatus{
		ActivityID:   msg.DeliveryID,
		TargetDomain: targetDomain,
		Status:       "attempting",
		Attempts:     msg.RetryCount + 1,
		LastAttempt:  time.Now(),
		CreatedAt:    msg.CreatedAt,
	}
	deliveryStatus.UpdateKeys()

	// Attempt delivery with optimized routing
	err = p.deliverWithRoutingOptimization(ctx, msg.Activity, msg.TargetInbox, signingActor, targetDomain)
	if err != nil {
		deliveryStatus.Status = "failed"
		deliveryStatus.Error = err.Error()

		// Classify the error to determine if we should retry
		errorType := p.classifyDeliveryError(err)

		logger.Info("delivery failed - analyzing error",
			zap.String("delivery_id", msg.DeliveryID),
			zap.String("error_type", errorType),
			zap.String("error_message", err.Error()),
			zap.Int("retry_count", msg.RetryCount),
			zap.Int("max_retries", msg.MaxRetries))

		// Check if this is a permanent error or max retries exceeded
		if errorType == errorTypePermanent || msg.RetryCount >= msg.MaxRetries {
			if errorType == errorTypePermanent {
				logger.Warn("permanent error detected, not retrying",
					zap.String("delivery_id", msg.DeliveryID),
					zap.String("error", err.Error()))
			} else {
				logger.Error("delivery failed after max retries",
					zap.String("delivery_id", msg.DeliveryID),
					zap.Int("attempts", msg.RetryCount+1),
					zap.Error(err))
			}

			deliveryStatus.Status = "permanently_failed"
			deliveryStatus.UpdateKeys()

			// Store final status
			if err := p.storeDeliveryStatus(ctx, deliveryStatus); err != nil {
				logger.Error("failed to store delivery status",
					zap.String("delivery_id", msg.DeliveryID),
					zap.Error(err))
			}

			// Send to dead letter queue by returning error
			logger.Error("delivery permanently failed",
				zap.String("delivery_id", msg.DeliveryID),
				zap.Int("total_attempts", msg.RetryCount+1),
				zap.String("error_type", errorType))
			return pkgErrors.FederationDeliveryMaxAttemptsExceeded()
		}

		// This is a temporary error - schedule retry
		backoffMinutes := p.calculateRetryBackoff(msg.RetryCount, errorType, targetDomain)
		nextRetry := time.Now().Add(time.Duration(backoffMinutes) * time.Minute)

		logger.Warn("temporary error detected, scheduling retry",
			zap.String("delivery_id", msg.DeliveryID),
			zap.String("error_type", errorType),
			zap.Int("retry_count", msg.RetryCount),
			zap.Time("next_retry", nextRetry),
			zap.Int("backoff_minutes", backoffMinutes),
			zap.Error(err))

		// Update message for retry
		msg.RetryCount++
		msg.LastAttemptAt = &deliveryStatus.LastAttempt
		msg.NextRetryAfter = &nextRetry
		msg.FailureReason = fmt.Sprintf("%s: %s", errorType, err.Error())

		// Update delivery status for retry
		deliveryStatus.Status = "retrying"
		deliveryStatus.NextRetry = nextRetry
		deliveryStatus.UpdateKeys()

		// Send back to queue with delay
		if err := p.requeueDelivery(ctx, &msg, backoffMinutes); err != nil {
			logger.Error("failed to requeue delivery",
				zap.String("delivery_id", msg.DeliveryID),
				zap.Error(err))
			// Return the original delivery error since requeuing failed
			return err
		}

		// Store delivery status
		if err := p.storeDeliveryStatus(ctx, deliveryStatus); err != nil {
			logger.Error("failed to store delivery status",
				zap.String("delivery_id", msg.DeliveryID),
				zap.Error(err))
		}

		// Log retry metrics
		p.recordRetryMetrics(ctx, msg.DeliveryID, errorType, msg.RetryCount, backoffMinutes)

		// Don't return error - we've handled the retry
		return nil
	}

	// Success!
	deliveredAt := time.Now()
	deliveryStatus.Status = "delivered"
	deliveryStatus.DeliveredAt = deliveredAt
	deliveryStatus.UpdateKeys()

	logger.Info("successfully delivered activity",
		zap.String("delivery_id", msg.DeliveryID),
		zap.String("target_inbox", msg.TargetInbox),
		zap.String("target_domain", targetDomain),
		zap.Int("attempts", msg.RetryCount+1),
		zap.Duration("total_time", time.Since(msg.CreatedAt)))

	// Record success metrics
	p.recordSuccessMetrics(ctx, msg.DeliveryID, msg.Activity.Type, targetDomain, msg.RetryCount+1, time.Since(msg.CreatedAt))

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
		p.logger.Error("failed to marshal message for requeue",
			zap.String("delivery_id", msg.DeliveryID),
			zap.Error(err))
		return pkgErrors.FederationDeliveryMessageMarshalFailure()
	}

	// Calculate delay seconds (SQS DelaySeconds max is 900 seconds / 15 minutes)
	delayMinutesInt := delayMinutes
	if delayMinutesInt < 0 {
		delayMinutesInt = 0
	}

	delaySeconds := delayMinutesInt * 60
	if delaySeconds > 900 {
		delaySeconds = 900 // Max SQS delay
	}

	// Since SQS DelaySeconds is always bounded to [0, 900],
	// and 900 < max int32 (2,147,483,647), the conversion is mathematically safe
	// #nosec G115 - Safe conversion: SQS DelaySeconds max is 900, well within int32 range
	delaySeconds32 := int32(delaySeconds)

	// Send message to SQS with delay
	_, err = sendSQSMessageFn(ctx, p.sqsClient, &sqs.SendMessageInput{
		QueueUrl:     aws.String(p.queueURL),
		MessageBody:  aws.String(string(messageBody)),
		DelaySeconds: delaySeconds32,
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
		return pkgErrors.FederationDeliveryMessageRequeueFailure()
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
	// The Object repository will handle the operations
	return createObjectFn(ctx, p.repos, status)
}

// assessTargetHealth performs health assessment of target domain before delivery
func (p *FederationDeliveryProcessor) assessTargetHealth(ctx context.Context, targetDomain string, retryCount int) (bool, string) {
	// Get instance health information
	instanceStats, err := getInstanceStatsFn(ctx, p.repos, targetDomain)
	if err != nil {
		p.logger.Debug("no instance stats available, allowing delivery",
			zap.String("domain", targetDomain),
			zap.Error(err))
		return true, "no_stats"
	}

	// Check basic health indicators
	if instanceStats.ErrorRate > 0.5 {
		return false, fmt.Sprintf("high_error_rate_%.2f", instanceStats.ErrorRate)
	}

	if instanceStats.AvgResponseTime > 30000 { // 30 seconds
		return false, fmt.Sprintf("slow_response_time_%.0fms", instanceStats.AvgResponseTime)
	}

	// For retry attempts, be more strict
	if retryCount > 0 {
		if instanceStats.ErrorRate > 0.2 {
			return false, fmt.Sprintf("retry_error_rate_%.2f", instanceStats.ErrorRate)
		}
		if instanceStats.AvgResponseTime > 15000 { // 15 seconds for retries
			return false, fmt.Sprintf("retry_slow_response_%.0fms", instanceStats.AvgResponseTime)
		}
	}

	// Check if instance was recently seen
	if time.Since(instanceStats.LastSeen) > 24*time.Hour {
		return false, fmt.Sprintf("stale_last_seen_%s", instanceStats.LastSeen.Format(time.RFC3339))
	}

	return true, "healthy"
}

// calculateHealthBasedBackoff calculates backoff based on health assessment
func calculateHealthBasedBackoff(retryCount int, healthReason string) int {
	baseBackoff := calculateBackoff(retryCount)

	// Increase backoff based on health issues
	switch {
	case strings.Contains(healthReason, "high_error_rate"):
		return baseBackoff * 3 // 3x longer for high error rates
	case strings.Contains(healthReason, "slow_response"):
		return baseBackoff * 2 // 2x longer for slow responses
	case strings.Contains(healthReason, "stale_last_seen"):
		return baseBackoff * 4 // 4x longer for stale instances
	default:
		return baseBackoff
	}
}

// deliverWithRoutingOptimization attempts delivery with route optimization
func (p *FederationDeliveryProcessor) deliverWithRoutingOptimization(ctx context.Context, activity *activitypub.Activity, targetInbox string, signingActor *activitypub.Actor, targetDomain string) error {
	// First try standard delivery
	err := deliverActivityFn(ctx, p.deliveryService, activity, targetInbox, signingActor)
	if err != nil {
		// Log delivery failure with routing context
		p.logger.Debug("standard delivery failed, no alternate routes available",
			zap.String("target_domain", targetDomain),
			zap.String("target_inbox", targetInbox),
			zap.Error(err))

		// Record routing failure analytics
		if analyticsErr := p.recordRoutingFailure(ctx, targetDomain, targetInbox, err); analyticsErr != nil {
			p.logger.Debug("failed to record routing analytics", zap.Error(analyticsErr))
		}

		return err
	}

	// Record successful routing analytics
	if analyticsErr := p.recordRoutingSuccess(ctx, targetDomain, targetInbox); analyticsErr != nil {
		p.logger.Debug("failed to record routing analytics", zap.Error(analyticsErr))
	}

	return nil
}

// recordRoutingFailure records failed delivery analytics
func (p *FederationDeliveryProcessor) recordRoutingFailure(ctx context.Context, targetDomain, _ string, deliveryErr error) error {
	activity := &storage.FederationActivity{
		ID:           fmt.Sprintf("delivery_failure_%d", time.Now().UnixNano()),
		Domain:       targetDomain,
		Type:         "egress",
		ActivityType: "delivery_failure",
		Success:      false,
		ErrorMessage: deliveryErr.Error(),
		Timestamp:    time.Now(),
		ResponseTime: 0, // Unknown since delivery failed
		ByteSize:     0, // Unknown since delivery failed
	}

	return recordFederationActivityFn(ctx, p.repos, activity)
}

// recordRoutingSuccess records successful delivery analytics
func (p *FederationDeliveryProcessor) recordRoutingSuccess(ctx context.Context, targetDomain, _ string) error {
	activity := &storage.FederationActivity{
		ID:           fmt.Sprintf("delivery_success_%d", time.Now().UnixNano()),
		Domain:       targetDomain,
		Type:         "egress",
		ActivityType: "delivery_success",
		Success:      true,
		Timestamp:    time.Now(),
		ResponseTime: 0, // Would need to measure in real implementation
		ByteSize:     0, // Would need to measure in real implementation
	}

	return recordFederationActivityFn(ctx, p.repos, activity)
}

// classifyDeliveryError classifies delivery errors as temporary or permanent
func (p *FederationDeliveryProcessor) classifyDeliveryError(err error) string {
	if err == nil {
		return "unknown"
	}

	errStr := strings.ToLower(err.Error())

	// Permanent HTTP errors (4xx except 429)
	permanentPatterns := []string{
		"status 400", "status 401", "status 403", "status 404",
		"status 405", "status 406", "status 410", "status 413",
		"status 422", "status 451",
		"signature verification failed",
		"invalid actor",
		"blocked domain",
		"spam detected",
		"account suspended",
		"invalid request format",
		"malformed json",
	}

	for _, pattern := range permanentPatterns {
		if strings.Contains(errStr, pattern) {
			return errorTypePermanent
		}
	}

	// Temporary errors (5xx, network issues, timeouts)
	temporaryPatterns := []string{
		"status 500", "status 502", "status 503", "status 504",
		"status 429", // Rate limiting
		"timeout", "connection refused", "connection reset",
		"no such host", "network unreachable", "temporary failure",
		"service unavailable", "internal server error",
		"bad gateway", "gateway timeout",
		"context deadline exceeded",
		"i/o timeout",
	}

	for _, pattern := range temporaryPatterns {
		if strings.Contains(errStr, pattern) {
			return errorTypeTemporary
		}
	}

	// Default to temporary for unknown errors to be safe
	return errorTypeTemporary
}

// calculateRetryBackoff calculates retry backoff with error-type specific adjustments
func (p *FederationDeliveryProcessor) calculateRetryBackoff(retryCount int, errorType, _ string) int {
	// Base exponential backoff
	baseBackoff := calculateBackoff(retryCount)

	// Adjust based on error type
	switch errorType {
	case "rate_limit":
		// Longer backoff for rate limiting
		return baseBackoff * 3
	case "server_error":
		// Moderate backoff for server errors
		return baseBackoff * 2
	case "network":
		// Standard backoff for network issues
		return baseBackoff
	case "timeout":
		// Slightly longer for timeout issues
		return int(float64(baseBackoff) * 1.5)
	default:
		// Default backoff for other temporary errors
		return baseBackoff
	}
}

// recordRetryMetrics records comprehensive metrics for retry attempts
func (p *FederationDeliveryProcessor) recordRetryMetrics(_ context.Context, deliveryID, errorType string, retryCount, backoffMinutes int) {
	p.logger.Info("federation_retry_metric",
		zap.String("metric_type", "retry_attempt"),
		zap.String("delivery_id", deliveryID),
		zap.String("error_type", errorType),
		zap.Int("retry_count", retryCount),
		zap.Int("backoff_minutes", backoffMinutes),
		zap.Time("timestamp", time.Now()))

	// Additional structured metrics for monitoring systems
	p.logger.Info("federation_delivery_status",
		zap.String("delivery_id", deliveryID),
		zap.String("status", "retrying"),
		zap.String("reason", fmt.Sprintf("%s_error", errorType)),
		zap.Int("attempt_number", retryCount+1),
		zap.Duration("next_retry_in", time.Duration(backoffMinutes)*time.Minute))
}

// recordSuccessMetrics records metrics for successful deliveries
func (p *FederationDeliveryProcessor) recordSuccessMetrics(_ context.Context, deliveryID, activityType, targetDomain string, totalAttempts int, totalDuration time.Duration) {
	p.logger.Info("federation_success_metric",
		zap.String("metric_type", "delivery_success"),
		zap.String("delivery_id", deliveryID),
		zap.String("activity_type", activityType),
		zap.String("target_domain", targetDomain),
		zap.Int("total_attempts", totalAttempts),
		zap.Duration("total_duration", totalDuration),
		zap.Bool("required_retry", totalAttempts > 1),
		zap.Time("timestamp", time.Now()))

	// Log domain-specific success rate data
	p.logger.Info("federation_domain_metric",
		zap.String("metric_type", "domain_delivery"),
		zap.String("target_domain", targetDomain),
		zap.String("result", "success"),
		zap.Int("attempts_needed", totalAttempts),
		zap.Duration("time_to_deliver", totalDuration))
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
