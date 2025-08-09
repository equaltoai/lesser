// Package main implements the notification-processor Lambda function for processing user notifications.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/apigatewaymanagementapi"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	snstypes "github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
)

// NotificationProcessor handles notification delivery across multiple channels
type NotificationProcessor struct {
	db                   core.DB
	tableName            string
	logger               *zap.Logger
	notificationRepo     *repositories.NotificationRepository
	userRepo             *repositories.UserRepository
	costTrackingRepo     *repositories.CostTrackingRepository
	notificationCostRepo *repositories.NotificationCostRepository
	snsClient            *sns.Client
	apiGatewayClient     *apigatewaymanagementapi.Client
	domain               string
	webSocketEndpoint    string
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

// NewNotificationProcessor creates a new notification processor instance
func NewNotificationProcessor(db core.DB, tableName string, domain string) *NotificationProcessor {
	// Initialize repositories
	logger := common.Logger()
	notificationRepo := repositories.NewNotificationRepository(db, tableName, logger)
	userRepo := repositories.NewUserRepository(db, tableName, logger)
	costTrackingRepo := repositories.NewCostTrackingRepository(db, tableName, logger)
	notificationCostRepo := repositories.NewNotificationCostRepository(db, tableName, logger)

	// Get configuration from environment
	webSocketEndpoint := os.Getenv("WEBSOCKET_ENDPOINT")

	return &NotificationProcessor{
		db:                   db,
		tableName:            tableName,
		logger:               common.Logger(),
		notificationRepo:     notificationRepo,
		userRepo:             userRepo,
		costTrackingRepo:     costTrackingRepo,
		notificationCostRepo: notificationCostRepo,
		domain:               domain,
		webSocketEndpoint:    webSocketEndpoint,
	}
}

func (np *NotificationProcessor) initializeAWSClients(ctx context.Context) error {
	// Load AWS configuration
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Initialize SNS client for push notifications
	np.snsClient = sns.NewFromConfig(cfg)

	// Initialize API Gateway Management API client for WebSocket
	if np.webSocketEndpoint != "" {
		np.apiGatewayClient = apigatewaymanagementapi.NewFromConfig(cfg, func(o *apigatewaymanagementapi.Options) {
			o.BaseEndpoint = aws.String(np.webSocketEndpoint)
		})
	}

	return nil
}

// HandleSQS implements the SQS handler interface for Lift
func (np *NotificationProcessor) HandleSQS(ctx *lift.Context, event events.SQSEvent) error {
	// Initialize AWS clients using the underlying context
	if err := np.initializeAWSClients(ctx.Request.Context()); err != nil {
		np.logger.Error("failed to initialize AWS clients", zap.Error(err))
		return lift.NewLiftError("AWS_INIT_FAILED", "failed to initialize AWS clients", 500).WithCause(err)
	}

	np.logger.Info("processing notification delivery batch",
		zap.String("request_id", ctx.GetRequestID()),
		zap.Int("message_count", len(event.Records)),
	)

	// Process messages in parallel with error collection (preserving existing pattern)
	var errors []error
	var errorMutex sync.Mutex

	var wg sync.WaitGroup
	sem := make(chan struct{}, 5) // Limit concurrency to 5

	for _, record := range event.Records {
		wg.Add(1)
		sem <- struct{}{}

		go func(record events.SQSMessage) {
			defer wg.Done()
			defer func() { <-sem }()

			if err := np.processMessage(ctx.Request.Context(), record); err != nil {
				errorMutex.Lock()
				errors = append(errors, err)
				errorMutex.Unlock()

				np.logger.Error("failed to process message",
					zap.String("message_id", record.MessageId),
					zap.Error(err),
				)
			}
		}(record)
	}

	wg.Wait()

	if len(errors) > 0 {
		return lift.NewLiftError("PARTIAL_BATCH_FAILURE",
			fmt.Sprintf("partial batch failure: %d of %d messages failed", len(errors), len(event.Records)),
			500)
	}

	return nil
}

func (np *NotificationProcessor) processMessage(ctx context.Context, record events.SQSMessage) error {
	// Parse the delivery request
	var request NotificationDeliveryRequest
	if err := json.Unmarshal([]byte(record.Body), &request); err != nil {
		return fmt.Errorf("failed to unmarshal delivery request: %w", err)
	}

	np.logger.Info("processing notification delivery",
		zap.String("notification_id", request.NotificationID),
		zap.String("user_id", request.UserID),
		zap.Strings("channels", request.Channels),
		zap.Int("retry_count", request.RetryCount),
	)

	// Check if this is a scheduled delivery
	if request.ScheduledAt != nil && time.Now().Before(*request.ScheduledAt) {
		np.logger.Info("notification scheduled for future delivery",
			zap.String("notification_id", request.NotificationID),
			zap.Time("scheduled_at", *request.ScheduledAt),
		)
		return nil // Skip for now, would requeue in real implementation
	}

	// Get the notification
	notification, err := np.notificationRepo.GetNotification(ctx, request.NotificationID)
	if err != nil {
		return fmt.Errorf("failed to get notification: %w", err)
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
		return fmt.Errorf("notification delivery blocked: user budget exceeded")
	}

	// Attempt delivery on each requested channel
	deliveryResults := make([]DeliveryResult, 0, len(request.Channels))
	var lastError error

	for _, channel := range request.Channels {
		result := np.deliverToChannel(ctx, notification, userPrefs, channel)
		deliveryResults = append(deliveryResults, result)

		if !result.Success {
			lastError = fmt.Errorf("delivery failed on channel %s: %s", channel, result.Error)
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
	if lastError != nil && request.RetryCount < 3 {
		np.logger.Info("scheduling retry for failed delivery",
			zap.String("notification_id", request.NotificationID),
			zap.Int("retry_count", request.RetryCount+1),
		)

		// In a real implementation, you'd requeue the message with exponential backoff
		// For now, we'll just log the retry attempt
		return fmt.Errorf("delivery failed, retry needed: %w", lastError)
	}

	return lastError
}

func (np *NotificationProcessor) deliverToChannel(ctx context.Context, notification *models.Notification, userPrefs *UserPreferences, channel string) DeliveryResult {
	start := time.Now()
	result := DeliveryResult{
		Channel:   channel,
		Timestamp: start,
	}

	// Initialize cost tracking builder
	costBuilder := models.NewNotificationCostTrackingBuilder().
		WithNotification(notification.ID, notification.UserID, notification.UserID, notification.Type).
		WithDelivery(channel, channel, false, 0).
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
				costBuilder.WithDelivery(channel, channel, true, 0)

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
				costBuilder.WithDelivery(channel, channel, true, 0)

				// Calculate websocket cost
				websocketCost := models.CalculateWebSocketCost(1)
				result.Cost = websocketCost
				costBuilder.WithCosts(0, websocketCost, lambdaCostMicroCents, 0)
			}
		}

	default:
		result.Error = fmt.Sprintf("unsupported delivery channel: %s (only push and websocket are supported)", channel)
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

	// Add additional context
	costBuilder.WithProperty("notification_type", notification.Type)
	costBuilder.WithProperty("delivery_channel", channel)
	costBuilder.WithProperty("user_preferences_enabled", userPrefs != nil)
	costBuilder.WithTag("domain", np.domain)
	costBuilder.WithTag("delivery_method", channel)

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
		zap.Duration("total_duration", totalDuration),
		zap.Duration("delivery_duration", deliveryDuration),
		zap.Int64("cost_micro_cents", result.Cost),
		zap.Float64("cost_dollars", float64(result.Cost)/1_000_000.0),
	)

	return result
}

