package observability

import (
	"fmt"
	"os"
)

// WithSNSNotifier creates an option with the specified SNS notifier
func WithSNSNotifier(notifier *SNSNotifier) interface{} {
	return notifier
}

// WithDefaultErrorNotifications creates an SNS notifier with default configuration
// It builds the SNS topic ARN using the standard Pay Theory format
func WithDefaultErrorNotifications(snsClient SNSClient) *SNSNotifier {
	partner := os.Getenv("PARTNER")
	stage := os.Getenv("STAGE")
	region := os.Getenv("AWS_REGION")
	accountID := os.Getenv("AWS_ACCOUNT_ID")

	if partner == "" || stage == "" || region == "" || accountID == "" {
		// Return nil if environment variables are not set
		return nil
	}

	// Build the standard SNS topic ARN
	topicARN := fmt.Sprintf("arn:aws:sns:%s:%s:cns-%s-%s", region, accountID, partner, stage)

	return WithErrorNotifications(snsClient, topicARN)
}

// WithErrorNotifications creates an SNS notifier with the specified topic ARN
func WithErrorNotifications(snsClient SNSClient, topicARN string) *SNSNotifier {
	config := SNSConfig{
		Client:   snsClient,
		TopicARN: topicARN,
	}

	return NewSNSNotifier(config)
}
