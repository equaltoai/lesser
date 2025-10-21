package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
)

// SNSConfig contains configuration for SNS notifications
type SNSConfig struct {
	Client   SNSClient
	TopicARN string
}

// SNSNotifier handles sending error notifications to AWS SNS
type SNSNotifier struct {
	snsClient SNSClient
	targetARN string
}

// SNSNotificationMessage represents the structure sent to SNS
type SNSNotificationMessage struct {
	AlertConfig AlertConfig `json:"alert_config"`
	LogTime     string      `json:"log_time"`
	Environment string      `json:"environment,omitempty"`
	Service     string      `json:"service,omitempty"`
	Partner     string      `json:"partner"`
	Stage       string      `json:"stage"`
	AWSRegion   string      `json:"aws_region"`
	AWSAccount  string      `json:"aws_account"`
	Function    string      `json:"function"`
	Subsystem   string      `json:"subsystem"`
	Severity    string      `json:"severity"`
	Message     string      `json:"message"`
}

// AlertConfig contains alert configuration
type AlertConfig struct {
	AlertType       string `json:"alert_type"`
	AlertTargetType string `json:"alert_target_type"`
}

// NewSNSNotifier creates a new SNS notifier from configuration
func NewSNSNotifier(config SNSConfig) *SNSNotifier {
	return &SNSNotifier{
		snsClient: config.Client,
		targetARN: config.TopicARN,
	}
}

// GetTopicARN returns the configured SNS topic ARN
func (n *SNSNotifier) GetTopicARN() string {
	return n.targetARN
}

// NotifyError sends an error notification to SNS when an error is logged
func (n *SNSNotifier) NotifyError(ctx context.Context, logEntry *LogEntry) error {
	// Only notify for ERROR level logs
	if logEntry.Level != "ERROR" {
		return nil
	}

	// Get function name once to use for both Function and Subsystem
	functionName := getEnvOrDefault("AWS_LAMBDA_FUNCTION_NAME", "unknown")

	// Create JSON string representation of the log entry
	logEntryJSON, err := json.Marshal(logEntry)
	if err != nil {
		// Fallback to just the message if marshaling fails
		logEntryJSON = []byte(logEntry.Message)
	}

	// Build the notification message
	notification := SNSNotificationMessage{
		AlertConfig: AlertConfig{
			AlertType:       "LiftError",
			AlertTargetType: "SLACK",
		},
		LogTime:    logEntry.Timestamp.UTC().Format(time.RFC3339),
		Partner:    strings.ToLower(getEnvOrDefault("PARTNER", "unknown")),
		Stage:      strings.ToLower(getEnvOrDefault("STAGE", "unknown")),
		AWSRegion:  getEnvOrDefault("AWS_REGION", "unknown"),
		AWSAccount: getEnvOrDefault("AWS_ACCOUNT_ID", "unknown"),
		Severity:   "ERROR",
		Function:   functionName,
		Subsystem:  functionName, // Set to same value as Function
		Message:    string(logEntryJSON),
	}

	// Add environment and service from fields if available
	if env, ok := logEntry.Fields["environment"].(string); ok {
		notification.Environment = env
	}
	if svc, ok := logEntry.Fields["service"].(string); ok {
		notification.Service = svc
	}

	// Override function name if provided in fields
	if funcName, ok := logEntry.Fields["function_name"].(string); ok {
		notification.Function = funcName
		notification.Subsystem = funcName // Keep subsystem same as function
	} else if funcName, ok := logEntry.Fields["function"].(string); ok {
		notification.Function = funcName
		notification.Subsystem = funcName // Keep subsystem same as function
	}

	// Override AWS region if provided in fields
	if region, ok := logEntry.Fields["aws_region"].(string); ok {
		notification.AWSRegion = region
	} else if region, ok := logEntry.Fields["region"].(string); ok {
		notification.AWSRegion = region
	}

	// Override AWS account if provided in fields
	// Check for account_id from Lift context first (this comes from Context.AccountID())
	if account, ok := logEntry.Fields["account_id"].(string); ok {
		notification.AWSAccount = account
	} else if account, ok := logEntry.Fields["aws_account"].(string); ok {
		notification.AWSAccount = account
	}

	// Marshal the notification to JSON
	messageJSON, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("failed to marshal SNS notification: %w", err)
	}

	// Publish to SNS
	_, err = n.snsClient.Publish(ctx, &sns.PublishInput{
		TargetArn: aws.String(n.targetARN),
		Message:   aws.String(string(messageJSON)),
	})

	if err != nil {
		return fmt.Errorf("failed to publish to CNS via SNS: %w", err)
	}

	return nil
}

// getEnvOrDefault returns the environment variable value or a default if not set
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
