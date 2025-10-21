package zap

import (
	"github.com/aws/aws-sdk-go-v2/service/sns"

	"github.com/pay-theory/lift/pkg/observability"
)

// WithSNSNotifier creates a ZapLoggerOptions with the specified SNS notifier
func WithSNSNotifier(notifier *observability.SNSNotifier) ZapLoggerOptions {
	return ZapLoggerOptions{
		Notifier: notifier,
	}
}

// WithDefaultErrorNotifications creates a ZapLoggerOptions with default SNS notifications for errors
// It builds the SNS topic ARN using the standard Pay Theory format
func WithDefaultErrorNotifications(snsClient *sns.Client) ZapLoggerOptions {
	notifier := observability.WithDefaultErrorNotifications(snsClient)

	return ZapLoggerOptions{
		Notifier: notifier,
	}
}

// WithErrorNotifications creates a ZapLoggerOptions with SNS notifications for errors
func WithErrorNotifications(snsClient *sns.Client, topicARN string) ZapLoggerOptions {
	notifier := observability.WithErrorNotifications(snsClient, topicARN)

	return ZapLoggerOptions{
		Notifier: notifier,
	}
}
