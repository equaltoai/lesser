package main

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
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/apigatewaymanagementapi"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/common"
)

// StreamMessage represents a message sent over WebSocket
type StreamMessage struct {
	Event   string          `json:"event"`
	Payload json.RawMessage `json:"payload"`
	Stream  string          `json:"stream"`
}

var (
	dynamoClient       *dynamodb.Client
	apiClient          *apigatewaymanagementapi.Client
	globalCfg          aws.Config
	subscriptionsTable string
	wsEndpoint         string
	log                *zap.Logger
)

func init() {
	// Initialize logger
	log = common.Logger()

	// Initialize AWS clients
	var err error
	globalCfg, err = config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatal("failed to load AWS config", zap.Error(err))
	}

	dynamoClient = dynamodb.NewFromConfig(globalCfg)

	// Get environment variables
	subscriptionsTable = os.Getenv("SUBSCRIPTIONS_TABLE")
	if subscriptionsTable == "" {
		subscriptionsTable = "lesser-streaming-subscriptions"
	}

	wsEndpoint = os.Getenv("WEBSOCKET_ENDPOINT")
	if wsEndpoint == "" {
		log.Fatal("WEBSOCKET_ENDPOINT environment variable not set")
	}
}

func handler(ctx context.Context, event events.DynamoDBEvent) error {
	for _, record := range event.Records {
		if err := processRecord(ctx, record); err != nil {
			log.Error("failed to process record", zap.Error(err))
			// Continue processing other records
		}
	}
	return nil
}

func processRecord(ctx context.Context, record events.DynamoDBEventRecord) error {
	log := log.With(
		zap.String("eventName", record.EventName),
		zap.String("tableName", extractTableName(record.EventSourceArn)),
	)

	// Only process INSERT and MODIFY events
	if record.EventName != "INSERT" && record.EventName != "MODIFY" {
		return nil
	}

	// Determine which table this event is from
	tableName := extractTableName(record.EventSourceArn)

	switch {
	case strings.Contains(tableName, "statuses"):
		return processStatusEvent(ctx, record)
	case strings.Contains(tableName, "notifications"):
		return processNotificationEvent(ctx, record)
	case strings.Contains(tableName, "accounts"):
		return processAccountEvent(ctx, record)
	default:
		log.Debug("ignoring event from table", zap.String("table", tableName))
		return nil
	}
}

