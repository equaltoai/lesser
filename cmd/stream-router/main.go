// Package main implements the stream-router Lambda function for routing streaming events to WebSocket connections.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/apigatewaymanagementapi"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/lift/patterns"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm/stream"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/streaming"
)

// DynamoDB stream event type constants
const (
	eventNameInsert = "INSERT"
	eventNameModify = "MODIFY"
	eventNameRemove = "REMOVE"
)

// Stream event type constants
const (
	streamEventUpdate       = "update"
	streamEventNotification = "notification"
	streamEventStatusUpdate = "status.update"
)

// Entity type constants
const (
	entityTypeStatus       = "status"
	entityTypeNotification = "notification"
)

// Media type constants
const (
	mediaTypeImage   = "image"
	mediaTypeVideo   = "video"
	mediaTypeAudio   = "audio"
	mediaTypeUnknown = "unknown"
)

// StreamMessage represents a message sent over WebSocket
type StreamMessage struct {
	Event   string          `json:"event"`
	Payload json.RawMessage `json:"payload"`
	Stream  string          `json:"stream"`
}

// Notification represents a notification extracted from DynamoDB stream
type Notification struct {
	ID        string    `dynamodbav:"id"`
	Type      string    `dynamodbav:"type"`                // follow, mention, favourite, reblog
	Username  string    `dynamodbav:"username"`            // Recipient of the notification
	AccountID string    `dynamodbav:"account_id"`          // Who triggered the notification
	StatusID  string    `dynamodbav:"status_id,omitempty"` // Related status (if any)
	Read      bool      `dynamodbav:"read"`
	CreatedAt time.Time `dynamodbav:"created_at"`
}

// StreamRouterHandler handles DynamoDB stream events and routes them to WebSocket subscribers
type StreamRouterHandler struct {
	db                 core.DB
	tableName          string
	logger             *zap.Logger
	apiClient          *apigatewaymanagementapi.Client
	subscriptionsTable string
	wsEndpoint         string
	userRepo           *repositories.UserRepository
	actorRepo          *repositories.ActorRepository
	statusRepo         *repositories.StatusRepository
	eventBus           *streaming.EventBus
	domain             string
}

var (
	globalCfg aws.Config
	logger    *zap.Logger
	handler   *StreamRouterHandler
)

func init() {
	// Initialize logger
	logger = common.Logger()

	// Initialize AWS config
	var err error
	globalCfg, err = config.LoadDefaultConfig(context.Background())
	if err != nil {
		logger.Fatal("failed to load AWS config", zap.Error(err))
	}

	// Initialize the handler
	handler, err = NewStreamRouterHandler()
	if err != nil {
		logger.Fatal("failed to create stream router handler", zap.Error(err))
	}
}

// NewStreamRouterHandler creates a new stream router handler with DynamORM
func NewStreamRouterHandler() (*StreamRouterHandler, error) {
	// Initialize DynamORM database connection
	lambdaDB, err := dynamorm.GetLambdaClient(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to initialize DynamORM: %w", err)
	}

	// Get environment variables
	subscriptionsTable := os.Getenv("SUBSCRIPTIONS_TABLE")
	if subscriptionsTable == "" {
		subscriptionsTable = "lesser-streaming-subscriptions"
	}

	wsEndpoint := os.Getenv("WEBSOCKET_ENDPOINT")
	if wsEndpoint == "" {
		return nil, fmt.Errorf("WEBSOCKET_ENDPOINT environment variable not set")
	}

	// Initialize repositories
	db := lambdaDB.WithLambdaTimeoutBuffer(500) // 500ms buffer
	tableName := "lesser-main"

	// Get domain from environment
	domain := os.Getenv("DOMAIN_NAME")
	if domain == "" {
		domain = "localhost"
		logger.Warn("DOMAIN_NAME not set, using localhost as default")
	}

	userRepo := repositories.NewUserRepository(db, tableName, logger)
	actorRepo := repositories.NewActorRepository(db, tableName, logger)
	statusRepo := repositories.NewStatusRepository(db, tableName, logger)

	// Initialize API Gateway Management API client
	apiClient := apigatewaymanagementapi.NewFromConfig(globalCfg, func(o *apigatewaymanagementapi.Options) {
		o.BaseEndpoint = aws.String(wsEndpoint)
	})

	// Initialize and start the internal event bus
	eventBusConfig := streaming.DefaultEventBusConfig()
	eventBusConfig.BufferSize = 2000    // Larger buffer for high-throughput streams
	eventBusConfig.MaxSubscribers = 500 // Reasonable limit for GraphQL subscriptions

	eventBus := streaming.NewEventBus(eventBusConfig, logger)

	// Start the event bus in a background context
	// We use a background context here since the Lambda will manage the lifecycle
	if err := eventBus.Start(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to start internal event bus: %w", err)
	}

	logger.Info("internal event bus started for stream router")

	return &StreamRouterHandler{
		db:                 db,
		tableName:          tableName,
		logger:             logger,
		apiClient:          apiClient,
		subscriptionsTable: subscriptionsTable,
		wsEndpoint:         wsEndpoint,
		userRepo:           userRepo,
		actorRepo:          actorRepo,
		statusRepo:         statusRepo,
		eventBus:           eventBus,
		domain:             domain,
	}, nil
}

