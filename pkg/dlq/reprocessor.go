package dlq

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/equaltoai/lesser/pkg/httpclient"
	"go.uber.org/zap"
)

// ReprocessorClient handles reprocessing of failed messages
type ReprocessorClient struct {
	sqsClient  SQSClient
	httpClient httpDoer
	logger     *zap.Logger
	queueURLs  map[string]string // Cache for queue URLs
}

type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// NewReprocessorClient creates a new reprocessor client
func NewReprocessorClient(logger *zap.Logger) *ReprocessorClient {
	// Create HTTP client with short timeout for HEAD requests
	httpClient := httpclient.NewSecureClient(
		httpclient.WithTimeout(5*time.Second),
		httpclient.WithLogger(logger),
		httpclient.WithMaxRedirects(3),
	)

	return &ReprocessorClient{
		httpClient: httpClient,
		logger:     logger,
		queueURLs:  make(map[string]string),
	}
}

// SetSQSClient sets the SQS client
func (r *ReprocessorClient) SetSQSClient(client SQSClient) {
	r.sqsClient = client
}

// ReprocessConfig defines configuration for message reprocessing
type ReprocessConfig struct {
	ValidateMessage    func(map[string]interface{}) error
	CheckAccessibility func(context.Context, map[string]interface{}) error
	ReprocessType      string
}

// reprocessWithValidation provides a generic reprocessing workflow
func (r *ReprocessorClient) reprocessWithValidation(ctx context.Context, originalMessage *OriginalMessage, messageType string, config ReprocessConfig) error {
	r.logger.Info("reprocessing message",
		zap.String("message_id", originalMessage.MessageID),
		zap.String("message_type", messageType),
	)

	// Parse the message request
	var messageRequest map[string]interface{}
	if err := json.Unmarshal([]byte(originalMessage.Body), &messageRequest); err != nil {
		return fmt.Errorf("invalid %s message format: %w", messageType, err)
	}

	// Validate message structure
	if err := config.ValidateMessage(messageRequest); err != nil {
		return fmt.Errorf("%s message validation failed: %w", messageType, err)
	}

	// Check accessibility if configured
	if config.CheckAccessibility != nil {
		if err := config.CheckAccessibility(ctx, messageRequest); err != nil {
			return fmt.Errorf("%s not accessible for reprocessing: %w", messageType, err)
		}
	}

	// Send back to the original queue
	queueURL, err := r.getQueueURL(ctx, originalMessage.SourceQueue)
	if err != nil {
		return fmt.Errorf("failed to get queue URL for %s: %w", originalMessage.SourceQueue, err)
	}

	return r.sendMessageToQueue(ctx, queueURL, originalMessage.Body, originalMessage.Attributes, config.ReprocessType)
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
	return r.reprocessWithValidation(ctx, originalMessage, "media", ReprocessConfig{
		ValidateMessage: r.validateMediaMessage,
		CheckAccessibility: func(ctx context.Context, request map[string]interface{}) error {
			if mediaURL, exists := request["media_url"]; exists {
				if urlStr, ok := mediaURL.(string); ok {
					return r.validateMediaAccessibility(ctx, urlStr)
				}
			}
			return nil
		},
		ReprocessType: "media_reprocess",
	})
}

// ReprocessFederation reprocesses a failed federation delivery message
func (r *ReprocessorClient) ReprocessFederation(ctx context.Context, originalMessage *OriginalMessage) error {
	return r.reprocessWithValidation(ctx, originalMessage, "federation", ReprocessConfig{
		ValidateMessage: r.validateFederationMessage,
		CheckAccessibility: func(ctx context.Context, request map[string]interface{}) error {
			if inboxURL, exists := request["inbox_url"]; exists {
				if urlStr, ok := inboxURL.(string); ok {
					return r.validateInboxAccessibility(ctx, urlStr)
				}
			}
			return nil
		},
		ReprocessType: "federation_reprocess",
	})
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
			return fmt.Errorf("%w: %s", ErrMissingRequiredField, field)
		}
	}

	// Validate channels is an array
	if channels, exists := message["channels"]; exists {
		if _, ok := channels.([]interface{}); !ok {
			return ErrChannelsMustBeArray
		}
	}

	return nil
}

