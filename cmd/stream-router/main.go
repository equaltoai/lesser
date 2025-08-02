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
)

// StreamMessage represents a message sent over WebSocket
type StreamMessage struct {
	Event   string          `json:"event"`
	Payload json.RawMessage `json:"payload"`
	Stream  string          `json:"stream"`
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
	userRepo := repositories.NewUserRepository(db, tableName, logger)
	actorRepo := repositories.NewActorRepository(db, tableName, logger)
	statusRepo := repositories.NewStatusRepository(db, tableName, logger)

	// Initialize API Gateway Management API client
	apiClient := apigatewaymanagementapi.NewFromConfig(globalCfg, func(o *apigatewaymanagementapi.Options) {
		o.BaseEndpoint = aws.String(wsEndpoint)
	})

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
	if record.EventName != "INSERT" && record.EventName != "MODIFY" {
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
			"url":  fmt.Sprintf("https://example.com/tags/%s", hashtag), // TODO: Use proper domain
		})
	}

	// Process mentions from status model
	for _, mention := range status.Mentions {
		statusPayload["mentions"] = append(statusPayload["mentions"].([]any), map[string]any{
			"id":       mention,
			"username": mention,
			"acct":     mention,
			"url":      fmt.Sprintf("https://example.com/users/%s", mention), // TODO: Use proper domain
		})
	}

	// Process attachments from Note
	if note.Attachment != nil {
		// Skip attachment processing for now - complex type conversion needed
		// TODO: Implement proper attachment processing based on ActivityPub attachment structure
		h.logger.Debug("skipping attachment processing for now", 
			zap.String("request_id", ctx.GetRequestID()),
			zap.Int("attachment_count", len(note.Attachment)))
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
	eventType := "update"
	if record.EventName == "MODIFY" {
		eventType = "status.update"
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

	return nil
}

// processNotificationEvent processes notification events using DynamORM stream utilities
func (h *StreamRouterHandler) processNotificationEvent(ctx *lift.Context, record events.DynamoDBEventRecord) error {
	// Extract the notification from the DynamoDB image
	if record.EventName != "INSERT" {
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

	// Extract the notification
	type Notification struct {
		ID        string    `dynamodbav:"id"`
		Type      string    `dynamodbav:"type"`                // follow, mention, favourite, reblog
		Username  string    `dynamodbav:"username"`            // Recipient of the notification
		AccountID string    `dynamodbav:"account_id"`          // Who triggered the notification
		StatusID  string    `dynamodbav:"status_id,omitempty"` // Related status (if any)
		Read      bool      `dynamodbav:"read"`
		CreatedAt time.Time `dynamodbav:"created_at"`
	}

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
		notifPayload["status"] = map[string]any{
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
	return h.broadcastToStream(ctx, fmt.Sprintf("user:notification:%s", username), "notification", payload)
}

// processAccountEvent processes account/user events using DynamORM stream utilities  
func (h *StreamRouterHandler) processAccountEvent(ctx *lift.Context, record events.DynamoDBEventRecord) error {
	// Account updates (profile changes, etc.)
	if record.EventName != "MODIFY" {
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
	h.logger.Info("account update event",
		zap.String("request_id", ctx.GetRequestID()),
		zap.String("account_id", accountID),
		zap.Int("payload_size", len(payload)))

	return nil
}


func extractTableName(arn string) string {
	// Extract table name from DynamoDB stream ARN
	// Format: arn:aws:dynamodb:region:account:table/tablename/stream/timestamp
	parts := strings.Split(arn, "/")
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
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

// Helper function to check if a slice contains a value
func contains(slice []string, value string) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}
	return false
}

// Helper function to generate a random ID
func generateID(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}

// Helper function to determine attachment type from media type
func getAttachmentType(mediaType string) string {
	if strings.HasPrefix(mediaType, "video/") {
		return "video"
	} else if strings.HasPrefix(mediaType, "audio/") {
		return "audio"
	}
	return "image"
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

// generateID generates a random ID (method on handler)
func (h *StreamRouterHandler) generateID(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}

// getAttachmentType determines attachment type from a map
func (h *StreamRouterHandler) getAttachmentType(attMap map[string]any) string {
	if mediaType, ok := attMap["mediaType"].(string); ok {
		if strings.HasPrefix(mediaType, "video/") {
			return "video"
		} else if strings.HasPrefix(mediaType, "audio/") {
			return "audio"
		}
	}
	return "image"
}

// broadcastToStream sends a message to a specific stream
func (h *StreamRouterHandler) broadcastToStream(ctx *lift.Context, streamName, eventType string, payload []byte) error {
	// TODO: Implement actual broadcasting to stream using DynamoDB subscriptions table
	h.logger.Debug("broadcasting to stream",
		zap.String("request_id", ctx.GetRequestID()),
		zap.String("stream", streamName),
		zap.String("event", eventType),
		zap.Int("payload_size", len(payload)))
	return nil
}

func main() {
	// Use the Lift pattern for DynamoDB streams with proper middleware
	patterns.StartDynamoDBStreamLambda("stream-router", handler, logger)
}
