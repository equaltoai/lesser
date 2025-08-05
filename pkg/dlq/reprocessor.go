package dlq

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"go.uber.org/zap"
)

// ReprocessorClient handles reprocessing of failed messages
type ReprocessorClient struct {
	sqsClient *sqs.Client
	logger    *zap.Logger
	queueURLs map[string]string // Cache for queue URLs
}

// NewReprocessorClient creates a new reprocessor client
func NewReprocessorClient(logger *zap.Logger) *ReprocessorClient {
	return &ReprocessorClient{
		logger:    logger,
		queueURLs: make(map[string]string),
	}
}

// SetSQSClient sets the SQS client
func (r *ReprocessorClient) SetSQSClient(client *sqs.Client) {
	r.sqsClient = client
}

// ReprocessNotification reprocesses a failed notification message
func (r *ReprocessorClient) ReprocessNotification(ctx context.Context, originalMessage *OriginalMessage) error {
	r.logger.Info("reprocessing notification message",
		zap.String("message_id", originalMessage.MessageID),
	)

	// Parse the notification message to validate it
	var notificationRequest map[string]interface{}
	if err := json.Unmarshal([]byte(originalMessage.Body), &notificationRequest); err != nil {
		return fmt.Errorf("invalid notification message format: %w", err)
	}

	// Validate required fields
	if err := r.validateNotificationMessage(notificationRequest); err != nil {
		return fmt.Errorf("notification message validation failed: %w", err)
	}

	// Send back to the original queue
	queueURL, err := r.getQueueURL(ctx, originalMessage.SourceQueue)
	if err != nil {
		return fmt.Errorf("failed to get queue URL for %s: %w", originalMessage.SourceQueue, err)
	}

	return r.sendMessageToQueue(ctx, queueURL, originalMessage.Body, originalMessage.Attributes, "notification_reprocess")
}

// ReprocessActivity reprocesses a failed activity message
func (r *ReprocessorClient) ReprocessActivity(ctx context.Context, originalMessage *OriginalMessage) error {
	r.logger.Info("reprocessing activity message",
		zap.String("message_id", originalMessage.MessageID),
	)

	// Parse the activity message
	var activityRequest map[string]interface{}
	if err := json.Unmarshal([]byte(originalMessage.Body), &activityRequest); err != nil {
		return fmt.Errorf("invalid activity message format: %w", err)
	}

	// Validate ActivityPub structure
	if err := r.validateActivityMessage(activityRequest); err != nil {
		return fmt.Errorf("activity message validation failed: %w", err)
	}

	// Send back to the original queue
	queueURL, err := r.getQueueURL(ctx, originalMessage.SourceQueue)
	if err != nil {
		return fmt.Errorf("failed to get queue URL for %s: %w", originalMessage.SourceQueue, err)
	}

	return r.sendMessageToQueue(ctx, queueURL, originalMessage.Body, originalMessage.Attributes, "activity_reprocess")
}

// ReprocessMedia reprocesses a failed media processing message
func (r *ReprocessorClient) ReprocessMedia(ctx context.Context, originalMessage *OriginalMessage) error {
	r.logger.Info("reprocessing media message",
		zap.String("message_id", originalMessage.MessageID),
	)

	// Parse the media processing request
	var mediaRequest map[string]interface{}
	if err := json.Unmarshal([]byte(originalMessage.Body), &mediaRequest); err != nil {
		return fmt.Errorf("invalid media message format: %w", err)
	}

	// Validate media processing request
	if err := r.validateMediaMessage(mediaRequest); err != nil {
		return fmt.Errorf("media message validation failed: %w", err)
	}

	// Check if media is still accessible (for transient network errors)
	if mediaURL, exists := mediaRequest["media_url"]; exists {
		if urlStr, ok := mediaURL.(string); ok {
			if err := r.validateMediaAccessibility(ctx, urlStr); err != nil {
				return fmt.Errorf("media not accessible for reprocessing: %w", err)
			}
		}
	}

	// Send back to the original queue
	queueURL, err := r.getQueueURL(ctx, originalMessage.SourceQueue)
	if err != nil {
		return fmt.Errorf("failed to get queue URL for %s: %w", originalMessage.SourceQueue, err)
	}

	return r.sendMessageToQueue(ctx, queueURL, originalMessage.Body, originalMessage.Attributes, "media_reprocess")
}