// validateActivityMessage validates an ActivityPub activity message
func (r *ReprocessorClient) validateActivityMessage(message map[string]interface{}) error {
	// Check for ActivityPub required fields
	if activityType, exists := message["type"]; !exists {
		return ErrMissingActivityPubType
	} else if _, ok := activityType.(string); !ok {
		return ErrActivityPubTypeMustBeString
	}

	if actor, exists := message["actor"]; !exists {
		return ErrMissingActivityPubActor
	} else if _, ok := actor.(string); !ok {
		return ErrActivityPubActorMustBeString
	}

	return nil
}

// validateMediaMessage validates a media processing message
func (r *ReprocessorClient) validateMediaMessage(message map[string]interface{}) error {
	requiredFields := []string{"media_id", "media_url", "processing_type"}

	for _, field := range requiredFields {
		if _, exists := message[field]; !exists {
			return fmt.Errorf("%w: %s", ErrMissingRequiredField, field)
		}
	}

	// Validate media URL format
	if mediaURL, exists := message["media_url"]; exists {
		if urlStr, ok := mediaURL.(string); ok {
			if !r.isValidURL(urlStr) {
				return ErrInvalidMediaURLFormat
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
			return fmt.Errorf("%w: %s", ErrMissingRequiredField, field)
		}
	}

	// Validate inbox URL format
	if inboxURL, exists := message["inbox_url"]; exists {
		if urlStr, ok := inboxURL.(string); ok {
			if !r.isValidURL(urlStr) {
				return ErrInvalidInboxURLFormat
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
			return fmt.Errorf("%w: %s", ErrMissingRequiredField, field)
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
				return fmt.Errorf("%w: %s", ErrInvalidAction, actionStr)
			}
		}
	}

	return nil
}

// validateMediaAccessibility checks if media is still accessible
func (r *ReprocessorClient) validateMediaAccessibility(ctx context.Context, mediaURL string) error {
	// Basic URL validation
	if !r.isValidURL(mediaURL) {
		return ErrInvalidMediaURL
	}

	// Perform HTTP HEAD request to check accessibility
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, mediaURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create HEAD request: %w", err)
	}

	// Add appropriate headers for media requests
	req.Header.Set("User-Agent", "Lesser/1.0 (ActivityPub; +https://lesser.social)")
	req.Header.Set("Accept", "*/*")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		// Network/timeout errors are retryable
		r.logger.Debug("Media HEAD request failed, treating as retryable",
			zap.String("url", mediaURL),
			zap.Error(err))
		return nil // Allow retry
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			r.logger.Warn("Failed to close response body", zap.Error(closeErr))
		}
	}()

	// Classify HTTP response codes
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 400:
		// Success or redirect - media is accessible
		r.logger.Debug("Media is accessible, allowing retry",
			zap.String("url", mediaURL),
			zap.Int("status_code", resp.StatusCode))
		return nil

	case resp.StatusCode == 404 || resp.StatusCode == 410:
		// Not Found or Gone - permanent failure
		r.logger.Info("Media permanently unavailable, marking as non-retryable",
			zap.String("url", mediaURL),
			zap.Int("status_code", resp.StatusCode))
		return fmt.Errorf("%w (HTTP %d)", ErrMediaPermanentlyUnavailable, resp.StatusCode)

	case resp.StatusCode == 429 || resp.StatusCode == 503:
		// Rate limited or service unavailable - temporary issue
		r.logger.Debug("Media temporarily unavailable, allowing retry",
			zap.String("url", mediaURL),
			zap.Int("status_code", resp.StatusCode))
		return nil

	case resp.StatusCode == 401 || resp.StatusCode == 403:
		// Auth issues - may be permanent depending on context
		r.logger.Warn("Media access denied, treating as potentially permanent",
			zap.String("url", mediaURL),
			zap.Int("status_code", resp.StatusCode))
		return fmt.Errorf("%w (HTTP %d)", ErrMediaAccessDenied, resp.StatusCode)

	default:
		// Classify error based on HTTP status code semantics
		if r.isRetryableHTTPStatus(resp.StatusCode) {
			r.logger.Debug("Media HEAD request returned retryable error",
				zap.String("url", mediaURL),
				zap.Int("status_code", resp.StatusCode))
			return nil
		}
		r.logger.Warn("Media HEAD request returned non-retryable error",
			zap.String("url", mediaURL),
			zap.Int("status_code", resp.StatusCode))
		return fmt.Errorf("%w (HTTP %d)", ErrMediaValidationFailed, resp.StatusCode)
	}
}