// HandleStream implements the patterns.DynamoDBStreamHandler interface
func (h *StreamRouterHandler) HandleStream(ctx *lift.Context, event events.DynamoDBEvent) error {
	h.logger.Info("processing stream router batch",
		zap.String("request_id", ctx.GetRequestID()),
		zap.Int("record_count", len(event.Records)),
	)

	// Process all records, collecting errors but not failing fast
	var errors []error
	for _, record := range event.Records {
		if err := h.processRecord(ctx, record); err != nil {
			h.logger.Error("failed to process record",
				zap.String("request_id", ctx.GetRequestID()),
				zap.String("event_id", record.EventID),
				zap.Error(err),
			)
			errors = append(errors, err)
			// Continue processing other records
		}
	}

	// Return error only if all records failed
	if len(errors) == len(event.Records) && len(errors) > 0 {
		return fmt.Errorf("all %d records failed processing", len(errors))
	}

	// Log partial failures but don't return error
	if len(errors) > 0 {
		h.logger.Warn("partial batch failure",
			zap.String("request_id", ctx.GetRequestID()),
			zap.Int("failed_count", len(errors)),
			zap.Int("total_count", len(event.Records)),
		)
	}

	return nil
}

// processRecord processes a single DynamoDB stream record using DynamORM patterns
func (h *StreamRouterHandler) processRecord(ctx *lift.Context, record events.DynamoDBEventRecord) error {
	logger := h.logger.With(
		zap.String("request_id", ctx.GetRequestID()),
		zap.String("event_name", record.EventName),
		zap.String("event_id", record.EventID),
	)

	// Only process INSERT and MODIFY events
	if record.EventName != eventNameInsert && record.EventName != eventNameModify {
		return nil
	}

	// Determine the entity type from the stream record
	entityType, err := stream.GetEventType(record)
	if err != nil {
		logger.Debug("failed to get entity type", zap.Error(err))
		return nil // Skip records we can't identify
	}

	logger = logger.With(zap.String("entity_type", entityType))

	// Route to appropriate handler based on entity type
	switch entityType {
	case "STATUS", "OBJECT":
		return h.processStatusEvent(ctx, record)
	case "NOTIFICATION":
		return h.processNotificationEvent(ctx, record)
	case "USER", "ACTOR":
		return h.processAccountEvent(ctx, record)
	case "TOMBSTONE":
		return h.processTombstoneEvent(ctx, record)
	default:
		logger.Debug("ignoring event for unknown entity type")
		return nil
	}
}