// ReprocessFederation reprocesses a failed federation delivery message
func (r *ReprocessorClient) ReprocessFederation(ctx context.Context, originalMessage *OriginalMessage) error {
	r.logger.Info("reprocessing federation message",
		zap.String("message_id", originalMessage.MessageID),
	)

	// Parse the federation delivery request
	var federationRequest map[string]interface{}
	if err := json.Unmarshal([]byte(originalMessage.Body), &federationRequest); err != nil {
		return fmt.Errorf("invalid federation message format: %w", err)
	}

	// Validate federation delivery request
	if err := r.validateFederationMessage(federationRequest); err != nil {
		return fmt.Errorf("federation message validation failed: %w", err)
	}

	// Check if the target inbox is now accessible
	if inboxURL, exists := federationRequest["inbox_url"]; exists {
		if urlStr, ok := inboxURL.(string); ok {
			if err := r.validateInboxAccessibility(ctx, urlStr); err != nil {
				return fmt.Errorf("inbox not accessible for reprocessing: %w", err)
			}
		}
	}

	// Send back to the original queue
	queueURL, err := r.getQueueURL(ctx, originalMessage.SourceQueue)
	if err != nil {
		return fmt.Errorf("failed to get queue URL for %s: %w", originalMessage.SourceQueue, err)
	}

	return r.sendMessageToQueue(ctx, queueURL, originalMessage.Body, originalMessage.Attributes, "federation_reprocess")
}

// ReprocessSearch reprocesses a failed search indexing message
func (r *ReprocessorClient) ReprocessSearch(ctx context.Context, originalMessage *OriginalMessage) error {
	r.logger.Info("reprocessing search message",
		zap.String("message_id", originalMessage.MessageID),
	)

	// Parse the search indexing request
	var searchRequest map[string]interface{}
	if err := json.Unmarshal([]byte(originalMessage.Body), &searchRequest); err != nil {
		return fmt.Errorf("invalid search message format: %w", err)
	}

	// Validate search indexing request
	if err := r.validateSearchMessage(searchRequest); err != nil {
		return fmt.Errorf("search message validation failed: %w", err)
	}

	// Send back to the original queue
	queueURL, err := r.getQueueURL(ctx, originalMessage.SourceQueue)
	if err != nil {
		return fmt.Errorf("failed to get queue URL for %s: %w", originalMessage.SourceQueue, err)
	}

	return r.sendMessageToQueue(ctx, queueURL, originalMessage.Body, originalMessage.Attributes, "search_reprocess")
}

// ReprocessGeneric reprocesses a message for an unknown service
func (r *ReprocessorClient) ReprocessGeneric(ctx context.Context, sourceQueue string, originalMessage *OriginalMessage) error {
	r.logger.Info("reprocessing generic message",
		zap.String("message_id", originalMessage.MessageID),
		zap.String("source_queue", sourceQueue),
	)

	// Basic JSON validation
	var messageContent map[string]interface{}
	if err := json.Unmarshal([]byte(originalMessage.Body), &messageContent); err != nil {
		// If it's not JSON, treat as plain text message
		r.logger.Warn("message is not JSON, treating as plain text",
			zap.String("message_id", originalMessage.MessageID),
		)
	}

	// Send back to the original queue
	queueURL, err := r.getQueueURL(ctx, sourceQueue)
	if err != nil {
		return fmt.Errorf("failed to get queue URL for %s: %w", sourceQueue, err)
	}

	return r.sendMessageToQueue(ctx, queueURL, originalMessage.Body, originalMessage.Attributes, "generic_reprocess")
}

// getQueueURL gets the URL for a queue name (with caching)
func (r *ReprocessorClient) getQueueURL(ctx context.Context, queueName string) (string, error) {
	// Check cache first
	if url, exists := r.queueURLs[queueName]; exists {
		return url, nil
	}

	// Get queue URL from SQS
	result, err := r.sqsClient.GetQueueUrl(ctx, &sqs.GetQueueUrlInput{
		QueueName: aws.String(queueName),
	})
	if err != nil {
		return "", fmt.Errorf("failed to get queue URL: %w", err)
	}

	// Cache the URL
	r.queueURLs[queueName] = *result.QueueUrl
	return *result.QueueUrl, nil
}

