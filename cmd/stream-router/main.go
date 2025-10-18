// Package main implements the stream-router Lambda function for routing streaming events to WebSocket connections.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/apigatewaymanagementapi"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/lift/patterns"
	"github.com/equaltoai/lesser/pkg/mastodon"
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
	accountRepo        *repositories.AccountRepository
	statusRepo         *repositories.StatusRepository
	eventBus           *streaming.EventBus
	publisher          streaming.Publisher
	streamingRepo      *repositories.StreamingConnectionRepository
	domain             string
}

// connectionRepositoryAdapter adapts StreamingConnectionRepository to streaming.ConnectionRepository
type connectionRepositoryAdapter struct {
	streamingRepo *repositories.StreamingConnectionRepository
	logger        *zap.Logger
}

// GetUserConnections implements streaming.ConnectionRepository
func (a *connectionRepositoryAdapter) GetUserConnections(ctx context.Context, userID string) ([]*streaming.StreamConnection, error) {
	connections, err := a.streamingRepo.GetConnectionsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Convert models.WebSocketConnection to streaming.StreamConnection
	streamConns := make([]*streaming.StreamConnection, len(connections))
	for i, conn := range connections {
		streamConns[i] = &streaming.StreamConnection{
			ConnectionID: conn.ConnectionID,
			UserID:       conn.UserID,
			Username:     conn.Username,
			Streams:      conn.Streams,
			LastActivity: conn.LastActivity,
		}
	}

	return streamConns, nil
}

// GetStreamConnections implements streaming.ConnectionRepository
func (a *connectionRepositoryAdapter) GetStreamConnections(ctx context.Context, streamName string) ([]*streaming.StreamConnection, error) {
	subscriptions, err := a.streamingRepo.GetSubscriptionsForStream(ctx, streamName)
	if err != nil {
		return nil, FailedToGetSubscriptionsForStream(err)
	}

	if err := common.ValidateSliceNotEmpty("subscriptions", subscriptions); err != nil {
		return []*streaming.StreamConnection{}, nil
	}

	// Get unique connection IDs from subscriptions
	connectionIDs := make(map[string]bool)
	for _, sub := range subscriptions {
		connectionIDs[sub.ConnectionID] = true
	}

	// We need to get the full connection details for each connection ID
	// Since StreamingConnectionRepository doesn't have GetConnection, we need to implement it
	var streamConns []*streaming.StreamConnection

	for connID := range connectionIDs {
		// Get the connection details by querying for the specific connection
		conn, err := a.getConnectionByID(ctx, connID)
		if err != nil {
			a.logger.Warn("failed to get connection details, skipping",
				zap.String("connection_id", connID),
				zap.String("stream", streamName),
				zap.Error(err))
			continue
		}

		if conn != nil {
			streamConns = append(streamConns, conn)
		}
	}

	return streamConns, nil
}

// getConnectionByID retrieves a connection by its ID
func (a *connectionRepositoryAdapter) getConnectionByID(ctx context.Context, connectionID string) (*streaming.StreamConnection, error) {
	// We need to query the connection directly from the database
	// Since we don't have a direct GetConnection method, we'll scan for it
	var connections []models.WebSocketConnection

	// Query by PK pattern CONN#{connectionID}
	err := a.streamingRepo.GetDB().WithContext(ctx).Model(&models.WebSocketConnection{}).
		Where("PK", "=", fmt.Sprintf("CONN#%s", connectionID)).
		All(&connections)

	if err != nil {
		return nil, FailedToQueryConnection(err)
	}

	if err := common.ValidateSliceNotEmpty("connections", connections); err != nil {
		return nil, ConnectionNotFound()
	}

	conn := connections[0]
	return &streaming.StreamConnection{
		ConnectionID: conn.ConnectionID,
		UserID:       conn.UserID,
		Username:     conn.Username,
		Streams:      conn.Streams,
		LastActivity: conn.LastActivity,
	}, nil
}

// GetConversationConnections implements streaming.ConnectionRepository
func (a *connectionRepositoryAdapter) GetConversationConnections(ctx context.Context, conversationID string) ([]*streaming.StreamConnection, error) {
	// To get conversation connections, we need to:
	// 1. Find all participants in the conversation
	// 2. Get all connections for those participants

	// First, we need to get the conversation participants
	// This would typically require a ConversationRepository to get participants
	// Since we don't have that, we'll use a stream-based approach

	conversationStreamName := fmt.Sprintf("conversation:%s", conversationID)
	return a.GetStreamConnections(ctx, conversationStreamName)
}