func processStatusEvent(ctx context.Context, record events.DynamoDBEventRecord) error {
	// Extract the status from the DynamoDB image
	if record.EventName != "INSERT" && record.EventName != "MODIFY" {
		return nil
	}

	// Convert from events.DynamoDBAttributeValue to SDK v2 types
	statusItem := make(map[string]types.AttributeValue)
	for k, v := range record.Change.NewImage {
		statusItem[k] = convertEventAttributeValue(v)
	}

	// Check if this is an object record (status/note)
	pkAttr, hasPK := statusItem["PK"]
	if !hasPK {
		return nil
	}

	pk := ""
	if s, ok := pkAttr.(*types.AttributeValueMemberS); ok {
		pk = s.Value
	}

	// Only process OBJECT# records
	if !strings.HasPrefix(pk, "OBJECT#") {
		return nil
	}

	// Check if this is the metadata record
	skAttr, hasSK := statusItem["SK"]
	if !hasSK {
		return nil
	}

	sk := ""
	if s, ok := skAttr.(*types.AttributeValueMemberS); ok {
		sk = s.Value
	}

	if sk != "METADATA" {
		return nil // Skip non-metadata records
	}

	// Convert the Object attribute to a proper structure
	type ObjectTag struct {
		Type string `dynamodbav:"type" json:"type"`
		Href string `dynamodbav:"href,omitempty" json:"href,omitempty"`
		Name string `dynamodbav:"name" json:"name"`
	}

	type ObjectAttachment struct {
		Type      string `dynamodbav:"type" json:"type"`
		URL       string `dynamodbav:"url" json:"url"`
		MediaType string `dynamodbav:"mediaType,omitempty" json:"mediaType,omitempty"`
		Name      string `dynamodbav:"name,omitempty" json:"name,omitempty"`
	}

	type Object struct {
		ID           string             `dynamodbav:"id" json:"id"`
		Type         string             `dynamodbav:"type" json:"type"`
		AttributedTo string             `dynamodbav:"attributedTo,omitempty" json:"attributedTo,omitempty"`
		Content      string             `dynamodbav:"content,omitempty" json:"content,omitempty"`
		Summary      string             `dynamodbav:"summary,omitempty" json:"summary,omitempty"`
		URL          string             `dynamodbav:"url,omitempty" json:"url,omitempty"`
		Published    time.Time          `dynamodbav:"published" json:"published"`
		To           []string           `dynamodbav:"to,omitempty" json:"to,omitempty"`
		CC           []string           `dynamodbav:"cc,omitempty" json:"cc,omitempty"`
		InReplyTo    *string            `dynamodbav:"inReplyTo,omitempty" json:"inReplyTo,omitempty"`
		Sensitive    bool               `dynamodbav:"sensitive,omitempty" json:"sensitive,omitempty"`
		Attachment   []ObjectAttachment `dynamodbav:"attachment,omitempty" json:"attachment,omitempty"`
		Tag          []ObjectTag        `dynamodbav:"tag,omitempty" json:"tag,omitempty"`
	}

	var objectRecord struct {
		PK        string    `dynamodbav:"PK"`
		SK        string    `dynamodbav:"SK"`
		Object    *Object   `dynamodbav:"Object"`
		CreatedAt time.Time `dynamodbav:"CreatedAt"`
		UpdatedAt time.Time `dynamodbav:"UpdatedAt"`
	}
	if err := attributevalue.UnmarshalMap(statusItem, &objectRecord); err != nil {
		log.Error("failed to unmarshal object record", zap.Error(err))
		return nil
	}

	object := objectRecord.Object
	if object == nil {
		return nil
	}

	// Create simplified status payload for WebSocket
	statusPayload := map[string]any{
		"id":           strings.TrimPrefix(object.ID, "https://"),
		"uri":          object.ID,
		"url":          object.URL,
		"content":      object.Content,
		"created_at":   object.Published.Format(time.RFC3339),
		"visibility":   "public", // Will be updated below
		"language":     "en",     // Default
		"spoiler_text": object.Summary,
		"sensitive":    object.Sensitive,
		"account": map[string]any{
			"id":       extractUsernameFromActorID(object.AttributedTo),
			"username": extractUsernameFromActorID(object.AttributedTo),
			"acct":     extractUsernameFromActorID(object.AttributedTo),
			"url":      object.AttributedTo,
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
	if object.InReplyTo != nil && *object.InReplyTo != "" {
		statusPayload["in_reply_to_id"] = strings.TrimPrefix(*object.InReplyTo, "https://")
		statusPayload["in_reply_to_account_id"] = nil
	}

	// Process tags
	for _, tag := range object.Tag {
		if tag.Type == "Hashtag" {
			tagName := strings.TrimPrefix(tag.Name, "#")
			statusPayload["tags"] = append(statusPayload["tags"].([]any), map[string]any{
				"name": tagName,
				"url":  tag.Href,
			})
		}
	}

	// Process attachments
	for _, att := range object.Attachment {
		attachment := map[string]any{
			"id":          generateID(8),
			"type":        getAttachmentType(att.MediaType),
			"url":         att.URL,
			"preview_url": att.URL,
			"description": att.Name,
		}
		statusPayload["media_attachments"] = append(statusPayload["media_attachments"].([]any), attachment)
	}

	// Marshal the status for payload
	payload, err := json.Marshal(statusPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal status: %w", err)
	}

	// Determine visibility
	visibility := "public" // default
	if len(object.To) > 0 || len(object.CC) > 0 {
		if contains(object.To, activitypub.PublicAddress) {
			visibility = "public"
		} else if contains(object.CC, activitypub.PublicAddress) {
			visibility = "unlisted"
		} else if len(object.To) == 1 && !strings.Contains(object.To[0], "/followers") {
			visibility = "direct"
		} else {
			visibility = "private"
		}
	}

	// Route to appropriate streams
	streams := []string{}

	// Public timelines
	if visibility == "public" {
		streams = append(streams, "public", "public:local")
	}

	// User stream for the author
	username := extractUsernameFromActorID(object.AttributedTo)
	if username != "" {
		streams = append(streams, fmt.Sprintf("user:%s", username))
	}

	// Send to all relevant streams
	eventType := "update"
	if record.EventName == "MODIFY" {
		eventType = "status.update"
	}

	for _, stream := range streams {
		if err := broadcastToStream(ctx, stream, eventType, payload); err != nil {
			log.Error("failed to broadcast to stream",
				zap.String("stream", stream),
				zap.Error(err))
			// Continue with other streams
		}
	}

	return nil
}

func processNotificationEvent(ctx context.Context, record events.DynamoDBEventRecord) error {
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
		log.Error("failed to unmarshal notification record", zap.Error(err))
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
	return broadcastToStream(ctx, fmt.Sprintf("user:notification:%s", username), "notification", payload)
}

func processAccountEvent(ctx context.Context, record events.DynamoDBEventRecord) error {
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
		log.Error("failed to broadcast to followers", zap.Error(err))
		// Don't return error - logging is sufficient for broadcast failures
	}
	log.Info("account update event",
		zap.String("accountID", accountID),
		zap.Int("payloadSize", len(payload)))

	return nil
}

func broadcastToStream(ctx context.Context, stream, eventType string, payload json.RawMessage) error {
	log := log.With(
		zap.String("stream", stream),
		zap.String("event", eventType),
	)

	// Initialize API Gateway Management API client if not already done
	if apiClient == nil {
		apiClient = apigatewaymanagementapi.NewFromConfig(globalCfg, func(o *apigatewaymanagementapi.Options) {
			o.BaseEndpoint = aws.String(wsEndpoint)
		})
	}

	// Query subscriptions for this stream
	result, err := dynamoClient.Query(ctx, &dynamodb.QueryInput{
		TableName:              &subscriptionsTable,
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: "SUB#" + stream},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to query subscriptions: %w", err)
	}

	// Send to each connection
	message := StreamMessage{
		Event:   eventType,
		Payload: payload,
		Stream:  stream,
	}

	messageBytes, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	successCount := 0
	failureCount := 0

	for _, item := range result.Items {
		// Get connection ID
		connectionID := ""
		if attr, ok := item["ConnectionID"]; ok {
			if s, ok := attr.(*types.AttributeValueMemberS); ok {
				connectionID = s.Value
			}
		}

		if connectionID == "" {
			continue
		}

		// Send message
		_, err := apiClient.PostToConnection(ctx, &apigatewaymanagementapi.PostToConnectionInput{
			ConnectionId: &connectionID,
			Data:         messageBytes,
		})

		if err != nil {
			// Check if connection is gone
			if strings.Contains(err.Error(), "410") || strings.Contains(err.Error(), "GoneException") {
				// Clean up stale subscription
				cleanupErr := deleteSubscription(ctx, connectionID, stream)
				if cleanupErr != nil {
					log.Error("failed to cleanup stale subscription",
						zap.String("connectionID", connectionID),
						zap.Error(cleanupErr))
				}
			} else {
				log.Error("failed to send to connection",
					zap.String("connectionID", connectionID),
					zap.Error(err))
			}
			failureCount++
		} else {
			successCount++
		}
	}

	log.Info("broadcast complete",
		zap.Int("success", successCount),
		zap.Int("failure", failureCount))

	return nil
}

func deleteSubscription(ctx context.Context, connectionID, stream string) error {
	_, err := dynamoClient.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: &subscriptionsTable,
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "SUB#" + stream},
			"SK": &types.AttributeValueMemberS{Value: "CONN#" + connectionID},
		},
	})
	return err
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

	log.Info("broadcasting account update to followers",
		zap.String("account_id", accountID),
		zap.Int("payload_size", len(payload)))

	// Placeholder implementation - would be replaced with actual follower query and broadcast
	return nil
}

func main() {
	lambda.Start(handler)
}