func (np *NotificationProcessor) deliverPush(ctx context.Context, notification *models.Notification, _ *UserPreferences) error {
	if np.snsClient == nil {
		return fmt.Errorf("SNS client not initialized")
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
		return fmt.Errorf("failed to marshal push payload: %w", err)
	}

	// Send push notification via SNS to FCM/APNS
	err = np.sendPushNotification(ctx, notification.UserID, payloadJSON)
	if err != nil {
		np.logger.Error("failed to send push notification",
			zap.String("notification_id", notification.ID),
			zap.String("user_id", notification.UserID),
			zap.Error(err))
		return fmt.Errorf("failed to send push notification: %w", err)
	}

	np.logger.Info("push notification delivered successfully",
		zap.String("notification_id", notification.ID),
		zap.String("user_id", notification.UserID))

	// Mark push as sent in the notification
	if err := np.notificationRepo.MarkPushNotificationSent(ctx, notification.ID); err != nil {
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
		return fmt.Errorf("failed to get websocket connections: %w", err)
	}

	if len(connections) == 0 {
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
		return fmt.Errorf("failed to deliver to any websocket connections: %w", lastError)
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
	if np.apiGatewayClient == nil {
		return fmt.Errorf("API Gateway client not initialized")
	}

	messageBytes, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal websocket message: %w", err)
	}

	_, err = np.apiGatewayClient.PostToConnection(ctx, &apigatewaymanagementapi.PostToConnectionInput{
		ConnectionId: aws.String(connectionID),
		Data:         messageBytes,
	})

	return err
}

// Removed unused getActorName function
// If needed in the future, this function looked up actor details from storage

