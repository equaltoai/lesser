// Package main implements the notification-processor Lambda function for processing user notifications.
package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	snstypes "github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/theory-cloud/apptheory/v2/pkg/streamer"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
	"github.com/theory-cloud/tabletheory/v2/pkg/core"
	"go.uber.org/zap"

	awsInit "github.com/equaltoai/lesser/pkg/aws"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	"github.com/equaltoai/lesser/pkg/lambdastorage"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/storage/theorydb"
)

type notificationRepository interface {
	GetUserNotification(ctx context.Context, userID, notificationID string) (*models.Notification, error)
	UpdateNotification(ctx context.Context, notification *models.Notification) error
}

type userRepository interface {
	GetUserPreferences(ctx context.Context, username string) (*storage.UserPreferences, error)
}

type trackingRepository interface {
	Create(ctx context.Context, tracking *models.DynamoDBCostRecord) error
}

type notificationCostRepository interface {
	CreateCostTracking(ctx context.Context, tracking *models.NotificationCostTracking) error
	GetBudget(ctx context.Context, username, period string) (*models.NotificationBudget, error)
	GetDailySpending(ctx context.Context, username string) (int64, error)
}

type webSocketSubscriptionRepository interface {
	GetUserConnections(ctx context.Context, userID string) ([]string, error)
}

type snsPublisher interface {
	Publish(ctx context.Context, params *sns.PublishInput, optFns ...func(*sns.Options)) (*sns.PublishOutput, error)
}

type sqsSender interface {
	SendMessage(ctx context.Context, params *sqs.SendMessageInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageOutput, error)
}

// NotificationProcessor handles notification delivery across multiple channels
type NotificationProcessor struct {
	db                        core.DB
	tableName                 string
	logger                    *zap.Logger
	notificationRepo          notificationRepository
	userRepo                  userRepository
	costTrackingRepo          trackingRepository
	notificationCostRepo      notificationCostRepository
	webSocketSubscriptionRepo webSocketSubscriptionRepository
	snsClient                 snsPublisher
	wsClient                  streamer.Client
	sqsClient                 sqsSender
	domain                    string
	webSocketEndpoint         string
	retryQueueURL             string
	deadLetterQueueURL        string
}

// NotificationDeliveryRequest represents a request to deliver a notification
type NotificationDeliveryRequest struct {
	NotificationID string     `json:"notification_id"`
	UserID         string     `json:"user_id"`
	Channels       []string   `json:"channels"`     // push, websocket
	Priority       string     `json:"priority"`     // high, medium, low
	RetryCount     int        `json:"retry_count"`  // current retry attempt
	ScheduledAt    *time.Time `json:"scheduled_at"` // for delayed delivery
}