// processStatusEvent processes status/object events using DynamORM stream utilities
func (h *StreamRouterHandler) processStatusEvent(ctx *lift.Context, record events.DynamoDBEventRecord) error {
	// Use DynamORM stream utilities to unmarshal the status
	var status models.Status
	if err := stream.UnmarshalItem(record, &status); err != nil {
		h.logger.Debug("failed to unmarshal status from stream",
			zap.String("request_id", ctx.GetRequestID()),
			zap.Error(err))
		return nil // Skip records we can't unmarshal
	}

	// Validate that we have a proper status
	if status.StatusID == "" || status.Note == nil {
		h.logger.Debug("invalid status record, missing required fields",
			zap.String("request_id", ctx.GetRequestID()))
		return nil
	}

	// Create simplified status payload for WebSocket using the DynamORM status model
	note := status.Note
	statusPayload := map[string]any{
		"id":           status.StatusID,
		"uri":          note.ID,
		"url":          note.ID, // Use ID as URL since Note doesn't have a separate URL field
		"content":      status.Content,
		"created_at":   note.Published.Format(time.RFC3339),
		"visibility":   status.Visibility,
		"language":     status.Language,
		"spoiler_text": note.Summary,
		"sensitive":    status.Sensitive,
		"account": map[string]any{
			"id":       status.AuthorUsername,
			"username": status.AuthorUsername,
			"acct":     status.AuthorUsername,
			"url":      status.AuthorID,
		},
		"media_attachments": []any{},
		"mentions":          []any{},
		"tags":              []any{},
		"emojis":            []any{},
		"reblogs_count":     0,
		"favourites_count":  0,
		"replies_count":     0,
	}

	// Add reply info if present
	if status.InReplyToID != "" {
		statusPayload["in_reply_to_id"] = status.InReplyToID
		statusPayload["in_reply_to_account_id"] = nil
	}

	// Process hashtags from status model
	for _, hashtag := range status.Hashtags {
		statusPayload["tags"] = append(statusPayload["tags"].([]any), map[string]any{
			"name": hashtag,
			"url":  fmt.Sprintf("https://%s/tags/%s", h.domain, hashtag),
		})
	}

	// Process mentions from status model
	for _, mention := range status.Mentions {
		statusPayload["mentions"] = append(statusPayload["mentions"].([]any), map[string]any{
			"id":       mention,
			"username": mention,
			"acct":     mention,
			"url":      fmt.Sprintf("https://%s/@%s", h.domain, mention),
		})
	}

	// Process attachments from Note
	if len(note.Attachment) > 0 {
		attachments := []any{}
		for _, att := range note.Attachment {
			// Convert ActivityPub attachment to Mastodon API format
			attachment := map[string]any{
				"id":          generateAttachmentID(att.URL), // Generate ID from URL
				"type":        mapAttachmentType(att.Type),
				"url":         att.URL,
				"preview_url": att.URL, // Use same URL for preview
				"remote_url":  att.URL,
				"meta":        map[string]any{},
			}

			// Add metadata if available
			if att.Name != "" {
				attachment["description"] = att.Name
			}

			// Add dimensions if available
			if att.Width > 0 && att.Height > 0 {
				attachment["meta"] = map[string]any{
					"original": map[string]any{
						"width":  att.Width,
						"height": att.Height,
						"size":   fmt.Sprintf("%dx%d", att.Width, att.Height),
						"aspect": float64(att.Width) / float64(att.Height),
					},
				}
			}

			attachments = append(attachments, attachment)
		}
		statusPayload["media_attachments"] = attachments
	}

	// Marshal the status for payload
	payload, err := json.Marshal(statusPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal status: %w", err)
	}

	// Route to appropriate streams based on visibility
	streams := []string{}

	// Public timelines
	if status.Visibility == "public" {
		streams = append(streams, "public", "public:local")
	}

	// User stream for the author
	if status.AuthorUsername != "" {
		streams = append(streams, fmt.Sprintf("user:%s", status.AuthorUsername))
	}

	// Send to all relevant streams
	eventType := streamEventUpdate
	if record.EventName == eventNameModify {
		eventType = streamEventStatusUpdate
	}

	for _, streamName := range streams {
		if err := h.broadcastToStream(ctx, streamName, eventType, payload); err != nil {
			h.logger.Error("failed to broadcast to stream",
				zap.String("request_id", ctx.GetRequestID()),
				zap.String("stream", streamName),
				zap.Error(err))
			// Continue with other streams
		}
	}

	// Publish to internal event bus for GraphQL subscriptions
	if err := h.publishStatusEventToInternalBus(ctx, record, &status, streams); err != nil {
		h.logger.Warn("failed to publish status event to internal bus",
			zap.String("request_id", ctx.GetRequestID()),
			zap.String("status_id", status.StatusID),
			zap.Error(err))
		// Don't return error - this is not critical for WebSocket functionality
	}

	return nil
}