func (np *NotificationProcessor) getUserPreferences(ctx context.Context, userID string) (*UserPreferences, error) {
	// Get user preferences from storage
	userPrefs, err := np.userRepo.GetUserPreferences(ctx, userID)
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
	_ = userPrefs // Use the variable to avoid unused error
	return &UserPreferences{
		PushNotifications:      true, // Could be derived from userPrefs fields
		WebSocketNotifications: true, // Could be derived from userPrefs fields
		PushEndpoint:           "",   // Would be stored in user preferences
	}, nil
}

func (np *NotificationProcessor) getActiveWebSocketConnections(_ context.Context, userID string) ([]string, error) {
	// Query the WebSocket connections from storage using userRepo
	// In a full implementation, this would query a WebSocket connections table
	// For now, we'll return an empty list as a safe fallback
	connections := []struct {
		ConnectionID string
	}{}
	var err error
	if err != nil {
		np.logger.Warn("failed to get active websocket connections",
			zap.String("user_id", userID),
			zap.Error(err))
		// Return empty slice on error - websocket delivery is not critical
		return []string{}, nil
	}

	// Extract connection IDs
	connectionIDs := make([]string, len(connections))
	for i, conn := range connections {
		connectionIDs[i] = conn.ConnectionID
	}

	return connectionIDs, nil
}

// sendPushNotification sends a push notification via SNS to FCM/APNS
func (np *NotificationProcessor) sendPushNotification(ctx context.Context, userID string, payload []byte) error {
	// Get user's push notification endpoints from preferences
	// In a full implementation, users would have registered FCM tokens or APNS device tokens

	// Create SNS message for push notification
	message := string(payload)

	// Get push notification topic from environment
	pushTopicArn := os.Getenv("PUSH_NOTIFICATION_TOPIC_ARN")
	if pushTopicArn == "" {
		return fmt.Errorf("PUSH_NOTIFICATION_TOPIC_ARN not configured")
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
		return fmt.Errorf("failed to publish push notification to SNS: %w", err)
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
	logger    *zap.Logger
	cfg       *config.Config
	processor *NotificationProcessor
	db        core.DB
)

func init() {
	// Initialize logger
	logger = common.Logger()

	// Load configuration
	cfg = config.Get()

	// Initialize DynamORM with Lambda optimizations
	var err error
	db, err = dynamorm.NewLambdaOptimizedClient(context.Background(), cfg.Region)
	if err != nil {
		logger.Fatal("Failed to initialize DynamORM", zap.Error(err))
	}

	// Initialize processor
	processor = NewNotificationProcessor(db, cfg.DynamoTableName, cfg.Domain)
}

func main() {
	// Create Lift app
	app := lift.New()

	// Add request ID middleware
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			requestID := fmt.Sprintf("notification-%d", time.Now().UnixNano())
			ctx.Set("requestID", requestID)
			return next.Handle(ctx)
		})
	})

	// Add logging middleware
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			start := time.Now()
			requestID := ctx.Get("requestID").(string)

			logger.Info("processing SQS batch",
				zap.String("request_id", requestID),
			)

			err := next.Handle(ctx)

			duration := time.Since(start)
			if err != nil {
				logger.Error("failed to process SQS batch",
					zap.String("request_id", requestID),
					zap.Error(err),
					zap.Duration("duration", duration),
				)
			} else {
				logger.Info("successfully processed SQS batch",
					zap.String("request_id", requestID),
					zap.Duration("duration", duration),
				)
			}

			return err
		})
	})

	// Add error handling middleware
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			err := next.Handle(ctx)
			if err != nil {
				processor.logger.Error("handler error",
					zap.String("request_id", ctx.Get("requestID").(string)),
					zap.Error(err),
				)
			}
			return err
		})
	})

	// Set SQS handler for notification delivery
	_ = app.SQS("notification-delivery", func(ctx *lift.Context) error {
		// Extract SQS event from Lift context
		if ctx.Request.RawEvent == nil {
			return lift.NewLiftError("MISSING_EVENT", "no SQS event in request", 400)
		}

		// Parse the raw event as SQS event
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
		// On error, allow the notification (fail open)
		return true, nil
	}

	// If no budget is set, use default limits
	if budget == nil {
		// Default budget: $0.01 per user per day (1000 micro-cents)
		dailyBudgetMicroCents := int64(1000)

		np.logger.Debug("no budget set, using default",
			zap.String("username", username),
			zap.Int64("estimated_cost_micro_cents", estimatedCostMicroCents),
			zap.Int64("daily_budget_micro_cents", dailyBudgetMicroCents))

		// For now, always allow (budget checking can be enabled by setting budgets)
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