// sendMessageToQueue sends a message to an SQS queue
func (r *ReprocessorClient) sendMessageToQueue(ctx context.Context, queueURL, messageBody string, attributes map[string]string, reprocessType string) error {
	// Add reprocessing metadata
	messageAttributes := make(map[string]types.MessageAttributeValue)
	
	// Copy original attributes
	for key, value := range attributes {
		messageAttributes[key] = types.MessageAttributeValue{
			DataType:    aws.String("String"),
			StringValue: aws.String(value),
		}
	}

	// Add reprocessing metadata
	messageAttributes["DLQ.ReprocessType"] = types.MessageAttributeValue{
		DataType:    aws.String("String"),
		StringValue: aws.String(reprocessType),
	}
	
	messageAttributes["DLQ.ReprocessTimestamp"] = types.MessageAttributeValue{
		DataType:    aws.String("String"),
		StringValue: aws.String(time.Now().Format(time.RFC3339)),
	}

	// Send message
	_, err := r.sqsClient.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:          aws.String(queueURL),
		MessageBody:       aws.String(messageBody),
		MessageAttributes: messageAttributes,
		DelaySeconds:      30, // Add a small delay to avoid immediate re-failure
	})

	if err != nil {
		return fmt.Errorf("failed to send message to queue: %w", err)
	}

	r.logger.Info("successfully reprocessed message",
		zap.String("queue_url", queueURL),
		zap.String("reprocess_type", reprocessType),
	)

	return nil
}

// Validation functions

// validateNotificationMessage validates a notification message structure
func (r *ReprocessorClient) validateNotificationMessage(message map[string]interface{}) error {
	requiredFields := []string{"notification_id", "user_id", "channels"}
	
	for _, field := range requiredFields {
		if _, exists := message[field]; !exists {
			return fmt.Errorf("missing required field: %s", field)
		}
	}

	// Validate channels is an array
	if channels, exists := message["channels"]; exists {
		if _, ok := channels.([]interface{}); !ok {
			return fmt.Errorf("channels must be an array")
		}
	}

	return nil
}

// validateActivityMessage validates an ActivityPub activity message
func (r *ReprocessorClient) validateActivityMessage(message map[string]interface{}) error {
	// Check for ActivityPub required fields
	if activityType, exists := message["type"]; !exists {
		return fmt.Errorf("missing ActivityPub type field")
	} else if _, ok := activityType.(string); !ok {
		return fmt.Errorf("ActivityPub type must be a string")
	}

	if actor, exists := message["actor"]; !exists {
		return fmt.Errorf("missing ActivityPub actor field")
	} else if _, ok := actor.(string); !ok {
		return fmt.Errorf("ActivityPub actor must be a string")
	}

	return nil
}

// validateMediaMessage validates a media processing message
func (r *ReprocessorClient) validateMediaMessage(message map[string]interface{}) error {
	requiredFields := []string{"media_id", "media_url", "processing_type"}
	
	for _, field := range requiredFields {
		if _, exists := message[field]; !exists {
			return fmt.Errorf("missing required field: %s", field)
		}
	}

	// Validate media URL format
	if mediaURL, exists := message["media_url"]; exists {
		if urlStr, ok := mediaURL.(string); ok {
			if !r.isValidURL(urlStr) {
				return fmt.Errorf("invalid media URL format")
			}
		}
	}

	return nil
}

// validateFederationMessage validates a federation delivery message
func (r *ReprocessorClient) validateFederationMessage(message map[string]interface{}) error {
	requiredFields := []string{"inbox_url", "activity", "actor_id"}
	
	for _, field := range requiredFields {
		if _, exists := message[field]; !exists {
			return fmt.Errorf("missing required field: %s", field)
		}
	}

	// Validate inbox URL format
	if inboxURL, exists := message["inbox_url"]; exists {
		if urlStr, ok := inboxURL.(string); ok {
			if !r.isValidURL(urlStr) {
				return fmt.Errorf("invalid inbox URL format")
			}
		}
	}

	return nil
}

// validateSearchMessage validates a search indexing message
func (r *ReprocessorClient) validateSearchMessage(message map[string]interface{}) error {
	requiredFields := []string{"object_id", "object_type", "action"}
	
	for _, field := range requiredFields {
		if _, exists := message[field]; !exists {
			return fmt.Errorf("missing required field: %s", field)
		}
	}

	// Validate action is a known value
	if action, exists := message["action"]; exists {
		if actionStr, ok := action.(string); ok {
			validActions := []string{"index", "update", "delete"}
			valid := false
			for _, validAction := range validActions {
				if actionStr == validAction {
					valid = true
					break
				}
			}
			if !valid {
				return fmt.Errorf("invalid action: %s", actionStr)
			}
		}
	}

	return nil
}

// validateMediaAccessibility checks if media is still accessible
func (r *ReprocessorClient) validateMediaAccessibility(ctx context.Context, mediaURL string) error {
	// For now, just validate URL format
	// In a full implementation, you would make an HTTP HEAD request
	if !r.isValidURL(mediaURL) {
		return fmt.Errorf("invalid media URL")
	}
	
	// TODO: Add HTTP HEAD request to check if media is accessible
	// This would help avoid reprocessing messages for permanently deleted media
	
	return nil
}