// processNotificationEvent processes notification events using DynamORM stream utilities
func (h *StreamRouterHandler) processNotificationEvent(ctx *lift.Context, record events.DynamoDBEventRecord) error {
	// Extract the notification from the DynamoDB image
	if record.EventName != eventNameInsert {
		return nil
	}

	// Convert from events.DynamoDBAttributeValue to SDK v2 types
	notifItem := make(map[string]types.AttributeValue)
	for k, v := range record.Change.NewImage {
		notifItem[k] = convertEventAttributeValue(v)
	}

	// Check if this is a notification record
	pkAttr, hasPK := notifItem["PK"]
	if !hasPK {
		return nil
	}

	pk := ""
	if s, ok := pkAttr.(*types.AttributeValueMemberS); ok {
		pk = s.Value
	}

	// Only process NOTIFICATION# records
	if !strings.HasPrefix(pk, "NOTIFICATION#") {
		return nil
	}

	// Extract the notification (struct defined at top level)

	var notifRecord struct {
		PK           string        `dynamodbav:"PK"`
		SK           string        `dynamodbav:"SK"`
		Notification *Notification `dynamodbav:"Notification"`
		CreatedAt    time.Time     `dynamodbav:"CreatedAt"`
	}

	if err := attributevalue.UnmarshalMap(notifItem, &notifRecord); err != nil {
		h.logger.Error("failed to unmarshal notification record",
			zap.String("request_id", ctx.GetRequestID()),
			zap.Error(err))
		return nil
	}

	notification := notifRecord.Notification
	if notification == nil {
		return nil
	}

	// Create simplified notification payload for WebSocket
	notifPayload := map[string]any{
		"id":         notification.ID,
		"type":       notification.Type,
		"created_at": notification.CreatedAt.Format(time.RFC3339),
		"account": map[string]any{
			"id":       extractUsernameFromActorID(notification.AccountID),
			"username": extractUsernameFromActorID(notification.AccountID),
			"acct":     extractUsernameFromActorID(notification.AccountID),
			"url":      notification.AccountID,
		},
	}

	// Add status info if present
	if notification.StatusID != "" {
		notifPayload[entityTypeStatus] = map[string]any{
			"id": strings.TrimPrefix(notification.StatusID, "https://"),
		}
	}

	// Marshal the notification for payload
	payload, err := json.Marshal(notifPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	// Use the username from the notification (recipient)
	username := notification.Username
	if username == "" {
		return fmt.Errorf("notification missing recipient username")
	}

	// Send to user's notification stream
	streamName := fmt.Sprintf("user:notification:%s", username)
	if err := h.broadcastToStream(ctx, streamName, streamEventNotification, payload); err != nil {
		h.logger.Error("failed to broadcast notification to stream",
			zap.String("request_id", ctx.GetRequestID()),
			zap.String("stream", streamName),
			zap.Error(err))
		// Continue with internal event bus
	}

	// Publish to internal event bus for GraphQL subscriptions
	if err := h.publishNotificationEventToInternalBus(ctx, notification, []string{streamName}); err != nil {
		h.logger.Warn("failed to publish notification event to internal bus",
			zap.String("request_id", ctx.GetRequestID()),
			zap.String("notification_id", notification.ID),
			zap.Error(err))
		// Don't return error - this is not critical for WebSocket functionality
	}

	return nil
}

// processAccountEvent processes account/user events using DynamORM stream utilities
func (h *StreamRouterHandler) processAccountEvent(ctx *lift.Context, record events.DynamoDBEventRecord) error {
	// Account updates (profile changes, etc.)
	if record.EventName != eventNameModify {
		return nil
	}

	// Convert from events.DynamoDBAttributeValue to SDK v2 types
	accountItem := make(map[string]types.AttributeValue)
	for k, v := range record.Change.NewImage {
		accountItem[k] = convertEventAttributeValue(v)
	}

	// Get the account ID
	accountID := ""
	if attr, ok := accountItem["ID"]; ok {
		if s, ok := attr.(*types.AttributeValueMemberS); ok {
			accountID = s.Value
		}
	}

	if accountID == "" {
		return fmt.Errorf("account missing ID")
	}

	// Create proper account payload for streaming
	accountPayload, err := createAccountPayload(accountID, record.EventName)
	if err != nil {
		return fmt.Errorf("failed to create account payload: %w", err)
	}

	payload, err := json.Marshal(accountPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal account: %w", err)
	}

	// Send account update to followers' streams
	if err := broadcastToFollowers(accountID, payload); err != nil {
		h.logger.Error("failed to broadcast to followers",
			zap.String("request_id", ctx.GetRequestID()),
			zap.Error(err))
		// Don't return error - logging is sufficient for broadcast failures
	}

	// Publish to internal event bus for GraphQL subscriptions
	streams := []string{fmt.Sprintf("account:%s", accountID)}
	if err := h.publishAccountEventToInternalBus(ctx, accountID, record.EventName, streams); err != nil {
		h.logger.Warn("failed to publish account event to internal bus",
			zap.String("request_id", ctx.GetRequestID()),
			zap.String("account_id", accountID),
			zap.Error(err))
		// Don't return error - this is not critical for WebSocket functionality
	}

	h.logger.Info("account update event",
		zap.String("request_id", ctx.GetRequestID()),
		zap.String("account_id", accountID),
		zap.Int("payload_size", len(payload)))

	return nil
}

// convertEventAttributeValue converts from events.DynamoDBAttributeValue to SDK v2 types.AttributeValue
func convertEventAttributeValue(attr events.DynamoDBAttributeValue) types.AttributeValue {
	switch attr.DataType() {
	case events.DataTypeString:
		return &types.AttributeValueMemberS{Value: attr.String()}
	case events.DataTypeNumber:
		return &types.AttributeValueMemberN{Value: attr.Number()}
	case events.DataTypeBinary:
		return &types.AttributeValueMemberB{Value: attr.Binary()}
	case events.DataTypeBoolean:
		return &types.AttributeValueMemberBOOL{Value: attr.Boolean()}
	case events.DataTypeNull:
		return &types.AttributeValueMemberNULL{Value: true}
	case events.DataTypeList:
		list := make([]types.AttributeValue, 0, len(attr.List()))
		for _, item := range attr.List() {
			list = append(list, convertEventAttributeValue(item))
		}
		return &types.AttributeValueMemberL{Value: list}
	case events.DataTypeMap:
		m := make(map[string]types.AttributeValue)
		for k, v := range attr.Map() {
			m[k] = convertEventAttributeValue(v)
		}
		return &types.AttributeValueMemberM{Value: m}
	case events.DataTypeStringSet:
		return &types.AttributeValueMemberSS{Value: attr.StringSet()}
	case events.DataTypeNumberSet:
		return &types.AttributeValueMemberNS{Value: attr.NumberSet()}
	case events.DataTypeBinarySet:
		return &types.AttributeValueMemberBS{Value: attr.BinarySet()}
	default:
		return nil
	}
}

// Helper function to extract username from actor ID
func extractUsernameFromActorID(actorID string) string {
	// Extract username from actor ID like "https://example.com/users/alice" -> "alice"
	parts := strings.Split(actorID, "/")
	if len(parts) >= 2 && parts[len(parts)-2] == "users" {
		return parts[len(parts)-1]
	}
	return ""
}

// createAccountPayload creates a proper account payload for streaming
func createAccountPayload(accountID, eventType string) (map[string]any, error) {
	// Create account streaming payload with proper structure
	payload := map[string]any{
		"id":         accountID,
		"event_type": eventType,
		"type":       "account",
		"timestamp":  time.Now().Unix(),
	}

	// In a full implementation, this would fetch the full account details
	// and format them according to the Mastodon streaming API spec
	return payload, nil
}

// broadcastToFollowers sends updates to all followers of an account
func broadcastToFollowers(accountID string, payload []byte) error {
	// Query followers for this account
	// This would typically involve:
	// 1. Query the followers table/index to get all followers
	// 2. For each follower, find their active streaming connections
	// 3. Send the payload to each connection

	// For now, implement basic logging - in production this would:
	// - Query GSI2 to get followers
	// - Query subscriptions table for each follower's connections
	// - Send via API Gateway WebSocket

	logger.Info("broadcasting account update to followers",
		zap.String("account_id", accountID),
		zap.Int("payload_size", len(payload)))

	// Placeholder implementation - would be replaced with actual follower query and broadcast
	return nil
}

// Helper methods for StreamRouterHandler

// publishStatusEventToInternalBus publishes a status event to the internal event bus
func (h *StreamRouterHandler) publishStatusEventToInternalBus(ctx *lift.Context, record events.DynamoDBEventRecord, status *models.Status, streams []string) error {
	// Determine event type and action
	var eventType streaming.EventType
	var action streaming.EventAction

	switch record.EventName {
	case eventNameInsert:
		eventType = streaming.EventTypeStatus
		action = streaming.ActionCreate
	case eventNameModify:
		eventType = streaming.EventTypeStatusUpdate
		action = streaming.ActionUpdate
	case eventNameRemove:
		eventType = streaming.EventTypeStatusDelete
		action = streaming.ActionDelete
	default:
		return fmt.Errorf("unknown event name: %s", record.EventName)
	}

	// Create status event payload
	statusPayload := &streaming.StatusEventPayload{
		StatusID:       status.StatusID,
		AuthorID:       status.AuthorID,
		AuthorUsername: status.AuthorUsername,
		Content:        status.Content,
		Visibility:     status.Visibility,
		InReplyToID:    status.InReplyToID,
		ReblogOfID:     status.ReblogOfID,
		Sensitive:      status.Sensitive,
		Language:       status.Language,
		Hashtags:       status.Hashtags,
		Mentions:       status.Mentions,
		CreatedAt:      status.PublishedAt,
		UpdatedAt:      status.UpdatedAt,
	}

	// Create the internal event
	event := streaming.CreateEvent(eventType, action, statusPayload)
	event.WithActor(status.AuthorUsername).
		WithTarget(status.StatusID).
		WithUser(status.AuthorUsername).
		WithStreams(streams...).
		WithPriority(streaming.PriorityNormal)

	// Add metadata for filtering
	event.WithMetadata("visibility", status.Visibility)
	event.WithMetadata("entity_type", entityTypeStatus)
	event.WithMetadata("request_id", ctx.GetRequestID())

	if status.Language != "" {
		event.WithMetadata("language", status.Language)
	}

	// Add hashtags to metadata for filtering
	if len(status.Hashtags) > 0 {
		for i, hashtag := range status.Hashtags {
			event.WithMetadata(fmt.Sprintf("hashtag_%d", i), hashtag)
		}
	}

	// Publish to the internal event bus
	if err := h.eventBus.Publish(event); err != nil {
		return fmt.Errorf("failed to publish to internal event bus: %w", err)
	}

	h.logger.Debug("published status event to internal bus",
		zap.String("request_id", ctx.GetRequestID()),
		zap.String("event_id", event.ID),
		zap.String("status_id", status.StatusID),
		zap.String("event_type", string(eventType)),
		zap.Strings("streams", streams))

	return nil
}

// publishNotificationEventToInternalBus publishes a notification event to the internal event bus
func (h *StreamRouterHandler) publishNotificationEventToInternalBus(ctx *lift.Context, notification *Notification, streams []string) error {
	// Create notification event payload
	notificationPayload := &streaming.NotificationEventPayload{
		NotificationID: notification.ID,
		Type:           notification.Type,
		RecipientID:    notification.Username,
		ActorID:        notification.AccountID,
		StatusID:       notification.StatusID,
		Read:           notification.Read,
		CreatedAt:      notification.CreatedAt,
	}

	// Create the internal event
	event := streaming.CreateEvent(streaming.EventTypeNotification, streaming.ActionCreate, notificationPayload)
	event.WithActor(notification.AccountID).
		WithTarget(notification.ID).
		WithUser(notification.Username).
		WithStreams(streams...).
		WithPriority(streaming.PriorityHigh) // Notifications are high priority

	// Add metadata for filtering
	event.WithMetadata("notification_type", notification.Type)
	event.WithMetadata("entity_type", entityTypeNotification)
	event.WithMetadata("recipient", notification.Username)
	event.WithMetadata("request_id", ctx.GetRequestID())

	if notification.StatusID != "" {
		event.WithMetadata("status_id", notification.StatusID)
	}

	// Publish to the internal event bus
	if err := h.eventBus.Publish(event); err != nil {
		return fmt.Errorf("failed to publish to internal event bus: %w", err)
	}

	h.logger.Debug("published notification event to internal bus",
		zap.String("request_id", ctx.GetRequestID()),
		zap.String("event_id", event.ID),
		zap.String("notification_id", notification.ID),
		zap.String("notification_type", notification.Type),
		zap.Strings("streams", streams))

	return nil
}

// publishAccountEventToInternalBus publishes an account event to the internal event bus
func (h *StreamRouterHandler) publishAccountEventToInternalBus(ctx *lift.Context, accountID, eventName string, streams []string) error {
	// Determine event type and action
	var eventType streaming.EventType
	var action streaming.EventAction

	switch eventName {
	case eventNameInsert:
		eventType = streaming.EventTypeAccountUpdate
		action = streaming.ActionCreate
	case eventNameModify:
		eventType = streaming.EventTypeAccountUpdate
		action = streaming.ActionUpdate
	case eventNameRemove:
		eventType = streaming.EventTypeAccountUpdate
		action = streaming.ActionDelete
	default:
		return fmt.Errorf("unknown event name: %s", eventName)
	}

	// Create account event payload (simplified for now)
	// In a full implementation, we'd extract more details from the DynamoDB record
	accountPayload := &streaming.AccountEventPayload{
		AccountID: accountID,
		Username:  extractUsernameFromActorID(accountID),
		UpdatedAt: time.Now(),
	}

	// Create the internal event
	event := streaming.CreateEvent(eventType, action, accountPayload)
	event.WithActor(accountID).
		WithTarget(accountID).
		WithUser(extractUsernameFromActorID(accountID)).
		WithStreams(streams...).
		WithPriority(streaming.PriorityNormal)

	// Add metadata for filtering
	event.WithMetadata("entity_type", "account")
	event.WithMetadata("account_id", accountID)
	event.WithMetadata("request_id", ctx.GetRequestID())

	// Publish to the internal event bus
	if err := h.eventBus.Publish(event); err != nil {
		return fmt.Errorf("failed to publish to internal event bus: %w", err)
	}

	h.logger.Debug("published account event to internal bus",
		zap.String("request_id", ctx.GetRequestID()),
		zap.String("event_id", event.ID),
		zap.String("account_id", accountID),
		zap.String("event_type", string(eventType)),
		zap.Strings("streams", streams))

	return nil
}

// GetEventBus returns the internal event bus for external subscribers (like GraphQL)
func (h *StreamRouterHandler) GetEventBus() *streaming.EventBus {
	return h.eventBus
}

// GetEventBusMetrics returns metrics about the internal event bus
func (h *StreamRouterHandler) GetEventBusMetrics() *streaming.EventBusMetrics {
	if h.eventBus == nil {
		return nil
	}
	return h.eventBus.GetMetrics()
}

// broadcastToStream sends a message to a specific stream
func (h *StreamRouterHandler) broadcastToStream(ctx *lift.Context, streamName, eventType string, payload []byte) error {
	// Query subscriptions for this stream from DynamoDB
	subscriptions, err := h.getStreamSubscriptions(ctx, streamName)
	if err != nil {
		return fmt.Errorf("failed to get subscriptions for stream %s: %w", streamName, err)
	}

	if len(subscriptions) == 0 {
		h.logger.Debug("no active subscriptions for stream",
			zap.String("request_id", ctx.GetRequestID()),
			zap.String("stream", streamName))
		return nil
	}

	// Create the WebSocket message
	message := StreamMessage{
		Event:   eventType,
		Payload: payload,
		Stream:  streamName,
	}

	messageData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Send to all subscribed connections
	var errors []error
	successCount := 0

	for _, connectionID := range subscriptions {
		input := &apigatewaymanagementapi.PostToConnectionInput{
			ConnectionId: aws.String(connectionID),
			Data:         messageData,
		}

		_, err := h.apiClient.PostToConnection(ctx, input)
		if err != nil {
			h.logger.Warn("failed to send to connection",
				zap.String("request_id", ctx.GetRequestID()),
				zap.String("connection_id", connectionID),
				zap.String("stream", streamName),
				zap.Error(err))
			errors = append(errors, err)

			// Remove stale connections
			if isStaleConnection(err) {
				h.removeSubscription(ctx, streamName, connectionID)
			}
		} else {
			successCount++
		}
	}

	h.logger.Info("broadcast completed",
		zap.String("request_id", ctx.GetRequestID()),
		zap.String("stream", streamName),
		zap.String("event", eventType),
		zap.Int("total_connections", len(subscriptions)),
		zap.Int("successful", successCount),
		zap.Int("failed", len(errors)))

	if len(errors) > 0 {
		return fmt.Errorf("failed to send to %d connections", len(errors))
	}

	return nil
}

// GetGlobalEventBus returns the global stream router's event bus for external access
// This allows other parts of the system (like GraphQL) to subscribe to internal events
func GetGlobalEventBus() *streaming.EventBus {
	if handler == nil {
		return nil
	}
	return handler.GetEventBus()
}

// generateAttachmentID generates a stable ID from attachment URL
func generateAttachmentID(url string) string {
	// Use last part of URL as ID, or hash if needed
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		lastPart := parts[len(parts)-1]
		// Remove file extension if present
		if idx := strings.LastIndex(lastPart, "."); idx > 0 {
			return lastPart[:idx]
		}
		return lastPart
	}
	// Fallback: use a simple hash
	return fmt.Sprintf("%x", url)
}

