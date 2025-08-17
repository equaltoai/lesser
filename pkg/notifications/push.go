// Package notifications provides push notification services using SQS for message queuing and delivery.
package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/equaltoai/lesser/pkg/common"
	"go.uber.org/zap"
)

// PushService handles queuing push notifications
type PushService struct {
	sqsClient *sqs.Client
	queueURL  string
	logger    *zap.Logger
}

// PushMessage represents a push notification message
type PushMessage struct {
	Username         string `json:"username"`
	NotificationType string `json:"notification_type"`
	Title            string `json:"title"`
	Body             string `json:"body"`
	Icon             string `json:"icon,omitempty"`
	NotificationID   string `json:"notification_id"`
	AccessToken      string `json:"access_token"`
}

// NewPushService creates a new push notification service
func NewPushService() (*PushService, error) {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	sqsClient := sqs.NewFromConfig(cfg)

	queueURL := os.Getenv("PUSH_NOTIFICATION_QUEUE_URL")
	if err := common.ValidateRequiredParam("queueURL", queueURL); err != nil {
		// Queue might not be configured, return nil service
		return nil, nil
	}

	logger := common.Logger()

	return &PushService{
		sqsClient: sqsClient,
		queueURL:  queueURL,
		logger:    logger,
	}, nil
}

// QueueNotification queues a push notification for delivery
func (s *PushService) QueueNotification(ctx context.Context, msg *PushMessage) error {
	if s == nil {
		// Push notifications not configured
		return nil
	}

	messageBody, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal push message: %w", err)
	}

	input := &sqs.SendMessageInput{
		QueueUrl:    aws.String(s.queueURL),
		MessageBody: aws.String(string(messageBody)),
	}

	_, err = s.sqsClient.SendMessage(ctx, input)
	if err != nil {
		s.logger.Error("failed to send push notification to queue",
			zap.String("username", msg.Username),
			zap.String("type", msg.NotificationType),
			zap.Error(err))
		return fmt.Errorf("failed to queue push notification: %w", err)
	}

	s.logger.Info("queued push notification",
		zap.String("username", msg.Username),
		zap.String("type", msg.NotificationType))

	return nil
}

// FormatNotificationTitle formats a notification title based on type
func FormatNotificationTitle(notificationType string, actorName string) string {
	switch notificationType {
	case "follow":
		return fmt.Sprintf("%s followed you", actorName)
	case "favourite":
		return fmt.Sprintf("%s favourited your post", actorName)
	case "reblog":
		return fmt.Sprintf("%s boosted your post", actorName)
	case "mention":
		return fmt.Sprintf("%s mentioned you", actorName)
	case "poll":
		return "A poll you voted in has ended"
	case "follow_request":
		return fmt.Sprintf("%s requested to follow you", actorName)
	case "status":
		return fmt.Sprintf("%s posted", actorName)
	case "update":
		return fmt.Sprintf("%s edited a post", actorName)
	default:
		return "New notification"
	}
}

// FormatNotificationBody formats a notification body based on type and content
func FormatNotificationBody(notificationType string, content string) string {
	if notificationType == "mention" && content != "" {
		// For mentions, include a preview of the content
		if len(content) > 100 {
			content = content[:97] + "..."
		}
		return content
	}

	// For other types, return empty body or a simple message
	return ""
}