// validateInboxAccessibility checks if a federation inbox is accessible
func (r *ReprocessorClient) validateInboxAccessibility(ctx context.Context, inboxURL string) error {
	// For now, just validate URL format
	// In a full implementation, you would make an HTTP request to check accessibility
	if !r.isValidURL(inboxURL) {
		return fmt.Errorf("invalid inbox URL")
	}
	
	// TODO: Add HTTP request to check if inbox is accessible
	// This would help avoid reprocessing messages for permanently offline instances
	
	return nil
}

// isValidURL performs basic URL validation
func (r *ReprocessorClient) isValidURL(urlStr string) bool {
	// Basic URL validation
	return len(urlStr) > 0 && 
		   (len(urlStr) >= 8) &&
		   (urlStr[:7] == "http://" || urlStr[:8] == "https://")
}

// BatchReprocess reprocesses multiple messages in batch
func (r *ReprocessorClient) BatchReprocess(ctx context.Context, messages []*OriginalMessage, service string) (*BatchReprocessResult, error) {
	result := &BatchReprocessResult{
		TotalMessages:     len(messages),
		SuccessfulReprocesses: 0,
		FailedReprocesses:     0,
		Errors:            make([]string, 0),
	}

	for _, message := range messages {
		var err error
		
		switch service {
		case "notification-processor":
			err = r.ReprocessNotification(ctx, message)
		case "activity-processor":
			err = r.ReprocessActivity(ctx, message)
		case "media-processor":
			err = r.ReprocessMedia(ctx, message)
		case "federation-delivery":
			err = r.ReprocessFederation(ctx, message)
		case "search-indexer":
			err = r.ReprocessSearch(ctx, message)
		default:
			err = r.ReprocessGeneric(ctx, message.SourceQueue, message)
		}

		if err != nil {
			result.FailedReprocesses++
			result.Errors = append(result.Errors, fmt.Sprintf("Message %s: %v", message.MessageID, err))
		} else {
			result.SuccessfulReprocesses++
		}
	}

	r.logger.Info("completed batch reprocessing",
		zap.String("service", service),
		zap.Int("total", result.TotalMessages),
		zap.Int("successful", result.SuccessfulReprocesses),
		zap.Int("failed", result.FailedReprocesses),
	)

	return result, nil
}

// BatchReprocessResult represents the result of batch reprocessing
type BatchReprocessResult struct {
	TotalMessages         int      `json:"total_messages"`
	SuccessfulReprocesses int      `json:"successful_reprocesses"`
	FailedReprocesses     int      `json:"failed_reprocesses"`
	Errors                []string `json:"errors,omitempty"`
}

// ReprocessingStrategy represents different strategies for reprocessing
type ReprocessingStrategy struct {
	MaxRetries      int           `json:"max_retries"`
	DelaySeconds    int32         `json:"delay_seconds"`
	BackoffStrategy string        `json:"backoff_strategy"` // "linear", "exponential", "fixed"
	ValidateFirst   bool          `json:"validate_first"`
	CheckAccessibility bool        `json:"check_accessibility"`
}

// GetDefaultStrategy returns the default reprocessing strategy for a service
func GetDefaultStrategy(service string) *ReprocessingStrategy {
	strategies := map[string]*ReprocessingStrategy{
		"notification-processor": {
			MaxRetries:         3,
			DelaySeconds:       30,
			BackoffStrategy:    "exponential",
			ValidateFirst:      true,
			CheckAccessibility: false,
		},
		"activity-processor": {
			MaxRetries:         5,
			DelaySeconds:       60,
			BackoffStrategy:    "exponential",
			ValidateFirst:      true,
			CheckAccessibility: true,
		},
		"media-processor": {
			MaxRetries:         3,
			DelaySeconds:       120,
			BackoffStrategy:    "linear",
			ValidateFirst:      true,
			CheckAccessibility: true,
		},
		"federation-delivery": {
			MaxRetries:         5,
			DelaySeconds:       300,
			BackoffStrategy:    "exponential",
			ValidateFirst:      true,
			CheckAccessibility: true,
		},
		"search-indexer": {
			MaxRetries:         3,
			DelaySeconds:       60,
			BackoffStrategy:    "fixed",
			ValidateFirst:      true,
			CheckAccessibility: false,
		},
	}

	if strategy, exists := strategies[service]; exists {
		return strategy
	}

	// Default strategy
	return &ReprocessingStrategy{
		MaxRetries:         3,
		DelaySeconds:       60,
		BackoffStrategy:    "exponential",
		ValidateFirst:      true,
		CheckAccessibility: false,
	}
}