// mapAttachmentType maps ActivityPub attachment types to Mastodon API types
func mapAttachmentType(apType string) string {
	switch strings.ToLower(apType) {
	case mediaTypeImage:
		return mediaTypeImage
	case mediaTypeVideo:
		return mediaTypeVideo
	case mediaTypeAudio:
		return mediaTypeAudio
	case "document":
		return mediaTypeUnknown
	default:
		// Try to infer from type if it contains media type info
		if strings.Contains(strings.ToLower(apType), mediaTypeImage) {
			return mediaTypeImage
		}
		if strings.Contains(strings.ToLower(apType), mediaTypeVideo) {
			return mediaTypeVideo
		}
		if strings.Contains(strings.ToLower(apType), mediaTypeAudio) {
			return mediaTypeAudio
		}
		return mediaTypeUnknown
	}
}

// GetGlobalEventBusMetrics returns metrics for the global event bus
func GetGlobalEventBusMetrics() *streaming.EventBusMetrics {
	if handler == nil {
		return nil
	}
	return handler.GetEventBusMetrics()
}

// getStreamSubscriptions retrieves active subscriptions for a stream
func (h *StreamRouterHandler) getStreamSubscriptions(ctx *lift.Context, streamName string) ([]string, error) {
	// In a production implementation, this would:
	// 1. Query the subscriptions table/index by stream name
	// 2. Return all active connection IDs for this stream
	// 3. Handle pagination if there are many subscribers

	h.logger.Debug("getting subscriptions for stream",
		zap.String("request_id", ctx.GetRequestID()),
		zap.String("stream", streamName))

	// Placeholder implementation - would query subscription repository
	// Example query pattern:
	// subscriptions, err := h.subscriptionRepo.GetStreamSubscriptions(ctx, streamName)

	// For now, return empty slice to allow compilation
	return []string{}, nil
}

