package main

import "github.com/equaltoai/lesser/pkg/errors"

// Error functions for notification processor

// Budget and delivery errors

// ErrNotificationBudgetExceeded returns an error indicating notification delivery blocked due to user budget exceeded.
func ErrNotificationBudgetExceeded() error {
	return errors.NewAppError(errors.CodeRateLimited, errors.CategoryValidation, "notification delivery blocked: user budget exceeded")
}

// Client initialization errors

// ErrSNSClientNotInitialized returns an error indicating SNS client not initialized.
func ErrSNSClientNotInitialized() error {
	return errors.NewLambdaError(errors.CodeInternal, "SNS client not initialized")
}

// ErrAPIGatewayClientNotInitialized returns an error indicating API Gateway client not initialized.
func ErrAPIGatewayClientNotInitialized() error {
	return errors.NewLambdaError(errors.CodeInternal, "API Gateway client not initialized")
}

// ErrSQSClientNotInitialized returns an error indicating SQS client not initialized.
func ErrSQSClientNotInitialized() error {
	return errors.NewLambdaError(errors.CodeInternal, "SQS client not initialized")
}

// Configuration errors

// ErrPushTopicNotConfigured returns an error indicating PUSH_NOTIFICATION_TOPIC_ARN not configured.
func ErrPushTopicNotConfigured() error {
	return errors.NewLambdaError(errors.CodeInternal, "PUSH_NOTIFICATION_TOPIC_ARN not configured")
}

// ErrRetryQueueNotConfigured returns an error indicating retry queue URL not configured.
func ErrRetryQueueNotConfigured() error {
	return errors.NewLambdaError(errors.CodeInternal, "retry queue URL not configured")
}

// ErrSQSConfigurationIncomplete returns an error indicating SQS client or retry queue URL not configured.
func ErrSQSConfigurationIncomplete() error {
	return errors.NewLambdaError(errors.CodeInternal, "SQS client or retry queue URL not configured")
}

// Channel delivery errors

// ErrUnsupportedDeliveryChannel returns an error indicating unsupported delivery channel.
func ErrUnsupportedDeliveryChannel(channel string) error {
	return errors.NewAppError(errors.CodeInvalidInput, errors.CategoryValidation, "unsupported delivery channel").
		WithMetadata("channel", channel)
}

// ErrDeliveryChannelFailed returns an error indicating delivery failed on channel.
func ErrDeliveryChannelFailed() error {
	return errors.NewLambdaError(errors.CodeInternal, "delivery failed on channel")
}

// Processing errors

// ErrUnmarshalDeliveryRequest returns an error indicating failed to unmarshal delivery request.
func ErrUnmarshalDeliveryRequest(err error) error {
	return errors.WrapError(err, errors.CodeInvalidFormat, errors.CategoryValidation, "failed to unmarshal delivery request")
}

// ErrGetNotification returns an error indicating failed to get notification.
func ErrGetNotification(err error) error {
	return errors.NewStorageInternalError(errors.CodeQueryFailed, "failed to get notification", err)
}

// ErrMarshalPushPayload returns an error indicating failed to marshal push payload.
func ErrMarshalPushPayload(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "failed to marshal push payload", err)
}

// ErrSendPushNotification returns an error indicating failed to send push notification.
func ErrSendPushNotification(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "failed to send push notification", err)
}

// ErrGetWebSocketConnections returns an error indicating failed to get websocket connections.
func ErrGetWebSocketConnections(err error) error {
	return errors.NewStorageInternalError(errors.CodeQueryFailed, "failed to get websocket connections", err)
}

// ErrDeliverWebSocketMessage returns an error indicating failed to deliver to any websocket connections.
func ErrDeliverWebSocketMessage(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "failed to deliver to any websocket connections", err)
}

// ErrMarshalWebSocketMessage returns an error indicating failed to marshal websocket message.
func ErrMarshalWebSocketMessage(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "failed to marshal websocket message", err)
}

// ErrPublishToSNS returns an error indicating failed to publish push notification to SNS.
func ErrPublishToSNS(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "failed to publish push notification to SNS", err)
}

// ErrMarshalScheduledRequest returns an error indicating failed to marshal scheduled notification request.
func ErrMarshalScheduledRequest(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "failed to marshal scheduled notification request", err)
}

// ErrRequeueNotification returns an error indicating failed to requeue scheduled notification.
func ErrRequeueNotification(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "failed to requeue scheduled notification", err)
}

// ErrMarshalRetryRequest returns an error indicating failed to marshal retry request.
func ErrMarshalRetryRequest(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "failed to marshal retry request", err)
}

// ErrScheduleRetry returns an error indicating failed to schedule retry.
func ErrScheduleRetry(err error) error {
	return errors.NewLambdaInternalError(errors.CodeInternal, "failed to schedule retry", err)
}

// Batch processing errors

// ErrPartialBatchFailure returns an error indicating partial batch failure.
func ErrPartialBatchFailure() error {
	return errors.NewLambdaError(errors.CodeSQSProcessingFailed, "partial batch failure")
}
