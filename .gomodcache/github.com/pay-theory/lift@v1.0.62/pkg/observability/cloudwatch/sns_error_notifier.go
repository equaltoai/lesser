package cloudwatch

import (
	"github.com/aws/aws-sdk-go-v2/service/sns"

	"github.com/pay-theory/lift/pkg/observability"
)

// WithErrorNotifications creates CloudWatch logger options with SNS error notifications
func WithErrorNotifications(snsClient *sns.Client, topicARN string) CloudWatchLoggerOptions {
	notifier := observability.WithErrorNotifications(snsClient, topicARN)

	return CloudWatchLoggerOptions{
		Notifier: notifier,
	}
}

// WithDefaultErrorNotifications creates CloudWatch logger options with default SNS error notifications
func WithDefaultErrorNotifications(snsClient *sns.Client) CloudWatchLoggerOptions {
	notifier := observability.WithDefaultErrorNotifications(snsClient)

	return CloudWatchLoggerOptions{
		Notifier: notifier,
	}
}