// removeSubscription removes a stale subscription
func (h *StreamRouterHandler) removeSubscription(ctx *lift.Context, streamName, connectionID string) {
	h.logger.Info("removing stale subscription",
		zap.String("request_id", ctx.GetRequestID()),
		zap.String("stream", streamName),
		zap.String("connection_id", connectionID))

	// In a production implementation, this would:
	// 1. Remove the subscription from the database
	// 2. Clean up any associated resources
	// 3. Log the removal for audit purposes

	// Example implementation:
	// err := h.subscriptionRepo.RemoveSubscription(ctx, streamName, connectionID)
	// if err != nil {
	//     h.logger.Error("failed to remove subscription",
	//         zap.String("request_id", ctx.GetRequestID()),
	//         zap.String("stream", streamName),
	//         zap.String("connection_id", connectionID),
	//         zap.Error(err))
	// }
}

// isStaleConnection checks if an error indicates a stale connection
func isStaleConnection(err error) bool {
	if err == nil {
		return false
	}

	// Check for specific API Gateway WebSocket errors that indicate stale connections
	errStr := err.Error()

	// Common indicators of stale connections:
	// - GoneException: Connection no longer exists
	// - 410 Gone: HTTP status for gone resources
	// - Connection not found
	// - Invalid connection ID

	staleIndicators := []string{
		"GoneException",
		"410",
		"Gone",
		"connection not found",
		"invalid connection",
		"connection does not exist",
	}

	for _, indicator := range staleIndicators {
		if strings.Contains(strings.ToLower(errStr), strings.ToLower(indicator)) {
			return true
		}
	}

	return false
}