// Global variables for standardized Lambda initialization
var (
	lambdaCtx *common.LambdaContext
	handler   *StreamRouterHandler
)

func init() {
	if common.RunningUnitTests() {
		return
	}
	// Standardized Lambda initialization for background processors
	lambdaCtx = common.MustInitializeLambda(common.LambdaConfig{
		ServiceName: "stream-router",
		LambdaType:  common.LambdaTypeProcessor, // Background processing
	})

	// Automatic dependency injection handled by handler initialization

	// Initialize with processor-specific defaults
	err := lambdaCtx.InitializeWithDefaults()
	if err != nil {
		lambdaCtx.Logger.Warn("failed to initialize with defaults", zap.Error(err))
	}

	// Stream router-specific initialization
	var initErr error
	handler, initErr = NewStreamRouterHandler()
	if initErr != nil {
		lambdaCtx.Logger.Fatal("failed to create stream router handler", zap.Error(initErr))
	}
}

// NewStreamRouterHandler creates a new stream router handler with DynamORM
func NewStreamRouterHandler() (*StreamRouterHandler, error) {
	// Get config from the initialized lambda context
	globalCfg := lambdaCtx.AWSServices.Config

	// Use the standardized database connection
	db := lambdaCtx.DynamoDB.(core.DB)
	tableName := lambdaCtx.Config.DynamoTableName

	// Get configuration values
	subscriptionsTable := lambdaCtx.Config.SubscriptionsTable
	if err := common.ValidateRequiredParam("subscriptionsTable", subscriptionsTable); err != nil {
		subscriptionsTable = "lesser-streaming-subscriptions"
	}

	wsEndpoint := lambdaCtx.Config.WebSocketEndpoint
	if err := common.ValidateRequiredParam("wsEndpoint", wsEndpoint); err != nil {
		return nil, WebSocketEndpointNotSet()
	}

	// Get domain from config
	domain := lambdaCtx.Config.Domain
	if err := common.ValidateRequiredParam("domain", domain); err != nil {
		domain = "localhost"
		lambdaCtx.Logger.Warn("DOMAIN_NAME not set, using localhost as default")
	}

	userRepo := repositories.NewUserRepository(db, tableName, lambdaCtx.Logger)
	actorRepo := repositories.NewActorRepository(db, tableName, lambdaCtx.Logger)
	accountRepo := repositories.NewAccountRepository(db, tableName, domain, lambdaCtx.Logger)
	statusRepo := repositories.NewStatusRepository(db, tableName, lambdaCtx.Logger, nil)

	// Initialize API Gateway Management API client
	apiClient := apigatewaymanagementapi.NewFromConfig(globalCfg, func(o *apigatewaymanagementapi.Options) {
		o.BaseEndpoint = aws.String(wsEndpoint)
	})

	// Initialize streaming repository
	streamingRepo := repositories.NewStreamingConnectionRepository(db, tableName, db, subscriptionsTable, lambdaCtx.Logger, nil)

	// Create connection repository adapter
	connRepoAdapter := &connectionRepositoryAdapter{
		streamingRepo: streamingRepo,
		logger:        lambdaCtx.Logger,
	}

	// Initialize publisher
	publisher := streaming.NewAPIGatewayPublisher(apiClient, connRepoAdapter, wsEndpoint, lambdaCtx.Logger)

	// Initialize and start the internal event bus
	eventBusConfig := streaming.DefaultEventBusConfig()
	eventBusConfig.BufferSize = 2000    // Larger buffer for high-throughput streams
	eventBusConfig.MaxSubscribers = 500 // Reasonable limit for GraphQL subscriptions

	eventBus := streaming.NewEventBus(eventBusConfig, lambdaCtx.Logger)

	// Start the event bus in a background context
	// We use a background context here since the Lambda will manage the lifecycle
	if err := eventBus.Start(context.Background()); err != nil {
		return nil, FailedToStartInternalEventBus(err)
	}

	lambdaCtx.Logger.Info("internal event bus started for stream router")

	return &StreamRouterHandler{
		db:                 db,
		tableName:          tableName,
		logger:             lambdaCtx.Logger,
		apiClient:          apiClient,
		subscriptionsTable: subscriptionsTable,
		wsEndpoint:         wsEndpoint,
		userRepo:           userRepo,
		actorRepo:          actorRepo,
		accountRepo:        accountRepo,
		statusRepo:         statusRepo,
		eventBus:           eventBus,
		publisher:          publisher,
		streamingRepo:      streamingRepo,
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
	var errs []error
	for _, record := range event.Records {
		if err := h.processRecord(ctx, record); err != nil {
			h.logger.Error("failed to process record",
				zap.String("request_id", ctx.GetRequestID()),
				zap.String("event_id", record.EventID),
				zap.Error(err),
			)
			errs = append(errs, err)
			// Continue processing other records
		}
	}

	// Return error only if all records failed
	if len(errs) == len(event.Records) {
		if err := common.ValidateSliceNotEmpty("errs", errs); err == nil {
			return AllRecordsFailedProcessing()
		}
	}

	// Log partial failures but don't return error
	if err := common.ValidateSliceNotEmpty("errs", errs); err == nil {
		h.logger.Warn("partial batch failure",
			zap.String("request_id", ctx.GetRequestID()),
			zap.Int("failed_count", len(errs)),
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
	if err := common.ValidateRequiredParam("statusID", status.StatusID); err != nil || status.Note == nil {
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
	if err := common.ValidateSliceNotEmpty("note.Attachment", note.Attachment); err == nil {
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
		return FailedToMarshalStatus(err)
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
		return FailedToMarshalNotification(err)
	}

	// Use the username from the notification (recipient)
	username := notification.Username
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return NotificationMissingUsername()
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

	if err := common.ValidateRequiredParam("accountID", accountID); err != nil {
		return AccountMissingID()
	}

	// Create proper account payload for streaming
	accountPayload, err := createAccountPayload(accountID, record.EventName)
	if err != nil {
		return FailedToCreateAccountPayload(err)
	}

	payload, err := json.Marshal(accountPayload)
	if err != nil {
		return FailedToMarshalAccount(err)
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
	// Extract username from account ID for follower lookup
	username := extractUsernameFromActorID(accountID)
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return CouldNotExtractUsername()
	}

	// Get the handler instance to access repositories
	if handler == nil {
		lambdaCtx.Logger.Error("handler not initialized, cannot broadcast to followers")
		return HandlerNotInitialized()
	}

	// Create a context for the operation
	ctx := &lift.Context{}
	ctx.SetRequestID(fmt.Sprintf("broadcast-%d", time.Now().Unix()))

	// Get followers for this account (limit to reasonable batch size for stream processing)
	followers, _, err := handler.getFollowersForUser(ctx, username, 200)
	if err != nil {
		lambdaCtx.Logger.Error("failed to get followers for broadcast",
			zap.String("account_id", accountID),
			zap.String("username", username),
			zap.Error(err))
		return FailedToGetFollowers(err)
	}

	if err := common.ValidateSliceNotEmpty("followers", followers); err != nil {
		lambdaCtx.Logger.Debug("no followers found for account",
			zap.String("account_id", accountID),
			zap.String("username", username))
		return nil
	}

	lambdaCtx.Logger.Info("broadcasting account update to followers",
		zap.String("account_id", accountID),
		zap.String("username", username),
		zap.Int("follower_count", len(followers)),
		zap.Int("payload_size", len(payload)))

	// Send to each follower's user stream
	var errs []error
	successCount := 0

	for _, followerUsername := range followers {
		streamName := fmt.Sprintf("user:%s", followerUsername)

		// Create account update message for the follower's stream
		message := StreamMessage{
			Event:   "account.update",
			Payload: payload,
			Stream:  streamName,
		}

		if err := handler.broadcastMessage(ctx, message); err != nil {
			lambdaCtx.Logger.Warn("failed to broadcast to follower",
				zap.String("account_id", accountID),
				zap.String("follower", followerUsername),
				zap.String("stream", streamName),
				zap.Error(err))
			errs = append(errs, err)
		} else {
			successCount++
		}
	}

	lambdaCtx.Logger.Info("completed follower broadcast",
		zap.String("account_id", accountID),
		zap.Int("total_followers", len(followers)),
		zap.Int("successful", successCount),
		zap.Int("failed", len(errs)))

	// Return error only if all broadcasts failed
	if len(errs) == len(followers) {
		if err := common.ValidateSliceNotEmpty("errs", errs); err == nil {
			return BroadcastToAllFollowersFailed()
		}
	}

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
		return UnknownEventName()
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
	if err := common.ValidateSliceNotEmpty("status.Hashtags", status.Hashtags); err == nil {
		for i, hashtag := range status.Hashtags {
			event.WithMetadata(fmt.Sprintf("hashtag_%d", i), hashtag)
		}
	}

	// Publish to the internal event bus
	if err := h.eventBus.Publish(event); err != nil {
		return FailedToPublishToInternalEventBus(err)
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
		return FailedToPublishToInternalEventBus(err)
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
		return UnknownEventName()
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
		return FailedToPublishToInternalEventBus(err)
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
		return FailedToGetSubscriptionsForStream(err)
	}

	if err := common.ValidateSliceNotEmpty("subscriptions", subscriptions); err != nil {
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
		return FailedToMarshalMessage(err)
	}

	// Send to all subscribed connections
	var errs []error
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
			errs = append(errs, err)

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
		zap.Int("failed", len(errs)))

	if err := common.ValidateSliceNotEmpty("errs", errs); err == nil {
		return SendToAllConnectionsFailed()
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
	if err := common.ValidateSliceNotEmpty("parts", parts); err == nil {
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
	h.logger.Debug("getting subscriptions for stream",
		zap.String("request_id", ctx.GetRequestID()),
		zap.String("stream", streamName))

	// Use the StreamingConnectionRepository to get subscriptions for the stream
	subscriptions, err := h.streamingRepo.GetSubscriptionsForStream(ctx, streamName)
	if err != nil {
		h.logger.Error("failed to get subscriptions from repository",
			zap.String("request_id", ctx.GetRequestID()),
			zap.String("stream", streamName),
			zap.Error(err))
		return nil, FailedToGetSubscriptions(err)
	}

	// Extract connection IDs from subscription objects
	connectionIDs := make([]string, 0, len(subscriptions))
	for _, subscription := range subscriptions {
		if subscription.ConnectionID != "" {
			connectionIDs = append(connectionIDs, subscription.ConnectionID)
		}
	}

	h.logger.Debug("found subscriptions for stream",
		zap.String("request_id", ctx.GetRequestID()),
		zap.String("stream", streamName),
		zap.Int("subscription_count", len(subscriptions)),
		zap.Int("connection_count", len(connectionIDs)))

	return connectionIDs, nil
}

// removeSubscription removes a stale subscription
func (h *StreamRouterHandler) removeSubscription(ctx *lift.Context, streamName, connectionID string) {
	h.logger.Info("removing stale subscription",
		zap.String("request_id", ctx.GetRequestID()),
		zap.String("stream", streamName),
		zap.String("connection_id", connectionID))

	// Remove the subscription from the database using the streaming repository
	err := h.streamingRepo.DeleteSubscription(ctx, connectionID, streamName)
	if err != nil {
		h.logger.Error("failed to remove subscription",
			zap.String("request_id", ctx.GetRequestID()),
			zap.String("stream", streamName),
			zap.String("connection_id", connectionID),
			zap.Error(err))
		return
	}

	h.logger.Info("successfully removed stale subscription",
		zap.String("request_id", ctx.GetRequestID()),
		zap.String("stream", streamName),
		zap.String("connection_id", connectionID))

	// Also try to remove the connection entirely if it's stale
	err = h.streamingRepo.DeleteConnection(ctx, connectionID)
	if err != nil {
		h.logger.Warn("failed to remove stale connection, but subscription was removed",
			zap.String("request_id", ctx.GetRequestID()),
			zap.String("connection_id", connectionID),
			zap.Error(err))
		// This is not critical - the subscription removal is what matters
	}
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
		return FailedToGetFollowers(err)
	}

	// Create deletion message for home timelines
	deletionMessage := StreamMessage{
		Event:   "delete",
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

// removeFromHashtagStreams removes objects from hashtag streams when content is deleted
func (h *StreamRouterHandler) removeFromHashtagStreams(ctx *lift.Context, objectID string) error {
	logger := h.logger.With(zap.String("object_id", objectID))

	// Step 1: Get the status content to extract hashtags
	status, err := h.statusRepo.GetStatus(ctx, objectID)
	if err != nil {
		// Status not found is not an error for deletion - it may already be cleaned up
		if strings.Contains(err.Error(), "not found") {
			logger.Debug("status not found for hashtag removal, may already be deleted")
			return nil
		}
		logger.Error("failed to get status for hashtag extraction", zap.Error(err))
		return FailedToGetStatusForHashtagExtraction(err)
	}

	// Step 2: Extract hashtags from status content
	var hashtags []string
	if status.Content != "" {
		hashtags = mastodon.ExtractHashtags(status.Content)
	}

	if err := common.ValidateSliceNotEmpty("hashtags", hashtags); err != nil {
		logger.Debug("no hashtags found in deleted object")
		return nil
	}

	// Step 3: Create deletion event for hashtag streams
	deletionEvent := &streaming.Event{
		Type: streaming.StatusDeleted,
		Payload: map[string]interface{}{
			"id":         objectID,
			"deleted_at": time.Now().Format(time.RFC3339),
		},
		Timestamp: time.Now(),
	}

	// Step 4: Send deletion event to each hashtag stream
	var errs []error
	for _, hashtag := range hashtags {
		streamName := streaming.HashtagStreamName(hashtag)

		if err := h.publisher.PublishToStream(ctx, streamName, deletionEvent); err != nil {
			logger.Warn("failed to remove object from hashtag stream",
				zap.String("hashtag", hashtag),
				zap.String("stream", streamName),
				zap.Error(err))
			errs = append(errs, HashtagProcessingFailed(err))
		} else {
			logger.Debug("successfully removed object from hashtag stream",
				zap.String("hashtag", hashtag),
				zap.String("stream", streamName))
		}
	}

	// Return combined errors if any occurred, but don't fail the entire operation
	if err := common.ValidateSliceNotEmpty("errs", errs); err == nil {
		logger.Warn("some hashtag stream removals failed",
			zap.Int("failed_count", len(errs)),
			zap.Int("total_hashtags", len(hashtags)))
		// Don't return error to avoid failing the tombstone processing
		// Hashtag stream cleanup is not critical for data consistency
	}

	logger.Info("completed hashtag stream removal",
		zap.Strings("hashtags", hashtags),
		zap.Int("streams_updated", len(hashtags)-len(errs)))

	return nil
}

// getFollowersForUser gets a limited list of followers for an actor
func (h *StreamRouterHandler) getFollowersForUser(ctx *lift.Context, username string, limit int) ([]string, string, error) {
	logger := h.logger.With(
		zap.String("username", username),
		zap.Int("limit", limit))

	// Validate inputs
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return nil, "", UsernameCannotBeEmpty()
	}

	if limit <= 0 || limit > 500 {
		limit = 100 // Default reasonable limit
	}

	// Call the AccountRepository GetFollowers method
	actors, nextCursor, err := h.accountRepo.GetFollowers(ctx, username, limit, "")
	if err != nil {
		logger.Error("failed to get followers from account repository", zap.Error(err))
		return nil, "", FailedToGetFollowers(err)
	}

	if err := common.ValidateSliceNotEmpty("actors", actors); err != nil {
		logger.Debug("no followers found for user")
		return []string{}, "", nil
	}

	// Convert Actor objects to username strings
	followerUsernames := make([]string, 0, len(actors))
	for _, actor := range actors {
		if actor == nil {
			logger.Warn("encountered nil actor in followers list")
			continue
		}

		// Extract username from actor ID or preferredUsername
		username := h.extractUsernameFromActorID(actor.ID)
		if err := common.ValidateRequiredParam("username", username); err != nil && actor.PreferredUsername != "" {
			username = actor.PreferredUsername
		}

		if username != "" {
			followerUsernames = append(followerUsernames, username)
		} else {
			logger.Warn("could not extract username from actor",
				zap.String("actor_id", actor.ID),
				zap.String("preferred_username", actor.PreferredUsername))
		}
	}

	logger.Info("successfully retrieved followers",
		zap.Int("total_actors", len(actors)),
		zap.Int("valid_usernames", len(followerUsernames)),
		zap.String("next_cursor", nextCursor))

	return followerUsernames, nextCursor, nil
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
		return FailedToMarshalMessage(err)
	}

	// Use existing broadcast method
	return h.broadcastToStream(ctx, message.Stream, message.Event, payload)
}

func main() {
	// Use the Lift pattern for DynamoDB streams with proper middleware
	patterns.StartDynamoDBStreamLambda("stream-router", handler, lambdaCtx.Logger)
}
