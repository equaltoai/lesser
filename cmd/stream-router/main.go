// Package main implements the stream-router Lambda function for routing streaming events to WebSocket connections.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/lift/pkg/streamer"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/mastodon"
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
// Events are routed exclusively via DynamoDB streams → API Gateway WebSocket connections
type StreamRouterHandler struct {
	db                 core.DB
	tableName          string
	logger             *zap.Logger
	apiClient          streamer.Client
	subscriptionsTable string
	wsEndpoint         string
	userRepo           *repositories.UserRepository
	actorRepo          *repositories.ActorRepository
	accountRepo        followerActorRepository
	statusRepo         statusRepository
	publisher          streaming.Publisher
	streamingRepo      streamConnectionRepository
	domain             string
	streamEventLog     *streaming.StreamEventLog
}

type followerActorRepository interface {
	GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]*activitypub.Actor, string, error)
}

type statusRepository interface {
	GetStatus(ctx context.Context, statusID string) (*models.Status, error)
}

type streamConnectionRepository interface {
	GetConnection(ctx context.Context, connectionID string) (*models.WebSocketConnection, error)
	GetConnectionsByUser(ctx context.Context, userID string) ([]models.WebSocketConnection, error)
	GetSubscriptionsForStream(ctx context.Context, stream string) ([]models.WebSocketSubscription, error)
	DeleteSubscription(ctx context.Context, connectionID, stream string) error
	DeleteConnection(ctx context.Context, connectionID string) error
}

// connectionRepositoryAdapter adapts StreamingConnectionRepository to streaming.ConnectionRepository
type connectionRepositoryAdapter struct {
	streamingRepo streamConnectionRepository
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
	conn, err := a.streamingRepo.GetConnection(ctx, connectionID)
	if err != nil {
		if errors.HasCode(err, errors.CodeNotFound) {
			return nil, ConnectionNotFound()
		}
		return nil, FailedToQueryConnection(err)
	}
	if conn == nil {
		return nil, ConnectionNotFound()
	}

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

	newLambdaOptimizedClient = dynamorm.NewLambdaOptimizedClient
	newStreamerClient        = func(ctx context.Context, cfg streamer.ClientConfig) (streamer.Client, error) {
		return streamer.NewClient(ctx, cfg)
	}
	startLambda = lambda.Start
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

	// Ensure we have a Dynamo client even if the default bootstrap path failed
	if lambdaCtx.DynamoDB == nil {
		if manualErr := initializeManualServices(); manualErr != nil {
			lambdaCtx.Logger.Fatal("failed to initialize manual services", zap.Error(manualErr))
		}
	}

	// Stream router-specific initialization
	var initErr error
	handler, initErr = NewStreamRouterHandler()
	if initErr != nil {
		lambdaCtx.Logger.Fatal("failed to create stream router handler", zap.Error(initErr))
	}
}

func initializeManualServices() error {
	if lambdaCtx == nil {
		return fmt.Errorf("lambda context not initialized")
	}

	if lambdaCtx.Config == nil {
		lambdaCtx.Config = config.Get()
	}

	cfg := lambdaCtx.Config

	region := cfg.Region
	if region == "" {
		region = os.Getenv("AWS_REGION")
		if region == "" {
			region = "us-east-1"
		}
		cfg.Region = region
	}

	client, err := newLambdaOptimizedClient(context.Background(), region)
	if err != nil {
		lambdaCtx.Logger.Error("failed to initialize dynamo client manually",
			zap.String("region", region),
			zap.Error(err))
		return err
	}

	lambdaCtx.DynamoDB = client

	if cfg.DynamoTableName == "" {
		if table := os.Getenv("DYNAMO_TABLE_NAME"); table != "" {
			cfg.DynamoTableName = table
		}
	}

	if cfg.SubscriptionsTable == "" {
		if table := os.Getenv("STREAMING_SUBSCRIPTIONS_TABLE"); table != "" {
			cfg.SubscriptionsTable = table
		}
	}

	if cfg.WebSocketEndpoint == "" {
		if endpoint := os.Getenv("WEBSOCKET_API_URL"); endpoint != "" {
			cfg.WebSocketEndpoint = endpoint
		} else if endpoint := os.Getenv("WEBSOCKET_ENDPOINT"); endpoint != "" {
			cfg.WebSocketEndpoint = endpoint
		}
	}

	if strings.HasPrefix(cfg.WebSocketEndpoint, "wss://") {
		cfg.WebSocketEndpoint = "https://" + strings.TrimPrefix(cfg.WebSocketEndpoint, "wss://")
	}

	if cfg.WebSocketEndpoint == "" {
		if apiID := os.Getenv("WEBSOCKET_API_ID"); apiID != "" {
			stage := os.Getenv("WEBSOCKET_STAGE")
			if stage == "" {
				stage = "development"
			}
			cfg.WebSocketEndpoint = fmt.Sprintf("https://%s.execute-api.%s.amazonaws.com/%s", apiID, cfg.Region, stage)
		}
	}

	if cfg.WebSocketEndpoint == "" && cfg.Domain != "" {
		host := strings.TrimPrefix(strings.TrimPrefix(cfg.Domain, "https://"), "http://")
		if host != "" {
			cfg.WebSocketEndpoint = fmt.Sprintf("https://ws.%s", host)
		}
	}

	if cfg.Domain == "" {
		cfg.Domain = os.Getenv("DOMAIN_NAME")
	}

	lambdaCtx.Logger.Info("manual services initialized for stream-router",
		zap.String("table_name", cfg.DynamoTableName),
		zap.String("region", cfg.Region))

	return nil
}