// processTombstoneEvent processes tombstone events and removes deleted objects from timelines
func (h *StreamRouterHandler) processTombstoneEvent(ctx *lift.Context, record events.DynamoDBEventRecord) error {
	logger := h.logger.With(
		zap.String("request_id", ctx.GetRequestID()),
		zap.String("event_name", record.EventName),
	)

	// Only process INSERT events for tombstones (when they are created)
	if record.EventName != eventNameInsert {
		return nil
	}

	// Convert from events.DynamoDBAttributeValue to SDK v2 types
	tombstoneItem := make(map[string]types.AttributeValue)
	for k, v := range record.Change.NewImage {
		tombstoneItem[k] = convertEventAttributeValue(v)
	}

	// Unmarshal the tombstone record
	var tombstone models.Tombstone
	if err := attributevalue.UnmarshalMap(tombstoneItem, &tombstone); err != nil {
		logger.Error("failed to unmarshal tombstone record", zap.Error(err))
		return nil // Don't fail the stream processing
	}

	logger = logger.With(
		zap.String("object_id", tombstone.ID),
		zap.String("deleted_by", tombstone.DeletedBy),
		zap.String("former_type", tombstone.FormerType),
	)

	logger.Info("processing tombstone event for object deletion")

	// Create deletion messages for different streams based on former object type
	deletionMessage := StreamMessage{
		Event:  "delete",
		Stream: "", // Will be set for each stream
		Payload: json.RawMessage(fmt.Sprintf(`{
			"id": "%s",
			"type": "%s",
			"deleted_by": "%s",
			"deleted_at": "%s",
			"former_type": "%s"
		}`, tombstone.ID, "Tombstone", tombstone.DeletedBy, 
			tombstone.Deleted.Format(time.RFC3339), tombstone.FormerType)),
	}

	// Send deletion events to relevant streams based on the former object type
	if err := h.broadcastDeletionToStreams(ctx, &tombstone, deletionMessage); err != nil {
		logger.Error("failed to broadcast deletion to streams", zap.Error(err))
		// Don't return error - this is not critical
	}

	// Remove the object from user timelines (followers of the deleter)
	if err := h.removeFromFollowerTimelines(ctx, tombstone.DeletedBy, tombstone.ID); err != nil {
		logger.Warn("failed to remove from follower timelines", zap.Error(err))
	}

	logger.Info("successfully processed tombstone event")
	return nil
}