// DeliveryResult represents the result of a delivery attempt
type DeliveryResult struct {
	Channel   string    `json:"channel"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	Cost      int64     `json:"cost_micros,omitempty"` // cost in micro-dollars
}

// WebSocketMessage represents a message sent over WebSocket
type WebSocketMessage struct {
	Type    string         `json:"type"`
	Event   string         `json:"event"`
	Payload map[string]any `json:"payload"`
}

// UserPreferences represents user notification preferences
type UserPreferences struct {
	PushNotifications      bool   `json:"push_notifications"`
	WebSocketNotifications bool   `json:"websocket_notifications"`
	PushEndpoint           string `json:"push_endpoint"`
}

// RetryableError represents an error that can be retried
type RetryableError struct {
	OriginalError error
	RetryAfter    time.Duration
	IsTemporary   bool
}

func (r *RetryableError) Error() string {
	if r.OriginalError != nil {
		return "retryable error: " + r.OriginalError.Error()
	}
	return "retryable error"
}

// RetryPolicy defines the retry configuration
type RetryPolicy struct {
	MaxRetries    int
	InitialDelay  time.Duration
	MaxDelay      time.Duration
	BackoffFactor float64
	JitterPercent float64
}

// DefaultRetryPolicy returns the default retry policy for notifications
func DefaultRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		MaxRetries:    5,
		InitialDelay:  time.Second,
		MaxDelay:      5 * time.Minute,
		BackoffFactor: 2.0,
		JitterPercent: 0.1, // 10% jitter
	}
}

// NewNotificationProcessor creates a new notification processor instance
func NewNotificationProcessor(lambdaCtx *common.LambdaContext) *NotificationProcessor {
	// Get logger and config
	logger := lambdaCtx.Logger
	cfg := lambdaCtx.Config

	var db core.DB
	if lambdaCtx.DynamoDB != nil {
		if storageDB, ok := lambdaCtx.DynamoDB.(core.DB); ok && storageDB != nil {
			db = storageDB
		}
	}
	if db == nil {
		var err error
		// Legacy unit-test fallback: production startup populates LambdaContext via
		// pkg/lambdastorage before constructing the processor.
		db, err = dynamormGetClientFn(context.Background())
		if err != nil {
			lambdaCtx.Logger.Fatal("failed to initialize DynamORM database", zap.Error(err))
		}
	}
	// Initialize repositories
	notificationRepo := repositories.NewNotificationRepository(db, cfg.DynamoTableName, logger, nil)
	userRepo := repositories.NewUserRepository(db, cfg.DynamoTableName, logger)
	costTrackingRepo := repositories.NewTrackingRepository(db, cfg.DynamoTableName, logger, nil)
	notificationCostRepo := repositories.NewNotificationCostRepository(db, cfg.DynamoTableName, logger, nil)
	webSocketSubscriptionRepo := repositories.NewWebSocketSubscriptionManagerRepository(db, cfg.DynamoTableName, logger, nil)

	// Get configuration from centralized config
	appCfg := config.Get()
	webSocketEndpoint := appCfg.WebSocketEndpoint
	retryQueueURL := appCfg.NotificationRetryQueueURL
	deadLetterQueueURL := appCfg.NotificationDLQURL

	var wsClient streamer.Client
	if webSocketEndpoint != "" && lambdaCtx.AWSServices != nil {
		client, err := streamerNewClientFn(context.Background(), webSocketEndpoint, streamer.WithAWSConfig(lambdaCtx.AWSServices.Config))
		if err != nil {
			logger.Warn("failed to initialize WebSocket client, websocket notifications disabled", zap.Error(err))
		} else {
			wsClient = client
		}
	}

	return &NotificationProcessor{
		db:                        db,
		tableName:                 cfg.DynamoTableName,
		logger:                    logger,
		notificationRepo:          notificationRepo,
		userRepo:                  userRepo,
		costTrackingRepo:          costTrackingRepo,
		notificationCostRepo:      notificationCostRepo,
		webSocketSubscriptionRepo: webSocketSubscriptionRepo,
		snsClient: func() *sns.Client {
			if lambdaCtx.AWSServices != nil {
				return lambdaCtx.AWSServices.SNS
			}
			return nil
		}(),
		sqsClient: func() *sqs.Client {
			if lambdaCtx.AWSServices != nil {
				return lambdaCtx.AWSServices.SQS
			}
			return nil
		}(),
		wsClient:           wsClient,
		domain:             cfg.Domain,
		webSocketEndpoint:  webSocketEndpoint,
		retryQueueURL:      retryQueueURL,
		deadLetterQueueURL: deadLetterQueueURL,
	}
}

// initializeAWSClients is no longer needed as AWS clients are pre-initialized by Lambda framework

func (np *NotificationProcessor) HandleSQSMessage(ctx *apptheory.EventContext, msg events.SQSMessage) (err error) {
	requestID := ""
	runCtx := context.Background()
	if ctx != nil {
		requestID = ctx.RequestID
		runCtx = ctx.Context()
	}

	if np.logger == nil {
		np.logger = zap.NewNop()
	}

	defer func() {
		if r := recover(); r != nil {
			np.logger.Error("panic processing notification message",
				zap.String("request_id", requestID),
				zap.String("message_id", msg.MessageId),
				zap.Any("panic", r),
			)
			err = fmt.Errorf("panic recovered: %v", r)
		}
	}()

	np.logger.Info("processing notification delivery message",
		zap.String("request_id", requestID),
		zap.String("message_id", msg.MessageId),
	)

	if err := np.processMessage(runCtx, msg); err != nil {
		np.logger.Error("failed to process notification message",
			zap.String("request_id", requestID),
			zap.String("message_id", msg.MessageId),
			zap.Error(err),
		)
		return err
	}
	return nil
}

func (np *NotificationProcessor) processMessage(ctx context.Context, record events.SQSMessage) error {
	// Parse the delivery request
	var request NotificationDeliveryRequest
	if err := json.Unmarshal([]byte(record.Body), &request); err != nil {
		np.logger.Error("failed to unmarshal delivery request",
			zap.String("message_id", record.MessageId),
			zap.Error(err))
		return ErrUnmarshalDeliveryRequest(err)
	}

	np.logger.Info("processing notification delivery",
		zap.String("notification_id", request.NotificationID),
		zap.String("user_id", request.UserID),
		zap.Strings("channels", request.Channels),
		zap.Int("retry_count", request.RetryCount),
	)

	// Check if this is a scheduled delivery
	if request.ScheduledAt != nil && time.Now().Before(*request.ScheduledAt) {
		np.logger.Info("notification scheduled for future delivery, requeuing",
			zap.String("notification_id", request.NotificationID),
			zap.Time("scheduled_at", *request.ScheduledAt),
		)
		// Requeue with delay until scheduled time
		return np.requeueScheduledNotification(ctx, request)
	}

	// Get the recipient-owned notification. Queue messages carry the recipient
	// user ID so push delivery updates the authoritative USER#{userID} row.
	notification, err := np.notificationRepo.GetUserNotification(ctx, request.UserID, request.NotificationID)
	if err != nil {
		np.logger.Error("failed to get notification",
			zap.String("notification_id", request.NotificationID),
			zap.Error(err))
		return ErrGetNotification(err)
	}

	// Get user preferences
	userPrefs, err := np.getUserPreferences(ctx, request.UserID)
	if err != nil {
		np.logger.Warn("failed to get user preferences, using defaults",
			zap.String("user_id", request.UserID),
			zap.Error(err),
		)
		userPrefs = &UserPreferences{
			PushNotifications:      true,
			WebSocketNotifications: true,
		}
	}

	// Estimate total cost for budget checking
	var estimatedCostMicroCents int64
	for _, channel := range request.Channels {
		switch channel {
		case "push":
			estimatedCostMicroCents += models.CalculatePushCost(1)
		case "websocket":
			estimatedCostMicroCents += models.CalculateWebSocketCost(1)
		default:
			// Unsupported channels don't contribute to cost estimates
		}
	}
	estimatedCostMicroCents += 20 // Add Lambda base cost

	// Check notification budget before proceeding
	canSend, err := np.checkNotificationBudget(ctx, request.UserID, estimatedCostMicroCents)
	if err != nil {
		np.logger.Error("failed to check notification budget",
			zap.String("user_id", request.UserID),
			zap.Error(err))
		// Continue anyway - budget errors shouldn't block notifications
	} else if !canSend {
		np.logger.Warn("notification blocked due to budget limits",
			zap.String("notification_id", request.NotificationID),
			zap.String("user_id", request.UserID),
			zap.Int64("estimated_cost_micro_cents", estimatedCostMicroCents))
		return ErrNotificationBudgetExceeded()
	}

	// Attempt delivery on each requested channel
	deliveryResults := make([]DeliveryResult, 0, len(request.Channels))
	var lastError error

	for _, channel := range request.Channels {
		result := np.deliverToChannel(ctx, notification, userPrefs, channel)
		deliveryResults = append(deliveryResults, result)

		if !result.Success {
			np.logger.Error("delivery failed on channel",
				zap.String("channel", channel),
				zap.String("error", result.Error))
			lastError = ErrDeliveryChannelFailed()
		}
	}

	// Update notification delivery status
	if err := np.updateDeliveryStatus(ctx, notification, deliveryResults); err != nil {
		np.logger.Error("failed to update delivery status",
			zap.String("notification_id", request.NotificationID),
			zap.Error(err),
		)
	}

	// Handle retry logic for failed deliveries
	if lastError != nil {
		retryPolicy := DefaultRetryPolicy()
		if request.RetryCount < retryPolicy.MaxRetries {
			// Check if error is retryable
			if np.isRetryableError(lastError) {
				np.logger.Info("scheduling retry for failed delivery",
					zap.String("notification_id", request.NotificationID),
					zap.Int("retry_count", request.RetryCount),
					zap.Int("next_retry_count", request.RetryCount+1),
					zap.Error(lastError),
				)
				// Queue retry with exponential backoff
				return np.scheduleRetry(ctx, request, lastError)
			}
			np.logger.Error("permanent error, sending to dead letter queue",
				zap.String("notification_id", request.NotificationID),
				zap.Error(lastError),
			)
			return np.sendToDeadLetterQueue(ctx, request, lastError)
		}
		np.logger.Error("maximum retries exceeded, sending to dead letter queue",
			zap.String("notification_id", request.NotificationID),
			zap.Int("retry_count", request.RetryCount),
			zap.Int("max_retries", retryPolicy.MaxRetries),
			zap.Error(lastError),
		)
		return np.sendToDeadLetterQueue(ctx, request, lastError)
	}

	// If no errors occurred, update the notification as successfully delivered
	if lastError == nil {
		notification.Data["delivery_status"] = "delivered"
		notification.Data["delivered_at"] = time.Now().Format(time.RFC3339)
		if err := np.notificationRepo.UpdateNotification(ctx, notification); err != nil {
			np.logger.Warn("failed to update successful delivery status",
				zap.String("notification_id", request.NotificationID),
				zap.Error(err))
		}
	}

	return lastError
}

func (np *NotificationProcessor) deliverToChannel(ctx context.Context, notification *models.Notification, userPrefs *UserPreferences, channel string) DeliveryResult {
	start := time.Now()
	result := DeliveryResult{
		Channel:   channel,
		Timestamp: start,
	}

	// Get retry count from the notification data if available
	retryCount := 0
	if notification.Data != nil {
		if rc, ok := notification.Data["retry_count"].(int); ok {
			retryCount = rc
		} else if rc, ok := notification.Data["retry_count"].(float64); ok {
			retryCount = int(rc)
		}
	}

	// Initialize cost tracking builder with retry information
	costBuilder := models.NewNotificationCostTrackingBuilder().
		WithNotification(notification.ID, notification.UserID, notification.UserID, notification.Type).
		WithDelivery(channel, channel, false, retryCount).
		WithContext("", "notification-processor", os.Getenv("AWS_LAMBDA_FUNCTION_NAME"), os.Getenv("AWS_LAMBDA_LOG_STREAM_NAME")).
		WithTimestamp(start)

	var deliveryStart time.Time
	var lambdaCostMicroCents int64 = 20 // Base Lambda invocation cost

	switch channel {
	case "push":
		if !userPrefs.PushNotifications {
			result.Error = "push notifications disabled"
			costBuilder.WithError(result.Error)
		} else {
			deliveryStart = time.Now()
			if err := np.deliverPush(ctx, notification, userPrefs); err != nil {
				result.Error = err.Error()
				costBuilder.WithError(result.Error)
			} else {
				result.Success = true
				costBuilder.WithDelivery(channel, channel, true, retryCount)

				// Calculate push cost
				pushCost := models.CalculatePushCost(1)
				result.Cost = pushCost
				costBuilder.WithCosts(pushCost, 0, lambdaCostMicroCents, 0)
			}
		}

	case "websocket":
		if !userPrefs.WebSocketNotifications {
			result.Error = "websocket notifications disabled"
			costBuilder.WithError(result.Error)
		} else {
			deliveryStart = time.Now()
			if err := np.deliverWebSocket(ctx, notification); err != nil {
				result.Error = err.Error()
				costBuilder.WithError(result.Error)
			} else {
				result.Success = true
				costBuilder.WithDelivery(channel, channel, true, retryCount)

				// Calculate websocket cost
				websocketCost := models.CalculateWebSocketCost(1)
				result.Cost = websocketCost
				costBuilder.WithCosts(0, websocketCost, lambdaCostMicroCents, 0)
			}
		}

	default:
		np.logger.Error("unsupported delivery channel",
			zap.String("channel", channel))
		result.Error = "unsupported delivery channel: only push and websocket are supported"
		costBuilder.WithError(result.Error)
	}

	// Calculate performance metrics
	totalDuration := time.Since(start)
	var deliveryDuration time.Duration
	if !deliveryStart.IsZero() {
		deliveryDuration = time.Since(deliveryStart)
	}

	processingDuration := totalDuration - deliveryDuration

	costBuilder.WithPerformance(
		processingDuration.Milliseconds(),
		deliveryDuration.Milliseconds(),
		0, // Response code would be set by specific delivery methods
		0, // Response size would be set by specific delivery methods
	)

	// Add additional context including retry information
	costBuilder.WithProperty("notification_type", notification.Type)
	costBuilder.WithProperty("delivery_channel", channel)
	costBuilder.WithProperty("user_preferences_enabled", userPrefs != nil)
	costBuilder.WithProperty("retry_count", retryCount)
	costBuilder.WithProperty("is_retry", retryCount > 0)
	costBuilder.WithTag("domain", np.domain)
	costBuilder.WithTag("delivery_method", channel)
	if retryCount > 0 {
		costBuilder.WithTag("retry_attempt", fmt.Sprintf("retry_%d", retryCount))
	}

	// Create cost tracking record
	costTracking := costBuilder.Build()

	// Store cost tracking record asynchronously to avoid impacting delivery performance
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := np.storeCostTracking(ctx, costTracking); err != nil {
			np.logger.Warn("failed to store notification cost tracking",
				zap.String("notification_id", notification.ID),
				zap.String("channel", channel),
				zap.Error(err))
		}
	}()

	np.logger.Info("delivery attempt completed",
		zap.String("notification_id", notification.ID),
		zap.String("channel", channel),
		zap.Bool("success", result.Success),
		zap.String("error", result.Error),
		zap.Int("retry_count", retryCount),
		zap.Bool("is_retry", retryCount > 0),
		zap.Duration("total_duration", totalDuration),
		zap.Duration("delivery_duration", deliveryDuration),
		zap.Int64("cost_micro_cents", result.Cost),
		zap.Float64("cost_dollars", float64(result.Cost)/1_000_000.0),
	)

	return result
}

func (np *NotificationProcessor) deliverPush(ctx context.Context, notification *models.Notification, _ *UserPreferences) error {
	if np.snsClient == nil {
		return ErrSNSClientNotInitialized()
	}

	// Build push notification payload
	payload := map[string]any{
		"title": notification.Title,
		"body":  notification.Body,
		"data": map[string]any{
			"notification_id": notification.ID,
			"type":            notification.Type,
			"actor_id":        notification.ActorID,
			"target_id":       notification.TargetID,
		},
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		np.logger.Error("failed to marshal push payload",
			zap.String("notification_id", notification.ID),
			zap.Error(err))
		return ErrMarshalPushPayload(err)
	}

	// Send push notification via SNS to FCM/APNS
	err = np.sendPushNotification(ctx, notification.UserID, payloadJSON)
	if err != nil {
		np.logger.Error("failed to send push notification",
			zap.String("notification_id", notification.ID),
			zap.String("user_id", notification.UserID),
			zap.Error(err))
		return ErrSendPushNotification(err)
	}

	np.logger.Info("push notification delivered successfully",
		zap.String("notification_id", notification.ID),
		zap.String("user_id", notification.UserID))

	// Mark push as sent on the already-loaded authoritative row.
	notification.MarkPushSent()
	if err := np.notificationRepo.UpdateNotification(ctx, notification); err != nil {
		np.logger.Warn("failed to mark push as sent", zap.Error(err))
	}

	return nil
}

func (np *NotificationProcessor) deliverWebSocket(ctx context.Context, notification *models.Notification) error {
	// Build WebSocket message
	message := WebSocketMessage{
		Type:  "notification",
		Event: "notification.new",
		Payload: map[string]any{
			"id":         notification.ID,
			"type":       notification.Type,
			"title":      notification.Title,
			"body":       notification.Body,
			"actor_id":   notification.ActorID,
			"target_id":  notification.TargetID,
			"created_at": notification.CreatedAt.Format(time.RFC3339),
		},
	}

	// Get active WebSocket connections for the user
	connections, err := np.getActiveWebSocketConnections(ctx, notification.UserID)
	if err != nil {
		np.logger.Error("failed to get websocket connections",
			zap.String("user_id", notification.UserID),
			zap.Error(err))
		return ErrGetWebSocketConnections(err)
	}

	if err := common.ValidateSliceNotEmpty("connections", connections); err != nil {
		np.logger.Info("no active websocket connections for user",
			zap.String("user_id", notification.UserID),
		)
		return nil // Not an error - user just isn't connected
	}

	// Send to all active connections
	var lastError error
	successCount := 0

	for _, connectionID := range connections {
		if err := np.sendWebSocketMessage(ctx, connectionID, message); err != nil {
			np.logger.Warn("failed to send websocket message to connection",
				zap.String("connection_id", connectionID),
				zap.Error(err),
			)
			lastError = err
		} else {
			successCount++
		}
	}

	if successCount == 0 && lastError != nil {
		np.logger.Error("failed to deliver to any websocket connections",
			zap.String("notification_id", notification.ID),
			zap.String("user_id", notification.UserID),
			zap.Error(lastError))
		return ErrDeliverWebSocketMessage(lastError)
	}

	np.logger.Info("websocket notification delivered",
		zap.String("notification_id", notification.ID),
		zap.String("user_id", notification.UserID),
		zap.Int("connections", successCount),
		zap.Int("total_connections", len(connections)),
	)

	return nil
}

func (np *NotificationProcessor) sendWebSocketMessage(ctx context.Context, connectionID string, message WebSocketMessage) error {
	if np.wsClient == nil {
		return ErrAPIGatewayClientNotInitialized()
	}

	messageBytes, err := json.Marshal(message)
	if err != nil {
		np.logger.Error("failed to marshal websocket message",
			zap.String("connection_id", connectionID),
			zap.Error(err))
		return ErrMarshalWebSocketMessage(err)
	}

	return np.wsClient.PostToConnection(ctx, connectionID, messageBytes)
}

// Removed unused getActorName function
// If needed in the future, this function looked up actor details from storage

func (np *NotificationProcessor) getUserPreferences(ctx context.Context, userID string) (*UserPreferences, error) {
	// Get user preferences from storage
	prefs, err := np.userRepo.GetUserPreferences(ctx, userID)
	if err != nil {
		np.logger.Warn("failed to get user preferences, using defaults",
			zap.String("user_id", userID),
			zap.Error(err))
		// Return default preferences if not found
		return &UserPreferences{
			PushNotifications:      true,
			WebSocketNotifications: true,
			PushEndpoint:           "",
		}, nil
	}

	// Convert storage preferences to notification preferences
	// Check if prefs has preferences map
	if prefs != nil && prefs.Preferences != nil {
		return &UserPreferences{
			PushNotifications: func() bool {
				result, _ := common.ParseAndValidateBoolean(prefs.Preferences["push_enabled"])
				return result
			}(),
			WebSocketNotifications: prefs.Preferences["websocket_enabled"] != "false", // Default true
			PushEndpoint:           prefs.Preferences["push_endpoint"],
		}, nil
	}

	// Default preferences
	return &UserPreferences{
		PushNotifications:      true,
		WebSocketNotifications: true,
		PushEndpoint:           "",
	}, nil
}

func (np *NotificationProcessor) getActiveWebSocketConnections(ctx context.Context, userID string) ([]string, error) {
	// Query active WebSocket connections from the WebSocket subscription repository
	connectionIDs, err := np.webSocketSubscriptionRepo.GetUserConnections(ctx, userID)
	if err != nil {
		np.logger.Warn("failed to get active websocket connections",
			zap.String("user_id", userID),
			zap.Error(err))
		// Return empty slice on error - websocket delivery is not critical
		return []string{}, nil
	}

	np.logger.Debug("found active websocket connections",
		zap.String("user_id", userID),
		zap.Int("connection_count", len(connectionIDs)))

	return connectionIDs, nil
}

// sendPushNotification sends a push notification via SNS to FCM/APNS
func (np *NotificationProcessor) sendPushNotification(ctx context.Context, userID string, payload []byte) error {
	// Get user's push notification endpoints from preferences
	// In a full implementation, users would have registered FCM tokens or APNS device tokens

	// Create SNS message for push notification
	message := string(payload)

	// Get push notification topic from configuration
	appCfg := config.Get()
	pushTopicArn := appCfg.PushNotificationTopicArn
	if err := common.ValidateRequiredParam("pushTopicArn", pushTopicArn); err != nil {
		return ErrPushTopicNotConfigured()
	}

	// Publish to SNS topic for push notifications
	// This would route to FCM for Android or APNS for iOS based on user's device registrations
	_, err := np.snsClient.Publish(ctx, &sns.PublishInput{
		TopicArn: aws.String(pushTopicArn),
		Message:  aws.String(message),
		MessageAttributes: map[string]snstypes.MessageAttributeValue{
			"userID": {
				DataType:    aws.String("String"),
				StringValue: aws.String(userID),
			},
			"platform": {
				DataType:    aws.String("String"),
				StringValue: aws.String("mobile"), // Would be determined by user's registered devices
			},
		},
	})

	if err != nil {
		np.logger.Error("failed to publish to SNS",
			zap.String("user_id", userID),
			zap.String("topic_arn", pushTopicArn),
			zap.Error(err))
		return ErrPublishToSNS(err)
	}

	return nil
}

func (np *NotificationProcessor) updateDeliveryStatus(ctx context.Context, notification *models.Notification, results []DeliveryResult) error {
	// Update notification with delivery results
	if notification.Data == nil {
		notification.Data = make(map[string]interface{})
	}

	notification.Data["delivery_results"] = results
	notification.Data["last_delivery_attempt"] = time.Now().Format(time.RFC3339)

	// Check if any delivery was successful
	anySuccess := false
	for _, result := range results {
		if result.Success {
			anySuccess = true
			break
		}
	}

	if anySuccess {
		notification.Data["delivery_status"] = "delivered"
	} else {
		notification.Data["delivery_status"] = "failed"
	}

	return np.notificationRepo.UpdateNotification(ctx, notification)
}

// Removed unused extractUsernameFromActorID function
// This function extracted usernames from ActivityPub actor IDs

var (
	lambdaCtx *common.LambdaContext
	processor *NotificationProcessor
)

func init() {
	if common.RunningUnitTests() {
		return
	}

	if err := initializeNotificationProcessor(); err != nil {
		lambdaCtx.Logger.Fatal("failed to initialize notification processor", zap.Error(err))
	}
}

var (
	mustInitializeLambdaFn     = common.MustInitializeLambda
	newNotificationProcessorFn = NewNotificationProcessor
	dynamormGetClientFn        = theorydb.GetClient
	streamerNewClientFn        = streamer.NewClient
	randReadFn                 = rand.Read
	lambdaStartFn              = lambda.Start
)

func initializeNotificationProcessor() error {
	// Standardized Lambda initialization for processor functions
	lambdaCtx = mustInitializeLambdaFn(common.LambdaConfig{
		ServiceName: "notification-processor",
		LambdaType:  common.LambdaTypeProcessor,
		CustomServiceConfig: &awsInit.ServiceConfig{
			RequiresDynamoDB:   true,
			RequiresCloudWatch: true,
			RequiresSNS:        true,
			RequiresSQS:        true,
			ServiceName:        "notification-processor",
		},
	})

	if lambdaCtx.Config != nil {
		if _, err := lambdastorage.Initialize(context.Background(), lambdaCtx, lambdastorage.Options{
			ServiceName:         "notification-processor",
			RequireRepositories: true,
			AllowEmptyRegion:    true,
			NewDB: func(ctx context.Context, _ string) (core.DB, error) {
				return dynamormGetClientFn(ctx)
			},
		}); err != nil {
			return fmt.Errorf("failed to initialize storage: %w", err)
		}
	} else if !common.RunningUnitTests() {
		return fmt.Errorf("failed to initialize storage: config is nil")
	}

	processor = newNotificationProcessorFn(lambdaCtx)
	return nil
}

func main() {
	app := apptheory.New()

	appName := strings.TrimSpace(os.Getenv("APP_NAME"))
	stage := strings.TrimSpace(os.Getenv("STAGE"))
	queueName := naming.ResourceNameWithApp(appName, "notification-processor-queue", stage)

	app.SQS(queueName, func(ctx *apptheory.EventContext, msg events.SQSMessage) error {
		if processor == nil {
			return fmt.Errorf("notification processor not initialized")
		}
		return processor.HandleSQSMessage(ctx, msg)
	})

	lambdaStartFn(func(ctx context.Context, event json.RawMessage) (any, error) {
		return app.HandleLambda(ctx, event)
	})
}

// storeCostTracking stores the notification cost tracking record
func (np *NotificationProcessor) storeCostTracking(ctx context.Context, costTracking *models.NotificationCostTracking) error {
	// Store the detailed notification cost tracking record
	if err := np.notificationCostRepo.CreateCostTracking(ctx, costTracking); err != nil {
		np.logger.Error("failed to create notification cost tracking record",
			zap.String("notification_id", costTracking.NotificationID),
			zap.Error(err))
		// Continue anyway - this shouldn't block notification delivery
	}

	// Create a cost record for the main cost tracking system for aggregation
	costRecord := &models.DynamoDBCostRecord{
		Table:                np.tableName,
		OperationType:        "NotificationDelivery",
		ServiceName:          "notification-processor",
		Timestamp:            costTracking.Timestamp,
		TotalCostMicroCents:  costTracking.TotalCostMicroCents,
		EstimatedCostDollars: float64(costTracking.TotalCostMicroCents) / 1_000_000.0,
		Properties: map[string]interface{}{
			"notification_id":   costTracking.NotificationID,
			"user_id":           costTracking.UserID,
			"username":          costTracking.Username,
			"delivery_method":   costTracking.DeliveryMethod,
			"notification_type": costTracking.NotificationType,
			"success":           costTracking.Success,
			"retry_count":       costTracking.RetryCount,
		},
		Tags: costTracking.Tags,
	}

	// Store the general cost record
	if err := np.costTrackingRepo.Create(ctx, costRecord); err != nil {
		np.logger.Error("failed to create general cost record",
			zap.String("notification_id", costTracking.NotificationID),
			zap.Error(err))
		// Continue anyway - this shouldn't block notification delivery
	}

	return nil
}

// checkNotificationBudget checks if the user has exceeded their notification budget
func (np *NotificationProcessor) checkNotificationBudget(ctx context.Context, username string, estimatedCostMicroCents int64) (bool, error) {
	// Get the user's daily budget
	budget, err := np.notificationCostRepo.GetBudget(ctx, username, "daily")
	if err != nil {
		np.logger.Error("failed to get user budget",
			zap.String("username", username),
			zap.Error(err))
		// On error, allow the notification (fail open) but log the issue
		return true, nil
	}

	// If no budget is set, use default limits and enforce them
	if budget == nil {
		// Default budget: $0.01 per user per day (1000 micro-cents)
		dailyBudgetMicroCents := int64(1000)

		// Get current daily spending to check against default budget
		currentSpending, err := np.notificationCostRepo.GetDailySpending(ctx, username)
		if err != nil {
			np.logger.Warn("failed to get current daily spending, allowing notification",
				zap.String("username", username),
				zap.Error(err))
			return true, nil
		}

		projectedSpending := currentSpending + estimatedCostMicroCents

		np.logger.Debug("checking against default budget",
			zap.String("username", username),
			zap.Int64("estimated_cost_micro_cents", estimatedCostMicroCents),
			zap.Int64("current_spending_micro_cents", currentSpending),
			zap.Int64("projected_spending_micro_cents", projectedSpending),
			zap.Int64("daily_budget_micro_cents", dailyBudgetMicroCents))

		// Enforce default budget
		if projectedSpending > dailyBudgetMicroCents {
			np.logger.Warn("notification would exceed default daily budget",
				zap.String("username", username),
				zap.Int64("projected_spending_micro_cents", projectedSpending),
				zap.Int64("daily_budget_micro_cents", dailyBudgetMicroCents))
			return false, nil
		}

		return true, nil
	}

	// Check if budget enforcement is enabled
	if !budget.Enabled {
		np.logger.Debug("budget disabled for user",
			zap.String("username", username))
		return true, nil
	}

	// Check if adding this cost would exceed the budget
	projectedSpending := budget.SpentMicroCents + estimatedCostMicroCents

	np.logger.Debug("checking notification budget",
		zap.String("username", username),
		zap.Int64("estimated_cost_micro_cents", estimatedCostMicroCents),
		zap.Int64("current_spent_micro_cents", budget.SpentMicroCents),
		zap.Int64("projected_spending_micro_cents", projectedSpending),
		zap.Int64("budget_limit_micro_cents", budget.LimitMicroCents),
		zap.Bool("budget_exceeded", budget.BudgetExceeded))

	// Check if delivery should be blocked
	if budget.ShouldBlockDelivery() {
		np.logger.Warn("notification blocked due to budget limits",
			zap.String("username", username),
			zap.Int64("spent_micro_cents", budget.SpentMicroCents),
			zap.Int64("limit_micro_cents", budget.LimitMicroCents),
			zap.Bool("budget_exceeded", budget.BudgetExceeded))
		return false, nil
	}

	// Check if this would exceed the budget
	if projectedSpending > budget.LimitMicroCents {
		np.logger.Warn("notification would exceed budget",
			zap.String("username", username),
			zap.Int64("projected_spending_micro_cents", projectedSpending),
			zap.Int64("limit_micro_cents", budget.LimitMicroCents))
		return false, nil
	}

	return true, nil
}

// requeueScheduledNotification requeues a notification for future delivery
func (np *NotificationProcessor) requeueScheduledNotification(ctx context.Context, request NotificationDeliveryRequest) error {
	if np.sqsClient == nil {
		return ErrSQSClientNotInitialized()
	}
	if err := common.ValidateRequiredParam("retryQueueURL", np.retryQueueURL); err != nil {
		return ErrRetryQueueNotConfigured()
	}

	// Calculate delay until scheduled time
	delay := time.Until(*request.ScheduledAt)
	if delay < 0 {
		// If scheduled time has passed, deliver immediately
		delay = 0
	}

	// Requeue with delay (SQS supports up to 15 minutes delay)
	delaySeconds := int32(delay.Seconds())
	if delaySeconds > 900 { // 15 minutes max
		delaySeconds = 900
	}

	// Serialize request
	messageBody, err := json.Marshal(request)
	if err != nil {
		np.logger.Error("failed to marshal scheduled request",
			zap.String("notification_id", request.NotificationID),
			zap.Error(err))
		return ErrMarshalScheduledRequest(err)
	}

	// Send to retry queue with delay
	_, err = np.sqsClient.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:     aws.String(np.retryQueueURL),
		MessageBody:  aws.String(string(messageBody)),
		DelaySeconds: delaySeconds,
		MessageAttributes: map[string]types.MessageAttributeValue{
			"notification_id": {
				DataType:    aws.String("String"),
				StringValue: aws.String(request.NotificationID),
			},
			"retry_type": {
				DataType:    aws.String("String"),
				StringValue: aws.String("scheduled"),
			},
		},
	})

	if err != nil {
		np.logger.Error("failed to requeue notification",
			zap.String("notification_id", request.NotificationID),
			zap.String("queue_url", np.retryQueueURL),
			zap.Error(err))
		return ErrRequeueNotification(err)
	}

	np.logger.Info("requeued scheduled notification",
		zap.String("notification_id", request.NotificationID),
		zap.Time("scheduled_at", *request.ScheduledAt),
		zap.Int32("delay_seconds", delaySeconds))

	return nil
}

// scheduleRetry schedules a retry for a failed notification with exponential backoff
func (np *NotificationProcessor) scheduleRetry(ctx context.Context, request NotificationDeliveryRequest, originalError error) error {
	if np.sqsClient == nil || common.ValidateRequiredParam("retryQueueURL", np.retryQueueURL) != nil {
		return ErrSQSConfigurationIncomplete()
	}

	retryPolicy := DefaultRetryPolicy()

	// Calculate exponential backoff delay
	delay := np.calculateRetryDelay(request.RetryCount, retryPolicy)

	// Create retry request
	retryRequest := request
	retryRequest.RetryCount++

	// Serialize request
	messageBody, err := json.Marshal(retryRequest)
	if err != nil {
		np.logger.Error("failed to marshal retry request",
			zap.String("notification_id", request.NotificationID),
			zap.Int("retry_count", retryRequest.RetryCount),
			zap.Error(err))
		return ErrMarshalRetryRequest(err)
	}

	// Calculate SQS delay (max 15 minutes)
	delaySeconds := int32(delay.Seconds())
	if delaySeconds > 900 {
		delaySeconds = 900
	}

	// Send to retry queue with delay
	_, err = np.sqsClient.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:     aws.String(np.retryQueueURL),
		MessageBody:  aws.String(string(messageBody)),
		DelaySeconds: delaySeconds,
		MessageAttributes: map[string]types.MessageAttributeValue{
			"notification_id": {
				DataType:    aws.String("String"),
				StringValue: aws.String(request.NotificationID),
			},
			"retry_type": {
				DataType:    aws.String("String"),
				StringValue: aws.String("failed_delivery"),
			},
			"retry_count": {
				DataType:    aws.String("Number"),
				StringValue: aws.String(fmt.Sprintf("%d", retryRequest.RetryCount)),
			},
			"original_error": {
				DataType:    aws.String("String"),
				StringValue: aws.String(originalError.Error()),
			},
		},
	})

	if err != nil {
		np.logger.Error("failed to schedule retry",
			zap.String("notification_id", request.NotificationID),
			zap.Int("retry_count", retryRequest.RetryCount),
			zap.String("queue_url", np.retryQueueURL),
			zap.Error(err))
		return ErrScheduleRetry(err)
	}

	np.logger.Info("scheduled notification retry",
		zap.String("notification_id", request.NotificationID),
		zap.Int("retry_count", retryRequest.RetryCount),
		zap.Duration("delay", delay),
		zap.Int32("delay_seconds", delaySeconds),
		zap.Error(originalError))

	return nil
}

// sendToDeadLetterQueue sends a failed notification to the dead letter queue
func (np *NotificationProcessor) sendToDeadLetterQueue(ctx context.Context, request NotificationDeliveryRequest, finalError error) error {
	if np.sqsClient == nil || common.ValidateRequiredParam("deadLetterQueueURL", np.deadLetterQueueURL) != nil {
		np.logger.Error("SQS client or DLQ URL not configured, cannot send to DLQ",
			zap.String("notification_id", request.NotificationID))
		// Return the original error since we couldn't DLQ it
		return finalError
	}

	// Create DLQ message with failure details
	dlqMessage := map[string]interface{}{
		"original_request": request,
		"final_error":      finalError.Error(),
		"failed_at":        time.Now().Format(time.RFC3339),
		"retry_count":      request.RetryCount,
	}

	// Serialize DLQ message
	messageBody, err := json.Marshal(dlqMessage)
	if err != nil {
		np.logger.Error("failed to marshal DLQ message",
			zap.String("notification_id", request.NotificationID),
			zap.Error(err))
		return finalError
	}

	// Send to dead letter queue
	_, err = np.sqsClient.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    aws.String(np.deadLetterQueueURL),
		MessageBody: aws.String(string(messageBody)),
		MessageAttributes: map[string]types.MessageAttributeValue{
			"notification_id": {
				DataType:    aws.String("String"),
				StringValue: aws.String(request.NotificationID),
			},
			"user_id": {
				DataType:    aws.String("String"),
				StringValue: aws.String(request.UserID),
			},
			"failure_type": {
				DataType:    aws.String("String"),
				StringValue: aws.String("max_retries_exceeded"),
			},
			"retry_count": {
				DataType:    aws.String("Number"),
				StringValue: aws.String(fmt.Sprintf("%d", request.RetryCount)),
			},
		},
	})

	if err != nil {
		np.logger.Error("failed to send message to dead letter queue",
			zap.String("notification_id", request.NotificationID),
			zap.Error(err))
		return finalError
	}

	np.logger.Error("notification sent to dead letter queue",
		zap.String("notification_id", request.NotificationID),
		zap.String("user_id", request.UserID),
		zap.Int("retry_count", request.RetryCount),
		zap.Error(finalError))

	// Return nil since we successfully handled the failure by sending to DLQ
	return nil
}

// calculateRetryDelay calculates the delay for a retry attempt with jitter
func (np *NotificationProcessor) calculateRetryDelay(retryCount int, policy *RetryPolicy) time.Duration {
	// Calculate exponential backoff
	delay := time.Duration(float64(policy.InitialDelay) * math.Pow(policy.BackoffFactor, float64(retryCount)))

	// Cap at max delay
	if delay > policy.MaxDelay {
		delay = policy.MaxDelay
	}

	// Add jitter to avoid thundering herd
	if policy.JitterPercent > 0 {
		jitterRange := time.Duration(float64(delay) * policy.JitterPercent)

		// Generate random jitter
		jitterBytes := make([]byte, 8)
		if _, err := randReadFn(jitterBytes); err != nil {
			// If we can't generate random jitter, proceed without it
			np.logger.Warn("failed to generate random jitter", zap.Error(err))
			return delay
		}

		// Convert to int64 for calculation
		jitterValue := int64(0)
		for i, b := range jitterBytes {
			jitterValue |= int64(b) << (i * 8)
		}

		// Apply jitter (can be positive or negative)
		jitter := time.Duration(jitterValue%int64(jitterRange*2)) - jitterRange
		delay += jitter

		// Ensure delay is not negative
		if delay < 0 {
			delay = policy.InitialDelay
		}
	}

	return delay
}

// isRetryableError determines if an error is retryable
func (np *NotificationProcessor) isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errorStr := strings.ToLower(err.Error())

	// Permanent errors that should not be retried
	permanentErrors := []string{
		"invalid notification",
		"user not found",
		"notification not found",
		"invalid request",
		"unauthorized",
		"forbidden",
		"malformed",
		"budget exceeded", // Budget errors are permanent for that period
	}

	for _, permErr := range permanentErrors {
		if strings.Contains(errorStr, permErr) {
			return false
		}
	}

	// Temporary errors that can be retried
	temporaryErrors := []string{
		"timeout",
		"connection refused",
		"connection reset",
		"network",
		"unavailable",
		"throttled",
		"rate limit",
		"internal server error",
		"service unavailable",
		"bad gateway",
		"gateway timeout",
		"temporary",
	}

	for _, tempErr := range temporaryErrors {
		if strings.Contains(errorStr, tempErr) {
			return true
		}
	}

	// Default to retryable for unknown errors (fail open for retries)
	// This helps ensure transient issues don't permanently lose notifications
	return true
}
