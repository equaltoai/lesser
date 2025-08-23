package main

import "errors"

// Error constants for notification processor
var (
	// Budget and delivery errors
	ErrNotificationBudgetExceeded = errors.New("notification delivery blocked: user budget exceeded")
	
	// Client initialization errors
	ErrSNSClientNotInitialized        = errors.New("SNS client not initialized")
	ErrAPIGatewayClientNotInitialized = errors.New("API Gateway client not initialized")
	ErrSQSClientNotInitialized        = errors.New("SQS client not initialized")
	
	// Configuration errors
	ErrPushTopicNotConfigured     = errors.New("PUSH_NOTIFICATION_TOPIC_ARN not configured")
	ErrRetryQueueNotConfigured    = errors.New("retry queue URL not configured")
	ErrSQSConfigurationIncomplete = errors.New("SQS client or retry queue URL not configured")
	
	// Channel delivery errors
	ErrUnsupportedDeliveryChannel = errors.New("unsupported delivery channel")
	ErrDeliveryChannelFailed      = errors.New("delivery failed on channel")
	
	// Processing errors
	ErrUnmarshalDeliveryRequest = errors.New("failed to unmarshal delivery request")
	ErrGetNotification          = errors.New("failed to get notification")
	ErrMarshalPushPayload       = errors.New("failed to marshal push payload")
	ErrSendPushNotification     = errors.New("failed to send push notification")
	ErrGetWebSocketConnections  = errors.New("failed to get websocket connections")
	ErrDeliverWebSocketMessage  = errors.New("failed to deliver to any websocket connections")
	ErrMarshalWebSocketMessage  = errors.New("failed to marshal websocket message")
	ErrPublishToSNS             = errors.New("failed to publish push notification to SNS")
	ErrMarshalScheduledRequest  = errors.New("failed to marshal scheduled notification request")
	ErrRequeueNotification      = errors.New("failed to requeue scheduled notification")
	ErrMarshalRetryRequest      = errors.New("failed to marshal retry request")
	ErrScheduleRetry            = errors.New("failed to schedule retry")
	
	// Batch processing errors
	ErrPartialBatchFailure = errors.New("partial batch failure")
)