// broadcastDeletionToStreams sends deletion events to appropriate streams
func (h *StreamRouterHandler) broadcastDeletionToStreams(ctx *lift.Context, tombstone *models.Tombstone, message StreamMessage) error {
	objectID := tombstone.ID
	
	// Based on the former object type, determine which streams to update
	switch tombstone.FormerType {
	case "Note", "Article", "Status":
		// Public timeline
		message.Stream = "public"
		if err := h.broadcastMessage(ctx, message); err != nil {
			h.logger.Warn("failed to broadcast deletion to public stream", zap.Error(err))
		}

		// Local timeline (if it was a local object)
		message.Stream = "public:local"
		if err := h.broadcastMessage(ctx, message); err != nil {
			h.logger.Warn("failed to broadcast deletion to local stream", zap.Error(err))
		}

		// Hashtag streams - extract hashtags from the deleted object if available
		if err := h.removeFromHashtagStreams(ctx, objectID); err != nil {
			h.logger.Warn("failed to remove from hashtag streams", zap.Error(err))
		}

	case "Follow", "Like", "Announce":
		// For activity deletions, we might need different handling
		h.logger.Debug("processing deletion of activity object",
			zap.String("object_id", objectID),
			zap.String("type", tombstone.FormerType))
	}

	return nil
}

// removeFromFollowerTimelines removes the deleted object from followers' home timelines  
func (h *StreamRouterHandler) removeFromFollowerTimelines(ctx *lift.Context, actorID, objectID string) error {
	// Extract username from actor ID
	username := h.extractUsernameFromActorID(actorID)
	
	// Get followers (limited batch for stream processing efficiency)
	followers, _, err := h.getFollowersForUser(ctx, username, 100)
	if err != nil {
		return fmt.Errorf("failed to get followers: %w", err)
	}

	// Create deletion message for home timelines
	deletionMessage := StreamMessage{
		Event: "delete",
		Payload: json.RawMessage(fmt.Sprintf(`{"id": "%s"}`, objectID)),
	}

	// Send to each follower's home timeline
	for _, follower := range followers {
		streamName := fmt.Sprintf("user:%s", follower)
		deletionMessage.Stream = streamName
		
		if err := h.broadcastMessage(ctx, deletionMessage); err != nil {
			h.logger.Warn("failed to send deletion to follower timeline",
				zap.String("follower", follower),
				zap.String("object_id", objectID),
				zap.Error(err))
			// Continue with other followers
		}
	}

	return nil
}

// removeFromHashtagStreams removes objects from hashtag streams (implementation placeholder)
func (h *StreamRouterHandler) removeFromHashtagStreams(_ *lift.Context, objectID string) error {
	// This would need to:
	// 1. Look up hashtags associated with the deleted object
	// 2. Send deletion events to each hashtag stream
	// For now, just log that this should be implemented
	h.logger.Debug("hashtag stream removal needed", zap.String("object_id", objectID))
	return nil
}

// getFollowersForUser gets a limited list of followers for an actor (placeholder implementation)
func (h *StreamRouterHandler) getFollowersForUser(_ *lift.Context, username string, limit int) ([]string, string, error) {
	// This would use the relationship repository to get actual followers
	// For now, return empty slice to avoid errors
	h.logger.Debug("getting followers for user", 
		zap.String("username", username),
		zap.Int("limit", limit))
	return []string{}, "", nil
}

// extractUsernameFromActorID extracts username from actor ID URL  
func (h *StreamRouterHandler) extractUsernameFromActorID(actorID string) string {
	// Extract from URLs like https://domain.com/users/username
	parts := strings.Split(actorID, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-1]
	}
	return actorID
}

// broadcastMessage broadcasts a message to WebSocket connections
func (h *StreamRouterHandler) broadcastMessage(ctx *lift.Context, message StreamMessage) error {
	// Marshal the message
	payload, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Use existing broadcast method
	return h.broadcastToStream(ctx, message.Stream, message.Event, payload)
}

func main() {
	// Use the Lift pattern for DynamoDB streams with proper middleware
	patterns.StartDynamoDBStreamLambda("stream-router", handler, logger)
}