// validateInboxAccessibility checks if a federation inbox is accessible
func (r *ReprocessorClient) validateInboxAccessibility(ctx context.Context, inboxURL string) error {
	// Basic URL validation
	if !r.isValidURL(inboxURL) {
		return ErrInvalidInboxURL
	}

	// Perform HTTP HEAD request to check accessibility
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, inboxURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create HEAD request: %w", err)
	}

	// Add ActivityPub headers for inbox requests
	req.Header.Set("User-Agent", "Lesser/1.0 (ActivityPub; +https://lesser.social)")
	req.Header.Set("Accept", "application/activity+json, application/ld+json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		// Network/timeout errors are retryable - instance might be temporarily down
		r.logger.Debug("Inbox HEAD request failed, treating as retryable",
			zap.String("url", inboxURL),
			zap.Error(err))
		return nil // Allow retry
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			r.logger.Warn("Failed to close response body", zap.Error(closeErr))
		}
	}()

	// Classify HTTP response codes
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 400:
		// Success or redirect - inbox is accessible
		r.logger.Debug("Inbox is accessible, allowing retry",
			zap.String("url", inboxURL),
			zap.Int("status_code", resp.StatusCode))
		return nil

	case resp.StatusCode == 404 || resp.StatusCode == 410:
		// Not Found or Gone - might be permanent but could be temporary server config
		// For federation, we're more conservative and allow retries
		r.logger.Debug("Inbox returned 404/410, allowing retry for federation",
			zap.String("url", inboxURL),
			zap.Int("status_code", resp.StatusCode))
		return nil

	case resp.StatusCode == 429 || resp.StatusCode == 503:
		// Rate limited or service unavailable - temporary issue
		r.logger.Debug("Inbox temporarily unavailable, allowing retry",
			zap.String("url", inboxURL),
			zap.Int("status_code", resp.StatusCode))
		return nil

	case resp.StatusCode == 401 || resp.StatusCode == 403:
		// Auth issues - for federation this might indicate signature problems
		// Allow retry as it might be a temporary key/signature issue
		r.logger.Debug("Inbox access denied, allowing retry for potential signature issues",
			zap.String("url", inboxURL),
			zap.Int("status_code", resp.StatusCode))
		return nil

	case resp.StatusCode >= 500:
		// Server errors - definitely retryable
		r.logger.Debug("Inbox server error, allowing retry",
			zap.String("url", inboxURL),
			zap.Int("status_code", resp.StatusCode))
		return nil

	default:
		// Other client errors - treat as retryable to be safe for federation
		r.logger.Debug("Inbox HEAD request returned client error, allowing retry",
			zap.String("url", inboxURL),
			zap.Int("status_code", resp.StatusCode))
		return nil
	}
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
		TotalMessages:         len(messages),
		SuccessfulReprocesses: 0,
		FailedReprocesses:     0,
		Errors:                make([]string, 0),
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
	MaxRetries         int    `json:"max_retries"`
	DelaySeconds       int32  `json:"delay_seconds"`
	BackoffStrategy    string `json:"backoff_strategy"` // "linear", "exponential", "fixed"
	ValidateFirst      bool   `json:"validate_first"`
	CheckAccessibility bool   `json:"check_accessibility"`
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

// isRetryableHTTPStatus determines if an HTTP status code indicates a retryable error
func (r *ReprocessorClient) isRetryableHTTPStatus(statusCode int) bool {
	switch {
	case statusCode >= 200 && statusCode < 400:
		// Success and redirects - not an error
		return true

	case statusCode >= 500 && statusCode < 600:
		// Server errors - generally retryable
		return true

	case statusCode == 408: // Request Timeout
		return true

	case statusCode == 429: // Too Many Requests
		return true

	case statusCode == 502 || statusCode == 503 || statusCode == 504:
		// Bad Gateway, Service Unavailable, Gateway Timeout
		return true

	case statusCode >= 400 && statusCode < 500:
		// Client errors - generally not retryable
		switch statusCode {
		case 404, 410: // Not Found, Gone - permanent
			return false
		case 401, 403: // Unauthorized, Forbidden - depends on context
			return false
		case 400, 405, 406, 409, 422: // Bad Request, Method Not Allowed, etc.
			return false
		default:
			// Unknown 4xx errors - be conservative and don't retry
			return false
		}

	default:
		// Unknown status codes - be conservative and don't retry
		return false
	}
}