// NewStreamRouterHandler creates a new stream router handler with DynamORM
func NewStreamRouterHandler() (*StreamRouterHandler, error) {
	// Get config from the initialized lambda context
	globalCfg := lambdaCtx.AWSServices.Config

	// Use the standardized database connection
	var db core.DB
	if lambdaCtx.DynamoDB != nil {
		if dynamo, ok := lambdaCtx.DynamoDB.(core.DB); ok && dynamo != nil {
			db = dynamo
		}
	}

	if db == nil {
		if manualErr := initializeManualServices(); manualErr != nil {
			return nil, fmt.Errorf("failed to initialize DynamoDB client: %w", manualErr)
		}
		if dynamo, ok := lambdaCtx.DynamoDB.(core.DB); ok && dynamo != nil {
			db = dynamo
		}
	}

	if db == nil {
		return nil, fmt.Errorf("DynamoDB client unavailable for stream router")
	}
	tableName := lambdaCtx.Config.DynamoTableName

	// Get configuration values
	subscriptionsTable := lambdaCtx.Config.SubscriptionsTable
	if err := common.ValidateRequiredParam("subscriptionsTable", subscriptionsTable); err != nil {
		// Default to primary table so subscriptions live alongside connections
		subscriptionsTable = tableName
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

	apiClient, err := newStreamerClient(context.Background(), streamer.ClientConfig{
		AWSConfig: &globalCfg,
		Endpoint:  wsEndpoint,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create WebSocket client: %w", err)
	}

	// Initialize streaming repository
	streamingRepo := repositories.NewStreamingConnectionRepository(db, tableName, db, subscriptionsTable, lambdaCtx.Logger, nil)

	// Create connection repository adapter
	connRepoAdapter := &connectionRepositoryAdapter{
		streamingRepo: streamingRepo,
		logger:        lambdaCtx.Logger,
	}

	// Initialize publisher for WebSocket delivery via API Gateway
	publisher := streaming.NewAPIGatewayPublisher(apiClient, connRepoAdapter, wsEndpoint, lambdaCtx.Logger)

	// Initialize stream event log for SSE fanout (optional).
	var streamEventLog *streaming.StreamEventLog
	if table := lambdaCtx.Config.StreamEventsTable; table != "" {
		streamEventLog = streaming.NewStreamEventLog(db, 30*time.Minute)
	}

	lambdaCtx.Logger.Info("stream router initialized with DynamoDB-backed routing")

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
		publisher:          publisher,
		streamingRepo:      streamingRepo,
		domain:             domain,
		streamEventLog:     streamEventLog,
	}, nil
}

func (h *StreamRouterHandler) HandleDynamoDBRecord(ctx *apptheory.EventContext, record events.DynamoDBEventRecord) error {
	requestID := ""
	runCtx := context.Background()
	if ctx != nil {
		requestID = strings.TrimSpace(ctx.RequestID)
		runCtx = ctx.Context()
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		requestID = "unknown"
	}

	if err := h.processRecord(runCtx, requestID, record); err != nil {
		h.logger.Error("failed to process record",
			zap.String("request_id", requestID),
			zap.String("event_id", record.EventID),
			zap.Error(err),
		)
		// Match previous Lift behavior: log and continue; do not fail the batch.
		return nil
	}

	return nil
}

// processRecord processes a single DynamoDB stream record using DynamORM patterns
func (h *StreamRouterHandler) processRecord(ctx context.Context, requestID string, record events.DynamoDBEventRecord) error {
	logger := h.logger.With(
		zap.String("request_id", requestID),
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
		return h.processStatusEvent(ctx, requestID, record)
	case "NOTIFICATION":
		return h.processNotificationEvent(ctx, requestID, record)
	case "USER", "ACTOR":
		return h.processAccountEvent(ctx, requestID, record)
	case "TOMBSTONE":
		return h.processTombstoneEvent(ctx, requestID, record)
	default:
		logger.Debug("ignoring event for unknown entity type")
		return nil
	}
}

// processStatusEvent processes status/object events using DynamORM stream utilities
func (h *StreamRouterHandler) processStatusEvent(ctx context.Context, requestID string, record events.DynamoDBEventRecord) error {
	// Use DynamORM stream utilities to unmarshal the status
	var status models.Status
	if err := stream.UnmarshalItem(record, &status); err != nil {
		h.logger.Debug("failed to unmarshal status from stream",
			zap.String("request_id", requestID),
			zap.Error(err))
		return nil // Skip records we can't unmarshal
	}

	// Validate that we have a proper status
	if err := common.ValidateRequiredParam("statusID", status.StatusID); err != nil || status.Note == nil {
		h.logger.Debug("invalid status record, missing required fields",
			zap.String("request_id", requestID))
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
	wsStreams := []string{}

	// Public timelines
	if status.Visibility == "public" {
		wsStreams = append(wsStreams, streaming.PublicStream, streaming.PublicLocalStream)
	}

	// User stream for the author
	if status.AuthorUsername != "" {
		wsStreams = append(wsStreams, streaming.UserStreamName(status.AuthorUsername))
	}

	// Send to all relevant streams
	eventType := streamEventUpdate
	if record.EventName == eventNameModify {
		eventType = streamEventStatusUpdate
	}

	// Record for SSE fanout (independent of WebSocket subscriptions).
	sseStreams := h.buildSSEStatusStreams(ctx, requestID, &status)
	for _, streamName := range sseStreams {
		h.appendStreamEvent(ctx, requestID, streamName, eventType, string(payload))
	}

	for _, streamName := range wsStreams {
		if err := h.broadcastToStream(ctx, requestID, streamName, eventType, payload); err != nil {
			h.logger.Error("failed to broadcast to stream",
				zap.String("request_id", requestID),
				zap.String("stream", streamName),
				zap.Error(err))
			// Continue with other streams
		}
	}

	// Publish to internal event bus for GraphQL subscriptions
	if err := h.publishStatusEventToInternalBus(requestID, record, &status, wsStreams); err != nil {
		h.logger.Warn("failed to publish status event to internal bus",
			zap.String("request_id", requestID),
			zap.String("status_id", status.StatusID),
			zap.Error(err))
		// Don't return error - this is not critical for WebSocket functionality
	}

	return nil
}

// processNotificationEvent processes notification events using DynamORM stream utilities
func (h *StreamRouterHandler) processNotificationEvent(ctx context.Context, requestID string, record events.DynamoDBEventRecord) error {
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
			zap.String("request_id", requestID),
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
	streamName := streaming.UserNotificationStreamName(username)

	// Record for SSE fanout (Mastodon user stream includes notifications too).
	h.appendStreamEvent(ctx, requestID, streaming.UserStreamName(username), streamEventNotification, string(payload))
	h.appendStreamEvent(ctx, requestID, streamName, streamEventNotification, string(payload))

	if err := h.broadcastToStream(ctx, requestID, streamName, streamEventNotification, payload); err != nil {
		h.logger.Error("failed to broadcast notification to stream",
			zap.String("request_id", requestID),
			zap.String("stream", streamName),
			zap.Error(err))
		// Continue with internal event bus
	}

	// Publish to internal event bus for GraphQL subscriptions
	if err := h.publishNotificationEventToInternalBus(requestID, notification, []string{streamName}); err != nil {
		h.logger.Warn("failed to publish notification event to internal bus",
			zap.String("request_id", requestID),
			zap.String("notification_id", notification.ID),
			zap.Error(err))
		// Don't return error - this is not critical for WebSocket functionality
	}

	return nil
}

// processAccountEvent processes account/user events using DynamORM stream utilities
func (h *StreamRouterHandler) processAccountEvent(ctx context.Context, requestID string, record events.DynamoDBEventRecord) error {
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
	if err := h.broadcastToFollowers(ctx, requestID, accountID, payload); err != nil {
		h.logger.Error("failed to broadcast to followers",
			zap.String("request_id", requestID),
			zap.Error(err))
		// Don't return error - logging is sufficient for broadcast failures
	}

	// Publish to internal event bus for GraphQL subscriptions
	streams := []string{fmt.Sprintf("account:%s", accountID)}
	if err := h.publishAccountEventToInternalBus(requestID, accountID, record.EventName, streams); err != nil {
		h.logger.Warn("failed to publish account event to internal bus",
			zap.String("request_id", requestID),
			zap.String("account_id", accountID),
			zap.Error(err))
		// Don't return error - this is not critical for WebSocket functionality
	}

	h.logger.Info("account update event",
		zap.String("request_id", requestID),
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
func (h *StreamRouterHandler) broadcastToFollowers(ctx context.Context, requestID, accountID string, payload []byte) error {
	// Extract username from account ID for follower lookup
	username := extractUsernameFromActorID(accountID)
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return CouldNotExtractUsername()
	}

	// Get followers for this account (limit to reasonable batch size for stream processing)
	followers, _, err := h.getFollowersForUser(ctx, requestID, username, 200)
	if err != nil {
		h.logger.Error("failed to get followers for broadcast",
			zap.String("request_id", requestID),
			zap.String("account_id", accountID),
			zap.String("username", username),
			zap.Error(err))
		return FailedToGetFollowers(err)
	}

	if err := common.ValidateSliceNotEmpty("followers", followers); err != nil {
		h.logger.Debug("no followers found for account",
			zap.String("request_id", requestID),
			zap.String("account_id", accountID),
			zap.String("username", username))
		return nil
	}

	h.logger.Info("broadcasting account update to followers",
		zap.String("request_id", requestID),
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

		if err := h.broadcastMessage(ctx, requestID, message); err != nil {
			h.logger.Warn("failed to broadcast to follower",
				zap.String("request_id", requestID),
				zap.String("account_id", accountID),
				zap.String("follower", followerUsername),
				zap.String("stream", streamName),
				zap.Error(err))
			errs = append(errs, err)
		} else {
			successCount++
		}
	}

	h.logger.Info("completed follower broadcast",
		zap.String("request_id", requestID),
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
func (h *StreamRouterHandler) publishStatusEventToInternalBus(requestID string, record events.DynamoDBEventRecord, status *models.Status, streams []string) error {
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
	event.WithMetadata("request_id", requestID)

	if status.Language != "" {
		event.WithMetadata("language", status.Language)
	}

	// Add hashtags to metadata for filtering
	if err := common.ValidateSliceNotEmpty("status.Hashtags", status.Hashtags); err == nil {
		for i, hashtag := range status.Hashtags {
			event.WithMetadata(fmt.Sprintf("hashtag_%d", i), hashtag)
		}
	}

	// Note: Event routing to WebSocket connections happens via the publisher
	// which is called by routeEventToWebSockets() in the main routing logic
	h.logger.Debug("status event ready for routing",
		zap.String("request_id", requestID),
		zap.String("event_id", event.ID),
		zap.String("status_id", status.StatusID),
		zap.String("event_type", string(eventType)),
		zap.Strings("streams", streams))

	return nil
}

// publishNotificationEventToInternalBus publishes a notification event to the internal event bus
func (h *StreamRouterHandler) publishNotificationEventToInternalBus(requestID string, notification *Notification, streams []string) error {
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
	event.WithMetadata("request_id", requestID)

	if notification.StatusID != "" {
		event.WithMetadata("status_id", notification.StatusID)
	}

	// Note: Event routing to WebSocket connections happens via the publisher
	// which is called by routeEventToWebSockets() in the main routing logic
	h.logger.Debug("notification event ready for routing",
		zap.String("request_id", requestID),
		zap.String("event_id", event.ID),
		zap.String("notification_id", notification.ID),
		zap.String("notification_type", notification.Type),
		zap.Strings("streams", streams))

	return nil
}

// publishAccountEventToInternalBus publishes an account event to the internal event bus
func (h *StreamRouterHandler) publishAccountEventToInternalBus(requestID, accountID, eventName string, streams []string) error {
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
	event.WithMetadata("request_id", requestID)

	// Note: Event routing to WebSocket connections happens via the publisher
	// which is called by routeEventToWebSockets() in the main routing logic
	h.logger.Debug("account event ready for routing",
		zap.String("request_id", requestID),
		zap.String("event_id", event.ID),
		zap.String("account_id", accountID),
		zap.String("event_type", string(eventType)),
		zap.Strings("streams", streams))

	return nil
}

// broadcastToStream sends a message to a specific stream
func (h *StreamRouterHandler) broadcastToStream(ctx context.Context, requestID, streamName, eventType string, payload []byte) error {
	// Query subscriptions for this stream from DynamoDB
	subscriptions, err := h.getStreamSubscriptions(ctx, requestID, streamName)
	if err != nil {
		return FailedToGetSubscriptionsForStream(err)
	}

	if err := common.ValidateSliceNotEmpty("subscriptions", subscriptions); err != nil {
		h.logger.Debug("no active subscriptions for stream",
			zap.String("request_id", requestID),
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
		err := h.apiClient.PostToConnection(ctx, connectionID, messageData)
		if err != nil {
			h.logger.Warn("failed to send to connection",
				zap.String("request_id", requestID),
				zap.String("connection_id", connectionID),
				zap.String("stream", streamName),
				zap.Error(err))
			errs = append(errs, err)

			// Remove stale connections
			if isStaleConnection(err) {
				h.removeSubscription(ctx, requestID, streamName, connectionID)
			}
		} else {
			successCount++
		}
	}

	h.logger.Info("broadcast completed",
		zap.String("request_id", requestID),
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

func (h *StreamRouterHandler) appendStreamEvent(ctx context.Context, requestID, streamName, eventType, data string) {
	if h.streamEventLog == nil || !h.streamEventLog.Enabled() {
		return
	}
	if streamName == "" || eventType == "" {
		return
	}

	if _, err := h.streamEventLog.Append(ctx, streamName, eventType, data); err != nil {
		h.logger.Warn("failed to append stream event",
			zap.String("request_id", requestID),
			zap.String("stream", streamName),
			zap.String("event", eventType),
			zap.Error(err))
	}
}

func (h *StreamRouterHandler) buildSSEStatusStreams(ctx context.Context, requestID string, status *models.Status) []string {
	if status == nil {
		return []string{}
	}

	streams := make(map[string]struct{})
	add := func(name string) {
		if name == "" {
			return
		}
		streams[name] = struct{}{}
	}

	// Author always receives their own events.
	if status.AuthorUsername != "" {
		add(streaming.UserStreamName(status.AuthorUsername))
	}

	// Public + hashtag streams only apply to public visibility.
	isLocalAuthor := h.isLocalActorID(status.AuthorID)
	if status.Visibility == models.VisibilityPublic {
		add(streaming.PublicStream)
		if isLocalAuthor {
			add(streaming.PublicLocalStream)
		} else {
			add(streaming.PublicRemoteStream)
		}

		for _, tag := range status.Hashtags {
			add(streaming.HashtagStreamName(tag))
			if isLocalAuthor {
				add(localHashtagStreamName(tag))
			}
		}
	}

	// Home timeline fanout: deliver to local followers (and to direct recipients when applicable).
	if status.AuthorUsername != "" {
		if status.Visibility == models.VisibilityDirect {
			for _, username := range h.localRecipients(status) {
				add(streaming.DirectStreamName(username))
				add(streaming.UserStreamName(username))
			}
		} else {
			followers, _, err := h.getFollowersForUser(ctx, requestID, status.AuthorUsername, 100)
			if err != nil {
				h.logger.Warn("failed to get followers for status fanout",
					zap.String("request_id", requestID),
					zap.String("username", status.AuthorUsername),
					zap.Error(err))
			} else {
				for _, follower := range followers {
					add(streaming.UserStreamName(follower))
				}
			}
		}
	}

	result := make([]string, 0, len(streams))
	for name := range streams {
		result = append(result, name)
	}

	return result
}

func (h *StreamRouterHandler) isLocalActorID(actorID string) bool {
	if actorID == "" || h.domain == "" {
		return false
	}
	u, err := url.Parse(actorID)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if host == "" {
		host = u.Host
	}
	return strings.EqualFold(host, h.domain)
}

func (h *StreamRouterHandler) localRecipients(status *models.Status) []string {
	if status == nil {
		return []string{}
	}

	usernames := make(map[string]struct{})
	for _, list := range [][]string{
		status.ToRecipients,
		status.CcRecipients,
		status.BtoRecipients,
		status.BccRecipients,
	} {
		for _, raw := range list {
			if username := h.extractLocalUsername(raw); username != "" {
				usernames[username] = struct{}{}
			}
		}
	}

	result := make([]string, 0, len(usernames))
	for username := range usernames {
		result = append(result, username)
	}

	return result
}

func (h *StreamRouterHandler) extractLocalUsername(raw string) string {
	if raw == "" || h.domain == "" {
		return ""
	}

	acct := strings.TrimPrefix(raw, "acct:")
	if parts := strings.SplitN(acct, "@", 2); len(parts) == 2 {
		if strings.EqualFold(parts[1], h.domain) {
			return parts[0]
		}
		return ""
	}

	if strings.HasPrefix(raw, "https://") || strings.HasPrefix(raw, "http://") {
		u, err := url.Parse(raw)
		if err != nil {
			return ""
		}
		if !strings.EqualFold(u.Hostname(), h.domain) {
			return ""
		}
		pathParts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(pathParts) == 0 {
			return ""
		}
		return pathParts[len(pathParts)-1]
	}

	return ""
}

func localHashtagStreamName(hashtag string) string {
	if hashtag == "" {
		return ""
	}
	return fmt.Sprintf("hashtag:local:%s", hashtag)
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

// getStreamSubscriptions retrieves active subscriptions for a stream
func (h *StreamRouterHandler) getStreamSubscriptions(ctx context.Context, requestID, streamName string) ([]string, error) {
	h.logger.Debug("getting subscriptions for stream",
		zap.String("request_id", requestID),
		zap.String("stream", streamName))

	// Use the StreamingConnectionRepository to get subscriptions for the stream
	subscriptions, err := h.streamingRepo.GetSubscriptionsForStream(ctx, streamName)
	if err != nil {
		h.logger.Error("failed to get subscriptions from repository",
			zap.String("request_id", requestID),
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
		zap.String("request_id", requestID),
		zap.String("stream", streamName),
		zap.Int("subscription_count", len(subscriptions)),
		zap.Int("connection_count", len(connectionIDs)))

	return connectionIDs, nil
}

// removeSubscription removes a stale subscription
func (h *StreamRouterHandler) removeSubscription(ctx context.Context, requestID, streamName, connectionID string) {
	h.logger.Info("removing stale subscription",
		zap.String("request_id", requestID),
		zap.String("stream", streamName),
		zap.String("connection_id", connectionID))

	// Remove the subscription from the database using the streaming repository
	err := h.streamingRepo.DeleteSubscription(ctx, connectionID, streamName)
	if err != nil {
		h.logger.Error("failed to remove subscription",
			zap.String("request_id", requestID),
			zap.String("stream", streamName),
			zap.String("connection_id", connectionID),
			zap.Error(err))
		return
	}

	h.logger.Info("successfully removed stale subscription",
		zap.String("request_id", requestID),
		zap.String("stream", streamName),
		zap.String("connection_id", connectionID))

	// Also try to remove the connection entirely if it's stale
	err = h.streamingRepo.DeleteConnection(ctx, connectionID)
	if err != nil {
		h.logger.Warn("failed to remove stale connection, but subscription was removed",
			zap.String("request_id", requestID),
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
func (h *StreamRouterHandler) processTombstoneEvent(ctx context.Context, requestID string, record events.DynamoDBEventRecord) error {
	logger := h.logger.With(
		zap.String("request_id", requestID),
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
	if err := h.broadcastDeletionToStreams(ctx, requestID, &tombstone, deletionMessage); err != nil {
		logger.Error("failed to broadcast deletion to streams", zap.Error(err))
		// Don't return error - this is not critical
	}

	// Remove the object from user timelines (followers of the deleter)
	if err := h.removeFromFollowerTimelines(ctx, requestID, tombstone.DeletedBy, tombstone.ID); err != nil {
		logger.Warn("failed to remove from follower timelines", zap.Error(err))
	}

	logger.Info("successfully processed tombstone event")
	return nil
}

// broadcastDeletionToStreams sends deletion events to appropriate streams
func (h *StreamRouterHandler) broadcastDeletionToStreams(ctx context.Context, requestID string, tombstone *models.Tombstone, message StreamMessage) error {
	objectID := tombstone.ID

	// Based on the former object type, determine which streams to update
	switch tombstone.FormerType {
	case "Note", "Article", "Status":
		// Public timeline
		message.Stream = "public"
		if err := h.broadcastMessage(ctx, requestID, message); err != nil {
			h.logger.Warn("failed to broadcast deletion to public stream", zap.Error(err))
		}
		h.appendStreamEvent(ctx, requestID, streaming.PublicStream, "delete", objectID)

		// Local timeline (if it was a local object)
		message.Stream = "public:local"
		if err := h.broadcastMessage(ctx, requestID, message); err != nil {
			h.logger.Warn("failed to broadcast deletion to local stream", zap.Error(err))
		}
		h.appendStreamEvent(ctx, requestID, streaming.PublicLocalStream, "delete", objectID)

		// Hashtag streams - extract hashtags from the deleted object if available
		if err := h.removeFromHashtagStreams(ctx, requestID, objectID); err != nil {
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
func (h *StreamRouterHandler) removeFromFollowerTimelines(ctx context.Context, requestID, actorID, objectID string) error {
	// Extract username from actor ID
	username := h.extractUsernameFromActorID(actorID)

	// Get followers (limited batch for stream processing efficiency)
	followers, _, err := h.getFollowersForUser(ctx, requestID, username, 100)
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
		streamName := streaming.UserStreamName(follower)
		deletionMessage.Stream = streamName

		h.appendStreamEvent(ctx, requestID, streamName, "delete", objectID)

		if err := h.broadcastMessage(ctx, requestID, deletionMessage); err != nil {
			h.logger.Warn("failed to send deletion to follower timeline",
				zap.String("request_id", requestID),
				zap.String("follower", follower),
				zap.String("object_id", objectID),
				zap.Error(err))
			// Continue with other followers
		}
	}

	return nil
}

// removeFromHashtagStreams removes objects from hashtag streams when content is deleted
func (h *StreamRouterHandler) removeFromHashtagStreams(ctx context.Context, requestID, objectID string) error {
	logger := h.logger.With(
		zap.String("request_id", requestID),
		zap.String("object_id", objectID),
	)

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
	isLocalAuthor := h.isLocalActorID(status.AuthorID)

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

		h.appendStreamEvent(ctx, requestID, streamName, "delete", objectID)
		if isLocalAuthor {
			h.appendStreamEvent(ctx, requestID, localHashtagStreamName(hashtag), "delete", objectID)
		}

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
func (h *StreamRouterHandler) getFollowersForUser(ctx context.Context, requestID, username string, limit int) ([]string, string, error) {
	logger := h.logger.With(
		zap.String("request_id", requestID),
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

		// Stream fanout is only relevant for local users (remote followers use federation, not SSE).
		if actor.ID != "" && !h.isLocalActorID(actor.ID) {
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
func (h *StreamRouterHandler) broadcastMessage(ctx context.Context, requestID string, message StreamMessage) error {
	// Marshal the message
	payload, err := json.Marshal(message)
	if err != nil {
		return FailedToMarshalMessage(err)
	}

	// Use existing broadcast method
	return h.broadcastToStream(ctx, requestID, message.Stream, message.Event, payload)
}

func main() {
	app := apptheory.New()
	app.DynamoDB(lambdaCtx.Config.DynamoTableName, handleStreamRouterStreamRecord)

	startLambda(func(ctx context.Context, event json.RawMessage) (any, error) {
		return app.HandleLambda(ctx, event)
	})
}

func handleStreamRouterStreamRecord(ctx *apptheory.EventContext, record events.DynamoDBEventRecord) error {
	if handler == nil {
		return HandlerNotInitialized()
	}
	return handler.HandleDynamoDBRecord(ctx, record)
}
